<template>
  <v-card subtitle="Mieru">
    <template v-if="direction == 'in'">
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            :label="$t('in.addr')"
            hide-details
            required
            v-model="data.listen">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="5">
          <v-text-field
            :label="$t('in.port')"
            hide-details
            type="number"
            min="1"
            max="65535"
            required
            v-model="inboundPort">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="3">
          <v-select
            hide-details
            label="Transport"
            :items="transports"
            v-model="data.transport">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="5" v-if="showUserHintMandatory">
          <v-switch
            color="primary"
            label="User Hint Is Mandatory"
            v-model="data.user_hint_is_mandatory"
            hide-details>
          </v-switch>
        </v-col>
      </v-row>
    </template>
    <template v-else-if="direction == 'out'">
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
            :label="$t('out.addr')"
            hide-details
            required
            v-model="data.server">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="5">
          <v-text-field
            label="Port / Port Range"
            hint="Single port or a single range, e.g. 2999 or 2090-2099"
            persistent-hint
            hide-details
            required
            v-model="outboundPort">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="3">
          <v-select
            hide-details
            label="Transport"
            :items="transports"
            v-model="data.transport">
          </v-select>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Username"
            hide-details
            v-model="data.username">
          </v-text-field>
        </v-col>
        <v-col cols="12" sm="6">
          <v-text-field
            label="Password"
            hide-details
            v-model="data.password">
          </v-text-field>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4" v-if="showUDPOverTCP && !hideCommonUDPField">
          <v-switch
            color="primary"
            label="UDP over TCP"
            v-model="data.udp"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            label="Multiplexing"
            :items="multiplexingLevels"
            v-model="data.multiplexing">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            label="Handshake Mode"
            :items="handshakeModes"
            v-model="data.handshake_mode">
          </v-select>
        </v-col>
      </v-row>
    </template>
    <template v-else>
      <v-row>
        <v-col cols="12" sm="6" md="4" v-if="showUDPOverTCP && !hideCommonUDPField">
          <v-switch
            color="primary"
            label="UDP over TCP"
            v-model="data.udp"
            hide-details>
          </v-switch>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            label="Multiplexing"
            :items="multiplexingLevels"
            v-model="data.multiplexing">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            hide-details
            label="Handshake Mode"
            :items="handshakeModes"
            v-model="data.handshake_mode">
          </v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="isMihomoOutJson">
          <v-text-field
            :label="$t('rule.portRange')"
            hint="Single range only, e.g. 2090-2099"
            persistent-hint
            hide-details
            v-model="clientPortRange">
          </v-text-field>
        </v-col>
      </v-row>
    </template>
  </v-card>
</template>

<script lang="ts">
import { normalizePortRangeInput } from '@/plugins/portRange'

function normalizeMieruBindings(raw: string): string[] {
  return normalizePortRangeInput(String(raw ?? '').replace(/\uFF1A/g, ':'))
    .map((item) => item.replace(/:/g, '-'))
    .filter((item, index, arr) => arr.indexOf(item) === index)
}

function normalizeSingleMieruPortRange(raw: string): string | undefined {
  if (typeof raw !== 'string') return undefined
  const value = raw.trim().replace(/\uFF1A/g, ':')
  if (value === '' || value.includes(',') || value.includes('\uFF0C')) return undefined
  const normalized = normalizeMieruBindings(value)
  if (normalized.length !== 1) return undefined
  const binding = normalized[0]
  if (!binding.includes('-')) return undefined
  const [startRaw, endRaw] = binding.split('-')
  const start = Number.parseInt(startRaw, 10)
  const end = Number.parseInt(endRaw, 10)
  if (!Number.isInteger(start) || !Number.isInteger(end)) return undefined
  if (start < 1 || end > 65535 || start >= end) return undefined
  return `${start}-${end}`
}

function firstBindingPort(binding: string): number | undefined {
  const normalized = String(binding ?? '').trim()
  if (normalized === '') return undefined
  const first = normalized.split('-')[0]
  const port = Number.parseInt(first, 10)
  return Number.isNaN(port) ? undefined : port
}

export default {
  props: {
    direction: {
      type: String,
      required: true,
    },
    data: {
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
      transports: ['TCP', 'UDP'],
      multiplexingLevels: [
        'MULTIPLEXING_OFF',
        'MULTIPLEXING_LOW',
        'MULTIPLEXING_MIDDLE',
        'MULTIPLEXING_HIGH',
      ],
      handshakeModes: [
        'HANDSHAKE_STANDARD',
        'HANDSHAKE_NO_WAIT',
      ],
    }
  },
  computed: {
    inboundPort: {
      get(): string {
        if (typeof this.data.listen_port === 'number' && this.data.listen_port > 0) {
          return String(this.data.listen_port)
        }
        return ''
      },
      set(v: string) {
        const value = String(v ?? '').trim()
        if (value === '') {
          this.data.listen_port = undefined
          return
        }
        const parsed = Number.parseInt(value, 10)
        if (Number.isInteger(parsed) && parsed >= 1 && parsed <= 65535) {
          this.data.listen_port = parsed
        }
      }
    },
    outboundPort: {
      get(): string {
        if (typeof this.data.port_range === 'string' && this.data.port_range.trim() !== '') {
          return this.data.port_range
        }
        if (typeof this.data.server_port === 'number' && this.data.server_port > 0) {
          return String(this.data.server_port)
        }
        return ''
      },
      set(v: string) {
        const normalized = normalizeMieruBindings(v)
        if (normalized.length === 0) {
          this.data.server_port = undefined
          this.data.port_range = undefined
          return
        }

        const binding = normalized[0]
        const primaryPort = firstBindingPort(binding)
        this.data.server_port = primaryPort
        if (binding.includes('-')) {
          this.data.port_range = binding
          return
        }
        this.data.port_range = undefined
      }
    },
    clientPortRange: {
      get(): string {
        if (typeof this.data.port_range === 'string') {
          return this.data.port_range
        }
        return ''
      },
      set(v: string) {
        const value = String(v ?? '').trim()
        if (value === '') {
          this.data.port_range = undefined
          return
        }
        const normalized = normalizeSingleMieruPortRange(value)
        this.data.port_range = normalized ?? value
      }
    },
    isMihomoOutJson(): boolean {
      return this.$props.namespace === 'mihomo' && this.$props.direction === 'out_json'
    },
    showUserHintMandatory(): boolean {
      return this.$props.namespace === 'mihomo' && this.$props.direction === 'in'
    },
    showUDPOverTCP(): boolean {
      return this.data.transport !== 'UDP'
    },
    hideCommonUDPField(): boolean {
      return this.$props.namespace === 'mihomo' && this.$props.direction === 'out_json'
    },
  },
  watch: {
    'data.transport'(value: string) {
      if (value === 'UDP') {
        this.data.udp = undefined
      }
    }
  }
}
</script>
