package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/alireza0/s-ui/database"

	"github.com/gin-gonic/gin"
)

const apiDefaultRequestMaxBytes int64 = 8 * 1024 * 1024

// TLS certificate derivation accepts at most 2 MiB of PEM material. Keep a
// small bounded envelope for JSON/form encoding overhead without exposing the
// general 8 MiB API limit to certificate parsing endpoints.
const apiTLSCertificateRequestMaxBytes int64 = 4 * 1024 * 1024

// Retrying the runtime rebuild takes no mutation payload. Keep this endpoint
// deliberately tiny and verify that callers are not attempting to replay a
// previously committed save.
const apiSingboxRuntimeRetryRequestMaxBytes int64 = 4 * 1024

// Settings may carry both subscription extensions. Each field is capped at
// 100 MiB after JSON decoding, while quotes, slashes and newlines can expand
// the encoded request body. Keep a bounded envelope for settings writes;
// all other API actions retain the 8 MiB default.
const apiSettingsRequestMaxBytes int64 = 512 * 1024 * 1024

func applyAPIRequestBodyLimit(c *gin.Context, action string) {
	if c == nil || c.Request == nil || c.Request.Body == nil {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, apiRequestBodyLimit(action))
}

func apiRequestBodyLimit(action string) int64 {
	switch action {
	case "singbox-runtime-retry":
		return apiSingboxRuntimeRetryRequestMaxBytes
	case "tlsSha256", "tlsFingerprint", "tlsCertAlgorithm", "tlsCertificateInfo", "tlsSelfSignedTemplate":
		return apiTLSCertificateRequestMaxBytes
	case "save", "settings-patch":
		return apiSettingsRequestMaxBytes
	case "mihomo-dns-save":
		return 64 * 1024
	case "singbox-dns-save":
		return 256 * 1024
	case "singbox-basics-save":
		return 256 * 1024
	case "singbox-route-save", "mihomo-route-save":
		return 2 * 1024 * 1024
	case "system-log-optimization-content", "system-sysctl-optimization-content", "system-linux-dns-optimization-content":
		return 256 * 1024
	case "system-linux-dns-optimization-nameservers":
		return 16 * 1024
	case "importdb":
		return database.MaxDatabaseImportUploadBytes()
	case "restore-db-backup":
		return database.MaxDBBackupArchiveUploadBytes()
	default:
		return apiDefaultRequestMaxBytes
	}
}

func validateSingboxRuntimeRetryPayload(body io.Reader) error {
	if body == nil {
		return nil
	}

	raw, err := io.ReadAll(io.LimitReader(body, apiSingboxRuntimeRetryRequestMaxBytes+1))
	if err != nil {
		return fmt.Errorf("read sing-box runtime retry request: %w", err)
	}
	if int64(len(raw)) > apiSingboxRuntimeRetryRequestMaxBytes {
		return fmt.Errorf("sing-box runtime retry request exceeds %d bytes", apiSingboxRuntimeRetryRequestMaxBytes)
	}

	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil || len(payload) != 0 {
		return fmt.Errorf("sing-box runtime retry request must be an empty JSON object")
	}
	return nil
}
