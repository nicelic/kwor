package ddns

import (
	"context"
	"errors"
	"strings"
)

type RecordParam struct {
	Domain string
	Type   string // "A" or "AAAA"
	IP     string
	TTL    int
	Proxy  bool // For Cloudflare orange cloud CDN
}

type ProviderFieldDef struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Required    bool   `json:"required"`
	Placeholder string `json:"placeholder,omitempty"`
	Secret      bool   `json:"secret,omitempty"`
}

type ProviderMeta struct {
	Code              string             `json:"code"`
	Name              string             `json:"name"`
	Helper            string             `json:"helper"`
	Fields            []ProviderFieldDef `json:"fields"`
	SupportGeneralDNS bool               `json:"supportGeneralDns"`
}

type DNSRecord struct {
	ID       string `json:"id"`
	Name     string `json:"name"`     // Host record / RR (e.g. "@", "www", "api")
	Type     string `json:"type"`     // A, AAAA, CNAME, TXT, MX, NS, SRV, CAA
	Value    string `json:"value"`    // Record content / Target / IP
	TTL      int    `json:"ttl"`      // Time To Live seconds
	Priority int    `json:"priority"` // Priority for MX
	Proxied  bool   `json:"proxied"`  // Cloudflare CDN proxy
}

type DDNSProvider interface {
	Code() string
	Name() string
	TestAuth(ctx context.Context, env map[string]string) error
	SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (recordID string, err error)
	DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error
}

type GeneralDNSProvider interface {
	DDNSProvider
	ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error)
	CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error)
	UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error
	DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error
	ListDomains(ctx context.Context, env map[string]string) ([]string, error)
}

var providers = map[string]DDNSProvider{}

func RegisterProvider(p DDNSProvider) {
	providers[p.Code()] = p
}

func GetProvider(code string) (DDNSProvider, error) {
	if p, ok := providers[code]; ok {
		return p, nil
	}
	// Fallback to generic provider if defined
	if p, ok := providers["generic_"+code]; ok {
		return p, nil
	}
	return &GenericProvider{code: code}, nil
}

func GetProviderCatalog() []ProviderMeta {
	catalog := []ProviderMeta{
		{
			Name:              "阿里云",
			Code:              "dns_ali",
			Helper:            "阿里云云解析 DNS；需要具备 AliyunDNSFullAccess 权限的 AccessKey",
			SupportGeneralDNS: true,
			Fields: []ProviderFieldDef{
				{Key: "Ali_Key", Label: "Access Key", Required: true, Secret: false, Placeholder: "LTAI..."},
				{Key: "Ali_Secret", Label: "Secret Key", Required: true, Secret: true, Placeholder: "Secret..."},
			},
		},
		{
			Name:              "腾讯云 DNSPod",
			Code:              "dns_tencent",
			Helper:            "腾讯云 DNSPod 解析；需要具备 DNSPod 读写权限的 SecretId 与 SecretKey",
			SupportGeneralDNS: true,
			Fields: []ProviderFieldDef{
				{Key: "Tencent_SecretId", Label: "SecretId", Required: true, Secret: false, Placeholder: "AKID..."},
				{Key: "Tencent_SecretKey", Label: "SecretKey", Required: true, Secret: true, Placeholder: "SecretKey..."},
			},
		},
		{
			Name:              "Cloudflare",
			Code:              "dns_cf",
			Helper:            "Cloudflare DNS；支持 Token 模式（CF_Token 具备 Zone.DNS 权限，CF_Zone_ID 可选）或 Global Key 模式（CF_Email + CF_Key）",
			SupportGeneralDNS: true,
			Fields: []ProviderFieldDef{
				{Key: "CF_Token", Label: "API Token", Required: false, Secret: true, Placeholder: "Bearer Token with Zone.DNS edit permission"},
				{Key: "CF_Zone_ID", Label: "Zone ID（可选）", Required: false, Secret: false, Placeholder: "留空将自动根据域名解析查找"},
				{Key: "CF_Account_ID", Label: "Account ID（可选）", Required: false, Secret: false},
				{Key: "CF_Email", Label: "Global API Email（可选）", Required: false, Secret: false},
				{Key: "CF_Key", Label: "Global API Key（可选）", Required: false, Secret: true},
			},
		},
		{
			Name:              "Spaceship",
			Code:              "dns_spaceship",
			Helper:            "Spaceship DNS；API Key 需要 domains:read（用于自动发现根域名）、dnsrecords:read 和 dnsrecords:write 权限",
			SupportGeneralDNS: true,
			Fields: []ProviderFieldDef{
				{Key: "SPACESHIP_API_KEY", Label: "API Key", Required: true, Secret: false},
				{Key: "SPACESHIP_API_SECRET", Label: "API Secret", Required: true, Secret: true},
				{Key: "SPACESHIP_ROOT_DOMAIN", Label: "根域名（可选）", Required: false, Placeholder: "example.com"},
			},
		},
		{
			Name:         "Amazon Route53",
			Code:         "dns_aws",
			Helper:       "AWS Route53 DNS；支持静态 AK/SK",
			Fields: []ProviderFieldDef{
				{Key: "AWS_ACCESS_KEY_ID", Label: "Access Key ID", Required: true, Secret: false},
				{Key: "AWS_SECRET_ACCESS_KEY", Label: "Secret Access Key", Required: true, Secret: true},
				{Key: "AWS_DNS_SLOWRATE", Label: "Slow Rate Seconds（可选）", Required: false},
			},
		},
		{
			Name:         "华为云",
			Code:         "dns_huaweicloud",
			Helper:       "华为云 DNS；HUAWEICLOUD_Region 可选，默认 ap-southeast-1",
			Fields: []ProviderFieldDef{
				{Key: "HUAWEICLOUD_Username", Label: "用户名", Required: true, Secret: false},
				{Key: "HUAWEICLOUD_Password", Label: "密码", Required: true, Secret: true},
				{Key: "HUAWEICLOUD_DomainName", Label: "DomainName", Required: true, Secret: false},
				{Key: "HUAWEICLOUD_Region", Label: "Region（可选）", Required: false, Placeholder: "cn-north-4"},
			},
		},
		{
			Name:              "GoDaddy",
			Code:              "dns_gd",
			Helper:            "GoDaddy DNS API",
			SupportGeneralDNS: true,
			Fields: []ProviderFieldDef{
				{Key: "GD_Key", Label: "API Key", Required: true, Secret: false},
				{Key: "GD_Secret", Label: "API Secret", Required: true, Secret: true},
			},
		},
		{
			Name:         "Vercel",
			Code:         "dns_vercel",
			Helper:       "Vercel DNS API",
			Fields: []ProviderFieldDef{
				{Key: "VERCEL_TOKEN", Label: "API Token", Required: true, Secret: true},
			},
		},
		{
			Name:         "Google Cloud DNS",
			Code:         "dns_gcloud",
			Helper:       "Google Cloud DNS；依赖本地 gcloud 配置或服务凭证",
			Fields: []ProviderFieldDef{
				{Key: "CLOUDSDK_ACTIVE_CONFIG_NAME", Label: "gcloud 配置名称（可选）", Required: false, Placeholder: "default"},
			},
		},
		{
			Name:         "Azure DNS",
			Code:         "dns_azure",
			Helper:       "Azure DNS；必填订阅 ID，支持服务主体模式或 Bearer Token",
			Fields: []ProviderFieldDef{
				{Key: "AZUREDNS_SUBSCRIPTIONID", Label: "Subscription ID", Required: true, Secret: false},
				{Key: "AZUREDNS_TENANTID", Label: "Tenant ID", Required: false, Secret: false},
				{Key: "AZUREDNS_APPID", Label: "App ID", Required: false, Secret: false},
				{Key: "AZUREDNS_CLIENTSECRET", Label: "Client Secret", Required: false, Secret: true},
				{Key: "AZUREDNS_BEARERTOKEN", Label: "Bearer Token（可选）", Required: false, Secret: true},
			},
		},
		{
			Name:         "Oracle Cloud Infrastructure DNS",
			Code:         "dns_oci",
			Helper:       "OCI DNS；支持租户、用户、区域与 API 密钥",
			Fields: []ProviderFieldDef{
				{Key: "OCI_CLI_TENANCY", Label: "Tenancy OCID（可选）", Required: false, Secret: false},
				{Key: "OCI_CLI_USER", Label: "User OCID（可选）", Required: false, Secret: false},
				{Key: "OCI_CLI_REGION", Label: "Region（可选）", Required: false, Placeholder: "us-ashburn-1"},
				{Key: "OCI_CLI_KEY_FILE", Label: "API Signing Key 文件路径（可选）", Required: false, Secret: false},
				{Key: "OCI_CLI_KEY", Label: "API Signing Key PEM（可选）", Required: false, Secret: true},
			},
		},
		{
			Name:         "NS1",
			Code:         "dns_nsone",
			Helper:       "NS1 DNS API",
			Fields: []ProviderFieldDef{
				{Key: "NS1_Key", Label: "API Key", Required: true, Secret: true},
			},
		},
		{
			Name:         "Akamai Connected Cloud（Linode）",
			Code:         "dns_linode_v4",
			Helper:       "Linode DNS API v4",
			Fields: []ProviderFieldDef{
				{Key: "LINODE_V4_API_KEY", Label: "API Key", Required: true, Secret: true},
			},
		},
		{
			Name:         "DigitalOcean",
			Code:         "dns_dgon",
			Helper:       "DigitalOcean DNS API",
			Fields: []ProviderFieldDef{
				{Key: "DO_API_KEY", Label: "API Key", Required: true, Secret: true},
			},
		},
		{
			Name:         "Vultr",
			Code:         "dns_vultr",
			Helper:       "Vultr DNS API",
			Fields: []ProviderFieldDef{
				{Key: "VULTR_API_KEY", Label: "API Key", Required: true, Secret: true},
			},
		},
		{
			Name:         "Namecheap",
			Code:         "dns_namecheap",
			Helper:       "Namecheap DNS API；需要开启 Namecheap API 权限",
			Fields: []ProviderFieldDef{
				{Key: "NAMECHEAP_API_KEY", Label: "API Key", Required: true, Secret: true},
				{Key: "NAMECHEAP_USERNAME", Label: "Username", Required: true, Secret: false},
				{Key: "NAMECHEAP_SOURCEIP", Label: "Source IP（可选）", Required: false},
			},
		},
		{
			Name:         "Gandi LiveDNS",
			Code:         "dns_gandi_livedns",
			Helper:       "Gandi LiveDNS；优先使用 Personal Access Token",
			Fields: []ProviderFieldDef{
				{Key: "GANDI_LIVEDNS_TOKEN", Label: "Personal Access Token（推荐）", Required: false, Secret: true},
				{Key: "GANDI_LIVEDNS_KEY", Label: "旧版 API Key（可选）", Required: false, Secret: true},
			},
		},
		{
			Name:              "Porkbun",
			Code:              "dns_porkbun",
			Helper:            "Porkbun DNS API v3",
			SupportGeneralDNS: true,
			Fields: []ProviderFieldDef{
				{Key: "PORKBUN_API_KEY", Label: "API Key", Required: true, Secret: false},
				{Key: "PORKBUN_SECRET_API_KEY", Label: "Secret API Key", Required: true, Secret: true},
			},
		},
		{
			Name:         "Name.com",
			Code:         "dns_namecom",
			Helper:       "Name.com DNS API",
			Fields: []ProviderFieldDef{
				{Key: "Namecom_Username", Label: "Username", Required: true, Secret: false},
				{Key: "Namecom_Token", Label: "API Token", Required: true, Secret: true},
			},
		},
		{
			Name:         "Njalla",
			Code:         "dns_njalla",
			Helper:       "Njalla DNS API",
			Fields: []ProviderFieldDef{
				{Key: "NJALLA_Token", Label: "API Token", Required: true, Secret: true},
			},
		},
		{
			Name:         "ClouDNS",
			Code:         "dns_cloudns",
			Helper:       "ClouDNS API；AUTH_ID 与 SUB_AUTH_ID 至少填写一个",
			Fields: []ProviderFieldDef{
				{Key: "CLOUDNS_AUTH_ID", Label: "Auth ID（可选）", Required: false, Secret: false},
				{Key: "CLOUDNS_SUB_AUTH_ID", Label: "Sub Auth ID（可选）", Required: false, Secret: false},
				{Key: "CLOUDNS_AUTH_PASSWORD", Label: "Auth Password", Required: true, Secret: true},
			},
		},
		{
			Name:         "Hurricane Electric DNS",
			Code:         "dns_he",
			Helper:       "Hurricane Electric DNS；使用 dns.he.net 账号密码或 DDNS 密钥管理记录",
			Fields: []ProviderFieldDef{
				{Key: "HE_Username", Label: "Username / DDNS 域名", Required: true, Secret: false},
				{Key: "HE_Password", Label: "Password / DDNS Key", Required: true, Secret: true},
			},
		},
		{
			Name:         "DNS Made Easy",
			Code:         "dns_me",
			Helper:       "DNS Made Easy API",
			Fields: []ProviderFieldDef{
				{Key: "ME_Key", Label: "API Key", Required: true, Secret: false},
				{Key: "ME_Secret", Label: "API Secret", Required: true, Secret: true},
			},
		},
		{
			Name:         "Constellix",
			Code:         "dns_constellix",
			Helper:       "Constellix DNS API",
			Fields: []ProviderFieldDef{
				{Key: "CONSTELLIX_Key", Label: "API Key", Required: true, Secret: false},
				{Key: "CONSTELLIX_Secret", Label: "API Secret", Required: true, Secret: true},
			},
		},
		{
			Name:         "FreeDNS（afraid.org）",
			Code:         "dns_freedns",
			Helper:       "FreeDNS 账号密码或动态更新 Token",
			Fields: []ProviderFieldDef{
				{Key: "FREEDNS_User", Label: "Username / Token", Required: true, Secret: false},
				{Key: "FREEDNS_Password", Label: "Password（可选）", Required: false, Secret: true},
			},
		},
		{
			Name:         "ZoneEdit",
			Code:         "dns_zoneedit",
			Helper:       "ZoneEdit DNS API",
			Fields: []ProviderFieldDef{
				{Key: "ZONEEDIT_ID", Label: "ID", Required: true, Secret: false},
				{Key: "ZONEEDIT_Token", Label: "API Token", Required: true, Secret: true},
			},
		},
		{
			Name:         "Rage4",
			Code:         "dns_rage4",
			Helper:       "Rage4 DNS API",
			Fields: []ProviderFieldDef{
				{Key: "RAGE4_USERNAME", Label: "Username", Required: true, Secret: false},
				{Key: "RAGE4_TOKEN", Label: "API Token", Required: true, Secret: true},
			},
		},
		{
			Name:         "Yandex Cloud DNS",
			Code:         "dns_yc",
			Helper:       "Yandex Cloud DNS API",
			Fields: []ProviderFieldDef{
				{Key: "YC_Zone_ID", Label: "DNS Zone ID（可选）", Required: false, Secret: false},
				{Key: "YC_Folder_ID", Label: "Folder ID（可选）", Required: false, Secret: false},
				{Key: "YC_SA_ID", Label: "Service Account ID", Required: false, Secret: false},
				{Key: "YC_SA_Key_ID", Label: "Service Account IAM Key ID", Required: false, Secret: false},
			},
		},
		{
			Name:         "DuckDNS",
			Code:         "dns_duckdns",
			Helper:       "DuckDNS 官方免费动态域名服务",
			Fields: []ProviderFieldDef{
				{Key: "DuckDNS_Token", Label: "API Token", Required: true, Secret: true, Placeholder: "DuckDNS 个人 Token"},
			},
		},
		{
			Name:         "Dynu",
			Code:         "dns_dynu",
			Helper:       "Dynu DNS；支持 Client ID / Secret 或 DDNS 密码模式",
			Fields: []ProviderFieldDef{
				{Key: "Dynu_ClientId", Label: "Client ID / 用户名", Required: true, Secret: false},
				{Key: "Dynu_Secret", Label: "Client Secret / 密码", Required: true, Secret: true},
			},
		},
		{
			Name:         "火山引擎 DNS",
			Code:         "dns_volcengine",
			Helper:       "火山引擎 (Volcano Engine) 云解析 DNS",
			Fields: []ProviderFieldDef{
				{Key: "Volcengine_ACCESS_KEY_ID", Label: "Access Key ID", Required: true, Secret: false},
				{Key: "Volcengine_SECRET_ACCESS_KEY", Label: "Secret Access Key", Required: true, Secret: true},
				{Key: "Volcengine_SESSION_TOKEN", Label: "Session Token（可选）", Required: false, Secret: true},
			},
		},
		{
			Name:         "百度智能云 DNS",
			Code:         "dns_baidu",
			Helper:       "百度智能云 DNS API",
			Fields: []ProviderFieldDef{
				{Key: "Baidu_AK", Label: "AccessKeyId", Required: true, Secret: false},
				{Key: "Baidu_SK", Label: "SecretAccessKey", Required: true, Secret: true},
				{Key: "Baidu_API_Preference", Label: "API Preference（可选）", Required: false, Placeholder: "auto"},
			},
		},
		{
			Name:         "西部数码 West.cn",
			Code:         "dns_west_cn",
			Helper:       "西部数码 DNS API",
			Fields: []ProviderFieldDef{
				{Key: "WEST_Username", Label: "API Username", Required: true, Secret: false},
				{Key: "WEST_Key", Label: "API Key", Required: true, Secret: true},
			},
		},
		{
			Name:         "自定义 Webhook",
			Code:         "webhook",
			Helper:       "通过 HTTP 请求向自定义服务同步 IP，URL 或 Body 支持 #{ip}、#{domain}、#{type}、#{ttl} 变量替换",
			Fields: []ProviderFieldDef{
				{Key: "Webhook_URL", Label: "请求 URL", Required: true, Secret: false, Placeholder: "https://example.com/api/ddns?ip=#{ip}&domain=#{domain}"},
				{Key: "Webhook_Method", Label: "请求方法", Required: false, Secret: false, Placeholder: "GET 或 POST（默认 GET）"},
				{Key: "Webhook_Headers", Label: "自定义请求头 (KEY=VALUE 一行一个)", Required: false, Secret: false, Placeholder: "Authorization=Bearer xxx"},
				{Key: "Webhook_Body", Label: "请求体 (POST 模式)", Required: false, Secret: false, Placeholder: `{"ip":"#{ip}","domain":"#{domain}","type":"#{type}"}`},
			},
		},
	}
	for i := range catalog {
		catalog[i].SupportGeneralDNS = true
	}
	return catalog
}

// SplitDomain splits full domain like "sub.example.com" into subDomain "sub" and rootDomain "example.com".
func SplitDomain(fullDomain string) (subDomain string, rootDomain string, err error) {
	fullDomain = strings.TrimSpace(strings.TrimSuffix(fullDomain, "."))
	parts := strings.Split(fullDomain, ".")
	if len(parts) < 2 {
		return "", "", errors.New("invalid domain name: " + fullDomain)
	}
	// Common multi-part TLDs (e.g., .com.cn, .net.cn, .org.cn, .co.uk)
	if len(parts) >= 3 {
		penultimate := parts[len(parts)-2]
		last := parts[len(parts)-1]
		if (last == "cn" || last == "uk" || last == "jp" || last == "hk" || last == "tw") &&
			(penultimate == "com" || penultimate == "net" || penultimate == "org" || penultimate == "gov" || penultimate == "edu" || penultimate == "co") {
			rootDomain = strings.Join(parts[len(parts)-3:], ".")
			subParts := parts[:len(parts)-3]
			if len(subParts) == 0 {
				subDomain = "@"
			} else {
				subDomain = strings.Join(subParts, ".")
			}
			return subDomain, rootDomain, nil
		}
	}
	rootDomain = strings.Join(parts[len(parts)-2:], ".")
	subParts := parts[:len(parts)-2]
	if len(subParts) == 0 {
		subDomain = "@"
	} else {
		subDomain = strings.Join(subParts, ".")
	}
	return subDomain, rootDomain, nil
}
