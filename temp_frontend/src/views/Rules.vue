<template>
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
      <v-btn color="primary" @click="showRuleModal(-1)" style="margin: 0 5px;">{{ $t('rule.add') }}</v-btn>
      <v-btn color="primary" @click="showRulesetModal(-1)" style="margin: 0 5px;">{{ $t('ruleset.add') }}</v-btn>
      <v-btn variant="outlined" color="warning" @click="saveConfig" :loading="loading" :disabled="stateChange">
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
            @click:clear="delete route.final"
            :items="outboundTags"
            v-model="route.final">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-text-field
            v-model="route.default_interface"
            hide-details
            clearable
            @click:clear="delete route.default_interface"
            :label="$t('basic.routing.defaultIf')">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-text-field
            v-model.number="routeMark"
            hide-details
            type="number"
            min="0"
            :label="$t('basic.routing.defaultRm')">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-switch
            v-model="route.auto_detect_interface"
            color="primary"
            :label="$t('basic.routing.autoBind')"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="props.namespace === 'mihomo'">
          <v-switch
            v-model="mihomoSniffUi"
            color="primary"
            label="sniff"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="props.namespace === 'mihomo'">
          <v-switch
            v-model="mihomoNoResolveUi"
            color="primary"
            label="no-resolve_全局开关"
            hide-details>
          </v-switch>
        </v-col>
      </v-row>
    </v-col>
  </v-row>
  <v-row>
    <v-col class="v-card-subtitle" cols="12">{{ $t('rule.ruleset') }}</v-col>
    <v-col cols="12" sm="4" md="3" lg="2" v-for="(item, index) in <any[]>rulesets" :key="item.tag">
        <v-card rounded="xl" elevation="5" min-width="200" :title="item.tag">
        <v-card-subtitle style="margin-top: -20px;">
          <v-row>
            <v-col>{{ props.namespace === 'mihomo' ? item.type : $t('ruleset.' + item.type) }}</v-col>
          </v-row>
        </v-card-subtitle>
        <v-card-text>
          <v-row v-if="props.namespace === 'mihomo'">
            <v-col>behavior</v-col>
            <v-col>{{ item.behavior ?? 'classical' }}</v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('ruleset.format') }}</v-col>
            <v-col>{{ item.format }}</v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('actions.update') }}</v-col>
            <v-col>{{ item.update_interval ?? '-' }}</v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" @click="showRulesetModal(index)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" style="margin-inline-start: 0;" color="warning" @click="delRulesetOverlay[index] = true">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
          <v-overlay
            v-model="delRulesetOverlay[index]"
            contained
            class="align-center justify-center">
            <v-card :title="$t('actions.del')" rounded="lg">
              <v-divider></v-divider>
              <v-card-text>{{ $t('confirm') }}</v-card-text>
              <v-card-actions>
                <v-btn color="error" variant="outlined" @click="delRuleset(index)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" @click="delRulesetOverlay[index] = false">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
        </v-card-actions>
      </v-card>
    </v-col>
  </v-row>
  <v-row>
    <v-col class="v-card-subtitle" cols="12">{{ $t('pages.rules') }}</v-col>
    <v-col
      cols="12"
      sm="4"
      md="3"
      lg="2"
      v-for="(item, index) in <any[]>rules"
      :key="getRuleCardKey(item, index)"
      :draggable="true"
      @dragstart="onDragStart(index)"
      @dragover.prevent
      @drop="onDrop(index)">
      <v-card rounded="xl" elevation="5" min-width="200" :title="index + 1">
        <v-card-subtitle style="margin-top: -20px;">
          <v-row>
            <v-col>{{ item.type != undefined ? $t('rule.logical') + ' (' + item.mode + ')' : $t('rule.simple') }}</v-col>
          </v-row>
        </v-card-subtitle>
        <v-card-text>
          <v-row>
            <v-col>{{ $t('admin.action') }}</v-col>
            <v-col>{{ item.action }}</v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('objects.outbound') }}</v-col>
            <v-col>{{ item.outbound ?? '-' }}</v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('pages.rules') }}</v-col>
            <v-col>{{ item.rules ? item.rules.length : Object.keys(item).filter(r => !actionKeys.includes(r)).length }}</v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('rule.invert') }}</v-col>
            <v-col>{{ $t((item.invert ?? false) ? 'yes' : 'no') }}</v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" @click="showRuleModal(index)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" style="margin-inline-start: 0;" color="warning" @click="delRuleOverlay[index] = true">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
          <v-overlay
            v-model="delRuleOverlay[index]"
            contained
            class="align-center justify-center">
            <v-card :title="$t('actions.del')" rounded="lg">
              <v-divider></v-divider>
              <v-card-text>{{ $t('confirm') }}</v-card-text>
              <v-card-actions>
                <v-btn color="error" variant="outlined" @click="delRule(index)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" @click="delRuleOverlay[index] = false">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
        </v-card-actions>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import { computed, ref, onMounted, watch } from 'vue'
import { push } from 'notivue'
import { i18n } from '@/locales'
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
  getMihomoBuiltInTargets,
} from '@/types/rules'
import { FindDiff } from '@/plugins/utils'
import { getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const store = getNamespaceStore(props.namespace)
const oldConfig = ref(<Config>{})
const appConfig = ref<Config>({} as Config)
const loading = ref(false)
const initialized = ref(false)
const mihomoSniffUi = ref(false)
const mihomoNoResolveUi = ref(true)

const cloneConfig = (value: any): Config => {
  return JSON.parse(JSON.stringify(value ?? {}))
}

const normalizeEditableConfig = (value: Config): Config => {
  const nextConfig = cloneConfig(value)
  if (!nextConfig.route || typeof nextConfig.route !== 'object') {
    nextConfig.route = {
      rules: [],
      rule_set: [],
      default_domain_resolver: '',
    }
  }
  nextConfig.route = sanitizeRouteForNamespace(nextConfig.route, props.namespace)
  return nextConfig
}

const normalizeRoute = () => {
  if (!appConfig.value) {
    return
  }
  appConfig.value = normalizeEditableConfig(appConfig.value)
}

const syncMihomoSniffUiFromConfig = () => {
  if (props.namespace !== 'mihomo') {
    return
  }
  const sniffer = (<any>appConfig.value)?.sniffer
  if (sniffer && typeof sniffer === 'object') {
    mihomoSniffUi.value = sniffer.enable !== false
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

const route = computed((): any => {
  return appConfig.value.route ?? {}
})

onMounted(async () => {
  const nextConfig = normalizeEditableConfig(store.config)
  appConfig.value = nextConfig
  oldConfig.value = cloneConfig(nextConfig)
  initialized.value = true
  syncMihomoSniffUiFromConfig()
  syncMihomoNoResolveUiFromConfig()
})

watch(mihomoSniffUi, () => {
  syncMihomoSnifferConfig()
})

watch(mihomoNoResolveUi, () => {
  syncMihomoNoResolveConfig()
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

const routeMark = computed({
  get() {
    return route.value.default_mark ?? 0
  },
  set(v: number) {
    v > 0 ? route.value.default_mark = v : delete appConfig.value.route.default_mark
  },
})

const stateChange = computed(() => {
  return FindDiff.deepCompare(appConfig.value, oldConfig.value)
})

watch(
  () => store.config,
  (config) => {
    if (!initialized.value) {
      return
    }
    if (!stateChange.value) {
      return
    }
    const nextConfig = normalizeEditableConfig(config)
    appConfig.value = nextConfig
    oldConfig.value = cloneConfig(nextConfig)
    syncMihomoSniffUiFromConfig()
    syncMihomoNoResolveUiFromConfig()
  },
  { deep: true },
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
  const errors = validateRulesetForNamespace(data, props.namespace)
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
  loading.value = true
  normalizeRoute()
  syncMihomoSnifferConfig()
  syncMihomoNoResolveConfig()
  if (props.namespace === 'mihomo') {
    const errors = validateRouteForNamespace(appConfig.value.route, props.namespace, {
      outboundTags: outboundTags.value,
    })
    if (errors.length > 0) {
      loading.value = false
      showValidationErrors(errors)
      return
    }
  }
  const success = await store.save('config', 'set', appConfig.value)
  if (success) {
    const nextConfig = normalizeEditableConfig(store.config)
    appConfig.value = nextConfig
    oldConfig.value = cloneConfig(nextConfig)
    syncMihomoSniffUiFromConfig()
    syncMihomoNoResolveUiFromConfig()
  }
  loading.value = false
}

const clients = computed((): string[] => {
  return store.clients.map((c: any) => c.name)
})

const rules = computed((): any[] => {
  const data = route.value
  if (!data) {
    return []
  }
  if (!('rules' in data) || !Array.isArray(data.rules)) {
    data.rules = []
  }
  return data.rules
})

const rulesets = computed((): any[] => {
  const data = route.value
  if (!data) {
    return []
  }
  if (!('rule_set' in data) || !Array.isArray(data.rule_set)) {
    data.rule_set = []
  }
  return data.rule_set
})

const rulesetTags = computed((): any[] => {
  return rulesets.value.map((rs: any) => rs.tag)
})

const outboundTags = computed((): string[] => {
  const outbounds = [...(store.outbounds?.map((o: any) => o.tag) ?? [])]
  if (props.namespace === 'mihomo') {
    return Array.from(new Set([...getMihomoBuiltInTargets(), ...outbounds]))
  }
  return [...outbounds, ...(store.endpoints?.map((e: any) => e.tag) ?? [])]
})

const inboundTags = computed((): string[] => {
  const tags = [
    ...store.inbounds?.map((o: any) => o.route_tag ?? o.tag),
    ...store.endpoints?.filter((e: any) => e.listen_port > 0).map((e: any) => e.route_tag ?? e.tag),
  ]
  return Array.from(new Set(tags.filter((tag: any) => typeof tag === 'string' && tag.length > 0)))
})

const delRuleOverlay = ref(new Array<boolean>())
const delRulesetOverlay = ref(new Array<boolean>())

const ruleModal = ref({
  visible: false,
  index: -1,
  data: '',
})

const showRuleModal = (index: number) => {
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
    })
    if (errors.length > 0) {
      showValidationErrors(errors)
      return
    }
  }
  if (ruleModal.value.index == -1) {
    rules.value.push(normalized)
  } else {
    rules.value[ruleModal.value.index] = normalized
  }
  ruleModal.value.visible = false
}

const delRule = (index: number) => {
  rules.value.splice(index, 1)
  delRuleOverlay.value[index] = false
}

const rulesetModal = ref({
  visible: false,
  index: -1,
  data: '',
})

const showRulesetModal = (index: number) => {
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
  if (rulesetModal.value.index == -1) {
    rulesets.value.push(data)
  } else {
    rulesets.value[rulesetModal.value.index] = data
  }
  rulesetModal.value.visible = false
}

const delRuleset = (index: number) => {
  rulesets.value.splice(index, 1)
  delRulesetOverlay.value[index] = false
}

const getRuleCardKey = (item: any, index: number): string => {
  if (item?.id != null) {
    return `id:${item.id}`
  }
  return `idx:${index}:${JSON.stringify(item)}`
}

const draggedItemIndex = ref<number | null>(null)

const onDragStart = (index: number) => {
  draggedItemIndex.value = index
}

const onDrop = (index: number) => {
  if (draggedItemIndex.value !== null) {
    const draggedItem = rules.value[draggedItemIndex.value]
    rules.value.splice(draggedItemIndex.value, 1)
    rules.value.splice(index, 0, draggedItem)
    draggedItemIndex.value = null
  }
}
</script>
