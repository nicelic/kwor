<template>
  <ServiceVue 
    v-model="modal.visible"
    :visible="modal.visible"
    :id="modal.id"
    :data="modal.data"
    :inTags="inTags"
    :tsTags="tsTags"
    :ssTags="ssTags"
    :tlsConfigs="tlsConfigs"
    @close="closeModal"
  />
  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn color="primary" :disabled="deletingId !== null" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3" lg="2" v-for="item in <any[]>services" :key="item.id">
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
              {{ item.listen }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('in.port') }}</v-col>
            <v-col>
              {{ item.listen_port }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>{{ $t('objects.tls') }}</v-col>
            <v-col>
              {{ item.tls_id > 0 ? $t('enable') : $t('disable') }}
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" :disabled="deletingId !== null" @click="showModal(item.id)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" style="margin-inline-start:0;" color="warning" :disabled="deletingId !== null" @click="delOverlay[item.id] = true">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
          <v-overlay
            v-model="delOverlay[item.id]"
            contained
            :persistent="deletingId !== null"
            class="align-center justify-center"
          >
            <v-card :title="$t('actions.del')" rounded="lg">
              <v-divider></v-divider>
              <v-card-text>{{ $t('confirm') }}</v-card-text>
              <v-card-actions>
                <v-btn color="error" variant="outlined" :loading="deletingId === item.id" :disabled="deletingId !== null" @click="delSrv(item.id)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" :disabled="deletingId !== null" @click="delete delOverlay[item.id]">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
        </v-card-actions>
      </v-card>      
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import Data from '@/store/modules/data'
import { Srv } from '@/types/services'
import { computed, ref } from 'vue'
import ServiceVue from '@/layouts/modals/Service.vue'

const services = computed((): Srv[] => {
  return <Srv[]> Data().services
})

const srvTags = computed((): any[] => {
  return services.value?.map((o:Srv) => o.tag)
})

const tsTags = computed((): any[] => {
  return Data().endpoints?.filter((o:any) => o.type == "tailscale")?.map((o:any) => o.tag)
})

const ssTags = computed((): any[] => {
  return Data().inbounds?.filter((o:any) => o.type == "shadowsocks" && !o.users)?.map((o:any) => o.tag)
})

const inTags = computed((): any[] => {
  const tags = [
    ...(Data().inbounds ?? []).map((inbound: any) => inbound.route_tag ?? inbound.tag),
    ...(Data().endpoints ?? [])
      .filter((endpoint: any) => Number(endpoint.listen_port ?? 0) > 0)
      .map((endpoint: any) => endpoint.route_tag ?? endpoint.tag),
  ]
  return [...new Set(tags
    .filter((tag: unknown): tag is string => typeof tag === 'string')
    .map((tag: string) => tag.trim())
    .filter((tag: string) => tag.length > 0))]
})

const tlsConfigs = computed((): any[] => {
  return <any[]> Data().tlsConfigs
})

const modal = ref({
  visible: false,
  id: 0,
  data: "",
})

const deletingId = ref<number | null>(null)

const delOverlay = ref<Record<number, boolean>>({})

const showModal = (id: number) => {
  if (deletingId.value !== null) return
  modal.value.id = id
  modal.value.data = id == 0 ? '' : JSON.stringify(services.value.findLast(o => o.id == id))
  modal.value.visible = true
}

const closeModal = () => {
  if (deletingId.value !== null) return
  modal.value.visible = false
}

const delSrv = async (id: number) => {
  if (deletingId.value !== null) return
  const index = services.value.findIndex(i => i.id == id)
  const service = services.value[index]
  if (!service?.tag) return
  deletingId.value = id
  try {
    const success = await Data().save("services", "del", service.tag)
    if (success) delete delOverlay.value[id]
  } finally {
    deletingId.value = null
  }
}
</script>
