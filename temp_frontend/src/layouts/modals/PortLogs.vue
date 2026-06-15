<template>
  <v-dialog transition="dialog-bottom-transition" width="900">
    <v-card class="rounded-lg">
      <v-card-title>
        <v-row>
          <v-col cols="auto">{{ $t('portLogs.title') }}</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <v-icon icon="mdi-close" @click="$emit('close')"></v-icon>
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
        <v-table v-else fixed-header height="420" density="compact">
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
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="warning" variant="outlined" @click="$emit('clear')">{{ $t('portLogs.clear') }}</v-btn>
        <v-btn color="primary" variant="outlined" @click="$emit('close')">{{ $t('actions.close') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts" setup>
interface PortLogItem {
  id: string
  timestamp: number
  tag: string
  range: string
  message: string
}

defineProps<{
  visible: boolean
  logs: PortLogItem[]
}>()

defineEmits(['close', 'clear'])

const formatTime = (timestamp: number): string => {
  if (!timestamp) return "-"
  return new Date(timestamp).toLocaleString()
}
</script>
