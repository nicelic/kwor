<template>
  <v-dialog transition="dialog-bottom-transition" width="800" @after-enter="updateData(id ?? 0)">
    <v-card class="rounded-lg" :loading="loading">
      <v-card-title>
        {{ $t('actions.' + title) + " " + $t('objects.inbound') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-skeleton-loader
          class="mx-auto border"
          width="95%"
          type="card, text, divider, list-item-two-line"
          v-if="loading"
        ></v-skeleton-loader>
      <v-card-text style="padding: 0 16px; overflow-y: scroll;">
        <v-container style="padding: 0;" :hidden="loading">
          <v-row>
            <v-col cols="12" sm="6" md="4">
                <v-select
              hide-details
              :label="$t('type')"
              :items="inTypeItems"
              v-model="inbound.type"
              @update:modelValue="changeType">
              </v-select>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="inbound.tag" :label="$t('objects.tag')" hide-details></v-text-field>
            </v-col>
          </v-row>
          <v-tabs
            v-if="HasInData.includes(inbound.type)"
            v-model="side"
            density="compact"
            fixed-tabs
            align-tabs="center"
          >
            <v-tab value="s">{{ $t('in.sSide') }}</v-tab>
            <v-tab value="c">{{ $t('in.cSide') }}</v-tab>
          </v-tabs>
          <v-window v-model="side" style="margin-top: 10px;">
            <v-window-item value="s">
              <Listen
                :data="inbound"
                :inTags="inTags"
                :disable-detour-option="isMihomoShadowTLS"
                :disable-tcp-options="isMihomoShadowTLS"
                :disable-udp-options="isMihomoShadowTLS"
                v-if="inbound.type == inTypes.TrustTunnel"
                @listen-port-blur="handleListenPortBlur"
              />
              <Listen
                :data="inbound"
                :inTags="inTags"
                :disable-detour-option="isMihomoShadowTLS"
                :disable-tcp-options="isMihomoShadowTLS"
                :disable-udp-options="isMihomoShadowTLS"
                v-if="inbound.type != inTypes.Tun && inbound.type != inTypes.Mieru && inbound.type != inTypes.TrustTunnel"
                @listen-port-blur="handleListenPortBlur"
              />
              <Direct v-if="inbound.type == inTypes.Direct" :data="inbound" />
              <Snell
                v-if="inbound.type == inTypes.Snell"
                direction="in"
                :data="inbound"
                :sync-target="inbound.out_json"
              />
              <Shadowsocks v-if="inbound.type == inTypes.Shadowsocks" direction="in" :data="inbound" :namespace="namespace" />
              <Hysteria
                v-if="inbound.type == inTypes.Hysteria"
                direction="in"
                :data="inbound"
              />
              <Hysteria2
                v-if="inbound.type == inTypes.Hysteria2"
                direction="in"
                :data="inbound"
                :namespace="namespace"
              />
              <TrustTunnel v-if="inbound.type == inTypes.TrustTunnel" direction="in" :data="inbound" />
              <Naive v-if="inbound.type == inTypes.Naive" :inbound="inbound" :tls-configs="tlsConfigs" />
              <ShadowTls v-if="inbound.type == inTypes.ShadowTLS" direction="in" :data="inbound" :namespace="namespace" />
              <Tuic v-if="inbound.type == inTypes.TUIC" direction="in" :data="inbound" :namespace="namespace" />
              <Tun v-if="inbound.type == inTypes.Tun" :data="inbound" />
              <AnyTls v-if="inbound.type == inTypes.AnyTls" :data="inbound" direction="in" />
              <SshInbound v-if="inbound.type == inTypes.SSH" :data="inbound" :namespace="namespace" />
              <Mieru v-if="inbound.type == inTypes.Mieru" :data="inbound" direction="in" :namespace="namespace" />
              <Sudoku v-if="inbound.type == inTypes.Sudoku" :data="inbound" direction="in" />
              <TProxy v-if="inbound.type == inTypes.TProxy" :inbound="inbound" />
              <Transport
                v-if="Object.hasOwn(inbound,'transport') && inbound.type != inTypes.Mieru"
                :data="inbound"
                :namespace="namespace"
              />
              <v-card v-if="namespace === 'mihomo' && inbound.type === inTypes.VLESS" subtitle="VLESS Encryption" style="margin-top: 1rem;">
                <v-row>
                  <v-col cols="12" sm="6" md="4">
                    <v-switch color="primary" label="VLESS Encryption" v-model="vlessEncryptionEnabled" hide-details></v-switch>
                  </v-col>
                </v-row>
                <template v-if="vlessEncryptionEnabled">
                  <v-row>
                    <v-col cols="12" sm="6" md="4">
                      <v-select
                        hide-details
                        label="Mode"
                        :items="vlessEncryptionModeItems"
                        v-model="vlessEncryptionMode">
                      </v-select>
                    </v-col>
                    <v-col cols="12" sm="6" md="4">
                      <v-text-field
                        hide-details
                        label="RTT Mode (Server)"
                        placeholder="例如600s，300-600s，0s"
                        v-model="vlessEncryptionServerRTT">
                      </v-text-field>
                    </v-col>
                    <v-col cols="12" sm="6" md="4">
                      <v-select
                        hide-details
                        label="Auth Method"
                        :items="vlessEncryptionAuthMethodItems"
                        v-model="vlessEncryptionAuthMethod">
                      </v-select>
                    </v-col>
                  </v-row>
                  <v-row>
                    <v-col cols="12">
                      <v-text-field
                        hide-details
                        label="Padding"
                        v-model="vlessEncryptionPadding">
                      </v-text-field>
                    </v-col>
                  </v-row>
                  <v-row>
                    <v-col cols="12" sm="6" md="4">
                      <v-btn
                        variant="tonal"
                        density="compact"
                        prepend-icon="mdi-refresh"
                        :loading="vlessEncryptionRefreshLoading"
                        @click="refreshVLESSEncryptionKeys">
                        Refresh Keyset
                      </v-btn>
                    </v-col>
                  </v-row>
                  <v-row>
                    <v-col cols="12" sm="6">
                      <v-text-field
                        hide-details
                        readonly
                        :label="vlessEncryptionServerKeyLabel"
                        :model-value="vlessEncryptionServerKeyValue">
                      </v-text-field>
                    </v-col>
                    <v-col cols="12" sm="6">
                      <v-text-field
                        hide-details
                        readonly
                        :label="vlessEncryptionClientKeyLabel"
                        :model-value="vlessEncryptionClientKeyValue">
                      </v-text-field>
                    </v-col>
                  </v-row>
                </template>
              </v-card>
              <Users v-if="hasUser" :clients="clients" :data="initUsers" />
              <InTls v-if="HasTls.includes(inbound.type)"  :inbound="inbound" :tlsConfigs="tlsConfigs" :tls_id="inbound.tls_id" />
              <Multiplex v-if="Object.hasOwn(inbound,'multiplex')" direction="in" :data="inbound" />
            </v-window-item>
            <v-window-item value="c">
              <OutJsonVue v-if="inbound.type != inTypes.Mieru && inbound.type != inTypes.Sudoku" :inData="inbound" :type="inbound.type" :namespace="namespace" @port-hop-range-blur="handlePortHopRangeBlur" />
              <Mieru v-else-if="inbound.type == inTypes.Mieru" :data="inbound.out_json" direction="out_json" :namespace="namespace" />
              <Sudoku v-else :data="inbound.out_json" direction="out_json" />
              <Snell
                v-if="inbound.type == inTypes.Snell"
                direction="out_json"
                :data="inbound.out_json"
              />
              <Multiplex v-if="namespace !== 'mihomo' && inbound.type == inTypes.ShadowTLS" direction="out" :data="stlsClientSsConfig" />
              <Multiplex v-else-if="namespace !== 'mihomo' && Object.hasOwn(inbound,'multiplex')" direction="out" :data="inbound.out_json" />
              <MihomoClientCommonFields
                v-if="namespace === 'mihomo' && HasInData.includes(inbound.type)"
                :data="clientCommonFieldTarget"
                :protocol="inbound.type"
              />
              <v-card
                v-if="namespace === 'mihomo' && inbound.type === inTypes.VLESS && vlessEncryptionEnabled"
                subtitle="VLESS Encryption"
                style="margin-top: 1rem;"
              >
                <v-row>
                  <v-col cols="12" sm="6" md="4">
                    <v-select
                      hide-details
                      label="RTT Mode (Client)"
                      :items="vlessEncryptionClientRTTItems"
                      v-model="vlessEncryptionClientRTT">
                    </v-select>
                  </v-col>
                </v-row>
              </v-card>
              <v-card v-if="inbound.type == inTypes.ShadowTLS" subtitle="uTLS" style="margin-top: 1rem;">
                <v-row>
                  <v-col cols="12" sm="6" md="4">
                    <v-switch color="primary" label="uTLS" v-model="stlsUtlsEnabled" hide-details></v-switch>
                  </v-col>
                  <v-col cols="12" sm="6" md="4" v-if="stlsUtlsEnabled">
                    <v-select
                      hide-details
                      label="Fingerprint"
                      :items="stlsFingerprints"
                      v-model="stlsUtlsFingerprint">
                    </v-select>
                  </v-col>
                </v-row>
              </v-card>
              <v-card v-if="inbound.type != inTypes.TrustTunnel" style="margin-top: 1rem;">
                <v-card-subtitle>{{ $t('in.multiDomain') }}
                  <v-chip color="primary" density="compact" variant="elevated" @click="add_addr"><v-icon icon="mdi-plus" /></v-chip>
                </v-card-subtitle>
                <template v-for="addr,index in inbound.addrs">
                  {{ $t('in.addr') }} #{{ (index+1) }} <v-icon icon="mdi-delete" color="error" @click="inbound.addrs?.splice(index,1)" />
                  <v-divider></v-divider>
                  <AddrVue :addr="addr" :hasTls="HasTls.includes(inbound.type)" />
                </template>
              </v-card>
            </v-window-item>
          </v-window>
        </v-container>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          color="primary"
          variant="outlined"
          @click="closeModal"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="loading"
          @click="saveChanges"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { InTypes, createInbound, Addr, ShadowTLS } from '@/types/inbounds'
import RandomUtil from '@/plugins/randomUtil'

import Listen from '@/components/Listen.vue'
import Direct from '@/components/protocols/Direct.vue'
import Snell from '@/components/protocols/Snell.vue'
import Users from '@/components/Users.vue'
import Shadowsocks from '@/components/protocols/Shadowsocks.vue'
import Hysteria from '@/components/protocols/Hysteria.vue'
import Hysteria2 from '@/components/protocols/Hysteria2.vue'
import TrustTunnel from '@/components/protocols/TrustTunnel.vue'
import Naive from '@/components/protocols/Naive.vue'
import ShadowTls from '@/components/protocols/ShadowTls.vue'
import Tuic from '@/components/protocols/Tuic.vue'
import Tun from '@/components/protocols/Tun.vue'
import AnyTls from '@/components/protocols/AnyTls.vue'
import SshInbound from '@/components/protocols/SshInbound.vue'
import Mieru from '@/components/protocols/Mieru.vue'
import Sudoku from '@/components/protocols/Sudoku.vue'
import InTls from '@/components/tls/InTLS.vue'
import TProxy from '@/components/protocols/TProxy.vue'
import Multiplex from '@/components/Multiplex.vue'
import Transport from '@/components/Transport.vue'
import AddrVue from '@/components/Addr.vue'
import OutJsonVue from '@/components/OutJson.vue'
import MihomoClientCommonFields from '@/components/MihomoClientCommonFields.vue'
import { push } from 'notivue'
import { PORT_RANGE_TEMPLATE, checkPortOccupancy } from '@/plugins/portCheck'
import { getNamespaceStore } from '@/store/uiNamespace'
import HttpUtils from '@/plugins/httputil'
export default {
  props: {
    visible: Boolean,
    id: Number,
    inTags: Array,
    tlsConfigs: Array,
    namespace: {
      type: String,
      default: 'default',
    },
  },
  emits: ['close'],
  data() {
    return {
      inbound: createInbound("direct",{ id:0, "tag": "" }),
      title: "add",
      loading: false,
      side: "s",
      inTypes: InTypes,
      mihomoUnsupportedTypes: ['direct', 'naive', 'hysteria'],
      defaultUnsupportedTypes: [InTypes.Snell, InTypes.Mieru, InTypes.Sudoku, InTypes.TrustTunnel],
      stlsFingerprints: [
        { title: "Chrome", value: "chrome" },
        { title: "Firefox", value: "firefox" },
        { title: "Microsoft Edge", value: "edge" },
        { title: "Apple Safari", value: "safari" },
        { title: "360", value: "360" },
        { title: "QQ", value: "qq" },
        { title: "Apple IOS", value: "ios" },
        { title: "Android", value: "android" },
        { title: "Random", value: "random" },
        { title: "Randomized", value: "randomized" },
      ],
      inboundWithUsers: ['mixed', 'socks', 'http', 'snell', 'shadowsocks', 'vmess', 'trojan', 'naive', 'hysteria', 'shadowtls', 'tuic', 'hysteria2', 'vless', 'anytls', 'mieru', 'sudoku'],
      initUsers: {
        model: 'none',
        values: <any>[],
      },
      HasInData: [
        InTypes.SOCKS,
        InTypes.HTTP,
        InTypes.Mixed,
        InTypes.Snell,
        InTypes.Shadowsocks,
        InTypes.VMess,
        InTypes.ShadowTLS,
        InTypes.Trojan,
        InTypes.Hysteria,
        InTypes.VLESS,
        InTypes.AnyTls,
        InTypes.TUIC,
        InTypes.Hysteria2,
        InTypes.TrustTunnel,
        InTypes.Naive,
        InTypes.Mieru,
        InTypes.Sudoku,
      ],
      HasTls: [
        InTypes.HTTP,
        InTypes.VMess,
        InTypes.Trojan,
        InTypes.Naive,
        InTypes.Hysteria,
        InTypes.TUIC,
        InTypes.Hysteria2,
        InTypes.TrustTunnel,
        InTypes.VLESS,
        InTypes.AnyTls,
      ],
      OnlyTLS: [InTypes.Hysteria, InTypes.Hysteria2, InTypes.TrustTunnel, InTypes.TUIC, InTypes.Naive, InTypes.AnyTls ],
      singlePortCheckSeq: 0,
      portRangeCheckSeq: 0,
      portCheckUnsupportedHinted: false,
      vlessEncryptionPrefix: 'mlkem768x25519plus',
      vlessEncryptionDefaultMode: 'random',
      vlessEncryptionDefaultServerRTT: '0s',
      vlessEncryptionDefaultClientRTT: '1rtt',
      vlessEncryptionDefaultPadding: '100-111-1111.75-0-111.50-0-3333',
      vlessEncryptionAuthMethodDefault: 'x25519',
      vlessEncryptionX25519DecodedLength: 32,
      vlessEncryptionMLKEMSeedDecodedLength: 64,
      vlessEncryptionMLKEMClientDecodedLength: 1184,
      vlessEncryptionPaddingFirstMinLength: 35,
      vlessEncryptionPaddingTotalMaxLength: 65553,
      vlessEncryptionModeItems: [
        { title: 'native', value: 'native' },
        { title: 'xorpub', value: 'xorpub' },
        { title: 'random', value: 'random' },
      ],
      vlessEncryptionAuthMethodItems: [
        { title: 'X25519', value: 'x25519' },
        { title: 'ML-KEM-768', value: 'mlkem768' },
      ],
      vlessEncryptionClientRTTItems: [
        { title: '1rtt', value: '1rtt' },
        { title: '0rtt', value: '0rtt' },
      ],
      vlessEncryptionRefreshLoading: false,
    }
  },
  methods: {
    initShadowTlsClientDefaults() {
      if (this.inbound.type != this.inTypes.ShadowTLS) return
      if (!this.inbound.out_json) this.inbound.out_json = {}
      if (!this.inbound.out_json.tls) this.inbound.out_json.tls = { enabled: true }
      if (!this.inbound.out_json.tls.utls) {
        this.inbound.out_json.tls.utls = { enabled: true, fingerprint: 'safari' }
      }
    },
    initAnyTlsClientDefaults() {
      if (this.inbound.type != this.inTypes.AnyTls) return
      if (!this.inbound.out_json) this.inbound.out_json = {}
      if (!this.inbound.out_json.idle_session_check_interval) {
        this.inbound.out_json.idle_session_check_interval = "30s"
      }
      if (!this.inbound.out_json.idle_session_timeout) {
        this.inbound.out_json.idle_session_timeout = "30s"
      }
    },
    initTrustTunnelClientDefaults() {
      if (this.inbound.type != this.inTypes.TrustTunnel) return
      if (!this.inbound.out_json) this.inbound.out_json = {}
      const trustTunnelNetwork = Array.isArray((this.inbound as any).network) ? (this.inbound as any).network : []
      const supportsUDP = trustTunnelNetwork.includes('udp')
      if (this.inbound.out_json.udp === undefined) {
        this.inbound.out_json.udp = supportsUDP
      }
      if (this.inbound.out_json.health_check === undefined) {
        this.inbound.out_json.health_check = false
      }
      if (!this.inbound.out_json.congestion_controller) {
        this.inbound.out_json.congestion_controller = "bbr"
      }
    },
    initNaiveClientDefaults() {
      if (this.inbound.type != this.inTypes.Naive) return
      if (!this.inbound.out_json) this.inbound.out_json = {}
      if (this.inbound.out_json.quic === undefined) {
        this.inbound.out_json.quic = false
      }
      if (typeof this.inbound.out_json.insecure_concurrency !== 'number') {
        this.inbound.out_json.insecure_concurrency = 0
      }
      if (this.inbound.out_json.quic_congestion_control === undefined) {
        this.inbound.out_json.quic_congestion_control = 'bbr2'
      }
      if (this.inbound.out_json.udp_over_tcp === undefined) {
        this.inbound.out_json.udp_over_tcp = false
      }
    },
    initHysteriaClientBandwidthDefaults(forceDefaults: boolean = false) {
      if (this.inbound.type != this.inTypes.Hysteria && this.inbound.type != this.inTypes.Hysteria2) return
      if (!this.inbound.out_json) this.inbound.out_json = {}
      if (forceDefaults && this.inbound.out_json.up_mbps === undefined) {
        this.inbound.out_json.up_mbps = 2000
      }
      if (forceDefaults && this.inbound.out_json.down_mbps === undefined) {
        this.inbound.out_json.down_mbps = 2000
      }
    },
    normalizeVLESSMihomoEncryptionMode(raw: string): string {
      const value = (raw ?? '').trim().toLowerCase()
      if (value === 'native' || value === 'xorpub' || value === 'random') {
        return value
      }
      return this.vlessEncryptionDefaultMode
    },
    normalizeVLESSMihomoEncryptionAuthMethod(raw: string): string {
      const value = (raw ?? '').trim().toLowerCase()
      if (value === 'mlkem768' || value === 'ml-kem-768' || value === 'mlkem') {
        return 'mlkem768'
      }
      return 'x25519'
    },
    normalizeVLESSMihomoEncryptionServerRTT(raw: string, keepEmpty: boolean = false): string {
      const value = (raw ?? '').trim().toLowerCase()
      if (value === '') {
        return keepEmpty ? '' : this.vlessEncryptionDefaultServerRTT
      }
      if (value === '1rtt') return '0s'
      if (value === '0rtt') return '600s'
      if (value === '0s') return '0s'
      if (/^\d+s$/.test(value) || /^\d+-\d+s$/.test(value)) {
        return value
      }
      return keepEmpty ? value : this.vlessEncryptionDefaultServerRTT
    },
    normalizeVLESSMihomoEncryptionClientRTT(raw: string): string {
      const value = (raw ?? '').trim().toLowerCase()
      if (value === '1rtt' || value === '0rtt') {
        return value
      }
      return this.vlessEncryptionDefaultClientRTT
    },
    inferVLESSMihomoLegacyRTTPair(raw?: string): { serverRTT: string; clientRTT: string } {
      const value = (raw ?? '').trim().toLowerCase()
      if (value === '600s' || value === '300-600s') {
        return { serverRTT: value, clientRTT: '0rtt' }
      }
      if (value === '0rtt') {
        return { serverRTT: '600s', clientRTT: '0rtt' }
      }
      if (value === '1rtt' || value === '0s') {
        return { serverRTT: '0s', clientRTT: '1rtt' }
      }
      return {
        serverRTT: this.vlessEncryptionDefaultServerRTT,
        clientRTT: this.vlessEncryptionDefaultClientRTT,
      }
    },
    looksLikeVLESSMihomoAuthKeySegment(raw: string): boolean {
      const value = (raw ?? '').trim()
      if (value.length < 16) return false
      if (!/^[A-Za-z0-9+/_=-]+$/.test(value)) return false
      return /[A-Za-z+/_=]/.test(value)
    },
    parseVLESSMihomoEncryptionValue(raw: string) {
      const value = (raw ?? '').trim()
      if (value === '') return null
      const parts = value.split('.').map((item) => item.trim()).filter((item) => item !== '')
      if (parts.length < 4) return null
      if (parts[0] !== this.vlessEncryptionPrefix) return null

      const mode = this.normalizeVLESSMihomoEncryptionMode(parts[1] ?? '')
      const rtt = (parts[2] ?? '').trim().toLowerCase()
      const tail = parts.slice(3)
      if (tail.length === 0) return null

      const authKeys: string[] = []
      let splitIdx = tail.length - 1
      for (let i = tail.length - 1; i >= 0; i--) {
        if (!this.looksLikeVLESSMihomoAuthKeySegment(tail[i])) break
        authKeys.unshift(tail[i])
        splitIdx = i - 1
      }
      if (authKeys.length === 0) {
        authKeys.push(tail[tail.length - 1])
        splitIdx = tail.length - 2
      }

      const paddingParts = splitIdx >= 0 ? tail.slice(0, splitIdx + 1) : []
      const padding = paddingParts.length > 0 ? paddingParts.join('.') : this.vlessEncryptionDefaultPadding
      return { mode, rtt, padding, authKeys }
    },
    inferVLESSMihomoEncryptionServerRTT(decryptionRTT?: string, legacyRTT?: string): string {
      const normalizedDecryption = this.normalizeVLESSMihomoEncryptionServerRTT(decryptionRTT ?? '', true)
      if (normalizedDecryption !== '') {
        return this.normalizeVLESSMihomoEncryptionServerRTT(normalizedDecryption)
      }
      return this.inferVLESSMihomoLegacyRTTPair(legacyRTT).serverRTT
    },
    inferVLESSMihomoEncryptionClientRTT(encryptionRTT?: string, decryptionRTT?: string, legacyRTT?: string): string {
      const rawEncryption = (encryptionRTT ?? '').trim().toLowerCase()
      if (rawEncryption === '1rtt' || rawEncryption === '0rtt') {
        return rawEncryption
      }

      const dec = this.normalizeVLESSMihomoEncryptionServerRTT(decryptionRTT ?? '', true)
      if (dec !== '') {
        return dec === '0s' ? '1rtt' : '0rtt'
      }

      return this.inferVLESSMihomoLegacyRTTPair(legacyRTT).clientRTT
    },
    inferVLESSMihomoEncryptionAuthMethod(parsedDecryption: any, parsedEncryption: any, rawPreferred?: string): string {
      if (typeof rawPreferred === 'string' && rawPreferred.trim() !== '') {
        return this.normalizeVLESSMihomoEncryptionAuthMethod(rawPreferred)
      }

      const decryptionKeys = Array.isArray(parsedDecryption?.authKeys) ? parsedDecryption.authKeys : []
      const encryptionKeys = Array.isArray(parsedEncryption?.authKeys) ? parsedEncryption.authKeys : []
      if (decryptionKeys.length >= 2 && encryptionKeys.length >= 2) {
        return this.vlessEncryptionAuthMethodDefault
      }

      const serverKey = (decryptionKeys[0] ?? '').trim()
      const clientKey = (encryptionKeys[0] ?? '').trim()
      if (serverKey.length > 96 || clientKey.length > 96) {
        return 'mlkem768'
      }
      return this.vlessEncryptionAuthMethodDefault
    },
    clearInactiveVLESSMihomoEncryptionKeys() {
      if (this.namespace !== 'mihomo' || this.inbound.type !== this.inTypes.VLESS) return
      const inbound = this.inbound as any
      const method = this.normalizeVLESSMihomoEncryptionAuthMethod(inbound.vless_encryption_auth_method)
      if (method === 'mlkem768') {
        inbound.vless_encryption_x25519_private_key = ''
        inbound.vless_encryption_x25519_password = ''
      } else {
        inbound.vless_encryption_mlkem_seed = ''
        inbound.vless_encryption_mlkem_client = ''
      }
    },
    hydrateVLESSMihomoEncryptionFromLegacyFields(): boolean {
      const inbound = this.inbound as any
      const decryption = typeof inbound.decryption === 'string' ? inbound.decryption : ''
      const outJSON = inbound.out_json && typeof inbound.out_json === 'object' ? inbound.out_json : {}
      const encryption = typeof outJSON.encryption === 'string' ? outJSON.encryption : ''

      const parsedDecryption = this.parseVLESSMihomoEncryptionValue(decryption)
      const parsedEncryption = this.parseVLESSMihomoEncryptionValue(encryption)
      if (!parsedDecryption || !parsedEncryption) {
        return false
      }

      inbound.vless_encryption_enabled = true
      inbound.vless_encryption_mode = this.normalizeVLESSMihomoEncryptionMode(
        parsedDecryption.mode || parsedEncryption.mode || this.vlessEncryptionDefaultMode
      )
      inbound.vless_encryption_server_rtt = this.inferVLESSMihomoEncryptionServerRTT(
        parsedDecryption.rtt,
        inbound.vless_encryption_rtt
      )
      inbound.vless_encryption_client_rtt = this.inferVLESSMihomoEncryptionClientRTT(
        parsedEncryption.rtt,
        parsedDecryption.rtt,
        inbound.vless_encryption_rtt
      )
      inbound.vless_encryption_padding =
        (parsedDecryption.padding || parsedEncryption.padding || this.vlessEncryptionDefaultPadding).trim()

      const authMethod = this.inferVLESSMihomoEncryptionAuthMethod(
        parsedDecryption,
        parsedEncryption,
        inbound.vless_encryption_auth_method
      )
      inbound.vless_encryption_auth_method = authMethod

      const decryptionKeys = Array.isArray(parsedDecryption.authKeys) ? parsedDecryption.authKeys : []
      const encryptionKeys = Array.isArray(parsedEncryption.authKeys) ? parsedEncryption.authKeys : []
      if (decryptionKeys.length >= 2 && encryptionKeys.length >= 2) {
        inbound.vless_encryption_x25519_private_key = decryptionKeys[0]
        inbound.vless_encryption_mlkem_seed = decryptionKeys[1]
        inbound.vless_encryption_x25519_password = encryptionKeys[0]
        inbound.vless_encryption_mlkem_client = encryptionKeys[1]
      } else {
        const serverKey = (decryptionKeys[0] ?? '').trim()
        const clientKey = (encryptionKeys[0] ?? '').trim()
        if (authMethod === 'mlkem768') {
          inbound.vless_encryption_mlkem_seed = serverKey
          inbound.vless_encryption_mlkem_client = clientKey
          inbound.vless_encryption_x25519_private_key = ''
          inbound.vless_encryption_x25519_password = ''
        } else {
          inbound.vless_encryption_x25519_private_key = serverKey
          inbound.vless_encryption_x25519_password = clientKey
          inbound.vless_encryption_mlkem_seed = ''
          inbound.vless_encryption_mlkem_client = ''
        }
      }
      return true
    },
    initVLESSMihomoEncryptionDefaults() {
      if (this.namespace !== 'mihomo' || this.inbound.type !== this.inTypes.VLESS) return
      const inbound = this.inbound as any
      if (inbound.vless_encryption_enabled === undefined && !this.hydrateVLESSMihomoEncryptionFromLegacyFields()) {
        return
      }
      if (!inbound.vless_encryption_enabled) {
        return
      }
      if (inbound.vless_encryption_mode === undefined || inbound.vless_encryption_mode === '') {
        inbound.vless_encryption_mode = this.vlessEncryptionDefaultMode
      } else {
        inbound.vless_encryption_mode = this.normalizeVLESSMihomoEncryptionMode(inbound.vless_encryption_mode)
      }
      const legacyRTT = typeof inbound.vless_encryption_rtt === 'string' ? inbound.vless_encryption_rtt : ''
      if (inbound.vless_encryption_server_rtt === undefined) {
        inbound.vless_encryption_server_rtt = this.inferVLESSMihomoEncryptionServerRTT('', legacyRTT)
      } else {
        inbound.vless_encryption_server_rtt = this.normalizeVLESSMihomoEncryptionServerRTT(
          inbound.vless_encryption_server_rtt,
          true
        )
      }
      if (inbound.vless_encryption_client_rtt === undefined || inbound.vless_encryption_client_rtt === '') {
        inbound.vless_encryption_client_rtt = this.inferVLESSMihomoEncryptionClientRTT(
          '',
          inbound.vless_encryption_server_rtt,
          legacyRTT
        )
      } else {
        inbound.vless_encryption_client_rtt = this.normalizeVLESSMihomoEncryptionClientRTT(
          inbound.vless_encryption_client_rtt
        )
      }
      if (inbound.vless_encryption_padding === undefined || String(inbound.vless_encryption_padding).trim() === '') {
        inbound.vless_encryption_padding = this.vlessEncryptionDefaultPadding
      }
      if (inbound.vless_encryption_auth_method === undefined || inbound.vless_encryption_auth_method === '') {
        const hasX25519Pair =
          typeof inbound.vless_encryption_x25519_private_key === 'string' && inbound.vless_encryption_x25519_private_key.trim() !== '' &&
          typeof inbound.vless_encryption_x25519_password === 'string' && inbound.vless_encryption_x25519_password.trim() !== ''
        const hasMLKEMPair =
          typeof inbound.vless_encryption_mlkem_seed === 'string' && inbound.vless_encryption_mlkem_seed.trim() !== '' &&
          typeof inbound.vless_encryption_mlkem_client === 'string' && inbound.vless_encryption_mlkem_client.trim() !== ''
        if (!hasX25519Pair && hasMLKEMPair) {
          inbound.vless_encryption_auth_method = 'mlkem768'
        } else {
          inbound.vless_encryption_auth_method = this.vlessEncryptionAuthMethodDefault
        }
      } else {
        inbound.vless_encryption_auth_method = this.normalizeVLESSMihomoEncryptionAuthMethod(inbound.vless_encryption_auth_method)
      }
      this.clearInactiveVLESSMihomoEncryptionKeys()
    },
    sanitizeVLESSMihomoEncryptionFields() {
      if (this.namespace !== 'mihomo' || this.inbound.type !== this.inTypes.VLESS) return
      const inbound = this.inbound as any
      if (inbound.vless_encryption_enabled === undefined) return
      inbound.vless_encryption_mode = this.normalizeVLESSMihomoEncryptionMode(inbound.vless_encryption_mode)
      inbound.vless_encryption_server_rtt = this.normalizeVLESSMihomoEncryptionServerRTT(
        inbound.vless_encryption_server_rtt
      )
      inbound.vless_encryption_client_rtt = this.normalizeVLESSMihomoEncryptionClientRTT(
        inbound.vless_encryption_client_rtt
      )
      inbound.vless_encryption_auth_method = this.normalizeVLESSMihomoEncryptionAuthMethod(inbound.vless_encryption_auth_method)
      inbound.vless_encryption_padding =
        typeof inbound.vless_encryption_padding === 'string' && inbound.vless_encryption_padding.trim() !== ''
          ? inbound.vless_encryption_padding.trim()
          : this.vlessEncryptionDefaultPadding
      delete inbound.vless_encryption_rtt

      const keyFields = [
        'vless_encryption_x25519_private_key',
        'vless_encryption_x25519_password',
        'vless_encryption_mlkem_seed',
        'vless_encryption_mlkem_client',
      ]
      keyFields.forEach((field) => {
        if (typeof inbound[field] === 'string') {
          inbound[field] = inbound[field].trim()
        }
      })
      this.clearInactiveVLESSMihomoEncryptionKeys()
    },
    decodeVLESSMihomoBase64URLLength(raw: string): number | null {
      const value = (raw ?? '').trim()
      if (value === '') return null
      const normalized = value.replace(/-/g, '+').replace(/_/g, '/')
      const padLength = (4 - (normalized.length % 4)) % 4
      const padded = normalized + '='.repeat(padLength)
      try {
        return atob(padded).length
      } catch {
        return null
      }
    },
    validateVLESSMihomoPadding(raw: string): string | null {
      const normalized = (raw ?? '').trim().replace(/^\.+|\.+$/g, '')
      if (normalized === '') return null

      const segments = normalized.split('.').map((item) => item.trim()).filter((item) => item !== '')
      if (segments.length === 0) return null

      let maxLength = 0
      for (let i = 0; i < segments.length; i++) {
        const segment = segments[i]
        const parts = segment.split('-').map((item) => item.trim())
        if (parts.length < 3 || parts[0] === '' || parts[1] === '' || parts[2] === '') {
          return `Padding segment "${segment}" is invalid.`
        }

        const probability = Number(parts[0])
        const minValue = Number(parts[1])
        const maxValue = Number(parts[2])
        if (!Number.isInteger(probability) || !Number.isInteger(minValue) || !Number.isInteger(maxValue)) {
          return `Padding segment "${segment}" is invalid.`
        }

        if (
          i === 0 &&
          (
            probability < 100 ||
            minValue < this.vlessEncryptionPaddingFirstMinLength ||
            maxValue < this.vlessEncryptionPaddingFirstMinLength
          )
        ) {
          return `First padding segment must be "100-<min>-<max>" with min/max >= ${this.vlessEncryptionPaddingFirstMinLength}.`
        }

        if (i % 2 === 0) {
          maxLength += Math.max(minValue, maxValue)
        }
      }

      if (maxLength > this.vlessEncryptionPaddingTotalMaxLength) {
        return `Padding max length exceeds ${this.vlessEncryptionPaddingTotalMaxLength}.`
      }
      return null
    },
    validateVLESSMihomoKeyLength(raw: string, expectedLength: number, fieldLabel: string): string | null {
      const value = (raw ?? '').trim()
      if (value === '') {
        return `${fieldLabel} is required.`
      }

      const decodedLength = this.decodeVLESSMihomoBase64URLLength(value)
      if (decodedLength === null) {
        return `${fieldLabel} must be base64url text.`
      }
      if (decodedLength !== expectedLength) {
        return `${fieldLabel} decoded length must be ${expectedLength} bytes.`
      }
      return null
    },
    validateVLESSMihomoEncryption(): boolean {
      if (this.namespace !== 'mihomo' || this.inbound.type !== this.inTypes.VLESS) return true
      const inbound = this.inbound as any
      if (!inbound.vless_encryption_enabled) return true

      const method = this.normalizeVLESSMihomoEncryptionAuthMethod(inbound.vless_encryption_auth_method)
      const errors: string[] = []
      if (method === 'mlkem768') {
        const serverErr = this.validateVLESSMihomoKeyLength(
          inbound.vless_encryption_mlkem_seed,
          this.vlessEncryptionMLKEMSeedDecodedLength,
          'ML-KEM Seed (Server)'
        )
        if (serverErr) errors.push(serverErr)
        const clientErr = this.validateVLESSMihomoKeyLength(
          inbound.vless_encryption_mlkem_client,
          this.vlessEncryptionMLKEMClientDecodedLength,
          'ML-KEM Client (Client)'
        )
        if (clientErr) errors.push(clientErr)
      } else {
        const serverErr = this.validateVLESSMihomoKeyLength(
          inbound.vless_encryption_x25519_private_key,
          this.vlessEncryptionX25519DecodedLength,
          'X25519 PrivateKey (Server)'
        )
        if (serverErr) errors.push(serverErr)
        const clientErr = this.validateVLESSMihomoKeyLength(
          inbound.vless_encryption_x25519_password,
          this.vlessEncryptionX25519DecodedLength,
          'X25519 Password (Client)'
        )
        if (clientErr) errors.push(clientErr)
      }

      const paddingError = this.validateVLESSMihomoPadding(inbound.vless_encryption_padding)
      if (paddingError) {
        errors.push(paddingError)
      }

      if (errors.length > 0) {
        push.warning({
          title: 'VLESS Encryption',
          duration: 7000,
          message: errors.join(' '),
        })
        return false
      }
      return true
    },
    parseKeypairValue(lines: string[], key: string): string {
      const prefix = `${key}:`
      for (const rawLine of lines) {
        const line = (rawLine ?? '').trim()
        if (!line.startsWith(prefix)) continue
        return line.slice(prefix.length).trim()
      }
      return ''
    },
    async refreshVLESSEncryptionKeys(showSuccess: boolean = true) {
      if (this.vlessEncryptionRefreshLoading) return
      this.vlessEncryptionRefreshLoading = true

      const inbound = this.inbound as any
      const method = this.normalizeVLESSMihomoEncryptionAuthMethod(inbound.vless_encryption_auth_method)
      const command = method === 'mlkem768' ? 'vless-mlkem768' : 'vless-x25519'
      const msg = await HttpUtils.get('api/keypairs', { k: command })
      this.vlessEncryptionRefreshLoading = false

      const lines = Array.isArray(msg.obj) ? msg.obj : []
      if (method === 'mlkem768') {
        const seed = this.parseKeypairValue(lines, 'Seed')
        const client = this.parseKeypairValue(lines, 'Client')
        if (!seed || !client) {
          push.warning({
            title: 'VLESS Encryption',
            duration: 7000,
            message: 'Failed to generate ML-KEM-768 key pair.',
          })
          return
        }
        inbound.vless_encryption_mlkem_seed = seed
        inbound.vless_encryption_mlkem_client = client
        inbound.vless_encryption_x25519_private_key = ''
        inbound.vless_encryption_x25519_password = ''
      } else {
        const privateKey = this.parseKeypairValue(lines, 'PrivateKey')
        const password = this.parseKeypairValue(lines, 'Password')
        if (!privateKey || !password) {
          push.warning({
            title: 'VLESS Encryption',
            duration: 7000,
            message: 'Failed to generate X25519 key pair.',
          })
          return
        }
        inbound.vless_encryption_x25519_private_key = privateKey
        inbound.vless_encryption_x25519_password = password
        inbound.vless_encryption_mlkem_seed = ''
        inbound.vless_encryption_mlkem_client = ''
      }

      if (showSuccess) {
        const methodText = method === 'mlkem768' ? 'ML-KEM-768' : 'X25519'
        push.success({
          title: 'VLESS Encryption',
          duration: 3000,
          message: `Generated a new ${methodText} key pair.`,
        })
      }
    },
    initMihomoFastOpenDefaults() {
      if (!this.inbound.out_json) this.inbound.out_json = {}

      if (this.inbound.type === this.inTypes.Hysteria2) {
        if (this.inbound.out_json.mihomo_fast_open === undefined) {
          this.inbound.out_json.mihomo_fast_open = false
        }
        return
      }

      if (this.namespace !== 'mihomo') return

      if (this.inbound.type === this.inTypes.TUIC) {
        if (this.inbound.out_json.mihomo_fast_open === undefined) {
          this.inbound.out_json.mihomo_fast_open = false
        }
        return
      }

      if (this.inbound.type === this.inTypes.Hysteria && this.inbound.out_json.mihomo_fast_open === undefined) {
        this.inbound.out_json.mihomo_fast_open = true
      }
    },
    initProtocolClientDefaults(forceHyBandwidthDefaults: boolean = false) {
      if (!this.HasInData.includes(this.inbound.type)) return
      if (!this.inbound.out_json) this.inbound.out_json = {}
      if (this.inbound.type === this.inTypes.Snell) {
        const inbound = this.inbound as any
        inbound.version = [4, 5].includes(Number(inbound.version)) ? Number(inbound.version) : 5
        if (!this.inbound.out_json.obfs_opts && inbound.obfs_opts?.mode) {
          this.inbound.out_json.obfs_opts = { ...inbound.obfs_opts }
        }
        if (this.inbound.out_json.version === undefined) {
          this.inbound.out_json.version = 5
        }
        if (this.inbound.out_json.reuse === undefined) {
          this.inbound.out_json.reuse = false
        }
      }
      this.initMihomoFastOpenDefaults()
      this.initShadowTlsClientDefaults()
      this.sanitizeMihomoShadowTLSUnsupportedFields()
      this.initAnyTlsClientDefaults()
      this.initTrustTunnelClientDefaults()
      this.initNaiveClientDefaults()
      this.initHysteriaClientBandwidthDefaults(forceHyBandwidthDefaults)
      this.initVLESSMihomoEncryptionDefaults()
    },
    sanitizeMihomoShadowTLSUnsupportedFields() {
      if (this.namespace !== 'mihomo' || this.inbound.type !== this.inTypes.ShadowTLS) return
      const inbound = this.inbound as any
      delete inbound.strict_mode
      delete inbound.wildcard_sni
      delete inbound.handshake_for_server_name
      delete inbound.detour
      delete inbound.tcp_fast_open
      delete inbound.tcp_multi_path
      delete inbound.udp_fragment
      delete inbound.udp_timeout
      if (inbound.ss_config && typeof inbound.ss_config === 'object' && !Array.isArray(inbound.ss_config)) {
        delete inbound.ss_config.network
      }
      if (!inbound.handshake || typeof inbound.handshake !== 'object' || Array.isArray(inbound.handshake)) {
        inbound.handshake = {
          server: '',
          server_port: 443,
        }
      }
      const handshake = inbound.handshake as Record<string, any>
      const dest = typeof handshake.dest === 'string' ? handshake.dest.trim() : ''
      if (dest !== '' && (typeof handshake.server !== 'string' || handshake.server.trim() === '')) {
        let server = dest
        let port: number | undefined
        if (dest.startsWith('[')) {
          const endBracket = dest.indexOf(']')
          if (endBracket > 0) {
            server = dest.slice(1, endBracket)
            const suffix = dest.slice(endBracket + 1)
            if (suffix.startsWith(':')) {
              const parsed = Number.parseInt(suffix.slice(1), 10)
              if (Number.isInteger(parsed) && parsed > 0) {
                port = parsed
              }
            }
          }
        } else {
          const firstColon = dest.indexOf(':')
          const lastColon = dest.lastIndexOf(':')
          if (firstColon > 0 && firstColon === lastColon) {
            server = dest.slice(0, lastColon)
            const parsed = Number.parseInt(dest.slice(lastColon + 1), 10)
            if (Number.isInteger(parsed) && parsed > 0) {
              port = parsed
            }
          }
        }
        handshake.server = server
        if (port !== undefined) {
          handshake.server_port = port
        }
      }
      delete handshake.dest
      delete handshake.proxy
      delete handshake.detour
      if (typeof handshake.server !== 'string') {
        handshake.server = ''
      }
      const rawPort = Number.parseInt(String(handshake.server_port ?? ''), 10)
      handshake.server_port = Number.isInteger(rawPort) && rawPort > 0 ? rawPort : 443
    },
    showPortCheckUnsupportedHint() {
      if (this.portCheckUnsupportedHinted) return
      this.portCheckUnsupportedHinted = true
      push.warning({
        title: "Notice",
        duration: 5000,
        message: "Port occupancy check is supported only on Linux"
      })
    },
    buildSinglePortStatusText(port: number, tcpUsed: boolean, udpUsed: boolean): string {
      if (tcpUsed && udpUsed) {
        return `${port} TCP/UDP is occupied`
      }
      if (tcpUsed) {
        return `${port} TCP is occupied`
      }
      if (udpUsed) {
        return `${port} UDP is occupied`
      }
      return `${port} TCP/UDP is free`
    },
    buildRangePortText(ports: number[]): string {
      if (!ports || ports.length === 0) return "-"
      if (ports.length <= 20) return ports.join(",")
      return `${ports.slice(0, 20).join(",")} ...`
    },
    normalizeMieruPortRange(raw: string): string | undefined {
      if (typeof raw !== 'string') return undefined
      const value = raw.trim().replace(/\uFF1A/g, ':')
      if (value === '' || value.includes(',') || value.includes('\uFF0C')) return undefined
      const normalized = value.replace(/\s+/g, '').replace(/-/g, ':')
      const parts = normalized.split(':')
      if (parts.length !== 2) return undefined
      const start = Number.parseInt(parts[0], 10)
      const end = Number.parseInt(parts[1], 10)
      if (!Number.isInteger(start) || !Number.isInteger(end)) return undefined
      if (start < 1 || end > 65535 || start >= end) return undefined
      return `${start}-${end}`
    },
    validateMihomoMieruClientPortRange(): boolean {
      if (this.namespace !== 'mihomo' || this.inbound.type !== this.inTypes.Mieru) return true
      if (!this.inbound.out_json || typeof this.inbound.out_json !== 'object') {
        this.inbound.out_json = {}
      }
      delete (<any>this.inbound).port_bindings

      const raw = typeof this.inbound.out_json.port_range === 'string'
        ? this.inbound.out_json.port_range.trim()
        : ''
      if (raw === '') {
        delete this.inbound.out_json.port_range
        delete (<any>this.inbound).port_range
        return true
      }

      const normalized = this.normalizeMieruPortRange(raw)
      if (!normalized) {
        push.warning({
          title: "Mieru",
          duration: 7000,
          message: "Port range must be a single range like 400-500 (supports 400:500 / 400-500 / 400：500)"
        })
        return false
      }
      this.inbound.out_json.port_range = normalized
      ;(<any>this.inbound).port_range = normalized
      return true
    },
    async handleListenPortBlur(portValue: number | string) {
      const parsed = Number(portValue)
      if (!Number.isInteger(parsed) || parsed < 1 || parsed > 65535) return

      const seq = ++this.singlePortCheckSeq
      const resp = await checkPortOccupancy({
        single_ports: [parsed]
      })
      if (seq !== this.singlePortCheckSeq || !resp) return
      if (!resp.supported) {
        this.showPortCheckUnsupportedHint()
        return
      }

      const status = resp.single.find((item) => item.port === parsed)
      if (!status) return

      const text = this.buildSinglePortStatusText(parsed, status.tcp, status.udp)
      if (status.tcp || status.udp) {
        push.warning({
          title: "Port Check",
          duration: 5000,
          message: text
        })
      } else {
        push.success({
          title: "Port Check",
          duration: 5000,
          message: text
        })
      }
    },
    async handlePortHopRangeBlur(rangeValue: string) {
      if (this.inbound.type !== this.inTypes.Hysteria && this.inbound.type !== this.inTypes.Hysteria2) return
      const range = (rangeValue ?? "").trim()
      if (range === "") return

      const seq = ++this.portRangeCheckSeq
      const resp = await checkPortOccupancy({
        udp_ranges: [{
          id: `${this.inbound.id || 0}`,
          tag: this.inbound.tag ?? "",
          range: range
        }]
      })
      if (seq !== this.portRangeCheckSeq || !resp) return
      if (!resp.supported) {
        this.showPortCheckUnsupportedHint()
        return
      }

      const status = resp.udp_ranges?.[0]
      if (!status) return

      if (!status.valid) {
        push.warning({
          title: "Port Check",
          duration: 7000,
          message: `Invalid range format. Example: ${PORT_RANGE_TEMPLATE}`
        })
        return
      }

      if (status.occupied_count > 0) {
        push.warning({
          title: "Port Check",
          duration: 7000,
          message: `UDP range occupied: ${this.buildRangePortText(status.occupied_ports)}`
        })
      } else {
        push.success({
          title: "Port Check",
          duration: 5000,
          message: `UDP range is free: ${status.normalized || status.input}`
        })
      }
    },
    async loadData(id: number) {
      this.loading = true
      const inboundArray = await getNamespaceStore(this.namespace).loadInbounds([id])
      this.inbound = inboundArray[0]
      this.initProtocolClientDefaults(false)
      this.loading = false
    },
    updateData(id: number) {
      if (id > 0) {
        this.loadData(id)
        this.title = "edit"
      }
      else {
        const port = RandomUtil.randomIntRange(10000, 60000)
        const defaultType = this.namespace === 'mihomo' ? this.inTypes.Mixed : this.inTypes.Direct
        this.inbound = createInbound(defaultType,{ id: 0, tag: `${defaultType}-${port}` ,listen: "::", listen_port: port })
        if (this.HasInData.includes(this.inbound.type)){
          this.inbound.addrs = []
          this.inbound.out_json = {}
        } else {
          delete this.inbound.addrs
          delete this.inbound.out_json
        }
        this.initProtocolClientDefaults(true)
        this.title = "add"
        this.loading = false
      }
      this.side = "s"
      this.initUsers = {
        model: 'none',
        values: [],
      }
    },
    supportsMihomoCommonBBRProfile(protocol: string | undefined) {
      const normalized = typeof protocol === 'string' ? protocol.trim().toLowerCase() : ''
      return ['hysteria2', 'tuic', 'trusttunnel', 'masque'].includes(normalized)
    },
    ensureMihomoCommonStore(root: any, protocol?: string) {
      if (!root || typeof root !== 'object') return {}
      if (!root.mihomo_common || typeof root.mihomo_common !== 'object' || Array.isArray(root.mihomo_common)) {
        root.mihomo_common = {}
      }

      const common = root.mihomo_common
      const commonKeys = ['udp', 'ip_version', 'routing_mark', 'tcp_fast_open', 'tcp_multi_path']
      for (const key of commonKeys) {
        if (common[key] === undefined && root[key] !== undefined) {
          common[key] = root[key]
        }
        if (root[key] !== undefined) {
          delete root[key]
        }
      }

      if (this.supportsMihomoCommonBBRProfile(protocol)) {
        const legacyCommonProfile = common['bbr-profile']
        if (common.bbr_profile === undefined && legacyCommonProfile !== undefined) {
          common.bbr_profile = legacyCommonProfile
        }
        if (common.bbr_profile === undefined) {
          if (root.bbr_profile !== undefined) {
            common.bbr_profile = root.bbr_profile
          } else if (root['bbr-profile'] !== undefined) {
            common.bbr_profile = root['bbr-profile']
          }
        }
      } else {
        delete common.bbr_profile
      }
      delete common['bbr-profile']
      delete root.bbr_profile
      delete root['bbr-profile']

      if (!common.smux || typeof common.smux !== 'object' || Array.isArray(common.smux)) {
        if (root.multiplex && typeof root.multiplex === 'object' && !Array.isArray(root.multiplex)) {
          common.smux = JSON.parse(JSON.stringify(root.multiplex))
        } else {
          common.smux = undefined
        }
      }
      if (root.multiplex !== undefined) {
        delete root.multiplex
      }

      return common
    },
    changeType() {
      if (!this.inbound.listen_port) this.inbound.listen_port = RandomUtil.randomIntRange(10000, 60000)
      // Tag change only in add inbound
      const currentId = this.$props.id ?? 0
      const tag = currentId > 0 ? this.inbound.tag : this.inbound.type + "-" + this.inbound.listen_port
      // Use previous data
      const prevConfig = { id: this.inbound.id, tag: tag, listen: this.inbound.listen?? "::", listen_port: this.inbound.listen_port }
      this.inbound = createInbound(this.inbound.type, this.inbound.type != this.inTypes.Tun ? prevConfig : { tag: tag })
      if (this.HasInData.includes(this.inbound.type)){
        this.inbound.addrs = []
        this.inbound.out_json = {}
      } else {
        delete this.inbound.addrs
        delete this.inbound.out_json
      }
      this.initProtocolClientDefaults(true)
      this.side = "s"
    },
    add_addr() {
      this.inbound.addrs?.push(<Addr>{ server: location.hostname, server_port: this.inbound.listen_port })
    },
    closeModal() {
      this.updateData(0) // reset
      this.$emit('close')
    },
    async saveChanges() {
      if (!this.$props.visible) return
      // check duplicate tag
      const store = getNamespaceStore(this.namespace)
      const isDuplicatedTag = store.checkTag('inbound', this.inbound.id, this.inbound.tag)
      if (isDuplicatedTag) return
      if (!this.validateMihomoMieruClientPortRange()) return
      this.sanitizeMihomoShadowTLSUnsupportedFields()
      this.sanitizeVLESSMihomoEncryptionFields()
      if (!this.validateVLESSMihomoEncryption()) return

      // save data
      this.loading = true
      let clientIds = []
      if (this.hasUser) {
        switch (this.initUsers.model) {
          case 'all':
            clientIds = this.clients.map((c:any) => c.id)
            break
          case 'group':
            clientIds = this.clients.filter((c:any) => this.initUsers.values.includes(c.group)).map((c:any) => c.id)
            break
          case 'client':
            clientIds = this.initUsers.values
        }
      }
      const currentId = this.$props.id ?? 0
      const success = await store.save('inbounds', currentId == 0 ? 'new' : 'edit', this.inbound, clientIds)
      if (success) this.closeModal()
      this.loading = false
    },
  },
  computed: {
    isMihomoShadowTLS(): boolean {
      return this.namespace === 'mihomo' && this.inbound.type === this.inTypes.ShadowTLS
    },
    vlessEncryptionEnabled: {
      get(): boolean {
        if (this.namespace !== 'mihomo' || this.inbound.type !== this.inTypes.VLESS) return false
        return (this.inbound as any).vless_encryption_enabled === true
      },
      set(value: boolean) {
        const inbound = this.inbound as any
        const wasEnabled = inbound.vless_encryption_enabled === true
        inbound.vless_encryption_enabled = value
        if (value) {
          this.initVLESSMihomoEncryptionDefaults()
          if (!wasEnabled) {
            void this.refreshVLESSEncryptionKeys(false)
          }
        }
      },
    },
    vlessEncryptionMode: {
      get(): string {
        const inbound = this.inbound as any
        return this.normalizeVLESSMihomoEncryptionMode(inbound.vless_encryption_mode)
      },
      set(value: string) {
        ;(this.inbound as any).vless_encryption_mode = this.normalizeVLESSMihomoEncryptionMode(value)
      },
    },
    vlessEncryptionServerRTT: {
      get(): string {
        const inbound = this.inbound as any
        const value = inbound.vless_encryption_server_rtt
        if (value === undefined || value === null) {
          return this.vlessEncryptionDefaultServerRTT
        }
        return this.normalizeVLESSMihomoEncryptionServerRTT(String(value), true)
      },
      set(value: string) {
        ;(this.inbound as any).vless_encryption_server_rtt =
          typeof value === 'string' ? value.trim().toLowerCase() : ''
      },
    },
    vlessEncryptionClientRTT: {
      get(): string {
        const inbound = this.inbound as any
        return this.normalizeVLESSMihomoEncryptionClientRTT(inbound.vless_encryption_client_rtt)
      },
      set(value: string) {
        ;(this.inbound as any).vless_encryption_client_rtt = this.normalizeVLESSMihomoEncryptionClientRTT(value)
      },
    },
    vlessEncryptionAuthMethod: {
      get(): string {
        const inbound = this.inbound as any
        return this.normalizeVLESSMihomoEncryptionAuthMethod(inbound.vless_encryption_auth_method)
      },
      set(value: string) {
        const inbound = this.inbound as any
        const normalized = this.normalizeVLESSMihomoEncryptionAuthMethod(value)
        const changed = inbound.vless_encryption_auth_method !== normalized
        inbound.vless_encryption_auth_method = normalized
        this.clearInactiveVLESSMihomoEncryptionKeys()
        if (changed && this.vlessEncryptionEnabled) {
          void this.refreshVLESSEncryptionKeys(false)
        }
      },
    },
    vlessEncryptionPadding: {
      get(): string {
        const inbound = this.inbound as any
        const value = typeof inbound.vless_encryption_padding === 'string' ? inbound.vless_encryption_padding.trim() : ''
        return value || this.vlessEncryptionDefaultPadding
      },
      set(value: string) {
        ;(this.inbound as any).vless_encryption_padding = value
      },
    },
    vlessEncryptionServerKeyLabel(): string {
      return this.vlessEncryptionAuthMethod === 'mlkem768' ? 'ML-KEM Seed (Server)' : 'X25519 PrivateKey (Server)'
    },
    vlessEncryptionClientKeyLabel(): string {
      return this.vlessEncryptionAuthMethod === 'mlkem768' ? 'ML-KEM Client (Client)' : 'X25519 Password (Client)'
    },
    vlessEncryptionServerKeyValue(): string {
      const inbound = this.inbound as any
      if (this.vlessEncryptionAuthMethod === 'mlkem768') {
        const value = inbound.vless_encryption_mlkem_seed
        return typeof value === 'string' ? value : ''
      }
      const value = inbound.vless_encryption_x25519_private_key
      return typeof value === 'string' ? value : ''
    },
    vlessEncryptionClientKeyValue(): string {
      const inbound = this.inbound as any
      if (this.vlessEncryptionAuthMethod === 'mlkem768') {
        const value = inbound.vless_encryption_mlkem_client
        return typeof value === 'string' ? value : ''
      }
      const value = inbound.vless_encryption_x25519_password
      return typeof value === 'string' ? value : ''
    },
    inTypeItems() {
      const entries = Object.entries(this.inTypes)
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

      moveEntry(this.inTypes.Hysteria2, this.inTypes.Hysteria, 'before')
      moveEntry(this.inTypes.TrustTunnel, this.inTypes.Hysteria2, 'after')
      moveEntry(this.inTypes.Snell, this.inTypes.Shadowsocks, 'before')
      moveEntry(this.inTypes.ShadowTLS, this.inTypes.Shadowsocks, 'after')
      moveEntry(this.inTypes.Sudoku, this.inTypes.Mieru, 'after')

      return entries
        .filter(([, value]) => !unsupportedTypes.has(value) || value === this.inbound.type)
        .map(([key, value]) => ({
          title: value === this.inTypes.SSH ? 'SSH(\u4ec5\u8ba2\u9605\u3001\u51fa\u7ad9)' : key,
          value,
        }))
    },
    validate() {
      if (this.inbound == undefined) return false
      if (this.inbound.tag == "") return false
      if (this.inbound.listen_port > 65535 || this.inbound.listen_port < 1) return false
      if (this.OnlyTLS.includes(this.inbound.type) && this.inbound.tls_id == 0) return false
      return true
    },
    clients() {
      return getNamespaceStore(this.namespace).clients ?? []
    },
    stlsUtlsEnabled: {
      get(): boolean {
        return this.inbound.out_json?.tls?.utls?.enabled === true
      },
      set(v: boolean) {
        if (!this.inbound.out_json) this.inbound.out_json = {}
        if (!this.inbound.out_json.tls) this.inbound.out_json.tls = { enabled: true }
        if (v) {
          this.inbound.out_json.tls.utls = { enabled: true, fingerprint: 'safari' }
        } else {
          delete this.inbound.out_json.tls.utls
        }
      }
    },
    stlsUtlsFingerprint: {
      get(): string {
        return this.inbound.out_json?.tls?.utls?.fingerprint ?? 'safari'
      },
      set(v: string) {
        if (this.inbound.out_json?.tls?.utls) {
          this.inbound.out_json.tls.utls.fingerprint = v
        }
      }
    },
    stlsClientSsConfig() {
      if (this.inbound.type !== this.inTypes.ShadowTLS) return {}
      if (!this.inbound.out_json) this.inbound.out_json = {}
      if (!this.inbound.out_json.ss_config) {
        // Initialize from server-side ss_config if available
        const ssConfig = (<ShadowTLS>this.inbound).ss_config
        if (ssConfig) {
          this.inbound.out_json.ss_config = JSON.parse(JSON.stringify(ssConfig))
        } else {
          this.inbound.out_json.ss_config = {}
        }
      }
      return this.inbound.out_json.ss_config
    },
    clientCommonFieldTarget() {
      if (!this.inbound.out_json) this.inbound.out_json = {}
      if (this.inbound.type === this.inTypes.ShadowTLS) {
        return this.ensureMihomoCommonStore(this.stlsClientSsConfig, this.inbound.type)
      }
      return this.ensureMihomoCommonStore(this.inbound.out_json, this.inbound.type)
    },
    hasUser() {
      if ((this.$props.id ?? 0) > 0) return false
      if (!this.inboundWithUsers.includes(this.inbound.type)) return false
      if (this.namespace === 'mihomo') {
        switch (this.inbound.type) {
        case InTypes.Snell:
        case InTypes.Shadowsocks:
        case InTypes.VMess:
        case InTypes.Trojan:
        case InTypes.ShadowTLS:
        case InTypes.TUIC:
        case InTypes.Hysteria2:
        case InTypes.VLESS:
        case InTypes.AnyTls:
        case InTypes.Mieru:
        case InTypes.Sudoku:
          break
        default:
          return false
        }
      } else if (this.inbound.type == InTypes.ShadowTLS && (<ShadowTLS>this.inbound).version < 3 ) return false
      if ((<any>this.inbound).managed) return false
      return true
    }
  },
  watch: {
    visible(newValue) {
      if (newValue) {
        this.loading = true
      }
    },
  },
  components: {
    Listen, InTls, Hysteria2, TrustTunnel, Naive, Direct, Shadowsocks,
    Users, Hysteria, ShadowTls, Snell, TProxy, Multiplex, Tuic, Tun,
    AnyTls, SshInbound, Mieru, Sudoku, Transport, AddrVue, OutJsonVue, MihomoClientCommonFields
  }
}
</script>
