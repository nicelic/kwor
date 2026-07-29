<template>
  <section class="rp-page">
    <v-row class="mt-1">
      <v-col cols="12" xl="8">
        <v-card class="rp-hero" rounded="xl" :loading="loading && !hasLoaded">
          <div class="rp-hero__bg"></div>
          <v-card-text class="rp-hero__content">
            <div class="rp-hero__top">
              <div class="d-flex align-center ga-3">
                <div class="rp-hero__icon">
                  <v-icon size="30">mdi-source-branch</v-icon>
                </div>
                <div>
                  <div class="text-overline rp-hero__eyebrow">{{ reverseProxyCopy.heroEyebrow }}</div>
                  <div class="text-h5 font-weight-bold">{{ reverseProxyCopy.title }}</div>
                  <div class="text-body-2 text-medium-emphasis mt-1">{{ reverseProxyCopy.subtitle }}</div>
                </div>
              </div>
              <div class="rp-hero__toolbar">
                <v-btn
                  class="rp-hero-action"
                  variant="outlined"
                  prepend-icon="mdi-refresh"
                  :loading="refreshing"
                  :disabled="mutationBusy"
                  @click="refreshOverview">
                  {{ reverseProxyCopy.refresh }}
                </v-btn>
                <v-btn
                  class="rp-hero-action"
                  variant="outlined"
                  prepend-icon="mdi-plus"
                  :disabled="actionsDisabled"
                  @click="openRuleDialog()">
                  {{ reverseProxyCopy.newRule }}
                </v-btn>
              </div>
            </div>

            <div class="rp-hero__chips">
              <v-chip size="small" :color="overview.available ? 'success' : 'warning'" variant="flat">
                {{ overview.available ? reverseProxyCopy.available : reverseProxyCopy.unavailable }}
              </v-chip>
              <v-chip size="small" color="secondary" variant="flat" class="rp-hero-chip rp-hero-chip--sync">
                {{ reverseProxyCopy.lastSync }}: {{ lastSyncLabel }}
              </v-chip>
              <v-chip size="small" color="primary" variant="flat" class="rp-hero-chip rp-hero-chip--count">
                {{ reverseProxyCopy.listeners }} {{ overview.listenerCount }}
              </v-chip>
            </div>

            <v-row class="mt-2">
              <v-col cols="12" sm="4">
                <div class="rp-metric">
                  <div class="text-caption rp-muted-label">{{ reverseProxyCopy.enabledRules }}</div>
                  <div class="text-h5 mt-1">{{ overview.enabledCount }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="4">
                <div class="rp-metric">
                  <div class="text-caption rp-muted-label">{{ reverseProxyCopy.totalRules }}</div>
                  <div class="text-h5 mt-1">{{ overview.ruleCount }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="4">
                <div class="rp-metric">
                  <div class="text-caption rp-muted-label">{{ reverseProxyCopy.certificates }}</div>
                  <div class="text-h5 mt-1">{{ overview.certificateCount }}</div>
                </div>
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" xl="4">
        <v-card rounded="xl" variant="outlined" class="rp-side">
          <v-card-title class="text-subtitle-1 font-weight-medium">{{ reverseProxyCopy.runtimeTitle }}</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="rp-side__row">
              <span>{{ reverseProxyCopy.runtimeStatus }}</span>
              <strong :class="overview.started ? 'text-success' : 'text-warning'">
                {{ overview.started ? reverseProxyCopy.running : reverseProxyCopy.stopped }}
              </strong>
            </div>
            <div class="rp-side__row">
              <span>{{ reverseProxyCopy.listeners }}</span>
              <strong>{{ overview.listenerCount }}</strong>
            </div>
            <div class="rp-side__row">
              <span>{{ reverseProxyCopy.certificates }}</span>
              <strong>{{ overview.certificateCount }}</strong>
            </div>
            <v-alert
              v-if="overview.error"
              type="warning"
              variant="tonal"
              density="comfortable"
              class="mt-4">
              {{ overview.error }}
            </v-alert>
            <v-alert
              v-else
              type="info"
              variant="tonal"
              density="comfortable"
              class="mt-4">
              {{ reverseProxyCopy.runtimeHint }}
            </v-alert>
            <v-alert
              v-if="overview.warnings?.length"
              type="warning"
              variant="tonal"
              density="comfortable"
              class="mt-3">
              {{ overview.warnings.join('；') }}
            </v-alert>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-card rounded="xl" variant="outlined" class="rp-resource-card mt-1">
      <v-card-title class="rp-resource-card__header">
        <div>
          <div class="text-subtitle-1 font-weight-medium">{{ reverseProxyCopy.resourceTitle }}</div>
          <div class="text-caption text-medium-emphasis mt-1">{{ reverseProxyCopy.resourceSubtitle }}</div>
        </div>
        <v-btn
          color="primary"
          variant="outlined"
          prepend-icon="mdi-tune-variant"
          :disabled="actionsDisabled"
          @click="openResourceDialog">
          {{ reverseProxyCopy.resourceEdit }}
        </v-btn>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <v-row dense>
          <v-col cols="12" sm="6" md="3">
            <div class="rp-resource-metric">
              <span>{{ reverseProxyCopy.listenerConnectionLimit }}</span>
              <strong>{{ overview.resourceSettings.listenerConnectionLimit || '不限额' }}</strong>
            </div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="rp-resource-metric">
              <span>{{ reverseProxyCopy.globalHttpMaxConcurrent }}</span>
              <strong>{{ overview.resourceSettings.globalHttpMaxConcurrent || '不限额' }} / {{ runtimeUsage.activeHttpRequests }}</strong>
            </div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="rp-resource-metric">
              <span>{{ reverseProxyCopy.globalDnsMaxConcurrent }}</span>
              <strong>{{ overview.resourceSettings.globalDnsMaxConcurrent || '不限额' }} / {{ runtimeUsage.activeDnsQueries }}</strong>
            </div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="rp-resource-metric">
              <span>{{ reverseProxyCopy.http2MaxConcurrentStreams }}</span>
              <strong>{{ overview.resourceSettings.http2MaxConcurrentStreams }}</strong>
            </div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="rp-resource-metric">
              <span>{{ reverseProxyCopy.quicMaxIncomingStreams }}</span>
              <strong>{{ overview.resourceSettings.quicMaxIncomingStreams }}</strong>
            </div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="rp-resource-metric">
              <span>{{ reverseProxyCopy.defaultUpstreamMaxIdleConnections }}</span>
              <strong>{{ overview.resourceSettings.defaultUpstreamMaxIdleConnections || '不限额' }}</strong>
            </div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="rp-resource-metric">
              <span>{{ reverseProxyCopy.memoryPoolBytes }}</span>
              <strong>{{ formatReverseProxyBytes(runtimeUsage.memoryUsedBytes) }} / {{ formatReverseProxyBytes(overview.resourceSettings.memoryPoolBytes) }}</strong>
              <small>{{ reverseProxyCopy.runtimeMemory }}</small>
            </div>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <div class="rp-resource-metric">
              <span>{{ reverseProxyCopy.responseRewriteMaxConcurrent }}</span>
              <strong>{{ overview.resourceSettings.responseRewriteMaxConcurrent }}</strong>
              <small>{{ reverseProxyCopy.runtimeCache }} {{ formatReverseProxyBytes(runtimeUsage.cacheUsedBytes) }} · {{ reverseProxyCopy.runtimeRewrite }} {{ formatReverseProxyBytes(runtimeUsage.rewriteUsedBytes) }}</small>
            </div>
          </v-col>
        </v-row>
        <v-alert type="info" variant="tonal" density="comfortable" class="mt-4">
          {{ reverseProxyCopy.resourceMemoryHint }}
        </v-alert>
      </v-card-text>
    </v-card>

    <v-alert
      v-if="configurationConflict"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mt-4">
      {{ reverseProxyCopy.revisionConflict }}
    </v-alert>

    <v-alert
      v-if="loadError"
      type="error"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      <div class="d-flex align-center justify-space-between ga-3 flex-wrap">
        <span>{{ reverseProxyCopy.loadFailed }}：{{ loadError }}</span>
        <v-btn variant="text" prepend-icon="mdi-refresh" :loading="refreshing" :disabled="mutationBusy" @click="refreshOverview">
          {{ reverseProxyCopy.refresh }}
        </v-btn>
      </div>
    </v-alert>

    <v-alert
      v-if="hasLoaded && !overview.available"
      type="info"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      {{ reverseProxyCopy.unavailableHint }}
    </v-alert>

    <v-card rounded="xl" variant="outlined" class="rp-table-card">
      <v-card-title class="rp-table-card__toolbar">
        <div>
          <div class="text-subtitle-1 font-weight-medium">{{ reverseProxyCopy.tableTitle }}</div>
          <div class="text-caption text-medium-emphasis mt-1">{{ reverseProxyCopy.tableSubtitle }}</div>
        </div>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <v-row class="mb-2">
          <v-col cols="12" md="5">
            <v-text-field
              v-model="searchText"
              :label="reverseProxyCopy.search"
              prepend-inner-icon="mdi-magnify"
              clearable
              hide-details />
          </v-col>
        </v-row>

        <v-data-table
          v-if="!smAndDown"
          :headers="reverseProxyHeaders"
          :items="filteredRules"
          item-value="id"
          class="rp-table"
          hide-no-data
          fixed-header>
          <template #item.displayId="{ item }">
            <div class="font-weight-medium">{{ item.displayId }}</div>
          </template>

          <template #item.listOrder="{ item }">
            <div class="font-weight-medium">{{ item.listOrder }}</div>
          </template>

          <template #item.status="{ item }">
            <div class="py-2">
              <v-chip size="small" :color="statusColor(item.runtimeStatus)" variant="flat">
                {{ runtimeStatusLabel(item.runtimeStatus) }}
              </v-chip>
              <div class="text-caption text-medium-emphasis mt-1">
                {{ item.enabled ? reverseProxyCopy.ruleEnabled : reverseProxyCopy.ruleDisabled }}
              </div>
              <div v-if="item.lastError" class="text-caption text-warning mt-1">
                {{ item.lastError }}
              </div>
            </div>
          </template>

          <template #item.listenProtocol="{ item }">
            <div class="py-2">
              <v-chip size="small" :color="item.listenProtocol === 'http' ? 'info' : 'success'" variant="flat">
                {{ protocolLabel(item.listenProtocol) }}
              </v-chip>
            </div>
          </template>

          <template #item.connectionCounts="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ connectionCountsDisplay(item) }}</div>
              <div class="text-caption text-medium-emphasis mt-1">{{ reverseProxyCopy.connectionHint }}</div>
            </div>
          </template>

          <template #item.listen="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">:{{ item.listenPort }}</div>
              <div class="text-caption text-medium-emphasis mt-1">
                {{ listenMatchDisplay(item) || '*' }}
              </div>
            </div>
          </template>

          <template #item.path="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ item.listenProtocol.startsWith('dns_') ? (item.listenDnsPath || '-') : (item.pathPrefix || '全部路径') }}</div>
            </div>
          </template>

          <template #item.target="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ protocolLabel(item.targetProtocol) }} -> {{ joinDisplay(item.targetAddresses) }}:{{ item.targetPort }}</div>
              <div class="text-caption text-medium-emphasis mt-1">{{ item.targetProtocol.startsWith('dns_') ? (item.targetDnsPath || '-') : (item.targetPath || '/') }}</div>
            </div>
          </template>

          <template #item.strategy="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ ipStrategyLabel(item.ipStrategy) }}</div>
              <div class="text-caption text-medium-emphasis mt-1">
                {{ httpVersionStrategyLabel(item.httpVersionStrategy, item.targetProtocol) }}
              </div>
            </div>
          </template>

          <template #item.certificate="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ certificateDisplay(item) }}</div>
            </div>
          </template>

          <template #item.remark="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ item.name || `#${item.displayId}` }}</div>
              <div class="text-caption text-medium-emphasis mt-1">{{ item.remark || '-' }}</div>
            </div>
          </template>

          <template #item.actions="{ item }">
            <div class="rp-actions">
              <v-switch
                class="rp-actions__switch"
                density="compact"
                color="success"
                hide-details
                inset
                :model-value="item.enabled"
                :loading="rowBusyId === item.id"
                :disabled="actionsDisabled"
                :aria-label="reverseProxyCopy.enableLabel"
                @update:modelValue="(value) => toggleRule(item, Boolean(value))" />
              <div class="rp-actions__buttons">
                <v-btn
                  icon="mdi-arrow-up"
                  variant="text"
                  size="small"
                  density="comfortable"
                  color="secondary"
                  class="rp-action-btn"
                  :aria-label="reverseProxyCopy.reorderUp"
                  :title="reverseProxyCopy.reorderUp"
                  :disabled="actionsDisabled"
                  @click.stop="moveRule(item, -1)" />
                <v-btn
                  icon="mdi-arrow-down"
                  variant="text"
                  size="small"
                  density="comfortable"
                  color="secondary"
                  class="rp-action-btn"
                  :aria-label="reverseProxyCopy.reorderDown"
                  :title="reverseProxyCopy.reorderDown"
                  :disabled="actionsDisabled"
                  @click.stop="moveRule(item, 1)" />
                <v-btn
                  icon="mdi-pencil"
                  variant="text"
                  size="small"
                  density="comfortable"
                  color="primary"
                  class="rp-action-btn"
                  :aria-label="reverseProxyCopy.edit"
                  :title="reverseProxyCopy.edit"
                  :disabled="actionsDisabled"
                  @click.stop="openRuleDialog(item)" />
                <v-btn
                  icon="mdi-delete"
                  variant="text"
                  size="small"
                  density="comfortable"
                  color="error"
                  class="rp-action-btn"
                  :aria-label="reverseProxyCopy.delete"
                  :title="reverseProxyCopy.delete"
                  :disabled="actionsDisabled"
                  @click.stop="removeRule(item)" />
              </div>
            </div>
          </template>
        </v-data-table>

        <div v-else class="rp-mobile-list">
          <v-card v-for="item in filteredRules" :key="item.id" variant="outlined" rounded="lg" class="rp-mobile-rule">
            <v-card-text>
              <div class="d-flex align-start justify-space-between ga-2">
                <div class="min-w-0">
                  <div class="font-weight-medium text-truncate">{{ item.name || `#${item.displayId}` }}</div>
                  <div v-if="item.remark" class="text-caption text-medium-emphasis mt-1 rp-wrap">{{ item.remark }}</div>
                </div>
                <div class="d-flex flex-column align-end ga-1">
                  <v-chip size="small" :color="statusColor(item.runtimeStatus)" variant="flat">{{ runtimeStatusLabel(item.runtimeStatus) }}</v-chip>
                  <v-chip size="x-small" :color="item.enabled ? 'success' : 'grey'" variant="tonal">
                    {{ item.enabled ? reverseProxyCopy.ruleEnabled : reverseProxyCopy.ruleDisabled }}
                  </v-chip>
                </div>
              </div>
              <div class="rp-mobile-grid mt-4">
                <div>
                  <span>监听</span>
                  <strong>{{ protocolLabel(item.listenProtocol) }} :{{ item.listenPort }}</strong>
                  <small>{{ listenMatchDisplay(item) || '*' }}</small>
                  <small>{{ item.listenProtocol.startsWith('dns_') ? (item.listenDnsPath || '-') : (item.pathPrefix || '全部路径') }}</small>
                </div>
                <div>
                  <span>{{ reverseProxyCopy.targetLabel }}</span>
                  <strong>{{ protocolLabel(item.targetProtocol) }} :{{ item.targetPort }}</strong>
                  <small>{{ joinDisplay(item.targetAddresses) }}</small>
                  <small>{{ item.targetProtocol.startsWith('dns_') ? (item.targetDnsPath || '-') : (item.targetPath || '/') }}</small>
                </div>
                <div>
                  <span>{{ reverseProxyCopy.connectionLabel }}</span>
                  <strong>{{ connectionCountsDisplay(item) }}</strong>
                  <small>{{ reverseProxyCopy.connectionHint }}</small>
                </div>
                <div>
                  <span>{{ reverseProxyCopy.certificateLabel }}</span>
                  <strong>{{ certificateDisplay(item) }}</strong>
                  <small>{{ ipStrategyLabel(item.ipStrategy) }} · {{ httpVersionStrategyLabel(item.httpVersionStrategy, item.targetProtocol) }}</small>
                </div>
              </div>
              <v-alert v-if="item.lastError" type="error" variant="tonal" density="compact" class="mt-3 rp-wrap">
                {{ item.lastError }}
              </v-alert>
              <div class="d-flex align-center justify-end ga-1 mt-3">
                <v-switch
                  density="compact"
                  color="success"
                  hide-details
                  inset
                  :model-value="item.enabled"
                  :loading="rowBusyId === item.id"
                  :disabled="actionsDisabled"
                  :aria-label="reverseProxyCopy.enableLabel"
                  @update:modelValue="(value) => toggleRule(item, Boolean(value))" />
                <v-btn icon="mdi-arrow-up" variant="text" size="small" color="secondary" :aria-label="reverseProxyCopy.reorderUp" :title="reverseProxyCopy.reorderUp" :disabled="actionsDisabled" @click="moveRule(item, -1)" />
                <v-btn icon="mdi-arrow-down" variant="text" size="small" color="secondary" :aria-label="reverseProxyCopy.reorderDown" :title="reverseProxyCopy.reorderDown" :disabled="actionsDisabled" @click="moveRule(item, 1)" />
                <v-btn icon="mdi-pencil" variant="text" size="small" color="primary" :aria-label="reverseProxyCopy.edit" :title="reverseProxyCopy.edit" :disabled="actionsDisabled" @click="openRuleDialog(item)" />
                <v-btn icon="mdi-delete" variant="text" size="small" color="error" :aria-label="reverseProxyCopy.delete" :title="reverseProxyCopy.delete" :disabled="actionsDisabled" @click="removeRule(item)" />
              </div>
            </v-card-text>
          </v-card>
        </div>

        <div v-if="hasLoaded && filteredRules.length === 0" class="rp-empty">
          <v-icon size="36" color="grey">mdi-lan-disconnect</v-icon>
          <div class="text-subtitle-2 mt-2">{{ reverseProxyCopy.empty }}</div>
        </div>
      </v-card-text>
    </v-card>

    <v-dialog v-model="dialogVisible" :fullscreen="smAndDown" scrollable max-width="1080">
      <v-card :rounded="smAndDown ? '0' : 'xl'">
        <v-card-title class="rp-dialog-title">
          <div class="rp-dialog-title__top">
            <div class="text-subtitle-1 font-weight-medium">{{ dialogTitle }}</div>
            <v-switch
              v-model="editingRule.enabled"
              color="success"
              hide-details
              inset
              :disabled="mutationBusy"
              :label="reverseProxyCopy.enableLabel" />
          </div>
          <div class="rp-dialog-title__subtitle text-caption text-medium-emphasis">{{ reverseProxyCopy.dialogSubtitle }}</div>
        </v-card-title>
        <v-alert v-if="configurationConflict" type="warning" variant="tonal" density="comfortable" class="mx-6 mb-4">
          {{ reverseProxyCopy.revisionConflict }}
        </v-alert>
        <v-divider />

        <v-card-text class="pt-5 rp-dialog-body">
          <v-row>
            <v-col cols="12" md="4">
              <v-text-field
                v-model="editingRule.name"
                :label="reverseProxyCopy.name"
                hide-details />
            </v-col>
            <v-col cols="12" md="8">
              <v-text-field
                v-model="editingRule.remark"
                :label="reverseProxyCopy.remark"
                hide-details />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12" lg="4">
              <div class="rp-panel">
                <div class="rp-panel__title">{{ reverseProxyCopy.listenPanel }}</div>
                <div class="rp-panel__subtitle">{{ reverseProxyCopy.listenPanelHint }}</div>
                <v-row class="mt-1">
                  <v-col cols="12" md="6" lg="12">
                    <v-select
                      :model-value="editingRule.listenProtocol"
                      :items="protocolItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.listenProtocol"
                      @update:modelValue="changeListenProtocol"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ listenProtocolBehavior }}</div>
                  </v-col>
                  <v-col v-if="!listenIsPlainDNS" cols="12" md="6" lg="12">
                    <v-text-field
                      v-model="editingRule.hostsText"
                      :label="reverseProxyCopy.hosts"
                      :placeholder="reverseProxyCopy.hostsPlaceholder"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.listenIpLocalHint }}</div>
                  </v-col>
                  <v-col cols="12" md="6" lg="12">
                    <v-text-field
                      v-model.number="editingRule.listenPort"
                      type="number"
                      min="1"
                      max="65535"
                      :label="reverseProxyCopy.listenPort"
                      hide-details />
                  </v-col>
                  <v-col v-if="!listenIsDNS" cols="12" md="6" lg="12">
                    <v-text-field
                      v-model.number="editingRule.maxConcurrentConnections"
                      type="number"
                      min="0"
                      max="1000000"
                      :label="reverseProxyCopy.maxConcurrentConnections"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.ruleResourceHint }}</div>
                  </v-col>
                  <v-col v-if="!listenIsDNS" cols="12" lg="12">
                    <v-text-field
                      v-model="editingRule.pathPrefix"
                      :label="reverseProxyCopy.pathPrefix"
                      placeholder="留空 / 或 /88999"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.pathPrefixStrictHint }}</div>
                  </v-col>
                  <v-col v-if="listenIsDNS && (editingRule.listenProtocol === 'dns_doh' || editingRule.listenProtocol === 'dns_doh3')" cols="12" lg="12">
                    <v-text-field
                      v-model="editingRule.listenDnsPath"
                      :label="reverseProxyCopy.listenDnsPath"
                      placeholder="/dns-query"
                      hide-details />
                  </v-col>
                  <template v-if="listenIsDNS">
                    <v-col cols="12" lg="12">
                      <div class="rp-panel__section-title">{{ reverseProxyCopy.dnsAccessTitle }}</div>
                      <v-text-field
                        v-model="editingRule.dnsAllowedCidrsText"
                        :label="reverseProxyCopy.dnsAllowedCidrs"
                        placeholder="192.0.2.0/24, 2001:db8::/32"
                        hide-details />
                      <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.dnsAllowedCidrsHint }}</div>
                    </v-col>
                    <v-col cols="12" md="6" lg="12">
                      <v-text-field
                        v-model.number="editingRule.dnsRateLimitQps"
                        type="number"
                        min="1"
                        max="10000"
                        :label="reverseProxyCopy.dnsRateLimitQps"
                        hide-details />
                    </v-col>
                    <v-col cols="12" md="6" lg="12">
                      <v-text-field
                        v-model.number="editingRule.dnsMaxConcurrentQueries"
                        type="number"
                        min="0"
                        max="4096"
                        :label="reverseProxyCopy.dnsMaxConcurrentQueries"
                        hide-details />
                      <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.ruleResourceHint }}</div>
                    </v-col>
                  </template>
                  <v-col v-if="listenIsDNS" cols="12" lg="12">
                    <v-switch
                      v-model="editingRule.ednsEnabled"
                      color="primary"
                      :label="reverseProxyCopy.ednsEnabled"
                      hide-details />
                  </v-col>
                  <v-col v-if="listenIsDNS && editingRule.ednsEnabled" cols="12" lg="12">
                    <v-select
                      v-model="editingRule.ednsMode"
                      :items="ednsModeItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.ednsMode"
                      hide-details />
                  </v-col>
                  <v-col v-if="listenIsDNS && editingRule.ednsEnabled && editingRule.ednsMode === 'custom'" cols="12" lg="12">
                    <v-text-field
                      v-model="editingRule.ednsCustomIp"
                      :label="reverseProxyCopy.ednsCustomIp"
                      placeholder="14.119.184.1"
                      @blur="normalizeCustomEDNSInput"
                      hide-details />
                  </v-col>
                  <v-col v-if="listenIsDNS && editingRule.ednsEnabled && editingRule.ednsMode === 'auto'" cols="12" lg="12">
                    <v-select
                      v-model="editingRule.ednsClientSubnetPolicy"
                      :items="ednsClientSubnetPolicyItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.ednsClientSubnetPolicy"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.ednsPolicyHint }}</div>
                  </v-col>
                  <v-col v-if="listenIsDNS" cols="12" lg="12">
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.ednsHint }}</div>
                  </v-col>
                  <v-col v-if="listenIsDNS" cols="12" lg="12">
                    <v-switch
                      v-model="editingRule.disableIpv4Answer"
                      color="primary"
                      :label="reverseProxyCopy.disableIpv4Answer"
                      hide-details />
                  </v-col>
                  <v-col v-if="listenIsDNS" cols="12" lg="12">
                    <v-switch
                      v-model="editingRule.disableIpv6Answer"
                      color="primary"
                      :label="reverseProxyCopy.disableIpv6Answer"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.dnsAnswerFilterHint }}</div>
                  </v-col>
                  <v-col v-if="!listenIsDNS" cols="12" lg="12">
                    <v-switch
                      v-model="editingRule.apiPassthrough"
                      color="primary"
                      :label="reverseProxyCopy.apiPassthrough"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.apiPassthroughHint }}</div>
                  </v-col>
                  <v-col v-if="listenCanAdvertiseHTTP3" cols="12" lg="12">
                    <v-switch
                      v-model="editingRule.advertiseHttp3"
                      color="primary"
                      :label="reverseProxyCopy.advertiseHttp3"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.advertiseHttp3Hint }}</div>
                  </v-col>
                </v-row>
              </div>
            </v-col>

            <v-col cols="12" lg="4">
              <div class="rp-panel rp-panel--target">
                <div class="rp-panel__title">{{ reverseProxyCopy.targetPanel }}</div>
                <div class="rp-panel__subtitle">{{ reverseProxyCopy.targetPanelHint }}</div>
                <v-row class="mt-1">
                  <v-col cols="12" md="6" lg="12">
                    <v-select
                      :model-value="editingRule.targetProtocol"
                      :items="protocolItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.targetProtocol"
                      @update:modelValue="changeTargetProtocol"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ targetProtocolBehavior }}</div>
                  </v-col>
                  <v-col cols="12" lg="12">
                    <v-text-field
                      v-model="editingRule.targetAddressesText"
                      :label="reverseProxyCopy.targetAddresses"
                      :placeholder="reverseProxyCopy.targetAddressesPlaceholder"
                      hide-details />
                  </v-col>
                  <v-col cols="12" md="6" lg="12">
                    <v-text-field
                      v-model.number="editingRule.targetPort"
                      type="number"
                      min="1"
                      max="65535"
                      :label="reverseProxyCopy.targetPort"
                      hide-details />
                  </v-col>
                  <v-col v-if="!targetIsDNS" cols="12" lg="12">
                    <v-text-field
                      v-model.number="editingRule.maxConcurrentRequests"
                      type="number"
                      min="0"
                      max="10000"
                      :label="reverseProxyCopy.maxConcurrentRequests"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.ruleResourceHint }}</div>
                  </v-col>
                  <v-col v-if="!targetIsDNS" cols="12" md="6" lg="12">
                    <v-text-field
                      v-model.number="editingRule.upstreamMaxConnections"
                      type="number"
                      min="0"
                      max="1000000"
                      :label="reverseProxyCopy.upstreamMaxConnections"
                      hide-details />
                  </v-col>
                  <v-col v-if="!targetIsDNS" cols="12" md="6" lg="12">
                    <v-text-field
                      v-model.number="editingRule.upstreamMaxIdleConnections"
                      type="number"
                      min="0"
                      max="1000000"
                      :label="reverseProxyCopy.upstreamMaxIdleConnections"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.ruleResourceHint }}</div>
                  </v-col>
                  <v-col cols="12" md="6" lg="12">
                    <v-text-field
                      v-model.number="editingRule.memoryLimitBytes"
                      type="number"
                      min="0"
                      :label="reverseProxyCopy.memoryLimitBytes"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.ruleResourceHint }}</div>
                  </v-col>
                  <v-col v-if="!targetIsDNS" cols="12" lg="12">
                    <v-text-field
                      v-model="editingRule.targetPath"
                      :label="reverseProxyCopy.targetPath"
                      placeholder="/image-001"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.targetPathRewriteHint }}</div>
                  </v-col>
                  <v-col v-if="targetIsDNS && (editingRule.targetProtocol === 'dns_doh' || editingRule.targetProtocol === 'dns_doh3')" cols="12" lg="12">
                    <v-text-field
                      v-model="editingRule.targetDnsPath"
                      :label="reverseProxyCopy.targetDnsPath"
                      placeholder="/dns-query"
                      hide-details />
                  </v-col>
                  <v-col v-if="targetIsDNS" cols="12" lg="12">
                    <v-text-field
                      v-model.number="editingRule.dnsUpstreamTimeoutSeconds"
                      type="number"
                      min="1"
                      max="120"
                      :label="reverseProxyCopy.dnsUpstreamTimeout"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.dnsUpstreamTimeoutHint }}</div>
                  </v-col>
                  <v-col v-if="targetIsDNS" cols="12" lg="12">
                    <v-textarea
                      v-model="editingRule.fallbackDnsUpstreams"
                      :label="reverseProxyCopy.fallbackDnsUpstreams"
                      placeholder="tls://1.1.1.1"
                      rows="3"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.fallbackDnsUpstreamsHint }}</div>
                  </v-col>
                  <v-col v-if="targetIsDNS" cols="12" lg="12">
                    <div class="rp-panel__section-title">{{ reverseProxyCopy.dnsCacheTitle }}</div>
                    <v-switch
                      v-model="editingRule.dnsCacheEnabled"
                      color="primary"
                      :label="reverseProxyCopy.dnsCacheEnabled"
                      hide-details />
                  </v-col>
                  <template v-if="targetIsDNS">
                    <v-col cols="12" lg="12">
                      <v-text-field
                        v-model.number="editingRule.dnsCacheSizeBytes"
                        type="number"
                        min="1"
                        :label="reverseProxyCopy.dnsCacheSizeBytes"
                        :disabled="!editingRule.dnsCacheEnabled"
                        hide-details />
                      <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.dnsCacheSizeBytesHint }}</div>
                    </v-col>
                    <v-col cols="12" lg="12">
                      <v-text-field
                        v-model.number="editingRule.dnsCacheMinTtl"
                        type="number"
                        min="0"
                        max="4294967295"
                        :label="reverseProxyCopy.dnsCacheMinTtl"
                        :disabled="!editingRule.dnsCacheEnabled"
                        hide-details />
                      <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.dnsCacheMinTtlHint }}</div>
                    </v-col>
                    <v-col cols="12" lg="12">
                      <v-text-field
                        v-model.number="editingRule.dnsCacheMaxTtl"
                        type="number"
                        min="0"
                        max="4294967295"
                        :label="reverseProxyCopy.dnsCacheMaxTtl"
                        :disabled="!editingRule.dnsCacheEnabled"
                        hide-details />
                      <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.dnsCacheMaxTtlHint }}</div>
                    </v-col>
                  </template>
                  <v-col cols="12" lg="12">
                    <v-select
                      v-model="editingRule.ipStrategy"
                      :items="ipStrategyItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.ipStrategy"
                      hide-details />
                  </v-col>
                  <v-col v-if="!targetIsDNS" cols="12" lg="12">
                    <v-select
                      v-model="editingRule.httpVersionStrategy"
                      :items="httpVersionItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.httpVersionStrategy"
                      :disabled="!targetVersionConfigurable"
                      hide-details />
                  </v-col>
                  <v-col cols="12" lg="12">
                    <v-switch
                      v-model="editingRule.upstreamTlsVerify"
                      color="primary"
                      :label="reverseProxyCopy.upstreamTlsVerify"
                      :disabled="!targetIsHTTPS"
                      hide-details />
                  </v-col>
                </v-row>
              </div>
            </v-col>

            <v-col cols="12" lg="4">
              <div class="rp-panel rp-panel--tls">
                <div class="rp-panel__title">{{ reverseProxyCopy.tlsPanel }}</div>
                <div class="rp-panel__subtitle">{{ reverseProxyCopy.tlsPanelHint }}</div>
                <v-row class="mt-1">
                  <v-col cols="12">
                    <v-select
                      v-model="editingRule.certificateRecordIds"
                      :items="overview.certificates"
                      item-title="mainDomain"
                      item-value="id"
                      :label="reverseProxyCopy.certificate"
                      :disabled="!listenIsHTTPS"
                      multiple
                      chips
                      clearable
                      hide-details>
                      <template #item="{ props: itemProps, item }">
                        <v-list-item
                          v-bind="itemProps"
                          :title="`${item.raw.displayId} / ${item.raw.mainDomain}`"
                          :subtitle="joinDisplay(item.raw.domains)" />
                      </template>
                      <template #selection="{ item, index }">
                        <span>
                          {{ item.raw.displayId }} / {{ item.raw.mainDomain }}<span v-if="index < selectedCertificates.length - 1">, </span>
                        </span>
                      </template>
                    </v-select>
                  </v-col>
                  <v-col cols="12">
                    <v-alert
                      :type="listenIsHTTPS ? 'info' : 'warning'"
                      variant="tonal"
                      density="comfortable">
                      <template v-if="listenIsHTTPS && selectedCertificates.length > 0">
                        {{ reverseProxyCopy.certificateBound }}: {{ selectedCertificates.map(item => `${item.displayId} / ${item.mainDomain}`).join(', ') }}
                      </template>
                      <template v-else-if="listenIsHTTPS">
                        {{ reverseProxyCopy.certificateRequired }}
                      </template>
                      <template v-else>
                        {{ reverseProxyCopy.currentHTTPNoCert }}
                      </template>
                    </v-alert>
                  </v-col>
                  <v-col
                    v-if="listenIsHTTPS && selectedCertificates.length > 0 && currentCertificateHints.length > 0"
                    cols="12">
                    <v-alert
                      type="warning"
                      variant="tonal"
                      density="comfortable">
                      {{ currentCertificateHints.join(', ') }}
                    </v-alert>
                  </v-col>
                  <v-col v-if="ipCertificateRoutingHint" cols="12">
                    <v-alert
                      type="info"
                      variant="tonal"
                      density="comfortable">
                      {{ ipCertificateRoutingHint }}
                    </v-alert>
                  </v-col>
                </v-row>
              </div>
            </v-col>
          </v-row>
        </v-card-text>

        <v-card-actions class="px-6 pb-5">
          <v-spacer />
          <v-btn variant="text" @click="dialogVisible = false">{{ reverseProxyCopy.cancel }}</v-btn>
          <v-btn color="primary" :loading="saving" :disabled="configurationConflict || actionsDisabled" @click="saveRule">{{ reverseProxyCopy.save }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="resourceDialogVisible" :fullscreen="smAndDown" scrollable max-width="980">
      <v-card :rounded="smAndDown ? '0' : 'xl'">
        <v-card-title class="rp-dialog-title">
          <div class="rp-dialog-title__top">
            <div class="text-subtitle-1 font-weight-medium">{{ reverseProxyCopy.resourceTitle }}</div>
          </div>
          <div class="rp-dialog-title__subtitle text-caption text-medium-emphasis">{{ reverseProxyCopy.resourceSubtitle }}</div>
        </v-card-title>
        <v-alert v-if="configurationConflict" type="warning" variant="tonal" density="comfortable" class="mx-6 mb-4">
          {{ reverseProxyCopy.revisionConflict }}
        </v-alert>
        <v-divider />
        <v-card-text class="pt-5 rp-dialog-body">
          <v-row>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.listenerConnectionLimit"
                type="number"
                min="0"
                max="1000000"
                :label="reverseProxyCopy.listenerConnectionLimit"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.globalHttpMaxConcurrent"
                type="number"
                min="0"
                max="1000000"
                :label="reverseProxyCopy.globalHttpMaxConcurrent"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.globalDnsMaxConcurrent"
                type="number"
                min="0"
                max="1000000"
                :label="reverseProxyCopy.globalDnsMaxConcurrent"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.http2MaxConcurrentStreams"
                type="number"
                min="1"
                max="65535"
                :label="reverseProxyCopy.http2MaxConcurrentStreams"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.quicMaxIncomingStreams"
                type="number"
                min="1"
                max="65535"
                :label="reverseProxyCopy.quicMaxIncomingStreams"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.defaultUpstreamMaxIdleConnections"
                type="number"
                min="0"
                max="1000000"
                :label="reverseProxyCopy.defaultUpstreamMaxIdleConnections"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.memoryPoolBytes"
                type="number"
                min="512000"
                :label="reverseProxyCopy.memoryPoolBytes"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.defaultRuleMemoryLimitBytes"
                type="number"
                min="512000"
                :label="reverseProxyCopy.defaultRuleMemoryLimitBytes"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.responseRewriteInputBytes"
                type="number"
                min="512000"
                :label="reverseProxyCopy.responseRewriteInputBytes"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.responseRewriteOutputBytes"
                type="number"
                min="512000"
                :label="reverseProxyCopy.responseRewriteOutputBytes"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model.number="editingResources.responseRewriteMaxConcurrent"
                type="number"
                min="1"
                max="1000000"
                :label="reverseProxyCopy.responseRewriteMaxConcurrent"
                hide-details />
            </v-col>
          </v-row>
          <v-alert type="info" variant="tonal" density="comfortable" class="mt-4">
            {{ reverseProxyCopy.resourceMemoryHint }}
          </v-alert>
        </v-card-text>
        <v-card-actions class="px-6 pb-5">
          <v-spacer />
          <v-btn variant="text" @click="resourceDialogVisible = false">{{ reverseProxyCopy.cancel }}</v-btn>
          <v-btn color="primary" :loading="savingResources" :disabled="configurationConflict || actionsDisabled" @click="saveResources">{{ reverseProxyCopy.resourceSave }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup lang="ts">
import { useDisplay } from 'vuetify'
import {
  certificateDisplay,
  connectionCountsDisplay,
  ednsClientSubnetPolicyItems,
  ednsModeItems,
  formatReverseProxyBytes,
  httpVersionItems,
  httpVersionStrategyLabel,
  ipStrategyLabel,
  ipStrategyItems,
  joinDisplay,
  listenMatchDisplay,
  protocolItems,
  protocolLabel,
  reverseProxyCopy,
  reverseProxyHeaders,
  runtimeStatusLabel,
  statusColor,
  useReverseProxyManage,
} from './SettingsReverseProxyManage.shared'

const props = withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})

const { smAndDown } = useDisplay()

const {
  loading,
  refreshing,
  saving,
  savingResources,
  mutationBusy,
  hasLoaded,
  loadError,
  actionsDisabled,
  dialogVisible,
  resourceDialogVisible,
  rowBusyId,
  searchText,
  overview,
  runtimeUsage,
  editingResources,
  configurationConflict,
  editingRule,
  filteredRules,
  lastSyncLabel,
  dialogTitle,
  selectedCertificates,
  currentCertificateHints,
  ipCertificateRoutingHint,
  targetIsHTTPS,
  listenIsHTTPS,
  listenIsDNS,
  listenIsPlainDNS,
  targetIsDNS,
  targetVersionConfigurable,
  listenCanAdvertiseHTTP3,
  hasPreviewProtocol,
  listenProtocolBehavior,
  targetProtocolBehavior,
  refreshOverview,
  openResourceDialog,
  saveResources,
  openRuleDialog,
  changeListenProtocol,
  changeTargetProtocol,
  normalizeCustomEDNSInput,
  saveRule,
  removeRule,
  toggleRule,
  moveRule,
} = useReverseProxyManage(props)
</script>

<style scoped>
.rp-page {
  margin-top: 20px;
}

.rp-hero {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(16, 94, 125, 0.18);
  background: linear-gradient(135deg, rgba(10, 32, 58, 0.96), rgba(18, 61, 102, 0.92));
  color: #eef6ff;
}

.rp-hero__bg {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(circle at 18% 26%, rgba(74, 170, 255, 0.2), transparent 32%),
    radial-gradient(circle at 80% 18%, rgba(72, 220, 184, 0.18), transparent 30%);
}

.rp-hero__content,
.rp-side,
.rp-table-card {
  position: relative;
}

.rp-hero__top,
.rp-table-card__toolbar,
.rp-resource-card__header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  flex-wrap: wrap;
}

.rp-hero__icon {
  width: 58px;
  height: 58px;
  border-radius: 18px;
  display: grid;
  place-items: center;
  background: rgba(255, 255, 255, 0.14);
  border: 1px solid rgba(255, 255, 255, 0.22);
}

.rp-hero__eyebrow {
  letter-spacing: 0.18em;
  color: rgba(221, 235, 255, 0.82);
}

.rp-hero__toolbar,
.rp-hero__chips {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.rp-hero__toolbar {
  justify-content: flex-end;
  gap: 10px;
}

.rp-hero__chips :deep(.v-chip) {
  font-weight: 700;
  letter-spacing: 0;
}

.rp-hero-chip {
  min-height: 28px;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.12);
}

.rp-hero-chip--sync {
  min-width: 138px;
  background: rgba(15, 118, 110, 0.56) !important;
  color: #ecfeff !important;
}

.rp-hero-chip--count {
  min-width: 84px;
  justify-content: center;
  background: rgba(37, 99, 235, 0.54) !important;
  color: #eff6ff !important;
}

.rp-hero-chip--sync :deep(.v-chip__content),
.rp-hero-chip--count :deep(.v-chip__content) {
  color: inherit !important;
}

.rp-hero-action {
  min-width: 112px;
  min-height: 38px;
  border: 1px solid rgba(94, 234, 212, 0.52) !important;
  border-radius: 11px;
  color: #f8fafc !important;
  background: rgba(15, 78, 75, 0.24) !important;
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.05),
    0 10px 24px rgba(8, 47, 73, 0.14);
  backdrop-filter: blur(8px);
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    box-shadow 0.16s ease,
    transform 0.16s ease;
}

.rp-hero-action:hover {
  border-color: rgba(153, 246, 228, 0.86) !important;
  background: rgba(20, 184, 166, 0.2) !important;
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.08),
    0 12px 28px rgba(20, 184, 166, 0.18);
  transform: translateY(-1px);
}

.rp-hero-action:focus-visible {
  outline: 2px solid rgba(204, 251, 241, 0.72);
  outline-offset: 2px;
}

.rp-hero-action :deep(.v-btn__content) {
  min-width: 0;
  color: inherit !important;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 0;
  white-space: nowrap;
}

.rp-hero-action :deep(.v-btn__prepend) {
  margin-inline-end: 8px;
}

.rp-hero-action :deep(.v-icon) {
  color: #ecfeff !important;
  opacity: 1;
}

.rp-hero-action.v-btn--disabled {
  opacity: 1 !important;
  border-color: var(--kwor-disabled-button-border) !important;
  color: var(--kwor-disabled-button-foreground) !important;
  background: var(--kwor-disabled-button-background) !important;
  box-shadow: none;
}

.rp-hero-action.v-btn--disabled :deep(.v-btn__content),
.rp-hero-action.v-btn--disabled :deep(.v-icon) {
  opacity: 1;
}

.rp-metric,
.rp-side__row,
.rp-panel {
  border: 1px solid rgba(148, 163, 184, 0.14);
  border-radius: 18px;
  color: rgba(226, 232, 240, 0.94);
  background: rgba(15, 23, 42, 0.46);
}

.rp-muted-label {
  color: rgba(148, 163, 184, 0.92);
}

.rp-side__row span {
  color: rgba(191, 219, 254, 0.88);
}

.rp-side__row strong:not(.text-success):not(.text-warning) {
  color: rgba(226, 232, 240, 0.96);
}

.rp-metric {
  padding: 14px;
  min-height: 108px;
}

.rp-resource-card {
  overflow: hidden;
}

.rp-resource-metric {
  min-width: 0;
  min-height: 86px;
  padding: 13px 14px;
  display: grid;
  align-content: space-between;
  gap: 6px;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.28);
}

.rp-resource-metric span,
.rp-resource-metric small {
  overflow-wrap: anywhere;
  color: rgba(100, 116, 139, 0.96);
  font-size: 12px;
}

.rp-resource-metric strong {
  min-width: 0;
  overflow-wrap: anywhere;
  font-size: 15px;
}

.rp-side__row {
  padding: 12px 14px;
  display: flex;
  justify-content: space-between;
  margin-bottom: 10px;
}

.rp-empty {
  min-height: 160px;
  display: grid;
  place-items: center;
  color: rgba(60, 72, 80, 0.8);
}

.rp-mobile-list {
  display: grid;
  gap: 12px;
}

.rp-mobile-rule {
  overflow: hidden;
}

.rp-mobile-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.rp-mobile-grid > div {
  min-width: 0;
  display: grid;
  gap: 3px;
}

.rp-mobile-grid span,
.rp-mobile-grid small {
  color: rgba(148, 163, 184, 0.92);
  font-size: 12px;
}

.rp-mobile-grid strong,
.rp-mobile-grid small,
.rp-wrap {
  overflow-wrap: anywhere;
}

.rp-dialog-body {
  overflow-y: auto;
}

.rp-dialog-title {
  display: grid;
  gap: 4px;
  white-space: normal;
}

.rp-dialog-title__top {
  min-width: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.rp-dialog-title__top :deep(.v-switch) {
  flex: 0 0 auto;
}

.rp-dialog-title__subtitle {
  min-width: 0;
  line-height: 1.45;
  white-space: normal;
  overflow-wrap: anywhere;
}

.rp-table :deep(.v-table__wrapper) {
  overflow-x: auto;
}

.rp-table :deep(table) {
  min-width: 1680px;
}

.rp-table :deep(th:last-child),
.rp-table :deep(td:last-child) {
  position: sticky;
  right: 0;
  min-width: 250px;
  background: rgb(var(--v-theme-surface));
  box-shadow: -10px 0 14px rgba(15, 23, 42, 0.08);
  z-index: 2;
}

.rp-table :deep(th:last-child) {
  z-index: 3;
}

.rp-panel {
  padding: 18px;
  min-height: 100%;
}

.rp-panel--target {
  background: rgba(17, 25, 44, 0.5);
}

.rp-panel--tls {
  background: rgba(19, 33, 47, 0.5);
}

.rp-panel__title {
  font-size: 15px;
  font-weight: 600;
}

.rp-panel__subtitle {
	margin-top: 6px;
	font-size: 12px;
	color: rgba(148, 163, 184, 0.9);
}

.rp-panel__section-title {
	margin-bottom: 8px;
	font-size: 13px;
	font-weight: 600;
	color: rgba(226, 232, 240, 0.94);
}

.rp-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 14px;
  min-width: 230px;
  white-space: nowrap;
}

.rp-actions__switch {
  flex: 0 0 auto;
}

.rp-actions__switch :deep(.v-selection-control) {
  min-height: 32px;
}

.rp-actions__buttons {
  display: flex;
  align-items: center;
  gap: 6px;
  padding-left: 10px;
  border-left: 1px solid rgba(148, 163, 184, 0.22);
}

.rp-action-btn {
  flex: 0 0 auto;
}

@media (max-width: 959px) {
  .rp-hero__top,
  .rp-table-card__toolbar,
  .rp-resource-card__header {
    flex-direction: column;
  }

  .rp-resource-card__header > .v-btn {
    width: 100%;
  }

  .rp-hero__toolbar {
    width: 100%;
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .rp-hero-action {
    width: 100%;
    min-width: 0;
  }

  .rp-hero-action :deep(.v-btn__content) {
    overflow: hidden;
    text-overflow: ellipsis;
  }
}

@media (max-width: 599px) {
  .rp-page {
    margin-top: 12px;
  }

  .rp-mobile-grid {
    grid-template-columns: 1fr;
  }

  .rp-panel {
    padding: 14px;
  }
}
</style>
