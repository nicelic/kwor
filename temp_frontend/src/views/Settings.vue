<template>
  <v-card :loading="loading">
    <v-tabs
      v-if="hasVerifiedSettings"
      v-model="tab"
      color="primary"
      align-tabs="center"
      show-arrows
    >
      <v-tab value="t1">{{ $t('setting.interface') }}</v-tab>
      <v-tab value="t2">{{ $t('setting.sub') }}</v-tab>
      <v-tab value="t3">{{ $t('setting.jsonSub') }}</v-tab>
      <v-tab value="t4">{{ $t('setting.clashSub') }}</v-tab>
      <v-tab value="t5">Language</v-tab>
      <v-tab value="t6">{{ $t('setting.trafficManage') }}</v-tab>
      <v-tab value="t7">防火墙</v-tab>
      <v-tab value="t8">转发</v-tab>
      <v-tab value="t9">优化</v-tab>
      <v-tab value="t10">证书管理</v-tab>
      <v-tab value="t11">反向代理</v-tab>
      <v-tab value="t12">{{ $t('setting.kernelManage') }}</v-tab>
    </v-tabs>

    <v-card-text>
      <v-row v-if="hasVerifiedSettings && showTopActionBar" align="center" justify="center" style="margin-bottom: 10px;">
        <v-col cols="auto">
          <v-btn color="primary" @click="save" :loading="loading" :disabled="!stateChange || panelLifecycleBusy || !hasVerifiedSettings">
            {{ $t('actions.save') }}
          </v-btn>
        </v-col>
        <v-col cols="auto">
          <v-btn variant="outlined" color="warning" @click="restartApp" :loading="loading" :disabled="stateChange || panelLifecycleBusy || !hasVerifiedSettings || !panelCanRestart">
            {{ $t('actions.restartApp') }}
          </v-btn>
        </v-col>
        <v-col cols="auto" v-if="showSubPageResetButton">
          <v-btn variant="outlined" color="error" @click="openResetDialog" :disabled="loading || panelLifecycleBusy">
            {{ resetButtonText }}
          </v-btn>
        </v-col>
      </v-row>

      <v-dialog v-if="hasVerifiedSettings" v-model="resetDialogVisible" max-width="460">
        <v-card>
		  <v-card-title>{{ $t('subscriptionEditor.resetConfirmTitle') }}</v-card-title>
          <v-card-text>{{ resetDialogMessage }}</v-card-text>
          <v-card-actions>
            <v-spacer></v-spacer>
			<v-btn variant="text" :disabled="loading" @click="closeResetDialog">{{ $t('actions.close') }}</v-btn>
			<v-btn color="error" variant="outlined" :loading="loading" :disabled="loading" @click="confirmResetSubPage">{{ $t('subscriptionEditor.resetConfirm') }}</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <v-row v-if="!hasVerifiedSettings" justify="center" class="py-6">
        <v-col cols="12" sm="10" md="8" lg="7">
          <v-alert :type="settingsLoadState === 'error' ? 'error' : 'info'" variant="tonal">
            <div v-if="settingsLoadState === 'error'" class="text-body-2">
              {{ $t('setting.settingsLoadFailed') }}：{{ settingsLoadError || $t('setting.settingsLoadFallback') }}
            </div>
            <div v-else class="d-flex align-center flex-wrap" style="gap: 10px;">
              <v-progress-circular indeterminate size="20" width="2" />
              <span>{{ $t('setting.settingsLoading') }}</span>
            </div>
            <div v-if="settingsLoadState === 'error'" class="d-flex flex-wrap mt-3" style="gap: 8px;">
              <v-btn color="primary" variant="outlined" @click="retryLoadData">
                {{ $t('setting.settingsReload') }}
              </v-btn>
            </div>
          </v-alert>
        </v-col>
      </v-row>

      <v-window v-else v-model="tab">
        <v-window-item value="t1">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webListen" :label="$t('setting.addr')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webPort" min="1" type="number" :label="$t('setting.port')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webPath" :label="$t('setting.webPath')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webDomain" :label="$t('setting.domain')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.webURI" :label="$t('setting.webUri')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                type="number"
                v-model="settings.sessionMaxAge"
                min="0"
                :label="$t('setting.sessionAge')"
                :suffix="$t('date.m')"
                hide-details
              ></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                type="number"
                v-model="settings.trafficAge"
                min="0"
                :label="$t('setting.trafficAge')"
                :suffix="$t('date.d')"
                hide-details
              ></v-text-field>
              <div class="text-caption text-medium-emphasis mt-1">{{ $t('setting.trafficAgeHint') }}</div>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="settings.timeLocation"
                :items="timeZoneOptions"
                item-title="title"
                item-value="value"
                item-props="props"
                :label="$t('setting.panelTimeLoc')"
                hide-details
                density="comfortable"
                variant="outlined"
                :menu-props="{ maxHeight: 360 }"
              ></v-select>
              <div v-if="hiddenPanelTimeLocation" class="text-caption text-warning mt-1">
                {{ $t('setting.panelTimeUnknownHint', { value: hiddenPanelTimeLocation }) }}
              </div>
              <div class="text-caption text-medium-emphasis mt-1">{{ $t('setting.panelTimeScope') }}</div>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="systemTimeLocation"
                :items="timeZoneOptions"
                item-title="title"
                item-value="value"
                item-props="props"
                :label="$t('setting.systemTimeLoc')"
                hide-details
                density="comfortable"
                variant="outlined"
                :menu-props="{ maxHeight: 360 }"
                :disabled="systemTimeZoneLoadState === 'loading' || systemTimeZoneLoadState === 'error'"
                @update:model-value="onSystemTimeLocationSelected"
              ></v-select>
              <div v-if="systemTimeZoneLoadState === 'error'" class="text-caption text-error mt-1">
                <div>{{ $t('setting.systemTimeReadFailed') }}：{{ systemTimeZoneLoadError || $t('setting.requestFailed') }}</div>
                <v-btn class="mt-1" size="small" variant="text" color="primary" @click="retryLoadSystemTimeZone">
                  {{ $t('setting.systemTimeReload') }}
                </v-btn>
              </div>
              <div v-else-if="systemTimeZoneStatus.reason" class="text-caption text-medium-emphasis mt-1">
                {{ systemTimeZoneStatus.reason }}
              </div>
              <div v-else class="text-caption text-medium-emphasis mt-1">{{ $t('setting.systemTimeScope') }}</div>
            </v-col>
          </v-row>

          <v-alert v-if="!panelCanRestart" type="warning" variant="tonal" density="compact" class="mt-3">
            {{ panelRestartHint }}
          </v-alert>
          <v-alert type="info" variant="tonal" density="compact" class="mt-3">
            {{ $t('setting.panelRestartRequiredHint') }}
          </v-alert>

          <v-divider class="my-6"></v-divider>

          <v-row align="center" class="mb-2">
            <v-col cols="12" class="d-flex align-center flex-wrap" style="gap: 8px;">
              <v-chip variant="outlined" color="success" size="small" label>
                <v-progress-circular
                  v-if="panelStatusLoading"
                  indeterminate
                  size="12"
                  width="2"
                  class="mr-1"
                ></v-progress-circular>
                {{ $t('setting.panelLocal') }}: {{ panelLocalVersionLabel }}
              </v-chip>
              <v-chip variant="outlined" color="info" size="small" label>
                <v-progress-circular
                  v-if="panelRemoteLoading"
                  indeterminate
                  size="12"
                  width="2"
                  class="mr-1"
                ></v-progress-circular>
                {{ $t('setting.panelRemote') }}: {{ panelRemoteVersionLabel }}
              </v-chip>
              <v-chip v-if="panelBinaryName" variant="tonal" size="small" label>
                {{ $t('setting.panelFile') }}: {{ panelBinaryName }}
                <v-tooltip
                  v-if="panelUpdateStatus?.binaryPath"
                  activator="parent"
                  location="top"
                  :text="panelUpdateStatus.binaryPath"
                />
              </v-chip>
            </v-col>
          </v-row>

          <v-row align="center">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="panelSelectedVersion"
                v-model:menu="panelVersionMenuVisible"
                :items="panelVersionItems"
                item-title="title"
                item-value="value"
                :label="$t('setting.panelVersion')"
                variant="outlined"
                density="compact"
                hide-details
                :loading="panelRemoteLoading"
                :disabled="panelRemoteLoading || panelLoadingMoreVersions || panelLifecycleBusy"
                :menu-props="{ maxHeight: 260 }"
                :no-data-text="panelVersionNoDataText"
                @update:menu="onPanelVersionMenuUpdate"
              >
                <template #item="{ props: itemProps, item }">
                  <v-list-item
                    v-bind="itemProps"
                    :subtitle="item.raw.assetName || undefined"
                  >
                    <template #append>
                      <v-chip
                        v-if="item.raw.prerelease"
                        size="x-small"
                        color="warning"
                        variant="flat"
                      >
                        {{ $t('setting.panelPrerelease') }}
                      </v-chip>
                    </template>
                  </v-list-item>
                </template>
                <template #append-item>
                  <v-divider v-if="panelVersionItems.length > 0" class="mt-1" />
                  <div
                    v-if="panelVersionItems.length > 0"
                    class="panel-version-footer px-3 py-3 d-flex align-center justify-space-between flex-wrap"
                    style="gap: 10px;"
                  >
                    <span class="text-caption panel-version-footer__summary">
                      {{ $t('setting.panelVersionsLoaded', { count: panelVersionItems.length }) }}
                    </span>
                    <div class="d-flex align-center flex-wrap" style="gap: 8px;">
                      <v-btn
                        size="small"
                        color="primary"
                        variant="tonal"
                        class="panel-version-footer__action"
                        :loading="panelLoadingMoreVersions"
                        :disabled="panelLifecycleBusy || panelRemoteLoading || panelAllVersionsLoaded"
                        @mousedown.prevent
                        @click.stop="loadMorePanelVersions"
                      >
                        {{ panelAllVersionsLoaded ? $t('setting.panelNoMoreVersions') : $t('setting.panelLoadMoreVersions') }}
                      </v-btn>
                    </div>
                  </div>
                </template>
              </v-select>
            </v-col>

            <v-col cols="auto">
              <v-btn
                color="secondary"
                variant="tonal"
                prepend-icon="mdi-refresh"
                :loading="panelRemoteLoading"
                :disabled="panelRemoteLoading || panelLoadingMoreVersions || panelLifecycleBusy"
                @click="checkPanelUpdates"
              >
                {{ $t('setting.panelCheckUpdates') }}
              </v-btn>
            </v-col>

            <v-col cols="12" sm="auto">
              <v-btn
                color="primary"
                variant="flat"
                :prepend-icon="panelUpdateTaskActive ? (panelUpdateTaskApplying ? 'mdi-progress-wrench' : 'mdi-stop') : 'mdi-download'"
                :disabled="panelUpdateTaskActive
                  ? panelUpdateStopRequestPending || !panelUpdateTaskCanCancel
                  : !panelSelectedVersion || panelLifecycleBusy || panelRemoteLoading || !panelCanInstall"
                @click="panelUpdateTaskActive ? stopPanelUpdateTask() : openPanelInstallDialog()"
              >
                {{ panelUpdateTaskButtonText }}
              </v-btn>
            </v-col>
          </v-row>

          <v-row v-if="panelManagedUpdateTask" class="mt-1">
            <v-col cols="12">
              <v-alert
                :type="panelUpdateTaskAlertType"
                variant="tonal"
                density="compact"
                class="panel-update-task-status"
              >
                <div class="d-flex align-center justify-space-between flex-wrap" style="gap: 8px;">
                  <span class="panel-update-task-status__text">{{ panelUpdateTaskStatusText }}</span>
                  <span v-if="panelManagedUpdateTask.id" class="text-caption text-medium-emphasis panel-update-task-status__id">
                    {{ panelManagedUpdateTask.id }}
                  </span>
                </div>
              </v-alert>
            </v-col>
          </v-row>

          <v-row v-if="panelUpdateFeedback" class="mt-1">
            <v-col v-if="panelUpdateFeedback" cols="12">
              <v-alert
                :type="panelUpdateFeedbackType"
                variant="tonal"
                density="compact"
                closable
                @click:close="panelUpdateFeedback = ''"
              >
                <div class="d-flex align-center justify-space-between flex-wrap" style="gap: 8px;">
                  <span>{{ panelUpdateFeedback }}</span>
                  <v-btn
                    v-if="panelUpdateStatus?.lastUpdateLogPath"
                    size="small"
                    variant="text"
                    color="primary"
                    :loading="panelUpdateLogLoading"
                    @click="openPanelUpdateLogDialog"
                  >
                    {{ $t('setting.panelViewLog') }}
                  </v-btn>
                </div>
              </v-alert>
            </v-col>
          </v-row>
          <v-row v-if="panelInstallHint" class="mt-1">
            <v-col cols="12">
              <v-alert
                type="warning"
                variant="tonal"
                density="compact"
              >
                {{ panelInstallHint }}
              </v-alert>
            </v-col>
          </v-row>

          <v-row v-if="panelUninstallFailed" class="mt-1">
            <v-col cols="12">
              <v-alert type="error" variant="tonal" density="compact">
                <div class="d-flex align-start justify-space-between flex-wrap" style="gap: 8px;">
                  <div class="panel-uninstall-failure">
                    <div class="font-weight-medium">
                      {{ $t('setting.uninstallPanelFailed') }}
                      <span v-if="panelUninstallPhase">: {{ panelUninstallPhase }}</span>
                    </div>
                    <div v-if="panelUninstallError" class="text-body-2 mt-1">{{ panelUninstallError }}</div>
                    <ul v-if="panelUninstallFailures.length > 0" class="panel-uninstall-message-list mt-2">
                      <li v-for="failure in panelUninstallFailures" :key="failure">{{ failure }}</li>
                    </ul>
                    <ul v-if="panelUninstallWarnings.length > 0" class="panel-uninstall-message-list text-medium-emphasis mt-2">
                      <li v-for="warning in panelUninstallWarnings" :key="warning">{{ warning }}</li>
                    </ul>
                  </div>
                  <v-btn
                    v-if="panelUninstallCanRetry"
                    color="error"
                    variant="outlined"
                    prepend-icon="mdi-reload"
                    :disabled="panelStatusLoading || loading || panelLifecycleBusy"
                    @click="requestPanelUninstall"
                  >
                    {{ $t('setting.uninstallPanelRetry') }}
                  </v-btn>
                </div>
              </v-alert>
            </v-col>
          </v-row>

          <v-divider class="mt-6 mb-4" />
          <v-row class="mt-0" justify="end">
            <v-col cols="12" sm="auto" class="d-flex justify-end">
              <v-tooltip
                :disabled="panelCanUseUninstallAction || !panelUninstallHint"
                location="top"
                :text="panelUninstallHint"
              >
                <template #activator="{ props }">
                  <span v-bind="props" class="panel-uninstall-trigger">
                    <v-btn
                      class="panel-uninstall-button"
                      color="error"
                      variant="outlined"
                      prepend-icon="mdi-delete-forever"
                      :loading="panelUninstalling"
                      :disabled="panelStatusLoading || loading || panelLifecycleBusy || !panelCanUseUninstallAction"
                      @click="requestPanelUninstall"
                    >
                      {{ panelUninstallButtonText }}
                    </v-btn>
                  </span>
                </template>
              </v-tooltip>
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t2">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-switch color="primary" v-model="subEncode" :label="$t('setting.subEncode')" hide-details />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-switch color="primary" v-model="subShowInfo" :label="$t('setting.subInfo')" hide-details />
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.subListen" :label="$t('setting.addr')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                type="number"
                v-model="settings.subPort"
                min="1"
                :label="$t('setting.port')"
                hide-details
              ></v-text-field>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.subDomain" :label="$t('setting.domain')" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.subPath" :label="$t('setting.path')" hide-details></v-text-field>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                type="number"
                v-model="settings.subUpdates"
                min="1"
                :label="$t('setting.update')"
                :suffix="$t('date.h')"
                hide-details
              ></v-text-field>
              <div class="text-caption text-medium-emphasis mt-1">{{ $t('setting.subUpdatesHint') }}</div>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="settings.subURI" :label="$t('setting.subUri')" hide-details></v-text-field>
            </v-col>
          </v-row>
          <v-alert type="info" variant="tonal" density="compact" class="mt-3">
            {{ $t('setting.subRestartRequiredHint') }}
          </v-alert>
        </v-window-item>

		<v-window-item value="t3">
          <SubJsonExtVue
			v-if="tab === 't3' && subJsonDraftLoadState === 'ready'"
			:key="`json-${subscriptionDraftGeneration}`"
            ref="subJsonExtRef"
			:settings="subJsonDraftSettings"
			:canonical-default="settingsDefaults.jsonExt"
			:initial-dirty="subJsonDraftDirty"
			:initial-reset="subJsonResetPending"
			:rule-set-sources="ruleSetSources.json"
			@dirty-change="onSubJsonDirtyChange"
          />
          <v-alert v-else-if="tab === 't3'" :type="subJsonDraftLoadState === 'error' ? 'error' : 'info'" variant="tonal" class="my-3">
            <div v-if="subJsonDraftLoadState === 'loading'" class="d-flex align-center" style="gap: 8px;">
              <v-progress-circular indeterminate size="18" width="2" />
              <span>{{ $t('setting.subscriptionExtensionLoading') }}</span>
            </div>
            <template v-else>
              <div>{{ subJsonDraftLoadError || $t('setting.subscriptionExtensionLoadFailed') }}</div>
              <v-btn v-if="subJsonDraftLoadState === 'error'" class="mt-2" size="small" variant="outlined" @click="retrySubscriptionDraft('json')">
                {{ $t('setting.subscriptionExtensionRetry') }}
              </v-btn>
            </template>
          </v-alert>
        </v-window-item>

		<v-window-item value="t4">
          <SubClashExtVue
			v-if="tab === 't4' && subClashDraftLoadState === 'ready'"
			:key="`clash-${subscriptionDraftGeneration}`"
            ref="subClashExtRef"
			:settings="subClashDraftSettings"
			:canonical-default="settingsDefaults.clashExt"
			:initial-dirty="subClashDraftDirty"
			:initial-reset="subClashResetPending"
			:rule-set-sources="ruleSetSources.clash"
			@dirty-change="onSubClashDirtyChange"
          />
          <v-alert v-else-if="tab === 't4'" :type="subClashDraftLoadState === 'error' ? 'error' : 'info'" variant="tonal" class="my-3">
            <div v-if="subClashDraftLoadState === 'loading'" class="d-flex align-center" style="gap: 8px;">
              <v-progress-circular indeterminate size="18" width="2" />
              <span>{{ $t('setting.subscriptionExtensionLoading') }}</span>
            </div>
            <template v-else>
              <div>{{ subClashDraftLoadError || $t('setting.subscriptionExtensionLoadFailed') }}</div>
              <v-btn v-if="subClashDraftLoadState === 'error'" class="mt-2" size="small" variant="outlined" @click="retrySubscriptionDraft('clash')">
                {{ $t('setting.subscriptionExtensionRetry') }}
              </v-btn>
            </template>
          </v-alert>
        </v-window-item>

        <v-window-item value="t5">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-select
                hide-details
                label="Language"
                :items="languages"
                v-model="$i18n.locale"
                @update:modelValue="changeLocale"
              >
              </v-select>
            </v-col>
          </v-row>
        </v-window-item>

        <v-window-item value="t6">
          <SettingsTrafficManageVue :active="tab === 't6'" />
        </v-window-item>

        <v-window-item value="t7">
          <SettingsFirewallManageVue :active="tab === 't7'" />
        </v-window-item>

        <v-window-item value="t8">
          <SettingsPortForwardManageVue :active="tab === 't8'" />
        </v-window-item>

        <v-window-item value="t9">
          <SettingsOptimizationManageVue :active="tab === 't9'" />
        </v-window-item>

        <v-window-item value="t10">
          <SettingsAcmeManageVue :active="tab === 't10'" />
        </v-window-item>

        <v-window-item value="t11">
          <SettingsReverseProxyManageVue :active="tab === 't11'" />
        </v-window-item>

        <v-window-item value="t12">
          <SettingsKernelManageVue :active="tab === 't12'" />
        </v-window-item>
      </v-window>

      <v-dialog v-model="panelInstallDialogVisible" max-width="480">
        <v-card>
          <v-card-title>{{ $t('setting.panelInstallConfirmTitle') }}</v-card-title>
          <v-card-text>
            {{ $t('setting.panelInstallConfirmMessage', { version: panelSelectedVersion || '-' }) }}
          </v-card-text>
          <v-card-actions>
            <v-spacer></v-spacer>
            <v-btn variant="text" :disabled="panelInstalling" @click="panelInstallDialogVisible = false">{{ $t('setting.panelCancel') }}</v-btn>
            <v-btn color="primary" variant="flat" :disabled="panelInstalling" @click="installPanelVersion">
              {{ panelInstalling ? '正在提交' : $t('setting.panelConfirmInstall') }}
            </v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <v-dialog v-model="panelDockerUninstallDialogVisible" max-width="860">
        <v-card>
          <v-card-title class="text-subtitle-1 font-weight-medium">{{ $t('setting.uninstallDockerGuideTitle') }}</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-4">{{ $t('setting.uninstallDockerGuideDesc') }}</div>
            <section
              v-for="instruction in panelDockerUninstallCommands"
              :key="instruction.id || instruction.command"
              class="docker-uninstall-command mb-4"
            >
              <div class="d-flex align-center justify-space-between" style="gap: 8px;">
                <div class="text-subtitle-2">{{ panelDockerUninstallCommandLabel(instruction.id) }}</div>
                <v-btn
                  icon="mdi-content-copy"
                  size="small"
                  variant="text"
                  :aria-label="$t('copyToClipboard')"
                  @click="copyDockerUninstallCommand(instruction.command)"
                >
                  <v-tooltip activator="parent" location="top" :text="$t('copyToClipboard')" />
                </v-btn>
              </div>
              <pre class="docker-uninstall-command__content"><code>{{ instruction.command }}</code></pre>
            </section>
          </v-card-text>
          <v-divider />
          <v-card-actions>
            <v-spacer />
            <v-btn variant="text" @click="panelDockerUninstallDialogVisible = false">{{ $t('actions.close') }}</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>

      <v-overlay :model-value="panelRestartOverlay" class="align-center justify-center" persistent>
        <v-card width="400" rounded="lg">
          <v-card-text class="text-center py-8">
            <v-progress-circular indeterminate size="52" width="5" color="primary" class="mb-4" />
            <div class="text-subtitle-1 font-weight-medium">{{ $t('setting.panelRestartingTitle') }}</div>
            <div class="text-caption text-medium-emphasis mt-2">{{ $t('setting.panelRestartingDesc') }}</div>
          </v-card-text>
        </v-card>
      </v-overlay>

      <v-overlay :model-value="panelUninstallOverlay" class="align-center justify-center" persistent>
        <v-card class="panel-uninstall-overlay-card">
          <v-card-text class="text-center py-8">
            <v-progress-circular indeterminate size="52" width="5" color="error" class="mb-4" />
            <div class="text-subtitle-1 font-weight-medium">{{ $t('setting.uninstallPanelPendingTitle') }}</div>
            <div class="text-caption text-medium-emphasis mt-2">{{ $t('setting.uninstallPanelPendingDesc') }}</div>
          </v-card-text>
        </v-card>
      </v-overlay>

      <v-dialog v-model="panelUpdateLogDialogVisible" max-width="960">
        <v-card rounded="xl" :loading="panelUpdateLogLoading">
          <v-card-title class="text-subtitle-1 font-weight-medium">{{ $t('setting.panelUpdateLogTitle') }}</v-card-title>
          <v-divider />
          <v-card-text>
            <div class="text-body-2 text-medium-emphasis mb-2">
              {{ $t('setting.panelLogPath') }}：{{ panelUpdateStatus?.lastUpdateLogPath || '-' }}
            </div>
            <div class="text-body-2 text-medium-emphasis mb-4" v-if="panelUpdateLogModifiedText">
              {{ $t('setting.panelLogUpdatedAt') }}：{{ panelUpdateLogModifiedText }}
            </div>
            <pre class="acme-log">{{ panelUpdateLogContent }}</pre>
          </v-card-text>
          <v-divider />
          <v-card-actions>
            <v-spacer />
            <v-btn variant="text" @click="panelUpdateLogDialogVisible = false">{{ $t('actions.close') }}</v-btn>
          </v-card-actions>
        </v-card>
      </v-dialog>
    </v-card-text>
  </v-card>
</template>

<script lang="ts" setup>
import { useLocale } from 'vuetify'
import { i18n, languages } from '@/locales'
import { Ref, computed, defineAsyncComponent, inject, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import HttpUtils, { type Msg } from '@/plugins/httputil'
import router from '@/router'
import { FindDiff } from '@/plugins/utils'
import { formatPanelDateTime, refreshPanelTimeContext } from '@/plugins/panelTime'
import { confirm } from '@/plugins/confirm'
import { push } from 'notivue'
import SettingsTrafficManageVue from '@/components/SettingsTrafficManage.vue'
import SettingsFirewallManageVue from '@/components/SettingsFirewallManage.vue'
import SettingsPortForwardManageVue from '@/components/SettingsPortForwardManage.vue'
import SettingsOptimizationManageVue from '@/components/SettingsOptimizationManage.vue'
import SettingsAcmeManageVue from '@/components/SettingsAcmeManage.vue'
import SettingsReverseProxyManageVue from '@/components/SettingsReverseProxyManage.vue'
import SettingsKernelManageVue from '@/components/SettingsKernelManage.vue'

const SubJsonExtVue = defineAsyncComponent(() => import('@/components/SubJsonExt.vue'))
const SubClashExtVue = defineAsyncComponent(() => import('@/components/SubClashExt.vue'))

const locale = useLocale()
const tab = ref('t1')
const loading: Ref = inject('loading') ?? ref(false)
type SettingsLoadState = 'idle' | 'loading' | 'ready' | 'error'
type SystemTimeZoneLoadState = 'idle' | 'loading' | 'ready' | 'error'
type RuleSetSourceEntry = {
	id: string
	title: string
	domainTemplate?: string
	ipTemplate?: string
	format: string
}
type SettingsSnapshot = {
	revision: number
	values: Record<string, string>
	defaults: { jsonExt: string; clashExt: string }
	ruleSetSources: { json: RuleSetSourceEntry[]; clash: RuleSetSourceEntry[] }
	extensionsIncluded: boolean
}

type SubscriptionSettingsSnapshot = {
	revision: number
	kind: 'json' | 'clash'
	value: string
	default: string
	ruleSetSources: RuleSetSourceEntry[]
}

type SubscriptionInitialResetResult = {
	revision: number
	kind: 'json' | 'clash'
	changedKeys: string[]
	values: Record<string, string>
	warnings?: string[]
}

const settingsLoadState = ref<SettingsLoadState>('idle')
const settingsLoadError = ref('')
let settingsLoadRequestSequence = 0
let systemTimeZoneRequestSequence = 0
const hasVerifiedSettings = computed(() => settingsLoadState.value === 'ready')
const oldSettings = ref<Record<string, any>>({})
const subJsonExtRef = ref<any>(null)
const subClashExtRef = ref<any>(null)
const settingsRevision = ref(0)
const settingsDefaults = ref({ jsonExt: '', clashExt: '' })
const ruleSetSources = ref<{ json: RuleSetSourceEntry[]; clash: RuleSetSourceEntry[] }>({ json: [], clash: [] })
const subJsonDraftSettings = ref<Record<string, any>>({})
const subClashDraftSettings = ref<Record<string, any>>({})
const subJsonDraftValue = ref('')
const subClashDraftValue = ref('')
const subJsonDraftDirty = ref(false)
const subClashDraftDirty = ref(false)
const subJsonDraftError = ref('')
const subClashDraftError = ref('')
type SubscriptionDraftLoadState = 'idle' | 'loading' | 'ready' | 'error'
const subJsonDraftLoadState = ref<SubscriptionDraftLoadState>('idle')
const subClashDraftLoadState = ref<SubscriptionDraftLoadState>('idle')
const subJsonDraftLoadError = ref('')
const subClashDraftLoadError = ref('')
const subscriptionDraftLoadRequestSequence: Record<'json' | 'clash', number> = { json: 0, clash: 0 }
const subJsonResetPending = ref(false)
const subClashResetPending = ref(false)
const subscriptionDraftGeneration = ref(0)
const resetDialogVisible = ref(false)
const resetTarget = ref<'json' | 'clash' | ''>('')

type PanelVersionItem = {
  title: string
  value: string
  tagName: string
  name?: string
  prerelease?: boolean
  publishedAt?: string
  assetName?: string
  assetSize?: number
}

type PanelManagedUpdateTask = {
  id: string
  state: string
  phase: string
  canCancel: boolean
  stopRequested: boolean
  deadlineExceeded: boolean
  startedAt: number
  updatedAt: number
  deadlineAt: number
  finishedAt: number
  error: string
}

type PanelUpdateStatus = {
  localVersion?: string
  binaryPath?: string
  binaryName?: string
  installDir?: string
  serviceFilePath?: string
  serviceBinaryPath?: string
  runningBinaryPath?: string
  installSource?: string
  platform?: string
  canRestart?: boolean
  restartHint?: string
  canInstall?: boolean
  installHint?: string
  canUninstall?: boolean
  uninstallHint?: string
  uninstallMode?: 'native' | 'docker-guide' | 'unsupported'
  uninstallState?: string
  uninstallPhase?: string
  uninstallError?: string
  uninstallFailures?: string[]
  uninstallWarnings?: string[]
  uninstallCanRetry?: boolean
  dockerUninstallCommands?: Array<{
    id?: string
    command?: string
  }>
  lastUpdateLogPath?: string
  lastUpdateError?: string
  updateTask?: PanelManagedUpdateTask
}

type PanelUpdateLogView = {
  path?: string
  exists?: boolean
  lines?: string[]
  tooLong?: boolean
  modified?: number
}

type TimeZoneOption = {
  title: string
  value: string
  props?: {
    disabled?: boolean
  }
}

type SystemTimeZoneStatus = {
  timeLocation?: string
  displayable?: boolean
  canModify?: boolean
  reason?: string
}

const panelStatusLoading = ref(false)
const panelRemoteLoading = ref(false)
const panelLoadingMoreVersions = ref(false)
const panelInstalling = ref(false)
const panelUninstalling = ref(false)
const panelVersionMenuVisible = ref(false)
const panelInstallDialogVisible = ref(false)
const panelRestartOverlay = ref(false)
const panelUninstallOverlay = ref(false)
const panelDockerUninstallDialogVisible = ref(false)
const panelUpdateLogDialogVisible = ref(false)
const panelUpdateLogLoading = ref(false)
const panelUpdateStatus = ref<PanelUpdateStatus | null>(null)
const panelUpdateLog = ref<PanelUpdateLogView | null>(null)
const panelSelectedVersion = ref('')
const panelVersionItems = ref<PanelVersionItem[]>([])
const panelHasMoreVersions = ref(false)
const panelAllVersionsLoaded = ref(false)
const panelUpdateFeedback = ref('')
const panelUpdateFeedbackType = ref<'success' | 'error' | 'info' | 'warning'>('info')
let panelVersionsRequest: Promise<void> | null = null
const panelReconnectTimerId = ref<number | null>(null)
const panelUninstallPollTimerId = ref<number | null>(null)
const panelUpdateTaskPollTimerId = ref<number | null>(null)
const panelUpdateStopRequestPending = ref(false)
let panelUpdateTaskRequest: Promise<void> | null = null

const settings = ref<Record<string, string>>({
  webListen: '',
  webDomain: '',
  webPort: '8888',
  webPath: '/app/',
  webURI: '',
  panelAssignedCertificateRecordID: '0',
  panelAssignedCertificateRecordIDs: '[]',
  sessionMaxAge: '0',
  trafficAge: '30',
  timeLocation: 'UTC',
  subListen: '',
  subPort: '22780',
  subPath: '',
  subDomain: '',
  subAssignedCertificateRecordID: '0',
  subAssignedCertificateRecordIDs: '[]',
  subUpdates: '12',
  subEncode: 'false',
  subShowInfo: 'false',
  subURI: '',
  serverTlsStoreEnabled: 'true',
  serverTlsStore: 'chrome',
  clientTlsStoreEnabled: 'true',
  clientTlsStore: 'chrome',
  subJsonExt: '',
  subClashExt: '',
})
const systemTimeLocation = ref('')
const oldSystemTimeLocation = ref('')
const hiddenPanelTimeLocation = ref('')
const systemTimeZoneStatus = ref<SystemTimeZoneStatus>({})
const systemTimeZoneLoadState = ref<SystemTimeZoneLoadState>('idle')
const systemTimeZoneLoadError = ref('')

const DEFAULT_WEB_PORT = '8888'
const DEFAULT_SUB_PORT = '22780'
const DEFAULT_TIME_LOCATION = 'UTC'
const SETTINGS_SAVE_KEYS = [
  'webListen',
  'webDomain',
  'webPort',
  'webPath',
  'webURI',
  'sessionMaxAge',
  'trafficAge',
  'timeLocation',
  'subListen',
  'subPort',
  'subPath',
  'subDomain',
  'subUpdates',
  'subEncode',
  'subShowInfo',
  'subURI',
  'serverTlsStoreEnabled',
  'serverTlsStore',
  'clientTlsStoreEnabled',
  'clientTlsStore',
  'subJsonExt',
  'subClashExt',
] as const

const timeZoneHeaderKeys: Record<string, string> = {
  '推荐国家 / 地区': 'setting.timeZoneRecommended',
  '亚洲': 'setting.timeZoneAsia',
  '欧洲': 'setting.timeZoneEurope',
  '非洲': 'setting.timeZoneAfrica',
  '美洲': 'setting.timeZoneAmericas',
  '大洋洲': 'setting.timeZoneOceania',
}

const createTimeZoneHeader = (title: string): TimeZoneOption => ({
  title: i18n.global.t(timeZoneHeaderKeys[title] || title),
  value: `__header__${title}`,
  props: {
    disabled: true,
  },
})

const createTimeZoneOption = (value: string, _label?: string): TimeZoneOption => ({
  title: value,
  value,
})

const timeZoneOptions = computed<TimeZoneOption[]>(() => {
  // Make the grouped labels reactive to language changes. IANA identifiers are
  // intentionally kept stable so operators can compare them with host logs.
  void locale.current.value
  return [
  createTimeZoneHeader('推荐国家 / 地区'),
  createTimeZoneOption('UTC', '国际标准时间'),
  createTimeZoneOption('Asia/Shanghai', '中国'),
  createTimeZoneOption('Asia/Hong_Kong', '中国香港'),
  createTimeZoneOption('Asia/Taipei', '中国台湾'),
  createTimeZoneOption('Asia/Tokyo', '日本'),
  createTimeZoneOption('Asia/Seoul', '韩国'),
  createTimeZoneOption('Asia/Singapore', '新加坡'),
  createTimeZoneOption('Asia/Bangkok', '泰国'),
  createTimeZoneOption('Asia/Ho_Chi_Minh', '越南'),
  createTimeZoneOption('Asia/Kuala_Lumpur', '马来西亚'),
  createTimeZoneOption('Asia/Jakarta', '印度尼西亚'),
  createTimeZoneOption('Asia/Manila', '菲律宾'),
  createTimeZoneOption('Asia/Kolkata', '印度'),
  createTimeZoneOption('Asia/Karachi', '巴基斯坦'),
  createTimeZoneOption('Asia/Dhaka', '孟加拉国'),
  createTimeZoneOption('Asia/Dubai', '阿联酋'),
  createTimeZoneOption('Asia/Riyadh', '沙特阿拉伯'),
  createTimeZoneOption('Asia/Tehran', '伊朗'),
  createTimeZoneOption('Asia/Jerusalem', '以色列'),
  createTimeZoneOption('Europe/London', '英国'),
  createTimeZoneOption('Europe/Paris', '法国'),
  createTimeZoneOption('Europe/Berlin', '德国'),
  createTimeZoneOption('Europe/Moscow', '俄罗斯'),
  createTimeZoneOption('Europe/Istanbul', '土耳其'),
  createTimeZoneOption('Africa/Cairo', '埃及'),
  createTimeZoneOption('Africa/Johannesburg', '南非'),
  createTimeZoneOption('America/New_York', '美国东部'),
  createTimeZoneOption('America/Chicago', '美国中部'),
  createTimeZoneOption('America/Denver', '美国山地'),
  createTimeZoneOption('America/Los_Angeles', '美国西部'),
  createTimeZoneOption('America/Toronto', '加拿大东部'),
  createTimeZoneOption('America/Vancouver', '加拿大西部'),
  createTimeZoneOption('America/Mexico_City', '墨西哥'),
  createTimeZoneOption('America/Sao_Paulo', '巴西'),
  createTimeZoneOption('America/Argentina/Buenos_Aires', '阿根廷'),
  createTimeZoneOption('Australia/Sydney', '澳大利亚悉尼'),
  createTimeZoneOption('Australia/Perth', '澳大利亚珀斯'),
  createTimeZoneOption('Pacific/Auckland', '新西兰'),
  createTimeZoneHeader('亚洲'),
  createTimeZoneOption('Asia/Kathmandu', '尼泊尔'),
  createTimeZoneOption('Asia/Almaty', '哈萨克斯坦'),
  createTimeZoneOption('Asia/Tashkent', '乌兹别克斯坦'),
  createTimeZoneHeader('欧洲'),
  createTimeZoneOption('Europe/Dublin', '爱尔兰'),
  createTimeZoneOption('Europe/Lisbon', '葡萄牙'),
  createTimeZoneOption('Europe/Madrid', '西班牙'),
  createTimeZoneOption('Europe/Brussels', '比利时'),
  createTimeZoneOption('Europe/Amsterdam', '荷兰'),
  createTimeZoneOption('Europe/Zurich', '瑞士'),
  createTimeZoneOption('Europe/Rome', '意大利'),
  createTimeZoneOption('Europe/Vienna', '奥地利'),
  createTimeZoneOption('Europe/Prague', '捷克'),
  createTimeZoneOption('Europe/Warsaw', '波兰'),
  createTimeZoneOption('Europe/Stockholm', '瑞典'),
  createTimeZoneOption('Europe/Oslo', '挪威'),
  createTimeZoneOption('Europe/Helsinki', '芬兰'),
  createTimeZoneOption('Europe/Athens', '希腊'),
  createTimeZoneOption('Europe/Bucharest', '罗马尼亚'),
  createTimeZoneOption('Europe/Kyiv', '乌克兰'),
  createTimeZoneHeader('非洲'),
  createTimeZoneOption('Africa/Casablanca', '摩洛哥'),
  createTimeZoneOption('Africa/Lagos', '尼日利亚'),
  createTimeZoneOption('Africa/Nairobi', '肯尼亚'),
  createTimeZoneHeader('美洲'),
  createTimeZoneOption('America/Anchorage', '美国阿拉斯加'),
  createTimeZoneOption('Pacific/Honolulu', '美国夏威夷'),
  createTimeZoneOption('America/Bogota', '哥伦比亚'),
  createTimeZoneOption('America/Lima', '秘鲁'),
  createTimeZoneOption('America/Santiago', '智利'),
  createTimeZoneOption('America/Caracas', '委内瑞拉'),
  createTimeZoneOption('America/Montevideo', '乌拉圭'),
  createTimeZoneHeader('大洋洲'),
  createTimeZoneOption('Australia/Melbourne', '澳大利亚墨尔本'),
  createTimeZoneOption('Australia/Brisbane', '澳大利亚布里斯班'),
  createTimeZoneOption('Pacific/Fiji', '斐济'),
  createTimeZoneOption('Pacific/Guam', '关岛'),
  ]
})

const validTimeZoneValues = computed(() => new Set(timeZoneOptions.value.filter(item => !item.props?.disabled).map(item => item.value)))

const normalizeTimeLocationValue = (value: unknown) => {
  const trimmed = String(value ?? '').trim()
  if (validTimeZoneValues.value.has(trimmed)) return trimmed
  return ''
}

const changeLocale = (l: any) => {
  locale.current.value = l ?? 'en'
  localStorage.setItem('locale', locale.current.value)
}

const delaySettingsRetry = () => new Promise<void>(resolve => window.setTimeout(resolve, 600))

const isSettingsPayload = (value: unknown): value is Record<string, any> => {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

const isVerifiedSettingsPayload = (value: unknown): value is Record<string, any> => {
  if (!isSettingsPayload(value)) return false
  return ['webPort', 'webPath', 'subPort', 'subPath', 'timeLocation']
    .every(key => typeof value[key] === 'string')
}

const isVerifiedSettingsSnapshot = (value: unknown): value is SettingsSnapshot => {
	if (!isSettingsPayload(value)) return false
	const snapshot = value as Record<string, any>
	return Number.isInteger(snapshot.revision)
	  && snapshot.revision >= 1
	  && isVerifiedSettingsPayload(snapshot.values)
	  && typeof snapshot.extensionsIncluded === 'boolean'
	  && isSettingsPayload(snapshot.defaults)
	  && typeof snapshot.defaults.jsonExt === 'string'
	  && typeof snapshot.defaults.clashExt === 'string'
	  && isSettingsPayload(snapshot.ruleSetSources)
	  && Array.isArray(snapshot.ruleSetSources.json)
	  && Array.isArray(snapshot.ruleSetSources.clash)
}

const isSystemTimeZonePayload = (value: unknown): value is SystemTimeZoneStatus => {
  return isSettingsPayload(value)
    && typeof value.canModify === 'boolean'
    && typeof value.displayable === 'boolean'
}

const loadSettingsWithRetry = async (): Promise<Msg> => {
  let lastMsg: Msg = { success: false, msg: '', obj: null }
  for (let attempt = 0; attempt < 2; attempt += 1) {
	const msg = await HttpUtils.get('api/settings-snapshot?includeExtensions=false', {}, { timeout: 15000, silentErrorToast: true })
	if (msg.success && isVerifiedSettingsSnapshot(msg.obj)) {
      return msg
    }
    lastMsg = msg.success
      ? { success: false, msg: i18n.global.t('setting.settingsPayloadInvalid'), obj: null }
      : msg
    if (attempt === 0) {
      await delaySettingsRetry()
    }
  }
  return lastMsg
}

const loadData = async (): Promise<boolean> => {
  const requestSequence = ++settingsLoadRequestSequence
  settingsLoadState.value = 'loading'
  settingsLoadError.value = ''
  loading.value = true
  try {
    const msg = await loadSettingsWithRetry()
    if (requestSequence !== settingsLoadRequestSequence) return false
	if (!msg.success || !isVerifiedSettingsSnapshot(msg.obj)) {
      settingsLoadState.value = 'error'
      settingsLoadError.value = String(msg.msg || i18n.global.t('setting.settingsLoadFallback'))
      return false
    }

	setData(msg.obj)
    settingsLoadState.value = 'ready'
    void loadSystemTimeZone(requestSequence)
    return true
  } finally {
    if (requestSequence === settingsLoadRequestSequence) {
      loading.value = false
    }
  }
}

const loadSettingsAndPanelStatus = async () => {
  const loaded = await loadData()
  if (loaded) {
    void loadPanelUpdateStatus()
  }
}

const retryLoadData = () => {
  void loadSettingsAndPanelStatus()
}

const setData = (snapshot: SettingsSnapshot) => {
	const data = snapshot.values
  const rawPanelTimeLocation = String(data?.timeLocation ?? '').trim()
  const panelTimeLocation = normalizeTimeLocationValue(rawPanelTimeLocation)
  // 有效但不在固定列表中的数据库时区仍由后端使用；界面必须保持为空，
  // 同时保存其它设置时不能意外把它覆盖为 UTC。
  hiddenPanelTimeLocation.value = panelTimeLocation === '' ? rawPanelTimeLocation : ''
  const normalized = {
    ...data,
    timeLocation: panelTimeLocation,
  }
	settings.value = {
	  ...normalized,
	  subJsonExt: '',
	  subClashExt: '',
	}
  oldSettings.value = { ...settings.value }
	settingsRevision.value = snapshot.revision
	settingsDefaults.value = { ...snapshot.defaults }
	ruleSetSources.value = {
	  json: [...snapshot.ruleSetSources.json],
	  clash: [...snapshot.ruleSetSources.clash],
	}
	subJsonDraftValue.value = ''
	subClashDraftValue.value = ''
	subJsonDraftSettings.value = {}
	subClashDraftSettings.value = {}
	subJsonDraftDirty.value = false
	subClashDraftDirty.value = false
	subJsonDraftError.value = ''
	subClashDraftError.value = ''
	subJsonDraftLoadState.value = 'idle'
	subClashDraftLoadState.value = 'idle'
	subJsonDraftLoadError.value = ''
	subClashDraftLoadError.value = ''
	subscriptionDraftLoadRequestSequence.json += 1
	subscriptionDraftLoadRequestSequence.clash += 1
	subJsonResetPending.value = false
	subClashResetPending.value = false
	subscriptionDraftGeneration.value += 1
}

const isSubscriptionSettingsSnapshot = (value: unknown, kind: 'json' | 'clash'): value is SubscriptionSettingsSnapshot => {
	if (!isSettingsPayload(value)) return false
	const snapshot = value as Record<string, any>
	return Number.isInteger(snapshot.revision)
	  && snapshot.revision >= 1
	  && snapshot.kind === kind
	  && typeof snapshot.value === 'string'
	  && typeof snapshot.default === 'string'
	  && Array.isArray(snapshot.ruleSetSources)
}

const setSubscriptionDraftLoadState = (target: 'json' | 'clash', state: SubscriptionDraftLoadState, error = '') => {
	if (target === 'json') {
		subJsonDraftLoadState.value = state
		subJsonDraftLoadError.value = error
		return
	}
	subClashDraftLoadState.value = state
	subClashDraftLoadError.value = error
}

const isSubscriptionDraftDirty = (target: 'json' | 'clash') => target === 'json'
	? subJsonDraftDirty.value
	: subClashDraftDirty.value

const loadSubscriptionDraft = async (target: 'json' | 'clash', retryAfterRevisionRefresh = true): Promise<boolean> => {
	const currentState = target === 'json' ? subJsonDraftLoadState.value : subClashDraftLoadState.value
	if (currentState === 'loading' || currentState === 'ready') return currentState === 'ready'

	const requestSequence = ++subscriptionDraftLoadRequestSequence[target]
	setSubscriptionDraftLoadState(target, 'loading')
	const msg = await HttpUtils.get(`api/subscription-settings-snapshot?kind=${target}`, {}, { timeout: 15000, silentErrorToast: true })
	if (requestSequence !== subscriptionDraftLoadRequestSequence[target]) return false
	if (!msg.success || !isSubscriptionSettingsSnapshot(msg.obj, target)) {
		setSubscriptionDraftLoadState(target, 'error', String(msg.msg || i18n.global.t('setting.subscriptionExtensionLoadFailed')))
		return false
	}

	const snapshot = msg.obj
	if (snapshot.revision !== settingsRevision.value) {
		if (retryAfterRevisionRefresh && !stateChange.value && !isSubscriptionDraftDirty(target)) {
			const reloaded = await loadData()
			if (reloaded) return loadSubscriptionDraft(target, false)
		}
		setSubscriptionDraftLoadState(target, 'error', i18n.global.t('setting.subscriptionExtensionRevisionChanged'))
		return false
	}

	// Empty extension text is the persisted first-installation state. Do not
	// replace it with the current code template, or a reset cannot reproduce
	// the page and editor state from the first installation.
	const value = snapshot.value
	if (target === 'json') {
		settingsDefaults.value.jsonExt = snapshot.default
		ruleSetSources.value.json = [...snapshot.ruleSetSources]
		subJsonDraftValue.value = snapshot.value
		subJsonDraftSettings.value = { ...settings.value, subJsonExt: value }
		subJsonDraftDirty.value = false
		subJsonDraftError.value = ''
		subJsonResetPending.value = false
	} else {
		settingsDefaults.value.clashExt = snapshot.default
		ruleSetSources.value.clash = [...snapshot.ruleSetSources]
		subClashDraftValue.value = snapshot.value
		subClashDraftSettings.value = { subClashExt: value }
		subClashDraftDirty.value = false
		subClashDraftError.value = ''
		subClashResetPending.value = false
	}
	setSubscriptionDraftLoadState(target, 'ready')
	subscriptionDraftGeneration.value += 1
	return true
}

const retrySubscriptionDraft = (target: 'json' | 'clash') => {
	setSubscriptionDraftLoadState(target, 'idle')
	void loadSubscriptionDraft(target)
}

const isSubscriptionInitialResetResult = (value: unknown, kind: 'json' | 'clash'): value is SubscriptionInitialResetResult => {
	if (!isSettingsPayload(value)) return false
	const result = value as Record<string, any>
	if (!Number.isInteger(result.revision) || result.revision < 1 || result.kind !== kind || !Array.isArray(result.changedKeys) || !isSettingsPayload(result.values)) {
		return false
	}
	const requiredKeys = kind === 'json'
		? ['subJsonExt', 'serverTlsStoreEnabled', 'serverTlsStore', 'clientTlsStoreEnabled', 'clientTlsStore']
		: ['subClashExt']
	return requiredKeys.every(key => typeof result.values[key] === 'string')
}

const applySubscriptionInitialReset = (target: 'json' | 'clash', result: SubscriptionInitialResetResult) => {
	settingsRevision.value = result.revision
	if (target === 'json') {
		for (const key of ['serverTlsStoreEnabled', 'serverTlsStore', 'clientTlsStoreEnabled', 'clientTlsStore']) {
			const value = String(result.values[key])
			settings.value[key] = value
			oldSettings.value[key] = value
		}
		const value = String(result.values.subJsonExt)
		subJsonDraftValue.value = value
		subJsonDraftSettings.value = { ...settings.value, subJsonExt: value }
		subJsonDraftDirty.value = false
		subJsonDraftError.value = ''
		subJsonResetPending.value = false
		setSubscriptionDraftLoadState('json', 'ready')
	} else {
		const value = String(result.values.subClashExt)
		subClashDraftValue.value = value
		subClashDraftSettings.value = { subClashExt: value }
		subClashDraftDirty.value = false
		subClashDraftError.value = ''
		subClashResetPending.value = false
		setSubscriptionDraftLoadState('clash', 'ready')
	}
	subscriptionDraftGeneration.value += 1
}

const loadSystemTimeZone = async (settingsRequestSequence?: number): Promise<boolean> => {
  const requestSequence = ++systemTimeZoneRequestSequence
  const isCurrentRequest = () => requestSequence === systemTimeZoneRequestSequence
    && (settingsRequestSequence === undefined || settingsRequestSequence === settingsLoadRequestSequence)

  if (isCurrentRequest()) {
    systemTimeZoneLoadState.value = 'loading'
    systemTimeZoneLoadError.value = ''
  }

  let lastMsg: Msg = { success: false, msg: '', obj: null }
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const msg = await HttpUtils.get('api/system-timezone', {}, { timeout: 8000, silentErrorToast: true })
    if (msg.success && isSystemTimeZonePayload(msg.obj)) {
      if (isCurrentRequest()) {
        const status = msg.obj as SystemTimeZoneStatus
        systemTimeZoneStatus.value = status
        const visible = status.canModify === true && status.displayable === true
          ? normalizeTimeLocationValue(status.timeLocation)
          : ''
        systemTimeLocation.value = visible
        oldSystemTimeLocation.value = visible
        systemTimeZoneLoadState.value = 'ready'
      }
      return true
    }
    lastMsg = msg.success
      ? { success: false, msg: i18n.global.t('setting.systemTimePayloadInvalid'), obj: null }
      : msg
    if (attempt === 0) {
      await delaySettingsRetry()
    }
  }

  if (isCurrentRequest()) {
    systemTimeZoneLoadState.value = 'error'
    systemTimeZoneLoadError.value = String(lastMsg.msg || i18n.global.t('setting.requestFailed'))
    systemTimeZoneStatus.value = {}
    systemTimeLocation.value = ''
    oldSystemTimeLocation.value = ''
  }
  return false
}

const retryLoadSystemTimeZone = () => {
  void loadSystemTimeZone(settingsLoadRequestSequence)
}

onMounted(() => {
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handlePanelUpdateTaskVisibilityChange)
  }
  void loadSettingsAndPanelStatus()
})

const onSystemTimeLocationSelected = () => {
  if (systemTimeZoneStatus.value.canModify === true) return
  push.warning({
    title: i18n.global.t('failed'),
    duration: 5000,
    message: systemTimeZoneStatus.value.reason || i18n.global.t('setting.systemTimePermissionDenied'),
  })
}

const panelLocalVersionLabel = computed(() => {
  const version = String(panelUpdateStatus.value?.localVersion ?? '').trim()
  return version ? `v${version.replace(/^v/i, '')}` : i18n.global.t('setting.panelUnknown')
})

const panelBinaryName = computed(() => String(panelUpdateStatus.value?.binaryName ?? '').trim())
const panelCanRestart = computed(() => panelUpdateStatus.value?.canRestart === true)
const panelRestartHint = computed(() => String(panelUpdateStatus.value?.restartHint ?? i18n.global.t('setting.restartStatusLoading')).trim())
const panelCanInstall = computed(() => panelUpdateStatus.value?.canInstall === true)
const panelInstallHint = computed(() => String(panelUpdateStatus.value?.installHint ?? '').trim())
const panelUninstallMode = computed(() => String(panelUpdateStatus.value?.uninstallMode ?? '').trim())
const panelCanUninstall = computed(() => panelUninstallMode.value === 'native' && panelUpdateStatus.value?.canUninstall === true)
const panelUninstallHint = computed(() => String(panelUpdateStatus.value?.uninstallHint ?? '').trim())
const panelUninstallState = computed(() => String(panelUpdateStatus.value?.uninstallState ?? '').trim())
const panelUninstallPhase = computed(() => String(panelUpdateStatus.value?.uninstallPhase ?? '').trim())
const panelUninstallError = computed(() => String(panelUpdateStatus.value?.uninstallError ?? '').trim())
const panelUninstallFailures = computed(() => Array.isArray(panelUpdateStatus.value?.uninstallFailures)
  ? panelUpdateStatus.value!.uninstallFailures!.map(value => String(value).trim()).filter(Boolean)
  : [])
const panelUninstallWarnings = computed(() => Array.isArray(panelUpdateStatus.value?.uninstallWarnings)
  ? panelUpdateStatus.value!.uninstallWarnings!.map(value => String(value).trim()).filter(Boolean)
  : [])
const panelUninstallCanRetry = computed(() => panelUpdateStatus.value?.uninstallCanRetry === true)
const panelDockerUninstallCommands = computed(() => Array.isArray(panelUpdateStatus.value?.dockerUninstallCommands)
  ? panelUpdateStatus.value!.dockerUninstallCommands!.filter(item => String(item?.command ?? '').trim() !== '')
  : [])
const panelHasDockerUninstallGuide = computed(() => panelUninstallMode.value === 'docker-guide' && panelDockerUninstallCommands.value.length > 0)
const panelCanUseUninstallAction = computed(() => panelCanUninstall.value || panelHasDockerUninstallGuide.value)
const panelUninstallFailed = computed(() => panelUninstallState.value === 'failed')
const panelUninstallButtonText = computed(() => panelHasDockerUninstallGuide.value
  ? i18n.global.t('setting.uninstallDockerGuide')
  : i18n.global.t('setting.uninstallPanel'))
const panelManagedUpdateTask = computed(() => panelUpdateStatus.value?.updateTask ?? null)
const panelUpdateTaskActive = computed(() => {
  const state = String(panelManagedUpdateTask.value?.state ?? '').trim().toLowerCase()
  return state === 'queued' || state === 'running' || state === 'stopping'
})
const panelUpdateTaskStopping = computed(() => (
  panelUpdateTaskActive.value && (
    panelManagedUpdateTask.value?.stopRequested === true
    || String(panelManagedUpdateTask.value?.state ?? '').trim().toLowerCase() === 'stopping'
  )
))
const panelUpdateTaskCanCancel = computed(() => (
  panelUpdateTaskActive.value
  && panelManagedUpdateTask.value?.canCancel === true
  && !panelUpdateTaskStopping.value
))
const panelUpdateTaskApplying = computed(() => (
  panelUpdateTaskActive.value
  && !panelUpdateTaskStopping.value
  && panelManagedUpdateTask.value?.canCancel === false
))
const panelUpdateTaskButtonText = computed(() => {
  if (panelUpdateStopRequestPending.value || panelUpdateTaskStopping.value) return '正在停止'
  if (panelUpdateTaskActive.value) {
    return panelUpdateTaskCanCancel.value ? '停止' : '正在应用'
  }
  return i18n.global.t('setting.panelInstall')
})
const panelUpdateTaskAlertType = computed<'info' | 'success' | 'warning' | 'error'>(() => {
  const state = String(panelManagedUpdateTask.value?.state ?? '').trim().toLowerCase()
  if (state === 'success') return 'success'
  if (state === 'error') return 'error'
  if (state === 'cancelled' || state === 'timed_out') return 'warning'
  return 'info'
})
const panelUpdateTaskStatusText = computed(() => {
  const task = panelManagedUpdateTask.value
  if (task == null) return ''
  const state = task.state.trim().toLowerCase()
  const phase = task.phase.trim()
  if (panelUpdateTaskStopping.value) return '正在停止面板更新任务'
  if (panelUpdateTaskActive.value && !task.canCancel) return phase ? `正在应用：${phase}` : '正在应用面板更新'
  if (panelUpdateTaskActive.value) return phase ? `正在准备更新：${phase}` : '正在准备面板更新'
  if (state === 'success') return phase === 'handoff' ? '更新 worker 已接手，面板将自动重启' : '面板更新准备已完成'
  if (state === 'cancelled') return '面板更新已停止'
  if (state === 'timed_out') return '面板更新准备超时，已停止并清理临时文件'
  if (state === 'error') return task.error ? `面板更新失败：${task.error}` : '面板更新失败'
  return phase || '面板更新状态未知'
})
const panelLifecycleBusy = computed(() => panelInstalling.value || panelUninstalling.value || panelUpdateTaskActive.value)

const panelRemoteVersionLabel = computed(() => {
  if (panelRemoteLoading.value) return i18n.global.t('setting.panelLoading')
  if (panelVersionItems.value.length > 0) return panelVersionItems.value[0].value
  return i18n.global.t('setting.panelNotLoaded')
})

const panelLastUpdateError = computed(() => String(panelUpdateStatus.value?.lastUpdateError ?? '').trim())
const panelVersionNoDataText = computed(() => {
  if (panelRemoteLoading.value) return i18n.global.t('setting.panelVersionsLoading')
  if (panelVersionItems.value.length > 0) return i18n.global.t('setting.panelNoMoreVersions')
  return i18n.global.t('setting.panelOpenToLoad')
})
const panelUpdateLogContent = computed(() => {
  const lines = Array.isArray(panelUpdateLog.value?.lines) ? panelUpdateLog.value?.lines : []
  return lines.length > 0 ? lines.join('\n') : i18n.global.t('setting.panelNoLogs')
})
const panelUpdateLogModifiedText = computed(() => {
  const unix = Number(panelUpdateLog.value?.modified ?? 0)
  if (!Number.isFinite(unix) || unix <= 0) return ''
  return formatPanelDateTime(unix * 1000)
})

const normalizePanelVersionTag = (value: string) => {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return ''
  return trimmed.startsWith('v') ? trimmed : `v${trimmed}`
}

const describePanelUninstallFailure = (status: PanelUpdateStatus | null) => {
  const phase = String(status?.uninstallPhase ?? '').trim()
  const error = String(status?.uninstallError ?? '').trim()
  const failures = Array.isArray(status?.uninstallFailures)
    ? status!.uninstallFailures!.map(value => String(value).trim()).filter(Boolean)
    : []
  const reason = error || failures[0] || i18n.global.t('setting.uninstallPanelStartFailed')
  const prefix = i18n.global.t('setting.uninstallPanelFailed')
  return phase ? `${prefix} (${phase})：${reason}` : `${prefix}：${reason}`
}

const normalizePanelManagedUpdateTask = (raw: any): PanelManagedUpdateTask => ({
  id: String(raw?.id ?? '').trim(),
  state: String(raw?.state ?? '').trim().toLowerCase() || 'idle',
  phase: String(raw?.phase ?? '').trim(),
  canCancel: raw?.canCancel === true,
  stopRequested: raw?.stopRequested === true,
  deadlineExceeded: raw?.deadlineExceeded === true,
  startedAt: Number(raw?.startedAt) || 0,
  updatedAt: Number(raw?.updatedAt) || 0,
  deadlineAt: Number(raw?.deadlineAt) || 0,
  finishedAt: Number(raw?.finishedAt) || 0,
  error: String(raw?.error ?? '').trim(),
})

const normalizePanelUpdateStatus = (raw: any): PanelUpdateStatus | null => {
  if (raw == null || typeof raw !== 'object') return null
  const updateTask = raw.updateTask == null ? undefined : normalizePanelManagedUpdateTask(raw.updateTask)
  return { ...raw, updateTask }
}

const isPanelUpdatePollingAllowed = () => (
  tab.value === 't1'
  && (typeof document === 'undefined' || document.visibilityState === 'visible')
)

const clearPanelUpdateTaskPolling = () => {
  if (panelUpdateTaskPollTimerId.value !== null) {
    window.clearTimeout(panelUpdateTaskPollTimerId.value)
    panelUpdateTaskPollTimerId.value = null
  }
}

const schedulePanelUpdateTaskPolling = () => {
  clearPanelUpdateTaskPolling()
  if (!panelUpdateTaskActive.value || !isPanelUpdatePollingAllowed()) return
  panelUpdateTaskPollTimerId.value = window.setTimeout(() => {
    void pollPanelUpdateTask()
  }, 1200)
}

const handlePanelUpdateTaskTerminal = () => {
  const task = panelManagedUpdateTask.value
  if (task == null || panelUpdateTaskActive.value) return
  panelUpdateStopRequestPending.value = false
  panelInstalling.value = false
  clearPanelUpdateTaskPolling()
  // A page can reload after the updater accepts the handoff but before this
  // panel process exits. The terminal handoff snapshot itself is sufficient to
  // resume reconnect polling; a new panel process has no such in-memory task.
  const shouldReconnect = task.state === 'success' && task.phase === 'handoff'
  if (shouldReconnect) {
    startPanelReconnectPolling()
  }
}

const applyPanelUpdateStatus = (raw: any) => {
  panelUpdateStatus.value = normalizePanelUpdateStatus(raw)
}

const loadPanelUpdateStatus = async () => {
  panelStatusLoading.value = true
  try {
    const msg = await HttpUtils.get('api/panel-update-status', {}, { silentAuthCheck: true })
    if (msg.success) {
      applyPanelUpdateStatus(msg.obj)
      if (panelUninstallFailed.value) {
        panelUpdateFeedback.value = describePanelUninstallFailure(panelUpdateStatus.value)
        panelUpdateFeedbackType.value = 'error'
      } else if (panelLastUpdateError.value) {
    panelUpdateFeedback.value = `${i18n.global.t('setting.panelPreviousUpdateFailed')}：${panelLastUpdateError.value}`
        panelUpdateFeedbackType.value = 'warning'
      }
      if (panelUpdateTaskActive.value) {
        schedulePanelUpdateTaskPolling()
      } else {
        handlePanelUpdateTaskTerminal()
      }
    }
  } finally {
    panelStatusLoading.value = false
  }
}

const openPanelUpdateLogDialog = async () => {
  panelUpdateLogDialogVisible.value = true
  panelUpdateLogLoading.value = true
  try {
    const msg = await HttpUtils.get('api/panel-update-log', {}, { silentAuthCheck: true })
    if (msg.success) {
      panelUpdateLog.value = msg.obj ?? null
    } else {
      panelUpdateLog.value = {
      lines: [String(msg.msg || i18n.global.t('setting.panelLogReadFailed'))],
      }
    }
  } finally {
    panelUpdateLogLoading.value = false
  }
}

const buildPanelVersionItems = (versions: any[]): PanelVersionItem[] => {
  const items: PanelVersionItem[] = []
  versions.forEach(item => {
    const tagName = normalizePanelVersionTag(item?.tag_name ?? item?.tagName ?? '')
    if (!tagName) return
    items.push({
      title: tagName,
      value: tagName,
      tagName,
      name: item?.name ?? '',
      prerelease: item?.prerelease === true,
      publishedAt: item?.published_at ?? item?.publishedAt ?? '',
      assetName: item?.asset_name ?? item?.assetName ?? '',
      assetSize: item?.asset_size ?? item?.assetSize ?? 0,
    })
  })
  return items
}

const applyPanelVersionResponse = (obj: any, append: boolean) => {
  const nextItems = buildPanelVersionItems(Array.isArray(obj?.versions) ? obj.versions : [])
  const existing = append ? [...panelVersionItems.value] : []
  const seen = new Set(existing.map(item => item.value))
  nextItems.forEach(item => {
    if (!seen.has(item.value)) {
      existing.push(item)
      seen.add(item.value)
    }
  })
  panelVersionItems.value = existing
  panelHasMoreVersions.value = obj?.has_more === true || obj?.hasMore === true
  panelAllVersionsLoaded.value = append && !panelHasMoreVersions.value
  if (!append && panelVersionItems.value.length > 0) {
    panelSelectedVersion.value = panelVersionItems.value[0].value
  } else if (!panelSelectedVersion.value && panelVersionItems.value.length > 0) {
    panelSelectedVersion.value = panelVersionItems.value[0].value
  }
}

const loadPanelVersions = async (append = false) => {
  if (panelVersionsRequest) return panelVersionsRequest

  const request = (async () => {
    if (append) {
      panelLoadingMoreVersions.value = true
    } else {
      panelRemoteLoading.value = true
      panelAllVersionsLoaded.value = false
    }

    try {
      const msg = await HttpUtils.get('api/panel-update-versions', {
        offset: append ? panelVersionItems.value.length : 0,
        limit: 5,
      }, { silentAuthCheck: true })

      if (msg.success) {
        applyPanelVersionResponse(msg.obj, append)
        if (append) {
          panelUpdateFeedback.value = panelAllVersionsLoaded.value
            ? i18n.global.t('setting.panelNoMoreVersions')
            : i18n.global.t('setting.panelMoreLoaded')
          panelUpdateFeedbackType.value = panelAllVersionsLoaded.value ? 'info' : 'success'
          return
        }
        panelUpdateFeedback.value = i18n.global.t('setting.panelUpdateDone')
        panelUpdateFeedbackType.value = 'success'
      } else if (msg.msg) {
        panelUpdateFeedback.value = msg.msg
        panelUpdateFeedbackType.value = 'error'
      }
    } finally {
      if (append) {
        panelLoadingMoreVersions.value = false
      } else {
        panelRemoteLoading.value = false
      }
    }
  })()

  panelVersionsRequest = request
  try {
    await request
  } finally {
    if (panelVersionsRequest === request) {
      panelVersionsRequest = null
    }
  }
}

const checkPanelUpdates = async () => {
  await loadPanelVersions(false)
}

const ensurePanelVersionsLoaded = async () => {
  if (panelRemoteLoading.value || panelLoadingMoreVersions.value) return
  if (panelVersionItems.value.length > 0) return
  await loadPanelVersions(false)
}

const onPanelVersionMenuUpdate = (opened: boolean) => {
  if (!opened) return
  void ensurePanelVersionsLoaded()
}

const loadMorePanelVersions = async () => {
  if (panelAllVersionsLoaded.value) {
    panelUpdateFeedback.value = i18n.global.t('setting.panelNoMoreVersions')
    panelUpdateFeedbackType.value = 'info'
    return
  }
  await loadPanelVersions(true)
}

const openPanelInstallDialog = () => {
  if (panelUninstalling.value) return
  if (!panelCanInstall.value) {
    panelUpdateFeedback.value = panelInstallHint.value || i18n.global.t('setting.panelInstallUnsupported')
    panelUpdateFeedbackType.value = 'warning'
    return
  }
  if (!panelSelectedVersion.value) return
  panelInstallDialogVisible.value = true
}

const requestPanelUninstall = async () => {
  if (panelStatusLoading.value || loading.value || panelLifecycleBusy.value) return
  if (panelHasDockerUninstallGuide.value) {
    panelDockerUninstallDialogVisible.value = true
    return
  }
  if (!panelCanUninstall.value) return

  const confirmed = await confirm({
    severity: 'danger',
    title: i18n.global.t('setting.uninstallPanelConfirmTitle'),
    message: i18n.global.t('setting.uninstallPanelConfirm'),
    confirmText: i18n.global.t('confirmDialog.actions.uninstall'),
  })
  if (!confirmed || panelStatusLoading.value || loading.value || panelLifecycleBusy.value || !panelCanUninstall.value) return

  panelUninstalling.value = true
  let accepted = false
  try {
    const msg = await HttpUtils.post('api/panel-uninstall', {}, { silentAuthCheck: true, timeout: 10000 })
    if (msg.success) {
      accepted = true
      panelUninstallOverlay.value = true
      startPanelUninstallStatusPolling()
      return
    }
    push.warning({
      title: i18n.global.t('failed'),
      duration: 6000,
      message: msg.msg || i18n.global.t('setting.uninstallPanelStartFailed'),
    })
  } finally {
    if (!accepted) {
      panelUninstalling.value = false
    }
  }
}

const panelDockerUninstallCommandLabel = (id?: string) => id === 'compose'
  ? i18n.global.t('setting.uninstallDockerCompose')
  : i18n.global.t('setting.uninstallDockerRun')

const copyDockerUninstallCommand = async (command?: string) => {
  const text = String(command ?? '').trim()
  if (!text) return
  try {
    if (navigator.clipboard?.writeText && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.setAttribute('readonly', '')
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      const copied = document.execCommand('copy')
      document.body.removeChild(textarea)
      if (!copied) throw new Error('copy command was rejected')
    }
    push.success({
      title: i18n.global.t('success'),
      duration: 3000,
      message: i18n.global.t('copyToClipboard'),
    })
  } catch {
    push.error({
      title: i18n.global.t('failed'),
      duration: 5000,
      message: i18n.global.t('copyToClipboard'),
    })
  }
}

const clearPanelReconnectTimer = () => {
  if (panelReconnectTimerId.value !== null) {
    window.clearTimeout(panelReconnectTimerId.value)
    panelReconnectTimerId.value = null
  }
}

const clearPanelUninstallStatusTimer = () => {
  if (panelUninstallPollTimerId.value !== null) {
    window.clearTimeout(panelUninstallPollTimerId.value)
    panelUninstallPollTimerId.value = null
  }
}

const pollPanelUpdateTask = async (): Promise<void> => {
  if (panelUpdateTaskRequest) return panelUpdateTaskRequest
  if (!isPanelUpdatePollingAllowed()) return
  const request = (async () => {
    const msg = await HttpUtils.get('api/panel-update-status', {}, { silentAuthCheck: true })
    if (!msg.success) {
      schedulePanelUpdateTaskPolling()
      return
    }
    applyPanelUpdateStatus(msg.obj)
    if (panelUpdateTaskActive.value) {
      schedulePanelUpdateTaskPolling()
      return
    }
    handlePanelUpdateTaskTerminal()
  })()
  panelUpdateTaskRequest = request
  try {
    await request
  } finally {
    if (panelUpdateTaskRequest === request) {
      panelUpdateTaskRequest = null
    }
  }
}

const startPanelUpdateTaskPolling = () => {
	clearPanelUpdateTaskPolling()
	if (!panelUpdateTaskActive.value || !isPanelUpdatePollingAllowed()) return
	void pollPanelUpdateTask()
}

const handlePanelUpdateTaskVisibilityChange = () => {
  if (typeof document === 'undefined') return
  if (document.visibilityState !== 'visible') {
    clearPanelUpdateTaskPolling()
    return
  }
  if (tab.value === 't1') {
    void loadPanelUpdateStatus()
  }
}

const stopPanelUpdateTask = async () => {
  const task = panelManagedUpdateTask.value
  if (task == null || !panelUpdateTaskCanCancel.value || panelUpdateStopRequestPending.value) return
  panelUpdateStopRequestPending.value = true
  panelUpdateStatus.value = {
    ...(panelUpdateStatus.value ?? {}),
    updateTask: {
      ...task,
      state: 'stopping',
      phase: 'stopping',
      canCancel: false,
      stopRequested: true,
    },
  }
  try {
    const msg = await HttpUtils.post('api/panel-update-stop', { id: task.id }, { silentAuthCheck: true })
    if (msg.success && msg.obj) {
      panelUpdateStatus.value = {
        ...(panelUpdateStatus.value ?? {}),
        updateTask: normalizePanelManagedUpdateTask(msg.obj),
      }
    }
  } catch {
    // 由紧随其后的状态轮询确认停止请求是否已被后端受理。
  } finally {
    startPanelUpdateTaskPolling()
  }
}

const startPanelUninstallStatusPolling = () => {
  clearPanelUninstallStatusTimer()

  const poll = async () => {
    try {
      const msg = await HttpUtils.get('api/panel-update-status', {}, { silentAuthCheck: true })
      if (msg.success) {
        panelUpdateStatus.value = msg.obj ?? null
        if (panelUninstallFailed.value) {
          panelUninstallOverlay.value = false
          panelUninstalling.value = false
          panelUpdateFeedback.value = describePanelUninstallFailure(panelUpdateStatus.value)
          panelUpdateFeedbackType.value = 'error'
          clearPanelUninstallStatusTimer()
          return
        }
      }
    } catch {
      // 原生卸载成功后连接会中断；遮罩保持，避免已确认的操作被误判为失败。
    }
    panelUninstallPollTimerId.value = window.setTimeout(poll, 2000)
  }

  panelUninstallPollTimerId.value = window.setTimeout(poll, 1200)
}

const startPanelReconnectPolling = () => {
  clearPanelReconnectTimer()
  panelRestartOverlay.value = true

  const poll = async () => {
    try {
      const [sessionMsg, statusMsg] = await Promise.all([
        HttpUtils.get('api/session', {}, { silentAuthCheck: true }),
        HttpUtils.get('api/panel-update-status', {}, { silentAuthCheck: true }),
      ])

      if (!sessionMsg.success && sessionMsg.failureKind === 'api') {
        clearPanelReconnectTimer()
        panelRestartOverlay.value = false
        await router.replace('/login')
        return
      }

      if (sessionMsg.success && statusMsg.success) {
        const nextStatus = statusMsg.obj ?? null
        const nextVersion = String(nextStatus?.localVersion ?? '').trim().replace(/^v/i, '')
        const targetVersion = String(panelSelectedVersion.value ?? '').trim().replace(/^v/i, '')

        if (nextVersion && targetVersion && nextVersion === targetVersion) {
          window.location.reload()
          return
        }

        if (String(nextStatus?.lastUpdateError ?? '').trim()) {
          panelRestartOverlay.value = false
          panelUpdateStatus.value = nextStatus
          panelUpdateFeedback.value = `${i18n.global.t('setting.panelUpdateFailed')}：${String(nextStatus.lastUpdateError).trim()}`
          panelUpdateFeedbackType.value = 'error'
          clearPanelReconnectTimer()
          return
        }
      }
    } catch {
      // 等待面板恢复连接
    }

    panelReconnectTimerId.value = window.setTimeout(poll, 4000)
  }

  panelReconnectTimerId.value = window.setTimeout(poll, 6000)
}

const installPanelVersion = async () => {
  if (!panelSelectedVersion.value || panelUninstalling.value) return
  panelInstalling.value = true
  const version = panelSelectedVersion.value
  try {
    const msg = await HttpUtils.post('api/panel-update-install', { version }, { silentAuthCheck: true, timeout: 35000 })
    panelInstallDialogVisible.value = false

    if (msg.success && msg.obj) {
      const task = normalizePanelManagedUpdateTask(msg.obj)
      panelUpdateStatus.value = {
        ...(panelUpdateStatus.value ?? {}),
        updateTask: task,
      }
      panelUpdateFeedback.value = ''
      panelUpdateFeedbackType.value = 'info'
      startPanelUpdateTaskPolling()
      void loadPanelUpdateStatus()
    } else {
      await loadPanelUpdateStatus()
      if (!panelUpdateTaskActive.value) {
        panelUpdateFeedback.value = msg.msg || i18n.global.t('setting.panelInstallFailed')
        panelUpdateFeedbackType.value = 'error'
      }
    }
  } catch {
    await loadPanelUpdateStatus()
    if (!panelUpdateTaskActive.value) {
      panelUpdateFeedback.value = i18n.global.t('setting.panelInstallFailed')
      panelUpdateFeedbackType.value = 'error'
    }
  } finally {
    panelInstalling.value = false
  }
}

type SubscriptionSerializeResult = {
	ok: boolean
	dirty: boolean
	reset?: boolean
	value: string
	error?: string
}

const onSubJsonDirtyChange = (dirty: boolean) => {
	subJsonDraftDirty.value = dirty
}

const onSubClashDirtyChange = (dirty: boolean) => {
	subClashDraftDirty.value = dirty
}

const persistSubscriptionDraft = (target: 'json' | 'clash', requireValid = false): boolean => {
	const component = target === 'json' ? subJsonExtRef.value : subClashExtRef.value
	const alreadyDirty = target === 'json' ? subJsonDraftDirty.value : subClashDraftDirty.value
	if (!component) return !requireValid || !alreadyDirty
	if (!alreadyDirty && component.isDirty?.() !== true) return true

	const result = component.validateAndSerialize?.() as SubscriptionSerializeResult | undefined
	if (!result) return true
	if (target === 'json') {
	  subJsonDraftDirty.value = result.dirty === true
	  subJsonDraftValue.value = result.value
	  subJsonDraftError.value = result.ok ? '' : String(result.error || i18n.global.t('subscriptionEditor.validationFailed'))
	  subJsonResetPending.value = result.reset === true
	  if (!result.reset) subJsonDraftSettings.value.subJsonExt = result.value
	} else {
	  subClashDraftDirty.value = result.dirty === true
	  subClashDraftValue.value = result.value
	  subClashDraftError.value = result.ok ? '' : String(result.error || i18n.global.t('subscriptionEditor.validationFailed'))
	  subClashResetPending.value = result.reset === true
	  if (!result.reset) subClashDraftSettings.value.subClashExt = result.value
	}
	if (!result.ok && requireValid) {
	  push.error({
		title: i18n.global.t('failed'),
		duration: 5000,
		message: result.error || i18n.global.t('subscriptionEditor.validationFailed'),
	  })
	  return false
	}
	return true
}

watch(tab, (value, previous) => {
	if (previous === 't3') persistSubscriptionDraft('json')
	if (previous === 't4') persistSubscriptionDraft('clash')
	if (previous === 't1') clearPanelUpdateTaskPolling()
	if (value === 't3') void loadSubscriptionDraft('json')
	if (value === 't4') void loadSubscriptionDraft('clash')
	if (value === 't1') {
		void loadPanelUpdateStatus()
		if (panelUpdateTaskActive.value) startPanelUpdateTaskPolling()
	}
})

watch(() => settings.value.timeLocation, value => {
  if (validTimeZoneValues.value.has(String(value ?? '').trim())) {
    hiddenPanelTimeLocation.value = ''
  }
})

onBeforeUnmount(() => {
  const settingsRequestWasLoading = settingsLoadState.value === 'loading'
  settingsLoadRequestSequence += 1
  systemTimeZoneRequestSequence += 1
	subscriptionDraftLoadRequestSequence.json += 1
	subscriptionDraftLoadRequestSequence.clash += 1
	if (tab.value === 't3') persistSubscriptionDraft('json')
	if (tab.value === 't4') persistSubscriptionDraft('clash')
  clearPanelReconnectTimer()
  clearPanelUninstallStatusTimer()
  clearPanelUpdateTaskPolling()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handlePanelUpdateTaskVisibilityChange)
  }
  if (settingsRequestWasLoading) {
    loading.value = false
  }
})

const save = async () => {
  if (!hasVerifiedSettings.value || settingsLoadState.value === 'loading') return
  applyPortDefaultsBeforeSave()
  const previousSettings = { ...settings.value }
	if (tab.value === 't3' && !persistSubscriptionDraft('json', true)) return
	if (tab.value === 't4' && !persistSubscriptionDraft('clash', true)) return
	const inactiveDraftError = subJsonDraftDirty.value && subJsonDraftError.value
	  ? subJsonDraftError.value
	  : subClashDraftDirty.value && subClashDraftError.value
		? subClashDraftError.value
		: ''
	if (inactiveDraftError) {
	  push.error({
		title: i18n.global.t('failed'),
		duration: 5000,
		message: inactiveDraftError,
	  })
	  return
	}

	if (subJsonDraftDirty.value) {
	  for (const key of ['serverTlsStoreEnabled', 'serverTlsStore', 'clientTlsStoreEnabled', 'clientTlsStore']) {
		settings.value[key] = String(subJsonDraftSettings.value[key] ?? settings.value[key] ?? '')
	  }
	}
	const normalizedSettings = buildSettingsSavePayload(settings.value)
	const changes: Record<string, string> = {}
	for (const key of SETTINGS_SAVE_KEYS) {
	  if (key === 'subJsonExt' || key === 'subClashExt') continue
	  const nextValue = String(normalizedSettings[key] ?? '')
	  const previousValue = String(oldSettings.value[key] ?? '')
	  if (nextValue !== previousValue) changes[key] = nextValue
	}
	if (subJsonDraftDirty.value) changes.subJsonExt = subJsonDraftValue.value
	if (subClashDraftDirty.value) changes.subClashExt = subClashDraftValue.value
	const requestedSystemTimeLocation = systemTimeLocation.value !== oldSystemTimeLocation.value
	  ? systemTimeLocation.value.trim()
	  : ''
	if (Object.keys(changes).length === 0 && requestedSystemTimeLocation === '') return
	const clearingTrafficHistory = String(normalizedSettings.trafficAge ?? '').trim() === '0'
	  && String(oldSettings.value.trafficAge ?? '').trim() !== '0'
	if (clearingTrafficHistory) {
	  const confirmed = await confirm({
		severity: 'danger',
		title: i18n.global.t('setting.trafficHistoryClearConfirmTitle'),
		message: i18n.global.t('setting.trafficHistoryClearConfirm'),
		confirmText: i18n.global.t('subscriptionEditor.resetConfirm'),
	  })
	  if (!confirmed) return
	}
	const rotatingSubscriptionPath = Object.prototype.hasOwnProperty.call(changes, 'subPath')
	if (rotatingSubscriptionPath) {
	  const confirmed = await confirm({
		severity: 'warning',
		title: i18n.global.t('setting.subPathChangeConfirmTitle'),
		message: i18n.global.t('setting.subPathChangeConfirm'),
		confirmText: i18n.global.t('subscriptionEditor.resetConfirm'),
	  })
	  if (!confirmed) return
	}

  loading.value = true
  try {
	const msg = await HttpUtils.post('api/settings-patch', {
	  expectedRevision: settingsRevision.value,
	  changes,
	  systemTimeLocation: requestedSystemTimeLocation || undefined,
	  confirmTrafficHistoryClear: clearingTrafficHistory || undefined,
	}, {
	  headers: { 'Content-Type': 'application/json' },
	  timeout: 30000,
	  silentErrorToast: true,
	})
    if (msg.success) {
	  const warnings = Array.isArray(msg.obj?.warnings) ? msg.obj.warnings.map(String) : []
      push.success({
        title: i18n.global.t('success'),
        duration: 5000,
        message: i18n.global.t('actions.set') + ' ' + i18n.global.t('pages.settings'),
      })
	  if (warnings.length > 0) {
		push.warning({
		  title: i18n.global.t('subscriptionEditor.settingsSavedWithWarnings'),
		  duration: 8000,
		  message: warnings.join('; '),
		})
	  }
	  if (msg.obj?.maintenanceQueued === true) {
		push.info({
		  title: i18n.global.t('setting.maintenanceQueuedTitle'),
		  duration: 6000,
		  message: i18n.global.t('setting.maintenanceQueuedMessage'),
		})
	  }
	  await loadData()
	  if (tab.value === 't3') await loadSubscriptionDraft('json')
	  if (tab.value === 't4') await loadSubscriptionDraft('clash')
      await Promise.all([
        refreshPanelTimeContext(),
        loadSystemTimeZone(),
      ])
	  await maybeRedirectToHttps(settings.value, previousSettings)
	} else if (msg.obj?.code === 'revision_conflict') {
	  await loadData()
	  await loadSystemTimeZone()
	  push.warning({
		title: i18n.global.t('subscriptionEditor.revisionConflictTitle'),
		duration: 7000,
		message: i18n.global.t('subscriptionEditor.revisionConflictMessage'),
	  })
	} else {
	  push.error({
		title: i18n.global.t('failed'),
		duration: 6000,
		message: msg.msg || i18n.global.t('subscriptionEditor.settingsSaveFailed'),
	  })
    }
  } finally {
    loading.value = false
  }
}

const currentResetTarget = computed<'json' | 'clash' | ''>(() => {
  if (tab.value === 't3' && subJsonDraftLoadState.value === 'ready') return 'json'
  if (tab.value === 't4' && subClashDraftLoadState.value === 'ready') return 'clash'
  return ''
})

const showSubPageResetButton = computed(() => currentResetTarget.value !== '')

const resetButtonText = computed(() => {
	if (currentResetTarget.value === 'json') return i18n.global.t('subscriptionEditor.resetJson')
	if (currentResetTarget.value === 'clash') return i18n.global.t('subscriptionEditor.resetClash')
  return ''
})

const resetDialogMessage = computed(() => {
  if (resetTarget.value === 'json') {
	return i18n.global.t('subscriptionEditor.resetJsonMessage')
  }
  if (resetTarget.value === 'clash') {
	return i18n.global.t('subscriptionEditor.resetClashMessage')
  }
  return ''
})

const openResetDialog = () => {
  if (!hasVerifiedSettings.value || !currentResetTarget.value) return
  resetTarget.value = currentResetTarget.value
  resetDialogVisible.value = true
}

const closeResetDialog = () => {
  resetDialogVisible.value = false
  resetTarget.value = ''
}

const confirmResetSubPage = async () => {
	const target = resetTarget.value
	if (!hasVerifiedSettings.value || !target || loading.value) return
	loading.value = true
	try {
		const msg = await HttpUtils.post('api/subscription-initial-reset', {
			kind: target,
			expectedRevision: settingsRevision.value,
		}, {
			headers: { 'Content-Type': 'application/json' },
			timeout: 30000,
			silentErrorToast: true,
		})
		if (msg.success && isSubscriptionInitialResetResult(msg.obj, target)) {
			applySubscriptionInitialReset(target, msg.obj)
			const warnings = Array.isArray(msg.obj.warnings) ? msg.obj.warnings.map(String) : []
			push.success({
				title: i18n.global.t('success'),
				duration: 5000,
				message: target === 'json'
					? i18n.global.t('subscriptionEditor.resetJsonSuccess')
					: i18n.global.t('subscriptionEditor.resetClashSuccess'),
			})
			if (warnings.length > 0) {
				push.warning({
					title: i18n.global.t('subscriptionEditor.settingsSavedWithWarnings'),
					duration: 8000,
					message: warnings.join('; '),
				})
			}
			closeResetDialog()
			return
		}
		if (msg.obj?.code === 'revision_conflict') {
			closeResetDialog()
			await loadData()
			await loadSystemTimeZone()
			push.warning({
				title: i18n.global.t('subscriptionEditor.revisionConflictTitle'),
				duration: 7000,
				message: i18n.global.t('subscriptionEditor.revisionConflictMessage'),
			})
			return
		}
		push.error({
			title: i18n.global.t('failed'),
			duration: 6000,
			message: msg.msg || i18n.global.t('subscriptionEditor.settingsSaveFailed'),
		})
	} finally {
		loading.value = false
	}
}

const sleep = (ms: number) => new Promise(resolve => setTimeout(resolve, ms))

const restartApp = async () => {
  if (!hasVerifiedSettings.value || settingsLoadState.value === 'loading' || !panelCanRestart.value) {
	if (!panelCanRestart.value && panelRestartHint.value) {
	  push.warning({
		title: i18n.global.t('failed'),
		duration: 5000,
		message: panelRestartHint.value,
	  })
	}
	return
  }
  loading.value = true
  try {
    const msg = await HttpUtils.post('api/restartApp', {})
    if (msg.success) {
      let url = settings.value.webURI
      if (!url || url === '') {
        const isTLS = isWebTLSEnabled(settings.value)
        url = buildURL(settings.value.webDomain, settings.value.webPort.toString(), isTLS, settings.value.webPath)
      }
      await sleep(3000)
      window.location.replace(url)
    }
  } finally {
    loading.value = false
  }
}

const isWebTLSEnabled = (value: any) => {
  const multiRaw = String(value?.panelAssignedCertificateRecordIDs ?? '').trim()
  if (multiRaw !== '') {
    try {
      const parsed = JSON.parse(multiRaw)
      if (Array.isArray(parsed)) {
        const cleaned = parsed
          .map(item => Number.parseInt(String(item ?? '').trim(), 10))
          .filter(item => Number.isFinite(item) && item > 0)
        if (cleaned.length > 0) {
          return true
        }
      }
    } catch {
      // fallback to legacy key below
    }
  }
  const raw = String(value?.panelAssignedCertificateRecordID ?? '').trim()
  return raw !== '' && raw !== '0'
}

const maybeRedirectToHttps = async (nextSettings: any, previousSettings: any) => {
  if (window.location.protocol !== 'http:') return
  if (!isWebTLSEnabled(nextSettings)) return
  if (isWebTLSEnabled(previousSettings)) return

  let url = nextSettings.webURI
  if (!url || url === '') {
    url = buildURL(nextSettings.webDomain, nextSettings.webPort.toString(), true, nextSettings.webPath)
  }
  await sleep(1200)
  window.location.replace(url)
}

const buildURL = (host: string, port: string, isTLS: boolean, path: string) => {
  if (!host || host.length === 0) host = window.location.hostname
  if (!port || port.length === 0) port = window.location.port
	if (host.includes(':') && !host.startsWith('[')) {
		host = `[${host}]`
	}

  const protocol = isTLS ? 'https:' : 'http:'

  if (port === '' || (isTLS && port === '443') || (!isTLS && port === '80')) {
    port = ''
  } else {
    port = `:${port}`
  }

  return `${protocol}//${host}${port}${path}settings`
}

const normalizePort = (value: unknown, defaultValue: string) => {
  const strValue = typeof value === 'string' ? value.trim() : String(value ?? '').trim()
  if (strValue === '') return defaultValue
  if (!/^\d+$/.test(strValue)) return strValue
  const parsed = Number(strValue)
  if (!Number.isSafeInteger(parsed) || parsed < 1 || parsed > 65535) return strValue
  return parsed.toString()
}

const applyPortDefaultsBeforeSave = () => {
  settings.value.webPort = normalizePort(settings.value.webPort, DEFAULT_WEB_PORT)
  const fallbackSubPort = normalizePort(oldSettings.value?.subPort, DEFAULT_SUB_PORT)
  settings.value.subPort = normalizePort(settings.value.subPort, fallbackSubPort)
}

const buildSettingsSavePayload = (value: Record<string, any>) => {
  const payload = Object.fromEntries(
    SETTINGS_SAVE_KEYS.map(key => [key, value[key]]),
  ) as Record<string, any>
  const visiblePanelTimeLocation = normalizeTimeLocationValue(payload.timeLocation)
  payload.timeLocation = visiblePanelTimeLocation || hiddenPanelTimeLocation.value || DEFAULT_TIME_LOCATION
  return payload
}

const subEncode = computed({
  get: () => {
    return settings.value.subEncode === 'true'
  },
  set: (v: boolean) => {
    settings.value.subEncode = v ? 'true' : 'false'
  },
})

const subShowInfo = computed({
  get: () => {
    return settings.value.subShowInfo === 'true'
  },
  set: (v: boolean) => {
    settings.value.subShowInfo = v ? 'true' : 'false'
  },
})

const stateChange = computed(() => {
  if (!hasVerifiedSettings.value) return false
  return !FindDiff.deepCompare(settings.value, oldSettings.value)
	|| subJsonDraftDirty.value
	|| subClashDraftDirty.value
    || systemTimeLocation.value !== oldSystemTimeLocation.value
})

const showTopActionBar = computed(() => tab.value !== 't6' && tab.value !== 't7' && tab.value !== 't8' && tab.value !== 't9' && tab.value !== 't10' && tab.value !== 't11' && tab.value !== 't12')
</script>

<style scoped>
.panel-version-footer__summary {
  color: rgba(255, 255, 255, 0.92);
}

.panel-version-footer__action,
.panel-version-footer__action :deep(.v-btn__content) {
  color: #fff !important;
}

.panel-version-footer__action.v-btn--disabled,
.panel-version-footer__action.v-btn--disabled :deep(.v-btn__content) {
  color: rgba(255, 255, 255, 0.88) !important;
  opacity: 1;
}

.panel-uninstall-trigger,
.panel-uninstall-button {
  width: 100%;
}

.panel-uninstall-trigger {
  display: block;
}

.panel-uninstall-overlay-card {
  width: calc(100vw - 32px);
  max-width: 420px;
}

.panel-uninstall-failure {
  min-width: 0;
  flex: 1 1 260px;
}

.panel-uninstall-message-list {
  margin-bottom: 0;
  padding-left: 20px;
  overflow-wrap: anywhere;
}

.panel-update-task-status__text,
.panel-update-task-status__id {
  min-width: 0;
  overflow-wrap: anywhere;
}

.docker-uninstall-command {
  border: 1px solid rgba(0, 0, 0, 0.14);
  border-radius: 6px;
  padding: 12px;
}

.docker-uninstall-command__content {
  max-width: 100%;
  margin: 10px 0 0;
  padding: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  background: #f4f6f7;
  color: #1d252c;
  border-radius: 4px;
}

@media (min-width: 600px) {
  .panel-uninstall-trigger,
  .panel-uninstall-button {
    width: auto;
  }

  .panel-uninstall-trigger {
    display: inline-flex;
  }
}
</style>
