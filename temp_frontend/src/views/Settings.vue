<template>
  <v-card :loading="loading">
    <v-tabs
      v-if="hasVerifiedSettings"
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
      <v-tab value="t13">DDNS</v-tab>
      <v-tab value="t14">{{ $t('setting.dnsManage') }}</v-tab>
    </v-tabs>

    <v-card-text>
      <v-row v-if="hasVerifiedSettings && showTopActionBar" align="center" justify="center" style="margin-bottom: 10px;">
        <v-col cols="auto">
          <v-btn color="primary" @click="save" :loading="loading" :disabled="!stateChange || panelLifecycleBusy || !hasVerifiedSettings">
            {{ $t('actions.save') }}
          </v-btn>
        </v-col>
        <v-col cols="auto">
          <v-btn variant="outlined" color="warning" @click="restartApp" :loading="loading" :disabled="stateChange || panelLifecycleBusy || !hasVerifiedSettings || !panelCanRestart">
            {{ $t('actions.restartApp') }}
          </v-btn>
        </v-col>
        <v-col cols="auto" v-if="showSubPageResetButton">
          <v-btn variant="outlined" color="error" @click="openResetDialog" :disabled="loading || panelLifecycleBusy">
            {{ resetButtonText }}
          </v-btn>
        </v-col>
      </v-row>

      <v-dialog v-if="hasVerifiedSettings" v-model="resetDialogVisible" max-width="460">
        <v-card>
		  <v-card-title>{{ $t('subscriptionEditor.resetConfirmTitle') }}</v-card-title>
          <v-card-text>{{ resetDialogMessage }}</v-card-text>
          <v-card-actions>
            <v-spacer></v-spacer>
			<v-btn variant="text" :disabled="loading" @click="closeResetDialog">{{ $t('actions.close') }}</v-btn>
			<v-btn color="error" variant="outlined" :loading="loading" :disabled="loading" @click="confirmResetSubPage">{{ $t('subscriptionEditor.resetConfirm') }}</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <v-row v-if="!hasVerifiedSettings" justify="center" class="py-6">
        <v-col cols="12" sm="10" md="8" lg="7">
          <v-alert :type="settingsLoadState === 'error' ? 'error' : 'info'" variant="tonal">
            <div v-if="settingsLoadState === 'error'" class="text-body-2">
              {{ $t('setting.settingsLoadFailed') }}：{{ settingsLoadError || $t('setting.settingsLoadFallback') }}
            </div>
            <div v-else class="d-flex align-center flex-wrap" style="gap: 10px;">
              <v-progress-circular indeterminate size="20" width="2" />
              <span>{{ $t('setting.settingsLoading') }}</span>
            </div>
            <div v-if="settingsLoadState === 'error'" class="d-flex flex-wrap mt-3" style="gap: 8px;">
              <v-btn color="primary" variant="outlined" @click="retryLoadData">
                {{ $t('setting.settingsReload') }}
              </v-btn>
            </div>
          </v-alert>
        </v-col>
      </v-row>

      <v-window v-else v-model="tab">
        <v-window-item value="t1">
          <SettingsInterfaceManageVue
            :settings="settings"
            v-model:system-time-location="systemTimeLocation"
            :system-time-zone-status="systemTimeZoneStatus"
            :system-time-zone-load-state="systemTimeZoneLoadState"
            :system-time-zone-load-error="systemTimeZoneLoadError"
            :time-zone-options="timeZoneOptions"
            :hidden-panel-time-location="hiddenPanelTimeLocation"
            :session-age-unit-items="sessionAgeUnitItems"
            :panel-can-restart="panelCanRestart"
            :panel-restart-hint="panelRestartHint"
            :disabled="loading"
            @retry-system-timezone="retryLoadSystemTimeZone"
            @system-time-location-selected="onSystemTimeLocationSelected"
            @busy-change="onInterfaceBusyChange"
            @can-restart-change="onInterfaceCanRestartChange"
            @start-reconnect="startPanelReconnectPolling"
          />
        </v-window-item>

        <v-window-item value="t2">
          <SettingsSubscriptionManageVue
            v-if="hasVerifiedSettings"
            v-model:settings="settings"
            :loading="loading"
          />
        </v-window-item>

		<v-window-item value="t3">
          <SubJsonExtVue
			v-if="tab === 't3' && subJsonDraftLoadState === 'ready'"
			:key="`json-${subscriptionDraftGeneration}`"
            ref="subJsonExtRef"
			:settings="subJsonDraftSettings"
            :canonical-default="settingsDefaults.jsonExt"
            :initial-dirty="subJsonDraftDirty"
            :initial-reset="subJsonResetPending"
            :initial-dirty-baseline="subJsonDraftBaseline"
            :rule-set-sources="ruleSetSources.json"
			@dirty-change="onSubJsonDirtyChange"
          />
          <v-alert v-else-if="tab === 't3'" :type="subJsonDraftLoadState === 'error' ? 'error' : 'info'" variant="tonal" class="my-3">
            <div v-if="subJsonDraftLoadState === 'loading'" class="d-flex align-center" style="gap: 8px;">
              <v-progress-circular indeterminate size="18" width="2" />
              <span>{{ $t('setting.subscriptionExtensionLoading') }}</span>
            </div>
            <template v-else>
              <div>{{ subJsonDraftLoadError || $t('setting.subscriptionExtensionLoadFailed') }}</div>
              <v-btn v-if="subJsonDraftLoadState === 'error'" class="mt-2" size="small" variant="outlined" @click="retrySubscriptionDraft('json')">
                {{ $t('setting.subscriptionExtensionRetry') }}
              </v-btn>
            </template>
          </v-alert>
        </v-window-item>

		<v-window-item value="t4">
          <SubClashExtVue
			v-if="tab === 't4' && subClashDraftLoadState === 'ready'"
			:key="`clash-${subscriptionDraftGeneration}`"
            ref="subClashExtRef"
			:settings="subClashDraftSettings"
            :canonical-default="settingsDefaults.clashExt"
            :initial-dirty="subClashDraftDirty"
            :initial-reset="subClashResetPending"
            :initial-dirty-baseline="subClashDraftBaseline"
            :rule-set-sources="ruleSetSources.clash"
			@dirty-change="onSubClashDirtyChange"
          />
          <v-alert v-else-if="tab === 't4'" :type="subClashDraftLoadState === 'error' ? 'error' : 'info'" variant="tonal" class="my-3">
            <div v-if="subClashDraftLoadState === 'loading'" class="d-flex align-center" style="gap: 8px;">
              <v-progress-circular indeterminate size="18" width="2" />
              <span>{{ $t('setting.subscriptionExtensionLoading') }}</span>
            </div>
            <template v-else>
              <div>{{ subClashDraftLoadError || $t('setting.subscriptionExtensionLoadFailed') }}</div>
              <v-btn v-if="subClashDraftLoadState === 'error'" class="mt-2" size="small" variant="outlined" @click="retrySubscriptionDraft('clash')">
                {{ $t('setting.subscriptionExtensionRetry') }}
              </v-btn>
            </template>
          </v-alert>
        </v-window-item>

        <v-window-item value="t5">
          <SettingsLanguageManageVue />
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
          <SettingsOptimizationManageVue v-if="tab === 't9'" :active="true" />
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
          <SettingsDdnsManageVue :active="tab === 't13'" />
        </v-window-item>

        <v-window-item value="t14">
          <SettingsDnsManageVue :active="tab === 't14'" />
        </v-window-item>
      </v-window>

      <v-overlay :model-value="panelRestartOverlay" class="align-center justify-center" persistent>
        <v-card class="panel-restart-overlay-card" rounded="lg">
          <v-card-text class="text-center py-8">
            <v-progress-circular indeterminate size="52" width="5" color="primary" class="mb-4" />
            <div class="text-subtitle-1 font-weight-medium">{{ $t('setting.panelRestartingTitle') }}</div>
            <div class="text-caption text-medium-emphasis mt-2">{{ $t('setting.panelRestartingDesc') }}</div>
          </v-card-text>
        </v-card>
      </v-overlay>
    </v-card-text>
  </v-card>
</template>

<script lang="ts" setup>
import { useLocale } from 'vuetify'
import { i18n, languages } from '@/locales'
import { Ref, computed, defineAsyncComponent, inject, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import HttpUtils, { type Msg } from '@/plugins/httputil'
import { panelBaseURL } from '@/plugins/api'
import { reloadToLogin, requestLoginNavigation } from '@/plugins/sessionNavigation'
import { FindDiff } from '@/plugins/utils'
import { formatPanelDateTime, refreshPanelTimeContext } from '@/plugins/panelTime'
import { confirm } from '@/plugins/confirm'
import { push } from 'notivue'

const SettingsTrafficManageVue = defineAsyncComponent(() => import('@/components/SettingsTrafficManage.vue'))
const SettingsFirewallManageVue = defineAsyncComponent(() => import('@/components/SettingsFirewallManage.vue'))
const SettingsPortForwardManageVue = defineAsyncComponent(() => import('@/components/SettingsPortForwardManage.vue'))
const SettingsOptimizationManageVue = defineAsyncComponent(() => import('@/components/SettingsOptimizationManage.vue'))
const SettingsAcmeManageVue = defineAsyncComponent(() => import('@/components/SettingsAcmeManage.vue'))
const SettingsReverseProxyManageVue = defineAsyncComponent(() => import('@/components/SettingsReverseProxyManage.vue'))
const SettingsKernelManageVue = defineAsyncComponent(() => import('@/components/SettingsKernelManage.vue'))
const SettingsDdnsManageVue = defineAsyncComponent(() => import('@/components/SettingsDdnsManage.vue'))
const SettingsDnsManageVue = defineAsyncComponent(() => import('@/components/SettingsDnsManage.vue'))
const SettingsJsonSubManageVue = defineAsyncComponent(() => import('@/components/SettingsJsonSubManage.vue'))
const SettingsClashSubManageVue = defineAsyncComponent(() => import('@/components/SettingsClashSubManage.vue'))
const SubJsonExtVue = SettingsJsonSubManageVue
const SubClashExtVue = SettingsClashSubManageVue
const SettingsInterfaceManageVue = defineAsyncComponent(() => import('@/components/SettingsInterfaceManage.vue'))
const SettingsSubscriptionManageVue = defineAsyncComponent(() => import('@/components/SettingsSubscriptionManage.vue'))
const SettingsLanguageManageVue = defineAsyncComponent(() => import('@/components/SettingsLanguageManage.vue'))

const locale = useLocale()
const tab = ref('t1')
const loading: Ref = inject('loading') ?? ref(false)
let settingsPageMounted = false
type SettingsLoadState = 'idle' | 'loading' | 'ready' | 'error'
type SystemTimeZoneLoadState = 'idle' | 'loading' | 'ready' | 'error'
type RuleSetSourceEntry = {
	id: string
	title: string
	domainTemplate?: string
	ipTemplate?: string
	format: string
}
type SettingsSnapshot = {
	revision: number
	values: Record<string, string>
	defaults: { jsonExt: string; clashExt: string }
	ruleSetSources: { json: RuleSetSourceEntry[]; clash: RuleSetSourceEntry[] }
	extensionsIncluded: boolean
}

type SubscriptionSettingsSnapshot = {
	revision: number
	kind: 'json' | 'clash'
	value: string
	default: string
	ruleSetSources: RuleSetSourceEntry[]
}

type SubscriptionInitialResetResult = {
	revision: number
	kind: 'json' | 'clash'
	changedKeys: string[]
	values: Record<string, string>
	warnings?: string[]
}

const settingsLoadState = ref<SettingsLoadState>('idle')
const settingsLoadError = ref('')
let settingsLoadRequestSequence = 0
let systemTimeZoneRequestSequence = 0
const hasVerifiedSettings = computed(() => settingsLoadState.value === 'ready')
const oldSettings = ref<Record<string, any>>({})
const subJsonExtRef = ref<any>(null)
const subClashExtRef = ref<any>(null)
const settingsRevision = ref(0)
const settingsDefaults = ref({ jsonExt: '', clashExt: '' })
const ruleSetSources = ref<{ json: RuleSetSourceEntry[]; clash: RuleSetSourceEntry[] }>({ json: [], clash: [] })
const subJsonDraftSettings = ref<Record<string, any>>({})
const subClashDraftSettings = ref<Record<string, any>>({})
const subJsonDraftValue = ref('')
const subClashDraftValue = ref('')
const subJsonDraftDirty = ref(false)
const subClashDraftDirty = ref(false)
const subJsonDraftBaseline = ref('')
const subClashDraftBaseline = ref('')
const subJsonDraftError = ref('')
const subClashDraftError = ref('')
type SubscriptionDraftLoadState = 'idle' | 'loading' | 'ready' | 'error'
const subJsonDraftLoadState = ref<SubscriptionDraftLoadState>('idle')
const subClashDraftLoadState = ref<SubscriptionDraftLoadState>('idle')
const subJsonDraftLoadError = ref('')
const subClashDraftLoadError = ref('')
const subscriptionDraftLoadRequestSequence: Record<'json' | 'clash', number> = { json: 0, clash: 0 }
const subscriptionDraftAbortControllers: Record<'json' | 'clash', AbortController | null> = { json: null, clash: null }
const subJsonResetPending = ref(false)
const subClashResetPending = ref(false)
const subscriptionDraftGeneration = ref(0)
const resetDialogVisible = ref(false)
const resetTarget = ref<'json' | 'clash' | ''>('')

type PanelReconnectState = {
  targetLoginURL: string
  trackUpdateStatus: boolean
  disconnectObserved: boolean
}

type TimeZoneOption = {
  title: string
  value: string
  props?: {
    disabled?: boolean
  }
}

type SystemTimeZoneStatus = {
  timeLocation?: string
  displayable?: boolean
  canModify?: boolean
  reason?: string
}

type SessionAgeUnit = 'm' | 'h' | 'd'

const panelRestartOverlay = ref(false)
let panelReconnectGeneration = 0
let panelReconnectState: PanelReconnectState | null = null
const panelReconnectTimerId = ref<number | null>(null)

const clearPanelReconnectTimer = () => {
  panelReconnectGeneration += 1
  if (panelReconnectTimerId.value !== null) {
    window.clearTimeout(panelReconnectTimerId.value)
    panelReconnectTimerId.value = null
  }
}

const interfaceBusy = ref(false)
const childPanelCanRestart = ref(false)
const childPanelRestartHint = ref('')
const onInterfaceBusyChange = (busy: boolean) => {
  interfaceBusy.value = busy
}
const onInterfaceCanRestartChange = (canRestart: boolean, hint: string) => {
  childPanelCanRestart.value = canRestart
  childPanelRestartHint.value = hint
}
const panelCanRestart = computed(() => childPanelCanRestart.value)
const panelRestartHint = computed(() => childPanelRestartHint.value || i18n.global.t('setting.restartStatusLoading'))
const panelLifecycleBusy = computed(() => interfaceBusy.value)

const settings = ref<Record<string, string>>({
  webListen: '',
  webDomain: '',
  webPort: '8888',
  webPath: '/app/',
  webURI: '',
  panelAssignedCertificateRecordID: '0',
  panelAssignedCertificateRecordIDs: '[]',
  sessionMaxAge: '0',
  sessionMaxAgeUnit: 'd',
  trafficAge: '30',
  timeLocation: 'UTC',
  subListen: '',
  subPort: '22780',
  subPath: '',
  subDomain: '',
  subAssignedCertificateRecordID: '0',
  subAssignedCertificateRecordIDs: '[]',
  subUpdates: '12',
  subEncode: 'true',
  subShowInfo: 'true',
  subURI: '',
  serverTlsStoreEnabled: 'true',
  serverTlsStore: 'chrome',
  clientTlsStoreEnabled: 'true',
  clientTlsStore: 'chrome',
  subJsonExt: '',
  subClashExt: '',
})
const systemTimeLocation = ref('')
const oldSystemTimeLocation = ref('')
const hiddenPanelTimeLocation = ref('')
const systemTimeZoneStatus = ref<SystemTimeZoneStatus>({})
const systemTimeZoneLoadState = ref<SystemTimeZoneLoadState>('idle')
const systemTimeZoneLoadError = ref('')

const DEFAULT_WEB_PORT = '8888'
const DEFAULT_SUB_PORT = '22780'
const DEFAULT_TIME_LOCATION = 'UTC'
const SESSION_MAX_AGE_MAX_MINUTES = 72 * 60
const SESSION_MAX_AGE_DEFAULT_UNIT: SessionAgeUnit = 'd'
const SETTINGS_SAVE_KEYS = [
  'webListen',
  'webDomain',
  'webPort',
  'webPath',
  'webURI',
  'sessionMaxAge',
  'sessionMaxAgeUnit',
  'trafficAge',
  'timeLocation',
  'subListen',
  'subPort',
  'subPath',
  'subDomain',
  'subUpdates',
  'subEncode',
  'subShowInfo',
  'subURI',
  'serverTlsStoreEnabled',
  'serverTlsStore',
  'clientTlsStoreEnabled',
  'clientTlsStore',
  'subJsonExt',
  'subClashExt',
] as const

const normalizeSessionAgeUnit = (value: unknown): SessionAgeUnit | '' => {
  const normalized = String(value ?? '').trim().toLowerCase()
  if (normalized === 'm' || normalized === 'h' || normalized === 'd') {
    return normalized
  }
  return ''
}

const sessionAgeUnitFactor = (unit: SessionAgeUnit) => {
  switch (unit) {
    case 'd':
      return 24 * 60
    case 'h':
      return 60
    default:
      return 1
  }
}

const deriveSessionAgeDisplay = (minutesValue: unknown, preferredUnit: unknown): { value: string; unit: SessionAgeUnit } => {
  const rawValue = String(minutesValue ?? '').trim()
  if (!/^\d+$/.test(rawValue)) {
    return { value: '3', unit: SESSION_MAX_AGE_DEFAULT_UNIT }
  }
  const parsedValue = Number.parseInt(rawValue, 10)
  if (!Number.isSafeInteger(parsedValue) || parsedValue <= 0) {
    return { value: '3', unit: SESSION_MAX_AGE_DEFAULT_UNIT }
  }
  const effectiveMinutes = Math.min(parsedValue, SESSION_MAX_AGE_MAX_MINUTES)
  const normalizedPreferred = normalizeSessionAgeUnit(preferredUnit)
  if (normalizedPreferred) {
    const factor = sessionAgeUnitFactor(normalizedPreferred)
    const displayValue = effectiveMinutes / factor
    if (Number.isInteger(displayValue) && displayValue >= 1) {
      return { value: String(displayValue), unit: normalizedPreferred }
    }
  }
  if (effectiveMinutes % (24 * 60) === 0) {
    return { value: String(effectiveMinutes / (24 * 60)), unit: 'd' }
  }
  if (effectiveMinutes % 60 === 0) {
    return { value: String(effectiveMinutes / 60), unit: 'h' }
  }
  return { value: String(effectiveMinutes), unit: 'm' }
}

const buildSessionAgeSaveFields = (displayValue: unknown, displayUnit: unknown) => {
  const rawValue = String(displayValue ?? '').trim()
  const normalizedUnit = normalizeSessionAgeUnit(displayUnit) || SESSION_MAX_AGE_DEFAULT_UNIT
  if (/^\d+$/.test(rawValue)) {
    const parsedValue = Number.parseInt(rawValue, 10)
    if (Number.isSafeInteger(parsedValue) && parsedValue >= 0) {
      if (parsedValue === 0) {
        return {
          sessionMaxAge: String(SESSION_MAX_AGE_MAX_MINUTES),
          sessionMaxAgeUnit: SESSION_MAX_AGE_DEFAULT_UNIT,
        }
      }
      return {
        sessionMaxAge: String(parsedValue * sessionAgeUnitFactor(normalizedUnit)),
        sessionMaxAgeUnit: normalizedUnit,
      }
    }
  }
  return {
    sessionMaxAge: rawValue,
    sessionMaxAgeUnit: normalizedUnit,
  }
}

const sessionAgeUnitItems = computed(() => {
  void locale.current.value
  return [
    { title: i18n.global.t('date.m'), value: 'm' },
    { title: i18n.global.t('date.h'), value: 'h' },
    { title: i18n.global.t('date.d'), value: 'd' },
  ]
})

const timeZoneHeaderKeys: Record<string, string> = {
  '推荐国家 / 地区': 'setting.timeZoneRecommended',
  '亚洲': 'setting.timeZoneAsia',
  '欧洲': 'setting.timeZoneEurope',
  '非洲': 'setting.timeZoneAfrica',
  '美洲': 'setting.timeZoneAmericas',
  '大洋洲': 'setting.timeZoneOceania',
}

const createTimeZoneHeader = (title: string): TimeZoneOption => ({
  title: i18n.global.t(timeZoneHeaderKeys[title] || title),
  value: `__header__${title}`,
  props: {
    disabled: true,
  },
})

const createTimeZoneOption = (value: string, _label?: string): TimeZoneOption => ({
  title: value,
  value,
})

const timeZoneOptions = computed<TimeZoneOption[]>(() => {
  // Make the grouped labels reactive to language changes. IANA identifiers are
  // intentionally kept stable so operators can compare them with host logs.
  void locale.current.value
  return [
  createTimeZoneHeader('推荐国家 / 地区'),
  createTimeZoneOption('UTC', '国际标准时间'),
  createTimeZoneOption('Asia/Shanghai', '中国'),
  createTimeZoneOption('Asia/Hong_Kong', '中国香港'),
  createTimeZoneOption('Asia/Taipei', '中国台湾'),
  createTimeZoneOption('Asia/Tokyo', '日本'),
  createTimeZoneOption('Asia/Seoul', '韩国'),
  createTimeZoneOption('Asia/Singapore', '新加坡'),
  createTimeZoneOption('Asia/Bangkok', '泰国'),
  createTimeZoneOption('Asia/Ho_Chi_Minh', '越南'),
  createTimeZoneOption('Asia/Kuala_Lumpur', '马来西亚'),
  createTimeZoneOption('Asia/Jakarta', '印度尼西亚'),
  createTimeZoneOption('Asia/Manila', '菲律宾'),
  createTimeZoneOption('Asia/Kolkata', '印度'),
  createTimeZoneOption('Asia/Karachi', '巴基斯坦'),
  createTimeZoneOption('Asia/Dhaka', '孟加拉国'),
  createTimeZoneOption('Asia/Dubai', '阿联酋'),
  createTimeZoneOption('Asia/Riyadh', '沙特阿拉伯'),
  createTimeZoneOption('Asia/Tehran', '伊朗'),
  createTimeZoneOption('Asia/Jerusalem', '以色列'),
  createTimeZoneOption('Europe/London', '英国'),
  createTimeZoneOption('Europe/Paris', '法国'),
  createTimeZoneOption('Europe/Berlin', '德国'),
  createTimeZoneOption('Europe/Moscow', '俄罗斯'),
  createTimeZoneOption('Europe/Istanbul', '土耳其'),
  createTimeZoneOption('Africa/Cairo', '埃及'),
  createTimeZoneOption('Africa/Johannesburg', '南非'),
  createTimeZoneOption('America/New_York', '美国东部'),
  createTimeZoneOption('America/Chicago', '美国中部'),
  createTimeZoneOption('America/Denver', '美国山地'),
  createTimeZoneOption('America/Los_Angeles', '美国西部'),
  createTimeZoneOption('America/Toronto', '加拿大东部'),
  createTimeZoneOption('America/Vancouver', '加拿大西部'),
  createTimeZoneOption('America/Mexico_City', '墨西哥'),
  createTimeZoneOption('America/Sao_Paulo', '巴西'),
  createTimeZoneOption('America/Argentina/Buenos_Aires', '阿根廷'),
  createTimeZoneOption('Australia/Sydney', '澳大利亚悉尼'),
  createTimeZoneOption('Australia/Perth', '澳大利亚珀斯'),
  createTimeZoneOption('Pacific/Auckland', '新西兰'),
  createTimeZoneHeader('亚洲'),
  createTimeZoneOption('Asia/Kathmandu', '尼泊尔'),
  createTimeZoneOption('Asia/Almaty', '哈萨克斯坦'),
  createTimeZoneOption('Asia/Tashkent', '乌兹别克斯坦'),
  createTimeZoneHeader('欧洲'),
  createTimeZoneOption('Europe/Dublin', '爱尔兰'),
  createTimeZoneOption('Europe/Lisbon', '葡萄牙'),
  createTimeZoneOption('Europe/Madrid', '西班牙'),
  createTimeZoneOption('Europe/Brussels', '比利时'),
  createTimeZoneOption('Europe/Amsterdam', '荷兰'),
  createTimeZoneOption('Europe/Zurich', '瑞士'),
  createTimeZoneOption('Europe/Rome', '意大利'),
  createTimeZoneOption('Europe/Vienna', '奥地利'),
  createTimeZoneOption('Europe/Prague', '捷克'),
  createTimeZoneOption('Europe/Warsaw', '波兰'),
  createTimeZoneOption('Europe/Stockholm', '瑞典'),
  createTimeZoneOption('Europe/Oslo', '挪威'),
  createTimeZoneOption('Europe/Helsinki', '芬兰'),
  createTimeZoneOption('Europe/Athens', '希腊'),
  createTimeZoneOption('Europe/Bucharest', '罗马尼亚'),
  createTimeZoneOption('Europe/Kyiv', '乌克兰'),
  createTimeZoneHeader('非洲'),
  createTimeZoneOption('Africa/Casablanca', '摩洛哥'),
  createTimeZoneOption('Africa/Lagos', '尼日利亚'),
  createTimeZoneOption('Africa/Nairobi', '肯尼亚'),
  createTimeZoneHeader('美洲'),
  createTimeZoneOption('America/Anchorage', '美国阿拉斯加'),
  createTimeZoneOption('Pacific/Honolulu', '美国夏威夷'),
  createTimeZoneOption('America/Bogota', '哥伦比亚'),
  createTimeZoneOption('America/Lima', '秘鲁'),
  createTimeZoneOption('America/Santiago', '智利'),
  createTimeZoneOption('America/Caracas', '委内瑞拉'),
  createTimeZoneOption('America/Montevideo', '乌拉圭'),
  createTimeZoneHeader('大洋洲'),
  createTimeZoneOption('Australia/Melbourne', '澳大利亚墨尔本'),
  createTimeZoneOption('Australia/Brisbane', '澳大利亚布里斯班'),
  createTimeZoneOption('Pacific/Fiji', '斐济'),
  createTimeZoneOption('Pacific/Guam', '关岛'),
  ]
})

const validTimeZoneValues = computed(() => new Set(timeZoneOptions.value.filter(item => !item.props?.disabled).map(item => item.value)))

const normalizeTimeLocationValue = (value: unknown) => {
  const trimmed = String(value ?? '').trim()
  if (validTimeZoneValues.value.has(trimmed)) return trimmed
  return ''
}

const changeLocale = (l: any) => {
  locale.current.value = l ?? 'en'
  localStorage.setItem('locale', locale.current.value)
}

const delaySettingsRetry = () => new Promise<void>(resolve => window.setTimeout(resolve, 600))

const isSettingsPayload = (value: unknown): value is Record<string, any> => {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

const isVerifiedSettingsPayload = (value: unknown): value is Record<string, any> => {
  if (!isSettingsPayload(value)) return false
  return ['webPort', 'webPath', 'subPort', 'subPath', 'timeLocation']
    .every(key => typeof value[key] === 'string')
}

const isVerifiedSettingsSnapshot = (value: unknown): value is SettingsSnapshot => {
	if (!isSettingsPayload(value)) return false
	const snapshot = value as Record<string, any>
	return Number.isInteger(snapshot.revision)
	  && snapshot.revision >= 1
	  && isVerifiedSettingsPayload(snapshot.values)
	  && typeof snapshot.extensionsIncluded === 'boolean'
	  && isSettingsPayload(snapshot.defaults)
	  && typeof snapshot.defaults.jsonExt === 'string'
	  && typeof snapshot.defaults.clashExt === 'string'
	  && isSettingsPayload(snapshot.ruleSetSources)
	  && Array.isArray(snapshot.ruleSetSources.json)
	  && Array.isArray(snapshot.ruleSetSources.clash)
}

const isSystemTimeZonePayload = (value: unknown): value is SystemTimeZoneStatus => {
  return isSettingsPayload(value)
    && typeof value.canModify === 'boolean'
    && typeof value.displayable === 'boolean'
}

const loadSettingsWithRetry = async (): Promise<Msg> => {
  let lastMsg: Msg = { success: false, msg: '', obj: null }
  for (let attempt = 0; attempt < 2; attempt += 1) {
	const msg = await HttpUtils.get('api/settings-snapshot?includeExtensions=false', {}, { timeout: 15000, silentErrorToast: true })
	if (msg.success && isVerifiedSettingsSnapshot(msg.obj)) {
      return msg
    }
    lastMsg = msg.success
      ? { success: false, msg: i18n.global.t('setting.settingsPayloadInvalid'), obj: null }
      : msg
    if (attempt === 0) {
      await delaySettingsRetry()
    }
  }
  return lastMsg
}

const loadData = async (): Promise<boolean> => {
  const requestSequence = ++settingsLoadRequestSequence
  settingsLoadState.value = 'loading'
  settingsLoadError.value = ''
  loading.value = true
  try {
    const msg = await loadSettingsWithRetry()
    if (requestSequence !== settingsLoadRequestSequence) return false
	if (!msg.success || !isVerifiedSettingsSnapshot(msg.obj)) {
      settingsLoadState.value = 'error'
      settingsLoadError.value = String(msg.msg || i18n.global.t('setting.settingsLoadFallback'))
      return false
    }

	setData(msg.obj)
    settingsLoadState.value = 'ready'
    void loadSystemTimeZone(requestSequence)
    return true
  } finally {
    if (requestSequence === settingsLoadRequestSequence) {
      loading.value = false
    }
  }
}

const retryLoadData = () => {
  void loadData()
}

const setData = (snapshot: SettingsSnapshot) => {
	const data = snapshot.values
  const sessionAgeDisplay = deriveSessionAgeDisplay(data?.sessionMaxAge, data?.sessionMaxAgeUnit)
  const rawPanelTimeLocation = String(data?.timeLocation ?? '').trim()
  const panelTimeLocation = normalizeTimeLocationValue(rawPanelTimeLocation)
  // 有效但不在固定列表中的数据库时区仍由后端使用；界面必须保持为空，
  // 同时保存其它设置时不能意外把它覆盖为 UTC。
  hiddenPanelTimeLocation.value = panelTimeLocation === '' ? rawPanelTimeLocation : ''
  const normalized = {
    ...data,
    sessionMaxAge: sessionAgeDisplay.value,
    sessionMaxAgeUnit: sessionAgeDisplay.unit,
    timeLocation: panelTimeLocation,
  }
	settings.value = {
	  ...normalized,
	  subJsonExt: '',
	  subClashExt: '',
	}
  oldSettings.value = { ...settings.value }
	settingsRevision.value = snapshot.revision
	settingsDefaults.value = { ...snapshot.defaults }
	ruleSetSources.value = {
	  json: [...snapshot.ruleSetSources.json],
	  clash: [...snapshot.ruleSetSources.clash],
	}
	subJsonDraftValue.value = ''
	subClashDraftValue.value = ''
	subJsonDraftSettings.value = {}
	subClashDraftSettings.value = {}
	subJsonDraftDirty.value = false
	subClashDraftDirty.value = false
	subJsonDraftBaseline.value = ''
	subClashDraftBaseline.value = ''
	subJsonDraftError.value = ''
	subClashDraftError.value = ''
	subJsonDraftLoadState.value = 'idle'
	subClashDraftLoadState.value = 'idle'
	subJsonDraftLoadError.value = ''
	subClashDraftLoadError.value = ''
	subscriptionDraftLoadRequestSequence.json += 1
	subscriptionDraftLoadRequestSequence.clash += 1
	subJsonResetPending.value = false
	subClashResetPending.value = false
	subscriptionDraftGeneration.value += 1
}

const isSubscriptionSettingsSnapshot = (value: unknown, kind: 'json' | 'clash'): value is SubscriptionSettingsSnapshot => {
	if (!isSettingsPayload(value)) return false
	const snapshot = value as Record<string, any>
	return Number.isInteger(snapshot.revision)
	  && snapshot.revision >= 1
	  && snapshot.kind === kind
	  && typeof snapshot.value === 'string'
	  && typeof snapshot.default === 'string'
	  && Array.isArray(snapshot.ruleSetSources)
}

const setSubscriptionDraftLoadState = (target: 'json' | 'clash', state: SubscriptionDraftLoadState, error = '') => {
	if (target === 'json') {
		subJsonDraftLoadState.value = state
		subJsonDraftLoadError.value = error
		return
	}
	subClashDraftLoadState.value = state
	subClashDraftLoadError.value = error
}

const isSubscriptionDraftDirty = (target: 'json' | 'clash') => target === 'json'
	? subJsonDraftDirty.value
	: subClashDraftDirty.value

const loadSubscriptionDraft = async (target: 'json' | 'clash', retryAfterRevisionRefresh = true): Promise<boolean> => {
	const currentState = target === 'json' ? subJsonDraftLoadState.value : subClashDraftLoadState.value
	if (currentState === 'loading' || currentState === 'ready') return currentState === 'ready'

	const requestSequence = ++subscriptionDraftLoadRequestSequence[target]
	subscriptionDraftAbortControllers[target]?.abort()
	const abortController = new AbortController()
	subscriptionDraftAbortControllers[target] = abortController
	setSubscriptionDraftLoadState(target, 'loading')
	let msg: Msg
	try {
		msg = await HttpUtils.get(`api/subscription-settings-snapshot?kind=${target}`, {}, {
			timeout: 15000,
			signal: abortController.signal,
			silentErrorToast: true,
		})
	} finally {
		if (subscriptionDraftAbortControllers[target] === abortController) {
			subscriptionDraftAbortControllers[target] = null
		}
	}
	if (requestSequence !== subscriptionDraftLoadRequestSequence[target] || abortController.signal.aborted) return false
	if (!msg.success || !isSubscriptionSettingsSnapshot(msg.obj, target)) {
		setSubscriptionDraftLoadState(target, 'error', String(msg.msg || i18n.global.t('setting.subscriptionExtensionLoadFailed')))
		return false
	}

	const snapshot = msg.obj
	if (snapshot.revision !== settingsRevision.value) {
		if (retryAfterRevisionRefresh && !stateChange.value && !isSubscriptionDraftDirty(target)) {
			const reloaded = await loadData()
			if (reloaded) return loadSubscriptionDraft(target, false)
		}
		setSubscriptionDraftLoadState(target, 'error', i18n.global.t('setting.subscriptionExtensionRevisionChanged'))
		return false
	}

	// Empty extension text is the persisted first-installation state. Do not
	// replace it with the current code template, or a reset cannot reproduce
	// the page and editor state from the first installation.
	const value = snapshot.value
	if (target === 'json') {
		settingsDefaults.value.jsonExt = snapshot.default
		ruleSetSources.value.json = [...snapshot.ruleSetSources]
		subJsonDraftValue.value = snapshot.value
		subJsonDraftSettings.value = { ...settings.value, subJsonExt: value }
		subJsonDraftDirty.value = false
		subJsonDraftBaseline.value = ''
		subJsonDraftError.value = ''
		subJsonResetPending.value = false
	} else {
		settingsDefaults.value.clashExt = snapshot.default
		ruleSetSources.value.clash = [...snapshot.ruleSetSources]
		subClashDraftValue.value = snapshot.value
		subClashDraftSettings.value = { subClashExt: value }
		subClashDraftDirty.value = false
		subClashDraftBaseline.value = ''
		subClashDraftError.value = ''
		subClashResetPending.value = false
	}
	setSubscriptionDraftLoadState(target, 'ready')
	subscriptionDraftGeneration.value += 1
	return true
}

const retrySubscriptionDraft = (target: 'json' | 'clash') => {
	setSubscriptionDraftLoadState(target, 'idle')
	void loadSubscriptionDraft(target)
}

const isSubscriptionInitialResetResult = (value: unknown, kind: 'json' | 'clash'): value is SubscriptionInitialResetResult => {
	if (!isSettingsPayload(value)) return false
	const result = value as Record<string, any>
	if (!Number.isInteger(result.revision) || result.revision < 1 || result.kind !== kind || !Array.isArray(result.changedKeys) || !isSettingsPayload(result.values)) {
		return false
	}
	const requiredKeys = kind === 'json'
		? ['subJsonExt', 'serverTlsStoreEnabled', 'serverTlsStore', 'clientTlsStoreEnabled', 'clientTlsStore']
		: ['subClashExt']
	return requiredKeys.every(key => typeof result.values[key] === 'string')
}

const applySubscriptionInitialReset = (target: 'json' | 'clash', result: SubscriptionInitialResetResult) => {
	settingsRevision.value = result.revision
	if (target === 'json') {
		for (const key of ['serverTlsStoreEnabled', 'serverTlsStore', 'clientTlsStoreEnabled', 'clientTlsStore']) {
			const value = String(result.values[key])
			settings.value[key] = value
			oldSettings.value[key] = value
		}
		const value = String(result.values.subJsonExt)
		subJsonDraftValue.value = value
		subJsonDraftSettings.value = { ...settings.value, subJsonExt: value }
		subJsonDraftDirty.value = false
		subJsonDraftBaseline.value = ''
		subJsonDraftError.value = ''
		subJsonResetPending.value = false
		setSubscriptionDraftLoadState('json', 'ready')
	} else {
		const value = String(result.values.subClashExt)
		subClashDraftValue.value = value
		subClashDraftSettings.value = { subClashExt: value }
		subClashDraftDirty.value = false
		subClashDraftBaseline.value = ''
		subClashDraftError.value = ''
		subClashResetPending.value = false
		setSubscriptionDraftLoadState('clash', 'ready')
	}
	subscriptionDraftGeneration.value += 1
}

const loadSystemTimeZone = async (settingsRequestSequence?: number): Promise<boolean> => {
  const requestSequence = ++systemTimeZoneRequestSequence
  const isCurrentRequest = () => requestSequence === systemTimeZoneRequestSequence
    && (settingsRequestSequence === undefined || settingsRequestSequence === settingsLoadRequestSequence)

  if (isCurrentRequest()) {
    systemTimeZoneLoadState.value = 'loading'
    systemTimeZoneLoadError.value = ''
  }

  let lastMsg: Msg = { success: false, msg: '', obj: null }
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const msg = await HttpUtils.get('api/system-timezone', {}, { timeout: 8000, silentErrorToast: true })
    if (msg.success && isSystemTimeZonePayload(msg.obj)) {
      if (isCurrentRequest()) {
        const status = msg.obj as SystemTimeZoneStatus
        systemTimeZoneStatus.value = status
        const visible = status.canModify === true && status.displayable === true
          ? normalizeTimeLocationValue(status.timeLocation)
          : ''
        systemTimeLocation.value = visible
        oldSystemTimeLocation.value = visible
        systemTimeZoneLoadState.value = 'ready'
      }
      return true
    }
    lastMsg = msg.success
      ? { success: false, msg: i18n.global.t('setting.systemTimePayloadInvalid'), obj: null }
      : msg
    if (attempt === 0) {
      await delaySettingsRetry()
    }
  }

  if (isCurrentRequest()) {
    systemTimeZoneLoadState.value = 'error'
    systemTimeZoneLoadError.value = String(lastMsg.msg || i18n.global.t('setting.requestFailed'))
    systemTimeZoneStatus.value = {}
    systemTimeLocation.value = ''
    oldSystemTimeLocation.value = ''
  }
  return false
}

const retryLoadSystemTimeZone = () => {
  void loadSystemTimeZone(settingsLoadRequestSequence)
}

const isPanelReconnectPollingAllowed = () => (
  settingsPageMounted
  && (typeof document === 'undefined' || document.visibilityState === 'visible')
)

const handlePanelVisibilityChange = () => {
  if (typeof document === 'undefined') return
  if (document.visibilityState !== 'visible') {
    clearPanelReconnectTimer()
    return
  }
  if (panelRestartOverlay.value) {
    startPanelReconnectPolling()
  }
}

onMounted(() => {
  settingsPageMounted = true
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handlePanelVisibilityChange)
  }
  void loadData()
})

const onSystemTimeLocationSelected = () => {
  if (systemTimeZoneStatus.value.canModify === true) return
  push.warning({
    title: i18n.global.t('failed'),
    duration: 5000,
    message: systemTimeZoneStatus.value.reason || i18n.global.t('setting.systemTimePermissionDenied'),
  })
}







const normalizePanelUpdateStatus = (raw: any): any => {
  if (raw == null || typeof raw !== 'object') return null
  return raw
}

const currentPanelLoginURL = () => {
  if (typeof window === 'undefined') return ''
  return new URL(`${panelBaseURL}login`, window.location.origin).href
}

const toPanelLoginURL = (value: string) => {
  const target = new URL(value, window.location.origin)
  target.search = ''
  target.hash = ''
  let path = target.pathname.replace(/\/+$/, '')
  if (path.endsWith('/settings')) {
    path = path.slice(0, -'/settings'.length)
  }
  target.pathname = `${path || ''}/login`
  return target.href
}

const configuredPanelLoginURL = () => {
  let panelURL = String(settings.value.webURI ?? '').trim()
  if (panelURL === '') {
    panelURL = buildURL(
      settings.value.webDomain,
      settings.value.webPort.toString(),
      isWebTLSEnabled(settings.value),
      settings.value.webPath,
    )
  }
  try {
    return toPanelLoginURL(panelURL)
  } catch {
    return ''
  }
}

const probePanelLoginURL = async (loginURL: string) => {
  if (typeof window === 'undefined' || typeof fetch !== 'function') return true

  try {
    const target = new URL(loginURL)
    // HTTPS 页面不能探测新的 HTTP 面板地址；确认旧入口断开后，
    // 顶层跳转仍然可以完成该协议切换。
    if (window.location.protocol === 'https:' && target.protocol === 'http:') return true
  } catch {
    return false
  }

  const controller = new AbortController()
  const timeoutId = window.setTimeout(() => controller.abort(), 5000)
  try {
    await fetch(loginURL, {
      method: 'GET',
      mode: 'no-cors',
      cache: 'no-store',
      credentials: 'omit',
      signal: controller.signal,
    })
    return true
  } catch {
    return false
  } finally {
    window.clearTimeout(timeoutId)
  }
}

const redirectToRestartedPanelLogin = async (
  reconnectState: PanelReconnectState,
  pollingGeneration: number,
  reconnectGeneration: number,
) => {
  const isCurrentRun = () => (
    reconnectGeneration === panelReconnectGeneration
    && panelReconnectState === reconnectState
    && isPanelReconnectPollingAllowed()
  )
  if (!isCurrentRun()) return true

  const targetLoginURL = reconnectState.targetLoginURL
  if (targetLoginURL === '' || targetLoginURL === currentPanelLoginURL()) {
    panelReconnectState = null
    clearPanelReconnectTimer()
    reloadToLogin()
    return true
  }

  const targetReady = await probePanelLoginURL(targetLoginURL)
  if (!isCurrentRun()) return true
  if (!targetReady) return false

  panelReconnectState = null
  clearPanelReconnectTimer()
  window.location.replace(targetLoginURL)
  return true
}

const startPanelReconnectPolling = (options?: {
  targetLoginURL?: string
  trackUpdateStatus?: boolean
}) => {
  clearPanelReconnectTimer()
  if (options != null) {
    panelReconnectState = {
      targetLoginURL: String(options.targetLoginURL ?? '').trim(),
      trackUpdateStatus: options.trackUpdateStatus === true,
      disconnectObserved: false,
    }
  }
  const reconnectState = panelReconnectState
  if (reconnectState == null || !isPanelReconnectPollingAllowed()) return

  const reconnectGeneration = panelReconnectGeneration
  panelRestartOverlay.value = true
  const isCurrentPollingRun = () => (
    reconnectGeneration === panelReconnectGeneration
    && panelReconnectState === reconnectState
    && isPanelReconnectPollingAllowed()
  )

  const poll = async () => {
    if (!isCurrentPollingRun()) return
    try {
      const sessionMsg = await HttpUtils.get('api/session', {}, {
        timeout: 5000,
        silentAuthCheck: true,
        silentErrorToast: true,
      })
      if (!isCurrentPollingRun()) return

      if (!sessionMsg.success && sessionMsg.failureKind === 'transport') {
        // 显式重启流程必须先确认旧面板入口已断开，不能把重启前仍
        // 成功的会话探测误判为新面板已经恢复。
        reconnectState.disconnectObserved = true
        if (reconnectState.targetLoginURL !== '' && reconnectState.targetLoginURL !== currentPanelLoginURL()) {
          if (await redirectToRestartedPanelLogin(reconnectState, 0, reconnectGeneration)) return
        }
      }

      if (!sessionMsg.success && sessionMsg.failureKind === 'api') {
        if (await redirectToRestartedPanelLogin(reconnectState, 0, reconnectGeneration)) return
      }

      if (sessionMsg.success && reconnectState.disconnectObserved) {
        if (await redirectToRestartedPanelLogin(reconnectState, 0, reconnectGeneration)) return
      }

      if (sessionMsg.success && reconnectState.trackUpdateStatus && !reconnectState.disconnectObserved) {
        const statusMsg = await HttpUtils.get('api/panel-update-status', {}, {
          timeout: 5000,
          silentAuthCheck: true,
          silentErrorToast: true,
        })
        if (!isCurrentPollingRun()) return
        if (statusMsg.success) {
          const nextStatus = normalizePanelUpdateStatus(statusMsg.obj)
          const updateError = String(nextStatus?.lastUpdateError ?? '').trim()
          if (updateError !== '') {
            panelReconnectState = null
            clearPanelReconnectTimer()
            panelRestartOverlay.value = false
            push.error({
              title: i18n.global.t('failed'),
              duration: 8000,
              message: `${i18n.global.t('setting.panelUpdateFailed')}：${updateError}`,
            })
            return
          }
        }
      }
    } catch {
      // 等待面板恢复连接
    }

    if (isCurrentPollingRun()) {
      panelReconnectTimerId.value = window.setTimeout(poll, 4000)
    }
  }

  panelReconnectTimerId.value = window.setTimeout(poll, 6000)
}



type SubscriptionSerializeResult = {
	ok: boolean
	dirty: boolean
	reset?: boolean
	value: string
	error?: string
}

const onSubJsonDirtyChange = (dirty: boolean) => {
	subJsonDraftDirty.value = dirty
}

const onSubClashDirtyChange = (dirty: boolean) => {
	subClashDraftDirty.value = dirty
}

const persistSubscriptionDraft = (target: 'json' | 'clash', requireValid = false): boolean => {
	const component = target === 'json' ? subJsonExtRef.value : subClashExtRef.value
	const alreadyDirty = target === 'json' ? subJsonDraftDirty.value : subClashDraftDirty.value
	if (!component) return !requireValid || !alreadyDirty
	const rememberDraftBaseline = () => {
		const snapshot = component.getDirtyTrackingBaseline?.()
		if (typeof snapshot !== 'string' || snapshot === '') return
		if (target === 'json') subJsonDraftBaseline.value = snapshot
		else subClashDraftBaseline.value = snapshot
	}
	const componentDirty = component.isDirty?.() === true
	if (!alreadyDirty && !componentDirty) {
		rememberDraftBaseline()
		return true
	}

	const result = component.validateAndSerialize?.() as SubscriptionSerializeResult | undefined
	rememberDraftBaseline()
	if (!result) return true
	if (target === 'json') {
	  subJsonDraftDirty.value = result.dirty === true
	  subJsonDraftValue.value = result.value
	  subJsonDraftError.value = result.ok ? '' : String(result.error || i18n.global.t('subscriptionEditor.validationFailed'))
	  subJsonResetPending.value = result.reset === true
	  if (!result.reset) subJsonDraftSettings.value.subJsonExt = result.value
	} else {
	  subClashDraftDirty.value = result.dirty === true
	  subClashDraftValue.value = result.value
	  subClashDraftError.value = result.ok ? '' : String(result.error || i18n.global.t('subscriptionEditor.validationFailed'))
	  subClashResetPending.value = result.reset === true
	  if (!result.reset) subClashDraftSettings.value.subClashExt = result.value
	}
	if (!result.ok && requireValid) {
	  push.error({
		title: i18n.global.t('failed'),
		duration: 5000,
		message: result.error || i18n.global.t('subscriptionEditor.validationFailed'),
	  })
	  return false
	}
	return true
}

watch(tab, (value, previous) => {
	if (previous === 't3') {
		persistSubscriptionDraft('json')
		if (subJsonDraftLoadState.value === 'loading') setSubscriptionDraftLoadState('json', 'idle')
		subscriptionDraftLoadRequestSequence.json += 1
		subscriptionDraftAbortControllers.json?.abort()
	}
	if (previous === 't4') {
		persistSubscriptionDraft('clash')
		if (subClashDraftLoadState.value === 'loading') setSubscriptionDraftLoadState('clash', 'idle')
		subscriptionDraftLoadRequestSequence.clash += 1
		subscriptionDraftAbortControllers.clash?.abort()
	}
	if (previous === 't1') {
		clearPanelReconnectTimer()
	}
	if (value === 't3') void loadSubscriptionDraft('json')
	if (value === 't4') void loadSubscriptionDraft('clash')
	if (value === 't1') {
		if (panelRestartOverlay.value) {
			startPanelReconnectPolling()
		}
	}
})

watch(() => settings.value.timeLocation, value => {
  if (validTimeZoneValues.value.has(String(value ?? '').trim())) {
    hiddenPanelTimeLocation.value = ''
  }
})

onBeforeUnmount(() => {
  settingsPageMounted = false
  const settingsRequestWasLoading = settingsLoadState.value === 'loading'
  settingsLoadRequestSequence += 1
  systemTimeZoneRequestSequence += 1
	subscriptionDraftLoadRequestSequence.json += 1
	subscriptionDraftLoadRequestSequence.clash += 1
	if (subJsonDraftLoadState.value === 'loading') setSubscriptionDraftLoadState('json', 'idle')
	if (subClashDraftLoadState.value === 'loading') setSubscriptionDraftLoadState('clash', 'idle')
	subscriptionDraftAbortControllers.json?.abort()
	subscriptionDraftAbortControllers.clash?.abort()
	if (tab.value === 't3') persistSubscriptionDraft('json')
	if (tab.value === 't4') persistSubscriptionDraft('clash')
  clearPanelReconnectTimer()
  panelReconnectState = null
  panelRestartOverlay.value = false
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handlePanelVisibilityChange)
  }
  if (settingsRequestWasLoading || loading.value) loading.value = false
})

const save = async () => {
  if (!hasVerifiedSettings.value || settingsLoadState.value === 'loading') return
  applyPortDefaultsBeforeSave()
  const previousSettings = { ...settings.value }
	if (tab.value === 't3' && !persistSubscriptionDraft('json', true)) return
	if (tab.value === 't4' && !persistSubscriptionDraft('clash', true)) return
	const inactiveDraftError = subJsonDraftDirty.value && subJsonDraftError.value
	  ? subJsonDraftError.value
	  : subClashDraftDirty.value && subClashDraftError.value
		? subClashDraftError.value
		: ''
	if (inactiveDraftError) {
	  push.error({
		title: i18n.global.t('failed'),
		duration: 5000,
		message: inactiveDraftError,
	  })
	  return
	}

	if (subJsonDraftDirty.value) {
	  for (const key of ['serverTlsStoreEnabled', 'serverTlsStore', 'clientTlsStoreEnabled', 'clientTlsStore']) {
		settings.value[key] = String(subJsonDraftSettings.value[key] ?? settings.value[key] ?? '')
	  }
	}
	const normalizedSettings = buildSettingsSavePayload(settings.value)
	const previousNormalizedSettings = buildSettingsSavePayload(oldSettings.value)
	const changes: Record<string, string> = {}
	for (const key of SETTINGS_SAVE_KEYS) {
	  if (key === 'subJsonExt' || key === 'subClashExt') continue
	  const nextValue = String(normalizedSettings[key] ?? '')
	  const previousValue = String(previousNormalizedSettings[key] ?? '')
	  if (nextValue !== previousValue) changes[key] = nextValue
	}
	if (subJsonDraftDirty.value) changes.subJsonExt = subJsonDraftValue.value
	if (subClashDraftDirty.value) changes.subClashExt = subClashDraftValue.value
	if (Object.keys(changes).length === 0) return
	const clearingTrafficHistory = String(normalizedSettings.trafficAge ?? '').trim() === '0'
	  && String(previousNormalizedSettings.trafficAge ?? '').trim() !== '0'
	if (clearingTrafficHistory) {
	  const confirmed = await confirm({
		severity: 'danger',
		title: i18n.global.t('setting.trafficHistoryClearConfirmTitle'),
		message: i18n.global.t('setting.trafficHistoryClearConfirm'),
		confirmText: i18n.global.t('subscriptionEditor.resetConfirm'),
	  })
	  if (!confirmed) return
	}
	const rotatingSubscriptionPath = Object.prototype.hasOwnProperty.call(changes, 'subPath')
	if (rotatingSubscriptionPath) {
	  const confirmed = await confirm({
		severity: 'warning',
		title: i18n.global.t('setting.subPathChangeConfirmTitle'),
		message: i18n.global.t('setting.subPathChangeConfirm'),
		confirmText: i18n.global.t('subscriptionEditor.resetConfirm'),
	  })
	  if (!confirmed) return
	}

  loading.value = true
  try {
	const msg = await HttpUtils.post('api/settings-patch', {
	  expectedRevision: settingsRevision.value,
	  changes,
	  confirmTrafficHistoryClear: clearingTrafficHistory || undefined,
	}, {
	  headers: { 'Content-Type': 'application/json' },
	  timeout: 30000,
	  silentErrorToast: true,
	})
    if (msg.success) {
	  const warnings = Array.isArray(msg.obj?.warnings) ? msg.obj.warnings.map(String) : []
      push.success({
        title: i18n.global.t('success'),
        duration: 5000,
        message: i18n.global.t('actions.set') + ' ' + i18n.global.t('pages.settings'),
      })
	  if (warnings.length > 0) {
		push.warning({
		  title: i18n.global.t('subscriptionEditor.settingsSavedWithWarnings'),
		  duration: 8000,
		  message: warnings.join('; '),
		})
	  }
	  if (msg.obj?.maintenanceQueued === true) {
		push.info({
		  title: i18n.global.t('setting.maintenanceQueuedTitle'),
		  duration: 6000,
		  message: i18n.global.t('setting.maintenanceQueuedMessage'),
		})
	  }
	  await loadData()
	  if (tab.value === 't3') await loadSubscriptionDraft('json')
	  if (tab.value === 't4') await loadSubscriptionDraft('clash')
      await Promise.all([
        refreshSessionTimeoutLease(),
        refreshPanelTimeContext(),
        loadSystemTimeZone(),
      ])
	  await maybeRedirectToHttps(settings.value, previousSettings)
	} else if (msg.obj?.code === 'revision_conflict') {
	  await loadData()
	  await loadSystemTimeZone()
	  push.warning({
		title: i18n.global.t('subscriptionEditor.revisionConflictTitle'),
		duration: 7000,
		message: i18n.global.t('subscriptionEditor.revisionConflictMessage'),
	  })
	} else {
	  push.error({
		title: i18n.global.t('failed'),
		duration: 6000,
		message: msg.msg || i18n.global.t('subscriptionEditor.settingsSaveFailed'),
	  })
    }
  } finally {
    loading.value = false
  }
}

const currentResetTarget = computed<'json' | 'clash' | ''>(() => {
  if (tab.value === 't3' && subJsonDraftLoadState.value === 'ready') return 'json'
  if (tab.value === 't4' && subClashDraftLoadState.value === 'ready') return 'clash'
  return ''
})

const showSubPageResetButton = computed(() => currentResetTarget.value !== '')

const resetButtonText = computed(() => {
	if (currentResetTarget.value === 'json') return i18n.global.t('subscriptionEditor.resetJson')
	if (currentResetTarget.value === 'clash') return i18n.global.t('subscriptionEditor.resetClash')
  return ''
})

const resetDialogMessage = computed(() => {
  if (resetTarget.value === 'json') {
	return i18n.global.t('subscriptionEditor.resetJsonMessage')
  }
  if (resetTarget.value === 'clash') {
	return i18n.global.t('subscriptionEditor.resetClashMessage')
  }
  return ''
})

const openResetDialog = () => {
  if (!hasVerifiedSettings.value || !currentResetTarget.value) return
  resetTarget.value = currentResetTarget.value
  resetDialogVisible.value = true
}

const closeResetDialog = () => {
  resetDialogVisible.value = false
  resetTarget.value = ''
}

const confirmResetSubPage = async () => {
	const target = resetTarget.value
	if (!hasVerifiedSettings.value || !target || loading.value) return
	loading.value = true
	try {
		const msg = await HttpUtils.post('api/subscription-initial-reset', {
			kind: target,
			expectedRevision: settingsRevision.value,
		}, {
			headers: { 'Content-Type': 'application/json' },
			timeout: 30000,
			silentErrorToast: true,
		})
		if (msg.success && isSubscriptionInitialResetResult(msg.obj, target)) {
			applySubscriptionInitialReset(target, msg.obj)
			const warnings = Array.isArray(msg.obj.warnings) ? msg.obj.warnings.map(String) : []
			push.success({
				title: i18n.global.t('success'),
				duration: 5000,
				message: target === 'json'
					? i18n.global.t('subscriptionEditor.resetJsonSuccess')
					: i18n.global.t('subscriptionEditor.resetClashSuccess'),
			})
			if (warnings.length > 0) {
				push.warning({
					title: i18n.global.t('subscriptionEditor.settingsSavedWithWarnings'),
					duration: 8000,
					message: warnings.join('; '),
				})
			}
			closeResetDialog()
			return
		}
		if (msg.obj?.code === 'revision_conflict') {
			closeResetDialog()
			await loadData()
			await loadSystemTimeZone()
			push.warning({
				title: i18n.global.t('subscriptionEditor.revisionConflictTitle'),
				duration: 7000,
				message: i18n.global.t('subscriptionEditor.revisionConflictMessage'),
			})
			return
		}
		push.error({
			title: i18n.global.t('failed'),
			duration: 6000,
			message: msg.msg || i18n.global.t('subscriptionEditor.settingsSaveFailed'),
		})
	} finally {
		loading.value = false
	}
}

const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))

const restartApp = async () => {
  if (!hasVerifiedSettings.value || settingsLoadState.value === 'loading' || !panelCanRestart.value) {
	if (!panelCanRestart.value && panelRestartHint.value) {
	  push.warning({
		title: i18n.global.t('failed'),
		duration: 5000,
		message: panelRestartHint.value,
	  })
	}
	return
  }
  const targetLoginURL = configuredPanelLoginURL()
  loading.value = true
  try {
    const msg = await HttpUtils.post('api/restartApp', {})
    if (msg.success) {
      startPanelReconnectPolling({ targetLoginURL })
      return
    }
  } finally {
    loading.value = false
  }
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

const refreshSessionTimeoutLease = async () => {
  const msg = await HttpUtils.get('api/session', {}, {
    silentAuthCheck: true,
    silentErrorToast: true,
    timeout: 10000,
  })
  if (!msg.success && msg.failureKind === 'api') {
    await requestLoginNavigation()
  }
}

const buildURL = (host: string, port: string, isTLS: boolean, path: string) => {
  if (!host || host.length === 0) host = window.location.hostname
  if (!port || port.length === 0) port = window.location.port
	if (host.includes(':') && !host.startsWith('[')) {
		host = `[${host}]`
	}

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
  if (!/^\d+$/.test(strValue)) return strValue
  const parsed = Number(strValue)
  if (!Number.isSafeInteger(parsed) || parsed < 1 || parsed > 65535) return strValue
  return parsed.toString()
}

const applyPortDefaultsBeforeSave = () => {
  settings.value.webPort = normalizePort(settings.value.webPort, DEFAULT_WEB_PORT)
  const fallbackSubPort = normalizePort(oldSettings.value?.subPort, DEFAULT_SUB_PORT)
  settings.value.subPort = normalizePort(settings.value.subPort, fallbackSubPort)
}

const buildSettingsSavePayload = (value: Record<string, any>) => {
  const payload = Object.fromEntries(
    SETTINGS_SAVE_KEYS.map(key => [key, value[key]]),
  ) as Record<string, any>
  const sessionAgeFields = buildSessionAgeSaveFields(payload.sessionMaxAge, payload.sessionMaxAgeUnit)
  payload.sessionMaxAge = sessionAgeFields.sessionMaxAge
  payload.sessionMaxAgeUnit = sessionAgeFields.sessionMaxAgeUnit
  const visiblePanelTimeLocation = normalizeTimeLocationValue(payload.timeLocation)
  payload.timeLocation = visiblePanelTimeLocation || hiddenPanelTimeLocation.value || DEFAULT_TIME_LOCATION
  return payload
}

const stateChange = computed(() => {
  if (!hasVerifiedSettings.value) return false
  return !FindDiff.deepCompare(settings.value, oldSettings.value)
	|| subJsonDraftDirty.value
	|| subClashDraftDirty.value
})

const showTopActionBar = computed(() => ['t1', 't2', 't3', 't4'].includes(tab.value))
</script>

<style scoped>
.panel-restart-overlay-card {
  width: calc(100vw - 32px);
  max-width: 400px;
}
</style>
