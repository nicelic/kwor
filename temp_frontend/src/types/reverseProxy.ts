export type ReverseProxyCertificateOption = {
  id: number
  displayId: number
  mainDomain: string
  domains: string[]
  notAfter: number
  status: string
}

export type ReverseProxyRule = {
  id: number
  displayId: number
  listOrder: number
  name: string
  enabled: boolean
  listenProtocol: 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss'
  listenProtocolAlias?: 'ws' | 'wss' | ''
  listenIP: string
  listenIPs: string[]
  listenPort: number
  hosts: string[]
  pathPrefix: string
  targetProtocol: 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss'
  targetProtocolAlias?: 'ws' | 'wss' | ''
  targetAddresses: string[]
  targetPort: number
  targetPath: string
  certificateRecordIds: number[]
  certificateRecordId: number
  certificateLabel: string
  certificateLabels?: string[]
  listenHttpVersionStrategy: '' | 'h2_h3' | 'h2_only' | 'h3_only'
  ipStrategy: 'ipv4_only' | 'ipv6_only' | 'prefer_ipv4' | 'prefer_ipv6'
  httpVersionStrategy: '' | 'h2_only' | 'h3_only' | 'prefer_h2' | 'prefer_h3' | 'dual_required_prefer_h3'
  upstreamTlsVerify: boolean
  apiPassthrough: boolean
  remark: string
  lastError: string
  runtimeStatus: string
  localConnectionCount: number
  upstreamConnectionCount: number
  certificateHints?: string[]
  updatedAt: number
  createdAt: number
}

export type ReverseProxyRuleForm = {
  id: number
  displayId: number
  name: string
  enabled: boolean
  listenProtocol: 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss'
  listenPort: number
  hostsText: string
  pathPrefix: string
  targetProtocol: 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss'
  targetAddressesText: string
  targetPort: number
  targetPath: string
  certificateRecordIds: number[]
  listenHttpVersionStrategy: '' | 'h2_h3' | 'h2_only' | 'h3_only'
  ipStrategy: 'ipv4_only' | 'ipv6_only' | 'prefer_ipv4' | 'prefer_ipv6'
  httpVersionStrategy: '' | 'h2_only' | 'h3_only' | 'prefer_h2' | 'prefer_h3' | 'dual_required_prefer_h3'
  upstreamTlsVerify: boolean
  apiPassthrough: boolean
  remark: string
}

export type ReverseProxyOverview = {
  available: boolean
  started: boolean
  listenerCount: number
  enabledCount: number
  ruleCount: number
  certificateCount: number
  lastSyncAt: number
  certificates: ReverseProxyCertificateOption[]
  rules: ReverseProxyRule[]
  warnings?: string[]
  error?: string
}
