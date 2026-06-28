<template>
  <div class="firewall-page">
    <v-row class="mb-4">
      <v-col cols="12" xl="8">
        <v-card class="firewall-hero" rounded="xl" :loading="loading && !overview.available">
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
                    以面板托管链路接管入站放行；SSH / 订阅按需保留。外部扫描结果仅用于展示和删除，不会自动并入面板放行链。
                  </div>
                </div>
              </div>
              <div class="firewall-hero__controls">
	                <div
	                  v-if="overview.nftables.supported && !overview.nftables.installed"
	                  class="firewall-hero__install">
	                  <div class="text-caption text-medium-emphasis mb-2">缺少 nft 命令</div>
                  <v-btn
                    color="primary"
                    prepend-icon="mdi-download"
                    :loading="installingNftables"
                    :disabled="loading || installingNftables"
                    @click="installNftables">
                    下载 nftables
                  </v-btn>
	                  <div class="text-caption text-medium-emphasis mt-2">
	                    {{ overview.nftables.packageManager || overview.nftables.systemFamily || 'Linux' }}
	                  </div>
	                  <div class="text-caption text-medium-emphasis mt-1">
	                    自动安装仅执行包管理器安装与服务启动，不再改写系统软件源配置。
	                  </div>
	                </div>
                <div class="firewall-hero__switch">
                  <div class="text-caption text-medium-emphasis mb-1">总开关</div>
                  <v-switch
                    :model-value="switchEnabled"
                    :disabled="!overview.available || switchBusy"
                    :loading="switchBusy"
                    color="success"
                    inset
                    hide-details
                    @update:modelValue="onToggleFirewall" />
                </div>
              </div>
            </div>

            <div class="firewall-hero__chips">
              <v-chip size="small" :color="overview.enabled ? 'success' : 'grey'" variant="flat">
                {{ overview.enabled ? '运行中' : '已关闭' }}
              </v-chip>
              <v-chip size="small" color="info" variant="tonal">
                当前保留 {{ formatPortList(overview.defaultPorts.active) }}
              </v-chip>
              <v-chip size="small" color="warning" variant="tonal">
                上次同步：{{ lastSyncLabel }}
              </v-chip>
            </div>

            <div class="firewall-ssh-row">
              <div class="firewall-ssh-card">
                <v-text-field
                  v-model="sshPortInput"
                  label="SSH端口"
                  type="number"
                  min="1"
                  max="65535"
                  hide-details="auto"
                  class="firewall-ssh-port-input"
                  :disabled="savingSSHPort || switchingSSHProxy || !overview.sshConfig.supported"
                  :error="sshPortInputTouched && !sshPortInputValid"
                  :error-messages="sshPortInputTouched && !sshPortInputValid ? ['端口必须是 1-65535 的正整数'] : []"
                  @focus="onSSHPortFocus"
                  @blur="onSSHPortBlur" />
                <div class="text-caption text-medium-emphasis mt-1">
                  检测端口：{{ formatPortList(overview.sshConfig.ports) }} · 配置：{{ overview.sshConfig.configPath || '-' }}
                </div>
              </div>

              <div class="firewall-ssh-card firewall-ssh-card--action">
                <v-btn
                  color="primary"
                  prepend-icon="mdi-content-save-outline"
                  :loading="savingSSHPort"
                  :disabled="!canSaveSSHPort || switchingSSHProxy"
                  @click="saveSSHPort">
                  保存
                </v-btn>
                <div class="text-caption text-medium-emphasis mt-2">保存后会重启 SSH 服务使端口生效</div>
              </div>

              <div class="firewall-ssh-card">
                <div class="d-flex align-center justify-space-between ga-4">
                  <div>
                    <div class="text-subtitle-2">是否开启代理</div>
                    <div class="text-caption text-medium-emphasis mt-1">
                      开启后会设置 AllowTcpForwarding / PermitOpen / GatewayPorts
                    </div>
                  </div>
                  <v-switch
                    :model-value="sshProxySwitch"
                    :disabled="switchingSSHProxy || savingSSHPort || !overview.sshConfig.supported"
                    :loading="switchingSSHProxy"
                    color="success"
                    hide-details
                    inset
                    @update:modelValue="onToggleSSHProxy" />
                </div>
                <div class="text-caption mt-2" :class="sshProxyHintClass">
                  {{ sshProxyHintText }}
                </div>
              </div>
            </div>

            <v-row class="mt-2">
              <v-col cols="12" sm="6" md="3">
                <div class="firewall-metric">
                  <div class="text-caption text-medium-emphasis">总规则数</div>
                  <div class="text-h5 mt-1">{{ overview.totalCount }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="firewall-metric">
                  <div class="text-caption text-medium-emphasis">面板规则</div>
                  <div class="text-h5 mt-1">{{ overview.manualCount }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="firewall-metric">
                  <div class="text-caption text-medium-emphasis">ACME 临时</div>
                  <div class="text-h5 mt-1">{{ overview.temporaryCount }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="firewall-metric">
                  <div class="text-caption text-medium-emphasis">系统保留</div>
                  <div class="text-h5 mt-1">{{ overview.systemCount }}</div>
                </div>
              </v-col>
            </v-row>
            <v-row class="mt-2">
              <v-col cols="12" md="4">
                <div class="firewall-metric firewall-metric--detail">
                  <div class="text-caption text-medium-emphasis">外部扫描</div>
                  <div class="text-h5 mt-1">{{ overview.externalCount }}</div>
                  <div class="text-caption text-medium-emphasis mt-2">系统中已扫描到的外部放行规则数，仅供展示与清理，不代表会被面板托管链自动放行</div>
                </div>
              </v-col>
              <v-col cols="12" md="4">
                <div class="firewall-metric firewall-metric--detail">
                  <div class="firewall-tcp-metric">
                    <div class="firewall-tcp-metric__primary">
                      <div class="text-caption text-medium-emphasis">系统活跃</div>
                      <div class="text-h5 mt-1">{{ formatMetricCount(overview.tcpActiveCount) }}</div>
                    </div>
                    <div class="firewall-tcp-metric__meta">
                      <div class="firewall-tcp-metric__meta-row">
                        <span>已建立</span>
                        <strong>{{ formatMetricCount(overview.tcpEstablishedCount) }}</strong>
                      </div>
                      <div class="firewall-tcp-metric__meta-row">
                        <span>TCP 半开</span>
                        <strong>{{ formatMetricCount(overview.tcpSynRecvCount) }}</strong>
                      </div>
                      <div class="firewall-tcp-metric__meta-row">
                        <span>异常累计</span>
                        <strong>{{ formatMetricCount(overview.tcpAnomalyTotal) }}</strong>
                      </div>
                    </div>
                  </div>
                  <div class="text-caption text-medium-emphasis mt-2">按系统全局 TCP 状态统计，不限端口；系统活跃包含已建立、TCP 半开，以及 SYN_SENT / FIN_WAIT / CLOSE_WAIT / LAST_ACK / CLOSING 等活动态连接，异常累计为系统自启动以来的 Syncookies / ListenDrops / ListenOverflows 总和。</div>
                </div>
              </v-col>
              <v-col cols="12" md="4">
                <div class="firewall-metric firewall-metric--detail">
                  <div class="d-flex align-start justify-space-between ga-3 flex-wrap">
                    <div>
                      <div class="text-caption text-medium-emphasis">UDP 当前</div>
                      <div class="text-h5 mt-1">{{ formatMetricCount(overview.udpSocketCount) }}</div>
                    </div>
                    <div class="firewall-metric__meta">
                      <div>异常累计 {{ formatMetricCount(overview.udpAnomalyTotal) }}</div>
                    </div>
                  </div>
                  <div class="text-caption text-medium-emphasis mt-2">按系统全局 UDP socket 条目统计，不限端口；异常累计为系统自启动以来的 NoPorts / InErrors / RcvbufErrors 总和。</div>
                </div>
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" xl="4">
        <v-card class="firewall-side" rounded="xl" variant="outlined">
          <v-card-title class="text-subtitle-1 font-weight-medium">系统保留端口</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="firewall-side__row">
              <div>
                <span>SSH</span>
                <div class="text-caption text-medium-emphasis mt-1">{{ formatPortList(overview.defaultPorts.ssh) }}</div>
              </div>
              <div class="d-flex align-center ga-2 flex-wrap justify-end">
                <v-chip size="small" :color="systemRuleStatusColor('ssh')" variant="flat">
                  {{ systemRuleStatusLabel('ssh') }}
                </v-chip>
                <v-btn
                  size="small"
                  variant="text"
                  color="warning"
                  :loading="systemRuleBusyKey === 'ssh'"
                  @click="setSystemRuleReserved('ssh', !overview.defaultPorts.sshReserved)">
                  {{ overview.defaultPorts.sshReserved ? '移除保留' : '恢复保留' }}
                </v-btn>
              </div>
            </div>
            <div class="firewall-side__row">
              <div>
                <span>界面</span>
                <div class="text-caption text-medium-emphasis mt-1">{{ formatPortList(overview.defaultPorts.panel) }}</div>
              </div>
              <v-chip size="small" :color="systemRuleStatusColor('panel')" variant="flat">
                {{ systemRuleStatusLabel('panel') }}
              </v-chip>
            </div>
            <div class="firewall-side__row">
              <div>
                <span>订阅</span>
                <div class="text-caption text-medium-emphasis mt-1">{{ formatPortList(overview.defaultPorts.sub) }}</div>
              </div>
              <div class="d-flex align-center ga-2 flex-wrap justify-end">
                <v-chip size="small" :color="systemRuleStatusColor('sub')" variant="flat">
                  {{ systemRuleStatusLabel('sub') }}
                </v-chip>
                <v-btn
                  size="small"
                  variant="text"
                  color="warning"
                  :loading="systemRuleBusyKey === 'sub'"
                  @click="setSystemRuleReserved('sub', !overview.defaultPorts.subReserved)">
                  {{ overview.defaultPorts.subReserved ? '移除保留' : '恢复保留' }}
                </v-btn>
              </div>
            </div>
            <v-alert
              variant="tonal"
              type="info"
              density="comfortable"
              class="mt-4">
              开启时会重建面板自己的防火墙链，只放行系统保留端口、GeoIP 白名单命中项和面板规则；外部扫描规则仅用于展示与删除。界面保留始终强制存在，SSH 和订阅可按需移除或恢复。
            </v-alert>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-alert
      v-if="overview.error"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      {{ overview.error }}
    </v-alert>

    <v-card rounded="xl" variant="outlined" class="firewall-rules">
      <v-card-title class="firewall-rules__toolbar">
        <div>
          <div class="text-subtitle-1 font-weight-medium">规则列表</div>
          <div class="text-caption text-medium-emphasis mt-1">
            支持 IPv4 / IPv6 / 双栈、端口段，以及源 IP / CIDR 限制。监听状态会随页面轮询自动刷新。
          </div>
        </div>
        <div class="d-flex align-center ga-2">
          <v-btn
            variant="tonal"
            color="info"
            prepend-icon="mdi-refresh"
            :loading="refreshing"
            @click="refreshOverview">
            立即刷新
          </v-btn>
          <v-btn
            color="primary"
            prepend-icon="mdi-plus"
            :disabled="!overview.enabled"
            @click="openRuleDialog()">
            新建规则
          </v-btn>
        </div>
      </v-card-title>
      <v-divider />

      <v-card-text>
        <v-row class="mb-2">
          <v-col cols="12" md="4">
            <v-text-field
              v-model="searchText"
              label="搜索名称 / 端口 / 源地址"
              prepend-inner-icon="mdi-magnify"
              clearable
              hide-details />
          </v-col>
          <v-col cols="12" md="4">
            <v-select
              v-model="familyFilter"
              :items="familyFilterItems"
              label="双栈筛选"
              hide-details />
          </v-col>
          <v-col cols="12" md="4">
            <v-select
              v-model="originFilter"
              :items="originFilterItems"
              label="来源筛选"
              hide-details />
          </v-col>
        </v-row>

        <v-data-table
          :headers="headers"
          :items="filteredRules"
          item-value="id"
          :mobile="smAndDown"
          mobile-breakpoint="sm"
          fixed-header
          class="rounded-lg firewall-table"
          hide-no-data>
          <template #item.name="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ item.name }}</div>
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
            <span>{{ item.sourceSpec || (isIcmpProtocol(item.protocol) ? '-' : '任意来源') }}</span>
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
                当前规则无需监听探测
              </div>
              <div class="text-caption text-medium-emphasis" v-else-if="item.listenerState.listenerCount === 0">
                未检测到监听
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
                  <div class="text-caption text-medium-emphasis firewall-listener-command" v-if="formatListenerCommand(listener.owners)">
                    {{ formatListenerCommand(listener.owners) }}
                  </div>
                </div>
              </div>
              <div class="text-caption text-medium-emphasis mt-1" v-if="item.listenerState.checkedAt">
                更新于 {{ formatTimestamp(item.listenerState.checkedAt) }}
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
            <div class="d-flex align-center ga-3">
              <v-icon
                size="18"
                :color="item.canEdit ? 'primary' : 'grey'"
                :class="{ 'firewall-action--disabled': !item.canEdit }"
                @click="item.canEdit && openRuleDialog(item)">
                mdi-pencil
              </v-icon>
              <v-icon
                size="18"
                :color="item.canDelete ? 'error' : 'grey'"
                :class="{ 'firewall-action--disabled': !item.canDelete }"
                @click="item.canDelete && removeRule(item)">
                mdi-delete
              </v-icon>
            </div>
          </template>
        </v-data-table>

        <div v-if="filteredRules.length === 0" class="firewall-empty">
          <v-icon size="38" color="grey">mdi-shield-off-outline</v-icon>
          <div class="text-subtitle-2 mt-2">
            {{ overview.enabled ? '当前没有匹配到规则' : '防火墙关闭后仅停止下发 nftables 规则，当前规则记录会保留' }}
          </div>
        </div>
      </v-card-text>
    </v-card>

    <v-card rounded="xl" variant="outlined" class="firewall-rules firewall-geo">
      <v-card-title class="firewall-rules__toolbar">
        <div>
          <div class="text-subtitle-1 font-weight-medium">来源 IP 识别</div>
          <div class="text-caption text-medium-emphasis mt-1">
            与上方规则列表独立维护，按国家或自定义规则集识别来源 IP，对指定端口执行阻断或只放行。
          </div>
        </div>
        <div class="firewall-geo__toolbar-right">
          <v-text-field
            v-model="geoIntervalInput"
            class="firewall-geo__interval"
            label="更新周期(分钟)"
            type="number"
            min="1"
            hide-details />
          <v-btn
            variant="tonal"
            color="secondary"
            prepend-icon="mdi-content-save-outline"
            :disabled="!geoSettingsDirty"
            :loading="savingGeoSettings"
            @click="saveGeoSettings">
            保存周期
          </v-btn>
          <v-btn
            variant="tonal"
            color="info"
            prepend-icon="mdi-database-sync-outline"
            :loading="geoRefreshing"
            @click="refreshGeoRules">
            立即更新
          </v-btn>
          <v-btn
            color="primary"
            prepend-icon="mdi-plus"
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
          GeoIP 规则会先于上方放行规则进行匹配；同端口选择“只放行”时，会把该端口变成来源白名单，仅允许命中国家访问，其余来源会直接丢弃，后面的普通放行规则不会再接管这部分流量。规则文件缓存目录：Promanager_data/geoip。当前最近一次全局刷新：{{ geoLastRefreshLabel }}。
        </v-alert>

        <v-row class="mb-2">
          <v-col cols="12" md="5">
            <v-text-field
              v-model="geoSearchText"
              label="搜索名称 / 端口 / 国家代码 / 来源"
              prepend-inner-icon="mdi-magnify"
              clearable
              hide-details />
          </v-col>
          <v-col cols="12" md="3">
            <v-select
              v-model="geoActionFilter"
              :items="geoActionFilterItems"
              label="动作筛选"
              hide-details />
          </v-col>
          <v-col cols="12" md="4">
            <v-select
              v-model="geoFamilyFilter"
              :items="familyFilterItems"
              label="双栈筛选"
              hide-details />
          </v-col>
        </v-row>

        <v-data-table
          :headers="geoHeaders"
          :items="filteredGeoRules"
          item-value="id"
          :mobile="smAndDown"
          mobile-breakpoint="sm"
          fixed-header
          class="rounded-lg firewall-table"
          hide-no-data>
          <template #item.name="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ item.name }}</div>
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
            <div class="d-flex align-center ga-3">
              <v-icon
                size="18"
                color="primary"
                @click="openGeoRuleDialog(item)">
                mdi-pencil
              </v-icon>
              <v-icon
                size="18"
                color="error"
                @click="removeGeoRule(item)">
                mdi-delete
              </v-icon>
            </div>
          </template>
        </v-data-table>

        <div v-if="filteredGeoRules.length === 0" class="firewall-empty">
          <v-icon size="38" color="grey">mdi-map-search-outline</v-icon>
          <div class="text-subtitle-2 mt-2">
            {{ overview.geoRuleCount > 0 ? '当前筛选条件下没有 GeoIP 规则' : '还没有来源 IP 识别规则，可按端口或端口范围创建' }}
          </div>
        </div>
      </v-card-text>
    </v-card>

    <v-dialog v-model="dialogVisible" max-width="760">
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
              <v-textarea
                v-model="editingRule.description"
                label="备注"
                rows="3"
                auto-grow
                hide-details />
            </v-col>
          </v-row>
          <v-alert variant="tonal" type="info" density="comfortable" class="mt-4">
            规则保存后会立即重建面板自己的防火墙链；界面保留不可编辑也不可删除，SSH 和订阅保留可在页面主卡片里按需移除或恢复。
          </v-alert>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="closeRuleDialog">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="savingRule" @click="saveRule">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="geoDialogVisible" max-width="900">
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
            未填写完整 URL 时，会按来源列表从上到下依次尝试；默认优先 JSON 规则集，再尝试 Clash 规则集，命中第一个可用源后停止，不会继续向后重复加载。
          </v-alert>

          <v-alert variant="tonal" type="info" density="comfortable" class="mt-4">
            这里的 GeoIP 规则与上方规则表独立保存。防火墙关闭时依然可以创建、编辑并按更新周期刷新缓存；开启后会热更新替换内存中的旧规则并同步更新实体文件。GeoIP 规则只影响面板托管链，不会自动吸收系统里其他外部放行规则。
          </v-alert>
        </v-card-text>
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="closeGeoRuleDialog">取消</v-btn>
          <v-btn color="primary" variant="tonal" :loading="savingGeoRule" @click="saveGeoRule">
            保存
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import HttpUtils from '@/plugins/httputil'
import { firewallGeoCountryCodeOptions, firewallGeoSourceProviderOptions } from './SettingsFirewallGeoOptions'
import { push } from 'notivue'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useDisplay } from 'vuetify'

type FirewallRule = {
  id: number
  name: string
  description: string
  enabled: boolean
  origin: string
  systemKey: string
  temporaryType: string
  temporaryExpireAt: number
  family: string
  protocol: string
  portSpec: string
  sourceSpec: string
  canEdit: boolean
  canDelete: boolean
  listenerState: FirewallRuleListenerState
}

type FirewallListenerOwner = {
  pid: number
  name: string
  command: string
  executable: string
}

type FirewallPortListener = {
  port: number
  protocol: string
  socketFamily: string
  stack: string
  stackSource: string
  bindAddress: string
  owners: FirewallListenerOwner[]
}

type FirewallRuleListenerState = {
  supported: boolean
  checkedAt: number
  occupied: boolean
  listenerCount: number
  listeners: FirewallPortListener[]
  error?: string
}

type FirewallGeoRule = {
  id: number
  name: string
  description: string
  enabled: boolean
  family: string
  protocol: string
  portSpec: string
  action: string
  countryCode: string
  sourceProviders: string[]
  customSourceUrls: string[]
  resolvedSources: string[]
  cachedFiles: string[]
  contentHash: string
  prefixCount: number
  lastRefreshAt: number
  lastRefreshError: string
}

type FirewallSSHConfig = {
  supported: boolean
  configPath: string
  ports: number[]
  port: number
  proxyEnabled: boolean
  allowTcpForwarding: string
  permitOpen: string
  gatewayPorts: string
  error?: string
}

type FirewallNftablesStatus = {
  supported: boolean
  installed: boolean
  autoInstallSupported: boolean
  binaryPath: string
  systemFamily: string
  packageManager: string
  manualCommands: string[]
  reason: string
}

type FirewallOverview = {
  enabled: boolean
  available: boolean
  mode: string
  nftables: FirewallNftablesStatus
  lastSyncAt: number
  defaultPorts: {
    ssh: number[]
    panel: number[]
    sub: number[]
    all: number[]
    active: number[]
    sshReserved: boolean
    panelReserved: boolean
    subReserved: boolean
  }
  sshConfig: FirewallSSHConfig
  tcpActiveCount: number
  tcpSynRecvCount: number
  tcpEstablishedCount: number
  tcpAnomalyTotal: number
  udpSocketCount: number
  udpAnomalyTotal: number
  manualCount: number
  temporaryCount: number
  externalCount: number
  systemCount: number
  totalCount: number
  rules: FirewallRule[]
  geoRuleCount: number
  geoUpdateIntervalMinutes: number
  geoLastRefreshAt: number
  geoRules: FirewallGeoRule[]
  error?: string
}

type FirewallRuleForm = {
  id: number
  name: string
  description: string
  family: string
  protocol: string
  portSpec: string
  sourceSpec: string
}

type FirewallGeoRuleForm = {
  id: number
  name: string
  description: string
  family: string
  protocol: string
  portSpec: string
  action: string
  countryCode: string
  sourceProviders: string[]
  customSourceUrls: string
}

const props = withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})

const { smAndDown } = useDisplay()

const emptyOverview = (): FirewallOverview => ({
  enabled: false,
  available: true,
  mode: 'nftables',
  nftables: {
    supported: false,
    installed: false,
    autoInstallSupported: false,
    binaryPath: '',
    systemFamily: '',
    packageManager: '',
    manualCommands: [],
    reason: 'not_linux',
  },
  lastSyncAt: 0,
  defaultPorts: {
    ssh: [],
    panel: [],
    sub: [],
    all: [],
    active: [],
    sshReserved: true,
    panelReserved: true,
    subReserved: true,
  },
  sshConfig: {
    supported: true,
    configPath: '',
    ports: [22],
    port: 22,
    proxyEnabled: false,
    allowTcpForwarding: '',
    permitOpen: '',
    gatewayPorts: '',
  },
  tcpActiveCount: 0,
  tcpSynRecvCount: 0,
  tcpEstablishedCount: 0,
  tcpAnomalyTotal: 0,
  udpSocketCount: 0,
  udpAnomalyTotal: 0,
  manualCount: 0,
  temporaryCount: 0,
  externalCount: 0,
  systemCount: 0,
  totalCount: 0,
  rules: [],
  geoRuleCount: 0,
  geoUpdateIntervalMinutes: 360,
  geoLastRefreshAt: 0,
  geoRules: [],
})

const createEmptyRuleForm = (): FirewallRuleForm => ({
  id: 0,
  name: '',
  description: '',
  family: 'dual',
  protocol: 'tcp_udp',
  portSpec: '',
  sourceSpec: '',
})

const createEmptyGeoRuleForm = (): FirewallGeoRuleForm => ({
  id: 0,
  name: '',
  description: '',
  family: 'dual',
  protocol: 'tcp_udp',
  portSpec: '',
  action: 'block',
  countryCode: '',
  sourceProviders: [],
  customSourceUrls: '',
})

const loading = ref(false)
const refreshing = ref(false)
const switchBusy = ref(false)
const installingNftables = ref(false)
const savingSSHPort = ref(false)
const switchingSSHProxy = ref(false)
const systemRuleBusyKey = ref('')
const savingRule = ref(false)
const savingGeoRule = ref(false)
const savingGeoSettings = ref(false)
const geoRefreshing = ref(false)
const overview = ref<FirewallOverview>(emptyOverview())
const switchEnabled = ref(false)
const sshPortInput = ref('22')
const sshPortInputTouched = ref(false)
const sshPortDirty = ref(false)
const sshProxySwitch = ref(false)
const searchText = ref('')
const familyFilter = ref('all')
const originFilter = ref('all')
const dialogVisible = ref(false)
const geoDialogVisible = ref(false)
const pollTimer = ref<number | null>(null)
const geoSearchText = ref('')
const geoFamilyFilter = ref('all')
const geoActionFilter = ref('all')
const geoIntervalInput = ref('360')
const geoSettingsDirty = ref(false)
const syncingGeoInterval = ref(false)

const editingRule = ref<FirewallRuleForm>(createEmptyRuleForm())
const editingGeoRule = ref<FirewallGeoRuleForm>(createEmptyGeoRuleForm())

const headers = [
  { title: '规则', key: 'name' },
  { title: '协议', key: 'protocol', sortable: false },
  { title: '端口', key: 'portSpec', sortable: false },
  { title: '源地址', key: 'sourceSpec', sortable: false },
  { title: '监听状态', key: 'listenerState', sortable: false, width: 360 },
  { title: '双栈', key: 'family', sortable: false },
  { title: '来源', key: 'origin', sortable: false },
  { title: '操作', key: 'actions', sortable: false, width: 120 },
]

const geoHeaders = [
  { title: '规则', key: 'name' },
  { title: '动作', key: 'action', sortable: false },
  { title: '协议', key: 'protocol', sortable: false },
  { title: '端口', key: 'portSpec', sortable: false },
  { title: '国家 / 来源', key: 'countryCode', sortable: false },
  { title: '状态', key: 'status', sortable: false },
  { title: '操作', key: 'actions', sortable: false, width: 120 },
]

const familyItems = [
  { title: '双栈', value: 'dual' },
  { title: '仅 IPv4', value: 'ipv4' },
  { title: '仅 IPv6', value: 'ipv6' },
]

const familyFilterItems = [
  { title: '全部双栈', value: 'all' },
  ...familyItems,
]

const originFilterItems = [
  { title: '全部来源', value: 'all' },
  { title: '系统保留', value: 'system' },
  { title: '面板规则', value: 'manual' },
  { title: 'ACME 临时', value: 'temporary' },
  { title: '外部扫描', value: 'external' },
]

const protocolItems = [
  { title: 'TCP + UDP', value: 'tcp_udp' },
  { title: 'TCP', value: 'tcp' },
  { title: 'UDP', value: 'udp' },
  { title: 'ICMP', value: 'icmp' },
  { title: 'ICMP v4', value: 'icmp_v4' },
  { title: 'ICMP v6', value: 'icmp_v6' },
]

const geoProtocolItems = protocolItems.filter(item => !['any', 'icmp', 'icmp_v4', 'icmp_v6'].includes(item.value))

const geoActionItems = [
  { title: '只放行', value: 'allow' },
  { title: '直接阻断', value: 'block' },
]

const geoActionFilterItems = [
  { title: '全部动作', value: 'all' },
  ...geoActionItems,
]

const geoProviderTitleMap = firewallGeoSourceProviderOptions.reduce<Record<string, string>>((result, item) => {
  result[item.value] = item.title
  return result
}, {})

const emptyListenerState = (): FirewallRuleListenerState => ({
  supported: true,
  checkedAt: 0,
  occupied: false,
  listenerCount: 0,
  listeners: [],
})

const normalizeNumberArray = (value: unknown): number[] => {
  if (!Array.isArray(value)) return []
  const result: number[] = []
  const seen = new Set<number>()
  for (const item of value) {
    const parsed = Number.parseInt(String(item ?? '').trim(), 10)
    if (!Number.isInteger(parsed) || parsed < 1 || parsed > 65535 || seen.has(parsed)) {
      continue
    }
    seen.add(parsed)
    result.push(parsed)
  }
  return result
}

const normalizeListenerOwners = (value: unknown): FirewallListenerOwner[] => {
  if (!Array.isArray(value)) return []
  return value.map((item: any) => ({
    pid: Number.parseInt(String(item?.pid ?? 0), 10) || 0,
    name: String(item?.name ?? '').trim(),
    command: String(item?.command ?? '').trim(),
    executable: String(item?.executable ?? '').trim(),
  })).filter(item => item.pid > 0 || item.name || item.command || item.executable)
}

const normalizeListenerState = (value: any): FirewallRuleListenerState => ({
  supported: value?.supported !== false,
  checkedAt: Number.parseInt(String(value?.checkedAt ?? 0), 10) || 0,
  occupied: value?.occupied === true,
  listenerCount: Number.parseInt(String(value?.listenerCount ?? 0), 10) || 0,
  listeners: Array.isArray(value?.listeners) ? value.listeners.map((item: any) => ({
    port: Number.parseInt(String(item?.port ?? 0), 10) || 0,
    protocol: String(item?.protocol ?? '').trim(),
    socketFamily: String(item?.socketFamily ?? '').trim(),
    stack: String(item?.stack ?? '').trim(),
    stackSource: String(item?.stackSource ?? '').trim(),
    bindAddress: String(item?.bindAddress ?? '').trim(),
    owners: normalizeListenerOwners(item?.owners),
  })).filter((item: FirewallPortListener) => item.port > 0) : [],
  error: typeof value?.error === 'string' ? value.error : undefined,
})

const sshPortBaseline = computed(() => {
  const port = Number.parseInt(String(overview.value.sshConfig.port || '').trim(), 10)
  if (Number.isInteger(port) && port >= 1 && port <= 65535) {
    return port
  }
  return 22
})

const sshPortInputValue = computed(() => Number.parseInt(String(sshPortInput.value || '').trim(), 10))

const sshPortInputValid = computed(() => {
  const value = sshPortInputValue.value
  return Number.isInteger(value) && value >= 1 && value <= 65535
})

const canSaveSSHPort = computed(() => {
  return overview.value.sshConfig.supported && sshPortInputValid.value && sshPortInputValue.value !== sshPortBaseline.value
})

const sshProxyHintText = computed(() => {
  if (!overview.value.sshConfig.supported) {
    return overview.value.sshConfig.error || '当前平台不支持 SSH 配置管理'
  }
  if (overview.value.sshConfig.error) {
    return `检测异常：${overview.value.sshConfig.error}`
  }
  if (overview.value.sshConfig.proxyEnabled) {
    return '当前状态：已开启（AllowTcpForwarding yes / PermitOpen any / GatewayPorts no）'
  }
  const allow = overview.value.sshConfig.allowTcpForwarding || '未配置'
  const permit = overview.value.sshConfig.permitOpen || '未配置'
  const gateway = overview.value.sshConfig.gatewayPorts || '未配置'
  return `当前状态：未开启（AllowTcpForwarding ${allow} / PermitOpen ${permit} / GatewayPorts ${gateway}）`
})

const sshProxyHintClass = computed(() => {
  if (!overview.value.sshConfig.supported || overview.value.sshConfig.error) {
    return 'text-warning'
  }
  return overview.value.sshConfig.proxyEnabled ? 'text-success' : 'text-medium-emphasis'
})

const lastSyncLabel = computed(() => {
  if (!overview.value.lastSyncAt) return '未同步'
  return formatTimestamp(overview.value.lastSyncAt)
})

const geoLastRefreshLabel = computed(() => {
  if (!overview.value.geoLastRefreshAt) return '未刷新'
  return formatTimestamp(overview.value.geoLastRefreshAt)
})

const geoDialogUsesCustomSources = computed(() => editingGeoRule.value.customSourceUrls.trim().length > 0)

const isIcmpProtocol = (protocol: string) => ['icmp', 'icmp_v4', 'icmp_v6'].includes(protocol)
const ruleNeedsListenerTracking = (rule: Pick<FirewallRule, 'protocol' | 'portSpec'>) => !isIcmpProtocol(rule.protocol) && String(rule.portSpec || '').trim().length > 0

const normalizeRuleFamilyForSubmit = (protocol: string, family: string) => {
  if (protocol === 'icmp') return 'dual'
  if (protocol === 'icmp_v4') return 'ipv4'
  if (protocol === 'icmp_v6') return 'ipv6'
  return family || 'dual'
}

const editingRuleUsesFixedFamily = computed(() => isIcmpProtocol(editingRule.value.protocol))
const editingRuleNeedsPort = computed(() => !isIcmpProtocol(editingRule.value.protocol))
const editingRuleNeedsSource = computed(() => !isIcmpProtocol(editingRule.value.protocol))
const rulePortPlaceholder = computed(() => {
  if (isIcmpProtocol(editingRule.value.protocol)) return 'ICMP rules do not use ports'
  return '22 / 80,443 / 10000-10100'
})
const ruleSourcePlaceholder = computed(() => {
  if (isIcmpProtocol(editingRule.value.protocol)) return 'ICMP rules do not use source filters'
  return '留空表示任意来源，例如：1.2.3.4/32, 2001:db8::/64'
})

const filteredRules = computed(() => {
  const keyword = searchText.value.trim().toLowerCase()
  return overview.value.rules.filter(rule => {
    if (familyFilter.value !== 'all' && rule.family !== familyFilter.value) {
      return false
    }
    if (originFilter.value !== 'all' && rule.origin !== originFilter.value) {
      return false
    }
    if (!keyword) {
      return true
    }
    return [
      rule.name,
      rule.description,
      rule.portSpec,
      rule.sourceSpec,
      rule.origin,
    ].some(value => (value || '').toLowerCase().includes(keyword))
  })
})

const filteredGeoRules = computed(() => {
  const keyword = geoSearchText.value.trim().toLowerCase()
  return overview.value.geoRules.filter(rule => {
    if (geoFamilyFilter.value !== 'all' && rule.family !== geoFamilyFilter.value) {
      return false
    }
    if (geoActionFilter.value !== 'all' && rule.action !== geoActionFilter.value) {
      return false
    }
    if (!keyword) {
      return true
    }
    return [
      rule.name,
      rule.description,
      rule.portSpec,
      rule.countryCode,
      rule.action,
      ...rule.sourceProviders,
      ...rule.customSourceUrls,
      ...rule.resolvedSources,
      rule.lastRefreshError,
    ].some(value => (value || '').toLowerCase().includes(keyword))
  })
})

const formatPortList = (ports: number[]) => {
  if (!ports || ports.length === 0) return '-'
  return ports.join(', ')
}

const formatTimestamp = (timestamp: number) => {
  if (!timestamp) return '未刷新'
  return new Date(timestamp * 1000).toLocaleString()
}

const formatMetricCount = (value: number) => {
  const normalized = Number(value ?? 0)
  if (!Number.isFinite(normalized) || normalized <= 0) return '0'
  return normalized.toLocaleString('zh-CN')
}

const familyLabel = (family: string) => {
  if (family === 'ipv4') return 'IPv4'
  if (family === 'ipv6') return 'IPv6'
  return '双栈'
}

const protocolLabel = (protocol: string) => {
  if (protocol === 'tcp') return 'TCP'
  if (protocol === 'udp') return 'UDP'
  if (protocol === 'icmp') return 'ICMP'
  if (protocol === 'icmp_v4') return 'ICMP v4'
  if (protocol === 'icmp_v6') return 'ICMP v6'
  if (protocol === 'any') return 'ANY'
  return 'TCP+UDP'
}

const originLabel = (origin: string) => {
  if (origin === 'system') return '系统保留'
  if (origin === 'temporary') return 'ACME 临时'
  if (origin === 'external') return '外部扫描'
  return '面板规则'
}

const originColor = (origin: string) => {
  if (origin === 'system') return 'success'
  if (origin === 'temporary') return 'info'
  if (origin === 'external') return 'warning'
  return 'primary'
}

const systemRuleReserved = (systemKey: string) => {
  if (systemKey === 'ssh') return overview.value.defaultPorts.sshReserved
  if (systemKey === 'sub') return overview.value.defaultPorts.subReserved
  if (systemKey === 'panel') return overview.value.defaultPorts.panelReserved
  return false
}

const systemRuleStatusLabel = (systemKey: string) => {
  if (systemKey === 'panel') return '强制保留'
  return systemRuleReserved(systemKey) ? '已保留' : '已移除'
}

const systemRuleStatusColor = (systemKey: string) => {
  if (systemKey === 'panel') return 'info'
  return systemRuleReserved(systemKey) ? 'success' : 'warning'
}

const listenerStackLabel = (listener: FirewallPortListener) => {
  const suffix = listener.stackSource === 'inferred'
    ? '（推断）'
    : listener.stackSource === 'unknown'
      ? '（待确认）'
      : ''
  if (listener.stack === 'dual') return `双栈${suffix}`
  if (listener.stack === 'ipv4') return `仅 IPv4${suffix}`
  if (listener.stack === 'ipv6') return `仅 IPv6${suffix}`
  return `未知${suffix}`
}

const listenerStackColor = (listener: FirewallPortListener) => {
  if (listener.stack === 'dual') return 'success'
  if (listener.stack === 'ipv4') return 'info'
  if (listener.stack === 'ipv6') return 'secondary'
  return 'warning'
}

const formatListenerOwners = (owners: FirewallListenerOwner[]) => {
  if (!owners || owners.length === 0) {
    return '未解析到进程信息'
  }
  return owners.map(owner => {
    const label = owner.name || owner.executable || owner.command || `pid ${owner.pid}`
    return `${label} (PID ${owner.pid})`
  }).join(' / ')
}

const formatListenerCommand = (owners: FirewallListenerOwner[]) => {
  if (!owners || owners.length === 0) return ''
  const command = owners[0].command || owners[0].executable || ''
  if (!command) return ''
  if (owners.length === 1) return command
  return `${command} 等 ${owners.length} 个进程`
}

const listenerKey = (listener: FirewallPortListener) => {
  const ownerKey = listener.owners.map(owner => owner.pid).join(',')
  return `${listener.port}-${listener.protocol}-${listener.socketFamily}-${listener.stack}-${listener.bindAddress}-${ownerKey}`
}

const geoActionLabel = (action: string) => {
  if (action === 'allow') return '只放行'
  return '直接阻断'
}

const geoActionColor = (action: string) => {
  if (action === 'allow') return 'success'
  return 'error'
}

const geoCountryLabel = (countryCode: string) => {
  const normalized = (countryCode || '').trim()
  if (!normalized) return '自定义源'
  return normalized.toUpperCase()
}

const geoStatusColor = (rule: FirewallGeoRule) => {
  if (rule.lastRefreshError) return 'warning'
  if (rule.lastRefreshAt > 0 && rule.prefixCount > 0) return 'success'
  return 'grey'
}

const geoStatusLabel = (rule: FirewallGeoRule) => {
  if (rule.lastRefreshError) return '缓存回退'
  if (rule.lastRefreshAt > 0 && rule.prefixCount > 0) return '缓存可用'
  return '待刷新'
}

const normalizeStringArray = (value: unknown): string[] => {
  if (!Array.isArray(value)) return []
  const result: string[] = []
  const seen = new Set<string>()
  for (const item of value) {
    const normalized = String(item ?? '').trim()
    if (!normalized || seen.has(normalized)) continue
    seen.add(normalized)
    result.push(normalized)
  }
  return result
}

const compactGeoSourceLabel = (value: string) => {
  const normalized = (value || '').trim()
  if (!normalized) return ''
  try {
    const parsed = new URL(normalized)
    const pathParts = parsed.pathname.split('/').filter(Boolean)
    const tail = pathParts.length > 0 ? pathParts[pathParts.length - 1] : ''
    return tail ? `${parsed.hostname} / ${tail}` : parsed.hostname
  } catch {
    return normalized
  }
}

const geoProviderSummary = (rule: FirewallGeoRule) => {
  if (rule.customSourceUrls.length > 0) {
    return `自定义规则集 ${rule.customSourceUrls.length} 个`
  }
  if (rule.sourceProviders.length === 0) {
    return '默认顺序：JSON 优先，其次 Clash'
  }
  const labels = rule.sourceProviders
    .slice(0, 2)
    .map(value => geoProviderTitleMap[value] || value)
  if (rule.sourceProviders.length > 2) {
    return `${labels.join(' / ')} 等 ${rule.sourceProviders.length} 个来源`
  }
  return labels.join(' / ')
}

const geoResolvedSourceSummary = (rule: FirewallGeoRule) => {
  if (rule.resolvedSources.length === 0) {
    return '尚未命中可用源'
  }
  if (rule.resolvedSources.length === 1) {
    return compactGeoSourceLabel(rule.resolvedSources[0])
  }
  return `${compactGeoSourceLabel(rule.resolvedSources[0])} 等 ${rule.resolvedSources.length} 个源`
}

const geoCacheSummary = (rule: FirewallGeoRule) => {
  if (rule.cachedFiles.length === 0) {
    return '未生成缓存文件'
  }
  return `已缓存 ${rule.cachedFiles.length} 个文件`
}

const syncGeoIntervalInput = (force = false) => {
  if (!force && geoSettingsDirty.value) {
    return
  }
  syncingGeoInterval.value = true
  geoIntervalInput.value = String(overview.value.geoUpdateIntervalMinutes || 360)
  geoSettingsDirty.value = false
  syncingGeoInterval.value = false
}

const syncSSHInputsFromOverview = (force = false) => {
  sshProxySwitch.value = overview.value.sshConfig.proxyEnabled === true
  if (!force && sshPortDirty.value) {
    return
  }
  sshPortInput.value = String(sshPortBaseline.value)
  sshPortInputTouched.value = false
  sshPortDirty.value = false
}

const applyOverview = (raw: any) => {
  overview.value = {
    ...emptyOverview(),
    ...(raw ?? {}),
    nftables: {
      ...emptyOverview().nftables,
      ...(raw?.nftables ?? {}),
      manualCommands: normalizeStringArray(raw?.nftables?.manualCommands),
    },
    defaultPorts: {
      ...emptyOverview().defaultPorts,
      ...(raw?.defaultPorts ?? {}),
      ssh: normalizeNumberArray(raw?.defaultPorts?.ssh),
      panel: normalizeNumberArray(raw?.defaultPorts?.panel),
      sub: normalizeNumberArray(raw?.defaultPorts?.sub),
      all: normalizeNumberArray(raw?.defaultPorts?.all),
      active: normalizeNumberArray(raw?.defaultPorts?.active),
      sshReserved: raw?.defaultPorts?.sshReserved !== false,
      panelReserved: raw?.defaultPorts?.panelReserved !== false,
      subReserved: raw?.defaultPorts?.subReserved !== false,
    },
    sshConfig: {
      ...emptyOverview().sshConfig,
      ...(raw?.sshConfig ?? {}),
      ports: normalizeNumberArray(raw?.sshConfig?.ports),
    },
    tcpActiveCount: Number(raw?.tcpActiveCount ?? 0),
    tcpSynRecvCount: Number(raw?.tcpSynRecvCount ?? 0),
    tcpEstablishedCount: Number(raw?.tcpEstablishedCount ?? 0),
    tcpAnomalyTotal: Number(raw?.tcpAnomalyTotal ?? 0),
    udpSocketCount: Number(raw?.udpSocketCount ?? 0),
    udpAnomalyTotal: Number(raw?.udpAnomalyTotal ?? 0),
    manualCount: Number(raw?.manualCount ?? 0),
    temporaryCount: Number(raw?.temporaryCount ?? 0),
    externalCount: Number(raw?.externalCount ?? 0),
    systemCount: Number(raw?.systemCount ?? 0),
    totalCount: Number(raw?.totalCount ?? 0),
    rules: Array.isArray(raw?.rules) ? raw.rules.map((item: any) => ({
      ...item,
      temporaryType: typeof item?.temporaryType === 'string' ? item.temporaryType : '',
      temporaryExpireAt: Number(item?.temporaryExpireAt ?? 0),
      canEdit: item?.canEdit === true,
      canDelete: item?.canDelete === true,
      listenerState: normalizeListenerState(item?.listenerState ?? emptyListenerState()),
    })) : [],
    geoRules: Array.isArray(raw?.geoRules) ? raw.geoRules.map((item: any) => ({
      ...item,
      sourceProviders: normalizeStringArray(item?.sourceProviders),
      customSourceUrls: normalizeStringArray(item?.customSourceUrls),
      resolvedSources: normalizeStringArray(item?.resolvedSources),
      cachedFiles: normalizeStringArray(item?.cachedFiles),
    })) : [],
  }
  switchEnabled.value = overview.value.enabled === true
  syncGeoIntervalInput()
  syncSSHInputsFromOverview()
  systemRuleBusyKey.value = ''
}

const fetchOverview = async (silent = false) => {
  if (!silent) {
    loading.value = true
  }
  try {
    const msg = await HttpUtils.get('api/firewall-overview')
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
    }
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

const refreshOverview = async () => {
  refreshing.value = true
  try {
    await fetchOverview(true)
  } finally {
    refreshing.value = false
  }
}

const installNftables = async () => {
  installingNftables.value = true
  try {
    const msg = await HttpUtils.post('api/firewall-nftables-install', {}, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      push.success({
        duration: 4000,
        message: 'nftables 已安装',
      })
      return
    }
    await fetchOverview(true)
  } finally {
    installingNftables.value = false
  }
}

const onToggleFirewall = async (nextValue: boolean | null) => {
  if (nextValue == null) {
    switchEnabled.value = overview.value.enabled
    return
  }
  if (nextValue === switchEnabled.value) return

  const confirmed = window.confirm(
    nextValue
      ? '开启后会按当前面板规则重建防火墙链，仅放行系统保留端口和面板已配置规则，并扫描系统已有放行规则供展示，是否继续？'
      : '关闭后会删除面板自己的防火墙链，但保留当前规则记录，是否继续？'
  )
  if (!confirmed) {
    switchEnabled.value = overview.value.enabled
    return
  }

  switchBusy.value = true
  try {
    const msg = await HttpUtils.post('api/firewall-switch', { enabled: nextValue }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      push.success({
        duration: 4000,
        message: nextValue ? '防火墙已开启并完成规则应用' : '防火墙已关闭并停止下发规则',
      })
    } else {
      switchEnabled.value = overview.value.enabled
    }
  } finally {
    switchBusy.value = false
  }
}

const onSSHPortFocus = () => {
  sshPortInputTouched.value = true
}

const onSSHPortBlur = () => {
  sshPortInputTouched.value = true
  sshPortDirty.value = String(sshPortInput.value || '').trim() !== String(sshPortBaseline.value)
}

const saveSSHPort = async () => {
  sshPortInputTouched.value = true
  if (!sshPortInputValid.value) {
    push.warning({
      duration: 4000,
      message: 'SSH 端口必须是 1-65535 的正整数',
    })
    return
  }
  const nextPort = sshPortInputValue.value
  if (nextPort === sshPortBaseline.value) {
    return
  }

  const confirmed = window.confirm(`确认将 SSH 端口改为 ${nextPort}，并重启 SSH 服务使其生效吗？`)
  if (!confirmed) {
    return
  }

  savingSSHPort.value = true
  try {
    const msg = await HttpUtils.post('api/firewall-ssh-port', { port: nextPort }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      syncSSHInputsFromOverview(true)
      push.success({
        duration: 4000,
        message: 'SSH 端口已更新并重启 SSH 服务',
      })
    }
  } finally {
    savingSSHPort.value = false
  }
}

const onToggleSSHProxy = async (nextValue: boolean | null) => {
  if (nextValue == null) {
    sshProxySwitch.value = overview.value.sshConfig.proxyEnabled
    return
  }
  if (nextValue === overview.value.sshConfig.proxyEnabled) {
    sshProxySwitch.value = overview.value.sshConfig.proxyEnabled
    return
  }

  sshProxySwitch.value = nextValue
  switchingSSHProxy.value = true
  try {
    const msg = await HttpUtils.post('api/firewall-ssh-proxy', { enabled: nextValue }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      push.success({
        duration: 4000,
        message: nextValue ? 'SSH 代理能力已开启并重启 SSH 服务' : 'SSH 代理能力已关闭并重启 SSH 服务',
      })
      return
    }
    sshProxySwitch.value = overview.value.sshConfig.proxyEnabled
  } finally {
    switchingSSHProxy.value = false
  }
}

const openRuleDialog = (rule?: FirewallRule) => {
  if (!overview.value.enabled) return
  const normalizedProtocol = rule?.protocol === 'any' ? 'tcp_udp' : (rule?.protocol || 'tcp_udp')
  editingRule.value = rule
    ? {
        id: rule.id,
        name: rule.name,
        description: rule.description,
        family: rule.family || 'dual',
        protocol: normalizedProtocol,
        portSpec: rule.portSpec || '',
        sourceSpec: rule.sourceSpec || '',
      }
    : createEmptyRuleForm()
  editingRule.value.family = normalizeRuleFamilyForSubmit(editingRule.value.protocol, editingRule.value.family)
  dialogVisible.value = true
}

const closeRuleDialog = () => {
  dialogVisible.value = false
}

const saveRule = async () => {
  if (!overview.value.enabled) return
  savingRule.value = true
  try {
    const normalizedProtocol = editingRule.value.protocol
    if (normalizedProtocol === 'any') {
      push.warning({
        duration: 4000,
        message: 'ANY 协议已禁用，请选择 TCP / UDP / TCP+UDP / ICMP',
      })
      return
    }
    if (editingRuleNeedsPort.value && !String(editingRule.value.portSpec || '').trim()) {
      push.warning({
        duration: 4000,
        message: '请填写端口或端口范围',
      })
      return
    }
    const normalizedFamily = normalizeRuleFamilyForSubmit(normalizedProtocol, editingRule.value.family)
    const payload = {
      id: editingRule.value.id || 0,
      name: editingRule.value.name,
      description: editingRule.value.description,
      family: normalizedFamily,
      protocol: normalizedProtocol,
      portSpec: editingRuleNeedsPort.value ? editingRule.value.portSpec : '',
      sourceSpec: editingRuleNeedsSource.value ? editingRule.value.sourceSpec : '',
    }
    const msg = await HttpUtils.post('api/firewall-rule', payload, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      dialogVisible.value = false
      push.success({
        duration: 4000,
        message: payload.id > 0 ? '防火墙规则已更新' : '防火墙规则已创建',
      })
    }
  } finally {
    savingRule.value = false
  }
}

const removeRule = async (rule: FirewallRule) => {
  const confirmed = window.confirm(
    rule.origin === 'system'
      ? `确定移除系统保留「${rule.name}」吗？移除后防火墙将不再自动放行对应端口。`
      : `确定删除规则「${rule.name}」吗？`
  )
  if (!confirmed) return

  const msg = await HttpUtils.post('api/firewall-rule-delete', { id: rule.id }, {
    headers: {
      'Content-Type': 'application/json',
    },
  })
  if (msg.success && msg.obj) {
    applyOverview(msg.obj)
    push.success({
      duration: 4000,
      message: '防火墙规则已删除',
    })
  }
}

const setSystemRuleReserved = async (systemKey: string, enabled: boolean) => {
  if (!['ssh', 'sub'].includes(systemKey)) return
  const actionLabel = enabled ? '恢复保留' : '移除保留'
  const confirmed = window.confirm(
    enabled
      ? `确定恢复 ${systemKey === 'ssh' ? 'SSH' : '订阅'} 的系统保留吗？恢复后会重新自动放行对应端口。`
      : `确定移除 ${systemKey === 'ssh' ? 'SSH' : '订阅'} 的系统保留吗？移除后防火墙将不再自动放行对应端口。`
  )
  if (!confirmed) return

  systemRuleBusyKey.value = systemKey
  try {
    const msg = await HttpUtils.post('api/firewall-system-rule', { systemKey, enabled }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      push.success({
        duration: 4000,
        message: `${systemKey === 'ssh' ? 'SSH' : '订阅'} 已${actionLabel}`,
      })
    }
  } finally {
    systemRuleBusyKey.value = ''
  }
}

const openGeoRuleDialog = (rule?: FirewallGeoRule) => {
  editingGeoRule.value = rule
    ? {
        id: rule.id,
        name: rule.name,
        description: rule.description,
        family: rule.family || 'dual',
        protocol: rule.protocol || 'tcp_udp',
        portSpec: rule.portSpec || '',
        action: rule.action || 'block',
        countryCode: geoCountryLabel(rule.countryCode) === '自定义源' ? '' : geoCountryLabel(rule.countryCode),
        sourceProviders: normalizeStringArray(rule.sourceProviders),
        customSourceUrls: normalizeStringArray(rule.customSourceUrls).join('\n'),
      }
    : createEmptyGeoRuleForm()
  geoDialogVisible.value = true
}

const closeGeoRuleDialog = () => {
  geoDialogVisible.value = false
}

const saveGeoRule = async () => {
  const normalizedCountryCode = String(editingGeoRule.value.countryCode || '').trim().toUpperCase()
  const normalizedPortSpec = String(editingGeoRule.value.portSpec || '').trim()
  if (!normalizedPortSpec) {
    push.warning({
      duration: 4000,
      message: 'GeoIP 规则必须填写端口或端口范围',
    })
    return
  }
  if (!editingGeoRule.value.customSourceUrls.trim() && !normalizedCountryCode) {
    push.warning({
      duration: 4000,
      message: '请填写国家代码，或提供至少一个完整规则集 URL',
    })
    return
  }

  savingGeoRule.value = true
  try {
    const payload = {
      id: editingGeoRule.value.id || 0,
      name: editingGeoRule.value.name,
      description: editingGeoRule.value.description,
      family: editingGeoRule.value.family,
      protocol: editingGeoRule.value.protocol,
      portSpec: normalizedPortSpec,
      action: editingGeoRule.value.action,
      countryCode: normalizedCountryCode,
      sourceProviders: normalizeStringArray(editingGeoRule.value.sourceProviders),
      customSourceUrls: editingGeoRule.value.customSourceUrls,
    }
    const msg = await HttpUtils.post('api/firewall-geo-rule', payload, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      geoDialogVisible.value = false
      push.success({
        duration: 4000,
        message: payload.id > 0 ? 'GeoIP 规则已更新' : 'GeoIP 规则已创建并缓存',
      })
    }
  } finally {
    savingGeoRule.value = false
  }
}

const removeGeoRule = async (rule: FirewallGeoRule) => {
  const confirmed = window.confirm(`确定删除 GeoIP 规则「${rule.name}」吗？`)
  if (!confirmed) return

  const msg = await HttpUtils.post('api/firewall-geo-rule-delete', { id: rule.id }, {
    headers: {
      'Content-Type': 'application/json',
    },
  })
  if (msg.success && msg.obj) {
    applyOverview(msg.obj)
    push.success({
      duration: 4000,
      message: 'GeoIP 规则已删除，缓存文件也已同步清理',
    })
  }
}

const refreshGeoRules = async () => {
  geoRefreshing.value = true
  try {
    const msg = await HttpUtils.post('api/firewall-geo-refresh', {}, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      push.success({
        duration: 4000,
        message: 'GeoIP 规则已完成热更新',
      })
    }
  } finally {
    geoRefreshing.value = false
  }
}

const saveGeoSettings = async () => {
  const intervalMinutes = Number.parseInt(String(geoIntervalInput.value || '').trim(), 10)
  if (!Number.isFinite(intervalMinutes) || intervalMinutes <= 0) {
    push.warning({
      duration: 4000,
      message: '更新周期必须是大于 0 的分钟数',
    })
    return
  }

  savingGeoSettings.value = true
  try {
    const msg = await HttpUtils.post('api/firewall-geo-settings', { intervalMinutes }, {
      headers: {
        'Content-Type': 'application/json',
      },
    })
    if (msg.success && msg.obj) {
      applyOverview(msg.obj)
      syncGeoIntervalInput(true)
      push.success({
        duration: 4000,
        message: 'GeoIP 更新周期已保存',
      })
    }
  } finally {
    savingGeoSettings.value = false
  }
}

const stopPolling = () => {
  if (pollTimer.value != null) {
    window.clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

const startPolling = () => {
  stopPolling()
  if (!props.active) return
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  pollTimer.value = window.setInterval(() => {
    void fetchOverview(true)
  }, 4000)
}

const handleVisibilityChange = () => {
  if (document.visibilityState === 'visible') {
    void fetchOverview(true)
    startPolling()
    return
  }
  stopPolling()
}

watch(geoIntervalInput, value => {
  if (syncingGeoInterval.value) return
  geoSettingsDirty.value = value.trim() !== String(overview.value.geoUpdateIntervalMinutes || 360)
})

watch(sshPortInput, value => {
  if (!sshPortInputTouched.value) return
  sshPortDirty.value = value.trim() !== String(sshPortBaseline.value)
})

watch(() => editingRule.value.protocol, protocol => {
  if (!isIcmpProtocol(protocol)) {
    return
  }
  editingRule.value.family = normalizeRuleFamilyForSubmit(protocol, editingRule.value.family)
  editingRule.value.portSpec = ''
  editingRule.value.sourceSpec = ''
})

watch(() => props.active, (active) => {
  if (active) {
    void fetchOverview(true)
    startPolling()
    return
  }
  stopPolling()
})

onMounted(() => {
  void fetchOverview()
  startPolling()
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
})

onBeforeUnmount(() => {
  stopPolling()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
})
</script>

<style scoped>
.firewall-page {
  min-height: 460px;
}

.firewall-metric--detail {
  min-height: 96px;
}

.firewall-metric__meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.7);
  text-align: right;
}

.firewall-tcp-metric {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: start;
  gap: 18px;
}

.firewall-tcp-metric__primary {
  min-width: 0;
}

.firewall-tcp-metric__meta {
  min-width: 112px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.firewall-tcp-metric__meta-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.78);
  white-space: nowrap;
}

.firewall-tcp-metric__meta-row strong {
  font-size: 14px;
  font-weight: 700;
  color: rgba(255, 255, 255, 0.96);
}

.firewall-hero {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(99, 179, 237, 0.18);
  background:
    radial-gradient(circle at top right, rgba(59, 130, 246, 0.2), transparent 36%),
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
  gap: 16px;
  flex-wrap: wrap;
}

.firewall-hero__controls {
  display: flex;
  align-items: stretch;
  justify-content: flex-end;
  gap: 12px;
  flex-wrap: wrap;
}

.firewall-hero__icon {
  width: 58px;
  height: 58px;
  border-radius: 18px;
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

.firewall-hero__switch {
  min-width: 126px;
  padding: 12px 14px;
  border-radius: 16px;
  background: rgba(15, 23, 42, 0.42);
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.firewall-hero__install {
  min-width: 200px;
  padding: 12px 14px;
  border-radius: 16px;
  background: rgba(15, 23, 42, 0.42);
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.firewall-hero__chips {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 18px;
}

.firewall-ssh-row {
  margin-top: 14px;
  display: grid;
  grid-template-columns: minmax(240px, 1.3fr) minmax(140px, auto) minmax(260px, 1.5fr);
  gap: 12px;
}

.firewall-ssh-card {
  padding: 14px;
  border-radius: 16px;
  background: rgba(15, 23, 42, 0.46);
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.firewall-ssh-card--action {
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  text-align: center;
}

.firewall-ssh-port-input {
  margin-bottom: 2px;
}

.firewall-metric {
  height: 100%;
  padding: 14px;
  border-radius: 16px;
  background: rgba(15, 23, 42, 0.46);
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.firewall-side {
  height: 100%;
}

.firewall-side__row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 10px 0;
  border-bottom: 1px dashed rgba(148, 163, 184, 0.16);
}

.firewall-side__row:last-child {
  border-bottom: none;
}

.firewall-rules {
  margin-top: 20px;
}

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

.firewall-listener-cell {
  min-width: 0;
}

.firewall-listener-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.firewall-listener-entry {
  padding: 8px 10px;
  border-radius: 12px;
  background: rgba(15, 23, 42, 0.32);
  border: 1px solid rgba(148, 163, 184, 0.1);
}

.firewall-listener-command {
  word-break: break-all;
}

.firewall-action--disabled {
  opacity: 0.45;
  pointer-events: none;
}

.firewall-protocol-chip {
  background: #f3e3a1 !important;
  color: #5f4a00 !important;
  border: 1px solid rgba(255, 245, 204, 0.5);
}

.firewall-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 150px;
  color: rgba(255, 255, 255, 0.72);
}

.firewall-geo__toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.firewall-geo__interval {
  min-width: 160px;
}

.firewall-geo__note {
  border: 1px solid rgba(59, 130, 246, 0.14);
}

.firewall-geo__cell {
  min-width: 0;
}

.firewall-geo__error {
  word-break: break-word;
}

@media (max-width: 960px) {
  .firewall-hero__controls {
    width: 100%;
  }

  .firewall-hero__install {
    width: 100%;
  }

  .firewall-hero__switch {
    width: 100%;
  }

  .firewall-ssh-row {
    grid-template-columns: 1fr;
  }

  .firewall-geo__interval {
    width: 100%;
  }

  .firewall-tcp-metric {
    grid-template-columns: 1fr;
  }

  .firewall-tcp-metric__meta {
    min-width: 0;
  }
}
</style>
