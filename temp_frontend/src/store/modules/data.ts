import HttpUtils from '@/plugins/httputil'
import { defineStore } from 'pinia'
import { push } from 'notivue'
import { i18n } from '@/locales'
import { Inbound } from '@/types/inbounds'
import { Client } from '@/types/clients'

const Data = defineStore('Data', {
  state: () => ({ 
    lastLoad: 0,
    reloadItems: localStorage.getItem("reloadItems")?.split(',')?? <string[]>[],
    subURI: "",
    enableTraffic: false,
    onlines: {inbound: <string[]>[], outbound: <string[]>[], user: <string[]>[]},
    config: <any>{},
    inbounds: <any[]>[],
    outbounds: <any[]>[],
    outboundgroups: <any[]>[],
    suboutbounds: <any[]>[],
    subgroups: <any[]>[],
    services: <any[]>[],
    endpoints: <any[]>[],
    clients: <any>[],
    tlsConfigs: <any[]>[],
  }),
  actions: {
    async loadData() {
      const params = this.lastLoad > 0
        ? { lu: this.lastLoad, light: 'true' }
        : {}
      const msg = await HttpUtils.get('api/load', params)
      if(msg.success) {
        this.onlines = msg.obj.onlines
        if (msg.obj.config) {
          this.setNewData(msg.obj)
        }
      }
    },
    setNewData(data: any) {
      this.lastLoad = Math.floor((new Date()).getTime()/1000)
      if (Object.hasOwn(data, 'subURI')) this.subURI = data.subURI ?? ""
      if (Object.hasOwn(data, 'enableTraffic')) this.enableTraffic = data.enableTraffic === true
      if (Object.hasOwn(data, 'config')) this.config = data.config ?? {}
      if (Object.hasOwn(data, 'clients')) this.clients = data.clients ?? []
      if (Object.hasOwn(data, 'inbounds')) this.inbounds = data.inbounds ?? []
      if (Object.hasOwn(data, 'outbounds')) this.outbounds = data.outbounds ?? []
      if (Object.hasOwn(data, 'outboundgroups')) this.outboundgroups = data.outboundgroups ?? []
      if (Object.hasOwn(data, 'suboutbounds')) this.suboutbounds = data.suboutbounds ?? []
      if (Object.hasOwn(data, 'subgroups')) this.subgroups = data.subgroups ?? []
      if (Object.hasOwn(data, 'services')) this.services = data.services ?? []
      if (Object.hasOwn(data, 'endpoints')) this.endpoints = data.endpoints ?? []
      if (Object.hasOwn(data, 'tls')) this.tlsConfigs = data.tls ?? []
    },
    async loadInbounds(ids: number[]): Promise<Inbound[]> {
      const options = ids.length > 0 ? {id: ids.join(",")} : {}
      const msg = await HttpUtils.get('api/inbounds', options)
      if(msg.success) {
        return msg.obj.inbounds
      }
      return <Inbound[]>[]
    },
    async loadClients(id: number): Promise<Client> {
      const options = id > 0 ? {id: id} : {}
      const msg = await HttpUtils.get('api/clients', options)
      if(msg.success) {
        return <Client>msg.obj.clients[0]??{}
      }
      return <Client>{}
    },
    async loadSubGroups(): Promise<any[]> {
      const msg = await HttpUtils.get('api/subgroups')
      if(msg.success) {
        this.subgroups = msg.obj.subgroups ?? []
        return this.subgroups
      }
      return []
    },
    async loadOutboundGroups(): Promise<any[]> {
      const msg = await HttpUtils.get('api/outboundgroups')
      if(msg.success) {
        return msg.obj.outboundgroups ?? []
      }
      return []
    },
    async save (object: string, action: string, data: any, initUsers?: number[]): Promise<boolean> {
      let postData = {
        object: object,
        action: action,
        data: JSON.stringify(data, null, 2),
        initUsers: initUsers?.join(',') ?? undefined
      }
      const msg = await HttpUtils.post('api/save', postData)
      if (msg.success) {
        const objectNameMap: Record<string, string> = {
          outboundgroups: 'group',
        }
        const objectName = objectNameMap[object] ?? (['tls', 'config'].includes(object) ? object : object.substring(0, object.length - 1))
        push.success({
          title: i18n.global.t('success'),
          duration: 5000,
          message: i18n.global.t('actions.' + action) + " " + i18n.global.t('objects.' + objectName)
        })
        this.setNewData(msg.obj)
      }
      return msg.success
    },
    // Check duplicate client name
    checkClientName (id: number, newName: string): boolean {
      const oldName = id > 0 ? this.clients.findLast((i: any) => i.id == id)?.name : null
      if (newName != oldName && this.clients.findIndex((c: any) => c.name == newName) != -1) {
        push.error({
          message: i18n.global.t('error.dplData') + ": " + i18n.global.t('client.name')
        })
        return true
      }
      return false
    },
    // Check bulk client names
    checkBulkClientNames (names: string[]): boolean {
      const newNames = new Set(names)
      const oldNames = new Set(this.clients.map((c: any) => c.name))
      const allNames = new Set([...oldNames, ...newNames])
      console.log(oldNames, newNames, allNames)
      if (newNames.size != names.length || oldNames.size + newNames.size != allNames.size) {
        push.error({
          message: i18n.global.t('error.dplData') + ": " + i18n.global.t('client.name')
        })
        return true
      }
      return false
    },
    // check duplicate tag
    checkTag (object: string, id: number, tag: string): boolean {
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
          message: i18n.global.t('error.dplData') + ": " + i18n.global.t('objects.tag')
        })
        return true
      }
      return false
    },
  }
})

export default Data
