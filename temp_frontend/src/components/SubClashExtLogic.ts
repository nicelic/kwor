// Clash subscription UI logic (Options API mixin). This file is independent from SubJsonExtLogic.ts.

import { push } from 'notivue'
import { i18n } from '@/locales'
import yaml from 'yaml'
import {
  defaultClashConfig,
  defaultFakeIpRange,
  clashDomainIpTypes,
  CLASH_RULE_SET_URL_TEMPLATES,
  CLASH_METACUBEX_NAME_MAP,
  CLASH_SOURCES_NEED_NAME_MAP,
  CLASH_RULE_SET_NAME_OPTIONS_BY_SOURCE,
} from './SubClashExtConstants'

const CLASH_ALLOWED_RULE_SET_EXTENSIONS = new Set(['.mrs', '.yaml', '.yml', '.txt', '.list'])

function normalizeClashRuleSetSource(input: any): string {
  if (typeof input !== 'string') return 'metacubex_cdn'
  const source = input.trim()
  if (source === '') return ''

  if (source === 'karingx_github' || source === 'chocolate4u_github' || source === 'lyc8503_github') {
    return 'metacubex_github'
  }
  if (source === 'karingx_cdn' || source === 'chocolate4u_cdn' || source === 'lyc8503_cdn' || source === 'lyc8503_cdn1') {
    return 'metacubex_cdn'
  }
  if (source === 'loyalsoldier_github') {
    return source
  }

  if (Object.prototype.hasOwnProperty.call(CLASH_RULE_SET_URL_TEMPLATES, source)) {
    return source
  }
  return 'metacubex_cdn'
}

function normalizeOptionalClashRuleSetSource(input: any): string | null {
  if (input == null) return null
  if (typeof input !== 'string') return null

  const source = input.trim()
  if (source === '') return ''

  if (source === 'karingx_github' || source === 'chocolate4u_github' || source === 'lyc8503_github') {
    return 'metacubex_github'
  }
  if (source === 'karingx_cdn' || source === 'chocolate4u_cdn' || source === 'lyc8503_cdn' || source === 'lyc8503_cdn1') {
    return 'metacubex_cdn'
  }
  if (source === 'loyalsoldier_github') {
    return source
  }
  if (Object.prototype.hasOwnProperty.call(CLASH_RULE_SET_URL_TEMPLATES, source)) {
    return source
  }

  return null
}

function getClashRuleSetSourceCacheKey(source: string): string {
  return source || '__custom_url__'
}

function sanitizeClashResolvedRuleSetUrls(input: any): Record<string, any> {
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

    const actualSource = normalizeOptionalClashRuleSetSource((value as any)?.source)
    if (actualSource == null) continue
    if (getClashRuleSetSourceCacheKey(actualSource) !== expectedSourceKey) continue

    next[key] = value
  }

  return next
}

function normalizeName(input: string): string {
  let name = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (name.startsWith('geosite-')) name = name.substring(8)
  if (name.startsWith('geoip-')) name = name.substring(6)
  return name
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

function isSupportedClashRuleSetUrl(input: string): boolean {
  const ext = getRuleSetUrlExtension(input)
  return CLASH_ALLOWED_RULE_SET_EXTENSIONS.has(ext)
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

function buildClashRuleSetUrl(source: string, prefix: 'geosite' | 'geoip', name: string): string {
  const rawName = typeof name === 'string' ? name.trim() : ''
  if (!rawName) return ''

  if (isHttpRuleSetInput(rawName)) {
    return rawName
  }

  const cleanName = normalizeName(rawName)
  if (!cleanName) return ''

  const normalizedSource = normalizeClashRuleSetSource(source)
  const templates = CLASH_RULE_SET_URL_TEMPLATES[normalizedSource]
  if (templates) {
    let finalName = cleanName
    if (CLASH_SOURCES_NEED_NAME_MAP.includes(normalizedSource)) {
      finalName = CLASH_METACUBEX_NAME_MAP[cleanName] || cleanName
    }
    const template = prefix === 'geosite' ? templates.geosite : templates.geoip
    return template.replace('{name}', finalName)
  }

  return ''
}

function getRuleSetEntryFormat(source: string, url: string): 'mrs' | 'text' | 'yaml' {
  const normalizedSource = normalizeClashRuleSetSource(source)
  const ext = getRuleSetUrlExtension(url)
  if (ext === '.mrs') return 'mrs'
  if (ext === '.txt' || ext === '.list') return 'text'
  if (ext === '.yaml' || ext === '.yml') return 'yaml'
  if (normalizedSource === 'loyalsoldier_github') return 'text'
  return 'mrs'
}

function getRuleSetEntryBehavior(
  source: string,
  prefix: 'geosite' | 'geoip',
  cleanName: string
): 'domain' | 'ipcidr' | 'classical' {
  const normalizedSource = normalizeClashRuleSetSource(source)
  if (normalizedSource === 'loyalsoldier_github' && prefix === 'geosite' && cleanName === 'applications') {
    return 'classical'
  }
  return prefix === 'geosite' ? 'domain' : 'ipcidr'
}

function parseRouteOrder(orderStr: string): number[] {
  const defaultOrder = [1, 2, 3, 4, 5, 6, 7, 8, 9]
  if (!orderStr || orderStr.trim() === '') return defaultOrder

  const cleaned = orderStr.trim()
  let digits: number[]

  if (cleaned.includes(',')) {
    digits = cleaned.split(',').map((s: string) => parseInt(s.trim(), 10)).filter((n: number) => !isNaN(n) && n >= 1 && n <= 9)
  } else if (cleaned.includes('.')) {
    digits = cleaned.split('.').map((s: string) => parseInt(s.trim(), 10)).filter((n: number) => !isNaN(n) && n >= 1 && n <= 9)
  } else {
    digits = cleaned.split('').map((c: string) => parseInt(c, 10)).filter((n: number) => !isNaN(n) && n >= 1 && n <= 9)
  }

  if (digits.length === 0) return defaultOrder

  for (let i = 1; i <= 9; i++) {
    if (!digits.includes(i)) {
      digits.push(i)
    }
  }
  return digits
}

function splitByDelimiters(input: string): string[] {
  return input.split(/[,\s\n\r]+/).map((s: string) => s.trim()).filter((s: string) => s.length > 0)
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

async function validateUrl(url: string): Promise<boolean> {
  if (!url) return false
  try {
    const response = await fetch(url, { method: 'HEAD', mode: 'cors', signal: AbortSignal.timeout(8000) })
    return response.ok
  } catch {
    try {
      const response = await fetch(url, { method: 'HEAD', mode: 'no-cors', signal: AbortSignal.timeout(8000) })
      return response.type === 'opaque'
    } catch {
      return false
    }
  }
}

function deepCopyClashValue<T>(value: T): T {
  return JSON.parse(JSON.stringify(value))
}

function normalizeLatencyInput(input: any): string {
  if (typeof input === 'string') return input.trim()
  if (typeof input === 'number' && Number.isFinite(input)) return String(Math.trunc(input))
  return ''
}

const defaultMihomoLatencyTestInterval = '180s'
const defaultMihomoLatencyTolerance = '50'

function getMihomoLatencyIntervalError(input: any): string {
  const value = normalizeLatencyInput(input)
  if (!value) return ''

  if (/^\d+$/.test(value)) {
    return '测试延迟间隔必须使用秒单位 s，例如 30s。'
  }

  if (!/^[1-9]\d*s$/i.test(value)) {
    return '测试延迟间隔格式无效，仅支持“正整数 + s（秒）”，不支持 ms/m/h/d。'
  }

  return ''
}

function normalizeMihomoLatencyInterval(input: any): string {
  const value = normalizeLatencyInput(input)
  if (!value) return ''
  if (getMihomoLatencyIntervalError(value)) return ''
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

function normalizeLatencyToleranceMs(input: any): string {
  const value = normalizeLatencyInput(input)
  if (!value) return ''
  if (getLatencyToleranceMsError(value)) return ''
  return value
}

function parseIntervalToSeconds(interval: string): number {
  if (!interval) return 86400
  const match = interval.match(/^(\d+)\s*([dhms]?)$/i)
  if (!match) return 86400
  const num = parseInt(match[1], 10)
  const unit = match[2].toLowerCase()
  switch (unit) {
    case 'd': return num * 86400
    case 'h': return num * 3600
    case 'm': return num * 60
    case 's': return num
    default: return num * 86400
  }
}

function parseFakeIpTtlSeconds(input: any): number | null {
  if (typeof input === 'number' && Number.isFinite(input)) {
    const value = Math.trunc(input)
    return value >= 0 ? value : null
  }

  if (typeof input !== 'string') return null
  const raw = input.trim()
  if (!raw) return null

  const match = raw.match(/^(\d+)\s*s?$/i)
  if (!match) return null

  const value = parseInt(match[1], 10)
  return Number.isFinite(value) ? value : null
}

const defaultKeepAliveIdle = 0
const defaultKeepAliveInterval = 30
const defaultDisableKeepAlive = false

function normalizeNonNegativeInteger(input: any, fallback: number): number {
  if (typeof input === 'number' && Number.isFinite(input)) {
    const value = Math.trunc(input)
    return value >= 0 ? value : fallback
  }
  if (typeof input === 'string') {
    const raw = input.trim()
    if (!raw) return fallback
    const value = parseInt(raw, 10)
    if (Number.isFinite(value) && value >= 0) return value
  }
  return fallback
}

function normalizeBoolean(input: any, fallback: boolean): boolean {
  if (typeof input === 'boolean') return input
  if (typeof input === 'number') return input !== 0
  if (typeof input === 'string') {
    const raw = input.trim().toLowerCase()
    if (raw === 'true' || raw === '1') return true
    if (raw === 'false' || raw === '0') return false
  }
  return fallback
}

function normalizeOptionalBoolean(input: any): boolean | null {
  if (input === undefined || input === null) return null
  if (typeof input === 'boolean') return input
  if (typeof input === 'number') return input !== 0
  if (typeof input === 'string') {
    const raw = input.trim().toLowerCase()
    if (!raw) return null
    if (raw === 'true' || raw === '1') return true
    if (raw === 'false' || raw === '0') return false
  }
  return null
}

function normalizeOptionalString(input: any): string {
  if (typeof input === 'string') {
    return input.trim()
  }
  return ''
}

function normalizeOptionalNonNegativeInteger(input: any): number | null {
  if (typeof input === 'number' && Number.isFinite(input)) {
    const value = Math.trunc(input)
    return value >= 0 ? value : null
  }
  if (typeof input === 'string') {
    const raw = input.trim()
    if (!raw) return null
    const value = parseInt(raw, 10)
    if (Number.isFinite(value) && value >= 0) return value
  }
  return null
}

type ClashHostsValue = string | string[]
type ClashHostsMap = Record<string, ClashHostsValue>
type ClashHostsEntry = { key: string; value: ClashHostsValue }

function stripWrappingQuotes(input: string): string {
  const raw = typeof input === 'string' ? input.trim() : ''
  if (raw.length < 2) return raw
  if ((raw.startsWith("'") && raw.endsWith("'")) || (raw.startsWith('"') && raw.endsWith('"'))) {
    return raw.slice(1, -1).trim()
  }
  return raw
}

function normalizeHostScalar(input: string): string {
  let value = stripWrappingQuotes(input)
  if (!value) return ''
  if (value.startsWith('[') && value.endsWith(']')) {
    const inner = value.slice(1, -1).trim()
    // Treat [::1] as a single IPv6 value, not an array.
    if (inner.length > 0 && !/[,\s，]/.test(inner)) {
      value = stripWrappingQuotes(inner)
    }
  }
  return value
}

function isHostsEntryDelimiter(input: string): boolean {
  return input === ',' || input === '，' || /\s/.test(input)
}

function skipHostsEntryDelimiters(input: string, start: number): number {
  let index = start
  while (index < input.length && isHostsEntryDelimiter(input[index])) {
    index++
  }
  return index
}

function readQuotedText(input: string, start: number): { token: string; next: number } {
  const quote = input[start]
  let index = start + 1
  while (index < input.length) {
    const ch = input[index]
    if (ch === quote && input[index - 1] !== '\\') {
      index++
      break
    }
    index++
  }
  return { token: input.slice(start, index), next: index }
}

function readBracketText(input: string, start: number): { token: string; next: number } {
  let index = start
  let depth = 0
  let inSingleQuote = false
  let inDoubleQuote = false
  while (index < input.length) {
    const ch = input[index]
    if (ch === "'" && !inDoubleQuote && input[index - 1] !== '\\') {
      inSingleQuote = !inSingleQuote
    } else if (ch === '"' && !inSingleQuote && input[index - 1] !== '\\') {
      inDoubleQuote = !inDoubleQuote
    }

    if (!inSingleQuote && !inDoubleQuote) {
      if (ch === '[') {
        depth++
      } else if (ch === ']') {
        depth--
        if (depth <= 0) {
          index++
          break
        }
      }
    }
    index++
  }
  return { token: input.slice(start, index), next: index }
}

function readHostsKeyToken(input: string, start: number): { token: string; next: number } {
  if (start >= input.length) return { token: '', next: start }
  const first = input[start]
  if (first === "'" || first === '"') {
    return readQuotedText(input, start)
  }

  let index = start
  while (index < input.length) {
    const ch = input[index]
    if (ch === ':' || isHostsEntryDelimiter(ch)) break
    index++
  }
  return { token: input.slice(start, index), next: index }
}

function readHostsValueToken(input: string, start: number): { token: string; next: number } {
  let index = start
  while (index < input.length && /\s/.test(input[index])) {
    index++
  }
  if (index >= input.length) return { token: '', next: index }

  const first = input[index]
  if (first === "'" || first === '"') {
    return readQuotedText(input, index)
  }
  if (first === '[') {
    return readBracketText(input, index)
  }

  const begin = index
  while (index < input.length && !isHostsEntryDelimiter(input[index])) {
    index++
  }
  return { token: input.slice(begin, index), next: index }
}

function splitHostsValueTokens(input: string): string[] {
  const tokens: string[] = []
  let current = ''
  let inSingleQuote = false
  let inDoubleQuote = false
  let bracketDepth = 0

  for (let index = 0; index < input.length; index++) {
    const ch = input[index]
    if (ch === "'" && !inDoubleQuote && input[index - 1] !== '\\') {
      inSingleQuote = !inSingleQuote
      current += ch
      continue
    }
    if (ch === '"' && !inSingleQuote && input[index - 1] !== '\\') {
      inDoubleQuote = !inDoubleQuote
      current += ch
      continue
    }

    if (!inSingleQuote && !inDoubleQuote) {
      if (ch === '[') {
        bracketDepth++
        current += ch
        continue
      }
      if (ch === ']' && bracketDepth > 0) {
        bracketDepth--
        current += ch
        continue
      }
      if (bracketDepth === 0 && (ch === ',' || ch === '，' || /\s/.test(ch))) {
        const token = current.trim()
        if (token.length > 0) tokens.push(token)
        current = ''
        continue
      }
    }

    current += ch
  }

  const lastToken = current.trim()
  if (lastToken.length > 0) tokens.push(lastToken)
  return tokens
}

function uniqueHostScalars(values: string[]): string[] {
  const result: string[] = []
  const seen = new Set<string>()
  for (const raw of values) {
    const value = normalizeHostScalar(raw)
    if (!value || seen.has(value)) continue
    seen.add(value)
    result.push(value)
  }
  return result
}

function normalizeHostsKey(input: string): string {
  const key = normalizeHostScalar(input)
  if (!key) return ''
  return key
}

function normalizeHostsValueToken(input: string): ClashHostsValue | null {
  let value = stripWrappingQuotes(input)
  if (!value) return null

  if (value.startsWith('[') && value.endsWith(']')) {
    const inner = value.slice(1, -1).trim()
    const values = uniqueHostScalars(splitHostsValueTokens(inner))
    if (values.length === 0) return null
    if (values.length === 1) return values[0]
    return values
  }

  const splitValues = uniqueHostScalars(splitHostsValueTokens(value))
  if (splitValues.length > 1) return splitValues
  if (splitValues.length === 1) return splitValues[0]

  value = normalizeHostScalar(value)
  return value || null
}

function hostsValueToArray(value: ClashHostsValue): string[] {
  return Array.isArray(value) ? value : [value]
}

function mergeHostsValues(prevValue: ClashHostsValue, nextValue: ClashHostsValue): ClashHostsValue {
  const mergedValues = uniqueHostScalars([
    ...hostsValueToArray(prevValue),
    ...hostsValueToArray(nextValue),
  ])
  if (mergedValues.length <= 1) return mergedValues[0] ?? ''
  return mergedValues
}

function parseHostsEntriesFromText(input: string): ClashHostsEntry[] {
  const entries: ClashHostsEntry[] = []
  if (typeof input !== 'string' || input.trim().length === 0) return entries

  let index = 0
  while (index < input.length) {
    index = skipHostsEntryDelimiters(input, index)
    if (index >= input.length) break

    const keyToken = readHostsKeyToken(input, index)
    if (!keyToken.token) break
    index = keyToken.next

    while (index < input.length && /\s/.test(input[index])) {
      index++
    }
    if (input[index] !== ':') {
      if (index < input.length) {
        index++
      } else {
        break
      }
      continue
    }

    index++
    const valueToken = readHostsValueToken(input, index)
    index = valueToken.next

    const key = normalizeHostsKey(keyToken.token)
    const value = normalizeHostsValueToken(valueToken.token)
    if (!key || value == null) continue
    entries.push({ key, value })
  }

  return entries
}

function normalizeHostsEntriesInput(input: any): ClashHostsMap {
  const values = Array.isArray(input) ? input : input != null ? [input] : []
  const hosts: ClashHostsMap = {}

  for (const item of values) {
    if (typeof item !== 'string') continue
    const entries = parseHostsEntriesFromText(item)
    for (const entry of entries) {
      const existing = hosts[entry.key]
      if (existing === undefined) {
        hosts[entry.key] = entry.value
      } else {
        const merged = mergeHostsValues(existing, entry.value)
        if (typeof merged === 'string' && merged.length === 0) continue
        hosts[entry.key] = merged
      }
    }
  }

  return hosts
}

function normalizeHostsMap(input: any): ClashHostsMap {
  if (!input || typeof input !== 'object' || Array.isArray(input)) return {}
  const hosts: ClashHostsMap = {}

  for (const [rawKey, rawValue] of Object.entries(input)) {
    const key = normalizeHostsKey(String(rawKey))
    if (!key) continue

    if (Array.isArray(rawValue)) {
      const list = uniqueHostScalars(rawValue.map((item: any) => String(item ?? '')))
      if (list.length === 0) continue
      hosts[key] = list.length === 1 ? list[0] : list
      continue
    }

    if (rawValue == null) continue
    const value = normalizeHostsValueToken(String(rawValue))
    if (value == null) continue
    hosts[key] = value
  }

  return hosts
}

function formatHostsValueForInput(value: ClashHostsValue): string {
  if (Array.isArray(value)) {
    return `[${value.join(', ')}]`
  }
  return value
}

function buildHostsEntryTexts(input: any): string[] {
  const hosts = normalizeHostsMap(input)
  return Object.entries(hosts).map(([key, value]) => `${key}: ${formatHostsValueForInput(value)}`)
}

type ClashRuleRoute = 'REJECT' | 'DIRECT' | 'Proxy'
type ClashSelectorTag = '节点选择' | '自动选择' | '全球直连' | '全球拦截' | '漏网之鱼'
type ClashRuleKind = 'custom' | 'ruleset'
type ClashRuleSetScope = 'domain' | 'ip'
type ClashRuleSetSourceBinding = 'global' | 'override'

type ClashRuleRow = {
  kind: ClashRuleKind
  name: string
  customType: string
  ruleSetScope: ClashRuleSetScope
  ruleSetSourceOverride: string | null
  route: ClashRuleRoute
  noResolve: boolean
  values: string[]
}

type ClashDnsPolicyMatchType = 'domain' | 'geosite' | 'rule-set'
type ClashDnsPolicyRouteTarget = 'nameserver' | 'fallback' | 'direct-nameserver'
type ClashDnsSuffixTarget = 'direct-nameserver' | 'proxy-server-nameserver' | 'nameserver' | 'fallback' | 'default-nameserver'
type ClashDnsSuffixSelection = '节点选择' | 'proxy' | 'disable-ipv4=true' | 'disable-ipv6=true' | 'skip-cert-verify=true' | 'h3=true'
type ClashDnsSuffixRow = {
  targets: ClashDnsSuffixTarget[]
  selections: ClashDnsSuffixSelection[]
}

type ClashDnsPolicyRow = {
  matchType: ClashDnsPolicyMatchType
  routeTarget: ClashDnsPolicyRouteTarget
  values: string[]
}

type ClashUdpPortRange = {
  start: number
  end: number
}

type ClashUdpPortRangesInputParseResult = {
  ranges: ClashUdpPortRange[]
  normalized: string
  error: string
}

const clashRuleRouteValues: ClashRuleRoute[] = ['REJECT', 'DIRECT', 'Proxy']
const clashRuleKindValues: ClashRuleKind[] = ['custom', 'ruleset']
const clashRuleSetScopeValues: ClashRuleSetScope[] = ['domain', 'ip']
const clashNoResolveSupportedCustomTypes = new Set<string>(['IP-CIDR', 'IP-CIDR6', 'IP-SUFFIX', 'IP-ASN', 'GEOIP'])
const clashNodeSelectorTag = '节点选择'
const clashAutoSelectorTag = '自动选择'
const clashGlobalDirectSelectorTag = '全球直连'
const clashGlobalBlockSelectorTag = '全球拦截'
const clashFinalSelectorTag = '漏网之鱼'
const clashGlobalSelectorTag = 'GLOBAL'
const clashSelectorTagValues: ClashSelectorTag[] = [
  clashNodeSelectorTag,
  clashAutoSelectorTag,
  clashGlobalDirectSelectorTag,
  clashGlobalBlockSelectorTag,
  clashFinalSelectorTag,
]
const legacyClashSelectorTagMap: Record<string, string> = {
  '🚀 节点选择': clashNodeSelectorTag,
  '🚀节点选择': clashNodeSelectorTag,
  '\\U0001F680 节点选择': clashNodeSelectorTag,
  '\\U0001F680节点选择': clashNodeSelectorTag,
  '🎈 自动选择': clashAutoSelectorTag,
  '🎈自动选择': clashAutoSelectorTag,
  '\\U0001F388 自动选择': clashAutoSelectorTag,
  '\\U0001F388自动选择': clashAutoSelectorTag,
  '🎯 全球直连': clashGlobalDirectSelectorTag,
  '🎯全球直连': clashGlobalDirectSelectorTag,
  '\\U0001F3AF 全球直连': clashGlobalDirectSelectorTag,
  '\\U0001F3AF全球直连': clashGlobalDirectSelectorTag,
  '🛑 全球拦截': clashGlobalBlockSelectorTag,
  '🛑全球拦截': clashGlobalBlockSelectorTag,
  '\\U0001F6D1 全球拦截': clashGlobalBlockSelectorTag,
  '\\U0001F6D1全球拦截': clashGlobalBlockSelectorTag,
  '🐟 漏网之鱼': clashFinalSelectorTag,
  '🐟漏网之鱼': clashFinalSelectorTag,
  '\\U0001F41F 漏网之鱼': clashFinalSelectorTag,
  '\\U0001F41F漏网之鱼': clashFinalSelectorTag,
}
const clashUdpPortMin = 1
const clashUdpPortMax = 65535
const clashRejectQuicDefaultUdpPortRanges: ClashUdpPortRange[] = [
  { start: 80, end: 80 },
  { start: 443, end: 443 },
  { start: 2443, end: 2443 },
  { start: 4443, end: 4443 },
  { start: 6443, end: 6443 },
  { start: 8080, end: 8080 },
  { start: 8081, end: 8081 },
  { start: 8443, end: 8443 },
]
const clashDnsPolicyMatchTypeValues: ClashDnsPolicyMatchType[] = ['domain', 'geosite', 'rule-set']
const clashDnsPolicyRouteTargetValues: ClashDnsPolicyRouteTarget[] = ['nameserver', 'fallback', 'direct-nameserver']
const clashDnsSuffixTargetValues: ClashDnsSuffixTarget[] = ['direct-nameserver', 'proxy-server-nameserver', 'nameserver', 'fallback', 'default-nameserver']
const clashDnsSuffixSelectionValues: ClashDnsSuffixSelection[] = ['节点选择', 'proxy', 'disable-ipv4=true', 'disable-ipv6=true', 'skip-cert-verify=true', 'h3=true']
const clashDnsSuffixRouteSelectionValues: ClashDnsSuffixSelection[] = ['节点选择', 'proxy']
const clashDnsSuffixSelectionSet = new Set<string>(clashDnsSuffixSelectionValues)
const clashDnsSuffixRouteSelectionSet = new Set<string>(clashDnsSuffixRouteSelectionValues)

function normalizeClashUdpPortRange(range: ClashUdpPortRange): ClashUdpPortRange | null {
  const rawStart = typeof range?.start === 'number' ? Math.trunc(range.start) : NaN
  const rawEnd = typeof range?.end === 'number' ? Math.trunc(range.end) : NaN
  if (!Number.isFinite(rawStart) || !Number.isFinite(rawEnd)) return null
  if (rawStart < clashUdpPortMin || rawStart > clashUdpPortMax) return null
  if (rawEnd < clashUdpPortMin || rawEnd > clashUdpPortMax) return null
  const start = Math.min(rawStart, rawEnd)
  const end = Math.max(rawStart, rawEnd)
  return { start, end }
}

function mergeClashUdpPortRanges(input: ClashUdpPortRange[]): ClashUdpPortRange[] {
  const normalized = (Array.isArray(input) ? input : [])
    .map((range: ClashUdpPortRange) => normalizeClashUdpPortRange(range))
    .filter((range: ClashUdpPortRange | null): range is ClashUdpPortRange => range !== null)

  if (normalized.length === 0) return []

  normalized.sort((a: ClashUdpPortRange, b: ClashUdpPortRange) => {
    if (a.start !== b.start) return a.start - b.start
    return a.end - b.end
  })

  const merged: ClashUdpPortRange[] = [{ ...normalized[0] }]
  for (let i = 1; i < normalized.length; i++) {
    const current = normalized[i]
    const last = merged[merged.length - 1]
    if (current.start <= last.end + 1) {
      if (current.end > last.end) {
        last.end = current.end
      }
      continue
    }
    merged.push({ ...current })
  }

  return merged
}

function serializeClashUdpPortRanges(input: ClashUdpPortRange[]): string {
  const merged = mergeClashUdpPortRanges(input)
  return merged
    .map((range: ClashUdpPortRange) => (range.start === range.end ? `${range.start}` : `${range.start}-${range.end}`))
    .join(',')
}

function splitClashUdpPortInputTokens(input: string): string[] {
  return input
    .replace(/[，]/g, ',')
    .replace(/[\r\n]+/g, ',')
    .split(',')
    .map((item: string) => item.trim())
    .filter((item: string) => item.length > 0)
}

function parseClashUdpPortRuleToken(input: string): ClashUdpPortRange | null {
  const raw = typeof input === 'string' ? input.trim() : ''
  if (!raw) return null

  const normalized = raw
    .replace(/[：]/g, ':')
    .replace(/[；]/g, ';')
    .replace(/[－—–~～]/g, '-')

  if (/^\d{1,5}$/.test(normalized)) {
    const value = parseInt(normalized, 10)
    return normalizeClashUdpPortRange({ start: value, end: value })
  }

  const rangeMatch = normalized.match(/^(\d{1,5})\s*([-:;])\s*(\d{1,5})$/)
  if (!rangeMatch) return null

  const start = parseInt(rangeMatch[1], 10)
  const end = parseInt(rangeMatch[3], 10)
  return normalizeClashUdpPortRange({ start, end })
}

function parseClashUdpPortRangesInput(input: any): ClashUdpPortRangesInputParseResult {
  const raw = normalizeOptionalString(input)
  if (!raw) {
    return { ranges: [], normalized: '', error: '' }
  }

  const tokens = splitClashUdpPortInputTokens(raw)
  if (tokens.length === 0) {
    return { ranges: [], normalized: '', error: '' }
  }

  const ranges: ClashUdpPortRange[] = []
  for (const token of tokens) {
    const parsed = parseClashUdpPortRuleToken(token)
    if (!parsed) {
      return {
        ranges: [],
        normalized: '',
        error: `端口输入格式无效：${token}。示例：443,888-999 或 443，888：999`,
      }
    }
    ranges.push(parsed)
  }

  const normalizedRanges = mergeClashUdpPortRanges(ranges)
  return {
    ranges: normalizedRanges,
    normalized: serializeClashUdpPortRanges(normalizedRanges),
    error: '',
  }
}

function getClashUdpPortRangesInputError(input: any): string {
  return parseClashUdpPortRangesInput(input).error
}

function buildClashRejectUdpRulesFromRanges(input: ClashUdpPortRange[]): string[] {
  return mergeClashUdpPortRanges(input).map((range: ClashUdpPortRange) => {
    const portValue = range.start === range.end ? `${range.start}` : `${range.start}-${range.end}`
    return `AND,((NETWORK,UDP),(DST-PORT,${portValue})),REJECT`
  })
}

const clashRejectQuicRules = buildClashRejectUdpRulesFromRanges(clashRejectQuicDefaultUdpPortRanges)

function extractClashRejectUdpPortRangeFromRule(input: any): ClashUdpPortRange | null {
  const rawRule = typeof input === 'string' ? input.trim() : ''
  if (!rawRule) return null

  const match = rawRule.match(
    /^AND\s*,\s*\(\(\s*NETWORK\s*,\s*UDP\s*\)\s*,\s*\(\s*DST-PORT\s*,\s*([^)]+?)\s*\)\)\s*,\s*REJECT\s*$/i
  )
  if (!match) return null
  return parseClashUdpPortRuleToken(match[1])
}

function extractClashRejectUdpPortRangesFromRules(input: any): ClashUdpPortRange[] {
  const rawRules = Array.isArray(input) ? input : []
  const ranges: ClashUdpPortRange[] = []
  for (const rule of rawRules) {
    const parsed = extractClashRejectUdpPortRangeFromRule(rule)
    if (!parsed) continue
    ranges.push(parsed)
  }
  return mergeClashUdpPortRanges(ranges)
}

function isClashUdpPortRangeCovered(range: ClashUdpPortRange, sourceRanges: ClashUdpPortRange[]): boolean {
  const normalized = normalizeClashUdpPortRange(range)
  if (!normalized) return false

  const source = mergeClashUdpPortRanges(sourceRanges)
  return source.some(
    (item: ClashUdpPortRange) => item.start <= normalized.start && item.end >= normalized.end
  )
}

function areClashUdpPortRangesFullyCovered(expectedRanges: ClashUdpPortRange[], sourceRanges: ClashUdpPortRange[]): boolean {
  const expected = mergeClashUdpPortRanges(expectedRanges)
  if (expected.length === 0) return true
  if (sourceRanges.length === 0) return false

  return expected.every((range: ClashUdpPortRange) => isClashUdpPortRangeCovered(range, sourceRanges))
}

function subtractClashUdpPortRanges(sourceRanges: ClashUdpPortRange[], removeRanges: ClashUdpPortRange[]): ClashUdpPortRange[] {
  let source = mergeClashUdpPortRanges(sourceRanges)
  const removals = mergeClashUdpPortRanges(removeRanges)

  for (const removal of removals) {
    const next: ClashUdpPortRange[] = []
    for (const sourceRange of source) {
      if (removal.end < sourceRange.start || removal.start > sourceRange.end) {
        next.push({ ...sourceRange })
        continue
      }

      if (removal.start > sourceRange.start) {
        next.push({
          start: sourceRange.start,
          end: removal.start - 1,
        })
      }

      if (removal.end < sourceRange.end) {
        next.push({
          start: removal.end + 1,
          end: sourceRange.end,
        })
      }
    }
    source = next
  }

  return mergeClashUdpPortRanges(source)
}

function normalizeLegacyClashSelectorTag(input: any): string {
  const raw = typeof input === 'string' ? input.trim() : ''
  if (!raw) return ''
  return legacyClashSelectorTagMap[raw] || raw
}

function normalizeClashCustomType(input: any): string {
  const type = typeof input === 'string' ? input.trim() : ''
  if (!type) return 'DOMAIN-KEYWORD'
  // Compatibility: legacy UI had "IP" as a standalone type.
  // Mihomo uses CIDR forms, so map it to IP-CIDR.
  if (type.toUpperCase() === 'IP') return 'IP-CIDR'
  const exists = clashDomainIpTypes.some((item: any) => item?.value === type)
  return exists ? type : 'DOMAIN-KEYWORD'
}

function normalizeCustomRuleValueForType(type: string, value: string): string {
  const normalizedType = normalizeClashCustomType(type)
  const trimmed = typeof value === 'string' ? value.trim() : ''
  if (!trimmed) return ''

  if (normalizedType === 'IP-CIDR' || normalizedType === 'IP-CIDR6') {
    if (trimmed.includes('/')) return trimmed
    if (/^\d{1,3}(\.\d{1,3}){3}$/.test(trimmed)) return `${trimmed}/32`
    if (trimmed.includes(':')) return `${trimmed}/128`
  }

  return trimmed
}

function normalizeClashSelectorTag(
  input: any,
  fallback: ClashSelectorTag,
  proxyFallback: ClashSelectorTag
) : string {
  const routeRaw = normalizeLegacyClashSelectorTag(input)
  const route = typeof routeRaw === 'string' ? routeRaw.trim() : ''
  if (!route) return fallback

  if (clashSelectorTagValues.includes(route as ClashSelectorTag)) {
    return route
  }
  if (route === clashGlobalSelectorTag) return clashNodeSelectorTag

  const lower = route.toLowerCase()
  if (lower === 'proxy') return proxyFallback
  if (lower === 'auto') return clashAutoSelectorTag
  if (lower === 'direct' || lower === 'global-direct') return clashGlobalDirectSelectorTag
  if (lower === 'global-proxy' || lower === 'global') return clashNodeSelectorTag
  if (lower === 'global-block' || lower === 'block' || lower === 'reject' || lower === 'reject-drop') return clashGlobalBlockSelectorTag
  if (lower === 'final') return clashFinalSelectorTag

  // Conversion failed: keep the original value as fallback.
  return route
}

function normalizeClashUpdateMethod(input: any): string {
  return normalizeClashSelectorTag(input, clashGlobalDirectSelectorTag, clashNodeSelectorTag)
}

function normalizeClashRouteFinal(input: any): string {
  return normalizeClashSelectorTag(input, clashNodeSelectorTag, clashNodeSelectorTag)
}

function normalizeClashRoute(input: any): ClashRuleRoute {
  const route = normalizeLegacyClashSelectorTag(input)
  if (clashRuleRouteValues.includes(route as ClashRuleRoute)) {
    return route as ClashRuleRoute
  }

  const lower = route.toLowerCase()
  if (lower === 'reject' || lower === 'block') return 'REJECT'
  if (lower === 'direct' || lower === 'global-direct') return 'DIRECT'
  if (lower === 'proxy' || lower === 'global-proxy') return 'Proxy'
  if (route === clashNodeSelectorTag || route === clashAutoSelectorTag || route === clashFinalSelectorTag || route === clashGlobalSelectorTag) return 'Proxy'
  if (route === clashGlobalDirectSelectorTag) return 'DIRECT'
  if (route === clashGlobalBlockSelectorTag) return 'REJECT'
  return 'REJECT'
}

function normalizeClashRouteOutbound(input: any): string {
  const route = normalizeClashRoute(input)
  if (route === 'Proxy') return clashNodeSelectorTag
  return route
}

function normalizeClashRuleKind(input: any): ClashRuleKind {
  const kind = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (clashRuleKindValues.includes(kind as ClashRuleKind)) {
    return kind as ClashRuleKind
  }
  return 'custom'
}

function normalizeClashRuleSetScope(input: any): ClashRuleSetScope {
  const scope = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (scope === 'geosite') return 'domain'
  if (scope === 'geoip') return 'ip'
  if (clashRuleSetScopeValues.includes(scope as ClashRuleSetScope)) {
    return scope as ClashRuleSetScope
  }
  return 'domain'
}

function isClashCustomTypeSupportsNoResolve(input: any): boolean {
  const customType = normalizeClashCustomType(input)
  return clashNoResolveSupportedCustomTypes.has(customType)
}

function supportsClashRuleRowNoResolve(row: any): boolean {
  if (!row) return false
  const kind = normalizeClashRuleKind((row as any)?.kind)
  if (kind === 'ruleset') {
    return normalizeClashRuleSetScope((row as any)?.ruleSetScope) === 'ip'
  }
  return isClashCustomTypeSupportsNoResolve((row as any)?.customType)
}

function normalizeClashRuleNoResolve(input: any): boolean {
  return normalizeBoolean(input, true)
}

function normalizeClashSelectorName(input: any): string {
  if (typeof input !== 'string') return ''
  return normalizeLegacyClashSelectorTag(input)
}

function normalizeClashSelectorNameKey(input: any): string {
  return normalizeClashSelectorName(input).toLowerCase()
}

function getClashRulePrefix(scope: ClashRuleSetScope): 'geosite' | 'geoip' {
  return scope === 'ip' ? 'geoip' : 'geosite'
}

function getClashRuleSetCacheKey(
  prefix: 'geosite' | 'geoip',
  rawName: string,
  source: string,
  sourceBinding: ClashRuleSetSourceBinding
): string {
  const cleanName = normalizeName(rawName)
  if (!cleanName) return ''
  return `${sourceBinding}:${getClashRuleSetSourceCacheKey(source)}:${prefix}:${cleanName}`
}

function normalizeClashRuleValues(input: any): string[] {
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

function normalizeDnsServerList(input: any): string[] {
  return normalizeClashRuleValues(input)
}

function normalizeClashDnsSuffixTargets(input: any): ClashDnsSuffixTarget[] {
  const list = Array.isArray(input) ? input : input != null ? [input] : []
  const result: ClashDnsSuffixTarget[] = []
  const seen = new Set<string>()
  for (const item of list) {
    if (typeof item !== 'string') continue
    const value = item.trim()
    if (!clashDnsSuffixTargetValues.includes(value as ClashDnsSuffixTarget) || seen.has(value)) continue
    seen.add(value)
    result.push(value as ClashDnsSuffixTarget)
  }
  return result
}

function normalizeClashDnsSuffixSelections(input: any): ClashDnsSuffixSelection[] {
  const list = Array.isArray(input) ? input : input != null ? [input] : []
  const result: ClashDnsSuffixSelection[] = []
  const seen = new Set<string>()
  let lastRouteSelection = ''

  for (const item of list) {
    if (typeof item !== 'string') continue
    const value = item.trim()
    if (!clashDnsSuffixSelectionValues.includes(value as ClashDnsSuffixSelection) || seen.has(value)) continue
    seen.add(value)
    result.push(value as ClashDnsSuffixSelection)
    if (clashDnsSuffixRouteSelectionSet.has(value)) {
      lastRouteSelection = value
    }
  }

  if (!lastRouteSelection) {
    return result
  }

  return result.filter((item: ClashDnsSuffixSelection) => {
    if (!clashDnsSuffixRouteSelectionSet.has(item)) return true
    return item === lastRouteSelection
  })
}

function createDefaultClashDnsSuffixRow(targets: any = [], selections: any = []): ClashDnsSuffixRow {
  return {
    targets: normalizeClashDnsSuffixTargets(targets),
    selections: normalizeClashDnsSuffixSelections(selections),
  }
}

function normalizeClashDnsSuffixRows(input: any): ClashDnsSuffixRow[] {
  const rawRows = Array.isArray(input) ? input : []
  const rows = rawRows.map((raw: any) =>
    createDefaultClashDnsSuffixRow(
      raw?.targets ?? raw?.target ?? raw?.dnsTargets ?? raw?.dnsTarget,
      raw?.selections ?? raw?.selection ?? raw?.suffixes ?? raw?.suffix
    )
  )

  if (rows.length === 0) {
    rows.push(createDefaultClashDnsSuffixRow())
  }

  return rows
}

function buildLegacyClashDnsSuffixRows(config: any): ClashDnsSuffixRow[] {
  return normalizeClashDnsSuffixRows([
    {
      targets: config?.clashDnsSuffixTargets,
      selections: config?.clashDnsSuffixSelections,
    },
  ])
}

function filterPersistedClashDnsSuffixRows(rows: ClashDnsSuffixRow[]): ClashDnsSuffixRow[] {
  return rows
    .filter((row: ClashDnsSuffixRow) => row.targets.length > 0 && row.selections.length > 0)
    .map((row: ClashDnsSuffixRow) => ({
      targets: [...row.targets],
      selections: [...row.selections],
    }))
}

function cloneClashDnsSuffixRows(rows: ClashDnsSuffixRow[]): ClashDnsSuffixRow[] {
  return rows.map((row: ClashDnsSuffixRow) => ({
    targets: [...row.targets],
    selections: [...row.selections],
  }))
}

function isSameClashDnsSuffixRows(a: ClashDnsSuffixRow[], b: ClashDnsSuffixRow[]): boolean {
  if (!Array.isArray(a) || !Array.isArray(b)) return false
  if (a.length !== b.length) return false

  for (let i = 0; i < a.length; i++) {
    if (!isSameStringArray(a[i]?.targets ?? [], b[i]?.targets ?? [])) return false
    if (!isSameStringArray(a[i]?.selections ?? [], b[i]?.selections ?? [])) return false
  }

  return true
}

function splitClashDnsServerSuffixTokens(input: string): { address: string; tokens: string[] } {
  const raw = typeof input === 'string' ? input.trim() : ''
  if (!raw) {
    return { address: '', tokens: [] }
  }

  const hashIndex = raw.indexOf('#')
  if (hashIndex < 0) {
    return { address: raw, tokens: [] }
  }

  const address = raw.slice(0, hashIndex).trim()
  const suffixText = raw.slice(hashIndex + 1)
  const tokens = suffixText
    .split('&')
    .map((item: string) => item.trim())
    .filter((item: string) => item.length > 0)

  return { address, tokens }
}

function isClashDnsRouteSuffixToken(input: string): boolean {
  const token = typeof input === 'string' ? input.trim() : ''
  return token.length > 0 && !token.includes('=')
}

function isHttpsClashDnsServerAddress(input: string): boolean {
  return typeof input === 'string' && input.trim().toLowerCase().startsWith('https://')
}

function collectClashDnsSuffixManagedTokensForTarget(
  rows: ClashDnsSuffixRow[],
  target: ClashDnsSuffixTarget,
  address: string
): { routeToken: string; selectionTokens: ClashDnsSuffixSelection[] } {
  const normalizedRows = Array.isArray(rows) ? rows : []
  const selectionTokens: ClashDnsSuffixSelection[] = []
  const seenSelections = new Set<string>()
  let routeToken = ''

  for (const row of normalizedRows) {
    if (!Array.isArray(row?.targets) || !row.targets.includes(target)) continue

    const normalizedSelections = normalizeClashDnsSuffixSelections(row?.selections)
    const rowRouteToken =
      normalizedSelections.find((item: ClashDnsSuffixSelection) => clashDnsSuffixRouteSelectionSet.has(item)) ?? ''
    if (rowRouteToken) {
      routeToken = rowRouteToken
    }

    for (const selection of normalizedSelections) {
      if (clashDnsSuffixRouteSelectionSet.has(selection)) continue
      if (selection === 'h3=true' && !isHttpsClashDnsServerAddress(address)) continue
      if (seenSelections.has(selection)) continue
      seenSelections.add(selection)
      selectionTokens.push(selection)
    }
  }

  return { routeToken, selectionTokens }
}

function stripClashDnsSuffixRowsFromServer(input: string, rows: ClashDnsSuffixRow[], target: ClashDnsSuffixTarget): string {
  const { address, tokens } = splitClashDnsServerSuffixTokens(input)
  if (!address) return normalizeOptionalString(input)

  const { routeToken, selectionTokens } = collectClashDnsSuffixManagedTokensForTarget(rows, target, address)
  const selectionTokenSet = new Set<string>(selectionTokens)

  let remainingTokens = [...tokens]
  if (routeToken) {
    remainingTokens = remainingTokens.filter((token: string) => token !== routeToken)
  }
  if (selectionTokenSet.size > 0) {
    remainingTokens = remainingTokens.filter((token: string) => !selectionTokenSet.has(token))
  }

  if (remainingTokens.length === 0) {
    return address
  }

  return `${address}#${remainingTokens.join('&')}`
}

function applyClashDnsSuffixRowsToServer(input: string, rows: ClashDnsSuffixRow[], target: ClashDnsSuffixTarget): string {
  const { address, tokens } = splitClashDnsServerSuffixTokens(input)
  if (!address) return normalizeOptionalString(input)

  const { routeToken, selectionTokens } = collectClashDnsSuffixManagedTokensForTarget(rows, target, address)
  const selectionTokenSet = new Set<string>(selectionTokens)
  let remainingTokens = [...tokens]

  if (routeToken) {
    remainingTokens = remainingTokens.filter((token: string) => !isClashDnsRouteSuffixToken(token))
  }
  if (selectionTokenSet.size > 0) {
    remainingTokens = remainingTokens.filter((token: string) => !selectionTokenSet.has(token))
  }

  const nextTokens: string[] = []
  if (routeToken) {
    nextTokens.push(routeToken)
  }
  nextTokens.push(...remainingTokens)
  nextTokens.push(...selectionTokens)

  if (nextTokens.length === 0) {
    return address
  }

  return `${address}#${nextTokens.join('&')}`
}

function normalizeGeoipCountryCode(input: any): string {
  if (typeof input !== 'string') return ''
  const token = input
    .trim()
    .split(/[,\s]+/)
    .map((item: string) => item.trim())
    .find((item: string) => item.length > 0) ?? ''
  if (!token) return ''
  return token.toUpperCase().replace(/[^A-Z]/g, '')
}

function isDnsPolicyRuleSetBehaviorSupported(behavior: any): boolean {
  if (typeof behavior !== 'string') return false
  const normalized = behavior.trim().toLowerCase()
  return normalized === 'domain' || normalized === 'classical'
}

function getDnsPolicyRuleSetTagFromKey(key: string): string {
  if (typeof key !== 'string') return ''
  const normalized = key.trim()
  if (!normalized.toLowerCase().startsWith('rule-set:')) return ''
  return normalized.slice('rule-set:'.length).trim()
}

function normalizeClashDnsPolicyMatchType(input: any): ClashDnsPolicyMatchType {
  const matchType = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (clashDnsPolicyMatchTypeValues.includes(matchType as ClashDnsPolicyMatchType)) {
    return matchType as ClashDnsPolicyMatchType
  }
  // Backward compatibility: old UI used these non-mihomo matcher names.
  if (matchType === 'domain-suffix' || matchType === 'domain-keyword') {
    return 'domain'
  }
  return 'geosite'
}

function normalizeClashDnsPolicyRouteTarget(input: any): ClashDnsPolicyRouteTarget {
  const routeTarget = typeof input === 'string' ? input.trim().toLowerCase() : ''
  if (clashDnsPolicyRouteTargetValues.includes(routeTarget as ClashDnsPolicyRouteTarget)) {
    return routeTarget as ClashDnsPolicyRouteTarget
  }
  return 'nameserver'
}

function normalizeDomainPolicyValue(input: string, legacyType: string = ''): string {
  let value = input.trim().toLowerCase()
  if (!value) return ''

  const idx = value.indexOf(':')
  if (idx > 0 && idx < value.length - 1) {
    const prefix = value.slice(0, idx).trim()
    const body = value.slice(idx + 1).trim()
    if (prefix === 'domain' || prefix === 'domain-suffix' || prefix === 'domain-keyword') {
      value = body
      legacyType = prefix
    }
  }
  if (!value) return ''

  if (legacyType === 'domain-suffix') {
    if (value.startsWith('.')) return `+${value}`
    if (!value.startsWith('+.') && !value.startsWith('*.')) return `+.${value}`
  }

  return value
}

function normalizeClashDnsPolicySelectorValue(matchType: ClashDnsPolicyMatchType, input: any, legacyType: string = ''): string {
  if (typeof input !== 'string') return ''
  const value = input.trim()
  if (!value) return ''

  if (matchType === 'rule-set') return value
  if (matchType === 'geosite') return normalizeName(value)
  return normalizeDomainPolicyValue(value, legacyType)
}

function normalizeClashDnsPolicyValues(matchType: ClashDnsPolicyMatchType, input: any, legacyType: string = ''): string[] {
  const list = Array.isArray(input) ? input : input != null ? [input] : []
  const result: string[] = []
  const seen = new Set<string>()

  for (const item of list) {
    const value = normalizeClashDnsPolicySelectorValue(matchType, item, legacyType)
    if (!value || seen.has(value)) continue
    seen.add(value)
    result.push(value)
  }

  return result
}

function createDefaultClashDnsPolicyRow(routeTarget: ClashDnsPolicyRouteTarget = 'nameserver'): ClashDnsPolicyRow {
  return {
    matchType: 'geosite',
    routeTarget,
    values: [],
  }
}

function normalizeClashDnsPolicyRows(input: any): ClashDnsPolicyRow[] {
  const rawRows = Array.isArray(input) ? input : []
  const rows = rawRows.map((raw: any) => {
    const rawMatchType = typeof (raw?.matchType ?? raw?.type) === 'string'
      ? String(raw?.matchType ?? raw?.type).trim().toLowerCase()
      : ''
    const matchType = normalizeClashDnsPolicyMatchType(rawMatchType)
    const routeTarget = normalizeClashDnsPolicyRouteTarget(raw?.routeTarget ?? raw?.route)
    const values = normalizeClashDnsPolicyValues(matchType, raw?.values, rawMatchType)

    return {
      matchType,
      routeTarget,
      values,
    }
  })

  if (rows.length === 0) {
    rows.push(createDefaultClashDnsPolicyRow('nameserver'))
  }

  return rows
}

function filterNonEmptyClashDnsPolicyRows(rows: ClashDnsPolicyRow[]): ClashDnsPolicyRow[] {
  return rows.filter((row: ClashDnsPolicyRow) => row.values.length > 0)
}

function splitDnsPolicyKey(input: string): { matchType: ClashDnsPolicyMatchType; values: string[] } | null {
  if (typeof input !== 'string') return null
  const raw = input.trim()
  if (!raw) return null

  const idx = raw.indexOf(':')
  if (idx > 0 && idx < raw.length - 1) {
    const rawType = raw.slice(0, idx).trim().toLowerCase()
    const rawValues = raw
      .slice(idx + 1)
      .split(',')
      .map((item: string) => item.trim())
      .filter((item: string) => item.length > 0)

    if (rawType === 'geosite' || rawType === 'rule-set') {
      if (rawValues.length === 0) return null
      const matchType = rawType as ClashDnsPolicyMatchType
      return {
        matchType,
        values: normalizeClashDnsPolicyValues(matchType, rawValues),
      }
    }

    if (rawType === 'domain' || rawType === 'domain-suffix' || rawType === 'domain-keyword') {
      if (rawValues.length === 0) return null
      return {
        matchType: 'domain',
        values: normalizeClashDnsPolicyValues('domain', rawValues, rawType),
      }
    }
  }

  const domainValue = normalizeClashDnsPolicySelectorValue('domain', raw)
  if (!domainValue) return null
  return {
    matchType: 'domain',
    values: [domainValue],
  }
}

function buildClashDnsPolicyKeys(row: ClashDnsPolicyRow): string[] {
  const matchType = normalizeClashDnsPolicyMatchType(row?.matchType)
  const values = normalizeClashDnsPolicyValues(matchType, row?.values)
  if (values.length === 0) return []
  if (matchType === 'domain') return values
  return values.map((value: string) => `${matchType}:${value}`)
}

function buildLegacyClashDnsPolicyRows(metaJson: any): ClashDnsPolicyRow[] {
  const policy = metaJson?.dns?.['nameserver-policy']
  if (!policy || typeof policy !== 'object' || Array.isArray(policy)) {
    return normalizeClashDnsPolicyRows([])
  }

  const directNameserverList = normalizeDnsServerList(metaJson?.dns?.['direct-nameserver'])
  const nameserverList = normalizeDnsServerList(metaJson?.dns?.['nameserver'])
  const fallbackList = normalizeDnsServerList(metaJson?.dns?.['fallback'])
  const rows: ClashDnsPolicyRow[] = []

  for (const [rawKey, rawValue] of Object.entries(policy)) {
    const parsed = splitDnsPolicyKey(rawKey)
    if (!parsed) continue

    const serverList = normalizeDnsServerList(rawValue)
    if (serverList.length === 0) continue

    let routeTarget: ClashDnsPolicyRouteTarget = 'nameserver'
    if (
      directNameserverList.length > 0 &&
      isSameStringArray(serverList, directNameserverList) &&
      !isSameStringArray(directNameserverList, nameserverList) &&
      !isSameStringArray(directNameserverList, fallbackList)
    ) {
      routeTarget = 'direct-nameserver'
    } else if (
      fallbackList.length > 0 &&
      isSameStringArray(serverList, fallbackList) &&
      !isSameStringArray(fallbackList, nameserverList)
    ) {
      routeTarget = 'fallback'
    }
    // Merge contiguous entries with the same selector type and route target.
    // This keeps UI rows compact for configs that store one key per value.
    const last = rows.length > 0 ? rows[rows.length - 1] : null
    if (last && last.matchType === parsed.matchType && last.routeTarget === routeTarget) {
      const merged = normalizeClashDnsPolicyValues(parsed.matchType, [...last.values, ...parsed.values])
      last.values = merged
      continue
    }

    rows.push({
      matchType: parsed.matchType,
      routeTarget,
      values: [...parsed.values],
    })
  }

  return normalizeClashDnsPolicyRows(rows)
}

function createDefaultClashRuleRow(kind: ClashRuleKind, route: ClashRuleRoute = 'REJECT'): ClashRuleRow {
  return {
    kind,
    name: '',
    customType: 'DOMAIN-KEYWORD',
    ruleSetScope: 'domain',
    ruleSetSourceOverride: null,
    route,
    noResolve: true,
    values: [],
  }
}

type ClashRuleNameConstraintIssue = {
  code: 'custom_conflicts_ruleset' | 'custom_type_mismatch'
  name: string
  currentType: string
  expectedType?: string
}

type ClashRuleNameConstraintResult = {
  rows: ClashRuleRow[]
  changed: boolean
  issues: ClashRuleNameConstraintIssue[]
}

function applyClashRuleNameConstraints(rows: ClashRuleRow[]): ClashRuleNameConstraintResult {
  const normalizedRows = normalizeClashRuleRows(rows)
  const constrainedRows = normalizedRows.map((row: ClashRuleRow) => ({
    ...row,
    values: [...row.values],
  }))

  const rulesetNameKeys = new Set<string>()
  for (const row of constrainedRows) {
    if (row.kind !== 'ruleset') continue
    const key = normalizeClashSelectorNameKey(row.name)
    if (!key) continue
    rulesetNameKeys.add(key)
  }

  const customNameTypes = new Map<string, string>()
  const issues: ClashRuleNameConstraintIssue[] = []
  let changed = false

  for (const row of constrainedRows) {
    if (row.kind !== 'custom') continue

    const name = normalizeClashSelectorName(row.name)
    if (!name) continue

    const key = normalizeClashSelectorNameKey(name)
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

function normalizeClashRuleRows(input: any): ClashRuleRow[] {
  const rawRows = Array.isArray(input) ? input : []
  const rows = rawRows.map((raw: any) => {
    let inferredKind: ClashRuleKind
    if (raw?.kind !== undefined) {
      inferredKind = normalizeClashRuleKind(raw.kind)
    } else if (raw?.ruleSetScope !== undefined || raw?.scope !== undefined || raw?.prefix !== undefined) {
      inferredKind = 'ruleset'
    } else {
      inferredKind = 'custom'
    }

    const customType = normalizeClashCustomType(raw?.customType ?? raw?.type)
    const name = normalizeClashSelectorName(raw?.name ?? raw?.selectorName)
    const ruleSetScope = normalizeClashRuleSetScope(raw?.ruleSetScope ?? raw?.scope ?? raw?.prefix)
    const ruleSetSourceOverride = normalizeOptionalClashRuleSetSource(raw?.ruleSetSourceOverride ?? raw?.rowRuleSetSource)
    const route = normalizeClashRoute(raw?.route)
    const noResolve = normalizeClashRuleNoResolve(raw?.noResolve ?? raw?.['no-resolve'] ?? raw?.no_resolve)
    const values = normalizeClashRuleValues(raw?.values)

    return {
      kind: inferredKind,
      name,
      customType,
      ruleSetScope,
      ruleSetSourceOverride: inferredKind === 'ruleset' ? ruleSetSourceOverride : null,
      route,
      noResolve,
      values,
    }
  })

  if (rows.length === 0) {
    rows.push(createDefaultClashRuleRow('custom', 'REJECT'))
  }

  return rows
}

function filterNonEmptyClashRuleRows(rows: ClashRuleRow[]): ClashRuleRow[] {
  return rows.filter((row: ClashRuleRow) => row.values.length > 0)
}

function buildLegacyClashRuleRows(config: any): ClashRuleRow[] {
  if (Array.isArray(config?.clashRuleRows)) {
    return normalizeClashRuleRows(config.clashRuleRows)
  }

  const slots: Record<number, ClashRuleRow> = {}

  const pushLegacyCustomSlot = (slot: number, typeVal: any, valuesVal: any, route: ClashRuleRoute) => {
    const values = normalizeClashRuleValues(valuesVal)
    if (values.length === 0) return
    slots[slot] = {
      kind: 'custom',
      name: '',
      customType: normalizeClashCustomType(typeVal),
      ruleSetScope: 'domain',
      ruleSetSourceOverride: null,
      route,
      noResolve: true,
      values,
    }
  }

  const pushLegacyRuleSetSlot = (slot: number, valuesVal: any, scope: ClashRuleSetScope, route: ClashRuleRoute) => {
    const values = normalizeClashRuleValues(valuesVal)
    if (values.length === 0) return
    slots[slot] = {
      kind: 'ruleset',
      name: '',
      customType: 'DOMAIN-KEYWORD',
      ruleSetScope: scope,
      ruleSetSourceOverride: null,
      route,
      noResolve: true,
      values,
    }
  }

  pushLegacyCustomSlot(1, config?.customBlockType, config?.customBlockValue, 'REJECT')
  pushLegacyCustomSlot(2, config?.customDirectType, config?.customDirectValue, 'DIRECT')
  pushLegacyCustomSlot(3, config?.customProxyType, config?.customProxyValue, 'Proxy')

  pushLegacyRuleSetSlot(4, config?.blockRuleSet, 'domain', 'REJECT')
  pushLegacyRuleSetSlot(5, config?.blockRuleSetIp, 'ip', 'REJECT')
  pushLegacyRuleSetSlot(6, config?.proxyRuleSet, 'domain', 'Proxy')
  pushLegacyRuleSetSlot(7, config?.proxyRuleSetIp, 'ip', 'Proxy')
  pushLegacyRuleSetSlot(8, config?.directRuleSet, 'domain', 'DIRECT')
  pushLegacyRuleSetSlot(9, config?.directRuleSetIp, 'ip', 'DIRECT')

  const order = parseRouteOrder(typeof config?.routeOrder === 'string' ? config.routeOrder : '')
  const rows: ClashRuleRow[] = []
  for (const slot of order) {
    const row = slots[slot]
    if (row) rows.push(row)
  }

  if (rows.length === 0) {
    return normalizeClashRuleRows([])
  }

  return normalizeClashRuleRows(rows)
}

const validationTimers: Record<string, ReturnType<typeof setTimeout>> = {}

export const SubClashExtMixin = {
  created(this: any) {
    this.captureClashRuleRowsValidationSnapshot(this.clashRuleRows)
  },
  watch: {
    'settings.subClashExt': {
      handler(this: any, v: string) {
        if (!v) {
          this.metaJson = {}
          return
        }
        try {
          const parsed = yaml.parse(v) ?? {}
          if (JSON.stringify(parsed) !== JSON.stringify(this.metaJson)) {
            this.metaJson = parsed
          }
        } catch (_e) {
          // Ignore invalid yaml while typing.
        }
      },
      immediate: true,
    },

    metaJson: {
      handler(this: any, v: any) {
        const str = (!v || Object.keys(v).length === 0) ? '' : yaml.stringify(v)
        if (str !== this.settings.subClashExt) {
          this.settings.subClashExt = str
        }
      },
      deep: true,
    },

    'metaJson._uiConfig': {
      handler(this: any, config: any) {
        if (!config) return
        if (this._uiConfigLoaded) return
        this._suspendClashRegeneration = true
        this._uiConfigLoaded = true

        try {
          if (config.ruleSetSource !== undefined) this.ruleSetSource = normalizeClashRuleSetSource(config.ruleSetSource)
          if (config.resolvedRuleSetUrls && typeof config.resolvedRuleSetUrls === 'object') {
            this.resolvedRuleSetUrls = sanitizeClashResolvedRuleSetUrls(config.resolvedRuleSetUrls)
          }

          const restoredRows = Array.isArray(config.clashRuleRows)
            ? normalizeClashRuleRows(config.clashRuleRows)
            : buildLegacyClashRuleRows(config)
          this.clashRuleRows = restoredRows
          this.captureClashRuleRowsValidationSnapshot(restoredRows)

          if (config.noResolveGlobal !== undefined) {
            this.clashNoResolveGlobal = normalizeOptionalBoolean(config.noResolveGlobal)
          } else {
            this.clashNoResolveGlobal = null
          }

          const restoredDnsPolicyRows = Array.isArray(config.clashDnsPolicyRows)
            ? normalizeClashDnsPolicyRows(config.clashDnsPolicyRows)
            : buildLegacyClashDnsPolicyRows(this.metaJson)
          this.clashDnsPolicyRows = restoredDnsPolicyRows

          const restoredDnsSuffixRows = Array.isArray(config.clashDnsSuffixRows)
            ? normalizeClashDnsSuffixRows(config.clashDnsSuffixRows)
            : buildLegacyClashDnsSuffixRows(config)
          this.clashDnsSuffixRows = restoredDnsSuffixRows
          this.clashDnsSuffixAppliedRowsSnapshot = filterPersistedClashDnsSuffixRows(restoredDnsSuffixRows)

          if (config.updateMethod !== undefined) this.updateMethod = normalizeClashUpdateMethod(config.updateMethod)
          if (config.updateInterval !== undefined) this.updateInterval = config.updateInterval
          if (config.routeFinal !== undefined) this.routeFinal = normalizeClashRouteFinal(config.routeFinal)
          if (config.latencyTestUrl !== undefined) this.latencyTestUrl = config.latencyTestUrl
          if (config.latencyTestInterval !== undefined) {
            const normalizedLatencyTestInterval = normalizeMihomoLatencyInterval(config.latencyTestInterval)
            this.latencyTestInterval = normalizedLatencyTestInterval || defaultMihomoLatencyTestInterval
          }
          if (config.latencyTolerance !== undefined) {
            const normalizedLatencyTolerance = normalizeLatencyToleranceMs(config.latencyTolerance)
            this.latencyTolerance = normalizedLatencyTolerance || defaultMihomoLatencyTolerance
          }
          const metaSniffer =
            this.metaJson['sniffer'] && typeof this.metaJson['sniffer'] === 'object' && !Array.isArray(this.metaJson['sniffer'])
              ? this.metaJson['sniffer']
              : null
          if (config.enableSniff !== undefined) {
            this.enableSniff = normalizeBoolean(config.enableSniff, true)
          } else if (metaSniffer) {
            this.enableSniff = normalizeBoolean(metaSniffer['enable'], true)
          }
          if (config.snifferOverrideDestination !== undefined) {
            this.snifferOverrideDestination = normalizeOptionalBoolean(config.snifferOverrideDestination)
          } else if (metaSniffer) {
            this.snifferOverrideDestination = normalizeOptionalBoolean(metaSniffer['override-destination'])
          } else {
            this.snifferOverrideDestination = null
          }
          if (config.snifferForceDnsMapping !== undefined) {
            this.snifferForceDnsMapping = normalizeOptionalBoolean(config.snifferForceDnsMapping)
          } else if (metaSniffer) {
            this.snifferForceDnsMapping = normalizeOptionalBoolean(metaSniffer['force-dns-mapping'])
          } else {
            this.snifferForceDnsMapping = null
          }
          if (config.snifferParsePureIp !== undefined) {
            this.snifferParsePureIp = normalizeOptionalBoolean(config.snifferParsePureIp)
          } else if (metaSniffer) {
            this.snifferParsePureIp = normalizeOptionalBoolean(metaSniffer['parse-pure-ip'])
          } else {
            this.snifferParsePureIp = null
          }
          const hasRejectUdpPortsConfig = config.rejectUdpPortsInput !== undefined || config.rejectUdpPorts !== undefined
          const rejectUdpRangesFromRules = extractClashRejectUdpPortRangesFromRules(this.metaJson?.['rules'])
          const shouldInferRejectUdpPortsFromRules =
            !hasRejectUdpPortsConfig &&
            config.enableReject443Udp === undefined &&
            config.enableRejectQuic === undefined

          if (config.enableRejectQuic !== undefined) {
            this.enableRejectQuic = config.enableRejectQuic === true
          } else if (shouldInferRejectUdpPortsFromRules && rejectUdpRangesFromRules.length > 0) {
            this.enableRejectQuic = areClashUdpPortRangesFullyCovered(
              clashRejectQuicDefaultUdpPortRanges,
              rejectUdpRangesFromRules
            )
          }

          let rejectUdpRangesToPersist: ClashUdpPortRange[] = []
          let hasInvalidRejectUdpInput = false
          let invalidRejectUdpInputRaw = ''
          if (hasRejectUdpPortsConfig) {
            const parsedRejectUdpInput = parseClashUdpPortRangesInput(config.rejectUdpPortsInput ?? config.rejectUdpPorts)
            if (parsedRejectUdpInput.error) {
              hasInvalidRejectUdpInput = true
              invalidRejectUdpInputRaw = normalizeOptionalString(config.rejectUdpPortsInput ?? config.rejectUdpPorts)
            } else {
              rejectUdpRangesToPersist = parsedRejectUdpInput.ranges
            }
          } else if (config.enableReject443Udp === true) {
            rejectUdpRangesToPersist = [{ start: 443, end: 443 }]
          } else if (shouldInferRejectUdpPortsFromRules && rejectUdpRangesFromRules.length > 0) {
            rejectUdpRangesToPersist = this.enableRejectQuic
              ? subtractClashUdpPortRanges(rejectUdpRangesFromRules, clashRejectQuicDefaultUdpPortRanges)
              : rejectUdpRangesFromRules
          }
          this.rejectUdpPortsInput = hasInvalidRejectUdpInput
            ? invalidRejectUdpInputRaw
            : serializeClashUdpPortRanges(rejectUdpRangesToPersist)

          const hasKeepAliveInMeta =
            this.metaJson['keep-alive-idle'] !== undefined ||
            this.metaJson['keep-alive-interval'] !== undefined ||
            this.metaJson['disable-keep-alive'] !== undefined
          if (config.mihomoKeepAlive !== undefined) {
            this.mihomoKeepAlive = normalizeBoolean(config.mihomoKeepAlive, false)
          } else if (hasKeepAliveInMeta) {
            this.mihomoKeepAlive = true
          }
          if (config.keepAliveIdle !== undefined) {
            this.keepAliveIdle = normalizeNonNegativeInteger(config.keepAliveIdle, defaultKeepAliveIdle)
          } else if (this.metaJson['keep-alive-idle'] !== undefined) {
            this.keepAliveIdle = normalizeNonNegativeInteger(this.metaJson['keep-alive-idle'], defaultKeepAliveIdle)
          }
          if (config.keepAliveInterval !== undefined) {
            this.keepAliveInterval = normalizeNonNegativeInteger(config.keepAliveInterval, defaultKeepAliveInterval)
          } else if (this.metaJson['keep-alive-interval'] !== undefined) {
            this.keepAliveInterval = normalizeNonNegativeInteger(this.metaJson['keep-alive-interval'], defaultKeepAliveInterval)
          }
          if (config.disableKeepAlive !== undefined) {
            this.disableKeepAlive = normalizeBoolean(config.disableKeepAlive, defaultDisableKeepAlive)
          } else if (this.metaJson['disable-keep-alive'] !== undefined) {
            this.disableKeepAlive = normalizeBoolean(this.metaJson['disable-keep-alive'], defaultDisableKeepAlive)
          }
        } finally {
          this._suspendClashRegeneration = false
        }
        this.regenerateClashConfig()
      },
      immediate: true,
    },

    ruleSetSource(this: any, value: string) {
      const normalized = normalizeClashRuleSetSource(value)
      if (normalized !== value) {
        this.ruleSetSource = normalized
        return
      }
      this.onClashRuleSetSourceChanged()
      this.regenerateClashConfig()
    },
    clashRuleRows: {
      handler(this: any, rows: any[], oldRows: any[]) {
        if (this._suspendClashRegeneration) return
        const previousRowsForValidation = this.getPreviousClashRuleRowsForValidation(oldRows)

        const normalizedRows = normalizeClashRuleRows(rows)
        const withSplitRows = normalizedRows.map((row: ClashRuleRow) => {
          const split = autoSplitArrayItems(row.values)
          if (split) {
            return { ...row, values: split }
          }
          return row
        })

        const constrained = this.applyClashRuleNameConstraints(withSplitRows)
        const finalRows = constrained.rows

        if (JSON.stringify(rows) !== JSON.stringify(finalRows)) {
          this.captureClashRuleRowsValidationSnapshot(finalRows)
          this.clashRuleRows = finalRows
          this.showClashRuleNameConstraintWarnings(constrained.issues)
          return
        }

        this.regenerateClashConfig()
        this.validateNewClashRuleRows(finalRows, previousRowsForValidation)
        this.captureClashRuleRowsValidationSnapshot(finalRows)
      },
      deep: true,
    },
    clashDnsPolicyRows: {
      handler(this: any, rows: any[]) {
        if (this._suspendClashRegeneration) return

        const normalizedRows = normalizeClashDnsPolicyRows(rows)
        const withSplitRows = normalizedRows.map((row: ClashDnsPolicyRow) => {
          const split = autoSplitArrayItems(row.values)
          if (split) {
            return { ...row, values: split }
          }
          return row
        })

        if (JSON.stringify(rows) !== JSON.stringify(withSplitRows)) {
          this.clashDnsPolicyRows = withSplitRows
          return
        }

        this.regenerateClashConfig()
      },
      deep: true,
    },
    dnsNameserver: {
      handler(this: any) {
        if (this._suspendClashRegeneration) return
        if (!this.hasActiveClashDnsPolicyRows()) return
        this.regenerateClashConfig()
      },
      deep: true,
    },
    dnsFallback: {
      handler(this: any) {
        if (this._suspendClashRegeneration) return
        if (!this.hasActiveClashDnsPolicyRows()) return
        this.regenerateClashConfig()
      },
      deep: true,
    },
    dnsDirectNameserver: {
      handler(this: any) {
        if (this._suspendClashRegeneration) return
        if (!this.hasActiveClashDnsPolicyRows()) return
        this.regenerateClashConfig()
      },
      deep: true,
    },
    dnsDirectNameserverFollowPolicy(this: any) {
      if (this._suspendClashRegeneration) return
      if (!this.hasActiveClashDnsPolicyRows()) return
      this.regenerateClashConfig()
    },
    clashDnsSuffixRows: {
      handler(this: any, rows: any[]) {
        if (this._suspendClashRegeneration) return
        const normalized = normalizeClashDnsSuffixRows(rows)
        if (!isSameClashDnsSuffixRows(rows, normalized)) {
          this.clashDnsSuffixRows = normalized
          return
        }
        this.syncClashDnsSuffixUiConfig()
      },
      deep: true,
    },
    clashNoResolveGlobal(this: any, v: any) {
      const normalized = normalizeOptionalBoolean(v)
      if (v !== normalized) {
        this.clashNoResolveGlobal = normalized
        return
      }
      this.regenerateClashConfig()
    },
    updateMethod(this: any) { this.regenerateClashConfig() },
    updateInterval(this: any) { this.regenerateClashConfig() },
    routeFinal(this: any) { this.regenerateClashConfig() },
    enableSniff(this: any) { this.regenerateClashConfig() },
    snifferOverrideDestination(this: any, v: any) {
      const normalized = normalizeOptionalBoolean(v)
      if (v !== normalized) {
        this.snifferOverrideDestination = normalized
        return
      }
      if (this.enableSniff) {
        this.regenerateClashConfig()
      }
    },
    snifferForceDnsMapping(this: any, v: any) {
      const normalized = normalizeOptionalBoolean(v)
      if (v !== normalized) {
        this.snifferForceDnsMapping = normalized
        return
      }
      if (this.enableSniff) {
        this.regenerateClashConfig()
      }
    },
    snifferParsePureIp(this: any, v: any) {
      const normalized = normalizeOptionalBoolean(v)
      if (v !== normalized) {
        this.snifferParsePureIp = normalized
        return
      }
      if (this.enableSniff) {
        this.regenerateClashConfig()
      }
    },
    enableRejectQuic(this: any) { this.regenerateClashConfig() },
    rejectUdpPortsInput(this: any, v: any) {
      if (typeof v !== 'string') {
        this.rejectUdpPortsInput = normalizeOptionalString(v)
        return
      }
      this.regenerateClashConfig()
    },
    latencyTestUrl(this: any) { this.regenerateClashConfig() },
    latencyTestInterval(this: any) { this.regenerateClashConfig() },
    latencyTolerance(this: any) { this.regenerateClashConfig() },
    mihomoKeepAlive(this: any, v: any) {
      const normalized = normalizeBoolean(v, false)
      if (v !== normalized) {
        this.mihomoKeepAlive = normalized
        return
      }
      this.regenerateClashConfig()
    },
    keepAliveIdle(this: any, v: any) {
      const normalized = normalizeNonNegativeInteger(v, defaultKeepAliveIdle)
      if (v !== normalized) {
        this.keepAliveIdle = normalized
        return
      }
      if (this.mihomoKeepAlive) {
        this.regenerateClashConfig()
      }
    },
    keepAliveInterval(this: any, v: any) {
      const normalized = normalizeNonNegativeInteger(v, defaultKeepAliveInterval)
      if (v !== normalized) {
        this.keepAliveInterval = normalized
        return
      }
      if (this.mihomoKeepAlive) {
        this.regenerateClashConfig()
      }
    },
    disableKeepAlive(this: any, v: any) {
      const normalized = normalizeBoolean(v, defaultDisableKeepAlive)
      if (v !== normalized) {
        this.disableKeepAlive = normalized
        return
      }
      if (this.mihomoKeepAlive) {
        this.regenerateClashConfig()
      }
    },
  },

  methods: {
    getPreviousClashRuleRowsForValidation(this: any, oldRows: any[]): ClashRuleRow[] {
      const snapshot = this._clashRuleRowsValidationSnapshot
      if (Array.isArray(snapshot) && snapshot.length > 0) {
        return normalizeClashRuleRows(snapshot)
      }
      return normalizeClashRuleRows(oldRows)
    },
    captureClashRuleRowsValidationSnapshot(this: any, rows: any) {
      this._clashRuleRowsValidationSnapshot = JSON.parse(JSON.stringify(normalizeClashRuleRows(rows)))
    },
    openEditor(this: any) {
      this.enableEditor = true
    },
    resetSubClashPage(this: any) {
      const dataFactory = this.$options?.data
      const defaults = typeof dataFactory === 'function' ? deepCopyClashValue(dataFactory.call(this)) : null

      this._suspendClashRegeneration = true
      if (defaults && typeof defaults === 'object') {
        for (const [key, value] of Object.entries(defaults)) {
          this[key] = value
        }
      }

      this.metaJson = {}
      this.settings.subClashExt = ''
      this._uiConfigLoaded = false
      this.captureClashRuleRowsValidationSnapshot(this.clashRuleRows)

      this.$nextTick(() => {
        this._suspendClashRegeneration = false
      })
    },
    saveEditor(this: any, data: string) {
      try {
        const result = yaml.parse(data)
        if (typeof result !== 'object' || Array.isArray(result)) {
          push.error({
            message: i18n.global.t('failed') + ': ' + i18n.global.t('error.invalidData'),
            duration: 5000,
          })
          return
        }
        this.metaJson = result
        this._uiConfigLoaded = false
        this.enableEditor = false
      } catch (_e) {
        push.error({
          message: i18n.global.t('failed') + ': ' + i18n.global.t('error.invalidData'),
          duration: 5000,
        })
      }
    },
    syncClashDnsSuffixUiConfig(this: any) {
      const rows = normalizeClashDnsSuffixRows(this.clashDnsSuffixRows)
      const baseMeta =
        this.metaJson && typeof this.metaJson === 'object' && !Array.isArray(this.metaJson)
          ? this.metaJson
          : {}
      const currentUiConfig =
        baseMeta._uiConfig && typeof baseMeta._uiConfig === 'object' && !Array.isArray(baseMeta._uiConfig)
          ? baseMeta._uiConfig
          : {}
      const hasPersistedRows = Array.isArray(currentUiConfig.clashDnsSuffixRows)
      const currentRows = hasPersistedRows
        ? normalizeClashDnsSuffixRows(currentUiConfig.clashDnsSuffixRows)
        : buildLegacyClashDnsSuffixRows(currentUiConfig)

      if (hasPersistedRows && isSameClashDnsSuffixRows(currentRows, rows)) {
        return
      }

      const nextUiConfig: any = {
        ...currentUiConfig,
        clashDnsSuffixRows: cloneClashDnsSuffixRows(rows),
      }
      delete nextUiConfig.clashDnsSuffixTargets
      delete nextUiConfig.clashDnsSuffixSelections

      this._uiConfigLoaded = true
      this.metaJson = {
        ...baseMeta,
        _uiConfig: nextUiConfig,
      }
    },

    getTypeLabel(this: any, type: string): string {
      const found = clashDomainIpTypes.find((t) => t.value === type)
      return found ? found.title : type
    },

    getRuleSetScopeLabel(this: any, scope: string): string {
      return scope === 'ip' ? 'IP 规则集' : '域名规则集'
    },

    getClashRuleSetResolveContextForRow(
      this: any,
      row: ClashRuleRow
    ): { source: string; sourceBinding: ClashRuleSetSourceBinding } {
      const rowSource = normalizeOptionalClashRuleSetSource((row as any)?.ruleSetSourceOverride)
      if (rowSource == null) {
        return {
          source: normalizeClashRuleSetSource(this.ruleSetSource),
          sourceBinding: 'global',
        }
      }
      return {
        source: rowSource,
        sourceBinding: 'override',
      }
    },

    getClashRuleSetNameOptions(this: any, scope: string, row?: any): string[] {
      const sourceContext = row
        ? this.getClashRuleSetResolveContextForRow(row)
        : { source: normalizeClashRuleSetSource(this.ruleSetSource), sourceBinding: 'global' as ClashRuleSetSourceBinding }
      const normalizedSource = normalizeClashRuleSetSource(sourceContext.source)
      const optionsBySource = CLASH_RULE_SET_NAME_OPTIONS_BY_SOURCE[normalizedSource]
      const fallbackDomain = Array.isArray(this.clashGeositeNameOptions) ? this.clashGeositeNameOptions : []
      const fallbackIp = Array.isArray(this.clashGeoipNameOptions) ? this.clashGeoipNameOptions : []

      if (!optionsBySource) {
        return scope === 'ip' ? fallbackIp : fallbackDomain
      }

      const options = scope === 'ip' ? optionsBySource.ip : optionsBySource.domain
      if (!Array.isArray(options) || options.length === 0) {
        return scope === 'ip' ? fallbackIp : fallbackDomain
      }
      return options
    },

    getClashDnsPolicyValueOptions(this: any, row: any): string[] {
      const matchType = normalizeClashDnsPolicyMatchType(row?.matchType)
      if (matchType === 'geosite') {
        return this.getClashRuleSetNameOptions('domain')
      }
      if (matchType === 'rule-set') {
        const providerMap = this.metaJson?.['rule-providers']
        if (!providerMap || typeof providerMap !== 'object' || Array.isArray(providerMap)) return []
        return Object.entries(providerMap)
          .filter(([tag, rawProvider]: [string, any]) => {
            if (typeof tag !== 'string' || tag.trim().length === 0) return false
            if (!rawProvider || typeof rawProvider !== 'object' || Array.isArray(rawProvider)) return false
            return isDnsPolicyRuleSetBehaviorSupported(rawProvider.behavior)
          })
          .map(([tag]) => tag)
      }
      return []
    },

    hasActiveClashDnsPolicyRows(this: any): boolean {
      const rows = normalizeClashDnsPolicyRows(this.clashDnsPolicyRows)
      return filterNonEmptyClashDnsPolicyRows(rows).length > 0
    },

    applyClashRuleNameConstraints(this: any, rows: ClashRuleRow[]): ClashRuleNameConstraintResult {
      return applyClashRuleNameConstraints(normalizeClashRuleRows(rows))
    },

    showClashRuleNameConstraintWarnings(this: any, issues: ClashRuleNameConstraintIssue[]) {
      if (!Array.isArray(issues) || issues.length === 0) return

      const seen = new Set<string>()
      for (const issue of issues) {
        const name = normalizeClashSelectorName(issue?.name)
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

    mapNamedClashSelectorDefaultOutbound(this: any): string {
      return clashNodeSelectorTag
    },

    getClashRowRouteTarget(this: any, row: ClashRuleRow): string {
      const selectorName = normalizeClashSelectorName((row as any)?.name)
      if (selectorName) return selectorName
      return normalizeClashRouteOutbound(row.route)
    },

    isClashRowNoResolveSupported(this: any, row: any): boolean {
      return supportsClashRuleRowNoResolve(row)
    },

    getClashRowNoResolveDisplayValue(this: any, row: any): boolean {
      if (!this.isClashRowNoResolveSupported(row)) return false
      const globalSwitch = normalizeOptionalBoolean(this.clashNoResolveGlobal)
      if (globalSwitch != null) return globalSwitch
      return normalizeClashRuleNoResolve((row as any)?.noResolve)
    },

    isClashRowNoResolveDisabled(this: any, row: any): boolean {
      if (!this.isClashRowNoResolveSupported(row)) return true
      return normalizeOptionalBoolean(this.clashNoResolveGlobal) != null
    },

    setClashRowNoResolve(this: any, row: any, value: any) {
      if (!row || typeof row !== 'object') return
      if (this.isClashRowNoResolveDisabled(row)) return
      row.noResolve = normalizeClashRuleNoResolve(value)
    },

    isClashRowNoResolveEnabled(this: any, row: any): boolean {
      if (!this.isClashRowNoResolveSupported(row)) return false
      const globalSwitch = normalizeOptionalBoolean(this.clashNoResolveGlobal)
      if (globalSwitch != null) return globalSwitch
      return normalizeClashRuleNoResolve((row as any)?.noResolve)
    },

    buildNamedClashSelectorGroups(this: any, rows: ClashRuleRow[]): Array<{ name: string; defaultOutbound: string }> {
      const result: Array<{ name: string; defaultOutbound: string }> = []
      const seen = new Set<string>()
      for (const row of rows) {
        const name = normalizeClashSelectorName((row as any)?.name)
        if (!name || seen.has(name)) continue
        seen.add(name)
        result.push({
          name,
          defaultOutbound: this.mapNamedClashSelectorDefaultOutbound(),
        })
      }
      return result
    },

    canDeleteClashRuleRow(this: any, index: number): boolean {
      const rows = normalizeClashRuleRows(this.clashRuleRows)
      return index >= 0 && index < rows.length && rows.length > 1
    },

    insertClashRuleRow(this: any, index: number) {
      const rows = normalizeClashRuleRows(this.clashRuleRows)
      const safeIndex = Number.isInteger(index)
        ? Math.max(-1, Math.min(index, rows.length - 1))
        : rows.length - 1
      const current = rows[safeIndex] ?? createDefaultClashRuleRow('custom', 'REJECT')
      rows.splice(safeIndex + 1, 0, createDefaultClashRuleRow(current.kind, current.route))
      this.clashRuleRows = rows
    },

    removeClashRuleRow(this: any, index: number) {
      const rows = normalizeClashRuleRows(this.clashRuleRows)
      if (index < 0 || index >= rows.length || rows.length <= 1) return
      rows.splice(index, 1)
      this.clashRuleRows = normalizeClashRuleRows(rows)
    },

    moveClashRuleRow(this: any, index: number, delta: number) {
      if (!Number.isInteger(index) || !Number.isInteger(delta) || delta === 0) return
      const rows = normalizeClashRuleRows(this.clashRuleRows)
      const target = index + delta
      if (index < 0 || index >= rows.length || target < 0 || target >= rows.length) return
      const [current] = rows.splice(index, 1)
      rows.splice(target, 0, current)
      this.clashRuleRows = rows
    },

    commitClashRuleRows(this: any) {
      const normalizedRows = normalizeClashRuleRows(this.clashRuleRows)
      const persistedRows = filterNonEmptyClashRuleRows(normalizedRows)
      this.clashRuleRows = normalizeClashRuleRows(persistedRows)
      this.regenerateClashConfig()
    },

    canDeleteClashDnsPolicyRow(this: any, index: number): boolean {
      const rows = normalizeClashDnsPolicyRows(this.clashDnsPolicyRows)
      return index >= 0 && index < rows.length && rows.length > 1
    },

    insertClashDnsPolicyRow(this: any, index: number) {
      const rows = normalizeClashDnsPolicyRows(this.clashDnsPolicyRows)
      const safeIndex = Number.isInteger(index)
        ? Math.max(-1, Math.min(index, rows.length - 1))
        : rows.length - 1
      const current = rows[safeIndex] ?? createDefaultClashDnsPolicyRow('nameserver')
      rows.splice(safeIndex + 1, 0, createDefaultClashDnsPolicyRow(current.routeTarget))
      this.clashDnsPolicyRows = rows
    },

    removeClashDnsPolicyRow(this: any, index: number) {
      const rows = normalizeClashDnsPolicyRows(this.clashDnsPolicyRows)
      if (index < 0 || index >= rows.length || rows.length <= 1) return
      rows.splice(index, 1)
      this.clashDnsPolicyRows = normalizeClashDnsPolicyRows(rows)
    },

    moveClashDnsPolicyRow(this: any, index: number, delta: number) {
      if (!Number.isInteger(index) || !Number.isInteger(delta) || delta === 0) return
      const rows = normalizeClashDnsPolicyRows(this.clashDnsPolicyRows)
      const target = index + delta
      if (index < 0 || index >= rows.length || target < 0 || target >= rows.length) return
      const [current] = rows.splice(index, 1)
      rows.splice(target, 0, current)
      this.clashDnsPolicyRows = rows
    },

    commitClashDnsPolicyRows(this: any) {
      const normalizedRows = normalizeClashDnsPolicyRows(this.clashDnsPolicyRows)
      const persistedRows = filterNonEmptyClashDnsPolicyRows(normalizedRows)
      this.clashDnsPolicyRows = normalizeClashDnsPolicyRows(persistedRows)
      this.regenerateClashConfig()
    },

    canDeleteClashDnsSuffixRow(this: any, index: number): boolean {
      const rows = normalizeClashDnsSuffixRows(this.clashDnsSuffixRows)
      return index >= 0 && index < rows.length && rows.length > 1
    },

    insertClashDnsSuffixRow(this: any, index: number) {
      const rows = normalizeClashDnsSuffixRows(this.clashDnsSuffixRows)
      const safeIndex = Number.isInteger(index)
        ? Math.max(-1, Math.min(index, rows.length - 1))
        : rows.length - 1
      const current = rows[safeIndex] ?? createDefaultClashDnsSuffixRow()
      rows.splice(safeIndex + 1, 0, createDefaultClashDnsSuffixRow(current.targets, []))
      this.clashDnsSuffixRows = rows
    },

    removeClashDnsSuffixRow(this: any, index: number) {
      const rows = normalizeClashDnsSuffixRows(this.clashDnsSuffixRows)
      if (index < 0 || index >= rows.length || rows.length <= 1) return
      rows.splice(index, 1)
      this.clashDnsSuffixRows = normalizeClashDnsSuffixRows(rows)
    },

    moveClashDnsSuffixRow(this: any, index: number, delta: number) {
      if (!Number.isInteger(index) || !Number.isInteger(delta) || delta === 0) return
      const rows = normalizeClashDnsSuffixRows(this.clashDnsSuffixRows)
      const target = index + delta
      if (index < 0 || index >= rows.length || target < 0 || target >= rows.length) return
      const [current] = rows.splice(index, 1)
      rows.splice(target, 0, current)
      this.clashDnsSuffixRows = rows
    },

    commitClashDnsSuffixSelections(this: any): boolean {
      const normalizedRows = normalizeClashDnsSuffixRows(this.clashDnsSuffixRows)
      if (!isSameClashDnsSuffixRows(this.clashDnsSuffixRows, normalizedRows)) {
        this.clashDnsSuffixRows = normalizedRows
      }

      const incompleteRows = normalizedRows
        .map((row: ClashDnsSuffixRow, index: number) => ({ row, index }))
        .filter(({ row }: { row: ClashDnsSuffixRow }) => {
          const hasTargets = row.targets.length > 0
          const hasSelections = row.selections.length > 0
          return hasTargets !== hasSelections
        })

      if (incompleteRows.length > 0) {
        const messages = incompleteRows.map(({ row, index }: { row: ClashDnsSuffixRow; index: number }) => {
          if (row.targets.length === 0) {
            return `第 ${index + 1} 行 dns-选择 不能为空。`
          }
          return `第 ${index + 1} 行 dns后缀 不能为空。`
        })
        push.error({
          title: '保存失败',
          message: messages.join(' '),
          duration: 5000,
        })
        return false
      }

      const persistedRows = filterPersistedClashDnsSuffixRows(normalizedRows)
      const cleanedRows = normalizeClashDnsSuffixRows(persistedRows)
      if (!isSameClashDnsSuffixRows(this.clashDnsSuffixRows, cleanedRows)) {
        this.clashDnsSuffixRows = cleanedRows
      }

      this.syncClashDnsSuffixUiConfig()

      const dnsConfig = this.metaJson['dns']
      if (!dnsConfig || typeof dnsConfig !== 'object' || Array.isArray(dnsConfig)) {
        if (persistedRows.length > 0) {
          push.error({
            title: '保存失败',
            message: 'DNS 未启用或未填写，无法应用 dns后缀。',
            duration: 5000,
          })
          return false
        }
        this.clashDnsSuffixAppliedRowsSnapshot = cloneClashDnsSuffixRows(persistedRows)
        return true
      }

      if (persistedRows.length > 0) {
        const activeTargets = Array.from(new Set(persistedRows.flatMap((row: ClashDnsSuffixRow) => row.targets)))
        const missingTargets = activeTargets.filter((target: ClashDnsSuffixTarget) => normalizeDnsServerList(dnsConfig[target]).length === 0)
        if (missingTargets.length > 0) {
          push.error({
            title: '保存失败',
            message: `${missingTargets.map((target: ClashDnsSuffixTarget) => `(${target})`).join('、')} 没有填写 DNS，无法应用 dns后缀。`,
            duration: 5000,
          })
          return false
        }
      }

      const previousRows = filterPersistedClashDnsSuffixRows(normalizeClashDnsSuffixRows(this.clashDnsSuffixAppliedRowsSnapshot))
      const nextDnsConfig: any = { ...dnsConfig }
      let changed = false

      for (const target of clashDnsSuffixTargetValues) {
        const currentList = normalizeDnsServerList(nextDnsConfig[target])
        if (currentList.length === 0) continue

        const baseList = normalizeDnsServerList(
          currentList.map((item: string) => stripClashDnsSuffixRowsFromServer(item, previousRows, target))
        )
        const nextList = normalizeDnsServerList(
          baseList.map((item: string) => applyClashDnsSuffixRowsToServer(item, persistedRows, target))
        )

        if (!isSameStringArray(currentList, nextList)) {
          changed = true
        }

        if (nextList.length > 0) {
          nextDnsConfig[target] = nextList
        } else {
          delete nextDnsConfig[target]
        }
      }

      const directNameserverList = normalizeDnsServerList(nextDnsConfig['direct-nameserver'])
      if (directNameserverList.length === 0 && nextDnsConfig['direct-nameserver-follow-policy'] === true) {
        delete nextDnsConfig['direct-nameserver-follow-policy']
        changed = true
      }

      if (changed) {
        this.updateMetaJson(nextDnsConfig, 'dns')
      }

      this.clashDnsSuffixAppliedRowsSnapshot = cloneClashDnsSuffixRows(persistedRows)
      return true
    },

    updateMetaJson(this: any, data: any, key: string) {
      const newMeta = { ...this.metaJson }
      if (data == null) {
        delete newMeta[key]
      } else {
        newMeta[key] = data
      }
      this.metaJson = newMeta
    },

    regenerateClashConfig(this: any) {
      if (this._suspendClashRegeneration) return
      if (!this.metaJson || typeof this.metaJson !== 'object') {
        this.metaJson = {}
      }
      const normalizedUpdateMethod = normalizeClashUpdateMethod(this.updateMethod)
      if (this.updateMethod !== normalizedUpdateMethod) {
        this.updateMethod = normalizedUpdateMethod
        return
      }
      const normalizedRouteFinal = normalizeClashRouteFinal(this.routeFinal)
      if (this.routeFinal !== normalizedRouteFinal) {
        this.routeFinal = normalizedRouteFinal
        return
      }

      const normalizedRows = normalizeClashRuleRows(this.clashRuleRows)
      if (JSON.stringify(this.clashRuleRows) !== JSON.stringify(normalizedRows)) {
        this.clashRuleRows = normalizedRows
        return
      }

      const normalizedDnsPolicyRows = normalizeClashDnsPolicyRows(this.clashDnsPolicyRows)
      if (JSON.stringify(this.clashDnsPolicyRows) !== JSON.stringify(normalizedDnsPolicyRows)) {
        this.clashDnsPolicyRows = normalizedDnsPolicyRows
        return
      }
      const normalizedDnsSuffixRows = normalizeClashDnsSuffixRows(this.clashDnsSuffixRows)
      if (!isSameClashDnsSuffixRows(this.clashDnsSuffixRows, normalizedDnsSuffixRows)) {
        this.clashDnsSuffixRows = normalizedDnsSuffixRows
        return
      }

      const constrained = this.applyClashRuleNameConstraints(normalizedRows)
      if (JSON.stringify(normalizedRows) !== JSON.stringify(constrained.rows)) {
        this.clashRuleRows = constrained.rows
        this.showClashRuleNameConstraintWarnings(constrained.issues)
        return
      }

      const previousUiConfig = this.metaJson?._uiConfig && typeof this.metaJson._uiConfig === 'object'
        ? this.metaJson._uiConfig
        : {}
      const hadDnsPolicyUiConfig = Array.isArray(previousUiConfig.clashDnsPolicyRows)
      const persistedRows = filterNonEmptyClashRuleRows(constrained.rows)
      const persistedDnsPolicyRows = filterNonEmptyClashDnsPolicyRows(normalizedDnsPolicyRows)
      const namedSelectorGroups = this.buildNamedClashSelectorGroups(persistedRows)
      const legacyRuleSetBuckets = {
        blockRuleSet: [] as string[],
        blockRuleSetIp: [] as string[],
        proxyRuleSet: [] as string[],
        proxyRuleSetIp: [] as string[],
        directRuleSet: [] as string[],
        directRuleSetIp: [] as string[],
      }
      const legacyCustomBuckets = {
        customBlockType: 'DOMAIN-KEYWORD',
        customBlockValue: [] as string[],
        customDirectType: 'DOMAIN',
        customDirectValue: [] as string[],
        customProxyType: 'DOMAIN-KEYWORD',
        customProxyValue: [] as string[],
      }

      for (const row of persistedRows) {
        if (row.kind === 'custom') {
          if (row.route === 'REJECT') {
            legacyCustomBuckets.customBlockType = row.customType
            legacyCustomBuckets.customBlockValue.push(...row.values)
          }
          if (row.route === 'DIRECT') {
            legacyCustomBuckets.customDirectType = row.customType
            legacyCustomBuckets.customDirectValue.push(...row.values)
          }
          if (row.route === 'Proxy') {
            legacyCustomBuckets.customProxyType = row.customType
            legacyCustomBuckets.customProxyValue.push(...row.values)
          }
          continue
        }

        if (row.route === 'REJECT' && row.ruleSetScope === 'domain') legacyRuleSetBuckets.blockRuleSet.push(...row.values)
        if (row.route === 'REJECT' && row.ruleSetScope === 'ip') legacyRuleSetBuckets.blockRuleSetIp.push(...row.values)
        if (row.route === 'Proxy' && row.ruleSetScope === 'domain') legacyRuleSetBuckets.proxyRuleSet.push(...row.values)
        if (row.route === 'Proxy' && row.ruleSetScope === 'ip') legacyRuleSetBuckets.proxyRuleSetIp.push(...row.values)
        if (row.route === 'DIRECT' && row.ruleSetScope === 'domain') legacyRuleSetBuckets.directRuleSet.push(...row.values)
        if (row.route === 'DIRECT' && row.ruleSetScope === 'ip') legacyRuleSetBuckets.directRuleSetIp.push(...row.values)
      }

      const source = normalizeClashRuleSetSource(this.ruleSetSource)
      if (source !== this.ruleSetSource) {
        this.ruleSetSource = source
      }
      const sanitizedResolvedRuleSetUrls = sanitizeClashResolvedRuleSetUrls(this.resolvedRuleSetUrls)
      if (JSON.stringify(this.resolvedRuleSetUrls || {}) !== JSON.stringify(sanitizedResolvedRuleSetUrls)) {
        this.resolvedRuleSetUrls = sanitizedResolvedRuleSetUrls
      }
      const normalizedLatencyTestInterval = normalizeMihomoLatencyInterval(this.latencyTestInterval)
      const normalizedLatencyTolerance = normalizeLatencyToleranceMs(this.latencyTolerance)
      const parsedCustomRejectUdpPorts = parseClashUdpPortRangesInput(this.rejectUdpPortsInput)
      if (!parsedCustomRejectUdpPorts.error && this.rejectUdpPortsInput !== parsedCustomRejectUdpPorts.normalized) {
        this.rejectUdpPortsInput = parsedCustomRejectUdpPorts.normalized
        return
      }
      const nextUiConfig: any = {
        ruleSetSource: source,
        noResolveGlobal: normalizeOptionalBoolean(this.clashNoResolveGlobal),
        resolvedRuleSetUrls: { ...sanitizedResolvedRuleSetUrls },
        clashRuleRows: persistedRows.map((row: ClashRuleRow) => ({
          kind: row.kind,
          name: row.name,
          customType: row.customType,
          ruleSetScope: row.ruleSetScope,
          ruleSetSourceOverride: row.ruleSetSourceOverride,
          route: row.route,
          noResolve: normalizeClashRuleNoResolve(row.noResolve),
          values: [...row.values],
        })),
        clashSelectorGroups: namedSelectorGroups.map((item: { name: string; defaultOutbound: string }) => ({
          name: item.name,
          defaultOutbound: item.defaultOutbound,
        })),
        customBlockType: legacyCustomBuckets.customBlockType,
        customBlockValue: [...legacyCustomBuckets.customBlockValue],
        customDirectType: legacyCustomBuckets.customDirectType,
        customDirectValue: [...legacyCustomBuckets.customDirectValue],
        customProxyType: legacyCustomBuckets.customProxyType,
        customProxyValue: [...legacyCustomBuckets.customProxyValue],
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
        latencyTestInterval: normalizedLatencyTestInterval,
        latencyTolerance: normalizedLatencyTolerance,
        enableSniff: this.enableSniff,
        snifferOverrideDestination: normalizeOptionalBoolean(this.snifferOverrideDestination),
        snifferForceDnsMapping: normalizeOptionalBoolean(this.snifferForceDnsMapping),
        snifferParsePureIp: normalizeOptionalBoolean(this.snifferParsePureIp),
        enableRejectQuic: this.enableRejectQuic,
        rejectUdpPortsInput: parsedCustomRejectUdpPorts.error
          ? normalizeOptionalString(this.rejectUdpPortsInput)
          : parsedCustomRejectUdpPorts.normalized,
        clashDnsSuffixRows: cloneClashDnsSuffixRows(normalizedDnsSuffixRows),
        mihomoKeepAlive: this.mihomoKeepAlive === true,
        keepAliveIdle: normalizeNonNegativeInteger(this.keepAliveIdle, defaultKeepAliveIdle),
        keepAliveInterval: normalizeNonNegativeInteger(this.keepAliveInterval, defaultKeepAliveInterval),
        disableKeepAlive: normalizeBoolean(this.disableKeepAlive, defaultDisableKeepAlive),
      }

      if (persistedDnsPolicyRows.length > 0 || hadDnsPolicyUiConfig) {
        nextUiConfig.clashDnsPolicyRows = persistedDnsPolicyRows.map((row: ClashDnsPolicyRow) => ({
          matchType: row.matchType,
          routeTarget: row.routeTarget,
          values: [...row.values],
        }))
      }

      this.metaJson._uiConfig = nextUiConfig

      const detour = normalizedUpdateMethod
      const intervalSeconds = parseIntervalToSeconds(this.updateInterval)

      const ruleProviders: Record<string, any> = {}
      const usedRuleProviderTags = new Set<string>()
      const createUniqueRuleProviderTag = (baseTag: string): string => {
        if (!usedRuleProviderTags.has(baseTag)) {
          usedRuleProviderTags.add(baseTag)
          return baseTag
        }
        let index = 1
        while (usedRuleProviderTags.has(`${baseTag}-${index}`)) {
          index++
        }
        const next = `${baseTag}-${index}`
        usedRuleProviderTags.add(next)
        return next
      }
      const rules: string[] = []
      const injectedRejectRules = new Set<string>()
      const injectRejectRule = (rule: string) => {
        const cleanRule = typeof rule === 'string' ? rule.trim() : ''
        if (!cleanRule || injectedRejectRules.has(cleanRule)) return
        injectedRejectRules.add(cleanRule)
        rules.push(cleanRule)
      }

      if (this.enableRejectQuic) {
        for (const rule of clashRejectQuicRules) {
          injectRejectRule(rule)
        }
      }
      if (!parsedCustomRejectUdpPorts.error) {
        for (const rule of buildClashRejectUdpRulesFromRanges(parsedCustomRejectUdpPorts.ranges)) {
          injectRejectRule(rule)
        }
      }

      for (const row of persistedRows) {
        const routeTarget = this.getClashRowRouteTarget(row)
        const enableNoResolve = this.isClashRowNoResolveEnabled(row)
        if (row.kind === 'custom') {
          rules.push(
            ...this.buildCustomClashRules(
              row.customType,
              row.values,
              row.route,
              routeTarget !== normalizeClashRouteOutbound(row.route) ? routeTarget : '',
              enableNoResolve
            )
          )
          continue
        }

        const prefix = getClashRulePrefix(row.ruleSetScope)
        const sourceContext = this.getClashRuleSetResolveContextForRow(row)
        for (const rawName of row.values) {
          const fromUrl = isHttpRuleSetInput(rawName)
          const cleanName = fromUrl ? extractRuleSetNameFromUrl(rawName) : normalizeName(rawName)
          if (!cleanName) continue

          const resolvedUrl = this.getResolvedClashRuleSetUrl(prefix, rawName, sourceContext.source, sourceContext.sourceBinding)
          const url = resolvedUrl || buildClashRuleSetUrl(sourceContext.source, prefix, rawName)
          if (!url) continue
          if (!isSupportedClashRuleSetUrl(url)) continue
          const behavior = getRuleSetEntryBehavior(sourceContext.source, prefix, cleanName)
          const format = getRuleSetEntryFormat(sourceContext.source, url)

          const baseTag = fromUrl ? cleanName : `${prefix}-${cleanName}`
          const tag = createUniqueRuleProviderTag(baseTag)

          const entry: any = {
            type: 'http',
            behavior,
            format,
            url,
            interval: intervalSeconds,
          }
          entry.proxy = detour
          ruleProviders[tag] = entry

          if (tag && ruleProviders[tag]) {
            const outbound = routeTarget || normalizeClashRouteOutbound(row.route)
            if (outbound) {
              const noResolveSuffix = enableNoResolve ? ',no-resolve' : ''
              rules.push(`RULE-SET,${tag},${outbound}${noResolveSuffix}`)
            }
          }
        }
      }

      const finalOutbound = normalizedRouteFinal
      rules.push(`MATCH,${finalOutbound}`)

      if (Object.keys(ruleProviders).length > 0) {
        this.metaJson['rule-providers'] = ruleProviders
      } else {
        delete this.metaJson['rule-providers']
      }

      this.metaJson['rules'] = rules
      this.metaJson['mode'] = 'rule'
      delete this.metaJson['global-client-fingerprint']

      const tunConfig = this.metaJson['tun']
      if (tunConfig && typeof tunConfig === 'object' && !Array.isArray(tunConfig)) {
        const autoDetectInterface = normalizeOptionalBoolean(tunConfig['auto-detect-interface'])
        if (autoDetectInterface == null) {
          delete tunConfig['auto-detect-interface']
        } else {
          tunConfig['auto-detect-interface'] = autoDetectInterface
        }

        if (tunConfig['recvmsgx'] === undefined && tunConfig['recvmmsg'] !== undefined) {
          const recvmsgx = normalizeOptionalBoolean(tunConfig['recvmmsg'])
          if (recvmsgx == null) {
            delete tunConfig['recvmsgx']
          } else {
            tunConfig['recvmsgx'] = recvmsgx
          }
        } else if (tunConfig['recvmsgx'] !== undefined) {
          const recvmsgx = normalizeOptionalBoolean(tunConfig['recvmsgx'])
          if (recvmsgx == null) {
            delete tunConfig['recvmsgx']
          } else {
            tunConfig['recvmsgx'] = recvmsgx
          }
        }

        if (tunConfig['sendmsgx'] !== undefined) {
          const sendmsgx = normalizeOptionalBoolean(tunConfig['sendmsgx'])
          if (sendmsgx == null) {
            delete tunConfig['sendmsgx']
          } else {
            tunConfig['sendmsgx'] = sendmsgx
          }
        }
        delete tunConfig['recvmmsg']
        delete tunConfig['route-address']
        delete tunConfig['inet4-route-address']
        delete tunConfig['inet6-route-address']

        const inet4AddressValues = normalizeDnsServerList(tunConfig['inet4-address'])
        if (inet4AddressValues.length > 0) {
          tunConfig['inet4-address'] = inet4AddressValues
        } else {
          delete tunConfig['inet4-address']
        }

        const inet6AddressValues = normalizeDnsServerList(tunConfig['inet6-address'])
        if (inet6AddressValues.length > 0) {
          tunConfig['inet6-address'] = inet6AddressValues
        } else {
          delete tunConfig['inet6-address']
        }
      }

      const dnsConfig = this.metaJson['dns']
      if (dnsConfig && typeof dnsConfig === 'object' && !Array.isArray(dnsConfig)) {
        const fakeIpRange = normalizeOptionalString(dnsConfig['fake-ip-range'])
        const fakeIpRange6 = normalizeOptionalString(dnsConfig['fake-ip-range6'])
        if (dnsConfig['enhanced-mode'] === 'fake-ip' && !fakeIpRange && !fakeIpRange6) {
          dnsConfig['fake-ip-range'] = defaultFakeIpRange
          delete dnsConfig['fake-ip-range6']
        } else {
          if (fakeIpRange) {
            dnsConfig['fake-ip-range'] = fakeIpRange
          } else {
            delete dnsConfig['fake-ip-range']
          }
          if (fakeIpRange6) {
            dnsConfig['fake-ip-range6'] = fakeIpRange6
          } else {
            delete dnsConfig['fake-ip-range6']
          }
        }

        const ipv6Timeout = normalizeOptionalNonNegativeInteger(dnsConfig['ipv6-timeout'])
        if (dnsConfig['ipv6'] === true && ipv6Timeout != null) {
          dnsConfig['ipv6-timeout'] = ipv6Timeout
        } else {
          delete dnsConfig['ipv6-timeout']
        }

        dnsConfig['prefer-h3'] = normalizeBoolean(dnsConfig['prefer-h3'], false)

        if (dnsConfig['fallback-filter'] && typeof dnsConfig['fallback-filter'] === 'object' && !Array.isArray(dnsConfig['fallback-filter'])) {
          const fallbackFilter: any = { ...dnsConfig['fallback-filter'] }
          const ipcidrValues = normalizeDnsServerList(fallbackFilter['ipcidr'])
          if (ipcidrValues.length > 0) {
            fallbackFilter['ipcidr'] = ipcidrValues
          } else {
            delete fallbackFilter['ipcidr']
          }
          const domainValues = normalizeDnsServerList(fallbackFilter['domain'])
          if (domainValues.length > 0) {
            fallbackFilter['domain'] = domainValues
          } else {
            delete fallbackFilter['domain']
          }
          delete fallbackFilter['geosite']
          if (Object.keys(fallbackFilter).length > 0) {
            dnsConfig['fallback-filter'] = fallbackFilter
          } else {
            delete dnsConfig['fallback-filter']
          }
        }

        const directNameserverList = normalizeDnsServerList(dnsConfig['direct-nameserver'])
        if (directNameserverList.length === 0 || dnsConfig['direct-nameserver-follow-policy'] !== true) {
          delete dnsConfig['direct-nameserver-follow-policy']
        }

        if (persistedDnsPolicyRows.length > 0) {
          const nameserverList = normalizeDnsServerList(dnsConfig['nameserver'])
          const fallbackList = normalizeDnsServerList(dnsConfig['fallback'])
          const nameserverPolicy: Record<string, string[]> = {}
          const followDirectNameserverPolicy = dnsConfig['direct-nameserver-follow-policy'] === true && directNameserverList.length > 0
          const providerMap = this.metaJson?.['rule-providers']

          for (const row of persistedDnsPolicyRows) {
            const targetServers =
              row.routeTarget === 'direct-nameserver'
                ? (followDirectNameserverPolicy ? directNameserverList : [])
                : row.routeTarget === 'fallback'
                  ? fallbackList
                  : nameserverList
            if (targetServers.length === 0) continue

            let keys = buildClashDnsPolicyKeys(row)
            if (row.matchType === 'rule-set') {
              keys = keys.filter((key: string) => {
                const tag = getDnsPolicyRuleSetTagFromKey(key)
                if (!tag) return false
                if (!providerMap || typeof providerMap !== 'object' || Array.isArray(providerMap)) return false
                const provider = providerMap[tag]
                if (!provider || typeof provider !== 'object' || Array.isArray(provider)) return false
                return isDnsPolicyRuleSetBehaviorSupported(provider.behavior)
              })
            }
            if (keys.length === 0) continue

            for (const key of keys) {
              nameserverPolicy[key] = [...targetServers]
            }
          }

          if (Object.keys(nameserverPolicy).length > 0) {
            dnsConfig['nameserver-policy'] = nameserverPolicy
          } else {
            delete dnsConfig['nameserver-policy']
          }
        } else if (hadDnsPolicyUiConfig) {
          delete dnsConfig['nameserver-policy']
        }
      }

      const globalIpv6 = normalizeOptionalBoolean(this.metaJson['ipv6'])
      if (globalIpv6 == null) {
        delete this.metaJson['ipv6']
      } else {
        this.metaJson['ipv6'] = globalIpv6
      }

      if (this.enableSniff) {
        const snifferOverrideDestination = normalizeOptionalBoolean(this.snifferOverrideDestination)
        const snifferForceDnsMapping = normalizeOptionalBoolean(this.snifferForceDnsMapping)
        const snifferParsePureIp = normalizeOptionalBoolean(this.snifferParsePureIp)
        const snifferConfig: any = {
          enable: true,
          sniff: {
            HTTP: { ports: ['1-65535'] },
            TLS: { ports: ['1-65535'] },
            QUIC: { ports: ['1-65535'] },
          },
        }
        if (snifferForceDnsMapping != null) {
          snifferConfig['force-dns-mapping'] = snifferForceDnsMapping
        }
        if (snifferParsePureIp != null) {
          snifferConfig['parse-pure-ip'] = snifferParsePureIp
        }
        if (snifferOverrideDestination != null) {
          snifferConfig['override-destination'] = snifferOverrideDestination
        }
        this.metaJson['sniffer'] = snifferConfig
      } else if (this.metaJson['sniffer']) {
        this.metaJson['sniffer']['enable'] = false
      }

      if (this.mihomoKeepAlive === true) {
        this.metaJson['keep-alive-idle'] = normalizeNonNegativeInteger(this.keepAliveIdle, defaultKeepAliveIdle)
        this.metaJson['keep-alive-interval'] = normalizeNonNegativeInteger(this.keepAliveInterval, defaultKeepAliveInterval)
        this.metaJson['disable-keep-alive'] = normalizeBoolean(this.disableKeepAlive, defaultDisableKeepAlive)
      } else {
        delete this.metaJson['keep-alive-idle']
        delete this.metaJson['keep-alive-interval']
        delete this.metaJson['disable-keep-alive']
      }

      this.metaJson = { ...this.metaJson }
    },

    buildCustomClashRules(
      this: any,
      type: string,
      values: string[],
      outbound: ClashRuleRoute,
      overrideOutbound: string = '',
      noResolveEnabled: boolean = false
    ): string[] {
      const normalizedType = normalizeClashCustomType(type)
      const normalizedOutbound = normalizeClashRouteOutbound(outbound)
      const forcedOutbound = normalizeClashSelectorName(overrideOutbound)
      const normalizedValues = normalizeClashRuleValues(values)
        .map((value: string) => normalizeCustomRuleValueForType(normalizedType, value))
        .filter((value: string) => value.length > 0)
      if (normalizedValues.length === 0) return []
      const noResolveSuffix = noResolveEnabled ? ',no-resolve' : ''
      return normalizedValues.map((value: string) => `${normalizedType},${value},${forcedOutbound || normalizedOutbound}${noResolveSuffix}`)
    },
    onClashRuleSetSourceChanged(this: any) {
      if (this._suspendClashRegeneration) return
      this.ruleSetResolutionRunToken = (this.ruleSetResolutionRunToken || 0) + 1
      this.clearResolvedClashRuleSetUrlsForGlobalRows()
      this.validateAllClashRuleSetEntries(true)
    },
    clearResolvedClashRuleSetUrls(this: any) {
      this.resolvedRuleSetUrls = {}
    },
    clearResolvedClashRuleSetUrlsForGlobalRows(this: any) {
      const current = this.resolvedRuleSetUrls && typeof this.resolvedRuleSetUrls === 'object'
        ? this.resolvedRuleSetUrls
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
        this.resolvedRuleSetUrls = next
      }
    },
    getClashRuleSetSourceOrderForFallback(this: any): string[] {
      const options = Array.isArray(this.clashRuleSetSourceOptions) ? this.clashRuleSetSourceOptions : []
      return options
        .map((item: any) => (typeof item?.value === 'string' ? item.value.trim() : ''))
        .filter((value: string) => value.length > 0 && Boolean(CLASH_RULE_SET_URL_TEMPLATES[value]))
        .filter((value: string, idx: number, arr: string[]) => arr.indexOf(value) === idx)
    },
    getClashRuleSetSourceTitle(this: any, source: string): string {
      const options = Array.isArray(this.clashRuleSetSourceOptions) ? this.clashRuleSetSourceOptions : []
      const found = options.find((item: any) => item?.value === source)
      return typeof found?.title === 'string' && found.title.trim() ? found.title : source
    },
    getResolvedClashRuleSetUrl(
      this: any,
      prefix: 'geosite' | 'geoip',
      rawName: string,
      source: string,
      sourceBinding: ClashRuleSetSourceBinding
    ): string {
      if (isHttpRuleSetInput(rawName)) return ''
      const key = getClashRuleSetCacheKey(prefix, rawName, source, sourceBinding)
      if (!key) return ''
      const matched = this.resolvedRuleSetUrls?.[key]
      if (sourceBinding === 'override') {
        const matchedSource = normalizeOptionalClashRuleSetSource(matched?.source)
        if (matchedSource == null) return ''
        if (getClashRuleSetSourceCacheKey(matchedSource) !== getClashRuleSetSourceCacheKey(source)) return ''
      }
      return typeof matched?.url === 'string' ? matched.url : ''
    },
    validateAllClashRuleSetEntries(this: any, onlyGlobalRows: boolean = false) {
      const rows = normalizeClashRuleRows(this.clashRuleRows).filter((row: ClashRuleRow) => row.kind === 'ruleset')
      for (const row of rows) {
        const sourceContext = this.getClashRuleSetResolveContextForRow(row)
        if (onlyGlobalRows && sourceContext.sourceBinding !== 'global') continue
        const prefix = getClashRulePrefix(row.ruleSetScope)
        const typeLabel = row.ruleSetScope === 'ip' ? 'IP 规则集' : '域名规则集'
        for (const rawName of row.values) {
          this.validateRuleSetEntry(rawName, prefix, typeLabel, sourceContext)
        }
      }
    },

    validateRuleSetEntry(
      this: any,
      rawName: string,
      prefix: 'geosite' | 'geoip',
      typeLabel: string,
      sourceContext: { source: string; sourceBinding: ClashRuleSetSourceBinding }
    ) {
      const fromUrl = isHttpRuleSetInput(rawName)
      const cleanName = fromUrl ? extractRuleSetNameFromUrl(rawName) : normalizeName(rawName)
      if (!cleanName) return
      const source = normalizeClashRuleSetSource(sourceContext?.source)
      const sourceBinding: ClashRuleSetSourceBinding = sourceContext?.sourceBinding === 'override' ? 'override' : 'global'
      const allowFallback = sourceBinding === 'global'
      const noFallbackMessage = (currentSource: string) => currentSource
        ? `${typeLabel} ${cleanName}：当前来源 ${this.getClashRuleSetSourceTitle(currentSource)} 检测失败，不进行回退`
        : `${typeLabel} ${cleanName}：当前所选规则集来源检测失败，不进行回退`

      const timerKey = fromUrl
        ? `clash-${sourceBinding}-${prefix}-url-${rawName.trim()}`
        : `clash-${sourceBinding}-${getClashRuleSetSourceCacheKey(source)}-${prefix}-${cleanName}`
      if (validationTimers[timerKey]) {
        clearTimeout(validationTimers[timerKey])
      }
      validationTimers[timerKey] = setTimeout(async () => {
        try {
          const token = this.ruleSetResolutionRunToken || 0

          if (fromUrl) {
            const url = rawName.trim()
            if (!url) return
            if (!isSupportedClashRuleSetUrl(url)) {
              push.error({
                title: '规则集校验',
                message: `${typeLabel} ${cleanName}：不支持的文件后缀。Clash 订阅仅支持 .mrs/.yaml/.yml/.txt/.list`,
                duration: 5000,
              })
              return
            }
            const isValid = await validateUrl(url)
            if (token !== (this.ruleSetResolutionRunToken || 0)) return
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

          const sourceOrder = this.getClashRuleSetSourceOrderForFallback()
          const currentSource = normalizeClashRuleSetSource(source)
          let matchedSource = ''
          let matchedUrl = ''
          let notifiedNoFallback = false

          if (currentSource && CLASH_RULE_SET_URL_TEMPLATES[currentSource]) {
            const currentUrl = buildClashRuleSetUrl(currentSource, prefix, rawName)
            if (currentUrl && isSupportedClashRuleSetUrl(currentUrl)) {
              const currentValid = await validateUrl(currentUrl)
              if (token !== (this.ruleSetResolutionRunToken || 0)) return
              if (currentValid) {
                matchedSource = currentSource
                matchedUrl = currentUrl
                push.success({
                  title: '规则集校验',
                  message: `${typeLabel} ${cleanName}：当前来源 ${this.getClashRuleSetSourceTitle(currentSource)} 可用`,
                  duration: 3000,
                })
              } else {
                push.warning({
                  title: '规则集校验',
                  message: allowFallback
                    ? `${typeLabel} ${cleanName}：当前来源 ${this.getClashRuleSetSourceTitle(currentSource)} 检测失败，开始回退`
                    : noFallbackMessage(currentSource),
                  duration: 3000,
                })
                notifiedNoFallback = !allowFallback
              }
            }
          }

          const fallbackSources = sourceOrder.filter((source: string) => source !== currentSource)
          if (!matchedUrl && allowFallback) {
            for (const source of fallbackSources) {
              const url = buildClashRuleSetUrl(source, prefix, rawName)
              if (!url) continue
              if (!isSupportedClashRuleSetUrl(url)) continue
              const isValid = await validateUrl(url)
              if (token !== (this.ruleSetResolutionRunToken || 0)) return
              if (isValid) {
                matchedSource = source
                matchedUrl = url
                break
              }
            }
          }

          const cacheKey = getClashRuleSetCacheKey(prefix, cleanName, currentSource, sourceBinding)
          if (!cacheKey) return
          if (!matchedUrl) {
            const cached = this.resolvedRuleSetUrls?.[cacheKey]
            if (cached) {
              const next = { ...(this.resolvedRuleSetUrls || {}) }
              delete next[cacheKey]
              this.resolvedRuleSetUrls = next
              this.regenerateClashConfig()
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
              message: `${typeLabel} ${cleanName}：已回退到 ${this.getClashRuleSetSourceTitle(matchedSource)}`,
              duration: 3000,
            })
          }

          const current = this.resolvedRuleSetUrls?.[cacheKey]
          if (current?.url !== matchedUrl || current?.source !== matchedSource) {
            this.resolvedRuleSetUrls = {
              ...(this.resolvedRuleSetUrls || {}),
              [cacheKey]: { url: matchedUrl, source: matchedSource },
            }
            this.regenerateClashConfig()
          }
        } finally {
          delete validationTimers[timerKey]
        }
      }, 500)
    },

    validateNewClashRuleRows(this: any, newRows: any[], oldRows: any[]) {
      const normalizedNewRows = normalizeClashRuleRows(newRows)
      const normalizedOldRows = normalizeClashRuleRows(oldRows)
      const oldEntryCount = new Map<string, number>()
      const newEntryCount = new Map<string, number>()
      const newEntryMeta = new Map<string, {
        name: string
        prefix: 'geosite' | 'geoip'
        typeLabel: string
        sourceContext: { source: string; sourceBinding: ClashRuleSetSourceBinding }
      }>()

      for (const row of normalizedOldRows) {
        if (row.kind !== 'ruleset') continue
        const prefix = getClashRulePrefix(row.ruleSetScope)
        const sourceContext = this.getClashRuleSetResolveContextForRow(row)
        for (const rawName of row.values) {
          const name = typeof rawName === 'string' ? rawName.trim() : ''
          if (!name) continue
          const key = getClashRuleSetCacheKey(prefix, name, sourceContext.source, sourceContext.sourceBinding)
          if (!key) continue
          oldEntryCount.set(key, (oldEntryCount.get(key) || 0) + 1)
        }
      }

      for (const row of normalizedNewRows) {
        if (row.kind !== 'ruleset') continue
        const prefix = getClashRulePrefix(row.ruleSetScope)
        const typeLabel = row.ruleSetScope === 'ip' ? 'IP 规则集' : '域名规则集'
        const sourceContext = this.getClashRuleSetResolveContextForRow(row)

        for (const rawName of row.values) {
          const name = typeof rawName === 'string' ? rawName.trim() : ''
          if (!name) continue
          const key = getClashRuleSetCacheKey(prefix, name, sourceContext.source, sourceContext.sourceBinding)
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
  },

  computed: {
    latencyTestIntervalError(this: any): string {
      return getMihomoLatencyIntervalError(this.latencyTestInterval)
    },
    latencyToleranceError(this: any): string {
      return getLatencyToleranceMsError(this.latencyTolerance)
    },
    rejectUdpPortsInputError(this: any): string {
      return getClashUdpPortRangesInputError(this.rejectUdpPortsInput)
    },
    editorData(this: any): string {
      if (!this.metaJson || Object.keys(this.metaJson).length === 0) return ''
      const filtered: any = {}
      for (const key of Object.keys(this.metaJson)) {
        if (key !== '_uiConfig') {
          filtered[key] = this.metaJson[key]
        }
      }
      return yaml.stringify(filtered)
    },

    // ===== Basic Settings =====
    mixedPort: {
      get(this: any) { return this.metaJson['mixed-port'] ?? 7890 },
      set(this: any, v: number) { this.updateMetaJson(v, 'mixed-port') },
    },
    globalIpv6: {
      get(this: any) {
        return normalizeOptionalBoolean(this.metaJson['ipv6'])
      },
      set(this: any, v: boolean | null) {
        const value = normalizeOptionalBoolean(v)
        this.updateMetaJson(value, 'ipv6')
      },
    },
    allowLan: {
      get(this: any) { return this.metaJson['allow-lan'] ?? false },
      set(this: any, v: boolean) { this.updateMetaJson(v, 'allow-lan') },
    },
    externalController: {
      get(this: any) { return this.metaJson['external-controller'] ?? '' },
      set(this: any, v: string) { this.updateMetaJson(v || null, 'external-controller') },
    },
    logLevel: {
      get(this: any) { return this.metaJson['log-level'] ?? 'info' },
      set(this: any, v: string) { this.updateMetaJson(v, 'log-level') },
    },

    // ===== TUN Settings =====
    tunEnabled: {
      get(this: any) { return this.metaJson['tun']?.['enable'] ?? false },
      set(this: any, v: boolean) {
        if (v) {
          if (!this.metaJson['tun']) {
            this.updateMetaJson(JSON.parse(JSON.stringify(defaultClashConfig['tun'])), 'tun')
          } else {
            this.updateMetaJson({ ...this.metaJson['tun'], enable: true }, 'tun')
          }
        } else {
          if (this.metaJson['tun']) {
            this.updateMetaJson({ ...this.metaJson['tun'], enable: false }, 'tun')
          }
        }
      },
    },
    tunStack: {
      get(this: any) { return this.metaJson['tun']?.['stack'] ?? 'mixed' },
      set(this: any, v: string) {
        if (this.metaJson['tun']) {
          this.updateMetaJson({ ...this.metaJson['tun'], stack: v }, 'tun')
        }
      },
    },
    tunAutoRoute: {
      get(this: any) { return this.metaJson['tun']?.['auto-route'] ?? true },
      set(this: any, v: boolean) {
        if (this.metaJson['tun']) {
          this.updateMetaJson({ ...this.metaJson['tun'], 'auto-route': v }, 'tun')
        }
      },
    },
    tunStrictRoute: {
      get(this: any) { return this.metaJson['tun']?.['strict-route'] ?? false },
      set(this: any, v: boolean) {
        if (this.metaJson['tun']) {
          this.updateMetaJson({ ...this.metaJson['tun'], 'strict-route': v }, 'tun')
        }
      },
    },
    tunMtu: {
      get(this: any) { return this.metaJson['tun']?.['mtu'] ?? 1500 },
      set(this: any, v: number) {
        if (this.metaJson['tun']) {
          this.updateMetaJson({ ...this.metaJson['tun'], mtu: v }, 'tun')
        }
      },
    },
    tunAutoDetectInterface: {
      get(this: any) {
        return normalizeOptionalBoolean(this.metaJson['tun']?.['auto-detect-interface'])
      },
      set(this: any, v: boolean | null) {
        if (this.metaJson['tun']) {
          const tunConfig: any = { ...this.metaJson['tun'] }
          const value = normalizeOptionalBoolean(v)
          if (value == null) {
            delete tunConfig['auto-detect-interface']
          } else {
            tunConfig['auto-detect-interface'] = value
          }
          this.updateMetaJson(tunConfig, 'tun')
        }
      },
    },
    tunRecvmsgx: {
      get(this: any) {
        const tunConfig = this.metaJson['tun']
        if (!tunConfig || typeof tunConfig !== 'object' || Array.isArray(tunConfig)) return null
        if (tunConfig['recvmsgx'] !== undefined) return normalizeOptionalBoolean(tunConfig['recvmsgx'])
        if (tunConfig['recvmmsg'] !== undefined) return normalizeOptionalBoolean(tunConfig['recvmmsg'])
        return null
      },
      set(this: any, v: any) {
        if (this.metaJson['tun']) {
          const tunConfig: any = { ...this.metaJson['tun'] }
          const value = normalizeOptionalBoolean(v)
          if (value == null) {
            delete tunConfig['recvmsgx']
          } else {
            tunConfig['recvmsgx'] = value
          }
          delete tunConfig['recvmmsg']
          this.updateMetaJson(tunConfig, 'tun')
        }
      },
    },
    tunSendmsgx: {
      get(this: any) {
        const tunConfig = this.metaJson['tun']
        if (!tunConfig || typeof tunConfig !== 'object' || Array.isArray(tunConfig)) return null
        if (tunConfig['sendmsgx'] === undefined) return null
        return normalizeOptionalBoolean(tunConfig['sendmsgx'])
      },
      set(this: any, v: any) {
        if (this.metaJson['tun']) {
          const tunConfig: any = { ...this.metaJson['tun'] }
          const value = normalizeOptionalBoolean(v)
          if (value == null) {
            delete tunConfig['sendmsgx']
          } else {
            tunConfig['sendmsgx'] = value
          }
          this.updateMetaJson(tunConfig, 'tun')
        }
      },
    },
    tunInet4Address: {
      get(this: any) {
        return normalizeDnsServerList(this.metaJson['tun']?.['inet4-address'])
      },
      set(this: any, v: string[]) {
        if (this.metaJson['tun']) {
          const tunConfig: any = { ...this.metaJson['tun'] }
          const values = normalizeDnsServerList(v)
          if (values.length > 0) {
            tunConfig['inet4-address'] = values
          } else {
            delete tunConfig['inet4-address']
          }
          this.updateMetaJson(tunConfig, 'tun')
        }
      },
    },
    tunInet6Address: {
      get(this: any) {
        return normalizeDnsServerList(this.metaJson['tun']?.['inet6-address'])
      },
      set(this: any, v: string[]) {
        if (this.metaJson['tun']) {
          const tunConfig: any = { ...this.metaJson['tun'] }
          const values = normalizeDnsServerList(v)
          if (values.length > 0) {
            tunConfig['inet6-address'] = values
          } else {
            delete tunConfig['inet6-address']
          }
          this.updateMetaJson(tunConfig, 'tun')
        }
      },
    },

    // ===== DNS Settings =====
    dnsEnabled: {
      get(this: any) { return this.metaJson['dns']?.['enable'] ?? false },
      set(this: any, v: boolean) {
        if (v) {
          if (!this.metaJson['dns']) {
            this.updateMetaJson(JSON.parse(JSON.stringify(defaultClashConfig['dns'])), 'dns')
          } else {
            this.updateMetaJson({ ...this.metaJson['dns'], enable: true }, 'dns')
          }
        } else {
          if (this.metaJson['dns']) {
            this.updateMetaJson({ ...this.metaJson['dns'], enable: false }, 'dns')
          }
        }
      },
    },
    dnsIpv6: {
      get(this: any) { return this.metaJson['dns']?.['ipv6'] ?? false },
      set(this: any, v: boolean) {
        if (this.metaJson['dns']) {
          const dnsConfig: any = { ...this.metaJson['dns'], ipv6: v }
          if (v !== true) {
            delete dnsConfig['ipv6-timeout']
          }
          this.updateMetaJson(dnsConfig, 'dns')
        }
      },
    },
    dnsEnhancedMode: {
      get(this: any) { return this.metaJson['dns']?.['enhanced-mode'] ?? 'fake-ip' },
      set(this: any, v: string) {
        if (this.metaJson['dns']) {
          this.updateMetaJson({ ...this.metaJson['dns'], 'enhanced-mode': v }, 'dns')
        }
      },
    },
    dnsFakeIpRange: {
      get(this: any) { return normalizeOptionalString(this.metaJson['dns']?.['fake-ip-range']) },
      set(this: any, v: string) {
        if (this.metaJson['dns']) {
          const dnsConfig: any = { ...this.metaJson['dns'] }
          const value = normalizeOptionalString(v)
          if (value) {
            dnsConfig['fake-ip-range'] = value
          } else {
            delete dnsConfig['fake-ip-range']
          }
          this.updateMetaJson(dnsConfig, 'dns')
        }
      },
    },
    dnsFakeIpRange6: {
      get(this: any) { return normalizeOptionalString(this.metaJson['dns']?.['fake-ip-range6']) },
      set(this: any, v: string) {
        if (this.metaJson['dns']) {
          const dnsConfig: any = { ...this.metaJson['dns'] }
          const value = normalizeOptionalString(v)
          if (value) {
            dnsConfig['fake-ip-range6'] = value
          } else {
            delete dnsConfig['fake-ip-range6']
          }
          this.updateMetaJson(dnsConfig, 'dns')
        }
      },
    },
    dnsIpv6Timeout: {
      get(this: any) {
        const value = normalizeOptionalNonNegativeInteger(this.metaJson['dns']?.['ipv6-timeout'])
        return value == null ? '' : String(value)
      },
      set(this: any, v: any) {
        if (this.metaJson['dns']) {
          const dnsConfig: any = { ...this.metaJson['dns'] }
          const value = normalizeOptionalNonNegativeInteger(v)
          if (dnsConfig['ipv6'] === true && value != null) {
            dnsConfig['ipv6-timeout'] = value
          } else {
            delete dnsConfig['ipv6-timeout']
          }
          this.updateMetaJson(dnsConfig, 'dns')
        }
      },
    },
    dnsFakeIpTtl: {
      get(this: any) {
        const seconds = parseFakeIpTtlSeconds(this.metaJson['dns']?.['fake-ip-ttl'])
        return seconds == null ? '' : String(seconds)
      },
      set(this: any, v: string) {
        if (!this.metaJson['dns'] || typeof this.metaJson['dns'] !== 'object' || Array.isArray(this.metaJson['dns'])) {
          return
        }
        const dnsConfig: any = { ...this.metaJson['dns'] }
        const seconds = parseFakeIpTtlSeconds(v)
        if (seconds == null) {
          delete dnsConfig['fake-ip-ttl']
        } else {
          dnsConfig['fake-ip-ttl'] = seconds
        }
        this.updateMetaJson(dnsConfig, 'dns')
      },
    },
    dnsUseSystemHosts: {
      get(this: any) {
        return normalizeOptionalBoolean(this.metaJson['dns']?.['use-system-hosts'])
      },
      set(this: any, v: boolean | null) {
        const baseDnsConfig =
          this.metaJson['dns'] && typeof this.metaJson['dns'] === 'object' && !Array.isArray(this.metaJson['dns'])
            ? this.metaJson['dns']
            : (defaultClashConfig['dns'] ?? {})
        const dnsConfig: any = { ...baseDnsConfig }
        const value = normalizeOptionalBoolean(v)
        if (value == null) {
          delete dnsConfig['use-system-hosts']
        } else {
          dnsConfig['use-system-hosts'] = value
        }
        this.updateMetaJson(dnsConfig, 'dns')
      },
    },
    dnsUseHosts: {
      get(this: any) {
        return normalizeOptionalBoolean(this.metaJson['dns']?.['use-hosts'])
      },
      set(this: any, v: boolean | null) {
        const baseDnsConfig =
          this.metaJson['dns'] && typeof this.metaJson['dns'] === 'object' && !Array.isArray(this.metaJson['dns'])
            ? this.metaJson['dns']
            : (defaultClashConfig['dns'] ?? {})
        const dnsConfig: any = { ...baseDnsConfig }
        const value = normalizeOptionalBoolean(v)
        if (value == null) {
          delete dnsConfig['use-hosts']
        } else {
          dnsConfig['use-hosts'] = value
        }
        this.updateMetaJson(dnsConfig, 'dns')
      },
    },
    dnsDirectNameserver: {
      get(this: any) { return this.metaJson['dns']?.['direct-nameserver'] ?? [] },
      set(this: any, v: string[]) {
        if (this.metaJson['dns']) {
          const directNameserver = normalizeDnsServerList(v)
          const nextDns: any = { ...this.metaJson['dns'], 'direct-nameserver': directNameserver }
          if (directNameserver.length === 0) {
            delete nextDns['direct-nameserver-follow-policy']
          }
          this.updateMetaJson(nextDns, 'dns')
        }
      },
    },
    dnsProxyServerNameserver: {
      get(this: any) { return this.metaJson['dns']?.['proxy-server-nameserver'] ?? [] },
      set(this: any, v: string[]) {
        if (this.metaJson['dns']) {
          this.updateMetaJson({ ...this.metaJson['dns'], 'proxy-server-nameserver': normalizeDnsServerList(v) }, 'dns')
        }
      },
    },
    dnsDirectNameserverFollowPolicy: {
      get(this: any) { return this.metaJson['dns']?.['direct-nameserver-follow-policy'] === true },
      set(this: any, v: boolean) {
        if (this.metaJson['dns']) {
          const directNameserver = normalizeDnsServerList(this.metaJson['dns']?.['direct-nameserver'])
          const nextDns: any = { ...this.metaJson['dns'], 'direct-nameserver': directNameserver }
          if (directNameserver.length === 0 || v !== true) {
            delete nextDns['direct-nameserver-follow-policy']
          } else {
            nextDns['direct-nameserver-follow-policy'] = true
          }
          this.updateMetaJson(nextDns, 'dns')
        }
      },
    },
    dnsNameserver: {
      get(this: any) { return this.metaJson['dns']?.['nameserver'] ?? [] },
      set(this: any, v: string[]) {
        if (this.metaJson['dns']) {
          this.updateMetaJson({ ...this.metaJson['dns'], nameserver: v }, 'dns')
        }
      },
    },
    dnsFallback: {
      get(this: any) { return this.metaJson['dns']?.['fallback'] ?? [] },
      set(this: any, v: string[]) {
        if (this.metaJson['dns']) {
          this.updateMetaJson({ ...this.metaJson['dns'], fallback: v }, 'dns')
        }
      },
    },
    dnsFallbackFilterGeoip: {
      get(this: any) {
        const fallbackFilter = this.metaJson['dns']?.['fallback-filter']
        if (!fallbackFilter || typeof fallbackFilter !== 'object' || Array.isArray(fallbackFilter)) return true
        return fallbackFilter['geoip'] !== false
      },
      set(this: any, v: boolean) {
        if (this.metaJson['dns']) {
          const dnsConfig = { ...this.metaJson['dns'] }
          const rawFallbackFilter = dnsConfig['fallback-filter']
          const fallbackFilter =
            rawFallbackFilter && typeof rawFallbackFilter === 'object' && !Array.isArray(rawFallbackFilter)
              ? { ...rawFallbackFilter }
              : {}
          fallbackFilter['geoip'] = v === true
          dnsConfig['fallback-filter'] = fallbackFilter
          this.updateMetaJson(dnsConfig, 'dns')
        }
      },
    },
    dnsFallbackFilterIpcidr: {
      get(this: any) {
        return normalizeDnsServerList(this.metaJson['dns']?.['fallback-filter']?.['ipcidr'])
      },
      set(this: any, v: string[]) {
        if (this.metaJson['dns']) {
          const dnsConfig = { ...this.metaJson['dns'] }
          const rawFallbackFilter = dnsConfig['fallback-filter']
          const fallbackFilter =
            rawFallbackFilter && typeof rawFallbackFilter === 'object' && !Array.isArray(rawFallbackFilter)
              ? { ...rawFallbackFilter }
              : {}
          const values = normalizeDnsServerList(v)
          if (values.length > 0) {
            fallbackFilter['ipcidr'] = values
          } else {
            delete fallbackFilter['ipcidr']
          }
          if (Object.keys(fallbackFilter).length > 0) {
            dnsConfig['fallback-filter'] = fallbackFilter
          } else {
            delete dnsConfig['fallback-filter']
          }
          this.updateMetaJson(dnsConfig, 'dns')
        }
      },
    },
    dnsFallbackFilterDomain: {
      get(this: any) {
        return normalizeDnsServerList(this.metaJson['dns']?.['fallback-filter']?.['domain'])
      },
      set(this: any, v: string[]) {
        if (this.metaJson['dns']) {
          const dnsConfig = { ...this.metaJson['dns'] }
          const rawFallbackFilter = dnsConfig['fallback-filter']
          const fallbackFilter =
            rawFallbackFilter && typeof rawFallbackFilter === 'object' && !Array.isArray(rawFallbackFilter)
              ? { ...rawFallbackFilter }
              : {}
          const values = normalizeDnsServerList(v)
          if (values.length > 0) {
            fallbackFilter['domain'] = values
          } else {
            delete fallbackFilter['domain']
          }
          if (Object.keys(fallbackFilter).length > 0) {
            dnsConfig['fallback-filter'] = fallbackFilter
          } else {
            delete dnsConfig['fallback-filter']
          }
          this.updateMetaJson(dnsConfig, 'dns')
        }
      },
    },
    dnsFallbackFilterGeoipCode: {
      get(this: any) {
        return normalizeGeoipCountryCode(this.metaJson['dns']?.['fallback-filter']?.['geoip-code'])
      },
      set(this: any, v: string) {
        if (this.metaJson['dns']) {
          const dnsConfig = { ...this.metaJson['dns'] }
          const rawFallbackFilter = dnsConfig['fallback-filter']
          const fallbackFilter =
            rawFallbackFilter && typeof rawFallbackFilter === 'object' && !Array.isArray(rawFallbackFilter)
              ? { ...rawFallbackFilter }
              : {}
          const normalizedCode = normalizeGeoipCountryCode(v)
          if (normalizedCode.length > 0) {
            fallbackFilter['geoip-code'] = normalizedCode
          } else {
            delete fallbackFilter['geoip-code']
          }
          if (Object.keys(fallbackFilter).length > 0) {
            dnsConfig['fallback-filter'] = fallbackFilter
          } else {
            delete dnsConfig['fallback-filter']
          }
          this.updateMetaJson(dnsConfig, 'dns')
        }
      },
    },
    dnsDefaultNameserver: {
      get(this: any) { return this.metaJson['dns']?.['default-nameserver'] ?? [] },
      set(this: any, v: string[]) {
        if (this.metaJson['dns']) {
          this.updateMetaJson({ ...this.metaJson['dns'], 'default-nameserver': v }, 'dns')
        }
      },
    },
    dnsFakeIpFilter: {
      get(this: any) { return this.metaJson['dns']?.['fake-ip-filter'] ?? [] },
      set(this: any, v: string[]) {
        if (this.metaJson['dns']) {
          this.updateMetaJson({ ...this.metaJson['dns'], 'fake-ip-filter': v }, 'dns')
        }
      },
    },
    dnsPreferH3: {
      get(this: any) { return this.metaJson['dns']?.['prefer-h3'] ?? false },
      set(this: any, v: boolean) {
        if (this.metaJson['dns']) {
          this.updateMetaJson({ ...this.metaJson['dns'], 'prefer-h3': v }, 'dns')
        }
      },
    },
    clashHostsEntries: {
      get(this: any) {
        return buildHostsEntryTexts(this.metaJson['hosts'])
      },
      set(this: any, v: string[]) {
        const hosts = normalizeHostsEntriesInput(v)
        if (Object.keys(hosts).length === 0) {
          this.updateMetaJson(null, 'hosts')
          return
        }
        this.updateMetaJson(hosts, 'hosts')
      },
    },
    // ===== Mihomo-Specific Settings =====
    unifiedDelay: {
      get(this: any) { return this.metaJson['unified-delay'] ?? false },
      set(this: any, v: boolean) { this.updateMetaJson(v, 'unified-delay') },
    },
    tcpConcurrent: {
      get(this: any) { return this.metaJson['tcp-concurrent'] ?? false },
      set(this: any, v: boolean) { this.updateMetaJson(v, 'tcp-concurrent') },
    },
    findProcessMode: {
      get(this: any) { return this.metaJson['find-process-mode'] ?? 'off' },
      set(this: any, v: string) { this.updateMetaJson(v, 'find-process-mode') },
    },
    storeSelected: {
      get(this: any) { return this.metaJson['profile']?.['store-selected'] ?? false },
      set(this: any, v: boolean) {
        const profile = this.metaJson['profile'] ?? {}
        this.updateMetaJson({ ...profile, 'store-selected': v }, 'profile')
      },
    },
    storeFakeIp: {
      get(this: any) { return this.metaJson['profile']?.['store-fake-ip'] ?? false },
      set(this: any, v: boolean) {
        const profile = this.metaJson['profile'] ?? {}
        this.updateMetaJson({ ...profile, 'store-fake-ip': v }, 'profile')
      },
    },
  },
}
