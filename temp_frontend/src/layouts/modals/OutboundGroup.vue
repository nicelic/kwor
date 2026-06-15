<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="800" max-width="90vw">
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row align="center">
          <v-col cols="auto">{{ $t('actions.group') }}</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <v-btn color="primary" variant="flat" @click="showAddDialog">
              {{ $t('actions.add') }}
            </v-btn>
          </v-col>
          <v-col cols="auto">
            <v-icon icon="mdi-close" @click="$emit('close')"></v-icon>
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 16px; min-height: 400px;">
        <v-list v-if="groups.length > 0">
          <v-list-item
            v-for="(group, index) in groups"
            :key="group.id"
            class="mb-2 outbound-group-sort-item"
            :class="{
              'outbound-group-sort-item--active': dragOverIndex === index,
              'outbound-group-sort-item--dragging': draggedGroupIndex === index
            }"
            rounded="lg"
            border
            @dragover.prevent="handleDragOver(index)"
            @drop.prevent="handleDrop(index)"
          >
            <template v-slot:prepend>
              <div class="d-flex align-center outbound-group-prepend">
                <div
                  class="outbound-group-drag-handle d-flex align-center justify-center mr-3"
                  :class="{ 'outbound-group-drag-handle--disabled': groups.length < 2 || reordering }"
                  :draggable="groups.length > 1 && !reordering"
                  @dragstart="handleDragStart(index)"
                  @dragend="handleDragEnd"
                >
                  <v-icon size="small">mdi-drag</v-icon>
                  <v-tooltip activator="parent" location="top">拖动排序</v-tooltip>
                </div>
                <v-list-item-title class="text-h6">{{ group.name }}</v-list-item-title>
              </div>
            </template>
            <template v-slot:append>
              <v-btn
                icon="mdi-delete"
                size="small"
                color="error"
                variant="text"
                :disabled="reordering"
                @click="confirmDelete(index)"
              >
                <v-icon />
                <v-tooltip activator="parent" location="top">{{ $t('actions.del') }}</v-tooltip>
              </v-btn>
              <v-btn
                icon="mdi-pencil"
                size="small"
                color="primary"
                variant="text"
                :disabled="reordering"
                @click="showEditDialog(index)"
              >
                <v-icon />
                <v-tooltip activator="parent" location="top">{{ $t('actions.edit') }}</v-tooltip>
              </v-btn>
              <v-btn
                v-if="group.subscription_url"
                icon="mdi-sync"
                size="small"
                color="info"
                variant="text"
                :disabled="reordering"
                :loading="refreshingGroup === group.name"
                @click="refreshSubscription(group)"
              >
                <v-icon />
                <v-tooltip activator="parent" location="top">{{ refreshTooltip }}</v-tooltip>
              </v-btn>
            </template>
            <v-list-item-subtitle class="mt-2">
              <v-chip
                v-if="group.subscription_url"
                size="small"
                color="info"
                class="mr-1 mb-1"
              >
                <v-icon start size="x-small">mdi-link</v-icon>
                {{ subscriptionChipLabel }}
              </v-chip>
              <v-chip
                v-for="outbound in getGroupOutbounds(group)"
                :key="outbound"
                size="small"
                class="mr-1 mb-1"
              >
                {{ outbound }}
              </v-chip>
            </v-list-item-subtitle>
          </v-list-item>
        </v-list>
        <v-alert v-else type="info" variant="tonal">
          暂无分组，点击“添加”按钮创建新分组
        </v-alert>
      </v-card-text>
    </v-card>
  </v-dialog>

  <v-dialog v-model="editDialog" max-width="500">
    <v-card class="rounded-lg">
      <v-card-title>
        {{ editingIndex === -1 ? $t('actions.add') : $t('actions.edit') }}{{ $t('actions.group') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 16px;">
        <v-row>
          <v-col cols="12">
            <v-text-field
              v-model="editingGroup.name"
              label="名称"
              variant="outlined"
              density="compact"
              hide-details
            ></v-text-field>
          </v-col>
        </v-row>
        <v-row class="mt-4">
          <v-col cols="12">
            <v-text-field
              v-model="editingGroup.subscription_url"
              :label="subscriptionFieldLabel"
              :placeholder="subscriptionFieldPlaceholder"
              variant="outlined"
              density="compact"
              hide-details
              clearable
            ></v-text-field>
          </v-col>
        </v-row>
        <v-row class="mt-2" v-if="editingGroup.subscription_url">
          <v-col cols="12">
            <v-switch
              v-model="editingGroup.allow_insecure"
              label="允许不安全连接（跳过证书校验）"
              color="warning"
              density="compact"
              hide-details
            ></v-switch>
          </v-col>
        </v-row>
      </v-card-text>
      <v-divider></v-divider>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="primary" variant="outlined" @click="editDialog = false">
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn color="primary" variant="flat" :loading="saving" @click="saveGroup">
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-dialog v-model="deleteDialog" max-width="400">
    <v-card class="rounded-lg">
      <v-card-title>{{ $t('actions.del') }}</v-card-title>
      <v-divider></v-divider>
      <v-card-text>{{ $t('confirm') }}</v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="success" variant="outlined" @click="deleteDialog = false">
          {{ $t('no') }}
        </v-btn>
        <v-btn color="error" variant="outlined" @click="deleteGroup">
          {{ $t('yes') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-dialog v-model="refreshResultDialog" max-width="600">
    <v-card class="rounded-lg">
      <v-card-title>刷新订阅结果</v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 16px;">
        <v-alert v-if="refreshResult.error" type="error" variant="tonal" class="mb-3">
          {{ refreshResult.error }}
        </v-alert>
        <div v-else>
          <v-alert v-if="refreshResult.added.length > 0" type="success" variant="tonal" class="mb-2">
            <div class="font-weight-bold mb-1">新增节点 ({{ refreshResult.added.length }}):</div>
            <v-chip v-for="node in refreshResult.added" :key="node" size="small" class="mr-1 mb-1" color="success">
              {{ node }}
            </v-chip>
          </v-alert>
          <v-alert v-if="refreshResult.removed.length > 0" type="warning" variant="tonal" class="mb-2">
            <div class="font-weight-bold mb-1">删除节点 ({{ refreshResult.removed.length }}):</div>
            <v-chip v-for="node in refreshResult.removed" :key="node" size="small" class="mr-1 mb-1" color="warning">
              {{ node }}
            </v-chip>
          </v-alert>
          <v-alert v-if="refreshResult.updated.length > 0" type="info" variant="tonal" class="mb-2">
            <div class="font-weight-bold mb-1">更新节点 ({{ refreshResult.updated.length }}):</div>
            <v-chip v-for="node in refreshResult.updated" :key="node" size="small" class="mr-1 mb-1" color="info">
              {{ node }}
            </v-chip>
          </v-alert>
          <v-alert
            v-if="refreshResult.added.length === 0 && refreshResult.removed.length === 0 && refreshResult.updated.length === 0"
            type="info"
            variant="tonal"
          >
            订阅内容没有变化
          </v-alert>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="primary" variant="flat" @click="refreshResultDialog = false">
          确定
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { push } from 'notivue'
import HttpUtils from '@/plugins/httputil'
import { OutboundGroup, createOutboundGroup } from '@/types/outboundgroups'
import { getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'

const props = defineProps<{
  visible: boolean
  namespace?: UiNamespace
}>()

const emit = defineEmits(['close', 'update:modelValue'])

const dialogVisible = ref(props.visible)
watch(() => props.visible, (newVal) => {
  dialogVisible.value = newVal
  if (newVal) {
    loadGroups()
  }
})
watch(dialogVisible, (newVal) => {
  if (!newVal) {
    emit('close')
    emit('update:modelValue', false)
  }
})

const groups = ref<OutboundGroup[]>([])
const draggedGroupIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)
const reordering = ref(false)
const editDialog = ref(false)
const editingIndex = ref(-1)
const editingGroup = ref<OutboundGroup>(createOutboundGroup())
const saving = ref(false)

const deleteDialog = ref(false)
const deletingIndex = ref(-1)

const refreshingGroup = ref('')
const refreshResultDialog = ref(false)
const refreshResult = ref({
  added: <string[]>[],
  removed: <string[]>[],
  updated: <string[]>[],
  error: ''
})

const isMihomoNamespace = computed(() => props.namespace === 'mihomo')
const store = computed(() => getNamespaceStore(props.namespace))
const fetchEndpoint = computed(() => isMihomoNamespace.value ? 'api/fetchMihomoOutboundSubscription' : 'api/fetchOutboundSubscription')
const refreshEndpoint = computed(() => isMihomoNamespace.value ? 'api/refreshMihomoOutboundSubscription' : 'api/refreshOutboundSubscription')
const saveObject = computed(() => isMihomoNamespace.value ? 'mihomo_outboundgroups' : 'outboundgroups')
const subscriptionFieldLabel = computed(() => isMihomoNamespace.value ? 'Clash 订阅链接（可选）' : '订阅链接（可选）')
const subscriptionFieldPlaceholder = computed(() => (
  isMihomoNamespace.value
    ? '输入 Clash 订阅链接用于批量导入 mihomo 节点'
    : '输入订阅 JSON 链接用于批量导入'
))
const subscriptionChipLabel = computed(() => isMihomoNamespace.value ? 'Clash 订阅' : '订阅链接')
const refreshTooltip = computed(() => isMihomoNamespace.value ? '刷新 Clash 订阅' : '刷新订阅')

const getGroupOutbounds = (group: OutboundGroup): string[] => {
  if (typeof group.outbounds === 'string') {
    try {
      return JSON.parse(group.outbounds)
    } catch {
      return []
    }
  }
  return group.outbounds as unknown as string[]
}

const loadGroups = async () => {
  const data = await store.value.loadOutboundGroups()
  groups.value = data || []
}

const saveGroupOrder = async (): Promise<boolean> => {
  const ids = groups.value
    .map((group) => Number(group.id))
    .filter((id) => Number.isInteger(id) && id > 0)

  if (ids.length !== groups.value.length) {
    push.warning({
      title: '分组',
      message: '分组顺序保存失败，请刷新后重试'
    })
    await loadGroups()
    return false
  }

  reordering.value = true
  try {
    const msg = await HttpUtils.post('api/save', {
      object: saveObject.value,
      action: 'reorder',
      data: JSON.stringify({ ids }, null, 2)
    })
    if (msg.success && msg.obj) {
      store.value.setNewData(msg.obj)
      groups.value = (msg.obj.outboundgroups ?? []) as OutboundGroup[]
      push.success({
        title: '分组',
        message: '分组顺序已保存'
      })
      return true
    }
    return false
  } finally {
    reordering.value = false
  }
}

const handleDragStart = (index: number) => {
  if (reordering.value || groups.value.length < 2) {
    return
  }
  draggedGroupIndex.value = index
  dragOverIndex.value = index
}

const handleDragOver = (index: number) => {
  if (draggedGroupIndex.value === null || reordering.value) {
    return
  }
  dragOverIndex.value = index
}

const handleDragEnd = () => {
  draggedGroupIndex.value = null
  dragOverIndex.value = null
}

const handleDrop = async (index: number) => {
  const fromIndex = draggedGroupIndex.value
  if (fromIndex === null || reordering.value) {
    handleDragEnd()
    return
  }

  if (fromIndex === index) {
    handleDragEnd()
    return
  }

  const previousGroups = groups.value.slice()
  const [movedGroup] = groups.value.splice(fromIndex, 1)
  groups.value.splice(index, 0, movedGroup)
  handleDragEnd()

  const success = await saveGroupOrder()
  if (!success) {
    groups.value = previousGroups
    await loadGroups()
  }
}

const showAddDialog = () => {
  editingIndex.value = -1
  editingGroup.value = createOutboundGroup()
  editDialog.value = true
}
const showEditDialog = (index: number) => {
  editingIndex.value = index
  const group = groups.value[index]
  editingGroup.value = {
    ...group,
    outbounds: getGroupOutbounds(group),
    subscription_url: group.subscription_url || '',
    allow_insecure: group.allow_insecure || false
  }
  editDialog.value = true
}

const saveGroup = async () => {
  if (!editingGroup.value.name.trim()) return

  saving.value = true
  try {
    const groupData = {
      ...editingGroup.value,
      outbounds: JSON.stringify(editingGroup.value.outbounds || []),
      subscription_url: editingGroup.value.subscription_url || '',
      allow_insecure: editingGroup.value.allow_insecure || false
    }
    const action = editingIndex.value === -1 ? 'new' : 'edit'
    const success = await store.value.save('outboundgroups', action, groupData)
    if (!success) return

    if (editingGroup.value.subscription_url) {
      if (action === 'new') {
        await fetchAndSaveSubscription(
          editingGroup.value.name,
          editingGroup.value.subscription_url,
          editingGroup.value.allow_insecure || false
        )
      } else {
        await refreshSubscriptionByParams(
          editingGroup.value.name,
          editingGroup.value.subscription_url,
          editingGroup.value.allow_insecure || false,
          false
        )
      }
    }

    editDialog.value = false
    await loadGroups()
  } finally {
    saving.value = false
  }
}

const fetchAndSaveSubscription = async (groupName: string, url: string, allowInsecure: boolean) => {
  const formData = new FormData()
  formData.append('group_name', groupName)
  formData.append('url', url)
  formData.append('allow_insecure', String(allowInsecure))
  const msg = await HttpUtils.post(fetchEndpoint.value, formData)
  if (msg.success && msg.obj) {
    store.value.setNewData(msg.obj)
  }
  return msg
}

const refreshSubscriptionByParams = async (
  groupName: string,
  url: string,
  allowInsecure: boolean,
  showResult: boolean
) => {
  const formData = new FormData()
  formData.append('group_name', groupName)
  formData.append('url', url)
  formData.append('allow_insecure', String(allowInsecure))

  const msg = await HttpUtils.post(refreshEndpoint.value, formData)
  if (msg.success && msg.obj) {
    store.value.setNewData(msg.obj)
    if (showResult) {
      refreshResult.value = msg.obj.result || {
        added: [],
        removed: [],
        updated: [],
        error: ''
      }
      refreshResultDialog.value = true
    }
  } else if (showResult) {
    refreshResult.value = {
      added: [],
      removed: [],
      updated: [],
      error: msg.msg || '刷新订阅失败'
    }
    refreshResultDialog.value = true
  }

  return msg
}

const refreshSubscription = async (group: OutboundGroup) => {
  if (!group.subscription_url) return
  refreshingGroup.value = group.name
  try {
    await refreshSubscriptionByParams(
      group.name,
      group.subscription_url,
      group.allow_insecure || false,
      true
    )
    await loadGroups()
  } finally {
    refreshingGroup.value = ''
  }
}

const confirmDelete = (index: number) => {
  deletingIndex.value = index
  deleteDialog.value = true
}

const deleteGroup = async () => {
  const groupName = groups.value[deletingIndex.value].name
  const success = await store.value.save('outboundgroups', 'del', groupName)
  if (success) {
    deleteDialog.value = false
    await loadGroups()
  }
}
</script>

<style scoped>
.outbound-group-prepend {
  min-width: 0;
}

.outbound-group-drag-handle {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  color: rgba(var(--v-theme-on-surface), 0.66);
  cursor: grab;
  user-select: none;
  transition: background-color 0.18s ease, color 0.18s ease;
}

.outbound-group-drag-handle:hover {
  background: rgba(var(--v-theme-on-surface), 0.08);
  color: rgba(var(--v-theme-on-surface), 0.9);
}

.outbound-group-drag-handle:active {
  cursor: grabbing;
}

.outbound-group-drag-handle--disabled {
  cursor: default;
  opacity: 0.45;
}

.outbound-group-sort-item {
  transition: border-color 0.18s ease, background-color 0.18s ease, opacity 0.18s ease;
}

.outbound-group-sort-item--active {
  border-color: rgb(var(--v-theme-primary)) !important;
  background: rgba(var(--v-theme-primary), 0.08);
}

.outbound-group-sort-item--dragging {
  opacity: 0.72;
}
</style>


