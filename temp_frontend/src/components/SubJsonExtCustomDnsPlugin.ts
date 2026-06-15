type CustomRuleRoute = 'reject' | 'direct' | 'proxy'
type ManagedDomainMatcherKey = 'domain' | 'domain_suffix' | 'domain_keyword' | 'domain_regex'

type ManagedCustomDomainKeywordDnsRule = {
  action: 'reject' | 'route'
  server?: string
  domain?: string[]
  domain_suffix?: string[]
  domain_keyword?: string[]
  domain_regex?: string[]
}

const managedDomainMatcherKeys: ManagedDomainMatcherKey[] = [
  'domain',
  'domain_suffix',
  'domain_keyword',
  'domain_regex',
]

const managedDirectDnsServer = 'direct-dns'
const managedProxyDnsServer = 'proxy-dns'
const managedFakeipDnsServer = 'fakeip'

function isPlainObject(input: any): input is Record<string, any> {
  return Boolean(input) && typeof input === 'object' && !Array.isArray(input)
}

function normalizeStringArray(input: any): string[] {
  const list = Array.isArray(input) ? input : input != null ? [input] : []
  const result: string[] = []
  const seen = new Set<string>()
  for (const item of list) {
    if (typeof item !== 'string') continue
    const value = item.trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    result.push(value)
  }
  return result
}

function normalizeCustomRuleRoute(input: any): CustomRuleRoute | '' {
  const route = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (route === 'reject' || route === 'direct' || route === 'proxy') {
    return route
  }
  return ''
}

function normalizeManagedDnsServer(input: any): string {
  const server = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (server === managedDirectDnsServer || server === managedProxyDnsServer || server === managedFakeipDnsServer) {
    return server
  }
  return ''
}

function normalizeManagedDomainMatcherKey(input: any): ManagedDomainMatcherKey | '' {
  const key = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (key === 'domain' || key === 'domain_suffix' || key === 'domain_keyword' || key === 'domain_regex') {
    return key
  }
  return ''
}

function hasOnlyKeys(input: Record<string, any>, allowedKeys: string[]): boolean {
  const allowed = new Set<string>(allowedKeys)
  return Object.keys(input).every((key) => allowed.has(key))
}

function buildLegacyManagedCustomDomainKeywordDnsRuleKey(
  action: 'reject' | 'route',
  domainKeywordInput: any,
  serverInput?: any
): string {
  const domainKeyword = normalizeStringArray(domainKeywordInput)
  if (domainKeyword.length === 0) return ''

  if (action === 'reject') {
    return JSON.stringify({
      action: 'reject',
      domain_keyword: domainKeyword,
    })
  }

  const server = normalizeManagedDnsServer(serverInput)
  if (!server) return ''
  return JSON.stringify({
    action: 'route',
    server,
    domain_keyword: domainKeyword,
  })
}

export function buildManagedCustomDomainKeywordDnsRuleKey(
  action: 'reject' | 'route',
  matcherKeyInput: any,
  matcherValuesInput: any,
  serverInput?: any
): string {
  const matcherKey = normalizeManagedDomainMatcherKey(matcherKeyInput)
  if (!matcherKey) return ''

  const matcherValues = normalizeStringArray(matcherValuesInput)
  if (matcherValues.length === 0) return ''

  if (action === 'reject') {
    return JSON.stringify({
      action: 'reject',
      matcher_key: matcherKey,
      matcher_values: matcherValues,
    })
  }

  const server = normalizeManagedDnsServer(serverInput)
  if (!server) return ''
  return JSON.stringify({
    action: 'route',
    server,
    matcher_key: matcherKey,
    matcher_values: matcherValues,
  })
}

function getManagedDomainMatcherFromRule(rule: any): { key: ManagedDomainMatcherKey; values: string[] } | null {
  if (!isPlainObject(rule)) return null
  const matched: Array<{ key: ManagedDomainMatcherKey; values: string[] }> = []
  for (const key of managedDomainMatcherKeys) {
    const values = normalizeStringArray(rule[key])
    if (values.length === 0) continue
    matched.push({ key, values })
  }
  if (matched.length !== 1) return null
  return matched[0]
}

function getLegacyManagedCustomDomainKeywordDnsRuleKey(rule: any): string {
  if (!isPlainObject(rule)) return ''
  if (Object.prototype.hasOwnProperty.call(rule, 'rule_set')) return ''
  if (Object.prototype.hasOwnProperty.call(rule, 'query_type')) return ''

  const domainKeyword = normalizeStringArray(rule.domain_keyword)
  if (domainKeyword.length === 0) return ''

  const action = typeof rule.action === 'string' ? rule.action.trim().toLowerCase() : ''
  if (action === 'reject') {
    if (!hasOnlyKeys(rule, ['action', 'domain_keyword'])) return ''
    return buildLegacyManagedCustomDomainKeywordDnsRuleKey('reject', domainKeyword)
  }

  if (action !== 'route') return ''
  const server = normalizeManagedDnsServer(rule.server)
  if (!server) return ''
  if (!hasOnlyKeys(rule, ['action', 'server', 'domain_keyword'])) return ''
  return buildLegacyManagedCustomDomainKeywordDnsRuleKey('route', domainKeyword, server)
}

export function getManagedCustomDomainKeywordDnsRuleKey(rule: any): string {
  if (!isPlainObject(rule)) return ''
  if (Object.prototype.hasOwnProperty.call(rule, 'rule_set')) return ''
  if (Object.prototype.hasOwnProperty.call(rule, 'query_type')) return ''

  const matcher = getManagedDomainMatcherFromRule(rule)
  if (!matcher) return ''

  const action = typeof rule.action === 'string' ? rule.action.trim().toLowerCase() : ''
  if (action === 'reject') {
    if (!hasOnlyKeys(rule, ['action', matcher.key])) return ''
    return buildManagedCustomDomainKeywordDnsRuleKey('reject', matcher.key, matcher.values)
  }

  if (action !== 'route') return ''
  const server = normalizeManagedDnsServer(rule.server)
  if (!server) return ''
  if (!hasOnlyKeys(rule, ['action', 'server', matcher.key])) return ''
  return buildManagedCustomDomainKeywordDnsRuleKey('route', matcher.key, matcher.values, server)
}

export function normalizeManagedCustomDomainKeywordDnsRuleKeys(input: any): string[] {
  const list = Array.isArray(input) ? input : []
  const result: string[] = []
  const seen = new Set<string>()
  for (const item of list) {
    if (typeof item !== 'string') continue
    const key = item.trim()
    if (!key || seen.has(key)) continue
    seen.add(key)
    result.push(key)
  }
  return result
}

export function mergeManagedCustomDomainKeywordDnsRuleKeys(previousKeys: any, currentKeys: any): string[] {
  const merged = [
    ...normalizeManagedCustomDomainKeywordDnsRuleKeys(previousKeys),
    ...normalizeManagedCustomDomainKeywordDnsRuleKeys(currentKeys),
  ]
  return normalizeManagedCustomDomainKeywordDnsRuleKeys(merged)
}

export function stripManagedCustomDomainKeywordDnsRules(rulesInput: any, managedRuleKeys: any): any[] {
  const rules = Array.isArray(rulesInput) ? rulesInput : []
  const keySet = new Set<string>(normalizeManagedCustomDomainKeywordDnsRuleKeys(managedRuleKeys))
  if (keySet.size === 0) return [...rules]

  return rules.filter((rule: any) => {
    const key = getManagedCustomDomainKeywordDnsRuleKey(rule)
    if (key && keySet.has(key)) return false

    const legacyKey = getLegacyManagedCustomDomainKeywordDnsRuleKey(rule)
    if (legacyKey && keySet.has(legacyKey)) return false

    return true
  })
}

export function collectManagedCustomDomainKeywordDnsRuleKeysFromRules(rulesInput: any): string[] {
  const rules = Array.isArray(rulesInput) ? rulesInput : []
  const keys = new Set<string>()

  for (const rule of rules) {
    const key = getManagedCustomDomainKeywordDnsRuleKey(rule)
    if (key) {
      keys.add(key)
      continue
    }

    const legacyKey = getLegacyManagedCustomDomainKeywordDnsRuleKey(rule)
    if (legacyKey) {
      keys.add(legacyKey)
    }
  }

  return Array.from(keys)
}

function buildManagedCustomDomainKeywordDnsRule(
  matcherKey: ManagedDomainMatcherKey,
  matcherValues: string[],
  route: CustomRuleRoute,
  fakeipEnabled: boolean
): ManagedCustomDomainKeywordDnsRule | null {
  if (matcherValues.length === 0) return null

  const rule: ManagedCustomDomainKeywordDnsRule = route === 'reject'
    ? { action: 'reject' }
    : {
        action: 'route',
        server: route === 'direct'
          ? managedDirectDnsServer
          : (fakeipEnabled ? managedFakeipDnsServer : managedProxyDnsServer),
      }

  rule[matcherKey] = matcherValues
  return rule
}

export function buildManagedCustomDomainKeywordDnsRules(
  ruleRowsInput: any,
  fakeipEnabled: boolean
): { rules: ManagedCustomDomainKeywordDnsRule[]; ruleKeys: string[] } {
  const ruleRows = Array.isArray(ruleRowsInput) ? ruleRowsInput : []
  const rules: ManagedCustomDomainKeywordDnsRule[] = []
  const ruleKeys = new Set<string>()

  for (const row of ruleRows) {
    const kind = typeof row?.kind === 'string' ? row.kind.trim().toLowerCase() : ''
    if (kind !== 'custom') continue

    const name = typeof row?.name === 'string' ? row.name.trim() : ''
    if (name) continue

    const matcherKey = normalizeManagedDomainMatcherKey(row?.customType)
    if (!matcherKey) continue

    const route = normalizeCustomRuleRoute(row?.route)
    if (!route) continue

    const matcherValues = normalizeStringArray(row?.values)
    if (matcherValues.length === 0) continue

    const built = buildManagedCustomDomainKeywordDnsRule(matcherKey, matcherValues, route, fakeipEnabled)
    if (!built) continue

    const key = buildManagedCustomDomainKeywordDnsRuleKey(
      built.action,
      matcherKey,
      matcherValues,
      built.server
    )
    if (!key) continue

    rules.push(built)
    ruleKeys.add(key)
  }

  return {
    rules,
    ruleKeys: Array.from(ruleKeys),
  }
}
