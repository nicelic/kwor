<template>
  <v-card>
    <v-card-subtitle>
      {{ $t('objects.headers') }}
      <v-chip color="primary" density="compact" variant="elevated" :disabled="disabled || headerRows.length >= maxHeaders" @click="add_header">
      <v-icon icon="mdi-plus" />
      </v-chip>
    </v-card-subtitle>
    <v-row v-for="(header, index) in headerRows" :key="header.key">
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          :label="$t('objects.key')"
          :disabled="disabled"
          :maxlength="maxHeaderNameBytes"
          hide-details
          @update:model-value="updateHeaderName(index, $event)"
          v-model="header.name">
        </v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-text-field
          :label="$t('objects.value')"
          :disabled="disabled"
          :maxlength="maxHeaderValueBytes"
          hide-details
          @update:model-value="updateHeaderValue(index, $event)"
          v-model="header.value">
          <template v-slot:append>
            <v-icon v-if="!disabled" @click="del_header(index)" color="error" icon="mdi-delete" />
          </template>
        </v-text-field>
      </v-col>
    </v-row>
  </v-card>
</template>

<script lang="ts">

type Header = {
  key: string
  name: string
  value: string
}
export default {
  props: {
    data: { type: Object, required: true },
    maxHeaders: { type: Number, default: Number.MAX_SAFE_INTEGER },
    disabled: { type: Boolean, default: false },
    maxHeaderNameBytes: { type: Number, default: 128 },
    maxHeaderValueBytes: { type: Number, default: 1024 },
  },
  data() {
    return {
      headerRows: <Header[]>[],
      headerRowSequence: 0,
    }
  },
  methods: {
    add_header() {
      if (this.$props.disabled || this.headerRows.length >= this.$props.maxHeaders) return
      this.headerRows.push({ key: `header-${++this.headerRowSequence}`, name: "Host", value: "" })
      this.commitHeaderRows()
    },
    del_header(i:number) {
      if (this.$props.disabled) return
      this.headerRows.splice(i,1)
      this.commitHeaderRows()
    },
    updateHeaderName(i:number, value: unknown) {
      if (this.$props.disabled) return
      const row = this.headerRows[i]
      if (!row) return
      row.name = String(value ?? '')
      this.commitHeaderRows()
    },
    updateHeaderValue(i:number, value: unknown) {
      if (this.$props.disabled) return
      const row = this.headerRows[i]
      if (!row) return
      row.value = String(value ?? '')
      this.commitHeaderRows()
    },
    headerIdentity(name: string, value: string): string {
      return JSON.stringify([name, value])
    },
    syncHeaderRows() {
      const reusableKeys = new Map<string, string[]>()
      for (const row of this.headerRows) {
        const identity = this.headerIdentity(row.name, row.value)
        const keys = reusableKeys.get(identity) ?? []
        keys.push(row.key)
        reusableKeys.set(identity, keys)
      }

      const nextRows: Header[] = []
      const headers = this.$props.data?.headers as Record<string, unknown> | undefined
      if (headers && typeof headers === 'object' && !Array.isArray(headers)) {
        for (const name of Object.keys(headers)) {
          const rawValue = headers[name]
          const values = Array.isArray(rawValue) ? rawValue : [rawValue]
          for (const rawItem of values) {
            const value = String(rawItem ?? '')
            const identity = this.headerIdentity(name, value)
            const key = reusableKeys.get(identity)?.shift() ?? `header-${++this.headerRowSequence}`
            nextRows.push({ key, name, value })
          }
        }
      }
      this.headerRows = nextRows
    },
    commitHeaderRows() {
      if (this.headerRows.length === 0) {
        delete this.$props.data.headers
        return
      }

      const headers = Object.create(null) as Record<string, string | string[]>
      for (const row of this.headerRows) {
        const name = row.name.trim()
        if (name === '') continue
        if (Object.hasOwn(headers, name)) {
          const existing = headers[name]
          headers[name] = Array.isArray(existing)
            ? [...existing, row.value]
            : [existing, row.value]
        } else {
          headers[name] = row.value
        }
      }
      if (Object.keys(headers).length === 0) {
        delete this.$props.data.headers
        return
      }
      this.$props.data.headers = headers
    },
  },
  created() {
    this.syncHeaderRows()
  },
  watch: {
    data() {
      this.syncHeaderRows()
    },
    'data.headers': {
      handler() {
        this.syncHeaderRows()
      },
      deep: true,
    },
  }
}
</script>
