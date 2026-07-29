<template>
  <Notivue v-slot="item">
    <NotivueSwipe :item="item">
      <Notification
        :item="item"
        :dir="notificationDirection(item)"
        :theme="theme"
        :icons="outlinedIcons"
        :hideClose="false"
        @click="item.clear"
      />
    </NotivueSwipe>
  </Notivue>
</template>

<script lang="ts" setup>
import { Notivue, Notification, NotivueSwipe, outlinedIcons, pastelTheme, darkTheme } from 'notivue'
import { computed } from 'vue'
import { useTheme } from 'vuetify'
import vuetify from '@/plugins/vuetify'

const Theme = useTheme()

const theme = computed(() => {
  let currentTheme = Theme.global.name.value == "light" ? pastelTheme : darkTheme
  currentTheme = {
    ...currentTheme,
    '--nv-width': 'min(24rem, calc(100vw - 2.5rem))',
  }
  return currentTheme
})

const direction = computed<'ltr' | 'rtl'>(() => {
  return vuetify.locale.isRtl ? 'rtl' : 'ltr'
})

type NotificationDirectionItem = {
  message?: unknown
  title?: unknown
}

const rtlCharacter = /[\u0590-\u08FF\uFB1D-\uFEFC]/
const ltrCharacter = /[A-Za-z0-9\u00C0-\u058F\u0900-\uF8FF]/

function textDirection(value: unknown): 'ltr' | 'rtl' | undefined {
  if (typeof value !== 'string') return undefined

  for (const character of value) {
    if (rtlCharacter.test(character)) return 'rtl'
    if (ltrCharacter.test(character)) return 'ltr'
  }

  return undefined
}

function notificationDirection(item: NotificationDirectionItem): 'ltr' | 'rtl' {
  return textDirection(item.message) ?? textDirection(item.title) ?? direction.value
}
</script>

<style>
:root {
  --nv-z: 10020;
}

.Notivue__content {
  min-width: 0;
}

.Notivue__content-message {
  overflow-wrap: anywhere;
}

.Notivue__notification[dir='ltr'] {
  --tip-width-fx: 1;
}

.Notivue__notification[dir='ltr'] .Notivue__icon {
  margin: var(--nv-spacing) 0 var(--nv-spacing) var(--nv-spacing);
}

.Notivue__notification[dir='ltr'] .Notivue__close {
  margin: var(--nv-spacing) var(--nv-spacing) var(--nv-spacing) 0;
}
</style>
