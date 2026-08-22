<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="800" max-width="95vw" max-height="90vh" :persistent="loading">
    <v-card class="rounded-lg">
      <v-card-title>
        {{ $t('actions.addbulk') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 0 16px; max-height: calc(90vh - 132px); overflow-y: auto;">
        <v-container style="padding: 0;">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model.number="count" type="number" min="1" max="100" :label="$t('count')" hide-details></v-text-field>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="8">
              <v-combobox
                chips
                multiple
                v-model="bulkData.name"
                :items="patterns"
                :label="$t('client.name')"
                hide-details>
              </v-combobox>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="8">
              <v-combobox
                chips
                multiple
                v-model="bulkData.desc"
                :items="patterns"
                :label="$t('client.desc')"
                hide-details>
              </v-combobox>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-combobox v-model="bulkData.group" :items="groups" :label="$t('client.group')" hide-details></v-combobox>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model.number="bulkData.Volume" type="number" min="0" step="0.01" :label="$t('stats.volume')" suffix="GiB" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <DatePick :expiry="bulkData.expiry" picker-type="date" submit-mode="day-end" @submit="setDate" />
            </v-col>
          </v-row>
          <v-row>
            <v-col>
              <v-select
                v-model="bulkData.clientInbounds"
                :items="inboundTags"
                :label="$t('client.inboundTags')"
                multiple
                chips
                hide-details
              ></v-select>
            </v-col>
          </v-row>
        </v-container>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          color="primary"
          variant="outlined"
          :disabled="loading"
          @click="closeModal"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="loading"
          :disabled="loading"
          @click="saveChanges"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import DatePick from '@/components/DateTime.vue'
import { push } from 'notivue'
import RandomUtil from '@/plugins/randomUtil'
import { clientVolumeGiBToBytes, Client, createClient, randomConfigs } from '@/types/clients'
import { i18n } from '@/locales'
import { getNamespaceStore } from '@/store/uiNamespace'

export default {
  props: {
    modelValue: {
      type: Boolean,
      default: undefined,
    },
    visible: Boolean,
    inboundTags: Array,
    groups: Array,
    namespace: {
      type: String,
      default: 'default',
    },
  },
  emits: ['close', 'update:modelValue'],
  data() {
    return {
      count: 1,
      clients: <Client[]>[],
      bulkData: {
        name: <any[]>[],
        desc: <any[]>[],
        group: '',
        clientInbounds: <number[]>[],
        expiry: 0,
        Volume: 0,
      },
      patterns: [
        { title: i18n.global.t("bulk.random"), value: "random" },
        { title: i18n.global.t("bulk.order"), value: "order" },
      ],
      loading: false,
    }
  },
  methods: {
    resetData() {
      this.count = 1,
      this.clients = [],
      this.bulkData = {
        name: [this.patterns[1], "-", this.patterns[0]],
        desc: [],
        group: '',
        clientInbounds: [],
        expiry: 0,
        Volume: 0,
      }
    },
    closeModal() {
      if (this.loading) return
      this.$emit('update:modelValue', false)
      this.$emit('close')
    },
    async saveChanges() {
      if (!this.dialogVisible || this.loading) return
      const count = Math.floor(Number(this.count))
      if (!Number.isFinite(count) || count < 1 || count > 100) {
        push.error('批量数量必须在 1 到 100 之间')
        return
      }
      this.count = count
      if (!this.bulkData.name.some(n => n && typeof n === 'object' && ['random', 'order'].includes(n.value))) {
        push.error(i18n.global.t('error.dplData'))
        return
      }
      const volumeBytes = clientVolumeGiBToBytes(this.bulkData.Volume)
      if (volumeBytes === null) {
        push.error('流量上限必须是大于等于 0 的数字')
        return
      }
      const inboundIds = this.normalizeInboundIds(this.bulkData.clientInbounds)
      if (!this.validateMihomoSnellInboundBindings(inboundIds, count)) return
      this.bulkData.clientInbounds = inboundIds
      this.clients = []
        this.loading = true
      try {
        for(let i=0;i<count;i++){
          const name = this.genByPattern(this.bulkData.name, i).trim()
          if (name === '') {
            push.error('批量生成的用户名称不能为空')
            return
          }
          this.clients.push(createClient({
            enable: true,
            name: name,
            config: randomConfigs(name, this.namespace),
            inbounds: [...inboundIds],
            links: [],
            volume: volumeBytes,
            expiry: this.bulkData.expiry,
            up: 0,
            down: 0,
            desc: this.genByPattern(this.bulkData.desc, i),
            group: this.bulkData.group
          }, this.namespace))
        }
        // Check duplicate names
        const store = getNamespaceStore(this.namespace)
        const isDuplicateName = store.checkBulkClientNames(this.clients.map(c => c.name))
        if (isDuplicateName) return
        const success = await store.save('clients', 'addbulk', this.clients)
        if (success) this.closeModal()
      } finally {
        this.loading = false
      }
    },
    genByPattern(pattern: any[], order: number){
      if (pattern.length == 0) return ''
      let result = ''
      pattern.forEach(p => {
        switch(typeof p){
          case 'object':
            if (!p) break
            switch(p.value){
              case "random":
                result += RandomUtil.randomSeq(8)
                break
              case "order":
                result += order+1
            }
            break
          default:
            result += p
        }
      })
      return result
    },
    normalizeInboundIds(raw: unknown): number[] {
      if (!Array.isArray(raw)) return []
      const ids: number[] = []
      const seen = new Set<number>()
      for (const value of raw) {
        const id = Number(value)
        if (!Number.isInteger(id) || id <= 0 || seen.has(id)) continue
        seen.add(id)
        ids.push(id)
      }
      return ids
    },
    validateMihomoSnellInboundBindings(inboundIds: number[], count: number): boolean {
      if (this.namespace !== 'mihomo') return true
      const store = getNamespaceStore(this.namespace)
      const snellInbounds = (store.inbounds ?? []).filter((inbound: any) =>
        inbound?.type === 'snell' && inboundIds.includes(Number(inbound.id ?? 0)),
      )
      for (const inbound of snellInbounds) {
        if (count > 1) {
          push.warning({ message: `Snell 入站 ${inbound.tag} 只能绑定一个用户，批量数量必须为 1` })
          return false
        }
        const owner = (store.clients ?? []).find((client: any) =>
          this.normalizeInboundIds(client?.inbounds).includes(Number(inbound.id ?? 0)),
        )
        if (owner) {
          push.warning({ message: `Snell 入站 ${inbound.tag} 只能绑定一个用户，当前已被 ${owner.name} 使用` })
          return false
        }
      }
      return true
    },
    setDate(v:number){
      this.bulkData.expiry = v
    }
  },
  computed: {
    dialogVisible: {
      get(): boolean {
        return this.$props.modelValue ?? this.$props.visible ?? false
      },
      set(value: boolean) {
        if (value) {
          this.$emit('update:modelValue', true)
          return
        }
        this.closeModal()
      },
    },
  },
  watch: {
    visible(newValue) {
      if (newValue) {
        this.resetData()
      }
    },
  },
  components: { DatePick },
}

</script>
