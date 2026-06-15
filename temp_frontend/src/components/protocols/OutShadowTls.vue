<template>
  <div>
    <v-card subtitle="ShadowTls">
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            :items="[1,2,3]"
            :label="$t('version')"
            v-model="version">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="data.version == 3">
          <v-select
            label="Wildcard SNI"
            :items="['off', 'authed', 'all']"
            clearable
            v-model="wildcardSni">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="data.version == 3">
          <v-switch
            color="primary"
            label="Strict Mode"
            v-model="strictMode"
            hide-details>
          </v-switch>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
          :label="$t('types.shdwTls.hs')"
          hide-details
          v-model="handshakeServer">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
          :label="$t('out.port')"
          type="number"
          min="0"
          hide-details
          v-model.number="handshakeServerPort">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="data.version > 1">
          <v-text-field
          :label="$t('types.pw')"
          hide-details
          v-model="data.password">
          </v-text-field>
        </v-col>
      </v-row>
    </v-card>
    
    <!-- Shadowsocks 配置 -->
    <v-card subtitle="Shadowsocks" style="margin-top: 16px;">
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            :label="$t('in.ssMethod')"
            :items="ssMethods"
            @update:model-value="changeMethod($event)"
            v-model="ssConfig.method">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            :label="$t('network')"
            :items="networks"
            v-model="ssNetwork">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-switch
            v-model="udpOverTcpEnable"
            color="primary"
            label="UDP over TCP"
            hide-details>
          </v-switch>
        </v-col>
      </v-row>
      <v-row v-if="ssConfig.method != 'none'">
        <v-col cols="12" sm="8">
          <v-text-field
            v-model="ssConfig.password"
            :label="$t('types.pw')"
            hide-details
            append-inner-icon="mdi-refresh"
            @click:append-inner="changeMethod(ssConfig.method)">
          </v-text-field>
        </v-col>
      </v-row>
      <!-- 多路复用 -->
      <v-card :subtitle="$t('objects.multiplex')" style="margin-top: 12px;">
        <v-row>
          <v-col cols="12" sm="6" md="3">
            <v-switch color="primary" :label="$t('mux.enable')" v-model="muxEnable" hide-details></v-switch>
          </v-col>
          <template v-if="ssConfig.multiplex && ssConfig.multiplex.enabled">
            <v-col cols="12" sm="6" md="3">
              <v-select
                hide-details
                :label="$t('mux.protocol')"
                :items="muxProtocols"
                v-model="ssConfig.multiplex.protocol">
              </v-select>
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-text-field
                :label="$t('mux.maxConnections')"
                hide-details
                type="number"
                min="1"
                v-model.number="ssConfig.multiplex.max_connections">
              </v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-text-field
                :label="$t('mux.minStreams')"
                hide-details
                type="number"
                min="1"
                v-model.number="ssConfig.multiplex.min_streams">
              </v-text-field>
            </v-col>
          </template>
        </v-row>
        <v-row v-if="ssConfig.multiplex && ssConfig.multiplex.enabled">
          <v-col cols="12" sm="6" md="3">
            <v-switch color="primary" :label="$t('mux.padding')" v-model="ssConfig.multiplex.padding" hide-details></v-switch>
          </v-col>
          <v-col cols="12" sm="6" md="3">
            <v-switch color="primary" :label="$t('mux.enableBrutal')" v-model="brutalEnable" hide-details></v-switch>
          </v-col>
        </v-row>
        <v-row v-if="ssConfig.multiplex && ssConfig.multiplex.brutal && ssConfig.multiplex.brutal.enabled">
          <v-col cols="12" sm="6" md="4">
            <v-text-field
            :label="$t('stats.upload')"
            hide-details
            type="number"
            :suffix="$t('stats.Mbps')"
            v-model.number="upMbps">
            </v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
            :label="$t('stats.download')"
            hide-details
            type="number"
            :suffix="$t('stats.Mbps')"
            min="0"
            v-model.number="downMbps">
            </v-text-field>
          </v-col>
        </v-row>
      </v-card>
    </v-card>
  </div>
</template>

<script lang="ts">
import RandomUtil from '@/plugins/randomUtil'

export default {
  props: ['data'],
  data() {
    return {
      ssMethods: [
        "none",
        "aes-128-gcm",
        "aes-192-gcm",
        "aes-256-gcm",
        "chacha20-ietf-poly1305",
        "xchacha20-ietf-poly1305",
        "2022-blake3-aes-128-gcm",
        "2022-blake3-aes-256-gcm",
        "2022-blake3-chacha20-poly1305"
      ],
      networks: [
        { title: "TCP/UDP", value: '' },
        { title: "TCP", value: 'tcp' },
        { title: "UDP", value: 'udp' },
      ],
      muxProtocols: [
        { title: "H2mux", value: 'h2mux' },
        { title: "Smux", value: 'smux' },
        { title: "Yamux", value: 'yamux' },
      ],
      ssNetwork: '',
    }
  },
  methods: {
    changeMethod(ssMethod: string) {
      if (ssMethod.startsWith('2022')) {
        this.ssConfig.password = ssMethod == "2022-blake3-aes-128-gcm" ? RandomUtil.randomShadowsocksPassword(16) : RandomUtil.randomShadowsocksPassword(32)
      } else if (ssMethod == 'none') {
        delete this.ssConfig.password
      } else {
        this.ssConfig.password = RandomUtil.randomSeq(10)
      }
    },
    initSsConfig() {
      if (!this.$props.data.ss_config) {
        this.$props.data.ss_config = {
          method: '2022-blake3-aes-128-gcm',
          password: RandomUtil.randomShadowsocksPassword(16),
          network: 'tcp',
          udp_over_tcp: false,
          multiplex: {
            enabled: true,
            protocol: 'h2mux',
            max_connections: 8,
            min_streams: 16,
            padding: true
          }
        }
      }
      // 确保有密码
      if (!this.$props.data.ss_config.password && this.$props.data.ss_config.method != 'none') {
        this.changeMethod(this.$props.data.ss_config.method)
      }
    }
  },
  mounted() {
    this.initSsConfig()
  },
  computed: {
    version: {
      get() { return this.$props.data.version ?? 3 },
      set(v: number) {
        this.$props.data.version = v
        if (v == 1) {
          delete this.$props.data.password
          delete this.$props.data.wildcard_sni
          delete this.$props.data.strict_mode
        } else if (this.$props.data.password === undefined) {
          this.$props.data.password = ""
        }
        if (v == 3) {
          if (this.$props.data.wildcard_sni == undefined) {
            this.$props.data.wildcard_sni = 'off'
          }
          if (this.$props.data.strict_mode == undefined) {
            this.$props.data.strict_mode = true
          }
        } else {
          delete this.$props.data.wildcard_sni
          delete this.$props.data.strict_mode
        }
      }
    },
    wildcardSni: {
      get(): string { return this.$props.data.wildcard_sni ?? 'off' },
      set(v: string) { this.$props.data.wildcard_sni = (v && v.trim() !== '') ? v : 'off' }
    },
    strictMode: {
      get(): boolean { return this.$props.data.strict_mode ?? true },
      set(v: boolean) { this.$props.data.strict_mode = v }
    },
    handshakeServer: {
      get(): string {
        const handshakeServer = this.$props.data.handshake?.server
        if (typeof handshakeServer === 'string' && handshakeServer.trim() !== '') {
          return handshakeServer
        }
        return this.$props.data.tls?.server_name ?? ''
      },
      set(v: string) {
        const value = (v ?? '').trim()
        if (!this.$props.data.handshake) {
          this.$props.data.handshake = { server: '', server_port: 443 }
        }
        this.$props.data.handshake.server = value
        if (!this.$props.data.tls) this.$props.data.tls = {}
        this.$props.data.tls.server_name = value !== '' ? value : undefined
      }
    },
    handshakeServerPort: {
      get(): number {
        const handshakePort = this.$props.data.handshake?.server_port
        if (typeof handshakePort === 'number' && handshakePort > 0) {
          return handshakePort
        }
        if (typeof this.$props.data.server_port === 'number' && this.$props.data.server_port > 0) {
          return this.$props.data.server_port
        }
        return 443
      },
      set(v: number) {
        const port = Number.isFinite(Number(v)) && Number(v) > 0 ? Math.floor(Number(v)) : 443
        if (!this.$props.data.handshake) {
          this.$props.data.handshake = { server: '', server_port: port }
        }
        this.$props.data.handshake.server_port = port
      }
    },
    ssConfig(): any {
      if (!this.$props.data.ss_config) {
        this.initSsConfig()
      }
      return this.$props.data.ss_config
    },
    udpOverTcpEnable: {
      get(): boolean { return this.ssConfig.udp_over_tcp === true || (this.ssConfig.udp_over_tcp && this.ssConfig.udp_over_tcp.enabled) },
      set(v: boolean) { this.ssConfig.udp_over_tcp = v }
    },
    muxEnable: {
      get(): boolean { return this.ssConfig.multiplex ? this.ssConfig.multiplex.enabled : false },
      set(newValue: boolean) { 
        if (newValue) {
          this.ssConfig.multiplex = { 
            enabled: true, 
            protocol: 'h2mux',
            max_connections: 8,
            min_streams: 16,
            padding: true
          }
        } else {
          delete this.ssConfig.multiplex
        }
      }
    },
    brutalEnable: {
      get(): boolean { return this.ssConfig.multiplex?.brutal ? this.ssConfig.multiplex.brutal.enabled : false },
      set(newValue: boolean) { 
        if (!this.ssConfig.multiplex) this.ssConfig.multiplex = { enabled: true }
        this.ssConfig.multiplex.brutal = newValue ? { enabled: newValue, up_mbps: 1000, down_mbps: 1000 } : undefined 
      }
    },
    downMbps: {
      get() { return this.ssConfig.multiplex?.brutal?.down_mbps ?? 1000 },
      set(newValue: any) { 
        if (this.ssConfig.multiplex?.brutal) {
          this.ssConfig.multiplex.brutal.down_mbps = newValue.length != 0 ? newValue : 1000
        }
      }
    },
    upMbps: {
      get() { return this.ssConfig.multiplex?.brutal?.up_mbps ?? 1000 },
      set(newValue: any) {
        if (this.ssConfig.multiplex?.brutal) {
          this.ssConfig.multiplex.brutal.up_mbps = newValue.length != 0 ? newValue : 1000
        }
      }
    },
  },
  watch: {
    data: {
      handler() {
        this.initSsConfig()
        this.ssNetwork = this.ssConfig.network ?? ''
      },
      immediate: true,
    },
    ssNetwork(newValue: string) {
      if (!this.$props.data.ss_config) {
        this.initSsConfig()
      }
      const ssConfig = this.$props.data.ss_config
      if (!ssConfig) return
      if (newValue === '') {
        delete ssConfig.network
      } else {
        ssConfig.network = newValue as 'tcp' | 'udp'
      }
    },
  },
}
</script>
