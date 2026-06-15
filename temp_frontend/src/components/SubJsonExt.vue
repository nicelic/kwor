<template>
  <Editor
    v-model="enableEditor"
    :data="editorData"
    :visible="enableEditor"
    :title="$t('editor') + ' - ' + $t('setting.jsonSub')"
    @close="enableEditor = false"
    @save="saveEditor"
    />
  <v-card>
    <!-- Server tls_store settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="enableServerTlsStore" color="primary" label="服务端证书库 tls_store" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="enableServerTlsStore">
        <v-select hide-details label="store" :items="tlsStoreOptions" v-model="serverTlsStore"></v-select>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="enableClientTlsStore" color="primary" label="客户端证书库 tls_store" hide-details />
      </v-col>
      <v-col cols="12" sm="4" md="2" lg="2" v-if="enableClientTlsStore">
        <v-select hide-details label="store" :items="tlsStoreOptions" v-model="clientTlsStore"></v-select>
      </v-col>
    </v-row>

    <!-- Log settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="enableLog" color="primary" :label="$t('basic.log.title')" hide-details />
      </v-col>
    </v-row>

    <v-row v-if="enableLog">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          hide-details
          :label="$t('basic.log.level')"
          :items="levels"
          v-model="subJsonExt.log.level">
        </v-select>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="subJsonExt.log.timestamp" color="primary" :label="$t('setting.timestamp')" hide-details />
      </v-col>
    </v-row>

    <!-- DNS switch -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="enableDns" color="primary" :label="$t('pages.dns')" hide-details />
      </v-col>
    </v-row>
    <!-- DNS settings -->
    <template v-if="enableDns">
      <v-row>
        <v-col cols="12" sm="6" md="4" lg="4">
          <v-row no-gutters>
            <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">代理流量 DNS</v-col>
            <v-col cols="4">
              <v-select hide-details :label="$t('type')" :items="dnsTypeOptions" density="compact" class="noGutters" v-model="proxyDnsType" @update:model-value="onProxyDnsTypeChange"></v-select>
            </v-col>
            <v-col cols="5" v-if="proxyDnsShowServer">
              <v-text-field v-model="proxyDnsServer" :label="$t('in.addr')" density="compact" class="noGutters" hide-details></v-text-field>
            </v-col>
            <v-col cols="3" v-if="proxyDnsShowServer">
              <v-text-field v-model.number="proxyDnsPort" :label="$t('in.port')" density="compact" type="number" class="noGutters" min="1" hide-details></v-text-field>
            </v-col>
          </v-row>
        </v-col>
        <v-col cols="12" sm="6" md="4" lg="4">
          <v-row no-gutters>
            <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">直连流量 DNS</v-col>
            <v-col cols="4">
              <v-select hide-details :label="$t('type')" :items="dnsTypeOptions" density="compact" class="noGutters" v-model="directDnsType" @update:model-value="onDirectDnsTypeChange"></v-select>
            </v-col>
            <v-col cols="5" v-if="directDnsShowServer">
              <v-text-field v-model="directDnsServer" :label="$t('in.addr')" density="compact" class="noGutters" hide-details></v-text-field>
            </v-col>
            <v-col cols="3" v-if="directDnsShowServer">
              <v-text-field v-model.number="directDnsPort" :label="$t('in.port')" density="compact" type="number" class="noGutters" min="1" hide-details></v-text-field>
            </v-col>
          </v-row>
        </v-col>
      </v-row>
      <!-- DNS bootstrap settings -->
      <v-row>
        <v-col cols="12" sm="6" md="4" lg="4">
          <v-row no-gutters>
            <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">代理流量 bootstrap DNS</v-col>
            <v-col cols="4">
              <v-select hide-details :label="$t('type')" :items="dnsTypeOptions" density="compact" class="noGutters" v-model="proxyBootstrapDnsType" @update:model-value="onProxyBootstrapDnsTypeChange"></v-select>
            </v-col>
            <v-col cols="5" v-if="proxyBootstrapDnsShowServer">
              <v-text-field v-model="proxyBootstrapDnsServer" :label="$t('in.addr')" density="compact" class="noGutters" hide-details></v-text-field>
            </v-col>
            <v-col cols="3" v-if="proxyBootstrapDnsShowServer">
              <v-text-field v-model.number="proxyBootstrapDnsPort" :label="$t('in.port')" density="compact" type="number" class="noGutters" min="1" hide-details></v-text-field>
            </v-col>
          </v-row>
        </v-col>
        <v-col cols="12" sm="6" md="4" lg="4">
          <v-row no-gutters>
            <v-col cols="12" class="v-card-subtitle" style="margin-top: -5px;">直连流量 bootstrap DNS</v-col>
            <v-col cols="4">
              <v-select hide-details :label="$t('type')" :items="dnsTypeOptions" density="compact" class="noGutters" v-model="directBootstrapDnsType" @update:model-value="onDirectBootstrapDnsTypeChange"></v-select>
            </v-col>
            <v-col cols="5" v-if="directBootstrapDnsShowServer">
              <v-text-field v-model="directBootstrapDnsServer" :label="$t('in.addr')" density="compact" class="noGutters" hide-details></v-text-field>
            </v-col>
            <v-col cols="3" v-if="directBootstrapDnsShowServer">
              <v-text-field v-model.number="directBootstrapDnsPort" :label="$t('in.port')" density="compact" type="number" class="noGutters" min="1" hide-details></v-text-field>
            </v-col>
          </v-row>
        </v-col>
      </v-row>
      <!-- DNS row 3: final_dns and query_type switch -->
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select hide-details label="final_dns" :items="dnsFinalOptions" v-model="subJsonExt.dns.final"></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-switch v-model="enableDnsQueryType" color="primary" label="query_type" hide-details />
        </v-col>
      </v-row>
      <!-- DNS row 4: fakeip switch and fakeip ranges -->
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-switch v-model="enableFakeip" color="primary" label="fakeip" hide-details />
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
          ></v-combobox>
        </v-col>
      </v-row>
      <!-- DNS row 5: sortable route rows -->
      <v-row
        v-for="(dnsRouteRow, dnsRowIdx) in dnsRouteRows"
        :key="`dns-route-row-${dnsRowIdx}`"
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
          ></v-combobox>
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
          ></v-select>
        </v-col>
        <v-col cols="12" sm="12" md="2">
          <div class="d-flex align-center justify-end ga-1">
            <v-btn
              icon="mdi-arrow-up"
              size="small"
              variant="text"
              :disabled="dnsRowIdx === 0"
              @click="moveDnsRouteRow(dnsRowIdx, -1)"
            ></v-btn>
            <v-btn
              icon="mdi-arrow-down"
              size="small"
              variant="text"
              :disabled="dnsRowIdx >= dnsRouteRows.length - 1"
              @click="moveDnsRouteRow(dnsRowIdx, 1)"
            ></v-btn>
            <v-btn
              v-if="dnsRouteRow.kind === 'rule-set'"
              icon="mdi-plus"
              size="small"
              variant="text"
              @click="insertDnsRouteRow(dnsRowIdx)"
            ></v-btn>
            <v-btn
              v-if="dnsRouteRow.kind === 'rule-set' && canDeleteDnsRouteRow(dnsRowIdx)"
              icon="mdi-delete"
              size="small"
              variant="text"
              @click="removeDnsRouteRow(dnsRowIdx)"
            ></v-btn>
          </div>
        </v-col>
      </v-row>
      <!-- DNS row 6: resolver strategy -->
      <v-row>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select v-model="dnsStrategy" :items="dnsStrategyOptions" label="域名解析策略" hide-details></v-select>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select
            v-model="routeDefaultDomainResolver"
            :items="dnsFinalOptions"
            label="default_domain_resolve"
            hide-details
          ></v-select>
        </v-col>
      </v-row>
    </template>

    <!-- Inbound settings -->
    <v-row>
      <v-col cols="12" sm="4" md="2" lg="2">
        <v-switch v-model="enableInb" color="primary" label="Inbound" hide-details />
      </v-col>
    </v-row>
    <template v-if="enableInb">
      <!-- TUN switches -->
      <v-row>
        <v-col cols="12" sm="4" md="2" lg="2">
          <v-switch v-model="enableTun" color="primary" label="tun" hide-details />
        </v-col>
        <v-col cols="12" sm="4" md="2" lg="2" v-if="enableTun">
          <v-switch v-model="autoRoute" color="primary" label="自动路由" hide-details />
        </v-col>
        <v-col cols="12" sm="4" md="2" lg="2" v-if="enableTun && autoRoute">
          <v-switch v-model="strictRoute" color="primary" label="严格路由" hide-details />
        </v-col>
        <v-col cols="12" sm="4" md="3" lg="3" v-if="enableTun">
          <v-switch v-model="endpointIndependentNat" color="primary" label="endpoint_independent_nat" hide-details />
        </v-col>
      </v-row>
      <!-- TUN address and MTU -->
      <v-row v-if="enableTun">
        <v-col cols="12" sm="6" md="3">
          <v-combobox v-model="tunAddress" :items="defaultTunAddress" chips multiple hide-details label="TUN网卡IP"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-text-field type="number" v-model.number="tunMtu" hide-details label="MTU"></v-text-field>
        </v-col>
      </v-row>
      <v-row v-if="enableTun">
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-select v-model="tunMode" :items="['system', 'mixed', 'gvisor']" label="TUN 模式" hide-details></v-select>
        </v-col>
      </v-row>
      <!-- Mixed inbound listen settings -->
      <v-row>
        <v-col cols="12" sm="4" md="3" lg="2">
          <v-text-field v-model="mixedListen" label="默认监听地址" hide-details placeholder="127.0.0.1"></v-text-field>
        </v-col>
        <v-col cols="12" sm="2" md="2" lg="1">
          <v-text-field type="number" v-model.number="mixedListenPort" label="端口" hide-details placeholder="2080"></v-text-field>
        </v-col>
      </v-row>
      <!-- TUN package exclusion and platform proxy -->
      <v-row v-if="enableTun">
        <v-col cols="12" sm="6" md="3">
          <v-combobox v-model="tunExcludePackage" :items="['ir.mci.ecareapp','com.myirancell']" chips multiple hide-details :label="$t('setting.excludePkg')"></v-combobox>
        </v-col>
        <v-col cols="12" sm="6" md="3" lg="2">
          <v-switch v-model="platformProxy" hide-details color="primary" label="平台 HTTP 代理"></v-switch>
        </v-col>
      </v-row>
    </template>

    <!-- Rule set source -->
    <v-row>
      <v-col cols="12" sm="6" md="3">
        <v-select v-model="ruleSetSource" :items="ruleSetSourceOptions" label="全局规则集来源" hide-details></v-select>
      </v-col>
    </v-row>
        <!-- Match/ruleset unified list -->
    <v-row
      v-for="(row, idx) in ruleRows"
      :key="`rule-row-${idx}`"
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
          :items="ruleKindOptions"
          label="规则类型"
          hide-details
        ></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-select
          v-if="row.kind === 'custom'"
          v-model="row.customType"
          :items="domainIpTypes"
          :label="idx === 0 ? '自定义匹配类型' : '匹配类型'"
          hide-details
        ></v-select>
        <v-select
          v-else
          v-model="row.ruleSetScope"
          :items="ruleSetScopeOptions"
          label="规则集类型"
          hide-details
        ></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2" v-if="row.kind === 'ruleset'">
        <v-select
          v-model="row.ruleSetSourceOverride"
          :items="ruleSetSourceOverrideOptions"
          label="规则集来源"
          hide-details
        ></v-select>
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
        ></v-combobox>
      </v-col>
      <v-col cols="12" sm="4" md="2">
        <v-select
          v-model="row.route"
          :items="customRouteOptions"
          :label="row.name && row.name.trim() ? '路由（设置名称后禁用）' : '路由'"
          :disabled="Boolean(row.name && row.name.trim())"
          hide-details
        ></v-select>
      </v-col>
      <v-col cols="12" sm="12" md="2">
        <div class="d-flex align-center justify-end ga-1">
          <v-btn
            icon="mdi-arrow-up"
            size="small"
            variant="text"
            :disabled="idx === 0"
            @click="moveRuleRow(idx, -1)"
          ></v-btn>
          <v-btn
            icon="mdi-arrow-down"
            size="small"
            variant="text"
            :disabled="idx >= ruleRows.length - 1"
            @click="moveRuleRow(idx, 1)"
          ></v-btn>
          <v-btn
            icon="mdi-plus"
            size="small"
            variant="text"
            @click="insertRuleRow(idx)"
          ></v-btn>
          <v-btn
            v-if="canDeleteRuleRow(idx)"
            icon="mdi-delete"
            size="small"
            variant="text"
            @click="removeRuleRow(idx)"
          ></v-btn>
        </div>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="3" md="2">
        <v-select v-model="updateMethod" :items="updateMethodOptions" label="更新方式" hide-details></v-select>
      </v-col>
      <v-col cols="12" sm="3" md="2">
        <v-text-field v-model="updateInterval" label="更新时间，例如 1m 1h 1d" hide-details placeholder="1d"></v-text-field>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="3" md="2">
        <v-select v-model="routeFinal" :items="routeFinalOptions" label="路由最终出口（都不匹配时）" hide-details></v-select>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="6">
        <v-combobox v-model="latencyTestUrl" :items="latencyTestUrlOptions" label="真实延迟测试链接" hide-details></v-combobox>
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3">
        <v-text-field
          v-model="latencyTestInterval"
          label="测试延迟间隔（自动切换代理相关）"
          hide-details="auto"
          hint="Sing-box：必须带单位（s/m/h/d，不支持 ms）"
          persistent-hint
          :error-messages="latencyTestIntervalError ? [latencyTestIntervalError] : []"
          placeholder="例如 3m 或 30s"
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
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableRejectQuic" color="primary" label="拒绝quic" hide-details />
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableReject443Udp" color="primary" label="拒绝_443_udp" hide-details />
      </v-col>
    </v-row>
    <v-row>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableExp" color="primary" label="本地缓存（缓存删除数据困难）" hide-details />
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableSubClashApi" color="primary" label="clash_api" hide-details />
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableSniff" color="primary" label="sniff" hide-details />
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch v-model="enableHijackDns" color="primary" label="hijack-dns" hide-details />
      </v-col>
    </v-row>
    <v-row v-if="subJsonExt.experimental?.clash_api">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field
          v-model="subJsonExt.experimental.clash_api.external_controller"
          hide-details
          label="external_controller"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field
          v-model="subJsonExt.experimental.clash_api.secret"
          hide-details
          label="secret"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="subJsonExt.experimental.clash_api.default_mode"
          :items="clashApiModeOptions"
          hide-details
          label="default_mode"
        ></v-select>
      </v-col>
    </v-row>
    <v-row v-if="subJsonExt.experimental?.clash_api">
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-text-field
          v-model="subJsonExt.experimental.clash_api.external_ui"
          hide-details
          label="external_ui"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="12" md="6">
        <v-text-field
          v-model="subJsonExt.experimental.clash_api.external_ui_download_url"
          hide-details
          label="external_ui_download_url"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-select
          v-model="subJsonExt.experimental.clash_api.external_ui_download_detour"
          :items="subSelectorTagOptions"
          hide-details
          clearable
          label="external_ui_download_detour"
        ></v-select>
      </v-col>
    </v-row>
    <v-row v-if="subJsonExt.experimental?.clash_api">
      <v-col cols="12" sm="12" md="6">
        <v-text-field
          v-model="subClashApiOrigin"
          hide-details
          label="access_control_allow_origin (comma separated)"
        ></v-text-field>
      </v-col>
      <v-col cols="12" sm="6" md="3" lg="2">
        <v-switch
          v-model="subJsonExt.experimental.clash_api.access_control_allow_private_network"
          color="primary"
          label="allow_private_network"
          hide-details
        ></v-switch>
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
import { SubJsonExtMixin } from './SubJsonExtLogic'
import {
  levels,
  tunIpOptions,
  dnsStrategyOptions,
  tlsStoreOptions,
  ruleSetSourceOptions,
  domainIpTypes,
  ruleSetOptions,
  geositeNameOptions,
  geoipNameOptions,
  updateMethodOptions,
  latencyTestUrlOptions,
  clashApiModeOptions,
  subSelectorTagOptions,
  geositeList,
  geoList,
  geo,
  defaultInb,
} from './SubJsonExtConstants'

export default {
  props: ['settings'],
  components: { Editor },
  mixins: [SubJsonExtMixin],
  data() {
    return {
      // Reactive state
      subJsonExt: {} as any,
      menu: false,
      enableEditor: false,
      ruleSetSource: "karingx_github" as string,
      autoMatchedRuleSetUrls: {} as Record<string, { url: string; source: string }>,
      autoMatchRunToken: 0,
      dnsRouteRows: [
        { kind: "rule-set", server: "proxy-dns", ruleSet: [] as string[] },
      ] as Array<{ kind: string; server: string; ruleSet: string[] }>,
      ruleRows: [
        { kind: "custom", name: "", customType: "domain", ruleSetScope: "domain", ruleSetSourceOverride: null as string | null, route: "reject", values: [] as string[] },
      ] as Array<{ kind: string; name: string; customType: string; ruleSetScope: string; ruleSetSourceOverride: string | null; route: string; values: string[] }>,
      ruleKindOptions: [
        { title: "自定义匹配", value: "custom" },
        { title: "规则集", value: "ruleset" },
      ],
      ruleSetScopeOptions: [
        { title: "域名", value: "domain" },
        { title: "IP", value: "ip" },
      ],
      customRouteOptions: [
        { title: "屏蔽 (reject)", value: "reject" },
        { title: "直连 (direct)", value: "direct" },
        { title: "代理 (proxy)", value: "proxy" },
      ],
      updateMethod: "全球直连" as string,
      updateInterval: "1d" as string,
      routeFinal: "漏网之鱼" as string,
      routeFinalOptions: [
        { title: "节点选择", value: "节点选择" },
        { title: "自动选择", value: "自动选择" },
        { title: "全球直连", value: "全球直连" },
        { title: "全球拦截", value: "全球拦截" },
        { title: "漏网之鱼", value: "漏网之鱼" },
      ],
      clashApiModeOptions,
      subSelectorTagOptions,
      latencyTestUrl: "https://cp.cloudflare.com/generate_204" as string,
      latencyTestInterval: "3m" as string,
      latencyTolerance: "50" as string,
      enableSniff: true,
      enableHijackDns: true,
      enableRejectQuic: false,
      enableReject443Udp: false,
      _uiConfigLoaded: false,
      _suspendRuleRegeneration: false,

      // DNS server type options.
      dnsTypeOptions: ['udp', 'tcp', 'local', 'dhcp', 'tls', 'quic', 'h3', 'https'],
      noServerTypes: ['local', 'dhcp'],
      defaultTunAddress: ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],

      // Shared constant lists.
      levels,
      tunIpOptions,
      dnsStrategyOptions,
      tlsStoreOptions,
      ruleSetSourceOptions,
      ruleSetSourceOverrideOptions: [
        { title: "使用全局规则集来源", value: null as string | null },
        ...ruleSetSourceOptions,
      ],
      domainIpTypes,
      ruleSetOptions,
      geositeNameOptions: geositeNameOptions.filter((item: string) => item.trim().length > 0),
      geoipNameOptions: geoipNameOptions.filter((item: string) => item.trim().length > 0),
      updateMethodOptions,
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
      if (t === 'https') return 443
      if (['tls', 'quic', 'h3'].includes(t)) return 853
      return 53
    },
    onProxyDnsTypeChange(t: string) {
      const dns = this.proxyDnsObj
      if (!dns || !dns.tag) return
      const tlsTypes = ['tls', 'quic', 'h3', 'https']
      if (this.noServerTypes.includes(t)) {
        delete dns.server; delete dns.server_port; delete dns.tls; delete dns.domain_resolver
      } else {
        if (!dns.server) {
          dns.server = ''
          dns.server_port = this.defaultPortForDnsType(t)
        }
        if (tlsTypes.includes(t)) {
          if (!dns.tls) dns.tls = { enabled: true, insecure: false, min_version: '1.3', server_name: dns.server || '' }
        } else {
          delete dns.tls
        }
        if (!dns.domain_resolver) dns.domain_resolver = 'proxy-bootstrap-dns'
      }
      this.updateJson()
    },
    onDirectDnsTypeChange(t: string) {
      const dns = this.directDnsObj
      if (!dns || !dns.tag) return
      const tlsTypes = ['tls', 'quic', 'h3', 'https']
      if (this.noServerTypes.includes(t)) {
        delete dns.server; delete dns.server_port; delete dns.tls; delete dns.domain_resolver
      } else {
        if (!dns.server) {
          dns.server = ''
          dns.server_port = this.defaultPortForDnsType(t)
        }
        if (tlsTypes.includes(t)) {
          if (!dns.tls) dns.tls = { enabled: true, insecure: false, min_version: '1.3', server_name: dns.server || '' }
        } else {
          delete dns.tls
        }
        if (!dns.domain_resolver) dns.domain_resolver = 'direct-bootstrap-dns'
      }
      this.updateJson()
    },
    onProxyBootstrapDnsTypeChange(t: string) {
      const dns = this.proxyBootstrapDnsObj
      if (!dns || !dns.tag) return
      const tlsTypes = ['tls', 'quic', 'h3', 'https']
      if (this.noServerTypes.includes(t)) {
        delete dns.server; delete dns.server_port; delete dns.tls; delete dns.domain_resolver
      } else {
        if (!dns.server) {
          dns.server = ''
          dns.server_port = this.defaultPortForDnsType(t)
        }
        if (tlsTypes.includes(t)) {
          if (!dns.tls) dns.tls = { enabled: true, insecure: false, min_version: '1.3', server_name: dns.server || '' }
        } else {
          delete dns.tls
        }
      }
      this.updateJson()
    },
    onDirectBootstrapDnsTypeChange(t: string) {
      const dns = this.directBootstrapDnsObj
      if (!dns || !dns.tag) return
      const tlsTypes = ['tls', 'quic', 'h3', 'https']
      if (this.noServerTypes.includes(t)) {
        delete dns.server; delete dns.server_port; delete dns.tls; delete dns.domain_resolver
      } else {
        if (!dns.server) {
          dns.server = ''
          dns.server_port = this.defaultPortForDnsType(t)
        }
        if (tlsTypes.includes(t)) {
          if (!dns.tls) dns.tls = { enabled: true, insecure: false, min_version: '1.3', server_name: dns.server || '' }
        } else {
          delete dns.tls
        }
      }
      this.updateJson()
    },
  },
}
</script>




