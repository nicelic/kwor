import HttpUtils, { type Msg } from '@/plugins/httputil'
import { confirm } from '@/plugins/confirm'
import { i18n } from '@/locales'
import {
  formatPanelDateTime,
  panelCalendarDateToEpochSeconds,
  panelCalendarParts,
  panelNow,
} from '@/plugins/panelTime'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { push } from 'notivue'

export type PortForwardRuntimeConflict = {
  ruleId: number
  ruleName: string
  localPortSpec: string
  family: string
  protocol: string
  port: number
  socketFamily: string
  socketStack: string
  stackSource: string
  bindAddress: string
  owners: Array<{
    pid: number
    name: string
  }>
  checkedAt: number
}

export type PortForwardRule = {
  id: number
  name: string
  description: string
  enabled: boolean
  family: string
  protocol: string
  localPortMode: string
  localPortSpec: string
  localPortStart: number
  localPortCount: number
  localPortEnd: number
  targetIP: string
  targetPort: number
  rateLimitMbps: number
  trafficLimitBytes: number
  trafficLimitGiB: number
  trafficResetDay: number
  trafficExpiryDate: string
  effectiveRateLimitMbps: number
  limitStatus: string
  limitWarning: string
  currentUp: number
  currentDown: number
  currentTotal: number
  trafficNextResetAt: number
  trafficLastResetAt: number
  trafficLimitReached: boolean
  trafficExpired: boolean
  trafficBlocked: boolean
  trafficBlockReason: string
  runtimeConflictCount: number
}

type PortForwardOverview = {
  supported: boolean
  ready: boolean
  available: boolean
  nftVersion: string
  kernelVersion: string
  compatibilityMode: string
  rendererSupported: boolean
  supportsTransportHeader: boolean
  supportsTableComments: boolean
  capabilityError: string
  versionProbeError: string
  jsonProbeError: string
  meterProbeError: string
  layoutPending: boolean
  lastApplyError: string
  lastSyncAt: number
  kernelIPv4Forward: boolean
  kernelIPv6Forward: boolean
  enabledCount: number
  limitedCount: number
  totalUp: number
  totalDown: number
  totalTraffic: number
  rules: PortForwardRule[]
  runtimeConflicts: PortForwardRuntimeConflict[]
  warnings?: string[]
  error?: string
}

type PortForwardRuleForm = {
  id: number
  name: string
  description: string
  enabled: boolean
  family: string
  protocol: string
  localPortMode: string
  localPortSpec: string
  localPortStart: number
  localPortCount: number
  localPortEnd: number
  targetIP: string
  targetPort: number
  rateLimitMbps: number
  trafficLimitGiB: number
  trafficResetDay: number
  trafficExpiryDate: string
}

const tr = (key: string, params?: Record<string, unknown>) => String(i18n.global.t(`portForward.${key}`, params ?? {}))

const errorMessage = (error: unknown, fallback: string) => {
  if (error instanceof Error && error.message.trim()) return error.message.trim()
  if (typeof error === 'string' && error.trim()) return error.trim()
  return fallback
}

const touchLocale = () => {
  void i18n.global.locale.value
}

const emptyOverview = (): PortForwardOverview => ({
  supported: false,
  ready: false,
  available: false,
  nftVersion: '',
  kernelVersion: '',
  compatibilityMode: 'conservative',
  rendererSupported: false,
  supportsTransportHeader: false,
  supportsTableComments: false,
  capabilityError: '',
  versionProbeError: '',
  jsonProbeError: '',
  meterProbeError: '',
  layoutPending: false,
  lastApplyError: '',
  lastSyncAt: 0,
  kernelIPv4Forward: false,
  kernelIPv6Forward: false,
  enabledCount: 0,
  limitedCount: 0,
  totalUp: 0,
  totalDown: 0,
  totalTraffic: 0,
  rules: [],
  runtimeConflicts: [],
  warnings: [],
  error: '',
})

const createEmptyRuleForm = (): PortForwardRuleForm => ({
  id: 0,
  name: '',
  description: '',
  enabled: true,
  family: 'ipv4',
  protocol: 'tcp',
  localPortMode: 'single',
  localPortSpec: '',
  localPortStart: 0,
  localPortCount: 1,
  localPortEnd: 0,
  targetIP: '',
  targetPort: 0,
  rateLimitMbps: 0,
  trafficLimitGiB: 0,
  trafficResetDay: 0,
  trafficExpiryDate: '',
})

const toNumber = (value: unknown, fallback = 0) => {
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : fallback
}

const normalizeFamilyValue = (raw: unknown): string => {
  const value = String(raw ?? '').trim().toLowerCase()
  if (value === 'ipv6') return 'ipv6'
  if (value === 'dual' || value === 'ipv4/ipv6' || value === 'ipv4ipv6') return 'dual'
  return 'ipv4'
}

const normalizeProtocolValue = (raw: unknown): string => {
  const value = String(raw ?? '').trim().toLowerCase()
  if (value === 'udp') return 'udp'
  if (value === 'tcp_udp' || value === 'tcp/udp' || value === 'tcp+udp' || value === 'tcpudp') return 'tcp_udp'
  return 'tcp'
}

const normalizeRule = (raw: Partial<PortForwardRule> = {}): PortForwardRule => ({
  id: toNumber(raw.id),
  name: String(raw.name ?? ''),
  description: String(raw.description ?? ''),
  enabled: Boolean(raw.enabled),
  family: normalizeFamilyValue(raw.family),
  protocol: normalizeProtocolValue(raw.protocol),
  localPortMode: String(raw.localPortMode ?? 'single'),
  localPortSpec: String(raw.localPortSpec ?? ''),
  localPortStart: toNumber(raw.localPortStart),
  localPortCount: toNumber(raw.localPortCount, 1),
  localPortEnd: toNumber(raw.localPortEnd),
  targetIP: String(raw.targetIP ?? ''),
  targetPort: toNumber(raw.targetPort),
  rateLimitMbps: toNumber(raw.rateLimitMbps),
  trafficLimitBytes: toNumber(raw.trafficLimitBytes),
  trafficLimitGiB: toNumber(raw.trafficLimitGiB),
  trafficResetDay: toNumber(raw.trafficResetDay),
  trafficExpiryDate: String(raw.trafficExpiryDate ?? ''),
  effectiveRateLimitMbps: toNumber(raw.effectiveRateLimitMbps),
  limitStatus: String(raw.limitStatus ?? ''),
  limitWarning: String(raw.limitWarning ?? ''),
  currentUp: toNumber(raw.currentUp),
  currentDown: toNumber(raw.currentDown),
  currentTotal: toNumber(raw.currentTotal),
  trafficNextResetAt: toNumber(raw.trafficNextResetAt),
  trafficLastResetAt: toNumber(raw.trafficLastResetAt),
  trafficLimitReached: Boolean(raw.trafficLimitReached),
  trafficExpired: Boolean(raw.trafficExpired),
  trafficBlocked: Boolean(raw.trafficBlocked),
  trafficBlockReason: String(raw.trafficBlockReason ?? ''),
  runtimeConflictCount: toNumber(raw.runtimeConflictCount),
})

const normalizeConflict = (raw: Partial<PortForwardRuntimeConflict> = {}): PortForwardRuntimeConflict => ({
  ruleId: toNumber(raw.ruleId),
  ruleName: String(raw.ruleName ?? ''),
  localPortSpec: String(raw.localPortSpec ?? ''),
  family: normalizeFamilyValue(raw.family),
  protocol: normalizeProtocolValue(raw.protocol),
  port: toNumber(raw.port),
  socketFamily: String(raw.socketFamily ?? ''),
  socketStack: String(raw.socketStack ?? ''),
  stackSource: String(raw.stackSource ?? ''),
  bindAddress: String(raw.bindAddress ?? ''),
  owners: Array.isArray(raw.owners) ? raw.owners.map(owner => ({
    pid: toNumber(owner?.pid),
    name: String(owner?.name ?? ''),
  })) : [],
  checkedAt: toNumber(raw.checkedAt),
})

const normalizeWarnings = (raw: unknown): string[] => (
  Array.isArray(raw) ? raw.map(item => String(item ?? '').trim()).filter(Boolean) : []
)

const formatTimestamp = (value: number) => {
  if (!value) return '-'
  return formatPanelDateTime(value * 1000)
}

const mapRuleToForm = (rule?: PortForwardRule): PortForwardRuleForm => ({
  id: rule?.id ?? 0,
  name: rule?.name ?? '',
  description: rule?.description ?? '',
  enabled: rule?.enabled ?? true,
  family: normalizeFamilyValue(rule?.family),
  protocol: normalizeProtocolValue(rule?.protocol),
  // Legacy count records represent an inclusive range. Keep the current
  // semantic visible instead of silently treating it as a single port.
  localPortMode: rule?.localPortMode === 'count' ? 'range' : (rule?.localPortMode ?? 'single'),
  localPortSpec: rule?.localPortSpec ?? '',
  localPortStart: rule?.localPortStart ?? 0,
  localPortCount: rule?.localPortCount ?? 1,
  localPortEnd: rule?.localPortEnd ?? 0,
  targetIP: isLocalTargetIP(rule?.targetIP ?? '') ? '' : (rule?.targetIP ?? ''),
  targetPort: rule?.targetPort ?? 0,
  rateLimitMbps: rule?.rateLimitMbps ?? 0,
  trafficLimitGiB: rule?.trafficLimitGiB ?? 0,
  trafficResetDay: rule?.trafficResetDay ?? 0,
  trafficExpiryDate: rule?.trafficExpiryDate ?? '',
})

const buildPayload = (form: PortForwardRuleForm) => ({
  id: form.id,
  name: form.name.trim(),
  description: form.description.trim(),
  enabled: form.enabled,
  family: normalizeFamilyValue(form.family),
  protocol: normalizeProtocolValue(form.protocol),
  localPortMode: form.localPortMode,
  localPortSpec: form.localPortMode === 'multi'
    ? form.localPortSpec.trim()
    : form.localPortMode === 'single'
      ? String(toNumber(form.localPortStart) || '')
      : '',
  localPortStart: toNumber(form.localPortStart),
  localPortCount: toNumber(form.localPortCount, 1),
  localPortEnd: toNumber(form.localPortEnd),
  targetIP: form.targetIP.trim(),
  targetPort: toNumber(form.targetPort),
  rateLimitMbps: Math.max(0, toNumber(form.rateLimitMbps)),
  trafficLimitGiB: Math.max(0, toNumber(form.trafficLimitGiB)),
  trafficResetDay: Math.max(0, Math.floor(toNumber(form.trafficResetDay))),
  trafficExpiryDate: form.trafficExpiryDate.trim(),
})

export const familyLabel = (value: string) => {
  if (value === 'ipv6') return 'IPv6'
  if (value === 'dual' || value === 'ipv4/ipv6' || value === 'ipv4ipv6') return 'IPv4/IPv6'
  return 'IPv4'
}

export const protocolLabel = (value: string) => {
  if (value === 'udp') return 'UDP'
  if (value === 'tcp_udp' || value === 'tcp/udp' || value === 'tcp+udp' || value === 'tcpudp') return 'TCP/UDP'
  return 'TCP'
}

export const isLocalTargetIP = (value: string) => {
  const trimmed = String(value ?? '').trim().replace(/^\[|\]$/g, '').toLowerCase()
  return trimmed === '' || trimmed === 'localhost' || trimmed === '127.0.0.1' || trimmed === '::1'
}

const targetAddressFamily = (value: string): 'ipv4' | 'ipv6' | '' => {
  const address = String(value ?? '').trim().replace(/^\[|\]$/g, '')
  if (!address) return ''
  const ipv4 = address.split('.')
  if (ipv4.length === 4 && ipv4.every(part => /^\d+$/.test(part) && !/^0\d+/.test(part) && Number(part) >= 0 && Number(part) <= 255)) {
    return 'ipv4'
  }
  if (address.includes(':')) {
    try {
      // The browser parser gives the form-level check real IPv6 syntax
      // validation; the server repeats this with netip before persistence.
      const parsed = new URL(`http://[${address}]/`)
      if (parsed.hostname) return 'ipv6'
    } catch {
      return ''
    }
  }
  return ''
}

export const targetDisplayLabel = (targetIP: string, targetPort: number) => {
  const port = targetPort || 0
  if (isLocalTargetIP(targetIP)) {
    return `${tr('localTarget')}:${port}`
  }
  const address = String(targetIP ?? '').trim().replace(/^\[|\]$/g, '')
  return targetAddressFamily(address) === 'ipv6' ? `[${address}]:${port}` : `${address}:${port}`
}

export const localModeLabel = (value: string) => {
  if (value === 'multi') return tr('multiMode')
  if (value === 'range' || value === 'count') return tr('rangeMode')
  return tr('singleMode')
}

export const rateLimitLabel = (effectiveValue: number, configuredValue = 0, status = '') => {
  if (configuredValue > 0 && effectiveValue <= 0 && status === 'degraded') {
    return tr('effectiveZero')
  }
  return effectiveValue > 0 ? `${effectiveValue} Mbps` : tr('unlimited')
}

export const formatBytes = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let current = value
  let index = 0
  while (current >= 1024 && index < units.length - 1) {
    current /= 1024
    index += 1
  }
  const digits = current >= 100 || index === 0 ? 0 : current >= 10 ? 1 : 2
  return `${current.toFixed(digits)} ${units[index]}`
}

const trafficGiBBytes = 1024 * 1024 * 1024
const maxTrafficLimitGiB = Number.MAX_SAFE_INTEGER / trafficGiBBytes

export const formatTrafficGB = (value: number) => {
  const bytes = Number.isFinite(value) && value > 0 ? value : 0
  return `${(bytes / trafficGiBBytes).toFixed(2)} GB`
}

export const trafficLimitLabel = (rule: PortForwardRule) => (
  rule.trafficLimitGiB > 0 ? `${rule.trafficLimitGiB.toFixed(2)} GB` : tr('unlimited')
)

export const trafficUsageLabel = (rule: PortForwardRule) => (
  `${formatTrafficGB(rule.currentTotal)} / ${trafficLimitLabel(rule)}`
)

export const trafficUsagePercent = (rule: PortForwardRule) => {
  if (!Number.isFinite(rule.trafficLimitGiB) || rule.trafficLimitGiB <= 0) return 0
  const limitBytes = rule.trafficLimitGiB * trafficGiBBytes
  if (limitBytes <= 0) return 0
  return Math.min(100, Math.max(0, Math.round(rule.currentTotal * 100 / limitBytes)))
}

export const trafficBlockLabel = (rule: PortForwardRule) => {
  if (rule.trafficBlockReason === 'expiry' || rule.trafficExpired) return tr('trafficExpiryBlocked')
  if (rule.trafficBlockReason === 'quota' || rule.trafficLimitReached) return tr('trafficQuotaBlocked')
  return ''
}

const normalizeTrafficResetDay = (value: unknown) => {
  const day = Math.floor(toNumber(value))
  return Number.isInteger(day) && day >= 0 && day <= 31 ? day : 0
}

const normalizeTrafficExpiryDate = (value: unknown) => String(value ?? '').trim()

const validTrafficExpiryDate = (value: string) => {
  if (value === '') return true
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (match == null) return false
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const parsed = new Date(year, month - 1, day, 0, 0, 0, 0)
  return Number.isInteger(year) && Number.isInteger(month) && Number.isInteger(day) &&
    parsed.getFullYear() === year && parsed.getMonth() === month - 1 && parsed.getDate() === day
}

const parseEpochSeconds = (value: unknown): number | null => {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return Math.floor(Math.abs(value) < 1e11 ? value : value / 1000)
  }
  if (value instanceof Date && Number.isFinite(value.getTime())) return Math.floor(value.getTime() / 1000)
  if (typeof value === 'string' && value.trim() !== '') {
    if (/^-?\d+(?:\.\d+)?$/.test(value.trim())) return parseEpochSeconds(Number(value.trim()))
    const parsed = Date.parse(value)
    return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : null
  }
  return null
}

const trafficResetPickerEpoch = (value: unknown) => {
  const day = normalizeTrafficResetDay(value)
  if (day <= 0) return 0
  const now = panelCalendarParts(panelNow())
  const lastDay = new Date(now.year, now.month, 0).getDate()
  return panelCalendarDateToEpochSeconds(new Date(now.year, now.month - 1, Math.min(day, lastDay), 0, 0, 0, 0))
}

const trafficExpiryPickerEpoch = (value: unknown) => {
  const date = normalizeTrafficExpiryDate(value)
  if (!validTrafficExpiryDate(date)) return 0
  if (date === '') return 0
  const match = date.match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (match == null) return 0
  return panelCalendarDateToEpochSeconds(new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]), 0, 0, 0, 0))
}

const validPort = (value: unknown) => {
  const port = toNumber(value)
  return Number.isInteger(port) && port >= 1 && port <= 65535
}

const validatePortExpression = (raw: string) => {
  const parts = raw.replace(/，/g, ',').split(',').map(item => item.trim()).filter(Boolean)
  if (parts.length === 0 || parts.length > 128) return false
  return parts.every(part => {
    const match = part.match(/^(\d+)(?:\s*[-:]\s*(\d+))?$/)
    if (!match || !validPort(match[1])) return false
    return !match[2] || (validPort(match[2]) && Number(match[1]) <= Number(match[2]))
  })
}

const buildRuleValidationError = (form: PortForwardRuleForm): string => {
  if ([...form.name.trim()].length > 120) return tr('validationNameLength')
  if ([...form.description.trim()].length > 1000) return tr('validationDescriptionLength')
  if (form.localPortMode === 'single' && !validPort(form.localPortStart)) return tr('validationPort')
  if (form.localPortMode === 'range' && (!validPort(form.localPortStart) || !validPort(form.localPortEnd) || toNumber(form.localPortEnd) < toNumber(form.localPortStart))) {
    return tr('validationRange')
  }
  if (form.localPortMode === 'multi' && !validatePortExpression(form.localPortSpec)) return tr('validationMulti')
  if (!validPort(form.targetPort)) return tr('validationTargetPort')
  const rate = Number(form.rateLimitMbps)
  if (!Number.isFinite(rate) || !Number.isInteger(rate) || rate < 0 || rate > 1000000) return tr('validationRate')
  const trafficLimit = Number(form.trafficLimitGiB)
  if (!Number.isFinite(trafficLimit) || trafficLimit < 0 || trafficLimit > maxTrafficLimitGiB || Math.abs(trafficLimit * 100 - Math.round(trafficLimit * 100)) > 1e-7) {
    return tr('validationTrafficLimit')
  }
  if (!Number.isInteger(Number(form.trafficResetDay)) || Number(form.trafficResetDay) < 0 || Number(form.trafficResetDay) > 31) return tr('validationTrafficResetDay')
  if (!validTrafficExpiryDate(normalizeTrafficExpiryDate(form.trafficExpiryDate))) return tr('validationTrafficExpiryDate')
  if (!isLocalTargetIP(form.targetIP)) {
    const targetFamily = targetAddressFamily(form.targetIP)
    if (!targetFamily) return tr('validationTargetIP')
    if (form.family === 'dual' || targetFamily !== normalizeFamilyValue(form.family)) return tr('validationFamily')
  }
  return ''
}

export function usePortForwardManage(props: { active?: boolean }) {
  const loading = ref(false)
  const refreshing = ref(false)
  const mutationBusy = ref(false)
  const dialogVisible = ref(false)
  const savingRule = computed(() => mutationBusy.value && dialogVisible.value)
  const pollTimer = ref<number | null>(null)
  const overviewRequest = ref<Promise<Msg> | null>(null)
  const rowBusyId = ref(0)
  const lastWarningSignature = ref('')
  const hasLoaded = ref(false)
  const loadError = ref('')
  const searchText = ref('')
  const familyFilter = ref('all')
  const protocolFilter = ref('all')
  const overview = ref<PortForwardOverview>(emptyOverview())
  const editingRule = ref<PortForwardRuleForm>(createEmptyRuleForm())

  const headers = computed(() => {
    touchLocale()
    return [
      { title: tr('ruleLabel'), key: 'name', sortable: false },
      { title: tr('localLabel'), key: 'local', sortable: false },
      { title: tr('targetLabel'), key: 'target', sortable: false },
      { title: tr('laneLabel'), key: 'lane', sortable: false },
      { title: tr('limitColumn'), key: 'limit', sortable: false },
      { title: tr('trafficColumn'), key: 'traffic', sortable: false },
      { title: tr('actions'), key: 'actions', sortable: false, width: 188 },
    ]
  })
  const familyItems = computed(() => {
    touchLocale()
    return [
      { title: 'IPv4', value: 'ipv4' },
      { title: 'IPv6', value: 'ipv6' },
      { title: 'IPv4/IPv6', value: 'dual' },
    ]
  })
  const familyFilterItems = computed(() => [{ title: tr('allFamilies'), value: 'all' }, ...familyItems.value])
  const protocolItems = computed(() => {
    touchLocale()
    return [
      { title: 'TCP', value: 'tcp' },
      { title: 'UDP', value: 'udp' },
      { title: 'TCP/UDP', value: 'tcp_udp' },
    ]
  })
  const protocolFilterItems = computed(() => [{ title: tr('allProtocols'), value: 'all' }, ...protocolItems.value])
  const localModeItems = computed(() => {
    touchLocale()
    return [
      { title: tr('singleMode'), value: 'single' },
      { title: tr('multiMode'), value: 'multi' },
      { title: tr('rangeMode'), value: 'range' },
    ]
  })

  const applyOverview = (raw: Partial<PortForwardOverview> | null | undefined) => {
    const next = raw ?? {}
    overview.value = {
      ...emptyOverview(),
      ...next,
      supported: typeof next.supported === 'boolean' ? next.supported : Boolean(next.available),
      ready: typeof next.ready === 'boolean' ? next.ready : Boolean(next.available),
      available: Boolean(next.available),
      nftVersion: String(next.nftVersion ?? ''),
      kernelVersion: String(next.kernelVersion ?? ''),
      compatibilityMode: String(next.compatibilityMode ?? 'conservative'),
      rendererSupported: Boolean(next.rendererSupported),
      supportsTransportHeader: Boolean(next.supportsTransportHeader),
      supportsTableComments: Boolean(next.supportsTableComments),
      capabilityError: String(next.capabilityError ?? ''),
      versionProbeError: String(next.versionProbeError ?? ''),
      jsonProbeError: String(next.jsonProbeError ?? ''),
      meterProbeError: String(next.meterProbeError ?? ''),
      layoutPending: Boolean(next.layoutPending),
      lastApplyError: String(next.lastApplyError ?? ''),
      kernelIPv4Forward: Boolean(next.kernelIPv4Forward),
      kernelIPv6Forward: Boolean(next.kernelIPv6Forward),
      enabledCount: toNumber(next.enabledCount),
      limitedCount: toNumber(next.limitedCount),
      totalUp: toNumber(next.totalUp),
      totalDown: toNumber(next.totalDown),
      totalTraffic: toNumber(next.totalTraffic),
      lastSyncAt: toNumber(next.lastSyncAt),
      warnings: normalizeWarnings(next.warnings),
      error: String(next.error ?? ''),
      rules: Array.isArray(next.rules) ? next.rules.map(rule => normalizeRule(rule)) : [],
      runtimeConflicts: Array.isArray(next.runtimeConflicts) ? next.runtimeConflicts.map(conflict => normalizeConflict(conflict)) : [],
    }
    hasLoaded.value = true
    loadError.value = ''
  }

  const lastSyncLabel = computed(() => (
    overview.value.lastSyncAt > 0 ? formatTimestamp(overview.value.lastSyncAt) : '-'
  ))
  const compatibilityModeLabel = computed(() => {
    touchLocale()
    const modeMap: Record<string, string> = {
      native: tr('modeNative'),
      compatibility: tr('modeCompatibility'),
      conservative: tr('modeConservative'),
    }
    const mode = modeMap[overview.value.compatibilityMode] || tr('modeConservative')
    const pending = overview.value.layoutPending ? ` · ${tr('layoutPendingShort')}` : ''
    return `${mode}${pending}`
  })
  const capabilityLabel = computed(() => {
    const nftVersion = overview.value.nftVersion ? `nft ${overview.value.nftVersion}` : tr('nftVersionUnknown')
    const kernelVersion = overview.value.kernelVersion
      ? `${tr('kernelVersionLabel')} ${overview.value.kernelVersion}`
      : tr('kernelVersionUnknown')
    return `${nftVersion} · ${kernelVersion} · ${compatibilityModeLabel.value}`
  })
  const capabilityChipColor = computed(() => {
    if (!overview.value.rendererSupported || overview.value.layoutPending) return 'warning'
    return overview.value.compatibilityMode === 'native' ? 'success' : 'info'
  })
  const dialogTitle = computed(() => (
    editingRule.value.id > 0 ? tr('editTitle') : tr('createTitle')
  ))
  const localStartLabel = computed(() => (
    editingRule.value.localPortMode === 'single' ? tr('portLabel') : tr('startLabel')
  ))
  const localPreviewText = computed(() => {
    const form = editingRule.value
    let localSpec = '-'
    if (form.localPortMode === 'single') localSpec = String(form.localPortStart || 0)
    else if (form.localPortMode === 'multi') localSpec = form.localPortSpec.trim() || '-'
    else localSpec = `${form.localPortStart || 0}-${form.localPortEnd || 0}`
    return `${tr('localLabel')}: ${localSpec} → ${targetDisplayLabel(form.targetIP, form.targetPort)}`
  })
  const formError = computed(() => buildRuleValidationError(editingRule.value))
  const ruleTrafficResetPickerEpoch = computed(() => trafficResetPickerEpoch(editingRule.value.trafficResetDay))
  const ruleTrafficExpiryPickerEpoch = computed(() => trafficExpiryPickerEpoch(editingRule.value.trafficExpiryDate))
  const overviewResetBusy = computed(() => mutationBusy.value && rowBusyId.value === -1)
  const submitRuleTrafficResetDay = (rawValue: unknown) => {
    const epochSeconds = parseEpochSeconds(rawValue)
    if (epochSeconds == null) return
    if (epochSeconds <= 0) {
      editingRule.value.trafficResetDay = 0
      return
    }
    editingRule.value.trafficResetDay = normalizeTrafficResetDay(panelCalendarParts(epochSeconds * 1000).day)
  }
  const submitRuleTrafficExpiryDate = (rawValue: unknown) => {
    const epochSeconds = parseEpochSeconds(rawValue)
    if (epochSeconds == null) return
    if (epochSeconds <= 0) {
      editingRule.value.trafficExpiryDate = ''
      return
    }
    const selected = panelCalendarParts(epochSeconds * 1000)
    editingRule.value.trafficExpiryDate = `${selected.year.toString().padStart(4, '0')}-${selected.month.toString().padStart(2, '0')}-${selected.day.toString().padStart(2, '0')}`
  }
  const filteredRules = computed(() => {
    const keyword = searchText.value.trim().toLowerCase()
    return overview.value.rules.filter(rule => {
      if (familyFilter.value !== 'all' && rule.family !== familyFilter.value) return false
      if (protocolFilter.value !== 'all' && rule.protocol !== protocolFilter.value) return false
      if (!keyword) return true
      return [rule.name, rule.description, rule.localPortSpec, rule.limitWarning, rule.trafficBlockReason, rule.targetIP, String(rule.targetPort)]
        .some(value => value.toLowerCase().includes(keyword))
    })
  })

  const conflictsForRule = (ruleID: number) => overview.value.runtimeConflicts.filter(item => item.ruleId === ruleID)
  const formatConflictOwners = (conflict: PortForwardRuntimeConflict) => {
    if (!conflict.owners.length) return tr('unknownProcess')
    return conflict.owners.map(owner => owner.name ? `${owner.name} (${tr('pid')} ${owner.pid})` : `${tr('pid')} ${owner.pid}`).join(', ')
  }

  const handleWarnings = (warnings: string[], showToast: boolean) => {
    const signature = warnings.join('；')
    if (!signature) {
      lastWarningSignature.value = ''
      return
    }
    if (!showToast || signature === lastWarningSignature.value) return
    lastWarningSignature.value = signature
    push.warning({ duration: 6000, message: signature })
  }

  const fetchOverview = async (silent = false, showWarnings = !silent) => {
    if (silent && (!props.active || (typeof document !== 'undefined' && document.visibilityState !== 'visible'))) {
      return { success: false, msg: '', obj: null } as Msg
    }
    if (overviewRequest.value) return overviewRequest.value
    if (!silent) loading.value = true
    const request = (async () => {
      try {
        const msg = await HttpUtils.get('api/port-forward-overview', {}, { silentAuthCheck: true })
        if (msg.success && msg.obj) {
          const nextOverview = msg.obj as Partial<PortForwardOverview>
          applyOverview(nextOverview)
          handleWarnings(normalizeWarnings(nextOverview.warnings), showWarnings)
        } else {
          loadError.value = msg.msg || tr('loadFailed')
        }
        return msg
      } catch (error) {
        const message = errorMessage(error, tr('loadFailed'))
        loadError.value = message
        return { success: false, msg: message, obj: null } as Msg
      }
    })()
    overviewRequest.value = request
    try {
      return await request
    } finally {
      if (overviewRequest.value === request) overviewRequest.value = null
      if (!silent) loading.value = false
    }
  }

  const refreshOverview = async () => {
    if (mutationBusy.value) return
    refreshing.value = true
    try {
      await fetchOverview(true, true)
    } finally {
      refreshing.value = false
    }
  }

  const openRuleDialog = (rule?: PortForwardRule) => {
    if (mutationBusy.value) return
    editingRule.value = mapRuleToForm(rule)
    dialogVisible.value = true
  }
  const closeRuleDialog = () => {
    if (!mutationBusy.value) dialogVisible.value = false
  }

  const beginMutation = (ruleID = 0) => {
    if (mutationBusy.value) return false
    mutationBusy.value = true
    rowBusyId.value = ruleID
    return true
  }
  const endMutation = () => {
    rowBusyId.value = 0
    mutationBusy.value = false
  }

  const saveRule = async () => {
    const validation = formError.value
    if (validation) {
      push.warning({ duration: 4500, message: validation })
      return
    }
    if (!beginMutation(editingRule.value.id)) return
    const wasEditing = editingRule.value.id > 0
    const payload = buildPayload(editingRule.value)
    try {
      const msg = await HttpUtils.post('api/port-forward-rule', payload, {
        headers: { 'Content-Type': 'application/json' },
        silentAuthCheck: true,
      })
      if (msg.success && msg.obj) {
        const nextOverview = msg.obj as Partial<PortForwardOverview>
        applyOverview(nextOverview)
        dialogVisible.value = false
        push.success({ duration: 4000, message: wasEditing ? tr('updated') : tr('created') })
        const savedRule = overview.value.rules.find(rule => rule.id === payload.id || (!payload.id && rule.name === payload.name))
        if (savedRule?.limitStatus === 'degraded' && savedRule.limitWarning) {
          push.warning({ duration: 6000, message: savedRule.limitWarning })
        }
        handleWarnings(normalizeWarnings(nextOverview.warnings), true)
      } else {
        push.warning({ duration: 6000, message: msg.msg || tr('saveFailed') })
      }
    } catch (error) {
      push.warning({ duration: 6000, message: errorMessage(error, tr('saveFailed')) })
    } finally {
      endMutation()
    }
  }

  const toggleRule = async (rule: PortForwardRule, enabled: boolean) => {
    if (!beginMutation(rule.id)) return
    try {
      const msg = await HttpUtils.post('api/port-forward-rule', {
        ...buildPayload(mapRuleToForm(rule)),
        enabled,
      }, {
        headers: { 'Content-Type': 'application/json' },
        silentAuthCheck: true,
      })
      if (msg.success && msg.obj) {
        const nextOverview = msg.obj as Partial<PortForwardOverview>
        applyOverview(nextOverview)
        handleWarnings(normalizeWarnings(nextOverview.warnings), true)
      } else {
        push.warning({ duration: 6000, message: msg.msg || tr('saveFailed') })
      }
    } catch (error) {
      push.warning({ duration: 6000, message: errorMessage(error, tr('saveFailed')) })
    } finally {
      endMutation()
    }
  }

  const removeRule = async (rule: PortForwardRule) => {
    if (mutationBusy.value) return
    const confirmed = await confirm({
      message: tr('deleteConfirm', { name: rule.name || tr('ruleFallback') }),
      severity: 'danger',
      confirmText: tr('delete'),
    })
    if (!confirmed || !beginMutation(rule.id)) return
    try {
      const msg = await HttpUtils.post('api/port-forward-rule-delete', { id: rule.id }, {
        headers: { 'Content-Type': 'application/json' },
        silentAuthCheck: true,
      })
      if (msg.success && msg.obj) {
        applyOverview(msg.obj as Partial<PortForwardOverview>)
        push.success({ duration: 4000, message: tr('deleted') })
      } else {
        push.warning({ duration: 6000, message: msg.msg || tr('deleteFailed') })
      }
    } catch (error) {
      push.warning({ duration: 6000, message: errorMessage(error, tr('deleteFailed')) })
    } finally {
      endMutation()
    }
  }

  const resetRuleTraffic = async (rule: PortForwardRule) => {
    if (mutationBusy.value) return
    const confirmed = await confirm({
      message: tr('resetRuleTrafficConfirm', { name: rule.name || tr('ruleFallback') }),
      severity: 'danger',
      confirmText: tr('resetTraffic'),
    })
    if (!confirmed || !beginMutation(rule.id)) return
    try {
      const msg = await HttpUtils.post('api/port-forward-rule-traffic-reset', { id: rule.id }, {
        headers: { 'Content-Type': 'application/json' },
        silentAuthCheck: true,
      })
      if (msg.success && msg.obj) {
        applyOverview(msg.obj as Partial<PortForwardOverview>)
        push.success({ duration: 4000, message: tr('ruleTrafficReset') })
      } else {
        push.warning({ duration: 6000, message: msg.msg || tr('resetTrafficFailed') })
      }
    } catch (error) {
      push.warning({ duration: 6000, message: errorMessage(error, tr('resetTrafficFailed')) })
    } finally {
      endMutation()
    }
  }

  const resetOverviewTraffic = async () => {
    if (mutationBusy.value) return
    const confirmed = await confirm({
      message: tr('resetOverviewTrafficConfirm'),
      severity: 'danger',
      confirmText: tr('resetTraffic'),
    })
    if (!confirmed || !beginMutation(-1)) return
    try {
      const msg = await HttpUtils.post('api/port-forward-overview-traffic-reset', {}, {
        headers: { 'Content-Type': 'application/json' },
        silentAuthCheck: true,
      })
      if (msg.success && msg.obj) {
        applyOverview(msg.obj as Partial<PortForwardOverview>)
        push.success({ duration: 4000, message: tr('overviewTrafficReset') })
      } else {
        push.warning({ duration: 6000, message: msg.msg || tr('resetTrafficFailed') })
      }
    } catch (error) {
      push.warning({ duration: 6000, message: errorMessage(error, tr('resetTrafficFailed')) })
    } finally {
      endMutation()
    }
  }

  const stopPolling = () => {
    if (pollTimer.value != null) {
      window.clearTimeout(pollTimer.value)
      pollTimer.value = null
    }
  }
  const schedulePolling = (delay = 10000) => {
    stopPolling()
    if (!props.active || (typeof document !== 'undefined' && document.visibilityState !== 'visible')) return
    pollTimer.value = window.setTimeout(async () => {
      pollTimer.value = null
      const msg = await fetchOverview(true)
      schedulePolling(msg.success ? 10000 : 30000)
    }, delay)
  }
  const startPolling = () => schedulePolling()
  const handleVisibilityChange = () => {
    if (document.visibilityState === 'visible' && props.active) {
      void fetchOverview(true, true)
      startPolling()
      return
    }
    stopPolling()
  }

  watch(() => props.active, (active) => {
    if (active) {
      void fetchOverview(true, true)
      startPolling()
      return
    }
    stopPolling()
  })
  onMounted(() => {
    if (props.active) void fetchOverview()
    startPolling()
    if (typeof document !== 'undefined') document.addEventListener('visibilitychange', handleVisibilityChange)
  })
  onBeforeUnmount(() => {
    stopPolling()
    if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', handleVisibilityChange)
  })

  return {
    loading,
    refreshing,
    savingRule,
    mutationBusy,
    dialogVisible,
    rowBusyId,
    hasLoaded,
    loadError,
    searchText,
    familyFilter,
    protocolFilter,
    overview,
    editingRule,
    headers,
    familyItems,
    familyFilterItems,
    protocolItems,
    protocolFilterItems,
    localModeItems,
    lastSyncLabel,
    capabilityLabel,
    capabilityChipColor,
    compatibilityModeLabel,
    dialogTitle,
    localStartLabel,
    localPreviewText,
    formError,
    ruleTrafficResetPickerEpoch,
    ruleTrafficExpiryPickerEpoch,
    overviewResetBusy,
    filteredRules,
    conflictsForRule,
    formatConflictOwners,
    refreshOverview,
    openRuleDialog,
    closeRuleDialog,
    saveRule,
    toggleRule,
    removeRule,
    resetRuleTraffic,
    resetOverviewTraffic,
    submitRuleTrafficResetDay,
    submitRuleTrafficExpiryDate,
    t: tr,
  }
}
