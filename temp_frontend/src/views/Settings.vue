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


