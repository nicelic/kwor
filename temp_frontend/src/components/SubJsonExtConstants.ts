// Comment cleaned to avoid mojibake.

export const levels = ["trace", "debug", "info", "warn", "error", "fatal", "panic"]

export const tunIpOptions = ["198.18.0.0/15", "fc00::/18"]

export const dnsStrategyOptions = ["prefer_ipv4", "prefer_ipv6", "ipv4_only", "ipv6_only"]

export const tlsStoreOptions = [
  { title: "System", value: "system" },
  { title: "Mozilla", value: "mozilla" },
  { title: "Chrome", value: "chrome" },
  { title: "None", value: "none" },
]

export const ruleSetSourceOptions = [
  { title: "KaringX GitHub", value: "karingx_github" },
  { title: "KaringX CDN", value: "karingx_cdn" },
  { title: "Loyalsoldier_IP Github", value: "loyalsoldier_ip_github" },
  { title: "Loyalsoldier_IP CDN", value: "loyalsoldier_ip_cdn" },
  { title: "QuixoticHeart Github", value: "quixoticheart_github" },
  { title: "SagerNet Github", value: "sagernet_github" },
  { title: "SagerNet CDN", value: "sagernet_cdn" },
  { title: "MetaCubeX Github", value: "metacubex_github" },
  { title: "MetaCubeX CDN", value: "metacubex_cdn" },
  { title: "Chocolate4U Github", value: "chocolate4u_github" },
  { title: "Chocolate4U CDN", value: "chocolate4u_cdn" },
  { title: "lyc8503 Github", value: "lyc8503_github" },
  { title: "lyc8503 CDN", value: "lyc8503_cdn" },
  { title: "lyc8503 CDN 1", value: "lyc8503_cdn1" },
  { title: "自定义规则集完整链接", value: "" },
]

export const domainIpTypes = [
  { title: "域名 (domain)", value: "domain" },
  { title: "域名后缀 (domain_suffix)", value: "domain_suffix" },
  { title: "域名关键词 (domain_keyword)", value: "domain_keyword" },
  { title: "域名正则 (domain_regex)", value: "domain_regex" },
  { title: "IP CIDR (ip_cidr)", value: "ip_cidr" },
  { title: "私有IP (ip_is_private)", value: "ip_is_private" },
]

// Comment cleaned to avoid mojibake.
export const geositeNameOptions = [
  "geolocation-!cn", "gfw", "private", "cn", "ir", "vn", "ads",
  "google", "facebook", "twitter", "youtube", "telegram",
  "netflix", "amazon", "apple", "microsoft", "github",
  "tiktok", "spotify", "whatsapp", "instagram", "discord",
  "openai", "bing", "cloudflare", "steam", "paypal",
]

// Comment cleaned to avoid mojibake.
export const geoipNameOptions = [
  "private", "cn", "ir", "vn",
  "google", "facebook", "twitter", "telegram",
  "netflix", "amazon", "apple", "cloudflare",
]

export const ruleSetOptions = [
  { title: "Site-Geolocation-!CN", value: "geosite-geolocation-!cn" },
  { title: "Site-GFW", value: "geosite-gfw" },
  { title: "Site-Private", value: "geosite-private" },
  { title: "IP-Private", value: "geoip-private" },
  { title: "Site-Ads", value: "geosite-ads" },
  { title: "🇮🇷 Site-Iran", value: "geosite-ir" },
  { title: "🇮🇷 IP-Iran", value: "geoip-ir" },
  { title: "🇨🇳 Site-China", value: "geosite-cn" },
  { title: "🇨🇳 IP-China", value: "geoip-cn" },
  { title: "🇻🇳 Site-Vietnam", value: "geosite-vn" },
  { title: "🇻🇳 IP-Vietnam", value: "geoip-vn" },
]

export const updateMethodOptions = [
  { title: "节点选择", value: "节点选择" },
  { title: "自动选择", value: "自动选择" },
  { title: "全球直连", value: "全球直连" },
  { title: "全球拦截", value: "全球拦截" },
  { title: "漏网之鱼", value: "漏网之鱼" },
]

export const latencyTestUrlOptions = [
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
  "http://edge.microsoft.com/generate_204",
  "https://edge.microsoft.com/generate_204",
  "http://www.msftconnecttest.com/connecttest.txt",
  "https://www.msftconnecttest.com/connecttest.txt",
]

export const geositeList = [
  { title: "Geolocation-!CN", value: "geosite-geolocation-!cn" },
  { title: "GFW", value: "geosite-gfw" },
  { title: "Private", value: "geosite-private" },
  { title: "Ads", value: "geosite-ads" },
  { title: "🇮🇷 Iran", value: "geosite-ir" },
  { title: "🇨🇳 China", value: "geosite-cn" },
  { title: "🇻🇳 Vietnam", value: "geosite-vn" },
]

export const geoList = [
  { title: "Site-Private", value: "geosite-private" },
  { title: "IP-Private", value: "geoip-private" },
  { title: "Site-Ads", value: "geosite-ads" },
  { title: "🇮🇷 Site-Iran", value: "geosite-ir" },
  { title: "🇮🇷 IP-Iran", value: "geoip-ir" },
  { title: "🇨🇳 Site-China", value: "geosite-cn" },
  { title: "🇨🇳 IP-China", value: "geoip-cn" },
  { title: "🇻🇳 Site-Vietnam", value: "geosite-vn" },
  { title: "🇻🇳 IP-Vietnam", value: "geoip-vn" },
]

export const geo = [
  { tag: "geosite-ads", type: "remote", format: "binary", url: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/category-ads-all.srs", download_detour: "direct" },
  { tag: "geosite-private", type: "remote", format: "binary", url: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/private.srs", download_detour: "direct" },
  { tag: "geosite-ir", type: "remote", format: "binary", url: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/category-ir.srs", download_detour: "direct" },
  { tag: "geosite-cn", type: "remote", format: "binary", url: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/cn.srs", download_detour: "direct" },
  { tag: "geosite-vn", type: "remote", format: "binary", url: "https://github.com/Thaomtam/Geosite-vn/raw/rule-set/Geosite-vn.srs", download_detour: "direct" },
  { tag: "geoip-private", type: "remote", format: "binary", url: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/private.srs", download_detour: "direct" },
  { tag: "geoip-ir", type: "remote", format: "binary", url: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/ir.srs", download_detour: "direct" },
  { tag: "geoip-cn", type: "remote", format: "binary", url: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/cn.srs", download_detour: "direct" },
  { tag: "geoip-vn", type: "remote", format: "binary", url: "https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/vn.srs", download_detour: "direct" },
]

// Comment cleaned to avoid mojibake.
// Comment cleaned to avoid mojibake.
// Comment cleaned to avoid mojibake.

export const RULE_SET_URL_TEMPLATES: Record<string, { geosite: string; geoip: string }> = {
  // SagerNet Github
  sagernet_github: {
    geosite: 'https://github.com/SagerNet/sing-geosite/raw/rule-set/geosite-{name}.srs',
    geoip: 'https://github.com/SagerNet/sing-geoip/raw/rule-set/geoip-{name}.srs',
  },
  // SagerNet CDN (fastly.jsdelivr)
  sagernet_cdn: {
    geosite: 'https://fastly.jsdelivr.net/gh/SagerNet/sing-geosite@rule-set/geosite-{name}.srs',
    geoip: 'https://fastly.jsdelivr.net/gh/SagerNet/sing-geoip@rule-set/geoip-{name}.srs',
  },
  // KaringX GitHub
  karingx_github: {
    geosite: 'https://github.com/KaringX/karing-ruleset/raw/refs/heads/sing/geo/geosite/{name}.srs',
    geoip: 'https://github.com/KaringX/karing-ruleset/raw/refs/heads/sing/geo/geoip/{name}.srs',
  },
  // KaringX CDN (fastly.jsdelivr)
  karingx_cdn: {
    geosite: 'https://fastly.jsdelivr.net/gh/KaringX/karing-ruleset@sing/geo/geosite/{name}.srs',
    geoip: 'https://fastly.jsdelivr.net/gh/KaringX/karing-ruleset@sing/geo/geoip/{name}.srs',
  },
  // Loyalsoldier_IP Github
  loyalsoldier_ip_github: {
    geosite: 'https://raw.githubusercontent.com/Loyalsoldier/geoip/release/srs/{name}.srs',
    geoip: 'https://raw.githubusercontent.com/Loyalsoldier/geoip/release/srs/{name}.srs',
  },
  // Loyalsoldier_IP CDN (fastly.jsdelivr)
  loyalsoldier_ip_cdn: {
    geosite: 'https://fastly.jsdelivr.net/gh/Loyalsoldier/geoip@release/srs/{name}.srs',
    geoip: 'https://fastly.jsdelivr.net/gh/Loyalsoldier/geoip@release/srs/{name}.srs',
  },
  // QuixoticHeart Github (singbox version4)
  quixoticheart_github: {
    geosite: 'https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/singbox/version4/{name}.srs',
    geoip: 'https://github.com/QuixoticHeart/rule-set/raw/refs/heads/ruleset/singbox/version4/{name}.srs',
  },
  // MetaCubeX Github
  metacubex_github: {
    geosite: 'https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geosite/{name}.srs',
    geoip: 'https://raw.githubusercontent.com/MetaCubeX/meta-rules-dat/sing/geo/geoip/{name}.srs',
  },
  // MetaCubeX CDN (testingcf.jsdelivr)
  metacubex_cdn: {
    geosite: 'https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geosite/{name}.srs',
    geoip: 'https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@sing/geo/geoip/{name}.srs',
  },
  // Chocolate4U Github
  chocolate4u_github: {
    geosite: 'https://raw.githubusercontent.com/Chocolate4U/Iran-sing-box-rules/rule-set/geosite-{name}.srs',
    geoip: 'https://raw.githubusercontent.com/Chocolate4U/Iran-sing-box-rules/rule-set/geoip-{name}.srs',
  },
  // Chocolate4U CDN (cdn.jsdelivr)
  chocolate4u_cdn: {
    geosite: 'https://cdn.jsdelivr.net/gh/Chocolate4U/Iran-sing-box-rules@rule-set/geosite-{name}.srs',
    geoip: 'https://cdn.jsdelivr.net/gh/Chocolate4U/Iran-sing-box-rules@rule-set/geoip-{name}.srs',
  },
  // lyc8503 Github
  lyc8503_github: {
    geosite: 'https://github.com/lyc8503/sing-box-rules/raw/refs/heads/rule-set-geosite/geosite-{name}.srs',
    geoip: 'https://github.com/lyc8503/sing-box-rules/raw/refs/heads/rule-set-geoip/geoip-{name}.srs',
  },
  // lyc8503 CDN (cdn.jsdelivr)
  lyc8503_cdn: {
    geosite: 'https://cdn.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geosite/geosite-{name}.srs',
    geoip: 'https://cdn.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geoip/geoip-{name}.srs',
  },
  // lyc8503 CDN 1 (fastly.jsdelivr)
  lyc8503_cdn1: {
    geosite: 'https://fastly.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geosite/geosite-{name}.srs',
    geoip: 'https://fastly.jsdelivr.net/gh/lyc8503/sing-box-rules@rule-set-geoip/geoip-{name}.srs',
  },
}

// MetaCubeX 名称映射（部分名称在 MetaCubeX 中需要特殊映射）
export const METACUBEX_NAME_MAP: Record<string, string> = {
  'ads': 'category-ads-all',
  'ir': 'category-ir',
}

// 需要名称映射的来源列表 (MetaCubeX 系列)
// Sources requiring name mapping (MetaCubeX family).
export const SOURCES_NEED_NAME_MAP = ['metacubex_github', 'metacubex_cdn', 'karingx_github', 'karingx_cdn']

// ===== Rule Slot Mapping =====
// Slots 1-3: custom rules (block/direct/proxy).
// Slots 4-9: ruleset rules (block-domain/block-ip/proxy-domain/proxy-ip/direct-domain/direct-ip).
export const RULE_ORDER_CONFIG = [
  { id: 1, kind: 'custom', outbound: 'reject', label: 'Custom Block' },
  { id: 2, kind: 'custom', outbound: 'direct', label: 'Custom Direct' },
  { id: 3, kind: 'custom', outbound: 'proxy', label: 'Custom Proxy' },
  { id: 4, kind: 'ruleset', prefix: 'geosite', outbound: 'reject', label: 'Block Ruleset - Domain' },
  { id: 5, kind: 'ruleset', prefix: 'geoip', outbound: 'reject', label: 'Block Ruleset - IP' },
  { id: 6, kind: 'ruleset', prefix: 'geosite', outbound: 'proxy', label: 'Proxy Ruleset - Domain' },
  { id: 7, kind: 'ruleset', prefix: 'geoip', outbound: 'proxy', label: 'Proxy Ruleset - IP' },
  { id: 8, kind: 'ruleset', prefix: 'geosite', outbound: 'direct', label: 'Direct Ruleset - Domain' },
  { id: 9, kind: 'ruleset', prefix: 'geoip', outbound: 'direct', label: 'Direct Ruleset - IP' },
]

// ===== Default Config Objects =====
export const defaultLog = {
  level: "info",
  timestamp: true,
}

export const defaultTunInbound = {
  type: "tun",
  address: ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
  mtu: 1500,
  auto_route: true,
  strict_route: true,
  endpoint_independent_nat: false,
  stack: "mixed",
  exclude_package: [] as string[],
}

export const defaultInb = [
  {
    type: "tun",
    address: ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
    mtu: 1500,
    auto_route: true,
    strict_route: true,
    endpoint_independent_nat: false,
    stack: "mixed",
    exclude_package: [] as string[],
  },
  {
    type: "mixed",
    listen: "127.0.0.1",
    listen_port: 2080,
    users: [] as any[],
  },
]

export const defaultExp = {
  cache_file: {
    enabled: true,
    store_fakeip: true,
  },
  // clash_api: {
  //   default_mode: "rule",
  //   external_controller: "127.0.0.1:9090",
  //   external_ui: "ui",
  //   external_ui_download_detour: "direct",
  //   external_ui_download_url: "https://mirror.ghproxy.com/https://github.com/MetaCubeX/Yacd-meta/archive/gh-pages.zip",
  //   secret: "",
  // },
}

export const defaultSubClashApi = {
  external_controller: "127.0.0.1:20123",
  secret: "",
  default_mode: "rule",
  external_ui: "",
  external_ui_download_url: "",
  external_ui_download_detour: "全球直连",
  access_control_allow_origin: ["*"],
  access_control_allow_private_network: false,
}

export const clashApiModeOptions = [
  "rule",
  "direct",
  "global",
]

export const subSelectorTagOptions = [
  "节点选择",
  "自动选择",
  "全球直连",
  "全球拦截",
  "漏网之鱼",
  "GLOBAL",
  "direct",
  "block",
]

export const defaultDns = {
  final: "direct-dns",
  rules: [],
  servers: [
    {
      detour: "proxy",
      domain_resolver: "proxy-bootstrap-dns",
      server: "1.1.1.1",
      server_port: 53,
      tag: "proxy-dns",
      type: "udp",
    },
    {
      domain_resolver: "direct-bootstrap-dns",
      server: "223.5.5.5",
      server_port: 443,
      tag: "direct-dns",
      tls: {
        enabled: true,
        insecure: false,
        min_version: "1.3",
        server_name: "223.5.5.5",
      },
      type: "https",
    },
    { tag: "proxy-bootstrap-dns", type: "udp", server: "1.1.1.1", server_port: 53 },
    { tag: "direct-bootstrap-dns", type: "udp", server: "223.5.5.5", server_port: 53 },
    { inet4_range: "198.18.0.0/15", inet6_range: "fc00::/18", tag: "fakeip", type: "fakeip" },
  ],
  strategy: "prefer_ipv4",
}

