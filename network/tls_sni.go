package network

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
)

// ParseLeafCertificate extracts the leaf certificate from a TLS keypair.
func ParseLeafCertificate(cert *tls.Certificate) (*x509.Certificate, error) {
	if cert == nil {
		return nil, fmt.Errorf("nil tls certificate")
	}
	if cert.Leaf != nil {
		return cert.Leaf, nil
	}
	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("empty certificate chain")
	}

	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}
	cert.Leaf = leaf
	return leaf, nil
}

// StrictSNIVerifier validates that DNS SNI is covered by certificate SANs.
// IP SNI is intentionally allowed to support IP access with self-signed certs.
func StrictSNIVerifier(leaf *x509.Certificate) func(tls.ConnectionState) error {
	return func(state tls.ConnectionState) error {
		if leaf == nil {
			return nil
		}

		sni := strings.TrimSuffix(strings.TrimSpace(state.ServerName), ".")
		if sni == "" {
			return nil
		}
		if net.ParseIP(sni) != nil {
			return nil
		}
		if err := leaf.VerifyHostname(sni); err != nil {
			return fmt.Errorf("tls sni %q mismatch certificate san: %w", sni, err)
		}

		return nil
	}
}
