import { Dial } from "./dial"

export interface tls {
  id: number
  name: string
  certificateRecordId?: number
  server: iTls
  client: oTls
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

  delete (value.client as Record<string, unknown>).mihomo_use_fingerprint
  stripConflictingTlsFields(value)

  return value
}

const stripMihomoTlsFields = (value: tls): tls => {
  stripLegacyTlsFields(value)

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
