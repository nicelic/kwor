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
    <!-- 子模态弹窗组件 -->
    <SubOutboundVue 
      v-model="modal.visible"
      :visible="modal.visible"
      :id="modal.id"
      :data="modal.data"
      :tags="subOutboundTags"
      @close="closeModal"
    />
    <SubManagerQrCode
      v-model="qrcode.visible"
      :visible="qrcode.visible"
      :tag="qrcode.tag"
      @close="closeQrCode"
    />
    <Stats
      v-model="stats.visible"
      :visible="stats.visible"
      :resource="stats.resource"
      :tag="stats.tag"
      @close="closeStats"
    />
    <SubGroup
      v-model="groupModal.visible"
      :visible="groupModal.visible"
      @close="closeGroupModal"
    />

    <!-- 全局安全删除节点对话框 -->
    <v-dialog v-model="deleteDialog.visible" max-width="440" :persistent="subManagerWriteBusy">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center">
          <v-icon color="error" icon="mdi-alert-circle-outline" class="mr-2" />
          {{ $t('actions.del') }}
        </v-card-title>
        <v-divider></v-divider>
        <v-card-text>
          <div class="mb-2">
            确认删除订阅节点 <strong>“{{ deleteDialog.tag }}”</strong> 吗？
          </div>
          <v-alert v-if="deleteDialog.isManaged" type="warning" variant="tonal" density="compact" class="mt-2 text-caption">
            注意：该节点由“用户管理”自动同步托管。删除此节点将在来源入站写入阻止记录，并关闭对应用户的自动同步。
          </v-alert>
          <div v-else class="text-caption text-medium-emphasis mt-2">
            此操作无法撤销。
          </div>
        </v-card-text>
        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn color="success" variant="outlined" :disabled="subManagerWriteBusy" @click="deleteDialog.visible = false">
            {{ $t('no') }}
          </v-btn>
          <v-btn color="error" variant="flat" :loading="subManagerWriteBusy" :disabled="subManagerWriteBusy" @click="delSubOutbound">
            {{ $t('yes') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 清空订阅管理对话框 -->
    <v-dialog v-model="showClearDialog" max-width="460" :persistent="clearingSubManager">
      <v-card rounded="lg">
        <v-card-title class="d-flex align-center">
          <v-icon color="error" icon="mdi-delete-sweep" class="mr-2" />
          清空订阅管理
        </v-card-title>
        <v-divider></v-divider>
        <v-card-text>
          <v-alert type="warning" variant="tonal" class="mb-3">
            此操作会清空订阅管理的全部节点和卡片，并清空所有分组内节点引用。
          </v-alert>
          <div class="text-body-2">分组名称、订阅源链接及自动更新设置将被保留。</div>
        </v-card-text>
        <v-card-actions>
          <v-spacer></v-spacer>
          <v-btn color="success" variant="outlined" :disabled="clearingSubManager" @click="showClearDialog = false">
            {{ $t('no') }}
          </v-btn>
          <v-btn color="error" variant="flat" :loading="clearingSubManager" :disabled="clearingSubManager" @click="clearSubManager">
            {{ $t('yes') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- 顶部资产概览与统计胶囊栏 -->
    <v-row dense class="mb-3">
      <v-col cols="12" class="d-flex align-center flex-wrap ga-2">
        <v-chip color="primary" variant="tonal" prepend-icon="mdi-cube-outline" class="font-weight-medium">
          节点总数: {{ totalCount }}
        </v-chip>
        <v-chip 
          :color="onlineCount > 0 ? 'success' : 'grey'" 
          variant="tonal" 
          prepend-icon="mdi-broadcast" 
          class="font-weight-medium cursor-pointer"
          @click="toggleOnlineQuickFilter"
        >
          在线: {{ onlineCount }}
        </v-chip>
        <v-chip color="secondary" variant="tonal" prepend-icon="mdi-folder-multiple-outline" class="font-weight-medium cursor-pointer" @click="showGroupModal">
          分组: {{ groupCount }}
        </v-chip>
      </v-col>
    </v-row>

    <!-- 综合控制工具栏（搜索、筛选、双模切换与操作） -->
    <v-card rounded="lg" elevation="1" border class="mb-4 pa-3">
      <v-row align="center" dense>
        <!-- 搜索框 -->
        <v-col cols="12" sm="5" md="4" lg="3">
          <v-text-field
            v-model="searchQuery"
            density="compact"
            variant="outlined"
            placeholder="搜索名称 / 地址 / 端口 / 协议"
            prepend-inner-icon="mdi-magnify"
            hide-details
            clearable
          />
        </v-col>

        <!-- 协议筛选 -->
        <v-col cols="6" sm="3" md="2" lg="2">
          <v-select
            v-model="typeFilter"
            :items="availableTypes"
            density="compact"
            variant="outlined"
            label="协议类型"
            hide-details
          />
        </v-col>

        <!-- 状态筛选 -->
        <v-col cols="6" sm="4" md="2" lg="2">
          <v-select
            v-model="onlineFilter"
            :items="onlineOptions"
            density="compact"
            variant="outlined"
            label="在线状态"
            hide-details
          />
        </v-col>

        <!-- 右侧操作与视图切换 -->
        <v-col cols="12" md="4" lg="5" class="d-flex align-center justify-end flex-wrap ga-2 mt-2 mt-md-0">
          <v-btn-toggle
            v-model="viewMode"
            mandatory
            density="compact"
            color="primary"
            variant="outlined"
            @update:model-value="saveViewPreference"
          >
            <v-btn value="card" size="small" prepend-icon="mdi-view-grid">
              卡片
            </v-btn>
            <v-btn value="table" size="small" prepend-icon="mdi-view-list">
              表格
            </v-btn>
          </v-btn-toggle>

          <v-btn
            color="primary"
            prepend-icon="mdi-plus"
            :disabled="subManagerWriteBusy"
            @click="showModal(0)"
          >
            {{ $t('actions.add') }}
          </v-btn>

          <v-btn
            color="secondary"
            variant="tonal"
            prepend-icon="mdi-folder-outline"
            :disabled="subManagerWriteBusy"
            @click="showGroupModal"
          >
            {{ $t('actions.group') }}
          </v-btn>

          <!-- 更多危险/全局操作菜单 -->
          <v-menu location="bottom end">
            <template v-slot:activator="{ props: menuProps }">
              <v-btn
                v-bind="menuProps"
                icon="mdi-dots-vertical"
                variant="text"
                density="comfortable"
                :disabled="subManagerWriteBusy"
              />
            </template>
            <v-list density="compact" min-width="160">
              <v-list-item
                prepend-icon="mdi-refresh"
                title="刷新数据"
                @click="initialize"
              />
              <v-divider class="my-1"></v-divider>
              <v-list-item
                prepend-icon="mdi-delete-sweep"
                title="清空订阅管理"
                class="text-error"
                @click="showClearDialog = true"
              />
            </v-list>
          </v-menu>
        </v-col>
      </v-row>
    </v-card>

    <!-- 空状态提示 -->
    <v-card v-if="subOutbounds.length === 0" rounded="lg" border class="pa-8 text-center">
      <v-icon size="56" color="grey-lighten-1" icon="mdi-bookmark-off-outline" class="mb-3" />
      <div class="text-h6 text-medium-emphasis mb-2">暂无订阅节点</div>
      <div class="text-body-2 text-medium-emphasis mb-4">
        您可以手动添加订阅节点，或者进入“分组”导入外部订阅与配置自动更新。
      </div>
      <div class="d-flex justify-center ga-3">
        <v-btn color="primary" prepend-icon="mdi-plus" @click="showModal(0)">
          {{ $t('actions.add') }}
        </v-btn>
        <v-btn color="secondary" variant="outlined" prepend-icon="mdi-folder-outline" @click="showGroupModal">
          {{ $t('actions.group') }}
        </v-btn>
      </div>
    </v-card>

    <!-- 筛选无匹配提示 -->
    <v-card v-else-if="filteredSubOutbounds.length === 0" rounded="lg" border class="pa-6 text-center">
      <v-icon size="40" color="warning" icon="mdi-filter-off-outline" class="mb-2" />
      <div class="text-subtitle-1 mb-2">未找到符合条件的订阅节点</div>
      <v-btn color="primary" variant="outlined" size="small" @click="resetFilters">
        重置筛选条件
      </v-btn>
    </v-card>

    <!-- 视图 A：精致卡片视图 -->
    <v-row v-else-if="viewMode === 'card'">
      <v-col
        v-for="(item, index) in filteredSubOutbounds"
        :key="item.id || `suboutbound-${index}`"
        cols="12"
        sm="6"
        md="4"
        lg="3"
      >
        <v-card rounded="xl" elevation="3" border class="suboutbound-card h-100 d-flex flex-column">
          <!-- 卡片头部：Tag、协议Badge、在线状态 -->
          <v-card-item class="pb-2">
            <template v-slot:prepend>
              <v-avatar size="32" :color="getProtocolColor(item.type)" variant="tonal">
                <v-icon size="18" :icon="getProtocolIcon(item.type)" />
              </v-avatar>
            </template>
            <v-card-title class="text-subtitle-1 font-weight-bold d-flex align-center justify-space-between">
              <span class="text-truncate node-title-text" v-tooltip:top="displayValue(item.tag)">
                {{ displayValue(item.tag) }}
              </span>
              <v-btn
                icon="mdi-content-copy"
                size="x-small"
                variant="text"
                color="medium-emphasis"
                class="ml-1"
                @click.stop="copyText(item.tag, '节点名称')"
              >
                <v-icon size="14" />
                <v-tooltip activator="parent" location="top">复制标签</v-tooltip>
              </v-btn>
            </v-card-title>
            <v-card-subtitle class="d-flex align-center justify-space-between pt-1" style="border-bottom: none;">
              <v-chip size="x-small" :color="getProtocolColor(item.type)" variant="flat" label class="text-uppercase font-weight-bold">
                {{ displayValue(item.type) }}
              </v-chip>
              <div>
                <v-chip v-if="onlines.includes(normalizeText(item.tag))" density="comfortable" size="x-small" color="success" variant="flat">
                  {{ $t('online') }}
                </v-chip>
                <v-chip v-else density="comfortable" size="x-small" color="grey" variant="tonal">
                  离线
                </v-chip>
              </div>
            </v-card-subtitle>
          </v-card-item>

          <v-divider></v-divider>

          <!-- 卡片核心指标属性区 -->
          <v-card-text class="pt-3 pb-2 flex-grow-1">
            <div class="d-flex align-center justify-space-between mb-2">
              <span class="text-caption text-medium-emphasis">{{ $t('in.addr') }}</span>
              <span class="text-caption font-weight-medium text-truncate text-end ml-2" style="max-width: 70%;" v-tooltip:top="displayValue(item.server)">
                {{ displayValue(item.server) }}
                <v-btn
                  v-if="normalizeText(item.server)"
                  icon="mdi-content-copy"
                  size="x-small"
                  variant="text"
                  density="compact"
                  class="ml-1"
                  @click.stop="copyText(item.server, '服务器地址')"
                >
                  <v-icon size="12" />
                  <v-tooltip activator="parent" location="top">复制地址</v-tooltip>
                </v-btn>
              </span>
            </div>

            <div class="d-flex align-center justify-space-between mb-2">
              <span class="text-caption text-medium-emphasis">{{ $t('in.port') }}</span>
              <span class="text-caption font-weight-medium font-monospace">
                {{ formatServerPort(item) }}
              </span>
            </div>

            <div class="d-flex align-center justify-space-between mb-2">
              <span class="text-caption text-medium-emphasis">{{ $t('objects.tls') }}</span>
              <span>
                <v-chip size="x-small" :color="tlsColor(item)" variant="tonal" label>
                  {{ tlsStatus(item) }}
                </v-chip>
              </span>
            </div>

            <!-- SSH 协议定制折叠信息 -->
            <template v-if="isSSH(item)">
              <v-divider class="my-2"></v-divider>
              <div class="d-flex align-center justify-space-between">
                <span class="text-caption font-weight-bold text-blue-grey">SSH 凭据与算法</span>
                <v-btn
                  size="x-small"
                  variant="text"
                  :icon="sshExpanded[item.id] ? 'mdi-chevron-up' : 'mdi-chevron-down'"
                  @click="sshExpanded[item.id] = !sshExpanded[item.id]"
                />
              </div>
              <v-expand-transition>
                <div v-show="sshExpanded[item.id]" class="mt-2 text-caption bg-surface-variant rounded pa-2">
                  <div class="d-flex justify-space-between mb-1">
                    <span class="text-medium-emphasis">user:</span>
                    <span>{{ readSSHUsername(item) || '-' }}</span>
                  </div>
                  <div class="d-flex justify-space-between mb-1">
                    <span class="text-medium-emphasis">key:</span>
                    <span>{{ hasSSHPrivateKey(item) ? 'configured' : '-' }}</span>
                  </div>
                  <div class="d-flex justify-space-between mb-1">
                    <span class="text-medium-emphasis">host-key:</span>
                    <span class="text-truncate" style="max-width: 60%;" v-tooltip:top="formatSSHList(item, ['host_key', 'host-key'])">
                      {{ formatSSHList(item, ['host_key', 'host-key']) }}
                    </span>
                  </div>
                  <div class="d-flex justify-space-between">
                    <span class="text-medium-emphasis">cipher:</span>
                    <span class="text-truncate" style="max-width: 60%;" v-tooltip:top="formatSSHList(item, ['cipher'])">
                      {{ formatSSHList(item, ['cipher']) }}
                    </span>
                  </div>
                </div>
              </v-expand-transition>
            </template>
          </v-card-text>

          <v-divider></v-divider>

          <!-- 卡片操作区 -->
          <v-card-actions class="pa-2 d-flex justify-end ga-1">
            <v-tooltip location="top" :text="$t('actions.edit')">
              <template #activator="{ props: tooltipProps }">
                <v-btn
                  v-bind="tooltipProps"
                  icon="mdi-pencil"
                  size="small"
                  variant="text"
                  color="primary"
                  :disabled="subManagerWriteBusy || !isValidOutbound(item)"
                  @click="showModal(item.id)"
                />
              </template>
            </v-tooltip>

            <v-tooltip location="top" text="二维码">
              <template #activator="{ props: tooltipProps }">
                <v-btn
                  v-bind="tooltipProps"
                  icon="mdi-qrcode"
                  size="small"
                  variant="text"
                  color="info"
                  :disabled="subManagerWriteBusy || !hasOutboundTag(item)"
                  @click="showQrCode(item.tag)"
                />
              </template>
            </v-tooltip>

            <v-tooltip v-if="Data().enableTraffic" location="top" :text="$t('stats.graphTitle')">
              <template #activator="{ props: tooltipProps }">
                <v-btn
                  v-bind="tooltipProps"
                  icon="mdi-chart-line"
                  size="small"
                  variant="text"
                  color="success"
                  :disabled="subManagerWriteBusy || !hasOutboundTag(item)"
                  @click="showStats(item.tag)"
                />
              </template>
            </v-tooltip>

            <v-tooltip location="top" :text="$t('actions.del')">
              <template #activator="{ props: tooltipProps }">
                <v-btn
                  v-bind="tooltipProps"
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  :disabled="subManagerWriteBusy || !isValidOutbound(item)"
                  @click="confirmDeleteOutbound(item)"
                />
              </template>
            </v-tooltip>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>

    <!-- 视图 B：专业数据表格视图 -->
    <v-row v-else-if="viewMode === 'table'">
      <v-col cols="12">
        <v-data-table
          :headers="tableHeaders"
          :items="tableRows"
          :items-per-page="itemsPerPage"
          :items-per-page-options="[10, 25, 50, 100]"
          @update:items-per-page="itemsPerPage = $event"
          :mobile="smAndDown"
          fixed-header
          item-value="id"
          class="elevation-2 rounded-lg border"
        >
          <!-- 在线状态列 -->
          <template v-slot:item.online="{ item }">
            <div class="text-center">
              <v-chip
                v-if="onlines.includes(normalizeText(item.tag))"
                density="comfortable"
                size="small"
                color="success"
                variant="flat"
              >
                {{ $t('online') }}
              </v-chip>
              <span v-else class="text-medium-emphasis">-</span>
            </div>
          </template>

          <!-- 标签列 -->
          <template v-slot:item.tag="{ item }">
            <div class="font-weight-medium d-flex align-center">
              <span class="text-truncate" style="max-width: 260px;" v-tooltip:top="item.tag">
                {{ item.tag }}
              </span>
              <v-btn
                icon="mdi-content-copy"
                size="x-small"
                variant="text"
                density="compact"
                class="ml-1"
                @click.stop="copyText(item.tag, '节点标签')"
              >
                <v-icon size="12" />
                <v-tooltip activator="parent" location="top">复制标签</v-tooltip>
              </v-btn>
            </div>
          </template>

          <!-- 协议类型列 -->
          <template v-slot:item.type="{ item }">
            <v-chip size="small" :color="getProtocolColor(item.type)" variant="tonal" label class="font-weight-medium">
              {{ item.type }}
            </v-chip>
          </template>

          <!-- 地址列 -->
          <template v-slot:item.server="{ item }">
            <div class="d-flex align-center">
              <span class="text-truncate font-monospace" style="max-width: 220px;" v-tooltip:top="item.server">
                {{ item.server }}
              </span>
              <v-btn
                v-if="item.server !== '-'"
                icon="mdi-content-copy"
                size="x-small"
                variant="text"
                density="compact"
                class="ml-1"
                @click.stop="copyText(item.server, '服务器地址')"
              >
                <v-icon size="12" />
                <v-tooltip activator="parent" location="top">复制地址</v-tooltip>
              </v-btn>
            </div>
          </template>

          <!-- 端口列 -->
          <template v-slot:item.port="{ item }">
            <span class="font-monospace">{{ item.port }}</span>
          </template>

          <!-- TLS 列 -->
          <template v-slot:item.tls="{ item }">
            <v-chip size="x-small" :color="tlsColor(item.raw)" variant="tonal" label>
              {{ item.tls }}
            </v-chip>
          </template>

          <!-- 操作列 -->
          <template v-slot:item.actions="{ item }">
            <div class="d-flex align-center justify-end ga-1">
              <v-tooltip location="top" :text="$t('actions.edit')">
                <template #activator="{ props: tooltipProps }">
                  <v-btn
                    v-bind="tooltipProps"
                    icon="mdi-pencil"
                    size="small"
                    variant="text"
                    color="primary"
                    :disabled="subManagerWriteBusy || !isValidOutbound(item.raw)"
                    @click="showModal(item.id)"
                  />
                </template>
              </v-tooltip>

              <v-tooltip location="top" text="二维码">
                <template #activator="{ props: tooltipProps }">
                  <v-btn
                    v-bind="tooltipProps"
                    icon="mdi-qrcode"
                    size="small"
                    variant="text"
                    color="info"
                    :disabled="subManagerWriteBusy || !hasOutboundTag(item.raw)"
                    @click="showQrCode(item.tag)"
                  />
                </template>
              </v-tooltip>

              <v-tooltip v-if="Data().enableTraffic" location="top" :text="$t('stats.graphTitle')">
                <template #activator="{ props: tooltipProps }">
                  <v-btn
                    v-bind="tooltipProps"
                    icon="mdi-chart-line"
                    size="small"
                    variant="text"
                    color="success"
                    :disabled="subManagerWriteBusy || !hasOutboundTag(item.raw)"
                    @click="showStats(item.tag)"
                  />
                </template>
              </v-tooltip>

              <v-tooltip location="top" :text="$t('actions.del')">
                <template #activator="{ props: tooltipProps }">
                  <v-btn
                    v-bind="tooltipProps"
                    icon="mdi-delete"
                    size="small"
                    variant="text"
                    color="error"
                    :disabled="subManagerWriteBusy || !isValidOutbound(item.raw)"
                    @click="confirmDeleteOutbound(item.raw)"
                  />
                </template>
              </v-tooltip>
            </div>
          </template>
        </v-data-table>
      </v-col>
    </v-row>
  </template>
</template>

<script lang="ts" setup>
import Data from '@/store/modules/data'
import HttpUtils from '@/plugins/httputil'
import SubOutboundVue from '@/layouts/modals/SubOutbound.vue'
import SubManagerQrCode from '@/layouts/modals/SubManagerQrCode.vue'
import Stats from '@/layouts/modals/Stats.vue'
import SubGroup from '@/layouts/modals/SubGroup.vue'
import { Outbound } from '@/types/outbounds'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { formatServerPortDisplay } from '@/plugins/portRange'
import { push } from 'notivue'
import { i18n } from '@/locales'
import { useDisplay } from 'vuetify'

const { smAndDown } = useDisplay()
const store = Data()
const initializing = ref(true)
const loadFailed = ref(false)
let componentActive = true

// 视图模式：卡片模式 (card) 或表格模式 (table)，本地存储持久化
const viewMode = ref<'card' | 'table'>(
  typeof localStorage !== 'undefined' && localStorage.getItem('submanager_view_mode') === 'table'
    ? 'table'
    : 'card'
)

const saveViewPreference = (mode: 'card' | 'table') => {
  if (typeof localStorage !== 'undefined') {
    localStorage.setItem('submanager_view_mode', mode)
  }
}

// 搜索与过滤条件
const searchQuery = ref('')
const typeFilter = ref('all')
const onlineFilter = ref<'all' | 'online' | 'offline'>('all')
const itemsPerPage = ref(25)
const sshExpanded = ref<Record<number, boolean>>({})

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

const subOutbounds = computed((): Outbound[] => {
  return Array.isArray(store.suboutbounds)
    ? store.suboutbounds.filter((item: any): item is Outbound => !!item && typeof item === 'object' && !Array.isArray(item)) as Outbound[]
    : []
})

const subOutboundTags = computed((): string[] => {
  const tags = [
    ...(Array.isArray(store.suboutbounds) ? store.suboutbounds.map((o: Outbound) => normalizeText(o?.tag)) : []),
    ...(Array.isArray(store.endpoints) ? store.endpoints.map((e: any) => normalizeText(e?.tag)) : []),
  ]
  return [...new Set(tags.filter((tag): tag is string => tag !== ''))]
})

const onlines = computed((): string[] => {
  return Array.isArray(store.onlines?.outbound)
    ? store.onlines.outbound.map((tag) => normalizeText(tag)).filter((tag): tag is string => tag !== '')
    : []
})

// 宏观统计指标
const totalCount = computed(() => subOutbounds.value.length)
const onlineCount = computed(() => subOutbounds.value.filter((o) => onlines.value.includes(normalizeText(o.tag))).length)
const groupCount = computed(() => Array.isArray(store.subgroups) ? store.subgroups.length : 0)

const toggleOnlineQuickFilter = () => {
  onlineFilter.value = onlineFilter.value === 'online' ? 'all' : 'online'
}

// 可选协议列表（从现有数据中提取）
const availableTypes = computed((): Array<{ title: string; value: string }> => {
  const set = new Set<string>()
  for (const item of subOutbounds.value) {
    const t = normalizeText(item.type)
    if (t) set.add(t)
  }
  const sorted = Array.from(set).sort()
  return [
    { title: '全部协议', value: 'all' },
    ...sorted.map(t => ({ title: t, value: t }))
  ]
})

const onlineOptions = [
  { title: '全部状态', value: 'all' },
  { title: '仅在线', value: 'online' },
  { title: '仅离线', value: 'offline' },
]

// 过滤后的节点列表
const filteredSubOutbounds = computed((): Outbound[] => {
  let list = subOutbounds.value

  if (typeFilter.value && typeFilter.value !== 'all') {
    list = list.filter((item) => normalizeText(item.type).toLowerCase() === typeFilter.value.toLowerCase())
  }

  if (onlineFilter.value === 'online') {
    list = list.filter((item) => onlines.value.includes(normalizeText(item.tag)))
  } else if (onlineFilter.value === 'offline') {
    list = list.filter((item) => !onlines.value.includes(normalizeText(item.tag)))
  }

  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    list = list.filter((item) => {
      const tag = normalizeText(item.tag).toLowerCase()
      const server = normalizeText(item.server).toLowerCase()
      const port = formatServerPort(item).toLowerCase()
      const type = normalizeText(item.type).toLowerCase()
      return tag.includes(q) || server.includes(q) || port.includes(q) || type.includes(q)
    })
  }

  return list
})

const resetFilters = () => {
  searchQuery.value = ''
  typeFilter.value = 'all'
  onlineFilter.value = 'all'
}

// 表格列定义与行数据映射
const tableHeaders = computed(() => [
  { title: i18n.global.t('online') || '在线', key: 'online', sortable: false, width: '90px', align: 'center' as const },
  { title: '标签', key: 'tag', sortable: true, minWidth: '180px' },
  { title: i18n.global.t('type') || '类型', key: 'type', sortable: true, width: '130px' },
  { title: i18n.global.t('in.addr') || '地址', key: 'server', sortable: true, minWidth: '160px' },
  { title: i18n.global.t('in.port') || '端口', key: 'port', sortable: true, width: '100px' },
  { title: i18n.global.t('objects.tls') || 'TLS', key: 'tls', sortable: false, width: '90px', align: 'center' as const },
  { title: i18n.global.t('actions.actions') || '操作', key: 'actions', sortable: false, align: 'end' as const, width: '180px' },
])

const tableRows = computed(() => {
  return filteredSubOutbounds.value.map((item) => ({
    id: item.id,
    tag: displayValue(item.tag),
    type: displayValue(item.type),
    server: displayValue(item.server),
    port: formatServerPort(item),
    tls: tlsStatus(item),
    raw: item,
  }))
})

const formatServerPort = (item: any): string => {
  return displayValue(formatServerPortDisplay(item?.server_port, item?.server_ports))
}

const normalizeText = (value: unknown): string => {
  if (value === null || value === undefined) return ''
  const text = String(value).trim()
  return text === '' || text.toLowerCase() === 'null' || text.toLowerCase() === 'undefined' ? '' : text
}

const displayValue = (value: unknown, fallback = '-'): string => normalizeText(value) || fallback

const hasOutboundTag = (item: any): boolean => normalizeText(item?.tag) !== ''

const isValidOutbound = (item: any): boolean => Number.isInteger(Number(item?.id)) && Number(item.id) > 0 && hasOutboundTag(item)

const tlsStatus = (item: any): string => {
  if (!Object.hasOwn(item ?? {}, 'tls') || !item?.tls || typeof item.tls !== 'object') return '-'
  return i18n.global.t(item.tls.enabled === true ? 'enable' : 'disable')
}

const tlsColor = (item: any): string => {
  if (!Object.hasOwn(item ?? {}, 'tls') || !item?.tls || typeof item.tls !== 'object') return 'grey'
  return item.tls.enabled === true ? 'success' : 'grey'
}

const isSSH = (item: any): boolean => {
  return String(item?.type ?? '').trim().toLowerCase() === 'ssh'
}

const readSSHField = (item: any, keys: string[]): string => {
  if (!item || !Array.isArray(keys)) return ''
  for (const key of keys) {
    const value = item?.[key]
    if (typeof value === 'string' && value.trim().length > 0) {
      return value.trim()
    }
  }
  return ''
}

const readSSHList = (item: any, keys: string[]): string[] => {
  if (!item || !Array.isArray(keys)) return []
  for (const key of keys) {
    const value = item?.[key]
    if (Array.isArray(value)) {
      const list = value
        .map((entry) => String(entry ?? '').trim())
        .filter((entry) => entry.length > 0)
      if (list.length > 0) return list
    }
    if (typeof value === 'string' && value.trim().length > 0) {
      return value.split(/[\n,]+/).map((entry) => entry.trim()).filter((entry) => entry.length > 0)
    }
  }
  return []
}

const formatSSHList = (item: any, keys: string[]): string => {
  const list = readSSHList(item, keys)
  return list.length > 0 ? list.join(', ') : '-'
}

const readSSHUsername = (item: any): string => {
  return readSSHField(item, ['username', 'user'])
}

const hasSSHPrivateKey = (item: any): boolean => {
  return readSSHField(item, ['private_key', 'private-key', 'private_key_path']).length > 0
}

const getProtocolColor = (type: unknown): string => {
  const t = String(type || '').trim().toLowerCase()
  switch (t) {
    case 'shadowquic':
      return 'deep-purple'
    case 'vless':
      return 'teal'
    case 'vmess':
      return 'indigo'
    case 'trojan':
      return 'cyan'
    case 'shadowsocks':
      return 'purple'
    case 'hysteria':
    case 'hysteria2':
      return 'deep-orange'
    case 'tuic':
      return 'pink'
    case 'wireguard':
      return 'blue'
    case 'ssh':
      return 'blue-grey'
    default:
      return 'primary'
  }
}

const getProtocolIcon = (type: unknown): string => {
  const t = String(type || '').trim().toLowerCase()
  switch (t) {
    case 'ssh':
      return 'mdi-console'
    case 'wireguard':
      return 'mdi-shield-lock-outline'
    case 'shadowquic':
    case 'hysteria':
    case 'hysteria2':
    case 'tuic':
      return 'mdi-lightning-bolt-outline'
    default:
      return 'mdi-server-network'
  }
}

// 复制文本辅助函数
const copyText = async (text: unknown, label = '内容') => {
  const clean = normalizeText(text)
  if (!clean) return
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(clean)
      push.success({
        title: '复制成功',
        message: `${label}已复制到剪贴板`,
        duration: 2500,
      })
    }
  } catch (err) {
    push.error({
      title: '复制失败',
      message: String(err),
      duration: 3000,
    })
  }
}

// 模态弹窗状态
const modal = ref({
  visible: false,
  id: 0,
  data: '',
})

const showClearDialog = ref(false)
const clearingSubManager = ref(false)

// 全局安全删除节点弹窗状态
const deleteDialog = ref({
  visible: false,
  id: 0,
  tag: '',
  isManaged: false,
})

const isManagedOutbound = (item: any): boolean => {
  return Boolean(
    item?.source_client_id !== undefined &&
    item?.source_client_id !== null &&
    String(item?.source_client_id).trim() !== ''
  ) || Boolean(
    item?.source_type === 'client' ||
    item?.source_type === 'mihomo_client'
  )
}

const confirmDeleteOutbound = (item: any) => {
  if (subManagerWriteBusy.value || !isValidOutbound(item)) return
  deleteDialog.value = {
    visible: true,
    id: Number(item.id),
    tag: normalizeText(item.tag),
    isManaged: isManagedOutbound(item),
  }
}

const deletingSubOutboundId = ref<number | null>(null)
const subManagerWriteBusy = computed(() => clearingSubManager.value || deletingSubOutboundId.value !== null)

const showModal = (id: number) => {
  if (subManagerWriteBusy.value) return
  modal.value.id = id
  modal.value.data = id == 0 ? '' : JSON.stringify(subOutbounds.value.findLast((o) => o.id == id))
  modal.value.visible = true
}

const closeModal = () => {
  if (subManagerWriteBusy.value) return
  modal.value.visible = false
}

const qrcode = ref({
  visible: false,
  tag: '',
})

const showQrCode = (tag: string) => {
  if (subManagerWriteBusy.value) return
  qrcode.value.tag = tag
  qrcode.value.visible = true
}

const closeQrCode = () => {
  qrcode.value.visible = false
}

const stats = ref({
  visible: false,
  resource: 'outbound',
  tag: '',
})

const delSubOutbound = async () => {
  if (subManagerWriteBusy.value || !deleteDialog.value.id) return
  const id = deleteDialog.value.id
  const outbound = subOutbounds.value.find((item: any) => item.id === id)
  if (!outbound?.tag) return
  deletingSubOutboundId.value = id
  try {
    const success = await Data().save('suboutbounds', 'del', outbound.tag)
    if (success) {
      deleteDialog.value.visible = false
    }
  } finally {
    deletingSubOutboundId.value = null
  }
}

const showStats = (tag: string) => {
  if (subManagerWriteBusy.value) return
  stats.value.tag = tag
  stats.value.visible = true
}

const closeStats = () => {
  stats.value.visible = false
}

const groupModal = ref({
  visible: false,
})

const showGroupModal = () => {
  if (subManagerWriteBusy.value) return
  groupModal.value.visible = true
}

const closeGroupModal = () => {
  groupModal.value.visible = false
}

const clearSubManager = async () => {
  if (subManagerWriteBusy.value) return
  clearingSubManager.value = true
  try {
    const msg = await HttpUtils.post('api/clearSubManager', {})
    if (msg.obj && (msg.success || msg.obj.committed === true)) {
      store.setNewData(msg.obj)
      showClearDialog.value = false
      if (msg.obj.committed === true && !msg.success) {
        push.warning({
          title: '订阅管理',
          duration: 7000,
          message: msg.msg || '订阅节点已清空，但后置运行配置校验失败。',
        })
      }
    }
  } finally {
    clearingSubManager.value = false
  }
}

onMounted(() => {
  void initialize()
})

onUnmounted(() => {
  componentActive = false
})
</script>

<style scoped>
.node-title-text {
  max-width: calc(100% - 32px);
}
.suboutbound-card {
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}
.suboutbound-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.25) !important;
}
.cursor-pointer {
  cursor: pointer;
}
</style>
