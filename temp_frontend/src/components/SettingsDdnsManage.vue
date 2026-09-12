<template>
  <section class="ddns-page">
    <!-- Hero Header -->
    <v-row class="mt-1">
      <v-col cols="12">
        <v-card class="ddns-hero" rounded="xl" :loading="loading">
          <v-card-text class="ddns-hero__content">
            <div class="ddns-hero__top">
              <div class="d-flex align-center ga-3">
                <div class="ddns-hero__icon">
                  <v-icon size="32">mdi-dns</v-icon>
                </div>
                <div>
                  <div class="text-overline text-primary">Dynamic DNS</div>
                  <div class="text-h5 font-weight-bold">DDNS 动态域名解析</div>
                  <div class="text-body-2 text-medium-emphasis mt-1">
                    定时探测本地或外网公网 IPv4 / IPv6，自动同步更新主流 DNS 解析记录。
                  </div>
                </div>
              </div>
              <div class="ddns-hero__toolbar">
                <v-btn
                  class="ddns-action-btn"
                  variant="outlined"
                  prepend-icon="mdi-sync"
                  :loading="refreshing"
                  :disabled="mutationBusy"
                  @click="syncAll"
                >
                  立即同步全部
                </v-btn>
                <v-btn
                  class="ddns-action-btn"
                  variant="outlined"
                  prepend-icon="mdi-key-chain"
                  @click="accountManageVisible = true"
                >
                  DNS 账号 ({{ overview.accounts.length }})
                </v-btn>
                <v-btn
                  class="ddns-action-btn"
                  variant="outlined"
                  color="primary"
                  prepend-icon="mdi-plus"
                  @click="openRuleDialog()"
                >
                  新建规则
                </v-btn>
              </div>
            </div>

            <div class="ddns-hero__chips mt-4 d-flex flex-wrap ga-2">
              <v-chip size="small" color="primary" variant="tonal">
                总规则数: {{ overview.rules.length }}
              </v-chip>
              <v-chip size="small" color="success" variant="tonal">
                启用规则: {{ enabledRuleCount }}
              </v-chip>
              <v-chip size="small" color="info" variant="tonal">
                DNS 账号: {{ overview.accounts.length }}
              </v-chip>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- Rules Container -->
    <v-card rounded="xl" variant="outlined" class="mt-4 ddns-card-wrapper">
      <v-card-title class="d-flex align-center justify-space-between flex-wrap py-3 px-4 ga-3">
        <div class="d-flex align-center ga-2">
          <v-icon color="primary" size="22">mdi-format-list-bulleted</v-icon>
          <span class="text-subtitle-1 font-weight-bold">解析任务列表</span>
          <v-chip size="x-small" color="primary" variant="tonal" class="ml-1 font-weight-medium">
            {{ filteredRules.length }} / {{ overview.rules.length }}
          </v-chip>
        </div>
        <div class="d-flex align-center ga-2 flex-grow-1 justify-end flex-wrap" style="max-width: 620px;">
          <v-text-field
            v-model="searchText"
            density="compact"
            variant="outlined"
            placeholder="搜索规则名 / 域名 / IP / 账号..."
            prepend-inner-icon="mdi-magnify"
            clearable
            hide-details
            style="min-width: 220px; max-width: 320px;"
          />
          <v-select
            v-model="statusFilter"
            :items="statusFilterOptions"
            item-title="title"
            item-value="value"
            density="compact"
            variant="outlined"
            hide-details
            style="min-width: 130px; max-width: 160px;"
          />
          <v-btn
            size="small"
            variant="tonal"
            icon="mdi-refresh"
            :loading="loading"
            title="刷新列表"
            @click="fetchOverview(false)"
          />
        </div>
      </v-card-title>
      <v-divider />

      <!-- Empty State: No Rules Configured -->
      <div v-if="overview.rules.length === 0" class="text-center py-12 text-medium-emphasis">
        <v-icon size="48" class="mb-2">mdi-cloud-search-outline</v-icon>
        <div class="text-body-1">暂无 DDNS 规则</div>
        <div class="text-caption mt-1">请点击右上角“新建规则”添加动态解析任务</div>
        <v-btn class="mt-4" variant="outlined" color="primary" prepend-icon="mdi-plus" @click="openRuleDialog()">
          立即创建
        </v-btn>
      </div>

      <!-- Empty State: Filtered Out -->
      <div v-else-if="filteredRules.length === 0" class="text-center py-10 text-medium-emphasis">
        <v-icon size="40" class="mb-2">mdi-filter-off-outline</v-icon>
        <div class="text-body-1">未找到匹配的解析规则</div>
        <div class="text-caption mt-1">请尝试修改搜索词或重置状态过滤</div>
        <v-btn class="mt-3" size="small" variant="text" color="primary" @click="searchText = ''; statusFilter = 'all'">
          清空筛选条件
        </v-btn>
      </div>

      <!-- Desktop Table (hidden on smAndDown) -->
      <v-table v-else class="d-none d-md-table ddns-table">
        <thead>
          <tr>
            <th class="text-center" style="width: 72px;">启用</th>
            <th style="width: 14%;">规则名称</th>
            <th style="width: 20%;">解析域名</th>
            <th style="width: 16%;">DNS 供应商 / 账号</th>
            <th style="width: 88px;" class="text-center">类型</th>
            <th style="width: 19%;">当前探测 / 同步 IP</th>
            <th style="width: 19%;">状态 / 上次同步</th>
            <th class="text-right" style="width: 130px;">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="rule in filteredRules" :key="rule.id" class="ddns-table-row">
            <!-- 启用 -->
            <td class="text-center">
              <v-switch
                :model-value="rule.enabled"
                color="primary"
                hide-details
                density="compact"
                class="d-inline-flex"
                @update:model-value="toggleRule(rule)"
              />
            </td>
            <!-- 规则名称 -->
            <td>
              <div class="font-weight-bold text-body-2">{{ rule.name }}</div>
              <div class="text-caption text-medium-emphasis mt-0-5 d-flex align-center ga-1">
                <v-icon size="12">mdi-timer-outline</v-icon>
                <span>{{ rule.intervalMinutes }} 分钟 / TTL {{ rule.ttl || 60 }}s</span>
              </div>
            </td>
            <!-- 解析域名 -->
            <td>
              <div class="d-flex flex-wrap ga-1">
                <v-chip
                  v-for="d in formatDomains(rule.domains)"
                  :key="d"
                  size="small"
                  variant="tonal"
                  color="primary"
                  class="ddns-domain-chip"
                  title="点击复制域名"
                  @click="copyToClipboard(d, '域名')"
                >
                  <v-icon start size="14">mdi-web</v-icon>
                  <span class="text-body-2 font-weight-medium">{{ d }}</span>
                  <v-icon end size="12" class="opacity-60">mdi-content-copy</v-icon>
                </v-chip>
              </div>
            </td>
            <!-- DNS 供应商 / 账号 -->
            <td>
              <div class="d-flex align-center ga-2">
                <v-avatar size="26" color="primary" variant="tonal" class="rounded-circle">
                  <v-icon size="14">mdi-server-network</v-icon>
                </v-avatar>
                <div>
                  <div class="text-body-2 font-weight-medium">{{ getAccountName(rule.accountId) }}</div>
                  <div class="text-caption text-medium-emphasis">{{ getProviderName(rule.accountId) }}</div>
                </div>
              </div>
            </td>
            <!-- 类型 -->
            <td class="text-center">
              <v-chip
                size="x-small"
                :color="rule.ipType === 'dual' ? 'purple' : rule.ipType === 'ipv6' ? 'teal' : 'blue'"
                variant="flat"
                class="font-weight-bold"
              >
                {{ rule.ipType === 'dual' ? '双栈' : rule.ipType.toUpperCase() }}
              </v-chip>
            </td>
            <!-- 当前探测 / 同步 IP -->
            <td>
              <div v-if="rule.lastIpv4" class="mb-1">
                <div
                  v-for="ip in getIpList(rule.lastIpv4)"
                  :key="'pc-v4-' + ip"
                  class="text-caption d-flex align-center ga-1 mb-0-5"
                >
                  <v-chip size="x-small" color="blue" variant="outlined" density="compact">v4</v-chip>
                  <span class="font-weight-medium text-body-2">{{ ip }}</span>
                  <v-btn
                    size="x-small"
                    variant="text"
                    icon="mdi-content-copy"
                    density="compact"
                    title="复制 IPv4"
                    @click="copyToClipboard(ip, 'IPv4')"
                  />
                </div>
              </div>
              <div v-if="rule.lastIpv6">
                <div
                  v-for="ip in getIpList(rule.lastIpv6)"
                  :key="'pc-v6-' + ip"
                  class="text-caption d-flex align-center ga-1 mb-0-5"
                >
                  <v-chip size="x-small" color="teal" variant="outlined" density="compact">v6</v-chip>
                  <span class="font-weight-medium text-body-2 text-truncate" style="max-width: 160px;" :title="ip">{{ ip }}</span>
                  <v-btn
                    size="x-small"
                    variant="text"
                    icon="mdi-content-copy"
                    density="compact"
                    title="复制 IPv6"
                    @click="copyToClipboard(ip, 'IPv6')"
                  />
                </div>
              </div>
              <div v-if="!rule.lastIpv4 && !rule.lastIpv6" class="text-caption text-medium-emphasis font-italic d-flex align-center ga-1">
                <v-icon size="14">mdi-clock-outline</v-icon>
                <span>等待首次探测同步</span>
              </div>
            </td>
            <!-- 状态 / 上次同步 -->
            <td>
              <div class="d-flex align-center ga-1">
                <v-icon
                  size="16"
                  :color="rule.lastStatus === 'success' ? 'success' : rule.lastStatus === 'error' ? 'error' : 'warning'"
                >
                  {{ rule.lastStatus === 'success' ? 'mdi-check-circle' : rule.lastStatus === 'error' ? 'mdi-alert-circle' : 'mdi-clock-outline' }}
                </v-icon>
                <span :class="rule.lastStatus === 'success' ? 'text-success font-weight-medium' : rule.lastStatus === 'error' ? 'text-error font-weight-medium' : 'text-warning font-weight-medium'" class="text-caption">
                  {{ rule.lastStatus === 'success' ? '同步成功' : rule.lastStatus === 'error' ? '同步失败' : '待同步' }}
                </span>
              </div>
              <div class="text-caption text-disabled mt-0-5">
                {{ rule.lastSyncTime ? formatTime(rule.lastSyncTime) : '尚未执行' }}
              </div>
              <div v-if="rule.lastStatus === 'error' && rule.lastError" class="text-caption text-error text-truncate mt-0-5" style="max-width: 220px;" :title="rule.lastError">
                {{ rule.lastError }}
              </div>
            </td>
            <!-- 操作 -->
            <td class="text-right">
              <div class="d-flex align-center justify-end ga-1">
                <v-btn
                  size="small"
                  variant="text"
                  icon="mdi-sync"
                  title="立即同步"
                  :loading="syncingRuleId === rule.id"
                  :disabled="deletingRuleId === rule.id || mutationBusy"
                  @click="syncRule(rule)"
                />
                <v-btn
                  size="small"
                  variant="text"
                  icon="mdi-pencil"
                  title="编辑"
                  :disabled="deletingRuleId === rule.id || mutationBusy"
                  @click="openRuleDialog(rule)"
                />
                <v-btn
                  size="small"
                  variant="text"
                  color="error"
                  icon="mdi-delete"
                  title="删除规则（同步删除服务商云端记录）"
                  :loading="deletingRuleId === rule.id"
                  :disabled="mutationBusy && deletingRuleId !== rule.id"
                  @click="removeRule(rule)"
                />
              </div>
            </td>
          </tr>
        </tbody>
      </v-table>

      <!-- Table Footer / Summary Bar -->
      <v-divider v-if="filteredRules.length > 0" class="d-none d-md-block" />
      <div v-if="overview.rules.length > 0" class="d-none d-md-flex align-center justify-space-between px-4 py-3 bg-surface-variant-subtle text-caption text-medium-emphasis">
        <div class="d-flex align-center ga-2">
          <v-icon size="14" color="primary">mdi-shield-check-outline</v-icon>
          <span>共 {{ filteredRules.length }} 个解析任务<span v-if="filteredRules.length !== overview.rules.length">（已过滤，总计 {{ overview.rules.length }} 个）</span>，其中 {{ enabledRuleCount }} 个处于启用状态</span>
        </div>
        <div class="d-flex align-center ga-2">
          <v-icon size="14">mdi-information-outline</v-icon>
          <span>删除任务时将自动同步向 DNS 服务商请求删除云端对应的解析记录</span>
        </div>
      </div>

      <!-- Mobile Cards (shown on smAndDown) -->
      <div v-if="filteredRules.length > 0" class="d-md-none pa-3">
        <v-card
          v-for="rule in filteredRules"
          :key="rule.id"
          variant="outlined"
          rounded="lg"
          class="mb-3 pa-3 ddns-mobile-card"
        >
          <div class="d-flex align-center justify-space-between mb-2">
            <div class="d-flex align-center ga-2">
              <span class="font-weight-bold text-subtitle-1">{{ rule.name }}</span>
              <v-chip
                size="x-small"
                :color="rule.ipType === 'dual' ? 'purple' : rule.ipType === 'ipv6' ? 'teal' : 'blue'"
                variant="flat"
              >
                {{ rule.ipType === 'dual' ? '双栈' : rule.ipType.toUpperCase() }}
              </v-chip>
            </div>
            <v-switch
              :model-value="rule.enabled"
              color="primary"
              hide-details
              density="compact"
              @update:model-value="toggleRule(rule)"
            />
          </div>

          <div class="text-caption mb-1 d-flex align-center ga-1 text-medium-emphasis">
            <v-icon size="14">mdi-server-network</v-icon>
            <span>供应商:</span>
            <strong class="text-high-emphasis">{{ getAccountName(rule.accountId) }}</strong>
            <span>({{ getProviderName(rule.accountId) }})</span>
          </div>

          <div class="d-flex flex-wrap ga-1 my-2">
            <v-chip
              v-for="d in formatDomains(rule.domains)"
              :key="d"
              size="small"
              variant="tonal"
              color="primary"
              @click="copyToClipboard(d, '域名')"
            >
              <v-icon start size="12">mdi-web</v-icon>
              <span>{{ d }}</span>
              <v-icon end size="10">mdi-content-copy</v-icon>
            </v-chip>
          </div>

          <div v-if="rule.lastIpv4" class="my-1">
            <div
              v-for="ip in getIpList(rule.lastIpv4)"
              :key="'mob-v4-' + ip"
              class="text-caption my-0-5 d-flex align-center ga-1"
            >
              <v-chip size="x-small" color="blue" variant="outlined">IPv4</v-chip>
              <span>{{ ip }}</span>
              <v-btn size="x-small" variant="text" icon="mdi-content-copy" density="compact" @click="copyToClipboard(ip, 'IPv4')" />
            </div>
          </div>
          <div v-if="rule.lastIpv6" class="my-1">
            <div
              v-for="ip in getIpList(rule.lastIpv6)"
              :key="'mob-v6-' + ip"
              class="text-caption my-0-5 d-flex align-center ga-1"
            >
              <v-chip size="x-small" color="teal" variant="outlined">IPv6</v-chip>
              <span class="text-truncate" style="max-width: 200px;" :title="ip">{{ ip }}</span>
              <v-btn size="x-small" variant="text" icon="mdi-content-copy" density="compact" @click="copyToClipboard(ip, 'IPv6')" />
            </div>
          </div>
          <div v-if="!rule.lastIpv4 && !rule.lastIpv6" class="text-caption text-medium-emphasis font-italic my-1">
            等待首次同步探测
          </div>

          <div class="d-flex align-center justify-space-between mt-3 pt-2 border-t">
            <div class="d-flex align-center ga-1">
              <v-icon
                size="16"
                :color="rule.lastStatus === 'success' ? 'success' : rule.lastStatus === 'error' ? 'error' : 'warning'"
              >
                {{ rule.lastStatus === 'success' ? 'mdi-check-circle' : rule.lastStatus === 'error' ? 'mdi-alert-circle' : 'mdi-clock-outline' }}
              </v-icon>
              <span class="text-caption font-weight-medium" :class="rule.lastStatus === 'success' ? 'text-success' : rule.lastStatus === 'error' ? 'text-error' : 'text-warning'">
                {{ rule.lastStatus === 'success' ? '同步成功' : rule.lastStatus === 'error' ? '失败' : '待处理' }}
              </span>
              <span class="text-caption text-disabled ml-1">
                {{ rule.lastSyncTime ? formatTime(rule.lastSyncTime) : '未执行' }}
              </span>
            </div>
            <div class="d-flex ga-1">
              <v-btn
                size="small"
                variant="outlined"
                prepend-icon="mdi-sync"
                :loading="syncingRuleId === rule.id"
                :disabled="deletingRuleId === rule.id || mutationBusy"
                @click="syncRule(rule)"
              >同步</v-btn>
              <v-btn
                size="small"
                variant="outlined"
                prepend-icon="mdi-pencil"
                :disabled="deletingRuleId === rule.id || mutationBusy"
                @click="openRuleDialog(rule)"
              >编辑</v-btn>
              <v-btn
                size="small"
                variant="outlined"
                color="error"
                icon="mdi-delete"
                :loading="deletingRuleId === rule.id"
                :disabled="mutationBusy && deletingRuleId !== rule.id"
                @click="removeRule(rule)"
              />
            </div>
          </div>
          <div v-if="rule.lastStatus === 'error' && rule.lastError" class="text-caption text-error mt-2">
            {{ rule.lastError }}
          </div>
        </v-card>
      </div>
    </v-card>

    <!-- Rule Edit Dialog -->
    <v-dialog v-model="ruleDialogVisible" max-width="800" persistent>
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">
          {{ ruleForm.id > 0 ? '编辑 DDNS 规则' : '新建 DDNS 规则' }}
        </v-card-title>
        <v-divider />
        <v-card-text class="overflow-y-auto" style="max-height: 75vh;">
          <v-row>
            <v-col cols="12" md="6">
              <v-text-field v-model="ruleForm.name" label="规则名称 *" placeholder="如: 家里软路由-IPv6" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="ruleForm.accountId"
                :items="accountSelectItems"
                item-title="title"
                item-value="value"
                label="DNS 账号 *"
                hide-details
              >
                <template #append>
                  <v-btn size="small" variant="text" icon="mdi-plus" title="新增 DNS 账号" @click="openAccountDialog()" />
                </template>
              </v-select>
            </v-col>
          </v-row>

          <v-textarea
            v-model="ruleForm.domains"
            label="解析域名 (支持多个，逗号或换行分隔) *"
            placeholder="sub.example.com&#10;vpn.example.com"
            rows="2"
            auto-grow
            variant="outlined"
            class="mt-3"
            hide-details
          />

          <!-- IP Type Selection -->
          <div class="mt-4 mb-1 text-subtitle-2">解析记录类型</div>
          <v-btn-toggle v-model="ruleForm.ipType" mandatory color="primary" variant="outlined" density="comfortable">
            <v-btn value="ipv4">仅 IPv4 (A)</v-btn>
            <v-btn value="ipv6">仅 IPv6 (AAAA)</v-btn>
            <v-btn value="dual">双栈 (IPv4 + IPv6)</v-btn>
          </v-btn-toggle>

          <!-- IPv4 Source Settings -->
          <v-card
            v-if="ruleForm.ipType === 'ipv4' || ruleForm.ipType === 'dual'"
            variant="outlined"
            rounded="lg"
            class="ddns-source-card mt-4 pa-3"
          >
            <div class="text-subtitle-2 mb-2 font-weight-medium">IPv4 获取方式</div>
            <v-row>
              <v-col cols="12" :md="ipv4Sources.includes('interface') && ipv4Sources.includes('api') ? 12 : 5">
                <v-select
                  v-model="ipv4Sources"
                  :items="ipv4SourceOptions"
                  item-title="title"
                  item-value="value"
                  label="IPv4 来源 (支持多选)"
                  multiple
                  chips
                  closable-chips
                  hide-details
                />
              </v-col>
              <v-col
                cols="12"
                :md="ipv4Sources.includes('interface') && ipv4Sources.includes('api') ? 6 : (ipv4Sources.includes('api') ? 6 : 7)"
                v-if="ipv4Sources.includes('interface')"
              >
                <v-select
                  v-model="ruleForm.ipv4Interface"
                  :items="interfaceItems"
                  label="选择本地网络接口"
                  hide-details
                />
              </v-col>
              <v-col
                cols="12"
                :md="ipv4Sources.includes('interface') && ipv4Sources.includes('api') ? 6 : (ipv4Sources.includes('interface') ? 6 : 7)"
                v-if="ipv4Sources.includes('api')"
              >
                <v-combobox
                  v-model="ipv4Urls"
                  :items="ipv4PresetUrls"
                  label="自定义 IPv4 探测 URL (可多选或输入，留空使用内置源)"
                  placeholder="https://api-ipv4.ip.sb/ip"
                  multiple
                  chips
                  closable-chips
                  hide-details
                />
              </v-col>
              <v-col cols="12" v-if="ipv4Sources.includes('interface') && ipv4Sources.includes('api')" class="pt-1 pb-0">
                <div class="text-caption text-medium-emphasis d-flex align-center">
                  <v-icon size="14" class="mr-1">mdi-information-outline</v-icon>
                  已启用双重探测模式：优先直取本地网卡 IPv4，若网卡无有效公网 IP 则自动回退至公网 API 探测。
                </div>
              </v-col>
            </v-row>
          </v-card>

          <!-- IPv6 Source Settings -->
          <v-card
            v-if="ruleForm.ipType === 'ipv6' || ruleForm.ipType === 'dual'"
            variant="outlined"
            rounded="lg"
            class="ddns-source-card mt-3 pa-3"
          >
            <div class="text-subtitle-2 mb-2 font-weight-medium">IPv6 获取方式</div>
            <v-row>
              <v-col cols="12" :md="ipv6Sources.includes('interface') && ipv6Sources.includes('api') ? 12 : 5">
                <v-select
                  v-model="ipv6Sources"
                  :items="ipv6SourceOptions"
                  item-title="title"
                  item-value="value"
                  label="IPv6 来源 (支持多选)"
                  multiple
                  chips
                  closable-chips
                  hide-details
                />
              </v-col>
              <v-col
                cols="12"
                :md="ipv6Sources.includes('interface') && ipv6Sources.includes('api') ? 6 : (ipv6Sources.includes('api') ? 6 : 7)"
                v-if="ipv6Sources.includes('interface')"
              >
                <v-select
                  v-model="ruleForm.ipv6Interface"
                  :items="interfaceItems"
                  label="选择本地网络接口"
                  hide-details
                />
              </v-col>
              <v-col
                cols="12"
                :md="ipv6Sources.includes('interface') && ipv6Sources.includes('api') ? 6 : (ipv6Sources.includes('interface') ? 6 : 7)"
                v-if="ipv6Sources.includes('api')"
              >
                <v-combobox
                  v-model="ipv6Urls"
                  :items="ipv6PresetUrls"
                  label="自定义 IPv6 探测 URL (可多选或输入，留空使用内置源)"
                  placeholder="https://api-ipv6.ip.sb/ip"
                  multiple
                  chips
                  closable-chips
                  hide-details
                />
              </v-col>
              <v-col cols="12" v-if="ipv6Sources.includes('interface') && ipv6Sources.includes('api')" class="pt-1 pb-0">
                <div class="text-caption text-medium-emphasis d-flex align-center">
                  <v-icon size="14" class="mr-1">mdi-information-outline</v-icon>
                  已启用双重探测模式：优先直取本地网卡 IPv6，若网卡无有效公网 IP 则自动回退至公网 API 探测。
                </div>
              </v-col>
            </v-row>
          </v-card>

          <!-- Schedule & Options -->
          <v-row class="mt-3">
            <!-- 本地网卡检查频率 -->
            <v-col
              v-if="hasInterfaceMode"
              cols="12"
              :sm="hasInterfaceMode && hasApiMode ? 6 : 6"
              :md="hasInterfaceMode && hasApiMode ? 3 : 4"
            >
              <v-select
                v-model="ruleForm.localIntervalSeconds"
                :items="localIntervalOptions"
                item-title="title"
                item-value="value"
                label="本地网卡检查频率"
                hide-details
              />
            </v-col>

            <!-- 公网 API 检查频率 -->
            <v-col
              v-if="hasApiMode || (!hasInterfaceMode && !hasApiMode)"
              cols="12"
              :sm="hasInterfaceMode && hasApiMode ? 6 : 6"
              :md="hasInterfaceMode && hasApiMode ? 3 : 4"
            >
              <v-select
                v-model="ruleForm.intervalMinutes"
                :items="publicIntervalOptions"
                item-title="title"
                item-value="value"
                label="公网 API 检查频率"
                hide-details
              />
            </v-col>

            <!-- DNS TTL (秒) -->
            <v-col
              cols="12"
              :sm="hasInterfaceMode && hasApiMode ? 6 : 6"
              :md="hasInterfaceMode && hasApiMode ? 3 : 4"
            >
              <v-text-field
                v-model.number="ruleForm.ttl"
                type="number"
                label="DNS TTL (秒)"
                placeholder="60"
                hide-details
              />
            </v-col>

            <!-- Cloudflare CDN 代理 (Proxy) -->
            <v-col
              cols="12"
              :sm="hasInterfaceMode && hasApiMode ? 6 : 6"
              :md="hasInterfaceMode && hasApiMode ? 3 : 4"
              class="d-flex align-center"
            >
              <v-switch
                v-model="ruleForm.cloudflareProxy"
                color="warning"
                label="Cloudflare CDN 代理 (Proxy)"
                hide-details
              />
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" :disabled="mutationBusy" @click="ruleDialogVisible = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :loading="mutationBusy" @click="saveRule">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Accounts Management List Dialog -->
    <v-dialog v-model="accountManageVisible" max-width="750">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <span class="text-subtitle-1 font-weight-medium">DNS 账号管理</span>
          <v-btn color="primary" size="small" prepend-icon="mdi-plus" @click="openAccountDialog()">新增账号</v-btn>
        </v-card-title>
        <v-divider />
        <v-card-text class="overflow-y-auto" style="max-height: 75vh;">
          <div v-if="overview.accounts.length === 0" class="text-center py-6 text-medium-emphasis">
            暂无已配置的 DNS 账号
          </div>
          <v-list v-else lines="two">
            <v-list-item
              v-for="acc in overview.accounts"
              :key="acc.id"
              class="border rounded-lg mb-2"
            >
              <template #title>
                <div class="font-weight-medium">{{ acc.name }}</div>
              </template>
              <template #subtitle>
                <div class="text-caption text-medium-emphasis">
                  供应商: {{ getProviderName(acc.id) }} | 备注: {{ acc.remark || '无' }}
                </div>
              </template>
              <template #append>
                <v-btn size="small" variant="text" icon="mdi-pencil" @click="openAccountDialog(acc)" />
                <v-btn size="small" variant="text" color="error" icon="mdi-delete" @click="removeAccount(acc)" />
              </template>
            </v-list-item>
          </v-list>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="accountManageVisible = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Account Edit Dialog -->
    <v-dialog v-model="accountDialogVisible" max-width="650" persistent>
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">
          {{ accountForm.id > 0 ? '编辑 DNS 账号' : '新增 DNS 账号' }}
        </v-card-title>
        <v-divider />
        <v-card-text class="overflow-y-auto" style="max-height: 75vh;">
          <v-row>
            <v-col cols="12" md="6">
              <v-text-field v-model="accountForm.name" label="账号别名 *" placeholder="如: 我的 Cloudflare" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="accountForm.providerCode"
                :items="providerSelectItems"
                item-title="title"
                item-value="value"
                label="DNS 供应商 *"
                hide-details
              />
            </v-col>
          </v-row>

          <v-alert
            v-if="selectedProvider"
            type="info"
            variant="tonal"
            density="comfortable"
            class="my-3 text-caption"
          >
            {{ selectedProvider.helper }}
          </v-alert>

          <!-- Dynamic Provider Fields -->
          <div v-if="selectedProvider" class="mt-2">
            <v-row>
              <v-col
                v-for="field in selectedProvider.fields"
                :key="field.key"
                cols="12"
                :md="field.key === 'Webhook_Headers' || field.key === 'Webhook_Body' ? 12 : 6"
              >
                <v-textarea
                  v-if="field.key === 'Webhook_Headers' || field.key === 'Webhook_Body'"
                  v-model="accountEnvMap[field.key]"
                  :label="`${field.label}${field.required ? ' *' : ''}`"
                  :placeholder="field.placeholder || ''"
                  rows="2"
                  auto-grow
                  variant="outlined"
                  hide-details
                />
                <v-text-field
                  v-else
                  v-model="accountEnvMap[field.key]"
                  :type="field.secret ? 'password' : 'text'"
                  :label="`${field.label}${field.required ? ' *' : ''}`"
                  :placeholder="field.placeholder || ''"
                  hide-details
                />
              </v-col>
            </v-row>
          </div>

          <v-text-field
            v-model="accountForm.remark"
            label="备注"
            class="mt-3"
            placeholder="可选备注说明"
            hide-details
          />
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-btn
            variant="outlined"
            color="info"
            prepend-icon="mdi-lightning-bolt"
            :loading="testAuthBusy"
            @click="testAccountAuth"
          >
            测试连接
          </v-btn>
          <v-spacer />
          <v-btn variant="text" :disabled="mutationBusy" @click="accountDialogVisible = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :loading="mutationBusy" @click="saveAccount">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { useDdnsManage } from './SettingsDdnsManage.shared'

const props = defineProps<{
  active: boolean
}>()

const activeRef = toRef(props, 'active')
const {
  loading,
  refreshing,
  mutationBusy,
  testAuthBusy,
  deletingRuleId,
  syncingRuleId,
  searchText,
  statusFilter,
  filteredRules,
  getIpList,
  copyToClipboard,
  overview,
  ruleDialogVisible,
  ruleForm,
  ipv4Sources,
  ipv4Urls,
  ipv6Sources,
  ipv6Urls,
  accountDialogVisible,
  accountManageVisible,
  accountForm,
  accountEnvMap,
  selectedProvider,
  fetchOverview,
  openRuleDialog,
  saveRule,
  toggleRule,
  removeRule,
  syncRule,
  syncAll,
  openAccountDialog,
  testAccountAuth,
  saveAccount,
  removeAccount,
  getAccountName,
  getProviderName,
} = useDdnsManage(activeRef)

const statusFilterOptions = [
  { title: '全部状态', value: 'all' },
  { title: '仅看启用', value: 'enabled' },
  { title: '仅看停用', value: 'disabled' },
  { title: '同步成功', value: 'success' },
  { title: '待同步', value: 'pending' },
  { title: '同步异常', value: 'error' },
]

const ipv4SourceOptions = [
  { title: '通过公网 API 探测', value: 'api' },
  { title: '本地网卡接口直取', value: 'interface' },
]

const ipv6SourceOptions = [
  { title: '本地网卡接口直取 (推荐)', value: 'interface' },
  { title: '通过公网 API 探测', value: 'api' },
]

const ipv4PresetUrls = [
  'https://api-ipv4.ip.sb/ip',
  'https://api4.ipify.org',
  'https://v4.ident.me',
  'https://ipv4.icanhazip.com',
]

const ipv6PresetUrls = [
  'https://api-ipv6.ip.sb/ip',
  'https://api64.ipify.org',
  'https://v6.ident.me',
  'https://speed.neu6.edu.cn/getIP.php',
  'https://ipv6.icanhazip.com',
]

const needV4 = computed(() => ruleForm.value.ipType === 'ipv4' || ruleForm.value.ipType === 'dual')
const needV6 = computed(() => ruleForm.value.ipType === 'ipv6' || ruleForm.value.ipType === 'dual')

const hasInterfaceMode = computed(() => {
  return (needV4.value && ipv4Sources.value.includes('interface')) ||
         (needV6.value && ipv6Sources.value.includes('interface'))
})

const hasApiMode = computed(() => {
  return (needV4.value && ipv4Sources.value.includes('api')) ||
         (needV6.value && ipv6Sources.value.includes('api'))
})

const localIntervalOptions = [
  { title: '每 5 秒', value: 5 },
  { title: '每 10 秒 (推荐)', value: 10 },
  { title: '每 20 秒', value: 20 },
  { title: '每 30 秒', value: 30 },
  { title: '每 60 秒 (1 分钟)', value: 60 },
  { title: '每 5 分钟', value: 300 },
]

const publicIntervalOptions = [
  { title: '每 1 分钟 (推荐)', value: 1 },
  { title: '每 2 分钟', value: 2 },
  { title: '每 5 分钟', value: 5 },
  { title: '每 10 分钟', value: 10 },
  { title: '每 30 分钟', value: 30 },
  { title: '每 60 分钟', value: 60 },
]

const enabledRuleCount = computed(() => {
  return overview.value.rules.filter((r) => r.enabled).length
})

const accountSelectItems = computed(() => {
  return (overview.value.accounts || []).map((a) => ({
    title: `${a.name} (${getProviderName(a.id)})`,
    value: a.id,
  }))
})

const providerSelectItems = computed(() => {
  return (overview.value.providers || []).map((p) => ({
    title: p.name,
    value: p.code,
  }))
})

const interfaceItems = computed(() => {
  return (overview.value.interfaces || []).map((i) => {
    const ipList = Array.isArray(i.ips) ? i.ips.filter(Boolean) : []
    return {
      title: `${i.name} (${ipList.join(', ') || '无IP'})`,
      value: i.name,
    }
  })
})

function formatDomains(domains: string): string[] {
  if (!domains) return []
  return domains
    .replace(/\r/g, '')
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso)
    return d.toLocaleString()
  } catch {
    return iso
  }
}
</script>

<style scoped>
.ddns-page {
  padding-bottom: 24px;
}

.ddns-hero {
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.ddns-hero__top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
}

.ddns-hero__icon {
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: rgba(var(--v-theme-primary), 0.12);
  color: rgb(var(--v-theme-primary));
  display: flex;
  align-items: center;
  justify-content: center;
}

.ddns-hero__toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.ddns-action-btn {
  text-transform: none;
  font-weight: 500;
}

.ddns-source-card {
  border-color: rgba(255, 255, 255, 0.1) !important;
  background-color: rgba(255, 255, 255, 0.02) !important;
}

@media (max-width: 959px) {
  .ddns-hero__toolbar {
    width: 100%;
  }
  .ddns-action-btn {
    flex: 1 1 100%;
  }
}

.ddns-card-wrapper {
  background: rgba(255, 255, 255, 0.015);
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.ddns-table {
  width: 100%;
}

.ddns-table table {
  width: 100% !important;
  table-layout: fixed;
}

.ddns-table-row:hover {
  background-color: rgba(255, 255, 255, 0.035) !important;
  transition: background-color 0.2s ease;
}

.ddns-domain-chip {
  cursor: pointer;
  transition: all 0.2s ease;
}

.ddns-domain-chip:hover {
  opacity: 0.9;
  transform: translateY(-1px);
}

.bg-surface-variant-subtle {
  background: rgba(255, 255, 255, 0.02);
  border-top: 1px solid rgba(255, 255, 255, 0.05);
}

.ddns-mobile-card {
  border-color: rgba(255, 255, 255, 0.08) !important;
  background-color: rgba(255, 255, 255, 0.02) !important;
}
</style>
