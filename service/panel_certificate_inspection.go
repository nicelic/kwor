package service

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alireza0/s-ui/network"
)

type PanelCertificateClass string

const (
	PanelCertificateUnknown    PanelCertificateClass = "unknown"
	PanelCertificateMissing    PanelCertificateClass = "missing"
	PanelCertificateTrusted    PanelCertificateClass = "trusted"
	PanelCertificateSelfSigned PanelCertificateClass = "self_signed"
	PanelCertificateInvalid    PanelCertificateClass = "invalid"
)

type PanelCertificateInspection struct {
	Fingerprint  string
	NotBefore    time.Time
	NotAfter     time.Time
	Class        PanelCertificateClass
	Reason       string
	Expired      bool
	NotYetValid  bool
	TrustedChain bool
}

func LoadPanelCertificatePEMFromPaths(certPath string, keyPath string) ([]byte, []byte, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return nil, nil, fmt.Errorf("certificate/key path is empty")
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

func InspectPanelCertificatePaths(certPath string, keyPath string, now time.Time) PanelCertificateInspection {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return PanelCertificateInspection{
			Class:  PanelCertificateMissing,
			Reason: "certificate_path/key_path not configured",
		}
	}

	certPEM, certErr := os.ReadFile(certPath)
	keyPEM, keyErr := os.ReadFile(keyPath)
	return inspectPanelCertificatePEM(certPEM, keyPEM, certPath+"|"+keyPath, certErr, keyErr, now)
}

func InspectPanelCertificatePEM(certPEM []byte, keyPEM []byte, sourceID string, now time.Time) PanelCertificateInspection {
	return inspectPanelCertificatePEM(certPEM, keyPEM, sourceID, nil, nil, now)
}

func inspectPanelCertificatePEM(certPEM []byte, keyPEM []byte, sourceID string, certErr error, keyErr error, now time.Time) PanelCertificateInspection {
	fingerprint := hashPanelCertMaterial(sourceID, certPEM, keyPEM)

	if certErr != nil {
		return PanelCertificateInspection{
			Fingerprint: fingerprint,
			Class:       PanelCertificateInvalid,
			Reason:      "read certificate file failed: " + certErr.Error(),
		}
	}
	if keyErr != nil {
		return PanelCertificateInspection{
			Fingerprint: fingerprint,
			Class:       PanelCertificateInvalid,
			Reason:      "read key file failed: " + keyErr.Error(),
		}
	}
	if len(certPEM) == 0 || len(keyPEM) == 0 {
		return PanelCertificateInspection{
			Fingerprint: fingerprint,
			Class:       PanelCertificateMissing,
			Reason:      "certificate/key pem not configured",
		}
	}

	certPair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return PanelCertificateInspection{
			Fingerprint: fingerprint,
			Class:       PanelCertificateInvalid,
			Reason:      "certificate/key mismatch or parse error: " + err.Error(),
		}
	}

	leaf, err := network.ParseLeafCertificate(&certPair)
	if err != nil {
		return PanelCertificateInspection{
			Fingerprint: fingerprint,
			Class:       PanelCertificateInvalid,
			Reason:      "parse leaf certificate failed: " + err.Error(),
		}
	}

	sum := sha256.Sum256(leaf.Raw)
	fingerprint = hex.EncodeToString(sum[:])

	expired := !now.Before(leaf.NotAfter)
	notYetValid := now.Before(leaf.NotBefore)

	if isPanelSelfSignedCert(leaf) {
		reason := "self-signed certificate"
		if expired {
			reason = "self-signed certificate expired"
		} else if notYetValid {
			reason = "self-signed certificate is not valid yet"
		}
		return PanelCertificateInspection{
			Fingerprint: fingerprint,
			NotBefore:   leaf.NotBefore,
			NotAfter:    leaf.NotAfter,
			Class:       PanelCertificateSelfSigned,
			Reason:      reason,
			Expired:     expired,
			NotYetValid: notYetValid,
		}
	}

	if err := verifyTrustedPanelCertificate(leaf, certPair.Certificate, now); err == nil {
		return PanelCertificateInspection{
			Fingerprint:  fingerprint,
			NotBefore:    leaf.NotBefore,
			NotAfter:     leaf.NotAfter,
			Class:        PanelCertificateTrusted,
			Reason:       "trusted certificate",
			TrustedChain: true,
		}
	}

	trustedChain := false
	if expired || notYetValid {
		probeTime := choosePanelTrustProbeTime(leaf, now)
		if !probeTime.IsZero() && verifyTrustedPanelCertificate(leaf, certPair.Certificate, probeTime) == nil {
			trustedChain = true
		}
	}

	reason := "trusted-chain verification failed"
	if expired {
		reason = "certificate expired"
	} else if notYetValid {
		reason = "certificate is not valid yet"
	}
	if trustedChain && expired {
		reason = "trusted certificate expired"
	}
	if trustedChain && notYetValid {
		reason = "trusted certificate not yet valid"
	}

	return PanelCertificateInspection{
		Fingerprint:  fingerprint,
		NotBefore:    leaf.NotBefore,
		NotAfter:     leaf.NotAfter,
		Class:        PanelCertificateInvalid,
		Reason:       reason,
		Expired:      expired,
		NotYetValid:  notYetValid,
		TrustedChain: trustedChain,
	}
}

func IsUsablePanelPathCertificate(certPath string, keyPath string, now time.Time) bool {
	inspect := InspectPanelCertificatePaths(certPath, keyPath, now)
	switch inspect.Class {
	case PanelCertificateTrusted, PanelCertificateSelfSigned:
		return !inspect.Expired && !inspect.NotYetValid
	default:
		return false
	}
}

func hashPanelCertMaterial(sourceID string, certPEM []byte, keyPEM []byte) string {
	data := []byte(strings.TrimSpace(sourceID) + "|")
	data = append(data, certPEM...)
	data = append(data, keyPEM...)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func isPanelSelfSignedCert(cert *x509.Certificate) bool {
	if cert == nil {
		return false
	}
	if !bytes.Equal(cert.RawIssuer, cert.RawSubject) {
		return false
	}
	return cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) == nil
}

func choosePanelTrustProbeTime(leaf *x509.Certificate, now time.Time) time.Time {
	if leaf == nil {
		return time.Time{}
	}
	if leaf.NotAfter.After(leaf.NotBefore) {
		if now.Before(leaf.NotBefore) {
			probe := leaf.NotBefore.Add(time.Minute)
			if probe.After(leaf.NotAfter) {
				return leaf.NotBefore
			}
			return probe
		}
		if !now.Before(leaf.NotAfter) {
			probe := leaf.NotAfter.Add(-time.Minute)
			if probe.Before(leaf.NotBefore) {
				return leaf.NotBefore
			}
			return probe
		}
	}
	return time.Time{}
}

func verifyTrustedPanelCertificate(leaf *x509.Certificate, chainDER [][]byte, now time.Time) error {
	if leaf == nil {
		return os.ErrInvalid
	}

	var roots *x509.CertPool
	systemRoots, err := x509.SystemCertPool()
	if err == nil && systemRoots != nil {
		roots = systemRoots
	}

	intermediates := x509.NewCertPool()
	for i := 1; i < len(chainDER); i++ {
		cert, parseErr := x509.ParseCertificate(chainDER[i])
		if parseErr == nil && cert != nil {
			intermediates.AddCert(cert)
		}
	}

	opts := x509.VerifyOptions{
		CurrentTime:   now,
		Roots:         roots,
		Intermediates: intermediates,
	}

	_, err = leaf.Verify(opts)
	return err
}
