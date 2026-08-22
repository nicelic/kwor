<template>
  <v-card style="background-color: inherit;">
    <v-row>
      <v-col cols="12" v-if="optionInbound">
        <v-combobox v-model="rule.inbound" :items="inTags" :label="$t('pages.inbounds')" multiple chips hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" v-if="optionClient">
        <v-combobox v-model="rule.auth_user" :items="clients" :label="$t('pages.clients')" multiple chips hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionIPver">
        <v-select v-model.number="rule.ip_version" :items="[4, 6]" :label="$t('rule.ipVer')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionQueryType">
        <v-combobox v-model="rule.query_type" :items="queryTypes" label="query_type" multiple chips hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" md="4" v-if="optionNetwork">
        <v-select v-model="rule.network" :items="networks" :label="$t('network')" multiple chips hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="optionProtocol">
        <v-combobox v-model="rule.protocol" :items="protocols" :label="$t('protocol')" multiple chips hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-row v-if="optionDomain">
      <v-col cols="12" sm="6" md="4">
        <v-select v-model="domainOption" :items="domainKeys" hide-details :disabled="disabled" @update:model-value="updateDomainOption($event)" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.domain !== undefined">
        <v-text-field v-model="domain" :label="$t('rule.domain') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.domain_suffix !== undefined">
        <v-text-field v-model="domain_suffix" :label="$t('rule.domainSufix') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.domain_keyword !== undefined">
        <v-text-field v-model="domain_keyword" :label="$t('rule.domainKw') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.domain_regex !== undefined">
        <v-text-field v-model="domain_regex" :label="$t('rule.domainRgx') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.ip_cidr !== undefined">
        <v-text-field v-model="ip_cidr" :label="$t('rule.ip') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.ip_is_private !== undefined">
        <v-switch v-model="rule.ip_is_private" color="primary" :label="$t('rule.privateIp')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.ip_accept_any !== undefined">
        <v-switch v-model="rule.ip_accept_any" color="primary" label="ip_accept_any" hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-row v-if="optionPort">
      <v-col cols="12" sm="6" md="4">
        <v-select v-model="portOption" :items="portKeys" hide-details :disabled="disabled" @update:model-value="updatePortOption($event)" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.port !== undefined">
        <v-text-field v-model="port" :label="$t('rule.port') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.port_range !== undefined">
        <v-text-field v-model="port_range" :label="$t('rule.portRange') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-row v-if="optionSrcIP">
      <v-col cols="12" sm="6" md="4">
        <v-select v-model="srcIPOption" :items="srcIPKeys" hide-details :disabled="disabled" @update:model-value="updateSrcIPOption($event)" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.source_ip_cidr !== undefined">
        <v-text-field v-model="source_ip_cidr" :label="$t('rule.srcCidr') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.source_ip_is_private !== undefined">
        <v-switch v-model="rule.source_ip_is_private" color="primary" :label="$t('rule.srcPrivateIp')" hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-row v-if="optionSrcPort">
      <v-col cols="12" sm="6" md="4">
        <v-select v-model="srcPortOption" :items="srcPortKeys" hide-details :disabled="disabled" @update:model-value="updateSrcPortOption($event)" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.source_port !== undefined">
        <v-text-field v-model="source_port" :label="$t('rule.srcPort') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.source_port_range !== undefined">
        <v-text-field v-model="source_port_range" :label="$t('rule.srcPortRange') + ' ' + $t('commaSeparated')" hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-row v-if="optionProcess">
      <v-col cols="12" sm="6" md="4">
        <v-select v-model="processOption" :items="processKeys" hide-details :disabled="disabled" @update:model-value="updateProcessOption($event)" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.process_name !== undefined">
        <v-text-field v-model="process_name" label="Process Name" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.process_path !== undefined">
        <v-text-field v-model="process_path" label="Process Path" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.process_path_regex !== undefined">
        <v-text-field v-model="process_path_regex" label="Process Path Regex" hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-row v-if="optionUser">
      <v-col cols="12" sm="6" v-if="rule.user !== undefined">
        <v-combobox v-model="rule.user" label="user" multiple chips hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.user_id !== undefined">
        <v-text-field v-model="user_id" label="UID" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.package_name !== undefined">
        <v-combobox v-model="rule.package_name" label="Package Name" multiple chips hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6" v-if="rule.clash_mode !== undefined">
        <v-combobox v-model="rule.clash_mode" :items="clashModes" label="Clash Mode" hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-row v-if="optionRuleSet">
      <v-col cols="12" sm="6">
        <v-combobox v-model="rule.rule_set" :items="ruleSets" :label="$t('rule.ruleset')" multiple chips hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6">
        <v-switch v-model="rule.rule_set_ip_cidr_match_source" color="primary" :label="$t('rule.rulesetMatchSrc')" hide-details :disabled="disabled" />
      </v-col>
      <v-col cols="12" sm="6">
        <v-switch v-model="rule.rule_set_ip_cidr_accept_empty" color="primary" label="rule_set_ip_cidr_accept_empty" hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-row v-if="optionMatchResponse">
      <v-col cols="12" sm="6" md="4">
        <v-switch v-model="rule.match_response" color="primary" label="match_response" hide-details :disabled="disabled" />
      </v-col>
    </v-row>

    <v-card-actions>
      <v-spacer />
      <v-menu v-model="menu" :close-on-content-click="false" location="start">
        <template v-slot:activator="{ props }">
          <v-btn v-bind="props" hide-details variant="tonal" :disabled="disabled">{{ $t('rule.options') }}</v-btn>
        </template>
        <v-card>
          <v-list>
            <v-list-item><v-switch v-model="optionInbound" color="primary" :label="$t('pages.inbounds')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionClient" color="primary" :label="$t('pages.clients')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionIPver" color="primary" :label="$t('rule.ipVer')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionQueryType" color="primary" label="query_type" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionNetwork" color="primary" :label="$t('network')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionProtocol" color="primary" :label="$t('protocol')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionDomain" color="primary" :label="$t('rule.domainRules')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionPort" color="primary" :label="$t('in.port')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionSrcIP" color="primary" :label="$t('rule.srcIpRules')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionSrcPort" color="primary" :label="$t('rule.srcPortRules')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionProcess" color="primary" label="Process Rules" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionUser" color="primary" label="User Rules" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionRuleSet" color="primary" :label="$t('rule.ruleset')" hide-details :disabled="disabled" /></v-list-item>
            <v-list-item><v-switch v-model="optionMatchResponse" color="primary" label="match_response" hide-details :disabled="disabled" /></v-list-item>
          </v-list>
        </v-card>
      </v-menu>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
export default {
  props: {
    rule: { type: Object, required: true },
    clients: { type: Array, default: () => [] },
    inTags: { type: Array, default: () => [] },
    ruleSets: { type: Array, default: () => [] },
    disabled: { type: Boolean, default: false },
  },
  data() {
    return {
      menu: false,
      domainKeys: ['domain', 'domain_suffix', 'domain_keyword', 'domain_regex', 'ip_cidr', 'ip_is_private', 'ip_accept_any'],
      portKeys: ['port', 'port_range'],
      srcIPKeys: ['source_ip_cidr', 'source_ip_is_private'],
      srcPortKeys: ['source_port', 'source_port_range'],
      processKeys: ['process_name', 'process_path', 'process_path_regex'],
      domainOption: 'domain',
      portOption: 'port',
      srcIPOption: 'source_ip_cidr',
      srcPortOption: 'source_port',
      processOption: 'process_name',
      queryTypes: ['A', 'AAAA', 'CNAME', 'MX', 'NS', 'PTR', 'SOA', 'SRV', 'TXT'],
      networks: ['tcp', 'udp', 'icmp'],
      protocols: ['http', 'tls', 'quic', 'stun', 'dns'],
      clashModes: ['global', 'direct', 'rule'],
    }
  },
  methods: {
    parseIntegerList(value: string): Array<number | string> {
      if (value.length === 0) return []
      return value.split(',').map((item: string) => {
        const token = item.trim()
        if (/^\d+$/.test(token)) {
          const parsed = Number(token)
          if (Number.isSafeInteger(parsed)) return parsed
        }
        return token
      })
    },
    updateDomainOption(option: string) {
      this.domainKeys.forEach(key => delete this.rule[key])
      this.rule[option] = option === 'ip_is_private' || option === 'ip_accept_any' ? false : []
    },
    updatePortOption(option: string) {
      this.portKeys.forEach(key => delete this.rule[key])
      this.rule[option] = []
    },
    updateSrcIPOption(option: string) {
      this.srcIPKeys.forEach(key => delete this.rule[key])
      this.rule[option] = option === 'source_ip_is_private' ? false : []
    },
    updateSrcPortOption(option: string) {
      this.srcPortKeys.forEach(key => delete this.rule[key])
      this.rule[option] = []
    },
    updateProcessOption(option: string) {
      this.processKeys.forEach(key => delete this.rule[key])
      this.rule[option] = []
    },
    syncOptionSelections() {
      const ruleKeys = Object.keys(this.rule ?? {})
      const selected = (keys: string[], fallback: string): string => keys.find(key => ruleKeys.includes(key)) ?? fallback
      this.domainOption = selected(this.domainKeys, 'domain')
      this.portOption = selected(this.portKeys, 'port')
      this.srcIPOption = selected(this.srcIPKeys, 'source_ip_cidr')
      this.srcPortOption = selected(this.srcPortKeys, 'source_port')
      this.processOption = selected(this.processKeys, 'process_name')
    },
  },
  computed: {
    optionInbound: {
      get(): boolean { return this.rule.inbound !== undefined },
      set(value: boolean) { this.rule.inbound = value ? [] : undefined },
    },
    optionClient: {
      get(): boolean { return this.rule.auth_user !== undefined },
      set(value: boolean) { this.rule.auth_user = value ? [] : undefined },
    },
    optionIPver: {
      get(): boolean { return this.rule.ip_version !== undefined },
      set(value: boolean) { this.rule.ip_version = value ? 4 : undefined },
    },
    optionQueryType: {
      get(): boolean { return this.rule.query_type !== undefined },
      set(value: boolean) { this.rule.query_type = value ? ['A'] : undefined },
    },
    optionNetwork: {
      get(): boolean { return this.rule.network !== undefined },
      set(value: boolean) { this.rule.network = value ? [] : undefined },
    },
    optionProtocol: {
      get(): boolean { return this.rule.protocol !== undefined },
      set(value: boolean) { this.rule.protocol = value ? ['dns'] : undefined },
    },
    optionDomain: {
      get(): boolean { return this.domainKeys.some(key => this.rule[key] !== undefined) },
      set(value: boolean) {
        if (value) this.rule.domain = []
        else this.domainKeys.forEach(key => delete this.rule[key])
        this.domainOption = 'domain'
      },
    },
    optionPort: {
      get(): boolean { return this.portKeys.some(key => this.rule[key] !== undefined) },
      set(value: boolean) {
        if (value) this.rule.port = []
        else this.portKeys.forEach(key => delete this.rule[key])
        this.portOption = 'port'
      },
    },
    optionSrcIP: {
      get(): boolean { return this.srcIPKeys.some(key => this.rule[key] !== undefined) },
      set(value: boolean) {
        if (value) this.rule.source_ip_cidr = []
        else this.srcIPKeys.forEach(key => delete this.rule[key])
        this.srcIPOption = 'source_ip_cidr'
      },
    },
    optionSrcPort: {
      get(): boolean { return this.srcPortKeys.some(key => this.rule[key] !== undefined) },
      set(value: boolean) {
        if (value) this.rule.source_port = []
        else this.srcPortKeys.forEach(key => delete this.rule[key])
        this.srcPortOption = 'source_port'
      },
    },
    optionProcess: {
      get(): boolean { return this.processKeys.some(key => this.rule[key] !== undefined) },
      set(value: boolean) {
        if (value) this.rule.process_name = []
        else this.processKeys.forEach(key => delete this.rule[key])
        this.processOption = 'process_name'
      },
    },
    optionUser: {
      get(): boolean { return ['user', 'user_id', 'package_name', 'clash_mode'].some(key => this.rule[key] !== undefined) },
      set(value: boolean) {
        if (value) this.rule.user = []
        else ['user', 'user_id', 'package_name', 'clash_mode'].forEach(key => delete this.rule[key])
      },
    },
    optionRuleSet: {
      get(): boolean {
        return this.rule.rule_set !== undefined ||
          this.rule.rule_set_ip_cidr_match_source !== undefined ||
          this.rule.rule_set_ip_cidr_accept_empty !== undefined
      },
      set(value: boolean) {
        if (value) {
          this.rule.rule_set = []
          this.rule.rule_set_ip_cidr_match_source = false
          this.rule.rule_set_ip_cidr_accept_empty = false
        } else {
          delete this.rule.rule_set
          delete this.rule.rule_set_ip_cidr_match_source
          delete this.rule.rule_set_ip_cidr_accept_empty
        }
      },
    },
    optionMatchResponse: {
      get(): boolean { return this.rule.match_response !== undefined },
      set(value: boolean) { this.rule.match_response = value ? false : undefined },
    },
    domain: {
      get(): string { return this.rule.domain?.join(',') ?? '' },
      set(value: string) { this.rule.domain = value.length > 0 ? value.split(',') : [] },
    },
    domain_suffix: {
      get(): string { return this.rule.domain_suffix?.join(',') ?? '' },
      set(value: string) { this.rule.domain_suffix = value.length > 0 ? value.split(',') : [] },
    },
    domain_keyword: {
      get(): string { return this.rule.domain_keyword?.join(',') ?? '' },
      set(value: string) { this.rule.domain_keyword = value.length > 0 ? value.split(',') : [] },
    },
    domain_regex: {
      get(): string { return this.rule.domain_regex?.join(',') ?? '' },
      set(value: string) { this.rule.domain_regex = value.length > 0 ? value.split(',') : [] },
    },
    ip_cidr: {
      get(): string { return this.rule.ip_cidr?.join(',') ?? '' },
      set(value: string) { this.rule.ip_cidr = value.length > 0 ? value.split(',') : [] },
    },
    port: {
      get(): string { return this.rule.port?.join(',') ?? '' },
      set(value: string) { if (!value.endsWith(',')) this.rule.port = this.parseIntegerList(value) },
    },
    port_range: {
      get(): string { return this.rule.port_range?.join(',') ?? '' },
      set(value: string) { this.rule.port_range = value.length > 0 ? value.split(',') : [] },
    },
    source_ip_cidr: {
      get(): string { return this.rule.source_ip_cidr?.join(',') ?? '' },
      set(value: string) { this.rule.source_ip_cidr = value.length > 0 ? value.split(',') : [] },
    },
    source_port: {
      get(): string { return this.rule.source_port?.join(',') ?? '' },
      set(value: string) { if (!value.endsWith(',')) this.rule.source_port = this.parseIntegerList(value) },
    },
    source_port_range: {
      get(): string { return this.rule.source_port_range?.join(',') ?? '' },
      set(value: string) { this.rule.source_port_range = value.length > 0 ? value.split(',') : [] },
    },
    process_name: {
      get(): string { return this.rule.process_name?.join(',') ?? '' },
      set(value: string) { this.rule.process_name = value.length > 0 ? value.split(',') : [] },
    },
    process_path: {
      get(): string { return this.rule.process_path?.join(',') ?? '' },
      set(value: string) { this.rule.process_path = value.length > 0 ? value.split(',') : [] },
    },
    process_path_regex: {
      get(): string { return this.rule.process_path_regex?.join(',') ?? '' },
      set(value: string) { this.rule.process_path_regex = value.length > 0 ? value.split(',') : [] },
    },
    user_id: {
      get(): string { return this.rule.user_id?.join(',') ?? '' },
      set(value: string) { if (!value.endsWith(',')) this.rule.user_id = this.parseIntegerList(value) },
    },
  },
  watch: {
    rule(newValue: unknown, oldValue: unknown) {
      if (newValue !== oldValue) this.syncOptionSelections()
    },
  },
  mounted() {
    this.syncOptionSelections()
  },
}
</script>
