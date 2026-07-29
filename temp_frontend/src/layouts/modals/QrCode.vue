<template>
  <v-dialog transition="dialog-bottom-transition" width="calc(100vw - 32px)" max-width="400">
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
            <template v-if="subscriptionUriReady">
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
            </template>
            <v-alert v-else class="ma-3" type="error" variant="tonal">
              <div v-if="subscriptionUriLoading" class="d-flex align-center flex-wrap" style="gap: 10px;">
                <v-progress-circular indeterminate size="20" width="2" />
                <span>正在验证订阅地址…</span>
              </div>
              <template v-else>
                <div class="text-body-2">无法生成订阅二维码：{{ subscriptionUriError || '订阅地址不可用。' }}</div>
                <div class="d-flex flex-wrap mt-3" style="gap: 8px;">
                  <v-btn color="primary" size="small" variant="outlined" @click="refreshSubscriptionUri">
                    重新加载订阅地址
                  </v-btn>
                </div>
              </template>
            </v-alert>
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
      subscriptionUriReady: false,
      subscriptionUriLoading: false,
      subscriptionUriError: '',
      subscriptionUriRequestSequence: 0,
      clientLoadRequestSequence: 0,
    }
  },
  methods: {
    async load() {
      const requestSequence = ++this.clientLoadRequestSequence
      this.loading = true
      try {
        const newData = await getNamespaceStore(this.namespace).loadClients(this.$props.id ?? 0)
        if (requestSequence !== this.clientLoadRequestSequence || !this.$props.visible) return
        this.client = newData
      } finally {
        if (requestSequence === this.clientLoadRequestSequence) {
          this.loading = false
        }
      }
    },
    copyToClipboard(txt: string) {
      if (!txt) return
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
    async refreshSubscriptionUri() {
      const requestSequence = ++this.subscriptionUriRequestSequence
      this.subscriptionUriReady = false
      this.subscriptionUriLoading = true
      this.subscriptionUriError = ''
      try {
        const store = getNamespaceStore(this.namespace)
        const success = await store.refreshSubscriptionURI()
        if (requestSequence !== this.subscriptionUriRequestSequence || !this.$props.visible) return
        if (success) {
          this.subscriptionUriReady = true
          return
        }
        this.subscriptionUriError = store.subscriptionUriError || '订阅地址不可用。'
      } finally {
        if (requestSequence === this.subscriptionUriRequestSequence) {
          this.subscriptionUriLoading = false
        }
      }
    },
    buildSubscriptionUrl(format?: string) {
      if (!this.subscriptionUriReady) return ''
      const baseURI = getNamespaceStore(this.namespace).subURI
      if (!baseURI) return ''
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
      if (!this.clientJsonSub) return ''
      return 'sing-box://import-remote-profile?url=' + encodeURIComponent(this.clientJsonSub) + '#' + encodeURIComponent(String(this.client?.name ?? ''))
    },
    clientLinks() {
      return this.client.links ?? []
    },
    size() {
      return Math.max(120, Math.min(300, window.innerWidth - 72))
    },
  },
  watch: {
    visible(v) {
      if (v) {
        this.tab = this.showSubscriptionQr ? 'sub' : 'link'
        void this.load()
        if (this.showSubscriptionQr) {
          void this.refreshSubscriptionUri()
        }
      } else {
        this.clientLoadRequestSequence += 1
        this.subscriptionUriRequestSequence += 1
        this.client = {}
        this.loading = false
        this.subscriptionUriReady = false
        this.subscriptionUriLoading = false
      }
    },
  },
  components: { QrcodeVue },
}
</script>
