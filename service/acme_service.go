package service

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"io"
	"math"
	"net/http"
	"net/mail"
	"net/netip"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/network"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	acmeScriptPathKey          = "acmeScriptPath"
	acmeContactEmailKey        = "acmeContactEmail"
	acmePreferredCAKey         = "acmePreferredCA"
	acmeDefaultChallengeKey    = "acmeDefaultChallenge"
	acmeDefaultWebrootKey      = "acmeDefaultWebroot"
	acmeDefaultDNSProviderKey  = "acmeDefaultDNSProvider"
	acmeDefaultKeyLengthKey    = "acmeDefaultKeyLength"
	acmeAutoUpgradeKey         = "acmeAutoUpgrade"
	acmeManagedPathManifestKey = "acmeManagedPathManifest"

	defaultAcmePreferredCA           = "letsencrypt"
	defaultAcmeChallenge             = "standalone"
	defaultAcmeKeyLength             = "ec-256"
	defaultAcmeAutoRenewDays         = 30
	defaultAcmeInstallScriptURL      = "https://raw.githubusercontent.com/acmesh-official/acme.sh/master/acme.sh"
	acmeGitHubReleasesAPI            = "https://api.github.com/repos/acmesh-official/acme.sh/releases"
	acmeGitHubReleaseTagAPI          = "https://api.github.com/repos/acmesh-official/acme.sh/releases/tags/"
	acmeGitHubTagsAPI                = "https://api.github.com/repos/acmesh-official/acme.sh/tags"
	acmeLogMaxLines                  = 800
	acmeLogTTL                       = 30 * time.Minute
	acmeCertificateTypeDomain        = "domain"
	acmeCertificateTypeIP            = "ip"
	acmeLEProductionDirectory        = "https://acme-v02.api.letsencrypt.org/directory"
	acmeLEStagingDirectory           = "https://acme-staging-v02.api.letsencrypt.org/directory"
	acmeZeroSSLDirectory             = "https://acme.zerossl.com/v2/DV90"
	acmeIPCertificateMaxIPs          = 100
	acmeIPCertificatePortHTTP        = 80
	acmeIPCertificatePortALPN        = 443
	acmeMaskedEnvValue               = "********"
	acmeManagedWorkspaceStagePrefix  = "acme-home-stage-"
	acmeManagedWorkspaceBackupPrefix = "acme-home-backup-"
)

type acmeIPFamilyMode string

const (
	acmeIPFamilyUnknown acmeIPFamilyMode = ""
	acmeIPFamilyIPv4    acmeIPFamilyMode = "ipv4"
	acmeIPFamilyIPv6    acmeIPFamilyMode = "ipv6"
	acmeIPFamilyDual    acmeIPFamilyMode = "dual"
)

var (
	acmeOperationMu      sync.Mutex
	acmeLegacyDNSMu      sync.Mutex
	acmeAutoRenewRunning atomic.Bool
	acmeMaintenanceMu    sync.Mutex
	acmeMaintenanceAt    atomic.Int64
	acmeMaintenanceDB    atomic.Value

	acmeEnvPattern      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	acmeLogIDPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{8,96}$`)
	acmeAnsiCodePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	acmeLogSessionStore = newAcmeLogStore()

	acmeManagedRootFileNames = map[string]struct{}{
		"acme.sh":         {},
		"account.conf":    {},
		"ca.conf":         {},
		"http.header":     {},
		"http.header.bak": {},
	}

	acmeManagedRootDirNames = map[string]struct{}{
		"ca":     {},
		"dnsapi": {},
		"deploy": {},
		"notify": {},
	}

	acmeSystemdUnitCandidates = []string{
		"acme.sh.service",
		"acme.sh.timer",
		"acme.service",
		"acme.timer",
		"acme-renew.service",
		"acme-renew.timer",
		"acme_renew.service",
		"acme_renew.timer",
	}
)

var defaultAcmeCAOptions = []AcmeCAOption{
	{Name: "Let's Encrypt", Value: "letsencrypt"},
	{Name: "ZeroSSL", Value: "zerossl"},
}

var defaultAcmeDNSProviderCatalog = []AcmeDNSProviderMeta{
	{
		Name:         "阿里云",
		ProviderCode: "dns_ali",
		Helper:       "acme.sh 官方: dns_ali",
		Fields: []AcmeDNSFieldDef{
			{Key: "Ali_Key", Label: "Access Key", Required: true},
			{Key: "Ali_Secret", Label: "Secret Key", Required: true},
		},
	},
	{
		Name:         "腾讯云 DNSPod",
		ProviderCode: "dns_tencent",
		Helper:       "acme.sh 官方: dns_tencent",
		Fields: []AcmeDNSFieldDef{
			{Key: "Tencent_SecretId", Label: "SecretId", Required: true},
			{Key: "Tencent_SecretKey", Label: "SecretKey", Required: true},
		},
	},
	{
		Name:         "Cloudflare",
		ProviderCode: "dns_cf",
		Helper:       "acme.sh 官方: dns_cf；支持 Token 模式（CF_Token + CF_Account_ID/CF_Zone_ID）或 Global Key 模式（CF_Email + CF_Key）",
		Fields: []AcmeDNSFieldDef{
			{Key: "CF_Token", Label: "API Token", Required: false},
			{Key: "CF_Account_ID", Label: "Account ID（可选）", Required: false},
			{Key: "CF_Zone_ID", Label: "Zone ID（可选）", Required: false},
			{Key: "CF_Email", Label: "Global API Email（可选）", Required: false},
			{Key: "CF_Key", Label: "Global API Key（可选）", Required: false},
		},
	},
	{
		Name:         "Amazon Route53",
		ProviderCode: "dns_aws",
		Helper:       "acme.sh 官方: dns_aws；支持静态 AK/SK，或留空 AK/SK 使用实例/容器 IAM Role",
		Fields: []AcmeDNSFieldDef{
			{Key: "AWS_ACCESS_KEY_ID", Label: "Access Key ID（可选）", Required: false},
			{Key: "AWS_SECRET_ACCESS_KEY", Label: "Secret Access Key（可选）", Required: false},
			{Key: "AWS_DNS_SLOWRATE", Label: "Slow Rate Seconds（可选）", Required: false},
		},
	},
	{
		Name:         "华为云",
		ProviderCode: "dns_huaweicloud",
		Helper:       "acme.sh 官方: dns_huaweicloud",
		Fields: []AcmeDNSFieldDef{
			{Key: "HUAWEICLOUD_Username", Label: "用户名", Required: true},
			{Key: "HUAWEICLOUD_Password", Label: "密码", Required: true},
			{Key: "HUAWEICLOUD_DomainName", Label: "DomainName", Required: true},
		},
	},
	{
		Name:         "GoDaddy",
		ProviderCode: "dns_gd",
		Helper:       "acme.sh 官方: dns_gd",
		Fields: []AcmeDNSFieldDef{
			{Key: "GD_Key", Label: "API Key", Required: true},
			{Key: "GD_Secret", Label: "API Secret", Required: true},
		},
	},
	{
		Name:         "Vercel",
		ProviderCode: "dns_vercel",
		Helper:       "acme.sh 官方: dns_vercel",
		Fields: []AcmeDNSFieldDef{
			{Key: "VERCEL_TOKEN", Label: "API Token", Required: true},
		},
	},
}

type AcmeService struct {
	SettingService
}

var certificateInventory = &CertificateInventoryService{}

type AcmeOverview struct {
	Supported          bool                    `json:"supported"`
	Installed          bool                    `json:"installed"`
	Version            string                  `json:"version"`
	ScriptPath         string                  `json:"scriptPath"`
	HomeDir            string                  `json:"homeDir"`
	ContactEmail       string                  `json:"contactEmail"`
	PreferredCA        string                  `json:"preferredCA"`
	DefaultChallenge   string                  `json:"defaultChallenge"`
	DefaultWebroot     string                  `json:"defaultWebroot"`
	DefaultDNSProvider string                  `json:"defaultDnsProvider"`
	DefaultKeyLength   string                  `json:"defaultKeyLength"`
	AutoRenewWindow    AcmeAutoRenewWindowInfo `json:"autoRenewWindow"`
	AutoUpgrade        bool                    `json:"autoUpgrade"`
	CAOptions          []AcmeCAOption          `json:"caOptions"`
	DNSProviders       []AcmeDNSProviderMeta   `json:"dnsProviders"`
	AcmeAccounts       []AcmeAccountView       `json:"acmeAccounts"`
	DNSAccounts        []AcmeDNSAccountView    `json:"dnsAccounts"`
	Certificates       []AcmeCertificateView   `json:"certificates"`
	Error              string                  `json:"error,omitempty"`
}

type AcmeAutoRenewWindowInfo struct {
	WindowDays          int   `json:"windowDays"`
	DynamicByValidity   bool  `json:"dynamicByValidity"`
	ThresholdDays       int   `json:"thresholdDays"`
	MinDynamicWindowDay int   `json:"minDynamicWindowDay"`
	Examples            []int `json:"examples"`
}

type AcmeCAOption struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type AcmeDNSProviderMeta struct {
	Name         string            `json:"name"`
	ProviderCode string            `json:"providerCode"`
	Helper       string            `json:"helper"`
	Fields       []AcmeDNSFieldDef `json:"fields"`
}

type AcmeDNSFieldDef struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder"`
}

type AcmeAccountView struct {
	Id        uint   `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Server    string `json:"server"`
	KeyLength string `json:"keyLength"`
	Remark    string `json:"remark"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

type AcmeDNSAccountView struct {
	Id           uint              `json:"id"`
	Name         string            `json:"name"`
	ProviderName string            `json:"providerName"`
	ProviderCode string            `json:"providerCode"`
	Env          map[string]string `json:"env"`
	Remark       string            `json:"remark"`
	CreatedAt    int64             `json:"createdAt"`
	UpdatedAt    int64             `json:"updatedAt"`
}

type AcmeCertificateView = CertificateRecordView

type AcmeIssuePayload struct {
	DomainsText     string
	CertificateType string
	Challenge       string
	Webroot         string
	DNSProvider     string
	DNSEnvText      string
	Server          string
	KeyLength       string
	CustomArgs      string

	AcmeAccountID uint
	DNSAccountID  uint
	AutoRenew     bool
	Remark        string

	ApplyTarget  string
	PushDir      string
	PushExplicit bool
	LogSessionID string
}

type AcmeRenewPayload struct {
	ID          uint
	Force       bool
	ApplyTarget string
}

type AcmePushPayload struct {
	ID        uint
	TargetDir string
}

type AcmeSetAutoRenewPayload struct {
	ID        uint
	AutoRenew bool
}

type AcmeApplyPayload struct {
	ID     uint
	Target string
}

type AcmeUnapplyPayload struct {
	ID     uint
	Target string
}

type AcmeDeletePayload struct {
	ID uint
}

type AcmeInstallPayload struct {
	Email         string
	EmailProvided bool
	Version       string
}

type AcmeRemovePayload struct {
	RemoveCertificates bool
}

type AcmeAccountPayload struct {
	ID        uint
	Name      string
	Email     string
	Server    string
	KeyLength string
	Remark    string
}

type AcmeDNSAccountPayload struct {
	ID           uint
	Name         string
	ProviderCode string
	EnvJSON      string
	Remark       string
}

type AcmeActionResult struct {
	Overview    *AcmeOverview        `json:"overview,omitempty"`
	Certificate *AcmeCertificateView `json:"certificate,omitempty"`
	Msg         string               `json:"msg,omitempty"`
	Output      string               `json:"output,omitempty"`
}

type AcmeVersionItem struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
	Source      string `json:"source,omitempty"`
}

type AcmeVersionListResult struct {
	Versions []AcmeVersionItem `json:"versions"`
	Page     int               `json:"page"`
	PerPage  int               `json:"per_page"`
	HasMore  bool              `json:"has_more"`
}

type AcmeVersionCheckResult struct {
	Supported      bool   `json:"supported"`
	Installed      bool   `json:"installed"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	Message        string `json:"message"`
}

type acmeGitHubTag struct {
	Name string `json:"name"`
}

type acmeRemoveOptions struct {
	removeCertificates bool
	removeRuntimeData  bool
}

type acmeLegacyDNSCandidate struct {
	provider AcmeDNSProviderMeta
	env      map[string]string
}

type AcmeLogSessionView struct {
	Id         string   `json:"id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Lines      []string `json:"lines"`
	Error      string   `json:"error,omitempty"`
	StartedAt  int64    `json:"startedAt"`
	UpdatedAt  int64    `json:"updatedAt"`
	FinishedAt int64    `json:"finishedAt,omitempty"`
}

type AcmeIPPortStatus struct {
	Supported bool             `json:"supported"`
	CheckedAt int64            `json:"checkedAt"`
	Ports     []AcmeIPPortItem `json:"ports"`
}

type AcmeIPPortItem struct {
	Challenge   string `json:"challenge"`
	Port        int    `json:"port"`
	Occupied    bool   `json:"occupied"`
	Available   bool   `json:"available"`
	TCPOccupied bool   `json:"tcpOccupied"`
	UDPOccupied bool   `json:"udpOccupied"`
	Recommended bool   `json:"recommended"`
	Reason      string `json:"reason"`
	Message     string `json:"message"`
}

type acmeChallengePortDecision struct {
	InputChallenge string
	Challenge      string
	Port           int
	TCPOccupied    bool
	UDPOccupied    bool
	Available      bool
	Recommended    bool
	Switched       bool
	Reason         string
}

type acmeChallengePortSnapshot struct {
	Supported bool
	CheckedAt int64
	ByPort    map[int]SinglePortStatus
}

func (s *AcmeService) GetOverview() (*AcmeOverview, error) {
	if err := s.EnsureOverviewRuntimeConsistency(false); err != nil {
		return nil, err
	}

	overview := &AcmeOverview{
		Supported: runtime.GOOS == "linux",
	}

	overview.ContactEmail = normalizeAcmeEmail(s.readSettingWithDefault(acmeContactEmailKey, ""))
	overview.PreferredCA = normalizeSupportedAcmeDomainServer(s.readSettingWithDefault(acmePreferredCAKey, defaultAcmePreferredCA))
	if overview.PreferredCA == "" {
		overview.PreferredCA = defaultAcmePreferredCA
	}
	overview.DefaultChallenge = normalizeAcmeChallenge(s.readSettingWithDefault(acmeDefaultChallengeKey, defaultAcmeChallenge))
	overview.DefaultWebroot = strings.TrimSpace(s.readSettingWithDefault(acmeDefaultWebrootKey, ""))
	overview.DefaultDNSProvider = strings.TrimSpace(s.readSettingWithDefault(acmeDefaultDNSProviderKey, ""))
	overview.DefaultKeyLength = normalizeAcmeKeyLength(s.readSettingWithDefault(acmeDefaultKeyLengthKey, defaultAcmeKeyLength))
	overview.AutoRenewWindow = AcmeAutoRenewWindowInfo{
		WindowDays:          defaultAcmeAutoRenewDays,
		DynamicByValidity:   true,
		ThresholdDays:       40,
		MinDynamicWindowDay: 1,
		Examples:            []int{30, 14, 2},
	}
	overview.AutoUpgrade = strings.EqualFold(strings.TrimSpace(s.readSettingWithDefault(acmeAutoUpgradeKey, "true")), "true")
	overview.CAOptions = append([]AcmeCAOption(nil), defaultAcmeCAOptions...)
	overview.DNSProviders = append([]AcmeDNSProviderMeta(nil), defaultAcmeDNSProviderCatalog...)

	if !overview.Supported {
		overview.Error = "ACME certificate management is only supported on Linux"
	}

	scriptPath, homeDir, installed := s.resolveAcmeScript()
	overview.ScriptPath = scriptPath
	overview.HomeDir = homeDir
	overview.Installed = installed

	if installed {
		version, err := readAcmeVersionByScript(scriptPath, homeDir)
		if err != nil {
			overview.Error = strings.TrimSpace(err.Error())
		} else {
			overview.Version = version
		}
	}

	certs, err := certificateInventory.List()
	if err != nil {
		return nil, err
	}
	overview.Certificates = certs

	acmeAccounts, err := s.listAcmeAccounts()
	if err != nil {
		return nil, err
	}
	overview.AcmeAccounts = acmeAccounts

	dnsAccounts, err := s.listDNSAccounts()
	if err != nil {
		return nil, err
	}
	overview.DNSAccounts = dnsAccounts

	return overview, nil
}

func (s *AcmeService) EnsureOverviewRuntimeConsistency(force bool) error {
	now := time.Now().Unix()
	dbKey := currentAcmeMaintenanceDBKey()
	if !force {
		last := acmeMaintenanceAt.Load()
		if last > 0 && now-last < 8 && acmeMaintenanceDBKeyMatches(dbKey) {
			return nil
		}
	}

	acmeMaintenanceMu.Lock()
	defer acmeMaintenanceMu.Unlock()

	if !force {
		last := acmeMaintenanceAt.Load()
		if last > 0 && now-last < 8 && acmeMaintenanceDBKeyMatches(dbKey) {
			return nil
		}
	}

	if err := s.removeLegacyDefaultPushSetting(); err != nil {
		return err
	}
	if err := cleanupLegacyCertificateManagedDirs(); err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		if err := s.migrateLegacyDNSSecretsFromAccountConf(); err != nil {
			return err
		}
	}
	if err := s.cleanupNonDNSCertificateDNSReferences(); err != nil {
		return err
	}
	if err := s.syncInventoryFromAcmeDB(); err != nil {
		return err
	}

	acmeMaintenanceAt.Store(time.Now().Unix())
	acmeMaintenanceDB.Store(dbKey)
	return nil
}

func (s *AcmeService) removeLegacyDefaultPushSetting() error {
	return database.GetDB().Where("key = ?", "acmeDefaultPushDir").Delete(&model.Setting{}).Error
}

func currentAcmeMaintenanceDBKey() string {
	db := database.GetDB()
	if db == nil {
		return ""
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		return ""
	}
	return fmt.Sprintf("%p", sqlDB)
}

func acmeMaintenanceDBKeyMatches(dbKey string) bool {
	value := acmeMaintenanceDB.Load()
	lastDBKey, _ := value.(string)
	return lastDBKey == dbKey
}

func (s *AcmeService) GetLogSession(id string) (*AcmeLogSessionView, error) {
	return acmeLogSessionStore.get(id), nil
}

func (s *AcmeService) GetIPCertificatePortStatus() (*AcmeIPPortStatus, error) {
	snapshot, err := collectAcmeChallengePortSnapshot()
	if err != nil {
		return nil, err
	}

	status := &AcmeIPPortStatus{
		Supported: snapshot.Supported,
		CheckedAt: snapshot.CheckedAt,
		Ports:     []AcmeIPPortItem{},
	}
	recommended := selectRecommendedAcmePortChallenge(acmeCertificateTypeDomain, snapshot)
	for _, challenge := range acmePortChallengesForType(acmeCertificateTypeDomain) {
		port, ok := acmePortForChallenge(challenge)
		if !ok {
			continue
		}
		tcpOccupied, udpOccupied := acmeChallengePortOccupied(snapshot, port)
		reason := acmePortStatusReasonForChallenge(challenge, tcpOccupied)
		item := buildAcmeIPPortItem(
			challenge,
			port,
			tcpOccupied,
			udpOccupied,
			recommended.Available && recommended.Challenge == challenge,
			reason,
		)
		status.Ports = append(status.Ports, item)
	}
	return status, nil
}

func (s *AcmeService) MigrateLegacyDNSSecretsOnStartup() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	return s.migrateLegacyDNSSecretsFromAccountConf()
}

func (s *AcmeService) GetRemoteVersionsPage(page int, perPage int) (*AcmeVersionListResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if runtime.GOOS != "linux" {
		return &AcmeVersionListResult{
			Versions: []AcmeVersionItem{},
			Page:     1,
			PerPage:  5,
			HasMore:  false,
		}, nil
	}

	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 5
	}
	if perPage > 20 {
		perPage = 20
	}

	client := &http.Client{Timeout: 20 * time.Second}
	releases, releaseHasMore, releaseErr := s.fetchAcmeReleasePage(client, page, perPage)
	if releaseErr == nil && len(releases) > 0 {
		return &AcmeVersionListResult{
			Versions: releases,
			Page:     page,
			PerPage:  perPage,
			HasMore:  releaseHasMore,
		}, nil
	}

	tags, tagHasMore, tagErr := s.fetchAcmeTagPage(client, page, perPage)
	if tagErr != nil {
		if releaseErr != nil {
			return nil, common.NewError("failed to fetch acme versions: ", releaseErr, "; fallback tags failed: ", tagErr)
		}
		return nil, common.NewError("failed to fetch acme versions from tags: ", tagErr)
	}
	return &AcmeVersionListResult{
		Versions: tags,
		Page:     page,
		PerPage:  perPage,
		HasMore:  tagHasMore,
	}, nil
}

func (s *AcmeService) CheckUpdate() (*AcmeVersionCheckResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	result := &AcmeVersionCheckResult{
		Supported: runtime.GOOS == "linux",
		Installed: false,
	}
	if !result.Supported {
		result.Message = "ACME certificate management is only supported on Linux"
		return result, nil
	}

	scriptPath, homeDir, installed := s.resolveAcmeScript()
	result.Installed = installed
	if installed {
		current, err := readAcmeVersionByScript(scriptPath, homeDir)
		if err == nil {
			result.CurrentVersion = current
		}
	}

	latest, err := s.fetchAcmeLatestVersion()
	if err != nil {
		result.Message = strings.TrimSpace(err.Error())
		return result, nil
	}

	result.LatestVersion = latest
	if result.CurrentVersion == "" {
		result.HasUpdate = false
		if result.Installed {
			result.Message = "已安装 acme.sh，但未能识别当前版本"
		} else {
			result.Message = "acme.sh 尚未安装"
		}
		return result, nil
	}

	switch compareSemverLikeTags(result.CurrentVersion, result.LatestVersion) {
	case -1:
		result.HasUpdate = true
		result.Message = fmt.Sprintf("发现新版本：%s -> %s", result.CurrentVersion, result.LatestVersion)
	case 0:
		result.HasUpdate = false
		result.Message = fmt.Sprintf("当前已是最新版本：%s", result.CurrentVersion)
	default:
		result.HasUpdate = false
		result.Message = fmt.Sprintf("当前版本 %s 高于远端版本 %s", result.CurrentVersion, result.LatestVersion)
	}

	return result, nil
}

func (s *AcmeService) Install(email string) (*AcmeActionResult, error) {
	return s.InstallOrReinstall(AcmeInstallPayload{
		Email:         email,
		EmailProvided: true,
	})
}

func (s *AcmeService) InstallOrReinstall(payload AcmeInstallPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if runtime.GOOS != "linux" {
		return nil, common.NewError("ACME certificate management is only supported on Linux")
	}

	contact := normalizeAcmeEmail(payload.Email)
	version := normalizeAcmeVersionTag(payload.Version)
	if !payload.EmailProvided {
		contact = normalizeAcmeEmail(s.readSettingWithDefault(acmeContactEmailKey, ""))
	}
	if contact != "" {
		validContact, validErr := validateAcmeEmail(contact)
		if validErr != nil {
			return nil, validErr
		}
		contact = validContact
	}

	if version != "" {
		ok, checkErr := s.checkVersionDownloadableLocked(version)
		if checkErr != nil {
			return nil, checkErr
		}
		if !ok {
			return nil, common.NewError("selected acme.sh version is unavailable: ", version)
		}
	}

	beforeVersion := ""
	if scriptPath, homeDir, installed := s.resolveAcmeScript(); installed {
		if v, err := readAcmeVersionByScript(scriptPath, homeDir); err == nil {
			beforeVersion = v
		}
	}

	tmpFile, err := os.CreateTemp("", "acme-install-*.sh")
	if err != nil {
		return nil, common.NewError("create acme installer temp file failed: ", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := downloadAcmeInstallerScript(tmpPath); err != nil {
		return nil, err
	}

	shellPath, err := resolveManagedScriptShell()
	if err != nil {
		return nil, err
	}

	if err := cleanupStaleManagedAcmeInstallWorkspaces(managedAcmeWorkspaceParentDir()); err != nil {
		return nil, err
	}

	stagedHomeDir, cleanupStagedHomeDir, err := createManagedAcmeInstallWorkspace(acmeManagedWorkspaceStagePrefix)
	if err != nil {
		return nil, err
	}
	defer cleanupStagedHomeDir()

	args := []string{
		tmpPath,
		"--install-online",
		"--nocron",
		"--noprofile",
		"--home", stagedHomeDir,
	}
	if version != "" {
		args = append(args, "--branch", version)
	}
	if contact != "" {
		args = append(args, "--accountemail", contact)
	}
	output, err := runCommandOutputWithTimeoutEnv(90*time.Second, shellPath, args, nil)
	if err != nil {
		return nil, common.NewError("install acme.sh failed: ", err)
	}

	stagedScriptPath := filepath.Clean(filepath.Join(stagedHomeDir, "acme.sh"))
	if !pathExists(stagedScriptPath) {
		detail := summarizeAcmeInstallOutput(output)
		if detail != "" {
			return nil, common.NewError("acme.sh install finished but staged script path was not found: ", detail)
		}
		return nil, common.NewError("acme.sh install finished but staged script path was not found")
	}
	if _, err := readAcmeVersionByScript(stagedScriptPath, stagedHomeDir); err != nil {
		detail := summarizeAcmeInstallOutput(output)
		if detail != "" {
			return nil, common.NewError("staged acme.sh install is incomplete: ", detail)
		}
		return nil, common.NewError("staged acme.sh install is incomplete: ", err)
	}

	scriptPath, err := s.activateManagedAcmeInstallLocked(stagedHomeDir)
	if err != nil {
		return nil, err
	}

	if err := s.persistManagedAcmeManifestLocked(managedAcmeHomeDir()); err != nil {
		return nil, err
	}

	if err := s.setString(acmeScriptPathKey, scriptPath); err != nil {
		return nil, err
	}
	if payload.EmailProvided {
		if err := s.setString(acmeContactEmailKey, contact); err != nil {
			return nil, err
		}
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	newVersion := strings.TrimSpace(overview.Version)
	var msg string
	if beforeVersion == "" {
		if newVersion != "" {
			msg = fmt.Sprintf("acme.sh installed, current version: %s", newVersion)
		} else {
			msg = "acme.sh 已安装"
		}
	} else {
		displayNew := newVersion
		if displayNew == "" {
			displayNew = "未知版本"
		}
		msg = fmt.Sprintf("acme.sh 已重装：%s -> %s", beforeVersion, displayNew)
	}
	return &AcmeActionResult{
		Overview: overview,
		Msg:      msg,
		Output:   strings.TrimSpace(output),
	}, nil
}

func (s *AcmeService) Upgrade() (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if runtime.GOOS != "linux" {
		return nil, common.NewError("ACME certificate management is only supported on Linux")
	}

	scriptPath, homeDir, installed := s.resolveAcmeScript()
	if !installed {
		return nil, common.NewError("acme.sh is not installed")
	}

	autoUpgrade := strings.EqualFold(strings.TrimSpace(s.readSettingWithDefault(acmeAutoUpgradeKey, "true")), "true")
	envPairs := []string{}
	if !autoUpgrade {
		envPairs = append(envPairs, "AUTO_UPGRADE=0")
	}

	output, err := runCommandOutputWithTimeoutEnv(90*time.Second, scriptPath, append(acmeHomeArgs(homeDir), "--upgrade"), envPairs)
	if err != nil {
		return nil, common.NewError("upgrade acme.sh failed: ", err)
	}

	if err := s.setString(acmeScriptPathKey, scriptPath); err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{
		Overview: overview,
		Output:   strings.TrimSpace(output),
	}, nil
}

func (s *AcmeService) RemoveManagedAcme(payload AcmeRemovePayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if runtime.GOOS != "linux" {
		return nil, common.NewError("ACME certificate management is only supported on Linux")
	}
	return s.removeManagedAcmeWithOptionsLocked(acmeRemoveOptions{
		removeCertificates: payload.RemoveCertificates,
		removeRuntimeData:  false,
	})
}

func (s *AcmeService) RemoveManagedAcmeForUninstall() (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	return s.removeManagedAcmeWithOptionsLocked(acmeRemoveOptions{
		removeCertificates: true,
		removeRuntimeData:  true,
	})
}

func (s *AcmeService) Issue(payload AcmeIssuePayload) (*AcmeActionResult, error) {
	logSession := acmeLogSessionStore.start(payload.LogSessionID, "证书签发")
	logSession.append("进入 ACME 签发队列")
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()
	logSession.append("开始执行 ACME 签发")

	if runtime.GOOS != "linux" {
		logSession.fail("ACME certificate management is only supported on Linux")
		return nil, common.NewError("ACME certificate management is only supported on Linux")
	}

	scriptPath, homeDir, installed := s.resolveAcmeScript()
	if !installed {
		logSession.fail("acme.sh is not installed")
		return nil, common.NewError("acme.sh is not installed")
	}
	logSession.append("已找到 acme.sh: " + scriptPath)

	certificateType := normalizeAcmeCertificateType(payload.CertificateType)
	if certificateType == acmeCertificateTypeIP {
		logSession.append("证书类型: IP 证书")
	} else {
		logSession.append("证书类型: 域名证书")
		if payload.AcmeAccountID == 0 {
			logSession.fail("域名证书签发必须选择 ACME 账号")
			return nil, common.NewError("域名证书签发必须选择 ACME 账号")
		}
	}

	domains := normalizeAcmeIssueIdentifiers(payload.DomainsText, certificateType)
	if len(domains) == 0 {
		message := "domain list is required"
		if certificateType == acmeCertificateTypeIP {
			message = "IP list is required"
		}
		logSession.fail(message)
		return nil, common.NewError(message)
	}
	if certificateType == acmeCertificateTypeIP && len(domains) > acmeIPCertificateMaxIPs {
		logSession.fail("IP 证书最多支持 100 个 IP")
		return nil, common.NewError("IP 证书最多支持 100 个 IP")
	}
	if certificateType == acmeCertificateTypeIP {
		logSession.append("准备签发 IP: " + strings.Join(domains, ", "))
	} else {
		logSession.append("准备签发域名: " + strings.Join(domains, ", "))
	}

	challenge := normalizeAcmeChallenge(payload.Challenge)
	if challenge == "" {
		challenge = normalizeAcmeChallenge(s.readSettingWithDefault(acmeDefaultChallengeKey, defaultAcmeChallenge))
	}
	if certificateType == acmeCertificateTypeIP {
		challenge = normalizeAcmeIPChallenge(challenge)
		if challenge == "" {
			logSession.fail("IP 证书只能使用 HTTP Standalone 或 TLS ALPN 验证")
			return nil, common.NewError("IP 证书只能使用 HTTP Standalone 或 TLS ALPN 验证")
		}
	}
	logSession.append("验证方式: " + challenge)
	ipFamilyMode := detectAcmeIPFamilyMode(domains)
	if certificateType == acmeCertificateTypeIP {
		logSession.append("IP 地址族: " + acmeIPFamilyModeLabel(ipFamilyMode))
		logSession.append("IP 证书校验走 HTTP/TLS 到字面 IP，不走 DNS-01")
		logSession.append("端口空闲只代表本机未占用，不代表外部 IPv6 一定可达")
	}
	keyLength := normalizeAcmeKeyLength(payload.KeyLength)
	if keyLength == "" {
		keyLength = normalizeAcmeKeyLength(s.readSettingWithDefault(acmeDefaultKeyLengthKey, defaultAcmeKeyLength))
	}
	if keyLength == "" {
		keyLength = defaultAcmeKeyLength
	}
	useECC := strings.HasPrefix(strings.ToLower(keyLength), "ec-")
	logSession.append("证书算法: " + keyLength)

	caServer := ""
	if certificateType == acmeCertificateTypeDomain {
		serverInput := strings.TrimSpace(payload.Server)
		if serverInput != "" && !isSupportedAcmeDomainServer(serverInput) {
			logSession.fail("域名证书仅支持 Let's Encrypt 或 ZeroSSL")
			return nil, common.NewError("域名证书仅支持 Let's Encrypt 或 ZeroSSL")
		}
		caServer = normalizeSupportedAcmeDomainServer(serverInput)
		if caServer == "" {
			caServer = normalizeSupportedAcmeDomainServer(s.readSettingWithDefault(acmePreferredCAKey, defaultAcmePreferredCA))
		}
		if caServer == "" {
			caServer = defaultAcmePreferredCA
		}
	} else {
		caServer = acmeLEProductionDirectory
	}
	logSession.append("CA 平台: " + caServer)

	webroot := strings.TrimSpace(payload.Webroot)
	if webroot == "" {
		webroot = strings.TrimSpace(s.readSettingWithDefault(acmeDefaultWebrootKey, ""))
	}
	useDNSChallenge := shouldUseAcmeDNSChallenge(certificateType, challenge)
	var challengeDecision acmeChallengePortDecision
	if !useDNSChallenge && isAcmePortChallenge(challenge) {
		snapshot, snapshotErr := collectAcmeChallengePortSnapshot()
		if snapshotErr != nil {
			logSession.fail(snapshotErr.Error())
			return nil, snapshotErr
		}
		decision, decisionErr := selectAcmeChallengePortDecision(certificateType, challenge, snapshot)
		if decisionErr != nil {
			logSession.fail(decisionErr.Error())
			return nil, decisionErr
		}
		challengeDecision = decision
		challenge = decision.Challenge
		if challengeDecision.Switched {
			logSession.append(fmt.Sprintf(
				"port challenge switched: %s -> %s (%s)",
				challengeDecision.InputChallenge,
				challengeDecision.Challenge,
				challengeDecision.Reason,
			))
		} else {
			logSession.append(fmt.Sprintf(
				"port challenge selected: %s (%s)",
				challengeDecision.Challenge,
				challengeDecision.Reason,
			))
		}
	}
	useDNSChallenge = shouldUseAcmeDNSChallenge(certificateType, challenge)
	dnsProvider := strings.TrimSpace(payload.DNSProvider)
	if useDNSChallenge && dnsProvider == "" {
		dnsProvider = strings.TrimSpace(s.readSettingWithDefault(acmeDefaultDNSProviderKey, ""))
	}
	if !useDNSChallenge {
		dnsProvider = ""
	}
	dnsEnvText := strings.TrimSpace(payload.DNSEnvText)
	customArgs := strings.TrimSpace(payload.CustomArgs)
	acmeAccountName := ""
	dnsAccountName := ""
	acmeAccountEmail := ""
	if certificateType == acmeCertificateTypeDomain && payload.AcmeAccountID > 0 {
		account := &model.AcmeAccount{}
		if err := database.GetDB().Where("id = ?", payload.AcmeAccountID).First(account).Error; err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
		acmeAccountName = strings.TrimSpace(account.Name)
		if acmeAccountName != "" {
			logSession.append("使用 ACME 账号: " + acmeAccountName)
		}
		if strings.TrimSpace(account.Server) != "" {
			accountServer := normalizeSupportedAcmeDomainServer(account.Server)
			if accountServer == "" {
				logSession.fail("所选 ACME 账号的 CA 平台无效，仅支持 Let's Encrypt 或 ZeroSSL")
				return nil, common.NewError("所选 ACME 账号的 CA 平台无效，仅支持 Let's Encrypt 或 ZeroSSL")
			}
			caServer = accountServer
		}
		if strings.TrimSpace(account.KeyLength) != "" {
			keyLength = normalizeAcmeKeyLength(account.KeyLength)
			if keyLength == "" {
				keyLength = defaultAcmeKeyLength
			}
			useECC = strings.HasPrefix(strings.ToLower(keyLength), "ec-")
		}
		acmeAccountEmail = normalizeAcmeEmail(strings.TrimSpace(account.Email))
		if acmeAccountEmail == "" {
			message := "所选 ACME 账号未配置邮箱，请到“ACME 账号管理”补全后重试"
			logSession.fail(message)
			return nil, common.NewError(message)
		}
		logSession.append("使用 ACME 账号邮箱: " + acmeAccountEmail)
		if err := s.ensureAcmeAccountEmailForServer(scriptPath, homeDir, acmeAccountEmail, caServer, logSession); err != nil {
			if isAcmeInvalidContactError(err) {
				message := "ACME 账号邮箱格式无效，请到“ACME 账号管理”修正邮箱后重试"
				logSession.fail(message + ": " + err.Error())
				return nil, common.NewError(message)
			}
			message := "同步 ACME 账号邮箱失败，请检查账号邮箱或 CA 账号状态后重试"
			logSession.fail(message + ": " + err.Error())
			return nil, common.NewError(message)
		}
	}
	if certificateType == acmeCertificateTypeIP {
		if payload.AcmeAccountID > 0 {
			logSession.append("IP 证书流程忽略 ACME 账号参数")
		}
		caServer = acmeLEProductionDirectory
		logSession.append("IP 证书强制使用 Let's Encrypt shortlived profile")
		contactEmail := normalizeAcmeEmail(s.readSettingWithDefault(acmeContactEmailKey, ""))
		if contactEmail == "" {
			logSession.append("未设置联系邮箱，跳过账号邮箱同步")
		} else {
			logSession.append("使用联系邮箱同步 IP 证书账号: " + contactEmail)
			if err := s.ensureAcmeAccountEmailForServer(scriptPath, homeDir, contactEmail, caServer, logSession); err != nil {
				if isAcmeInvalidContactError(err) {
					message := "联系邮箱格式无效，请在 acme.sh 运行时修正后重试"
					logSession.fail(message + ": " + err.Error())
					return nil, common.NewError(message)
				}
				message := "IP 证书通道联系邮箱同步失败，请检查 CA 账号状态后重试"
				logSession.fail(message + ": " + err.Error())
				return nil, common.NewError(message)
			}
		}
	} else {
		logSession.append("最终 CA 平台: " + caServer)
		logSession.append("最终证书算法: " + keyLength)
	}

	dnsEnvFromAccount := []string{}
	if useDNSChallenge && payload.DNSAccountID > 0 {
		dnsAccount := &model.AcmeDNSAccount{}
		if err := database.GetDB().Where("id = ?", payload.DNSAccountID).First(dnsAccount).Error; err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
		dnsAccountName = strings.TrimSpace(dnsAccount.Name)
		if dnsAccountName != "" {
			logSession.append("使用 DNS 账号: " + dnsAccountName)
		}
		if strings.TrimSpace(dnsAccount.ProviderCode) != "" {
			dnsProvider = strings.TrimSpace(dnsAccount.ProviderCode)
		}
		envMap, err := parseAcmeEnvJSON(dnsAccount.EnvJSON)
		if err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
		dnsEnvFromAccount = envMapToEnvPairs(envMap)
	}
	if challenge == "webroot" && strings.TrimSpace(webroot) == "" {
		logSession.fail("webroot challenge requires webroot path")
		return nil, common.NewError("webroot challenge requires webroot path")
	}
	if challenge == "dns" && strings.TrimSpace(dnsProvider) == "" {
		logSession.fail("dns challenge requires dns provider (for example dns_cf)")
		return nil, common.NewError("dns challenge requires dns provider (for example dns_cf)")
	}
	if challenge == "dns" {
		logSession.append("DNS Provider: " + dnsProvider)
		logSession.append("开始 DNS 验证流程，acme.sh 将添加并等待 TXT 记录生效")
	}
	var tempFirewall *acmeTemporaryFirewallRule
	if !useDNSChallenge && isAcmePortChallenge(challenge) && challengeDecision.Port > 0 {
		prepared, err := s.prepareTemporaryAcmeFirewallRule(challengeDecision.Port, logSession)
		if err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
		tempFirewall = prepared
		if tempFirewall != nil {
			defer s.cleanupTemporaryAcmeFirewallRule(tempFirewall, logSession)
		}
	}

	if certificateType == acmeCertificateTypeIP {
		logAcmeIPFamilyListenStrategy(logSession, ipFamilyMode)
	}
	defer cleanupAcmeWorkingTree(homeDir, domains[0], useECC)
	commandArgs := buildAcmeIssueCommandArgs(domains, challenge, webroot, dnsProvider, keyLength, caServer, customArgs, certificateType == acmeCertificateTypeIP, ipFamilyMode)
	dnsEnv := []string{}
	if useDNSChallenge {
		parsedEnv, parseErr := normalizeAcmeEnvAssignments(dnsEnvText)
		if parseErr != nil {
			logSession.fail(parseErr.Error())
			return nil, parseErr
		}
		dnsEnv = parsedEnv
		if len(dnsEnvFromAccount) > 0 {
			dnsEnv = mergeEnvPairs(dnsEnvFromAccount, dnsEnv)
		}
		if len(dnsEnv) > 0 {
			defer s.cleanupAcmeAccountConfSecrets(homeDir, dnsEnv, logSession)
		}
	}

	logSession.append("执行 acme.sh --issue")
	output, err := runCommandOutputWithTimeoutEnvLog(3*time.Minute, scriptPath, append(acmeHomeArgs(homeDir), commandArgs...), dnsEnv, logSession)
	skippedBecauseDomainsUnchanged := false
	if err != nil {
		if isAcmeDomainsNotChangedError(err) {
			skippedBecauseDomainsUnchanged = true
			output = strings.TrimSpace(err.Error())
			logSession.append("检测到域名未变化，复用已有证书并同步到托管目录")
		} else {
			logSession.fail(err.Error())
			return nil, common.NewError("issue certificate failed: ", err)
		}
	}

	if skippedBecauseDomainsUnchanged {
		logSession.append("未触发重新签发，开始安装已有证书文件")
	} else {
		logSession.append("签发成功，开始安装证书文件")
	}
	paths, cleanupInstalledCert, err := s.installCertToManagedDir(scriptPath, homeDir, domains[0], useECC, dnsEnv, logSession)
	if err != nil {
		logSession.fail(err.Error())
		return nil, err
	}
	defer cleanupInstalledCert()

	certEntry, err := s.upsertCertificateFromPaths(0, domains, certificateType, acmeCertProfileForType(certificateType), challenge, keyLength, caServer, useECC, homeDir, webroot, dnsProvider, dnsEnvText, customArgs, paths, time.Now().Unix())
	if err != nil {
		logSession.fail(err.Error())
		return nil, err
	}
	applyAcmeAccountBinding(certEntry, certificateType, payload.AcmeAccountID, acmeAccountName)
	if useDNSChallenge {
		certEntry.DNSAccountID = payload.DNSAccountID
		certEntry.DNSAccountName = dnsAccountName
	} else {
		certEntry.DNSAccountID = 0
		certEntry.DNSAccountName = ""
	}
	certEntry.AutoRenew = payload.AutoRenew
	certEntry.PushDir = strings.TrimSpace(payload.PushDir)
	certEntry.Remark = strings.TrimSpace(payload.Remark)
	certEntry.Webroot = webroot
	certEntry.DNSProvider = dnsProvider
	certEntry.DNSEnvText = dnsEnvText
	certEntry.CustomArgs = customArgs
	certEntry.LastOutput = strings.TrimSpace(output)
	if normalizedApplyTarget, ok := normalizeAcmeApplyTarget(payload.ApplyTarget); ok {
		certEntry.ApplyTarget = string(normalizedApplyTarget)
	} else {
		certEntry.ApplyTarget = ""
	}
	if err := database.GetDB().Save(certEntry).Error; err != nil {
		logSession.fail(err.Error())
		return nil, err
	}
	if _, upsertErr := upsertInventoryFromAcme(certEntry); upsertErr != nil {
		logSession.fail(upsertErr.Error())
		return nil, upsertErr
	}

	if certificateType != acmeCertificateTypeIP {
		if err := s.persistAcmeDefaults(payload, challenge, keyLength, caServer, dnsProvider, useDNSChallenge); err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
	}

	logSession.append("执行签发后动作")
	if err := s.applyIssuePostActions(certEntry, payload.ApplyTarget, payload.PushDir, payload.PushExplicit); err != nil {
		logSession.fail(err.Error())
		return nil, err
	}
	if _, upsertErr := upsertInventoryFromAcme(certEntry); upsertErr != nil {
		logSession.fail(upsertErr.Error())
		return nil, upsertErr
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		logSession.fail(err.Error())
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		logSession.fail(err.Error())
		return nil, err
	}
	view := convertAcmeCertificate(certEntry)
	if skippedBecauseDomainsUnchanged {
		logSession.finish("域名未变化，已同步已有证书")
	} else {
		logSession.finish("证书签发完成")
	}
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
		Output:      strings.TrimSpace(output),
	}, nil
}

func (s *AcmeService) Renew(payload AcmeRenewPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if payload.ID == 0 {
		return nil, common.NewError("certificate id is required")
	}
	row, getErr := certificateInventory.GetRecordByID(payload.ID)
	if getErr != nil {
		return nil, getErr
	}
	if row.SourceType != CertificateSourceACME {
		return s.renewInventorySelfSignedCertificate(row)
	}
	if runtime.GOOS != "linux" {
		return nil, common.NewError("ACME certificate management is only supported on Linux")
	}

	acmeID := payload.ID
	if row.SourceRef != "" {
		if parsed, parseErr := strconv.ParseUint(strings.TrimSpace(row.SourceRef), 10, 64); parseErr == nil {
			acmeID = uint(parsed)
		}
	}

	entry, err := s.findCertificateByID(acmeID)
	if err != nil {
		return nil, err
	}

	scriptPath, homeDir, installed := s.resolveAcmeScript()
	if !installed {
		return nil, common.NewError("acme.sh is not installed")
	}
	if strings.TrimSpace(entry.AcmeHome) != "" {
		homeDir = strings.TrimSpace(entry.AcmeHome)
	}

	certificateType := acmeCertificateTypeForEntry(entry)
	isIPCert := isAcmeIPCertificate(entry)
	domains := decodeCertificateDomains(entry.DomainSet)
	if len(domains) == 0 && strings.TrimSpace(entry.MainDomain) != "" {
		domains = []string{strings.TrimSpace(entry.MainDomain)}
	}
	if len(domains) == 0 {
		return nil, common.NewError("certificate domains are empty")
	}
	challenge := normalizeAcmeChallenge(entry.Challenge)
	if challenge == "" {
		challenge = "standalone"
	}
	if isIPCert {
		challenge = normalizeAcmeIPChallenge(challenge)
		if challenge == "" {
			err := common.NewError("IP certificates only support standalone or alpn challenge")
			_ = s.markCertificateError(entry.Id, err.Error())
			return nil, err
		}
	}
	webroot := strings.TrimSpace(entry.Webroot)
	if webroot == "" {
		webroot = strings.TrimSpace(s.readSettingWithDefault(acmeDefaultWebrootKey, ""))
	}
	keyLength := normalizeAcmeKeyLength(entry.KeyLength)
	if keyLength == "" {
		keyLength = normalizeAcmeKeyLength(s.readSettingWithDefault(acmeDefaultKeyLengthKey, defaultAcmeKeyLength))
	}
	if keyLength == "" {
		keyLength = defaultAcmeKeyLength
	}
	dnsProvider, providerErr := resolveAcmeDNSProviderFromAccount(entry.DNSAccountID, entry.DNSProvider)
	if providerErr != nil {
		return nil, providerErr
	}
	customArgs := strings.TrimSpace(entry.CustomArgs)
	useDNSChallenge := shouldUseAcmeDNSChallenge(certificateType, challenge)
	ipFamilyMode := acmeIPFamilyUnknown
	if isIPCert {
		ipFamilyMode = detectAcmeIPFamilyMode(domains)
	}

	var logSession *acmeLogSession
	logSessionFinished := false
	if !useDNSChallenge && isAcmePortChallenge(challenge) {
		logTitle := "ACME certificate renew"
		if isIPCert {
			logTitle = "IP certificate renew"
		}
		logSession = acmeLogSessionStore.start("", logTitle)
		logSession.append("renew target: " + entry.MainDomain)
		if isIPCert {
			logSession.append("IP family mode: " + acmeIPFamilyModeLabel(ipFamilyMode))
			logSession.append("port appears free locally, but external IPv6 reachability is not guaranteed")
			logAcmeIPFamilyListenStrategy(logSession, ipFamilyMode)
		}
		defer func() {
			if logSession == nil || logSessionFinished {
				return
			}
			logSession.finish("certificate renew flow finished")
		}()
	}

	var challengeDecision acmeChallengePortDecision
	if !useDNSChallenge && isAcmePortChallenge(challenge) {
		snapshot, snapshotErr := collectAcmeChallengePortSnapshot()
		if snapshotErr != nil {
			if logSession != nil {
				logSession.fail(snapshotErr.Error())
				logSessionFinished = true
			}
			_ = s.markCertificateError(entry.Id, snapshotErr.Error())
			return nil, snapshotErr
		}
		decision, decisionErr := selectAcmeChallengePortDecision(certificateType, challenge, snapshot)
		if decisionErr != nil {
			if logSession != nil {
				logSession.fail(decisionErr.Error())
				logSessionFinished = true
			}
			_ = s.markCertificateError(entry.Id, decisionErr.Error())
			return nil, decisionErr
		}
		challengeDecision = decision
		challenge = decision.Challenge
		useDNSChallenge = shouldUseAcmeDNSChallenge(certificateType, challenge)
		if logSession != nil {
			if challengeDecision.Switched {
				logSession.append(fmt.Sprintf("port challenge switched: %s -> %s (%s)", challengeDecision.InputChallenge, challengeDecision.Challenge, challengeDecision.Reason))
			} else {
				logSession.append(fmt.Sprintf("port challenge selected: %s (%s)", challengeDecision.Challenge, challengeDecision.Reason))
			}
		}
	}
	if challenge == "webroot" && strings.TrimSpace(webroot) == "" {
		err := common.NewError("webroot challenge requires webroot path")
		_ = s.markCertificateError(entry.Id, err.Error())
		return nil, err
	}
	if challenge == "dns" && strings.TrimSpace(dnsProvider) == "" {
		err := common.NewError("dns challenge requires dns provider (for example dns_cf)")
		_ = s.markCertificateError(entry.Id, err.Error())
		return nil, err
	}

	renewEnv := []string{}
	if useDNSChallenge {
		parsedEnv, parseErr := resolveAcmeDNSRuntimeEnv(entry.DNSAccountID, entry.DNSEnvText)
		if parseErr != nil {
			_ = s.markCertificateError(entry.Id, parseErr.Error())
			return nil, parseErr
		}
		renewEnv = parsedEnv
		if len(renewEnv) > 0 {
			defer s.cleanupAcmeAccountConfSecrets(homeDir, renewEnv, nil)
		}
	}

	var tempFirewall *acmeTemporaryFirewallRule
	if !useDNSChallenge && isAcmePortChallenge(challenge) && challengeDecision.Port > 0 {
		prepared, prepareErr := s.prepareTemporaryAcmeFirewallRule(challengeDecision.Port, logSession)
		if prepareErr != nil {
			if logSession != nil {
				logSession.fail(prepareErr.Error())
				logSessionFinished = true
			}
			_ = s.markCertificateError(entry.Id, prepareErr.Error())
			return nil, prepareErr
		}
		tempFirewall = prepared
		if tempFirewall != nil {
			defer s.cleanupTemporaryAcmeFirewallRule(tempFirewall, logSession)
		}
	}

	defer cleanupAcmeWorkingTree(homeDir, domains[0], entry.UseECC)
	commandArgs := buildAcmeIssueCommandArgs(domains, challenge, webroot, dnsProvider, keyLength, strings.TrimSpace(entry.CAServer), customArgs, isIPCert, ipFamilyMode)
	if payload.Force {
		commandArgs = append(commandArgs, "--force")
	}
	output, err := runCommandOutputWithTimeoutEnvLog(3*time.Minute, scriptPath, append(acmeHomeArgs(homeDir), commandArgs...), renewEnv, logSession)
	if err != nil {
		if logSession != nil {
			logSession.fail(err.Error())
			logSessionFinished = true
		}
		_ = s.markCertificateError(entry.Id, err.Error())
		return nil, common.NewError("renew certificate failed: ", err)
	}

	paths, cleanupInstalledCert, tempErr := createAcmeTempInstallPaths(entry.MainDomain)
	if tempErr != nil {
		return nil, tempErr
	}
	defer cleanupInstalledCert()
	if err := s.installCertByRecord(scriptPath, homeDir, entry, paths, renewEnv, logSession); err != nil {
		return nil, err
	}

	updated, err := s.upsertCertificateFromPaths(
		entry.Id,
		domains,
		certificateType,
		acmeCertProfileForType(certificateType),
		challenge,
		keyLength,
		strings.TrimSpace(entry.CAServer),
		entry.UseECC,
		homeDir,
		webroot,
		dnsProvider,
		strings.TrimSpace(entry.DNSEnvText),
		customArgs,
		paths,
		time.Now().Unix(),
	)
	if err != nil {
		return nil, err
	}
	updated.LastIssuedAt = entry.LastIssuedAt
	updated.AcmeAccountID = entry.AcmeAccountID
	updated.AcmeAccountName = entry.AcmeAccountName
	if useDNSChallenge {
		updated.DNSAccountID = entry.DNSAccountID
		updated.DNSAccountName = entry.DNSAccountName
	} else {
		updated.DNSAccountID = 0
		updated.DNSAccountName = ""
	}
	updated.AutoRenew = entry.AutoRenew
	updated.PushDir = entry.PushDir
	updated.PushFiles = entry.PushFiles
	updated.Remark = entry.Remark
	updated.ApplyTarget = entry.ApplyTarget
	updated.Webroot = webroot
	updated.DNSProvider = dnsProvider
	updated.DNSEnvText = strings.TrimSpace(entry.DNSEnvText)
	updated.CustomArgs = customArgs
	updated.LastOutput = strings.TrimSpace(output)
	if err := database.GetDB().Save(updated).Error; err != nil {
		return nil, err
	}
	if _, upsertErr := upsertInventoryFromAcme(updated); upsertErr != nil {
		return nil, upsertErr
	}

	applyTarget := strings.TrimSpace(payload.ApplyTarget)
	if err := s.applyIssuePostActions(updated, applyTarget, "", false); err != nil {
		return nil, err
	}
	if _, upsertErr := upsertInventoryFromAcme(updated); upsertErr != nil {
		return nil, upsertErr
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	view := convertAcmeCertificate(updated)
	if logSession != nil {
		logSession.finish("certificate renew completed")
		logSessionFinished = true
	}
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
		Output:      strings.TrimSpace(output),
	}, nil
}
func (s *AcmeService) Push(payload AcmePushPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if payload.ID == 0 {
		return nil, common.NewError("certificate id is required")
	}
	row, getErr := certificateInventory.GetRecordByID(payload.ID)
	if getErr != nil {
		return nil, getErr
	}
	targetDir := strings.TrimSpace(payload.TargetDir)
	if targetDir == "" {
		return nil, common.NewError("target directory is required")
	}

	sourceEntry, err := loadAcmeSourceEntryForInventoryRow(row)
	if err != nil {
		return nil, err
	}

	pushState, err := syncCertificateDirectoryPushState(targetDir, row.PushDir, row.PushFiles, row.CertPEM, row.KeyPEM, row.FullchainPEM, row.ChainPEM)
	if err != nil {
		return nil, err
	}

	row.PushDir = pushState.PushDir
	row.PushFiles = pushState.PushFiles
	if sourceEntry != nil {
		sourceEntry.PushDir = pushState.PushDir
		sourceEntry.PushFiles = pushState.PushFiles
	}
	if err := persistCertificatePushState(row, sourceEntry); err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	view := convertCertificateRecord(row)
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
	}, nil
}

func (s *AcmeService) Apply(payload AcmeApplyPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if payload.ID == 0 {
		return nil, common.NewError("certificate id is required")
	}
	target, ok := normalizeAcmeApplyTarget(payload.Target)
	if !ok {
		return nil, common.NewError("target must be panel or sub")
	}
	row, getErr := certificateInventory.GetRecordByID(payload.ID)
	if getErr != nil {
		return nil, getErr
	}
	if err := s.applyInventoryRecordToTarget(row, target); err != nil {
		return nil, err
	}
	targets, targetErr := assignedTargetsForCertificateRecord(row.Id)
	if targetErr != nil {
		return nil, targetErr
	}
	row.ApplyTarget = formatAssignedApplyTarget(targets)
	if err := database.GetDB().Save(row).Error; err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	view := convertCertificateRecord(row)
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
	}, nil
}

func (s *AcmeService) Unapply(payload AcmeUnapplyPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if payload.ID == 0 {
		return nil, common.NewError("certificate id is required")
	}
	target, ok := normalizeAcmeApplyTarget(payload.Target)
	if !ok {
		return nil, common.NewError("target must be panel or sub")
	}
	row, getErr := certificateInventory.GetRecordByID(payload.ID)
	if getErr != nil {
		return nil, getErr
	}

	changed, err := s.unapplyInventoryRecordFromTarget(row, target)
	if err != nil {
		return nil, err
	}

	targets, targetErr := assignedTargetsForCertificateRecord(row.Id)
	if targetErr != nil {
		return nil, targetErr
	}
	row.ApplyTarget = formatAssignedApplyTarget(targets)
	if err := database.GetDB().Save(row).Error; err != nil {
		return nil, err
	}

	if changed {
		if err := DrainPanelTLSRuntimeConnectionsByFingerprint(target, strings.TrimSpace(row.Fingerprint), PanelTLSUnapplyDrainGracePeriod()); err != nil {
			return nil, err
		}
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	view := convertCertificateRecord(row)
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
	}, nil
}

func (s *AcmeService) SetAutoRenew(payload AcmeSetAutoRenewPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if payload.ID == 0 {
		return nil, common.NewError("certificate id is required")
	}
	row, getErr := certificateInventory.GetRecordByID(payload.ID)
	if getErr != nil {
		return nil, getErr
	}
	if row.SourceType == CertificateSourceSelfSigned {
		row.AutoRenew = payload.AutoRenew
		if err := database.GetDB().Save(row).Error; err != nil {
			return nil, err
		}
		if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
			return nil, err
		}
		overview, err := s.GetOverview()
		if err != nil {
			return nil, err
		}
		view := convertCertificateRecord(row)
		msg := "自动续签已关闭"
		if payload.AutoRenew {
			msg = "自动续签已开启"
		}
		return &AcmeActionResult{
			Overview:    overview,
			Certificate: &view,
			Msg:         msg,
		}, nil
	}
	if row.SourceType != CertificateSourceACME {
		return nil, common.NewError("仅 ACME 或自签证书可设置自动续签")
	}
	acmeID := payload.ID
	if row.SourceRef != "" {
		if parsed, parseErr := strconv.ParseUint(strings.TrimSpace(row.SourceRef), 10, 64); parseErr == nil {
			acmeID = uint(parsed)
		}
	}

	entry, err := s.findCertificateByID(acmeID)
	if err != nil {
		return nil, err
	}
	entry.AutoRenew = payload.AutoRenew
	if err := database.GetDB().Save(entry).Error; err != nil {
		return nil, err
	}
	if _, upsertErr := upsertInventoryFromAcme(entry); upsertErr != nil {
		return nil, upsertErr
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	view := convertAcmeCertificate(entry)
	msg := "自动续签已关闭"
	if payload.AutoRenew {
		msg = "自动续签已开启"
	}
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
		Msg:         msg,
	}, nil
}

func (s *AcmeService) Delete(payload AcmeDeletePayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if payload.ID == 0 {
		return nil, common.NewError("certificate id is required")
	}
	row, getErr := certificateInventory.GetRecordByID(payload.ID)
	if getErr != nil {
		return nil, getErr
	}
	if certificateAssignedRecordMatches(PanelSelfSignedTargetPanel, row.Id) || certificateAssignedRecordMatches(PanelSelfSignedTargetSub, row.Id) {
		return nil, common.NewError("certificate is in use by panel or subscription")
	}
	if err := ensureCertificateRecordNotUsedByTLS(row.Id); err != nil {
		return nil, err
	}
	if err := ensureCertificateRecordNotUsedByReverseProxy(row.Id); err != nil {
		return nil, err
	}
	if err := removeTrackedCertificateFilesFromDirectory(strings.TrimSpace(row.PushDir), parseTrackedPushFiles(row.PushFiles)); err != nil {
		return nil, err
	}
	if row.SourceType != CertificateSourceACME {
		if row.SourceType == CertificateSourceImported {
			if err := clearLegacySettingsPathCertificateSource(&SettingService{}, row.SourceRef); err != nil {
				return nil, err
			}
		}
		if err := certificateInventory.DeleteByID(payload.ID); err != nil {
			return nil, err
		}
		if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
			return nil, err
		}
		overview, overviewErr := s.GetOverview()
		if overviewErr != nil {
			return nil, overviewErr
		}
		return &AcmeActionResult{
			Overview: overview,
			Msg:      "certificate deleted",
		}, nil
	}
	acmeID := payload.ID
	if row.SourceRef != "" {
		if parsed, parseErr := strconv.ParseUint(strings.TrimSpace(row.SourceRef), 10, 64); parseErr == nil {
			acmeID = uint(parsed)
		}
	}
	_, findErr := s.findCertificateByID(acmeID)
	if findErr != nil {
		if database.IsNotFound(findErr) {
			if err := certificateInventory.DeleteByID(payload.ID); err != nil {
				return nil, err
			}
			if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
				return nil, err
			}
			overview, overviewErr := s.GetOverview()
			if overviewErr != nil {
				return nil, overviewErr
			}
			return &AcmeActionResult{
				Overview: overview,
				Msg:      "certificate deleted",
			}, nil
		}
		return nil, findErr
	}
	if err := database.GetDB().Where("id = ?", acmeID).Delete(&model.AcmeCertificate{}).Error; err != nil {
		return nil, err
	}
	if err := certificateInventory.DeleteByID(payload.ID); err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{
		Overview: overview,
		Msg:      "certificate deleted",
	}, nil
}

func (s *AcmeService) renewInventorySelfSignedCertificate(row *model.CertificateRecord) (*AcmeActionResult, error) {
	if row == nil {
		return nil, common.NewError("certificate record is nil")
	}
	if strings.TrimSpace(row.SourceType) != CertificateSourceSelfSigned {
		return nil, common.NewError("仅 ACME 或自签证书可执行续签")
	}
	cfgText := strings.TrimSpace(row.RenewConfig)
	if cfgText == "" {
		return nil, common.NewError("当前自签证书缺少续签配置")
	}
	cfg := SelfSignedRenewConfig{}
	if err := json.Unmarshal([]byte(cfgText), &cfg); err != nil {
		return nil, common.NewError("解析自签续签配置失败: ", err)
	}

	domains := cfg.Domains
	if len(domains) == 0 && !cfg.AllowEmptyNames && strings.TrimSpace(cfg.Identity) != "" {
		domains = []string{strings.TrimSpace(cfg.Identity)}
	}
	if len(domains) == 0 && !cfg.AllowEmptyNames {
		domains = decodeCertificateDomains(row.DomainSet)
	}
	if len(domains) == 0 && !cfg.AllowEmptyNames && strings.TrimSpace(row.MainDomain) != "" {
		domains = []string{strings.TrimSpace(row.MainDomain)}
	}
	if len(domains) == 0 && !cfg.AllowEmptyNames {
		return nil, common.NewError("当前自签证书缺少可续签的域名或 IP")
	}

	authorityID := cfg.AuthorityID
	if authorityID == 0 {
		authorities, err := (&SelfSignedService{}).ListAuthorities()
		if err == nil {
			for _, authority := range authorities {
				if strings.EqualFold(strings.TrimSpace(authority.PlatformCode), strings.TrimSpace(cfg.PlatformCode)) {
					authorityID = authority.Id
					break
				}
			}
		}
	}

	payload := SelfSignedIssuePayload{
		ExistingRecordID:   row.Id,
		PreferredSourceRef: strings.TrimSpace(row.SourceRef),
		AuthorityID:        authorityID,
		AuthorityName:      strings.TrimSpace(cfg.AuthorityName),
		PlatformCode:       strings.TrimSpace(cfg.PlatformCode),
		PlatformName:       strings.TrimSpace(cfg.PlatformName),
		DomainsText:        strings.Join(domains, "\n"),
		AllowEmptyNames:    cfg.AllowEmptyNames,
		KeyAlgorithm:       strings.TrimSpace(cfg.KeyAlgorithm),
		SignatureAlgorithm: strings.TrimSpace(cfg.SignatureAlgorithm),
		DurationValue:      cfg.DurationValue,
		DurationUnit:       strings.TrimSpace(cfg.DurationUnit),
		Remark:             strings.TrimSpace(row.Remark),
		PushDir:            strings.TrimSpace(row.PushDir),
		PushExplicit:       false,
		TrackedPushFiles:   strings.TrimSpace(row.PushFiles),
		ApplyTarget:        strings.TrimSpace(row.ApplyTarget),
	}
	return (&SelfSignedService{}).Issue(payload)
}

func (s *AcmeService) SaveAcmeAccount(payload AcmeAccountPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	name := strings.TrimSpace(payload.Name)
	email, emailErr := validateAcmeEmail(payload.Email)
	if emailErr != nil {
		return nil, emailErr
	}
	serverInput := strings.TrimSpace(payload.Server)
	server := normalizeSupportedAcmeDomainServer(serverInput)
	keyLength := normalizeAcmeKeyLength(payload.KeyLength)
	remark := strings.TrimSpace(payload.Remark)

	if name == "" {
		return nil, common.NewError("acme 账号名称不能为空")
	}
	if serverInput != "" && server == "" {
		return nil, common.NewError("acme 账号 CA 平台仅支持 letsencrypt 或 zerossl")
	}
	if server == "" {
		server = normalizeSupportedAcmeDomainServer(s.readSettingWithDefault(acmePreferredCAKey, defaultAcmePreferredCA))
		if server == "" {
			server = defaultAcmePreferredCA
		}
	}
	if keyLength == "" {
		keyLength = normalizeAcmeKeyLength(s.readSettingWithDefault(acmeDefaultKeyLengthKey, defaultAcmeKeyLength))
	}
	if keyLength == "" {
		keyLength = defaultAcmeKeyLength
	}

	entry := &model.AcmeAccount{}
	db := database.GetDB()
	if payload.ID > 0 {
		if err := db.Where("id = ?", payload.ID).First(entry).Error; err != nil {
			return nil, err
		}
	}
	entry.Name = name
	entry.Email = email
	entry.Server = server
	entry.KeyLength = keyLength
	entry.Remark = remark

	if err := db.Save(entry).Error; err != nil {
		return nil, err
	}

	if err := s.setString(acmePreferredCAKey, server); err != nil {
		return nil, err
	}
	if err := s.setString(acmeDefaultKeyLengthKey, keyLength); err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}
	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{Overview: overview}, nil
}

func (s *AcmeService) SaveContactEmail(email string) (*AcmeActionResult, error) {
	email = normalizeAcmeEmail(email)
	if email != "" {
		validEmail, validErr := validateAcmeEmail(email)
		if validErr != nil {
			return nil, validErr
		}
		email = validEmail
	}
	if err := s.setString(acmeContactEmailKey, email); err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{Overview: overview}, nil
}

func (s *AcmeService) DeleteAcmeAccount(id uint) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if id == 0 {
		return nil, common.NewError("acme 账号 id 不能为空")
	}
	if err := database.GetDB().Where("id = ?", id).Delete(&model.AcmeAccount{}).Error; err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{Overview: overview}, nil
}

func (s *AcmeService) SaveDNSAccount(payload AcmeDNSAccountPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	name := strings.TrimSpace(payload.Name)
	providerCode := strings.TrimSpace(payload.ProviderCode)
	remark := strings.TrimSpace(payload.Remark)
	if name == "" {
		return nil, common.NewError("dns 账号名称不能为空")
	}
	providerMeta, ok := lookupAcmeDNSProvider(providerCode)
	if !ok {
		return nil, common.NewError("不支持的 dns 提供商: ", providerCode)
	}
	envMap, err := parseAcmeEnvJSON(payload.EnvJSON)
	if err != nil {
		return nil, err
	}
	envMap = sanitizeDNSAccountEnvForProvider(providerMeta, envMap)

	entry := &model.AcmeDNSAccount{}
	db := database.GetDB()
	existingEnvMap := map[string]string{}
	if payload.ID > 0 {
		if err := db.Where("id = ?", payload.ID).First(entry).Error; err != nil {
			return nil, err
		}
		existingEnvMap, _ = parseAcmeEnvJSON(entry.EnvJSON)
	}
	if payload.ID > 0 && !strings.EqualFold(strings.TrimSpace(entry.ProviderCode), providerMeta.ProviderCode) {
		existingEnvMap = map[string]string{}
	}
	envMap = mergeAcmeDNSAccountEnv(existingEnvMap, envMap)
	if err := validateDNSProviderEnv(providerMeta, envMap); err != nil {
		return nil, err
	}

	envRaw, err := json.Marshal(envMap)
	if err != nil {
		return nil, err
	}

	entry.Name = name
	entry.ProviderName = providerMeta.Name
	entry.ProviderCode = providerMeta.ProviderCode
	entry.EnvJSON = string(envRaw)
	entry.Remark = remark

	if err := db.Save(entry).Error; err != nil {
		return nil, err
	}

	if err := s.setString(acmeDefaultDNSProviderKey, providerMeta.ProviderCode); err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{Overview: overview}, nil
}

func (s *AcmeService) DeleteDNSAccount(id uint) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if id == 0 {
		return nil, common.NewError("dns 账号 id 不能为空")
	}
	db := database.GetDB()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", id).Delete(&model.AcmeDNSAccount{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AcmeCertificate{}).
			Where("dns_account_id = ?", id).
			Updates(map[string]interface{}{
				"dns_account_id":   0,
				"dns_account_name": "",
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.CertificateRecord{}).
			Where("source_type = ? AND dns_account_id = ?", CertificateSourceACME, id).
			Updates(map[string]interface{}{
				"dns_account_id":   0,
				"dns_account_name": "",
			}).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{Overview: overview}, nil
}

func (s *AcmeService) RunAutoRenew() (int, error) {
	if !acmeAutoRenewRunning.CompareAndSwap(false, true) {
		logger.Info("acme auto-renew skipped: previous run is still in progress")
		return 0, nil
	}
	defer acmeAutoRenewRunning.Store(false)

	renewedCount := 0
	failedMessages := make([]string, 0)

	if runtime.GOOS == "linux" {
		_, _, installed := s.resolveAcmeScript()
		if installed {
			rows := make([]model.AcmeCertificate, 0)
			if err := database.GetDB().Where("auto_renew = ?", true).Order("not_after ASC, id ASC").Find(&rows).Error; err != nil {
				return 0, err
			}
			for i := range rows {
				entry := rows[i]
				freshEntry := &model.AcmeCertificate{}
				if err := database.GetDB().Where("id = ?", entry.Id).First(freshEntry).Error; err != nil {
					if database.IsNotFound(err) {
						logger.Info("acme auto-renew skipped removed certificate id: ", entry.Id)
						continue
					}
					failedMessages = append(failedMessages, fmt.Sprintf("%s: refresh certificate failed: %v", entry.MainDomain, err))
					continue
				}
				now := time.Now().Unix()
				windowSeconds := computeAutoRenewWindowSeconds(freshEntry)
				if !shouldAutoRenewCertificate(freshEntry, now, windowSeconds) {
					logger.Info("acme auto-renew skipped by fresh-check for certificate: ", strings.TrimSpace(freshEntry.MainDomain))
					continue
				}

				recordID, err := certificateRecordIDForACMEEntry(freshEntry)
				if err != nil {
					failedMessages = append(failedMessages, fmt.Sprintf("%s: %v", freshEntry.MainDomain, err))
					continue
				}

				_, err = s.Renew(AcmeRenewPayload{
					ID:    recordID,
					Force: false,
				})
				if err != nil {
					failedMessages = append(failedMessages, fmt.Sprintf("%s: %v", freshEntry.MainDomain, err))
					continue
				}
				renewedCount++
			}
		}
	}

	selfSignedRows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().
		Where("source_type = ? AND auto_renew = ?", CertificateSourceSelfSigned, true).
		Order("not_after ASC, id ASC").
		Find(&selfSignedRows).Error; err != nil {
		return renewedCount, err
	}
	for i := range selfSignedRows {
		row := selfSignedRows[i]
		freshRow, freshErr := certificateInventory.GetRecordByID(row.Id)
		if freshErr != nil {
			if database.IsNotFound(freshErr) {
				logger.Info("acme auto-renew skipped removed self-signed certificate id: ", row.Id)
				continue
			}
			failedMessages = append(failedMessages, fmt.Sprintf("%s: refresh self-signed certificate failed: %v", row.MainDomain, freshErr))
			continue
		}
		now := time.Now().Unix()
		if !shouldAutoRenewInventorySelfSigned(freshRow, now) {
			logger.Info("acme auto-renew skipped self-signed by fresh-check for certificate: ", strings.TrimSpace(freshRow.MainDomain))
			continue
		}
		_, err := s.Renew(AcmeRenewPayload{ID: freshRow.Id})
		if err != nil {
			failedMessages = append(failedMessages, fmt.Sprintf("%s: %v", freshRow.MainDomain, err))
			continue
		}
		renewedCount++
	}

	if len(failedMessages) > 0 {
		return renewedCount, common.NewError(strings.Join(failedMessages, "; "))
	}

	return renewedCount, nil
}

func shouldAutoRenewInventorySelfSigned(entry *model.CertificateRecord, nowUnix int64) bool {
	if entry == nil || !entry.AutoRenew || entry.NotAfter <= 0 {
		return false
	}
	windowSeconds := int64(defaultSelfSignedDurationValue/3) * 24 * 3600
	if windowSeconds <= 0 {
		windowSeconds = 30 * 24 * 3600
	}
	return entry.NotAfter <= nowUnix+windowSeconds
}

func (s *AcmeService) listCertificates() ([]AcmeCertificateView, error) {
	return certificateInventory.List()
}

func (s *AcmeService) syncInventoryFromAcmeDB() error {
	rows := make([]model.AcmeCertificate, 0)
	if err := database.GetDB().Find(&rows).Error; err != nil {
		return err
	}
	activeIDs := make(map[string]struct{}, len(rows))
	for i := range rows {
		activeIDs[strconv.FormatUint(uint64(rows[i].Id), 10)] = struct{}{}
		if _, err := upsertInventoryFromAcme(&rows[i]); err != nil {
			return err
		}
	}
	inventoryRows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().
		Where("source_type = ?", CertificateSourceACME).
		Find(&inventoryRows).Error; err != nil {
		return err
	}
	for i := range inventoryRows {
		sourceRef := strings.TrimSpace(inventoryRows[i].SourceRef)
		if sourceRef == "" {
			if err := certificateInventory.DeleteByID(inventoryRows[i].Id); err != nil {
				return err
			}
			continue
		}
		if _, ok := activeIDs[sourceRef]; ok {
			continue
		}
		if _, parseErr := strconv.ParseUint(sourceRef, 10, 64); parseErr != nil {
			if err := certificateInventory.DeleteByID(inventoryRows[i].Id); err != nil {
				return err
			}
			continue
		}
		if err := certificateInventory.DeleteByID(inventoryRows[i].Id); err != nil {
			return err
		}
	}
	if err := certificateInventory.RepairDisplayIDs(); err != nil {
		return err
	}
	return nil
}

func (s *AcmeService) cleanupNonDNSCertificateDNSReferences() error {
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.AcmeCertificate{}).
			Where("LOWER(TRIM(challenge)) <> ? AND (dns_account_id <> 0 OR TRIM(dns_account_name) <> '')", "dns").
			Updates(map[string]interface{}{
				"dns_account_id":   0,
				"dns_account_name": "",
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.CertificateRecord{}).
			Where("source_type = ? AND LOWER(TRIM(challenge)) <> ? AND (dns_account_id <> 0 OR TRIM(dns_account_name) <> '')", CertificateSourceACME, "dns").
			Updates(map[string]interface{}{
				"dns_account_id":   0,
				"dns_account_name": "",
			}).Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *AcmeService) listAcmeAccounts() ([]AcmeAccountView, error) {
	rows := make([]model.AcmeAccount, 0)
	if err := database.GetDB().Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]AcmeAccountView, 0, len(rows))
	for i := range rows {
		entry := rows[i]
		result = append(result, AcmeAccountView{
			Id:        entry.Id,
			Name:      entry.Name,
			Email:     entry.Email,
			Server:    entry.Server,
			KeyLength: entry.KeyLength,
			Remark:    entry.Remark,
			CreatedAt: entry.CreatedAt.Unix(),
			UpdatedAt: entry.UpdatedAt.Unix(),
		})
	}
	return result, nil
}

func (s *AcmeService) listDNSAccounts() ([]AcmeDNSAccountView, error) {
	rows := make([]model.AcmeDNSAccount, 0)
	if err := database.GetDB().Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]AcmeDNSAccountView, 0, len(rows))
	for i := range rows {
		entry := rows[i]
		envMap, _ := parseAcmeEnvJSON(entry.EnvJSON)
		envMap = sanitizeAcmeEnvMap(envMap)
		result = append(result, AcmeDNSAccountView{
			Id:           entry.Id,
			Name:         entry.Name,
			ProviderName: entry.ProviderName,
			ProviderCode: entry.ProviderCode,
			Env:          envMap,
			Remark:       entry.Remark,
			CreatedAt:    entry.CreatedAt.Unix(),
			UpdatedAt:    entry.UpdatedAt.Unix(),
		})
	}
	return result, nil
}

func shouldAutoRenewCertificate(entry *model.AcmeCertificate, nowUnix int64, windowSeconds int64) bool {
	if entry == nil {
		return false
	}
	if !entry.AutoRenew {
		return false
	}
	if entry.NotAfter <= 0 {
		return false
	}
	if nowUnix <= 0 {
		return false
	}
	if windowSeconds <= 0 {
		windowSeconds = computeAutoRenewWindowSeconds(entry)
	}
	return entry.NotAfter <= nowUnix+windowSeconds
}

func computeAutoRenewWindowSeconds(entry *model.AcmeCertificate) int64 {
	defaultWindow := int64(defaultAcmeAutoRenewDays) * 24 * 3600
	if entry == nil {
		return defaultWindow
	}
	if entry.NotBefore <= 0 || entry.NotAfter <= 0 || entry.NotAfter <= entry.NotBefore {
		return defaultWindow
	}
	validitySeconds := entry.NotAfter - entry.NotBefore
	validityDays := float64(validitySeconds) / 86400.0
	if validityDays > 40 {
		return defaultWindow
	}

	windowDays := int64(math.Floor(validityDays / 3.0))
	if windowDays < 1 {
		windowDays = 1
	}
	return windowDays * 24 * 3600
}

func (s *AcmeService) findCertificateByID(id uint) (*model.AcmeCertificate, error) {
	entry := &model.AcmeCertificate{}
	if err := database.GetDB().Where("id = ?", id).First(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

type certificateDirectoryPushState struct {
	PushDir   string
	PushFiles string
}

func syncCertificateDirectoryPushState(targetDir string, currentDir string, currentTracked string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) (certificateDirectoryPushState, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return certificateDirectoryPushState{}, common.NewError("target directory is empty")
	}

	currentDir = strings.TrimSpace(currentDir)
	oldTracked := parseTrackedPushFiles(currentTracked)
	if currentDir != "" && !sameCleanPath(currentDir, targetDir) {
		if err := removeTrackedCertificateFilesFromDirectory(currentDir, oldTracked); err != nil {
			return certificateDirectoryPushState{}, err
		}
	}

	writeTrackedBase := oldTracked
	if currentDir != "" && !sameCleanPath(currentDir, targetDir) {
		writeTrackedBase = nil
	}

	writtenFiles, err := replaceCertificateInDirectoryWithTrackedFiles(targetDir, writeTrackedBase, certPEM, keyPEM, fullchainPEM, chainPEM)
	if err != nil {
		return certificateDirectoryPushState{}, err
	}

	return certificateDirectoryPushState{
		PushDir:   targetDir,
		PushFiles: encodeTrackedPushFiles(writtenFiles),
	}, nil
}

func loadAcmeSourceEntryForInventoryRow(row *model.CertificateRecord) (*model.AcmeCertificate, error) {
	if row == nil {
		return nil, common.NewError("certificate record is nil")
	}
	if strings.TrimSpace(row.SourceType) != CertificateSourceACME {
		return nil, nil
	}

	sourceRef := strings.TrimSpace(row.SourceRef)
	if sourceRef == "" {
		return nil, nil
	}

	sourceID, err := strconv.ParseUint(sourceRef, 10, 64)
	if err != nil {
		return nil, nil
	}

	entry, err := (&AcmeService{}).findCertificateByID(uint(sourceID))
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return entry, nil
}

func persistCertificatePushState(record *model.CertificateRecord, sourceEntry *model.AcmeCertificate) error {
	if record == nil {
		return common.NewError("certificate record is nil")
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(record).Error; err != nil {
			return err
		}
		if sourceEntry != nil {
			if err := tx.Save(sourceEntry).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func certificateRecordIDForACMEEntry(entry *model.AcmeCertificate) (uint, error) {
	if entry == nil || entry.Id == 0 {
		return 0, common.NewError("acme certificate id is required")
	}
	sourceRef := strconv.FormatUint(uint64(entry.Id), 10)
	record := &model.CertificateRecord{}
	err := database.GetDB().
		Where("source_type = ? AND source_ref = ?", CertificateSourceACME, sourceRef).
		First(record).Error
	if err == nil {
		return record.Id, nil
	}
	if !database.IsNotFound(err) {
		return 0, err
	}

	record, err = upsertInventoryFromAcme(entry)
	if err != nil {
		return 0, err
	}
	if record == nil || record.Id == 0 {
		return 0, common.NewError("certificate inventory record is empty")
	}
	return record.Id, nil
}

func upsertInventoryFromAcme(entry *model.AcmeCertificate) (*model.CertificateRecord, error) {
	if entry == nil {
		return nil, common.NewError("certificate record is nil")
	}
	sourceRef := fmt.Sprintf("%d", entry.Id)
	return certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType: CertificateSourceACME,
		SourceRef:  sourceRef,

		MainDomain: entry.MainDomain,
		Domains:    decodeCertificateDomains(entry.DomainSet),

		CertificateType: acmeCertificateTypeForEntry(entry),
		CertProfile:     strings.TrimSpace(entry.CertProfile),
		Challenge:       entry.Challenge,
		KeyLength:       entry.KeyLength,
		CAServer:        entry.CAServer,
		UseECC:          entry.UseECC,
		AutoRenew:       entry.AutoRenew,

		AcmeAccountID:   entry.AcmeAccountID,
		AcmeAccountName: entry.AcmeAccountName,
		DNSAccountID:    entry.DNSAccountID,
		DNSAccountName:  entry.DNSAccountName,
		ApplyTarget:     entry.ApplyTarget,
		PushDir:         entry.PushDir,
		PushFiles:       entry.PushFiles,
		Remark:          entry.Remark,

		AcmeHome:    entry.AcmeHome,
		Webroot:     entry.Webroot,
		DNSProvider: entry.DNSProvider,
		DNSEnvText:  entry.DNSEnvText,
		CustomArgs:  entry.CustomArgs,

		CertPath:      entry.CertPath,
		KeyPath:       entry.KeyPath,
		FullchainPath: entry.FullchainPath,
		ChainPath:     entry.ChainPath,

		CertPEM:      entry.CertPEM,
		KeyPEM:       entry.KeyPEM,
		FullchainPEM: entry.FullchainPEM,
		ChainPEM:     entry.ChainPEM,

		Fingerprint: entry.Fingerprint,
		NotBefore:   entry.NotBefore,
		NotAfter:    entry.NotAfter,

		LastIssuedAt:  entry.LastIssuedAt,
		LastRenewedAt: entry.LastRenewedAt,
		LastError:     entry.LastError,
		LastOutput:    entry.LastOutput,
	})
}

type acmeManagedCertPaths struct {
	CertPath      string
	KeyPath       string
	FullchainPath string
	ChainPath     string
	BaseDir       string
}

func (s *AcmeService) installCertToManagedDir(scriptPath string, homeDir string, mainDomain string, useECC bool, envPairs []string, logSession *acmeLogSession) (*acmeManagedCertPaths, func(), error) {
	paths, cleanup, err := createAcmeTempInstallPaths(mainDomain)
	if err != nil {
		return nil, nil, err
	}
	if err := s.installCertByDomain(scriptPath, homeDir, mainDomain, useECC, paths, envPairs, logSession); err != nil {
		cleanup()
		return nil, nil, err
	}
	return paths, cleanup, nil
}

func (s *AcmeService) installCertByRecord(scriptPath string, homeDir string, entry *model.AcmeCertificate, paths *acmeManagedCertPaths, envPairs []string, logSession *acmeLogSession) error {
	if entry == nil {
		return common.NewError("certificate record is nil")
	}
	if paths == nil {
		return common.NewError("managed certificate paths are nil")
	}
	return s.installCertByDomain(scriptPath, homeDir, entry.MainDomain, entry.UseECC, paths, envPairs, logSession)
}

func (s *AcmeService) installCertByDomain(scriptPath string, homeDir string, mainDomain string, useECC bool, paths *acmeManagedCertPaths, envPairs []string, logSession *acmeLogSession) error {
	mainDomain = strings.TrimSpace(mainDomain)
	if mainDomain == "" {
		return common.NewError("main domain is empty")
	}
	if paths == nil {
		return common.NewError("install cert paths are nil")
	}
	if err := os.MkdirAll(filepath.Dir(paths.CertPath), 0o755); err != nil {
		return common.NewError("create acme managed directory failed: ", err)
	}

	args := []string{
		"--install-cert",
		"-d", mainDomain,
		"--cert-file", paths.CertPath,
		"--key-file", paths.KeyPath,
		"--fullchain-file", paths.FullchainPath,
		"--ca-file", paths.ChainPath,
	}
	if useECC {
		args = append(args, "--ecc")
	}

	if logSession != nil {
		logSession.append("执行 acme.sh --install-cert")
	}
	if _, err := runCommandOutputWithTimeoutEnvLog(2*time.Minute, scriptPath, append(acmeHomeArgs(homeDir), args...), envPairs, logSession); err != nil {
		return common.NewError("install certificate files failed: ", err)
	}
	return nil
}

func (s *AcmeService) upsertCertificateFromPaths(
	recordID uint,
	domains []string,
	certificateType string,
	certProfile string,
	challenge string,
	keyLength string,
	caServer string,
	useECC bool,
	homeDir string,
	webroot string,
	dnsProvider string,
	dnsEnvText string,
	customArgs string,
	paths *acmeManagedCertPaths,
	renewUnix int64,
) (*model.AcmeCertificate, error) {
	if len(domains) == 0 {
		return nil, common.NewError("domains are empty")
	}
	if paths == nil {
		return nil, common.NewError("paths are empty")
	}

	certPEM, keyPEM, fullchainPEM, chainPEM, err := readCertificateBundle(paths)
	if err != nil {
		return nil, err
	}
	fingerprint, notBefore, notAfter, err := inspectCertificateFingerprint(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	mainDomain := domains[0]
	domainJSON, err := json.Marshal(domains)
	if err != nil {
		return nil, err
	}

	var entry model.AcmeCertificate
	db := database.GetDB()
	if recordID > 0 {
		if err := db.Where("id = ?", recordID).First(&entry).Error; err != nil {
			return nil, err
		}
	} else {
		entry = model.AcmeCertificate{}
	}

	now := time.Now().Unix()
	entry.MainDomain = mainDomain
	entry.DomainSet = string(domainJSON)
	entry.CertificateType = normalizeAcmeCertificateType(certificateType)
	entry.CertProfile = strings.TrimSpace(certProfile)
	entry.Challenge = challenge
	entry.KeyLength = keyLength
	entry.CAServer = caServer
	entry.UseECC = useECC
	entry.AcmeHome = homeDir
	entry.Webroot = strings.TrimSpace(webroot)
	entry.DNSProvider = strings.TrimSpace(dnsProvider)
	entry.DNSEnvText = strings.TrimSpace(dnsEnvText)
	entry.CustomArgs = strings.TrimSpace(customArgs)
	entry.CertPath = ""
	entry.KeyPath = ""
	entry.FullchainPath = ""
	entry.ChainPath = ""
	entry.CertPEM = certPEM
	entry.KeyPEM = keyPEM
	entry.FullchainPEM = fullchainPEM
	entry.ChainPEM = chainPEM
	entry.Fingerprint = fingerprint
	entry.NotBefore = notBefore.Unix()
	entry.NotAfter = notAfter.Unix()
	entry.LastError = ""
	if entry.LastIssuedAt == 0 {
		entry.LastIssuedAt = now
	}
	entry.LastRenewedAt = renewUnix
	if renewUnix <= 0 {
		entry.LastRenewedAt = now
	}

	if err := db.Save(&entry).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

func (s *AcmeService) markCertificateError(id uint, message string) error {
	message = strings.TrimSpace(message)
	if id == 0 {
		return nil
	}
	return database.GetDB().Model(&model.AcmeCertificate{}).Where("id = ?", id).Update("last_error", message).Error
}

func (s *AcmeService) applyIssuePostActions(entry *model.AcmeCertificate, applyTarget string, pushDir string, pushExplicit bool) error {
	if entry == nil {
		return common.NewError("certificate record is nil")
	}
	record, err := upsertInventoryFromAcme(entry)
	if err != nil {
		return err
	}

	if normalizedTarget, ok := normalizeAcmeApplyTarget(applyTarget); ok {
		if err := s.applyInventoryRecordToTarget(record, normalizedTarget); err != nil {
			return err
		}
		targets, targetErr := assignedTargetsForCertificateRecord(record.Id)
		if targetErr != nil {
			return targetErr
		}
		applyTargetText := formatAssignedApplyTarget(targets)
		entry.ApplyTarget = applyTargetText
		record.ApplyTarget = applyTargetText
	}

	pushDir = strings.TrimSpace(pushDir)
	if pushExplicit && pushDir != "" {
		pushState, err := syncCertificateDirectoryPushState(pushDir, entry.PushDir, entry.PushFiles, entry.CertPEM, entry.KeyPEM, entry.FullchainPEM, entry.ChainPEM)
		if err != nil {
			return err
		}
		entry.PushDir = pushState.PushDir
		entry.PushFiles = pushState.PushFiles
		record.PushDir = pushState.PushDir
		record.PushFiles = pushState.PushFiles
	}
	if err := persistCertificatePushState(record, entry); err != nil {
		return err
	}
	if err := ApplyPanelTLSRuntimeSettingsForRecord(record.Id); err != nil {
		return err
	}
	if _, err := ForceSyncTLSBindingsForCertificateRecord(record.Id, ""); err != nil {
		return err
	}
	return nil
}

func (s *AcmeService) applyInventoryRecordToTarget(row *model.CertificateRecord, target PanelSelfSignedTarget) error {
	if row == nil {
		return common.NewError("certificate record is nil")
	}
	settingService := &SettingService{}
	if !target.isValid() {
		return common.NewError("invalid apply target")
	}

	assignedIDs, err := GetAssignedCertificateRecordIDs(settingService, target)
	if err != nil {
		return err
	}
	nextIDs := make([]uint, 0, len(assignedIDs)+1)
	nextIDs = append(nextIDs, row.Id)
	for _, id := range assignedIDs {
		if id == row.Id {
			continue
		}
		nextIDs = append(nextIDs, id)
	}

	if err := SetAssignedCertificateRecordIDs(settingService, target, nextIDs); err != nil {
		return err
	}
	if err := ApplyPanelTLSRuntimeSettings(target); err != nil {
		return err
	}
	return nil
}

func (s *AcmeService) unapplyInventoryRecordFromTarget(row *model.CertificateRecord, target PanelSelfSignedTarget) (bool, error) {
	if row == nil {
		return false, common.NewError("certificate record is nil")
	}
	settingService := &SettingService{}
	if !target.isValid() {
		return false, common.NewError("invalid apply target")
	}

	assignedIDs, err := GetAssignedCertificateRecordIDs(settingService, target)
	if err != nil {
		return false, err
	}
	index := slices.Index(assignedIDs, row.Id)
	if index < 0 {
		return false, nil
	}
	if len(assignedIDs) <= 1 {
		return false, common.NewError("at least one certificate must remain for target")
	}

	nextIDs := make([]uint, 0, len(assignedIDs)-1)
	nextIDs = append(nextIDs, assignedIDs[:index]...)
	nextIDs = append(nextIDs, assignedIDs[index+1:]...)
	if len(nextIDs) == 0 {
		return false, common.NewError("at least one certificate must remain for target")
	}

	if err := SetAssignedCertificateRecordIDs(settingService, target, nextIDs); err != nil {
		return false, err
	}
	if err := ApplyPanelTLSRuntimeSettings(target); err != nil {
		return false, err
	}
	return true, nil
}

func (s *AcmeService) persistAcmeDefaults(payload AcmeIssuePayload, challenge string, keyLength string, caServer string, dnsProvider string, useDNSChallenge bool) error {
	if err := s.setString(acmeDefaultChallengeKey, challenge); err != nil {
		return err
	}
	if err := s.setString(acmeDefaultKeyLengthKey, keyLength); err != nil {
		return err
	}
	if err := s.setString(acmePreferredCAKey, caServer); err != nil {
		return err
	}
	if strings.TrimSpace(payload.Webroot) != "" {
		if err := s.setString(acmeDefaultWebrootKey, strings.TrimSpace(payload.Webroot)); err != nil {
			return err
		}
	}
	if useDNSChallenge && strings.TrimSpace(dnsProvider) != "" {
		if err := s.setString(acmeDefaultDNSProviderKey, strings.TrimSpace(dnsProvider)); err != nil {
			return err
		}
	}
	return nil
}

type acmeTemporaryFirewallRule struct {
	id   uint
	port int
}

const (
	acmeTemporaryFirewallType     = "acme"
	acmeTemporaryFirewallLifetime = 30 * time.Minute
	acmeTemporaryFirewallNameFmt  = "ACME temporary allow %d/%d"
	acmeTemporaryFirewallDescText = "Temporary ACME validation rule, auto removed after issue or renew"
	acmeTemporaryFirewallPortSpec = "80, 443"
)

func collectAcmeChallengePortSnapshot() (*acmeChallengePortSnapshot, error) {
	resp, err := (&PortCheckService{}).Check(PortCheckRequest{
		SinglePorts: []int{acmeIPCertificatePortHTTP, acmeIPCertificatePortALPN},
	})
	if err != nil {
		return nil, common.NewError("check challenge ports failed: ", err)
	}
	snapshot := &acmeChallengePortSnapshot{
		Supported: resp.Supported,
		CheckedAt: resp.CheckedAt,
		ByPort:    map[int]SinglePortStatus{},
	}
	for _, item := range resp.Single {
		snapshot.ByPort[item.Port] = item
	}
	if _, ok := snapshot.ByPort[acmeIPCertificatePortHTTP]; !ok {
		snapshot.ByPort[acmeIPCertificatePortHTTP] = SinglePortStatus{Port: acmeIPCertificatePortHTTP}
	}
	if _, ok := snapshot.ByPort[acmeIPCertificatePortALPN]; !ok {
		snapshot.ByPort[acmeIPCertificatePortALPN] = SinglePortStatus{Port: acmeIPCertificatePortALPN}
	}
	return snapshot, nil
}

func acmePortChallengesForType(certificateType string) []string {
	if normalizeAcmeCertificateType(certificateType) == acmeCertificateTypeIP {
		return []string{"standalone", "alpn"}
	}
	return []string{"standalone", "webroot", "alpn"}
}

func acmePortForChallenge(challenge string) (int, bool) {
	switch normalizeAcmeChallenge(challenge) {
	case "standalone", "webroot":
		return acmeIPCertificatePortHTTP, true
	case "alpn":
		return acmeIPCertificatePortALPN, true
	default:
		return 0, false
	}
}

func isAcmePortChallenge(challenge string) bool {
	_, ok := acmePortForChallenge(challenge)
	return ok
}

func acmeChallengePortOccupied(snapshot *acmeChallengePortSnapshot, port int) (bool, bool) {
	if snapshot == nil {
		return false, false
	}
	item, ok := snapshot.ByPort[port]
	if !ok {
		return false, false
	}
	return item.TCP, item.UDP
}

func selectRecommendedAcmePortChallenge(certificateType string, snapshot *acmeChallengePortSnapshot) acmeChallengePortDecision {
	decision, err := selectAcmeChallengePortDecision(certificateType, "standalone", snapshot)
	if err != nil {
		return acmeChallengePortDecision{}
	}
	return decision
}

func selectAcmeChallengePortDecision(certificateType string, inputChallenge string, snapshot *acmeChallengePortSnapshot) (acmeChallengePortDecision, error) {
	result := acmeChallengePortDecision{
		InputChallenge: normalizeAcmeChallenge(inputChallenge),
		Challenge:      normalizeAcmeChallenge(inputChallenge),
	}
	normalizedType := normalizeAcmeCertificateType(certificateType)
	if normalizedType == acmeCertificateTypeIP {
		result.InputChallenge = normalizeAcmeIPChallenge(result.InputChallenge)
		result.Challenge = result.InputChallenge
		if result.InputChallenge == "" {
			return result, common.NewError("IP certificates only support standalone or alpn challenge")
		}
	}
	if !isAcmePortChallenge(result.InputChallenge) {
		return result, common.NewError("challenge does not use validation port 80/443")
	}
	candidates := acmePortChallengesForType(normalizedType)
	if !slices.Contains(candidates, result.InputChallenge) {
		return result, common.NewError("unsupported challenge for certificate type")
	}
	port, _ := acmePortForChallenge(result.InputChallenge)
	result.Port = port
	result.TCPOccupied, result.UDPOccupied = acmeChallengePortOccupied(snapshot, result.Port)

	if snapshot == nil || !snapshot.Supported {
		result.Available = true
		result.Recommended = true
		result.Reason = "port check is unsupported on current host; continue with selected challenge"
		return result, nil
	}
	if result.InputChallenge == "webroot" {
		result.Available = true
		result.Recommended = false
		result.Reason = acmePortStatusReasonForChallenge("webroot", result.TCPOccupied)
		return result, nil
	}
	if !result.TCPOccupied {
		result.Available = true
		result.Recommended = true
		result.Reason = fmt.Sprintf("%d/tcp is available", result.Port)
		return result, nil
	}

	for _, candidate := range candidates {
		if candidate == result.InputChallenge {
			continue
		}
		if candidate == "webroot" {
			continue
		}
		candidatePort, ok := acmePortForChallenge(candidate)
		if !ok {
			continue
		}
		tcpOccupied, udpOccupied := acmeChallengePortOccupied(snapshot, candidatePort)
		if tcpOccupied {
			continue
		}
		return acmeChallengePortDecision{
			InputChallenge: result.InputChallenge,
			Challenge:      candidate,
			Port:           candidatePort,
			TCPOccupied:    tcpOccupied,
			UDPOccupied:    udpOccupied,
			Available:      true,
			Recommended:    true,
			Switched:       candidate != result.InputChallenge,
			Reason:         fmt.Sprintf("%d/tcp is occupied; switched to %s because %d/tcp is available", result.Port, candidate, candidatePort),
		}, nil
	}

	return result, common.NewError("no available validation port combination for 80/443")
}

func acmePortStatusReasonForChallenge(challenge string, tcpOccupied bool) string {
	switch normalizeAcmeChallenge(challenge) {
	case "webroot":
		if tcpOccupied {
			return "检测到本机 80/tcp 已由现有 Web 服务监听。Webroot 验证可继续，请确认挑战文件可被外部访问。"
		}
		return "未检测到本机 80/tcp 监听。Webroot 验证仍可继续，但请确认挑战文件最终能被外部访问。"
	default:
		port, ok := acmePortForChallenge(challenge)
		if !ok {
			return ""
		}
		if tcpOccupied {
			return fmt.Sprintf("%d 端口已被占用", port)
		}
		return fmt.Sprintf("%d 端口空闲", port)
	}
}

func appendAcmeRenewChallengeArgs(args *[]string, challenge string) {
	if args == nil {
		return
	}
	switch normalizeAcmeChallenge(challenge) {
	case "standalone":
		*args = append(*args, "--standalone")
	case "alpn":
		*args = append(*args, "--alpn")
	case "webroot":
		// Keep existing acme.sh renewal profile for webroot.
	default:
	}
}

func buildAcmeIPPortItem(challenge string, port int, tcpOccupied bool, udpOccupied bool, recommended bool, reason string) AcmeIPPortItem {
	message := fmt.Sprintf("%d 端口空闲", port)
	occupied := tcpOccupied
	available := !tcpOccupied
	if normalizeAcmeChallenge(challenge) == "webroot" {
		available = true
	}
	if strings.TrimSpace(reason) != "" {
		message = strings.TrimSpace(reason)
	}
	if occupied && strings.TrimSpace(reason) == "" {
		message = fmt.Sprintf("%d 端口已被占用", port)
	}
	return AcmeIPPortItem{
		Challenge:   challenge,
		Port:        port,
		Occupied:    occupied,
		Available:   available,
		TCPOccupied: tcpOccupied,
		UDPOccupied: udpOccupied,
		Recommended: recommended,
		Reason:      message,
		Message:     message,
	}
}

func ensureAcmeIPPortFree(port int, logSession *acmeLogSession) error {
	if logSession != nil {
		logSession.append(fmt.Sprintf("检测验证端口占用: %d/tcp", port))
	}
	resp, err := (&PortCheckService{}).Check(PortCheckRequest{SinglePorts: []int{port}})
	if err != nil {
		return common.NewError("检测端口占用失败: ", err)
	}
	if !resp.Supported {
		if logSession != nil {
			logSession.append("当前系统不支持 /proc 端口占用检测，继续交给 acme.sh 执行")
		}
		return nil
	}
	for _, item := range resp.Single {
		if item.Port != port {
			continue
		}
		if item.TCP {
			return common.NewError(fmt.Sprintf("%d 端口已被占用，无法使用当前 IP 证书验证方式", port))
		}
		if logSession != nil {
			logSession.append(fmt.Sprintf("%d/tcp 空闲，可用于验证", port))
		}
		return nil
	}
	return nil
}

func (s *AcmeService) prepareTemporaryAcmeFirewallRule(port int, logSession *acmeLogSession) (*acmeTemporaryFirewallRule, error) {
	if logSession != nil {
		logSession.append(fmt.Sprintf("检查防火墙是否需要临时放行 80/443 tcp+udp（当前验证端口 %d/tcp）", port))
	}
	if runtime.GOOS != "linux" {
		if logSession != nil {
			logSession.append("非 Linux 系统，跳过防火墙临时放行")
		}
		return nil, nil
	}

	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	firewallSvc := &FirewallService{}
	enabled, err := firewallSvc.getFirewallEnabledLocked()
	if err != nil {
		return nil, err
	}
	if !enabled {
		if logSession != nil {
			logSession.append("防火墙未开启，无需临时放行")
		}
		return nil, nil
	}
	if !firewallSupported() {
		if logSession != nil {
			logSession.append("nftables firewall is unavailable, skip temporary allow rule")
		}
		return nil, nil
	}

	if allowed, err := firewallHasManagedDualTCPUDPPortsAllowLocked(acmeIPCertificatePortHTTP, acmeIPCertificatePortALPN); err != nil {
		return nil, err
	} else if allowed {
		if logSession != nil {
			logSession.append("防火墙已完整放行 80/443 tcp+udp，无需新增规则")
		}
		if err := firewallSvc.reconcileLocked(0); err != nil {
			return nil, err
		}
		return nil, nil
	}

	row := buildAcmeTemporaryFirewallRuleRow()
	if err := database.GetDB().Create(&row).Error; err != nil {
		return nil, err
	}
	keepRule := true
	defer func() {
		if keepRule {
			return
		}
		_ = database.GetDB().Where("id = ?", row.Id).Delete(&model.FirewallRule{}).Error
	}()
	if logSession != nil {
		logSession.append(fmt.Sprintf("temporary firewall rule created id=%d, allow 80/443 tcp+udp (selected %d/tcp)", row.Id, port))
	}
	if err := firewallSvc.reconcileLocked(0); err != nil {
		keepRule = false
		return nil, err
	}
	if logSession != nil {
		logSession.append("临时防火墙规则已生效")
	}
	return &acmeTemporaryFirewallRule{id: row.Id, port: port}, nil
}

func (s *AcmeService) cleanupTemporaryAcmeFirewallRule(rule *acmeTemporaryFirewallRule, logSession *acmeLogSession) {
	if rule == nil || rule.id == 0 {
		return
	}
	if logSession != nil {
		logSession.append(fmt.Sprintf("开始还原防火墙，删除临时 80/443 tcp+udp 规则 id=%d", rule.id))
	}
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()
	if err := database.GetDB().Where("id = ?", rule.id).Delete(&model.FirewallRule{}).Error; err != nil {
		if logSession != nil {
			logSession.append("删除临时防火墙规则失败: " + err.Error())
		}
		return
	}
	if err := (&FirewallService{}).reconcileLocked(0); err != nil {
		if logSession != nil {
			logSession.append("防火墙还原失败: " + err.Error())
		}
		return
	}
	if logSession != nil {
		logSession.append("firewall restored, 80/443 tcp+udp temporary allow removed")
	}
}

func buildAcmeTemporaryFirewallRuleRow() model.FirewallRule {
	now := time.Now().Unix()
	return model.FirewallRule{
		Name:              fmt.Sprintf(acmeTemporaryFirewallNameFmt, acmeIPCertificatePortHTTP, acmeIPCertificatePortALPN),
		Description:       acmeTemporaryFirewallDescText,
		Enabled:           true,
		Origin:            firewallOriginTemporary,
		SystemKey:         "",
		TemporaryType:     acmeTemporaryFirewallType,
		TemporaryExpireAt: time.Now().Add(acmeTemporaryFirewallLifetime).Unix(),
		Direction:         firewallDirectionIngress,
		Family:            firewallFamilyDual,
		Protocol:          firewallProtocolTCPUDP,
		PortSpec:          acmeTemporaryFirewallPortSpec,
		SourceSpec:        "",
		LastSeenAt:        now,
	}
}

func firewallHasManagedDualTCPUDPPortAllowLocked(port int) (bool, error) {
	rows, err := loadFirewallRulesLocked()
	if err != nil {
		return false, err
	}
	v4TCP := false
	v4UDP := false
	v6TCP := false
	v6UDP := false
	for _, row := range rows {
		if !row.Enabled || !firewallRuleParticipatesInManagedChain(row) {
			continue
		}
		if row.Direction != firewallDirectionIngress {
			continue
		}
		if strings.TrimSpace(row.SourceSpec) != "" {
			continue
		}
		protocol := normalizeFirewallProtocol(row.Protocol)
		protocolTCP := protocol == firewallProtocolTCP || protocol == firewallProtocolTCPUDP
		protocolUDP := protocol == firewallProtocolUDP || protocol == firewallProtocolTCPUDP
		if !protocolTCP && !protocolUDP {
			continue
		}
		if !firewallPortSpecContains(row.PortSpec, port) {
			continue
		}
		family := normalizeFirewallFamily(row.Family)
		coversV4 := family == firewallFamilyDual || family == firewallFamilyIPv4
		coversV6 := family == firewallFamilyDual || family == firewallFamilyIPv6
		if coversV4 && protocolTCP {
			v4TCP = true
		}
		if coversV4 && protocolUDP {
			v4UDP = true
		}
		if coversV6 && protocolTCP {
			v6TCP = true
		}
		if coversV6 && protocolUDP {
			v6UDP = true
		}
		if v4TCP && v4UDP && v6TCP && v6UDP {
			return true, nil
		}
	}
	return false, nil
}

func firewallHasManagedDualTCPUDPPortsAllowLocked(ports ...int) (bool, error) {
	seen := make(map[int]struct{}, len(ports))
	checked := false
	for _, port := range ports {
		if port < 1 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		checked = true
		allowed, err := firewallHasManagedDualTCPUDPPortAllowLocked(port)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return checked, nil
}

func firewallPortSpecContains(spec string, port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	for _, item := range parsePortRangeInput(spec) {
		if port >= item.start && port <= item.end {
			return true
		}
	}
	return false
}

func (s *AcmeService) readSettingWithDefault(key string, fallback string) string {
	value, err := s.getString(key)
	if err != nil {
		return fallback
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func normalizeAcmeEmail(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	normalized = strings.Map(func(r rune) rune {
		switch r {
		case '\u00a0':
			return ' '
		case '\u200b', '\u200c', '\u200d', '\ufeff':
			return -1
		case '＜':
			return '<'
		case '＞':
			return '>'
		case '＠', '﹫':
			return '@'
		case '。', '．', '｡':
			return '.'
		default:
			if r < 32 || r == 127 {
				return -1
			}
			return r
		}
	}, normalized)
	normalized = strings.Join(strings.Fields(normalized), "")
	if parsed, err := mail.ParseAddress(normalized); err == nil && parsed != nil {
		return strings.TrimSpace(parsed.Address)
	}
	if left := strings.LastIndex(normalized, "<"); left >= 0 {
		rightOffset := strings.Index(normalized[left+1:], ">")
		if rightOffset > 0 {
			inner := strings.TrimSpace(normalized[left+1 : left+1+rightOffset])
			if parsed, err := mail.ParseAddress(inner); err == nil && parsed != nil {
				return strings.TrimSpace(parsed.Address)
			}
			if inner != "" {
				return inner
			}
		}
	}
	return normalized
}

func validateAcmeEmail(value string) (string, error) {
	normalized := normalizeAcmeEmail(value)
	if normalized == "" {
		return "", common.NewError("acme 邮箱不能为空")
	}
	if !isASCIIEmailAddress(normalized) {
		return "", common.NewError("acme 邮箱格式无效：仅支持 ASCII 邮箱地址（示例：name@example.com）")
	}
	parsed, err := mail.ParseAddress(normalized)
	if err != nil || parsed == nil {
		return "", common.NewError("acme 邮箱格式无效（示例：name@example.com）")
	}
	address := strings.TrimSpace(parsed.Address)
	if address == "" {
		return "", common.NewError("acme 邮箱格式无效（示例：name@example.com）")
	}
	if strings.Count(address, "@") != 1 {
		return "", common.NewError("acme 邮箱格式无效（示例：name@example.com）")
	}
	local, domain, ok := strings.Cut(address, "@")
	if !ok || strings.TrimSpace(local) == "" || strings.TrimSpace(domain) == "" {
		return "", common.NewError("acme 邮箱格式无效（示例：name@example.com）")
	}
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", common.NewError("acme 邮箱格式无效（示例：name@example.com）")
	}
	return address, nil
}

func isASCIIEmailAddress(value string) bool {
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch < 33 || ch > 126 {
			return false
		}
	}
	return true
}

func (s *AcmeService) resolveAcmeScript() (string, string, bool) {
	candidates := make([]string, 0, 10)
	saved := strings.TrimSpace(s.readSettingWithDefault(acmeScriptPathKey, ""))
	if saved != "" {
		candidates = append(candidates, saved)
	}
	candidates = append(candidates, filepath.Join(managedAcmeHomeDir(), "acme.sh"))
	candidates = append(candidates, filepath.Join(legacyManagedAcmeHomeDir(), "acme.sh"))
	if envHome := strings.TrimSpace(os.Getenv("HOME")); envHome != "" {
		candidates = append(candidates, filepath.Join(envHome, ".acme.sh", "acme.sh"))
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, ".acme.sh", "acme.sh"))
	}
	candidates = append(candidates, "/root/.acme.sh/acme.sh")
	candidates = append(candidates, "/.acme.sh/acme.sh")
	if lookPath, err := exec.LookPath("acme.sh"); err == nil {
		candidates = append(candidates, lookPath)
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		clean := filepath.Clean(candidate)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		if !pathExists(clean) {
			continue
		}
		homeDir := strings.TrimSpace(filepath.Dir(clean))
		if homeDir == "." || homeDir == "/" {
			homeDir = ""
		}
		return clean, homeDir, true
	}
	return "", "", false
}

func managedAcmeHomeDir() string {
	return filepath.Join(config.GetDataDir(), "acme")
}

func legacyManagedAcmeHomeDir() string {
	return filepath.Join(config.GetDataDir(), "acme", "home")
}

func managedAcmeWorkspaceParentDir() string {
	return filepath.Clean(filepath.Dir(managedAcmeHomeDir()))
}

func createManagedAcmeInstallWorkspace(prefix string) (string, func(), error) {
	parentDir := managedAcmeWorkspaceParentDir()
	if parentDir == "" || parentDir == "." {
		return "", nil, common.NewError("acme managed workspace parent directory is empty")
	}
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return "", nil, common.NewError("create acme managed workspace parent directory failed: ", err)
	}
	baseDir, err := os.MkdirTemp(parentDir, prefix)
	if err != nil {
		return "", nil, common.NewError("create acme managed workspace failed: ", err)
	}
	cleanup := func() {
		if err := os.RemoveAll(baseDir); err != nil && !os.IsNotExist(err) {
			logger.Warning("cleanup acme managed workspace failed: ", err)
		}
	}
	return baseDir, cleanup, nil
}

func cleanupStaleManagedAcmeInstallWorkspaces(parentDir string) error {
	parentDir = filepath.Clean(strings.TrimSpace(parentDir))
	if parentDir == "" || parentDir == "." {
		return nil
	}
	entries, err := os.ReadDir(parentDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return common.NewError("list acme managed workspaces failed: ", err)
	}
	for _, entry := range entries {
		if entry == nil || !entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if !isManagedAcmeWorkspaceName(name) {
			continue
		}
		target := filepath.Join(parentDir, name)
		if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
			return common.NewError("remove stale acme managed workspace failed: ", target, ": ", err)
		}
	}
	return nil
}

func isManagedAcmeWorkspaceName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	return strings.HasPrefix(name, acmeManagedWorkspaceStagePrefix) ||
		strings.HasPrefix(name, acmeManagedWorkspaceBackupPrefix)
}

func acmeManagedInstallRoots() []string {
	roots := []string{
		filepath.Clean(managedAcmeHomeDir()),
		filepath.Clean(legacyManagedAcmeHomeDir()),
	}
	result := make([]string, 0, len(roots))
	seen := make(map[string]struct{}, len(roots))
	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "" || root == "." {
			continue
		}
		if _, exists := seen[root]; exists {
			continue
		}
		seen[root] = struct{}{}
		result = append(result, root)
	}
	return result
}

func listManagedAcmeInstallEntryNames(root string) ([]string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return []string{}, nil
	}
	if !pathExists(root) {
		return []string{}, nil
	}

	names := make([]string, 0, len(acmeManagedRootFileNames)+len(acmeManagedRootDirNames))
	for name := range acmeManagedRootFileNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if pathExists(filepath.Join(root, name)) {
			names = append(names, name)
		}
	}
	for name := range acmeManagedRootDirNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if pathExists(filepath.Join(root, name)) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func rollbackManagedAcmeInstallActivation(targetHomeDir string, backupRoot string, movedNew []string, movedOld []string) error {
	targetHomeDir = filepath.Clean(strings.TrimSpace(targetHomeDir))
	backupRoot = filepath.Clean(strings.TrimSpace(backupRoot))
	var restoreErrs []string

	for i := len(movedNew) - 1; i >= 0; i-- {
		name := strings.TrimSpace(movedNew[i])
		if name == "" {
			continue
		}
		target := filepath.Join(targetHomeDir, name)
		if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
			restoreErrs = append(restoreErrs, fmt.Sprintf("remove new artifact %s failed: %v", target, err))
		}
	}

	if targetHomeDir != "" && targetHomeDir != "." {
		if err := os.MkdirAll(targetHomeDir, 0o755); err != nil {
			restoreErrs = append(restoreErrs, fmt.Sprintf("recreate managed home failed: %v", err))
		}
	}
	for i := len(movedOld) - 1; i >= 0; i-- {
		name := strings.TrimSpace(movedOld[i])
		if name == "" {
			continue
		}
		src := filepath.Join(backupRoot, name)
		if !pathExists(src) {
			continue
		}
		dst := filepath.Join(targetHomeDir, name)
		if err := os.Rename(src, dst); err != nil {
			restoreErrs = append(restoreErrs, fmt.Sprintf("restore old artifact %s failed: %v", dst, err))
		}
	}

	if len(restoreErrs) > 0 {
		return common.NewError(strings.Join(restoreErrs, "; "))
	}
	return nil
}

func (s *AcmeService) activateManagedAcmeInstallLocked(stagedHomeDir string) (string, error) {
	stagedHomeDir = filepath.Clean(strings.TrimSpace(stagedHomeDir))
	if stagedHomeDir == "" || stagedHomeDir == "." {
		return "", common.NewError("staged acme home directory is empty")
	}
	stageScriptPath := filepath.Join(stagedHomeDir, "acme.sh")
	if !pathExists(stageScriptPath) {
		return "", common.NewError("staged acme.sh script was not found")
	}

	targetHomeDir := filepath.Clean(managedAcmeHomeDir())
	if err := os.MkdirAll(targetHomeDir, 0o755); err != nil {
		return "", common.NewError("create managed acme home directory failed: ", err)
	}

	backupRoot, cleanupBackup, err := createManagedAcmeInstallWorkspace(acmeManagedWorkspaceBackupPrefix)
	if err != nil {
		return "", err
	}
	defer cleanupBackup()

	oldNames, err := listManagedAcmeInstallEntryNames(targetHomeDir)
	if err != nil {
		return "", err
	}
	movedOld := make([]string, 0, len(oldNames))
	for _, name := range oldNames {
		src := filepath.Join(targetHomeDir, name)
		dst := filepath.Join(backupRoot, name)
		if err := os.Rename(src, dst); err != nil {
			rollbackErr := rollbackManagedAcmeInstallActivation(targetHomeDir, backupRoot, nil, movedOld)
			if rollbackErr != nil {
				return "", common.NewError("backup current managed acme install failed: ", err, "; rollback failed: ", rollbackErr)
			}
			return "", common.NewError("backup current managed acme install failed: ", err)
		}
		movedOld = append(movedOld, name)
	}

	newNames, err := listManagedAcmeInstallEntryNames(stagedHomeDir)
	if err != nil {
		rollbackErr := rollbackManagedAcmeInstallActivation(targetHomeDir, backupRoot, nil, movedOld)
		if rollbackErr != nil {
			return "", common.NewError("list staged managed acme install failed: ", err, "; rollback failed: ", rollbackErr)
		}
		return "", common.NewError("list staged managed acme install failed: ", err)
	}
	if len(newNames) == 0 {
		rollbackErr := rollbackManagedAcmeInstallActivation(targetHomeDir, backupRoot, nil, movedOld)
		if rollbackErr != nil {
			return "", common.NewError("staged managed acme install is empty; rollback failed: ", rollbackErr)
		}
		return "", common.NewError("staged managed acme install is empty")
	}

	movedNew := make([]string, 0, len(newNames))
	for _, name := range newNames {
		src := filepath.Join(stagedHomeDir, name)
		dst := filepath.Join(targetHomeDir, name)
		if err := os.Rename(src, dst); err != nil {
			rollbackErr := rollbackManagedAcmeInstallActivation(targetHomeDir, backupRoot, movedNew, movedOld)
			if rollbackErr != nil {
				return "", common.NewError("activate staged acme install failed: ", err, "; rollback failed: ", rollbackErr)
			}
			return "", common.NewError("activate staged acme install failed: ", err)
		}
		movedNew = append(movedNew, name)
	}

	scriptPath := filepath.Clean(filepath.Join(targetHomeDir, "acme.sh"))
	if !pathExists(scriptPath) {
		rollbackErr := rollbackManagedAcmeInstallActivation(targetHomeDir, backupRoot, movedNew, movedOld)
		if rollbackErr != nil {
			return "", common.NewError("activated acme.sh script path was not found; rollback failed: ", rollbackErr)
		}
		return "", common.NewError("activated acme.sh script path was not found")
	}
	return scriptPath, nil
}

func acmeHomeArgs(homeDir string) []string {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return nil
	}
	return []string{"--home", homeDir}
}

func buildAcmeIssueCommandArgs(domains []string, challenge string, webroot string, dnsProvider string, keyLength string, caServer string, customArgs string, shortlivedProfile bool, ipFamilyMode acmeIPFamilyMode) []string {
	args := []string{"--issue"}
	for _, domain := range domains {
		args = append(args, "-d", domain)
	}

	switch challenge {
	case "webroot":
		if strings.TrimSpace(webroot) != "" {
			args = append(args, "--webroot", strings.TrimSpace(webroot))
		} else {
			args = append(args, "--standalone")
		}
	case "dns":
		if strings.TrimSpace(dnsProvider) != "" {
			args = append(args, "--dns", strings.TrimSpace(dnsProvider))
		} else {
			args = append(args, "--standalone")
		}
	case "alpn":
		args = append(args, "--alpn")
	default:
		args = append(args, "--standalone")
	}

	keyLength = normalizeAcmeKeyLength(keyLength)
	if keyLength != "" {
		args = append(args, "--keylength", keyLength)
	}
	caServer = normalizeAcmeServer(caServer)
	if caServer != "" {
		args = append(args, "--server", caServer)
	}
	if shortlivedProfile {
		args = append(args, "--cert-profile", "shortlived")
	}
	if shortlivedProfile {
		appendAcmeIPFamilyListenArgs(&args, ipFamilyMode)
	}

	customArgs = strings.TrimSpace(customArgs)
	if customArgs != "" {
		args = append(args, strings.Fields(customArgs)...)
	}
	return args
}

func normalizeAcmeDomains(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return []string{}
	}

	seen := map[string]struct{}{}
	domains := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.ToLower(strings.TrimSpace(field))
		value = strings.Trim(value, ".")
		if value == "" {
			continue
		}
		if strings.Contains(value, "/") {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		domains = append(domains, value)
	}
	return domains
}

func normalizeAcmeIssueIdentifiers(text string, certificateType string) []string {
	if certificateType == acmeCertificateTypeIP {
		return normalizeAcmeIPIdentifiers(text)
	}
	return normalizeAcmeDomains(text)
}

func normalizeAcmeIPIdentifiers(text string) []string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		addr, ok := normalizeAcmeIPAddressToken(field)
		if !ok {
			continue
		}
		if _, exists := seen[addr]; exists {
			continue
		}
		seen[addr] = struct{}{}
		result = append(result, addr)
	}
	return result
}

func normalizeAcmeIPAddressToken(value string) (string, bool) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	if value == "" || strings.Contains(value, "/") {
		return "", false
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return "", false
	}
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return "", false
	}
	return addr.Unmap().String(), true
}

func detectAcmeIPFamilyMode(domains []string) acmeIPFamilyMode {
	hasIPv4 := false
	hasIPv6 := false
	for _, domain := range domains {
		addr, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(domain), "[]"))
		if err != nil {
			continue
		}
		if addr.Is4() || addr.Is4In6() {
			hasIPv4 = true
			continue
		}
		if addr.Is6() {
			hasIPv6 = true
		}
	}
	switch {
	case hasIPv4 && hasIPv6:
		return acmeIPFamilyDual
	case hasIPv6:
		return acmeIPFamilyIPv6
	case hasIPv4:
		return acmeIPFamilyIPv4
	default:
		return acmeIPFamilyUnknown
	}
}

func appendAcmeIPFamilyListenArgs(args *[]string, mode acmeIPFamilyMode) {
	if args == nil {
		return
	}
	switch mode {
	case acmeIPFamilyIPv4:
		*args = append(*args, "--listen-v4")
	case acmeIPFamilyIPv6:
		*args = append(*args, "--listen-v6")
	}
}

func acmeIPFamilyModeLabel(mode acmeIPFamilyMode) string {
	switch mode {
	case acmeIPFamilyIPv4:
		return "纯 IPv4"
	case acmeIPFamilyIPv6:
		return "纯 IPv6"
	case acmeIPFamilyDual:
		return "混合 IPv4/IPv6"
	default:
		return "未知"
	}
}

func logAcmeIPFamilyListenStrategy(logSession *acmeLogSession, mode acmeIPFamilyMode) {
	if logSession == nil {
		return
	}
	switch mode {
	case acmeIPFamilyIPv4:
		logSession.append("本次将显式追加 acme.sh 参数: --listen-v4")
	case acmeIPFamilyIPv6:
		logSession.append("本次将显式追加 acme.sh 参数: --listen-v6")
	case acmeIPFamilyDual:
		logSession.append("检测到混合 IPv4/IPv6 IP，本次不强制单一监听族；请确认 dual-stack 外部可达")
	default:
		logSession.append("IP family could not be identified, skip acme.sh listen-family arguments")
	}
}

func normalizeAcmeCertificateType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case acmeCertificateTypeIP, "ipcert", "ip_certificate":
		return acmeCertificateTypeIP
	default:
		return acmeCertificateTypeDomain
	}
}

func normalizeAcmeIPChallenge(value string) string {
	switch normalizeAcmeChallenge(value) {
	case "standalone":
		return "standalone"
	case "alpn":
		return "alpn"
	default:
		return ""
	}
}

func acmeRequiredPortForChallenge(challenge string) (int, error) {
	switch normalizeAcmeChallenge(challenge) {
	case "standalone":
		return acmeIPCertificatePortHTTP, nil
	case "alpn":
		return acmeIPCertificatePortALPN, nil
	default:
		return 0, common.NewError("IP 证书只能使用 HTTP Standalone 或 TLS ALPN 验证")
	}
}

func normalizeAcmeChallenge(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "standalone":
		return "standalone"
	case "webroot":
		return "webroot"
	case "dns":
		return "dns"
	case "alpn":
		return "alpn"
	default:
		return ""
	}
}

func shouldUseAcmeDNSChallenge(certificateType string, challenge string) bool {
	if normalizeAcmeCertificateType(certificateType) == acmeCertificateTypeIP {
		return false
	}
	return normalizeAcmeChallenge(challenge) == "dns"
}

func shouldBindAcmeAccount(certificateType string) bool {
	return normalizeAcmeCertificateType(certificateType) == acmeCertificateTypeDomain
}

func applyAcmeAccountBinding(entry *model.AcmeCertificate, certificateType string, accountID uint, accountName string) {
	if entry == nil {
		return
	}
	if shouldBindAcmeAccount(certificateType) {
		entry.AcmeAccountID = accountID
		entry.AcmeAccountName = strings.TrimSpace(accountName)
		return
	}
	entry.AcmeAccountID = 0
	entry.AcmeAccountName = ""
}

func normalizeAcmeServer(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, "/")
	if value == "" {
		return ""
	}
	switch value {
	case "let", "le":
		return "letsencrypt"
	case "zero":
		return "zerossl"
	}
	return value
}

func isSupportedAcmeDomainServer(value string) bool {
	normalized := normalizeAcmeServer(value)
	switch normalized {
	case "letsencrypt", "zerossl":
		return true
	case strings.TrimSuffix(strings.ToLower(acmeLEProductionDirectory), "/"), strings.TrimSuffix(strings.ToLower(acmeLEStagingDirectory), "/"), strings.TrimSuffix(strings.ToLower(acmeZeroSSLDirectory), "/"):
		return true
	default:
		return false
	}
}

func normalizeSupportedAcmeDomainServer(value string) string {
	normalized := normalizeAcmeServer(value)
	switch normalized {
	case "letsencrypt", strings.TrimSuffix(strings.ToLower(acmeLEProductionDirectory), "/"), strings.TrimSuffix(strings.ToLower(acmeLEStagingDirectory), "/"):
		return "letsencrypt"
	case "zerossl", strings.TrimSuffix(strings.ToLower(acmeZeroSSLDirectory), "/"):
		return "zerossl"
	default:
		return ""
	}
}

func normalizeAcmeKeyLength(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	switch value {
	case "2048", "3072", "4096", "8192", "ec-256", "ec-384", "ec-521":
		return value
	default:
		return ""
	}
}

func normalizeAcmeApplyTarget(value string) (PanelSelfSignedTarget, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "panel":
		return PanelSelfSignedTargetPanel, true
	case "sub":
		return PanelSelfSignedTargetSub, true
	default:
		return "", false
	}
}

func formatAssignedApplyTarget(targets []PanelSelfSignedTarget) string {
	hasPanel := false
	hasSub := false
	for _, target := range targets {
		switch target {
		case PanelSelfSignedTargetPanel:
			hasPanel = true
		case PanelSelfSignedTargetSub:
			hasSub = true
		}
	}
	switch {
	case hasPanel && hasSub:
		return "panel,sub"
	case hasPanel:
		return "panel"
	case hasSub:
		return "sub"
	default:
		return ""
	}
}

func normalizeAcmeEnvAssignments(raw string) ([]string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	envPairs := make([]string, 0, len(lines))
	seen := map[string]struct{}{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !acmeEnvPattern.MatchString(line) {
			return nil, common.NewError("invalid env line: ", line)
		}
		key := line
		if idx := strings.Index(line, "="); idx >= 0 {
			key = strings.TrimSpace(line[:idx])
		}
		if key == "" {
			return nil, common.NewError("invalid env line: ", line)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		envPairs = append(envPairs, line)
	}
	return envPairs, nil
}

func createAcmeTempInstallPaths(mainDomain string) (*acmeManagedCertPaths, func(), error) {
	prefix := "issue-"
	if normalized := sanitizeAcmeTempName(mainDomain); normalized != "" {
		prefix = normalized + "-"
	}
	baseDir, err := os.MkdirTemp("", "sui-acme-"+prefix)
	if err != nil {
		return nil, nil, common.NewError("create acme temp directory failed: ", err)
	}
	paths := &acmeManagedCertPaths{
		CertPath:      filepath.Join(baseDir, "cert.pem"),
		KeyPath:       filepath.Join(baseDir, "key.pem"),
		FullchainPath: filepath.Join(baseDir, "fullchain.pem"),
		ChainPath:     filepath.Join(baseDir, "chain.pem"),
		BaseDir:       baseDir,
	}
	cleanup := func() {
		if strings.TrimSpace(baseDir) == "" {
			return
		}
		if err := os.RemoveAll(baseDir); err != nil && !os.IsNotExist(err) {
			logger.Warning("cleanup acme temp install dir failed: ", err)
		}
	}
	return paths, cleanup, nil
}

func sanitizeAcmeTempName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Trim(value, ".")
	if value == "" {
		return ""
	}
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, value)
	safe = strings.Trim(safe, "-")
	if safe == "" {
		return ""
	}
	if len(safe) > 48 {
		safe = safe[:48]
	}
	return safe
}

func readCertificateBundle(paths *acmeManagedCertPaths) ([]byte, []byte, []byte, []byte, error) {
	if paths == nil {
		return nil, nil, nil, nil, common.NewError("certificate paths are nil")
	}
	certPEM, err := os.ReadFile(paths.CertPath)
	if err != nil {
		return nil, nil, nil, nil, common.NewError("read cert.pem failed: ", err)
	}
	keyPEM, err := os.ReadFile(paths.KeyPath)
	if err != nil {
		return nil, nil, nil, nil, common.NewError("read key.pem failed: ", err)
	}
	fullchainPEM, err := os.ReadFile(paths.FullchainPath)
	if err != nil {
		return nil, nil, nil, nil, common.NewError("read fullchain.pem failed: ", err)
	}

	chainPEM := []byte{}
	if pathExists(paths.ChainPath) {
		chainPEM, err = os.ReadFile(paths.ChainPath)
		if err != nil {
			return nil, nil, nil, nil, common.NewError("read chain.pem failed: ", err)
		}
	}

	return certPEM, keyPEM, fullchainPEM, chainPEM, nil
}

func cleanupAcmeWorkingTree(homeDir string, mainDomain string, useECC bool) {
	homeDir = strings.TrimSpace(homeDir)
	mainDomain = strings.TrimSpace(mainDomain)
	if homeDir == "" || mainDomain == "" {
		return
	}
	candidates := []string{}
	if useECC {
		candidates = append(candidates, filepath.Join(homeDir, mainDomain+"_ecc"))
	} else {
		candidates = append(candidates,
			filepath.Join(homeDir, mainDomain),
			filepath.Join(homeDir, mainDomain+"_rsa"),
		)
	}
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if candidate == "" || candidate == "." || candidate == "/" {
			continue
		}
		if err := os.RemoveAll(candidate); err != nil && !os.IsNotExist(err) {
			logger.Warning("cleanup acme working tree failed: ", candidate, ": ", err)
		}
	}
}

func inspectCertificateFingerprint(certPEM []byte, keyPEM []byte) (string, time.Time, time.Time, error) {
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return "", time.Time{}, time.Time{}, common.NewError("parse certificate/key failed: ", err)
	}
	leaf, err := network.ParseLeafCertificate(&pair)
	if err != nil {
		return "", time.Time{}, time.Time{}, common.NewError("parse leaf certificate failed: ", err)
	}
	sum := sha256.Sum256(leaf.Raw)
	return hex.EncodeToString(sum[:]), leaf.NotBefore, leaf.NotAfter, nil
}

func convertAcmeCertificate(entry *model.AcmeCertificate) AcmeCertificateView {
	if entry == nil {
		return AcmeCertificateView{}
	}
	row, err := upsertInventoryFromAcme(entry)
	if err != nil {
		return AcmeCertificateView{}
	}
	return convertCertificateRecord(row)
}

type certificateBundleFile struct {
	Name string
	Data []byte
	Perm os.FileMode
}

func certificateBundleFiles(certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) ([]certificateBundleFile, error) {
	if len(certPEM) == 0 || len(keyPEM) == 0 || len(fullchainPEM) == 0 {
		return nil, common.NewError("certificate bundle is incomplete")
	}
	files := []certificateBundleFile{
		{Name: "cert.pem", Data: certPEM, Perm: 0o644},
		{Name: "key.pem", Data: keyPEM, Perm: 0o600},
		{Name: "fullchain.pem", Data: fullchainPEM, Perm: 0o644},
	}
	if len(chainPEM) > 0 {
		files = append(files, certificateBundleFile{Name: "chain.pem", Data: chainPEM, Perm: 0o644})
	}
	return files, nil
}

func sameCleanPath(a string, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	return filepath.Clean(strings.TrimSpace(a)) == filepath.Clean(strings.TrimSpace(b))
}

func isAcmeIPCertificate(entry *model.AcmeCertificate) bool {
	if entry == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(entry.CertificateType), acmeCertificateTypeIP) {
		return true
	}
	if strings.TrimSpace(entry.CertificateType) != "" {
		return false
	}
	if strings.TrimSpace(entry.CertProfile) != "" && !strings.EqualFold(strings.TrimSpace(entry.CertProfile), "shortlived") {
		return false
	}
	domains := decodeCertificateDomains(entry.DomainSet)
	if len(domains) == 0 && strings.TrimSpace(entry.MainDomain) != "" {
		domains = []string{strings.TrimSpace(entry.MainDomain)}
	}
	if len(domains) == 0 {
		return false
	}
	for _, domain := range domains {
		if _, ok := normalizeAcmeIPAddressToken(domain); !ok {
			return false
		}
	}
	return true
}

func acmeCertificateTypeForEntry(entry *model.AcmeCertificate) string {
	if isAcmeIPCertificate(entry) {
		return acmeCertificateTypeIP
	}
	return acmeCertificateTypeDomain
}

func acmeCertProfileForType(certificateType string) string {
	if normalizeAcmeCertificateType(certificateType) == acmeCertificateTypeIP {
		return "shortlived"
	}
	return ""
}

func normalizeDefaultPushParentDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	if strings.Contains(dir, "/") {
		cleaned := path.Clean(dir)
		parent := path.Dir(cleaned)
		if parent == "." || parent == cleaned {
			return cleaned
		}
		return parent
	}
	cleaned := filepath.Clean(dir)
	parent := filepath.Dir(cleaned)
	if parent == "." || parent == cleaned {
		return cleaned
	}
	return parent
}

func cleanupLegacyCertificateManagedDirs() error {
	if err := cleanupLegacyCertificateManagedDir(
		filepath.Join(config.GetDataDir(), "acme", "live"),
		map[string]struct{}{"cert.pem": {}, "key.pem": {}, "fullchain.pem": {}, "chain.pem": {}},
		false,
	); err != nil {
		return err
	}
	if err := cleanupLegacyCertificateManagedDir(
		filepath.Join(config.GetDataDir(), "self_signed", "live"),
		map[string]struct{}{"cert.pem": {}, "key.pem": {}, "fullchain.pem": {}, "chain.pem": {}},
		false,
	); err != nil {
		return err
	}
	if err := cleanupLegacyCertificateManagedDir(
		filepath.Join(config.GetDataDir(), "acme", "tmp-install"),
		map[string]struct{}{"cert.pem": {}, "key.pem": {}, "fullchain.pem": {}, "chain.pem": {}},
		true,
	); err != nil {
		return err
	}
	if err := cleanupStaleManagedAcmeInstallWorkspaces(managedAcmeWorkspaceParentDir()); err != nil {
		return err
	}
	if err := cleanupObsoleteLegacyManagedAcmeInstallRoot(); err != nil {
		return err
	}
	return nil
}

func cleanupObsoleteLegacyManagedAcmeInstallRoot() error {
	currentScript := filepath.Join(managedAcmeHomeDir(), "acme.sh")
	if !pathExists(currentScript) {
		return nil
	}
	legacyRoot := filepath.Clean(legacyManagedAcmeHomeDir())
	if legacyRoot == "" || legacyRoot == "." || !pathExists(legacyRoot) {
		return nil
	}
	return removeManagedInstallArtifactsAtRoot(legacyRoot, true)
}

func cleanupLegacyCertificateManagedDir(root string, whitelist map[string]struct{}, removeUnknown bool) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	root = filepath.Clean(root)
	if !pathExists(root) {
		return nil
	}

	rootSlash := filepath.ToSlash(root)
	var cleanOne func(string) (bool, error)
	cleanOne = func(dir string) (bool, error) {
		dir = filepath.Clean(dir)
		dirSlash := filepath.ToSlash(dir)
		if dirSlash != rootSlash && !strings.HasPrefix(dirSlash, rootSlash+"/") {
			return false, common.NewError("cleanup path escapes managed root: ", dir)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return true, nil
			}
			return false, err
		}
		empty := true
		for _, entry := range entries {
			name := strings.TrimSpace(entry.Name())
			if name == "" {
				empty = false
				continue
			}
			target := filepath.Join(dir, name)
			if entry.IsDir() {
				childEmpty, childErr := cleanOne(target)
				if childErr != nil {
					return false, childErr
				}
				if childEmpty {
					if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
						return false, err
					}
					continue
				}
				empty = false
				continue
			}
			if _, ok := whitelist[name]; ok {
				if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
					return false, err
				}
				continue
			}
			if removeUnknown {
				if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
					return false, err
				}
				continue
			}
			logger.Warning("skip unknown file while cleaning legacy certificate dir: ", target)
			empty = false
		}
		return empty, nil
	}

	isEmpty, err := cleanOne(root)
	if err != nil {
		return err
	}
	if isEmpty {
		_ = os.Remove(root)
	}
	return nil
}

func removeCertificateBundleFromDirectory(targetDir string) error {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return nil
	}
	for _, name := range []string{"cert.pem", "key.pem", "fullchain.pem", "chain.pem"} {
		path := filepath.Join(targetDir, name)
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return common.NewError("delete ", name, " failed: ", err)
		}
	}
	return nil
}

func parseTrackedPushFiles(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed := make([]string, 0)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}
	return normalizeTrackedPushFiles(parsed)
}

func normalizeTrackedPushFiles(files []string) []string {
	if len(files) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(files))
	for _, item := range files {
		name := strings.TrimSpace(filepath.Base(strings.TrimSpace(item)))
		if name == "" || name == "." || name == ".." {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func encodeTrackedPushFiles(files []string) string {
	files = normalizeTrackedPushFiles(files)
	if len(files) == 0 {
		return ""
	}
	raw, err := json.Marshal(files)
	if err != nil {
		return ""
	}
	return string(raw)
}

func removeTrackedCertificateFilesFromDirectory(targetDir string, tracked []string) error {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return nil
	}
	tracked = normalizeTrackedPushFiles(tracked)
	if len(tracked) == 0 {
		return removeCertificateBundleFromDirectory(targetDir)
	}
	for _, name := range tracked {
		path := filepath.Join(targetDir, name)
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return common.NewError("delete ", name, " failed: ", err)
		}
	}
	return nil
}

func writeCertificateToDirectory(targetDir string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) error {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return common.NewError("target directory is empty")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return common.NewError("create target directory failed: ", err)
	}

	writeTargets, err := certificateBundleFiles(certPEM, keyPEM, fullchainPEM, chainPEM)
	if err != nil {
		return err
	}

	for _, target := range writeTargets {
		path := filepath.Join(targetDir, target.Name)
		if err := os.WriteFile(path, target.Data, target.Perm); err != nil {
			return common.NewError("write ", target.Name, " failed: ", err)
		}
	}
	return nil
}

func writeCertificateToDirectoryTracked(targetDir string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) ([]string, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return nil, common.NewError("target directory is empty")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, common.NewError("create target directory failed: ", err)
	}
	writeTargets, err := certificateBundleFiles(certPEM, keyPEM, fullchainPEM, chainPEM)
	if err != nil {
		return nil, err
	}
	tracked := make([]string, 0, len(writeTargets))
	for _, target := range writeTargets {
		path := filepath.Join(targetDir, target.Name)
		if err := os.WriteFile(path, target.Data, target.Perm); err != nil {
			return nil, common.NewError("write ", target.Name, " failed: ", err)
		}
		tracked = append(tracked, target.Name)
	}
	return normalizeTrackedPushFiles(tracked), nil
}

func verifyCertificateDirectoryContent(targetDir string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) error {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return common.NewError("target directory is empty")
	}

	writeTargets, err := certificateBundleFiles(certPEM, keyPEM, fullchainPEM, chainPEM)
	if err != nil {
		return err
	}

	for _, target := range writeTargets {
		path := filepath.Join(targetDir, target.Name)
		actual, readErr := os.ReadFile(path)
		if readErr != nil {
			return common.NewError("read ", target.Name, " for verify failed: ", readErr)
		}
		if !bytes.Equal(actual, target.Data) {
			return common.NewError("verify ", target.Name, " failed: file content does not match latest certificate")
		}
	}

	// If chain is empty for this cert, cleanup stale chain file and ensure it is absent.
	if len(chainPEM) == 0 {
		chainPath := filepath.Join(targetDir, "chain.pem")
		if err := os.Remove(chainPath); err != nil && !os.IsNotExist(err) {
			return common.NewError("delete stale chain.pem failed: ", err)
		}
	}

	return nil
}

func replaceCertificateInDirectory(targetDir string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) error {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return common.NewError("target directory is empty")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return common.NewError("create target directory failed: ", err)
	}
	if err := removeCertificateBundleFromDirectory(targetDir); err != nil {
		return err
	}
	if err := writeCertificateToDirectory(targetDir, certPEM, keyPEM, fullchainPEM, chainPEM); err != nil {
		return err
	}
	if err := verifyCertificateDirectoryContent(targetDir, certPEM, keyPEM, fullchainPEM, chainPEM); err != nil {
		return err
	}
	return nil
}

func replaceCertificateInDirectoryWithTrackedFiles(targetDir string, oldTracked []string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) ([]string, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return nil, common.NewError("target directory is empty")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return nil, common.NewError("create target directory failed: ", err)
	}
	if err := removeTrackedCertificateFilesFromDirectory(targetDir, oldTracked); err != nil {
		return nil, err
	}
	written, err := writeCertificateToDirectoryTracked(targetDir, certPEM, keyPEM, fullchainPEM, chainPEM)
	if err != nil {
		return nil, err
	}
	if err := verifyCertificateDirectoryContent(targetDir, certPEM, keyPEM, fullchainPEM, chainPEM); err != nil {
		return nil, err
	}
	return written, nil
}

func lookupAcmeDNSProvider(code string) (AcmeDNSProviderMeta, bool) {
	code = strings.TrimSpace(code)
	for _, item := range defaultAcmeDNSProviderCatalog {
		if strings.EqualFold(item.ProviderCode, code) {
			return item, true
		}
	}
	return AcmeDNSProviderMeta{}, false
}

func parseAcmeEnvJSON(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	mapped := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &mapped); err != nil {
		return nil, common.NewError("dns 账号 env_json 格式错误: ", err)
	}
	result := make(map[string]string, len(mapped))
	for key, value := range mapped {
		trimKey := strings.TrimSpace(key)
		if trimKey == "" {
			continue
		}
		trimValue := strings.TrimSpace(value)
		if trimValue == "" {
			continue
		}
		result[trimKey] = trimValue
	}
	return result, nil
}

func resolveAcmeDNSProviderFromAccount(dnsAccountID uint, fallback string) (string, error) {
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		return fallback, nil
	}
	if dnsAccountID == 0 {
		return "", nil
	}
	dnsAccount := &model.AcmeDNSAccount{}
	if err := database.GetDB().Where("id = ?", dnsAccountID).First(dnsAccount).Error; err != nil {
		return "", err
	}
	return strings.TrimSpace(dnsAccount.ProviderCode), nil
}

func resolveAcmeDNSRuntimeEnv(dnsAccountID uint, dnsEnvText string) ([]string, error) {
	dnsEnvText = strings.TrimSpace(dnsEnvText)
	dnsEnv := []string{}
	if dnsAccountID > 0 {
		dnsAccount := &model.AcmeDNSAccount{}
		if err := database.GetDB().Where("id = ?", dnsAccountID).First(dnsAccount).Error; err == nil {
			envMap, parseErr := parseAcmeEnvJSON(dnsAccount.EnvJSON)
			if parseErr != nil {
				return nil, parseErr
			}
			dnsEnv = envMapToEnvPairs(envMap)
		}
	}
	if dnsEnvText == "" {
		return dnsEnv, nil
	}
	parsedEnv, err := normalizeAcmeEnvAssignments(dnsEnvText)
	if err != nil {
		return nil, err
	}
	if len(dnsEnv) > 0 {
		parsedEnv = mergeEnvPairs(dnsEnv, parsedEnv)
	}
	return parsedEnv, nil
}

func isMaskedAcmeEnvValue(value string) bool {
	return strings.TrimSpace(value) == acmeMaskedEnvValue
}

func mergeAcmeDNSAccountEnv(existing map[string]string, incoming map[string]string) map[string]string {
	result := make(map[string]string, len(incoming))
	for key, value := range incoming {
		trimKey := strings.TrimSpace(key)
		if trimKey == "" {
			continue
		}
		trimValue := strings.TrimSpace(value)
		if trimValue == "" {
			continue
		}
		if isMaskedAcmeEnvValue(trimValue) {
			if oldValue := strings.TrimSpace(existing[trimKey]); oldValue != "" {
				result[trimKey] = oldValue
			}
			continue
		}
		result[trimKey] = trimValue
	}
	return result
}

func sanitizeDNSAccountEnvForProvider(provider AcmeDNSProviderMeta, env map[string]string) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}

	providerKeys := map[string]struct{}{}
	for _, field := range provider.Fields {
		key := strings.TrimSpace(field.Key)
		if key == "" {
			continue
		}
		providerKeys[key] = struct{}{}
	}

	knownProviderKeys := map[string]struct{}{}
	for _, item := range defaultAcmeDNSProviderCatalog {
		for _, field := range item.Fields {
			key := strings.TrimSpace(field.Key)
			if key == "" {
				continue
			}
			knownProviderKeys[key] = struct{}{}
		}
	}

	result := make(map[string]string, len(env))
	for key, value := range env {
		trimKey := strings.TrimSpace(key)
		trimValue := strings.TrimSpace(value)
		if trimKey == "" || trimValue == "" {
			continue
		}
		if _, ok := providerKeys[trimKey]; ok {
			result[trimKey] = trimValue
			continue
		}
		if _, known := knownProviderKeys[trimKey]; known {
			continue
		}
		result[trimKey] = trimValue
	}
	return result
}

func sanitizeAcmeEnvMap(env map[string]string) map[string]string {
	if len(env) == 0 {
		return map[string]string{}
	}
	result := make(map[string]string, len(env))
	for key, value := range env {
		trimKey := strings.TrimSpace(key)
		trimValue := strings.TrimSpace(value)
		if trimKey == "" || trimValue == "" {
			continue
		}
		if isAcmeSecretEnvKey(trimKey) {
			result[trimKey] = acmeMaskedEnvValue
			continue
		}
		result[trimKey] = trimValue
	}
	return result
}

func isAcmeSecretEnvKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "private_key") ||
		strings.Contains(key, "access_key") ||
		strings.Contains(key, "api_key") ||
		strings.HasSuffix(key, "_key") ||
		strings.HasSuffix(key, "_key_id") ||
		strings.HasSuffix(key, "_secret")
}

func validateDNSProviderEnv(provider AcmeDNSProviderMeta, env map[string]string) error {
	trim := func(key string) string {
		return strings.TrimSpace(env[key])
	}
	switch strings.ToLower(strings.TrimSpace(provider.ProviderCode)) {
	case "dns_cf":
		token := trim("CF_Token")
		accountID := trim("CF_Account_ID")
		zoneID := trim("CF_Zone_ID")
		email := trim("CF_Email")
		key := trim("CF_Key")
		tokenMode := token != "" && (accountID != "" || zoneID != "")
		legacyMode := email != "" && key != ""
		if !tokenMode && !legacyMode {
			return common.NewError("Cloudflare DNS 需填写以下其一：CF_Token + (CF_Account_ID 或 CF_Zone_ID)，或 CF_Email + CF_Key")
		}
		return nil
	case "dns_aws":
		accessKeyID := trim("AWS_ACCESS_KEY_ID")
		secretAccessKey := trim("AWS_SECRET_ACCESS_KEY")
		if accessKeyID == "" && secretAccessKey == "" {
			return nil
		}
		if accessKeyID == "" || secretAccessKey == "" {
			return common.NewError("AWS DNS 静态凭据模式需同时填写 AWS_ACCESS_KEY_ID 与 AWS_SECRET_ACCESS_KEY；若使用 IAM Role 可留空这两项")
		}
		return nil
	}
	for _, field := range provider.Fields {
		if !field.Required {
			continue
		}
		if strings.TrimSpace(env[field.Key]) == "" {
			return common.NewError("缺少必填 DNS 参数: ", field.Label, " (", field.Key, ")")
		}
	}
	return nil
}

func (s *AcmeService) migrateLegacyDNSSecretsFromAccountConf() error {
	acmeLegacyDNSMu.Lock()
	defer acmeLegacyDNSMu.Unlock()

	_, homeDir, installed := s.resolveAcmeScript()
	if !installed && strings.TrimSpace(homeDir) == "" {
		savedPath := strings.TrimSpace(s.readSettingWithDefault(acmeScriptPathKey, ""))
		if savedPath != "" {
			homeDir = strings.TrimSpace(filepath.Dir(savedPath))
		}
	}
	homeCandidates := []string{
		strings.TrimSpace(homeDir),
		managedAcmeHomeDir(),
		legacyManagedAcmeHomeDir(),
	}
	finalHome := ""
	for _, candidate := range homeCandidates {
		cleaned := strings.TrimSpace(filepath.Clean(candidate))
		if cleaned == "" || cleaned == "." {
			continue
		}
		if pathExists(filepath.Join(cleaned, "account.conf")) {
			finalHome = cleaned
			break
		}
	}
	if finalHome == "" {
		return nil
	}

	candidates, err := loadLegacyDNSCandidatesFromAccountConf(finalHome)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	return s.persistLegacyDNSCandidates(finalHome, candidates)
}

func loadLegacyDNSCandidatesFromAccountConf(homeDir string) ([]acmeLegacyDNSCandidate, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return []acmeLegacyDNSCandidate{}, nil
	}
	confPath := filepath.Join(homeDir, "account.conf")
	raw, err := os.ReadFile(confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []acmeLegacyDNSCandidate{}, nil
		}
		return nil, common.NewError("read account.conf failed: ", err)
	}

	lines := strings.Split(strings.ReplaceAll(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\r", "\n"), "\n")
	envMap := map[string]string{}
	for _, line := range lines {
		key := parseAcmeEnvLineKey(line)
		if key == "" {
			continue
		}
		val := parseAcmeEnvLineValue(line)
		if val == "" {
			continue
		}
		envMap[key] = val
	}
	return buildLegacyDNSCandidatesFromEnvMap(envMap), nil
}

func buildLegacyDNSCandidatesFromEnvMap(envMap map[string]string) []acmeLegacyDNSCandidate {
	candidates := make([]acmeLegacyDNSCandidate, 0, len(defaultAcmeDNSProviderCatalog))
	for _, provider := range defaultAcmeDNSProviderCatalog {
		if strings.TrimSpace(provider.ProviderCode) == "" {
			continue
		}
		candidateEnv := map[string]string{}
		for _, field := range provider.Fields {
			key := strings.TrimSpace(field.Key)
			if key == "" {
				continue
			}
			value := strings.TrimSpace(envMap[key])
			if value == "" {
				value = strings.TrimSpace(envMap["SAVED_"+key])
			}
			if value == "" {
				continue
			}
			candidateEnv[key] = value
		}
		if len(candidateEnv) == 0 {
			continue
		}
		if err := validateDNSProviderEnv(provider, candidateEnv); err != nil {
			continue
		}
		candidates = append(candidates, acmeLegacyDNSCandidate{
			provider: provider,
			env:      candidateEnv,
		})
	}
	return candidates
}

func parseAcmeEnvLineValue(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	if strings.HasPrefix(line, "export ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	}
	idx := strings.Index(line, "=")
	if idx <= 0 || idx >= len(line)-1 {
		return ""
	}
	value := strings.TrimSpace(line[idx+1:])
	value = strings.Trim(value, "\"'")
	return strings.TrimSpace(value)
}

func (s *AcmeService) persistLegacyDNSCandidates(homeDir string, candidates []acmeLegacyDNSCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	db := database.GetDB()
	for _, item := range candidates {
		existing := &model.AcmeDNSAccount{}
		err := db.Where("provider_code = ?", item.provider.ProviderCode).First(existing).Error
		if err == nil {
			continue
		}
		if !database.IsNotFound(err) {
			return err
		}
		envRaw, marshalErr := json.Marshal(item.env)
		if marshalErr != nil {
			return marshalErr
		}
		row := &model.AcmeDNSAccount{
			Name:         "legacy-" + item.provider.ProviderCode,
			ProviderName: item.provider.Name,
			ProviderCode: item.provider.ProviderCode,
			EnvJSON:      string(envRaw),
			Remark:       "migrated from account.conf",
		}
		if createErr := db.Create(row).Error; createErr != nil {
			return createErr
		}
	}

	secretPairs := envMapToEnvPairs(collectLegacySecretEnvPairs(candidates))
	if len(secretPairs) > 0 {
		if _, err := stripAcmeAccountConfSecrets(homeDir, secretPairs); err != nil {
			return err
		}
	}
	return nil
}

func collectLegacySecretEnvPairs(candidates []acmeLegacyDNSCandidate) map[string]string {
	merged := map[string]string{}
	for _, item := range candidates {
		for key, value := range item.env {
			trimKey := strings.TrimSpace(key)
			trimValue := strings.TrimSpace(value)
			if trimKey == "" || trimValue == "" {
				continue
			}
			merged[trimKey] = trimValue
		}
	}
	return merged
}

func (s *AcmeService) cleanupAcmeAccountConfSecrets(homeDir string, envPairs []string, logSession *acmeLogSession) {
	removedCount, err := stripAcmeAccountConfSecrets(homeDir, envPairs)
	if err != nil {
		if logSession != nil {
			logSession.append("清理 account.conf 中的 DNS 凭据失败: " + err.Error())
		}
		return
	}
	if removedCount > 0 && logSession != nil {
		logSession.append(fmt.Sprintf("已从 account.conf 清理 %d 项 DNS 凭据", removedCount))
	}
}

func stripAcmeAccountConfSecrets(homeDir string, envPairs []string) (int, error) {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return 0, nil
	}
	keys := buildAcmeSecretEnvKeySet(envPairs)
	if len(keys) == 0 {
		return 0, nil
	}

	confPath := filepath.Join(homeDir, "account.conf")
	raw, err := os.ReadFile(confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, common.NewError("read account.conf failed: ", err)
	}

	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	keptLines := make([]string, 0, len(lines))
	removedCount := 0

	for _, line := range lines {
		key := parseAcmeEnvLineKey(line)
		if key != "" {
			if _, ok := keys[key]; ok {
				removedCount++
				continue
			}
		}
		keptLines = append(keptLines, line)
	}

	if removedCount == 0 {
		return 0, nil
	}

	output := strings.Join(keptLines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	mode := os.FileMode(0o600)
	if info, statErr := os.Stat(confPath); statErr == nil {
		mode = info.Mode().Perm()
	}

	tmpFile, err := os.CreateTemp(homeDir, "account.conf.clean-*")
	if err != nil {
		return 0, common.NewError("create account.conf temp file failed: ", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.WriteString(output); err != nil {
		_ = tmpFile.Close()
		return 0, common.NewError("write account.conf temp file failed: ", err)
	}
	if err := tmpFile.Chmod(mode); err != nil {
		_ = tmpFile.Close()
		return 0, common.NewError("chmod account.conf temp file failed: ", err)
	}
	if err := tmpFile.Close(); err != nil {
		return 0, common.NewError("close account.conf temp file failed: ", err)
	}
	if err := os.Rename(tmpPath, confPath); err != nil {
		return 0, common.NewError("replace account.conf failed: ", err)
	}
	return removedCount, nil
}

func buildAcmeSecretEnvKeySet(envPairs []string) map[string]struct{} {
	result := make(map[string]struct{}, len(envPairs)*2)
	for _, pair := range envPairs {
		key := parseAcmeEnvLineKey(pair)
		if key == "" {
			continue
		}
		result[key] = struct{}{}
		result["SAVED_"+key] = struct{}{}
	}
	return result
}

func parseAcmeEnvLineKey(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ""
	}
	if strings.HasPrefix(line, "export ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	}
	idx := strings.Index(line, "=")
	if idx <= 0 {
		return ""
	}
	key := strings.TrimSpace(line[:idx])
	if key == "" || !isValidAcmeEnvKey(key) {
		return ""
	}
	return key
}

func isValidAcmeEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for idx, r := range key {
		if idx == 0 {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' {
				continue
			}
			return false
		}
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func envMapToEnvPairs(env map[string]string) []string {
	if len(env) == 0 {
		return []string{}
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	// keep deterministic output for easier debugging
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(env[key])
		if value == "" {
			continue
		}
		pairs = append(pairs, key+"="+value)
	}
	return pairs
}

func mergeEnvPairs(base []string, override []string) []string {
	merged := make(map[string]string, len(base)+len(override))
	apply := func(items []string) {
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			idx := strings.Index(item, "=")
			if idx <= 0 {
				continue
			}
			key := strings.TrimSpace(item[:idx])
			value := strings.TrimSpace(item[idx+1:])
			if key == "" || value == "" {
				continue
			}
			merged[key] = value
		}
	}
	apply(base)
	apply(override)
	return envMapToEnvPairs(merged)
}

type acmeCommandRunner func(timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error)

func defaultAcmeCommandRunner(runner acmeCommandRunner) acmeCommandRunner {
	if runner != nil {
		return runner
	}
	return runCommandOutputWithTimeoutEnvLog
}

func (s *AcmeService) ensureAcmeAccountEmailForServer(scriptPath string, homeDir string, email string, server string, logSession *acmeLogSession) error {
	return s.ensureAcmeAccountEmailForServerWithRunner(scriptPath, homeDir, email, server, logSession, nil)
}

func (s *AcmeService) ensureAcmeAccountEmailForServerWithRunner(scriptPath string, homeDir string, email string, server string, logSession *acmeLogSession, runner acmeCommandRunner) error {
	validEmail, err := validateAcmeEmail(email)
	if err != nil {
		return err
	}

	runner = defaultAcmeCommandRunner(runner)
	server = strings.TrimSpace(server)

	updateFirst := []string{"--update-account", "-m", validEmail}
	updateSecond := []string{"--update-account", "--accountemail", validEmail}
	if server != "" {
		updateFirst = append(updateFirst, "--server", server)
		updateSecond = append(updateSecond, "--server", server)
	}

	tryRun := func(args []string) error {
		_, runErr := runner(90*time.Second, scriptPath, append(acmeHomeArgs(homeDir), args...), nil, logSession)
		return runErr
	}

	tryUpdate := func() error {
		if err := tryRun(updateFirst); err == nil {
			return nil
		} else {
			if isAcmeInvalidContactError(err) {
				return err
			}
			if isAcmeAccountNotRegisteredError(err) {
				return err
			}
			if isAcmeUnsupportedEmailFlagError(err) {
				return tryRun(updateSecond)
			}
			if fallbackErr := tryRun(updateSecond); fallbackErr == nil {
				return nil
			} else if isAcmeUnsupportedEmailFlagError(fallbackErr) {
				return err
			} else {
				return fallbackErr
			}
		}
	}

	updateErr := tryUpdate()
	if updateErr == nil {
		return nil
	}
	if isAcmeInvalidContactError(updateErr) {
		return updateErr
	}
	if !isAcmeAccountNotRegisteredError(updateErr) {
		return updateErr
	}

	if logSession != nil {
		logSession.append("检测到 ACME 账号未注册，先注册后重试邮箱同步")
	}
	if err := s.registerAcmeAccountIfNeededWithRunner(scriptPath, homeDir, validEmail, server, logSession, runner); err != nil {
		return err
	}
	return tryUpdate()
}

func (s *AcmeService) registerAcmeAccountIfNeeded(scriptPath string, homeDir string, email string, server string, logSession *acmeLogSession) error {
	return s.registerAcmeAccountIfNeededWithRunner(scriptPath, homeDir, email, server, logSession, nil)
}

func (s *AcmeService) registerAcmeAccountIfNeededWithRunner(scriptPath string, homeDir string, email string, server string, logSession *acmeLogSession, runner acmeCommandRunner) error {
	validEmail, err := validateAcmeEmail(email)
	if err != nil {
		return err
	}

	runner = defaultAcmeCommandRunner(runner)
	server = strings.TrimSpace(server)

	// Prefer modern acme.sh syntax: --register-account -m <email>.
	first := []string{"--register-account", "-m", validEmail}
	second := []string{"--register-account", "--accountemail", validEmail}
	if server != "" {
		first = append(first, "--server", server)
		second = append(second, "--server", server)
	}

	tryRegister := func(args []string) error {
		_, runErr := runner(90*time.Second, scriptPath, append(acmeHomeArgs(homeDir), args...), nil, logSession)
		if runErr == nil {
			return nil
		}
		text := normalizeAcmeOutputForMatch(runErr.Error())
		if strings.Contains(text, "already") || strings.Contains(text, "exists") {
			return nil
		}
		return runErr
	}

	if err := tryRegister(first); err == nil {
		return nil
	} else {
		if isAcmeInvalidContactError(err) {
			// Email is invalid for CA. Do not mask root cause by switching flags.
			return err
		}
		if isAcmeUnsupportedEmailFlagError(err) {
			return tryRegister(second)
		}
		// Non-flag error: still try legacy form once for compatibility.
		if fallbackErr := tryRegister(second); fallbackErr == nil {
			return nil
		} else if isAcmeUnsupportedEmailFlagError(fallbackErr) {
			return err
		} else {
			return fallbackErr
		}
	}
}

func isAcmeInvalidContactError(err error) bool {
	if err == nil {
		return false
	}
	text := normalizeAcmeOutputForMatch(err.Error())
	if text == "" {
		return false
	}
	return strings.Contains(text, "invalidcontact") ||
		(strings.Contains(text, "contact") && strings.Contains(text, "unable to parse email"))
}

func isAcmeAccountNotRegisteredError(err error) bool {
	if err == nil {
		return false
	}
	text := normalizeAcmeOutputForMatch(err.Error())
	if text == "" {
		return false
	}
	return strings.Contains(text, "please add '--register-account'") ||
		strings.Contains(text, "please add \"--register-account\"") ||
		strings.Contains(text, "please register account first") ||
		strings.Contains(text, "register account first") ||
		strings.Contains(text, "account is not valid yet") ||
		strings.Contains(text, "account not registered") ||
		strings.Contains(text, "no account is registered")
}

func isAcmeUnsupportedEmailFlagError(err error) bool {
	if err == nil {
		return false
	}
	text := normalizeAcmeOutputForMatch(err.Error())
	if text == "" {
		return false
	}
	return strings.Contains(text, "unknown parameter") ||
		strings.Contains(text, "unknown option") ||
		strings.Contains(text, "invalid option") ||
		strings.Contains(text, "unrecognized option")
}

func (s *AcmeService) removeManagedAcmeLocked(removeCertificates bool) (*AcmeActionResult, error) {
	return s.removeManagedAcmeWithOptionsLocked(acmeRemoveOptions{
		removeCertificates: removeCertificates,
		removeRuntimeData:  false,
	})
}

func (s *AcmeService) removeManagedAcmeWithOptionsLocked(opts acmeRemoveOptions) (*AcmeActionResult, error) {
	var removedUnits []string
	var outputParts []string

	if err := cleanupStaleManagedAcmeInstallWorkspaces(managedAcmeWorkspaceParentDir()); err != nil {
		return nil, err
	}

	scriptPath, homeDir, installed := s.resolveAcmeScript()
	if installed && (isManagedAcmeScriptPath(scriptPath) || isManagedAcmeHomeDir(homeDir)) {
		if uninstallOutput, err := runCommandOutputWithTimeoutEnv(60*time.Second, scriptPath, append(acmeHomeArgs(homeDir), "--uninstall"), nil); err == nil {
			trimmed := strings.TrimSpace(uninstallOutput)
			if trimmed != "" {
				outputParts = append(outputParts, trimmed)
			}
		}
	}

	if runtime.GOOS == "linux" && !runningInsideContainer() {
		systemctlPath, lookErr := exec.LookPath("systemctl")
		if lookErr == nil {
			for _, unit := range acmeSystemdUnitCandidates {
				unit = strings.TrimSpace(unit)
				if unit == "" {
					continue
				}
				if runCommandWithTimeout(10*time.Second, systemctlPath, "stop", unit) == nil {
					removedUnits = append(removedUnits, unit+"(stopped)")
				}
				if runCommandWithTimeout(10*time.Second, systemctlPath, "disable", unit) == nil {
					removedUnits = append(removedUnits, unit+"(disabled)")
				}
			}
			_ = runCommandWithTimeout(10*time.Second, systemctlPath, "daemon-reload")
			_ = runCommandWithTimeout(10*time.Second, systemctlPath, "reset-failed")
		}
	}

	manifestLoaded, err := s.removeManagedFilesByManifestLocked()
	if err != nil {
		return nil, err
	}
	if err := s.removeManagedRootFallbackLocked(manifestLoaded); err != nil {
		return nil, err
	}

	if err := s.setString(acmeManagedPathManifestKey, ""); err != nil {
		return nil, err
	}
	savedScriptPath := strings.TrimSpace(s.readSettingWithDefault(acmeScriptPathKey, ""))
	if savedScriptPath == "" || isManagedAcmeScriptPath(savedScriptPath) || isManagedAcmeHomeDir(filepath.Dir(savedScriptPath)) {
		if err := s.setString(acmeScriptPathKey, ""); err != nil {
			return nil, err
		}
	}

	if opts.removeCertificates {
		if err := s.removeAcmeCertificatesAndInventoryLocked(); err != nil {
			return nil, err
		}
	}

	if opts.removeRuntimeData {
		runtimeRoot := filepath.Clean(filepath.Join(config.GetDataDir(), "acme"))
		_ = os.RemoveAll(runtimeRoot)
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}

	msg := "acme.sh 已删除（仅删除受管安装内容，未触碰证书与推送目录）"
	if opts.removeCertificates {
		msg = "acme.sh 与关联证书记录已删除"
	}
	if len(removedUnits) > 0 {
		msg = msg + "；已清理 systemd: " + strings.Join(uniqueStringList(removedUnits), ", ")
	}
	return &AcmeActionResult{
		Overview: overview,
		Msg:      msg,
		Output:   strings.Join(outputParts, "\n"),
	}, nil
}

func (s *AcmeService) removeAcmeRowsOnlyLocked() error {
	db := database.GetDB()
	if err := db.Where("source_type = ?", CertificateSourceACME).Delete(&model.CertificateRecord{}).Error; err != nil {
		return err
	}
	if err := db.Where("1 = 1").Delete(&model.AcmeCertificate{}).Error; err != nil {
		return err
	}
	if err := db.Where("1 = 1").Delete(&model.AcmeAccount{}).Error; err != nil {
		return err
	}
	if err := db.Where("1 = 1").Delete(&model.AcmeDNSAccount{}).Error; err != nil {
		return err
	}
	return nil
}

func (s *AcmeService) removeAcmeCertificatesAndInventoryLocked() error {
	if err := s.removeAcmeRowsOnlyLocked(); err != nil {
		return err
	}
	db := database.GetDB()
	if err := db.Where("1 = 1").Delete(&model.CertificateRecord{}).Error; err != nil {
		return err
	}
	if err := db.Where("1 = 1").Delete(&model.SelfSignedAuthority{}).Error; err != nil {
		return err
	}
	if err := db.Where("key IN ?", []string{"webCertFile", "webKeyFile", "subCertFile", "subKeyFile"}).Delete(&model.Setting{}).Error; err != nil {
		return err
	}
	return nil
}

func (s *AcmeService) removeManagedFilesByManifestLocked() (bool, error) {
	manifestRaw := strings.TrimSpace(s.readSettingWithDefault(acmeManagedPathManifestKey, ""))
	if manifestRaw == "" {
		return false, nil
	}

	var paths []string
	if err := json.Unmarshal([]byte(manifestRaw), &paths); err != nil {
		return false, nil
	}
	sort.Slice(paths, func(i, j int) bool {
		left := filepath.Clean(strings.TrimSpace(paths[i]))
		right := filepath.Clean(strings.TrimSpace(paths[j]))
		return len(strings.Split(left, string(os.PathSeparator))) > len(strings.Split(right, string(os.PathSeparator)))
	})

	touchedRoots := make(map[string]struct{})

	for _, raw := range paths {
		cleaned := filepath.Clean(strings.TrimSpace(raw))
		if cleaned == "" {
			continue
		}
		root, rel, matched := matchManagedAcmeInstallRoot(cleaned)
		if !matched || !isAllowedManagedAcmeManifestRelativePath(rel) {
			continue
		}
		touchedRoots[root] = struct{}{}
		if !pathExists(cleaned) {
			continue
		}
		info, statErr := os.Lstat(cleaned)
		if statErr != nil {
			continue
		}
		if info.IsDir() {
			continue
		}
		if err := os.Remove(cleaned); err != nil && !os.IsNotExist(err) {
			return true, common.NewError("remove managed acme file failed: ", cleaned, ": ", err)
		}
	}
	for root := range touchedRoots {
		if err := cleanupEmptyManagedAcmeInstallDirs(root); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (s *AcmeService) removeManagedRootFallbackLocked(manifestLoaded bool) error {
	_ = manifestLoaded
	for _, root := range acmeManagedInstallRoots() {
		if err := removeManagedInstallArtifactsAtRoot(root, false); err != nil {
			return err
		}
	}
	return nil
}

func removeManagedInstallArtifactsAtRoot(root string, removeManagedDirContents bool) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		target := filepath.Join(root, name)
		if entry.IsDir() {
			if _, ok := acmeManagedRootDirNames[name]; !ok {
				continue
			}
			if removeManagedDirContents {
				if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
					return common.NewError("remove managed acme fallback directory failed: ", target, ": ", err)
				}
				continue
			}
			empty, pruneErr := pruneManagedAcmeDirTreeIfEmpty(target)
			if pruneErr != nil {
				return common.NewError("cleanup managed acme fallback directory failed: ", target, ": ", pruneErr)
			}
			if empty {
				if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
					return common.NewError("remove empty managed acme fallback directory failed: ", target, ": ", err)
				}
			}
			continue
		}
		if _, ok := acmeManagedRootFileNames[name]; !ok {
			continue
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return common.NewError("remove managed acme fallback file failed: ", target, ": ", err)
		}
	}
	if err := cleanupEmptyManagedAcmeInstallDirs(root); err != nil {
		return err
	}
	return nil
}

func cleanupEmptyManagedAcmeInstallDirs(root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." || !pathExists(root) {
		return nil
	}
	for name := range acmeManagedRootDirNames {
		target := filepath.Join(root, strings.TrimSpace(name))
		empty, err := pruneManagedAcmeDirTreeIfEmpty(target)
		if err != nil {
			return common.NewError("cleanup empty managed acme directory failed: ", target, ": ", err)
		}
		if empty {
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return common.NewError("remove empty managed acme directory failed: ", target, ": ", err)
			}
		}
	}
	remain, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(remain) == 0 {
		_ = os.Remove(root)
	}
	return nil
}

func pruneManagedAcmeDirTreeIfEmpty(root string) (bool, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return true, nil
	}
	info, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	if !info.IsDir() {
		return false, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	empty := true
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			empty = false
			continue
		}
		target := filepath.Join(root, name)
		if entry.IsDir() {
			childEmpty, childErr := pruneManagedAcmeDirTreeIfEmpty(target)
			if childErr != nil {
				return false, childErr
			}
			if childEmpty {
				if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
					return false, err
				}
				continue
			}
			empty = false
			continue
		}
		empty = false
	}
	return empty, nil
}

func isManagedAcmeHomeDir(path string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" || cleaned == "." {
		return false
	}
	for _, root := range acmeManagedInstallRoots() {
		if cleaned == root {
			return true
		}
	}
	return false
}

func isManagedAcmeScriptPath(path string) bool {
	_, rel, matched := matchManagedAcmeInstallRoot(path)
	if !matched {
		return false
	}
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "" || rel == "." {
		return false
	}
	return rel == "acme.sh"
}

func matchManagedAcmeInstallRoot(path string) (string, string, bool) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if cleaned == "" || cleaned == "." {
		return "", "", false
	}
	for _, root := range acmeManagedInstallRoots() {
		rel, ok := relativePathWithinRoot(root, cleaned)
		if ok {
			return root, rel, true
		}
	}
	return "", "", false
}

func relativePathWithinRoot(root string, target string) (string, bool) {
	root = filepath.Clean(strings.TrimSpace(root))
	target = filepath.Clean(strings.TrimSpace(target))
	if root == "" || root == "." || target == "" || target == "." {
		return "", false
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", false
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return rel, true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return rel, true
}

func isAllowedManagedAcmeManifestRelativePath(rel string) bool {
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "" || rel == "." {
		return false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) == 0 {
		return false
	}
	first := strings.TrimSpace(parts[0])
	if first == "" {
		return false
	}
	if _, ok := acmeManagedRootDirNames[first]; ok {
		return true
	}
	if _, ok := acmeManagedRootFileNames[first]; ok {
		return len(parts) == 1
	}
	return false
}

func (s *AcmeService) persistManagedAcmeManifestLocked(homeDir string) error {
	files, err := collectManagedAcmeInstallPaths(homeDir)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(files)
	if err != nil {
		return err
	}
	return s.setString(acmeManagedPathManifestKey, string(raw))
}

func collectManagedAcmeInstallPaths(homeDir string) ([]string, error) {
	root := filepath.Clean(strings.TrimSpace(homeDir))
	if root == "" {
		return []string{}, nil
	}
	if !pathExists(root) {
		return []string{}, nil
	}

	result := make([]string, 0, len(acmeManagedRootFileNames)+len(acmeManagedRootDirNames))
	for name := range acmeManagedRootFileNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		target := filepath.Join(root, name)
		if pathExists(target) {
			result = append(result, filepath.Clean(target))
		}
	}
	for name := range acmeManagedRootDirNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		target := filepath.Join(root, name)
		if !pathExists(target) {
			continue
		}
		_ = filepath.WalkDir(target, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry == nil || entry.IsDir() {
				return nil
			}
			result = append(result, filepath.Clean(path))
			return nil
		})
	}
	sort.Strings(result)
	return result, nil
}

func (s *AcmeService) checkVersionDownloadableLocked(version string) (bool, error) {
	version = normalizeAcmeVersionTag(version)
	if version == "" {
		return false, nil
	}
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest("GET", acmeGitHubReleaseTagAPI+version, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "kwor")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		// Fallback to tags endpoint because some tags may not have release entries.
		page := 1
		perPage := 100
		for page <= 3 {
			url := fmt.Sprintf("%s?per_page=%d&page=%d", acmeGitHubTagsAPI, perPage, page)
			tags, hasMore, err := s.fetchAcmeTagPageByURL(client, url, perPage)
			if err != nil {
				return false, err
			}
			for _, tag := range tags {
				if strings.EqualFold(normalizeAcmeVersionTag(tag.TagName), version) {
					return true, nil
				}
			}
			if !hasMore {
				break
			}
			page++
		}
		return false, nil
	}
	return false, common.NewError("GitHub API returned ", resp.StatusCode)
}

func (s *AcmeService) fetchAcmeLatestVersion() (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	versions, _, err := s.fetchAcmeReleasePage(client, 1, 1)
	if err == nil && len(versions) > 0 {
		return normalizeAcmeVersionTag(versions[0].TagName), nil
	}

	tags, _, tagErr := s.fetchAcmeTagPage(client, 1, 1)
	if tagErr != nil {
		if err != nil {
			return "", common.NewError("failed to fetch latest acme version: ", err, "; fallback tags failed: ", tagErr)
		}
		return "", tagErr
	}
	if len(tags) == 0 {
		return "", common.NewError("no remote acme version found")
	}
	return normalizeAcmeVersionTag(tags[0].TagName), nil
}

func (s *AcmeService) fetchAcmeReleasePage(client *http.Client, page int, perPage int) ([]AcmeVersionItem, bool, error) {
	apiURL := fmt.Sprintf("%s?per_page=%d&page=%d", acmeGitHubReleasesAPI, perPage, page)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "kwor")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, false, common.NewError("GitHub releases API returned ", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	var releases []GitHubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, false, err
	}

	result := make([]AcmeVersionItem, 0, len(releases))
	for _, r := range releases {
		tag := normalizeAcmeVersionTag(r.TagName)
		if tag == "" {
			continue
		}
		publishedAt := strings.TrimSpace(r.PublishedAt)
		if publishedAt == "" {
			publishedAt = strings.TrimSpace(r.CreatedAt)
		}
		result = append(result, AcmeVersionItem{
			TagName:     tag,
			Name:        strings.TrimSpace(r.Name),
			PublishedAt: publishedAt,
			Source:      "release",
		})
	}
	return result, len(releases) >= perPage, nil
}

func (s *AcmeService) fetchAcmeTagPage(client *http.Client, page int, perPage int) ([]AcmeVersionItem, bool, error) {
	apiURL := fmt.Sprintf("%s?per_page=%d&page=%d", acmeGitHubTagsAPI, perPage, page)
	return s.fetchAcmeTagPageByURL(client, apiURL, perPage)
}

func (s *AcmeService) fetchAcmeTagPageByURL(client *http.Client, apiURL string, perPage int) ([]AcmeVersionItem, bool, error) {
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", "kwor")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, false, common.NewError("GitHub tags API returned ", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}
	var tags []acmeGitHubTag
	if err := json.Unmarshal(body, &tags); err != nil {
		return nil, false, err
	}
	result := make([]AcmeVersionItem, 0, len(tags))
	for _, t := range tags {
		tag := normalizeAcmeVersionTag(t.Name)
		if tag == "" {
			continue
		}
		result = append(result, AcmeVersionItem{
			TagName: tag,
			Name:    tag,
			Source:  "tag",
		})
	}
	if perPage <= 0 {
		perPage = 100
	}
	return result, len(tags) >= perPage, nil
}

func readAcmeVersionByScript(scriptPath string, homeDir string) (string, error) {
	output, err := runCommandOutputWithTimeoutEnv(12*time.Second, scriptPath, append(acmeHomeArgs(homeDir), "--version"), nil)
	if err != nil {
		return "", err
	}
	version := extractAcmeVersionFromOutput(output)
	if version == "" {
		return "", common.NewError("unable to detect acme.sh version from output")
	}
	return version, nil
}

func normalizeAcmeVersionTag(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.Trim(trimmed, "\"'")
	fields := strings.Fields(trimmed)
	for _, field := range fields {
		field = strings.TrimSpace(strings.Trim(field, "\"'"))
		if field == "" {
			continue
		}
		if strings.HasPrefix(field, "v") || strings.HasPrefix(field, "V") {
			field = "v" + strings.TrimSpace(field[1:])
		}
		if isLikelySemverTag(field) {
			return field
		}
	}
	return trimmed
}

func extractAcmeVersionFromOutput(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for _, line := range lines {
		version := normalizeAcmeVersionTag(line)
		if version != "" && isLikelySemverTag(version) {
			return version
		}
	}

	version := normalizeAcmeVersionTag(firstNonEmptyLine(raw))
	if version != "" && isLikelySemverTag(version) {
		return version
	}
	return ""
}

func isLikelySemverTag(value string) bool {
	v := strings.TrimSpace(value)
	if v == "" {
		return false
	}
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, "V") {
		v = strings.TrimSpace(v[1:])
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		digitSeen := false
		for _, r := range part {
			if r >= '0' && r <= '9' {
				digitSeen = true
				continue
			}
			if r == '-' || r == '+' || r == '_' {
				continue
			}
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				continue
			}
			return false
		}
		if !digitSeen {
			return false
		}
	}
	return true
}

func compareSemverLikeTags(a string, b string) int {
	pa, sa := splitSemverLike(a)
	pb, sb := splitSemverLike(b)
	maxLen := len(pa)
	if len(pb) > maxLen {
		maxLen = len(pb)
	}
	for i := 0; i < maxLen; i++ {
		va := 0
		if i < len(pa) {
			va = pa[i]
		}
		vb := 0
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	if sa == sb {
		return 0
	}
	if sa == "" {
		return 1
	}
	if sb == "" {
		return -1
	}
	return compareSemverLikeSuffix(sa, sb)
}

func compareSemverLikeSuffix(a string, b string) int {
	ta := tokenizeSemverLikeSuffix(a)
	tb := tokenizeSemverLikeSuffix(b)
	maxLen := len(ta)
	if len(tb) > maxLen {
		maxLen = len(tb)
	}
	for i := 0; i < maxLen; i++ {
		if i >= len(ta) {
			return -1
		}
		if i >= len(tb) {
			return 1
		}
		left := ta[i]
		right := tb[i]
		leftNum, leftIsNum := parseSemverLikeNumericToken(left)
		rightNum, rightIsNum := parseSemverLikeNumericToken(right)
		switch {
		case leftIsNum && rightIsNum:
			if leftNum < rightNum {
				return -1
			}
			if leftNum > rightNum {
				return 1
			}
		default:
			leftLower := strings.ToLower(left)
			rightLower := strings.ToLower(right)
			if leftLower < rightLower {
				return -1
			}
			if leftLower > rightLower {
				return 1
			}
		}
	}
	return 0
}

func tokenizeSemverLikeSuffix(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	result := make([]string, 0, len(value))
	var current strings.Builder
	currentKind := byte(0)
	flush := func() {
		if current.Len() == 0 {
			return
		}
		result = append(result, current.String())
		current.Reset()
		currentKind = 0
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			if currentKind != 'd' {
				flush()
				currentKind = 'd'
			}
			current.WriteRune(r)
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			if currentKind != 'a' {
				flush()
				currentKind = 'a'
			}
			current.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return result
}

func parseSemverLikeNumericToken(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return n, true
}

func splitSemverLike(value string) ([]int, string) {
	v := strings.TrimSpace(value)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		core := strings.TrimSpace(v[:idx])
		suffix := strings.TrimSpace(v[idx+1:])
		return parseSemverNumbers(core), suffix
	}
	return parseSemverNumbers(v), ""
}

func parseSemverNumbers(value string) []int {
	parts := strings.Split(strings.TrimSpace(value), ".")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			result = append(result, 0)
			continue
		}
		numPart := part
		for idx, r := range part {
			if r < '0' || r > '9' {
				numPart = part[:idx]
				break
			}
		}
		if numPart == "" {
			result = append(result, 0)
			continue
		}
		n, err := strconv.Atoi(numPart)
		if err != nil {
			n = 0
		}
		result = append(result, n)
	}
	return result
}

func uniqueStringList(items []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		result = append(result, text)
	}
	return result
}

func downloadAcmeInstallerScript(targetPath string) error {
	if targetPath == "" {
		return common.NewError("acme installer target path is empty")
	}

	if curlPath, err := exec.LookPath("curl"); err == nil {
		if err := runCommandWithTimeout(45*time.Second, curlPath, "-fsSL", defaultAcmeInstallScriptURL, "-o", targetPath); err == nil {
			return nil
		}
	}
	if wgetPath, err := exec.LookPath("wget"); err == nil {
		if err := runCommandWithTimeout(45*time.Second, wgetPath, "-qO", targetPath, defaultAcmeInstallScriptURL); err == nil {
			return nil
		}
	}
	return common.NewError("failed to download acme.sh installer: curl/wget unavailable or network failed")
}

func firstNonEmptyLine(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func summarizeAcmeInstallOutput(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	keywords := []string{
		"error",
		"failed",
		"cannot",
		"not found",
		"please install",
		"pre-check",
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				return line
			}
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func isAcmeDomainsNotChangedError(err error) bool {
	if err == nil {
		return false
	}
	text := normalizeAcmeOutputForMatch(err.Error())
	if text == "" {
		return false
	}
	if !strings.Contains(text, "domains not changed") {
		return false
	}
	return strings.Contains(text, "add '--force'") ||
		strings.Contains(text, "add \"--force\"") ||
		strings.Contains(text, "force renewal")
}

func isAcmeRenewSkippedError(err error) bool {
	if err == nil {
		return false
	}
	text := normalizeAcmeOutputForMatch(err.Error())
	if text == "" {
		return false
	}
	if strings.Contains(text, "skipping. next renewal time is") &&
		(strings.Contains(text, "add '--force'") ||
			strings.Contains(text, "add \"--force\"") ||
			strings.Contains(text, "force renewal")) {
		return true
	}
	return isAcmeDomainsNotChangedError(err)
}

func normalizeAcmeOutputForMatch(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = acmeAnsiCodePattern.ReplaceAllString(raw, "")
	raw = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case r < 32 || r == 127:
			return -1
		default:
			return r
		}
	}, raw)
	return strings.ToLower(strings.Join(strings.Fields(raw), " "))
}

func runCommandOutputWithTimeoutEnv(timeout time.Duration, command string, args []string, envPairs []string) (string, error) {
	return runCommandOutputWithTimeoutEnvLog(timeout, command, args, envPairs, nil)
}

func runCommandOutputWithTimeoutEnvLog(timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	if len(envPairs) > 0 {
		cmd.Env = append(os.Environ(), envPairs...)
	}

	var output strings.Builder
	var outputMu sync.Mutex
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var wg sync.WaitGroup
	collect := func(reader io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), "\r")
			outputMu.Lock()
			output.WriteString(line)
			output.WriteString("\n")
			outputMu.Unlock()
			if logSession != nil {
				logSession.append(line)
			}
		}
		if scanErr := scanner.Err(); scanErr != nil && logSession != nil {
			logSession.append("读取命令输出失败: " + scanErr.Error())
		}
	}
	wg.Add(2)
	go collect(stdout)
	go collect(stderr)

	err = cmd.Wait()
	wg.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out (%s %s)", command, strings.Join(args, " "))
	}
	if err != nil {
		outputMu.Lock()
		text := strings.TrimSpace(output.String())
		outputMu.Unlock()
		if text == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, text)
	}
	outputMu.Lock()
	text := output.String()
	outputMu.Unlock()
	return text, nil
}

type acmeLogStore struct {
	mu       sync.Mutex
	sessions map[string]*acmeLogSession
}

type acmeLogSession struct {
	id         string
	title      string
	status     string
	lines      []string
	errText    string
	startedAt  int64
	updatedAt  int64
	finishedAt int64
}

func newAcmeLogStore() *acmeLogStore {
	return &acmeLogStore{
		sessions: make(map[string]*acmeLogSession),
	}
}

func (s *acmeLogStore) start(id string, title string) *acmeLogSession {
	id = normalizeAcmeLogSessionID(id)
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)

	session := &acmeLogSession{
		id:        id,
		title:     strings.TrimSpace(title),
		status:    "running",
		startedAt: now,
		updatedAt: now,
	}
	if session.title == "" {
		session.title = "ACME 任务"
	}
	s.sessions[id] = session
	return session
}

func (s *acmeLogStore) get(id string) *AcmeLogSessionView {
	id = strings.TrimSpace(id)
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)

	session := s.sessions[id]
	if session == nil {
		return &AcmeLogSessionView{
			Id:        id,
			Title:     "ACME 任务",
			Status:    "missing",
			Lines:     []string{"log session not found"},
			StartedAt: now,
			UpdatedAt: now,
		}
	}
	return session.snapshotLocked()
}

func (s *acmeLogStore) pruneLocked(now int64) {
	ttlSeconds := int64(acmeLogTTL / time.Second)
	for id, session := range s.sessions {
		if now-session.updatedAt > ttlSeconds {
			delete(s.sessions, id)
		}
	}
}

func (s *acmeLogSession) append(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	acmeLogSessionStore.mu.Lock()
	defer acmeLogSessionStore.mu.Unlock()
	s.lines = append(s.lines, line)
	if len(s.lines) > acmeLogMaxLines {
		s.lines = append([]string{"日志过长，已隐藏较早输出"}, s.lines[len(s.lines)-acmeLogMaxLines:]...)
	}
	s.updatedAt = time.Now().Unix()
}

func (s *acmeLogSession) finish(line string) {
	acmeLogSessionStore.mu.Lock()
	defer acmeLogSessionStore.mu.Unlock()
	s.appendLocked(line)
	s.status = "success"
	now := time.Now().Unix()
	s.updatedAt = now
	s.finishedAt = now
}

func (s *acmeLogSession) fail(message string) {
	acmeLogSessionStore.mu.Lock()
	defer acmeLogSessionStore.mu.Unlock()
	message = strings.TrimSpace(message)
	if message == "" {
		message = "ACME task failed"
	}
	s.appendLocked("失败: " + message)
	s.status = "error"
	s.errText = message
	now := time.Now().Unix()
	s.updatedAt = now
	s.finishedAt = now
}

func (s *acmeLogSession) appendLocked(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	s.lines = append(s.lines, line)
	if len(s.lines) > acmeLogMaxLines {
		s.lines = append([]string{"日志过长，已隐藏较早输出"}, s.lines[len(s.lines)-acmeLogMaxLines:]...)
	}
}

func (s *acmeLogSession) snapshotLocked() *AcmeLogSessionView {
	lines := append([]string(nil), s.lines...)
	return &AcmeLogSessionView{
		Id:         s.id,
		Title:      s.title,
		Status:     s.status,
		Lines:      lines,
		Error:      s.errText,
		StartedAt:  s.startedAt,
		UpdatedAt:  s.updatedAt,
		FinishedAt: s.finishedAt,
	}
}

func normalizeAcmeLogSessionID(id string) string {
	id = strings.TrimSpace(id)
	if acmeLogIDPattern.MatchString(id) {
		return id
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("acme-%d", time.Now().UnixNano())
	}
	return "acme-" + hex.EncodeToString(buf[:])
}
