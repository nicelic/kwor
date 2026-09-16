<template>
  <section class="opt-page">
    <!-- 全局加载失败提示 -->
    <v-alert
      v-if="pageLoadError"
      type="error"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      <div class="d-flex align-center justify-space-between flex-wrap ga-3">
        <span>{{ pageLoadError }}</span>
        <v-btn variant="outlined" prepend-icon="mdi-refresh" :loading="pageLoading" @click="refreshAll">
          重新加载
        </v-btn>
      </div>
    </v-alert>

    <!-- 顶部状态与概览 Hero 卡片 -->
    <v-card class="opt-hero mb-4" rounded="xl" :loading="pageLoading && !pageReady">
      <div class="opt-hero__bg"></div>
      <v-card-text class="opt-hero__content">
        <div class="opt-hero__top">
          <div class="d-flex align-center ga-3">
            <div class="opt-hero__icon">
              <v-icon size="30">mdi-tune-vertical</v-icon>
            </div>
            <div>
              <div class="text-overline opt-hero__eyebrow">SYSTEM OPTIMIZATION</div>
              <div class="text-h5 font-weight-bold">系统环境与性能优化</div>
              <div class="text-body-2 text-medium-emphasis mt-1">
                接管与调优主机 Linux 持久化日志、内核 TCP 网络参数、系统 DNS 解析及物理网卡 MTU。
              </div>
            </div>
          </div>
          <div class="opt-hero__toolbar">
            <v-btn
              class="opt-hero-action"
              variant="outlined"
              prepend-icon="mdi-refresh"
              :loading="pageLoading"
              :disabled="overviewInteractionDisabled && !pageLoadError"
              @click="refreshAll">
              重新加载
            </v-btn>
          </div>
        </div>

        <div class="opt-hero__chips mt-3 d-flex flex-wrap align-center ga-2">
          <v-chip
            size="small"
            :color="pageReady ? 'success' : pageLoading ? 'info' : 'warning'"
            variant="flat">
            {{ pageReady ? '运行状态就绪' : pageLoading ? '正在同步概览...' : '加载异常' }}
          </v-chip>
          <v-chip
            v-if="pageReady"
            size="small"
            :color="lockedFilesCount > 0 ? 'success' : 'warning'"
            variant="flat"
            class="opt-hero-chip--lock">
            <v-icon start size="small">mdi-shield-lock-outline</v-icon>
            受管文件加锁：{{ lockedFilesCount }} / 3
          </v-chip>
          <v-chip
            v-if="pageReady && mtuOverview.enabled"
            size="small"
            color="primary"
            variant="flat">
            <v-icon start size="small">mdi-ethernet</v-icon>
            MTU：{{ mtuOverview.currentMtu || 1470 }} (已接管)
          </v-chip>
        </div>
      </v-card-text>
    </v-card>

    <!-- 四大核心功能卡片 2x2 网格 -->
    <v-row class="opt-grid">
      <!-- 模块 1：系统日志持久化 -->
      <v-col cols="12" md="6">
        <v-card rounded="xl" variant="outlined" class="opt-card card-cyan h-100 d-flex flex-column">
          <v-card-title class="d-flex align-center justify-space-between flex-wrap ga-2 py-3 px-4">
            <div class="text-subtitle-1 font-weight-bold d-flex align-center ga-2">
              <v-icon size="20" color="info">mdi-text-box-remove-outline</v-icon>
              <span>系统日志持久化</span>
            </div>
            <div class="d-flex align-center flex-wrap ga-2">
              <v-chip
                size="x-small"
                :color="logOverview.immutable ? 'success' : 'warning'"
                variant="tonal">
                <v-icon start size="x-small">mdi-lock-outline</v-icon>
                {{ logOverview.immutable ? '已加锁(+i)' : '未锁定' }}
              </v-chip>
              <v-switch
                :model-value="logOverview.enabled"
                :loading="switchingLog"
                :disabled="overviewInteractionDisabled || !logLoaded || loadingLog || switchingLog || !logOverview.supported"
                color="success"
                density="compact"
                inset
                hide-details
                label="禁用持久日志"
                @update:modelValue="onToggleLogSwitch" />
            </div>
          </v-card-title>
          <v-divider />
          <v-card-text class="flex-grow-1 pt-3 pb-3">
            <div class="text-body-2 text-medium-emphasis mb-3">
              关闭 systemd-journald 持久化日志存储，减少固态硬盘（SSD）高频磨损并降低系统开销；关闭时仅解除加锁，保留配置。
            </div>

            <v-alert
              v-if="logOverview.error"
              type="warning"
              variant="tonal"
              density="compact"
              class="mb-3">
              {{ logOverview.error }}
            </v-alert>

            <div class="opt-meta-box">
              <div class="opt-meta__row">
                <span class="opt-meta__label">生效配置文件</span>
                <strong class="opt-meta__value text-mono">{{ logOverview.configPath || '-' }}</strong>
              </div>
              <div class="opt-meta__row">
                <span class="opt-meta__label">当前运行状态</span>
                <span class="opt-meta__value">
                  <v-badge
                    dot
                    inline
                    :color="logOverview.enabled ? 'success' : 'grey'"
                    class="mr-1" />
                  {{ logOverview.enabled ? '已开启（持久日志已禁用）' : '已关闭（默认持久记录）' }}
                </span>
              </div>
            </div>
          </v-card-text>
          <v-divider />
          <v-card-actions class="px-4 py-3 d-flex justify-space-between align-center flex-wrap ga-2">
            <v-tooltip location="top" text="每次保存执行完整重建流程：解除锁定 -> 删旧重建 -> 写入校验 -> 重新加锁并重启 journald">
              <template #activator="{ props: tipProps }">
                <span v-bind="tipProps" class="text-caption text-medium-emphasis d-flex align-center ga-1 cursor-pointer">
                  <v-icon size="small">mdi-information-outline</v-icon>
                  支持底层 chattr +i 防篡改
                </span>
              </template>
            </v-tooltip>
            <v-btn
              color="primary"
              variant="tonal"
              size="small"
              prepend-icon="mdi-file-document-edit-outline"
              :disabled="overviewInteractionDisabled || !logLoaded || loadingLog || !logOverview.supported"
              @click="openLogEditor">
              编辑完整配置
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-col>

      <!-- 模块 2：Linux 内核参数优化 (sysctl) -->
      <v-col cols="12" md="6">
        <v-card rounded="xl" variant="outlined" class="opt-card card-blue h-100 d-flex flex-column">
          <v-card-title class="d-flex align-center justify-space-between flex-wrap ga-2 py-3 px-4">
            <div class="text-subtitle-1 font-weight-bold d-flex align-center ga-2">
              <v-icon size="20" color="primary">mdi-tune-variant</v-icon>
              <span>内核网络参数 (sysctl)</span>
            </div>
            <div class="d-flex align-center flex-wrap ga-2">
              <v-chip
                size="x-small"
                :color="sysctlOverview.immutable ? 'success' : 'warning'"
                variant="tonal">
                <v-icon start size="x-small">mdi-lock-outline</v-icon>
                {{ sysctlOverview.immutable ? '已加锁(+i)' : '未锁定' }}
              </v-chip>
              <v-switch
                :model-value="sysctlOverview.enabled"
                :loading="switchingSysctl"
                :disabled="overviewInteractionDisabled || !sysctlLoaded || loadingSysctl || switchingSysctl || !sysctlOverview.supported"
                color="success"
                density="compact"
                inset
                hide-details
                label="启用优化参数"
                @update:modelValue="onToggleSysctlSwitch" />
            </div>
          </v-card-title>
          <v-divider />
          <v-card-text class="flex-grow-1 pt-3 pb-3">
            <div class="text-body-2 text-medium-emphasis mb-3">
              优化 Linux 内核 TCP 读写缓冲区、并发连接队列与拥塞控制算法，加锁后立即执行生效，提升网络传输吞吐与稳定性。
            </div>

            <v-alert
              v-if="sysctlOverview.error"
              type="warning"
              variant="tonal"
              density="compact"
              class="mb-3">
              {{ sysctlOverview.error }}
            </v-alert>

            <div class="opt-meta-box">
              <div class="opt-meta__row">
                <span class="opt-meta__label">生效配置文件</span>
                <strong class="opt-meta__value text-mono">{{ sysctlOverview.configPath || '-' }}</strong>
              </div>
              <div class="opt-meta__row">
                <span class="opt-meta__label">当前运行状态</span>
                <span class="opt-meta__value">
                  <v-badge
                    dot
                    inline
                    :color="sysctlOverview.enabled ? 'success' : 'grey'"
                    class="mr-1" />
                  {{ sysctlOverview.enabled ? '已启用优化参数' : '已关闭优化参数' }}
                </span>
              </div>
            </div>
          </v-card-text>
          <v-divider />
          <v-card-actions class="px-4 py-3 d-flex justify-space-between align-center flex-wrap ga-2">
            <v-tooltip location="top" text="接管 /etc/sysctl.d/99-s-ui-optimize.conf 与 /etc/sysctl.conf，保存即加锁并执行 sysctl -p 立即应用">
              <template #activator="{ props: tipProps }">
                <span v-bind="tipProps" class="text-caption text-medium-emphasis d-flex align-center ga-1 cursor-pointer">
                  <v-icon size="small">mdi-information-outline</v-icon>
                  双配置双锁定并即时生效
                </span>
              </template>
            </v-tooltip>
            <v-btn
              color="primary"
              variant="tonal"
              size="small"
              prepend-icon="mdi-file-document-edit-outline"
              :disabled="overviewInteractionDisabled || !sysctlLoaded || loadingSysctl || !sysctlOverview.supported"
              @click="openSysctlEditor">
              编辑完整配置
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-col>

      <!-- 模块 3：系统 DNS 解析 (resolv.conf) -->
      <v-col cols="12" md="6">
        <v-card rounded="xl" variant="outlined" class="opt-card card-cyan h-100 d-flex flex-column">
          <v-card-title class="d-flex align-center justify-space-between flex-wrap ga-2 py-3 px-4">
            <div class="text-subtitle-1 font-weight-bold d-flex align-center ga-2">
              <v-icon size="20" color="info">mdi-dns-outline</v-icon>
              <span>系统 DNS 解析</span>
            </div>
            <div class="d-flex align-center flex-wrap ga-2">
              <v-chip
                size="x-small"
                :color="dnsOverview.immutable ? 'success' : 'warning'"
                variant="tonal">
                <v-icon start size="x-small">mdi-lock-outline</v-icon>
                {{ dnsOverview.immutable ? '已加锁(+i)' : '未锁定' }}
              </v-chip>
              <v-btn
                color="primary"
                variant="tonal"
                size="small"
                prepend-icon="mdi-file-document-edit-outline"
                :disabled="overviewInteractionDisabled || !dnsLoaded || loadingDns || !dnsOverview.supported"
                @click="openDnsEditor">
                完整文件
              </v-btn>
            </div>
          </v-card-title>
          <v-divider />
          <v-card-text class="flex-grow-1 pt-3 pb-3">
            <div class="text-body-2 text-medium-emphasis mb-3">
              管理主机 /etc/resolv.conf 解析地址。快速编辑支持空格或换行混合输入，保存自动补全 nameserver 语法并加锁保护。
            </div>

            <v-alert
              v-if="dnsOverview.error"
              type="warning"
              variant="tonal"
              density="compact"
              class="mb-3">
              {{ dnsOverview.error }}
            </v-alert>

            <div class="opt-meta-box mb-3">
              <div class="opt-meta__row">
                <span class="opt-meta__label">生效路径</span>
                <strong class="opt-meta__value text-mono">{{ dnsOverview.configPath || '-' }}</strong>
              </div>
              <div class="opt-meta__row">
                <span class="opt-meta__label">当前生效 DNS</span>
                <div class="opt-meta__value d-flex flex-wrap ga-1 justify-end">
                  <template v-if="dnsOverview.activeNameServers && dnsOverview.activeNameServers.length > 0">
                    <v-chip
                      v-for="ns in dnsOverview.activeNameServers"
                      :key="ns"
                      size="x-small"
                      variant="outlined"
                      color="info">
                      {{ ns }}
                    </v-chip>
                  </template>
                  <span v-else class="text-medium-emphasis">-</span>
                </div>
              </div>
            </div>

            <div class="dns-quick-layout">
              <v-textarea
                :model-value="dnsNameServerInput"
                label="DNS 地址快速设置（支持空格/换行混合）"
                variant="outlined"
                density="comfortable"
                :rows="getDnsNameServerRows()"
                :maxlength="DNS_INPUT_MAX_LENGTH"
                :disabled="overviewInteractionDisabled || !dnsLoaded || loadingDns || savingDnsNameServers || !dnsOverview.supported"
                hide-details
                @update:model-value="updateDnsNameServerInput"
                class="dns-quick-input" />
              <div class="dns-quick-action">
                <v-btn
                  color="primary"
                  block
                  :loading="savingDnsNameServers"
                  :disabled="overviewInteractionDisabled || !dnsLoaded || loadingDns || savingDnsNameServers || !dnsOverview.supported"
                  @click="saveDnsNameServers">
                  保存 DNS
                </v-btn>
              </div>
            </div>
          </v-card-text>
          <v-divider />
          <v-card-actions class="px-4 py-2">
            <span class="text-caption text-medium-emphasis d-flex align-center ga-1">
              <v-icon size="small">mdi-shield-check-outline</v-icon>
              保存自动写入并加锁，防止外部 DHCP/NetworkManager 覆盖
            </span>
          </v-card-actions>
        </v-card>
      </v-col>

      <!-- 模块 4：网卡 MTU 调优 -->
      <v-col cols="12" md="6">
        <v-card rounded="xl" variant="outlined" class="opt-card card-blue h-100 d-flex flex-column">
          <v-card-title class="d-flex align-center justify-space-between flex-wrap ga-2 py-3 px-4">
            <div class="text-subtitle-1 font-weight-bold d-flex align-center ga-2">
              <v-icon size="20" color="primary">mdi-ethernet</v-icon>
              <span>网卡 MTU 优化</span>
            </div>
            <div class="d-flex align-center flex-wrap ga-2">
              <v-chip
                size="x-small"
                :color="mtuOverview.serviceEnabled ? 'success' : 'default'"
                variant="tonal">
                <v-icon start size="x-small">mdi-cog-sync-outline</v-icon>
                systemd：{{ mtuOverview.serviceEnabled ? '自启已注册' : '未注册' }}
              </v-chip>
              <v-switch
                :model-value="mtuOverview.enabled"
                :loading="switchingMtu"
                :disabled="overviewInteractionDisabled || !mtuLoaded || loadingMtu || switchingMtu || (!mtuOverview.supported && !mtuOverview.enabled)"
                color="success"
                density="compact"
                inset
                hide-details
                label="启用 MTU 开关"
                @update:modelValue="onToggleMtuSwitch" />
            </div>
          </v-card-title>
          <v-divider />
          <v-card-text class="flex-grow-1 pt-3 pb-3">
            <div class="text-body-2 text-medium-emphasis mb-3">
              批量调优物理与虚拟网卡 MTU（范围 1280-1600），离开输入框自动生效并注册开机服务；关闭开关自动恢复 1470。
            </div>

            <v-alert
              v-if="mtuOverview.error"
              type="warning"
              variant="tonal"
              density="compact"
              class="mb-3">
              {{ mtuOverview.error }}
            </v-alert>

            <div class="mb-3">
              <v-tooltip
                :model-value="isMtuOutOfRange"
                location="top"
                text="可填写数值 1280 - 1600">
                <template #activator="{ props: tooltipProps }">
                  <v-text-field
                    v-bind="tooltipProps"
                    v-model="mtuInput"
                    label="目标 MTU 数值（1280 - 1600）"
                    type="number"
                    min="1280"
                    max="1600"
                    density="comfortable"
                    variant="outlined"
                    hide-details
                    :loading="savingMtu"
                    :disabled="overviewInteractionDisabled || !mtuLoaded || loadingMtu || switchingMtu || !mtuOverview.enabled || (!mtuOverview.supported && !mtuOverview.enabled)"
                    class="w-100"
                    @blur="onMtuInputBlur"
                    @keydown.enter="($event.target as HTMLInputElement)?.blur()" />
                </template>
              </v-tooltip>
            </div>

            <div class="opt-meta-box">
              <div class="opt-meta__row align-start">
                <span class="opt-meta__label mt-1">生效网卡列表</span>
                <div class="opt-meta__value d-flex flex-wrap ga-1 justify-end">
                  <template v-if="mtuOverview.interfaceDetails && mtuOverview.interfaceDetails.length > 0">
                    <v-chip
                      v-for="item in mtuOverview.interfaceDetails"
                      :key="item.name"
                      size="x-small"
                      variant="tonal"
                      color="primary">
                      {{ item.name }} (MTU: {{ formatMtuValue(item.currentMtu) }})
                    </v-chip>
                  </template>
                  <template v-else-if="mtuOverview.interfaces && mtuOverview.interfaces.length > 0">
                    <v-chip
                      v-for="name in mtuOverview.interfaces"
                      :key="name"
                      size="x-small"
                      variant="tonal"
                      color="primary">
                      {{ name }} (MTU: {{ formatMtuValue(mtuOverview.currentMtus?.[name] || mtuOverview.currentMtu) }})
                    </v-chip>
                  </template>
                  <span v-else class="text-medium-emphasis">
                    {{ mtuOverview.interface || '-' }} · MTU: {{ formatMtuValue(mtuOverview.currentMtu) }}
                  </span>
                </div>
              </div>
            </div>
          </v-card-text>
          <v-divider />
          <v-card-actions class="px-4 py-2">
            <span class="text-caption text-medium-emphasis d-flex align-center ga-1">
              <v-icon size="small">mdi-refresh-auto</v-icon>
              失焦或回车后自动应用生效 · 运行态服务：{{ mtuOverview.serviceActive || '-' }}
            </span>
          </v-card-actions>
        </v-card>
      </v-col>
    </v-row>

    <!-- 日志编辑弹窗 -->
    <v-dialog
      v-model="logDialogVisible"
      max-width="980"
      :fullscreen="smAndDown"
      scrollable
      :persistent="savingLog || resettingLog">
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">编辑 systemd journald 配置</v-card-title>
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
            保存时会执行：chattr -i -> 删除旧文件并重建 -> 写入并校验 -> chattr +i 并重启 journald（即使内容未变也会完整执行）。
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

    <!-- sysctl 编辑弹窗 -->
    <v-dialog
      v-model="sysctlDialogVisible"
      max-width="980"
      :fullscreen="smAndDown"
      scrollable
      :persistent="savingSysctl || resettingSysctl">
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">编辑 sysctl 参数配置</v-card-title>
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
            保存时会执行：两处文件解除锁定 -> 删除旧文件并重建 -> 写入校验 -> 重新加锁并立即应用参数（即使内容未变也会完整执行）。
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

    <!-- DNS 编辑弹窗 -->
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

type InterfaceMTUDetail = {
  name: string
  currentMtu: number
}

type MTUOptimizationOverview = {
  supported: boolean
  enabled: boolean
  interface: string
  interfaces?: string[]
  currentMtu: number
  currentMtus?: Record<string, number>
  interfaceDetails?: InterfaceMTUDetail[]
  mtu: number
  originalMtu: number
  originalMtus?: Record<string, number>
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
  interfaces: [],
  currentMtu: 0,
  currentMtus: {},
  interfaceDetails: [],
  mtu: 1500,
  originalMtu: 0,
  originalMtus: {},
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

const lockedFilesCount = computed(() => {
  let count = 0
  if (logOverview.value.immutable) count++
  if (sysctlOverview.value.immutable) count++
  if (dnsOverview.value.immutable) count++
  return count
})

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

const normalizeMtuOverview = (raw: unknown): MTUOptimizationOverview => {
  const data = (raw ?? {}) as Record<string, unknown>
  const rawDetails = Array.isArray(data.interfaceDetails) ? data.interfaceDetails : []
  const interfaceDetails: InterfaceMTUDetail[] = rawDetails
    .map((item) => {
      const detail = (item ?? {}) as Record<string, unknown>
      return {
        name: readString(detail, 'name', ''),
        currentMtu: readInt(detail, 'currentMtu', 0),
      }
    })
    .filter((item) => Boolean(item.name))

  return {
    supported: readBool(data, 'supported', false),
    enabled: readBool(data, 'enabled', false),
    interface: readString(data, 'interface', ''),
    interfaces: readStringArray(data, 'interfaces'),
    currentMtu: readInt(data, 'currentMtu', 0),
    currentMtus: (data.currentMtus ?? {}) as Record<string, number>,
    interfaceDetails,
    mtu: readInt(data, 'mtu', 1500),
    originalMtu: readInt(data, 'originalMtu', 0),
    originalMtus: (data.originalMtus ?? {}) as Record<string, number>,
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
  const nextInputValue = next.currentMtu > 0 ? next.currentMtu : (next.mtu > 0 ? next.mtu : 1470)
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

const MTU_MIN = 1280
const MTU_MAX = 1600

const isMtuOutOfRange = computed(() => {
  if (!mtuOverview.value.enabled) return false
  const raw = mtuInput.value.trim()
  if (!raw) return false
  if (!/^\d+$/.test(raw)) return true
  const parsed = Number(raw)
  return !Number.isSafeInteger(parsed) || parsed < MTU_MIN || parsed > MTU_MAX
})
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

const onMtuInputBlur = async () => {
  if (!mtuOverview.value.enabled || switchingMtu.value || savingMtu.value) {
    return
  }
  const parsed = parseMtuInputValue()
  if (parsed === null) {
    const fallbackMtu = mtuOverview.value.currentMtu > 0 ? mtuOverview.value.currentMtu : (mtuOverview.value.mtu > 0 ? mtuOverview.value.mtu : 1470)
    mtuInput.value = String(fallbackMtu)
    return
  }
  const currentSystemMtu = mtuOverview.value.currentMtu > 0 ? mtuOverview.value.currentMtu : mtuOverview.value.mtu
  if (parsed === currentSystemMtu) {
    return
  }
  await saveMtu(parsed)
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

const saveMtu = async (targetMtu?: number) => {
  const parsed = typeof targetMtu === 'number' ? targetMtu : parseMtuInputValue()
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

.opt-hero {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(148, 163, 184, 0.18);
  background: linear-gradient(135deg, rgba(30, 41, 59, 0.5) 0%, rgba(15, 23, 42, 0.7) 100%);
}

.opt-hero__content {
  position: relative;
  z-index: 1;
  padding: 20px 24px;
}

.opt-hero__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
}

.opt-hero__icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: rgba(34, 211, 238, 0.12);
  color: #22d3ee;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(34, 211, 238, 0.24);
}

.opt-hero__eyebrow {
  color: #22d3ee;
  letter-spacing: 0.12em;
  font-size: 11px;
}

.opt-hero-chip--lock {
  background: #0f766e !important;
  color: #ecfeff !important;
}

.card-cyan {
  border-color: rgba(34, 211, 238, 0.45) !important;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.card-cyan:hover {
  border-color: rgba(34, 211, 238, 0.75) !important;
}

.card-blue {
  border-color: rgba(96, 165, 250, 0.45) !important;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}
.card-blue:hover {
  border-color: rgba(96, 165, 250, 0.75) !important;
}

.opt-card {
  min-height: 260px;
}

.opt-meta-box {
  background: rgba(15, 23, 42, 0.35);
  border: 1px solid rgba(148, 163, 184, 0.12);
  border-radius: 12px;
  padding: 12px 14px;
}

.opt-meta__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 8px;
}

.opt-meta__row:last-child {
  margin-bottom: 0;
}

.opt-meta__label {
  color: rgba(148, 163, 184, 0.9);
  font-size: 13px;
  flex-shrink: 0;
}

.opt-meta__value {
  font-size: 13px;
  text-align: right;
  overflow-wrap: anywhere;
}

.text-mono {
  font-family: Consolas, "Courier New", monospace;
}

.dns-quick-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 110px;
  gap: 12px;
  align-items: stretch;
}

.dns-quick-action {
  display: flex;
  align-items: stretch;
}

.dns-quick-action .v-btn {
  height: 100% !important;
  min-height: 48px;
}

.dns-quick-input :deep(textarea) {
  white-space: pre-wrap;
  font-family: Consolas, "Courier New", monospace;
  font-size: 13px;
}

@media (max-width: 960px) {
  .dns-quick-layout {
    grid-template-columns: 1fr;
  }
  .dns-quick-action .v-btn {
    height: 40px !important;
  }
}

@media (max-width: 599px) {
  .opt-hero__content {
    padding: 16px;
  }

  .opt-meta__row {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }

  .opt-meta__value {
    text-align: left;
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
