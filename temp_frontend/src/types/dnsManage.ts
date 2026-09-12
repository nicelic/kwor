import type { ProviderMeta } from './ddns'

export interface DNSAccount {
  id: number
  displayId?: number
  name: string
  providerCode: string
  envJson: string
  remark: string
  createdAt?: string
  updatedAt?: string
}

export interface DNSRecord {
  id: string
  name: string
  type: string
  value: string
  ttl: number
  priority?: number
  proxied?: boolean
}

export interface DNSDomainFavorite {
  id: number
  accountId: number
  domain: string
  remark: string
  isDefault: boolean
  createdAt?: string
  updatedAt?: string
}

export interface DNSOverview {
  accounts: DNSAccount[]
  providers: ProviderMeta[]
  favorites: DNSDomainFavorite[]
}

export interface RecordForm {
  id: string
  name: string
  type: string
  value: string
  ttl: number
  priority: number
  proxied: boolean
}
