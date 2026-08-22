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
  listenProtocol: 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  listenProtocolAlias?: 'ws' | 'wss' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp' | ''
  listenPort: number
  listenCompressionEnabled: boolean
  listenCompressionAlgorithms: string[]
  hosts: string[]
  pathPrefix: string
  listenDnsPath?: string
  targetProtocol: 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  targetProtocolAlias?: 'ws' | 'wss' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp' | ''
  targetAddresses: string[]
  targetPort: number
  targetCompressionEnabled: boolean
  targetCompressionAlgorithms: string[]
  targetPath: string
  targetDnsPath?: string
  fallbackDnsUpstreams: string
  dnsUpstreamTimeoutSeconds: number
  dnsCacheEnabled: boolean
  dnsCacheSizeBytes: number
  dnsCacheMinTtl: number
  dnsCacheMaxTtl: number
  dnsAllowedCidrs: string[]
  dnsRateLimitQps: number
  dnsMaxConcurrentQueries: number
  ednsEnabled: boolean
  ednsMode: 'auto' | 'custom'
  ednsCustomIp: string
  ednsClientSubnetPolicy: 'client_ip' | 'prefer_request_public'
  disableIpv4Answer: boolean
  disableIpv6Answer: boolean
  certificateRecordIds: number[]
  certificateRecordId: number
  certificateLabel: string
  certificateLabels?: string[]
  listenHttpVersionStrategy: '' | 'h2_h3' | 'h2_only' | 'h3_only'
  ipStrategy: 'ipv4_only' | 'ipv6_only' | 'prefer_ipv4' | 'prefer_ipv6'
  httpVersionStrategy: '' | 'h2_only' | 'h3_only' | 'prefer_h2' | 'prefer_h3' | 'dual_required_prefer_h3'
  upstreamTlsVerify: boolean
  maxConcurrentConnections: number
  maxConcurrentRequests: number
  upstreamMaxConnections: number
  upstreamMaxIdleConnections: number
  memoryLimitBytes: number
  apiPassthrough: boolean
  advertiseHttp3: boolean
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
  listenProtocol: 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  listenPort: number
  listenCompressionEnabled: boolean
  listenCompressionAlgorithms: string[]
  hostsText: string
  pathPrefix: string
  listenDnsPath: string
  targetProtocol: 'http' | 'https' | 'h2' | 'h3' | 'ws' | 'wss' | 'dns_doh' | 'dns_doh3' | 'dns_doq' | 'dns_dot' | 'dns_udp' | 'dns_tcp'
  targetAddressesText: string
  targetPort: number
  targetCompressionEnabled: boolean
  targetCompressionAlgorithms: string[]
  targetPath: string
  targetDnsPath: string
  fallbackDnsUpstreams: string
  dnsUpstreamTimeoutSeconds: number
  dnsCacheEnabled: boolean
  dnsCacheSizeBytes: number
  dnsCacheMinTtl: number
  dnsCacheMaxTtl: number
  dnsAllowedCidrsText: string
  dnsRateLimitQps: number
  dnsMaxConcurrentQueries: number
  ednsEnabled: boolean
  ednsMode: 'auto' | 'custom'
  ednsCustomIp: string
  ednsClientSubnetPolicy: 'client_ip' | 'prefer_request_public'
  disableIpv4Answer: boolean
  disableIpv6Answer: boolean
  certificateRecordIds: number[]
  listenHttpVersionStrategy: '' | 'h2_h3' | 'h2_only' | 'h3_only'
  ipStrategy: 'ipv4_only' | 'ipv6_only' | 'prefer_ipv4' | 'prefer_ipv6'
  httpVersionStrategy: '' | 'h2_only' | 'h3_only' | 'prefer_h2' | 'prefer_h3' | 'dual_required_prefer_h3'
  upstreamTlsVerify: boolean
  maxConcurrentConnections: number
  maxConcurrentRequests: number
  upstreamMaxConnections: number
  upstreamMaxIdleConnections: number
  memoryLimitBytes: number
  apiPassthrough: boolean
  advertiseHttp3: boolean
  remark: string
}

export type ReverseProxyOverview = {
	revision: number
	resourceSettings: ReverseProxyResourceSettings
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

export type ReverseProxyResourceSettings = {
  listenerConnectionLimit: number
  globalHttpMaxConcurrent: number
  globalDnsMaxConcurrent: number
  http2MaxConcurrentStreams: number
  quicMaxIncomingStreams: number
  defaultUpstreamMaxIdleConnections: number
  memoryPoolBytes: number
  defaultRuleMemoryLimitBytes: number
  responseRewriteInputBytes: number
  responseRewriteOutputBytes: number
  responseRewriteMaxConcurrent: number
}

export type ReverseProxyRuntimeRuleState = {
  id: number
  runtimeStatus: string
  lastError: string
  localConnectionCount: number
  upstreamConnectionCount: number
}

export type ReverseProxyRuntimeOverview = {
  revision: number
  available: boolean
  started: boolean
  listenerCount: number
  lastSyncAt: number
  rules: ReverseProxyRuntimeRuleState[]
  resources: {
    activeHttpRequests: number
    activeDnsQueries: number
    memoryUsedBytes: number
    cacheUsedBytes: number
    rewriteUsedBytes: number
  }
  warnings?: string[]
  error?: string
}
