import axios from 'axios'

const api = axios.create({
  baseURL: './',
})

api.defaults.headers.post['Content-Type'] = 'application/x-www-form-urlencoded; charset=UTF-8'
api.defaults.headers.common['X-Requested-With'] = 'XMLHttpRequest'

type PendingRequestEntry = {
  cancel: (message?: string) => void
  signature: string
}

const pendingRequests = new Map<string, PendingRequestEntry>()

const stableSerialize = (value: unknown): string => {
  if (value == null) return ''
  if (typeof value === 'string') return value
  if (typeof value !== 'object') return String(value)
  if (Array.isArray(value)) return `[${value.map(stableSerialize).join(',')}]`

  const entries = Object.entries(value as Record<string, unknown>).sort(([left], [right]) => left.localeCompare(right))
  return `{${entries.map(([key, item]) => `${key}:${stableSerialize(item)}`).join(',')}}`
}

const buildRequestKey = (config: any) => `${String(config?.method ?? '').toLowerCase()}:${String(config?.url ?? '')}`

const buildRequestSignature = (config: any) => {
  const params = stableSerialize(config?.params)
  const data = stableSerialize(config?.data)
  return `${buildRequestKey(config)}|params=${params}|data=${data}`
}

api.interceptors.request.use(
  (config) => {
    const requestKey = buildRequestKey(config)
    const requestSignature = buildRequestSignature(config)

    if (pendingRequests.has(requestKey)) {
      const existing = pendingRequests.get(requestKey)
      if (existing?.signature === requestSignature) {
        existing.cancel('Duplicate request cancelled')
      }
    }

    const cancelSource = axios.CancelToken.source()
    config.cancelToken = cancelSource.token
    pendingRequests.set(requestKey, {
      cancel: cancelSource.cancel,
      signature: requestSignature,
    })

    if (typeof FormData !== 'undefined' && config.data instanceof FormData) {
      config.headers = config.headers ?? {}
      config.headers['Content-Type'] = 'multipart/form-data'
    }
    return config
  },
  (error) => Promise.reject(error),
)

api.interceptors.response.use(
  (response) => {
    const requestKey = buildRequestKey(response.config)
    pendingRequests.delete(requestKey)
    return response
  },
  (error) => {
    if (axios.isCancel(error)) {
      console.warn(error.message)
    }
    const requestKey = buildRequestKey(error?.config)
    if (requestKey !== 'undefined:undefined') {
      pendingRequests.delete(requestKey)
    }
    return Promise.reject(error)
  },
)

export default api
