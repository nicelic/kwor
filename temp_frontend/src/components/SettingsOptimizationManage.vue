<template>
  <section class="opt-page">
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
              :disabled="loadingLog || switchingLog || !logOverview.supported"
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
              :disabled="loadingLog || !logOverview.supported"
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
              :disabled="loadingSysctl || switchingSysctl || !sysctlOverview.supported"
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
              :disabled="loadingSysctl || !sysctlOverview.supported"
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
              :disabled="loadingDns || !dnsOverview.supported"
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
                v-model="dnsNameServerInput"
                label="DNS 地址（支持空格/换行混合）"
                variant="outlined"
                :rows="getDnsNameServerRows()"
                auto-grow
                class="dns-quick-input" />
              <div class="dns-quick-action">
                <v-btn
                  color="primary"
                  block
                  :loading="savingDnsNameServers"
                  :disabled="loadingDns || savingDnsNameServers || !dnsOverview.supported"
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
              :disabled="loadingMtu || switchingMtu || !mtuOverview.supported"
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
                class="dns-quick-input" />
              <div class="dns-quick-action">
                <v-btn
                  color="primary"
                  block
                  :loading="savingMtu"
                  :disabled="!canSaveMtu"
                  @click="saveMtu">
                  保存
                </v-btn>
              </div>
            </div>
            <div class="text-caption text-medium-emphasis mt-2">
              默认网卡：{{ mtuOverview.interface || '-' }} · 当前 MTU：{{ formatMtuValue(mtuOverview.currentMtu) }} ·
              systemd：{{ mtuOverview.serviceEnabled ? '已注册' : '未注册' }} · 状态：{{ mtuOverview.serviceActive || '-' }}
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-dialog v-model="logDialogVisible" max-width="980">
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
            v-model="logEditorContent"
            label="配置内容"
            variant="outlined"
            rows="14"
            auto-grow
            class="opt-editor" />
          <div class="text-caption text-medium-emphasis mt-2">
            保存时会执行：chattr -i -> 删除旧文件并重建 -> 写入并校验 -> chattr +i -> 重启 journald（即使内容未变也会完整执行）。
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="closeLogEditor">取消</v-btn>
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
            :disabled="savingLog || resettingLog || logEditorContent.trim().length === 0"
            @click="saveLogContent">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="sysctlDialogVisible" max-width="980">
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
            v-model="sysctlEditorContent"
            label="配置内容"
            variant="outlined"
            rows="14"
            auto-grow
            class="opt-editor" />
          <div class="text-caption text-medium-emphasis mt-2">
            保存时会执行：两处文件 chattr -i -> 删除旧文件并重建 -> 写入并校验 -> chattr +i -> 按系统可用命令应用 sysctl 参数（即使内容未变也会完整执行）。
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="closeSysctlEditor">取消</v-btn>
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
            :disabled="savingSysctl || resettingSysctl || sysctlEditorContent.trim().length === 0"
            @click="saveSysctlContent">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="dnsDialogVisible" max-width="980">
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
            v-model="dnsEditorContent"
            label="配置内容"
            variant="outlined"
            rows="14"
            auto-grow
            class="opt-editor" />
          <div class="text-caption text-medium-emphasis mt-2">
            保存时会执行：chattr -i -> 删除旧文件并重建 -> 写入并校验 -> chattr +i（即使内容未变也会完整执行）。
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="closeDnsEditor">取消</v-btn>
          <v-btn
            color="primary"
            :loading="savingDns"
            :disabled="savingDns || dnsEditorContent.trim().length === 0"
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
const switchingLog = ref(false)
const logDialogVisible = ref(false)
const savingLog = ref(false)
const resettingLog = ref(false)
const logEditorContent = ref('')

const loadingSysctl = ref(false)
const switchingSysctl = ref(false)
const sysctlDialogVisible = ref(false)
const savingSysctl = ref(false)
const resettingSysctl = ref(false)
const sysctlEditorContent = ref('')

const loadingDns = ref(false)
const dnsDialogVisible = ref(false)
const savingDns = ref(false)
const dnsEditorContent = ref('')
const savingDnsNameServers = ref(false)
const dnsNameServerInput = ref('')

const loadingMtu = ref(false)
const switchingMtu = ref(false)
const savingMtu = ref(false)
const mtuInput = ref('')

const logRefreshFlight = ref<Promise<void> | null>(null)
const sysctlRefreshFlight = ref<Promise<void> | null>(null)
const dnsRefreshFlight = ref<Promise<void> | null>(null)
const mtuRefreshFlight = ref<Promise<void> | null>(null)

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
  if (typeof value === 'number' && Number.isFinite(value)) {
    return Math.trunc(value)
  }
  if (typeof value === 'string') {
    const parsed = Number.parseInt(value.trim(), 10)
    if (Number.isFinite(parsed)) {
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
  dnsEditorContent.value = next.content
}

const applyMtuOverview = (raw: unknown) => {
  const next = normalizeMtuOverview(raw)
  mtuOverview.value = next
  const nextInputValue = next.currentMtu > 0 ? next.currentMtu : (next.mtu > 0 ? next.mtu : 1500)
  mtuInput.value = String(nextInputValue)
}

const refreshLogOverview = async () => {
  if (logRefreshFlight.value) {
    return logRefreshFlight.value
  }

  const flight = (async () => {
    loadingLog.value = true
    try {
      const msg = await HttpUtils.get('api/system-log-optimization-overview')
      if (msg.success) {
        applyLogOverview(msg.obj)
      }
    } finally {
      loadingLog.value = false
    }
  })()

  logRefreshFlight.value = flight.finally(() => {
    logRefreshFlight.value = null
  })

  return logRefreshFlight.value
}

const refreshSysctlOverview = async () => {
  if (sysctlRefreshFlight.value) {
    return sysctlRefreshFlight.value
  }

  const flight = (async () => {
    loadingSysctl.value = true
    try {
      const msg = await HttpUtils.get('api/system-sysctl-optimization-overview')
      if (msg.success) {
        applySysctlOverview(msg.obj)
      }
    } finally {
      loadingSysctl.value = false
    }
  })()

  sysctlRefreshFlight.value = flight.finally(() => {
    sysctlRefreshFlight.value = null
  })

  return sysctlRefreshFlight.value
}

const refreshDnsOverview = async () => {
  if (dnsRefreshFlight.value) {
    return dnsRefreshFlight.value
  }

  const flight = (async () => {
    loadingDns.value = true
    try {
      const msg = await HttpUtils.get('api/system-linux-dns-optimization-overview')
      if (msg.success) {
        applyDnsOverview(msg.obj)
      }
    } finally {
      loadingDns.value = false
    }
  })()

  dnsRefreshFlight.value = flight.finally(() => {
    dnsRefreshFlight.value = null
  })

  return dnsRefreshFlight.value
}

const refreshMtuOverview = async () => {
  if (mtuRefreshFlight.value) {
    return mtuRefreshFlight.value
  }

  const flight = (async () => {
    loadingMtu.value = true
    try {
      const msg = await HttpUtils.get('api/system-mtu-optimization-overview')
      if (msg.success) {
        applyMtuOverview(msg.obj)
      }
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
  const parsed = Number.parseInt(mtuInput.value.trim(), 10)
  if (!Number.isFinite(parsed)) return null
  if (parsed < MTU_MIN || parsed > MTU_MAX) return null
  return parsed
}

const formatMtuValue = (value: number): string => {
  if (!Number.isFinite(value) || value <= 0) {
    return '-'
  }
  return String(Math.trunc(value))
}

const canSaveMtu = computed(() => {
  return (
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
  switchingMtu.value = true
  try {
    const payload: Record<string, unknown> = { enabled }
    if (enabled) {
      const parsed = parseMtuInputValue()
      if (parsed !== null) {
        payload.mtu = parsed
      }
    }
    const msg = await HttpUtils.post('api/system-mtu-optimization-switch', payload)
    if (msg.success) {
      applyMtuOverview(msg.obj)
    }
  } finally {
    switchingMtu.value = false
  }
}

const openLogEditor = async () => {
  await refreshLogOverview()
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
  await refreshSysctlOverview()
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
  await refreshDnsOverview()
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
  return Math.max(3, Math.ceil(count / 3))
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

const refreshAll = async () => {
  await Promise.all([refreshLogOverview(), refreshSysctlOverview(), refreshDnsOverview(), refreshMtuOverview()])
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

:deep(.opt-editor textarea) {
  font-family: Consolas, "Courier New", monospace;
  line-height: 1.5;
}
</style>
