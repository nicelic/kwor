<template>
  <SettingsClashSubManage v-bind="$attrs" />
</template>

<script lang="ts">
import SettingsClashSubManage from './SettingsClashSubManage.vue'

export default {
  name: 'SubClashExtCompat',
  components: {
    SettingsClashSubManage,
  },
}
</script>
