/**
 * SettingsTrafficManage.shared.ts
 * 流量管理业务共享逻辑、类型定义与纯工具函数模块
 */

import {
  panelCalendarDateFromInstant,
  panelCalendarDateToEpochSeconds,
  panelCalendarDateToInstant,
  panelNow,
} from '@/plugins/panelTime'

export type TrafficOverview = {
  source: string
  interface: string
  enabled: boolean
  status: string
  available: boolean
  up: number
  down: number
  total: number
  accumUp: number
  accumDown: number
  accumTotal: number
  limitGiB: number
  resetDay: number
  expiryDate: string
  expired: boolean
  nextResetAt: number
  updatedAt: number
  vnstat: VnstatStatus
  error?: string
}

export type VnstatStatus = {
  supported: boolean
  canManage: boolean
  installed: boolean
  managed: boolean
  ownership: string
  ownershipState: string
  ownershipHint: string
  running: boolean
  version: string
  systemFamily: string
  systemId: string
  systemVersion: string
  packageManager: string
  installMethod: string
  binaryPath: string
  fileCount: number
  dataPaths: string[]
  runtimeConflict: VnstatRuntimeConflict | null
  manageHint: string
  error?: string
}

export type VnstatRuntimeConflict = {
  message: string
  paths: string[]
  pids: number[]
  units: string[]
  detectedAt: number
}

export type VnstatVersionItem = {
  value: string
  title: string
  description: string
  available: boolean
  reason: string
  props?: {
    disabled?: boolean
    title?: string
  }
}

export type VnstatUpdateInfo = {
  supported: boolean
  canManage: boolean
  installed: boolean
  managed: boolean
  currentVersion: string
  latestVersion: string
  hasUpdate: boolean
  source: string
  message: string
}

export type VnstatInstallJob = {
  id: string
  source: string
  state: string
  phase: string
  canCancel: boolean
  stopRequested: boolean
  deadlineExceeded: boolean
  error: string
  startedAt: number
  finishedAt: number
}

export type VnstatRemovalJob = {
  id: string
  state: string
  phase: string
  error: string
  startedAt: number
  finishedAt: number
}

export type TrafficOverviewRaw = Record<string, unknown>

// 业务文案与常量字典
export const RESET_DAY_LABEL = '每月流量重置日'
export const EXPIRY_DATE_LABEL = '流量到期期限'
export const DISABLED_LABEL = '未启用'
export const DAY_SUFFIX = '号'
export const MONTHLY_HINT = '每月在该日 00:00 重置；若当月天数不足则自动在月末最后一天 00:00 重置。'
export const EXPIRY_HINT = '到达该日 00:00 后，将按流量用尽处理并封禁流量。'
export const RESET_PERIOD_CONFIRM_TEXT = '是否确认重置当前周期的流量统计？（历史总计数据不受影响）'
export const RESET_TOTAL_CONFIRM_TEXT = '是否确认重置历史累计总使用流量？（此操作不可逆）'
export const NEXT_RESET_LABEL = '下一次重置时间'
export const REMOVE_VNSTAT_CONFIRM_TEXT = '确认删除面板安装的 vnStat 吗？将停止并卸载经核验的受管程序、服务、配置和流量数据。'
export const INSTALL_VNSTAT_CONFIRM_TEXT = '继续后将仅检测正在运行的非面板 vnstatd；发现后会停止对应进程，并只停止、禁用已核验的 systemd/SysV 服务。非面板目录的程序、配置和统计文件会保留；面板固定路径如需释放，将由所选系统包管理器处理。随后安装面板 vnStat。确认继续吗？'
export const DISABLE_TRAFFIC_CONFIRM_TEXT = '确认关闭流量统计吗？关闭期间产生的流量不会计入面板统计，再次开启会从当前数值继续统计。'
export const SELECT_VNSTAT_SOURCE_HINT = '请先选择来源'

// 基础字段安全提取工具
export const readNumberField = (raw: TrafficOverviewRaw, keys: string[], fallback = 0): number => {
  for (const key of keys) {
    const value = raw[key]
    if (typeof value === 'number' && Number.isFinite(value)) {
      return value
    }
    if (typeof value === 'string') {
      const parsed = Number(value.trim())
      if (Number.isFinite(parsed)) {
        return parsed
      }
    }
  }
  return fallback
}

export const readStringField = (raw: TrafficOverviewRaw, keys: string[], fallback = ''): string => {
  for (const key of keys) {
    const value = raw[key]
    if (typeof value === 'string') {
      return value
    }
  }
  return fallback
}

export const readBoolField = (raw: TrafficOverviewRaw, keys: string[], fallback = false): boolean => {
  for (const key of keys) {
    const value = raw[key]
    if (typeof value === 'boolean') {
      return value
    }
    if (typeof value === 'number') {
      return value !== 0
    }
    if (typeof value === 'string') {
      const normalized = value.trim().toLowerCase()
      if (normalized === 'true' || normalized === '1') {
        return true
      }
      if (normalized === 'false' || normalized === '0') {
        return false
      }
    }
  }
  return fallback
}

export const readStringArrayField = (raw: TrafficOverviewRaw, keys: string[], fallback: string[] = []): string[] => {
  for (const key of keys) {
    const value = raw[key]
    if (Array.isArray(value)) {
      return value.map(item => String(item ?? '').trim()).filter(item => item.length > 0)
    }
  }
  return [...fallback]
}

export const readNumberArrayField = (raw: TrafficOverviewRaw, keys: string[], fallback: number[] = []): number[] => {
  for (const key of keys) {
    const value = raw[key]
    if (Array.isArray(value)) {
      return value.map(item => Number(item)).filter(item => Number.isFinite(item) && item > 0)
    }
  }
  return [...fallback]
}

// 规范化与解析工具函数
export const normalizeVnstatRuntimeConflict = (raw: unknown): VnstatRuntimeConflict | null => {
  if (raw == null || typeof raw !== 'object') {
    return null
  }
  const input = raw as TrafficOverviewRaw
  const conflict: VnstatRuntimeConflict = {
    message: readStringField(input, ['message'], '').trim(),
    paths: readStringArrayField(input, ['paths'], []),
    pids: readNumberArrayField(input, ['pids'], []),
    units: readStringArrayField(input, ['units'], []),
    detectedAt: readNumberField(input, ['detectedAt', 'detected_at'], 0),
  }
  return conflict.message !== '' || conflict.paths.length > 0 || conflict.pids.length > 0 || conflict.units.length > 0
    ? conflict
    : null
}

export const normalizeVnstatStatus = (raw: unknown): VnstatStatus => {
  const input = (raw ?? {}) as TrafficOverviewRaw
  return {
    supported: readBoolField(input, ['supported'], false),
    canManage: readBoolField(input, ['canManage', 'can_manage'], false),
    installed: readBoolField(input, ['installed'], false),
    managed: readBoolField(input, ['managed'], false),
    ownership: readStringField(input, ['ownership'], ''),
    ownershipState: readStringField(input, ['ownershipState', 'ownership_state'], ''),
    ownershipHint: readStringField(input, ['ownershipHint', 'ownership_hint'], ''),
    running: readBoolField(input, ['running'], false),
    version: readStringField(input, ['version'], ''),
    systemFamily: readStringField(input, ['systemFamily', 'system_family'], ''),
    systemId: readStringField(input, ['systemId', 'system_id'], ''),
    systemVersion: readStringField(input, ['systemVersion', 'system_version'], ''),
    packageManager: readStringField(input, ['packageManager', 'package_manager'], ''),
    installMethod: readStringField(input, ['installMethod', 'install_method'], ''),
    binaryPath: readStringField(input, ['binaryPath', 'binary_path'], ''),
    fileCount: readNumberField(input, ['fileCount', 'file_count'], 0),
    dataPaths: readStringArrayField(input, ['dataPaths', 'data_paths'], []),
    runtimeConflict: normalizeVnstatRuntimeConflict(input.runtimeConflict ?? input.runtime_conflict),
    manageHint: readStringField(input, ['manageHint', 'manage_hint'], ''),
    error: readStringField(input, ['error'], ''),
  }
}

export const normalizeVnstatVersionItems = (raw: unknown): VnstatVersionItem[] => {
  const input = (raw ?? {}) as TrafficOverviewRaw
  const rawVersions = input.versions
  if (!Array.isArray(rawVersions)) {
    return []
  }
  return rawVersions.map(rawVersion => {
    const item = (rawVersion ?? {}) as TrafficOverviewRaw
    const value = readStringField(item, ['value'], '').trim()
    const title = readStringField(item, ['title'], value).trim() || value
    return {
      value,
      title,
      description: readStringField(item, ['description'], '').trim(),
      available: readBoolField(item, ['available'], false),
      reason: readStringField(item, ['reason'], '').trim(),
    }
  }).filter(item => item.value === 'system-package' || item.value === 'github-release')
}

export const normalizeVnstatUpdateInfo = (raw: unknown): VnstatUpdateInfo => {
  const input = (raw ?? {}) as TrafficOverviewRaw
  return {
    supported: readBoolField(input, ['supported'], false),
    canManage: readBoolField(input, ['canManage', 'can_manage'], false),
    installed: readBoolField(input, ['installed'], false),
    managed: readBoolField(input, ['managed'], false),
    currentVersion: readStringField(input, ['currentVersion', 'current_version'], ''),
    latestVersion: readStringField(input, ['latestVersion', 'latest_version'], ''),
    hasUpdate: readBoolField(input, ['hasUpdate', 'has_update'], false),
    source: readStringField(input, ['source'], ''),
    message: readStringField(input, ['message'], ''),
  }
}

export const normalizeVnstatInstallJob = (raw: unknown): VnstatInstallJob => {
  const input = (raw ?? {}) as TrafficOverviewRaw
  return {
    id: readStringField(input, ['id'], '').trim(),
    source: readStringField(input, ['source'], '').trim(),
    state: readStringField(input, ['state'], 'idle').trim().toLowerCase() || 'idle',
    phase: readStringField(input, ['phase'], '').trim(),
    canCancel: readBoolField(input, ['canCancel', 'can_cancel'], false),
    stopRequested: readBoolField(input, ['stopRequested', 'stop_requested'], false),
    deadlineExceeded: readBoolField(input, ['deadlineExceeded', 'deadline_exceeded'], false),
    error: readStringField(input, ['error'], '').trim(),
    startedAt: readNumberField(input, ['startedAt', 'started_at'], 0),
    finishedAt: readNumberField(input, ['finishedAt', 'finished_at'], 0),
  }
}

export const normalizeVnstatRemovalJob = (raw: unknown): VnstatRemovalJob => {
  const input = (raw ?? {}) as TrafficOverviewRaw
  return {
    id: readStringField(input, ['id'], '').trim(),
    state: readStringField(input, ['state'], 'idle').trim().toLowerCase() || 'idle',
    phase: readStringField(input, ['phase'], '').trim(),
    error: readStringField(input, ['error'], '').trim(),
    startedAt: readNumberField(input, ['startedAt', 'started_at'], 0),
    finishedAt: readNumberField(input, ['finishedAt', 'finished_at'], 0),
  }
}

export const normalizeLimitGiB = (value: number): number => {
  if (!Number.isFinite(value) || value <= 0) return 0
  const rounded = Math.round(value * 100) / 100
  if (rounded > 0 && rounded < 0.01) return 0.01
  return rounded
}

export const normalizeResetDay = (value: number): number => {
  if (!Number.isFinite(value) || value <= 0) return 0
  if (value > 31) return 31
  return Math.floor(value)
}

export const daysInMonth = (year: number, monthIndex: number): number => (
  new Date(year, monthIndex + 1, 0).getDate()
)

export const computeResetBoundary = (day: number, year: number, month: number): Date => {
  const maxDay = daysInMonth(year, month)
  const effectiveDay = Math.min(day, maxDay)
  return new Date(year, month, effectiveDay, 0, 0, 0, 0)
}

export const normalizeExpiryDateInput = (value: string): string => {
  const trimmed = value.trim()
  if (trimmed === '') {
    return ''
  }
  const normalized = trimmed.replace(/\//g, '-').replace(/\./g, '-')
  const match = normalized.match(/^(\d{4})-(\d{1,2})-(\d{1,2})$/)
  if (match == null) {
    return ''
  }
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  if (year < 1 || month < 1 || month > 12 || day < 1 || day > daysInMonth(year, month - 1)) {
    return ''
  }
  return `${year.toString().padStart(4, '0')}-${month.toString().padStart(2, '0')}-${day.toString().padStart(2, '0')}`
}

export const getNextResetAt = (day: number): Date | null => {
  const normalizedDay = normalizeResetDay(day)
  if (normalizedDay <= 0) {
    return null
  }

  const now = panelCalendarDateFromInstant(panelNow())
  const thisBoundary = computeResetBoundary(normalizedDay, now.getFullYear(), now.getMonth())
  if (now.getTime() < thisBoundary.getTime()) {
    return panelCalendarDateToInstant(thisBoundary)
  }

  const nextMonthDate = new Date(now.getFullYear(), now.getMonth() + 1, 1, 0, 0, 0, 0)
  return panelCalendarDateToInstant(computeResetBoundary(normalizedDay, nextMonthDate.getFullYear(), nextMonthDate.getMonth()))
}

export const buildPickerEpochFromExpiryDate = (value: string): number => {
  const normalized = normalizeExpiryDateInput(value)
  if (normalized === '') {
    return 0
  }
  const match = normalized.match(/^(\d{4})-(\d{2})-(\d{2})$/)
  if (match == null) {
    return 0
  }
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const parsed = new Date(year, month - 1, day, 0, 0, 0, 0)
  if (
    !Number.isFinite(parsed.getTime()) ||
    parsed.getFullYear() !== year ||
    parsed.getMonth() !== month - 1 ||
    parsed.getDate() !== day
  ) {
    return 0
  }
  return panelCalendarDateToEpochSeconds(parsed)
}

export const parseEpochSeconds = (value: unknown): number | null => {
  if (typeof value === 'number' && Number.isFinite(value)) {
    const abs = Math.abs(value)
    return abs > 0 && abs < 1e11 ? Math.floor(value) : Math.floor(value / 1000)
  }

  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (trimmed.length === 0) {
      return null
    }

    if (/^-?\d+(?:\.\d+)?$/.test(trimmed)) {
      return parseEpochSeconds(Number(trimmed))
    }

    const parsed = Date.parse(trimmed)
    if (!Number.isFinite(parsed)) {
      return null
    }
    return Math.floor(parsed / 1000)
  }

  if (value instanceof Date) {
    const millis = value.getTime()
    if (!Number.isFinite(millis)) {
      return null
    }
    return Math.floor(millis / 1000)
  }

  return null
}

export const formatGB = (bytes: number): string => {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0.00 GB'
  }
  let gb = bytes / (1024 * 1024 * 1024)
  if (gb > 0 && gb < 0.01) {
    gb = 0.01
  }
  return `${gb.toFixed(2)} GB`
}

export const formatDynamicBytes = (bytes: number): string => {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B'
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let index = 0
  let current = bytes
  while (current >= 1024 && index < units.length - 1) {
    current /= 1024
    index++
  }
  return `${current.toFixed(index === 0 ? 0 : 2)} ${units[index]}`
}

export const createDefaultOverview = (): TrafficOverview => ({
  source: 'vnstat',
  interface: '',
  enabled: true,
  status: 'stopped',
  available: false,
  up: 0,
  down: 0,
  total: 0,
  accumUp: 0,
  accumDown: 0,
  accumTotal: 0,
  limitGiB: 0,
  resetDay: 0,
  expiryDate: '',
  expired: false,
  nextResetAt: 0,
  updatedAt: 0,
  vnstat: {
    supported: false,
    canManage: false,
    installed: false,
    managed: false,
    ownership: '',
    ownershipState: '',
    ownershipHint: '',
    running: false,
    version: '',
    systemFamily: '',
    systemId: '',
    systemVersion: '',
    packageManager: '',
    installMethod: '',
    binaryPath: '',
    fileCount: 0,
    dataPaths: [],
    runtimeConflict: null,
    manageHint: '',
  },
})

export const createIdleVnstatUpdateInfo = (status?: VnstatStatus): VnstatUpdateInfo => ({
  supported: status?.supported ?? false,
  canManage: status?.canManage ?? false,
  installed: status?.installed ?? false,
  managed: status?.managed ?? false,
  currentVersion: status?.version ?? '',
  latestVersion: '',
  hasUpdate: false,
  source: '',
  message: '',
})
