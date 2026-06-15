<template>
  <v-card v-if="showCard" :subtitle="cardSubtitle">
    <v-row v-if="showBasicFields">
      <v-col cols="12" sm="6" md="4" v-if="showVersionField">
        <v-select
          hide-details
          :label="$t('version')"
          :items="versionItems"
          v-model="versionValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showPskField">
        <v-text-field
          label="PSK"
          hide-details
          v-model="data.psk">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showReuseField">
        <v-switch
          color="primary"
          label="Reuse"
          hide-details
          v-model="reuseValue">
        </v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="showUDPField">
        <v-switch
          color="primary"
          label="UDP"
          hide-details
          v-model="udpValue">
        </v-switch>
      </v-col>
    </v-row>
    <v-row v-if="showObfsEditor">
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          label="obfs-opts.mode"
          :items="obfsModeItems"
          v-model="obfsModeValue">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="8">
        <v-text-field
          hide-details
          label="obfs-opts.host"
          :disabled="isObfsHostDisabled"
          :readonly="isObfsHostDisabled"
          v-model="obfsHostValue">
        </v-text-field>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
export default {
  props: {
    data: {
      type: Object,
      required: true,
    },
    direction: {
      type: String,
      required: true,
    },
    syncTarget: {
      type: Object,
      default: undefined,
    },
  },
  data() {
    return {
      obfsModeItems: [
        { title: '', value: '' },
        { title: 'http', value: 'http' },
        { title: 'tls', value: 'tls' },
      ],
    }
  },
  computed: {
    showCard(): boolean {
      return this.direction !== 'out_json'
    },
    cardSubtitle(): string | undefined {
      return this.direction === 'out_json' ? undefined : 'Snell'
    },
    showVersionField(): boolean {
      return this.direction === 'in' || this.direction === 'out'
    },
    showPskField(): boolean {
      return this.direction === 'out'
    },
    showReuseField(): boolean {
      return this.direction === 'out'
    },
    showUDPField(): boolean {
      return this.direction === 'in' || this.direction === 'out'
    },
    showBasicFields(): boolean {
      return this.showVersionField || this.showPskField || this.showReuseField || this.showUDPField
    },
    versionItems(): number[] {
      return this.direction === 'in' ? [4, 5] : [1, 2, 3, 4, 5]
    },
    showObfsEditor(): boolean {
      return this.direction === 'in' || this.direction === 'out'
    },
    isObfsHostDisabled(): boolean {
      return !this.obfsModeValue
    },
    versionValue: {
      get(): number {
        if (typeof this.data.version === 'number') return this.data.version
        return this.direction === 'in' ? 5 : 5
      },
      set(v: number) {
        this.data.version = v
        if (this.direction !== 'in' && this.syncTarget && typeof this.syncTarget === 'object') {
          this.syncTarget.version = v
        }
      },
    },
    reuseValue: {
      get(): boolean {
        return this.data.reuse === true
      },
      set(v: boolean) {
        this.data.reuse = v === true
      },
    },
    udpValue: {
      get(): boolean {
        return this.data.udp !== false
      },
      set(v: boolean) {
        this.data.udp = v === true
      },
    },
    obfsModeValue: {
      get(): string {
        return this.data.obfs_opts?.mode ?? ''
      },
      set(v: string) {
        const normalized = typeof v === 'string' ? v.trim() : ''
        if (normalized === '') {
          delete this.data.obfs_opts
          if (this.syncTarget && typeof this.syncTarget === 'object') {
            delete this.syncTarget.obfs_opts
          }
          return
        }
        const next = {
          mode: normalized,
          host: this.data.obfs_opts?.host ?? 'www.bing.com',
        }
        this.data.obfs_opts = next
        if (this.syncTarget && typeof this.syncTarget === 'object') {
          this.syncTarget.obfs_opts = { ...next }
        }
      },
    },
    obfsHostValue: {
      get(): string {
        return this.data.obfs_opts?.host ?? 'www.bing.com'
      },
      set(v: string) {
        const host = (v ?? '').trim() || 'www.bing.com'
        const mode = this.data.obfs_opts?.mode
        if (!mode) return
        const next = { mode, host }
        this.data.obfs_opts = next
        if (this.syncTarget && typeof this.syncTarget === 'object') {
          this.syncTarget.obfs_opts = { ...next }
        }
      },
    },
  },
}
</script>
