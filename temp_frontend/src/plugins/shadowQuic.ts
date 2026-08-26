export const shadowQuicBBRProfileItems = ['conservative', 'standard', 'aggressive'] as const

export type ShadowQuicBBRProfile = typeof shadowQuicBBRProfileItems[number]

export type ShadowQuicInboundDefaultOptions = {
  alpn: string[]
  quic_versions: string[]
  zero_rtt: boolean
  congestion_controller: string
  up: number
  down: number
  cwnd: number
  max_idle_time: number
  max_datagram_frame_size: number
  recv_window_conn: number
  recv_window: number
  jls_upstream: {
    rate_limit: number
  }
}

// Keep new-inbound defaults independent from component mount timing.
export function createShadowQuicInboundDefaultOptions(): ShadowQuicInboundDefaultOptions {
  return {
    alpn: ['h3'],
    quic_versions: ['v2'],
    zero_rtt: true,
    congestion_controller: 'bbr',
    up: 500,
    down: 500,
    cwnd: 720,
    max_idle_time: 600000,
    max_datagram_frame_size: 1400,
    recv_window_conn: 33000000,
    recv_window: 160000000,
    jls_upstream: {
      rate_limit: 204800,
    },
  }
}

export function normalizeShadowQuicBBRProfile(value: unknown): ShadowQuicBBRProfile | '' {
  const profile = typeof value === 'string' ? value.trim().toLowerCase() : ''
  if (shadowQuicBBRProfileItems.includes(profile as ShadowQuicBBRProfile)) {
    return profile as ShadowQuicBBRProfile
  }
  return ''
}

export function normalizeShadowQuicJlsUpstreamAddr(value: unknown): string {
  const addr = typeof value === 'string' ? value.trim() : ''
  if (addr === '') return ''
  if (addr.startsWith('[')) {
    const closing = addr.indexOf(']')
    if (closing < 0) return addr
    const suffix = addr.slice(closing + 1)
    return suffix === '' ? `${addr}:443` : addr
  }
  const lastColon = addr.lastIndexOf(':')
  return lastColon < 0 || lastColon === addr.length - 1 || !/^\d+$/.test(addr.slice(lastColon + 1))
    ? `${addr}:443`
    : addr
}

export function shadowQuicJlsSniFromAddr(value: unknown): string {
  const addr = normalizeShadowQuicJlsUpstreamAddr(value)
  if (addr.startsWith('[')) {
    const closing = addr.indexOf(']')
    return closing > 0 ? addr.slice(1, closing) : ''
  }
  const lastColon = addr.lastIndexOf(':')
  return lastColon > 0 ? addr.slice(0, lastColon) : addr
}

function hasText(value: unknown): boolean {
  return typeof value === 'string' && value.trim() !== ''
}

function isValidPort(value: unknown): boolean {
  if (typeof value === 'number') {
    return Number.isInteger(value) && value >= 1 && value <= 65535
  }
  if (typeof value !== 'string') return false

  const text = value.trim()
  if (!/^\d+$/.test(text)) return false

  const port = Number(text)
  return Number.isInteger(port) && port >= 1 && port <= 65535
}

export function validateShadowQuicOutbound(outbound: Record<string, unknown> | null | undefined): string | undefined {
  if (!outbound) return 'ShadowQUIC 节点不能为空'
  if (!hasText(outbound.tag)) return 'ShadowQUIC 标签不能为空'
  if (!hasText(outbound.server)) return 'ShadowQUIC 服务器地址不能为空'
  if (!isValidPort(outbound.server_port)) return 'ShadowQUIC 端口必须在 1 到 65535 之间'
  if (!hasText(outbound.username)) return 'ShadowQUIC 用户名不能为空'
  if (!hasText(outbound.password)) return 'ShadowQUIC 密码不能为空'
  return undefined
}
