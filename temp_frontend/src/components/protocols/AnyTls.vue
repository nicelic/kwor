<template>
  <v-card>
    <v-card-subtitle v-if="direction != 'out_json'">AnyTls</v-card-subtitle>
    <v-row v-if="direction == 'in'">
      <v-col cols="12" sm="8">
        <v-textarea
        label="Padding scheme"
        auto-grow
        hide-details
        v-model="padding_scheme">
        </v-textarea>
      </v-col>
    </v-row>
    <v-row v-else>
      <v-col cols="12" sm="8" v-if="direction == 'out'">
        <v-text-field
        :label="$t('types.pw')"
        hide-details
        v-model="data.password">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('types.anytls.idleInterval')"
        hide-details
        type="number"
        min="1"
        step="any"
        :suffix="$t('date.s')"
        v-model="idleInterval">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('types.anytls.idleTimeout')"
        hide-details
        type="number"
        min="1"
        step="any"
        :suffix="$t('date.s')"
        v-model="idleTimeout">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('types.anytls.minIdle')"
        type="number"
        min="0"
        step="1"
        hide-details
        v-model="minIdle">
        </v-text-field>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
import { readSingboxDuration, writeSingboxDuration } from '@/plugins/singboxDuration'
import { parseSingboxInteger } from '@/plugins/singboxInteger'

export default {
  props: ['data', 'direction'],
  data() {
    return {}
  },
  computed: {
    padding_scheme: {
      get() {
        if (typeof this.data.padding_scheme === 'string') {
          return this.data.padding_scheme
        }
        if (Array.isArray(this.data.padding_scheme)) {
          return this.data.padding_scheme.join("\n")
        }
        return ''
      },
      set(v:string) {
        const normalized = v
          .replace(/\r\n/g, '\n')
          .split('\n')
          .map((line: string) => line.trim())
          .filter((line: string) => line.length > 0)
          .join('\n')
        this.data.padding_scheme = normalized.length > 0 ? normalized : undefined
      }
    },
    idleInterval: {
      get() { return readSingboxDuration(this.data.idle_session_check_interval, 's') ?? 30 },
      set(v:unknown) { this.data.idle_session_check_interval = writeSingboxDuration(v, 's', { minimum: 1 }) }
    },
    idleTimeout: {
      get() { return readSingboxDuration(this.data.idle_session_timeout, 's') ?? 30 },
      set(v:unknown) { this.data.idle_session_timeout = writeSingboxDuration(v, 's', { minimum: 1 }) }
    },
    minIdle: {
      get() { return parseSingboxInteger(this.data.min_idle_session, { min: 0 }) ?? 0 },
      set(v:unknown) { this.data.min_idle_session = parseSingboxInteger(v, { min: 0 }) }
    }
  }
}
</script>
