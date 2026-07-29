<template>
  <v-dialog transition="dialog-bottom-transition" width="calc(100vw - 32px)" max-width="400">
    <v-card class="rounded-lg" id="subgroup-qrcode-modal" :loading="loading || subscriptionUriLoading">
      <v-card-title>
        <v-row>
          <v-col class="text-truncate">{{ groupName }} - QrCode</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto"><v-icon icon="mdi-close-box" @click="$emit('close')" /></v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="overflow-y: auto; padding: 0" v-if="groupName">
        <template v-if="subscriptionUriReady">
          <v-row>
            <v-col style="text-align: center;">
              <v-chip>{{ $t('setting.sub') }} (JSON)</v-chip><br />
              <QrcodeVue :value="subJsonUrl" :size="size" @click="copyToClipboard(subJsonUrl)" :margin="1" style="border-radius: 1rem; cursor: copy;" />
            </v-col>
          </v-row>
          <v-row>
            <v-col style="text-align: center;">
              <v-chip>{{ $t('setting.clashSub') }}</v-chip><br />
              <QrcodeVue :value="subClashUrl" :size="size" @click="copyToClipboard(subClashUrl)" :margin="1" style="border-radius: 1rem; cursor: copy;" />
            </v-col>
          </v-row>
          <v-row>
            <v-col style="text-align: center;">
              <v-chip>SING-BOX (scan only)</v-chip><br />
              <QrcodeVue :value="singboxUrl" :size="size" :margin="1" style="border-radius: .8rem; cursor: not-allowed;" />
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
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import QrcodeVue from 'qrcode.vue'
import Data from '@/store/modules/data'
import Clipboard from 'clipboard'
import { i18n } from '@/locales'
import { push } from 'notivue'

export default {
  props: ['groupName', 'visible'],
  data() {
    return {
      loading: false,
      subscriptionUriReady: false,
      subscriptionUriLoading: false,
      subscriptionUriError: '',
      subscriptionUriRequestSequence: 0,
    }
  },
  methods: {
    copyToClipboard(txt: string) {
      if (!txt) return
      const hiddenButton = document.createElement('button')
      hiddenButton.className = 'subgroup-clipboard-btn'
      document.body.appendChild(hiddenButton)

      const clipboard = new Clipboard('.subgroup-clipboard-btn', {
        text: () => txt,
        container: document.getElementById('subgroup-qrcode-modal') ?? undefined
      })

      clipboard.on('success', () => {
        clipboard.destroy()
        push.success({
          message: i18n.global.t('success') + ": " + i18n.global.t('copyToClipboard'),
          duration: 5000,
        })
      })

      clipboard.on('error', () => {
        clipboard.destroy()
        push.error({
          message: i18n.global.t('failed') + ": " + i18n.global.t('copyToClipboard'),
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
        const store = Data()
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
    }
  },
  computed: {
    subJsonUrl() {
      if (!this.subscriptionUriReady) return ''
      return Data().subURI + "q/group?name=" + encodeURIComponent(this.groupName) + "&format=json"
    },
    subClashUrl() {
      if (!this.subscriptionUriReady) return ''
      return Data().subURI + "q/group?name=" + encodeURIComponent(this.groupName) + "&format=clash"
    },
    singboxUrl() {
      if (!this.subJsonUrl) return ''
      return "sing-box://import-remote-profile?url=" + encodeURIComponent(this.subJsonUrl) + "#" + encodeURIComponent(this.groupName)
    },
    size() {
      return Math.max(120, Math.min(300, window.innerWidth - 72))
    }
  },
  watch: {
    visible(v) {
      if (v) {
        void this.refreshSubscriptionUri()
      } else {
        this.subscriptionUriRequestSequence += 1
        this.subscriptionUriReady = false
        this.subscriptionUriLoading = false
      }
    }
  },
  components: { QrcodeVue }
}
</script>
