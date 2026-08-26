package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util"

	"gorm.io/gorm"
)

// ========================================
// ProManager - 配置监视器和生成器
// ========================================
// 功能:
// 1. 监听入站、出站、用户管理、TLS、mux等配置变化
// 2. 当配置变化时，自动组合生成最终的 sing-box Core 配置
// 3. 仅将 core/singbox/config.json 写入 Runtime Store
// ========================================

// ConfigEventType 配置事件类型
type ConfigEventType string

const (
	EventCreate ConfigEventType = "create"
	EventUpdate ConfigEventType = "update"
	EventDelete ConfigEventType = "delete"
)

// ConfigEventSource 配置事件来源
type ConfigEventSource string

const (
	SourceInbound  ConfigEventSource = "inbound"
	SourceOutbound ConfigEventSource = "outbound"
	SourceClient   ConfigEventSource = "client"
	SourceTls      ConfigEventSource = "tls"
	SourceDns      ConfigEventSource = "dns"
	SourceRoute    ConfigEventSource = "route"
	SourceRuleSet  ConfigEventSource = "ruleset"
	SourceService  ConfigEventSource = "service"
	SourceEndpoint ConfigEventSource = "endpoint"
	SourceConfig   ConfigEventSource = "config"
)

// ConfigEvent 配置变更事件
type ConfigEvent struct {
	Source    ConfigEventSource `json:"source"`
	EventType ConfigEventType   `json:"event_type"`
	Timestamp int64             `json:"timestamp"`
	Tag       string            `json:"tag,omitempty"`
	Id        uint              `json:"id,omitempty"`
	Data      json.RawMessage   `json:"data,omitempty"`
}

// ProManagerSingBoxConfig 完整的 sing-box 配置结构
type ProManagerSingBoxConfig struct {
	Certificate json.RawMessage   `json:"certificate,omitempty"`
	Log         json.RawMessage   `json:"log,omitempty"`
	Dns         json.RawMessage   `json:"dns,omitempty"`
	Ntp         json.RawMessage   `json:"ntp,omitempty"`
	Inbounds    []json.RawMessage `json:"inbounds,omitempty"`
	Outbounds   []json.RawMessage `json:"outbounds,omitempty"`
	Services    []json.RawMessage `json:"services,omitempty"`
	Endpoints   []json.RawMessage `json:"endpoints,omitempty"`
	Route       json.RawMessage   `json:"route,omitempty"`
	// Legacy top-level field kept only for backward-compatible decode.
	// GenerateFullConfig always normalizes this into route.rule_set.
	RuleSets     []json.RawMessage `json:"rule_set,omitempty"`
	Experimental json.RawMessage   `json:"experimental,omitempty"`
}

// SingleInboundConfig is retained for source compatibility.
// Deprecated: ProManager no longer persists per-inbound copies.
type SingleInboundConfig struct {
	Inbound  json.RawMessage   `json:"inbound"`
	Users    []json.RawMessage `json:"users,omitempty"`
	Tls      json.RawMessage   `json:"tls,omitempty"`
	Metadata *InboundMetadata  `json:"metadata"`
}

// InboundMetadata is retained for source compatibility.
// Deprecated: ProManager no longer persists inbound metadata sidecars.
type InboundMetadata struct {
	Id        uint   `json:"id"`
	Tag       string `json:"tag"`
	Type      string `json:"type"`
	TlsId     uint   `json:"tls_id,omitempty"`
	UserCount int    `json:"user_count"`
	UpdatedAt int64  `json:"updated_at"`
}

// SingleOutboundConfig is retained for source compatibility.
// Deprecated: ProManager no longer persists per-outbound copies.
type SingleOutboundConfig struct {
	Outbound json.RawMessage   `json:"outbound"`
	Metadata *OutboundMetadata `json:"metadata"`
}

// OutboundMetadata is retained for source compatibility.
// Deprecated: ProManager no longer persists outbound metadata sidecars.
type OutboundMetadata struct {
	Id        uint   `json:"id"`
	Tag       string `json:"tag"`
	Type      string `json:"type"`
	UpdatedAt int64  `json:"updated_at"`
}

// ProManagerService 配置管理服务
type ProManagerService struct {
	*ConfigService
	eventChan   chan ConfigEvent
	stopChan    chan struct{}
	wg          sync.WaitGroup
	mu          sync.RWMutex
	jsonService JsonServiceInterface
	initialized bool
}

var (
	proManagerInstance *ProManagerService
	proManagerOnce     sync.Once
	// Serializes default-chain runtime snapshots. SQLite serializes individual
	// queries, not a multi-query config build, so this lock prevents concurrent
	// generators from publishing stale or mixed snapshots.
	singboxConfigGenerationMu sync.Mutex
)

// GetProManagerService 获取ProManager单例
func GetProManagerService(configService *ConfigService) *ProManagerService {
	proManagerOnce.Do(func() {
		proManagerInstance = &ProManagerService{
			ConfigService: configService,
			eventChan:     make(chan ConfigEvent, 100),
			stopChan:      make(chan struct{}),
		}
		proManagerInstance.init()
	})
	return proManagerInstance
}

// NewProManagerService 创建ProManager服务 (兼容旧接口)
func NewProManagerService(configService *ConfigService) *ProManagerService {
	return GetProManagerService(configService)
}

// init 初始化ProManager
func (s *ProManagerService) init() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.initialized {
		return
	}

	// Ensure core runtime directory layout is ready before any config generation.
	if err := EnsureManagedCoreLayout(); err != nil {
		logger.Errorf("[ProManager] 初始化 core 目录结构失败: %v", err)
		return
	}

	// 启动事件处理协程
	s.wg.Add(1)
	go s.eventProcessor()

	s.initialized = true
	logger.Info("[ProManager] 配置监视器已初始化")
}

// Stop 停止ProManager
func (s *ProManagerService) Stop() {
	close(s.stopChan)
	s.wg.Wait()
	logger.Info("[ProManager] 配置监视器已停止")
}

// eventProcessor 事件处理器
func (s *ProManagerService) eventProcessor() {
	defer s.wg.Done()

	// 批量处理定时器
	batchTimer := time.NewTimer(500 * time.Millisecond)
	defer batchTimer.Stop()

	pendingEvents := make(map[ConfigEventSource][]ConfigEvent)

	for {
		select {
		case <-s.stopChan:
			// 处理剩余事件
			s.processBatchEvents(pendingEvents)
			return

		case event := <-s.eventChan:
			pendingEvents[event.Source] = append(pendingEvents[event.Source], event)
			batchTimer.Reset(500 * time.Millisecond)

		case <-batchTimer.C:
			if len(pendingEvents) > 0 {
				s.processBatchEvents(pendingEvents)
				pendingEvents = make(map[ConfigEventSource][]ConfigEvent)
			}
		}
	}
}

// processBatchEvents 批量处理事件
func (s *ProManagerService) processBatchEvents(events map[ConfigEventSource][]ConfigEvent) {
	needUpdateCore := false

	for source, eventList := range events {
		for _, event := range eventList {
			logger.Debugf("[ProManager] 处理事件: source=%s, type=%s, tag=%s",
				event.Source, event.EventType, event.Tag)
		}

		switch source {
		case SourceInbound, SourceClient, SourceTls, SourceOutbound,
			SourceDns, SourceRoute, SourceRuleSet, SourceConfig,
			SourceService, SourceEndpoint:
			needUpdateCore = true
		}
	}

	if needUpdateCore {
		s.regenerateCoreConfig()
	}
}

// EmitEvent 发送配置变更事件
func (s *ProManagerService) EmitEvent(source ConfigEventSource, eventType ConfigEventType, tag string, id uint, data json.RawMessage) {
	if !s.initialized {
		return
	}

	event := ConfigEvent{
		Source:    source,
		EventType: eventType,
		Timestamp: time.Now().Unix(),
		Tag:       tag,
		Id:        id,
		Data:      data,
	}

	select {
	case s.eventChan <- event:
	default:
		logger.Warning("[ProManager] 事件队列已满，丢弃事件")
	}
}

// ========================================
// 核心完整配置生成
// ========================================

// regenerateCoreConfig 重新生成核心完整配置
// 仅更新 Runtime Store 中的 core/singbox/config.json。
func (s *ProManagerService) regenerateCoreConfig() {
	if err := s.RegenerateCoreConfig(); err != nil {
		logger.Errorf("[ProManager] 重新生成核心配置失败: %v", err)
	}
}

// RegenerateCoreConfig rebuilds the final sing-box config and returns the
// actual generation or Runtime Store write error to callers that must verify
// certificate propagation before scheduling a running Core restart.
func (s *ProManagerService) RegenerateCoreConfig() error {
	singboxConfigGenerationMu.Lock()
	defer singboxConfigGenerationMu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return fmt.Errorf("初始化 core 目录结构失败: %w", err)
	}

	config, err := s.GenerateFullConfig()
	if err != nil {
		return fmt.Errorf("生成完整配置失败: %w", err)
	}

	// 保存完整配置
	configJson, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	// 保存到 core/singbox/config.json
	filePath := GetSingboxConfigPath()
	if err := ManagedRuntimeWriteFile(filePath, configJson); err != nil {
		return fmt.Errorf("写入核心配置失败: %w", err)
	}

	logger.Infof("[ProManager] 已更新核心配置: %s", filePath)
	return nil
}

// GenerateFullConfig 聚合所有信息并生成完整的 sing-box 配置
func (s *ProManagerService) GenerateFullConfig() (*ProManagerSingBoxConfig, error) {
	return s.GenerateFullConfigWithDB(database.GetDB())
}

// GenerateFullConfigWithDB builds a default-chain configuration from one
// caller-owned database handle. It is used to validate subscription imports
// before their short SQLite transaction commits.
func (s *ProManagerService) GenerateFullConfigWithDB(db *gorm.DB) (*ProManagerSingBoxConfig, error) {
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	// 获取基础配置 (DNS, Route, Log 等)
	baseData, err := s.SettingService.GetConfigWithDB(db)
	if err != nil {
		return nil, fmt.Errorf("获取基础配置失败: %w", err)
	}

	config := &ProManagerSingBoxConfig{}
	if err := json.Unmarshal([]byte(baseData), config); err != nil {
		return nil, fmt.Errorf("解析基础配置失败: %w", err)
	}

	// Normalize rule_set placement for sing-box:
	// keep only route.rule_set, never emit top-level rule_set.
	if err := normalizeRouteRuleSetPlacement(config); err != nil {
		return nil, fmt.Errorf("规范化路由规则集失败: %w", err)
	}
	coreLog, err := buildCurrentSingboxCoreLogConfig(db)
	if err != nil {
		return nil, fmt.Errorf("获取内核日志级别失败: %w", err)
	}
	config.Log = coreLog
	if err := normalizeSingboxDNSConfig(config); err != nil {
		return nil, fmt.Errorf("规范化 DNS 配置失败: %w", err)
	}
	if err := (&DnsServerService{}).ApplySelectedServerToCoreConfig(db, config); err != nil {
		return nil, fmt.Errorf("应用选中的 DNS 服务器失败: %w", err)
	}

	// 聚合所有数据库对象
	config.Inbounds, err = s.InboundService.GetAllConfig(db)
	if err != nil {
		return nil, fmt.Errorf("获取入站配置失败: %w", err)
	}
	if err := validateSingboxRuntimeTaggedObjects("inbound", config.Inbounds); err != nil {
		return nil, fmt.Errorf("validate sing-box runtime inbounds failed: %w", err)
	}

	config.Outbounds, err = s.OutboundService.GetAllConfig(db)
	outboundTLSStore := ""
	if err != nil {
		return nil, fmt.Errorf("获取出站配置失败: %w", err)
	}

	config.Outbounds, outboundTLSStore, err = normalizeSingboxRuntimeOutbounds(config.Outbounds)
	if err != nil {
		return nil, fmt.Errorf("sanitize sing-box outbounds failed: %w", err)
	}
	if err := validateSingboxRuntimeTaggedObjects("outbound", config.Outbounds); err != nil {
		return nil, fmt.Errorf("validate sing-box runtime outbounds failed: %w", err)
	}

	config.Services, err = s.ServicesService.GetAllConfig(db)
	if err != nil {
		return nil, fmt.Errorf("获取服务配置失败: %w", err)
	}

	config.Endpoints, err = s.EndpointService.GetAllConfig(db)
	if err != nil {
		return nil, fmt.Errorf("获取端点配置失败: %w", err)
	}

	if err := s.applyServerTLSStore(db, config, outboundTLSStore); err != nil {
		return nil, fmt.Errorf("apply server tls_store failed: %w", err)
	}

	return config, nil
}

func normalizeRouteRuleSetPlacement(config *ProManagerSingBoxConfig) error {
	if config == nil {
		return nil
	}

	routeMap := map[string]interface{}{}
	if len(config.Route) > 0 {
		if err := json.Unmarshal(config.Route, &routeMap); err != nil {
			return err
		}
	}

	// Backward compatibility:
	// if legacy top-level rule_set exists, move it into route.rule_set only when route doesn't already define it.
	if len(config.RuleSets) > 0 {
		if _, exists := routeMap["rule_set"]; !exists {
			ruleSetItems := make([]interface{}, 0, len(config.RuleSets))
			for _, item := range config.RuleSets {
				var decoded interface{}
				if err := json.Unmarshal(item, &decoded); err != nil {
					return err
				}
				ruleSetItems = append(ruleSetItems, decoded)
			}
			routeMap["rule_set"] = ruleSetItems
		}
	}

	// sing-box's current remote rule-set schema stores the download route in
	// http_client.detour. Migrate the legacy download_detour field while the
	// configuration is being assembled so generated Core configs are always
	// emitted in the current format.
	if err := normalizeSingboxRouteRuleSetHTTPClients(routeMap); err != nil {
		return err
	}

	// Always clear top-level rule_set in output.
	config.RuleSets = nil

	if len(routeMap) > 0 {
		routeData, err := json.Marshal(routeMap)
		if err != nil {
			return err
		}
		config.Route = routeData
	}

	return nil
}

func normalizeSingboxRouteRuleSetHTTPClients(routeMap map[string]interface{}) error {
	if routeMap == nil {
		return nil
	}
	raw, exists := routeMap["rule_set"]
	if !exists || raw == nil {
		return nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	for index, item := range items {
		ruleSet, ok := item.(map[string]interface{})
		if !ok || ruleSet == nil {
			continue
		}
		typeValue, _ := ruleSet["type"].(string)
		if !strings.EqualFold(strings.TrimSpace(typeValue), "remote") {
			continue
		}

		legacyDetour := ""
		if rawDetour, present := ruleSet["download_detour"]; present && rawDetour != nil {
			value, ok := rawDetour.(string)
			if !ok {
				return fmt.Errorf("route.rule_set #%d has an invalid download_detour", index+1)
			}
			legacyDetour = strings.TrimSpace(value)
		}

		if rawClient, present := ruleSet["http_client"]; present && rawClient != nil {
			client, ok := rawClient.(map[string]interface{})
			if !ok {
				return fmt.Errorf("route.rule_set #%d has an invalid http_client", index+1)
			}
			if rawDetour, present := client["detour"]; present && rawDetour != nil {
				value, ok := rawDetour.(string)
				if !ok {
					return fmt.Errorf("route.rule_set #%d has an invalid http_client.detour", index+1)
				}
				value = strings.TrimSpace(value)
				if value == "" {
					delete(client, "detour")
				} else {
					client["detour"] = value
				}
			}
			if _, present := client["detour"]; !present && legacyDetour != "" {
				client["detour"] = legacyDetour
			}
			if len(client) == 0 {
				delete(ruleSet, "http_client")
			} else {
				ruleSet["http_client"] = client
			}
		} else if legacyDetour != "" {
			ruleSet["http_client"] = map[string]interface{}{"detour": legacyDetour}
		}
		delete(ruleSet, "download_detour")
	}
	return nil
}

func normalizeSingboxDNSConfig(config *ProManagerSingBoxConfig) error {
	if config == nil || len(config.Dns) == 0 {
		return nil
	}

	dnsMap := map[string]interface{}{}
	if err := json.Unmarshal(config.Dns, &dnsMap); err != nil {
		return err
	}

	changed, err := sanitizeSingboxDNSMap(dnsMap, true)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	dnsData, err := json.Marshal(dnsMap)
	if err != nil {
		return err
	}
	config.Dns = dnsData
	return nil
}

func (s *ProManagerService) applyServerTLSStore(db *gorm.DB, config *ProManagerSingBoxConfig, fallbackStore string) error {
	enabled, err := s.SettingService.GetServerTLSStoreEnabledWithDB(db)
	if err != nil {
		return err
	}

	certificate := map[string]interface{}{}
	if len(config.Certificate) > 0 {
		var raw interface{}
		if err := json.Unmarshal(config.Certificate, &raw); err == nil {
			if certMap, ok := raw.(map[string]interface{}); ok && certMap != nil {
				for k, v := range certMap {
					certificate[k] = v
				}
			}
		}
	}

	if enabled {
		store, err := s.SettingService.GetServerTLSStoreWithDB(db)
		if err != nil {
			return err
		}
		certificate["store"] = store
	} else {
		fallbackStore = normalizeCertificateStoreValue(fallbackStore)
		if fallbackStore != "" {
			certificate["store"] = fallbackStore
		} else {
			delete(certificate, "store")
		}
	}

	if len(certificate) == 0 {
		config.Certificate = nil
		return nil
	}

	raw, err := json.Marshal(certificate)
	if err != nil {
		return err
	}
	config.Certificate = raw
	return nil
}

// SaveInboundJson 保留旧入口兼容性；实际只生成最终 sing-box Core 配置。
func (s *ProManagerService) SaveInboundJson() {
	s.regenerateCoreConfig()
}

// ========================================
// JSON订阅服务兼容接口
// ========================================

// JsonServiceInterface 定义JsonService需要实现的接口
type JsonServiceInterface interface {
	GetJson(subId string, format string) (*string, []string, error)
}

func (s *ProManagerService) SetJsonService(jsonService JsonServiceInterface) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jsonService = jsonService
}

func (s *ProManagerService) getJsonService() JsonServiceInterface {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jsonService
}

// getClientJsonSubscription 获取客户端的JSON订阅内容
// 复用现有的JsonService逻辑，保持与系统的一致性
func (s *ProManagerService) getClientJsonSubscription(clientName string) (*string, error) {
	if jsonService := s.getJsonService(); jsonService != nil {
		result, _, err := jsonService.GetJson(clientName, "json")
		if err != nil {
			return nil, fmt.Errorf("获取JSON订阅失败: %w", err)
		}
		return result, nil
	}

	db := database.GetDB()

	// 验证客户端存在且启用
	client := &model.Client{}
	err := db.Model(model.Client{}).Where("enable = true and name = ?", clientName).First(client).Error
	if err != nil {
		return nil, fmt.Errorf("客户端不存在或未启用: %w", err)
	}

	// 获取客户端关联的入站
	var clientInbounds []uint
	if err := json.Unmarshal(client.Inbounds, &clientInbounds); err != nil {
		return nil, fmt.Errorf("解析入站列表失败: %w", err)
	}

	if len(clientInbounds) == 0 {
		return nil, nil
	}

	// 获取入站信息
	var inbounds []*model.Inbound
	err = db.Model(model.Inbound{}).Preload("Tls").Where("id in ?", clientInbounds).Find(&inbounds).Error
	if err != nil {
		return nil, fmt.Errorf("获取入站失败: %w", err)
	}
	inbounds = util.OrderBaseInboundPtrsByIDs(clientInbounds, inbounds)

	// 构建出站配置
	outbounds, outTags, err := s.buildClientOutbounds(client, inbounds)
	if err != nil {
		return nil, fmt.Errorf("构建出站失败: %w", err)
	}

	if len(*outbounds) == 0 {
		return nil, nil
	}

	// 添加默认出站
	s.addDefaultOutbounds(outbounds, outTags)

	// 构建完整的JSON配置
	jsonConfig := s.buildJsonConfig(outbounds)

	// 从 TLS 配置中提取证书库设置，注入到每个出站 TLS 块（不再添加顶级 certificate 对象）
	tlsStore := normalizeCertificateStoreValue(s.extractTlsStoreFromInbounds(inbounds))
	if tlsStore != "" {
		// 将 store 注入到每个出站的 TLS 块中
		certificate := map[string]interface{}{}
		if existing, ok := jsonConfig["certificate"].(map[string]interface{}); ok && existing != nil {
			for k, v := range existing {
				certificate[k] = v
			}
		}
		certificate["store"] = tlsStore
		jsonConfig["certificate"] = certificate
	}

	result, err := json.MarshalIndent(jsonConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("JSON序列化失败: %w", err)
	}

	resultStr := string(result)
	return &resultStr, nil
}

// injectTlsStoreToProOutbounds 将 store 值注入到每个出站的 TLS 块中，并移除 tls_store
func injectTlsStoreToProOutbounds(outbounds *[]map[string]interface{}, tlsStore string) {
	for i := range *outbounds {
		outbound := &(*outbounds)[i]
		tlsRaw, ok := (*outbound)["tls"]
		if !ok {
			continue
		}
		tlsMap, ok := tlsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		// 移除旧的 tls_store 字段（如果存在）
		delete(tlsMap, "tls_store")
		// 注入 store 字段
		tlsMap["store"] = tlsStore
	}
}

// extractTlsStoreFromInbounds 从入站关联的 TLS 配置中提取 tls_store 值
// 返回第一个找到的非空 tls_store 值
func (s *ProManagerService) extractTlsStoreFromInbounds(inbounds []*model.Inbound) string {
	for _, inData := range inbounds {
		if inData.TlsId > 0 && inData.Tls != nil && len(inData.Tls.Client) > 0 {
			var tlsClient map[string]interface{}
			if err := json.Unmarshal(inData.Tls.Client, &tlsClient); err == nil {
				if store, ok := tlsClient["tls_store"].(string); ok && store != "" {
					return store
				}
			}
		}
	}
	return ""
}

// buildClientOutbounds 构建客户端的出站配置列表
func (s *ProManagerService) buildClientOutbounds(client *model.Client, inbounds []*model.Inbound) (*[]map[string]interface{}, *[]string, error) {
	var outbounds []map[string]interface{}
	var outTags []string

	var configs map[string]interface{}
	if err := json.Unmarshal(client.Config, &configs); err != nil {
		return nil, nil, fmt.Errorf("解析客户端配置失败: %w", err)
	}

	for _, inbound := range inbounds {
		if inbound == nil || util.IsSubscriptionServerOnlyInboundType(inbound.Type) {
			continue
		}
		if len(inbound.OutJson) < 5 {
			continue
		}

		var outbound map[string]interface{}
		if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
			continue
		}
		util.StripSubscriptionOutboundPanelFields(outbound)

		protocol, _ := outbound["type"].(string)
		if util.IsSubscriptionServerOnlyInboundType(protocol) {
			continue
		}

		// ShadowTLS: 生成 shadowsocks + shadowtls 两个出站
		if protocol == "shadowtls" {
			ssOutbound, stlsOutbound := s.buildShadowTLSClientOutbounds(outbound, configs, inbound)
			if ssOutbound != nil && stlsOutbound != nil {
				stripClashOnlyTLSFields(ssOutbound)
				stripClashOnlyTLSFields(stlsOutbound)
				tag, _ := ssOutbound["tag"].(string)
				outTags = append(outTags, tag)
				outbounds = append(outbounds, ssOutbound)
				outbounds = append(outbounds, stlsOutbound)
			}
			continue
		}

		// 应用用户配置
		if protocol == "shadowsocks" {
			s.applyShadowsocksConfigSimple(&outbound, configs, inbound)
		} else {
			config, _ := configs[protocol].(map[string]interface{})
			for key, value := range config {
				if util.ShouldSkipSingboxOutboundClientConfigKey(protocol, key, inbound.TlsId != 0) {
					continue
				}
				outbound[key] = value
			}
			if !util.ShouldSkipSingboxOutboundClientConfigKey(protocol, "username", inbound.TlsId != 0) &&
				strings.TrimSpace(firstString(outbound["username"])) == "" {
				if username := util.SubscriptionClientConfigUsername(config); username != "" {
					outbound["username"] = username
				}
			}
		}
		if protocol == "hysteria" {
			util.ApplyHysteriaInboundQUICToOutbound(outbound, inbound.Options)
		}

		stripClashOnlyTLSFields(outbound)
		tag, _ := outbound["tag"].(string)
		outTags = append(outTags, tag)
		outbounds = append(outbounds, outbound)
	}
	outbounds, outTags = util.FilterTaggedSubscriptionOutbounds(
		outbounds,
		outTags,
		util.SupportsSingboxSubscriptionOutboundType,
	)
	for i := range outbounds {
		util.SanitizeSingboxSubscriptionOutbound(outbounds[i])
	}

	return &outbounds, &outTags, nil
}

// buildShadowTLSClientOutbounds 构建 ShadowTLS 的客户端出站配置
// 返回: shadowsocks 出站, shadowtls 出站
func (s *ProManagerService) buildShadowTLSClientOutbounds(outJson map[string]interface{}, configs map[string]interface{}, inbound *model.Inbound) (map[string]interface{}, map[string]interface{}) {
	if inbound == nil {
		return util.BuildShadowTLSClientPair(outJson, configs, nil)
	}
	return util.BuildShadowTLSClientPair(outJson, configs, inbound.Options)
}

func (s *ProManagerService) buildShadowTLSClientOutboundsLegacy(outJson map[string]interface{}, configs map[string]interface{}) (map[string]interface{}, map[string]interface{}) {
	tag, _ := outJson["tag"].(string)
	if tag == "" {
		return nil, nil
	}

	// 获取 ss_config
	ssConfig, hasSsConfig := outJson["ss_config"].(map[string]interface{})

	// 获取用户的 shadowtls 配置（password）
	stlsConfig, _ := configs["shadowtls"].(map[string]interface{})
	stlsPassword, _ := stlsConfig["password"].(string)

	// 构建 shadowtls 出站
	stlsTag := tag + "-out"
	stlsOutbound := map[string]interface{}{
		"type":        "shadowtls",
		"tag":         stlsTag,
		"server":      outJson["server"],
		"server_port": outJson["server_port"],
		"version":     outJson["version"],
		"password":    stlsPassword,
	}

	// 复制 TLS 配置
	if tls, ok := outJson["tls"]; ok {
		stlsOutbound["tls"] = tls
	}

	if !hasSsConfig || ssConfig == nil {
		return nil, stlsOutbound
	}

	// 构建 shadowsocks 出站
	ssOutbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    tag,
		"detour": stlsTag,
	}

	if method, ok := ssConfig["method"]; ok {
		ssOutbound["method"] = method
	}
	if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
		ssOutbound["network"] = network
	}
	if password, ok := ssConfig["password"]; ok {
		ssOutbound["password"] = password
	}
	if udpOverTcp, ok := ssConfig["udp_over_tcp"]; ok {
		ssOutbound["udp_over_tcp"] = udpOverTcp
	}
	if multiplex, ok := ssConfig["multiplex"]; ok {
		ssOutbound["multiplex"] = multiplex
	}

	return ssOutbound, stlsOutbound
}

// stripClashOnlyTLSFields removes Clash/Mihomo-only TLS fields from sing-box JSON outbounds.
func stripClashOnlyTLSFields(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}
	util.StripMihomoHysteria2ReceiveWindowsForSingbox(outbound)
	delete(outbound, "mihomo_common")
	delete(outbound, "mihomo_hy2")
	delete(outbound, "mihomo_fast_open")
	delete(outbound, "fast_open")
	tlsRaw, ok := outbound["tls"]
	if !ok {
		return
	}
	tlsMap, ok := tlsRaw.(map[string]interface{})
	if !ok || tlsMap == nil {
		return
	}
	delete(tlsMap, "tls_store")
	delete(tlsMap, "store")
	delete(tlsMap, "fingerprint")
	delete(tlsMap, "mihomo_use_fingerprint")
	delete(tlsMap, "include_server_certificate")
	delete(tlsMap, "include_server_fingerprint")
}

// applyShadowsocksConfigSimple 应用Shadowsocks配置（简化版）
func (s *ProManagerService) applyShadowsocksConfigSimple(outbound *map[string]interface{}, configs map[string]interface{}, inbound *model.Inbound) {
	var inbOptions map[string]interface{}
	if err := json.Unmarshal(inbound.Options, &inbOptions); err != nil {
		return
	}

	if inbPass, ok := inbOptions["password"].(string); ok && inbPass != "" {
		(*outbound)["password"] = inbPass
	}
}

// addDefaultOutbounds 添加默认出站
func (s *ProManagerService) addDefaultOutbounds(outbounds *[]map[string]interface{}, outTags *[]string) {
	selectorGroups := []selectorGroupConfig{}
	if othersStr, err := s.SettingService.GetSubJsonExt(); err == nil && len(othersStr) > 0 {
		var extJson map[string]interface{}
		if unmarshalErr := json.Unmarshal([]byte(othersStr), &extJson); unmarshalErr == nil {
			selectorGroups = parseSelectorGroupsFromExt(extJson)
		}
	}
	customSelectors := buildNamedSelectorOutbounds(selectorGroups, *outTags)

	defaultOutbounds := []map[string]interface{}{
		{
			"type":                        "selector",
			"tag":                         nodeSelectorTag,
			"outbounds":                   append([]string{autoSelectorTag}, *outTags...),
			"interrupt_exist_connections": true,
		},
		{
			"type":                        "urltest",
			"tag":                         autoSelectorTag,
			"outbounds":                   *outTags,
			"url":                         "http://www.gstatic.com/generate_204",
			"interval":                    "10m",
			"tolerance":                   50,
			"interrupt_exist_connections": true,
		},
		{
			"type":                        "selector",
			"tag":                         globalDirectSelectorTag,
			"outbounds":                   append([]string{"direct", "block"}, *outTags...),
			"interrupt_exist_connections": true,
		},
		{
			"type":                        "selector",
			"tag":                         globalBlockSelectorTag,
			"outbounds":                   append([]string{"block", "direct"}, *outTags...),
			"interrupt_exist_connections": true,
		},
		{
			"type":                        "selector",
			"tag":                         finalSelectorTag,
			"outbounds":                   append([]string{nodeSelectorTag, globalDirectSelectorTag}, *outTags...),
			"interrupt_exist_connections": true,
		},
		{
			"type":                        "selector",
			"tag":                         globalSelectorTag,
			"outbounds":                   append([]string{nodeSelectorTag, autoSelectorTag, globalDirectSelectorTag, globalBlockSelectorTag, finalSelectorTag}, *outTags...),
			"interrupt_exist_connections": true,
		},
	}
	defaultOutbounds = append(defaultOutbounds, customSelectors...)
	defaultOutbounds = append(defaultOutbounds,
		map[string]interface{}{"type": "direct", "tag": "direct"},
		map[string]interface{}{"type": "block", "tag": "block"},
	)
	*outbounds = append(defaultOutbounds, *outbounds...)
}

// buildJsonConfig 构建完整的JSON配置
func (s *ProManagerService) buildJsonConfig(outbounds *[]map[string]interface{}) map[string]interface{} {
	jsonConfig := map[string]interface{}{
		"inbounds": []map[string]interface{}{
			{
				"type":                     "tun",
				"address":                  []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"mtu":                      9000,
				"auto_route":               true,
				"strict_route":             false,
				"endpoint_independent_nat": false,
				"stack":                    "system",
				"platform": map[string]interface{}{
					"http_proxy": map[string]interface{}{
						"enabled":     true,
						"server":      "127.0.0.1",
						"server_port": 2080,
					},
				},
			},
			{
				"type":        "mixed",
				"listen":      "127.0.0.1",
				"listen_port": 2080,
				"users":       []interface{}{},
			},
		},
		"outbounds": outbounds,
		"route": map[string]interface{}{
			"auto_detect_interface": true,
			"final":                 finalSelectorTag,
			"rules": []interface{}{
				map[string]interface{}{"action": "sniff"},
				map[string]interface{}{"clash_mode": "direct", "action": "route", "outbound": globalDirectSelectorTag},
				map[string]interface{}{"clash_mode": "global", "action": "route", "outbound": globalSelectorTag},
			},
		},
	}

	// 添加扩展配置
	s.applyJsonExtras(&jsonConfig)

	return jsonConfig
}

// applyJsonExtras 应用扩展配置
func (s *ProManagerService) applyJsonExtras(jsonConfig *map[string]interface{}) {
	othersStr, err := s.SettingService.GetSubJsonExt()
	if err != nil || len(othersStr) == 0 {
		return
	}

	var othersJson map[string]interface{}
	if err := json.Unmarshal([]byte(othersStr), &othersJson); err != nil {
		return
	}

	if log, ok := othersJson["log"]; ok {
		(*jsonConfig)["log"] = log
	}
	if dns, ok := othersJson["dns"]; ok {
		(*jsonConfig)["dns"] = normalizeSubDnsDetours(removeDeprecatedDnsClashModeRules(dns))
	}
	if inbounds, ok := othersJson["inbounds"]; ok {
		(*jsonConfig)["inbounds"] = inbounds
	}
	if experimental, ok := othersJson["experimental"]; ok {
		(*jsonConfig)["experimental"] = experimental
	}
	if httpClients, ok := buildSubHTTPClients(othersJson); ok {
		(*jsonConfig)["http_clients"] = httpClients
	}

	// 清理 _uiConfig（仅前端 UI 状态，不应出现在最终配置中）
	delete(othersJson, "_uiConfig")

	if route, ok := (*jsonConfig)["route"].(map[string]interface{}); ok {
		if _, ok := (*jsonConfig)["http_clients"]; ok {
			route["default_http_client"] = managedSubHTTPClientTag
		}
		if ruleSet, ok := othersJson["rule_set"]; ok {
			route["rule_set"] = normalizeSubRuleSetDownloadDetours(ruleSet)
		}
		if settingRules, ok := othersJson["rules"].([]interface{}); ok {
			rulesStart := []interface{}{
				map[string]interface{}{"action": "sniff"},
				map[string]interface{}{"clash_mode": "direct", "action": "route", "outbound": globalDirectSelectorTag},
			}
			rulesEnd := []interface{}{
				map[string]interface{}{"clash_mode": "global", "action": "route", "outbound": globalSelectorTag},
			}
			rules := append(rulesStart, normalizeSubRouteRules(settingRules)...)
			route["rules"] = append(rules, rulesEnd...)
		}
		if routeFinal, ok := othersJson["route_final"].(string); ok {
			route["final"] = normalizeRouteFinalOutbound(routeFinal)
		}
		if resolver, ok := othersJson["default_domain_resolver"].(string); ok {
			route["default_domain_resolver"] = resolver
		}
	}
}

// ========================================
// 辅助函数
// ========================================

// sanitizeFilename 清理文件名，移除不安全字符
func sanitizeFilename(name string) string {
	// 替换不安全的文件名字符
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range unsafe {
		result = replaceAll(result, char, "_")
	}
	return result
}

func replaceAll(s, old, new string) string {
	for {
		idx := indexOf(s, old)
		if idx == -1 {
			return s
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ========================================
// 便捷的事件触发方法
// ========================================

// OnInboundChange 入站变更时调用
func (s *ProManagerService) OnInboundChange(eventType ConfigEventType, tag string, id uint) {
	s.EmitEvent(SourceInbound, eventType, tag, id, nil)
}

// OnOutboundChange 出站变更时调用
func (s *ProManagerService) OnOutboundChange(eventType ConfigEventType, tag string, id uint) {
	s.EmitEvent(SourceOutbound, eventType, tag, id, nil)
}

// OnClientChange 用户变更时调用
func (s *ProManagerService) OnClientChange(eventType ConfigEventType, name string, id uint) {
	s.EmitEvent(SourceClient, eventType, name, id, nil)
}

// OnTlsChange TLS变更时调用
func (s *ProManagerService) OnTlsChange(eventType ConfigEventType, name string, id uint) {
	s.EmitEvent(SourceTls, eventType, name, id, nil)
}

// OnConfigChange 核心配置变更时调用
func (s *ProManagerService) OnConfigChange() {
	s.EmitEvent(SourceConfig, EventUpdate, "", 0, nil)
}
