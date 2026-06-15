package util

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func encodeBase64URL(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func TestValidateVLESSMihomoEncryptionSource_X25519Valid(t *testing.T) {
	source := map[string]interface{}{
		VLESSInboundEncryptionEnabledKey:          true,
		VLESSInboundEncryptionAuthMethodKey:       "x25519",
		VLESSInboundEncryptionModeKey:             "random",
		VLESSInboundEncryptionServerRTTKey:        "600s",
		VLESSInboundEncryptionClientRTTKey:        "0rtt",
		VLESSInboundEncryptionPaddingKey:          VLESSMihomoEncryptionDefaultPadding,
		VLESSInboundEncryptionX25519PrivateKeyKey: encodeBase64URL(bytes.Repeat([]byte{1}, VLESSMihomoX25519DecodedLength)),
		VLESSInboundEncryptionX25519PasswordKey:   encodeBase64URL(bytes.Repeat([]byte{2}, VLESSMihomoX25519DecodedLength)),
	}

	if err := ValidateVLESSMihomoEncryptionSource(source); err != nil {
		t.Fatalf("expected valid x25519 source, got error: %v", err)
	}
}

func TestValidateVLESSMihomoEncryptionSource_X25519InvalidLength(t *testing.T) {
	source := map[string]interface{}{
		VLESSInboundEncryptionEnabledKey:          true,
		VLESSInboundEncryptionAuthMethodKey:       "x25519",
		VLESSInboundEncryptionModeKey:             "random",
		VLESSInboundEncryptionPaddingKey:          VLESSMihomoEncryptionDefaultPadding,
		VLESSInboundEncryptionX25519PrivateKeyKey: encodeBase64URL(bytes.Repeat([]byte{1}, VLESSMihomoX25519DecodedLength-1)),
		VLESSInboundEncryptionX25519PasswordKey:   encodeBase64URL(bytes.Repeat([]byte{2}, VLESSMihomoX25519DecodedLength)),
	}

	if err := ValidateVLESSMihomoEncryptionSource(source); err == nil {
		t.Fatal("expected error for invalid x25519 private key length, got nil")
	}
}

func TestValidateVLESSMihomoEncryptionSource_InvalidPadding(t *testing.T) {
	source := map[string]interface{}{
		VLESSInboundEncryptionEnabledKey:          true,
		VLESSInboundEncryptionAuthMethodKey:       "x25519",
		VLESSInboundEncryptionModeKey:             "random",
		VLESSInboundEncryptionPaddingKey:          "100-1-10",
		VLESSInboundEncryptionX25519PrivateKeyKey: encodeBase64URL(bytes.Repeat([]byte{1}, VLESSMihomoX25519DecodedLength)),
		VLESSInboundEncryptionX25519PasswordKey:   encodeBase64URL(bytes.Repeat([]byte{2}, VLESSMihomoX25519DecodedLength)),
	}

	if err := ValidateVLESSMihomoEncryptionSource(source); err == nil {
		t.Fatal("expected error for invalid padding, got nil")
	}
}

func TestValidateVLESSMihomoEncryptionSource_MLKEMValid(t *testing.T) {
	source := map[string]interface{}{
		VLESSInboundEncryptionEnabledKey:     true,
		VLESSInboundEncryptionAuthMethodKey:  "mlkem768",
		VLESSInboundEncryptionModeKey:        "random",
		VLESSInboundEncryptionServerRTTKey:   "300-600s",
		VLESSInboundEncryptionClientRTTKey:   "0rtt",
		VLESSInboundEncryptionPaddingKey:     VLESSMihomoEncryptionDefaultPadding,
		VLESSInboundEncryptionMLKEMSeedKey:   encodeBase64URL(bytes.Repeat([]byte{3}, VLESSMihomoMLKEMSeedDecodedLength)),
		VLESSInboundEncryptionMLKEMClientKey: encodeBase64URL(bytes.Repeat([]byte{4}, VLESSMihomoMLKEMClientDecodedLength)),
	}

	if err := ValidateVLESSMihomoEncryptionSource(source); err != nil {
		t.Fatalf("expected valid mlkem source, got error: %v", err)
	}
}

func TestValidateVLESSMihomoEncryptionSource_DisabledSkipsValidation(t *testing.T) {
	source := map[string]interface{}{
		VLESSInboundEncryptionEnabledKey: false,
		VLESSInboundEncryptionPaddingKey: "invalid",
	}

	if err := ValidateVLESSMihomoEncryptionSource(source); err != nil {
		t.Fatalf("expected disabled helper to skip validation, got error: %v", err)
	}
}
