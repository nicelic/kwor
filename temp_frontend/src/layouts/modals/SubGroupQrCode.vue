<template>
  <v-dialog transition="dialog-bottom-transition" width="400">
    <v-card class="rounded-lg" id="subgroup-qrcode-modal" :loading="loading">
      <v-card-title>
        <v-row>
          <v-col>{{ groupName }} - QrCode</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto"><v-icon icon="mdi-close-box" @click="$emit('close')" /></v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="overflow-y: auto; padding: 0" v-if="groupName">
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
    }
  },
  methods: {
    copyToClipboard(txt: string) {
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
    }
  },
  computed: {
    subJsonUrl() {
      return Data().subURI + "q/group?name=" + encodeURIComponent(this.groupName) + "&format=json"
    },
    subClashUrl() {
      return Data().subURI + "q/group?name=" + encodeURIComponent(this.groupName) + "&format=clash"
    },
    singboxUrl() {
      return "sing-box://import-remote-profile?url=" + encodeURIComponent(this.subJsonUrl) + "#" + encodeURIComponent(this.groupName)
    },
    size() {
      if (window.innerWidth > 380) return 300
      if (window.innerWidth > 330) return 280
      return 250
    }
  },
  components: { QrcodeVue }
}
</script>
