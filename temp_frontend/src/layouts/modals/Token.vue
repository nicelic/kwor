<template>
  <v-dialog
    :model-value="visible"
    transition="dialog-bottom-transition"
    width="800"
    max-width="calc(100vw - 24px)"
    max-height="calc(100vh - 24px)"
    scrollable
    @update:model-value="updateDialog"
  >
    <v-card class="rounded-lg" :loading="loading">
      <v-card-title>
        <v-row>
          <v-col>{{ $t('admin.api.title') }}</v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto">
            <v-btn icon="mdi-close" variant="text" :aria-label="$t('actions.close')" @click="$emit('close')">
              <v-tooltip activator="parent" location="top" :text="$t('actions.close')" />
            </v-btn>
          </v-col>
        </v-row>
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text>
        <v-alert
          v-if="newToken.token.length>0"
          color="success"
          density="compact"
          icon="mdi-alert-circle-outline"
        >
          {{ $t('admin.api.msg') }}
          <v-text-field
            readonly
            variant="outlined"
            bg-color="warning"
            append-inner-icon="mdi-content-copy"
            @click:append-inner="copyToClipboard(newToken.token)"
            v-model="newToken.token"
          ></v-text-field>
        </v-alert>
        <div class="token-table-wrap">
        <v-table density="compact" class="token-table">
          <thead>
            <tr>
              <th>#</th>
              <th>{{ $t('admin.api.token') }}</th>
              <th>{{ $t('client.desc') }}</th>
              <th>{{ $t('date.expiry') }}</th>
              <th>{{ $t('actions.del') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(token, index) of tokens" :key="token.id">
              <td>{{ token.id }}</td>
              <td>{{ token.token }}</td>
              <td>{{ token.desc }}</td>
              <td>{{ dateFormatted(token.expiry) }}</td>
              <td>
                <v-menu
                  v-model="delOverlay[index]"
                  :close-on-content-click="false"
                  location="top center"
                >
                  <template v-slot:activator="{ props }">
                    <v-btn
                      icon="mdi-delete"
                      size="small"
                      color="error"
                      variant="text"
                      v-bind="props"
                      :aria-label="$t('actions.del')"
                    >
                      <v-tooltip activator="parent" location="top" :text="$t('actions.del')" />
                    </v-btn>
                  </template>
                  <v-card :title="$t('actions.del')" rounded="lg">
                    <v-divider></v-divider>
                    <v-card-text>{{ $t('confirm') }}</v-card-text>
                    <v-card-actions>
                      <v-btn color="error" variant="outlined" @click="deleteToken(token.id)">{{ $t('yes') }}</v-btn>
                      <v-btn color="success" variant="outlined" @click="delOverlay[index] = false">{{ $t('no') }}</v-btn>
                    </v-card-actions>
                  </v-card>
                </v-menu>
              </td>
            </tr>
          </tbody>
        </v-table>
        </div>
        <v-btn color="primary" @click="showAddToken()">
          {{ $t('actions.add') }}
        </v-btn>
        <v-dialog v-model="showNewToken" width="300" max-width="calc(100vw - 24px)">
          <v-card class="rounded-lg">
            <v-card-title>
              <v-row>
                <v-col>{{ $t('admin.api.token') }}</v-col>
                <v-spacer></v-spacer>
                <v-col cols="auto">
                  <v-btn icon="mdi-close" variant="text" :aria-label="$t('actions.close')" @click="showNewToken = false">
                    <v-tooltip activator="parent" location="top" :text="$t('actions.close')" />
                  </v-btn>
                </v-col>
              </v-row>
            </v-card-title>
            <v-divider></v-divider>
            <v-card-text>
              <v-row>
                <v-col>
                  <v-text-field :label="$t('client.desc')" v-model="newToken.desc"></v-text-field>
                </v-col>
              </v-row>
              <v-row>
                <v-col>
                  <v-text-field
                    :label="$t('date.expiry')"
                    v-model.number="newToken.expiry"
                    min="0"
                    step="1"
                    type="number"
                    :suffix="$t('date.d')"
                  ></v-text-field>
                </v-col>
              </v-row>
            </v-card-text>
            <v-card-actions>
              <v-spacer></v-spacer>
              <v-btn
                color="blue-darken-1"
                variant="outlined"
                @click="showNewToken = false"
              >
                {{ $t('actions.close') }}
              </v-btn>
              <v-btn
                color="blue-darken-1"
                variant="tonal"
                @click="addToken"
              >
                {{ $t('actions.add') }}
              </v-btn>
            </v-card-actions>
          </v-card>
        </v-dialog>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          color="blue-darken-1"
          variant="outlined"
          @click="closeModal"
        >
          {{ $t('actions.close') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { i18n } from '@/locales'
import HttpUtils from '@/plugins/httputil'
import { formatPanelDate } from '@/plugins/panelTime'
import Clipboard from 'clipboard'
import { push } from 'notivue';

export default {
  props: ['visible', 'user', 'modelValue'],
  data() {
    return {
      loading: false,
      tokens: <any[]>[],
      showNewToken: false,
      newToken: {
        desc: '',
        token: '',
        expiry: 0,
      },
      delOverlay: new Array<boolean>(0),
    }
  },
  computed: {
    locale() {
      const l = i18n.global.locale.value
      switch (l) {
        case "zhHans":
          return "zh-cn"
        case "zhHant":
          return "zh-tw"
        default:
          return l
      }
    },
  },
  methods: {
    updateDialog(value: boolean) {
      this.$emit('update:modelValue', value)
      if (!value) this.$emit('close')
    },
    async loadData() {
      this.loading = true
      try {
        const data = await HttpUtils.get('api/tokens')
        if (data.success) {
          this.tokens = data.obj ?? []
          this.delOverlay = new Array<boolean>(this.tokens.length).fill(false)
        }
      } finally {
        this.loading = false
      }
    },
    resetNewToken() {
      this.newToken={
          desc: '',
          token: '',
          expiry: 30,
        }
    },
    showAddToken() {
      this.resetNewToken()
      this.showNewToken = true
    },
    async addToken() {
      this.loading = true
      try {
        this.newToken.expiry = this.newToken.expiry>0 ? this.newToken.expiry : 0
        const response = await HttpUtils.post('api/addToken', { desc: this.newToken.desc, expiry: this.newToken.expiry })
        if (response.success) {
          this.newToken.token = response.obj
          await this.loadData()
          this.showNewToken = false
        }
      } finally {
        this.loading = false
      }
    },
    async deleteToken(id: number) {
      this.loading = true
      try {
        const response = await HttpUtils.post('api/deleteToken', { id: id })
        if (response.success) {
          await this.loadData()
        }
      } finally {
        this.loading = false
      }
    },
    dateFormatted(expiry: number) {
      if (expiry == 0) return i18n.global.t('unlimited')
      return formatPanelDate(expiry * 1000, this.locale, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
      })
    },
    copyToClipboard(txt:string) {
      const hiddenButton = document.createElement('button')
      hiddenButton.className = 'clipboard-btn'
      document.body.appendChild(hiddenButton)

      const clipboard = new Clipboard('.clipboard-btn', {
        text: () => txt,
        container: document.getElementById('qrcode-modal')?? undefined
      });

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

      // Perform click on hidden button to trigger copy
      hiddenButton.click()
      document.body.removeChild(hiddenButton)
    },
    closeModal() {
      this.$emit('close')
    },
  },
  watch: {
    visible(v) {
      if (v) {
        this.resetNewToken()
        this.loadData()
      }
    },
  },
}
</script>

<style scoped>
.token-table-wrap {
  max-width: 100%;
  overflow-x: auto;
}

.token-table {
  min-width: 620px;
}
</style>
