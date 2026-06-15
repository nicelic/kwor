<template>
  <OutboundGroup
    v-if="allowOutboundGroups"
    v-model="groupModal.visible"
    :visible="groupModal.visible"
    :namespace="props.namespace"
    @close="closeGroupModal"
  />
  <OutboundVue
    v-model="modal.visible"
    :visible="modal.visible"
    :id="modal.id"
    :namespace="props.namespace"
    :data="modal.data"
    :tags="outboundTags"
    @close="closeModal"
  />
  <Stats
    v-model="stats.visible"
    :visible="stats.visible"
    :resource="stats.resource"
    :tag="stats.tag"
    @close="closeStats"
  />
  <v-row>
    <v-col cols="12" justify="center" align="center">
      <v-btn color="primary" @click="showModal(0)">{{ $t('actions.add') }}</v-btn>
      <v-btn v-if="allowOutboundGroups" color="primary" class="ml-2" @click="showGroupModal">{{ $t('actions.group') }}</v-btn>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="4" md="3" lg="2" v-for="(item, index) in <any[]>outbounds" :key="item.tag">
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
              {{ item.server ?? '-' }}
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
              {{ Object.hasOwn(item, 'tls') ? $t(item.tls?.enabled ? 'enable' : 'disable') : '-' }}
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
          <v-btn icon="mdi-file-edit" @click="showModal(item.id)">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('actions.edit')"></v-tooltip>
          </v-btn>
          <v-btn icon="mdi-file-remove" style="margin-inline-start: 0;" color="warning" @click="delOverlay[index] = true">
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
                <v-btn color="error" variant="outlined" @click="delOutbound(item.tag)">{{ $t('yes') }}</v-btn>
                <v-btn color="success" variant="outlined" @click="delOverlay[index] = false">{{ $t('no') }}</v-btn>
              </v-card-actions>
            </v-card>
          </v-overlay>
          <v-btn icon="mdi-chart-line" @click="showStats(item.tag)" v-if="enableTraffic">
            <v-icon />
            <v-tooltip activator="parent" location="top" :text="$t('stats.graphTitle')"></v-tooltip>
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-col>
  </v-row>
</template>

<script lang="ts" setup>
import OutboundVue from '@/layouts/modals/Outbound.vue'
import OutboundGroup from '@/layouts/modals/OutboundGroup.vue'
import Stats from '@/layouts/modals/Stats.vue'
import { Outbound } from '@/types/outbounds'
import { computed, ref } from 'vue'
import { formatServerPortDisplay } from '@/plugins/portRange'
import { getNamespaceStore, type UiNamespace } from '@/store/uiNamespace'

const props = withDefaults(defineProps<{ namespace?: UiNamespace }>(), {
  namespace: 'default',
})

const store = getNamespaceStore(props.namespace)
const allowOutboundGroups = true

const outbounds = computed((): Outbound[] => {
  return <Outbound[]>store.outbounds
})

const outboundTags = computed((): string[] => {
  return [...store.outbounds?.map((o: Outbound) => o.tag), ...store.endpoints?.map((e: any) => e.tag)]
})

const onlines = computed(() => {
  return store.onlines.outbound ?? []
})

const enableTraffic = computed(() => {
  return store.enableTraffic
})

const modal = ref({
  visible: false,
  id: 0,
  data: '',
})

const delOverlay = ref(new Array<boolean>())
const groupModal = ref({
  visible: false,
})

const showModal = (id: number) => {
  modal.value.id = id
  modal.value.data = id == 0 ? '' : JSON.stringify(outbounds.value.findLast(o => o.id == id))
  modal.value.visible = true
}

const closeModal = () => {
  modal.value.visible = false
}

const showGroupModal = () => {
  groupModal.value.visible = true
}

const closeGroupModal = () => {
  groupModal.value.visible = false
}

const formatServerPort = (item: any): string => {
  return formatServerPortDisplay(item?.server_port, item?.server_ports)
}

const stats = ref({
  visible: false,
  resource: 'outbound',
  tag: '',
})

const delOutbound = async (tag: string) => {
  const index = outbounds.value.findIndex(i => i.tag == tag)
  const success = await store.save('outbounds', 'del', tag)
  if (success) delOverlay.value[index] = false
}

const showStats = (tag: string) => {
  stats.value.tag = tag
  stats.value.visible = true
}

const closeStats = () => {
  stats.value.visible = false
}
</script>
