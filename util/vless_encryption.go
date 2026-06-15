package util

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
)

const (
	VLESSMihomoEncryptionPrefix           = "mlkem768x25519plus"
	VLESSMihomoEncryptionDefaultMode      = "random"
	VLESSMihomoEncryptionDefaultServerRTT = "0s"
	VLESSMihomoEncryptionDefaultClientRTT = "1rtt"
	VLESSMihomoEncryptionDefaultPadding   = "100-111-1111.75-0-111.50-0-3333"
	VLESSMihomoEncryptionDefaultAuth      = "x25519"
)

const (
	VLESSMihomoX25519DecodedLength      = 32
	VLESSMihomoMLKEMSeedDecodedLength   = 64
	VLESSMihomoMLKEMClientDecodedLength = 1184
	VLESSMihomoPaddingFirstMinLength    = 35
	VLESSMihomoPaddingTotalMaxLength    = 65553
)

const (
	VLESSInboundEncryptionEnabledKey          = "vless_encryption_enabled"
	VLESSInboundEncryptionAuthMethodKey       = "vless_encryption_auth_method"
	VLESSInboundEncryptionModeKey             = "vless_encryption_mode"
	VLESSInboundEncryptionServerRTTKey        = "vless_encryption_server_rtt"
	VLESSInboundEncryptionClientRTTKey        = "vless_encryption_client_rtt"
	VLESSInboundEncryptionRTTKey              = "vless_encryption_rtt"
	VLESSInboundEncryptionPaddingKey          = "vless_encryption_padding"
	VLESSInboundEncryptionX25519PrivateKeyKey = "vless_encryption_x25519_private_key"
	VLESSInboundEncryptionX25519PasswordKey   = "vless_encryption_x25519_password"
	VLESSInboundEncryptionMLKEMSeedKey        = "vless_encryption_mlkem_seed"
	VLESSInboundEncryptionMLKEMClientKey      = "vless_encryption_mlkem_client"
)

var VLESSInboundEncryptionHelperKeys = []string{
	VLESSInboundEncryptionEnabledKey,
	VLESSInboundEncryptionAuthMethodKey,
	VLESSInboundEncryptionModeKey,
	VLESSInboundEncryptionServerRTTKey,
	VLESSInboundEncryptionClientRTTKey,
	VLESSInboundEncryptionRTTKey,
	VLESSInboundEncryptionPaddingKey,
	VLESSInboundEncryptionX25519PrivateKeyKey,
	VLESSInboundEncryptionX25519PasswordKey,
	VLESSInboundEncryptionMLKEMSeedKey,
	VLESSInboundEncryptionMLKEMClientKey,
}

func ValidateVLESSMihomoEncryptionSource(source map[string]interface{}) error {
	if source == nil {
		return nil
	}

	enabled, hasEnabled := VLESSInboundEncryptionEnabled(source)
	if !hasEnabled || !enabled {
		return nil
	}

	if err := validateVLESSMihomoEncryptionRTT(source); err != nil {
		return err
	}
	if err := validateVLESSMihomoEncryptionPaddingValue(vlessEncryptionString(source[VLESSInboundEncryptionPaddingKey])); err != nil {
		return err
	}

	authMethod := NormalizeVLESSMihomoEncryptionAuthMethod(vlessEncryptionString(source[VLESSInboundEncryptionAuthMethodKey]))
	if err := validateVLESSMihomoEncryptionAuthKeys(source, authMethod); err != nil {
		return err
	}

	if _, ok := BuildVLESSMihomoDecryption(source); !ok {
		return fmt.Errorf("invalid vless encryption: unable to build decryption")
	}
	if _, ok := BuildVLESSMihomoEncryption(source); !ok {
		return fmt.Errorf("invalid vless encryption: unable to build encryption")
	}
	return nil
}

func validateVLESSMihomoEncryptionRTT(source map[string]interface{}) error {
	if source == nil {
		return nil
	}

	serverRTT := strings.ToLower(strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionServerRTTKey])))
	if serverRTT != "" && !isVLESSMihomoServerRTTRaw(serverRTT) {
		return fmt.Errorf("invalid vless encryption server rtt: %s", serverRTT)
	}

	clientRTT := strings.ToLower(strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionClientRTTKey])))
	if clientRTT != "" && clientRTT != "1rtt" && clientRTT != "0rtt" {
		return fmt.Errorf("invalid vless encryption client rtt: %s", clientRTT)
	}

	legacyRTT := strings.ToLower(strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionRTTKey])))
	if legacyRTT != "" && !isVLESSMihomoLegacyRTTRaw(legacyRTT) {
		return fmt.Errorf("invalid vless encryption legacy rtt: %s", legacyRTT)
	}

	return nil
}

func isVLESSMihomoServerRTTRaw(raw string) bool {
	switch raw {
	case "1rtt", "0rtt":
		return true
	default:
		return isVLESSMihomoEncryptionServerRTTValue(raw)
	}
}

func isVLESSMihomoLegacyRTTRaw(raw string) bool {
	switch raw {
	case "1rtt", "0rtt":
		return true
	default:
		return isVLESSMihomoEncryptionServerRTTValue(raw)
	}
}

func validateVLESSMihomoEncryptionPaddingValue(raw string) error {
	padding := NormalizeVLESSMihomoEncryptionPadding(raw)
	segments := splitVLESSMihomoPaddingSegments(padding)
	if len(segments) == 0 {
		return nil
	}

	maxLen := 0
	for i, segment := range segments {
		parts := strings.Split(segment, "-")
		if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return fmt.Errorf("invalid vless encryption padding segment: %s", segment)
		}

		probability, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid vless encryption padding segment: %s", segment)
		}
		minValue, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid vless encryption padding segment: %s", segment)
		}
		maxValue, err := strconv.Atoi(parts[2])
		if err != nil {
			return fmt.Errorf("invalid vless encryption padding segment: %s", segment)
		}

		if i == 0 && (probability < 100 || minValue < VLESSMihomoPaddingFirstMinLength || maxValue < VLESSMihomoPaddingFirstMinLength) {
			return fmt.Errorf("invalid vless encryption first padding segment: probability must be >=100 and min/max must be >=%d", VLESSMihomoPaddingFirstMinLength)
		}

		if i%2 == 0 {
			maxLen += maxVLESSMihomoPaddingValue(minValue, maxValue)
		}
	}

	if maxLen > VLESSMihomoPaddingTotalMaxLength {
		return fmt.Errorf("invalid vless encryption padding: total max length exceeds %d", VLESSMihomoPaddingTotalMaxLength)
	}
	return nil
}

func maxVLESSMihomoPaddingValue(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func validateVLESSMihomoEncryptionAuthKeys(source map[string]interface{}, authMethod string) error {
	if source == nil {
		return nil
	}

	x25519Server := strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionX25519PrivateKeyKey]))
	x25519Client := strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionX25519PasswordKey]))
	mlkemServer := strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionMLKEMSeedKey]))
	mlkemClient := strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionMLKEMClientKey]))

	if authMethod == "mlkem768" {
		if mlkemServer == "" {
			return fmt.Errorf("missing vless encryption mlkem seed")
		}
		if mlkemClient == "" {
			return fmt.Errorf("missing vless encryption mlkem client key")
		}
		if err := validateVLESSMihomoBase64DecodedLength(mlkemServer, VLESSMihomoMLKEMSeedDecodedLength, "mlkem seed"); err != nil {
			return err
		}
		if err := validateVLESSMihomoBase64DecodedLength(mlkemClient, VLESSMihomoMLKEMClientDecodedLength, "mlkem client key"); err != nil {
			return err
		}
		return nil
	}

	if x25519Server == "" {
		return fmt.Errorf("missing vless encryption x25519 private key")
	}
	if x25519Client == "" {
		return fmt.Errorf("missing vless encryption x25519 password")
	}
	if err := validateVLESSMihomoBase64DecodedLength(x25519Server, VLESSMihomoX25519DecodedLength, "x25519 private key"); err != nil {
		return err
	}
	if err := validateVLESSMihomoBase64DecodedLength(x25519Client, VLESSMihomoX25519DecodedLength, "x25519 password"); err != nil {
		return err
	}
	return nil
}

func validateVLESSMihomoBase64DecodedLength(raw string, expected int, fieldName string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("missing vless encryption %s", fieldName)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("invalid vless encryption %s: must be base64url without padding", fieldName)
	}
	if len(decoded) != expected {
		return fmt.Errorf("invalid vless encryption %s: decoded length %d, expected %d", fieldName, len(decoded), expected)
	}
	return nil
}

func VLESSInboundEncryptionEnabled(source map[string]interface{}) (bool, bool) {
	if source == nil {
		return false, false
	}
	return toVLESSBool(source[VLESSInboundEncryptionEnabledKey])
}

func NormalizeVLESSMihomoEncryptionMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "native":
		return "native"
	case "xorpub":
		return "xorpub"
	case "random":
		return "random"
	default:
		return VLESSMihomoEncryptionDefaultMode
	}
}

func NormalizeVLESSMihomoEncryptionAuthMethod(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "mlkem768", "ml-kem-768", "mlkem":
		return "mlkem768"
	default:
		return VLESSMihomoEncryptionDefaultAuth
	}
}

func NormalizeVLESSMihomoEncryptionServerRTT(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "1rtt":
		return "0s"
	case "0rtt":
		return "600s"
	}
	if isVLESSMihomoEncryptionServerRTTValue(value) {
		return value
	}
	return VLESSMihomoEncryptionDefaultServerRTT
}

func NormalizeVLESSMihomoEncryptionClientRTT(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0rtt":
		return "0rtt"
	case "1rtt":
		return "1rtt"
	default:
		return VLESSMihomoEncryptionDefaultClientRTT
	}
}

func ResolveVLESSMihomoLegacyRTTPair(raw string) (string, string) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "600s", "300-600s":
		return value, "0rtt"
	case "0rtt":
		return "600s", "0rtt"
	case "1rtt", "0s":
		return "0s", "1rtt"
	default:
		if isVLESSMihomoEncryptionServerRTTValue(value) {
			if value == "0s" {
				return "0s", "1rtt"
			}
			return value, "0rtt"
		}
		return VLESSMihomoEncryptionDefaultServerRTT, VLESSMihomoEncryptionDefaultClientRTT
	}
}

func ResolveVLESSMihomoEncryptionRTTPair(raw string) (string, string) {
	return ResolveVLESSMihomoLegacyRTTPair(raw)
}

func ResolveVLESSMihomoEncryptionRTTPairFromSource(source map[string]interface{}) (string, string) {
	if source == nil {
		return VLESSMihomoEncryptionDefaultServerRTT, VLESSMihomoEncryptionDefaultClientRTT
	}

	serverRTT := VLESSMihomoEncryptionDefaultServerRTT
	clientRTT := VLESSMihomoEncryptionDefaultClientRTT
	serverRaw := vlessEncryptionString(source[VLESSInboundEncryptionServerRTTKey])
	clientRaw := vlessEncryptionString(source[VLESSInboundEncryptionClientRTTKey])
	hasServerRTT := serverRaw != ""
	hasClientRTT := clientRaw != ""
	if hasServerRTT || hasClientRTT {
		if hasServerRTT {
			serverRTT = NormalizeVLESSMihomoEncryptionServerRTT(serverRaw)
		}
		if hasClientRTT {
			clientRTT = NormalizeVLESSMihomoEncryptionClientRTT(clientRaw)
		}
		return serverRTT, clientRTT
	}

	legacyRaw := vlessEncryptionString(source[VLESSInboundEncryptionRTTKey])
	legacyServerRTT, legacyClientRTT := ResolveVLESSMihomoLegacyRTTPair(legacyRaw)
	if legacyRaw != "" {
		serverRTT = legacyServerRTT
		clientRTT = legacyClientRTT
	}

	return serverRTT, clientRTT
}

func NormalizeVLESSMihomoEncryptionPadding(raw string) string {
	normalized := strings.TrimSpace(raw)
	normalized = strings.Trim(normalized, ".")
	if normalized == "" {
		return VLESSMihomoEncryptionDefaultPadding
	}
	return normalized
}

func BuildVLESSMihomoDecryption(source map[string]interface{}) (string, bool) {
	if source == nil {
		return "", false
	}

	mode := NormalizeVLESSMihomoEncryptionMode(vlessEncryptionString(source[VLESSInboundEncryptionModeKey]))
	serverRTT, _ := ResolveVLESSMihomoEncryptionRTTPairFromSource(source)
	padding := NormalizeVLESSMihomoEncryptionPadding(vlessEncryptionString(source[VLESSInboundEncryptionPaddingKey]))
	authMethod := NormalizeVLESSMihomoEncryptionAuthMethod(vlessEncryptionString(source[VLESSInboundEncryptionAuthMethodKey]))
	authKey := resolveVLESSMihomoAuthKey(source, authMethod, true)
	if authKey == "" {
		return "", false
	}

	parts := []string{VLESSMihomoEncryptionPrefix, mode, serverRTT}
	parts = append(parts, splitVLESSMihomoPaddingSegments(padding)...)
	parts = append(parts, authKey)
	return strings.Join(parts, "."), true
}

func BuildVLESSMihomoEncryption(source map[string]interface{}) (string, bool) {
	if source == nil {
		return "", false
	}

	mode := NormalizeVLESSMihomoEncryptionMode(vlessEncryptionString(source[VLESSInboundEncryptionModeKey]))
	_, clientRTT := ResolveVLESSMihomoEncryptionRTTPairFromSource(source)
	padding := NormalizeVLESSMihomoEncryptionPadding(vlessEncryptionString(source[VLESSInboundEncryptionPaddingKey]))
	authMethod := NormalizeVLESSMihomoEncryptionAuthMethod(vlessEncryptionString(source[VLESSInboundEncryptionAuthMethodKey]))
	authKey := resolveVLESSMihomoAuthKey(source, authMethod, false)
	if authKey == "" {
		return "", false
	}

	parts := []string{VLESSMihomoEncryptionPrefix, mode, clientRTT}
	parts = append(parts, splitVLESSMihomoPaddingSegments(padding)...)
	parts = append(parts, authKey)
	return strings.Join(parts, "."), true
}

func resolveVLESSMihomoAuthKey(source map[string]interface{}, authMethod string, serverSide bool) string {
	if source == nil {
		return ""
	}

	x25519Server := strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionX25519PrivateKeyKey]))
	x25519Client := strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionX25519PasswordKey]))
	mlkemServer := strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionMLKEMSeedKey]))
	mlkemClient := strings.TrimSpace(vlessEncryptionString(source[VLESSInboundEncryptionMLKEMClientKey]))

	if serverSide {
		if authMethod == "mlkem768" {
			if mlkemServer != "" {
				return mlkemServer
			}
			if x25519Server != "" {
				return x25519Server
			}
			return ""
		}
		if x25519Server != "" {
			return x25519Server
		}
		if mlkemServer != "" {
			return mlkemServer
		}
		return ""
	}

	if authMethod == "mlkem768" {
		if mlkemClient != "" {
			return mlkemClient
		}
		if x25519Client != "" {
			return x25519Client
		}
		return ""
	}
	if x25519Client != "" {
		return x25519Client
	}
	if mlkemClient != "" {
		return mlkemClient
	}
	return ""
}

func splitVLESSMihomoPaddingSegments(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, ".")
	if raw == "" {
		return nil
	}

	segments := strings.Split(raw, ".")
	normalized := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		normalized = append(normalized, segment)
	}
	return normalized
}

func isVLESSMihomoEncryptionServerRTTValue(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || !strings.HasSuffix(value, "s") {
		return false
	}

	body := strings.TrimSuffix(value, "s")
	if body == "" {
		return false
	}

	if strings.Contains(body, "-") {
		parts := strings.SplitN(body, "-", 2)
		if len(parts) != 2 {
			return false
		}
		return isDigits(parts[0]) && isDigits(parts[1])
	}

	return isDigits(body)
}

func isDigits(raw string) bool {
	if raw == "" {
		return false
	}
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func vlessEncryptionString(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []string:
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				return item
			}
		}
	case []interface{}:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func toVLESSBool(raw interface{}) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case int:
		return value != 0, true
	case int32:
		return value != 0, true
	case int64:
		return value != 0, true
	case float32:
		return value != 0, true
	case float64:
		return value != 0, true
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return false, false
		}
		if value == "1" {
			return true, true
		}
		if value == "0" {
			return false, true
		}
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed, true
		}
	}
	return false, false
}
