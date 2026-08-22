interface generalRule {
  invert: boolean
  action: 'route' | 'reject' | 'hijack-dns' | 'sniff' | 'resolve'
  outbound?: string
  override_address?: string
  override_port?: number
  network_strategy?: string
  fallback_delay?: string
  udp_disable_domain_unmapping?: boolean
  udp_connect?: boolean
  udp_timeout?: string
  method?: string
  no_drop?: boolean
  sniffer: string[]
  timeout: string
  strategy: string
  server: string
  disable_cache?: boolean
  disable_optimistic_cache?: boolean
  rewrite_ttl?: number | string
  client_subnet?: string
}

export const actionKeys = [
  'invert',
  'action',
  'outbound',
  'override_address',
  'override_port',
  'network_strategy',
  'fallback_delay',
  'udp_disable_domain_unmapping',
  'udp_connect',
  'udp_timeout',
  'method',
  'no_drop',
  'sniffer',
  'timeout',
  'strategy',
  'server',
  'disable_cache',
  'disable_optimistic_cache',
  'rewrite_ttl',
  'client_subnet'
]
export interface logicalRule extends generalRule {
  type: 'logical' | 'simple'
  mode: 'and' | 'or'
  rules: rule[]
}

// Mihomo route inputs are edited as text. Keep malformed tokens representable
// in the draft so validation can reject them instead of silently truncating
// values such as "80x" to 80.
export type MihomoNumericRuleValue = number | string

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
  source_port?: MihomoNumericRuleValue[]
  source_port_range?: string[]
  port?: MihomoNumericRuleValue[]
  port_range?: string[]
  process_name?: string[]
  process_path?: string[]
  process_path_regex?: string[]
  package_name?: string[]
  user?: string[]
  user_id?: MihomoNumericRuleValue[]
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
  rules?: unknown[]
  download_detour?: string
  proxy?: string
  update_interval?: string
  initial_path?: string
  http_client?: Record<string, unknown>
}

export type RuleNamespace = 'default' | 'mihomo'

export interface RuleValidationOptions {
  outboundTags?: string[]
  ruleSetTags?: string[]
  inboundTags?: string[]
}

export const mihomoRouteResourceLimits = {
  configBytes: 2 * 1024 * 1024,
  rules: 256,
  ruleProviders: 128,
  valuesPerMatcher: 64,
  valueBytes: 1024,
  combinationsPerRule: 512,
  renderedRules: 8192,
} as const

export const singboxRouteResourceLimits = {
  routeBytes: 1 * 1024 * 1024,
  rules: 512,
  ruleSets: 256,
  ruleBytes: 32 * 1024,
  ruleSetBytes: 32 * 1024,
  rulesBytes: 512 * 1024,
  logicalDepth: 6,
  logicalChildren: 32,
} as const

const mihomoMatcherValueKeys = [
  'domain',
  'domain_suffix',
  'domain_keyword',
  'domain_regex',
  'ip_cidr',
  'network',
  'auth_user',
  'source_ip_cidr',
  'process_name',
  'process_path',
  'process_path_regex',
  'rule_set',
  'port',
  'port_range',
  'source_port',
  'source_port_range',
  'user_id',
  'inbound',
]

const mihomoCombinationKeys = [
  'domain',
  'domain_suffix',
  'domain_keyword',
  'domain_regex',
  'ip_cidr',
  'network',
  'auth_user',
  'source_ip_cidr',
  'process_name',
  'process_path',
  'process_path_regex',
  'rule_set',
  'user_id',
]

const resourceTextEncoder = new TextEncoder()

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
  'disable_cache',
  'disable_optimistic_cache',
  'rewrite_ttl',
  'client_subnet',
]

const mihomoBuiltInRouteTargets = ['DIRECT', 'REJECT', 'REJECT-DROP']
const mihomoRouteTargetAliases = new Set([...mihomoBuiltInRouteTargets, 'BLOCK'])
const mihomoDefaultDirectOutboundKeys = new Set(['id', 'type', 'tag'])

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

const isValidMihomoInterval = (value: string): boolean => {
  const normalized = value.trim().toLowerCase()
  if (/^\d+$/.test(normalized)) {
    return Number(normalized) > 0
  }
  const match = /^(\d+)\s*([smhd])$/.exec(normalized)
  return match != null && Number(match[1]) > 0
}

const normalizeMihomoRouteTargetAlias = (value: unknown): string => {
  const target = trimString(value)
  switch (target.toUpperCase()) {
    case 'DIRECT':
      return 'DIRECT'
    case 'REJECT':
    case 'BLOCK':
      return 'REJECT'
    case 'REJECT-DROP':
      return 'REJECT-DROP'
    default:
      return target
  }
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

/**
 * The database historically seeded a bare `direct` outbound for Mihomo.
 * Mihomo already provides the equivalent built-in `DIRECT` target, so the
 * bare compatibility row should not be offered as a second route choice.
 * Keep any direct outbound carrying extra options visible so user settings
 * are never hidden or altered by the route selector.
 */
export const isMihomoDefaultDirectOutbound = (value: unknown): boolean => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return false
  }

  const outbound = value as Record<string, unknown>
  const type = trimString(outbound.type).toLowerCase()
  const tag = trimString(outbound.tag)
  if (type !== 'direct' || tag !== 'direct') {
    return false
  }

  return Object.entries(outbound).every(([key, item]) => {
    if (mihomoDefaultDirectOutboundKeys.has(key)) {
      return true
    }
    if (item == null || item === '') {
      return true
    }
    return Array.isArray(item) && item.length === 0
  })
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
  } else {
    const target = normalizeMihomoRouteTargetAlias(rule.outbound)
    if (target) rule.outbound = target
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
  const finalTarget = normalizeMihomoRouteTargetAlias(route.final)
  if (finalTarget) route.final = finalTarget
  else delete route.final
  route.rules = sanitizeRulesForNamespace(Array.isArray(route.rules) ? route.rules : [], namespace)
  return route
}

const mihomoRuleValues = (value: unknown): unknown[] => {
  return Array.isArray(value) ? value : value == null ? [] : [value]
}

const validateMihomoIntegerMatcher = (
  value: unknown,
  field: string,
  min: number,
  max: number,
): string[] => {
  const errors: string[] = []
  mihomoRuleValues(value).forEach((item, index) => {
    if (typeof item !== 'number' || !Number.isSafeInteger(item) || item < min || item > max) {
      errors.push(`${field}[${index + 1}] must be an integer from ${min} to ${max}.`)
    }
  })
  return errors
}

const validateMihomoPortRangeMatcher = (value: unknown, field: string): string[] => {
  const errors: string[] = []
  mihomoRuleValues(value).forEach((item, index) => {
    if (typeof item !== 'string') {
      errors.push(`${field}[${index + 1}] must use the form start-end.`)
      return
    }
    const match = /^(\d+)-(\d+)$/.exec(item.trim())
    if (!match) {
      errors.push(`${field}[${index + 1}] must use the form start-end.`)
      return
    }
    const start = Number(match[1])
    const end = Number(match[2])
    if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start < 1 || end > 65535 || start > end) {
      errors.push(`${field}[${index + 1}] must be a valid port range from 1 to 65535.`)
    }
  })
  return errors
}

export const validateMihomoRuleNumericFields = (value: any): string[] => {
  return [
    ...validateMihomoIntegerMatcher(value?.port, 'port', 1, 65535),
    ...validateMihomoPortRangeMatcher(value?.port_range, 'port_range'),
    ...validateMihomoIntegerMatcher(value?.source_port, 'source_port', 1, 65535),
    ...validateMihomoPortRangeMatcher(value?.source_port_range, 'source_port_range'),
    ...validateMihomoIntegerMatcher(value?.user_id, 'user_id', 0, Number.MAX_SAFE_INTEGER),
  ]
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

  if (options.inboundTags) {
    const knownInboundTags = new Set(normalizeStringList(options.inboundTags))
    for (const tag of normalizeStringList(normalized.inbound)) {
      if (!knownInboundTags.has(tag)) {
        errors.push(`Route rule references unavailable inbound "${tag}".`)
      }
    }
  }

  for (const tag of normalizeStringList(normalized.rule_set)) {
    if (!ruleSetTags.has(tag)) {
      errors.push(`Route rule references unknown rule_set "${tag}".`)
    }
  }

  errors.push(...validateMihomoRuleNumericFields(normalized))

  return errors
}

export const validateRulesetForNamespace = (
  value: any,
  namespace: RuleNamespace | string = 'default',
  options: RuleValidationOptions = {},
): string[] => {
  const tag = trimString(value?.tag)
  const type = trimString(value?.type).toLowerCase()
  const errors: string[] = []

  if (tag.length === 0) {
    errors.push('Rule set requires a tag.')
  }

	if (namespace !== 'mihomo') {
		if (type.length === 0) {
			return errors
		}
		const format = trimString(value?.format)
		const path = trimString(value?.path)
		const url = trimString(value?.url)
    const detour = trimString(value?.download_detour || value?.http_client?.detour)
        if (type === 'local') {
          if (path.length === 0) errors.push(`Rule set "${tag || '(unnamed)'}" requires a path.`)
          if (url.length > 0) errors.push(`Rule set "${tag || '(unnamed)'}" cannot contain a URL.`)
          if (detour.length > 0) errors.push(`Rule set "${tag || '(unnamed)'}" cannot contain a remote download detour.`)
          if (value?.initial_path != null || value?.http_client != null) errors.push(`Rule set "${tag || '(unnamed)'}" contains fields that are not valid for local rules.`)
    } else if (type === 'remote') {
      if (url.length === 0) errors.push(`Rule set "${tag || '(unnamed)'}" requires a URL.`)
      if (path.length > 0) errors.push(`Rule set "${tag || '(unnamed)'}" cannot contain a path.`)
      if (value?.http_client !== undefined && (value.http_client == null || typeof value.http_client !== 'object' || Array.isArray(value.http_client))) {
        errors.push(`Rule set "${tag || '(unnamed)'}" has an invalid http_client.`)
      }
      if (detour.length > 0 && options.outboundTags && !options.outboundTags.includes(detour)) {
        errors.push(`Rule set "${tag || '(unnamed)'}" references unknown download detour "${detour}".`)
      }
        } else if (type === 'inline') {
			if (!Array.isArray(value?.rules) || value.rules.length === 0) {
				errors.push(`Rule set "${tag || '(unnamed)'}" requires inline rules.`)
			}
          if (format.length > 0 || path.length > 0 || url.length > 0 || detour.length > 0 || trimString(value?.update_interval).length > 0 || value?.initial_path != null || value?.http_client != null) {
            errors.push(`Rule set "${tag || '(unnamed)'}" contains fields that are not valid for inline rules.`)
          }
		} else {
			errors.push(`Rule set "${tag || '(unnamed)'}" uses unsupported type "${type || '(empty)'}".`)
		}
		if (format.length > 0 && format !== 'source' && format !== 'binary') {
			errors.push(`Rule set "${tag || '(unnamed)'}" uses unsupported format "${format}".`)
		}
		return errors
	}

  if (type === 'file' || type === 'local') {
    if (trimString(value?.path).length === 0) {
      errors.push(`Rule set "${tag || '(unnamed)'}" requires a path.`)
    }
    const format = trimString(value?.format).toLowerCase()
    const behavior = trimString(value?.behavior).toLowerCase()
    if (format && !['yaml', 'text', 'mrs', 'source', 'binary'].includes(format)) {
      errors.push(`Rule set "${tag || '(unnamed)'}" uses unsupported format "${format}".`)
    }
    if (format === 'mrs' || format === 'binary') {
      if (behavior && behavior !== 'domain' && behavior !== 'ipcidr' && behavior !== 'classical') {
        errors.push(`Rule set "${tag || '(unnamed)'}" with MRS format uses unsupported behavior "${behavior}".`)
      }
    } else if (behavior && !['classical', 'domain', 'ipcidr'].includes(behavior)) {
      errors.push(`Rule set "${tag || '(unnamed)'}" uses unsupported behavior "${behavior}".`)
    }
    return errors
  }

  if (type === 'http' || type === 'remote') {
    if (trimString(value?.url).length === 0) {
      errors.push(`Rule set "${tag || '(unnamed)'}" requires a URL.`)
    }
    const proxy = trimString(value?.proxy || value?.download_detour)
    if (proxy && options.outboundTags && !isKnownMihomoRouteTarget(proxy, normalizeStringList(options.outboundTags))) {
      errors.push(`Rule set "${tag || '(unnamed)'}" references unknown proxy "${proxy}".`)
    }
    const interval = trimString(value?.update_interval)
    if (interval && !isValidMihomoInterval(interval)) {
      errors.push(`Rule set "${tag || '(unnamed)'}" uses an invalid update interval.`)
    }
    const format = trimString(value?.format).toLowerCase()
    const behavior = trimString(value?.behavior).toLowerCase()
    if (format && !['yaml', 'text', 'mrs', 'source', 'binary'].includes(format)) {
      errors.push(`Rule set "${tag || '(unnamed)'}" uses unsupported format "${format}".`)
    }
    if (format === 'mrs' || format === 'binary') {
      if (behavior && behavior !== 'domain' && behavior !== 'ipcidr' && behavior !== 'classical') {
        errors.push(`Rule set "${tag || '(unnamed)'}" with MRS format uses unsupported behavior "${behavior}".`)
      }
    } else if (behavior && !['classical', 'domain', 'ipcidr'].includes(behavior)) {
      errors.push(`Rule set "${tag || '(unnamed)'}" uses unsupported behavior "${behavior}".`)
    }
    return errors
  }

  if (type === 'inline') {
    if (normalizeStringList(value?.payload).length === 0) {
      errors.push(`Rule set "${tag || '(unnamed)'}" requires payload entries.`)
    }
    const behavior = trimString(value?.behavior).toLowerCase()
    if (behavior && !['classical', 'domain', 'ipcidr'].includes(behavior)) {
      errors.push(`Rule set "${tag || '(unnamed)'}" uses unsupported behavior "${behavior}".`)
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
    const rulesetErrors = validateRulesetForNamespace(rawRuleset, namespace, options)
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
      inboundTags: options.inboundTags,
    })) {
      errors.push(`Rule #${index + 1}: ${message}`)
    }
  })

  return errors
}

const resourceValues = (value: unknown): unknown[] => Array.isArray(value) ? value : value == null ? [] : [value]

const resourceValueCount = (rule: any, key: string): number => {
  if (key === 'port' || key === 'source_port') {
    const rangeKey = key === 'port' ? 'port_range' : 'source_port_range'
    return resourceValues(rule?.[key]).length + resourceValues(rule?.[rangeKey]).length
  }
  return resourceValues(rule?.[key]).length
}

export const validateMihomoRouteResourceBounds = (route: any, inboundCount = 0, config: unknown = { route: route ?? {} }): string[] => {
  const errors: string[] = []
  const rules = Array.isArray(route?.rules) ? route.rules : []
  const providers = Array.isArray(route?.rule_set) ? route.rule_set : []
  if (rules.length > mihomoRouteResourceLimits.rules) {
    errors.push(`Mihomo 路由规则最多允许 ${mihomoRouteResourceLimits.rules} 条。`)
  }
  if (providers.length > mihomoRouteResourceLimits.ruleProviders) {
    errors.push(`Mihomo 规则集 provider 最多允许 ${mihomoRouteResourceLimits.ruleProviders} 条。`)
  }

  // Every generated rule list ends with MATCH. Count those terminal rules too,
  // otherwise a configuration at the apparent limit can still fail after save.
  let renderedRules = Math.max(1, inboundCount + 1)
  if (renderedRules > mihomoRouteResourceLimits.renderedRules) {
    errors.push(`当前配置展开后不能超过 ${mihomoRouteResourceLimits.renderedRules} 条 Mihomo 运行规则。`)
  }
  for (const [index, rule] of rules.entries()) {
    for (const message of validateMihomoRuleNumericFields(rule)) {
      errors.push(`Rule #${index + 1}: ${message}`)
    }
    for (const key of mihomoMatcherValueKeys) {
      const values = resourceValues(rule?.[key])
      if (values.length > mihomoRouteResourceLimits.valuesPerMatcher) {
        errors.push(`规则 #${index + 1} 的 ${key} 最多允许 ${mihomoRouteResourceLimits.valuesPerMatcher} 项。`)
      }
      for (const value of values) {
        if (typeof value === 'string' && resourceTextEncoder.encode(value).byteLength > mihomoRouteResourceLimits.valueBytes) {
          errors.push(`规则 #${index + 1} 的 ${key} 单项不能超过 ${mihomoRouteResourceLimits.valueBytes} 字节。`)
          break
        }
      }
    }

    let combinations = 1
    for (const key of mihomoCombinationKeys) {
      const count = resourceValueCount(rule, key)
      if (count > 0) combinations *= count
    }
    const portCount = resourceValueCount(rule, 'port')
    const sourcePortCount = resourceValueCount(rule, 'source_port')
    if (portCount > 0) combinations *= portCount
    if (sourcePortCount > 0) combinations *= sourcePortCount
    if (combinations > mihomoRouteResourceLimits.combinationsPerRule) {
      errors.push(`规则 #${index + 1} 的条件组合不能超过 ${mihomoRouteResourceLimits.combinationsPerRule} 条。`)
      continue
    }

    const selectedInbounds = resourceValues(rule?.inbound).filter((value) => typeof value === 'string' && value.trim() !== '').length
    renderedRules += combinations * (selectedInbounds > 0 ? selectedInbounds : inboundCount + 1)
    if (renderedRules > mihomoRouteResourceLimits.renderedRules) {
      errors.push(`当前配置展开后不能超过 ${mihomoRouteResourceLimits.renderedRules} 条 Mihomo 运行规则。`)
      break
    }
  }

  const serialized = JSON.stringify(config ?? { route: route ?? {} }, null, 2)
  if (resourceTextEncoder.encode(serialized).byteLength > mihomoRouteResourceLimits.configBytes) {
    errors.push(`Mihomo 路由配置不能超过 ${mihomoRouteResourceLimits.configBytes / 1024 / 1024} MiB。`)
  }
  return Array.from(new Set(errors))
}

export const validateSingboxRouteResourceBounds = (route: any): string[] => {
  const errors: string[] = []
  const encodedSize = (value: unknown): number => resourceTextEncoder.encode(JSON.stringify(value ?? null)).byteLength
  const rules = Array.isArray(route?.rules) ? route.rules : []
  const ruleSets = Array.isArray(route?.rule_set) ? route.rule_set : []
  if (encodedSize(route ?? {}) > singboxRouteResourceLimits.routeBytes) {
    errors.push(`sing-box 路由配置不能超过 ${singboxRouteResourceLimits.routeBytes / 1024 / 1024} MiB。`)
  }
  if (rules.length > singboxRouteResourceLimits.rules) {
    errors.push(`sing-box 路由规则最多允许 ${singboxRouteResourceLimits.rules} 条。`)
  }
  if (ruleSets.length > singboxRouteResourceLimits.ruleSets) {
    errors.push(`sing-box 规则集最多允许 ${singboxRouteResourceLimits.ruleSets} 条。`)
  }
  for (const [index, ruleSet] of ruleSets.entries()) {
    if (encodedSize(ruleSet) > singboxRouteResourceLimits.ruleSetBytes) {
      errors.push(`规则集 #${index + 1} 不能超过 ${singboxRouteResourceLimits.ruleSetBytes / 1024} KiB。`)
    }
  }

  let totalRules = 0
  let totalRuleBytes = 0
  const visitRules = (items: unknown, depth: number): void => {
    if (!Array.isArray(items)) return
    for (const [index, rule] of items.entries()) {
      totalRules += 1
      const ruleBytes = encodedSize(rule)
      totalRuleBytes += ruleBytes
      if (ruleBytes > singboxRouteResourceLimits.ruleBytes) {
        errors.push(`规则 #${index + 1} 不能超过 ${singboxRouteResourceLimits.ruleBytes / 1024} KiB。`)
      }
      if (depth > singboxRouteResourceLimits.logicalDepth) {
        errors.push(`logical 规则嵌套不能超过 ${singboxRouteResourceLimits.logicalDepth} 层。`)
      }
      const children = (rule as any)?.rules
      if (children !== undefined) {
        if (!Array.isArray(children) || children.length === 0 || children.length > singboxRouteResourceLimits.logicalChildren) {
          errors.push(`logical 规则的子规则数量必须在 1 到 ${singboxRouteResourceLimits.logicalChildren} 之间。`)
        }
        visitRules(children, depth + 1)
      }
    }
  }
  visitRules(rules, 1)
  if (totalRules > singboxRouteResourceLimits.rules) {
    errors.push(`sing-box 路由规则总数最多允许 ${singboxRouteResourceLimits.rules} 条。`)
  }
  if (totalRuleBytes > singboxRouteResourceLimits.rulesBytes) {
    errors.push(`sing-box 路由规则总大小不能超过 ${singboxRouteResourceLimits.rulesBytes / 1024} KiB。`)
  }
  return Array.from(new Set(errors))
}
