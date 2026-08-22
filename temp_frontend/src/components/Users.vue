<template>
  <v-card :subtitle="$t('pages.clients')">
    <v-row>
      <v-col cols="12" sm="6" md="4">
      <v-select v-model="data.model" :items="availableInitUsersModels" @update:model-value="resetValues" hide-details></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!single && data.model == 'group'">
        <v-select v-model="data.values" multiple chips :items="groupNames" :label="$t('client.group')" hide-details></v-select>
      </v-col>
      <v-col cols="12" sm="8" v-if="data.model == 'client'">
        <v-select
          :model-value="clientSelection"
          :multiple="!single"
          :chips="!single"
          :clearable="single"
          :items="clientNames"
          :label="$t('pages.clients')"
          hide-details
          @update:model-value="setClientSelection"
        ></v-select>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
import { i18n } from '@/locales'


export default {
  props: {
    data: {
      type: Object,
      required: true,
    },
    clients: {
      type: Array,
      default: () => [],
    },
    single: {
      type: Boolean,
      default: false,
    },
  },
  data() {
    return {
      initUsersModels: [
        { title: i18n.global.t('none'), value: 'none' },
        { title: i18n.global.t('all'), value: 'all' },
        { title: i18n.global.t('client.group'), value: 'group' },
        { title: i18n.global.t('pages.clients'), value: 'client' },
      ],
    }
  },
  computed: {
    availableInitUsersModels(): any[] {
      if (!this.single) return this.initUsersModels
      return this.initUsersModels.filter((item: any) => item.value === 'none' || item.value === 'client')
    },
    clientNames(): any[] {
      return this.$props.clients.map((c:any) => { return { title: c.name, value: c.id } })
    },
    groupNames(): any[] {
      return Array.from(new Set(this.$props.clients.map((c:any) => c.group)))
    },
    clientSelection(): any {
      const values: any[] = Array.isArray(this.$props.data.values) ? this.$props.data.values : []
      return this.single ? (values[0] ?? null) : values
    },
  },
  methods: {
    resetValues() {
      this.$props.data.values = []
    },
    setClientSelection(value: unknown) {
      if (this.single) {
        this.$props.data.values = value === null || value === undefined || value === '' ? [] : [value]
        return
      }
      this.$props.data.values = Array.isArray(value) ? value : []
    },
    normalizeSelection() {
      const data = this.$props.data
      if (!data || typeof data !== 'object') return
      if (!Array.isArray(data.values)) {
        data.values = data.values === null || data.values === undefined ? [] : [data.values]
      }
      if (this.single) {
        if (data.model !== 'none' && data.model !== 'client') {
          data.model = 'none'
          data.values = []
        } else if (data.values.length > 1) {
          data.values = data.values.slice(0, 1)
        }
      }
    },
  },
  mounted() {
    this.normalizeSelection()
  },
  watch: {
    single() {
      this.normalizeSelection()
    },
    'data.model'() {
      this.normalizeSelection()
    },
  }
} as any
</script>
