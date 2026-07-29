<template>
  <AdminModal 
    v-model="editModal.visible"
    :visible="editModal.visible"
    :user="editModal.user"
    @close="closeEditModal"
    @save="saveEditModal"
  />
  <ChangeModal 
    v-model="changesModal.visible"
    :visible="changesModal.visible"
    :admins="users.map((u:any) => u.username)"
    :actor="changesModal.actor"
    @close="closeChangesModal"
  />
  <TokenModal 
    v-model="tokenModal.visible"
    :visible="tokenModal.visible"
    @close="closeTokenModal"
  />
  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn color="primary" @click="showChangesModal('')" style="margin: 0 5px;">{{ $t('admin.changes') }}</v-btn>
      <v-btn color="primary" @click="showTokenModal()">{{ $t('admin.api.token') }}</v-btn>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3" lg="2" v-for="(item, index) in <any[]>users" :key="item.id">
      <v-card rounded="xl" elevation="5" min-width="200" :title="item.username">
        <v-card-subtitle style="margin-top: -20px;">
          {{ $t('admin.lastLogin') }}
        </v-card-subtitle>
        <v-card-text>
          <v-row>
            <v-col>{{ $t('admin.date') }}</v-col>
            <v-col>
              {{ item.loginDate }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('admin.time') }}</v-col>
            <v-col>
              {{ item.loginTime }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>IP</v-col>
            <v-col>
              {{ item.ip }}
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-account-edit" @click="showEditModal(item)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-list-box-outline" @click="showChangesModal(item.username)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('admin.changes')"></v-tooltip>
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import AdminModal from '@/layouts/modals/Admin.vue'
import ChangeModal  from '@/layouts/modals/Changes.vue'
import TokenModal from '@/layouts/modals/Token.vue'
import { i18n } from '@/locales'
import HttpUtils from '@/plugins/httputil'
import {
  formatPanelDateTime,
  formatPanelTime,
  panelCalendarPartsToInstant,
} from '@/plugins/panelTime'
import { Ref, ref, inject, onMounted } from 'vue'

const loading:Ref = inject('loading')?? ref(false)

const users = ref(<any[]>[])

type LoginDisplay = {
  loginDate: string
  loginTime: string
  ip: string
}

const legacyLoginTimestampPattern = /^(\d{4})-(\d{1,2})-(\d{1,2})\s+(\d{1,2}):(\d{1,2}):(\d{1,2})$/

onMounted(() => { void loadData() })

const loadData = async () => {
  loading.value = true
  try {
    const msg = await HttpUtils.get('api/users')
    if (msg.success) {
      users.value = msg.obj.map((u:any) => {
        const login = parseLoginRecord(u.lastLogin)
        return {
          id: u.id,
          username: u.username,
          loginDate: login.loginDate,
          loginTime: login.loginTime,
          ip: login.ip,
        }
      })
    }
  } finally {
    loading.value = false
  }
}

const displayLocale = () => {
  return i18n.global.locale.value.replace('zh', 'zh-')
}

const parseLoginTimestamp = (raw: unknown): number | null => {
  const value = String(raw ?? '').trim()
  const legacy = value.match(legacyLoginTimestampPattern)
  if (legacy != null) {
    const year = Number(legacy[1])
    const month = Number(legacy[2])
    const day = Number(legacy[3])
    const hour = Number(legacy[4])
    const minute = Number(legacy[5])
    const second = Number(legacy[6])
    const validator = new Date(Date.UTC(year, month - 1, day, hour, minute, second))
    if (
      validator.getUTCFullYear() !== year ||
      validator.getUTCMonth() !== month - 1 ||
      validator.getUTCDate() !== day ||
      validator.getUTCHours() !== hour ||
      validator.getUTCMinutes() !== minute ||
      validator.getUTCSeconds() !== second
    ) {
      return null
    }
    return panelCalendarPartsToInstant(year, month, day, hour, minute, second).getTime()
  }

  const timestamp = Date.parse(value)
  return Number.isFinite(timestamp) ? timestamp : null
}

const parseLoginRecord = (raw: unknown): LoginDisplay => {
  const value = String(raw ?? '').trim()
  const separator = value.lastIndexOf(' ')
  if (separator <= 0) {
    return { loginDate: '-', loginTime: '-', ip: '-' }
  }

  const timestamp = parseLoginTimestamp(value.slice(0, separator))
  if (timestamp == null) {
    return { loginDate: '-', loginTime: '-', ip: value.slice(separator + 1).trim() || '-' }
  }

  const locale = displayLocale()
  return {
    loginDate: formatPanelDateTime(timestamp, locale, { dateStyle: 'short' }),
    loginTime: formatPanelTime(timestamp, locale),
    ip: value.slice(separator + 1).trim() || '-',
  }
}

const editModal = ref({
  visible: false,
  user: {},
})

const showEditModal = (user: any) => {
  editModal.value.user = user
  editModal.value.visible = true
}
const closeEditModal = () => {
  editModal.value.visible = false
  editModal.value.user = {}
}
const saveEditModal = async (data:any) => {
  loading.value=true
  try {
    const response = await HttpUtils.post('api/changePass',data)
    if(response.success){
      await new Promise(resolve => window.setTimeout(resolve, 500))
      editModal.value.visible = false
    }
  } finally {
    loading.value=false
  }
}

const changesModal = ref({
  visible: false,
  actor: '',
})
const showChangesModal = (actor: string) => {
  changesModal.value.actor = actor
  changesModal.value.visible = true
}
const closeChangesModal = () => {
  changesModal.value.visible = false
  changesModal.value.actor = ''
}

const tokenModal = ref({
  visible: false,
})
const showTokenModal = () => {
  tokenModal.value.visible = true
}
const closeTokenModal = () => {
  tokenModal.value.visible = false
}
</script>
