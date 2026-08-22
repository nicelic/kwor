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
    <DnsVue
      v-model="dnsModal.visible"
      :visible="dnsModal.visible"
      :index="dnsModalIndex"
      :data="dnsModal.data"
      :tsTags="tsTags"
      :rslvdTags="rslvdTags"
      :dialTags="dialTags"
      :busy="loading"
      @close="closeDnsModal"
      @save="saveDnsModal"
    />
    <DnsRuleVue
      v-model="dnsRuleModal.visible"
      :visible="dnsRuleModal.visible"
      :index="dnsRuleModalIndex"
      :data="dnsRuleModal.data"
      :clients="clients"
      :inTags="inboundTags"
       :serverTags="dnsServerTags"
      :ruleSets="ruleSets"
      :busy="loading"
      @close="closeDnsRuleModal"
      @save="saveDnsRuleModal"
    />
    <v-row>
      <v-col cols="12" justify="center" align="center">
        <v-btn color="primary" @click="showDnsModal(null)" :disabled="loading" style="margin: 0 5px;">{{ $t('dns.add') }}</v-btn>
        <v-btn color="primary" @click="showDnsRuleModal(null)" :disabled="loading || !effectiveDnsServerTag" style="margin: 0 5px;">{{ $t('dns.rule.add') }}</v-btn>
        <v-btn variant="outlined" color="warning" @click="saveConfig" :loading="loading" :disabled="loading || (!stateChange && !runtimeRefreshFailed)">
          {{ $t('actions.save') }}
        </v-btn>
      </v-col>
    </v-row>
    <v-row>
      <v-col class="v-card-subtitle" cols="12">{{ $t('pages.basics') }}</v-col>
      <v-col cols="12">
        <v-row>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-select hide-details :label="$t('dns.final')" :items="[ {title: $t('dns.firstServer'), value: ''}, ...dnsServerTags]" :disabled="loading" v-model="finalDns" />
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-select hide-details :label="$t('dns.domainStrategy')" clearable @click:clear="delete dns.strategy" :items="['prefer_ipv4','prefer_ipv6','ipv4_only','ipv6_only']" :disabled="loading" v-model="dns.strategy" />
          </v-col>
          <v-col cols="12" sm="6" md="3" lg="2">
            <v-text-field v-model="dns.client_subnet" hide-details clearable @click:clear="delete dns.client_subnet" :disabled="loading" :label="$t('dns.rule.action.clientSubnet')" />
          </v-col>
          <v-col cols="auto">
             <v-text-field v-model.number="dns.cache_capacity" type="number" min="1024" hide-details clearable @click:clear="delete dns.cache_capacity" :disabled="loading" :label="$t('dns.cacheCapacity')" />
          </v-col>
          <v-col cols="auto"><v-checkbox v-model="dns.disable_cache" :disabled="loading" hide-details :label="$t('dns.disableCache')" /></v-col>
          <v-col cols="auto"><v-checkbox v-model="dns.disable_expire" :disabled="loading" hide-details :label="$t('dns.disableExpire')" /></v-col>
          <v-col cols="auto"><v-checkbox v-model="dns.reverse_mapping" :disabled="loading" hide-details :label="$t('dns.reverseMapping')" /></v-col>
        </v-row>
      </v-col>
    </v-row>
    <v-row>
      <v-col class="v-card-subtitle" cols="12">{{ $t('dns.title') }}</v-col>
      <v-col cols="12" sm="4" md="3" lg="2" v-for="(item, index) in servers" :key="item.id">
        <v-card rounded="xl" elevation="5" min-width="200" :title="item.tag">
          <v-card-subtitle style="margin-top: -20px;"><v-row><v-col>{{ item.type }}</v-col></v-row></v-card-subtitle>
          <v-card-text>
            <v-row><v-col>{{ $t('dns.server') }}</v-col><v-col>{{ item.server?? '-' }}</v-col></v-row>
            <v-row><v-col>{{ $t('in.port') }}</v-col><v-col>{{ item.server_port?? '-' }}</v-col></v-row>
            <v-row><v-col>{{ $t('objects.tls') }}</v-col><v-col>{{ Object.hasOwn(item,'tls') ? $t(item.tls?.enabled ? 'enable' : 'disable') : '-' }}</v-col></v-row>
          </v-card-text>
          <v-divider />
          <v-card-actions style="padding: 0;">
            <v-btn icon="mdi-file-edit" :disabled="loading" @click="showDnsModal(item.id)"><v-icon /><v-tooltip activator="parent" location="top" :text="$t('actions.edit')" /></v-btn>
            <v-btn icon="mdi-file-remove" style="margin-inline-start:0;" color="warning" :disabled="loading" @click="delDnsOverlay[item.id] = true"><v-icon /><v-tooltip activator="parent" location="top" :text="$t('actions.del')" /></v-btn>
            <v-overlay v-model="delDnsOverlay[item.id]" contained class="align-center justify-center">
              <v-card :title="$t('actions.del')" rounded="lg"><v-divider /><v-card-text>{{ $t('confirm') }}</v-card-text><v-card-actions>
                <v-btn color="error" variant="outlined" :loading="loading" :disabled="loading" @click="delDns(item.id)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" :disabled="loading" @click="delDnsOverlay[item.id] = false">{{ $t('no') }}</v-btn>
              </v-card-actions></v-card>
            </v-overlay>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>
    <v-row>
      <v-col class="v-card-subtitle" cols="12">{{ $t('dns.rule.title') }}</v-col>
      <v-col cols="12" sm="4" md="3" lg="2" v-for="(item, index) in dnsRules" :key="getDnsRuleKey(item, index)" :draggable="!loading" @dragstart="onDragStart(index)" @dragover.prevent @drop="onDrop(index)">
        <v-card rounded="xl" elevation="5" min-width="200" :title="index+1">
          <v-card-subtitle style="margin-top: -20px;"><v-row><v-col>{{ item.type != undefined ? $t('rule.logical') + ' (' + item.mode + ')' : $t('rule.simple') }}</v-col></v-row></v-card-subtitle>
          <v-card-text>
            <v-row><v-col>{{ $t('admin.action') }}</v-col><v-col>{{ item.action }}</v-col></v-row>
            <v-row><v-col>{{ $t('dns.server') }}</v-col><v-col>{{ item.server?? '-' }}</v-col></v-row>
            <v-row><v-col>{{ $t('pages.rules') }}</v-col><v-col>{{ (item as any).rules ? (item as any).rules.length : Object.keys(item).filter(r => !actionDnsRuleKeys.includes(r)).length }}</v-col></v-row>
            <v-row><v-col>{{ $t('rule.invert') }}</v-col><v-col>{{ $t((item.invert?? false)? 'yes' : 'no') }}</v-col></v-row>
          </v-card-text>
          <v-divider />
          <v-card-actions style="padding: 0;">
            <v-btn icon="mdi-file-edit" :disabled="loading" @click="showDnsRuleModal(item)"><v-icon /><v-tooltip activator="parent" location="top" :text="$t('actions.edit')" /></v-btn>
            <v-btn icon="mdi-file-remove" style="margin-inline-start:0;" color="warning" :disabled="loading" @click="requestDnsRuleDelete(item)"><v-icon /><v-tooltip activator="parent" location="top" :text="$t('actions.del')" /></v-btn>
            <v-overlay :model-value="dnsRuleDeleteTarget === item" contained class="align-center justify-center" @update:model-value="value => { if (!value) closeDnsRuleDelete(item) }">
              <v-card :title="$t('actions.del')" rounded="lg"><v-divider /><v-card-text>{{ $t('confirm') }}</v-card-text><v-card-actions>
                <v-btn color="error" variant="outlined" :disabled="loading" @click="delDnsRule(item)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" :disabled="loading" @click="closeDnsRuleDelete(item)">{{ $t('no') }}</v-btn>
              </v-card-actions></v-card>
            </v-overlay>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>
  </template>
</template>

<script lang="ts" setup>
import Data, { type SingboxDNSSnapshot } from '@/store/modules/data'
import { computed, ref, onMounted, onUnmounted } from 'vue'
import { push } from 'notivue'
import DnsVue from '@/layouts/modals/Dns.vue'
import DnsRuleVue from '@/layouts/modals/DnsRule.vue'
import { actionDnsRuleKeys, dnsRule } from '@/types/dns'
import { FindDiff } from '@/plugins/utils'

const store = Data()
const snapshot = ref<SingboxDNSSnapshot | null>(null)
const draftDns = ref<any>({ rules: [] })
const oldDns = ref<any>({ rules: [] })
const loading = ref(false)
const initializing = ref(true)
const initialized = ref(false)
const loadFailed = ref(false)
const runtimeRefreshFailed = ref(false)
let componentActive = true
let singboxContextRefreshTimer: number | undefined
let singboxContextRefreshBusy = false

const clone = <T>(value: T): T => JSON.parse(JSON.stringify(value ?? {}))
const parseDns = (raw: any): any => {
  let value: any
  try { value = typeof raw === 'string' ? JSON.parse(raw) : clone(raw ?? {}) } catch { value = {} }
  if (!value || typeof value !== 'object' || Array.isArray(value)) return { rules: [] }
  delete value.servers
  delete value.independent_cache
  if (!Array.isArray(value.rules)) value.rules = []
  return value
}
const applySnapshot = (next: SingboxDNSSnapshot) => {
  snapshot.value = { ...next, servers: Array.isArray(next.servers) ? next.servers : [] }
  const value = parseDns(next.dns)
  draftDns.value = clone(value)
  oldDns.value = clone(value)
}
const initialize = async () => {
  initializing.value = true
  loadFailed.value = false
  initialized.value = false
  try {
    const next = await store.loadSingboxDNSSnapshot()
    if (!next || !componentActive) { if (componentActive) loadFailed.value = true; return }
    applySnapshot(next)
    runtimeRefreshFailed.value = false
    initialized.value = true
  } catch { if (componentActive) loadFailed.value = true }
  finally { if (componentActive) initializing.value = false }
}
const refreshSingboxDNSContextWhenClean = async () => {
  if ((typeof document !== 'undefined' && document.visibilityState !== 'visible') || !initialized.value || loading.value || stateChange.value || singboxContextRefreshBusy) return
  if (dnsModal.value.visible || dnsRuleModal.value.visible || dnsRuleDeleteTarget.value !== null) return
  singboxContextRefreshBusy = true
  try {
    const next = await store.loadSingboxDNSSnapshot()
    if (!next || !componentActive) return
    if (Number(next.revision ?? 0) <= Number(snapshot.value?.revision ?? 0)) return
    applySnapshot(next)
    runtimeRefreshFailed.value = false
  } finally {
    singboxContextRefreshBusy = false
  }
}

const stopContextRefreshTimer = () => {
  if (singboxContextRefreshTimer === undefined) return
  window.clearTimeout(singboxContextRefreshTimer)
  singboxContextRefreshTimer = undefined
}

const scheduleContextRefresh = (delay = 30_000) => {
  if ((typeof document !== 'undefined' && document.visibilityState !== 'visible') || !componentActive) return
  if (singboxContextRefreshTimer !== undefined) window.clearTimeout(singboxContextRefreshTimer)
  singboxContextRefreshTimer = window.setTimeout(async () => {
    singboxContextRefreshTimer = undefined
    try {
      await refreshSingboxDNSContextWhenClean()
    } catch {
      // Retry on the next clean-context pass.
    } finally {
      scheduleContextRefresh()
    }
  }, delay)
}

const startContextRefreshTimer = () => {
  if ((typeof document !== 'undefined' && document.visibilityState !== 'visible') || singboxContextRefreshTimer !== undefined) return
  scheduleContextRefresh()
}
onMounted(() => {
  void initialize()
  startContextRefreshTimer()
  if (typeof document !== 'undefined') document.addEventListener('visibilitychange', handleVisibilityChange)
})
onUnmounted(() => {
  componentActive = false
  stopContextRefreshTimer()
  if (typeof document !== 'undefined') document.removeEventListener('visibilitychange', handleVisibilityChange)
})

const handleVisibilityChange = () => {
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') {
    stopContextRefreshTimer()
    return
  }
  startContextRefreshTimer()
  void refreshSingboxDNSContextWhenClean()
}

const dns = computed(() => draftDns.value)
const servers = computed(() => snapshot.value?.servers ?? [])
const stateChange = computed(() => !FindDiff.deepCompare(draftDns.value, oldDns.value))
const tsTags = computed(() => snapshot.value?.tailscaleTags ?? [])
const rslvdTags = computed(() => snapshot.value?.resolvedTags ?? [])
const clients = computed(() => snapshot.value?.clientNames ?? [])
const inboundTags = computed(() => snapshot.value?.inboundTags ?? [])
const ruleSets = computed(() => snapshot.value?.ruleSetTags ?? [])
const dnsServerTags = computed(() => servers.value.map((s: any) => s.tag).filter((tag: any) => typeof tag === 'string' && tag.trim() !== ''))
const dialTags = computed(() => snapshot.value?.dialTags ?? [])
const finalDns = computed({ get: () => dns.value?.final ?? '', set: (value: string) => value ? dns.value.final = value : delete dns.value.final })
const dnsRules = computed((): any[] => Array.isArray(dns.value.rules) ? dns.value.rules : [])
const effectiveDnsServerTag = computed(() => {
  const requested = typeof dns.value?.final === 'string' ? dns.value.final.trim() : ''
  if (requested && dnsServerTags.value.includes(requested)) return requested
  return dnsServerTags.value[0] ?? ''
})
const effectiveDnsServerTags = computed(() => effectiveDnsServerTag.value ? [effectiveDnsServerTag.value] : [])

const dnsModal = ref<{ visible: boolean, serverId: number | null, data: string }>({ visible: false, serverId: null, data: '' })
const dnsModalIndex = computed(() => {
  if (dnsModal.value.serverId === null) return -1
  return servers.value.findIndex((server: any) => Number(server?.id) === Number(dnsModal.value.serverId))
})
const showDnsModal = (serverId: number | null) => {
  if (loading.value) return
  if (serverId === null) {
    dnsModal.value = { visible: true, serverId: null, data: '' }
    return
  }
  const server = servers.value.find((item: any) => Number(item?.id) === Number(serverId))
  if (!server) return
  dnsModal.value = { visible: true, serverId: Number(server.id), data: JSON.stringify(server) }
}
const closeDnsModal = () => { if (!loading.value) dnsModal.value.visible = false }
// Keep confirmation state keyed by the database identity. List indexes can
// change after a concurrent refresh and must never select another DNS card.
const delDnsOverlay = ref<Record<number, boolean>>({})

const dnsRuleModal = ref<{ visible: boolean, rule: any | null, data: string }>({ visible: false, rule: null, data: '' })
const dnsRuleModalIndex = computed(() => {
  if (dnsRuleModal.value.rule === null) return -1
  return dnsRules.value.indexOf(dnsRuleModal.value.rule)
})
const showDnsRuleModal = (rule: any | null) => {
  if (loading.value) return
  if (rule === null) {
    dnsRuleModal.value = { visible: true, rule: null, data: '' }
    return
  }
  const index = dnsRules.value.indexOf(rule)
  if (index < 0) return
  dnsRuleModal.value = { visible: true, rule, data: JSON.stringify(rule) }
}
const closeDnsRuleModal = () => { if (!loading.value) dnsRuleModal.value.visible = false }
const dnsRuleDeleteTarget = ref<any | null>(null)
const dnsRuleKeys = new WeakMap<object, string>()
const getDnsRuleKey = (item: any, index: number): string => {
  if (item && typeof item === 'object') {
    const existing = dnsRuleKeys.get(item)
    if (existing) return existing
    const key = `dns-rule:${typeof globalThis.crypto?.randomUUID === 'function' ? globalThis.crypto.randomUUID() : `${Date.now()}:${index}:${Math.random()}`}`
    dnsRuleKeys.set(item, key)
    return key
  }
  return `dns-rule:${index}`
}

const refreshFromResponse = async (response: Awaited<ReturnType<typeof store.saveSingboxDNSMutation>>) => {
  if (response.conflict) {
    await initialize()
    push.warning({ title: 'DNS 配置已变更', message: '其他页面或窗口已更新默认 sing-box DNS，请重新加载后再保存。', duration: 7000 })
    return false
  }
  if (!response.ok || !response.result?.snapshot) return false
  applySnapshot(response.result.snapshot)
  runtimeRefreshFailed.value = response.runtimeRefreshFailed
  return true
}

const saveDnsModal = async (data: any) => {
  if (loading.value || !snapshot.value) return
  const serverId = dnsModal.value.serverId
  const isNew = serverId === null
  if (!isNew && !servers.value.some((server: any) => Number(server?.id) === Number(serverId))) {
    dnsModal.value.visible = false
    push.warning({ title: 'DNS', message: 'The selected DNS server is no longer present.' })
    return
  }
  const previousModal = { visible: true, serverId, data: JSON.stringify(data) }
  dnsModal.value.visible = false
  loading.value = true
  try {
    const response = await store.saveSingboxDNSMutation({
      expectedRevision: snapshot.value.revision,
      serverAction: isNew ? 'new' : 'edit',
      serverId: isNew ? undefined : serverId,
      server: clone(data),
    })
    const refreshed = await refreshFromResponse(response)
    if (!refreshed && !response.conflict) dnsModal.value = previousModal
  } finally { loading.value = false }
}
const delDns = async (id: number) => {
  if (loading.value || !snapshot.value) return
  const server = servers.value.find((item: any) => Number(item?.id) === Number(id))
  if (!server?.id) return
  loading.value = true
  try {
    const response = await store.saveSingboxDNSMutation({ expectedRevision: snapshot.value.revision, serverAction: 'del', serverId: server.id })
    if (await refreshFromResponse(response)) delete delDnsOverlay.value[Number(server.id)]
  } finally { loading.value = false }
}
const saveDnsRuleModal = (data: dnsRule) => {
  if (loading.value) return
  const currentRule = dnsRuleModal.value.rule
  if (currentRule === null) {
    dns.value.rules.push(data)
  } else {
    const index = dnsRules.value.indexOf(currentRule)
    if (index < 0) {
      dnsRuleModal.value.visible = false
      push.warning({ title: 'DNS', message: 'The selected DNS rule is no longer present.' })
      return
    }
    dns.value.rules[index] = data
  }
  dnsRuleModal.value.visible = false
}
const requestDnsRuleDelete = (rule: any) => {
  if (loading.value) return
  dnsRuleDeleteTarget.value = rule
}
const closeDnsRuleDelete = (rule: any) => {
  if (dnsRuleDeleteTarget.value === rule) dnsRuleDeleteTarget.value = null
}
const delDnsRule = (rule: any) => {
  if (loading.value) return
  const index = dnsRules.value.indexOf(rule)
  if (index < 0) {
    dnsRuleDeleteTarget.value = null
    push.warning({ title: 'DNS', message: 'The selected DNS rule is no longer present.' })
    return
  }
  dns.value.rules.splice(index, 1)
  dnsRuleDeleteTarget.value = null
}

const draggedItemIndex = ref<number | null>(null)
const onDragStart = (index: number) => { if (!loading.value) draggedItemIndex.value = index }
const onDrop = (index: number) => {
  if (loading.value || draggedItemIndex.value == null) return
  const from = draggedItemIndex.value
  if (from < 0 || from >= dnsRules.value.length || index < 0 || index >= dnsRules.value.length || from === index) {
    draggedItemIndex.value = null
    return
  }
  const item = dnsRules.value[from]
  dns.value.rules.splice(from, 1)
  const target = from < index ? index - 1 : index
  dns.value.rules.splice(target, 0, item)
  draggedItemIndex.value = null
}
const saveConfig = async () => {
  if (loading.value || !snapshot.value || (!stateChange.value && !runtimeRefreshFailed.value)) return
  loading.value = true
  try {
    const payload: Record<string, unknown> = {
      expectedRevision: snapshot.value.revision,
      retryRuntime: runtimeRefreshFailed.value,
    }
    if (stateChange.value) payload.dns = clone(draftDns.value)
    const response = await store.saveSingboxDNSMutation(payload)
    await refreshFromResponse(response)
  } finally { loading.value = false }
}
</script>
