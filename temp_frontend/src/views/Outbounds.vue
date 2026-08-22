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
    <OutboundGroup
      v-if="allowOutboundGroups"
      v-model="groupModal.visible"
      :visible="groupModal.visible"
      :namespace="props.namespace"
      @close="closeGroupModal"
    />
  <OutboundVue
    v-model="modal.visible"
    :visible="modal.visible"
    :id="modal.id"
    :namespace="props.namespace"
    :data="modal.data"
    :tags="outboundTags"
    @close="closeModal"
  />
  <Stats
    v-if="props.namespace !== 'mihomo'"
    v-model="stats.visible"
    :visible="stats.visible"
    :resource="stats.resource"
    :tag="stats.tag"
    :namespace="props.namespace"
    @close="closeStats"
  />
  <v-dialog v-model="deleteDialog.visible" max-width="400" max-height="90vh" :persistent="deleteDialog.saving">
    <v-card class="rounded-lg">
      <v-card-title>{{ $t('actions.del') }}</v-card-title>
      <v-divider></v-divider>
      <v-card-text class="outbound-delete-copy">
        <template v-if="props.namespace === 'mihomo'">
          删除“{{ deleteDialog.tag }}”前会检查代理组、路由和入站引用；存在引用时不会删除。
        </template>
        <template v-else>
          删除“{{ deleteDialog.tag }}”会从出站分组中移除该节点；若仍被路由、其它出站或其它分组引用，删除会被拒绝。
        </template>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="success" variant="outlined" :disabled="deleteDialog.saving" @click="deleteDialog.visible = false">{{ $t('no') }}</v-btn>
        <v-btn color="error" variant="outlined" :loading="deleteDialog.saving" :disabled="deleteDialog.saving" @click="delOutbound">{{ $t('yes') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
    <v-row>
      <v-col cols="12" justify="center" align="center">
        <v-btn color="primary" :disabled="outboundWriteBusy" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
        <v-btn v-if="allowOutboundGroups" color="primary" class="ml-2" :disabled="outboundWriteBusy" @click="showGroupModal">{{ $t('actions.group') }}</v-btn>
      </v-col>
    </v-row>
    <v-alert v-if="outbounds.length === 0" type="info" variant="tonal" class="mb-4">
      暂无出站节点
    </v-alert>
    <v-row v-else>
      <v-col cols="12" sm="4" md="3" lg="2" v-for="item in pagedOutbounds" :key="item.tag">
      <v-card rounded="xl" elevation="5" min-width="200" :title="item.tag">
        <v-card-subtitle style="margin-top: -20px;">
          <v-row>
            <v-col>{{ item.type }}</v-col>
          </v-row>
        </v-card-subtitle>
        <v-card-text>
          <v-row>
            <v-col>{{ $t('in.addr') }}</v-col>
            <v-col>
              {{ item.server ?? '-' }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('in.port') }}</v-col>
            <v-col>
              {{ formatServerPort(item) }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('objects.tls') }}</v-col>
            <v-col>
              {{ Object.hasOwn(item, 'tls') ? $t(item.tls?.enabled ? 'enable' : 'disable') : '-' }}
            </v-col>
          </v-row>
          <v-row v-if="props.namespace !== 'mihomo'">
            <v-col>{{ $t('online') }}</v-col>
            <v-col>
              <template v-if="onlineTags.has(item.tag)">
                <v-chip density="comfortable" size="small" color="success" variant="flat">{{ $t('online') }}</v-chip>
              </template>
              <template v-else>-</template>
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" :disabled="outboundWriteBusy" @click="showModal(item.id)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" style="margin-inline-start: 0;" color="warning" :disabled="outboundWriteBusy" @click="showDeleteDialog(item.tag)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-chart-line" :disabled="outboundWriteBusy" @click="showStats(item.tag)" v-if="enableTraffic && props.namespace !== 'mihomo'">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('stats.graphTitle')"></v-tooltip>
          </v-btn>
        </v-card-actions>
      </v-card>
      </v-col>
    </v-row>
    <v-row v-if="pageCount > 1" class="mt-2" align="center" justify="center">
    <v-col cols="12" sm="auto">
      <v-select
        v-model="itemsPerPage"
        :items="pageSizeOptions"
        label="每页节点数"
        density="compact"
        hide-details
        style="min-width: 148px"
      />
    </v-col>
    <v-col cols="12" sm="auto">
      <v-pagination v-model="page" :length="pageCount" :total-visible="5" density="comfortable" />
    </v-col>
    </v-row>
  </template>
</template>

<script lang="ts" setup>
import OutboundVue from '@/layouts/modals/Outbound.vue'
import OutboundGroup from '@/layouts/modals/OutboundGroup.vue'
import { Outbound } from '@/types/outbounds'
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, watch } from 'vue'
import { formatServerPortDisplay } from '@/plugins/portRange'
import { getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const store = getNamespaceStore(props.namespace)
const allowOutboundGroups = true
const Stats = defineAsyncComponent(() => import('@/layouts/modals/Stats.vue'))
const initializing = ref(true)
const loadFailed = ref(false)
let componentActive = true

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

const outbounds = computed((): Outbound[] => {
  return <Outbound[]>store.outbounds
})

const outboundTags = computed((): string[] => {
  const tags = store.outbounds
    ?.map((outbound: Outbound) => outbound.tag)
    .filter((tag: unknown): tag is string => typeof tag === 'string' && tag.trim().length > 0) ?? []
  if (props.namespace === 'mihomo') return tags
  const endpointTags = store.endpoints
    ?.map((endpoint: any) => endpoint.tag)
    .filter((tag: unknown): tag is string => typeof tag === 'string' && tag.trim().length > 0) ?? []
  return [...new Set([...tags, ...endpointTags])]
})

const onlines = computed(() => {
  return store.onlines.outbound ?? []
})

const onlineTags = computed(() => new Set(onlines.value))

const enableTraffic = computed(() => {
  return store.enableTraffic
})

const modal = ref({
  visible: false,
  id: 0,
  data: '',
})

const groupModal = ref({
  visible: false,
})

const deleteDialog = ref({
  visible: false,
  tag: '',
  saving: false,
})

const outboundWriteBusy = computed(() => deleteDialog.value.saving)

const showModal = (id: number) => {
  if (outboundWriteBusy.value) return
  modal.value.id = id
  modal.value.data = id == 0 ? '' : JSON.stringify(outbounds.value.findLast(o => o.id == id))
  modal.value.visible = true
}

const closeModal = () => {
  modal.value.visible = false
}

const showGroupModal = () => {
  if (outboundWriteBusy.value) return
  groupModal.value.visible = true
}

const closeGroupModal = () => {
  groupModal.value.visible = false
}

const formatServerPort = (item: any): string => {
  return formatServerPortDisplay(item?.server_port, item?.server_ports)
}

const stats = ref({
  visible: false,
  resource: 'outbound',
  tag: '',
})

const page = ref(1)
const itemsPerPage = ref(48)
const pageSizeOptions = [24, 48, 96]
const pageCount = computed(() => Math.max(1, Math.ceil(outbounds.value.length / itemsPerPage.value)))
const pagedOutbounds = computed(() => {
  const start = (page.value - 1) * itemsPerPage.value
  return outbounds.value.slice(start, start + itemsPerPage.value)
})

watch([outbounds, itemsPerPage], () => {
  if (page.value > pageCount.value) page.value = pageCount.value
})

const showDeleteDialog = (tag: string) => {
  if (outboundWriteBusy.value) return
  deleteDialog.value.tag = tag
  deleteDialog.value.visible = true
}

const delOutbound = async () => {
  const tag = deleteDialog.value.tag
  if (!tag || outboundWriteBusy.value) return
  deleteDialog.value.saving = true
  try {
    const success = await store.save('outbounds', 'del', tag)
    if (success) {
      deleteDialog.value.visible = false
      deleteDialog.value.tag = ''
    }
  } finally {
    deleteDialog.value.saving = false
  }
}

const showStats = (tag: string) => {
  if (outboundWriteBusy.value) return
  stats.value.tag = tag
  stats.value.visible = true
}

const closeStats = () => {
  stats.value.visible = false
}

onMounted(() => {
  void initialize()
})

onUnmounted(() => {
  componentActive = false
})
</script>

<style scoped>
.outbound-delete-copy {
  overflow-wrap: anywhere;
}
</style>
