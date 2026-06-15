<template>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.hosts')"
      hide-details
      v-model="hosts">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.path')"
      hide-details
      v-model="transport.path">
      </v-text-field>
    </v-col>
  </v-row>
</template>

<script lang="ts">
import { H2 } from '../../types/transport'

export default {
  props: ['transport'],
  computed: {
    H2Transport(): H2 {
      return <H2>this.$props.transport ?? {}
    },
    hosts: {
      get() {
        return this.normalizeHostValues(this.H2Transport.host).join(',')
      },
      set(newValue: string) {
        const values = this.normalizeHostValues(newValue)
        if (values.length > 0) {
          this.$props.transport.host = values
        } else {
          delete this.$props.transport.host
        }
      },
    },
  },
  methods: {
    normalizeHostValues(raw: unknown): string[] {
      if (typeof raw === 'string') {
        return raw
          .split(',')
          .map((value: string) => value.trim())
          .filter((value: string) => value.length > 0)
      }
      if (Array.isArray(raw)) {
        return raw
          .filter((value: unknown) => typeof value === 'string')
          .map((value: unknown) => String(value).trim())
          .filter((value: string) => value.length > 0)
      }
      return []
    },
  },
}
</script>
