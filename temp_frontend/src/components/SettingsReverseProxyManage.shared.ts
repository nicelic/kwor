import HttpUtils, { type Msg } from '@/plugins/httputil'
import { confirm } from '@/plugins/confirm'
import { i18n } from '@/locales'
import type {
  ReverseProxyCertificateOption,
  ReverseProxyOverview,
  ReverseProxyResourceSettings,
  ReverseProxyRuntimeOverview,
  ReverseProxyRule,
  ReverseProxyRuleForm,
} from '@/types/reverseProxy'
import { push } from 'notivue'
import { formatPanelDateTime } from '@/plugins/panelTime'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

export const reverseProxyCopy = {
  heroEyebrow: 'GO REVERSE PROXY',
  title: '反向代理',
  subtitle: '由 Go 直接监听本地 HTTP / HTTPS / DNS，并按规则转发到对应的上游服务。',
  refresh: '立即刷新',
  newRule: '新建反代',
  available: '可用',
  unavailable: '不可用',
  unavailableHint: '当前环境可能无法完整运行反向代理监听，但你仍然可以先维护规则和证书绑定。',
  loadFailed: '反向代理配置加载失败',
  listeners: '监听器',
  connectionLabel: '连接数',
  connectionHint: '本地 | 目标',
  enabledRules: '启用规则',
  certificates: '证书',
  totalRules: '规则总数',
  lastSync: '最近同步',
  search: '搜索名称 / 域名 / 路径 / 目标',
  tableTitle: '规则列表',
  tableSubtitle: '列表顺序就是匹配顺序。同一监听口下会按从上到下的顺序严格匹配 Host 与路径。',
  empty: '当前没有可显示的反向代理规则',
  runtimeTitle: '运行态',
  runtimeStatus: '运行状态',
  running: '已启动',
  stopped: '未启动',
  reorderUp: '上移',
  reorderDown: '下移',
  edit: '编辑',
  delete: '删除',
  deleteConfirm: '确定删除反向代理规则 {name} 吗？',
  createTitle: '新建反向代理',
  editTitle: '编辑反向代理',
  dialogSubtitle: '左侧定义本地监听和命中条件，右侧定义被代理的上游地址与连接策略。DNS 与 HTTP 会按各自协议单独处理。',
  name: '名称',
  listenPanel: '本地监听',
  targetPanel: '目标连接',
  tlsPanel: 'HTTPS / TLS',
  listenProtocol: '本地协议',
  listenPort: '监听端口',
  hosts: '域名',
  hostsPlaceholder: 'ss.cc, *.ss.cc',
  pathPrefix: 'URL 路径（可选）',
  listenDnsPath: 'DNS URL 路径',
  targetProtocol: '目标协议',
  targetAddresses: '目标地址/域名',
  targetAddressesPlaceholder: '1.1.1.1, example.com, 2606:4700:4700::1111',
  compressionEnabled: '启用压缩算法请求头',
  compressionAlgorithms: '压缩算法',
  compressionHint: '仅控制 Accept-Encoding 请求头的生成，不改变后台支持的解码和编码能力。关闭后不生成此请求头。',
  compressionDisabled: '关闭',
  compressionZstd: 'zstd',
  compressionS2: 's2',
  compressionSnappy: 'snappy',
  compressionBr: 'br',
  compressionDeflate: 'deflate',
  compressionGzip: 'gzip',
  targetPort: '目标端口',
  targetPath: '目标基础路径',
  targetDnsPath: '目标 DNS URL 路径',
  dnsUpstreamTimeout: '上游超时（秒）',
  dnsUpstreamTimeoutHint: '指定等待上游服务器响应的秒数。',
  fallbackDnsUpstreams: '后备 DNS 服务器',
  fallbackDnsUpstreamsHint: '当主要上游没有响应或返回错误时使用。这里采用 AdGuardHome/dnsproxy 多行上游语法，不同于上方只接收目标地址的主要上游输入框；支持注释、域名规则以及 udp://、tcp://、tls://、https://、quic://、h3://、sdns://。',
  dnsCacheTitle: 'DNS 缓存配置',
  dnsCacheEnabled: '启用 DNS 缓存',
  dnsCacheSizeBytes: '缓存大小',
  dnsCacheSizeBytesHint: 'DNS 缓存大小（单位：字节）',
  dnsCacheMinTtl: '覆盖最小 TTL 值',
  dnsCacheMinTtlHint: '缓存 DNS 响应时，延长从上游服务器接收到的 TTL 值（秒）。填 0 表示不覆盖。',
  dnsCacheMaxTtl: '覆盖最大 TTL 值',
  dnsCacheMaxTtlHint: '设定 DNS 缓存条目的最大 TTL 值（秒）。填 0 表示不覆盖。',
  dnsAccessTitle: 'DNS 访问控制',
  dnsAllowedCidrs: '允许 CIDR',
  dnsAllowedCidrsHint: 'DNS 固定监听全部 IPv4/IPv6 网卡，必须填写至少一个非全网 CIDR；多个 CIDR 用逗号分隔。',
  dnsRateLimitQps: '每客户端 QPS',
  dnsMaxConcurrentQueries: 'DNS 最大并发查询',
  maxConcurrentConnections: '规则本地最大连接数',
  maxConcurrentRequests: '规则最大并发请求',
  upstreamMaxConnections: '上游最大活动连接',
  upstreamMaxIdleConnections: '每目标最大空闲连接',
  memoryLimitBytes: '规则内存上限（字节）',
  resourceTitle: '资源控制',
  resourceSubtitle: '连接、HTTP/DNS 并发与默认上游空闲连接填 0 表示不额外限额；H2/QUIC 流和正文改写必须为正数。规则填 0 时仍受全局安全阀保护。',
  resourceEdit: '调整资源控制',
  resourceSave: '保存资源控制',
  listenerConnectionLimit: '每监听组连接安全阀',
  globalHttpMaxConcurrent: '全局 HTTP 请求并发',
  globalDnsMaxConcurrent: '全局 DNS 查询并发',
  http2MaxConcurrentStreams: '每条 H2 连接最大流数',
  quicMaxIncomingStreams: '每条 QUIC 双向/单向流数',
  defaultUpstreamMaxIdleConnections: '默认每目标空闲连接',
  memoryPoolBytes: '共享动态内存池（字节）',
  defaultRuleMemoryLimitBytes: '默认规则内存上限（字节）',
  responseRewriteInputBytes: '正文改写输入上限（字节）',
  responseRewriteOutputBytes: '正文改写输出上限（字节）',
  responseRewriteMaxConcurrent: '正文改写最大并发',
  resourceMemoryHint: '内存池与正文改写参数会同时约束 DNS 缓存和普通反代正文改写。默认值为 8 GiB 共享池、单规则 384 MiB、输入 4 MiB、输出 8 MiB、并发 32；8 GiB 是按需准入上限，不会在启动时直接分配。',
  ruleResourceHint: '填写 0 表示不额外限制：本地连接仍受监听组安全阀保护，请求仍受全局 HTTP 并发保护，上游空闲连接使用全局默认值，内存使用全局默认规则上限。',
  resourceInvalid: '资源控制数值无效：连接/并发必须在允许范围内；H2、QUIC 与正文改写并发必须为正整数；内存和正文改写缓冲必须在 500 KiB 到 64 GiB 范围内，且输入加双输出缓冲不得超过默认规则内存上限。',
  resourceSaved: '资源控制已保存',
  runtimeHttp: '活动 HTTP',
  runtimeDns: '活动 DNS',
  runtimeMemory: '内存使用',
  runtimeCache: '缓存',
  runtimeRewrite: '正文改写',
  revisionConflict: '配置已被其他页面修改，已刷新为最新配置；当前草稿没有自动覆盖或重放。',
  ipCertificateRoutingHint: 'IP 访问只使用证书真实 IP SAN。单个公网 IP 经 NAT 时，普通单 IP 证书即可；同一地址族的多个公网 IP 汇入同一内网地址时，才需要一张覆盖该地址族全部允许 IP 的证书。',
  dnsAccessInvalid: 'DNS 每客户端 QPS 必须为 1 到 10000 的整数，DNS 最大并发必须为 0 到 4096 的整数（0 表示不额外限制）',
  requestLimitInvalid: '规则最大并发请求必须为 0 到 10000 的整数（0 表示不额外限制）',
  ruleResourceInvalid: '规则连接、请求、上游连接与空闲连接必须为指定范围内的非负整数；规则内存上限填 0 使用全局默认值，否则必须在 500 KiB 到当前共享内存池上限之间。',
  dnsCIDRRequired: 'DNS 监听全部网卡时必须填写至少一个非全网 CIDR 白名单',
  dnsCacheInvalid: 'DNS 缓存大小必须是大于 0 的安全整数；TTL 必须是 0 到 4294967295 的安全整数，且最大 TTL 非 0 时不能小于最小 TTL',
  dnsUpstreamTimeoutInvalid: '上游超时必须在 1 到 120 秒之间',
  listenPortInvalid: '监听端口必须是 1 到 65535 的整数',
  targetPortInvalid: '目标端口必须是 1 到 65535 的整数',
  ednsTitle: 'EDNS 客户端子网',
  ednsEnabled: '启用 EDNS 客户端子网',
  ednsMode: 'EDNS 模式',
  ednsModeAuto: '自动来源 IP',
  ednsModeCustom: '自定义 IPv4',
  ednsCustomIp: 'EDNS 自定义 IPv4',
  ednsClientSubnetPolicy: '自动模式来源策略',
  ednsClientSubnetPolicyClientIP: '使用连接客户端 IP',
  ednsClientSubnetPolicyPreferRequestPublic: '优先使用请求自带公网 ECS',
  ednsHint: '自动模式下：IPv4 会自动脱敏为末尾 .1；IPv6 直接使用连接到本机监听器的客户端 IPv6，不额外改写。自定义模式当前仅支持 IPv4，并会自动改写为末尾 .1。',
  ednsPolicyHint: '若请求里已带 ECS，且你选择“优先使用请求自带公网 ECS”，则仅在该 ECS 为公网地址时采用；私网、环回、链路本地等地址会被忽略并回退到客户端连接 IP。',
  ednsCustomRequired: '请输入有效的 IPv4 地址；保存时会自动改写为末尾 .1',
  disableIpv4Answer: '禁用 IPv4 地址解析结果',
  disableIpv6Answer: '禁用 IPv6 地址解析结果',
  dnsAnswerFilterHint: '仅作用于本地监听返回结果。禁用后会丢弃对应地址记录以及与 A/AAAA 直接相关的附属记录，剩余数据继续按上游结果透传。',
  certificate: '证书',
  ipStrategy: 'IP 优先策略',
  httpVersionStrategy: 'HTTP 版本策略',
  upstreamTlsVerify: '是否校验证书',
  apiPassthrough: '流式/API 透传',
  advertiseHttp3: '向浏览器广播 HTTP/3',
  advertiseHttp3Hint: '仅用于 HTTPS（H2+H3）。只有同端口 UDP/H3 listener 实际启动且公网 UDP 可达时才会广播；同一域名存在 WSS 规则时不会广播。广播端口按请求 Host 的外部端口推断，未带端口时使用 443；全部关闭或 H3 不可用时会清理浏览器保存的 HTTP/3 路由。',
  remark: '备注',
  cancel: '取消',
  save: '保存',
  orderLabel: '顺序',
  statusLabel: '状态',
  pathLabel: '路径',
  targetLabel: '目标',
  protocolLabel: '协议',
  certificateLabel: '证书',
  strategyLabel: '策略',
  remarkLabel: '备注',
  actionLabel: '操作',
  noCertificate: '无需证书',
  saveCreated: '反向代理已创建',
  saveUpdated: '反向代理已更新',
  reorderSaved: '匹配顺序已更新',
  enableLabel: '启用',
  listenPanelHint: '规则按端口监听全部 IPv4/IPv6 网卡。域名条件始终严格匹配 Host/SNI；留空仅允许由 IP SAN 证书覆盖的 IP 类连接。',
  targetPanelHint: '目标支持 HTTP / HTTPS / DNS。多个目标会按填写顺序依次尝试；DNS 目标之间也会按顺序回退。',
  tlsPanelHint: 'HTTPS、WSS、DoH、DoH3、DoT、DoQ 必须绑定证书。域名连接同时校验规则、证书 DNS SAN 与 Host/SNI；无 SNI 或 IP SNI 只使用覆盖目标 IP 的真实 IP SAN。',
  certificateRequired: '请选择至少一张 TLS 监听证书',
  certificateBound: '已绑定证书',
  currentHTTPNoCert: '当前监听协议无需证书',
  targetHTTPMode: 'HTTP 目标',
  ruleEnabled: '已启用',
  ruleDisabled: '已停用',
  pathRequired: '路径不能为空',
  listenMatchRequired: '域名可留空',
  listenIPLiteralNotAllowed: '域名条件不能填写 IP；IP 访问由证书真实 IP SAN 单独匹配',
  listenPortInlineNotAllowed: '域名里不能带端口，请把端口填在监听端口',
  targetAddressInlineNotAllowed: '目标地址 / 域名 里不能带端口，请把端口填在目标端口',
  targetRequired: '请填写至少一个目标地址',
  certRequiredSave: 'TLS 监听必须至少选择一张证书',
  dnsPathRequired: '当前 DNS 协议必须填写 URL 路径',
  dnsProtocolPairRequired: 'DNS 反代要求本地协议和目标协议都使用 DNS',
  dnsHostUnused: '传统 UDP/TCP DNS 没有 SNI 或 Host，按端口进入固定规则',
  dnsHttpFieldUnused: 'DNS 反代不使用 HTTP 路径改写与 API 透传',
  listenModeHTTP: 'HTTP：仅监听明文 HTTP 请求。',
  listenModeHTTPS: 'HTTPS（H2+H3）：TCP 仅提供 HTTP/2，UDP 仅提供 HTTP/3，不提供 HTTP/1.1。默认不广播 HTTP/3，开启下方开关后浏览器才会自动优先尝试 H3。',
  listenModeH2: 'H2：仅监听 TCP，仅提供 HTTPS/HTTP2，不提供 HTTP/1.1。',
  listenModeH3: 'H3：仅监听 UDP，仅提供 HTTPS/HTTP3，不提供 TCP/H2 或 HTTP/1.1。',
  listenModeDNSDoH: 'DoH（DNS）：仅监听 TCP/TLS 上的 DNS over HTTPS（H2；不会打开 H3 UDP 监听），可自定义端口和 URL 路径。',
  listenModeDNSDoHH3: 'DoH3（DNS）：仅监听 H3 的 DNS over HTTPS，可自定义端口和 URL 路径。',
  listenModeDNSDoQ: 'DoQ（DNS）：通过 QUIC 提供 DNS over QUIC，可自定义端口。',
  listenModeDNSDoT: 'DoT（DNS）：通过 TLS 提供 DNS over TLS，可自定义端口。',
  listenModeDNSUDP: 'UDP（DNS）：通过 UDP 提供传统 DNS，可自定义端口。',
  listenModeDNSTCP: 'TCP（DNS）：通过 TCP 提供传统 DNS，可自定义端口。',
  targetModeHTTP: 'HTTP：向上游发起明文 HTTP 连接。',
  targetModeHTTPS: 'HTTPS：同时支持 H2/H3 上游协商，按探测结果选择可用连接。',
  targetModeH2: 'H2：仅向上游发起 HTTPS/H2 连接。',
  targetModeH3: 'H3：仅向上游发起 HTTPS/H3 连接。',
  targetModeDNSDoH: 'DoH（DNS）：向上游发起 DNS over HTTPS，请求会实时转发。',
  targetModeDNSDoHH3: 'DoH3（DNS）：向上游发起基于 HTTP/3 的 DNS over HTTPS。',
  targetModeDNSDoQ: 'DoQ（DNS）：向上游发起 DNS over QUIC。',
  targetModeDNSDoT: 'DoT（DNS）：向上游发起 DNS over TLS。',
  targetModeDNSUDP: 'UDP（DNS）：向上游发起传统 DNS UDP 请求。',
  targetModeDNSTCP: 'TCP（DNS）：向上游发起传统 DNS TCP 请求。',
  tlsModeRequired: '当前监听协议需要 TLS 证书。',
  listenIpLocalHint: '精确域名严格匹配；*.example.com 仅匹配一个最左侧标签。多个条件任一命中即可，留空时拒绝所有域名连接。',
  targetPathRewriteHint: '目标基础路径会作为上游前缀，例如填 /api 后，请求 /foo 会转发到 /api/foo。',
  apiPassthroughHint: '开启后不改写响应正文，适合 AI、SSE 与 API 直通，避免流式内容被缓冲或替换；响应头仍按反代规则处理。',
  runtimeHint: '当请求没有命中任何规则时，HTTP 返回 404；HTTPS 的 SNI 或 Host 不匹配返回 421。',
  pathPrefixStrictHint: '填写 888 会保存为 /888；只有 /888 或 /888/后续目标路径会命中，/8888 不会命中。',
}

export const reverseProxyCompressionItems = [
  { title: reverseProxyCopy.compressionZstd, value: 'zstd' },
  { title: reverseProxyCopy.compressionS2, value: 's2' },
  { title: reverseProxyCopy.compressionSnappy, value: 'snappy' },
  { title: reverseProxyCopy.compressionBr, value: 'br' },
  { title: reverseProxyCopy.compressionDeflate, value: 'deflate' },
  { title: reverseProxyCopy.compressionGzip, value: 'gzip' },
] as const

const reverseProxyCompressionOrder = reverseProxyCompressionItems.map(item => item.value)
export const reverseProxyHeaders = [
  { title: 'ID', key: 'displayId', sortable: false, width: 72 },
  { title: reverseProxyCopy.orderLabel, key: 'listOrder', sortable: false, width: 72 },
  { title: reverseProxyCopy.statusLabel, key: 'status', sortable: false, width: 140 },
  { title: reverseProxyCopy.protocolLabel, key: 'listenProtocol', sortable: false, width: 92 },
  { title: reverseProxyCopy.connectionLabel, key: 'connectionCounts', sortable: false, width: 132 },
  { title: '监听', key: 'listen', sortable: false },
  { title: reverseProxyCopy.pathLabel, key: 'path', sortable: false, width: 150 },
  { title: reverseProxyCopy.targetLabel, key: 'target', sortable: false },
  { title: reverseProxyCopy.strategyLabel, key: 'strategy', sortable: false, width: 180 },
  { title: reverseProxyCopy.certificateLabel, key: 'certificate', sortable: false, width: 180 },
  { title: reverseProxyCopy.remarkLabel, key: 'remark', sortable: false, width: 200 },
  { title: reverseProxyCopy.actionLabel, key: 'actions', sortable: false, width: 260 },
]

const reverseProxyDNSMaxTTL = 4294967295
const reverseProxyMaximumConfiguredLimit = 1000000
const reverseProxyMaximumConfiguredStreams = 65535
const reverseProxyMinimumMemoryBytes = 500 * 1024
const reverseProxyMaximumMemoryBytes = 64 * 1024 * 1024 * 1024

export const protocolItems = [
  { title: 'HTTP', value: 'http' },
  { title: 'WS', value: 'ws' },
  { title: 'HTTPS (H2+H3)', value: 'https' },
  { title: 'WSS', value: 'wss' },
  { title: 'H2 only', value: 'h2' },
  { title: 'H3 only', value: 'h3' },
  { title: 'DoH（DNS）', value: 'dns_doh' },
  { title: 'DoH3（DNS）', value: 'dns_doh3' },
  { title: 'DoQ（DNS）', value: 'dns_doq' },
  { title: 'DoT（DNS）', value: 'dns_dot' },
  { title: 'UDP（DNS）', value: 'dns_udp' },
  { title: 'TCP（DNS）', value: 'dns_tcp' },
] as const

export const ipStrategyItems = [
  { title: '仅 IPv4', value: 'ipv4_only' },
  { title: '仅 IPv6', value: 'ipv6_only' },
  { title: '优先 IPv4', value: 'prefer_ipv4' },
  { title: '优先 IPv6', value: 'prefer_ipv6' },
] as const

export const httpVersionItems = [
  { title: 'H2/H3 均需可用（优先 H3）', value: 'dual_required_prefer_h3' },
  { title: '仅 H2', value: 'h2_only' },
  { title: '仅 H3', value: 'h3_only' },
  { title: '优先 H2', value: 'prefer_h2' },
  { title: '优先 H3', value: 'prefer_h3' },
] as const

export const ednsModeItems = [
  { title: reverseProxyCopy.ednsModeAuto, value: 'auto' },
  { title: reverseProxyCopy.ednsModeCustom, value: 'custom' },
] as const

export const ednsClientSubnetPolicyItems = [
  { title: reverseProxyCopy.ednsClientSubnetPolicyClientIP, value: 'client_ip' },
  { title: reverseProxyCopy.ednsClientSubnetPolicyPreferRequestPublic, value: 'prefer_request_public' },
] as const

const emptyOverview = (): ReverseProxyOverview => ({
  revision: 0,
  resourceSettings: defaultResourceSettings(),
  available: false,
  started: false,
  listenerCount: 0,
  enabledCount: 0,
  ruleCount: 0,
  certificateCount: 0,
  lastSyncAt: 0,
  certificates: [],
  rules: [],
  warnings: [],
  error: '',
})

export const defaultResourceSettings = (): ReverseProxyResourceSettings => ({
  listenerConnectionLimit: 4096,
  globalHttpMaxConcurrent: 4096,
  globalDnsMaxConcurrent: 4096,
  http2MaxConcurrentStreams: 250,
  quicMaxIncomingStreams: 256,
  defaultUpstreamMaxIdleConnections: 32,
  // This deliberately mirrors the backend admission ceiling. It is not an
  // eager 8 GiB browser or server allocation.
  memoryPoolBytes: 8 * 1024 * 1024 * 1024,
  defaultRuleMemoryLimitBytes: 384 * 1024 * 1024,
  responseRewriteInputBytes: 4 * 1024 * 1024,
  responseRewriteOutputBytes: 8 * 1024 * 1024,
  responseRewriteMaxConcurrent: 32,
})

export const createEmptyReverseProxyRuleForm = (): ReverseProxyRuleForm => ({
  id: 0,
  displayId: 0,
  name: '',
  enabled: true,
  listenProtocol: 'http',
  listenPort: 80,
  listenCompressionEnabled: true,
  listenCompressionAlgorithms: [...reverseProxyCompressionOrder],
  hostsText: '',
  pathPrefix: '',
  listenDnsPath: '/dns-query',
  targetProtocol: 'http',
  targetAddressesText: '',
  targetPort: 80,
  targetCompressionEnabled: true,
  targetCompressionAlgorithms: [...reverseProxyCompressionOrder],
  targetPath: '',
  targetDnsPath: '/dns-query',
  fallbackDnsUpstreams: '',
  dnsUpstreamTimeoutSeconds: 12,
  dnsCacheEnabled: false,
  dnsCacheSizeBytes: 4 * 1024 * 1024,
  dnsCacheMinTtl: 0,
  dnsCacheMaxTtl: 0,
  dnsAllowedCidrsText: '',
  dnsRateLimitQps: 50,
  dnsMaxConcurrentQueries: 0,
  ednsEnabled: false,
  ednsMode: 'auto',
  ednsCustomIp: '',
  ednsClientSubnetPolicy: 'client_ip',
  disableIpv4Answer: false,
  disableIpv6Answer: false,
  certificateRecordIds: [],
  listenHttpVersionStrategy: '',
  ipStrategy: 'prefer_ipv4',
  httpVersionStrategy: '',
  upstreamTlsVerify: true,
  maxConcurrentConnections: 0,
  maxConcurrentRequests: 0,
  upstreamMaxConnections: 0,
  upstreamMaxIdleConnections: 0,
  memoryLimitBytes: 0,
  apiPassthrough: false,
  advertiseHttp3: false,
  remark: '',
})

const asNumber = (value: unknown, fallback = 0) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

export const formatReverseProxyBytes = (value: number) => {
  const bytes = Number(value)
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const scaled = bytes / (1024 ** exponent)
  const precision = scaled >= 100 || exponent === 0 ? 0 : (scaled >= 10 ? 1 : 2)
  return `${scaled.toFixed(precision)} ${units[exponent]}`
}

const asString = (value: unknown, fallback = '') => {
  if (typeof value === 'string') return value
  if (value == null) return fallback
  return String(value)
}

const normalizeCompressionAlgorithms = (value: unknown) => {
  if (!Array.isArray(value)) return [...reverseProxyCompressionOrder]
  const selected = new Set(value.map(item => asString(item).trim().toLowerCase()))
  return reverseProxyCompressionOrder.filter(item => selected.has(item))
}

const protocolSupportsCompression = (value: string) => {
  const normalized = value.trim().toLowerCase()
  return normalized === 'http'
    || normalized === 'https'
    || normalized === 'h2'
    || normalized === 'h3'
    || normalized === 'dns_doh'
    || normalized === 'dns_doh3'
}

const asBoolean = (value: unknown, fallback = false) => {
  if (typeof value === 'boolean') return value
  if (value === 'true') return true
  if (value === 'false') return false
  return fallback
}

const normalizeStringList = (value: unknown): string[] => {
  if (!Array.isArray(value)) return []
  const seen = new Set<string>()
  const out: string[] = []
  value.forEach((item) => {
    const text = asString(item).trim()
    if (!text) return
    const key = text.toLowerCase()
    if (seen.has(key)) return
    seen.add(key)
    out.push(text)
  })
  return out
}

const normalizeNumberList = (value: unknown): number[] => {
  if (!Array.isArray(value)) return []
  const seen = new Set<number>()
  const out: number[] = []
  value.forEach((item) => {
    const id = asNumber(item)
    if (!Number.isFinite(id) || id <= 0 || seen.has(id)) return
    seen.add(id)
    out.push(id)
  })
  return out
}

const splitInputTokens = (value: string) => {
  return value
    .split(/[\s,]+/)
    .map(item => item.trim())
    .filter(Boolean)
}

const normalizeIPLiteral = (value: string) => value.trim().replace(/^\[|\]$/g, '')

const isIPv4Literal = (value: string) => {
  const normalized = normalizeIPLiteral(value)
  const parts = normalized.split('.')
  if (parts.length !== 4) return false
  return parts.every((part) => /^\d{1,3}$/.test(part) && Number(part) >= 0 && Number(part) <= 255)
}

const isIPv6Literal = (value: string) => {
  const normalized = normalizeIPLiteral(value)
  if (!normalized.includes(':')) return false
  try {
    const parsed = new URL(`http://[${normalized}]/`).hostname
    return normalizeIPLiteral(parsed).toLowerCase() === normalized.toLowerCase()
  } catch {
    return false
  }
}

const isIPLiteral = (value: string) => isIPv4Literal(value) || isIPv6Literal(value)
const normalizeEDNSCustomIPv4 = (value: string) => {
  const normalized = normalizeIPLiteral(value.trim())
  if (!isIPv4Literal(normalized)) return ''
  const parts = normalized.split('.')
  parts[3] = '1'
  return parts.join('.')
}
export const isValidEDNSCustomIP = (value: string) => normalizeEDNSCustomIPv4(value) !== ''

const hasExplicitPort = (value: string) => {
  const trimmed = value.trim()
  if (!trimmed.includes(':')) return false
  if (isIPLiteral(trimmed)) return false
  return /^(\[[0-9a-f:]+\]|[^:\[\]]+):\d+$/i.test(trimmed)
}

const splitDomainTokens = (value: string) => splitInputTokens(value).filter(token => !isIPLiteral(token))

const sortCertificateIDsByOptionOrder = (ids: number[], options: ReverseProxyCertificateOption[]) => {
  if (ids.length <= 1 || options.length === 0) return [...ids]
  const optionIndex = new Map<number, number>()
  options.forEach((item, index) => {
    optionIndex.set(item.id, index)
  })
  return [...ids].sort((a, b) => {
    const aIndex = optionIndex.get(a)
    const bIndex = optionIndex.get(b)
    if (aIndex == null && bIndex == null) return a - b
    if (aIndex == null) return 1
    if (bIndex == null) return -1
    return aIndex - bIndex
  })
}

const normalizeCertificates = (value: unknown): ReverseProxyCertificateOption[] => {
  if (!Array.isArray(value)) return []
  return value.flatMap((raw) => {
    if (raw == null || typeof raw !== 'object') return []
    const item = raw as Partial<ReverseProxyCertificateOption>
    const id = asNumber(item.id)
    if (!Number.isSafeInteger(id) || id <= 0) return []
    return [{
      id,
      displayId: asNumber(item.displayId),
      mainDomain: asString(item.mainDomain),
      domains: normalizeStringList(item.domains),
      notAfter: asNumber(item.notAfter),
      status: asString(item.status),
    }]
  })
}

const normalizeRule = (value: unknown): ReverseProxyRule => {
  const item = (value ?? {}) as Partial<ReverseProxyRule>
  const listenProtocolRaw = asString(item.listenProtocol, 'http')
  const targetProtocolRaw = asString(item.targetProtocol, 'http')
  const listenProtocolAliasRaw = asString(item.listenProtocolAlias, '')
  const targetProtocolAliasRaw = asString(item.targetProtocolAlias, '')
  const listenHttpVersionStrategy = normalizeListenHTTPVersionStrategy(asString(item.listenHttpVersionStrategy, ''))
  const httpVersionStrategy = normalizeTargetHTTPVersionStrategy(asString(item.httpVersionStrategy, ''))
  const certificateRecordIds = normalizeNumberList(item.certificateRecordIds)
  if (certificateRecordIds.length === 0) {
    const legacyCertificateRecordId = asNumber(item.certificateRecordId)
    if (legacyCertificateRecordId > 0) {
      certificateRecordIds.push(legacyCertificateRecordId)
    }
  }
  return {
    id: asNumber(item.id),
    displayId: asNumber(item.displayId),
    listOrder: asNumber(item.listOrder),
    name: asString(item.name),
    enabled: asBoolean(item.enabled, true),
    listenProtocol: deriveListenProtocolForForm(listenProtocolRaw, listenHttpVersionStrategy, listenProtocolAliasRaw),
    listenPort: asNumber(item.listenPort),
    listenCompressionEnabled: asBoolean(item.listenCompressionEnabled, true),
    listenCompressionAlgorithms: normalizeCompressionAlgorithms(item.listenCompressionAlgorithms),
    hosts: normalizeStringList(item.hosts),
    pathPrefix: asString(item.pathPrefix),
    listenDnsPath: asString(item.listenDnsPath),
    targetProtocol: deriveTargetProtocolForForm(targetProtocolRaw, httpVersionStrategy, targetProtocolAliasRaw),
    targetAddresses: normalizeStringList(item.targetAddresses),
    targetPort: asNumber(item.targetPort),
    targetCompressionEnabled: asBoolean(item.targetCompressionEnabled, true),
    targetCompressionAlgorithms: normalizeCompressionAlgorithms(item.targetCompressionAlgorithms),
    targetPath: asString(item.targetPath),
    targetDnsPath: asString(item.targetDnsPath),
    fallbackDnsUpstreams: asString(item.fallbackDnsUpstreams),
    dnsUpstreamTimeoutSeconds: asNumber(item.dnsUpstreamTimeoutSeconds, 12),
    dnsCacheEnabled: asBoolean(item.dnsCacheEnabled, false),
    dnsCacheSizeBytes: asNumber(item.dnsCacheSizeBytes, 4 * 1024 * 1024),
    dnsCacheMinTtl: asNumber(item.dnsCacheMinTtl),
    dnsCacheMaxTtl: asNumber(item.dnsCacheMaxTtl),
    dnsAllowedCidrs: normalizeStringList(item.dnsAllowedCidrs),
    dnsRateLimitQps: asNumber(item.dnsRateLimitQps, 50),
    dnsMaxConcurrentQueries: asNumber(item.dnsMaxConcurrentQueries),
    ednsEnabled: asBoolean(item.ednsEnabled, false),
    ednsMode: asString(item.ednsMode, 'auto') === 'custom' ? 'custom' : 'auto',
    ednsCustomIp: asString(item.ednsCustomIp),
    ednsClientSubnetPolicy: asString(item.ednsClientSubnetPolicy, 'client_ip') === 'prefer_request_public' ? 'prefer_request_public' : 'client_ip',
    disableIpv4Answer: asBoolean(item.disableIpv4Answer, false),
    disableIpv6Answer: asBoolean(item.disableIpv6Answer, false),
    certificateRecordIds,
    certificateRecordId: certificateRecordIds[0] ?? asNumber(item.certificateRecordId),
    certificateLabel: asString(item.certificateLabel),
    certificateLabels: normalizeStringList(item.certificateLabels),
    listenHttpVersionStrategy,
    ipStrategy: asString(item.ipStrategy, 'prefer_ipv4') as ReverseProxyRule['ipStrategy'],
    httpVersionStrategy,
    upstreamTlsVerify: asBoolean(item.upstreamTlsVerify, true),
    maxConcurrentConnections: asNumber(item.maxConcurrentConnections),
    maxConcurrentRequests: asNumber(item.maxConcurrentRequests),
    upstreamMaxConnections: asNumber(item.upstreamMaxConnections),
    upstreamMaxIdleConnections: asNumber(item.upstreamMaxIdleConnections),
    memoryLimitBytes: asNumber(item.memoryLimitBytes),
    apiPassthrough: asBoolean(item.apiPassthrough, false),
    advertiseHttp3: asBoolean(item.advertiseHttp3, false),
    remark: asString(item.remark),
    lastError: asString(item.lastError),
    runtimeStatus: asString(item.runtimeStatus),
    localConnectionCount: asNumber(item.localConnectionCount),
    upstreamConnectionCount: asNumber(item.upstreamConnectionCount),
    certificateHints: normalizeStringList(item.certificateHints),
    updatedAt: asNumber(item.updatedAt),
    createdAt: asNumber(item.createdAt),
  }
}

const normalizeResourceSettings = (value: unknown): ReverseProxyResourceSettings => {
  const item = (value ?? {}) as Partial<ReverseProxyResourceSettings>
  const defaults = defaultResourceSettings()
  return {
    listenerConnectionLimit: asNumber(item.listenerConnectionLimit, defaults.listenerConnectionLimit),
    globalHttpMaxConcurrent: asNumber(item.globalHttpMaxConcurrent, defaults.globalHttpMaxConcurrent),
    globalDnsMaxConcurrent: asNumber(item.globalDnsMaxConcurrent, defaults.globalDnsMaxConcurrent),
    http2MaxConcurrentStreams: asNumber(item.http2MaxConcurrentStreams, defaults.http2MaxConcurrentStreams),
    quicMaxIncomingStreams: asNumber(item.quicMaxIncomingStreams, defaults.quicMaxIncomingStreams),
    defaultUpstreamMaxIdleConnections: asNumber(item.defaultUpstreamMaxIdleConnections, defaults.defaultUpstreamMaxIdleConnections),
    memoryPoolBytes: asNumber(item.memoryPoolBytes, defaults.memoryPoolBytes),
    defaultRuleMemoryLimitBytes: asNumber(item.defaultRuleMemoryLimitBytes, defaults.defaultRuleMemoryLimitBytes),
    responseRewriteInputBytes: asNumber(item.responseRewriteInputBytes, defaults.responseRewriteInputBytes),
    responseRewriteOutputBytes: asNumber(item.responseRewriteOutputBytes, defaults.responseRewriteOutputBytes),
    responseRewriteMaxConcurrent: asNumber(item.responseRewriteMaxConcurrent, defaults.responseRewriteMaxConcurrent),
  }
}

const normalizeOverview = (value: unknown): ReverseProxyOverview => {
  const item = (value ?? {}) as Partial<ReverseProxyOverview>
  return {
    revision: asNumber(item.revision),
    resourceSettings: normalizeResourceSettings(item.resourceSettings),
    available: asBoolean(item.available, false),
    started: asBoolean(item.started),
    listenerCount: asNumber(item.listenerCount),
    enabledCount: asNumber(item.enabledCount),
    ruleCount: asNumber(item.ruleCount),
    certificateCount: asNumber(item.certificateCount),
    lastSyncAt: asNumber(item.lastSyncAt),
    certificates: normalizeCertificates(item.certificates),
    rules: Array.isArray(item.rules) ? item.rules.map(normalizeRule) : [],
    warnings: normalizeStringList(item.warnings),
    error: asString(item.error),
  }
}

const normalizeRuntimeOverview = (value: unknown): ReverseProxyRuntimeOverview => {
  const item = (value ?? {}) as Partial<ReverseProxyRuntimeOverview>
  const rawResources = (item.resources ?? {}) as Partial<ReverseProxyRuntimeOverview['resources']>
  const rawRules = Array.isArray(item.rules) ? item.rules : []
  return {
    revision: asNumber(item.revision),
    available: asBoolean(item.available, false),
    started: asBoolean(item.started),
    listenerCount: asNumber(item.listenerCount),
    lastSyncAt: asNumber(item.lastSyncAt),
    rules: rawRules.map((raw) => {
      const rule = raw as Partial<ReverseProxyRuntimeOverview['rules'][number]>
      return {
        id: asNumber(rule.id),
        runtimeStatus: asString(rule.runtimeStatus),
        lastError: asString(rule.lastError),
        localConnectionCount: asNumber(rule.localConnectionCount),
        upstreamConnectionCount: asNumber(rule.upstreamConnectionCount),
      }
    }),
    resources: {
      activeHttpRequests: asNumber(rawResources.activeHttpRequests),
      activeDnsQueries: asNumber(rawResources.activeDnsQueries),
      memoryUsedBytes: asNumber(rawResources.memoryUsedBytes),
      cacheUsedBytes: asNumber(rawResources.cacheUsedBytes),
      rewriteUsedBytes: asNumber(rawResources.rewriteUsedBytes),
    },
    warnings: normalizeStringList(item.warnings),
    error: asString(item.error),
  }
}

export const formatTimestamp = (value: number) => {
  if (!value) return '-'
  return formatPanelDateTime(value * 1000)
}

export const protocolLabel = (value: string) => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'dns_doh') return 'DoH（DNS）'
  if (normalized === 'dns_doh3') return 'DoH3（DNS）'
  if (normalized === 'dns_doq') return 'DoQ（DNS）'
  if (normalized === 'dns_dot') return 'DoT（DNS）'
  if (normalized === 'dns_udp') return 'UDP（DNS）'
  if (normalized === 'dns_tcp') return 'TCP（DNS）'
  if (normalized === 'ws') return 'WS'
  if (normalized === 'wss') return 'WSS'
  if (normalized === 'https') return 'HTTPS'
  if (normalized === 'h2') return 'H2'
  if (normalized === 'h3') return 'H3'
  return 'HTTP'
}

export const joinDisplay = (items: string[]) => items.join(', ')

export const certificateDisplay = (item: ReverseProxyRule) => item.certificateLabel || reverseProxyCopy.noCertificate

export const connectionCountsDisplay = (item: ReverseProxyRule) => `${item.localConnectionCount} | ${item.upstreamConnectionCount}`

export const listenMatchDisplay = (item: ReverseProxyRule) => {
  const hosts = joinDisplay(item.hosts ?? [])
  if (hosts) return hosts
  if (item.listenProtocol === 'dns_udp' || item.listenProtocol === 'dns_tcp') return '端口固定规则'
  return 'IP 连接'
}

export const statusColor = (value: string) => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'running') return 'success'
  if (normalized === 'pending') return 'info'
  if (normalized === 'upstream_error' || normalized === 'proxy_error') return 'warning'
  if (normalized === 'listener_error') return 'error'
  return 'grey'
}

export const runtimeStatusLabel = (value: string) => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'running') return '运行中'
  if (normalized === 'pending') return '等待应用'
  if (normalized === 'disabled') return '已停用'
  if (normalized === 'upstream_error') return '上游异常'
  if (normalized === 'proxy_error') return '代理异常'
  if (normalized === 'listener_error') return '监听异常'
  if (normalized === 'stopped') return '已停止'
  return '未运行'
}

export const ipStrategyLabel = (value: string) => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'ipv4_only') return '仅 IPv4'
  if (normalized === 'ipv6_only') return '仅 IPv6'
  if (normalized === 'prefer_ipv4') return '优先 IPv4'
  if (normalized === 'prefer_ipv6') return '优先 IPv6'
  return '自动选择'
}

export const httpVersionStrategyLabel = (value: string, targetProtocol = '') => {
  const normalizedProtocol = targetProtocol.trim().toLowerCase()
  if (normalizedProtocol.startsWith('dns_')) return 'DNS 上游'
  if (normalizedProtocol === 'http' || normalizedProtocol === 'ws') return reverseProxyCopy.targetHTTPMode
  if (normalizedProtocol === 'wss') return 'WSS 上游'
  const normalized = value.trim().toLowerCase()
  if (normalized === 'dual_required_prefer_h3') return 'H2/H3 均需可用（优先 H3）'
  if (normalized === 'h2_only') return '仅 H2'
  if (normalized === 'h3_only') return '仅 H3'
  if (normalized === 'prefer_h2') return '优先 H2'
  if (normalized === 'prefer_h3') return '优先 H3'
  return '-'
}

const normalizePathInput = (value: string, allowEmpty: boolean) => {
  const trimmed = value.trim()
  if (!trimmed) {
    return allowEmpty ? '' : '/'
  }
  if (trimmed.startsWith('/')) return trimmed
  return `/${trimmed}`
}

const reverseProxyAddressProtocolPrefixRE = /^https?:\/\//i

const stripReverseProxyAddressProtocolPrefix = (value: string) => value.trim().replace(reverseProxyAddressProtocolPrefixRE, '')

const normalizeListTextInput = (
  value: string,
  options: {
    stripHttpProtocolPrefix?: boolean
  } = {},
) => splitInputTokens(value)
  .map((item) => options.stripHttpProtocolPrefix ? stripReverseProxyAddressProtocolPrefix(item) : item.trim())
  .filter(item => item !== '')
  .join(', ')

const trimReverseProxyRuleFormText = (form: ReverseProxyRuleForm) => {
  form.name = form.name.trim()
  form.dnsAllowedCidrsText = normalizeListTextInput(form.dnsAllowedCidrsText)
  form.hostsText = normalizeListTextInput(form.hostsText, { stripHttpProtocolPrefix: true })
  form.pathPrefix = form.pathPrefix.trim()
  form.listenDnsPath = form.listenDnsPath.trim()
  form.targetAddressesText = normalizeListTextInput(form.targetAddressesText, { stripHttpProtocolPrefix: true })
  form.targetPath = form.targetPath.trim()
  form.targetDnsPath = form.targetDnsPath.trim()
  form.fallbackDnsUpstreams = form.fallbackDnsUpstreams.replace(/\r\n?/g, '\n').trim()
  form.ednsCustomIp = form.ednsCustomIp.trim()
  form.remark = form.remark.trim()
}

const protocolIsHTTP = (value: string) => {
  const normalized = value.trim().toLowerCase()
  return normalized === 'http' || normalized === 'ws'
}
const protocolIsDNS = (value: string) => value.trim().toLowerCase().startsWith('dns_')
const dnsProtocolUsesPath = (value: string) => {
  const normalized = value.trim().toLowerCase()
  return normalized === 'dns_doh' || normalized === 'dns_doh3'
}
const protocolIsTLS = (value: string) => {
  const normalized = value.trim().toLowerCase()
  if (protocolIsDNS(normalized)) {
    return normalized === 'dns_doh' || normalized === 'dns_doh3' || normalized === 'dns_doq' || normalized === 'dns_dot'
  }
  return !protocolIsHTTP(value)
}

const protocolNeedsCertificates = (value: string) => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'https' || normalized === 'wss' || normalized === 'h2' || normalized === 'h3') return true
  return protocolIsDNS(normalized) && protocolIsTLS(normalized)
}

const normalizeVirtualProtocol = (value: string): 'http' | 'https' | 'h2' | 'h3' => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'ws') return 'http'
  if (normalized === 'wss') return 'https'
  if (normalized === 'h2' || normalized === 'h3' || normalized === 'https') return normalized
  return 'http'
}

const normalizeListenHTTPVersionStrategy = (value: string): '' | 'h2_h3' | 'h2_only' | 'h3_only' => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'h2_only' || normalized === 'h3_only' || normalized === 'h2_h3') return normalized
  return ''
}

const normalizeTargetHTTPVersionStrategy = (value: string): ReverseProxyRule['httpVersionStrategy'] => {
  const normalized = value.trim().toLowerCase()
  if (
    normalized === 'h2_only' ||
    normalized === 'h3_only' ||
    normalized === 'prefer_h2' ||
    normalized === 'prefer_h3' ||
    normalized === 'dual_required_prefer_h3'
  ) {
    return normalized
  }
  return ''
}

const deriveListenProtocolForForm = (
  listenProtocol: string,
  listenHttpVersionStrategy: string,
  listenProtocolAlias = '',
): 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp' => {
  const alias = listenProtocolAlias.trim().toLowerCase()
  if (protocolIsDNS(alias)) return alias as 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  if (alias === 'ws') return 'ws'
  if (alias === 'wss') return 'wss'
  const raw = listenProtocol.trim().toLowerCase()
  if (protocolIsDNS(raw)) return raw as 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  if (raw === 'ws') return 'ws'
  if (raw === 'wss') return 'wss'
  const protocol = normalizeVirtualProtocol(listenProtocol)
  if (protocol !== 'https') return protocol
  const strategy = normalizeListenHTTPVersionStrategy(listenHttpVersionStrategy)
  if (strategy === 'h2_only') return 'h2'
  if (strategy === 'h3_only') return 'h3'
  return 'https'
}

const deriveTargetProtocolForForm = (
  targetProtocol: string,
  httpVersionStrategy: string,
  targetProtocolAlias = '',
): 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp' => {
  const alias = targetProtocolAlias.trim().toLowerCase()
  if (protocolIsDNS(alias)) return alias as 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  if (alias === 'ws') return 'ws'
  if (alias === 'wss') return 'wss'
  const raw = targetProtocol.trim().toLowerCase()
  if (protocolIsDNS(raw)) return raw as 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  if (raw === 'ws') return 'ws'
  if (raw === 'wss') return 'wss'
  const protocol = normalizeVirtualProtocol(targetProtocol)
  if (protocol !== 'https') return protocol
  const strategy = normalizeTargetHTTPVersionStrategy(httpVersionStrategy)
  if (strategy === 'h2_only') return 'h2'
  if (strategy === 'h3_only') return 'h3'
  return 'https'
}

const mapListenProtocolToBackend = (protocol: string): {
  listenProtocol: 'http' | 'https' | 'dns'
  listenProtocolAlias?: '' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  listenHttpVersionStrategy: '' | 'h2_h3' | 'h2_only' | 'h3_only'
} => {
  const raw = protocol.trim().toLowerCase()
  if (protocolIsDNS(raw)) {
    return {
      listenProtocol: 'dns',
      listenProtocolAlias: raw as 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp',
      listenHttpVersionStrategy: '',
    }
  }
  if (raw === 'ws') {
    return { listenProtocol: 'http', listenHttpVersionStrategy: '' }
  }
  if (raw === 'wss') {
    return { listenProtocol: 'https', listenHttpVersionStrategy: 'h2_only' }
  }
  const normalized = normalizeVirtualProtocol(protocol)
  if (normalized === 'http') {
    return { listenProtocol: 'http', listenHttpVersionStrategy: '' }
  }
  if (normalized === 'h2') {
    return { listenProtocol: 'https', listenHttpVersionStrategy: 'h2_only' }
  }
  if (normalized === 'h3') {
    return { listenProtocol: 'https', listenHttpVersionStrategy: 'h3_only' }
  }
  return { listenProtocol: 'https', listenHttpVersionStrategy: 'h2_h3' }
}

const mapTargetProtocolToBackend = (
  protocol: string,
  strategy: ReverseProxyRuleForm['httpVersionStrategy'],
): {
  targetProtocol: 'http' | 'https' | 'dns'
  targetProtocolAlias?: '' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  httpVersionStrategy: ReverseProxyRuleForm['httpVersionStrategy']
} => {
  const raw = protocol.trim().toLowerCase()
  if (protocolIsDNS(raw)) {
    return {
      targetProtocol: 'dns',
      targetProtocolAlias: raw as 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp',
      httpVersionStrategy: '',
    }
  }
  if (raw === 'ws') {
    return { targetProtocol: 'http', httpVersionStrategy: '' }
  }
  if (raw === 'wss') {
    return { targetProtocol: 'https', httpVersionStrategy: '' }
  }
  const normalized = normalizeVirtualProtocol(protocol)
  if (normalized === 'http') {
    return { targetProtocol: 'http', httpVersionStrategy: '' }
  }
  if (normalized === 'h2') {
    return { targetProtocol: 'https', httpVersionStrategy: 'h2_only' }
  }
  if (normalized === 'h3') {
    return { targetProtocol: 'https', httpVersionStrategy: 'h3_only' }
  }
  const normalizedStrategy = normalizeTargetHTTPVersionStrategy(strategy)
  return {
    targetProtocol: 'https',
    httpVersionStrategy: normalizedStrategy || 'prefer_h2',
  }
}

export const mapRuleToForm = (rule?: ReverseProxyRule): ReverseProxyRuleForm => {
  const listenProtocol = deriveListenProtocolForForm(
    rule?.listenProtocol ?? 'http',
    rule?.listenHttpVersionStrategy ?? '',
    rule?.listenProtocolAlias ?? '',
  )
  const targetProtocol = deriveTargetProtocolForForm(
    rule?.targetProtocol ?? 'http',
    rule?.httpVersionStrategy ?? '',
    rule?.targetProtocolAlias ?? '',
  )
  const normalizedTargetStrategy = normalizeTargetHTTPVersionStrategy(rule?.httpVersionStrategy ?? '')
  const targetStrategy = (() => {
    if (targetProtocol === 'http') return ''
    if (targetProtocol === 'wss') return ''
    if (targetProtocol === 'h2') return 'h2_only'
    if (targetProtocol === 'h3') return 'h3_only'
    return normalizedTargetStrategy || 'prefer_h2'
  })()
  return {
    id: rule?.id ?? 0,
    displayId: rule?.displayId ?? 0,
    name: rule?.name ?? '',
    enabled: rule?.enabled ?? true,
    listenProtocol,
    listenPort: rule?.listenPort ?? 80,
    listenCompressionEnabled: rule?.listenCompressionEnabled ?? true,
    listenCompressionAlgorithms: normalizeCompressionAlgorithms(rule?.listenCompressionAlgorithms),
    hostsText: normalizeStringList(rule?.hosts ?? []).join(', '),
    pathPrefix: rule?.pathPrefix ?? '',
    listenDnsPath: rule?.listenDnsPath ?? (dnsProtocolUsesPath(listenProtocol) ? '/dns-query' : ''),
    targetProtocol,
    targetAddressesText: (rule?.targetAddresses ?? []).join(', '),
    targetPort: rule?.targetPort ?? 80,
    targetCompressionEnabled: rule?.targetCompressionEnabled ?? true,
    targetCompressionAlgorithms: normalizeCompressionAlgorithms(rule?.targetCompressionAlgorithms),
    targetPath: rule?.targetPath ?? '',
    targetDnsPath: rule?.targetDnsPath ?? (dnsProtocolUsesPath(targetProtocol) ? '/dns-query' : ''),
    fallbackDnsUpstreams: rule?.fallbackDnsUpstreams ?? '',
    dnsUpstreamTimeoutSeconds: rule?.dnsUpstreamTimeoutSeconds ?? 12,
    dnsCacheEnabled: rule?.dnsCacheEnabled ?? false,
    dnsCacheSizeBytes: rule?.dnsCacheSizeBytes ?? (4 * 1024 * 1024),
    dnsCacheMinTtl: rule?.dnsCacheMinTtl ?? 0,
    dnsCacheMaxTtl: rule?.dnsCacheMaxTtl ?? 0,
    dnsAllowedCidrsText: normalizeStringList(rule?.dnsAllowedCidrs ?? []).join(', '),
    dnsRateLimitQps: rule?.dnsRateLimitQps ?? 50,
    dnsMaxConcurrentQueries: rule?.dnsMaxConcurrentQueries ?? 0,
    ednsEnabled: rule?.ednsEnabled ?? false,
    ednsMode: rule?.ednsMode === 'custom' ? 'custom' : 'auto',
    ednsCustomIp: rule?.ednsCustomIp ?? '',
    ednsClientSubnetPolicy: rule?.ednsClientSubnetPolicy === 'prefer_request_public' ? 'prefer_request_public' : 'client_ip',
    disableIpv4Answer: rule?.disableIpv4Answer ?? false,
    disableIpv6Answer: rule?.disableIpv6Answer ?? false,
    certificateRecordIds: (() => {
      const ids = normalizeNumberList(rule?.certificateRecordIds ?? [])
      if (ids.length > 0) return ids
      const legacyID = asNumber(rule?.certificateRecordId ?? 0)
      return legacyID > 0 ? [legacyID] : []
    })(),
    listenHttpVersionStrategy: mapListenProtocolToBackend(listenProtocol).listenHttpVersionStrategy,
    ipStrategy: rule?.ipStrategy ?? 'prefer_ipv4',
    httpVersionStrategy: targetStrategy,
    upstreamTlsVerify: targetProtocol === 'http' ? false : (rule?.upstreamTlsVerify ?? true),
    maxConcurrentConnections: rule?.maxConcurrentConnections ?? 0,
    maxConcurrentRequests: rule?.maxConcurrentRequests ?? 0,
    upstreamMaxConnections: rule?.upstreamMaxConnections ?? 0,
    upstreamMaxIdleConnections: rule?.upstreamMaxIdleConnections ?? 0,
    memoryLimitBytes: rule?.memoryLimitBytes ?? 0,
    apiPassthrough: rule?.apiPassthrough ?? false,
    advertiseHttp3: rule?.advertiseHttp3 ?? false,
    remark: rule?.remark ?? '',
  }
}

const normalizeEDNSCustomIPInForm = (form: ReverseProxyRuleForm) => {
  if (!protocolIsDNS(form.listenProtocol) || !form.ednsEnabled || form.ednsMode !== 'custom') return
  const normalized = normalizeEDNSCustomIPv4(form.ednsCustomIp)
  if (!normalized) return
  form.ednsCustomIp = normalized
}

export const buildReverseProxyPayload = (
  form: ReverseProxyRuleForm,
  certificates: ReverseProxyCertificateOption[] = [],
) => {
  const name = form.name.trim()
  const hostsText = normalizeListTextInput(form.hostsText, { stripHttpProtocolPrefix: true })
  const pathPrefix = form.pathPrefix.trim()
  const targetAddressesText = normalizeListTextInput(form.targetAddressesText, { stripHttpProtocolPrefix: true })
  const targetPath = form.targetPath.trim()
  const listenDnsPath = form.listenDnsPath.trim()
  const targetDnsPath = form.targetDnsPath.trim()
  const fallbackDnsUpstreams = form.fallbackDnsUpstreams.replace(/\r\n?/g, '\n').trim()
  const dnsAllowedCidrsText = normalizeListTextInput(form.dnsAllowedCidrsText)
  const ednsCustomIp = normalizeEDNSCustomIPv4(form.ednsCustomIp)
  const remark = form.remark.trim()
  const listenNames = splitInputTokens(hostsText)
  const listenProtocol = mapListenProtocolToBackend(form.listenProtocol)
  const targetProtocol = mapTargetProtocolToBackend(form.targetProtocol, form.httpVersionStrategy)
  const listenProtocolAlias = (() => {
    const raw = form.listenProtocol.trim().toLowerCase()
    if (raw === 'ws' || raw === 'wss') return raw
    return ''
  })()
  const targetProtocolAlias = (() => {
    const raw = form.targetProtocol.trim().toLowerCase()
    if (raw === 'ws' || raw === 'wss') return raw
    return ''
  })()
  const certificateRecordIds = sortCertificateIDsByOptionOrder(
    normalizeNumberList(form.certificateRecordIds),
    certificates,
  )
  const listenCompressionSupported = protocolSupportsCompression(form.listenProtocol)
  const targetCompressionSupported = protocolSupportsCompression(form.targetProtocol)
  const listenCompressionEnabled = listenCompressionSupported && form.listenCompressionEnabled !== false
  const targetCompressionEnabled = targetCompressionSupported && form.targetCompressionEnabled !== false
  return {
    id: form.id,
    name,
    enabled: form.enabled,
    listenProtocol: listenProtocol.listenProtocol,
    listenProtocolAlias: listenProtocol.listenProtocolAlias || listenProtocolAlias,
    listenPort: asNumber(form.listenPort),
    listenCompressionEnabled,
    listenCompressionAlgorithms: listenCompressionEnabled
      ? normalizeCompressionAlgorithms(form.listenCompressionAlgorithms)
      : [],
    hosts: (!protocolIsDNS(form.listenProtocol) || protocolNeedsCertificates(form.listenProtocol)) ? listenNames.join(', ') : '',
    pathPrefix: normalizePathInput(pathPrefix, true),
    listenDnsPath: dnsProtocolUsesPath(form.listenProtocol) ? normalizePathInput(listenDnsPath, true) : '',
    targetProtocol: targetProtocol.targetProtocol,
    targetProtocolAlias: targetProtocol.targetProtocolAlias || targetProtocolAlias,
    targetAddresses: targetAddressesText,
    targetPort: asNumber(form.targetPort),
    targetCompressionEnabled,
    targetCompressionAlgorithms: targetCompressionEnabled
      ? normalizeCompressionAlgorithms(form.targetCompressionAlgorithms)
      : [],
    targetPath: normalizePathInput(targetPath, true),
    targetDnsPath: dnsProtocolUsesPath(form.targetProtocol) ? normalizePathInput(targetDnsPath, true) : '',
    fallbackDnsUpstreams: protocolIsDNS(form.listenProtocol) && protocolIsDNS(form.targetProtocol) ? fallbackDnsUpstreams : '',
    dnsUpstreamTimeoutSeconds: protocolIsDNS(form.listenProtocol) && protocolIsDNS(form.targetProtocol) ? asNumber(form.dnsUpstreamTimeoutSeconds, 12) : 12,
    dnsCacheEnabled: protocolIsDNS(form.listenProtocol) && protocolIsDNS(form.targetProtocol) ? form.dnsCacheEnabled : false,
    dnsCacheSizeBytes: protocolIsDNS(form.listenProtocol) && protocolIsDNS(form.targetProtocol) ? asNumber(form.dnsCacheSizeBytes, 4 * 1024 * 1024) : (4 * 1024 * 1024),
    dnsCacheMinTtl: protocolIsDNS(form.listenProtocol) && protocolIsDNS(form.targetProtocol) ? asNumber(form.dnsCacheMinTtl) : 0,
    dnsCacheMaxTtl: protocolIsDNS(form.listenProtocol) && protocolIsDNS(form.targetProtocol) ? asNumber(form.dnsCacheMaxTtl) : 0,
    dnsAllowedCidrs: protocolIsDNS(form.listenProtocol) ? dnsAllowedCidrsText : '',
    dnsRateLimitQps: protocolIsDNS(form.listenProtocol) ? asNumber(form.dnsRateLimitQps, 50) : 50,
    dnsMaxConcurrentQueries: protocolIsDNS(form.listenProtocol) ? asNumber(form.dnsMaxConcurrentQueries) : 0,
    ednsEnabled: protocolIsDNS(form.listenProtocol) ? form.ednsEnabled : false,
    ednsMode: protocolIsDNS(form.listenProtocol) ? form.ednsMode : 'auto',
    ednsCustomIp: protocolIsDNS(form.listenProtocol) && form.ednsEnabled && form.ednsMode === 'custom' ? ednsCustomIp : '',
    ednsClientSubnetPolicy: protocolIsDNS(form.listenProtocol) && form.ednsEnabled ? form.ednsClientSubnetPolicy : 'client_ip',
    disableIpv4Answer: protocolIsDNS(form.listenProtocol) ? form.disableIpv4Answer : false,
    disableIpv6Answer: protocolIsDNS(form.listenProtocol) ? form.disableIpv6Answer : false,
    certificateRecordIds: protocolNeedsCertificates(form.listenProtocol) ? certificateRecordIds : [],
    certificateRecordId: protocolNeedsCertificates(form.listenProtocol) ? (certificateRecordIds[0] ?? 0) : 0,
    listenHttpVersionStrategy: listenProtocol.listenHttpVersionStrategy,
    ipStrategy: form.ipStrategy,
    httpVersionStrategy: targetProtocol.targetProtocol === 'https' ? targetProtocol.httpVersionStrategy : '',
    upstreamTlsVerify: protocolIsTLS(form.targetProtocol) ? form.upstreamTlsVerify : false,
    maxConcurrentConnections: protocolIsDNS(form.listenProtocol) ? 0 : asNumber(form.maxConcurrentConnections),
    maxConcurrentRequests: protocolIsDNS(form.listenProtocol) ? 0 : asNumber(form.maxConcurrentRequests),
    upstreamMaxConnections: protocolIsDNS(form.targetProtocol) ? 0 : asNumber(form.upstreamMaxConnections),
    upstreamMaxIdleConnections: protocolIsDNS(form.targetProtocol) ? 0 : asNumber(form.upstreamMaxIdleConnections),
    memoryLimitBytes: asNumber(form.memoryLimitBytes),
    apiPassthrough: form.apiPassthrough,
    advertiseHttp3: form.listenProtocol === 'https' ? form.advertiseHttp3 : false,
    remark,
  }
}

export function useReverseProxyManage(props: { active?: boolean }) {
  const loading = ref(false)
  const refreshing = ref(false)
  const saving = ref(false)
  const savingResources = ref(false)
  const mutationBusy = ref(false)
  const hasLoaded = ref(false)
  const loadError = ref('')
  const dialogVisible = ref(false)
  const resourceDialogVisible = ref(false)
  const rowBusyId = ref(0)
  const searchText = ref('')
  const overview = ref<ReverseProxyOverview>(emptyOverview())
  const runtimeUsage = ref<ReverseProxyRuntimeOverview['resources']>({
    activeHttpRequests: 0,
    activeDnsQueries: 0,
    memoryUsedBytes: 0,
    cacheUsedBytes: 0,
    rewriteUsedBytes: 0,
  })
  const editingRule = ref<ReverseProxyRuleForm>(createEmptyReverseProxyRuleForm())
  const editingResources = ref<ReverseProxyResourceSettings>(defaultResourceSettings())
  const editingRuleRevision = ref(0)
  const editingResourcesRevision = ref(0)
  const pollTimer = ref<number | null>(null)
  const overviewRequest = ref<Promise<Msg> | null>(null)
  const runtimeRequest = ref<Promise<Msg> | null>(null)
  const configurationConflict = ref(false)
  const actionsDisabled = computed(() => mutationBusy.value || !hasLoaded.value || Boolean(loadError.value))
  let latestOverviewRequestId = 0
  const isRecord = (value: unknown): value is Record<string, unknown> => value != null && typeof value === 'object' && !Array.isArray(value)

  const applyOverview = (raw: unknown, clearConflict = true) => {
    if (!isRecord(raw) || !Array.isArray(raw.rules) || !isRecord(raw.resourceSettings)) return false
    const nextOverview = normalizeOverview(raw)
    // A GET started before a successful write can finish afterwards.  Revisions
    // are monotonic, so never let that stale response roll the UI back and make
    // the following queued write use an obsolete expectedRevision.
    if (nextOverview.revision > 0 && overview.value.revision > nextOverview.revision) return false
    overview.value = nextOverview
    hasLoaded.value = true
    loadError.value = ''
    if (!resourceDialogVisible.value) {
      editingResources.value = { ...overview.value.resourceSettings }
    }
    if (clearConflict) configurationConflict.value = false
    return true
  }

  const runMutation = async (operation: () => Promise<Msg>, applySnapshot = true): Promise<Msg | null> => {
    if (mutationBusy.value) return null
    mutationBusy.value = true
    try {
      const msg = await operation()
      if (msg.success && applySnapshot) applyOverview(msg.obj)
      return msg
    } finally {
      mutationBusy.value = false
    }
  }

  const isRevisionConflict = (msg: Msg | null) => !msg?.success && msg?.obj?.code === 'revision_conflict'

  const handleRevisionConflict = async (msg: Msg | null) => {
    if (!isRevisionConflict(msg)) return false
    configurationConflict.value = true
    await fetchOverview(true, true)
    push.warning({ duration: 5000, message: reverseProxyCopy.revisionConflict })
    return true
  }

  const fetchOverview = async (silent = false, preserveConflict = false) => {
    if (overviewRequest.value) {
      return overviewRequest.value
    }
    if (!silent) loading.value = true
    const requestId = ++latestOverviewRequestId
    const request = (async () => {
      const msg = await HttpUtils.get('api/reverse-proxy-overview', {}, { silentErrorToast: silent })
      if (msg.success && requestId === latestOverviewRequestId) {
        if (!applyOverview(msg.obj, !preserveConflict)) {
          loadError.value = reverseProxyCopy.loadFailed
          overview.value.available = false
        }
      } else if (!msg.success && requestId === latestOverviewRequestId) {
        loadError.value = msg.msg || reverseProxyCopy.loadFailed
        overview.value.available = false
      }
      return msg
    })()
    overviewRequest.value = request
    try {
      return await request
    } finally {
      if (overviewRequest.value === request) {
        overviewRequest.value = null
      }
      if (!silent) loading.value = false
    }
  }

  const refreshOverview = async () => {
    refreshing.value = true
    try {
      await fetchOverview(false)
    } finally {
      refreshing.value = false
    }
  }

  const mergeRuntime = (raw: unknown) => {
    const runtime = normalizeRuntimeOverview(raw)
    if (runtime.revision > 0 && overview.value.revision > 0 && runtime.revision !== overview.value.revision) {
      configurationConflict.value = true
      // applyOverview deliberately leaves open dialog drafts intact, so the
      // newest persisted configuration can be fetched immediately without
      // replaying or silently overwriting a rule/resource edit in progress.
      void fetchOverview(true, true)
      return
    }
    overview.value.available = runtime.available
    overview.value.started = runtime.started
    overview.value.listenerCount = runtime.listenerCount
    overview.value.lastSyncAt = runtime.lastSyncAt
    overview.value.warnings = runtime.warnings ?? []
    overview.value.error = runtime.error ?? ''
    const byID = new Map(runtime.rules.map(rule => [rule.id, rule]))
    overview.value.rules = overview.value.rules.map((rule) => {
      const state = byID.get(rule.id)
      if (!state || !rule.enabled) {
        return {
          ...rule,
          runtimeStatus: rule.enabled ? 'pending' : 'disabled',
          lastError: '',
          localConnectionCount: 0,
          upstreamConnectionCount: 0,
        }
      }
      return {
        ...rule,
        runtimeStatus: state.runtimeStatus,
        lastError: state.lastError,
        localConnectionCount: state.localConnectionCount,
        upstreamConnectionCount: state.upstreamConnectionCount,
      }
    })
    runtimeUsage.value = runtime.resources
  }

  const fetchRuntime = async () => {
    if (!props.active || !hasLoaded.value || Boolean(loadError.value)) return null
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return null
    if (runtimeRequest.value) return runtimeRequest.value
    const request = (async () => {
      const msg = await HttpUtils.get('api/reverse-proxy-runtime', {}, { silentErrorToast: true })
      if (msg.success && isRecord(msg.obj) && Array.isArray(msg.obj.rules) && isRecord(msg.obj.resources)) mergeRuntime(msg.obj)
      return msg
    })()
    runtimeRequest.value = request
    try {
      return await request
    } finally {
      if (runtimeRequest.value === request) runtimeRequest.value = null
    }
  }

  const openResourceDialog = () => {
    if (actionsDisabled.value) return
    editingResources.value = { ...overview.value.resourceSettings }
    editingResourcesRevision.value = overview.value.revision
    configurationConflict.value = false
    resourceDialogVisible.value = true
  }

  const resourcesAreValid = (value: ReverseProxyResourceSettings) => {
    const boundedNonNegative = [
      value.listenerConnectionLimit,
      value.globalHttpMaxConcurrent,
      value.globalDnsMaxConcurrent,
      value.defaultUpstreamMaxIdleConnections,
    ]
    if (boundedNonNegative.some(item => !Number.isSafeInteger(Number(item)) || Number(item) < 0 || Number(item) > reverseProxyMaximumConfiguredLimit)) return false
    if (!Number.isSafeInteger(Number(value.http2MaxConcurrentStreams))
      || Number(value.http2MaxConcurrentStreams) < 1
      || Number(value.http2MaxConcurrentStreams) > reverseProxyMaximumConfiguredStreams) return false
    if (!Number.isSafeInteger(Number(value.quicMaxIncomingStreams))
      || Number(value.quicMaxIncomingStreams) < 1
      || Number(value.quicMaxIncomingStreams) > reverseProxyMaximumConfiguredStreams) return false
    const memoryValues = [
      value.memoryPoolBytes,
      value.defaultRuleMemoryLimitBytes,
      value.responseRewriteInputBytes,
      value.responseRewriteOutputBytes,
    ]
    if (memoryValues.some(item => !Number.isSafeInteger(Number(item))
      || Number(item) < reverseProxyMinimumMemoryBytes
      || Number(item) > reverseProxyMaximumMemoryBytes)) return false
    if (!Number.isSafeInteger(Number(value.responseRewriteMaxConcurrent))
      || Number(value.responseRewriteMaxConcurrent) < 1
      || Number(value.responseRewriteMaxConcurrent) > reverseProxyMaximumConfiguredLimit) return false
    return Number(value.defaultRuleMemoryLimitBytes) <= Number(value.memoryPoolBytes)
      && Number(value.responseRewriteInputBytes) + Number(value.responseRewriteOutputBytes) * 2 <= Number(value.defaultRuleMemoryLimitBytes)
  }

  const saveResources = async () => {
    if (savingResources.value || actionsDisabled.value) return
    if (!resourcesAreValid(editingResources.value)) {
      push.warning({ duration: 5000, message: reverseProxyCopy.resourceInvalid })
      return
    }
    const payload = {
      ...editingResources.value,
      expectedRevision: editingResourcesRevision.value,
    }
    savingResources.value = true
    try {
      const msg = await runMutation(() => HttpUtils.post('api/reverse-proxy-settings', payload, { headers: { 'Content-Type': 'application/json' } }))
      if (msg?.success) {
        resourceDialogVisible.value = false
        push.success({ duration: 3500, message: reverseProxyCopy.resourceSaved })
      } else {
        await handleRevisionConflict(msg)
      }
    } finally {
      savingResources.value = false
    }
  }

  const openRuleDialog = (rule?: ReverseProxyRule) => {
    if (actionsDisabled.value) return
    editingRule.value = mapRuleToForm(rule)
    editingRuleRevision.value = overview.value.revision
    configurationConflict.value = false
    if (protocolIsHTTP(editingRule.value.listenProtocol)) {
      editingRule.value.certificateRecordIds = []
    }
    editingRule.value.certificateRecordIds = sortCertificateIDsByOptionOrder(
      normalizeNumberList(editingRule.value.certificateRecordIds),
      overview.value.certificates,
    )
    if (protocolIsHTTP(editingRule.value.targetProtocol)) {
      editingRule.value.httpVersionStrategy = ''
      editingRule.value.upstreamTlsVerify = false
    } else if (editingRule.value.targetProtocol === 'h2') {
      editingRule.value.httpVersionStrategy = 'h2_only'
      editingRule.value.upstreamTlsVerify = true
    } else if (editingRule.value.targetProtocol === 'h3') {
      editingRule.value.httpVersionStrategy = 'h3_only'
      editingRule.value.upstreamTlsVerify = true
    } else if (!editingRule.value.httpVersionStrategy) {
      editingRule.value.httpVersionStrategy = 'prefer_h2'
      editingRule.value.upstreamTlsVerify = true
    }
    dialogVisible.value = true
  }

  const normalizeCustomEDNSInput = () => {
    normalizeEDNSCustomIPInForm(editingRule.value)
  }

  const normalizeRuleTextInputs = () => {
    trimReverseProxyRuleFormText(editingRule.value)
    normalizeEDNSCustomIPInForm(editingRule.value)
  }

  const saveRule = async () => {
    if (saving.value || actionsDisabled.value) return
    normalizeRuleTextInputs()
    const listenPort = Number(editingRule.value.listenPort)
    if (!Number.isSafeInteger(listenPort) || listenPort < 1 || listenPort > 65535) {
      push.warning({ duration: 4000, message: reverseProxyCopy.listenPortInvalid })
      return
    }
    const targetPort = Number(editingRule.value.targetPort)
    if (!Number.isSafeInteger(targetPort) || targetPort < 1 || targetPort > 65535) {
      push.warning({ duration: 4000, message: reverseProxyCopy.targetPortInvalid })
      return
    }
    if (protocolIsDNS(editingRule.value.listenProtocol) !== protocolIsDNS(editingRule.value.targetProtocol)) {
      push.warning({ duration: 4000, message: reverseProxyCopy.dnsProtocolPairRequired })
      return
    }
    if (protocolIsDNS(editingRule.value.listenProtocol)) {
      editingRule.value.maxConcurrentConnections = 0
      editingRule.value.maxConcurrentRequests = 0
      editingRule.value.upstreamMaxConnections = 0
      editingRule.value.upstreamMaxIdleConnections = 0
      const timeout = Number(editingRule.value.dnsUpstreamTimeoutSeconds)
      if (!Number.isSafeInteger(timeout) || timeout < 1 || timeout > 120) {
        push.warning({ duration: 4000, message: reverseProxyCopy.dnsUpstreamTimeoutInvalid })
        return
      }
      const cacheSize = Number(editingRule.value.dnsCacheSizeBytes)
      const minTtl = Number(editingRule.value.dnsCacheMinTtl)
      const maxTtl = Number(editingRule.value.dnsCacheMaxTtl)
      if (!Number.isSafeInteger(cacheSize) || cacheSize <= 0 || !Number.isSafeInteger(minTtl) || !Number.isSafeInteger(maxTtl) || minTtl < 0 || maxTtl < 0 || minTtl > reverseProxyDNSMaxTTL || maxTtl > reverseProxyDNSMaxTTL || (maxTtl > 0 && minTtl > maxTtl)) {
        push.warning({ duration: 4000, message: reverseProxyCopy.dnsCacheInvalid })
        return
      }
      const configuredMemoryLimit = Number(editingRule.value.memoryLimitBytes)
      const effectiveRuleMemory = configuredMemoryLimit > 0
        ? configuredMemoryLimit
        : Number(overview.value.resourceSettings.defaultRuleMemoryLimitBytes)
      if (editingRule.value.dnsCacheEnabled && (!Number.isSafeInteger(effectiveRuleMemory)
        || effectiveRuleMemory < reverseProxyMinimumMemoryBytes
        || effectiveRuleMemory > Number(overview.value.resourceSettings.memoryPoolBytes)
        || cacheSize > effectiveRuleMemory)) {
        push.warning({ duration: 4000, message: reverseProxyCopy.dnsCacheInvalid })
        return
      }
      const dnsRateLimitQps = Number(editingRule.value.dnsRateLimitQps)
      const dnsMaxConcurrentQueries = Number(editingRule.value.dnsMaxConcurrentQueries)
      if (!Number.isSafeInteger(dnsRateLimitQps) || dnsRateLimitQps < 1 || dnsRateLimitQps > 10000 || !Number.isSafeInteger(dnsMaxConcurrentQueries) || dnsMaxConcurrentQueries < 0 || dnsMaxConcurrentQueries > 4096) {
        push.warning({ duration: 4000, message: reverseProxyCopy.dnsAccessInvalid })
        return
      }
      if (splitInputTokens(editingRule.value.dnsAllowedCidrsText).length === 0) {
        push.warning({ duration: 4000, message: reverseProxyCopy.dnsCIDRRequired })
        return
      }
    }
    const maxConcurrentRequests = Number(editingRule.value.maxConcurrentRequests)
    if (!Number.isSafeInteger(maxConcurrentRequests) || maxConcurrentRequests < 0 || maxConcurrentRequests > 10000) {
      push.warning({ duration: 4000, message: reverseProxyCopy.requestLimitInvalid })
      return
    }
    const maxConcurrentConnections = Number(editingRule.value.maxConcurrentConnections)
    const upstreamMaxConnections = Number(editingRule.value.upstreamMaxConnections)
    const upstreamMaxIdleConnections = Number(editingRule.value.upstreamMaxIdleConnections)
    const memoryLimitBytes = Number(editingRule.value.memoryLimitBytes)
    if (!Number.isSafeInteger(maxConcurrentConnections) || maxConcurrentConnections < 0 || maxConcurrentConnections > reverseProxyMaximumConfiguredLimit
      || !Number.isSafeInteger(upstreamMaxConnections) || upstreamMaxConnections < 0 || upstreamMaxConnections > 1_000_000
      || !Number.isSafeInteger(upstreamMaxIdleConnections) || upstreamMaxIdleConnections < 0 || upstreamMaxIdleConnections > 1_000_000
      || !Number.isSafeInteger(memoryLimitBytes) || memoryLimitBytes < 0
      || (memoryLimitBytes > 0 && (memoryLimitBytes < reverseProxyMinimumMemoryBytes
        || memoryLimitBytes > reverseProxyMaximumMemoryBytes
        || memoryLimitBytes > Number(overview.value.resourceSettings.memoryPoolBytes)))) {
      push.warning({ duration: 4000, message: reverseProxyCopy.ruleResourceInvalid })
      return
    }
    const listenUsesDomainCondition = !protocolIsDNS(editingRule.value.listenProtocol) || protocolNeedsCertificates(editingRule.value.listenProtocol)
    if (listenUsesDomainCondition && splitInputTokens(editingRule.value.hostsText).some(isIPLiteral)) {
      push.warning({ duration: 4000, message: reverseProxyCopy.listenIPLiteralNotAllowed })
      return
    }
    if (listenUsesDomainCondition && splitInputTokens(editingRule.value.hostsText).some(hasExplicitPort)) {
      push.warning({ duration: 4000, message: reverseProxyCopy.listenPortInlineNotAllowed })
      return
    }
    if (splitInputTokens(editingRule.value.targetAddressesText).some(hasExplicitPort)) {
      push.warning({ duration: 4000, message: reverseProxyCopy.targetAddressInlineNotAllowed })
      return
    }
    if (!editingRule.value.targetAddressesText.trim()) {
      push.warning({ duration: 4000, message: reverseProxyCopy.targetRequired })
      return
    }
    if (!protocolIsDNS(editingRule.value.listenProtocol)) {
      editingRule.value.pathPrefix = normalizePathInput(editingRule.value.pathPrefix, true)
    }
    if (!protocolIsDNS(editingRule.value.targetProtocol)) {
      editingRule.value.targetPath = normalizePathInput(editingRule.value.targetPath, true)
    }
    if (dnsProtocolUsesPath(editingRule.value.listenProtocol) && !editingRule.value.listenDnsPath.trim()) {
      push.warning({ duration: 4000, message: reverseProxyCopy.dnsPathRequired })
      return
    }
    if (dnsProtocolUsesPath(editingRule.value.targetProtocol) && !editingRule.value.targetDnsPath.trim()) {
      push.warning({ duration: 4000, message: reverseProxyCopy.dnsPathRequired })
      return
    }
    if (protocolIsDNS(editingRule.value.listenProtocol) && editingRule.value.ednsEnabled && editingRule.value.ednsMode === 'custom' && !isValidEDNSCustomIP(editingRule.value.ednsCustomIp)) {
      push.warning({ duration: 4000, message: reverseProxyCopy.ednsCustomRequired })
      return
    }
    if (protocolNeedsCertificates(editingRule.value.listenProtocol) && editingRule.value.certificateRecordIds.length === 0) {
      push.warning({ duration: 4000, message: reverseProxyCopy.certRequiredSave })
      return
    }

    const editingID = editingRule.value.id
    const payload = {
      ...buildReverseProxyPayload(editingRule.value, overview.value.certificates),
      expectedRevision: editingRuleRevision.value,
    }
    saving.value = true
    try {
      const msg = await runMutation(() => HttpUtils.post(
        'api/reverse-proxy-rule',
        payload,
        {
          headers: {
            'Content-Type': 'application/json',
          },
        },
      ))
      if (msg?.success) {
        dialogVisible.value = false
        push.success({
          duration: 4000,
          message: editingID > 0 ? reverseProxyCopy.saveUpdated : reverseProxyCopy.saveCreated,
        })
      } else {
        await handleRevisionConflict(msg)
      }
    } finally {
      saving.value = false
    }
  }

  const removeRule = async (rule: ReverseProxyRule) => {
    if (actionsDisabled.value) return
    const confirmed = await confirm({
      message: reverseProxyCopy.deleteConfirm.replace('{name}', rule.name || `#${rule.displayId}`),
      severity: 'danger',
      confirmText: i18n.global.t('confirmDialog.actions.delete'),
    })
    if (!confirmed || actionsDisabled.value) return
    const payload = { id: rule.id, expectedRevision: overview.value.revision }
    rowBusyId.value = rule.id
    try {
      const msg = await runMutation(() => HttpUtils.post('api/reverse-proxy-rule-delete', payload, {
        headers: {
          'Content-Type': 'application/json',
        },
      }))
      await handleRevisionConflict(msg)
    } finally {
      rowBusyId.value = 0
    }
  }

  const toggleRule = async (rule: ReverseProxyRule, enabled: boolean) => {
    if (actionsDisabled.value) return
    const payload = { id: rule.id, enabled, expectedRevision: overview.value.revision }
    rowBusyId.value = rule.id
    try {
      const msg = await runMutation(() => HttpUtils.post('api/reverse-proxy-rule-status', payload, {
        headers: {
          'Content-Type': 'application/json',
        },
      }), false)
      if (msg?.success) {
        const result = msg.obj ?? {}
        overview.value.revision = asNumber(result.revision, overview.value.revision)
        overview.value.rules = overview.value.rules.map(item => item.id === rule.id ? {
          ...item,
          enabled,
          runtimeStatus: enabled ? 'pending' : 'disabled',
          lastError: '',
          localConnectionCount: 0,
          upstreamConnectionCount: 0,
        } : item)
        overview.value.enabledCount = overview.value.rules.filter(item => item.enabled).length
        configurationConflict.value = false
      } else {
        await handleRevisionConflict(msg)
      }
    } finally {
      rowBusyId.value = 0
    }
  }

  const moveRule = async (rule: ReverseProxyRule, direction: -1 | 1) => {
    if (actionsDisabled.value) return
    const index = overview.value.rules.findIndex(item => item.id === rule.id)
    if (index < 0) return
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= overview.value.rules.length) return
    const payload = { id: rule.id, direction, expectedRevision: overview.value.revision }
    const previousOrder = overview.value.rules[index].listOrder
    const adjacentOrder = overview.value.rules[nextIndex].listOrder
    rowBusyId.value = rule.id
    try {
      const msg = await runMutation(() => HttpUtils.post('api/reverse-proxy-rule-move', payload, {
        headers: { 'Content-Type': 'application/json' },
      }), false)
      if (msg?.success) {
        const result = msg.obj ?? {}
        overview.value.revision = asNumber(result.revision, overview.value.revision)
        const rules = [...overview.value.rules]
        const current = { ...rules[index], listOrder: adjacentOrder }
        const adjacent = { ...rules[nextIndex], listOrder: previousOrder }
        rules[index] = adjacent
        rules[nextIndex] = current
        overview.value.rules = rules
        configurationConflict.value = false
        if (previousOrder === adjacentOrder) void fetchOverview(true)
        push.success({ duration: 3200, message: reverseProxyCopy.reorderSaved })
      } else {
        await handleRevisionConflict(msg)
      }
    } finally {
      rowBusyId.value = 0
    }
  }

  const filteredRules = computed(() => {
    const keyword = searchText.value.trim().toLowerCase()
    if (!keyword) return overview.value.rules
    return overview.value.rules.filter((rule) => {
      return [
        rule.name,
        rule.pathPrefix,
        rule.listenDnsPath,
        rule.remark,
        rule.listenProtocol,
        rule.targetProtocol,
        listenMatchDisplay(rule),
        joinDisplay(rule.targetAddresses),
        rule.targetDnsPath,
        rule.targetPath,
      ].some(item => (item ?? '').toLowerCase().includes(keyword))
    })
  })

  const lastSyncLabel = computed(() => formatTimestamp(overview.value.lastSyncAt))
  const dialogTitle = computed(() => editingRule.value.id > 0 ? reverseProxyCopy.editTitle : reverseProxyCopy.createTitle)
  const selectedCertificates = computed(() => {
    const ids = sortCertificateIDsByOptionOrder(
      normalizeNumberList(editingRule.value.certificateRecordIds),
      overview.value.certificates,
    )
    const byID = new Map<number, ReverseProxyCertificateOption>()
    overview.value.certificates.forEach((item) => {
      byID.set(item.id, item)
    })
    const selected: ReverseProxyCertificateOption[] = []
    ids.forEach((id) => {
      const cert = byID.get(id)
      if (cert) selected.push(cert)
    })
    return selected
  })
  const currentCertificateHints = computed(() => {
    const certs = selectedCertificates.value
    if (certs.length === 0) return []
    const matches = splitDomainTokens(editingRule.value.hostsText)
    const certNames = certs
      .flatMap(cert => [cert.mainDomain, ...(cert.domains ?? [])])
      .map(item => item.trim().toLowerCase())
      .filter(Boolean)
    const wildcardMatch = (pattern: string, host: string) => {
      const normalizedPattern = pattern.trim().toLowerCase()
      const normalizedHost = host.trim().toLowerCase()
      if (normalizedPattern === normalizedHost) return true
      if (!normalizedPattern.startsWith('*.')) return false
      const suffix = normalizedPattern.slice(2)
      if (!suffix || !normalizedHost.endsWith(`.${suffix}`)) return false
      const remain = normalizedHost.slice(0, normalizedHost.length - suffix.length - 1)
      return remain.length > 0 && !remain.includes('.')
    }
    const hints: string[] = []
    matches.forEach((match) => {
      if (!certNames.some(name => wildcardMatch(name, match) || wildcardMatch(match, name))) {
        hints.push(`证书未覆盖域名: ${match}`)
      }
    })
    return hints
  })
  const targetIsHTTPS = computed(() => {
    const value = editingRule.value.targetProtocol.trim().toLowerCase()
    if (protocolIsDNS(value)) return protocolNeedsCertificates(value)
    if (value === 'ws') return false
    if (value === 'wss') return true
    return protocolIsTLS(editingRule.value.targetProtocol)
  })
  const listenIsHTTPS = computed(() => {
    const value = editingRule.value.listenProtocol.trim().toLowerCase()
    if (protocolIsDNS(value)) return protocolNeedsCertificates(value)
    if (value === 'ws') return false
    if (value === 'wss') return true
    return protocolIsTLS(editingRule.value.listenProtocol)
  })
  const ipCertificateRoutingHint = computed(() => {
    if (!listenIsHTTPS.value || selectedCertificates.value.length === 0) return ''
    return reverseProxyCopy.ipCertificateRoutingHint
  })
  const targetVersionConfigurable = computed(() => {
    return editingRule.value.targetProtocol.trim().toLowerCase() === 'https'
  })
  const listenCanAdvertiseHTTP3 = computed(() => {
    const value = editingRule.value.listenProtocol.trim().toLowerCase()
    return value === 'https' && editingRule.value.listenHttpVersionStrategy === 'h2_h3'
  })
  const listenIsDNS = computed(() => protocolIsDNS(editingRule.value.listenProtocol))
  const listenIsPlainDNS = computed(() => editingRule.value.listenProtocol === 'dns_udp' || editingRule.value.listenProtocol === 'dns_tcp')
  const targetIsDNS = computed(() => protocolIsDNS(editingRule.value.targetProtocol))
  const listenCompressionVisible = computed(() => protocolSupportsCompression(editingRule.value.listenProtocol))
  const targetCompressionVisible = computed(() => protocolSupportsCompression(editingRule.value.targetProtocol))
  const hasPreviewProtocol = computed(() => {
    return false
  })
  const listenProtocolBehavior = computed(() => {
    const value = editingRule.value.listenProtocol
    if (value === 'dns_doh') return reverseProxyCopy.listenModeDNSDoH
    if (value === 'dns_doh3') return reverseProxyCopy.listenModeDNSDoHH3
    if (value === 'dns_doq') return reverseProxyCopy.listenModeDNSDoQ
    if (value === 'dns_dot') return reverseProxyCopy.listenModeDNSDoT
    if (value === 'dns_udp') return reverseProxyCopy.listenModeDNSUDP
    if (value === 'dns_tcp') return reverseProxyCopy.listenModeDNSTCP
    if (value === 'ws') return 'WS：仅监听明文 WebSocket（ws://）。'
    if (value === 'wss') return 'WSS：通过 TLS 监听传统 HTTP/1.1 WebSocket（wss://），需绑定证书；不属于严格 H2/H3 入口。'
    if (value === 'h2') return reverseProxyCopy.listenModeH2
    if (value === 'h3') return reverseProxyCopy.listenModeH3
    if (value === 'https') return reverseProxyCopy.listenModeHTTPS
    return reverseProxyCopy.listenModeHTTP
  })
  const targetProtocolBehavior = computed(() => {
    const value = editingRule.value.targetProtocol
    if (value === 'dns_doh') return reverseProxyCopy.targetModeDNSDoH
    if (value === 'dns_doh3') return reverseProxyCopy.targetModeDNSDoHH3
    if (value === 'dns_doq') return reverseProxyCopy.targetModeDNSDoQ
    if (value === 'dns_dot') return reverseProxyCopy.targetModeDNSDoT
    if (value === 'dns_udp') return reverseProxyCopy.targetModeDNSUDP
    if (value === 'dns_tcp') return reverseProxyCopy.targetModeDNSTCP
    if (value === 'ws') return 'WS：向上游发起明文 WebSocket（ws://）。'
    if (value === 'wss') return 'WSS：向上游发起 TLS WebSocket（wss://）。'
    if (value === 'h2') return reverseProxyCopy.targetModeH2
    if (value === 'h3') return reverseProxyCopy.targetModeH3
    if (value === 'https') return reverseProxyCopy.targetModeHTTPS
    return reverseProxyCopy.targetModeHTTP
  })

  const stopPolling = () => {
    if (pollTimer.value != null) {
      window.clearTimeout(pollTimer.value)
      pollTimer.value = null
    }
  }

  const schedulePolling = (delay = 10000) => {
    stopPolling()
    if (!props.active) return
    if (!hasLoaded.value || Boolean(loadError.value) || !overview.value.available) return
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
    pollTimer.value = window.setTimeout(async () => {
      pollTimer.value = null
      const msg = await fetchRuntime()
      schedulePolling(msg?.success ? 10000 : 30000)
    }, delay)
  }

  const startPolling = () => schedulePolling()

  const handleVisibilityChange = () => {
    if (document.visibilityState === 'visible' && props.active) {
      void fetchOverview(hasLoaded.value).then(() => startPolling())
    } else {
      stopPolling()
    }
  }

  watch(() => props.active, (active) => {
    if (active) {
      void fetchOverview(hasLoaded.value).then(() => startPolling())
    } else {
      stopPolling()
    }
  })

  const defaultProtocolPort = (value: string) => {
    const normalized = value.trim().toLowerCase()
    if (normalized === 'http' || normalized === 'ws') return 80
    if (normalized === 'dns_udp' || normalized === 'dns_tcp') return 53
    if (normalized === 'dns_dot' || normalized === 'dns_doq') return 853
    return 443
  }

  const changeListenProtocol = (nextValue: unknown) => {
    if (typeof nextValue !== 'string') return
    const value = nextValue.trim().toLowerCase()
    const previous = editingRule.value.listenProtocol
    if (!value || value === previous) return
    const enteringCompressionSupportedProtocol = !protocolSupportsCompression(previous) && protocolSupportsCompression(value)
    const previousPort = defaultProtocolPort(previous)
    const shouldApplyDefaultPort = !editingRule.value.listenPort || editingRule.value.listenPort === previousPort
    editingRule.value.listenProtocol = value as ReverseProxyRuleForm['listenProtocol']
    editingRule.value.listenHttpVersionStrategy = mapListenProtocolToBackend(value).listenHttpVersionStrategy
    if (enteringCompressionSupportedProtocol
      && !editingRule.value.listenCompressionEnabled
      && editingRule.value.listenCompressionAlgorithms.length === 0) {
      editingRule.value.listenCompressionEnabled = true
      editingRule.value.listenCompressionAlgorithms = [...reverseProxyCompressionOrder]
    }
    if (protocolIsDNS(value)) {
      editingRule.value.pathPrefix = ''
      editingRule.value.apiPassthrough = true
      editingRule.value.maxConcurrentConnections = 0
      editingRule.value.maxConcurrentRequests = 0
      editingRule.value.upstreamMaxConnections = 0
      editingRule.value.upstreamMaxIdleConnections = 0
      if (dnsProtocolUsesPath(value) && !editingRule.value.listenDnsPath.trim()) {
        editingRule.value.listenDnsPath = '/dns-query'
      }
      if (!dnsProtocolUsesPath(value)) {
        editingRule.value.listenDnsPath = ''
      }
      if (value === 'dns_udp' || value === 'dns_tcp') {
        editingRule.value.hostsText = ''
        editingRule.value.certificateRecordIds = []
      }
      if (shouldApplyDefaultPort) editingRule.value.listenPort = defaultProtocolPort(value)
      return
    }
    if (value !== 'https') {
      editingRule.value.advertiseHttp3 = false
    }
    editingRule.value.ednsEnabled = false
    editingRule.value.ednsMode = 'auto'
    editingRule.value.ednsCustomIp = ''
    editingRule.value.ednsClientSubnetPolicy = 'client_ip'
    editingRule.value.disableIpv4Answer = false
    editingRule.value.disableIpv6Answer = false
    editingRule.value.listenDnsPath = ''
    if (protocolIsHTTP(value)) {
      editingRule.value.certificateRecordIds = []
    }
    if (shouldApplyDefaultPort) editingRule.value.listenPort = defaultProtocolPort(value)
  }

  watch(
    () => [overview.value.certificates, editingRule.value.certificateRecordIds] as const,
    () => {
      const sorted = sortCertificateIDsByOptionOrder(
        normalizeNumberList(editingRule.value.certificateRecordIds),
        overview.value.certificates,
      )
      const current = normalizeNumberList(editingRule.value.certificateRecordIds)
      if (sorted.length === current.length && sorted.every((id, index) => id === current[index])) {
        return
      }
      editingRule.value.certificateRecordIds = sorted
    },
    { deep: true },
  )

  const changeTargetProtocol = (nextValue: unknown) => {
    if (typeof nextValue !== 'string') return
    const value = nextValue.trim().toLowerCase()
    const previous = editingRule.value.targetProtocol
    if (!value || value === previous) return
    const enteringCompressionSupportedProtocol = !protocolSupportsCompression(previous) && protocolSupportsCompression(value)
    const previousPort = defaultProtocolPort(previous)
    const shouldApplyDefaultPort = !editingRule.value.targetPort || editingRule.value.targetPort === previousPort
    editingRule.value.targetProtocol = value as ReverseProxyRuleForm['targetProtocol']
    if (enteringCompressionSupportedProtocol
      && !editingRule.value.targetCompressionEnabled
      && editingRule.value.targetCompressionAlgorithms.length === 0) {
      editingRule.value.targetCompressionEnabled = true
      editingRule.value.targetCompressionAlgorithms = [...reverseProxyCompressionOrder]
    }
    if (protocolIsDNS(value)) {
      editingRule.value.httpVersionStrategy = ''
      editingRule.value.upstreamTlsVerify = protocolIsTLS(value)
      editingRule.value.targetPath = ''
      editingRule.value.maxConcurrentRequests = 0
      editingRule.value.upstreamMaxConnections = 0
      editingRule.value.upstreamMaxIdleConnections = 0
      if (dnsProtocolUsesPath(value) && !editingRule.value.targetDnsPath.trim()) {
        editingRule.value.targetDnsPath = '/dns-query'
      }
      if (!dnsProtocolUsesPath(value)) {
        editingRule.value.targetDnsPath = ''
      }
      if (shouldApplyDefaultPort) editingRule.value.targetPort = defaultProtocolPort(value)
      return
    }
    editingRule.value.fallbackDnsUpstreams = ''
    editingRule.value.dnsCacheEnabled = false
    editingRule.value.targetDnsPath = ''
    if (value === 'http') {
      editingRule.value.httpVersionStrategy = ''
      editingRule.value.upstreamTlsVerify = false
    } else if (value === 'ws') {
      editingRule.value.httpVersionStrategy = ''
      editingRule.value.upstreamTlsVerify = false
    } else if (value === 'wss') {
      editingRule.value.httpVersionStrategy = ''
      editingRule.value.upstreamTlsVerify = true
    } else if (value === 'h2') {
      editingRule.value.httpVersionStrategy = 'h2_only'
      editingRule.value.upstreamTlsVerify = true
    } else if (value === 'h3') {
      editingRule.value.httpVersionStrategy = 'h3_only'
      editingRule.value.upstreamTlsVerify = true
    } else {
      const normalized = normalizeTargetHTTPVersionStrategy(editingRule.value.httpVersionStrategy)
      if (!normalized || normalized === 'h2_only' || normalized === 'h3_only') {
        editingRule.value.httpVersionStrategy = 'prefer_h2'
      }
      editingRule.value.upstreamTlsVerify = true
    }
    if (shouldApplyDefaultPort) editingRule.value.targetPort = defaultProtocolPort(value)
  }

  watch(() => editingRule.value.pathPrefix, (value) => {
    if (!value.trim()) return
    editingRule.value.pathPrefix = normalizePathInput(value, true)
  })

  watch(() => editingRule.value.ednsMode, (value) => {
    if (value !== 'custom') return
    normalizeEDNSCustomIPInForm(editingRule.value)
  })

  onMounted(() => {
    if (props.active) {
      void fetchOverview().then(() => startPolling())
    }
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', handleVisibilityChange)
    }
  })

  onBeforeUnmount(() => {
    stopPolling()
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  })

  return {
    loading,
    refreshing,
    saving,
    savingResources,
    mutationBusy,
    hasLoaded,
    loadError,
    actionsDisabled,
    dialogVisible,
    resourceDialogVisible,
    rowBusyId,
    searchText,
    overview,
    runtimeUsage,
    editingResources,
    configurationConflict,
    editingRule,
    filteredRules,
    lastSyncLabel,
    dialogTitle,
    selectedCertificates,
    currentCertificateHints,
    ipCertificateRoutingHint,
    targetIsHTTPS,
    listenIsHTTPS,
    listenIsDNS,
    listenIsPlainDNS,
    targetIsDNS,
    listenCompressionVisible,
    targetCompressionVisible,
    targetVersionConfigurable,
    listenCanAdvertiseHTTP3,
    hasPreviewProtocol,
    listenProtocolBehavior,
    targetProtocolBehavior,
    fetchOverview,
    fetchRuntime,
    refreshOverview,
    openResourceDialog,
    saveResources,
    openRuleDialog,
    changeListenProtocol,
    changeTargetProtocol,
    normalizeCustomEDNSInput,
    normalizeRuleTextInputs,
    saveRule,
    removeRule,
    toggleRule,
    moveRule,
  }
}
