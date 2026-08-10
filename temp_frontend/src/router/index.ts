import { createRouter, createWebHistory } from 'vue-router'
import Login from '@/views/Login.vue'
import Data from '@/store/modules/data'
import MihomoData from '@/store/modules/mihomoData'
import HttpUtils from '@/plugins/httputil'
import { panelBaseURL } from '@/plugins/api'
import { cancelConfirm } from '@/plugins/confirm'
import { clearPanelTimeContext, ensurePanelTimeContext, panelNow } from '@/plugins/panelTime'
import { i18n } from '@/locales'
import { push } from 'notivue'
import { registerLoginNavigator, requestLoginNavigation } from '@/plugins/sessionNavigation'

const routes = [
  {
    path: '/login',
    name: 'pages.login',
    component: Login,
  },
  {
    path: '/',
    component: () => import('@/layouts/default/Default.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '/',
        name: 'pages.home',
        component: () => import('@/views/Home.vue'),
      },
      {
        path: '/submanager',
        name: 'pages.submanager',
        component: () => import('@/views/SubManager.vue'),
      },
      {
        path: '/inbounds',
        name: 'pages.inbounds',
        component: () => import('@/views/Inbounds.vue'),
      },
      {
        path: '/clients',
        name: 'pages.clients',
        component: () => import('@/views/Clients.vue'),
      },
      {
        path: '/outbounds',
        name: 'pages.outbounds',
        component: () => import('@/views/Outbounds.vue'),
      },
      {
        path: '/services',
        name: 'pages.services',
        component: () => import('@/views/Services.vue'),
        meta: { temporarilyHidden: true },
      },
      {
        path: '/endpoints',
        name: 'pages.endpoints',
        component: () => import('@/views/Endpoints.vue'),
        meta: { temporarilyHidden: true },
      },
      {
        path: '/rules',
        name: 'pages.rules',
        component: () => import('@/views/Rules.vue'),
      },
      {
        path: '/tls',
        name: 'pages.tls',
        component: () => import('@/views/Tls.vue'),
      },
      {
        path: '/basics',
        name: 'pages.basics',
        component: () => import('@/views/Basics.vue'),
      },
      {
        path: '/dns',
        name: 'pages.dns',
        component: () => import('@/views/Dns.vue'),
      },
      {
        path: '/admins',
        name: 'pages.admins',
        component: () => import('@/views/Admins.vue'),
      },
      {
        path: '/mihomo_inbounds',
        name: 'mihomo_入站管理',
        component: () => import('@/views/MihomoInbounds.vue'),
      },
      {
        path: '/mihomo_clients',
        name: 'mihomo_用户管理',
        component: () => import('@/views/MihomoClients.vue'),
      },
      {
        path: '/mihomo_outbounds',
        name: 'mihomo_出站管理',
        component: () => import('@/views/MihomoOutbounds.vue'),
      },
      {
        path: '/mihomo_tls',
        name: 'mihomo_TLS 设置',
        component: () => import('@/views/MihomoTls.vue'),
      },
      {
        path: '/mihomo_rules',
        name: 'mihomo_路由列表',
        component: () => import('@/views/MihomoRules.vue'),
      },
      {
        path: '/mihomo_dns',
        name: 'mihomo_DNS',
        component: () => import('@/views/MihomoDns.vue'),
      },
      {
        path: '/settings',
        name: 'pages.settings',
        component: () => import('@/views/Settings.vue'),
        meta: { skipGlobalDataPolling: true },
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(panelBaseURL),
  routes,
})

registerLoginNavigator(async () => {
  if (router.currentRoute.value.path !== '/login') {
    await router.replace('/login')
  }
})

let intervalId: any
let dataPollingEnabled = false
let globalDataRefreshPromise: Promise<void> | null = null
const sessionKeepaliveFallbackIntervalMs = 30_000
const sessionKeepaliveMinIntervalMs = 15_000
const sessionKeepaliveMaxIntervalMs = 60_000
let sessionKeepaliveEnabled = false
let sessionLifecycleGeneration = 0
let sessionKeepaliveTimerId: number | undefined
let sessionDeadlineAt: number | null = null
let sessionKeepaliveRedirecting = false

const stopDataInterval = () => {
  if (!intervalId) return
  clearInterval(intervalId)
  intervalId = undefined
}

const refreshGlobalData = async () => {
  if (globalDataRefreshPromise) return globalDataRefreshPromise

  globalDataRefreshPromise = (async () => {
    await Data().loadData()
    await MihomoData().loadData()
  })().finally(() => {
    globalDataRefreshPromise = null
  })
  return globalDataRefreshPromise
}

const syncDataInterval = () => {
  const pageVisible = typeof document === 'undefined' || document.visibilityState === 'visible'
  if (!dataPollingEnabled || !pageVisible) {
    stopDataInterval()
    return
  }
  if (intervalId) return
  void refreshGlobalData()
  intervalId = setInterval(() => {
    void refreshGlobalData()
  }, 10000)
}

type SessionProbeState = 'authenticated' | 'unauthenticated' | 'unreachable'
type SessionProbeResult = {
  state: SessionProbeState
  deadlineAt: number | null
}

let lastSessionConnectionWarningAt = 0
let sessionProbePromise: Promise<SessionProbeResult> | null = null
let sessionProbeGeneration = -1

const delaySessionRetry = () => new Promise<void>(resolve => window.setTimeout(resolve, 400))

const readSessionDeadline = (message: { obj?: unknown }): number | null => {
  const payload = (message.obj as { deadlineAt?: unknown; idleDeadlineAt?: unknown } | null) ?? null
  const rawSeconds = Number(payload?.deadlineAt ?? payload?.idleDeadlineAt ?? 0)
  if (!Number.isFinite(rawSeconds) || rawSeconds <= 0) return null
  return Math.floor(rawSeconds * 1000)
}

const probeSession = (): Promise<SessionProbeResult> => {
  const probeGeneration = sessionLifecycleGeneration
  if (sessionProbePromise && sessionProbeGeneration === probeGeneration) return sessionProbePromise

  const request = (async (): Promise<SessionProbeResult> => {
    for (let attempt = 0; attempt < 2; attempt += 1) {
      const msg = await HttpUtils.get('api/session', {}, { silentAuthCheck: true, timeout: 10000 })
      if (msg.success) {
        return { state: 'authenticated', deadlineAt: readSessionDeadline(msg) }
      }
      if (msg.failureKind === 'api') {
        return { state: 'unauthenticated', deadlineAt: null }
      }
      if (attempt === 0) await delaySessionRetry()
    }
    return { state: 'unreachable', deadlineAt: null }
  })()
  sessionProbePromise = request
  sessionProbeGeneration = probeGeneration
  void request.finally(() => {
    if (sessionProbePromise === request) {
      sessionProbePromise = null
      sessionProbeGeneration = -1
    }
  })
  return request
}

const notifySessionConnectionFailure = () => {
  const now = Date.now()
  if (now - lastSessionConnectionWarningAt < 15000) return
  lastSessionConnectionWarningAt = now
  push.warning({
    title: i18n.global.t('sessionConnectionErrorTitle'),
    duration: 6000,
    message: i18n.global.t('sessionConnectionErrorMessage'),
  })
}

const stopSessionKeepalive = () => {
  if (sessionKeepaliveTimerId == null) return
  window.clearTimeout(sessionKeepaliveTimerId)
  sessionKeepaliveTimerId = undefined
}

const isPageVisible = () => typeof document === 'undefined' || document.visibilityState === 'visible'

const scheduleSessionKeepalive = () => {
  if (!sessionKeepaliveEnabled || !isPageVisible() || sessionKeepaliveTimerId != null) return
  let delay = sessionKeepaliveFallbackIntervalMs
  if (sessionDeadlineAt != null) {
    const remainingMs = Math.max(0, sessionDeadlineAt - panelNow().getTime())
    delay = Math.max(
      sessionKeepaliveMinIntervalMs,
      Math.min(sessionKeepaliveMaxIntervalMs, Math.floor(remainingMs / 2)),
    )
  }
  sessionKeepaliveTimerId = window.setTimeout(() => {
    sessionKeepaliveTimerId = undefined
    void runSessionKeepalive()
  }, delay)
}

const syncSessionKeepalive = () => {
  if (!sessionKeepaliveEnabled || !isPageVisible()) {
    stopSessionKeepalive()
    return
  }
  scheduleSessionKeepalive()
}

const updateSessionDeadline = (deadlineAt: number | null) => {
  sessionDeadlineAt = deadlineAt
  stopSessionKeepalive()
  syncSessionKeepalive()
}

const stopAuthenticatedPolling = () => {
  sessionLifecycleGeneration += 1
  dataPollingEnabled = false
  stopDataInterval()
  sessionKeepaliveEnabled = false
  stopSessionKeepalive()
  sessionDeadlineAt = null
  clearPanelTimeContext()
}

const startAuthenticatedPolling = () => {
  if (!sessionKeepaliveEnabled) {
    sessionLifecycleGeneration += 1
  }
  sessionKeepaliveEnabled = true
}

const redirectToLoginAfterSessionExpiry = async () => {
  stopAuthenticatedPolling()
  if (sessionKeepaliveRedirecting) return
  sessionKeepaliveRedirecting = true
  try {
    await requestLoginNavigation()
  } finally {
    sessionKeepaliveRedirecting = false
  }
}

const runSessionKeepalive = async () => {
  if (!sessionKeepaliveEnabled || !isPageVisible()) return
  const lifecycleGeneration = sessionLifecycleGeneration
  const session = await probeSession()
  if (lifecycleGeneration !== sessionLifecycleGeneration || !sessionKeepaliveEnabled) return
  if (session.state === 'authenticated') {
    updateSessionDeadline(session.deadlineAt)
    return
  }
  if (session.state === 'unreachable') {
    syncSessionKeepalive()
    return
  }

  await redirectToLoginAfterSessionExpiry()
}

const handleVisibilityChange = () => {
  syncDataInterval()
  if (!isPageVisible()) {
    syncSessionKeepalive()
    return
  }
  if (sessionKeepaliveEnabled) {
    stopSessionKeepalive()
    void runSessionKeepalive()
    return
  }
  syncSessionKeepalive()
}

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', handleVisibilityChange)
}

router.beforeEach(async (to) => {
  const enteringLogin = to.path === '/login'
  if (enteringLogin) {
    stopAuthenticatedPolling()
  }

  const session = await probeSession()
  const sessionState = session.state
  const isAuthenticated = sessionState === 'authenticated'
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth)

  if (enteringLogin) {
    if (isAuthenticated) {
      void ensurePanelTimeContext()
      return '/'
    }
    return true
  }

  if (sessionState === 'unreachable') {
    notifySessionConnectionFailure()
    return true
  }

  if (requiresAuth && !isAuthenticated) {
    stopAuthenticatedPolling()
    return '/login'
  }

  // Temporarily hidden in UI because these pages are not needed for now.
  if (to.matched.some(record => record.meta.temporarilyHidden)) {
    return '/'
  }

  if (requiresAuth && isAuthenticated) {
    await ensurePanelTimeContext()
    dataPollingEnabled = !to.matched.some(record => record.meta.skipGlobalDataPolling)
    syncDataInterval()
    startAuthenticatedPolling()
    updateSessionDeadline(session.deadlineAt)
  }

  return true
})

router.afterEach((_to, _from, failure) => {
  if (!failure) {
    cancelConfirm()
  }
})

export default router
