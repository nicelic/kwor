import HttpUtils from '@/plugins/httputil'

export type SubscriptionURIResult = {
  success: boolean
  subURI: string
  error: string
}

let pendingSubscriptionURIRequest: Promise<SubscriptionURIResult> | null = null

export const normalizeSubscriptionURI = (value: unknown): string => {
  const subURI = String(value ?? '').trim()
  if (!subURI) return ''
  try {
    const url = new URL(subURI)
    if (url.protocol !== 'http:' && url.protocol !== 'https:') return ''
    if (!url.host || url.search || url.hash) return ''
    return `${subURI.replace(/\/+$/, '')}/`
  } catch {
    return ''
  }
}

export const refreshSubscriptionURI = (): Promise<SubscriptionURIResult> => {
  if (pendingSubscriptionURIRequest) return pendingSubscriptionURIRequest

  pendingSubscriptionURIRequest = (async () => {
    const msg = await HttpUtils.get('api/subscription-uri', {}, {
      timeout: 15000,
      silentErrorToast: true,
    })
    if (!msg.success) {
      return {
        success: false,
        subURI: '',
        error: String(msg.msg || '无法读取订阅地址。'),
      }
    }

    const subURI = normalizeSubscriptionURI(msg.obj?.subURI)
    if (!subURI) {
      return {
        success: false,
        subURI: '',
        error: '订阅地址无效。',
      }
    }
    return { success: true, subURI, error: '' }
  })().finally(() => {
    pendingSubscriptionURIRequest = null
  })

  return pendingSubscriptionURIRequest
}
