package util

import "testing"

func TestFilterTaggedSubscriptionOutbounds(t *testing.T) {
	outbounds := []map[string]interface{}{
		{"type": "vmess", "tag": "vmess-node"},
		{"type": "mieru", "tag": "mieru-node"},
		{"type": "trusttunnel", "tag": "trusttunnel-node"},
		{"type": "shadowtls", "tag": "shadowtls-node"},
	}
	outTags := []string{"vmess-node", "mieru-node", "trusttunnel-node", "shadowtls-node"}

	filteredOutbounds, filteredTags := FilterTaggedSubscriptionOutbounds(
		outbounds,
		outTags,
		SupportsSingboxSubscriptionOutboundType,
	)

	if len(filteredOutbounds) != 2 {
		t.Fatalf("expected 2 supported sing-box outbounds, got %d", len(filteredOutbounds))
	}
	if len(filteredTags) != 2 {
		t.Fatalf("expected 2 supported sing-box tags, got %d", len(filteredTags))
	}
	if filteredTags[0] != "vmess-node" || filteredTags[1] != "shadowtls-node" {
		t.Fatalf("unexpected filtered tags: %#v", filteredTags)
	}
}

func TestSupportsMihomoSubscriptionTypes(t *testing.T) {
	if !SupportsSingboxSubscriptionOutboundType("naive") {
		t.Fatalf("expected naive runtime outbound to be supported by sing-box subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("mieru") {
		t.Fatalf("expected mieru runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("sudoku") {
		t.Fatalf("expected sudoku runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("trusttunnel") {
		t.Fatalf("expected trusttunnel runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("ssh") {
		t.Fatalf("expected ssh runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("snell") {
		t.Fatalf("expected snell runtime outbound to be supported by mihomo subscription conversion")
	}
	if SupportsMihomoSubscriptionOutboundType("shadowtls") {
		t.Fatalf("expected standalone shadowtls runtime outbound to be unsupported for mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionClashProxyType("ss") {
		t.Fatalf("expected ss clash proxy type to be supported by mihomo")
	}
	if !SupportsMihomoSubscriptionClashProxyType("trusttunnel") {
		t.Fatalf("expected trusttunnel clash proxy type to be supported by mihomo")
	}
	if !SupportsMihomoSubscriptionClashProxyType("sudoku") {
		t.Fatalf("expected sudoku clash proxy type to be supported by mihomo")
	}
	if !SupportsMihomoSubscriptionClashProxyType("ssh") {
		t.Fatalf("expected ssh clash proxy type to be supported by mihomo")
	}
	if !SupportsMihomoSubscriptionClashProxyType("snell") {
		t.Fatalf("expected snell clash proxy type to be supported by mihomo")
	}
	if SupportsMihomoSubscriptionClashProxyType("tor") {
		t.Fatalf("expected tor clash proxy type to be unsupported by mihomo")
	}
}
