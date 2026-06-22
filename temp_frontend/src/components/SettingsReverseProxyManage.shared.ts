import HttpUtils, { type Msg } from '@/plugins/httputil'
import type {
  ReverseProxyCertificateOption,
  ReverseProxyOverview,
  ReverseProxyRule,
  ReverseProxyRuleForm,
} from '@/types/reverseProxy'
import { push } from 'notivue'
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
  targetPort: '目标端口',
  targetPath: '目标基础路径',
  targetDnsPath: '目标 DNS URL 路径',
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
  listenPanelHint: 'Go 运行时会在该端口同时监听 IPv4 与 IPv6；这里填写的是域名命中条件，HTTP 校验域名型 Host，HTTPS/H2/H3 校验域名型 SNI；通过 IP 访问或无 SNI 访问始终允许；URL 路径按完整路径段严格匹配。',
  targetPanelHint: '目标支持 HTTP / HTTPS / DNS。多个目标会按填写顺序依次尝试；DNS 目标之间也会按顺序回退。',
  tlsPanelHint: '本地协议为 HTTPS、WSS、DoH、DoH3、DoT、DoQ 时必须绑定至少一张证书。',
  certificateRequired: '请选择至少一张 TLS 监听证书',
  certificateBound: '已绑定证书',
  currentHTTPNoCert: '当前监听协议无需证书',
  targetHTTPMode: 'HTTP 目标',
  ruleEnabled: '已启用',
  ruleDisabled: '已停用',
  pathRequired: '路径不能为空',
  listenMatchRequired: '域名可留空',
  listenIPLiteralNotAllowed: '这里只能填写域名，不能填写 IP；通过 IP 访问默认允许',
  listenPortInlineNotAllowed: '域名里不能带端口，请把端口填在监听端口',
  targetAddressInlineNotAllowed: '目标地址 / 域名 里不能带端口，请把端口填在目标端口',
  targetRequired: '请填写至少一个目标地址',
  certRequiredSave: 'TLS 监听必须至少选择一张证书',
  dnsPathRequired: '当前 DNS 协议必须填写 URL 路径',
  dnsProtocolPairRequired: 'DNS 反代要求本地协议和目标协议都使用 DNS',
  dnsHostUnused: 'DNS 反代不使用域名命中条件',
  dnsHttpFieldUnused: 'DNS 反代不使用 HTTP 路径改写与 API 透传',
  listenModeHTTP: 'HTTP：仅监听明文 HTTP 请求。',
  listenModeHTTPS: 'HTTPS：同时监听 TCP(H2) 与 UDP(H3)，浏览器按标准协商决定使用哪种版本。',
  listenModeH2: 'H2：仅监听 TCP，仅提供 HTTPS/HTTP2。',
  listenModeH3: 'H3：仅监听 UDP，仅提供 HTTPS/HTTP3。',
  listenModeDNSDoH: 'DoH（DNS）：同时监听 H2 与 H3 的 DNS over HTTPS，可自定义端口和 URL 路径。',
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
  listenIpLocalHint: '填写后仅约束域名型 Host / SNI；通过 IP 访问或无 SNI 访问仍允许。留空时不做域名命中限制。',
  targetPathRewriteHint: '目标基础路径会作为上游前缀，例如填 /api 后，请求 /foo 会转发到 /api/foo。',
  apiPassthroughHint: '开启后不改写响应正文，适合 AI、SSE 与 API 直通，避免流式内容被缓冲或替换；响应头仍按反代规则处理。',
  runtimeHint: '当请求没有命中任何规则时，Go 运行时会拒绝该请求。',
  pathPrefixStrictHint: '填写 888 会保存为 /888；只有 /888 或 /888/后续目标路径会命中，/8888 不会命中。',
}
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
  { title: 'IPv4 only', value: 'ipv4_only' },
  { title: 'IPv6 only', value: 'ipv6_only' },
  { title: 'Prefer IPv4', value: 'prefer_ipv4' },
  { title: 'Prefer IPv6', value: 'prefer_ipv6' },
] as const

export const httpVersionItems = [
  { title: 'Dual required (Prefer H3)', value: 'dual_required_prefer_h3' },
  { title: 'H2 only', value: 'h2_only' },
  { title: 'H3 only', value: 'h3_only' },
  { title: 'Prefer H2', value: 'prefer_h2' },
  { title: 'Prefer H3', value: 'prefer_h3' },
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
  available: true,
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

export const createEmptyReverseProxyRuleForm = (): ReverseProxyRuleForm => ({
  id: 0,
  displayId: 0,
  name: '',
  enabled: true,
  listenProtocol: 'http',
  listenPort: 80,
  hostsText: '',
  pathPrefix: '',
  listenDnsPath: '/dns-query',
  targetProtocol: 'http',
  targetAddressesText: '',
  targetPort: 80,
  targetPath: '',
  targetDnsPath: '/dns-query',
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
  upstreamTlsVerify: false,
  apiPassthrough: false,
  remark: '',
})

const asNumber = (value: unknown, fallback = 0) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

const asString = (value: unknown, fallback = '') => {
  if (typeof value === 'string') return value
  if (value == null) return fallback
  return String(value)
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
  return value.map((raw) => {
    const item = raw as Partial<ReverseProxyCertificateOption>
    return {
      id: asNumber(item.id),
      displayId: asNumber(item.displayId),
      mainDomain: asString(item.mainDomain),
      domains: normalizeStringList(item.domains),
      notAfter: asNumber(item.notAfter),
      status: asString(item.status),
    }
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
    listenIP: asString(item.listenIP),
    listenIPs: normalizeStringList(item.listenIPs),
    listenPort: asNumber(item.listenPort),
    hosts: normalizeStringList(item.hosts),
    pathPrefix: asString(item.pathPrefix),
    listenDnsPath: asString(item.listenDnsPath),
    targetProtocol: deriveTargetProtocolForForm(targetProtocolRaw, httpVersionStrategy, targetProtocolAliasRaw),
    targetAddresses: normalizeStringList(item.targetAddresses),
    targetPort: asNumber(item.targetPort),
    targetPath: asString(item.targetPath),
    targetDnsPath: asString(item.targetDnsPath),
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
    apiPassthrough: asBoolean(item.apiPassthrough, false),
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

const normalizeOverview = (value: unknown): ReverseProxyOverview => {
  const item = (value ?? {}) as Partial<ReverseProxyOverview>
  return {
    available: asBoolean(item.available, true),
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

export const formatTimestamp = (value: number) => {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
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

export const listenMatchDisplay = (item: ReverseProxyRule) => joinDisplay([
  ...(item.listenIPs ?? []),
  ...(item.hosts ?? []),
])

export const statusColor = (value: string) => {
  const normalized = value.trim().toLowerCase()
  if (normalized === 'running') return 'success'
  if (normalized === 'pending') return 'info'
  if (normalized === 'upstream_error' || normalized === 'proxy_error') return 'warning'
  return 'grey'
}

const normalizePathInput = (value: string, allowEmpty: boolean) => {
  const trimmed = value.trim()
  if (!trimmed) {
    return allowEmpty ? '' : '/'
  }
  if (trimmed.startsWith('/')) return trimmed
  return `/${trimmed}`
}

const normalizeListTextInput = (value: string) => splitInputTokens(value).join(', ')

const trimReverseProxyRuleFormText = (form: ReverseProxyRuleForm) => {
  form.name = form.name.trim()
  form.hostsText = normalizeListTextInput(form.hostsText)
  form.pathPrefix = form.pathPrefix.trim()
  form.targetAddressesText = normalizeListTextInput(form.targetAddressesText)
  form.targetPath = form.targetPath.trim()
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
    return { listenProtocol: 'https', listenHttpVersionStrategy: 'h2_h3' }
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
    return { targetProtocol: 'https', httpVersionStrategy: 'prefer_h2' }
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
    hostsText: normalizeStringList(rule?.hosts ?? []).join(', '),
    pathPrefix: rule?.pathPrefix ?? '',
    listenDnsPath: rule?.listenDnsPath ?? (dnsProtocolUsesPath(listenProtocol) ? '/dns-query' : ''),
    targetProtocol,
    targetAddressesText: (rule?.targetAddresses ?? []).join(', '),
    targetPort: rule?.targetPort ?? 80,
    targetPath: rule?.targetPath ?? '',
    targetDnsPath: rule?.targetDnsPath ?? (dnsProtocolUsesPath(targetProtocol) ? '/dns-query' : ''),
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
    apiPassthrough: rule?.apiPassthrough ?? false,
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
  const hostsText = normalizeListTextInput(form.hostsText)
  const pathPrefix = form.pathPrefix.trim()
  const targetAddressesText = normalizeListTextInput(form.targetAddressesText)
  const targetPath = form.targetPath.trim()
  const listenDnsPath = form.listenDnsPath.trim()
  const targetDnsPath = form.targetDnsPath.trim()
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
  return {
    id: form.id,
    name,
    enabled: form.enabled,
    listenProtocol: listenProtocol.listenProtocol,
    listenProtocolAlias: listenProtocol.listenProtocolAlias || listenProtocolAlias,
    listenPort: asNumber(form.listenPort),
    listenIPs: '',
    hosts: listenNames.join(', '),
    pathPrefix: normalizePathInput(pathPrefix, true),
    listenDnsPath: dnsProtocolUsesPath(form.listenProtocol) ? normalizePathInput(listenDnsPath, true) : '',
    targetProtocol: targetProtocol.targetProtocol,
    targetProtocolAlias: targetProtocol.targetProtocolAlias || targetProtocolAlias,
    targetAddresses: targetAddressesText,
    targetPort: asNumber(form.targetPort),
    targetPath: normalizePathInput(targetPath, true),
    targetDnsPath: dnsProtocolUsesPath(form.targetProtocol) ? normalizePathInput(targetDnsPath, true) : '',
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
    apiPassthrough: form.apiPassthrough,
    remark,
  }
}

export function useReverseProxyManage(props: { active?: boolean }) {
  const loading = ref(false)
  const refreshing = ref(false)
  const saving = ref(false)
  const dialogVisible = ref(false)
  const rowBusyId = ref(0)
  const searchText = ref('')
  const overview = ref<ReverseProxyOverview>(emptyOverview())
  const editingRule = ref<ReverseProxyRuleForm>(createEmptyReverseProxyRuleForm())
  const pollTimer = ref<number | null>(null)
  const overviewRequest = ref<Promise<Msg> | null>(null)
  let latestOverviewRequestId = 0

  const applyOverview = (raw: unknown) => {
    overview.value = normalizeOverview(raw)
  }

  const fetchOverview = async (silent = false) => {
    if (overviewRequest.value) {
      return overviewRequest.value
    }
    if (!silent) loading.value = true
    const requestId = ++latestOverviewRequestId
    const request = (async () => {
      const msg = await HttpUtils.get('api/reverse-proxy-overview')
      if (msg.success && requestId === latestOverviewRequestId) {
        applyOverview(msg.obj)
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
      await fetchOverview(true)
    } finally {
      refreshing.value = false
    }
  }

  const openRuleDialog = (rule?: ReverseProxyRule) => {
    editingRule.value = mapRuleToForm(rule)
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

  const saveRule = async () => {
    if (saving.value) return
    trimReverseProxyRuleFormText(editingRule.value)
    normalizeEDNSCustomIPInForm(editingRule.value)
    if (protocolIsDNS(editingRule.value.listenProtocol) !== protocolIsDNS(editingRule.value.targetProtocol)) {
      push.warning({ duration: 4000, message: reverseProxyCopy.dnsProtocolPairRequired })
      return
    }
    if (!protocolIsDNS(editingRule.value.listenProtocol) && splitInputTokens(editingRule.value.hostsText).some(isIPLiteral)) {
      push.warning({ duration: 4000, message: reverseProxyCopy.listenIPLiteralNotAllowed })
      return
    }
    if (!protocolIsDNS(editingRule.value.listenProtocol) && splitInputTokens(editingRule.value.hostsText).some(hasExplicitPort)) {
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

    saving.value = true
    try {
      const msg = await HttpUtils.post(
        'api/reverse-proxy-rule',
        buildReverseProxyPayload(editingRule.value, overview.value.certificates),
        {
          headers: {
            'Content-Type': 'application/json',
          },
        },
      )
      if (msg.success) {
        applyOverview(msg.obj)
        dialogVisible.value = false
        push.success({
          duration: 4000,
          message: editingRule.value.id > 0 ? reverseProxyCopy.saveUpdated : reverseProxyCopy.saveCreated,
        })
      }
    } finally {
      saving.value = false
    }
  }

  const removeRule = async (rule: ReverseProxyRule) => {
    if (rowBusyId.value === rule.id) return
    const confirmed = window.confirm(reverseProxyCopy.deleteConfirm.replace('{name}', rule.name || `#${rule.displayId}`))
    if (!confirmed) return
    rowBusyId.value = rule.id
    try {
      const msg = await HttpUtils.post('api/reverse-proxy-rule-delete', { id: rule.id }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success) {
        applyOverview(msg.obj)
      }
    } finally {
      rowBusyId.value = 0
    }
  }

  const toggleRule = async (rule: ReverseProxyRule, enabled: boolean) => {
    if (rowBusyId.value === rule.id) return
    rowBusyId.value = rule.id
    try {
      const msg = await HttpUtils.post('api/reverse-proxy-rule', {
        ...buildReverseProxyPayload(mapRuleToForm(rule), overview.value.certificates),
        enabled,
      }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success) {
        applyOverview(msg.obj)
      }
    } finally {
      rowBusyId.value = 0
    }
  }

  const reorderRules = async (ids: number[]) => {
    const msg = await HttpUtils.post('api/reverse-proxy-rule-reorder', { ids }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success) {
      applyOverview(msg.obj)
      push.success({
        duration: 3200,
        message: reverseProxyCopy.reorderSaved,
      })
    }
  }

  const moveRule = async (rule: ReverseProxyRule, direction: -1 | 1) => {
    if (rowBusyId.value === rule.id) return
    const ids = overview.value.rules.map(item => item.id)
    const index = ids.findIndex(id => id === rule.id)
    if (index < 0) return
    const nextIndex = index + direction
    if (nextIndex < 0 || nextIndex >= ids.length) return
    const swapped = [...ids]
    const temp = swapped[index]
    swapped[index] = swapped[nextIndex]
    swapped[nextIndex] = temp
    rowBusyId.value = rule.id
    try {
      await reorderRules(swapped)
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
    const hasIPSANCert = certs.some(cert => [cert.mainDomain, ...(cert.domains ?? [])].some(item => isIPLiteral(item)))
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
      if (hasIPSANCert) {
        return
      }
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
  const targetVersionConfigurable = computed(() => !protocolIsDNS(editingRule.value.targetProtocol) && normalizeVirtualProtocol(editingRule.value.targetProtocol) === 'https')
  const listenIsDNS = computed(() => protocolIsDNS(editingRule.value.listenProtocol))
  const targetIsDNS = computed(() => protocolIsDNS(editingRule.value.targetProtocol))
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
    if (value === 'wss') return 'WSS：通过 TLS 监听 WebSocket（wss://），需绑定证书。'
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
      window.clearInterval(pollTimer.value)
      pollTimer.value = null
    }
  }

  const startPolling = () => {
    stopPolling()
    if (!props.active) return
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
    pollTimer.value = window.setInterval(() => {
      void fetchOverview(true)
    }, 4000)
  }

  const handleVisibilityChange = () => {
    if (document.visibilityState === 'visible') {
      void fetchOverview(true)
      startPolling()
    } else {
      stopPolling()
    }
  }

  watch(() => props.active, (active) => {
    if (active) {
      void fetchOverview(true)
      startPolling()
    } else {
      stopPolling()
    }
  })

  watch(() => editingRule.value.listenProtocol, (value) => {
    editingRule.value.listenHttpVersionStrategy = mapListenProtocolToBackend(value).listenHttpVersionStrategy
    if (protocolIsDNS(value)) {
      editingRule.value.hostsText = ''
      editingRule.value.pathPrefix = ''
      editingRule.value.apiPassthrough = true
      if (dnsProtocolUsesPath(value) && !editingRule.value.listenDnsPath.trim()) {
        editingRule.value.listenDnsPath = '/dns-query'
      }
      if (!dnsProtocolUsesPath(value)) {
        editingRule.value.listenDnsPath = ''
      }
      if (value === 'dns_udp' || value === 'dns_tcp') {
        if (!editingRule.value.listenPort || editingRule.value.listenPort === 443 || editingRule.value.listenPort === 853) {
          editingRule.value.listenPort = 53
        }
        editingRule.value.certificateRecordIds = []
      } else if (value === 'dns_doh' || value === 'dns_doh3') {
        if (!editingRule.value.listenPort || editingRule.value.listenPort === 53 || editingRule.value.listenPort === 853) {
          editingRule.value.listenPort = 443
        }
      } else if (!editingRule.value.listenPort || editingRule.value.listenPort === 53 || editingRule.value.listenPort === 443) {
        editingRule.value.listenPort = 853
      }
      return
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
      if (!editingRule.value.listenPort || editingRule.value.listenPort === 443) {
        editingRule.value.listenPort = 80
      }
    } else if (!editingRule.value.listenPort || editingRule.value.listenPort === 80) {
      editingRule.value.listenPort = 443
    }
  })

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

  watch(() => editingRule.value.targetProtocol, (value) => {
    if (protocolIsDNS(value)) {
      editingRule.value.httpVersionStrategy = ''
      editingRule.value.upstreamTlsVerify = protocolIsTLS(value)
      editingRule.value.targetPath = ''
      if (dnsProtocolUsesPath(value) && !editingRule.value.targetDnsPath.trim()) {
        editingRule.value.targetDnsPath = '/dns-query'
      }
      if (!dnsProtocolUsesPath(value)) {
        editingRule.value.targetDnsPath = ''
      }
      if (value === 'dns_udp' || value === 'dns_tcp') {
        if (!editingRule.value.targetPort || editingRule.value.targetPort === 443 || editingRule.value.targetPort === 853) {
          editingRule.value.targetPort = 53
        }
      } else if (value === 'dns_doh' || value === 'dns_doh3') {
        if (!editingRule.value.targetPort || editingRule.value.targetPort === 53 || editingRule.value.targetPort === 853) {
          editingRule.value.targetPort = 443
        }
      } else if (!editingRule.value.targetPort || editingRule.value.targetPort === 53 || editingRule.value.targetPort === 443) {
        editingRule.value.targetPort = 853
      }
      return
    }
    editingRule.value.targetDnsPath = ''
    if (value === 'http') {
      editingRule.value.httpVersionStrategy = ''
      editingRule.value.upstreamTlsVerify = false
      if (!editingRule.value.targetPort || editingRule.value.targetPort === 443) {
        editingRule.value.targetPort = 80
      }
    } else if (value === 'ws') {
      editingRule.value.httpVersionStrategy = ''
      editingRule.value.upstreamTlsVerify = false
      if (!editingRule.value.targetPort || editingRule.value.targetPort === 443) {
        editingRule.value.targetPort = 80
      }
    } else if (value === 'wss') {
      editingRule.value.httpVersionStrategy = 'prefer_h2'
      editingRule.value.upstreamTlsVerify = true
      if (!editingRule.value.targetPort || editingRule.value.targetPort === 80) {
        editingRule.value.targetPort = 443
      }
    } else if (value === 'h2') {
      editingRule.value.httpVersionStrategy = 'h2_only'
      editingRule.value.upstreamTlsVerify = true
      if (!editingRule.value.targetPort || editingRule.value.targetPort === 80) {
        editingRule.value.targetPort = 443
      }
    } else if (value === 'h3') {
      editingRule.value.httpVersionStrategy = 'h3_only'
      editingRule.value.upstreamTlsVerify = true
      if (!editingRule.value.targetPort || editingRule.value.targetPort === 80) {
        editingRule.value.targetPort = 443
      }
    } else {
      const normalized = normalizeTargetHTTPVersionStrategy(editingRule.value.httpVersionStrategy)
      if (!normalized || normalized === 'h2_only' || normalized === 'h3_only') {
        editingRule.value.httpVersionStrategy = 'prefer_h2'
      }
      editingRule.value.upstreamTlsVerify = true
      if (!editingRule.value.targetPort || editingRule.value.targetPort === 80) {
        editingRule.value.targetPort = 443
      }
    }
  })

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
      void fetchOverview()
    }
    startPolling()
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
    dialogVisible,
    rowBusyId,
    searchText,
    overview,
    editingRule,
    filteredRules,
    lastSyncLabel,
    dialogTitle,
    selectedCertificates,
    currentCertificateHints,
    targetIsHTTPS,
    listenIsHTTPS,
    listenIsDNS,
    targetIsDNS,
    targetVersionConfigurable,
    hasPreviewProtocol,
    listenProtocolBehavior,
    targetProtocolBehavior,
    fetchOverview,
    refreshOverview,
    openRuleDialog,
    normalizeCustomEDNSInput,
    saveRule,
    removeRule,
    toggleRule,
    moveRule,
  }
}



