<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="90%" max-width="520">
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row>
          <v-col>{{ $t('main.backup.title') }}</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <v-icon icon="mdi-close" @click="closeDialog" />
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text class="pt-5">
        <div class="text-body-2 text-medium-emphasis mb-4">
          {{ $t('main.backup.desc') }}
        </div>
        <v-row>
          <v-col cols="12" sm="6">
            <v-btn
              color="primary"
              block
              :loading="downloading"
              :disabled="busy"
              @click="downloadBackup">
              {{ $t('main.backup.backup') }}
            </v-btn>
          </v-col>
          <v-col cols="12" sm="6">
            <v-btn
              color="primary"
              block
              :loading="restoring"
              :disabled="busy"
              @click="restoreBackup">
              {{ $t('main.backup.restore') }}
            </v-btn>
          </v-col>
        </v-row>
      </v-card-text>
    </v-card>
  </v-dialog>

  <v-overlay :model-value="overlayVisible" class="align-center justify-center" persistent>
    <v-card width="420" rounded="lg">
      <v-card-text class="text-center py-8">
        <v-progress-circular indeterminate size="52" width="5" color="primary" class="mb-4" />
        <div class="text-subtitle-1 font-weight-medium">{{ overlayTitle }}</div>
        <div class="text-caption text-medium-emphasis mt-2">{{ overlayDesc }}</div>
      </v-card-text>
    </v-card>
  </v-overlay>
</template>

<script lang="ts" setup>
import api from '@/plugins/api'
import HttpUtils from '@/plugins/httputil'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { i18n } from '@/locales'
import { push } from 'notivue'

type ControlState = {
  visible: boolean
}

const props = defineProps<{
  control: ControlState
  visible: boolean
}>()

const downloading = ref(false)
const restoring = ref(false)
const overlayVisible = ref(false)
const overlayTitle = ref('')
const overlayDesc = ref('')
const reconnectTimerId = ref<number | null>(null)

const busy = computed(() => downloading.value || restoring.value)
const dialogVisible = computed({
  get: () => props.visible,
  set: (value: boolean) => {
    if (!value && busy.value) return
    props.control.visible = value
  },
})

const t = (key: string) => i18n.global.t(key)

const closeDialog = () => {
  dialogVisible.value = false
}

const clearReconnectTimer = () => {
  if (reconnectTimerId.value !== null) {
    window.clearTimeout(reconnectTimerId.value)
    reconnectTimerId.value = null
  }
}

const clearOverlay = () => {
  overlayVisible.value = false
  overlayTitle.value = ''
  overlayDesc.value = ''
}

const applyOverlay = (title: string, desc: string) => {
  overlayTitle.value = title
  overlayDesc.value = desc
  overlayVisible.value = true
}

const startReconnectPolling = () => {
  clearReconnectTimer()
  applyOverlay(t('main.backup.restartingTitle'), t('main.backup.restartingDesc'))

  const poll = async () => {
    try {
      const resp = await fetch('./api/session', {
        method: 'GET',
        credentials: 'include',
        cache: 'no-store',
        headers: {
          'X-Requested-With': 'XMLHttpRequest',
        },
      })
      if (resp.ok) {
        const body = await resp.json()
        if (body?.success === true) {
          window.location.reload()
          return
        }
        if (typeof body?.msg === 'string' && body.msg === 'Invalid login') {
          window.location.replace('./login')
          return
        }
      }
    } catch {
      // 等待面板恢复连接
    }
    reconnectTimerId.value = window.setTimeout(poll, 3500)
  }

  reconnectTimerId.value = window.setTimeout(poll, 5000)
}

const parseJSONBlobMessage = async (blob: Blob): Promise<string | null> => {
  try {
    const text = await blob.text()
    const payload = JSON.parse(text) as { success?: boolean; msg?: string }
    if (typeof payload?.msg === 'string' && payload.msg.trim()) {
      return payload.msg.trim()
    }
  } catch {
    return null
  }
  return null
}

const triggerBrowserDownload = (blob: Blob, fileName: string) => {
  const url = window.URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = fileName
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  window.setTimeout(() => window.URL.revokeObjectURL(url), 1000)
}

const readDownloadFileName = (disposition: string | undefined): string => {
  const raw = String(disposition || '')
  const match = raw.match(/filename\*?=(?:UTF-8'')?("?)([^";]+)\1/i)
  if (match?.[2]) {
    return decodeURIComponent(match[2])
  }
  return `kwor_db_backup_${Date.now()}.zip`
}

const downloadBackup = async () => {
  downloading.value = true
  applyOverlay(t('main.backup.downloadingTitle'), t('main.backup.downloadingDesc'))

  try {
    const response = await api.get('api/download-db-backup', {
      responseType: 'blob',
    })
    const contentType = String(response.headers['content-type'] || '').toLowerCase()
    const blob = response.data as Blob

    if (contentType.includes('application/json')) {
      const errorMsg = await parseJSONBlobMessage(blob)
      throw new Error(errorMsg || t('main.backup.downloadFailed'))
    }

    const fileName = readDownloadFileName(response.headers['content-disposition'])
    triggerBrowserDownload(blob, fileName)
    push.success({
      message: t('main.backup.downloaded'),
      duration: 5000,
    })
    props.control.visible = false
  } catch (error) {
    push.error({
      message: error instanceof Error ? error.message : t('main.backup.downloadFailed'),
      duration: 5000,
    })
  } finally {
    downloading.value = false
    if (!restoring.value) {
      clearOverlay()
    }
  }
}

const restoreBackup = () => {
  const fileInput = document.createElement('input')
  fileInput.type = 'file'
  fileInput.accept = '.zip,application/zip'

  fileInput.addEventListener('change', async (event: Event) => {
    const input = event.target as HTMLInputElement
    const backupFile = input.files?.[0] ?? null
    if (!backupFile) {
      return
    }

    restoring.value = true
    applyOverlay(t('main.backup.restoringTitle'), t('main.backup.restoringDesc'))

    const formData = new FormData()
    formData.append('backup', backupFile)

    try {
      props.control.visible = false
      const msg = await HttpUtils.post('api/restore-db-backup', formData, { silentAuthCheck: true })

      if (!msg.success) {
        push.error({
          message: String(msg.msg || t('main.backup.restoreFailed')),
          duration: 6000,
        })
        clearOverlay()
        return
      }

      applyOverlay(t('main.backup.restartingTitle'), t('main.backup.restartingDesc'))
      startReconnectPolling()
    } catch (error) {
      push.error({
        message: error instanceof Error ? error.message : t('main.backup.restoreFailed'),
        duration: 6000,
      })
      clearOverlay()
    } finally {
      restoring.value = false
    }
  })

  fileInput.click()
}

watch(
  () => props.visible,
  (value) => {
    if (!value && !restoring.value && !downloading.value) {
      clearOverlay()
    }
  },
)

onBeforeUnmount(() => {
  clearReconnectTimer()
})
</script>
