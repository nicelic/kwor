// Comment cleaned to avoid mojibake.

import { push } from 'notivue'
import { i18n } from '@/locales'
import {
  defaultLog,
  defaultTunInbound,
  defaultInb,
  defaultExp,
  defaultSubClashApi,
  defaultDns,
  domainIpTypes,
  tunIpOptions,
  RULE_SET_URL_TEMPLATES,
  METACUBEX_NAME_MAP,
  SOURCES_NEED_NAME_MAP,
} from './SubJsonExtConstants'
import {
  buildManagedCustomDomainKeywordDnsRules,
  collectManagedCustomDomainKeywordDnsRuleKeysFromRules,
  mergeManagedCustomDomainKeywordDnsRuleKeys,
  normalizeManagedCustomDomainKeywordDnsRuleKeys,
  stripManagedCustomDomainKeywordDnsRules,
} from './SubJsonExtCustomDnsPlugin'

const tlsStoreValues = ['system', 'mozilla', 'chrome', 'none']
const QUIXOTICHEART_GITHUB_SOURCE = 'quixoticheart_github'
const SINGBOX_ALLOWED_RULE_SET_EXTENSIONS = new Set(['.srs', '.json'])

// Comment cleaned to avoid mojibake.

/**
 * Comment cleaned.
 */
function normalizeName(input: string): string {
  let name = input.trim().toLowerCase()
  if (name.startsWith('geosite-')) name = name.substring(8)
  if (name.startsWith('geoip-')) name = name.substring(6)
  return name
}

function normalizeTlsStore(input: any): string {
  if (typeof input !== 'string') return ''
  const store = input.trim().toLowerCase()
  return tlsStoreValues.includes(store) ? store : ''
}

function isHttpRuleSetInput(input: string): boolean {
  const value = typeof input === 'string' ? input.trim().toLowerCase() : ''
  return value.startsWith('http://') || value.startsWith('https://')
}

function decodeUriComponentSafely(input: string): string {
  try {
    return decodeURIComponent(input)
  } catch {
    return input
  }
}

function extractUrlPathname(input: string): string {
  const raw = typeof input === 'string' ? input.trim() : ''
  if (!raw) return ''

  try {
    return new URL(raw).pathname || ''
  } catch {
    const withoutHash = raw.split('#', 1)[0]
    return withoutHash.split('?', 1)[0]
  }
}

function getRuleSetUrlExtension(input: string): string {
  const path = decodeUriComponentSafely(extractUrlPathname(input)).trim().toLowerCase()
  if (!path) return ''
  const idx = path.lastIndexOf('.')
  if (idx < 0 || idx === path.length - 1) return ''
  return path.substring(idx)
}

function isSupportedSingBoxRuleSetUrl(input: string): boolean {
  const ext = getRuleSetUrlExtension(input)
  return SINGBOX_ALLOWED_RULE_SET_EXTENSIONS.has(ext)
}

function stripTrailingFileExtension(input: string): string {
  const value = typeof input === 'string' ? input.trim() : ''
  if (!value) return ''

  const idx = value.lastIndexOf('.')
  if (idx <= 0 || idx === value.length - 1) return value
  return value.substring(0, idx)
}

function extractRuleSetNameFromUrl(input: string): string {
  const raw = typeof input === 'string' ? input.trim() : ''
  if (!raw) return ''

  let fileName = ''

  try {
    const parsed = new URL(raw)
    const segments = (parsed.pathname || '').split('/').filter(Boolean)
    fileName = segments.length > 0 ? segments[segments.length - 1] : ''
  } catch {
    const withoutHash = raw.split('#', 1)[0]
    const withoutQuery = withoutHash.split('?', 1)[0]
    const idx = withoutQuery.lastIndexOf('/')
    fileName = idx >= 0 ? withoutQuery.substring(idx + 1) : withoutQuery
  }

  const decodedFileName = decodeUriComponentSafely(fileName).trim()
  if (!decodedFileName) return ''

  const withoutExt = stripTrailingFileExtension(decodedFileName)
  return normalizeName(withoutExt)
}

function getSingBoxRuleSetFormat(url: string): 'binary' | 'source' {
  const ext = getRuleSetUrlExtension(url)
  if (ext === '.json') return 'source'
  return 'binary'
}

/**
 * Comment cleaned.
 */
function resolveTemplateNamesForRuleSetSource(
  source: string,
  prefix: 'geosite' | 'geoip',
  cleanName: string
): string[] {
  let baseName = cleanName
  if (SOURCES_NEED_NAME_MAP.includes(source)) {
    baseName = METACUBEX_NAME_MAP[cleanName] || cleanName
  }

  if (source !== QUIXOTICHEART_GITHUB_SOURCE || prefix !== 'geoip') {
    return [baseName]
  }

  // QuixoticHeart singbox geoip:
  // - Country/region-style short names are usually "{name}cidr.srs".
  // - Service-style names are usually "{name}.srs".
  // Keep a second candidate for validation/auto-match fallback.
  if (baseName.endsWith('cidr')) return [baseName]
  const preferCidr = /^[a-z]{2}$/.test(baseName)
  const candidates = preferCidr ? [`${baseName}cidr`, baseName] : [baseName, `${baseName}cidr`]
  return Array.from(new Set(candidates))
}

function buildRuleSetUrlCandidates(source: string, prefix: 'geosite' | 'geoip', name: string): string[] {
  const rawName = typeof name === 'string' ? name.trim() : ''
  if (!rawName) return []

  if (isHttpRuleSetInput(rawName)) {
    return [rawName]
  }

  const cleanName = normalizeName(rawName)
  if (!cleanName) return []

  const templates = RULE_SET_URL_TEMPLATES[source]
  if (!templates) return []

  const template = prefix === 'geosite' ? templates.geosite : templates.geoip
  const names = resolveTemplateNamesForRuleSetSource(source, prefix, cleanName)
  return Array.from(
    new Set(
      names
        .map((item) => template.replace('{name}', item))
        .filter((url) => typeof url === 'string' && url.trim().length > 0)
    )
  )
}

/**
 * Comment cleaned.
 */
function buildRuleSetUrl(source: string, prefix: 'geosite' | 'geoip', name: string): string {
  const candidates = buildRuleSetUrlCandidates(source, prefix, name)
  return candidates.length > 0 ? candidates[0] : ''
}

function normalizeRuleSetSourceSelection(input: any): string {
  if (typeof input !== 'string') return ''
  const source = input.trim()
  if (!source) return ''
  return RULE_SET_URL_TEMPLATES[source] ? source : ''
}

function normalizeRuleSetSourceOverride(input: any): string | null {
  if (input == null) return null
  if (typeof input !== 'string') return null
  const source = input.trim()
  if (source === '') return ''
  return RULE_SET_URL_TEMPLATES[source] ? source : null
}

function getRuleSetSourceCacheKey(source: string): string {
  return source || '__custom_url__'
}

function sanitizeAutoMatchedRuleSetUrls(input: any): Record<string, any> {
  const current = input && typeof input === 'object' && !Array.isArray(input)
    ? input
    : {}
  const next: Record<string, any> = {}

  for (const [key, value] of Object.entries(current)) {
    if (typeof key !== 'string') continue
    if (!key.startsWith('override:')) {
      next[key] = value
      continue
    }

    const parts = key.split(':')
    const expectedSourceKey = parts.length >= 2 ? parts[1] : ''
    if (!expectedSourceKey) continue

    const actualSource = normalizeRuleSetSourceOverride((value as any)?.source)
    if (actualSource == null) continue
    if (getRuleSetSourceCacheKey(actualSource) !== expectedSourceKey) continue

    next[key] = value
  }

  return next
}

function getRuleSetAutoMatchKey(
  prefix: 'geosite' | 'geoip',
  rawName: string,
  source: string,
  sourceBinding: 'global' | 'override'
): string {
  const cleanName = normalizeName(rawName)
  if (!cleanName) return ''
  return `${sourceBinding}:${getRuleSetSourceCacheKey(source)}:${prefix}:${cleanName}`
}

/**
 * Build one rule_set download entry.
 */
function buildRuleSetEntry(
  source: string,
  prefix: 'geosite' | 'geoip',
  name: string,
  detour: string,
  interval: string,
  overrideUrl?: string
): any {
  const rawName = typeof name === 'string' ? name.trim() : ''
  if (!rawName) return null

  const forcedUrl = typeof overrideUrl === 'string' ? overrideUrl.trim() : ''
  const url = forcedUrl || buildRuleSetUrl(source, prefix, rawName)
  if (!url) return null
  if (!isSupportedSingBoxRuleSetUrl(url)) return null

  const fromUrl = isHttpRuleSetInput(rawName)
  const cleanName = fromUrl ? extractRuleSetNameFromUrl(rawName) : normalizeName(rawName)
  if (!cleanName) return null

  const tagBase = fromUrl ? cleanName : `${prefix}-${cleanName}`
  const identity = fromUrl
    ? `url:${url}`
    : forcedUrl
      ? `resolved:${prefix}:${cleanName}:${url}`
      : `template:${source}:${prefix}:${cleanName}`
  const entry: any = {
    tag: tagBase,
    type: 'remote',
    format: getSingBoxRuleSetFormat(url),
    url,
    download_detour: detour,
  }

  if (interval) {
    entry.update_interval = interval
  }

  return {
    identity,
    tagBase,
    entry,
  }
}

function createUniqueRuleSetTag(baseTag: string, usedTags: Set<string>): string {
  if (!usedTags.has(baseTag)) {
    usedTags.add(baseTag)
    return baseTag
  }

  let index = 1
  while (usedTags.has(`${baseTag}-${index}`)) {
    index++
  }

  const next = `${baseTag}-${index}`
  usedTags.add(next)
  return next
}

function buildRuleSetPayload(
  detour: string,
  interval: string,
  groups: Array<{
    key: string
    names: string[]
    prefix: 'geosite' | 'geoip'
    source: string
    sourceBinding: 'global' | 'override'
  }>,
  resolveUrl?: (
    source: string,
    prefix: 'geosite' | 'geoip',
    rawName: string,
    sourceBinding: 'global' | 'override'
  ) => string
): { ruleSetMap: Map<string, any>; tagsByGroup: Record<string, string[]> } {
  const ruleSetMap = new Map<string, any>()
  const tagsByGroup: Record<string, string[]> = {}
  const usedTags = new Set<string>()

  for (const { key, names, prefix, source, sourceBinding } of groups) {
    const tags: string[] = []
    const seenInGroup = new Set<string>()
    for (const rawName of names || []) {
      const overrideUrl = resolveUrl ? resolveUrl(source, prefix, rawName, sourceBinding) : ''
      const built = buildRuleSetEntry(source, prefix, rawName, detour, interval, overrideUrl)
      if (!built) continue

      const tag = createUniqueRuleSetTag(built.tagBase, usedTags)
      built.entry.tag = tag
      ruleSetMap.set(tag, built.entry)

      if (!seenInGroup.has(tag)) {
        seenInGroup.add(tag)
        tags.push(tag)
      }
    }
    tagsByGroup[key] = tags
  }

  return { ruleSetMap, tagsByGroup }
}

/**
 * Supported formats: "123456789", "1,2,3,4,5,6,7,8,9", "1.2.3.4.5.6.7.8.9"
 */
function parseRouteOrder(orderStr: string): number[] {
  const defaultOrder = [1, 2, 3, 4, 5, 6, 7, 8, 9]

  if (!orderStr || orderStr.trim() === '') return defaultOrder

  const cleaned = orderStr.trim()
  let digits: number[]

  if (cleaned.includes(',')) {
    digits = cleaned.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n) && n >= 1 && n <= 9)
  } else if (cleaned.includes('.')) {
    digits = cleaned.split('.').map(s => parseInt(s.trim())).filter(n => !isNaN(n) && n >= 1 && n <= 9)
  } else {
    digits = cleaned.split('').map(c => parseInt(c)).filter(n => !isNaN(n) && n >= 1 && n <= 9)
  }

  if (digits.length === 0) return defaultOrder

  // Check whether it is the default order.
  if (JSON.stringify(digits) === JSON.stringify(defaultOrder)) return defaultOrder

  // Comment cleaned to avoid mojibake.
  for (let i = 1; i <= 9; i++) {
    if (!digits.includes(i)) {
      digits.push(i)
    }
  }

  return digits
}

const subSelectorTagValues = ['节点选择', '自动选择', '全球直连', '全球拦截', '漏网之鱼'] as const
type SubSelectorTag = typeof subSelectorTagValues[number]

const legacySubSelectorTagMap: Record<string, SubSelectorTag> = {
  '🚀 节点选择': '节点选择',
  '🚀节点选择': '节点选择',
  '\\U0001F680 节点选择': '节点选择',
  '\\U0001F680节点选择': '节点选择',
  '🎈 自动选择': '自动选择',
  '🎈自动选择': '自动选择',
  '\\U0001F388 自动选择': '自动选择',
  '\\U0001F388自动选择': '自动选择',
  '🎯 全球直连': '全球直连',
  '🎯全球直连': '全球直连',
  '\\U0001F3AF 全球直连': '全球直连',
  '\\U0001F3AF全球直连': '全球直连',
  '🛑 全球拦截': '全球拦截',
  '🛑全球拦截': '全球拦截',
  '\\U0001F6D1 全球拦截': '全球拦截',
  '\\U0001F6D1全球拦截': '全球拦截',
  '🐟 漏网之鱼': '漏网之鱼',
  '🐟漏网之鱼': '漏网之鱼',
  '\\U0001F41F 漏网之鱼': '漏网之鱼',
  '\\U0001F41F漏网之鱼': '漏网之鱼',
}

function normalizeSelectorTagValue(
  input: any,
  fallback: SubSelectorTag,
  proxyFallback: SubSelectorTag
) : string {
  if (typeof input !== 'string') return fallback

  let trimmed = input.trim()
  if (!trimmed) return fallback

  trimmed = legacySubSelectorTagMap[trimmed] || trimmed
  if ((subSelectorTagValues as readonly string[]).includes(trimmed)) {
    return trimmed as SubSelectorTag
  }
  if (trimmed === 'GLOBAL') return '节点选择'

  const lower = trimmed.toLowerCase()
  if (lower === 'proxy') return proxyFallback
  if (lower === 'auto') return '自动选择'
  if (lower === 'direct' || lower === 'global-direct') return '全球直连'
  if (lower === 'global-proxy' || lower === 'global') return '节点选择'
  if (lower === 'global-block' || lower === 'block' || lower === 'reject' || lower === 'reject-drop') return '全球拦截'
  if (lower === 'final') return '漏网之鱼'

  // Conversion failed: keep the original value as fallback.
  return trimmed
}

function normalizeUpdateMethodValue(input: any): string {
  return normalizeSelectorTagValue(input, '全球直连', '节点选择')
}

function normalizeRouteFinalValue(input: any): string {
  // Keep legacy "proxy" semantics: old "proxy" final mapped to "漏网之鱼".
  return normalizeSelectorTagValue(input, '漏网之鱼', '漏网之鱼')
}

type CustomRuleRoute = 'reject' | 'direct' | 'proxy'
type RuleRowKind = 'custom' | 'ruleset'
type RuleSetScope = 'domain' | 'ip'
type DnsRouteRowKind = 'rule-set' | 'query-type'
type RuleSetSourceBinding = 'global' | 'override'

type CustomRuleRow = {
  type: string
  route: CustomRuleRoute
  values: string[]
}

type RuleRow = {
  kind: RuleRowKind
  name: string
  customType: string
  ruleSetScope: RuleSetScope
  ruleSetSourceOverride: string | null
  route: CustomRuleRoute
  values: string[]
}

type DnsRouteRow = {
  kind: DnsRouteRowKind
  server: string
  ruleSet: string[]
}

const customRuleRouteValues: CustomRuleRoute[] = ['reject', 'direct', 'proxy']
const ruleRowKindValues: RuleRowKind[] = ['custom', 'ruleset']
const ruleSetScopeValues: RuleSetScope[] = ['domain', 'ip']
const dnsRouteRowKindValues: DnsRouteRowKind[] = ['rule-set', 'query-type']
const dnsQueryTypeValues = ['A', 'AAAA']

function normalizeCustomRuleType(input: any): string {
  const type = typeof input === 'string' ? input.trim() : ''
  if (!type) return 'domain'
  const exists = domainIpTypes.some((item: any) => item?.value === type)
  return exists ? type : 'domain'
}

function normalizeCustomRuleRoute(input: any): CustomRuleRoute {
  const route = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (customRuleRouteValues.includes(route as CustomRuleRoute)) {
    return route as CustomRuleRoute
  }
  return 'reject'
}

function normalizeCustomRuleValues(input: any): string[] {
  const list = Array.isArray(input) ? input : input != null ? [input] : []
  const result: string[] = []
  const seen = new Set<string>()
  for (const item of list) {
    if (typeof item !== 'string') continue
    const val = item.trim()
    if (!val || seen.has(val)) continue
    seen.add(val)
    result.push(val)
  }
  return result
}

function normalizeRuleRowKind(input: any): RuleRowKind {
  const kind = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (ruleRowKindValues.includes(kind as RuleRowKind)) {
    return kind as RuleRowKind
  }
  return 'custom'
}

function normalizeRuleSetScope(input: any): RuleSetScope {
  const scope = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (scope === 'geoip') return 'ip'
  if (scope === 'geosite') return 'domain'
  if (ruleSetScopeValues.includes(scope as RuleSetScope)) {
    return scope as RuleSetScope
  }
  return 'domain'
}

function normalizeRuleSelectorName(input: any): string {
  if (typeof input !== 'string') return ''
  return input.trim()
}

function normalizeRuleSelectorNameKey(input: any): string {
  return normalizeRuleSelectorName(input).toLowerCase()
}

type RuleNameConstraintIssue = {
  code: 'custom_conflicts_ruleset' | 'custom_type_mismatch'
  name: string
  currentType: string
  expectedType?: string
}

type RuleNameConstraintResult = {
  rows: RuleRow[]
  changed: boolean
  issues: RuleNameConstraintIssue[]
}

function applyRuleNameConstraints(rows: RuleRow[]): RuleNameConstraintResult {
  const normalizedRows = normalizeRuleRows(rows)
  const constrainedRows = normalizedRows.map((row: RuleRow) => ({
    ...row,
    values: [...row.values],
  }))

  const rulesetNameKeys = new Set<string>()
  for (const row of constrainedRows) {
    if (row.kind !== 'ruleset') continue
    const key = normalizeRuleSelectorNameKey(row.name)
    if (!key) continue
    rulesetNameKeys.add(key)
  }

  const customNameTypes = new Map<string, string>()
  const issues: RuleNameConstraintIssue[] = []
  let changed = false

  for (const row of constrainedRows) {
    if (row.kind !== 'custom') continue

    const name = normalizeRuleSelectorName(row.name)
    if (!name) continue

    const key = normalizeRuleSelectorNameKey(name)
    if (!key) continue

    if (rulesetNameKeys.has(key)) {
      issues.push({
        code: 'custom_conflicts_ruleset',
        name,
        currentType: row.customType,
      })
      row.name = ''
      changed = true
      continue
    }

    const existingType = customNameTypes.get(key)
    if (!existingType) {
      customNameTypes.set(key, row.customType)
      continue
    }

    if (existingType !== row.customType) {
      issues.push({
        code: 'custom_type_mismatch',
        name,
        currentType: row.customType,
        expectedType: existingType,
      })
      row.name = ''
      changed = true
    }
  }

  return { rows: constrainedRows, changed, issues }
}

function getRuleSetPrefixFromScope(scope: RuleSetScope): 'geosite' | 'geoip' {
  return scope === 'ip' ? 'geoip' : 'geosite'
}

function normalizeDnsRouteRowKind(input: any): DnsRouteRowKind {
  const kind = typeof input === 'string'
    ? input.trim().toLowerCase().replace(/_/g, '-')
    : ''
  if (dnsRouteRowKindValues.includes(kind as DnsRouteRowKind)) {
    return kind as DnsRouteRowKind
  }
  return 'rule-set'
}

function createDefaultDnsRouteRow(server: string = 'proxy-dns'): DnsRouteRow {
  const normalizedServer = typeof server === 'string' && server.trim() ? server.trim() : 'proxy-dns'
  return {
    kind: 'rule-set',
    server: normalizedServer,
    ruleSet: [],
  }
}

function createDefaultDnsQueryTypeRouteRow(server: string = 'proxy-dns'): DnsRouteRow {
  const normalizedServer = typeof server === 'string' && server.trim() ? server.trim() : 'proxy-dns'
  return {
    kind: 'query-type',
    server: normalizedServer,
    ruleSet: [],
  }
}

function normalizeDnsRouteRows(input: any, fakeipEnabled: boolean = false): DnsRouteRow[] {
  const rawRows = Array.isArray(input) ? input : []
  const rows: DnsRouteRow[] = []
  let hasQueryTypeRow = false

  for (const raw of rawRows) {
    const rawKind = typeof raw?.kind === 'string' ? raw.kind.trim().toLowerCase() : ''
    if (rawKind === 'fakeip') continue
    const kind = normalizeDnsRouteRowKind(rawKind)
    const rawServer = typeof raw?.server === 'string' && raw.server.trim() ? raw.server.trim() : 'proxy-dns'
    const server = rawServer === 'fakeip' && !fakeipEnabled ? 'proxy-dns' : rawServer
    if (kind === 'query-type') {
      if (hasQueryTypeRow) continue
      hasQueryTypeRow = true
      rows.push(createDefaultDnsQueryTypeRouteRow(server))
      continue
    }
    const ruleSet = normalizeRuleSetValues(raw?.ruleSet)
    rows.push({ kind, server, ruleSet })
  }

  if (!rows.some((row: DnsRouteRow) => row.kind === 'rule-set')) {
    rows.unshift(createDefaultDnsRouteRow())
  }

  return rows
}

type LegacyDnsRouteOrderKind = 'proxy' | 'direct'

function normalizeDnsRouteOrder(input: any, _fakeipEnabled?: boolean): LegacyDnsRouteOrderKind[] {
  const list = Array.isArray(input) ? input : []
  const result: LegacyDnsRouteOrderKind[] = []
  const seen = new Set<LegacyDnsRouteOrderKind>()
  const legacyKinds: LegacyDnsRouteOrderKind[] = ['proxy', 'direct']

  for (const item of list) {
    const value = typeof item === 'string' ? item.trim().toLowerCase() : ''
    if (!legacyKinds.includes(value as LegacyDnsRouteOrderKind)) continue
    const kind = value as LegacyDnsRouteOrderKind
    if (seen.has(kind)) continue
    seen.add(kind)
    result.push(kind)
  }

  for (const required of ['proxy', 'direct'] as LegacyDnsRouteOrderKind[]) {
    if (!seen.has(required)) {
      seen.add(required)
      result.push(required)
    }
  }

  return result
}

function buildDnsRouteRowsFromDnsRules(rules: any, fakeipEnabled: boolean): DnsRouteRow[] {
  const dnsRules = Array.isArray(rules) ? rules : []
  const rows: DnsRouteRow[] = []

  for (const rule of dnsRules) {
    if (isDnsRuleSetRouteRule(rule)) {
      const rawServer = typeof rule?.server === 'string' && rule.server.trim() ? rule.server.trim() : 'proxy-dns'
      const server = rawServer === 'fakeip' && !fakeipEnabled ? 'proxy-dns' : rawServer
      const ruleSet = normalizeRuleSetValues(rule?.rule_set)
      rows.push({ kind: 'rule-set', server, ruleSet })
      continue
    }
    if (isDnsQueryTypeRouteRule(rule)) {
      const rawServer = typeof rule?.server === 'string' && rule.server.trim() ? rule.server.trim() : 'proxy-dns'
      const server = rawServer === 'fakeip' && !fakeipEnabled ? 'proxy-dns' : rawServer
      rows.push(createDefaultDnsQueryTypeRouteRow(server))
    }
  }

  return normalizeDnsRouteRows(rows, fakeipEnabled)
}

function isManagedDnsRouteRule(rule: any): boolean {
  return isDnsRuleSetRouteRule(rule) || isDnsQueryTypeRouteRule(rule)
}

function buildManagedDnsRulesFromRows(rows: DnsRouteRow[]): any[] {
  const normalizedRows = normalizeDnsRouteRows(rows, true)
  const rules: any[] = []

  for (const row of normalizedRows) {
    if (row.kind === 'query-type') {
      rules.push({
        action: 'route',
        query_type: [...dnsQueryTypeValues],
        server: row.server || 'proxy-dns',
      })
      continue
    }
    const ruleSet = normalizeRuleSetValues(row.ruleSet)
    if (ruleSet.length === 0) continue
    rules.push({ action: 'route', rule_set: ruleSet, server: row.server || 'proxy-dns' })
  }

  return rules
}

function createDefaultCustomRuleRow(route: CustomRuleRoute = 'reject'): CustomRuleRow {
  return {
    type: 'domain',
    route,
    values: [],
  }
}

function normalizeCustomRuleRows(input: any): CustomRuleRow[] {
  const rawRows = Array.isArray(input) ? input : []
  const rows = rawRows.map((row: any) => ({
    type: normalizeCustomRuleType(row?.type),
    route: normalizeCustomRuleRoute(row?.route),
    values: normalizeCustomRuleValues(row?.values),
  }))
  if (rows.length === 0) {
    rows.push(createDefaultCustomRuleRow())
  }
  return rows
}

function filterNonEmptyCustomRuleRows(rows: CustomRuleRow[]): CustomRuleRow[] {
  return rows.filter((row: CustomRuleRow) => row.type === 'ip_is_private' || row.values.length > 0)
}

function createDefaultRuleRow(kind: RuleRowKind, route: CustomRuleRoute = 'reject'): RuleRow {
  return {
    kind,
    name: '',
    customType: 'domain',
    ruleSetScope: 'domain',
    ruleSetSourceOverride: null,
    route,
    values: [],
  }
}

function normalizeRuleRows(input: any): RuleRow[] {
  const rawRows = Array.isArray(input) ? input : []
  const rows = rawRows.map((raw: any) => {
    let inferredKind: RuleRowKind
    if (raw?.kind !== undefined) {
      inferredKind = normalizeRuleRowKind(raw.kind)
    } else if (raw?.ruleSetScope !== undefined || raw?.prefix !== undefined || raw?.scope !== undefined) {
      inferredKind = 'ruleset'
    } else {
      inferredKind = 'custom'
    }

    const customType = normalizeCustomRuleType(raw?.customType ?? raw?.type)
    const name = normalizeRuleSelectorName(raw?.name ?? raw?.selectorName)
    const ruleSetScope = normalizeRuleSetScope(raw?.ruleSetScope ?? raw?.scope ?? raw?.prefix)
    const ruleSetSourceOverride = normalizeRuleSetSourceOverride(raw?.ruleSetSourceOverride ?? raw?.rowRuleSetSource)
    const route = normalizeCustomRuleRoute(raw?.route)
    const values = normalizeCustomRuleValues(raw?.values)

    return {
      kind: inferredKind,
      name,
      customType,
      ruleSetScope,
      ruleSetSourceOverride: inferredKind === 'ruleset' ? ruleSetSourceOverride : null,
      route,
      values,
    }
  })

  if (rows.length === 0) {
    rows.push(createDefaultRuleRow('custom'))
  }

  return rows
}

function filterNonEmptyRuleRows(rows: RuleRow[]): RuleRow[] {
  return rows.filter((row) =>
    row.kind === 'custom'
      ? row.customType === 'ip_is_private' || row.values.length > 0
      : row.values.length > 0
  )
}

function buildLegacyCustomRuleRows(config: any): CustomRuleRow[] {
  const rows: CustomRuleRow[] = []

  const blockValues = normalizeCustomRuleValues(config?.customBlockValue)
  if (blockValues.length > 0) {
    rows.push({
      type: normalizeCustomRuleType(config?.customBlockType),
      route: 'reject',
      values: blockValues,
    })
  }

  const directValues = normalizeCustomRuleValues(config?.customDirectValue)
  if (directValues.length > 0) {
    rows.push({
      type: normalizeCustomRuleType(config?.customDirectType),
      route: 'direct',
      values: directValues,
    })
  }

  const proxyValues = normalizeCustomRuleValues(config?.customProxyValue)
  if (proxyValues.length > 0) {
    rows.push({
      type: normalizeCustomRuleType(config?.customProxyType),
      route: 'proxy',
      values: proxyValues,
    })
  }

  if (rows.length === 0) {
    rows.push(createDefaultCustomRuleRow())
  }

  return rows
}

function buildLegacyRuleRows(config: any): RuleRow[] {
  const rows: RuleRow[] = []

  const customRows = Array.isArray(config?.customRuleRows)
    ? normalizeCustomRuleRows(config.customRuleRows)
    : buildLegacyCustomRuleRows(config)

  for (const row of customRows) {
    rows.push({
      kind: 'custom',
      name: '',
      customType: row.type,
      ruleSetScope: 'domain',
      ruleSetSourceOverride: null,
      route: row.route,
      values: [...row.values],
    })
  }

  const legacyRuleSetRows: Array<{ values: any; route: CustomRuleRoute; scope: RuleSetScope }> = [
    { values: config?.blockRuleSet, route: 'reject', scope: 'domain' },
    { values: config?.blockRuleSetIp, route: 'reject', scope: 'ip' },
    { values: config?.proxyRuleSet, route: 'proxy', scope: 'domain' },
    { values: config?.proxyRuleSetIp, route: 'proxy', scope: 'ip' },
    { values: config?.directRuleSet, route: 'direct', scope: 'domain' },
    { values: config?.directRuleSetIp, route: 'direct', scope: 'ip' },
  ]

  for (const row of legacyRuleSetRows) {
    const values = normalizeCustomRuleValues(row.values)
    if (values.length === 0) continue
    rows.push({
      kind: 'ruleset',
      name: '',
      customType: 'domain',
      ruleSetScope: row.scope,
      ruleSetSourceOverride: null,
      route: row.route,
      values,
    })
  }

  return normalizeRuleRows(rows)
}

function getRuleSetTypeLabelForRow(route: CustomRuleRoute, scope: RuleSetScope): string {
  const routeLabelMap: Record<CustomRuleRoute, string> = {
    reject: 'Reject',
    direct: 'Direct',
    proxy: 'Proxy',
  }
  const scopeLabel = scope === 'ip' ? 'IP' : 'Domain'
  return `${routeLabelMap[route]} Ruleset (${scopeLabel})`
}

async function validateUrl(url: string): Promise<boolean> {
  if (!url) return false
  try {
    const response = await fetch(url, {
      method: 'HEAD',
      mode: 'cors',
      signal: AbortSignal.timeout(8000),
    })
    return response.ok
  } catch {
    try {
      const response = await fetch(url, {
        method: 'HEAD',
        mode: 'no-cors',
        signal: AbortSignal.timeout(8000),
      })
      return response.type === 'opaque'
    } catch {
      return false
    }
  }
}

function normalizeLatencyInput(input: any): string {
  return typeof input === 'string' ? input.trim() : ''
}

function getSingboxLatencyIntervalError(input: any): string {
  const value = normalizeLatencyInput(input)
  if (!value) return ''

  if (/^\d+$/.test(value)) {
    return '测试延迟间隔需要带单位（s/m/h/d），例如 3m 或 30s。'
  }

  if (!/^[1-9]\d*(s|m|h|d)$/i.test(value)) {
    return '测试延迟间隔格式无效，仅支持“正整数 + 单位（s/m/h/d）”，不支持 ms。'
  }

  return ''
}

function normalizeSingboxLatencyInterval(input: any): string {
  const value = normalizeLatencyInput(input)
  if (!value) return ''
  if (getSingboxLatencyIntervalError(value)) return ''
  return value.toLowerCase()
}

function getLatencyToleranceMsError(input: any): string {
  const value = normalizeLatencyInput(input)
  if (!value) return ''

  if (!/^[1-9]\d*$/.test(value)) {
    return '延迟容差单位为 ms；可留空，填写时仅输入数字（不要写 ms）。'
  }

  return ''
}

function parseLatencyToleranceMs(input: any): number | null {
  const value = normalizeLatencyInput(input)
  if (!value) return null
  if (getLatencyToleranceMsError(value)) return null

  const parsed = parseInt(value, 10)
  if (!Number.isFinite(parsed) || parsed <= 0) return null
  return parsed
}

function splitByDelimiters(input: string): string[] {
  return input
    .split(/[,\s\n\r]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
}

function autoSplitArrayItems(arr: string[]): string[] | null {
  let changed = false
  const result: string[] = []
  for (const item of arr) {
    if (/[,\s\n\r]/.test(item)) {
      const parts = splitByDelimiters(item)
      result.push(...parts)
      changed = true
    } else {
      result.push(item)
    }
  }
  if (!changed) return null
  return [...new Set(result)]
}

function isSameStringArray(a: string[], b: string[]): boolean {
  if (!Array.isArray(a) || !Array.isArray(b)) return false
  if (a.length !== b.length) return false
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false
  }
  return true
}

function deepCopy<T>(value: T): T {
  return JSON.parse(JSON.stringify(value))
}

function extractRuleSetValue(input: any): string {
  if (typeof input === 'string') return input.trim()
  if (!input || typeof input !== 'object') return ''
  if (typeof input.value === 'string') return input.value.trim()
  if (typeof input.tag === 'string') return input.tag.trim()
  if (typeof input.title === 'string') return input.title.trim()
  return ''
}

function normalizeRuleSetValues(input: any): string[] {
  const rawList = Array.isArray(input) ? input : input != null ? [input] : []
  const result: string[] = []
  const seen = new Set<string>()
  for (const item of rawList) {
    const value = extractRuleSetValue(item)
    if (!value || seen.has(value)) continue
    seen.add(value)
    result.push(value)
  }
  return result
}

function isDnsRuleSetRouteRule(rule: any): boolean {
  if (!rule || typeof rule !== 'object') return false
  if (rule.action !== 'route') return false
  if (typeof rule.server !== 'string' || !rule.server.trim()) return false
  return Object.prototype.hasOwnProperty.call(rule, 'rule_set')
}

function isDnsQueryTypeRouteRule(rule: any): boolean {
  if (!rule || typeof rule !== 'object') return false
  if (rule.action !== 'route') return false
  return Object.prototype.hasOwnProperty.call(rule, 'query_type')
}

function isTopSystemRouteRule(rule: any): boolean {
  if (!rule || typeof rule !== 'object') return false
  const action = typeof rule.action === 'string' ? rule.action.toLowerCase() : ''
  if (action === 'sniff') return true
  if (action === 'hijack-dns') return true
  if (action === 'reject') return true
  if (action !== 'route') return false
  const clashMode = typeof rule.clash_mode === 'string' ? rule.clash_mode.toLowerCase() : ''
  return clashMode === 'direct' || clashMode === 'global'
}

function isManagedFakeipFallbackRouteRule(rule: any): boolean {
  if (!rule || typeof rule !== 'object') return false
  const action = typeof rule.action === 'string' ? rule.action.toLowerCase() : ''
  if (action !== 'route') return false
  const outbound = typeof rule.outbound === 'string' ? rule.outbound.trim() : ''
  const outboundLower = outbound.toLowerCase()
  if (outboundLower !== 'proxy') return false
  const cidrs = normalizeCustomRuleValues(rule.ip_cidr)
  if (cidrs.length === 0) return false
  return true
}

function syncFakeipFallbackRouteRule(rules: any, enabled: boolean, cidrInput: any): any[] {
  const list = Array.isArray(rules) ? [...rules] : []
  let insertAt = 0
  while (insertAt < list.length && isTopSystemRouteRule(list[insertAt])) {
    insertAt++
  }
  if (insertAt < list.length && isManagedFakeipFallbackRouteRule(list[insertAt])) {
    list.splice(insertAt, 1)
  }

  if (!enabled) return list

  const cidrs = normalizeCustomRuleValues(cidrInput)
  if (cidrs.length === 0) return list

  const fallbackRule = {
    action: 'route',
    ip_cidr: cidrs,
    outbound: 'proxy',
  }
  list.splice(insertAt, 0, fallbackRule)
  return list
}

function findDnsRuleSetRouteRuleIndexByServer(rules: any[], server: string): number {
  if (!server) return -1
  return rules.findIndex((r: any) => isDnsRuleSetRouteRule(r) && r.server === server)
}

function findDnsRuleSetRouteRuleByServer(rules: any[], server: string): any | undefined {
  if (!server) return undefined
  return rules.find((r: any) => isDnsRuleSetRouteRule(r) && r.server === server)
}

function findDnsRuleSetRouteRuleIndexByServerAndRuleSet(rules: any[], server: string, expectedRuleSet: string[]): number {
  if (!server) return -1
  const normalizedExpected = normalizeRuleSetValues(expectedRuleSet)
  if (normalizedExpected.length === 0) return -1
  return rules.findIndex((rule: any) => {
    if (!isDnsRuleSetRouteRule(rule) || rule.server !== server) return false
    const current = normalizeRuleSetValues(rule.rule_set)
    return isSameStringArray(current, normalizedExpected)
  })
}

function isDeprecatedDnsClashModeRule(rule: any): boolean {
  if (!rule || typeof rule !== 'object') return false
  const action = typeof rule.action === 'string' ? rule.action.toLowerCase() : ''
  const clashMode = typeof rule.clash_mode === 'string' ? rule.clash_mode.toLowerCase() : ''
  if (action !== 'route') return false
  return clashMode === 'global' || clashMode === 'direct'
}

function reorderDnsRules(rules: any[], proxyServer: string, directServer: string, routeOrderInput?: any): any[] {
  if (!Array.isArray(rules) || rules.length === 0) return rules
  const proxyTag = typeof proxyServer === 'string' ? proxyServer.trim() : ''
  const directTag = typeof directServer === 'string' ? directServer.trim() : ''
  const others: any[] = []
  let proxyRule: any | undefined
  let directRule: any | undefined

  for (const rule of rules) {
    if (proxyTag && isDnsRuleSetRouteRule(rule) && rule.server === proxyTag && !proxyRule) {
      proxyRule = rule
      continue
    }
    if (directTag && directTag !== proxyTag && isDnsRuleSetRouteRule(rule) && rule.server === directTag && !directRule) {
      directRule = rule
      continue
    }
    others.push(rule)
  }

  const routeOrder = normalizeDnsRouteOrder(routeOrderInput)
  const ordered: any[] = [...others]
  for (const kind of routeOrder) {
    if (kind === 'proxy' && proxyRule) ordered.push(proxyRule)
    if (kind === 'direct' && directRule) ordered.push(directRule)
  }
  return ordered
}

function normalizeSubJsonDns(parsed: any) {
  const dns = parsed?.dns
  if (!dns || typeof dns !== 'object') return

  if (!Array.isArray(dns.servers)) dns.servers = []
  if (!Array.isArray(dns.rules)) dns.rules = []
  if (typeof dns.final !== 'string' || !dns.final.trim()) dns.final = 'direct-dns'

  dns.rules = dns.rules.filter((r: any) => !isDeprecatedDnsClashModeRule(r))

  for (const rule of dns.rules) {
    if (!rule || typeof rule !== 'object') continue
    if (!Object.prototype.hasOwnProperty.call(rule, 'rule_set')) continue
    const normalized = normalizeRuleSetValues(rule.rule_set)
    if (normalized.length > 0) {
      rule.rule_set = normalized
    } else {
      delete rule.rule_set
    }
  }

  const fakeipEnabled = dns.servers.some((server: any) => server?.type === 'fakeip')
  if (!fakeipEnabled && dns.final === 'fakeip') {
    dns.final = 'direct-dns'
  }
  const uiRows = parsed?._uiConfig?.dnsRouteRows
  if (Array.isArray(uiRows)) {
    const normalizedRows = normalizeDnsRouteRows(uiRows, fakeipEnabled)
    const managedRuleSetRules = buildManagedDnsRulesFromRows(normalizedRows)
    const managedCustomDns = buildManagedCustomDomainKeywordDnsRules(
      parsed?._uiConfig?.ruleRows,
      fakeipEnabled
    )
    const existingManagedCustomRuleKeys = collectManagedCustomDomainKeywordDnsRuleKeysFromRules(dns.rules)
    const previousManagedCustomRuleKeys = normalizeManagedCustomDomainKeywordDnsRuleKeys(
      mergeManagedCustomDomainKeywordDnsRuleKeys(
        parsed?._uiConfig?.customDomainDnsRuleKeys,
        parsed?._uiConfig?.customDomainKeywordDnsRuleKeys
      )
    )
    const managedCustomRuleKeys = mergeManagedCustomDomainKeywordDnsRuleKeys(
      mergeManagedCustomDomainKeywordDnsRuleKeys(
        previousManagedCustomRuleKeys,
        existingManagedCustomRuleKeys
      ),
      managedCustomDns.ruleKeys
    )
    const nonManagedRules = stripManagedCustomDomainKeywordDnsRules(
      dns.rules.filter((rule: any) => !isManagedDnsRouteRule(rule)),
      managedCustomRuleKeys
    )
    dns.rules = [...managedCustomDns.rules, ...managedRuleSetRules, ...nonManagedRules]
    if (parsed?._uiConfig && typeof parsed._uiConfig === 'object') {
      parsed._uiConfig.customDomainDnsRuleKeys = [...managedCustomDns.ruleKeys]
      parsed._uiConfig.customDomainKeywordDnsRuleKeys = [...managedCustomDns.ruleKeys]
    }
    return
  }

  const legacyProxyServerTag = typeof parsed?._uiConfig?.dnsToProxyServer === 'string' && parsed._uiConfig.dnsToProxyServer.trim()
    ? parsed._uiConfig.dnsToProxyServer.trim()
    : 'proxy-dns'
  const legacyDirectServerTag = typeof parsed?._uiConfig?.dnsToDirectServer === 'string' && parsed._uiConfig.dnsToDirectServer.trim()
    ? parsed._uiConfig.dnsToDirectServer.trim()
    : 'direct-dns'
  const routeOrder = parsed?._uiConfig?.dnsRouteOrder

  dns.rules = reorderDnsRules(dns.rules, legacyProxyServerTag, legacyDirectServerTag, routeOrder)
}

function normalizeDefaultDomainResolver(parsed: any) {
  if (!parsed || typeof parsed !== 'object') return
  if (!parsed.dns || typeof parsed.dns !== 'object') return
  const resolver = typeof parsed.default_domain_resolver === 'string'
    ? parsed.default_domain_resolver.trim()
    : ''
  if (!resolver) {
    parsed.default_domain_resolver = 'direct-dns'
  }
}

function normalizeTunPlatform(parsed: any) {
  const tunInb = parsed?.inbounds?.find((i: any) => i?.type === 'tun')
  if (!tunInb || typeof tunInb !== 'object') return
  if (!tunInb.platform || typeof tunInb.platform !== 'object') return

  const hp = tunInb.platform.http_proxy
  if (hp && typeof hp === 'object' && hp.enabled !== true) {
    delete tunInb.platform.http_proxy
  }

  if (Object.keys(tunInb.platform).length === 0) {
    delete tunInb.platform
  }
}

function normalizeSubJsonClashApiDetour(parsed: any) {
  const clashApi = parsed?.experimental?.clash_api
  if (!clashApi || typeof clashApi !== 'object') return

  if (!Object.prototype.hasOwnProperty.call(clashApi, 'external_ui_download_detour')) return
  const raw = clashApi.external_ui_download_detour
  if (typeof raw !== 'string') return

  const normalized = normalizeSelectorTagValue(raw, '全球直连', '节点选择')
  if (normalized) {
    clashApi.external_ui_download_detour = normalized
  } else {
    delete clashApi.external_ui_download_detour
  }
}

// Debounce timers for validation warnings.
const validationTimers: Record<string, ReturnType<typeof setTimeout>> = {}

export const SubJsonExtMixin = {
  created(this: any) {
    this.captureRuleRowsValidationSnapshot(this.ruleRows)
  },
  watch: {
    'settings.subJsonExt': {
      handler(this: any, v: string) {
        if (!v) {
          this.subJsonExt = {}
          return
        }
        try {
          const parsed = JSON.parse(v)
          if (parsed._uiConfig && typeof parsed._uiConfig === 'object' && parsed._uiConfig.routeFinal !== undefined) {
            parsed._uiConfig.routeFinal = normalizeRouteFinalValue(parsed._uiConfig.routeFinal)
          }
          if (parsed.route_final !== undefined) {
            parsed.route_final = normalizeRouteFinalValue(parsed.route_final)
          } else if (parsed._uiConfig?.routeFinal !== undefined) {
            parsed.route_final = normalizeRouteFinalValue(parsed._uiConfig.routeFinal)
          }
          normalizeSubJsonDns(parsed)
          normalizeDefaultDomainResolver(parsed)
          normalizeTunPlatform(parsed)
          normalizeSubJsonClashApiDetour(parsed)
          if (JSON.stringify(parsed) !== JSON.stringify(this.subJsonExt)) {
            this.subJsonExt = parsed
          }
        } catch {
          // invalid json, ignore
        }
      },
      immediate: true,
    },
    subJsonExt: {
      handler(this: any, v: any) {
        const str =
          Object.keys(v).length === 0
            ? ''
            : JSON.stringify(
                v,
                (_key: string, value: any) => {
                  if (value && typeof value === 'object' && !Array.isArray(value)) {
                    return Object.keys(value)
                      .sort()
                      .reduce((sorted: any, k: string) => {
                        sorted[k] = value[k]
                        return sorted
                      }, {} as any)
                  }
                  return value
                },
                2
              )
        if (str !== this.settings.subJsonExt) {
          this.settings.subJsonExt = str
        }
      },
      deep: true,
    },

    'subJsonExt._uiConfig': {
      handler(this: any, config: any) {
        if (!config) return
        if (this._uiConfigLoaded) return
        this._suspendRuleRegeneration = true
        this._uiConfigLoaded = true

        try {
          if (config.ruleSetSource !== undefined) this.ruleSetSource = normalizeRuleSetSourceSelection(config.ruleSetSource)
          const restoredRuleRows = Array.isArray(config.ruleRows)
            ? normalizeRuleRows(config.ruleRows)
            : buildLegacyRuleRows(config)
          this.ruleRows = restoredRuleRows
          this.captureRuleRowsValidationSnapshot(restoredRuleRows)
          if (config.updateMethod !== undefined) this.updateMethod = normalizeUpdateMethodValue(config.updateMethod)
          if (config.updateInterval !== undefined) this.updateInterval = config.updateInterval
          if (config.routeFinal !== undefined) this.routeFinal = normalizeRouteFinalValue(config.routeFinal)
          if (config.latencyTestUrl !== undefined) this.latencyTestUrl = config.latencyTestUrl
          if (config.latencyTestInterval !== undefined) this.latencyTestInterval = config.latencyTestInterval
          if (config.latencyTolerance !== undefined) this.latencyTolerance = config.latencyTolerance
          if (config.enableSniff !== undefined) this.enableSniff = config.enableSniff
          if (config.enableHijackDns !== undefined) this.enableHijackDns = config.enableHijackDns
          if (config.enableRejectQuic !== undefined) this.enableRejectQuic = config.enableRejectQuic === true
          if (config.enableReject443Udp !== undefined) this.enableReject443Udp = config.enableReject443Udp === true
          const restoredDnsRouteRows = Array.isArray(config.dnsRouteRows)
            ? normalizeDnsRouteRows(config.dnsRouteRows, this.enableFakeip === true)
            : buildDnsRouteRowsFromDnsRules(this.subJsonExt?.dns?.rules, this.enableFakeip === true)
          this.dnsRouteRows = restoredDnsRouteRows
          if (config.autoMatchedRuleSetUrls && typeof config.autoMatchedRuleSetUrls === 'object') {
            this.autoMatchedRuleSetUrls = sanitizeAutoMatchedRuleSetUrls(config.autoMatchedRuleSetUrls)
          }
        } finally {
          this._suspendRuleRegeneration = false
        }
        this.regenerateRuleConfig()
      },
      immediate: true,
    },

    ruleSetSource(this: any) {
      this.onRuleSetSourceChanged()
      this.regenerateRuleConfig()
    },
    ruleRows: {
      handler(this: any, rows: any[], oldRows: any[]) {
        if (this._suspendRuleRegeneration) return
        const previousRowsForValidation = this.getPreviousRuleRowsForValidation(oldRows)

        const normalizedRows = normalizeRuleRows(rows)
        const withSplitRows = normalizedRows.map((row: RuleRow) => {
          const split = autoSplitArrayItems(row.values)
          if (split) {
            return { ...row, values: split }
          }
          return row
        })

        const constrained = this.applyRuleNameConstraints(withSplitRows)
        const finalRows = constrained.rows

        if (JSON.stringify(rows) !== JSON.stringify(finalRows)) {
          this.captureRuleRowsValidationSnapshot(finalRows)
          this.ruleRows = finalRows
          this.showRuleNameConstraintWarnings(constrained.issues)
          return
        }

        this.regenerateRuleConfig()
        this.validateNewRuleRowEntries(finalRows, previousRowsForValidation)
        this.captureRuleRowsValidationSnapshot(finalRows)
      },
      deep: true,
    },
    dnsRouteRows: {
      handler(this: any, rows: any[]) {
        if (this._suspendRuleRegeneration) return

        const normalizedRows = normalizeDnsRouteRows(rows, this.enableFakeip === true)
        const withSplitRows = normalizedRows.map((row: DnsRouteRow) => {
          if (row.kind !== 'rule-set') return row
          const split = autoSplitArrayItems(row.ruleSet)
          if (split) {
            return { ...row, ruleSet: split }
          }
          return row
        })

        if (JSON.stringify(rows) !== JSON.stringify(withSplitRows)) {
          this.dnsRouteRows = withSplitRows
          return
        }

        this.regenerateRuleConfig()
      },
      deep: true,
    },
    updateMethod(this: any) { this.regenerateRuleConfig() },
    updateInterval(this: any) { this.regenerateRuleConfig() },
    routeFinal(this: any) { this.regenerateRuleConfig() },
    enableSniff(this: any) { this.regenerateRuleConfig() },
    enableHijackDns(this: any) { this.regenerateRuleConfig() },
    enableRejectQuic(this: any) { this.regenerateRuleConfig() },
    enableReject443Udp(this: any) { this.regenerateRuleConfig() },
    latencyTestUrl(this: any) { this.regenerateRuleConfig() },
    latencyTestInterval(this: any) { this.regenerateRuleConfig() },
    latencyTolerance(this: any) { this.regenerateRuleConfig() },

  },
  methods: {
    normalizeTunIpSelection(this: any, input: any): string[] {
      const normalized = normalizeCustomRuleValues(input)
      const ipv4 = normalized.find((ip: string) => !ip.includes(':'))
      const ipv6 = normalized.find((ip: string) => ip.includes(':'))
      const result: string[] = []
      if (ipv4) result.push(ipv4)
      if (ipv6) result.push(ipv6)
      return result
    },
    syncFakeipServerWithSelection(this: any, selectionInput: any): boolean {
      if (!this.subJsonExt?.dns) return false

      const dns = this.subJsonExt.dns
      if (!Array.isArray(dns.servers)) dns.servers = []

      const selection = this.normalizeTunIpSelection(selectionInput)
      this.subJsonExt._uiConfig = this.subJsonExt._uiConfig || {}
      this.subJsonExt._uiConfig.tunIp = [...selection]

      const fakeipIndex = dns.servers.findIndex((server: any) => server?.type === 'fakeip')
      if (selection.length === 0) {
        if (fakeipIndex >= 0) {
          dns.servers.splice(fakeipIndex, 1)
        }
        if (dns.final === 'fakeip') {
          dns.final = 'direct-dns'
        }
        this.dnsRouteRows = this.getNormalizedDnsRouteRows().map((row: DnsRouteRow) => {
          if (row.server === 'fakeip') {
            return { ...row, server: 'proxy-dns' }
          }
          return row
        })
        if (this.subJsonExt.experimental?.cache_file) {
          delete this.subJsonExt.experimental.cache_file.store_fakeip
        }
        return false
      }

      const ipv4 = selection.find((ip: string) => !ip.includes(':'))
      const ipv6 = selection.find((ip: string) => ip.includes(':'))
      const fakeip = fakeipIndex >= 0
        ? dns.servers[fakeipIndex]
        : { tag: 'fakeip', type: 'fakeip' }

      fakeip.tag = 'fakeip'
      fakeip.type = 'fakeip'
      if (ipv4) {
        fakeip.inet4_range = ipv4
      } else {
        delete fakeip.inet4_range
      }
      if (ipv6) {
        fakeip.inet6_range = ipv6
      } else {
        delete fakeip.inet6_range
      }

      if (fakeipIndex < 0) {
        dns.servers.push(fakeip)
      }
      if (this.subJsonExt.experimental?.cache_file) {
        this.subJsonExt.experimental.cache_file.store_fakeip = true
      }
      return true
    },
    getPreviousRuleRowsForValidation(this: any, oldRows: any[]): RuleRow[] {
      const snapshot = this._ruleRowsValidationSnapshot
      if (Array.isArray(snapshot) && snapshot.length > 0) {
        return normalizeRuleRows(snapshot)
      }
      return normalizeRuleRows(oldRows)
    },
    captureRuleRowsValidationSnapshot(this: any, rows: any) {
      this._ruleRowsValidationSnapshot = deepCopy(normalizeRuleRows(rows))
    },
    openEditor(this: any) {
      this.enableEditor = true
    },
    resetSubJsonPage(this: any) {
      const dataFactory = this.$options?.data
      const defaults = typeof dataFactory === 'function' ? deepCopy(dataFactory.call(this)) : null

      this._suspendRuleRegeneration = true
      if (defaults && typeof defaults === 'object') {
        for (const [key, value] of Object.entries(defaults)) {
          this[key] = value
        }
      }

      this.subJsonExt = {}
      this.settings.subJsonExt = ''
      this._uiConfigLoaded = false
      this.captureRuleRowsValidationSnapshot(this.ruleRows)

      this.$nextTick(() => {
        this._suspendRuleRegeneration = false
      })
    },
    saveEditor(this: any, data: string) {
      try {
        const result = JSON.parse(data)
        if (typeof result !== 'object' || Array.isArray(result)) {
          push.warning({
            title: i18n.global.t('error'),
            message: i18n.global.t('setting.jsonNotObj'),
            duration: 3000,
          })
          return
        }
        normalizeSubJsonDns(result)
        normalizeDefaultDomainResolver(result)
        normalizeTunPlatform(result)
        const fakeipEnabled = Array.isArray(result?.dns?.servers) &&
          result.dns.servers.some((s: any) => s?.type === 'fakeip')
        this.dnsRouteRows = buildDnsRouteRowsFromDnsRules(result?.dns?.rules, fakeipEnabled)
        this.subJsonExt = result
        this._uiConfigLoaded = false
        this.updateJson()
        this.enableEditor = false
      } catch (e) {
        push.warning({
          title: i18n.global.t('error'),
          message: i18n.global.t('setting.jsonParseErr'),
          duration: 3000,
        })
      }
    },
    getTypeLabel(this: any, type: string): string {
      const found = domainIpTypes.find((t) => t.value === type)
      return found ? found.title : type
    },
    getRuleSetScopeLabel(this: any, scope: string): string {
      return scope === 'ip' ? 'IP 规则集' : '域名规则集'
    },
    applyRuleNameConstraints(this: any, rows: RuleRow[]): RuleNameConstraintResult {
      return applyRuleNameConstraints(normalizeRuleRows(rows))
    },
    showRuleNameConstraintWarnings(this: any, issues: RuleNameConstraintIssue[]) {
      if (!Array.isArray(issues) || issues.length === 0) return

      const seen = new Set<string>()
      for (const issue of issues) {
        const name = normalizeRuleSelectorName(issue?.name)
        if (!name) continue

        const key = `${issue.code}:${name.toLowerCase()}:${issue.currentType}:${issue.expectedType || ''}`
        if (seen.has(key)) continue
        seen.add(key)

        const message = issue.code === 'custom_conflicts_ruleset'
          ? `规则集名称冲突：自定义匹配名称“${name}”与规则集名称重复，已自动清空该名称。`
          : `匹配类型不一致：自定义匹配名称“${name}”仅允许同匹配类型合并（当前应为 ${issue.expectedType || '同类型'}），已自动清空该名称。`

        push.warning({
          title: '名称已自动修正',
          message,
          duration: 5000,
        })
      }
    },
    mapNamedSelectorDefaultOutbound(this: any): string {
      return 'proxy'
    },
    getRowRouteTarget(this: any, row: RuleRow): string {
      const selectorName = normalizeRuleSelectorName((row as any)?.name)
      if (selectorName) return selectorName
      return normalizeCustomRuleRoute(row.route)
    },
    buildNamedSelectorGroups(this: any, rows: RuleRow[]): Array<{ tag: string; default_outbound: string }> {
      const result: Array<{ tag: string; default_outbound: string }> = []
      const seen = new Set<string>()

      for (const row of rows) {
        const tag = normalizeRuleSelectorName((row as any)?.name)
        if (!tag || seen.has(tag)) continue
        seen.add(tag)
        result.push({
          tag,
          default_outbound: this.mapNamedSelectorDefaultOutbound(),
        })
      }

      return result
    },
    canDeleteRuleRow(this: any, index: number): boolean {
      const rows = normalizeRuleRows(this.ruleRows)
      return index >= 0 && index < rows.length && rows.length > 1
    },
    insertRuleRow(this: any, index: number) {
      const rows = normalizeRuleRows(this.ruleRows)
      const safeIndex = Number.isInteger(index)
        ? Math.max(-1, Math.min(index, rows.length - 1))
        : rows.length - 1
      const current = rows[safeIndex] ?? createDefaultRuleRow('custom')
      rows.splice(safeIndex + 1, 0, createDefaultRuleRow(current.kind, current.route))
      this.ruleRows = rows
    },
    removeRuleRow(this: any, index: number) {
      const rows = normalizeRuleRows(this.ruleRows)
      if (index < 0 || index >= rows.length || rows.length <= 1) return
      rows.splice(index, 1)
      this.ruleRows = normalizeRuleRows(rows)
    },
    moveRuleRow(this: any, index: number, delta: number) {
      if (!Number.isInteger(index) || !Number.isInteger(delta) || delta === 0) return
      const rows = normalizeRuleRows(this.ruleRows)
      const target = index + delta
      if (index < 0 || index >= rows.length || target < 0 || target >= rows.length) return
      const [current] = rows.splice(index, 1)
      rows.splice(target, 0, current)
      this.ruleRows = rows
    },
    commitCustomRuleRows(this: any) {
      const normalizedRows = normalizeRuleRows(this.ruleRows)
      const persistedRows = filterNonEmptyRuleRows(normalizedRows)
      this.ruleRows = normalizeRuleRows(persistedRows)
      this.regenerateRuleConfig()
    },
    getNonEmptyRuleRows(this: any): RuleRow[] {
      return filterNonEmptyRuleRows(normalizeRuleRows(this.ruleRows))
    },
    getNormalizedDnsRouteRows(this: any): DnsRouteRow[] {
      return normalizeDnsRouteRows(this.dnsRouteRows, this.enableFakeip === true)
    },
    getDnsRouteRowsForUiConfig(this: any): DnsRouteRow[] {
      const normalizedRows = this.getNormalizedDnsRouteRows()
      const persistedRows = normalizedRows.filter((row: DnsRouteRow) => (
        row.kind === 'query-type' ||
        normalizeRuleSetValues(row.ruleSet).length > 0
      ))
      return normalizeDnsRouteRows(persistedRows, this.enableFakeip === true)
    },
    applyDnsRouteRowsToDnsRules(this: any) {
      if (!this.subJsonExt?.dns) return
      if (!Array.isArray(this.subJsonExt.dns.rules)) this.subJsonExt.dns.rules = []

      const normalizedRows = this.getNormalizedDnsRouteRows()
      const managedRuleSetRules = buildManagedDnsRulesFromRows(normalizedRows)
      const managedCustomDns = buildManagedCustomDomainKeywordDnsRules(
        this.getNonEmptyRuleRows(),
        this.enableFakeip === true
      )
      const existingManagedCustomRuleKeys = collectManagedCustomDomainKeywordDnsRuleKeysFromRules(
        this.subJsonExt.dns.rules
      )
      const previousManagedCustomRuleKeys = normalizeManagedCustomDomainKeywordDnsRuleKeys(
        mergeManagedCustomDomainKeywordDnsRuleKeys(
          this.subJsonExt?._uiConfig?.customDomainDnsRuleKeys,
          this.subJsonExt?._uiConfig?.customDomainKeywordDnsRuleKeys
        )
      )
      const managedCustomRuleKeys = mergeManagedCustomDomainKeywordDnsRuleKeys(
        mergeManagedCustomDomainKeywordDnsRuleKeys(
          previousManagedCustomRuleKeys,
          existingManagedCustomRuleKeys
        ),
        managedCustomDns.ruleKeys
      )

      if (!this.subJsonExt._uiConfig || typeof this.subJsonExt._uiConfig !== 'object') {
        this.subJsonExt._uiConfig = {}
      }
      this.subJsonExt._uiConfig.customDomainDnsRuleKeys = [...managedCustomDns.ruleKeys]
      this.subJsonExt._uiConfig.customDomainKeywordDnsRuleKeys = [...managedCustomDns.ruleKeys]

      const nonManagedRules = stripManagedCustomDomainKeywordDnsRules(
        this.subJsonExt.dns.rules.filter((rule: any) => !isManagedDnsRouteRule(rule)),
        managedCustomRuleKeys
      )
      this.subJsonExt.dns.rules = [...managedCustomDns.rules, ...managedRuleSetRules, ...nonManagedRules]
    },
    canDeleteDnsRouteRow(this: any, index: number): boolean {
      const rows = this.getNormalizedDnsRouteRows()
      if (index < 0 || index >= rows.length) return false
      if (rows[index]?.kind !== 'rule-set') return false
      const ruleSetRowCount = rows.filter((row: DnsRouteRow) => row.kind === 'rule-set').length
      return ruleSetRowCount > 1
    },
    insertDnsRouteRow(this: any, index: number) {
      const rows = this.getNormalizedDnsRouteRows()
      const safeIndex = Number.isInteger(index)
        ? Math.max(-1, Math.min(index, rows.length - 1))
        : rows.length - 1
      rows.splice(safeIndex + 1, 0, createDefaultDnsRouteRow())
      this.dnsRouteRows = normalizeDnsRouteRows(rows, this.enableFakeip === true)
    },
    removeDnsRouteRow(this: any, index: number) {
      const rows = this.getNormalizedDnsRouteRows()
      if (index < 0 || index >= rows.length) return
      if (rows[index]?.kind !== 'rule-set') return
      const ruleSetRowCount = rows.filter((row: DnsRouteRow) => row.kind === 'rule-set').length
      if (ruleSetRowCount <= 1) return
      rows.splice(index, 1)
      this.dnsRouteRows = normalizeDnsRouteRows(rows, this.enableFakeip === true)
    },
    moveDnsRouteRow(this: any, index: number, delta: number) {
      if (!Number.isInteger(index) || !Number.isInteger(delta) || delta === 0) return
      const rows = this.getNormalizedDnsRouteRows()
      const target = index + delta
      if (index < 0 || index >= rows.length || target < 0 || target >= rows.length) return
      const [current] = rows.splice(index, 1)
      rows.splice(target, 0, current)
      this.dnsRouteRows = normalizeDnsRouteRows(rows, this.enableFakeip === true)
    },
    commitDnsRouteRows(this: any) {
      const rows = this.getDnsRouteRowsForUiConfig()
      this.dnsRouteRows = rows
      this.applyDnsRouteRowsToDnsRules()
      this.updateJson()
    },
    onRuleSetSourceChanged(this: any) {
      if (this._suspendRuleRegeneration) return
      this.autoMatchRunToken = (this.autoMatchRunToken || 0) + 1
      this.clearAutoMatchedRuleSetUrlsForGlobalSourceRows()
      this.autoMatchAllRuleSetEntries(true)
    },
    clearAutoMatchedRuleSetUrls(this: any) {
      this.autoMatchedRuleSetUrls = {}
    },
    clearAutoMatchedRuleSetUrlsForGlobalSourceRows(this: any) {
      const current = this.autoMatchedRuleSetUrls && typeof this.autoMatchedRuleSetUrls === 'object'
        ? this.autoMatchedRuleSetUrls
        : {}
      const next: Record<string, any> = {}
      let changed = false

      for (const [key, value] of Object.entries(current)) {
        if (typeof key === 'string' && key.startsWith('global:')) {
          changed = true
          continue
        }
        next[key] = value
      }

      if (changed) {
        this.autoMatchedRuleSetUrls = next
      }
    },
    getRuleSetSourceOrderForFallback(this: any): string[] {
      const options = Array.isArray(this.ruleSetSourceOptions) ? this.ruleSetSourceOptions : []
      return options
        .map((item: any) => (typeof item?.value === 'string' ? item.value : ''))
        .filter((value: string) => value.length > 0 && Boolean(RULE_SET_URL_TEMPLATES[value]))
        .filter((value: string, idx: number, arr: string[]) => arr.indexOf(value) === idx)
    },
    getCurrentRuleSetSource(this: any): string {
      return normalizeRuleSetSourceSelection(this.ruleSetSource)
    },
    getRuleSetSourceTitle(this: any, source: string): string {
      const options = Array.isArray(this.ruleSetSourceOptions) ? this.ruleSetSourceOptions : []
      const found = options.find((item: any) => item?.value === source)
      return typeof found?.title === 'string' && found.title.trim() ? found.title : source
    },
    getRuleSetResolveContextForRow(this: any, row: RuleRow): { source: string; sourceBinding: RuleSetSourceBinding } {
      const rowSource = normalizeRuleSetSourceOverride((row as any)?.ruleSetSourceOverride)
      if (rowSource == null) {
        return {
          source: this.getCurrentRuleSetSource(),
          sourceBinding: 'global',
        }
      }
      return {
        source: rowSource,
        sourceBinding: 'override',
      }
    },
    getResolvedRuleSetUrl(
      this: any,
      prefix: 'geosite' | 'geoip',
      rawName: string,
      source: string,
      sourceBinding: RuleSetSourceBinding
    ): string {
      if (isHttpRuleSetInput(rawName)) return ''
      const key = getRuleSetAutoMatchKey(prefix, rawName, source, sourceBinding)
      if (!key) return ''
      const matched = this.autoMatchedRuleSetUrls?.[key]
      if (sourceBinding === 'override') {
        const matchedSource = normalizeRuleSetSourceOverride(matched?.source)
        if (matchedSource == null) return ''
        if (getRuleSetSourceCacheKey(matchedSource) !== getRuleSetSourceCacheKey(source)) return ''
      }
      return typeof matched?.url === 'string' ? matched.url : ''
    },
    autoMatchAllRuleSetEntries(this: any, onlyGlobalRows: boolean = false) {
      const rows = this.getNonEmptyRuleRows().filter((row: RuleRow) => row.kind === 'ruleset')
      for (const row of rows) {
        const sourceContext = this.getRuleSetResolveContextForRow(row)
        if (onlyGlobalRows && sourceContext.sourceBinding !== 'global') continue
        const prefix = getRuleSetPrefixFromScope(row.ruleSetScope)
        const typeLabel = getRuleSetTypeLabelForRow(row.route, row.ruleSetScope)
        for (const rawName of row.values) {
          this.scheduleAutoMatchForEntry(rawName, prefix, typeLabel, sourceContext)
        }
      }
    },
    scheduleAutoMatchForEntry(
      this: any,
      rawName: string,
      prefix: 'geosite' | 'geoip',
      typeLabel: string,
      sourceContext: { source: string; sourceBinding: RuleSetSourceBinding }
    ) {
      const fromUrl = isHttpRuleSetInput(rawName)
      const cleanName = fromUrl ? extractRuleSetNameFromUrl(rawName) : normalizeName(rawName)
      if (!cleanName) return
      const source = normalizeRuleSetSourceSelection(sourceContext?.source)
      const sourceBinding: RuleSetSourceBinding = sourceContext?.sourceBinding === 'override' ? 'override' : 'global'
      const timerKey = fromUrl
        ? `auto-match:${sourceBinding}:${prefix}:url:${rawName.trim()}`
        : `auto-match:${sourceBinding}:${getRuleSetSourceCacheKey(source)}:${prefix}:${cleanName}`
      if (validationTimers[timerKey]) {
        clearTimeout(validationTimers[timerKey])
      }
      validationTimers[timerKey] = setTimeout(async () => {
        await this.tryAutoMatchRuleSetEntry(rawName, prefix, typeLabel, { source, sourceBinding })
        delete validationTimers[timerKey]
      }, 500)
    },
    async tryAutoMatchRuleSetEntry(
      this: any,
      rawName: string,
      prefix: 'geosite' | 'geoip',
      typeLabel: string,
      sourceContext: { source: string; sourceBinding: RuleSetSourceBinding }
    ) {
      const fromUrl = isHttpRuleSetInput(rawName)
      const cleanName = fromUrl ? extractRuleSetNameFromUrl(rawName) : normalizeName(rawName)
      if (!cleanName) return

      const token = this.autoMatchRunToken || 0
      const currentSource = normalizeRuleSetSourceSelection(sourceContext?.source)
      const sourceBinding: RuleSetSourceBinding = sourceContext?.sourceBinding === 'override' ? 'override' : 'global'
      const allowFallback = sourceBinding === 'global'
      const noFallbackMessage = (sourceValue: string) => sourceValue
        ? `${typeLabel} ${cleanName}：当前来源 ${this.getRuleSetSourceTitle(sourceValue)} 检测失败，不进行回退`
        : `${typeLabel} ${cleanName}：当前所选规则集来源检测失败，不进行回退`

      if (fromUrl) {
        const url = rawName.trim()
        if (!url) return
        if (!isSupportedSingBoxRuleSetUrl(url)) {
          push.error({
            title: 'Rule-set validation',
            message: `${typeLabel} ${cleanName}: unsupported file extension. JSON subscription only supports .json or .srs`,
            duration: 5000,
          })
          return
        }
        const isValid = await validateUrl(url)
        if (token !== (this.autoMatchRunToken || 0)) return
        if (isValid) {
          push.success({
            title: '规则集校验',
            message: `${typeLabel} ${cleanName}：链接可用`,
            duration: 3000,
          })
        } else {
          push.warning({
            title: '规则集校验',
            message: `${typeLabel} ${cleanName}：链接不可用，不进行回退`,
            duration: 3000,
          })
        }
        return
      }

      const key = getRuleSetAutoMatchKey(prefix, cleanName, currentSource, sourceBinding)
      if (!key) return

      const sourceOrder = this.getRuleSetSourceOrderForFallback()
      let matchedSource = ''
      let matchedUrl = ''
      let notifiedNoFallback = false

      if (currentSource) {
        const currentUrls = buildRuleSetUrlCandidates(currentSource, prefix, cleanName)
        for (const url of currentUrls) {
          if (!isSupportedSingBoxRuleSetUrl(url)) continue
          const isValid = await validateUrl(url)
          if (token !== (this.autoMatchRunToken || 0)) return
          if (isValid) {
            matchedSource = currentSource
            matchedUrl = url
            break
          }
        }
        if (matchedUrl) {
          push.success({
            title: '规则集校验',
            message: `${typeLabel} ${cleanName}：当前来源 ${this.getRuleSetSourceTitle(currentSource)} 可用`,
            duration: 3000,
          })
        } else {
          push.warning({
            title: '规则集校验',
            message: allowFallback
              ? `${typeLabel} ${cleanName}：当前来源 ${this.getRuleSetSourceTitle(currentSource)} 检测失败，开始回退`
              : noFallbackMessage(currentSource),
            duration: 3000,
          })
          notifiedNoFallback = !allowFallback
        }
      }

      const fallbackSources = sourceOrder.filter((source: string) => source !== currentSource)
      if (!matchedUrl && allowFallback) {
        for (const source of fallbackSources) {
          const urls = buildRuleSetUrlCandidates(source, prefix, cleanName)
          for (const url of urls) {
            if (!isSupportedSingBoxRuleSetUrl(url)) continue
            const isValid = await validateUrl(url)
            if (token !== (this.autoMatchRunToken || 0)) return
            if (isValid) {
              matchedSource = source
              matchedUrl = url
              break
            }
          }
          if (matchedUrl) break
        }
      }

      if (!matchedUrl) {
        const cached = this.autoMatchedRuleSetUrls?.[key]
        if (cached) {
          const next = { ...this.autoMatchedRuleSetUrls }
          delete next[key]
          this.autoMatchedRuleSetUrls = next
          this.regenerateRuleConfig()
        }
        if (allowFallback || !notifiedNoFallback) {
          push.warning({
            title: '规则集校验',
            message: allowFallback
              ? `${typeLabel} ${cleanName}：未找到可用来源`
              : noFallbackMessage(currentSource),
            duration: 3000,
          })
        }
        return
      }

      if (allowFallback && (!currentSource || matchedSource !== currentSource)) {
        push.success({
          title: '规则集校验',
          message: `${typeLabel} ${cleanName}：已回退到 ${this.getRuleSetSourceTitle(matchedSource)}`,
          duration: 3000,
        })
      }

      const current = this.autoMatchedRuleSetUrls?.[key]
      if (current?.url === matchedUrl && current?.source === matchedSource) return

      this.autoMatchedRuleSetUrls = {
        ...(this.autoMatchedRuleSetUrls || {}),
        [key]: { url: matchedUrl, source: matchedSource },
      }
      this.regenerateRuleConfig()
    },
    updateJson(this: any) {
      if (this.subJsonExt?.route_final !== undefined) {
        this.subJsonExt.route_final = normalizeRouteFinalValue(this.subJsonExt.route_final)
      }
      if (this.subJsonExt?._uiConfig?.routeFinal !== undefined) {
        this.subJsonExt._uiConfig.routeFinal = normalizeRouteFinalValue(this.subJsonExt._uiConfig.routeFinal)
      }
      if (this.subJsonExt?.dns) {
        normalizeSubJsonDns(this.subJsonExt)
        normalizeDefaultDomainResolver(this.subJsonExt)
        this.applyDnsRouteRowsToDnsRules()
      }
      if (Array.isArray(this.subJsonExt?.rules)) {
        this.subJsonExt.rules = syncFakeipFallbackRouteRule(
          this.subJsonExt.rules,
          this.enableFakeip === true,
          this.tunIp
        )
      }
      normalizeSubJsonClashApiDetour(this.subJsonExt)
      this.subJsonExt = { ...this.subJsonExt }
    },

    /**
     * Comment cleaned.
     */
    regenerateRuleConfig(this: any) {
      if (this._suspendRuleRegeneration) return
      if (!this.subJsonExt || typeof this.subJsonExt !== 'object') {
        this.subJsonExt = {}
      }
      const normalizedUpdateMethod = normalizeUpdateMethodValue(this.updateMethod)
      if (this.updateMethod !== normalizedUpdateMethod) {
        this.updateMethod = normalizedUpdateMethod
        return
      }
      const normalizedRouteFinal = normalizeRouteFinalValue(this.routeFinal)
      if (this.routeFinal !== normalizedRouteFinal) {
        this.routeFinal = normalizedRouteFinal
        return
      }

      const normalizedRows = normalizeRuleRows(this.ruleRows)
      if (JSON.stringify(this.ruleRows) !== JSON.stringify(normalizedRows)) {
        this.ruleRows = normalizedRows
        return
      }
      const normalizedDnsRouteRows = normalizeDnsRouteRows(this.dnsRouteRows, this.enableFakeip === true)
      if (JSON.stringify(this.dnsRouteRows) !== JSON.stringify(normalizedDnsRouteRows)) {
        this.dnsRouteRows = normalizedDnsRouteRows
        return
      }
      const constrained = this.applyRuleNameConstraints(normalizedRows)
      if (JSON.stringify(normalizedRows) !== JSON.stringify(constrained.rows)) {
        this.ruleRows = constrained.rows
        this.showRuleNameConstraintWarnings(constrained.issues)
        return
      }

      const persistedRows = filterNonEmptyRuleRows(constrained.rows)
      const persistedDnsRouteRows = this.getDnsRouteRowsForUiConfig()
      const namedSelectorGroups = this.buildNamedSelectorGroups(persistedRows)
      const persistedCustomRows: CustomRuleRow[] = persistedRows
        .filter((row: RuleRow) => row.kind === 'custom')
        .map((row: RuleRow) => ({
          type: row.customType,
          route: row.route,
          values: [...row.values],
        }))

      const legacyRuleSetBuckets = {
        blockRuleSet: [] as string[],
        blockRuleSetIp: [] as string[],
        proxyRuleSet: [] as string[],
        proxyRuleSetIp: [] as string[],
        directRuleSet: [] as string[],
        directRuleSetIp: [] as string[],
      }
      for (const row of persistedRows) {
        if (row.kind !== 'ruleset') continue
        if (row.route === 'reject' && row.ruleSetScope === 'domain') legacyRuleSetBuckets.blockRuleSet.push(...row.values)
        if (row.route === 'reject' && row.ruleSetScope === 'ip') legacyRuleSetBuckets.blockRuleSetIp.push(...row.values)
        if (row.route === 'proxy' && row.ruleSetScope === 'domain') legacyRuleSetBuckets.proxyRuleSet.push(...row.values)
        if (row.route === 'proxy' && row.ruleSetScope === 'ip') legacyRuleSetBuckets.proxyRuleSetIp.push(...row.values)
        if (row.route === 'direct' && row.ruleSetScope === 'domain') legacyRuleSetBuckets.directRuleSet.push(...row.values)
        if (row.route === 'direct' && row.ruleSetScope === 'ip') legacyRuleSetBuckets.directRuleSetIp.push(...row.values)
      }

      const sanitizedAutoMatchedRuleSetUrls = sanitizeAutoMatchedRuleSetUrls(this.autoMatchedRuleSetUrls)
      if (JSON.stringify(this.autoMatchedRuleSetUrls || {}) !== JSON.stringify(sanitizedAutoMatchedRuleSetUrls)) {
        this.autoMatchedRuleSetUrls = sanitizedAutoMatchedRuleSetUrls
      }
      const managedCustomDns = buildManagedCustomDomainKeywordDnsRules(
        persistedRows,
        this.enableFakeip === true
      )

      this.subJsonExt._uiConfig = {
        ruleSetSource: this.getCurrentRuleSetSource(),
        ruleRows: persistedRows.map((row: RuleRow) => ({
          kind: row.kind,
          name: row.name,
          customType: row.customType,
          ruleSetScope: row.ruleSetScope,
          ruleSetSourceOverride: row.ruleSetSourceOverride,
          route: row.route,
          values: [...row.values],
        })),
        customRuleRows: persistedCustomRows.map((row: CustomRuleRow) => ({
          type: row.type,
          route: row.route,
          values: [...row.values],
        })),
        blockRuleSet: [...legacyRuleSetBuckets.blockRuleSet],
        blockRuleSetIp: [...legacyRuleSetBuckets.blockRuleSetIp],
        proxyRuleSet: [...legacyRuleSetBuckets.proxyRuleSet],
        proxyRuleSetIp: [...legacyRuleSetBuckets.proxyRuleSetIp],
        directRuleSet: [...legacyRuleSetBuckets.directRuleSet],
        directRuleSetIp: [...legacyRuleSetBuckets.directRuleSetIp],
        updateMethod: normalizedUpdateMethod,
        updateInterval: this.updateInterval,
        routeFinal: normalizedRouteFinal,
        latencyTestUrl: this.latencyTestUrl,
        latencyTestInterval: this.latencyTestInterval,
        latencyTolerance: this.latencyTolerance,
        enableSniff: this.enableSniff,
        enableHijackDns: this.enableHijackDns,
        enableRejectQuic: this.enableRejectQuic,
        enableReject443Udp: this.enableReject443Udp,
        dnsRouteRows: persistedDnsRouteRows.map((row: DnsRouteRow) => ({
          kind: row.kind,
          server: row.server,
          ruleSet: [...row.ruleSet],
        })),
        customDomainDnsRuleKeys: [...managedCustomDns.ruleKeys],
        customDomainKeywordDnsRuleKeys: [...managedCustomDns.ruleKeys],
        tunIp: [...this.tunIp],
        autoMatchedRuleSetUrls: { ...sanitizedAutoMatchedRuleSetUrls },
      }

      const detour = normalizedUpdateMethod
      const interval = this.updateInterval

      const allRuleSetInputs = persistedRows
        .map((row: RuleRow, idx: number) => {
          if (row.kind !== 'ruleset') return null
          const sourceContext = this.getRuleSetResolveContextForRow(row)
          return {
            key: `row-${idx}`,
            names: row.values,
            prefix: getRuleSetPrefixFromScope(row.ruleSetScope),
            source: sourceContext.source,
            sourceBinding: sourceContext.sourceBinding,
          }
        })
        .filter((item): item is {
          key: string
          names: string[]
          prefix: 'geosite' | 'geoip'
          source: string
          sourceBinding: RuleSetSourceBinding
        } => item !== null)

      const resolveRuleSetUrl = (
        source: string,
        prefix: 'geosite' | 'geoip',
        rawName: string,
        sourceBinding: RuleSetSourceBinding
      ) => this.getResolvedRuleSetUrl(prefix, rawName, source, sourceBinding)
      const { ruleSetMap, tagsByGroup } = buildRuleSetPayload(detour, interval, allRuleSetInputs, resolveRuleSetUrl)

      const rules: any[] = []

      // System rules first.
      if (this.enableSniff) {
        rules.push({ action: 'sniff' })
      }
      if (this.enableRejectQuic) {
        rules.push({ protocol: ['quic'], action: 'reject' })
      }
      if (this.enableReject443Udp) {
        rules.push({ network: 'udp', port: 443, action: 'reject' })
      }
      rules.push({ action: 'route', clash_mode: 'direct', outbound: 'direct' })
      // Put global mode rule before custom/ruleset rules to avoid partial matches.
      rules.push({ action: 'route', clash_mode: 'global', outbound: 'global-proxy' })
      if (this.enableHijackDns) {
        rules.push({ action: 'hijack-dns', protocol: 'dns' })
      }

      // Follow UI order: custom row and ruleset row can be interleaved.
      for (let idx = 0; idx < persistedRows.length; idx++) {
        const row = persistedRows[idx]
        const routeTarget = this.getRowRouteTarget(row)
        if (row.kind === 'custom') {
          const built = this.buildCustomRule(row.customType, row.values, row.route, routeTarget !== normalizeCustomRuleRoute(row.route) ? routeTarget : '')
          if (built) {
            rules.push(built)
          }
          continue
        }

        const tags = tagsByGroup[`row-${idx}`] || []
        if (tags.length === 0) continue

        if (!normalizeRuleSelectorName(row.name) && row.route === 'reject') {
          rules.push({ action: 'reject', rule_set: tags })
        } else {
          rules.push({ action: 'route', outbound: routeTarget, rule_set: tags })
        }
      }

      // route_final controls the default outbound; no catch-all rule is added here.
      const finalOutbound = normalizedRouteFinal

      // 7. Write generated rule_set and rules to subJsonExt.
      if (ruleSetMap.size > 0) {
        this.subJsonExt.rule_set = Array.from(ruleSetMap.values())
      } else {
        delete this.subJsonExt.rule_set
      }

      this.subJsonExt.rules = rules
      if (namedSelectorGroups.length > 0) {
        this.subJsonExt.selector_groups = namedSelectorGroups
      } else {
        delete this.subJsonExt.selector_groups
      }

      // Expose route_final for backend use.
      this.subJsonExt.route_final = finalOutbound

      // Expose latency-test settings for backend use.
      if (this.latencyTestUrl) {
        this.subJsonExt.latency_test_url = this.latencyTestUrl
      } else {
        delete this.subJsonExt.latency_test_url
      }
      const normalizedLatencyInterval = normalizeSingboxLatencyInterval(this.latencyTestInterval)
      if (normalizedLatencyInterval) {
        this.subJsonExt.latency_test_interval = normalizedLatencyInterval
      } else {
        delete this.subJsonExt.latency_test_interval
      }
      const parsedLatencyTolerance = parseLatencyToleranceMs(this.latencyTolerance)
      if (parsedLatencyTolerance !== null) {
        this.subJsonExt.latency_tolerance = parsedLatencyTolerance
      } else {
        delete this.subJsonExt.latency_tolerance
      }

      this.syncDnsRouteRuleSetsWithRuleRows()
      this.updateJson()
    },

    /**
     * Comment cleaned.
     */
    buildCustomRule(this: any, type: string, values: string[], outbound: string, overrideOutbound: string = ''): any {
      const normalizedType = normalizeCustomRuleType(type)
      const normalizedOutbound = normalizeCustomRuleRoute(outbound)
      const forcedOutbound = normalizeRuleSelectorName(overrideOutbound)
      const normalizedValues = normalizeCustomRuleValues(values)
      if (normalizedType !== 'ip_is_private' && normalizedValues.length === 0) return null

      // For reject: use action: 'reject' directly (sing-box new format, no outbound field).
      // For proxy/direct: use action: 'route' with outbound field.
      const rule: any = (!forcedOutbound && normalizedOutbound === 'reject')
        ? { action: 'reject' }
        : { action: 'route', outbound: forcedOutbound || normalizedOutbound }
      if (normalizedType === 'ip_is_private') {
        rule.ip_is_private = true
      } else {
        rule[normalizedType] = [...normalizedValues]
      }
      return rule
    },

    /**
     * Comment cleaned.
     */
    validateRuleSetEntry(
      this: any,
      rawName: string,
      prefix: 'geosite' | 'geoip',
      typeLabel: string,
      sourceContext: { source: string; sourceBinding: RuleSetSourceBinding }
    ) {
      this.scheduleAutoMatchForEntry(rawName, prefix, typeLabel, sourceContext)
    },
    validateNewRuleRowEntries(this: any, newRows: any[], oldRows: any[]) {
      const normalizedNewRows = normalizeRuleRows(newRows)
      const normalizedOldRows = normalizeRuleRows(oldRows)
      const oldEntryCount = new Map<string, number>()
      const newEntryCount = new Map<string, number>()
      const newEntryMeta = new Map<string, {
        name: string
        prefix: 'geosite' | 'geoip'
        typeLabel: string
        sourceContext: { source: string; sourceBinding: RuleSetSourceBinding }
      }>()

      for (const row of normalizedOldRows) {
        if (row.kind !== 'ruleset') continue
        const prefix = getRuleSetPrefixFromScope(row.ruleSetScope)
        const sourceContext = this.getRuleSetResolveContextForRow(row)
        for (const rawName of row.values) {
          const name = typeof rawName === 'string' ? rawName.trim() : ''
          if (!name) continue
          const key = getRuleSetAutoMatchKey(prefix, name, sourceContext.source, sourceContext.sourceBinding)
          if (!key) continue
          oldEntryCount.set(key, (oldEntryCount.get(key) || 0) + 1)
        }
      }

      for (const row of normalizedNewRows) {
        if (row.kind !== 'ruleset') continue
        const prefix = getRuleSetPrefixFromScope(row.ruleSetScope)
        const typeLabel = getRuleSetTypeLabelForRow(row.route, row.ruleSetScope)
        const sourceContext = this.getRuleSetResolveContextForRow(row)
        for (const rawName of row.values) {
          const name = typeof rawName === 'string' ? rawName.trim() : ''
          if (!name) continue
          const key = getRuleSetAutoMatchKey(prefix, name, sourceContext.source, sourceContext.sourceBinding)
          if (!key) continue
          newEntryCount.set(key, (newEntryCount.get(key) || 0) + 1)
          if (!newEntryMeta.has(key)) {
            newEntryMeta.set(key, { name, prefix, typeLabel, sourceContext })
          }
        }
      }

      for (const [key, count] of newEntryCount.entries()) {
        const oldCount = oldEntryCount.get(key) || 0
        if (count <= oldCount) continue
        const meta = newEntryMeta.get(key)
        if (!meta) continue
        this.validateRuleSetEntry(meta.name, meta.prefix, meta.typeLabel, meta.sourceContext)
      }
    },
    syncDnsRouteRuleSetsWithRuleRows(this: any) {
      if (!this.subJsonExt?.dns) return

      const allowedTags = new Set((this.generatedRuleSetTags?.proxy || []) as string[])
      const normalizedRows = this.getNormalizedDnsRouteRows().map((row: DnsRouteRow) => {
        if (row.kind !== 'rule-set') return row
        const filteredRuleSet = normalizeRuleSetValues(row.ruleSet).filter((tag: string) => allowedTags.has(tag))
        return {
          ...row,
          ruleSet: filteredRuleSet,
        }
      })

      if (JSON.stringify(this.dnsRouteRows) !== JSON.stringify(normalizedRows)) {
        this.dnsRouteRows = normalizedRows
      }

      this.applyDnsRouteRowsToDnsRules()
    },

    /**
     * Comment cleaned.
     */
    showValidationWarning(this: any, typeLabel: string, name: string) {
      push.warning({
        title: '规则集校验失败',
        message: `${typeLabel} ${name} 无效`,
        duration: 5000,
      })
    },
  },

  computed: {
    latencyTestIntervalError(this: any): string {
      return getSingboxLatencyIntervalError(this.latencyTestInterval)
    },
    latencyToleranceError(this: any): string {
      return getLatencyToleranceMsError(this.latencyTolerance)
    },
    editorData(this: any): string {
      if (Object.keys(this.subJsonExt).length === 0) return ''
      // Comment cleaned to avoid mojibake.
      const hiddenKeys = new Set(['_uiConfig', 'route_final', 'latency_test_url', 'latency_test_interval', 'latency_tolerance', 'selector_groups'])
      const filtered: any = {}
      for (const key of Object.keys(this.subJsonExt)) {
        if (!hiddenKeys.has(key)) {
          filtered[key] = this.subJsonExt[key]
        }
      }
      return JSON.stringify(filtered, null, 2)
    },

    // Comment cleaned to avoid mojibake.
    enableServerTlsStore: {
      get(this: any): boolean {
        return this.settings?.serverTlsStoreEnabled === 'true'
      },
      set(this: any, v: boolean) {
        if (!this.settings) return
        this.settings.serverTlsStoreEnabled = v ? 'true' : 'false'
        if (v) {
          const normalized = normalizeTlsStore(this.settings.serverTlsStore)
          this.settings.serverTlsStore = normalized || 'chrome'
        }
      },
    },
    serverTlsStore: {
      get(this: any): string {
        const normalized = normalizeTlsStore(this.settings?.serverTlsStore)
        return normalized || 'chrome'
      },
      set(this: any, v: string) {
        if (!this.settings) return
        const normalized = normalizeTlsStore(v)
        this.settings.serverTlsStore = normalized || 'chrome'
      },
    },
    enableClientTlsStore: {
      get(this: any): boolean {
        return this.settings?.clientTlsStoreEnabled === 'true'
      },
      set(this: any, v: boolean) {
        if (!this.settings) return
        this.settings.clientTlsStoreEnabled = v ? 'true' : 'false'
        if (v) {
          const normalized = normalizeTlsStore(this.settings.clientTlsStore)
          this.settings.clientTlsStore = normalized || 'chrome'
        }
      },
    },
    clientTlsStore: {
      get(this: any): string {
        const normalized = normalizeTlsStore(this.settings?.clientTlsStore)
        return normalized || 'chrome'
      },
      set(this: any, v: string) {
        if (!this.settings) return
        const normalized = normalizeTlsStore(v)
        this.settings.clientTlsStore = normalized || 'chrome'
      },
    },
    enableLog: {
      get(this: any): boolean {
        return this.subJsonExt?.log !== undefined
      },
      set(this: any, v: boolean) {
        if (v) {
          this.subJsonExt.log = JSON.parse(JSON.stringify(defaultLog))
        } else {
          delete this.subJsonExt.log
        }
        this.updateJson()
      },
    },
    enableDns: {
      get(this: any): boolean {
        return this.subJsonExt?.dns !== undefined
      },
      set(this: any, v: boolean) {
        if (v) {
          this.subJsonExt.dns = JSON.parse(JSON.stringify(defaultDns))
        } else {
          delete this.subJsonExt.dns
        }
        this.updateJson()
      },
    },
    enableInb: {
      get(this: any): boolean {
        return this.subJsonExt?.inbounds !== undefined
      },
      set(this: any, v: boolean) {
        if (v) {
          this.subJsonExt.inbounds = JSON.parse(JSON.stringify(defaultInb))
        } else {
          delete this.subJsonExt.inbounds
        }
        this.updateJson()
      },
    },
    enableExp: {
      get(this: any): boolean {
        return this.subJsonExt?.experimental !== undefined
      },
      set(this: any, v: boolean) {
        if (v) {
          this.subJsonExt.experimental = JSON.parse(JSON.stringify(defaultExp))
        } else {
          delete this.subJsonExt.experimental
        }
        this.updateJson()
      },
    },
    enableSubClashApi: {
      get(this: any): boolean {
        return this.subJsonExt?.experimental?.clash_api !== undefined
      },
      set(this: any, v: boolean) {
        if (v) {
          if (!this.subJsonExt.experimental) {
            this.subJsonExt.experimental = {}
          }
          this.subJsonExt.experimental.clash_api = JSON.parse(JSON.stringify(defaultSubClashApi))
        } else if (this.subJsonExt?.experimental?.clash_api) {
          delete this.subJsonExt.experimental.clash_api
          if (Object.keys(this.subJsonExt.experimental).length === 0) {
            delete this.subJsonExt.experimental
          }
        }
        this.updateJson()
      },
    },
    subClashApiOrigin: {
      get(this: any): string {
        const list = this.subJsonExt?.experimental?.clash_api?.access_control_allow_origin
        return Array.isArray(list) ? list.join(",") : ""
      },
      set(this: any, v: string) {
        const clashApi = this.subJsonExt?.experimental?.clash_api
        if (!clashApi) return
        const parts = String(v || "")
          .split(",")
          .map((item: string) => item.trim())
          .filter((item: string) => item.length > 0)
        clashApi.access_control_allow_origin = parts.length > 0 ? parts : undefined
      },
    },

    enableDnsQueryType: {
      get(this: any): boolean {
        return this.getNormalizedDnsRouteRows().some((row: DnsRouteRow) => row.kind === 'query-type')
      },
      set(this: any, v: boolean) {
        const rows = this.getNormalizedDnsRouteRows()
        let nextRows = rows

        if (v) {
          const exists = rows.some((row: DnsRouteRow) => row.kind === 'query-type')
          if (exists) return
          nextRows = [...rows, createDefaultDnsQueryTypeRouteRow()]
        } else {
          nextRows = rows.filter((row: DnsRouteRow) => row.kind !== 'query-type')
          if (nextRows.length === rows.length) return
        }

        this.dnsRouteRows = normalizeDnsRouteRows(nextRows, this.enableFakeip === true)
        this.applyDnsRouteRowsToDnsRules()
        this.updateJson()
      },
    },

    // ===== FakeIP =====
    enableFakeip: {
      get(this: any): boolean {
        const servers = this.subJsonExt?.dns?.servers
        if (!servers || !Array.isArray(servers)) return false
        return servers.some((s: any) => s.type === 'fakeip')
      },
      set(this: any, v: boolean) {
        if (!this.subJsonExt?.dns) return
        if (v) {
          this.syncFakeipServerWithSelection(tunIpOptions)
        } else {
          this.syncFakeipServerWithSelection([])
        }
        this.applyDnsRouteRowsToDnsRules()
        this.updateJson()
      },
    },

    // Comment cleaned to avoid mojibake.
    dns: {
      get(this: any): any {
        return this.subJsonExt?.dns ?? {}
      },
      set(this: any, v: any) {
        this.subJsonExt.dns = v
      },
    },
    proxyDns(this: any): any {
      const servers = this.subJsonExt?.dns?.servers
      if (!servers || !Array.isArray(servers)) return {}
      return servers.find((s: any) => s.tag === 'proxy-dns') ?? {}
    },
    directDns(this: any): any {
      const servers = this.subJsonExt?.dns?.servers
      if (!servers || !Array.isArray(servers)) return {}
      return servers.find((s: any) => s.tag === 'direct-dns') ?? {}
    },
    localDns(this: any): any {
      const servers = this.subJsonExt?.dns?.servers
      if (!servers || !Array.isArray(servers)) return {}
      return servers.find((s: any) => s.tag === 'local-dns') ?? {}
    },
    dnsTags(this: any): string[] {
      const servers = this.subJsonExt?.dns?.servers
      if (!servers || !Array.isArray(servers)) return []
      return servers.map((s: any) => s.tag).filter(Boolean)
    },
    dnsFinalOptions(this: any): string[] {
      const fixed = ['proxy-dns', 'direct-dns', 'proxy-bootstrap-dns', 'direct-bootstrap-dns']
      if (this.enableFakeip) fixed.push('fakeip')
      const merged = [...fixed, ...this.dnsTags]
      return merged.filter((tag: string, idx: number) => tag && merged.indexOf(tag) === idx)
    },
    dnsRouteServerOptions(this: any): string[] {
      const options = this.dnsFinalOptions
      if (this.enableFakeip) return options
      return options.filter((item: string) => item !== 'fakeip')
    },
    final: {
      get(this: any): string {
        return this.subJsonExt?.dns?.final ?? 'direct-dns'
      },
      set(this: any, v: string) {
        if (this.subJsonExt?.dns) this.subJsonExt.dns.final = v
      },
    },
    generatedRuleSetTags(this: any): Record<CustomRuleRoute, string[]> {
      const rows = this.getNonEmptyRuleRows().filter(
        (row: RuleRow) => row.kind === 'ruleset' && row.ruleSetScope === 'domain'
      ) as RuleRow[]
      const detour = normalizeUpdateMethodValue(this.updateMethod)
      const allRuleSetInputs = rows.map((row: RuleRow, idx: number) => {
        const sourceContext = this.getRuleSetResolveContextForRow(row)
        return {
          key: `row-${idx}`,
          names: row.values,
          prefix: getRuleSetPrefixFromScope(row.ruleSetScope),
          source: sourceContext.source,
          sourceBinding: sourceContext.sourceBinding,
        }
      })
      const resolveRuleSetUrl = (
        source: string,
        prefix: 'geosite' | 'geoip',
        rawName: string,
        sourceBinding: RuleSetSourceBinding
      ) => this.getResolvedRuleSetUrl(prefix, rawName, source, sourceBinding)

      const { tagsByGroup } = buildRuleSetPayload(
        detour,
        this.updateInterval,
        allRuleSetInputs,
        resolveRuleSetUrl
      )
      const tagsByRoute: Record<CustomRuleRoute, string[]> = {
        reject: [],
        direct: [],
        proxy: [],
      }
      const allDomainTags: string[] = []

      for (let idx = 0; idx < rows.length; idx++) {
        const row = rows[idx]
        const tags = tagsByGroup[`row-${idx}`] || []
        for (const tag of tags) {
          if (!allDomainTags.includes(tag)) {
            allDomainTags.push(tag)
          }
          if (!tagsByRoute[row.route].includes(tag)) {
            tagsByRoute[row.route].push(tag)
          }
        }
      }

      // For DNS route UI: as long as it is ruleset + domain, expose all tags
      // to both "route to proxy dns" and "route to direct dns".
      tagsByRoute.proxy = [...allDomainTags]
      tagsByRoute.direct = [...allDomainTags]

      return tagsByRoute
    },
    dnsRouteRuleSetOptions(this: any): string[] {
      const generatedTags = (this.generatedRuleSetTags?.proxy || []) as string[]
      const currentTags = this.getNormalizedDnsRouteRows()
        .filter((row: DnsRouteRow) => row.kind === 'rule-set')
        .flatMap((row: DnsRouteRow) => row.ruleSet)
      return Array.from(new Set([...generatedTags, ...currentTags]))
    },
    dnsStrategy: {
      get(this: any): string {
        return this.subJsonExt?.dns?.strategy ?? 'prefer_ipv4'
      },
      set(this: any, v: string) {
        if (this.subJsonExt?.dns) this.subJsonExt.dns.strategy = v
      },
    },
    routeDefaultDomainResolver: {
      get(this: any): string {
        const resolver = this.subJsonExt?.default_domain_resolver
        return typeof resolver === 'string' && resolver.trim() ? resolver : 'direct-dns'
      },
      set(this: any, v: string) {
        if (!this.subJsonExt || typeof this.subJsonExt !== 'object') return
        const resolver = typeof v === 'string' && v.trim() ? v : 'direct-dns'
        this.subJsonExt.default_domain_resolver = resolver
        this.updateJson()
      },
    },

    // Comment cleaned to avoid mojibake.
    inbounds: {
      get(this: any): any[] {
        return this.subJsonExt?.inbounds ?? []
      },
      set(this: any, v: any[]) {
        this.subJsonExt.inbounds = v
      },
    },
    tunInbound(this: any): any {
      const inbs = this.subJsonExt?.inbounds ?? []
      return inbs.find((i: any) => i.type === 'tun') ?? {}
    },
    mixedInbound(this: any): any {
      const inbs = this.subJsonExt?.inbounds ?? []
      return inbs.find((i: any) => i.type === 'mixed') ?? {}
    },
    enableTun: {
      get(this: any): boolean {
        const inbs = this.subJsonExt?.inbounds ?? []
        return inbs.some((i: any) => i.type === 'tun')
      },
      set(this: any, v: boolean) {
        if (!this.subJsonExt?.inbounds) return
        if (v) {
          const tunEntry = JSON.parse(JSON.stringify(defaultTunInbound))
          this.subJsonExt.inbounds = [tunEntry, ...this.subJsonExt.inbounds]
        } else {
          this.subJsonExt.inbounds = this.subJsonExt.inbounds.filter(
            (i: any) => i.type !== 'tun'
          )
        }
      },
    },
    autoRoute: {
      get(this: any): boolean {
        return this.tunInbound?.auto_route ?? false
      },
      set(this: any, v: boolean) {
        if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.auto_route = v
      },
    },
    strictRoute: {
      get(this: any): boolean {
        return this.tunInbound?.strict_route ?? false
      },
      set(this: any, v: boolean) {
        if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.strict_route = v
      },
    },
    endpointIndependentNat: {
      get(this: any): boolean {
        return this.tunInbound?.endpoint_independent_nat ?? false
      },
      set(this: any, v: boolean) {
        if (this.tunInbound && this.tunInbound.type === 'tun') {
          this.tunInbound.endpoint_independent_nat = v
        }
      },
    },
    tunIp: {
      get(this: any): string[] {
        const uiVal = this.subJsonExt?._uiConfig?.tunIp
        if (Array.isArray(uiVal)) return this.normalizeTunIpSelection(uiVal)

        const fakeip = this.subJsonExt?.dns?.servers?.find((s: any) => s.type === 'fakeip')
        const ranges: string[] = []
        if (fakeip?.inet4_range) ranges.push(fakeip.inet4_range)
        if (fakeip?.inet6_range) ranges.push(fakeip.inet6_range)

        return this.normalizeTunIpSelection(ranges)
      },
      set(this: any, v: string[]) {
        this.syncFakeipServerWithSelection(v)
        this.updateJson()
      },
    },
    tunMode: {
      get(this: any): string {
        return this.tunInbound?.stack ?? 'mixed'
      },
      set(this: any, v: string) {
        if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.stack = v
      },
    },
    mixedListen: {
      get(this: any): string {
        const inbs = this.subJsonExt?.inbounds ?? []
        const mixed = inbs.find((i: any) => i.type === 'mixed')
        return mixed?.listen ?? '127.0.0.1'
      },
      set(this: any, v: string) {
        const inbs = this.subJsonExt?.inbounds ?? []
        const mixed = inbs.find((i: any) => i.type === 'mixed')
        if (mixed) mixed.listen = v
      },
    },
    mixedListenPort: {
      get(this: any): number {
        const inbs = this.subJsonExt?.inbounds ?? []
        const mixed = inbs.find((i: any) => i.type === 'mixed')
        return mixed?.listen_port ?? 2080
      },
      set(this: any, v: number) {
        const inbs = this.subJsonExt?.inbounds ?? []
        const mixed = inbs.find((i: any) => i.type === 'mixed')
        if (mixed) mixed.listen_port = v
      },
    },
    platformProxy: {
      get(this: any): boolean {
        return this.tunInbound?.platform?.http_proxy?.enabled ?? false
      },
      set(this: any, v: boolean) {
        const tun = this.tunInbound
        if (!tun || tun.type !== 'tun') return
        if (v) {
          if (!tun.platform) {
            tun.platform = {
              http_proxy: { enabled: true, server: '127.0.0.1', server_port: 2080 },
            }
          } else if (!tun.platform.http_proxy) {
            tun.platform.http_proxy = { enabled: true, server: '127.0.0.1', server_port: 2080 }
          }
          tun.platform.http_proxy.enabled = true
        } else {
          if (tun.platform?.http_proxy) {
            delete tun.platform.http_proxy
          }
          if (tun.platform && Object.keys(tun.platform).length === 0) {
            delete tun.platform
          }
        }
        this.updateJson()
      },
    },
  },
}
