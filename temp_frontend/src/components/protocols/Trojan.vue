<template>
  <v-card subtitle="Trojan">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-text-field v-model="data.password" :label="$t('types.pw')" hide-details></v-text-field>
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
  },
  methods: {
    sanitizeMihomoUnsupportedFields() {
      if (this.isMihomoNamespace) delete this.data.network
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
