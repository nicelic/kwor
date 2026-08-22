<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="800" max-width="90vw" max-height="90vh" :persistent="operationBusy">
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row align="center">
          <v-col cols="auto">{{ $t('actions.group') }}</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <v-btn color="primary" variant="flat" :disabled="operationBusy" @click="showAddDialog">
              {{ $t('actions.add') }}
            </v-btn>
          </v-col>
          <v-col cols="auto">
            <v-btn
              icon="mdi-close"
              size="small"
              variant="text"
              :disabled="operationBusy"
              @click="dialogVisible = false"
            >
              <v-icon />
              <v-tooltip activator="parent" location="top">{{ $t('actions.close') }}</v-tooltip>
            </v-btn>
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 16px; min-height: min(400px, calc(90vh - 92px)); max-height: calc(90vh - 92px); overflow-y: auto;">
        <v-alert v-if="groupsLoadFailed && groups.length > 0" type="warning" variant="tonal" class="mb-3">
          分组列表刷新失败，当前仍显示上次成功读取的数据。
          <v-btn class="mt-2" size="small" variant="outlined" :loading="groupsLoading" :disabled="operationBusy" @click="loadGroups">
            {{ $t('actions.update') }}
          </v-btn>
        </v-alert>
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
            :class="{ 'outbound-group-drag-handle--disabled': groups.length < 2 || operationBusy }"
            :draggable="groups.length > 1 && !operationBusy"
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
                :disabled="operationBusy"
                @click="confirmDelete(group.id)"
              >
                <v-icon />
                <v-tooltip activator="parent" location="top">{{ $t('actions.del') }}</v-tooltip>
              </v-btn>
              <v-btn
                icon="mdi-pencil"
                size="small"
                color="primary"
                variant="text"
                :disabled="operationBusy"
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
                :disabled="operationBusy"
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
                v-for="outbound in previewGroupOutbounds(group)"
                :key="outbound"
                size="small"
                class="mr-1 mb-1"
              >
                {{ outbound }}
              </v-chip>
              <span
                v-if="getGroupOutboundCount(group) > groupOutboundPreviewLimit"
                class="text-caption ml-1"
              >
                还有 {{ getGroupOutboundCount(group) - groupOutboundPreviewLimit }} 个节点
              </span>
            </v-list-item-subtitle>
          </v-list-item>
        </v-list>
        <v-alert v-else-if="groupsLoadFailed" type="error" variant="tonal">
          分组列表加载失败，请稍后重试。
          <v-btn class="mt-2" size="small" variant="outlined" :loading="groupsLoading" :disabled="operationBusy" @click="loadGroups">
            {{ $t('actions.update') }}
          </v-btn>
        </v-alert>
        <v-alert v-else type="info" variant="tonal">
          暂无分组，点击“添加”按钮创建新分组
        </v-alert>
      </v-card-text>
    </v-card>
  </v-dialog>

  <v-dialog v-model="editDialog" width="500" max-width="90vw" max-height="90vh" :persistent="operationBusy">
    <v-card class="rounded-lg">
      <v-card-title>
        {{ editingGroupId === null ? $t('actions.add') : $t('actions.edit') }}{{ $t('actions.group') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 16px; max-height: calc(90vh - 152px); overflow-y: auto;">
        <v-row>
          <v-col cols="12">
            <v-text-field
              v-model="editingGroup.name"
              :disabled="operationBusy"
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
              :disabled="operationBusy"
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
              :disabled="operationBusy"
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
        <v-btn color="primary" variant="outlined" :disabled="operationBusy" @click="editDialog = false">
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn color="primary" variant="flat" :loading="saving" :disabled="operationBusy" @click="saveGroup">
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-dialog v-model="deleteDialog" width="400" max-width="90vw" max-height="90vh" :persistent="operationBusy">
    <v-card class="rounded-lg">
      <v-card-title>{{ $t('actions.del') }}</v-card-title>
      <v-divider></v-divider>
      <v-card-text class="outbound-group-delete-copy">
        <template v-if="isMihomoNamespace">
          删除分组会同时删除组内 Mihomo 节点；存在代理组、路由、入站或其它分组引用时不会删除。
        </template>
        <template v-else>
          删除分组会删除组内出站节点；节点被路由、其它出站或其它分组引用时会拒绝删除。
        </template>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="success" variant="outlined" :disabled="operationBusy" @click="deleteDialog = false">
          {{ $t('no') }}
        </v-btn>
        <v-btn color="error" variant="outlined" :loading="deleting" :disabled="operationBusy" @click="deleteGroup">
          {{ $t('yes') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <v-dialog v-model="refreshResultDialog" width="600" max-width="90vw" max-height="90vh" :persistent="operationBusy">
    <v-card class="rounded-lg">
      <v-card-title>刷新订阅结果</v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 16px; max-height: calc(90vh - 144px); overflow-y: auto;">
        <v-alert v-if="refreshResult.error" type="error" variant="tonal" class="mb-3">
          {{ refreshResult.error }}
        </v-alert>
        <div v-else>
          <v-alert v-if="refreshResult.warning" type="warning" variant="tonal" class="mb-3">
            {{ refreshResult.warning }}
          </v-alert>
          <v-alert v-if="refreshResult.added.length > 0" type="success" variant="tonal" class="mb-2">
            <div class="font-weight-bold mb-1">新增节点 ({{ refreshResult.added.length }}):</div>
            <v-chip v-for="node in previewRefreshNodes(refreshResult.added)" :key="node" size="small" class="mr-1 mb-1" color="success">
              {{ node }}
            </v-chip>
            <div v-if="refreshResult.added.length > refreshResultPreviewLimit" class="text-caption mt-1">
              仅显示前 {{ refreshResultPreviewLimit }} 个节点标签
            </div>
          </v-alert>
          <v-alert v-if="refreshResult.removed.length > 0" type="warning" variant="tonal" class="mb-2">
            <div class="font-weight-bold mb-1">删除节点 ({{ refreshResult.removed.length }}):</div>
            <v-chip v-for="node in previewRefreshNodes(refreshResult.removed)" :key="node" size="small" class="mr-1 mb-1" color="warning">
              {{ node }}
            </v-chip>
            <div v-if="refreshResult.removed.length > refreshResultPreviewLimit" class="text-caption mt-1">
              仅显示前 {{ refreshResultPreviewLimit }} 个节点标签
            </div>
          </v-alert>
          <v-alert v-if="refreshResult.updated.length > 0" type="info" variant="tonal" class="mb-2">
            <div class="font-weight-bold mb-1">更新节点 ({{ refreshResult.updated.length }}):</div>
            <v-chip v-for="node in previewRefreshNodes(refreshResult.updated)" :key="node" size="small" class="mr-1 mb-1" color="info">
              {{ node }}
            </v-chip>
            <div v-if="refreshResult.updated.length > refreshResultPreviewLimit" class="text-caption mt-1">
              仅显示前 {{ refreshResultPreviewLimit }} 个节点标签
            </div>
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
        <v-btn color="primary" variant="flat" :disabled="operationBusy" @click="refreshResultDialog = false">
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
const groupsLoading = ref(false)
const groupsLoadFailed = ref(false)
const draggedGroupIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)
const reordering = ref(false)
const editDialog = ref(false)
const editingGroupId = ref<number | null>(null)
const editingGroup = ref<OutboundGroup>(createOutboundGroup())
const saving = ref(false)
const originalSubscription = ref({
  url: '',
  allowInsecure: false,
})

const deleteDialog = ref(false)
const deletingGroupId = ref<number | null>(null)
const deleting = ref(false)

const refreshingGroup = ref('')
const groupOutboundPreviewLimit = 80
const refreshResultPreviewLimit = 80
const refreshResultDialog = ref(false)
type SubscriptionRefreshResult = {
  added: string[]
  removed: string[]
  updated: string[]
  error: string
  warning: string
}

const refreshResult = ref<SubscriptionRefreshResult>({
  added: <string[]>[],
  removed: <string[]>[],
  updated: <string[]>[],
  error: '',
  warning: ''
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
const subscriptionBusy = computed(() => refreshingGroup.value !== '')
const operationBusy = computed(() => (
  saving.value
  || deleting.value
  || reordering.value
  || subscriptionBusy.value
))
const isCommittedSubscriptionResult = (msg: { success: boolean, obj: any | null }) => msg.success || msg.obj?.committed === true
const normalizeRefreshNodes = (raw: unknown): string[] => {
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  const nodes: string[] = []
  for (const value of raw) {
    if (typeof value !== 'string') continue
    const node = value.trim()
    if (!node || seen.has(node)) continue
    seen.add(node)
    nodes.push(node)
  }
  return nodes
}
const normalizeRefreshResult = (raw: unknown, fallbackError = ''): SubscriptionRefreshResult => {
  const result = raw && typeof raw === 'object' && !Array.isArray(raw)
    ? raw as Record<string, unknown>
    : {}
  const error = typeof result.error === 'string' && result.error.trim() !== ''
    ? result.error.trim()
    : fallbackError
  const warning = typeof result.warning === 'string' ? result.warning.trim() : ''
  return {
    added: normalizeRefreshNodes(result.added),
    removed: normalizeRefreshNodes(result.removed),
    updated: normalizeRefreshNodes(result.updated),
    error,
    warning,
  }
}
const previewRefreshNodes = (nodes: unknown): string[] => normalizeRefreshNodes(nodes).slice(0, refreshResultPreviewLimit)

const getGroupOutbounds = (group: OutboundGroup): string[] => {
  if (typeof group.outbounds === 'string') {
    try {
      const parsed = JSON.parse(group.outbounds)
      return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string') : []
    } catch {
      return []
    }
  }
  return Array.isArray(group.outbounds)
    ? group.outbounds.filter((item): item is string => typeof item === 'string')
    : []
}

const previewGroupOutbounds = (group: OutboundGroup): string[] => {
  return getGroupOutbounds(group).slice(0, groupOutboundPreviewLimit)
}

const getGroupOutboundCount = (group: OutboundGroup): number => {
  return getGroupOutbounds(group).length
}

const loadGroups = async () => {
  groupsLoading.value = true
  try {
    const data = await store.value.loadOutboundGroups()
    if (Array.isArray(data)) {
      groups.value = data
      groupsLoadFailed.value = false
      return true
    }
  } catch {
    // Keep the last known list visible when a refresh request fails.
  } finally {
    groupsLoading.value = false
  }
  groupsLoadFailed.value = true
  push.warning({ title: '分组', message: '分组列表加载失败，请稍后重试。' })
  return false
}

const saveGroupOrder = async (): Promise<boolean> => {
  if (operationBusy.value) return false
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
      if (Array.isArray(msg.obj.outboundgroups)) {
        groups.value = msg.obj.outboundgroups as OutboundGroup[]
        groupsLoadFailed.value = false
      } else {
        await loadGroups()
      }
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
  if (operationBusy.value || groups.value.length < 2) {
    return
  }
  draggedGroupIndex.value = index
  dragOverIndex.value = index
}

const handleDragOver = (index: number) => {
  if (draggedGroupIndex.value === null || operationBusy.value) {
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
  if (fromIndex === null || operationBusy.value) {
    handleDragEnd()
    return
  }

  if (fromIndex === index) {
    handleDragEnd()
    return
  }

  const previousGroups = groups.value.slice()
  const [movedGroup] = groups.value.splice(fromIndex, 1)
  const targetIndex = fromIndex < index ? index - 1 : index
  groups.value.splice(targetIndex, 0, movedGroup)
  handleDragEnd()

  const success = await saveGroupOrder()
  if (!success) {
    groups.value = previousGroups
    await loadGroups()
  }
}

const showAddDialog = () => {
  if (operationBusy.value) return
  editingGroupId.value = null
  editingGroup.value = createOutboundGroup()
  originalSubscription.value = { url: '', allowInsecure: false }
  editDialog.value = true
}
const showEditDialog = (index: number) => {
  if (operationBusy.value) return
  const group = groups.value[index]
  const id = Number(group?.id)
  if (!group || !Number.isInteger(id) || id <= 0) return
  editingGroupId.value = id
  editingGroup.value = {
    ...group,
    outbounds: getGroupOutbounds(group),
    subscription_url: group.subscription_url || '',
    allow_insecure: group.allow_insecure || false
  }
  originalSubscription.value = {
    url: editingGroup.value.subscription_url || '',
    allowInsecure: editingGroup.value.allow_insecure === true,
  }
  editDialog.value = true
}

const saveGroup = async () => {
  if (operationBusy.value || !editingGroup.value.name.trim()) return

  saving.value = true
  try {
    const subscriptionURL = editingGroup.value.subscription_url || ''
    const action = editingGroupId.value === null ? 'new' : 'edit'
    const shouldRefreshSubscription = subscriptionURL !== '' && (
      action === 'new'
      || subscriptionURL !== originalSubscription.value.url
      || (editingGroup.value.allow_insecure === true) !== originalSubscription.value.allowInsecure
    )
    const groupData = {
      ...editingGroup.value,
      outbounds: JSON.stringify(editingGroup.value.outbounds || []),
      subscription_url: shouldRefreshSubscription ? originalSubscription.value.url : subscriptionURL,
      allow_insecure: shouldRefreshSubscription ? originalSubscription.value.allowInsecure : (editingGroup.value.allow_insecure || false)
    }
    const success = await store.value.save(saveObject.value, action, groupData)
    if (!success) return

    if (shouldRefreshSubscription) {
      refreshingGroup.value = editingGroup.value.name
      try {
        let subscriptionSuccess = false
        if (action === 'new') {
          const msg = await fetchAndSaveSubscription(
            editingGroup.value.name,
            subscriptionURL,
            editingGroup.value.allow_insecure || false
          )
          subscriptionSuccess = isCommittedSubscriptionResult(msg)
        } else {
          const msg = await refreshSubscriptionByParams(
            editingGroup.value.name,
            subscriptionURL,
            editingGroup.value.allow_insecure || false,
            false
          )
          subscriptionSuccess = isCommittedSubscriptionResult(msg)
        }
        if (!subscriptionSuccess) {
          if (action === 'new') {
            await store.value.save(saveObject.value, 'del', editingGroup.value.name)
          }
          await loadGroups()
          return
        }
      } finally {
        refreshingGroup.value = ''
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
  if (isCommittedSubscriptionResult(msg) && msg.obj) {
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
  if (isCommittedSubscriptionResult(msg) && msg.obj) {
    store.value.setNewData(msg.obj)
    if (showResult) {
      const result = normalizeRefreshResult(msg.obj.result)
      if (msg.obj.committed === true) result.warning = msg.msg || '订阅数据已保存，但运行配置未更新'
      refreshResult.value = result
      refreshResultDialog.value = true
    }
  } else if (showResult) {
    refreshResult.value = normalizeRefreshResult(null, msg.msg || '刷新订阅失败')
    refreshResultDialog.value = true
  }

  return msg
}

const refreshSubscription = async (group: OutboundGroup) => {
  if (operationBusy.value || !group.subscription_url) return
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

const confirmDelete = (groupId: number) => {
  if (operationBusy.value) return
  const id = Number(groupId)
  if (!Number.isInteger(id) || id <= 0) return
  deletingGroupId.value = id
  deleteDialog.value = true
}

watch(deleteDialog, (visible) => {
  if (!visible && !deleting.value) deletingGroupId.value = null
})

const deleteGroup = async () => {
  const id = deletingGroupId.value
  if (id == null || operationBusy.value) return
  const group = groups.value.find((item) => Number(item.id) === id)
  if (!group?.name) {
    deleteDialog.value = false
    deletingGroupId.value = null
    return
  }
  deleting.value = true
  try {
    const success = await store.value.save(saveObject.value, 'del', group.name)
    if (success) {
      deleteDialog.value = false
      deletingGroupId.value = null
      await loadGroups()
    }
  } finally {
    deleting.value = false
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

.outbound-group-delete-copy {
  overflow-wrap: anywhere;
}
</style>
