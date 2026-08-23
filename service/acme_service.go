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
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	"unicode/utf8"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/network"
	"github.com/alireza0/s-ui/util/common"
	"golang.org/x/net/idna"
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
	acmeRuntimeSchemaV2Key     = "acmeRuntimeSchemaV2"

	defaultAcmePreferredCA                         = "letsencrypt"
	defaultAcmeChallenge                           = "standalone"
	defaultAcmeKeyLength                           = "ec-256"
	defaultAcmeAutoRenewDays                       = 30
	defaultAcmeInstallScriptURL                    = "https://raw.githubusercontent.com/acmesh-official/acme.sh/master/acme.sh"
	acmeGitHubReleasesAPI                          = "https://api.github.com/repos/acmesh-official/acme.sh/releases"
	acmeGitHubReleaseTagAPI                        = "https://api.github.com/repos/acmesh-official/acme.sh/releases/tags/"
	acmeGitHubTagsAPI                              = "https://api.github.com/repos/acmesh-official/acme.sh/tags"
	acmeGitHubResponseMaxBytes               int64 = 4 * 1024 * 1024
	acmeCommandOutputMaxBytes                int   = 512 * 1024
	acmeCommandErrorMaxBytes                 int   = 16 * 1024
	acmeCommandOutputLineMaxBytes            int   = 64 * 1024
	acmeStoredOutputMaxBytes                 int   = 128 * 1024
	acmeLogMaxLines                                = 400
	acmeLogMaxBytes                                = 256 * 1024
	acmeLogMaxLineBytes                            = 4 * 1024
	acmeLogTTL                                     = 30 * time.Minute
	acmeTaskTTL                                    = 30 * time.Minute
	acmeTerminalTaskCleanupDelay                   = 5 * time.Second
	acmeTaskQueueCapacity                          = 4096
	acmeTaskResultOutputMaxRunes                   = 4096
	acmeTaskDeadline                               = 30 * time.Minute
	acmeDomainCertificateMaxNames                  = 2048
	acmeCertificateTypeDomain                      = "domain"
	acmeCertificateTypeIP                          = "ip"
	acmeLEProductionDirectory                      = "https://acme-v02.api.letsencrypt.org/directory"
	acmeLEStagingDirectory                         = "https://acme-staging-v02.api.letsencrypt.org/directory"
	acmeZeroSSLDirectory                           = "https://acme.zerossl.com/v2/DV90"
	acmeIPCertificateMaxIPs                        = 2048
	acmeIPCertificatePortHTTP                      = 80
	acmeIPCertificatePortALPN                      = 443
	acmeMaskedEnvValue                             = "********"
	acmeManagedWorkspaceStagePrefix                = "acme-home-stage-"
	acmeManagedWorkspaceBackupPrefix               = "acme-home-backup-"
	acmeAutoRenewRetryPhaseRapid                   = "rapid_retry"
	acmeAutoRenewRetryPhasePeriodic                = "periodic_retry"
	acmeAutoRenewRetryPhaseExpiredDisabled         = "expired_disabled"
	acmeAutoRenewRapidRetryLimit                   = 3
	acmeAutoRenewBatchDuration                     = time.Hour
	acmeAutoRenewRapidRetryInterval                = 10 * time.Minute
	acmeAutoRenewPeriodicRetryInterval             = 6 * time.Hour
	certificateAutoRenewBatchStateSettingKey       = "certificateAutoRenewBatchStateV1"
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
	acmeAutoRenewBatch  = acmeAutoRenewBatchState{}

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

var acmeInstallTaskManager = NewManagedDownloadTaskManager("acme.sh install")
var acmeInstallTaskState = struct {
	sync.Mutex
	id           string
	logSessionID string
}{}

type acmeAutoRenewBatchState struct {
	mu           sync.Mutex
	loaded       bool
	startedAt    int64
	endsAt       int64
	candidateIDs map[uint]struct{}
	completedIDs map[uint]struct{}
}

type acmeAutoRenewBatchPersistentState struct {
	StartedAt int64 `json:"startedAt"`
	EndsAt    int64 `json:"endsAt"`
}

func init() {
	database.RegisterDBResetHook(func() {
		acmeAutoRenewBatch.mu.Lock()
		defer acmeAutoRenewBatch.mu.Unlock()
		acmeAutoRenewBatch.loaded = false
		acmeAutoRenewBatch.startedAt = 0
		acmeAutoRenewBatch.endsAt = 0
		acmeAutoRenewBatch.candidateIDs = nil
		acmeAutoRenewBatch.completedIDs = nil
	})
}

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
		Helper:       "acme.sh 官方: dns_cf；支持 Token 模式（CF_Token 可单独使用，CF_Account_ID/CF_Zone_ID 可选）或 Global Key 模式（CF_Email + CF_Key）",
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
		Helper:       "acme.sh 官方: dns_huaweicloud；HUAWEICLOUD_Region 可选，默认 ap-southeast-1",
		Fields: []AcmeDNSFieldDef{
			{Key: "HUAWEICLOUD_Username", Label: "用户名", Required: true},
			{Key: "HUAWEICLOUD_Password", Label: "密码", Required: true},
			{Key: "HUAWEICLOUD_DomainName", Label: "DomainName", Required: true},
			{Key: "HUAWEICLOUD_Region", Label: "Region（可选）", Required: false, Placeholder: "cn-north-4"},
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
	{
		Name:         "Spaceship",
		ProviderCode: "dns_spaceship",
		Helper:       "acme.sh 官方: dns_spaceship；API Key 需要 dnsrecords:read 和 dnsrecords:write 权限；根域名自动识别失败时可填写 SPACESHIP_ROOT_DOMAIN",
		Fields: []AcmeDNSFieldDef{
			{Key: "SPACESHIP_API_KEY", Label: "API Key", Required: true},
			{Key: "SPACESHIP_API_SECRET", Label: "API Secret", Required: true},
			{Key: "SPACESHIP_ROOT_DOMAIN", Label: "根域名（可选）", Required: false, Placeholder: "example.com"},
		},
	},
	{
		Name:         "Google Cloud DNS",
		ProviderCode: "dns_gcloud",
		Helper:       "acme.sh 官方: dns_gcloud；依赖服务器已安装并授权 gcloud CLI，CLOUDSDK_ACTIVE_CONFIG_NAME 可选",
		Fields: []AcmeDNSFieldDef{
			{Key: "CLOUDSDK_ACTIVE_CONFIG_NAME", Label: "gcloud 配置名称（可选）", Required: false, Placeholder: "default"},
		},
	},
	{
		Name:         "Azure DNS",
		ProviderCode: "dns_azure",
		Helper:       "acme.sh 官方: dns_azure；必填订阅 ID，并选择托管身份、Bearer Token 或服务主体（Tenant/App/Client Secret）之一",
		Fields: []AcmeDNSFieldDef{
			{Key: "AZUREDNS_SUBSCRIPTIONID", Label: "Subscription ID", Required: true},
			{Key: "AZUREDNS_TENANTID", Label: "Tenant ID（服务主体模式）", Required: false},
			{Key: "AZUREDNS_APPID", Label: "App ID（服务主体模式）", Required: false},
			{Key: "AZUREDNS_CLIENTSECRET", Label: "Client Secret（服务主体模式）", Required: false},
			{Key: "AZUREDNS_MANAGEDIDENTITY", Label: "Managed Identity（可选）", Required: false, Placeholder: "true"},
			{Key: "AZUREDNS_BEARERTOKEN", Label: "Bearer Token（可选）", Required: false},
		},
	},
	{
		Name:         "Oracle Cloud Infrastructure DNS",
		ProviderCode: "dns_oci",
		Helper:       "acme.sh 官方: dns_oci；默认读取 OCI CLI 的 DEFAULT 配置，也可填写租户、用户、区域和签名密钥参数",
		Fields: []AcmeDNSFieldDef{
			{Key: "OCI_CLI_TENANCY", Label: "Tenancy OCID（可选）", Required: false},
			{Key: "OCI_CLI_USER", Label: "User OCID（可选）", Required: false},
			{Key: "OCI_CLI_REGION", Label: "Region（可选）", Required: false, Placeholder: "us-ashburn-1"},
			{Key: "OCI_CLI_KEY_FILE", Label: "API Signing Key 文件路径（可选）", Required: false},
			{Key: "OCI_CLI_KEY", Label: "API Signing Key PEM（可选）", Required: false},
		},
	},
	{
		Name:         "NS1",
		ProviderCode: "dns_nsone",
		Helper:       "acme.sh 官方: dns_nsone",
		Fields: []AcmeDNSFieldDef{
			{Key: "NS1_Key", Label: "API Key", Required: true},
		},
	},
	{
		Name:         "Akamai Connected Cloud（Linode）",
		ProviderCode: "dns_linode_v4",
		Helper:       "acme.sh 官方: dns_linode_v4",
		Fields: []AcmeDNSFieldDef{
			{Key: "LINODE_V4_API_KEY", Label: "API Key", Required: true},
		},
	},
	{
		Name:         "DigitalOcean",
		ProviderCode: "dns_dgon",
		Helper:       "acme.sh 官方: dns_dgon",
		Fields: []AcmeDNSFieldDef{
			{Key: "DO_API_KEY", Label: "API Key", Required: true},
		},
	},
	{
		Name:         "Vultr",
		ProviderCode: "dns_vultr",
		Helper:       "acme.sh 官方: dns_vultr",
		Fields: []AcmeDNSFieldDef{
			{Key: "VULTR_API_KEY", Label: "API Key", Required: true},
		},
	},
	{
		Name:         "Namecheap",
		ProviderCode: "dns_namecheap",
		Helper:       "acme.sh 官方: dns_namecheap；需要开启 Namecheap API，Source IP 可选，未填写时由 acme.sh 尝试探测",
		Fields: []AcmeDNSFieldDef{
			{Key: "NAMECHEAP_API_KEY", Label: "API Key", Required: true},
			{Key: "NAMECHEAP_USERNAME", Label: "Username", Required: true},
			{Key: "NAMECHEAP_SOURCEIP", Label: "Source IP（可选）", Required: false},
		},
	},
	{
		Name:         "Gandi LiveDNS",
		ProviderCode: "dns_gandi_livedns",
		Helper:       "acme.sh 官方: dns_gandi_livedns；优先使用 GANDI_LIVEDNS_TOKEN，旧版 API Key 仅作兼容",
		Fields: []AcmeDNSFieldDef{
			{Key: "GANDI_LIVEDNS_TOKEN", Label: "Personal Access Token（推荐）", Required: false},
			{Key: "GANDI_LIVEDNS_KEY", Label: "旧版 API Key（可选）", Required: false},
		},
	},
	{
		Name:         "Porkbun",
		ProviderCode: "dns_porkbun",
		Helper:       "acme.sh 官方: dns_porkbun",
		Fields: []AcmeDNSFieldDef{
			{Key: "PORKBUN_API_KEY", Label: "API Key", Required: true},
			{Key: "PORKBUN_SECRET_API_KEY", Label: "Secret API Key", Required: true},
		},
	},
	{
		Name:         "Name.com",
		ProviderCode: "dns_namecom",
		Helper:       "acme.sh 官方: dns_namecom",
		Fields: []AcmeDNSFieldDef{
			{Key: "Namecom_Username", Label: "Username", Required: true},
			{Key: "Namecom_Token", Label: "API Token", Required: true},
		},
	},
	{
		Name:         "Njalla",
		ProviderCode: "dns_njalla",
		Helper:       "acme.sh 官方: dns_njalla",
		Fields: []AcmeDNSFieldDef{
			{Key: "NJALLA_Token", Label: "API Token", Required: true},
		},
	},
	{
		Name:         "ClouDNS",
		ProviderCode: "dns_cloudns",
		Helper:       "acme.sh 官方: dns_cloudns；CLOUDNS_AUTH_ID 与 CLOUDNS_SUB_AUTH_ID 至少填写一个，并填写密码",
		Fields: []AcmeDNSFieldDef{
			{Key: "CLOUDNS_AUTH_ID", Label: "Auth ID（可选）", Required: false},
			{Key: "CLOUDNS_SUB_AUTH_ID", Label: "Sub Auth ID（可选）", Required: false},
			{Key: "CLOUDNS_AUTH_PASSWORD", Label: "Auth Password", Required: true},
		},
	},
	{
		Name:         "Hurricane Electric DNS",
		ProviderCode: "dns_he",
		Helper:       "acme.sh 官方: dns_he；使用 dns.he.net 账号密码管理 DNS 记录",
		Fields: []AcmeDNSFieldDef{
			{Key: "HE_Username", Label: "Username", Required: true},
			{Key: "HE_Password", Label: "Password", Required: true},
		},
	},
	{
		Name:         "DNS Made Easy",
		ProviderCode: "dns_me",
		Helper:       "acme.sh 官方: dns_me",
		Fields: []AcmeDNSFieldDef{
			{Key: "ME_Key", Label: "API Key", Required: true},
			{Key: "ME_Secret", Label: "API Secret", Required: true},
		},
	},
	{
		Name:         "Constellix",
		ProviderCode: "dns_constellix",
		Helper:       "acme.sh 官方: dns_constellix",
		Fields: []AcmeDNSFieldDef{
			{Key: "CONSTELLIX_Key", Label: "API Key", Required: true},
			{Key: "CONSTELLIX_Secret", Label: "API Secret", Required: true},
		},
	},
	{
		Name:         "FreeDNS（afraid.org）",
		ProviderCode: "dns_freedns",
		Helper:       "acme.sh 官方: dns_freedns；使用 FreeDNS 账号密码",
		Fields: []AcmeDNSFieldDef{
			{Key: "FREEDNS_User", Label: "Username", Required: true},
			{Key: "FREEDNS_Password", Label: "Password", Required: true},
		},
	},
	{
		Name:         "ZoneEdit",
		ProviderCode: "dns_zoneedit",
		Helper:       "acme.sh 官方: dns_zoneedit",
		Fields: []AcmeDNSFieldDef{
			{Key: "ZONEEDIT_ID", Label: "ID", Required: true},
			{Key: "ZONEEDIT_Token", Label: "API Token", Required: true},
		},
	},
	{
		Name:         "Rage4",
		ProviderCode: "dns_rage4",
		Helper:       "acme.sh 官方: dns_rage4",
		Fields: []AcmeDNSFieldDef{
			{Key: "RAGE4_USERNAME", Label: "Username", Required: true},
			{Key: "RAGE4_TOKEN", Label: "API Token", Required: true},
		},
	},
	{
		Name:         "Yandex Cloud DNS",
		ProviderCode: "dns_yc",
		Helper:       "acme.sh 官方: dns_yc；需要 Zone ID 或 Folder ID，以及 Service Account ID、IAM Key ID 和 RSA 私钥文件路径或 Base64 内容",
		Fields: []AcmeDNSFieldDef{
			{Key: "YC_Zone_ID", Label: "DNS Zone ID（可选）", Required: false},
			{Key: "YC_Folder_ID", Label: "Folder ID（可选）", Required: false},
			{Key: "YC_SA_ID", Label: "Service Account ID", Required: false},
			{Key: "YC_SA_Key_ID", Label: "Service Account IAM Key ID", Required: false},
			{Key: "YC_SA_Key_File_Path", Label: "RSA 私钥文件路径（可选）", Required: false},
			{Key: "YC_SA_Key_File_PEM_b64", Label: "RSA 私钥 Base64（可选）", Required: false},
		},
	},
	{
		Name:         "DuckDNS",
		ProviderCode: "dns_duckdns",
		Helper:       "acme.sh 官方: dns_duckdns",
		Fields: []AcmeDNSFieldDef{
			{Key: "DuckDNS_Token", Label: "API Token", Required: true},
		},
	},
	{
		Name:         "Dynu",
		ProviderCode: "dns_dynu",
		Helper:       "acme.sh 官方: dns_dynu；使用 Dynu API Client ID 与 Secret",
		Fields: []AcmeDNSFieldDef{
			{Key: "Dynu_ClientId", Label: "Client ID", Required: true},
			{Key: "Dynu_Secret", Label: "Client Secret", Required: true},
		},
	},
	{
		Name:         "Volcano Engine DNS",
		ProviderCode: "dns_volcengine",
		Helper:       "acme.sh 官方: dns_volcengine；支持长期 AK/SK，也支持临时 STS Session Token",
		Fields: []AcmeDNSFieldDef{
			{Key: "Volcengine_ACCESS_KEY_ID", Label: "Access Key ID", Required: true},
			{Key: "Volcengine_SECRET_ACCESS_KEY", Label: "Secret Access Key", Required: true},
			{Key: "Volcengine_SESSION_TOKEN", Label: "Session Token（可选）", Required: false},
		},
	},
	{
		Name:         "百度智能云 DNS",
		ProviderCode: "dns_baidu",
		Helper:       "acme.sh 官方: dns_baidu；默认自动选择新版 DNS API，Baidu_API_Preference 等高级参数可通过额外环境变量覆盖",
		Fields: []AcmeDNSFieldDef{
			{Key: "Baidu_AK", Label: "AccessKeyId", Required: true},
			{Key: "Baidu_SK", Label: "SecretAccessKey", Required: true},
			{Key: "Baidu_API_Preference", Label: "API Preference（可选）", Required: false, Placeholder: "auto"},
		},
	},
	{
		Name:         "西部数码 West.cn",
		ProviderCode: "dns_west_cn",
		Helper:       "acme.sh 官方: dns_west_cn",
		Fields: []AcmeDNSFieldDef{
			{Key: "WEST_Username", Label: "API Username", Required: true},
			{Key: "WEST_Key", Label: "API Key", Required: true},
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
	Id               uint   `json:"id"`
	DisplayID        uint64 `json:"displayId"`
	ResourceID       string `json:"resourceId"`
	Name             string `json:"name"`
	Email            string `json:"email"`
	Server           string `json:"server"`
	AccountKeyLength string `json:"accountKeyLength"`
	Registered       bool   `json:"registered"`
	Remark           string `json:"remark"`
	CreatedAt        int64  `json:"createdAt"`
	UpdatedAt        int64  `json:"updatedAt"`
}

type AcmeDNSAccountView struct {
	Id             uint              `json:"id"`
	DisplayID      uint64            `json:"displayId"`
	ResourceID     string            `json:"resourceId"`
	Name           string            `json:"name"`
	ProviderName   string            `json:"providerName"`
	ProviderCode   string            `json:"providerCode"`
	ProviderLocked bool              `json:"providerLocked"`
	Env            map[string]string `json:"env"`
	Remark         string            `json:"remark"`
	CreatedAt      int64             `json:"createdAt"`
	UpdatedAt      int64             `json:"updatedAt"`
}

type AcmeCertificateView = CertificateRecordView

type AcmeIssuePayload struct {
	// ExistingRecordID turns an issuance into an in-place reissue. The
	// certificate inventory row and its panel/subscription assignments remain.
	ExistingRecordID uint
	DomainsText      string
	CertificateType  string
	Challenge        string
	Webroot          string
	DNSProvider      string
	DNSEnvText       string
	Server           string
	KeyLength        string
	CustomArgs       string

	AcmeAccountID uint
	DNSAccountID  uint
	AutoRenew     bool
	Remark        string

	ApplyTarget  string
	PushDir      string
	PushExplicit bool
	LogSessionID string

	// The HTTP API keeps its existing JSON shape but records which optional
	// fields were actually supplied. Reissue merges only omitted values from
	// the current certificate record, so an explicit value can still override
	// its prior configuration.
	DomainsProvided         bool
	CertificateTypeProvided bool
	ChallengeProvided       bool
	WebrootProvided         bool
	DNSProviderProvided     bool
	DNSEnvProvided          bool
	KeyLengthProvided       bool
	CustomArgsProvided      bool
	AcmeAccountProvided     bool
	DNSAccountProvided      bool
	AutoRenewProvided       bool
	RemarkProvided          bool
	ApplyTargetProvided     bool
	PushDirProvided         bool
}

type AcmeRenewPayload struct {
	ID    uint
	Force bool
	// Manual is retained for wire compatibility with older callers. It no
	// longer changes force semantics: only Force requests a fresh issuance.
	Manual       bool
	ApplyTarget  string
	LogSessionID string
}

type AcmePushPayload struct {
	ID        uint
	TargetDir string
	Clear     bool
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
	ID               uint
	Name             string
	Email            string
	Server           string
	AccountKeyLength string
	// KeyLength is only accepted during the schema transition from the old API.
	// It is never used as a certificate key-length setting again.
	KeyLength string
	Remark    string

	EmailProvided            bool
	ServerProvided           bool
	AccountKeyLengthProvided bool
	RemarkProvided           bool
}

type AcmeAccountRotateKeyPayload struct {
	ID               uint
	AccountKeyLength string
}

type AcmeDNSAccountPayload struct {
	ID           uint
	Name         string
	ProviderCode string
	EnvJSON      string
	Remark       string

	EnvJSONProvided bool
	RemarkProvided  bool
}

type AcmeActionResult struct {
	Overview    *AcmeOverview        `json:"overview,omitempty"`
	Certificate *AcmeCertificateView `json:"certificate,omitempty"`
	Msg         string               `json:"msg,omitempty"`
	Output      string               `json:"output,omitempty"`
	Warnings    []string             `json:"warnings,omitempty"`
}

// AcmeTaskView exposes a short-lived, in-memory background operation. It
// deliberately contains only the already-safe action view; certificate PEM
// material and DNS credentials never enter task responses or log sessions.
type AcmeTaskView struct {
	ID           string            `json:"id"`
	Operation    string            `json:"operation"`
	Status       string            `json:"status"`
	LogSessionID string            `json:"logSessionId"`
	StartedAt    int64             `json:"startedAt"`
	UpdatedAt    int64             `json:"updatedAt"`
	FinishedAt   int64             `json:"finishedAt,omitempty"`
	Error        string            `json:"error,omitempty"`
	Warnings     []string          `json:"warnings,omitempty"`
	Result       *AcmeActionResult `json:"result,omitempty"`
}

type AcmeInstallTaskStatus struct {
	ManagedDownloadTaskStatus
	LogSessionID string `json:"logSessionId,omitempty"`
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

type acmeDNSRuntimeConfig struct {
	ProviderCode string
	EnvPairs     []string
	AccountName  string
}

type AcmeLogSessionView struct {
	Id         string            `json:"id"`
	Title      string            `json:"title"`
	Status     string            `json:"status"`
	Lines      []string          `json:"lines"`
	LineStart  int               `json:"lineStart"`
	LineNext   int               `json:"lineNext"`
	Error      string            `json:"error,omitempty"`
	TaskID     string            `json:"taskId,omitempty"`
	TaskStatus string            `json:"taskStatus,omitempty"`
	Warnings   []string          `json:"warnings,omitempty"`
	Result     *AcmeActionResult `json:"result,omitempty"`
	StartedAt  int64             `json:"startedAt"`
	UpdatedAt  int64             `json:"updatedAt"`
	FinishedAt int64             `json:"finishedAt,omitempty"`
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
	overview := &AcmeOverview{
		Supported: IsSystemPlatformLinux(),
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
		version, err := readAcmeVersionFromScriptFile(scriptPath)
		if err != nil {
			overview.Error = strings.TrimSpace(err.Error())
		} else {
			overview.Version = version
		}
	}

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
	if err := s.cleanupNonDNSCertificateDNSReferences(); err != nil {
		return err
	}
	if err := s.scrubLegacyAcmeCertificateRuntimeFields(); err != nil {
		return err
	}
	if err := certificateInventory.RepairDisplayIDs(); err != nil {
		return err
	}
	if err := repairAcmeAccountDisplayIDs(database.GetDB()); err != nil {
		return err
	}
	if err := repairDNSAccountDisplayIDs(database.GetDB()); err != nil {
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

func (s *AcmeService) GetLogSessionAfter(id string, after int) (*AcmeLogSessionView, error) {
	return acmeLogSessionStore.getAfter(id, after), nil
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

// MigrateLegacyAcmeRuntimeOnStartup performs the one-time conversion from the
// retired shared acme.sh directory and mirror table into database-only state.
func (s *AcmeService) MigrateLegacyAcmeRuntimeOnStartup() error {
	return s.migrateLegacyAcmeRuntimeOnStartup()
}

func (s *AcmeService) GetRemoteVersionsPage(page int, perPage int) (*AcmeVersionListResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if !IsSystemPlatformLinux() {
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
		Supported: IsSystemPlatformLinux(),
		Installed: false,
	}
	if !result.Supported {
		result.Message = "ACME certificate management is only supported on Linux"
		return result, nil
	}

	scriptPath, _, installed := s.resolveAcmeScript()
	result.Installed = installed
	if installed {
		current, err := readAcmeVersionFromScriptFile(scriptPath)
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
	return s.installOrReinstallWithContext(context.Background(), payload, nil, nil, nil)
}

func (s *AcmeService) installOrReinstallWithContext(ctx context.Context, payload AcmeInstallPayload, report func(string), beginApplying func() bool, logSession *acmeLogSession) (*AcmeActionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := lockManagedDownloadTaskMutex(ctx, &acmeOperationMu); err != nil {
		return nil, err
	}
	defer acmeOperationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if report != nil {
		report("preparing")
	}

	if !IsSystemPlatformLinux() {
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
		ok, checkErr := s.checkVersionDownloadableLockedContext(ctx, version)
		if checkErr != nil {
			return nil, checkErr
		}
		if !ok {
			return nil, common.NewError("selected acme.sh version is unavailable: ", version)
		}
	}

	beforeVersion := ""
	if scriptPath, homeDir, installed := s.resolveAcmeScript(); installed {
		if v, err := readAcmeVersionByScriptContext(ctx, scriptPath, homeDir); err == nil {
			beforeVersion = v
		}
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

	// acme.sh's online installer creates its archive in the current working
	// directory. Keep both that archive and the installer script in this owned
	// workspace so every failure path can remove them together.
	tmpFile, err := os.CreateTemp(stagedHomeDir, ".acme-installer-*.sh")
	if err != nil {
		return nil, common.NewError("create acme installer temp file failed: ", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if report != nil {
		report("downloading")
	}
	if err := downloadAcmeInstallerScriptContext(ctx, tmpPath); err != nil {
		return nil, err
	}

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
	if report != nil {
		report("installing")
	}
	output, err := runCommandOutputWithTimeoutEnvContextLogInDir(ctx, 90*time.Second, shellPath, args, nil, logSession, stagedHomeDir)
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
	if _, err := readAcmeVersionByScriptContext(ctx, stagedScriptPath, stagedHomeDir); err != nil {
		detail := summarizeAcmeInstallOutput(output)
		if detail != "" {
			return nil, common.NewError("staged acme.sh install is incomplete: ", detail)
		}
		return nil, common.NewError("staged acme.sh install is incomplete: ", err)
	}
	if beginApplying != nil && !beginApplying() {
		return nil, coreDownloadTaskCancelledError(ctx)
	}
	if report != nil {
		report("applying")
	}

	acmeOwnership, err := BeginAcmeHostOwnership([]string{managedAcmeHomeDir()}, nil)
	if err != nil {
		return nil, common.NewError("record pending managed acme ownership failed: ", err)
	}
	ownershipActivated := false
	defer func() {
		if !ownershipActivated && acmeOwnership.ID != "" {
			_ = RemoveHostResource(acmeOwnership.ID)
		}
	}()
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
	if err := VerifyAndActivateHostResource(acmeOwnership.ID); err != nil {
		return nil, common.NewError("activate managed acme ownership failed: ", err)
	}
	ownershipActivated = true
	if err := syncManagedAcmeCertificateOwnership(); err != nil {
		return nil, common.NewError("refresh managed acme ownership failed: ", err)
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

func acmeInstallTaskFingerprint(payload AcmeInstallPayload) string {
	value := strings.TrimSpace(payload.Version) + "\x00" + normalizeAcmeEmail(payload.Email) + "\x00" + strconv.FormatBool(payload.EmailProvided)
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func startAcmeInstallTaskLog(taskID string, ctx context.Context) *acmeLogSession {
	logSession := acmeLogSessionStore.start("log-"+strings.TrimSpace(taskID), "acme.sh 下载 / 重装")
	if logSession == nil {
		return nil
	}
	acmeLogSessionStore.mu.Lock()
	logSession.taskID = strings.TrimSpace(taskID)
	logSession.taskStatus = managedDownloadTaskRunning
	logSession.ctx = ctx
	logSession.updatedAt = time.Now().Unix()
	acmeLogSessionStore.mu.Unlock()
	logSession.append("后台下载任务已启动")
	return logSession
}

func (s *AcmeService) StartManagedInstallOrReinstall(payload AcmeInstallPayload) (AcmeInstallTaskStatus, error) {
	handle, taskStatus, created, err := acmeInstallTaskManager.Start("acme-install", acmeInstallTaskFingerprint(payload))
	if err != nil {
		return AcmeInstallTaskStatus{}, err
	}
	if !created {
		return s.GetManagedAcmeInstall(taskStatus.ID), nil
	}
	logSessionID, err := acmeTaskSessionStore.enqueueManaged("install", "acme.sh 下载 / 重装", handle, func(queuedLogSessionID string) (*AcmeActionResult, error) {
		return s.runManagedInstallOrReinstall(handle, payload, queuedLogSessionID)
	})
	if err != nil {
		return AcmeInstallTaskStatus{}, err
	}
	acmeInstallTaskState.Lock()
	acmeInstallTaskState.id = handle.ID()
	acmeInstallTaskState.logSessionID = logSessionID
	acmeInstallTaskState.Unlock()
	return AcmeInstallTaskStatus{ManagedDownloadTaskStatus: taskStatus, LogSessionID: logSessionID}, nil
}

func (s *AcmeService) runManagedInstallOrReinstall(handle *ManagedDownloadTaskHandle, payload AcmeInstallPayload, logSessionID string) (*AcmeActionResult, error) {
	if handle == nil || !handle.MarkRunning("preparing") {
		if handle != nil {
			handle.FinishCancelled("cancelled")
		}
		return nil, context.Canceled
	}
	ctx := handle.Context()
	logSession := startAcmeInstallTaskLog(handle.ID(), ctx)
	result, installErr := s.installOrReinstallWithContext(ctx, payload, func(phase string) {
		handle.SetPhase(phase, true)
		if logSession != nil {
			logSession.append("阶段: " + phase)
		}
	}, func() bool {
		return handle.BeginApplying("applying")
	}, logSession)
	if installErr != nil {
		if ctx.Err() != nil {
			handle.FinishCancelled("cancelled")
		} else {
			handle.FinishError("failed", installErr)
		}
		status := handle.Snapshot()
		if logSession != nil {
			logSession.fail(firstNonEmpty(status.Error, installErr.Error()))
		}
		return nil, installErr
	}
	handle.FinishSuccess("completed")
	if logSession != nil {
		logSession.finish("acme.sh 下载 / 重装完成")
	}
	_ = logSessionID
	return result, nil
}

func (s *AcmeService) GetManagedAcmeInstall(id string) AcmeInstallTaskStatus {
	status := acmeInstallTaskManager.Get(id)
	if status.State == managedDownloadTaskIdle {
		return AcmeInstallTaskStatus{ManagedDownloadTaskStatus: status}
	}
	acmeInstallTaskState.Lock()
	logSessionID := ""
	if strings.TrimSpace(id) == "" || acmeInstallTaskState.id == strings.TrimSpace(id) || acmeInstallTaskState.id == status.ID {
		logSessionID = acmeInstallTaskState.logSessionID
	}
	acmeInstallTaskState.Unlock()
	return AcmeInstallTaskStatus{ManagedDownloadTaskStatus: status, LogSessionID: logSessionID}
}

func (s *AcmeService) StopManagedAcmeInstall(id string) (AcmeInstallTaskStatus, error) {
	status, err := acmeInstallTaskManager.Stop(id)
	if err == nil {
		acmeTaskSessionStore.finishQueuedManagedIfCancelled(status.ID)
	}
	result := s.GetManagedAcmeInstall(status.ID)
	if result.LogSessionID != "" {
		acmeLogSessionStore.mu.Lock()
		logSession := acmeLogSessionStore.sessions[result.LogSessionID]
		acmeLogSessionStore.mu.Unlock()
		if logSession != nil {
			logSession.append("正在停止下载任务")
		}
	}
	return result, err
}

func (s *AcmeService) Upgrade() (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if !IsSystemPlatformLinux() {
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

	if !IsSystemPlatformLinux() {
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
	finishOperation, operationErr := logSession.ensureManagedOperation("issue")
	if operationErr != nil {
		logSession.fail(operationErr.Error())
		return nil, operationErr
	}
	defer finishOperation()
	logSession.append("进入 ACME 签发队列")
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()
	if err := logSession.operationContext().Err(); err != nil {
		logSession.fail("ACME 签发任务已取消")
		return nil, err
	}
	logSession.append("开始执行 ACME 签发")

	if !IsSystemPlatformLinux() {
		logSession.fail("ACME certificate management is only supported on Linux")
		return nil, common.NewError("ACME certificate management is only supported on Linux")
	}

	scriptPath, homeDir, installed := s.resolveAcmeScript()
	if !installed {
		logSession.fail("acme.sh is not installed")
		return nil, common.NewError("acme.sh is not installed")
	}
	logSession.append("已找到 acme.sh: " + scriptPath)

	var existingRecord *model.CertificateRecord
	if payload.ExistingRecordID > 0 {
		existing, err := certificateInventory.GetRecordByID(payload.ExistingRecordID)
		if err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
		if strings.TrimSpace(existing.SourceType) != CertificateSourceACME {
			message := "只有 ACME 证书可以编辑并重新签发"
			logSession.fail(message)
			return nil, common.NewError(message)
		}
		existingRecord = existing
		applyAcmeReissueDefaults(&payload, existingRecord)
	}

	certificateType := normalizeAcmeCertificateType(payload.CertificateType)
	if existingRecord != nil {
		if normalizeAcmeCertificateType(existingRecord.CertificateType) != certificateType {
			message := "编辑并重新签发不能改变证书类型"
			logSession.fail(message)
			return nil, common.NewError(message)
		}
	}
	if certificateType == acmeCertificateTypeIP {
		logSession.append("证书类型: IP 证书")
	} else {
		logSession.append("证书类型: 域名证书")
		if payload.AcmeAccountID == 0 {
			logSession.fail("域名证书签发必须选择 ACME 账号")
			return nil, common.NewError("域名证书签发必须选择 ACME 账号")
		}
	}

	domains, domainErr := validateAcmeIssueIdentifiers(payload.DomainsText, certificateType)
	if domainErr != nil {
		logSession.fail(domainErr.Error())
		return nil, domainErr
	}
	if len(domains) == 0 {
		message := "domain list is required"
		if certificateType == acmeCertificateTypeIP {
			message = "IP list is required"
		}
		logSession.fail(message)
		return nil, common.NewError(message)
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
	if certificateType == acmeCertificateTypeDomain && hasAcmeWildcardDomain(domains) && challenge != "dns" {
		message := "通配符域名只能使用 DNS 验证"
		logSession.fail(message)
		return nil, common.NewError(message)
	}
	logSession.append("验证方式: " + challenge)
	ipFamilyMode := detectAcmeIPFamilyMode(domains)
	if certificateType == acmeCertificateTypeIP {
		logSession.append("IP 地址族: " + acmeIPFamilyModeLabel(ipFamilyMode))
		logSession.append("IP 证书校验走 HTTP/TLS 到字面 IP，不走 DNS-01")
		logSession.append("端口空闲只代表本机未占用，不代表外部 IPv6 一定可达")
	}
	keyLength := normalizeAcmeKeyLength(payload.KeyLength)
	if keyLength == "" && !payload.KeyLengthProvided {
		keyLength = normalizeAcmeKeyLength(s.readSettingWithDefault(acmeDefaultKeyLengthKey, defaultAcmeKeyLength))
	}
	if keyLength == "" {
		keyLength = defaultAcmeKeyLength
	}
	useECC := strings.HasPrefix(strings.ToLower(keyLength), "ec-")
	logSession.append("证书算法: " + keyLength)

	webroot := strings.TrimSpace(payload.Webroot)
	if webroot == "" && !payload.WebrootProvided {
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
	if useDNSChallenge && dnsProvider == "" && !payload.DNSProviderProvided {
		dnsProvider = strings.TrimSpace(s.readSettingWithDefault(acmeDefaultDNSProviderKey, ""))
	}
	if !useDNSChallenge {
		dnsProvider = ""
	}
	dnsEnvText := strings.TrimSpace(payload.DNSEnvText)
	customArgs, customArgsErr := validateAcmeCustomArgs(payload.CustomArgs)
	if customArgsErr != nil {
		logSession.fail(customArgsErr.Error())
		return nil, customArgsErr
	}
	var account *model.AcmeAccount
	caServer := ""
	if certificateType == acmeCertificateTypeDomain {
		if err := database.GetDB().Where("id = ? AND system = ?", payload.AcmeAccountID, false).First(&account).Error; err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
		caServer = normalizeSupportedAcmeDomainServer(account.Server)
		if caServer == "" {
			message := "所选 ACME 账号的 CA 平台无效，仅支持 Let's Encrypt 或 ZeroSSL"
			logSession.fail(message)
			return nil, common.NewError(message)
		}
		logSession.append("使用 ACME 账号: " + strings.TrimSpace(account.Name))
	} else {
		contact := strings.TrimSpace(s.readSettingWithDefault(acmeContactEmailKey, ""))
		var accountErr error
		account, accountErr = s.ensureIPRuntimeAccount(contact)
		if accountErr != nil {
			logSession.fail(accountErr.Error())
			return nil, accountErr
		}
		caServer = acmeLEProductionDirectory
		logSession.append("IP 证书使用独立系统运行态")
	}
	logSession.append("最终 CA 平台: " + caServer)
	logSession.append("最终证书算法: " + keyLength)

	dnsEnv := []string{}
	var dnsAccount *model.AcmeDNSAccount
	manualDNSProvider := ""
	manualDNSEnvText := ""
	manualDNSAccountID := uint(0)
	manualDNSAccountCommitted := false
	if useDNSChallenge {
		if payload.DNSAccountID > 0 && strings.TrimSpace(dnsEnvText) != "" {
			message := "已选择 DNS 账号时不能同时填写手工 DNS 环境变量；请先保存到该账号或取消选择后自动新建账号"
			logSession.fail(message)
			return nil, common.NewError(message)
		}
		if payload.DNSAccountID == 0 {
			if strings.TrimSpace(dnsProvider) == "" {
				message := "手工 DNS 凭据必须选择 DNS Provider"
				logSession.fail(message)
				return nil, common.NewError(message)
			}
			runtimeDNS, runtimeErr := resolveAcmeDNSRuntimeConfig(0, dnsProvider, dnsEnvText)
			if runtimeErr != nil {
				logSession.fail(runtimeErr.Error())
				return nil, runtimeErr
			}
			dnsProvider = runtimeDNS.ProviderCode
			dnsEnv = runtimeDNS.EnvPairs
			manualDNSProvider = dnsProvider
			manualDNSEnvText = dnsEnvText
			logSession.append("手工 DNS 凭据已校验，将在 ACME 签发成功后创建并绑定 DNS 账号")
		} else {
			runtimeDNS, runtimeErr := resolveAcmeDNSRuntimeConfig(payload.DNSAccountID, dnsProvider, "")
			if runtimeErr != nil {
				logSession.fail(runtimeErr.Error())
				return nil, runtimeErr
			}
			requestedProvider := strings.TrimSpace(dnsProvider)
			if providerMeta, ok := lookupAcmeDNSProvider(requestedProvider); ok {
				requestedProvider = providerMeta.ProviderCode
			}
			if payload.DNSProviderProvided && requestedProvider != "" && requestedProvider != runtimeDNS.ProviderCode {
				message := "已选择 DNS 账号时必须使用该账号绑定的 DNS Provider"
				logSession.fail(message)
				return nil, common.NewError(message)
			}
			dnsProvider = runtimeDNS.ProviderCode
			dnsEnv = runtimeDNS.EnvPairs
			dnsAccount = &model.AcmeDNSAccount{}
			if err := database.GetDB().Where("id = ?", payload.DNSAccountID).First(dnsAccount).Error; err != nil {
				logSession.fail(err.Error())
				return nil, err
			}
			if runtimeDNS.AccountName != "" {
				logSession.append("使用 DNS 账号: " + runtimeDNS.AccountName)
			}
		}
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
		if err := ensureAcmeDNSProviderScript(homeDir, dnsProvider); err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
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
	runtime, runtimeErr := newAcmeOperationRuntime(account)
	if runtimeErr != nil {
		logSession.fail(runtimeErr.Error())
		return nil, runtimeErr
	}
	runtimeSnapshotSaved := false
	defer func() {
		if !runtimeSnapshotSaved {
			if snapshotErr := runtime.snapshot(); snapshotErr != nil && logSession != nil {
				logSession.append("保存 ACME 临时运行态失败: " + snapshotErr.Error())
			}
		}
		runtime.cleanup()
	}()
	if err := s.ensureOperationRuntimeAccount(scriptPath, homeDir, runtime, account, caServer, logSession); err != nil {
		logSession.fail(err.Error())
		return nil, common.NewError("准备 ACME 账号运行态失败: ", err)
	}
	commandArgs := buildAcmeIssueCommandArgs(domains, challenge, webroot, dnsProvider, keyLength, caServer, customArgs, certificateType == acmeCertificateTypeIP, ipFamilyMode)
	commandArgs = ensureAcmeFreshIssueArgs(commandArgs)
	logSession.append("手动签发默认强制重新签发，避免复用旧证书")

	logSession.append("执行 acme.sh --issue")
	output, err := runCommandOutputWithTimeoutEnvLog(acmeTaskDeadline, scriptPath, append(runtime.commandArgs(homeDir), commandArgs...), dnsEnv, logSession)
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
	if err := runtime.snapshot(); err != nil {
		logSession.fail(err.Error())
		return nil, err
	}
	runtimeSnapshotSaved = true

	if skippedBecauseDomainsUnchanged {
		logSession.append("未触发重新签发，开始安装已有证书文件")
	} else {
		logSession.append("签发成功，开始安装证书文件")
	}
	paths, cleanupInstalledCert, err := s.installCertToManagedDirWithArgs(scriptPath, runtime.commandArgs(homeDir), domains[0], useECC, dnsEnv, logSession)
	if err != nil {
		logSession.fail(err.Error())
		return nil, err
	}
	defer cleanupInstalledCert()
	if manualDNSProvider != "" {
		created, createErr := s.createManualDNSAccountLocked(manualDNSProvider, manualDNSEnvText)
		if createErr != nil {
			logSession.fail(createErr.Error())
			return nil, createErr
		}
		dnsAccount = created
		manualDNSAccountID = created.Id
		defer func() {
			if manualDNSAccountID == 0 || manualDNSAccountCommitted {
				return
			}
			_ = database.GetDB().Where("id = ?", manualDNSAccountID).Delete(&model.AcmeDNSAccount{}).Error
		}()
		logSession.append("已创建并准备绑定 DNS 账号: " + created.Name)
	}

	certEntry, err := s.upsertAcmeCertificateRecordFromPaths(payload.ExistingRecordID, domains, certificateType, challenge, keyLength, caServer, useECC, webroot, dnsProvider, customArgs, account, dnsAccount, paths, payload.AutoRenew, payload.Remark, payload.ApplyTarget, payload.PushDir)
	if err != nil {
		logSession.fail(err.Error())
		return nil, err
	}
	manualDNSAccountCommitted = manualDNSAccountID == 0 || certEntry.DNSAccountID == manualDNSAccountID
	certEntry.LastOutput = truncateAcmeStoredOutput(output)
	if err := database.GetDB().Save(certEntry).Error; err != nil {
		logSession.fail(err.Error())
		return nil, err
	}

	warnings := make([]string, 0)
	if certificateType != acmeCertificateTypeIP {
		if err := s.persistAcmeDefaults(payload, challenge, keyLength, caServer, dnsProvider, useDNSChallenge); err != nil {
			warnings = append(warnings, "保存 ACME 默认配置失败: "+strings.TrimSpace(err.Error()))
		}
	}

	logSession.append("执行签发后动作")
	materialChanged := existingRecord == nil || normalizeCertificateFingerprint(existingRecord.Fingerprint) != normalizeCertificateFingerprint(certEntry.Fingerprint)
	warnings = append(warnings, s.applyCertificateRecordPostActionsWithCoreRestart(certEntry, payload.ApplyTarget, payload.PushDir, payload.PushExplicit, materialChanged)...)
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		warnings = append(warnings, "刷新证书运行态一致性失败: "+strings.TrimSpace(err.Error()))
	}
	warnings = persistCertificatePostActionWarnings(certEntry, warnings)

	var overview *AcmeOverview
	if loadedOverview, overviewErr := s.GetOverview(); overviewErr != nil {
		warnings = persistCertificatePostActionWarnings(certEntry, append(warnings, "刷新证书概览失败: "+strings.TrimSpace(overviewErr.Error())))
	} else {
		overview = loadedOverview
	}
	view := convertCertificateRecord(certEntry)
	if len(warnings) > 0 {
		logSession.finish("证书签发完成，但有后置动作警告")
	} else if skippedBecauseDomainsUnchanged {
		logSession.finish("域名未变化，已同步已有证书")
	} else {
		logSession.finish("证书签发完成")
	}
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
		Output:      strings.TrimSpace(output),
		Warnings:    warnings,
	}, nil
}

func (s *AcmeService) Renew(payload AcmeRenewPayload) (*AcmeActionResult, error) {
	logSession := acmeLogSessionStore.start(payload.LogSessionID, "证书续签")
	finishOperation, operationErr := logSession.ensureManagedOperation("renew")
	if operationErr != nil {
		logSession.fail(operationErr.Error())
		return nil, operationErr
	}
	defer finishOperation()
	logSession.append("进入 ACME 续签队列")
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()
	if err := logSession.operationContext().Err(); err != nil {
		logSession.fail("ACME 续签任务已取消")
		return nil, err
	}
	logSession.append("开始执行 ACME 续签")

	if payload.ID == 0 {
		logSession.fail("certificate id is required")
		return nil, common.NewError("certificate id is required")
	}
	row, getErr := certificateInventory.GetRecordByID(payload.ID)
	if getErr != nil {
		logSession.fail(getErr.Error())
		return nil, getErr
	}
	if row.SourceType != CertificateSourceACME {
		result, err := s.renewInventorySelfSignedCertificate(row)
		if err != nil {
			logSession.fail(err.Error())
			return nil, err
		}
		logSession.finish("自签证书续签完成")
		return result, nil
	}
	if !IsSystemPlatformLinux() {
		logSession.fail("ACME certificate management is only supported on Linux")
		return nil, common.NewError("ACME certificate management is only supported on Linux")
	}

	scriptPath, homeDir, installed := s.resolveAcmeScript()
	if !installed {
		logSession.fail("acme.sh is not installed")
		return nil, common.NewError("acme.sh is not installed")
	}

	certificateType := normalizeAcmeCertificateType(row.CertificateType)
	isIPCert := certificateType == acmeCertificateTypeIP
	domains := decodeCertificateDomains(row.DomainSet)
	if len(domains) == 0 && strings.TrimSpace(row.MainDomain) != "" {
		domains = []string{strings.TrimSpace(row.MainDomain)}
	}
	if len(domains) == 0 {
		return nil, common.NewError("certificate domains are empty")
	}
	challenge := normalizeAcmeChallenge(row.Challenge)
	if challenge == "" {
		challenge = "standalone"
	}
	if isIPCert {
		challenge = normalizeAcmeIPChallenge(challenge)
		if challenge == "" {
			err := common.NewError("IP certificates only support standalone or alpn challenge")
			_ = s.markCertificateError(row.Id, err.Error())
			return nil, err
		}
	}
	webroot := strings.TrimSpace(row.Webroot)
	if webroot == "" {
		webroot = strings.TrimSpace(s.readSettingWithDefault(acmeDefaultWebrootKey, ""))
	}
	keyLength := normalizeAcmeKeyLength(row.KeyLength)
	if keyLength == "" {
		keyLength = normalizeAcmeKeyLength(s.readSettingWithDefault(acmeDefaultKeyLengthKey, defaultAcmeKeyLength))
	}
	if keyLength == "" {
		keyLength = defaultAcmeKeyLength
	}
	useECC := strings.HasPrefix(strings.ToLower(keyLength), "ec-")
	dnsProvider := strings.TrimSpace(row.DNSProvider)
	customArgs, customArgsErr := validateAcmeCustomArgs(row.CustomArgs)
	if customArgsErr != nil {
		_ = s.markCertificateError(row.Id, customArgsErr.Error())
		return nil, customArgsErr
	}
	useDNSChallenge := shouldUseAcmeDNSChallenge(certificateType, challenge)
	ipFamilyMode := acmeIPFamilyUnknown
	if isIPCert {
		ipFamilyMode = detectAcmeIPFamilyMode(domains)
	}

	logSession.append("续签目标: " + row.MainDomain)
	if isIPCert {
		logSession.append("IP 地址族: " + acmeIPFamilyModeLabel(ipFamilyMode))
		logSession.append("端口空闲只代表本机未占用，不代表外部 IPv6 一定可达")
		logAcmeIPFamilyListenStrategy(logSession, ipFamilyMode)
	}

	var challengeDecision acmeChallengePortDecision
	if !useDNSChallenge && isAcmePortChallenge(challenge) {
		snapshot, snapshotErr := collectAcmeChallengePortSnapshot()
		if snapshotErr != nil {
			logSession.fail(snapshotErr.Error())
			_ = s.markCertificateError(row.Id, snapshotErr.Error())
			return nil, snapshotErr
		}
		decision, decisionErr := selectAcmeChallengePortDecision(certificateType, challenge, snapshot)
		if decisionErr != nil {
			logSession.fail(decisionErr.Error())
			_ = s.markCertificateError(row.Id, decisionErr.Error())
			return nil, decisionErr
		}
		challengeDecision = decision
		challenge = decision.Challenge
		useDNSChallenge = shouldUseAcmeDNSChallenge(certificateType, challenge)
		if challengeDecision.Switched {
			logSession.append(fmt.Sprintf("port challenge switched: %s -> %s (%s)", challengeDecision.InputChallenge, challengeDecision.Challenge, challengeDecision.Reason))
		} else {
			logSession.append(fmt.Sprintf("port challenge selected: %s (%s)", challengeDecision.Challenge, challengeDecision.Reason))
		}
	}
	if challenge == "webroot" && strings.TrimSpace(webroot) == "" {
		err := common.NewError("webroot challenge requires webroot path")
		_ = s.markCertificateError(row.Id, err.Error())
		return nil, err
	}
	renewEnv := []string{}
	var dnsAccount *model.AcmeDNSAccount
	if useDNSChallenge {
		if row.DNSAccountID == 0 {
			err := common.NewError("DNS 账号已删除，证书不可自动续签")
			_ = s.disableAutoRenewForMissingAccount(row.Id, err.Error())
			return nil, err
		}
		runtimeDNS, runtimeErr := resolveAcmeDNSRuntimeConfig(row.DNSAccountID, dnsProvider, "")
		if runtimeErr != nil {
			if database.IsNotFound(runtimeErr) {
				_ = s.disableAutoRenewForMissingAccount(row.Id, "DNS 账号已删除，证书不可自动续签")
			}
			_ = s.markCertificateError(row.Id, runtimeErr.Error())
			return nil, runtimeErr
		}
		dnsProvider = runtimeDNS.ProviderCode
		renewEnv = runtimeDNS.EnvPairs
		dnsAccount = &model.AcmeDNSAccount{}
		if err := database.GetDB().Where("id = ?", row.DNSAccountID).First(dnsAccount).Error; err != nil {
			_ = s.disableAutoRenewForMissingAccount(row.Id, "DNS 账号已删除，证书不可自动续签")
			return nil, err
		}
	}
	if challenge == "dns" && strings.TrimSpace(dnsProvider) == "" {
		err := common.NewError("dns challenge requires dns provider (for example dns_cf)")
		_ = s.markCertificateError(row.Id, err.Error())
		return nil, err
	}
	if challenge == "dns" {
		if err := ensureAcmeDNSProviderScript(homeDir, dnsProvider); err != nil {
			logSession.fail(err.Error())
			_ = s.markCertificateError(row.Id, err.Error())
			return nil, err
		}
	}

	var tempFirewall *acmeTemporaryFirewallRule
	if !useDNSChallenge && isAcmePortChallenge(challenge) && challengeDecision.Port > 0 {
		prepared, prepareErr := s.prepareTemporaryAcmeFirewallRule(challengeDecision.Port, logSession)
		if prepareErr != nil {
			logSession.fail(prepareErr.Error())
			_ = s.markCertificateError(row.Id, prepareErr.Error())
			return nil, prepareErr
		}
		tempFirewall = prepared
		if tempFirewall != nil {
			defer s.cleanupTemporaryAcmeFirewallRule(tempFirewall, logSession)
		}
	}

	var account *model.AcmeAccount
	caServer := ""
	if isIPCert {
		contact := strings.TrimSpace(s.readSettingWithDefault(acmeContactEmailKey, ""))
		var accountErr error
		account, accountErr = s.ensureIPRuntimeAccount(contact)
		if accountErr != nil {
			return nil, accountErr
		}
		caServer = acmeLEProductionDirectory
	} else {
		if row.AcmeAccountID == 0 {
			err := common.NewError("ACME 账号已删除，证书不可自动续签")
			_ = s.disableAutoRenewForMissingAccount(row.Id, err.Error())
			return nil, err
		}
		account = &model.AcmeAccount{}
		if err := database.GetDB().Where("id = ? AND system = ?", row.AcmeAccountID, false).First(account).Error; err != nil {
			_ = s.disableAutoRenewForMissingAccount(row.Id, "ACME 账号已删除，证书不可自动续签")
			return nil, err
		}
		caServer = normalizeSupportedAcmeDomainServer(account.Server)
		if caServer == "" {
			err := common.NewError("所选 ACME 账号的 CA 平台无效")
			_ = s.markCertificateError(row.Id, err.Error())
			return nil, err
		}
	}
	runtime, runtimeErr := newAcmeOperationRuntime(account)
	if runtimeErr != nil {
		return nil, runtimeErr
	}
	runtimeSnapshotSaved := false
	defer func() {
		if !runtimeSnapshotSaved {
			_ = runtime.snapshot()
		}
		runtime.cleanup()
	}()
	if err := s.ensureOperationRuntimeAccount(scriptPath, homeDir, runtime, account, caServer, logSession); err != nil {
		_ = s.markCertificateError(row.Id, err.Error())
		return nil, common.NewError("准备 ACME 账号运行态失败: ", err)
	}
	commandArgs := buildAcmeIssueCommandArgs(domains, challenge, webroot, dnsProvider, keyLength, caServer, customArgs, isIPCert, ipFamilyMode)
	if shouldForceAcmeRenew(payload) {
		commandArgs = ensureAcmeFreshIssueArgs(commandArgs)
		logSession.append("强制续签已附加 --force")
	} else {
		logSession.append("普通续签不附加 --force；域名未变化时只同步现有证书")
	}
	output, err := runCommandOutputWithTimeoutEnvLog(acmeTaskDeadline, scriptPath, append(runtime.commandArgs(homeDir), commandArgs...), renewEnv, logSession)
	skippedBecauseDomainsUnchanged := false
	if err != nil {
		if isAcmeDomainsNotChangedError(err) {
			skippedBecauseDomainsUnchanged = true
			output = strings.TrimSpace(err.Error())
			logSession.append("域名未变化，继续同步已有证书文件")
		} else {
			logSession.fail(err.Error())
			_ = s.markCertificateError(row.Id, err.Error())
			return nil, common.NewError("renew certificate failed: ", err)
		}
	}
	if err := runtime.snapshot(); err != nil {
		return nil, err
	}
	runtimeSnapshotSaved = true

	paths, cleanupInstalledCert, tempErr := s.installCertToManagedDirWithArgs(scriptPath, runtime.commandArgs(homeDir), row.MainDomain, useECC, renewEnv, logSession)
	if tempErr != nil {
		return nil, tempErr
	}
	defer cleanupInstalledCert()

	updated, err := s.upsertAcmeCertificateRecordFromPaths(
		row.Id,
		domains,
		certificateType,
		challenge,
		keyLength,
		caServer,
		useECC,
		webroot,
		dnsProvider,
		customArgs,
		account,
		dnsAccount,
		paths,
		row.AutoRenew,
		row.Remark,
		row.ApplyTarget,
		row.PushDir,
	)
	if err != nil {
		return nil, err
	}
	updated.LastOutput = truncateAcmeStoredOutput(output)
	if err := database.GetDB().Save(updated).Error; err != nil {
		return nil, err
	}

	applyTarget := strings.TrimSpace(payload.ApplyTarget)
	materialChanged := normalizeCertificateFingerprint(row.Fingerprint) != normalizeCertificateFingerprint(updated.Fingerprint)
	warnings := s.applyCertificateRecordPostActionsWithCoreRestart(updated, applyTarget, "", false, materialChanged)
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		warnings = append(warnings, "刷新证书运行态一致性失败: "+strings.TrimSpace(err.Error()))
	}
	warnings = persistCertificatePostActionWarnings(updated, warnings)

	var overview *AcmeOverview
	if loadedOverview, overviewErr := s.GetOverview(); overviewErr != nil {
		warnings = persistCertificatePostActionWarnings(updated, append(warnings, "刷新证书概览失败: "+strings.TrimSpace(overviewErr.Error())))
	} else {
		overview = loadedOverview
	}
	view := convertCertificateRecord(updated)
	if len(warnings) > 0 {
		logSession.finish("证书续签完成，但有后置动作警告")
	} else if skippedBecauseDomainsUnchanged {
		logSession.finish("域名未变化，已同步已有证书")
	} else {
		logSession.finish("证书续签完成")
	}
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
		Output:      strings.TrimSpace(output),
		Warnings:    warnings,
	}, nil
}

func shouldForceAcmeRenew(payload AcmeRenewPayload) bool {
	return payload.Force
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
	if payload.Clear {
		if !row.PushEnabled {
			return nil, common.NewError("certificate has no verified directory push state")
		}
		if err := removeVerifiedCertificateFiles(row.PushDir, row.PushFilePaths); err != nil {
			return nil, err
		}
		row.PushEnabled = false
		row.PushDir = ""
		row.PushFilePaths = ""
		row.PushFiles = ""
		if err := persistCertificatePushState(row); err != nil {
			return nil, err
		}
		if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
			return nil, err
		}
		if err := clearCertificatePostActionError(row); err != nil {
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
			Msg:         "证书目录推送记录已清除",
		}, nil
	}
	targetDir := strings.TrimSpace(payload.TargetDir)
	if targetDir == "" {
		return nil, common.NewError("target directory is required")
	}
	if strings.TrimSpace(row.SourceType) == CertificateSourceACME {
		if _, err := beginManagedAcmeCertificatePushOwnership(targetDir); err != nil {
			return nil, common.NewError("record pending certificate push ownership failed: ", err)
		}
	}

	pushState, err := syncCertificateDirectoryPushState(targetDir, row.PushEnabled, row.PushDir, row.PushFilePaths, row.CertPEM, row.KeyPEM, row.FullchainPEM, row.ChainPEM)
	if err != nil {
		return nil, err
	}

	row.PushEnabled = pushState.PushEnabled
	row.PushDir = pushState.PushDir
	row.PushFilePaths = pushState.PushFilePaths
	row.PushFiles = ""
	if err := persistCertificatePushState(row); err != nil {
		return nil, err
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}
	if err := clearCertificatePostActionError(row); err != nil {
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
	if err := clearCertificatePostActionError(row); err != nil {
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
		clearCertificateAutoRenewRetryFields(row)
		if payload.AutoRenew {
			row.LastError = ""
		}
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
	if payload.AutoRenew {
		certificateType := normalizeAcmeCertificateType(row.CertificateType)
		if certificateType != acmeCertificateTypeIP {
			if row.AcmeAccountID == 0 {
				return nil, common.NewError("请在“编辑并重新签发”中选择 ACME 账号后再开启自动续签")
			}
			account := &model.AcmeAccount{}
			if err := database.GetDB().Where("id = ? AND system = ?", row.AcmeAccountID, false).First(account).Error; err != nil {
				return nil, common.NewError("关联的 ACME 账号不存在，请重新签发后再开启自动续签")
			}
		}
		if shouldUseAcmeDNSChallenge(certificateType, row.Challenge) {
			if row.DNSAccountID == 0 {
				return nil, common.NewError("请在“编辑并重新签发”中选择 DNS 账号后再开启自动续签")
			}
			dnsAccount := &model.AcmeDNSAccount{}
			if err := database.GetDB().Where("id = ?", row.DNSAccountID).First(dnsAccount).Error; err != nil {
				return nil, common.NewError("关联的 DNS 账号不存在，请重新签发后再开启自动续签")
			}
		}
	}
	row.AutoRenew = payload.AutoRenew
	clearCertificateAutoRenewRetryFields(row)
	if payload.AutoRenew {
		row.LastError = ""
	}
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
	if row.PushEnabled {
		if err := removeVerifiedCertificateFiles(row.PushDir, row.PushFilePaths); err != nil {
			return nil, err
		}
	}
	if row.SourceType == CertificateSourceImported {
		if err := clearLegacySettingsPathCertificateSource(&SettingService{}, row.SourceRef); err != nil {
			return nil, err
		}
	}
	bindings, err := detachAndDeleteCertificateRecord(row)
	if err != nil {
		return nil, err
	}
	if row.SourceType == CertificateSourceACME {
		if err := syncManagedAcmeCertificateOwnership(); err != nil {
			return nil, fmt.Errorf("clear deleted certificate ownership: %w", err)
		}
	}
	syncDetachedCertificateBindings(bindings)
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{
		Overview: overview,
		Msg:      "证书已删除，关联应用绑定已解除",
	}, nil
}

type detachedCertificateBindings struct {
	panelTargets   []PanelSelfSignedTarget
	defaultTLSIDs  []uint
	mihomoTLSIDs   []uint
	reverseChanged bool
}

// detachAndDeleteCertificateRecord removes every database-level reference to
// one certificate before deleting its material row. ACME/DNS account rows are
// intentionally not touched: they are independent reusable credentials.
func detachAndDeleteCertificateRecord(row *model.CertificateRecord) (detachedCertificateBindings, error) {
	if row == nil || row.Id == 0 {
		return detachedCertificateBindings{}, common.NewError("certificate record is required")
	}
	return detachAndDeleteCertificateRecords([]model.CertificateRecord{*row}, nil)
}

func detachAndDeleteCertificateRecords(rows []model.CertificateRecord, finalize func(*gorm.DB) error) (detachedCertificateBindings, error) {
	result := detachedCertificateBindings{}
	certificateIDs := make([]uint, 0, len(rows))
	certificateIDSet := make(map[uint]struct{}, len(rows))
	for i := range rows {
		id := rows[i].Id
		if id == 0 {
			continue
		}
		if _, exists := certificateIDSet[id]; exists {
			continue
		}
		certificateIDSet[id] = struct{}{}
		certificateIDs = append(certificateIDs, id)
	}
	if len(certificateIDs) == 0 && finalize == nil {
		return result, nil
	}

	settingService := &SettingService{}
	previousAssignments := map[PanelSelfSignedTarget][]uint{}
	for _, target := range []PanelSelfSignedTarget{PanelSelfSignedTargetPanel, PanelSelfSignedTargetSub} {
		assigned, err := GetAssignedCertificateRecordIDs(settingService, target)
		if err != nil {
			return result, err
		}
		hasRemovedAssignment := false
		next := make([]uint, 0, len(assigned))
		for _, id := range assigned {
			if _, removed := certificateIDSet[id]; removed {
				hasRemovedAssignment = true
				continue
			}
			next = append(next, id)
		}
		if !hasRemovedAssignment {
			continue
		}
		previousAssignments[target] = append([]uint(nil), assigned...)
		if err := SetAssignedCertificateRecordIDs(settingService, target, next); err != nil {
			return result, err
		}
		result.panelTargets = append(result.panelTargets, target)
	}

	db := database.GetDB()
	err := db.Transaction(func(tx *gorm.DB) error {
		if len(certificateIDs) > 0 {
			if err := tx.Model(&model.Tls{}).
				Where("certificate_record_id IN ?", certificateIDs).
				Pluck("id", &result.defaultTLSIDs).Error; err != nil {
				return err
			}
			if len(result.defaultTLSIDs) > 0 {
				if err := tx.Model(&model.Tls{}).Where("certificate_record_id IN ?", certificateIDs).Update("certificate_record_id", 0).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&model.MihomoTls{}).
				Where("certificate_record_id IN ?", certificateIDs).
				Pluck("id", &result.mihomoTLSIDs).Error; err != nil {
				return err
			}
			if len(result.mihomoTLSIDs) > 0 {
				if err := tx.Model(&model.MihomoTls{}).Where("certificate_record_id IN ?", certificateIDs).Update("certificate_record_id", 0).Error; err != nil {
					return err
				}
			}
		}

		reverseRows := make([]model.ReverseProxyRule, 0)
		if err := tx.Model(&model.ReverseProxyRule{}).
			Select("id", "enabled", "listen_protocol", "listen_protocol_alias", "certificate_record_id", "certificate_record_list").
			Find(&reverseRows).Error; err != nil {
			return err
		}
		for i := range reverseRows {
			current := reverseProxyRuleCertificateIDs(&reverseRows[i])
			next := make([]uint, 0, len(current))
			for _, id := range current {
				if _, removed := certificateIDSet[id]; !removed {
					next = append(next, id)
				}
			}
			if len(next) == len(current) {
				continue
			}
			primaryID := uint(0)
			if len(next) > 0 {
				primaryID = next[0]
			}
			updates := map[string]interface{}{
				"certificate_record_id":   primaryID,
				"certificate_record_list": encodeReverseProxyUintList(next),
			}
			listenAlias := normalizeReverseProxyProtocolAlias(reverseRows[i].ListenProtocolAlias, reverseRows[i].ListenProtocol)
			if len(next) == 0 && reverseProxyListenerUsesManagedCertificates(reverseRows[i].ListenProtocol, listenAlias) {
				updates["enabled"] = false
				updates["runtime_status"] = "disabled"
				updates["last_error"] = ""
			}
			if err := tx.Model(&model.ReverseProxyRule{}).Where("id = ?", reverseRows[i].Id).Updates(updates).Error; err != nil {
				return err
			}
			result.reverseChanged = true
		}
		if result.reverseChanged {
			settings, settingsErr := loadReverseProxySettingsTx(tx)
			if settingsErr != nil {
				return settingsErr
			}
			if bumpErr := reverseProxyBumpRevisionTx(tx, settings); bumpErr != nil {
				return bumpErr
			}
		}

		if len(certificateIDs) > 0 {
			if err := tx.Where("certificate_record_id IN ?", certificateIDs).Delete(&model.PanelCertificateBalanceState{}).Error; err != nil {
				return err
			}
			if err := tx.Where("certificate_record_id IN ?", certificateIDs).Delete(&model.ReverseProxyCertificateBalanceState{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", certificateIDs).Delete(&model.CertificateRecord{}).Error; err != nil {
				return err
			}
		}
		if finalize != nil {
			return finalize(tx)
		}
		return nil
	})
	if err == nil {
		return result, nil
	}

	// Settings were changed before the transaction so their canonical filtering
	// could still see the record. Restore them if the database deletion failed.
	for target, ids := range previousAssignments {
		if restoreErr := SetAssignedCertificateRecordIDs(settingService, target, ids); restoreErr != nil {
			logger.Warning("restore certificate assignment after delete rollback failed: ", restoreErr)
		}
	}
	return detachedCertificateBindings{}, err
}

// Runtime refreshes happen only after the short DB transaction commits. A
// failed external refresh is logged: the binding is already safely removed and
// the normal runtime reconciler will retry from the now-authoritative database.
func syncDetachedCertificateBindings(bindings detachedCertificateBindings) {
	for _, target := range bindings.panelTargets {
		if err := ApplyPanelTLSRuntimeSettings(target); err != nil {
			logger.Warning("refresh panel TLS after certificate deletion failed: ", err)
		}
	}
	if _, err := ForceSyncTLSPathBindingsForTLSIDs(bindings.defaultTLSIDs, bindings.mihomoTLSIDs, ""); err != nil {
		logger.Warning("refresh TLS bindings after certificate deletion failed: ", err)
	}
	noteReverseProxyCertificateInventoryChanged()
	if err := (&ReverseProxyService{}).SyncCertificateInventoryNow(); err != nil {
		logger.Warning("refresh reverse proxy after certificate deletion failed: ", err)
	}
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
		ApplyTarget:        strings.TrimSpace(row.ApplyTarget),
	}
	return (&SelfSignedService{}).Issue(payload)
}

func (s *AcmeService) SaveAcmeAccount(payload AcmeAccountPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		return nil, common.NewError("acme 账号名称不能为空")
	}

	entry := &model.AcmeAccount{}
	db := database.GetDB()
	isUpdate := payload.ID > 0
	if payload.ID > 0 {
		if err := db.Where("id = ?", payload.ID).First(entry).Error; err != nil {
			return nil, err
		}
		if entry.System {
			return nil, common.NewError("系统 ACME 运行态不能在账号管理中修改")
		}
	}

	serverInput := strings.TrimSpace(payload.Server)
	serverProvided := payload.ServerProvided || serverInput != ""
	server := ""
	if isUpdate && !serverProvided {
		server = normalizeSupportedAcmeDomainServer(entry.Server)
	} else {
		server = normalizeSupportedAcmeDomainServer(serverInput)
		if serverInput != "" && server == "" {
			return nil, common.NewError("acme 账号 CA 平台仅支持 letsencrypt 或 zerossl")
		}
		if server == "" {
			server = normalizeSupportedAcmeDomainServer(s.readSettingWithDefault(acmePreferredCAKey, defaultAcmePreferredCA))
			if server == "" {
				server = defaultAcmePreferredCA
			}
		}
	}
	if server == "" {
		return nil, common.NewError("acme 账号 CA 平台仅支持 letsencrypt 或 zerossl")
	}

	emailProvided := payload.EmailProvided || strings.TrimSpace(payload.Email) != ""
	emailInput := payload.Email
	if isUpdate && !emailProvided {
		emailInput = entry.Email
	}
	email, emailErr := validateAcmeAccountEmailForServer(emailInput, server)
	if emailErr != nil {
		return nil, emailErr
	}

	accountKeyProvided := payload.AccountKeyLengthProvided || strings.TrimSpace(payload.AccountKeyLength) != "" || strings.TrimSpace(payload.KeyLength) != ""
	accountKeyLength := ""
	accountKeyUnchanged := isUpdate && !accountKeyProvided
	if !accountKeyUnchanged {
		accountKeyLength = normalizeAcmeKeyLength(payload.AccountKeyLength)
		if accountKeyLength == "" {
			accountKeyLength = normalizeAcmeKeyLength(payload.KeyLength)
		}
		if accountKeyLength == "" {
			accountKeyLength = defaultAcmeKeyLength
		}
	}

	remark := strings.TrimSpace(payload.Remark)
	if isUpdate && !payload.RemarkProvided && remark == "" {
		remark = entry.Remark
	}
	if entry.Registered && normalizeSupportedAcmeDomainServer(entry.Server) != server {
		return nil, common.NewError("已注册 ACME 账号不能变更 CA 平台；请新建账号")
	}
	if entry.Registered && accountKeyProvided && accountKeyLength != effectiveAcmeAccountKeyLength(entry) {
		return nil, common.NewError("已注册 ACME 账号的密钥算法请使用“轮换账号密钥”操作")
	}
	entry.Name = name
	entry.Email = email
	entry.Server = server
	if !accountKeyUnchanged {
		entry.KeyLength = accountKeyLength
		entry.AccountKeyLength = accountKeyLength
	}
	entry.Remark = remark
	if err := ensureAcmeAccountDisplayID(db, entry); err != nil {
		return nil, err
	}

	if err := db.Save(entry).Error; err != nil {
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
	db := database.GetDB()
	if err := db.Transaction(func(tx *gorm.DB) error {
		entry := &model.AcmeAccount{}
		if err := tx.Where("id = ?", id).First(entry).Error; err != nil {
			return err
		}
		if entry.System {
			return common.NewError("系统 ACME 运行态不能删除")
		}
		if err := tx.Model(&model.CertificateRecord{}).
			Where("source_type = ? AND acme_account_id = ?", CertificateSourceACME, id).
			Updates(map[string]interface{}{
				"acme_account_id":            0,
				"acme_account_name":          "",
				"auto_renew":                 false,
				"auto_renew_retry_phase":     "",
				"auto_renew_retry_count":     0,
				"auto_renew_next_retry_at":   0,
				"auto_renew_last_attempt_at": 0,
			}).Error; err != nil {
			return err
		}
		return tx.Delete(entry).Error
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

func (s *AcmeService) RotateAcmeAccountKey(payload AcmeAccountRotateKeyPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if payload.ID == 0 {
		return nil, common.NewError("acme 账号 id 不能为空")
	}
	keyLength := normalizeAcmeKeyLength(payload.AccountKeyLength)
	if keyLength == "" {
		return nil, common.NewError("不支持的 ACME 账号密钥算法")
	}
	account := &model.AcmeAccount{}
	if err := database.GetDB().Where("id = ?", payload.ID).First(account).Error; err != nil {
		return nil, err
	}
	if account.System || !account.Registered || len(account.RuntimeState) == 0 {
		return nil, common.NewError("该 ACME 账号尚未注册，首次签发时会按所选密钥算法注册")
	}
	scriptPath, homeDir, installed := s.resolveAcmeScript()
	if !installed {
		return nil, common.NewError("acme.sh is not installed")
	}
	runtime, err := newAcmeOperationRuntime(account)
	if err != nil {
		return nil, err
	}
	runtimeSnapshotSaved := false
	defer func() {
		if !runtimeSnapshotSaved {
			if snapshotErr := runtime.snapshot(); snapshotErr != nil {
				logger.Warning("保存账号密钥轮换运行态失败: ", snapshotErr)
			}
		}
		runtime.cleanup()
	}()
	args := append(runtime.commandArgs(homeDir), "--update-account-key", "--accountkeylength", keyLength, "--server", strings.TrimSpace(account.Server))
	if _, err := runCommandOutputWithTimeoutEnvLog(90*time.Second, scriptPath, args, nil, nil); err != nil {
		return nil, common.NewError("轮换 ACME 账号密钥失败: ", err)
	}
	account.AccountKeyLength = keyLength
	account.KeyLength = keyLength
	if err := runtime.snapshot(); err != nil {
		return nil, err
	}
	runtimeSnapshotSaved = true
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}
	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{Overview: overview, Msg: "ACME 账号密钥已轮换"}, nil
}

func (s *AcmeService) SaveDNSAccount(payload AcmeDNSAccountPayload) (*AcmeActionResult, error) {
	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	name := strings.TrimSpace(payload.Name)
	providerCode := strings.TrimSpace(payload.ProviderCode)
	if name == "" {
		return nil, common.NewError("dns 账号名称不能为空")
	}
	providerMeta, ok := lookupAcmeDNSProvider(providerCode)
	if !ok {
		return nil, common.NewError("不支持的 dns 提供商: ", providerCode)
	}
	entry := &model.AcmeDNSAccount{}
	db := database.GetDB()
	existingEnvMap := map[string]string{}
	isUpdate := payload.ID > 0
	sameProvider := false
	if payload.ID > 0 {
		if err := db.Where("id = ?", payload.ID).First(entry).Error; err != nil {
			return nil, err
		}
		if !strings.EqualFold(strings.TrimSpace(entry.ProviderCode), providerMeta.ProviderCode) {
			referenced, referenceErr := isDNSAccountReferencedByCertificateRecord(db, entry.Id)
			if referenceErr != nil {
				return nil, referenceErr
			}
			if referenced {
				return nil, common.NewError("已绑定 ACME 证书的 DNS 账号不能更改供应商；可在同一供应商下轮换凭据，或新建账号后重新签发证书")
			}
		}
		sameProvider = strings.EqualFold(strings.TrimSpace(entry.ProviderCode), providerMeta.ProviderCode)
		if sameProvider {
			existingEnvMap, _ = parseAcmeEnvJSON(entry.EnvJSON)
		}
	}

	envProvided := payload.EnvJSONProvided || strings.TrimSpace(payload.EnvJSON) != ""
	envRaw := payload.EnvJSON
	if isUpdate && !envProvided && sameProvider {
		envRaw = entry.EnvJSON
	} else if !envProvided {
		envRaw = "{}"
	}
	envMap, err := parseAcmeEnvJSON(envRaw)
	if err != nil {
		return nil, err
	}
	envMap = sanitizeDNSAccountEnvForProvider(providerMeta, envMap)
	envMap = mergeAcmeDNSAccountEnv(existingEnvMap, envMap)
	if err := validateDNSProviderEnv(providerMeta, envMap); err != nil {
		return nil, err
	}

	envJSON, err := json.Marshal(envMap)
	if err != nil {
		return nil, err
	}
	remark := strings.TrimSpace(payload.Remark)
	if isUpdate && !payload.RemarkProvided && remark == "" {
		remark = entry.Remark
	}

	entry.Name = name
	entry.ProviderName = providerMeta.Name
	entry.ProviderCode = providerMeta.ProviderCode
	entry.EnvJSON = string(envJSON)
	entry.Remark = remark
	if err := ensureDNSAccountDisplayID(db, entry); err != nil {
		return nil, err
	}

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
		entry := &model.AcmeDNSAccount{}
		if err := tx.Where("id = ?", id).First(entry).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.CertificateRecord{}).
			Where("source_type = ? AND dns_account_id = ?", CertificateSourceACME, id).
			Updates(map[string]interface{}{
				"dns_account_id":             0,
				"dns_account_name":           "",
				"auto_renew":                 false,
				"auto_renew_retry_phase":     "",
				"auto_renew_retry_count":     0,
				"auto_renew_next_retry_at":   0,
				"auto_renew_last_attempt_at": 0,
			}).Error; err != nil {
			return err
		}
		return tx.Delete(entry).Error
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

	now := time.Now().Unix()
	renewedCount := 0
	failedMessages := make([]string, 0)
	rows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().
		Where("source_type IN ? AND auto_renew = ?", []string{CertificateSourceACME, CertificateSourceSelfSigned}, true).
		Order("not_after ASC, id ASC").
		Find(&rows).Error; err != nil {
		return 0, err
	}

	processable := make([]model.CertificateRecord, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if row.NotAfter > 0 && row.NotAfter <= now {
			if err := disableExpiredCertificateAutoRenew(row.Id, now); err != nil {
				failedMessages = append(failedMessages, fmt.Sprintf("%s: disable expired auto-renew failed: %v", row.MainDomain, err))
			}
			continue
		}
		if row.SourceType == CertificateSourceACME && !IsSystemPlatformLinux() {
			continue
		}
		processable = append(processable, row)
	}

	candidateIDs := collectAutoRenewBatchCandidates(processable, now)
	for _, id := range candidateIDs {
		if autoRenewBatchCandidateCompleted(id) {
			continue
		}
		freshRow, freshErr := certificateInventory.GetRecordByID(id)
		if freshErr != nil {
			if database.IsNotFound(freshErr) {
				removeAutoRenewBatchCandidate(id)
				continue
			}
			failedMessages = append(failedMessages, fmt.Sprintf("certificate %d: refresh failed: %v", id, freshErr))
			continue
		}
		if !freshRow.AutoRenew {
			removeAutoRenewBatchCandidate(id)
			continue
		}
		if autoRenewBatchCandidateAlreadyRenewed(freshRow) {
			markAutoRenewBatchCandidateCompleted(id)
			continue
		}
		attemptAt := time.Now().Unix()
		if freshRow.NotAfter > 0 && freshRow.NotAfter <= attemptAt {
			if err := disableExpiredCertificateAutoRenew(freshRow.Id, attemptAt); err != nil {
				failedMessages = append(failedMessages, fmt.Sprintf("%s: disable expired auto-renew failed: %v", freshRow.MainDomain, err))
			}
			removeAutoRenewBatchCandidate(id)
			continue
		}
		if !certificateAutoRenewAttemptDue(freshRow, attemptAt) {
			continue
		}
		if err := markCertificateAutoRenewAttempt(freshRow.Id, attemptAt); err != nil {
			failedMessages = append(failedMessages, fmt.Sprintf("%s: record automatic attempt failed: %v", freshRow.MainDomain, err))
			continue
		}

		payload := AcmeRenewPayload{ID: freshRow.Id}
		if freshRow.SourceType == CertificateSourceACME {
			// Automatic renewal has already selected this record from its
			// inventory-derived window, so a fresh issuance is required.
			payload.Force = true
		}
		_, err := s.Renew(payload)
		if err != nil {
			if stateErr := recordCertificateAutoRenewFailure(freshRow.Id, attemptAt, err); stateErr != nil {
				failedMessages = append(failedMessages, fmt.Sprintf("%s: %v (save retry state failed: %v)", freshRow.MainDomain, err, stateErr))
			} else {
				failedMessages = append(failedMessages, fmt.Sprintf("%s: %v", freshRow.MainDomain, err))
			}
			continue
		}
		markAutoRenewBatchCandidateCompleted(freshRow.Id)
		renewedCount++
	}

	finishAutoRenewBatchIfReady(time.Now().Unix())

	if len(failedMessages) > 0 {
		return renewedCount, common.NewError(strings.Join(failedMessages, "; "))
	}

	return renewedCount, nil
}

func collectAutoRenewBatchCandidates(rows []model.CertificateRecord, nowUnix int64) []uint {
	acmeAutoRenewBatch.mu.Lock()
	defer acmeAutoRenewBatch.mu.Unlock()
	loadAutoRenewBatchWindowLocked()

	active := acmeAutoRenewBatch.startedAt > 0 && acmeAutoRenewBatch.endsAt > acmeAutoRenewBatch.startedAt
	if !active {
		triggered := false
		for i := range rows {
			if certificateStartsAutoRenewBatch(&rows[i], nowUnix) {
				triggered = true
				break
			}
		}
		if !triggered {
			return nil
		}
		acmeAutoRenewBatch.startedAt = nowUnix
		acmeAutoRenewBatch.endsAt = nowUnix + int64(acmeAutoRenewBatchDuration/time.Second)
		persistAutoRenewBatchWindowLocked()
		logger.Infof("certificate auto-renew batch opened: start=%d end=%d", acmeAutoRenewBatch.startedAt, acmeAutoRenewBatch.endsAt)
		acmeAutoRenewBatch.candidateIDs = make(map[uint]struct{})
		acmeAutoRenewBatch.completedIDs = make(map[uint]struct{})
	}
	if acmeAutoRenewBatch.candidateIDs == nil {
		acmeAutoRenewBatch.candidateIDs = make(map[uint]struct{})
	}
	if acmeAutoRenewBatch.completedIDs == nil {
		acmeAutoRenewBatch.completedIDs = make(map[uint]struct{})
	}

	for i := range rows {
		phase := strings.TrimSpace(rows[i].AutoRenewRetryPhase)
		if phase == acmeAutoRenewRetryPhaseRapid && rows[i].AutoRenewLastAttemptAt >= acmeAutoRenewBatch.startedAt {
			acmeAutoRenewBatch.candidateIDs[rows[i].Id] = struct{}{}
			continue
		}
		if nowUnix <= acmeAutoRenewBatch.endsAt && certificateJoinsAutoRenewBatch(&rows[i], acmeAutoRenewBatch.endsAt) {
			acmeAutoRenewBatch.candidateIDs[rows[i].Id] = struct{}{}
		}
	}

	ids := make([]uint, 0, len(acmeAutoRenewBatch.candidateIDs))
	for id := range acmeAutoRenewBatch.candidateIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func certificateStartsAutoRenewBatch(entry *model.CertificateRecord, nowUnix int64) bool {
	if entry == nil || !entry.AutoRenew || entry.Id == 0 {
		return false
	}
	phase := strings.TrimSpace(entry.AutoRenewRetryPhase)
	if phase == acmeAutoRenewRetryPhaseRapid || phase == acmeAutoRenewRetryPhasePeriodic {
		return entry.AutoRenewNextRetryAt > 0 && entry.AutoRenewNextRetryAt <= nowUnix
	}
	return certificateNormalAutoRenewAt(entry) <= nowUnix
}

func certificateJoinsAutoRenewBatch(entry *model.CertificateRecord, batchEndsAt int64) bool {
	if entry == nil || !entry.AutoRenew || entry.Id == 0 || batchEndsAt <= 0 {
		return false
	}
	phase := strings.TrimSpace(entry.AutoRenewRetryPhase)
	if phase == acmeAutoRenewRetryPhaseRapid || phase == acmeAutoRenewRetryPhasePeriodic {
		return entry.AutoRenewNextRetryAt > 0 && entry.AutoRenewNextRetryAt <= batchEndsAt
	}
	return certificateNormalAutoRenewAt(entry) <= batchEndsAt
}

func certificateNormalAutoRenewAt(entry *model.CertificateRecord) int64 {
	if entry == nil || entry.NotAfter <= 0 {
		return int64(^uint64(0) >> 1)
	}
	windowSeconds := computeAutoRenewWindowSecondsForRecord(entry)
	if strings.TrimSpace(entry.SourceType) == CertificateSourceSelfSigned {
		windowSeconds = int64(defaultSelfSignedDurationValue/3) * 24 * 3600
		if windowSeconds <= 0 {
			windowSeconds = 30 * 24 * 3600
		}
	}
	return entry.NotAfter - windowSeconds
}

func certificateAutoRenewAttemptDue(entry *model.CertificateRecord, nowUnix int64) bool {
	if entry == nil || !entry.AutoRenew {
		return false
	}
	phase := strings.TrimSpace(entry.AutoRenewRetryPhase)
	if phase == acmeAutoRenewRetryPhaseRapid || phase == acmeAutoRenewRetryPhasePeriodic {
		return entry.AutoRenewNextRetryAt > 0 && entry.AutoRenewNextRetryAt <= nowUnix
	}
	return true
}

func markCertificateAutoRenewAttempt(id uint, nowUnix int64) error {
	if id == 0 {
		return nil
	}
	return database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", id).Update("auto_renew_last_attempt_at", nowUnix).Error
}

func recordCertificateAutoRenewFailure(id uint, nowUnix int64, renewErr error) error {
	row, err := certificateInventory.GetRecordByID(id)
	if err != nil {
		if database.IsNotFound(err) {
			removeAutoRenewBatchCandidate(id)
			return nil
		}
		return err
	}
	if !row.AutoRenew {
		removeAutoRenewBatchCandidate(id)
		return nil
	}
	if row.NotAfter > 0 && row.NotAfter <= nowUnix {
		removeAutoRenewBatchCandidate(id)
		return disableExpiredCertificateAutoRenew(id, nowUnix)
	}

	phase, retryCount, nextRetryAt := nextCertificateAutoRenewFailureState(row.AutoRenewRetryPhase, row.AutoRenewRetryCount, nowUnix)
	return database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"auto_renew_retry_phase":     phase,
		"auto_renew_retry_count":     retryCount,
		"auto_renew_next_retry_at":   nextRetryAt,
		"auto_renew_last_attempt_at": nowUnix,
		"last_error":                 strings.TrimSpace(renewErr.Error()),
	}).Error
}

func nextCertificateAutoRenewFailureState(currentPhase string, completedRapidRetries int, nowUnix int64) (string, int, int64) {
	currentPhase = strings.TrimSpace(currentPhase)
	switch currentPhase {
	case acmeAutoRenewRetryPhaseRapid:
		completedRapidRetries++
		if completedRapidRetries >= acmeAutoRenewRapidRetryLimit {
			return acmeAutoRenewRetryPhasePeriodic, acmeAutoRenewRapidRetryLimit, nowUnix + int64(acmeAutoRenewPeriodicRetryInterval/time.Second)
		}
		return acmeAutoRenewRetryPhaseRapid, completedRapidRetries, nowUnix + int64(acmeAutoRenewRapidRetryInterval/time.Second)
	case acmeAutoRenewRetryPhasePeriodic:
		return acmeAutoRenewRetryPhasePeriodic, acmeAutoRenewRapidRetryLimit, nowUnix + int64(acmeAutoRenewPeriodicRetryInterval/time.Second)
	default:
		return acmeAutoRenewRetryPhaseRapid, 0, nowUnix + int64(acmeAutoRenewRapidRetryInterval/time.Second)
	}
}

func disableExpiredCertificateAutoRenew(id uint, nowUnix int64) error {
	if id == 0 {
		return nil
	}
	return database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"auto_renew":                 false,
		"auto_renew_retry_phase":     acmeAutoRenewRetryPhaseExpiredDisabled,
		"auto_renew_retry_count":     0,
		"auto_renew_next_retry_at":   0,
		"auto_renew_last_attempt_at": nowUnix,
		"last_error":                 "证书已到期，自动续签已停用",
	}).Error
}

func clearCertificateAutoRenewRetryFields(row *model.CertificateRecord) {
	if row == nil {
		return
	}
	row.AutoRenewRetryPhase = ""
	row.AutoRenewRetryCount = 0
	row.AutoRenewNextRetryAt = 0
	row.AutoRenewLastAttemptAt = 0
}

func autoRenewBatchCandidateCompleted(id uint) bool {
	acmeAutoRenewBatch.mu.Lock()
	defer acmeAutoRenewBatch.mu.Unlock()
	_, completed := acmeAutoRenewBatch.completedIDs[id]
	return completed
}

func autoRenewBatchCandidateAlreadyRenewed(row *model.CertificateRecord) bool {
	if row == nil || strings.TrimSpace(row.AutoRenewRetryPhase) != "" {
		return false
	}
	acmeAutoRenewBatch.mu.Lock()
	defer acmeAutoRenewBatch.mu.Unlock()
	return acmeAutoRenewBatch.startedAt > 0 && row.LastRenewedAt >= acmeAutoRenewBatch.startedAt
}

func markAutoRenewBatchCandidateCompleted(id uint) {
	acmeAutoRenewBatch.mu.Lock()
	defer acmeAutoRenewBatch.mu.Unlock()
	if acmeAutoRenewBatch.completedIDs == nil {
		acmeAutoRenewBatch.completedIDs = make(map[uint]struct{})
	}
	acmeAutoRenewBatch.completedIDs[id] = struct{}{}
}

func removeAutoRenewBatchCandidate(id uint) {
	acmeAutoRenewBatch.mu.Lock()
	defer acmeAutoRenewBatch.mu.Unlock()
	delete(acmeAutoRenewBatch.candidateIDs, id)
	delete(acmeAutoRenewBatch.completedIDs, id)
}

func finishAutoRenewBatchIfReady(nowUnix int64) {
	acmeAutoRenewBatch.mu.Lock()
	if acmeAutoRenewBatch.startedAt == 0 || nowUnix < acmeAutoRenewBatch.endsAt {
		acmeAutoRenewBatch.mu.Unlock()
		return
	}
	ids := make([]uint, 0, len(acmeAutoRenewBatch.candidateIDs))
	for id := range acmeAutoRenewBatch.candidateIDs {
		ids = append(ids, id)
	}
	acmeAutoRenewBatch.mu.Unlock()

	rapidCount := int64(0)
	if len(ids) > 0 {
		if err := database.GetDB().Model(&model.CertificateRecord{}).
			Where("id IN ? AND auto_renew = ? AND auto_renew_retry_phase = ?", ids, true, acmeAutoRenewRetryPhaseRapid).
			Count(&rapidCount).Error; err != nil {
			logger.Warning("check certificate rapid retry state failed: ", err)
			return
		}
	}
	if rapidCount > 0 {
		return
	}

	acmeAutoRenewBatch.mu.Lock()
	defer acmeAutoRenewBatch.mu.Unlock()
	if acmeAutoRenewBatch.startedAt > 0 && nowUnix >= acmeAutoRenewBatch.endsAt {
		logger.Infof("certificate auto-renew batch closed: start=%d end=%d", acmeAutoRenewBatch.startedAt, acmeAutoRenewBatch.endsAt)
		acmeAutoRenewBatch.startedAt = 0
		acmeAutoRenewBatch.endsAt = 0
		acmeAutoRenewBatch.candidateIDs = nil
		acmeAutoRenewBatch.completedIDs = nil
		persistAutoRenewBatchWindowLocked()
	}
}

func currentCertificateAutoRenewBatchWindow() (int64, int64, bool) {
	acmeAutoRenewBatch.mu.Lock()
	defer acmeAutoRenewBatch.mu.Unlock()
	loadAutoRenewBatchWindowLocked()
	active := acmeAutoRenewBatch.startedAt > 0 && acmeAutoRenewBatch.endsAt > acmeAutoRenewBatch.startedAt
	return acmeAutoRenewBatch.startedAt, acmeAutoRenewBatch.endsAt, active
}

func loadAutoRenewBatchWindowLocked() {
	if acmeAutoRenewBatch.loaded {
		return
	}
	acmeAutoRenewBatch.loaded = true
	if database.GetDB() == nil {
		return
	}
	setting, err := (&SettingService{}).getSetting(certificateAutoRenewBatchStateSettingKey)
	if database.IsNotFound(err) {
		return
	}
	if err != nil {
		logger.Warning("load certificate auto-renew batch state failed: ", err)
		return
	}
	persisted := acmeAutoRenewBatchPersistentState{}
	if err := json.Unmarshal([]byte(setting.Value), &persisted); err != nil {
		logger.Warning("parse certificate auto-renew batch state failed: ", err)
		return
	}
	if persisted.StartedAt > 0 && persisted.EndsAt > persisted.StartedAt {
		acmeAutoRenewBatch.startedAt = persisted.StartedAt
		acmeAutoRenewBatch.endsAt = persisted.EndsAt
		logger.Infof("certificate auto-renew batch restored: start=%d end=%d", persisted.StartedAt, persisted.EndsAt)
	}
}

func persistAutoRenewBatchWindowLocked() {
	if database.GetDB() == nil {
		return
	}
	raw, err := json.Marshal(acmeAutoRenewBatchPersistentState{
		StartedAt: acmeAutoRenewBatch.startedAt,
		EndsAt:    acmeAutoRenewBatch.endsAt,
	})
	if err != nil {
		logger.Warning("marshal certificate auto-renew batch state failed: ", err)
		return
	}
	if err := (&SettingService{}).saveSetting(certificateAutoRenewBatchStateSettingKey, string(raw)); err != nil {
		logger.Warning("persist certificate auto-renew batch state failed: ", err)
	}
}

func isCertificateAutoRenewBatchOpen() bool {
	_, _, active := currentCertificateAutoRenewBatchWindow()
	return active
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

func shouldAutoRenewCertificateRecord(entry *model.CertificateRecord, nowUnix int64, windowSeconds int64) bool {
	if entry == nil || !entry.AutoRenew || entry.NotAfter <= 0 {
		return false
	}
	if windowSeconds <= 0 {
		windowSeconds = 30 * 24 * 3600
	}
	return entry.NotAfter <= nowUnix+windowSeconds
}

func computeAutoRenewWindowSecondsForRecord(entry *model.CertificateRecord) int64 {
	defaultWindow := int64(defaultAcmeAutoRenewDays) * 24 * 3600
	if entry == nil || entry.NotBefore <= 0 || entry.NotAfter <= entry.NotBefore {
		return defaultWindow
	}
	validityDays := float64(entry.NotAfter-entry.NotBefore) / 86400.0
	if validityDays > 40 {
		return defaultWindow
	}
	windowDays := int64(math.Floor(validityDays / 3.0))
	if windowDays < 1 {
		windowDays = 1
	}
	return windowDays * 24 * 3600
}

func (s *AcmeService) listCertificates() ([]AcmeCertificateView, error) {
	return certificateInventory.List()
}

func (s *AcmeService) cleanupNonDNSCertificateDNSReferences() error {
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
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

// scrubLegacyAcmeCertificateRuntimeFields removes data that was meaningful
// only to the retired shared acme.sh runtime. In particular, DNSEnvText could
// contain provider credentials and must not remain in database backups after
// DNS accounts became the sole credential store.
func (s *AcmeService) scrubLegacyAcmeCertificateRuntimeFields() error {
	db := database.GetDB()
	rows := make([]model.CertificateRecord, 0)
	if err := db.Select(
		"id",
		"acme_home",
		"dns_env_text",
		"renew_config",
		"cert_path",
		"key_path",
		"fullchain_path",
		"chain_path",
	).Where("source_type = ?", CertificateSourceACME).Find(&rows).Error; err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for i := range rows {
			entry := &rows[i]
			hasLegacyRuntimeData := strings.TrimSpace(entry.AcmeHome) != "" ||
				strings.TrimSpace(entry.DNSEnvText) != "" ||
				strings.TrimSpace(entry.RenewConfig) != "" ||
				strings.TrimSpace(entry.CertPath) != "" ||
				strings.TrimSpace(entry.KeyPath) != "" ||
				strings.TrimSpace(entry.FullchainPath) != "" ||
				strings.TrimSpace(entry.ChainPath) != ""
			if !hasLegacyRuntimeData {
				continue
			}

			if err := tx.Model(&model.CertificateRecord{}).Where("id = ?", entry.Id).Updates(map[string]interface{}{
				"acme_home":      "",
				"dns_env_text":   "",
				"renew_config":   "",
				"cert_path":      "",
				"key_path":       "",
				"fullchain_path": "",
				"chain_path":     "",
				// Old command logs were produced while credentials lived in
				// account.conf or DNSEnvText, so they are not safe to retain.
				"last_output": "",
				"last_error":  "",
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *AcmeService) listAcmeAccounts() ([]AcmeAccountView, error) {
	rows := make([]model.AcmeAccount, 0)
	if err := database.GetDB().Where("system = ?", false).Order("display_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]AcmeAccountView, 0, len(rows))
	for i := range rows {
		entry := rows[i]
		result = append(result, AcmeAccountView{
			Id:               entry.Id,
			DisplayID:        entry.DisplayID,
			ResourceID:       acmeAccountResourceID(entry.DisplayID),
			Name:             entry.Name,
			Email:            entry.Email,
			Server:           entry.Server,
			AccountKeyLength: effectiveAcmeAccountKeyLength(&entry),
			Registered:       entry.Registered,
			Remark:           entry.Remark,
			CreatedAt:        entry.CreatedAt.Unix(),
			UpdatedAt:        entry.UpdatedAt.Unix(),
		})
	}
	return result, nil
}

func (s *AcmeService) listDNSAccounts() ([]AcmeDNSAccountView, error) {
	rows := make([]model.AcmeDNSAccount, 0)
	db := database.GetDB()
	if err := db.Order("display_id ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].Id)
	}
	referencedIDs, err := referencedDNSAccountIDsByCertificateRecord(db, ids)
	if err != nil {
		return nil, err
	}
	result := make([]AcmeDNSAccountView, 0, len(rows))
	for i := range rows {
		entry := rows[i]
		envMap, _ := parseAcmeEnvJSON(entry.EnvJSON)
		envMap = sanitizeAcmeEnvMap(envMap)
		result = append(result, AcmeDNSAccountView{
			Id:           entry.Id,
			DisplayID:    entry.DisplayID,
			ResourceID:   dnsAccountResourceID(entry.DisplayID),
			Name:         entry.Name,
			ProviderName: entry.ProviderName,
			ProviderCode: entry.ProviderCode,
			ProviderLocked: func() bool {
				_, referenced := referencedIDs[entry.Id]
				return referenced
			}(),
			Env:       envMap,
			Remark:    entry.Remark,
			CreatedAt: entry.CreatedAt.Unix(),
			UpdatedAt: entry.UpdatedAt.Unix(),
		})
	}
	return result, nil
}

// referencedDNSAccountIDsByCertificateRecord intentionally reads the unified
// certificate_records table only. acme_certificates is a retired migration
// source and must never influence live account lifecycle decisions.
func referencedDNSAccountIDsByCertificateRecord(db *gorm.DB, ids []uint) (map[uint]struct{}, error) {
	result := make(map[uint]struct{})
	if len(ids) == 0 {
		return result, nil
	}

	inventoryIDs := make([]uint, 0)
	if err := db.Model(&model.CertificateRecord{}).
		Where("source_type = ? AND dns_account_id IN ?", CertificateSourceACME, ids).
		Distinct("dns_account_id").
		Pluck("dns_account_id", &inventoryIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range inventoryIDs {
		if id > 0 {
			result[id] = struct{}{}
		}
	}
	return result, nil
}

func isDNSAccountReferencedByCertificateRecord(db *gorm.DB, id uint) (bool, error) {
	if id == 0 {
		return false, nil
	}
	ids, err := referencedDNSAccountIDsByCertificateRecord(db, []uint{id})
	if err != nil {
		return false, err
	}
	_, referenced := ids[id]
	return referenced, nil
}

type certificateDirectoryPushState struct {
	PushEnabled   bool
	PushDir       string
	PushFilePaths string
}

func syncCertificateDirectoryPushState(targetDir string, currentEnabled bool, currentDir string, currentFilePaths string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) (certificateDirectoryPushState, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return certificateDirectoryPushState{}, common.NewError("target directory is empty")
	}

	if currentEnabled {
		if err := removeVerifiedCertificateFiles(currentDir, currentFilePaths); err != nil {
			return certificateDirectoryPushState{}, err
		}
	}

	writtenPaths, err := writeAndVerifyCertificateDirectoryPush(targetDir, certPEM, keyPEM, fullchainPEM, chainPEM)
	if err != nil {
		return certificateDirectoryPushState{}, err
	}

	return certificateDirectoryPushState{
		PushEnabled:   true,
		PushDir:       filepath.Clean(targetDir),
		PushFilePaths: encodeCertificatePushFilePaths(writtenPaths),
	}, nil
}

func persistCertificatePushState(record *model.CertificateRecord) error {
	if record == nil {
		return common.NewError("certificate record is nil")
	}
	if err := database.GetDB().Save(record).Error; err != nil {
		return err
	}
	if strings.TrimSpace(record.SourceType) == CertificateSourceACME {
		if err := syncManagedAcmeCertificateOwnership(); err != nil {
			return fmt.Errorf("record managed certificate ownership: %w", err)
		}
	}
	return nil
}

func syncManagedAcmeCertificateOwnership() error {
	db := database.GetDB()
	if db == nil {
		return errors.New("database is not ready for managed certificate ownership")
	}
	type certificatePushInventory struct {
		PushEnabled   bool
		PushDir       string
		PushFilePaths string
	}
	rows := make([]certificatePushInventory, 0)
	if err := db.Model(&model.CertificateRecord{}).
		Select("push_enabled", "push_dir", "push_file_paths").
		Where("source_type = ? AND push_enabled = ?", CertificateSourceACME, true).
		Find(&rows).Error; err != nil {
		return err
	}
	paths := []string{managedAcmeHomeDir()}
	for _, row := range rows {
		if !row.PushEnabled {
			continue
		}
		files, err := parseCertificatePushFilePaths(row.PushFilePaths)
		if err != nil {
			return err
		}
		if err := validateCertificatePushFilePaths(row.PushDir, files); err != nil {
			return err
		}
		for _, filePath := range files {
			paths = append(paths, filePath)
		}
	}
	return RegisterAcmeHostOwnership(paths, nil)
}

func beginManagedAcmeCertificatePushOwnership(targetDir string) (HostResource, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return HostResource{}, common.NewError("certificate push target directory is empty")
	}
	paths := make([]string, 0, 4)
	for _, name := range []string{"cert.pem", "key.pem", "fullchain.pem", "chain.pem"} {
		paths = append(paths, filepath.Join(targetDir, name))
	}
	return BeginAcmeHostOwnership(paths, nil)
}

type acmeManagedCertPaths struct {
	CertPath      string
	KeyPath       string
	FullchainPath string
	ChainPath     string
	BaseDir       string
}

func (s *AcmeService) installCertToManagedDirWithArgs(scriptPath string, runtimeArgs []string, mainDomain string, useECC bool, envPairs []string, logSession *acmeLogSession) (*acmeManagedCertPaths, func(), error) {
	paths, cleanup, err := createAcmeTempInstallPaths(mainDomain)
	if err != nil {
		return nil, nil, err
	}
	if err := s.installCertByDomainWithArgs(scriptPath, runtimeArgs, mainDomain, useECC, paths, envPairs, logSession); err != nil {
		cleanup()
		return nil, nil, err
	}
	return paths, cleanup, nil
}

func (s *AcmeService) installCertByDomainWithArgs(scriptPath string, runtimeArgs []string, mainDomain string, useECC bool, paths *acmeManagedCertPaths, envPairs []string, logSession *acmeLogSession) error {
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
	if _, err := runCommandOutputWithTimeoutEnvLog(2*time.Minute, scriptPath, append(append([]string{}, runtimeArgs...), args...), envPairs, logSession); err != nil {
		return common.NewError("install certificate files failed: ", err)
	}
	return nil
}

func (s *AcmeService) markCertificateError(id uint, message string) error {
	message = strings.TrimSpace(message)
	if id == 0 {
		return nil
	}
	return database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", id).Update("last_error", message).Error
}

func (s *AcmeService) disableAutoRenewForMissingAccount(id uint, message string) error {
	if id == 0 {
		return nil
	}
	return database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", id).Updates(map[string]interface{}{
		"auto_renew":                 false,
		"auto_renew_retry_phase":     "",
		"auto_renew_retry_count":     0,
		"auto_renew_next_retry_at":   0,
		"auto_renew_last_attempt_at": 0,
		"last_error":                 strings.TrimSpace(message),
	}).Error
}

// applyCertificateRecordPostActions applies and pushes a certificate using
// the unified inventory record directly. Certificate material has already
// been persisted when this is called, so each failure becomes a recoverable
// warning instead of changing a successful issuance into a failed one.
func (s *AcmeService) applyCertificateRecordPostActions(record *model.CertificateRecord, applyTarget string, pushDir string, pushExplicit bool) []string {
	return s.applyCertificateRecordPostActionsWithCoreRestart(record, applyTarget, pushDir, pushExplicit, true)
}

func (s *AcmeService) applyCertificateRecordPostActionsWithCoreRestart(record *model.CertificateRecord, applyTarget string, pushDir string, pushExplicit bool, queueCoreRestart bool) []string {
	if record == nil {
		return []string{"证书后置动作失败：证书记录不存在"}
	}
	warnings := make([]string, 0)
	appendWarning := func(action string, err error) {
		if err == nil {
			return
		}
		warnings = append(warnings, strings.TrimSpace(action)+"失败: "+strings.TrimSpace(err.Error()))
	}

	if normalizedTarget, ok := normalizeAcmeApplyTarget(applyTarget); ok {
		if err := s.applyInventoryRecordToTarget(record, normalizedTarget); err != nil {
			appendWarning("应用证书", err)
		} else {
			targets, targetErr := assignedTargetsForCertificateRecord(record.Id)
			if targetErr != nil {
				appendWarning("读取应用状态", targetErr)
			} else {
				record.ApplyTarget = formatAssignedApplyTarget(targets)
			}
		}
	}

	pushDir = strings.TrimSpace(pushDir)
	pushTargetDir := ""
	if pushExplicit && pushDir != "" {
		pushTargetDir = pushDir
	} else if record.PushEnabled {
		pushTargetDir = strings.TrimSpace(record.PushDir)
	}
	if pushTargetDir != "" && strings.TrimSpace(record.SourceType) == CertificateSourceACME {
		if _, ownershipErr := beginManagedAcmeCertificatePushOwnership(pushTargetDir); ownershipErr != nil {
			appendWarning("登记证书目录所有权", ownershipErr)
			pushTargetDir = ""
		}
	}
	if pushTargetDir != "" {
		pushState, err := syncCertificateDirectoryPushState(pushTargetDir, record.PushEnabled, record.PushDir, record.PushFilePaths, record.CertPEM, record.KeyPEM, record.FullchainPEM, record.ChainPEM)
		if err != nil {
			appendWarning("推送证书目录", err)
		} else {
			record.PushEnabled = pushState.PushEnabled
			record.PushDir = pushState.PushDir
			record.PushFilePaths = pushState.PushFilePaths
			record.PushFiles = ""
		}
	}
	if err := persistCertificatePushState(record); err != nil {
		appendWarning("保存推送状态", err)
	}
	if err := ApplyPanelTLSRuntimeSettingsForRecord(record.Id); err != nil {
		appendWarning("刷新面板 TLS 运行态", err)
	}
	if _, err := ForceSyncTLSBindingsForCertificateRecordWithCoreRestart(record.Id, "", queueCoreRestart); err != nil {
		appendWarning("刷新 TLS 绑定", err)
	}
	if err := (&ReverseProxyService{}).SyncCertificateInventoryNow(); err != nil {
		appendWarning("刷新反向代理证书运行态", err)
	}
	return normalizeCertificatePostActionWarnings(warnings)
}

func normalizeCertificatePostActionWarnings(warnings []string) []string {
	seen := make(map[string]struct{}, len(warnings))
	result := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" {
			continue
		}
		if _, exists := seen[warning]; exists {
			continue
		}
		seen[warning] = struct{}{}
		result = append(result, warning)
	}
	return result
}

func persistCertificatePostActionWarnings(record *model.CertificateRecord, warnings []string) []string {
	if record == nil || record.Id == 0 {
		return normalizeCertificatePostActionWarnings(append(warnings, "保存后置动作告警失败: 证书记录不存在"))
	}
	warnings = normalizeCertificatePostActionWarnings(warnings)
	message := strings.Join(warnings, "\n")
	if err := database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", record.Id).Update("post_action_error", message).Error; err != nil {
		warnings = append(warnings, "保存后置动作告警失败: "+strings.TrimSpace(err.Error()))
		warnings = normalizeCertificatePostActionWarnings(warnings)
		message = strings.Join(warnings, "\n")
	}
	record.PostActionError = message
	return warnings
}

func clearCertificatePostActionError(record *model.CertificateRecord) error {
	if record == nil || record.Id == 0 {
		return common.NewError("certificate record is nil")
	}
	if err := database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", record.Id).Update("post_action_error", "").Error; err != nil {
		return err
	}
	record.PostActionError = ""
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
	acmeTemporaryFirewallNameFmt  = "ACME temporary allow %d/tcp"
	acmeTemporaryFirewallDescText = "Temporary ACME validation rule, auto removed after issue or renew"
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
		logSession.append(fmt.Sprintf("检查防火墙是否需要临时放行最终验证端口 %d/tcp", port))
	}
	if !IsSystemPlatformLinux() {
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

	if allowed, err := firewallHasManagedDualTCPPortAllowLocked(port); err != nil {
		return nil, err
	} else if allowed {
		if logSession != nil {
			logSession.append(fmt.Sprintf("防火墙已放行 %d/tcp，无需新增规则", port))
		}
		if err := firewallSvc.reconcileLocked(0); err != nil {
			return nil, err
		}
		return nil, nil
	}

	row := buildAcmeTemporaryFirewallRuleRow(port)
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
		logSession.append(fmt.Sprintf("已创建临时防火墙规则 id=%d，放行 %d/tcp", row.Id, port))
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
		logSession.append(fmt.Sprintf("开始还原防火墙，删除临时 %d/tcp 规则 id=%d", rule.port, rule.id))
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
		logSession.append(fmt.Sprintf("防火墙已还原，临时 %d/tcp 放行规则已删除", rule.port))
	}
}

func buildAcmeTemporaryFirewallRuleRow(port int) model.FirewallRule {
	now := time.Now().Unix()
	if port < 1 || port > 65535 {
		port = acmeIPCertificatePortHTTP
	}
	return model.FirewallRule{
		Name:              fmt.Sprintf(acmeTemporaryFirewallNameFmt, port),
		Description:       acmeTemporaryFirewallDescText,
		Enabled:           true,
		Origin:            firewallOriginTemporary,
		SystemKey:         "",
		TemporaryType:     acmeTemporaryFirewallType,
		TemporaryExpireAt: time.Now().Add(acmeTemporaryFirewallLifetime).Unix(),
		Direction:         firewallDirectionIngress,
		Family:            firewallFamilyDual,
		Protocol:          firewallProtocolTCP,
		PortSpec:          strconv.Itoa(port),
		SourceSpec:        "",
		LastSeenAt:        now,
	}
}

func firewallHasManagedDualTCPPortAllowLocked(port int) (bool, error) {
	rows, err := loadFirewallRulesLocked()
	if err != nil {
		return false, err
	}
	v4TCP := false
	v6TCP := false
	for _, row := range rows {
		if !row.Enabled || !firewallRuleParticipatesInManagedChain(row) || row.Direction != firewallDirectionIngress {
			continue
		}
		if strings.TrimSpace(row.SourceSpec) != "" || !firewallPortSpecContains(row.PortSpec, port) {
			continue
		}
		protocol := normalizeFirewallProtocol(row.Protocol)
		if protocol != firewallProtocolTCP && protocol != firewallProtocolTCPUDP {
			continue
		}
		family := normalizeFirewallFamily(row.Family)
		if family == firewallFamilyDual || family == firewallFamilyIPv4 {
			v4TCP = true
		}
		if family == firewallFamilyDual || family == firewallFamilyIPv6 {
			v6TCP = true
		}
		if v4TCP && v6TCP {
			return true, nil
		}
	}
	return false, nil
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

// validateAcmeAccountEmailForServer follows acme.sh account semantics: a
// Let's Encrypt account may omit contacts, while ZeroSSL needs an email to
// obtain EAB credentials. acme.sh accepts comma-separated contact addresses.
func validateAcmeAccountEmailForServer(value string, server string) (string, error) {
	parts := strings.Split(value, ",")
	contacts := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		validated, err := validateAcmeEmail(part)
		if err != nil {
			return "", err
		}
		key := strings.ToLower(validated)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		contacts = append(contacts, validated)
	}
	result := strings.Join(contacts, ",")
	if normalizeSupportedAcmeDomainServer(server) == "zerossl" && result == "" {
		return "", common.NewError("ZeroSSL ACME 账号必须填写邮箱")
	}
	return result, nil
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

// validateAcmeCustomArgs reserves flags that control the account, CA, working
// directories, challenge mode, or hook execution. Those values are selected
// by the certificate record and operation runtime, not by free-form input.
func validateAcmeCustomArgs(raw string) (string, error) {
	customArgs := strings.TrimSpace(raw)
	if customArgs == "" {
		return "", nil
	}
	for _, token := range strings.Fields(customArgs) {
		option := strings.ToLower(strings.TrimSpace(token))
		if index := strings.Index(option, "="); index >= 0 {
			option = option[:index]
		}
		if isManagedAcmeCustomArg(option) {
			return "", common.NewError("附加参数不能覆盖受管 ACME 选项: ", token)
		}
	}
	return customArgs, nil
}

func isManagedAcmeCustomArg(option string) bool {
	option = strings.TrimSpace(strings.ToLower(option))
	if option == "" {
		return false
	}
	if (strings.HasPrefix(option, "-d") && !strings.HasPrefix(option, "--")) ||
		(strings.HasPrefix(option, "-w") && !strings.HasPrefix(option, "--")) ||
		(strings.HasPrefix(option, "-k") && !strings.HasPrefix(option, "--")) ||
		(strings.HasPrefix(option, "-ak") && !strings.HasPrefix(option, "--")) ||
		(strings.HasPrefix(option, "-m") && !strings.HasPrefix(option, "--")) {
		return true
	}
	switch option {
	case "--home", "--config-home", "--cert-home", "--certhome", "--accountconf",
		"--cert-file", "--certpath", "--key-file", "--keypath", "--fullchain-file", "--fullchainpath", "--ca-file", "--capath",
		"--server", "--staging", "--set-default-ca", "--set-default-chain",
		"--accountkey", "--accountkeylength", "--register-account", "--update-account", "--update-account-key", "--deactivate-account", "--email", "--accountemail",
		"--issue", "--renew", "--renew-all", "--install", "--install-cert", "--uninstall", "--upgrade", "--cron", "--revoke", "--deactivate",
		"--domain", "--keylength", "--webroot", "--dns", "--standalone", "--alpn", "--httpport", "--tlsport", "--listen-v4", "--listen-v6", "--local-address",
		"--cert-profile", "--certificate-profile", "--valid-to", "--eab-kid", "--eab-hmac-key",
		"--output-insecure", "--log", "--logfile", "--syslog", "--pre-hook", "--post-hook", "--renew-hook", "--deploy-hook", "--notify-hook",
		"--challenge-alias", "--domain-alias", "--manual", "--insecure", "--ca-bundle":
		return true
	default:
		return false
	}
}

func ensureAcmeFreshIssueArgs(args []string) []string {
	if hasAnyAcmeArg(args, "--force", "-f") {
		return args
	}
	return append(args, "--force")
}

func hasAnyAcmeArg(args []string, names ...string) bool {
	if len(args) == 0 || len(names) == 0 {
		return false
	}
	for _, arg := range args {
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
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

// validateAcmeIssueIdentifiers is the strict path used by public ACME
// issuance. Self-signed issuance keeps normalizeAcmeDomains so local host
// names, localhost and IP identifiers remain compatible there.
func validateAcmeIssueIdentifiers(text string, certificateType string) ([]string, error) {
	if certificateType == acmeCertificateTypeIP {
		result := normalizeAcmeIPIdentifiers(text)
		if err := validateAcmeIssueIdentifierCount(certificateType, len(result)); err != nil {
			return nil, err
		}
		return result, nil
	}

	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return []string{}, nil
	}
	seen := make(map[string]struct{}, len(fields))
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		value, err := normalizeStrictAcmeDomain(field)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if err := validateAcmeIssueIdentifierCount(certificateType, len(result)); err != nil {
		return nil, err
	}
	return result, nil
}

func validateAcmeIssueIdentifierCount(certificateType string, count int) error {
	if certificateType == acmeCertificateTypeIP {
		if count > acmeIPCertificateMaxIPs {
			return common.NewError(fmt.Sprintf("IP 证书最多支持 %d 个 IP", acmeIPCertificateMaxIPs))
		}
		return nil
	}
	if count > acmeDomainCertificateMaxNames {
		return common.NewError(fmt.Sprintf("域名证书最多支持 %d 个域名", acmeDomainCertificateMaxNames))
	}
	return nil
}

func normalizeStrictAcmeDomain(raw string) (string, error) {
	original := strings.TrimSpace(raw)
	if original == "" {
		return "", common.NewError("域名不能为空")
	}
	if strings.ContainsAny(original, "/\\@:") {
		return "", common.NewError("域名格式无效: ", original)
	}

	wildcard := false
	value := original
	if strings.HasPrefix(value, "*.") {
		wildcard = true
		value = strings.TrimPrefix(value, "*.")
	}
	if strings.Contains(value, "*") {
		return "", common.NewError("通配符只能位于域名最前方: ", original)
	}
	value = strings.TrimSuffix(strings.TrimSpace(value), ".")
	if value == "" || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return "", common.NewError("域名格式无效: ", original)
	}
	if _, err := netip.ParseAddr(strings.Trim(value, "[]")); err == nil {
		return "", common.NewError("域名证书不能混入 IP 地址: ", original)
	}

	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil {
		return "", common.NewError("国际化域名格式无效: ", original)
	}
	ascii = strings.ToLower(strings.TrimSpace(ascii))
	if ascii == "" || len(ascii) > 253 {
		return "", common.NewError("域名长度无效: ", original)
	}
	labels := strings.Split(ascii, ".")
	if len(labels) < 2 {
		return "", common.NewError("ACME 域名必须是完整域名: ", original)
	}
	for _, label := range labels {
		if !isValidStrictAcmeDomainLabel(label) {
			return "", common.NewError("域名标签无效: ", original)
		}
	}
	if wildcard {
		return "*." + ascii, nil
	}
	return ascii, nil
}

func isValidStrictAcmeDomainLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, ch := range label {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			continue
		}
		return false
	}
	return true
}

func hasAcmeWildcardDomain(domains []string) bool {
	for _, domain := range domains {
		if strings.HasPrefix(strings.TrimSpace(domain), "*.") {
			return true
		}
	}
	return false
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

func effectiveAcmeAccountKeyLength(entry *model.AcmeAccount) string {
	if entry == nil {
		return defaultAcmeKeyLength
	}
	if value := normalizeAcmeKeyLength(entry.AccountKeyLength); value != "" {
		return value
	}
	if value := normalizeAcmeKeyLength(entry.KeyLength); value != "" {
		return value
	}
	return defaultAcmeKeyLength
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
		if isReservedAcmeRuntimeEnvKey(key) {
			return nil, common.NewError("dns 环境变量不能覆盖 ACME 运行环境: ", key)
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

var certificatePushFileNameSet = map[string]struct{}{
	"cert.pem":      {},
	"key.pem":       {},
	"fullchain.pem": {},
	"chain.pem":     {},
}

func decodeCertificatePushFilePaths(raw string) map[string]string {
	paths, err := parseCertificatePushFilePaths(raw)
	if err != nil {
		return map[string]string{}
	}
	return paths
}

func parseCertificatePushFilePaths(raw string) (map[string]string, error) {
	paths := make(map[string]string)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return paths, nil
	}
	if err := json.Unmarshal([]byte(raw), &paths); err != nil {
		return nil, common.NewError("parse certificate push file paths failed: ", err)
	}
	normalized := make(map[string]string, len(paths))
	for name, filePath := range paths {
		if _, ok := certificatePushFileNameSet[name]; !ok {
			return nil, common.NewError("unsupported pushed certificate filename: ", name)
		}
		filePath = strings.TrimSpace(filePath)
		if filePath == "" {
			return nil, common.NewError("pushed certificate file path is empty: ", name)
		}
		filePath = filepath.Clean(filePath)
		if filepath.Base(filePath) != name {
			return nil, common.NewError("pushed certificate file path does not match filename: ", name)
		}
		normalized[name] = filePath
	}
	return normalized, nil
}

func encodeCertificatePushFilePaths(paths map[string]string) string {
	if len(paths) == 0 {
		return ""
	}
	raw, err := json.Marshal(paths)
	if err != nil {
		return ""
	}
	normalized, err := parseCertificatePushFilePaths(string(raw))
	if err != nil || len(normalized) == 0 {
		return ""
	}
	raw, err = json.Marshal(normalized)
	if err != nil {
		return ""
	}
	return string(raw)
}

func validateCertificatePushFilePaths(targetDir string, paths map[string]string) error {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir == "" {
		return common.NewError("pushed certificate directory is empty")
	}
	if len(paths) == 0 {
		return common.NewError("pushed certificate file paths are empty")
	}
	baseDir := filepath.Clean(targetDir)
	for name, filePath := range paths {
		if _, ok := certificatePushFileNameSet[name]; !ok {
			return common.NewError("unsupported pushed certificate filename: ", name)
		}
		filePath = filepath.Clean(strings.TrimSpace(filePath))
		if filepath.Base(filePath) != name {
			return common.NewError("pushed certificate file path does not match filename: ", name)
		}
		rel, err := filepath.Rel(baseDir, filePath)
		if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return common.NewError("pushed certificate file path escapes target directory: ", filePath)
		}
	}
	return nil
}

func removeVerifiedCertificateFiles(targetDir string, rawPaths string) error {
	paths, err := parseCertificatePushFilePaths(rawPaths)
	if err != nil {
		return err
	}
	if err := validateCertificatePushFilePaths(targetDir, paths); err != nil {
		return err
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.Remove(paths[name]); err != nil && !os.IsNotExist(err) {
			return common.NewError("delete ", name, " failed: ", err)
		}
	}
	return nil
}

func writeAndVerifyCertificateDirectoryPush(targetDir string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) (map[string]string, error) {
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

	paths := make(map[string]string, len(writeTargets))
	written := make([]string, 0, len(writeTargets))
	cleanupWritten := func() {
		for _, writtenPath := range written {
			_ = os.Remove(writtenPath)
		}
	}
	for _, target := range writeTargets {
		filePath := filepath.Join(targetDir, target.Name)
		paths[target.Name] = filePath
		if err := os.WriteFile(filePath, target.Data, target.Perm); err != nil {
			cleanupWritten()
			return nil, common.NewError("write ", target.Name, " failed: ", err)
		}
		written = append(written, filePath)
	}
	for _, target := range writeTargets {
		actual, readErr := os.ReadFile(paths[target.Name])
		if readErr != nil {
			cleanupWritten()
			return nil, common.NewError("read ", target.Name, " for verify failed: ", readErr)
		}
		if !bytes.Equal(actual, target.Data) {
			cleanupWritten()
			return nil, common.NewError("verify ", target.Name, " failed: file content does not match latest certificate")
		}
	}
	return paths, nil
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

// ensureAcmeDNSProviderScript surfaces an actionable error before acme.sh
// starts a DNS-01 command with an installation that lacks the provider script.
func ensureAcmeDNSProviderScript(homeDir string, providerCode string) error {
	provider, known := lookupAcmeDNSProvider(providerCode)
	if !known {
		// Custom dnsapi providers retain acme.sh's existing compatibility path.
		return nil
	}

	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return common.NewError("无法确认 acme.sh 的 DNS API 目录，请重新安装或升级 acme.sh 后重试")
	}

	scriptPath := filepath.Join(homeDir, "dnsapi", provider.ProviderCode+".sh")
	info, err := os.Stat(scriptPath)
	if err == nil && !info.IsDir() {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return common.NewError("检查 acme.sh DNS API 脚本失败: ", err)
	}

	return common.NewError("当前安装的 acme.sh 不包含 ", provider.ProviderCode, " DNS API 脚本；请升级 acme.sh 后重试")
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

func resolveAcmeDNSRuntimeConfig(dnsAccountID uint, fallbackProvider string, dnsEnvText string) (acmeDNSRuntimeConfig, error) {
	result := acmeDNSRuntimeConfig{
		ProviderCode: strings.TrimSpace(fallbackProvider),
		EnvPairs:     []string{},
	}
	accountEnv := []string{}

	if dnsAccountID > 0 {
		dnsAccount := &model.AcmeDNSAccount{}
		if err := database.GetDB().Where("id = ?", dnsAccountID).First(dnsAccount).Error; err != nil {
			return acmeDNSRuntimeConfig{}, err
		}
		result.AccountName = strings.TrimSpace(dnsAccount.Name)
		result.ProviderCode = strings.TrimSpace(dnsAccount.ProviderCode)
		if result.ProviderCode == "" {
			return acmeDNSRuntimeConfig{}, common.NewError("已绑定 DNS 账号未配置 DNS 供应商")
		}
		if _, ok := lookupAcmeDNSProvider(result.ProviderCode); !ok {
			return acmeDNSRuntimeConfig{}, common.NewError("已绑定 DNS 账号使用不支持的 DNS 供应商: ", result.ProviderCode)
		}
		envMap, err := parseAcmeEnvJSON(dnsAccount.EnvJSON)
		if err != nil {
			return acmeDNSRuntimeConfig{}, err
		}
		accountEnv = envMapToEnvPairs(envMap)
	}

	manualEnv, err := normalizeAcmeEnvAssignments(strings.TrimSpace(dnsEnvText))
	if err != nil {
		return acmeDNSRuntimeConfig{}, err
	}
	result.EnvPairs = mergeEnvPairs(accountEnv, manualEnv)
	if err := validateAcmeDNSRuntimeEnvMap(envPairsToEnvMap(result.EnvPairs)); err != nil {
		return acmeDNSRuntimeConfig{}, err
	}

	if provider, ok := lookupAcmeDNSProvider(result.ProviderCode); ok {
		result.ProviderCode = provider.ProviderCode
		if err := validateDNSProviderEnv(provider, envPairsToEnvMap(result.EnvPairs)); err != nil {
			return acmeDNSRuntimeConfig{}, err
		}
	}
	return result, nil
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
		strings.Contains(key, "key_file") ||
		strings.Contains(key, "access_key") ||
		strings.Contains(key, "api_key") ||
		strings.HasSuffix(key, "_key") ||
		strings.HasSuffix(key, "_key_id") ||
		strings.HasSuffix(key, "_secret")
}

func validateDNSProviderEnv(provider AcmeDNSProviderMeta, env map[string]string) error {
	if err := validateAcmeDNSRuntimeEnvMap(env); err != nil {
		return err
	}
	trim := func(key string) string {
		return strings.TrimSpace(env[key])
	}
	switch strings.ToLower(strings.TrimSpace(provider.ProviderCode)) {
	case "dns_cf":
		token := trim("CF_Token")
		email := trim("CF_Email")
		key := trim("CF_Key")
		tokenMode := token != ""
		legacyMode := email != "" && key != ""
		if !tokenMode && !legacyMode {
			return common.NewError("Cloudflare DNS 需填写以下其一：CF_Token，或 CF_Email + CF_Key")
		}
		return nil
	case "dns_azure":
		if trim("AZUREDNS_SUBSCRIPTIONID") == "" {
			return common.NewError("Azure DNS 必须填写 AZUREDNS_SUBSCRIPTIONID")
		}
		if strings.EqualFold(trim("AZUREDNS_MANAGEDIDENTITY"), "true") {
			return nil
		}
		if trim("AZUREDNS_BEARERTOKEN") != "" {
			return nil
		}
		if trim("AZUREDNS_TENANTID") == "" || trim("AZUREDNS_APPID") == "" || trim("AZUREDNS_CLIENTSECRET") == "" {
			return common.NewError("Azure DNS 需使用 Managed Identity、Bearer Token，或同时填写 AZUREDNS_TENANTID、AZUREDNS_APPID、AZUREDNS_CLIENTSECRET")
		}
		return nil
	case "dns_gandi_livedns":
		if trim("GANDI_LIVEDNS_TOKEN") == "" && trim("GANDI_LIVEDNS_KEY") == "" {
			return common.NewError("Gandi LiveDNS 需填写 GANDI_LIVEDNS_TOKEN（推荐）或旧版 GANDI_LIVEDNS_KEY")
		}
		return nil
	case "dns_cloudns":
		if trim("CLOUDNS_AUTH_ID") == "" && trim("CLOUDNS_SUB_AUTH_ID") == "" {
			return common.NewError("ClouDNS 需填写 CLOUDNS_AUTH_ID 或 CLOUDNS_SUB_AUTH_ID")
		}
		if trim("CLOUDNS_AUTH_PASSWORD") == "" {
			return common.NewError("ClouDNS 必须填写 CLOUDNS_AUTH_PASSWORD")
		}
		return nil
	case "dns_yc":
		if trim("YC_Zone_ID") == "" && trim("YC_Folder_ID") == "" {
			return common.NewError("Yandex Cloud DNS 需填写 YC_Zone_ID 或 YC_Folder_ID")
		}
		if trim("YC_SA_ID") == "" || trim("YC_SA_Key_ID") == "" {
			return common.NewError("Yandex Cloud DNS 必须填写 YC_SA_ID 与 YC_SA_Key_ID")
		}
		if trim("YC_SA_Key_File_Path") == "" && trim("YC_SA_Key_File_PEM_b64") == "" {
			return common.NewError("Yandex Cloud DNS 需填写 RSA 私钥文件路径或 YC_SA_Key_File_PEM_b64")
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

	for _, homeDir := range s.legacyAcmeRuntimeHomes() {
		homeDir = strings.TrimSpace(filepath.Clean(homeDir))
		if homeDir == "" || homeDir == "." || !pathExists(filepath.Join(homeDir, "account.conf")) {
			continue
		}

		candidates, err := loadLegacyDNSCandidatesFromAccountConf(homeDir)
		if err != nil {
			return err
		}
		if len(candidates) == 0 {
			continue
		}
		if err := s.persistLegacyDNSCandidates(homeDir, candidates); err != nil {
			return err
		}
	}
	return nil
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

func isReservedAcmeRuntimeEnvKey(key string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	if key == "" {
		return false
	}
	if strings.HasPrefix(key, "LD_") || strings.HasPrefix(key, "DYLD_") ||
		strings.HasPrefix(key, "LE_") || strings.HasPrefix(key, "ACCOUNT_") ||
		strings.HasPrefix(key, "CA_") || strings.HasPrefix(key, "CERT_") ||
		strings.HasPrefix(key, "DOMAIN_") || strings.HasPrefix(key, "ACME_") {
		return true
	}
	switch key {
	case "LE_CONFIG_HOME", "LE_WORKING_DIR", "_SCRIPT_HOME",
		"PATH", "HOME", "PWD", "OLDPWD", "SHELL", "IFS", "ENV", "BASH_ENV",
		"TMPDIR", "TMP", "TEMP",
		"PYTHONPATH", "PYTHONHOME", "PERL5LIB", "PERL5OPT", "RUBYOPT", "RUBYLIB", "NODE_OPTIONS",
		"DEBUG", "OUTPUT_INSECURE", "LOG_FILE", "LOG_LEVEL", "SYS_LOG", "FORCE", "NO_TIMESTAMP":
		return true
	default:
		return false
	}
}

func isAcmeInheritedRuntimeEnvKey(key string) bool {
	// acme.sh's shebang uses /usr/bin/env, so retain PATH while filtering the
	// remaining variables that could redirect its state or startup behavior.
	return !strings.EqualFold(strings.TrimSpace(key), "PATH") && isReservedAcmeRuntimeEnvKey(key)
}

func buildAcmeCommandEnv(envPairs []string) []string {
	values := make(map[string]string, len(os.Environ())+len(envPairs))
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || strings.TrimSpace(key) == "" || isAcmeInheritedRuntimeEnvKey(key) {
			continue
		}
		values[strings.ToUpper(key)] = key + "=" + value
	}
	for _, pair := range envPairs {
		key, value, ok := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" || !isValidAcmeEnvKey(key) || isReservedAcmeRuntimeEnvKey(key) {
			continue
		}
		values[strings.ToUpper(key)] = key + "=" + value
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, values[key])
	}
	return result
}

func redactAcmeCommandOutput(raw string, envPairs []string) string {
	if raw == "" || len(envPairs) == 0 {
		return raw
	}
	secretValues := make([]string, 0, len(envPairs))
	seen := make(map[string]struct{}, len(envPairs))
	for _, pair := range envPairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || !isAcmeSecretEnvKey(key) || value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		secretValues = append(secretValues, value)
	}
	sort.Slice(secretValues, func(i, j int) bool {
		return len(secretValues[i]) > len(secretValues[j])
	})
	for _, value := range secretValues {
		raw = strings.ReplaceAll(raw, value, acmeMaskedEnvValue)
	}
	return raw
}

func validateAcmeDNSRuntimeEnvMap(env map[string]string) error {
	for key := range env {
		key = strings.TrimSpace(key)
		if !isValidAcmeEnvKey(key) {
			return common.NewError("无效 DNS 环境变量名: ", key)
		}
		if isReservedAcmeRuntimeEnvKey(key) {
			return common.NewError("dns 环境变量不能覆盖 ACME 运行环境: ", key)
		}
	}
	return nil
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

func envPairsToEnvMap(envPairs []string) map[string]string {
	result := make(map[string]string, len(envPairs))
	for _, pair := range envPairs {
		pair = strings.TrimSpace(pair)
		idx := strings.Index(pair, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(pair[:idx])
		value := strings.TrimSpace(pair[idx+1:])
		if key == "" || value == "" {
			continue
		}
		result[key] = value
	}
	return result
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
	managedInstallation := installed && isManagedAcmeInstallation(scriptPath, homeDir)
	if managedInstallation {
		uninstallOutput, uninstallErr := runCommandOutputWithTimeoutEnv(60*time.Second, scriptPath, append(acmeHomeArgs(homeDir), "--uninstall"), nil)
		if uninstallErr != nil {
			if runtime.GOOS == "linux" {
				detail := strings.TrimSpace(uninstallOutput)
				if detail != "" {
					return nil, fmt.Errorf("uninstall managed acme.sh failed: %w: %s", uninstallErr, detail)
				}
				return nil, fmt.Errorf("uninstall managed acme.sh failed: %w", uninstallErr)
			}
			logger.Warning("skip managed acme.sh uninstall outside Linux host: ", uninstallErr)
		}
		trimmed := strings.TrimSpace(uninstallOutput)
		if trimmed != "" {
			outputParts = append(outputParts, trimmed)
		}
	}

	if managedInstallation && runtime.GOOS == "linux" && !runningInsideContainer() {
		uninstallOptions := normalizeKworUninstallOptions(KworUninstallOptions{DataDir: config.GetDataDir()})
		for _, unit := range acmeSystemdUnitCandidates {
			unit = strings.TrimSpace(unit)
			if unit == "" || !legacySystemdUnitLooksOwned(unit, uninstallOptions) {
				continue
			}
			paths := ownedSystemdUnitArtifactPaths(unit, uninstallOptions, nil, nil, nil, true)
			if err := removeOwnedSystemdUnit(unit, uninstallOptions, paths, nil, nil, true); err != nil {
				return nil, fmt.Errorf("remove managed acme systemd unit %s: %w", unit, err)
			}
			removedUnits = append(removedUnits, unit)
		}
		if systemctlPath, lookErr := exec.LookPath("systemctl"); lookErr == nil {
			if err := runCommandWithTimeout(10*time.Second, systemctlPath, "daemon-reload"); err != nil {
				return nil, fmt.Errorf("reload systemd after managed acme cleanup: %w", err)
			}
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
		if err := os.RemoveAll(runtimeRoot); err != nil {
			return nil, fmt.Errorf("remove managed acme runtime data: %w", err)
		}
	}
	if err := s.EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}
	if err := RemoveHostResource("acme-managed-runtime"); err != nil {
		return nil, fmt.Errorf("clear managed acme ownership: %w", err)
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

func isManagedAcmeInstallation(scriptPath string, homeDir string) bool {
	return isManagedAcmeScriptPath(scriptPath) || isManagedAcmeHomeDir(homeDir)
}

func removeAcmeAccountRowsTx(tx *gorm.DB) error {
	if err := tx.Where("1 = 1").Delete(&model.AcmeAccount{}).Error; err != nil {
		return err
	}
	if err := tx.Where("1 = 1").Delete(&model.AcmeDNSAccount{}).Error; err != nil {
		return err
	}
	return nil
}

func (s *AcmeService) removeAcmeCertificatesAndInventoryLocked() error {
	db := database.GetDB()
	rows := make([]model.CertificateRecord, 0)
	if err := db.Select("id").Find(&rows).Error; err != nil {
		return err
	}
	bindings, err := detachAndDeleteCertificateRecords(rows, func(tx *gorm.DB) error {
		if err := removeAcmeAccountRowsTx(tx); err != nil {
			return err
		}
		if err := tx.Where("1 = 1").Delete(&model.SelfSignedAuthority{}).Error; err != nil {
			return err
		}
		return tx.Where("key IN ?", []string{"webCertFile", "webKeyFile", "subCertFile", "subKeyFile"}).Delete(&model.Setting{}).Error
	})
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		syncDetachedCertificateBindings(bindings)
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
	return s.checkVersionDownloadableLockedContext(context.Background(), version)
}

func (s *AcmeService) checkVersionDownloadableLockedContext(ctx context.Context, version string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	version = normalizeAcmeVersionTag(version)
	if version == "" {
		return false, nil
	}
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", acmeGitHubReleaseTagAPI+version, nil)
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
			tags, hasMore, err := s.fetchAcmeTagPageByURLContext(ctx, client, url, perPage)
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

	body, err := readBoundedHTTPResponseBody(resp.Body, acmeGitHubResponseMaxBytes)
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
	return s.fetchAcmeTagPageByURLContext(context.Background(), client, apiURL, perPage)
}

func (s *AcmeService) fetchAcmeTagPageByURLContext(ctx context.Context, client *http.Client, apiURL string, perPage int) ([]AcmeVersionItem, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
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
	body, err := readBoundedHTTPResponseBody(resp.Body, acmeGitHubResponseMaxBytes)
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
	return readAcmeVersionByScriptContext(context.Background(), scriptPath, homeDir)
}

// readAcmeVersionFromScriptFile keeps the overview path side-effect free. The
// installed acme.sh script exposes VER near its header, so a bounded local
// read avoids spawning acme.sh for every settings-page poll.
func readAcmeVersionFromScriptFile(scriptPath string) (string, error) {
	scriptPath = filepath.Clean(strings.TrimSpace(scriptPath))
	if scriptPath == "" || scriptPath == "." {
		return "", common.NewError("acme.sh script path is empty")
	}
	file, err := os.Open(scriptPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(io.LimitReader(file, 64*1024))
	scanner.Buffer(make([]byte, 256), 64*1024)
	for scanner.Scan() {
		key, value, found := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !found || strings.TrimSpace(key) != "VER" {
			continue
		}
		version := normalizeAcmeVersionTag(strings.Trim(strings.TrimSpace(value), "\"'"))
		if version != "" && isLikelySemverTag(version) {
			return version, nil
		}
		return "", common.NewError("unable to detect acme.sh version from script header")
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", common.NewError("unable to detect acme.sh version from script header")
}

func readAcmeVersionByScriptContext(ctx context.Context, scriptPath string, homeDir string) (string, error) {
	output, err := runCommandOutputWithTimeoutEnvContext(ctx, 12*time.Second, scriptPath, append(acmeHomeArgs(homeDir), "--version"), nil)
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
	return downloadAcmeInstallerScriptContext(context.Background(), targetPath)
}

func downloadAcmeInstallerScriptContext(parent context.Context, targetPath string) error {
	if targetPath == "" {
		return common.NewError("acme installer target path is empty")
	}
	if parent == nil {
		parent = context.Background()
	}

	if curlPath, err := exec.LookPath("curl"); err == nil {
		if err := runAcmeCommandWithContext(parent, 45*time.Second, curlPath, "-fsSL", defaultAcmeInstallScriptURL, "-o", targetPath); err == nil {
			return nil
		}
	}
	if wgetPath, err := exec.LookPath("wget"); err == nil {
		if err := runAcmeCommandWithContext(parent, 45*time.Second, wgetPath, "-qO", targetPath, defaultAcmeInstallScriptURL); err == nil {
			return nil
		}
	}
	return common.NewError("failed to download acme.sh installer: curl/wget unavailable or network failed")
}

func runAcmeCommandWithContext(parent context.Context, timeout time.Duration, command string, args ...string) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("command failed (%s): %w: %s", command, err, strings.TrimSpace(string(output)))
	}
	return nil
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
	return runCommandOutputWithTimeoutEnvContext(context.Background(), timeout, command, args, envPairs)
}

func runCommandOutputWithTimeoutEnvLog(timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error) {
	parent := context.Background()
	if logSession != nil {
		parent = logSession.operationContext()
	}
	return runCommandOutputWithTimeoutEnvContextLog(parent, timeout, command, args, envPairs, logSession)
}

func runCommandOutputWithTimeoutEnvContext(parent context.Context, timeout time.Duration, command string, args []string, envPairs []string) (string, error) {
	return runCommandOutputWithTimeoutEnvContextLog(parent, timeout, command, args, envPairs, nil)
}

func runCommandOutputWithTimeoutEnvContextLog(parent context.Context, timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error) {
	return runCommandOutputWithTimeoutEnvContextLogInDir(parent, timeout, command, args, envPairs, logSession, "")
}

func runCommandOutputWithTimeoutEnvContextLogInDir(parent context.Context, timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession, workingDir string) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = buildAcmeCommandEnv(envPairs)
	if workingDir = strings.TrimSpace(workingDir); workingDir != "" {
		cmd.Dir = filepath.Clean(workingDir)
	}
	PrepareKworManagedCommandContext(parent, cmd)

	var output boundedAcmeOutput
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
	if err := TrackKworManagedCommandContext(parent, cmd); err != nil {
		stopKworManagedCommand(cmd)
		_ = cmd.Wait()
		return "", fmt.Errorf("登记 ACME 命令进程失败: %w", err)
	}
	stopWatchingCommand := watchKworManagedCommandContext(ctx, cmd)
	defer stopWatchingCommand()

	var wg sync.WaitGroup
	collect := func(reader io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 1024), acmeCommandOutputLineMaxBytes)
		for scanner.Scan() {
			line := redactAcmeCommandOutput(strings.TrimRight(scanner.Text(), "\r"), envPairs)
			outputMu.Lock()
			output.AppendLine(line)
			outputMu.Unlock()
			if logSession != nil {
				logSession.append(line)
			}
		}
		if scanErr := scanner.Err(); scanErr != nil && logSession != nil && ctx.Err() == nil && !errors.Is(scanErr, os.ErrClosed) {
			logSession.append("读取命令输出失败: " + scanErr.Error())
		}
	}
	wg.Add(2)
	go collect(stdout)
	go collect(stderr)

	err = cmd.Wait()
	wg.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out (%s)", command)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return "", fmt.Errorf("command canceled (%s): %w", command, ctx.Err())
	}
	if err != nil {
		outputMu.Lock()
		text := strings.TrimSpace(output.String())
		outputMu.Unlock()
		if text == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, truncateAcmeTextByBytes(text, acmeCommandErrorMaxBytes))
	}
	outputMu.Lock()
	text := output.String()
	outputMu.Unlock()
	return text, nil
}

type boundedAcmeOutput struct {
	builder    strings.Builder
	truncated  bool
	byteLength int
}

func (b *boundedAcmeOutput) AppendLine(line string) {
	if b == nil || b.truncated {
		return
	}
	remaining := acmeCommandOutputMaxBytes - b.byteLength
	if remaining <= 0 {
		b.truncated = true
		return
	}
	line += "\n"
	if len(line) > remaining {
		truncated := truncateAcmeTextByBytes(line, remaining)
		b.builder.WriteString(truncated)
		b.byteLength += len(truncated)
		b.truncated = true
		return
	}
	b.builder.WriteString(line)
	b.byteLength += len(line)
}

func (b *boundedAcmeOutput) String() string {
	if b == nil {
		return ""
	}
	value := b.builder.String()
	if !b.truncated {
		return value
	}
	return value + "\n... command output truncated after 512 KiB"
}

func truncateAcmeStoredOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= acmeStoredOutputMaxBytes {
		return value
	}
	return strings.TrimSpace(truncateAcmeTextByBytes(value, acmeStoredOutputMaxBytes)) + "\n... 历史命令输出已截断（最多 128 KiB）"
}

func truncateAcmeTextByBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || value == "" {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
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
	lineBytes  int
	truncated  bool
	errText    string
	taskID     string
	taskStatus string
	warnings   []string
	result     *AcmeActionResult
	startedAt  int64
	updatedAt  int64
	finishedAt int64
	ctx        context.Context
	cancel     context.CancelFunc
	operation  *KworManagedOperationHandle
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
	if existing := s.sessions[id]; existing != nil && (existing.status == acmeTaskStatusQueued || existing.status == acmeTaskStatusRunning) {
		if strings.TrimSpace(title) != "" {
			existing.title = strings.TrimSpace(title)
		}
		existing.status = acmeTaskStatusRunning
		existing.taskStatus = acmeTaskStatusRunning
		existing.updatedAt = now
		return existing
	}

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

func (s *acmeLogStore) queue(id string, title string, taskID string, ctx context.Context, operation *KworManagedOperationHandle) *acmeLogSession {
	id = normalizeAcmeLogSessionID(id)
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)

	session := &acmeLogSession{
		id:         id,
		title:      strings.TrimSpace(title),
		status:     acmeTaskStatusQueued,
		taskID:     strings.TrimSpace(taskID),
		taskStatus: acmeTaskStatusQueued,
		startedAt:  now,
		updatedAt:  now,
		ctx:        ctx,
		operation:  operation,
	}
	if session.title == "" {
		session.title = "ACME 任务"
	}
	session.appendLocked("后台任务已进入队列")
	s.sessions[id] = session
	return session
}

func (s *acmeLogSession) operationContext() context.Context {
	if s == nil || s.ctx == nil {
		return context.Background()
	}
	return s.ctx
}

func (s *acmeLogSession) ensureManagedOperation(kind string) (func(), error) {
	if s == nil {
		return func() {}, nil
	}
	acmeLogSessionStore.mu.Lock()
	if s.operation != nil {
		acmeLogSessionStore.mu.Unlock()
		return func() {}, nil
	}
	acmeLogSessionStore.mu.Unlock()

	ctx, operation, err := BeginKworManagedOperation("acme-" + strings.TrimSpace(kind))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, acmeTaskDeadline)
	attached := false
	acmeLogSessionStore.mu.Lock()
	if s.operation == nil {
		s.ctx = ctx
		s.cancel = cancel
		s.operation = operation
		s.updatedAt = time.Now().Unix()
		attached = true
	}
	acmeLogSessionStore.mu.Unlock()
	if !attached {
		cancel()
		operation.Done()
		return func() {}, nil
	}
	return func() {
		cancel()
		operation.Done()
	}, nil
}

func (s *acmeLogSession) trackCommand(cmd *exec.Cmd) error {
	if s == nil || s.operation == nil {
		return nil
	}
	return s.operation.TrackCommand(cmd)
}

func (s *acmeLogStore) setTaskState(logSessionID string, taskID string, status string, warnings []string, result *AcmeActionResult, errText string) {
	logSessionID = strings.TrimSpace(logSessionID)
	if logSessionID == "" {
		return
	}
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.sessions[logSessionID]
	if session == nil {
		return
	}
	session.taskID = strings.TrimSpace(taskID)
	session.taskStatus = strings.TrimSpace(status)
	session.warnings = append([]string(nil), warnings...)
	session.result = cloneAcmeActionResult(result)
	if status != "" {
		session.status = status
	}
	if strings.TrimSpace(errText) != "" {
		session.errText = strings.TrimSpace(errText)
	}
	if status == acmeTaskStatusRunning {
		session.appendLocked("后台任务开始执行")
	}
	if status == acmeTaskStatusWarning {
		session.appendLocked("证书已签发，但部分后置动作需要处理")
	}
	if status == acmeTaskStatusSuccess {
		session.appendLocked("后台任务执行完成")
	}
	if status == acmeTaskStatusError && strings.TrimSpace(errText) != "" {
		session.appendLocked("失败: " + strings.TrimSpace(errText))
	}
	session.updatedAt = now
	if status == acmeTaskStatusSuccess || status == acmeTaskStatusWarning || status == acmeTaskStatusError {
		session.finishedAt = now
	}
}

func (s *acmeLogStore) get(id string) *AcmeLogSessionView {
	return s.getAfter(id, -1)
}

func (s *acmeLogStore) getAfter(id string, after int) *AcmeLogSessionView {
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
			LineStart: 0,
			LineNext:  1,
			StartedAt: now,
			UpdatedAt: now,
		}
	}
	return session.snapshotAfterLocked(after)
}

func (s *acmeLogStore) remove(id string) {
	if s == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
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
	acmeLogSessionStore.mu.Lock()
	defer acmeLogSessionStore.mu.Unlock()
	s.appendLocked(line)
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
	if len(line) > acmeLogMaxLineBytes {
		const suffix = "... 单行日志已截断"
		line = truncateAcmeTextByBytes(line, acmeLogMaxLineBytes-len(suffix)) + suffix
	}
	if s.truncated {
		return
	}
	lineBytes := len(line)
	if len(s.lines) >= acmeLogMaxLines || s.lineBytes+lineBytes > acmeLogMaxBytes {
		s.lines = []string{"日志过长，后续输出已截断"}
		s.lineBytes = len(s.lines[0])
		s.truncated = true
		return
	}
	s.lines = append(s.lines, line)
	s.lineBytes += lineBytes
}

func (s *acmeLogSession) snapshotLocked() *AcmeLogSessionView {
	return s.snapshotAfterLocked(-1)
}

func (s *acmeLogSession) snapshotAfterLocked(after int) *AcmeLogSessionView {
	start := 0
	if after >= 0 && after <= len(s.lines) {
		start = after
	}
	lines := append([]string(nil), s.lines[start:]...)
	return &AcmeLogSessionView{
		Id:         s.id,
		Title:      s.title,
		Status:     s.status,
		Lines:      lines,
		LineStart:  start,
		LineNext:   len(s.lines),
		Error:      s.errText,
		TaskID:     s.taskID,
		TaskStatus: s.taskStatus,
		Warnings:   append([]string(nil), s.warnings...),
		Result:     cloneAcmeActionResult(s.result),
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
