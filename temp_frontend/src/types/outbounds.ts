import { oTls } from "./tls"
import { oMultiplex } from "./multiplex"
import { Transport } from "./transport"
import { Dial } from "./dial"

export const OutTypes = {
  Direct: 'direct',
  SOCKS: 'socks',
  HTTP: 'http',
  Snell: 'snell',
  Shadowsocks: 'shadowsocks',
  VMess: 'vmess',
  Trojan: 'trojan',
  Hysteria: 'hysteria',
  VLESS: 'vless',
  ShadowTLS: 'shadowtls',
  TUIC: 'tuic',
  Hysteria2: 'hysteria2',
  AnyTls: 'anytls',
  Mieru: 'mieru',
  Sudoku: 'sudoku',
  TrustTunnel: 'trusttunnel',
  Tor: 'tor',
  SSH: 'ssh',
  Selector: 'selector',
  URLTest: 'urltest',
}

type OutType = typeof OutTypes[keyof typeof OutTypes]

interface OutboundBasics {
  id: number
  type: OutType
  tag: string
}

interface ShadowTLSHandshake extends Dial {
  server: string
  server_port: number
}

export interface WgPeer {
  server: string
  server_port: number
  public_key: string
  pre_shared_key?: string
  allowed_ips?: string[]
  reserved?: number[]
}

export interface Direct extends OutboundBasics, Dial {}

export interface SOCKS extends OutboundBasics, Dial {
  server: string
  server_port: number
  version?: "4" | "4a" | "5"
  username?: string
  password?: string
  network?: "udp" | "tcp"
  udp_over_tcp?: false | {
    enabled: true
    version?: number
  }
}

export interface HTTP extends OutboundBasics, Dial {
  server: string
  server_port: number
  username?: string
  password?: string
  path?: string
  headers?: {
    [key: string]: string
  }
  tls?: oTls
}

export interface Snell extends OutboundBasics, Dial {
  server: string
  server_port: number
  psk: string
  version?: 1 | 2 | 3 | 4 | 5
  udp?: boolean
  reuse?: boolean
  obfs_opts?: {
    mode?: "http" | "tls"
    host?: string
  }
}

export interface Shadowsocks extends OutboundBasics, Dial {
  server: string
  server_port: number
  method: string
  password: string
  network?: "udp" | "tcp"
  udp_over_tcp?: false | {
    enabled: true
    version?: number
  }
  multiplex?: oMultiplex
}

export interface VMESS extends OutboundBasics, Dial {
  server: string
  server_port: number
  uuid: string
  security?: string
  alter_id: 0
  global_padding?: boolean
  authenticated_length?: boolean
  network?: "udp" | "tcp"
  packet_encoding?: string
  tls?: oTls
  multiplex?: oMultiplex
  transport?: Transport
}

export interface Trojan extends OutboundBasics, Dial {
  server: string
  server_port: number
  password: string
  network?: "udp" | "tcp"
  tls?: oTls
  multiplex?: oMultiplex
  transport?: Transport
}

export interface Hysteria extends OutboundBasics, Dial {
  server: string
  server_port: number
  server_ports?: string[]
  hop_interval?: string
  mihomo_fast_open?: boolean
  up_mbps: number
  down_mbps: number
  obfs?: string
  auth_str?: string
  stream_receive_window?: number
  connection_receive_window?: number
  max_concurrent_streams?: number
  disable_path_mtu_discovery?: boolean
  network?: "udp" | "tcp"
  tls: oTls
}

// Shadowsocks 出站配置（用于 ShadowTLS 组合）
export interface ShadowsocksOutConfig {
  method: string
  password?: string
  network?: "udp" | "tcp"
  udp_over_tcp?: boolean | { enabled: boolean; version?: number }
  multiplex?: oMultiplex
}

export interface ShadowTLS extends OutboundBasics, Dial {
  server: string
  server_port: number
  version: 1|2|3
  password?: string
  handshake?: ShadowTLSHandshake
  strict_mode?: boolean
  wildcard_sni?: string
  tls: oTls
  // Shadowsocks 配置，用于生成组合的出站
  ss_config?: ShadowsocksOutConfig
}

export interface VLESS extends OutboundBasics, Dial {
  server: string
  server_port: number
  uuid: string
  flow?: string
  network?: "udp" | "tcp"
  packet_encoding?: string
  tls?: oTls
  multiplex?: oMultiplex
  transport?: Transport
}

export interface TUIC extends OutboundBasics, Dial {
  server: string
  server_port: number
  token?: string
  uuid?: string
  password?: string
  mihomo_fast_open?: boolean
  congestion_control?: "cubic"|"new_reno"|"bbr"
  udp_relay_mode?: "native" | "quic"
  udp_over_stream?: boolean
  udp_over_stream_version?: number
  zero_rtt_handshake?: boolean
  heartbeat?: string
  request_timeout?: string
  max_open_streams?: number
  max_udp_relay_packet_size?: number
  cwnd?: number
  ip?: string
  disable_mtu_discovery?: boolean
  max_datagram_frame_size?: number
  network?: "udp" | "tcp"
  tls: oTls
}

export interface Hysteria2 extends OutboundBasics, Dial {
  server: string
  server_port: number
  server_ports?: string[]
  hop_interval?: string
  hop_interval_max?: string
  mihomo_fast_open?: boolean
  up_mbps?: number
  down_mbps?: number
  bbr_profile?: "" | "conservative" | "standard" | "aggressive"
  obfs?: {
    type?: "salamander"
    password: string
  }
  password?: string
  network?: "udp" | "tcp"
  tls: oTls
  brutal_debug?: boolean
}

export interface AnyTls extends OutboundBasics, Dial {
  server: string
  server_port: number
  password: string
  idle_session_check_interval: string
  idle_session_timeout: string
  min_idle_session: number
  tls: oTls
}

export interface Mieru extends OutboundBasics, Dial {
	server: string
	server_port?: number
	port_range?: string
  transport: "TCP" | "UDP"
  udp?: boolean
  username: string
  password: string
  multiplexing?: "MULTIPLEXING_OFF" | "MULTIPLEXING_LOW" | "MULTIPLEXING_MIDDLE" | "MULTIPLEXING_HIGH"
	handshake_mode?: "HANDSHAKE_STANDARD" | "HANDSHAKE_NO_WAIT"
}

export interface SudokuHTTPMaskOutbound {
  disable?: boolean
  mode?: "legacy" | "stream" | "poll" | "auto" | "ws"
  tls?: boolean
  host?: string
  path_root?: string
  multiplex?: "off" | "auto" | "on"
}

export interface Sudoku extends OutboundBasics, Dial {
  server: string
  server_port: number
  key?: string
  aead_method?: "chacha20-poly1305" | "aes-128-gcm" | "none"
  padding_min?: number
  padding_max?: number
  table_type?: "prefer_ascii" | "prefer_entropy"
  custom_table?: string
  custom_tables?: string[]
  enable_pure_downlink?: boolean
  httpmask?: SudokuHTTPMaskOutbound
}

export interface TrustTunnel extends OutboundBasics, Dial {
  server: string
  server_port: number
  username?: string
  password?: string
  max_connections?: number
  min_streams?: number
  max_streams?: number
  udp?: boolean
  health_check?: boolean
  congestion_controller?: "" | "cubic" | "new_reno" | "bbr"
  quic?: boolean
  tls?: oTls
}

export interface Tor extends OutboundBasics, Dial {
  executable_path?: string
  extra_args?: string[]
  data_directory: string
  torrc?: {
    [options: string]: string
  }
}

export interface SSH extends OutboundBasics, Dial  {
  server: string
  server_port?: number
  username?: string
  user?: string
  password?: string
  private_key?: string
  private_key_path?: string
  private_key_passphrase?: string
  host_key?: string[]
  host_key_algorithms?: string[]
  client_version?: string
  cipher?: string[]
  mac?: string[]
  kex_algorithm?: string[]
}

export interface Selector extends OutboundBasics {
  outbounds: string[]
  default?: string
  interrupt_exist_connections?: boolean
}

export interface URLTest extends OutboundBasics {
  outbounds: string[]
  url?: string
  interval?: string
  tolerance?: number
  idle_timeout?: string
  interrupt_exist_connections?: boolean
}

// Create interfaces dynamically based on OutTypes keys
type InterfaceMap = {
  [Key in keyof typeof OutTypes]: {
    type: string
    [otherProperties: string]: any // You can add other properties as needed
  }
}

// Create union type from InterfaceMap
export type Outbound = InterfaceMap[keyof InterfaceMap]

function cloneDefaultOutbound<T>(value: T): T {
  if (value === undefined || value === null) return value
  if (typeof structuredClone === 'function') {
    return structuredClone(value)
  }
  return JSON.parse(JSON.stringify(value)) as T
}

// Create defaultValues object dynamically
const defaultValues: Record<OutType, Outbound> = {
  direct: { type: OutTypes.Direct },
  socks: { type: OutTypes.SOCKS, version: "5" },
  http: { type: OutTypes.HTTP, tls: {} },
  snell: {
    type: OutTypes.Snell,
    version: 5,
    udp: true,
    reuse: false,
    obfs_opts: {
      mode: undefined,
      host: 'www.bing.com',
    },
  },
  shadowsocks: { type: OutTypes.Shadowsocks, method: 'none', multiplex: {} },
  vmess: { type: OutTypes.VMess, tls: {}, multiplex: {}, transport: {}, security: 'auto', global_padding: false },
  trojan: { type: OutTypes.Trojan, tls: {}, multiplex: {}, transport: {} },
  hysteria: {
    type: OutTypes.Hysteria,
    up_mbps: 100,
    down_mbps: 100,
    stream_receive_window: 25000000,
    connection_receive_window: 67108864,
    tls: { enabled: true },
  },
  shadowtls: { type: OutTypes.ShadowTLS, version: 3, strict_mode: true, wildcard_sni: 'off', handshake: { server: 'addons.mozilla.org', server_port: 443 }, tls: { enabled: true }, ss_config: { method: '2022-blake3-aes-128-gcm', network: 'tcp', udp_over_tcp: false, multiplex: { enabled: true, protocol: 'h2mux', max_connections: 8, min_streams: 16, padding: true } } },
  vless: { type: OutTypes.VLESS, tls: {}, multiplex: {}, transport: {} },
  tuic: { type: OutTypes.TUIC, congestion_control: 'cubic', tls: { enabled: true } },
  hysteria2: { type: OutTypes.Hysteria2, tls: { enabled: true } },
  anytls: { type: OutTypes.AnyTls, tls: { enabled: true } },
  mieru: { type: OutTypes.Mieru, transport: 'TCP', udp: true, multiplexing: 'MULTIPLEXING_LOW', handshake_mode: 'HANDSHAKE_STANDARD' },
  sudoku: {
    type: OutTypes.Sudoku,
    aead_method: 'chacha20-poly1305',
    padding_min: 1,
    padding_max: 15,
    table_type: 'prefer_ascii',
    enable_pure_downlink: false,
    httpmask: {
      disable: false,
      mode: 'legacy',
      tls: true,
      multiplex: 'off',
    },
  },
  trusttunnel: { type: OutTypes.TrustTunnel, tls: { enabled: true }, udp: false, health_check: false, congestion_controller: 'bbr' },
  tor: { type: OutTypes.Tor, executable_path: './tor', data_directory: '$HOME/.cache/tor', torrc: { ClientOnly: '1' } },
  ssh: { type: OutTypes.SSH },
  selector: { type: OutTypes.Selector },
  urltest: { type: OutTypes.URLTest },
}

export function createOutbound<T extends Outbound>(type: string,json?: Partial<T>): Outbound {
  const defaultObject: Outbound = { ...cloneDefaultOutbound(defaultValues[type]), ...(json || {}) }
  return defaultObject
}
