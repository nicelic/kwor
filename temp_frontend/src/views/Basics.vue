<template>
  <template v-if="initializing">
    <v-row align="center" justify="center" style="min-height: 240px;">
      <v-col cols="12" class="text-center">
        <v-progress-circular indeterminate color="primary" />
        <div class="mt-3">{{ $t('loading') }}</div>
      </v-col>
    </v-row>
  </template>
  <template v-else-if="loadFailed">
    <v-row align="center" justify="center" style="min-height: 240px;">
      <v-col cols="12" sm="8" md="6">
        <v-alert type="error" variant="tonal" :title="$t('failed')" class="text-center">
          <v-btn color="primary" class="mt-2" prepend-icon="mdi-refresh" @click="initialize">
            {{ $t('actions.update') }}
          </v-btn>
        </v-alert>
      </v-col>
    </v-row>
  </template>
  <template v-else>
  <v-row style="margin-bottom: 10px;">
    <v-col cols="12" justify="center" align="center">
      <v-btn variant="outlined" color="warning" @click="saveConfig" :loading="loading" :disabled="loading || (!stateChange && !runtimeRefreshFailed)">
        {{ $t('actions.save') }}
      </v-btn>
    </v-col>
  </v-row>
  <v-expansion-panels :disabled="loading">
    <v-expansion-panel title="NTP">
      <v-expansion-panel-text>
        <v-row>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-switch v-model="enableNtp" color="primary" :label="$t('enable')" hide-details :disabled="loading"></v-switch>
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2" v-if="appConfig.ntp?.enabled">
            <v-text-field
               v-model="appConfig.ntp.server"
               hide-details
               :disabled="loading"
              :label="$t('out.addr')"
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2" v-if="appConfig.ntp?.enabled">
            <v-text-field
               v-model.number="appConfig.ntp.server_port"
               hide-details
               type="number"
               min="1"
               max="65535"
               step="1"
               clearable
               :disabled="loading"
              @click:clear="delete appConfig.ntp?.server_port"
              :label="$t('out.port')"
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2" v-if="appConfig.ntp?.enabled">
            <v-text-field
              v-model="ntpInterval"
              hide-details
               :suffix="$t('date.m')"
               min="1"
               step="1"
               type="number"
               :disabled="loading"
              :label="$t('ruleset.interval')"
            ></v-text-field>
          </v-col>
        </v-row>
        <Dial :dial="appConfig.ntp" :candidate-tags="outboundTags" :candidate-dns-tags="dnsServerTags" :disabled="loading" v-if="appConfig.ntp?.enabled" />
      </v-expansion-panel-text>
    </v-expansion-panel>
    <v-expansion-panel title="Experimental">
      <v-expansion-panel-text>
        <v-row>
          <v-col class="v-card-subtitle">Cache File</v-col>
        </v-row>
        <v-row>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-switch v-model="enableCacheFile" color="primary" :label="$t('enable')" hide-details :disabled="loading"></v-switch>
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2" v-if="appConfig.experimental.cache_file">
            <v-text-field
               v-model="appConfig.experimental.cache_file.path"
               hide-details
               :disabled="loading"
              :label="$t('transport.path')"
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2" v-if="appConfig.experimental.cache_file">
            <v-text-field
               v-model="appConfig.experimental.cache_file.cache_id"
               hide-details
               :disabled="loading"
              label="Cache ID"
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2" v-if="appConfig.experimental.cache_file">
            <v-switch v-model="appConfig.experimental.cache_file.store_fakeip"
               color="primary"
               :label="$t('basic.exp.storeFakeIp')"
               hide-details :disabled="loading"></v-switch>
          </v-col>
        </v-row>
        <v-row>
          <v-col class="v-card-subtitle">Clash API</v-col>
        </v-row>
        <v-row>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-switch v-model="enableClashApi" color="primary" :label="$t('enable')" hide-details :disabled="loading"></v-switch>
          </v-col>
          <template v-if="appConfig.experimental.clash_api">
            <v-col cols="12" sm="6" md="3" lg="2">
              <v-text-field
                v-model="appConfig.experimental.clash_api.external_controller"
                hide-details
                :disabled="loading"
                :label="$t('basic.exp.extController')"
              ></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="3" lg="2">
              <v-text-field
                v-model="appConfig.experimental.clash_api.secret"
                hide-details
                :disabled="loading"
                :label="$t('basic.exp.secret')"
              ></v-text-field>
            </v-col>
          </template>
        </v-row>
        <v-row v-if="appConfig.experimental.clash_api">
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-text-field
              v-model="appConfig.experimental.clash_api.external_ui"
              hide-details
              :disabled="loading"
              :label="$t('basic.exp.extUi')"
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="8" md="4">
            <v-text-field
              v-model="appConfig.experimental.clash_api.external_ui_download_url"
              hide-details
              :disabled="loading"
              :label="$t('basic.exp.extUiDownloadUrl')"
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-select
              v-model="appConfig.experimental.clash_api.external_ui_download_detour"
              hide-details
               :items="outboundTags"
               clearable
               :disabled="loading"
              @click:clear="delete appConfig.experimental.clash_api.external_ui_download_detour"
              :label="$t('basic.exp.extUiDownloadDetour')"
            ></v-select>
          </v-col>
        </v-row>
        <v-row v-if="appConfig.experimental.clash_api">
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-text-field
              v-model="appConfig.experimental.clash_api.default_mode"
              hide-details
              :disabled="loading"
              :label="$t('basic.exp.defaultMode')"
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="8" md="4">
            <v-text-field 
               v-model="origin"
               hide-details
               :disabled="loading"
              :label="$t('basic.exp.allowOrigin') + ' ' + $t('commaSeparated')"
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-switch v-model="appConfig.experimental.clash_api.access_control_allow_private_network" color="primary" :label="$t('basic.exp.allowPrivate')" hide-details :disabled="loading"></v-switch>
          </v-col>
        </v-row>
        <v-row>
          <v-col class="v-card-subtitle">V2Ray API</v-col>
        </v-row>
        <v-row>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-switch v-model="enableV2rayApi" color="primary" :label="$t('enable')" hide-details :disabled="loading"></v-switch>
          </v-col>
          <template v-if="appConfig.experimental.v2ray_api">
            <v-col cols="12" sm="6" md="3" lg="2">
              <v-text-field
                v-model="appConfig.experimental.v2ray_api.listen"
                hide-details
                :disabled="loading"
                :label="$t('objects.listen')"
              ></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="3" lg="2">
              <v-switch v-model="appConfig.experimental.v2ray_api.stats.enabled"
                color="primary"
                :label="$t('stats.enable')"
                hide-details :disabled="loading"></v-switch>
            </v-col>
          </template>
        </v-row>
        <v-row v-if="appConfig.experimental.v2ray_api?.stats?.enabled">
          <v-col cols="12" sm="6">
            <v-select
              hide-details
              :label="$t('pages.inbounds')"
              multiple chips closable-chips
               :items="inboundTags"
               :disabled="loading"
              v-model="appConfig.experimental.v2ray_api.stats.inbounds">
            </v-select>
          </v-col>
          <v-col cols="12" sm="6">
            <v-select
              hide-details
              :label="$t('pages.outbounds')"
              multiple chips closable-chips
               :items="outboundTags"
               :disabled="loading"
              v-model="appConfig.experimental.v2ray_api.stats.outbounds">
            </v-select>
          </v-col>
          <v-col cols="12" sm="6">
            <v-select
              hide-details
              :label="$t('pages.clients')"
              multiple chips closable-chips
               :items="clientNames"
               :disabled="loading"
              v-model="appConfig.experimental.v2ray_api.stats.users">
            </v-select>
          </v-col>
        </v-row>
      </v-expansion-panel-text>
    </v-expansion-panel>
  </v-expansion-panels>
  </template>
</template>

<script lang="ts" setup>
import Data, { type SingboxBasicsSnapshot } from '@/store/modules/data'
import Dial from '@/components/Dial.vue'
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { Config, Ntp } from '@/types/config'
import { FindDiff } from '@/plugins/utils'
import { readSingboxDuration, writeSingboxDuration } from '@/plugins/singboxDuration'
import { push } from 'notivue'

const store = Data()
const snapshot = ref<SingboxBasicsSnapshot | null>(null)
const draftConfig = ref<any>({ experimental: {} })
const oldConfig = ref<any>({ experimental: {} })
const loading = ref(false)
const initializing = ref(true)
const loadFailed = ref(false)
const runtimeRefreshFailed = ref(false)
let componentActive = true

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value ?? {}))
const parseBasics = (raw: any): any => {
  let value: any
  try { value = typeof raw === 'string' ? JSON.parse(raw) : clone(raw ?? {}) } catch { value = {} }
  if (!value || typeof value !== 'object' || Array.isArray(value)) value = {}
  delete value.log
  if (!value.experimental || typeof value.experimental !== 'object' || Array.isArray(value.experimental)) value.experimental = {}
  const v2ray = value.experimental.v2ray_api
  if (v2ray && typeof v2ray === 'object' && !Array.isArray(v2ray)) {
    if (!v2ray.stats || typeof v2ray.stats !== 'object' || Array.isArray(v2ray.stats)) {
      v2ray.stats = { enabled: false, inbounds: [], outbounds: [], users: [] }
    } else {
      if (typeof v2ray.stats.enabled !== 'boolean') v2ray.stats.enabled = false
      if (!Array.isArray(v2ray.stats.inbounds)) v2ray.stats.inbounds = []
      if (!Array.isArray(v2ray.stats.outbounds)) v2ray.stats.outbounds = []
      if (!Array.isArray(v2ray.stats.users)) v2ray.stats.users = []
    }
  }
  return value
}
const applySnapshot = (next: SingboxBasicsSnapshot) => {
  snapshot.value = {
    ...next,
    dialTags: Array.isArray(next.dialTags) ? next.dialTags : [],
    dnsServerTags: Array.isArray(next.dnsServerTags) ? next.dnsServerTags : [],
    inboundTags: Array.isArray(next.inboundTags) ? next.inboundTags : [],
    clientNames: Array.isArray(next.clientNames) ? next.clientNames : [],
  }
  const value = parseBasics(next.basics)
  draftConfig.value = clone(value)
  oldConfig.value = clone(value)
}
const initialize = async () => {
  initializing.value = true
  loadFailed.value = false
  try {
    const next = await store.loadSingboxBasicsSnapshot()
    if (!next || !componentActive) {
      if (componentActive) loadFailed.value = true
      return
    }
    applySnapshot(next)
    runtimeRefreshFailed.value = false
  } catch {
    if (componentActive) loadFailed.value = true
  } finally {
    if (componentActive) initializing.value = false
  }
}

const appConfig = computed((): Config => {
  return <Config> draftConfig.value
})

onMounted(() => { void initialize() })
onUnmounted(() => { componentActive = false })

const stateChange = computed(() => {
  return !FindDiff.deepCompare(appConfig.value, oldConfig.value)
})

const saveConfig = async () => {
  if (loading.value || !snapshot.value || (!stateChange.value && !runtimeRefreshFailed.value)) return
  loading.value = true
  try {
    const payload: Record<string, unknown> = {
      expectedRevision: snapshot.value.revision,
      retryRuntime: runtimeRefreshFailed.value,
    }
    if (stateChange.value) payload.basics = clone(draftConfig.value)
    const response = await store.saveSingboxBasicsMutation(payload)
    if (response.conflict) {
      await initialize()
      push.warning({ title: '基础配置已变更', message: '其他页面或窗口已更新默认 sing-box 配置，请重新加载后再保存。', duration: 7000 })
      return
    }
    if (!response.ok || !response.result || !snapshot.value) return
    applySnapshot({
      ...snapshot.value,
      revision: response.result.revision,
      basics: response.result.basics,
    })
    runtimeRefreshFailed.value = response.runtimeRefreshFailed
    if (response.committed) {
      push.warning({ title: '运行配置未刷新', message: '基础配置已保存；再次保存可重试生成 sing-box 运行配置。', duration: 7000 })
    }
  } finally {
    loading.value = false
  }
}

const inboundTags = computed((): string[] => {
  return snapshot.value?.inboundTags ?? []
})

const clientNames = computed((): string[] => {
  return snapshot.value?.clientNames ?? []
})

const outboundTags = computed((): string[] => {
  return snapshot.value?.dialTags ?? []
})

const dnsServerTags = computed((): string[] => snapshot.value?.dnsServerTags ?? [])

const enableNtp = computed({
  get() { return appConfig.value.ntp?.enabled?? false },
  set(v:boolean) { 
    if (v){
      appConfig.value.ntp = <Ntp>{ enabled: true, server: 'time.apple.com', server_port: 123, interval: '30m'}
    } else { delete appConfig.value.ntp }
  }
})

const ntpInterval = computed({
  get():string {
    const interval = appConfig.value.ntp?.interval
    if (typeof interval !== 'string') return ''
    const trimmed = interval.trim()
    const minutes = readSingboxDuration(trimmed, 'm')
    return minutes === undefined ? trimmed : String(minutes)
  },
  set(v:unknown) {
    if (!appConfig.value.ntp) return
    const raw = String(v ?? '').trim()
    if (raw === '') delete appConfig.value.ntp.interval
    else {
      const normalized = writeSingboxDuration(raw, 'm', { minimum: 1 })
      if (normalized) appConfig.value.ntp.interval = normalized
    }
  }
})

const enableCacheFile = computed({
  get() { return appConfig.value.experimental.cache_file?.enabled?? false },
  set(v:boolean) { 
    if (v){
      appConfig.value.experimental.cache_file = { enabled: true }
    } else { delete appConfig.value.experimental.cache_file  }
  }
})

const enableClashApi = computed({
  get() { return appConfig.value.experimental.clash_api != undefined },
  set(v:boolean) {
    if (v) appConfig.value.experimental.clash_api = { external_controller: '127.0.0.1:9090' }
    else delete appConfig.value.experimental.clash_api
  }
})

const enableV2rayApi = computed({
  get() { return appConfig.value.experimental.v2ray_api != undefined },
  set(v:boolean) {
    if (v) {
      appConfig.value.experimental.v2ray_api = {
        listen: '127.0.0.1:8080',
        stats: { enabled: false, inbounds: [], outbounds: [], users: [] },
      }
    } else delete appConfig.value.experimental.v2ray_api
  }
})

const origin = computed({
  get() { return appConfig.value.experimental.clash_api?.access_control_allow_origin &&
    appConfig.value.experimental.clash_api.access_control_allow_origin.length>0 ? appConfig.value.experimental.clash_api.access_control_allow_origin.join(',') : '' },
  set(v:string) {
    const clashApi = appConfig.value.experimental.clash_api
    if (!clashApi) return
    const origins = v.split(',').map(item => item.trim()).filter(Boolean)
    if (origins.length > 0) clashApi.access_control_allow_origin = origins
    else delete clashApi.access_control_allow_origin
  }
})
</script>
