<template>
  <v-dialog transition="dialog-bottom-transition" width="400">
    <v-card class="rounded-lg" id="qrcode-modal" :loading="loading">
      <v-card-title>
        <v-row>
          <v-col>QrCode</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto"><v-icon icon="mdi-close-box" @click="$emit('close')" /></v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-skeleton-loader
        class="mx-auto border"
        width="80%"
        type="text, image, divider, text, image"
        v-if="loading">
      </v-skeleton-loader>
      <v-card-text style="overflow-y: auto; padding: 0" :hidden="loading">
        <v-tabs
          v-if="showSubscriptionQr"
          v-model="tab"
          density="compact"
          fixed-tabs
          align-tabs="center">
          <v-tab value="sub">{{ $t('setting.sub') }}</v-tab>
          <v-tab value="link">{{ $t('client.links') }}</v-tab>
        </v-tabs>
        <v-window v-model="tab" style="margin-top: 10px;">
          <v-window-item value="sub" v-if="showSubscriptionQr">
            <v-row>
              <v-col style="text-align: center;">
                <v-chip>{{ $t('setting.sub') }}</v-chip><br />
                <QrcodeVue :value="clientSub" :size="size" @click="copyToClipboard(clientSub)" :margin="1" style="border-radius: 1rem; cursor: copy;" />
              </v-col>
            </v-row>
            <v-row>
              <v-col style="text-align: center;">
                <v-chip>{{ $t('setting.jsonSub') }}</v-chip><br />
                <QrcodeVue :value="clientJsonSub" :size="size" @click="copyToClipboard(clientJsonSub)" :margin="1" style="border-radius: 1rem; cursor: copy;" />
              </v-col>
            </v-row>
            <v-row>
              <v-col style="text-align: center;">
                <v-chip>{{ $t('setting.clashSub') }}</v-chip><br />
                <QrcodeVue :value="clientClashSub" :size="size" @click="copyToClipboard(clientClashSub)" :margin="1" style="border-radius: 1rem; cursor: copy;" />
              </v-col>
            </v-row>
            <v-row>
              <v-col style="text-align: center;">
                <v-chip>SING-BOX (scan only)</v-chip><br />
                <QrcodeVue :value="singbox" :size="size" :margin="1" style="border-radius: .8rem; cursor: not-allowed;" />
              </v-col>
            </v-row>
          </v-window-item>
          <v-window-item value="link">
            <v-row v-for="l in clientLinks">
              <v-col style="text-align: center;">
                <v-chip>{{ l.remark ?? $t('client.' + l.type) }}</v-chip><br />
                <QrcodeVue :value="l.uri" :size="size" @click="copyToClipboard(l.uri)" :margin="1" style="border-radius: .5rem; cursor: copy;" />
              </v-col>
            </v-row>
          </v-window-item>
        </v-window>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import QrcodeVue from 'qrcode.vue'
import Clipboard from 'clipboard'
import { i18n } from '@/locales'
import { push } from 'notivue'
import { getNamespaceApi, getNamespaceStore } from '@/store/uiNamespace'

export default {
  props: {
    id: Number,
    visible: Boolean,
    namespace: {
      type: String,
      default: 'default',
    },
  },
  data() {
    return {
      tab: 'sub',
      client: <any>{},
      loading: false,
    }
  },
  methods: {
    async load() {
      this.loading = true
      const newData = await getNamespaceStore(this.namespace).loadClients(this.$props.id ?? 0)
      this.client = newData
      this.loading = false
    },
    copyToClipboard(txt: string) {
      const hiddenButton = document.createElement('button')
      hiddenButton.className = 'clipboard-btn'
      document.body.appendChild(hiddenButton)

      const clipboard = new Clipboard('.clipboard-btn', {
        text: () => txt,
        container: document.getElementById('qrcode-modal') ?? undefined,
      })

      clipboard.on('success', () => {
        clipboard.destroy()
        push.success({
          message: i18n.global.t('success') + ': ' + i18n.global.t('copyToClipboard'),
          duration: 5000,
        })
      })

      clipboard.on('error', () => {
        clipboard.destroy()
        push.error({
          message: i18n.global.t('failed') + ': ' + i18n.global.t('copyToClipboard'),
          duration: 5000,
        })
      })

      hiddenButton.click()
      document.body.removeChild(hiddenButton)
    },
    buildSubscriptionUrl(format?: string) {
      const baseURI = getNamespaceStore(this.namespace).subURI
      const name = encodeURIComponent(String(this.client?.name ?? ''))
      const query = format ? '&format=' + encodeURIComponent(format) : ''
      if (this.namespace === 'mihomo') {
        return baseURI + 'q/mihomo?name=' + name + query
      }
      return baseURI + 'q/client?name=' + name + query
    },
  },
  computed: {
    showSubscriptionQr(): boolean {
      return getNamespaceApi(this.namespace).supportsSubscriptionQr
    },
    clientSub() {
      return this.buildSubscriptionUrl()
    },
    clientJsonSub() {
      return this.buildSubscriptionUrl('json')
    },
    clientClashSub() {
      return this.buildSubscriptionUrl('clash')
    },
    singbox() {
      return 'sing-box://import-remote-profile?url=' + encodeURIComponent(this.clientJsonSub) + '#' + encodeURIComponent(String(this.client?.name ?? ''))
    },
    clientLinks() {
      return this.client.links ?? []
    },
    size() {
      if (window.innerWidth > 380) return 300
      if (window.innerWidth > 330) return 280
      return 250
    },
  },
  watch: {
    visible(v) {
      if (v) {
        this.tab = this.showSubscriptionQr ? 'sub' : 'link'
        this.load()
      }
    },
  },
  components: { QrcodeVue },
}
</script>
