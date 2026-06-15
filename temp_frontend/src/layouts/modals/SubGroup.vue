<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="800" max-width="90vw">
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row align="center">
          <v-col cols="auto">
            {{ $t('actions.group') }}
          </v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <div class="d-flex align-center" style="gap: 12px;">
              <v-switch
                v-model="autoUpdateEnabled"
                label="自动更新"
                color="primary"
                density="compact"
                hide-details
                inset
                :loading="autoUpdateSaving"
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
                :disabled="!autoUpdateEnabled"
                :loading="autoUpdateSaving"
                @blur="handleAutoUpdateIntervalCommit"
                @keydown.enter.prevent="handleAutoUpdateIntervalCommit"
              ></v-text-field>
            </div>
          </v-col>
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
                  :class="{ 'subgroup-drag-handle--disabled': groups.length < 2 || reordering }"
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
              <!-- 订阅链接类型的分组显示刷新按钮 -->
              <v-btn
                v-if="group.subscription_url || group.subscription_url_clash"
                icon="mdi-sync"
                size="small"
                color="info"
                variant="text"
                :disabled="reordering"
                :loading="refreshingGroup === group.name"
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
                :disabled="reordering"
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
        <v-alert v-else type="info" variant="tonal">
          暂无分组，点击“添加”按钮创建新分组
        </v-alert>
      </v-card-text>
    </v-card>
  </v-dialog>

  <!-- 添加/编辑分组对话框 -->
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

  <!-- 删除确认对话框 -->
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

  <!-- 分组二维码对话框 -->
  <SubGroupQrCode
    v-model="qrcodeDialog"
    :visible="qrcodeDialog"
    :groupName="qrcodeGroupName"
    @close="closeQrCode"
  />

  <!-- 刷新结果对话框 -->
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

// 使用本地 ref 控制对话框显示，避免直接修改 props
const dialogVisible = ref(props.visible)

// 监听 props 变化并同步本地状态
watch(() => props.visible, (newVal) => {
  dialogVisible.value = newVal
  if (newVal) {
    void loadGroups()
    void loadAutoUpdateSettings()
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
const draggedGroupIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)
const reordering = ref(false)
const autoUpdateEnabled = ref(false)
const autoUpdateIntervalInput = ref('5')
const autoUpdateSaving = ref(false)

// 编辑对话框
const editDialog = ref(false)
const editingIndex = ref(-1)
const editingGroup = ref<SubGroup>(createSubGroup())
const saving = ref(false)

// 删除对话框
const deleteDialog = ref(false)
const deletingIndex = ref(-1)

// 二维码对话框
const qrcodeDialog = ref(false)
const qrcodeGroupName = ref('')

// 刷新订阅状态
const refreshingGroup = ref('')
const refreshResultDialog = ref(false)
const refreshResult = ref<{
  added: string[]
  removed: string[]
  updated: string[]
  error: string
}>({
  added: [],
  removed: [],
  updated: [],
  error: ''
})

const outboundOptions = computed(() => {
  const suboutbounds = Data().suboutbounds as Outbound[]
  return suboutbounds.map((o: Outbound) => ({
    title: o.tag,
    value: o.tag
  }))
})

const normalizeSelectedOutbounds = (raw: unknown): string[] => {
  if (!Array.isArray(raw)) {
    return []
  }

  const normalized: string[] = []
  const seen = new Set<string>()
  for (const item of raw) {
    const tag = String(item ?? '').trim()
    if (!tag || seen.has(tag)) {
      continue
    }
    seen.add(tag)
    normalized.push(tag)
  }
  return normalized
}

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
  if (dialogVisible.value) {
    groups.value = (newGroups ?? []) as SubGroup[]
  }
})

watch(() => editingGroup.value.outbounds, () => {
  applyEditingGroupOutboundOrder()
}, { deep: true })

watch(() => Data().suboutbounds, () => {
  if (!editDialog.value) {
    return
  }
  applyEditingGroupOutboundOrder()
}, { deep: true })

const loadGroups = async () => {
  // 从后端加载分组数据
  const data = await Data().loadSubGroups()
  if (data) {
    groups.value = data
  }
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
      object: 'subgroups',
      action: 'reorder',
      data: JSON.stringify({ ids }, null, 2)
    })
    if (msg.success && msg.obj) {
      Data().setNewData(msg.obj)
      groups.value = (msg.obj.subgroups ?? []) as SubGroup[]
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

const applyAutoUpdateInfo = (info: any) => {
  autoUpdateEnabled.value = info?.enabled === true
  autoUpdateIntervalInput.value = String(info?.intervalMinutes || 5)
}

const loadAutoUpdateSettings = async () => {
  const msg = await HttpUtils.get('api/subgroup-auto-update-info')
  if (msg.success && msg.obj) {
    applyAutoUpdateInfo(msg.obj)
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
  const success = await saveAutoUpdateSettings()
  if (!success) {
    await loadAutoUpdateSettings()
  }
}

const handleAutoUpdateIntervalCommit = async () => {
  const success = await saveAutoUpdateSettings()
  if (!success) {
    await loadAutoUpdateSettings()
  }
}

const getGroupOutbounds = (group: SubGroup): string[] => {
  // 如果 outbounds 是字符串，解析为数组
  if (typeof group.outbounds === 'string') {
    try {
      return JSON.parse(group.outbounds)
    } catch {
      return []
    }
  }
  return group.outbounds as unknown as string[]
}

const parseFailedSources = (group: SubGroup): string[] => {
  const raw = group.auto_update_failed_sources
  if (Array.isArray(raw)) {
    return raw.map((item) => String(item).trim().toLowerCase()).filter(Boolean)
  }
  if (typeof raw !== 'string' || !raw.trim()) {
    return []
  }
  try {
    const parsed = JSON.parse(raw)
    if (!Array.isArray(parsed)) {
      return []
    }
    return parsed.map((item) => String(item).trim().toLowerCase()).filter(Boolean)
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
  return String(group.auto_update_error || '').trim()
}

const showAddDialog = () => {
  editingIndex.value = -1
  editingGroup.value = createSubGroup()
  editDialog.value = true
}

const showEditDialog = (index: number) => {
  editingIndex.value = index
  const group = groups.value[index]
  const outbounds = normalizeSelectedOutbounds(getGroupOutbounds(group))
  editingGroup.value = {
    ...group,
    outbounds: outbounds as any,
    subscription_url: group.subscription_url || '',
    subscription_url_clash: group.subscription_url_clash || '',
    allow_insecure: group.allow_insecure || false
  }
  editDialog.value = true
}

const saveGroup = async () => {
  if (!editingGroup.value.name.trim()) {
    return
  }

  saving.value = true

  try {
    const normalizedOutbounds = normalizeSelectedOutbounds(editingGroup.value.outbounds)
    editingGroup.value = {
      ...editingGroup.value,
      outbounds: normalizedOutbounds as any
    }
    const groupData = {
      ...editingGroup.value,
      outbounds: JSON.stringify(normalizedOutbounds),
      subscription_url: editingGroup.value.subscription_url || '',
      subscription_url_clash: editingGroup.value.subscription_url_clash || '',
      allow_insecure: editingGroup.value.allow_insecure || false
    }

    const action = editingIndex.value === -1 ? 'new' : 'edit'
    const success = await Data().save('subgroups', action, groupData)
    
    if (success) {
      // 如果有订阅链接，触发抓取订阅
      if (editingGroup.value.subscription_url || editingGroup.value.subscription_url_clash) {
        try {
          await fetchAndSaveSubscription(
            editingGroup.value.name,
            editingGroup.value.subscription_url || '',
            editingGroup.value.subscription_url_clash || '',
            editingGroup.value.allow_insecure || false
          )
        } catch (e: any) {
          console.error('获取订阅失败:', e)
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
  if (msg.success && msg.obj) {
    Data().setNewData(msg.obj)
  }
  return msg
}

// 刷新订阅
const refreshSubscription = async (group: SubGroup) => {
  if (!group.subscription_url && !group.subscription_url_clash) return

  refreshingGroup.value = group.name

  try {
    const formData = new FormData()
    formData.append('group_name', group.name)
    formData.append('json_url', group.subscription_url || '')
    formData.append('clash_url', group.subscription_url_clash || '')
    formData.append('allow_insecure', String(group.allow_insecure || false))

    const msg = await HttpUtils.post('api/refreshSubscription', formData)
    
    if (msg.success && msg.obj) {
      if (Object.hasOwn(msg.obj, 'suboutbounds') || Object.hasOwn(msg.obj, 'subgroups')) {
        Data().setNewData(msg.obj)
      }
      refreshResult.value = msg.obj.result || msg.obj
    } else {
      refreshResult.value = {
        added: [],
        removed: [],
        updated: [],
        error: ''
      }
    }
    refreshResultDialog.value = true

    // 重新加载分组列表
    await loadGroups()
  } catch (e: any) {
    refreshResult.value = {
      added: [],
      removed: [],
      updated: [],
      error: e.message || '刷新订阅失败'
    }
    refreshResultDialog.value = true
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
  const success = await Data().save('subgroups', 'del', groupName)
  
  if (success) {
    deleteDialog.value = false
    // 重新加载分组列表
    await loadGroups()
  }
}

const showGroupQrCode = (group: SubGroup) => {
  qrcodeGroupName.value = group.name
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


