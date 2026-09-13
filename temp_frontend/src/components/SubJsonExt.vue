<template>
  <SettingsJsonSubManage v-bind="$attrs" />
</template>

<script lang="ts">
import SettingsJsonSubManage from './SettingsJsonSubManage.vue'

export default {
  name: 'SubJsonExtCompat',
  components: {
    SettingsJsonSubManage,
  },
}
</script>
