<template>
  <v-card subtitle="SSH">
    <template v-if="optionKey">
      <v-row v-if="!isMihomoNamespace">
        <v-col cols="auto">
          <v-btn-toggle
            v-model="usePath"
            class="rounded-xl"
            density="compact"
            variant="outlined"
            shaped
            mandatory>
            <v-btn @click="data.private_key = undefined; data.private_key_path = ''">{{ $t('tls.usePath') }}</v-btn>
            <v-btn @click="data.private_key_path = undefined; data.private_key = ''">{{ $t('tls.useText') }}</v-btn>
          </v-btn-toggle>
        </v-col>
      </v-row>
      <v-row v-if="!isMihomoNamespace && usePath == 0">
        <v-col cols="12" sm="6">
          <v-text-field :label="$t('tls.keyPath')" hide-details v-model="data.private_key_path"></v-text-field>
        </v-col>
      </v-row>
      <v-row v-else>
        <v-col cols="12" sm="6">
          <v-textarea :label="$t('tls.key')" hide-details v-model="data.private_key"></v-textarea>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6">
          <v-text-field :label="$t('types.ssh.passphrase')" hide-details v-model="data.private_key_passphrase"></v-text-field>
        </v-col>
      </v-row>
    </template>
    <template v-else>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-text-field v-model="usernameModel" :label="$t('types.un')" hide-details></v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field v-model="data.password" :label="$t('types.pw')" hide-details></v-text-field>
        </v-col>
      </v-row>
    </template>

    <v-row v-if="optionHostKey">
      <v-col cols="12" sm="6">
        <v-textarea :label="$t('types.ssh.hostKey')" hide-details v-model="hostKey"></v-textarea>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="4" v-if="data.host_key_algorithms != undefined">
        <v-text-field v-model="algorithms" :label="$t('types.ssh.algorithm') + ' ' + $t('commaSeparated')" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && data.client_version != undefined">
        <v-text-field
          v-model="data.client_version"
          :label="$t('types.ssh.clientVer') + ' (singbox)'"
          hide-details>
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && data.cipher != undefined">
        <v-text-field v-model="cipherAlgorithms" label="cipher (singbox)" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && data.mac != undefined">
        <v-text-field v-model="macAlgorithms" label="mac (singbox)" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="!isMihomoNamespace && data.kex_algorithm != undefined">
        <v-text-field v-model="kexAlgorithms" label="kex_algorithm (singbox)" hide-details></v-text-field>
      </v-col>
    </v-row>

    <v-card-actions>
      <v-spacer></v-spacer>
      <v-menu v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal">{{ $t('types.ssh.options') }}</v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item>
              <v-switch v-model="optionKey" color="primary" label="SSH Key" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionHostKey" color="primary" :label="$t('types.ssh.hostKey')" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionAlgorithms" color="primary" :label="$t('types.ssh.algorithm')" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionVer" color="primary" :label="$t('types.ssh.clientVer') + ' (singbox)'" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionCipher" color="primary" label="cipher (singbox)" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionMac" color="primary" label="mac (singbox)" hide-details></v-switch>
            </v-list-item>
            <v-list-item v-if="!isMihomoNamespace">
              <v-switch v-model="optionKex" color="primary" label="kex_algorithm (singbox)" hide-details></v-switch>
            </v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
export default {
  props: {
    data: {
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
      menu: false,
      usePath: 0,
    }
  },
  computed: {
    isMihomoNamespace(): boolean {
      return this.namespace === 'mihomo'
    },
    usernameModel: {
      get(): string {
        if (typeof this.data.username === 'string') return this.data.username
        if (typeof this.data.user === 'string') return this.data.user
        return ''
      },
      set(v: string) {
        this.data.username = v
        this.data.user = v
      },
    },
    optionKey: {
      get(): boolean {
        return this.data.private_key != undefined || this.data.private_key_path != undefined
      },
      set(v: boolean) {
        this.usePath = 0
        if (v) {
          this.data.private_key = this.data.private_key ?? ''
          if (!this.isMihomoNamespace) {
            this.data.private_key_path = this.data.private_key_path ?? ''
          }
          delete this.data.user
          delete this.data.username
          delete this.data.password
        } else {
          delete this.data.private_key_path
          delete this.data.private_key
          delete this.data.private_key_passphrase
        }
      },
    },
    optionHostKey: {
      get(): boolean { return this.data.host_key != undefined },
      set(v: boolean) { this.data.host_key = v ? [] : undefined },
    },
    optionAlgorithms: {
      get(): boolean { return this.data.host_key_algorithms != undefined },
      set(v: boolean) { this.data.host_key_algorithms = v ? [] : undefined },
    },
    optionVer: {
      get(): boolean { return this.data.client_version != undefined },
      set(v: boolean) { this.data.client_version = v ? 'SSH-2.0-OpenSSH_7.4p1' : undefined },
    },
    optionCipher: {
      get(): boolean { return this.data.cipher != undefined },
      set(v: boolean) { this.data.cipher = v ? [] : undefined },
    },
    optionMac: {
      get(): boolean { return this.data.mac != undefined },
      set(v: boolean) { this.data.mac = v ? [] : undefined },
    },
    optionKex: {
      get(): boolean { return this.data.kex_algorithm != undefined },
      set(v: boolean) { this.data.kex_algorithm = v ? [] : undefined },
    },
    hostKey: {
      get(): string {
        return this.data.host_key ? this.data.host_key.join('\n') : ''
      },
      set(v: string) {
        this.data.host_key = String(v ?? '')
          .split('\n')
          .map((item) => item.trim())
          .filter((item) => item.length > 0)
      },
    },
    algorithms: {
      get(): string {
        return this.data.host_key_algorithms ? this.data.host_key_algorithms.join(',') : ''
      },
      set(v: string) {
        this.data.host_key_algorithms = String(v ?? '')
          .split(/[\n,]+/)
          .map((item) => item.trim())
          .filter((item) => item.length > 0)
      },
    },
    cipherAlgorithms: {
      get(): string {
        return this.data.cipher ? this.data.cipher.join(',') : ''
      },
      set(v: string) {
        this.data.cipher = String(v ?? '')
          .split(/[\n,]+/)
          .map((item) => item.trim())
          .filter((item) => item.length > 0)
      },
    },
    macAlgorithms: {
      get(): string {
        return this.data.mac ? this.data.mac.join(',') : ''
      },
      set(v: string) {
        this.data.mac = String(v ?? '')
          .split(/[\n,]+/)
          .map((item) => item.trim())
          .filter((item) => item.length > 0)
      },
    },
    kexAlgorithms: {
      get(): string {
        return this.data.kex_algorithm ? this.data.kex_algorithm.join(',') : ''
      },
      set(v: string) {
        this.data.kex_algorithm = String(v ?? '')
          .split(/[\n,]+/)
          .map((item) => item.trim())
          .filter((item) => item.length > 0)
      },
    },
  },
  watch: {
    isMihomoNamespace: {
      immediate: true,
      handler(v: boolean) {
        if (!v) return
        delete this.data.private_key_path
        delete this.data.client_version
        delete this.data.cipher
        delete this.data.mac
        delete this.data.kex_algorithm
      },
    },
  },
  mounted() {
    if (this.data.private_key != undefined && this.data.private_key_path == undefined) {
      this.usePath = 1
    }
  },
}
</script>
