<template>
  <section class="dns-page">
    <!-- Hero Header -->
    <v-row class="mt-1">
      <v-col cols="12">
        <v-card class="dns-hero" rounded="xl" :loading="loading">
          <v-card-text class="dns-hero__content">
            <div class="dns-hero__top">
              <div class="d-flex align-center ga-3">
                <div class="dns-hero__icon">
                  <v-icon size="32">mdi-cloud-tags</v-icon>
                </div>
                <div>
                  <div class="text-overline text-primary">DNS MANAGEMENT</div>
                  <div class="text-h5 font-weight-bold">{{ $t('dnsManage.title', 'DNS 解析管理') }}</div>
                  <div class="text-body-2 text-medium-emphasis mt-1">
                    {{ $t('dnsManage.subtitle', '直接通过主流云服务商 API 实时管理域名的云端解析记录（A、AAAA、CNAME、TXT、MX 等）。') }}
                  </div>
                </div>
              </div>
              <div class="dns-hero__toolbar">
                <v-btn
                  class="dns-action-btn"
                  variant="outlined"
                  prepend-icon="mdi-key-chain"
                  @click="accountManageVisible = true"
                >
                  {{ $t('ddns.dnsAccounts', 'DNS 账号') }} ({{ overview.accounts.length }})
                </v-btn>
                <v-btn
                  class="dns-action-btn"
                  variant="outlined"
                  color="primary"
                  prepend-icon="mdi-plus"
                  :disabled="!selectedAccountId || !selectedDomain"
                  @click="openRecordDialog()"
                >
                  {{ $t('dnsManage.newRecord', '新建解析记录') }}
                </v-btn>
              </div>
            </div>

            <div class="dns-hero__chips mt-4 d-flex flex-wrap ga-2 align-center">
              <v-chip size="small" color="primary" variant="tonal">
                {{ $t('dnsManage.currentAccount', '当前账号') }}: {{ currentAccount?.name || $t('none', '未选择') }}
                <template v-if="currentProvider"> ({{ currentProvider.name }})</template>
              </v-chip>
              <v-chip size="small" color="info" variant="tonal">
                {{ $t('dnsManage.currentDomain', '当前域名') }}: {{ selectedDomain || $t('none', '未填写') }}
              </v-chip>
              <v-chip size="small" color="success" variant="tonal">
                {{ $t('dnsManage.recordCount', '云端记录数') }}: {{ records.length }}
              </v-chip>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <!-- Scope Selector Bar -->
    <v-card rounded="xl" variant="outlined" class="mt-4 pa-4 dns-scope-card">
      <v-row dense align="center">
        <v-col cols="12" sm="5" md="4">
          <v-select
            v-model="selectedAccountId"
            :items="generalAccounts"
            item-title="name"
            item-value="id"
            :label="$t('dnsManage.selectAccount', '选择 DNS 账号')"
            density="compact"
            variant="outlined"
            hide-details
            prepend-inner-icon="mdi-account-key"
            :no-data-text="$t('dnsManage.noGeneralAccounts', '暂无可用的通用 DNS 账号，请在右上角添加 Cloudflare/阿里云/腾讯云等账号')"
            @update:model-value="onAccountChanged"
          >
            <template #item="{ props, item }">
              <v-list-item v-bind="props">
                <template #append>
                  <v-chip size="x-small" color="primary" variant="tonal">
                    {{ providersMap[item.raw.providerCode]?.name || item.raw.providerCode }}
                  </v-chip>
                </template>
              </v-list-item>
            </template>
          </v-select>
        </v-col>

        <v-col cols="12" sm="5" md="5">
          <v-combobox
            v-model="selectedDomain"
            :items="availableDomainStrings"
            :loading="domainsLoading"
            :label="$t('dnsManage.domainLabel', '托管根域名 (如 example.com)')"
            :placeholder="domainsLoading ? $t('dnsManage.loadingDomains', '正在获取云端托管根域名...') : (availableDomainStrings.length ? $t('dnsManage.selectOrInputDomain', '请选择或输入托管根域名') : 'example.com')"
            density="compact"
            variant="outlined"
            hide-details
            prepend-inner-icon="mdi-web"
            clearable
            :menu-props="{ maxHeight: 300, openOnClick: true }"
            :hide-no-data="false"
            :auto-select-first="false"
            @keydown.enter="fetchRecords(false)"
            @update:model-value="onDomainChanged"
          >
            <template #item="{ props, item }">
              <v-list-item v-bind="props">
                <template #prepend>
                  <v-icon
                    size="small"
                    :color="domainMetaMap[item.raw]?.isFavorite ? 'warning' : 'primary'"
                  >
                    {{ domainMetaMap[item.raw]?.isFavorite ? 'mdi-star' : (domainMetaMap[item.raw]?.isCloud ? 'mdi-cloud-outline' : 'mdi-web') }}
                  </v-icon>
                </template>
                <template #append>
                  <v-chip
                    v-if="domainMetaMap[item.raw]?.isDefaultFavorite"
                    size="x-small"
                    color="warning"
                    variant="tonal"
                    class="mr-1"
                  >
                    {{ $t('dnsManage.defaultFavTag', '默认') }}
                  </v-chip>
                  <v-chip
                    v-if="domainMetaMap[item.raw]?.isCloud"
                    size="x-small"
                    color="primary"
                    variant="tonal"
                  >
                    {{ $t('dnsManage.cloudTag', '云端托管') }}
                  </v-chip>
                  <v-chip
                    v-else-if="domainMetaMap[item.raw]?.isFavorite"
                    size="x-small"
                    color="secondary"
                    variant="tonal"
                  >
                    {{ $t('dnsManage.favTag', '常用') }}
                  </v-chip>
                </template>
              </v-list-item>
            </template>
            <template #no-data>
              <v-list-item>
                <v-list-item-title class="text-caption text-medium-emphasis">
                  <span v-if="domainsLoading">{{ $t('dnsManage.loadingDomains', '正在获取云端托管根域名...') }}</span>
                  <span v-else-if="domainFetchError" class="text-warning">{{ domainFetchError }}</span>
                  <span v-else>{{ $t('dnsManage.noDomainsHint', '暂未获取到云端托管域名，支持直接输入') }}</span>
                </v-list-item-title>
              </v-list-item>
            </template>
            <template #append-inner>
              <v-btn
                v-if="selectedDomain"
                :icon="isCurrentDomainFavorite ? 'mdi-star' : 'mdi-star-outline'"
                size="x-small"
                variant="text"
                :color="isCurrentDomainFavorite ? 'warning' : undefined"
                :title="isCurrentDomainFavorite ? $t('dnsManage.unfavoriteDomain', '取消常用域名') : $t('dnsManage.saveFavorite', '收藏为常用域名')"
                @click.stop="toggleFavoriteDomain"
              />
            </template>
          </v-combobox>
        </v-col>

        <v-col cols="12" sm="2" md="3" class="d-flex ga-2">
          <v-btn
            color="primary"
            variant="flat"
            prepend-icon="mdi-magnify"
            :loading="recordsLoading"
            :disabled="!selectedAccountId || !selectedDomain"
            block
            @click="fetchRecords(false)"
          >
            {{ $t('dnsManage.fetchRecords', '查询记录') }}
          </v-btn>
        </v-col>
      </v-row>
    </v-card>

    <!-- Records Table Container -->
    <v-card rounded="xl" variant="outlined" class="mt-4 dns-card-wrapper" :loading="recordsLoading">
      <v-card-title class="d-flex align-center justify-space-between flex-wrap py-3 px-4 ga-3">
        <div class="d-flex align-center ga-2">
          <v-icon color="primary" size="22">mdi-format-list-bulleted</v-icon>
          <span class="text-subtitle-1 font-weight-bold">{{ $t('dnsManage.recordsList', '解析记录列表') }}</span>
          <v-chip size="x-small" color="primary" variant="tonal" class="ml-1 font-weight-medium">
            {{ filteredRecords.length }} / {{ records.length }}
          </v-chip>
        </div>

        <div class="d-flex align-center ga-2 flex-grow-1 justify-end flex-wrap" style="max-width: 620px;">
          <v-text-field
            v-model="searchText"
            density="compact"
            variant="outlined"
            :placeholder="$t('dnsManage.searchPlaceholder', '搜索主机记录 / 类型 / 解析值...')"
            prepend-inner-icon="mdi-magnify"
            clearable
            hide-details
            style="min-width: 200px; max-width: 280px;"
          />
          <v-select
            v-model="typeFilter"
            :items="typeFilterOptions"
            item-title="title"
            item-value="value"
            density="compact"
            variant="outlined"
            hide-details
            style="min-width: 110px; max-width: 140px;"
          />
          <v-btn
            size="small"
            variant="tonal"
            icon="mdi-refresh"
            :loading="recordsLoading"
            :disabled="!selectedAccountId || !selectedDomain"
            :title="$t('refresh', '刷新列表')"
            @click="fetchRecords(false)"
          />
        </div>
      </v-card-title>
      <v-divider />

      <!-- Empty State: Not selected -->
      <div v-if="!selectedAccountId || !selectedDomain" class="text-center py-12 text-medium-emphasis">
        <v-icon size="48" class="mb-2">mdi-cloud-search-outline</v-icon>
        <div class="text-body-1">{{ $t('dnsManage.emptyPrompt', '请选择 DNS 账号并输入根域名，点击“查询记录”拉取云端解析') }}</div>
      </div>

      <!-- Empty State: No records found -->
      <div v-else-if="records.length === 0 && !recordsLoading" class="text-center py-12 text-medium-emphasis">
        <v-icon size="48" class="mb-2">mdi-dns-outline</v-icon>
        <div class="text-body-1">{{ $t('dnsManage.noRecordsFound', '该域名暂无云端解析记录') }}</div>
        <div class="text-caption mt-1">{{ $t('dnsManage.createPrompt', '点击右上角“新建解析记录”添加首条解析') }}</div>
        <v-btn class="mt-4" variant="outlined" color="primary" prepend-icon="mdi-plus" @click="openRecordDialog()">
          {{ $t('dnsManage.newRecord', '新建解析记录') }}
        </v-btn>
      </div>

      <!-- Desktop Table (Width > 960px) -->
      <div v-else class="d-none d-md-block">
        <v-table hover density="comfortable" class="dns-table">
          <thead>
            <tr>
              <th style="width: 16%;">{{ $t('dnsManage.colHost', '主机记录 (Name)') }}</th>
              <th style="width: 12%;">{{ $t('dnsManage.colType', '记录类型 (Type)') }}</th>
              <th style="width: 32%;">{{ $t('dnsManage.colValue', '记录值 (Value / Target)') }}</th>
              <th style="width: 12%;">TTL</th>
              <th v-if="isCloudflare" style="width: 12%;">{{ $t('dnsManage.colProxy', 'CDN 代理') }}</th>
              <th style="width: 8%;">{{ $t('dnsManage.colPriority', '优先级') }}</th>
              <th style="width: 8%;" class="text-right">{{ $t('actions.operate', '操作') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="record in filteredRecords" :key="record.id">
              <!-- Name -->
              <td>
                <span class="font-weight-bold text-body-2">{{ record.name }}</span>
                <span class="text-caption text-medium-emphasis ml-1">.{{ selectedDomain }}</span>
              </td>

              <!-- Type -->
              <td>
                <v-chip size="small" :color="getTypeColor(record.type)" variant="tonal" class="font-weight-bold">
                  {{ record.type }}
                </v-chip>
              </td>

              <!-- Value -->
              <td>
                <div class="d-flex align-center ga-2 text-truncate" style="max-width: 380px;">
                  <span class="text-body-2 font-monospace text-truncate" :title="record.value">{{ record.value }}</span>
                  <v-btn
                    icon="mdi-content-copy"
                    size="x-small"
                    variant="text"
                    color="medium-emphasis"
                    :title="$t('copyToClipboard', '复制')"
                    @click="copyToClipboard(record.value, $t('dnsManage.colValue', '记录值'))"
                  />
                </div>
              </td>

              <!-- TTL -->
              <td>
                <span class="text-body-2">{{ formatTTL(record.ttl) }}</span>
              </td>

              <!-- Proxied (Cloudflare) -->
              <td v-if="isCloudflare">
                <template v-if="canProxy(record.type)">
                  <v-btn
                    size="x-small"
                    variant="tonal"
                    :color="record.proxied ? 'orange-darken-1' : 'medium-emphasis'"
                    :prepend-icon="record.proxied ? 'mdi-cloud-check' : 'mdi-cloud-off-outline'"
                    :loading="mutationBusy"
                    @click="toggleRecordProxy(record)"
                  >
                    {{ record.proxied ? $t('dnsManage.proxied', '已代理') : $t('dnsManage.dnsOnly', '仅 DNS') }}
                  </v-btn>
                </template>
                <template v-else>
                  <span class="text-caption text-medium-emphasis">-</span>
                </template>
              </td>

              <!-- Priority -->
              <td>
                <span class="text-body-2">{{ record.type === 'MX' && record.priority ? record.priority : '-' }}</span>
              </td>

              <!-- Actions -->
              <td class="text-right">
                <div class="d-flex align-center justify-end ga-1">
                  <v-btn
                    icon="mdi-pencil"
                    size="small"
                    variant="text"
                    color="primary"
                    :title="$t('actions.edit', '编辑')"
                    @click="openRecordDialog(record)"
                  />
                  <v-btn
                    icon="mdi-delete"
                    size="small"
                    variant="text"
                    color="error"
                    :loading="deletingRecordId === record.id"
                    :title="$t('actions.del', '删除')"
                    @click="removeRecord(record)"
                  />
                </div>
              </td>
            </tr>
          </tbody>
        </v-table>
      </div>

      <!-- Mobile Cards (Width <= 960px) -->
      <div v-if="records.length > 0" class="d-md-none pa-3">
        <v-row dense>
          <v-col v-for="record in filteredRecords" :key="record.id" cols="12">
            <v-card variant="outlined" rounded="lg" class="pa-3 mb-2 dns-mobile-card">
              <div class="d-flex align-center justify-space-between flex-wrap ga-2">
                <div class="d-flex align-center ga-2">
                  <v-chip size="x-small" :color="getTypeColor(record.type)" variant="tonal" class="font-weight-bold">
                    {{ record.type }}
                  </v-chip>
                  <span class="font-weight-bold text-body-1">{{ record.name }}</span>
                  <span class="text-caption text-medium-emphasis">.{{ selectedDomain }}</span>
                </div>
                <div class="d-flex align-center ga-1">
                  <v-btn
                    icon="mdi-pencil"
                    size="x-small"
                    variant="tonal"
                    color="primary"
                    @click="openRecordDialog(record)"
                  />
                  <v-btn
                    icon="mdi-delete"
                    size="x-small"
                    variant="tonal"
                    color="error"
                    :loading="deletingRecordId === record.id"
                    @click="removeRecord(record)"
                  />
                </div>
              </div>

              <div class="mt-2 text-body-2 font-monospace text-break d-flex align-center justify-space-between">
                <span>{{ record.value }}</span>
                <v-btn
                  icon="mdi-content-copy"
                  size="x-small"
                  variant="text"
                  @click="copyToClipboard(record.value, $t('dnsManage.colValue', '记录值'))"
                />
              </div>

              <div class="d-flex align-center justify-space-between text-caption text-medium-emphasis mt-2 pt-2 border-t">
                <span>TTL: {{ formatTTL(record.ttl) }}</span>
                <span v-if="record.type === 'MX' && record.priority">
                  {{ $t('dnsManage.colPriority', '优先级') }}: {{ record.priority }}
                </span>
                <span v-if="isCloudflare && canProxy(record.type)">
                  <v-chip
                    size="x-small"
                    :color="record.proxied ? 'orange-darken-1' : 'medium-emphasis'"
                    variant="tonal"
                    @click="toggleRecordProxy(record)"
                  >
                    {{ record.proxied ? $t('dnsManage.proxied', '已代理') : $t('dnsManage.dnsOnly', '仅 DNS') }}
                  </v-chip>
                </span>
              </div>
            </v-card>
          </v-col>
        </v-row>
      </div>

      <!-- Card Footer -->
      <v-divider />
      <div class="px-4 py-3 d-flex align-center justify-space-between flex-wrap text-caption text-medium-emphasis ga-2">
        <div>
          <span>{{ $t('dnsManage.footerSummary', { current: filteredRecords.length, total: records.length }) }}</span>
        </div>
        <div class="d-flex align-center ga-2">
          <v-icon size="16">mdi-information-outline</v-icon>
          <span>{{ $t('dnsManage.footerNotice', '添加、修改或删除操作将直接调用云厂商 OpenAPI 实时同步到全球 DNS。') }}</span>
        </div>
      </div>
    </v-card>

    <!-- Dialog: Create / Edit Record -->
    <v-dialog v-model="recordDialogVisible" max-width="580" persistent>
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between px-4 py-3">
          <div class="d-flex align-center ga-2">
            <v-icon color="primary">{{ isEditingRecord ? 'mdi-pencil' : 'mdi-plus-circle' }}</v-icon>
            <span class="text-h6 font-weight-bold">
              {{ isEditingRecord ? $t('dnsManage.editRecord', '编辑解析记录') : $t('dnsManage.newRecord', '新建解析记录') }}
            </span>
          </div>
          <v-btn icon="mdi-close" variant="text" density="compact" @click="recordDialogVisible = false" />
        </v-card-title>
        <v-divider />

        <v-card-text class="pa-4">
          <v-form @submit.prevent="saveRecord">
            <v-row dense>
              <!-- Type -->
              <v-col cols="12" sm="5">
                <v-select
                  v-model="recordForm.type"
                  :items="dnsRecordTypes"
                  :label="$t('dnsManage.colType', '记录类型')"
                  variant="outlined"
                  density="compact"
                  hide-details="auto"
                  :disabled="isEditingRecord"
                />
              </v-col>

              <!-- Name -->
              <v-col cols="12" sm="7">
                <v-text-field
                  v-model="recordForm.name"
                  :label="$t('dnsManage.colHost', '主机记录 (RR)')"
                  placeholder="@, www, mail, *"
                  variant="outlined"
                  density="compact"
                  hide-details="auto"
                  :suffix="'.' + selectedDomain"
                />
              </v-col>

              <!-- Value -->
              <v-col cols="12">
                <v-text-field
                  v-model="recordForm.value"
                  :label="$t('dnsManage.colValue', '记录值 (Value / Target)')"
                  :placeholder="getValuePlaceholder(recordForm.type)"
                  variant="outlined"
                  density="compact"
                  hide-details="auto"
                  required
                />
              </v-col>

              <!-- TTL -->
              <v-col cols="12" sm="6">
                <v-select
                  v-model="recordForm.ttl"
                  :items="ttlOptions"
                  item-title="title"
                  item-value="value"
                  label="TTL"
                  variant="outlined"
                  density="compact"
                  hide-details="auto"
                />
              </v-col>

              <!-- MX Priority (Conditional) -->
              <v-col v-if="recordForm.type === 'MX'" cols="12" sm="6">
                <v-text-field
                  v-model.number="recordForm.priority"
                  :label="$t('dnsManage.colPriority', 'MX 优先级')"
                  type="number"
                  placeholder="10"
                  variant="outlined"
                  density="compact"
                  hide-details="auto"
                />
              </v-col>

              <!-- Cloudflare Proxy (Conditional) -->
              <v-col v-if="isCloudflare && canProxy(recordForm.type)" cols="12">
                <v-switch
                  v-model="recordForm.proxied"
                  color="orange-darken-1"
                  hide-details
                  density="compact"
                  :label="$t('dnsManage.enableCfProxy', '开启 Cloudflare CDN 代理 (小云朵)')"
                />
              </v-col>
            </v-row>
          </v-form>
        </v-card-text>
        <v-divider />

        <v-card-actions class="px-4 py-3">
          <v-spacer />
          <v-btn variant="text" :disabled="mutationBusy" @click="recordDialogVisible = false">
            {{ $t('actions.cancel', '取消') }}
          </v-btn>
          <v-btn color="primary" variant="flat" :loading="mutationBusy" @click="saveRecord">
            {{ $t('actions.save', '保存') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Dialog: Account Management (Dedicated for DNS Management) -->
    <v-dialog v-model="accountManageVisible" max-width="760">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between px-4 py-3">
          <div class="d-flex align-center ga-2">
            <v-icon color="primary">mdi-key-chain</v-icon>
            <span class="text-h6 font-weight-bold">{{ $t('dnsManage.accountManageTitle', 'DNS 解析账号管理') }}</span>
          </div>
          <v-btn icon="mdi-close" variant="text" density="compact" @click="accountManageVisible = false" />
        </v-card-title>
        <v-divider />

        <v-card-text class="pa-4">
          <div class="d-flex align-center justify-space-between mb-3">
            <span class="text-body-2 text-medium-emphasis">
              {{ $t('dnsManage.accountIndependentNotice', '此处的账号仅用于 DNS 解析管理，与 DDNS 模块完全独立。') }}
            </span>
            <v-btn size="small" color="primary" prepend-icon="mdi-plus" variant="tonal" @click="openAccountDialog()">
              {{ $t('ddns.addAccount', '添加账号') }}
            </v-btn>
          </div>

          <v-table density="compact" class="border rounded-lg">
            <thead>
              <tr>
                <th>{{ $t('ddns.accountName', '账号名称') }}</th>
                <th>{{ $t('ddns.provider', '供应商') }}</th>
                <th>{{ $t('dnsManage.supportGeneral', '通用 DNS') }}</th>
                <th>{{ $t('ddns.remark', '备注') }}</th>
                <th class="text-right">{{ $t('actions.operate', '操作') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="acc in overview.accounts" :key="acc.id">
                <td class="font-weight-medium">{{ acc.name }}</td>
                <td>
                  <v-chip size="x-small" color="primary" variant="tonal">
                    {{ providersMap[acc.providerCode]?.name || acc.providerCode }}
                  </v-chip>
                </td>
                <td>
                  <v-chip
                    size="x-small"
                    :color="providersMap[acc.providerCode]?.supportGeneralDns ? 'success' : 'default'"
                    variant="tonal"
                  >
                    {{ providersMap[acc.providerCode]?.supportGeneralDns ? $t('dnsManage.supported', '支持') : $t('dnsManage.ddnsOnly', '仅 DDNS') }}
                  </v-chip>
                </td>
                <td class="text-medium-emphasis text-caption">{{ acc.remark || '-' }}</td>
                <td class="text-right">
                  <v-btn icon="mdi-pencil" size="x-small" variant="text" color="primary" @click="openAccountDialog(acc)" />
                  <v-btn icon="mdi-delete" size="x-small" variant="text" color="error" @click="removeAccount(acc)" />
                </td>
              </tr>
              <tr v-if="overview.accounts.length === 0">
                <td colspan="5" class="text-center py-4 text-medium-emphasis">{{ $t('ddns.noAccounts', '暂无 DNS 账号') }}</td>
              </tr>
            </tbody>
          </v-table>
        </v-card-text>
      </v-card>
    </v-dialog>

    <!-- Dialog: Account Edit / Create Form -->
    <v-dialog v-model="accountDialogVisible" max-width="520" persistent>
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between px-4 py-3">
          <div class="d-flex align-center ga-2">
            <v-icon color="primary">{{ accountForm.id ? 'mdi-pencil' : 'mdi-plus-circle' }}</v-icon>
            <span class="text-h6 font-weight-bold">
              {{ accountForm.id ? $t('ddns.editAccount', '编辑 DNS 账号') : $t('ddns.addAccount', '添加 DNS 账号') }}
            </span>
          </div>
          <v-btn icon="mdi-close" variant="text" density="compact" @click="accountDialogVisible = false" />
        </v-card-title>
        <v-divider />

        <v-card-text class="pa-4">
          <v-form @submit.prevent="saveAccount">
            <v-text-field
              v-model="accountForm.name"
              :label="$t('ddns.accountName', '账号名称')"
              placeholder="如：我的 Cloudflare 主账号"
              variant="outlined"
              density="compact"
              hide-details="auto"
              class="mb-3"
              required
            />

            <v-select
              v-model="accountForm.providerCode"
              :items="overview.providers"
              item-title="name"
              item-value="code"
              :label="$t('ddns.provider', 'DNS 供应商')"
              variant="outlined"
              density="compact"
              hide-details="auto"
              class="mb-3"
            />

            <v-alert v-if="selectedAccountProvider?.helper" type="info" variant="tonal" density="compact" class="mb-3">
              <span class="text-caption">{{ selectedAccountProvider.helper }}</span>
            </v-alert>

            <!-- Dynamic Fields -->
            <div v-if="selectedAccountProvider">
              <div v-for="field in selectedAccountProvider.fields" :key="field.key" class="mb-3">
                <v-text-field
                  v-model="accountEnvMap[field.key]"
                  :label="field.label"
                  :placeholder="field.placeholder"
                  :type="field.secret ? 'password' : 'text'"
                  variant="outlined"
                  density="compact"
                  hide-details="auto"
                  :required="field.required"
                />
              </div>
            </div>

            <v-text-field
              v-model="accountForm.remark"
              :label="$t('ddns.remark', '备注')"
              variant="outlined"
              density="compact"
              hide-details="auto"
              class="mt-1"
            />
          </v-form>
        </v-card-text>
        <v-divider />

        <v-card-actions class="px-4 py-3">
          <v-btn variant="outlined" color="info" :loading="testAuthBusy" @click="testAccountAuth">
            {{ $t('ddns.testAuth', '测试授权') }}
          </v-btn>
          <v-spacer />
          <v-btn variant="text" :disabled="mutationBusy" @click="accountDialogVisible = false">
            {{ $t('actions.cancel', '取消') }}
          </v-btn>
          <v-btn color="primary" variant="flat" :loading="mutationBusy" @click="saveAccount">
            {{ $t('actions.save', '保存') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup lang="ts">
import { toRef } from 'vue'
import { useDnsManage } from './SettingsDnsManage.shared'

const props = defineProps<{
  active: boolean
}>()

const activeRef = toRef(props, 'active')
const {
  loading,
  recordsLoading,
  mutationBusy,
  testAuthBusy,
  deletingRecordId,
  overview,
  selectedAccountId,
  selectedDomain,
  domainsLoading,
  domainFetchError,
  availableDomainStrings,
  domainMetaMap,
  records,
  filteredRecords,
  searchText,
  typeFilter,
  recordDialogVisible,
  isEditingRecord,
  recordForm,
  accountManageVisible,
  accountDialogVisible,
  accountForm,
  accountEnvMap,
  generalAccounts,
  providersMap,
  currentAccount,
  currentProvider,
  isCloudflare,
  selectedAccountProvider,
  fetchRecords,
  onAccountChanged,
  onDomainChanged,
  isCurrentDomainFavorite,
  toggleFavoriteDomain,
  saveAsFavoriteDomain,
  openRecordDialog,
  saveRecord,
  removeRecord,
  toggleRecordProxy,
  copyToClipboard,
  openAccountDialog,
  testAccountAuth,
  saveAccount,
  removeAccount,
} = useDnsManage(activeRef)

const dnsRecordTypes = ['A', 'AAAA', 'CNAME', 'TXT', 'MX', 'NS', 'CAA', 'SRV']

const typeFilterOptions = [
  { title: '全部类型', value: 'all' },
  { title: 'A 记录', value: 'A' },
  { title: 'AAAA 记录', value: 'AAAA' },
  { title: 'CNAME 记录', value: 'CNAME' },
  { title: 'TXT 记录', value: 'TXT' },
  { title: 'MX 记录', value: 'MX' },
  { title: 'NS 记录', value: 'NS' },
  { title: 'CAA 记录', value: 'CAA' },
]

const ttlOptions = [
  { title: '自动 (Auto)', value: 1 },
  { title: '60 秒 (1 分钟)', value: 60 },
  { title: '120 秒 (2 分钟)', value: 120 },
  { title: '300 秒 (5 分钟)', value: 300 },
  { title: '600 秒 (10 分钟)', value: 600 },
  { title: '1800 秒 (30 分钟)', value: 1800 },
  { title: '3600 秒 (1 小时)', value: 3600 },
  { title: '86400 秒 (1 天)', value: 86400 },
]

function getTypeColor(type: string) {
  switch (type.toUpperCase()) {
    case 'A':
      return 'primary'
    case 'AAAA':
      return 'cyan-darken-1'
    case 'CNAME':
      return 'purple'
    case 'TXT':
      return 'amber-darken-2'
    case 'MX':
      return 'green-darken-1'
    case 'NS':
      return 'blue-grey'
    default:
      return 'indigo'
  }
}

function canProxy(type: string) {
  const t = type.toUpperCase()
  return t === 'A' || t === 'AAAA' || t === 'CNAME'
}

function formatTTL(ttl: number) {
  if (ttl <= 1) return '自动'
  if (ttl < 60) return `${ttl}s`
  if (ttl < 3600) return `${Math.round(ttl / 60)}m`
  if (ttl < 86400) return `${Math.round(ttl / 3600)}h`
  return `${Math.round(ttl / 86400)}d`
}

function getValuePlaceholder(type: string) {
  switch (type.toUpperCase()) {
    case 'A':
      return '例如：192.0.2.1'
    case 'AAAA':
      return '例如：2001:db8::1'
    case 'CNAME':
      return '例如：target.example.com'
    case 'TXT':
      return '例如：v=spf1 include:_spf.google.com ~all'
    case 'MX':
      return '例如：mail.example.com'
    default:
      return '输入解析记录内容'
  }
}
</script>

<style scoped>
.dns-page {
  padding-bottom: 24px;
}
.dns-hero {
  border: 1px solid rgba(var(--v-border-color), var(--v-border-opacity));
}
.dns-hero__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 16px;
}
.dns-hero__icon {
  width: 52px;
  height: 52px;
  border-radius: 14px;
  background: rgba(var(--v-theme-primary), 0.1);
  color: rgb(var(--v-theme-primary));
  display: flex;
  align-items: center;
  justify-content: center;
}
.dns-hero__toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.dns-scope-card {
  background: rgba(var(--v-theme-surface), 0.5);
}
.dns-table {
  width: 100%;
}
.dns-mobile-card {
  border: 1px solid rgba(var(--v-border-color), 0.15);
  word-break: break-all;
  overflow-wrap: anywhere;
}
.font-monospace {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
