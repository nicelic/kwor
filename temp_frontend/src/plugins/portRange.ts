export function normalizePortRangeInput(raw: string): string[] {
  if (typeof raw !== 'string') return []
  return raw
    .replace(/\uFF0C/g, ',')
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part.length > 0)
    .map((part) => part.replace(/\s+/g, '').replace(/-/g, ':'))
}

export const MANAGED_PORT_HOP_MAX_RANGE_BYTES = 512
export const MANAGED_PORT_HOP_MAX_SEGMENTS = 32
export const MANAGED_PORT_HOP_MAX_PORTS = 4096

// Kept for current Mihomo callers. The parser is intentionally core-neutral:
// both managed inbound paths use the same panel/nft monitoring limits.
export const MIHOMO_PORT_HOP_MAX_RANGE_BYTES = MANAGED_PORT_HOP_MAX_RANGE_BYTES
export const MIHOMO_PORT_HOP_MAX_SEGMENTS = MANAGED_PORT_HOP_MAX_SEGMENTS
export const MIHOMO_PORT_HOP_MAX_PORTS = MANAGED_PORT_HOP_MAX_PORTS

export interface MihomoPortHopRangeResult {
  normalized: string
  error?: string
}

export type ManagedPortHopRangeResult = MihomoPortHopRangeResult

export function normalizeManagedPortHopRangeInput(raw: unknown): ManagedPortHopRangeResult {
  const input = typeof raw === 'string' ? raw.trim() : ''
  if (input === '') return { normalized: '' }
  if (new TextEncoder().encode(input).length > MANAGED_PORT_HOP_MAX_RANGE_BYTES) {
    return { normalized: '', error: `Port range is too long (max ${MANAGED_PORT_HOP_MAX_RANGE_BYTES} bytes).` }
  }

  const spans: Array<{ start: number; end: number }> = []
  for (const rawPart of input.replace(/\uFF0C/g, ',').split(',')) {
    const part = rawPart.trim().replace(/\s+/g, '')
    if (part === '') return { normalized: '', error: 'Port range contains an empty segment.' }
    const separators = [...part].filter((character) => character === '-' || character === ':')
    let startText = part
    let endText = part
    if (separators.length > 0) {
      if (separators.length !== 1) return { normalized: '', error: `Invalid port range segment: ${part}` }
      const separator = separators[0]
      const pieces = part.split(separator)
      if (pieces.length !== 2) return { normalized: '', error: `Invalid port range segment: ${part}` }
      ;[startText, endText] = pieces
    }
    if (!/^\d+$/.test(startText) || !/^\d+$/.test(endText)) {
      return { normalized: '', error: `Invalid port range segment: ${part}` }
    }
    const start = Number(startText)
    const end = Number(endText)
    if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start < 1 || end > 65535 || start > end) {
      return { normalized: '', error: `Invalid port range segment: ${part}` }
    }
    spans.push({ start, end })
  }

  spans.sort((left, right) => left.start - right.start || left.end - right.end)
  const merged: Array<{ start: number; end: number }> = []
  for (const current of spans) {
    const previous = merged[merged.length - 1]
    if (previous && current.start <= previous.end + 1) {
      previous.end = Math.max(previous.end, current.end)
    } else {
      merged.push({ ...current })
    }
  }
  if (merged.length > MANAGED_PORT_HOP_MAX_SEGMENTS) {
    return { normalized: '', error: `Port range has too many segments (max ${MANAGED_PORT_HOP_MAX_SEGMENTS}).` }
  }
  const total = merged.reduce((sum, span) => sum + span.end - span.start + 1, 0)
  if (total > MANAGED_PORT_HOP_MAX_PORTS) {
    return { normalized: '', error: `Port range contains too many ports (max ${MANAGED_PORT_HOP_MAX_PORTS}).` }
  }
  return {
    normalized: merged.map((span) => span.start === span.end ? String(span.start) : `${span.start}-${span.end}`).join(','),
  }
}

export function normalizeMihomoPortHopRangeInput(raw: unknown): MihomoPortHopRangeResult {
  return normalizeManagedPortHopRangeInput(raw)
}

export function parseServerPortInput(raw: string): number | undefined {
  if (typeof raw !== 'string') return undefined
  const input = raw.trim()
  if (!/^\d+$/.test(input)) return undefined
  const port = Number(input)
  return Number.isSafeInteger(port) && port >= 1 && port <= 65535 ? port : undefined
}

export function normalizeServerPortInput(raw: string): number | string | undefined {
  if (typeof raw !== 'string') return undefined
  const input = raw.trim()
  if (input === '') return undefined
  const port = parseServerPortInput(input)
  return port === undefined ? input : port
}

export function pickPrimaryPort(serverPorts: string[], fallback?: number): number | undefined {
  for (const item of serverPorts) {
    const parts = String(item ?? '').trim().replace(/-/g, ':').split(':')
    if (parts.length < 1 || parts.length > 2 || !parts.every((part) => /^\d+$/.test(part))) continue
    const port = Number(parts[0])
    if (Number.isSafeInteger(port) && port >= 1 && port <= 65535) return port
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
