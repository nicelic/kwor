<template>
  <v-row>
    <v-col cols="12" sm="8">
      <v-text-field
        v-model="privateKey"
        :label="$t('types.wg.privKey')"
        append-icon="mdi-key-star"
        @click:append="refreshKey"
        hide-details></v-text-field>
    </v-col>
    <v-col cols="12" sm="8">
      <v-text-field v-model="publicKey" :label="$t('types.wg.pubKey')" hide-details></v-text-field>
    </v-col>
    <v-col cols="12" sm="8">
      <v-text-field v-model="data.pre_shared_key" :label="$t('types.wg.psk')" hide-details></v-text-field>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('out.addr')"
      hide-details
      v-model="address">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('out.port')"
      type="number"
      min="1"
      max="65535"
      step="1"
      hide-details
      v-model="port">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      label="KeepAlive"
      type="number"
      min="1"
      max="65535"
      step="1"
      :suffix="$t('date.s')"
      hide-details
      v-model="keepAlive">
      </v-text-field>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="6">
      <v-text-field v-model="allowed_ips" :label="$t('types.wg.allowedIp') + ' ' + $t('commaSeparated')" hide-details></v-text-field>
    </v-col>
    <v-col cols="12" sm="6">
      <v-text-field v-model="reserved" :label="'Reserved ' + $t('commaSeparated')" hide-details></v-text-field>
    </v-col>
  </v-row>
</template>

<script lang="ts">
import { parseSingboxByteList, parseSingboxInteger } from '@/plugins/singboxInteger'

export default {
  props: ['data', 'ext'],
  emits: ['refreshPeerKey'],
  data() {
    return {}
  },
  methods: {
    refreshKey() {
      this.$emit('refreshPeerKey')
    }
  },
  computed: {
    allowed_ips: {
      get() { return this.$props.data.allowed_ips?.join(',') },
      set(v:string) {
        const values = v.split(',').map((item) => item.trim()).filter((item) => item !== '')
        this.$props.data.allowed_ips = values.length > 0 ? values : undefined
      }
    },
    reserved: {
      get() { return this.$props.data.reserved?.join(',') },
      set(v:string) {
        if (v.trim() === '') {
          this.$props.data.reserved = undefined
          return
        }
        const values = parseSingboxByteList(v)
        if (values) this.$props.data.reserved = values
      }
    },
    address: {
      get() { return this.$props.data.address },
      set(v:string) { this.$props.data.address = v.length > 0 ? v : undefined }
    },
    port: {
      get() { return this.$props.data.port },
      set(v:unknown) { this.$props.data.port = parseSingboxInteger(v, { min: 1, max: 65535 }) }
    },
    keepAlive: {
      get() { return parseSingboxInteger(this.$props.data.persistent_keepalive_interval, { min: 1, max: 65535 }) ?? 0 },
      set(v:unknown) { this.$props.data.persistent_keepalive_interval = parseSingboxInteger(v, { min: 1, max: 65535 }) }
    },
    privateKey: {
      get() {
        const indexKeys = this.$props.ext?.keys?.findIndex((key: any) => key.public_key == this.$props.data.public_key) ?? -1
        return indexKeys > -1 ? this.$props.ext.keys[indexKeys].private_key : ''
      },
      set(v:string) {
        const indexKeys = this.$props.ext?.keys?.findIndex((key: any) => key.public_key == this.$props.data.public_key) ?? -1
        if (indexKeys >= 0 && Array.isArray(this.$props.ext?.keys)) {
          this.$props.ext.keys[indexKeys].private_key = v
        }
      }
    },
    publicKey: {
      get() { return this.$props.data.public_key },
      set(v:string) {
        const indexKeys = this.$props.ext?.keys?.findIndex((key: any) => key.public_key == this.$props.data.public_key) ?? -1
        if (indexKeys >= 0 && Array.isArray(this.$props.ext?.keys)) {
          this.$props.ext.keys[indexKeys].public_key = v
        }
        this.$props.data.public_key = v
      }
    }
  }
}
</script>
