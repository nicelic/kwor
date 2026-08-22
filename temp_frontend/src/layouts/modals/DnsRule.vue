<template>
  <v-dialog transition="dialog-bottom-transition" width="800" max-width="95vw" max-height="90vh" :persistent="busy" v-model="dialogVisible" @update:model-value="handleDialogVisibility">
    <v-card class="rounded-lg">
      <v-card-title>
        {{ $t('actions.' + title) + " " + $t('objects.dnsrule') }}
      </v-card-title>
      <v-divider></v-divider>
      <v-card-text style="padding: 0 16px; max-height: 75vh; overflow-y: auto;">
        <div :style="{ pointerEvents: busy ? 'none' : 'auto' }" :aria-busy="busy">
        <v-row>
          <v-col cols="12" sm="6" md="4">
            <v-switch color="primary" v-model="logical" :label="$t('rule.logical')" hide-details :disabled="busy"></v-switch>
          </v-col>
          <v-spacer></v-spacer>
          <v-col cols="auto" v-if="logical" justify="center" align="center">
            <v-btn color="primary" @click="ruleData.rules.push(<dnsRule>{})" hide-details :disabled="busy || ruleData.rules.length >= 16">{{ $t('actions.add') + " " + $t('objects.rule') }}</v-btn>
          </v-col>
        </v-row>
        <v-card style="background-color: inherit; margin-bottom: 5px;" v-for="(r, index) in ruleData.rules" :key="`dns-rule-child-${index}`" v-if="ruleData.type == 'logical'">
          <v-card-subtitle>{{ $t('objects.rule') + ' ' + (Number(index)+1) }}
            <v-icon @click="ruleData.rules.splice(index,1)" icon="mdi-delete" v-if="ruleData.rules.length>1" :disabled="busy" />
          </v-card-subtitle>
          <v-card-text style="padding: 0;">
            <RuleOptions
              :rule="r"
              :clients="clients"
              :inTags="inTags"
              :ruleSets="ruleSets"
              :disabled="busy" />
          </v-card-text>
        </v-card>
        <RuleOptions
          v-else
          :rule="ruleData.rules[0]"
          :clients="clients"
          :inTags="inTags"
          :ruleSets="ruleSets"
          :disabled="busy" />
        <v-row>
          <v-col cols="12" sm="6" md="4">
            <v-select
              v-model="ruleData.action"
              :items="actions"
              :label="$t('dns.rule.action.title')"
              :disabled="busy"
              hide-details
            ></v-select>
          </v-col>
          <v-col cols="12" sm="6" md="4" v-if="logical">
            <v-combobox
              v-model="ruleData.mode"
              :items="['and', 'or']"
              :label="$t('rule.mode')"
              :disabled="busy"
              hide-details
            ></v-combobox>
          </v-col>
          <v-col cols="12" sm="6" md="4">
            <v-switch color="primary" v-model="ruleData.invert" :label="$t('rule.invert')" :disabled="busy" hide-details></v-switch>
          </v-col>
        </v-row>
        <v-card :subtitle="$t('dns.rule.action.route')" v-if="['route', 'route-options'].includes(ruleData.action)">
          <v-row v-if="ruleData.action == 'route'">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="ruleData.server"
                :items="serverTags"
                :label="$t('dns.server')"
                :disabled="busy"
                hide-details
              ></v-select>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="ruleData.strategy"
                :items="strategies"
                :label="$t('rule.strategy')"
                clearable
                @click:clear="delete ruleData.strategy"
                :disabled="busy"
                hide-details>
              </v-select>
            </v-col>
          </v-row>
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-switch v-model="ruleData.disable_cache" :label="$t('dns.disableCache')" :disabled="busy" hide-details></v-switch>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model.number="ruleData.rewrite_ttl" type="number" min="0" :label="$t('dns.rule.action.rewriteTtl')" :disabled="busy" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field v-model="ruleData.client_subnet" :label="$t('dns.rule.action.clientSubnet')" :disabled="busy" hide-details></v-text-field>
            </v-col>
          </v-row>
        </v-card>
        <v-card :subtitle="$t('dns.rule.action.reject')" v-if="ruleData.action == 'reject'">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="ruleData.method"
                :items="[{ title: 'Default', value: 'default' },{ title: 'Drop', value: 'drop'}]"
                :label="$t('rule.method')"
                clearable
                @click:clear="delete ruleData.method"
                :disabled="busy"
                hide-details>
            </v-select>
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-switch v-model="ruleData.no_drop" :label="$t('rule.noDrop')" :disabled="busy || ruleData.method === 'drop'" hide-details></v-switch>
            </v-col>
          </v-row>
        </v-card>
        <v-card :subtitle="$t('dns.rule.action.predefined')" v-if="ruleData.action == 'predefined'">
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="ruleData.rcode"
                :items="predefinedRcode"
                :label="$t('dns.rule.action.rcode')"
                :disabled="busy"
                clearable
                @click:clear="delete ruleData.rcode"
                hide-details>
              </v-select>
            </v-col>
          </v-row>
          <v-row v-if="ruleData.rcode == 'NOERROR'">
            <v-col cols="12" sm="8">
              <v-text-field v-model="answer" :label="$t('dns.rule.action.answer') + ' ' + $t('commaSeparated')" :disabled="busy" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="8">
              <v-text-field v-model="ns" :label="$t('dns.rule.action.ns') + ' ' + $t('commaSeparated')" :disabled="busy" hide-details></v-text-field>
            </v-col>
            <v-col cols="12" sm="8">
              <v-text-field v-model="extra" :label="$t('dns.rule.action.extra') + ' ' + $t('commaSeparated')" :disabled="busy" hide-details></v-text-field>
            </v-col>
          </v-row>
        </v-card>
        </div>
      </v-card-text>
      <v-card-actions>
        <v-spacer></v-spacer>
          <v-btn
            color="primary"
            variant="outlined"
            @click="closeModal"
            :disabled="busy"
        >
          {{ $t('actions.close') }}
        </v-btn>
        <v-btn
          color="primary"
          variant="tonal"
          :loading="busy"
          @click="saveChanges"
          :disabled="busy"
        >
          {{ $t('actions.save') }}
        </v-btn>
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

<script lang="ts">
import { logicalDnsRule, dnsRule, actionDnsRuleKeys } from '@/types/dns'
import RuleOptions from '@/components/DnsRule.vue'
import { i18n } from '@/locales'
export default {
  props: ['visible', 'data', 'index', 'clients', 'inTags', 'serverTags', 'ruleSets', 'busy'],
  emits: ['close', 'save'],
  data() {
    return {
      title: 'add',
      dialogVisible: false,
      ruleData: <any>{
        type: 'logical',
        mode: 'and',
        rules: <dnsRule[]>[{}],
        invert: false,
        action: 'route',
        server: 'local',
      },
      actions: [
        { title: i18n.global.t('dns.rule.action.route'), value: 'route'},
        { title: i18n.global.t('dns.rule.action.routeOptions'), value: 'route-options'},
        { title: i18n.global.t('dns.rule.action.reject'), value: 'reject'},
        { title: i18n.global.t('dns.rule.action.predefined'), value: 'predefined'},
      ],
      strategies: [
        { title: 'Prefer IPv4', value: 'prefer_ipv4' },
        { title: 'Prefer IPv6', value: 'prefer_ipv6' },
        { title: 'IPv4 Only', value: 'ipv4_only' },
        { title: 'IPv6 Only', value: 'ipv6_only' },
      ],
      predefinedRcode: [
        { title: i18n.global.t('dns.rule.action.rcodes.noError'), value: 'NOERROR' },
        { title: i18n.global.t('dns.rule.action.rcodes.formerr'), value: 'FORMERR' },
        { title: i18n.global.t('dns.rule.action.rcodes.servFail'), value: 'SERVFAIL' },
        { title: i18n.global.t('dns.rule.action.rcodes.nxDomain'), value: 'NXDOMAIN' },
        { title: i18n.global.t('dns.rule.action.rcodes.notImp'), value: 'NOTIMP' },
        { title: i18n.global.t('dns.rule.action.rcodes.refused'), value: 'REFUSED' },
      ],
    }
  },
  methods: {
    updateData() {
      if (this.$props.index != -1) {
        let newData: any = {}
        try {
          newData = JSON.parse(this.$props.data)
        } catch {
          newData = {}
        }
        if (newData.type) {
          this.ruleData = {
            ...newData,
            mode: newData.mode === 'or' ? 'or' : 'and',
            rules: Array.isArray(newData.rules) && newData.rules.length > 0 ? newData.rules : [{}],
          }
        } else {
          this.ruleData = {
            type: 'simple',
            mode: 'and',
            rules: <dnsRule[]>[{}],
          }
          Object.keys(newData).forEach(key => {
            if (actionDnsRuleKeys.includes(key)) {
              this.ruleData[key] = newData[key]
            } else {
              this.ruleData.rules[0][key] = newData[key]
            }
          })
        }
        this.title = 'edit'
      }
      else {
        this.ruleData = <logicalDnsRule>{
            type: 'simple',
            mode: 'and',
            rules: <dnsRule[]>[{}],
            invert: false,
            action: 'route',
            server: this.$props.serverTags[0]?? 'local',
          }
        this.title = 'add'
      }
    },
    closeModal() {
      if (this.busy) return
      this.$emit('close')
    },
    handleDialogVisibility(value: boolean) {
      if (!value && this.$props.visible && !this.busy) this.closeModal()
    },
    saveChanges() {
      if (this.busy) return
      let newRule = <any>{
        action: this.ruleData.action,
        invert: this.ruleData.invert? this.ruleData.invert : undefined,
      }

      // Filter action data
      switch (newRule.action){
        case 'route':
          newRule.server = this.ruleData.server
          newRule.strategy = this.ruleData.strategy?.length > 0 ? this.ruleData.strategy : undefined
          newRule.disable_cache = this.ruleData.disable_cache? true : undefined
          newRule.rewrite_ttl = this.ruleData.rewrite_ttl !== undefined && this.ruleData.rewrite_ttl !== null && this.ruleData.rewrite_ttl !== '' ? this.ruleData.rewrite_ttl : undefined
          newRule.client_subnet = this.ruleData.client_subnet?.length > 0 ? this.ruleData.client_subnet : undefined
          break
        case 'route-options':
          newRule.disable_cache = this.ruleData.disable_cache? true : undefined
          newRule.rewrite_ttl = this.ruleData.rewrite_ttl !== undefined && this.ruleData.rewrite_ttl !== null && this.ruleData.rewrite_ttl !== '' ? this.ruleData.rewrite_ttl : undefined
          newRule.client_subnet = this.ruleData.client_subnet?.length > 0 ? this.ruleData.client_subnet : undefined
          break
        case 'reject':
          newRule.method = this.ruleData.method?.length > 0 ? this.ruleData.method : undefined
          newRule.no_drop = this.ruleData.method === 'drop' ? undefined : (this.ruleData.no_drop? true : undefined)
          break
        case 'predefined':
          newRule.rcode = this.ruleData.rcode?.length > 0 ? this.ruleData.rcode : undefined
          if (this.ruleData.rcode == 'NOERROR') {
            const asList = (value: unknown): string[] | undefined => {
              if (Array.isArray(value)) return value.map(item => String(item).trim()).filter(Boolean)
              const text = String(value ?? '').trim()
              return text ? text.split(',').map(item => item.trim()).filter(Boolean) : undefined
            }
            newRule.answer = asList(this.ruleData.answer)
            newRule.ns = asList(this.ruleData.ns)
            newRule.extra = asList(this.ruleData.extra)
          }
          break
      }

      // Add rules
      if (this.ruleData.type == 'simple'){
        const conditionRule = this.ruleData.rules[0] ?? {}
        const conditionFields = Object.fromEntries(
          Object.entries(conditionRule).filter(([key]) => !actionDnsRuleKeys.includes(key))
        )
        newRule = { ...conditionFields, ...newRule }
      } else {
        const logicalConditionFields = Object.fromEntries(
          Object.entries(this.ruleData).filter(([key]) =>
            !actionDnsRuleKeys.includes(key) && !['type', 'mode', 'rules'].includes(key)
          )
        )
        newRule.type = 'logical'
        newRule.mode = this.ruleData.mode
        newRule.rules = this.ruleData.rules.map((rule: any) =>
          Object.fromEntries(Object.entries(rule ?? {}).filter(([key]) => !actionDnsRuleKeys.includes(key)))
        )
        newRule = { ...logicalConditionFields, ...newRule }
      }
      this.$emit('save', newRule)
    },
    deleteRule(index:number) {
      this.ruleData.rules.splice(index,1)
    }
  },
  computed: {
    logical: {
      get() { return this.ruleData.type == 'logical' },
      set(v:boolean) {
        this.ruleData.type = v? 'logical' : 'simple'
      }
    },
    answer: {
      get() { return this.ruleData.answer?.length > 0 ? this.ruleData.answer.join(',') : "" },
      set(v:string) { this.ruleData.answer = v.length > 0 ? v.split(',') : undefined }
    },
    ns: {
      get() { return this.ruleData.ns?.length > 0 ? this.ruleData.ns.join(',') : "" },
      set(v:string) { this.ruleData.ns = v.length > 0 ? v.split(',') : undefined }
    },
    extra: {
      get() { return this.ruleData.extra?.length > 0 ? this.ruleData.extra.join(',') : "" },
      set(v:string) { this.ruleData.extra = v.length > 0 ? v.split(',') : undefined }
    },
  },
  watch: {
    visible(newValue) {
      if (newValue) {
        this.updateData()
        this.dialogVisible = true
      } else {
        this.dialogVisible = false
      }
    },
  },
  components: { RuleOptions }
}

</script>
