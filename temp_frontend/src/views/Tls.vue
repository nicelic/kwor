<template>
  <TlsVue
    v-model="modal.visible"
    :visible="modal.visible"
    :id="modal.id"
    :namespace="props.namespace"
    :data="modal.data"
    @close="closeModal"
    @save="saveModal"
  />
  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn color="primary" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3" lg="2" v-for="(item, index) in <any[]>tlsConfigs" :key="item.id">
      <v-card rounded="xl" elevation="5" min-width="200" :title="item.name">
        <v-card-subtitle style="margin-top: -20px;">
          {{ item.server?.server_name?.length > 0 ? item.server.server_name : '-' }}
        </v-card-subtitle>
        <v-card-text>
          <v-row>
            <v-col>{{ $t('pages.inbounds') }}</v-col>
            <v-col>
              <template v-if="tlsInbounds(item.id).length > 0">
                <v-tooltip activator="parent" dir="ltr" location="bottom">
                  <span v-for="i in tlsInbounds(item.id)">{{ i }}<br /></span>
                </v-tooltip>
                {{ tlsInbounds(item.id).length }}
              </template>
              <template v-else>-</template>
            </v-col>
          </v-row>
          <v-row>
            <v-col>ACME</v-col>
            <v-col>
              {{ $t(item.server?.acme == undefined ? 'no' : 'yes') }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>ECH</v-col>
            <v-col>
              {{ $t(item.server?.ech == undefined ? 'no' : 'yes') }}
            </v-col>
          </v-row>
          <v-row>
            <v-col>Reality</v-col>
            <v-col>
              {{ $t(item.server?.reality == undefined ? 'no' : 'yes') }}
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider></v-divider>
        <v-card-actions style="padding: 0;">
          <v-btn icon="mdi-file-edit" @click="showModal(item.id)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn v-if="tlsInbounds(item.id).length == 0" icon="mdi-file-remove" style="margin-inline-start: 0;" color="warning" @click="delOverlay[index] = true">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.del')"></v-tooltip>
          </v-btn>
          <v-overlay
            v-model="delOverlay[index]"
            contained
            class="align-center justify-center">
            <v-card :title="$t('actions.del')" rounded="lg">
              <v-divider></v-divider>
              <v-card-text>{{ $t('confirm') }}</v-card-text>
              <v-card-actions>
                <v-btn color="error" variant="outlined" @click="delTls(item.id)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" @click="delOverlay[index] = false">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
          <v-btn icon="mdi-content-duplicate" @click="clone(item)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.clone')"></v-tooltip>
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import TlsVue from '@/layouts/modals/Tls.vue'
import { computed, ref } from 'vue'
import { Inbound } from '@/types/inbounds'
import { tls, sanitizeTlsForNamespace } from '@/types/tls'
import { getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const store = getNamespaceStore(props.namespace)

const tlsConfigs = computed((): any[] => {
  return store.tlsConfigs
})

const inbounds = computed((): Inbound[] => {
  return store.inbounds
})

const tlsInbounds = (id: number): string[] => {
  return inbounds.value.filter(i => i.tls_id == id).map(i => i.tag)
}

const modal = ref({
  visible: false,
  id: 0,
  data: '',
})

const delOverlay = ref(new Array<boolean>(tlsConfigs.value.length).fill(false))

const normalizeTls = (data?: tls | null): tls => {
  return sanitizeTlsForNamespace(data, props.namespace)
}

const showModal = (id: number) => {
  modal.value.id = id
  modal.value.data = id == 0 ? '{}' : JSON.stringify(normalizeTls(tlsConfigs.value.findLast(t => t.id == id)))
  modal.value.visible = true
}

const clone = (obj: any) => {
  const data = normalizeTls(obj)
  data.id = 0
  while (tlsConfigs.value.findIndex(t => t.name == data.name) != -1) {
    data.name += '-copy'
  }
  saveModal(data)
}

const closeModal = () => {
  modal.value.visible = false
}

const saveModal = async (data: tls) => {
  const normalized = normalizeTls(data)
  const success = await store.save('tls', normalized.id > 0 ? 'edit' : 'new', normalized)
  if (success) modal.value.visible = false
}

const delTls = async (id: number) => {
  const index = tlsConfigs.value.findIndex(t => t.id == id)
  const success = await store.save('tls', 'del', id)
  if (success) delOverlay.value[index] = false
}
</script>
