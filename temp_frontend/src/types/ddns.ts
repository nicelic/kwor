export interface ProviderFieldDef {
  key: string
  label: string
  required: boolean
  placeholder?: string
  secret?: boolean
}

export interface ProviderMeta {
  code: string
  name: string
  helper: string
  supportGeneralDns?: boolean
  fields: ProviderFieldDef[]
}

export interface DDNSAccount {
  id: number
  displayId?: number
  name: string
  providerCode: string
  envJson: string
  remark: string
  createdAt?: string
  updatedAt?: string
}

export interface DDNSRule {
  id: number
  displayId?: number
  listOrder?: number
  name: string
  enabled: boolean
  accountId: number
  domains: string
  ipType: 'ipv4' | 'ipv6' | 'dual'

  ipv4Source: string
  ipv4Url: string
  ipv4Interface: string

  ipv6Source: string
  ipv6Url: string
  ipv6Interface: string

  intervalMinutes: number
  localIntervalSeconds?: number
  ttl: number
  cloudflareProxy: boolean

  lastIpv4: string
  lastIpv6: string
  lastSyncTime?: string | null
  lastStatus: 'success' | 'error' | 'pending' | 'syncing'
  lastError: string

  createdAt?: string
  updatedAt?: string
}

export interface InterfaceInfo {
  name: string
  ips: string[]
  flags: string
}

export interface DDNSOverview {
  rules: DDNSRule[]
  accounts: DDNSAccount[]
  providers: ProviderMeta[]
  interfaces: InterfaceInfo[]
}
