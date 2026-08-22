<template>
  <v-dialog transition="dialog-bottom-transition" width="800" max-width="95vw">
    <v-card class="rounded-lg">
      <v-card-title>
        {{ $t('actions.' + title) + ' ' + $t('objects.ruleset') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 0 16px; max-height: 75vh; overflow-y: auto;">
        <v-row>
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="rule_set.type"
              hide-details
              :label="$t('type')"
              :items="typeItems"
              @update:model-value="updateType"
            ></v-select>
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
              v-model="rule_set.tag"
              :label="$t('objects.tag')"
              hide-details
            ></v-text-field>
          </v-col>
          <v-col v-if="showFormatField" cols="12" sm="6" md="4">
            <v-select
              v-model="rule_set.format"
              hide-details
              :label="$t('ruleset.format')"
              :items="formatItems"
              @update:model-value="updateFormat"
            ></v-select>
          </v-col>
          <v-col v-if="showBehaviorField" cols="12" sm="6" md="4">
            <v-select
              v-model="rule_set.behavior"
              hide-details
              label="Behavior"
              :items="behaviorItems"
            ></v-select>
          </v-col>
        </v-row>

        <v-row v-if="isMihomoNamespace && isInlineType">
          <v-col cols="12">
            <v-textarea
              v-model="payloadText"
              label="Payload"
              hide-details
              rows="6"
              placeholder="one item per line"
            ></v-textarea>
          </v-col>
        </v-row>

        <v-row v-else-if="isDefaultInlineType">
          <v-col cols="12">
            <v-textarea
              v-model="inlineRulesText"
              label="Inline Rules JSON"
              hide-details="auto"
              rows="9"
              :error-messages="inlineRulesError"
              @update:model-value="inlineRulesError = ''"
            ></v-textarea>
          </v-col>
        </v-row>

        <v-row v-else-if="isFileType">
          <v-col cols="12">
            <v-text-field
              v-model="rule_set.path"
              :label="$t('transport.path')"
              hide-details
            ></v-text-field>
          </v-col>
        </v-row>

        <v-row v-else>
          <v-col cols="12">
            <v-text-field
              v-model="rule_set.url"
              label="URL"
              hide-details
            ></v-text-field>
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="remoteProxy"
              hide-details
              :label="isMihomoNamespace ? 'Proxy' : $t('objects.outbound')"
              :items="outTags"
              clearable
              @click:clear="clearRemoteProxy"
            ></v-select>
          </v-col>
          <v-col v-if="isMihomoNamespace" cols="12" sm="6" md="4">
            <v-text-field
              v-model="rule_set.update_interval"
              hide-details
              label="Update interval"
              placeholder="24h"
            ></v-text-field>
          </v-col>
          <v-col v-else cols="12" sm="6" md="4">
            <v-text-field
              v-model="rule_set.update_interval"
              :label="$t('ruleset.interval')"
              placeholder="1d"
              hide-details
            ></v-text-field>
          </v-col>
        </v-row>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
        <v-btn
          color="primary"
          variant="outlined"
          @click="closeModal"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="loading"
          @click="saveChanges"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import RandomUtil from '@/plugins/randomUtil'
import { ruleset } from '@/types/rules'

const mihomoBehaviorValues = ['classical', 'domain', 'ipcidr']
const mihomoMrsBehaviorValues = ['domain', 'ipcidr']
const mihomoFormatValues = ['yaml', 'text', 'mrs']
const defaultFormatValues = ['source', 'binary']

function normalizePayloadEntries(raw: unknown): string[] | undefined {
  const values = Array.isArray(raw) ? raw : []
  const normalized = values
    .map((value) => typeof value === 'string' ? value.trim() : '')
    .filter((value) => value.length > 0)
  return normalized.length > 0 ? normalized : undefined
}

export default {
  props: {
    visible: Boolean,
    data: String,
    index: Number,
    outTags: Array,
    namespace: {
      type: String,
      default: 'default',
    },
  },
  emits: ['close', 'save'],
  data() {
    return {
      title: 'add',
      loading: false,
      rule_set: <ruleset>{},
      inlineRulesText: '[]',
      inlineRulesError: '',
    }
  },
  methods: {
    getDefaultDirectTag(): string {
      const tags = Array.isArray(this.$props.outTags)
        ? this.$props.outTags
          .filter((tag): tag is string => typeof tag === 'string')
          .map((tag) => tag.trim())
          .filter((tag) => tag.length > 0)
        : []
      const directTag = tags.find((tag) => tag.toLowerCase() === 'direct')
      return directTag ?? 'direct'
    },
    applyMihomoHttpDefaults(value: ruleset): ruleset {
      if (!this.isMihomoNamespace || value.type !== 'http') {
        return value
      }
      if (!value.proxy) {
        value.proxy = this.getDefaultDirectTag()
      }
      if (!value.update_interval) {
        value.update_interval = '24h'
      }
      return value
    },
    applyDefaultRemoteDefaults(value: ruleset): ruleset {
      if (this.isMihomoNamespace || value.type !== 'remote') {
        return value
      }
      const existingDetour = typeof value.http_client?.detour === 'string'
        ? value.http_client.detour.trim()
        : ''
      if (!existingDetour && !value.download_detour) {
        value.http_client = {
          ...(value.http_client ?? {}),
          detour: this.getDefaultDirectTag(),
        }
      }
      if (!value.update_interval) {
        value.update_interval = '1d'
      }
      return value
    },
    createDefaultRuleSet(): ruleset {
      const tag = `rs-${RandomUtil.randomSeq(3)}`
      if (this.isMihomoNamespace) {
        return {
          type: 'file',
          tag,
          format: 'yaml',
          behavior: 'classical',
        }
      }
      return {
        type: 'local',
        tag,
        format: 'binary',
      }
    },
    normalizeRuleSet(input: any): ruleset {
      const next = <ruleset>JSON.parse(JSON.stringify(input ?? {}))

      next.tag = typeof next.tag === 'string' ? next.tag.trim() : ''
      next.path = typeof next.path === 'string' ? next.path.trim() : undefined
      next.url = typeof next.url === 'string' ? next.url.trim() : undefined
      next.update_interval = typeof next.update_interval === 'string' ? next.update_interval.trim() : undefined
      next.proxy = typeof next.proxy === 'string' ? next.proxy.trim() : undefined
      next.download_detour = typeof next.download_detour === 'string' ? next.download_detour.trim() : undefined
      if (next.http_client && typeof next.http_client === 'object' && !Array.isArray(next.http_client)) {
        next.http_client = { ...next.http_client }
        if (typeof next.http_client.detour === 'string') {
          next.http_client.detour = next.http_client.detour.trim()
        }
      } else if (next.http_client !== undefined) {
        delete next.http_client
      }
      next.payload = normalizePayloadEntries(next.payload)

      if (this.isMihomoNamespace) {
        if (next.type === 'local') {
          next.type = 'file'
        } else if (next.type === 'remote') {
          next.type = 'http'
        } else if (next.type !== 'file' && next.type !== 'http' && next.type !== 'inline') {
          next.type = next.url ? 'http' : 'file'
        }

        if (!next.proxy && next.download_detour) {
          next.proxy = next.download_detour
        }
        delete next.download_detour

        if (next.type === 'inline') {
          delete next.path
          delete next.url
          delete next.proxy
          delete next.update_interval
          delete next.format
        } else {
          if (next.format === 'source') {
            next.format = 'yaml'
          } else if (next.format === 'binary') {
            next.format = 'mrs'
          } else if (!mihomoFormatValues.includes(next.format ?? '')) {
            next.format = 'yaml'
          }
        }

        const behavior = typeof next.behavior === 'string' ? next.behavior.trim().toLowerCase() : ''
        const allowedBehaviors = next.type === 'inline'
          ? mihomoBehaviorValues
          : (next.format === 'mrs' ? mihomoMrsBehaviorValues : mihomoBehaviorValues)
        next.behavior = <ruleset['behavior']>(allowedBehaviors.includes(behavior) ? behavior : allowedBehaviors[0])

        if (next.type === 'file') {
          delete next.url
          delete next.proxy
          delete next.update_interval
          delete next.payload
        } else if (next.type === 'http') {
          delete next.path
          delete next.payload
        }

        if (!next.path) delete next.path
        if (!next.url) delete next.url
        if (!next.proxy) delete next.proxy
        if (!next.update_interval) delete next.update_interval
        if (!next.payload?.length) delete next.payload

        return next
      }

      if (next.type === 'file') {
        next.type = 'local'
      } else if (next.type === 'http') {
        next.type = 'remote'
      } else if (next.type !== 'local' && next.type !== 'remote' && next.type !== 'inline') {
        next.type = next.url ? 'remote' : 'local'
      }

      if (next.format === 'yaml' || next.format === 'text') {
        next.format = 'source'
      } else if (next.format === 'mrs') {
        next.format = 'binary'
      } else if (!defaultFormatValues.includes(next.format ?? '')) {
        next.format = 'binary'
      }

      if (!next.download_detour && next.proxy) {
        next.download_detour = next.proxy
      }
      delete next.proxy
      delete next.behavior
      delete next.payload

      if (next.type === 'inline') {
        delete next.path
        delete next.url
        delete next.download_detour
        delete next.update_interval
        delete next.format
        delete next.initial_path
        delete next.http_client
        if (!Array.isArray(next.rules)) {
          next.rules = []
        }
      } else if (next.type === 'local') {
        delete next.url
        delete next.download_detour
        delete next.update_interval
        delete next.initial_path
        delete next.http_client
        delete next.rules
      } else {
        delete next.path
        delete next.rules
        if (!next.http_client?.detour && next.download_detour) {
          next.http_client = { ...(next.http_client ?? {}), detour: next.download_detour }
        }
        delete next.download_detour
      }

      if (!next.path) delete next.path
      if (!next.url) delete next.url
      if (!next.download_detour) delete next.download_detour
      if (next.http_client && Object.keys(next.http_client).length === 0) delete next.http_client
      if (!next.update_interval) delete next.update_interval

      return next
    },
    syncInlineRulesText() {
      const rules = Array.isArray(this.rule_set.rules) ? this.rule_set.rules : []
      this.inlineRulesText = JSON.stringify(rules, null, 2)
      this.inlineRulesError = ''
    },
    updateData() {
      if (this.$props.index != -1) {
        this.title = 'edit'
        this.rule_set = this.normalizeRuleSet(JSON.parse(this.$props.data ?? '{}'))
      } else {
        this.title = 'add'
        this.rule_set = this.createDefaultRuleSet()
      }
      this.syncInlineRulesText()
    },
    updateType(typeValue: string) {
      let next = this.normalizeRuleSet({
        ...this.rule_set,
        type: typeValue,
      })
      if (this.isMihomoNamespace && typeValue === 'http') {
        next = this.applyMihomoHttpDefaults(next)
      } else if (!this.isMihomoNamespace && typeValue === 'remote') {
        next = this.applyDefaultRemoteDefaults(next)
      }
      this.rule_set = next
      this.syncInlineRulesText()
    },
    updateFormat(formatValue: string) {
      this.rule_set = this.normalizeRuleSet({
        ...this.rule_set,
        format: formatValue,
      })
    },
    clearRemoteProxy() {
      if (this.isMihomoNamespace) {
        delete this.rule_set.proxy
        return
      }
      if (this.rule_set.http_client && typeof this.rule_set.http_client === 'object') {
        delete this.rule_set.http_client.detour
        if (Object.keys(this.rule_set.http_client).length === 0) delete this.rule_set.http_client
      }
    },
    closeModal() {
      this.$emit('close')
    },
    saveChanges() {
      this.loading = true
      try {
        if (!this.isMihomoNamespace && this.rule_set.type === 'inline') {
          let parsed: unknown
          try {
            parsed = JSON.parse(this.inlineRulesText)
          } catch {
            this.inlineRulesError = 'Inline rules must be valid JSON.'
            return
          }
          if (!Array.isArray(parsed)) {
            this.inlineRulesError = 'Inline rules must be a JSON array.'
            return
          }
          this.rule_set.rules = parsed
        }
        this.$emit('save', this.normalizeRuleSet(this.rule_set))
      } finally {
        this.loading = false
      }
    },
  },
  computed: {
    isMihomoNamespace(): boolean {
      return this.$props.namespace === 'mihomo'
    },
    isInlineType(): boolean {
      return this.rule_set.type === 'inline'
    },
    isDefaultInlineType(): boolean {
      return !this.isMihomoNamespace && this.isInlineType
    },
    isFileType(): boolean {
      return this.rule_set.type === 'local' || this.rule_set.type === 'file'
    },
    showFormatField(): boolean {
      return !this.isInlineType
    },
    showBehaviorField(): boolean {
      return this.isMihomoNamespace
    },
    typeItems(): { title: string; value: string }[] {
      return this.isMihomoNamespace
        ? [
          { title: 'file', value: 'file' },
          { title: 'http', value: 'http' },
          { title: 'inline', value: 'inline' },
        ]
        : [
          { title: this.$t('ruleset.local'), value: 'local' },
          { title: this.$t('ruleset.remote'), value: 'remote' },
          { title: 'Inline', value: 'inline' },
        ]
    },
    formatItems(): string[] {
      return this.isMihomoNamespace ? mihomoFormatValues : defaultFormatValues
    },
    behaviorItems(): string[] {
      if (!this.isMihomoNamespace || this.isInlineType) {
        return mihomoBehaviorValues
      }
      return this.rule_set.format === 'mrs' ? mihomoMrsBehaviorValues : mihomoBehaviorValues
    },
    payloadText: {
      get(): string {
        return Array.isArray(this.rule_set.payload) ? this.rule_set.payload.join('\n') : ''
      },
      set(value: string) {
        const lines = value
          .split('\n')
          .map((line) => line.trim())
          .filter((line) => line.length > 0)
        this.rule_set.payload = lines.length > 0 ? lines : undefined
      },
    },
    remoteProxy: {
      get(): string | undefined {
        if (this.isMihomoNamespace) return this.rule_set.proxy
        const httpClient = this.rule_set.http_client
        return typeof httpClient?.detour === 'string'
          ? httpClient.detour
          : this.rule_set.download_detour
      },
      set(value: string | undefined) {
        const normalized = typeof value === 'string' && value.trim() !== '' ? value.trim() : undefined
        if (this.isMihomoNamespace) {
          this.rule_set.proxy = normalized
          return
        }
        if (!normalized) {
          this.clearRemoteProxy()
          return
        }
        this.rule_set.http_client = {
          ...(this.rule_set.http_client ?? {}),
          detour: normalized,
        }
        delete this.rule_set.download_detour
      },
    },
  },
  watch: {
    visible(newValue: boolean) {
      if (newValue) {
        this.updateData()
      }
    },
  },
}
</script>
