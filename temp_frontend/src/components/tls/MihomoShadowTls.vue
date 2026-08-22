<template>
  <div class="pa-3 border rounded">
    <div class="text-subtitle-2 mb-3">ShadowTLS</div>
    <v-row>
      <v-col cols="12" sm="4">
      <v-select v-model="server.version" :items="versions" label="version (版本)" hide-details @update:model-value="onVersionChange" />
      </v-col>
    </v-row>
    <v-row v-if="Number(server.version) === 2">
      <v-col cols="12">
        <v-text-field
          v-model="sharedPassword"
          label="password (密码)"
          hide-details
          append-icon="mdi-refresh"
          @click:append="refreshCredentials" />
      </v-col>
    </v-row>
    <v-row v-if="Number(server.version) === 3">
      <v-col cols="12">
        <v-text-field
          v-model="sharedUsername"
          label="name (用户名)"
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
      <v-col cols="12"><v-text-field :model-value="server.handshake.dest" @update:model-value="onDestChange" @blur="normalizeDest" label="dest (默认握手目标地址)" :hint="defaultHandshakeHint" persistent-hint hide-details="auto" /></v-col>
    </v-row>
    <v-row>
      <v-col cols="12">
        <v-text-field
          v-model="server.handshake.proxy"
          label="proxy (握手代理，可选)"
          hint="（服务端出站节点名称或代理组名称）"
          persistent-hint
          hide-details="auto" />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12">
        <v-text-field
          v-model="clientSni"
          label="客户端 SNI（server_name，可选）"
          hint="仅写入客户端 server_name；可与默认握手目标 dest 不同，v2/v3 可用于匹配下方按 SNI 规则"
          persistent-hint
          hide-details="auto" />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="4" v-if="Number(server.version) === 3"><v-switch v-model="server.strict_mode" label="strict-mode (严格模式)" color="primary" hide-details /></v-col>
      <v-col cols="12" sm="4" v-if="Number(server.version) >= 3"><v-select v-model="server.wildcard_sni" :items="wildcardSniItems" label="wildcard-sni (通配 SNI)" hide-details /></v-col>
    </v-row>
    <div v-if="Number(server.version) > 1" class="handshake-rules-header mb-2">
      <span class="text-body-2">handshake-for-server-name (v2/v3 按 SNI 指定握手目标，可选)</span>
      <v-btn size="small" variant="tonal" prepend-icon="mdi-plus" @click="addHandshakeForServerName">添加规则</v-btn>
    </div>
    <div v-if="Number(server.version) > 1" class="handshake-rules">
      <section v-for="(rule, index) in handshakeRules" :key="rule.id" class="handshake-rule-group">
        <div class="handshake-rule-title mb-2">
          <span class="text-body-2">规则 {{ index + 1 }}</span>
          <v-btn
            icon="mdi-delete-outline"
            variant="text"
            color="error"
            aria-label="删除规则"
            @click="removeHandshakeForServerName(rule.id)">
            <v-tooltip activator="parent" location="top" text="删除规则" />
          </v-btn>
        </div>
        <v-row>
          <v-col cols="12" sm="5">
            <v-text-field
              :model-value="rule.sni"
              label="SNI (匹配域名)"
              hide-details
              @update:model-value="updateHandshakeRule(rule.id, 'sni', $event)" />
          </v-col>
          <v-col cols="12" sm="7">
            <v-text-field
              :model-value="rule.dest"
              label="dest (握手目标地址)"
              hint="例如 www.example.com:443"
              persistent-hint
              hide-details="auto"
              @update:model-value="updateHandshakeRule(rule.id, 'dest', $event)"
              @blur="normalizeHandshakeRuleDestination(rule.id)" />
          </v-col>
        </v-row>
        <v-row>
          <v-col cols="12">
            <v-text-field
              :model-value="rule.proxy"
              label="proxy (握手代理，可选)"
              hint="（服务端出站节点名称或代理组名称）"
              persistent-hint
              hide-details="auto"
              @update:model-value="updateHandshakeRule(rule.id, 'proxy', $event)" />
          </v-col>
        </v-row>
      </section>
    </div>
  </div>
</template>

<script lang="ts">
import { mihomoTlsSniFromDestination, normalizeMihomoTlsDestination } from '@/types/tls'

type HandshakeRuleDraft = {
  id: number
  sni: string
  dest: string
  proxy: string
}

type HandshakeRulePreparation = {
  valid: boolean
  message?: string
}

export default {
  props: {
    server: { type: Object, required: true },
    client: { type: Object, required: true },
    serverName: { type: String, default: '' },
    hasServerName: { type: Boolean, default: false },
  },
  emits: ['update:server-name', 'refresh-credentials', 'refresh-username', 'refresh-password'],
  data() {
    return {
      versions: [1, 2, 3],
      wildcardSniItems: [
        { title: 'off (关闭)', value: 'off' },
        { title: 'authed (认证后启用)', value: 'authed' },
        { title: 'all (始终启用)', value: 'all' },
      ],
      handshakeRules: [] as HandshakeRuleDraft[],
      handshakeRuleSequence: 0,
    }
  },
  computed: {
    sharedUsername: {
      get(): string {
        const users = (this.server as any).users
        const user = Array.isArray(users) ? users[0] : undefined
        const serverName = String(user?.name ?? user?.username ?? '')
        const clientName = String((this.client as any).username ?? '')
        return serverName.trim() !== '' ? serverName : clientName
      },
      set(value: string) {
        this.ensureUser()
        ;(this.server as any).users[0].name = String(value ?? '')
      },
    },
    sharedPassword: {
      get(): string {
        if (Number((this.server as any).version) === 2) {
          const serverPassword = String((this.server as any).password ?? '')
          const clientPassword = String((this.client as any).password ?? '')
          return serverPassword.trim() !== '' ? serverPassword : clientPassword
        }
        const users = (this.server as any).users
        const user = Array.isArray(users) ? users[0] : undefined
        const serverPassword = String(user?.password ?? '')
        const clientPassword = String((this.client as any).password ?? '')
        return serverPassword.trim() !== '' ? serverPassword : clientPassword
      },
      set(value: string) {
        const normalized = String(value ?? '')
        if (Number((this.server as any).version) === 2) {
          ;(this.server as any).password = normalized
        } else {
          this.ensureUser()
          ;(this.server as any).users[0].password = normalized
        }
        ;(this.client as any).password = normalized
      },
    },
    defaultHandshakeHint(): string {
      const version = Number((this.server as any).version)
      const wildcard = String((this.server as any).wildcard_sni ?? 'off').trim().toLowerCase()
      if (version === 3 && wildcard === 'all') {
        return '例如 www.example.com:443；支持 [IPv6]:端口，省略端口时自动补 443；wildcard-sni=all 时可留空，未匹配的 SNI 将按客户端 SNI:443 处理'
      }
      if (version === 3 && wildcard === 'authed') {
        return '例如 www.example.com:443；支持 [IPv6]:端口，省略端口时自动补 443；wildcard-sni=authed 仅认证连接按客户端 SNI:443 处理，默认目标仍需填写'
      }
      return '例如 www.example.com:443；支持 [IPv6]:端口，省略端口时自动补 443；当前模式需要填写默认目标'
    },
    clientSni: {
      get(): string {
        if (this.hasServerName) return String(this.serverName ?? '')
        return mihomoTlsSniFromDestination((this.server as any).handshake?.dest)
      },
      set(value: string) {
        this.$emit('update:server-name', String(value ?? '').trim())
      },
    },
  },
  methods: {
    normalizeVersionFields() {
      const version = Number((this.server as any).version)
      if (version !== 3) {
        delete (this.server as any).strict_mode
        delete (this.server as any).wildcard_sni
      }
      if (version <= 1) {
        delete (this.server as any).handshake_for_server_name
        this.handshakeRules = []
      }
      if (version === 3 && typeof (this.server as any).strict_mode !== 'boolean') {
        ;(this.server as any).strict_mode = true
      }
      if (version === 3 && !['off', 'authed', 'all'].includes(String((this.server as any).wildcard_sni ?? ''))) {
        ;(this.server as any).wildcard_sni = 'off'
      }
    },
    ensureUser() {
      const users = (this.server as any).users
      if (!Array.isArray(users) || users.length === 0) {
        ;(this.server as any).users = [{ name: '', password: '' }]
      } else {
        ;(this.server as any).users = [users[0]]
      }
    },
    onVersionChange(value: unknown) {
      const version = Number(value)
      if (![1, 2, 3].includes(version)) return
      ;(this.client as any).version = version
      if (version === 3) {
        delete (this.server as any).password
        this.ensureUser()
      } else {
        delete (this.server as any).users
      }
      if (version === 1) {
        delete (this.server as any).password
        delete (this.client as any).password
      }
      if (version < 3) delete (this.server as any).wildcard_sni
      if (version <= 1) {
        delete (this.server as any).handshake_for_server_name
        this.handshakeRules = []
      }
      if (version < 3) delete (this.server as any).strict_mode
      if (version === 3 && typeof (this.server as any).strict_mode !== 'boolean') {
        ;(this.server as any).strict_mode = true
      }
      if (version === 3 && !['off', 'authed', 'all'].includes(String((this.server as any).wildcard_sni ?? ''))) {
        ;(this.server as any).wildcard_sni = 'off'
      }
      this.$emit('refresh-credentials')
    },
    refreshCredentials() {
      this.$emit('refresh-credentials')
    },
    refreshUsername() {
      this.$emit('refresh-username')
    },
    refreshPassword() {
      this.$emit('refresh-password')
    },
    onDestChange(value: unknown) {
      if (!(this.server as any).handshake || typeof (this.server as any).handshake !== 'object') {
        ;(this.server as any).handshake = { dest: '' }
      }
      const dest = String(value ?? '').trim()
      ;(this.server as any).handshake.dest = dest
    },
    normalizeDest() {
      if (!(this.server as any).handshake || typeof (this.server as any).handshake !== 'object') return
      ;(this.server as any).handshake.dest = normalizeMihomoTlsDestination((this.server as any).handshake.dest)
    },
    addHandshakeForServerName() {
      this.handshakeRules.push({
        id: ++this.handshakeRuleSequence,
        sni: '',
        dest: '',
        proxy: '',
      })
    },
    updateHandshakeRule(id: number, field: 'sni' | 'dest' | 'proxy', value: unknown) {
      const rule = this.handshakeRules.find(item => item.id === id)
      if (!rule) return
      rule[field] = String(value ?? '')
      this.syncHandshakeRulesToServer()
    },
    normalizeHandshakeRuleDestination(id: number) {
      const rule = this.handshakeRules.find(item => item.id === id)
      if (!rule) return
      rule.dest = normalizeMihomoTlsDestination(rule.dest)
      this.syncHandshakeRulesToServer()
    },
    removeHandshakeForServerName(id: number) {
      const index = this.handshakeRules.findIndex(item => item.id === id)
      if (index < 0) return
      this.handshakeRules.splice(index, 1)
      this.syncHandshakeRulesToServer()
    },
    loadHandshakeRulesFromServer() {
      this.handshakeRules = []
      this.handshakeRuleSequence = 0
      if (Number((this.server as any).version) <= 1) return
      const source = (this.server as any).handshake_for_server_name
      if (!source || typeof source !== 'object' || Array.isArray(source)) return
      for (const [rawSni, rawRule] of Object.entries(source as Record<string, unknown>)) {
        if (!rawRule || typeof rawRule !== 'object' || Array.isArray(rawRule)) continue
        const value = rawRule as Record<string, unknown>
        this.handshakeRules.push({
          id: ++this.handshakeRuleSequence,
          sni: String(rawSni ?? '').trim(),
          dest: String(value.dest ?? '').trim(),
          proxy: String(value.proxy ?? '').trim(),
        })
      }
    },
    syncHandshakeRulesToServer() {
      const mappings: Record<string, { dest: string, proxy?: string }> = {}
      for (const rule of this.handshakeRules) {
        const sni = rule.sni.trim()
        const dest = rule.dest.trim()
        const proxy = rule.proxy.trim()
        if (!sni || !dest) continue
        mappings[sni] = proxy ? { dest, proxy } : { dest }
      }
      if (Object.keys(mappings).length === 0) {
        delete (this.server as any).handshake_for_server_name
      } else {
        ;(this.server as any).handshake_for_server_name = mappings
      }
    },
    prepareHandshakeRules(): HandshakeRulePreparation {
      const mappings: Record<string, { dest: string, proxy?: string }> = {}
      const seenSni = new Set<string>()
      for (let index = 0; index < this.handshakeRules.length; index++) {
        const rule = this.handshakeRules[index]
        const sni = rule.sni.trim()
        const dest = normalizeMihomoTlsDestination(rule.dest)
        const proxy = rule.proxy.trim()
        rule.sni = sni
        rule.dest = dest
        rule.proxy = proxy

        if (!sni && !dest && !proxy) continue
        if (!sni || !dest) {
          this.syncHandshakeRulesToServer()
          return { valid: false, message: `ShadowTLS 第 ${index + 1} 组按 SNI 握手规则必须同时填写 SNI 和 dest，或点击删除` }
        }
        if (!this.isHostPortAddress(dest)) {
          this.syncHandshakeRulesToServer()
          return { valid: false, message: `ShadowTLS 第 ${index + 1} 组 dest 必须是 host:port` }
        }
        if (seenSni.has(sni)) {
          this.syncHandshakeRulesToServer()
          return { valid: false, message: `ShadowTLS 第 ${index + 1} 组 SNI 与前面的规则重复` }
        }
        seenSni.add(sni)
        mappings[sni] = proxy ? { dest, proxy } : { dest }
      }
      if (Object.keys(mappings).length === 0) {
        delete (this.server as any).handshake_for_server_name
      } else {
        ;(this.server as any).handshake_for_server_name = mappings
      }
      return { valid: true }
    },
    isHostPortAddress(value: unknown): boolean {
      const raw = String(value ?? '').trim()
      const match = raw.match(/^\[([^\]]+)\]:(\d+)$/) ?? raw.match(/^([^:]+):(\d+)$/)
      if (!match) return false
      const port = Number(match[2])
      return Number.isInteger(port) && port >= 1 && port <= 65535
    },
  },
  mounted() {
    this.normalizeVersionFields()
    this.loadHandshakeRulesFromServer()
  },
}
</script>

<style scoped>
.handshake-rules-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.handshake-rules-header > span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.handshake-rules {
  display: grid;
  gap: 12px;
}

.handshake-rule-group {
  border: 1px solid rgba(var(--v-theme-on-surface), 0.16);
  border-radius: 4px;
  padding: 12px;
}

.handshake-rule-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

@media (max-width: 599px) {
  .handshake-rule-group {
    padding: 10px;
  }
}
</style>
