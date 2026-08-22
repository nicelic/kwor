<template>
  <section class="opt-page">
    <v-alert
      v-if="pageLoadError"
      type="error"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      <div class="d-flex align-center justify-space-between flex-wrap ga-3">
        <span>{{ pageLoadError }}</span>
        <v-btn variant="text" prepend-icon="mdi-refresh" :loading="pageLoading" @click="refreshAll">
          重新加载
        </v-btn>
      </div>
    </v-alert>
    <v-alert
      v-else-if="!pageReady"
      type="info"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      <div class="d-flex align-center ga-2">
        <v-progress-circular indeterminate size="18" width="2" />
        <span>正在读取系统优化概览</span>
      </div>
    </v-alert>
    <v-row class="mt-1">
      <v-col cols="12" md="4">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 opt-group-cyan">
          <v-card-title class="text-subtitle-1 font-weight-medium">禁用系统日志</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-3">
              从关闭切换到开启时会先检查并尝试解除 immutable(+i)，再删除旧文件、重建写入，最后重新加锁并重启 journald；关闭时仅解除锁定，不清空配置内容。
            </div>
            <v-switch
              :model-value="logOverview.enabled"
              :loading="switchingLog"
              :disabled="overviewInteractionDisabled || !logLoaded || loadingLog || switchingLog || !logOverview.supported"
              color="success"
              inset
              hide-details
              label="禁用 systemd journal 持久日志"
              @update:modelValue="onToggleLogSwitch" />
            <div class="text-caption text-medium-emphasis mt-2">
              当前状态：{{ logOverview.enabled ? '已开启' : '已关闭' }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="4">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 opt-group-cyan">
          <v-card-title class="text-subtitle-1 font-weight-medium">编辑</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-3">
              可编辑 journald 参数内容。每次点保存都会执行完整重建流程（即使内容未修改）：检查/解除锁定、删旧、重建、写入、加锁并重启 journald。
            </div>
            <v-btn
              color="primary"
              prepend-icon="mdi-file-document-edit-outline"
              :disabled="overviewInteractionDisabled || !logLoaded || loadingLog || !logOverview.supported"
              @click="openLogEditor">
              编辑
            </v-btn>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="4">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 opt-group-cyan">
          <v-card-title class="text-subtitle-1 font-weight-medium">日志运行信息</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="opt-meta__row">
              <span>生效路径</span>
              <strong>{{ logOverview.configPath || '-' }}</strong>
            </div>
            <div class="opt-meta__row">
              <span>文件锁定</span>
              <strong :class="logOverview.immutable ? 'text-success' : 'text-warning'">
                {{ logOverview.immutable ? '已锁定(+i)' : '未锁定' }}
              </strong>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-alert
      v-if="logOverview.error"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      {{ logOverview.error }}
    </v-alert>

    <v-row class="mt-4">
      <v-col cols="12" md="4">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 opt-group-blue">
          <v-card-title class="text-subtitle-1 font-weight-medium">sysctl 参数优化</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-3">
              开启后会同时接管并重建 /etc/sysctl.d/99-s-ui-optimize.conf 与 /etc/sysctl.conf，加锁后按系统可用命令立即生效；关闭时仅解除两处 immutable(+i)，不清空内容。
            </div>
            <v-switch
              :model-value="sysctlOverview.enabled"
              :loading="switchingSysctl"
              :disabled="overviewInteractionDisabled || !sysctlLoaded || loadingSysctl || switchingSysctl || !sysctlOverview.supported"
              color="success"
              inset
              hide-details
              label="启用 sysctl 优化参数"
              @update:modelValue="onToggleSysctlSwitch" />
            <div class="text-caption text-medium-emphasis mt-2">
              当前状态：{{ sysctlOverview.enabled ? '已开启' : '已关闭' }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="4">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 opt-group-blue">
          <v-card-title class="text-subtitle-1 font-weight-medium">编辑</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-3">
              可编辑 sysctl 参数内容。每次点保存都会对两处文件执行完整重建流程（即使内容未修改）：检查/解除锁定、删旧、重建、写入、加锁并应用参数。
            </div>
            <v-btn
              color="primary"
              prepend-icon="mdi-tune-variant"
              :disabled="overviewInteractionDisabled || !sysctlLoaded || loadingSysctl || !sysctlOverview.supported"
              @click="openSysctlEditor">
              编辑
            </v-btn>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="4">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 opt-group-blue">
          <v-card-title class="text-subtitle-1 font-weight-medium">sysctl 运行信息</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="opt-meta__row">
              <span>生效路径</span>
              <strong>{{ sysctlOverview.configPath || '-' }}</strong>
            </div>
            <div class="opt-meta__row">
              <span>文件锁定</span>
              <strong :class="sysctlOverview.immutable ? 'text-success' : 'text-warning'">
                {{ sysctlOverview.immutable ? '已锁定(+i)' : '未锁定' }}
              </strong>
            </div>
          </v-card-text>
        </v-card>
      </v-col>

    </v-row>

    <v-alert
      v-if="sysctlOverview.error"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      {{ sysctlOverview.error }}
    </v-alert>

    <v-alert
      v-if="dnsOverview.error"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      {{ dnsOverview.error }}
    </v-alert>

    <v-card rounded="xl" variant="outlined" class="opt-meta opt-group-cyan">
      <v-card-title class="text-subtitle-1 font-weight-medium">DNS 运行信息</v-card-title>
      <v-divider />
      <v-card-text>
        <div class="opt-meta__row">
          <span>生效路径</span>
          <strong>{{ dnsOverview.configPath || '-' }}</strong>
        </div>
        <div class="opt-meta__row">
          <span>文件锁定</span>
          <strong :class="dnsOverview.immutable ? 'text-success' : 'text-warning'">
            {{ dnsOverview.immutable ? '已锁定(+i)' : '未锁定' }}
          </strong>
        </div>
      </v-card-text>
    </v-card>

    <v-row class="mt-4">
      <v-col cols="12" md="4">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 opt-group-cyan">
          <v-card-title class="text-subtitle-1 font-weight-medium">编辑</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-3">
              可编辑 Linux DNS（resolv.conf）内容。每次点保存都会执行完整重建流程（即使内容未修改）：检查/解除锁定、删旧、重建、写入、再锁定。
            </div>
            <div class="text-caption text-medium-emphasis mb-3">
              当前系统 DNS：{{ dnsActiveNameServerText }}
            </div>
            <v-btn
              color="primary"
              prepend-icon="mdi-dns"
              :disabled="overviewInteractionDisabled || !dnsLoaded || loadingDns || !dnsOverview.supported"
              @click="openDnsEditor">
              编辑
            </v-btn>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="8">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 dns-quick-card opt-group-cyan">
          <v-card-title class="text-subtitle-1 font-weight-medium">
            DNS 快速编辑
          </v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-3">
              仅展示非注释的 nameserver，支持空格、换行或混合输入。保存时会自动补全 nameserver，并尽量保留你当前使用的分隔显示方式。
            </div>
            <div class="dns-quick-layout">
              <v-textarea
                :model-value="dnsNameServerInput"
                label="DNS 地址（支持空格/换行混合）"
                variant="outlined"
                :rows="getDnsNameServerRows()"
                :maxlength="DNS_INPUT_MAX_LENGTH"
                :disabled="overviewInteractionDisabled || !dnsLoaded || loadingDns || savingDnsNameServers || !dnsOverview.supported"
                @update:model-value="updateDnsNameServerInput"
                class="dns-quick-input" />
              <div class="dns-quick-action">
                <v-btn
                  color="primary"
                  block
                  :loading="savingDnsNameServers"
                  :disabled="overviewInteractionDisabled || !dnsLoaded || loadingDns || savingDnsNameServers || !dnsOverview.supported"
                  @click="saveDnsNameServers">
                  保存
                </v-btn>
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-alert
      v-if="mtuOverview.error"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      {{ mtuOverview.error }}
    </v-alert>

    <v-row class="mt-4">
      <v-col cols="12" md="4">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 opt-group-blue">
          <v-card-title class="text-subtitle-1 font-weight-medium">默认网卡 MTU 优化</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-3">
              开启后会在 Promanager_data/mtu 生成脚本并赋予执行权限，立即应用 MTU，同时自动注册 systemd（重启后延迟 10 秒执行）。
            </div>
            <v-switch
              :model-value="mtuOverview.enabled"
              :loading="switchingMtu"
              :disabled="overviewInteractionDisabled || !mtuLoaded || loadingMtu || switchingMtu || (!mtuOverview.supported && !mtuOverview.enabled)"
              color="success"
              inset
              hide-details
              label="启用默认网卡 MTU 开关"
              @update:modelValue="onToggleMtuSwitch" />
            <div class="text-caption text-medium-emphasis mt-2">
              当前状态：{{ mtuOverview.enabled ? '已开启' : '已关闭' }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" md="8">
        <v-card rounded="xl" variant="outlined" class="opt-card h-100 dns-quick-card opt-group-blue">
          <v-card-title class="text-subtitle-1 font-weight-medium">MTU 快速设置</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-3">
              输入新 MTU 后点击保存：会删除旧脚本、按新值重建脚本、赋权、立即执行并自动校验 systemd 自启动状态。
            </div>
            <div class="dns-quick-layout">
              <v-text-field
                v-model="mtuInput"
                label="MTU 值（576-9500）"
                type="number"
                min="576"
                max="9500"
                variant="outlined"
                hide-details
                :disabled="overviewInteractionDisabled || !mtuLoaded || loadingMtu || switchingMtu || (!mtuOverview.supported && !mtuOverview.enabled)"
                class="dns-quick-input" />
              <div class="dns-quick-action">
                <v-btn
                  color="primary"
                  block
                  :loading="savingMtu"
                  :disabled="overviewInteractionDisabled || !canSaveMtu"
                  @click="saveMtu">
                  保存
                </v-btn>
              </div>
            </div>
            <div class="text-caption text-medium-emphasis mt-2">
              默认网卡：{{ mtuOverview.interface || '-' }} · 当前 MTU：{{ formatMtuValue(mtuOverview.currentMtu) }} ·
              原始 MTU：{{ formatMtuValue(mtuOverview.originalMtu) }} ·
              systemd：{{ mtuOverview.serviceEnabled ? '已注册' : '未注册' }} · 状态：{{ mtuOverview.serviceActive || '-' }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-dialog
      v-model="logDialogVisible"
      max-width="980"
      :fullscreen="smAndDown"
      scrollable
      :persistent="savingLog || resettingLog">
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">编辑 journald 配置</v-card-title>
        <v-divider />
        <v-card-text>
          <v-text-field
            label="生效路径"
            :model-value="logOverview.configPath || '-'"
            readonly
            hide-details
            class="mb-3" />
          <v-textarea
            :model-value="logEditorContent"
            label="配置内容"
            variant="outlined"
            rows="14"
            :maxlength="OPTIMIZATION_CONTENT_MAX_LENGTH"
            :readonly="savingLog || resettingLog"
            @update:model-value="updateLogEditorContent"
            class="opt-editor" />
          <div class="text-caption text-medium-emphasis mt-1">
            UTF-8 字节：{{ formatUtf8ByteCount(logEditorContent) }}
          </div>
          <div class="text-caption text-medium-emphasis mt-2">
            保存时会执行：chattr -i -> 删除旧文件并重建 -> 写入并校验 -> chattr +i -> 重启 journald（即使内容未变也会完整执行）。
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="opt-editor-actions">
          <v-spacer />
          <v-btn variant="text" :disabled="savingLog || resettingLog" @click="closeLogEditor">取消</v-btn>
          <v-btn
            color="warning"
            variant="outlined"
            :loading="resettingLog"
            :disabled="savingLog || resettingLog"
            @click="resetLogContent">
            重置
          </v-btn>
          <v-btn
            color="primary"
            :loading="savingLog"
            :disabled="savingLog || resettingLog || !hasOptimizationContentWithinLimit(logEditorContent)"
            @click="saveLogContent">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog
      v-model="sysctlDialogVisible"
      max-width="980"
      :fullscreen="smAndDown"
      scrollable
      :persistent="savingSysctl || resettingSysctl">
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">编辑 sysctl 配置</v-card-title>
        <v-divider />
        <v-card-text>
          <v-text-field
            label="生效路径"
            :model-value="sysctlOverview.configPath || '-'"
            readonly
            hide-details
            class="mb-3" />
          <v-textarea
            :model-value="sysctlEditorContent"
            label="配置内容"
            variant="outlined"
            rows="14"
            :maxlength="OPTIMIZATION_CONTENT_MAX_LENGTH"
            :readonly="savingSysctl || resettingSysctl"
            @update:model-value="updateSysctlEditorContent"
            class="opt-editor" />
          <div class="text-caption text-medium-emphasis mt-1">
            UTF-8 字节：{{ formatUtf8ByteCount(sysctlEditorContent) }}
          </div>
          <div class="text-caption text-medium-emphasis mt-2">
            保存时会执行：两处文件 chattr -i -> 删除旧文件并重建 -> 写入并校验 -> chattr +i -> 按系统可用命令应用 sysctl 参数（即使内容未变也会完整执行）。
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="opt-editor-actions">
          <v-spacer />
          <v-btn variant="text" :disabled="savingSysctl || resettingSysctl" @click="closeSysctlEditor">取消</v-btn>
          <v-btn
            color="warning"
            variant="outlined"
            :loading="resettingSysctl"
            :disabled="savingSysctl || resettingSysctl"
            @click="resetSysctlContent">
            重置
          </v-btn>
          <v-btn
            color="primary"
            :loading="savingSysctl"
            :disabled="savingSysctl || resettingSysctl || !hasOptimizationContentWithinLimit(sysctlEditorContent)"
            @click="saveSysctlContent">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog
      v-model="dnsDialogVisible"
      max-width="980"
      :fullscreen="smAndDown"
      scrollable
      :persistent="savingDns">
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">编辑 Linux DNS（resolv.conf）</v-card-title>
        <v-divider />
        <v-card-text>
          <v-text-field
            label="生效路径"
            :model-value="dnsOverview.configPath || '-'"
            readonly
            hide-details
            class="mb-3" />
          <v-textarea
            :model-value="dnsEditorContent"
            label="配置内容"
            variant="outlined"
            rows="14"
            :maxlength="OPTIMIZATION_CONTENT_MAX_LENGTH"
            :readonly="savingDns"
            @update:model-value="updateDnsEditorContent"
            class="opt-editor" />
          <div class="text-caption text-medium-emphasis mt-1">
            UTF-8 字节：{{ formatUtf8ByteCount(dnsEditorContent) }}
          </div>
          <div class="text-caption text-medium-emphasis mt-2">
            保存时会执行：chattr -i -> 删除旧文件并重建 -> 写入并校验 -> chattr +i（即使内容未变也会完整执行）。
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions class="opt-editor-actions">
          <v-spacer />
          <v-btn variant="text" :disabled="savingDns" @click="closeDnsEditor">取消</v-btn>
          <v-btn
            color="primary"
            :loading="savingDns"
            :disabled="savingDns || !hasOptimizationContentWithinLimit(dnsEditorContent)"
            @click="saveDnsContent">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import { computed, ref, watch } from 'vue'
import { push } from 'notivue'
import { useDisplay } from 'vuetify'

type OptimizationOverview = {
  supported: boolean
  enabled: boolean
  configPath: string
  content: string
  nameServers: string[]
  nameServersInput: string
  activeNameServers: string[]
  immutable: boolean
  error?: string
}

type MTUOptimizationOverview = {
  supported: boolean
  enabled: boolean
  interface: string
  currentMtu: number
  mtu: number
  originalMtu: number
  scriptPath: string
  scriptExists: boolean
  serviceName: string
  servicePath: string
  serviceRegistered: boolean
  serviceEnabled: boolean
  serviceActive: string
  error?: string
}

const props = withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})

const { smAndDown } = useDisplay()

const logOverview = ref<OptimizationOverview>({
  supported: false,
  enabled: false,
  configPath: '',
  content: '',
  nameServers: [],
  nameServersInput: '',
  activeNameServers: [],
  immutable: false,
  error: '',
})

const sysctlOverview = ref<OptimizationOverview>({
  supported: false,
  enabled: false,
  configPath: '',
  content: '',
  nameServers: [],
  nameServersInput: '',
  activeNameServers: [],
  immutable: false,
  error: '',
})

const dnsOverview = ref<OptimizationOverview>({
  supported: false,
  enabled: false,
  configPath: '',
  content: '',
  nameServers: [],
  nameServersInput: '',
  activeNameServers: [],
  immutable: false,
  error: '',
})

const mtuOverview = ref<MTUOptimizationOverview>({
  supported: false,
  enabled: false,
  interface: '',
  currentMtu: 0,
  mtu: 1500,
  originalMtu: 0,
  scriptPath: '',
  scriptExists: false,
  serviceName: '',
  servicePath: '',
  serviceRegistered: false,
  serviceEnabled: false,
  serviceActive: '',
  error: '',
})

const loadingLog = ref(false)
const logLoaded = ref(false)
const switchingLog = ref(false)
const logDialogVisible = ref(false)
const savingLog = ref(false)
const resettingLog = ref(false)
const logEditorContent = ref('')

const loadingSysctl = ref(false)
const sysctlLoaded = ref(false)
const switchingSysctl = ref(false)
const sysctlDialogVisible = ref(false)
const savingSysctl = ref(false)
const resettingSysctl = ref(false)
const sysctlEditorContent = ref('')

const loadingDns = ref(false)
const dnsLoaded = ref(false)
const dnsDialogVisible = ref(false)
const savingDns = ref(false)
const dnsEditorContent = ref('')
const savingDnsNameServers = ref(false)
const dnsNameServerInput = ref('')

const loadingMtu = ref(false)
const mtuLoaded = ref(false)
const switchingMtu = ref(false)
const savingMtu = ref(false)
const mtuInput = ref('')

const logLoadError = ref('')
const sysctlLoadError = ref('')
const dnsLoadError = ref('')
const mtuLoadError = ref('')

const pageLoading = computed(() => (
  loadingLog.value || loadingSysctl.value || loadingDns.value || loadingMtu.value
))
const pageReady = computed(() => (
  logLoaded.value && sysctlLoaded.value && dnsLoaded.value && mtuLoaded.value
))
const optimizationMutationBusy = computed(() => (
  switchingLog.value
  || savingLog.value
  || resettingLog.value
  || switchingSysctl.value
  || savingSysctl.value
  || resettingSysctl.value
  || savingDns.value
  || savingDnsNameServers.value
  || switchingMtu.value
  || savingMtu.value
))
const overviewInteractionDisabled = computed(() => (
  !pageReady.value || pageLoading.value || optimizationMutationBusy.value || Boolean(pageLoadError.value)
))
const pageLoadError = computed(() => [
  logLoadError.value,
  sysctlLoadError.value,
  dnsLoadError.value,
  mtuLoadError.value,
].filter((value): value is string => Boolean(value && value.trim())).join('；'))

const logRefreshFlight = ref<Promise<boolean> | null>(null)
const sysctlRefreshFlight = ref<Promise<boolean> | null>(null)
const dnsRefreshFlight = ref<Promise<boolean> | null>(null)
const mtuRefreshFlight = ref<Promise<boolean> | null>(null)

const readString = (raw: Record<string, unknown>, key: string, fallback = ''): string => {
  const value = raw[key]
  return typeof value === 'string' ? value : fallback
}

const readBool = (raw: Record<string, unknown>, key: string, fallback = false): boolean => {
  const value = raw[key]
  if (typeof value === 'boolean') return value
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase()
    if (normalized === 'true' || normalized === '1') return true
    if (normalized === 'false' || normalized === '0') return false
  }
  if (typeof value === 'number') return value !== 0
  return fallback
}

const readStringArray = (raw: Record<string, unknown>, key: string): string[] => {
  const value = raw[key]
  if (!Array.isArray(value)) return []
  return value
    .filter((item) => typeof item === 'string')
    .map((item) => String(item).trim())
    .filter((item) => item.length > 0)
}

const readInt = (raw: Record<string, unknown>, key: string, fallback = 0): number => {
  const value = raw[key]
  if (typeof value === 'number' && Number.isSafeInteger(value)) {
    return value
  }
  if (typeof value === 'string') {
    const normalized = value.trim()
    if (!/^-?\d+$/.test(normalized)) return fallback
    const parsed = Number(normalized)
    if (Number.isSafeInteger(parsed)) {
      return parsed
    }
  }
  return fallback
}

const normalizeOverview = (raw: unknown): OptimizationOverview => {
  const data = (raw ?? {}) as Record<string, unknown>
  return {
    supported: readBool(data, 'supported', false),
    enabled: readBool(data, 'enabled', false),
    configPath: readString(data, 'configPath', ''),
    content: readString(data, 'content', ''),
    nameServers: readStringArray(data, 'nameServers'),
    nameServersInput: readString(data, 'nameServersInput', ''),
    activeNameServers: readStringArray(data, 'activeNameServers'),
    immutable: readBool(data, 'immutable', false),
    error: readString(data, 'error', ''),
  }
}

const dnsActiveNameServerText = computed(() => {
  const list = dnsOverview.value.activeNameServers
  if (!Array.isArray(list) || list.length === 0) {
    return '-'
  }
  return list.join(' ')
})

const normalizeMtuOverview = (raw: unknown): MTUOptimizationOverview => {
  const data = (raw ?? {}) as Record<string, unknown>
  return {
    supported: readBool(data, 'supported', false),
    enabled: readBool(data, 'enabled', false),
    interface: readString(data, 'interface', ''),
    currentMtu: readInt(data, 'currentMtu', 0),
    mtu: readInt(data, 'mtu', 1500),
    originalMtu: readInt(data, 'originalMtu', 0),
    scriptPath: readString(data, 'scriptPath', ''),
    scriptExists: readBool(data, 'scriptExists', false),
    serviceName: readString(data, 'serviceName', ''),
    servicePath: readString(data, 'servicePath', ''),
    serviceRegistered: readBool(data, 'serviceRegistered', false),
    serviceEnabled: readBool(data, 'serviceEnabled', false),
    serviceActive: readString(data, 'serviceActive', ''),
    error: readString(data, 'error', ''),
  }
}

const applyLogOverview = (raw: unknown) => {
  const next = normalizeOverview(raw)
  logOverview.value = next
  if (!logDialogVisible.value) {
    logEditorContent.value = next.content
  }
}

const applySysctlOverview = (raw: unknown) => {
  const next = normalizeOverview(raw)
  sysctlOverview.value = next
  if (!sysctlDialogVisible.value) {
    sysctlEditorContent.value = next.content
  }
}

const applyDnsOverview = (raw: unknown) => {
  const next = normalizeOverview(raw)
  dnsOverview.value = next
  dnsNameServerInput.value = next.nameServersInput || next.nameServers.join(' ')
  if (!dnsDialogVisible.value) {
    dnsEditorContent.value = next.content
  }
}

const applyMtuOverview = (raw: unknown) => {
  const next = normalizeMtuOverview(raw)
  mtuOverview.value = next
  const nextInputValue = next.currentMtu > 0 ? next.currentMtu : (next.mtu > 0 ? next.mtu : 1500)
  mtuInput.value = String(nextInputValue)
}

const refreshLogOverview = async (): Promise<boolean> => {
  if (logRefreshFlight.value) {
    return logRefreshFlight.value
  }

  const flight = (async () => {
    loadingLog.value = true
    try {
      const msg = await HttpUtils.get('api/system-log-optimization-overview')
      if (msg.success && msg.obj) {
        applyLogOverview(msg.obj)
        logLoaded.value = true
        logLoadError.value = ''
        return true
      } else {
        const message = msg.msg || '系统日志概览加载失败'
        logLoadError.value = message
      }
      return false
    } finally {
      loadingLog.value = false
    }
  })()

  logRefreshFlight.value = flight.finally(() => {
    logRefreshFlight.value = null
  })

  return logRefreshFlight.value
}

const refreshSysctlOverview = async (): Promise<boolean> => {
  if (sysctlRefreshFlight.value) {
    return sysctlRefreshFlight.value
  }

  const flight = (async () => {
    loadingSysctl.value = true
    try {
      const msg = await HttpUtils.get('api/system-sysctl-optimization-overview')
      if (msg.success && msg.obj) {
        applySysctlOverview(msg.obj)
        sysctlLoaded.value = true
        sysctlLoadError.value = ''
        return true
      } else {
        const message = msg.msg || 'sysctl 概览加载失败'
        sysctlLoadError.value = message
      }
      return false
    } finally {
      loadingSysctl.value = false
    }
  })()

  sysctlRefreshFlight.value = flight.finally(() => {
    sysctlRefreshFlight.value = null
  })

  return sysctlRefreshFlight.value
}

const refreshDnsOverview = async (): Promise<boolean> => {
  if (dnsRefreshFlight.value) {
    return dnsRefreshFlight.value
  }

  const flight = (async () => {
    loadingDns.value = true
    try {
      const msg = await HttpUtils.get('api/system-linux-dns-optimization-overview')
      if (msg.success && msg.obj) {
        applyDnsOverview(msg.obj)
        dnsLoaded.value = true
        dnsLoadError.value = ''
        return true
      } else {
        const message = msg.msg || 'Linux DNS 概览加载失败'
        dnsLoadError.value = message
      }
      return false
    } finally {
      loadingDns.value = false
    }
  })()

  dnsRefreshFlight.value = flight.finally(() => {
    dnsRefreshFlight.value = null
  })

  return dnsRefreshFlight.value
}

const refreshMtuOverview = async (): Promise<boolean> => {
  if (mtuRefreshFlight.value) {
    return mtuRefreshFlight.value
  }

  const flight = (async () => {
    loadingMtu.value = true
    try {
      const msg = await HttpUtils.get('api/system-mtu-optimization-overview')
      if (msg.success && msg.obj) {
        applyMtuOverview(msg.obj)
        mtuLoaded.value = true
        mtuLoadError.value = ''
        return true
      } else {
        const message = msg.msg || 'MTU 概览加载失败'
        mtuLoadError.value = message
      }
      return false
    } finally {
      loadingMtu.value = false
    }
  })()

  mtuRefreshFlight.value = flight.finally(() => {
    mtuRefreshFlight.value = null
  })

  return mtuRefreshFlight.value
}

const MTU_MIN = 576
const MTU_MAX = 9500
const OPTIMIZATION_CONTENT_MAX_LENGTH = 256 * 1024
const DNS_INPUT_MAX_LENGTH = 16 * 1024
const textEncoder = new TextEncoder()

const utf8ByteLength = (value: string): number => textEncoder.encode(value).byteLength

const formatUtf8ByteCount = (value: string): string => (
  `${utf8ByteLength(value)} / ${OPTIMIZATION_CONTENT_MAX_LENGTH} 字节`
)

const limitUtf8Input = (raw: unknown, maxBytes: number): string => {
  const value = typeof raw === 'string' ? raw : ''
  if (utf8ByteLength(value) <= maxBytes) {
    return value
  }

  let result = ''
  let usedBytes = 0
  for (const char of value) {
    const charBytes = utf8ByteLength(char)
    if (usedBytes + charBytes > maxBytes) {
      break
    }
    result += char
    usedBytes += charBytes
  }
  return result
}

const updateLogEditorContent = (value: unknown) => {
  logEditorContent.value = limitUtf8Input(value, OPTIMIZATION_CONTENT_MAX_LENGTH)
}

const updateSysctlEditorContent = (value: unknown) => {
  sysctlEditorContent.value = limitUtf8Input(value, OPTIMIZATION_CONTENT_MAX_LENGTH)
}

const updateDnsEditorContent = (value: unknown) => {
  dnsEditorContent.value = limitUtf8Input(value, OPTIMIZATION_CONTENT_MAX_LENGTH)
}

const updateDnsNameServerInput = (value: unknown) => {
  dnsNameServerInput.value = limitUtf8Input(value, DNS_INPUT_MAX_LENGTH)
}

const hasToastMessage = (message: unknown): boolean => {
  return typeof message === 'string' && message.trim().length > 0
}

const notifyQuickSaveResult = (scope: 'DNS' | 'MTU', success: boolean, rawMessage?: unknown) => {
  if (success) {
    push.success({
      duration: 4000,
      message: `${scope} 保存成功`,
    })
    return
  }
  const reason = typeof rawMessage === 'string' ? rawMessage.trim() : ''
  push.warning({
    duration: 5000,
    message: reason ? `${scope} 保存失败：${reason}` : `${scope} 保存失败`,
  })
}

const parseMtuInputValue = (): number | null => {
  const raw = mtuInput.value.trim()
  if (!/^\d+$/.test(raw)) return null
  const parsed = Number(raw)
  if (!Number.isSafeInteger(parsed)) return null
  if (parsed < MTU_MIN || parsed > MTU_MAX) return null
  return parsed
}

const hasOptimizationContentWithinLimit = (content: string): boolean => {
  return content.trim().length > 0 && utf8ByteLength(content) <= OPTIMIZATION_CONTENT_MAX_LENGTH
}

const formatMtuValue = (value: number): string => {
  if (!Number.isSafeInteger(value) || value <= 0) {
    return '-'
  }
  return String(value)
}

const canSaveMtu = computed(() => {
  return (
    mtuLoaded.value &&
    mtuOverview.value.supported &&
    mtuOverview.value.enabled &&
    !loadingMtu.value &&
    !savingMtu.value &&
    parseMtuInputValue() !== null
  )
})

const onToggleLogSwitch = async (value: unknown) => {
  const enabled = Boolean(value)
  switchingLog.value = true
  try {
    const msg = await HttpUtils.post('api/system-log-optimization-switch', { enabled })
    if (msg.success) {
      applyLogOverview(msg.obj)
    }
  } finally {
    switchingLog.value = false
  }
}

const onToggleSysctlSwitch = async (value: unknown) => {
  const enabled = Boolean(value)
  switchingSysctl.value = true
  try {
    const msg = await HttpUtils.post('api/system-sysctl-optimization-switch', { enabled })
    if (msg.success) {
      applySysctlOverview(msg.obj)
    }
  } finally {
    switchingSysctl.value = false
  }
}

const onToggleMtuSwitch = async (value: unknown) => {
  const enabled = Boolean(value)
  const payload: Record<string, unknown> = { enabled }
  if (enabled) {
    const parsed = parseMtuInputValue()
    if (parsed === null) {
      notifyQuickSaveResult('MTU', false, `MTU 必须是 ${MTU_MIN}-${MTU_MAX} 的整数`)
      return
    }
    payload.mtu = parsed
  }

  switchingMtu.value = true
  try {
    const msg = await HttpUtils.post('api/system-mtu-optimization-switch', payload)
    if (msg.success) {
      applyMtuOverview(msg.obj)
    }
  } finally {
    switchingMtu.value = false
  }
}

const openLogEditor = async () => {
  if (overviewInteractionDisabled.value) return
  const loaded = await refreshLogOverview()
  if (!loaded || !logLoaded.value || !logOverview.value.supported) return
  logEditorContent.value = logOverview.value.content
  logDialogVisible.value = true
}

const closeLogEditor = () => {
  logDialogVisible.value = false
}

const saveLogContent = async () => {
  savingLog.value = true
  try {
    const msg = await HttpUtils.post('api/system-log-optimization-content', {
      content: logEditorContent.value,
    })
    if (msg.success) {
      applyLogOverview(msg.obj)
      logDialogVisible.value = false
    }
  } finally {
    savingLog.value = false
  }
}

const resetLogContent = async () => {
  resettingLog.value = true
  try {
    const msg = await HttpUtils.post('api/system-log-optimization-reset', {})
    if (msg.success) {
      applyLogOverview(msg.obj)
      logEditorContent.value = logOverview.value.content
    }
  } finally {
    resettingLog.value = false
  }
}

const openSysctlEditor = async () => {
  if (overviewInteractionDisabled.value) return
  const loaded = await refreshSysctlOverview()
  if (!loaded || !sysctlLoaded.value || !sysctlOverview.value.supported) return
  sysctlEditorContent.value = sysctlOverview.value.content
  sysctlDialogVisible.value = true
}

const closeSysctlEditor = () => {
  sysctlDialogVisible.value = false
}

const saveSysctlContent = async () => {
  savingSysctl.value = true
  try {
    const msg = await HttpUtils.post('api/system-sysctl-optimization-content', {
      content: sysctlEditorContent.value,
    })
    if (msg.success) {
      applySysctlOverview(msg.obj)
      sysctlDialogVisible.value = false
    }
  } finally {
    savingSysctl.value = false
  }
}

const resetSysctlContent = async () => {
  resettingSysctl.value = true
  try {
    const msg = await HttpUtils.post('api/system-sysctl-optimization-reset', {})
    if (msg.success) {
      applySysctlOverview(msg.obj)
      sysctlEditorContent.value = sysctlOverview.value.content
    }
  } finally {
    resettingSysctl.value = false
  }
}

const openDnsEditor = async () => {
  if (overviewInteractionDisabled.value) return
  const loaded = await refreshDnsOverview()
  if (!loaded || !dnsLoaded.value || !dnsOverview.value.supported) return
  dnsEditorContent.value = dnsOverview.value.content
  dnsDialogVisible.value = true
}

const closeDnsEditor = () => {
  dnsDialogVisible.value = false
}

const saveDnsContent = async () => {
  savingDns.value = true
  try {
    const msg = await HttpUtils.post('api/system-linux-dns-optimization-content', {
      content: dnsEditorContent.value,
    })
    if (msg.success) {
      applyDnsOverview(msg.obj)
      dnsDialogVisible.value = false
    }
  } finally {
    savingDns.value = false
  }
}

const normalizeDnsNameServerInput = (raw: string): string[] => {
  return raw
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .replace(/,/g, ' ')
    .split(/\s+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
}

const getDnsNameServerRows = (): number => {
  const count = normalizeDnsNameServerInput(dnsNameServerInput.value).length
  return Math.min(6, Math.max(3, Math.ceil(count / 3)))
}

const saveDnsNameServers = async () => {
  savingDnsNameServers.value = true
  try {
    const msg = await HttpUtils.post('api/system-linux-dns-optimization-nameservers', {
      nameServers: dnsNameServerInput.value,
    })
    if (msg.success) {
      applyDnsOverview(msg.obj)
      if (!hasToastMessage(msg.msg)) {
        notifyQuickSaveResult('DNS', true)
      }
      return
    }
    if (!hasToastMessage(msg.msg)) {
      notifyQuickSaveResult('DNS', false, msg.msg)
    }
  } finally {
    savingDnsNameServers.value = false
  }
}

const saveMtu = async () => {
  const parsed = parseMtuInputValue()
  if (parsed === null) {
    return
  }
  savingMtu.value = true
  try {
    const msg = await HttpUtils.post('api/system-mtu-optimization-mtu', {
      mtu: parsed,
    })
    if (msg.success) {
      applyMtuOverview(msg.obj)
      if (!hasToastMessage(msg.msg)) {
        notifyQuickSaveResult('MTU', true)
      }
      return
    }
    if (!hasToastMessage(msg.msg)) {
      notifyQuickSaveResult('MTU', false, msg.msg)
    }
  } finally {
    savingMtu.value = false
  }
}

const refreshAll = async (): Promise<boolean> => {
  const logReady = await refreshLogOverview()
  const sysctlReady = await refreshSysctlOverview()
  const dnsReady = await refreshDnsOverview()
  const mtuReady = await refreshMtuOverview()
  return logReady && sysctlReady && dnsReady && mtuReady
}

watch(
  () => props.active,
  (active) => {
    if (active) {
      void refreshAll()
    }
  },
  { immediate: true },
)
</script>

<style scoped>
.opt-page {
  width: 100%;
}

.opt-card {
  min-height: 220px;
}

.opt-card,
.opt-meta {
  border-width: 1px;
  border-style: solid;
  transition:
    background-color 0.2s ease,
    border-color 0.2s ease,
    box-shadow 0.2s ease;
}

.opt-group-cyan {
  background-color: rgba(35, 191, 190, 0.08);
  border-color: rgba(35, 191, 190, 0.72);
  box-shadow: inset 0 0 0 1px rgba(35, 191, 190, 0.06);
}

.opt-group-cyan :deep(.v-divider) {
  border-color: rgba(35, 191, 190, 0.34);
  opacity: 1;
}

.opt-group-blue {
  background-color: rgba(84, 156, 255, 0.08);
  border-color: rgba(84, 156, 255, 0.72);
  box-shadow: inset 0 0 0 1px rgba(84, 156, 255, 0.06);
}

.opt-group-blue :deep(.v-divider) {
  border-color: rgba(84, 156, 255, 0.34);
  opacity: 1;
}

.opt-meta {
  margin-top: 12px;
}

.opt-meta__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 8px;
}

.opt-meta__row:last-child {
  margin-bottom: 0;
}

.dns-quick-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 140px;
  gap: 16px;
  align-items: stretch;
}

.dns-quick-action {
  display: flex;
  align-items: flex-end;
}

.dns-quick-input :deep(textarea) {
  white-space: pre-wrap;
}

@media (max-width: 960px) {
  .dns-quick-layout {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 599px) {
  .opt-meta__row {
    align-items: flex-start;
  }

  .opt-meta__row strong {
    min-width: 0;
    overflow-wrap: anywhere;
    text-align: right;
  }

  .opt-editor-actions {
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .opt-editor-actions :deep(.v-spacer) {
    display: none;
  }
}

:deep(.opt-editor textarea) {
  font-family: Consolas, "Courier New", monospace;
  line-height: 1.5;
}
</style>
