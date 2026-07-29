import { createRouter, createWebHistory } from 'vue-router'
import Login from '@/views/Login.vue'
import Data from '@/store/modules/data'
import MihomoData from '@/store/modules/mihomoData'
import HttpUtils from '@/plugins/httputil'
import { panelBaseURL } from '@/plugins/api'
import { cancelConfirm } from '@/plugins/confirm'
import { clearPanelTimeContext, ensurePanelTimeContext } from '@/plugins/panelTime'
import { i18n } from '@/locales'
import { push } from 'notivue'

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

let intervalId: any
let dataPollingEnabled = false
let globalDataRefreshPromise: Promise<void> | null = null

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

const handleVisibilityChange = () => {
  syncDataInterval()
}

if (typeof document !== 'undefined') {
  document.addEventListener('visibilitychange', handleVisibilityChange)
}

type SessionProbeState = 'authenticated' | 'unauthenticated' | 'unreachable'

let lastSessionConnectionWarningAt = 0

const delaySessionRetry = () => new Promise<void>(resolve => window.setTimeout(resolve, 400))

const probeSession = async (): Promise<SessionProbeState> => {
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const msg = await HttpUtils.get('api/session', {}, { silentAuthCheck: true, timeout: 10000 })
    if (msg.success) return 'authenticated'
    if (msg.failureKind === 'api') return 'unauthenticated'
    if (attempt === 0) await delaySessionRetry()
  }
  return 'unreachable'
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

router.beforeEach(async (to) => {
  const sessionState = await probeSession()
  const isAuthenticated = sessionState === 'authenticated'
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth)

  if (sessionState === 'unreachable') {
    notifySessionConnectionFailure()
    return true
  }

  if (to.path === '/login') {
    if (isAuthenticated) {
		  void ensurePanelTimeContext()
      return '/'
    }
    dataPollingEnabled = false
    stopDataInterval()
		clearPanelTimeContext()
    return true
  }

  if (requiresAuth && !isAuthenticated) {
    dataPollingEnabled = false
    stopDataInterval()
		clearPanelTimeContext()
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
  }

  return true
})

router.afterEach((_to, _from, failure) => {
  if (!failure) {
    cancelConfirm()
  }
})

export default router
