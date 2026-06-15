import HttpUtils from './httputil'

export interface PortRangeCheckItem {
  id: string
  tag: string
  range: string
}

export interface PortCheckRequest {
  single_ports?: number[]
  udp_ranges?: PortRangeCheckItem[]
}

export interface SinglePortStatus {
  port: number
  tcp: boolean
  udp: boolean
}

export interface UDPRangeStatus {
  id: string
  tag: string
  input: string
  normalized: string
  valid: boolean
  error?: string
  checked_port_count: number
  occupied_count: number
  occupied_ports: number[]
}

export interface PortCheckResponse {
  supported: boolean
  checked_at: number
  single: SinglePortStatus[]
  udp_ranges: UDPRangeStatus[]
}

export const PORT_RANGE_TEMPLATE = '2080-3000, 5000:6000, 55100'

export async function checkPortOccupancy(req: PortCheckRequest): Promise<PortCheckResponse | null> {
  // This endpoint binds JSON body on backend; force JSON to avoid form-encoding parse errors.
  const msg = await HttpUtils.post('api/portOccupancy', req, {
    headers: {
      'Content-Type': 'application/json'
    }
  })
  if (!msg.success || !msg.obj) return null
  return <PortCheckResponse>msg.obj
}
