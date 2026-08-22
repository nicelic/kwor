<template>
  <v-dialog v-model="dialogVisible" transition="dialog-bottom-transition" width="800" max-width="95vw" max-height="90vh" :persistent="busy || loading" @update:model-value="handleDialogVisibility">
    <v-card class="rounded-lg">
      <v-card-title>
        {{ $t('actions.' + title) + " " + $t('objects.dnsserver') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="max-height: 75vh; overflow-y: auto;">
        <div :style="{ pointerEvents: busy || loading ? 'none' : 'auto' }" :aria-busy="busy || loading">
        <v-row>
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="dnsServer.type"
              :disabled="busy || loading"
              :items="dnsTypes"
              :label="$t('type')"
              @update:modelValue="changeType"
              hide-details
            />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field v-model="dnsServer.tag" :label="$t('objects.tag')" hide-details :disabled="busy || loading" />
          </v-col>
        </v-row>
        <v-row v-if="HasServer.includes(dnsServer.type)">
          <v-col cols="12" sm="6" md="4">
            <v-text-field v-model="dnsServer.server" :label="$t('in.addr')" hide-details :disabled="busy || loading" />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field v-model.number="dnsServer.server_port" type="number" min="1" max="65535" :label="$t('in.port')" hide-details :disabled="busy || loading" />
          </v-col>
        </v-row>
        <v-row v-if="HasHeaders.includes(dnsServer.type)">
          <v-col cols="12" sm="8">
            <v-text-field v-model="dnsServer.path" :label="$t('transport.path')" hide-details :disabled="busy || loading" />
          </v-col>
        </v-row>
        <DialVue :dial="dnsServer" :namespace="'default'" :candidateTags="dialTags" :disabled="busy || loading" v-if="!WithoutDial.includes(dnsServer.type)" />
        <oTlsVue
          :outbound="dnsServer"
          :tlsRequired="true"
          v-if="HasTls.includes(dnsServer.type)"
        />
        <Headers :data="dnsServer" :disabled="busy || loading" v-if="HasHeaders.includes(dnsServer.type)" />
        <template v-if="dnsServer.type == 'hosts'">
          <v-row>
            <v-col cols="12" sm="6">
              <v-text-field v-model="hostsPath" :label="$t('transport.path') + $t('commaSeparated')" hide-details :disabled="busy || loading" />
            </v-col>
          </v-row>
          <v-card>
            <v-card-subtitle>Predefined
              <v-chip color="primary" density="compact" variant="elevated" :disabled="busy || loading" @click="addHostsPredefined"><v-icon icon="mdi-plus" /></v-chip>
            </v-card-subtitle>
            <v-row v-for="(pd, index) in hostsPredefined" :key="`dns-host-${index}`">
              <v-col cols="12" sm="6" md="4">
                  <v-text-field v-model="pd.name" :label="$t('setting.domain')" @input="update_pds_key(index,$event.target.value)" hide-details :disabled="busy || loading"></v-text-field>
              </v-col>
              <v-col cols="12" sm="6">
                <v-text-field
                  v-model="pd.value"
                  :label="$t('types.tun.addr') + $t('commaSeparated')"
                  @input="update_pds_value(index,$event.target.value)"
                  hide-details :disabled="busy || loading">
                  <template v-slot:append>
                  <v-icon @click="delHostsPredefined(index)" color="error" icon="mdi-delete" :disabled="busy || loading" />
                  </template>
                </v-text-field>
              </v-col>
            </v-row>
          </v-card>
        </template>
        <v-row v-if="dnsServer.type == 'dhcp'">
          <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="dnsServer.interface" :label="$t('types.tun.ifName')" hide-details :disabled="busy || loading" />
          </v-col>
        </v-row>
        <v-row v-if="dnsServer.type == 'fakeip'">
          <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="dnsServer.inet4_range" :label="$t('dns.rule.inet4Range')" hide-details :disabled="busy || loading" />
          </v-col>
          <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="dnsServer.inet6_range" :label="$t('dns.rule.inet6Range')" hide-details :disabled="busy || loading" />
          </v-col>
        </v-row>
        <v-row v-if="dnsServer.type == 'tailscale' || dnsServer.type == 'resolved'">
          <v-col cols="12" sm="6" md="4" v-if="dnsServer.type == 'tailscale'">
            <v-select v-model="dnsServer.endpoint" :label="$t('objects.endpoint')" :items="tsTags" hide-details :disabled="busy || loading" />
          </v-col>
          <v-col cols="12" sm="6" md="4" v-if="dnsServer.type == 'resolved'">
            <v-select v-model="dnsServer.service" :label="$t('objects.service')" :items="rslvdTags" hide-details :disabled="busy || loading" />
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-switch v-model="dnsServer.accept_default_resolvers" :label="$t('dns.rule.acceptDefault')" hide-details :disabled="busy || loading"></v-switch>
          </v-col>
        </v-row>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="blue-darken-1" variant="outlined" @click="close" :disabled="busy || loading">{{ $t('actions.close') }}</v-btn>
        <v-btn color="blue-darken-1" variant="tonal" @click="save" :loading="loading" :disabled="busy || loading">{{ $t('actions.save') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import DialVue from '@/components/Dial.vue' 
import oTlsVue from '@/components/tls/OutTLS.vue'
import Headers from '@/components/Headers.vue'
import RandomUtil from '@/plugins/randomUtil'
import { DnsTypes, createDnsServer, normalizeDnsServerHttpPath } from '@/types/dns'
export default {
  props: ['visible', 'data', 'index', 'tsTags', 'rslvdTags', 'dialTags', 'busy'],
  emits: ['close', 'save'],
  data() {
    return {
      title: "add",
      dialogVisible: false,
      dnsServer: createDnsServer("local",{tag: "dns-" + RandomUtil.randomSeq(3)}),
      loading: false,
      dnsTypes: Object.keys(DnsTypes).map((key,index) => ({title: key, value: Object.values(DnsTypes)[index]})),
      HasServer: [DnsTypes.TCP, DnsTypes.UDP, DnsTypes.TLS, DnsTypes.QUIC, DnsTypes.HTTPS, DnsTypes.HTTP3],
      HasHeaders: [DnsTypes.HTTPS, DnsTypes.HTTP3],
      HasTls: [DnsTypes.TLS, DnsTypes.QUIC, DnsTypes.HTTPS, DnsTypes.HTTP3],
      WithoutDial: [DnsTypes.Hosts, DnsTypes.Tailscale, DnsTypes.FakeIP, DnsTypes.Resolved],
    }
  },
  methods: {
    updateData() {
      if (this.$props.index != -1) {
        try {
          this.dnsServer = normalizeDnsServerHttpPath(JSON.parse(this.$props.data))
        } catch {
          this.dnsServer = createDnsServer("local",{tag: "dns-" + RandomUtil.randomSeq(3)})
        }
        this.title = 'edit'
      }
      else {
        this.dnsServer = createDnsServer("local",{tag: "dns-" + RandomUtil.randomSeq(3)})
        this.title = 'add'
      }
    },
    changeType(dnsType: string) {
      this.dnsServer = createDnsServer(dnsType,{tag: this.dnsServer.tag})
    },
    close() {
      this.$emit('close')
    },
    handleDialogVisibility(value: boolean) {
      if (!value && this.$props.visible && !this.busy && !this.loading) this.close()
    },
    save() {
      if (this.busy || this.loading) return
      this.dnsServer = normalizeDnsServerHttpPath(this.dnsServer)
      this.$emit('save', this.dnsServer)
    },
    addHostsPredefined() {
      if (this.busy || this.loading) return
      const newPredefined = { name:'localhost', value: '127.0.0.1,::1' }
      this.hostsPredefined = [...this.hostsPredefined, newPredefined]
    },
    delHostsPredefined(i:number) {
      if (this.busy || this.loading) return
      let pds = this.hostsPredefined
      pds.splice(i,1)
      this.hostsPredefined = pds
    },
    update_pds_key(i:number,k:string) {
      if (this.busy || this.loading) return
      let pds = this.hostsPredefined
      pds[i].name = k
      this.hostsPredefined = pds
    },
    update_pds_value(i:number,v:string) {
      if (this.busy || this.loading) return
      let pds = this.hostsPredefined
      pds[i].value = v
      this.hostsPredefined = pds
    },
  },
  computed:{
    hostsPath: {
      get() {
        const path = this.dnsServer.path
        return Array.isArray(path) ? path.join(',') : typeof path === 'string' ? path : ''
      },
      set(v: string) {
        const value = String(v ?? '').trim()
        this.dnsServer.path = value.length > 0 ? value.split(',').map((item: string) => item.trim()).filter(Boolean) : undefined
      }
    },
    hostsPredefined: {
      get() :any[] {
        let pds :any[] = []
        const h = this.dnsServer.predefined
        if (h) {
          Object.keys(h).forEach(key => {
            if (Array.isArray(h[key])){
              pds.push({ name: key, value: h[key].join(',') })
            } else {
              pds.push({ name: key, value: String(h[key] ?? '') })
            }
          })
        }
        return pds
       },
      set(v: any[]) {
        if (v.length>0) {
          let pds:any = {}
          v.forEach((pd:any) => {
            const name = String(pd.name ?? '').trim()
            if (!name) return
            const value = String(pd.value ?? '').trim()
            pds[name] = value.split(',').map((item: string) => item.trim()).filter(Boolean)
          })
          this.dnsServer.predefined = pds
        } else {
          this.dnsServer.predefined = undefined
        }
      }
    },
  },
  watch: {
    visible(v) {
      if (v) {
        this.updateData()
        this.dialogVisible = true
      } else {
        this.dialogVisible = false
      }
    },
  },
  components: { DialVue, oTlsVue, Headers }
}
</script>
