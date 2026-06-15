<template>
  <v-card subtitle="Shadowsocks">
    <v-row>
      <v-col cols="12" sm="6" md="4">
        <v-select
          hide-details
          :label="$t('in.ssMethod')"
          :items="ssMethods"
          @update:model-value="direction == 'in' ? changeMethod($event) : undefined"
          v-model="data.method">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <Network :data="data" />
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="direction == 'out'">
        <UoT :data="data" />
      </v-col>
      <!--
        开发者要求隐藏 SS API 专用入口，默认保持 managed=false，非必要不用开启。
        Developer requirement: hide SS API-only entry and keep managed=false by default; do not enable unless necessary.
        不影响 SS/SS2022 节点创建与常规客户端使用。
        This does not affect SS/SS2022 node creation or normal client usage.
      -->
      <v-col cols="12" sm="6" md="4" v-if="false && direction == 'in' && namespace != 'mihomo'">
        <v-switch
          v-model="data.managed"
          color="primary"
          :label="$t('in.ssManageable')"
          hide-details>
        </v-switch>
      </v-col>
    </v-row>
    <v-row v-if="data.method != 'none' || direction == 'out'">
      <v-col cols="12" sm="8">
        <v-text-field
          v-model="data.password"
          :label="$t('types.pw')"
          hide-details
          :append-inner-icon="direction == 'in' ? 'mdi-refresh' : undefined"
          @click:append-inner="changeMethod(data.method)">
        </v-text-field>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">
import Network from '@/components/Network.vue'
import UoT from '@/components/UoT.vue'
import RandomUtil from '@/plugins/randomUtil'

export default {
  props: ['direction','data','namespace'],
  created() {
    // 开发者要求默认关闭并隐藏 SS API 专用开关，统一兜底 managed=false。
    // Developer requirement: default OFF and hidden for SS API-only toggle; enforce managed=false as safety fallback.
    if (this.$props.direction === 'in' && this.$props.namespace !== 'mihomo' && this.$props.data?.managed !== false) {
      this.$props.data.managed = false
    }
  },
  data() {
    return {
      ssMethods: [
        "none",
        "aes-128-gcm",
        "aes-192-gcm",
        "aes-256-gcm",
        "chacha20-ietf-poly1305",
        "xchacha20-ietf-poly1305",
        "2022-blake3-aes-128-gcm",
        "2022-blake3-aes-256-gcm",
        "2022-blake3-chacha20-poly1305"
      ]
    }
  },
  methods: {
    changeMethod(ssMethod :string) {
      if (ssMethod.startsWith('2022')) {
        this.$props.data.password = ssMethod == "2022-blake3-aes-128-gcm" ? RandomUtil.randomShadowsocksPassword(16) : RandomUtil.randomShadowsocksPassword(32)
      } else if (ssMethod == 'none') {
        delete this.$props.data.password
      } else {
        this.$props.data.password = RandomUtil.randomSeq(10)
      }
    }
  },
  components: { Network, UoT }
}
</script>
