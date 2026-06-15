package service

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"strings"
)

type tlsTemplateSubjectProfile struct {
	CommonName         string
	Organization       []string
	OrganizationalUnit []string
	Country            []string
	Province           []string
	Locality           []string
}

type tlsSelfSignedTemplateProfile struct {
	Code         string
	Name         string
	Root         tlsTemplateSubjectProfile
	Intermediate tlsTemplateSubjectProfile
	CAURL        string
	OCSPURL      string
	CRLURL       string
}

type TLSSelfSignedTemplateOption struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

var tlsSelfSignedTemplateProfiles = []tlsSelfSignedTemplateProfile{
	{
		Code: "letsencrypt",
		Name: "Let's Encrypt",
		Root: tlsTemplateSubjectProfile{
			CommonName:   "ISRG Root X1",
			Organization: []string{"Internet Security Research Group"},
			Country:      []string{"US"},
		},
		Intermediate: tlsTemplateSubjectProfile{
			CommonName:         "Let's Encrypt R13",
			Organization:       []string{"Let's Encrypt"},
			OrganizationalUnit: []string{"DV TLS Issuing CA"},
			Country:            []string{"US"},
			Province:           []string{"California"},
			Locality:           []string{"San Francisco"},
		},
		CAURL:   "https://letsencrypt.org/certs/2024/r13.pem",
		OCSPURL: "",
		CRLURL:  "r13.c.lencr.org",
	},
	{
		Code: "zerossl",
		Name: "ZeroSSL",
		Root: tlsTemplateSubjectProfile{
			CommonName:   "USERTrust ECC Certification Authority",
			Organization: []string{"The USERTRUST Network"},
			Country:      []string{"US"},
		},
		Intermediate: tlsTemplateSubjectProfile{
			CommonName:         "ZeroSSL ECC DV SSL CA 2",
			Organization:       []string{"ZeroSSL GmbH"},
			OrganizationalUnit: []string{"DV TLS Issuing CA"},
			Country:            []string{"AT"},
			Province:           []string{"Vienna"},
			Locality:           []string{"Vienna"},
		},
		CAURL:   "http://crt.sectigo.com/ZeroSLECCDVSSLCA2.crt",
		OCSPURL: "http://ocsp.sectigo.com",
		CRLURL:  "",
	},
}

func ListTLSSelfSignedTemplateOptions() []TLSSelfSignedTemplateOption {
	options := make([]TLSSelfSignedTemplateOption, 0, len(tlsSelfSignedTemplateProfiles))
	for _, profile := range tlsSelfSignedTemplateProfiles {
		options = append(options, TLSSelfSignedTemplateOption{
			Code: profile.Code,
			Name: profile.Name,
		})
	}
	return options
}

func IsKnownTLSSelfSignedTemplateCode(code string) bool {
	return resolveTLSSelfSignedTemplate(code) != nil
}

func resolveTLSSelfSignedTemplate(code string) *tlsSelfSignedTemplateProfile {
	normalized := strings.ToLower(strings.TrimSpace(code))
	for i := range tlsSelfSignedTemplateProfiles {
		if tlsSelfSignedTemplateProfiles[i].Code == normalized {
			return &tlsSelfSignedTemplateProfiles[i]
		}
	}
	return nil
}

func applyTLSTemplateSubject(subject *pkix.Name, profile tlsTemplateSubjectProfile) {
	if subject == nil {
		return
	}
	subject.CommonName = strings.TrimSpace(profile.CommonName)
	subject.Organization = cloneStringSlice(profile.Organization)
	subject.OrganizationalUnit = cloneStringSlice(profile.OrganizationalUnit)
	subject.Country = cloneStringSlice(profile.Country)
	subject.Province = cloneStringSlice(profile.Province)
	subject.Locality = cloneStringSlice(profile.Locality)
}

func applyTLSTemplateCertificateDetails(cert *x509.Certificate, profile tlsTemplateSubjectProfile, template *tlsSelfSignedTemplateProfile) {
	if cert == nil {
		return
	}
	applyTLSTemplateSubject(&cert.Subject, profile)
	if template == nil {
		return
	}
	cert.IssuingCertificateURL = nonEmptyStrings(template.CAURL)
	cert.OCSPServer = nonEmptyStrings(template.OCSPURL)
	cert.CRLDistributionPoints = nonEmptyStrings(template.CRLURL)
}

func detectTLSSelfSignedTemplateCode(certs []*x509.Certificate) string {
	if len(certs) < 3 {
		return ""
	}
	leaf := certs[0]
	intermediate := certs[1]
	root := certs[2]
	for i := range tlsSelfSignedTemplateProfiles {
		profile := tlsSelfSignedTemplateProfiles[i]
		if !matchesTLSTemplateChain(leaf, intermediate, root, &profile) {
			continue
		}
		return profile.Code
	}
	return ""
}

func matchesTLSTemplateChain(leaf *x509.Certificate, intermediate *x509.Certificate, root *x509.Certificate, template *tlsSelfSignedTemplateProfile) bool {
	if leaf == nil || intermediate == nil || root == nil || template == nil {
		return false
	}
	if leaf.IsCA || !intermediate.IsCA || !root.IsCA {
		return false
	}
	if err := root.CheckSignatureFrom(root); err != nil {
		return false
	}
	if err := intermediate.CheckSignatureFrom(root); err != nil {
		return false
	}
	if err := leaf.CheckSignatureFrom(intermediate); err != nil {
		return false
	}
	if !matchesTLSTemplateCertificate(intermediate, template.Intermediate, template) {
		return false
	}
	if !matchesTLSTemplateCertificate(root, template.Root, nil) {
		return false
	}
	return true
}

func matchesTLSTemplateCertificate(cert *x509.Certificate, profile tlsTemplateSubjectProfile, template *tlsSelfSignedTemplateProfile) bool {
	if cert == nil {
		return false
	}
	if strings.TrimSpace(cert.Subject.CommonName) != strings.TrimSpace(profile.CommonName) {
		return false
	}
	if !equalTrimmedStringSlices(cert.Subject.Organization, profile.Organization) {
		return false
	}
	if !equalTrimmedStringSlices(cert.Subject.OrganizationalUnit, profile.OrganizationalUnit) {
		return false
	}
	if !equalTrimmedStringSlices(cert.Subject.Country, profile.Country) {
		return false
	}
	if !equalTrimmedStringSlices(cert.Subject.Province, profile.Province) {
		return false
	}
	if !equalTrimmedStringSlices(cert.Subject.Locality, profile.Locality) {
		return false
	}
	if template == nil {
		return true
	}
	if !equalTrimmedStringSlices(cert.IssuingCertificateURL, nonEmptyStrings(template.CAURL)) {
		return false
	}
	if !equalTrimmedStringSlices(cert.OCSPServer, nonEmptyStrings(template.OCSPURL)) {
		return false
	}
	if !equalTrimmedStringSlices(cert.CRLDistributionPoints, nonEmptyStrings(template.CRLURL)) {
		return false
	}
	return true
}

func equalTrimmedStringSlices(left []string, right []string) bool {
	leftNormalized := normalizeStringSlice(left)
	rightNormalized := normalizeStringSlice(right)
	if len(leftNormalized) != len(rightNormalized) {
		return false
	}
	for i := range leftNormalized {
		if leftNormalized[i] != rightNormalized[i] {
			return false
		}
	}
	return true
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
