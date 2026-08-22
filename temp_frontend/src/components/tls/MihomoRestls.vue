<template>
  <div class="pa-3 border rounded">
    <div class="text-subtitle-2 mb-3">Restls</div>
    <v-row>
      <v-col cols="12"><v-text-field :model-value="server.dest" @update:model-value="onDestChange" @blur="normalizeDest" label="dest (回落目标地址)" hint="例如 www.example.com:443；支持 [IPv6]:端口，省略端口时自动补 443" persistent-hint hide-details="auto" /></v-col>
    </v-row>
    <v-row>
      <v-col cols="12">
        <v-text-field
          v-model="sharedPassword"
          label="password (密码)"
          hide-details
          append-icon="mdi-refresh"
          @click:append="refreshCredentials" />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12"><v-textarea v-model="restlsScript" label="restls-script (脚本，可选)" hint="服务端 res-tls 与客户端 restls-opts 使用同一脚本" persistent-hint auto-grow rows="2" hide-details="auto" /></v-col>
    </v-row>
    <v-row>
      <v-col cols="12"><v-text-field v-model.number="server.min_record_len" type="number" min="0" label="min-record-len (最小记录长度)" hide-details /></v-col>
    </v-row>
    <v-row>
      <v-col cols="12">
        <v-text-field
          v-model="server.proxy"
          label="proxy (服务端代理，可选)"
          hint="（服务端出站节点名称或代理组名称）"
          persistent-hint
          hide-details="auto" />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="4"><v-select v-model="client.version_hint" :items="versionHints" label="version-hint (TLS 版本提示)" hide-details /></v-col>
      <v-col cols="12" sm="6" md="4"><v-text-field v-model.number="server.rate_limit" type="number" min="0" label="rate-limit (回落限速 bit/s)" hide-details /></v-col>
    </v-row>
    <v-row>
      <v-col cols="12">
        <v-text-field
          v-model="clientSni"
          label="客户端 SNI（server_name，可选）"
          hint="仅写入客户端 server_name；可与服务端回落目标 dest 不同"
          persistent-hint
          hide-details="auto" />
      </v-col>
    </v-row>
  </div>
</template>

<script lang="ts">
import { mihomoTlsSniFromDestination, normalizeMihomoTlsDestination } from '@/types/tls'

export default {
  props: {
    server: { type: Object, required: true },
    client: { type: Object, required: true },
    serverName: { type: String, default: '' },
    hasServerName: { type: Boolean, default: false },
  },
  emits: ['update:server-name', 'refresh-credentials'],
  data() {
    return { versionHints: [{ title: 'TLS 1.2', value: 'tls12' }, { title: 'TLS 1.3', value: 'tls13' }] }
  },
  computed: {
    sharedPassword: {
      get(): string {
        const serverPassword = String((this.server as any).password ?? '')
        const clientPassword = String((this.client as any).password ?? '')
        return serverPassword.trim() !== '' ? serverPassword : clientPassword
      },
      set(value: string) {
        const normalized = String(value ?? '')
        ;(this.server as any).password = normalized
        ;(this.client as any).password = normalized
      },
    },
    restlsScript: {
      get(): string {
        const serverScript = String((this.server as any).restls_script ?? '')
        if (serverScript.trim() !== '') return serverScript
        return String((this.client as any).restls_script ?? '')
      },
      set(value: string) {
        const normalized = String(value ?? '')
        if (normalized.trim() === '') {
          delete (this.server as any).restls_script
          delete (this.client as any).restls_script
          return
        }
        ;(this.server as any).restls_script = normalized
        ;(this.client as any).restls_script = normalized
      },
    },
    clientSni: {
      get(): string {
        if (this.hasServerName) return String(this.serverName ?? '')
        return mihomoTlsSniFromDestination((this.server as any).dest)
      },
      set(value: string) {
        this.$emit('update:server-name', String(value ?? '').trim())
      },
    },
  },
  methods: {
    refreshCredentials() {
      this.$emit('refresh-credentials')
    },
    onDestChange(value: unknown) {
      const dest = String(value ?? '').trim()
      ;(this.server as any).dest = dest
    },
    normalizeDest() {
      ;(this.server as any).dest = normalizeMihomoTlsDestination((this.server as any).dest)
    },
  },
}
</script>
