import { createRouter, createWebHistory } from 'vue-router'
import Login from '@/views/Login.vue'
import Data from '@/store/modules/data'
import MihomoData from '@/store/modules/mihomoData'
import HttpUtils from '@/plugins/httputil'

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
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory((window as any).BASE_URL),
  routes,
})

let intervalId: any

const stopDataInterval = () => {
  if (!intervalId) return
  clearInterval(intervalId)
  intervalId = undefined
}

const probeSession = async () => {
  const msg = await HttpUtils.get('api/session', {}, { silentAuthCheck: true })
  return msg.success
}

router.beforeEach(async (to) => {
  const isAuthenticated = await probeSession()
  const requiresAuth = to.matched.some(record => record.meta.requiresAuth)

  if (to.path === '/login') {
    if (isAuthenticated) {
      return '/'
    }
    stopDataInterval()
    return true
  }

  if (requiresAuth && !isAuthenticated) {
    stopDataInterval()
    return '/login'
  }

  // Temporarily hidden in UI because these pages are not needed for now.
  if (to.matched.some(record => record.meta.temporarilyHidden)) {
    return '/'
  }

  if (requiresAuth && isAuthenticated) {
    loadDataInterval()
  }

  return true
})

const loadDataInterval = () => {
  if (intervalId) return
  Data().loadData()
  MihomoData().loadData()
  intervalId = setInterval(() => {
    Data().loadData()
    MihomoData().loadData()
  }, 10000)
}

export default router
