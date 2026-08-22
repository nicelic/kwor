<template>
  <v-card subtitle="Direct">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('types.direct.overrideAddr')"
        hide-details
        v-model="data.override_address">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('types.direct.overridePort')"
        type="number"
        min="1"
        max="65535"
        step="1"
        hide-details
        v-model.number="override_port">
        </v-text-field>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
import { parseServerPortInput } from '@/plugins/portRange'

export default {
  props: ['data'],
  data() {
    return {}
  },
  computed: {
    override_port: {
        get() { return this.$props.data.override_port ?? '' },
        set(newValue: unknown) {
          if (newValue === '' || newValue === null || newValue === undefined || newValue === 0 || newValue === '0') {
            delete this.$props.data.override_port
            return
          }
          const port = parseServerPortInput(String(newValue).trim())
          if (port !== undefined) this.$props.data.override_port = port
        }
    },
  },
}
</script>
