<template>
  <v-card :loading="loading">
    <v-tabs
      v-model="tab"
      color="primary"
      align-tabs="center"
      show-arrows
    >
      <v-tab value="t1">{{ $t('setting.interface') }}</v-tab>
      <v-tab value="t2">{{ $t('setting.sub') }}</v-tab>
      <v-tab value="t3">{{ $t('setting.jsonSub') }}</v-tab>
      <v-tab value="t4">{{ $t('setting.clashSub') }}</v-tab>
      <v-tab value="t5">Language</v-tab>
      <v-tab value="t6">{{ $t('setting.trafficManage') }}</v-tab>
      <v-tab value="t7">防火墙</v-tab>
      <v-tab value="t8">转发</v-tab>
      <v-tab value="t9">优化</v-tab>
      <v-tab value="t10">证书管理</v-tab>
      <v-tab value="t11">反向代理</v-tab>
      <v-tab value="t12">{{ $t('setting.kernelManage') }}</v-tab>
      <v-tab value="t13">监控</v-tab>
    </v-tabs>

    <v-card-text>
      <v-row v-if="showTopActionBar" align="center" justify="center" style="margin-bottom: 10px;">
        <v-col cols="auto">
          <v-btn color="primary" @click="save" :loading="loading" :disabled="!stateChange">
            {{ $t('actions.save') }}
          </v-btn>
        </v-col>
        <v-col cols="auto">
          <v-btn variant="outlined" color="warning" @click="restartApp" :loading="loading" :disabled="stateChange">
            {{ $t('actions.restartApp') }}
          </v-btn>
        </v-col>
        <v-col cols="auto" v-if="showSubPageResetButton">
          <v-btn variant="outlined" color="error" @click="openResetDialog" :disabled="loading">
            {{ resetButtonText }}
          </v-btn>
        </v-col>
      </v-row>

      <v-dialog v-model="resetDialogVisible" max-width="460">
        <v-card>
          <v-card-title>确认重置</v-card-title>
          <v-card-text>{{ resetDialogMessage }}</v-card-text>
          <v-card-actions>
            <v-spacer></v-spacer>
            <v-btn variant="text" @click="closeResetDialog">取消</v-btn>
            <v-btn color="error" variant="outlined" @click="confirmResetSubPage">确定重置</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <v-window v-model="tab">
        <v-window-item value="t1">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webListen" :label="$t('setting.addr')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webPort" min="1" type="number" :label="$t('setting.port')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webPath" :label="$t('setting.webPath')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webDomain" :label="$t('setting.domain')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webURI" :label="$t('setting.webUri')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                type="number"
                v-model.number="sessionMaxAge"
                min="0"
                :label="$t('setting.sessionAge')"
                :suffix="$t('date.m')"
                hide-details
              ></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                type="number"
                v-model.number="trafficAge"
                min="0"
                :label="$t('setting.trafficAge')"
                :suffix="$t('date.d')"
                hide-details
              ></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.timeLocation" :label="$t('setting.timeLoc')" hide-details></v-text-field>
            </v-col>
          </v-row>

          <v-divider class="my-6"></v-divider>

          <v-row align="center" class="mb-2">
            <v-col cols="12" class="d-flex align-center flex-wrap" style="gap: 8px;">
              <v-chip variant="outlined" color="success" size="small" label>
                <v-progress-circular
                  v-if="panelStatusLoading"
                  indeterminate
                  size="12"
                  width="2"
                  class="mr-1"
                ></v-progress-circular>
                本地: {{ panelLocalVersionLabel }}
              </v-chip>
              <v-chip variant="outlined" color="info" size="small" label>
                <v-progress-circular
                  v-if="panelRemoteLoading"
                  indeterminate
                  size="12"
                  width="2"
                  class="mr-1"
                ></v-progress-circular>
                远程: {{ panelRemoteVersionLabel }}
              </v-chip>
              <v-chip v-if="panelBinaryName" variant="tonal" size="small" label>
                文件: {{ panelBinaryName }}
                <v-tooltip
                  v-if="panelUpdateStatus?.binaryPath"
                  activator="parent"
                  location="top"
                  :text="panelUpdateStatus.binaryPath"
                />
              </v-chip>
            </v-col>
          </v-row>

          <v-row align="center">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="panelSelectedVersion"
                :items="panelVersionItems"
                item-title="title"
                item-value="value"
                label="选择 kwor 版本"
                variant="outlined"
                density="compact"
                hide-details
                :loading="panelRemoteLoading"
                :disabled="panelVersionItems.length === 0"
                :menu-props="{ maxHeight: 260 }"
              >
                <template #item="{ props: itemProps, item }">
                  <v-list-item
                    v-bind="itemProps"
                    :subtitle="item.raw.assetName || undefined"
                  >
                    <template #append>
                      <v-chip
                        v-if="item.raw.prerelease"
                        size="x-small"
                        color="warning"
                        variant="flat"
                      >
                        预发布
                      </v-chip>
                    </template>
                  </v-list-item>
                </template>
              </v-select>
            </v-col>

            <v-col cols="auto">
              <v-btn
                color="secondary"
                variant="tonal"
                prepend-icon="mdi-refresh"
                :loading="panelRemoteLoading"
                :disabled="panelRemoteLoading || panelLoadingMoreVersions || panelInstalling"
                @click="checkPanelUpdates"
              >
                检查更新
              </v-btn>
            </v-col>

            <v-col cols="auto">
              <v-btn
                color="primary"
                variant="flat"
                prepend-icon="mdi-download"
                :loading="panelInstalling"
                :disabled="!panelSelectedVersion || panelInstalling || panelRemoteLoading"
                @click="openPanelInstallDialog"
              >
                安装
              </v-btn>
            </v-col>
          </v-row>

          <v-row v-if="panelVersionItems.length > 0 || panelUpdateFeedback" class="mt-1">
            <v-col cols="12" class="d-flex align-center justify-space-between flex-wrap" style="gap: 8px;">
              <span v-if="panelVersionItems.length > 0" class="text-caption text-medium-emphasis">
                已加载 {{ panelVersionItems.length }} 个版本
              </span>
              <div class="d-flex align-center" style="gap: 8px;">
                <v-btn
                  v-if="panelHasMoreVersions"
                  size="x-small"
                  variant="text"
                  :loading="panelLoadingMoreVersions"
                  :disabled="panelInstalling"
                  @click="loadMorePanelVersions"
                >
                  加载更多
                </v-btn>
                <v-btn
                  v-if="panelVersionItems.length > 5"
                  size="x-small"
                  variant="text"
                  :disabled="panelInstalling"
                  @click="resetPanelVersions"
                >
                  只看最新 5 个
                </v-btn>
              </div>
            </v-col>
            <v-col v-if="panelUpdateFeedback" cols="12">
              <v-alert
                :type="panelUpdateFeedbackType"
                variant="tonal"
                density="compact"
                closable
                @click:close="panelUpdateFeedback = ''"
              >
                {{ panelUpdateFeedback }}
              </v-alert>
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t2">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-switch color="primary" v-model="subEncode" :label="$t('setting.subEncode')" hide-details />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-switch color="primary" v-model="subShowInfo" :label="$t('setting.subInfo')" hide-details />
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.subListen" :label="$t('setting.addr')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                type="number"
                v-model="settings.subPort"
                min="1"
                :label="$t('setting.port')"
                hide-details
              ></v-text-field>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.subDomain" :label="$t('setting.domain')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.subPath" :label="$t('setting.path')" hide-details></v-text-field>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                type="number"
                v-model.number="subUpdates"
                min="0"
                :label="$t('setting.update')"
                hide-details
              ></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.subURI" :label="$t('setting.subUri')" hide-details></v-text-field>
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t3" eager>
          <SubJsonExtVue
            v-if="subJsonTabBooted"
            ref="subJsonExtRef"
            :settings="settings"
          />
        </v-window-item>

        <v-window-item value="t4" eager>
          <SubClashExtVue
            v-if="subClashTabBooted"
            ref="subClashExtRef"
            :settings="settings"
          />
        </v-window-item>

        <v-window-item value="t5">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-select
                hide-details
                label="Language"
                :items="languages"
                v-model="$i18n.locale"
                @update:modelValue="changeLocale"
              >
              </v-select>
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t6">
          <SettingsTrafficManageVue :active="tab === 't6'" />
        </v-window-item>

        <v-window-item value="t7">
          <SettingsFirewallManageVue :active="tab === 't7'" />
        </v-window-item>

        <v-window-item value="t8">
          <SettingsPortForwardManageVue :active="tab === 't8'" />
        </v-window-item>

        <v-window-item value="t9">
          <SettingsOptimizationManageVue :active="tab === 't9'" />
        </v-window-item>

        <v-window-item value="t10">
          <SettingsAcmeManageVue :active="tab === 't10'" />
        </v-window-item>

        <v-window-item value="t11">
          <SettingsReverseProxyManageVue :active="tab === 't11'" />
        </v-window-item>

        <v-window-item value="t12">
          <SettingsKernelManageVue :active="tab === 't12'" />
        </v-window-item>

        <v-window-item value="t13">
          <SettingsMonitorManageVue :active="tab === 't13'" />
        </v-window-item>
      </v-window>

      <v-dialog v-model="panelInstallDialogVisible" max-width="480">
        <v-card>
          <v-card-title>确认安装</v-card-title>
          <v-card-text>
            确定要安装 {{ panelSelectedVersion || '-' }} 吗？当前面板会自动下载、停止、替换并重新启动。
          </v-card-text>
          <v-card-actions>
            <v-spacer></v-spacer>
            <v-btn variant="text" :disabled="panelInstalling" @click="panelInstallDialogVisible = false">取消</v-btn>
            <v-btn color="primary" variant="flat" :loading="panelInstalling" @click="installPanelVersion">确定安装</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>
    </v-card-text>
  </v-card>
</template>

<script lang="ts" setup>
import { useLocale } from 'vuetify'
import { i18n, languages } from '@/locales'
import { Ref, computed, inject, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import HttpUtils from '@/plugins/httputil'
import { FindDiff } from '@/plugins/utils'
import SubJsonExtVue from '@/components/SubJsonExt.vue'
import SubClashExtVue from '@/components/SubClashExt.vue'
import { push } from 'notivue'
import SettingsTrafficManageVue from '@/components/SettingsTrafficManage.vue'
import SettingsFirewallManageVue from '@/components/SettingsFirewallManage.vue'
import SettingsPortForwardManageVue from '@/components/SettingsPortForwardManage.vue'
import SettingsOptimizationManageVue from '@/components/SettingsOptimizationManage.vue'
import SettingsAcmeManageVue from '@/components/SettingsAcmeManage.vue'
import SettingsReverseProxyManageVue from '@/components/SettingsReverseProxyManage.vue'
import SettingsKernelManageVue from '@/components/SettingsKernelManage.vue'
import SettingsMonitorManageVue from '@/components/SettingsMonitorManage.vue'

const locale = useLocale()
const tab = ref('t1')
const loading: Ref = inject('loading') ?? ref(false)
const oldSettings = ref({})
const subJsonExtRef = ref<any>(null)
const subClashExtRef = ref<any>(null)
const subJsonTabBooted = ref(false)
const subClashTabBooted = ref(false)
const resetDialogVisible = ref(false)
const resetTarget = ref<'json' | 'clash' | ''>('')
const heavyTabWarmupTimers: number[] = []

type PanelVersionItem = {
  title: string
  value: string
  tagName: string
  name?: string
  prerelease?: boolean
  publishedAt?: string
  assetName?: string
  assetSize?: number
}

type PanelUpdateStatus = {
  localVersion?: string
  binaryPath?: string
  binaryName?: string
  installDir?: string
  serviceFilePath?: string
  serviceBinaryPath?: string
  runningBinaryPath?: string
  installSource?: string
  platform?: string
}

const panelStatusLoading = ref(false)
const panelRemoteLoading = ref(false)
const panelLoadingMoreVersions = ref(false)
const panelInstalling = ref(false)
const panelInstallDialogVisible = ref(false)
const panelUpdateStatus = ref<PanelUpdateStatus | null>(null)
const panelSelectedVersion = ref('')
const panelVersionItems = ref<PanelVersionItem[]>([])
const panelHasMoreVersions = ref(false)
const panelUpdateFeedback = ref('')
const panelUpdateFeedbackType = ref<'success' | 'error' | 'info' | 'warning'>('info')
const panelVersionRequestSeq = ref(0)

const settings = ref({
  webListen: '',
  webDomain: '',
  webPort: '8888',
  webPath: '/app/',
  webURI: '',
  panelAssignedCertificateRecordID: '0',
  panelAssignedCertificateRecordIDs: '[]',
  sessionMaxAge: '0',
  trafficAge: '30',
  timeLocation: 'Asia/Tehran',
  subListen: '',
  subPort: '22780',
  subPath: '',
  subDomain: '',
  subAssignedCertificateRecordID: '0',
  subAssignedCertificateRecordIDs: '[]',
  subUpdates: '12',
  subEncode: 'true',
  subShowInfo: 'false',
  subURI: '',
  serverTlsStoreEnabled: 'true',
  serverTlsStore: 'chrome',
  clientTlsStoreEnabled: 'true',
  clientTlsStore: 'chrome',
  subJsonExt: '',
  subClashExt: '',
})

const DEFAULT_WEB_PORT = '8888'
const DEFAULT_SUB_PORT = '22780'

onMounted(async () => {
  loadData()
  void loadPanelUpdateStatus()
})

const changeLocale = (l: any) => {
  locale.current.value = l ?? 'en'
  localStorage.setItem('locale', locale.current.value)
}

const loadData = async () => {
  loading.value = true
  const msg = await HttpUtils.get('api/settings')
  loading.value = false
  if (msg.success) {
    setData(msg.obj)
  }
}

const setData = (data: any) => {
  settings.value = data
  oldSettings.value = { ...data }
  void nextTick().then(scheduleHeavyTabWarmup)
}

const panelLocalVersionLabel = computed(() => {
  const version = String(panelUpdateStatus.value?.localVersion ?? '').trim()
  return version ? `v${version.replace(/^v/i, '')}` : '未知'
})

const panelBinaryName = computed(() => String(panelUpdateStatus.value?.binaryName ?? '').trim())

const panelRemoteVersionLabel = computed(() => {
  if (panelRemoteLoading.value) return '加载中'
  if (panelVersionItems.value.length > 0) return panelVersionItems.value[0].value
  return '未加载'
})

const normalizePanelVersionTag = (value: string) => {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return ''
  return trimmed.startsWith('v') ? trimmed : `v${trimmed}`
}

const loadPanelUpdateStatus = async () => {
  panelStatusLoading.value = true
  const msg = await HttpUtils.get('api/panel-update-status', {}, { silentAuthCheck: true })
  panelStatusLoading.value = false
  if (msg.success) {
    panelUpdateStatus.value = msg.obj ?? null
  }
}

const buildPanelVersionItems = (versions: any[]): PanelVersionItem[] => {
  const items: PanelVersionItem[] = []
  versions.forEach(item => {
    const tagName = normalizePanelVersionTag(item?.tag_name ?? item?.tagName ?? '')
    if (!tagName) return
    items.push({
      title: tagName,
      value: tagName,
      tagName,
      name: item?.name ?? '',
      prerelease: item?.prerelease === true,
      publishedAt: item?.published_at ?? item?.publishedAt ?? '',
      assetName: item?.asset_name ?? item?.assetName ?? '',
      assetSize: item?.asset_size ?? item?.assetSize ?? 0,
    })
  })
  return items
}

const applyPanelVersionResponse = (obj: any, append: boolean) => {
  const nextItems = buildPanelVersionItems(Array.isArray(obj?.versions) ? obj.versions : [])
  const existing = append ? [...panelVersionItems.value] : []
  const seen = new Set(existing.map(item => item.value))
  nextItems.forEach(item => {
    if (!seen.has(item.value)) {
      existing.push(item)
      seen.add(item.value)
    }
  })
  panelVersionItems.value = existing
  panelHasMoreVersions.value = obj?.has_more === true || obj?.hasMore === true
  if (!append && panelVersionItems.value.length > 0) {
    panelSelectedVersion.value = panelVersionItems.value[0].value
  } else if (!panelSelectedVersion.value && panelVersionItems.value.length > 0) {
    panelSelectedVersion.value = panelVersionItems.value[0].value
  }
}

const loadPanelVersions = async (append = false) => {
  const seq = panelVersionRequestSeq.value + 1
  panelVersionRequestSeq.value = seq
  if (append) {
    panelLoadingMoreVersions.value = true
  } else {
    panelRemoteLoading.value = true
  }

  const msg = await HttpUtils.get('api/panel-update-versions', {
    offset: append ? panelVersionItems.value.length : 0,
    limit: 5,
  }, { silentAuthCheck: true })

  if (seq !== panelVersionRequestSeq.value) {
    return
  }

  if (append) {
    panelLoadingMoreVersions.value = false
  } else {
    panelRemoteLoading.value = false
  }

  if (msg.success) {
    applyPanelVersionResponse(msg.obj, append)
    panelUpdateFeedback.value = append ? '已加载更多版本' : '检查完成，已选择最新版本'
    panelUpdateFeedbackType.value = 'success'
  } else if (msg.msg) {
    panelUpdateFeedback.value = msg.msg
    panelUpdateFeedbackType.value = 'error'
  }
}

const checkPanelUpdates = async () => {
  await loadPanelVersions(false)
}

const loadMorePanelVersions = async () => {
  await loadPanelVersions(true)
}

const resetPanelVersions = () => {
  panelVersionItems.value = panelVersionItems.value.slice(0, 5)
  panelHasMoreVersions.value = true
  if (panelVersionItems.value.length > 0) {
    panelSelectedVersion.value = panelVersionItems.value[0].value
  }
}

const openPanelInstallDialog = () => {
  if (!panelSelectedVersion.value) return
  panelInstallDialogVisible.value = true
}

const installPanelVersion = async () => {
  if (!panelSelectedVersion.value) return
  panelInstalling.value = true
  const version = panelSelectedVersion.value
  const msg = await HttpUtils.post('api/panel-update-install', { version }, { silentAuthCheck: true, timeout: 120000 })
  panelInstalling.value = false
  panelInstallDialogVisible.value = false

  if (msg.success) {
    panelUpdateFeedback.value = `已开始安装 ${version}，请等待面板自动重启后刷新页面。`
    panelUpdateFeedbackType.value = 'info'
    window.setTimeout(() => {
      window.location.reload()
    }, 12000)
  } else {
    panelUpdateFeedback.value = msg.msg || '安装失败'
    panelUpdateFeedbackType.value = 'error'
  }
}

const clearHeavyTabWarmupTimers = () => {
  heavyTabWarmupTimers.forEach(timerId => window.clearTimeout(timerId))
  heavyTabWarmupTimers.length = 0
}

const scheduleHeavyTabWarmup = () => {
  if (typeof window === 'undefined') return
  if (subJsonTabBooted.value && subClashTabBooted.value) return

  clearHeavyTabWarmupTimers()

  if (!subJsonTabBooted.value) {
    heavyTabWarmupTimers.push(window.setTimeout(() => {
      subJsonTabBooted.value = true
    }, 0))
  }

  if (!subClashTabBooted.value) {
    heavyTabWarmupTimers.push(window.setTimeout(() => {
      subClashTabBooted.value = true
    }, subJsonTabBooted.value ? 0 : 180))
  }
}

watch(tab, value => {
  if (value === 't3') {
    subJsonTabBooted.value = true
  } else if (value === 't4') {
    subClashTabBooted.value = true
  }
})

onBeforeUnmount(() => {
  clearHeavyTabWarmupTimers()
})

const save = async () => {
  applyPortDefaultsBeforeSave()
  const previousSettings = { ...settings.value }
  subJsonExtRef.value?.commitCustomRuleRows?.()
  subJsonExtRef.value?.commitDnsRouteRows?.()
  const canCommitClashDnsSuffix = subClashExtRef.value?.commitClashDnsSuffixSelections?.()
  if (canCommitClashDnsSuffix === false) return
  subClashExtRef.value?.commitClashRuleRows?.()
  subClashExtRef.value?.commitClashDnsPolicyRows?.()
  await nextTick()
  const payloadSettings = buildSettingsSavePayload(settings.value)
  loading.value = true
  const msg = await HttpUtils.post('api/save', { object: 'settings', action: 'set', data: JSON.stringify(payloadSettings) })
  if (msg.success) {
    push.success({
      title: i18n.global.t('success'),
      duration: 5000,
      message: i18n.global.t('actions.set') + ' ' + i18n.global.t('pages.settings'),
    })
    setData(msg.obj.settings)
    await maybeRedirectToHttps(msg.obj.settings, previousSettings)
  }
  loading.value = false
}

const currentResetTarget = computed<'json' | 'clash' | ''>(() => {
  if (tab.value === 't3') return 'json'
  if (tab.value === 't4') return 'clash'
  return ''
})

const showSubPageResetButton = computed(() => currentResetTarget.value !== '')

const resetButtonText = computed(() => {
  if (currentResetTarget.value === 'json') return '重置 JSON 订阅'
  if (currentResetTarget.value === 'clash') return '重置 CLASH 订阅'
  return ''
})

const resetDialogMessage = computed(() => {
  if (resetTarget.value === 'json') {
    return '确定要将 JSON 订阅页面恢复到初始默认状态吗？当前页面未保存的修改会丢失。'
  }
  if (resetTarget.value === 'clash') {
    return '确定要将 CLASH 订阅页面恢复到初始默认状态吗？当前页面未保存的修改会丢失。'
  }
  return ''
})

const openResetDialog = () => {
  if (!currentResetTarget.value) return
  resetTarget.value = currentResetTarget.value
  resetDialogVisible.value = true
}

const closeResetDialog = () => {
  resetDialogVisible.value = false
  resetTarget.value = ''
}

const confirmResetSubPage = async () => {
  if (resetTarget.value === 'json') {
    subJsonExtRef.value?.resetSubJsonPage?.()
  } else if (resetTarget.value === 'clash') {
    subClashExtRef.value?.resetSubClashPage?.()
  }

  await nextTick()

  if (resetTarget.value === 'json') {
    push.success({
      title: i18n.global.t('success'),
      duration: 4000,
      message: 'JSON 订阅页面已重置为默认状态',
    })
  } else if (resetTarget.value === 'clash') {
    push.success({
      title: i18n.global.t('success'),
      duration: 4000,
      message: 'CLASH 订阅页面已重置为默认状态',
    })
  }

  closeResetDialog()
}

const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))

const restartApp = async () => {
  loading.value = true
  const msg = await HttpUtils.post('api/restartApp', {})
  if (msg.success) {
    let url = settings.value.webURI
    if (!url || url === '') {
      const isTLS = isWebTLSEnabled(settings.value)
      url = buildURL(settings.value.webDomain, settings.value.webPort.toString(), isTLS, settings.value.webPath)
    }
    await sleep(3000)
    window.location.replace(url)
  }
  loading.value = false
}

const isWebTLSEnabled = (value: any) => {
  const multiRaw = String(value?.panelAssignedCertificateRecordIDs ?? '').trim()
  if (multiRaw !== '') {
    try {
      const parsed = JSON.parse(multiRaw)
      if (Array.isArray(parsed)) {
        const cleaned = parsed
          .map(item => Number.parseInt(String(item ?? '').trim(), 10))
          .filter(item => Number.isFinite(item) && item > 0)
        if (cleaned.length > 0) {
          return true
        }
      }
    } catch {
      // fallback to legacy key below
    }
  }
  const raw = String(value?.panelAssignedCertificateRecordID ?? '').trim()
  return raw !== '' && raw !== '0'
}

const maybeRedirectToHttps = async (nextSettings: any, previousSettings: any) => {
  if (window.location.protocol !== 'http:') return
  if (!isWebTLSEnabled(nextSettings)) return
  if (isWebTLSEnabled(previousSettings)) return

  let url = nextSettings.webURI
  if (!url || url === '') {
    url = buildURL(nextSettings.webDomain, nextSettings.webPort.toString(), true, nextSettings.webPath)
  }
  await sleep(1200)
  window.location.replace(url)
}

const buildURL = (host: string, port: string, isTLS: boolean, path: string) => {
  if (!host || host.length === 0) host = window.location.hostname
  if (!port || port.length === 0) port = window.location.port

  const protocol = isTLS ? 'https:' : 'http:'

  if (port === '' || (isTLS && port === '443') || (!isTLS && port === '80')) {
    port = ''
  } else {
    port = `:${port}`
  }

  return `${protocol}//${host}${port}${path}settings`
}

const normalizePort = (value: unknown, defaultValue: string) => {
  const strValue = typeof value === 'string' ? value.trim() : String(value ?? '').trim()
  if (strValue === '') return defaultValue
  const parsed = Number.parseInt(strValue, 10)
  if (!Number.isFinite(parsed) || parsed <= 0) return defaultValue
  return parsed.toString()
}

const applyPortDefaultsBeforeSave = () => {
  settings.value.webPort = normalizePort(settings.value.webPort, DEFAULT_WEB_PORT)
  settings.value.subPort = normalizePort(settings.value.subPort, DEFAULT_SUB_PORT)
}

const buildSettingsSavePayload = (value: Record<string, any>) => {
  const payload = { ...value }
  // Certificate binding IDs are controlled by certificate center apply actions.
  delete payload.panelAssignedCertificateRecordID
  delete payload.panelAssignedCertificateRecordIDs
  delete payload.subAssignedCertificateRecordID
  delete payload.subAssignedCertificateRecordIDs
  return payload
}

const subEncode = computed({
  get: () => {
    return settings.value.subEncode === 'true'
  },
  set: (v: boolean) => {
    settings.value.subEncode = v ? 'true' : 'false'
  },
})

const subShowInfo = computed({
  get: () => {
    return settings.value.subShowInfo === 'true'
  },
  set: (v: boolean) => {
    settings.value.subShowInfo = v ? 'true' : 'false'
  },
})

const sessionMaxAge = computed({
  get: () => {
    return settings.value.sessionMaxAge.length > 0 ? parseInt(settings.value.sessionMaxAge) : 0
  },
  set: (v: number) => {
    settings.value.sessionMaxAge = v > 0 ? v.toString() : '0'
  },
})

const trafficAge = computed({
  get: () => {
    return settings.value.trafficAge.length > 0 ? parseInt(settings.value.trafficAge) : 0
  },
  set: (v: number) => {
    settings.value.trafficAge = v > 0 ? v.toString() : '0'
  },
})

const subUpdates = computed({
  get: () => {
    return settings.value.subUpdates.length > 0 ? parseInt(settings.value.subUpdates) : 12
  },
  set: (v: number) => {
    settings.value.subUpdates = v > 0 ? v.toString() : '12'
  },
})

const stateChange = computed(() => {
  return !FindDiff.deepCompare(settings.value, oldSettings.value)
})

const showTopActionBar = computed(() => tab.value !== 't6' && tab.value !== 't7' && tab.value !== 't8' && tab.value !== 't9' && tab.value !== 't10' && tab.value !== 't11' && tab.value !== 't12' && tab.value !== 't13')
</script>


