<template>
  <Editor
	v-if="enableEditor"
    v-model="enableEditor"
    :data="editorData"
    :visible="enableEditor"
    :title="$t('editor') + ' - ' + $t('setting.clashSub')"
    @close="enableEditor = false"
    @save="saveEditor"
    />
	<v-card @input.capture="onFormValueChange" @change.capture="onFormValueChange">
	  <v-alert v-if="parseError" type="error" variant="tonal" density="compact" class="mb-4">
	    {{ parseError }}
	  </v-alert>
    <!-- Basic settings: mixed port, LAN access, external controller, log level -->
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field type="number" v-model.number="mixedPort" min="1" max="65535" :label="$t('setting.mixedPort')" hide-details @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch color="primary" v-model="allowLan" :label="$t('types.ts.allowLanAccess')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field v-model="externalController" :label="$t('basic.exp.extController')" hide-details @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select v-model="logLevel" :items="clashLogLevels" :label="$t('basic.log.title') + ' - ' + $t('basic.log.level')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <!-- mihomo-specific settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
		<v-switch v-model="unifiedDelay" color="primary" :label="$t('subscriptionEditor.unifiedDelay')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2">
		<v-switch v-model="tcpConcurrent" color="primary" :label="$t('subscriptionEditor.tcpConcurrent')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="3" lg="2">
		<v-select v-model="findProcessMode" :items="findProcessModeOptions" :label="$t('subscriptionEditor.processMatchMode')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
		<v-switch v-model="storeSelected" color="primary" :label="$t('subscriptionEditor.storeSelected')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2">
		<v-switch v-model="storeFakeIp" color="primary" :label="$t('subscriptionEditor.storeFakeIp')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="dnsEnabled && dnsEnhancedMode === 'fake-ip'">
        <v-text-field
          v-model.lazy="dnsFakeIpTtl"
          :label="$t('subscriptionEditor.fakeIpTtlLabel')"
          :placeholder="$t('subscriptionEditor.fakeIpTtlPlaceholder')"
          hide-details
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
    </v-row>

    <!-- TUN settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="tunEnabled" color="primary" :label="$t('setting.tun')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="tunEnabled">
		<v-switch v-model="tunAutoRoute" color="primary" :label="$t('subscriptionEditor.autoRoute')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="tunEnabled && tunAutoRoute">
		<v-switch v-model="tunStrictRoute" color="primary" :label="$t('subscriptionEditor.strictRoute')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3" lg="2">
		<v-select v-model="tunStack" :items="tunStackOptions" :label="$t('subscriptionEditor.tunMode')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field type="number" v-model.number="tunMtu" hide-details label="MTU" @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3" lg="3">
        <v-select
          v-model="tunAutoDetectInterface"
          :items="optionalBoolOptions"
		  :label="$t('subscriptionEditor.autoDetectInterface')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3" lg="3">
        <v-select
          v-model="tunRecvmsgx"
          :items="optionalBoolOptions"
		  :label="$t('subscriptionEditor.recvmsgx')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3" lg="3">
        <v-select
          v-model="tunSendmsgx"
          :items="optionalBoolOptions"
		  :label="$t('subscriptionEditor.sendmsgx')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="4" lg="3">
        <v-combobox
          v-model="tunInet4Address"
          :items="tunInet4AddressOptions"
          label="inet4-address"
          multiple
          chips
          closable-chips
          clearable
          hide-details
          placeholder="198.18.0.1/30"
         @update:model-value="onFormValueChange"></v-combobox>
      </v-col>
      <v-col cols="12" sm="6" md="4" lg="3">
        <v-combobox
          v-model="tunInet6Address"
          :items="tunInet6AddressOptions"
          label="inet6-address"
          multiple
          chips
          closable-chips
          clearable
          hide-details
          placeholder="fdfe:dcba:9876::1/126"
         @update:model-value="onFormValueChange"></v-combobox>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="3">
        <v-select
          v-model="globalIpv6"
          :items="optionalBoolOptions"
		  :label="$t('subscriptionEditor.globalIpv6')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <!-- DNS settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="dnsEnabled" color="primary" :label="$t('pages.dns')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="dnsEnabled">
        <v-switch v-model="dnsIpv6" color="primary" label="DNS_IPv6" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="dnsEnabled">
        <v-switch v-model="dnsPreferH3" color="primary" label="prefer-h3" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>
    <template v-if="dnsEnabled">
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
		  <v-select v-model="dnsEnhancedMode" :items="enhancedModeOptions" :label="$t('subscriptionEditor.enhancedMode')" hide-details @update:model-value="onFormValueChange"></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="dnsEnhancedMode === 'fake-ip'">
          <v-combobox
            v-model="dnsFakeIpRange"
            :items="dnsFakeIpRangeOptions"
            label="fake-ip (fake-ip-range)"
            clearable
            hide-details
            placeholder="198.18.0.1/15"
           @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="dnsEnhancedMode === 'fake-ip'">
          <v-combobox
            v-model="dnsFakeIpRange6"
            :items="dnsFakeIpRange6Options"
            label="fake-ip6 (fake-ip-range6)"
            clearable
            hide-details
            placeholder="fc00::/18"
           @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="dnsEnhancedMode === 'fake-ip' && dnsIpv6">
          <v-text-field
            type="number"
            v-model="dnsIpv6Timeout"
            min="0"
            label="ipv6-timeout"
            placeholder="100"
            hide-details
           @update:model-value="onFormValueChange"></v-text-field>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsDirectNameserver" :items="clashDirectNameserverOptions" label="(direct-nameserver)" multiple chips closable-chips hide-details @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="dnsDirectNameserver.length > 0">
          <v-switch v-model="dnsDirectNameserverFollowPolicy" color="primary" label="direct-nameserver-follow-policy" hide-details  @update:model-value="onFormValueChange"/>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsProxyServerNameserver" :items="clashProxyServerNameserverOptions" label="(proxy-server-nameserver)" multiple chips closable-chips hide-details @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsNameserver" :items="clashNameserverOptions" label="(nameserver)" multiple chips closable-chips hide-details @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsFallback" :items="clashFallbackOptions" label="(fallback)" multiple chips closable-chips hide-details @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsDefaultNameserver" :items="clashDefaultNameserverOptions" label="(default-nameserver)" multiple chips closable-chips hide-details @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="dnsEnhancedMode === 'fake-ip'">
		  <v-combobox v-model="dnsFakeIpFilter" :items="clashFakeIpFilterDefaults" :label="$t('subscriptionEditor.fakeIpFilter')" multiple chips closable-chips hide-details @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-select
            v-model="dnsFallbackFilterGeoip"
            :items="dnsGeoipBoolOptions"
            label="fallback-filter.geoip"
            hide-details
           @update:model-value="onFormValueChange"></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-combobox
            v-model="dnsFallbackFilterGeoipCode"
            :items="clashGeoipCodeOptions"
            label="fallback-filter.geoip-code"
            hide-details
            clearable
           @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox
            v-model="dnsFallbackFilterIpcidr"
            :items="[]"
            label="fallback-filter.ipcidr"
            multiple
            chips
            closable-chips
            hide-details
           @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox
            v-model="dnsFallbackFilterDomain"
            :items="[]"
            label="fallback-filter.domain"
            multiple
            chips
            closable-chips
            hide-details
           @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
      </v-row>
      <v-row
        v-for="(dnsSuffixRow, dnsSuffixIdx) in clashDnsSuffixRows"
		:key="dnsSuffixRow.id"
        align="center"
      >
        <v-col cols="12" sm="6" md="4">
          <v-select
            v-model="dnsSuffixRow.targets"
            :items="clashDnsSuffixTargetOptions"
			:label="$t('subscriptionEditor.dnsSelection')"
            multiple
            chips
            closable-chips
            clearable
            hide-details
           @update:model-value="onFormValueChange"></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            v-model="dnsSuffixRow.selections"
            :items="clashDnsSuffixSelectionOptions"
			:label="$t('subscriptionEditor.dnsSuffix')"
            multiple
            chips
            closable-chips
            clearable
            hide-details
           @update:model-value="onFormValueChange"></v-select>
        </v-col>
		<v-col cols="12" class="subscription-row-actions">
          <div class="d-flex align-center justify-end ga-1">
            <v-btn
              icon="mdi-arrow-up"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.moveUp')"
			  :aria-label="$t('subscriptionEditor.moveUp')"
              size="small"
              variant="text"
              :disabled="dnsSuffixIdx === 0"
			  @click="onFormValueChange(); moveClashDnsSuffixRow(dnsSuffixIdx, -1)"
            ></v-btn>
            <v-btn
              icon="mdi-arrow-down"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.moveDown')"
			  :aria-label="$t('subscriptionEditor.moveDown')"
              size="small"
              variant="text"
              :disabled="dnsSuffixIdx >= clashDnsSuffixRows.length - 1"
			  @click="onFormValueChange(); moveClashDnsSuffixRow(dnsSuffixIdx, 1)"
            ></v-btn>
            <v-btn
              icon="mdi-plus"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.add')"
			  :aria-label="$t('subscriptionEditor.add')"
              size="small"
              variant="text"
			  @click="onFormValueChange(); insertClashDnsSuffixRow(dnsSuffixIdx)"
            ></v-btn>
            <v-btn
              v-if="canDeleteClashDnsSuffixRow(dnsSuffixIdx)"
              icon="mdi-delete"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.remove')"
			  :aria-label="$t('subscriptionEditor.remove')"
              size="small"
              variant="text"
			  @click="onFormValueChange(); removeClashDnsSuffixRow(dnsSuffixIdx)"
            ></v-btn>
          </div>
        </v-col>
      </v-row>
      <v-row
        v-for="(dnsPolicyRow, dnsPolicyIdx) in clashDnsPolicyRows"
		:key="dnsPolicyRow.id"
        align="center"
      >
        <v-col cols="12" sm="6" md="2">
          <v-select
            v-model="dnsPolicyRow.matchType"
            :items="clashDnsPolicyMatchTypeOptions"
			:label="$t('subscriptionEditor.ruleKind')"
            hide-details
           @update:model-value="onFormValueChange"></v-select>
        </v-col>
        <v-col cols="12" sm="8" md="4">
          <v-combobox
            v-if="dnsPolicyRow.matchType !== 'rule-set'"
            v-model="dnsPolicyRow.values"
            :items="getClashDnsPolicyValueOptions(dnsPolicyRow)"
			:label="dnsPolicyRow.matchType === 'rule-set' ? $t('subscriptionEditor.ruleSet') : $t('subscriptionEditor.matchValue')"
            multiple
            chips
            closable-chips
            hide-details
           @update:model-value="onFormValueChange"></v-combobox>
          <v-select
            v-else
            v-model="dnsPolicyRow.values"
            :items="getClashDnsPolicyValueOptions(dnsPolicyRow)"
			:label="$t('subscriptionEditor.ruleSet')"
            multiple
            chips
            closable-chips
            hide-details
           @update:model-value="onFormValueChange"></v-select>
        </v-col>
        <v-col cols="12" sm="4" md="2">
          <v-select
            v-model="dnsPolicyRow.routeTarget"
            :items="clashDnsPolicyRouteOptions"
			:label="$t('subscriptionEditor.dnsRoute')"
            hide-details
           @update:model-value="onFormValueChange"></v-select>
        </v-col>
		<v-col cols="12" class="subscription-row-actions">
          <div class="d-flex align-center justify-end ga-1">
            <v-btn
              icon="mdi-arrow-up"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.moveUp')"
			  :aria-label="$t('subscriptionEditor.moveUp')"
              size="small"
              variant="text"
              :disabled="dnsPolicyIdx === 0"
			  @click="onFormValueChange(); moveClashDnsPolicyRow(dnsPolicyIdx, -1)"
            ></v-btn>
            <v-btn
              icon="mdi-arrow-down"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.moveDown')"
			  :aria-label="$t('subscriptionEditor.moveDown')"
              size="small"
              variant="text"
              :disabled="dnsPolicyIdx >= clashDnsPolicyRows.length - 1"
			  @click="onFormValueChange(); moveClashDnsPolicyRow(dnsPolicyIdx, 1)"
            ></v-btn>
            <v-btn
              icon="mdi-plus"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.add')"
			  :aria-label="$t('subscriptionEditor.add')"
              size="small"
              variant="text"
			  @click="onFormValueChange(); insertClashDnsPolicyRow(dnsPolicyIdx)"
            ></v-btn>
            <v-btn
              v-if="canDeleteClashDnsPolicyRow(dnsPolicyIdx)"
              icon="mdi-delete"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.remove')"
			  :aria-label="$t('subscriptionEditor.remove')"
              size="small"
              variant="text"
			  @click="onFormValueChange(); removeClashDnsPolicyRow(dnsPolicyIdx)"
            ></v-btn>
          </div>
        </v-col>
      </v-row>
    </template>

    <!-- Excluded packages (TUN only) -->
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3">
        <v-combobox
          v-model="tunExcludePackage"
          :items="['ir.mci.ecareapp','com.myirancell']"
          chips
          multiple
          hide-details
          :label="$t('setting.excludePkg')"
         @update:model-value="onFormValueChange"></v-combobox>
      </v-col>
    </v-row>

    <!-- Rule set source -->
    <v-row>
      <v-col cols="12" sm="6" md="3">
		<v-select v-model="ruleSetSource" :items="clashRuleSetSourceOptions" :label="$t('subscriptionEditor.globalRuleSetSource')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <!-- Unified custom/ruleset rows -->
    <v-row
      v-for="(row, idx) in clashRuleRows"
	  :key="row.id"
      align="center"
    >
      <v-col cols="12" sm="3" md="2">
        <v-text-field
          v-model="row.name"
		  :label="idx === 0 ? $t('subscriptionEditor.optionalName') : $t('subscriptionEditor.name')"
		  :hint="idx === 0 ? $t('subscriptionEditor.ruleNameHelp') : ''"
          :persistent-hint="idx === 0"
          hide-details="auto"
          :placeholder="$t('subscriptionEditor.exampleCN')"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-select
          v-model="row.kind"
          :items="clashRuleKindOptions"
		  :label="$t('subscriptionEditor.ruleKind')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-select
          v-if="row.kind === 'custom'"
          v-model="row.customType"
          :items="clashDomainIpTypes"
		  :label="idx === 0 ? $t('subscriptionEditor.customMatchType') : $t('subscriptionEditor.matchType')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
        <v-select
          v-else
          v-model="row.ruleSetScope"
          :items="clashRuleSetScopeOptions"
		  :label="$t('subscriptionEditor.ruleSetScope')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2" v-if="row.kind === 'ruleset'">
        <v-select
          v-model="row.ruleSetSourceOverride"
		  :items="getClashRuleSetSourceOverrideOptions(row.ruleSetScope)"
		  :label="$t('subscriptionEditor.ruleSetSource')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="4">
        <v-combobox
          v-model="row.values"
          :items="row.kind === 'ruleset' ? getClashRuleSetNameOptions(row.ruleSetScope, row) : []"
          :label="row.kind === 'custom' ? getTypeLabel(row.customType) : getRuleSetScopeLabel(row.ruleSetScope)"
          hide-details
          multiple
          chips
          closable-chips
         @update:model-value="onFormValueChange"></v-combobox>
      </v-col>
      <v-col cols="12" sm="4" md="2">
        <v-select
          v-model="row.route"
          :items="clashCustomRouteOptions"
		  :label="row.name && row.name.trim() ? $t('subscriptionEditor.routeDisabledByName') : $t('subscriptionEditor.route')"
          :disabled="Boolean(row.name && row.name.trim())"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="4" md="2">
        <v-select
          :model-value="getClashRowNoResolveDisplayValue(row)"
          :items="dnsGeoipBoolOptions"
          label="no-resolve"
          :disabled="isClashRowNoResolveDisabled(row)"
          hide-details
		  @update:modelValue="setClashRowNoResolve(row, $event); onFormValueChange()"
        ></v-select>
      </v-col>
	  <v-col cols="12" class="subscription-row-actions">
        <div class="d-flex align-center justify-end ga-1">
          <v-btn
            icon="mdi-arrow-up"
			class="subscription-row-action"
			:title="$t('subscriptionEditor.moveUp')"
			:aria-label="$t('subscriptionEditor.moveUp')"
            size="small"
            variant="text"
            :disabled="idx === 0"
			@click="onFormValueChange(); moveClashRuleRow(idx, -1)"
          ></v-btn>
          <v-btn
            icon="mdi-arrow-down"
			class="subscription-row-action"
			:title="$t('subscriptionEditor.moveDown')"
			:aria-label="$t('subscriptionEditor.moveDown')"
            size="small"
            variant="text"
            :disabled="idx >= clashRuleRows.length - 1"
			@click="onFormValueChange(); moveClashRuleRow(idx, 1)"
          ></v-btn>
          <v-btn
            icon="mdi-plus"
			class="subscription-row-action"
			:title="$t('subscriptionEditor.add')"
			:aria-label="$t('subscriptionEditor.add')"
            size="small"
            variant="text"
			@click="onFormValueChange(); insertClashRuleRow(idx)"
          ></v-btn>
          <v-btn
            v-if="canDeleteClashRuleRow(idx)"
            icon="mdi-delete"
			class="subscription-row-action"
			:title="$t('subscriptionEditor.remove')"
			:aria-label="$t('subscriptionEditor.remove')"
            size="small"
            variant="text"
			@click="onFormValueChange(); removeClashRuleRow(idx)"
          ></v-btn>
        </div>
      </v-col>
    </v-row>

    <!-- Update method and interval -->
    <v-row>
      <v-col cols="12" sm="3" md="2">
		<v-select v-model="updateMethod" :items="clashUpdateMethodOptions" :label="$t('subscriptionEditor.updateMethod')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2">
		<v-text-field v-model="updateInterval" :label="$t('subscriptionEditor.updateInterval')" hide-details placeholder="1d" @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-select
          v-model="clashNoResolveGlobal"
          :items="optionalBoolOptions"
		  :label="$t('subscriptionEditor.globalNoResolve')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <!-- Final route outbound -->
    <v-row>
      <v-col cols="12" sm="3" md="2">
		<v-select v-model="routeFinal" :items="clashRouteFinalOptions" :label="$t('subscriptionEditor.routeFinal')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <!-- Latency test -->
    <v-row>
      <v-col cols="12" sm="6" md="6">
		<v-combobox v-model="latencyTestUrl" :items="clashLatencyTestUrlOptions" :label="$t('subscriptionEditor.latencyUrl')" hide-details @update:model-value="onFormValueChange"></v-combobox>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3">
        <v-text-field
          v-model="latencyTestInterval"
		  :label="$t('subscriptionEditor.latencyInterval')"
          hide-details="auto"
          :hint="$t('subscriptionEditor.clashIntervalHint')"
          persistent-hint
          :error-messages="latencyTestIntervalError ? [latencyTestIntervalError] : []"
          :placeholder="$t('subscriptionEditor.clashIntervalPlaceholder')"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <v-text-field
          v-model="latencyTolerance"
		  :label="$t('subscriptionEditor.latencyTolerance')"
          hide-details="auto"
          :hint="$t('subscriptionEditor.toleranceHint')"
          persistent-hint
          :error-messages="latencyToleranceError ? [latencyToleranceError] : []"
          :placeholder="$t('subscriptionEditor.tolerancePlaceholder')"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
    </v-row>

    <!-- Sniffer toggle -->
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
		<v-switch v-model="enableSniff" color="primary" :label="$t('subscriptionEditor.sniffer')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
		<v-switch v-model="enableRejectQuic" color="primary" :label="$t('subscriptionEditor.rejectQuicPorts')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="12" md="8" lg="6">
        <v-text-field
          v-model="rejectUdpPortsInput"
		  :label="$t('subscriptionEditor.customRejectUdpPorts')"
          hide-details="auto"
          :hint="$t('subscriptionEditor.udpPortsHint')"
          persistent-hint
          :error-messages="rejectUdpPortsInputError ? [rejectUdpPortsInputError] : []"
          :placeholder="$t('subscriptionEditor.udpPortsPlaceholder')"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="enableSniff">
      <v-col cols="12" sm="6" md="4" lg="3">
        <v-select
          v-model="snifferOverrideDestination"
          :items="optionalBoolOptions"
          label="override-destination"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row v-if="enableSniff">
      <v-col cols="12" sm="6" md="4" lg="3">
        <v-select
          v-model="snifferForceDnsMapping"
          :items="optionalBoolOptions"
          label="force-dns-mapping"
          :hint="$t('subscriptionEditor.forceDnsMappingHint')"
          persistent-hint
          hide-details="auto"
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row v-if="enableSniff">
      <v-col cols="12" sm="6" md="4" lg="3">
        <v-select
          v-model="snifferParsePureIp"
          :items="optionalBoolOptions"
		  :label="$t('subscriptionEditor.parsePureIp')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="dnsUseSystemHosts"
          :items="optionalBoolOptions"
          label="use-system-hosts"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="dnsUseHosts"
          :items="optionalBoolOptions"
          label="use-hosts"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="12" md="8" lg="6">
        <v-combobox
          v-model="clashHostsEntries"
          :items="[]"
          :delimiters="[]"
          label="hosts"
          :placeholder="$t('subscriptionEditor.hostsPlaceholder')"
          multiple
          chips
          closable-chips
          hide-details
         @update:model-value="onFormValueChange"></v-combobox>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch
          v-model="mihomoKeepAlive"
          color="primary"
          label="mihomo_keep-alive"
          hide-details
         @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>

    <v-row v-if="mihomoKeepAlive">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="disableKeepAlive"
          :items="dnsGeoipBoolOptions"
          label="disable-keep-alive"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <v-row v-if="mihomoKeepAlive">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field
          type="number"
          v-model.number="keepAliveIdle"
          min="0"
          label="keep-alive-idle"
          hide-details
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
    </v-row>

    <v-row v-if="mihomoKeepAlive">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field
          type="number"
          v-model.number="keepAliveInterval"
          min="0"
          label="keep-alive-interval"
          hide-details
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
    </v-row>

    <v-card-actions>
      <v-spacer></v-spacer>
      <v-btn @click="openEditor" variant="outlined" hide-details>{{ $t('editor') }}</v-btn>
    </v-card-actions>
  </v-card>
</template>

<script lang="ts">
import Editor from './Editor.vue'
import { SubClashExtMixin } from './SubClashExtLogic'
import {
  clashLogLevels,
  tunStackOptions,
  enhancedModeOptions,
  clashDomainIpTypes,
  clashGeositeNameOptions,
  clashGeoipNameOptions,
  clashLatencyTestUrlOptions,
  clashDirectNameserverOptions,
  clashProxyServerNameserverOptions,
  clashNameserverOptions,
  clashFallbackOptions,
  clashDefaultNameserverOptions,
  clashFakeIpFilterDefaults,
  defaultFakeIpRange,
  defaultFakeIpRange6,
  defaultTunInet4Address,
  defaultTunInet6Address,
  findProcessModeOptions,
} from './SubClashExtConstants'

export default {
	props: ['settings', 'canonicalDefault', 'initialDirty', 'initialReset', 'ruleSetSources'],
	emits: ['dirty-change'],
  components: { Editor },
  mixins: [SubClashExtMixin],
  data() {
	const backendRuleSetSourceOptions = (Array.isArray(this.ruleSetSources) ? this.ruleSetSources : [])
	  .filter((item: any) => item?.domainTemplate && item?.ipTemplate)
	  .map((item: any) => ({ title: item.title || item.id, value: item.id }))
	const selectorOptions = [
	  { title: this.$t('subscriptionEditor.nodeSelector'), value: '节点选择' },
	  { title: this.$t('subscriptionEditor.autoSelector'), value: '自动选择' },
	  { title: this.$t('subscriptionEditor.globalDirectSelector'), value: '全球直连' },
	  { title: this.$t('subscriptionEditor.globalBlockSelector'), value: '全球拦截' },
	  { title: this.$t('subscriptionEditor.finalSelector'), value: '漏网之鱼' },
	]
	const matchTypeTitleKeys: Record<string, string> = {
	  DOMAIN: 'subscriptionEditor.domainExact',
	  'DOMAIN-SUFFIX': 'subscriptionEditor.domainSuffix',
	  'DOMAIN-KEYWORD': 'subscriptionEditor.domainKeyword',
	  'DOMAIN-WILDCARD': 'subscriptionEditor.domainWildcard',
	  'DOMAIN-REGEX': 'subscriptionEditor.domainRegex',
	  'IP-CIDR': 'subscriptionEditor.ipCidr',
	  'IP-CIDR6': 'subscriptionEditor.ipCidr6',
	  'IP-SUFFIX': 'subscriptionEditor.ipSuffix',
	  'IP-ASN': 'IP ASN',
	  GEOIP: 'GEOIP',
	}
    return {
      // Reactive state.
      metaJson: {} as any,
      enableEditor: false,
      menu: false,
      _uiConfigLoaded: false,
      _suspendClashRegeneration: false,
	  _dirty: this.initialDirty === true,
	  _resetRequested: this.initialReset === true,
	  _parseError: '',
	  _rawSource: '',
	  _editorSourcePending: false,

      // Clash rule rows (independent from JSON sub rule rows).
      ruleSetSource: 'metacubex_cdn' as string,
      clashNoResolveGlobal: true as boolean | null,
      resolvedRuleSetUrls: {} as Record<string, { url: string; source: string }>,
      ruleSetResolutionRunToken: 0,
      clashRuleRows: [
		{ id: 'clash-rule-initial', kind: 'custom', name: '', customType: 'DOMAIN-KEYWORD', ruleSetScope: 'domain', ruleSetSourceOverride: null as string | null, route: 'REJECT', noResolve: true, values: [] as string[] },
	  ] as Array<{ id: string; kind: string; name: string; customType: string; ruleSetScope: string; ruleSetSourceOverride: string | null; route: string; noResolve: boolean; values: string[] }>,
      clashDnsPolicyRows: [
        { id: 'clash-dns-policy-initial', matchType: 'rule-set', routeTarget: 'nameserver', values: [] as string[] },
	  ] as Array<{ id: string; matchType: string; routeTarget: string; values: string[] }>,
      clashDnsSuffixRows: [
		{ id: 'clash-dns-suffix-initial', targets: [] as string[], selections: [] as string[] },
	  ] as Array<{ id: string; targets: string[]; selections: string[] }>,
      clashDnsSuffixAppliedRowsSnapshot: [] as Array<{ targets: string[]; selections: string[] }>,
      clashRuleKindOptions: [
		{ title: this.$t('subscriptionEditor.customMatch'), value: 'custom' },
		{ title: this.$t('subscriptionEditor.ruleSet'), value: 'ruleset' },
      ],
      clashDnsPolicyMatchTypeOptions: [
		{ title: this.$t('subscriptionEditor.domainWildcardRule'), value: 'domain' },
        { title: 'Geosite (geosite)', value: 'geosite' },
		{ title: `${this.$t('subscriptionEditor.ruleSet')} (rule-set)`, value: 'rule-set' },
      ],
      clashDnsPolicyRouteOptions: [
        { title: 'nameserver', value: 'nameserver' },
        { title: 'fallback', value: 'fallback' },
        { title: 'direct-nameserver', value: 'direct-nameserver' },
      ],
      clashDnsSuffixTargetOptions: [
        { title: 'direct-nameserver', value: 'direct-nameserver' },
        { title: 'proxy-server-nameserver', value: 'proxy-server-nameserver' },
        { title: 'nameserver', value: 'nameserver' },
        { title: 'fallback', value: 'fallback' },
        { title: 'default-nameserver', value: 'default-nameserver' },
      ],
      clashDnsSuffixSelectionOptions: [
		{ title: this.$t('subscriptionEditor.nodeSelector'), value: '节点选择' },
        { title: 'proxy', value: 'proxy' },
        { title: 'disable-ipv4=true', value: 'disable-ipv4=true' },
        { title: 'disable-ipv6=true', value: 'disable-ipv6=true' },
        { title: 'skip-cert-verify=true', value: 'skip-cert-verify=true' },
        { title: 'h3=true', value: 'h3=true' },
      ],
      clashRuleSetScopeOptions: [
		{ title: this.$t('subscriptionEditor.domain'), value: 'domain' },
        { title: 'IP', value: 'ip' },
      ],
      clashCustomRouteOptions: [
		{ title: this.$t('subscriptionEditor.blockRoute'), value: 'REJECT' },
		{ title: this.$t('subscriptionEditor.directRoute'), value: 'DIRECT' },
		{ title: this.$t('subscriptionEditor.proxyRoute'), value: 'Proxy' },
      ],
      updateMethod: '节点选择' as string,
      updateInterval: '1d' as string,
      routeFinal: '节点选择' as string,

      // Latency test settings.
      latencyTestUrl: 'https://cp.cloudflare.com/generate_204' as string,
      latencyTestInterval: '180s' as string,
      latencyTolerance: '50' as string,
      mihomoKeepAlive: false,
      keepAliveIdle: 0,
      keepAliveInterval: 30,
      disableKeepAlive: false,

      // Feature toggles.
      enableSniff: true,
      snifferOverrideDestination: true as boolean | null,
      snifferForceDnsMapping: null as boolean | null,
      snifferParsePureIp: null as boolean | null,
      enableRejectQuic: false,
      rejectUdpPortsInput: '' as string,

      // TUN excluded packages.
      tunExcludePackage: [] as string[],

      // Shared constants.
      clashLogLevels,
      tunStackOptions,
      enhancedModeOptions,
	  clashRuleSetSourceOptions: backendRuleSetSourceOptions,
      clashRuleSetSourceOverrideOptions: [
		{ title: this.$t('subscriptionEditor.useGlobalRuleSetSource'), value: null as string | null },
		...backendRuleSetSourceOptions,
      ],
      clashDomainIpTypes: clashDomainIpTypes.map((item: any) => ({
		...item,
		title: matchTypeTitleKeys[item.value]?.startsWith('subscriptionEditor.')
		  ? `${this.$t(matchTypeTitleKeys[item.value])} (${item.value})`
		  : matchTypeTitleKeys[item.value] || item.value,
	  })),
      clashGeositeNameOptions: clashGeositeNameOptions.filter((item: string) => item.trim().length > 0),
      clashGeoipNameOptions: clashGeoipNameOptions.filter((item: string) => item.trim().length > 0),
      clashUpdateMethodOptions: selectorOptions,
      clashLatencyTestUrlOptions,
      clashRouteFinalOptions: selectorOptions,
      clashDirectNameserverOptions,
      clashProxyServerNameserverOptions,
      clashNameserverOptions,
      clashFallbackOptions,
      clashDefaultNameserverOptions,
      clashFakeIpFilterDefaults,
      dnsFakeIpRangeOptions: [defaultFakeIpRange],
      dnsFakeIpRange6Options: [defaultFakeIpRange6],
      tunInet4AddressOptions: [defaultTunInet4Address],
      tunInet6AddressOptions: [defaultTunInet6Address],
      dnsGeoipBoolOptions: [
        { title: 'true', value: true },
        { title: 'false', value: false },
      ],
      optionalBoolOptions: [
        { title: '', value: null },
        { title: 'true', value: true },
        { title: 'false', value: false },
      ],
      clashGeoipCodeOptions: [
        'CN',
        'US',
        'JP',
        'KR',
        'SG',
        'HK',
        'TW',
        'GB',
        'DE',
        'FR',
        'NL',
        'CA',
        'AU',
        'IN',
        'BR',
        'RU',
      ],
      findProcessModeOptions: findProcessModeOptions.map((item: any) => ({
		...item,
		title: `${this.$t(`subscriptionEditor.processMode${String(item.value).replace(/^./, (value: string) => value.toUpperCase())}`)} (${item.value})`,
	  })),
    }
  },
}
</script>

<style scoped>
.subscription-row-action {
	width: 36px;
	height: 36px;
	min-width: 36px;
	flex: 0 0 36px;
}

.subscription-row-actions {
	flex-basis: 100%;
	max-width: 100%;
}
</style>
