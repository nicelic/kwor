<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="800" max-width="90vw" max-height="90vh" :persistent="operationBusy">
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row align="center" class="ga-2">
          <v-col cols="12" sm="auto">
            {{ $t('actions.group') }}
          </v-col>
          <v-col cols="12" sm="auto" class="d-flex justify-sm-end">
            <div class="d-flex align-center flex-wrap" style="gap: 12px;">
              <v-switch
                v-model="autoUpdateEnabled"
                label="自动更新"
                color="primary"
                density="compact"
                hide-details
                inset
                :loading="autoUpdateSaving || autoUpdateLoading"
                :disabled="autoUpdateLoading || operationBusy"
                @update:model-value="handleAutoUpdateToggle"
              ></v-switch>
              <v-text-field
                v-model="autoUpdateIntervalInput"
                label="分钟"
                placeholder="5 or 5m"
                variant="outlined"
                density="compact"
                hide-details
                style="width: 130px;"
                :disabled="!autoUpdateEnabled || autoUpdateLoading || operationBusy"
                :loading="autoUpdateSaving || autoUpdateLoading"
                @blur="handleAutoUpdateIntervalCommit"
                @keydown.enter.prevent="handleAutoUpdateIntervalCommit"
              ></v-text-field>
            </div>
          </v-col>
          <v-col cols="auto" class="ml-sm-auto">
            <v-btn color="primary" variant="flat" :disabled="operationBusy" @click="showAddDialog">
              {{ $t('actions.add') }}
            </v-btn>
          </v-col>
          <v-col cols="auto">
            <v-btn icon="mdi-close" variant="text" :disabled="operationBusy" @click="$emit('close')">
              <v-tooltip activator="parent" location="top">{{ $t('actions.close') }}</v-tooltip>
            </v-btn>
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 16px; min-height: 400px; max-height: calc(90vh - 96px); overflow-y: auto;">
        <v-alert v-if="groupsLoadFailed && groups.length > 0" type="warning" variant="tonal" class="mb-3">
          分组列表刷新失败，当前仍显示上次成功读取的数据。
          <v-btn class="mt-2" size="small" variant="outlined" :loading="groupsLoading" :disabled="operationBusy" @click="loadGroups">
            {{ $t('actions.update') }}
          </v-btn>
        </v-alert>
        <v-progress-linear v-if="groupsLoading && groups.length === 0" indeterminate color="primary" class="mb-3" />
        <v-list v-else-if="groups.length > 0">
          <v-list-item
            v-for="(group, index) in groups"
            :key="group.id || `subgroup-${index}`"
            class="mb-2 subgroup-sort-item"
            :class="{
              'subgroup-sort-item--active': dragOverIndex === index,
              'subgroup-sort-item--dragging': draggedGroupIndex === index
            }"
            rounded="lg"
            border
            @dragover.prevent="handleDragOver(index)"
            @drop.prevent="handleDrop(index)"
          >
            <template v-slot:prepend>
              <div class="d-flex align-center subgroup-prepend">
                <div
                  class="subgroup-drag-handle d-flex align-center justify-center mr-3"
                  :class="{ 'subgroup-drag-handle--disabled': groups.length < 2 || operationBusy }"
                  :draggable="groups.length > 1 && !operationBusy"
                  @dragstart="handleDragStart(index)"
                  @dragend="handleDragEnd"
                >
                  <v-icon size="small">mdi-drag</v-icon>
                  <v-tooltip activator="parent" location="top">拖动排序</v-tooltip>
                </div>
                <v-list-item-title class="text-h6">{{ displayGroupName(group) }}</v-list-item-title>
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
              <!-- 订阅链接类型的分组显示刷新按钮 -->
              <v-btn
                v-if="group.subscription_url || group.subscription_url_clash"
                icon="mdi-sync"
                size="small"
                color="info"
                variant="text"
                :disabled="operationBusy"
                :loading="refreshingGroup === displayGroupName(group)"
                @click="refreshSubscription(group)"
              >
                <v-icon />
                <v-tooltip activator="parent" location="top">刷新订阅</v-tooltip>
              </v-btn>
              <v-btn
                icon="mdi-qrcode"
                size="small"
                color="success"
                variant="text"
                :disabled="operationBusy"
                @click="showGroupQrCode(group)"
              >
                <v-icon />
                <v-tooltip activator="parent" location="top">订阅</v-tooltip>
              </v-btn>
            </template>
            <v-list-item-subtitle class="mt-2">
              <v-chip
                v-if="getAutoUpdateFailureLabel(group)"
                size="small"
                color="error"
                class="mr-1 mb-1"
              >
                <v-icon start size="x-small">mdi-alert-circle</v-icon>
                {{ getAutoUpdateFailureLabel(group) }}
                <v-tooltip
                  v-if="getAutoUpdateError(group)"
                  activator="parent"
                  location="top"
                >
                  {{ getAutoUpdateError(group) }}
                </v-tooltip>
              </v-chip>
              <v-chip
                v-if="group.subscription_url"
                size="small"
                color="info"
                class="mr-1 mb-1"
              >
                <v-icon start size="x-small">mdi-link</v-icon>
                JSON
              </v-chip>
              <v-chip
                v-if="group.subscription_url_clash"
                size="small"
                color="deep-purple-accent-2"
                class="mr-1 mb-1"
              >
                <v-icon start size="x-small">mdi-link-variant</v-icon>
                Clash
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

  <!-- 添加/编辑分组对话框 -->
  <v-dialog v-model="editDialog" width="500" max-width="90vw" max-height="90vh" :persistent="saving">
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
              label="名称"
              variant="outlined"
              density="compact"
              hide-details
              :disabled="saving"
            ></v-text-field>
          </v-col>
        </v-row>
        <v-row class="mt-4">
          <v-col cols="12">
            <v-select
              v-model="editingGroup.outbounds"
              :items="outboundOptions"
              label="订阅出站"
              variant="outlined"
              density="compact"
              multiple
              chips
              closable-chips
              hide-details
              :disabled="saving"
            ></v-select>
          </v-col>
        </v-row>
        <v-row class="mt-4">
          <v-col cols="12">
            <v-text-field
              v-model="editingGroup.subscription_url"
              label="JSON 订阅链接 (可选)"
              placeholder="输入 JSON 订阅链接用于导入节点"
              variant="outlined"
              density="compact"
              hide-details
              clearable
              :disabled="saving"
            ></v-text-field>
          </v-col>
        </v-row>
        <v-row class="mt-2">
          <v-col cols="12">
            <v-text-field
              v-model="editingGroup.subscription_url_clash"
              label="Clash 订阅链接 (可选)"
              placeholder="输入 Clash 订阅链接用于导入 Clash 原始参数"
              variant="outlined"
              density="compact"
              hide-details
              clearable
              :disabled="saving"
            ></v-text-field>
          </v-col>
        </v-row>
        <v-row class="mt-2" v-if="editingGroup.subscription_url || editingGroup.subscription_url_clash">
          <v-col cols="12">
            <v-switch
              v-model="editingGroup.allow_insecure"
              label="允许不安全（跳过证书验证）"
              color="warning"
              density="compact"
              hide-details
              :disabled="saving"
            ></v-switch>
          </v-col>
        </v-row>
      </v-card-text>
      <v-divider></v-divider>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="primary" variant="outlined" :disabled="saving" @click="editDialog = false">
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn color="primary" variant="flat" :loading="saving" :disabled="saving" @click="saveGroup">
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- 删除确认对话框 -->
  <v-dialog v-model="deleteDialog" width="400" max-width="90vw" :persistent="deletingGroupId !== null">
    <v-card class="rounded-lg">
      <v-card-title>{{ $t('actions.del') }}</v-card-title>
      <v-divider></v-divider>
      <v-card-text>{{ $t('confirm') }}</v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="success" variant="outlined" :disabled="deletingGroupId !== null" @click="closeDeleteDialog">
          {{ $t('no') }}
        </v-btn>
        <v-btn color="error" variant="outlined" :loading="deletingGroupId !== null" :disabled="deletingGroupId !== null" @click="deleteGroup">
          {{ $t('yes') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>

  <!-- 分组二维码对话框 -->
  <SubGroupQrCode
    v-model="qrcodeDialog"
    :visible="qrcodeDialog"
    :groupName="qrcodeGroupName"
    @close="closeQrCode"
  />

  <!-- 刷新结果对话框 -->
  <v-dialog v-model="refreshResultDialog" width="600" max-width="90vw" max-height="90vh" :persistent="operationBusy">
    <v-card class="rounded-lg">
      <v-card-title>刷新订阅结果</v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 16px; max-height: calc(90vh - 152px); overflow-y: auto;">
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
            <div v-if="refreshResult.added.length > refreshResultPreviewLimit" class="text-caption mt-1">仅显示前 {{ refreshResultPreviewLimit }} 个节点标签</div>
          </v-alert>
          <v-alert v-if="refreshResult.removed.length > 0" type="warning" variant="tonal" class="mb-2">
            <div class="font-weight-bold mb-1">删除节点 ({{ refreshResult.removed.length }}):</div>
            <v-chip v-for="node in previewRefreshNodes(refreshResult.removed)" :key="node" size="small" class="mr-1 mb-1" color="warning">
              {{ node }}
            </v-chip>
            <div v-if="refreshResult.removed.length > refreshResultPreviewLimit" class="text-caption mt-1">仅显示前 {{ refreshResultPreviewLimit }} 个节点标签</div>
          </v-alert>
          <v-alert v-if="refreshResult.updated.length > 0" type="info" variant="tonal" class="mb-2">
            <div class="font-weight-bold mb-1">更新节点 ({{ refreshResult.updated.length }}):</div>
            <v-chip v-for="node in previewRefreshNodes(refreshResult.updated)" :key="node" size="small" class="mr-1 mb-1" color="info">
              {{ node }}
            </v-chip>
            <div v-if="refreshResult.updated.length > refreshResultPreviewLimit" class="text-caption mt-1">仅显示前 {{ refreshResultPreviewLimit }} 个节点标签</div>
          </v-alert>
          <v-alert v-if="refreshResult.added.length === 0 && refreshResult.removed.length === 0 && refreshResult.updated.length === 0" type="info" variant="tonal">
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
import { ref, computed, watch } from 'vue'
import { push } from 'notivue'
import Data from '@/store/modules/data'
import { Outbound } from '@/types/outbounds'
import { SubGroup, createSubGroup } from '@/types/subgroups'
import SubGroupQrCode from './SubGroupQrCode.vue'
import HttpUtils from '@/plugins/httputil'

const props = defineProps<{
  visible: boolean
}>()

const emit = defineEmits(['close', 'update:modelValue'])

// Data.setNewData synchronizes shared subscription collections to the Mihomo
// store. Keep every group mutation on the same update boundary.
const applySubscriptionData = (data: any) => {
  Data().setNewData(data)
}

// 使用本地 ref 控制对话框显示，避免直接修改 props
const dialogVisible = ref(props.visible)

// 监听 props 变化并同步本地状态
watch(() => props.visible, (newVal) => {
  dialogVisible.value = newVal
  if (newVal) {
    void loadGroups()
    void loadAutoUpdateSettings()
  } else {
    groupsRequestSequence.value += 1
    autoUpdateRequestSequence.value += 1
  }
})

// 监听本地状态变化并向父组件同步
watch(dialogVisible, (newVal) => {
  if (!newVal) {
    emit('close')
    emit('update:modelValue', false)
  }
})

// 分组列表
const groups = ref<SubGroup[]>([])
const groupsLoading = ref(false)
const groupsLoadFailed = ref(false)
const groupsRequestSequence = ref(0)
const draggedGroupIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)
const reordering = ref(false)
const autoUpdateEnabled = ref(false)
const autoUpdateIntervalInput = ref('5')
const autoUpdateSaving = ref(false)
const autoUpdateLoading = ref(false)
const autoUpdateRequestSequence = ref(0)

// 编辑对话框
const editDialog = ref(false)
const editingGroupId = ref<number | null>(null)
const editingGroup = ref<SubGroup>(createSubGroup())
const saving = ref(false)

// 删除对话框
const deleteDialog = ref(false)
const deleteTargetGroupId = ref<number | null>(null)
const deletingGroupId = ref<number | null>(null)

// 二维码对话框
const qrcodeDialog = ref(false)
const qrcodeGroupName = ref('')

// 刷新订阅状态
const refreshingGroup = ref('')
const refreshResultDialog = ref(false)
const refreshResultPreviewLimit = 80
type SubscriptionRefreshResult = {
  added: string[]
  removed: string[]
  updated: string[]
  error: string
  warning: string
}
const refreshResult = ref<SubscriptionRefreshResult>({
  added: [],
  removed: [],
  updated: [],
  error: '',
  warning: ''
})

const operationBusy = computed(() => {
  return saving.value
    || deletingGroupId.value !== null
    || reordering.value
    || autoUpdateSaving.value
    || refreshingGroup.value !== ''
})

const normalizeText = (value: unknown): string => {
  if (value === null || value === undefined) return ''
  const text = String(value).trim()
  return text === '' || text.toLowerCase() === 'null' || text.toLowerCase() === 'undefined' ? '' : text
}

const normalizeSelectedOutbounds = (raw: unknown): string[] => {
  if (typeof raw === 'string') {
    try {
      raw = JSON.parse(raw)
    } catch {
      return []
    }
  }
  if (!Array.isArray(raw)) return []
  const normalized: string[] = []
  const seen = new Set<string>()
  for (const item of raw) {
    const tag = normalizeText(item)
    if (!tag || seen.has(tag)) continue
    seen.add(tag)
    normalized.push(tag)
  }
  return normalized
}

const normalizeSubGroup = (raw: unknown): SubGroup => {
  const source = raw && typeof raw === 'object' && !Array.isArray(raw) ? raw as Record<string, unknown> : {}
  const id = Number(source.id)
  return createSubGroup({
    ...source,
    id: Number.isInteger(id) && id > 0 ? id : 0,
    name: normalizeText(source.name),
    outbounds: normalizeSelectedOutbounds(source.outbounds),
    subscription_url: normalizeText(source.subscription_url),
    subscription_url_clash: normalizeText(source.subscription_url_clash),
    allow_insecure: source.allow_insecure === true,
    auto_update_failed_sources: typeof source.auto_update_failed_sources === 'string'
      || Array.isArray(source.auto_update_failed_sources)
      ? source.auto_update_failed_sources as string | string[]
      : '',
    auto_update_error: normalizeText(source.auto_update_error),
  })
}

const normalizeSubGroups = (raw: unknown): SubGroup[] => {
  if (!Array.isArray(raw)) return []
  return raw.map(normalizeSubGroup)
}

const displayGroupName = (group: SubGroup): string => normalizeText(group?.name) || '-'

const outboundOptions = computed(() => {
  const suboutbounds = Data().suboutbounds as Outbound[]
  const seen = new Set<string>()
  return (Array.isArray(suboutbounds) ? suboutbounds : []).reduce<Array<{ title: string; value: string }>>((items, outbound: Outbound) => {
    const tag = normalizeText(outbound?.tag)
    if (!tag || seen.has(tag)) return items
    seen.add(tag)
    items.push({ title: tag, value: tag })
    return items
  }, [])
})

const sameOutboundOrder = (left: unknown, right: unknown): boolean => {
  const a = Array.isArray(left) ? left : []
  const b = Array.isArray(right) ? right : []
  if (a.length !== b.length) {
    return false
  }
  return a.every((item, index) => String(item ?? '') === String(b[index] ?? ''))
}

const applyEditingGroupOutboundOrder = () => {
  const normalized = normalizeSelectedOutbounds(editingGroup.value.outbounds)
  if (sameOutboundOrder(editingGroup.value.outbounds, normalized)) {
    return
  }
  editingGroup.value = {
    ...editingGroup.value,
    outbounds: normalized as any
  }
}

watch(() => Data().subgroups, (newGroups) => {
  if (dialogVisible.value && Array.isArray(newGroups)) {
    groups.value = normalizeSubGroups(newGroups)
    groupsLoadFailed.value = false
  }
})

watch(() => editingGroup.value.outbounds, () => {
  applyEditingGroupOutboundOrder()
}, { deep: true })

watch(() => [editingGroup.value.subscription_url, editingGroup.value.subscription_url_clash], ([jsonUrl, clashUrl]) => {
  if (!String(jsonUrl ?? '').trim() && !String(clashUrl ?? '').trim()) {
    editingGroup.value.allow_insecure = false
  }
})

watch(() => Data().suboutbounds, () => {
  if (!editDialog.value) {
    return
  }
  applyEditingGroupOutboundOrder()
}, { deep: true })

const loadGroups = async () => {
  const requestSequence = ++groupsRequestSequence.value
  groupsLoading.value = true
  try {
    const data = await Data().loadSubGroups()
    if (requestSequence !== groupsRequestSequence.value || !dialogVisible.value) return false
    if (Array.isArray(data)) {
      groups.value = normalizeSubGroups(data)
      groupsLoadFailed.value = false
      return true
    }
  } catch {
    // Keep the last known list visible when a refresh request fails.
  } finally {
    if (requestSequence === groupsRequestSequence.value) groupsLoading.value = false
  }
  if (requestSequence !== groupsRequestSequence.value || !dialogVisible.value) return false
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
      object: 'subgroups',
      action: 'reorder',
      data: JSON.stringify({ ids }, null, 2)
    })
    if (msg.success && msg.obj) {
      applySubscriptionData(msg.obj)
      if (Array.isArray(msg.obj.subgroups)) {
        groups.value = msg.obj.subgroups as SubGroup[]
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

const applyAutoUpdateInfo = (info: any) => {
  autoUpdateEnabled.value = info?.enabled === true
  autoUpdateIntervalInput.value = String(info?.intervalMinutes || 5)
}

const loadAutoUpdateSettings = async () => {
  const requestSequence = ++autoUpdateRequestSequence.value
  autoUpdateLoading.value = true
  try {
    const msg = await HttpUtils.get('api/subgroup-auto-update-info')
    if (requestSequence !== autoUpdateRequestSequence.value || !dialogVisible.value) return
    if (msg.success && msg.obj) {
      applyAutoUpdateInfo(msg.obj)
    } else {
      push.warning({ title: '自动更新', message: '自动更新设置加载失败，请稍后重试。' })
    }
  } catch {
    if (requestSequence === autoUpdateRequestSequence.value && dialogVisible.value) {
      push.warning({ title: '自动更新', message: '自动更新设置加载失败，请稍后重试。' })
    }
  } finally {
    if (requestSequence === autoUpdateRequestSequence.value) autoUpdateLoading.value = false
  }
}

const normalizeAutoUpdateIntervalMinutes = (raw: string): number | null => {
  const trimmed = raw.trim().toLowerCase().replace(/m$/, '').trim()
  if (!/^\d+$/.test(trimmed)) {
    return null
  }
  const value = Number(trimmed)
  if (!Number.isInteger(value) || value <= 0) {
    return null
  }
  return value
}

const saveAutoUpdateSettings = async (): Promise<boolean> => {
  if (operationBusy.value) return false
  const intervalMinutes = normalizeAutoUpdateIntervalMinutes(autoUpdateIntervalInput.value)
  if (intervalMinutes == null) {
    push.error({
      title: '自动更新',
      message: '时间间隔必须是正整数分钟，例如 5 或 5m'
    })
    return false
  }

  autoUpdateSaving.value = true
  try {
    const msg = await HttpUtils.post('api/subgroup-auto-update-settings', {
      enabled: autoUpdateEnabled.value ? 'true' : 'false',
      interval: String(intervalMinutes)
    })
    if (msg.success && msg.obj) {
      applyAutoUpdateInfo(msg.obj)
      push.success({
        title: '自动更新',
        message: '自动更新设置已保存'
      })
      return true
    }
    return false
  } catch {
    return false
  } finally {
    autoUpdateSaving.value = false
  }
}

const handleAutoUpdateToggle = async () => {
  if (autoUpdateLoading.value || operationBusy.value) return
  const success = await saveAutoUpdateSettings()
  if (!success) {
    await loadAutoUpdateSettings()
  }
}

const handleAutoUpdateIntervalCommit = async () => {
  if (autoUpdateLoading.value || operationBusy.value) return
  const success = await saveAutoUpdateSettings()
  if (!success) {
    await loadAutoUpdateSettings()
  }
}

const getGroupOutbounds = (group: SubGroup): string[] => {
  return normalizeSelectedOutbounds(group?.outbounds)
}

const parseFailedSources = (group: SubGroup): string[] => {
  const raw = group.auto_update_failed_sources
  if (Array.isArray(raw)) {
    return raw.map((item) => normalizeText(item).toLowerCase()).filter(Boolean)
  }
  if (typeof raw !== 'string' || !raw.trim()) {
    return []
  }
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return []
    }
    return parsed.map((item) => normalizeText(item).toLowerCase()).filter(Boolean)
  } catch {
    return []
  }
}

const getFailureSourceLabel = (source: string): string => {
  switch (source) {
    case 'json':
      return 'JSON'
    case 'clash':
      return 'Clash'
    default:
      return source.toUpperCase()
  }
}

const getAutoUpdateFailureLabel = (group: SubGroup): string => {
  const failedSources = parseFailedSources(group)
  if (failedSources.length === 0) {
    return ''
  }
  return `自动更新失败 · ${failedSources.map(getFailureSourceLabel).join(' / ')}`
}

const getAutoUpdateError = (group: SubGroup): string => {
  return normalizeText(group?.auto_update_error)
}

const showAddDialog = () => {
  if (operationBusy.value) return
  editingGroupId.value = null
  editingGroup.value = createSubGroup()
  editDialog.value = true
}

const showEditDialog = (index: number) => {
  if (operationBusy.value) return
  const group = groups.value[index]
  const id = Number(group?.id)
  if (!group || !Number.isInteger(id) || id <= 0) return
  editingGroupId.value = id
  const outbounds = normalizeSelectedOutbounds(getGroupOutbounds(group))
  editingGroup.value = normalizeSubGroup({ ...group, outbounds })
  editDialog.value = true
}

const saveGroup = async () => {
  if (operationBusy.value) return
  editingGroup.value = normalizeSubGroup(editingGroup.value)
  if (!editingGroup.value.name) {
    return
  }

  saving.value = true

  try {
    const normalizedOutbounds = normalizeSelectedOutbounds(editingGroup.value.outbounds)
    const jsonUrl = String(editingGroup.value.subscription_url ?? '').trim()
    const clashUrl = String(editingGroup.value.subscription_url_clash ?? '').trim()
    const hasSubscriptionSource = jsonUrl !== '' || clashUrl !== ''
    const allowInsecure = hasSubscriptionSource && editingGroup.value.allow_insecure === true
    editingGroup.value = {
      ...editingGroup.value,
      outbounds: normalizedOutbounds as any
    }
    const groupData = {
      ...editingGroup.value,
      outbounds: JSON.stringify(normalizedOutbounds),
      subscription_url: jsonUrl,
      subscription_url_clash: clashUrl,
      allow_insecure: allowInsecure
    }

    const action = editingGroupId.value === null ? 'new' : 'edit'
    const success = await Data().save('subgroups', action, groupData)
    
    if (success) {
      // 如果有订阅链接，触发抓取订阅
      if (hasSubscriptionSource) {
        const result = await fetchAndSaveSubscription(
          editingGroup.value.name,
          jsonUrl,
          clashUrl,
          allowInsecure
        )
        if (!(result.success || result.obj?.committed === true)) {
          push.warning({
            title: '订阅管理',
            duration: 7000,
            message: result.msg || '分组已保存，但订阅节点未能刷新',
          })
        }
      }
      editDialog.value = false
      // 重新加载分组列表
      await loadGroups()
    }
  } finally {
    saving.value = false
  }
}

// 获取并保存订阅
const fetchAndSaveSubscription = async (groupName: string, jsonUrl: string, clashUrl: string, allowInsecure: boolean) => {
  const formData = new FormData()
  formData.append('group_name', groupName)
  formData.append('json_url', jsonUrl || '')
  formData.append('clash_url', clashUrl || '')
  formData.append('allow_insecure', String(allowInsecure))

  const msg = await HttpUtils.post('api/fetchSubscription', formData)
  if (msg.obj && (msg.success || msg.obj.committed === true)) {
    applySubscriptionData(msg.obj)
  }
  return msg
}

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
  return {
    added: normalizeRefreshNodes(result.added),
    removed: normalizeRefreshNodes(result.removed),
    updated: normalizeRefreshNodes(result.updated),
    error: typeof result.error === 'string' && result.error.trim() !== '' ? result.error.trim() : fallbackError,
    warning: typeof result.warning === 'string' ? result.warning.trim() : '',
  }
}

const previewRefreshNodes = (nodes: unknown): string[] => normalizeRefreshNodes(nodes).slice(0, refreshResultPreviewLimit)

// 刷新订阅
const refreshSubscription = async (group: SubGroup) => {
  const groupName = normalizeText(group?.name)
  const jsonUrl = normalizeText(group?.subscription_url)
  const clashUrl = normalizeText(group?.subscription_url_clash)
  if (operationBusy.value || !groupName || (!jsonUrl && !clashUrl)) return

  refreshingGroup.value = groupName

  try {
    const formData = new FormData()
    formData.append('group_name', groupName)
    formData.append('json_url', jsonUrl)
    formData.append('clash_url', clashUrl)
    formData.append('allow_insecure', String(group.allow_insecure === true))

    const msg = await HttpUtils.post('api/refreshSubscription', formData)
    
    if (msg.obj && (msg.success || msg.obj.committed === true)) {
      if (Object.hasOwn(msg.obj, 'suboutbounds') || Object.hasOwn(msg.obj, 'subgroups')) {
        applySubscriptionData(msg.obj)
      }
      const result = normalizeRefreshResult(msg.obj.result)
      if (msg.obj.committed === true) result.warning = msg.msg || '订阅数据已保存，但运行配置未更新'
      refreshResult.value = result
    } else {
      refreshResult.value = normalizeRefreshResult(null, msg.msg || '刷新订阅失败')
    }
    refreshResultDialog.value = true

    // 重新加载分组列表
    await loadGroups()
  } catch (e: any) {
    refreshResult.value = normalizeRefreshResult(null, e.message || '刷新订阅失败')
    refreshResultDialog.value = true
  } finally {
    refreshingGroup.value = ''
  }
}

const confirmDelete = (id: number) => {
  if (operationBusy.value) return
  deleteTargetGroupId.value = id
  deleteDialog.value = true
}

const closeDeleteDialog = () => {
  if (deletingGroupId.value !== null) return
  deleteTargetGroupId.value = null
  deleteDialog.value = false
}

watch(deleteDialog, (visible) => {
  if (!visible && deletingGroupId.value === null) deleteTargetGroupId.value = null
})

const deleteGroup = async () => {
  const id = deleteTargetGroupId.value
  if (id == null) return
  const group = groups.value.find((item) => Number(item.id) === id)
  const groupName = normalizeText(group?.name)
  if (!groupName) {
    deleteTargetGroupId.value = null
    deleteDialog.value = false
    return
  }
  deletingGroupId.value = id
  try {
    const success = await Data().save('subgroups', 'del', groupName)
    if (success) {
      deleteDialog.value = false
      deleteTargetGroupId.value = null
      await loadGroups()
    }
  } finally {
    deletingGroupId.value = null
  }
}

const showGroupQrCode = (group: SubGroup) => {
  const groupName = normalizeText(group?.name)
  if (operationBusy.value || !groupName) return
  qrcodeGroupName.value = groupName
  qrcodeDialog.value = true
}

const closeQrCode = () => {
  qrcodeDialog.value = false
}
</script>

<style scoped>
.subgroup-prepend {
  min-width: 0;
}

.subgroup-drag-handle {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  color: rgba(var(--v-theme-on-surface), 0.66);
  cursor: grab;
  user-select: none;
  transition: background-color 0.18s ease, color 0.18s ease;
}

.subgroup-drag-handle:hover {
  background: rgba(var(--v-theme-on-surface), 0.08);
  color: rgba(var(--v-theme-on-surface), 0.9);
}

.subgroup-drag-handle:active {
  cursor: grabbing;
}

.subgroup-drag-handle--disabled {
  cursor: default;
  opacity: 0.45;
}

.subgroup-sort-item {
  transition: border-color 0.18s ease, background-color 0.18s ease, opacity 0.18s ease;
}

.subgroup-sort-item--active {
  border-color: rgb(var(--v-theme-primary)) !important;
  background: rgba(var(--v-theme-primary), 0.08);
}

.subgroup-sort-item--dragging {
  opacity: 0.72;
}
</style>
