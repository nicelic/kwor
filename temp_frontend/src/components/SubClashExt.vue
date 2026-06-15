<template>
  <Editor
    v-model="enableEditor"
    :data="editorData"
    :visible="enableEditor"
    :title="$t('editor') + ' - ' + $t('setting.clashSub')"
    @close="enableEditor = false"
    @save="saveEditor"
    />
  <v-card>
    <!-- Basic settings: mixed port, LAN access, external controller, log level -->
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field type="number" v-model.number="mixedPort" min="1" max="65535" :label="$t('setting.mixedPort')" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch color="primary" v-model="allowLan" :label="$t('types.ts.allowLanAccess')" hide-details />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field v-model="externalController" :label="$t('basic.exp.extController')" hide-details></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select v-model="logLevel" :items="clashLogLevels" :label="$t('basic.log.title') + ' - ' + $t('basic.log.level')" hide-details></v-select>
      </v-col>
    </v-row>

    <!-- mihomo-specific settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="unifiedDelay" color="primary" label="统一延迟 (unified-delay)" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="tcpConcurrent" color="primary" label="TCP 并发 (tcp-concurrent)" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="3" lg="2">
        <v-select v-model="findProcessMode" :items="findProcessModeOptions" label="进程匹配模式" hide-details></v-select>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="storeSelected" color="primary" label="记住代理选择" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="storeFakeIp" color="primary" label="持久化 Fake-IP" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="dnsEnabled && dnsEnhancedMode === 'fake-ip'">
        <v-text-field
          v-model.lazy="dnsFakeIpTtl"
          label="fakeip_ttl（单位：s）"
          placeholder="留空不设置；示例：30 或 30s"
          hide-details
        ></v-text-field>
      </v-col>
    </v-row>

    <!-- TUN settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="tunEnabled" color="primary" :label="$t('setting.tun')" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="tunEnabled">
        <v-switch v-model="tunAutoRoute" color="primary" label="自动路由" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="tunEnabled && tunAutoRoute">
        <v-switch v-model="tunStrictRoute" color="primary" label="严格路由" hide-details />
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select v-model="tunStack" :items="tunStackOptions" label="TUN 模式" hide-details></v-select>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field type="number" v-model.number="tunMtu" hide-details label="MTU"></v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3" lg="3">
        <v-select
          v-model="tunAutoDetectInterface"
          :items="optionalBoolOptions"
          label="AutoDetectInterface（自动检测网卡）"
          hide-details
        ></v-select>
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3" lg="3">
        <v-select
          v-model="tunRecvmsgx"
          :items="optionalBoolOptions"
          label="recvmsgx(使用 recvmmsg 批量收UDP包)"
          hide-details
        ></v-select>
      </v-col>
    </v-row>
    <v-row v-if="tunEnabled">
      <v-col cols="12" sm="6" md="3" lg="3">
        <v-select
          v-model="tunSendmsgx"
          :items="optionalBoolOptions"
          label="sendmsgx(批量发送UDP包)"
          hide-details
        ></v-select>
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
        ></v-combobox>
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
        ></v-combobox>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="3">
        <v-select
          v-model="globalIpv6"
          :items="optionalBoolOptions"
          label="IPv6 总开关"
          hide-details
        ></v-select>
      </v-col>
    </v-row>

    <!-- DNS settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="dnsEnabled" color="primary" :label="$t('pages.dns')" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="dnsEnabled">
        <v-switch v-model="dnsIpv6" color="primary" label="DNS_IPv6" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="dnsEnabled">
        <v-switch v-model="dnsPreferH3" color="primary" label="prefer-h3" hide-details />
      </v-col>
    </v-row>
    <template v-if="dnsEnabled">
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select v-model="dnsEnhancedMode" :items="enhancedModeOptions" label="增强模式" hide-details></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="dnsEnhancedMode === 'fake-ip'">
          <v-combobox
            v-model="dnsFakeIpRange"
            :items="dnsFakeIpRangeOptions"
            label="fake-ip (fake-ip-range)"
            clearable
            hide-details
            placeholder="198.18.0.1/15"
          ></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="dnsEnhancedMode === 'fake-ip'">
          <v-combobox
            v-model="dnsFakeIpRange6"
            :items="dnsFakeIpRange6Options"
            label="fake-ip6 (fake-ip-range6)"
            clearable
            hide-details
            placeholder="fc00::/18"
          ></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2" v-if="dnsEnhancedMode === 'fake-ip' && dnsIpv6">
          <v-text-field
            type="number"
            v-model="dnsIpv6Timeout"
            min="0"
            label="ipv6-timeout"
            placeholder="100"
            hide-details
          ></v-text-field>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsDirectNameserver" :items="clashDirectNameserverOptions" label="(direct-nameserver)" multiple chips closable-chips hide-details></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="dnsDirectNameserver.length > 0">
          <v-switch v-model="dnsDirectNameserverFollowPolicy" color="primary" label="direct-nameserver-follow-policy" hide-details />
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsProxyServerNameserver" :items="clashProxyServerNameserverOptions" label="(proxy-server-nameserver)" multiple chips closable-chips hide-details></v-combobox>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsNameserver" :items="clashNameserverOptions" label="(nameserver)" multiple chips closable-chips hide-details></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsFallback" :items="clashFallbackOptions" label="(fallback)" multiple chips closable-chips hide-details></v-combobox>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-combobox v-model="dnsDefaultNameserver" :items="clashDefaultNameserverOptions" label="(default-nameserver)" multiple chips closable-chips hide-details></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="4" v-if="dnsEnhancedMode === 'fake-ip'">
          <v-combobox v-model="dnsFakeIpFilter" :items="clashFakeIpFilterDefaults" label="fake-ip 排除 (fake-ip-filter)" multiple chips closable-chips hide-details></v-combobox>
        </v-col>
      </v-row>
      <v-row>
        <v-col cols="12" sm="6" md="4">
          <v-select
            v-model="dnsFallbackFilterGeoip"
            :items="dnsGeoipBoolOptions"
            label="fallback-filter.geoip"
            hide-details
          ></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-combobox
            v-model="dnsFallbackFilterGeoipCode"
            :items="clashGeoipCodeOptions"
            label="fallback-filter.geoip-code"
            hide-details
            clearable
          ></v-combobox>
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
          ></v-combobox>
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
          ></v-combobox>
        </v-col>
      </v-row>
      <v-row
        v-for="(dnsSuffixRow, dnsSuffixIdx) in clashDnsSuffixRows"
        :key="`clash-dns-suffix-row-${dnsSuffixIdx}`"
        align="center"
      >
        <v-col cols="12" sm="6" md="4">
          <v-select
            v-model="dnsSuffixRow.targets"
            :items="clashDnsSuffixTargetOptions"
            label="dns-选择"
            multiple
            chips
            closable-chips
            clearable
            hide-details
          ></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="4">
          <v-select
            v-model="dnsSuffixRow.selections"
            :items="clashDnsSuffixSelectionOptions"
            label="dns后缀"
            multiple
            chips
            closable-chips
            clearable
            hide-details
          ></v-select>
        </v-col>
        <v-col cols="12" sm="12" md="2">
          <div class="d-flex align-center justify-end ga-1">
            <v-btn
              icon="mdi-arrow-up"
              size="small"
              variant="text"
              :disabled="dnsSuffixIdx === 0"
              @click="moveClashDnsSuffixRow(dnsSuffixIdx, -1)"
            ></v-btn>
            <v-btn
              icon="mdi-arrow-down"
              size="small"
              variant="text"
              :disabled="dnsSuffixIdx >= clashDnsSuffixRows.length - 1"
              @click="moveClashDnsSuffixRow(dnsSuffixIdx, 1)"
            ></v-btn>
            <v-btn
              icon="mdi-plus"
              size="small"
              variant="text"
              @click="insertClashDnsSuffixRow(dnsSuffixIdx)"
            ></v-btn>
            <v-btn
              v-if="canDeleteClashDnsSuffixRow(dnsSuffixIdx)"
              icon="mdi-delete"
              size="small"
              variant="text"
              @click="removeClashDnsSuffixRow(dnsSuffixIdx)"
            ></v-btn>
          </div>
        </v-col>
      </v-row>
      <v-row
        v-for="(dnsPolicyRow, dnsPolicyIdx) in clashDnsPolicyRows"
        :key="`clash-dns-policy-row-${dnsPolicyIdx}`"
        align="center"
      >
        <v-col cols="12" sm="6" md="2">
          <v-select
            v-model="dnsPolicyRow.matchType"
            :items="clashDnsPolicyMatchTypeOptions"
            label="规则类型"
            hide-details
          ></v-select>
        </v-col>
        <v-col cols="12" sm="8" md="4">
          <v-combobox
            v-if="dnsPolicyRow.matchType !== 'rule-set'"
            v-model="dnsPolicyRow.values"
            :items="getClashDnsPolicyValueOptions(dnsPolicyRow)"
            :label="dnsPolicyRow.matchType === 'rule-set' ? '规则集' : '匹配值'"
            multiple
            chips
            closable-chips
            hide-details
          ></v-combobox>
          <v-select
            v-else
            v-model="dnsPolicyRow.values"
            :items="getClashDnsPolicyValueOptions(dnsPolicyRow)"
            label="规则集"
            multiple
            chips
            closable-chips
            hide-details
          ></v-select>
        </v-col>
        <v-col cols="12" sm="4" md="2">
          <v-select
            v-model="dnsPolicyRow.routeTarget"
            :items="clashDnsPolicyRouteOptions"
            label="DNS 路由"
            hide-details
          ></v-select>
        </v-col>
        <v-col cols="12" sm="12" md="2">
          <div class="d-flex align-center justify-end ga-1">
            <v-btn
              icon="mdi-arrow-up"
              size="small"
              variant="text"
              :disabled="dnsPolicyIdx === 0"
              @click="moveClashDnsPolicyRow(dnsPolicyIdx, -1)"
            ></v-btn>
            <v-btn
              icon="mdi-arrow-down"
              size="small"
              variant="text"
              :disabled="dnsPolicyIdx >= clashDnsPolicyRows.length - 1"
              @click="moveClashDnsPolicyRow(dnsPolicyIdx, 1)"
            ></v-btn>
            <v-btn
              icon="mdi-plus"
              size="small"
              variant="text"
              @click="insertClashDnsPolicyRow(dnsPolicyIdx)"
            ></v-btn>
            <v-btn
              v-if="canDeleteClashDnsPolicyRow(dnsPolicyIdx)"
              icon="mdi-delete"
              size="small"
              variant="text"
              @click="removeClashDnsPolicyRow(dnsPolicyIdx)"
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
        ></v-combobox>
      </v-col>
    </v-row>

    <!-- Rule set source -->
    <v-row>
      <v-col cols="12" sm="6" md="3">
        <v-select v-model="ruleSetSource" :items="clashRuleSetSourceOptions" label="全局规则集来源" hide-details></v-select>
      </v-col>
    </v-row>

    <!-- Unified custom/ruleset rows -->
    <v-row
      v-for="(row, idx) in clashRuleRows"
      :key="`clash-rule-row-${idx}`"
      align="center"
    >
      <v-col cols="12" sm="3" md="2">
        <v-text-field
          v-model="row.name"
          :label="idx === 0 ? '名称（可选）' : '名称'"
          :hint="idx === 0 ? '提示：规则集可同名合并；自定义同名需同匹配类型，且不能与规则集重名。' : ''"
          :persistent-hint="idx === 0"
          hide-details="auto"
          placeholder="例如：CN"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-select
          v-model="row.kind"
          :items="clashRuleKindOptions"
          label="规则类型"
          hide-details
        ></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-select
          v-if="row.kind === 'custom'"
          v-model="row.customType"
          :items="clashDomainIpTypes"
          :label="idx === 0 ? '自定义匹配类型' : '匹配类型'"
          hide-details
        ></v-select>
        <v-select
          v-else
          v-model="row.ruleSetScope"
          :items="clashRuleSetScopeOptions"
          label="规则集类型"
          hide-details
        ></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2" v-if="row.kind === 'ruleset'">
        <v-select
          v-model="row.ruleSetSourceOverride"
          :items="clashRuleSetSourceOverrideOptions"
          label="规则集来源"
          hide-details
        ></v-select>
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
        ></v-combobox>
      </v-col>
      <v-col cols="12" sm="4" md="2">
        <v-select
          v-model="row.route"
          :items="clashCustomRouteOptions"
          :label="row.name && row.name.trim() ? '路由（设置名称后禁用）' : '路由'"
          :disabled="Boolean(row.name && row.name.trim())"
          hide-details
        ></v-select>
      </v-col>
      <v-col cols="12" sm="4" md="2">
        <v-select
          :model-value="getClashRowNoResolveDisplayValue(row)"
          :items="dnsGeoipBoolOptions"
          label="no-resolve"
          :disabled="isClashRowNoResolveDisabled(row)"
          hide-details
          @update:modelValue="setClashRowNoResolve(row, $event)"
        ></v-select>
      </v-col>
      <v-col cols="12" sm="12" md="2">
        <div class="d-flex align-center justify-end ga-1">
          <v-btn
            icon="mdi-arrow-up"
            size="small"
            variant="text"
            :disabled="idx === 0"
            @click="moveClashRuleRow(idx, -1)"
          ></v-btn>
          <v-btn
            icon="mdi-arrow-down"
            size="small"
            variant="text"
            :disabled="idx >= clashRuleRows.length - 1"
            @click="moveClashRuleRow(idx, 1)"
          ></v-btn>
          <v-btn
            icon="mdi-plus"
            size="small"
            variant="text"
            @click="insertClashRuleRow(idx)"
          ></v-btn>
          <v-btn
            v-if="canDeleteClashRuleRow(idx)"
            icon="mdi-delete"
            size="small"
            variant="text"
            @click="removeClashRuleRow(idx)"
          ></v-btn>
        </div>
      </v-col>
    </v-row>

    <!-- Update method and interval -->
    <v-row>
      <v-col cols="12" sm="3" md="2">
        <v-select v-model="updateMethod" :items="clashUpdateMethodOptions" label="更新方式" hide-details></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-text-field v-model="updateInterval" label="更新时间，例如 1m 1h 1d" hide-details placeholder="1d"></v-text-field>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-select
          v-model="clashNoResolveGlobal"
          :items="optionalBoolOptions"
          label="no-resolve_全局开关"
          hide-details
        ></v-select>
      </v-col>
    </v-row>

    <!-- Final route outbound -->
    <v-row>
      <v-col cols="12" sm="3" md="2">
        <v-select v-model="routeFinal" :items="clashRouteFinalOptions" label="路由最终出口（都不匹配时）" hide-details></v-select>
      </v-col>
    </v-row>

    <!-- Latency test -->
    <v-row>
      <v-col cols="12" sm="6" md="6">
        <v-combobox v-model="latencyTestUrl" :items="clashLatencyTestUrlOptions" label="真实延迟测试链接" hide-details></v-combobox>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3">
        <v-text-field
          v-model="latencyTestInterval"
          label="测试延迟间隔（自动切换代理相关）"
          hide-details="auto"
          hint="Mihomo：仅支持秒，必须使用 s（例如 30s）"
          persistent-hint
          :error-messages="latencyTestIntervalError ? [latencyTestIntervalError] : []"
          placeholder="例如 30s"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3">
        <v-text-field
          v-model="latencyTolerance"
          label="延迟容差"
          hide-details="auto"
          hint="单位为 ms，可留空；填写时仅输入数字"
          persistent-hint
          :error-messages="latencyToleranceError ? [latencyToleranceError] : []"
          placeholder="例如 50（不要写 ms）"
        ></v-text-field>
      </v-col>
    </v-row>

    <!-- Sniffer toggle -->
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableSniff" color="primary" label="sniffer (嗅探)" hide-details />
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableRejectQuic" color="primary" label="屏蔽UDP 80,443,2443,4443,6443,8080,8081,8443" hide-details />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="12" md="8" lg="6">
        <v-text-field
          v-model="rejectUdpPortsInput"
          label="自定义屏蔽 UDP 端口/端口范围"
          hide-details="auto"
          hint="端口（端口范围）之间支持中英文逗号；范围支持 - / : / ： / ; / ；，例如 443,888-999 或 443，888：999"
          persistent-hint
          :error-messages="rejectUdpPortsInputError ? [rejectUdpPortsInputError] : []"
          placeholder="例如 443,888-999"
        ></v-text-field>
      </v-col>
    </v-row>
    <v-row v-if="enableSniff">
      <v-col cols="12" sm="6" md="4" lg="3">
        <v-select
          v-model="snifferOverrideDestination"
          :items="optionalBoolOptions"
          label="override-destination"
          hide-details
        ></v-select>
      </v-col>
    </v-row>
    <v-row v-if="enableSniff">
      <v-col cols="12" sm="6" md="4" lg="3">
        <v-select
          v-model="snifferForceDnsMapping"
          :items="optionalBoolOptions"
          label="force-dns-mapping"
          hint="如果嗅探出域名，但 fake-ip 表中没有映射，强制建立 DNS 映射关系"
          persistent-hint
          hide-details="auto"
        ></v-select>
      </v-col>
    </v-row>
    <v-row v-if="enableSniff">
      <v-col cols="12" sm="6" md="4" lg="3">
        <v-select
          v-model="snifferParsePureIp"
          :items="optionalBoolOptions"
          label="parse-pure-ip（嗅探域名）"
          hide-details
        ></v-select>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="dnsUseSystemHosts"
          :items="optionalBoolOptions"
          label="use-system-hosts"
          hide-details
        ></v-select>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="dnsUseHosts"
          :items="optionalBoolOptions"
          label="use-hosts"
          hide-details
        ></v-select>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="12" md="8" lg="6">
        <v-combobox
          v-model="clashHostsEntries"
          :items="[]"
          :delimiters="[]"
          label="hosts"
          placeholder="示例：*.clash.dev: 127.0.0.1, alpha.clash.dev: [::1]"
          multiple
          chips
          closable-chips
          hide-details
        ></v-combobox>
      </v-col>
    </v-row>

    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch
          v-model="mihomoKeepAlive"
          color="primary"
          label="mihomo_keep-alive"
          hide-details
        />
      </v-col>
    </v-row>

    <v-row v-if="mihomoKeepAlive">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="disableKeepAlive"
          :items="dnsGeoipBoolOptions"
          label="disable-keep-alive"
          hide-details
        ></v-select>
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
        ></v-text-field>
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
        ></v-text-field>
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
  clashRuleSetSourceOptions,
  clashDomainIpTypes,
  clashGeositeNameOptions,
  clashGeoipNameOptions,
  clashUpdateMethodOptions,
  clashLatencyTestUrlOptions,
  clashRouteFinalOptions,
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
  props: ['settings'],
  components: { Editor },
  mixins: [SubClashExtMixin],
  data() {
    return {
      // Reactive state.
      metaJson: {} as any,
      enableEditor: false,
      menu: false,
      _uiConfigLoaded: false,
      _suspendClashRegeneration: false,

      // Clash rule rows (independent from JSON sub rule rows).
      ruleSetSource: 'metacubex_cdn' as string,
      clashNoResolveGlobal: null as boolean | null,
      resolvedRuleSetUrls: {} as Record<string, { url: string; source: string }>,
      ruleSetResolutionRunToken: 0,
      clashRuleRows: [
        { kind: 'custom', name: '', customType: 'DOMAIN-KEYWORD', ruleSetScope: 'domain', ruleSetSourceOverride: null as string | null, route: 'REJECT', noResolve: true, values: [] as string[] },
      ] as Array<{ kind: string; name: string; customType: string; ruleSetScope: string; ruleSetSourceOverride: string | null; route: string; noResolve: boolean; values: string[] }>,
      clashDnsPolicyRows: [
        { matchType: 'geosite', routeTarget: 'nameserver', values: [] as string[] },
      ] as Array<{ matchType: string; routeTarget: string; values: string[] }>,
      clashDnsSuffixRows: [
        { targets: [] as string[], selections: [] as string[] },
      ] as Array<{ targets: string[]; selections: string[] }>,
      clashDnsSuffixAppliedRowsSnapshot: [] as Array<{ targets: string[]; selections: string[] }>,
      clashRuleKindOptions: [
        { title: '自定义匹配', value: 'custom' },
        { title: '规则集', value: 'ruleset' },
      ],
      clashDnsPolicyMatchTypeOptions: [
        { title: '域名/通配 (domain/wildcard)', value: 'domain' },
        { title: 'Geosite (geosite)', value: 'geosite' },
        { title: '规则集 (rule-set)', value: 'rule-set' },
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
        { title: '节点选择', value: '节点选择' },
        { title: 'proxy', value: 'proxy' },
        { title: 'disable-ipv4=true', value: 'disable-ipv4=true' },
        { title: 'disable-ipv6=true', value: 'disable-ipv6=true' },
        { title: 'skip-cert-verify=true', value: 'skip-cert-verify=true' },
        { title: 'h3=true', value: 'h3=true' },
      ],
      clashRuleSetScopeOptions: [
        { title: '域名', value: 'domain' },
        { title: 'IP', value: 'ip' },
      ],
      clashCustomRouteOptions: [
        { title: '屏蔽 (REJECT)', value: 'REJECT' },
        { title: '直连 (DIRECT)', value: 'DIRECT' },
        { title: '代理 (Proxy)', value: 'Proxy' },
      ],
      updateMethod: '全球直连' as string,
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
      snifferOverrideDestination: null as boolean | null,
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
      clashRuleSetSourceOptions,
      clashRuleSetSourceOverrideOptions: [
        { title: '使用全局规则集来源', value: null as string | null },
        ...clashRuleSetSourceOptions,
      ],
      clashDomainIpTypes,
      clashGeositeNameOptions: clashGeositeNameOptions.filter((item: string) => item.trim().length > 0),
      clashGeoipNameOptions: clashGeoipNameOptions.filter((item: string) => item.trim().length > 0),
      clashUpdateMethodOptions,
      clashLatencyTestUrlOptions,
      clashRouteFinalOptions,
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
      findProcessModeOptions,
    }
  },
}
</script>
