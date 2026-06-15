import HttpUtils from '@/plugins/httputil'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { push } from 'notivue'

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
  effectiveRateLimitMbps: number
  limitStatus: string
  limitWarning: string
  currentUp: number
  currentDown: number
  currentTotal: number
}

type PortForwardOverview = {
  available: boolean
  lastSyncAt: number
  kernelIPv4Forward: boolean
  kernelIPv6Forward: boolean
  enabledCount: number
  limitedCount: number
  totalUp: number
  totalDown: number
  totalTraffic: number
  rules: PortForwardRule[]
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
}

export const copy = {
  heroEyebrow: 'NFTABLES FORWARDING',
  title: '\u7aef\u53e3\u8f6c\u53d1',
  subtitle: '\u5c06\u672c\u673a\u5165\u7ad9\u7aef\u53e3\u6620\u5c04\u5230\u8fdc\u7a0b IP:\u7aef\u53e3\u6216\u672c\u673a\u53e6\u4e00\u4e2a\u7aef\u53e3\uff0c\u53ef\u9009 TCP / UDP\u3001IPv4 / IPv6 \u4e0e\u7aef\u53e3\u9650\u901f\u3002',
  refresh: '\u7acb\u5373\u5237\u65b0',
  newRule: '\u65b0\u5efa\u8f6c\u53d1',
  available: '\u53ef\u7528',
  unavailable: '\u4e0d\u53ef\u7528',
  unavailableHint: '\u5f53\u524d\u7cfb\u7edf\u4e0d\u652f\u6301 nftables \u8f6c\u53d1\u4e0b\u53d1\uff08\u4ec5 Linux \u652f\u6301\uff09\u3002\u4f60\u4ecd\u53ef\u4ee5\u7ef4\u62a4\u89c4\u5219\u7528\u4e8e\u8c03\u8bd5\uff0c\u4f46\u8fd0\u884c\u65f6\u4e0d\u4f1a\u751f\u6548\u3002',
  lastSync: '\u4e0a\u6b21\u540c\u6b65',
  ruleCount: '\u89c4\u5219',
  enabledRules: '\u5df2\u542f\u7528\u89c4\u5219',
  limitedRules: '\u9650\u901f\u89c4\u5219',
  totalTraffic: '\u603b\u6d41\u91cf',
  runtimeTitle: '\u8fd0\u884c\u6001',
  kernelIPv4: 'IPv4 Forward',
  kernelIPv6: 'IPv6 Forward',
  forwardOn: '\u5df2\u6253\u5f00',
  forwardOff: '\u672a\u6253\u5f00',
  totalUpload: '\u603b\u4e0a\u884c',
  totalDownload: '\u603b\u4e0b\u884c',
  runtimeHint: '\u4e0a\u884c\u4ee3\u8868\u8fdc\u7aef\u56de\u5305\uff0c\u4e0b\u884c\u4ee3\u8868\u8fdb\u5165\u672c\u673a\u8f6c\u53d1\u7aef\u53e3\u7684\u6d41\u91cf\u3002\u9650\u901f\u53ea\u4f5c\u7528\u4e8e\u5de6\u4fa7\u672c\u5730\u7aef\u53e3\u3002',
  tableTitle: '\u8f6c\u53d1\u89c4\u5219',
  tableSubtitle: '\u652f\u6301\u5355\u7aef\u53e3\u3001\u591a\u7aef\u53e3\u4e0e\u7aef\u53e3\u8303\u56f4\uff0c\u53f3\u4fa7\u76ee\u6807\u7aef\u53e3\u56fa\u5b9a\u4e3a\u5355\u7aef\u53e3\u3002',
  searchLabel: '\u641c\u7d22\u540d\u79f0 / \u672c\u5730\u7aef\u53e3 / \u76ee\u6807',
  familyFilter: '\u53cc\u6808\u7b5b\u9009',
  protocolFilter: '\u534f\u8bae\u7b5b\u9009',
  allFamilies: '\u5168\u90e8\u53cc\u6808',
  allProtocols: '\u5168\u90e8\u534f\u8bae',
  ruleLabel: '\u89c4\u5219',
  localLabel: '\u672c\u5730\u7aef\u53e3',
  targetLabel: '\u51fa\u5411\u76ee\u6807',
  laneLabel: '\u901a\u9053',
  limitColumn: '\u9650\u5236',
  trafficColumn: '\u7edf\u8ba1',
  actions: '\u64cd\u4f5c',
  enabled: '\u542f\u7528',
  disabled: '\u505c\u7528',
  ruleFallback: '\u672a\u547d\u540d\u89c4\u5219',
  leftPortOnly: '\u4ec5\u9650\u5236\u5de6\u4fa7\u672c\u5730\u7aef\u53e3',
  up: '\u4e0a',
  down: '\u4e0b',
  emptyText: '\u5f53\u524d\u6ca1\u6709\u5339\u914d\u5230\u7684\u8f6c\u53d1\u89c4\u5219',
  createTitle: '\u65b0\u5efa\u7aef\u53e3\u8f6c\u53d1',
  editTitle: '\u7f16\u8f91\u7aef\u53e3\u8f6c\u53d1',
  dialogSubtitle: '\u5de6\u4fa7\u914d\u7f6e\u672c\u673a\u5165\u53e3\u4e0e\u8bbf\u95ee\u9650\u5236\uff0c\u53f3\u4fa7\u914d\u7f6e\u8f6c\u53d1\u76ee\u6807 IP:\u7aef\u53e3\uff1b\u76ee\u6807 IP \u7559\u7a7a\u6216\u586b\u672c\u5730 IP \u65f6\u89c6\u4e3a\u8f6c\u53d1\u5230\u672c\u673a\u7aef\u53e3\u3002',
  nameLabel: '\u540d\u79f0',
  descLabel: '\u8bf4\u660e',
  localPanelTitle: '\u672c\u5730\u5165\u53e3',
  localPanelHint: '\u5355\u6761\u89c4\u5219\u53ef\u5b9a\u4e49\u5165\u7ad9\u534f\u8bae\u3001IP \u6808\u3001\u5355\u7aef\u53e3 / \u591a\u7aef\u53e3 / \u7aef\u53e3\u8303\u56f4\u4e0e\u5165\u53e3\u9650\u901f\u3002',
  modeLabel: '\u7aef\u53e3\u6a21\u5f0f',
  singleMode: '\u5355\u7aef\u53e3',
  multiMode: '\u591a\u7aef\u53e3',
  rangeMode: '\u7aef\u53e3\u8303\u56f4',
  startLabel: '\u8d77\u59cb\u7aef\u53e3',
  portLabel: '\u7aef\u53e3',
  multiLabel: '\u591a\u7aef\u53e3',
  multiPlaceholder: '66,88,99',
  rangeEndLabel: '\u7ed3\u675f\u7aef\u53e3',
  targetPanelTitle: '\u51fa\u5411\u76ee\u6807',
  targetPanelHint: '\u8fd9\u91cc\u53ea\u914d\u7f6e\u8f6c\u53d1\u540e\u7684\u76ee\u6807 IP:\u7aef\u53e3\uff1b\u76ee\u6807\u7aef\u53e3\u56fa\u5b9a\u4e3a\u5355\u7aef\u53e3\uff0c\u76ee\u6807 IP \u7559\u7a7a\u3001`127.0.0.1`\u3001`::1` \u7b49\u672c\u5730\u5730\u5740\u65f6\uff0c\u8868\u793a\u8f6c\u53d1\u5230\u672c\u673a\u53e6\u4e00\u4e2a\u7aef\u53e3\u3002',
  protocolLabel: '\u534f\u8bae',
  familyLabel: '\u53cc\u6808',
  targetIPLabel: '\u76ee\u6807 IP',
  targetPortLabel: '\u76ee\u6807\u7aef\u53e3',
  rateLabel: '\u5165\u53e3\u9650\u901f Mbps',
  rateHint: '\u4e0d\u586b\u6216\u586b 0 \u8868\u793a\u4e0d\u9650\u901f\uff0c\u4e14\u4e0d\u751f\u6210\u9650\u901f\u89c4\u5219\u3002\u586b\u5199\u540e\uff0c\u8be5\u89c4\u5219\u547d\u4e2d\u7684\u6bcf\u4e2a\u7aef\u53e3\u90fd\u6309\u8be5\u503c\u9650\u901f\uff0cTCP/UDP \u4e92\u4e0d\u8986\u76d6\u3002',
  cancel: '\u53d6\u6d88',
  save: '\u4fdd\u5b58',
  deleteConfirm: '\u786e\u5b9a\u5220\u9664\u89c4\u5219 {name} \u5417\uff1f',
  unlimited: '\u4e0d\u9650\u901f',
  effectiveZero: '0 Mbps',
  limitDegraded: '\u9650\u901f\u672a\u751f\u6548',
  localTarget: '\u672c\u673a',
}

export const headers = [
  { title: copy.ruleLabel, key: 'name', sortable: false },
  { title: copy.localLabel, key: 'local', sortable: false },
  { title: copy.targetLabel, key: 'target', sortable: false },
  { title: copy.laneLabel, key: 'lane', sortable: false },
  { title: copy.limitColumn, key: 'limit', sortable: false },
  { title: copy.trafficColumn, key: 'traffic', sortable: false },
  { title: copy.actions, key: 'actions', sortable: false, width: 150 },
]

export const familyItems = [
  { title: 'IPv4', value: 'ipv4' },
  { title: 'IPv6', value: 'ipv6' },
  { title: 'IPv4/IPv6', value: 'dual' },
]

export const familyFilterItems = [
  { title: copy.allFamilies, value: 'all' },
  ...familyItems,
]

export const protocolItems = [
  { title: 'TCP', value: 'tcp' },
  { title: 'UDP', value: 'udp' },
  { title: 'TCP/UDP', value: 'tcp_udp' },
]

export const protocolFilterItems = [
  { title: copy.allProtocols, value: 'all' },
  ...protocolItems,
]

export const localModeItems = [
  { title: copy.singleMode, value: 'single' },
  { title: copy.multiMode, value: 'multi' },
  { title: copy.rangeMode, value: 'range' },
]

const emptyOverview = (): PortForwardOverview => ({
  available: true,
  lastSyncAt: 0,
  kernelIPv4Forward: false,
  kernelIPv6Forward: false,
  enabledCount: 0,
  limitedCount: 0,
  totalUp: 0,
  totalDown: 0,
  totalTraffic: 0,
  rules: [],
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
  effectiveRateLimitMbps: toNumber(raw.effectiveRateLimitMbps),
  limitStatus: String(raw.limitStatus ?? ''),
  limitWarning: String(raw.limitWarning ?? ''),
  currentUp: toNumber(raw.currentUp),
  currentDown: toNumber(raw.currentDown),
  currentTotal: toNumber(raw.currentTotal),
})

const normalizeWarnings = (raw: unknown): string[] => (
  Array.isArray(raw) ? raw.map(item => String(item ?? '').trim()).filter(Boolean) : []
)

const formatTimestamp = (value: number) => {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

const mapRuleToForm = (rule?: PortForwardRule): PortForwardRuleForm => ({
  id: rule?.id ?? 0,
  name: rule?.name ?? '',
  description: rule?.description ?? '',
  enabled: rule?.enabled ?? true,
  family: normalizeFamilyValue(rule?.family),
  protocol: normalizeProtocolValue(rule?.protocol),
  localPortMode: rule?.localPortMode === 'count' ? 'range' : (rule?.localPortMode ?? 'single'),
  localPortSpec: rule?.localPortSpec ?? '',
  localPortStart: rule?.localPortStart ?? 0,
  localPortCount: rule?.localPortCount ?? 1,
  localPortEnd: rule?.localPortEnd ?? 0,
  targetIP: isLocalTargetIP(rule?.targetIP ?? '') ? '' : (rule?.targetIP ?? ''),
  targetPort: rule?.targetPort ?? 0,
  rateLimitMbps: rule?.rateLimitMbps ?? 0,
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

export const targetDisplayLabel = (targetIP: string, targetPort: number) => {
  if (isLocalTargetIP(targetIP)) {
    return `${copy.localTarget}:${targetPort || 0}`
  }
  return `${targetIP}:${targetPort || 0}`
}

export const localModeLabel = (value: string) => {
  if (value === 'multi') return copy.multiMode
  if (value === 'range') return copy.rangeMode
  return copy.singleMode
}

export const rateLimitLabel = (effectiveValue: number, configuredValue = 0, status = '') => {
  if (configuredValue > 0 && effectiveValue <= 0 && status === 'degraded') {
    return copy.effectiveZero
  }
  return effectiveValue > 0 ? `${effectiveValue} Mbps` : copy.unlimited
}

export const formatBytes = (value: number) => {
  if (!Number.isFinite(value) || value <= 0) {
    return '0 B'
  }
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

export function usePortForwardManage(props: { active?: boolean }) {
  const loading = ref(false)
  const refreshing = ref(false)
  const savingRule = ref(false)
  const dialogVisible = ref(false)
  const pollTimer = ref<number | null>(null)
  const rowBusyId = ref(0)
  const lastWarningSignature = ref('')
  const searchText = ref('')
  const familyFilter = ref('all')
  const protocolFilter = ref('all')
  const overview = ref<PortForwardOverview>(emptyOverview())
  const editingRule = ref<PortForwardRuleForm>(createEmptyRuleForm())

  const applyOverview = (raw: Partial<PortForwardOverview> | null | undefined) => {
    const next = raw ?? {}
    overview.value = {
      ...emptyOverview(),
      ...next,
      available: next.available !== false,
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
    }
  }

  const lastSyncLabel = computed(() => (
    overview.value.lastSyncAt > 0 ? formatTimestamp(overview.value.lastSyncAt) : '-'
  ))

  const dialogTitle = computed(() => (
    editingRule.value.id > 0 ? copy.editTitle : copy.createTitle
  ))

  const localStartLabel = computed(() => (
    editingRule.value.localPortMode === 'single' ? copy.portLabel : copy.startLabel
  ))

  const localPreviewText = computed(() => {
    const form = editingRule.value
    let localSpec = '-'
    if (form.localPortMode === 'single') {
      localSpec = String(form.localPortStart || 0)
    } else if (form.localPortMode === 'multi') {
      localSpec = form.localPortSpec.trim() || '-'
    } else {
      localSpec = `${form.localPortStart || 0}-${form.localPortEnd || 0}`
    }
    const previewTarget = targetDisplayLabel(editingRule.value.targetIP || '', editingRule.value.targetPort)
    return `${copy.localLabel}: ${localSpec}  ->  ${previewTarget}`
  })

  const filteredRules = computed(() => {
    const keyword = searchText.value.trim().toLowerCase()
    return overview.value.rules.filter(rule => {
      if (familyFilter.value !== 'all' && rule.family !== familyFilter.value) {
        return false
      }
      if (protocolFilter.value !== 'all' && rule.protocol !== protocolFilter.value) {
        return false
      }
      if (!keyword) {
        return true
      }
      return [
        rule.name,
        rule.description,
        rule.localPortSpec,
        rule.limitWarning,
        rule.targetIP,
        String(rule.targetPort),
      ].some(value => value.toLowerCase().includes(keyword))
    })
  })

  const handleWarnings = (warnings: string[], showToast: boolean) => {
    const signature = warnings.join('；')
    if (!signature) {
      lastWarningSignature.value = ''
      return
    }
    if (!showToast || signature === lastWarningSignature.value) {
      return
    }
    lastWarningSignature.value = signature
    push.warning({
      duration: 6000,
      message: signature,
    })
  }

  const fetchOverview = async (silent = false, showWarnings = !silent) => {
    if (!silent) {
      loading.value = true
    }
    try {
      const msg = await HttpUtils.get('api/port-forward-overview')
      if (msg.success && msg.obj) {
        const nextOverview = msg.obj as Partial<PortForwardOverview>
        applyOverview(nextOverview)
        handleWarnings(normalizeWarnings(nextOverview.warnings), showWarnings)
      }
    } finally {
      if (!silent) {
        loading.value = false
      }
    }
  }

  const refreshOverview = async () => {
    refreshing.value = true
    try {
      await fetchOverview(true, true)
    } finally {
      refreshing.value = false
    }
  }

  const openRuleDialog = (rule?: PortForwardRule) => {
    editingRule.value = mapRuleToForm(rule)
    dialogVisible.value = true
  }

  const saveRule = async () => {
    if (savingRule.value) {
      return
    }
    if (editingRule.value.localPortMode === 'multi' && !editingRule.value.localPortSpec.trim()) {
      push.warning({
        duration: 4000,
        message: '请填写多端口，例如 66,88,99',
      })
      return
    }
    savingRule.value = true
    try {
      const msg = await HttpUtils.post('api/port-forward-rule', buildPayload(editingRule.value), {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        const nextOverview = msg.obj as Partial<PortForwardOverview>
        applyOverview(nextOverview)
        dialogVisible.value = false
        const ruleWarnings = normalizeWarnings(nextOverview.warnings)
        const degraded = overview.value.rules.find(rule => rule.id === editingRule.value.id || (editingRule.value.id === 0 && rule.name === buildPayload(editingRule.value).name))
        push.success({
          duration: 4000,
          message: editingRule.value.id > 0 ? '端口转发已更新' : '端口转发已创建',
        })
        if (degraded?.limitStatus === 'degraded' && degraded.limitWarning) {
          push.warning({
            duration: 6000,
            message: degraded.limitWarning,
          })
        }
        handleWarnings(ruleWarnings, true)
      }
    } finally {
      savingRule.value = false
    }
  }

  const toggleRule = async (rule: PortForwardRule, enabled: boolean) => {
    if (rowBusyId.value === rule.id) {
      return
    }
    rowBusyId.value = rule.id
    try {
      const msg = await HttpUtils.post('api/port-forward-rule', {
        ...buildPayload(mapRuleToForm(rule)),
        enabled,
      }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        const nextOverview = msg.obj as Partial<PortForwardOverview>
        applyOverview(nextOverview)
        handleWarnings(normalizeWarnings(nextOverview.warnings), true)
      }
    } finally {
      rowBusyId.value = 0
    }
  }

  const removeRule = async (rule: PortForwardRule) => {
    if (rowBusyId.value === rule.id) {
      return
    }
    const confirmed = window.confirm(copy.deleteConfirm.replace('{name}', rule.name || copy.ruleFallback))
    if (!confirmed) {
      return
    }
    rowBusyId.value = rule.id
    try {
      const msg = await HttpUtils.post('api/port-forward-rule-delete', { id: rule.id }, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
      if (msg.success && msg.obj) {
        applyOverview(msg.obj as Partial<PortForwardOverview>)
      }
    } finally {
      rowBusyId.value = 0
    }
  }

  const stopPolling = () => {
    if (pollTimer.value != null) {
      window.clearInterval(pollTimer.value)
      pollTimer.value = null
    }
  }

  const startPolling = () => {
    stopPolling()
    if (!props.active) {
      return
    }
    if (typeof document !== 'undefined' && document.visibilityState !== 'visible') {
      return
    }
    pollTimer.value = window.setInterval(() => {
      fetchOverview(true)
    }, 4000)
  }

  const handleVisibilityChange = () => {
    if (document.visibilityState === 'visible') {
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
    savingRule,
    dialogVisible,
    rowBusyId,
    searchText,
    familyFilter,
    protocolFilter,
    overview,
    editingRule,
    lastSyncLabel,
    dialogTitle,
    localStartLabel,
    localPreviewText,
    filteredRules,
    refreshOverview,
    openRuleDialog,
    saveRule,
    toggleRule,
    removeRule,
  }
}
