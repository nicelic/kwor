export const shadowQuicBBRProfileItems = ['conservative', 'standard', 'aggressive'] as const

export type ShadowQuicBBRProfile = typeof shadowQuicBBRProfileItems[number]

export function normalizeShadowQuicBBRProfile(value: unknown): ShadowQuicBBRProfile | '' {
  const profile = typeof value === 'string' ? value.trim().toLowerCase() : ''
  if (shadowQuicBBRProfileItems.includes(profile as ShadowQuicBBRProfile)) {
    return profile as ShadowQuicBBRProfile
  }
  return ''
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
