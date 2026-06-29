<template>
  <section class="pf-page">
    <v-row class="mt-1">
      <v-col cols="12" xl="8">
        <v-card class="pf-hero" rounded="xl" :loading="loading && !overview.available">
          <div class="pf-hero__bg"></div>
          <v-card-text class="pf-hero__content">
            <div class="pf-hero__top">
              <div class="d-flex align-center ga-3">
                <div class="pf-hero__icon">
                  <v-icon size="30">mdi-swap-horizontal-bold</v-icon>
                </div>
                <div>
                  <div class="text-overline pf-hero__eyebrow">{{ copy.heroEyebrow }}</div>
                  <div class="text-h5 font-weight-bold">{{ copy.title }}</div>
                  <div class="text-body-2 text-medium-emphasis mt-1">{{ copy.subtitle }}</div>
                </div>
              </div>
              <div class="pf-hero__toolbar">
                <v-btn
                  variant="tonal"
                  color="info"
                  prepend-icon="mdi-refresh"
                  :loading="refreshing"
                  @click="refreshOverview">
                  {{ copy.refresh }}
                </v-btn>
                <v-btn
                  color="primary"
                  prepend-icon="mdi-plus"
                  @click="openRuleDialog()">
                  {{ copy.newRule }}
                </v-btn>
              </div>
            </div>

            <div class="pf-hero__chips">
              <v-chip size="small" :color="overview.available ? 'success' : 'warning'" variant="flat">
                {{ overview.available ? copy.available : copy.unavailable }}
              </v-chip>
              <v-chip size="small" color="secondary" variant="flat" class="pf-hero-chip pf-hero-chip--sync">
                {{ copy.lastSync }}: {{ lastSyncLabel }}
              </v-chip>
              <v-chip size="small" color="primary" variant="flat" class="pf-hero-chip pf-hero-chip--count">
                {{ copy.ruleCount }} {{ overview.rules.length }}
              </v-chip>
            </div>

            <v-row class="mt-2">
              <v-col cols="12" sm="6" md="3">
                <div class="pf-metric">
                  <div class="text-caption pf-muted-label">{{ copy.enabledRules }}</div>
                  <div class="text-h5 mt-1">{{ overview.enabledCount }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="pf-metric">
                  <div class="text-caption pf-muted-label">{{ copy.limitedRules }}</div>
                  <div class="text-h5 mt-1">{{ overview.limitedCount }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="pf-metric">
                  <div class="text-caption pf-muted-label">{{ copy.totalTraffic }}</div>
                  <div class="text-h5 mt-1">{{ formatBytes(overview.totalTraffic) }}</div>
                </div>
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" xl="4">
        <v-card rounded="xl" variant="outlined" class="pf-side">
          <v-card-title class="text-subtitle-1 font-weight-medium">{{ copy.runtimeTitle }}</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="pf-side__row">
              <span>{{ copy.kernelIPv4 }}</span>
              <strong :class="overview.kernelIPv4Forward ? 'text-success' : 'text-warning'">
                {{ overview.kernelIPv4Forward ? copy.forwardOn : copy.forwardOff }}
              </strong>
            </div>
            <div class="pf-side__row">
              <span>{{ copy.kernelIPv6 }}</span>
              <strong :class="overview.kernelIPv6Forward ? 'text-success' : 'text-warning'">
                {{ overview.kernelIPv6Forward ? copy.forwardOn : copy.forwardOff }}
              </strong>
            </div>
            <div class="pf-side__row">
              <span>{{ copy.totalUpload }}</span>
              <strong>{{ formatBytes(overview.totalUp) }}</strong>
            </div>
            <div class="pf-side__row">
              <span>{{ copy.totalDownload }}</span>
              <strong>{{ formatBytes(overview.totalDown) }}</strong>
            </div>
            <v-alert type="info" variant="tonal" density="comfortable" class="mt-4">
              {{ copy.runtimeHint }}
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
    <v-alert
      v-if="!overview.available"
      type="info"
      variant="tonal"
      density="comfortable"
      class="mb-4">
      {{ copy.unavailableHint }}
    </v-alert>

    <v-card rounded="xl" variant="outlined" class="pf-table-card">
      <v-card-title class="pf-table-card__toolbar">
        <div>
          <div class="text-subtitle-1 font-weight-medium">{{ copy.tableTitle }}</div>
          <div class="text-caption text-medium-emphasis mt-1">{{ copy.tableSubtitle }}</div>
        </div>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <v-row class="mb-2">
          <v-col cols="12" md="5">
            <v-text-field
              v-model="searchText"
              :label="copy.searchLabel"
              prepend-inner-icon="mdi-magnify"
              clearable
              hide-details />
          </v-col>
          <v-col cols="12" md="3">
            <v-select
              v-model="familyFilter"
              :items="familyFilterItems"
              :label="copy.familyFilter"
              hide-details />
          </v-col>
          <v-col cols="12" md="4">
            <v-select
              v-model="protocolFilter"
              :items="protocolFilterItems"
              :label="copy.protocolFilter"
              hide-details />
          </v-col>
        </v-row>

        <v-data-table
          :headers="headers"
          :items="filteredRules"
          item-value="id"
          fixed-header
          class="rounded-lg pf-table"
          hide-no-data>
          <template #item.name="{ item }">
            <div class="py-2">
              <div class="d-flex align-center ga-2 flex-wrap">
                <div class="font-weight-medium">{{ item.name || copy.ruleFallback }}</div>
                <v-chip size="x-small" :color="item.enabled ? 'success' : 'grey'" variant="flat">
                  {{ item.enabled ? copy.enabled : copy.disabled }}
                </v-chip>
              </div>
              <div class="text-caption text-medium-emphasis" v-if="item.description">{{ item.description }}</div>
            </div>
          </template>

          <template #item.local="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ item.localPortSpec || '-' }}</div>
              <div class="text-caption text-medium-emphasis">{{ localModeLabel(item.localPortMode) }}</div>
            </div>
          </template>

          <template #item.target="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ targetDisplayLabel(item.targetIP, item.targetPort) }}</div>
            </div>
          </template>

          <template #item.lane="{ item }">
            <div class="py-2 d-flex flex-wrap ga-2">
              <v-chip size="small" variant="flat" class="pf-protocol-chip">{{ protocolLabel(item.protocol) }}</v-chip>
              <v-chip size="small" color="info" variant="outlined">{{ familyLabel(item.family) }}</v-chip>
            </div>
          </template>

          <template #item.limit="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ rateLimitLabel(item.effectiveRateLimitMbps, item.rateLimitMbps, item.limitStatus) }}</div>
              <div v-if="item.limitStatus === 'degraded' && item.limitWarning" class="text-caption text-warning">
                {{ copy.limitDegraded }}：{{ item.limitWarning }}
              </div>
              <div v-else class="text-caption text-medium-emphasis">{{ copy.leftPortOnly }}</div>
            </div>
          </template>

          <template #item.traffic="{ item }">
            <div class="py-2">
              <div class="font-weight-medium">{{ formatBytes(item.currentTotal) }}</div>
              <div class="text-caption text-medium-emphasis">
                {{ copy.up }} {{ formatBytes(item.currentUp) }} / {{ copy.down }} {{ formatBytes(item.currentDown) }}
              </div>
            </div>
          </template>

          <template #item.actions="{ item }">
            <div class="d-flex align-center justify-end ga-3">
              <v-switch
                density="compact"
                color="success"
                hide-details
                inset
                :model-value="item.enabled"
                :loading="rowBusyId === item.id"
                :disabled="rowBusyId === item.id"
                @update:modelValue="(value) => toggleRule(item, Boolean(value))" />
              <v-icon
                size="18"
                color="primary"
                :class="{ 'pf-action--disabled': rowBusyId === item.id }"
                @click="rowBusyId !== item.id && openRuleDialog(item)">
                mdi-pencil
              </v-icon>
              <v-icon
                size="18"
                color="error"
                :class="{ 'pf-action--disabled': rowBusyId === item.id }"
                @click="rowBusyId !== item.id && removeRule(item)">
                mdi-delete
              </v-icon>
            </div>
          </template>
        </v-data-table>

        <div v-if="filteredRules.length === 0" class="pf-empty">
          <v-icon size="36" color="grey">mdi-swap-horizontal-off</v-icon>
          <div class="text-subtitle-2 mt-2">{{ copy.emptyText }}</div>
        </div>
      </v-card-text>
    </v-card>

    <v-dialog v-model="dialogVisible" max-width="1040">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <div>
            <div class="text-subtitle-1 font-weight-medium">{{ dialogTitle }}</div>
            <div class="text-caption text-medium-emphasis mt-1">{{ copy.dialogSubtitle }}</div>
          </div>
          <v-switch
            v-model="editingRule.enabled"
            color="success"
            hide-details
            inset
            :label="copy.enabled" />
        </v-card-title>
        <v-divider />

        <v-card-text class="pt-5">
          <v-row>
            <v-col cols="12" md="4">
              <v-text-field
                v-model="editingRule.name"
                :label="copy.nameLabel"
                hide-details />
            </v-col>
            <v-col cols="12" md="8">
              <v-text-field
                v-model="editingRule.description"
                :label="copy.descLabel"
                hide-details />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12" lg="6">
              <div class="pf-panel">
                <div class="pf-panel__title">{{ copy.localPanelTitle }}</div>
                <div class="pf-panel__subtitle">{{ copy.localPanelHint }}</div>
                <v-row class="mt-1">
                  <v-col cols="12" md="6">
                    <v-select
                      v-model="editingRule.protocol"
                      :items="protocolItems"
                      :label="copy.protocolLabel"
                      hide-details />
                  </v-col>
                  <v-col cols="12" md="6">
                    <v-select
                      v-model="editingRule.family"
                      :items="familyItems"
                      :label="copy.familyLabel"
                      hide-details />
                  </v-col>
                  <v-col cols="12">
                    <v-select
                      v-model="editingRule.localPortMode"
                      :items="localModeItems"
                      :label="copy.modeLabel"
                      hide-details />
                  </v-col>
                  <v-col cols="12" sm="6" v-if="editingRule.localPortMode !== 'multi'">
                    <v-text-field
                      v-model.number="editingRule.localPortStart"
                      type="number"
                      min="1"
                      max="65535"
                      :label="localStartLabel"
                      hide-details />
                  </v-col>
                  <v-col cols="12" v-if="editingRule.localPortMode === 'multi'">
                    <v-text-field
                      v-model="editingRule.localPortSpec"
                      :label="copy.multiLabel"
                      :placeholder="copy.multiPlaceholder"
                      hide-details />
                  </v-col>
                  <v-col cols="12" sm="6" v-if="editingRule.localPortMode === 'range'">
                    <v-text-field
                      v-model.number="editingRule.localPortEnd"
                      type="number"
                      min="1"
                      max="65535"
                      :label="copy.rangeEndLabel"
                      hide-details />
                  </v-col>
                  <v-col cols="12" md="6">
                    <v-text-field
                      v-model.number="editingRule.rateLimitMbps"
                      type="number"
                      min="0"
                      :label="copy.rateLabel"
                      hide-details />
                  </v-col>
                </v-row>
                <div class="text-caption text-medium-emphasis mt-4">{{ copy.rateHint }}</div>
                <v-alert type="info" variant="tonal" density="comfortable" class="mt-4">
                  {{ localPreviewText }}
                </v-alert>
              </div>
            </v-col>

            <v-col cols="12" lg="6">
              <div class="pf-panel pf-panel--target">
                <div class="pf-panel__title">{{ copy.targetPanelTitle }}</div>
                <div class="pf-panel__subtitle">{{ copy.targetPanelHint }}</div>
                <v-row class="mt-1">
                  <v-col cols="12" md="8">
                    <v-text-field
                      v-model="editingRule.targetIP"
                      :label="copy.targetIPLabel"
                      hide-details />
                  </v-col>
                  <v-col cols="12" md="4">
                    <v-text-field
                      v-model.number="editingRule.targetPort"
                      type="number"
                      min="1"
                      max="65535"
                      :label="copy.targetPortLabel"
                      hide-details />
                  </v-col>
                </v-row>
              </div>
            </v-col>
          </v-row>
        </v-card-text>

        <v-card-actions class="px-6 pb-5">
          <v-spacer />
          <v-btn variant="text" @click="dialogVisible = false">{{ copy.cancel }}</v-btn>
          <v-btn color="primary" :loading="savingRule" @click="saveRule">{{ copy.save }}</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup lang="ts">
import {
  copy,
  familyFilterItems,
  familyItems,
  protocolFilterItems,
  protocolItems,
  localModeItems,
  headers,
  familyLabel,
  formatBytes,
  localModeLabel,
  protocolLabel,
  rateLimitLabel,
  targetDisplayLabel,
  usePortForwardManage,
} from './SettingsPortForwardManage.shared'

const props = withDefaults(defineProps<{
  active?: boolean
}>(), {
  active: false,
})

const {
  loading,
  refreshing,
  savingRule,
  dialogVisible,
  rowBusyId,
  searchText,
  familyFilter,
  protocolFilter,
  overview,
  editingRule,
  lastSyncLabel,
  dialogTitle,
  localStartLabel,
  localPreviewText,
  filteredRules,
  refreshOverview,
  openRuleDialog,
  saveRule,
  toggleRule,
  removeRule,
} = usePortForwardManage(props)
</script>

<style scoped>
.pf-page {
  margin-top: 20px;
}

.pf-hero {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(18, 120, 132, 0.18);
  background: linear-gradient(135deg, rgba(8, 50, 58, 0.96), rgba(18, 85, 88, 0.92));
  color: #eef8f8;
}

.pf-hero__bg {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(circle at 18% 24%, rgba(86, 214, 190, 0.22), transparent 34%),
    radial-gradient(circle at 78% 18%, rgba(247, 190, 78, 0.18), transparent 28%);
}

.pf-hero__content,
.pf-side,
.pf-table-card {
  position: relative;
}

.pf-hero__top,
.pf-table-card__toolbar {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  align-items: flex-start;
  flex-wrap: wrap;
}

.pf-hero__icon {
  width: 58px;
  height: 58px;
  border-radius: 18px;
  display: grid;
  place-items: center;
  background: rgba(255, 255, 255, 0.14);
  border: 1px solid rgba(255, 255, 255, 0.22);
}

.pf-hero__eyebrow {
  letter-spacing: 0.18em;
  color: rgba(220, 250, 246, 0.78);
}

.pf-hero__toolbar,
.pf-hero__chips {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.pf-hero__chips :deep(.v-chip) {
  font-weight: 600;
  letter-spacing: 0.02em;
}

.pf-hero-chip {
  min-height: 28px;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
}

.pf-hero-chip--sync {
  min-width: 138px;
  background: rgba(20, 184, 166, 0.24) !important;
  color: #ecfeff !important;
}

.pf-hero-chip--count {
  min-width: 84px;
  justify-content: center;
  background: rgba(59, 130, 246, 0.34) !important;
  color: #eff6ff !important;
}

.pf-hero-chip--sync :deep(.v-chip__content),
.pf-hero-chip--count :deep(.v-chip__content) {
  color: inherit !important;
}

.pf-metric,
.pf-side__row,
.pf-panel {
  border: 1px solid rgba(148, 163, 184, 0.14);
  border-radius: 18px;
  color: rgba(226, 232, 240, 0.94);
  background: rgba(15, 23, 42, 0.46);
}

.pf-muted-label {
  color: rgba(148, 163, 184, 0.92);
}

.pf-side__row span {
  color: rgba(191, 219, 254, 0.88);
}

.pf-side__row strong:not(.text-success):not(.text-warning) {
  color: rgba(226, 232, 240, 0.96);
}

.pf-metric {
  padding: 14px;
  min-height: 108px;
}

.pf-side__row {
  padding: 12px 14px;
  display: flex;
  justify-content: space-between;
  margin-bottom: 10px;
}

.pf-table {
  overflow: hidden;
}

.pf-empty {
  min-height: 160px;
  display: grid;
  place-items: center;
  color: rgba(60, 72, 80, 0.8);
}

.pf-panel {
  padding: 18px;
  min-height: 100%;
}

.pf-panel--target {
  background: rgba(17, 25, 44, 0.5);
}

.pf-panel__title {
  font-size: 15px;
  font-weight: 600;
}

.pf-panel__subtitle {
  margin-top: 6px;
  font-size: 12px;
  color: rgba(148, 163, 184, 0.9);
}

.pf-action--disabled {
  opacity: 0.4;
  pointer-events: none;
}

.pf-protocol-chip {
  background: #f3e3a1 !important;
  color: #5f4a00 !important;
  border: 1px solid rgba(255, 245, 204, 0.5);
}

@media (max-width: 959px) {
  .pf-hero__top,
  .pf-table-card__toolbar {
    flex-direction: column;
  }
}
</style>
