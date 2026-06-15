export const TrspTypes = {
  HTTP: 'http',
  H2: 'h2',
  WebSocket: 'ws',
  QUIC: 'quic',
  gRPC: 'grpc',
  HTTPUpgrade: 'httpupgrade',
  XHTTP: 'xhttp',
}

export type TrspType = typeof TrspTypes[keyof typeof TrspTypes]

export type Transport = HTTP | H2 | WebSocket | QUIC | gRPC | HTTPUpgrade | XHTTP

interface TransportBasics {
  type: TrspType
}

export interface HTTP extends TransportBasics {
  host?: string[]
  path?: string
  method?: string
  headers?: {}
}

export interface H2 extends TransportBasics {
  host?: string[]
  path?: string
}

export interface WebSocket extends TransportBasics {
  path: string
  headers?: {
    Host: string
  }
  max_early_data?: number
  early_data_header_name?: string
  v2ray_http_upgrade?: boolean
  v2ray_http_upgrade_fast_open?: boolean
}

export interface QUIC extends TransportBasics {}

export interface gRPC extends TransportBasics {
  service_name?: string
  grpc_user_agent?: string
  idle_timeout?: string
  ping_timeout?: string
  ping_interval?: number
  max_connections?: number
  min_streams?: number
  max_streams?: number
  permit_without_stream?: boolean
}

export interface HTTPUpgrade extends TransportBasics {
  host?: string
  path?: string
  headers?: {}
}

export interface XHTTPReuseSettings {
  max_connections?: string
  max_concurrency?: string
  c_max_reuse_times?: string
  h_max_request_times?: string
  h_max_reusable_secs?: string
}

export interface XHTTPDownloadSettings {
  path?: string
  host?: string
  headers?: {}
  no_grpc_header?: boolean
  x_padding_bytes?: string
  sc_max_each_post_bytes?: number
  reuse_settings?: XHTTPReuseSettings
}

export interface XHTTP extends TransportBasics {
  path?: string
  host?: string
  mode?: 'auto' | 'stream-one' | 'stream-up' | 'packet-up'
  headers?: {}
  no_grpc_header?: boolean
  x_padding_bytes?: string
  sc_max_each_post_bytes?: number
  reuse_settings?: XHTTPReuseSettings
  download_settings?: XHTTPDownloadSettings
}
