<template>
  <v-card subtitle="VLESS">
    <v-row>
      <v-col cols="12" sm="6">
        <v-text-field v-model="data.uuid" label="UUID" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('types.vless.flow')"
          :items="['','xtls-rprx-vision']"
          v-model="data.flow">
        </v-select>
      </v-col>
    </v-row>
    <v-row>
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
    </v-row>
  </v-card>
</template>

<script lang="ts">
import Network from '@/components/Network.vue'

export default {
  props: ['data', 'namespace'],
  data() {
    return {}
  },
  computed: {
    isMihomoNamespace(): boolean {
      return this.namespace === 'mihomo'
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
