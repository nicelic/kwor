export function normalizePortRangeInput(raw: string): string[] {
  if (typeof raw !== 'string') return []
  return raw
    .replace(/\uFF0C/g, ',')
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part.length > 0)
    .map((part) => part.replace(/\s+/g, '').replace(/-/g, ':'))
}

export function parseServerPortInput(raw: string): number | undefined {
  if (typeof raw !== 'string') return undefined
  const input = raw.trim()
  if (input === '') return undefined
  const port = Number.parseInt(input, 10)
  return Number.isNaN(port) ? undefined : port
}

export function pickPrimaryPort(serverPorts: string[], fallback?: number): number | undefined {
  for (const item of serverPorts) {
    const port = Number.parseInt(item.split(':')[0], 10)
    if (!Number.isNaN(port)) return port
  }
  return fallback
}

export function formatServerPortInput(serverPort: unknown, serverPorts: unknown): string {
  if (Array.isArray(serverPorts)) {
    const normalized = serverPorts
      .map((item) => String(item).trim())
      .filter((item) => item.length > 0)
    if (normalized.length > 0) {
      return normalized.join(',')
    }
  }
  if (typeof serverPort === 'number') return String(serverPort)
  if (typeof serverPort === 'string') return serverPort
  return ''
}

export function formatServerPortDisplay(serverPort: unknown, serverPorts: unknown): string {
  const formatted = formatServerPortInput(serverPort, serverPorts)
  if (formatted !== '') return formatted
  return '-'
}
