package service

import "encoding/json"

type SelfSignedRenewConfig struct {
	Mode               string   `json:"mode"`
	CertificateType    string   `json:"certificateType"`
	Domains            []string `json:"domains"`
	AllowEmptyNames    bool     `json:"allowEmptyNames"`
	KeyAlgorithm       string   `json:"keyAlgorithm"`
	SignatureAlgorithm string   `json:"signatureAlgorithm"`
	DurationValue      int      `json:"durationValue"`
	DurationUnit       string   `json:"durationUnit"`
	AuthorityID        uint     `json:"authorityId"`
	AuthorityName      string   `json:"authorityName"`
	PlatformCode       string   `json:"platformCode"`
	PlatformName       string   `json:"platformName"`
	Identity           string   `json:"identity"`
	IdentityKind       string   `json:"identityKind"`
	DetectionReason    string   `json:"detectionReason"`
}

func marshalSelfSignedRenewConfig(cfg SelfSignedRenewConfig) string {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return ""
	}
	return string(raw)
}
