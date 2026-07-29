import HttpUtils from '@/plugins/httputil'

export type SubscriptionRuleSetProbeItem = {
  id: string
  kind: 'json' | 'clash'
  sourceId?: string
  scope: 'domain' | 'ip'
  name?: string
  url?: string
  allowFallback?: boolean
}

export type SubscriptionRuleSetProbeResult = {
  id: string
  valid: boolean
  url?: string
  sourceId?: string
  format?: string
  scope: 'domain' | 'ip'
  error?: string
  cached?: boolean
}

export async function probeSubscriptionRuleSets(
  items: SubscriptionRuleSetProbeItem[],
  signal?: AbortSignal,
): Promise<SubscriptionRuleSetProbeResult[]> {
  if (items.length === 0) return []
  const msg = await HttpUtils.post(
    'api/subscription-ruleset-probe',
    { items },
    {
      headers: { 'Content-Type': 'application/json' },
      timeout: 7000,
      signal,
      silentErrorToast: true,
    },
  )
  if (!msg.success || !Array.isArray(msg.obj)) {
    return items.map(item => ({
      id: item.id,
      valid: false,
      scope: item.scope,
      error: msg.msg || 'Rule-set probe failed',
    }))
  }
  return msg.obj as SubscriptionRuleSetProbeResult[]
}
