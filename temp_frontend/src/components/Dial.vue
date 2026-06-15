<template>
  <v-card :subtitle="$t('objects.dial')" style="background-color: inherit;">
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="optionDetour">
        <v-select
          hide-details
          :label="$t('dial.detourText')"
          :items="outTags"
          v-model="dial.detour">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionBind">
        <v-text-field
        :label="$t('dial.bindIf')"
        hide-details
        v-model="dial.bind_interface"></v-text-field>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionIPV4">
        <v-text-field
        :label="$t('dial.bindIp4')"
        hide-details
        v-model="dial.inet4_bind_address"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionIPV6">
        <v-text-field
        :label="$t('dial.bindIp6')"
        hide-details
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
        v-model.number="routingMark"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionRA">
        <v-switch v-model="dial.reuse_addr" color="primary" :label="$t('dial.reuseAddr')" hide-details></v-switch>
      </v-col>
    </v-row>
    <v-row v-if="optionTCP">
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="dial.tcp_fast_open" color="primary" label="TCP Fast Open" hide-details></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="dial.tcp_multi_path" color="primary" label="TCP Multi Path" hide-details></v-switch>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionUDP">
        <v-switch v-model="dial.udp_fragment" color="primary" label="UDP Fragment" hide-details></v-switch>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && optionCT">
        <v-text-field
        :label="$t('dial.connTimeout')"
        hide-details
        type="number"
        min="1"
        :suffix="$t('date.s')"
        v-model.number="connectTimeout"></v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="!isMihomoNamespace && optionDR">
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('dial.domainResolver')"
          :items="dnsTags"
          v-model="dial.domain_resolver">
        </v-select>
      </v-col>
    </v-row>
    <v-card-actions class="pt-0">
      <v-spacer></v-spacer>
      <v-menu v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal">{{ $t('dial.options') }}</v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item>
              <v-switch v-model="optionDetour" color="primary" :label="$t('listen.detour')" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionBind" color="primary" :label="$t('dial.bindIf')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionIPV4" color="primary" :label="$t('dial.bindIp4')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionIPV6" color="primary" :label="$t('dial.bindIp6')" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionRM" color="primary" label="Routing Mark" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionRA" color="primary" :label="$t('dial.reuseAddr')" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionTCP" color="primary" :label="$t('listen.tcpOptions')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionUDP" color="primary" :label="$t('listen.udpOptions')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionCT" color="primary" :label="$t('dial.connTimeout')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionDR" color="primary" :label="$t('dial.domainResolver')" hide-details></v-switch>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
import { getNamespaceStore } from '@/store/uiNamespace'

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
    sanitizeMihomoUnsupportedFields() {
      if (!this.isMihomoNamespace) return
      delete this.$props.dial.inet4_bind_address
      delete this.$props.dial.inet6_bind_address
      delete this.$props.dial.reuse_addr
      delete this.$props.dial.udp_fragment
      delete this.$props.dial.connect_timeout
      delete this.$props.dial.domain_resolver
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
      const outboundTags = this.store.outbounds?.map((o: any) => o.tag)?.filter(Boolean) ?? []
      if (!this.isMihomoNamespace) {
        const endpointTags = this.store.endpoints?.map((e: any) => e.tag)?.filter(Boolean) ?? []
        return [...outboundTags, ...endpointTags]
      }
      const groupTags = this.store.outboundgroups?.map((g: any) => g.tag ?? g.name)?.filter(Boolean) ?? []
      return [...outboundTags, ...groupTags]
    },
    connectTimeout: {
      get() { return this.$props.dial.connect_timeout ? parseInt(this.$props.dial.connect_timeout.replace('s','')) : 5 },
      set(newValue:number) { this.$props.dial.connect_timeout = newValue > 0 ? newValue + 's' : '5s' }
    },
    routingMark: {
      get() { return this.$props.dial.routing_mark?? 0 },
      set(newValue:number) { this.$props.dial.routing_mark = newValue > 0 ? newValue : 0 }
    },
    optionDetour: {
      get(): boolean { return this.$props.dial.detour != undefined },
      set(v:boolean) { v ? this.$props.dial.detour = this.outTags[0]?? '' : delete this.$props.dial.detour }
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
      get(): boolean { return this.$props.dial.routing_mark != undefined },
      set(v:boolean) { v ? this.$props.dial.routing_mark = 0 : delete this.$props.dial.routing_mark }
    },
    optionRA: {
      get(): boolean { return this.$props.dial.reuse_addr != undefined },
      set(v:boolean) { v ? this.$props.dial.reuse_addr = true : delete this.$props.dial.reuse_addr }
    },
    optionTCP: {
      get(): boolean { 
        return this.$props.dial.tcp_fast_open != undefined && 
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
      set(v:boolean) { this.$props.dial.domain_resolver = v ? this.dnsTags[0]?? '' : undefined }
    },
    dnsTags() {
      return this.store.config.dns?.servers?.map((d: any) => d.tag)?.filter(Boolean) ?? []
    }
  }
}
</script>
