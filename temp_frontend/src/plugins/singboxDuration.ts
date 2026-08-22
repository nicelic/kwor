export type SingboxDurationUnit = 'ns' | 'us' | 'µs' | 'ms' | 's' | 'm' | 'h'

const durationUnitMilliseconds: Record<SingboxDurationUnit, number> = {
  ns: 0.000001,
  us: 0.001,
  'µs': 0.001,
  ms: 1,
  s: 1000,
  m: 60 * 1000,
  h: 60 * 60 * 1000,
}

const durationPartPattern = /(\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g
const decimalPattern = /^\d+(?:\.\d+)?$/

// Parses the positive/zero duration grammar used by the editable sing-box
// fields. Composite durations such as 1m30s remain meaningful when a page
// presents the value in one chosen unit.
export function readSingboxDuration(value: unknown, unit: SingboxDurationUnit): number | undefined {
  if (typeof value !== 'string') return undefined
  const raw = value.trim().toLowerCase()
  if (raw === '') return undefined

  let cursor = 0
  let milliseconds = 0
  for (const match of raw.matchAll(durationPartPattern)) {
    if (match.index !== cursor) return undefined
    const amount = Number(match[1])
    const sourceUnit = match[2] as SingboxDurationUnit
    if (!Number.isFinite(amount) || amount < 0) return undefined
    milliseconds += amount * durationUnitMilliseconds[sourceUnit]
    if (!Number.isFinite(milliseconds)) return undefined
    cursor += match[0].length
  }
  if (cursor !== raw.length) return undefined
  return milliseconds / durationUnitMilliseconds[unit]
}

export function writeSingboxDuration(
  value: unknown,
  unit: SingboxDurationUnit,
  options: { minimum?: number, allowZero?: boolean } = {},
): string | undefined {
  if (value === '' || value === null || value === undefined) return undefined
  const raw = typeof value === 'number' ? String(value) : String(value).trim()
  if (!decimalPattern.test(raw)) return undefined
  const amount = Number(raw)
  const minimum = options.minimum ?? 0
  const allowZero = options.allowZero ?? false
  if (!Number.isFinite(amount) || amount < minimum || (!allowZero && amount === 0)) return undefined
  return `${amount}${unit}`
}
