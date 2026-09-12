<template>
  <v-card subtitle="Hysteria2 Realm (中介服务器)">
    <v-row>
      <v-col cols="12" sm="6">
        <v-text-field
          label="Token (鉴权令牌)"
          placeholder="public"
          hint="客户端连接中介服务器的访问令牌（默认 public）"
          persistent-hint
          hide-details="auto"
          v-model="data.token">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6">
        <v-text-field
          label="Realm Name Pattern (房间名正则匹配)"
          placeholder="^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$"
          hint="允许注册的房间名正则表达式（留空默认官方规则）"
          persistent-hint
          hide-details="auto"
          v-model="data.realm_name_pattern">
        </v-text-field>
      </v-col>
    </v-row>

    <v-row v-if="optionLimits">
      <v-col cols="12" sm="6">
        <v-text-field
          label="max-realms (最大房间数)"
          type="number"
          min="1"
          placeholder="65536"
          hide-details
          v-model.number="maxRealms">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6">
        <v-text-field
          label="max-realms-per-ip (单 IP 最大房间数)"
          type="number"
          min="1"
          placeholder="4"
          hide-details
          v-model.number="maxRealmsPerIP">
        </v-text-field>
      </v-col>
    </v-row>

    <v-row v-if="optionTrustedProxy">
      <v-col cols="12">
        <v-text-field
          label="trusted-proxy-header (受信任反代请求头)"
          placeholder="X-Forwarded-For"
          hint="反向代理（如 Nginx、CDN）后用于识别客户端真实 IP 的 Header"
          persistent-hint
          hide-details="auto"
          v-model="data.trusted_proxy_header">
        </v-text-field>
      </v-col>
    </v-row>

    <v-card-actions>
      <v-spacer></v-spacer>
      <v-menu v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal">REALM 选项</v-btn>
        </template>
        <v-card min-width="260">
          <v-list>
            <v-list-item>
              <v-switch v-model="optionLimits" color="primary" label="配额与房间数限制" hide-details></v-switch>
            </v-list-item>
            <v-list-item>
              <v-switch v-model="optionTrustedProxy" color="primary" label="反代请求头 (trusted-proxy-header)" hide-details></v-switch>
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
    direction: {
      type: String,
      default: 'in',
    },
    namespace: {
      type: String,
      default: 'default',
    },
  },
  data() {
    return {
      menu: false,
    }
  },
  computed: {
    optionLimits: {
      get(): boolean {
        return this.data.max_realms !== undefined || this.data.max_realms_per_ip !== undefined
      },
      set(enabled: boolean) {
        if (!enabled) {
          delete this.data.max_realms
          delete this.data.max_realms_per_ip
        } else {
          if (this.data.max_realms === undefined) this.data.max_realms = 65536
          if (this.data.max_realms_per_ip === undefined) this.data.max_realms_per_ip = 4
        }
      },
    },
    optionTrustedProxy: {
      get(): boolean {
        return typeof this.data.trusted_proxy_header === 'string' && this.data.trusted_proxy_header.trim() !== ''
      },
      set(enabled: boolean) {
        if (!enabled) {
          delete this.data.trusted_proxy_header
        } else {
          this.data.trusted_proxy_header = 'X-Forwarded-For'
        }
      },
    },
    maxRealms: {
      get(): number | '' {
        const val = this.data.max_realms
        return typeof val === 'number' && val > 0 ? val : ''
      },
      set(val: number | string) {
        const num = Number(val)
        if (Number.isInteger(num) && num > 0) {
          this.data.max_realms = num
        } else {
          delete this.data.max_realms
        }
      },
    },
    maxRealmsPerIP: {
      get(): number | '' {
        const val = this.data.max_realms_per_ip
        return typeof val === 'number' && val > 0 ? val : ''
      },
      set(val: number | string) {
        const num = Number(val)
        if (Number.isInteger(num) && num > 0) {
          this.data.max_realms_per_ip = num
        } else {
          delete this.data.max_realms_per_ip
        }
      },
    },
  },
  created() {
    if (this.data.token === undefined || this.data.token === null) {
      this.data.token = 'public'
    }
    if (this.data.realm_name_pattern === undefined || this.data.realm_name_pattern === null) {
      this.data.realm_name_pattern = '^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$'
    }
  },
}
</script>
