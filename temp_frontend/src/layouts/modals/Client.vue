<template>
  <v-dialog transition="dialog-bottom-transition" width="800">
    <v-card class="rounded-lg" :loading="loading">
      <v-card-title>
        {{ $t('actions.' + title) + ' ' + $t('objects.client') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-skeleton-loader
        class="mx-auto border"
        width="95%"
        type="card, text, divider, list-item-two-line"
        v-if="loading">
      </v-skeleton-loader>
      <v-card-text style="padding: 0 16px; overflow-y: scroll;">
        <v-container style="padding: 0;" :hidden="loading">
          <v-tabs
            v-model="tab"
            align-tabs="center">
            <v-tab value="t1">{{ $t('client.basics') }}</v-tab>
            <v-tab value="t2">{{ $t('client.config') }}</v-tab>
            <v-tab value="t3">{{ $t('client.links') }}</v-tab>
          </v-tabs>
          <v-window v-model="tab">
            <v-window-item value="t1">
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-switch color="primary" v-model="client.enable" :label="$t('enable')" hide-details></v-switch>
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-combobox v-model="client.group" :items="groups" :label="$t('client.group')" hide-details></v-combobox>
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field
                    v-model="speedLimitInput"
                    :label="'\u9650\u901f-mbps'"
                    placeholder="200 / 200mbps"
                    clearable
                    hide-details
                    @blur="trimSpeedLimitInput">
                  </v-text-field>
                </v-col>
              </v-row>
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field v-model="client.name" :label="$t('client.name')" hide-details></v-text-field>
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field v-model="client.desc" :label="$t('client.desc')" hide-details></v-text-field>
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-combobox
                    v-model="serverIpValue"
                    :items="serverIpList"
                    :label="$t('client.serverIp')"
                    :loading="loadingIps"
                    hide-details
                    clearable
                    @blur="trimServerIp"
                    @keydown.enter="trimServerIp">
                    <template v-slot:append>
                      <v-icon @click="refreshServerIps" icon="mdi-refresh" v-tooltip:top="$t('refresh')" :class="{ rotating: loadingIps }" />
                    </template>
                  </v-combobox>
                </v-col>
              </v-row>
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field v-model.number="Volume" type="number" min="0" :label="$t('stats.volume')" suffix="GiB" hide-details></v-text-field>
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <DatePick :expiry="expDate" picker-type="date" submit-mode="day-end" @submit="setDate" />
                  <div class="text-caption text-medium-emphasis mt-1">
                    {{ $t('client.expiryHint') }}
                  </div>
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-select
                    v-model="ResetDay"
                    :items="resetDayOptions"
                    :label="$t('client.extra')"
                    clearable
                    hide-details />
                  <div class="text-caption text-medium-emphasis mt-1">
                    {{ $t('client.resetEventHint') }}
                  </div>
                </v-col>
              </v-row>
              <v-row v-if="(id ?? 0) > 0">
                <v-col cols="12" sm="6" md="4" class="d-flex flex-column">
                  <div class="d-flex justify-space-between align-center">
                    <div>
                      {{ $t('stats.usage') }}: {{ volumeUsage }}
                    </div>
                    <v-btn density="compact" variant="text" icon="mdi-restore" @click="requestTrafficReset">
                      <v-tooltip activator="parent" location="top">
                        {{ $t('client.resetTraffic') }}
                      </v-tooltip>
                      <v-icon />
                    </v-btn>
                  </div>
                  <v-progress-linear
                    v-model="percent"
                    :color="percentColor"
                    v-if="client.volume > 0"
                    bottom>
                  </v-progress-linear>
                </v-col>
                <v-col cols="12" sm="6" md="8">
                  <v-icon icon="mdi-upload" color="orange" /><span class="text-orange">{{ up }}</span>
                  /
                  <v-icon icon="mdi-download" color="success" /><span class="text-success">{{ down }}</span>
                </v-col>
              </v-row>
              <v-row>
                <v-col>
                  <v-select
                    v-model="clientInbounds"
                    :items="inboundTags"
                    :label="$t('client.inboundTags')"
                    clearable
                    multiple
                    chips
                    hide-details>
                    <template v-slot:append>
                      <v-icon @click="setAllInbounds" icon="mdi-set-all" v-tooltip:top="$t('all')" />
                    </template>
                  </v-select>
                </v-col>
              </v-row>
            </v-window-item>
            <v-window-item value="t2">
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-btn variant="tonal" @click="shuffle()">{{ $t('reset') + ' - ' + $t('all') }}<v-icon icon="mdi-refresh" /></v-btn>
                </v-col>
              </v-row>
              <v-row v-for="key in configKeys">
                <v-col cols="12" md="3" align="end" align-self="center">
                  {{ key }}
                  <v-icon @click="shuffle(key)" icon="mdi-refresh" v-tooltip:top="$t('reset')" />
                </v-col>
                <v-col>
                  <template v-if="showsUsernameField(key) || clientConfig[key].password != undefined || clientConfig[key].uuid != undefined || clientConfig[key].psk != undefined">
                    <v-row>
                      <v-col v-if="showsUsernameField(key)" cols="12" sm="6">
                        <v-text-field
                          label="Username"
                          v-model="clientConfig[key].username"
                          hide-details>
                        </v-text-field>
                      </v-col>
                      <v-col v-if="clientConfig[key].password != undefined" cols="12" :sm="showsUsernameField(key) ? 6 : 12">
                        <v-text-field
                          label="Password"
                          v-model="clientConfig[key].password"
                          hide-details>
                        </v-text-field>
                      </v-col>
                      <v-col v-if="clientConfig[key].uuid != undefined" cols="12" :sm="showsUsernameField(key) ? 6 : 12">
                        <v-text-field
                          :label="key === 'sudoku' ? 'Key / UUID' : key === 'tuic' ? 'Credential ID' : 'UUID'"
                          v-model="clientConfig[key].uuid"
                          hide-details>
                        </v-text-field>
                      </v-col>
                      <v-col v-if="clientConfig[key].psk != undefined" cols="12" :sm="showsUsernameField(key) ? 6 : 12">
                        <v-text-field
                          label="PSK"
                          v-model="clientConfig[key].psk"
                          hide-details>
                        </v-text-field>
                      </v-col>
                    </v-row>
                  </template>
                  <v-text-field
                    v-if="key == 'vless'"
                    label="Flow"
                    v-model="clientConfig[key].flow"
                    hide-details>
                  </v-text-field>
                  <v-text-field
                    v-if="key == 'hysteria'"
                    label="Auth"
                    v-model="clientConfig[key].auth_str"
                    hide-details>
                  </v-text-field>
                </v-col>
              </v-row>
            </v-window-item>
            <v-window-item value="t3">
              <v-row v-for="(lnk, index) in links">
                <v-col cols="auto">{{ index + 1 }}</v-col>
                <v-col style="direction: ltr; overflow-y: hidden;">{{ lnk.uri }}</v-col>
              </v-row>
              <v-row>
                <v-col>
                  <v-btn color="primary" @click="extLinks.push({ type: 'external', uri: '' })">{{ $t('actions.add') }} {{ $t('client.external') }}</v-btn>
                </v-col>
              </v-row>
              <v-row v-for="(lnk, index) in extLinks">
                <v-col>
                  <v-text-field
                    dir="ltr"
                    :label="$t('client.external') + ' ' + (index + 1)"
                    append-icon="mdi-delete"
                    @click:append="extLinks.splice(index, 1)"
                    placeholder="<protocol>://<data>"
                    v-model="lnk.uri" />
                </v-col>
              </v-row>
              <v-row>
                <v-col>
                  <v-btn color="primary" @click="subLinks.push({ type: 'sub', uri: '' })">{{ $t('actions.add') }} {{ $t('client.sub') }}</v-btn>
                </v-col>
              </v-row>
              <v-row v-for="(lnk, index) in subLinks">
                <v-col>
                  <v-text-field
                    dir="ltr"
                    :label="$t('client.sub') + ' ' + (index + 1)"
                    append-icon="mdi-delete"
                    @click:append="subLinks.splice(index, 1)"
                    placeholder="http[s]://<domain>[:]<port>/<path>"
                    v-model="lnk.uri" />
                </v-col>
              </v-row>
            </v-window-item>
          </v-window>
        </v-container>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          color="primary"
          variant="outlined"
          @click="closeModal">
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="loading"
          @click="saveChanges">
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { createClient, updateConfigs, Link, shuffleConfigs, getConfigKeys, supportsEditableUsernameField } from '@/types/clients'
import DatePick from '@/components/DateTime.vue'
import { HumanReadable } from '@/plugins/utils'
import HttpUtils from '@/plugins/httputil'
import { getNamespaceApi, getNamespaceStore } from '@/store/uiNamespace'
import { push } from 'notivue'

export default {
  props: {
    visible: Boolean,
    id: Number,
    inboundTags: Array,
    groups: Array,
    namespace: {
      type: String,
      default: 'default',
    },
  },
  emits: ['close'],
  data() {
    return {
      client: createClient(),
      initialClientName: '',
      title: 'add',
      loading: false,
      loadingIps: false,
      tab: 't1',
      clientConfig: <any>[],
      links: <Link[]>[],
      extLinks: <Link[]>[],
      subLinks: <Link[]>[],
      serverIpList: <string[]>[],
      inboundIpMap: <Record<number, string>>{},
      speedLimitInput: '',
    }
  },
  methods: {
    normalizeResetDayValue(raw: unknown): number {
      const value = Number(raw)
      if (!Number.isFinite(value) || value <= 0) {
        return 0
      }
      return Math.min(31, Math.floor(value))
    },
    normalizeInboundIds(raw: unknown): number[] {
      if (!Array.isArray(raw)) {
        return []
      }

      const normalized: number[] = []
      const seen = new Set<number>()
      for (const item of raw) {
        const value = Number(item)
        if (!Number.isInteger(value) || value <= 0 || seen.has(value)) {
          continue
        }
        seen.add(value)
        normalized.push(value)
      }
      return normalized
    },
    validateMihomoSnellInboundBindings(clientId: number, inboundIds: number[]): boolean {
      if (this.namespace !== 'mihomo') {
        return true
      }

      const store = getNamespaceStore(this.namespace)
      const snellInbounds = (store.inbounds ?? []).filter((inbound: any) =>
        inbound?.type === 'snell' && inboundIds.includes(Number(inbound.id ?? 0)),
      )
      if (snellInbounds.length === 0) {
        return true
      }

      const clients = store.clients ?? []
      for (const inbound of snellInbounds) {
        const owner = clients.find((item: any) => {
          if (Number(item?.id ?? 0) === clientId) return false
          const bindings = this.normalizeInboundIds(item?.inbounds)
          return bindings.includes(Number(inbound.id ?? 0))
        })
        if (owner) {
          push.warning({
            title: '失败',
            duration: 5000,
            message: `Snell 入站 ${inbound.tag} 只能绑定一个用户，当前已被 ${owner.name} 使用`,
          })
          return false
        }
      }

      return true
    },
    async updateData(id: number) {
      if (id > 0) {
        this.loading = true
        const newData = await getNamespaceStore(this.namespace).loadClients(id)
        this.client = createClient(newData, this.namespace)
        this.title = 'edit'
        this.loading = false
      } else {
        this.client = createClient(undefined, this.namespace)
        this.title = 'add'
      }
      this.initialClientName = this.client.name
      this.clientConfig = this.client.config
      this.links = this.client.links?.filter(l => l.type == 'local') ?? []
      this.extLinks = this.client.links?.filter(l => l.type == 'external') ?? []
      this.subLinks = this.client.links?.filter(l => l.type == 'sub') ?? []
      this.speedLimitInput = this.client.speedLimitMbps > 0 ? String(this.client.speedLimitMbps) : ''
      this.tab = 't1'
      this.loading = false
      this.refreshServerIps()
      this.fetchInboundIps()
    },
    async refreshServerIps() {
      this.loadingIps = true
      try {
        const namespaceApi = getNamespaceApi(this.namespace)
        const [serverIpsMsg, inboundIpsMsg] = await Promise.all([
          HttpUtils.get('api/server-ips?verify=true'),
          HttpUtils.get(namespaceApi.inboundIpsEndpoint),
        ])

        const serverIps: string[] = []
        if (serverIpsMsg.success && Array.isArray(serverIpsMsg.obj)) {
          serverIps.push(...serverIpsMsg.obj)
        }

        if (inboundIpsMsg.success && Array.isArray(inboundIpsMsg.obj)) {
          const ipMap: Record<number, string> = {}
          for (const item of inboundIpsMsg.obj) {
            ipMap[item.id] = item.server
            if (item.server && !serverIps.includes(item.server)) {
              serverIps.push(item.server)
            }
          }
          this.inboundIpMap = ipMap
        }

        this.serverIpList = serverIps
        this.autoSetFirstInboundIp()
      } catch (e) {
        console.error('Failed to fetch server IPs:', e)
      }
      this.loadingIps = false
    },
    async fetchInboundIps() {
      try {
        const msg = await HttpUtils.get(getNamespaceApi(this.namespace).inboundIpsEndpoint)
        if (msg.success && Array.isArray(msg.obj)) {
          const ipMap: Record<number, string> = {}
          for (const item of msg.obj) {
            ipMap[item.id] = item.server
          }
          this.inboundIpMap = ipMap
          this.autoSetFirstInboundIp()
        }
      } catch (e) {
        console.error('Failed to fetch inbound IPs:', e)
      }
    },
    autoSetFirstInboundIp() {
      if (this.client.inbounds.length > 0 && !this.client.serverIp) {
        const firstInboundId = this.client.inbounds[0]
        const firstInboundIp = this.inboundIpMap[firstInboundId]
        if (firstInboundIp) {
          this.client.serverIp = firstInboundIp
        }
      }
    },
    trimServerIp() {
      if (this.client.serverIp) {
        this.client.serverIp = this.client.serverIp.trim()
      }
    },
    trimSpeedLimitInput() {
      this.speedLimitInput = (this.speedLimitInput ?? '').trim()
    },
    normalizeSpeedLimitMbps(raw: string): number | null {
      const input = (raw ?? '').trim()
      if (input === '') {
        return 0
      }

      const matched = input.match(/^(\d+(?:\.\d+)?)\s*(?:m(?:b|bit)?ps)?$/i)
      if (!matched) {
        return null
      }

      const parsed = Number(matched[1])
      if (!Number.isFinite(parsed) || parsed < 0) {
        return null
      }
      if (parsed === 0) {
        return 0
      }
      return Math.max(1, Math.floor(parsed))
    },
    closeModal() {
      this.updateData(0)
      this.$emit('close')
    },
    async saveChanges() {
      if (!this.$props.visible) return

      const store = getNamespaceStore(this.namespace)
      const clientId = this.$props.id ?? 0
      const isDuplicateName = store.checkClientName(clientId, this.client.name)
      if (isDuplicateName) return
      if (!this.validateMihomoSnellInboundBindings(clientId, this.clientInbounds)) return

      this.loading = true
      const normalizedSpeedLimit = this.normalizeSpeedLimitMbps(this.speedLimitInput)
      if (normalizedSpeedLimit == null) {
        push.error({
          message: '\u9650\u901f-mbps \u683c\u5f0f\u65e0\u6548\uff0c\u8bf7\u8f93\u5165 200 \u6216 200mbps',
        })
        this.loading = false
        return
      }
      this.client.speedLimitMbps = normalizedSpeedLimit
      this.client.config = updateConfigs(this.clientConfig, this.client.name, this.initialClientName, this.namespace)
      this.client.links = [
        ...this.extLinks.filter(l => l.uri != ''),
        ...this.subLinks.filter(l => l.uri != ''),
      ]
      const success = await store.save('clients', clientId == 0 ? 'new' : 'edit', this.client)
      if (success) this.closeModal()
      this.loading = false
    },
    requestTrafficReset() {
      this.client.up = 0
      this.client.down = 0
      this.client.trafficResetRequested = true
    },
    setDate(newDate: number) {
      this.client.expiry = newDate
    },
    setAllInbounds() {
      this.client.inbounds = this.normalizeInboundIds((this.inboundTags ?? []).map((i: any) => i.value))
    },
    shuffle(k?: string) {
      shuffleConfigs(this.clientConfig, k, this.namespace)
    },
    showsUsernameField(key: string) {
      return supportsEditableUsernameField(key, this.namespace)
    },
  },
  computed: {
    configKeys(): string[] {
      return getConfigKeys(this.clientConfig, this.namespace)
    },
    resetDayOptions(): { title: string, value: number }[] {
      const options = [{ title: `0 - ${this.$t('none')}`, value: 0 }]
      for (let day = 1; day <= 31; day++) {
        options.push({ title: `${day}`, value: day })
      }
      return options
    },
    clientInbounds: {
      get() {
        return this.normalizeInboundIds(this.client.inbounds)
      },
      set(v: number[]) {
        this.client.inbounds = this.normalizeInboundIds(v)
      },
    },
    expDate: {
      get() {
        return this.client.expiry
      },
      set(v: any) {
        this.client.expiry = v
      },
    },
    Volume: {
      get() {
        return this.client.volume == 0 ? 0 : (this.client.volume / (1024 ** 3))
      },
      set(v: number) {
        this.client.volume = v > 0 ? v * (1024 ** 3) : 0
      },
    },
    ResetDay: {
      get() {
        return this.client.extra == 0 ? 0 : this.client.extra
      },
      set(v: number | null) {
        this.client.extra = this.normalizeResetDayValue(v)
      },
    },
    serverIpValue: {
      get() {
        return this.client.serverIp || ''
      },
      set(v: string | null) {
        this.client.serverIp = v ? v.trim() : ''
      },
    },
    up(): string {
      return HumanReadable.sizeFormat(this.client.up)
    },
    down(): string {
      return HumanReadable.sizeFormat(this.client.down)
    },
    total(): string {
      return HumanReadable.sizeFormat(this.client.down + this.client.up)
    },
    volumeUsage(): string {
      const used = HumanReadable.sizeFormat(this.client.up + this.client.down)
      if (this.client.volume > 0) {
        const volumeGiB = (this.client.volume / (1024 ** 3)).toFixed(0)
        return used + ' / ' + volumeGiB + ' GiB (' + this.percent + '%)'
      }
      return used + ' / -'
    },
    percent(): number {
      return this.client.volume > 0 ? Math.round((this.client.up + this.client.down) * 100 / this.client.volume) : 0
    },
    percentColor(): string {
      return (this.client.up + this.client.down) >= this.client.volume ? 'error' : this.percent > 90 ? 'warning' : 'success'
    },
  },
  watch: {
    visible(newValue) {
      if (newValue) {
        this.updateData(this.$props.id ?? 0)
      }
    },
    clientInbounds: {
      handler(newInbounds: number[], oldInbounds?: number[]) {
        if (newInbounds.length > 0) {
          const firstInboundId = newInbounds[0]
          const firstInboundIp = this.inboundIpMap[firstInboundId]
          if (firstInboundIp) {
            const oldFirstIp = oldInbounds && oldInbounds.length > 0 ? this.inboundIpMap[oldInbounds[0]] : null
            if (!this.client.serverIp || this.client.serverIp === oldFirstIp) {
              this.client.serverIp = firstInboundIp
            }
          }
        }
      },
      deep: true,
    },
  },
  components: { DatePick },
}
</script>

<style scoped>
.rotating {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}
</style>
