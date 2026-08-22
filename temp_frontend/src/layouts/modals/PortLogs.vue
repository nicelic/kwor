<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="900" max-width="95vw" max-height="90vh">
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row>
          <v-col cols="auto">{{ $t('portLogs.title') }}</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <v-tooltip location="top" :text="$t('actions.close')">
              <template #activator="{ props: tooltipProps }">
                <v-btn v-bind="tooltipProps" icon="mdi-close" density="compact" variant="text" @click="closeDialog" />
              </template>
            </v-tooltip>
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text>
        <v-alert
          v-if="logs.length === 0"
          type="info"
          variant="outlined"
          :text="$t('portLogs.noLogs')"
        />
        <div v-else class="port-logs-table-wrap">
          <v-table class="port-logs-table" fixed-header height="420" density="compact">
            <thead>
              <tr>
                <th>{{ $t('portLogs.time') }}</th>
                <th>{{ $t('objects.tag') }}</th>
                <th>{{ $t('portLogs.portRange') }}</th>
                <th>{{ $t('portLogs.result') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in logs" :key="item.id">
                <td>{{ formatTime(item.timestamp) }}</td>
                <td>{{ item.tag || "-" }}</td>
                <td>{{ item.range || "-" }}</td>
                <td>{{ item.message }}</td>
              </tr>
            </tbody>
          </v-table>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="warning" variant="outlined" @click="$emit('clear')">{{ $t('portLogs.clear') }}</v-btn>
        <v-btn color="primary" variant="outlined" @click="closeDialog">{{ $t('actions.close') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { formatPanelDateTime } from '@/plugins/panelTime'

interface PortLogItem {
  id: string
  timestamp: number
  tag: string
  range: string
  message: string
}

const props = defineProps<{
  modelValue?: boolean
  visible: boolean
  logs: PortLogItem[]
}>()

const emit = defineEmits(['close', 'clear', 'update:modelValue'])

const dialogVisible = computed({
  get: () => props.modelValue ?? props.visible,
  set: (value: boolean) => {
    emit('update:modelValue', value)
    if (!value) emit('close')
  },
})

const closeDialog = () => {
  emit('update:modelValue', false)
  emit('close')
}

const formatTime = (timestamp: number): string => {
  if (!timestamp) return "-"
  return formatPanelDateTime(timestamp)
}
</script>

<style scoped>
.port-logs-table-wrap {
  overflow-x: auto;
}

.port-logs-table {
  min-width: 620px;
}
</style>
