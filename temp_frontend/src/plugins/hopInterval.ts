export interface ParsedHopIntervalInput {
  hopInterval?: string
  hopIntervalMax?: string
}

const SINGLE_INTERVAL_RE = /^(\d+)\s*(ms|s|m|h|d)?$/i
const RANGE_INTERVAL_RE = /^(.+?)\s*[-:]\s*(.+)$/i

function normalizeIntervalUnit(unit?: string, fallback: string = 's'): string {
  const normalized = typeof unit === 'string' ? unit.trim().toLowerCase() : ''
  return normalized !== '' ? normalized : fallback
}

function intervalUnitToSeconds(amount: number, unit: string): number {
  if (!Number.isFinite(amount) || amount <= 0) return 0

  switch (normalizeIntervalUnit(unit)) {
    case 'd':
      return amount * 86400
    case 'h':
      return amount * 3600
    case 'm':
      return amount * 60
    case 'ms':
      return Math.max(1, Math.round(amount / 1000))
    default:
      return amount
  }
}

export function parseHopIntervalSeconds(raw: unknown): number {
  if (typeof raw === 'number') {
    return Number.isFinite(raw) && raw > 0 ? Math.floor(raw) : 0
  }
  if (typeof raw !== 'string') return 0

  const input = raw.trim()
  if (input === '') return 0

  const matched = input.match(SINGLE_INTERVAL_RE)
  if (!matched) return 0

  const amount = Number.parseInt(matched[1], 10)
  return intervalUnitToSeconds(amount, normalizeIntervalUnit(matched[2]))
}

export function parseHopIntervalInput(raw: unknown): ParsedHopIntervalInput | undefined {
  if (raw == undefined) {
    return { hopInterval: undefined, hopIntervalMax: undefined }
  }

  const input = String(raw)
    .trim()
    .replace(/\uFF1A/g, ':')
    .replace(/\u2013|\u2014|\u2212/g, '-')
  if (input === '') {
    return { hopInterval: undefined, hopIntervalMax: undefined }
  }

  const rangeMatch = input.match(RANGE_INTERVAL_RE)
  if (rangeMatch) {
    const leftRaw = rangeMatch[1].trim()
    const rightRaw = rangeMatch[2].trim()
    const leftMatch = leftRaw.match(SINGLE_INTERVAL_RE)
    const rightMatch = rightRaw.match(SINGLE_INTERVAL_RE)
    if (!leftMatch || !rightMatch) return undefined

    const leftUnit = normalizeIntervalUnit(leftMatch[2], normalizeIntervalUnit(rightMatch[2]))
    const rightUnit = normalizeIntervalUnit(rightMatch[2], normalizeIntervalUnit(leftMatch[2]))
    const leftSeconds = intervalUnitToSeconds(Number.parseInt(leftMatch[1], 10), leftUnit)
    const rightSeconds = intervalUnitToSeconds(Number.parseInt(rightMatch[1], 10), rightUnit)
    if (leftSeconds <= 0 || rightSeconds <= 0) return undefined

    const lower = Math.min(leftSeconds, rightSeconds)
    const upper = Math.max(leftSeconds, rightSeconds)
    return {
      hopInterval: `${lower}s`,
      hopIntervalMax: upper > lower ? `${upper}s` : undefined,
    }
  }

  const seconds = parseHopIntervalSeconds(input)
  if (seconds <= 0) return undefined

  return {
    hopInterval: `${seconds}s`,
    hopIntervalMax: undefined,
  }
}

export function applyHopIntervalInput(
  target: { [key: string]: any },
  raw: unknown,
): boolean {
  const parsed = parseHopIntervalInput(raw)
  if (!parsed) return false

  if (parsed.hopInterval) {
    target.hop_interval = parsed.hopInterval
  } else {
    delete target.hop_interval
  }

  if (parsed.hopIntervalMax) {
    target.hop_interval_max = parsed.hopIntervalMax
  } else {
    delete target.hop_interval_max
  }

  return true
}

export function formatHopIntervalInput(hopInterval: unknown, hopIntervalMax?: unknown): string {
  const rawPrimary = typeof hopInterval === 'string' ? hopInterval.trim() : ''
  if (rawPrimary !== '' && RANGE_INTERVAL_RE.test(rawPrimary)) {
    const parsedPrimary = parseHopIntervalInput(rawPrimary)
    if (parsedPrimary) {
      const lower = parseHopIntervalSeconds(parsedPrimary.hopInterval)
      const upper = parseHopIntervalSeconds(parsedPrimary.hopIntervalMax)
      if (lower > 0 && upper > 0) {
        const minValue = Math.min(lower, upper)
        const maxValue = Math.max(lower, upper)
        return minValue === maxValue ? `${minValue}s` : `${minValue}-${maxValue}s`
      }
      if (lower > 0) return `${lower}s`
    }
  }

  const lower = parseHopIntervalSeconds(hopInterval)
  const upper = parseHopIntervalSeconds(hopIntervalMax)

  if (lower > 0 && upper > 0) {
    const minValue = Math.min(lower, upper)
    const maxValue = Math.max(lower, upper)
    return minValue === maxValue ? `${minValue}s` : `${minValue}-${maxValue}s`
  }
  if (lower > 0) return `${lower}s`
  if (upper > 0) return `${upper}s`
  return ''
}
