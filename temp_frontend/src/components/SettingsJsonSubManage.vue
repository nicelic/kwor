<template>
  <div class="settings-json-sub-manage">
    <!-- 高级代码编辑器弹窗 -->
    <Editor
      v-if="enableEditor"
      v-model="enableEditor"
      :data="editorData"
      :visible="enableEditor"
      :title="$t('editor') + ' - ' + $t('setting.jsonSub')"
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

      <!-- Card 1: 核心安全与系统日志 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-shield-lock-outline</v-icon>
            <span>{{ $t('subscriptionEditor.jsonCardCore') }}</span>
          </div>
          <v-chip size="x-small" color="success" variant="outlined">
            {{ $t('setting.instantEffectBadge') }}
          </v-chip>
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-4">
          <!-- Server & Client tls_store -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableServerTlsStore"
                color="primary"
                :label="$t('subscriptionEditor.serverTlsStore')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3" v-if="enableServerTlsStore">
              <v-select
                v-model="serverTlsStore"
                :items="tlsStoreOptions"
                label="Server Store"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-certificate-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableClientTlsStore"
                color="primary"
                :label="$t('subscriptionEditor.clientTlsStore')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3" v-if="enableClientTlsStore">
              <v-select
                v-model="clientTlsStore"
                :items="tlsStoreOptions"
                label="Client Store"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-certificate-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- Log Settings -->
          <v-row align="center">
            <v-col cols="12" sm="4" md="3">
              <v-switch
                v-model="enableLog"
                color="primary"
                :label="$t('basic.log.title')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <template v-if="enableLog">
              <v-col cols="12" sm="4" md="3">
                <v-select
                  v-model="subJsonExt.log.level"
                  :items="levels"
                  :label="$t('basic.log.level')"
                  density="comfortable"
                  hide-details
                  prepend-inner-icon="mdi-format-list-bulleted-type"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="4" md="3">
                <v-switch
                  v-model="subJsonExt.log.timestamp"
                  color="primary"
                  :label="$t('setting.timestamp')"
                  hide-details
                  density="comfortable"
                  @update:model-value="onFormValueChange"
                />
              </v-col>
            </template>
          </v-row>
        </v-card-text>
      </v-card>

      <!-- Card 2: DNS 解析与 Fake-IP 域名服务 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-green">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-dns-outline</v-icon>
            <span>{{ $t('subscriptionEditor.jsonCardDns') }}</span>
          </div>
          <v-switch
            v-model="enableDns"
            color="primary"
            :label="$t('pages.dns')"
            hide-details
            density="compact"
            @update:model-value="onFormValueChange"
          />
        </v-card-title>
        <v-divider />
        <v-card-text v-if="enableDns" class="pt-4">
          <!-- 核心流量 DNS 配置组（消除小屏 cols="4" cols="5" cols="3" 挤压） -->
          <div class="text-caption font-weight-medium text-medium-emphasis mb-2">
            {{ $t('subscriptionEditor.proxyTrafficDns') }} &amp; {{ $t('subscriptionEditor.directTrafficDns') }}
          </div>
          <v-row>
            <!-- 代理流量 DNS -->
            <v-col cols="12" md="6">
              <v-card variant="outlined" rounded="md" class="pa-3">
                <div class="text-body-2 font-weight-medium mb-2 d-flex align-center ga-1">
                  <v-icon size="x-small" color="info">mdi-earth</v-icon>
                  <span>{{ $t('subscriptionEditor.proxyTrafficDns') }}</span>
                </div>
                <v-row dense>
                  <v-col cols="12" sm="4">
                    <v-select
                      v-model="proxyDnsType"
                      :items="dnsTypeOptions"
                      :label="$t('type')"
                      density="compact"
                      hide-details
                      @update:model-value="onProxyDnsTypeChange($event); onFormValueChange()"
                    />
                  </v-col>
                  <v-col cols="12" sm="5" v-if="proxyDnsShowServer">
                    <v-text-field
                      v-model="proxyDnsServer"
                      :label="$t('in.addr')"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                  <v-col cols="12" sm="3" v-if="proxyDnsShowServer">
                    <v-text-field
                      v-model.number="proxyDnsPort"
                      :label="$t('in.port')"
                      type="number"
                      min="1"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                  <v-col cols="12" v-if="proxyDnsUsesPath">
                    <v-text-field
                      v-model="proxyDnsPath"
                      :label="$t('transport.path')"
                      placeholder="/dns-query"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                </v-row>
              </v-card>
            </v-col>

            <!-- 直连流量 DNS -->
            <v-col cols="12" md="6">
              <v-card variant="outlined" rounded="md" class="pa-3">
                <div class="text-body-2 font-weight-medium mb-2 d-flex align-center ga-1">
                  <v-icon size="x-small" color="success">mdi-navigation-variant-outline</v-icon>
                  <span>{{ $t('subscriptionEditor.directTrafficDns') }}</span>
                </div>
                <v-row dense>
                  <v-col cols="12" sm="4">
                    <v-select
                      v-model="directDnsType"
                      :items="dnsTypeOptions"
                      :label="$t('type')"
                      density="compact"
                      hide-details
                      @update:model-value="onDirectDnsTypeChange($event); onFormValueChange()"
                    />
                  </v-col>
                  <v-col cols="12" sm="5" v-if="directDnsShowServer">
                    <v-text-field
                      v-model="directDnsServer"
                      :label="$t('in.addr')"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                  <v-col cols="12" sm="3" v-if="directDnsShowServer">
                    <v-text-field
                      v-model.number="directDnsPort"
                      :label="$t('in.port')"
                      type="number"
                      min="1"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                  <v-col cols="12" v-if="directDnsUsesPath">
                    <v-text-field
                      v-model="directDnsPath"
                      :label="$t('transport.path')"
                      placeholder="/dns-query"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                </v-row>
              </v-card>
            </v-col>
          </v-row>

          <!-- Bootstrap DNS 配置组 -->
          <div class="text-caption font-weight-medium text-medium-emphasis mt-3 mb-2">
            {{ $t('subscriptionEditor.proxyBootstrapDns') }} &amp; {{ $t('subscriptionEditor.directBootstrapDns') }}
          </div>
          <v-row>
            <!-- 代理 Bootstrap DNS -->
            <v-col cols="12" md="6">
              <v-card variant="outlined" rounded="md" class="pa-3">
                <div class="text-body-2 font-weight-medium mb-2 d-flex align-center ga-1">
                  <v-icon size="x-small" color="info">mdi-rocket-launch-outline</v-icon>
                  <span>{{ $t('subscriptionEditor.proxyBootstrapDns') }}</span>
                </div>
                <v-row dense>
                  <v-col cols="12" sm="4">
                    <v-select
                      v-model="proxyBootstrapDnsType"
                      :items="dnsTypeOptions"
                      :label="$t('type')"
                      density="compact"
                      hide-details
                      @update:model-value="onProxyBootstrapDnsTypeChange($event); onFormValueChange()"
                    />
                  </v-col>
                  <v-col cols="12" sm="5" v-if="proxyBootstrapDnsShowServer">
                    <v-text-field
                      v-model="proxyBootstrapDnsServer"
                      :label="$t('in.addr')"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                  <v-col cols="12" sm="3" v-if="proxyBootstrapDnsShowServer">
                    <v-text-field
                      v-model.number="proxyBootstrapDnsPort"
                      :label="$t('in.port')"
                      type="number"
                      min="1"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                  <v-col cols="12" v-if="proxyBootstrapDnsUsesPath">
                    <v-text-field
                      v-model="proxyBootstrapDnsPath"
                      :label="$t('transport.path')"
                      placeholder="/dns-query"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                </v-row>
              </v-card>
            </v-col>

            <!-- 直连 Bootstrap DNS -->
            <v-col cols="12" md="6">
              <v-card variant="outlined" rounded="md" class="pa-3">
                <div class="text-body-2 font-weight-medium mb-2 d-flex align-center ga-1">
                  <v-icon size="x-small" color="success">mdi-rocket-outline</v-icon>
                  <span>{{ $t('subscriptionEditor.directBootstrapDns') }}</span>
                </div>
                <v-row dense>
                  <v-col cols="12" sm="4">
                    <v-select
                      v-model="directBootstrapDnsType"
                      :items="dnsTypeOptions"
                      :label="$t('type')"
                      density="compact"
                      hide-details
                      @update:model-value="onDirectBootstrapDnsTypeChange($event); onFormValueChange()"
                    />
                  </v-col>
                  <v-col cols="12" sm="5" v-if="directBootstrapDnsShowServer">
                    <v-text-field
                      v-model="directBootstrapDnsServer"
                      :label="$t('in.addr')"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                  <v-col cols="12" sm="3" v-if="directBootstrapDnsShowServer">
                    <v-text-field
                      v-model.number="directBootstrapDnsPort"
                      :label="$t('in.port')"
                      type="number"
                      min="1"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                  <v-col cols="12" v-if="directBootstrapDnsUsesPath">
                    <v-text-field
                      v-model="directBootstrapDnsPath"
                      :label="$t('transport.path')"
                      placeholder="/dns-query"
                      density="compact"
                      hide-details
                      @update:model-value="onFormValueChange"
                    />
                  </v-col>
                </v-row>
              </v-card>
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- final_dns, query_type & Fake-IP -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="subJsonExt.dns.final"
                :items="dnsFinalOptions"
                :label="$t('subscriptionEditor.finalDns')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-export-variant"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-switch
                v-model="enableDnsQueryType"
                color="primary"
                :label="$t('subscriptionEditor.queryType')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-switch
                v-model="enableFakeip"
                color="primary"
                :label="$t('subscriptionEditor.fakeip')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" v-if="enableFakeip">
              <v-combobox
                v-model="tunIp"
                :items="tunIpOptions"
                chips
                multiple
                closable-chips
                clearable
                density="comfortable"
                hide-details
                :label="$t('subscriptionEditor.fakeip')"
                :placeholder="$t('subscriptionEditor.fakeipPlaceholder')"
                prepend-inner-icon="mdi-ip-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- DNS 路由规则列表 (dnsRouteRows) -->
          <div class="mt-4 mb-2 d-flex align-center justify-space-between flex-wrap ga-2">
            <div class="text-subtitle-2 font-weight-medium d-flex align-center ga-1">
              <v-icon size="small" color="primary">mdi-routes</v-icon>
              <span>{{ $t('subscriptionEditor.dnsRouteRules') }}</span>
            </div>
          </div>

          <v-card
            v-for="(dnsRouteRow, dnsRowIdx) in dnsRouteRows"
            :key="dnsRouteRow.id"
            variant="outlined"
            rounded="md"
            class="pa-3 mb-3"
          >
            <v-row dense align="center">
              <v-col cols="12" sm="6" md="5">
                <v-combobox
                  v-if="dnsRouteRow.kind === 'rule-set'"
                  v-model="dnsRouteRow.ruleSet"
                  :items="dnsRouteRuleSetOptions"
                  label="rule_set"
                  multiple
                  chips
                  closable-chips
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
                <v-text-field
                  v-else
                  model-value='"query_type": ["A", "AAAA"]'
                  label="query_type"
                  readonly
                  density="compact"
                  hide-details
                />
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <v-select
                  v-model="dnsRouteRow.server"
                  :items="dnsRouteServerOptions"
                  label="dns"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" md="4" class="d-flex align-center justify-end ga-1 flex-wrap">
                <v-btn
                  icon="mdi-arrow-up"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveUp')"
                  :aria-label="$t('subscriptionEditor.moveUp')"
                  :disabled="dnsRowIdx === 0"
                  @click="onFormValueChange(); moveDnsRouteRow(dnsRowIdx, -1)"
                />
                <v-btn
                  icon="mdi-arrow-down"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveDown')"
                  :aria-label="$t('subscriptionEditor.moveDown')"
                  :disabled="dnsRowIdx >= dnsRouteRows.length - 1"
                  @click="onFormValueChange(); moveDnsRouteRow(dnsRowIdx, 1)"
                />
                <v-btn
                  v-if="dnsRouteRow.kind === 'rule-set'"
                  icon="mdi-plus"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.add')"
                  :aria-label="$t('subscriptionEditor.add')"
                  @click="onFormValueChange(); insertDnsRouteRow(dnsRowIdx)"
                />
                <v-btn
                  v-if="dnsRouteRow.kind === 'rule-set' && canDeleteDnsRouteRow(dnsRowIdx)"
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  :title="$t('subscriptionEditor.remove')"
                  :aria-label="$t('subscriptionEditor.remove')"
                  @click="onFormValueChange(); removeDnsRouteRow(dnsRowIdx)"
                />
              </v-col>
            </v-row>
          </v-card>

          <!-- Resolver Strategy & Default Domain Resolver -->
          <v-row class="mt-2">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="dnsStrategy"
                :items="dnsStrategyOptions"
                :label="$t('subscriptionEditor.dnsStrategy')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-transit-connection-variant"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="routeDefaultDomainResolver"
                :items="dnsFinalOptions"
                label="default_domain_resolve"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-dns-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>
        </v-card-text>
      </v-card>

      <!-- Card 3: 进站路由与 TUN 网络栈 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-router-wireless</v-icon>
            <span>{{ $t('subscriptionEditor.jsonCardTun') }}</span>
          </div>
          <v-switch
            v-model="enableInb"
            color="primary"
            label="Inbound"
            hide-details
            density="compact"
            @update:model-value="onFormValueChange"
          />
        </v-card-title>
        <v-divider />
        <v-card-text v-if="enableInb" class="pt-4">
          <!-- TUN Switches -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableTun"
                color="primary"
                :label="$t('setting.tun')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3" v-if="enableTun">
              <v-switch
                v-model="autoRoute"
                color="primary"
                :label="$t('subscriptionEditor.autoRoute')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3" v-if="enableTun && autoRoute">
              <v-switch
                v-model="strictRoute"
                color="primary"
                :label="$t('subscriptionEditor.strictRoute')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3" v-if="enableTun">
              <v-switch
                v-model="endpointIndependentNat"
                color="primary"
                :label="$t('subscriptionEditor.endpointIndependentNat')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- TUN Address & MTU & Mode -->
          <v-row v-if="enableTun" class="mt-1" align="center">
            <v-col cols="12" sm="6" md="3">
              <v-select
                v-model="tunMode"
                :items="['system', 'mixed', 'gvisor']"
                :label="$t('subscriptionEditor.tunMode')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-layers-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-text-field
                v-model.number="tunMtu"
                type="number"
                min="576"
                max="65535"
                density="comfortable"
                hide-details
                :label="$t('subscriptionEditor.tunMtu')"
                prepend-inner-icon="mdi-speedometer"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" md="6">
              <v-combobox
                v-model="tunAddress"
                :items="defaultTunAddress"
                chips
                multiple
                closable-chips
                density="comfortable"
                hide-details
                :label="$t('subscriptionEditor.tunAddress')"
                prepend-inner-icon="mdi-ip-network-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- Mixed Inbound -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                v-model="mixedListen"
                :label="$t('subscriptionEditor.mixedInboundAddr')"
                placeholder="127.0.0.1"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-map-marker-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                v-model.number="mixedListenPort"
                type="number"
                min="1"
                max="65535"
                :label="$t('subscriptionEditor.mixedInboundPort')"
                placeholder="2080"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-numeric"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- Exclude packages & Platform proxy -->
          <v-row v-if="enableTun" class="mt-1" align="center">
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
              <v-switch
                v-model="platformProxy"
                color="primary"
                :label="$t('subscriptionEditor.platformProxy')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>
        </v-card-text>
      </v-card>

      <!-- Card 4: 规则集与路由分流策略 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-green">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-routes</v-icon>
            <span>{{ $t('subscriptionEditor.jsonCardRules') }}</span>
          </div>
          <v-chip size="x-small" color="info" variant="outlined">
            {{ $t('subscriptionEditor.ruleSet') }}
          </v-chip>
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-4">
          <!-- 全局规则集来源 -->
          <v-row class="mb-3">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="ruleSetSource"
                :items="ruleSetSourceOptions"
                :label="$t('subscriptionEditor.globalRuleSetSource')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-source-branch"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- 规则行统一渲染列表 -->
          <v-card
            v-for="(row, idx) in ruleRows"
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
                  :items="ruleKindOptions"
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
                  :items="domainIpTypes"
                  :label="idx === 0 ? $t('subscriptionEditor.customMatchType') : $t('subscriptionEditor.matchType')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
                <v-select
                  v-else
                  v-model="row.ruleSetScope"
                  :items="ruleSetScopeOptions"
                  :label="$t('subscriptionEditor.ruleSetScope')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" sm="3" md="2" v-if="row.kind === 'ruleset'">
                <v-select
                  v-model="row.ruleSetSourceOverride"
                  :items="getRuleSetSourceOverrideOptions(row.ruleSetScope)"
                  :label="$t('subscriptionEditor.ruleSetSource')"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
                />
              </v-col>
              <v-col cols="12" :sm="row.kind === 'ruleset' ? 6 : 6" :md="row.kind === 'ruleset' ? 4 : 4">
                <v-combobox
                  v-model="row.values"
                  :items="row.kind === 'ruleset' ? (row.ruleSetScope === 'ip' ? geoipNameOptions : geositeNameOptions) : []"
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
                  :items="customRouteOptions"
                  :label="row.name && row.name.trim() ? $t('subscriptionEditor.routeDisabledByName') : $t('subscriptionEditor.route')"
                  :disabled="Boolean(row.name && row.name.trim())"
                  density="compact"
                  hide-details
                  @update:model-value="onFormValueChange"
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
                  @click="onFormValueChange(); moveRuleRow(idx, -1)"
                />
                <v-btn
                  icon="mdi-arrow-down"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.moveDown')"
                  :aria-label="$t('subscriptionEditor.moveDown')"
                  :disabled="idx >= ruleRows.length - 1"
                  @click="onFormValueChange(); moveRuleRow(idx, 1)"
                />
                <v-btn
                  icon="mdi-plus"
                  size="small"
                  variant="text"
                  :title="$t('subscriptionEditor.add')"
                  :aria-label="$t('subscriptionEditor.add')"
                  @click="onFormValueChange(); insertRuleRow(idx)"
                />
                <v-btn
                  v-if="canDeleteRuleRow(idx)"
                  icon="mdi-delete"
                  size="small"
                  variant="text"
                  color="error"
                  :title="$t('subscriptionEditor.remove')"
                  :aria-label="$t('subscriptionEditor.remove')"
                  @click="onFormValueChange(); removeRuleRow(idx)"
                />
              </v-col>
            </v-row>
          </v-card>

          <!-- 最终出口、更新方式与更新周期 -->
          <v-row class="mt-3" align="center">
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="routeFinal"
                :items="routeFinalOptions"
                :label="$t('subscriptionEditor.routeFinal')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-call-split"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-select
                v-model="updateMethod"
                :items="updateMethodOptions"
                :label="$t('subscriptionEditor.updateMethod')"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-download-network-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="4">
              <v-text-field
                v-model="updateInterval"
                :label="$t('subscriptionEditor.updateInterval')"
                placeholder="1d"
                density="comfortable"
                hide-details
                prepend-inner-icon="mdi-history"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>
        </v-card-text>
      </v-card>

      <!-- Card 5: 延迟检测与实验特性 -->
      <v-card rounded="xl" variant="outlined" class="mb-4 card-cyan">
        <v-card-title class="d-flex align-center justify-space-between py-3 px-4 flex-wrap ga-2">
          <div class="text-subtitle-1 font-weight-medium d-flex align-center ga-2">
            <v-icon size="small" color="primary">mdi-flask-outline</v-icon>
            <span>{{ $t('subscriptionEditor.jsonCardExp') }}</span>
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
                :items="latencyTestUrlOptions"
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
                :hint="$t('subscriptionEditor.singboxIntervalHint')"
                persistent-hint
                :error-messages="latencyTestIntervalError ? [latencyTestIntervalError] : []"
                :placeholder="$t('subscriptionEditor.singboxIntervalPlaceholder')"
                density="comfortable"
                prepend-inner-icon="mdi-timer-outline"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-text-field
                v-model="latencyTolerance"
                :label="$t('subscriptionEditor.latencyTolerance')"
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

          <!-- 协议阻断与本地缓存 -->
          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableRejectQuic"
                color="primary"
                :label="$t('subscriptionEditor.rejectQuic')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableReject443Udp"
                color="primary"
                :label="$t('subscriptionEditor.reject443Udp')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableExp"
                color="primary"
                :label="$t('subscriptionEditor.localCache')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableSniff"
                color="primary"
                label="sniff"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <v-row align="center">
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableHijackDns"
                color="primary"
                label="hijack-dns"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
            <v-col cols="12" sm="6" md="3">
              <v-switch
                v-model="enableSubClashApi"
                color="primary"
                :label="$t('subscriptionEditor.clashApi')"
                hide-details
                density="comfortable"
                @update:model-value="onFormValueChange"
              />
            </v-col>
          </v-row>

          <!-- Clash API 进阶参数 -->
          <template v-if="subJsonExt.experimental?.clash_api">
            <v-card variant="outlined" rounded="md" class="pa-3 mt-3">
              <div class="text-body-2 font-weight-medium mb-3 d-flex align-center ga-1">
                <v-icon size="small" color="primary">mdi-api</v-icon>
                <span>{{ $t('subscriptionEditor.clashApi') }}</span>
              </div>
              <v-row>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field
                    v-model="subJsonExt.experimental.clash_api.external_controller"
                    :label="$t('subscriptionEditor.externalController')"
                    placeholder="127.0.0.1:9090"
                    density="compact"
                    hide-details
                    @update:model-value="onFormValueChange"
                  />
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-text-field
                    v-model="subJsonExt.experimental.clash_api.secret"
                    :label="$t('subscriptionEditor.secret')"
                    density="compact"
                    hide-details
                    @update:model-value="onFormValueChange"
                  />
                </v-col>
                <v-col cols="12" sm="6" md="4">
                  <v-select
                    v-model="subJsonExt.experimental.clash_api.default_mode"
                    :items="clashApiModeOptions"
                    :label="$t('subscriptionEditor.defaultMode')"
                    density="compact"
                    hide-details
                    @update:model-value="onFormValueChange"
                  />
                </v-col>
              </v-row>
              <v-row class="mt-1">
                <v-col cols="12" sm="6" md="4">
                  <v-text-field
                    v-model="subJsonExt.experimental.clash_api.external_ui"
                    :label="$t('subscriptionEditor.externalUi')"
                    placeholder="dashboard"
                    density="compact"
                    hide-details
                    @update:model-value="onFormValueChange"
                  />
                </v-col>
                <v-col cols="12" sm="12" md="5">
                  <v-text-field
                    v-model="subJsonExt.experimental.clash_api.external_ui_download_url"
                    :label="$t('subscriptionEditor.externalUiDownloadUrl')"
                    density="compact"
                    hide-details
                    @update:model-value="onFormValueChange"
                  />
                </v-col>
                <v-col cols="12" sm="6" md="3">
                  <v-select
                    v-model="subJsonExt.experimental.clash_api.external_ui_download_detour"
                    :items="subSelectorTagOptions"
                    :label="$t('subscriptionEditor.externalUiDownloadDetour')"
                    density="compact"
                    hide-details
                    clearable
                    @update:model-value="onFormValueChange"
                  />
                </v-col>
              </v-row>
              <v-row class="mt-1" align="center">
                <v-col cols="12" sm="8" md="8">
                  <v-text-field
                    v-model="subClashApiOrigin"
                    :label="$t('subscriptionEditor.accessControlAllowOrigin')"
                    density="compact"
                    hide-details
                    @update:model-value="onFormValueChange"
                  />
                </v-col>
                <v-col cols="12" sm="4" md="4">
                  <v-switch
                    v-model="subJsonExt.experimental.clash_api.access_control_allow_private_network"
                    color="primary"
                    :label="$t('subscriptionEditor.allowPrivateNetwork')"
                    density="compact"
                    hide-details
                    @update:model-value="onFormValueChange"
                  />
                </v-col>
              </v-row>
            </v-card>
          </template>
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
  name: 'SettingsJsonSubManage',
  props: ['settings', 'canonicalDefault', 'initialDirty', 'initialReset', 'initialDirtyBaseline', 'ruleSetSources'],
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
      ruleSetSource: 'karingx_github' as string,
      autoMatchedRuleSetUrls: {} as Record<string, { url: string; source: string }>,
      autoMatchRunToken: 0,
      dnsRouteRows: [
        { id: 'json-dns-initial', kind: 'rule-set', server: 'proxy-dns', ruleSet: [] as string[] },
      ] as Array<{ id: string; kind: string; server: string; ruleSet: string[] }>,
      ruleRows: [
        { id: 'json-rule-initial', kind: 'custom', name: '', customType: 'domain_keyword', ruleSetScope: 'domain', ruleSetSourceOverride: null as string | null, route: 'reject', values: [] as string[] },
      ] as Array<{ id: string; kind: string; name: string; customType: string; ruleSetScope: string; ruleSetSourceOverride: string | null; route: string; values: string[] }>,
      ruleKindOptions: [
        { title: this.$t('subscriptionEditor.customMatch'), value: 'custom' },
        { title: this.$t('subscriptionEditor.ruleSet'), value: 'ruleset' },
      ],
      ruleSetScopeOptions: [
        { title: this.$t('subscriptionEditor.domain'), value: 'domain' },
        { title: 'IP', value: 'ip' },
      ],
      customRouteOptions: [
        { title: this.$t('subscriptionEditor.blockRoute'), value: 'reject' },
        { title: this.$t('subscriptionEditor.directRoute'), value: 'direct' },
        { title: this.$t('subscriptionEditor.proxyRoute'), value: 'proxy' },
      ],
      updateMethod: '节点选择' as string,
      updateInterval: '1d' as string,
      routeFinal: '节点选择' as string,
      routeFinalOptions: selectorOptions,
      clashApiModeOptions,
      subSelectorTagOptions: selectorOptions,
      latencyTestUrl: 'https://cp.cloudflare.com/generate_204' as string,
      latencyTestInterval: '3m' as string,
      latencyTolerance: '50' as string,
      enableSniff: true,
      enableHijackDns: true,
      enableRejectQuic: true,
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
      defaultTunAddress: ['172.19.0.1/30', 'fdfe:dcba:9876::1/126'],

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
      set(v: string) { if (this.proxyDnsObj && this.proxyDnsObj.tag) this.proxyDnsObj.type = v },
    },
    proxyDnsServer: {
      get(): string { return this.proxyDnsObj?.server ?? '' },
      set(v: string) { if (this.proxyDnsObj && this.proxyDnsObj.tag) this.proxyDnsObj.server = v; if (this.proxyDnsObj?.tls) this.proxyDnsObj.tls.server_name = v },
    },
    proxyDnsPort: {
      get(): number { return this.proxyDnsObj?.server_port ?? 53 },
      set(v: number) { if (this.proxyDnsObj && this.proxyDnsObj.tag) this.proxyDnsObj.server_port = v },
    },
    proxyDnsShowServer(): boolean { return !this.noServerTypes.includes(this.proxyDnsType) },
    proxyDnsUsesPath(): boolean { return jsonSubscriptionDNSUsesPath(this.proxyDnsType) },
    proxyDnsPath: {
      get(): string { return typeof this.proxyDnsObj?.path === 'string' ? this.proxyDnsObj.path : '' },
      set(v: string) { if (this.proxyDnsObj && this.proxyDnsObj.tag) this.proxyDnsObj.path = v },
    },
    directDnsType: {
      get(): string { return this.directDnsObj?.type ?? 'https' },
      set(v: string) { if (this.directDnsObj && this.directDnsObj.tag) this.directDnsObj.type = v },
    },
    directDnsServer: {
      get(): string { return this.directDnsObj?.server ?? '' },
      set(v: string) { if (this.directDnsObj && this.directDnsObj.tag) this.directDnsObj.server = v; if (this.directDnsObj?.tls) this.directDnsObj.tls.server_name = v },
    },
    directDnsPort: {
      get(): number { return this.directDnsObj?.server_port ?? 443 },
      set(v: number) { if (this.directDnsObj && this.directDnsObj.tag) this.directDnsObj.server_port = v },
    },
    directDnsShowServer(): boolean { return !this.noServerTypes.includes(this.directDnsType) },
    directDnsUsesPath(): boolean { return jsonSubscriptionDNSUsesPath(this.directDnsType) },
    directDnsPath: {
      get(): string { return typeof this.directDnsObj?.path === 'string' ? this.directDnsObj.path : '' },
      set(v: string) { if (this.directDnsObj && this.directDnsObj.tag) this.directDnsObj.path = v },
    },
    proxyBootstrapDnsType: {
      get(): string { return this.proxyBootstrapDnsObj?.type ?? 'udp' },
      set(v: string) { if (this.proxyBootstrapDnsObj && this.proxyBootstrapDnsObj.tag) this.proxyBootstrapDnsObj.type = v },
    },
    proxyBootstrapDnsServer: {
      get(): string { return this.proxyBootstrapDnsObj?.server ?? '' },
      set(v: string) {
        if (this.proxyBootstrapDnsObj && this.proxyBootstrapDnsObj.tag) this.proxyBootstrapDnsObj.server = v
        if (this.proxyBootstrapDnsObj?.tls) this.proxyBootstrapDnsObj.tls.server_name = v
      },
    },
    proxyBootstrapDnsPort: {
      get(): number { return this.proxyBootstrapDnsObj?.server_port ?? 53 },
      set(v: number) { if (this.proxyBootstrapDnsObj && this.proxyBootstrapDnsObj.tag) this.proxyBootstrapDnsObj.server_port = v },
    },
    proxyBootstrapDnsShowServer(): boolean { return !this.noServerTypes.includes(this.proxyBootstrapDnsType) },
    proxyBootstrapDnsUsesPath(): boolean { return jsonSubscriptionDNSUsesPath(this.proxyBootstrapDnsType) },
    proxyBootstrapDnsPath: {
      get(): string { return typeof this.proxyBootstrapDnsObj?.path === 'string' ? this.proxyBootstrapDnsObj.path : '' },
      set(v: string) { if (this.proxyBootstrapDnsObj && this.proxyBootstrapDnsObj.tag) this.proxyBootstrapDnsObj.path = v },
    },
    directBootstrapDnsType: {
      get(): string { return this.directBootstrapDnsObj?.type ?? 'udp' },
      set(v: string) { if (this.directBootstrapDnsObj && this.directBootstrapDnsObj.tag) this.directBootstrapDnsObj.type = v },
    },
    directBootstrapDnsServer: {
      get(): string { return this.directBootstrapDnsObj?.server ?? '' },
      set(v: string) {
        if (this.directBootstrapDnsObj && this.directBootstrapDnsObj.tag) this.directBootstrapDnsObj.server = v
        if (this.directBootstrapDnsObj?.tls) this.directBootstrapDnsObj.tls.server_name = v
      },
    },
    directBootstrapDnsPort: {
      get(): number { return this.directBootstrapDnsObj?.server_port ?? 53 },
      set(v: number) { if (this.directBootstrapDnsObj && this.directBootstrapDnsObj.tag) this.directBootstrapDnsObj.server_port = v },
    },
    directBootstrapDnsShowServer(): boolean { return !this.noServerTypes.includes(this.directBootstrapDnsType) },
    directBootstrapDnsUsesPath(): boolean { return jsonSubscriptionDNSUsesPath(this.directBootstrapDnsType) },
    directBootstrapDnsPath: {
      get(): string { return typeof this.directBootstrapDnsObj?.path === 'string' ? this.directBootstrapDnsObj.path : '' },
      set(v: string) { if (this.directBootstrapDnsObj && this.directBootstrapDnsObj.tag) this.directBootstrapDnsObj.path = v },
    },
    // TUN inbound bindings.
    tunAddress: {
      get(): string[] { return this.tunInbound?.address ?? [] },
      set(v: string[]) { if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.address = v },
    },
    tunMtu: {
      get(): number { return this.tunInbound?.mtu ?? 1500 },
      set(v: number) { if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.mtu = v },
    },
    tunExcludePackage: {
      get(): string[] { return this.tunInbound?.exclude_package ?? [] },
      set(v: string[]) { if (this.tunInbound && this.tunInbound.type === 'tun') this.tunInbound.exclude_package = v },
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
        delete dns.server
        delete dns.server_port
        delete dns.tls
        delete dns.domain_resolver
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
        delete dns.server
        delete dns.server_port
        delete dns.tls
        delete dns.domain_resolver
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
        delete dns.server
        delete dns.server_port
        delete dns.tls
        delete dns.domain_resolver
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
        delete dns.server
        delete dns.server_port
        delete dns.tls
        delete dns.domain_resolver
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
