<template>
  <v-card subtitle="ECH" style="background-color: inherit;">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-switch color="primary" :label="$t('enable')" v-model="enabled" hide-details></v-switch>
      </v-col>
    </v-row>
    <template v-if="enabled">
      <v-row>
        <v-col cols="auto">
          <v-btn-toggle v-model="useEchPath"
          class="rounded-xl"
          density="compact"
          variant="outlined"
          shaped
          mandatory>
            <v-btn :value="0">{{ $t('tls.usePath') }}</v-btn>
            <v-btn :value="1">{{ $t('tls.useText') }}</v-btn>
          </v-btn-toggle>
        </v-col>
        <v-spacer></v-spacer>
        <v-col cols="auto">
          <v-btn
            variant="tonal"
            density="compact"
            icon="mdi-key-star"
            @click="genECH"
            :loading="loading">
            <v-icon />
            <v-tooltip activator="parent" location="top">
              {{ $t('actions.generate') }}
            </v-tooltip>
          </v-btn>
        </v-col>
      </v-row>
      <v-row v-if="useEchPath == 0">
        <v-col cols="12">
          <v-text-field
            :label="$t('tls.keyPath')"
            hide-details
            v-model="ech.key_path">
          </v-text-field>
        </v-col>
      </v-row>
      <v-row v-else>
        <v-col cols="12">
          <v-textarea
            :label="$t('tls.key')"
            hide-details
            v-model="echKeyText">
          </v-textarea>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="auto">
          <v-btn-toggle v-model="useEchConfigPath"
          class="rounded-xl"
          density="compact"
          variant="outlined"
          shaped
          mandatory>
            <v-btn :value="0">{{ $t('tls.usePath') }}</v-btn>
            <v-btn :value="1">{{ $t('tls.useText') }}</v-btn>
          </v-btn-toggle>
        </v-col>
      </v-row>
      <v-row v-if="useEchConfigPath == 0">
        <v-col cols="12">
          <v-text-field
            label="ECH Config Path"
            hide-details
            v-model="echConfigPath">
          </v-text-field>
        </v-col>
      </v-row>
      <v-row v-else>
        <v-col cols="12">
          <v-textarea
            label="ECH Config"
            hide-details
            v-model="echConfigText">
          </v-textarea>
        </v-col>
      </v-row>
    </template>
  </v-card>
</template>

<script lang="ts">
import { i18n } from '@/locales'
import HttpUtils from '@/plugins/httputil'
import { ech } from '@/types/tls'
import { push } from 'notivue'

export default {
  props: ['iTls','oTls'],
  data() {
    return {
      loading: false,
      echController: undefined as AbortController | undefined,
      echRequestSeq: 0,
    }
  },
  methods: {
    normalizeEchSources() {
      const server = this.$props.iTls?.ech
      const client = this.$props.oTls?.ech
      if (server && Array.isArray(server.key) && server.key.length > 0) {
        delete server.key_path
      }
      if (client && Array.isArray(client.config) && client.config.length > 0) {
        delete client.config_path
      }
    },
    ensureEchPayloads(): boolean {
      if (!this.$props.iTls || !this.$props.oTls) return false
      if (!this.$props.iTls.ech || typeof this.$props.iTls.ech !== 'object' || Array.isArray(this.$props.iTls.ech)) {
        this.$props.iTls.ech = { enabled: true }
      }
      if (!this.$props.oTls.ech || typeof this.$props.oTls.ech !== 'object' || Array.isArray(this.$props.oTls.ech)) {
        this.$props.oTls.ech = { enabled: true }
      }
      this.normalizeEchSources()
      return true
    },
    async genECH(){
      if (!this.ensureEchPayloads()) return
      this.loading = true
      this.echController?.abort()
      const controller = new AbortController()
      this.echController = controller
      const requestId = ++this.echRequestSeq
      try {
        let msg
        try {
          msg = await HttpUtils.get('api/keypairs', {
            k: "ech",
            o: this.iTls.server_name?? "''"
          }, { signal: controller.signal, silentErrorToast: true })
        } catch {
          return
        }
        if (requestId !== this.echRequestSeq || this.echController !== controller) return
        if (!msg.success || !Array.isArray(msg.obj) || !this.iTls.ech || !this.oTls.ech) {
          push.error({
            message: i18n.global.t('error') + ": " + msg.obj
          })
          return
        }
        if (msg.obj.length === 0) {
          push.error({
            message: i18n.global.t('error') + ": " + msg.obj
          })
          return
        }

        this.iTls.ech.key_path=undefined
        this.useEchPath = 1
        let config = <string[]>[]
        let key = <string[]>[]
        let isConfig = false
        let isKey = false

        msg.obj.forEach((line:string) => {
          if (line === "-----BEGIN ECH CONFIGS-----") {
            isConfig = true
            isKey = false
            config.push(line)
          } else if (line === "-----END ECH CONFIGS-----") {
            isConfig = false
            config.push(line)
          } else if (line === "-----BEGIN ECH KEYS-----") {
            isKey = true
            isConfig = false
            key.push(line)
          } else if (line === "-----END ECH KEYS-----") {
            isKey = false
            key.push(line)
          } else if (isConfig) {
            config.push(line)
          } else if (isKey) {
            key.push(line)
          }
        })
        if (key.length === 0 || config.length === 0) {
          push.error({
            message: i18n.global.t('error') + ': ECH 返回内容不完整'
          })
          return
        }
        this.iTls.ech.key = key
        this.oTls.ech.config = config
        delete this.oTls.ech.config_path
      } finally {
        if (this.echController === controller) this.echController = undefined
        if (requestId === this.echRequestSeq) this.loading = false
      }
    },
  },
  computed: {
    ech() {
      return <ech>this.$props.iTls.ech
    },
    enabled: {
      get() { return this.ech?.enabled?? false },
      set(v: boolean) { 
        if (!v) {
          this.echController?.abort()
          this.echRequestSeq++
          this.loading = false
          this.$props.iTls.ech = undefined
          this.$props.oTls.ech = undefined
          return
        }
        this.$props.iTls.ech = { enabled: true }
        this.$props.oTls.ech = { enabled: true }
      this.normalizeEchSources()
      }
    },
    useEchPath: {
      get(): number { return Array.isArray(this.ech?.key) && this.ech.key.length > 0 ? 1 : 0 },
      set(value: number) {
        if (!this.ech) return
        if (Number(value) === 1) delete this.ech.key_path
        else delete this.ech.key
      }
    },
    useEchConfigPath: {
      get(): number { return Array.isArray(this.oTls.ech?.config) && this.oTls.ech.config.length > 0 ? 1 : 0 },
      set(value: number) {
        if (!this.ensureEchPayloads()) return
        if (Number(value) === 1) delete this.oTls.ech.config_path
        else delete this.oTls.ech.config
      }
    },
    echKeyText: {
      get(): string { return this.ech?.key ? this.ech.key.join('\n') : '' },
      set(newValue:string) {
        if (!this.ech) return
        const lines = newValue.split('\n').map(line => line.trim()).filter(line => line.length > 0)
        this.ech.key = lines.length > 0 ? lines : undefined
        if (lines.length > 0) delete this.ech.key_path
      }
    },
    echConfigText: {
      get(): string { return this.oTls.ech?.config ? this.oTls.ech.config.join('\n') : '' },
      set(newValue:string) {
        if (!this.ensureEchPayloads()) return
        const lines = newValue.split('\n').map(line => line.trim()).filter(line => line.length > 0)
        this.oTls.ech.config = lines.length > 0 ? lines : undefined
        if (lines.length > 0) delete this.oTls.ech.config_path
      }
    },
    echConfigPath: {
      get(): string { return this.oTls.ech?.config_path ?? '' },
      set(newValue: string) {
        if (!this.ensureEchPayloads()) return
        const path = newValue.trim()
        this.oTls.ech.config_path = path.length > 0 ? path : undefined
        if (path.length > 0) delete this.oTls.ech.config
      }
    },
  },
  mounted() {
    this.normalizeEchSources()
  },
  beforeUnmount() {
    this.echController?.abort()
    this.echRequestSeq++
  }
}
</script>
