<template>
  <v-dialog transition="dialog-bottom-transition" width="800">
    <v-card class="rounded-lg">
      <v-card-title>
        {{ $t('actions.' + title) + " " + $t('objects.tls') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 0 16px; overflow-y: scroll;">
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
              <v-btn-toggle v-model="tlsType"
              class="rounded-xl"
              density="compact"
              variant="outlined"
              @update:model-value="changeTlsType"
              shaped
              mandatory>
                <v-btn>TLS</v-btn>
                <v-btn>Reality</v-btn>
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
              <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && inTls.min_version">
                <v-select
                  hide-details
                  :label="$t('tls.minVer')"
                  :items="tlsVersions"
                  v-model="inTls.min_version">
                </v-select>
              </v-col>
              <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && inTls.max_version">
                <v-select
                  hide-details
                  :label="$t('tls.maxVer')"
                  :items="tlsVersions"
                  v-model="inTls.max_version">
                </v-select>
              </v-col>
              <v-col cols="12" sm="6" md="4" v-if="inTls.alpn">
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
                  :loading="loading">
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
                      @click="tls.client.client_certificate=undefined; tls.client.client_key=undefined; inTls.client_certificate=undefined"
                    >{{ $t('tls.usePath') }}</v-btn>
                    <v-btn
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
                        :disabled="!verifyClashPublicKey"
                        :loading="serverFingerprintLoading"
                        @click="generateServerFingerprint">
                        {{ $t('actions.generate') }}
                      </v-btn>
                    </v-col>
                  </v-row>
                </v-col>
              </v-row>
            </template>
          </template>
          <template v-if="outTls.reality && inTls.reality">
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
                min="0"
                hide-details
                v-model="server_port">
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
                label="Max Time Diference"
                type="number"
                min="1"
                :suffix="$t('date.m')"
                hide-details
                v-model="max_time">
                </v-text-field>
              </v-col>
            </v-row>
          </template>
          <v-row v-if="outTls.utls != undefined">
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
            <v-menu v-model="menu" :close-on-content-click="false" location="start">
              <template v-slot:activator="{ props }">
                <v-btn v-bind="props" hide-details variant="tonal">{{ $t('tls.options') }}</v-btn>
              </template>
              <v-card>
                <v-list>
                  <template v-if="tlsType == 0">
                    <v-list-item v-if="!isMihomoNamespace">
                      <v-switch v-model="optionTlsStore" color="primary" label="TLS_store" hide-details></v-switch>
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
                  <template v-else>
                    <v-list-item>
                      <v-switch v-model="optionTime" color="primary" label="Max Time Difference" hide-details></v-switch>
                    </v-list-item>
                  </template>
                </v-list>
              </v-card>
            </v-menu>
          </v-card-actions>
        </v-card>
        <AcmeVue :tls="inTls" />
        <EchVue :iTls="inTls" :oTls="outTls" />
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
import { tls, iTls, defaultInTls, oTls, defaultOutTls, sanitizeTlsForNamespace } from '@/types/tls'
import AcmeVue from '@/components/tls/Acme.vue'
import EchVue from '@/components/tls/Ech.vue'
import HttpUtils from '@/plugins/httputil'
import { push } from 'notivue'
import { i18n } from '@/locales'
import RandomUtil from '@/plugins/randomUtil'

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

export default {
  props: {
    visible: { type: Boolean, default: false },
    data: { type: String, default: '{}' },
    id: { type: Number, default: 0 },
    namespace: { type: String, default: 'default' },
  },
  emits: ['close', 'save'],
  data() {
    return {
      tls: <tls>{ id: 0, name: '', server: <iTls>{ enabled: true }, client: <oTls>{} },
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
      serverCertRefreshSeq: 0,
      clientCertRefreshSeq: 0,
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
    normalizeTlsData(data?: tls | null) {
      const normalized = sanitizeTlsForNamespace(data, this.$props.namespace)
      if (normalized.server == null) normalized.server = { enabled: true }
      if (normalized.client == null) normalized.client = {}
      if (normalized.client.store != undefined && normalized.client.tls_store == undefined) {
        normalized.client.tls_store = normalized.client.store
        delete normalized.client.store
      }
      return normalized
    },
    normalizeCertificateOptions(raw: unknown): CertificateOption[] {
      const rows = Array.isArray(raw)
        ? raw
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
        const msg = await HttpUtils.get('api/certificate-list')
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

      this.applyingCertificate = true
      this.applyingCertificateRecord = true
      try {
        const msg = await HttpUtils.post('api/certificate-material', { id })
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
        this.applyingCertificateRecord = false
        this.applyingCertificate = false
      }
    },
    async updateData(id: number) {
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
          this.tlsType = this.tls.server?.reality == undefined ? 0 : 1
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
          if (this.tlsType == 0) {
            await this.refreshServerCertAlgorithms()
          }
          this.title = "edit"
        }
        else {
          this.tls = this.normalizeTlsData(<tls>{ id: 0, name: '', server: {enabled: true}, client: {} })
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
    },
    changeTlsType(){
      this.clearCertificateBinding()
      this.clearTLSTemplateSelection()
      if (this.tlsType) {
        this.tls.server = <iTls>{
          enabled: true,
          reality: { enabled: true, handshake: { server_port: 443 }, short_id: RandomUtil.randomShortId() },
          server_name: ""
        }
        this.tls.client = <oTls>{ reality: { public_key: "" }, utls: defaultOutTls.utls }
      } else {
        this.tls.server = <iTls>{ enabled: true }
        this.tls.client = <oTls>{}
      }
      this.showFingerprint = false
      this.tls = this.normalizeTlsData(this.tls)
    },
    closeModal() {
      this.clearPendingCertRefreshes()
      this.updateData(0) // reset
      this.$emit('close')
    },
    async saveChanges() {
      this.loading = true
      try {
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

        const payload = this.normalizeTlsData(this.tls)
        this.tls = payload
        this.$emit('save', payload)
      } finally {
        this.loading = false
      }
    },
    clearServerCertDerivedValues() {
      this.outTls.certificate_public_key_sha256 = undefined
      if (this.showFingerprint) {
        this.outTls.fingerprint = undefined
      }
    },
    clearClientCertDerivedValues() {
      this.inTls.client_certificate_public_key_sha256 = undefined
    },
    clearSha256DerivedValues() {
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
      this.clearCertificateBinding()
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
      this.loading = true
      const serverName = this.inTls.server_name ?? "''"
      const options = serverName + "," + this.certDuration + "," + this.certDurationUnit + "," + this.certKeyAlg + "," + this.certSigAlg
      const query: Record<string, string> = { k: "tls", o: options }
      if (templateCode) {
        query.template = templateCode
      }
      const msg = await HttpUtils.get('api/keypairs', query)
      this.loading = false
      if (msg.success) {
        this.inTls.key_path=undefined
        this.inTls.certificate_path=undefined
        this.usePath = 1
        if (msg.obj.length>0){
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

        } else {
          push.error({
            message: i18n.global.t('error') + ": " + msg.obj
          })
        }
      } else {
        push.error({
          message: msg.msg || i18n.global.t('error')
        })
      }
    },
    async genRealityKey(){
      this.loading = true
      const msg = await HttpUtils.get('api/keypairs', { k: "reality" })
      this.loading = false
      if (msg.success) {
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
    },
    async genClientCert(){
      this.loading = true
      const serverName = this.inTls.server_name ?? "client"
      const options = serverName + "," + this.certDuration + "," + this.certDurationUnit + "," + this.certKeyAlg + "," + this.certSigAlg + ",client"
      const msg = await HttpUtils.get('api/keypairs', { k: "tls", o: options })
      this.loading = false
      if (msg.success) {
        this.clientCertUsePath = 1
        if (msg.obj.length > 0){
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
        } else {
          push.error({
            message: i18n.global.t('error') + ": " + msg.obj
          })
        }
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
      if (this.serverCertRefreshTimer !== undefined) {
        window.clearTimeout(this.serverCertRefreshTimer)
        this.serverCertRefreshTimer = undefined
      }
      if (this.clientCertRefreshTimer !== undefined) {
        window.clearTimeout(this.clientCertRefreshTimer)
        this.clientCertRefreshTimer = undefined
      }
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
    extractFingerprintResult(msg: any): string | undefined {
      if (!msg?.success) {
        return undefined
      }
      const fingerprint = typeof msg.obj === 'string' ? msg.obj.trim().toUpperCase() : ''
      return fingerprint.length > 0 ? fingerprint : undefined
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
    async refreshServerCertAlgorithms() {
      // Only read algorithm strength from the upper server certificate section.
      const payload = this.buildServerCertPayload()
      if (!payload) {
        return
      }

      const msg = await HttpUtils.post('api/tlsCertAlgorithm', payload)
      if (!msg.success || !msg.obj) {
        return
      }
      this.applyAlgorithmInfo(msg.obj)
    },
    applyTLSTemplateDetectionResult(msg: any) {
      if (!msg?.success) {
        this.clearTLSTemplateSelection()
        return
      }
      const templateCode = this.normalizeTLSTemplateCode(msg.obj?.template_code)
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

      const requestId = ++this.serverCertRefreshSeq
      const [algorithmMsg, shaMsg, fingerprintMsg, templateMsg] = await Promise.all([
        HttpUtils.post('api/tlsCertAlgorithm', payload),
        this.showSHA256 ? HttpUtils.post('api/tlsSha256', payload) : Promise.resolve(undefined),
        (this.showFingerprint && this.verifyClashPublicKey) ? HttpUtils.post('api/tlsFingerprint', payload) : Promise.resolve(undefined),
        this.showTLSTemplateSelect ? HttpUtils.post('api/tlsSelfSignedTemplate', payload) : Promise.resolve(undefined),
      ])
      if (requestId !== this.serverCertRefreshSeq) {
        return
      }

      if (algorithmMsg?.success && algorithmMsg.obj) {
        this.applyAlgorithmInfo(algorithmMsg.obj)
      }

      if (this.showSHA256) {
        const sha = this.extractSha256Result(shaMsg)
        if (sha) {
          this.outTls.certificate_public_key_sha256 = [sha]
          this.outTls.certificate = undefined
          this.outTls.certificate_path = undefined
        }
      }

      if (this.showFingerprint && this.verifyClashPublicKey) {
        const fingerprint = this.extractFingerprintResult(fingerprintMsg)
        if (fingerprint) {
          this.outTls.fingerprint = fingerprint
        }
      }

      if (this.showTLSTemplateSelect) {
        this.applyTLSTemplateDetectionResult(templateMsg)
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

      const requestId = ++this.clientCertRefreshSeq
      const msg = await HttpUtils.post('api/tlsSha256', payload)
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
      const msg = await HttpUtils.post('api/tlsSha256', payload)
      this.serverSha256Loading = false
      if (!msg.success) {
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
      const msg = await HttpUtils.post('api/tlsSha256', payload)
      this.clientSha256Loading = false
      if (!msg.success) {
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
      const msg = await HttpUtils.post('api/tlsFingerprint', payload)
      this.serverFingerprintLoading = false
      if (!msg.success) {
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
          this.showFingerprint = false
          this.outTls.fingerprint = undefined
        }
      }
    },
    server_port: {
      get() { return this.inTls.reality?.handshake?.server_port ? this.inTls.reality.handshake.server_port : 443 },
      set(v: any) {
        if (this.inTls.reality){
          this.inTls.reality.handshake.server_port = v.length == 0 || v == 0 ? 443 : parseInt(v)
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
      get() { return this.inTls?.reality?.max_time_difference ? this.inTls.reality.max_time_difference.replace('m','') : 1 },
      set(v: number) {
        if (this.inTls.reality){
          this.inTls.reality.max_time_difference = v > 0 ? v + 'm' : '1m'
        }
      }
    },
    optionSNI: {
      get(): boolean { return this.inTls.server_name != undefined },
      set(v:boolean) { this.inTls.server_name = v ? '' : undefined }
    },
    optionALPN: {
      get(): boolean { return this.inTls.alpn != undefined },
      set(v:boolean) { this.inTls.alpn = v ? defaultInTls.alpn : undefined }
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
      get(): boolean { return this.inTls.cipher_suites != undefined },
      set(v:boolean) { this.inTls.cipher_suites = v ? defaultInTls.cipher_suites : undefined }
    },
    optionTlsStore: {
      get(): boolean { return this.outTls.tls_store != undefined },
      set(v:boolean) { this.tls.client.tls_store = v ? 'mozilla' : undefined }
    },
    optionFP: {
      get(): boolean { return this.outTls.utls != undefined },
      set(v:boolean) { this.outTls.utls = v ? defaultOutTls.utls : undefined }
    },
    optionSHA256: {
      get(): boolean { return this.showSHA256 },
      set(v:boolean) {
        if (!this.verifyPublicKey && v) {
          return
        }
        this.showSHA256 = v
        if (!v) {
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
      set(v:boolean) { this.outTls.ech = v ? defaultOutTls.ech : undefined }
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
  components: { AcmeVue, EchVue }
}
</script>

<style scoped>
.tls-certificate-select {
  max-width: 100%;
}
</style>
