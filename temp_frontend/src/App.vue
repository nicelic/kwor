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
    <v-snackbar
      v-if="route.path !== '/login'"
      :model-value="dataStore.runtimeRetryPending"
      :timeout="-1"
      color="warning"
      location="bottom"
      multi-line
    >
      运行配置未刷新，数据库数据已保存
      <template #actions>
        <v-btn
          variant="text"
          :loading="dataStore.runtimeRetryBusy"
          :disabled="dataStore.runtimeRetryBusy"
          @click="dataStore.retrySingboxRuntime()"
        >重试</v-btn>
      </template>
    </v-snackbar>
    <router-view />
  </v-app>
</template>

<script lang="ts" setup>
import Message from '@/components/message.vue'
import ConfirmDialog from '@/layouts/modals/ConfirmDialog.vue'
import { inject, ref, Ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import Data from '@/store/modules/data'

const loading:Ref = inject('loading')?? ref(false)
const route = useRoute()
const dataStore = Data()

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
