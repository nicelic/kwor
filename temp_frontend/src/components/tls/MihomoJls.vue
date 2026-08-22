<template>
  <div class="pa-3 border rounded">
    <div class="text-subtitle-2 mb-3">JLS</div>
    <v-row>
      <v-col cols="12">
        <v-text-field
          v-model="sharedUsername"
          label="username (用户名)"
          hide-details
          append-icon="mdi-refresh"
          @click:append="refreshUsername" />
      </v-col>
      <v-col cols="12">
        <v-text-field
          v-model="sharedPassword"
          label="password (密码)"
          hide-details
          append-icon="mdi-refresh"
          @click:append="refreshPassword" />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12"><v-text-field :model-value="server.dest" @update:model-value="onDestChange" @blur="normalizeDest" label="dest (回落目标地址)" hint="例如 www.example.com:443；支持 [IPv6]:端口，省略端口时自动补 443" persistent-hint hide-details="auto" /></v-col>
    </v-row>
    <v-row>
      <v-col cols="12" md="6"><v-select v-model="server.alpn" :items="alpn" multiple label="alpn (协议协商，可选)" hide-details /></v-col>
      <v-col cols="12" md="6"><v-text-field v-model.number="server.rate_limit" type="number" min="0" label="rate-limit (回落限速 bit/s)" hide-details /></v-col>
    </v-row>
    <v-row>
      <v-col cols="12">
        <v-text-field
          v-model="server.proxy"
          label="proxy (代理，可选)"
          hint="（服务端出站节点名称或代理组名称）"
          persistent-hint
          hide-details="auto" />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12">
        <v-text-field
          v-model="sharedSni"
          label="SNI（客户端与服务端必须一致）"
          hint="保存时同步到客户端 server_name 和服务端 jls_config.sni；可与回落目标 dest 不同"
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
  emits: ['update:server-name', 'refresh-username', 'refresh-password'],
  data() {
    return { alpn: ['h2', 'http/1.1'] }
  },
  computed: {
    sharedUsername: {
      get(): string {
        const users = (this.server as any).users
        const serverUsername = Array.isArray(users) ? String(users[0]?.username ?? '') : ''
        const clientUsername = String((this.client as any).username ?? '')
        return serverUsername.trim() !== '' ? serverUsername : clientUsername
      },
      set(value: string) {
        this.ensureUser()
        const normalized = String(value ?? '')
        ;(this.server as any).users[0].username = normalized
        ;(this.client as any).username = normalized
      },
    },
    sharedPassword: {
      get(): string {
        const users = (this.server as any).users
        const serverPassword = Array.isArray(users) ? String(users[0]?.password ?? '') : ''
        const clientPassword = String((this.client as any).password ?? '')
        return serverPassword.trim() !== '' ? serverPassword : clientPassword
      },
      set(value: string) {
        const normalized = String(value ?? '')
        this.ensureUser()
        ;(this.server as any).users[0].password = normalized
        ;(this.client as any).password = normalized
      },
    },
    sharedSni: {
      get(): string {
        if (this.hasServerName) return String(this.serverName ?? '')
        const serverSni = String((this.server as any).sni ?? '').trim()
        if (serverSni !== '') return serverSni
        return mihomoTlsSniFromDestination((this.server as any).dest)
      },
      set(value: string) {
        const normalized = String(value ?? '').trim()
        if (normalized === '') delete (this.server as any).sni
        else (this.server as any).sni = normalized
        this.$emit('update:server-name', normalized)
      },
    },
  },
  methods: {
    ensureUser() {
      const users = (this.server as any).users
      if (!Array.isArray(users) || users.length === 0) {
        ;(this.server as any).users = [{ username: '', password: '' }]
      } else {
        ;(this.server as any).users = [users[0]]
      }
    },
    refreshUsername() {
      this.$emit('refresh-username')
    },
    refreshPassword() {
      this.$emit('refresh-password')
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
