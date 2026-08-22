<template>
  <EndpointVue 
    v-model="modal.visible"
    :visible="modal.visible"
    :id="modal.id"
    :data="modal.data"
    :tags="endpointTags"
    @close="closeModal"
  />
  <Stats
    v-model="stats.visible"
    :visible="stats.visible"
    :resource="stats.resource"
    :tag="stats.tag"
    @close="closeStats"
  />
  <QrCode
    v-model="qrcode.visible"
    :visible="qrcode.visible"
    :data="qrcode.data"
    @close="closeQrCode"
  />
  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn color="primary" :disabled="endpointWriteBusy" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3" lg="2" v-for="item in <any[]>endpoints" :key="item.id">
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
              {{ item.address?.length>0 ? item.address[0] : '-' }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('in.port') }}</v-col>
            <v-col>
              {{ item.listen_port>0 ? item.listen_port : '-' }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('types.wg.peers') }}</v-col>
            <v-col>
              {{ item.peers?.length?? '-'  }}
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
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" :disabled="endpointWriteBusy" @click="showModal(item.id)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" style="margin-inline-start:0;" color="warning" :disabled="endpointWriteBusy" @click="delOverlay[item.id] = true">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
          <v-overlay
            v-model="delOverlay[item.id]"
            contained
            :persistent="endpointWriteBusy"
            class="align-center justify-center"
          >
            <v-card :title="$t('actions.del')" rounded="lg">
              <v-divider></v-divider>
              <v-card-text>{{ $t('confirm') }}</v-card-text>
              <v-card-actions>
                <v-btn color="error" variant="outlined" :loading="deletingId === item.id" :disabled="endpointWriteBusy" @click="delEndpoint(item.id)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" :disabled="endpointWriteBusy" @click="delete delOverlay[item.id]">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
          <v-btn v-if="item.type == 'wireguard' && item.peers?.length > 0" icon="mdi-qrcode" class="me-2" :disabled="endpointWriteBusy" @click="showQrCode(item.id)">
            <v-icon />
            <v-tooltip activator="parent" location="top" text="QrCode"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-chart-line" :disabled="endpointWriteBusy" @click="showStats(item.tag)" v-if="Data().enableTraffic">
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
import EndpointVue from '@/layouts/modals/Endpoint.vue'
import Stats from '@/layouts/modals/Stats.vue'
import QrCode from '@/layouts/modals/WgQrCode.vue'
import { Endpoint } from '@/types/endpoints'
import { computed, ref } from 'vue'

const endpoints = computed((): Endpoint[] => {
  return <Endpoint[]> Data().endpoints
})

const endpointTags = computed((): any[] => {
  return endpoints.value?.map((o:Endpoint) => o.tag)
})

const onlines = computed(() => {
  return [...Data().onlines.inbound?? [], ...Data().onlines.outbound??[] ]
})

const modal = ref({
  visible: false,
  id: 0,
  data: "",
})

const delOverlay = ref<Record<number, boolean>>({})
const deletingId = ref<number | null>(null)
const endpointWriteBusy = computed(() => deletingId.value !== null)

const showModal = (id: number) => {
  if (endpointWriteBusy.value) return
  modal.value.id = id
  modal.value.data = id == 0 ? '' : JSON.stringify(endpoints.value.findLast(o => o.id == id))
  modal.value.visible = true
}

const closeModal = () => {
  if (endpointWriteBusy.value) return
  modal.value.visible = false
}

const stats = ref({
  visible: false,
  resource: "endpoint",
  tag: "",
})

const delEndpoint = async (id: number) => {
  if (endpointWriteBusy.value) return
  const endpoint = endpoints.value.find((item: any) => item.id === id)
  if (!endpoint?.tag) return
  deletingId.value = id
  try {
    const success = await Data().save("endpoints", "del", endpoint.tag)
    if (success) delete delOverlay.value[id]
  } finally {
    deletingId.value = null
  }
}

const showStats = (tag: string) => {
  if (endpointWriteBusy.value) return
  stats.value.tag = tag
  stats.value.visible = true
}
const closeStats = () => {
  stats.value.visible = false
}

const qrcode = ref({
  visible: false,
  data: <any>{},
})

const showQrCode = (id: number) => {
  if (endpointWriteBusy.value) return
  qrcode.value.data = endpoints.value.findLast(o => o.id == id)
  qrcode.value.visible = true
}
const closeQrCode = () => {
  qrcode.value.visible = false
}
</script>
