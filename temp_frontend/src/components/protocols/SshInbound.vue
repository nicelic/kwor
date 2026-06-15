<template>
  <v-card subtitle="SSH">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-text-field v-model="usernameModel" label="username" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field v-model="data.password" label="password" hide-details></v-text-field>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12">
        <v-textarea
          v-model="data.private_key"
          label="private-key"
          placeholder="Private key content or path"
          auto-grow
          rows="2"
          hide-details>
        </v-textarea>
      </v-col>
    </v-row>

    <v-row v-if="!isMihomoNamespace">
      <v-col cols="12">
        <v-text-field
          v-model="data.private_key_path"
          label="private_key_path (singbox)"
          placeholder="$HOME/.ssh/id_rsa"
          hide-details>
        </v-text-field>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-text-field v-model="data.private_key_passphrase" label="private-key-passphrase" hide-details></v-text-field>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12">
        <v-textarea
          v-model="hostKeyText"
          label="host-key"
          placeholder="One host key per line; empty means accept all"
          auto-grow
          rows="2"
          hide-details>
        </v-textarea>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6">
        <v-text-field
          v-model="hostKeyAlgorithmsText"
          label="host-key-algorithms"
          placeholder="rsa, ecdsa"
          hide-details>
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" v-if="!isMihomoNamespace">
        <v-text-field v-model="data.client_version" label="client_version (singbox)" hide-details></v-text-field>
      </v-col>
    </v-row>

    <v-row v-if="!isMihomoNamespace">
      <v-col cols="12" sm="4">
        <v-text-field v-model="cipherText" label="cipher (singbox)" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="4">
        <v-text-field v-model="macText" label="mac (singbox)" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="4">
        <v-text-field v-model="kexText" label="kex_algorithm (singbox)" hide-details></v-text-field>
      </v-col>
    </v-row>
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
    hostKeyText: {
      get(): string {
        return Array.isArray(this.data.host_key) ? this.data.host_key.join('\n') : ''
      },
      set(v: string) {
        const list = String(v ?? '')
          .split('\n')
          .map((item) => item.trim())
          .filter((item) => item !== '')
        this.data.host_key = list.length > 0 ? list : undefined
      },
    },
    hostKeyAlgorithmsText: {
      get(): string {
        return Array.isArray(this.data.host_key_algorithms)
          ? this.data.host_key_algorithms.join(', ')
          : ''
      },
      set(v: string) {
        const list = String(v ?? '')
          .split(/[\n,]+/)
          .map((item) => item.trim())
          .filter((item) => item !== '')
        this.data.host_key_algorithms = list.length > 0 ? list : undefined
      },
    },
    cipherText: {
      get(): string {
        return Array.isArray(this.data.cipher) ? this.data.cipher.join(', ') : ''
      },
      set(v: string) {
        const list = String(v ?? '')
          .split(/[\n,]+/)
          .map((item) => item.trim())
          .filter((item) => item !== '')
        this.data.cipher = list.length > 0 ? list : undefined
      },
    },
    macText: {
      get(): string {
        return Array.isArray(this.data.mac) ? this.data.mac.join(', ') : ''
      },
      set(v: string) {
        const list = String(v ?? '')
          .split(/[\n,]+/)
          .map((item) => item.trim())
          .filter((item) => item !== '')
        this.data.mac = list.length > 0 ? list : undefined
      },
    },
    kexText: {
      get(): string {
        return Array.isArray(this.data.kex_algorithm) ? this.data.kex_algorithm.join(', ') : ''
      },
      set(v: string) {
        const list = String(v ?? '')
          .split(/[\n,]+/)
          .map((item) => item.trim())
          .filter((item) => item !== '')
        this.data.kex_algorithm = list.length > 0 ? list : undefined
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
}
</script>
