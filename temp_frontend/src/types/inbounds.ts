import { iMultiplex, oMultiplex } from "./multiplex"
import { iTls } from "./tls"
import { Dial } from "./dial"
import { Transport } from "./transport"

export const InTypes = {
  Direct: 'direct',
  Mixed: 'mixed',
  SOCKS: 'socks',
  HTTP: 'http',
  Snell: 'snell',
  Shadowsocks: 'shadowsocks',
  VMess: 'vmess',
  Trojan: 'trojan',
  Naive: 'naive',
  Hysteria: 'hysteria',
  ShadowTLS: 'shadowtls',
  TUIC: 'tuic',
  Hysteria2: 'hysteria2',
  TrustTunnel: 'trusttunnel',
  VLESS: 'vless',
  AnyTls: 'anytls',
  SSH: 'ssh',
  Mieru: 'mieru',
  Sudoku: 'sudoku',
  Tun: 'tun',
  Redirect: 'redirect',
  TProxy: 'tproxy',
}

type InType = typeof InTypes[keyof typeof InTypes]

export interface Addr {
  server: string
  server_port: number
  tls?: boolean
  insecure?: boolean
  server_name?: string
  remark?: string
}

export interface InboundUserManagement {
  selectable: boolean
  uses_users_field: boolean
  mode: string
  identity_type: string
  reason: string
}

export interface Listen {
  listen: string
  listen_port: number
  tcp_fast_open?: boolean
  tcp_multi_path?: boolean
  udp_fragment?: boolean
  udp_timeout?: string
  detour?: string
}

interface InboundBasics extends Listen {
  id: number
  type: InType
  tag: string
  tls_id: number
  addrs?: Addr[]
  out_json?: any
  user_management?: InboundUserManagement
}

interface ShadowTLSHandShake extends Dial {
  server: string
  server_port: number
}

export interface Direct extends InboundBasics {
  network?: "udp" | "tcp"
  override_address?: string
  override_port?: number
}
export interface Mixed extends InboundBasics {}
export interface SOCKS extends InboundBasics {}
export interface HTTP extends InboundBasics {}
export interface Snell extends InboundBasics {
  version?: 4 | 5
  udp?: boolean
  obfs_opts?: {
    mode?: "http" | "tls"
    host?: string
  }
}
export interface Shadowsocks extends InboundBasics {
  method: string
  password: string
  network?: "udp" | "tcp"
  multiplex?: iMultiplex
  managed?: boolean
}
export interface VMess extends InboundBasics {
  tls: iTls
  multiplex?: iMultiplex
  transport?: Transport
}
export interface Trojan extends InboundBasics {
  tls: iTls
  fallback?: {
    server: string
    server_port: number
  }
  multiplex?: iMultiplex
  transport?: Transport
}
export interface Naive extends InboundBasics {
  tls: iTls,
  network?: "tcp" | "udp"
  quic_congestion_control?: string
  naive_quic_congestion_control_omit?: boolean
}
export interface Hysteria extends InboundBasics {
  server_up_mbps?: number
  server_down_mbps?: number
  obfs?: string
  stream_receive_window?: number
  connection_receive_window?: number
  max_concurrent_streams?: number
  disable_path_mtu_discovery?: boolean
  // 端口跳跃（服务端自定义字段，不传给 sing-box）
  port_hop_range?: string
  port_hop_interval?: string
}
// Shadowsocks 内部配置（用于 ShadowTLS 组合）
// 使用 oMultiplex 因为这些设置同时用于客户端订阅生成
export interface ShadowsocksConfig {
  method: string
  password?: string
  network?: "udp" | "tcp"
  udp_over_tcp?: {
    enabled: boolean
    version?: number
  }
  multiplex?: oMultiplex
}

export interface ShadowTLS extends InboundBasics {
  version: 1|2|3
  password?: string
  handshake: ShadowTLSHandShake
  handshake_for_server_name?: {
    [server_name: string]: ShadowTLSHandShake
  }
  strict_mode?: boolean
  wildcard_sni?: string
  // Shadowsocks 配置，用于生成组合的入站
  ss_config?: ShadowsocksConfig
}
export interface VLESS extends InboundBasics {
  multiplex?: iMultiplex
  transport?: Transport
  tls: iTls
  vless_encryption_enabled?: boolean
  vless_encryption_auth_method?: "x25519" | "mlkem768"
  vless_encryption_mode?: "native" | "xorpub" | "random"
  vless_encryption_server_rtt?: string
  vless_encryption_client_rtt?: "1rtt" | "0rtt"
  vless_encryption_rtt?: "600s" | "300-600s" | "1rtt" | "0rtt" | "0s"
  vless_encryption_padding?: string
  vless_encryption_x25519_private_key?: string
  vless_encryption_x25519_password?: string
  vless_encryption_mlkem_seed?: string
  vless_encryption_mlkem_client?: string
}

export interface AnyTls extends InboundBasics {
  padding_scheme?: string
  tls: iTls
}
export interface SSH extends InboundBasics {
  username?: string
  user?: string
  password?: string
  private_key?: string
  private_key_passphrase?: string
  host_key?: string[]
  host_key_algorithms?: string[]
}
export interface Mieru extends InboundBasics {
  transport: "TCP" | "UDP"
  port_bindings?: string
  port_range?: string
  user_hint_is_mandatory?: boolean
}
export interface SudokuHTTPMaskInbound {
  disable?: boolean
  mode?: "legacy" | "stream" | "poll" | "auto" | "ws"
  path_root?: string
}
export interface Sudoku extends InboundBasics {
  key?: string
  aead_method?: "chacha20-poly1305" | "aes-128-gcm" | "none"
  padding_min?: number
  padding_max?: number
  table_type?: "prefer_ascii" | "prefer_entropy"
  custom_table?: string
  custom_tables?: string[]
  handshake_timeout?: number
  enable_pure_downlink?: boolean
  httpmask?: SudokuHTTPMaskInbound
  fallback?: string
  disable_http_mask?: boolean
}
export interface TUIC extends InboundBasics {
  congestion_control: ""|"cubic"|"new_reno"|"bbr"
  auth_timeout?: string
  max_idle_time?: string
  max_udp_relay_packet_size?: number
  cwnd?: number
  zero_rtt_handshake?: boolean
  heartbeat?: string
}
export interface Hysteria2 extends InboundBasics {
  server_up_mbps?: number
  server_down_mbps?: number
  bbr_profile?: "" | "conservative" | "standard" | "aggressive"
  obfs?: {
    type?: "salamander"
    password: string
  }
  ignore_client_bandwidth?: boolean
  masquerade?: string | {
    type: string
    directory?: string
    url?: string
    rewrite_host?: boolean
    status_code?: number
    headers?: Headers[]
    content?: string
  }
  brutal_debug?: boolean
  // 端口跳跃（服务端自定义字段，不传给 sing-box）
  port_hop_range?: string
  port_hop_interval?: string
  port_hop_interval_max?: string
}
export interface TrustTunnel extends InboundBasics {
  proxy?: string
  network?: Array<"tcp" | "udp">
  congestion_controller?: "" | "cubic" | "new_reno" | "bbr"
  max_connections?: number
  min_streams?: number
  max_streams?: number
  client_auth_type?: "" | "request" | "require-any" | "verify-if-given" | "require-and-verify"
  client_auth_cert?: string
  ech_key?: string
}
export interface Tun extends InboundBasics {
  interface_name?: string
  address?: string[]
  mtu?: number
  endpoint_independent_nat?: boolean
  udp_timeout?: string
  stack?: string
  auto_route?: boolean
  strict_route?: boolean
  // iproute2_table_index?: number
  // iproute2_rule_index?: number
  auto_redirect?: boolean
  // auto_redirect_input_mark?: string
  // auto_redirect_output_mark?: string
  // route_address?: string[]
  // route_exclude_address?: string[]
  // include_interface?: string[]
  // exclude_interface?: string[]
  // include_uid?: string[]
  // include_uid_range?: string[]
  // exclude_uid?: number[]
  // exclude_uid_range?: string[]
  // include_android_user?: number[]
  // include_package?: string[]
  // exclude_package?: string[]
}
export interface Redirect extends InboundBasics {}
export interface TProxy extends InboundBasics {
  network?: "udp" | "tcp"
}

// Create interfaces dynamically based on InTypes keys
type InterfaceMap = {
  direct: Direct
  mixed: Mixed
  socks: SOCKS
  http: SOCKS
  snell: Snell
  shadowsocks: Shadowsocks
  vmess: VMess
  trojan: Trojan
  naive: Naive
  hysteria: Hysteria
  shadowtls: ShadowTLS
  tuic: TUIC
  hysteria2: Hysteria2
  trusttunnel: TrustTunnel
  vless: VLESS
  anytls: AnyTls
  ssh: SSH
  mieru: Mieru
  sudoku: Sudoku
  tun: Tun
  redirect: Redirect
  tproxy: TProxy
}

// Create union type from InterfaceMap
export type Inbound = InterfaceMap[keyof InterfaceMap]

function cloneDefaultInbound<T>(value: T): T {
  if (value === undefined || value === null) return value
  if (typeof structuredClone === 'function') {
    return structuredClone(value)
  }
  return JSON.parse(JSON.stringify(value)) as T
}

// Create defaultValues object dynamically
const defaultValues: Record<InType, Inbound> = {
  direct: <Direct>{ type: InTypes.Direct },
  mixed: <Mixed>{ type: InTypes.Mixed },
  socks: <SOCKS>{ type: InTypes.SOCKS },
  http: <HTTP>{ type: InTypes.HTTP, tls_id: 0 },
  snell: <Snell>{
    type: InTypes.Snell,
    version: 5,
    udp: true,
    obfs_opts: {
      mode: undefined,
      host: 'www.bing.com',
    },
    out_json: {
      version: 5,
      reuse: false,
      obfs_opts: {
        mode: undefined,
        host: 'www.bing.com',
      },
    },
  },
  // 开发者要求隐藏 SS API 专用开关，默认关闭 managed。
  // Developer requirement: hide SS API-only toggle and keep managed disabled by default.
  // 说明 / Note: 不影响常规 SS/SS2022 节点创建与使用 / does not affect regular SS/SS2022 node creation or usage.
  shadowsocks: <Shadowsocks>{ type: InTypes.Shadowsocks, method: 'none', multiplex: {}, managed: false },
  vmess: <VMess>{ type: InTypes.VMess, tls_id: 0, multiplex: {}, transport: {} },
  trojan: <Trojan>{ type: InTypes.Trojan, tls_id: 0, multiplex: {}, transport: {} },
  naive: <Naive>{ type: InTypes.Naive, tls_id: 0, quic_congestion_control: 'bbr2' },
  hysteria: <Hysteria>{
    type: InTypes.Hysteria,
    server_up_mbps: 2000,
    server_down_mbps: 2000,
    stream_receive_window: 25000000,
    connection_receive_window: 99000000,
    tls_id: 0,
  },
  shadowtls: <ShadowTLS>{ type: InTypes.ShadowTLS, version: 3, handshake: { server: 'addons.mozilla.org', server_port: 443 }, handshake_for_server_name: {}, strict_mode: true, ss_config: { method: '2022-blake3-aes-128-gcm', network: 'tcp', udp_over_tcp: { enabled: true, version: 2 }, multiplex: { enabled: true, protocol: 'smux', max_connections: 250, max_streams: 8, padding: true } } },
  tuic: <TUIC>{ type: InTypes.TUIC, congestion_control: "cubic", tls_id: 0 },
  hysteria2: <Hysteria2>{ type: InTypes.Hysteria2, tls_id: 0, server_up_mbps: 2000, server_down_mbps: 2000 },
  trusttunnel: <TrustTunnel>{ type: InTypes.TrustTunnel, tls_id: 0, network: ['tcp'], congestion_controller: "bbr" },
  vless: <VLESS>{ type: InTypes.VLESS, tls_id: 0, multiplex: {}, transport: {} },
  anytls: <AnyTls>{ type: InTypes.AnyTls, tls_id: 0, padding_scheme: "stop=8\n0=30-30\n1=100-400\n2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000\n3=9-9,500-1000\n4=500-1000\n5=500-1000\n6=500-1000\n7=500-1000" },
  ssh: <SSH>{ type: InTypes.SSH },
  mieru: <Mieru>{ type: InTypes.Mieru, transport: "TCP", user_hint_is_mandatory: true },
  sudoku: <Sudoku>{
    type: InTypes.Sudoku,
    aead_method: 'chacha20-poly1305',
    padding_min: 1,
    padding_max: 15,
    table_type: 'prefer_ascii',
    handshake_timeout: 5,
    enable_pure_downlink: false,
    httpmask: {
      disable: false,
      mode: 'legacy',
    },
    disable_http_mask: false,
  },
  tun: <Tun>{ type: InTypes.Tun, mtu: 9000, stack: 'system', udp_timeout: '5m', auto_route: false },
  redirect: <Redirect>{ type: InTypes.Redirect },
  tproxy: <TProxy>{ type: InTypes.TProxy },
}

export function createInbound<T extends Inbound>(type: InType,json?: Partial<T>): Inbound {
  const defaultObject: Inbound = { ...(cloneDefaultInbound(defaultValues[type] ?? {} as Inbound)), ...(json ?? {}) }
  return defaultObject
}
