<template>
  <div class="settings-clash-sub-manage">
    <!-- 高级代码编辑器弹窗 -->
    <Editor
      v-if="enableEditor"
      v-model="enableEditor"
      :data="editorData"
      :visible="enableEditor"
      :title="$t('editor') + ' - ' + $t('setting.clashSub')"
      @close="enableEditor = false"
      @save="saveEditor"
    />

    <!-- 数据超出表单限制时的警示与 Editor 引导 -->
    <v-card v-if="formRowsTooLarge" rounded="xl" variant="outlined" class="mb-4">
      <v-alert type="warning" variant="tonal" density="comfortable" class="ma-4">
        {{ $t('subscriptionEditor.formRowsTooLarge') }}
      </v-alert>
      <v-card-actions class="px-4 pb-4">
        <v-spacer />
        <v-btn
          color="primary"
          variant="outlined"
          prepend-icon="mdi-code-json"
          @click="openEditor"
        >
          {{ $t('editor') }}
        </v-btn>
      </v-card-actions>
    </v-card>

    <!-- 主表单区域 -->
    <div v-else @input.capture="onFormValueChange" @change.capture="onFormValueChange">
      <!-- 解析错误告警 -->
      <v-alert
        v-if="parseError"
        type="error"
        variant="tonal"
        density="comfortable"
        class="mb-4"
      >
        {{ parseError }}
      </v-alert>

      <!-- Card 1: 客户端基础与 Mihomo 特性 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-application-cog-outline</v-icon>
            <span>{{ $t('subscriptionEditor.clashCardBasic') }}</span>
          </div>
          <v-chip size="x-small" color="success" variant="outlined">
            {{ $t('setting.instantEffectBadge') }}
          </v-chip>
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-4">
          <!-- 基础端口、局域网访问与控制器 -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-text-field
                type="number"
                v-model.number="mixedPort"
                min="1"
                max="65535"
                :label="$t('setting.mixedPort')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-numeric"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="allowLan"
                color="primary"
                :label="$t('types.ts.allowLanAccess')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-text-field
                v-model="externalController"
                :label="$t('basic.exp.extController')"
                placeholder="127.0.0.1:9090"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-api"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-select
                v-model="logLevel"
                :items="clashLogLevels"
                :label="$t('basic.log.title') + ' - ' + $t('basic.log.level')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-format-list-bulleted-type"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- Mihomo 专有特性 -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="unifiedDelay"
                color="primary"
                :label="$t('subscriptionEditor.unifiedDelay')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="tcpConcurrent"
                color="primary"
                :label="$t('subscriptionEditor.tcpConcurrent')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="storeSelected"
                color="primary"
                :label="$t('subscriptionEditor.storeSelected')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="storeFakeIp"
                color="primary"
                :label="$t('subscriptionEditor.storeFakeIp')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-row align="center" class="mt-1">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="findProcessMode"
                :items="findProcessModeOptions"
                :label="$t('subscriptionEditor.processMatchMode')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-application-search-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4" v-if="dnsEnabled && dnsEnhancedMode === 'fake-ip'">
              <v-text-field
                v-model.lazy="dnsFakeIpTtl"
                :label="$t('subscriptionEditor.fakeIpTtlLabel')"
                :placeholder="$t('subscriptionEditor.fakeIpTtlPlaceholder')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-timer-sand"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>
        </v-card-text>
      </v-card>

      <!-- Card 2: TUN 虚拟网卡与网络栈 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-green">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-lan-connect</v-icon>
            <span>{{ $t('subscriptionEditor.clashCardTun') }}</span>
          </div>
          <v-switch
            v-model="tunEnabled"
            color="primary"
            :label="$t('setting.tun')"
            density="compact"
            hide-details
            @update:model-value="onFormValueChange"
          />
        </v-card-title>
        <v-divider />
        <v-card-text v-if="tunEnabled" class="pt-4">
          <!-- TUN 开关与路由参数 -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="tunAutoRoute"
                color="primary"
                :label="$t('subscriptionEditor.autoRoute')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3" v-if="tunAutoRoute">
              <v-switch
                v-model="tunStrictRoute"
                color="primary"
                :label="$t('subscriptionEditor.strictRoute')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-select
                v-model="tunStack"
                :items="tunStackOptions"
                :label="$t('subscriptionEditor.tunMode')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-layers-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-text-field
                type="number"
                v-model.number="tunMtu"
                hide-details
                label="MTU"
                density="comfortable"
                prepend-inner-icon="mdi-speedometer"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-row align="center" class="mt-1">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="tunAutoDetectInterface"
                :items="optionalBoolOptions"
                :label="$t('subscriptionEditor.autoDetectInterface')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="tunRecvmsgx"
                :items="optionalBoolOptions"
                :label="$t('subscriptionEditor.recvmsgx')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="tunSendmsgx"
                :items="optionalBoolOptions"
                :label="$t('subscriptionEditor.sendmsgx')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- 遵守手册规范：TUN inet4 与 inet6 地址在桌面端也按上下两行单列排列，IPv6 位于 IPv4 下方 -->
          <v-row class="mt-2">
            <v-col cols="12">
              <v-combobox
                v-model="tunInet4Address"
                :items="tunInet4AddressOptions"
                label="inet4-address"
                multiple
                chips
                closable-chips
                clearable
                density="comfortable"
                hide-details
                placeholder="198.18.0.1/30"
                prepend-inner-icon="mdi-ip-network-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>
          <v-row class="mt-1">
            <v-col cols="12">
              <v-combobox
                v-model="tunInet6Address"
                :items="tunInet6AddressOptions"
                label="inet6-address"
                multiple
                chips
                closable-chips
                clearable
                density="comfortable"
                hide-details
                placeholder="fdfe:dcba:9876::1/126"
                prepend-inner-icon="mdi-ip-network-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- TUN 排除包与全局 IPv6 -->
          <v-row class="mt-1" align="center">
            <v-col cols="12" sm="8" md="6">
              <v-combobox
                v-model="tunExcludePackage"
                :items="['ir.mci.ecareapp','com.myirancell']"
                chips
                multiple
                closable-chips
                density="comfortable"
                hide-details
                :label="$t('setting.excludePkg')"
                prepend-inner-icon="mdi-package-variant-closed-remove"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="4" md="3">
              <v-select
                v-model="globalIpv6"
                :items="optionalBoolOptions"
                :label="$t('subscriptionEditor.globalIpv6')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>
        </v-card-text>
      </v-card>

      <!-- Card 3: DNS 解析与高级分流策略 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-dns-outline</v-icon>
            <span>{{ $t('subscriptionEditor.clashCardDns') }}</span>
          </div>
          <v-switch
            v-model="dnsEnabled"
            color="primary"
            :label="$t('pages.dns')"
            density="compact"
            hide-details
            @update:model-value="onFormValueChange"
          />
        </v-card-title>
        <v-divider />
        <v-card-text v-if="dnsEnabled" class="pt-4">
          <!-- 遵守手册规范：DNS_IPv6 与 prefer-h3 为三态选择器，垂直放在父开关下方的同一列中 -->
          <v-row>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="dnsIpv6"
                :items="optionalBoolOptions"
                label="DNS_IPv6"
                density="comfortable"
                hide-details
                class="mb-3"
                prepend-inner-icon="mdi-ip-outline"
                @update:model-value="onFormValueChange"
              />
              <v-select
                v-model="dnsPreferH3"
                :items="optionalBoolOptions"
                label="prefer-h3"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-flash-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="dnsEnhancedMode"
                :items="enhancedModeOptions"
                :label="$t('subscriptionEditor.enhancedMode')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-tune"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- Fake-IP 范围与 IPv6 超时 -->
          <template v-if="dnsEnhancedMode === 'fake-ip'">
            <v-row class="mt-1">
              <v-col cols="12" sm="6" md="4">
                <v-combobox
                  v-model="dnsFakeIpRange"
                  :items="dnsFakeIpRangeOptions"
                  label="fake-ip (fake-ip-range)"
                  clearable
                  density="comfortable"
                  hide-details
                  placeholder="198.18.0.1/15"
                  prepend-inner-icon="mdi-ip-outline"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-combobox
                  v-model="dnsFakeIpRange6"
                  :items="dnsFakeIpRange6Options"
                  label="fake-ip6 (fake-ip-range6)"
                  clearable
                  density="comfortable"
                  hide-details
                  placeholder="fc00::/18"
                  prepend-inner-icon="mdi-ip-outline"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="4" v-if="dnsIpv6">
                <v-text-field
                  type="number"
                  v-model="dnsIpv6Timeout"
                  min="0"
                  label="ipv6-timeout"
                  placeholder="100"
                  density="comfortable"
                  hide-details
                  prepend-inner-icon="mdi-timer-outline"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
            </v-row>
          </template>

          <v-divider class="my-4" />

          <!-- 核心 DNS 服务器组（严格遵循手册中的字段名与语义说明） -->
          <div class="text-subtitle-2 font-weight-medium mb-2 d-flex align-center ga-1">
            <v-icon size="small" color="primary">mdi-server-network</v-icon>
            <span>{{ $t('subscriptionEditor.dnsServer') }}</span>
          </div>

          <v-row>
            <v-col cols="12" md="6">
              <v-combobox
                v-model="dnsDirectNameserver"
                :items="clashDirectNameserverOptions"
                label="(direct-nameserver)（direct 出口域名解析的 DNS 服务器）"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" md="6" v-if="dnsDirectNameserver.length > 0">
              <v-switch
                v-model="dnsDirectNameserverFollowPolicy"
                color="primary"
                label="direct-nameserver-follow-policy（流量是否遵守 DNS 解析路由规则）"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12" md="6">
              <v-combobox
                v-model="dnsProxyServerNameserver"
                :items="clashProxyServerNameserverOptions"
                label="(proxy-server-nameserver)（代理节点域名解析服务器）"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" md="6">
              <v-combobox
                v-model="dnsNameserver"
                :items="clashNameserverOptions"
                label="(nameserver)（默认的域名解析服务器）"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12" md="6">
              <v-combobox
                v-model="dnsFallback"
                :items="clashFallbackOptions"
                label="(fallback)（后备域名解析服务器）"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" md="6">
              <v-combobox
                v-model="dnsDefaultNameserver"
                :items="clashDefaultNameserverOptions"
                label="(default-nameserver)（解析 DNS 服务器的域名，必须为 IP，可为加密 DNS）"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- Fake-IP Filter & Fallback Filter -->
          <v-row class="mt-1">
            <v-col cols="12" md="6" v-if="dnsEnhancedMode === 'fake-ip'">
              <v-combobox
                v-model="dnsFakeIpFilter"
                :items="clashFakeIpFilterDefaults"
                :label="$t('subscriptionEditor.fakeIpFilter')"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-select
                v-model="dnsFallbackFilterGeoip"
                :items="dnsGeoipBoolOptions"
                label="fallback-filter.geoip"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-combobox
                v-model="dnsFallbackFilterGeoipCode"
                :items="clashGeoipCodeOptions"
                label="fallback-filter.geoip-code"
                hide-details
                clearable
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12" sm="6" md="6">
              <v-combobox
                v-model="dnsFallbackFilterIpcidr"
                :items="[]"
                label="fallback-filter.ipcidr"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-ip-network-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="6">
              <v-combobox
                v-model="dnsFallbackFilterDomain"
                :items="[]"
                label="fallback-filter.domain"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-domain"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- DNS Suffix 过滤行 (clashDnsSuffixRows) -->
          <div class="mb-2 d-flex align-center justify-space-between flex-wrap ga-2">
            <div class="text-subtitle-2 font-weight-medium d-flex align-center ga-1">
              <v-icon size="small" color="primary">mdi-filter-variant</v-icon>
              <span>{{ $t('subscriptionEditor.dnsSuffix') }}</span>
            </div>
          </div>

          <v-card
            v-for="(dnsSuffixRow, dnsSuffixIdx) in clashDnsSuffixRows"
            :key="dnsSuffixRow.id"
            variant="outlined"
            rounded="md"
            class="pa-3 mb-3"
          >
            <v-row dense align="center">
              <v-col cols="12" sm="6" md="5">
                <v-select
                  v-model="dnsSuffixRow.targets"
                  :items="clashDnsSuffixTargetOptions"
                  :label="$t('subscriptionEditor.dnsSelection')"
                  multiple
                  chips
                  closable-chips
                  clearable
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="5">
                <v-select
                  v-model="dnsSuffixRow.selections"
                  :items="clashDnsSuffixSelectionOptions"
                  :label="$t('subscriptionEditor.dnsSuffix')"
                  multiple
                  chips
                  closable-chips
                  clearable
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" md="2" class="d-flex align-center justify-end ga-1 flex-wrap">
                <v-btn
                  icon="mdi-arrow-up"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveUp')"
                  :aria-label="$t('subscriptionEditor.moveUp')"
                  :disabled="dnsSuffixIdx === 0"
                  @click="onFormValueChange(); moveClashDnsSuffixRow(dnsSuffixIdx, -1)"
                />
                <v-btn
                  icon="mdi-arrow-down"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveDown')"
                  :aria-label="$t('subscriptionEditor.moveDown')"
                  :disabled="dnsSuffixIdx >= clashDnsSuffixRows.length - 1"
                  @click="onFormValueChange(); moveClashDnsSuffixRow(dnsSuffixIdx, 1)"
                />
                <v-btn
                  icon="mdi-plus"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.add')"
                  :aria-label="$t('subscriptionEditor.add')"
                  @click="onFormValueChange(); insertClashDnsSuffixRow(dnsSuffixIdx)"
                />
                <v-btn
                  v-if="canDeleteClashDnsSuffixRow(dnsSuffixIdx)"
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  :title="$t('subscriptionEditor.remove')"
                  :aria-label="$t('subscriptionEditor.remove')"
                  @click="onFormValueChange(); removeClashDnsSuffixRow(dnsSuffixIdx)"
                />
              </v-col>
            </v-row>
          </v-card>

          <!-- DNS Policy 规则策略行 (clashDnsPolicyRows) -->
          <div class="mt-4 mb-2 d-flex align-center justify-space-between flex-wrap ga-2">
            <div class="text-subtitle-2 font-weight-medium d-flex align-center ga-1">
              <v-icon size="small" color="primary">mdi-shield-link-variant-outline</v-icon>
              <span>{{ $t('subscriptionEditor.dnsRoute') }}</span>
            </div>
          </div>

          <v-card
            v-for="(dnsPolicyRow, dnsPolicyIdx) in clashDnsPolicyRows"
            :key="dnsPolicyRow.id"
            variant="outlined"
            rounded="md"
            class="pa-3 mb-3"
          >
            <v-row dense align="center">
              <v-col cols="12" sm="4" md="3">
                <v-select
                  v-model="dnsPolicyRow.matchType"
                  :items="clashDnsPolicyMatchTypeOptions"
                  :label="$t('subscriptionEditor.ruleKind')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="8" md="5">
                <v-combobox
                  v-if="dnsPolicyRow.matchType !== 'rule-set'"
                  v-model="dnsPolicyRow.values"
                  :items="getClashDnsPolicyValueOptions(dnsPolicyRow)"
                  :label="dnsPolicyRow.matchType === 'rule-set' ? $t('subscriptionEditor.ruleSet') : $t('subscriptionEditor.matchValue')"
                  multiple
                  chips
                  closable-chips
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
                <v-select
                  v-else
                  v-model="dnsPolicyRow.values"
                  :items="getClashDnsPolicyValueOptions(dnsPolicyRow)"
                  :label="$t('subscriptionEditor.ruleSet')"
                  multiple
                  chips
                  closable-chips
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="2">
                <v-select
                  v-model="dnsPolicyRow.routeTarget"
                  :items="clashDnsPolicyRouteOptions"
                  :label="$t('subscriptionEditor.dnsRoute')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="2" class="d-flex align-center justify-end ga-1 flex-wrap">
                <v-btn
                  icon="mdi-arrow-up"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveUp')"
                  :aria-label="$t('subscriptionEditor.moveUp')"
                  :disabled="dnsPolicyIdx === 0"
                  @click="onFormValueChange(); moveClashDnsPolicyRow(dnsPolicyIdx, -1)"
                />
                <v-btn
                  icon="mdi-arrow-down"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveDown')"
                  :aria-label="$t('subscriptionEditor.moveDown')"
                  :disabled="dnsPolicyIdx >= clashDnsPolicyRows.length - 1"
                  @click="onFormValueChange(); moveClashDnsPolicyRow(dnsPolicyIdx, 1)"
                />
                <v-btn
                  icon="mdi-plus"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.add')"
                  :aria-label="$t('subscriptionEditor.add')"
                  @click="onFormValueChange(); insertClashDnsPolicyRow(dnsPolicyIdx)"
                />
                <v-btn
                  v-if="canDeleteClashDnsPolicyRow(dnsPolicyIdx)"
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  :title="$t('subscriptionEditor.remove')"
                  :aria-label="$t('subscriptionEditor.remove')"
                  @click="onFormValueChange(); removeClashDnsPolicyRow(dnsPolicyIdx)"
                />
              </v-col>
            </v-row>
          </v-card>
        </v-card-text>
      </v-card>

      <!-- Card 4: 规则集与路由分流策略 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-green">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-routes</v-icon>
            <span>{{ $t('subscriptionEditor.clashCardRules') }}</span>
          </div>
          <v-chip size="x-small" color="info" variant="outlined">
            {{ $t('subscriptionEditor.ruleSet') }}
          </v-chip>
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-4">
          <!-- 全局规则集来源与全局 no-resolve -->
          <v-row class="mb-3" align="center">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="ruleSetSource"
                :items="clashRuleSetSourceOptions"
                :label="$t('subscriptionEditor.globalRuleSetSource')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-source-branch"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-select
                v-model="clashNoResolveGlobal"
                :items="optionalBoolOptions"
                :label="$t('subscriptionEditor.globalNoResolve')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- 统一分流规则表 -->
          <v-card
            v-for="(row, idx) in clashRuleRows"
            :key="row.id"
            variant="outlined"
            rounded="md"
            class="pa-3 mb-3"
          >
            <v-row dense align="center">
              <v-col cols="12" sm="3" md="2">
                <v-text-field
                  v-model="row.name"
                  :label="idx === 0 ? $t('subscriptionEditor.optionalName') : $t('subscriptionEditor.name')"
                  :hint="idx === 0 ? $t('subscriptionEditor.ruleNameHelp') : ''"
                  :persistent-hint="idx === 0"
                  density="compact"
                  hide-details="auto"
                  :placeholder="$t('subscriptionEditor.exampleCN')"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="3" md="2">
                <v-select
                  v-model="row.kind"
                  :items="clashRuleKindOptions"
                  :label="$t('subscriptionEditor.ruleKind')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="3" md="2">
                <v-select
                  v-if="row.kind === 'custom'"
                  v-model="row.customType"
                  :items="clashDomainIpTypes"
                  :label="idx === 0 ? $t('subscriptionEditor.customMatchType') : $t('subscriptionEditor.matchType')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
                <v-select
                  v-else
                  v-model="row.ruleSetScope"
                  :items="clashRuleSetScopeOptions"
                  :label="$t('subscriptionEditor.ruleSetScope')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="3" md="2" v-if="row.kind === 'ruleset'">
                <v-select
                  v-model="row.ruleSetSourceOverride"
                  :items="getClashRuleSetSourceOverrideOptions(row.ruleSetScope)"
                  :label="$t('subscriptionEditor.ruleSetSource')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" :sm="row.kind === 'ruleset' ? 6 : 6" :md="row.kind === 'ruleset' ? 4 : 4">
                <v-combobox
                  v-model="row.values"
                  :items="row.kind === 'ruleset' ? getClashRuleSetNameOptions(row.ruleSetScope, row) : []"
                  :label="row.kind === 'custom' ? getTypeLabel(row.customType) : getRuleSetScopeLabel(row.ruleSetScope)"
                  density="compact"
                  hide-details
                  multiple
                  chips
                  closable-chips
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="3" md="2">
                <v-select
                  v-model="row.route"
                  :items="clashCustomRouteOptions"
                  :label="row.name && row.name.trim() ? $t('subscriptionEditor.routeDisabledByName') : $t('subscriptionEditor.route')"
                  :disabled="Boolean(row.name && row.name.trim())"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="3" md="2">
                <v-select
                  :model-value="getClashRowNoResolveDisplayValue(row)"
                  :items="dnsGeoipBoolOptions"
                  label="no-resolve"
                  :disabled="isClashRowNoResolveDisabled(row)"
                  density="compact"
                  hide-details
                  @update:modelValue="setClashRowNoResolve(row, $event); onFormValueChange()"
                />
              </v-col>
              <v-col cols="12" class="d-flex align-center justify-end ga-1 flex-wrap pt-2">
                <v-btn
                  icon="mdi-arrow-up"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveUp')"
                  :aria-label="$t('subscriptionEditor.moveUp')"
                  :disabled="idx === 0"
                  @click="onFormValueChange(); moveClashRuleRow(idx, -1)"
                />
                <v-btn
                  icon="mdi-arrow-down"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveDown')"
                  :aria-label="$t('subscriptionEditor.moveDown')"
                  :disabled="idx >= clashRuleRows.length - 1"
                  @click="onFormValueChange(); moveClashRuleRow(idx, 1)"
                />
                <v-btn
                  icon="mdi-plus"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.add')"
                  :aria-label="$t('subscriptionEditor.add')"
                  @click="onFormValueChange(); insertClashRuleRow(idx)"
                />
                <v-btn
                  v-if="canDeleteClashRuleRow(idx)"
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  :title="$t('subscriptionEditor.remove')"
                  :aria-label="$t('subscriptionEditor.remove')"
                  @click="onFormValueChange(); removeClashRuleRow(idx)"
                />
              </v-col>
            </v-row>
          </v-card>

          <!-- 最终出口与更新周期 -->
          <v-row class="mt-3" align="center">
            <v-col cols="12" sm="4" md="3">
              <v-select
                v-model="routeFinal"
                :items="clashRouteFinalOptions"
                :label="$t('subscriptionEditor.routeFinal')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-call-split"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="4" md="3">
              <v-select
                v-model="updateMethod"
                :items="clashUpdateMethodOptions"
                :label="$t('subscriptionEditor.updateMethod')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="4" md="4">
              <v-text-field
                v-model="updateInterval"
                :label="$t('subscriptionEditor.updateInterval')"
                hide-details="auto"
                :hint="$t('subscriptionEditor.clashRuleProviderIntervalHint')"
                persistent-hint
                :error-messages="ruleProviderIntervalError ? [ruleProviderIntervalError] : []"
                :placeholder="$t('subscriptionEditor.clashRuleProviderIntervalPlaceholder')"
                density="comfortable"
                prepend-inner-icon="mdi-history"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>
        </v-card-text>
      </v-card>

      <!-- Card 5: 协议嗅探与安全进阶 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-shield-search-outline</v-icon>
            <span>{{ $t('subscriptionEditor.clashCardAdvanced') }}</span>
          </div>
          <v-chip size="x-small" color="info" variant="outlined">
            {{ $t('setting.instantEffectBadge') }}
          </v-chip>
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-4">
          <!-- 延迟测试 -->
          <v-row>
            <v-col cols="12" md="6">
              <v-combobox
                v-model="latencyTestUrl"
                :items="clashLatencyTestUrlOptions"
                :label="$t('subscriptionEditor.latencyUrl')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-web"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-text-field
                v-model="latencyTestInterval"
                :label="$t('subscriptionEditor.latencyInterval')"
                hide-details="auto"
                :hint="$t('subscriptionEditor.clashIntervalHint')"
                persistent-hint
                :error-messages="latencyTestIntervalError ? [latencyTestIntervalError] : []"
                :placeholder="$t('subscriptionEditor.clashIntervalPlaceholder')"
                density="comfortable"
                prepend-inner-icon="mdi-timer-outline"
                @update:model-value="onFormValueChange"
              />
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
                density="comfortable"
                prepend-inner-icon="mdi-speedometer-slow"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- 嗅探 (Sniffer) 与 协议阻断 -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableSniff"
                color="primary"
                :label="$t('subscriptionEditor.sniffer')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="5">
              <v-switch
                v-model="enableRejectQuic"
                color="primary"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              >
                <template #label>
                  <div>
                    <div>{{ $t('subscriptionEditor.rejectQuicPorts') }}</div>
                    <div class="text-caption text-medium-emphasis">(80, 443, 2443, 4443, 6443, 8080, 8081, 8443)</div>
                  </div>
                </template>
              </v-switch>
            </v-col>
          </v-row>

          <v-row class="mt-1">
            <v-col cols="12" md="8">
              <v-text-field
                v-model="rejectUdpPortsInput"
                :label="$t('subscriptionEditor.customRejectUdpPorts')"
                hide-details="auto"
                :hint="$t('subscriptionEditor.udpPortsHint')"
                persistent-hint
                :error-messages="rejectUdpPortsInputError ? [rejectUdpPortsInputError] : []"
                :placeholder="$t('subscriptionEditor.udpPortsPlaceholder')"
                density="comfortable"
                prepend-inner-icon="mdi-numeric"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- Sniffer 参数配置 -->
          <template v-if="enableSniff">
            <v-row class="mt-2">
              <v-col cols="12" sm="6" md="4">
                <v-select
                  v-model="snifferOverrideDestination"
                  :items="optionalBoolOptions"
                  label="override-destination"
                  density="comfortable"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-select
                  v-model="snifferForceDnsMapping"
                  :items="optionalBoolOptions"
                  label="force-dns-mapping"
                  :hint="$t('subscriptionEditor.forceDnsMappingHint')"
                  persistent-hint
                  density="comfortable"
                  hide-details="auto"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="4">
                <v-select
                  v-model="snifferParsePureIp"
                  :items="optionalBoolOptions"
                  :label="$t('subscriptionEditor.parsePureIp')"
                  density="comfortable"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
            </v-row>
          </template>

          <v-divider class="my-4" />

          <!-- Hosts 设置 -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-select
                v-model="dnsUseSystemHosts"
                :items="optionalBoolOptions"
                :label="$t('subscriptionEditor.useSystemHosts')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-select
                v-model="dnsUseHosts"
                :items="optionalBoolOptions"
                :label="$t('subscriptionEditor.useHosts')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" md="6">
              <v-combobox
                v-model="clashHostsEntries"
                :items="[]"
                :delimiters="[]"
                label="hosts"
                :placeholder="$t('subscriptionEditor.hostsPlaceholder')"
                multiple
                chips
                closable-chips
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-server"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- Mihomo Keep-Alive -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="mihomoKeepAlive"
                color="primary"
                :label="$t('subscriptionEditor.mihomoKeepAlive')"
                density="comfortable"
                hide-details
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <template v-if="mihomoKeepAlive">
              <v-col cols="12" sm="6" md="3">
                <v-select
                  v-model="disableKeepAlive"
                  :items="dnsGeoipBoolOptions"
                  :label="$t('subscriptionEditor.disableKeepAlive')"
                  density="comfortable"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <v-text-field
                  type="number"
                  v-model.number="keepAliveIdle"
                  min="0"
                  :label="$t('subscriptionEditor.keepAliveIdle')"
                  density="comfortable"
                  hide-details
                  prepend-inner-icon="mdi-timer-sand"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <v-text-field
                  type="number"
                  v-model.number="keepAliveInterval"
                  min="0"
                  :label="$t('subscriptionEditor.keepAliveInterval')"
                  density="comfortable"
                  hide-details
                  prepend-inner-icon="mdi-timer-outline"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
            </template>
          </v-row>
        </v-card-text>
        <v-divider />
        <!-- 底部快捷工具栏 -->
        <v-card-actions class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-caption text-medium-emphasis">
            {{ $t('subscriptionEditor.editorQuickOpenDesc') }}
          </div>
          <v-btn
            variant="tonal"
            color="primary"
            prepend-icon="mdi-code-json"
            @click="openEditor"
          >
            {{ $t('subscriptionEditor.editorQuickOpen') }}
          </v-btn>
        </v-card-actions>
      </v-card>
    </div>
  </div>
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
  name: 'SettingsClashSubManage',
  props: ['settings', 'canonicalDefault', 'initialDirty', 'initialReset', 'initialDirtyBaseline', 'ruleSetSources'],
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
      formRowsTooLarge: false,

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
      snifferForceDnsMapping: true as boolean | null,
      snifferParsePureIp: true as boolean | null,
      enableRejectQuic: true,
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
.card-cyan {
  border-color: rgba(34, 211, 238, 0.45) !important;
  transition: border-color 0.2s ease;
}
.card-cyan:hover {
  border-color: rgba(34, 211, 238, 0.75) !important;
}
.card-green {
  border-color: rgba(74, 222, 128, 0.45) !important;
  transition: border-color 0.2s ease;
}
.card-green:hover {
  border-color: rgba(74, 222, 128, 0.75) !important;
}
</style>
