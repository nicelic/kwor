<template>
  <v-card subtitle="Selector">
    <v-row>
      <v-col cols="12" sm="6">
        <v-combobox
          v-model="data.outbounds"
          :items="tags"
          :label="$t('pages.outbounds')"
          multiple
          @update:model-value="updateDefault"
          chips
          hide-details
        ></v-combobox>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-combobox
          v-model="selectedDefault"
          :items="data.outbounds"
          :label="$t('types.lb.defaultOut')"
          clearable
          hide-details
        ></v-combobox>
      </v-col>
      <v-col cols="12" sm="6" v-if="!isMihomoNamespace">
        <v-switch v-model="data.interrupt_exist_connections" color="primary" :label="$t('types.lb.interruptConn')" hide-details></v-switch>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">

export default {
  props: ['data','tags', 'namespace'],
  data() {
    return {}
  },
  methods: {
    movePreferredMemberToFront(members: string[], preferred: string): string[] {
      const normalizedPreferred = typeof preferred === 'string' ? preferred.trim() : ''
      if (normalizedPreferred === '' || !Array.isArray(members) || members.length === 0) {
        return Array.isArray(members) ? members.filter((value: string) => typeof value === 'string' && value.trim() !== '') : []
      }

      const result: string[] = []
      const seen = new Set<string>()
      const add = (value: unknown) => {
        if (typeof value !== 'string') return
        const normalized = value.trim()
        if (normalized === '' || seen.has(normalized)) return
        seen.add(normalized)
        result.push(normalized)
      }

      add(normalizedPreferred)
      members.forEach(add)
      return result
    },
    normalizeDefaultSelection(preferred?: string | null) {
      const outbounds = Array.isArray(this.$props.data.outbounds) ? [...this.$props.data.outbounds] : []
      const normalizedPreferred = typeof preferred === 'string' && outbounds.includes(preferred)
        ? preferred
        : outbounds[0]

      if (this.isMihomoNamespace) {
        this.$props.data.outbounds = normalizedPreferred
          ? this.movePreferredMemberToFront(outbounds, normalizedPreferred)
          : outbounds
        delete this.$props.data.default
        return
      }

      if (normalizedPreferred && outbounds.includes(normalizedPreferred)) {
        this.$props.data.default = normalizedPreferred
        return
      }
      delete this.$props.data.default
    },
    sanitizeForNamespace() {
      if (!this.isMihomoNamespace) {
        return
      }
      this.normalizeDefaultSelection(this.$props.data.default)
      delete this.$props.data.url
      delete this.$props.data.interval
      delete this.$props.data.tolerance
      delete this.$props.data.idle_timeout
      delete this.$props.data.interrupt_exist_connections
    },
    updateDefault() {
      this.normalizeDefaultSelection(this.selectedDefault)
    }
  },
  computed: {
    isMihomoNamespace(): boolean {
      return this.$props.namespace === 'mihomo'
    },
    selectedDefault: {
      get(): string | undefined {
        if (this.isMihomoNamespace) {
          return Array.isArray(this.$props.data.outbounds) ? this.$props.data.outbounds[0] : undefined
        }
        return this.$props.data.default
      },
      set(value: string | null) {
        this.normalizeDefaultSelection(value)
      }
    }
  },
  mounted() {
    this.sanitizeForNamespace()
  }
}
</script>
