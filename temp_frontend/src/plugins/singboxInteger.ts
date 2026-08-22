export interface SingboxIntegerOptions {
  min?: number
  max?: number
}

const wholeNumberPattern = /^\d+$/

// The server treats numeric options as exact integers. Keep the editor from
// silently truncating values such as 443.5 before its final validation runs.
export function parseSingboxInteger(value: unknown, options: SingboxIntegerOptions = {}): number | undefined {
  const min = options.min ?? 0
  const max = options.max ?? Number.MAX_SAFE_INTEGER
  if (!Number.isSafeInteger(min) || !Number.isSafeInteger(max) || min > max) return undefined

  if (typeof value === 'number') {
    return Number.isSafeInteger(value) && value >= min && value <= max ? value : undefined
  }
  if (typeof value !== 'string') return undefined

  const text = value.trim()
  if (!wholeNumberPattern.test(text)) return undefined
  const parsed = Number(text)
  return Number.isSafeInteger(parsed) && parsed >= min && parsed <= max ? parsed : undefined
}

export function parseSingboxByteList(value: unknown, length = 3): number[] | undefined {
  if (!Number.isSafeInteger(length) || length <= 0) return undefined

  const items = Array.isArray(value)
    ? value
    : typeof value === 'string'
      ? value.trim().split(',').map((item) => item.trim())
      : undefined
  if (!items || items.length !== length) return undefined

  const bytes = items.map((item) => parseSingboxInteger(item, { min: 0, max: 255 }))
  return bytes.every((item): item is number => item !== undefined) ? bytes : undefined
}
