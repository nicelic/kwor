package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	compressionalgorithm "github.com/alireza0/s-ui/compression/Compression-algorithm"
	"github.com/alireza0/s-ui/database/model"
)

// Zero selects the per-algorithm defaults in NewHTTPResponseWriter: zstd 8,
// general codecs 6. Keeping this as a sentinel avoids applying zstd's level
// model to Brotli, S2, DEFLATE or Gzip.
const reverseProxyCompressionLevel = 0

var reverseProxyCompressionAlgorithmOrder = [...]string{"zstd", "s2", "snappy", "br", "deflate", "gzip"}

const reverseProxyCompressionDisabledStorageValue = "[]"
const reverseProxyCompressionEmptyStorageValue = "[ ]"

func defaultReverseProxyCompressionAlgorithms() []string {
	result := make([]string, len(reverseProxyCompressionAlgorithmOrder))
	copy(result, reverseProxyCompressionAlgorithmOrder[:])
	return result
}

func reverseProxyCompressionFlag(value *bool) bool {
	return value == nil || *value
}

func normalizeReverseProxyCompressionAlgorithms(values []string) ([]string, error) {
	if values == nil {
		return defaultReverseProxyCompressionAlgorithms(), nil
	}
	if len(values) == 0 {
		return []string{}, nil
	}
	selected := make(map[string]struct{}, len(values))
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		valid := false
		for _, supported := range reverseProxyCompressionAlgorithmOrder {
			if name == supported {
				valid = true
				break
			}
		}
		if !valid {
			return nil, fmt.Errorf("unsupported reverse proxy compression algorithm: %s", name)
		}
		selected[name] = struct{}{}
	}
	result := make([]string, 0, len(selected))
	for _, supported := range reverseProxyCompressionAlgorithmOrder {
		if _, ok := selected[supported]; ok {
			result = append(result, supported)
		}
	}
	return result, nil
}

func reverseProxyStoredCompressionAlgorithms(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == reverseProxyCompressionDisabledStorageValue {
		return []string{}
	}
	if trimmed == "" {
		return defaultReverseProxyCompressionAlgorithms()
	}
	values := decodeReverseProxyList(raw)
	normalized, err := normalizeReverseProxyCompressionAlgorithms(values)
	if err != nil {
		return defaultReverseProxyCompressionAlgorithms()
	}
	return normalized
}

func reverseProxyCompressionSettingsFromModel(enabled bool, raw string) (bool, []string) {
	if strings.TrimSpace(raw) == reverseProxyCompressionDisabledStorageValue {
		return false, []string{}
	}
	// Empty columns are legacy rows created before the per-side setting existed.
	if strings.TrimSpace(raw) == "" {
		return true, defaultReverseProxyCompressionAlgorithms()
	}
	if !enabled {
		return false, []string{}
	}
	return true, reverseProxyStoredCompressionAlgorithms(raw)
}

func reverseProxyCompressionStorageValue(enabled bool, values []string) string {
	if !enabled {
		return reverseProxyCompressionDisabledStorageValue
	}
	normalized, err := normalizeReverseProxyCompressionAlgorithms(values)
	if err != nil {
		normalized = defaultReverseProxyCompressionAlgorithms()
	}
	if len(normalized) == 0 {
		return reverseProxyCompressionEmptyStorageValue
	}
	return encodeReverseProxyList(normalized)
}

func reverseProxyCodecAlgorithms(values []string) []compressionalgorithm.Algorithm {
	result := make([]compressionalgorithm.Algorithm, 0, len(values))
	for _, value := range values {
		result = append(result, compressionalgorithm.Algorithm(value))
	}
	return result
}

func reverseProxyProtocolSupportsCompression(protocol string, alias string) bool {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	alias = normalizeReverseProxyProtocolAlias(alias, protocol)
	if reverseProxyIsWebSocketAlias(alias) {
		return false
	}
	if reverseProxyProtocolIsDNS(alias) {
		return alias == reverseProxyDNSProtocolDoH || alias == reverseProxyDNSProtocolDoHH3
	}
	return protocol == reverseProxyProtocolHTTP || protocol == reverseProxyProtocolHTTPS
}

func reverseProxyListenCompressionOptions(rule *model.ReverseProxyRule) (bool, []compressionalgorithm.Algorithm) {
	if rule == nil || !reverseProxyProtocolSupportsCompression(rule.ListenProtocol, rule.ListenProtocolAlias) {
		return false, []compressionalgorithm.Algorithm{}
	}
	// An empty legacy column predates the enable flag and means the historical
	// all-algorithm default. Any non-empty value with a false flag is explicit.
	if !rule.ListenCompressionEnabled {
		if strings.TrimSpace(rule.ListenCompressionAlgorithms) == "" {
			return true, reverseProxyCodecAlgorithms(defaultReverseProxyCompressionAlgorithms())
		}
		return false, []compressionalgorithm.Algorithm{}
	}
	return true, reverseProxyCodecAlgorithms(reverseProxyStoredCompressionAlgorithms(rule.ListenCompressionAlgorithms))
}

func reverseProxyListenAcceptEncoding(rule *model.ReverseProxyRule) string {
	enabled, algorithms := reverseProxyListenCompressionOptions(rule)
	if !enabled {
		return ""
	}
	return compressionalgorithm.AcceptEncodingFor(algorithms)
}

func reverseProxyTargetAcceptEncoding(rule *model.ReverseProxyRule) string {
	if rule == nil || !reverseProxyProtocolSupportsCompression(rule.TargetProtocol, rule.TargetProtocolAlias) {
		return ""
	}
	if !rule.TargetCompressionEnabled {
		if strings.TrimSpace(rule.TargetCompressionAlgorithms) == "" {
			return compressionalgorithm.UpstreamAcceptEncoding()
		}
		return ""
	}
	return compressionalgorithm.AcceptEncodingFor(reverseProxyCodecAlgorithms(reverseProxyStoredCompressionAlgorithms(rule.TargetCompressionAlgorithms)))
}

func setReverseProxyAcceptEncoding(header http.Header, value string) {
	if strings.TrimSpace(value) == "" {
		header.Del("Accept-Encoding")
		return
	}
	header.Set("Accept-Encoding", value)
}

func reverseProxyDecodeUpstreamResponse(resp *http.Response, maxDecodedBytes int64) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	if (resp.StatusCode >= 100 && resp.StatusCode < 200) || resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusResetContent || resp.StatusCode == http.StatusNotModified {
		return nil
	}
	if resp.Request != nil && resp.Request.Method == http.MethodHead {
		return nil
	}
	if resp.Request != nil && resp.Request.Header.Get("Range") != "" {
		return nil
	}
	if resp.Header.Get("Content-Range") != "" {
		return nil
	}
	header := strings.TrimSpace(strings.Join(resp.Header.Values("Content-Encoding"), ","))
	if header == "" || strings.EqualFold(header, string(compressionalgorithm.AlgorithmIdentity)) {
		resp.Header.Del("Content-Encoding")
		return nil
	}
	decoded, err := compressionalgorithm.NewDecoder(resp.Body, header, maxDecodedBytes)
	if err != nil {
		// Unknown upstream extensions are passed through unchanged. The response
		// writer later checks the client's Accept-Encoding contract; known codec
		// initialization failures must fail the proxy response.
		if errors.Is(err, compressionalgorithm.ErrUnsupportedEncoding) {
			return nil
		}
		return err
	}
	resp.Body = decoded
	resp.ContentLength = -1
	resp.TransferEncoding = nil
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Content-Length")
	resp.Header.Del("ETag")
	resp.Header.Del("Content-MD5")
	resp.Header.Del("Digest")
	resp.Header.Del("Content-Digest")
	return nil
}
