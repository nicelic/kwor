<template>
  <v-row no-gutters>
    <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">{{ label }}</v-col>
    <!-- 类型选择 -->
    <v-col :cols="getTypeCols()">
      <v-select
        hide-details
        :label="$t('type')"
        :items="dnsTypes"
        @update:model-value="updateType($event)"
        density="compact"
        :class="!showServerFields ? '' : 'noGutters'"
        v-model="data.type">
      </v-select>
    </v-col>
    <!-- 地址和端口（当类型不是 local 和 dhcp 时显示） -->
    <v-col cols="5" v-if="showServerFields">
      <v-text-field
        v-model="data.server"
        :label="$t('in.addr')"
        density="compact"
        class="noGutters"
        hide-details>
      </v-text-field>
    </v-col>
    <v-col cols="3" v-if="showServerFields">
      <v-text-field
        v-model.number="data.server_port"
        :label="$t('in.port')"
        density="compact"
        type="number"
        class="noGutters"
        min="1"
        hide-details>
      </v-text-field>
    </v-col>
  </v-row>
</template>

<script lang="ts">
export default {
  props: {
    data: {
      type: Object,
      required: true
    },
    label: {
      type: String,
      default: ''
    },
    // 是否为 bootstrap DNS 模式
    isBootstrap: {
      type: Boolean,
      default: false
    },
    // 是否为代理 bootstrap DNS（只能选 local-dns 或 direct-dns）
    isProxyBootstrap: {
      type: Boolean,
      default: false
    }
  },
  data() {
    return {
      // 需要 TLS 配置的类型
      tlsTypes: ['tls', 'quic', 'h3', 'https'],
      // 不需要服务器地址的类型
      noServerTypes: ['local', 'dhcp']
    }
  },
  computed: {
    // 根据不同模式返回不同的 DNS 类型选项
    dnsTypes(): string[] {
      if (this.isProxyBootstrap) {
        // 代理 bootstrap DNS 只能选择 local-dns 或 direct-dns（但这里实际是下拉选择器，需要特殊处理）
        return ['local-dns', 'direct-dns']
      }
      // 普通 DNS 类型选项
      return ['udp', 'tcp', 'local', 'dhcp', 'tls', 'quic', 'h3', 'https']
    },
    // 是否显示服务器字段
    showServerFields(): boolean {
      if (this.isProxyBootstrap) {
        return false // 代理 bootstrap 不显示服务器字段
      }
      return !this.noServerTypes.includes(this.data.type)
    },
    // 是否需要 TLS 配置
    needsTls(): boolean {
      return this.tlsTypes.includes(this.data.type)
    }
  },
  methods: {
    getTypeCols(): number {
      if (this.isProxyBootstrap) {
        return 12
      }
      return this.showServerFields ? 4 : 12
    },
    updateType(t: string) {
      if (this.isProxyBootstrap) {
        // 代理 bootstrap DNS 选择的是 domain_resolver 的值
        // 这种情况下 data 对象是 proxy-dns 的配置
        this.data.domain_resolver = t
        return
      }

      // 如果是不需要服务器的类型
      if (this.noServerTypes.includes(t)) {
        delete this.data.server
        delete this.data.server_port
        delete this.data.tls
        delete this.data.domain_resolver
      } else {
        // 需要服务器的类型
        if (!this.data.server) {
          // 设置默认值
          if (this.tlsTypes.includes(t)) {
            this.data.server = ''
            this.data.server_port = 853
          } else {
            this.data.server = ''
            this.data.server_port = 53
          }
        }

        // 如果是需要 TLS 的类型
        if (this.tlsTypes.includes(t)) {
          if (!this.data.tls) {
            this.data.tls = {
              enabled: true,
              insecure: false,
              min_version: "1.3",
              server_name: this.data.server || ''
            }
          }
          // 确保有 domain_resolver
          if (!this.data.domain_resolver) {
            this.data.domain_resolver = 'local-dns'
          }
        } else {
          // 不需要 TLS 的类型，删除 tls 配置和 domain_resolver
          delete this.data.tls
          delete this.data.domain_resolver
        }
      }
    }
  },
  watch: {
    // 监听服务器地址变化，更新 TLS server_name
    'data.server'(newVal: string) {
      if (this.needsTls && this.data.tls) {
        this.data.tls.server_name = newVal
      }
    }
  }
}
</script>

<style>
.noGutters .v-field__input,
.noGutters .v-field {
  text-align: center !important;
  padding-inline-end: 0 !important;
}
</style>
