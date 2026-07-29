<template>
  <v-dialog
    :model-value="visible"
    width="calc(100vw - 32px)"
    max-width="560"
    scrollable
    content-class="kwor-confirm-dialog__overlay"
    :content-props="{
      'aria-labelledby': 'kwor-confirm-dialog-title',
      'aria-describedby': 'kwor-confirm-dialog-message',
    }"
    @update:model-value="onDialogModelValue">
    <v-card
      v-if="request !== null"
      class="kwor-confirm-dialog"
      :class="`kwor-confirm-dialog--${request.severity}`">
      <v-card-title id="kwor-confirm-dialog-title" class="kwor-confirm-dialog__header">
        <div
          class="kwor-confirm-dialog__icon"
          :class="`kwor-confirm-dialog__icon--${request.severity}`">
          <v-icon :icon="presentation.icon" :color="presentation.color" />
        </div>
        <div class="kwor-confirm-dialog__heading">
          <div class="kwor-confirm-dialog__title">{{ title }}</div>
        </div>
      </v-card-title>

      <v-card-text class="kwor-confirm-dialog__content">
        <div id="kwor-confirm-dialog-message" class="kwor-confirm-dialog__message">{{ request.message }}</div>
      </v-card-text>

      <v-card-actions class="kwor-confirm-dialog__actions">
        <v-btn
          autofocus
          variant="outlined"
          class="kwor-confirm-dialog__cancel"
          @click="settleConfirm(false)">
          {{ cancelText }}
        </v-btn>
        <v-btn
          :color="presentation.color"
          variant="flat"
          class="kwor-confirm-dialog__confirm"
          @click="settleConfirm(true)">
          {{ request.confirmText }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { activeConfirm, settleConfirm, type ConfirmSeverity } from '@/plugins/confirm'

const { t } = useI18n()

const presentationBySeverity: Record<ConfirmSeverity, { color: string; icon: string }> = {
  info: { color: 'primary', icon: 'mdi-information-outline' },
  warning: { color: 'warning', icon: 'mdi-alert-outline' },
  danger: { color: 'error', icon: 'mdi-alert-octagon-outline' },
}

const request = computed(() => activeConfirm.value?.options ?? null)
const visible = computed(() => request.value !== null)
const presentation = computed(() => presentationBySeverity[request.value?.severity ?? 'info'])
const title = computed(() => (
  request.value?.title?.trim()
  || t(`confirmDialog.${request.value?.severity ?? 'info'}Title`)
))
const cancelText = computed(() => request.value?.cancelText ?? t('confirmDialog.cancel'))

const onDialogModelValue = (value: boolean) => {
  if (!value) {
    settleConfirm(false)
  }
}

onBeforeUnmount(() => {
  settleConfirm(false)
})
</script>

<style scoped>
:global(.v-dialog > .v-overlay__content.kwor-confirm-dialog__overlay) {
  width: calc(100% - 32px);
  max-width: calc(100% - 32px);
  max-height: calc(100vh - 32px);
  margin: 16px;
}

@supports (height: 100dvh) {
  :global(.v-dialog > .v-overlay__content.kwor-confirm-dialog__overlay) {
    max-height: calc(100dvh - 32px);
  }
}

.kwor-confirm-dialog {
  display: flex;
  flex-direction: column;
  max-height: calc(100vh - 32px);
  border-top: 3px solid rgb(var(--v-theme-primary));
  border-radius: 8px;
}

@supports (height: 100dvh) {
  .kwor-confirm-dialog {
    max-height: calc(100dvh - 32px);
  }
}

.kwor-confirm-dialog--warning {
  border-top-color: rgb(var(--v-theme-warning));
}

.kwor-confirm-dialog--danger {
  border-top-color: rgb(var(--v-theme-error));
}

.kwor-confirm-dialog__header {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  padding: 20px 24px 12px;
  white-space: normal;
}

.kwor-confirm-dialog__icon {
  display: grid;
  flex: 0 0 auto;
  width: 40px;
  height: 40px;
  place-items: center;
  border-radius: 8px;
  background: rgba(var(--v-theme-primary), 0.14);
}

.kwor-confirm-dialog__icon--warning {
  background: rgba(var(--v-theme-warning), 0.16);
}

.kwor-confirm-dialog__icon--danger {
  background: rgba(var(--v-theme-error), 0.14);
}

.kwor-confirm-dialog__heading {
  min-width: 0;
}

.kwor-confirm-dialog__title {
  overflow-wrap: anywhere;
  font-size: 1.05rem;
  font-weight: 600;
  line-height: 1.45;
}

.kwor-confirm-dialog__content {
  min-height: 0;
  padding: 8px 24px 20px;
  overflow-y: auto;
}

.kwor-confirm-dialog__message {
  overflow-wrap: anywhere;
  white-space: pre-wrap;
  word-break: break-word;
  line-height: 1.65;
}

.kwor-confirm-dialog__actions {
  display: grid;
  flex: 0 0 auto;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 10rem), 1fr));
  gap: 12px;
  padding: 0 24px 24px;
}

.kwor-confirm-dialog__actions :deep(.v-btn) {
  width: 100%;
  min-width: 0;
  min-height: 44px;
  height: auto;
  padding-top: 9px;
  padding-bottom: 9px;
}

.kwor-confirm-dialog__actions :deep(.v-btn__content) {
  overflow-wrap: anywhere;
  text-align: center;
  white-space: normal;
}

@media (max-width: 599px) {
  .kwor-confirm-dialog__header {
    padding: 16px 16px 10px;
  }

  .kwor-confirm-dialog__content {
    padding: 8px 16px 16px;
  }

  .kwor-confirm-dialog__actions {
    grid-template-columns: 1fr;
    padding: 0 16px 16px;
  }
}
</style>
