package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util"
)

// ========================================
// ProManager - 配置监视器和生成器
// ========================================
// 功能:
// 1. 监听入站、出站、用户管理、TLS、mux等配置变化
// 2. 当配置变化时，自动组合生成完整的配置文件
// 3. 存储结构:
//    - Promanager_data/inbound/  - 单入站JSON (每个入站一个文件)
//    - Promanager_data/outbound/ - 出站JSON (每个出站一个文件)
//    - Promanager_data/core/     - 完整版核心配置
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

// SingleInboundConfig 单入站完整配置 (入站+用户)
type SingleInboundConfig struct {
	Inbound  json.RawMessage   `json:"inbound"`
	Users    []json.RawMessage `json:"users,omitempty"`
	Tls      json.RawMessage   `json:"tls,omitempty"`
	Metadata *InboundMetadata  `json:"metadata"`
}

// InboundMetadata 入站元数据
type InboundMetadata struct {
	Id        uint   `json:"id"`
	Tag       string `json:"tag"`
	Type      string `json:"type"`
	TlsId     uint   `json:"tls_id,omitempty"`
	UserCount int    `json:"user_count"`
	UpdatedAt int64  `json:"updated_at"`
}

// SingleOutboundConfig 单出站配置
type SingleOutboundConfig struct {
	Outbound json.RawMessage   `json:"outbound"`
	Metadata *OutboundMetadata `json:"metadata"`
}

// OutboundMetadata 出站元数据
type OutboundMetadata struct {
	Id        uint   `json:"id"`
	Tag       string `json:"tag"`
	Type      string `json:"type"`
	UpdatedAt int64  `json:"updated_at"`
}

// ProManagerService 配置管理服务
type ProManagerService struct {
	*ConfigService
	baseDir     string
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

	// 获取执行文件目录
	_, err := os.Executable()
	if err != nil {
		logger.Errorf("[ProManager] 获取执行路径失败: %v", err)
		return
	}
	s.baseDir = config.GetDataDir()

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

	needUpdateSubJson := false

	for source, eventList := range events {
		for _, event := range eventList {
			logger.Debugf("[ProManager] 处理事件: source=%s, type=%s, tag=%s",
				event.Source, event.EventType, event.Tag)
		}

		switch source {
		case SourceInbound, SourceClient, SourceTls:
			s.regenerateInboundConfigs()
			needUpdateCore = true
			needUpdateSubJson = true

		case SourceOutbound:
			s.regenerateOutboundConfigs()
			needUpdateCore = true

		case SourceDns, SourceRoute, SourceRuleSet, SourceConfig:
			needUpdateCore = true
			needUpdateSubJson = true

		case SourceService, SourceEndpoint:
			needUpdateCore = true
		}
	}

	if needUpdateCore {
		s.regenerateCoreConfig()
	}

	if needUpdateSubJson {
		s.regenerateSubJsonConfigs()
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
// 入站配置生成
// ========================================

// regenerateInboundConfigs 重新生成所有入站配置
// 注意：入站配置直接保存到 Inbound 目录（大写，与现有文件系统一致）
func (s *ProManagerService) regenerateInboundConfigs() {
	db := database.GetDB()

	// 获取所有入站
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).Preload("Tls").Find(&inbounds).Error; err != nil {
		logger.Errorf("[ProManager] 获取入站列表失败: %v", err)
		return
	}

	// 使用大写 Inbound 目录以与现有文件系统保持一致
	inboundDir := filepath.Join(s.baseDir, "Inbound")

	// 清理旧文件（但保留 inbound.json 完整配置，它在 regenerateCoreConfig 中生成）
	if err := ManagedRuntimeClearDirJSONFiles(inboundDir, "inbound.json"); err != nil {
		logger.Warning("[ProManager] 清理 Inbound 目录失败: ", err)
	}

	// 为每个入站生成配置
	for _, inbound := range inbounds {
		if err := s.generateSingleInboundConfig(inbound); err != nil {
			logger.Errorf("[ProManager] 生成入站配置失败 [%s]: %v", inbound.Tag, err)
		}
	}

	logger.Infof("[ProManager] 已重新生成 %d 个入站配置", len(inbounds))
}

// generateSingleInboundConfig 生成单个入站的完整配置
// 保存两个文件：
// 1. {tag}.json - 纯净的 singbox inbound 配置
// 2. {tag}_meta.json - 元数据信息（id, tag, type, updated_at等）
// 对于 ShadowTLS 入站，会在同一文件中生成组合配置（连续两个 JSON 对象，无外层数组）
func (s *ProManagerService) generateSingleInboundConfig(inbound *model.Inbound) error {
	gormDB := database.GetDB()

	// 生成入站JSON
	inboundJson, err := inbound.MarshalJSON()
	if err != nil {
		return fmt.Errorf("序列化入站失败: %w", err)
	}

	// 获取关联的用户
	users, err := s.getInboundUsers(gormDB, inbound)
	if err != nil {
		return fmt.Errorf("获取用户失败: %w", err)
	}

	// 生成带用户的完整入站配置
	fullInboundConfig, err := s.buildFullInboundWithUsers(inboundJson, users, inbound)
	if err != nil {
		return fmt.Errorf("构建完整入站配置失败: %w", err)
	}

	baseFilename := sanitizeFilename(inbound.Tag)
	// 使用大写 Inbound 目录以与现有文件系统保持一致
	inboundDir := filepath.Join(s.baseDir, "Inbound")

	// 处理 ShadowTLS 组合入站 - 保存到同一个文件中（连续两个 JSON 对象，无外层数组）
	if inbound.Type == "shadowtls" {
		shadowtlsJson, ssJson, err := s.processShadowTLSInboundConfig(fullInboundConfig, inbound)
		if err != nil {
			return fmt.Errorf("处理ShadowTLS入站失败: %w", err)
		}

		if ssJson != nil {
			if err := s.saveShadowTLSCombinedInboundConfigFile(inboundDir, baseFilename, shadowtlsJson, ssJson); err != nil {
				return err
			}
		} else {
			if err := s.saveInboundConfigFile(inboundDir, baseFilename, shadowtlsJson); err != nil {
				return err
			}
		}
	} else {
		// 普通入站，直接保存
		if err := s.saveInboundConfigFile(inboundDir, baseFilename, fullInboundConfig); err != nil {
			return err
		}
	}

	// 保存元数据文件
	metaFilePath := filepath.Join(inboundDir, fmt.Sprintf("%s_meta.json", baseFilename))
	metadata := &InboundMetadata{
		Id:        inbound.Id,
		Tag:       inbound.Tag,
		Type:      inbound.Type,
		TlsId:     inbound.TlsId,
		UserCount: len(users),
		UpdatedAt: time.Now().Unix(),
	}
	metaData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("元数据序列化失败: %w", err)
	}
	if err := ManagedRuntimeWriteFile(metaFilePath, metaData); err != nil {
		return fmt.Errorf("写入元数据文件失败: %w", err)
	}

	return nil
}

// saveInboundConfigFile 保存入站配置到文件
func (s *ProManagerService) saveInboundConfigFile(dir, filename string, configJson json.RawMessage) error {
	configFilePath := filepath.Join(dir, fmt.Sprintf("%s.json", filename))
	var prettyJson interface{}
	if err := json.Unmarshal(configJson, &prettyJson); err != nil {
		return fmt.Errorf("解析JSON失败: %w", err)
	}
	configData, err := json.MarshalIndent(prettyJson, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON序列化失败: %w", err)
	}
	if err := ManagedRuntimeWriteFile(configFilePath, configData); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	return nil
}

// saveShadowTLSCombinedInboundConfigFile 保存 ShadowTLS 组合入站（同一文件连续两个 JSON 对象，无外层数组）
func (s *ProManagerService) saveShadowTLSCombinedInboundConfigFile(dir, filename string, shadowtlsJson, ssJson json.RawMessage) error {
	configFilePath := filepath.Join(dir, fmt.Sprintf("%s.json", filename))

	var shadowtlsPretty interface{}
	if err := json.Unmarshal(shadowtlsJson, &shadowtlsPretty); err != nil {
		return fmt.Errorf("解析ShadowTLS JSON失败: %w", err)
	}
	shadowtlsData, err := json.MarshalIndent(shadowtlsPretty, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化ShadowTLS JSON失败: %w", err)
	}

	var ssPretty interface{}
	if err := json.Unmarshal(ssJson, &ssPretty); err != nil {
		return fmt.Errorf("解析Shadowsocks JSON失败: %w", err)
	}
	ssData, err := json.MarshalIndent(ssPretty, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化Shadowsocks JSON失败: %w", err)
	}

	combinedData := append(shadowtlsData, '\n')
	combinedData = append(combinedData, ssData...)

	if err := ManagedRuntimeWriteFile(configFilePath, combinedData); err != nil {
		return fmt.Errorf("写入ShadowTLS组合配置文件失败: %w", err)
	}
	return nil
}

// processShadowTLSInboundConfig 处理 ShadowTLS 入站，生成组合的入站配置
// 与 InboundService.processShadowTLSInbound 保持一致
// 返回: shadowtlsJson, ssJson, error
func (s *ProManagerService) processShadowTLSInboundConfig(inboundJson []byte, inbound *model.Inbound) (json.RawMessage, json.RawMessage, error) {
	var inboundData map[string]interface{}
	if err := json.Unmarshal(inboundJson, &inboundData); err != nil {
		return nil, nil, err
	}

	// 检查是否有 ss_config
	ssConfig, hasSsConfig := inboundData["ss_config"].(map[string]interface{})
	if !hasSsConfig || ssConfig == nil {
		// 没有 ss_config，返回原始配置
		return inboundJson, nil, nil
	}

	// 删除 ss_config，不需要在最终的 shadowtls 配置中
	delete(inboundData, "ss_config")

	// 清理空的 handshake_for_server_name
	if hfsn, ok := inboundData["handshake_for_server_name"].(map[string]interface{}); ok && len(hfsn) == 0 {
		delete(inboundData, "handshake_for_server_name")
	}

	// 清理空的或 "off" 的 wildcard_sni
	if wSni, ok := inboundData["wildcard_sni"].(string); ok && (wSni == "" || wSni == "off") {
		delete(inboundData, "wildcard_sni")
	}

	// 安全获取 tag
	tag, ok := inboundData["tag"].(string)
	if !ok || tag == "" {
		// 没有 tag，返回原始配置（删除 ss_config 后）
		shadowtlsJson, err := json.Marshal(inboundData)
		if err != nil {
			return nil, nil, err
		}
		return shadowtlsJson, nil, nil
	}

	// 生成内部 shadowsocks 入站的 tag
	ssTag := tag + "-in"

	// 设置 shadowtls 的 detour 指向内部 shadowsocks
	inboundData["detour"] = ssTag

	// 生成 shadowtls 入站配置
	shadowtlsJson, err := json.Marshal(inboundData)
	if err != nil {
		return nil, nil, err
	}

	// 生成内部 shadowsocks 入站配置
	ssInbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    ssTag,
		"listen": "127.0.0.1",
	}

	// 按顺序添加字段: network, method, password, multiplex
	if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
		ssInbound["network"] = network
	}
	if method, ok := ssConfig["method"]; ok && method != nil {
		ssInbound["method"] = method
	}
	if password, ok := ssConfig["password"]; ok && password != nil {
		ssInbound["password"] = password
	}

	// 添加多路复用配置（仅保留服务端需要的字段：enabled, padding, brutal）
	if multiplex, ok := ssConfig["multiplex"].(map[string]interface{}); ok && multiplex != nil {
		serverMux := map[string]interface{}{}
		if enabled, ok := multiplex["enabled"]; ok {
			serverMux["enabled"] = enabled
		}
		if padding, ok := multiplex["padding"]; ok {
			serverMux["padding"] = padding
		}
		if brutal, ok := multiplex["brutal"]; ok {
			serverMux["brutal"] = brutal
		}
		ssInbound["multiplex"] = serverMux
	}

	ssInboundJson, err := json.Marshal(ssInbound)
	if err != nil {
		return nil, nil, err
	}

	return shadowtlsJson, ssInboundJson, nil
}

// getInboundUsers 获取入站关联的用户配置
func (s *ProManagerService) getInboundUsers(db interface{}, inbound *model.Inbound) ([]json.RawMessage, error) {
	gormDB := database.GetDB()

	// 检查此入站类型是否支持用户
	if !s.InboundService.hasUser(inbound.Type) {
		return nil, nil
	}

	// 特殊处理 shadowtls 版本小于3的情况
	if inbound.Type == "shadowtls" {
		var options map[string]interface{}
		if err := json.Unmarshal(inbound.Options, &options); err == nil {
			if version, ok := options["version"].(float64); ok && int(version) < 3 {
				return nil, nil
			}
		}
	}

	// 查询关联的客户端
	var users []string
	condition := fmt.Sprintf("%d IN (SELECT json_each.value FROM json_each(clients.inbounds))", inbound.Id)

	inboundType := inbound.Type
	if inbound.Type == "shadowsocks" {
		var options map[string]interface{}
		if err := json.Unmarshal(inbound.Options, &options); err == nil {
			if method, ok := options["method"].(string); ok && method == "2022-blake3-aes-128-gcm" {
				inboundType = "shadowsocks16"
			}
		}
	}

	err := gormDB.Raw(
		fmt.Sprintf(`SELECT json_extract(clients.config, "$.%s")
		FROM clients WHERE enable = true AND %s`,
			inboundType, condition)).Scan(&users).Error
	if err != nil {
		return nil, err
	}

	usersJSON, err := normalizeSingboxUsersForList(inboundType, users, inbound.TlsId != 0)
	if err != nil {
		return nil, err
	}

	return usersJSON, nil
}

// buildFullInboundWithUsers 构建带用户的完整入站配置
func (s *ProManagerService) buildFullInboundWithUsers(inboundJson []byte, users []json.RawMessage, inbound *model.Inbound) (json.RawMessage, error) {
	var inboundData map[string]interface{}
	if err := json.Unmarshal(inboundJson, &inboundData); err != nil {
		return nil, err
	}

	// 添加用户
	if len(users) > 0 {
		inboundData["users"] = users
	}

	return json.Marshal(inboundData)
}

// ========================================
// 出站配置生成
// ========================================

// regenerateOutboundConfigs 重新生成所有出站配置
func (s *ProManagerService) regenerateOutboundConfigs() {
	db := database.GetDB()

	// 获取所有出站
	var outbounds []*model.Outbound
	if err := db.Model(model.Outbound{}).Find(&outbounds).Error; err != nil {
		logger.Errorf("[ProManager] 获取出站列表失败: %v", err)
		return
	}

	outboundDir := filepath.Join(s.baseDir, "outbound")

	// 清理旧文件
	s.cleanDirectory(outboundDir)

	// 为每个出站生成配置
	for _, outbound := range outbounds {
		if err := s.generateSingleOutboundConfig(outbound); err != nil {
			logger.Errorf("[ProManager] 生成出站配置失败 [%s]: %v", outbound.Tag, err)
		}
	}

	logger.Infof("[ProManager] 已重新生成 %d 个出站配置", len(outbounds))
}

// generateSingleOutboundConfig 生成单个出站的配置
// 保存两个文件：
// 1. {tag}.json - 纯净的 singbox outbound 配置
// 2. {tag}_meta.json - 元数据信息（id, tag, type, updated_at等）
func (s *ProManagerService) generateSingleOutboundConfig(outbound *model.Outbound) error {
	// 生成出站JSON
	outboundJson, err := resolveOutboundJSON(outbound)
	if err != nil {
		return fmt.Errorf("序列化出站失败: %w", err)
	}

	baseFilename := sanitizeFilename(outbound.Tag)
	outboundDir := filepath.Join(s.baseDir, "outbound")

	// 1. 保存纯净的 singbox 配置文件
	configFilePath := filepath.Join(outboundDir, fmt.Sprintf("%s.json", baseFilename))
	var prettyJson interface{}
	if err := json.Unmarshal(outboundJson, &prettyJson); err != nil {
		return fmt.Errorf("解析JSON失败: %w", err)
	}
	configData, err := json.MarshalIndent(prettyJson, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON序列化失败: %w", err)
	}
	if err := ManagedRuntimeWriteFile(configFilePath, configData); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	// 2. 保存元数据文件
	metaFilePath := filepath.Join(outboundDir, fmt.Sprintf("%s_meta.json", baseFilename))
	metadata := &OutboundMetadata{
		Id:        outbound.Id,
		Tag:       outbound.Tag,
		Type:      outbound.Type,
		UpdatedAt: time.Now().Unix(),
	}
	metaData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("元数据序列化失败: %w", err)
	}
	if err := ManagedRuntimeWriteFile(metaFilePath, metaData); err != nil {
		return fmt.Errorf("写入元数据文件失败: %w", err)
	}

	return nil
}

// ========================================
// 核心完整配置生成
// ========================================

// regenerateCoreConfig 重新生成核心完整配置
// 注意：此函数只更新 core/singbox/config.json 和 Inbound/inbound.json
// 单个入站文件由 regenerateInboundConfigs 负责生成
func (s *ProManagerService) regenerateCoreConfig() {
	if err := EnsureManagedCoreLayout(); err != nil {
		logger.Errorf("[ProManager] 初始化 core 目录结构失败: %v", err)
		return
	}

	config, err := s.GenerateFullConfig()
	if err != nil {
		logger.Errorf("[ProManager] 生成完整配置失败: %v", err)
		return
	}

	// 保存完整配置
	configJson, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		logger.Errorf("[ProManager] 序列化配置失败: %v", err)
		return
	}

	// 保存到 core/singbox/config.json
	filePath := GetSingboxConfigPath()
	if err := ManagedRuntimeWriteFile(filePath, configJson); err != nil {
		logger.Errorf("[ProManager] 写入核心配置失败: %v", err)
		return
	}

	// 同时保存完整配置到 Inbound/inbound.json（兼容旧版）
	// 注意：不清理目录，因为单个入站文件由 regenerateInboundConfigs 负责
	legacyDir := filepath.Join(s.baseDir, "Inbound")
	legacyPath := filepath.Join(legacyDir, "inbound.json")
	if err := ManagedRuntimeWriteFile(legacyPath, configJson); err != nil {
		logger.Warning("[ProManager] 写入兼容 inbound.json 失败: ", err)
	}

	logger.Infof("[ProManager] 已更新核心配置: %s", filePath)
}

// regenerateLegacyInboundFiles 在旧版 Inbound 目录中生成单个入站文件
func (s *ProManagerService) regenerateLegacyInboundFiles(legacyDir string) {
	db := database.GetDB()

	// 获取所有入站
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).Preload("Tls").Find(&inbounds).Error; err != nil {
		logger.Errorf("[ProManager] 获取入站列表失败: %v", err)
		return
	}

	// 为每个入站生成配置文件
	for _, inbound := range inbounds {
		// 生成入站JSON
		inboundJson, err := inbound.MarshalJSON()
		if err != nil {
			logger.Errorf("[ProManager] 序列化入站失败 [%s]: %v", inbound.Tag, err)
			continue
		}

		// 获取关联的用户
		users, err := s.getInboundUsers(db, inbound)
		if err != nil {
			logger.Errorf("[ProManager] 获取用户失败 [%s]: %v", inbound.Tag, err)
			continue
		}

		// 生成带用户的完整入站配置
		fullInboundConfig, err := s.buildFullInboundWithUsers(inboundJson, users, inbound)
		if err != nil {
			logger.Errorf("[ProManager] 构建完整入站配置失败 [%s]: %v", inbound.Tag, err)
			continue
		}

		baseFilename := sanitizeFilename(inbound.Tag)

		// 处理 ShadowTLS 组合入站（同一文件保存，连续两个 JSON 对象，无外层数组）
		if inbound.Type == "shadowtls" {
			shadowtlsJson, ssJson, err := s.processShadowTLSInboundConfig(fullInboundConfig, inbound)
			if err != nil {
				logger.Errorf("[ProManager] 处理ShadowTLS入站失败 [%s]: %v", inbound.Tag, err)
				continue
			}

			if ssJson != nil {
				if err := s.saveShadowTLSCombinedInboundConfigFile(legacyDir, baseFilename, shadowtlsJson, ssJson); err != nil {
					logger.Errorf("[ProManager] 保存ShadowTLS组合配置失败 [%s]: %v", inbound.Tag, err)
				}
			} else {
				if err := s.saveInboundConfigFile(legacyDir, baseFilename, shadowtlsJson); err != nil {
					logger.Errorf("[ProManager] 保存ShadowTLS配置失败 [%s]: %v", inbound.Tag, err)
				}
			}
		} else {
			// 普通入站，直接保存
			if err := s.saveInboundConfigFile(legacyDir, baseFilename, fullInboundConfig); err != nil {
				logger.Errorf("[ProManager] 保存入站配置失败 [%s]: %v", inbound.Tag, err)
			}
		}

		// 保存元数据文件
		metaFilePath := filepath.Join(legacyDir, fmt.Sprintf("%s_meta.json", baseFilename))
		metadata := &InboundMetadata{
			Id:        inbound.Id,
			Tag:       inbound.Tag,
			Type:      inbound.Type,
			TlsId:     inbound.TlsId,
			UserCount: len(users),
			UpdatedAt: time.Now().Unix(),
		}
		metaData, _ := json.MarshalIndent(metadata, "", "  ")
		if err := ManagedRuntimeWriteFile(metaFilePath, metaData); err != nil {
			logger.Warning("[ProManager] 写入兼容入站元数据失败: ", err)
		}
	}
}

// GenerateFullConfig 聚合所有信息并生成完整的 sing-box 配置
func (s *ProManagerService) GenerateFullConfig() (*ProManagerSingBoxConfig, error) {
	// 获取基础配置 (DNS, Route, Log 等)
	baseData, err := s.SettingService.GetConfig()
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
	if err := ensureCoreLogLevel(config); err != nil {
		return nil, fmt.Errorf("规范化日志级别失败: %w", err)
	}
	if err := normalizeSingboxDNSConfig(config); err != nil {
		return nil, fmt.Errorf("规范化 DNS 配置失败: %w", err)
	}

	// 聚合所有数据库对象
	db := database.GetDB()

	config.Inbounds, err = s.InboundService.GetAllConfig(db)
	if err != nil {
		return nil, fmt.Errorf("获取入站配置失败: %w", err)
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

	config.Services, err = s.ServicesService.GetAllConfig(db)
	if err != nil {
		return nil, fmt.Errorf("获取服务配置失败: %w", err)
	}

	config.Endpoints, err = s.EndpointService.GetAllConfig(db)
	if err != nil {
		return nil, fmt.Errorf("获取端点配置失败: %w", err)
	}

	if err := s.applyServerTLSStore(config, outboundTLSStore); err != nil {
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

func ensureCoreLogLevel(config *ProManagerSingBoxConfig) error {
	if config == nil {
		return nil
	}

	logMap := map[string]interface{}{}
	if len(config.Log) > 0 {
		if err := json.Unmarshal(config.Log, &logMap); err != nil {
			return err
		}
	}

	level, hasLevel := logMap["level"].(string)
	if !hasLevel || level == "" {
		logMap["level"] = "panic"
	}

	logData, err := json.Marshal(logMap)
	if err != nil {
		return err
	}
	config.Log = logData
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

// SaveInboundJson 将聚合后的配置异步保存到文件系统 (兼容旧接口)
func (s *ProManagerService) applyServerTLSStore(config *ProManagerSingBoxConfig, fallbackStore string) error {
	enabled, err := s.SettingService.GetServerTLSStoreEnabled()
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
		store, err := s.SettingService.GetServerTLSStore()
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

func (s *ProManagerService) SaveInboundJson() {
	s.regenerateInboundConfigs()
	s.regenerateOutboundConfigs()
	s.regenerateCoreConfig()
	s.SubOutboundService.RegenerateAllSubOutboundConfigs()
	s.regenerateSubJsonConfigs()
}

// ========================================
// JSON订阅配置生成 (使用现有JsonService)
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

// regenerateSubJsonConfigs 重新生成所有JSON订阅配置
// 包括：客户端订阅、订阅出站(SubOutbound)订阅、分组(SubGroup)订阅
// 这三类文件都存储在 sub_json 目录，所以统一清理、统一重建
func (s *ProManagerService) regenerateSubJsonConfigs() {
	db := database.GetDB()
	if err := validateManagedSubJSONFileNames(db); err != nil {
		logger.Errorf("[ProManager] sub_json filename validation failed: %v", err)
		return
	}

	subJsonDir := filepath.Join(s.baseDir, "sub_json")

	// 统一清理 sub_json 目录（一次性清理所有类型的文件）
	s.cleanDirectory(subJsonDir)

	// ============ 1. 重新生成客户端订阅文件 ============
	clientCount := 0

	// 获取所有启用的客户端
	var clients []model.Client
	if err := db.Model(model.Client{}).Where("enable = true").Find(&clients).Error; err != nil {
		logger.Errorf("[ProManager] 获取客户端列表失败: %v", err)
	} else {
		// 获取所有入站，用于生成文件名
		var inbounds []*model.Inbound
		if err := db.Model(model.Inbound{}).Find(&inbounds).Error; err != nil {
			logger.Errorf("[ProManager] 获取入站列表失败: %v", err)
		} else {
			// 创建入站ID到Tag的映射
			inboundTagMap := make(map[uint]string)
			for _, inb := range inbounds {
				inboundTagMap[inb.Id] = inb.Tag
			}

			// 为每个客户端的每个入站生成JSON订阅
			for _, client := range clients {
				var clientInbounds []uint
				if err := json.Unmarshal(client.Inbounds, &clientInbounds); err != nil {
					logger.Errorf("[ProManager] 解析客户端入站列表失败 [%s]: %v", client.Name, err)
					continue
				}

				jsonContent, err := s.getClientJsonSubscription(client.Name)
				if err != nil {
					logger.Errorf("[ProManager] 获取JSON订阅失败 [%s]: %v", client.Name, err)
					continue
				}

				if jsonContent == nil || *jsonContent == "" {
					continue
				}

				for _, inboundId := range clientInbounds {
					inboundTag, exists := inboundTagMap[inboundId]
					if !exists {
						continue
					}

					baseName := buildClientSubJSONBaseName(inboundTag, client.Name)
					if baseName == "" {
						continue
					}
					filename := baseName + ".json"
					filePath := filepath.Join(subJsonDir, filename)

					if err := ManagedRuntimeWriteFile(filePath, []byte(*jsonContent)); err != nil {
						logger.Errorf("[ProManager] 写入JSON订阅失败 [%s]: %v", filename, err)
						continue
					}
					clientCount++
				}
			}
		}
	}

	// ============ 2. 重新生成订阅出站(SubOutbound)的订阅文件 ============
	s.SubOutboundService.RegenerateAllSubJsonFiles()

	// ============ 3. 重新生成分组(SubGroup)的配置文件 ============
	s.SubGroupService.RegenerateAllGroupConfigs()

	logger.Infof("[ProManager] 已重新生成 sub_json 目录：客户端订阅 %d 个 + 订阅出站文件 + 分组文件", clientCount)
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
		if len(inbound.OutJson) < 5 {
			continue
		}

		var outbound map[string]interface{}
		if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
			continue
		}

		protocol, _ := outbound["type"].(string)

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

// cleanDirectory 清理目录中的所有JSON文件
func (s *ProManagerService) cleanDirectory(dir string) {
	if err := ManagedRuntimeClearDirJSONFiles(dir); err != nil {
		logger.Warning("[ProManager] 清理目录失败: ", err)
	}
}

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
