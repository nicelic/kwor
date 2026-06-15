<template>
  <SubOutboundVue 
    v-model="modal.visible"
    :visible="modal.visible"
    :id="modal.id"
    :data="modal.data"
    :tags="subOutboundTags"
    @close="closeModal"
  />
  <SubManagerQrCode
    v-model="qrcode.visible"
    :visible="qrcode.visible"
    :tag="qrcode.tag"
    @close="closeQrCode"
  />
  <Stats
    v-model="stats.visible"
    :visible="stats.visible"
    :resource="stats.resource"
    :tag="stats.tag"
    @close="closeStats"
  />
  <SubGroup
    v-model="groupModal.visible"
    :visible="groupModal.visible"
    @close="closeGroupModal"
  />
  <v-row justify="center" class="mb-3">
    <v-col cols="12" class="d-flex justify-center flex-wrap ga-3">
      <v-btn color="primary" size="large" min-width="96" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
      <v-btn color="primary" size="large" min-width="96" @click="showGroupModal">{{ $t('actions.group') }}</v-btn>
      <v-btn color="error" variant="outlined" size="large" min-width="96" @click="showClearDialog = true">清空</v-btn>
    </v-col>
  </v-row>
  <v-dialog v-model="showClearDialog" max-width="460">
    <v-card rounded="lg">
      <v-card-title>清空订阅管理</v-card-title>
      <v-divider></v-divider>
      <v-card-text>
        <v-alert type="warning" variant="tonal" class="mb-3">
          此操作会清空订阅管理的全部节点和卡片，并清空所有分组内节点。
        </v-alert>
        <div>分组名称与分组订阅链接会保留。</div>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn color="success" variant="outlined" :disabled="clearingSubManager" @click="showClearDialog = false">{{ $t('no') }}</v-btn>
        <v-btn color="error" variant="outlined" :loading="clearingSubManager" @click="clearSubManager">{{ $t('yes') }}</v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
  <v-row>
    <v-col cols="12" sm="4" md="3" lg="2" v-for="(item, index) in <any[]>subOutbounds" :key="item.tag">
      <v-card rounded="xl" elevation="5" min-width="200" :title="item.tag">
        <v-card-subtitle style="margin-top: -20px;">
          <v-row>
            <v-col>{{ item.type }}</v-col>
          </v-row>
        </v-card-subtitle>
        <v-card-text>
          <v-row>
            <v-col>{{ $t('in.addr') }}</v-col>
            <v-col>
              {{ item.server?? '-' }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('in.port') }}</v-col>
            <v-col>
              {{ formatServerPort(item) }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('objects.tls') }}</v-col>
            <v-col>
              {{ Object.hasOwn(item,'tls') ? $t(item.tls?.enabled ? 'enable' : 'disable') : '-'  }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('online') }}</v-col>
            <v-col>
              <template v-if="onlines.includes(item.tag)">
                <v-chip density="comfortable" size="small" color="success" variant="flat">{{ $t('online') }}</v-chip>
              </template>
              <template v-else>-</template>
            </v-col>
          </v-row>
          <template v-if="isSSH(item)">
            <v-row>
              <v-col>username</v-col>
              <v-col>{{ readSSHUsername(item) || '-' }}</v-col>
            </v-row>
            <v-row>
              <v-col>private-key</v-col>
              <v-col>{{ hasSSHPrivateKey(item) ? 'configured' : '-' }}</v-col>
            </v-row>
            <v-row>
              <v-col>host-key</v-col>
              <v-col>{{ formatSSHList(item, ['host_key', 'host-key']) }}</v-col>
            </v-row>
            <v-row>
              <v-col>host-key-algorithms</v-col>
              <v-col>{{ formatSSHList(item, ['host_key_algorithms', 'host-key-algorithms']) }}</v-col>
            </v-row>
            <v-row>
              <v-col>private_key_path (singbox)</v-col>
              <v-col>{{ readSSHField(item, ['private_key_path']) || '-' }}</v-col>
            </v-row>
            <v-row>
              <v-col>client_version (singbox)</v-col>
              <v-col>{{ readSSHField(item, ['client_version']) || '-' }}</v-col>
            </v-row>
            <v-row>
              <v-col>cipher (singbox)</v-col>
              <v-col>{{ formatSSHList(item, ['cipher']) }}</v-col>
            </v-row>
            <v-row>
              <v-col>mac (singbox)</v-col>
              <v-col>{{ formatSSHList(item, ['mac']) }}</v-col>
            </v-row>
            <v-row>
              <v-col>kex_algorithm (singbox)</v-col>
              <v-col>{{ formatSSHList(item, ['kex_algorithm']) }}</v-col>
            </v-row>
          </template>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" @click="showModal(item.id)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" style="margin-inline-start:0;" color="warning" @click="delOverlay[index] = true">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
          <v-overlay
            v-model="delOverlay[index]"
            contained
            class="align-center justify-center"
          >
            <v-card :title="$t('actions.del')" rounded="lg">
              <v-divider></v-divider>
              <v-card-text>{{ $t('confirm') }}</v-card-text>
              <v-card-actions>
                <v-btn color="error" variant="outlined" @click="delSubOutbound(item.tag)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" @click="delOverlay[index] = false">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
          <v-btn icon="mdi-qrcode" style="margin-inline-start:0;" @click="showQrCode(item.tag)">
            <v-icon />
            <v-tooltip activator="parent" location="top" text="QrCode"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-chart-line" @click="showStats(item.tag)" v-if="Data().enableTraffic">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('stats.graphTitle')"></v-tooltip>
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import Data from '@/store/modules/data'
import HttpUtils from '@/plugins/httputil'
import SubOutboundVue from '@/layouts/modals/SubOutbound.vue'
import SubManagerQrCode from '@/layouts/modals/SubManagerQrCode.vue'
import Stats from '@/layouts/modals/Stats.vue'
import SubGroup from '@/layouts/modals/SubGroup.vue'
import { Outbound } from '@/types/outbounds'
import { computed, ref } from 'vue'
import { formatServerPortDisplay } from '@/plugins/portRange'

const subOutbounds = computed((): Outbound[] => {
  return <Outbound[]> Data().suboutbounds
})

const subOutboundTags = computed((): string[] => {
  return [...Data().suboutbounds?.map((o:Outbound) => o.tag), ...Data().endpoints?.map((e:any) => e.tag)]
})

const onlines = computed(() => {
  return Data().onlines.outbound?? []
})

const formatServerPort = (item: any): string => {
  return formatServerPortDisplay(item?.server_port, item?.server_ports)
}

const isSSH = (item: any): boolean => {
  return String(item?.type ?? '').trim().toLowerCase() === 'ssh'
}

const readSSHField = (item: any, keys: string[]): string => {
  if (!item || !Array.isArray(keys)) return ''
  for (const key of keys) {
    const value = item?.[key]
    if (typeof value === 'string' && value.trim().length > 0) {
      return value.trim()
    }
  }
  return ''
}

const readSSHList = (item: any, keys: string[]): string[] => {
  if (!item || !Array.isArray(keys)) return []
  for (const key of keys) {
    const value = item?.[key]
    if (Array.isArray(value)) {
      const list = value
        .map((entry) => String(entry ?? '').trim())
        .filter((entry) => entry.length > 0)
      if (list.length > 0) return list
    }
    if (typeof value === 'string' && value.trim().length > 0) {
      return value.split(/[\n,]+/).map((entry) => entry.trim()).filter((entry) => entry.length > 0)
    }
  }
  return []
}

const formatSSHList = (item: any, keys: string[]): string => {
  const list = readSSHList(item, keys)
  return list.length > 0 ? list.join(', ') : '-'
}

const readSSHUsername = (item: any): string => {
  return readSSHField(item, ['username', 'user'])
}

const hasSSHPrivateKey = (item: any): boolean => {
  return readSSHField(item, ['private_key', 'private-key', 'private_key_path']).length > 0
}

const modal = ref({
  visible: false,
  id: 0,
  data: "",
})

let delOverlay = ref(new Array<boolean>)

const showModal = (id: number) => {
  modal.value.id = id
  modal.value.data = id == 0 ? '' : JSON.stringify(subOutbounds.value.findLast(o => o.id == id))
  modal.value.visible = true
}

const closeModal = () => {
  modal.value.visible = false
}

const qrcode = ref({
  visible: false,
  tag: "",
})

const showQrCode = (tag: string) => {
  qrcode.value.tag = tag
  qrcode.value.visible = true
}
const closeQrCode = () => {
  qrcode.value.visible = false
}

const stats = ref({
  visible: false,
  resource: "outbound",
  tag: "",
})

const delSubOutbound = async (tag: string) => {
  const index = subOutbounds.value.findIndex(i => i.tag == tag)
  const success = await Data().save("suboutbounds", "del", tag)
  if (success) delOverlay.value[index] = false
}

const showStats = (tag: string) => {
  stats.value.tag = tag
  stats.value.visible = true
}
const closeStats = () => {
  stats.value.visible = false
}

const groupModal = ref({
  visible: false,
})
const showClearDialog = ref(false)
const clearingSubManager = ref(false)

const showGroupModal = () => {
  groupModal.value.visible = true
}

const closeGroupModal = () => {
  groupModal.value.visible = false
}

const clearSubManager = async () => {
  if (clearingSubManager.value) {
    return
  }
  clearingSubManager.value = true
  try {
    const msg = await HttpUtils.post('api/clearSubManager', {})
    if (msg.success && msg.obj) {
      Data().setNewData(msg.obj)
      showClearDialog.value = false
    }
  } finally {
    clearingSubManager.value = false
  }
}

</script>

