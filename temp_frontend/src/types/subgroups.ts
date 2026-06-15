export interface SubGroup {
  id: number
  name: string
  sort_order?: number
  outbounds: string[]
  subscription_url?: string
  subscription_url_clash?: string
  allow_insecure?: boolean
  auto_update_last_at?: number
  auto_update_failed_sources?: string | string[]
  auto_update_error?: string
  createdAt?: string
  updatedAt?: string
}

export function createSubGroup(data?: Partial<SubGroup>): SubGroup {
  return {
    id: 0,
    name: '',
    sort_order: 0,
    outbounds: [],
    subscription_url: '',
    subscription_url_clash: '',
    allow_insecure: false,
    auto_update_last_at: 0,
    auto_update_failed_sources: '',
    auto_update_error: '',
    ...data
  }
}
