package compressionalgorithm

import (
	"fmt"
	"strings"
)

type acceptEncodingPreferences struct {
	explicit         map[string]float64
	wildcard         float64
	identityQuality  float64
	identityExplicit bool
}

// parseQuality parses the project's bounded decimal q syntax. Positive
// decimals of any precision are accepted because their magnitude is ignored
// during selection; zero remains the explicit refusal value. ParseFloat is
// deliberately not used here because it accepts exponent notation, signs,
// NaN and infinity.
func parseQuality(value string) (float64, bool) {
	if value != strings.TrimSpace(value) {
		return 0, false
	}
	if value == "0" || value == "0." {
		return 0, true
	}
	if strings.HasPrefix(value, "0.") {
		fraction := value[2:]
		if fraction == "" {
			return 0, true
		}
		positive := false
		for _, char := range fraction {
			if char < '0' || char > '9' {
				return 0, false
			}
			if char != '0' {
				positive = true
			}
		}
		if positive {
			return 1, true
		}
		return 0, true
	}
	if value == "1" || value == "1." {
		return 1, true
	}
	if strings.HasPrefix(value, "1.") {
		fraction := value[2:]
		for _, char := range fraction {
			if char != '0' {
				return 0, false
			}
		}
		return 1, true
	}
	return 0, false
}

func parseAcceptEncoding(header string) acceptEncodingPreferences {
	preferences := acceptEncodingPreferences{
		explicit:        make(map[string]float64),
		wildcard:        -1,
		identityQuality: 1,
	}
	for _, item := range strings.Split(header, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		parts := strings.Split(item, ";")
		name := strings.ToLower(strings.TrimSpace(parts[0]))
		if name == "" {
			continue
		}
		quality := 1.0
		qualitySeen := false
		valid := true
		for _, parameter := range parts[1:] {
			parameter = strings.TrimSpace(parameter)
			if parameter == "" {
				valid = false
				break
			}
			keyValue := strings.SplitN(parameter, "=", 2)
			if len(keyValue) != 2 || !strings.EqualFold(strings.TrimSpace(keyValue[0]), "q") {
				valid = false
				break
			}
			if qualitySeen {
				valid = false
				break
			}
			parsed, ok := parseQuality(keyValue[1])
			if !ok {
				valid = false
				break
			}
			quality = parsed
			qualitySeen = true
		}
		if !valid {
			continue
		}
		switch name {
		case "*":
			if preferences.wildcard < 0 || quality < preferences.wildcard {
				preferences.wildcard = quality
			}
		case string(AlgorithmIdentity):
			if !preferences.identityExplicit || quality < preferences.identityQuality {
				preferences.identityQuality = quality
			}
			preferences.identityExplicit = true
		default:
			if previous, exists := preferences.explicit[name]; !exists || quality < previous {
				preferences.explicit[name] = quality
			}
		}
	}
	// RFC 9110: identity is acceptable by default, except when it is
	// explicitly rejected or when *;q=0 rejects all unspecified codings.
	if preferences.wildcard == 0 && !preferences.identityExplicit {
		preferences.identityQuality = 0
	}
	return preferences
}

// SelectEncoding treats a positive qvalue as an acceptance flag and ignores
// its magnitude when choosing among the project's supported codings. The
// fixed Priority order is therefore authoritative whenever at least one
// supported coding has q>0. q=0 remains an explicit refusal. The bool is
// false only when the client rejects every supported coding and identity.
func SelectEncoding(header string) (Algorithm, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return AlgorithmIdentity, true
	}
	preferences := parseAcceptEncoding(header)

	for _, algorithm := range fixedPriorityOrder() {
		quality, ok := preferences.explicit[string(algorithm)]
		if !ok {
			quality = preferences.wildcard
		}
		if quality > 0 {
			return algorithm, true
		}
	}
	if preferences.identityQuality > 0 {
		return AlgorithmIdentity, true
	}
	return AlgorithmIdentity, false
}

// IdentityAcceptable reports whether a response without a Content-Encoding is
// permitted by the request's Accept-Encoding header.
func IdentityAcceptable(header string) bool {
	return parseAcceptEncoding(strings.TrimSpace(header)).identityQuality > 0
}

// ContentEncodingAcceptable reports whether every coding in a response's
// Content-Encoding field is acceptable to the request. An absent
// Accept-Encoding field means that the user agent accepts any coding.
func ContentEncodingAcceptable(contentEncoding string, acceptEncoding string) bool {
	if strings.TrimSpace(contentEncoding) == "" || strings.EqualFold(strings.TrimSpace(contentEncoding), string(AlgorithmIdentity)) {
		return true
	}
	if strings.TrimSpace(acceptEncoding) == "" {
		// This string-only helper cannot distinguish an absent field from an
		// explicitly empty field. Use ContentEncodingAcceptableValues when the
		// caller has access to the original header values.
		return true
	}
	return contentEncodingAcceptable(contentEncoding, acceptEncoding)
}

// ContentEncodingAcceptableValues is the presence-aware form used by HTTP
// handlers. An absent Accept-Encoding field accepts any coding; an explicitly
// empty field accepts no content coding.
func ContentEncodingAcceptableValues(contentEncoding string, acceptValues []string) bool {
	contentEncoding = strings.TrimSpace(contentEncoding)
	if contentEncoding == "" || strings.EqualFold(contentEncoding, string(AlgorithmIdentity)) {
		return true
	}
	if len(acceptValues) == 0 {
		return true
	}
	acceptEncoding := strings.TrimSpace(strings.Join(acceptValues, ","))
	if acceptEncoding == "" {
		return false
	}
	return contentEncodingAcceptable(contentEncoding, acceptEncoding)
}

func contentEncodingAcceptable(contentEncoding string, acceptEncoding string) bool {
	preferences := parseAcceptEncoding(acceptEncoding)
	for _, item := range strings.Split(contentEncoding, ",") {
		name := canonicalEncodingName(strings.ToLower(strings.TrimSpace(item)))
		if name == "" || name == string(AlgorithmIdentity) {
			continue
		}
		quality, ok := preferences.explicit[name]
		if !ok {
			quality = preferences.wildcard
		}
		if quality <= 0 {
			return false
		}
	}
	return true
}

// AcceptEncodingFor builds the project's ordered Accept-Encoding value for a
// selected subset of algorithms. The subset is always emitted in the fixed
// project order, with matching descending q-values, so both list-order and
// q-aware upstreams receive the same preference signal. An empty subset means
// that the caller should remove the header rather than send an empty value.
func AcceptEncodingFor(selected []Algorithm) string {
	if len(selected) == 0 {
		return ""
	}
	allowed := make(map[Algorithm]struct{}, len(selected))
	for _, algorithm := range selected {
		if isSupportedAlgorithm(algorithm) {
			allowed[algorithm] = struct{}{}
		}
	}
	priority := fixedPriorityOrder()
	items := make([]string, 0, len(priority))
	for _, algorithm := range priority {
		if _, ok := allowed[algorithm]; !ok {
			continue
		}
		quality := 1.0 - float64(len(items))*0.001
		items = append(items, fmt.Sprintf("%s;q=%.3f", algorithm, quality))
	}
	return strings.Join(items, ", ")
}

// UpstreamAcceptEncoding advertises every project-supported coding to an
// upstream HTTP service. It remains the compatibility default for callers
// that do not have a per-rule selection.
func UpstreamAcceptEncoding() string {
	priority := fixedPriorityOrder()
	selected := make([]Algorithm, len(priority))
	copy(selected, priority[:])
	return AcceptEncodingFor(selected)
}

// SelectEncodingWithAllowed applies the normal client negotiation rules while
// restricting the result to the configured subset. Identity remains governed
// by the client's header and is available whenever the client permits it.
func SelectEncodingWithAllowed(header string, allowed []Algorithm) (Algorithm, bool) {
	if allowed == nil {
		return SelectEncoding(header)
	}
	if len(allowed) == 0 {
		if IdentityAcceptable(strings.TrimSpace(header)) {
			return AlgorithmIdentity, true
		}
		return AlgorithmIdentity, false
	}
	allowedSet := make(map[Algorithm]struct{}, len(allowed))
	for _, algorithm := range allowed {
		if isSupportedAlgorithm(algorithm) {
			allowedSet[algorithm] = struct{}{}
		}
	}
	preferences := parseAcceptEncoding(strings.TrimSpace(header))
	for _, algorithm := range fixedPriorityOrder() {
		if _, ok := allowedSet[algorithm]; !ok {
			continue
		}
		quality, ok := preferences.explicit[string(algorithm)]
		if !ok {
			quality = preferences.wildcard
		}
		if quality > 0 {
			return algorithm, true
		}
	}
	if preferences.identityQuality > 0 {
		return AlgorithmIdentity, true
	}
	return AlgorithmIdentity, false
}

// ParseContentEncoding parses a possibly stacked Content-Encoding value.  A
// decoder must apply the returned list in reverse order.
func ParseContentEncoding(header string) ([]Algorithm, error) {
	header = strings.TrimSpace(header)
	if header == "" || strings.EqualFold(header, string(AlgorithmIdentity)) {
		return nil, nil
	}
	parts := strings.Split(header, ",")
	result := make([]Algorithm, 0, len(parts))
	for _, part := range parts {
		name := canonicalEncodingName(strings.ToLower(strings.TrimSpace(part)))
		if name == "" || name == string(AlgorithmIdentity) {
			continue
		}
		algorithm := Algorithm(name)
		if !isSupportedAlgorithm(algorithm) {
			return nil, ErrUnsupportedEncoding
		}
		result = append(result, algorithm)
	}
	return result, nil
}

func canonicalEncodingName(name string) string {
	if name == "x-gzip" {
		return string(AlgorithmGzip)
	}
	return name
}

func isSupportedAlgorithm(algorithm Algorithm) bool {
	switch algorithm {
	case AlgorithmZstd, AlgorithmS2, AlgorithmSnappy, AlgorithmBrotli, AlgorithmDeflate, AlgorithmGzip:
		return true
	default:
		return false
	}
}

func AppendVaryAcceptEncoding(header interface {
	Get(string) string
	Set(string, string)
}, value string) {
	current := header.Get("Vary")
	for _, item := range strings.Split(current, ",") {
		if strings.TrimSpace(item) == "*" {
			return
		}
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return
		}
	}
	if strings.TrimSpace(current) == "" {
		header.Set("Vary", value)
		return
	}
	header.Set("Vary", current+", "+value)
}
