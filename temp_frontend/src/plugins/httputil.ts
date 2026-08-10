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
    if (msg.success) {
      push.success({
        message: i18n.global.t('success') + ": " + i18n.global.t('actions.' + msg.msg),
        duration: 5000,
      })
    } else {
      push.warning({
        title: i18n.global.t('failed'),
        duration: 5000,
        message: msg.msg
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
