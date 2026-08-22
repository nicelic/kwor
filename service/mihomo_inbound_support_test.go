package service

import "testing"

func TestMihomoShadowTLSIsNotAnInboundType(t *testing.T) {
	if isSupportedMihomoInboundType("shadowtls") {
		t.Fatal("standalone shadowtls must not be an allowed Mihomo listener type")
	}
	if !isRemovedMihomoInboundType("shadowtls") {
		t.Fatal("standalone shadowtls must be excluded from Mihomo runtime metadata")
	}
	if !isSupportedMihomoInboundType("shadowsocks") {
		t.Fatal("shadowsocks must remain an allowed Mihomo listener type")
	}
	if isSupportedMihomoInboundType("ssh") {
		t.Fatal("ssh must not be accepted as a Mihomo listener type")
	}
	for _, inboundType := range []string{"redirect", "tproxy", "tun"} {
		if !isSupportedMihomoInboundType(inboundType) {
			t.Fatalf("%s must remain an allowed Mihomo listener type", inboundType)
		}
	}
}
