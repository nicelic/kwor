package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service/ddns"
)

type DDNSService struct{}

var (
	ddnsSyncMutex sync.Mutex
)

type DDNSOverviewResponse struct {
	Rules      []model.DDNSRule     `json:"rules"`
	Accounts   []model.DDNSAccount  `json:"accounts"`
	Providers  []ddns.ProviderMeta  `json:"providers"`
	Interfaces []ddns.InterfaceInfo `json:"interfaces"`
}

func (s *DDNSService) GetOverview() (*DDNSOverviewResponse, error) {
	db := database.GetDDNSDB()
	if db == nil {
		return nil, errors.New("failed to connect to ddns database")
	}
	var rules []model.DDNSRule
	if err := db.Order("list_order ASC, id ASC").Find(&rules).Error; err != nil {
		return nil, err
	}

	var accounts []model.DDNSAccount
	if err := db.Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, err
	}

	ifaces, _ := ddns.GetSystemInterfaces()

	return &DDNSOverviewResponse{
		Rules:      rules,
		Accounts:   accounts,
		Providers:  ddns.GetProviderCatalog(),
		Interfaces: ifaces,
	}, nil
}

func (s *DDNSService) SaveRule(rule *model.DDNSRule) error {
	if rule == nil {
		return errors.New("rule cannot be nil")
	}
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" {
		return errors.New("rule name cannot be empty")
	}
	rule.Domains = strings.TrimSpace(rule.Domains)
	if rule.Domains == "" {
		return errors.New("domains cannot be empty")
	}
	if rule.AccountID == 0 {
		return errors.New("must select a DNS account")
	}
	if rule.IntervalMinutes <= 0 {
		rule.IntervalMinutes = 1
	}
	if rule.LocalIntervalSeconds <= 0 {
		rule.LocalIntervalSeconds = 10
	}
	if rule.TTL <= 0 {
		rule.TTL = 60
	}

	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	var account model.DDNSAccount
	if err := db.First(&account, rule.AccountID).Error; err != nil {
		return errors.New("selected DNS account does not exist")
	}

	var saveErr error
	if rule.Id == 0 {
		saveErr = db.Create(rule).Error
	} else {
		saveErr = db.Model(&model.DDNSRule{}).Where("id = ?", rule.Id).Updates(map[string]any{
			"name":                   rule.Name,
			"account_id":             rule.AccountID,
			"domains":                rule.Domains,
			"ip_type":                rule.IPType,
			"ip_v4_source":           rule.IPV4Source,
			"ip_v4_url":              rule.IPV4URL,
			"ip_v4_interface":        rule.IPV4Interface,
			"ip_v6_source":           rule.IPV6Source,
			"ip_v6_url":              rule.IPV6URL,
			"ip_v6_interface":        rule.IPV6Interface,
			"interval_minutes":       rule.IntervalMinutes,
			"local_interval_seconds": rule.LocalIntervalSeconds,
			"ttl":                    rule.TTL,
			"cloudflare_proxy":       rule.CloudflareProxy,
			"last_sync_time":         nil,
		}).Error
	}
	if saveErr == nil {
		WakeDDNSRuntimeWorker()
	}
	return saveErr
}

func (s *DDNSService) ToggleRule(id uint, enabled bool) error {
	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	err := db.Model(&model.DDNSRule{}).Where("id = ?", id).Update("enabled", enabled).Error
	if err == nil {
		WakeDDNSRuntimeWorker()
	}
	return err
}

func (s *DDNSService) DeleteRule(id uint) error {
	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}

	var rule model.DDNSRule
	if err := db.First(&rule, id).Error; err != nil {
		return fmt.Errorf("ddns rule not found: %w", err)
	}

	// 1. 如果该规则绑定了 DNS 账号且配置了域名，必须先向服务商调用删除记录 API
	if rule.AccountID > 0 && strings.TrimSpace(rule.Domains) != "" {
		var account model.DDNSAccount
		if err := db.First(&account, rule.AccountID).Error; err == nil {
			p, pErr := ddns.GetProvider(account.ProviderCode)
			if pErr != nil {
				return fmt.Errorf("cannot delete remote records: %w", pErr)
			}

			var env map[string]string
			if err := json.Unmarshal([]byte(account.EnvJSON), &env); err != nil {
				return fmt.Errorf("invalid account credentials format: %w", err)
			}

			needV4 := rule.IPType == "ipv4" || rule.IPType == "dual"
			needV6 := rule.IPType == "ipv6" || rule.IPType == "dual"

			rawDomains := strings.ReplaceAll(rule.Domains, "\r", "")
			rawDomains = strings.ReplaceAll(rawDomains, "\n", ",")
			domainList := strings.Split(rawDomains, ",")

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			var deleteErrors []string

			for _, rawDomain := range domainList {
				domain := strings.TrimSpace(rawDomain)
				if domain == "" {
					continue
				}

				// 删除 IPv4 (A 记录)
				if needV4 {
					param := ddns.RecordParam{
						Domain: domain,
						Type:   "A",
						IP:     "",
					}
					if err := p.DeleteRecord(ctx, env, param); err != nil {
						deleteErrors = append(deleteErrors, fmt.Sprintf("%s (A): %v", domain, err))
					} else {
						logger.Infof("[DDNS] Deleted remote A record for %s via %s", domain, p.Name())
					}
					for _, ip := range parseIPList(rule.LastIPV4) {
						_ = p.DeleteRecord(ctx, env, ddns.RecordParam{Domain: domain, Type: "A", IP: ip})
					}
				}

				// 删除 IPv6 (AAAA 记录)
				if needV6 {
					param := ddns.RecordParam{
						Domain: domain,
						Type:   "AAAA",
						IP:     "",
					}
					if err := p.DeleteRecord(ctx, env, param); err != nil {
						deleteErrors = append(deleteErrors, fmt.Sprintf("%s (AAAA): %v", domain, err))
					} else {
						logger.Infof("[DDNS] Deleted remote AAAA record for %s via %s", domain, p.Name())
					}
					for _, ip := range parseIPList(rule.LastIPV6) {
						_ = p.DeleteRecord(ctx, env, ddns.RecordParam{Domain: domain, Type: "AAAA", IP: ip})
					}
				}
			}

			// 强一致性保障：若云商删除失败，绝对不能删除本地规则，必须向用户汇报错误
			if len(deleteErrors) > 0 {
				errMsg := fmt.Sprintf("服务商解析删除失败: %s", strings.Join(deleteErrors, "; "))
				logger.Errorf("[DDNS] Failed to delete remote DNS records for rule %s: %s", rule.Name, errMsg)
				return errors.New(errMsg)
			}
		}
	}

	// 2. 确认服务商解析记录删除完毕或无远端记录后，从本地 SQLite 数据库删除规则
	err := db.Delete(&model.DDNSRule{}, id).Error
	if err == nil {
		WakeDDNSRuntimeWorker()
		logger.Infof("[DDNS] Successfully deleted DDNS rule: %s (id: %d)", rule.Name, id)
	}
	return err
}

func (s *DDNSService) SaveAccount(account *model.DDNSAccount) error {
	if account == nil {
		return errors.New("account cannot be nil")
	}
	account.Name = strings.TrimSpace(account.Name)
	if account.Name == "" {
		return errors.New("account name cannot be empty")
	}
	if account.ProviderCode == "" {
		return errors.New("provider code cannot be empty")
	}

	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	var saveErr error
	if account.Id == 0 {
		saveErr = db.Create(account).Error
	} else {
		saveErr = db.Model(&model.DDNSAccount{}).Where("id = ?", account.Id).Updates(map[string]any{
			"name":          account.Name,
			"provider_code": account.ProviderCode,
			"env_json":      account.EnvJSON,
			"remark":        account.Remark,
		}).Error
	}
	if saveErr == nil {
		WakeDDNSRuntimeWorker()
	}
	return saveErr
}

func (s *DDNSService) DeleteAccount(id uint) error {
	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	var count int64
	if err := db.Model(&model.DDNSRule{}).Where("account_id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot delete DNS account: %d rule(s) are referencing it", count)
	}
	err := db.Delete(&model.DDNSAccount{}, id).Error
	if err == nil {
		WakeDDNSRuntimeWorker()
	}
	return err
}

func (s *DDNSService) TestAccountAuth(account *model.DDNSAccount) error {
	p, err := ddns.GetProvider(account.ProviderCode)
	if err != nil {
		return err
	}

	var env map[string]string
	if err := json.Unmarshal([]byte(account.EnvJSON), &env); err != nil {
		return errors.New("invalid credentials format")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	return p.TestAuth(ctx, env)
}

func (s *DDNSService) SyncRule(id uint) error {
	ddnsSyncMutex.Lock()
	defer ddnsSyncMutex.Unlock()

	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	var rule model.DDNSRule
	if err := db.First(&rule, id).Error; err != nil {
		return err
	}

	return s.executeSyncRule(&rule, true)
}

func (s *DDNSService) SyncAllRules() error {
	ddnsSyncMutex.Lock()
	defer ddnsSyncMutex.Unlock()

	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	var rules []model.DDNSRule
	if err := db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return err
	}

	var errList []string
	for i := range rules {
		if err := s.executeSyncRule(&rules[i], true); err != nil {
			errList = append(errList, fmt.Sprintf("[%s]: %v", rules[i].Name, err))
		}
	}

	if len(errList) > 0 {
		return errors.New(strings.Join(errList, "; "))
	}
	return nil
}

// detectRuleIPs 执行本地网卡或公网接口的 IP 并发探测（纯网络与本地 I/O，不涉及云商更新）
func (s *DDNSService) detectRuleIPs(ctx context.Context, rule *model.DDNSRule) (currentV4s, currentV6s []string, detectErrors []string) {
	needV4 := rule.IPType == "ipv4" || rule.IPType == "dual"
	needV6 := rule.IPType == "ipv6" || rule.IPType == "dual"

	if needV4 {
		hasInterface := strings.Contains(rule.IPV4Source, "interface") || rule.IPV4Source == "both"
		hasAPI := strings.Contains(rule.IPV4Source, "api") || rule.IPV4Source == "both" || rule.IPV4Source == ""
		if !hasInterface && !hasAPI {
			hasAPI = true
		}

		v4Map := make(map[string]struct{})

		if hasInterface && rule.IPV4Interface != "" {
			publicOnly := hasAPI
			ifaceV4s, err := ddns.DetectInterfaceIPs(rule.IPV4Interface, "ipv4", publicOnly)
			if err != nil {
				if !hasAPI {
					detectErrors = append(detectErrors, "IPv4 iface detect error: "+err.Error())
				}
			} else {
				for _, ip := range ifaceV4s {
					v4Map[ip] = struct{}{}
				}
			}
		}

		if hasAPI {
			apiV4s, err := ddns.DetectPublicIPs(ctx, "ipv4", rule.IPV4URL)
			if err != nil {
				if len(v4Map) == 0 {
					detectErrors = append(detectErrors, "IPv4 API detect error: "+err.Error())
				}
			} else {
				for _, ip := range apiV4s {
					v4Map[ip] = struct{}{}
				}
			}
		}

		for ip := range v4Map {
			currentV4s = append(currentV4s, ip)
		}
		sort.Strings(currentV4s)
	}

	if needV6 {
		hasInterface := strings.Contains(rule.IPV6Source, "interface") || rule.IPV6Source == "both"
		hasAPI := strings.Contains(rule.IPV6Source, "api") || rule.IPV6Source == "both"
		if rule.IPV6Source != "disabled" && !hasInterface && !hasAPI {
			hasAPI = true
		}

		v6Map := make(map[string]struct{})

		if hasInterface && rule.IPV6Interface != "" {
			publicOnly := hasAPI
			ifaceV6s, err := ddns.DetectInterfaceIPs(rule.IPV6Interface, "ipv6", publicOnly)
			if err != nil {
				if !hasAPI {
					detectErrors = append(detectErrors, "IPv6 iface detect error: "+err.Error())
				}
			} else {
				for _, ip := range ifaceV6s {
					v6Map[ip] = struct{}{}
				}
			}
		}

		if hasAPI {
			apiV6s, err := ddns.DetectPublicIPs(ctx, "ipv6", rule.IPV6URL)
			if err != nil {
				if len(v6Map) == 0 {
					detectErrors = append(detectErrors, "IPv6 API detect error: "+err.Error())
				}
			} else {
				for _, ip := range apiV6s {
					v6Map[ip] = struct{}{}
				}
			}
		}

		for ip := range v6Map {
			currentV6s = append(currentV6s, ip)
		}
		sort.Strings(currentV6s)
	}

	return currentV4s, currentV6s, detectErrors
}

func parseIPList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' '
	})
	seen := make(map[string]bool)
	var list []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			list = append(list, p)
		}
	}
	sort.Strings(list)
	return list
}

func diffIPs(targetIPs, lastIPs []string) (toAdd, toDelete []string) {
	targetSet := make(map[string]bool)
	for _, ip := range targetIPs {
		targetSet[ip] = true
	}
	lastSet := make(map[string]bool)
	for _, ip := range lastIPs {
		lastSet[ip] = true
	}

	for _, ip := range targetIPs {
		if !lastSet[ip] {
			toAdd = append(toAdd, ip)
		}
	}
	for _, ip := range lastIPs {
		if !targetSet[ip] {
			toDelete = append(toDelete, ip)
		}
	}
	return toAdd, toDelete
}

type DDNSSyncPlan struct {
	TargetV4s  []string
	TargetV6s  []string
	V4ToAdd    []string
	V4ToDelete []string
	V6ToAdd    []string
	V6ToDelete []string
	NeedV4     bool
	NeedV6     bool
	HasChanges bool
}

func (s *DDNSService) CalculateSyncPlan(rule *model.DDNSRule, currentV4s, currentV6s []string, detectErrors []string) DDNSSyncPlan {
	needV4 := rule.IPType == "ipv4" || rule.IPType == "dual"
	needV6 := rule.IPType == "ipv6" || rule.IPType == "dual"

	lastV4s := parseIPList(rule.LastIPV4)
	lastV6s := parseIPList(rule.LastIPV6)

	var targetV4s []string
	if needV4 {
		if len(currentV4s) > 0 {
			targetV4s = currentV4s
		} else if len(detectErrors) == 0 {
			targetV4s = []string{}
		} else {
			// 探测出错且未获得任何有效 IP，保留上一轮 IP 避免误清空
			targetV4s = lastV4s
		}
	} else {
		// 规则明确关闭了 IPv4，期望目标为空
		targetV4s = []string{}
	}

	var targetV6s []string
	if needV6 {
		if len(currentV6s) > 0 {
			targetV6s = currentV6s
		} else if len(detectErrors) == 0 {
			targetV6s = []string{}
		} else {
			// 探测出错且未获得任何有效 IP，保留上一轮 IP 避免误清空
			targetV6s = lastV6s
		}
	} else {
		// 规则明确关闭了 IPv6，期望目标为空
		targetV6s = []string{}
	}

	v4ToAdd, v4ToDelete := diffIPs(targetV4s, lastV4s)
	v6ToAdd, v6ToDelete := diffIPs(targetV6s, lastV6s)

	hasChanges := len(v4ToAdd) > 0 || len(v4ToDelete) > 0 || len(v6ToAdd) > 0 || len(v6ToDelete) > 0

	return DDNSSyncPlan{
		TargetV4s:  targetV4s,
		TargetV6s:  targetV6s,
		V4ToAdd:    v4ToAdd,
		V4ToDelete: v4ToDelete,
		V6ToAdd:    v6ToAdd,
		V6ToDelete: v6ToDelete,
		NeedV4:     needV4,
		NeedV6:     needV6,
		HasChanges: hasChanges,
	}
}

// touchRuleSyncTime 快路径短路：当 IP 未变动且非强制更新时，仅更新最后同步时间
func (s *DDNSService) touchRuleSyncTime(id uint) {
	db := database.GetDDNSDB()
	if db == nil {
		return
	}
	now := time.Now()
	_ = db.Model(&model.DDNSRule{}).Where("id = ?", id).Update("last_sync_time", &now).Error
}

// syncRuleDNS 慢路径更新：向 DNS 服务商推送/删除 A / AAAA 记录
func (s *DDNSService) syncRuleDNS(ctx context.Context, rule *model.DDNSRule, plan DDNSSyncPlan) error {
	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	var account model.DDNSAccount
	if err := db.First(&account, rule.AccountID).Error; err != nil {
		s.updateRuleStatus(rule.Id, "error", "DNS account not found: "+err.Error(), strings.Join(plan.TargetV4s, ","), strings.Join(plan.TargetV6s, ","))
		return err
	}

	p, err := ddns.GetProvider(account.ProviderCode)
	if err != nil {
		s.updateRuleStatus(rule.Id, "error", err.Error(), strings.Join(plan.TargetV4s, ","), strings.Join(plan.TargetV6s, ","))
		return err
	}

	var env map[string]string
	if err := json.Unmarshal([]byte(account.EnvJSON), &env); err != nil {
		s.updateRuleStatus(rule.Id, "error", "invalid account credentials format", strings.Join(plan.TargetV4s, ","), strings.Join(plan.TargetV6s, ","))
		return err
	}

	rawDomains := strings.ReplaceAll(rule.Domains, "\r", "")
	rawDomains = strings.ReplaceAll(rawDomains, "\n", ",")
	domainList := strings.Split(rawDomains, ",")

	var syncErrors []string

	for _, rawDomain := range domainList {
		domain := strings.TrimSpace(rawDomain)
		if domain == "" {
			continue
		}

		// --- IPv4 同步与删除 ---
		if !plan.NeedV4 {
			// 明确禁用 IPv4，若之前存在 IPv4 则清除远端 A 记录
			if rule.LastIPV4 != "" || len(plan.V4ToDelete) > 0 {
				param := ddns.RecordParam{Domain: domain, Type: "A", IP: ""}
				if err := p.DeleteRecord(ctx, env, param); err != nil {
					syncErrors = append(syncErrors, fmt.Sprintf("%s (clear A): %v", domain, err))
				} else {
					logger.Infof("[DDNS] Cleared obsolete A records for %s via %s", domain, p.Name())
				}
				for _, ip := range plan.V4ToDelete {
					_ = p.DeleteRecord(ctx, env, ddns.RecordParam{Domain: domain, Type: "A", IP: ip})
				}
			}
		} else {
			// 删除已被废弃或不再需要的 IPv4
			for _, ip := range plan.V4ToDelete {
				param := ddns.RecordParam{Domain: domain, Type: "A", IP: ip}
				if err := p.DeleteRecord(ctx, env, param); err != nil {
					syncErrors = append(syncErrors, fmt.Sprintf("%s (del A %s): %v", domain, ip, err))
				} else {
					logger.Infof("[DDNS] Deleted obsolete A record for %s -> %s via %s", domain, ip, p.Name())
				}
			}
			// 添加或更新新增的 IPv4
			for _, ip := range plan.V4ToAdd {
				param := ddns.RecordParam{
					Domain: domain,
					Type:   "A",
					IP:     ip,
					TTL:    rule.TTL,
					Proxy:  rule.CloudflareProxy,
				}
				if _, err := p.SyncRecord(ctx, env, param); err != nil {
					syncErrors = append(syncErrors, fmt.Sprintf("%s (add A %s): %v", domain, ip, err))
				} else {
					logger.Infof("[DDNS] Synced A record for %s -> %s via %s", domain, ip, p.Name())
				}
			}
		}

		// --- IPv6 同步与删除 ---
		if !plan.NeedV6 {
			// 明确禁用 IPv6（如双栈切为仅 IPv4），全量清除远端 AAAA 记录
			if rule.LastIPV6 != "" || len(plan.V6ToDelete) > 0 {
				param := ddns.RecordParam{Domain: domain, Type: "AAAA", IP: ""}
				if err := p.DeleteRecord(ctx, env, param); err != nil {
					syncErrors = append(syncErrors, fmt.Sprintf("%s (clear AAAA): %v", domain, err))
				} else {
					logger.Infof("[DDNS] Cleared obsolete AAAA records for %s via %s", domain, p.Name())
				}
				for _, ip := range plan.V6ToDelete {
					_ = p.DeleteRecord(ctx, env, ddns.RecordParam{Domain: domain, Type: "AAAA", IP: ip})
				}
			}
		} else {
			// 删除已被废弃或不再需要的 IPv6
			for _, ip := range plan.V6ToDelete {
				param := ddns.RecordParam{Domain: domain, Type: "AAAA", IP: ip}
				if err := p.DeleteRecord(ctx, env, param); err != nil {
					syncErrors = append(syncErrors, fmt.Sprintf("%s (del AAAA %s): %v", domain, ip, err))
				} else {
					logger.Infof("[DDNS] Deleted obsolete AAAA record for %s -> %s via %s", domain, ip, p.Name())
				}
			}
			// 添加或更新新增的 IPv6
			for _, ip := range plan.V6ToAdd {
				param := ddns.RecordParam{
					Domain: domain,
					Type:   "AAAA",
					IP:     ip,
					TTL:    rule.TTL,
					Proxy:  rule.CloudflareProxy,
				}
				if _, err := p.SyncRecord(ctx, env, param); err != nil {
					syncErrors = append(syncErrors, fmt.Sprintf("%s (add AAAA %s): %v", domain, ip, err))
				} else {
					logger.Infof("[DDNS] Synced AAAA record for %s -> %s via %s", domain, ip, p.Name())
				}
			}
		}
	}

	targetV4Str := strings.Join(plan.TargetV4s, ",")
	targetV6Str := strings.Join(plan.TargetV6s, ",")

	if len(syncErrors) > 0 {
		errMsg := strings.Join(syncErrors, "; ")
		// 严禁脏写未生效 IP：推送失败时必须保留远端确认生效的旧 IP，防止被误判为“已生效”而在下个周期短路跳过重试
		s.updateRuleStatus(rule.Id, "error", errMsg, rule.LastIPV4, rule.LastIPV6)
		return errors.New(errMsg)
	}

	s.updateRuleStatus(rule.Id, "success", "", targetV4Str, targetV6Str)
	return nil
}

func (s *DDNSService) executeSyncRule(rule *model.DDNSRule, force bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// 1. 高频 IP 并发探测（事务外）
	currentV4s, currentV6s, detectErrors := s.detectRuleIPs(ctx, rule)

	if len(detectErrors) > 0 && len(currentV4s) == 0 && len(currentV6s) == 0 {
		errMsg := strings.Join(detectErrors, "; ")
		s.updateRuleStatus(rule.Id, "error", errMsg, rule.LastIPV4, rule.LastIPV6)
		return errors.New(errMsg)
	}

	plan := s.CalculateSyncPlan(rule, currentV4s, currentV6s, detectErrors)

	// 2. 快慢分离短路校验：仅在非强制同步、IP未变动、非首次同步且上次同步状态非错误时才短路跳过外部云商推送
	isFirstSync := rule.LastSyncTime == nil
	hasPreviousError := rule.LastStatus == "error"
	if !force && !plan.HasChanges && !isFirstSync && !hasPreviousError {
		s.touchRuleSyncTime(rule.Id)
		logger.Infof("[DDNS] Rule %s: probed IPs unchanged (v4: %s, v6: %s), skipping DNS API push", rule.Name, rule.LastIPV4, rule.LastIPV6)
		return nil
	}

	// 3. 执行慢路径云商 DNS 同步
	return s.syncRuleDNS(ctx, rule, plan)
}

func (s *DDNSService) updateRuleStatus(id uint, status, lastError, v4, v6 string) {
	db := database.GetDDNSDB()
	if db == nil {
		return
	}
	now := time.Now()
	updates := map[string]any{
		"last_sync_time": &now,
		"last_status":    status,
		"last_error":     lastError,
		"last_ip_v4":     v4,
		"last_ip_v6":     v6,
	}
	_ = db.Model(&model.DDNSRule{}).Where("id = ?", id).Updates(updates).Error
}
