<template>
  <v-card :subtitle="cardSubtitle">
    <template v-if="direction === 'in'">
      <v-row>
        <v-col cols="12" sm="6">
          <v-select
            label="AEAD Method"
            :items="aeadMethods"
            hide-details
            v-model="aeadMethod">
          </v-select>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6">
          <v-select
            label="Table Type"
            :items="tableTypes"
            hide-details
            v-model="tableType">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Handshake Timeout"
            type="number"
            min="1"
            hide-details
            v-model.number="handshakeTimeout">
          </v-text-field>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Padding Min"
            type="number"
            min="1"
            hide-details
            v-model.number="paddingMin">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Padding Max"
            type="number"
            min="1"
            hide-details
            v-model.number="paddingMax">
          </v-text-field>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Custom Table"
            hide-details
            append-inner-icon="mdi-refresh"
            @click:append-inner="refreshCustomTable"
            v-model="customTable">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6">
          <v-textarea
            label="Custom Tables"
            hint='JSON array / comma / newline separated'
            persistent-hint
            rows="1"
            auto-grow
            append-inner-icon="mdi-refresh"
            @click:append-inner="refreshCustomTables"
            v-model="customTablesText">
          </v-textarea>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6">
          <v-select
            label="Enable Pure Downlink"
            :items="boolItems"
            hide-details
            v-model="enablePureDownlink">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6">
          <v-select
            label="Disable HTTP Mask"
            :items="boolItems"
            hide-details
            v-model="disableHTTPMask">
          </v-select>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6" md="6">
          <v-select
            label="HTTPMask Disable"
            :items="boolItems"
            hide-details
            v-model="httpmaskDisable">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="6">
          <v-select
            label="HTTPMask Mode"
            :items="httpmaskModes"
            hide-details
            v-model="httpmaskMode">
          </v-select>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Path Root"
            hide-details
            v-model="httpmaskPathRoot">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Fallback"
            hide-details
            v-model="fallbackValue">
          </v-text-field>
        </v-col>
      </v-row>

      <v-row v-if="showInboundHTTPMaskDisableHint">
        <v-col cols="12">
          <v-alert type="warning" variant="tonal" density="compact">
            mode、path_root、disable-http-mask 可能会自动不生效
          </v-alert>
        </v-col>
      </v-row>
    </template>

    <template v-else-if="direction === 'out'">
      <v-row>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Key / UUID"
            hide-details
            v-model="keyValue">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6">
          <v-select
            label="AEAD Method"
            :items="aeadMethods"
            hide-details
            v-model="aeadMethod">
          </v-select>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6">
          <v-select
            label="Table Type"
            :items="tableTypes"
            hide-details
            v-model="tableType">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6">
          <v-select
            label="Enable Pure Downlink"
            :items="boolItems"
            hide-details
            v-model="enablePureDownlink">
          </v-select>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Padding Min"
            type="number"
            min="1"
            hide-details
            v-model.number="paddingMin">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Padding Max"
            type="number"
            min="1"
            hide-details
            v-model.number="paddingMax">
          </v-text-field>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Custom Table"
            hide-details
            append-inner-icon="mdi-refresh"
            @click:append-inner="refreshCustomTable"
            v-model="customTable">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6">
          <v-textarea
            label="Custom Tables"
            hint='JSON array / comma / newline separated'
            persistent-hint
            rows="1"
            auto-grow
            append-inner-icon="mdi-refresh"
            @click:append-inner="refreshCustomTables"
            v-model="customTablesText">
          </v-textarea>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-select
            label="HTTPMask Disable"
            :items="boolItems"
            hide-details
            v-model="httpmaskDisable">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            label="HTTPMask Mode"
            :items="httpmaskModes"
            hide-details
            v-model="httpmaskMode">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            label="HTTPMask TLS"
            :items="boolItems"
            hide-details
            v-model="httpmaskTLS">
          </v-select>
        </v-col>
      </v-row>

      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            label="Mask Host / SNI"
            hide-details
            v-model="httpmaskHost">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            label="Path Root"
            hide-details
            v-model="httpmaskPathRoot">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            label="HTTPMask Multiplex"
            :items="httpmaskMultiplexModes"
            hide-details
            v-model="httpmaskMultiplex">
          </v-select>
        </v-col>
      </v-row>
    </template>

    <template v-else>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-select
            label="HTTPMask TLS"
            :items="boolItems"
            hide-details
            v-model="httpmaskTLS">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            label="Mask Host / SNI"
            hide-details
            v-model="httpmaskHost">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            label="HTTPMask Multiplex"
            :items="httpmaskMultiplexModes"
            hide-details
            v-model="httpmaskMultiplex">
          </v-select>
        </v-col>
      </v-row>
    </template>
  </v-card>
</template>

<script lang="ts">
type SudokuDirection = 'in' | 'out' | 'out_json'

export default {
  props: ['data', 'direction'],
  data() {
    return {
      aeadMethods: [
        'chacha20-poly1305',
        'aes-128-gcm',
        'none',
      ],
      tableTypes: [
        'prefer_ascii',
        'prefer_entropy',
      ],
      httpmaskModes: [
        { title: 'legacy', value: 'legacy' },
        { title: 'stream (split-stream)', value: 'stream' },
        { title: 'poll', value: 'poll' },
        { title: 'auto (stream then poll)', value: 'auto' },
        { title: 'ws (WebSocket tunnel)', value: 'ws' },
      ],
      httpmaskMultiplexModes: [
        'off',
        'auto',
        'on',
      ],
      boolItems: [
        { title: 'false', value: false },
        { title: 'true', value: true },
      ],
    }
  },
  methods: {
    normalizeStringInput(value: unknown): string {
      let normalized = String(value ?? '').trim()
      while (
        normalized.length >= 2 &&
        (
          (normalized.startsWith('"') && normalized.endsWith('"')) ||
          (normalized.startsWith("'") && normalized.endsWith("'"))
        )
      ) {
        normalized = normalized.slice(1, -1).trim()
      }
      return normalized
    },
    normalizeCustomTablePattern(value: unknown): string {
      let normalized = this.normalizeStringInput(value).toLowerCase()
      normalized = normalized.replace(/^\[+/, '').replace(/\]+$/, '').trim()
      normalized = this.normalizeStringInput(normalized).toLowerCase()
      return normalized
    },
    formatCustomTablesAsJSON(values: string[]): string {
      if (!Array.isArray(values) || values.length === 0) return ''
      return `["${values.join('","')}"]`
    },
    generateCustomTableValue(): string {
      const layout = ['x', 'x', 'p', 'p', 'v', 'v', 'v', 'v']
      for (let index = layout.length - 1; index > 0; index -= 1) {
        const swapIndex = Math.floor(Math.random() * (index + 1))
        ;[layout[index], layout[swapIndex]] = [layout[swapIndex], layout[index]]
      }
      return layout.join('')
    },
    generateCustomTablesList(count = 2): string[] {
      const target = Math.max(1, count)
      const values = new Set<string>()
      while (values.size < target) {
        values.add(this.generateCustomTableValue())
      }
      return Array.from(values)
    },
    refreshCustomTable() {
      this.data.custom_table = this.generateCustomTableValue()
      this.data.table_type = 'prefer_entropy'
    },
    refreshCustomTables() {
      this.data.custom_tables = this.generateCustomTablesList(2)
      this.data.table_type = 'prefer_entropy'
    },
    hasCustomTableValue(): boolean {
      return typeof this.data.custom_table === 'string' && this.normalizeStringInput(this.data.custom_table) !== ''
    },
    hasCustomTablesValue(): boolean {
      if (Array.isArray(this.data.custom_tables)) {
        return this.data.custom_tables.some((item: unknown) => this.normalizeStringInput(item) !== '')
      }
      if (typeof this.data.custom_tables === 'string') {
        return this.normalizeCustomTables(this.data.custom_tables).length > 0
      }
      return false
    },
    shouldAutoGenerateCustomTable(): boolean {
      if (this.direction === 'out_json') return false
      const inboundOrOutboundId = Number(this.data?.id ?? 0)
      if (Number.isFinite(inboundOrOutboundId) && inboundOrOutboundId > 0) return false
      return true
    },
    ensureCustomTableDefaults() {
      if (!this.shouldAutoGenerateCustomTable()) return
      if (!this.hasCustomTableValue()) {
        this.refreshCustomTable()
      }
      if (!this.hasCustomTablesValue()) {
        this.refreshCustomTables()
      }
    },
    normalizePositiveInteger(value: unknown, fallback: number): number {
      const raw = Number(value)
      if (!Number.isFinite(raw)) return fallback
      const normalized = Math.floor(raw)
      return normalized > 0 ? normalized : fallback
    },
    ensureHttpmask(): Record<string, any> {
      if (!this.data.httpmask || typeof this.data.httpmask !== 'object') {
        this.data.httpmask = {}
      }
      return this.data.httpmask
    },
    cleanupHttpmask() {
      if (!this.data.httpmask || typeof this.data.httpmask !== 'object') return
      const hasValue = Object.entries(this.data.httpmask).some(([_, value]) => {
        if (value === undefined || value === null) return false
        if (typeof value === 'string') return value.trim() !== ''
        return true
      })
      if (!hasValue) {
        delete this.data.httpmask
      }
    },
    getHttpmaskValue<T>(key: string, fallback: T): T {
      if (!this.data.httpmask || typeof this.data.httpmask !== 'object') return fallback
      const value = this.data.httpmask[key]
      if (value === undefined || value === null) return fallback
      if (typeof value === 'string' && value.trim() === '') return fallback
      return value
    },
    setHttpmaskValue(key: string, value: unknown) {
      const httpmask = this.ensureHttpmask()
      if (value === undefined || value === null) {
        delete httpmask[key]
        this.cleanupHttpmask()
        return
      }
      if (typeof value === 'string') {
        const normalized = this.normalizeStringInput(value)
        if (normalized === '') {
          delete httpmask[key]
          this.cleanupHttpmask()
          return
        }
        httpmask[key] = normalized
        return
      }
      httpmask[key] = value
    },
    normalizeCustomTables(value: unknown): string[] {
      return String(value ?? '')
        .replace(/\uFF0C/g, ',')
        .replace(/\r\n/g, '\n')
        .split(/[\n,]/)
        .map((item) => this.normalizeCustomTablePattern(item))
        .filter((item, index, list) => item !== '' && list.indexOf(item) === index)
    },
    normalizeAEADMethod(value: unknown): string {
      const normalized = this.normalizeStringInput(value).toLowerCase()
      switch (normalized) {
        case 'aes-128-gcm':
          return 'aes-128-gcm'
        case 'none':
          return 'none'
        default:
          return 'chacha20-poly1305'
      }
    },
    enforceAEADMethodCompatibility() {
      if (this.direction === 'out_json') return
      const currentMethod = this.normalizeAEADMethod(this.data.aead_method)
      if (this.data.aead_method !== undefined && currentMethod !== this.data.aead_method) {
        this.data.aead_method = currentMethod
      }
      if (this.data.enable_pure_downlink !== true && currentMethod === 'none') {
        this.data.aead_method = 'aes-128-gcm'
      }
    },
  },
  mounted() {
    this.enforceAEADMethodCompatibility()
    this.ensureCustomTableDefaults()
  },
  computed: {
    cardSubtitle(): string | undefined {
      return this.direction === 'out_json' ? undefined : 'Sudoku'
    },
    keyValue: {
      get(): string {
        return typeof this.data.key === 'string' ? this.data.key : ''
      },
      set(v: string) {
        const normalized = this.normalizeStringInput(v)
        this.data.key = normalized !== '' ? normalized : undefined
      }
    },
    aeadMethod: {
      get(): string {
        return this.data.aead_method ?? 'chacha20-poly1305'
      },
      set(v: string) {
        this.data.aead_method = this.normalizeAEADMethod(v)
        this.enforceAEADMethodCompatibility()
      }
    },
    tableType: {
      get(): string {
        return this.data.table_type ?? 'prefer_ascii'
      },
      set(v: string) {
        this.data.table_type = v || 'prefer_ascii'
      }
    },
    paddingMin: {
      get(): number {
        return this.normalizePositiveInteger(this.data.padding_min, 1)
      },
      set(v: number) {
        this.data.padding_min = this.normalizePositiveInteger(v, 1)
      }
    },
    paddingMax: {
      get(): number {
        return this.normalizePositiveInteger(this.data.padding_max, 15)
      },
      set(v: number) {
        this.data.padding_max = this.normalizePositiveInteger(v, 15)
      }
    },
    handshakeTimeout: {
      get(): number {
        return this.normalizePositiveInteger(this.data.handshake_timeout, 5)
      },
      set(v: number) {
        this.data.handshake_timeout = this.normalizePositiveInteger(v, 5)
      }
    },
    enablePureDownlink: {
      get(): boolean {
        return this.data.enable_pure_downlink === true
      },
      set(v: boolean) {
        this.data.enable_pure_downlink = v === true
        this.enforceAEADMethodCompatibility()
      }
    },
    disableHTTPMask: {
      get(): boolean {
        return this.data.disable_http_mask === true
      },
      set(v: boolean) {
        this.data.disable_http_mask = v === true
      }
    },
    customTable: {
      get(): string {
        return typeof this.data.custom_table === 'string' ? this.data.custom_table : ''
      },
      set(v: string) {
        const normalized = this.normalizeCustomTablePattern(v)
        this.data.custom_table = normalized !== '' ? normalized : undefined
      }
    },
    customTablesText: {
      get(): string {
        if (Array.isArray(this.data.custom_tables)) {
          return this.formatCustomTablesAsJSON(this.data.custom_tables)
        }
        if (typeof this.data.custom_tables === 'string') {
          const normalized = this.normalizeCustomTables(this.data.custom_tables)
          return normalized.length > 0 ? this.formatCustomTablesAsJSON(normalized) : ''
        }
        return ''
      },
      set(v: string) {
        const normalized = this.normalizeCustomTables(v)
        this.data.custom_tables = normalized.length > 0 ? normalized : undefined
      }
    },
    httpmaskDisable: {
      get(): boolean {
        return this.getHttpmaskValue('disable', false)
      },
      set(v: boolean) {
        this.setHttpmaskValue('disable', v === true)
      }
    },
    httpmaskMode: {
      get(): string {
        return this.getHttpmaskValue('mode', 'legacy')
      },
      set(v: string) {
        this.setHttpmaskValue('mode', v || 'legacy')
      }
    },
    httpmaskTLS: {
      get(): boolean {
        return this.getHttpmaskValue('tls', true)
      },
      set(v: boolean) {
        this.setHttpmaskValue('tls', v === true)
      }
    },
    httpmaskHost: {
      get(): string {
        return this.getHttpmaskValue('host', '')
      },
      set(v: string) {
        this.setHttpmaskValue('host', v)
      }
    },
    httpmaskPathRoot: {
      get(): string {
        return this.getHttpmaskValue('path_root', '')
      },
      set(v: string) {
        this.setHttpmaskValue('path_root', v)
      }
    },
    httpmaskMultiplex: {
      get(): string {
        return this.getHttpmaskValue('multiplex', 'off')
      },
      set(v: string) {
        this.setHttpmaskValue('multiplex', v || 'off')
      }
    },
    fallbackValue: {
      get(): string {
        return typeof this.data.fallback === 'string' ? this.data.fallback : ''
      },
      set(v: string) {
        const normalized = this.normalizeStringInput(v)
        this.data.fallback = normalized !== '' ? normalized : undefined
      }
    },
    showInboundHTTPMaskDisableHint(): boolean {
      return this.direction === 'in' && (this.disableHTTPMask || this.httpmaskDisable)
    },
  },
  watch: {
    'data.aead_method'() {
      this.enforceAEADMethodCompatibility()
    },
    'data.enable_pure_downlink'() {
      this.enforceAEADMethodCompatibility()
    },
    direction() {
      if (this.direction === ('in' as SudokuDirection)) {
        if (this.data.httpmask?.tls !== undefined) delete this.data.httpmask.tls
        if (this.data.httpmask?.host !== undefined) delete this.data.httpmask.host
        if (this.data.httpmask?.multiplex !== undefined) delete this.data.httpmask.multiplex
      } else if (this.data.disable_http_mask !== undefined && this.direction !== ('in' as SudokuDirection)) {
        delete this.data.disable_http_mask
      }
      this.cleanupHttpmask()
      this.ensureCustomTableDefaults()
    }
  }
}
</script>
