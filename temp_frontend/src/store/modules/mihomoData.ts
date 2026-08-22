import HttpUtils from '@/plugins/httputil'
import { defineStore } from 'pinia'
import { push } from 'notivue'
import { i18n } from '@/locales'
import { Inbound } from '@/types/inbounds'
import { Client } from '@/types/clients'
import Data from '@/store/modules/data'
import { normalizeSubscriptionURI, refreshSubscriptionURI } from '@/plugins/subscriptionUri'

const reloadItemsKey = 'mihomo_reloadItems'
let mihomoDataLoadPromise: Promise<boolean> | null = null

export interface MihomoPatchSaveResult {
  saved: boolean
  runtimeRefreshFailed: boolean
  changed: boolean
  conflict: boolean
  currentRevision?: number
  revision?: number
}

const MihomoData = defineStore('MihomoData', {
  state: () => ({
    isLoadingData: false,
    hasFullData: false,
    lastLoad: 0,
    reloadItems: localStorage.getItem(reloadItemsKey)?.split(',') ?? <string[]>[],
    subURI: '',
    subscriptionUriVerified: false,
    subscriptionUriRefreshing: false,
    subscriptionUriError: '',
    enableTraffic: false,
    onlines: { inbound: <string[]>[], outbound: <string[]>[], user: <string[]>[] },
    config: <any>{},
    inbounds: <any[]>[],
    outbounds: <any[]>[],
    outboundgroups: <any[]>[],
    suboutbounds: <any[]>[],
    subgroups: <any[]>[],
    services: <any[]>[],
    endpoints: <any[]>[],
    clients: <any[]>[],
    tlsConfigs: <any[]>[],
  }),
  actions: {
    loadData(force = false): Promise<boolean> {
      if (mihomoDataLoadPromise) return mihomoDataLoadPromise
      this.isLoadingData = true
      mihomoDataLoadPromise = (async () => {
        let loaded = false
        try {
          const params = !force && this.hasFullData && this.lastLoad > 0
            ? { lu: this.lastLoad, light: 'true' }
            : {}
          const msg = await HttpUtils.get('api/mihomo-load', params)
          if (msg.success) {
            const data = msg.obj ?? {}
            const hasFullSnapshot = Object.hasOwn(data, 'config')
              && Object.hasOwn(data, 'clients')
              && Object.hasOwn(data, 'tls')
              && Object.hasOwn(data, 'inbounds')
              && Object.hasOwn(data, 'outbounds')
              && Object.hasOwn(data, 'outboundgroups')
              && Object.hasOwn(data, 'suboutbounds')
              && Object.hasOwn(data, 'subgroups')
              && Object.hasOwn(data, 'subURI')
            if (hasFullSnapshot) this.hasFullData = true
            this.onlines = data.onlines ?? this.onlines
            if (Object.hasOwn(data, 'enableTraffic')) this.enableTraffic = data.enableTraffic === true
            if (Object.hasOwn(data, 'config')) {
              this.setNewData(data)
            }
            loaded = true
          }
        } finally {
          this.isLoadingData = false
          mihomoDataLoadPromise = null
        }
        return loaded
      })()
      return mihomoDataLoadPromise
    },
    async loadConfig(): Promise<any | null> {
      this.isLoadingData = true
      try {
        const msg = await HttpUtils.get('api/mihomo-config')
        if (!msg.success || !msg.obj || !Object.hasOwn(msg.obj, 'config')) return null
        const revision = Number(msg.obj.lastUpdate)
        if (Number.isSafeInteger(revision) && revision >= 0) this.lastLoad = revision
        this.config = msg.obj.config ?? {}
        return this.config
      } finally {
        this.isLoadingData = false
      }
    },
    async saveDnsConfig(data: { expectedRevision?: number, ipv6: boolean | null, tcpConcurrent: boolean | null, dns: Record<string, unknown> | null, retryRuntime?: boolean }): Promise<MihomoPatchSaveResult> {
      const msg = await HttpUtils.post('api/mihomo-dns-save', data, {
        headers: { 'Content-Type': 'application/json' },
      })
      if (msg.obj && Object.hasOwn(msg.obj, 'config')) {
        this.config = msg.obj.config ?? {}
      }
      const revision = Number(msg.obj?.revision)
      if (Number.isSafeInteger(revision) && revision >= 0) this.lastLoad = revision
      const committed = msg.obj?.committed === true
      return {
        saved: msg.success === true || committed,
        runtimeRefreshFailed: msg.obj?.retryRuntime === true || (committed && msg.success !== true),
        changed: msg.obj?.changed === true,
        conflict: msg.obj?.code === 'revision_conflict',
        currentRevision: Number.isSafeInteger(Number(msg.obj?.currentRevision)) ? Number(msg.obj.currentRevision) : undefined,
        revision: Number.isSafeInteger(revision) ? revision : undefined,
      }
    },
    async saveRouteConfig(data: { expectedRevision?: number, route: Record<string, unknown>, sniffer?: unknown, retryRuntime?: boolean }): Promise<MihomoPatchSaveResult> {
      const msg = await HttpUtils.post('api/mihomo-route-save', data, {
        headers: { 'Content-Type': 'application/json' },
      })
      if (msg.obj && Object.hasOwn(msg.obj, 'config')) {
        this.config = msg.obj.config ?? {}
      }
      const revision = Number(msg.obj?.revision)
      if (Number.isSafeInteger(revision) && revision >= 0) this.lastLoad = revision
      const committed = msg.obj?.committed === true
      if (msg.success || committed) {
        if (!msg.obj || !Object.hasOwn(msg.obj, 'config')) {
          await this.loadConfig()
        }
      }
      return {
        saved: msg.success === true || committed,
        runtimeRefreshFailed: msg.obj?.retryRuntime === true || (committed && msg.success !== true),
        changed: msg.obj?.changed === true,
        conflict: msg.obj?.code === 'revision_conflict',
        currentRevision: Number.isSafeInteger(Number(msg.obj?.currentRevision)) ? Number(msg.obj.currentRevision) : undefined,
        revision: Number.isSafeInteger(revision) ? revision : undefined,
      }
    },
    setNewData(data: any) {
      if (!data || typeof data !== 'object' || Array.isArray(data)) return
      const serverRevision = Number(data?.lastUpdate)
      if (Number.isSafeInteger(serverRevision) && serverRevision > 0) {
        this.lastLoad = serverRevision
      }
      if (Object.hasOwn(data, 'subURI')) {
        const subURI = normalizeSubscriptionURI(data.subURI)
        if (subURI) {
          this.subURI = subURI
          this.subscriptionUriVerified = true
          this.subscriptionUriError = ''
        }
      }
      if (Object.hasOwn(data, 'enableTraffic')) this.enableTraffic = data.enableTraffic === true
      if (Object.hasOwn(data, 'config') && data.config && typeof data.config === 'object' && !Array.isArray(data.config)) this.config = data.config
      if (Object.hasOwn(data, 'clients')) this.clients = Array.isArray(data.clients) ? data.clients : data.clients === null ? [] : this.clients
      if (Object.hasOwn(data, 'inbounds')) this.inbounds = Array.isArray(data.inbounds) ? data.inbounds : data.inbounds === null ? [] : this.inbounds
      if (Object.hasOwn(data, 'outbounds')) this.outbounds = Array.isArray(data.outbounds) ? data.outbounds : data.outbounds === null ? [] : this.outbounds
      if (Object.hasOwn(data, 'outboundgroups')) this.outboundgroups = Array.isArray(data.outboundgroups) ? data.outboundgroups : data.outboundgroups === null ? [] : this.outboundgroups
      if (Object.hasOwn(data, 'suboutbounds')) this.suboutbounds = Array.isArray(data.suboutbounds) ? data.suboutbounds : data.suboutbounds === null ? [] : this.suboutbounds
      if (Object.hasOwn(data, 'subgroups')) this.subgroups = Array.isArray(data.subgroups) ? data.subgroups : data.subgroups === null ? [] : this.subgroups
      if (Object.hasOwn(data, 'services')) this.services = Array.isArray(data.services) ? data.services : data.services === null ? [] : this.services
      if (Object.hasOwn(data, 'endpoints')) this.endpoints = Array.isArray(data.endpoints) ? data.endpoints : data.endpoints === null ? [] : this.endpoints
      if (Object.hasOwn(data, 'tls')) this.tlsConfigs = Array.isArray(data.tls) ? data.tls : data.tls === null ? [] : this.tlsConfigs

      const defaultData = Data()
      if (Object.hasOwn(data, 'suboutbounds')) defaultData.suboutbounds = Array.isArray(data.suboutbounds) ? data.suboutbounds : data.suboutbounds === null ? [] : defaultData.suboutbounds
      if (Object.hasOwn(data, 'subgroups')) defaultData.subgroups = Array.isArray(data.subgroups) ? data.subgroups : data.subgroups === null ? [] : defaultData.subgroups
    },
    async refreshSubscriptionURI(): Promise<boolean> {
      this.subscriptionUriRefreshing = true
      this.subscriptionUriError = ''
      try {
        const result = await refreshSubscriptionURI()
        if (!result.success) {
          this.subscriptionUriError = result.error
          return false
        }
        this.subURI = result.subURI
        this.subscriptionUriVerified = true
        this.subscriptionUriError = ''
        return true
      } finally {
        this.subscriptionUriRefreshing = false
      }
    },
    async loadInbounds(ids: number[], requestOptions: { signal?: AbortSignal; silentErrorToast?: boolean } = {}): Promise<Inbound[] | null> {
      const params = ids.length > 0 ? { id: ids.join(',') } : {}
      const msg = await HttpUtils.get('api/mihomo-inbounds', params, requestOptions)
      if (msg.success && Object.hasOwn(msg.obj ?? {}, 'inbounds')) {
        return Array.isArray(msg.obj?.inbounds) ? msg.obj.inbounds : []
      }
      return null
    },
    async loadClients(id: number): Promise<Client> {
      const options = id > 0 ? { id: id } : {}
      const msg = await HttpUtils.get('api/mihomo-clients', options)
      if (msg.success && Object.hasOwn(msg.obj ?? {}, 'clients')) {
        return <Client>(Array.isArray(msg.obj?.clients) ? msg.obj.clients[0] : undefined) ?? {}
      }
      return <Client>{}
    },
    async loadSubGroups(): Promise<any[] | null> {
      const msg = await HttpUtils.get('api/subgroups')
      if (msg.success && Object.hasOwn(msg.obj ?? {}, 'subgroups')) {
        this.subgroups = Array.isArray(msg.obj?.subgroups) ? msg.obj.subgroups : msg.obj?.subgroups === null ? [] : this.subgroups
        const defaultData = Data()
        defaultData.subgroups = this.subgroups
        return this.subgroups
      }
      return null
    },
    async loadOutboundGroups(): Promise<any[] | null> {
      const msg = await HttpUtils.get('api/mihomo-outboundgroups')
      if (msg.success && Object.hasOwn(msg.obj ?? {}, 'outboundgroups')) {
        this.outboundgroups = Array.isArray(msg.obj?.outboundgroups) ? msg.obj.outboundgroups : msg.obj?.outboundgroups === null ? [] : this.outboundgroups
        return this.outboundgroups
      }
      return null
    },
    async save(object: string, action: string, data: any, initUsers?: number[]): Promise<boolean> {
      const objectMap: Record<string, string> = {
        config: 'mihomo_config',
        clients: 'mihomo_clients',
        inbounds: 'mihomo_inbounds',
        outbounds: 'mihomo_outbounds',
        outboundgroups: 'mihomo_outboundgroups',
        mihomo_outboundgroups: 'mihomo_outboundgroups',
        tls: 'mihomo_tls',
      }
      const postData = {
        object: objectMap[object] ?? object,
        action: action,
        data: JSON.stringify(data, null, 2),
        initUsers: initUsers?.join(',') ?? undefined,
      }
      const msg = await HttpUtils.post('api/save', postData)
      if (msg.success) {
        const objectNameMap: Record<string, string> = {
          outboundgroups: 'group',
          mihomo_outboundgroups: 'group',
        }
        const objectName = objectNameMap[object] ?? (['tls', 'config'].includes(object) ? object : object.substring(0, object.length - 1))
        push.success({
          title: i18n.global.t('success'),
          duration: 5000,
          message: i18n.global.t('actions.' + action) + ' ' + i18n.global.t('objects.' + objectName),
        })
        this.setNewData(msg.obj)
      } else if (msg.obj?.committed === true) {
        let reloaded = false
        try {
          if (object === 'config') {
            reloaded = await this.loadConfig() !== null
          } else {
            reloaded = await this.loadData(true)
          }
        } catch {
          reloaded = false
        }
        if (msg.obj?.refreshFailed === true) {
          push.warning({
            title: i18n.global.t('warning'),
            duration: 7000,
            message: reloaded ? '数据已保存，列表已重新加载。' : '数据已保存，但列表刷新失败，请稍后刷新页面。',
          })
        }
        return true
      }
      return msg.success
    },
    checkClientName(id: number, newName: string): boolean {
      const oldName = id > 0 ? this.clients.findLast((i: any) => i.id == id)?.name : null
      if (newName != oldName && this.clients.findIndex((c: any) => c.name == newName) != -1) {
        push.error({
          message: i18n.global.t('error.dplData') + ': ' + i18n.global.t('client.name'),
        })
        return true
      }
      return false
    },
    checkBulkClientNames(names: string[]): boolean {
      const newNames = new Set(names)
      const oldNames = new Set(this.clients.map((c: any) => c.name))
      const allNames = new Set([...oldNames, ...newNames])
      if (newNames.size != names.length || oldNames.size + newNames.size != allNames.size) {
        push.error({
          message: i18n.global.t('error.dplData') + ': ' + i18n.global.t('client.name'),
        })
        return true
      }
      return false
    },
    checkTag(object: string, id: number, tag: string): boolean {
      let objects = <any[]>[]
      switch (object) {
        case 'inbound':
          objects = this.inbounds
          break
        case 'outbound':
          objects = this.outbounds
          break
        case 'suboutbound':
          objects = this.suboutbounds
          break
        case 'service':
          objects = this.services
          break
        case 'endpoint':
          objects = this.endpoints
          break
        default:
          return false
      }
      const oldObject = id > 0 ? objects.findLast((i: any) => i.id == id) : null
      if (tag != oldObject?.tag && objects.findIndex((i: any) => i.tag == tag) != -1) {
        push.error({
          message: i18n.global.t('error.dplData') + ': ' + i18n.global.t('objects.tag'),
        })
        return true
      }
      return false
    },
  },
})

export default MihomoData
