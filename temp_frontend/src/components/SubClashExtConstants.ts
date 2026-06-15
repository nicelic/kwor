// SubClashExt 组件的静态默认配置和选项数据（Clash/Mihomo 格式）

export const clashLogLevels = ["debug", "info", "warning", "error", "silent"]

export const tunStackOptions = ["system", "gvisor", "mixed"]

export const enhancedModeOptions = [
  { title: "fake-ip", value: "fake-ip" },
  { title: "redir-host", value: "redir-host" },
]

// Clash rule set sources.
export const clashRuleSetSourceOptions = [
  { title: "MetaCubeX Github", value: "metacubex_github" },
  { title: "MetaCubeX CDN", value: "metacubex_cdn" },
  { title: "QuixoticHeart Github", value: "quixoticheart_github" },
  { title: "Loyalsoldier Github", value: "loyalsoldier_github" },
  { title: "Loyalsoldier_IP Github", value: "loyalsoldier_ip_github" },
  { title: "Loyalsoldier_IP CDN", value: "loyalsoldier_ip_cdn" },
  { title: "自定义规则集完整链接", value: "" },
]

// Clash rule set URL templates.
export const CLASH_RULE_SET_URL_TEMPLATES: Record<string, { geosite: string; geoip: string }> = {
  metacubex_github: {
    geosite: 'https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geosite/{name}.mrs',
    geoip: 'https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/meta/geo/geoip/{name}.mrs',
  },
  metacubex_cdn: {
    geosite: 'https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/{name}.mrs',
    geoip: 'https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geoip/{name}.mrs',
  },
  quixoticheart_github: {
    geosite: 'https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/meta/domain/{name}.mrs',
    geoip: 'https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/meta/ipcidr/{name}.mrs',
  },
  loyalsoldier_github: {
    geosite: 'https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/{name}.txt',
    geoip: 'https://raw.githubusercontent.com/Loyalsoldier/clash-rules/release/{name}.txt',
  },
  loyalsoldier_ip_github: {
    geosite: 'https://raw.githubusercontent.com/Loyalsoldier/geoip/release/clash/ipcidr/{name}.txt',
    geoip: 'https://raw.githubusercontent.com/Loyalsoldier/geoip/release/clash/ipcidr/{name}.txt',
  },
  loyalsoldier_ip_cdn: {
    geosite: 'https://fastly.jsdelivr.net/gh/Loyalsoldier/geoip@release/clash/ipcidr/{name}.txt',
    geoip: 'https://fastly.jsdelivr.net/gh/Loyalsoldier/geoip@release/clash/ipcidr/{name}.txt',
  },
}

// MetaCubeX 名称映射（某些名称在 MetaCubeX 中有特殊映射）
export const CLASH_METACUBEX_NAME_MAP: Record<string, string> = {
  'ads': 'category-ads-all',
  'ir': 'category-ir',
}

// 需要名称映射的来源列表
export const CLASH_SOURCES_NEED_NAME_MAP = [
  'metacubex_github',
  'metacubex_cdn',
]

// Clash 自定义规则类型映射（sing-box 类型 → Clash 类型）
export const clashDomainIpTypes = [
  { title: "域名 (DOMAIN)", value: "DOMAIN" },
  { title: "域名后缀 (DOMAIN-SUFFIX)", value: "DOMAIN-SUFFIX" },
  { title: "域名关键词 (DOMAIN-KEYWORD)", value: "DOMAIN-KEYWORD" },
  { title: "域名通配 (DOMAIN-WILDCARD)", value: "DOMAIN-WILDCARD" },
  { title: "域名正则 (DOMAIN-REGEX)", value: "DOMAIN-REGEX" },
  { title: "IP CIDR (IP-CIDR)", value: "IP-CIDR" },
  { title: "IP CIDR6 (IP-CIDR6)", value: "IP-CIDR6" },
  { title: "IP 后缀 (IP-SUFFIX)", value: "IP-SUFFIX" },
  { title: "IP ASN (IP-ASN)", value: "IP-ASN" },
  { title: "GEOIP", value: "GEOIP" },
]

// Clash 出站映射
export const clashOutboundMap: Record<string, string> = {
  'block': 'REJECT',
  'direct': 'DIRECT',
  'proxy': 'Proxy',
}

// 域名规则集名称选项（不含前缀）
export const clashGeositeNameOptions = [
  "geolocation-!cn", "gfw", "private", "cn", "ir", "vn", "ads",
  "google", "facebook", "twitter", "youtube", "telegram",
  "netflix", "amazon", "apple", "microsoft", "github",
  "tiktok", "spotify", "whatsapp", "instagram", "discord",
  "openai", "bing", "cloudflare", "steam", "paypal",
]

// IP 规则集名称选项（不含前缀）
export const clashGeoipNameOptions = [
  "private", "cn", "ir", "vn",
  "google", "facebook", "twitter", "telegram",
  "netflix", "amazon", "apple", "cloudflare",
]

export const clashLoyalsoldierDomainNameOptions = [
  "reject",
  "icloud",
  "apple",
  "google",
  "proxy",
  "direct",
  "private",
  "gfw",
  "tld-not-cn",
  "applications",
]

export const clashLoyalsoldierIpNameOptions = [
  "telegramcidr",
  "cncidr",
  "lancidr",
]

export const CLASH_RULE_SET_NAME_OPTIONS_BY_SOURCE: Record<string, { domain: string[]; ip: string[] }> = {
  metacubex_github: {
    domain: clashGeositeNameOptions,
    ip: clashGeoipNameOptions,
  },
  metacubex_cdn: {
    domain: clashGeositeNameOptions,
    ip: clashGeoipNameOptions,
  },
  quixoticheart_github: {
    domain: clashGeositeNameOptions,
    ip: clashGeoipNameOptions,
  },
  loyalsoldier_github: {
    domain: clashLoyalsoldierDomainNameOptions,
    ip: clashLoyalsoldierIpNameOptions,
  },
  loyalsoldier_ip_github: {
    domain: clashGeositeNameOptions,
    ip: clashGeoipNameOptions,
  },
  loyalsoldier_ip_cdn: {
    domain: clashGeositeNameOptions,
    ip: clashGeoipNameOptions,
  },
}

// 更新方式选项
export const clashUpdateMethodOptions = [
  { title: "节点选择", value: "节点选择" },
  { title: "自动选择", value: "自动选择" },
  { title: "全球直连", value: "全球直连" },
  { title: "全球拦截", value: "全球拦截" },
  { title: "漏网之鱼", value: "漏网之鱼" },
]

// 延迟测试 URL 选项
export const clashLatencyTestUrlOptions = [
  "http://cp.cloudflare.com/generate_204",
  "https://cp.cloudflare.com/generate_204",
  "http://connectivitycheck.gstatic.com/generate_204",
  "https://connectivitycheck.gstatic.com/generate_204",
  "http://www.gstatic.com/generate_204",
  "https://www.gstatic.com/generate_204",
  "http://captive.apple.com/generate_204",
  "https://captive.apple.com/generate_204",
  "http://1.1.1.1/generate_204",
  "https://1.1.1.1/generate_204",
  "http://www.google.com/generate_204",
  "https://www.google.com/generate_204",
]

// 路由最终出站选项（Clash 格式）
export const clashRouteFinalOptions = [
  { title: "节点选择", value: "节点选择" },
  { title: "自动选择", value: "自动选择" },
  { title: "全球直连", value: "全球直连" },
  { title: "全球拦截", value: "全球拦截" },
  { title: "漏网之鱼", value: "漏网之鱼" },
]

// DNS 预设选项
const clashNodeSelectorSuffix = '#\u8282\u70b9\u9009\u62e9'

export const clashNameserverOptions = [
  `udp://8.8.8.8${clashNodeSelectorSuffix}`,
  `tcp://8.8.8.8${clashNodeSelectorSuffix}`,
  `udp://1.1.1.1${clashNodeSelectorSuffix}`,
  `tcp://1.1.1.1${clashNodeSelectorSuffix}`,
  `udp://8.8.4.4${clashNodeSelectorSuffix}`,
  `tcp://8.8.4.4${clashNodeSelectorSuffix}`,
  `udp://1.0.0.1${clashNodeSelectorSuffix}`,
  `tcp://1.0.0.1${clashNodeSelectorSuffix}`,
  "tls://1.1.1.1:853",
  "tls://8.8.8.8:853",
  "https://1.1.1.1/dns-query",
  "https://8.8.8.8/dns-query",
  "https://dns.google/dns-query",
  "https://cloudflare-dns.com/dns-query",
  "quic://dns.adguard.com:853",
]

export const clashFallbackOptions = [
  `udp://8.8.8.8${clashNodeSelectorSuffix}`,
  `tcp://8.8.8.8${clashNodeSelectorSuffix}`,
  `udp://1.1.1.1${clashNodeSelectorSuffix}`,
  `tcp://1.1.1.1${clashNodeSelectorSuffix}`,
  `udp://8.8.4.4${clashNodeSelectorSuffix}`,
  `tcp://8.8.4.4${clashNodeSelectorSuffix}`,
  `udp://1.0.0.1${clashNodeSelectorSuffix}`,
  `tcp://1.0.0.1${clashNodeSelectorSuffix}`,
  "tls://223.5.5.5:853",
  "tls://223.6.6.6:853",
  "https://doh.pub/dns-query",
  "https://dns.alidns.com/dns-query",
  "tcp://223.5.5.5:53",
  "tcp://119.29.29.29:53",
]

export const clashDefaultNameserverOptions = [
  "udp://223.5.5.5",
  "udp://223.6.6.6",
  "223.5.5.5",
  "223.6.6.6",
  "udp://8.8.8.8",
  "udp://1.1.1.1",
  "8.8.8.8",
  "1.1.1.1",
  "114.114.114.114",
  "119.29.29.29",
]

export const clashDirectNameserverOptions = Array.from(new Set([
  "tls://223.5.5.5",
  "quic://223.5.5.5",
  "https://dns.alidns.com/dns-query",
  "system",
  ...clashFallbackOptions,
  ...clashNameserverOptions,
  ...clashDefaultNameserverOptions,
]))

export const clashProxyServerNameserverOptions = Array.from(new Set([
  "udp://223.5.5.5",
  "udp://223.6.6.6",
  "udp://1.1.1.1",
  "udp://8.8.8.8",
  "udp://8.8.4.4",
  "udp://1.0.0.1",
  "udp://119.29.29.29",
  ...clashFallbackOptions,
  ...clashNameserverOptions,
]))

export const clashFakeIpFilterDefaults = [
  "*.lan",
  "localhost",
  "*.local",
  "*.localhost",
  "+.stun.*.*",
  "+.stun.*.*.*",
  "+.stun.*.*.*.*",
  "*.mcdn.bilivideo.cn",
]

// mihomo find-process-mode 选项
export const findProcessModeOptions = [
  { title: "关闭 (off)", value: "off" },
  { title: "总是 (always)", value: "always" },
  { title: "严格 (strict)", value: "strict" },
]

// 默认配置（mihomo/MetaCubeX 格式）
export const defaultTunInet4Address = "198.18.0.1/30"
export const defaultTunInet6Address = "fdfe:dcba:9876::1/126"
export const defaultFakeIpRange = "198.18.0.1/15"
export const defaultFakeIpRange6 = "fc00::/18"

export const defaultClashConfig: Record<string, any> = {
  "mixed-port": 7890,
  "allow-lan": false,
  "mode": "rule",
  "log-level": "info",
  "external-controller": "127.0.0.1:9090",
  "unified-delay": true,
  "tcp-concurrent": true,
  "find-process-mode": "strict",
  "profile": {
    "store-selected": true,
    "store-fake-ip": true,
  },
  "tun": {
    "enable": true,
    "stack": "mixed",
    "auto-route": true,
    "strict-route": true,
    "auto-detect-interface": true,
    "recvmsgx": true,
    "sendmsgx": false,
    "inet4-address": [defaultTunInet4Address],
    "inet6-address": [defaultTunInet6Address],
    "dns-hijack": ["any:53"],
    "mtu": 1500,
  },
  "sniffer": {
    "enable": true,
    "force-dns-mapping": true,
    "parse-pure-ip": true,
    "override-destination": false,
    "sniff": {
      "HTTP": { "ports": ["1-65535"] },
      "TLS": { "ports": ["1-65535"] },
      "QUIC": { "ports": ["1-65535"] },
    },
  },
  "dns": {
    "enable": true,
    "ipv6": false,
    "prefer-h3": false,
    "use-system-hosts": true,
    "use-hosts": true,
    "enhanced-mode": "fake-ip",
    "fake-ip-range": defaultFakeIpRange,
    "fake-ip-range6": defaultFakeIpRange6,
    "default-nameserver": ["udp://223.5.5.5", "udp://223.6.6.6"],
    "nameserver": [
      `udp://8.8.8.8${clashNodeSelectorSuffix}`,
      `tcp://8.8.8.8${clashNodeSelectorSuffix}`,
    ],
    "fallback": [
      `udp://8.8.4.4${clashNodeSelectorSuffix}`,
      `tcp://8.8.4.4${clashNodeSelectorSuffix}`,
    ],
    "direct-nameserver": [
      "tls://223.5.5.5",
      "quic://223.5.5.5",
      "https://dns.alidns.com/dns-query",
    ],
    "proxy-server-nameserver": [
      "udp://223.5.5.5",
      "udp://223.6.6.6",
    ],
    "fallback-filter": {
      "geoip": true,
      "geoip-code": "CN",
    },
    "fake-ip-filter": [
      "*.lan",
      "localhost",
      "*.local",
    ],
  },
  "rules": [
    "GEOIP,Private,DIRECT",
    "MATCH,节点选择",
  ],
}

// GeoIP 规则选项（GEOIP 格式，用于旧式规则）
export const clashGeoipRulesOptions = [
  { title: 'Private-Direct', value: 'GEOIP,Private,DIRECT' },
  { title: 'Private-Block', value: 'GEOIP,Private,REJECT' },
  { title: 'LAN-Direct', value: 'GEOIP,LAN,DIRECT' },
  { title: 'LAN-Block', value: 'GEOIP,LAN,REJECT' },
  { title: '🇨🇳 China-Direct', value: 'GEOIP,CN,DIRECT' },
  { title: '🇨🇳 China-Block', value: 'GEOIP,CN,REJECT' },
  { title: '🇮🇷 Iran-Direct', value: 'GEOIP,CATEGORY-IR,DIRECT' },
  { title: '🇮🇷 Iran-Block', value: 'GEOIP,CATEGORY-IR,REJECT' },
  { title: '🇻🇳 Vietnam-Direct', value: 'GEOIP,CATEGORY-VN,DIRECT' },
  { title: '🇻🇳 Vietnam-Block', value: 'GEOIP,CATEGORY-VN,REJECT' },
]
