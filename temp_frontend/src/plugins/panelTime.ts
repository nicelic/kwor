import { reactive } from 'vue'
import HttpUtils from '@/plugins/httputil'

export type PanelTimeContext = {
  unix: number
  timeLocation: string
  selectable: boolean
}

export type PanelCalendarParts = {
  year: number
  month: number
  day: number
  hour: number
  minute: number
  second: number
}

// 该状态只保存服务端时间锚点与 IANA 时区；没有 setInterval、轮询或常驻连接。
export const panelTimeContext = reactive<PanelTimeContext & {
  loaded: boolean
  loading: boolean
  anchorPerformanceNow: number
}>({
  unix: 0,
  timeLocation: 'UTC',
  selectable: true,
  loaded: false,
  loading: false,
  anchorPerformanceNow: 0,
})

let loadPromise: Promise<boolean> | null = null
let contextRequestGeneration = 0

const getPerformanceNow = () => (
  typeof performance !== 'undefined' && typeof performance.now === 'function'
    ? performance.now()
    : 0
)

const normalizeTimeZone = (value: unknown) => {
  const candidate = String(value ?? '').trim()
  if (candidate === '') return 'UTC'
  try {
    new Intl.DateTimeFormat('en-US', { timeZone: candidate }).format()
    return candidate
  } catch {
    return 'UTC'
  }
}

export const clearPanelTimeContext = () => {
  contextRequestGeneration += 1
  panelTimeContext.unix = 0
  panelTimeContext.timeLocation = 'UTC'
  panelTimeContext.selectable = true
  panelTimeContext.loaded = false
  panelTimeContext.loading = false
  panelTimeContext.anchorPerformanceNow = 0
  loadPromise = null
}

export const ensurePanelTimeContext = async (force = false): Promise<boolean> => {
  if (!force && panelTimeContext.loaded) return true
  if (!force && loadPromise != null) return loadPromise

  const requestGeneration = ++contextRequestGeneration
  const request = (async () => {
    panelTimeContext.loading = true
    try {
      const msg = await HttpUtils.get('api/panel-time-context', {}, { silentAuthCheck: true, timeout: 8000 })
      if (!msg.success || msg.obj == null) return false

      const unix = Number(msg.obj.unix)
      if (!Number.isFinite(unix) || unix <= 0) return false
      // A logout or a forced refresh can finish before this request. Do not
      // let an older response repopulate the context or overwrite a newer
      // panel timezone.
      if (requestGeneration !== contextRequestGeneration) return false

      panelTimeContext.unix = Math.floor(unix)
      panelTimeContext.timeLocation = normalizeTimeZone(msg.obj.timeLocation)
      panelTimeContext.selectable = msg.obj.selectable === true
      panelTimeContext.anchorPerformanceNow = getPerformanceNow()
      panelTimeContext.loaded = true
      return true
    } finally {
      if (requestGeneration === contextRequestGeneration) {
        panelTimeContext.loading = false
      }
    }
  })()

  loadPromise = request
  try {
    return await request
  } finally {
    if (loadPromise === request) loadPromise = null
  }
}

export const refreshPanelTimeContext = () => ensurePanelTimeContext(true)

export const panelTimeZone = () => normalizeTimeZone(panelTimeContext.timeLocation)

// 服务端锚点避免显示逻辑依赖浏览器的手工时钟；浏览器仅用于计算已过去的毫秒数。
export const panelNow = (): Date => {
  if (!panelTimeContext.loaded || panelTimeContext.unix <= 0) return new Date()
  const elapsed = Math.max(0, getPerformanceNow() - panelTimeContext.anchorPerformanceNow)
  return new Date(panelTimeContext.unix * 1000 + elapsed)
}

export const panelNowUnix = () => Math.floor(panelNow().getTime() / 1000)

const toDate = (value: Date | number | string): Date | null => {
  if (value instanceof Date) {
    return Number.isFinite(value.getTime()) ? new Date(value.getTime()) : null
  }
  if (typeof value === 'number') {
    const date = new Date(value)
    return Number.isFinite(date.getTime()) ? date : null
  }
  const date = new Date(value)
  return Number.isFinite(date.getTime()) ? date : null
}

const dateTimeOptions = (options: Intl.DateTimeFormatOptions = {}): Intl.DateTimeFormatOptions => {
  const zone = panelTimeZone()
  if (options.dateStyle != null || options.timeStyle != null) {
    return { ...options, timeZone: zone }
  }
  return {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    ...options,
    timeZone: zone,
  }
}

export const formatPanelDateTime = (
  value: Date | number | string,
  locale?: string,
  options: Intl.DateTimeFormatOptions = {},
) => {
  const date = toDate(value)
  if (date == null) return '-'
  return new Intl.DateTimeFormat(locale || undefined, dateTimeOptions(options)).format(date)
}

export const formatPanelDate = (
  value: Date | number | string,
  locale?: string,
  options: Intl.DateTimeFormatOptions = {},
) => {
  const date = toDate(value)
  if (date == null) return '-'
  return new Intl.DateTimeFormat(locale || undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    ...options,
    timeZone: panelTimeZone(),
  }).format(date)
}

const numericPart = (parts: Intl.DateTimeFormatPart[], type: Intl.DateTimeFormatPartTypes) => {
  const raw = parts.find(part => part.type === type)?.value ?? ''
  const value = Number(raw)
  return Number.isFinite(value) ? value : 0
}

export const panelCalendarParts = (value: Date | number | string = panelNow()): PanelCalendarParts => {
  const date = toDate(value) ?? panelNow()
  const formatter = new Intl.DateTimeFormat('en-US-u-ca-gregory', {
    timeZone: panelTimeZone(),
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hourCycle: 'h23',
  })
  const parts = formatter.formatToParts(date)
  return {
    year: numericPart(parts, 'year'),
    month: numericPart(parts, 'month'),
    day: numericPart(parts, 'day'),
    hour: numericPart(parts, 'hour'),
    minute: numericPart(parts, 'minute'),
    second: numericPart(parts, 'second'),
  }
}

// 日期选择器接收浏览器本地 Date。这里把它作为“面板日历字段”的载体，而非
// 浏览器时区的真实瞬间，避免选择日期时受管理端浏览器时区影响。
export const panelCalendarDateFromInstant = (value: Date | number | string = panelNow()) => {
  const parts = panelCalendarParts(value)
  return new Date(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute, parts.second, 0)
}

export const panelCalendarDateFromParts = (
  year: number,
  month: number,
  day: number,
  hour = 0,
  minute = 0,
  second = 0,
) => new Date(year, month - 1, day, hour, minute, second, 0)

const offsetMillisecondsAt = (instant: Date, timeZone: string) => {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    timeZoneName: 'longOffset',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(instant)
  const offsetName = parts.find(part => part.type === 'timeZoneName')?.value ?? 'GMT'
  if (offsetName === 'GMT' || offsetName === 'UTC') return 0
  const match = offsetName.match(/^(?:GMT|UTC)([+-])(\d{1,2})(?::?(\d{2}))?$/)
  if (match != null) {
    const sign = match[1] === '-' ? -1 : 1
    const hours = Number(match[2])
    const minutes = Number(match[3] ?? '0')
    return sign * (hours * 60 + minutes) * 60 * 1000
  }

  // longOffset is supported by current browsers. This fallback keeps the
  // conversion usable in older engines that only return localized names.
  const calendar = panelCalendarParts(instant)
  return Date.UTC(calendar.year, calendar.month - 1, calendar.day, calendar.hour, calendar.minute, calendar.second) - instant.getTime()
}

export const panelCalendarPartsToInstant = (
  year: number,
  month: number,
  day: number,
  hour = 0,
  minute = 0,
  second = 0,
  millisecond = 0,
) => {
  const zone = panelTimeZone()
  const base = Date.UTC(year, month - 1, day, hour, minute, second, millisecond)
  let candidate = base
  for (let i = 0; i < 3; i += 1) {
    const next = base - offsetMillisecondsAt(new Date(candidate), zone)
    if (next === candidate) break
    candidate = next
	}
	return new Date(candidate)
}

export const panelCalendarDateToInstant = (value: Date) => panelCalendarPartsToInstant(
	value.getFullYear(),
	value.getMonth() + 1,
	value.getDate(),
	value.getHours(),
	value.getMinutes(),
	value.getSeconds(),
	value.getMilliseconds(),
)

export const formatPanelTime = (
	value: Date | number | string,
	locale?: string,
	options: Intl.DateTimeFormatOptions = {},
) => {
	const date = toDate(value)
	if (date == null) return '-'
	const formatOptions = options.timeStyle != null
		? { ...options, timeZone: panelTimeZone() }
		: {
			hour: '2-digit' as const,
			minute: '2-digit' as const,
			second: '2-digit' as const,
			...options,
			timeZone: panelTimeZone(),
		}
	return new Intl.DateTimeFormat(locale || undefined, formatOptions).format(date)
}

export const panelCalendarDateToEpochSeconds = (value: Date) => (
  Math.floor(panelCalendarDateToInstant(value).getTime() / 1000)
)
