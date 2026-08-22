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
    <TlsVue
      v-model="modal.visible"
      :visible="modal.visible"
      :id="modal.id"
      :namespace="props.namespace"
      :data="modal.data"
      :saving="saving"
      @close="closeModal"
      @save="saveModal"
    />
    <v-row>
      <v-col cols="12" justify="center" align="center">
        <v-btn color="primary" :disabled="saving" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="4" md="3" lg="2" class="d-flex" v-for="item in <any[]>tlsConfigs" :key="item.id">
        <v-card rounded="lg" elevation="5" class="tls-card d-flex flex-column w-100">
          <v-card-item class="tls-card__header">
            <v-card-title class="tls-card__title" :title="item.name || '-'">{{ item.name || '-' }}</v-card-title>
            <v-card-subtitle class="tls-card__target" :title="tlsTarget(item)">{{ tlsTarget(item) }}</v-card-subtitle>
          </v-card-item>
          <v-card-text class="tls-card__body">
            <div class="tls-card__field">
              <span>引用入站</span>
              <span class="tls-card__value">
              <template v-if="tlsInboundTags[item.id]?.length > 0">
                <v-tooltip activator="parent" dir="ltr" location="bottom">
                  <span v-for="i in tlsInboundTags[item.id]" :key="i">{{ i }}<br /></span>
                </v-tooltip>
                {{ tlsInboundTags[item.id].length }}
              </template>
              <template v-else>-</template>
              </span>
            </div>
            <div class="tls-card__field">
              <span>引用服务</span>
              <span class="tls-card__value">
              <template v-if="tlsServiceTags[item.id]?.length > 0">
                <v-tooltip activator="parent" dir="ltr" location="bottom">
                  <span v-for="serviceTag in tlsServiceTags[item.id]" :key="serviceTag">{{ serviceTag }}<br /></span>
                </v-tooltip>
                {{ tlsServiceTags[item.id].length }}
              </template>
              <template v-else>-</template>
              </span>
            </div>
            <div class="tls-card__field"><span>模式</span><span class="tls-card__value">{{ modeLabel(item) }}</span></div>
            <div v-if="modeLabel(item) === 'TLS'" class="tls-card__field"><span>ECH</span><span class="tls-card__value">{{ $t(item.server?.ech == undefined ? 'no' : 'yes') }}</span></div>
          </v-card-text>
          <v-divider></v-divider>
          <v-card-actions class="tls-card__actions">
            <v-btn icon="mdi-file-edit" :disabled="saving" @click="showModal(item.id)">
              <v-icon />
              <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
            </v-btn>
            <v-btn v-if="canDeleteTls(item.id)" icon="mdi-file-remove" style="margin-inline-start: 0;" color="warning" :disabled="saving" @click="delOverlay[item.id] = true">
              <v-icon />
              <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
            </v-btn>
            <v-overlay
              v-model="delOverlay[item.id]"
              contained
              class="align-center justify-center"
              :persistent="saving">
              <v-card :title="$t('actions.del')" rounded="lg">
                <v-divider></v-divider>
                <v-card-text>{{ $t('confirm') }}</v-card-text>
                <v-card-actions>
                  <v-btn color="error" variant="outlined" :loading="saving" :disabled="saving" @click="delTls(item.id)">{{ $t('yes') }}</v-btn>
                  <v-btn color="success" variant="outlined" :disabled="saving" @click="delete delOverlay[item.id]">{{ $t('no') }}</v-btn>
                </v-card-actions>
              </v-card>
            </v-overlay>
            <v-btn icon="mdi-content-duplicate" :disabled="saving" @click="clone(item)">
              <v-icon />
              <v-tooltip activator="parent" location="top" :text="$t('actions.clone')"></v-tooltip>
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>
  </template>
</template>

<script lang="ts" setup>
import TlsVue from '@/layouts/modals/Tls.vue'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { Inbound } from '@/types/inbounds'
import { tls, sanitizeTlsForNamespace } from '@/types/tls'
import { getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const store = getNamespaceStore(props.namespace)
const initializing = ref(true)
const loadFailed = ref(false)
let componentActive = true

const tlsConfigs = computed((): any[] => {
  return store.tlsConfigs
})

const inbounds = computed((): Inbound[] => {
  return store.inbounds
})

const services = computed((): any[] => {
  return store.services
})

const tlsInboundTags = computed<Record<number, string[]>>(() => {
  const tags: Record<number, string[]> = {}
  for (const inbound of inbounds.value) {
    const tlsID = Number(inbound.tls_id ?? 0)
    if (!Number.isFinite(tlsID) || tlsID <= 0) continue
    if (!tags[tlsID]) tags[tlsID] = []
    tags[tlsID].push(inbound.tag)
  }
  return tags
})

const tlsServiceTags = computed<Record<number, string[]>>(() => {
  const tags: Record<number, string[]> = {}
  for (const service of services.value) {
    const tlsID = Number(service?.tls_id ?? 0)
    if (!Number.isFinite(tlsID) || tlsID <= 0) continue
    if (!tags[tlsID]) tags[tlsID] = []
    const tag = typeof service?.tag === 'string' && service.tag.trim() !== ''
      ? service.tag
      : `#${service?.id ?? ''}`
    tags[tlsID].push(tag)
  }
  return tags
})

const canDeleteTls = (id: number): boolean => {
  return (tlsInboundTags.value[id]?.length ?? 0) === 0
    && (tlsServiceTags.value[id]?.length ?? 0) === 0
}

const modal = ref({
  visible: false,
  id: 0,
  data: '',
})

const delOverlay = ref<Record<number, boolean>>({})
const saving = ref(false)

const normalizeTls = (data?: tls | null): tls => {
  return sanitizeTlsForNamespace(data, props.namespace)
}

const modeOf = (item: any): string => {
  if (typeof item?.mode === 'string' && item.mode) return item.mode
  return item?.server?.reality && typeof item.server.reality === 'object' ? 'reality' : 'tls'
}

const modeLabel = (item: any): string => ({ tls: 'TLS', reality: 'Reality', 'shadow-tls': 'ShadowTLS', restls: 'Restls', jls: 'JLS' } as Record<string, string>)[modeOf(item)] ?? 'TLS'

const tlsTarget = (item: any): string => {
  const mode = modeOf(item)
  const shadow = item?.server?.shadow_tls
  const mappedShadowTarget = shadow?.handshake_for_server_name && typeof shadow.handshake_for_server_name === 'object'
    ? Object.values(shadow.handshake_for_server_name).map((entry: any) => entry?.dest).find((dest: any) => typeof dest === 'string' && dest.trim() !== '')
    : ''
  const target = shadow?.handshake?.dest || mappedShadowTarget || item?.server?.res_tls?.dest || item?.server?.jls_config?.dest || item?.server?.server_name || item?.server?.reality?.handshake?.server || item?.server?.jls_config?.sni
  return `${modeLabel(item)}${target ? ` · ${target}` : ''}`
}

const initialize = async () => {
  const hadLoadedData = store.hasFullData
  initializing.value = true
  loadFailed.value = false
  try {
    const success = await store.loadData()
    if (!componentActive) return
    if (!success && !hadLoadedData) {
      loadFailed.value = true
    }
  } catch {
    if (componentActive && !hadLoadedData) loadFailed.value = true
  } finally {
    if (componentActive) initializing.value = false
  }
}

onMounted(() => {
  void initialize()
})

onUnmounted(() => {
  componentActive = false
})

const showModal = (id: number) => {
  if (saving.value) return
  const existing = id === 0 ? null : tlsConfigs.value.findLast(t => t.id == id)
  if (id !== 0 && !existing) return
  modal.value.id = id
  modal.value.data = id == 0 ? '{}' : JSON.stringify(normalizeTls(existing))
  modal.value.visible = true
}

const clone = (obj: any) => {
  if (saving.value) return
  const data = normalizeTls(obj)
  data.id = 0
  let copyIndex = 1
  const baseName = data.name || 'tls'
  data.name = baseName
  while (tlsConfigs.value.some(t => t.name === data.name)) {
    data.name = `${baseName}-copy${copyIndex > 1 ? `-${copyIndex}` : ''}`
    copyIndex++
  }
  saveModal(data)
}

const closeModal = () => {
  if (saving.value) return
  modal.value.visible = false
}

const saveModal = async (data: tls) => {
  if (saving.value) return
  saving.value = true
  const normalized = normalizeTls(data)
  try {
    const success = await store.save('tls', normalized.id > 0 ? 'edit' : 'new', normalized)
    if (success) modal.value.visible = false
  } finally {
    saving.value = false
  }
}

const delTls = async (id: number) => {
  if (saving.value) return
  if (!canDeleteTls(id)) {
    delete delOverlay.value[id]
    return
  }
  saving.value = true
  try {
    const success = await store.save('tls', 'del', id)
    if (success) delete delOverlay.value[id]
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
.tls-card {
  height: 264px;
  min-width: 0;
}

.tls-card__header {
  flex: 0 0 68px;
  min-width: 0;
  padding-block: 12px 8px;
}

.tls-card__title,
.tls-card__target,
.tls-card__value {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tls-card__body {
  flex: 1 1 auto;
  min-height: 0;
  padding-block: 8px;
}

.tls-card__field {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  column-gap: 16px;
  align-items: center;
  min-height: 30px;
}

.tls-card__actions {
  flex: 0 0 48px;
  padding: 0;
}
</style>
