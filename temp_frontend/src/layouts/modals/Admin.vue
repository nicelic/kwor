<template>
  <v-dialog
    :model-value="visible"
    transition="dialog-bottom-transition"
    width="400"
    max-width="calc(100vw - 24px)"
    @update:model-value="updateDialog"
  >
    <v-card class="rounded-lg">
      <v-card-title>
        {{ $t('admin.changeCred') + " " + user.username }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text>
        <v-row>
          <v-col>
            <v-text-field v-model="newData.oldPass" :label="$t('admin.oldPass')" :rules="passwordRules" type="password" required></v-text-field>
          </v-col>
        </v-row>
        <v-row>
          <v-col>
            <v-text-field v-model="newData.newUsername" :label="$t('admin.newUname')" :rules="usernameRules" required></v-text-field>
          </v-col>
        </v-row>
        <v-row>
          <v-col>
            <v-text-field v-model="newData.newPass" :label="$t('admin.newPass')" :rules="passwordRules" type="password" required></v-text-field>
          </v-col>
        </v-row>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          color="primary"
          variant="outlined"
          @click="closeModal"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          @click="saveChanges"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { i18n } from '@/locales'

export default {
  props: ['visible', 'user', 'modelValue'],
  data() {
    return {
      newData: {
        id: 0,
        oldPass: "",
        newUsername: "",
        newPass: ""
      },
      usernameRules: [
        (value: string) => {
          if (value?.length > 0) return true
          return i18n.global.t('login.unRules')
        },
      ],
      passwordRules: [
        (value: string) => {
          if (value?.length > 0) return true
          return i18n.global.t('login.pwRules')
        },
      ]
    }
  },
  methods: {
    updateDialog(value: boolean) {
      this.$emit('update:modelValue', value)
      if (!value) this.$emit('close')
    },
    resetData() {
      this.newData.id = this.$props.user.id
      this.newData.oldPass = ""
      this.newData.newUsername = ""
      this.newData.newPass = ""
    },
    closeModal() {
      this.resetData() // reset
      this.$emit('close')
    },
    saveChanges() {
      if (this.newData.oldPass == '' || this.newData.newUsername == '' || this.newData.newPass == '') return
      this.$emit('save', this.newData)
    },
  },
  watch: {
    visible(newValue) {
      if (newValue) {
        this.resetData()
      }
    },
  },
}
</script>
