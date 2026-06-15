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
        :suffix="$t('date.s')"
        v-model="idleInterval">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('types.anytls.idleTimeout')"
        hide-details
        :suffix="$t('date.s')"
        v-model="idleTimeout">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('types.anytls.minIdle')"
        type="number"
        min="0"
        hide-details
        v-model.number="minIdle">
        </v-text-field>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
export default {
  props: ['data', 'direction'],
  data() {
    return {}
  },
  methods: {
    normalizeSeconds(v: string): string | undefined {
      const raw = (v ?? '').toString().trim()
      if (raw.length === 0) return undefined

      const match = raw.match(/^(\d+)\s*s?$/i)
      if (!match) return undefined

      const value = parseInt(match[1], 10)
      return value > 0 ? `${value}s` : undefined
    },
    parseSecondsForDisplay(v: any, defaultValue: number): string {
      const raw = (v ?? '').toString().trim()
      const match = raw.match(/^(\d+)\s*s?$/i)
      if (!match) return `${defaultValue}`
      return match[1]
    },
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
      get() { return this.parseSecondsForDisplay(this.data.idle_session_check_interval, 30) },
      set(v:string) { this.data.idle_session_check_interval = this.normalizeSeconds(v) }
    },
    idleTimeout: {
      get() { return this.parseSecondsForDisplay(this.data.idle_session_timeout, 30) },
      set(v:string) { this.data.idle_session_timeout = this.normalizeSeconds(v) }
    },
    minIdle: {
      get() { return this.data.min_idle_session != undefined ? this.data.min_idle_session : 0 },
      set(v:number) { this.data.min_idle_session = v>0 ? v : undefined }
    }
  }
}
</script>
