<template>
  <v-dialog transition="dialog-bottom-transition" width="800" max-width="95vw" :persistent="loading || saving || applyingCertificate">
    <v-card class="rounded-lg">
      <v-card-title>
        {{ $t('actions.' + title) + " " + $t('objects.tls') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 0 16px; max-height: min(72vh, 760px); overflow-y: auto;">
        <v-card class="rounded-lg">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                :label="$t('client.name')"
                hide-details
                v-model="tls.name">
              </v-text-field>
            </v-col>
            <v-col align="end">
              <v-btn-toggle :model-value="tlsType"
              class="rounded-xl mihomo-tls-mode-toggle"
              density="compact"
              variant="outlined"
              :disabled="loading || saving || applyingCertificate"
              @update:model-value="changeTlsType"
              shaped
              mandatory>
                <v-btn :value="0">TLS</v-btn>
                <v-btn :value="1">Reality</v-btn>
                <v-btn v-if="isMihomoNamespace" :value="2">ShadowTLS</v-btn>
                <v-btn v-if="isMihomoNamespace" :value="3">Restls</v-btn>
                <v-btn v-if="isMihomoNamespace" :value="4">JLS</v-btn>
              </v-btn-toggle>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4" v-if="inTls.server_name != undefined">
              <v-text-field
                label="SNI"
                hide-details
                v-model="inTls.server_name">
              </v-text-field>
            </v-col>
            <template v-if="tlsType == 0">
              <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && inTls.min_version != undefined">
                <v-select
                  hide-details
                  :label="$t('tls.minVer')"
                  :items="tlsVersions"
                  v-model="inTls.min_version">
                </v-select>
              </v-col>
              <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && inTls.max_version != undefined">
                <v-select
                  hide-details
                  :label="$t('tls.maxVer')"
                  :items="tlsVersions"
                  v-model="inTls.max_version">
                </v-select>
              </v-col>
              <v-col cols="12" sm="6" md="4" v-if="inTls.alpn != undefined">
                <v-select
                  hide-details
                  label="ALPN"
                  multiple
                  :items="alpn"
                  v-model="inTls.alpn">
                </v-select>
              </v-col>
              <v-col cols="12" md="8" v-if="!isMihomoNamespace && inTls.cipher_suites != undefined">
                <v-select
                  hide-details
                  :label="$t('tls.cs')"
                  multiple
                  :items="cipher_suites"
                  v-model="inTls.cipher_suites">
                </v-select>
              </v-col>
            </template>
          </v-row>
          <template v-if="tlsType == 0">
            <v-row>
              <v-col>
                <v-btn-toggle v-model="usePath"
                class="rounded-xl"
                density="compact"
                variant="outlined"
                shaped
                :disabled="applyingCertificate"
                mandatory>
                  <v-btn :value="0">{{ $t('tls.usePath') }}</v-btn>
                  <v-btn :value="1">{{ $t('tls.useText') }}</v-btn>
                  <v-btn
                    :value="2"
                    prepend-icon="mdi-certificate-outline"
                    :loading="loadingCertificates"
                    @click="openCertificateCenter">
                    证书管理中心
                  </v-btn>
                </v-btn-toggle>
              </v-col>
              <v-spacer></v-spacer>
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  density="compact"
                  icon="mdi-key-star"
                  @click="genSelfSigned"
                  :loading="loading"
                  :disabled="applyingCertificate">
                  <v-icon />
                  <v-tooltip activator="parent" location="top">
                    {{ $t('actions.generate') }}
                  </v-tooltip>
                </v-btn>
              </v-col>
            </v-row>
            <v-row v-if="usePath == 2">
              <v-col cols="12">
                <v-select
                  v-model="selectedCertificateRecordId"
                  :items="certificateOptions"
                  item-title="mainDomain"
                  item-value="id"
                  :loading="loadingCertificates"
                  :disabled="applyingCertificate"
                  :menu-props="{ maxHeight: 300 }"
                  label="证书管理中心"
                  placeholder="请选择证书"
                  clearable
                  hide-details
                  class="tls-certificate-select"
                  @update:model-value="onCertificateRecordChanged">
                  <template #selection="{ item }">
                    <v-chip
                      size="small"
                      variant="tonal">
                      {{ item.raw.displayId }} / {{ item.raw.mainDomain }}
                    </v-chip>
                  </template>
                  <template #item="{ props: itemProps, item }">
                    <v-list-item v-bind="itemProps">
                      <template #prepend>
                        <v-icon
                          :icon="selectedCertificateRecordId === item.raw.id ? 'mdi-checkbox-marked-outline' : 'mdi-checkbox-blank-outline'" />
                      </template>
                      <v-list-item-title>{{ item.raw.displayId }} / {{ item.raw.mainDomain }}</v-list-item-title>
                      <v-list-item-subtitle>{{ certificateOptionSubtitle(item.raw) }}</v-list-item-subtitle>
                      <template #append>
                        <v-chip
                          size="x-small"
                          variant="tonal">
                          {{ item.raw.status || item.raw.sourceType || '-' }}
                        </v-chip>
                      </template>
                    </v-list-item>
                  </template>
                  <template #no-data>
                    <div class="text-caption px-4 py-2 text-medium-emphasis">证书管理中心暂无证书</div>
                  </template>
                </v-select>
              </v-col>
            </v-row>
            <v-row v-if="usePath == 0">
              <v-col cols="12" sm="6">
                <v-text-field
                  :label="$t('tls.certPath')"
                  hide-details
                  v-model="inTls.certificate_path">
                </v-text-field>
              </v-col>
              <v-col cols="12" sm="6">
                <v-text-field
                  :label="$t('tls.keyPath')"
                  hide-details
                  v-model="inTls.key_path">
                </v-text-field>
              </v-col>
            </v-row>
            <v-row v-else-if="usePath == 1">
              <v-col cols="12">
                <v-textarea
                  :label="$t('tls.cert')"
                  hide-details
                  v-model="certText">
                </v-textarea>
              </v-col>
              <v-col cols="12">
                <v-textarea
                  :label="$t('tls.key')"
                  hide-details
                  v-model="keyText">
                </v-textarea>
              </v-col>
            </v-row>
            <v-row>
              <v-col cols="12" sm="6" md="4">
                <v-switch color="primary" :label="$t('tls.disableSni')" v-model="disableSni" hide-details></v-switch>
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-switch color="primary" :label="$t('tls.insecure')" v-model="insecure" hide-details></v-switch>
              </v-col>
              <v-col cols="12" sm="6" md="4" v-if="showTLSTemplateSelect">
                <v-select
                  hide-details
                  clearable
                  label="证书模板"
                  :items="tlsTemplateOptions"
                  v-model="selectedTLSTemplateCode">
                </v-select>
              </v-col>
            </v-row>
            <v-row>
              <v-col cols="12" sm="6" md="4">
                <v-row no-gutters>
                  <v-col cols="7">
                    <v-text-field
                      :label="$t('tls.certDuration')"
                      type="number"
                      min="1"
                      hide-details
                      v-model.number="certDuration">
                    </v-text-field>
                  </v-col>
                  <v-col cols="5">
                    <v-select
                      hide-details
                      :items="certDurationUnits"
                      v-model="certDurationUnit">
                    </v-select>
                  </v-col>
                </v-row>
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-select
                  hide-details
                  :label="$t('tls.sigAlg')"
                  :items="certAlgorithms"
                  v-model="certSigAlg">
                </v-select>
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-select
                  hide-details
                  :label="$t('tls.keyAlg')"
                  :items="certAlgorithms"
                  v-model="certKeyAlg">
                </v-select>
              </v-col>
            </v-row>
            <template v-if="!isMihomoNamespace">
              <v-divider class="my-2"></v-divider>
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-select
                    hide-details
                    :label="$t('tls.clientAuth')"
                    :items="clientAuthModes"
                    v-model="clientAuthentication">
                  </v-select>
                </v-col>
              </v-row>
            </template>
            <!-- mTLS 客户端证书区域：当客户端认证非 No 时显示 -->
            <template v-if="!isMihomoNamespace && clientAuthentication !== 'no'">
              <v-divider class="my-2"></v-divider>
              <v-row>
                <v-col>
                  <v-btn-toggle v-model="clientCertUsePath"
                  class="rounded-xl"
                  density="compact"
                  variant="outlined"
                  shaped
                  mandatory>
                    <v-btn
                      :value="0"
                      @click="tls.client.client_certificate=undefined; tls.client.client_key=undefined; inTls.client_certificate=undefined"
                    >{{ $t('tls.usePath') }}</v-btn>
                    <v-btn
                      :value="1"
                      @click="tls.client.client_certificate_path=undefined; tls.client.client_key_path=undefined; inTls.client_certificate_path=undefined"
                    >{{ $t('tls.useText') }}</v-btn>
                  </v-btn-toggle>
                </v-col>
                <v-spacer></v-spacer>
                <v-col cols="auto">
                  <v-btn
                    variant="tonal"
                    density="compact"
                    icon="mdi-key-star"
                    @click="genClientCert"
                    :loading="loading">
                    <v-icon />
                    <v-tooltip activator="parent" location="top">
                      {{ $t('actions.generate') }}
                    </v-tooltip>
                  </v-btn>
                </v-col>
              </v-row>
              <v-row v-if="clientCertUsePath == 0">
                <v-col cols="12" sm="6">
                  <v-text-field
                    :label="$t('tls.clientCertPath')"
                    hide-details
                    v-model="clientCertPath">
                  </v-text-field>
                </v-col>
                <v-col cols="12" sm="6">
                  <v-text-field
                    :label="$t('tls.clientKeyPath')"
                    hide-details
                    v-model="tls.client.client_key_path">
                  </v-text-field>
                </v-col>
              </v-row>
              <v-row v-else>
                <v-col cols="12">
                  <v-textarea
                    :label="$t('tls.clientCert')"
                    hide-details
                    v-model="clientCertText">
                  </v-textarea>
                </v-col>
                <v-col cols="12">
                  <v-textarea
                    :label="$t('tls.clientKey')"
                    hide-details
                    v-model="clientKeyText">
                  </v-textarea>
                </v-col>
              </v-row>
            </template>
            <v-row v-if="!isMihomoNamespace && outTls.tls_store != undefined">
              <v-col cols="12" sm="6" md="4">
                <v-select
                  hide-details
                  :label="$t('tls.tlsStore')"
                  :items="tlsStoreOptions"
                  v-model="outTls.tls_store">
                </v-select>
              </v-col>
            </v-row>
            <template v-if="optionSHA256">
              <v-divider class="my-2"></v-divider>
              <v-row>
                <v-col cols="12">
                  <v-row no-gutters align="center">
                    <v-col>
                      <v-text-field
                        :label="$t('tls.serverCertPubkeySha256')"
                        :hint="$t('tls.serverCertPubkeySha256Hint')"
                        persistent-hint
                        v-model="serverCertSha256Text">
                      </v-text-field>
                    </v-col>
                    <v-col cols="auto" class="ps-2">
                      <v-btn
                        variant="tonal"
                        density="compact"
                        :loading="serverSha256Loading"
                        :disabled="serverSha256Loading"
                        @click="generateServerCertSha256">
                        {{ $t('actions.generate') }}
                      </v-btn>
                    </v-col>
                  </v-row>
                </v-col>
              </v-row>
              <v-row v-if="!isMihomoNamespace">
                <v-col cols="12">
                  <v-row no-gutters align="center">
                    <v-col>
                      <v-text-field
                        :label="$t('tls.clientCertPubkeySha256')"
                        :hint="$t('tls.clientCertPubkeySha256Hint')"
                        persistent-hint
                        v-model="clientCertSha256Text">
                      </v-text-field>
                    </v-col>
                    <v-col cols="auto" class="ps-2">
                      <v-btn
                        variant="tonal"
                        density="compact"
                        :loading="clientSha256Loading"
                        :disabled="clientSha256Loading"
                        @click="generateClientCertSha256">
                        {{ $t('actions.generate') }}
                      </v-btn>
                    </v-col>
                  </v-row>
                </v-col>
              </v-row>
            </template>
            <template v-if="optionFingerprint">
              <v-divider class="my-2"></v-divider>
              <v-row>
                <v-col cols="12">
                  <v-row no-gutters align="center">
                    <v-col>
                      <v-text-field
                        label="Certificate Fingerprint"
                        hint="Certificate SHA256 fingerprint"
                        persistent-hint
                        :disabled="!verifyClashPublicKey"
                        v-model="serverFingerprintText">
                      </v-text-field>
                    </v-col>
                    <v-col cols="auto" class="ps-2">
                      <v-btn
                        variant="tonal"
                        density="compact"
                        :loading="serverFingerprintLoading"
                        :disabled="!verifyClashPublicKey || serverFingerprintLoading"
                        @click="generateServerFingerprint">
                        {{ $t('actions.generate') }}
                      </v-btn>
                    </v-col>
                  </v-row>
                </v-col>
              </v-row>
            </template>
          </template>
          <template v-if="tlsType == 1 && outTls.reality && inTls.reality">
            <v-row>
              <v-col cols="12" sm="6" md="4">
                <v-text-field
                :label="$t('types.shdwTls.hs')"
                hide-details
                v-model="inTls.reality.handshake.server">
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
              <v-spacer></v-spacer>
              <v-col cols="auto">
                <v-btn
                  variant="tonal"
                  density="compact"
                  icon="mdi-key-star"
                  @click="genRealityKey"
                  :loading="loading">
                  <v-icon />
                  <v-tooltip activator="parent" location="top">
                    {{ $t('actions.generate') }}
                  </v-tooltip>
                </v-btn>
              </v-col>
            </v-row>
            <v-row>
              <v-col cols="12">
                <v-text-field
                  :label="$t('tls.privKey')"
                  hide-details
                  v-model="inTls.reality.private_key">
                </v-text-field>
              </v-col>
              <v-col cols="12">
                <v-text-field
                  :label="$t('tls.pubKey')"
                  hide-details
                  v-model="outTls.reality.public_key">
                </v-text-field>
              </v-col>
              <v-col cols="12">
                <v-text-field
                  label="Short IDs"
                  hide-details
                  append-icon="mdi-refresh"
                  @click:append="randomSID"
                  v-model="short_id">
                </v-text-field>
              </v-col>
              <v-col cols="12" sm="6" md="4" v-if="optionTime">
                <v-text-field
                label="Max Time Difference"
                type="number"
                min="1"
                step="1"
                :suffix="$t('date.m')"
                hide-details
                v-model.number="max_time">
                </v-text-field>
              </v-col>
            </v-row>
          </template>
          <MihomoShadowTlsVue
            v-if="tlsType == 2"
            ref="mihomoShadowTlsEditor"
            :server="inTls.shadow_tls || {}"
            :client="outTls.shadow_tls_opts || {}"
            :server-name="outTls.server_name || ''"
            :has-server-name="Object.prototype.hasOwnProperty.call(outTls, 'server_name')"
            @update:server-name="outTls.server_name = $event"
            @refresh-credentials="refreshMihomoTlsCredentials"
            @refresh-username="refreshMihomoShadowTlsUsername"
            @refresh-password="refreshMihomoShadowTlsPassword" />
          <MihomoRestlsVue
            v-if="tlsType == 3"
            :server="inTls.res_tls || {}"
            :client="outTls.restls_opts || {}"
            :server-name="outTls.server_name || ''"
            :has-server-name="Object.prototype.hasOwnProperty.call(outTls, 'server_name')"
            @update:server-name="outTls.server_name = $event"
            @refresh-credentials="refreshMihomoTlsCredentials" />
          <MihomoJlsVue
            v-if="tlsType == 4"
            :server="inTls.jls_config || {}"
            :client="outTls.jls_opts || {}"
            :server-name="outTls.server_name || ''"
            :has-server-name="Object.prototype.hasOwnProperty.call(outTls, 'server_name')"
            @update:server-name="outTls.server_name = $event"
            @refresh-username="refreshMihomoJlsUsername"
            @refresh-password="refreshMihomoJlsPassword" />
          <v-row v-if="tlsType <= 1 && outTls.utls != undefined">
            <v-col cols="12" sm="6" md="4">
              <v-select
                hide-details
                label="Fingerprint"
                :items="fingerprints"
                v-model="outTls.utls.fingerprint">
              </v-select>
            </v-col>
          </v-row>
          <v-card-actions>
            <v-spacer></v-spacer>
            <v-menu v-if="tlsType <= 1" v-model="menu" :close-on-content-click="false" location="start">
              <template v-slot:activator="{ props }">
                <v-btn v-bind="props" hide-details variant="tonal">{{ $t('tls.options') }}</v-btn>
              </template>
              <v-card>
                <v-list>
                  <template v-if="tlsType == 0">
                    <v-list-item v-if="!isMihomoNamespace">
                      <v-switch v-model="optionTlsStore" color="primary" label="TLS Store" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item>
                      <v-switch v-model="optionSNI" color="primary" label="SNI" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item>
                      <v-switch v-model="optionALPN" color="primary" label="ALPN" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item v-if="!isMihomoNamespace">
                      <v-switch v-model="optionMinV" color="primary" :label="$t('tls.minVer')" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item v-if="!isMihomoNamespace">
                      <v-switch v-model="optionMaxV" color="primary" :label="$t('tls.maxVer')" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item v-if="!isMihomoNamespace">
                      <v-switch v-model="optionCS" color="primary" :label="$t('tls.cs')" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item>
                      <v-switch v-model="optionFP" color="primary" label="UTLS" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item>
                      <v-switch v-model="optionSHA256" color="primary" label="SHA256" :disabled="!verifyPublicKey" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item>
                      <v-switch v-model="verifyPublicKey" color="primary" :label="$t('tls.verifyPublicKey')" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item>
                      <v-switch v-model="optionFingerprint" color="primary" label="Certificate Fingerprint" :disabled="!verifyClashPublicKey" hide-details></v-switch>
                    </v-list-item>
                    <v-list-item>
                      <v-switch v-model="verifyClashPublicKey" color="primary" :label="$t('tls.verifyClashPublicKey')" hide-details></v-switch>
                    </v-list-item>
                  </template>
                  <template v-else-if="tlsType == 1">
                    <v-list-item>
                      <v-switch v-model="optionTime" color="primary" label="Max Time Difference" hide-details></v-switch>
                    </v-list-item>
                  </template>
                </v-list>
              </v-card>
            </v-menu>
          </v-card-actions>
        </v-card>
        <AcmeVue v-if="showEmbeddedAcme && tlsType == 0" :tls="inTls" />
        <EchVue v-if="tlsType === 0" :iTls="inTls" :oTls="outTls" />
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          color="primary"
          variant="outlined"
          :disabled="loading || saving"
          @click="closeModal"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="loading || saving"
          :disabled="loading || saving"
          @click="saveChanges"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { tls, iTls, defaultInTls, oTls, defaultOutTls, defaultMihomoTlsRateLimit, mihomoTlsSniFromDestination, normalizeMihomoTlsDestination, sanitizeTlsForNamespace, mihomoShadowTlsServer } from '@/types/tls'
import AcmeVue from '@/components/tls/Acme.vue'
import EchVue from '@/components/tls/Ech.vue'
import MihomoShadowTlsVue from '@/components/tls/MihomoShadowTls.vue'
import MihomoRestlsVue from '@/components/tls/MihomoRestls.vue'
import MihomoJlsVue from '@/components/tls/MihomoJls.vue'
import HttpUtils from '@/plugins/httputil'
import { push } from 'notivue'
import { i18n } from '@/locales'
import RandomUtil from '@/plugins/randomUtil'
import { confirm } from '@/plugins/confirm'

type CertificateOption = {
  id: number
  displayId: number
  listOrderAt: number
  sourceType: string
  mainDomain: string
  domains: string[]
  fullchainPath: string
  certPath: string
  keyPath: string
  fingerprint: string
  status: string
}

type CertificateMaterial = {
  id: number
  mainDomain: string
  sourceType: string
  certPath: string
  keyPath: string
  fullchainPath: string
  chainPath: string
  fullchainPem: string
  keyPem: string
  fingerprint: string
}

type TLSTemplateOption = {
  title: string
  value: string
}

const cloneTlsDefault = <T>(value: T): T => JSON.parse(JSON.stringify(value)) as T

export default {
  props: {
    visible: { type: Boolean, default: false },
    saving: { type: Boolean, default: false },
    data: { type: String, default: '{}' },
    id: { type: Number, default: 0 },
    namespace: { type: String, default: 'default' },
  },
  emits: ['close', 'save'],
  data() {
    return {
      tls: <tls>{
        id: 0,
        name: '',
        server: <iTls>{ enabled: true, server_name: '', alpn: cloneTlsDefault(defaultInTls.alpn) },
        client: <oTls>{},
      },
      showEmbeddedAcme: false,
      title: "add",
      loading: false,
      menu: false,
      tlsType: 0,
      usePath: 0,
      showSHA256: false,
      showFingerprint: false,
      serverSha256Loading: false,
      clientSha256Loading: false,
      serverFingerprintLoading: false,
      loadingCertificates: false,
      loadingTLSTemplates: false,
      applyingCertificate: false,
      applyingCertificateRecord: false,
      selectedCertificateRecordId: 0 as number | null,
      selectedTLSTemplateCode: '',
      certificateOptions: <CertificateOption[]>[],
      tlsTemplateOptions: <TLSTemplateOption[]>[],
      serverCertRefreshTimer: undefined as number | undefined,
      clientCertRefreshTimer: undefined as number | undefined,
      serverCertRefreshController: undefined as AbortController | undefined,
      clientCertRefreshController: undefined as AbortController | undefined,
      serverSha256Controller: undefined as AbortController | undefined,
      clientSha256Controller: undefined as AbortController | undefined,
      serverFingerprintController: undefined as AbortController | undefined,
      certificateMaterialController: undefined as AbortController | undefined,
      serverCertRefreshSeq: 0,
      clientCertRefreshSeq: 0,
      serverSha256Seq: 0,
      clientSha256Seq: 0,
      serverFingerprintSeq: 0,
      certificateMaterialSeq: 0,
      alpn: [
        { title: "H3", value: 'h3' },
        { title: "H2", value: 'h2' },
        { title: "Http/1.1", value: 'http/1.1' },
      ],
      tlsVersions: [ '1.0', '1.1', '1.2', '1.3' ],
      cipher_suites: [
        { title: "RSA-AES128-CBC-SHA", value: "TLS_RSA_WITH_AES_128_CBC_SHA" },
        { title: "RSA-AES256-CBC-SHA", value: "TLS_RSA_WITH_AES_256_CBC_SHA" },
        { title: "RSA-AES128-GCM-SHA256", value: "TLS_RSA_WITH_AES_128_GCM_SHA256" },
        { title: "RSA-AES256-GCM-SHA384", value: "TLS_RSA_WITH_AES_256_GCM_SHA384" },
        { title: "AES128-GCM-SHA256", value: "TLS_AES_128_GCM_SHA256" },
        { title: "AES256-GCM-SHA384", value: "TLS_AES_256_GCM_SHA384" },
        { title: "CHACHA20-POLY1305-SHA256", value: "TLS_CHACHA20_POLY1305_SHA256" },
        { title: "ECDHE-ECDSA-AES128-CBC-SHA", value: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA" },
        { title: "ECDHE-ECDSA-AES256-CBC-SHA", value: "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA" },
        { title: "ECDHE-RSA-AES128-CBC-SHA", value: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA" },
        { title: "ECDHE-RSA-AES256-CBC-SHA", value: "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA" },
        { title: "ECDHE-ECDSA-AES128-GCM-SHA256", value: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256" },
        { title: "ECDHE-ECDSA-AES256-GCM-SHA384", value: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384" },
        { title: "ECDHE-RSA-AES128-GCM-SHA256", value: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256" },
        { title: "ECDHE-RSA-AES256-GCM-SHA384", value: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384" },
        { title: "ECDHE-ECDSA-CHACHA20-POLY1305-SHA256", value: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256" },
        { title: "ECDHE-RSA-CHACHA20-POLY1305-SHA256", value: "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256" }
      ],
      fingerprints: [
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
      certDuration: 1,
      certDurationUnit: 'y',
      certDurationUnits: [
        { title: '年', value: 'y' },
        { title: '月', value: 'm' },
        { title: '天', value: 'd' },
      ],
      certSigAlg: 'ecc256',
      certKeyAlg: 'ecc256',
      certAlgorithms: [
        { title: 'ECC224', value: 'ecc224' },
        { title: 'ECC256', value: 'ecc256' },
        { title: 'ECC384', value: 'ecc384' },
        { title: 'ECC521', value: 'ecc521' },
        { title: 'RSA1024', value: 'rsa1024' },
        { title: 'RSA2048', value: 'rsa2048' },
        { title: 'RSA3072', value: 'rsa3072' },
        { title: 'RSA4096', value: 'rsa4096' },
        { title: 'RSA8192', value: 'rsa8192' },
      ],
      clientCaUsePath: 0,
      clientCertUsePath: 0,
      clientAuthModes: [
        { title: 'No', value: 'no' },
        { title: 'Request', value: 'request' },
        { title: 'Require Any', value: 'require-any' },
        { title: 'Verify If Given', value: 'verify-if-given' },
        { title: 'Require & Verify', value: 'require-and-verify' },
      ],
      tlsStoreOptions: [
        { title: 'System', value: 'system' },
        { title: 'Mozilla', value: 'mozilla' },
        { title: 'Chrome', value: 'chrome' },
        { title: 'None', value: 'none' },
      ]
    }
  },
  methods: {
    modeNameForType(type: number): string {
      return ['tls', 'reality', 'shadow-tls', 'restls', 'jls'][type] ?? 'tls'
    },
    resolveMihomoTlsType(value: any): number {
      const mode = typeof value?.mode === 'string' ? value.mode : ''
      if (mode === 'reality') return 1
      if (mode === 'shadow-tls') return 2
      if (mode === 'restls') return 3
      if (mode === 'jls') return 4
      if (!this.isMihomoNamespace && value?.server?.reality && typeof value.server.reality === 'object' && !Array.isArray(value.server.reality)) {
        return 1
      }
      return 0
    },
    ensureMihomoTlsModeDefaults(type: number) {
      if (type === 1) {
        this.tls.server = {
          enabled: true,
          reality: this.tls.server?.reality ?? ({ enabled: true, handshake: { server: '', server_port: 443 }, private_key: '', short_id: RandomUtil.randomShortId() } as any),
          server_name: this.tls.server?.server_name ?? '',
        }
        this.tls.client = {
          ...(this.tls.client ?? {}),
          reality: this.tls.client?.reality ?? { enabled: true, public_key: '', short_id: '' },
          utls: this.tls.client?.utls ?? cloneTlsDefault(defaultOutTls.utls),
        }
        return
      }
      if (type === 2) {
        const sourceShadowTls = this.tls.server?.shadow_tls
        const shadowTls: mihomoShadowTlsServer = (sourceShadowTls && typeof sourceShadowTls === 'object' && !Array.isArray(sourceShadowTls))
          ? { ...sourceShadowTls }
          : { enable: true, version: 3, users: [{ name: '', password: '' }], handshake: { dest: '' }, strict_mode: true, wildcard_sni: 'off' }
        if (!shadowTls.handshake || typeof shadowTls.handshake !== 'object' || Array.isArray(shadowTls.handshake)) {
          shadowTls.handshake = { dest: '' }
        }
        if (!shadowTls.handshake.dest) shadowTls.handshake.dest = ''
        const shadowVersion = [1, 2, 3].includes(Number(shadowTls.version)) ? Number(shadowTls.version) : 3
        shadowTls.version = shadowVersion
        if (shadowVersion !== 3) {
          delete (shadowTls as any).strict_mode
          delete (shadowTls as any).wildcard_sni
        }
        if (shadowVersion <= 1) {
          delete (shadowTls as any).handshake_for_server_name
        }
        if (shadowVersion === 3) {
          const firstUser = Array.isArray(shadowTls.users) ? shadowTls.users[0] : undefined
          const clientPassword = String(this.tls.client?.shadow_tls_opts?.password ?? '')
          shadowTls.users = [{
            name: String(firstUser?.name ?? firstUser?.username ?? ''),
            password: String(firstUser?.password ?? clientPassword),
          }]
          delete shadowTls.password
        } else {
          delete shadowTls.users
          if (shadowVersion === 1) delete shadowTls.password
          else {
            const serverPassword = String(shadowTls.password ?? '')
            const clientPassword = String(this.tls.client?.shadow_tls_opts?.password ?? '')
            shadowTls.password = serverPassword.trim() !== '' ? serverPassword : clientPassword
          }
        }
        if (shadowVersion === 3 && shadowTls.wildcard_sni === undefined) shadowTls.wildcard_sni = 'off'
        const shadowOpts = this.tls.client?.shadow_tls_opts && typeof this.tls.client.shadow_tls_opts === 'object'
          ? { ...this.tls.client.shadow_tls_opts, version: shadowVersion }
          : { version: shadowVersion }
        if (shadowVersion === 1) delete (shadowOpts as any).password
        else if (!(shadowOpts as any).password) {
          const fallbackPassword = shadowVersion === 2
            ? shadowTls.password
            : shadowTls.users?.[0]?.password
          ;(shadowOpts as any).password = String(fallbackPassword ?? '')
        }
        this.tls.server = {
          enabled: true,
          shadow_tls: shadowTls,
        }
        this.tls.client = { ...(this.tls.client ?? {}), shadow_tls_opts: shadowOpts }
        return
      }
      if (type === 3) {
        const sourceRestls = this.tls.server?.res_tls
        const isNewRestls = !sourceRestls || typeof sourceRestls !== 'object' || Array.isArray(sourceRestls)
        const restls = sourceRestls && typeof sourceRestls === 'object' && !Array.isArray(sourceRestls)
          ? { ...sourceRestls }
          : { enable: true, dest: '', password: '', min_record_len: 0, rate_limit: defaultMihomoTlsRateLimit }
        if (isNewRestls) restls.rate_limit = defaultMihomoTlsRateLimit
        const existingRestlsOpts = this.tls.client?.restls_opts && typeof this.tls.client.restls_opts === 'object'
          ? { ...this.tls.client.restls_opts }
          : { password: '', version_hint: 'tls13' as const }
        const serverPassword = String(restls.password ?? '')
        const clientPassword = String(existingRestlsOpts.password ?? '')
        const password = serverPassword.trim() !== '' ? serverPassword : clientPassword
        restls.password = password
        existingRestlsOpts.password = password
        this.tls.server = {
          enabled: true,
          res_tls: restls,
        }
        this.tls.client = { ...(this.tls.client ?? {}), restls_opts: existingRestlsOpts }
        return
      }
      if (type === 4) {
        const sourceJls = this.tls.server?.jls_config
        const isNewJls = !sourceJls || typeof sourceJls !== 'object' || Array.isArray(sourceJls)
        const jls = sourceJls && typeof sourceJls === 'object' && !Array.isArray(sourceJls)
          ? { ...sourceJls }
          : { enable: true, users: [{ username: '', password: '' }], dest: '', alpn: ['h2', 'http/1.1'], rate_limit: defaultMihomoTlsRateLimit }
        if (isNewJls) jls.rate_limit = defaultMihomoTlsRateLimit
        const firstUser = Array.isArray(jls.users) ? jls.users[0] : undefined
        const existingJlsOpts = this.tls.client?.jls_opts && typeof this.tls.client.jls_opts === 'object'
          ? { ...this.tls.client.jls_opts }
          : { username: '', password: '' }
        const serverUsername = String(firstUser?.username ?? '')
        const clientUsername = String(existingJlsOpts.username ?? '')
        const serverPassword = String(firstUser?.password ?? '')
        const clientPassword = String(existingJlsOpts.password ?? '')
        jls.users = [{
          username: serverUsername.trim() !== '' ? serverUsername : clientUsername,
          password: serverPassword.trim() !== '' ? serverPassword : clientPassword,
        }]
        existingJlsOpts.username = jls.users[0].username
        existingJlsOpts.password = jls.users[0].password
        this.tls.server = {
          enabled: true,
          jls_config: jls,
        }
        this.tls.client = { ...(this.tls.client ?? {}), jls_opts: existingJlsOpts }
        return
      }
      this.tls.server = { enabled: true, ...(this.tls.server?.enabled !== undefined ? { enabled: this.tls.server.enabled } : {}) }
      this.tls.client = { ...(this.tls.client ?? {}) }
    },
    refreshMihomoTlsCredentials() {
      if (!this.isMihomoNamespace || this.tlsType < 2) return
      this.ensureMihomoTlsModeDefaults(this.tlsType)
      if (this.tlsType === 2) {
        const cfg: any = this.tls.server.shadow_tls
        const opts: any = this.tls.client.shadow_tls_opts
        const version = Number(cfg?.version)
        if (version !== 3) {
          delete cfg.strict_mode
          delete cfg.wildcard_sni
        }
        if (version <= 1) delete cfg.handshake_for_server_name
        if (version === 1) {
          delete cfg.password
          delete cfg.users
          delete opts.password
          return
        }
        const password = RandomUtil.randomExtendedUUID()
        opts.password = password
        if (version === 2) {
          cfg.password = password
          delete cfg.users
          return
        }
        cfg.users = [{ name: RandomUtil.randomExtendedUUID(), password }]
        delete cfg.password
        return
      }
      if (this.tlsType === 3) {
        const cfg: any = this.tls.server.res_tls
        const opts: any = this.tls.client.restls_opts
        const password = RandomUtil.randomExtendedUUID()
        cfg.password = password
        opts.password = password
        return
      }
      const cfg: any = this.tls.server.jls_config
      const opts: any = this.tls.client.jls_opts
      const username = RandomUtil.randomExtendedUUID()
      const password = RandomUtil.randomExtendedUUID()
      cfg.users = [{ username, password }]
      opts.username = username
      opts.password = password
    },
    refreshMihomoShadowTlsUsername() {
      this.refreshMihomoShadowTlsCredential('username')
    },
    refreshMihomoShadowTlsPassword() {
      this.refreshMihomoShadowTlsCredential('password')
    },
    refreshMihomoShadowTlsCredential(field: 'username' | 'password') {
      if (!this.isMihomoNamespace || this.tlsType !== 2) return
      this.ensureMihomoTlsModeDefaults(2)
      const cfg: any = this.tls.server.shadow_tls
      const opts: any = this.tls.client.shadow_tls_opts
      const version = Number(cfg?.version)
      if (field === 'username') {
        if (version !== 3) return
        const user = Array.isArray(cfg.users) ? cfg.users[0] ?? {} : {}
        const name = RandomUtil.randomExtendedUUID()
        const serverPassword = String(user.password ?? '')
        const clientPassword = String(opts.password ?? '')
        const password = serverPassword.trim() !== '' ? serverPassword : clientPassword
        cfg.users = [{ name, password }]
        delete cfg.password
        opts.password = password
        return
      }
      if (version === 1) return
      const user = Array.isArray(cfg.users) ? cfg.users[0] ?? {} : {}
      const password = RandomUtil.randomExtendedUUID()
      opts.password = password
      if (version === 2) {
        cfg.password = password
        delete cfg.users
        return
      }
      const name = String(user.name ?? user.username ?? '')
      cfg.users = [{ name, password }]
      delete cfg.password
    },
    refreshMihomoJlsUsername() {
      this.refreshMihomoJlsCredential('username')
    },
    refreshMihomoJlsPassword() {
      this.refreshMihomoJlsCredential('password')
    },
    refreshMihomoJlsCredential(field: 'username' | 'password') {
      if (!this.isMihomoNamespace || this.tlsType !== 4) return
      this.ensureMihomoTlsModeDefaults(4)
      const cfg: any = this.tls.server.jls_config
      const opts: any = this.tls.client.jls_opts
      const user = Array.isArray(cfg.users) ? cfg.users[0] ?? {} : {}
      const value = RandomUtil.randomExtendedUUID()
      const username = field === 'username'
        ? value
        : String(user.username ?? opts.username ?? '')
      const password = field === 'password'
        ? value
        : String(user.password ?? opts.password ?? '')
      cfg.users = [{ username, password }]
      opts.username = username
      opts.password = password
    },
    syncMihomoTlsCredentials() {
      if (!this.isMihomoNamespace || this.tlsType < 2) return
      this.ensureMihomoTlsModeDefaults(this.tlsType)
      this.normalizeMihomoTlsDestinations()
      this.syncMihomoTlsSni()
      if (this.tlsType === 2) {
        const cfg: any = this.tls.server.shadow_tls
        const opts: any = this.tls.client.shadow_tls_opts
        const version = Number(cfg?.version)
        if (version !== 3) {
          delete cfg.strict_mode
          delete cfg.wildcard_sni
        }
        if (version <= 1) delete cfg.handshake_for_server_name
        if (version === 1) {
          delete cfg.password
          delete cfg.users
          delete opts.password
        } else if (version === 2) {
          const password = String(cfg.password ?? opts.password ?? '')
          cfg.password = password
          opts.password = password
          delete cfg.users
        } else {
          const user = Array.isArray(cfg.users) ? cfg.users[0] ?? {} : {}
          const name = String(user.name ?? user.username ?? '')
          const password = String(user.password ?? opts.password ?? '')
          cfg.users = [{ name, password }]
          opts.password = password
          delete cfg.password
        }
        return
      }
      if (this.tlsType === 3) {
        const cfg: any = this.tls.server.res_tls
        const opts: any = this.tls.client.restls_opts
        const password = String(cfg.password ?? opts.password ?? '')
        cfg.password = password
        opts.password = password
        return
      }
      const cfg: any = this.tls.server.jls_config
      const opts: any = this.tls.client.jls_opts
      const user = Array.isArray(cfg.users) ? cfg.users[0] ?? {} : {}
      const username = String(user.username ?? opts.username ?? '')
      const password = String(user.password ?? opts.password ?? '')
      cfg.users = [{ username, password }]
      opts.username = username
      opts.password = password
    },
    prepareMihomoShadowTlsRules(): boolean {
      if (!this.isMihomoNamespace || this.tlsType !== 2) return true
      const editor = this.$refs.mihomoShadowTlsEditor as {
        prepareHandshakeRules?: () => { valid: boolean, message?: string }
      } | undefined
      const result = editor?.prepareHandshakeRules?.()
      if (!result || result.valid) return true
      push.warning({ duration: 4000, message: result.message ?? 'ShadowTLS 按 SNI 握手规则无效' })
      return false
    },
    async reloadMihomoShadowTlsRules() {
      if (!this.isMihomoNamespace || this.tlsType !== 2) return
      await this.$nextTick()
      const editor = this.$refs.mihomoShadowTlsEditor as {
        loadHandshakeRulesFromServer?: () => void
      } | undefined
      editor?.loadHandshakeRulesFromServer?.()
    },
    normalizeMihomoTlsDestinations() {
      if (!this.isMihomoNamespace || this.tlsType < 2) return
      const server: any = this.tls.server ?? {}
      if (this.tlsType === 2) {
        const handshake = server.shadow_tls?.handshake
        if (handshake && typeof handshake === 'object') {
          handshake.dest = normalizeMihomoTlsDestination(handshake.dest)
        }
        return
      }
      if (this.tlsType === 3 && server.res_tls && typeof server.res_tls === 'object') {
        server.res_tls.dest = normalizeMihomoTlsDestination(server.res_tls.dest)
        return
      }
      if (this.tlsType === 4 && server.jls_config && typeof server.jls_config === 'object') {
        server.jls_config.dest = normalizeMihomoTlsDestination(server.jls_config.dest)
      }
    },
    syncMihomoTlsSni() {
      if (!this.isMihomoNamespace || this.tlsType < 2) return
      const server: any = this.tls.server ?? {}
      const client: any = this.tls.client ?? {}
      const hasClientSni = typeof client.server_name === 'string'
      const clientSni = hasClientSni ? client.server_name.trim() : ''
      if (this.tlsType !== 4) {
        const destination = this.tlsType === 2
          ? server.shadow_tls?.handshake?.dest
          : server.res_tls?.dest
        const sni = clientSni || mihomoTlsSniFromDestination(destination)
        if (sni === '') delete client.server_name
        else client.server_name = sni
        return
      }

      const config: any = server.jls_config
      if (!config || typeof config !== 'object' || Array.isArray(config)) return
      const serverSni = typeof config.sni === 'string' ? config.sni.trim() : ''
      const fallbackSni = hasClientSni ? mihomoTlsSniFromDestination(config.dest) : (serverSni || mihomoTlsSniFromDestination(config.dest))
      const sni = clientSni || fallbackSni
      if (sni === '') {
        delete client.server_name
        delete config.sni
      } else {
        client.server_name = sni
        config.sni = sni
      }
    },
    clearInactiveMihomoTlsModes() {
      const server: any = this.tls.server ?? {}
      const client: any = this.tls.client ?? {}
      const mode = this.modeNameForType(this.tlsType)
      // Keep the persisted discriminator in lockstep with the visible toggle.
      // sanitizeTlsForNamespace() uses this value to decide which wrapper is
      // active; leaving the previous mode here would delete the newly-created
      // wrapper immediately after a mode switch.
      this.tls.mode = mode as any
      if (mode !== 'reality') delete server.reality
      if (mode !== 'reality') delete client.reality
      if (mode !== 'shadow-tls') { delete server.shadow_tls; delete client.shadow_tls_opts }
      if (mode !== 'restls') { delete server.res_tls; delete client.restls_opts }
      if (mode !== 'jls') { delete server.jls_config; delete client.jls_opts }
      if (mode !== 'tls') {
        delete server.certificate; delete server.certificate_path; delete server.key; delete server.key_path
        delete server.acme; delete server.ech; delete client.ech
        this.tls.certificateRecordId = undefined
      }
      this.tls.server = server
      this.tls.client = client
    },
    validateMihomoTlsMode(): boolean {
      if (!this.isMihomoNamespace || this.tlsType < 2) return true
      const server: any = this.tls.server ?? {}
      const client: any = this.tls.client ?? {}
      if (this.tlsType === 2) {
        const cfg = server.shadow_tls
        const opts = client.shadow_tls_opts
        const version = Number(cfg?.version)
        if (![1, 2, 3].includes(version) || version !== Number(opts?.version)) { push.warning({ duration: 4000, message: 'ShadowTLS 版本必须为 1/2/3 且客户端与服务端一致' }); return false }
        if (version !== 3) {
          delete cfg.strict_mode
          delete cfg.wildcard_sni
        }
        if (version <= 1) delete cfg.handshake_for_server_name
        if (version === 1) delete opts?.password
        const wildcardSni = String(cfg?.wildcard_sni ?? 'off').trim().toLowerCase()
        if (version === 3 && !['off', 'authed', 'all'].includes(wildcardSni)) { push.warning({ duration: 4000, message: 'ShadowTLS wildcard-sni 必须为 off、authed 或 all' }); return false }
        if ((version < 3 || wildcardSni !== 'all') && !cfg?.handshake?.dest) { push.warning({ duration: 4000, message: wildcardSni === 'authed' ? 'ShadowTLS wildcard-sni=authed 仍需要填写默认握手目标' : 'ShadowTLS 需要填写默认握手目标' }); return false }
        if (cfg?.handshake?.dest && !this.isHostPortAddress(cfg.handshake.dest)) { push.warning({ duration: 4000, message: 'ShadowTLS 握手目标必须是 host:port' }); return false }
        const handshakeMap = cfg?.handshake_for_server_name
        if (handshakeMap !== undefined) {
          if (version <= 1) { push.warning({ duration: 4000, message: 'ShadowTLS v1 不支持按 SNI 配置多个握手目标' }); return false }
          if (!handshakeMap || typeof handshakeMap !== 'object' || Array.isArray(handshakeMap) || Object.entries(handshakeMap).some(([name, value]: [string, any]) => !name.trim() || !value || typeof value !== 'object' || Array.isArray(value) || !value.dest || !this.isHostPortAddress(value.dest) || (value.proxy !== undefined && typeof value.proxy !== 'string'))) {
            push.warning({ duration: 4000, message: 'ShadowTLS 按 SNI 握手规则必须包含有效的 SNI 和 host:port 目标' }); return false
          }
        }
        if (version === 2) {
          const serverPassword = String(cfg.password ?? '')
          const clientPassword = String(opts.password ?? '')
          if (!serverPassword.trim() || !clientPassword.trim() || serverPassword !== clientPassword) { push.warning({ duration: 4000, message: 'ShadowTLS v2 客户端与服务端密码必须一致' }); return false }
        }
        if (version === 3) {
          if (!Array.isArray(cfg.users) || cfg.users.length !== 1) { push.warning({ duration: 4000, message: 'ShadowTLS v3 只能配置一个用户' }); return false }
          const user = cfg.users[0]
          if (!String(user?.name ?? '').trim() || !String(user?.password ?? '').trim()) { push.warning({ duration: 4000, message: 'ShadowTLS v3 用户名和密码必须填写' }); return false }
          const clientPassword = String(opts?.password ?? '')
          if (!clientPassword.trim() || String(user.password) !== clientPassword) { push.warning({ duration: 4000, message: 'ShadowTLS v3 客户端密码必须匹配服务端用户' }); return false }
        }
      } else if (this.tlsType === 3) {
        const cfg = server.res_tls
        const opts = client.restls_opts
        const serverPassword = String(cfg?.password ?? '')
        const clientPassword = String(opts?.password ?? '')
        if (!this.isHostPortAddress(cfg?.dest)) { push.warning({ duration: 4000, message: 'Restls 目标地址必须是 host:port' }); return false }
        if (!serverPassword.trim() || !clientPassword.trim() || serverPassword !== clientPassword) { push.warning({ duration: 4000, message: 'Restls 密码必须填写，且客户端与服务端一致' }); return false }
        if (!this.validateNonNegativeInteger(cfg?.min_record_len, 'Restls min-record-len')) return false
        if (!this.validateNonNegativeInteger(cfg?.rate_limit, 'Restls rate-limit')) return false
        if (!['tls12', 'tls13'].includes(String(opts.version_hint ?? '').toLowerCase())) { push.warning({ duration: 4000, message: 'Restls 版本提示必须为 TLS 1.2 或 TLS 1.3' }); return false }
        const serverScript = String(cfg.restls_script ?? '')
        const clientScript = String(opts.restls_script ?? '')
        const script = serverScript.trim() !== '' ? serverScript : clientScript
        if (script.trim() === '') {
          delete cfg.restls_script
          delete opts.restls_script
        } else {
          cfg.restls_script = script
          opts.restls_script = script
        }
      } else if (this.tlsType === 4) {
        const cfg = server.jls_config
        const opts = client.jls_opts
        const username = String(opts?.username ?? '')
        const password = String(opts?.password ?? '')
        if (!this.isHostPortAddress(cfg?.dest)) { push.warning({ duration: 4000, message: 'JLS 目标地址必须是 host:port' }); return false }
        if (!Array.isArray(cfg.users) || cfg.users.length !== 1 || !username.trim() || !password.trim()) { push.warning({ duration: 4000, message: 'JLS 只能配置一个用户名和密码' }); return false }
        const user = cfg.users[0]
        if (user.username !== username || user.password !== password) { push.warning({ duration: 4000, message: 'JLS 服务端与客户端凭据必须一致' }); return false }
        if (!this.validateNonNegativeInteger(cfg.rate_limit, 'JLS rate-limit')) return false
      }
      return true
    },
    validateNonNegativeInteger(value: unknown, label: string): boolean {
      if (value === undefined || value === null || value === '') return true
      if (typeof value !== 'number' || !Number.isSafeInteger(value) || value < 0) {
        push.warning({ duration: 4000, message: `${label} 必须是非负整数` })
        return false
      }
      return true
    },
    isHostPortAddress(value: unknown): boolean {
      const raw = String(value ?? '').trim()
      const match = raw.match(/^\[([^\]]+)\]:(\d+)$/) ?? raw.match(/^([^:]+):(\d+)$/)
      if (!match) return false
      const port = Number(match[2])
      return Number.isInteger(port) && port >= 1 && port <= 65535
    },
    normalizePort(value: unknown, fallback: number = 443): number {
      if (value === '' || value === null || value === undefined) return fallback
      const port = Number(value)
      return Number.isSafeInteger(port) && port >= 1 && port <= 65535 ? port : fallback
    },
    normalizeTlsData(data?: tls | null) {
      const normalized = sanitizeTlsForNamespace(data, this.$props.namespace)
      if (normalized.server == null) normalized.server = { enabled: true }
      if (normalized.client == null) normalized.client = {}
      if (normalized.server.ech && typeof normalized.server.ech === 'object' && !Array.isArray(normalized.server.ech) &&
        (!normalized.client.ech || typeof normalized.client.ech !== 'object' || Array.isArray(normalized.client.ech))) {
        normalized.client.ech = { enabled: true }
      }
      if (normalized.client.store != undefined && normalized.client.tls_store == undefined) {
        normalized.client.tls_store = normalized.client.store
        delete normalized.client.store
      }
      return normalized
    },
    normalizeCertificateOptions(raw: unknown): CertificateOption[] {
      const rows = Array.isArray(raw)
        ? raw
        : Array.isArray((raw as any)?.items)
          ? (raw as any).items
        : Array.isArray((raw as any)?.certificates)
          ? (raw as any).certificates
          : []
      return rows.map((item: any) => ({
        id: Number(item?.id ?? 0),
        displayId: Number(item?.displayId ?? 0),
        listOrderAt: Number(item?.listOrderAt ?? 0),
        sourceType: String(item?.sourceType ?? ''),
        mainDomain: String(item?.mainDomain ?? ''),
        domains: Array.isArray(item?.domains) ? item.domains.map((value: any) => String(value ?? '').trim()).filter((value: string) => value.length > 0) : [],
        fullchainPath: String(item?.fullchainPath ?? ''),
        certPath: String(item?.certPath ?? ''),
        keyPath: String(item?.keyPath ?? ''),
        fingerprint: String(item?.fingerprint ?? ''),
        status: String(item?.status ?? ''),
      }))
        .filter((item: CertificateOption) => item.id > 0)
        .sort((a: CertificateOption, b: CertificateOption) => {
          if (a.listOrderAt !== b.listOrderAt) {
            return b.listOrderAt - a.listOrderAt
          }
          return b.id - a.id
        })
    },
    normalizeTLSTemplateOptions(raw: unknown): TLSTemplateOption[] {
      const rows = Array.isArray(raw)
        ? raw
        : Array.isArray((raw as any)?.templates)
          ? (raw as any).templates
          : []
      return rows
        .map((item: any) => ({
          title: String(item?.name ?? '').trim(),
          value: this.normalizeTLSTemplateCode(item?.code),
        }))
        .filter((item: TLSTemplateOption) => item.title.length > 0 && item.value.length > 0)
    },
    certificateOptionSubtitle(item: CertificateOption): string {
      const domains = item.domains.filter(value => value !== item.mainDomain)
      const source = item.sourceType ? `来源: ${item.sourceType}` : ''
      const domainText = domains.length > 0 ? `其他域名: ${domains.join(', ')}` : '其他域名: 无'
      return [source, domainText].filter(Boolean).join(' / ')
    },
    async loadCertificateOptions() {
      if (this.loadingCertificates) {
        return
      }
      this.loadingCertificates = true
      try {
        const msg = await HttpUtils.get('api/certificate-options')
        if (msg.success) {
          this.certificateOptions = this.normalizeCertificateOptions(msg.obj)
        }
      } finally {
        this.loadingCertificates = false
      }
    },
    async loadTLSTemplateOptions(showError: boolean = false) {
      if (this.loadingTLSTemplates) {
        return
      }
      this.loadingTLSTemplates = true
      try {
        const msg = await HttpUtils.get('api/tlsSelfSignedTemplates')
        if (msg.success) {
          this.tlsTemplateOptions = this.normalizeTLSTemplateOptions(msg.obj)
          if (this.selectedTLSTemplateCode && !this.isKnownTLSTemplateCode(this.selectedTLSTemplateCode)) {
            this.clearTLSTemplateSelection()
          }
          return
        }
        this.tlsTemplateOptions = []
        if (showError) {
          push.warning({
            duration: 4000,
            message: msg.msg || 'TLS template list is unavailable',
          })
        }
      } finally {
        this.loadingTLSTemplates = false
      }
    },
    async ensureTLSTemplateOptions(showError: boolean = false) {
      if (this.tlsTemplateOptions.length > 0 || this.loadingTLSTemplates) {
        return
      }
      await this.loadTLSTemplateOptions(showError)
    },
    async openCertificateCenter() {
      this.usePath = 2
      await this.loadCertificateOptions()
    },
    async onCertificateRecordChanged(value: any) {
      const id = Number(value ?? 0)
      if (!Number.isFinite(id) || id <= 0) {
        this.clearCertificateBinding()
        this.clearServerCertificateMaterial()
        this.clearServerCertDerivedValues()
        return
      }
      await this.onCertificateRecordSelected(id)
    },
    isVirtualCertificatePath(path?: string): boolean {
      const normalized = String(path ?? '').trim().toLowerCase()
      return normalized === '' || normalized.startsWith('sqlite:')
    },
    normalizeMaterial(raw: unknown): CertificateMaterial {
      const item = (raw ?? {}) as any
      return {
        id: Number(item.id ?? 0),
        mainDomain: String(item.mainDomain ?? ''),
        sourceType: String(item.sourceType ?? ''),
        certPath: String(item.certPath ?? ''),
        keyPath: String(item.keyPath ?? ''),
        fullchainPath: String(item.fullchainPath ?? ''),
        chainPath: String(item.chainPath ?? ''),
        fullchainPem: String(item.fullchainPem ?? ''),
        keyPem: String(item.keyPem ?? ''),
        fingerprint: String(item.fingerprint ?? ''),
      }
    },
    clearCertificateBinding() {
      this.tls.certificateRecordId = undefined
      this.selectedCertificateRecordId = null
    },
    clearServerCertificateMaterial() {
      this.inTls.certificate_path = undefined
      this.inTls.key_path = undefined
      this.inTls.certificate = undefined
      this.inTls.key = undefined
    },
    hasServerCertificateMaterial(): boolean {
      const hasPathPair = (this.inTls.certificate_path?.trim().length ?? 0) > 0 && (this.inTls.key_path?.trim().length ?? 0) > 0
      const hasInlinePair = (this.inTls.certificate?.length ?? 0) > 0 && (this.inTls.key?.length ?? 0) > 0
      return hasPathPair || hasInlinePair
    },
    markCertificateBindingManualChange() {
      if (this.applyingCertificateRecord) {
        return
      }
      this.clearCertificateBinding()
    },
    async onCertificateRecordSelected(value: any): Promise<boolean> {
      const id = Number(value ?? 0)
      if (!Number.isFinite(id) || id <= 0) {
        this.clearCertificateBinding()
        return false
      }

      this.certificateMaterialController?.abort()
      const controller = new AbortController()
      this.certificateMaterialController = controller
      const requestId = ++this.certificateMaterialSeq
      this.applyingCertificate = true
      this.applyingCertificateRecord = true
      try {
        let msg
        try {
          msg = await HttpUtils.post('api/certificate-material', { id }, { signal: controller.signal, silentErrorToast: true })
        } catch {
          return false
        }
        if (requestId !== this.certificateMaterialSeq || this.certificateMaterialController !== controller || !this.$props.visible || this.tlsType !== 0) {
          return false
        }
        if (!msg.success || msg.obj == null) {
          this.selectedCertificateRecordId = this.tls.certificateRecordId ?? null
          return false
        }

        const material = this.normalizeMaterial(msg.obj)
        const fullchainPath = material.fullchainPath.trim()
        const certPath = material.certPath.trim()
        const certificatePath = !this.isVirtualCertificatePath(fullchainPath) ? fullchainPath : certPath
        const keyPath = material.keyPath.trim()
        const canUsePath = !this.isVirtualCertificatePath(certificatePath) && !this.isVirtualCertificatePath(keyPath)

        this.clearServerCertDerivedValues()
        if (canUsePath) {
          this.usePath = 2
          this.inTls.certificate_path = certificatePath
          this.inTls.key_path = keyPath
          this.inTls.certificate = undefined
          this.inTls.key = undefined
        } else {
          const certificateLines = this.splitToLines(material.fullchainPem)
          const keyLines = this.splitToLines(material.keyPem)
          if (!certificateLines || !keyLines) {
            push.warning({
              duration: 4000,
              message: '证书内容为空，无法应用到 TLS 设置',
            })
            this.selectedCertificateRecordId = this.tls.certificateRecordId ?? null
            return false
          }
          this.usePath = 2
          this.inTls.certificate = certificateLines
          this.inTls.key = keyLines
          this.inTls.certificate_path = undefined
          this.inTls.key_path = undefined
        }

        this.tls.certificateRecordId = id
        this.selectedCertificateRecordId = id
        await this.refreshServerCertDerivedFields()
        return true
      } finally {
        if (this.certificateMaterialController === controller) {
          this.certificateMaterialController = undefined
          this.applyingCertificateRecord = false
          this.applyingCertificate = false
        }
      }
    },
    async updateData(id: number) {
      this.clearPendingCertRefreshes()
      this.certSigAlg = 'ecc256'
      this.certKeyAlg = 'ecc256'
      this.clearTLSTemplateSelection()
      this.tlsTemplateOptions = []
      this.applyingCertificateRecord = true
      try {
        if (id > 0) {
          const newData = this.normalizeTlsData(<tls>JSON.parse(this.$props.data))
          this.tls = newData
          this.selectedCertificateRecordId = this.tls.certificateRecordId && this.tls.certificateRecordId > 0 ? this.tls.certificateRecordId : null
          const hasCertificateCenterBinding = (this.tls.certificateRecordId ?? 0) > 0
          if (hasCertificateCenterBinding) {
            void this.loadCertificateOptions()
          }
          this.tlsType = this.resolveMihomoTlsType(this.tls)
          if (this.tlsType > 0) this.ensureMihomoTlsModeDefaults(this.tlsType)
          if (this.isMihomoNamespace) this.clearInactiveMihomoTlsModes()
          this.usePath = hasCertificateCenterBinding ? 2 : (this.tls.server?.key == undefined ? 0 : 1)
          this.showSHA256 =
            this.verifyPublicKey && (
              (this.tls.client?.certificate_public_key_sha256?.length ?? 0) > 0 ||
              (!this.isMihomoNamespace && (this.tls.server?.client_certificate_public_key_sha256?.length ?? 0) > 0)
            )
          this.showFingerprint = this.verifyClashPublicKey && (this.tls.client?.fingerprint?.trim().length ?? 0) > 0
          this.clientCaUsePath = this.tls.server?.client_certificate != undefined ? 1 : 0
          this.clientCertUsePath = this.tls.client?.client_certificate != undefined ? 1 : 0
          if (this.tlsType == 0 && this.usePath == 1) {
            await this.ensureTLSTemplateOptions(true)
          }
          this.title = "edit"
        }
        else {
          const freshTls = this.normalizeTlsData(<tls>{ id: 0, name: '', server: { enabled: true }, client: {} })
          // SNI/ALPN are enabled for a newly created TLS draft. The save path
          // removes the empty SNI or an empty ALPN selection before persisting.
          freshTls.server.server_name = ''
          freshTls.server.alpn = cloneTlsDefault(defaultInTls.alpn)
          this.tls = freshTls
          this.selectedCertificateRecordId = null
          this.clearTLSTemplateSelection()
          this.tlsType = 0
          this.usePath = 0
          this.showSHA256 = false
          this.showFingerprint = false
          this.clientCaUsePath = 0
          this.clientCertUsePath = 0
          this.title = "add"
        }
      } finally {
        this.applyingCertificateRecord = false
      }
      await this.reloadMihomoShadowTlsRules()
    },
    changeTlsType(value: number) {
      const nextType = Number(value)
      if (!Number.isFinite(nextType) || nextType === this.tlsType) {
        return
      }
      this.tlsType = nextType
      this.tls.mode = this.modeNameForType(nextType) as any
      this.clearPendingCertRefreshes()
      this.menu = false
      this.clearCertificateBinding()
      this.clearTLSTemplateSelection()
      if (this.tlsType === 0) {
        this.tls.server = {
          enabled: true,
          server_name: '',
          alpn: cloneTlsDefault(defaultInTls.alpn),
        }
        this.tls.client = {}
      } else {
        this.ensureMihomoTlsModeDefaults(this.tlsType)
        this.refreshMihomoTlsCredentials()
      }
      if (this.isMihomoNamespace) this.clearInactiveMihomoTlsModes()
      this.showFingerprint = false
      this.tls = this.normalizeTlsData(this.tls)
      if (this.tlsType === 0) {
        // normalizeTlsData removes empty optional values for persistence; keep
        // the new TLS draft's SNI control visibly enabled until it is saved.
        this.tls.server.server_name = ''
        this.tls.server.alpn = cloneTlsDefault(defaultInTls.alpn)
      }
    },
    async confirmHighCostCertificateGeneration(): Promise<boolean> {
      if (this.certKeyAlg !== 'rsa8192' && this.certSigAlg !== 'rsa8192') {
        return true
      }
      return confirm({
        title: '高开销证书生成',
        message: 'RSA8192 会生成完整证书链，计算时间和 CPU 占用会明显增加。',
        severity: 'warning',
        confirmText: '继续生成',
        cancelText: '取消',
      })
    },
    closeModal() {
      if (this.loading || this.$props.saving || this.applyingCertificate) return
      this.clearPendingCertRefreshes()
      this.updateData(0) // reset
      this.$emit('close')
    },
    async saveChanges() {
      if (this.loading || this.$props.saving || this.applyingCertificate) return
      this.loading = true
      try {
        if (this.tlsType >= 2 && !this.isMihomoNamespace) {
          return
        }
        if (!this.prepareMihomoShadowTlsRules()) {
          return
        }
        this.syncMihomoTlsCredentials()
        if (!this.validateMihomoTlsMode()) {
          return
        }
        if (this.tlsType == 0 && this.usePath == 2) {
          const id = Number(this.selectedCertificateRecordId ?? this.tls.certificateRecordId ?? 0)
          if (!Number.isFinite(id) || id <= 0) {
            push.warning({
              duration: 4000,
              message: '请选择证书管理中心里的证书',
            })
            return
          }
          this.tls.certificateRecordId = id
          if (!this.hasServerCertificateMaterial()) {
            const applied = await this.onCertificateRecordSelected(id)
            if (!applied || !this.hasServerCertificateMaterial()) {
              return
            }
          }
        } else if (this.tlsType == 0) {
          this.clearCertificateBinding()
        }

        this.tls.mode = this.modeNameForType(this.tlsType) as any
        this.clearInactiveMihomoTlsModes()
        const payload = this.normalizeTlsData(this.tls)
        this.$emit('save', payload)
      } finally {
        this.loading = false
      }
    },
    clearServerCertDerivedValues() {
      this.serverCertRefreshSeq++
      this.serverCertRefreshController?.abort()
      this.serverCertRefreshController = undefined
      this.serverSha256Seq++
      this.serverSha256Controller?.abort()
      this.serverSha256Controller = undefined
      this.serverFingerprintSeq++
      this.serverFingerprintController?.abort()
      this.serverFingerprintController = undefined
      this.serverSha256Loading = false
      this.serverFingerprintLoading = false
      this.outTls.certificate_public_key_sha256 = undefined
      if (this.showFingerprint) {
        this.outTls.fingerprint = undefined
      }
    },
    clearClientCertDerivedValues() {
      this.clientCertRefreshSeq++
      this.clientCertRefreshController?.abort()
      this.clientCertRefreshController = undefined
      this.clientSha256Seq++
      this.clientSha256Controller?.abort()
      this.clientSha256Controller = undefined
      this.clientSha256Loading = false
      this.inTls.client_certificate_public_key_sha256 = undefined
    },
    clearSha256DerivedValues() {
      this.serverSha256Seq++
      this.clientSha256Seq++
      this.serverSha256Controller?.abort()
      this.serverSha256Controller = undefined
      this.clientSha256Controller?.abort()
      this.clientSha256Controller = undefined
      this.serverSha256Loading = false
      this.clientSha256Loading = false
      this.outTls.certificate_public_key_sha256 = undefined
      this.inTls.client_certificate_public_key_sha256 = undefined
    },
    normalizeTLSTemplateCode(value: any): string {
      if (typeof value !== 'string') {
        return ''
      }
      return value.trim().toLowerCase()
    },
    isKnownTLSTemplateCode(value: any): boolean {
      const normalized = this.normalizeTLSTemplateCode(value)
      return normalized.length > 0 && this.tlsTemplateOptions.some(item => item.value === normalized)
    },
    clearTLSTemplateSelection() {
      this.selectedTLSTemplateCode = ''
    },
    async genSelfSigned(){
      await this.ensureTLSTemplateOptions(true)
      let templateCode = this.normalizeTLSTemplateCode(this.selectedTLSTemplateCode)
      if (!templateCode) {
        templateCode = 'zerossl'
        this.selectedTLSTemplateCode = templateCode
      }
      if (templateCode && this.tlsTemplateOptions.length > 0 && !this.isKnownTLSTemplateCode(templateCode)) {
        push.warning({
          duration: 4000,
          message: 'TLS template is unavailable, please reload and try again',
        })
        return
      }
      if (!(await this.confirmHighCostCertificateGeneration())) {
        return
      }
      this.clearCertificateBinding()
      this.loading = true
      try {
        const serverName = this.inTls.server_name ?? "''"
        const options = serverName + "," + this.certDuration + "," + this.certDurationUnit + "," + this.certKeyAlg + "," + this.certSigAlg
        const query: Record<string, string> = { k: "tls", o: options }
        if (templateCode) {
          query.template = templateCode
        }
        const msg = await HttpUtils.get('api/keypairs', query)
        if (!msg.success || !Array.isArray(msg.obj) || msg.obj.length === 0) {
          push.error({
            message: msg.msg || i18n.global.t('error')
          })
          return
        }

        this.inTls.key_path=undefined
        this.inTls.certificate_path=undefined
        this.usePath = 1
        let privateKey = <string[]>[]
        let publicKey = <string[]>[]
        let isPrivateKey = false
        let isPublicKey = false

        msg.obj.forEach((line:string) => {
            if (line === "-----BEGIN PRIVATE KEY-----") {
                isPrivateKey = true
                isPublicKey = false
                privateKey.push(line)
            } else if (line === "-----END PRIVATE KEY-----") {
                isPrivateKey = false
                privateKey.push(line)
            } else if (line === "-----BEGIN CERTIFICATE-----") {
                isPublicKey = true
                isPrivateKey = false
                publicKey.push(line)
            } else if (line === "-----END CERTIFICATE-----") {
                isPublicKey = false
                publicKey.push(line)
            } else if (isPrivateKey) {
                privateKey.push(line)
            } else if (isPublicKey) {
                publicKey.push(line)
            }
        })
        this.clearServerCertDerivedValues()
        this.inTls.key = privateKey?? undefined
        this.inTls.certificate = publicKey?? undefined
        await this.refreshServerCertDerivedFields()
      } finally {
        this.loading = false
      }
    },
    async genRealityKey(){
      this.loading = true
      try {
        const msg = await HttpUtils.get('api/keypairs', { k: "reality" })
        if (msg.success && Array.isArray(msg.obj)) {
          msg.obj.forEach((line:string) => {
            if (this.inTls.reality && this.outTls.reality){
              if (line.startsWith("PrivateKey")){
                this.inTls.reality.private_key = line.substring(12)
              }
              if (line.startsWith("PublicKey")){
                this.outTls.reality.public_key = line.substring(11)
              }
            }
          })
        } else {
          push.error({
            message: i18n.global.t('error') + ": " + msg.obj
          })
        }
      } finally {
        this.loading = false
      }
    },
    async genClientCert(){
      if (!(await this.confirmHighCostCertificateGeneration())) {
        return
      }
      this.loading = true
      try {
        const serverName = this.inTls.server_name ?? "client"
        const options = serverName + "," + this.certDuration + "," + this.certDurationUnit + "," + this.certKeyAlg + "," + this.certSigAlg + ",client"
        const msg = await HttpUtils.get('api/keypairs', { k: "tls", o: options })
        if (!msg.success || !Array.isArray(msg.obj) || msg.obj.length === 0) {
          push.error({
            message: i18n.global.t('error') + ": " + msg.obj
          })
          return
        }

        this.clientCertUsePath = 1
        let privateKey = <string[]>[]
        let publicKey = <string[]>[]
        let isPrivateKey = false
        let isPublicKey = false

        msg.obj.forEach((line:string) => {
            if (line === "-----BEGIN PRIVATE KEY-----") {
                isPrivateKey = true
                isPublicKey = false
                privateKey.push(line)
            } else if (line === "-----END PRIVATE KEY-----") {
                isPrivateKey = false
                privateKey.push(line)
            } else if (line === "-----BEGIN CERTIFICATE-----") {
                isPublicKey = true
                isPrivateKey = false
                publicKey.push(line)
            } else if (line === "-----END CERTIFICATE-----") {
                isPublicKey = false
                publicKey.push(line)
            } else if (isPrivateKey) {
                privateKey.push(line)
            } else if (isPublicKey) {
                publicKey.push(line)
            }
        })
        this.clearClientCertDerivedValues()
        // 客户端证书和私钥 -> tls.client
        this.tls.client.client_certificate = publicKey.length > 0 ? publicKey : undefined
        this.tls.client.client_certificate_path = undefined
        this.tls.client.client_key = privateKey.length > 0 ? privateKey : undefined
        this.tls.client.client_key_path = undefined
        // 同时将客户端证书（CA）放到服务端 inTls.client_certificate，用于验证客户端
        this.inTls.client_certificate = publicKey.length > 0 ? [...publicKey] : undefined
        this.inTls.client_certificate_path = undefined
        this.clientCaUsePath = 1
      } finally {
        this.loading = false
      }
    },
    splitToLines(v: string): string[] | undefined {
      const lines = v
        .split('\n')
        .map(item => item.trim())
        .filter(item => item.length > 0)
      return lines.length > 0 ? lines : undefined
    },
    splitSha256List(v: string): string[] | undefined {
      const lines = v
        .split(/[\n,]/)
        .map(item => item.trim())
        .filter(item => item.length > 0)
      return lines.length > 0 ? lines : undefined
    },
    buildSha256Payload(usePath: number, certPath?: string, certLines?: string[]) {
      if (usePath === 0) {
        const path = certPath?.trim()
        if (!path) {
          return null
        }
        return {
          source_type: "path",
          certificate_path: path,
        }
      }

      const pem = certLines && certLines.length > 0 ? certLines.join('\n').trim() : ''
      if (!pem) {
        return null
      }
      return {
        source_type: "pem",
        certificate_pem: pem,
      }
    },
    buildServerCertPayload() {
      if (this.usePath === 2) {
        const pathPayload = this.buildSha256Payload(0, this.inTls.certificate_path, this.inTls.certificate)
        if (pathPayload) {
          return pathPayload
        }
        return this.buildSha256Payload(1, this.inTls.certificate_path, this.inTls.certificate)
      }
      return this.buildSha256Payload(this.usePath, this.inTls.certificate_path, this.inTls.certificate)
    },
    clearPendingCertRefreshes() {
      this.certificateMaterialSeq++
      this.certificateMaterialController?.abort()
      this.certificateMaterialController = undefined
      this.applyingCertificate = false
      this.applyingCertificateRecord = false
      this.serverCertRefreshSeq++
      this.clientCertRefreshSeq++
      this.serverSha256Seq++
      this.clientSha256Seq++
      this.serverFingerprintSeq++
      if (this.serverCertRefreshTimer !== undefined) {
        window.clearTimeout(this.serverCertRefreshTimer)
        this.serverCertRefreshTimer = undefined
      }
      if (this.clientCertRefreshTimer !== undefined) {
        window.clearTimeout(this.clientCertRefreshTimer)
        this.clientCertRefreshTimer = undefined
      }
      this.serverCertRefreshController?.abort()
      this.serverCertRefreshController = undefined
      this.clientCertRefreshController?.abort()
      this.clientCertRefreshController = undefined
      this.serverSha256Controller?.abort()
      this.serverSha256Controller = undefined
      this.clientSha256Controller?.abort()
      this.clientSha256Controller = undefined
      this.serverFingerprintController?.abort()
      this.serverFingerprintController = undefined
      this.serverSha256Loading = false
      this.clientSha256Loading = false
      this.serverFingerprintLoading = false
    },
    scheduleServerCertRefresh() {
      if (!this.$props.visible || this.tlsType !== 0) {
        return
      }
      if (this.serverCertRefreshTimer !== undefined) {
        window.clearTimeout(this.serverCertRefreshTimer)
      }
      this.serverCertRefreshTimer = window.setTimeout(() => {
        this.serverCertRefreshTimer = undefined
        void this.refreshServerCertDerivedFields()
      }, 450)
    },
    scheduleClientCertRefresh() {
      if (!this.$props.visible || this.tlsType !== 0 || this.isMihomoNamespace || !this.verifyPublicKey || !this.showSHA256) {
        return
      }
      if (this.clientCertRefreshTimer !== undefined) {
        window.clearTimeout(this.clientCertRefreshTimer)
      }
      this.clientCertRefreshTimer = window.setTimeout(() => {
        this.clientCertRefreshTimer = undefined
        void this.refreshClientCertDerivedFields()
      }, 450)
    },
    applyAlgorithmInfo(info: any) {
      if (!info) {
        return
      }

      const signatureAlg = this.normalizeCertAlgorithm(info.signature_algorithm)
      const keyAlg = this.normalizeCertAlgorithm(info.key_algorithm)
      if (keyAlg) {
        this.certKeyAlg = keyAlg
      }
      if (signatureAlg) {
        this.certSigAlg = signatureAlg
      } else if (keyAlg) {
        this.certSigAlg = keyAlg
      }
    },
    extractSha256Result(msg: any): string | undefined {
      if (!msg?.success) {
        return undefined
      }
      const sha = typeof msg.obj === 'string' ? msg.obj.trim() : ''
      return sha.length > 0 ? sha : undefined
    },
    normalizeCertAlgorithm(v: any): string | undefined {
      if (typeof v !== 'string') {
        return undefined
      }
      const normalized = v.toLowerCase().trim()
      return ['ecc224', 'ecc256', 'ecc384', 'ecc521', 'rsa1024', 'rsa2048', 'rsa3072', 'rsa4096', 'rsa8192'].includes(normalized)
        ? normalized
        : undefined
    },
    applyTLSTemplateDetectionResult(info: any) {
      const templateCode = this.normalizeTLSTemplateCode(info?.template_code)
      this.selectedTLSTemplateCode = templateCode
    },
    async refreshServerCertDerivedFields() {
      if (!this.showTLSTemplateSelect) {
        this.clearTLSTemplateSelection()
      }
      const payload = this.buildServerCertPayload()
      if (!payload) {
        if (this.showTLSTemplateSelect) {
          this.clearTLSTemplateSelection()
        }
        return
      }

      this.serverCertRefreshController?.abort()
      const controller = new AbortController()
      this.serverCertRefreshController = controller
      const requestId = ++this.serverCertRefreshSeq
      const requestOptions = { signal: controller.signal, silentErrorToast: true }
      let msg
      try {
        msg = await HttpUtils.post('api/tlsCertificateInfo', payload, requestOptions)
      } catch {
        return
      } finally {
        if (this.serverCertRefreshController === controller) {
          this.serverCertRefreshController = undefined
        }
      }
      if (requestId !== this.serverCertRefreshSeq) {
        return
      }
      if (!msg.success || !msg.obj) {
        return
      }

      this.applyAlgorithmInfo(msg.obj)

      if (this.showSHA256) {
        const sha = typeof msg.obj.public_key_sha256 === 'string' ? msg.obj.public_key_sha256.trim() : ''
        if (sha) {
          this.outTls.certificate_public_key_sha256 = [sha]
          this.outTls.certificate = undefined
          this.outTls.certificate_path = undefined
        }
      }

      if (this.showFingerprint && this.verifyClashPublicKey) {
        const fingerprint = typeof msg.obj.fingerprint === 'string' ? msg.obj.fingerprint.trim().toUpperCase() : ''
        if (fingerprint) {
          this.outTls.fingerprint = fingerprint
        }
      }

      if (this.showTLSTemplateSelect) {
        this.applyTLSTemplateDetectionResult(msg.obj)
      }
    },
    async refreshClientCertDerivedFields() {
      if (!this.showSHA256) {
        return
      }

      const payload = this.buildSha256Payload(
        this.clientCertUsePath,
        this.tls.client.client_certificate_path,
        this.tls.client.client_certificate
      )
      if (!payload) {
        return
      }

      this.clientCertRefreshController?.abort()
      const controller = new AbortController()
      this.clientCertRefreshController = controller
      const requestId = ++this.clientCertRefreshSeq
      let msg
      try {
        msg = await HttpUtils.post('api/tlsSha256', payload, { signal: controller.signal, silentErrorToast: true })
      } catch {
        return
      } finally {
        if (this.clientCertRefreshController === controller) {
          this.clientCertRefreshController = undefined
        }
      }
      if (requestId !== this.clientCertRefreshSeq) {
        return
      }

      const sha = this.extractSha256Result(msg)
      if (sha) {
        this.inTls.client_certificate_public_key_sha256 = [sha]
        this.inTls.client_certificate = undefined
        this.inTls.client_certificate_path = undefined
      }
    },
    async generateServerCertSha256() {
      const payload = this.buildServerCertPayload()
      if (!payload) {
        push.error({
          message: i18n.global.t('tls.sha256MissingServerCertSource')
        })
        return
      }

      this.serverSha256Loading = true
      this.serverSha256Controller?.abort()
      const controller = new AbortController()
      this.serverSha256Controller = controller
      const requestId = ++this.serverSha256Seq
      let msg
      try {
        msg = await HttpUtils.post('api/tlsSha256', payload, { signal: controller.signal })
      } catch {
        return
      } finally {
        if (this.serverSha256Controller === controller) {
          this.serverSha256Controller = undefined
        }
        if (requestId === this.serverSha256Seq) {
          this.serverSha256Loading = false
        }
      }
      if (requestId !== this.serverSha256Seq || !msg.success || !this.showSHA256 || !this.verifyPublicKey || this.tlsType !== 0) {
        return
      }

      const sha = typeof msg.obj === 'string' ? msg.obj.trim() : ''
      if (!sha) {
        push.error({
          message: i18n.global.t('tls.sha256InvalidResult')
        })
        return
      }

      this.outTls.certificate_public_key_sha256 = [sha]
      this.outTls.certificate = undefined
      this.outTls.certificate_path = undefined
    },
    async generateClientCertSha256() {
      const payload = this.buildSha256Payload(
        this.clientCertUsePath,
        this.tls.client.client_certificate_path,
        this.tls.client.client_certificate
      )
      if (!payload) {
        push.error({
          message: i18n.global.t('tls.sha256MissingClientCertSource')
        })
        return
      }

      this.clientSha256Loading = true
      this.clientSha256Controller?.abort()
      const controller = new AbortController()
      this.clientSha256Controller = controller
      const requestId = ++this.clientSha256Seq
      let msg
      try {
        msg = await HttpUtils.post('api/tlsSha256', payload, { signal: controller.signal })
      } catch {
        return
      } finally {
        if (this.clientSha256Controller === controller) {
          this.clientSha256Controller = undefined
        }
        if (requestId === this.clientSha256Seq) {
          this.clientSha256Loading = false
        }
      }
      if (requestId !== this.clientSha256Seq || !msg.success || !this.showSHA256 || !this.verifyPublicKey || this.tlsType !== 0) {
        return
      }

      const sha = typeof msg.obj === 'string' ? msg.obj.trim() : ''
      if (!sha) {
        push.error({
          message: i18n.global.t('tls.sha256InvalidResult')
        })
        return
      }

      this.inTls.client_certificate_public_key_sha256 = [sha]
      this.inTls.client_certificate = undefined
      this.inTls.client_certificate_path = undefined
    },
    tryGenerateServerFingerprintSilently() {
      const payload = this.buildServerCertPayload()
      if (!payload) {
        return
      }
      void this.generateServerFingerprint(payload, true)
    },
    async generateServerFingerprint(
      prebuiltPayload?: { source_type: string, certificate_path?: string, certificate_pem?: string } | null,
      silent = false
    ) {
      if (!this.verifyClashPublicKey) {
        this.showFingerprint = false
        this.outTls.fingerprint = undefined
        return
      }
      const payload = prebuiltPayload ?? this.buildServerCertPayload()
      if (!payload) {
        if (!silent) {
          push.error({
            message: i18n.global.t('tls.sha256MissingServerCertSource')
          })
        }
        return
      }

      this.serverFingerprintLoading = true
      this.serverFingerprintController?.abort()
      const controller = new AbortController()
      this.serverFingerprintController = controller
      const requestId = ++this.serverFingerprintSeq
      let msg
      try {
        msg = await HttpUtils.post('api/tlsFingerprint', payload, { signal: controller.signal })
      } catch {
        return
      } finally {
        if (this.serverFingerprintController === controller) {
          this.serverFingerprintController = undefined
        }
        if (requestId === this.serverFingerprintSeq) {
          this.serverFingerprintLoading = false
        }
      }
      if (requestId !== this.serverFingerprintSeq || !msg.success || !this.showFingerprint || !this.verifyClashPublicKey || this.tlsType !== 0) {
        return
      }

      const fingerprint = typeof msg.obj === 'string' ? msg.obj.trim().toUpperCase() : ''
      if (!fingerprint) {
        push.error({
          message: i18n.global.t('tls.sha256InvalidResult')
        })
        return
      }

      this.outTls.fingerprint = fingerprint
      this.showFingerprint = true
      this.tls.client.insecure = undefined
    },
    randomSID(){
      this.short_id = RandomUtil.randomShortId().join(',')
    }
  },
  computed: {
    isMihomoNamespace(): boolean {
      return this.$props.namespace === 'mihomo'
    },
    showTLSTemplateSelect(): boolean {
      return this.tlsType === 0 && this.usePath === 1
    },
    inTls(): iTls {
      return this.tls.server
    },
    outTls(): oTls {
      return this.tls.client
    },
    certText: {
      get(): string { return this.inTls.certificate ? this.inTls.certificate.join('\n') : '' },
      set(v:string) {
        this.markCertificateBindingManualChange()
        this.clearServerCertDerivedValues()
        this.inTls.certificate = this.splitToLines(v)
      }
    },
    keyText: {
      get(): string { return this.inTls.key ? this.inTls.key.join('\n') : '' },
      set(v:string) {
        this.markCertificateBindingManualChange()
        this.inTls.key = this.splitToLines(v)
      }
    },
    disableSni: {
      get() { return this.outTls.disable_sni ?? false },
      set(v: boolean) { this.tls.client.disable_sni = v ? true : undefined }
    },
    insecure: {
      get() { return this.outTls.insecure ?? false },
      set(v: boolean) { this.tls.client.insecure = v ? true : undefined }
    },
    verifyPublicKey: {
      get(): boolean {
        return this.outTls.include_server_certificate !== false
      },
      set(v: boolean) {
        this.tls.client.include_server_certificate = v ? undefined : false
        if (!v) {
          this.serverSha256Controller?.abort()
          this.clientSha256Controller?.abort()
          this.serverSha256Controller = undefined
          this.clientSha256Controller = undefined
          this.showSHA256 = false
          this.clearSha256DerivedValues()
        }
      }
    },
    verifyClashPublicKey: {
      get(): boolean {
        return this.outTls.include_server_fingerprint !== false
      },
      set(v: boolean) {
        this.tls.client.include_server_fingerprint = v ? undefined : false
        if (!v) {
          this.serverFingerprintController?.abort()
          this.serverFingerprintController = undefined
          this.showFingerprint = false
          this.outTls.fingerprint = undefined
        }
      }
    },
    server_port: {
      get() { return this.normalizePort(this.inTls.reality?.handshake?.server_port) },
      set(v: any) {
        if (this.inTls.reality){
          this.inTls.reality.handshake.server_port = this.normalizePort(v)
        }
      }
    },
    short_id: {
      get() { return this.inTls.reality?.short_id ? this.inTls.reality.short_id.join(',') : undefined },
      set(v: string) {
        if (this.inTls.reality){
          this.inTls.reality.short_id = v.length > 0 ? v.split(',') : []
        }
      }
    },
    max_time: {
      get(): number {
        const raw = String(this.inTls?.reality?.max_time_difference ?? '').trim().replace(/m$/i, '')
        const value = Number(raw)
        return Number.isSafeInteger(value) && value > 0 ? value : 1
      },
      set(v: number) {
        if (this.inTls.reality){
          this.inTls.reality.max_time_difference = Number.isSafeInteger(v) && v > 0 ? `${v}m` : '1m'
        }
      }
    },
    optionSNI: {
      get(): boolean { return typeof this.inTls.server_name === 'string' },
      set(v:boolean) { this.inTls.server_name = v ? '' : undefined }
    },
    optionALPN: {
      get(): boolean { return Array.isArray(this.inTls.alpn) },
      set(v:boolean) {
        this.inTls.alpn = v
          ? (Array.isArray(this.inTls.alpn) ? this.inTls.alpn : cloneTlsDefault(defaultInTls.alpn))
          : undefined
      }
    },
    optionMinV: {
      get(): boolean { return this.inTls.min_version != undefined },
      set(v:boolean) { this.inTls.min_version = v ? defaultInTls.min_version : undefined }
    },
    optionMaxV: {
      get(): boolean { return this.inTls.max_version != undefined },
      set(v:boolean) { this.inTls.max_version = v ? defaultInTls.max_version : undefined }
    },
    optionCS: {
      get(): boolean { return Array.isArray(this.inTls.cipher_suites) },
      set(v:boolean) {
        this.inTls.cipher_suites = v
          ? (Array.isArray(this.inTls.cipher_suites) ? this.inTls.cipher_suites : cloneTlsDefault(defaultInTls.cipher_suites))
          : undefined
      }
    },
    optionTlsStore: {
      get(): boolean { return this.outTls.tls_store != undefined },
      set(v:boolean) { this.tls.client.tls_store = v ? 'chrome' : undefined }
    },
    optionFP: {
      get(): boolean { return this.outTls.utls != undefined },
      set(v:boolean) { this.outTls.utls = v ? cloneTlsDefault(defaultOutTls.utls) : undefined }
    },
    optionSHA256: {
      get(): boolean { return this.showSHA256 },
      set(v:boolean) {
        if (!this.verifyPublicKey && v) {
          return
        }
        this.showSHA256 = v
        if (!v) {
          this.serverSha256Controller?.abort()
          this.clientSha256Controller?.abort()
          this.serverSha256Controller = undefined
          this.clientSha256Controller = undefined
          this.clearSha256DerivedValues()
          return
        }
        void this.refreshServerCertDerivedFields()
        if (!this.isMihomoNamespace) {
          void this.refreshClientCertDerivedFields()
        }
      }
    },
    optionFingerprint: {
      get(): boolean { return this.showFingerprint },
      set(v: boolean) {
        if (!this.verifyClashPublicKey && v) {
          return
        }
        this.showFingerprint = v
        if (!v) {
          this.serverFingerprintController?.abort()
          this.serverFingerprintController = undefined
          this.outTls.fingerprint = undefined
          return
        }
        this.tls.client.insecure = undefined
        if (!this.outTls.fingerprint) {
          this.tryGenerateServerFingerprintSilently()
        }
      }
    },
    optionEch: {
      get(): boolean { return this.outTls.ech != undefined },
      set(v:boolean) { this.outTls.ech = v ? cloneTlsDefault(defaultOutTls.ech) : undefined }
    },
    optionTime: {
      get(): boolean { return this.inTls?.reality?.max_time_difference != undefined },
      set(v:boolean) { if (this.inTls.reality) this.inTls.reality.max_time_difference = v ? "1m" : undefined }
    },
    clientAuthentication: {
      get(): string { return this.inTls.client_authentication ?? 'no' },
      set(v: string) {
        this.inTls.client_authentication = v === 'no' ? undefined : v
        if (v === 'no') {
          // 关闭 mTLS: 清除所有客户端证书相关字段
          this.inTls.client_certificate = undefined
          this.inTls.client_certificate_path = undefined
          this.inTls.client_certificate_public_key_sha256 = undefined
          this.tls.client.client_certificate = undefined
          this.tls.client.client_certificate_path = undefined
          this.tls.client.client_key = undefined
          this.tls.client.client_key_path = undefined
        }
        // 非 No 模式：不自动复制服务器证书，用户需要通过生成按钮或手动输入来设置客户端证书
      }
    },
    clientCertPath: {
      get(): string { return this.tls.client.client_certificate_path ?? '' },
      set(v: string) {
        const val = v.length > 0 ? v : undefined
        this.clearClientCertDerivedValues()
        this.tls.client.client_certificate_path = val
        // 同步到服务端 client_certificate_path（CA证书路径）
        this.inTls.client_certificate_path = val
      }
    },
    clientCertText: {
      get(): string { return this.outTls.client_certificate ? this.outTls.client_certificate.join('\n') : '' },
      set(v: string) {
        const arr = this.splitToLines(v)
        this.clearClientCertDerivedValues()
        this.tls.client.client_certificate = arr
        // 同步到服务端 client_certificate（CA证书，用于验证客户端）
        this.inTls.client_certificate = arr ? [...arr] : undefined
      }
    },
    clientKeyText: {
      get(): string { return this.outTls.client_key ? this.outTls.client_key.join('\n') : '' },
      set(v: string) { this.tls.client.client_key = this.splitToLines(v) }
    },
    serverCertSha256Text: {
      get(): string { return this.outTls.certificate_public_key_sha256 ? this.outTls.certificate_public_key_sha256.join('\n') : '' },
      set(v: string) {
        const arr = this.splitSha256List(v)
        this.outTls.certificate_public_key_sha256 = arr
        if (arr) {
          this.outTls.certificate = undefined
          this.outTls.certificate_path = undefined
        }
      }
    },
    clientCertSha256Text: {
      get(): string { return this.inTls.client_certificate_public_key_sha256 ? this.inTls.client_certificate_public_key_sha256.join('\n') : '' },
      set(v: string) {
        const arr = this.splitSha256List(v)
        this.inTls.client_certificate_public_key_sha256 = arr
        if (arr) {
          this.inTls.client_certificate = undefined
          this.inTls.client_certificate_path = undefined
        }
      }
    },
    serverFingerprintText: {
      get(): string { return this.outTls.fingerprint ?? '' },
      set(v: string) {
        const normalized = v.trim().toUpperCase()
        this.outTls.fingerprint = normalized.length > 0 ? normalized : undefined
      }
    }
  },
  watch: {
    async visible(v) {
      if (v) {
        await this.updateData(this.$props.id)
        this.scheduleServerCertRefresh()
        this.scheduleClientCertRefresh()
      } else {
        this.clearPendingCertRefreshes()
      }
    },
    usePath(value: number) {
      if (this.applyingCertificateRecord) {
        return
      }

      if (value == 2) {
        this.clearTLSTemplateSelection()
        this.clearCertificateBinding()
        this.clearServerCertificateMaterial()
        this.clearServerCertDerivedValues()
        return
      }

      this.clearCertificateBinding()
      if (value !== 1) {
        this.clearTLSTemplateSelection()
      } else {
        void this.ensureTLSTemplateOptions(true)
      }
      if (value == 0) {
        this.inTls.key = undefined
        this.inTls.certificate = undefined
      } else {
        this.inTls.key_path = undefined
        this.inTls.certificate_path = undefined
      }
      this.clearServerCertDerivedValues()
      this.scheduleServerCertRefresh()
    },
    certText() {
      this.markCertificateBindingManualChange()
      this.scheduleServerCertRefresh()
    },
    keyText() {
      this.markCertificateBindingManualChange()
    },
    'inTls.certificate_path'() {
      this.markCertificateBindingManualChange()
      this.clearServerCertDerivedValues()
      this.scheduleServerCertRefresh()
    },
    'inTls.key_path'() {
      this.markCertificateBindingManualChange()
    },
    clientCertUsePath() {
      this.clearClientCertDerivedValues()
      this.scheduleClientCertRefresh()
    },
    clientCertPath() {
      this.scheduleClientCertRefresh()
    },
    clientCertText() {
      this.scheduleClientCertRefresh()
    },
  },
  beforeUnmount() {
    this.clearPendingCertRefreshes()
  },
  components: { AcmeVue, EchVue, MihomoShadowTlsVue, MihomoRestlsVue, MihomoJlsVue }
}
</script>

<style scoped>
.tls-certificate-select {
  max-width: 100%;
}

.mihomo-tls-mode-toggle {
  max-width: 100%;
  overflow-x: auto;
  white-space: nowrap;
}
</style>
