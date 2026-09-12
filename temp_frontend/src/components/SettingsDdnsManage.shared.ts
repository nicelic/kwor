import { ref, computed, watch, onBeforeUnmount, type Ref } from 'vue'
import HttpUtils, { type Msg } from '@/plugins/httputil'
import { push } from 'notivue'
import { confirm } from '@/plugins/confirm'
import { i18n } from '@/locales'
import type {
  DDNSOverview,
  DDNSRule,
  DDNSAccount,
  ProviderMeta
} from '@/types/ddns'

const ddnsJsonHeaders = { headers: { 'Content-Type': 'application/json' } }

export function useDdnsManage(active: Ref<boolean>) {
  const loading = ref(false)
  const refreshing = ref(false)
  const mutationBusy = ref(false)
  const testAuthBusy = ref(false)
  const deletingRuleId = ref<number | null>(null)
  const syncingRuleId = ref<number | null>(null)
  const searchText = ref('')
  const statusFilter = ref<'all' | 'enabled' | 'disabled' | 'success' | 'error' | 'pending'>('all')

  const overview = ref<DDNSOverview>({
    rules: [],
    accounts: [],
    providers: [],
    interfaces: [],
  })

  // Rule Dialog State
  const ruleDialogVisible = ref(false)
  const ruleForm = ref<DDNSRule>({
    id: 0,
    name: '',
    enabled: true,
    accountId: 0,
    domains: '',
    ipType: 'ipv4',
    ipv4Source: 'api',
    ipv4Url: '',
    ipv4Interface: '',
    ipv6Source: 'disabled',
    ipv6Url: '',
    ipv6Interface: '',
    intervalMinutes: 1,
    localIntervalSeconds: 10,
    ttl: 60,
    cloudflareProxy: false,
    lastIpv4: '',
    lastIpv6: '',
    lastStatus: 'pending',
    lastError: '',
  })
  const ipv4Sources = ref<string[]>(['api'])
  const ipv4Urls = ref<string[]>([])
  const ipv6Sources = ref<string[]>(['interface'])
  const ipv6Urls = ref<string[]>([])

  // Account Dialog State
  const accountDialogVisible = ref(false)
  const accountManageVisible = ref(false)
  const accountForm = ref<DDNSAccount>({
    id: 0,
    name: '',
    providerCode: 'cloudflare',
    envJson: '{}',
    remark: '',
  })
  const accountEnvMap = ref<Record<string, string>>({})

  let pollTimer: ReturnType<typeof setInterval> | null = null

  const selectedProvider = computed<ProviderMeta | undefined>(() => {
    return overview.value.providers.find((p) => p.code === accountForm.value.providerCode)
  })

  async function fetchOverview(silent = false) {
    if (!silent) {
      loading.value = true
    }
    try {
      const resp = await HttpUtils.get('api/ddns-overview', {}, { silentErrorToast: silent })
      if (resp && resp.success && resp.obj) {
        overview.value = resp.obj as DDNSOverview
      }
    } catch {
      // Handled by HttpUtils
    } finally {
      if (!silent) {
        loading.value = false
      }
      refreshing.value = false
    }
  }

  function startPolling() {
    stopPolling()
    pollTimer = setInterval(() => {
      if (active.value) {
        fetchOverview(true)
      }
    }, 30000)
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  watch(
    active,
    (val) => {
      if (val) {
        fetchOverview()
        startPolling()
      } else {
        stopPolling()
      }
    },
    { immediate: true }
  )

  onBeforeUnmount(() => {
    stopPolling()
  })

  function openRuleDialog(target?: DDNSRule) {
    if (target) {
      ruleForm.value = JSON.parse(JSON.stringify(target))
      // 反序列化 IPv4 来源
      const v4Src = (ruleForm.value.ipv4Source || '').trim()
      if (v4Src === 'both') {
        ipv4Sources.value = ['interface', 'api']
      } else if (v4Src) {
        const parts = v4Src.split(',').map((s) => s.trim()).filter(Boolean)
        ipv4Sources.value = parts.length > 0 ? parts : ['api']
      } else {
        ipv4Sources.value = ['api']
      }

      // 反序列化 IPv4 URLs
      if (ruleForm.value.ipv4Url) {
        ipv4Urls.value = ruleForm.value.ipv4Url
          .split(/[,;\n]+/)
          .map((s) => s.trim())
          .filter(Boolean)
      } else {
        ipv4Urls.value = []
      }

      // 反序列化 IPv6 来源
      const v6Src = (ruleForm.value.ipv6Source || '').trim()
      if (v6Src === 'both') {
        ipv6Sources.value = ['interface', 'api']
      } else if (v6Src && v6Src !== 'disabled') {
        const parts = v6Src.split(',').map((s) => s.trim()).filter(Boolean)
        ipv6Sources.value = parts.length > 0 ? parts : ['interface']
      } else {
        ipv6Sources.value = ['interface']
      }

      // 反序列化 IPv6 URLs
      if (ruleForm.value.ipv6Url) {
        ipv6Urls.value = ruleForm.value.ipv6Url
          .split(/[,;\n]+/)
          .map((s) => s.trim())
          .filter(Boolean)
      } else {
        ipv6Urls.value = []
      }

      if (!ruleForm.value.localIntervalSeconds || ruleForm.value.localIntervalSeconds <= 0) {
        ruleForm.value.localIntervalSeconds = 10
      }
      if (!ruleForm.value.intervalMinutes || ruleForm.value.intervalMinutes <= 0) {
        ruleForm.value.intervalMinutes = 1
      }
      if (!ruleForm.value.ttl || ruleForm.value.ttl <= 0) {
        ruleForm.value.ttl = 60
      }
    } else {
      const defaultAccount = overview.value.accounts.length > 0 ? overview.value.accounts[0].id : 0
      ruleForm.value = {
        id: 0,
        name: '',
        enabled: true,
        accountId: defaultAccount,
        domains: '',
        ipType: 'ipv4',
        ipv4Source: 'api',
        ipv4Url: '',
        ipv4Interface: '',
        ipv6Source: 'disabled',
        ipv6Url: '',
        ipv6Interface: '',
        intervalMinutes: 1,
        localIntervalSeconds: 10,
        ttl: 60,
        cloudflareProxy: false,
        lastIpv4: '',
        lastIpv6: '',
        lastStatus: 'pending',
        lastError: '',
      }
      ipv4Sources.value = ['api']
      ipv4Urls.value = []
      ipv6Sources.value = ['interface']
      ipv6Urls.value = []
    }
    ruleDialogVisible.value = true
  }

  async function saveRule() {
    if (!ruleForm.value.name.trim()) {
      push.error(i18n.global.t('ddns.ruleNameRequired'))
      return
    }
    if (!ruleForm.value.domains.trim()) {
      push.error(i18n.global.t('ddns.domainsRequired'))
      return
    }
    if (!ruleForm.value.accountId) {
      push.error(i18n.global.t('ddns.accountRequired'))
      return
    }

    const needV4 = ruleForm.value.ipType === 'ipv4' || ruleForm.value.ipType === 'dual'
    const needV6 = ruleForm.value.ipType === 'ipv6' || ruleForm.value.ipType === 'dual'

    if (needV4) {
      if (!ipv4Sources.value || ipv4Sources.value.length === 0) {
        push.error(i18n.global.t('ddns.ipv4ModeRequired'))
        return
      }
      if (ipv4Sources.value.includes('interface') && !ruleForm.value.ipv4Interface) {
        push.error(i18n.global.t('ddns.ipv4InterfaceRequired'))
        return
      }
      ruleForm.value.ipv4Source = ipv4Sources.value.join(',')
      ruleForm.value.ipv4Url = ipv4Urls.value.map((s) => s.trim()).filter(Boolean).join(',')
    } else {
      ruleForm.value.ipv4Source = 'disabled'
    }

    if (needV6) {
      if (!ipv6Sources.value || ipv6Sources.value.length === 0) {
        push.error(i18n.global.t('ddns.ipv6ModeRequired'))
        return
      }
      if (ipv6Sources.value.includes('interface') && !ruleForm.value.ipv6Interface) {
        push.error(i18n.global.t('ddns.ipv6InterfaceRequired'))
        return
      }
      ruleForm.value.ipv6Source = ipv6Sources.value.join(',')
      ruleForm.value.ipv6Url = ipv6Urls.value.map((s) => s.trim()).filter(Boolean).join(',')
    } else {
      ruleForm.value.ipv6Source = 'disabled'
    }

    mutationBusy.value = true
    try {
      const resp = await HttpUtils.post('api/ddns-rule-save', ruleForm.value, ddnsJsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('ddns.ruleSaved'))
        ruleDialogVisible.value = false
        await fetchOverview(true)
      }
    } catch {
      // handled
    } finally {
      mutationBusy.value = false
    }
  }

  async function toggleRule(rule: DDNSRule) {
    try {
      const resp = await HttpUtils.post('api/ddns-rule-status', {
        id: rule.id,
        enabled: !rule.enabled,
      }, ddnsJsonHeaders)
      if (resp && resp.success) {
        rule.enabled = !rule.enabled
        push.success(rule.enabled ? i18n.global.t('ddns.ruleEnabled') : i18n.global.t('ddns.ruleDisabled'))
      }
    } catch {
      // handled
    }
  }

  const filteredRules = computed(() => {
    let list = overview.value.rules
    const st = searchText.value.trim().toLowerCase()
    if (st) {
      list = list.filter((r) => {
        const nameMatch = (r.name || '').toLowerCase().includes(st)
        const domainMatch = (r.domains || '').toLowerCase().includes(st)
        const accName = getAccountName(r.accountId).toLowerCase()
        const provName = getProviderName(r.accountId).toLowerCase()
        const ipMatch = (r.lastIpv4 || '').includes(st) || (r.lastIpv6 || '').includes(st)
        return nameMatch || domainMatch || accName.includes(st) || provName.includes(st) || ipMatch
      })
    }
    if (statusFilter.value !== 'all') {
      if (statusFilter.value === 'enabled') {
        list = list.filter((r) => r.enabled)
      } else if (statusFilter.value === 'disabled') {
        list = list.filter((r) => !r.enabled)
      } else if (statusFilter.value === 'success') {
        list = list.filter((r) => r.lastStatus === 'success')
      } else if (statusFilter.value === 'error') {
        list = list.filter((r) => r.lastStatus === 'error')
      } else if (statusFilter.value === 'pending') {
        list = list.filter((r) => r.lastStatus === 'pending')
      }
    }
    return list
  })

  function getIpList(ipStr?: string): string[] {
    if (!ipStr) return []
    return ipStr
      .split(/[,;\s\n\r]+/)
      .map((s) => s.trim())
      .filter(Boolean)
  }

  async function copyToClipboard(text: string, label = '内容') {
    if (!text) return
    try {
      if (navigator && navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text)
        push.success(i18n.global.t('ddns.copied', { label, text }))
      } else {
        const el = document.createElement('textarea')
        el.value = text
        document.body.appendChild(el)
        el.select()
        document.execCommand('copy')
        document.body.removeChild(el)
        push.success(i18n.global.t('ddns.copied', { label, text }))
      }
    } catch {
      push.info(i18n.global.t('ddns.copyFailed', { text }))
    }
  }

  async function removeRule(rule: DDNSRule) {
    const ok = await confirm({
      title: i18n.global.t('actions.del'),
      message: i18n.global.t('ddns.deleteRuleConfirm', { name: rule.name }),
      severity: 'danger',
      confirmText: i18n.global.t('actions.del'),
    })
    if (!ok) return

    deletingRuleId.value = rule.id
    mutationBusy.value = true
    try {
      const resp = await HttpUtils.post('api/ddns-rule-delete', { id: rule.id }, ddnsJsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('ddns.ruleDeleted', { name: rule.name }))
        await fetchOverview(true)
      }
    } catch {
      // handled by HttpUtils, rule remains in list on failure
    } finally {
      deletingRuleId.value = null
      mutationBusy.value = false
    }
  }

  async function syncRule(rule: DDNSRule) {
    syncingRuleId.value = rule.id
    push.info(i18n.global.t('ddns.syncingRule', { name: rule.name }))
    try {
      const resp = await HttpUtils.post('api/ddns-rule-sync', { id: rule.id }, ddnsJsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('ddns.syncRuleSuccess', { name: rule.name }))
        await fetchOverview(true)
      }
    } catch {
      // handled
    } finally {
      syncingRuleId.value = null
    }
  }

  async function syncAll() {
    refreshing.value = true
    try {
      const resp = await HttpUtils.post('api/ddns-rule-sync-all', {}, ddnsJsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('ddns.syncAllSuccess'))
        await fetchOverview(true)
      }
    } catch {
      // handled
    } finally {
      refreshing.value = false
    }
  }

  // Account management
  function openAccountDialog(target?: DDNSAccount) {
    if (target) {
      accountForm.value = JSON.parse(JSON.stringify(target))
      try {
        accountEnvMap.value = JSON.parse(target.envJson || '{}')
      } catch {
        accountEnvMap.value = {}
      }
    } else {
      accountForm.value = {
        id: 0,
        name: '',
        providerCode: overview.value.providers.length > 0 ? overview.value.providers[0].code : 'cloudflare',
        envJson: '{}',
        remark: '',
      }
      accountEnvMap.value = {}
    }
    accountDialogVisible.value = true
  }

  async function testAccountAuth() {
    accountForm.value.envJson = JSON.stringify(accountEnvMap.value)
    testAuthBusy.value = true
    try {
      const resp = await HttpUtils.post('api/ddns-account-test', accountForm.value, ddnsJsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('ddns.accountVerified'))
      }
    } catch {
      // handled
    } finally {
      testAuthBusy.value = false
    }
  }

  async function saveAccount() {
    if (!accountForm.value.name.trim()) {
      push.error(i18n.global.t('ddns.accountNameRequired'))
      return
    }
    accountForm.value.envJson = JSON.stringify(accountEnvMap.value)
    mutationBusy.value = true
    try {
      const resp = await HttpUtils.post('api/ddns-account-save', accountForm.value, ddnsJsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('ddns.accountSaved'))
        accountDialogVisible.value = false
        await fetchOverview(true)
      }
    } finally {
      mutationBusy.value = false
    }
  }

  async function removeAccount(account: DDNSAccount) {
    const ok = await confirm({
      title: i18n.global.t('actions.del'),
      message: i18n.global.t('ddns.deleteAccountConfirm', { name: account.name }),
      severity: 'danger',
      confirmText: i18n.global.t('actions.del'),
    })
    if (!ok) return

    mutationBusy.value = true
    try {
      const resp = await HttpUtils.post('api/ddns-account-delete', { id: account.id }, ddnsJsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('ddns.accountDeleted'))
        await fetchOverview(true)
      }
    } finally {
      mutationBusy.value = false
    }
  }

  function getAccountName(accountId: number): string {
    const acc = overview.value.accounts.find((a) => a.id === accountId)
    return acc ? acc.name : '未知账号'
  }

  function getProviderName(accountId: number): string {
    const acc = overview.value.accounts.find((a) => a.id === accountId)
    if (!acc) return '-'
    const prov = overview.value.providers.find((p) => p.code === acc.providerCode)
    return prov ? prov.name : acc.providerCode
  }

  return {
    loading,
    refreshing,
    mutationBusy,
    testAuthBusy,
    deletingRuleId,
    syncingRuleId,
    searchText,
    statusFilter,
    filteredRules,
    getIpList,
    copyToClipboard,
    overview,
    ruleDialogVisible,
    ruleForm,
    ipv4Sources,
    ipv4Urls,
    ipv6Sources,
    ipv6Urls,
    accountDialogVisible,
    accountManageVisible,
    accountForm,
    accountEnvMap,
    selectedProvider,
    fetchOverview,
    openRuleDialog,
    saveRule,
    toggleRule,
    removeRule,
    syncRule,
    syncAll,
    openAccountDialog,
    testAccountAuth,
    saveAccount,
    removeAccount,
    getAccountName,
    getProviderName,
  }
}
