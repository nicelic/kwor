<template>
  <Editor
	v-if="enableEditor"
    v-model="enableEditor"
    :data="editorData"
    :visible="enableEditor"
    :title="$t('editor') + ' - ' + $t('setting.jsonSub')"
    @close="enableEditor = false"
    @save="saveEditor"
    />
	<v-card @input.capture="onFormValueChange" @change.capture="onFormValueChange">
	  <template v-if="formRowsTooLarge">
	    <v-alert type="warning" variant="tonal" density="compact" class="ma-4">
	      {{ $t('subscriptionEditor.formRowsTooLarge') }}
	    </v-alert>
	    <v-card-actions>
	      <v-spacer></v-spacer>
	      <v-btn @click="openEditor" variant="outlined" hide-details>{{ $t('editor') }}</v-btn>
	    </v-card-actions>
	  </template>
	  <template v-else>
	  <v-alert v-if="parseError" type="error" variant="tonal" density="compact" class="mb-4">
	    {{ parseError }}
	  </v-alert>
    <!-- Server tls_store settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
		<v-switch v-model="enableServerTlsStore" color="primary" :label="$t('subscriptionEditor.serverTlsStore')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="enableServerTlsStore">
        <v-select hide-details label="store" :items="tlsStoreOptions" v-model="serverTlsStore" @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
		<v-switch v-model="enableClientTlsStore" color="primary" :label="$t('subscriptionEditor.clientTlsStore')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="enableClientTlsStore">
        <v-select hide-details label="store" :items="tlsStoreOptions" v-model="clientTlsStore" @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>

    <!-- Log settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="enableLog" color="primary" :label="$t('basic.log.title')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>

    <v-row v-if="enableLog">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          hide-details
          :label="$t('basic.log.level')"
          :items="levels"
          v-model="subJsonExt.log.level" @update:model-value="onFormValueChange">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="subJsonExt.log.timestamp" color="primary" :label="$t('setting.timestamp')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>

    <!-- DNS switch -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="enableDns" color="primary" :label="$t('pages.dns')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>
    <!-- DNS settings -->
    <template v-if="enableDns">
      <v-row>
        <v-col cols="12" sm="6" md="4" lg="4">
          <v-row no-gutters>
            <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">{{ $t('subscriptionEditor.proxyTrafficDns') }}</v-col>
            <v-col cols="4">
              <v-select hide-details :label="$t('type')" :items="dnsTypeOptions" density="compact" class="noGutters" v-model="proxyDnsType" @update:model-value="onProxyDnsTypeChange($event); onFormValueChange()"></v-select>
            </v-col>
            <v-col cols="5" v-if="proxyDnsShowServer">
              <v-text-field v-model="proxyDnsServer" :label="$t('in.addr')" density="compact" class="noGutters" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
            <v-col cols="3" v-if="proxyDnsShowServer">
              <v-text-field v-model.number="proxyDnsPort" :label="$t('in.port')" density="compact" type="number" class="noGutters" min="1" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
            <v-col cols="12" v-if="proxyDnsUsesPath">
              <v-text-field v-model="proxyDnsPath" :label="$t('transport.path')" density="compact" class="noGutters" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
          </v-row>
        </v-col>
        <v-col cols="12" sm="6" md="4" lg="4">
          <v-row no-gutters>
            <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">{{ $t('subscriptionEditor.directTrafficDns') }}</v-col>
            <v-col cols="4">
              <v-select hide-details :label="$t('type')" :items="dnsTypeOptions" density="compact" class="noGutters" v-model="directDnsType" @update:model-value="onDirectDnsTypeChange($event); onFormValueChange()"></v-select>
            </v-col>
            <v-col cols="5" v-if="directDnsShowServer">
              <v-text-field v-model="directDnsServer" :label="$t('in.addr')" density="compact" class="noGutters" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
            <v-col cols="3" v-if="directDnsShowServer">
              <v-text-field v-model.number="directDnsPort" :label="$t('in.port')" density="compact" type="number" class="noGutters" min="1" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
            <v-col cols="12" v-if="directDnsUsesPath">
              <v-text-field v-model="directDnsPath" :label="$t('transport.path')" density="compact" class="noGutters" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
          </v-row>
        </v-col>
      </v-row>
      <!-- DNS bootstrap settings -->
      <v-row>
        <v-col cols="12" sm="6" md="4" lg="4">
          <v-row no-gutters>
            <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">{{ $t('subscriptionEditor.proxyBootstrapDns') }}</v-col>
            <v-col cols="4">
              <v-select hide-details :label="$t('type')" :items="dnsTypeOptions" density="compact" class="noGutters" v-model="proxyBootstrapDnsType" @update:model-value="onProxyBootstrapDnsTypeChange($event); onFormValueChange()"></v-select>
            </v-col>
            <v-col cols="5" v-if="proxyBootstrapDnsShowServer">
              <v-text-field v-model="proxyBootstrapDnsServer" :label="$t('in.addr')" density="compact" class="noGutters" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
            <v-col cols="3" v-if="proxyBootstrapDnsShowServer">
              <v-text-field v-model.number="proxyBootstrapDnsPort" :label="$t('in.port')" density="compact" type="number" class="noGutters" min="1" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
            <v-col cols="12" v-if="proxyBootstrapDnsUsesPath">
              <v-text-field v-model="proxyBootstrapDnsPath" :label="$t('transport.path')" density="compact" class="noGutters" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
          </v-row>
        </v-col>
        <v-col cols="12" sm="6" md="4" lg="4">
          <v-row no-gutters>
            <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">{{ $t('subscriptionEditor.directBootstrapDns') }}</v-col>
            <v-col cols="4">
              <v-select hide-details :label="$t('type')" :items="dnsTypeOptions" density="compact" class="noGutters" v-model="directBootstrapDnsType" @update:model-value="onDirectBootstrapDnsTypeChange($event); onFormValueChange()"></v-select>
            </v-col>
            <v-col cols="5" v-if="directBootstrapDnsShowServer">
              <v-text-field v-model="directBootstrapDnsServer" :label="$t('in.addr')" density="compact" class="noGutters" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
            <v-col cols="3" v-if="directBootstrapDnsShowServer">
              <v-text-field v-model.number="directBootstrapDnsPort" :label="$t('in.port')" density="compact" type="number" class="noGutters" min="1" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
            <v-col cols="12" v-if="directBootstrapDnsUsesPath">
              <v-text-field v-model="directBootstrapDnsPath" :label="$t('transport.path')" density="compact" class="noGutters" hide-details @update:model-value="onFormValueChange"></v-text-field>
            </v-col>
          </v-row>
        </v-col>
      </v-row>
      <!-- DNS row 3: final_dns and query_type switch -->
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select hide-details label="final_dns" :items="dnsFinalOptions" v-model="subJsonExt.dns.final" @update:model-value="onFormValueChange"></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-switch v-model="enableDnsQueryType" color="primary" label="query_type" hide-details  @update:model-value="onFormValueChange"/>
        </v-col>
      </v-row>
      <!-- DNS row 4: fakeip switch and fakeip ranges -->
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-switch v-model="enableFakeip" color="primary" label="fakeip" hide-details  @update:model-value="onFormValueChange"/>
        </v-col>
        <v-col cols="12" sm="12" md="6" lg="4" v-if="enableFakeip">
          <v-combobox
            v-model="tunIp"
            :items="tunIpOptions"
            chips
            multiple
            closable-chips
            clearable
            hide-details
            label="fakeip"
           @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
      </v-row>
      <!-- DNS row 5: sortable route rows -->
      <v-row
        v-for="(dnsRouteRow, dnsRowIdx) in dnsRouteRows"
		:key="dnsRouteRow.id"
        align="center"
      >
        <v-col cols="12" sm="6" md="3">
          <v-combobox
            v-if="dnsRouteRow.kind === 'rule-set'"
            v-model="dnsRouteRow.ruleSet"
            :items="dnsRouteRuleSetOptions"
            label="rule_set"
            multiple
            chips
            closable-chips
            hide-details
           @update:model-value="onFormValueChange"></v-combobox>
          <v-text-field
            v-else
            model-value="&quot;query_type&quot;: [&quot;A&quot;, &quot;AAAA&quot;]"
            label="query_type"
            readonly
            hide-details
          ></v-text-field>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select
            v-model="dnsRouteRow.server"
            :items="dnsRouteServerOptions"
            label="dns"
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
              :disabled="dnsRowIdx === 0"
			  @click="onFormValueChange(); moveDnsRouteRow(dnsRowIdx, -1)"
            ></v-btn>
            <v-btn
              icon="mdi-arrow-down"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.moveDown')"
			  :aria-label="$t('subscriptionEditor.moveDown')"
              size="small"
              variant="text"
              :disabled="dnsRowIdx >= dnsRouteRows.length - 1"
			  @click="onFormValueChange(); moveDnsRouteRow(dnsRowIdx, 1)"
            ></v-btn>
            <v-btn
              v-if="dnsRouteRow.kind === 'rule-set'"
              icon="mdi-plus"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.add')"
			  :aria-label="$t('subscriptionEditor.add')"
              size="small"
              variant="text"
			  @click="onFormValueChange(); insertDnsRouteRow(dnsRowIdx)"
            ></v-btn>
            <v-btn
              v-if="dnsRouteRow.kind === 'rule-set' && canDeleteDnsRouteRow(dnsRowIdx)"
              icon="mdi-delete"
			  class="subscription-row-action"
			  :title="$t('subscriptionEditor.remove')"
			  :aria-label="$t('subscriptionEditor.remove')"
              size="small"
              variant="text"
			  @click="onFormValueChange(); removeDnsRouteRow(dnsRowIdx)"
            ></v-btn>
          </div>
        </v-col>
      </v-row>
      <!-- DNS row 6: resolver strategy -->
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
		  <v-select v-model="dnsStrategy" :items="dnsStrategyOptions" :label="$t('subscriptionEditor.dnsStrategy')" hide-details @update:model-value="onFormValueChange"></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select
            v-model="routeDefaultDomainResolver"
            :items="dnsFinalOptions"
            label="default_domain_resolve"
            hide-details
           @update:model-value="onFormValueChange"></v-select>
        </v-col>
      </v-row>
    </template>

    <!-- Inbound settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="enableInb" color="primary" label="Inbound" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>
    <template v-if="enableInb">
      <!-- TUN switches -->
      <v-row>
        <v-col cols="12" sm="4" md="2" lg="2">
          <v-switch v-model="enableTun" color="primary" label="tun" hide-details  @update:model-value="onFormValueChange"/>
        </v-col>
        <v-col cols="12" sm="4" md="2" lg="2" v-if="enableTun">
		  <v-switch v-model="autoRoute" color="primary" :label="$t('subscriptionEditor.autoRoute')" hide-details  @update:model-value="onFormValueChange"/>
        </v-col>
        <v-col cols="12" sm="4" md="2" lg="2" v-if="enableTun && autoRoute">
		  <v-switch v-model="strictRoute" color="primary" :label="$t('subscriptionEditor.strictRoute')" hide-details  @update:model-value="onFormValueChange"/>
        </v-col>
        <v-col cols="12" sm="4" md="3" lg="3" v-if="enableTun">
          <v-switch v-model="endpointIndependentNat" color="primary" label="endpoint_independent_nat" hide-details  @update:model-value="onFormValueChange"/>
        </v-col>
      </v-row>
      <!-- TUN address and MTU -->
      <v-row v-if="enableTun">
        <v-col cols="12" sm="6" md="3">
		  <v-combobox v-model="tunAddress" :items="defaultTunAddress" chips multiple hide-details :label="$t('subscriptionEditor.tunAddress')" @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-text-field type="number" v-model.number="tunMtu" hide-details label="MTU" @update:model-value="onFormValueChange"></v-text-field>
        </v-col>
      </v-row>
      <v-row v-if="enableTun">
        <v-col cols="12" sm="6" md="3" lg="2">
		  <v-select v-model="tunMode" :items="['system', 'mixed', 'gvisor']" :label="$t('subscriptionEditor.tunMode')" hide-details @update:model-value="onFormValueChange"></v-select>
        </v-col>
      </v-row>
      <!-- Mixed inbound listen settings -->
      <v-row>
        <v-col cols="12" sm="4" md="3" lg="2">
		  <v-text-field v-model="mixedListen" :label="$t('subscriptionEditor.defaultListen')" hide-details placeholder="127.0.0.1" @update:model-value="onFormValueChange"></v-text-field>
        </v-col>
        <v-col cols="12" sm="2" md="2" lg="1">
		  <v-text-field type="number" v-model.number="mixedListenPort" :label="$t('setting.port')" hide-details placeholder="2080" @update:model-value="onFormValueChange"></v-text-field>
        </v-col>
      </v-row>
      <!-- TUN package exclusion and platform proxy -->
      <v-row v-if="enableTun">
        <v-col cols="12" sm="6" md="3">
          <v-combobox v-model="tunExcludePackage" :items="['ir.mci.ecareapp','com.myirancell']" chips multiple hide-details :label="$t('setting.excludePkg')" @update:model-value="onFormValueChange"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
		  <v-switch v-model="platformProxy" hide-details color="primary" :label="$t('subscriptionEditor.platformHttpProxy')" @update:model-value="onFormValueChange"></v-switch>
        </v-col>
      </v-row>
    </template>

    <!-- Rule set source -->
    <v-row>
      <v-col cols="12" sm="6" md="3">
		<v-select v-model="ruleSetSource" :items="ruleSetSourceOptions" :label="$t('subscriptionEditor.globalRuleSetSource')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
        <!-- Match/ruleset unified list -->
    <v-row
      v-for="(row, idx) in ruleRows"
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
          :items="ruleKindOptions"
		  :label="$t('subscriptionEditor.ruleKind')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-select
          v-if="row.kind === 'custom'"
          v-model="row.customType"
          :items="domainIpTypes"
		  :label="idx === 0 ? $t('subscriptionEditor.customMatchType') : $t('subscriptionEditor.matchType')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
        <v-select
          v-else
          v-model="row.ruleSetScope"
          :items="ruleSetScopeOptions"
		  :label="$t('subscriptionEditor.ruleSetScope')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2" v-if="row.kind === 'ruleset'">
        <v-select
          v-model="row.ruleSetSourceOverride"
		  :items="getRuleSetSourceOverrideOptions(row.ruleSetScope)"
		  :label="$t('subscriptionEditor.ruleSetSource')"
          hide-details
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <v-combobox
          v-model="row.values"
          :items="row.kind === 'ruleset' ? (row.ruleSetScope === 'ip' ? geoipNameOptions : geositeNameOptions) : []"
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
          :items="customRouteOptions"
		  :label="row.name && row.name.trim() ? $t('subscriptionEditor.routeDisabledByName') : $t('subscriptionEditor.route')"
          :disabled="Boolean(row.name && row.name.trim())"
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
            :disabled="idx === 0"
			@click="onFormValueChange(); moveRuleRow(idx, -1)"
          ></v-btn>
          <v-btn
            icon="mdi-arrow-down"
			class="subscription-row-action"
			:title="$t('subscriptionEditor.moveDown')"
			:aria-label="$t('subscriptionEditor.moveDown')"
            size="small"
            variant="text"
            :disabled="idx >= ruleRows.length - 1"
			@click="onFormValueChange(); moveRuleRow(idx, 1)"
          ></v-btn>
          <v-btn
            icon="mdi-plus"
			class="subscription-row-action"
			:title="$t('subscriptionEditor.add')"
			:aria-label="$t('subscriptionEditor.add')"
            size="small"
            variant="text"
			@click="onFormValueChange(); insertRuleRow(idx)"
          ></v-btn>
          <v-btn
            v-if="canDeleteRuleRow(idx)"
            icon="mdi-delete"
			class="subscription-row-action"
			:title="$t('subscriptionEditor.remove')"
			:aria-label="$t('subscriptionEditor.remove')"
            size="small"
            variant="text"
			@click="onFormValueChange(); removeRuleRow(idx)"
          ></v-btn>
        </div>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="3" md="2">
		<v-select v-model="updateMethod" :items="updateMethodOptions" :label="$t('subscriptionEditor.updateMethod')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2">
		<v-text-field v-model="updateInterval" :label="$t('subscriptionEditor.updateInterval')" hide-details placeholder="1d" @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="3" md="2">
		<v-select v-model="routeFinal" :items="routeFinalOptions" :label="$t('subscriptionEditor.routeFinal')" hide-details @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="6">
		<v-combobox v-model="latencyTestUrl" :items="latencyTestUrlOptions" :label="$t('subscriptionEditor.latencyUrl')" hide-details @update:model-value="onFormValueChange"></v-combobox>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3">
        <v-text-field
          v-model="latencyTestInterval"
		  :label="$t('subscriptionEditor.latencyInterval')"
          hide-details="auto"
          :hint="$t('subscriptionEditor.singboxIntervalHint')"
          persistent-hint
          :error-messages="latencyTestIntervalError ? [latencyTestIntervalError] : []"
          :placeholder="$t('subscriptionEditor.singboxIntervalPlaceholder')"
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
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
		<v-switch v-model="enableRejectQuic" color="primary" :label="$t('subscriptionEditor.rejectQuic')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
		<v-switch v-model="enableReject443Udp" color="primary" :label="$t('subscriptionEditor.reject443Udp')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
		<v-switch v-model="enableExp" color="primary" :label="$t('subscriptionEditor.localCache')" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableSubClashApi" color="primary" label="clash_api" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableSniff" color="primary" label="sniff" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableHijackDns" color="primary" label="hijack-dns" hide-details  @update:model-value="onFormValueChange"/>
      </v-col>
    </v-row>
    <v-row v-if="subJsonExt.experimental?.clash_api">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field
          v-model="subJsonExt.experimental.clash_api.external_controller"
          hide-details
          label="external_controller"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field
          v-model="subJsonExt.experimental.clash_api.secret"
          hide-details
          label="secret"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="subJsonExt.experimental.clash_api.default_mode"
          :items="clashApiModeOptions"
          hide-details
          label="default_mode"
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row v-if="subJsonExt.experimental?.clash_api">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field
          v-model="subJsonExt.experimental.clash_api.external_ui"
          hide-details
          label="external_ui"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="12" md="6">
        <v-text-field
          v-model="subJsonExt.experimental.clash_api.external_ui_download_url"
          hide-details
          label="external_ui_download_url"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="subJsonExt.experimental.clash_api.external_ui_download_detour"
          :items="subSelectorTagOptions"
          hide-details
          clearable
          label="external_ui_download_detour"
         @update:model-value="onFormValueChange"></v-select>
      </v-col>
    </v-row>
    <v-row v-if="subJsonExt.experimental?.clash_api">
      <v-col cols="12" sm="12" md="6">
        <v-text-field
          v-model="subClashApiOrigin"
          hide-details
          label="access_control_allow_origin (comma separated)"
         @update:model-value="onFormValueChange"></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch
          v-model="subJsonExt.experimental.clash_api.access_control_allow_private_network"
          color="primary"
          label="allow_private_network"
          hide-details
         @update:model-value="onFormValueChange"></v-switch>
      </v-col>
    </v-row>
    <v-card-actions>
      <v-spacer></v-spacer>
      <v-btn @click="openEditor" variant="outlined" hide-details>{{ $t('editor') }}</v-btn>
    </v-card-actions>
	  </template>
  </v-card>
</template>

<script lang="ts">
import Editor from './Editor.vue'
import {
  SubJsonExtMixin,
  jsonSubscriptionDNSUsesPath,
  normalizeJSONSubscriptionDNSPath,
} from './SubJsonExtLogic'
import {
  levels,
  tunIpOptions,
  dnsStrategyOptions,
  tlsStoreOptions,
  ruleSetOptions,
  geositeNameOptions,
  geoipNameOptions,
  latencyTestUrlOptions,
  clashApiModeOptions,
  geositeList,
  geoList,
  geo,
  defaultInb,
} from './SubJsonExtConstants'

export default {
	props: ['settings', 'canonicalDefault', 'initialDirty', 'initialReset', 'ruleSetSources'],
	emits: ['dirty-change'],
  components: { Editor },
  mixins: [SubJsonExtMixin],
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
    return {
      // Reactive state
      subJsonExt: {} as any,
      menu: false,
      enableEditor: false,
      ruleSetSource: "karingx_github" as string,
      autoMatchedRuleSetUrls: {} as Record<string, { url: string; source: string }>,
      autoMatchRunToken: 0,
      dnsRouteRows: [
		{ id: "json-dns-initial", kind: "rule-set", server: "proxy-dns", ruleSet: [] as string[] },
	  ] as Array<{ id: string; kind: string; server: string; ruleSet: string[] }>,
      ruleRows: [
		{ id: "json-rule-initial", kind: "custom", name: "", customType: "domain_keyword", ruleSetScope: "domain", ruleSetSourceOverride: null as string | null, route: "reject", values: [] as string[] },
	  ] as Array<{ id: string; kind: string; name: string; customType: string; ruleSetScope: string; ruleSetSourceOverride: string | null; route: string; values: string[] }>,
      ruleKindOptions: [
		{ title: this.$t('subscriptionEditor.customMatch'), value: "custom" },
		{ title: this.$t('subscriptionEditor.ruleSet'), value: "ruleset" },
      ],
      ruleSetScopeOptions: [
		{ title: this.$t('subscriptionEditor.domain'), value: "domain" },
        { title: "IP", value: "ip" },
      ],
      customRouteOptions: [
		{ title: this.$t('subscriptionEditor.blockRoute'), value: "reject" },
		{ title: this.$t('subscriptionEditor.directRoute'), value: "direct" },
		{ title: this.$t('subscriptionEditor.proxyRoute'), value: "proxy" },
      ],
      updateMethod: "节点选择" as string,
      updateInterval: "1d" as string,
      routeFinal: "节点选择" as string,
      routeFinalOptions: selectorOptions,
      clashApiModeOptions,
      subSelectorTagOptions: selectorOptions,
      latencyTestUrl: "https://cp.cloudflare.com/generate_204" as string,
      latencyTestInterval: "3m" as string,
      latencyTolerance: "50" as string,
      enableSniff: true,
      enableHijackDns: true,
      enableRejectQuic: false,
      enableReject443Udp: false,
      _uiConfigLoaded: false,
      _suspendRuleRegeneration: false,
	  _dirty: this.initialDirty === true,
	  _resetRequested: this.initialReset === true,
	  _parseError: '',
	  _rawSource: '',
      _editorSourcePending: false,
	  formRowsTooLarge: false,

      // DNS server type options.
      dnsTypeOptions: ['udp', 'tcp', 'local', 'dhcp', 'tls', 'quic', 'h3', 'https'],
      noServerTypes: ['local', 'dhcp'],
      defaultTunAddress: ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],

      // Shared constant lists.
      levels,
      tunIpOptions,
      dnsStrategyOptions,
      tlsStoreOptions,
	  ruleSetSourceOptions: backendRuleSetSourceOptions,
      ruleSetSourceOverrideOptions: [
		{ title: this.$t('subscriptionEditor.useGlobalRuleSetSource'), value: null as string | null },
		...backendRuleSetSourceOptions,
      ],
      domainIpTypes: [
		{ title: this.$t('subscriptionEditor.domainExact'), value: 'domain' },
		{ title: this.$t('subscriptionEditor.domainSuffix'), value: 'domain_suffix' },
		{ title: this.$t('subscriptionEditor.domainKeyword'), value: 'domain_keyword' },
		{ title: this.$t('subscriptionEditor.domainRegex'), value: 'domain_regex' },
		{ title: this.$t('subscriptionEditor.ipCidr'), value: 'ip_cidr' },
		{ title: this.$t('subscriptionEditor.privateIp'), value: 'ip_is_private' },
	  ],
      ruleSetOptions,
      geositeNameOptions: geositeNameOptions.filter((item: string) => item.trim().length > 0),
      geoipNameOptions: geoipNameOptions.filter((item: string) => item.trim().length > 0),
      updateMethodOptions: selectorOptions,
      latencyTestUrlOptions,
      geositeList,
      geoList,
      geo,
      defaultInb,
    }
  },
  computed: {
    // DNS server object accessors.
    proxyDnsObj(): any {
      const servers = this.subJsonExt?.dns?.servers
      if (!servers || !Array.isArray(servers)) return {}
      return servers.find((s: any) => s.tag === 'proxy-dns') ?? {}
    },
    directDnsObj(): any {
      const servers = this.subJsonExt?.dns?.servers
      if (!servers || !Array.isArray(servers)) return {}
      return servers.find((s: any) => s.tag === 'direct-dns') ?? {}
    },
    proxyBootstrapDnsObj(): any {
      const servers = this.subJsonExt?.dns?.servers
      if (!servers || !Array.isArray(servers)) return {}
      return servers.find((s: any) => s.tag === 'proxy-bootstrap-dns') ?? {}
    },
    directBootstrapDnsObj(): any {
      const servers = this.subJsonExt?.dns?.servers
      if (!servers || !Array.isArray(servers)) return {}
      return servers.find((s: any) => s.tag === 'direct-bootstrap-dns') ?? {}
    },
    proxyDnsType: {
      get(): string { return this.proxyDnsObj?.type ?? 'udp' },
      set(v: string) { if (this.proxyDnsObj && this.proxyDnsObj.tag) this.proxyDnsObj.type = v }
    },
    proxyDnsServer: {
      get(): string { return this.proxyDnsObj?.server ?? '' },
      set(v: string) { if (this.proxyDnsObj && this.proxyDnsObj.tag) this.proxyDnsObj.server = v; if (this.proxyDnsObj?.tls) this.proxyDnsObj.tls.server_name = v }
    },
    proxyDnsPort: {
      get(): number { return this.proxyDnsObj?.server_port ?? 53 },
      set(v: number) { if (this.proxyDnsObj && this.proxyDnsObj.tag) this.proxyDnsObj.server_port = v }
    },
    proxyDnsShowServer(): boolean { return !this.noServerTypes.includes(this.proxyDnsType) },
    proxyDnsUsesPath(): boolean { return jsonSubscriptionDNSUsesPath(this.proxyDnsType) },
    proxyDnsPath: {
      get(): string { return typeof this.proxyDnsObj?.path === 'string' ? this.proxyDnsObj.path : '' },
      set(v: string) { if (this.proxyDnsObj && this.proxyDnsObj.tag) this.proxyDnsObj.path = v }
    },
    directDnsType: {
      get(): string { return this.directDnsObj?.type ?? 'https' },
      set(v: string) { if (this.directDnsObj && this.directDnsObj.tag) this.directDnsObj.type = v }
    },
    directDnsServer: {
      get(): string { return this.directDnsObj?.server ?? '' },
      set(v: string) { if (this.directDnsObj && this.directDnsObj.tag) this.directDnsObj.server = v; if (this.directDnsObj?.tls) this.directDnsObj.tls.server_name = v }
    },
    directDnsPort: {
      get(): number { return this.directDnsObj?.server_port ?? 443 },
      set(v: number) { if (this.directDnsObj && this.directDnsObj.tag) this.directDnsObj.server_port = v }
    },
    directDnsShowServer(): boolean { return !this.noServerTypes.includes(this.directDnsType) },
    directDnsUsesPath(): boolean { return jsonSubscriptionDNSUsesPath(this.directDnsType) },
    directDnsPath: {
      get(): string { return typeof this.directDnsObj?.path === 'string' ? this.directDnsObj.path : '' },
      set(v: string) { if (this.directDnsObj && this.directDnsObj.tag) this.directDnsObj.path = v }
    },
    proxyBootstrapDnsType: {
      get(): string { return this.proxyBootstrapDnsObj?.type ?? 'udp' },
      set(v: string) { if (this.proxyBootstrapDnsObj && this.proxyBootstrapDnsObj.tag) this.proxyBootstrapDnsObj.type = v }
    },
    proxyBootstrapDnsServer: {
      get(): string { return this.proxyBootstrapDnsObj?.server ?? '' },
      set(v: string) {
        if (this.proxyBootstrapDnsObj && this.proxyBootstrapDnsObj.tag) this.proxyBootstrapDnsObj.server = v
        if (this.proxyBootstrapDnsObj?.tls) this.proxyBootstrapDnsObj.tls.server_name = v
      }
    },
    proxyBootstrapDnsPort: {
      get(): number { return this.proxyBootstrapDnsObj?.server_port ?? 53 },
      set(v: number) { if (this.proxyBootstrapDnsObj && this.proxyBootstrapDnsObj.tag) this.proxyBootstrapDnsObj.server_port = v }
    },
    proxyBootstrapDnsShowServer(): boolean { return !this.noServerTypes.includes(this.proxyBootstrapDnsType) },
    proxyBootstrapDnsUsesPath(): boolean { return jsonSubscriptionDNSUsesPath(this.proxyBootstrapDnsType) },
    proxyBootstrapDnsPath: {
      get(): string { return typeof this.proxyBootstrapDnsObj?.path === 'string' ? this.proxyBootstrapDnsObj.path : '' },
      set(v: string) { if (this.proxyBootstrapDnsObj && this.proxyBootstrapDnsObj.tag) this.proxyBootstrapDnsObj.path = v }
    },
    directBootstrapDnsType: {
      get(): string { return this.directBootstrapDnsObj?.type ?? 'udp' },
      set(v: string) { if (this.directBootstrapDnsObj && this.directBootstrapDnsObj.tag) this.directBootstrapDnsObj.type = v }
    },
    directBootstrapDnsServer: {
      get(): string { return this.directBootstrapDnsObj?.server ?? '' },
      set(v: string) {
        if (this.directBootstrapDnsObj && this.directBootstrapDnsObj.tag) this.directBootstrapDnsObj.server = v
        if (this.directBootstrapDnsObj?.tls) this.directBootstrapDnsObj.tls.server_name = v
      }
    },
    directBootstrapDnsPort: {
      get(): number { return this.directBootstrapDnsObj?.server_port ?? 53 },
      set(v: number) { if (this.directBootstrapDnsObj && this.directBootstrapDnsObj.tag) this.directBootstrapDnsObj.server_port = v }
    },
    directBootstrapDnsShowServer(): boolean { return !this.noServerTypes.includes(this.directBootstrapDnsType) },
    directBootstrapDnsUsesPath(): boolean { return jsonSubscriptionDNSUsesPath(this.directBootstrapDnsType) },
    directBootstrapDnsPath: {
      get(): string { return typeof this.directBootstrapDnsObj?.path === 'string' ? this.directBootstrapDnsObj.path : '' },
      set(v: string) { if (this.directBootstrapDnsObj && this.directBootstrapDnsObj.tag) this.directBootstrapDnsObj.path = v }
    },
    // TUN inbound bindings.
    tunAddress: {
      get(): string[] { return this.tunInbound?.address ?? [] },
      set(v: string[]) { if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.address = v }
    },
    tunMtu: {
      get(): number { return this.tunInbound?.mtu ?? 1500 },
      set(v: number) { if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.mtu = v }
    },
    tunExcludePackage: {
      get(): string[] { return this.tunInbound?.exclude_package ?? [] },
      set(v: string[]) { if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.exclude_package = v }
    },
  },
  methods: {
    defaultPortForDnsType(t: string): number {
	  if (['https', 'h3'].includes(t)) return 443
	  if (['tls', 'quic'].includes(t)) return 853
      return 53
    },
    syncDnsPathForType(dns: any, type: string) {
      if (!dns || typeof dns !== 'object') return
      if (jsonSubscriptionDNSUsesPath(type)) {
        dns.path = normalizeJSONSubscriptionDNSPath(dns.path)
      } else {
        delete dns.path
      }
    },
    onProxyDnsTypeChange(t: string) {
      const dns = this.proxyDnsObj
      if (!dns || !dns.tag) return
      const tlsTypes = ['tls', 'quic', 'h3', 'https']
      if (this.noServerTypes.includes(t)) {
        delete dns.server; delete dns.server_port; delete dns.tls; delete dns.domain_resolver
      } else {
		if (!dns.server) dns.server = ''
		dns.server_port = this.defaultPortForDnsType(t)
        if (tlsTypes.includes(t)) {
          if (!dns.tls) dns.tls = { enabled: true, insecure: false, min_version: '1.3', server_name: dns.server || '' }
        } else {
          delete dns.tls
        }
        if (!dns.domain_resolver) dns.domain_resolver = 'proxy-bootstrap-dns'
      }
      this.syncDnsPathForType(dns, t)
      this.updateJson()
    },
    onDirectDnsTypeChange(t: string) {
      const dns = this.directDnsObj
      if (!dns || !dns.tag) return
      const tlsTypes = ['tls', 'quic', 'h3', 'https']
      if (this.noServerTypes.includes(t)) {
        delete dns.server; delete dns.server_port; delete dns.tls; delete dns.domain_resolver
      } else {
		if (!dns.server) dns.server = ''
		dns.server_port = this.defaultPortForDnsType(t)
        if (tlsTypes.includes(t)) {
          if (!dns.tls) dns.tls = { enabled: true, insecure: false, min_version: '1.3', server_name: dns.server || '' }
        } else {
          delete dns.tls
        }
        if (!dns.domain_resolver) dns.domain_resolver = 'direct-bootstrap-dns'
      }
      this.syncDnsPathForType(dns, t)
      this.updateJson()
    },
    onProxyBootstrapDnsTypeChange(t: string) {
      const dns = this.proxyBootstrapDnsObj
      if (!dns || !dns.tag) return
      const tlsTypes = ['tls', 'quic', 'h3', 'https']
      if (this.noServerTypes.includes(t)) {
        delete dns.server; delete dns.server_port; delete dns.tls; delete dns.domain_resolver
      } else {
		if (!dns.server) dns.server = ''
		dns.server_port = this.defaultPortForDnsType(t)
        if (tlsTypes.includes(t)) {
          if (!dns.tls) dns.tls = { enabled: true, insecure: false, min_version: '1.3', server_name: dns.server || '' }
        } else {
          delete dns.tls
        }
      }
      this.syncDnsPathForType(dns, t)
      this.updateJson()
    },
    onDirectBootstrapDnsTypeChange(t: string) {
      const dns = this.directBootstrapDnsObj
      if (!dns || !dns.tag) return
      const tlsTypes = ['tls', 'quic', 'h3', 'https']
      if (this.noServerTypes.includes(t)) {
        delete dns.server; delete dns.server_port; delete dns.tls; delete dns.domain_resolver
      } else {
		if (!dns.server) dns.server = ''
		dns.server_port = this.defaultPortForDnsType(t)
        if (tlsTypes.includes(t)) {
          if (!dns.tls) dns.tls = { enabled: true, insecure: false, min_version: '1.3', server_name: dns.server || '' }
        } else {
          delete dns.tls
        }
      }
      this.syncDnsPathForType(dns, t)
      this.updateJson()
    },
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
