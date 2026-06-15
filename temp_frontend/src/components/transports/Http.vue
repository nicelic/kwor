<template>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.hosts')"
      hide-details
      v-model="hosts">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-text-field
      :label="$t('transport.path')"
      hide-details
      v-model="transport.path">
      </v-text-field>
    </v-col>
    <v-col cols="12" sm="6" md="4">
      <v-combobox
      :label="$t('transport.httpMethod')"
      hide-details
      clearable
      :items="methodList"
      v-model="methodModel">
      </v-combobox>
    </v-col>
  </v-row>
  <v-row>
    <v-col cols="12" sm="6" md="4">
      <v-switch
      color="primary"
      label="keep-alive"
      hide-details
      v-model="keepAliveEnabled">
      </v-switch>
    </v-col>
  </v-row>
  <Headers :data="transport" />
</template>

<script lang="ts">
import { HTTP } from '../../types/transport'
import Headers from '../Headers.vue'
export default {
  props: {
    transport: {
      type: Object,
      required: true,
    },
    namespace: {
      type: String,
      default: 'default',
    },
  },
  data() {
    return {
      methodList: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']
    }
  },
  computed: {
    Http(): HTTP {
      return <HTTP> this.$props.transport?? {}
    },
    methodModel: {
      get() {
        return typeof this.Http.method === 'string' ? this.Http.method : ''
      },
      set(newValue: string) {
        const method = this.normalizeMethod(newValue)
        if (method.length > 0) {
          this.$props.transport.method = method
        } else {
          delete this.$props.transport.method
        }
      },
    },
    hosts: {
      get() { return this.readHostValues().join(',') },
      set(newValue:string) {
        const hostValues = this.normalizeHeaderValues(newValue)
        this.writeHeaderValue('Host', hostValues.length > 0 ? hostValues : undefined)
        delete this.$props.transport.host
      },
    },
    keepAliveEnabled: {
      get(): boolean {
        return this.readHeaderValues('Connection').some((value: string) => value.toLowerCase() === 'keep-alive')
      },
      set(enabled: boolean) {
        if (enabled) {
          this.writeHeaderValue('Connection', ['keep-alive'])
          return
        }
        this.writeHeaderValue('Connection', undefined)
      },
    },
  },
  methods: {
    normalizeMethod(raw: unknown): string {
      if (typeof raw !== 'string') return ''
      return raw.trim().toUpperCase()
    },
    normalizeHeaderValues(raw: unknown): string[] {
      if (typeof raw === 'string') {
        return raw
          .split(',')
          .map((value: string) => value.trim())
          .filter((value: string) => value.length > 0)
      }
      if (Array.isArray(raw)) {
        return raw
          .filter((value: unknown) => typeof value === 'string')
          .map((value: unknown) => String(value).trim())
          .filter((value: string) => value.length > 0)
      }
      return []
    },
    ensureHeadersMap(): Record<string, unknown> {
      const rawHeaders = this.$props.transport.headers
      if (!rawHeaders || typeof rawHeaders !== 'object' || Array.isArray(rawHeaders)) {
        this.$props.transport.headers = {}
      }
      return this.$props.transport.headers as Record<string, unknown>
    },
    findHeaderKey(name: string): string | undefined {
      const rawHeaders = this.$props.transport.headers
      if (!rawHeaders || typeof rawHeaders !== 'object' || Array.isArray(rawHeaders)) {
        return undefined
      }
      const headers = rawHeaders as Record<string, unknown>
      const target = name.toLowerCase()
      for (const key of Object.keys(headers)) {
        if (key.toLowerCase() === target) {
          return key
        }
      }
      return undefined
    },
    readHeaderValues(name: string): string[] {
      const key = this.findHeaderKey(name)
      if (!key) return []
      const headers = this.$props.transport.headers as Record<string, unknown>
      return this.normalizeHeaderValues(headers[key])
    },
    readHostValues(): string[] {
      const hostHeaders = this.readHeaderValues('Host')
      if (hostHeaders.length > 0) return hostHeaders
      return this.normalizeHeaderValues(this.$props.transport.host)
    },
    writeHeaderValue(name: string, values?: string[]) {
      const headers = this.ensureHeadersMap()
      const key = this.findHeaderKey(name) ?? name
      if (!values || values.length === 0) {
        delete headers[key]
        this.cleanupEmptyHeaders()
        return
      }
      headers[key] = values.length === 1 ? values[0] : values
      this.$props.transport.headers = headers
    },
    cleanupEmptyHeaders() {
      const rawHeaders = this.$props.transport.headers
      if (!rawHeaders || typeof rawHeaders !== 'object' || Array.isArray(rawHeaders)) {
        delete this.$props.transport.headers
        return
      }
      if (Object.keys(rawHeaders as Record<string, unknown>).length === 0) {
        delete this.$props.transport.headers
      }
    },
    sanitizeLegacyHttpTransportFields() {
      if (!this.$props.transport || this.$props.transport.type !== 'http') return

      delete this.$props.transport.idle_timeout
      delete this.$props.transport.ping_timeout

      const legacyHostValues = this.normalizeHeaderValues(this.$props.transport.host)
      if (legacyHostValues.length > 0 && this.readHeaderValues('Host').length === 0) {
        this.writeHeaderValue('Host', legacyHostValues)
      }
      delete this.$props.transport.host

      const normalizedMethod = this.normalizeMethod(this.$props.transport.method)
      if (normalizedMethod.length > 0) {
        this.$props.transport.method = normalizedMethod
      } else {
        delete this.$props.transport.method
      }

      this.cleanupEmptyHeaders()
    }
  },
  mounted() {
    this.sanitizeLegacyHttpTransportFields()
  },
  watch: {
    transport: {
      handler() {
        this.sanitizeLegacyHttpTransportFields()
      },
      deep: false,
    },
  },
  components: { Headers }
}
</script>
