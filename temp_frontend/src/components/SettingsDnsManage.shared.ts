import { ref, computed, watch, type Ref } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { push } from 'notivue'
import { confirm } from '@/plugins/confirm'
import { i18n } from '@/locales'
import type {
  DNSRecord,
  DNSDomainFavorite,
  DNSOverview,
  RecordForm,
  DNSAccount
} from '@/types/dnsManage'
import type { ProviderMeta } from '@/types/ddns'

const jsonHeaders = { headers: { 'Content-Type': 'application/json' } }

export interface DomainMeta {
  domain: string
  isFavorite: boolean
  isDefaultFavorite: boolean
  isCloud: boolean
}

export function useDnsManage(active: Ref<boolean>) {
  const loading = ref(false)
  const recordsLoading = ref(false)
  const mutationBusy = ref(false)
  const testAuthBusy = ref(false)
  const deletingRecordId = ref<string | null>(null)

  const overview = ref<DNSOverview>({
    accounts: [],
    providers: [],
    favorites: [],
  })

  // Selected query scope
  const selectedAccountId = ref<number | null>(null)
  const selectedDomain = ref('')
  const domainsLoading = ref(false)
  const accountDomains = ref<string[]>([])
  const records = ref<DNSRecord[]>([])

  // Filter and search
  const searchText = ref('')
  const typeFilter = ref<string>('all')

  // Record Dialog State
  const recordDialogVisible = ref(false)
  const isEditingRecord = ref(false)
  const recordForm = ref<RecordForm>({
    id: '',
    name: '@',
    type: 'A',
    value: '',
    ttl: 600,
    priority: 10,
    proxied: false,
  })

  // Account Dialog State
  const accountManageVisible = ref(false)
  const accountDialogVisible = ref(false)
  const accountForm = ref<DNSAccount>({
    id: 0,
    name: '',
    providerCode: 'cloudflare',
    envJson: '{}',
    remark: '',
  })
  const accountEnvMap = ref<Record<string, string>>({})

  // Providers map
  const providersMap = computed<Record<string, ProviderMeta>>(() => {
    const map: Record<string, ProviderMeta> = {}
    overview.value.providers.forEach((p) => {
      map[p.code] = p
    })
    return map
  })

  // All DNS accounts (fully supported)
  const generalAccounts = computed<DNSAccount[]>(() => {
    return overview.value.accounts
  })

  const currentAccount = computed<DNSAccount | undefined>(() => {
    return overview.value.accounts.find((a) => a.id === selectedAccountId.value)
  })

  const currentProvider = computed<ProviderMeta | undefined>(() => {
    if (!currentAccount.value) return undefined
    return providersMap.value[currentAccount.value.providerCode]
  })

  const isCloudflare = computed(() => {
    return currentAccount.value?.providerCode === 'dns_cf' || currentAccount.value?.providerCode === 'cloudflare'
  })

  const selectedAccountProvider = computed<ProviderMeta | undefined>(() => {
    return overview.value.providers.find((p) => p.code === accountForm.value.providerCode)
  })

  // Filtered records
  const currentAccountFavorites = computed(() => {
    if (!selectedAccountId.value) return []
    return overview.value.favorites.filter((f) => f.accountId === selectedAccountId.value)
  })

  const domainMetaMap = computed<Record<string, DomainMeta>>(() => {
    const map: Record<string, DomainMeta> = {}

    accountDomains.value.forEach((d) => {
      const dm = cleanDomain(d)
      if (dm) {
        map[dm] = {
          domain: dm,
          isFavorite: false,
          isDefaultFavorite: false,
          isCloud: true,
        }
      }
    })

    currentAccountFavorites.value.forEach((fav) => {
      const dm = cleanDomain(fav.domain)
      if (dm) {
        const existing = Object.keys(map).find((k) => k.toLowerCase() === dm.toLowerCase())
        const key = existing || dm
        map[key] = {
          domain: key,
          isFavorite: true,
          isDefaultFavorite: !!fav.isDefault,
          isCloud: !!existing,
        }
      }
    })

    return map
  })

  const availableDomainStrings = computed<string[]>(() => {
    const list = Object.values(domainMetaMap.value)
    list.sort((a, b) => {
      if (a.isDefaultFavorite && !b.isDefaultFavorite) return -1
      if (!a.isDefaultFavorite && b.isDefaultFavorite) return 1
      if (a.isFavorite && !b.isFavorite) return -1
      if (!a.isFavorite && b.isFavorite) return 1
      return a.domain.localeCompare(b.domain)
    })
    return list.map((item) => item.domain)
  })

  const filteredRecords = computed(() => {
    let list = records.value
    if (typeFilter.value !== 'all') {
      list = list.filter((r) => r.type.toUpperCase() === typeFilter.value.toUpperCase())
    }
    const q = searchText.value.trim().toLowerCase()
    if (q) {
      list = list.filter((r) => {
        return (
          r.name.toLowerCase().includes(q) ||
          r.type.toLowerCase().includes(q) ||
          r.value.toLowerCase().includes(q)
        )
      })
    }
    return list
  })

  async function fetchOverview(silent = false) {
    if (!silent) {
      loading.value = true
    }
    try {
      const resp = await HttpUtils.get('api/dns-overview', {}, { silentErrorToast: silent })
      if (resp && resp.success && resp.obj) {
        overview.value = resp.obj as DNSOverview

        // Default selection if not chosen yet or previous selection no longer exists
        const hasSelected = generalAccounts.value.some((a) => a.id === selectedAccountId.value)
        if ((!selectedAccountId.value || !hasSelected) && generalAccounts.value.length > 0) {
          selectedAccountId.value = generalAccounts.value[0].id
          selectedDomain.value = ''
          records.value = []
        }

        if (selectedAccountId.value) {
          await fetchAccountDomains(selectedAccountId.value)
          if (selectedDomain.value && !silent) {
            await fetchRecords(true)
          }
        }
      }
    } catch {
      // Handled
    } finally {
      if (!silent) {
        loading.value = false
      }
    }
  }

  function cleanDomain(input: string): string {
    let d = (input || '').trim()
    d = d.replace(/^https?:\/\//i, '')
    d = d.replace(/\/.*$/, '')
    d = d.replace(/\.+$/, '')
    return d
  }

  const currentFavoriteDomain = computed<DNSDomainFavorite | undefined>(() => {
    const d = cleanDomain(selectedDomain.value)
    if (!d) return undefined
    return overview.value.favorites.find((f) => f.domain.toLowerCase() === d.toLowerCase())
  })

  const isCurrentDomainFavorite = computed(() => {
    return !!currentFavoriteDomain.value
  })

  async function fetchRecords(silent = false) {
    const d = cleanDomain(selectedDomain.value)
    if (!selectedAccountId.value) {
      if (!silent) {
        push.warning(i18n.global.t('dnsManage.selectAccountRequired', '请先选择 DNS 账号'))
      }
      return
    }
    if (!d) {
      if (!silent) {
        push.warning(i18n.global.t('dnsManage.domainRequired', '请输入托管根域名 (如 example.com)'))
      }
      return
    }
    if (!silent) {
      recordsLoading.value = true
    }
    try {
      const resp = await HttpUtils.get(
        'api/dns-records',
        {
          accountId: selectedAccountId.value,
          domain: d,
        },
        { silentErrorToast: silent }
      )
      if (resp && resp.success && resp.obj) {
        records.value = (resp.obj as DNSRecord[]) || []
      }
    } catch {
      records.value = []
    } finally {
      if (!silent) {
        recordsLoading.value = false
      }
    }
  }

  function onDomainChanged() {
    const d = cleanDomain(selectedDomain.value)
    if (d && selectedAccountId.value) {
      fetchRecords()
    } else {
      records.value = []
    }
  }

  let domainFetchSeq = 0
  const domainFetchError = ref('')
  async function fetchAccountDomains(accountId: number) {
    if (!accountId) {
      accountDomains.value = []
      domainFetchError.value = ''
      return
    }

    const currentSeq = ++domainFetchSeq
    domainsLoading.value = true
    domainFetchError.value = ''
    try {
      const resp = await HttpUtils.get('api/dns-domains', { accountId }, { silentErrorToast: true })
      if (currentSeq !== domainFetchSeq) return

      if (resp && resp.success && Array.isArray(resp.obj)) {
        accountDomains.value = (resp.obj as string[]).filter((d) => !!d && typeof d === 'string')
      } else {
        accountDomains.value = []
        if (resp && !resp.success && resp.msg) {
          domainFetchError.value = resp.msg
        }
      }
    } catch (e: any) {
      if (currentSeq === domainFetchSeq) {
        accountDomains.value = []
        domainFetchError.value = e?.message || ''
      }
    } finally {
      if (currentSeq === domainFetchSeq) {
        domainsLoading.value = false
      }
    }
  }

  async function onAccountChanged(newAccountId: number) {
    if (!newAccountId) {
      selectedAccountId.value = null
      accountDomains.value = []
      selectedDomain.value = ''
      records.value = []
      domainFetchError.value = ''
      return
    }
    selectedAccountId.value = newAccountId
    selectedDomain.value = ''
    records.value = []
    domainFetchError.value = ''
    await fetchAccountDomains(newAccountId)
  }

  async function saveAsFavoriteDomain() {
    const d = cleanDomain(selectedDomain.value)
    if (!d) return
    if (!selectedAccountId.value) return

    try {
      const resp = await HttpUtils.post(
        'api/dns-domain-save',
        {
          accountId: selectedAccountId.value,
          domain: d,
          remark: '',
          isDefault: true,
        },
        jsonHeaders
      )
      if (resp && resp.success) {
        push.success(i18n.global.t('dnsManage.domainSaved', '已保存常用域名'))
        await fetchOverview(true)
      }
    } catch {
      // handled
    }
  }

  async function toggleFavoriteDomain() {
    const d = cleanDomain(selectedDomain.value)
    if (!d || !selectedAccountId.value) return

    if (isCurrentDomainFavorite.value && currentFavoriteDomain.value) {
      const fav = currentFavoriteDomain.value
      try {
        const resp = await HttpUtils.post('api/dns-domain-delete', { id: fav.id }, jsonHeaders)
        if (resp && resp.success) {
          push.success(i18n.global.t('dnsManage.domainUnfavorited', '已取消常用域名'))
          await fetchOverview(true)
        }
      } catch {
        // handled
      }
    } else {
      await saveAsFavoriteDomain()
    }
  }

  async function removeFavoriteDomain(fav: DNSDomainFavorite) {
    try {
      const resp = await HttpUtils.post('api/dns-domain-delete', { id: fav.id }, jsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('dnsManage.domainDeleted', '已删除常用域名'))
        await fetchOverview(true)
      }
    } catch {
      // handled
    }
  }

  function openRecordDialog(target?: DNSRecord) {
    if (target) {
      isEditingRecord.value = true
      recordForm.value = {
        id: target.id,
        name: target.name,
        type: target.type,
        value: target.value,
        ttl: target.ttl > 0 ? target.ttl : 600,
        priority: target.priority || 10,
        proxied: !!target.proxied,
      }
    } else {
      isEditingRecord.value = false
      recordForm.value = {
        id: '',
        name: '@',
        type: 'A',
        value: '',
        ttl: 600,
        priority: 10,
        proxied: false,
      }
    }
    recordDialogVisible.value = true
  }

  async function saveRecord() {
    const d = cleanDomain(selectedDomain.value)
    if (!selectedAccountId.value || !d) {
      push.error(i18n.global.t('dnsManage.selectAccountAndDomain', '请先选择 DNS 账号并输入域名'))
      return
    }
    const val = recordForm.value.value.trim()
    if (!val) {
      push.error(i18n.global.t('dnsManage.recordValueRequired', '解析记录值不能为空'))
      return
    }

    let hostName = recordForm.value.name.trim()
    if (!hostName || hostName === '@') {
      hostName = '@'
    }

    mutationBusy.value = true
    try {
      const payload = {
        accountId: selectedAccountId.value,
        domain: d,
        record: {
          id: recordForm.value.id,
          name: hostName,
          type: recordForm.value.type.toUpperCase(),
          value: val,
          ttl: Number(recordForm.value.ttl) || 600,
          priority: Number(recordForm.value.priority) || 10,
          proxied: isCloudflare.value ? recordForm.value.proxied : false,
        },
      }

      const endpoint = isEditingRecord.value ? 'api/dns-record-update' : 'api/dns-record-create'
      const resp = await HttpUtils.post(endpoint, payload, jsonHeaders)
      if (resp && resp.success) {
        push.success(
          isEditingRecord.value
            ? i18n.global.t('dnsManage.recordUpdated', '解析记录已更新')
            : i18n.global.t('dnsManage.recordCreated', '解析记录已创建')
        )
        recordDialogVisible.value = false
        await fetchRecords()
      }
    } finally {
      mutationBusy.value = false
    }
  }

  async function removeRecord(record: DNSRecord) {
    const ok = await confirm({
      title: i18n.global.t('actions.del', '删除'),
      message: i18n.global.t('dnsManage.deleteRecordConfirm', {
        type: record.type,
        name: record.name,
      }) || `确认删除 ${record.type} 记录 "${record.name}" 吗？此操作将立即在云端生效。`,
      severity: 'danger',
      confirmText: i18n.global.t('actions.del', '删除'),
    })
    if (!ok) return

    deletingRecordId.value = record.id
    mutationBusy.value = true
    try {
      const payload = {
        accountId: selectedAccountId.value,
        domain: selectedDomain.value.trim(),
        recordId: record.id,
      }
      const resp = await HttpUtils.post('api/dns-record-delete', payload, jsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('dnsManage.recordDeleted', '解析记录已从云端删除'))
        await fetchRecords()
      }
    } finally {
      deletingRecordId.value = null
      mutationBusy.value = false
    }
  }

  async function toggleRecordProxy(record: DNSRecord) {
    if (!isCloudflare.value || (record.type !== 'A' && record.type !== 'AAAA' && record.type !== 'CNAME')) {
      return
    }

    mutationBusy.value = true
    try {
      const payload = {
        accountId: selectedAccountId.value,
        domain: selectedDomain.value.trim(),
        record: {
          id: record.id,
          name: record.name,
          type: record.type,
          value: record.value,
          ttl: record.ttl,
          priority: record.priority || 0,
          proxied: !record.proxied,
        },
      }
      const resp = await HttpUtils.post('api/dns-record-update', payload, jsonHeaders)
      if (resp && resp.success) {
        record.proxied = !record.proxied
        push.success(
          record.proxied
            ? i18n.global.t('dnsManage.proxyEnabled', '已开启 Cloudflare CDN 代理')
            : i18n.global.t('dnsManage.proxyDisabled', '已关闭 Cloudflare CDN 代理')
        )
      }
    } finally {
      mutationBusy.value = false
    }
  }

  async function copyToClipboard(text: string, label = '内容') {
    if (!text) return
    try {
      if (navigator && navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(text)
        push.success(i18n.global.t('ddns.copied', { label, text }) || `已复制: ${text}`)
      } else {
        const el = document.createElement('textarea')
        el.value = text
        document.body.appendChild(el)
        el.select()
        document.execCommand('copy')
        document.body.removeChild(el)
        push.success(i18n.global.t('ddns.copied', { label, text }) || `已复制: ${text}`)
      }
    } catch {
      push.info(i18n.global.t('ddns.copyFailed', { text }) || `复制失败: ${text}`)
    }
  }

  // Account Management Modal (Dedicated for DNS Management)
  function openAccountDialog(target?: DNSAccount) {
    if (target) {
      accountForm.value = JSON.parse(JSON.stringify(target))
      try {
        accountEnvMap.value = JSON.parse(target.envJson || '{}')
      } catch {
        accountEnvMap.value = {}
      }
    } else {
      const firstGeneral = overview.value.providers.find((p) => p.supportGeneralDns)
      accountForm.value = {
        id: 0,
        name: '',
        providerCode: firstGeneral ? firstGeneral.code : 'cloudflare',
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
      const resp = await HttpUtils.post('api/dns-account-test', accountForm.value, jsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('ddns.accountVerified', 'DNS 账号凭证验证通过'))
      }
    } finally {
      testAuthBusy.value = false
    }
  }

  async function saveAccount() {
    if (!accountForm.value.name.trim()) {
      push.error(i18n.global.t('ddns.accountNameRequired', '请输入账号名称'))
      return
    }
    accountForm.value.envJson = JSON.stringify(accountEnvMap.value)
    mutationBusy.value = true
    try {
      const resp = await HttpUtils.post('api/dns-account-save', accountForm.value, jsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('dnsManage.accountSaved', 'DNS 解析账号保存成功'))
        accountDialogVisible.value = false
        await fetchOverview(true)
      }
    } finally {
      mutationBusy.value = false
    }
  }

  async function removeAccount(account: DNSAccount) {
    const ok = await confirm({
      title: i18n.global.t('actions.del', '删除'),
      message: i18n.global.t('dnsManage.deleteAccountConfirm', { name: account.name }) || `确认删除 DNS 解析账号 "${account.name}" 吗？`,
      severity: 'danger',
      confirmText: i18n.global.t('actions.del', '删除'),
    })
    if (!ok) return

    mutationBusy.value = true
    try {
      const resp = await HttpUtils.post('api/dns-account-delete', { id: account.id }, jsonHeaders)
      if (resp && resp.success) {
        push.success(i18n.global.t('dnsManage.accountDeleted', 'DNS 解析账号已删除'))
        if (selectedAccountId.value === account.id) {
          selectedAccountId.value = null
          records.value = []
        }
        await fetchOverview(true)
      }
    } finally {
      mutationBusy.value = false
    }
  }

  watch(
    active,
    (val) => {
      if (val) {
        fetchOverview().then(() => {
          if (selectedAccountId.value && selectedDomain.value) {
            fetchRecords()
          }
        })
      }
    },
    { immediate: true }
  )

  return {
    loading,
    recordsLoading,
    mutationBusy,
    testAuthBusy,
    deletingRecordId,
    overview,
    selectedAccountId,
    selectedDomain,
    domainsLoading,
    domainFetchError,
    accountDomains,
    availableDomainStrings,
    domainMetaMap,
    records,
    filteredRecords,
    searchText,
    typeFilter,
    recordDialogVisible,
    isEditingRecord,
    recordForm,
    accountManageVisible,
    accountDialogVisible,
    accountForm,
    accountEnvMap,
    generalAccounts,
    providersMap,
    currentAccount,
    currentProvider,
    isCloudflare,
    selectedAccountProvider,
    fetchOverview,
    fetchRecords,
    fetchAccountDomains,
    onAccountChanged,
    onDomainChanged,
    cleanDomain,
    currentFavoriteDomain,
    isCurrentDomainFavorite,
    toggleFavoriteDomain,
    saveAsFavoriteDomain,
    removeFavoriteDomain,
    openRecordDialog,
    saveRecord,
    removeRecord,
    toggleRecordProxy,
    copyToClipboard,
    openAccountDialog,
    testAccountAuth,
    saveAccount,
    removeAccount,
  }
}
