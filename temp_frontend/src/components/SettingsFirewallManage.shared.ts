import HttpUtils, { type Msg } from '@/plugins/httputil'
import { confirm } from '@/plugins/confirm'
import { i18n } from '@/locales'
import { formatPanelDateTime } from '@/plugins/panelTime'
import { firewallGeoCountryCodeOptions, firewallGeoSourceProviderOptions } from './SettingsFirewallGeoOptions'
import { push } from 'notivue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const confirmAction = (action: string) => i18n.global.t(`confirmDialog.actions.${action}`)

export type FirewallRule = {
  id: number
  name: string
  description: string
  enabled: boolean
  origin: string
  systemKey: string
  temporaryType: string
  temporaryExpireAt: number
  family: string
  protocol: string
  portSpec: string
  sourceSpec: string
  sourceMode: string
  canEdit: boolean
  canDelete: boolean
  listenerState: FirewallRuleListenerState
}

export type FirewallListenerOwner = {
  pid: number
  name: string
  command: string
  executable: string
}

export type FirewallPortListener = {
  port: number
  protocol: string
  socketFamily: string
  stack: string
  stackSource: string
  bindAddress: string
  owners: FirewallListenerOwner[]
}

export type FirewallRuleListenerState = {
  supported: boolean
  checkedAt: number
  occupied: boolean
  listenerCount: number
  listeners: FirewallPortListener[]
  error?: string
}

export type FirewallGeoRule = {
  id: number
  name: string
  description: string
  enabled: boolean
  family: string
  protocol: string
  portSpec: string
  action: string
  countryCode: string
  sourceProviders: string[]
  customSourceUrls: string[]
  resolvedSources: string[]
  cachedFiles: string[]
  contentHash: string
  prefixCount: number
  lastRefreshAt: number
  lastRefreshError: string
}

export type FirewallSSHConfig = {
  supported: boolean
  configPath: string
  ports: number[]
  port: number
  proxyEnabled: boolean
  allowTcpForwarding: string
  permitOpen: string
  gatewayPorts: string
  error?: string
}

export type FirewallNftablesStatus = {
  supported: boolean
  installed: boolean
  autoInstallSupported: boolean
  binaryPath: string
  nftVersion: string
  kernelVersion: string
  compatibilityMode: string
  rendererSupported: boolean
  supportsJson: boolean
  supportsNamedCounters: boolean
  supportsMeters: boolean
  supportsInetNat: boolean
  supportsTransportHeader: boolean
  supportsTableComments: boolean
  capabilityError: string
  versionProbeError: string
  jsonProbeError: string
  meterProbeError: string
  layoutPending: boolean
  lastApplyError: string
  systemFamily: string
  packageManager: string
  manualCommands: string[]
  reason: string
}

export type ManagedInstallTask = {
  id: string
  state: string
  phase: string
  canCancel: boolean
  stopRequested: boolean
  deadlineExceeded: boolean
  error: string
  startedAt: number
  updatedAt: number
  deadlineAt: number
  finishedAt: number
}

export type FirewallOverview = {
  enabled: boolean
  available: boolean
  mode: string
  nftables: FirewallNftablesStatus
  lastSyncAt: number
  defaultPorts: {
    ssh: number[]
    panel: number[]
    sub: number[]
    all: number[]
    active: number[]
    sshReserved: boolean
    panelReserved: boolean
    subReserved: boolean
  }
  sshConfig: FirewallSSHConfig
  tcpActiveCount: number
  tcpSynRecvCount: number
  tcpEstablishedCount: number
  tcpAnomalyTotal: number
  udpSocketCount: number
  udpAnomalyTotal: number
  manualCount: number
  temporaryCount: number
  externalCount: number
  systemCount: number
  totalCount: number
  rules: FirewallRule[]
  geoRuleCount: number
  geoUpdateIntervalMinutes: number
  geoLastRefreshAt: number
  geoRules: FirewallGeoRule[]
  error?: string
}

export type FirewallRuleForm = {
  id: number
  name: string
  description: string
  family: string
  protocol: string
  portSpec: string
  sourceSpec: string
  sourceMode: string
}

export type FirewallGeoRuleForm = {
  id: number
  name: string
  description: string
  family: string
  protocol: string
  portSpec: string
  action: string
  countryCode: string
  sourceProviders: string[]
  customSourceUrls: string
}

export const emptyOverview = (): FirewallOverview => ({
  enabled: false,
  available: true,
  mode: 'nftables',
  nftables: {
    supported: false,
    installed: false,
    autoInstallSupported: false,
    binaryPath: '',
    nftVersion: '',
    kernelVersion: '',
    compatibilityMode: 'conservative',
    rendererSupported: false,
    supportsJson: false,
    supportsNamedCounters: false,
    supportsMeters: false,
    supportsInetNat: false,
    supportsTransportHeader: false,
    supportsTableComments: false,
    capabilityError: '',
    versionProbeError: '',
    jsonProbeError: '',
    meterProbeError: '',
    layoutPending: false,
    lastApplyError: '',
    systemFamily: '',
    packageManager: '',
    manualCommands: [],
    reason: 'not_linux',
  },
  lastSyncAt: 0,
  defaultPorts: {
    ssh: [],
    panel: [],
    sub: [],
    all: [],
    active: [],
    sshReserved: true,
    panelReserved: true,
    subReserved: true,
  },
  sshConfig: {
    supported: true,
    configPath: '',
    ports: [22],
    port: 22,
    proxyEnabled: false,
    allowTcpForwarding: '',
    permitOpen: '',
    gatewayPorts: '',
  },
  tcpActiveCount: 0,
  tcpSynRecvCount: 0,
  tcpEstablishedCount: 0,
  tcpAnomalyTotal: 0,
  udpSocketCount: 0,
  udpAnomalyTotal: 0,
  manualCount: 0,
  temporaryCount: 0,
  externalCount: 0,
  systemCount: 0,
  totalCount: 0,
  rules: [],
  geoRuleCount: 0,
  geoUpdateIntervalMinutes: 360,
  geoLastRefreshAt: 0,
  geoRules: [],
})

export const createEmptyRuleForm = (): FirewallRuleForm => ({
  id: 0,
  name: '',
  description: '',
  family: 'dual',
  protocol: 'tcp_udp',
  portSpec: '',
  sourceSpec: '',
  sourceMode: '',
})

export const createEmptyGeoRuleForm = (): FirewallGeoRuleForm => ({
  id: 0,
  name: '',
  description: '',
  family: 'dual',
  protocol: 'tcp_udp',
  portSpec: '',
  action: 'block',
  countryCode: '',
  sourceProviders: [],
  customSourceUrls: '',
})

export const headers = [
  { title: '规则', key: 'name' },
  { title: '协议', key: 'protocol', sortable: false },
  { title: '端口', key: 'portSpec', sortable: false },
  { title: '源地址策略', key: 'sourceSpec', sortable: false },
  { title: '监听状态', key: 'listenerState', sortable: false, width: 340 },
  { title: '双栈', key: 'family', sortable: false },
  { title: '来源', key: 'origin', sortable: false },
  { title: '操作', key: 'actions', sortable: false, width: 110 },
]

export const geoHeaders = [
  { title: '规则', key: 'name' },
  { title: '动作', key: 'action', sortable: false },
  { title: '协议', key: 'protocol', sortable: false },
  { title: '端口', key: 'portSpec', sortable: false },
  { title: '国家 / 来源', key: 'countryCode', sortable: false },
  { title: '状态', key: 'status', sortable: false },
  { title: '操作', key: 'actions', sortable: false, width: 110 },
]

export const familyItems = [
  { title: '双栈', value: 'dual' },
  { title: '仅 IPv4', value: 'ipv4' },
  { title: '仅 IPv6', value: 'ipv6' },
]

export const familyFilterItems = [
  { title: '全部双栈', value: 'all' },
  ...familyItems,
]

export const originFilterItems = [
  { title: '全部来源', value: 'all' },
  { title: '系统保留', value: 'system' },
  { title: '面板规则', value: 'manual' },
  { title: 'ACME 临时', value: 'temporary' },
  { title: '外部扫描', value: 'external' },
]

export const protocolItems = [
  { title: 'TCP + UDP', value: 'tcp_udp' },
  { title: 'TCP', value: 'tcp' },
  { title: 'UDP', value: 'udp' },
  { title: 'ICMP', value: 'icmp' },
  { title: 'ICMP v4', value: 'icmp_v4' },
  { title: 'ICMP v6', value: 'icmp_v6' },
]

export const sourceModeItems = [
  { title: '不设置', value: '' },
  { title: '只屏蔽', value: 'block' },
  { title: '只放行', value: 'allow' },
]

export const geoProtocolItems = protocolItems.filter(item => !['any', 'icmp', 'icmp_v4', 'icmp_v6'].includes(item.value))

export const geoActionItems = [
  { title: '只放行', value: 'allow' },
  { title: '直接阻断', value: 'block' },
]

export const geoActionFilterItems = [
  { title: '全部动作', value: 'all' },
  ...geoActionItems,
]

export const geoProviderTitleMap = firewallGeoSourceProviderOptions.reduce<Record<string, string>>((result, item) => {
  result[item.value] = item.title
  return result
}, {})

export const emptyListenerState = (): FirewallRuleListenerState => ({
  supported: true,
  checkedAt: 0,
  occupied: false,
  listenerCount: 0,
  listeners: [],
})

export const parseStrictPositiveInteger = (value: unknown): number | null => {
  const normalized = String(value ?? '').trim()
  if (!/^\d+$/.test(normalized)) return null
  const parsed = Number(normalized)
  if (!Number.isSafeInteger(parsed) || parsed <= 0) return null
  return parsed
}

export const normalizeNumberArray = (value: unknown): number[] => {
  if (!Array.isArray(value)) return []
  const result: number[] = []
  const seen = new Set<number>()
  for (const item of value) {
    const parsed = parseStrictPositiveInteger(item)
    if (parsed == null || parsed > 65535 || seen.has(parsed)) {
      continue
    }
    seen.add(parsed)
    result.push(parsed)
  }
  return result
}

export const normalizeListenerOwners = (value: unknown): FirewallListenerOwner[] => {
  if (!Array.isArray(value)) return []
  return value.map((item: any) => ({
    pid: Number.parseInt(String(item?.pid ?? 0), 10) || 0,
    name: String(item?.name ?? '').trim(),
    command: String(item?.command ?? '').trim(),
    executable: String(item?.executable ?? '').trim(),
  })).filter(item => item.pid > 0 || item.name || item.command || item.executable)
}

export const normalizeListenerState = (value: any): FirewallRuleListenerState => ({
  supported: value?.supported !== false,
  checkedAt: Number.parseInt(String(value?.checkedAt ?? 0), 10) || 0,
  occupied: value?.occupied === true,
  listenerCount: Number.parseInt(String(value?.listenerCount ?? 0), 10) || 0,
  listeners: Array.isArray(value?.listeners) ? value.listeners.map((item: any) => ({
    port: Number.parseInt(String(item?.port ?? 0), 10) || 0,
    protocol: String(item?.protocol ?? '').trim(),
    socketFamily: String(item?.socketFamily ?? '').trim(),
    stack: String(item?.stack ?? '').trim(),
    stackSource: String(item?.stackSource ?? '').trim(),
    bindAddress: String(item?.bindAddress ?? '').trim(),
    owners: normalizeListenerOwners(item?.owners),
  })).filter((item: FirewallPortListener) => item.port > 0) : [],
  error: typeof value?.error === 'string' ? value.error : undefined,
})

export const formatPortList = (ports: number[]) => {
  if (!ports || ports.length === 0) return '-'
  return ports.join(', ')
}

export const formatTimestamp = (timestamp: number) => {
  if (!timestamp) return '未刷新'
  return formatPanelDateTime(timestamp * 1000)
}

export const formatMetricCount = (value: number) => {
  const normalized = Number(value ?? 0)
  if (!Number.isFinite(normalized) || normalized <= 0) return '0'
  return normalized.toLocaleString('zh-CN')
}

export const familyLabel = (family: string) => {
  if (family === 'ipv4') return 'IPv4'
  if (family === 'ipv6') return 'IPv6'
  return '双栈'
}

export const protocolLabel = (protocol: string) => {
  if (protocol === 'tcp') return 'TCP'
  if (protocol === 'udp') return 'UDP'
  if (protocol === 'icmp') return 'ICMP'
  if (protocol === 'icmp_v4') return 'ICMP v4'
  if (protocol === 'icmp_v6') return 'ICMP v6'
  if (protocol === 'any') return 'ANY'
  return 'TCP+UDP'
}

export const originLabel = (origin: string) => {
  if (origin === 'system') return '系统保留'
  if (origin === 'temporary') return 'ACME 临时'
  if (origin === 'external') return '外部扫描'
  return '面板规则'
}

export const originColor = (origin: string) => {
  if (origin === 'system') return 'success'
  if (origin === 'temporary') return 'info'
  if (origin === 'external') return 'warning'
  return 'primary'
}

export const listenerStackLabel = (listener: FirewallPortListener) => {
  const suffix = listener.stackSource === 'inferred'
    ? '（推断）'
    : listener.stackSource === 'unknown'
      ? '（待确认）'
      : ''
  if (listener.stack === 'dual') return `双栈${suffix}`
  if (listener.stack === 'ipv4') return `仅 IPv4${suffix}`
  if (listener.stack === 'ipv6') return `仅 IPv6${suffix}`
  return `未知${suffix}`
}

export const listenerStackColor = (listener: FirewallPortListener) => {
  if (listener.stack === 'dual') return 'success'
  if (listener.stack === 'ipv4') return 'info'
  if (listener.stack === 'ipv6') return 'secondary'
  return 'warning'
}

export const formatListenerOwners = (owners: FirewallListenerOwner[]) => {
  if (!owners || owners.length === 0) {
    return '未解析到进程信息'
  }
  return owners.map(owner => {
    const label = owner.name || owner.executable || owner.command || `pid ${owner.pid}`
    return `${label} (PID ${owner.pid})`
  }).join(' / ')
}

export const formatListenerCommand = (owners: FirewallListenerOwner[]) => {
  if (!owners || owners.length === 0) return ''
  const command = owners[0].command || owners[0].executable || ''
  if (!command) return ''
  if (owners.length === 1) return command
  return `${command} 等 ${owners.length} 个进程`
}

export const listenerKey = (listener: FirewallPortListener) => {
  const ownerKey = listener.owners.map(owner => owner.pid).join(',')
  return `${listener.port}-${listener.protocol}-${listener.socketFamily}-${listener.stack}-${listener.bindAddress}-${ownerKey}`
}

export const geoActionLabel = (action: string) => {
  if (action === 'allow') return '只放行'
  return '直接阻断'
}

export const geoActionColor = (action: string) => {
  if (action === 'allow') return 'success'
  return 'error'
}

export const geoCountryLabel = (countryCode: string) => {
  const normalized = (countryCode || '').trim()
  if (!normalized) return '自定义源'
  return normalized.toUpperCase()
}

export const geoStatusColor = (rule: FirewallGeoRule) => {
  if (rule.lastRefreshError) return 'warning'
  if (rule.lastRefreshAt > 0 && rule.prefixCount > 0) return 'success'
  return 'grey'
}

export const geoStatusLabel = (rule: FirewallGeoRule) => {
  if (rule.lastRefreshError) return '缓存回退'
  if (rule.lastRefreshAt > 0 && rule.prefixCount > 0) return '缓存可用'
  return '待刷新'
}

export const normalizeStringArray = (value: unknown): string[] => {
  if (!Array.isArray(value)) return []
  const result: string[] = []
  const seen = new Set<string>()
  for (const item of value) {
    const normalized = String(item ?? '').trim()
    if (!normalized || seen.has(normalized)) continue
    seen.add(normalized)
    result.push(normalized)
  }
  return result
}

export const compactGeoSourceLabel = (value: string) => {
  const normalized = (value || '').trim()
  if (!normalized) return ''
  try {
    const parsed = new URL(normalized)
    const pathParts = parsed.pathname.split('/').filter(Boolean)
    const tail = pathParts.length > 0 ? pathParts[pathParts.length - 1] : ''
    return tail ? `${parsed.hostname} / ${tail}` : parsed.hostname
  } catch {
    return normalized
  }
}

export const geoProviderSummary = (rule: FirewallGeoRule) => {
  if (rule.customSourceUrls.length > 0) {
    return `自定义规则集 ${rule.customSourceUrls.length} 个`
  }
  if (rule.sourceProviders.length === 0) {
    return '默认顺序：JSON 优先，其次 Clash'
  }
  const labels = rule.sourceProviders
    .slice(0, 2)
    .map(value => geoProviderTitleMap[value] || value)
  if (rule.sourceProviders.length > 2) {
    return `${labels.join(' / ')} 等 ${rule.sourceProviders.length} 个来源`
  }
  return labels.join(' / ')
}

export const geoResolvedSourceSummary = (rule: FirewallGeoRule) => {
  if (rule.resolvedSources.length === 0) {
    return '尚未命中可用源'
  }
  if (rule.resolvedSources.length === 1) {
    return compactGeoSourceLabel(rule.resolvedSources[0])
  }
  return `${compactGeoSourceLabel(rule.resolvedSources[0])} 等 ${rule.resolvedSources.length} 个源`
}

export const geoCacheSummary = (rule: FirewallGeoRule) => {
  if (rule.cachedFiles.length === 0) {
    return '未生成缓存文件'
  }
  return `已缓存 ${rule.cachedFiles.length} 个文件`
}

export const isIcmpProtocol = (protocol: string) => ['icmp', 'icmp_v4', 'icmp_v6'].includes(protocol)
export const ruleNeedsListenerTracking = (rule: Pick<FirewallRule, 'protocol' | 'portSpec'>) => !isIcmpProtocol(rule.protocol) && String(rule.portSpec || '').trim().length > 0

export const sourceModeForRule = (rule: Pick<FirewallRule, 'sourceMode' | 'sourceSpec'>) => {
  const mode = String(rule.sourceMode || '').trim().toLowerCase()
  if (mode === 'block' || mode === 'allow') return mode
  return String(rule.sourceSpec || '').trim() ? 'allow' : ''
}

export const normalizeRuleFamilyForSubmit = (protocol: string, family: string) => {
  if (protocol === 'icmp') return 'dual'
  if (protocol === 'icmp_v4') return 'ipv4'
  if (protocol === 'icmp_v6') return 'ipv6'
  return family || 'dual'
}

export const normalizeManagedInstallTask = (raw: any): ManagedInstallTask => ({
  id: String(raw?.id ?? '').trim(),
  state: String(raw?.state ?? 'idle').trim().toLowerCase() || 'idle',
  phase: String(raw?.phase ?? '').trim(),
  canCancel: raw?.canCancel === true,
  stopRequested: raw?.stopRequested === true,
  deadlineExceeded: raw?.deadlineExceeded === true,
  error: String(raw?.error ?? '').trim(),
  startedAt: Number(raw?.startedAt ?? 0) || 0,
  updatedAt: Number(raw?.updatedAt ?? 0) || 0,
  deadlineAt: Number(raw?.deadlineAt ?? 0) || 0,
  finishedAt: Number(raw?.finishedAt ?? 0) || 0,
})

export function useFirewallManage(props: { active?: boolean }) {
  const activeTab = ref<'rules' | 'geo' | 'diagnostics'>('rules')
  const loading = ref(false)
  const hasLoaded = ref(false)
  const loadError = ref('')
  const refreshing = ref(false)
  const switchBusy = ref(false)
  const installingNftables = ref(false)
  const nftablesInstallTask = ref<ManagedInstallTask>({
    id: '',
    state: 'idle',
    phase: '',
    canCancel: false,
    stopRequested: false,
    deadlineExceeded: false,
    error: '',
    startedAt: 0,
    updatedAt: 0,
    deadlineAt: 0,
    finishedAt: 0,
  })
  const nftablesStopRequestPending = ref(false)
  const nftablesInstallPollingTimer = ref<number | null>(null)
  const savingSSHPort = ref(false)
  const switchingSSHProxy = ref(false)
  const systemRuleBusyKey = ref('')
  const savingRule = ref(false)
  const savingGeoRule = ref(false)
  const savingGeoSettings = ref(false)
  const geoRefreshing = ref(false)
  const overview = ref<FirewallOverview>(emptyOverview())
  const switchEnabled = ref(false)
  const sshPortInput = ref('22')
  const sshPortInputTouched = ref(false)
  const sshPortDirty = ref(false)
  const sshProxySwitch = ref(false)
  const searchText = ref('')
  const familyFilter = ref('all')
  const originFilter = ref('all')
  const dialogVisible = ref(false)
  const geoDialogVisible = ref(false)
  const sshDialogVisible = ref(false)
  const pollTimer = ref<number | null>(null)
  const overviewRequest = ref<Promise<Msg> | null>(null)
  let overviewAbortController: AbortController | null = null
  let overviewRequestToken = 0
  let overviewMutationToken = 0
  let runtimeAbortController: AbortController | null = null
  let runtimeRequestToken = 0
  let runtimePollCount = 0
  let pollingGeneration = 0
  const geoSearchText = ref('')
  const geoFamilyFilter = ref('all')
  const geoActionFilter = ref('all')
  const geoIntervalInput = ref('360')
  const geoSettingsDirty = ref(false)
  const syncingGeoInterval = ref(false)

  const editingRule = ref<FirewallRuleForm>(createEmptyRuleForm())
  const editingGeoRule = ref<FirewallGeoRuleForm>(createEmptyGeoRuleForm())

  const sshPortBaseline = computed(() => {
    const port = Number.parseInt(String(overview.value.sshConfig.port || '').trim(), 10)
    if (Number.isInteger(port) && port >= 1 && port <= 65535) {
      return port
    }
    return 22
  })

  const sshPortInputValue = computed(() => parseStrictPositiveInteger(sshPortInput.value) ?? 0)

  const sshPortInputValid = computed(() => {
    const value = sshPortInputValue.value
    return Number.isInteger(value) && value >= 1 && value <= 65535
  })

  const canSaveSSHPort = computed(() => {
    return overview.value.sshConfig.supported && sshPortInputValid.value && sshPortInputValue.value !== sshPortBaseline.value
  })

  const sshProxyHintText = computed(() => {
    if (!overview.value.sshConfig.supported) {
      return overview.value.sshConfig.error || '当前平台不支持 SSH 配置管理'
    }
    if (overview.value.sshConfig.error) {
      return `检测异常：${overview.value.sshConfig.error}`
    }
    if (overview.value.sshConfig.proxyEnabled) {
      return '当前状态：已开启（AllowTcpForwarding yes / PermitOpen any / GatewayPorts no）'
    }
    const allow = overview.value.sshConfig.allowTcpForwarding || '未配置'
    const permit = overview.value.sshConfig.permitOpen || '未配置'
    const gateway = overview.value.sshConfig.gatewayPorts || '未配置'
    return `当前状态：未开启（AllowTcpForwarding ${allow} / PermitOpen ${permit} / GatewayPorts ${gateway}）`
  })

  const sshProxyHintClass = computed(() => {
    if (!overview.value.sshConfig.supported || overview.value.sshConfig.error) {
      return 'text-warning'
    }
    return overview.value.sshConfig.proxyEnabled ? 'text-success' : 'text-medium-emphasis'
  })

  const lastSyncLabel = computed(() => {
    if (!overview.value.lastSyncAt) return '未同步'
    return formatTimestamp(overview.value.lastSyncAt)
  })

  const nftCapabilityLabel = computed(() => {
    const nftables = overview.value.nftables
    const version = nftables.nftVersion ? `nft ${nftables.nftVersion}` : 'nft 版本未知'
    const kernel = nftables.kernelVersion ? `内核 ${nftables.kernelVersion}` : '内核版本未知'
    const modeMap: Record<string, string> = {
      native: '原生布局',
      compatibility: '兼容布局',
      conservative: '保守兼容布局',
    }
    const renderer = nftables.rendererSupported ? '' : ' · 渲染不可用'
    const pending = nftables.layoutPending ? ' · 待完整校验' : ''
    return `${version} · ${kernel} · ${modeMap[nftables.compatibilityMode] || '保守兼容布局'}${renderer}${pending}`
  })

  const nftCapabilityChipColor = computed(() => {
    if (!overview.value.nftables.rendererSupported || overview.value.nftables.layoutPending) {
      return 'warning'
    }
    switch (overview.value.nftables.compatibilityMode) {
      case 'native':
        return 'success'
      case 'compatibility':
        return 'info'
      default:
        return 'warning'
    }
  })

  const hasActiveNftablesInstall = computed(() => (
    ['queued', 'running', 'stopping'].includes(nftablesInstallTask.value.state)
  ))

  const hasTerminalNftablesInstallTask = computed(() => (
    nftablesInstallTask.value.id !== ''
    && ['error', 'cancelled', 'timed_out'].includes(nftablesInstallTask.value.state)
  ))

  const hasFirewallWriteInProgress = computed(() => (
    switchBusy.value
    || installingNftables.value
    || nftablesStopRequestPending.value
    || savingSSHPort.value
    || switchingSSHProxy.value
    || systemRuleBusyKey.value !== ''
    || savingRule.value
    || savingGeoRule.value
    || savingGeoSettings.value
    || geoRefreshing.value
    || hasActiveNftablesInstall.value
  ))

  const showNftablesInstallTask = computed(() => (
    (overview.value.nftables.supported && !overview.value.nftables.installed)
    || hasActiveNftablesInstall.value
    || hasTerminalNftablesInstallTask.value
  ))

  const nftablesStopButtonLabel = computed(() => {
    if (nftablesStopRequestPending.value || nftablesInstallTask.value.state === 'stopping') return '正在停止'
    return nftablesInstallTask.value.canCancel ? '停止' : '正在应用'
  })

  const nftablesInstallTerminalText = computed(() => {
    if (nftablesInstallTask.value.state === 'cancelled') return 'nftables 下载已停止'
    if (nftablesInstallTask.value.state === 'timed_out') return 'nftables 下载超时，任务已停止'
    return 'nftables 安装失败'
  })

  const geoLastRefreshLabel = computed(() => {
    if (!overview.value.geoLastRefreshAt) return '未刷新'
    return formatTimestamp(overview.value.geoLastRefreshAt)
  })

  const geoDialogUsesCustomSources = computed(() => editingGeoRule.value.customSourceUrls.trim().length > 0)

  const editingRuleUsesFixedFamily = computed(() => isIcmpProtocol(editingRule.value.protocol))
  const editingRuleNeedsPort = computed(() => !isIcmpProtocol(editingRule.value.protocol))
  const editingRuleNeedsSource = computed(() => !isIcmpProtocol(editingRule.value.protocol))

  const rulePortPlaceholder = computed(() => {
    if (isIcmpProtocol(editingRule.value.protocol)) return 'ICMP rules do not use ports'
    return '22 / 80,443 / 10000-10100'
  })

  const ruleSourcePlaceholder = computed(() => {
    if (isIcmpProtocol(editingRule.value.protocol)) return 'ICMP rules do not use source filters'
    return '例如：1.2.3.4/32, 2001:db8::/64'
  })

  const filteredRules = computed(() => {
    const keyword = searchText.value.trim().toLowerCase()
    return overview.value.rules.filter(rule => {
      if (familyFilter.value !== 'all' && rule.family !== familyFilter.value) {
        return false
      }
      if (originFilter.value !== 'all' && rule.origin !== originFilter.value) {
        return false
      }
      if (!keyword) {
        return true
      }
      return [
        rule.name,
        rule.description,
        rule.portSpec,
        rule.sourceSpec,
        sourceModeForRule(rule),
        rule.origin,
      ].some(value => (value || '').toLowerCase().includes(keyword))
    })
  })

  const filteredGeoRules = computed(() => {
    const keyword = geoSearchText.value.trim().toLowerCase()
    return overview.value.geoRules.filter(rule => {
      if (geoFamilyFilter.value !== 'all' && rule.family !== geoFamilyFilter.value) {
        return false
      }
      if (geoActionFilter.value !== 'all' && rule.action !== geoActionFilter.value) {
        return false
      }
      if (!keyword) {
        return true
      }
      return [
        rule.name,
        rule.description,
        rule.portSpec,
        rule.countryCode,
        rule.action,
        ...rule.sourceProviders,
        ...rule.customSourceUrls,
        ...rule.resolvedSources,
        rule.lastRefreshError,
      ].some(value => (value || '').toLowerCase().includes(keyword))
    })
  })

  const systemRuleReserved = (systemKey: string) => {
    if (systemKey === 'ssh') return overview.value.defaultPorts.sshReserved
    if (systemKey === 'sub') return overview.value.defaultPorts.subReserved
    if (systemKey === 'panel') return overview.value.defaultPorts.panelReserved
    return false
  }

  const systemRuleStatusLabel = (systemKey: string) => {
    if (systemKey === 'panel') return '强制保留'
    return systemRuleReserved(systemKey) ? '已保留' : '已移除'
  }

  const systemRuleStatusColor = (systemKey: string) => {
    if (systemKey === 'panel') return 'info'
    return systemRuleReserved(systemKey) ? 'success' : 'warning'
  }

  const setSystemRuleReserved = async (systemKey: 'ssh' | 'sub', reserved: boolean) => {
    if (!overview.value.available || hasFirewallWriteInProgress.value) return
    systemRuleBusyKey.value = systemKey
    beginOverviewMutation()
    try {
      const msg = await HttpUtils.post('api/firewall-system-rule', { systemKey, reserved }, {
        headers: { 'Content-Type': 'application/json' },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        push.success({
          duration: 4000,
          message: `${systemKey === 'ssh' ? 'SSH' : '订阅'} 保留状态已${reserved ? '恢复' : '移除'}`,
        })
      }
    } finally {
      systemRuleBusyKey.value = ''
    }
  }

  const syncGeoIntervalInput = (force = false) => {
    if (!force && geoSettingsDirty.value) {
      return
    }
    syncingGeoInterval.value = true
    geoIntervalInput.value = String(overview.value.geoUpdateIntervalMinutes || 360)
    geoSettingsDirty.value = false
    syncingGeoInterval.value = false
  }

  const syncSSHInputsFromOverview = (force = false) => {
    sshProxySwitch.value = overview.value.sshConfig.proxyEnabled === true
    if (!force && sshPortDirty.value) {
      return
    }
    sshPortInput.value = String(sshPortBaseline.value)
    sshPortInputTouched.value = false
    sshPortDirty.value = false
  }

  const applyOverview = (raw: any) => {
    overview.value = {
      ...emptyOverview(),
      ...(raw ?? {}),
      nftables: {
        ...emptyOverview().nftables,
        ...(raw?.nftables ?? {}),
        manualCommands: normalizeStringArray(raw?.nftables?.manualCommands),
      },
      defaultPorts: {
        ...emptyOverview().defaultPorts,
        ...(raw?.defaultPorts ?? {}),
        ssh: normalizeNumberArray(raw?.defaultPorts?.ssh),
        panel: normalizeNumberArray(raw?.defaultPorts?.panel),
        sub: normalizeNumberArray(raw?.defaultPorts?.sub),
        all: normalizeNumberArray(raw?.defaultPorts?.all),
        active: normalizeNumberArray(raw?.defaultPorts?.active),
        sshReserved: raw?.defaultPorts?.sshReserved !== false,
        panelReserved: raw?.defaultPorts?.panelReserved !== false,
        subReserved: raw?.defaultPorts?.subReserved !== false,
      },
      sshConfig: {
        ...emptyOverview().sshConfig,
        ...(raw?.sshConfig ?? {}),
        ports: normalizeNumberArray(raw?.sshConfig?.ports),
      },
      tcpActiveCount: Number(raw?.tcpActiveCount ?? 0),
      tcpSynRecvCount: Number(raw?.tcpSynRecvCount ?? 0),
      tcpEstablishedCount: Number(raw?.tcpEstablishedCount ?? 0),
      tcpAnomalyTotal: Number(raw?.tcpAnomalyTotal ?? 0),
      udpSocketCount: Number(raw?.udpSocketCount ?? 0),
      udpAnomalyTotal: Number(raw?.udpAnomalyTotal ?? 0),
      manualCount: Number(raw?.manualCount ?? 0),
      temporaryCount: Number(raw?.temporaryCount ?? 0),
      externalCount: Number(raw?.externalCount ?? 0),
      systemCount: Number(raw?.systemCount ?? 0),
      totalCount: Number(raw?.totalCount ?? 0),
      rules: Array.isArray(raw?.rules) ? raw.rules.map((item: any) => ({
        ...item,
        temporaryType: typeof item?.temporaryType === 'string' ? item.temporaryType : '',
        temporaryExpireAt: Number(item?.temporaryExpireAt ?? 0),
        sourceMode: typeof item?.sourceMode === 'string'
          ? item.sourceMode
          : (String(item?.sourceSpec || '').trim() ? 'allow' : ''),
        canEdit: item?.canEdit === true,
        canDelete: item?.canDelete === true,
        listenerState: normalizeListenerState(item?.listenerState ?? emptyListenerState()),
      })) : [],
      geoRules: Array.isArray(raw?.geoRules) ? raw.geoRules.map((item: any) => ({
        ...item,
        sourceProviders: normalizeStringArray(item?.sourceProviders),
        customSourceUrls: normalizeStringArray(item?.customSourceUrls),
        resolvedSources: normalizeStringArray(item?.resolvedSources),
        cachedFiles: normalizeStringArray(item?.cachedFiles),
      })) : [],
    }
    switchEnabled.value = overview.value.enabled === true
    syncGeoIntervalInput()
    syncSSHInputsFromOverview()
    systemRuleBusyKey.value = ''
    hasLoaded.value = true
    loadError.value = ''
  }

  const beginOverviewMutation = () => {
    overviewMutationToken++
    overviewRequestToken++
    const controller = overviewAbortController
    overviewAbortController = null
    overviewRequest.value = null
    loading.value = false
    controller?.abort()
  }

  const applyMutationOverview = (raw: any) => {
    beginOverviewMutation()
    applyOverview(raw)
  }

  const fetchOverview = async (silent = false) => {
    if (overviewRequest.value) {
      return overviewRequest.value
    }
    if (!silent) {
      loading.value = true
      loadError.value = ''
    }
    overviewAbortController?.abort()
    const controller = new AbortController()
    overviewAbortController = controller
    const token = ++overviewRequestToken
    const mutationToken = overviewMutationToken
    const request = (async () => {
      const msg = await HttpUtils.get('api/firewall-overview', {}, { silentErrorToast: silent, signal: controller.signal })
      if (token === overviewRequestToken && mutationToken === overviewMutationToken && !controller.signal.aborted) {
        if (msg.success && msg.obj) {
          applyOverview(msg.obj)
        } else if (msg.failureKind !== 'cancelled') {
          loadError.value = msg.msg || '防火墙概览加载失败'
        }
      }
      return msg
    })()
    overviewRequest.value = request
    try {
      return await request
    } finally {
      const isCurrentRequest = overviewRequest.value === request
      if (isCurrentRequest) {
        overviewRequest.value = null
      }
      if (overviewAbortController === controller) {
        overviewAbortController = null
      }
      if (!silent && isCurrentRequest) {
        loading.value = false
      }
    }
  }

  const refreshRuntime = async () => {
    runtimeAbortController?.abort()
    const controller = new AbortController()
    runtimeAbortController = controller
    const token = ++runtimeRequestToken
    try {
      const msg = await HttpUtils.get('api/firewall-runtime', {}, { silentErrorToast: true, signal: controller.signal })
      if (!msg.success || !msg.obj || token !== runtimeRequestToken || controller.signal.aborted) return msg
      const raw = msg.obj
      overview.value = {
        ...overview.value,
        lastSyncAt: Number(raw.lastSyncAt ?? overview.value.lastSyncAt),
        error: typeof raw.error === 'string' && raw.error.trim() ? raw.error : undefined,
      }
      return msg
    } finally {
      if (runtimeAbortController === controller) {
        runtimeAbortController = null
      }
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

  const clearNftablesInstallPolling = () => {
    if (nftablesInstallPollingTimer.value != null) {
      window.clearTimeout(nftablesInstallPollingTimer.value)
      nftablesInstallPollingTimer.value = null
    }
  }

  const resetNftablesInstallTask = () => {
    clearNftablesInstallPolling()
    nftablesInstallTask.value = {
      id: '',
      state: 'idle',
      phase: '',
      canCancel: false,
      stopRequested: false,
      deadlineExceeded: false,
      error: '',
      startedAt: 0,
      updatedAt: 0,
      deadlineAt: 0,
      finishedAt: 0,
    }
    installingNftables.value = false
    nftablesStopRequestPending.value = false
  }

  const scheduleNftablesInstallPolling = () => {
    clearNftablesInstallPolling()
    if (!hasActiveNftablesInstall.value || !props.active) return
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
    nftablesInstallPollingTimer.value = window.setTimeout(() => {
      void pollNftablesInstall()
    }, 1200)
  }

  let completedNftablesTaskID = ''
  const applyNftablesInstallTask = async (raw: any, allowTerminal = true) => {
    const task = normalizeManagedInstallTask(raw)
    nftablesInstallTask.value = task
    installingNftables.value = hasActiveNftablesInstall.value
    nftablesStopRequestPending.value = task.stopRequested || task.state === 'stopping'
    if (hasActiveNftablesInstall.value) {
      scheduleNftablesInstallPolling()
      return
    }
    clearNftablesInstallPolling()
    if (!task.id) {
      if (!allowTerminal) resetNftablesInstallTask()
      return
    }
    if (completedNftablesTaskID === task.id) {
      resetNftablesInstallTask()
      return
    }
    if (!allowTerminal) {
      resetNftablesInstallTask()
      return
    }
    completedNftablesTaskID = task.id
    try {
      await fetchOverview(true)
      if (task.state === 'success') {
        push.success({ duration: 4000, message: i18n.global.t('notifications.nftablesInstalled') })
        return
      }
      if (task.state === 'cancelled' || task.state === 'timed_out') {
        push.info({ duration: 5000, message: task.state === 'timed_out' ? i18n.global.t('notifications.nftablesInstallTimeout') : i18n.global.t('notifications.nftablesInstallStopped') })
        return
      }
      if (task.state === 'error') {
        push.warning({ duration: 6000, message: task.error || task.phase || i18n.global.t('notifications.nftablesInstallFailed') })
      }
    } finally {
      resetNftablesInstallTask()
    }
  }

  const recoverNftablesInstall = async (allowTerminal = false) => {
    const msg = await HttpUtils.get('api/firewall-nftables-install-status', {}, { silentAuthCheck: true })
    if (!msg.success || !msg.obj) return false
    const task = normalizeManagedInstallTask(msg.obj)
    const terminal = task.id !== '' && ['success', 'error', 'cancelled', 'timed_out'].includes(task.state)
    await applyNftablesInstallTask(task, allowTerminal)
    return hasActiveNftablesInstall.value || (allowTerminal && terminal)
  }

  const pollNftablesInstall = async () => {
    if (!props.active || (typeof document !== 'undefined' && document.visibilityState !== 'visible')) return
    const msg = await HttpUtils.get('api/firewall-nftables-install-status', {}, { silentAuthCheck: true })
    if (!msg.success || !msg.obj) {
      scheduleNftablesInstallPolling()
      return
    }
    await applyNftablesInstallTask(msg.obj)
  }

  const installNftables = async () => {
    completedNftablesTaskID = ''
    resetNftablesInstallTask()
    installingNftables.value = true
    beginOverviewMutation()
    try {
      const msg = await HttpUtils.post('api/firewall-nftables-install', {}, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        await applyNftablesInstallTask(msg.obj)
        return
      }
      if (await recoverNftablesInstall()) return
      await fetchOverview(true)
    } finally {
      if (!hasActiveNftablesInstall.value) {
        installingNftables.value = false
      }
    }
  }

  const stopNftablesInstall = async () => {
    const id = nftablesInstallTask.value.id.trim()
    if (!id || !nftablesInstallTask.value.canCancel || nftablesStopRequestPending.value) return
    nftablesStopRequestPending.value = true
    nftablesInstallTask.value = { ...nftablesInstallTask.value, state: 'stopping', canCancel: false, stopRequested: true, phase: '正在停止' }
    beginOverviewMutation()
    try {
      const msg = await HttpUtils.post('api/firewall-nftables-install-stop', { id }, {
        headers: { 'Content-Type': 'application/json' },
        silentAuthCheck: true,
      })
      if (msg.success && msg.obj) {
        await applyNftablesInstallTask(msg.obj)
        return
      }
      await pollNftablesInstall()
    } catch {
      await pollNftablesInstall()
    } finally {
      if (nftablesInstallTask.value.state !== 'stopping') {
        nftablesStopRequestPending.value = false
      }
    }
  }

  const onToggleFirewall = async (nextValue: boolean | null) => {
    if (nextValue == null) {
      switchEnabled.value = overview.value.enabled
      return
    }
    if (nextValue === switchEnabled.value) return

    let firewallSwitchMessage = nextValue
      ? '开启后会按当前面板规则重建防火墙链，仅放行系统保留端口和面板已配置规则，并扫描系统已有放行规则供展示，是否继续？'
      : '关闭后会删除面板自己的防火墙链，但保留当前规则记录，是否继续？'
    if (nextValue && !overview.value.defaultPorts.sshReserved) {
      firewallSwitchMessage += '\n当前 SSH 系统保留已移除，开启后不会自动放行 SSH，可能导致新的远程 SSH 连接无法建立。'
    }

    const confirmed = await confirm({
      message: firewallSwitchMessage,
      severity: 'warning',
      confirmText: confirmAction(nextValue ? 'enable' : 'disable'),
    })
    if (!confirmed || switchBusy.value || !overview.value.available || nextValue === overview.value.enabled) {
      switchEnabled.value = overview.value.enabled
      return
    }

    beginOverviewMutation()
    switchBusy.value = true
    try {
      const msg = await HttpUtils.post('api/firewall-switch', { enabled: nextValue }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        push.success({
          duration: 4000,
          message: nextValue ? '防火墙已开启并完成规则应用' : '防火墙已关闭并停止下发规则',
        })
      } else {
        switchEnabled.value = overview.value.enabled
      }
    } finally {
      switchBusy.value = false
    }
  }

  const onSSHPortFocus = () => {
    sshPortInputTouched.value = true
  }

  const onSSHPortBlur = () => {
    sshPortInputTouched.value = true
    sshPortDirty.value = String(sshPortInput.value || '').trim() !== String(sshPortBaseline.value)
  }

  const saveSSHPort = async () => {
    sshPortInputTouched.value = true
    if (!sshPortInputValid.value) {
      push.warning({
        duration: 4000,
        message: 'SSH 端口必须是 1-65535 的正整数',
      })
      return
    }
    const nextPort = sshPortInputValue.value
    if (nextPort === sshPortBaseline.value) {
      return
    }

    const sshPortConfirmNotes: string[] = []
    if (overview.value.enabled && !overview.value.defaultPorts.sshReserved) {
      sshPortConfirmNotes.push('当前防火墙已开启，但 SSH 系统保留已移除；新端口不会被项目防火墙自动放行。')
    }
    if (overview.value.sshConfig.ports.length > 1) {
      sshPortConfirmNotes.push('检测到多个 SSH 端口；本次只修改主配置端口，不会删除 sshd_config.d 等文件中的其他 Port。')
    }
    const sshPortConfirmMessage = [
      `确认将 SSH 端口改为 ${nextPort}，并重启 SSH 服务使其生效吗？`,
      ...sshPortConfirmNotes,
    ].join('\n')

    const confirmed = await confirm({
      message: sshPortConfirmMessage,
      severity: 'warning',
      confirmText: confirmAction('save'),
    })
    if (
      !confirmed
      || savingSSHPort.value
      || !sshPortInputValid.value
      || sshPortInputValue.value !== nextPort
      || nextPort === sshPortBaseline.value
    ) {
      return
    }

    beginOverviewMutation()
    savingSSHPort.value = true
    try {
      const msg = await HttpUtils.post('api/firewall-ssh-port', { port: nextPort }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        syncSSHInputsFromOverview(true)
        push.success({
          duration: 4000,
          message: 'SSH 端口已更新并重启 SSH 服务',
        })
      }
    } finally {
      savingSSHPort.value = false
    }
  }

  const onToggleSSHProxy = async (nextValue: boolean | null) => {
    if (nextValue == null) {
      sshProxySwitch.value = overview.value.sshConfig.proxyEnabled
      return
    }
    if (nextValue === overview.value.sshConfig.proxyEnabled) {
      sshProxySwitch.value = overview.value.sshConfig.proxyEnabled
      return
    }

    sshProxySwitch.value = nextValue
    beginOverviewMutation()
    switchingSSHProxy.value = true
    try {
      const msg = await HttpUtils.post('api/firewall-ssh-proxy', { enabled: nextValue }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        push.success({
          duration: 4000,
          message: nextValue ? 'SSH 代理能力已开启并重启 SSH 服务' : 'SSH 代理能力已关闭并重启 SSH 服务',
        })
        return
      }
      sshProxySwitch.value = overview.value.sshConfig.proxyEnabled
    } finally {
      switchingSSHProxy.value = false
    }
  }

  const openRuleDialog = (rule?: FirewallRule) => {
    if (!overview.value.enabled) return
    const normalizedProtocol = rule?.protocol === 'any' ? 'tcp_udp' : (rule?.protocol || 'tcp_udp')
    editingRule.value = rule
      ? {
          id: rule.id,
          name: rule.name,
          description: rule.description,
          family: rule.family || 'dual',
          protocol: normalizedProtocol,
          portSpec: rule.portSpec || '',
          sourceSpec: rule.sourceSpec || '',
          sourceMode: sourceModeForRule(rule),
        }
      : createEmptyRuleForm()
    editingRule.value.family = normalizeRuleFamilyForSubmit(editingRule.value.protocol, editingRule.value.family)
    dialogVisible.value = true
  }

  const closeRuleDialog = () => {
    dialogVisible.value = false
  }

  const saveRule = async () => {
    if (!overview.value.enabled) return
    savingRule.value = true
    try {
      const normalizedProtocol = editingRule.value.protocol
      if (normalizedProtocol === 'any') {
        push.warning({
          duration: 4000,
          message: 'ANY 协议已禁用，请选择 TCP / UDP / TCP+UDP / ICMP',
        })
        return
      }

      const normalizedFamily = normalizeRuleFamilyForSubmit(normalizedProtocol, editingRule.value.family)
      const payload: any = {
        id: editingRule.value.id,
        name: editingRule.value.name.trim(),
        description: editingRule.value.description.trim(),
        family: normalizedFamily,
        protocol: normalizedProtocol,
        portSpec: editingRuleNeedsPort.value ? editingRule.value.portSpec.trim() : '',
        sourceSpec: editingRuleNeedsSource.value ? editingRule.value.sourceSpec.trim() : '',
        sourceMode: editingRuleNeedsSource.value ? editingRule.value.sourceMode : '',
      }

      if (['block', 'allow'].includes(payload.sourceMode) && !payload.sourceSpec) {
        push.warning({
          duration: 4000,
          message: '设置了源地址策略时，必须填写源地址限制',
        })
        return
      }

      beginOverviewMutation()
      const msg = await HttpUtils.post('api/firewall-rule', payload, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        closeRuleDialog()
        push.success({
          duration: 4000,
          message: payload.id > 0 ? '防火墙规则已更新' : '防火墙规则已创建',
        })
      }
    } finally {
      savingRule.value = false
    }
  }

  const removeRule = async (rule: FirewallRule) => {
    if (rule.origin !== 'system' && !overview.value.enabled) return
    const confirmed = await confirm({
      message: `确认删除规则「${rule.name || rule.portSpec || '未命名'}」吗？`,
      severity: 'danger',
      confirmText: confirmAction('delete'),
    })
    if (!confirmed) return

    beginOverviewMutation()
    try {
      const msg = await HttpUtils.post('api/firewall-rule-delete', { id: rule.id }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        push.success({
          duration: 4000,
          message: '防火墙规则已删除',
        })
      }
    } catch {
      // handled by HttpUtils
    }
  }

  const openGeoRuleDialog = (rule?: FirewallGeoRule) => {
    editingGeoRule.value = rule
      ? {
          id: rule.id,
          name: rule.name,
          description: rule.description,
          family: rule.family || 'dual',
          protocol: rule.protocol || 'tcp_udp',
          portSpec: rule.portSpec || '',
          action: rule.action || 'block',
          countryCode: rule.countryCode || '',
          sourceProviders: [...rule.sourceProviders],
          customSourceUrls: rule.customSourceUrls.join('\n'),
        }
      : createEmptyGeoRuleForm()
    geoDialogVisible.value = true
  }

  const closeGeoRuleDialog = () => {
    geoDialogVisible.value = false
  }

  const saveGeoRule = async () => {
    savingGeoRule.value = true
    try {
      const customUrls = editingGeoRule.value.customSourceUrls
        .split(/[\n,]/)
        .map(url => url.trim())
        .filter(Boolean)

      const payload = {
        id: editingGeoRule.value.id,
        name: editingGeoRule.value.name.trim(),
        description: editingGeoRule.value.description.trim(),
        family: editingGeoRule.value.family || 'dual',
        protocol: editingGeoRule.value.protocol || 'tcp_udp',
        portSpec: editingGeoRule.value.portSpec.trim(),
        action: editingGeoRule.value.action || 'block',
        countryCode: editingGeoRule.value.countryCode.trim().toUpperCase(),
        sourceProviders: [...editingGeoRule.value.sourceProviders],
        customSourceUrls: customUrls,
      }

      if (!payload.portSpec) {
        push.warning({
          duration: 4000,
          message: '必须填写端口或端口段',
        })
        return
      }

      if (!payload.countryCode && customUrls.length === 0) {
        push.warning({
          duration: 4000,
          message: '国家代码与自定义规则集 URL 至少填写一项',
        })
        return
      }

      beginOverviewMutation()
      const msg = await HttpUtils.post('api/firewall-geo-rule', payload, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        closeGeoRuleDialog()
        push.success({
          duration: 4000,
          message: payload.id > 0 ? 'GeoIP 规则已更新' : 'GeoIP 规则已创建并加入定时刷新',
        })
      }
    } finally {
      savingGeoRule.value = false
    }
  }

  const removeGeoRule = async (rule: FirewallGeoRule) => {
    const confirmed = await confirm({
      message: `确认删除 GeoIP 规则「${rule.name || rule.portSpec || '未命名'}」吗？`,
      severity: 'danger',
      confirmText: confirmAction('delete'),
    })
    if (!confirmed) return

    beginOverviewMutation()
    try {
      const msg = await HttpUtils.post('api/firewall-geo-rule-delete', { id: rule.id }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        push.success({
          duration: 4000,
          message: 'GeoIP 规则已删除',
        })
      }
    } catch {
      // handled by HttpUtils
    }
  }

  const refreshGeoRules = async () => {
    geoRefreshing.value = true
    beginOverviewMutation()
    try {
      const msg = await HttpUtils.post('api/firewall-geo-refresh', {}, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        push.success({
          duration: 4000,
          message: 'GeoIP 规则已完成热更新',
        })
      }
    } finally {
      geoRefreshing.value = false
    }
  }

  const saveGeoSettings = async () => {
    const intervalMinutes = parseStrictPositiveInteger(geoIntervalInput.value)
    if (intervalMinutes == null) {
      push.warning({
        duration: 4000,
        message: '更新周期必须是大于 0 的分钟数',
      })
      return
    }

    savingGeoSettings.value = true
    beginOverviewMutation()
    try {
      const msg = await HttpUtils.post('api/firewall-geo-settings', { intervalMinutes }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyMutationOverview(msg.obj)
        syncGeoIntervalInput(true)
        push.success({
          duration: 4000,
          message: 'GeoIP 更新周期已保存',
        })
      }
    } finally {
      savingGeoSettings.value = false
    }
  }

  const stopPolling = () => {
    pollingGeneration++
    if (pollTimer.value != null) {
      window.clearTimeout(pollTimer.value)
      pollTimer.value = null
    }
  }

  const clearPollingTimer = () => {
    if (pollTimer.value != null) {
      window.clearTimeout(pollTimer.value)
      pollTimer.value = null
    }
  }

  const schedulePolling = (delay = 10000, generation = pollingGeneration) => {
    clearPollingTimer()
    if (generation !== pollingGeneration) return
    if (!props.active) return
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
    pollTimer.value = window.setTimeout(async () => {
      pollTimer.value = null
      if (generation !== pollingGeneration || !props.active || (typeof document !== 'undefined' && document.visibilityState !== 'visible')) return
      runtimePollCount++
      const msg = runtimePollCount % 6 === 0
        ? await fetchOverview(true)
        : await refreshRuntime()
      if (generation !== pollingGeneration) return
      schedulePolling(msg.success ? 10000 : 30000, generation)
    }, delay)
  }

  const startPolling = () => {
    stopPolling()
    runtimePollCount = 0
    schedulePolling(10000, pollingGeneration)
  }

  const abortOverviewRequest = () => {
    overviewRequestToken++
    overviewAbortController?.abort()
    overviewAbortController = null
    overviewRequest.value = null
    loading.value = false
    runtimeRequestToken++
    runtimeAbortController?.abort()
    runtimeAbortController = null
  }

  const handleVisibilityChange = () => {
    if (document.visibilityState === 'visible') {
      if (!props.active) return
      void fetchOverview(hasLoaded.value)
      void recoverNftablesInstall()
      startPolling()
      return
    }
    stopPolling()
    abortOverviewRequest()
    clearNftablesInstallPolling()
  }

  watch(geoIntervalInput, value => {
    if (syncingGeoInterval.value) return
    geoSettingsDirty.value = value.trim() !== String(overview.value.geoUpdateIntervalMinutes || 360)
  })

  watch(sshPortInput, value => {
    if (!sshPortInputTouched.value) return
    sshPortDirty.value = value.trim() !== String(sshPortBaseline.value)
  })

  watch(() => editingRule.value.protocol, protocol => {
    if (!isIcmpProtocol(protocol)) {
      return
    }
    editingRule.value.family = normalizeRuleFamilyForSubmit(protocol, editingRule.value.family)
    editingRule.value.portSpec = ''
    editingRule.value.sourceSpec = ''
    editingRule.value.sourceMode = ''
  })

  watch(() => props.active, active => {
    if (active) {
      void fetchOverview(hasLoaded.value)
      void recoverNftablesInstall()
      startPolling()
      return
    }
    stopPolling()
    abortOverviewRequest()
    clearNftablesInstallPolling()
  })

  onMounted(() => {
    if (props.active) {
      void fetchOverview()
      void recoverNftablesInstall()
      startPolling()
    }
    if (typeof document !== 'undefined') {
      document.addEventListener('visibilitychange', handleVisibilityChange)
    }
  })

  onBeforeUnmount(() => {
    stopPolling()
    abortOverviewRequest()
    clearNftablesInstallPolling()
    if (typeof document !== 'undefined') {
      document.removeEventListener('visibilitychange', handleVisibilityChange)
    }
  })

  return {
    activeTab,
    loading,
    hasLoaded,
    loadError,
    refreshing,
    switchBusy,
    installingNftables,
    nftablesInstallTask,
    nftablesStopRequestPending,
    savingSSHPort,
    switchingSSHProxy,
    systemRuleBusyKey,
    savingRule,
    savingGeoRule,
    savingGeoSettings,
    geoRefreshing,
    overview,
    switchEnabled,
    sshPortInput,
    sshPortInputTouched,
    sshPortDirty,
    sshProxySwitch,
    searchText,
    familyFilter,
    originFilter,
    dialogVisible,
    geoDialogVisible,
    sshDialogVisible,
    geoSearchText,
    geoFamilyFilter,
    geoActionFilter,
    geoIntervalInput,
    geoSettingsDirty,
    editingRule,
    editingGeoRule,

    // Computeds
    sshPortBaseline,
    sshPortInputValue,
    sshPortInputValid,
    canSaveSSHPort,
    sshProxyHintText,
    sshProxyHintClass,
    lastSyncLabel,
    nftCapabilityLabel,
    nftCapabilityChipColor,
    hasActiveNftablesInstall,
    hasTerminalNftablesInstallTask,
    hasFirewallWriteInProgress,
    showNftablesInstallTask,
    nftablesStopButtonLabel,
    nftablesInstallTerminalText,
    geoLastRefreshLabel,
    geoDialogUsesCustomSources,
    editingRuleUsesFixedFamily,
    editingRuleNeedsPort,
    editingRuleNeedsSource,
    rulePortPlaceholder,
    ruleSourcePlaceholder,
    filteredRules,
    filteredGeoRules,

    // Methods
    systemRuleReserved,
    systemRuleStatusLabel,
    systemRuleStatusColor,
    setSystemRuleReserved,
    fetchOverview,
    refreshRuntime,
    refreshOverview,
    installNftables,
    stopNftablesInstall,
    onToggleFirewall,
    onSSHPortFocus,
    onSSHPortBlur,
    saveSSHPort,
    onToggleSSHProxy,
    openRuleDialog,
    closeRuleDialog,
    saveRule,
    removeRule,
    openGeoRuleDialog,
    closeGeoRuleDialog,
    saveGeoRule,
    removeGeoRule,
    refreshGeoRules,
    saveGeoSettings,
  }
}
