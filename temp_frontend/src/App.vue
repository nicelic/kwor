<template>
  <v-app style="overflow: auto;">
    <v-overlay
      :model-value="loading"
      persistent
      content-class="text-center"
      class="align-center justify-center"
    >
      <v-progress-circular
        indeterminate
        size="64"
      ></v-progress-circular>
      <br />
      {{ $t('loading') }}
    </v-overlay>
    <Message />
    <ConfirmDialog />
    <router-view />
  </v-app>
</template>

<script lang="ts" setup>
import Message from '@/components/message.vue'
import ConfirmDialog from '@/layouts/modals/ConfirmDialog.vue'
import { inject, ref, Ref, watch } from 'vue'
import { useRoute } from 'vue-router'

const loading:Ref = inject('loading')?? ref(false)
const route = useRoute()

watch(() => route.path, path => {
  if (path === '/login') {
    loading.value = false
  }
}, { immediate: true })

// Change page title
document.title = "kwor " + document.location.hostname
</script>

<style>
.v-overlay .v-list-item,
.v-field__input {
  direction: ltr;
}
</style>
