<template>
  <section class="rp-page">
    <v-row class="mt-1">
      <v-col cols="12" xl="8">
        <v-card class="rp-hero" rounded="xl" :loading="loading && !overview.available">
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
                  variant="tonal"
                  color="info"
                  prepend-icon="mdi-refresh"
                  :loading="refreshing"
                  @click="refreshOverview">
                  {{ reverseProxyCopy.refresh }}
                </v-btn>
                <v-btn
                  color="primary"
                  prepend-icon="mdi-plus"
                  @click="openRuleDialog()">
                  {{ reverseProxyCopy.newRule }}
                </v-btn>
              </div>
            </div>

            <div class="rp-hero__chips">
              <v-chip size="small" :color="overview.available ? 'success' : 'warning'" variant="flat">
                {{ overview.available ? reverseProxyCopy.available : reverseProxyCopy.unavailable }}
              </v-chip>
              <v-chip size="small" color="secondary" variant="tonal">
                {{ reverseProxyCopy.lastSync }}: {{ lastSyncLabel }}
              </v-chip>
              <v-chip size="small" color="primary" variant="tonal">
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

    <v-alert
      v-if="!overview.available"
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
                {{ item.runtimeStatus || 'idle' }}
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
              <div class="font-weight-medium">{{ item.pathPrefix || '全部路径' }}</div>
            </div>
          </template>

          <template #item.target="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ protocolLabel(item.targetProtocol) }} -> {{ joinDisplay(item.targetAddresses) }}:{{ item.targetPort }}</div>
              <div class="text-caption text-medium-emphasis mt-1">{{ item.targetPath || '/' }}</div>
            </div>
          </template>

          <template #item.strategy="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ item.ipStrategy }}</div>
              <div class="text-caption text-medium-emphasis mt-1">
                {{ item.targetProtocol === 'http' ? reverseProxyCopy.targetHTTPMode : item.httpVersionStrategy || '-' }}
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
                :disabled="rowBusyId === item.id"
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
                  :disabled="rowBusyId === item.id"
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
                  :disabled="rowBusyId === item.id"
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
                  :disabled="rowBusyId === item.id"
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
                  :disabled="rowBusyId === item.id"
                  @click.stop="removeRule(item)" />
              </div>
            </div>
          </template>
        </v-data-table>

        <div v-if="filteredRules.length === 0" class="rp-empty">
          <v-icon size="36" color="grey">mdi-lan-disconnect</v-icon>
          <div class="text-subtitle-2 mt-2">{{ reverseProxyCopy.empty }}</div>
        </div>
      </v-card-text>
    </v-card>

    <v-dialog v-model="dialogVisible" max-width="1080">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <div>
            <div class="text-subtitle-1 font-weight-medium">{{ dialogTitle }}</div>
            <div class="text-caption text-medium-emphasis mt-1">{{ reverseProxyCopy.dialogSubtitle }}</div>
          </div>
          <v-switch
            v-model="editingRule.enabled"
            color="success"
            hide-details
            inset
            :label="reverseProxyCopy.enableLabel" />
        </v-card-title>
        <v-divider />

        <v-card-text class="pt-5">
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
                      v-model="editingRule.listenProtocol"
                      :items="protocolItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.listenProtocol"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ listenProtocolBehavior }}</div>
                  </v-col>
                  <v-col cols="12" md="6" lg="12">
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
                  <v-col cols="12" lg="12">
                    <v-text-field
                      v-model="editingRule.pathPrefix"
                      :label="reverseProxyCopy.pathPrefix"
                      placeholder="留空 / 或 /88999"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.pathPrefixStrictHint }}</div>
                  </v-col>
                  <v-col cols="12" lg="12">
                    <v-switch
                      v-model="editingRule.apiPassthrough"
                      color="primary"
                      :label="reverseProxyCopy.apiPassthrough"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.apiPassthroughHint }}</div>
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
                      v-model="editingRule.targetProtocol"
                      :items="protocolItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.targetProtocol"
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
                  <v-col cols="12" lg="12">
                    <v-text-field
                      v-model="editingRule.targetPath"
                      :label="reverseProxyCopy.targetPath"
                      placeholder="/image-001"
                      hide-details />
                    <div class="text-caption text-medium-emphasis mt-2">{{ reverseProxyCopy.targetPathRewriteHint }}</div>
                  </v-col>
                  <v-col cols="12" lg="12">
                    <v-select
                      v-model="editingRule.ipStrategy"
                      :items="ipStrategyItems"
                      item-title="title"
                      item-value="value"
                      :label="reverseProxyCopy.ipStrategy"
                      hide-details />
                  </v-col>
                  <v-col cols="12" lg="12">
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
                </v-row>
              </div>
            </v-col>
          </v-row>
        </v-card-text>

        <v-card-actions class="px-6 pb-5">
          <v-spacer />
          <v-btn variant="text" @click="dialogVisible = false">{{ reverseProxyCopy.cancel }}</v-btn>
          <v-btn color="primary" :loading="saving" @click="saveRule">{{ reverseProxyCopy.save }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup lang="ts">
import {
  certificateDisplay,
  connectionCountsDisplay,
  httpVersionItems,
  ipStrategyItems,
  joinDisplay,
  listenMatchDisplay,
  protocolItems,
  protocolLabel,
  reverseProxyCopy,
  reverseProxyHeaders,
  statusColor,
  useReverseProxyManage,
} from './SettingsReverseProxyManage.shared'

const props = withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})

const {
  loading,
  refreshing,
  saving,
  dialogVisible,
  rowBusyId,
  searchText,
  overview,
  editingRule,
  filteredRules,
  lastSyncLabel,
  dialogTitle,
  selectedCertificates,
  currentCertificateHints,
  targetIsHTTPS,
  listenIsHTTPS,
  targetVersionConfigurable,
  hasPreviewProtocol,
  listenProtocolBehavior,
  targetProtocolBehavior,
  refreshOverview,
  openRuleDialog,
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
.rp-table-card__toolbar {
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
  .rp-table-card__toolbar {
    flex-direction: column;
  }
}
</style>
