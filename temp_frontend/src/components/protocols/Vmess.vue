<template>
  <v-card subtitle="VMESS">
    <v-row>
      <v-col cols="12" sm="6">
        <v-text-field v-model="data.uuid" label="UUID" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          label="Alter ID"
          hide-details
          type="number"
          min=0
          step="1"
          v-model="alterID">
        </v-text-field>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('types.vmess.security')"
          :items="securities"
          v-model="data.security">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('types.vless.udpEnc')"
          :items="['none','packetaddr','xudp']"
          v-model="packet_encoding">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace">
        <Network :data="data" />
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="data.global_padding" color="primary" :label="$t('types.vmess.globalPadding')" hide-details></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="data.authenticated_length" color="primary" :label="$t('types.vmess.authLen')" hide-details></v-switch>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
import Network from '@/components/Network.vue'
import { parseSingboxInteger } from '@/plugins/singboxInteger'

export default {
  props: ['data', 'namespace'],
  data() {
    return {
      securities: [
        "auto",
        "none",
        "zero",
        "aes-128-gcm",
        "aes-128-ctr",
        "chacha20-poly1305",
      ]
    }
  },
  computed: {
    isMihomoNamespace(): boolean {
      return this.namespace === 'mihomo'
    },
    alterID: {
      get() { return parseSingboxInteger(this.$props.data.alter_id, { min: 0 }) ?? 0 },
      set(value:unknown) { this.$props.data.alter_id = parseSingboxInteger(value, { min: 0 }) ?? 0 }
    },
    packet_encoding: {
      get() { return this.$props.data.packet_encoding != undefined ? this.$props.data.packet_encoding : 'none' },
      set(newValue:string) { this.$props.data.packet_encoding = newValue != "none" ? newValue : undefined }
    },
  },
  methods: {
    sanitizeMihomoUnsupportedFields() {
      if (this.isMihomoNamespace) delete this.$props.data.network
    },
  },
  mounted() {
    this.sanitizeMihomoUnsupportedFields()
  },
  watch: {
    data() {
      this.sanitizeMihomoUnsupportedFields()
    },
    namespace() {
      this.sanitizeMihomoUnsupportedFields()
    },
  },
  components: { Network }
}
</script>
