package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// sanitizeMihomoShadowQUICInboundOptions is intentionally strict. The panel
// deliberately excludes generic listener route/dial/TLS settings for
// ShadowQUIC, so storing only its supported panel schema prevents accidental
// runtime YAML generation from stale UI fields.
func sanitizeMihomoShadowQUICInboundOptions(inbound *model.MihomoInbound) error {
	if inbound == nil || !strings.EqualFold(strings.TrimSpace(inbound.Type), "shadowquic") {
		return nil
	}

	inbound.TlsId = 0
	inbound.Tls = nil

	options := map[string]interface{}{}
	if len(inbound.Options) > 0 {
		if err := json.Unmarshal(inbound.Options, &options); err != nil {
			return fmt.Errorf("parse shadowquic inbound options: %w", err)
		}
		if options == nil {
			options = map[string]interface{}{}
		}
	}

	clean := map[string]interface{}{}
	if listen := strings.TrimSpace(firstString(options["listen"])); listen != "" {
		clean["listen"] = listen
	}
	if listenPort, ok := toInt(options["listen_port"]); ok && listenPort > 0 && listenPort <= 65535 {
		clean["listen_port"] = listenPort
	} else if listenPort, ok := toInt(options["port"]); ok && listenPort > 0 && listenPort <= 65535 {
		clean["listen_port"] = listenPort
	}

	upstreamSource := shadowQUICInboundMap(options["jls-upstream"])
	if canonical := shadowQUICInboundMap(options["jls_upstream"]); canonical != nil {
		if upstreamSource == nil {
			upstreamSource = map[string]interface{}{}
		}
		for key, value := range canonical {
			upstreamSource[key] = value
		}
	}
	upstream, err := sanitizeMihomoShadowQUICJLSUpstream(upstreamSource)
	if err != nil {
		return err
	}
	clean["jls_upstream"] = upstream

	if alpn := util.NormalizeMihomoShadowQUICALPN(shadowQUICInboundFirst(options, "alpn")); len(alpn) > 0 {
		clean["alpn"] = alpn
	}
	if versions := util.NormalizeMihomoShadowQUICVersions(shadowQUICInboundFirst(options, "quic_versions", "quic-versions")); len(versions) > 0 {
		clean["quic_versions"] = versions
	}
	shadowQUICInboundCopyBool(options, clean, "zero_rtt", "zero_rtt", "zero-rtt")
	if controller, ok := util.NormalizeMihomoShadowQUICCongestionController(shadowQUICInboundFirst(options, "congestion_controller", "congestion-controller")); ok {
		clean["congestion_controller"] = controller
	}
	shadowQUICInboundCopyNonNegativeInt(options, clean, "up", "up")
	shadowQUICInboundCopyNonNegativeInt(options, clean, "down", "down")
	shadowQUICInboundCopyBool(options, clean, "ignore_client_bandwidth", "ignore_client_bandwidth", "ignore-client-bandwidth")
	shadowQUICInboundCopyNonNegativeInt(options, clean, "cwnd", "cwnd")
	if profile, ok := util.NormalizeMihomoBBRProfile(shadowQUICInboundFirst(options, "bbr_profile", "bbr-profile")); ok {
		clean["bbr_profile"] = profile
	}
	shadowQUICInboundCopyNonNegativeInt(options, clean, "max_idle_time", "max_idle_time", "max-idle-time")
	shadowQUICInboundCopyNonNegativeInt(options, clean, "max_datagram_frame_size", "max_datagram_frame_size", "max-datagram-frame-size")
	shadowQUICInboundCopyNonNegativeInt(options, clean, "recv_window_conn", "recv_window_conn", "recv-window-conn")
	shadowQUICInboundCopyNonNegativeInt(options, clean, "recv_window", "recv_window")
	shadowQUICInboundCopyBool(options, clean, "disable_mtu_discovery", "disable_mtu_discovery", "disable-mtu-discovery")

	encoded, err := json.Marshal(clean)
	if err != nil {
		return fmt.Errorf("marshal shadowquic inbound options: %w", err)
	}
	inbound.Options = json.RawMessage(encoded)
	return nil
}

func validateMihomoShadowQUICJLSUpstreamProxy(tx *gorm.DB, inbound *model.MihomoInbound) error {
	if inbound == nil || !strings.EqualFold(strings.TrimSpace(inbound.Type), "shadowquic") {
		return nil
	}
	if tx == nil {
		return fmt.Errorf("shadowquic jls-upstream.proxy validation requires a database transaction")
	}

	proxy, err := shadowQUICJLSProxyFromOptions(inbound.Options)
	if err != nil {
		return err
	}
	if proxy == "" {
		return nil
	}
	targets, err := loadMihomoRouteTargets(tx)
	if err != nil {
		return err
	}
	normalized, ok := normalizeMihomoTarget(proxy, targets)
	if !ok || normalized == "REJECT" || normalized == "REJECT-DROP" {
		return fmt.Errorf("shadowquic jls-upstream.proxy target %q is not a supported Mihomo proxy or proxy group", proxy)
	}
	options := map[string]interface{}{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		return fmt.Errorf("parse shadowquic inbound options: %w", err)
	}
	upstream, ok := options["jls_upstream"].(map[string]interface{})
	if !ok || upstream == nil {
		return fmt.Errorf("shadowquic jls-upstream.proxy normalization requires jls_upstream")
	}
	upstream["proxy"] = normalized
	encoded, err := json.Marshal(options)
	if err != nil {
		return fmt.Errorf("marshal shadowquic inbound options: %w", err)
	}
	inbound.Options = json.RawMessage(encoded)
	return nil
}

func shadowQUICJLSProxyFromOptions(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}

	options := map[string]interface{}{}
	if err := json.Unmarshal(raw, &options); err != nil {
		return "", fmt.Errorf("parse shadowquic inbound options: %w", err)
	}

	proxy := ""
	for _, key := range []string{"jls-upstream", "jls_upstream"} {
		upstream, ok := options[key].(map[string]interface{})
		if !ok || upstream == nil {
			continue
		}
		if candidate := normalizeMihomoShadowQUICJLSProxyTarget(firstString(upstream["proxy"])); candidate != "" {
			proxy = candidate
		}
	}
	return proxy, nil
}

func findMihomoShadowQUICJLSProxyInboundTags(tx *gorm.DB, targets ...string) ([]string, error) {
	if tx == nil {
		return nil, fmt.Errorf("shadowquic jls-upstream.proxy lookup requires a database transaction")
	}

	targetSet := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target != "" && normalizeMihomoShadowQUICJLSProxyTarget(target) != "DIRECT" {
			targetSet[target] = struct{}{}
		}
	}
	if len(targetSet) == 0 {
		return nil, nil
	}

	var inbounds []model.MihomoInbound
	if err := tx.Model(&model.MihomoInbound{}).
		Select("id", "tag", "options").
		Where("lower(type) = ?", "shadowquic").
		Find(&inbounds).Error; err != nil {
		return nil, err
	}

	matched := make([]string, 0)
	for _, inbound := range inbounds {
		proxy, err := shadowQUICJLSProxyFromOptions(inbound.Options)
		if err != nil {
			return nil, err
		}
		if _, exists := targetSet[proxy]; !exists {
			continue
		}
		tag := strings.TrimSpace(inbound.Tag)
		if tag == "" {
			tag = fmt.Sprintf("id=%d", inbound.Id)
		}
		matched = append(matched, tag)
	}
	return matched, nil
}

func replaceMihomoShadowQUICJLSProxyReferences(tx *gorm.DB, oldTarget, newTarget string) error {
	oldTarget = strings.TrimSpace(oldTarget)
	if normalizeMihomoShadowQUICJLSProxyTarget(oldTarget) == "DIRECT" {
		return nil
	}
	newTarget = normalizeMihomoShadowQUICJLSProxyTarget(newTarget)
	if oldTarget == "" || newTarget == "" || oldTarget == newTarget {
		return nil
	}
	if tx == nil {
		return fmt.Errorf("shadowquic jls-upstream.proxy update requires a database transaction")
	}

	var inbounds []model.MihomoInbound
	if err := tx.Model(&model.MihomoInbound{}).
		Select("id", "options").
		Where("lower(type) = ?", "shadowquic").
		Find(&inbounds).Error; err != nil {
		return err
	}

	for _, inbound := range inbounds {
		options := map[string]interface{}{}
		if err := json.Unmarshal(inbound.Options, &options); err != nil {
			return fmt.Errorf("parse shadowquic inbound options: %w", err)
		}

		changed := false
		for _, key := range []string{"jls-upstream", "jls_upstream"} {
			upstream, ok := options[key].(map[string]interface{})
			if !ok || upstream == nil || normalizeMihomoShadowQUICJLSProxyTarget(firstString(upstream["proxy"])) != oldTarget {
				continue
			}
			upstream["proxy"] = newTarget
			changed = true
		}
		if !changed {
			continue
		}

		encoded, err := json.Marshal(options)
		if err != nil {
			return fmt.Errorf("marshal shadowquic inbound options: %w", err)
		}
		if err := tx.Model(&model.MihomoInbound{}).Where("id = ?", inbound.Id).Update("options", json.RawMessage(encoded)).Error; err != nil {
			return err
		}
	}

	return nil
}

func sanitizeMihomoShadowQUICOutboundRaw(raw json.RawMessage, outboundType string) json.RawMessage {
	if !strings.EqualFold(strings.TrimSpace(outboundType), "shadowquic") || len(raw) == 0 {
		return raw
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return raw
	}
	util.SanitizeMihomoShadowQUICOutbound(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return json.RawMessage(encoded)
}

// ensureMihomoShadowQUICClientCredentials repairs the per-client credential
// block required by a ShadowQUIC listener. It intentionally stores only the
// two fields accepted by Mihomo's users schema.
func ensureMihomoShadowQUICClientCredentials(client *model.MihomoClient) (bool, error) {
	if client == nil {
		return false, nil
	}

	configs := map[string]interface{}{}
	rawConfig := strings.TrimSpace(string(client.Config))
	if rawConfig != "" && rawConfig != "null" {
		if err := json.Unmarshal(client.Config, &configs); err != nil {
			return false, fmt.Errorf("parse mihomo client %q config: %w", client.Name, err)
		}
		if configs == nil {
			configs = map[string]interface{}{}
		}
	}

	existing, _ := configs["shadowquic"].(map[string]interface{})
	username := ""
	password := ""
	if existing != nil {
		username = strings.TrimSpace(firstString(existing["username"]))
		if username == "" {
			username = strings.TrimSpace(firstString(existing["name"]))
		}
		password = strings.TrimSpace(firstString(existing["password"]))
	}
	if username == "" {
		username = strings.TrimSpace(client.Name)
	}
	if username == "" {
		return false, fmt.Errorf("mihomo shadowquic client username is required")
	}
	if password == "" {
		password = common.Random(32)
	}

	if existing != nil && len(existing) == 2 && existing["username"] == username && existing["password"] == password {
		return false, nil
	}

	configs["shadowquic"] = map[string]interface{}{
		"username": username,
		"password": password,
	}
	encoded, err := json.Marshal(configs)
	if err != nil {
		return false, fmt.Errorf("marshal mihomo client %q config: %w", client.Name, err)
	}
	client.Config = json.RawMessage(encoded)
	return true, nil
}

func ensureMihomoShadowQUICCredentialsForClientBindings(tx *gorm.DB, client *model.MihomoClient) (bool, error) {
	if tx == nil || client == nil {
		return false, nil
	}

	inboundIDs, err := parseClientInboundIDs(client.Inbounds)
	if err != nil {
		return false, err
	}
	if len(inboundIDs) == 0 {
		return false, nil
	}

	var count int64
	if err := tx.Model(model.MihomoInbound{}).
		Where("id IN ? AND lower(type) = ?", inboundIDs, "shadowquic").
		Count(&count).Error; err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	return ensureMihomoShadowQUICClientCredentials(client)
}

// repairMihomoShadowQUICInboundClientCredentials handles records created
// before ShadowQUIC credentials were provisioned during binding. Config
// regeneration invokes it before reading users, so the repaired values are
// immediately used by the listener and later client subscriptions.
func repairMihomoShadowQUICInboundClientCredentials(db *gorm.DB, inboundID uint) error {
	if db == nil || inboundID == 0 {
		return nil
	}

	var clients []model.MihomoClient
	if err := db.Model(model.MihomoClient{}).Find(&clients).Error; err != nil {
		return err
	}
	clients = filterMihomoClientsBoundToInboundIDs(clients, []uint{inboundID})

	for index := range clients {
		changed, err := ensureMihomoShadowQUICClientCredentials(&clients[index])
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if err := db.Model(model.MihomoClient{}).Where("id = ?", clients[index].Id).Update("config", clients[index].Config).Error; err != nil {
			return err
		}
	}

	return nil
}

func sanitizeMihomoShadowQUICJLSUpstream(source map[string]interface{}) (map[string]interface{}, error) {
	if source == nil {
		return nil, fmt.Errorf("shadowquic jls-upstream.addr is required")
	}

	addr, inferredSNI, err := util.NormalizeMihomoShadowQUICJLSUpstreamAddr(firstString(source["addr"]))
	if err != nil {
		return nil, err
	}
	clean := map[string]interface{}{
		"addr": addr,
	}
	if sni := strings.TrimSpace(firstString(source["sni"])); sni != "" {
		clean["sni"] = sni
	} else {
		clean["sni"] = inferredSNI
	}
	if proxy := normalizeMihomoShadowQUICJLSProxyTarget(firstString(source["proxy"])); proxy != "" {
		clean["proxy"] = proxy
	}
	shadowQUICInboundCopyNonNegativeInt(source, clean, "rate_limit", "rate_limit", "rate-limit")
	return clean, nil
}

func normalizeMihomoShadowQUICJLSProxyTarget(raw string) string {
	target := strings.TrimSpace(raw)
	if strings.EqualFold(target, "DIRECT") {
		return "DIRECT"
	}
	return target
}

func shadowQUICInboundMap(raw interface{}) map[string]interface{} {
	value, ok := raw.(map[string]interface{})
	if !ok || value == nil {
		return nil
	}
	result := make(map[string]interface{}, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func shadowQUICInboundFirst(source map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := source[key]; ok {
			return value
		}
	}
	return nil
}

func shadowQUICInboundCopyString(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	if value := strings.TrimSpace(firstString(shadowQUICInboundFirst(source, sourceKeys...))); value != "" {
		target[targetKey] = value
	}
}

func shadowQUICInboundCopyStringSlice(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	values := toStringSlice(shadowQUICInboundFirst(source, sourceKeys...))
	if len(values) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) > 0 {
		target[targetKey] = result
	}
}

func shadowQUICInboundCopyBool(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	if value, ok := toBool(shadowQUICInboundFirst(source, sourceKeys...)); ok {
		target[targetKey] = value
	}
}

func shadowQUICInboundCopyNonNegativeInt(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	if value, ok := toInt(shadowQUICInboundFirst(source, sourceKeys...)); ok && value >= 0 {
		target[targetKey] = value
	}
}
