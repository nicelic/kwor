<template>
  <div class="settings-subscription-manage">
    <!-- Card 1: 订阅网络与端点配置 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
      <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-server-network</v-icon>
          <span>{{ $t('setting.subNetworkConfig') }}</span>
        </div>
        <v-chip size="x-small" color="info" variant="outlined">
          {{ $t('setting.restartRequiredBadge') }}
        </v-chip>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <v-row>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="settings.subListen"
              :label="$t('setting.addr')"
              :hint="$t('setting.subListenHint')"
              persistent-hint
              density="comfortable"
              prepend-inner-icon="mdi-ip-network-outline"
            />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="settings.subPort"
              type="number"
              min="1"
              max="65535"
              :label="$t('setting.port')"
              :hint="$t('setting.subPortHint')"
              persistent-hint
              density="comfortable"
              prepend-inner-icon="mdi-numeric"
            />
          </v-col>
          <v-col cols="12" sm="12" md="4">
            <v-text-field
              v-model="settings.subDomain"
              :label="$t('setting.domain')"
              :hint="$t('setting.subDomainHint')"
              persistent-hint
              density="comfortable"
              prepend-inner-icon="mdi-domain"
            />
          </v-col>

          <v-col cols="12">
            <div class="d-flex align-center justify-space-between mb-1 flex-wrap ga-2">
              <label class="text-body-2 text-medium-emphasis font-weight-medium">
                {{ $t('setting.path') }}
              </label>
              <v-btn
                size="small"
                variant="tonal"
                color="primary"
                prepend-icon="mdi-refresh"
                @click="generateRandomPath"
              >
                {{ $t('setting.subRandomPathBtn') }}
              </v-btn>
            </div>
            <v-text-field
              v-model="settings.subPath"
              placeholder="/abc123-def456-ghi789/"
              density="comfortable"
              prepend-inner-icon="mdi-folder-key-network-outline"
              hide-details
            />
            <v-slide-y-transition>
              <v-alert
                v-if="isSubPathModified"
                type="warning"
                variant="tonal"
                density="compact"
                class="mt-2"
                icon="mdi-alert-outline"
              >
                {{ $t('setting.subPathChangeWarning') }}
              </v-alert>
            </v-slide-y-transition>
          </v-col>

          <v-col cols="12">
            <v-text-field
              v-model="settings.subURI"
              :label="$t('setting.subUri')"
              :hint="$t('setting.subUriHint')"
              persistent-hint
              density="comfortable"
              prepend-inner-icon="mdi-link-variant"
            />
          </v-col>
        </v-row>

        <v-alert type="info" variant="tonal" density="compact" class="mt-4" icon="mdi-information-outline">
          {{ $t('setting.subRestartRequiredHint') }}
        </v-alert>
      </v-card-text>
    </v-card>

    <!-- Card 2: 订阅分发与客户端行为 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-green">
      <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-tune-variant</v-icon>
          <span>{{ $t('setting.subClientConfig') }}</span>
        </div>
        <v-chip size="x-small" color="success" variant="outlined">
          {{ $t('setting.instantEffectBadge') }}
        </v-chip>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <v-row>
          <v-col cols="12" md="6">
            <v-switch
              v-model="subEncode"
              color="primary"
              :label="$t('setting.subEncode')"
              hide-details
            />
            <div class="text-caption text-medium-emphasis mt-1">
              {{ $t('setting.subEncodeHint') }}
            </div>
          </v-col>

          <v-col cols="12" md="6">
            <v-switch
              v-model="subShowInfo"
              color="primary"
              :label="$t('setting.subInfo')"
              hide-details
            />
            <div class="text-caption text-medium-emphasis mt-1">
              {{ $t('setting.subInfoHint') }}
            </div>
          </v-col>

          <v-col cols="12">
            <div class="d-flex align-center justify-space-between mb-2 flex-wrap ga-2">
              <label class="text-body-2 text-medium-emphasis font-weight-medium">
                {{ $t('setting.update') }}
              </label>
              <div class="d-flex align-center ga-1 flex-wrap">
                <span class="text-caption text-medium-emphasis mr-1">快捷预设:</span>
                <v-chip
                  v-for="hours in [6, 12, 24, 48]"
                  :key="hours"
                  size="x-small"
                  variant="outlined"
                  color="primary"
                  class="cursor-pointer"
                  @click="applyPresetUpdates(hours)"
                >
                  {{ hours }}h
                </v-chip>
              </div>
            </div>
            <v-text-field
              v-model="settings.subUpdates"
              type="number"
              min="1"
              :suffix="$t('date.h')"
              :hint="$t('setting.subUpdatesHint')"
              persistent-hint
              density="comfortable"
              prepend-inner-icon="mdi-clock-outline"
            />
          </v-col>
        </v-row>
      </v-card-text>
    </v-card>

    <!-- Card 3: 订阅基准链接实时预览与工具箱 -->
    <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
      <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
        <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
          <v-icon size="small" color="primary">mdi-link-box-outline</v-icon>
          <span>{{ $t('setting.subPreviewTitle') }}</span>
        </div>
        <v-chip
          size="x-small"
          :color="isCustomUriMode ? 'primary' : 'secondary'"
          variant="tonal"
        >
          {{ isCustomUriMode ? $t('setting.subPreviewCustomUriBadge') : $t('setting.subPreviewDynamicBadge') }}
        </v-chip>
      </v-card-title>
      <v-divider />
      <v-card-text class="pt-4">
        <div class="sub-preview-box pa-3 rounded-lg d-flex align-center justify-space-between ga-2 flex-wrap">
          <div class="sub-preview-text font-monospace text-body-2 text-primary font-weight-medium">
            {{ calculatedBaseURI }}
          </div>
          <v-btn
            size="small"
            color="primary"
            variant="tonal"
            prepend-icon="mdi-content-copy"
            @click="copyBaseURI"
          >
            {{ $t('actions.copy') || '复制' }}
          </v-btn>
        </div>
        <div class="text-caption text-medium-emphasis mt-2">
          {{ $t('setting.subPreviewHint') }}
        </div>
      </v-card-text>
    </v-card>
  </div>
</template>

<script lang="ts" setup>
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { push } from 'notivue'

const props = defineProps<{
  settings: Record<string, string>
  loading?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:settings', val: Record<string, string>): void
}>()

const { t } = useI18n()

const initialSubPath = ref('')

onMounted(() => {
  initialSubPath.value = props.settings.subPath || ''
})

const isSubPathModified = computed(() => {
  return initialSubPath.value !== '' && props.settings.subPath !== initialSubPath.value
})

const subEncode = computed({
  get: () => props.settings.subEncode === 'true',
  set: (v: boolean) => {
    props.settings.subEncode = v ? 'true' : 'false'
  },
})

const subShowInfo = computed({
  get: () => props.settings.subShowInfo === 'true',
  set: (v: boolean) => {
    props.settings.subShowInfo = v ? 'true' : 'false'
  },
})

// 生成与后端完全相同的 /xxx000-xxx000-xxx000/ 随机安全路径
const generateRandomPath = () => {
  const letters = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz'
  let result = '/'
  for (let segment = 0; segment < 3; segment++) {
    if (segment > 0) result += '-'
    for (let i = 0; i < 3; i++) {
      result += letters.charAt(Math.floor(Math.random() * letters.length))
    }
    for (let i = 0; i < 3; i++) {
      result += Math.floor(Math.random() * 10).toString()
    }
  }
  result += '/'
  props.settings.subPath = result
  push.info({
    title: t('actions.set') || '设置',
    message: t('setting.subRandomPathGenerated'),
  })
}

const applyPresetUpdates = (hours: number) => {
  props.settings.subUpdates = String(hours)
}

const isCustomUriMode = computed(() => {
  return !!props.settings.subURI?.trim()
})

const calculatedBaseURI = computed(() => {
  const custom = props.settings.subURI?.trim()
  if (custom) {
    return custom.endsWith('/') ? custom : `${custom}/`
  }

  const hostname = (typeof window !== 'undefined' && window.location?.hostname) ? window.location.hostname : '127.0.0.1'
  const rawDomain = props.settings.subDomain?.trim() || hostname
  const host = rawDomain.includes(':') && !rawDomain.startsWith('[')
    ? `[${rawDomain}]`
    : rawDomain

  const protocol = (typeof window !== 'undefined' && window.location?.protocol) ? window.location.protocol : 'http:'
  const isHttps = protocol === 'https:'
  const port = props.settings.subPort?.trim() || ''

  let portSuffix = ''
  if (port) {
    if (!((port === '80' && !isHttps) || (port === '443' && isHttps))) {
      portSuffix = `:${port}`
    }
  }

  let path = props.settings.subPath?.trim() || '/'
  if (!path.startsWith('/')) path = `/${path}`
  if (!path.endsWith('/')) path = `${path}/`

  return `${protocol}//${host}${portSuffix}${path}`
})

const copyBaseURI = async () => {
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(calculatedBaseURI.value)
    } else {
      const input = document.createElement('input')
      input.value = calculatedBaseURI.value
      document.body.appendChild(input)
      input.select()
      document.execCommand('copy')
      document.body.removeChild(input)
    }
    push.success({
      title: t('success') || '成功',
      message: t('setting.subPreviewCopySuccess'),
    })
  } catch {
    push.error({
      title: t('error') || '错误',
      message: t('setting.subPreviewCopyFailed'),
    })
  }
}
</script>

<style scoped>
.settings-subscription-manage {
  width: 100%;
}

.sub-preview-box {
  background-color: rgba(var(--v-theme-primary), 0.04);
  border: 1px dashed rgba(var(--v-theme-primary), 0.4);
}

.sub-preview-text {
  word-break: break-all;
  overflow-wrap: anywhere;
  user-select: all;
}

.cursor-pointer {
  cursor: pointer;
}

.card-cyan {
  border-color: rgba(34, 211, 238, 0.45) !important;
  transition: border-color 0.2s ease;
}
.card-cyan:hover {
  border-color: rgba(34, 211, 238, 0.75) !important;
}
.card-green {
  border-color: rgba(74, 222, 128, 0.45) !important;
  transition: border-color 0.2s ease;
}
.card-green:hover {
  border-color: rgba(74, 222, 128, 0.75) !important;
}
</style>
