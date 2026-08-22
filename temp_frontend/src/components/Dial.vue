<template>
  <v-card :subtitle="$t('objects.dial')" style="background-color: inherit;">
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="optionDetour">
        <v-select
          hide-details
          :label="$t('dial.detourText')"
          :items="outTags"
          :disabled="disabled"
          v-model="dial.detour">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionBind">
        <v-text-field
        :label="$t('dial.bindIf')"
        hide-details
        :disabled="disabled"
        v-model="dial.bind_interface"></v-text-field>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionIPV4">
        <v-text-field
        :label="$t('dial.bindIp4')"
        hide-details
        :disabled="disabled"
        v-model="dial.inet4_bind_address"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionIPV6">
        <v-text-field
        :label="$t('dial.bindIp6')"
        hide-details
        :disabled="disabled"
        v-model="dial.inet6_bind_address"></v-text-field>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="optionRM">
        <v-text-field
        label="Linux Routing Mark"
        hide-details
        type="number"
        min="0"
        :disabled="disabled"
        v-model.number="routingMark"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionRA">
        <v-switch v-model="dial.reuse_addr" color="primary" :label="$t('dial.reuseAddr')" hide-details :disabled="disabled"></v-switch>
      </v-col>
    </v-row>
    <v-row v-if="optionTCP">
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="dial.tcp_fast_open" color="primary" label="TCP Fast Open" hide-details :disabled="disabled"></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="dial.tcp_multi_path" color="primary" label="TCP Multi Path" hide-details :disabled="disabled"></v-switch>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionUDP">
        <v-switch v-model="dial.udp_fragment" color="primary" label="UDP Fragment" hide-details :disabled="disabled"></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionCT">
        <v-text-field
        :label="$t('dial.connTimeout')"
        hide-details
        type="number"
        min="1"
        step="any"
        :suffix="$t('date.s')"
        :disabled="disabled"
        v-model.number="connectTimeout"></v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="!isMihomoNamespace && optionDR">
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('dial.domainResolver')"
          :items="dnsTags"
          :disabled="disabled"
          v-model="dial.domain_resolver">
        </v-select>
      </v-col>
    </v-row>
    <v-card-actions class="pt-0">
      <v-spacer></v-spacer>
      <v-menu v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal" :disabled="disabled">{{ $t('dial.options') }}</v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item>
              <v-switch
                v-model="optionDetour"
                color="primary"
                :label="$t('listen.detour')"
                hide-details
                :disabled="disabled || (!optionDetour && outTags.length === 0)"
              ></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionBind" color="primary" :label="$t('dial.bindIf')" hide-details :disabled="disabled"></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionIPV4" color="primary" :label="$t('dial.bindIp4')" hide-details :disabled="disabled"></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionIPV6" color="primary" :label="$t('dial.bindIp6')" hide-details :disabled="disabled"></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionRM" color="primary" label="Routing Mark" hide-details :disabled="disabled"></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionRA" color="primary" :label="$t('dial.reuseAddr')" hide-details :disabled="disabled"></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionTCP" color="primary" :label="$t('listen.tcpOptions')" hide-details :disabled="disabled"></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionUDP" color="primary" :label="$t('listen.udpOptions')" hide-details :disabled="disabled"></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionCT" color="primary" :label="$t('dial.connTimeout')" hide-details :disabled="disabled"></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch
                v-model="optionDR"
                color="primary"
                :label="$t('dial.domainResolver')"
                hide-details
                :disabled="disabled || (!optionDR && dnsTags.length === 0)"
              ></v-switch>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
import { getNamespaceStore } from '@/store/uiNamespace'
import { readSingboxDuration, writeSingboxDuration } from '@/plugins/singboxDuration'

export default {
  props: {
    dial: {
      type: Object,
      required: true,
    },
    namespace: {
      type: String,
      default: 'default',
    },
    candidateTags: {
      type: Array,
      default: undefined,
    },
    candidateDnsTags: {
      type: Array,
      default: undefined,
    },
    disabled: {
      type: Boolean,
      default: false,
    },
  },
  data() {
    return {
      menu: false
    }
  },
  created() {
    this.sanitizeMihomoUnsupportedFields()
  },
  methods: {
    normalizeNonNegativeInteger(value: unknown): number | undefined {
      if (value === '' || value === null || value === undefined) return undefined
      const normalized = Number(value)
      return Number.isSafeInteger(normalized) && normalized >= 0 ? normalized : undefined
    },
    sanitizeMihomoUnsupportedFields() {
      if (!this.isMihomoNamespace) return
      delete this.$props.dial.inet4_bind_address
      delete this.$props.dial.inet6_bind_address
      delete this.$props.dial.reuse_addr
      delete this.$props.dial.udp_fragment
      delete this.$props.dial.connect_timeout
      delete this.$props.dial.domain_resolver
      const routingMark = this.normalizeNonNegativeInteger(this.$props.dial.routing_mark)
      if (routingMark === undefined) delete this.$props.dial.routing_mark
      else this.$props.dial.routing_mark = routingMark
    },
  },
      computed: {
    isMihomoNamespace(): boolean {
      return this.namespace === 'mihomo'
    },
    store() {
      return getNamespaceStore(this.namespace)
    },
    outTags() {
      if (Array.isArray(this.candidateTags)) {
        return [...new Set(this.candidateTags
          .filter((tag: unknown): tag is string => typeof tag === 'string')
          .map((tag: string) => tag.trim())
          .filter((tag: string) => tag.length > 0))]
      }
      const outboundTags = this.store.outbounds?.map((o: any) => o.tag)?.filter(Boolean) ?? []
      if (!this.isMihomoNamespace) {
        const endpointTags = this.store.endpoints?.map((e: any) => e.tag)?.filter(Boolean) ?? []
        return [...outboundTags, ...endpointTags]
      }
      // Mihomo panel subscription groups are storage-only collections. They
      // never become proxy-groups in server.yaml, so they are not valid
      // dialer-proxy targets.
      return [...new Set(outboundTags)]
    },
    connectTimeout: {
      get() {
      const raw = typeof this.$props.dial.connect_timeout === 'string'
          ? this.$props.dial.connect_timeout.trim()
          : ''
        return readSingboxDuration(raw, 's')
      },
      set(newValue:unknown) {
        if (newValue === '' || newValue === null || newValue === undefined) {
          delete this.$props.dial.connect_timeout
          return
        }
        const raw = String(newValue).trim()
        const normalized = writeSingboxDuration(raw, 's', { minimum: 0 })
        if (!normalized) return
        this.$props.dial.connect_timeout = normalized
      }
    },
    routingMark: {
      get() { return this.normalizeNonNegativeInteger(this.$props.dial.routing_mark) ?? 0 },
      set(newValue:number) {
        const normalized = this.normalizeNonNegativeInteger(newValue)
        if (normalized === undefined) {
          delete this.$props.dial.routing_mark
          return
        }
        this.$props.dial.routing_mark = normalized
      }
    },
    optionDetour: {
      get(): boolean { return this.$props.dial.detour != undefined },
      set(v:boolean) {
        if (!v) {
          delete this.$props.dial.detour
          return
        }
        const detour = this.outTags[0]
        if (detour) this.$props.dial.detour = detour
      }
    },
    optionBind: {
      get(): boolean { return this.$props.dial.bind_interface != undefined },
      set(v:boolean) { v ? this.$props.dial.bind_interface = '' : delete this.$props.dial.bind_interface }
    },
    optionIPV4: {
      get(): boolean { return this.$props.dial.inet4_bind_address != undefined },
      set(v:boolean) { v ? this.$props.dial.inet4_bind_address = '' : delete this.$props.dial.inet4_bind_address }
    },
    optionIPV6: {
      get(): boolean { return this.$props.dial.inet6_bind_address != undefined },
      set(v:boolean) { v ? this.$props.dial.inet6_bind_address = '' : delete this.$props.dial.inet6_bind_address }
    },
    optionRM: {
      get(): boolean { return this.normalizeNonNegativeInteger(this.$props.dial.routing_mark) !== undefined },
      set(v:boolean) { v ? this.$props.dial.routing_mark = 0 : delete this.$props.dial.routing_mark }
    },
    optionRA: {
      get(): boolean { return this.$props.dial.reuse_addr != undefined },
      set(v:boolean) { v ? this.$props.dial.reuse_addr = true : delete this.$props.dial.reuse_addr }
    },
    optionTCP: {
      get(): boolean { 
        return this.$props.dial.tcp_fast_open != undefined || 
               this.$props.dial.tcp_multi_path != undefined
      },
      set(v:boolean) {
        if (v) {
          this.$props.dial.tcp_fast_open = false
          this.$props.dial.tcp_multi_path = false
        } else {
          delete this.$props.dial.tcp_fast_open
          delete this.$props.dial.tcp_multi_path
        }
      }
    },
    optionUDP: {
      get(): boolean { return this.$props.dial.udp_fragment != undefined },
      set(v:boolean) { v ? this.$props.dial.udp_fragment = true : delete this.$props.dial.udp_fragment }
    },
    optionCT: {
      get(): boolean { return this.$props.dial.connect_timeout != undefined },
      set(v:boolean) { v ? this.$props.dial.connect_timeout = '5s' : delete this.$props.dial.connect_timeout }
    },
    optionDR: {
      get(): boolean { return this.$props.dial.domain_resolver != undefined },
      set(v:boolean) {
        if (!v) {
          delete this.$props.dial.domain_resolver
          return
        }
        const resolver = this.dnsTags[0]
        if (resolver) this.$props.dial.domain_resolver = resolver
      }
    },
    dnsTags() {
      if (Array.isArray(this.candidateDnsTags)) {
        return [...new Set(this.candidateDnsTags
          .filter((tag: unknown): tag is string => typeof tag === 'string')
          .map((tag: string) => tag.trim())
          .filter((tag: string) => tag.length > 0))]
      }
      const finalTag = this.store.config?.dns?.final
      return typeof finalTag === 'string' && finalTag.trim() ? [finalTag.trim()] : []
    }
  },
  watch: {
    dial() {
      this.sanitizeMihomoUnsupportedFields()
    },
    namespace() {
      this.sanitizeMihomoUnsupportedFields()
    },
  }
}
</script>
