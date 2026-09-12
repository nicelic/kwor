import api from './api'
import { i18n } from '@/locales'
import axios from 'axios'
import { push } from 'notivue'
import { requestLoginNavigation } from './sessionNavigation'

export interface Msg {
  success: boolean
  msg: string
  obj: any | null
  failureKind?: 'api' | 'transport' | 'cancelled' | 'protocol'
}

export interface HttpRequestOptions {
  // Keeps the historic fully silent behaviour used by low-noise probes.
  silentAuthCheck?: boolean
  // Suppresses ordinary failure toasts, but still handles an expired login.
  silentErrorToast?: boolean
  [key: string]: any
}

const KNOWN_API_MESSAGE_KEYS: Record<string, string> = {
  'sync failed': 'apiMessages.syncFailed',
  'sync completed successfully': 'apiMessages.syncCompletedSuccessfully',
  'some rules failed to sync': 'apiMessages.someRulesFailedToSync',
  'all active rules synced': 'apiMessages.allActiveRulesSynced',
  'invalid request payload': 'apiMessages.invalidRequestPayload',
  'ddns rule saved successfully': 'apiMessages.ddnsRuleSavedSuccessfully',
  'ddns rule status updated': 'apiMessages.ddnsRuleStatusUpdated',
  'ddns rule deleted': 'apiMessages.ddnsRuleDeleted',
  'dns account saved successfully': 'apiMessages.dnsAccountSavedSuccessfully',
  'dns account deleted': 'apiMessages.dnsAccountDeleted',
  'authentication failed': 'apiMessages.authenticationFailed',
  'credentials verified successfully': 'apiMessages.credentialsVerifiedSuccessfully',
  'save': 'actions.save',
  'retryruntime': 'actions.retryRuntime',
  'restartapp': 'actions.restartApp',
  'startcore': 'actions.startCore',
  'stopcore': 'actions.stopCore',
  'restartcore': 'actions.restartCore',
  'deletecore': 'actions.deleteCore',
  'save mihomo core log level': 'apiMessages.saveMihomoCoreLogLevel',
  'save sing-box core log level': 'apiMessages.saveSingboxCoreLogLevel',
}

export function formatApiMessage(rawMsg: string): string {
  if (!rawMsg || typeof rawMsg !== 'string') {
    return ''
  }
  const trimmed = rawMsg.trim()
  if (!trimmed) {
    return ''
  }

  const lookupKey = (key: string): string | null => {
    const normalized = key.toLowerCase().trim()
    const mapped = KNOWN_API_MESSAGE_KEYS[normalized]
    if (mapped && i18n.global.te(mapped)) {
      return i18n.global.t(mapped)
    }
    if (i18n.global.te('apiMessages.' + key)) {
      return i18n.global.t('apiMessages.' + key)
    }
    if (i18n.global.te('actions.' + key)) {
      return i18n.global.t('actions.' + key)
    }
    return null
  }

  // 1. Exact match
  const fullMatch = lookupKey(trimmed)
  if (fullMatch) {
    return fullMatch
  }

  // 2. Prefix with colon delimiter e.g. "Sync failed: ss.ccc.cc (A): Spaceship DNS update error..."
  const colonIndex = trimmed.indexOf(': ')
  if (colonIndex > 0) {
    const prefix = trimmed.slice(0, colonIndex).trim()
    const detail = trimmed.slice(colonIndex + 2)
    const translatedPrefix = lookupKey(prefix)
    if (translatedPrefix) {
      return `${translatedPrefix}: ${detail}`
    }
  }

  // 3. Network / Timeout checks
  if (trimmed.includes('timeout of') || trimmed.includes('timed out')) {
    if (i18n.global.te('apiMessages.timeout')) {
      return i18n.global.t('apiMessages.timeout')
    }
  }
  if (trimmed.includes('Network Error')) {
    if (i18n.global.te('apiMessages.networkError')) {
      return i18n.global.t('apiMessages.networkError')
    }
  }

  return trimmed
}

function _handleMsg(msg: any, options: HttpRequestOptions = {}): void {
  if (options.silentAuthCheck === true) {
    return
  }
  if (!isMsg(msg)) {
    return
  }
  if(msg.msg){
    if (!msg.success && msg.msg == "Invalid login") {
      push.warning({
        title: i18n.global.t('invalidLogin'),
        duration: 5000,
      })
      logout()
      return
    }
    if (!msg.success && options.silentErrorToast === true) {
      return
    }
    const formatted = formatApiMessage(msg.msg)
    if (msg.success) {
      push.success({
        message: i18n.global.t('success') + (formatted ? ": " + formatted : ""),
        duration: 5000,
      })
    } else {
      push.warning({
        title: i18n.global.t('failed'),
        duration: 5000,
        message: formatted
      })
    }
  }
}

let logoutPromise: Promise<void> | null = null

export const logout = async () => {
  if (logoutPromise) return logoutPromise

  const operation = (async () => {
    try {
      await HttpUtils.get('api/logout', {}, {
        silentAuthCheck: true,
        silentErrorToast: true,
        timeout: 5000,
      })
    } finally {
      await requestLoginNavigation()
    }
  })()

  logoutPromise = operation
  try {
    await operation
  } finally {
    if (logoutPromise === operation) {
      logoutPromise = null
    }
  }
}

function _respToMsg(resp: any): Msg {
  const data = resp.data
  if (data == null) {
    return { success: true, msg: "", obj: null }
  } else if (isMsg(data)) {
    return {
      success: data.success,
      msg: data.msg,
      obj: data.obj ?? null,
      failureKind: data.success ? undefined : 'api',
    }
  } else {
    return { success: false, msg: `unknown data: ${data}`, obj: null, failureKind: 'protocol' }
  }
}

function isMsg(obj: any): obj is Msg {
  return obj !== null
    && typeof obj === 'object'
    && Object.hasOwn(obj, 'success')
    && Object.hasOwn(obj, 'msg')
    && Object.hasOwn(obj, 'obj')
}
  
const HttpUtils = {
  async get(url: string, data: object = {}, options: HttpRequestOptions = {}): Promise<Msg> {
    const { silentAuthCheck, silentErrorToast, ...requestOptions } = options ?? {}
    let msg: Msg
    try {
        const resp = await api.get(url, { params: data, ...requestOptions })
        msg = _respToMsg(resp)
    } catch (e: any) {
        if (axios.isCancel(e)) {
            msg = { success: false, msg: "", obj: null, failureKind: 'cancelled' }
        } else {
            msg = { success: false, msg: e.toString(), obj: null, failureKind: 'transport' }
        }
    }
    _handleMsg(msg, { silentAuthCheck, silentErrorToast })
    return msg
  },
  async post(url: string, data: object | null, options: HttpRequestOptions = {}): Promise<Msg> {
    const { silentAuthCheck, silentErrorToast, ...requestOptions } = options ?? {}
    let msg: Msg
    try {
        const resp = await api.post(url, data, requestOptions)
        msg = _respToMsg(resp)
    } catch (e: any) {
        if (axios.isCancel(e)) {
            msg = { success: false, msg: "", obj: null, failureKind: 'cancelled' }
        } else {
            msg = { success: false, msg: e.toString(), obj: null, failureKind: 'transport' }
        }
    }
    _handleMsg(msg, { silentAuthCheck, silentErrorToast })
    return msg
  },
}

export default HttpUtils
