export interface OutboundGroup {
  id: number
  name: string
  sort_order?: number
  outbounds: string[]
  subscription_url?: string
  allow_insecure?: boolean
  createdAt?: string
  updatedAt?: string
}

export function createOutboundGroup(data?: Partial<OutboundGroup>): OutboundGroup {
  return {
    id: 0,
    name: '',
    sort_order: 0,
    outbounds: [],
    subscription_url: '',
    allow_insecure: false,
    ...data
  }
}
