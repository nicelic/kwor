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
    <RuleVue
      v-model="ruleModal.visible"
      :visible="ruleModal.visible"
      :index="ruleModal.index"
      :data="ruleModal.data"
      :namespace="props.namespace"
      :clients="clients"
      :inTags="inboundTags"
      :outTags="outboundTags"
      :rsTags="rulesetTags"
      @close="closeRuleModal"
      @save="saveRuleModal"
    />
    <RulesetVue
      v-model="rulesetModal.visible"
      :visible="rulesetModal.visible"
      :index="rulesetModal.index"
      :data="rulesetModal.data"
      :namespace="props.namespace"
      :outTags="outboundTags"
      @close="closeRulesetModal"
      @save="saveRulesetModal"
    />
  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn color="primary" class="mx-1" @click="showRuleModal(-1)" :disabled="!initialized || loading || ruleLimitReached">{{ $t('rule.add') }}</v-btn>
      <v-btn color="primary" class="mx-1" @click="showRulesetModal(-1)" :disabled="!initialized || loading || rulesetLimitReached">{{ $t('ruleset.add') }}</v-btn>
      <v-btn variant="outlined" color="warning" @click="saveConfig" :loading="loading" :disabled="loading || (isPristine && !runtimeRefreshFailed) || !initialized || ruleModal.visible || rulesetModal.visible">
        {{ $t('actions.save') }}
      </v-btn>
    </v-col>
  </v-row>
  <v-row>
    <v-col class="v-card-subtitle" cols="12">{{ $t('basic.routing.title') }}</v-col>
    <v-col cols="12">
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select
            hide-details
              :label="$t('basic.routing.defaultOut')"
              clearable
              @click:clear="clearRouteFinal"
              @update:model-value="markDirty"
              :items="outboundTags"
              :disabled="editingDisabled"
              v-model="route.final">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-text-field
            v-model="route.default_interface"
            hide-details
            clearable
            @click:clear="clearDefaultInterface"
            @update:model-value="markDirty"
            :disabled="editingDisabled"
            :label="$t('basic.routing.defaultIf')">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
            <v-text-field
            v-model="routeMarkInput"
            hide-details
            type="number"
            min="0"
            :disabled="editingDisabled"
            :label="$t('basic.routing.defaultRm')">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-switch
            v-model="route.auto_detect_interface"
            color="primary"
            @update:model-value="markDirty"
            :disabled="editingDisabled"
            :label="$t('basic.routing.autoBind')"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="props.namespace === 'mihomo'">
          <v-switch
            v-model="mihomoSniffUi"
            color="primary"
            :disabled="editingDisabled"
            label="sniff（HTTP/TLS/QUIC 全端口）"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="props.namespace === 'mihomo'">
          <v-switch
            v-model="mihomoNoResolveUi"
            color="primary"
            :disabled="editingDisabled"
            label="no-resolve_全局开关"
            hide-details>
          </v-switch>
        </v-col>
      </v-row>
    </v-col>
  </v-row>
  <v-row>
    <v-col class="v-card-subtitle" cols="12">{{ $t('rule.ruleset') }}</v-col>
    <v-col cols="12" sm="6" md="4" lg="3" v-for="entry in paginatedRulesets" :key="getRulesetCardKey(entry.item, entry.index)">
        <v-card rounded="lg" elevation="2" class="h-100" :title="entry.item.tag">
        <v-card-subtitle>
          <v-row>
            <v-col>{{ props.namespace === 'mihomo' ? entry.item.type : (entry.item.type === 'inline' ? 'Inline' : $t('ruleset.' + entry.item.type)) }}</v-col>
          </v-row>
        </v-card-subtitle>
        <v-card-text>
          <v-row v-if="props.namespace === 'mihomo'">
            <v-col>behavior</v-col>
            <v-col>{{ entry.item.behavior ?? 'classical' }}</v-col>
          </v-row>
          <v-row v-if="entry.item.type !== 'inline'">
            <v-col>{{ $t('ruleset.format') }}</v-col>
            <v-col>{{ entry.item.format }}</v-col>
          </v-row>
          <v-row v-else>
            <v-col>Inline Rules</v-col>
            <v-col>{{ Array.isArray(entry.item.rules) ? entry.item.rules.length : 0 }}</v-col>
          </v-row>
          <v-row v-if="entry.item.type !== 'inline'">
            <v-col>{{ $t('actions.update') }}</v-col>
            <v-col>{{ entry.item.update_interval ?? '-' }}</v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" :disabled="editingDisabled" @click="showRulesetModal(entry.index)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" class="ms-0" color="warning" :disabled="editingDisabled" @click="requestDelete('ruleset', entry.index)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-col>
    <v-col v-if="rulesetPageCount > 1" cols="12" class="d-flex justify-center">
      <v-pagination v-model="rulesetPage" :length="rulesetPageCount" :total-visible="5" density="comfortable" />
    </v-col>
  </v-row>
  <v-row>
    <v-col class="v-card-subtitle" cols="12">{{ $t('pages.rules') }}</v-col>
    <v-col
      cols="12"
      sm="6"
      md="4"
      lg="3"
      v-for="entry in paginatedRules"
       :key="getRuleCardKey(entry.item, entry.index)"
       :draggable="!editingDisabled"
       @dragstart="onDragStart(entry.index)"
       @dragend="clearDraggedItem"
       @dragover.prevent
      @drop="onDrop(entry.index)">
      <v-card rounded="lg" elevation="2" class="h-100" :title="entry.index + 1">
        <v-card-subtitle>
          <v-row>
            <v-col>{{ entry.item.type != undefined ? $t('rule.logical') + ' (' + entry.item.mode + ')' : $t('rule.simple') }}</v-col>
          </v-row>
        </v-card-subtitle>
        <v-card-text>
          <v-row>
            <v-col>{{ $t('admin.action') }}</v-col>
            <v-col>{{ entry.item.action }}</v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('objects.outbound') }}</v-col>
            <v-col>{{ entry.item.outbound ?? '-' }}</v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('pages.rules') }}</v-col>
            <v-col>{{ entry.item.rules ? entry.item.rules.length : Object.keys(entry.item).filter(r => !actionKeys.includes(r)).length }}</v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('rule.invert') }}</v-col>
            <v-col>{{ $t((entry.item.invert ?? false) ? 'yes' : 'no') }}</v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" :disabled="editingDisabled" @click="showRuleModal(entry.index)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn
            icon="mdi-arrow-up"
            :disabled="editingDisabled || entry.index === 0"
            @click="moveRule(entry.index, -1)">
            <v-icon />
            <v-tooltip activator="parent" location="top" text="上移规则"></v-tooltip>
          </v-btn>
          <v-btn
            icon="mdi-arrow-down"
            :disabled="editingDisabled || entry.index === rules.length - 1"
            @click="moveRule(entry.index, 1)">
            <v-icon />
            <v-tooltip activator="parent" location="top" text="下移规则"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" class="ms-0" color="warning" :disabled="editingDisabled" @click="requestDelete('rule', entry.index)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-col>
    <v-col v-if="rulePageCount > 1" cols="12" class="d-flex justify-center">
      <v-pagination v-model="rulePage" :length="rulePageCount" :total-visible="5" density="comfortable" />
    </v-col>
  </v-row>
  <v-dialog v-model="deleteConfirmVisible" max-width="95vw" width="420">
    <v-card rounded="lg" :title="$t('actions.del')">
      <v-divider />
      <v-card-text>{{ $t('confirm') }}</v-card-text>
      <v-card-actions>
        <v-spacer />
        <v-btn color="error" variant="outlined" :disabled="editingDisabled" @click="confirmDelete">{{ $t('yes') }}</v-btn>
        <v-btn color="primary" variant="outlined" :disabled="editingDisabled" @click="deleteConfirmVisible = false">{{ $t('no') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
  </template>
</template>

<script lang="ts" setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { push } from 'notivue'
import { i18n } from '@/locales'
import HttpUtils from '@/plugins/httputil'
import RuleVue from '@/layouts/modals/Rule.vue'
import RulesetVue from '@/layouts/modals/Ruleset.vue'
import { Config } from '@/types/config'
import {
  actionKeys,
  ruleset,
  normalizeMihomoRouteNoResolve,
  sanitizeRouteForNamespace,
  sanitizeRuleForNamespace,
  validateRouteForNamespace,
  validateRuleForNamespace,
  validateRulesetForNamespace,
  mihomoRouteResourceLimits,
  validateMihomoRouteResourceBounds,
  singboxRouteResourceLimits,
  validateSingboxRouteResourceBounds,
} from '@/types/rules'
import { getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'
import Data from '@/store/modules/data'
import MihomoData from '@/store/modules/mihomoData'

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const store = getNamespaceStore(props.namespace)
const singboxStore = Data()
const mihomoStore = MihomoData()
const appConfig = ref<Config>({} as Config)
const loading = ref(false)
const initialized = ref(false)
const initializing = ref(true)
const loadFailed = ref(false)
let componentActive = true
let singboxContextRefreshTimer: number | undefined
let singboxContextRefreshBusy = false
const editingDisabled = computed(() => !initialized.value || loading.value)
const dirty = ref(false)
const singboxRouteRevision = ref(0)
const singboxRouteInboundTags = ref<string[]>([])
const singboxRouteOutboundTags = ref<string[]>([])
const singboxRouteClientNames = ref<string[]>([])
const runtimeRefreshFailed = ref(false)
const replacingDraft = ref(false)
const mihomoSniffUi = ref(false)
const mihomoSniffUiTouched = ref(false)
const mihomoNoResolveUi = ref(true)
const mihomoRouteTargets = ref<string[]>([])
const mihomoRouteInboundTags = ref<string[]>([])
const mihomoRouteRevision = ref(0)
const rulePage = ref(1)
const rulesetPage = ref(1)
const pageSize = 24
const deleteConfirmVisible = ref(false)
const deleteRequest = ref<{ kind: 'rule' | 'ruleset', target: any } | null>(null)
const ruleCardKeys = new WeakMap<object, string>()
const rulesetCardKeys = new WeakMap<object, string>()

const cloneConfig = (value: any): Config => {
  return JSON.parse(JSON.stringify(value ?? {}))
}

const isDocumentVisible = () => typeof document === 'undefined' || document.visibilityState === 'visible'

const normalizeEditableConfig = (value: Config): Config => {
  const nextConfig = cloneConfig(value)
  if (!nextConfig.route || typeof nextConfig.route !== 'object') {
    nextConfig.route = {
      rules: [],
      rule_set: [],
    }
  }
  nextConfig.route = sanitizeRouteForNamespace(nextConfig.route, props.namespace)
  if (!Array.isArray(nextConfig.route.rules)) {
    nextConfig.route.rules = []
  }
  if (!Array.isArray(nextConfig.route.rule_set)) {
    nextConfig.route.rule_set = []
  }
  return nextConfig
}

const normalizeRoute = () => {
  if (!appConfig.value) {
    return
  }
  appConfig.value = normalizeEditableConfig(appConfig.value)
}

const replaceDraft = async (value: Config) => {
  replacingDraft.value = true
  appConfig.value = normalizeEditableConfig(value)
  mihomoSniffUiTouched.value = false
  syncMihomoSniffUiFromConfig()
  syncMihomoNoResolveUiFromConfig()
  await nextTick()
  mihomoSniffUiTouched.value = false
  replacingDraft.value = false
}

const loadMihomoRouteEditorContext = async (): Promise<boolean> => {
  if (props.namespace !== 'mihomo') return false
  const msg = await HttpUtils.get('api/mihomo-route-editor-context')
  if (msg.success) {
    mihomoRouteTargets.value = msg.obj?.routeTargets ?? []
    mihomoRouteInboundTags.value = msg.obj?.inboundTags ?? []
    mihomoRouteRevision.value = Number(msg.obj?.revision ?? mihomoRouteRevision.value)
    await replaceDraft(msg.obj?.config ?? {})
    return true
  }
  return false
}

const refreshMihomoRouteContextWhenClean = async () => {
  if (props.namespace !== 'mihomo' || !isDocumentVisible() || !initialized.value || dirty.value || loading.value) return
  if (ruleModal.value.visible || rulesetModal.value.visible || singboxContextRefreshBusy) return
  singboxContextRefreshBusy = true
  try {
    const msg = await HttpUtils.get('api/mihomo-route-editor-context')
    if (!msg.success || !msg.obj || !componentActive) return
    const nextRevision = Number(msg.obj.revision ?? 0)
    if (Number.isSafeInteger(nextRevision) && nextRevision > 0 && nextRevision <= mihomoRouteRevision.value) return
    mihomoRouteTargets.value = Array.isArray(msg.obj.routeTargets) ? msg.obj.routeTargets : []
    mihomoRouteInboundTags.value = Array.isArray(msg.obj.inboundTags) ? msg.obj.inboundTags : []
    mihomoRouteRevision.value = nextRevision > 0 ? nextRevision : mihomoRouteRevision.value
    await replaceDraft(msg.obj.config ?? {})
    dirty.value = false
  } finally {
    singboxContextRefreshBusy = false
  }
}

const loadSingboxRouteEditorContext = async (preserveDraft = false): Promise<boolean> => {
  if (props.namespace === 'mihomo') return false
  const msg = await HttpUtils.get('api/singbox-route-editor-context')
  if (!msg.success || !msg.obj) return false
  return applySingboxRouteContext(msg.obj, preserveDraft)
}

const applySingboxRouteContext = async (context: any, preserveDraft = false): Promise<boolean> => {
  if (!context || typeof context !== 'object') return false
  singboxRouteRevision.value = Number(context.revision ?? 0)
  singboxRouteInboundTags.value = Array.isArray(context.inboundTags) ? context.inboundTags : []
  singboxRouteOutboundTags.value = Array.isArray(context.outboundTags) ? context.outboundTags : []
  singboxRouteClientNames.value = Array.isArray(context.clientNames) ? context.clientNames : []
  if (!preserveDraft) {
    await replaceDraft({ route: context.route ?? {} } as Config)
  }
  return true
}

const refreshSingboxRouteContextWhenClean = async () => {
  if (props.namespace === 'mihomo' || !isDocumentVisible() || !initialized.value || dirty.value || loading.value || singboxContextRefreshBusy) return
  if (ruleModal.value.visible || rulesetModal.value.visible) return
  singboxContextRefreshBusy = true
  try {
    const msg = await HttpUtils.get('api/singbox-route-editor-context')
    if (!msg.success || !msg.obj || !componentActive) return
    const nextRevision = Number(msg.obj.revision ?? 0)
    if (!Number.isSafeInteger(nextRevision) || nextRevision <= singboxRouteRevision.value) return
    await applySingboxRouteContext(msg.obj, false)
    dirty.value = false
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
  if (!isDocumentVisible() || !componentActive) return
  if (singboxContextRefreshTimer !== undefined) window.clearTimeout(singboxContextRefreshTimer)
  singboxContextRefreshTimer = window.setTimeout(async () => {
    singboxContextRefreshTimer = undefined
    try {
      if (props.namespace === 'mihomo') {
        await refreshMihomoRouteContextWhenClean()
      } else {
        await refreshSingboxRouteContextWhenClean()
      }
    } catch {
      // Retry on the next clean-context pass.
    } finally {
      scheduleContextRefresh()
    }
  }, delay)
}

const startContextRefreshTimer = () => {
  if (!isDocumentVisible() || singboxContextRefreshTimer !== undefined) return
  scheduleContextRefresh()
}

const syncMihomoSniffUiFromConfig = () => {
  if (props.namespace !== 'mihomo') {
    return
  }
  const sniffer = (<any>appConfig.value)?.sniffer
  if (sniffer && typeof sniffer === 'object') {
    mihomoSniffUi.value = sniffer.enable === true
    return
  }
  mihomoSniffUi.value = sniffer === true
}

const syncMihomoSnifferConfig = () => {
  if (props.namespace !== 'mihomo') {
    return
  }
  if (mihomoSniffUi.value) {
    const current = (<any>appConfig.value)?.sniffer
    if (current && typeof current === 'object') {
      current.enable = true
    } else {
      ;(<any>appConfig.value).sniffer = { enable: true }
    }
    return
  }
  delete (<any>appConfig.value).sniffer
}

const syncMihomoNoResolveUiFromConfig = () => {
  if (props.namespace !== 'mihomo') {
    return
  }
  mihomoNoResolveUi.value = normalizeMihomoRouteNoResolve(route.value)
}

const syncMihomoNoResolveConfig = () => {
  if (props.namespace !== 'mihomo') {
    return
  }
  route.value.no_resolve = mihomoNoResolveUi.value === true
}

const markDirty = () => {
  if (initialized.value && !replacingDraft.value) {
    dirty.value = true
  }
}

const clearRouteFinal = () => {
  delete route.value.final
  markDirty()
}

const clearDefaultInterface = () => {
  delete route.value.default_interface
  markDirty()
}

const route = computed((): any => {
  return appConfig.value.route ?? {}
})

const initialize = async () => {
  initializing.value = true
  loadFailed.value = false
  initialized.value = false
  try {
    let success = false
    if (props.namespace === 'mihomo') {
      success = await loadMihomoRouteEditorContext()
      if (!success) {
        const storeLoaded = await store.loadData()
        if (storeLoaded || store.hasFullData) {
          await replaceDraft(store.config)
          success = true
        }
      }
    } else {
      success = await loadSingboxRouteEditorContext()
    }
    if (!componentActive) return
    if (!success) {
      loadFailed.value = true
      return
    }
    initialized.value = true
  } catch {
    if (componentActive) loadFailed.value = true
  } finally {
    if (componentActive) initializing.value = false
  }
}

onMounted(() => {
  void initialize()
  startContextRefreshTimer()
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
})

onUnmounted(() => {
  componentActive = false
  stopContextRefreshTimer()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
})

const handleVisibilityChange = () => {
  if (!isDocumentVisible()) {
    stopContextRefreshTimer()
    return
  }
  startContextRefreshTimer()
  if (props.namespace === 'mihomo') {
    void refreshMihomoRouteContextWhenClean()
  } else {
    void refreshSingboxRouteContextWhenClean()
  }
}

watch(mihomoSniffUi, () => {
  if (props.namespace !== 'mihomo' || replacingDraft.value || !initialized.value) return
  mihomoSniffUiTouched.value = true
  syncMihomoSnifferConfig()
  markDirty()
})

watch(mihomoNoResolveUi, () => {
  syncMihomoNoResolveConfig()
  markDirty()
})

watch(
  () => (<any>appConfig.value)?.sniffer,
  () => {
    syncMihomoSniffUiFromConfig()
  },
  { deep: true },
)

watch(
  () => [route.value?.no_resolve, route.value?.['no-resolve'], route.value?.noResolve],
  () => {
    syncMihomoNoResolveUiFromConfig()
  },
)

const routeMarkInput = computed({
  get() {
    return route.value.default_mark == null ? '' : String(route.value.default_mark)
  },
  set(value: string) {
    const text = String(value ?? '').trim()
    if (text === '') {
      delete appConfig.value.route.default_mark
    } else if (/^\d+$/.test(text)) {
      const parsed = Number(text)
      if (Number.isSafeInteger(parsed) && parsed >= 0) {
        route.value.default_mark = parsed
      }
    } else {
      // Keep the complete token in the input; server-side validation will
      // reject it instead of silently truncating a malformed value.
      route.value.default_mark = text as unknown as number
    }
    markDirty()
  },
})

const isPristine = computed(() => !dirty.value)

watch(
  () => store.config,
  (config) => {
    if (props.namespace !== 'mihomo' || !initialized.value || dirty.value) {
      return
    }
    void replaceDraft(config)
  },
)

const showValidationErrors = (errors: string[]) => {
  Array.from(new Set(errors)).forEach((message) => {
    push.warning({
      title: i18n.global.t('failed'),
      duration: 5000,
      message,
    })
  })
}

const validateRulesetModalData = (data: ruleset): string[] => {
  const errors = validateRulesetForNamespace(data, props.namespace, {
    outboundTags: outboundTags.value,
  })
  const tag = typeof data?.tag === 'string' ? data.tag.trim() : ''
  if (errors.length > 0) {
    return errors
  }

  const duplicateIndex = rulesets.value.findIndex((item: any, index: number) => {
    if (index === rulesetModal.value.index) {
      return false
    }
    return typeof item?.tag === 'string' && item.tag.trim() === tag
  })
  if (duplicateIndex !== -1) {
    return [`Rule set tag "${tag}" is duplicated.`]
  }

  return []
}

const saveConfig = async () => {
  if (!initialized.value || loading.value) return
  loading.value = true
  try {
    normalizeRoute()
    if (mihomoSniffUiTouched.value) {
      syncMihomoSnifferConfig()
    }
    syncMihomoNoResolveConfig()
    if (props.namespace === 'mihomo') {
      const errors = validateRouteForNamespace(appConfig.value.route, props.namespace, {
        outboundTags: outboundTags.value,
        inboundTags: inboundTags.value,
      })
      errors.push(...validateMihomoRouteResourceBounds(appConfig.value.route, inboundTags.value.length, appConfig.value))
      if (errors.length > 0) {
        showValidationErrors(errors)
        return
      }
    } else {
      const errors = rulesets.value.flatMap((item: any, index: number) => validateRulesetForNamespace(
        item,
        props.namespace,
        { outboundTags: outboundTags.value },
      ).map((message) => `Rule set #${index + 1}: ${message}`))
      errors.push(...validateSingboxRouteResourceBounds(appConfig.value.route))
      if (errors.length > 0) {
        showValidationErrors(errors)
        return
      }
    }
    if (props.namespace === 'mihomo') {
      const routeRequest: { expectedRevision: number, route: Record<string, unknown>, sniffer?: unknown } = {
        expectedRevision: mihomoRouteRevision.value,
        route: cloneConfig(appConfig.value.route ?? {}) as unknown as Record<string, unknown>,
      }
      if (mihomoSniffUiTouched.value) {
        routeRequest.sniffer = Object.hasOwn(appConfig.value as object, 'sniffer')
          ? cloneConfig((appConfig.value as any).sniffer)
          : null
      }
      ;(routeRequest as { retryRuntime?: boolean }).retryRuntime = runtimeRefreshFailed.value
      const result = await mihomoStore.saveRouteConfig(routeRequest)
      if (result.saved) {
        if (!await loadMihomoRouteEditorContext()) {
          await replaceDraft(mihomoStore.config)
        }
        dirty.value = false
        runtimeRefreshFailed.value = result.runtimeRefreshFailed
      } else if (result.conflict) {
        push.warning({
          title: '路由配置已变更',
          message: '其他页面或窗口已更新 Mihomo 配置，请重新加载路由页面后再保存。',
        })
      }
      return
    }
    const payload: Record<string, unknown> = {
      expectedRevision: singboxRouteRevision.value,
      retryRuntime: runtimeRefreshFailed.value,
    }
    if (!runtimeRefreshFailed.value || dirty.value) {
      payload.route = cloneConfig(appConfig.value.route)
    }
    const msg = await HttpUtils.post('api/singbox-route-save', payload, { headers: { 'Content-Type': 'application/json' } })
    const result = msg.success ? msg.obj : msg.obj?.result
    if (result?.route) {
      singboxRouteRevision.value = Number(result.revision ?? singboxRouteRevision.value)
      await replaceDraft({ route: result.route } as Config)
      dirty.value = false
      const retryFailed = msg.obj?.retryRuntime === true || (msg.obj?.committed === true && msg.success !== true)
      const retryRequested = runtimeRefreshFailed.value
      runtimeRefreshFailed.value = retryFailed
      if (retryFailed) singboxStore.setSingboxRuntimeRetryPending(true)
      else if (retryRequested || result.changed === true) singboxStore.setSingboxRuntimeRetryPending(false)
      return
    }
    if (msg.obj?.code === 'revision_conflict') {
      push.warning({
        title: '路由配置已变更',
        message: '其他页面或窗口已更新默认 sing-box 配置，请重新加载路由页面后再保存。',
        duration: 7000,
      })
    }
  } finally {
    loading.value = false
  }
}

const clients = computed((): string[] => {
  if (props.namespace !== 'mihomo') return singboxRouteClientNames.value
  return store.clients.map((c: any) => c.name)
})

const rules = computed((): any[] => {
  return Array.isArray(route.value?.rules) ? route.value.rules : []
})

const rulesets = computed((): any[] => {
  return Array.isArray(route.value?.rule_set) ? route.value.rule_set : []
})

const rulesetTags = computed((): any[] => {
  return rulesets.value.map((rs: any) => rs.tag)
})

const outboundTags = computed((): string[] => {
  if (props.namespace === 'mihomo') {
    return mihomoRouteTargets.value
  }
  return singboxRouteOutboundTags.value
})

const inboundTags = computed((): string[] => {
  if (props.namespace === 'mihomo') {
    return mihomoRouteInboundTags.value
  }
  return singboxRouteInboundTags.value
})

const rulePageCount = computed(() => Math.max(1, Math.ceil(rules.value.length / pageSize)))
const rulesetPageCount = computed(() => Math.max(1, Math.ceil(rulesets.value.length / pageSize)))
const paginatedRules = computed(() => {
  const page = Math.min(rulePage.value, rulePageCount.value)
  const offset = (page - 1) * pageSize
  return rules.value.slice(offset, offset + pageSize).map((item, index) => ({ item, index: offset + index }))
})
const paginatedRulesets = computed(() => {
  const page = Math.min(rulesetPage.value, rulesetPageCount.value)
  const offset = (page - 1) * pageSize
  return rulesets.value.slice(offset, offset + pageSize).map((item, index) => ({ item, index: offset + index }))
})
const ruleLimitReached = computed(() => props.namespace === 'mihomo'
  ? rules.value.length >= mihomoRouteResourceLimits.rules
  : rules.value.length >= singboxRouteResourceLimits.rules)
const rulesetLimitReached = computed(() => props.namespace === 'mihomo'
  ? rulesets.value.length >= mihomoRouteResourceLimits.ruleProviders
  : rulesets.value.length >= singboxRouteResourceLimits.ruleSets)

const ruleModal = ref({
  visible: false,
  index: -1,
  data: '',
})

const showRuleModal = (index: number) => {
  if (loading.value || !initialized.value) return
  if (index >= rules.value.length) return
  ruleModal.value.index = index
  ruleModal.value.data = index == -1 ? '' : JSON.stringify(sanitizeRuleForNamespace(rules.value[index], props.namespace) ?? {})
  ruleModal.value.visible = true
}

const closeRuleModal = () => {
  ruleModal.value.visible = false
}

const saveRuleModal = (data: any) => {
  const normalized = sanitizeRuleForNamespace(data, props.namespace)
  if (normalized == null) {
    if (props.namespace === 'mihomo') {
      showValidationErrors(['Mihomo rules only support simple route/reject entries.'])
    }
    return
  }
  if (props.namespace === 'mihomo') {
    const errors = validateRuleForNamespace(normalized, props.namespace, {
      outboundTags: outboundTags.value,
      ruleSetTags: rulesetTags.value,
      inboundTags: inboundTags.value,
    })
    const candidateRules = [...rules.value]
    if (ruleModal.value.index === -1) candidateRules.push(normalized)
    else candidateRules[ruleModal.value.index] = normalized
    errors.push(...validateMihomoRouteResourceBounds({
      ...route.value,
      rules: candidateRules,
    }, inboundTags.value.length, {
      ...appConfig.value,
      route: {
        ...route.value,
        rules: candidateRules,
      },
    }))
    if (errors.length > 0) {
      showValidationErrors(errors)
      return
    }
  } else {
    const candidateRules = [...rules.value]
    if (ruleModal.value.index === -1) candidateRules.push(normalized)
    else candidateRules[ruleModal.value.index] = normalized
    const errors = validateSingboxRouteResourceBounds({ ...route.value, rules: candidateRules })
    if (errors.length > 0) {
      showValidationErrors(errors)
      return
    }
  }
  if (ruleModal.value.index == -1) {
    rules.value.push(normalized)
    rulePage.value = rulePageCount.value
  } else {
    rules.value[ruleModal.value.index] = normalized
  }
  dirty.value = true
  ruleModal.value.visible = false
}

const moveRule = (index: number, offset: -1 | 1) => {
  if (editingDisabled.value) return
  const targetIndex = index + offset
  if (index < 0 || index >= rules.value.length || targetIndex < 0 || targetIndex >= rules.value.length) {
    return
  }
  const [item] = rules.value.splice(index, 1)
  rules.value.splice(targetIndex, 0, item)
  rulePage.value = Math.min(rulePage.value, rulePageCount.value)
  dirty.value = true
}

const delRule = (index: number) => {
  rules.value.splice(index, 1)
  rulePage.value = Math.min(rulePage.value, rulePageCount.value)
  dirty.value = true
}

const rulesetModal = ref({
  visible: false,
  index: -1,
  data: '',
})

const showRulesetModal = (index: number) => {
  if (loading.value || !initialized.value) return
  if (index >= rulesets.value.length) return
  rulesetModal.value.index = index
  rulesetModal.value.data = index == -1 ? '' : JSON.stringify(rulesets.value[index])
  rulesetModal.value.visible = true
}

const closeRulesetModal = () => {
  rulesetModal.value.visible = false
}

const saveRulesetModal = (data: ruleset) => {
  const errors = validateRulesetModalData(data)
  if (errors.length > 0) {
    showValidationErrors(errors)
    return
  }
  if (props.namespace !== 'mihomo') {
    const candidateRulesets = [...rulesets.value]
    if (rulesetModal.value.index === -1) candidateRulesets.push(data)
    else candidateRulesets[rulesetModal.value.index] = data
    const resourceErrors = validateSingboxRouteResourceBounds({ ...route.value, rule_set: candidateRulesets })
    if (resourceErrors.length > 0) {
      showValidationErrors(resourceErrors)
      return
    }
  }
  if (rulesetModal.value.index == -1) {
    rulesets.value.push(data)
    rulesetPage.value = rulesetPageCount.value
  } else {
    rulesets.value[rulesetModal.value.index] = data
  }
  dirty.value = true
  rulesetModal.value.visible = false
}

const delRuleset = (index: number) => {
  rulesets.value.splice(index, 1)
  rulesetPage.value = Math.min(rulesetPage.value, rulesetPageCount.value)
  dirty.value = true
}

const getRuleCardKey = (item: any, index: number): string => {
  if (item && typeof item === 'object') {
    const existing = ruleCardKeys.get(item)
    if (existing) return existing
    const key = `rule:${typeof globalThis.crypto?.randomUUID === 'function' ? globalThis.crypto.randomUUID() : `${Date.now()}:${index}:${Math.random()}`}`
    ruleCardKeys.set(item, key)
    return key
  }
  return `rule:${index}`
}

const getRulesetCardKey = (item: any, index: number): string => {
  if (item && typeof item === 'object') {
    const existing = rulesetCardKeys.get(item)
    if (existing) return existing
    const key = `ruleset:${typeof globalThis.crypto?.randomUUID === 'function' ? globalThis.crypto.randomUUID() : `${Date.now()}:${index}:${Math.random()}`}`
    rulesetCardKeys.set(item, key)
    return key
  }
  return `ruleset:${index}`
}

const requestDelete = (kind: 'rule' | 'ruleset', index: number) => {
  if (loading.value || !initialized.value) return
  const target = kind === 'rule' ? rules.value[index] : rulesets.value[index]
  if (target == null) return
  deleteRequest.value = { kind, target }
  deleteConfirmVisible.value = true
}

const confirmDelete = () => {
  const request = deleteRequest.value
  deleteConfirmVisible.value = false
  deleteRequest.value = null
  if (!request) return
  const list = request.kind === 'rule' ? rules.value : rulesets.value
  const index = list.indexOf(request.target)
  if (index < 0) {
    showValidationErrors(['The selected item is no longer present. Reload the page before trying again.'])
    return
  }
  if (request.kind === 'rule') delRule(index)
  else delRuleset(index)
}

const draggedItemIndex = ref<number | null>(null)

const onDragStart = (index: number) => {
  draggedItemIndex.value = index
}

const onDrop = (index: number) => {
  if (editingDisabled.value) {
    draggedItemIndex.value = null
    return
  }
  const fromIndex = draggedItemIndex.value
  if (fromIndex !== null && fromIndex >= 0 && fromIndex < rules.value.length && index >= 0 && index < rules.value.length && fromIndex !== index) {
    const draggedItem = rules.value[fromIndex]
    rules.value.splice(fromIndex, 1)
    const targetIndex = fromIndex < index ? index - 1 : index
    rules.value.splice(Math.max(0, Math.min(targetIndex, rules.value.length)), 0, draggedItem)
    draggedItemIndex.value = null
    dirty.value = true
  }
}

const clearDraggedItem = () => {
  draggedItemIndex.value = null
}
</script>
