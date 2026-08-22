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
    <ClientModal
      v-model="modal.visible"
      :visible="modal.visible"
      :id="modal.id"
      :namespace="props.namespace"
      :groups="groups"
      :inboundTags="inboundTags"
      @close="closeModal"
    />
  <ClientBulk
    v-model="addBulkModal"
    :visible="addBulkModal"
    :namespace="props.namespace"
    :groups="groups"
    :inboundTags="inboundTags"
    @close="closeBulk"
  />
  <QrCode
    v-model="qrcode.visible"
    :visible="qrcode.visible"
    :id="qrcode.id"
    :namespace="props.namespace"
    @close="closeQrCode"
  />
  <Stats
    v-model="stats.visible"
    :visible="stats.visible"
    :resource="stats.resource"
    :tag="stats.tag"
    :namespace="props.namespace"
    @close="closeStats"
  />
    <v-row justify="center" align="center">
    <v-col cols="auto">
      <v-btn color="primary" :disabled="clientOperationBusy" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
    </v-col>
    <v-col cols="auto">
      <v-menu v-model="actionMenu" :disabled="clientOperationBusy" :close-on-content-click="false" location="bottom center">
        <template v-slot:activator="{ props: menuProps }">
          <v-btn v-bind="menuProps" hide-details variant="text" icon :disabled="clientOperationBusy">
            <v-icon icon="mdi-tools" color="primary" />
          </v-btn>
        </template>
        <v-list density="compact" nav>
          <v-list-item link :disabled="clientOperationBusy" @click="addBulk">
            <template v-slot:prepend>
              <v-icon icon="mdi-account-multiple-plus"></v-icon>
            </template>
            <v-list-item-title v-text="$t('actions.addbulk')"></v-list-item-title>
          </v-list-item>
        </v-list>
      </v-menu>
    </v-col>
    <v-col cols="auto">
      <v-menu v-model="filterMenu" :close-on-content-click="false" location="bottom center">
        <template v-slot:activator="{ props: menuProps }">
          <v-btn v-bind="menuProps" hide-details variant="text" icon>
            <v-icon :icon="filterSettings.enabled ? 'mdi-filter-check-outline' : 'mdi-filter-menu-outline'" :color="filterSettings.enabled ? 'primary' : ''" />
          </v-btn>
        </template>
        <v-card>
          <v-container>
            <v-row>
              <v-col>
                <v-select
                  variant="underlined"
                  density="compact"
                  :label="$t('type')"
                  :items="filterItems"
                  v-model="filterSettings.state">
                </v-select>
              </v-col>
            </v-row>
            <v-row>
              <v-col>
                <v-select
                  variant="underlined"
                  density="compact"
                  :label="$t('client.group')"
                  :items="[{ title: $t('all'), value: '-' }, ...groups.map(g => ({ title: g.length > 0 ? g : $t('none'), value: g }))]"
                  v-model="filterSettings.group">
                </v-select>
              </v-col>
            </v-row>
            <v-row>
              <v-col>
                <v-text-field
                  variant="underlined"
                  density="compact"
                  :label="$t('client.name')"
                  v-model="filterSettings.text">
                </v-text-field>
              </v-col>
            </v-row>
          </v-container>
          <v-card-actions>
            <v-spacer></v-spacer>
            <v-btn
              color="blue-darken-1"
              variant="outlined"
              @click="clearFilter">
              {{ $t('actions.del') }}
            </v-btn>
            <v-btn
              color="blue-darken-1"
              variant="tonal"
              @click="doFilter">
              {{ $t('actions.update') }}
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-menu>
    </v-col>
  </v-row>
    <v-row>
      <v-col cols="12">
      <v-data-table
        :headers="headers"
        :items="visibleClients"
        :hide-default-footer="hideDefaultFooter"
        :items-per-page="itemPerPage"
        @update:items-per-page="setItemPerPage($event)"
        hide-no-data
        fixed-header
        item-value="id"
        :mobile="smAndDown"
        mobile-breakpoint="sm"
        width="100%"
        class="clients-table elevation-3 rounded">
        <template v-slot:item.inbounds="{ item }">
          <span>
            <v-tooltip activator="parent" dir="ltr" location="start" v-if="clientInboundIds(item).length > 0">
              <span v-for="i in clientInboundIds(item)" :key="i">{{ inboundTagMap.get(Number(i)) ?? i }}<br /></span>
            </v-tooltip>
            {{ clientInboundIds(item).length }}
          </span>
        </template>
        <template v-slot:item.volume="{ item }">
          <div class="text-start" v-tooltip:top="$t('stats.usage') + ': ' + HumanReadable.sizeFormat(item.up + item.down)">
            <v-chip
              size="small"
              :color="item.volume > 0 && item.volume <= (item.up + item.down) ? 'error' : ''"
              label>
              {{ item.volume == 0 ? $t('unlimited') : HumanReadable.sizeFormat(item.volume) }}
            </v-chip>
          </div>
          <v-progress-linear
            :model-value="percent(item)"
            :color="percentColor(item)"
            v-if="item.volume > 0"
            bottom>
          </v-progress-linear>
        </template>
        <template v-slot:item.expiry="{ item }">
          <div class="text-start">
            <v-chip
              size="small"
              :color="item.expiry > 0 && item.expiry <= panelNowUnix() ? 'error' : ''"
              label>
              {{ HumanReadable.remainedDays(item.expiry) }}
            </v-chip>
          </div>
        </template>
        <template v-slot:item.online="{ item }">
          <div class="text-start">
            <template v-if="onlineNames.has(item.name)">
              <v-chip density="comfortable" size="small" color="success" variant="flat">{{ $t('online') }}</v-chip>
            </template>
            <template v-else>-</template>
          </div>
        </template>
        <template v-slot:item.actions="{ item }">
          <div class="d-flex align-center flex-wrap client-actions">
            <v-menu
              :model-value="delOverlay[item.id] === true"
              :disabled="clientOperationBusy && deletingClientId !== item.id"
              :persistent="deletingClientId === item.id"
              @update:model-value="setDeleteOverlay(item.id, $event)"
              :close-on-content-click="false"
              location="top center">
              <template v-slot:activator="{ props: menuProps }">
                <v-tooltip location="top" :text="$t('actions.del')">
                  <template #activator="{ props: tooltipProps }">
                    <span class="client-action-tooltip" v-bind="tooltipProps">
                      <v-btn
                        v-bind="menuProps"
                        icon="mdi-delete"
                        size="small"
                        variant="text"
                        color="error"
                        :disabled="clientOperationBusy" />
                    </span>
                  </template>
                </v-tooltip>
              </template>
              <v-card :title="$t('actions.del')" rounded="lg">
                <v-divider></v-divider>
                <v-card-text>{{ $t('confirm') }}</v-card-text>
                <v-card-actions>
                  <v-btn color="error" variant="outlined" :loading="isDeletingClient(item.id)" :disabled="clientOperationBusy" @click="delClient(item.id)">{{ $t('yes') }}</v-btn>
                  <v-btn color="success" variant="outlined" :disabled="clientOperationBusy" @click="setDeleteOverlay(item.id, false)">{{ $t('no') }}</v-btn>
                </v-card-actions>
              </v-card>
            </v-menu>
            <v-tooltip location="top" :text="$t('actions.edit')">
              <template #activator="{ props: tooltipProps }">
                <v-btn v-bind="tooltipProps" icon="mdi-pencil" size="small" variant="text" :disabled="clientOperationBusy" @click="showModal(item.id)" />
              </template>
            </v-tooltip>
            <v-tooltip location="top" text="QrCode">
              <template #activator="{ props: tooltipProps }">
                <v-btn v-bind="tooltipProps" icon="mdi-qrcode" size="small" variant="text" :disabled="clientOperationBusy" @click="showQrCode(item.id)" />
              </template>
            </v-tooltip>
            <v-tooltip v-if="enableTraffic" location="top" :text="$t('stats.graphTitle')">
              <template #activator="{ props: tooltipProps }">
                <v-btn v-bind="tooltipProps" icon="mdi-chart-line" size="small" variant="text" :disabled="clientOperationBusy" @click="showStats(item.name)" />
              </template>
            </v-tooltip>
            <v-tooltip location="top" :text="$t('actions.syncToSubManager')">
              <template #activator="{ props: tooltipProps }">
                <v-btn
                  v-bind="tooltipProps"
                  icon="mdi-sync"
                  size="small"
                  variant="text"
                  :color="item.autoSync ? 'primary' : undefined"
                  :loading="syncLoading[item.id]"
                  :disabled="clientOperationBusy || syncLoading[item.id]"
                  @click="syncToSubManager(item)" />
              </template>
            </v-tooltip>
          </div>
        </template>
      </v-data-table>
      </v-col>
    </v-row>
  </template>
</template>

<style scoped>
  .clients-table :deep(.v-data-table__tr--mobile td) {
  height: fit-content;
  min-height: 36px !important;
}

  .clients-table :deep(.v-data-table__tr--mobile td div) {
  max-width: 100%;
  white-space: normal;
  overflow-wrap: anywhere;
}

.client-actions {
  min-width: 248px;
  gap: 2px;
}

.client-action-tooltip {
  display: inline-flex;
}
</style>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import ClientModal from '@/layouts/modals/Client.vue'
import ClientBulk from '@/layouts/modals/ClientBulk.vue'
import QrCode from '@/layouts/modals/QrCode.vue'
import Stats from '@/layouts/modals/Stats.vue'
import { Client } from '@/types/clients'
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { HumanReadable } from '@/plugins/utils'
import { panelNowUnix } from '@/plugins/panelTime'
import { i18n } from '@/locales'
import { push } from 'notivue'
import { useDisplay } from 'vuetify'
import { getNamespaceApi, getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'

const { smAndDown } = useDisplay()

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const store = getNamespaceStore(props.namespace)
const namespaceApi = getNamespaceApi(props.namespace)
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

const clients = computed((): any[] => {
  return store.clients
})

const onlineNames = computed(() => new Set(store.onlines?.user ?? []))

const inbounds = computed((): any[] => {
  return store.inbounds ?? []
})

const inboundTagMap = computed(() => {
  const map = new Map<number, string>()
  for (const inbound of inbounds.value) {
    const id = Number(inbound?.id)
    if (id > 0) map.set(id, String(inbound?.tag ?? ''))
  }
  return map
})

const inboundTags = computed((): any[] => {
  if (!inbounds.value) return []
  return inbounds.value?.filter(i => i.tag != '' && (i.user_management?.selectable ?? !!i.users)).map(i => {
    return { title: i.tag, value: i.id }
  })
})

const enableTraffic = computed(() => {
  return store.enableTraffic
})

const actionMenu = ref(false)
const filterMenu = ref(false)
const filterSettings = ref({
  enabled: false,
  state: '',
  group: '-',
  text: '',
})

const visibleClients = computed((): any[] => {
  if (!filterSettings.value.enabled) return clients.value ?? []

  let result = clients.value.slice()
  if (filterSettings.value.group !== '-') {
    result = result.filter(client => String(client.group ?? '') === filterSettings.value.group)
  }
  const query = filterSettings.value.text.trim().toLocaleLowerCase()
  if (query) {
    result = result.filter(client =>
      String(client.name ?? '').toLocaleLowerCase().includes(query) ||
      String(client.desc ?? '').toLocaleLowerCase().includes(query),
    )
  }
  switch (filterSettings.value.state) {
    case 'disable':
      return result.filter(client => client.enable === false)
    case 'expired':
      return result.filter(client => client.expiry > 0 && client.expiry <= panelNowUnix())
    case 'online':
      return result.filter(client => onlineNames.value.has(client.name))
    default:
      return result
  }
})

const groups = computed((): string[] => Array.from(new Set(clients.value.map(client => String(client.group ?? '')))))

const normalizeClientInboundIds = (raw: unknown): Array<number | string> => {
  const values = Array.isArray(raw)
    ? raw
    : typeof raw === 'string'
      ? (() => {
          try {
            const parsed = JSON.parse(raw)
            return Array.isArray(parsed) ? parsed : []
          } catch {
            return []
          }
        })()
      : []
  const ids: Array<number | string> = []
  const seen = new Set<string>()
  for (const value of values) {
    const normalized = Number.isInteger(Number(value)) && Number(value) > 0 ? Number(value) : String(value ?? '').trim()
    if (normalized === '' || seen.has(String(normalized))) continue
    seen.add(String(normalized))
    ids.push(normalized)
  }
  return ids
}

const clientInboundIdsByClientId = computed(() => {
  const map = new Map<number, Array<number | string>>()
  for (const client of clients.value) {
    const id = Number(client?.id)
    if (Number.isInteger(id) && id > 0) {
      map.set(id, normalizeClientInboundIds(client?.inbounds))
    }
  }
  return map
})

const clientInboundIds = (client: any): Array<number | string> => {
  const id = Number(client?.id)
  return clientInboundIdsByClientId.value.get(id) ?? normalizeClientInboundIds(client?.inbounds)
}

const filterItems = [
  { title: i18n.global.t('none'), value: '' },
  { title: i18n.global.t('disable'), value: 'disable' },
  { title: i18n.global.t('date.expired'), value: 'expired' },
  { title: i18n.global.t('online'), value: 'online' },
]

const headers = [
  { title: i18n.global.t('client.name'), key: 'name' },
  { title: i18n.global.t('client.desc'), key: 'desc' },
  { title: i18n.global.t('client.group'), key: 'group' },
  { title: i18n.global.t('pages.inbounds'), key: 'inbounds', width: 100, minWidth: 100 },
  { title: i18n.global.t('actions.action'), key: 'actions', sortable: false, width: 260, minWidth: 248, nowrap: true },
  { title: i18n.global.t('stats.volume'), key: 'volume' },
  { title: i18n.global.t('date.expiry'), key: 'expiry' },
  { title: i18n.global.t('online'), key: 'online' },
  { key: 'data-table-group', width: 0 },
]

const normalizeItemsPerPage = (raw: unknown): number => {
  const parsed = Number(raw)
  if (parsed === -1) return -1
  return Number.isInteger(parsed) && parsed >= 1 ? parsed : 10
}

const itemPerPage = ref(normalizeItemsPerPage(localStorage.getItem(namespaceApi.itemsPerPageKey)))

const hideDefaultFooter = computed(() => (
  itemPerPage.value === -1 || visibleClients.value.length <= itemPerPage.value
))

const setItemPerPage = (items: number) => {
  itemPerPage.value = normalizeItemsPerPage(items)
  localStorage.setItem(namespaceApi.itemsPerPageKey, itemPerPage.value.toString())
}

const modal = ref({
  visible: false,
  id: 0,
})

const delOverlay = reactive<Record<number, boolean>>({})
const syncLoading = reactive<Record<number, boolean>>({})
const deletingClientId = ref<number | null>(null)
const clientOperationBusy = computed(() => (
  deletingClientId.value !== null
  || Object.values(syncLoading).some(Boolean)
))

const isDeletingClient = (id: number) => deletingClientId.value === id

const setDeleteOverlay = (id: number, open: boolean) => {
  if (open) {
    if (clientOperationBusy.value) return
    for (const key of Object.keys(delOverlay)) {
      if (Number(key) !== id) delete delOverlay[Number(key)]
    }
    delOverlay[id] = true
    return
  }
  if (deletingClientId.value === id) return
  delete delOverlay[id]
}

const showModal = (id: number) => {
  if (clientOperationBusy.value) return
  modal.value.id = id
  modal.value.visible = true
}

const closeModal = () => {
  modal.value.visible = false
}

const delClient = async (id: number) => {
  if (clientOperationBusy.value || !Number.isInteger(id) || id <= 0) return
  deletingClientId.value = id
  try {
    const success = await store.save('clients', 'del', id)
    if (success) delete delOverlay[id]
  } finally {
    if (deletingClientId.value === id) deletingClientId.value = null
  }
}

const qrcode = ref({
  visible: false,
  id: 0,
})

const showQrCode = (id: number) => {
  if (clientOperationBusy.value) return
  qrcode.value.id = id
  qrcode.value.visible = true
}

const closeQrCode = () => {
  qrcode.value.visible = false
}

const stats = ref({
  visible: false,
  resource: 'client',
  tag: '',
})

const showStats = (tag: string) => {
  if (clientOperationBusy.value) return
  stats.value.tag = tag
  stats.value.visible = true
}

const closeStats = () => {
  stats.value.visible = false
}

const doFilter = () => {
  filterSettings.value.enabled = true
  filterMenu.value = false
}

const clearFilter = () => {
  filterSettings.value = {
    enabled: false,
    state: '',
    group: '-',
    text: '',
  }
  filterMenu.value = false
}

const addBulkModal = ref(false)

const addBulk = () => {
  if (clientOperationBusy.value) return
  addBulkModal.value = true
  actionMenu.value = false
}

const closeBulk = () => {
  addBulkModal.value = false
}

const syncToSubManager = async (client: any) => {
  const clientId = Number(client.id)
  const clientName = String(client.name ?? '')
  if (!Number.isInteger(clientId) || clientId <= 0 || clientOperationBusy.value) return
  syncLoading[clientId] = true
  try {
    const msg = await HttpUtils.post(namespaceApi.syncEndpoint, { name: clientName }, { silentErrorToast: true })
    if (msg.success) {
      const result = msg.obj ?? {}
      const count = Number.isFinite(Number(result.count)) ? Number(result.count) : 0
      const refreshed = await store.loadData(true)
      if (!refreshed) {
        push.warning({
          title: i18n.global.t('failed'),
          message: i18n.global.t('actions.syncToSubManagerRefreshFailed'),
          duration: 5000,
        })
      } else if (count === 0) {
        push.warning({
          title: i18n.global.t('success'),
          message: i18n.global.t('actions.syncToSubManagerNoNodes', { name: clientName }),
          duration: 5000,
        })
      } else {
        push.success({
          message: i18n.global.t('actions.syncToSubManagerSuccess', { name: clientName, count }),
          duration: 3000,
        })
      }
    } else if (msg.obj?.committed === true) {
      const result = msg.obj?.result ?? {}
      const count = Number.isFinite(Number(result.count)) ? Number(result.count) : 0
      const autoSyncEnabled = msg.obj?.autoSyncEnabled === true
      const refreshed = await store.loadData(true)
      if (!refreshed) {
        push.warning({
          title: i18n.global.t('failed'),
          message: i18n.global.t('actions.syncToSubManagerRefreshFailed'),
          duration: 5000,
        })
      } else {
        push.warning({
          title: i18n.global.t('warning'),
          message: autoSyncEnabled
            ? i18n.global.t('actions.syncToSubManagerCommittedWithAuto', { name: clientName, count })
            : i18n.global.t('actions.syncToSubManagerCommittedWithoutAuto', { name: clientName, count }),
          duration: 7000,
        })
      }
    } else if (msg.msg) {
      push.warning({
        title: i18n.global.t('failed'),
        message: msg.msg,
        duration: 5000,
      })
    }
  } finally {
    delete syncLoading[clientId]
  }
}

onMounted(() => {
  void initialize()
})

onUnmounted(() => {
  componentActive = false
})

const percent = (c: Client) => {
  return c.volume > 0 ? Math.round((c.up + c.down) * 100 / c.volume) : 0
}

const percentColor = (c: Client) => {
  return (c.up + c.down) >= c.volume ? 'error' : percent(c) > 90 ? 'warning' : 'success'
}
</script>
