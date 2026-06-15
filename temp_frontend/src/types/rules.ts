interface generalRule {
  invert: boolean
  action: 'route' | 'route-options' | 'reject' | 'hijack-dns' | 'sniff' | 'resolve'
  outbound?: string
  override_address?: string
  override_port?: number
  udp_disable_domain_unmapping?: boolean
  udp_connect?: boolean
  udp_timeout?: string
  method?: string
  no_drop?: boolean
  sniffer: string[]
  timeout: string
  strategy: string
  server: string
}

export const actionKeys = [
  'invert',
  'action',
  'outbound',
  'override_address',
  'override_port',
  'udp_disable_domain_unmapping',
  'udp_connect',
  'udp_timeout',
  'method',
  'no_drop',
  'sniffer',
  'timeout',
  'strategy',
  'server'
]
export interface logicalRule extends generalRule {
  type: 'logical' | 'simple'
  mode: 'and' | 'or'
  rules: rule[]
}

export interface rule extends generalRule {
  inbound?: string[]
  ip_version?: 4 | 6
  network?: string[]
  auth_user?: string[]
  protocol?: string[]
  domain?: string[]
  domain_suffix?: string[]
  domain_keyword?: string[]
  domain_regex?: string[]
  source_ip_cidr?: string[]
  source_ip_is_private?: boolean
  ip_cidr?: string[]
  ip_is_private?: boolean
  source_port?: number[]
  source_port_range?: string[]
  port?: number[]
  port_range?: string[]
  process_name?: string[]
  process_path?: string[]
  process_path_regex?: string[]
  package_name?: string[]
  user?: string[]
  user_id?: number[]
  clash_mode?: string
  rule_set?: string[]
  rule_set_ip_cidr_match_source?: boolean
}

export interface ruleset {
  type: 'local' | 'remote' | 'file' | 'http' | 'inline'
  tag: string
  format?: 'source' | 'binary' | 'yaml' | 'text' | 'mrs'
  behavior?: 'domain' | 'ipcidr' | 'classical'
  path?: string
  url?: string
  payload?: string[]
  download_detour?: string
  proxy?: string
  update_interval?: string
}

export type RuleNamespace = 'default' | 'mihomo'

export interface RuleValidationOptions {
  outboundTags?: string[]
  ruleSetTags?: string[]
}

const mihomoHiddenRuleKeys = [
  'clash_mode',
  'ip_version',
  'package_name',
  'protocol',
  'user',
]

const mihomoTransientRuleKeys = [
  'type',
  'mode',
  'rules',
  'invert',
  'override_address',
  'override_port',
  'network_strategy',
  'fallback_delay',
  'udp_disable_domain_unmapping',
  'udp_connect',
  'udp_timeout',
  'sniffer',
  'timeout',
  'strategy',
  'server',
]

const mihomoBuiltInRouteTargets = ['DIRECT', 'REJECT', 'REJECT-DROP']
const mihomoRouteTargetAliases = new Set([...mihomoBuiltInRouteTargets, 'BLOCK'])

const cloneRuleValue = <T>(value: T): T => {
  if (value == null) {
    return value
  }
  return JSON.parse(JSON.stringify(value))
}

const normalizeStringList = (value: unknown): string[] => {
  const values = Array.isArray(value) ? value : [value]
  const normalized: string[] = []
  const seen = new Set<string>()
  for (const item of values) {
    if (typeof item !== 'string') {
      continue
    }
    const trimmed = item.trim()
    if (trimmed.length === 0 || seen.has(trimmed)) {
      continue
    }
    seen.add(trimmed)
    normalized.push(trimmed)
  }
  return normalized
}

const trimString = (value: unknown): string => {
  return typeof value === 'string' ? value.trim() : ''
}

const isKnownMihomoRouteTarget = (value: string, outboundTags: string[]): boolean => {
  const normalized = value.trim()
  if (normalized.length === 0) {
    return false
  }
  if (mihomoRouteTargetAliases.has(normalized.toUpperCase())) {
    return true
  }
  return outboundTags.some((tag) => tag === normalized)
}

export const getMihomoBuiltInTargets = (): string[] => {
  return [...mihomoBuiltInRouteTargets]
}

const toOptionalBool = (value: any): boolean | null => {
  if (value === true || value === false) {
    return value
  }
  if (typeof value === 'number') {
    return value !== 0
  }
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    if (normalized === 'true' || normalized === '1') return true
    if (normalized === 'false' || normalized === '0') return false
  }
  return null
}

export const normalizeMihomoRouteNoResolve = (route: any): boolean => {
  const sources = route == null || typeof route !== 'object'
    ? []
    : [
      route.no_resolve,
      route['no-resolve'],
      route.noResolve,
    ]

  for (const raw of sources) {
    const parsed = toOptionalBool(raw)
    if (parsed != null) {
      return parsed
    }
  }
  return true
}

export const sanitizeRuleForNamespace = (value: any, namespace: RuleNamespace | string = 'default'): any | null => {
  if (value == null || typeof value !== 'object') {
    return null
  }

  const rule = cloneRuleValue(value)
  if (namespace !== 'mihomo') {
    return rule
  }

  if (rule.type === 'logical') {
    return null
  }

  if (rule.action !== 'route' && rule.action !== 'reject') {
    return null
  }

  mihomoHiddenRuleKeys.forEach(key => delete rule[key])
  mihomoTransientRuleKeys.forEach(key => delete rule[key])

  if (rule.action !== 'route') {
    delete rule.outbound
  }
  delete rule.no_drop
  if (rule.action !== 'reject') {
    delete rule.method
  }

  return rule
}

export const sanitizeRulesForNamespace = (value: any[], namespace: RuleNamespace | string = 'default'): any[] => {
  if (!Array.isArray(value)) {
    return []
  }

  return value
    .map(rule => sanitizeRuleForNamespace(rule, namespace))
    .filter((rule): rule is any => rule != null)
}

export const sanitizeRouteForNamespace = (value: any, namespace: RuleNamespace | string = 'default'): any => {
  const route = cloneRuleValue(value ?? {})
  if (namespace !== 'mihomo') {
    return route
  }

  route.no_resolve = normalizeMihomoRouteNoResolve(route)
  delete route['no-resolve']
  delete route.noResolve
  route.rules = sanitizeRulesForNamespace(Array.isArray(route.rules) ? route.rules : [], namespace)
  return route
}

export const validateRuleForNamespace = (
  value: any,
  namespace: RuleNamespace | string = 'default',
  options: RuleValidationOptions = {},
): string[] => {
  if (namespace !== 'mihomo') {
    return []
  }

  const normalized = sanitizeRuleForNamespace(value, namespace)
  if (normalized == null) {
    return ['Mihomo rules only support simple route/reject entries.']
  }

  const outboundTags = normalizeStringList(options.outboundTags)
  const ruleSetTags = new Set(normalizeStringList(options.ruleSetTags))
  const errors: string[] = []
  const action = trimString(normalized.action)

  if (action === 'route') {
    const outbound = trimString(normalized.outbound)
    if (outbound.length === 0) {
      errors.push('Route rule requires an outbound target.')
    } else if (!isKnownMihomoRouteTarget(outbound, outboundTags)) {
      errors.push(`Route rule references unknown outbound "${outbound}".`)
    }
  }

  for (const tag of normalizeStringList(normalized.rule_set)) {
    if (!ruleSetTags.has(tag)) {
      errors.push(`Route rule references unknown rule_set "${tag}".`)
    }
  }

  return errors
}

export const validateRulesetForNamespace = (
  value: any,
  namespace: RuleNamespace | string = 'default',
): string[] => {
  if (namespace !== 'mihomo') {
    return []
  }

  const tag = trimString(value?.tag)
  const type = trimString(value?.type).toLowerCase()
  const errors: string[] = []

  if (tag.length === 0) {
    errors.push('Rule set requires a tag.')
  }

  if (type === 'file' || type === 'local') {
    if (trimString(value?.path).length === 0) {
      errors.push(`Rule set "${tag || '(unnamed)'}" requires a path.`)
    }
    return errors
  }

  if (type === 'http' || type === 'remote') {
    if (trimString(value?.url).length === 0) {
      errors.push(`Rule set "${tag || '(unnamed)'}" requires a URL.`)
    }
    return errors
  }

  if (type === 'inline') {
    if (normalizeStringList(value?.payload).length === 0) {
      errors.push(`Rule set "${tag || '(unnamed)'}" requires payload entries.`)
    }
    return errors
  }

  errors.push(`Rule set "${tag || '(unnamed)'}" uses unsupported type "${type || '(empty)'}".`)
  return errors
}

export const validateRouteForNamespace = (
  value: any,
  namespace: RuleNamespace | string = 'default',
  options: RuleValidationOptions = {},
): string[] => {
  if (namespace !== 'mihomo') {
    return []
  }

  const route = sanitizeRouteForNamespace(value, namespace)
  const outboundTags = normalizeStringList(options.outboundTags)
  const routeRuleSets = Array.isArray(route.rule_set) ? route.rule_set : []
  const ruleSetTags: string[] = []
  const knownRuleSetTags = new Set<string>()
  const errors: string[] = []

  routeRuleSets.forEach((rawRuleset: any, index: number) => {
    const tag = trimString(rawRuleset?.tag)
    if (tag.length === 0) {
      errors.push(`Rule set #${index + 1} requires a tag.`)
      return
    }
    if (knownRuleSetTags.has(tag)) {
      errors.push(`Rule set tag "${tag}" is duplicated.`)
      return
    }
    knownRuleSetTags.add(tag)
    const rulesetErrors = validateRulesetForNamespace(rawRuleset, namespace)
    rulesetErrors.forEach((message) => {
      errors.push(`Rule set #${index + 1}: ${message}`)
    })
    if (rulesetErrors.length === 0) {
      ruleSetTags.push(tag)
    }
  })

  const finalTarget = trimString(route.final)
  if (finalTarget.length > 0 && !isKnownMihomoRouteTarget(finalTarget, outboundTags)) {
    errors.push(`route.final references unknown outbound "${finalTarget}".`)
  }

  const rules: any[] = Array.isArray(route.rules) ? route.rules : []
  rules.forEach((rule: any, index: number) => {
    for (const message of validateRuleForNamespace(rule, namespace, {
      outboundTags,
      ruleSetTags,
    })) {
      errors.push(`Rule #${index + 1}: ${message}`)
    }
  })

  return errors
}
