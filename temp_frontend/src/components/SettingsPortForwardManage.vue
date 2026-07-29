<template>
  <section class="pf-page">
    <v-row class="mt-1">
      <v-col cols="12" xl="8">
        <v-card class="pf-hero" rounded="xl" :loading="loading && !hasLoaded">
          <div class="pf-hero__bg"></div>
          <v-card-text class="pf-hero__content">
            <div class="pf-hero__top">
              <div class="d-flex align-center ga-3">
                <div class="pf-hero__icon"><v-icon size="30">mdi-swap-horizontal-bold</v-icon></div>
                <div>
                  <div class="text-overline pf-hero__eyebrow">{{ t('heroEyebrow') }}</div>
                  <div class="text-h5 font-weight-bold">{{ t('title') }}</div>
                  <div class="text-body-2 text-medium-emphasis mt-1">{{ t('subtitle') }}</div>
                </div>
              </div>
              <div class="pf-hero__toolbar">
                <v-btn class="pf-hero-action" variant="outlined" prepend-icon="mdi-refresh" :loading="refreshing" :disabled="mutationBusy" @click="refreshOverview">
                  {{ t('refresh') }}
                </v-btn>
                <v-btn class="pf-hero-action" variant="outlined" prepend-icon="mdi-plus" :disabled="mutationBusy" @click="openRuleDialog()">
                  {{ t('newRule') }}
                </v-btn>
              </div>
            </div>

            <div class="pf-hero__chips">
              <v-chip size="small" :color="overview.available ? 'success' : hasLoaded ? 'warning' : 'info'" variant="flat">
                {{ !hasLoaded ? t('unknown') : overview.available ? t('available') : t('unavailable') }}
              </v-chip>
              <v-chip size="small" color="secondary" variant="flat" class="pf-hero-chip pf-hero-chip--sync">
                {{ t('lastSync') }}: {{ lastSyncLabel }}
              </v-chip>
              <v-chip size="small" color="primary" variant="flat" class="pf-hero-chip pf-hero-chip--count">
                {{ t('ruleCount') }} {{ overview.rules.length }}
              </v-chip>
              <v-chip
                v-if="hasLoaded"
                size="small"
                :color="capabilityChipColor"
                variant="flat"
                class="pf-hero-chip pf-hero-chip--capability">
                {{ capabilityLabel }}
              </v-chip>
            </div>

            <v-row class="mt-2">
              <v-col cols="12" sm="6" md="3">
                <div class="pf-metric"><div class="text-caption pf-muted-label">{{ t('enabledRules') }}</div><div class="text-h5 mt-1">{{ overview.enabledCount }}</div></div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="pf-metric"><div class="text-caption pf-muted-label">{{ t('limitedRules') }}</div><div class="text-h5 mt-1">{{ overview.limitedCount }}</div></div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="pf-metric"><div class="text-caption pf-muted-label">{{ t('totalTraffic') }}</div><div class="text-h5 mt-1">{{ formatTrafficGB(overview.totalTraffic) }}</div></div>
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12" xl="4">
        <v-card rounded="xl" variant="outlined" class="pf-side">
          <v-card-title class="text-subtitle-1 font-weight-medium">{{ t('runtimeTitle') }}</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="pf-side__row"><span>{{ t('supported') }}</span><strong :class="hasLoaded && overview.supported ? 'text-success' : 'text-warning'">{{ !hasLoaded ? t('unknown') : overview.supported ? t('yes') : t('no') }}</strong></div>
            <div class="pf-side__row"><span>{{ t('ready') }}</span><strong :class="hasLoaded && overview.ready ? 'text-success' : 'text-warning'">{{ !hasLoaded ? t('unknown') : overview.ready ? t('yes') : t('no') }}</strong></div>
            <div class="pf-side__row"><span>{{ t('nftVersionLabel') }}</span><strong>{{ overview.nftVersion || t('unknown') }}</strong></div>
            <div class="pf-side__row"><span>{{ t('kernelVersionLabel') }}</span><strong>{{ overview.kernelVersion || t('unknown') }}</strong></div>
            <div class="pf-side__row"><span>{{ t('compatibilityModeLabel') }}</span><strong :class="overview.layoutPending ? 'text-warning' : ''">{{ compatibilityModeLabel }}</strong></div>
            <div class="pf-side__row"><span>{{ t('kernelIPv4') }}</span><strong :class="hasLoaded && overview.kernelIPv4Forward ? 'text-success' : 'text-warning'">{{ !hasLoaded ? t('unknown') : overview.kernelIPv4Forward ? t('forwardOn') : t('forwardOff') }}</strong></div>
            <div class="pf-side__row"><span>{{ t('kernelIPv6') }}</span><strong :class="hasLoaded && overview.kernelIPv6Forward ? 'text-success' : 'text-warning'">{{ !hasLoaded ? t('unknown') : overview.kernelIPv6Forward ? t('forwardOn') : t('forwardOff') }}</strong></div>
            <div class="pf-side__row"><span>{{ t('totalTraffic') }}</span><strong>{{ formatTrafficGB(overview.totalTraffic) }}</strong></div>
            <div class="pf-side__row"><span>{{ t('totalUpload') }}</span><strong>{{ formatTrafficGB(overview.totalUp) }}</strong></div>
            <div class="pf-side__row"><span>{{ t('totalDownload') }}</span><strong>{{ formatTrafficGB(overview.totalDown) }}</strong></div>
            <v-btn block variant="tonal" color="warning" prepend-icon="mdi-restart" :loading="overviewResetBusy" :disabled="mutationBusy" @click="resetOverviewTraffic">{{ t('resetOverviewTraffic') }}</v-btn>
            <v-alert type="info" variant="tonal" density="comfortable" class="mt-4">{{ t('runtimeHint') }}</v-alert>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-alert v-if="loadError || overview.error" type="warning" variant="tonal" density="comfortable" class="mb-4">
      {{ loadError || overview.error }}
    </v-alert>
    <v-alert v-if="hasLoaded && !overview.available" type="info" variant="tonal" density="comfortable" class="mb-4">
      {{ t('unavailableHint') }}
    </v-alert>

    <v-card rounded="xl" variant="outlined" class="pf-table-card">
      <v-card-title class="pf-table-card__toolbar">
        <div>
          <div class="text-subtitle-1 font-weight-medium">{{ t('tableTitle') }}</div>
          <div class="text-caption text-medium-emphasis mt-1">{{ t('tableSubtitle') }}</div>
        </div>
      </v-card-title>
      <v-divider />
      <v-card-text>
        <v-row class="mb-2">
          <v-col cols="12" md="5"><v-text-field v-model="searchText" :label="t('searchLabel')" prepend-inner-icon="mdi-magnify" clearable hide-details /></v-col>
          <v-col cols="12" md="3"><v-select v-model="familyFilter" :items="familyFilterItems" :label="t('familyFilter')" hide-details /></v-col>
          <v-col cols="12" md="4"><v-select v-model="protocolFilter" :items="protocolFilterItems" :label="t('protocolFilter')" hide-details /></v-col>
        </v-row>

        <v-data-table v-if="!smAndDown" :headers="headers" :items="filteredRules" item-value="id" fixed-header class="rounded-lg pf-table" hide-no-data>
          <template #item.name="{ item }">
            <div class="py-2">
              <div class="d-flex align-center ga-2 flex-wrap">
                <div class="font-weight-medium">{{ item.name || t('ruleFallback') }}</div>
                <v-chip size="x-small" :color="item.enabled ? 'success' : 'grey'" variant="flat">{{ item.enabled ? t('enabled') : t('disabled') }}</v-chip>
                <v-chip v-if="trafficBlockLabel(item)" size="x-small" color="error" variant="tonal">{{ trafficBlockLabel(item) }}</v-chip>
                <v-chip v-if="item.runtimeConflictCount" size="x-small" color="warning" variant="tonal">{{ t('runtimeConflictShort', { count: item.runtimeConflictCount }) }}</v-chip>
              </div>
              <div v-if="item.description" class="text-caption text-medium-emphasis">{{ item.description }}</div>
              <div v-for="(conflict, conflictIndex) in conflictsForRule(item.id)" :key="`${conflict.protocol}-${conflict.port}-${conflict.socketFamily}-${conflict.bindAddress}-${conflictIndex}`" class="text-caption text-warning mt-1 pf-conflict-line">
                {{ t('runtimeConflictLine', { protocol: protocolLabel(conflict.protocol), port: conflict.port, bind: conflict.bindAddress, process: formatConflictOwners(conflict) }) }}
              </div>
            </div>
          </template>
          <template #item.local="{ item }"><div class="py-2"><div class="font-weight-medium">{{ item.localPortSpec || '-' }}</div><div class="text-caption text-medium-emphasis">{{ localModeLabel(item.localPortMode) }}</div></div></template>
          <template #item.target="{ item }"><div class="py-2 font-weight-medium">{{ targetDisplayLabel(item.targetIP, item.targetPort) }}</div></template>
          <template #item.lane="{ item }"><div class="py-2 d-flex flex-wrap ga-2"><v-chip size="small" variant="flat" class="pf-protocol-chip">{{ protocolLabel(item.protocol) }}</v-chip><v-chip size="small" color="info" variant="outlined">{{ familyLabel(item.family) }}</v-chip></div></template>
          <template #item.limit="{ item }">
            <div class="py-2"><div class="font-weight-medium">{{ rateLimitLabel(item.effectiveRateLimitMbps, item.rateLimitMbps, item.limitStatus) }}</div><div v-if="item.limitStatus === 'degraded' && item.limitWarning" class="text-caption text-warning">{{ t('limitDegraded') }}：{{ item.limitWarning }}</div><div v-else class="text-caption text-medium-emphasis">{{ t('leftPortOnly') }}</div></div>
          </template>
          <template #item.traffic="{ item }">
            <div class="py-2 pf-rule-traffic">
              <div class="font-weight-medium">{{ trafficUsageLabel(item) }}</div>
              <v-progress-linear v-if="item.trafficLimitGiB > 0" :model-value="trafficUsagePercent(item)" :color="item.trafficBlockReason ? 'error' : trafficUsagePercent(item) >= 90 ? 'warning' : 'success'" height="6" rounded class="mt-1" />
              <div class="text-caption text-medium-emphasis mt-1">{{ t('up') }} {{ formatTrafficGB(item.currentUp) }} / {{ t('down') }} {{ formatTrafficGB(item.currentDown) }}</div>
              <div v-if="trafficBlockLabel(item)" class="text-caption text-error mt-1">{{ trafficBlockLabel(item) }}</div>
            </div>
          </template>
          <template #item.actions="{ item }">
            <div class="d-flex align-center justify-end ga-1">
              <v-switch density="compact" color="success" hide-details inset :model-value="item.enabled" :loading="rowBusyId === item.id" :disabled="mutationBusy" :aria-label="t('toggleRule', { name: item.name || t('ruleFallback') })" @update:modelValue="value => toggleRule(item, Boolean(value))" />
              <v-btn icon="mdi-restart" size="small" variant="text" color="warning" :aria-label="t('resetRuleTraffic', { name: item.name || t('ruleFallback') })" :title="t('resetTraffic')" :disabled="mutationBusy" @click="resetRuleTraffic(item)" />
              <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" :aria-label="t('edit')" :title="t('edit')" :disabled="mutationBusy" @click="openRuleDialog(item)" />
              <v-btn icon="mdi-delete" size="small" variant="text" color="error" :aria-label="t('delete')" :title="t('delete')" :disabled="mutationBusy" @click="removeRule(item)" />
            </div>
          </template>
        </v-data-table>

        <div v-else class="pf-mobile-list">
          <v-card v-for="item in filteredRules" :key="item.id" variant="outlined" rounded="lg" class="pf-mobile-rule">
            <v-card-text>
              <div class="d-flex align-start justify-space-between ga-2">
                <div>
                  <div class="font-weight-medium">{{ item.name || t('ruleFallback') }}</div>
                  <div v-if="item.description" class="text-caption text-medium-emphasis mt-1">{{ item.description }}</div>
                </div>
                <div class="d-flex flex-column align-end ga-1">
                  <v-chip size="small" :color="item.enabled ? 'success' : 'grey'" variant="flat">{{ item.enabled ? t('enabled') : t('disabled') }}</v-chip>
                  <v-chip v-if="trafficBlockLabel(item)" size="x-small" color="error" variant="tonal">{{ trafficBlockLabel(item) }}</v-chip>
                </div>
              </div>
              <div class="pf-mobile-grid mt-4">
                <div><span>{{ t('localLabel') }}</span><strong>{{ item.localPortSpec || '-' }}</strong><small>{{ localModeLabel(item.localPortMode) }} · {{ protocolLabel(item.protocol) }} · {{ familyLabel(item.family) }}</small></div>
                <div><span>{{ t('targetLabel') }}</span><strong>{{ targetDisplayLabel(item.targetIP, item.targetPort) }}</strong></div>
                <div><span>{{ t('limitColumn') }}</span><strong>{{ rateLimitLabel(item.effectiveRateLimitMbps, item.rateLimitMbps, item.limitStatus) }}</strong><small v-if="item.limitWarning" class="text-warning">{{ item.limitWarning }}</small></div>
                <div class="pf-mobile-traffic"><span>{{ t('trafficColumn') }}</span><strong>{{ trafficUsageLabel(item) }}</strong><v-progress-linear v-if="item.trafficLimitGiB > 0" :model-value="trafficUsagePercent(item)" :color="item.trafficBlockReason ? 'error' : trafficUsagePercent(item) >= 90 ? 'warning' : 'success'" height="6" rounded /><small>{{ t('up') }} {{ formatTrafficGB(item.currentUp) }} / {{ t('down') }} {{ formatTrafficGB(item.currentDown) }}</small><small v-if="trafficBlockLabel(item)" class="text-error">{{ trafficBlockLabel(item) }}</small></div>
              </div>
              <v-alert v-for="(conflict, conflictIndex) in conflictsForRule(item.id)" :key="`${conflict.protocol}-${conflict.port}-${conflict.socketFamily}-${conflict.bindAddress}-${conflictIndex}`" type="warning" variant="tonal" density="compact" class="mt-3">
                {{ t('runtimeConflictLine', { protocol: protocolLabel(conflict.protocol), port: conflict.port, bind: conflict.bindAddress, process: formatConflictOwners(conflict) }) }}
              </v-alert>
              <div class="d-flex align-center justify-end ga-1 mt-3">
                <v-switch density="compact" color="success" hide-details inset :model-value="item.enabled" :loading="rowBusyId === item.id" :disabled="mutationBusy" :aria-label="t('toggleRule', { name: item.name || t('ruleFallback') })" @update:modelValue="value => toggleRule(item, Boolean(value))" />
                <v-btn icon="mdi-restart" size="small" variant="text" color="warning" :aria-label="t('resetRuleTraffic', { name: item.name || t('ruleFallback') })" :title="t('resetTraffic')" :disabled="mutationBusy" @click="resetRuleTraffic(item)" />
                <v-btn icon="mdi-pencil" size="small" variant="text" color="primary" :aria-label="t('edit')" :title="t('edit')" :disabled="mutationBusy" @click="openRuleDialog(item)" />
                <v-btn icon="mdi-delete" size="small" variant="text" color="error" :aria-label="t('delete')" :title="t('delete')" :disabled="mutationBusy" @click="removeRule(item)" />
              </div>
            </v-card-text>
          </v-card>
        </div>

        <div v-if="filteredRules.length === 0" class="pf-empty"><v-icon size="36" color="grey">mdi-swap-horizontal-off</v-icon><div class="text-subtitle-2 mt-2">{{ t('emptyText') }}</div></div>
      </v-card-text>
    </v-card>

    <v-dialog :model-value="dialogVisible" :fullscreen="smAndDown" :persistent="mutationBusy" scrollable max-width="1040" @update:model-value="value => value ? (dialogVisible = true) : closeRuleDialog()">
      <v-card :rounded="smAndDown ? '0' : 'xl'">
        <v-card-title class="d-flex align-center justify-space-between flex-wrap ga-3">
          <div><div class="text-subtitle-1 font-weight-medium">{{ dialogTitle }}</div><div class="text-caption text-medium-emphasis mt-1">{{ t('dialogSubtitle') }}</div></div>
          <v-switch v-model="editingRule.enabled" color="success" hide-details inset :label="t('enabled')" :disabled="mutationBusy" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-5 pf-dialog-body">
          <v-alert v-if="formError" type="warning" variant="tonal" density="comfortable" class="mb-4">{{ formError }}</v-alert>
          <v-row>
            <v-col cols="12" md="4"><v-text-field v-model="editingRule.name" :label="t('nameLabel')" :disabled="mutationBusy" :counter="120" hide-details /></v-col>
            <v-col cols="12" md="8"><v-text-field v-model="editingRule.description" :label="t('descLabel')" :disabled="mutationBusy" :counter="1000" hide-details /></v-col>
          </v-row>
          <v-row class="mt-1">
            <v-col cols="12" lg="6"><div class="pf-panel"><div class="pf-panel__title">{{ t('localPanelTitle') }}</div><div class="pf-panel__subtitle">{{ t('localPanelHint') }}</div>
              <v-row class="mt-1">
                <v-col cols="12" md="6"><v-select v-model="editingRule.protocol" :items="protocolItems" :label="t('protocolLabel')" :disabled="mutationBusy" hide-details /></v-col>
                <v-col cols="12" md="6"><v-select v-model="editingRule.family" :items="familyItems" :label="t('familyLabel')" :disabled="mutationBusy" hide-details /></v-col>
                <v-col cols="12"><v-select v-model="editingRule.localPortMode" :items="localModeItems" :label="t('modeLabel')" :disabled="mutationBusy" hide-details /></v-col>
                <v-col v-if="editingRule.localPortMode !== 'multi'" cols="12" sm="6"><v-text-field v-model.number="editingRule.localPortStart" type="number" min="1" max="65535" :label="localStartLabel" :disabled="mutationBusy" hide-details /></v-col>
                <v-col v-if="editingRule.localPortMode === 'multi'" cols="12"><v-text-field v-model="editingRule.localPortSpec" :label="t('multiLabel')" :placeholder="t('multiPlaceholder')" :disabled="mutationBusy" hide-details /></v-col>
                <v-col v-if="editingRule.localPortMode === 'range'" cols="12" sm="6"><v-text-field v-model.number="editingRule.localPortEnd" type="number" min="1" max="65535" :label="t('rangeEndLabel')" :disabled="mutationBusy" hide-details /></v-col>
                <v-col cols="12" md="6"><v-text-field v-model.number="editingRule.rateLimitMbps" type="number" min="0" max="1000000" :label="t('rateLabel')" :disabled="mutationBusy" hide-details /></v-col>
              </v-row>
              <div class="text-caption text-medium-emphasis mt-4">{{ t('rateHint') }}</div><v-alert type="info" variant="tonal" density="comfortable" class="mt-4">{{ localPreviewText }}</v-alert>
            </div></v-col>
            <v-col cols="12" lg="6"><div class="pf-panel pf-panel--target"><div class="pf-panel__title">{{ t('targetPanelTitle') }}</div><div class="pf-panel__subtitle">{{ t('targetPanelHint') }}</div>
              <v-row class="mt-1"><v-col cols="12" md="8"><v-text-field v-model="editingRule.targetIP" :label="t('targetIPLabel')" :disabled="mutationBusy" hide-details /></v-col><v-col cols="12" md="4"><v-text-field v-model.number="editingRule.targetPort" type="number" min="1" max="65535" :label="t('targetPortLabel')" :disabled="mutationBusy" hide-details /></v-col></v-row>
            </div></v-col>
          </v-row>
          <div class="pf-panel mt-4">
            <div class="pf-panel__title">{{ t('trafficControlTitle') }}</div>
            <div class="pf-panel__subtitle">{{ t('trafficControlHint') }}</div>
            <v-row class="mt-1">
              <v-col cols="12" md="4"><v-text-field v-model.number="editingRule.trafficLimitGiB" type="number" min="0" step="0.01" :label="t('trafficLimitLabel')" :disabled="mutationBusy" hide-details /></v-col>
              <v-col cols="12" md="4"><div class="pf-date-picker" :class="{ 'pf-date-picker--disabled': mutationBusy }" :aria-disabled="mutationBusy"><DatePick :expiry="ruleTrafficResetPickerEpoch" input-id="port-forward-traffic-reset-day" picker-type="date" :label-text="t('trafficResetDayLabel')" :zero-text="t('notEnabled')" @submit="submitRuleTrafficResetDay" /></div></v-col>
              <v-col cols="12" md="4"><div class="pf-date-picker" :class="{ 'pf-date-picker--disabled': mutationBusy }" :aria-disabled="mutationBusy"><DatePick :expiry="ruleTrafficExpiryPickerEpoch" input-id="port-forward-traffic-expiry" picker-type="date" :label-text="t('trafficExpiryDateLabel')" :zero-text="t('notEnabled')" @submit="submitRuleTrafficExpiryDate" /></div></v-col>
            </v-row>
          </div>
        </v-card-text>
        <v-card-actions class="px-6 pb-5"><v-spacer /><v-btn variant="text" :disabled="mutationBusy" @click="closeRuleDialog">{{ t('cancel') }}</v-btn><v-btn color="primary" :loading="savingRule" :disabled="Boolean(formError) || mutationBusy" @click="saveRule">{{ t('save') }}</v-btn></v-card-actions>
      </v-card>
    </v-dialog>
  </section>
</template>

<script setup lang="ts">
import { useDisplay } from 'vuetify'
import DatePick from '@/components/DateTime.vue'
import {
  familyLabel,
  formatTrafficGB,
  localModeLabel,
  protocolLabel,
  rateLimitLabel,
  targetDisplayLabel,
  trafficBlockLabel,
  trafficUsageLabel,
  trafficUsagePercent,
  usePortForwardManage,
} from './SettingsPortForwardManage.shared'

const props = withDefaults(defineProps<{ active?: boolean }>(), { active: false })
const { smAndDown } = useDisplay()
const {
  loading, refreshing, savingRule, mutationBusy, dialogVisible, rowBusyId, hasLoaded, loadError,
  searchText, familyFilter, protocolFilter, overview, editingRule, headers, familyItems,
  familyFilterItems, protocolItems, protocolFilterItems, localModeItems, lastSyncLabel,
  dialogTitle, localStartLabel, localPreviewText, formError, ruleTrafficResetPickerEpoch,
  ruleTrafficExpiryPickerEpoch, overviewResetBusy, filteredRules, conflictsForRule,
  capabilityLabel, capabilityChipColor, compatibilityModeLabel,
  formatConflictOwners, refreshOverview, openRuleDialog, closeRuleDialog, saveRule, toggleRule,
  removeRule, resetRuleTraffic, resetOverviewTraffic, submitRuleTrafficResetDay,
  submitRuleTrafficExpiryDate, t,
} = usePortForwardManage(props)
</script>

<style scoped>
.pf-page { margin-top: 20px; }
.pf-hero { position: relative; overflow: hidden; border: 1px solid rgba(18, 120, 132, 0.18); background: linear-gradient(135deg, rgba(8, 50, 58, 0.96), rgba(18, 85, 88, 0.92)); color: #eef8f8; }
.pf-hero__bg { position: absolute; inset: 0; background: radial-gradient(circle at 18% 24%, rgba(86, 214, 190, 0.22), transparent 34%), radial-gradient(circle at 78% 18%, rgba(247, 190, 78, 0.18), transparent 28%); }
.pf-hero__content, .pf-side, .pf-table-card { position: relative; }
.pf-hero__top, .pf-table-card__toolbar { display: flex; justify-content: space-between; gap: 16px; align-items: flex-start; flex-wrap: wrap; }
.pf-hero__icon { width: 58px; height: 58px; border-radius: 18px; display: grid; place-items: center; background: rgba(255, 255, 255, 0.14); border: 1px solid rgba(255, 255, 255, 0.22); }
.pf-hero__eyebrow { letter-spacing: 0.18em; color: rgba(220, 250, 246, 0.78); }
.pf-hero__toolbar, .pf-hero__chips { display: flex; gap: 8px; flex-wrap: wrap; }
.pf-hero__toolbar { justify-content: flex-end; gap: 10px; }
.pf-hero-action { min-width: 112px; min-height: 38px; border: 1px solid rgba(94, 234, 212, 0.52) !important; border-radius: 11px; color: #f8fafc !important; background: rgba(15, 78, 75, 0.24) !important; }
.pf-hero-action.v-btn--disabled { opacity: 0.7 !important; }
.pf-hero-chip { min-height: 28px; box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.12); }
.pf-hero-chip--sync { min-width: 138px; background: #0f766e !important; color: #ecfeff !important; }
.pf-hero-chip--count { min-width: 84px; justify-content: center; background: #1d4ed8 !important; color: #eff6ff !important; }
.pf-hero-chip--capability { max-width: 100%; height: auto; min-height: 28px; }
.pf-hero-chip--capability :deep(.v-chip__content) { padding-block: 2px; white-space: normal; overflow-wrap: anywhere; }
.pf-metric, .pf-side__row, .pf-panel { border: 1px solid rgba(148, 163, 184, 0.14); border-radius: 18px; color: rgba(226, 232, 240, 0.94); background: rgba(15, 23, 42, 0.46); }
.pf-muted-label, .pf-side__row span { color: rgba(148, 163, 184, 0.92); }
.pf-side__row strong:not(.text-success):not(.text-warning) { color: rgba(226, 232, 240, 0.96); }
.pf-metric { padding: 14px; min-height: 108px; }
.pf-side__row { padding: 12px 14px; display: flex; justify-content: space-between; margin-bottom: 10px; gap: 12px; }
.pf-side__row strong { min-width: 0; text-align: right; overflow-wrap: anywhere; }
.pf-table { overflow: hidden; }
.pf-empty { min-height: 160px; display: grid; place-items: center; color: rgba(60, 72, 80, 0.8); text-align: center; }
.pf-panel { padding: 18px; min-height: 100%; }
.pf-panel--target { background: rgba(17, 25, 44, 0.5); }
.pf-panel__title { font-size: 15px; font-weight: 600; }
.pf-panel__subtitle { margin-top: 6px; font-size: 12px; color: rgba(148, 163, 184, 0.9); }
.pf-protocol-chip { background: #f3e3a1 !important; color: #5f4a00 !important; border: 1px solid rgba(255, 245, 204, 0.5); }
.pf-conflict-line { overflow-wrap: anywhere; }
.pf-rule-traffic, .pf-mobile-traffic { min-width: 0; overflow-wrap: anywhere; }
.pf-mobile-traffic .v-progress-linear { margin-top: 4px; }
.pf-date-picker--disabled { pointer-events: none; opacity: 0.65; }
.pf-mobile-list { display: grid; gap: 12px; }
.pf-mobile-rule { overflow: hidden; }
.pf-mobile-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.pf-mobile-grid div { min-width: 0; display: grid; gap: 3px; }
.pf-mobile-grid span { color: rgba(100, 116, 139, 0.95); font-size: 12px; }
.pf-mobile-grid strong, .pf-mobile-grid small { overflow-wrap: anywhere; }
.pf-mobile-grid small { color: rgba(100, 116, 139, 0.95); }
.pf-dialog-body { overflow-y: auto; }
@media (max-width: 959px) {
  .pf-hero__top, .pf-table-card__toolbar { flex-direction: column; }
  .pf-hero__toolbar { width: 100%; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .pf-hero-action { width: 100%; min-width: 0; }
}
@media (max-width: 599px) {
  .pf-page { margin-top: 12px; }
  .pf-mobile-grid { grid-template-columns: 1fr; }
  .pf-panel { padding: 14px; }
}
</style>
