import { Dial } from "./dial"

export interface tls {
  id: number
  name: string
  mode?: MihomoTlsMode
  certificateRecordId?: number
  server: iTls
  client: oTls
}

export type MihomoTlsMode = 'tls' | 'reality' | 'shadow-tls' | 'restls' | 'jls'

export const defaultMihomoTlsRateLimit = 204800

// Wrapper destinations use host:port. Keep the UI normalization shared so
// ShadowTLS, Restls, and JLS handle domain/IPv4 and bracketed IPv6 alike.
export function normalizeMihomoTlsDestination(value: unknown): string {
  const raw = typeof value === 'string' ? value.trim() : ''
  if (raw === '') return ''

  const ipv6 = raw.match(/^\[([^\]]+)\](?::(\d+))?$/)
  if (ipv6 && ipv6[1].trim() !== '') {
    const host = ipv6[1].trim()
    return ipv6[2] ? `[${host}]:${ipv6[2]}` : `[${host}]:443`
  }

  const host = raw.match(/^([^:\s]+)(?::(\d+))?$/)
  if (host) {
    return host[2] ? `${host[1]}:${host[2]}` : `${host[1]}:443`
  }

  // Keep malformed values unchanged so the existing host:port validation can
  // report them instead of silently changing their meaning.
  return raw
}

export function mihomoTlsSniFromDestination(value: unknown): string {
  const destination = typeof value === 'string' ? value.trim() : ''
  const ipv6 = destination.match(/^\[([^\]]+)\](?::\d*)?$/)
  if (ipv6) return ipv6[1]

  const host = destination.match(/^([^:\s]+)(?::\d*)?$/)
  return host ? host[1] : ''
}

export interface mihomoShadowTlsUser {
  name?: string
  username?: string
  password: string
}

export interface mihomoShadowTlsServer {
  enable?: boolean
  version: number
  password?: string
  users?: mihomoShadowTlsUser[]
  handshake: {
    dest: string
    proxy?: string
  }
  strict_mode?: boolean
  wildcard_sni?: 'off' | 'authed' | 'all'
  handshake_for_server_name?: Record<string, {
    dest: string
    proxy?: string
  }>
}

export interface mihomoRestlsServer {
  enable?: boolean
  dest: string
  password: string
  restls_script?: string
  min_record_len?: number
  rate_limit?: number
  proxy?: string
}

export interface mihomoJlsUser {
  username: string
  password: string
}

export interface mihomoJlsServer {
  enable?: boolean
  users: mihomoJlsUser[]
  dest: string
  sni?: string
  alpn?: string[]
  proxy?: string
  rate_limit?: number
}

export interface iTls {
  enabled?: boolean
  server_name?: string
  alpn?: string[]
  min_version?: string
  max_version?: string
  cipher_suites?: string[]
  certificate?: string[]
  certificate_path?: string
  key?: string[]
  key_path?: string
  client_authentication?: string
  client_certificate?: string[]
  client_certificate_path?: string
  client_certificate_public_key_sha256?: string[]
  acme?: acme
  ech?: ech
  reality?: reality
  shadow_tls?: mihomoShadowTlsServer
  res_tls?: mihomoRestlsServer
  jls_config?: mihomoJlsServer
}

export interface acme {
  domain: string[]
  data_directory?: string
  default_server_name?: string
  email?: string
  provider?: string
  disable_http_challenge?: boolean
  disable_tls_alpn_challenge?: boolean
  alternative_http_port?: number
  alternative_tls_port?: number
  external_account?: {
    key_id: string
    mac_key: string
  }
  dns01_challenge?: {
    provider: string
    [key: string]: string
  }
}

export interface ech {
  enabled: boolean
  key?: string[]
  key_path?: string
}

interface realityHanshake extends Dial {
  server: string
  server_port: number
}

export interface reality {
  enabled: boolean
  handshake: realityHanshake
  private_key: string
  short_id: string[]
  max_time_difference?: string
}

export const defaultInTls: iTls = {
  alpn: ['h3', 'h2', 'http/1.1'],
  min_version: "1.2",
  max_version: "1.3",
  cipher_suites: [],
}

export interface oTls {
  enabled?: boolean
  disable_sni?: boolean
  server_name?: string
  insecure?: boolean
  fingerprint?: string
  include_server_certificate?: boolean
  include_server_fingerprint?: boolean
  alpn?: string[]
  min_version?: string
  max_version?: string
  cipher_suites?: string[]
  certificate?: string
  certificate_path?: string
  certificate_public_key_sha256?: string[]
  client_certificate?: string[]
  client_certificate_path?: string
  client_key?: string[]
  client_key_path?: string
  fragment?: boolean
  fragment_fallback_delay?: string
  record_fragment?: boolean
  ech?: {
    enabled: boolean
    pq_signature_schemes_enabled?: boolean
    dynamic_record_sizing_disabled?: boolean
    config?: string[],
    config_path?: string
  },
  store?: string
  tls_store?: string
  utls?: {
    enabled: boolean
    fingerprint: string
  },
  reality?: {
    enabled: boolean
    public_key: string
    short_id: string
  }
  shadow_tls_opts?: {
    version: number
    password?: string
  }
  restls_opts?: {
    password: string
    version_hint: 'tls12' | 'tls13'
    restls_script?: string
  }
  jls_opts?: {
    username: string
    password: string
  }
}

export const defaultOutTls: oTls = {
  alpn: ['h3', 'h2', 'http/1.1'],
  min_version: "1.2",
  max_version: "1.3",
  cipher_suites: [],
  utls: {
    enabled: true,
    fingerprint: "chrome",
  },
  reality: {
    enabled: true,
    public_key: "",
    short_id: "",
  },
  ech: {
    enabled: true,
    pq_signature_schemes_enabled: false,
    dynamic_record_sizing_disabled: false,
    config_path: "",
  }
}

export type TlsNamespace = 'default' | 'mihomo'

const cloneTlsConfig = (value?: tls | null): tls => {
  return JSON.parse(JSON.stringify(value ?? { id: 0, name: '', server: { enabled: true }, client: {} }))
}

const hasNonEmptyList = (value: unknown): value is string[] => {
  return Array.isArray(value) && value.some(item => typeof item === 'string' && item.trim().length > 0)
}

// Optional TLS SNI/ALPN controls use field presence as their enabled state in
// the editor. Keep empty input from crossing the storage/config boundary.
const sanitizeOptionalTlsFields = (target: Record<string, unknown>): void => {
  const rawSni = target.server_name
  if (rawSni !== undefined) {
    if (typeof rawSni !== 'string') {
      delete target.server_name
    } else {
      const sni = rawSni.trim()
      if (sni === '') delete target.server_name
      else target.server_name = sni
    }
  }

  const rawAlpn = target.alpn
  if (rawAlpn !== undefined) {
    if (!Array.isArray(rawAlpn)) {
      delete target.alpn
    } else {
      const alpn = rawAlpn
        .filter((item): item is string => typeof item === 'string')
        .map(item => item.trim())
        .filter(item => item.length > 0)
      if (alpn.length === 0) delete target.alpn
      else target.alpn = alpn
    }
  }
}

const stripConflictingTlsFields = (value: tls): tls => {
  value.server = value.server ?? {}
  value.client = value.client ?? {}

  if (hasNonEmptyList(value.client.certificate_public_key_sha256)) {
    delete value.client.certificate
    delete value.client.certificate_path
  }

  if (hasNonEmptyList(value.server.client_certificate_public_key_sha256)) {
    delete value.server.client_certificate
    delete value.server.client_certificate_path
  }

  return value
}

const stripLegacyTlsFields = (value: tls): tls => {
  value.server = value.server ?? {}
  value.client = value.client ?? {}

  sanitizeOptionalTlsFields(value.server as Record<string, unknown>)
  sanitizeOptionalTlsFields(value.client as Record<string, unknown>)
  delete (value.client as Record<string, unknown>).mihomo_use_fingerprint
  stripConflictingTlsFields(value)

  return value
}

const sanitizeMihomoShadowTlsServer = (value: mihomoShadowTlsServer | undefined): void => {
  if (!value) return

  const version = Number((value as any).version)
  if (version !== 3) {
    delete (value as any).strict_mode
    delete (value as any).wildcard_sni
  }
  if (version <= 1) {
    delete (value as any).handshake_for_server_name
  }

  if (version === 3 && value.wildcard_sni !== undefined && !['off', 'authed', 'all'].includes(String(value.wildcard_sni))) {
    delete value.wildcard_sni
  }

  const mappings = value.handshake_for_server_name
  if (mappings === undefined) return
  if (!mappings || typeof mappings !== 'object' || Array.isArray(mappings)) {
    delete value.handshake_for_server_name
    return
  }

  const normalized: Record<string, { dest: string, proxy?: string }> = {}
  for (const [rawName, rawValue] of Object.entries(mappings)) {
    const name = rawName.trim()
    if (!name || !rawValue || typeof rawValue !== 'object' || Array.isArray(rawValue)) continue
    const dest = typeof rawValue.dest === 'string' ? rawValue.dest.trim() : ''
    if (!dest) continue
    const proxy = typeof rawValue.proxy === 'string' ? rawValue.proxy.trim() : ''
    normalized[name] = proxy ? { dest, proxy } : { dest }
  }
  if (Object.keys(normalized).length > 0) {
    value.handshake_for_server_name = normalized
  } else {
    delete value.handshake_for_server_name
  }
}

const stripMihomoTlsFields = (value: tls): tls => {
  const mode = value.mode === 'reality' || value.mode === 'shadow-tls' || value.mode === 'restls' || value.mode === 'jls'
    ? value.mode
    : 'tls'
  const hadExplicitBlankClientSni = (mode === 'shadow-tls' || mode === 'restls' || mode === 'jls') &&
    Object.prototype.hasOwnProperty.call(value.client ?? {}, 'server_name') &&
    typeof value.client?.server_name === 'string' &&
    value.client.server_name.trim() === ''
  stripLegacyTlsFields(value)

  value.mode = mode
  if (hadExplicitBlankClientSni) value.client.server_name = ''
  if (mode !== 'shadow-tls') delete (value.server as any).shadow_tls
  if (mode !== 'restls') delete (value.server as any).res_tls
  if (mode !== 'jls') delete (value.server as any).jls_config
  if (mode === 'tls') {
    delete (value.server as any).reality
  } else if (mode === 'reality') {
    for (const key of ['certificate', 'certificate_path', 'key', 'key_path', 'acme', 'ech']) delete (value.server as any)[key]
    value.certificateRecordId = undefined
  } else {
    for (const key of ['certificate', 'certificate_path', 'key', 'key_path', 'client_authentication', 'client_certificate', 'client_certificate_path', 'client_certificate_public_key_sha256', 'reality', 'acme', 'ech']) delete (value.server as any)[key]
    value.certificateRecordId = undefined
  }

  // SNI belongs to the outer client TLS object, not any *-opts wrapper map.
  // Normalize payloads created by an older version of these editors before
  // they are sent to the API or rendered into subscriptions.
  const activeWrapperKey = mode === 'shadow-tls'
    ? 'shadow_tls_opts'
    : mode === 'restls'
      ? 'restls_opts'
      : mode === 'jls'
        ? 'jls_opts'
        : ''
  for (const key of ['shadow_tls_opts', 'restls_opts', 'jls_opts'] as const) {
    const opts = (value.client as any)[key]
    if (!opts || typeof opts !== 'object' || Array.isArray(opts)) continue
    if (key === activeWrapperKey && !Object.prototype.hasOwnProperty.call(value.client, 'server_name')) {
      const nestedSni = typeof opts.server_name === 'string' ? opts.server_name.trim() : ''
      if (nestedSni !== '') value.client.server_name = nestedSni
    }
    delete opts.server_name
  }
  if (activeWrapperKey && !Object.prototype.hasOwnProperty.call(value.client, 'server_name')) {
    const legacySni = typeof value.server.server_name === 'string' ? value.server.server_name.trim() : ''
    if (legacySni !== '') value.client.server_name = legacySni
  }
  if (activeWrapperKey) delete value.server.server_name

  const rawClientSni = value.client.server_name
  const hasClientSni = typeof rawClientSni === 'string'
  const clientSni = hasClientSni ? rawClientSni.trim() : ''
  const wrapperDestination = mode === 'shadow-tls'
    ? (value.server.shadow_tls as any)?.handshake?.dest
    : mode === 'restls'
      ? (value.server.res_tls as any)?.dest
      : mode === 'jls'
        ? (value.server.jls_config as any)?.dest
        : ''
  const derivedSni = mihomoTlsSniFromDestination(wrapperDestination)
  const jls = value.server.jls_config as any
  if (mode === 'jls' && jls && typeof jls === 'object' && !Array.isArray(jls)) {
    const serverSni = typeof jls.sni === 'string' ? jls.sni.trim() : ''
    const fallbackSni = hasClientSni ? derivedSni : (serverSni || derivedSni)
    const sni = clientSni || fallbackSni
    if (sni === '') {
      delete value.client.server_name
      delete jls.sni
    } else {
      value.client.server_name = sni
      jls.sni = sni
    }
  } else if (activeWrapperKey) {
    const sni = clientSni || derivedSni
    if (sni === '') delete value.client.server_name
    else value.client.server_name = sni
  }

  for (const key of ['shadow_tls_opts', 'restls_opts', 'jls_opts', 'reality', 'ech'] as const) {
    const keep = (mode === 'shadow-tls' && key === 'shadow_tls_opts') ||
      (mode === 'restls' && key === 'restls_opts') ||
      (mode === 'jls' && key === 'jls_opts') ||
      (mode === 'reality' && key === 'reality') ||
      (mode === 'tls' && key === 'ech')
    if (!keep) delete (value.client as any)[key]
  }

  const restls = value.server.res_tls as any
  if (restls) {
    if (restls.rate_limit == null || restls.rate_limit === '') delete restls.rate_limit
    if (restls.min_record_len == null || restls.min_record_len === '') delete restls.min_record_len
  }
  if (jls && (jls.rate_limit == null || jls.rate_limit === '')) delete jls.rate_limit

  sanitizeMihomoShadowTlsServer(value.server.shadow_tls)

  if (value.mode && value.mode !== 'tls') {
    value.certificateRecordId = undefined
  }

  delete value.server.min_version
  delete value.server.max_version
  delete value.server.cipher_suites
  delete value.server.client_authentication
  delete value.server.client_certificate
  delete value.server.client_certificate_path

  delete value.client.store
  delete value.client.tls_store
  delete value.client.certificate
  delete value.client.certificate_path
  delete value.server.client_certificate_public_key_sha256
  delete value.client.client_certificate
  delete value.client.client_certificate_path
  delete value.client.client_key
  delete value.client.client_key_path
  delete (value.client as Record<string, unknown>).fragment
  delete (value.client as Record<string, unknown>).fragment_fallback_delay
  delete (value.client as Record<string, unknown>).record_fragment

  return value
}

export const sanitizeMihomoTls = (value?: tls | null): tls => {
  return stripMihomoTlsFields(cloneTlsConfig(value))
}

export const sanitizeTlsForNamespace = (value?: tls | null, namespace: TlsNamespace | string = 'default'): tls => {
  const cloned = cloneTlsConfig(value)
  stripLegacyTlsFields(cloned)
  if (namespace === 'mihomo') {
    return stripMihomoTlsFields(cloned)
  }
  return cloned
}
