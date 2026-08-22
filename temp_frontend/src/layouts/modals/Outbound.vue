<template>
  <v-dialog transition="dialog-bottom-transition" width="800" max-width="90vw" max-height="90vh" :persistent="loading">
    <v-card class="rounded-lg" :loading="loading">
      <v-card-title>
        {{ $t('actions.' + title) + " " + $t('objects.outbound') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 0 16px; max-height: calc(90vh - 152px); overflow-y: auto;">
        <div :style="{ pointerEvents: loading ? 'none' : 'auto' }" :aria-busy="loading">
        <v-container style="padding: 0;">
          <v-tabs
            v-model="tab"
            align-tabs="center"
          >
            <v-tab value="t1">{{ $t('client.basics') }}</v-tab>
            <v-tab value="t2">{{ $t('client.external') }}</v-tab>
          </v-tabs>
          <v-window v-model="tab">
            <v-window-item value="t1">
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-select
                  hide-details
                  :label="$t('type')"
                  :items="outTypeItems"
                  v-model="outbound.type"
                  @update:modelValue="changeType">
                  </v-select>
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field v-model="outbound.tag" :label="$t('objects.tag')" hide-details></v-text-field>
                </v-col>
              </v-row>
              <v-row v-if="!NoServer.includes(outbound.type)">
                <v-col cols="12" sm="6" md="4">
                  <v-text-field
                  :label="$t('out.addr')"
                  hide-details
                  v-model="outbound.server">
                  </v-text-field>
                </v-col>
                <v-col cols="12" sm="6" md="4" v-if="outbound.type == outTypes.Hysteria && namespace !== 'mihomo'">
                  <v-text-field
                  :label="$t('types.lb.interval')"
                  type="number"
                  min="0"
                  :suffix="$t('date.s')"
                  hide-details
                  v-model.number="hopIntervalSeconds">
                  </v-text-field>
                </v-col>
                <v-col cols="12" sm="6" md="4" v-if="isHy2Type">
                  <v-text-field
                  :label="$t('types.lb.interval')"
                  placeholder="30 | 30s | 30-60 | 30:60s"
                  hide-details
                  v-model="hy2HopIntervalInput"
                  @blur="applyHy2HopIntervalInput"
                  @keydown.enter.prevent="applyHy2HopIntervalInput">
                  </v-text-field>
                </v-col>
                <v-col cols="12" sm="6" md="4" v-if="!isHyType">
                  <v-text-field
                  :label="$t('out.port')"
                  type="number"
                  min="0"
                  hide-details
                  v-model="serverPortInput">
                  </v-text-field>
                </v-col>
              </v-row>
              <v-row v-if="!NoServer.includes(outbound.type) && isHyType">
                <v-col cols="12" sm="6" md="4">
                  <v-text-field
                  :label="$t('out.port')"
                  type="number"
                  min="0"
                  hide-details
                  v-model="hyServerPortInput">
                  </v-text-field>
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field
                  :label="$t('rule.portRange')"
                  hide-details
                  v-model="hyServerPortRangeInput">
                  </v-text-field>
                </v-col>
              </v-row>
              <Socks v-if="outbound.type == outTypes.SOCKS" :data="outbound" :namespace="namespace" />
              <Direct v-if="outbound.type == outTypes.Direct" :data="outbound" />
              <Http v-if="outbound.type == outTypes.HTTP" :data="outbound" :namespace="namespace" />
              <Snell v-if="outbound.type == outTypes.Snell" direction="out" :data="outbound" />
              <Shadowsocks v-if="outbound.type == outTypes.Shadowsocks" direction="out" :data="outbound" />
              <Vmess v-if="outbound.type == outTypes.VMess" :data="outbound" :namespace="namespace" />
              <Trojan v-if="outbound.type == outTypes.Trojan" :data="outbound" :namespace="namespace" />
              <Hysteria v-if="outbound.type == outTypes.Hysteria" direction="out" :data="outbound" :namespace="namespace" />
              <ShadowTls v-if="outbound.type == outTypes.ShadowTLS" :data="outbound" />
              <ShadowQuic v-if="outbound.type == outTypes.ShadowQUIC" direction="out" :data="outbound" />
              <Vless v-if="outbound.type == outTypes.VLESS" :data="outbound" :namespace="namespace" />
              <Tuic v-if="outbound.type == outTypes.TUIC" direction="out" :data="outbound" :namespace="namespace" />
              <Hysteria2 v-if="outbound.type == outTypes.Hysteria2" direction="out" :data="outbound" :namespace="namespace" :hide-port-hop-editors="true" />
              <AnyTls v-if="outbound.type == outTypes.AnyTls" :data="outbound" direction="out" />
              <Mieru v-if="outbound.type == outTypes.Mieru" :data="outbound" direction="out" />
              <Sudoku v-if="outbound.type == outTypes.Sudoku" :data="outbound" direction="out" />
              <TrustTunnel v-if="outbound.type == outTypes.TrustTunnel" :data="outbound" direction="out" :namespace="namespace" />
              <Tor v-if="outbound.type == outTypes.Tor" :data="outbound" />
              <Ssh v-if="outbound.type == outTypes.SSH" :data="outbound" :namespace="namespace" />
              <Selector v-if="outbound.type == outTypes.Selector" :data="outbound" :tags="tags" :namespace="namespace" />
              <UrlTest v-if="outbound.type == outTypes.URLTest" :data="outbound" :tags="tags" :namespace="namespace" />

              <Transport
                v-if="showOutboundTransportEditor"
                :data="outbound"
                :namespace="namespace"
              />
              <OutTLS v-if="showOutboundTlsEditor" :outbound="outbound" :namespace="namespace" />
              <Multiplex v-if="showOutboundMultiplexEditor" direction="out" :data="outbound" />
              <Dial v-if="!NoDial.includes(outbound.type)" :dial="outbound" :namespace="namespace" />
            </v-window-item>
            <v-window-item value="t2">
              <v-row>
                <v-col cols="12">
                  <v-text-field v-model="link" :label="$t('client.external')" hide-details />
                </v-col>
                <v-col cols="12" align="center">
                  <v-btn hide-details variant="tonal" :loading="loading" :disabled="loading" @click="linkConvert">{{ $t('submit') }}</v-btn>
                </v-col>
              </v-row>
            </v-window-item>
          </v-window>
        </v-container>
        </div>
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
import { OutTypes, createOutbound } from '@/types/outbounds'
import RandomUtil from '@/plugins/randomUtil'
import Dial from '@/components/Dial.vue'
import Multiplex from '@/components/Multiplex.vue'
import Transport from '@/components/Transport.vue'
import OutTLS from '@/components/tls/OutTLS.vue'
import Direct from '@/components/protocols/Direct.vue'
import Socks from '@/components/protocols/Socks.vue'
import Http from '@/components/protocols/Http.vue'
import Snell from '@/components/protocols/Snell.vue'
import Shadowsocks from '@/components/protocols/Shadowsocks.vue'
import Vmess from '@/components/protocols/Vmess.vue'
import Trojan from '@/components/protocols/Trojan.vue'
import Wireguard from '@/components/protocols/Wireguard.vue'
import Hysteria from '@/components/protocols/Hysteria.vue'
import ShadowTls from '@/components/protocols/OutShadowTls.vue'
import ShadowQuic from '@/components/protocols/ShadowQuic.vue'
import Vless from '@/components/protocols/Vless.vue'
import Tuic from '@/components/protocols/Tuic.vue'
import Hysteria2 from '@/components/protocols/Hysteria2.vue'
import Tor from '@/components/protocols/Tor.vue'
import Ssh from '@/components/protocols/Ssh.vue'
import Selector from '@/components/protocols/Selector.vue'
import UrlTest from '@/components/protocols/UrlTest.vue'
import HttpUtils from '@/plugins/httputil'
import AnyTls from '@/components/protocols/AnyTls.vue'
import Mieru from '@/components/protocols/Mieru.vue'
import Sudoku from '@/components/protocols/Sudoku.vue'
import TrustTunnel from '@/components/protocols/TrustTunnel.vue'
import { applyHopIntervalInput, formatHopIntervalInput, parseHopIntervalInput, parseHopIntervalSeconds } from '@/plugins/hopInterval'
import { normalizePortRangeInput, normalizeServerPortInput, pickPrimaryPort } from '@/plugins/portRange'
import { parseSingboxInteger } from '@/plugins/singboxInteger'
import { getNamespaceStore } from '@/store/uiNamespace'
import { validateShadowQuicOutbound } from '@/plugins/shadowQuic'
import { push } from 'notivue'
export default {
  props: {
    visible: Boolean,
    data: String,
    id: Number,
    tags: Array,
    namespace: {
      type: String,
      default: 'default',
    },
  },
  emits: ['close'],
  data() {
    return {
      outbound: createOutbound("direct",{ "tag": "" }),
      title: "add",
      tab: "t1",
      link: "",
      loading: false,
      hy2HopIntervalInput: '',
      outTypes: OutTypes,
      mihomoUnsupportedTypes: [OutTypes.Tor],
      defaultUnsupportedTypes: [OutTypes.Snell, OutTypes.Mieru, OutTypes.Sudoku, OutTypes.TrustTunnel, OutTypes.ShadowQUIC],
      NoDial: [OutTypes.Selector, OutTypes.URLTest, OutTypes.ShadowQUIC],
      NoServer: [OutTypes.Direct, OutTypes.Selector, OutTypes.URLTest, OutTypes.Tor, OutTypes.Mieru],
    }
  },
  methods: {
    initMihomoProtocolDefaults() {
      if (this.namespace !== 'mihomo') return
      if ([this.outTypes.Hysteria2, this.outTypes.TUIC].includes(this.outbound.type)) {
        delete this.outbound.mihomo_fast_open
        delete this.outbound.fast_open
        delete this.outbound['fast-open']
        return
      }
      if (this.outbound.type !== this.outTypes.Hysteria) return
      if (this.outbound.mihomo_fast_open === undefined) {
        this.outbound.mihomo_fast_open = true
      }
    },
    syncHy2HopIntervalInput() {
      this.hy2HopIntervalInput = formatHopIntervalInput(this.outbound.hop_interval, this.outbound.hop_interval_max)
    },
    applyHy2HopIntervalInput() {
      if (!this.isHy2Type) return
      const parsed = parseHopIntervalInput(this.hy2HopIntervalInput)
      if (!parsed) {
        this.syncHy2HopIntervalInput()
        return
      }
      applyHopIntervalInput(this.outbound, this.hy2HopIntervalInput)
      this.hy2HopIntervalInput = formatHopIntervalInput(parsed.hopInterval, parsed.hopIntervalMax)
    },
    updateData(id: number) {
      if (id > 0) {
        const newData = JSON.parse(this.$props.data ?? '{}')
        this.outbound = createOutbound(newData.type, newData)
        this.initMihomoProtocolDefaults()
        this.title = "edit"
      }
      else {
        this.outbound = createOutbound("direct",{ tag: "direct-" + RandomUtil.randomSeq(3) })
        this.initMihomoProtocolDefaults()
        this.title = "add"
      }
      this.syncHy2HopIntervalInput()
      this.tab = "t1"
    },
    changeType() {
      // Tag change only in add outbound
      const currentId = this.$props.id ?? 0
      const tag = currentId > 0 ? this.outbound.tag : this.outbound.type + "-" + RandomUtil.randomSeq(3)
      // Use previous data
      const prevConfig = { id: this.outbound.id, tag: tag, listen: this.outbound.listen, listen_port: this.outbound.listen_port }
      this.outbound = createOutbound(this.outbound.type, prevConfig)
      this.initMihomoProtocolDefaults()
      this.syncHy2HopIntervalInput()
    },
    closeModal() {
      this.updateData(0) // reset
      this.$emit('close')
    },
    async saveChanges() {
      if (!this.$props.visible || this.loading) return
      this.applyHy2HopIntervalInput()
      // check duplicate tag
      const store = getNamespaceStore(this.namespace)
      const isDuplicatedTag = store.checkTag('outbound', this.outbound.id, this.outbound.tag)
      if (isDuplicatedTag) return
      if (this.namespace === 'mihomo' && this.outbound.type === this.outTypes.ShadowQUIC) {
        const validationError = validateShadowQuicOutbound(this.outbound)
        if (validationError) {
          push.warning({ title: 'ShadowQUIC 参数错误', message: validationError })
          return
        }
      }

      // save data
      this.loading = true
      try {
        const currentId = this.$props.id ?? 0
        const success = await store.save('outbounds', currentId == 0 ? 'new' : 'edit', this.outbound)
        if (success) this.closeModal()
      } finally {
        this.loading = false
      }
    },
    async linkConvert() {
      if (this.loading || this.link.length === 0) return
      if (this.link.length>0){
        this.loading = true
        try {
          const msg = await HttpUtils.post('api/linkConvert', { link: this.link })
          if (msg.success) {
            const convertedType = typeof msg.obj?.type === 'string' ? msg.obj.type.trim().toLowerCase() : ''
            if (!convertedType || !Object.values(this.outTypes).includes(convertedType)) {
              push.warning({ title: '协议不支持', message: '链接转换返回了当前编辑器不支持的协议。' })
              return
            }
            const unsupportedTypes = this.namespace === 'mihomo'
              ? this.mihomoUnsupportedTypes
              : this.defaultUnsupportedTypes
            if (unsupportedTypes.includes(convertedType)) {
              push.warning({
                title: '协议不支持',
                message: `${convertedType} 不能添加到当前出站管理。`
              })
              return
            }
            this.outbound = createOutbound(convertedType, { ...msg.obj, type: convertedType })
            this.initMihomoProtocolDefaults()
            this.syncHy2HopIntervalInput()
            this.tab = "t1"
            this.link = ""
          }
        } finally {
          this.loading = false
        }
      }
    }
  },
  computed: {
    outTypeItems() {
      const entries = Object.entries(this.outTypes)
      const unsupportedTypes = new Set<string>(
        this.namespace === 'mihomo' ? this.mihomoUnsupportedTypes : this.defaultUnsupportedTypes
      )
      const moveEntry = (valueToMove: string, anchorValue: string, position: 'before' | 'after') => {
        const fromIndex = entries.findIndex(([, value]) => value === valueToMove)
        const anchorIndex = entries.findIndex(([, value]) => value === anchorValue)
        if (fromIndex === -1 || anchorIndex === -1 || fromIndex === anchorIndex) return

        const [entry] = entries.splice(fromIndex, 1)
        const nextAnchorIndex = entries.findIndex(([, value]) => value === anchorValue)
        const insertIndex = position === 'before' ? nextAnchorIndex : nextAnchorIndex + 1
        entries.splice(insertIndex, 0, entry)
      }

      moveEntry(this.outTypes.Snell, this.outTypes.Shadowsocks, 'before')

      return entries
        .filter(([, value]) => !unsupportedTypes.has(value) || value === this.outbound.type)
        .map(([key, value]) => ({ title: key, value }))
    },
    isHyType(): boolean {
      return this.outbound.type === this.outTypes.Hysteria || this.outbound.type === this.outTypes.Hysteria2
    },
    isHy2Type(): boolean {
      return this.outbound.type === this.outTypes.Hysteria2
    },
    serverPortInput: {
      get(): string {
        if (typeof this.outbound.server_port === 'number') return String(this.outbound.server_port)
        if (typeof this.outbound.server_port === 'string') return this.outbound.server_port
        return ''
      },
      set(v: string) {
        const input = typeof v === 'string' ? v : String(v ?? '')
        this.outbound.server_port = normalizeServerPortInput(input)
      }
    },
    hyServerPortInput: {
      get(): string {
        if (typeof this.outbound.server_port === 'number') return String(this.outbound.server_port)
        if (typeof this.outbound.server_port === 'string' && this.outbound.server_port.trim() !== '') {
          return this.outbound.server_port.trim()
        }
        if (Array.isArray(this.outbound.server_ports) && this.outbound.server_ports.length > 0) {
          const primary = pickPrimaryPort(this.outbound.server_ports)
          return primary !== undefined ? String(primary) : ''
        }
        return ''
      },
      set(v: string) {
        const input = typeof v === 'string' ? v : String(v ?? '')
        this.outbound.server_port = normalizeServerPortInput(input)
      }
    },
    hyServerPortRangeInput: {
      get(): string {
        if (!Array.isArray(this.outbound.server_ports)) return ''
        return this.outbound.server_ports
          .map((item: unknown) => String(item).trim())
          .filter((item: string) => item.length > 0)
          .join(',')
      },
      set(v: string) {
        const input = typeof v === 'string' ? v : String(v ?? '')
        const normalized = normalizePortRangeInput(input)
        this.outbound.server_ports = normalized.length > 0 ? normalized : undefined
        if (normalized.length > 0 && this.outbound.server_port == undefined) {
          this.outbound.server_port = pickPrimaryPort(normalized, this.outbound.server_port)
        }
      }
    },
    hopIntervalSeconds: {
      get(): number {
        return parseHopIntervalSeconds(this.outbound.hop_interval)
      },
      set(v: unknown) {
        const seconds = parseSingboxInteger(v, { min: 1 }) ?? 0
        this.outbound.hop_interval = seconds > 0 ? `${seconds}s` : undefined
      }
    },
    showOutboundTransportEditor(): boolean {
      return [this.outTypes.VMess, this.outTypes.Trojan, this.outTypes.VLESS].includes(this.outbound.type)
        || (this.outbound.type !== this.outTypes.Mieru && Object.hasOwn(this.outbound, 'transport'))
    },
    showOutboundMultiplexEditor(): boolean {
      return [this.outTypes.Shadowsocks, this.outTypes.VMess, this.outTypes.Trojan, this.outTypes.VLESS].includes(this.outbound.type)
        || Object.hasOwn(this.outbound, 'multiplex')
    },
    showOutboundTlsEditor(): boolean {
      return [
        this.outTypes.HTTP,
        this.outTypes.VMess,
        this.outTypes.Trojan,
        this.outTypes.Hysteria,
        this.outTypes.ShadowTLS,
        this.outTypes.VLESS,
        this.outTypes.TUIC,
        this.outTypes.Hysteria2,
        this.outTypes.AnyTls,
        this.outTypes.TrustTunnel,
      ].includes(this.outbound.type) || Object.hasOwn(this.outbound, 'tls')
    }
  },
  watch: {
    visible(newValue) {
      if (newValue) {
        this.updateData(this.$props.id ?? 0)
      }
    },
  },
  components: { Dial, Multiplex, Transport, OutTLS,
    Direct, Socks, Http, Snell, Shadowsocks, Vmess, Trojan,
    Wireguard, Hysteria, ShadowTls, ShadowQuic, Vless, Tuic,
    Hysteria2, AnyTls, Mieru, Sudoku, TrustTunnel, Tor, Ssh, Selector, UrlTest }
}
</script>
