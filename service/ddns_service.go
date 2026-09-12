package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
						IP:     rule.LastIPV4,
					}
					if err := p.DeleteRecord(ctx, env, param); err != nil {
						deleteErrors = append(deleteErrors, fmt.Sprintf("%s (A): %v", domain, err))
					} else {
						logger.Infof("[DDNS] Deleted remote A record for %s via %s", domain, p.Name())
					}
				}

				// 删除 IPv6 (AAAA 记录)
				if needV6 {
					param := ddns.RecordParam{
						Domain: domain,
						Type:   "AAAA",
						IP:     rule.LastIPV6,
					}
					if err := p.DeleteRecord(ctx, env, param); err != nil {
						deleteErrors = append(deleteErrors, fmt.Sprintf("%s (AAAA): %v", domain, err))
					} else {
						logger.Infof("[DDNS] Deleted remote AAAA record for %s via %s", domain, p.Name())
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

// detectRuleIPs 执行本地网卡或公网接口的 IP 探测（纯网络与本地 I/O，不涉及云商更新）
func (s *DDNSService) detectRuleIPs(ctx context.Context, rule *model.DDNSRule) (currentV4, currentV6 string, detectErrors []string) {
	needV4 := rule.IPType == "ipv4" || rule.IPType == "dual"
	needV6 := rule.IPType == "ipv6" || rule.IPType == "dual"

	if needV4 {
		hasInterface := strings.Contains(rule.IPV4Source, "interface") || rule.IPV4Source == "both"
		hasAPI := strings.Contains(rule.IPV4Source, "api") || rule.IPV4Source == "both" || rule.IPV4Source == ""
		if !hasInterface && !hasAPI {
			hasAPI = true
		}

		if hasInterface && rule.IPV4Interface != "" {
			publicOnly := hasAPI
			v4, err := ddns.DetectInterfaceIP(rule.IPV4Interface, "ipv4", publicOnly)
			if err != nil {
				if !hasAPI {
					detectErrors = append(detectErrors, "IPv4 iface detect error: "+err.Error())
				}
			} else if v4 != "" {
				currentV4 = v4
			}
		}

		if currentV4 == "" && hasAPI {
			v4, err := ddns.DetectPublicIP(ctx, "ipv4", rule.IPV4URL)
			if err != nil {
				detectErrors = append(detectErrors, "IPv4 API detect error: "+err.Error())
			} else {
				currentV4 = v4
			}
		}
	}

	if needV6 {
		hasInterface := strings.Contains(rule.IPV6Source, "interface") || rule.IPV6Source == "both"
		hasAPI := strings.Contains(rule.IPV6Source, "api") || rule.IPV6Source == "both"
		if rule.IPV6Source != "disabled" && !hasInterface && !hasAPI {
			hasAPI = true
		}

		if hasInterface && rule.IPV6Interface != "" {
			publicOnly := hasAPI
			v6, err := ddns.DetectInterfaceIP(rule.IPV6Interface, "ipv6", publicOnly)
			if err != nil {
				if !hasAPI {
					detectErrors = append(detectErrors, "IPv6 iface detect error: "+err.Error())
				}
			} else if v6 != "" {
				currentV6 = v6
			}
		}

		if currentV6 == "" && hasAPI {
			v6, err := ddns.DetectPublicIP(ctx, "ipv6", rule.IPV6URL)
			if err != nil {
				detectErrors = append(detectErrors, "IPv6 API detect error: "+err.Error())
			} else {
				currentV6 = v6
			}
		}
	}

	return currentV4, currentV6, detectErrors
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

// syncRuleDNS 慢路径更新：向 DNS 服务商推送 A / AAAA 记录
func (s *DDNSService) syncRuleDNS(ctx context.Context, rule *model.DDNSRule, currentV4, currentV6 string, v4Changed, v6Changed, force bool) error {
	db := database.GetDDNSDB()
	if db == nil {
		return errors.New("failed to connect to ddns database")
	}
	var account model.DDNSAccount
	if err := db.First(&account, rule.AccountID).Error; err != nil {
		s.updateRuleStatus(rule.Id, "error", "DNS account not found: "+err.Error(), currentV4, currentV6)
		return err
	}

	p, err := ddns.GetProvider(account.ProviderCode)
	if err != nil {
		s.updateRuleStatus(rule.Id, "error", err.Error(), currentV4, currentV6)
		return err
	}

	var env map[string]string
	if err := json.Unmarshal([]byte(account.EnvJSON), &env); err != nil {
		s.updateRuleStatus(rule.Id, "error", "invalid account credentials format", currentV4, currentV6)
		return err
	}

	needV4 := rule.IPType == "ipv4" || rule.IPType == "dual"
	needV6 := rule.IPType == "ipv6" || rule.IPType == "dual"

	rawDomains := strings.ReplaceAll(rule.Domains, "\r", "")
	rawDomains = strings.ReplaceAll(rawDomains, "\n", ",")
	domainList := strings.Split(rawDomains, ",")

	var syncErrors []string

	for _, rawDomain := range domainList {
		domain := strings.TrimSpace(rawDomain)
		if domain == "" {
			continue
		}

		// 同步 IPv4
		if needV4 && currentV4 != "" && (force || v4Changed) {
			param := ddns.RecordParam{
				Domain: domain,
				Type:   "A",
				IP:     currentV4,
				TTL:    rule.TTL,
				Proxy:  rule.CloudflareProxy,
			}
			if _, err := p.SyncRecord(ctx, env, param); err != nil {
				syncErrors = append(syncErrors, fmt.Sprintf("%s (A): %v", domain, err))
			} else {
				logger.Infof("[DDNS] Updated A record for %s -> %s via %s", domain, currentV4, p.Name())
			}
		}

		// 同步 IPv6
		if needV6 && currentV6 != "" && (force || v6Changed) {
			param := ddns.RecordParam{
				Domain: domain,
				Type:   "AAAA",
				IP:     currentV6,
				TTL:    rule.TTL,
				Proxy:  rule.CloudflareProxy,
			}
			if _, err := p.SyncRecord(ctx, env, param); err != nil {
				syncErrors = append(syncErrors, fmt.Sprintf("%s (AAAA): %v", domain, err))
			} else {
				logger.Infof("[DDNS] Updated AAAA record for %s -> %s via %s", domain, currentV6, p.Name())
			}
		}
	}

	if len(syncErrors) > 0 {
		errMsg := strings.Join(syncErrors, "; ")
		s.updateRuleStatus(rule.Id, "error", errMsg, currentV4, currentV6)
		return errors.New(errMsg)
	}

	s.updateRuleStatus(rule.Id, "success", "", currentV4, currentV6)
	return nil
}

func (s *DDNSService) executeSyncRule(rule *model.DDNSRule, force bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// 1. 高频 IP 探测（事务外）
	currentV4, currentV6, detectErrors := s.detectRuleIPs(ctx, rule)

	if len(detectErrors) > 0 && currentV4 == "" && currentV6 == "" {
		errMsg := strings.Join(detectErrors, "; ")
		s.updateRuleStatus(rule.Id, "error", errMsg, currentV4, currentV6)
		return errors.New(errMsg)
	}

	needV4 := rule.IPType == "ipv4" || rule.IPType == "dual"
	needV6 := rule.IPType == "ipv6" || rule.IPType == "dual"

	// 2. 快慢分离短路校验：如果未变动且非强制，则跳过外部云商推送
	v4Changed := needV4 && currentV4 != "" && currentV4 != rule.LastIPV4
	v6Changed := needV6 && currentV6 != "" && currentV6 != rule.LastIPV6

	if !force && !v4Changed && !v6Changed {
		s.touchRuleSyncTime(rule.Id)
		return nil
	}

	// 3. 执行慢路径云商 DNS 同步
	return s.syncRuleDNS(ctx, rule, currentV4, currentV6, v4Changed, v6Changed, force)
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
	}
	if v4 != "" {
		updates["last_ip_v4"] = v4
	}
	if v6 != "" {
		updates["last_ip_v6"] = v6
	}
	_ = db.Model(&model.DDNSRule{}).Where("id = ?", id).Updates(updates).Error
}
