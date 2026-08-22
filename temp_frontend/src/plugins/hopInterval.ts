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

function parsePositiveHopInteger(value: unknown): number {
  if (typeof value === 'number') {
    return Number.isSafeInteger(value) && value > 0 ? value : 0
  }
  if (typeof value !== 'string' || !/^\d+$/.test(value)) return 0
  const parsed = Number(value)
  return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : 0
}

function intervalUnitToSeconds(amount: number, unit: string): number {
  if (!Number.isSafeInteger(amount) || amount <= 0) return 0

  let seconds = amount
  switch (normalizeIntervalUnit(unit)) {
    case 'd':
      seconds = amount * 86400
      break
    case 'h':
      seconds = amount * 3600
      break
    case 'm':
      seconds = amount * 60
      break
    case 'ms':
      if (amount % 1000 !== 0) return 0
      seconds = amount / 1000
      break
  }
  return Number.isSafeInteger(seconds) && seconds > 0 ? seconds : 0
}

export function parseHopIntervalSeconds(raw: unknown): number {
  if (typeof raw === 'number') {
    return parsePositiveHopInteger(raw)
  }
  if (typeof raw !== 'string') return 0

  const input = raw.trim()
  if (input === '') return 0

  const matched = input.match(SINGLE_INTERVAL_RE)
  if (!matched) return 0

  const amount = parsePositiveHopInteger(matched[1])
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
    const leftSeconds = intervalUnitToSeconds(parsePositiveHopInteger(leftMatch[1]), leftUnit)
    const rightSeconds = intervalUnitToSeconds(parsePositiveHopInteger(rightMatch[1]), rightUnit)
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
