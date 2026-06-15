<template>
  <v-navigation-drawer
    v-model="showDrawer"
    :temporary="isMobile"
    :expand-on-hover="!isMobile"
    :rail="!isMobile"
    :permanent="!isMobile"
    @click="isMobile ? $emit('toggleDrawer') : null">
    <v-list-item
      height="63"
      prepend-avatar="@/assets/logo.svg"
      title="kwor">
      <template v-slot:append v-if="isMobile">
        <v-icon icon="mdi-close" />
      </template>
    </v-list-item>

    <v-divider></v-divider>

    <v-list density="compact" nav>
      <v-list-item
        link
        v-for="item in visibleMenu"
        :key="item.path"
        :to="item.path"
        :active="router.currentRoute.value.path == item.path">
        <template v-slot:prepend>
          <v-icon :icon="item.icon"></v-icon>
        </template>
        <v-list-item-title v-text="menuLabel(item)"></v-list-item-title>
      </v-list-item>
    </v-list>
    <template v-slot:append>
      <v-list-item prepend-icon="mdi-logout" :title="$t('menu.logout')" @click="Logout"></v-list-item>
    </template>
  </v-navigation-drawer>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import router from '@/router'
import { logout } from '@/plugins/httputil'
import { i18n } from '@/locales'

const props = defineProps(['isMobile', 'displayDrawer'])

const showDrawer = computed((): boolean => {
  return props.displayDrawer
})

const menu = [
  { title: 'pages.home', icon: 'mdi-home', path: '/' },
  { title: 'pages.submanager', icon: 'mdi-bookmark-multiple', path: '/submanager' },
  { title: 'pages.inbounds', icon: 'mdi-cloud-download', path: '/inbounds', prefix: 'singbox_' },
  { title: 'pages.clients', icon: 'mdi-account-multiple', path: '/clients', prefix: 'singbox_' },
  { title: 'pages.outbounds', icon: 'mdi-cloud-upload', path: '/outbounds', prefix: 'singbox_' },
  { title: 'pages.tls', icon: 'mdi-certificate', path: '/tls', prefix: 'singbox_' },
  { title: 'pages.rules', icon: 'mdi-routes', path: '/rules', prefix: 'singbox_' },
  { title: 'pages.dns', icon: 'mdi-dns', path: '/dns', prefix: 'singbox_' },
  // Temporarily hidden in UI because these entries are not needed for now.
  { title: 'pages.endpoints', icon: 'mdi-cloud-tags', path: '/endpoints', hidden: true },
  { title: 'pages.services', icon: 'mdi-server', path: '/services', hidden: true },
  { title: 'pages.basics', icon: 'mdi-application-cog', path: '/basics' },
  { title: 'pages.admins', icon: 'mdi-account-tie', path: '/admins' },
  { title: 'mihomo_入站管理', icon: 'mdi-cloud-download', path: '/mihomo_inbounds' },
  { title: 'mihomo_用户管理', icon: 'mdi-account-multiple', path: '/mihomo_clients' },
  { title: 'mihomo_出站管理', icon: 'mdi-cloud-upload', path: '/mihomo_outbounds' },
  { title: 'mihomo_TLS 设置', icon: 'mdi-certificate', path: '/mihomo_tls' },
  { title: 'mihomo_路由列表', icon: 'mdi-routes', path: '/mihomo_rules' },
  { title: 'mihomo_DNS', icon: 'mdi-dns', path: '/mihomo_dns' },
  { title: 'pages.settings', icon: 'mdi-cog', path: '/settings' },
]

const visibleMenu = computed(() => menu.filter(item => !item.hidden))

const Logout = async () => {
  logout()
}

const menuLabel = (item: { title: string; prefix?: string }) => {
  const label = item.title.startsWith('pages.') ? i18n.global.t(item.title) : item.title
  return item.prefix ? `${item.prefix}${label}` : label
}
</script>
