import axios, { type InternalAxiosRequestConfig } from 'axios'

const resolveApiBaseURL = () => {
  const injectedBaseURL = typeof window === 'undefined'
    ? ''
    : String((window as typeof window & { BASE_URL?: unknown }).BASE_URL ?? '').trim()
  let baseURL = injectedBaseURL

  // index.html leaves the template marker in place during local Vite work.
  if (!baseURL || baseURL.startsWith('{')) {
    baseURL = '/app/'
  }
  const normalizedPath = baseURL.replace(/^\/+/, '').replace(/\/+$/, '')
  return normalizedPath ? `/${normalizedPath}/` : '/'
}

export const panelBaseURL = resolveApiBaseURL()

const api = axios.create({
  baseURL: panelBaseURL,
  timeout: 30000,
})

api.defaults.headers.post['Content-Type'] = 'application/x-www-form-urlencoded; charset=UTF-8'
api.defaults.headers.common['X-Requested-With'] = 'XMLHttpRequest'

type PendingRequestEntry = {
  cancel: (message?: string) => void
  signature: string
}

const pendingRequests = new Map<string, PendingRequestEntry>()

type PendingRequestConfig = InternalAxiosRequestConfig & {
  __kworPendingRequestEntry?: PendingRequestEntry
}

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

const isDeduplicatedRequest = (config: any) => {
  const method = String(config?.method ?? '').toLowerCase()
  return method === 'get' || method === 'head'
}

const clearPendingRequest = (config: unknown) => {
  const entry = (config as PendingRequestConfig | undefined)?.__kworPendingRequestEntry
  if (entry && pendingRequests.get(entry.signature) === entry) {
    pendingRequests.delete(entry.signature)
  }
}

api.interceptors.request.use(
  (config) => {
    if (typeof FormData !== 'undefined' && config.data instanceof FormData) {
      config.headers = config.headers ?? {}
      config.headers['Content-Type'] = 'multipart/form-data'
    }

    if (!isDeduplicatedRequest(config)) {
      return config
    }

    const requestSignature = buildRequestSignature(config)
    const existing = pendingRequests.get(requestSignature)
    if (existing) {
      existing.cancel('Duplicate request cancelled')
    }

    const cancelSource = axios.CancelToken.source()
    config.cancelToken = cancelSource.token
    const entry: PendingRequestEntry = {
      cancel: cancelSource.cancel,
      signature: requestSignature,
    }
    const pendingConfig = config as PendingRequestConfig
    pendingConfig.__kworPendingRequestEntry = entry
    pendingRequests.set(requestSignature, entry)
    return config
  },
  (error) => Promise.reject(error),
)

api.interceptors.response.use(
  (response) => {
    clearPendingRequest(response.config)
    return response
  },
  (error) => {
    if (axios.isCancel(error)) {
      console.warn(error.message)
    }
    clearPendingRequest(error?.config)
    return Promise.reject(error)
  },
)

export default api
