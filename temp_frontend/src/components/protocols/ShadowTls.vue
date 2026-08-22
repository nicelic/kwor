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
        <v-col cols="12" sm="6" md="4" v-if="!isMihomo && Inbound.wildcard_sni != undefined">
          <v-select label="Wildcard SNI" :items="['off', 'authed', 'all']" clearable v-model="Inbound.wildcard_sni"></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="!isMihomo && Inbound.version == 3">
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
          v-model="Inbound.handshake.server">
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
          v-model.number="server_port">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="Inbound.version == 2">
          <v-text-field
            :label="$t('types.pw')"
            hide-details
            v-model="Inbound.password">
          </v-text-field>
        </v-col>
      </v-row>
      <Dial v-if="!isMihomo" :dial="Inbound.handshake" />
    </v-card>
    
    <!-- Shadowsocks 内部配置 -->
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
      <v-col cols="12" sm="6" md="4" v-if="!isMihomo">
        <v-select
          hide-details
          :label="$t('network')"
          :items="networks"
          v-model="ssNetwork">
        </v-select>
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
  </v-card>

  <v-card subtitle="ShadowTls" style="margin-top: 16px;" v-if="!isMihomo && Inbound.handshake_for_server_name != undefined">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
        :label="$t('types.shdwTls.addHS')"
        hide-details
        v-model="handshake_server">
        <template v-slot:append>
          <v-chip 
            color="primary"
            density="compact"
            variant="elevated"
            :disabled="handshake_server == ''"
            @click="addHandshakeServer()">
            <v-icon icon="mdi-plus" />
          </v-chip>
        </template>
        </v-text-field>
      </v-col>
    </v-row>
    <v-card
      v-for="(value, key) in Inbound.handshake_for_server_name"
      :key="key"
      border
      density="compact"
      style="margin: 5px;"
      color="background">
      <v-card-title>
        <v-row>
          <v-col>{{ key }}
            <v-icon icon="mdi-delete" color="error" size="small"
            @click="Inbound.handshake_for_server_name ? delete Inbound.handshake_for_server_name[key] : null" />
          </v-col>
        </v-row>
      </v-card-title>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
          :label="$t('types.shdwTls.hs')"
          hide-details
          v-model="value.server">
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
          v-model.number="value.server_port">
          </v-text-field>
        </v-col>
      </v-row>
      <Dial :dial="value" />
    </v-card>
  </v-card>
  </div>
</template>

<script lang="ts">
import { ShadowTLS } from '@/types/inbounds'
import Dial from '../Dial.vue'
import RandomUtil from '@/plugins/randomUtil'

export default {
  props: {
    data: { type: Object, required: true },
    namespace: {
      type: String,
      default: 'default',
    },
  },
  data() {
    return {
      handshake_server: '',
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
      ssNetwork: '',
    }
  },
  methods: {
    normalizeVersion(value: unknown): 1 | 2 | 3 {
      const version = Number(value)
      return version === 1 || version === 2 || version === 3 ? version : 3
    },
    parseValidPort(value: unknown): number | undefined {
      if (value === '' || value === null || value === undefined) return undefined
      if (typeof value === 'string' && !/^\d+$/.test(value.trim())) return undefined
      const port = Number(value)
      return Number.isSafeInteger(port) && port >= 1 && port <= 65535 ? port : undefined
    },
    sanitizeMihomoShadowTLS() {
      if (!this.isMihomo) return
      delete this.Inbound.strict_mode
      delete this.Inbound.wildcard_sni
      delete this.Inbound.handshake_for_server_name
      if (!this.Inbound.handshake || typeof this.Inbound.handshake !== 'object') {
        this.Inbound.handshake = {
          server: '',
          server_port: 443,
        }
      }
      const handshake = this.Inbound.handshake as any
      const dest = typeof handshake.dest === 'string' ? handshake.dest.trim() : ''
      if (dest !== '') {
        if (typeof handshake.server !== 'string' || handshake.server.trim() === '') {
          let server = dest
          let port: number | undefined
          if (dest.startsWith('[')) {
            const endBracket = dest.indexOf(']')
            if (endBracket > 0) {
              server = dest.slice(1, endBracket)
              const suffix = dest.slice(endBracket + 1)
              if (suffix.startsWith(':')) {
                const parsed = this.parseValidPort(suffix.slice(1))
                if (parsed !== undefined) {
                  port = parsed
                }
              }
            }
          } else {
            const firstColon = dest.indexOf(':')
            const lastColon = dest.lastIndexOf(':')
            if (firstColon > 0 && firstColon === lastColon) {
              server = dest.slice(0, lastColon)
              const parsed = this.parseValidPort(dest.slice(lastColon + 1))
              if (parsed !== undefined) {
                port = parsed
              }
            }
          }
          handshake.server = server
          if (port !== undefined) {
            handshake.server_port = port
          }
        }
      }
      delete handshake.dest
      delete handshake.proxy
      delete handshake.detour
      handshake.server_port = this.parseValidPort(handshake.server_port) ?? 443
      if (this.Inbound.ss_config && typeof this.Inbound.ss_config === 'object') {
        delete this.Inbound.ss_config.network
      }
    },
    addHandshakeServer() {
      const serverName = this.handshake_server.trim()
      if (!serverName) return
      if (!this.Inbound.handshake_for_server_name) {
        this.Inbound.handshake_for_server_name = {}
      }
      if (!this.Inbound.handshake_for_server_name[serverName]) {
        const primaryHandshake = this.Inbound.handshake ?? { server: '', server_port: 443 }
        this.Inbound.handshake_for_server_name[serverName] = {
          server: typeof primaryHandshake.server === 'string' ? primaryHandshake.server.trim() : '',
          server_port: this.parseValidPort(primaryHandshake.server_port) ?? 443,
        }
      }
      // Clear the input field after adding the server
      this.handshake_server = ''
    },
    changeMethod(ssMethod: string) {
      if (ssMethod.startsWith('2022')) {
        this.ssConfig.password = ssMethod == "2022-blake3-aes-128-gcm" ? RandomUtil.randomShadowsocksPassword(16) : RandomUtil.randomShadowsocksPassword(32)
      } else if (ssMethod == 'none') {
        delete this.ssConfig.password
      } else {
        this.ssConfig.password = RandomUtil.randomSeq(10)
      }
    },
    ensurePrimaryHandshake() {
      if (!this.Inbound.handshake || typeof this.Inbound.handshake !== 'object' || Array.isArray(this.Inbound.handshake)) {
        this.Inbound.handshake = { server: '', server_port: 443 }
        return
      }
      if (typeof this.Inbound.handshake.server !== 'string') {
        this.Inbound.handshake.server = ''
      }
      this.Inbound.handshake.server_port = this.parseValidPort(this.Inbound.handshake.server_port) ?? 443
    },
    initSsConfig() {
      if (!this.Inbound.ss_config || typeof this.Inbound.ss_config !== 'object' || Array.isArray(this.Inbound.ss_config)) {
        this.Inbound.ss_config = {
          method: '2022-blake3-aes-128-gcm',
          password: RandomUtil.randomShadowsocksPassword(16),
          network: 'tcp',
          udp_over_tcp: {
            enabled: true,
            version: 2
          },
          multiplex: {
            enabled: true,
            protocol: 'smux',
            max_connections: 250,
            max_streams: 8,
            padding: true,
            brutal: {
              enabled: false,
              up_mbps: 1000,
              down_mbps: 1000
            }
          }
        }
        if (this.isMihomo) {
          delete this.Inbound.ss_config.network
        }
      }
      // 确保有密码
      if (!this.Inbound.ss_config.password && this.Inbound.ss_config.method != 'none') {
        this.changeMethod(this.Inbound.ss_config.method)
      }
    },
    ensureVersionSpecificFields() {
      const version = this.normalizeVersion(this.Inbound.version)
      this.Inbound.version = version
      this.ensurePrimaryHandshake()
      if (this.isMihomo) {
        delete this.Inbound.handshake_for_server_name
        delete this.Inbound.wildcard_sni
        delete this.Inbound.strict_mode
        return
      }

      if (version === 2 || version === 3) {
        if (!this.Inbound.handshake_for_server_name ||
          typeof this.Inbound.handshake_for_server_name !== 'object' ||
          Array.isArray(this.Inbound.handshake_for_server_name)) {
          this.Inbound.handshake_for_server_name = {}
        }
      } else {
        delete this.Inbound.handshake_for_server_name
      }

      if (version === 3) {
        if (this.Inbound.wildcard_sni === undefined || this.Inbound.wildcard_sni === null) {
          this.Inbound.wildcard_sni = ''
        }
        if (this.Inbound.strict_mode === undefined) {
          this.Inbound.strict_mode = true
        }
      } else {
        delete this.Inbound.wildcard_sni
        delete this.Inbound.strict_mode
      }
    }
  },
  mounted() {
    this.Inbound.version = this.normalizeVersion(this.Inbound.version)
    this.ensurePrimaryHandshake()
    this.initSsConfig()
    this.ensureVersionSpecificFields()
    this.sanitizeMihomoShadowTLS()
  },
  computed: {
    isMihomo(): boolean {
      return this.$props.namespace === 'mihomo'
    },
    version: {
      get() {
        return this.Inbound.version
      },
      set(newValue: any) {
        switch (newValue) {
        case 1:
          delete this.Inbound.password
          delete this.Inbound.handshake_for_server_name
          delete this.Inbound.wildcard_sni
          delete this.Inbound.strict_mode
          break
        case 2:
          if (!this.Inbound.password) {
            this.Inbound.password = ""
          }
          if (!this.isMihomo && !this.Inbound.handshake_for_server_name) {
            this.Inbound.handshake_for_server_name = {}
          } else if (this.isMihomo) {
            delete this.Inbound.handshake_for_server_name
          }
          delete this.Inbound.wildcard_sni
          delete this.Inbound.strict_mode
          break
        case 3:
          delete this.Inbound.password
          if (!this.isMihomo && !this.Inbound.handshake_for_server_name) {
            this.Inbound.handshake_for_server_name = {}
          }
          if (!this.isMihomo) {
            if (!this.Inbound.wildcard_sni) {
              this.Inbound.wildcard_sni = ""
            }
            if (this.Inbound.strict_mode == undefined) {
              this.Inbound.strict_mode = true
            }
          } else {
            delete this.Inbound.handshake_for_server_name
            delete this.Inbound.wildcard_sni
            delete this.Inbound.strict_mode
          }
          break
        }
        this.Inbound.version = newValue
        this.sanitizeMihomoShadowTLS()
      }
    },
    Inbound(): ShadowTLS {
      return <ShadowTLS>this.$props.data
    },
    server_port: {
      get() { return this.parseValidPort(this.Inbound.handshake.server_port) ?? 443 },
      set(newValue: any) { this.Inbound.handshake.server_port = this.parseValidPort(newValue) ?? 443 }
    },
    strictMode: {
      get(): boolean { return this.Inbound.strict_mode ?? true },
      set(newValue: boolean) { this.Inbound.strict_mode = newValue }
    },
    // Shadowsocks 配置相关
    ssConfig(): any {
      if (!this.Inbound.ss_config) {
        this.initSsConfig()
      }
      return this.Inbound.ss_config
    },
  },
  watch: {
    data: {
      handler() {
        this.initSsConfig()
        this.ensureVersionSpecificFields()
        this.sanitizeMihomoShadowTLS()
        this.ssNetwork = this.ssConfig.network ?? ''
      },
      immediate: true,
    },
    ssNetwork(newValue: string) {
      if (!this.Inbound.ss_config) {
        this.initSsConfig()
      }
      const ssConfig = this.Inbound.ss_config
      if (!ssConfig) return
      if (newValue === '') {
        delete ssConfig.network
      } else {
        ssConfig.network = newValue as 'tcp' | 'udp'
      }
    },
  },
  components: { Dial }
}
</script>
