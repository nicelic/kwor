<template>
  <div class="firewall-page">
    <!-- 顶部状态与概览 Hero 卡片 -->
    <v-card class="firewall-hero mb-4" rounded="xl" :loading="loading && !hasLoaded">
      <div class="firewall-hero__bg"></div>
      <v-card-text class="firewall-hero__content">
        <div class="firewall-hero__title-row">
          <div class="d-flex align-center ga-3">
            <div class="firewall-hero__icon">
              <v-icon size="30">mdi-shield-lock-outline</v-icon>
            </div>
            <div>
              <div class="text-overline firewall-hero__eyebrow">NFTABLES FIREWALL</div>
              <div class="text-h5 font-weight-bold">入站防火墙</div>
              <div class="text-body-2 text-medium-emphasis mt-1">
                以面板托管链路接管入站放行；SSH / 订阅按需保留。外部扫描结果仅用于展示和删除，不自动并入放行链。
              </div>
            </div>
          </div>

          <div class="firewall-hero__controls">
            <!-- nftables 安装与下载状态 -->
            <div v-if="showNftablesInstallTask" class="firewall-hero__install">
              <div v-if="!overview.nftables.installed" class="text-caption text-medium-emphasis mb-1">缺少 nft 命令</div>
              <v-btn
                v-if="hasActiveNftablesInstall"
                size="small"
                :color="nftablesInstallTask.canCancel ? 'error' : 'primary'"
                :prepend-icon="nftablesInstallTask.canCancel ? 'mdi-stop-circle-outline' : 'mdi-progress-wrench'"
                :disabled="nftablesStopRequestPending || !nftablesInstallTask.canCancel"
                @click="stopNftablesInstall">
                {{ nftablesStopButtonLabel }}
              </v-btn>
              <v-btn
                v-else-if="!overview.nftables.installed"
                size="small"
                color="primary"
                prepend-icon="mdi-download"
                :disabled="!hasLoaded || loading || installingNftables || hasFirewallWriteInProgress"
                @click="installNftables">
                下载 nftables
              </v-btn>
              <div v-if="hasActiveNftablesInstall" class="firewall-nftables-progress mt-2" role="status" aria-live="polite">
                <v-progress-circular indeterminate size="14" width="2" color="primary" />
                <span>{{ nftablesInstallTask.phase || (nftablesInstallTask.canCancel ? '正在准备下载' : '正在应用 nftables') }}</span>
              </div>
              <div v-else-if="hasTerminalNftablesInstallTask" class="firewall-nftables-progress mt-2" role="status" aria-live="polite">
                <v-icon :color="nftablesInstallTask.state === 'error' ? 'error' : 'warning'" size="14">
                  {{ nftablesInstallTask.state === 'error' ? 'mdi-alert-circle-outline' : 'mdi-information-outline' }}
                </v-icon>
                <span>{{ nftablesInstallTask.error || nftablesInstallTask.phase || nftablesInstallTerminalText }}</span>
              </div>
              <div class="text-caption text-medium-emphasis mt-2">
                {{ overview.nftables.packageManager || overview.nftables.systemFamily || 'Linux' }}
              </div>
              <div class="text-caption text-medium-emphasis mt-1">
                自动安装仅执行包管理器安装与服务启动，不改写系统软件源配置。
              </div>
            </div>

            <!-- SSH 管理快捷入口按钮 -->
            <v-btn
              variant="outlined"
              color="primary"
              class="firewall-hero__ssh-btn"
              prepend-icon="mdi-ssh"
              :disabled="!hasLoaded || hasFirewallWriteInProgress"
              @click="sshDialogVisible = true">
              <div class="d-flex flex-column align-start text-left">
                <span class="font-weight-medium">SSH 管理 ({{ overview.sshConfig.port || 22 }})</span>
                <span class="text-caption" style="font-size: 11px; line-height: 1.1; opacity: 0.85;">
                  {{ overview.sshConfig.proxyEnabled ? '代理已开启' : '代理未开启' }}
                </span>
              </div>
            </v-btn>

            <!-- 总开关 -->
            <div class="firewall-hero__switch">
              <div class="text-caption text-medium-emphasis mb-1">总开关</div>
              <v-switch
                :model-value="switchEnabled"
                :disabled="!hasLoaded || !overview.available || hasFirewallWriteInProgress || hasActiveNftablesInstall"
                :loading="switchBusy"
                color="success"
                inset
                hide-details
                @update:modelValue="onToggleFirewall" />
            </div>
          </div>
        </div>

        <!-- 状态 Chips -->
        <div class="firewall-hero__chips">
          <v-chip
            size="small"
            :color="!hasLoaded ? 'info' : overview.enabled ? 'success' : 'grey'"
            variant="flat"
            :class="[
              'firewall-hero-chip',
              !hasLoaded ? 'firewall-hero-chip--loading' : overview.enabled ? 'firewall-hero-chip--running' : 'firewall-hero-chip--stopped',
            ]">
            {{ !hasLoaded ? '加载中' : overview.enabled ? '运行中' : '已关闭' }}
          </v-chip>
          <v-chip size="small" color="info" variant="flat" class="firewall-hero-chip firewall-hero-chip--ports">
            当前保留 {{ formatPortList(overview.defaultPorts.active) }}
          </v-chip>
          <v-chip size="small" color="warning" variant="flat" class="firewall-hero-chip firewall-hero-chip--sync">
            上次同步：{{ lastSyncLabel }}
          </v-chip>
          <v-chip
            v-if="overview.nftables.installed || overview.nftables.kernelVersion"
            size="small"
            :color="nftCapabilityChipColor"
            variant="flat"
            class="firewall-hero-chip firewall-hero-chip--capability">
            {{ nftCapabilityLabel }}
          </v-chip>
        </div>

        <!-- 紧凑指标条 -->
        <div class="firewall-metric-bar mt-3">
          <div class="firewall-metric-item">
            <span class="text-caption text-medium-emphasis">总规则</span>
            <strong class="text-subtitle-1">{{ overview.totalCount }}</strong>
          </div>
          <div class="firewall-metric-item">
            <span class="text-caption text-medium-emphasis">面板规则</span>
            <strong class="text-subtitle-1">{{ overview.manualCount }}</strong>
          </div>
          <div class="firewall-metric-item">
            <span class="text-caption text-medium-emphasis">GeoIP 规则</span>
            <strong class="text-subtitle-1">{{ overview.geoRuleCount }}</strong>
          </div>
          <div class="firewall-metric-item">
            <span class="text-caption text-medium-emphasis">ACME 临时</span>
            <strong class="text-subtitle-1">{{ overview.temporaryCount }}</strong>
          </div>
          <div class="firewall-metric-item">
            <span class="text-caption text-medium-emphasis">系统保留</span>
            <strong class="text-subtitle-1">{{ overview.systemCount }}</strong>
          </div>
          <div class="firewall-metric-item">
            <span class="text-caption text-medium-emphasis">外部扫描</span>
            <strong class="text-subtitle-1">{{ overview.externalCount }}</strong>
          </div>
        </div>
      </v-card-text>
    </v-card>

    <!-- 异常与提示 Alert -->
    <v-alert
      v-if="loadError"
      type="error"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      <div class="d-flex align-center justify-space-between flex-wrap ga-3">
        <span>{{ loadError }}</span>
        <v-btn variant="text" prepend-icon="mdi-refresh" :loading="loading" @click="fetchOverview(false)">
          重新加载
        </v-btn>
      </div>
    </v-alert>
    <v-alert
      v-else-if="!hasLoaded"
      type="info"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      <div class="d-flex align-center ga-2">
        <v-progress-circular indeterminate size="18" width="2" />
        <span>正在读取防火墙概览</span>
      </div>
    </v-alert>
    <v-alert
      v-if="overview.error"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      {{ overview.error }}
    </v-alert>

    <!-- 核心视图 Tabs 导航 -->
    <v-tabs v-model="activeTab" color="primary" class="mb-4 firewall-tabs" density="comfortable">
      <v-tab value="rules">
        <v-icon start>mdi-shield-check-outline</v-icon>
        入站规则 ({{ overview.rules.length }})
      </v-tab>
      <v-tab value="geo">
        <v-icon start>mdi-map-search-outline</v-icon>
        来源 IP 识别 (GeoIP) ({{ overview.geoRules.length }})
      </v-tab>
      <v-tab value="diagnostics">
        <v-icon start>mdi-chart-timeline-variant</v-icon>
        网络监控与诊断
      </v-tab>
    </v-tabs>

    <!-- TAB 1: 入站规则 (核心) -->
    <div v-show="activeTab === 'rules'">
      <!-- 系统保留端口快速栏 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 firewall-reserved-bar">
        <v-card-text class="py-3">
          <div class="d-flex align-center justify-space-between flex-wrap ga-3">
            <div class="d-flex align-center ga-2">
              <v-icon color="info" size="20">mdi-shield-lock</v-icon>
              <span class="text-subtitle-2 font-weight-medium">系统保留端口放行：</span>
            </div>
            <div class="d-flex align-center flex-wrap ga-3">
              <!-- SSH -->
              <div class="firewall-reserved-item">
                <span class="text-caption text-medium-emphasis mr-1">SSH</span>
                <span class="font-weight-medium mr-1">{{ formatPortList(overview.defaultPorts.ssh) }}</span>
                <v-chip size="x-small" :color="systemRuleStatusColor('ssh')" variant="flat" class="mr-1">
                  {{ systemRuleStatusLabel('ssh') }}
                </v-chip>
                <v-btn
                  size="x-small"
                  variant="text"
                  color="warning"
                  :disabled="!hasLoaded || hasFirewallWriteInProgress"
                  :loading="systemRuleBusyKey === 'ssh'"
                  @click="setSystemRuleReserved('ssh', !overview.defaultPorts.sshReserved)">
                  {{ overview.defaultPorts.sshReserved ? '移除保留' : '恢复保留' }}
                </v-btn>
              </div>

              <v-divider vertical class="my-1" />

              <!-- 界面 -->
              <div class="firewall-reserved-item">
                <span class="text-caption text-medium-emphasis mr-1">界面</span>
                <span class="font-weight-medium mr-1">{{ formatPortList(overview.defaultPorts.panel) }}</span>
                <v-chip size="x-small" :color="systemRuleStatusColor('panel')" variant="flat">
                  {{ systemRuleStatusLabel('panel') }}
                </v-chip>
              </div>

              <v-divider vertical class="my-1" />

              <!-- 订阅 -->
              <div class="firewall-reserved-item">
                <span class="text-caption text-medium-emphasis mr-1">订阅</span>
                <span class="font-weight-medium mr-1">{{ formatPortList(overview.defaultPorts.sub) }}</span>
                <v-chip size="x-small" :color="systemRuleStatusColor('sub')" variant="flat" class="mr-1">
                  {{ systemRuleStatusLabel('sub') }}
                </v-chip>
                <v-btn
                  size="x-small"
                  variant="text"
                  color="warning"
                  :disabled="!hasLoaded || hasFirewallWriteInProgress"
                  :loading="systemRuleBusyKey === 'sub'"
                  @click="setSystemRuleReserved('sub', !overview.defaultPorts.subReserved)">
                  {{ overview.defaultPorts.subReserved ? '移除保留' : '恢复保留' }}
                </v-btn>
              </div>
            </div>
          </div>
          <div class="text-caption text-medium-emphasis mt-2 pt-2 border-t">
            <v-icon size="14" color="info" class="mr-1">mdi-information-outline</v-icon>
            开启时会重建面板自己的防火墙链，只放行系统保留端口、GeoIP 白名单命中项和面板规则；界面保留始终强制存在，SSH 和订阅可按需移除或恢复。
          </div>
        </v-card-text>
      </v-card>

      <!-- 规则列表卡片 -->
      <v-card rounded="xl" variant="outlined" class="firewall-rules">
        <v-card-title class="firewall-rules__toolbar">
          <div>
            <div class="text-subtitle-1 font-weight-medium">规则列表</div>
            <div class="text-caption text-medium-emphasis mt-1">
              支持 IPv4 / IPv6 / 双栈、端口段，以及源 IP / CIDR 的屏蔽或放行策略。
            </div>
          </div>
          <div class="d-flex align-center ga-2">
            <v-btn
              variant="tonal"
              color="info"
              prepend-icon="mdi-refresh"
              :loading="refreshing"
              :disabled="!hasLoaded || refreshing || hasFirewallWriteInProgress"
              @click="refreshOverview">
              立即刷新
            </v-btn>
            <v-btn
              color="primary"
              prepend-icon="mdi-plus"
              :disabled="!hasLoaded || !overview.enabled || hasFirewallWriteInProgress"
              @click="openRuleDialog()">
              新建规则
            </v-btn>
          </div>
        </v-card-title>
        <v-divider />

        <v-card-text>
          <!-- 搜索与筛选工具栏 -->
          <v-row class="mb-2">
            <v-col cols="12" md="4">
              <v-text-field
                v-model="searchText"
                label="搜索名称 / 端口 / 源地址"
                prepend-inner-icon="mdi-magnify"
                clearable
                density="compact"
                hide-details />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="familyFilter"
                :items="familyFilterItems"
                label="双栈筛选"
                density="compact"
                hide-details />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="originFilter"
                :items="originFilterItems"
                label="来源筛选"
                density="compact"
                hide-details />
            </v-col>
          </v-row>

          <!-- PC 端表格展示 -->
          <v-data-table
            v-if="!smAndDown"
            :headers="headers"
            :items="filteredRules"
            item-value="id"
            fixed-header
            class="rounded-lg firewall-table"
            hide-no-data>
            <template #item.name="{ item }">
              <div class="py-2">
                <div class="font-weight-medium">{{ item.name || '未命名规则' }}</div>
                <div class="text-caption text-medium-emphasis" v-if="item.description">{{ item.description }}</div>
              </div>
            </template>

            <template #item.protocol="{ item }">
              <v-chip size="small" variant="flat" class="firewall-protocol-chip">
                {{ protocolLabel(item.protocol) }}
              </v-chip>
            </template>

            <template #item.portSpec="{ item }">
              <span class="font-weight-medium">{{ item.portSpec || (isIcmpProtocol(item.protocol) ? '-' : '全部') }}</span>
            </template>

            <template #item.sourceSpec="{ item }">
              <div class="firewall-source-cell">
                <v-chip
                  v-if="sourceModeForRule(item)"
                  size="x-small"
                  :color="sourceModeForRule(item) === 'block' ? 'error' : 'success'"
                  variant="tonal">
                  {{ sourceModeForRule(item) === 'block' ? '只屏蔽' : '只放行' }}
                </v-chip>
                <span>{{ item.sourceSpec || (isIcmpProtocol(item.protocol) ? '-' : '任意来源') }}</span>
              </div>
            </template>

            <template #item.listenerState="{ item }">
              <div class="py-2 firewall-listener-cell">
                <div class="text-caption text-medium-emphasis" v-if="!item.listenerState.supported">
                  当前环境不支持监听探测
                </div>
                <div class="text-caption text-error" v-else-if="item.listenerState.error">
                  {{ item.listenerState.error }}
                </div>
                <div class="text-caption text-medium-emphasis" v-else-if="!ruleNeedsListenerTracking(item)">
                  无需监听探测
                </div>
                <div class="text-caption text-medium-emphasis" v-else-if="item.listenerState.listenerCount === 0">
                  <v-chip size="x-small" color="grey" variant="tonal">未检测到监听</v-chip>
                </div>
                <div v-else class="firewall-listener-list">
                  <div
                    v-for="listener in item.listenerState.listeners"
                    :key="listenerKey(listener)"
                    class="firewall-listener-entry">
                    <div class="d-flex align-center ga-2 flex-wrap">
                      <v-chip size="x-small" color="success" variant="flat">
                        {{ listener.port }}/{{ protocolLabel(listener.protocol) }}
                      </v-chip>
                      <v-chip
                        size="x-small"
                        variant="outlined"
                        :color="listenerStackColor(listener)">
                        {{ listenerStackLabel(listener) }}
                      </v-chip>
                      <span class="text-caption text-medium-emphasis">{{ listener.bindAddress || '*' }}</span>
                    </div>
                    <div class="text-caption text-medium-emphasis mt-1">
                      {{ formatListenerOwners(listener.owners) }}
                    </div>
                    <div class="text-caption text-medium-emphasis firewall-listener-command mt-1" v-if="formatListenerCommand(listener.owners)">
                      {{ formatListenerCommand(listener.owners) }}
                    </div>
                  </div>
                  <div class="text-caption text-medium-emphasis mt-1" v-if="item.listenerState.checkedAt">
                    更新于 {{ formatTimestamp(item.listenerState.checkedAt) }}
                  </div>
                </div>
              </div>
            </template>

            <template #item.family="{ item }">
              <v-chip size="small" variant="outlined" color="info">
                {{ familyLabel(item.family) }}
              </v-chip>
            </template>

            <template #item.origin="{ item }">
              <v-chip size="small" :color="originColor(item.origin)" variant="flat">
                {{ originLabel(item.origin) }}
              </v-chip>
            </template>

            <template #item.actions="{ item }">
              <div class="d-flex align-center ga-1">
                <v-btn
                  icon="mdi-pencil"
                  size="small"
                  variant="text"
                  color="primary"
                  aria-label="编辑规则"
                  title="编辑规则"
                  :disabled="!hasLoaded || !overview.enabled || !item.canEdit || hasFirewallWriteInProgress"
                  @click="openRuleDialog(item)" />
                <v-btn
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  aria-label="删除规则"
                  title="删除规则"
                  :disabled="!hasLoaded || !item.canDelete || (!overview.enabled && item.origin !== 'system') || hasFirewallWriteInProgress"
                  @click="removeRule(item)" />
              </div>
            </template>
          </v-data-table>

          <!-- 移动端专属卡片列表 (smAndDown) -->
          <div v-else class="firewall-mobile-list">
            <v-card
              v-for="item in filteredRules"
              :key="item.id"
              variant="outlined"
              rounded="lg"
              class="firewall-mobile-card mb-3">
              <v-card-text>
                <div class="d-flex align-start justify-space-between ga-2">
                  <div>
                    <div class="font-weight-medium text-subtitle-1">{{ item.name || '未命名规则' }}</div>
                    <div v-if="item.description" class="text-caption text-medium-emphasis mt-1">{{ item.description }}</div>
                  </div>
                  <div class="d-flex flex-column align-end ga-1">
                    <v-chip size="small" :color="originColor(item.origin)" variant="flat">
                      {{ originLabel(item.origin) }}
                    </v-chip>
                  </div>
                </div>

                <div class="firewall-mobile-grid mt-3">
                  <div>
                    <span class="text-caption text-medium-emphasis">端口 / 协议：</span>
                    <strong class="mr-1">{{ item.portSpec || (isIcmpProtocol(item.protocol) ? '-' : '全部') }}</strong>
                    <v-chip size="x-small" variant="flat" class="firewall-protocol-chip mr-1">
                      {{ protocolLabel(item.protocol) }}
                    </v-chip>
                    <v-chip size="x-small" variant="outlined" color="info">
                      {{ familyLabel(item.family) }}
                    </v-chip>
                  </div>
                  <div>
                    <span class="text-caption text-medium-emphasis">源地址策略：</span>
                    <v-chip
                      v-if="sourceModeForRule(item)"
                      size="x-small"
                      :color="sourceModeForRule(item) === 'block' ? 'error' : 'success'"
                      variant="tonal"
                      class="mr-1">
                      {{ sourceModeForRule(item) === 'block' ? '只屏蔽' : '只放行' }}
                    </v-chip>
                    <span>{{ item.sourceSpec || (isIcmpProtocol(item.protocol) ? '-' : '任意来源') }}</span>
                  </div>
                </div>

                <!-- 移动端监听探测 -->
                <div v-if="ruleNeedsListenerTracking(item)" class="mt-3 pt-2 border-t">
                  <div class="d-flex align-center justify-space-between text-caption text-medium-emphasis mb-1">
                    <span>监听探测：</span>
                    <span v-if="item.listenerState.checkedAt">更新于 {{ formatTimestamp(item.listenerState.checkedAt) }}</span>
                  </div>
                  <div v-if="item.listenerState.listenerCount === 0" class="text-caption text-medium-emphasis">未检测到监听</div>
                  <div v-else class="d-flex flex-wrap ga-2">
                    <v-chip
                      v-for="listener in item.listenerState.listeners"
                      :key="listenerKey(listener)"
                      size="x-small"
                      color="success"
                      variant="tonal">
                      {{ listener.port }}/{{ protocolLabel(listener.protocol) }}
                      <span v-if="listener.owners.length > 0">({{ listener.owners[0].name || listener.owners[0].pid }})</span>
                    </v-chip>
                  </div>
                </div>

                <!-- 移动端操作按钮组 -->
                <div class="d-flex align-center justify-end ga-2 mt-3 pt-2 border-t">
                  <v-btn
                    size="small"
                    variant="tonal"
                    color="primary"
                    prepend-icon="mdi-pencil"
                    :disabled="!hasLoaded || !overview.enabled || !item.canEdit || hasFirewallWriteInProgress"
                    @click="openRuleDialog(item)">
                    编辑
                  </v-btn>
                  <v-btn
                    size="small"
                    variant="tonal"
                    color="error"
                    prepend-icon="mdi-delete"
                    :disabled="!hasLoaded || !item.canDelete || (!overview.enabled && item.origin !== 'system') || hasFirewallWriteInProgress"
                    @click="removeRule(item)">
                    删除
                  </v-btn>
                </div>
              </v-card-text>
            </v-card>
          </div>

          <!-- 空状态 -->
          <div v-if="hasLoaded && filteredRules.length === 0" class="firewall-empty">
            <v-icon size="38" color="grey">mdi-shield-off-outline</v-icon>
            <div class="text-subtitle-2 mt-2">
              {{ overview.enabled ? '当前没有匹配到规则' : '防火墙关闭后仅停止下发 nftables 规则，当前规则记录会保留' }}
            </div>
          </div>
        </v-card-text>
      </v-card>
    </div>

    <!-- TAB 2: GeoIP 来源 IP 识别 -->
    <div v-show="activeTab === 'geo'">
      <v-card rounded="xl" variant="outlined" class="firewall-rules firewall-geo">
        <v-card-title class="firewall-rules__toolbar">
          <div>
            <div class="text-subtitle-1 font-weight-medium">来源 IP 识别 (GeoIP)</div>
            <div class="text-caption text-medium-emphasis mt-1">
              按国家或规则集识别来源 IP，对指定端口执行阻断或放行策略。
            </div>
          </div>
          <div class="firewall-geo__toolbar-right">
            <v-text-field
              v-model="geoIntervalInput"
              class="firewall-geo__interval"
              label="更新周期(分钟)"
              type="number"
              min="1"
              density="compact"
              :disabled="hasFirewallWriteInProgress"
              hide-details />
            <v-btn
              variant="tonal"
              color="secondary"
              prepend-icon="mdi-content-save-outline"
              :disabled="!hasLoaded || !geoSettingsDirty || hasFirewallWriteInProgress"
              :loading="savingGeoSettings"
              @click="saveGeoSettings">
              保存周期
            </v-btn>
            <v-btn
              variant="tonal"
              color="info"
              prepend-icon="mdi-database-sync-outline"
              :loading="geoRefreshing"
              :disabled="!hasLoaded || hasFirewallWriteInProgress"
              @click="refreshGeoRules">
              立即更新
            </v-btn>
            <v-btn
              color="primary"
              prepend-icon="mdi-plus"
              :disabled="!hasLoaded || hasFirewallWriteInProgress"
              @click="openGeoRuleDialog()">
              新建 GeoIP 规则
            </v-btn>
          </div>
        </v-card-title>
        <v-divider />

        <v-card-text>
          <v-alert
            variant="tonal"
            type="info"
            density="comfortable"
            class="mb-4 firewall-geo__note">
            手工源地址策略优先于 GeoIP：同端口选择“只放行”时，会把该端口变为国家来源白名单，其余来源直接丢弃。规则文件缓存于 Promanager_data/geoip。最近刷新：{{ geoLastRefreshLabel }}。
          </v-alert>

          <!-- 搜索与筛选 -->
          <v-row class="mb-2">
            <v-col cols="12" md="5">
              <v-text-field
                v-model="geoSearchText"
                label="搜索名称 / 端口 / 国家代码 / 来源"
                prepend-inner-icon="mdi-magnify"
                clearable
                density="compact"
                hide-details />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-select
                v-model="geoActionFilter"
                :items="geoActionFilterItems"
                label="动作筛选"
                density="compact"
                hide-details />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="geoFamilyFilter"
                :items="familyFilterItems"
                label="双栈筛选"
                density="compact"
                hide-details />
            </v-col>
          </v-row>

          <!-- PC 端表格 -->
          <v-data-table
            v-if="!smAndDown"
            :headers="geoHeaders"
            :items="filteredGeoRules"
            item-value="id"
            fixed-header
            class="rounded-lg firewall-table"
            hide-no-data>
            <template #item.name="{ item }">
              <div class="py-2">
                <div class="font-weight-medium">{{ item.name || '未命名 GeoIP 规则' }}</div>
                <div class="text-caption text-medium-emphasis" v-if="item.description">{{ item.description }}</div>
              </div>
            </template>

            <template #item.action="{ item }">
              <v-chip size="small" :color="geoActionColor(item.action)" variant="flat">
                {{ geoActionLabel(item.action) }}
              </v-chip>
            </template>

            <template #item.protocol="{ item }">
              <v-chip size="small" variant="flat" class="firewall-protocol-chip">
                {{ protocolLabel(item.protocol) }}
              </v-chip>
            </template>

            <template #item.portSpec="{ item }">
              <span class="font-weight-medium">{{ item.portSpec }}</span>
            </template>

            <template #item.countryCode="{ item }">
              <div class="py-2 firewall-geo__cell">
                <div class="d-flex align-center ga-2 flex-wrap">
                  <v-chip size="small" variant="outlined" color="info">
                    {{ geoCountryLabel(item.countryCode) }}
                  </v-chip>
                  <v-chip
                    v-if="item.customSourceUrls.length > 0"
                    size="small"
                    color="secondary"
                    variant="tonal">
                    自定义 {{ item.customSourceUrls.length }}
                  </v-chip>
                  <v-chip
                    v-else-if="item.sourceProviders.length === 0"
                    size="small"
                    color="warning"
                    variant="tonal">
                    默认顺序
                  </v-chip>
                </div>
                <div class="text-caption text-medium-emphasis mt-1">
                  {{ geoProviderSummary(item) }}
                </div>
                <div class="text-caption text-medium-emphasis mt-1" v-if="item.resolvedSources.length > 0">
                  当前命中：{{ geoResolvedSourceSummary(item) }}
                </div>
              </div>
            </template>

            <template #item.status="{ item }">
              <div class="py-2 firewall-geo__cell">
                <div class="d-flex align-center ga-2 flex-wrap">
                  <v-chip size="small" :color="geoStatusColor(item)" variant="flat">
                    {{ geoStatusLabel(item) }}
                  </v-chip>
                  <v-chip size="small" color="success" variant="tonal">
                    {{ item.prefixCount }} 条前缀
                  </v-chip>
                </div>
                <div class="text-caption text-medium-emphasis mt-1">
                  上次刷新：{{ formatTimestamp(item.lastRefreshAt) }}
                </div>
                <div class="text-caption text-medium-emphasis mt-1" v-if="item.cachedFiles.length > 0">
                  {{ geoCacheSummary(item) }}
                </div>
                <div class="text-caption text-error mt-1 firewall-geo__error" v-if="item.lastRefreshError">
                  {{ item.lastRefreshError }}
                </div>
              </div>
            </template>

            <template #item.actions="{ item }">
              <div class="d-flex align-center ga-1">
                <v-btn
                  icon="mdi-pencil"
                  size="small"
                  variant="text"
                  color="primary"
                  aria-label="编辑 GeoIP 规则"
                  title="编辑 GeoIP 规则"
                  :disabled="!hasLoaded || hasFirewallWriteInProgress"
                  @click="openGeoRuleDialog(item)" />
                <v-btn
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  aria-label="删除 GeoIP 规则"
                  title="删除 GeoIP 规则"
                  :disabled="!hasLoaded || hasFirewallWriteInProgress"
                  @click="removeGeoRule(item)" />
              </div>
            </template>
          </v-data-table>

          <!-- 移动端专属卡片 (smAndDown) -->
          <div v-else class="firewall-mobile-list">
            <v-card
              v-for="item in filteredGeoRules"
              :key="item.id"
              variant="outlined"
              rounded="lg"
              class="firewall-mobile-card mb-3">
              <v-card-text>
                <div class="d-flex align-start justify-space-between ga-2">
                  <div>
                    <div class="font-weight-medium text-subtitle-1">{{ item.name || '未命名 GeoIP 规则' }}</div>
                    <div v-if="item.description" class="text-caption text-medium-emphasis mt-1">{{ item.description }}</div>
                  </div>
                  <v-chip size="small" :color="geoActionColor(item.action)" variant="flat">
                    {{ geoActionLabel(item.action) }}
                  </v-chip>
                </div>

                <div class="firewall-mobile-grid mt-3">
                  <div>
                    <span class="text-caption text-medium-emphasis">端口 / 协议：</span>
                    <strong class="mr-1">{{ item.portSpec }}</strong>
                    <v-chip size="x-small" variant="flat" class="firewall-protocol-chip">
                      {{ protocolLabel(item.protocol) }}
                    </v-chip>
                  </div>
                  <div>
                    <span class="text-caption text-medium-emphasis">地区 / 规则集：</span>
                    <v-chip size="x-small" variant="outlined" color="info" class="mr-1">
                      {{ geoCountryLabel(item.countryCode) }}
                    </v-chip>
                    <span class="text-caption">{{ item.prefixCount }} 条前缀</span>
                  </div>
                </div>

                <div class="text-caption text-medium-emphasis mt-2">
                  {{ geoProviderSummary(item) }}
                </div>
                <div class="text-caption text-medium-emphasis mt-1" v-if="item.resolvedSources.length > 0">
                  当前命中：{{ geoResolvedSourceSummary(item) }}
                </div>
                <div class="text-caption text-medium-emphasis mt-1">
                  上次刷新：{{ formatTimestamp(item.lastRefreshAt) }}
                  <span v-if="item.cachedFiles.length > 0">· {{ geoCacheSummary(item) }}</span>
                </div>
                <div v-if="item.lastRefreshError" class="text-caption text-error mt-1">
                  错误：{{ item.lastRefreshError }}
                </div>

                <div class="d-flex align-center justify-end ga-2 mt-3 pt-2 border-t">
                  <v-btn
                    size="small"
                    variant="tonal"
                    color="primary"
                    prepend-icon="mdi-pencil"
                    :disabled="!hasLoaded || hasFirewallWriteInProgress"
                    @click="openGeoRuleDialog(item)">
                    编辑
                  </v-btn>
                  <v-btn
                    size="small"
                    variant="tonal"
                    color="error"
                    prepend-icon="mdi-delete"
                    :disabled="!hasLoaded || hasFirewallWriteInProgress"
                    @click="removeGeoRule(item)">
                    删除
                  </v-btn>
                </div>
              </v-card-text>
            </v-card>
          </div>

          <div v-if="hasLoaded && filteredGeoRules.length === 0" class="firewall-empty">
            <v-icon size="38" color="grey">mdi-map-search-outline</v-icon>
            <div class="text-subtitle-2 mt-2">
              {{ overview.geoRuleCount > 0 ? '当前筛选条件下没有 GeoIP 规则' : '还没有来源 IP 识别规则，可按端口或端口范围创建' }}
            </div>
          </div>
        </v-card-text>
      </v-card>
    </div>

    <!-- TAB 3: 网络监控与连接诊断 -->
    <div v-show="activeTab === 'diagnostics'">
      <v-row class="mb-4">
        <!-- TCP 活跃连接 -->
        <v-col cols="12" md="6">
          <v-card rounded="xl" variant="outlined" class="h-100">
            <v-card-item>
              <template #prepend>
                <v-icon color="primary" size="24">mdi-lan-connect</v-icon>
              </template>
              <v-card-title class="text-subtitle-1">TCP 系统活跃连接</v-card-title>
              <v-card-subtitle>系统全局 TCP 状态统计，不限端口</v-card-subtitle>
            </v-card-item>
            <v-divider />
            <v-card-text>
              <div class="d-flex align-center justify-space-between mb-3">
                <span class="text-h4 font-weight-bold text-primary">{{ formatMetricCount(overview.tcpActiveCount) }}</span>
                <span class="text-caption text-medium-emphasis">活跃连接总计</span>
              </div>
              <div class="firewall-diag-stats">
                <div class="firewall-diag-row">
                  <span>已建立 (ESTABLISHED)</span>
                  <strong>{{ formatMetricCount(overview.tcpEstablishedCount) }}</strong>
                </div>
                <div class="firewall-diag-row">
                  <span>TCP 半开 (SYN_RECV)</span>
                  <strong>{{ formatMetricCount(overview.tcpSynRecvCount) }}</strong>
                </div>
                <div class="firewall-diag-row">
                  <span>异常累计 (Syncookies / Drops / Overflows)</span>
                  <strong :class="overview.tcpAnomalyTotal > 0 ? 'text-warning' : ''">{{ formatMetricCount(overview.tcpAnomalyTotal) }}</strong>
                </div>
              </div>
              <div class="text-caption text-medium-emphasis mt-3">
                包含 SYN_SENT / FIN_WAIT / CLOSE_WAIT / LAST_ACK / CLOSING 等活动态连接；异常累计为系统自启动以来的总和。
              </div>
            </v-card-text>
          </v-card>
        </v-col>

        <!-- UDP 套接字 -->
        <v-col cols="12" md="6">
          <v-card rounded="xl" variant="outlined" class="h-100">
            <v-card-item>
              <template #prepend>
                <v-icon color="info" size="24">mdi-swap-vertical-bold</v-icon>
              </template>
              <v-card-title class="text-subtitle-1">UDP 当前套接字</v-card-title>
              <v-card-subtitle>系统全局 UDP socket 条目统计，不限端口</v-card-subtitle>
            </v-card-item>
            <v-divider />
            <v-card-text>
              <div class="d-flex align-center justify-space-between mb-3">
                <span class="text-h4 font-weight-bold text-info">{{ formatMetricCount(overview.udpSocketCount) }}</span>
                <span class="text-caption text-medium-emphasis">UDP 当前条目</span>
              </div>
              <div class="firewall-diag-stats">
                <div class="firewall-diag-row">
                  <span>异常累计 (NoPorts / InErrors / RcvbufErrors)</span>
                  <strong :class="overview.udpAnomalyTotal > 0 ? 'text-warning' : ''">{{ formatMetricCount(overview.udpAnomalyTotal) }}</strong>
                </div>
              </div>
              <div class="text-caption text-medium-emphasis mt-3">
                按系统全局 UDP socket 条目统计，异常累计为系统自启动以来的总和。
              </div>
            </v-card-text>
          </v-card>
        </v-col>
      </v-row>

      <!-- 外部扫描规则展示与就地清理 -->
      <v-card rounded="xl" variant="outlined">
        <v-card-title class="d-flex align-center justify-space-between flex-wrap ga-3">
          <div>
            <div class="text-subtitle-1 font-weight-medium">系统外部扫描放行规则</div>
            <div class="text-caption text-medium-emphasis mt-1">
              系统中扫描到的非受管外部规则（共 {{ overview.externalCount }} 条），仅用于展示和清理，不代表会被面板托管链自动放行。
            </div>
          </div>
        </v-card-title>
        <v-divider />
        <v-card-text>
          <v-alert
            v-if="externalRules.length === 0"
            variant="tonal"
            type="success"
            density="comfortable">
            当前系统中未检测到外部非受管放行规则，防火墙环境纯净。
          </v-alert>
          <div v-else class="firewall-external-list">
            <div
              v-for="extRule in externalRules"
              :key="extRule.id"
              class="d-flex align-center justify-space-between pa-3 rounded-lg border mb-2 flex-wrap ga-2">
              <div>
                <div class="font-weight-medium">{{ extRule.name || '外部规则 #' + extRule.id }}</div>
                <div class="text-caption text-medium-emphasis">
                  端口: {{ extRule.portSpec || '全部' }} · 协议: {{ protocolLabel(extRule.protocol) }} · 双栈: {{ familyLabel(extRule.family) }}
                </div>
              </div>
              <v-btn
                size="small"
                color="error"
                variant="tonal"
                prepend-icon="mdi-delete"
                :disabled="!hasLoaded || !extRule.canDelete || hasFirewallWriteInProgress"
                @click="removeRule(extRule)">
                清理规则
              </v-btn>
            </div>
          </div>
        </v-card-text>
      </v-card>
    </div>

    <!-- DIALOG: SSH 安全管理弹窗 -->
    <v-dialog v-model="sshDialogVisible" max-width="640" :fullscreen="smAndDown" scrollable>
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <div class="d-flex align-center ga-2">
            <v-icon color="primary">mdi-shield-key-outline</v-icon>
            <span class="text-h6 font-weight-bold">SSH 安全与代理配置</span>
          </div>
          <v-btn icon="mdi-close" variant="text" size="small" @click="sshDialogVisible = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-4">
          <!-- SSH 端口设置 -->
          <v-card variant="outlined" rounded="lg" class="pa-4 mb-4">
            <div class="text-subtitle-2 font-weight-medium mb-1">SSH 服务端口设置</div>
            <div class="text-caption text-medium-emphasis mb-3">
              检测端口：{{ formatPortList(overview.sshConfig.ports) }} · 配置路径：{{ overview.sshConfig.configPath || '-' }}
            </div>
            <div class="d-flex align-center ga-3">
              <v-text-field
                v-model="sshPortInput"
                label="SSH 端口"
                type="number"
                min="1"
                max="65535"
                density="compact"
                hide-details="auto"
                style="max-width: 200px"
                :disabled="!hasLoaded || hasFirewallWriteInProgress || !overview.sshConfig.supported || !!overview.sshConfig.error"
                :error="sshPortInputTouched && !sshPortInputValid"
                :error-messages="sshPortInputTouched && !sshPortInputValid ? ['端口必须是 1-65535 的正整数'] : []"
                @focus="onSSHPortFocus"
                @blur="onSSHPortBlur" />
              <v-btn
                color="primary"
                prepend-icon="mdi-content-save-outline"
                :loading="savingSSHPort"
                :disabled="!hasLoaded || !canSaveSSHPort || hasFirewallWriteInProgress || !!overview.sshConfig.error"
                @click="saveSSHPort">
                保存端口
              </v-btn>
            </div>
            <div class="text-caption text-warning mt-2">
              注意：修改保存后会自动重启 SSH 服务并更新防火墙放行规则。
            </div>
          </v-card>

          <!-- 是否开启 SSH 代理能力 -->
          <v-card variant="outlined" rounded="lg" class="pa-4">
            <div class="d-flex align-center justify-space-between ga-3">
              <div>
                <div class="text-subtitle-2 font-weight-medium">SSH 转发与代理能力</div>
                <div class="text-caption text-medium-emphasis mt-1">
                  开启后会自动配置 AllowTcpForwarding / PermitOpen / GatewayPorts
                </div>
              </div>
              <v-switch
                :model-value="sshProxySwitch"
                :disabled="!hasLoaded || hasFirewallWriteInProgress || !overview.sshConfig.supported || !!overview.sshConfig.error"
                :loading="switchingSSHProxy"
                color="success"
                hide-details
                inset
                @update:modelValue="onToggleSSHProxy" />
            </div>
            <div class="text-caption mt-2" :class="sshProxyHintClass">
              {{ sshProxyHintText }}
            </div>
          </v-card>

          <v-alert variant="tonal" type="info" density="comfortable" class="mt-4">
            SSH 端口修改与防火墙总开关互不干扰。开启防火墙前请务必确认 SSH 端口处于保留放行状态，防止新的远程 SSH 会话被拒绝。
          </v-alert>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="sshDialogVisible = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- DIALOG: 普通规则新建与编辑 -->
    <v-dialog v-model="dialogVisible" max-width="760" :fullscreen="smAndDown" :persistent="savingRule" scrollable>
      <v-card rounded="xl" :loading="savingRule">
        <v-card-title>{{ editingRule.id > 0 ? '编辑防火墙规则' : '新建防火墙规则' }}</v-card-title>
        <v-divider />
        <v-card-text>
          <v-row>
            <v-col cols="12" md="6">
              <v-text-field v-model="editingRule.name" label="规则名称" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="editingRule.family"
                :items="familyItems"
                :disabled="editingRuleUsesFixedFamily"
                label="IP 栈"
                hide-details />
            </v-col>
          </v-row>
          <v-row class="mt-1">
            <v-col cols="12" md="6">
              <v-select
                v-model="editingRule.protocol"
                :items="protocolItems"
                label="协议"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model="editingRule.portSpec"
                :disabled="!editingRuleNeedsPort"
                :placeholder="rulePortPlaceholder"
                label="端口 / 端口段"
                hide-details />
            </v-col>
          </v-row>
          <v-row class="mt-1">
            <v-col cols="12">
              <v-text-field
                v-model="editingRule.sourceSpec"
                :disabled="!editingRuleNeedsSource"
                label="源地址限制"
                :placeholder="ruleSourcePlaceholder"
                hide-details />
            </v-col>
          </v-row>
          <v-row class="mt-1">
            <v-col cols="12">
              <v-select
                v-model="editingRule.sourceMode"
                :items="sourceModeItems"
                :disabled="!editingRuleNeedsSource"
                label="源地址策略"
                hide-details />
            </v-col>
          </v-row>
          <v-row class="mt-1">
            <v-col cols="12">
              <v-textarea
                v-model="editingRule.description"
                label="备注"
                rows="3"
                auto-grow
                hide-details />
            </v-col>
          </v-row>
          <v-alert variant="tonal" type="info" density="comfortable" class="mt-4">
            规则保存后会立即重建面板自己的防火墙链；源地址策略为“不设置”时忽略上方源地址，SSH 和订阅保留可在主卡片中按需移除或恢复。
          </v-alert>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" :disabled="savingRule" @click="closeRuleDialog">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="savingRule" :disabled="savingRule" @click="saveRule">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- DIALOG: GeoIP 规则新建与编辑 -->
    <v-dialog v-model="geoDialogVisible" max-width="900" :fullscreen="smAndDown" :persistent="savingGeoRule" scrollable>
      <v-card rounded="xl" :loading="savingGeoRule">
        <v-card-title>{{ editingGeoRule.id > 0 ? '编辑 GeoIP 规则' : '新建 GeoIP 规则' }}</v-card-title>
        <v-divider />
        <v-card-text>
          <v-row>
            <v-col cols="12" md="6">
              <v-text-field
                v-model="editingGeoRule.name"
                label="规则名称"
                placeholder="留空自动按动作、国家和端口生成"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="editingGeoRule.family"
                :items="familyItems"
                label="IP 栈"
                hide-details />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12" md="4">
              <v-select
                v-model="editingGeoRule.protocol"
                :items="geoProtocolItems"
                label="协议"
                hide-details />
            </v-col>
            <v-col cols="12" md="4">
              <v-text-field
                v-model="editingGeoRule.portSpec"
                label="端口 / 端口段"
                placeholder="443 / 80-8080 / 80,443"
                hide-details />
            </v-col>
            <v-col cols="12" md="4">
              <v-select
                v-model="editingGeoRule.action"
                :items="geoActionItems"
                label="动作"
                hide-details />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12" md="4">
              <v-combobox
                v-model="editingGeoRule.countryCode"
                :items="firewallGeoCountryCodeOptions"
                label="国家代码 / GeoIP 名称"
                placeholder="US / JP / PRIVATE"
                clearable
                hide-details />
            </v-col>
            <v-col cols="12" md="8">
              <v-combobox
                v-model="editingGeoRule.sourceProviders"
                :items="firewallGeoSourceProviderOptions"
                item-title="title"
                item-value="value"
                label="规则集来源顺序"
                placeholder="留空使用默认顺序：JSON 优先，其次 Clash"
                multiple
                chips
                closable-chips
                hide-details />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12">
              <v-textarea
                v-model="editingGeoRule.customSourceUrls"
                label="完整规则集来源 URL"
                placeholder="支持 .srs / .mrs / .json / .txt，一行一个，或使用英文逗号分隔"
                rows="4"
                auto-grow
                hide-details />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12">
              <v-textarea
                v-model="editingGeoRule.description"
                label="备注"
                rows="3"
                auto-grow
                hide-details />
            </v-col>
          </v-row>

          <v-alert
            v-if="geoDialogUsesCustomSources"
            variant="tonal"
            type="warning"
            density="comfortable"
            class="mt-4">
            当前填写了完整规则集 URL。保存时会逐个下载并校验可用性，成功后合并写入 Promanager_data/geoip；删除规则时会同步删除对应缓存文件。
          </v-alert>
          <v-alert
            v-else
            variant="tonal"
            type="info"
            density="comfortable"
            class="mt-4">
            未填写完整 URL 时，会按来源列表从上到下依次尝试；默认优先 JSON 规则集，再尝试 Clash 规则集，命中第一个可用源后停止。
          </v-alert>

          <v-alert variant="tonal" type="info" density="comfortable" class="mt-4">
            这里的 GeoIP 规则与普通规则表独立保存。防火墙关闭时依然可以创建、编辑并按更新周期刷新缓存；开启后会热更新替换内存中的旧规则并同步更新实体文件。
          </v-alert>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" :disabled="savingGeoRule" @click="closeGeoRuleDialog">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="savingGeoRule" :disabled="savingGeoRule" @click="saveGeoRule">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useDisplay } from 'vuetify'
import { firewallGeoCountryCodeOptions, firewallGeoSourceProviderOptions } from './SettingsFirewallGeoOptions'
import {
  headers,
  geoHeaders,
  familyItems,
  familyFilterItems,
  originFilterItems,
  protocolItems,
  sourceModeItems,
  geoProtocolItems,
  geoActionItems,
  geoActionFilterItems,
  formatPortList,
  formatTimestamp,
  formatMetricCount,
  familyLabel,
  protocolLabel,
  originLabel,
  originColor,
  listenerStackLabel,
  listenerStackColor,
  formatListenerOwners,
  formatListenerCommand,
  listenerKey,
  geoActionLabel,
  geoActionColor,
  geoCountryLabel,
  geoStatusColor,
  geoStatusLabel,
  geoProviderSummary,
  geoResolvedSourceSummary,
  geoCacheSummary,
  isIcmpProtocol,
  ruleNeedsListenerTracking,
  sourceModeForRule,
  useFirewallManage,
} from './SettingsFirewallManage.shared'

const props = withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})

const { smAndDown } = useDisplay()

const {
  activeTab,
  loading,
  hasLoaded,
  loadError,
  refreshing,
  switchBusy,
  installingNftables,
  nftablesInstallTask,
  nftablesStopRequestPending,
  savingSSHPort,
  switchingSSHProxy,
  systemRuleBusyKey,
  savingRule,
  savingGeoRule,
  savingGeoSettings,
  geoRefreshing,
  overview,
  switchEnabled,
  sshPortInput,
  sshPortInputTouched,
  sshProxySwitch,
  searchText,
  familyFilter,
  originFilter,
  dialogVisible,
  geoDialogVisible,
  sshDialogVisible,
  geoSearchText,
  geoFamilyFilter,
  geoActionFilter,
  geoIntervalInput,
  geoSettingsDirty,
  editingRule,
  editingGeoRule,

  // Computeds
  sshPortInputValid,
  canSaveSSHPort,
  sshProxyHintText,
  sshProxyHintClass,
  lastSyncLabel,
  nftCapabilityLabel,
  nftCapabilityChipColor,
  hasActiveNftablesInstall,
  hasTerminalNftablesInstallTask,
  hasFirewallWriteInProgress,
  showNftablesInstallTask,
  nftablesStopButtonLabel,
  nftablesInstallTerminalText,
  geoLastRefreshLabel,
  geoDialogUsesCustomSources,
  editingRuleUsesFixedFamily,
  editingRuleNeedsPort,
  editingRuleNeedsSource,
  rulePortPlaceholder,
  ruleSourcePlaceholder,
  filteredRules,
  filteredGeoRules,

  // Methods
  systemRuleStatusLabel,
  systemRuleStatusColor,
  setSystemRuleReserved,
  fetchOverview,
  refreshOverview,
  installNftables,
  stopNftablesInstall,
  onToggleFirewall,
  onSSHPortFocus,
  onSSHPortBlur,
  saveSSHPort,
  onToggleSSHProxy,
  openRuleDialog,
  closeRuleDialog,
  saveRule,
  removeRule,
  openGeoRuleDialog,
  closeGeoRuleDialog,
  saveGeoRule,
  removeGeoRule,
  refreshGeoRules,
  saveGeoSettings,
} = useFirewallManage(props)

const externalRules = computed(() => {
  return overview.value.rules.filter(rule => rule.origin === 'external')
})
</script>

<style scoped>
.firewall-page {
  min-height: 460px;
}

.firewall-hero {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(99, 179, 237, 0.18);
  background:
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.18), transparent 36%),
    linear-gradient(135deg, rgba(20, 27, 38, 0.96), rgba(32, 41, 58, 0.96));
}

.firewall-hero__bg {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(90deg, rgba(255, 255, 255, 0.03) 1px, transparent 1px),
    linear-gradient(rgba(255, 255, 255, 0.03) 1px, transparent 1px);
  background-size: 18px 18px;
  mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.9), transparent);
}

.firewall-hero__content {
  position: relative;
  z-index: 1;
}

.firewall-hero__title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.firewall-hero__controls {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  flex-wrap: wrap;
}

.firewall-hero__icon {
  width: 54px;
  height: 54px;
  border-radius: 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #dbeafe;
  background: linear-gradient(135deg, rgba(37, 99, 235, 0.65), rgba(16, 185, 129, 0.35));
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.06);
}

.firewall-hero__eyebrow {
  letter-spacing: 0.2em;
  color: rgba(191, 219, 254, 0.92);
}

.firewall-hero__ssh-btn {
  height: 48px;
  border-radius: 14px;
  padding: 0 16px;
}

.firewall-hero__switch {
  min-width: 104px;
  padding: 8px 12px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.42);
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.firewall-hero__install {
  min-width: 180px;
  padding: 8px 12px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.42);
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.firewall-nftables-progress {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-width: 0;
  color: rgba(219, 234, 254, 0.96);
  font-size: 12px;
}

.firewall-hero__chips {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 14px;
}

.firewall-hero-chip {
  min-height: 26px;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.12);
}

.firewall-hero-chip--running {
  background: #15803d !important;
  color: #f0fdf4 !important;
}

.firewall-hero-chip--stopped {
  background: #475569 !important;
  color: #f8fafc !important;
}

.firewall-hero-chip--ports {
  background: #1d4ed8 !important;
  color: #eff6ff !important;
}

.firewall-hero-chip--sync {
  background: #92400e !important;
  color: #fef3c7 !important;
}

/* 紧凑指标条 */
.firewall-metric-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  padding-top: 8px;
}

.firewall-metric-item {
  flex: 1 1 120px;
  display: flex;
  flex-direction: column;
  padding: 8px 14px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.46);
  border: 1px solid rgba(148, 163, 184, 0.12);
}

/* 系统保留端口快速栏 */
.firewall-reserved-bar {
  background: rgba(15, 23, 42, 0.32);
}

.firewall-reserved-item {
  display: inline-flex;
  align-items: center;
}

/* 标签页与卡片 */
.firewall-rules__toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.firewall-table {
  border: 1px solid rgba(148, 163, 184, 0.12);
}

.firewall-protocol-chip {
  background: #f3e3a1 !important;
  color: #5f4a00 !important;
  border: 1px solid rgba(255, 245, 204, 0.5);
}

.firewall-source-cell {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  min-width: 0;
}

.firewall-listener-entry {
  padding: 6px 8px;
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.32);
  border: 1px solid rgba(148, 163, 184, 0.1);
}

.firewall-listener-command {
  word-break: break-all;
}

.firewall-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 140px;
  color: rgba(255, 255, 255, 0.72);
}

.firewall-geo__toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.firewall-geo__interval {
  min-width: 140px;
}

.firewall-diag-stats {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.firewall-diag-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 0;
  border-bottom: 1px dashed rgba(148, 163, 184, 0.12);
  font-size: 13px;
}

.firewall-diag-row:last-child {
  border-bottom: none;
}

/* 移动端卡片样式 */
.firewall-mobile-list {
  display: flex;
  flex-direction: column;
}

.firewall-mobile-card {
  border: 1px solid rgba(148, 163, 184, 0.16);
  background: rgba(15, 23, 42, 0.28);
}

.firewall-mobile-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
  font-size: 13px;
}

@media (max-width: 960px) {
  .firewall-hero__controls {
    width: 100%;
    justify-content: stretch;
  }

  .firewall-hero__ssh-btn {
    flex: 1 1 100%;
  }

  .firewall-hero__switch {
    flex: 1 1 100%;
  }

  .firewall-hero__install {
    flex: 1 1 100%;
  }

  .firewall-geo__toolbar-right {
    width: 100%;
  }

  .firewall-geo__interval {
    width: 100%;
  }
}
</style>
