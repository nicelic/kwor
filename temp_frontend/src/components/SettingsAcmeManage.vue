<template>
  <section class="acme-page">
    <v-row class="mt-1">
      <v-col cols="12">
        <v-card class="acme-hero" rounded="xl" :loading="loadingOverview && !overview.supported">
          <div class="acme-hero__bg"></div>
          <v-card-text class="acme-hero__content">
            <div class="acme-hero__top">
              <div class="d-flex align-center ga-3">
                <div class="acme-hero__icon">
                  <v-icon size="30">mdi-certificate-outline</v-icon>
                </div>
                <div>
                  <div class="text-overline acme-hero__eyebrow">ACME CERTIFICATE CENTER</div>
                  <div class="text-h5 font-weight-bold">证书管理中心</div>
                  <div class="text-body-2 text-medium-emphasis mt-1">
                    基于 acme.sh 外挂实现申请、续签、部署证书，支持常用 CA 与 DNS API 账号托管。
                  </div>
                </div>
              </div>
              <div class="acme-hero__toolbar">
                <v-btn
                  color="primary"
                  prepend-icon="mdi-file-certificate-outline"
                  :disabled="!overview.supported || !overview.installed"
                  @click="openIssueDialog">
                  申请证书
                </v-btn>
                <v-btn
                  variant="tonal"
                  color="secondary"
                  prepend-icon="mdi-shield-lock-outline"
                  @click="openSelfSignedDialog">
                  自签证书
                </v-btn>
                <v-btn
                  variant="tonal"
                  color="secondary"
                  prepend-icon="mdi-account-circle-outline"
                  @click="acmeAccountDialogVisible = true">
                  ACME 账号
                </v-btn>
                <v-btn
                  variant="tonal"
                  color="secondary"
                  prepend-icon="mdi-cloud-key-outline"
                  @click="dnsAccountDialogVisible = true">
                  DNS 账号
                </v-btn>
              </div>
            </div>

            <div class="acme-hero__chips">
              <v-chip size="small" :color="overview.supported ? 'info' : 'warning'" variant="flat">
                {{ overview.supported ? '当前系统支持 ACME' : '仅 Linux 支持 ACME' }}
              </v-chip>
              <v-chip size="small" :color="overview.installed ? 'success' : 'warning'" variant="flat">
                {{ overview.installed ? 'acme.sh 已安装' : 'acme.sh 未安装' }}
              </v-chip>
              <v-chip size="small" color="primary" variant="flat" class="acme-hero-chip acme-hero-chip--version">
                版本：{{ overview.version || '-' }}
              </v-chip>
              <v-chip size="small" color="secondary" variant="flat" class="acme-hero-chip acme-hero-chip--ca">
                默认 CA：{{ caLabel(overview.preferredCA) }}
              </v-chip>
            </div>

            <v-row class="mt-2">
              <v-col cols="12" sm="6" md="3">
                <div class="acme-metric">
                  <div class="text-caption acme-muted">证书数</div>
                  <div class="text-h5 mt-1">{{ overview.certificates.length }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="acme-metric">
                  <div class="text-caption acme-muted">ACME 账号</div>
                  <div class="text-h5 mt-1">{{ overview.acmeAccounts.length }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="acme-metric">
                  <div class="text-caption acme-muted">DNS 账号</div>
                  <div class="text-h5 mt-1">{{ overview.dnsAccounts.length }}</div>
                </div>
              </v-col>
              <v-col cols="12" sm="6" md="3">
                <div class="acme-metric">
                  <div class="text-caption acme-muted">自动续签窗口</div>
                  <div class="text-h5 mt-1">{{ autoRenewWindowText }}</div>
                  <div class="text-caption text-medium-emphasis mt-1">{{ autoRenewWindowHint }}</div>
                </div>
              </v-col>
            </v-row>
          </v-card-text>
        </v-card>
      </v-col>

      <v-col cols="12">
        <v-card rounded="xl" variant="outlined" class="acme-runtime">
          <v-card-title class="text-subtitle-1 font-weight-medium">acme.sh 运行时</v-card-title>
          <v-divider />
          <v-card-text>
            <v-text-field
              v-model="installEmail"
              label="联系邮箱（可选）"
              placeholder="admin@example.com"
              autocomplete="off"
              autocapitalize="off"
              autocorrect="off"
              spellcheck="false"
              hide-details
              :loading="savingInstallEmail"
              @focus="onInstallEmailFocus"
              @blur="onInstallEmailBlur"
              class="mb-3" />

            <div class="acme-runtime__actions">
              <v-select
                v-model="selectedAcmeVersion"
                :items="acmeVersionSelectItems"
                item-title="title"
                item-value="value"
                label="安装版本"
                density="comfortable"
                hide-details
                class="acme-version-select"
                :loading="loadingAcmeVersions"
                :disabled="installing || upgrading || removingAcme || !overview.supported"
                @update:menu="onAcmeVersionMenuUpdate"
                @update:model-value="onAcmeVersionChanged" />
              <div class="acme-runtime__button-group">
                <v-btn
                  class="acme-runtime-btn acme-runtime-btn--install"
                  color="primary"
                  prepend-icon="mdi-download"
                  :loading="installing"
                  :disabled="installing || upgrading || removingAcme || !overview.supported"
                  @click="installAcme">
                  下载 / 重装
                </v-btn>
                <v-btn
                  class="acme-runtime-btn acme-runtime-btn--check"
                  variant="outlined"
                  color="primary"
                  prepend-icon="mdi-cloud-search"
                  :loading="checkingAcmeUpdate"
                  :disabled="installing || upgrading || removingAcme || checkingAcmeUpdate || !overview.supported"
                  @click="checkAcmeUpdate">
                  检测更新
                </v-btn>
                <v-btn
                  class="acme-runtime-btn acme-runtime-btn--danger"
                  variant="outlined"
                  color="error"
                  prepend-icon="mdi-delete-outline"
                  :loading="removingAcme"
                  :disabled="installing || upgrading || removingAcme || !overview.supported || !overview.installed"
                  @click="removeAcme">
                  删除 acme.sh
                </v-btn>
              </div>
            </div>

            <div class="acme-runtime__rows mt-4">
              <div class="acme-runtime__row">
                <span>当前版本</span>
                <strong>{{ overview.version || '-' }}</strong>
              </div>
              <div class="acme-runtime__row">
                <span>脚本路径</span>
                <strong class="acme-code">{{ overview.scriptPath || '-' }}</strong>
              </div>
              <div class="acme-runtime__row">
                <span>工作目录</span>
                <strong class="acme-code">{{ overview.homeDir || '-' }}</strong>
              </div>
              <div class="acme-runtime__row">
                <span>默认验证方式</span>
                <strong>{{ challengeLabel(overview.defaultChallenge) }}</strong>
              </div>
              <div class="acme-runtime__row">
                <span>更新状态</span>
                <strong>{{ acmeUpdateStatusText }}</strong>
              </div>
            </div>
          </v-card-text>
        </v-card>
      </v-col>
    </v-row>

    <v-alert
      v-if="overview.error"
      type="warning"
      variant="tonal"
      density="comfortable"
      class="mt-4 mb-4">
      {{ overview.error }}
    </v-alert>

    <v-card rounded="xl" variant="outlined" class="acme-table-card">
      <v-card-title class="acme-table-card__toolbar">
        <div>
          <div class="text-subtitle-1 font-weight-medium">证书列表</div>
          <div class="text-caption text-medium-emphasis mt-1">
            默认证书内容会保存在 SQLite，同时可以按需推送到本地目录并应用到面板或订阅。
          </div>
        </div>
        <div class="acme-table-card__toolbar-right">
          <v-text-field
            v-model="searchText"
            label="搜索域名 / 账号 / 备注"
            prepend-inner-icon="mdi-magnify"
            clearable
            density="comfortable"
            hide-details
            class="acme-search" />
          <div class="acme-count-text">（证书数量{{ overview.certificates.length }}）</div>
        </div>
      </v-card-title>
      <v-divider />

      <v-card-text>
        <v-table density="comfortable" class="acme-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>主域名</th>
              <th>申请方式</th>
              <th>CA 平台</th>
              <th>账号</th>
              <th>状态</th>
              <th>到期时间</th>
              <th>自动续签</th>
              <th>备注</th>
              <th class="text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="cert in filteredCertificates" :key="cert.id">
              <td class="acme-id-cell">{{ cert.displayId }}</td>
              <td>
                <div class="font-weight-medium">{{ cert.mainDomain }}</div>
                <div class="text-caption text-medium-emphasis mt-1">
                  其他域名：{{ cert.domains.slice(1).join(', ') || '无' }}
                </div>
              </td>
              <td>
                <div>{{ challengeLabel(cert.challenge) }}</div>
                <div class="text-caption text-medium-emphasis mt-1">请求密钥：{{ certificateAlgorithmLabel(cert.keyLength) }}</div>
                <div class="text-caption text-medium-emphasis mt-1">实际密钥：{{ certificateAlgorithmLabel(cert.issuedKeyAlgorithm) }}</div>
                <div class="text-caption text-medium-emphasis mt-1">实际签名：{{ certificateAlgorithmLabel(cert.issuedSignatureAlgorithm) }}</div>
                <div class="text-caption text-medium-emphasis mt-1">指纹：{{ shortFingerprint(cert.fingerprint) || '-' }}</div>
              </td>
              <td>{{ caLabel(cert.caServer) }}</td>
              <td>
                <div>ACME：{{ cert.acmeAccountName || '默认' }}</div>
                <div class="text-caption text-medium-emphasis mt-1">DNS：{{ cert.dnsAccountName || '-' }}</div>
              </td>
              <td>
                <v-chip size="small" :color="statusColor(cert)" variant="flat">{{ statusText(cert) }}</v-chip>
                <div class="text-caption text-error mt-1" v-if="cert.lastError">{{ cert.lastError }}</div>
              </td>
              <td>
                <div>{{ formatTimestamp(cert.notAfter) }}</div>
                <div class="text-caption text-medium-emphasis mt-1">{{ expireSummary(cert.notAfter) }}</div>
              </td>
              <td>
                <v-chip size="small" :color="cert.autoRenew ? 'success' : 'grey'" variant="tonal">
                  {{ cert.autoRenew ? '开启' : '关闭' }}
                </v-chip>
                <div class="text-caption text-medium-emphasis mt-1" v-if="cert.usageLabel">{{ cert.usageLabel }}</div>
              </td>
              <td>
                <div class="acme-remark">{{ cert.remark || '-' }}</div>
              </td>
              <td class="text-right">
                <v-menu location="bottom end">
                  <template #activator="{ props: menuProps }">
                    <v-btn
                      v-bind="menuProps"
                      variant="text"
                      size="small"
                      icon="mdi-dots-vertical"
                      :loading="rowBusyId === cert.id" />
                  </template>
                  <v-list density="compact" nav>
                    <v-list-item prepend-icon="mdi-eye-outline" title="查看证书" @click="openViewDialog(cert)" />
                    <v-list-item prepend-icon="mdi-refresh" title="续签证书" :disabled="!supportsRenew(cert)" @click="renewCertificate(cert, false)" />
                    <v-list-item prepend-icon="mdi-alert" title="强制续签" :disabled="!isAcmeCertificate(cert)" @click="renewCertificate(cert, true)" />
                    <v-list-item
                      :prepend-icon="cert.autoRenew ? 'mdi-toggle-switch' : 'mdi-toggle-switch-off-outline'"
                      :title="cert.autoRenew ? '关闭自动续签' : '开启自动续签'"
                      :disabled="!supportsAutoRenew(cert)"
                      @click="toggleCertificateAutoRenew(cert)" />
                    <v-list-item prepend-icon="mdi-folder-arrow-up-outline" title="推送到目录" @click="openPushDialog(cert)" />
                    <v-list-item
                      :prepend-icon="cert.inUseByPanel ? 'mdi-monitor-off' : 'mdi-monitor-lock'"
                      :title="cert.inUseByPanel ? '取消应用到面板' : '应用到面板'"
                      :subtitle="cert.inUseByPanel && isUnapplyDisabled(cert, 'panel') ? unapplyDisabledMessage('panel') : undefined"
                      :disabled="cert.inUseByPanel && isUnapplyDisabled(cert, 'panel')"
                      :class="{ 'acme-menu-item--disabled': cert.inUseByPanel && isUnapplyDisabled(cert, 'panel') }"
                      @click="toggleCertificateApply(cert, 'panel')" />
                    <v-list-item
                      :prepend-icon="cert.inUseBySub ? 'mdi-link-variant-off' : 'mdi-link-variant'"
                      :title="cert.inUseBySub ? '取消应用到订阅' : '应用到订阅'"
                      :subtitle="cert.inUseBySub && isUnapplyDisabled(cert, 'sub') ? unapplyDisabledMessage('sub') : undefined"
                      :disabled="cert.inUseBySub && isUnapplyDisabled(cert, 'sub')"
                      :class="{ 'acme-menu-item--disabled': cert.inUseBySub && isUnapplyDisabled(cert, 'sub') }"
                      @click="toggleCertificateApply(cert, 'sub')" />
                    <v-list-item prepend-icon="mdi-text-box-search-outline" title="查看日志" @click="openLogDialog(cert)" />
                    <v-list-item
                      prepend-icon="mdi-delete-outline"
                      title="删除证书"
                      :disabled="cert.deleteBlocked"
                      :class="{ 'acme-menu-item--disabled': cert.deleteBlocked }"
                      @click="deleteCertificate(cert)" />
                  </v-list>
                </v-menu>
              </td>
            </tr>
            <tr v-if="filteredCertificates.length === 0">
              <td colspan="10" class="text-center text-medium-emphasis py-8">暂无证书记录</td>
            </tr>
          </tbody>
        </v-table>
      </v-card-text>
    </v-card>

    <v-dialog v-model="issueDialogVisible" max-width="1080" content-class="acme-issue-dialog">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <div>
            <div class="text-subtitle-1 font-weight-medium">申请证书</div>
            <div class="text-caption text-medium-emphasis mt-1">支持 DNS、HTTP（webroot/standalone）和 ALPN 验证方式；{{ acmePortCheckHint }}</div>
          </div>
          <v-chip size="small" color="info" variant="tonal">将调用 acme.sh 执行签发</v-chip>
        </v-card-title>
        <v-divider />

        <v-card-text class="pt-5">
          <v-row>
            <v-col cols="12" md="6">
              <v-select
                v-model="issueForm.certificateType"
                :items="certificateModeItems"
                label="验证方式"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-if="!isIPCertificateMode"
                v-model="issueForm.mainDomain"
                label="主域名"
                placeholder="example.com"
                hide-details />
              <v-combobox
                v-else
                v-model="issueForm.ipAddresses"
                :items="ipCertificateItems"
                label="IP 地址"
                placeholder="选择或输入公网 IP"
                :loading="loadingIPOptions"
                multiple
                chips
                closable-chips
                clearable
                hide-details
                @update:model-value="normalizeIssueIPSelection">
                <template #append>
                  <v-icon
                    icon="mdi-refresh"
                    :class="{ rotating: loadingIPOptions }"
                    v-tooltip:top="'刷新 IP'"
                    @click="refreshIPCertificateOptions" />
                </template>
              </v-combobox>
            </v-col>
            <v-col cols="12" md="6" v-if="!isIPCertificateMode">
              <v-text-field
                v-model="issueForm.extraDomains"
                label="其他域名"
                placeholder="www.example.com, api.example.com"
                hide-details />
            </v-col>
            <v-col cols="12" md="2" v-if="isIPCertificateMode">
              <v-chip class="mt-2" color="info" variant="tonal">
                {{ issueForm.ipAddresses.length }}/100
              </v-chip>
            </v-col>
            <v-col cols="12" md="4">
              <v-select
                v-model="issueForm.challenge"
                :items="activeChallengeItems"
                :label="isIPCertificateMode ? 'IP 验证端口' : '域名验证方式'"
                hide-details />
            </v-col>
            <v-col cols="12" md="4">
              <v-select
                v-model="issueForm.keyLength"
                :items="keyLengthItems"
                label="密钥算法"
                hide-details />
            </v-col>
            <v-col cols="12" md="4">
              <v-text-field
                :model-value="issueSignatureAlgorithmText"
                label="签名算法"
                readonly
                hide-details />
            </v-col>
            <v-col cols="12" md="4">
              <v-select
                v-model="issueForm.server"
                :items="activeCAServerItems"
                label="CA 平台"
                :disabled="isIPCertificateMode"
                hide-details />
            </v-col>
            <v-col cols="12" md="6" v-if="!isIPCertificateMode">
              <v-select
                v-model="issueForm.acmeAccountId"
                :items="acmeAccountItems"
                item-title="nameText"
                item-value="value"
                label="ACME 账号"
                :no-data-text="acmeAccountNoDataText"
                :messages="acmeAccountSelectMessages"
                hide-details="auto">
                <template #item="{ props: itemProps, item }">
                  <v-list-item v-bind="itemProps">
                    <template #title>
                      <div class="acme-account-option__primary">{{ item.raw.nameText }}</div>
                    </template>
                    <template #subtitle>
                      <div class="acme-account-option__secondary">{{ item.raw.metaText }}</div>
                    </template>
                  </v-list-item>
                </template>
                <template #selection="{ item }">
                  <div class="acme-account-selection">
                    <div class="acme-account-option__primary">{{ item.raw.nameText }}</div>
                    <div class="acme-account-option__secondary">{{ item.raw.metaText }}</div>
                  </div>
                </template>
              </v-select>
            </v-col>
            <v-col cols="12" v-if="issueForm.challenge === 'webroot' && !isIPCertificateMode">
              <v-text-field
                v-model="issueForm.webroot"
                label="Webroot 路径"
                placeholder="/var/www/html"
                hide-details />
            </v-col>
            <v-col cols="12" md="6" v-if="issueForm.challenge === 'dns' && !isIPCertificateMode">
              <v-select
                v-model="issueForm.dnsAccountId"
                :items="dnsAccountItems"
                label="DNS 账号"
                hide-details />
            </v-col>
            <v-col cols="12" md="6" v-if="issueForm.challenge === 'dns' && !isIPCertificateMode">
              <v-select
                v-model="issueForm.dnsProvider"
                :items="dnsProviderItems"
                label="DNS Provider"
                hide-details />
            </v-col>
            <v-col cols="12" v-if="issueForm.challenge === 'dns' && !isIPCertificateMode">
              <v-textarea
                v-model="issueForm.dnsEnv"
                label="额外 DNS 环境变量（可选，KEY=VALUE 一行一个）"
                auto-grow
                rows="2"
                variant="outlined" />
            </v-col>
            <v-col cols="12" v-if="shouldShowPortStatus">
              <div class="text-caption text-medium-emphasis mb-2">
                {{ acmePortCheckHint }}，自动切换按 {{ acmePortFallbackHint }} 执行。
              </div>
              <div class="acme-ip-status">
                <div
                  v-for="item in visiblePortStatusItems"
                  :key="item.challenge"
                  class="acme-ip-status__item">
                  <v-icon
                    :icon="item.available ? 'mdi-check-circle-outline' : 'mdi-alert-circle-outline'"
                    :color="item.available ? 'success' : 'warning'" />
                  <div>
                    <div class="font-weight-medium">
                      {{ ipChallengeTitle(item.challenge) }}
                      <v-chip v-if="item.recommended" size="x-small" color="success" variant="tonal" class="ml-2">推荐</v-chip>
                    </div>
                    <div class="text-caption text-medium-emphasis">{{ item.message }}</div>
                    <div class="text-caption text-medium-emphasis">TCP: {{ item.tcpOccupied ? '占用' : '空闲' }} / UDP: {{ item.udpOccupied ? '占用' : '空闲' }}</div>
                  </div>
                </div>
              </div>
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="issueForm.applyTarget"
                :items="applyTargetItems"
                label="签发后立即应用"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model="issueForm.pushDir"
                label="签发后推送目录（可选）"
                placeholder="/etc/ssl/my-app"
                hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field
                v-model="issueForm.customArgs"
                label="附加参数（可选）"
                placeholder="--debug 2"
                hide-details />
            </v-col>
            <v-col cols="12">
              <v-textarea
                v-model="issueForm.remark"
                label="备注"
                auto-grow
                rows="2"
                variant="outlined" />
            </v-col>
          </v-row>

          <v-alert
            :type="shouldShowPortStatus && !selectedPortChallengeAvailable ? 'warning' : 'info'"
            variant="tonal"
            density="comfortable"
            class="mt-4">
            {{ issuePreviewText }}
          </v-alert>
        </v-card-text>

        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="issueDialogVisible = false">取消</v-btn>
          <v-btn
            color="primary"
            :loading="issuing"
            :disabled="!canSubmitIssue"
            @click="issueCertificate">
            开始签发
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="selfSignedDialogVisible" max-width="980">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <div>
            <div class="text-subtitle-1 font-weight-medium">自签证书</div>
            <div class="text-caption text-medium-emphasis mt-1">调用 sing-box TLS 本地签发能力，证书仅托管到本页库存</div>
          </div>
          <v-chip size="small" color="warning" variant="tonal">本地模拟签发，不进行 CA 外部验证</v-chip>
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-5">
          <v-row>
            <v-col cols="12" md="6">
              <v-select
                v-model="selfSignedForm.authorityId"
                :items="selfSignedAuthorityItems"
                label="签发平台模板"
                :loading="loadingSelfSignedAuthorities"
                :disabled="loadingSelfSignedAuthorities"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-btn
                block
                variant="outlined"
                color="secondary"
                prepend-icon="mdi-pencil-box-outline"
                class="mt-md-1"
                :disabled="loadingSelfSignedAuthorities"
                @click="openSelfSignedAuthorityManager">
                编辑平台
              </v-btn>
              <div class="text-caption text-medium-emphasis mt-2">
                在独立窗口中创建、查看和删除自定义平台模板。
              </div>
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model="selfSignedForm.mainDomain"
                label="主域名"
                placeholder="example.com"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model="selfSignedForm.extraDomains"
                label="其他域名"
                placeholder="www.example.com, api.example.com"
                hide-details />
            </v-col>
            <v-col cols="12" md="4">
              <v-select
                v-model="selfSignedForm.keyAlgorithm"
                :items="selfSignedAlgorithmItems"
                label="密钥算法"
                hide-details />
            </v-col>
            <v-col cols="12" md="4">
              <v-select
                v-model="selfSignedForm.signatureAlgorithm"
                :items="selfSignedAlgorithmItems"
                label="签名算法"
                hide-details />
            </v-col>
            <v-col cols="12" md="4">
              <v-row no-gutters>
                <v-col cols="7">
                  <v-text-field
                    v-model.number="selfSignedForm.durationValue"
                    type="number"
                    min="1"
                    label="有效期"
                    hide-details />
                </v-col>
                <v-col cols="5">
                  <v-select
                    v-model="selfSignedForm.durationUnit"
                    :items="durationUnitItems"
                    hide-details />
                </v-col>
              </v-row>
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="selfSignedForm.applyTarget"
                :items="applyTargetItems"
                label="签发后立即应用"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-model="selfSignedForm.pushDir"
                label="签发后推送目录（可选）"
                placeholder="/etc/ssl/my-app"
                hide-details />
            </v-col>
            <v-col cols="12">
              <v-textarea
                v-model="selfSignedForm.remark"
                label="备注"
                auto-grow
                rows="2"
                variant="outlined" />
            </v-col>
          </v-row>
          <v-alert type="info" variant="tonal" density="comfortable">
            即将签发域名：{{ selfSignedDomainPreview || '请先填写主域名' }}
          </v-alert>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="selfSignedDialogVisible = false">取消</v-btn>
          <v-btn
            color="primary"
            :loading="issuingSelfSigned"
            :disabled="loadingSelfSignedAuthorities || !canSubmitSelfSignedIssue"
            @click="issueSelfSignedCertificate">
            开始签发
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="selfSignedAuthorityManagerVisible" max-width="1120" persistent>
      <v-card rounded="xl" class="self-authority-manager">
        <v-card-title class="d-flex align-center justify-space-between flex-wrap ga-3">
          <div class="d-flex align-center ga-3">
            <v-btn
              variant="text"
              prepend-icon="mdi-arrow-left"
              @click="selfSignedAuthorityManagerVisible = false">
              返回
            </v-btn>
            <div class="text-h6 font-weight-medium">自签平台</div>
          </div>
          <v-btn
            icon="mdi-close"
            variant="text"
            aria-label="关闭平台管理"
            @click="selfSignedAuthorityManagerVisible = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-5">
          <div class="d-flex justify-space-between align-center flex-wrap ga-3 mb-4">
            <v-btn color="primary" prepend-icon="mdi-plus" @click="openSelfSignedAuthorityForm()">
              创建平台
            </v-btn>
            <div class="text-body-2 text-medium-emphasis">
              内置平台仅供参考与查看详情，不能删除。
            </div>
          </div>

          <v-table density="comfortable" class="acme-sub-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>密钥算法</th>
                <th>时间</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in selfSignedAuthorities" :key="item.id">
                <td>{{ item.name }}</td>
                <td>{{ selfSignedAlgorithmLabel(item.keyAlgorithm) }}</td>
                <td>{{ formatTimestamp(item.updatedAt || item.createdAt) }}</td>
                <td class="text-right">
                  <div class="d-flex justify-end ga-2 flex-wrap">
                    <v-btn
                      size="small"
                      variant="text"
                      color="primary"
                      @click="selectSelfSignedAuthority(item)">
                      签发证书
                    </v-btn>
                    <v-btn
                      size="small"
                      variant="text"
                      color="primary"
                      @click="openSelfSignedAuthorityDetail(item)">
                      详情
                    </v-btn>
                    <v-btn
                      size="small"
                      variant="text"
                      color="info"
                      @click="downloadSelfSignedAuthority(item)">
                      下载
                    </v-btn>
                    <v-btn
                      v-if="!item.builtin"
                      size="small"
                      variant="text"
                      color="error"
                      @click="deleteSelfSignedAuthority(item)">
                      删除
                    </v-btn>
                  </div>
                </td>
              </tr>
              <tr v-if="selfSignedAuthorities.length === 0">
                <td colspan="4" class="text-center text-medium-emphasis py-6">还没有自签平台</td>
              </tr>
            </tbody>
          </v-table>
        </v-card-text>
      </v-card>
    </v-dialog>

    <v-dialog v-model="selfSignedAuthorityFormVisible" max-width="780">
      <v-card rounded="xl" :loading="savingSelfSignedAuthority">
        <v-card-title class="d-flex align-center justify-space-between">
          <span class="text-subtitle-1 font-weight-medium">{{ selfSignedAuthorityForm.id > 0 ? '编辑平台' : '创建平台' }}</span>
          <v-btn
            icon="mdi-close"
            variant="text"
            aria-label="关闭平台编辑"
            @click="selfSignedAuthorityFormVisible = false" />
        </v-card-title>
        <v-divider />
        <v-card-text>
          <v-row>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.name" label="机构名称 *" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.subjectCn" label="证书主体名称(CN) *" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.organization" label="公司/组织 *" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.department" label="部门" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field
                v-model="selfSignedAuthorityForm.country"
                label="国家代号 *"
                maxlength="2"
                placeholder="US"
                hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.province" label="省份" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.city" label="城市" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field v-model="selfSignedAuthorityForm.issuerName" label="颁发者" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field v-model="selfSignedAuthorityForm.issuerOrg" label="颁发者组织" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.brand" label="品牌" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.caUrl" label="CA URL" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.ocspUrl" label="OCSP 地址" hide-details />
            </v-col>
            <v-col cols="12">
              <v-text-field v-model="selfSignedAuthorityForm.crlUrl" label="CRL 地址" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field v-model="selfSignedAuthorityForm.keyUsage" label="密钥用途" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field v-model="selfSignedAuthorityForm.extKeyUsage" label="密钥扩展用途" hide-details />
            </v-col>
            <v-col cols="12">
              <v-textarea
                v-model="selfSignedAuthorityForm.notes"
                label="说明"
                rows="2"
                auto-grow
                variant="outlined" />
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="selfSignedAuthorityFormVisible = false">取消</v-btn>
          <v-btn color="primary" :disabled="!canSaveSelfSignedAuthority" @click="saveSelfSignedAuthority">确认</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="selfSignedAuthorityDetailVisible" max-width="980">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <div class="d-flex align-center ga-3">
            <v-btn
              variant="text"
              prepend-icon="mdi-arrow-left"
              @click="selfSignedAuthorityDetailVisible = false">
              返回
            </v-btn>
            <div class="text-h6 font-weight-medium">机构详情</div>
          </div>
          <v-btn
            icon="mdi-close"
            variant="text"
            aria-label="关闭平台详情"
            @click="selfSignedAuthorityDetailVisible = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pt-5">
          <v-tabs v-model="selfSignedAuthorityDetailTab" color="primary">
            <v-tab value="profile">机构详情</v-tab>
            <v-tab value="certificate">证书</v-tab>
            <v-tab value="privateKey">私钥</v-tab>
          </v-tabs>
          <v-window v-model="selfSignedAuthorityDetailTab" class="mt-4">
            <v-window-item value="profile">
              <v-table density="comfortable" class="acme-sub-table">
                <tbody>
                  <tr>
                    <th class="self-authority-detail__label">名称</th>
                    <td>{{ selfSignedAuthorityDetail.name || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">证书主体名称(CN)</th>
                    <td>{{ selfSignedAuthorityDetail.subjectCn || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">颁发者</th>
                    <td>{{ selfSignedAuthorityDetail.issuerName || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">颁发者组织</th>
                    <td>{{ selfSignedAuthorityDetail.issuerOrg || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">颁发组织</th>
                    <td>{{ selfSignedAuthorityDetail.organization || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">部门</th>
                    <td>{{ selfSignedAuthorityDetail.department || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">国家代号</th>
                    <td>{{ selfSignedAuthorityDetail.country || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">省份</th>
                    <td>{{ selfSignedAuthorityDetail.province || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">城市</th>
                    <td>{{ selfSignedAuthorityDetail.city || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">平台</th>
                    <td>{{ selfSignedAuthorityDetail.platformName || selfSignedAuthorityDetail.platformCode || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">品牌</th>
                    <td>{{ selfSignedAuthorityDetail.brand || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">密钥算法</th>
                    <td>{{ selfSignedAlgorithmLabel(selfSignedAuthorityDetail.keyAlgorithm) }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">签名算法</th>
                    <td>{{ selfSignedAuthorityDetail.signAlgo || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">CA URL</th>
                    <td>{{ selfSignedAuthorityDetail.caUrl || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">OCSP 地址</th>
                    <td>{{ selfSignedAuthorityDetail.ocspUrl || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">CRL 地址</th>
                    <td>{{ selfSignedAuthorityDetail.crlUrl || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">密钥用途</th>
                    <td>{{ selfSignedAuthorityDetail.keyUsage || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">密钥扩展用途</th>
                    <td>{{ selfSignedAuthorityDetail.extKeyUsage || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">说明</th>
                    <td>{{ selfSignedAuthorityDetail.notes || '-' }}</td>
                  </tr>
                  <tr>
                    <th class="self-authority-detail__label">更新时间</th>
                    <td>{{ formatTimestamp(selfSignedAuthorityDetail.updatedAt || selfSignedAuthorityDetail.createdAt) }}</td>
                  </tr>
                </tbody>
              </v-table>
            </v-window-item>
            <v-window-item value="certificate">
              <v-textarea
                :model-value="selfSignedAuthorityCertificateText"
                label="证书内容"
                rows="12"
                auto-grow
                readonly
                variant="outlined"
                class="acme-view-text" />
            </v-window-item>
            <v-window-item value="privateKey">
              <v-textarea
                :model-value="selfSignedAuthorityPrivateKeyText"
                label="私钥内容"
                rows="12"
                auto-grow
                readonly
                variant="outlined"
                class="acme-view-text" />
            </v-window-item>
          </v-window>
        </v-card-text>
      </v-card>
    </v-dialog>

    <v-dialog v-model="acmeAccountDialogVisible" max-width="980">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <span class="text-subtitle-1 font-weight-medium">ACME 账号管理</span>
          <v-btn color="primary" prepend-icon="mdi-plus" @click="openAcmeAccountForm()">新增账号</v-btn>
        </v-card-title>
        <v-divider />
        <v-card-text>
          <v-table density="comfortable" class="acme-sub-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>邮箱</th>
                <th>CA 平台</th>
                <th>密钥算法</th>
                <th>备注</th>
                <th>更新时间</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in overview.acmeAccounts" :key="item.id">
                <td>{{ item.name }}</td>
                <td>{{ item.email }}</td>
                <td>{{ caLabel(item.server) }}</td>
                <td>{{ keyLengthLabel(item.keyLength) }}</td>
                <td>{{ item.remark || '-' }}</td>
                <td>{{ formatTimestamp(item.updatedAt) }}</td>
                <td class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <v-btn size="small" variant="text" color="primary" icon="mdi-pencil" @click="openAcmeAccountForm(item)" />
                    <v-btn size="small" variant="text" color="error" icon="mdi-delete" @click="deleteAcmeAccount(item)" />
                  </div>
                </td>
              </tr>
              <tr v-if="overview.acmeAccounts.length === 0">
                <td colspan="7" class="text-center text-medium-emphasis py-6">还没有 ACME 账号</td>
              </tr>
            </tbody>
          </v-table>
        </v-card-text>
      </v-card>
    </v-dialog>

    <v-dialog v-model="acmeAccountFormVisible" max-width="680">
      <v-card rounded="xl" :loading="savingAcmeAccount">
        <v-card-title class="text-subtitle-1 font-weight-medium">{{ acmeAccountForm.id > 0 ? '编辑 ACME 账号' : '新增 ACME 账号' }}</v-card-title>
        <v-divider />
        <v-card-text>
          <v-row>
            <v-col cols="12" md="6">
              <v-text-field v-model="acmeAccountForm.name" label="账号名称" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field v-model="acmeAccountForm.email" label="邮箱" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-select v-model="acmeAccountForm.server" :items="caServerItems" label="CA 平台" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-select v-model="acmeAccountForm.keyLength" :items="keyLengthItems" label="密钥算法" hide-details />
            </v-col>
            <v-col cols="12">
              <v-textarea v-model="acmeAccountForm.remark" label="备注" rows="2" auto-grow variant="outlined" />
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="acmeAccountFormVisible = false">取消</v-btn>
          <v-btn color="primary" :disabled="!canSaveAcmeAccount" @click="saveAcmeAccount">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="dnsAccountDialogVisible" max-width="1080">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <span class="text-subtitle-1 font-weight-medium">DNS 账号管理</span>
          <v-btn color="primary" prepend-icon="mdi-plus" @click="openDNSAccountForm()">新增账号</v-btn>
        </v-card-title>
        <v-divider />
        <v-card-text>
          <v-table density="comfortable" class="acme-sub-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>供应商</th>
                <th>参数摘要</th>
                <th>备注</th>
                <th>更新时间</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in overview.dnsAccounts" :key="item.id">
                <td>{{ item.name }}</td>
                <td>{{ item.providerName }} ({{ item.providerCode }})</td>
                <td>{{ dnsEnvSummary(item.env) }}</td>
                <td>{{ item.remark || '-' }}</td>
                <td>{{ formatTimestamp(item.updatedAt) }}</td>
                <td class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <v-btn size="small" variant="text" color="primary" icon="mdi-pencil" @click="openDNSAccountForm(item)" />
                    <v-btn size="small" variant="text" color="error" icon="mdi-delete" @click="deleteDNSAccount(item)" />
                  </div>
                </td>
              </tr>
              <tr v-if="overview.dnsAccounts.length === 0">
                <td colspan="6" class="text-center text-medium-emphasis py-6">还没有 DNS 账号</td>
              </tr>
            </tbody>
          </v-table>
        </v-card-text>
      </v-card>
    </v-dialog>

    <v-dialog v-model="dnsAccountFormVisible" max-width="820">
      <v-card rounded="xl" :loading="savingDNSAccount">
        <v-card-title class="text-subtitle-1 font-weight-medium">{{ dnsAccountForm.id > 0 ? '编辑 DNS 账号' : '新增 DNS 账号' }}</v-card-title>
        <v-divider />
        <v-card-text>
          <v-row>
            <v-col cols="12" md="6">
              <v-text-field v-model="dnsAccountForm.name" label="账号名称" hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="dnsAccountForm.providerCode"
                :items="dnsProviderItems"
                label="DNS 供应商"
                hide-details />
            </v-col>
          </v-row>

          <v-alert
            v-if="selectedDNSProvider"
            type="info"
            variant="tonal"
            density="comfortable"
            class="mt-3 mb-3">
            {{ selectedDNSProvider.helper }}
          </v-alert>

          <v-row v-if="selectedDNSProvider">
            <v-col cols="12" md="6" v-for="field in selectedDNSProvider.fields" :key="field.key">
              <v-text-field
                :model-value="dnsEnvFieldValue(field.key)"
                @update:modelValue="(value) => setDnsEnvField(field.key, String(value ?? ''))"
                :type="isSecretLikeField(field.key) ? 'password' : 'text'"
                :label="`${field.label} (${field.key})${field.required ? ' *' : ''}`"
                :placeholder="field.placeholder || ''"
                hide-details />
            </v-col>
          </v-row>

          <v-textarea
            v-model="dnsAccountForm.extraEnvText"
            label="额外环境变量（可选，KEY=VALUE 一行一个）"
            rows="3"
            auto-grow
            variant="outlined"
            class="mt-2" />

          <v-textarea
            v-model="dnsAccountForm.remark"
            label="备注"
            rows="2"
            auto-grow
            variant="outlined"
            class="mt-2" />
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="dnsAccountFormVisible = false">取消</v-btn>
          <v-btn color="primary" :disabled="!canSaveDNSAccount" @click="saveDNSAccount">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="pushDialogVisible" max-width="680">
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">推送证书到本地目录</v-card-title>
        <v-divider />
        <v-card-text>
          <div class="text-body-2 text-medium-emphasis mb-3">
            将写入 <code>cert.pem</code>、<code>key.pem</code>、<code>fullchain.pem</code>（以及可用时的 <code>chain.pem</code>）。
          </div>
          <v-text-field
            v-model="pushDialogTargetDir"
            label="目标目录"
            placeholder="/etc/ssl/my-app"
            hide-details />
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="closePushDialog">取消</v-btn>
          <v-btn
            color="primary"
            :loading="pushingId === pushDialogCertId"
            :disabled="pushDialogCertId === 0 || pushDialogTargetDir.trim().length === 0"
            @click="confirmPushDialog">
            推送
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="logDialogVisible" max-width="960">
      <v-card rounded="xl">
        <v-card-title class="text-subtitle-1 font-weight-medium">签发 / 续签日志</v-card-title>
        <v-divider />
        <v-card-text>
          <div class="text-body-2 text-medium-emphasis mb-2" v-if="selectedLogCertificate">
            域名：{{ selectedLogCertificate.mainDomain }}
          </div>
          <pre class="acme-log">{{ selectedLogContent }}</pre>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="logDialogVisible = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="viewDialogVisible" max-width="1280">
      <v-card rounded="xl" :loading="viewLoading">
        <v-card-title class="text-subtitle-1 font-weight-medium">查看证书</v-card-title>
        <v-divider />
        <v-card-text>
          <div class="text-body-2 text-medium-emphasis mb-2">
            域名：{{ viewContent.mainDomain || '-' }}
          </div>
          <div class="text-body-2 text-medium-emphasis mb-4">
            来源：{{ viewContent.sourceType || '-' }} / {{ viewContent.sourceRef || '-' }}
          </div>
          <div class="text-body-2 text-medium-emphasis mb-2">
            请求密钥算法：{{ certificateAlgorithmLabel(selectedViewCertificate?.keyLength || '') }}
          </div>
          <div class="text-body-2 text-medium-emphasis mb-2">
            实际密钥算法：{{ certificateAlgorithmLabel(viewContent.issuedKeyAlgorithm) }}
          </div>
          <div class="text-body-2 text-medium-emphasis mb-4">
            实际签名算法：{{ certificateAlgorithmLabel(viewContent.issuedSignatureAlgorithm) }}
          </div>
          <v-row>
            <v-col cols="12">
              <v-textarea
                :model-value="viewContent.fullchainPem"
                label="公钥（fullchain.pem）"
                rows="10"
                auto-grow
                readonly
                variant="outlined"
                class="acme-view-text" />
            </v-col>
            <v-col cols="12">
              <v-textarea
                :model-value="viewContent.keyPem"
                label="私钥（key.pem）"
                rows="10"
                auto-grow
                readonly
                variant="outlined"
                class="acme-view-text" />
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="viewDialogVisible = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <div v-if="issueLogVisible" class="acme-floating-log" :style="issueLogStyle">
      <div class="acme-floating-log__header">
        <div>
          <div class="text-subtitle-2 font-weight-medium">签发日志</div>
          <div class="text-caption text-medium-emphasis">{{ issueLogStatusText }}</div>
        </div>
        <v-btn
          variant="text"
          density="comfortable"
          icon="mdi-close"
          aria-label="关闭签发日志"
          @click="closeIssueLog" />
      </div>
      <v-divider />
      <div ref="issueLogBodyRef" class="acme-floating-log__body">
        <div v-for="(line, index) in issueLogLines" :key="index" class="acme-floating-log__line">
          {{ line }}
        </div>
      </div>
    </div>
  </section>
</template>

<script lang="ts" setup>
import HttpUtils from '@/plugins/httputil'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { push } from 'notivue'

type AcmeCertificate = {
  id: number
  displayId: number
  sourceType: string
  sourceRef: string
  mainDomain: string
  domains: string[]
  challenge: string
  keyLength: string
  issuedKeyAlgorithm: string
  issuedSignatureAlgorithm: string
  caServer: string
  useEcc: boolean
  autoRenew: boolean
  acmeAccountId: number
  acmeAccountName: string
  dnsAccountId: number
  dnsAccountName: string
  applyTarget: string
  pushDir: string
  remark: string
  acmeHome: string
  certPath: string
  keyPath: string
  fullchainPath: string
  chainPath: string
  fingerprint: string
  notBefore: number
  notAfter: number
  lastIssuedAt: number
  lastRenewedAt: number
  updatedAt: number
  createdAt: number
  lastError: string
  lastOutput: string
  status: string
  inUseByPanel: boolean
  inUseBySub: boolean
  usageLabel: string
  deleteBlocked: boolean
}

type AcmeCertificateMaterial = {
  id: number
  mainDomain: string
  sourceType: string
  sourceRef: string
  fullchainPem: string
  keyPem: string
  issuedKeyAlgorithm: string
  issuedSignatureAlgorithm: string
}

type AcmeCAOption = {
  name: string
  value: string
}

type AcmeDNSFieldDef = {
  key: string
  label: string
  required: boolean
  placeholder?: string
}

type AcmeDNSProviderMeta = {
  name: string
  providerCode: string
  helper: string
  fields: AcmeDNSFieldDef[]
}

type AcmeAccount = {
  id: number
  name: string
  email: string
  server: string
  keyLength: string
  remark: string
  createdAt: number
  updatedAt: number
}

type AcmeDNSAccount = {
  id: number
  name: string
  providerName: string
  providerCode: string
  env: Record<string, string>
  remark: string
  createdAt: number
  updatedAt: number
}

type SelfSignedAuthority = {
  id: number
  name: string
  platformCode: string
  platformName: string
  subjectCn: string
  organization: string
  department: string
  country: string
  province: string
  city: string
  keyAlgorithm: string
  issuerName: string
  issuerOrg: string
  caUrl: string
  ocspUrl: string
  crlUrl: string
  keyUsage: string
  extKeyUsage: string
  signAlgo: string
  brand: string
  notes: string
  builtin: boolean
  notBefore: number
  notAfter: number
  createdAt: number
  updatedAt: number
}

type SelfSignedAuthorityForm = {
  id: number
  name: string
  platformCode: string
  platformName: string
  subjectCn: string
  organization: string
  department: string
  country: string
  province: string
  city: string
  issuerName: string
  issuerOrg: string
  caUrl: string
  ocspUrl: string
  crlUrl: string
  keyUsage: string
  extKeyUsage: string
  brand: string
  notes: string
}

type AcmeOverview = {
  supported: boolean
  installed: boolean
  version: string
  scriptPath: string
  homeDir: string
  contactEmail: string
  preferredCA: string
  defaultChallenge: string
  defaultWebroot: string
  defaultDnsProvider: string
  defaultKeyLength: string
  autoRenewWindow?: {
    windowDays: number
    dynamicByValidity: boolean
    thresholdDays: number
    minDynamicWindowDay: number
    examples: number[]
  }
  autoUpgrade: boolean
  caOptions: AcmeCAOption[]
  dnsProviders: AcmeDNSProviderMeta[]
  acmeAccounts: AcmeAccount[]
  dnsAccounts: AcmeDNSAccount[]
  certificates: AcmeCertificate[]
  error?: string
}

type AcmeActionResult = {
  overview?: AcmeOverview
  certificate?: AcmeCertificate
  msg?: string
  output?: string
}

type AcmeVersionItem = {
  tag_name: string
  name: string
  published_at: string
  source?: string
}

type AcmeVersionListResult = {
  versions: AcmeVersionItem[]
  page: number
  per_page: number
  has_more: boolean
}

type AcmeVersionCheckResult = {
  supported: boolean
  installed: boolean
  currentVersion: string
  latestVersion: string
  hasUpdate: boolean
  message: string
}

type AcmeLogSession = {
  id: string
  title: string
  status: string
  lines: string[]
  error?: string
  startedAt: number
  updatedAt: number
  finishedAt?: number
}

type AcmeIPPortItem = {
  challenge: string
  port: number
  occupied: boolean
  available: boolean
  tcpOccupied: boolean
  udpOccupied: boolean
  recommended: boolean
  reason: string
  message: string
}

type AcmeIPPortStatus = {
  supported: boolean
  checkedAt: number
  ports: AcmeIPPortItem[]
}

const props = withDefaults(defineProps<{ active?: boolean }>(), {
  active: false,
})

const loadingOverview = ref(false)
const installing = ref(false)
const upgrading = ref(false)
const removingAcme = ref(false)
const checkingAcmeUpdate = ref(false)
const loadingAcmeVersions = ref(false)
const issuing = ref(false)
const issuingSelfSigned = ref(false)
const loadingSelfSignedAuthorities = ref(false)
const savingAcmeAccount = ref(false)
const savingDNSAccount = ref(false)
const savingSelfSignedAuthority = ref(false)
const loadingIPOptions = ref(false)
const loadingIPPortStatus = ref(false)
const rowBusyId = ref(0)
const pushingId = ref(0)

const installEmail = ref('')
const installEmailEditing = ref(false)
const installEmailHydrated = ref(false)
const installEmailLastSaved = ref('')
const savingInstallEmail = ref(false)
const searchText = ref('')
const selectedAcmeVersion = ref('')
const acmeVersionItems = ref<AcmeVersionItem[]>([])
const acmeVersionPage = ref(1)
const acmeVersionPerPage = ref(5)
const acmeVersionHasMore = ref(false)
const acmeVersionLoaded = ref(false)
const loadingMoreAcmeVersions = ref(false)
const acmeUpdateInfo = ref<AcmeVersionCheckResult>({
  supported: false,
  installed: false,
  currentVersion: '',
  latestVersion: '',
  hasUpdate: false,
  message: '',
})

const createIdleAcmeUpdateInfo = (): AcmeVersionCheckResult => ({
  supported: overview.value.supported,
  installed: overview.value.installed,
  currentVersion: overview.value.version,
  latestVersion: '',
  hasUpdate: false,
  message: '',
})

const issueDialogVisible = ref(false)
const selfSignedDialogVisible = ref(false)
const selfSignedAuthorityManagerVisible = ref(false)
const selfSignedAuthorityFormVisible = ref(false)
const selfSignedAuthorityDetailVisible = ref(false)
const acmeAccountDialogVisible = ref(false)
const acmeAccountFormVisible = ref(false)
const dnsAccountDialogVisible = ref(false)
const dnsAccountFormVisible = ref(false)
const pushDialogVisible = ref(false)
const logDialogVisible = ref(false)
const viewDialogVisible = ref(false)
const viewingCertId = ref(0)
const viewLoading = ref(false)
const selfSignedAuthorityDetailTab = ref('profile')
const viewContent = ref<AcmeCertificateMaterial>({
  id: 0,
  mainDomain: '',
  sourceType: '',
  sourceRef: '',
  fullchainPem: '',
  keyPem: '',
  issuedKeyAlgorithm: '',
  issuedSignatureAlgorithm: '',
})

const pushDialogCertId = ref(0)
const pushDialogTargetDir = ref('')
const logCertId = ref(0)

const pollTimer = ref<number | null>(null)
const issueLogTimer = ref<number | null>(null)
const issueLogCloseTimer = ref<number | null>(null)
const issueLogVisible = ref(false)
const issueLogSessionId = ref('')
const issueLogStatus = ref('idle')
const issueLogLines = ref<string[]>([])
const issueLogBodyRef = ref<HTMLElement | null>(null)
const issueLogZIndex = ref(2600)
let overviewRequestPromise: Promise<void> | null = null
let selfSignedAuthoritiesRequestPromise: Promise<void> | null = null
let overviewLoaded = false
let selfSignedAuthoritiesLoaded = false
let selfSignedAuthoritiesDirty = true
const ipCertificateOptions = ref<string[]>([])
const ipPortStatus = ref<AcmeIPPortStatus>({
  supported: false,
  checkedAt: 0,
  ports: [
    { challenge: 'standalone', port: 80, occupied: false, available: true, tcpOccupied: false, udpOccupied: false, recommended: true, reason: '', message: '80 port not checked yet' },
    { challenge: 'webroot', port: 80, occupied: false, available: true, tcpOccupied: false, udpOccupied: false, recommended: true, reason: '', message: '80 port not checked yet' },
    { challenge: 'alpn', port: 443, occupied: false, available: true, tcpOccupied: false, udpOccupied: false, recommended: false, reason: '', message: '443 port not checked yet' },
  ],
})

const createEmptyOverview = (): AcmeOverview => ({
  supported: false,
  installed: false,
  version: '',
  scriptPath: '',
  homeDir: '',
  contactEmail: '',
  preferredCA: 'letsencrypt',
  defaultChallenge: 'standalone',
  defaultWebroot: '',
  defaultDnsProvider: '',
  defaultKeyLength: 'ec-256',
  autoRenewWindow: {
    windowDays: 30,
    dynamicByValidity: true,
    thresholdDays: 40,
    minDynamicWindowDay: 1,
    examples: [30, 14, 2],
  },
  autoUpgrade: true,
  caOptions: [],
  dnsProviders: [],
  acmeAccounts: [],
  dnsAccounts: [],
  certificates: [],
  error: '',
})

const overview = ref<AcmeOverview>(createEmptyOverview())
const selfSignedAuthorities = ref<SelfSignedAuthority[]>([])
const createEmptySelfSignedAuthorityForm = (): SelfSignedAuthorityForm => ({
  id: 0,
  name: '',
  platformCode: '',
  platformName: '',
  subjectCn: '',
  organization: '',
  department: '',
  country: 'US',
  province: '',
  city: '',
  issuerName: '',
  issuerOrg: '',
  caUrl: '',
  ocspUrl: '',
  crlUrl: '',
  keyUsage: 'Digital Signature',
  extKeyUsage: 'Server Auth',
  brand: '',
  notes: '',
})
const selfSignedAuthorityForm = ref<SelfSignedAuthorityForm>(createEmptySelfSignedAuthorityForm())
const selfSignedAuthorityDetail = ref<SelfSignedAuthority>({
  id: 0,
  name: '',
  platformCode: '',
  platformName: '',
  subjectCn: '',
  organization: '',
  department: '',
  country: '',
  province: '',
  city: '',
  issuerName: '',
  issuerOrg: '',
  caUrl: '',
  ocspUrl: '',
  crlUrl: '',
  keyAlgorithm: '',
  keyUsage: '',
  extKeyUsage: '',
  signAlgo: '',
  brand: '',
  notes: '',
  builtin: false,
  notBefore: 0,
  notAfter: 0,
  createdAt: 0,
  updatedAt: 0,
})

const issueForm = ref({
  certificateType: 'domain',
  mainDomain: '',
  extraDomains: '',
  ipAddresses: [] as string[],
  challenge: 'dns',
  webroot: '',
  dnsProvider: '',
  dnsAccountId: 0,
  dnsEnv: '',
  server: 'letsencrypt',
  keyLength: 'ec-256',
  customArgs: '',
  acmeAccountId: 0,
  remark: '',
  applyTarget: '',
  pushDir: '',
})

const selfSignedForm = ref({
  authorityId: 0,
  mainDomain: '',
  extraDomains: '',
  keyAlgorithm: 'ecc256',
  signatureAlgorithm: 'ecc256',
  durationValue: 90,
  durationUnit: 'd',
  remark: '',
  applyTarget: '',
  pushDir: '',
})

const acmeAccountForm = ref({
  id: 0,
  name: '',
  email: '',
  server: 'letsencrypt',
  keyLength: 'ec-256',
  remark: '',
})

const dnsAccountForm = ref({
  id: 0,
  name: '',
  providerCode: '',
  env: {} as Record<string, string>,
  extraEnvText: '',
  remark: '',
})

const challengeItems = [
  { title: 'DNS 验证（推荐）', value: 'dns' },
  { title: 'HTTP Standalone（80 优先）', value: 'standalone' },
  { title: 'HTTP Webroot（80 侧）', value: 'webroot' },
  { title: 'TLS ALPN（443 兜底）', value: 'alpn' },
]

const certificateModeItems = [
  { title: '域名证书', value: 'domain' },
  { title: 'IP 证书', value: 'ip' },
]

const ipCertificateChallengeItems = [
  { title: 'HTTP Standalone（80 优先）', value: 'standalone' },
  { title: 'TLS ALPN（443 兜底）', value: 'alpn' },
]

const keyLengthItems = [
  { title: 'EC-256', value: 'ec-256' },
  { title: 'EC-384', value: 'ec-384' },
  { title: 'RSA-2048', value: '2048' },
  { title: 'RSA-4096', value: '4096' },
  { title: 'RSA-8192', value: '8192' },
]

const selfSignedAlgorithmItems = [
  { title: 'ECC-256', value: 'ecc256' },
  { title: 'ECC-384', value: 'ecc384' },
  { title: 'ECC-521', value: 'ecc521' },
  { title: 'RSA-2048', value: 'rsa2048' },
  { title: 'RSA-3072', value: 'rsa3072' },
  { title: 'RSA-4096', value: 'rsa4096' },
]

const durationUnitItems = [
  { title: '天', value: 'd' },
  { title: '月', value: 'm' },
  { title: '年', value: 'y' },
]

const applyTargetItems = [
  { title: '不立即应用', value: '' },
  { title: '应用到面板 HTTPS', value: 'panel' },
  { title: '应用到订阅 HTTPS', value: 'sub' },
]

const asString = (value: unknown, fallback = ''): string => {
  return typeof value === 'string' ? value : fallback
}

const asNumber = (value: unknown, fallback = 0): number => {
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number.parseInt(value, 10)
    if (Number.isFinite(parsed)) return parsed
  }
  return fallback
}

const asBoolean = (value: unknown, fallback = false): boolean => {
  if (typeof value === 'boolean') return value
  return fallback
}

const normalizeDomainCAValue = (value: string, fallback = 'letsencrypt'): string => {
  const normalized = value.trim().toLowerCase().replace(/\/+$/g, '')
  if (normalized === '') return fallback
  if (
    normalized === 'let'
    || normalized === 'le'
    || normalized === 'letsencrypt'
    || normalized === 'https://acme-v02.api.letsencrypt.org/directory'
    || normalized === 'https://acme-staging-v02.api.letsencrypt.org/directory'
  ) {
    return 'letsencrypt'
  }
  if (
    normalized === 'zero'
    || normalized === 'zerossl'
    || normalized === 'https://acme.zerossl.com/v2/dv90'
  ) {
    return 'zerossl'
  }
  return fallback
}

const normalizeCAOptions = (value: unknown): AcmeCAOption[] => {
  if (!Array.isArray(value)) return []
  return value.map((raw) => {
    const item = raw as Partial<AcmeCAOption>
    return {
      name: asString(item.name),
      value: asString(item.value),
    }
  }).filter(item => item.name !== '' && item.value !== '')
}

const normalizeDNSProviders = (value: unknown): AcmeDNSProviderMeta[] => {
  if (!Array.isArray(value)) return []
  return value.map((raw) => {
    const item = raw as Partial<AcmeDNSProviderMeta>
    const fieldsRaw = Array.isArray(item.fields) ? item.fields : []
    const fields = fieldsRaw.map((entry) => {
      const field = entry as Partial<AcmeDNSFieldDef>
      return {
        key: asString(field.key),
        label: asString(field.label),
        required: asBoolean(field.required),
        placeholder: asString(field.placeholder),
      }
    }).filter(field => field.key !== '')

    return {
      name: asString(item.name),
      providerCode: asString(item.providerCode),
      helper: asString(item.helper),
      fields,
    }
  }).filter(item => item.providerCode !== '')
}

const normalizeAcmeAccounts = (value: unknown): AcmeAccount[] => {
  if (!Array.isArray(value)) return []
  return value.map((raw) => {
    const item = raw as Partial<AcmeAccount>
    return {
      id: asNumber(item.id),
      name: asString(item.name),
      email: asString(item.email),
      server: asString(item.server),
      keyLength: asString(item.keyLength),
      remark: asString(item.remark),
      createdAt: asNumber(item.createdAt),
      updatedAt: asNumber(item.updatedAt),
    }
  })
}

const normalizeDNSAccounts = (value: unknown): AcmeDNSAccount[] => {
  if (!Array.isArray(value)) return []
  return value.map((raw) => {
    const item = raw as Partial<AcmeDNSAccount>
    const env: Record<string, string> = {}
    const envRaw = item.env
    if (envRaw && typeof envRaw === 'object') {
      Object.entries(envRaw).forEach(([key, val]) => {
        const normKey = key.trim()
        const normVal = String(val ?? '').trim()
        if (normKey && normVal) {
          env[normKey] = normVal
        }
      })
    }
    return {
      id: asNumber(item.id),
      name: asString(item.name),
      providerName: asString(item.providerName),
      providerCode: asString(item.providerCode),
      env,
      remark: asString(item.remark),
      createdAt: asNumber(item.createdAt),
      updatedAt: asNumber(item.updatedAt),
    }
  })
}

const normalizeSelfSignedAuthorities = (value: unknown): SelfSignedAuthority[] => {
  if (!Array.isArray(value)) return []
  return value.map((raw) => {
    const item = raw as Partial<SelfSignedAuthority>
    return {
      id: asNumber(item.id),
      name: asString(item.name),
      platformCode: asString(item.platformCode),
      platformName: asString(item.platformName),
      subjectCn: asString(item.subjectCn),
      organization: asString(item.organization),
      department: asString(item.department),
      country: asString(item.country),
      province: asString(item.province),
      city: asString(item.city),
      keyAlgorithm: asString(item.keyAlgorithm),
      issuerName: asString(item.issuerName),
      issuerOrg: asString(item.issuerOrg),
      caUrl: asString(item.caUrl),
      ocspUrl: asString(item.ocspUrl),
      crlUrl: asString(item.crlUrl),
      keyUsage: asString(item.keyUsage),
      extKeyUsage: asString(item.extKeyUsage),
      signAlgo: asString(item.signAlgo),
      brand: asString(item.brand),
      notes: asString(item.notes),
      builtin: asBoolean(item.builtin),
      notBefore: asNumber(item.notBefore),
      notAfter: asNumber(item.notAfter),
      createdAt: asNumber(item.createdAt),
      updatedAt: asNumber(item.updatedAt),
    }
  }).filter(item => item.id > 0)
}

const normalizeCertificates = (value: unknown): AcmeCertificate[] => {
  if (!Array.isArray(value)) return []
  return value.map((raw) => {
    const item = raw as Partial<AcmeCertificate>
    const domains = Array.isArray(item.domains)
      ? item.domains.map(value => String(value ?? '').trim()).filter(value => value !== '')
      : []
    return {
      id: asNumber(item.id),
      displayId: asNumber((item as any).displayId),
      sourceType: asString((item as any).sourceType),
      sourceRef: asString((item as any).sourceRef),
      mainDomain: asString(item.mainDomain),
      domains,
      challenge: asString(item.challenge),
      keyLength: asString(item.keyLength),
      issuedKeyAlgorithm: asString((item as any).issuedKeyAlgorithm),
      issuedSignatureAlgorithm: asString((item as any).issuedSignatureAlgorithm),
      caServer: asString(item.caServer),
      useEcc: asBoolean(item.useEcc),
      autoRenew: asBoolean(item.autoRenew, false),
      acmeAccountId: asNumber(item.acmeAccountId),
      acmeAccountName: asString(item.acmeAccountName),
      dnsAccountId: asNumber(item.dnsAccountId),
      dnsAccountName: asString(item.dnsAccountName),
      applyTarget: asString(item.applyTarget),
      pushDir: asString(item.pushDir),
      remark: asString(item.remark),
      acmeHome: asString(item.acmeHome),
      certPath: asString(item.certPath),
      keyPath: asString(item.keyPath),
      fullchainPath: asString(item.fullchainPath),
      chainPath: asString(item.chainPath),
      fingerprint: asString(item.fingerprint),
      notBefore: asNumber(item.notBefore),
      notAfter: asNumber(item.notAfter),
      lastIssuedAt: asNumber(item.lastIssuedAt),
      lastRenewedAt: asNumber(item.lastRenewedAt),
      updatedAt: asNumber(item.updatedAt),
      createdAt: asNumber(item.createdAt),
      lastError: asString(item.lastError),
      lastOutput: asString(item.lastOutput),
      status: asString(item.status),
      inUseByPanel: asBoolean((item as any).inUseByPanel),
      inUseBySub: asBoolean((item as any).inUseBySub),
      usageLabel: asString((item as any).usageLabel),
      deleteBlocked: asBoolean((item as any).deleteBlocked),
    }
  })
}

const normalizeInlineEmail = (value: string): string => {
  return String(value ?? '')
    .replace(/\u00a0/g, ' ')
    .replace(/[\u200b\u200c\u200d\ufeff]/g, '')
    .replace(/[＜]/g, '<')
    .replace(/[＞]/g, '>')
    .replace(/[＠﹫]/g, '@')
    .replace(/[。．｡]/g, '.')
    .replace(/\s+/g, '')
}

const isAsciiEmail = (value: string): boolean => /^[\x21-\x7e]+$/.test(value)

const isLikelyValidAcmeEmail = (value: string): boolean => {
  const normalized = normalizeInlineEmail(value)
  if (normalized === '' || !isAsciiEmail(normalized)) return false
  const parts = normalized.split('@')
  if (parts.length !== 2) return false
  const [local, domain] = parts
  if (!local || !domain) return false
  if (domain.startsWith('.') || domain.endsWith('.') || domain.includes('..')) return false
  return true
}

const syncInstallEmailFromOverview = (value: string) => {
  const normalized = normalizeInlineEmail(value)
  if (
    !installEmailHydrated.value
    || (!installEmailEditing.value && normalizeInlineEmail(installEmail.value) === installEmailLastSaved.value)
  ) {
    installEmail.value = normalized
    installEmailLastSaved.value = normalized
    installEmailHydrated.value = true
  }
}

const onInstallEmailFocus = () => {
  installEmailEditing.value = true
}

const saveInstallEmail = async () => {
  const normalized = normalizeInlineEmail(installEmail.value)
  if (normalized !== installEmail.value) {
    installEmail.value = normalized
  }
  if (!installEmailHydrated.value) {
    installEmailLastSaved.value = normalizeInlineEmail(overview.value.contactEmail)
    installEmailHydrated.value = true
  }
  if (normalized === installEmailLastSaved.value || savingInstallEmail.value) return

  savingInstallEmail.value = true
  try {
    const msg = await HttpUtils.post('api/acme-contact-email-save', {
      email: normalized,
    })
    if (msg.success) {
      installEmailLastSaved.value = normalized
      applyActionResult(msg.obj)
    }
  } finally {
    savingInstallEmail.value = false
  }
}

const onInstallEmailBlur = async () => {
  installEmailEditing.value = false
  await saveInstallEmail()
}

const applyOverview = (raw: unknown) => {
  const data = (raw ?? {}) as Partial<AcmeOverview>
  const nextValue: AcmeOverview = {
    ...createEmptyOverview(),
    supported: asBoolean(data.supported),
    installed: asBoolean(data.installed),
    version: asString(data.version),
    scriptPath: asString(data.scriptPath),
    homeDir: asString(data.homeDir),
    contactEmail: normalizeInlineEmail(asString(data.contactEmail)),
    preferredCA: normalizeDomainCAValue(asString(data.preferredCA), 'letsencrypt'),
    defaultChallenge: asString(data.defaultChallenge, 'standalone'),
    defaultWebroot: asString(data.defaultWebroot),
    defaultDnsProvider: asString(data.defaultDnsProvider),
    defaultKeyLength: asString(data.defaultKeyLength, 'ec-256'),
    autoRenewWindow: {
      windowDays: asNumber((data as any).autoRenewWindow?.windowDays, 30),
      dynamicByValidity: asBoolean((data as any).autoRenewWindow?.dynamicByValidity, true),
      thresholdDays: asNumber((data as any).autoRenewWindow?.thresholdDays, 40),
      minDynamicWindowDay: asNumber((data as any).autoRenewWindow?.minDynamicWindowDay, 1),
      examples: Array.isArray((data as any).autoRenewWindow?.examples)
        ? ((data as any).autoRenewWindow?.examples as unknown[]).map(v => asNumber(v)).filter(v => v > 0)
        : [30, 14, 2],
    },
    autoUpgrade: asBoolean(data.autoUpgrade, true),
    caOptions: normalizeCAOptions(data.caOptions),
    dnsProviders: normalizeDNSProviders(data.dnsProviders),
    acmeAccounts: normalizeAcmeAccounts(data.acmeAccounts),
    dnsAccounts: normalizeDNSAccounts(data.dnsAccounts),
    certificates: normalizeCertificates(data.certificates),
    error: asString(data.error),
  }

  overview.value = nextValue
  overviewLoaded = true
  if (
    acmeUpdateInfo.value.currentVersion.trim() !== nextValue.version.trim()
    || acmeUpdateInfo.value.installed !== nextValue.installed
  ) {
    acmeUpdateInfo.value = createIdleAcmeUpdateInfo()
  }
  syncInstallEmailFromOverview(nextValue.contactEmail)
  if (!issueDialogVisible.value) {
    fillIssueDefaults()
  }
}

const applyActionResult = (raw: unknown) => {
  const data = (raw ?? {}) as AcmeActionResult
  if (data.overview) {
    applyOverview(data.overview)
  }
  if (!data.overview) {
    void refreshOverview(true)
  }
  const output = asString(data.output).trim()
  if (output !== '') {
    push.success({
      duration: 4200,
      message: output.split('\n')[0],
    })
  }
}

const normalizeAcmeVersionItem = (raw: unknown): AcmeVersionItem | null => {
  const item = (raw ?? {}) as Partial<AcmeVersionItem>
  const tag = asString((item as any).tag_name || (item as any).tagName).trim()
  if (tag === '') return null
  return {
    tag_name: tag,
    name: asString(item.name),
    published_at: asString((item as any).published_at || (item as any).publishedAt),
    source: asString(item.source),
  }
}

const appendAcmeVersions = (items: AcmeVersionItem[]) => {
  if (items.length === 0) return
  const exists = new Set(acmeVersionItems.value.map(item => item.tag_name))
  items.forEach((item) => {
    if (exists.has(item.tag_name)) return
    exists.add(item.tag_name)
    acmeVersionItems.value.push(item)
  })
}

const fetchAcmeVersions = async (page = 1, append = false) => {
  if (loadingAcmeVersions.value || loadingMoreAcmeVersions.value) return
  if (append) {
    loadingMoreAcmeVersions.value = true
  } else {
    loadingAcmeVersions.value = true
  }
  try {
    const msg = await HttpUtils.get('api/acme-versions', {
      page,
      per_page: acmeVersionPerPage.value,
    })
    if (!msg.success || msg.obj == null) return
    const data = msg.obj as Partial<AcmeVersionListResult>
    const versionsRaw = Array.isArray(data.versions) ? data.versions : []
    const versions = versionsRaw.map(normalizeAcmeVersionItem).filter((item): item is AcmeVersionItem => item != null)
    if (!append) {
      acmeVersionItems.value = []
    }
    appendAcmeVersions(versions)
    acmeVersionPage.value = asNumber((data as any).page, page)
    acmeVersionHasMore.value = asBoolean((data as any).has_more || (data as any).hasMore)
    acmeVersionLoaded.value = true
  } finally {
    loadingAcmeVersions.value = false
    loadingMoreAcmeVersions.value = false
  }
}

const ensureAcmeVersionsLoaded = async () => {
  if (acmeVersionLoaded.value && acmeVersionItems.value.length > 0) return
  await fetchAcmeVersions(1, false)
}

const loadMoreAcmeVersions = async () => {
  if (!acmeVersionHasMore.value) return
  await fetchAcmeVersions(acmeVersionPage.value + 1, true)
}

const normalizeAcmeUpdateInfo = (raw: unknown): AcmeVersionCheckResult => {
  const item = (raw ?? {}) as Partial<AcmeVersionCheckResult>
  return {
    supported: asBoolean(item.supported),
    installed: asBoolean(item.installed),
    currentVersion: asString(item.currentVersion),
    latestVersion: asString(item.latestVersion),
    hasUpdate: asBoolean(item.hasUpdate),
    message: asString(item.message),
  }
}

const normalizeDomainToken = (value: string): string => {
  const text = value.trim().toLowerCase().replace(/^\.+|\.+$/g, '')
  if (text === '') return ''
  if (text.includes('/')) return ''
  return text
}

const normalizeIPToken = (value: string): string => {
  const text = value.trim().replace(/^\[|\]$/g, '')
  if (text === '' || text.includes('/')) return ''
  const ipv4 = /^(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)(\.(25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)){3}$/
  const ipv6 = /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$/
  if (ipv4.test(text)) return text
  if (text.includes(':') && ipv6.test(text)) return text.toLowerCase()
  return ''
}

const normalizeIPList = (raw: unknown[]): string[] => {
  const seen = new Set<string>()
  const result: string[] = []
  raw.forEach((item) => {
    String(item ?? '')
      .replace(/,/g, ' ')
      .split(/\s+/)
      .forEach((entry) => {
        const normalized = normalizeIPToken(entry)
        if (normalized === '' || seen.has(normalized)) return
        seen.add(normalized)
        result.push(normalized)
      })
  })
  return result.slice(0, 100)
}

const buildIssueDomains = (): string[] => {
  if (isIPCertificateMode.value) {
    return normalizeIPList(issueForm.value.ipAddresses)
  }
  const source = `${issueForm.value.mainDomain}\n${issueForm.value.extraDomains}`
    .replace(/,/g, ' ')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')

  const result: string[] = []
  const seen = new Set<string>()
  source.split(/[\s\n]+/).forEach((entry) => {
    const normalized = normalizeDomainToken(entry)
    if (normalized === '' || seen.has(normalized)) return
    seen.add(normalized)
    result.push(normalized)
  })

  return result
}

const buildSelfSignedDomains = (): string[] => {
  const source = `${selfSignedForm.value.mainDomain}\n${selfSignedForm.value.extraDomains}`
    .replace(/,/g, ' ')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')

  const result: string[] = []
  const seen = new Set<string>()
  source.split(/[\s\n]+/).forEach((entry) => {
    const normalized = normalizeDomainToken(entry)
    if (normalized === '' || seen.has(normalized)) return
    seen.add(normalized)
    result.push(normalized)
  })
  return result
}

const issueDomainPreview = computed(() => {
  return buildIssueDomains().join(', ')
})

const selfSignedDomainPreview = computed(() => {
  return buildSelfSignedDomains().join(', ')
})

const isIPCertificateMode = computed(() => issueForm.value.certificateType === 'ip')

const activeChallengeItems = computed(() => {
  return isIPCertificateMode.value ? ipCertificateChallengeItems : challengeItems
})

const activeCAServerItems = computed(() => {
  if (isIPCertificateMode.value) {
    return [{ title: 'Let\'s Encrypt（IP 证书 shortlived）', value: 'letsencrypt' }]
  }
  return caServerItems.value
})

const ipCertificateItems = computed(() => {
  const seen = new Set<string>()
  const result: string[] = []
  const add = (value: string) => {
    const normalized = normalizeIPToken(value)
    if (normalized === '' || seen.has(normalized)) return
    seen.add(normalized)
    result.push(normalized)
  }
  ipCertificateOptions.value.forEach(add)
  issueForm.value.ipAddresses.forEach(add)
  return result
})

const portChallengeValues = ['standalone', 'webroot', 'alpn']

const isPortChallengeSelected = computed(() => {
  return portChallengeValues.includes(issueForm.value.challenge)
})

const shouldShowPortStatus = computed(() => {
  if (isIPCertificateMode.value) return true
  return isPortChallengeSelected.value
})

const visiblePortStatusItems = computed(() => {
  const allowed = new Set<string>(isIPCertificateMode.value
    ? ['standalone', 'alpn']
    : ['standalone', 'webroot', 'alpn'])
  return ipPortStatus.value.ports.filter(item => allowed.has(item.challenge))
})

const selectedIPPortItem = computed(() => {
  if (!shouldShowPortStatus.value) return null
  const challenge = issueForm.value.challenge
  if (challenge === 'webroot') {
    return ipPortStatus.value.ports.find(item => item.challenge === 'webroot')
      ?? ipPortStatus.value.ports.find(item => item.challenge === 'standalone')
      ?? null
  }
  return ipPortStatus.value.ports.find(item => item.challenge === challenge) ?? null
})

const selectedPortChallengeAvailable = computed(() => {
  if (!shouldShowPortStatus.value) return true
  const item = selectedIPPortItem.value
  return item == null ? false : item.available
})

const isExplicitWebrootChallenge = computed(() => {
  return !isIPCertificateMode.value && issueForm.value.challenge === 'webroot'
})

const autoSwitchPortStatusItems = computed(() => {
  if (!shouldShowPortStatus.value) return [] as AcmeIPPortItem[]
  if (isIPCertificateMode.value) return visiblePortStatusItems.value
  if (isExplicitWebrootChallenge.value) {
    return visiblePortStatusItems.value.filter(item => item.challenge === 'webroot')
  }
  return visiblePortStatusItems.value.filter(item => item.challenge === 'standalone' || item.challenge === 'alpn')
})

const hasAnyPortChallengeAvailable = computed(() => {
  if (!shouldShowPortStatus.value) return true
  if (isExplicitWebrootChallenge.value) return true
  return autoSwitchPortStatusItems.value.some(item => item.available)
})

const recommendedPortChallengeItem = computed(() => {
  if (!shouldShowPortStatus.value) return null
  return autoSwitchPortStatusItems.value.find(item => item.recommended && item.available)
    ?? autoSwitchPortStatusItems.value.find(item => item.available)
    ?? null
})

const issueIPFamilyMode = computed(() => {
  let hasIPv4 = false
  let hasIPv6 = false
  issueForm.value.ipAddresses.forEach((value) => {
    const normalized = normalizeIPToken(value)
    if (normalized === '') return
    if (normalized.includes(':')) {
      hasIPv6 = true
    } else {
      hasIPv4 = true
    }
  })
  if (hasIPv4 && hasIPv6) return 'dual'
  if (hasIPv6) return 'ipv6'
  if (hasIPv4) return 'ipv4'
  return ''
})

const issueSignatureAlgorithmText = computed(() => '由 CA 决定（当前不可指定）')

const acmePortCheckHint = '同时检查 80/443 的 tcp/udp'
const acmePortFallbackHint = '80 优先、443 兜底'

const issuePreviewText = computed(() => {
  const preview = issueDomainPreview.value
  if (!isIPCertificateMode.value) {
    if (acmeAccountItems.value.length === 0) {
      return acmeAccountNoDataText.value
    }
    if (issueForm.value.acmeAccountId <= 0) {
      return 'Please select an ACME account before issuing a domain certificate.'
    }
    if (issueForm.value.challenge === 'dns') {
      return `Ready to issue domain certificate for ${preview || 'please fill main domain'} with DNS validation.`
    }
    if (issueForm.value.challenge === 'webroot') {
      return selectedIPPortItem.value?.message
        || `Ready to issue domain certificate for ${preview || 'please fill main domain'} with HTTP Webroot validation (${acmePortCheckHint}).`
    }
    if (!hasAnyPortChallengeAvailable.value) {
      return `No available validation port combination on 80/443 (${acmePortCheckHint}). Please free the ports or stop conflicting services.`
    }
    const selected = selectedIPPortItem.value
    const recommended = recommendedPortChallengeItem.value
    if (selected != null && !selected.available && recommended != null && recommended.challenge !== selected.challenge) {
      return `Current challenge ${ipChallengeTitle(selected.challenge)} is unavailable. The system will switch to ${ipChallengeTitle(recommended.challenge)} automatically (${acmePortFallbackHint}).`
    }
    return `Ready to issue domain certificate for ${preview || 'please fill main domain'} with ${ipChallengeTitle(issueForm.value.challenge)} validation (${acmePortCheckHint}).`
  }

  if (!hasAnyPortChallengeAvailable.value) {
    return `No available validation port combination on 80/443 (${acmePortCheckHint}). IP certificate issuance cannot continue.`
  }
  if (!selectedPortChallengeAvailable.value) {
    const selected = selectedIPPortItem.value
    const recommended = recommendedPortChallengeItem.value
    if (recommended != null) {
      return `Current challenge ${ipChallengeTitle(selected?.challenge || issueForm.value.challenge)} is unavailable. The system will switch to ${ipChallengeTitle(recommended.challenge)} automatically (${acmePortFallbackHint}).`
    }
    return `Current challenge is unavailable: ${selected?.message || 'port occupied'}`
  }
  if (issueIPFamilyMode.value === 'ipv6') {
    return `Ready to issue IP certificate for ${preview || 'please select or input public IP'} with IPv6 listen mode.`
  }
  if (issueIPFamilyMode.value === 'dual') {
    return `Ready to issue IP certificate for ${preview || 'please select or input public IP'}. Dual-stack detected, please ensure external IPv4/IPv6 reachability.`
  }
  return `Ready to issue IP certificate for ${preview || 'please select or input public IP'}.`
})

const canSubmitIssue = computed(() => {
  if (!overview.value.supported || !overview.value.installed) return false
  const domains = buildIssueDomains()
  if (domains.length === 0) return false
  if (isIPCertificateMode.value) {
    if (domains.length > 100) return false
    if (!['standalone', 'alpn'].includes(issueForm.value.challenge)) return false
  } else {
    if (selectedAcmeAccountForIssue.value == null) return false
    if (issueForm.value.challenge === 'webroot' && issueForm.value.webroot.trim() === '') return false
    if (issueForm.value.challenge === 'dns' && issueForm.value.dnsProvider.trim() === '') return false
  }
  if (isPortChallengeSelected.value && !isExplicitWebrootChallenge.value && !hasAnyPortChallengeAvailable.value) {
    return false
  }
  return true
})

const canSubmitSelfSignedIssue = computed(() => {
  if (buildSelfSignedDomains().length === 0) return false
  if (selfSignedForm.value.authorityId <= 0) {
    return false
  }
  if (selfSignedForm.value.durationValue <= 0) return false
  if (!['d', 'm', 'y'].includes(selfSignedForm.value.durationUnit)) return false
  return true
})

const canSaveSelfSignedAuthority = computed(() => {
  return selfSignedAuthorityForm.value.name.trim() !== ''
    && selfSignedAuthorityForm.value.subjectCn.trim() !== ''
    && selfSignedAuthorityForm.value.organization.trim() !== ''
    && selfSignedAuthorityForm.value.country.trim().length === 2
})

const caServerItems = computed(() => {
  return [
    { title: 'Let\'s Encrypt', value: 'letsencrypt' },
    { title: 'ZeroSSL', value: 'zerossl' },
  ]
})

const issueDomainCAServer = computed(() => {
  return normalizeDomainCAValue(issueForm.value.server, 'letsencrypt')
})

const availableAcmeAccountsByCA = computed(() => {
  if (isIPCertificateMode.value) return [] as AcmeAccount[]
  const server = issueDomainCAServer.value
  return overview.value.acmeAccounts.filter((item) => {
    return normalizeDomainCAValue(item.server, server) === server
  })
})

const acmeAccountItems = computed(() => {
  return availableAcmeAccountsByCA.value.map(item => ({
    nameText: item.name.trim() === '' ? `账号 #${item.id}` : item.name.trim(),
    value: item.id,
    metaText: `CA 平台：${caLabel(item.server)} · 邮箱：${item.email || '-'}`,
  }))
})

const acmeAccountNoDataText = computed(() => {
  const currentCA = caLabel(issueDomainCAServer.value)
  return `当前 CA 平台（${currentCA}）下暂无 ACME 账号，请先在 ACME 账号管理中创建`
})

const acmeAccountSelectMessages = computed(() => {
  if (isIPCertificateMode.value) return [] as string[]
  if (acmeAccountItems.value.length > 0) return [] as string[]
  return [acmeAccountNoDataText.value]
})

const selectedAcmeAccountForIssue = computed(() => {
  if (isIPCertificateMode.value) return null
  if (issueForm.value.acmeAccountId <= 0) return null
  return availableAcmeAccountsByCA.value.find(item => item.id === issueForm.value.acmeAccountId) ?? null
})

const dnsProviderItems = computed(() => {
  return overview.value.dnsProviders.map(item => ({
    title: `${item.name} (${item.providerCode})`,
    value: item.providerCode,
  }))
})

const dnsAccountItems = computed(() => {
  const base = [{ title: '不指定 DNS 账号', value: 0 }]
  const items = overview.value.dnsAccounts.map(item => ({
    title: `${item.name} (${item.providerCode})`,
    value: item.id,
  }))
  return [...base, ...items]
})

const ipChallengeTitle = (value: string): string => {
  if (value === 'standalone') return 'HTTP Standalone'
  if (value === 'webroot') return 'HTTP Webroot'
  if (value === 'alpn') return 'TLS ALPN'
  return value || '-'
}

const selfSignedAuthorityItems = computed(() => {
  const rows = selfSignedAuthorities.value.map(item => ({
    title: `${item.name} (${item.platformName || item.platformCode || 'custom'})`,
    value: item.id,
  }))
  return [{ title: '请选择平台模板', value: 0 }, ...rows]
})

const shouldPauseOverviewPolling = computed(() => {
  return issueDialogVisible.value
    || selfSignedDialogVisible.value
    || selfSignedAuthorityManagerVisible.value
    || selfSignedAuthorityFormVisible.value
    || selfSignedAuthorityDetailVisible.value
    || logDialogVisible.value
    || viewDialogVisible.value
    || issueLogVisible.value
})

const filteredCertificates = computed(() => {
  const keyword = searchText.value.trim().toLowerCase()
  if (keyword === '') return overview.value.certificates

  return overview.value.certificates.filter((item) => {
    const bucket = [
      String(item.displayId || ''),
      item.mainDomain,
      item.domains.join(' '),
      item.acmeAccountName,
      item.dnsAccountName,
      item.remark,
      item.caServer,
      item.challenge,
    ].join(' ').toLowerCase()
    return bucket.includes(keyword)
  })
})

const panelAssignedCertificateCount = computed(() => {
  return overview.value.certificates.filter(item => item.inUseByPanel).length
})

const subAssignedCertificateCount = computed(() => {
  return overview.value.certificates.filter(item => item.inUseBySub).length
})

const unapplyDisabledMessage = (target: 'panel' | 'sub'): string => {
  return target === 'panel'
    ? '面板目标至少保留 1 张证书'
    : '订阅目标至少保留 1 张证书'
}

const isUnapplyDisabled = (cert: AcmeCertificate, target: 'panel' | 'sub'): boolean => {
  if (target === 'panel') {
    if (!cert.inUseByPanel) return false
    return panelAssignedCertificateCount.value <= 1
  }
  if (!cert.inUseBySub) return false
  return subAssignedCertificateCount.value <= 1
}

const selectedLogCertificate = computed(() => {
  return overview.value.certificates.find(item => item.id === logCertId.value) ?? null
})

const selectedViewCertificate = computed(() => {
  return overview.value.certificates.find(item => item.id === viewingCertId.value) ?? null
})

const selectedLogContent = computed(() => {
  const cert = selectedLogCertificate.value
  if (cert == null) return '暂无日志'

  const parts: string[] = []
  if (cert.lastError.trim() !== '') {
    parts.push(`最近错误:\n${cert.lastError.trim()}`)
  }
  if (cert.lastOutput.trim() !== '') {
    parts.push(`最近输出:\n${cert.lastOutput.trim()}`)
  }
  if (parts.length === 0) return '暂无输出日志'
  return parts.join('\n\n')
})

const issueLogStatusText = computed(() => {
  switch (issueLogStatus.value) {
    case 'running':
      return '正在执行'
    case 'success':
      return '已完成，3 秒后自动关闭'
    case 'error':
      return '失败，3 秒后自动关闭'
    default:
      return '准备中'
  }
})

const issueLogStyle = computed(() => ({
  zIndex: String(issueLogZIndex.value),
}))

const selectedSelfSignedAuthority = computed(() => {
  return selfSignedAuthorities.value.find(item => item.id === selfSignedForm.value.authorityId) ?? null
})

const selfSignedAuthorityCertificateText = computed(() => {
  return '当前平台模板未托管独立证书内容。该详情仅用于管理机构资料与签发模板。'
})

const selfSignedAuthorityPrivateKeyText = computed(() => {
  return '当前平台模板未托管独立私钥内容。签发时会由本地 sing-box TLS 能力生成证书材料。'
})

const selectedDNSProvider = computed(() => {
  return overview.value.dnsProviders.find(item => item.providerCode === dnsAccountForm.value.providerCode) ?? null
})

const canSaveAcmeAccount = computed(() => {
  return acmeAccountForm.value.name.trim() !== '' && isLikelyValidAcmeEmail(acmeAccountForm.value.email)
})

const canSaveDNSAccount = computed(() => {
  if (dnsAccountForm.value.name.trim() === '') return false
  if (dnsAccountForm.value.providerCode.trim() === '') return false

  const provider = selectedDNSProvider.value
  if (provider == null) return false

  for (const field of provider.fields) {
    if (!field.required) continue
    const value = dnsEnvFieldValue(field.key).trim()
    if (value === '') return false
  }
  if (provider.providerCode === 'dns_cf') {
    const token = dnsEnvFieldValue('CF_Token').trim()
    const accountId = dnsEnvFieldValue('CF_Account_ID').trim()
    const zoneId = dnsEnvFieldValue('CF_Zone_ID').trim()
    const email = dnsEnvFieldValue('CF_Email').trim()
    const key = dnsEnvFieldValue('CF_Key').trim()
    const tokenMode = token !== '' && (accountId !== '' || zoneId !== '')
    const legacyMode = email !== '' && key !== ''
    if (!tokenMode && !legacyMode) return false
  }
  if (provider.providerCode === 'dns_aws') {
    const accessKeyId = dnsEnvFieldValue('AWS_ACCESS_KEY_ID').trim()
    const secretAccessKey = dnsEnvFieldValue('AWS_SECRET_ACCESS_KEY').trim()
    if ((accessKeyId === '' && secretAccessKey !== '') || (accessKeyId !== '' && secretAccessKey === '')) {
      return false
    }
  }

  return true
})

const autoRenewWindowText = computed(() => {
  const info = overview.value.autoRenewWindow
  if (!info || !info.dynamicByValidity) {
    return `${info?.windowDays || 30} 天`
  }
  return `>${info.thresholdDays}天:${info.windowDays}天`
})

const autoRenewWindowHint = computed(() => {
  const info = overview.value.autoRenewWindow
  if (!info || !info.dynamicByValidity) {
    return ''
  }
  return `<=${info.thresholdDays}天证书按 1/3 周期自动续签（至少 ${info.minDynamicWindowDay} 天）`
})

const acmeVersionSelectItems = computed(() => {
  const items = acmeVersionItems.value.map((item) => {
    const published = item.published_at.trim() === '' ? '' : `（${item.published_at.slice(0, 10)}）`
    return {
      title: `${item.tag_name}${published}`,
      value: item.tag_name,
    }
  })
  if (acmeVersionHasMore.value) {
    items.push({
      title: loadingMoreAcmeVersions.value ? '正在加载更多版本...' : '加载更多版本...',
      value: '__load_more__',
    })
  }
  return items
})

const acmeUpdateStatusText = computed(() => {
  const info = acmeUpdateInfo.value
  if (!overview.value.supported) return '当前系统不支持'
  if (info.message.trim() !== '') return info.message
  if (!info.installed) return 'acme.sh 尚未安装'
  if (info.currentVersion.trim() !== '' && info.latestVersion.trim() !== '') {
    if (info.hasUpdate) return `可更新：${info.currentVersion} -> ${info.latestVersion}`
    return `已是最新：${info.currentVersion}`
  }
  return '未检测更新'
})

const challengeLabel = (value: string): string => {
  switch (value) {
    case 'dns':
      return 'DNS 验证'
    case 'standalone':
      return 'HTTP Standalone'
    case 'webroot':
      return 'HTTP Webroot'
    case 'alpn':
      return 'TLS ALPN'
    default:
      return value || '-'
  }
}

const keyLengthLabel = (value: string): string => {
  const normalized = value.trim().toLowerCase()
  if (normalized === '') return '-'
  if (normalized === 'ec-256') return 'EC-256'
  if (normalized === 'ec-384') return 'EC-384'
  return `RSA-${normalized}`
}

const certificateAlgorithmLabel = (value: string): string => {
  const normalized = value.trim().toLowerCase()
  if (normalized === '') return '-'
  if (normalized.startsWith('ecc')) return selfSignedAlgorithmLabel(normalized)
  if (normalized.startsWith('rsa')) return selfSignedAlgorithmLabel(normalized)
  return keyLengthLabel(normalized)
}

const selfSignedAlgorithmLabel = (value: string): string => {
  const normalized = value.trim().toLowerCase()
  switch (normalized) {
    case 'ecc256':
      return 'EC 256'
    case 'ecc384':
      return 'EC 384'
    case 'ecc521':
      return 'EC 521'
    case 'rsa2048':
      return 'RSA 2048'
    case 'rsa3072':
      return 'RSA 3072'
    case 'rsa4096':
      return 'RSA 4096'
    case 'rsa8192':
      return 'RSA 8192'
    default:
      return value || '-'
  }
}

const caLabel = (value: string): string => {
  const normalized = value.trim().replace(/\/+$/g, '')
  if (normalized === '') return '-'

  const canonical = normalizeDomainCAValue(normalized, '')
  const hit = overview.value.caOptions.find(item => item.value === canonical || item.value === normalized)
  if (hit) return hit.name

  if (canonical === 'letsencrypt') return 'Let\'s Encrypt'
  if (canonical === 'zerossl') return 'ZeroSSL'
  return normalized
}

const shortFingerprint = (value: string): string => {
  const normalized = value.trim()
  if (normalized.length <= 24) return normalized
  return `${normalized.slice(0, 12)}...${normalized.slice(-10)}`
}

const formatTimestamp = (unixTs: number): string => {
  if (!Number.isFinite(unixTs) || unixTs <= 0) return '-'
  return new Date(unixTs * 1000).toLocaleString()
}

const expireSummary = (unixTs: number): string => {
  if (!Number.isFinite(unixTs) || unixTs <= 0) return '未知到期时间'
  const now = Date.now()
  const target = unixTs * 1000
  const diff = target - now
  const days = Math.floor(Math.abs(diff) / (24 * 3600 * 1000))
  if (diff < 0) return `已过期 ${days} 天`
  if (days === 0) return '24 小时内到期'
  return `剩余 ${days} 天`
}

const statusText = (cert: AcmeCertificate): string => {
  if (cert.status === 'expired') return '已过期'
  if (cert.status === 'error') return '异常'
  if (cert.notAfter > 0) {
    const remainDays = Math.floor((cert.notAfter * 1000 - Date.now()) / (24 * 3600 * 1000))
    if (remainDays <= 7) return '即将到期'
  }
  return '正常'
}

const statusColor = (cert: AcmeCertificate): string => {
  if (cert.status === 'expired') return 'error'
  if (cert.status === 'error') return 'warning'
  if (cert.notAfter > 0) {
    const remainDays = Math.floor((cert.notAfter * 1000 - Date.now()) / (24 * 3600 * 1000))
    if (remainDays <= 7) return 'warning'
  }
  return 'success'
}

const isAcmeCertificate = (cert: AcmeCertificate): boolean => {
  return cert.sourceType === 'acme'
}

const isSelfSignedCertificate = (cert: AcmeCertificate): boolean => {
  return cert.sourceType === 'self_signed'
}

const supportsRenew = (cert: AcmeCertificate): boolean => {
  return isAcmeCertificate(cert) || isSelfSignedCertificate(cert)
}

const supportsAutoRenew = (cert: AcmeCertificate): boolean => {
  return supportsRenew(cert)
}

const dnsEnvSummary = (env: Record<string, string>): string => {
  const keys = Object.keys(env)
  if (keys.length === 0) return '-'
  if (keys.length === 1) return keys[0]
  return `${keys[0]} 等 ${keys.length} 项`
}

const fillIssueDefaults = () => {
  issueForm.value.challenge = overview.value.defaultChallenge || 'standalone'
  issueForm.value.webroot = overview.value.defaultWebroot || ''
  issueForm.value.dnsProvider = overview.value.defaultDnsProvider || ''
  issueForm.value.server = normalizeDomainCAValue(overview.value.preferredCA, 'letsencrypt')
  issueForm.value.keyLength = overview.value.defaultKeyLength || 'ec-256'
  issueForm.value.pushDir = ''
  issueForm.value.customArgs = ''
  issueForm.value.dnsEnv = ''
  issueForm.value.applyTarget = ''
  issueForm.value.acmeAccountId = 0
  issueForm.value.dnsAccountId = 0
  issueForm.value.remark = ''
  if (issueForm.value.certificateType === 'ip') {
    applyIPCertificateDefaults()
  }
}

const applyIPCertificateDefaults = () => {
  issueForm.value.challenge = ['standalone', 'alpn'].includes(issueForm.value.challenge)
    ? issueForm.value.challenge
    : 'standalone'
  issueForm.value.acmeAccountId = 0
  issueForm.value.webroot = ''
  issueForm.value.dnsProvider = ''
  issueForm.value.dnsAccountId = 0
  issueForm.value.dnsEnv = ''
  issueForm.value.server = 'letsencrypt'
}

const clearSelfSignedForm = () => {
  selfSignedForm.value.authorityId = selfSignedAuthorities.value[0]?.id ?? 0
  selfSignedForm.value.mainDomain = ''
  selfSignedForm.value.extraDomains = ''
  selfSignedForm.value.keyAlgorithm = 'ecc256'
  selfSignedForm.value.signatureAlgorithm = 'ecc256'
  selfSignedForm.value.durationValue = 90
  selfSignedForm.value.durationUnit = 'd'
  selfSignedForm.value.remark = ''
  selfSignedForm.value.applyTarget = ''
  selfSignedForm.value.pushDir = ''
}

const clearIssueForm = () => {
  issueForm.value.certificateType = 'domain'
  issueForm.value.mainDomain = ''
  issueForm.value.extraDomains = ''
  issueForm.value.ipAddresses = []
  fillIssueDefaults()
}

const refreshOverview = async (silent = false, force = false) => {
  if (overviewRequestPromise != null) {
    return overviewRequestPromise
  }
  if (!force && overviewLoaded && !props.active) {
    return
  }
  if (!silent && (force || overviewRequestPromise == null)) {
    loadingOverview.value = true
  }
  let request: Promise<void> | null = null
  request = (async () => {
    try {
      const msg = await HttpUtils.get('api/acme-overview')
      if (msg.success) {
        applyOverview(msg.obj)
      }
    } finally {
      if (overviewRequestPromise === request) {
        overviewRequestPromise = null
      }
      if (!silent) {
        loadingOverview.value = false
      }
    }
  })()
  overviewRequestPromise = request
  return request
}

const refreshSelfSignedAuthorities = async (force = false) => {
  if (selfSignedAuthoritiesRequestPromise != null) {
    return selfSignedAuthoritiesRequestPromise
  }
  if (!force && selfSignedAuthoritiesLoaded && !selfSignedAuthoritiesDirty) {
    return
  }
  loadingSelfSignedAuthorities.value = true
  let request: Promise<void> | null = null
  request = (async () => {
    try {
      const msg = await HttpUtils.get('api/self-signed-authorities')
      if (!msg.success) return
      selfSignedAuthorities.value = normalizeSelfSignedAuthorities(msg.obj)
      selfSignedAuthoritiesLoaded = true
      selfSignedAuthoritiesDirty = false
      if (selfSignedForm.value.authorityId <= 0) {
        selfSignedForm.value.authorityId = selfSignedAuthorities.value[0]?.id ?? 0
      }
    } finally {
      if (selfSignedAuthoritiesRequestPromise === request) {
        selfSignedAuthoritiesRequestPromise = null
      }
      loadingSelfSignedAuthorities.value = false
    }
  })()
  selfSignedAuthoritiesRequestPromise = request
  return request
}

const refreshIPCertificateOptions = async () => {
  loadingIPOptions.value = true
  try {
    const [serverIpsMsg, inboundIpsMsg] = await Promise.all([
      HttpUtils.get('api/server-ips?verify=true'),
      HttpUtils.get('api/mihomo-inbound-ips'),
    ])
    const values: string[] = []
    if (serverIpsMsg.success && Array.isArray(serverIpsMsg.obj)) {
      values.push(...serverIpsMsg.obj.map((item: unknown) => String(item ?? '')))
    }
    if (inboundIpsMsg.success && Array.isArray(inboundIpsMsg.obj)) {
      inboundIpsMsg.obj.forEach((item: any) => {
        values.push(String(item?.server ?? ''))
      })
    }
    ipCertificateOptions.value = normalizeIPList(values)
    if (issueForm.value.ipAddresses.length === 0 && ipCertificateOptions.value.length > 0) {
      issueForm.value.ipAddresses = [ipCertificateOptions.value[0]]
    }
  } finally {
    loadingIPOptions.value = false
  }
}

const refreshIPPortStatus = async () => {
  loadingIPPortStatus.value = true
  try {
    const msg = await HttpUtils.get('api/acme-ip-port-status')
    if (!msg.success || msg.obj == null) return
    const raw = msg.obj as Partial<AcmeIPPortStatus>
    const ports = Array.isArray(raw.ports)
      ? raw.ports.map((item) => {
        const row = item as Partial<AcmeIPPortItem>
        const tcpOccupied = asBoolean(row.tcpOccupied, asBoolean(row.occupied))
        const udpOccupied = asBoolean(row.udpOccupied)
        const availableFallback = !tcpOccupied || asString(row.challenge) === 'webroot'
        const reason = asString(row.reason, asString(row.message))
        return {
          challenge: asString(row.challenge),
          port: asNumber(row.port),
          occupied: asBoolean(row.occupied, tcpOccupied),
          available: asBoolean(row.available, availableFallback),
          tcpOccupied,
          udpOccupied,
          recommended: asBoolean(row.recommended),
          reason,
          message: asString(row.message, reason),
        }
      }).filter(item => item.challenge !== '' && item.port > 0)
      : []
    ipPortStatus.value = {
      supported: asBoolean(raw.supported),
      checkedAt: asNumber(raw.checkedAt),
      ports: ports.length > 0 ? ports : ipPortStatus.value.ports,
    }
  } finally {
    loadingIPPortStatus.value = false
  }
}

const normalizeIssueIPSelection = (value: unknown) => {
  const raw = Array.isArray(value) ? value : [value]
  const normalized = normalizeIPList(raw)
  if (raw.length > 100 || normalized.length >= 100) {
    push.warning({
      duration: 3600,
      message: 'IP 证书最多选择或输入 100 个 IP',
    })
  }
  issueForm.value.ipAddresses = normalized
}

const stopIssueLogPolling = () => {
  if (issueLogTimer.value != null) {
    window.clearInterval(issueLogTimer.value)
    issueLogTimer.value = null
  }
}

const clearIssueLogCloseTimer = () => {
  if (issueLogCloseTimer.value != null) {
    window.clearTimeout(issueLogCloseTimer.value)
    issueLogCloseTimer.value = null
  }
}

const scrollIssueLogToBottom = () => {
  void nextTick(() => {
    const el = issueLogBodyRef.value
    if (el == null) return
    el.scrollTop = el.scrollHeight
  })
}

const scheduleIssueLogAutoClose = () => {
  clearIssueLogCloseTimer()
  issueLogCloseTimer.value = window.setTimeout(() => {
    closeIssueLog()
  }, 3000)
}

const closeIssueLog = () => {
  issueLogVisible.value = false
  stopIssueLogPolling()
  clearIssueLogCloseTimer()
}

const syncIssueLogZIndex = () => {
  void nextTick(() => {
    let baseZIndex = 2600
    const dialogContent = document.querySelector<HTMLElement>('.acme-issue-dialog')
    if (dialogContent != null) {
      const overlay = dialogContent.closest('.v-overlay') as HTMLElement | null
      const target = overlay ?? dialogContent
      const computed = Number.parseInt(window.getComputedStyle(target).zIndex || '', 10)
      if (Number.isFinite(computed) && computed > 0) {
        baseZIndex = computed
      }
    }
    issueLogZIndex.value = baseZIndex + 1
  })
}

const normalizeAcmeLogSession = (raw: unknown): AcmeLogSession => {
  const data = (raw ?? {}) as Partial<AcmeLogSession>
  const lines = Array.isArray(data.lines)
    ? data.lines.map(line => String(line ?? '')).filter(line => line.trim() !== '')
    : []
  return {
    id: asString(data.id),
    title: asString(data.title),
    status: asString(data.status, 'missing'),
    lines,
    error: asString(data.error),
    startedAt: asNumber(data.startedAt),
    updatedAt: asNumber(data.updatedAt),
    finishedAt: asNumber(data.finishedAt),
  }
}

const pollIssueLog = async () => {
  if (issueLogSessionId.value === '') return
  const msg = await HttpUtils.get('api/acme-log', { id: issueLogSessionId.value })
  if (!msg.success) return

  const session = normalizeAcmeLogSession(msg.obj)
  issueLogStatus.value = session.status
  issueLogLines.value = session.lines.length > 0 ? session.lines : ['等待后端开始写入日志...']
  scrollIssueLogToBottom()

  if (session.status === 'success' || session.status === 'error') {
    stopIssueLogPolling()
    scheduleIssueLogAutoClose()
  }
}

const makeIssueLogSessionId = (): string => {
  const randomPart = typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function'
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`
  return `issue-${randomPart}`
}

const openIssueLog = (sessionId: string) => {
  stopIssueLogPolling()
  clearIssueLogCloseTimer()
  issueLogSessionId.value = sessionId
  issueLogStatus.value = 'running'
  issueLogLines.value = ['准备提交签发任务...']
  issueLogVisible.value = true
  syncIssueLogZIndex()
  scrollIssueLogToBottom()
  issueLogTimer.value = window.setInterval(() => {
    void pollIssueLog()
  }, 1000)
}

const installAcme = async () => {
  const beforeVersion = overview.value.version.trim()
  let targetVersion = selectedAcmeVersion.value.trim()
  if (targetVersion === '__load_more__') {
    targetVersion = ''
  }
  if (targetVersion !== '') {
    const hit = acmeVersionItems.value.find(item => item.tag_name === targetVersion)
    if (!hit) {
      push.warning({
        duration: 3500,
        message: `所选版本不可用：${targetVersion}`,
      })
      return
    }
  }

  installing.value = true
  try {
    const msg = await HttpUtils.post('api/acme-install', {
      email: installEmail.value.trim(),
      version: targetVersion,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      const afterVersion = overview.value.version.trim()
      const displayAfter = afterVersion || targetVersion || '未知版本'
      if (beforeVersion === '') {
        push.success({
          duration: 3500,
          message: `acme.sh 已安装，当前版本：${displayAfter}`,
        })
      } else {
        push.success({
          duration: 3500,
          message: `acme.sh 已重装：${beforeVersion} -> ${displayAfter}`,
        })
      }
      if (targetVersion !== '') {
        selectedAcmeVersion.value = targetVersion
      }
      void refreshOverview(true)
      return
    }
    if (targetVersion !== '') {
      push.warning({
        duration: 4200,
        message: `版本 ${targetVersion} 无法下载或安装`,
      })
    }
  } finally {
    installing.value = false
  }
}

const checkAcmeUpdate = async (silent = false) => {
  checkingAcmeUpdate.value = true
  try {
    const msg = await HttpUtils.get('api/acme-update-info')
    if (!msg.success) return
    acmeUpdateInfo.value = normalizeAcmeUpdateInfo(msg.obj)
    if (!silent && acmeUpdateInfo.value.message.trim() !== '') {
      push.success({
        duration: 4200,
        message: acmeUpdateInfo.value.message,
      })
    }
  } finally {
    checkingAcmeUpdate.value = false
  }
}

const removeAcme = async () => {
  if (!window.confirm('确认删除已下载的 acme.sh 吗？仅删除受管 acme，不会删除证书与用户自放文件。')) {
    return
  }
  removingAcme.value = true
  try {
    const msg = await HttpUtils.post('api/acme-remove', {
      removeCertificates: false,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      selectedAcmeVersion.value = ''
      acmeUpdateInfo.value = normalizeAcmeUpdateInfo({
        supported: overview.value.supported,
        installed: false,
        currentVersion: '',
        latestVersion: '',
        hasUpdate: false,
        message: 'acme.sh 已删除',
      })
      push.success({
        duration: 4200,
        message: 'acme.sh 已删除（证书与推送目录未受影响）',
      })
    }
  } finally {
    removingAcme.value = false
  }
}

const onAcmeVersionMenuUpdate = (opened: boolean) => {
  if (!opened) return
  void ensureAcmeVersionsLoaded()
}

const onAcmeVersionChanged = (value: string) => {
  if (value !== '__load_more__') return
  const last = acmeVersionItems.value[acmeVersionItems.value.length - 1]
  selectedAcmeVersion.value = last?.tag_name || ''
  void loadMoreAcmeVersions()
}

const openIssueDialog = () => {
  clearIssueForm()
  issueDialogVisible.value = true
  void refreshIPCertificateOptions()
  void refreshIPPortStatus()
}

const openSelfSignedDialog = () => {
  clearSelfSignedForm()
  selfSignedDialogVisible.value = true
  void refreshSelfSignedAuthorities()
}

const resetSelfSignedAuthorityForm = () => {
  selfSignedAuthorityForm.value = createEmptySelfSignedAuthorityForm()
}

const buildSelfSignedAuthorityPlatformCode = (value: string): string => {
  const normalized = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return normalized || 'custom'
}

const openSelfSignedAuthorityManager = () => {
  selfSignedAuthorityManagerVisible.value = true
  void refreshSelfSignedAuthorities()
}

const openSelfSignedAuthorityForm = (item?: SelfSignedAuthority) => {
  if (!item) {
    resetSelfSignedAuthorityForm()
  } else {
    selfSignedAuthorityForm.value = {
      id: item.id,
      name: item.name,
      platformCode: item.platformCode,
      platformName: item.platformName,
      subjectCn: item.subjectCn,
      organization: item.organization,
      department: item.department,
      country: item.country || 'US',
      province: item.province,
      city: item.city,
      issuerName: item.issuerName,
      issuerOrg: item.issuerOrg,
      caUrl: item.caUrl,
      ocspUrl: item.ocspUrl,
      crlUrl: item.crlUrl,
      keyUsage: item.keyUsage,
      extKeyUsage: item.extKeyUsage,
      brand: item.brand,
      notes: item.notes,
    }
  }
  selfSignedAuthorityFormVisible.value = true
}

const openSelfSignedAuthorityDetail = (item: SelfSignedAuthority) => {
  selfSignedAuthorityDetail.value = { ...item }
  selfSignedAuthorityDetailTab.value = 'profile'
  selfSignedAuthorityDetailVisible.value = true
}

const selectSelfSignedAuthority = (item: SelfSignedAuthority) => {
  selfSignedForm.value.authorityId = item.id
  selfSignedAuthorityManagerVisible.value = false
  if (!selfSignedDialogVisible.value) {
    selfSignedDialogVisible.value = true
  }
}

const saveSelfSignedAuthority = async () => {
  if (!canSaveSelfSignedAuthority.value) return

  savingSelfSignedAuthority.value = true
  try {
    const name = selfSignedAuthorityForm.value.name.trim()
    const platformCode = selfSignedAuthorityForm.value.platformCode.trim() || buildSelfSignedAuthorityPlatformCode(name)
    const platformName = selfSignedAuthorityForm.value.platformName.trim() || name
    const msg = await HttpUtils.post('api/self-signed-authority-save', {
      id: selfSignedAuthorityForm.value.id > 0 ? selfSignedAuthorityForm.value.id : undefined,
      name,
      platformCode,
      platformName,
      subjectCn: selfSignedAuthorityForm.value.subjectCn.trim(),
      organization: selfSignedAuthorityForm.value.organization.trim(),
      department: selfSignedAuthorityForm.value.department.trim(),
      country: selfSignedAuthorityForm.value.country.trim().toUpperCase(),
      province: selfSignedAuthorityForm.value.province.trim(),
      city: selfSignedAuthorityForm.value.city.trim(),
      issuerName: selfSignedAuthorityForm.value.issuerName.trim(),
      issuerOrg: selfSignedAuthorityForm.value.issuerOrg.trim(),
      caUrl: selfSignedAuthorityForm.value.caUrl.trim(),
      ocspUrl: selfSignedAuthorityForm.value.ocspUrl.trim(),
      crlUrl: selfSignedAuthorityForm.value.crlUrl.trim(),
      keyUsage: selfSignedAuthorityForm.value.keyUsage.trim(),
      extKeyUsage: selfSignedAuthorityForm.value.extKeyUsage.trim(),
      brand: selfSignedAuthorityForm.value.brand.trim(),
      notes: selfSignedAuthorityForm.value.notes.trim(),
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      selfSignedAuthoritiesDirty = true
      await refreshSelfSignedAuthorities(true)
      selfSignedAuthorityFormVisible.value = false
      push.success({
        duration: 3600,
        message: '平台已保存',
      })
    }
  } finally {
    savingSelfSignedAuthority.value = false
  }
}

const deleteSelfSignedAuthority = async (item: SelfSignedAuthority) => {
  if (item.builtin) return
  const confirmed = window.confirm(`确认删除平台「${item.name}」吗？`)
  if (!confirmed) return

  const msg = await HttpUtils.post('api/self-signed-authority-delete', {
    id: item.id,
  })
  if (msg.success) {
    applyActionResult(msg.obj)
    selfSignedAuthoritiesDirty = true
    await refreshSelfSignedAuthorities(true)
    if (selfSignedForm.value.authorityId === item.id) {
      selfSignedForm.value.authorityId = selfSignedAuthorities.value[0]?.id ?? 0
    }
    push.success({
      duration: 3200,
      message: '平台已删除',
    })
  }
}

const downloadSelfSignedAuthority = (item: SelfSignedAuthority) => {
  const lines = [
    `名称: ${item.name || '-'}`,
    `平台编码: ${item.platformCode || '-'}`,
    `平台显示名: ${item.platformName || '-'}`,
    `证书主体名称(CN): ${item.subjectCn || '-'}`,
    `公司/组织: ${item.organization || '-'}`,
    `部门: ${item.department || '-'}`,
    `国家代号: ${item.country || '-'}`,
    `省份: ${item.province || '-'}`,
    `城市: ${item.city || '-'}`,
    `密钥算法: ${selfSignedAlgorithmLabel(item.keyAlgorithm)}`,
    `更新时间: ${formatTimestamp(item.updatedAt || item.createdAt)}`,
  ]
  const blob = new Blob([`${lines.join('\r\n')}\r\n`], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${(item.platformCode || item.name || 'authority').replace(/[^a-zA-Z0-9._-]+/g, '_')}.txt`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

const issueCertificate = async () => {
  if (!isIPCertificateMode.value && selectedAcmeAccountForIssue.value == null) {
    const warningMessage = acmeAccountItems.value.length === 0
      ? acmeAccountNoDataText.value
      : '域名证书签发必须选择 ACME 账号'
    push.warning({
      duration: 4000,
      message: warningMessage,
    })
    return
  }

  if (!isIPCertificateMode.value && selectedAcmeAccountForIssue.value != null) {
    const account = selectedAcmeAccountForIssue.value
    if (!isLikelyValidAcmeEmail(account.email)) {
      push.warning({
        duration: 4200,
        message: '所选 ACME 账号邮箱格式无效，请先在 ACME 账号管理中修正邮箱',
      })
      return
    }
  }

  const domains = buildIssueDomains()
  if (domains.length === 0) {
    push.warning({
      duration: 4000,
      message: isIPCertificateMode.value ? '请至少选择或输入一个 IP' : '请至少填写一个域名',
    })
    return
  }
  if (isIPCertificateMode.value && domains.length > 100) {
    push.warning({
      duration: 4000,
      message: 'IP 证书最多支持 100 个 IP',
    })
    return
  }
  if (isPortChallengeSelected.value) {
    await refreshIPPortStatus()
    if (!isExplicitWebrootChallenge.value && !hasAnyPortChallengeAvailable.value) {
      push.warning({
        duration: 5000,
        message: '80/443 没有可用的验证端口组合，请调整站点端口或停止冲突服务后重试。',
      })
      return
    }
  }
  issuing.value = true
  const logSessionId = makeIssueLogSessionId()
  openIssueLog(logSessionId)
  const isDNSChallenge = !isIPCertificateMode.value && issueForm.value.challenge === 'dns'
  try {
    const msg = await HttpUtils.post('api/acme-issue', {
      domains: domains.join('\n'),
      certificateType: issueForm.value.certificateType,
      challenge: issueForm.value.challenge,
      webroot: issueForm.value.webroot,
      dnsProvider: isDNSChallenge ? issueForm.value.dnsProvider : undefined,
      dnsEnv: isDNSChallenge ? issueForm.value.dnsEnv : undefined,
      server: issueForm.value.server,
      keyLength: issueForm.value.keyLength,
      customArgs: issueForm.value.customArgs,
      acmeAccountId: !isIPCertificateMode.value && issueForm.value.acmeAccountId > 0 ? issueForm.value.acmeAccountId : undefined,
      dnsAccountId: isDNSChallenge && issueForm.value.dnsAccountId > 0 ? issueForm.value.dnsAccountId : undefined,
      remark: issueForm.value.remark,
      applyTarget: issueForm.value.applyTarget,
      pushDir: issueForm.value.pushDir,
      logSessionId,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      push.success({
        duration: 4200,
        message: '证书签发完成',
      })
      issueDialogVisible.value = false
      clearIssueForm()
    } else {
      issueLogStatus.value = 'error'
      issueLogLines.value = [...issueLogLines.value, msg.msg || '证书签发失败']
      stopIssueLogPolling()
      scheduleIssueLogAutoClose()
    }
  } finally {
    void pollIssueLog()
    issuing.value = false
  }
}

const issueSelfSignedCertificate = async () => {
  const domains = buildSelfSignedDomains()
  if (domains.length === 0) {
    push.warning({
      duration: 4000,
      message: '请至少填写一个域名',
    })
    return
  }

  issuingSelfSigned.value = true
  try {
    const msg = await HttpUtils.post('api/self-signed-issue', {
      authorityId: selfSignedForm.value.authorityId > 0
        ? selfSignedForm.value.authorityId
        : undefined,
      domains: domains.join('\n'),
      keyAlgorithm: selfSignedForm.value.keyAlgorithm,
      signatureAlgorithm: selfSignedForm.value.signatureAlgorithm,
      durationValue: selfSignedForm.value.durationValue,
      durationUnit: selfSignedForm.value.durationUnit,
      remark: selfSignedForm.value.remark,
      applyTarget: selfSignedForm.value.applyTarget,
      pushDir: selfSignedForm.value.pushDir,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      push.success({
        duration: 4200,
        message: '自签证书签发完成',
      })
      selfSignedDialogVisible.value = false
      clearSelfSignedForm()
      selfSignedAuthoritiesDirty = true
      await refreshSelfSignedAuthorities(true)
    }
  } finally {
    issuingSelfSigned.value = false
  }
}

const renewCertificate = async (cert: AcmeCertificate, force: boolean) => {
  if (force) {
    const confirmed = window.confirm(`确认强制续签证书 ${cert.mainDomain} 吗？`)
    if (!confirmed) return
  }
  rowBusyId.value = cert.id
  try {
    const msg = await HttpUtils.post('api/acme-renew', {
      id: cert.id,
      force,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      push.success({
        duration: 3500,
        message: force ? '强制续签完成' : '证书续签完成',
      })
    }
  } finally {
    rowBusyId.value = 0
  }
}

const toggleCertificateAutoRenew = async (cert: AcmeCertificate) => {
  if (!supportsAutoRenew(cert)) return
  rowBusyId.value = cert.id
  try {
    const msg = await HttpUtils.post('api/acme-set-auto-renew', {
      id: cert.id,
      autoRenew: !cert.autoRenew,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      push.success({
        duration: 3200,
        message: !cert.autoRenew ? '自动续签已开启' : '自动续签已关闭',
      })
    }
  } finally {
    rowBusyId.value = 0
  }
}

const toggleCertificateApply = async (cert: AcmeCertificate, target: 'panel' | 'sub') => {
  const isApplied = target === 'panel' ? cert.inUseByPanel : cert.inUseBySub
  const targetLabel = target === 'panel' ? '面板' : '订阅'
  if (isApplied && isUnapplyDisabled(cert, target)) {
    push.warning({
      duration: 4000,
      message: unapplyDisabledMessage(target),
    })
    return
  }

  if (isApplied) {
    const confirmed = window.confirm(['确认取消应用到', targetLabel, '吗？'].join(''))
    if (!confirmed) return
  }

  rowBusyId.value = cert.id
  try {
    const msg = await HttpUtils.post(isApplied ? 'api/acme-unapply' : 'api/acme-apply', {
      id: cert.id,
      target,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      push.success({
        duration: 3500,
        message: isApplied
          ? (target === 'panel' ? '证书已取消应用到面板' : '证书已取消应用到订阅')
          : (target === 'panel' ? '证书已应用到面板' : '证书已应用到订阅'),
      })
    }
  } finally {
    rowBusyId.value = 0
  }
}

const deleteCertificate = async (cert: AcmeCertificate) => {
  if (cert.deleteBlocked) {
    push.warning({
      duration: 4000,
      message: cert.usageLabel || '证书正在被界面或订阅使用，无法删除',
    })
    return
  }
  const confirmed = window.confirm(`确认删除证书 ${cert.mainDomain} 吗？`)
  if (!confirmed) return

  rowBusyId.value = cert.id
  try {
    const msg = await HttpUtils.post('api/acme-delete', { id: cert.id })
    if (msg.success) {
      applyActionResult(msg.obj)
      push.success({
        duration: 3200,
        message: '证书已删除',
      })
    }
  } finally {
    rowBusyId.value = 0
  }
}

const openPushDialog = (cert: AcmeCertificate) => {
  pushDialogCertId.value = cert.id
  pushDialogTargetDir.value = cert.pushDir || ''
  pushDialogVisible.value = true
}

const closePushDialog = () => {
  pushDialogVisible.value = false
  pushDialogCertId.value = 0
  pushDialogTargetDir.value = ''
}

const openViewDialog = async (cert: AcmeCertificate) => {
  viewingCertId.value = cert.id
  viewLoading.value = true
  viewDialogVisible.value = true
  viewContent.value = {
    id: cert.id,
    mainDomain: cert.mainDomain,
    sourceType: cert.sourceType,
    sourceRef: cert.sourceRef,
    fullchainPem: '',
    keyPem: '',
    issuedKeyAlgorithm: cert.issuedKeyAlgorithm,
    issuedSignatureAlgorithm: cert.issuedSignatureAlgorithm,
  }
  try {
    const msg = await HttpUtils.post('api/acme-view', { id: cert.id })
    if (msg.success) {
      const raw = (msg.obj ?? {}) as Partial<AcmeCertificateMaterial>
      viewContent.value = {
        id: asNumber(raw.id, cert.id),
        mainDomain: asString(raw.mainDomain, cert.mainDomain),
        sourceType: asString(raw.sourceType, cert.sourceType),
        sourceRef: asString(raw.sourceRef, cert.sourceRef),
        fullchainPem: asString(raw.fullchainPem),
        keyPem: asString(raw.keyPem),
        issuedKeyAlgorithm: asString(raw.issuedKeyAlgorithm, cert.issuedKeyAlgorithm),
        issuedSignatureAlgorithm: asString(raw.issuedSignatureAlgorithm, cert.issuedSignatureAlgorithm),
      }
    }
  } finally {
    viewLoading.value = false
  }
}

const confirmPushDialog = async () => {
  if (pushDialogCertId.value === 0) return
  const targetDir = pushDialogTargetDir.value.trim()
  if (targetDir === '') return

  pushingId.value = pushDialogCertId.value
  try {
    const msg = await HttpUtils.post('api/acme-push', {
      id: pushDialogCertId.value,
      targetDir,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      push.success({
        duration: 3500,
        message: '证书已推送到目录',
      })
      closePushDialog()
    }
  } finally {
    pushingId.value = 0
  }
}

const openLogDialog = (cert: AcmeCertificate) => {
  logCertId.value = cert.id
  logDialogVisible.value = true
}

const openAcmeAccountForm = (item?: AcmeAccount) => {
  if (!item) {
    acmeAccountForm.value = {
      id: 0,
      name: '',
      email: '',
      server: normalizeDomainCAValue(overview.value.preferredCA, 'letsencrypt'),
      keyLength: overview.value.defaultKeyLength || 'ec-256',
      remark: '',
    }
  } else {
    acmeAccountForm.value = {
      id: item.id,
      name: item.name,
      email: item.email,
      server: normalizeDomainCAValue(item.server, 'letsencrypt'),
      keyLength: item.keyLength,
      remark: item.remark,
    }
  }
  acmeAccountFormVisible.value = true
}

const saveAcmeAccount = async () => {
  if (!canSaveAcmeAccount.value) return
  const normalizedEmail = normalizeInlineEmail(acmeAccountForm.value.email)
  if (!isLikelyValidAcmeEmail(normalizedEmail)) {
    push.warning({
      duration: 3600,
      message: '邮箱格式无效，请使用 ASCII 邮箱（示例：name@example.com）',
    })
    return
  }
  if (normalizedEmail !== acmeAccountForm.value.email) {
    acmeAccountForm.value.email = normalizedEmail
  }
  const normalizedServer = normalizeDomainCAValue(acmeAccountForm.value.server, '')
  if (normalizedServer === '') {
    push.warning({
      duration: 3600,
      message: 'CA 平台仅支持 Let\'s Encrypt 或 ZeroSSL',
    })
    return
  }
  acmeAccountForm.value.server = normalizedServer

  savingAcmeAccount.value = true
  try {
    const msg = await HttpUtils.post('api/acme-account-save', {
      id: acmeAccountForm.value.id > 0 ? acmeAccountForm.value.id : undefined,
      name: acmeAccountForm.value.name,
      email: normalizedEmail,
      server: normalizedServer,
      keyLength: acmeAccountForm.value.keyLength,
      remark: acmeAccountForm.value.remark,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      acmeAccountFormVisible.value = false
      push.success({
        duration: 3600,
        message: 'ACME 账号已保存',
      })
    }
  } finally {
    savingAcmeAccount.value = false
  }
}

const deleteAcmeAccount = async (item: AcmeAccount) => {
  const confirmed = window.confirm(`确认删除 ACME 账号「${item.name}」吗？`)
  if (!confirmed) return

  const msg = await HttpUtils.post('api/acme-account-delete', {
    id: item.id,
  })
  if (msg.success) {
    applyActionResult(msg.obj)
    push.success({
      duration: 3200,
      message: 'ACME 账号已删除',
    })
  }
}

const openDNSAccountForm = (item?: AcmeDNSAccount) => {
  if (!item) {
    dnsAccountForm.value = {
      id: 0,
      name: '',
      providerCode: overview.value.defaultDnsProvider || (overview.value.dnsProviders[0]?.providerCode || ''),
      env: {},
      extraEnvText: '',
      remark: '',
    }
  } else {
    dnsAccountForm.value = {
      id: item.id,
      name: item.name,
      providerCode: item.providerCode,
      env: { ...item.env },
      extraEnvText: '',
      remark: item.remark,
    }

    const provider = overview.value.dnsProviders.find(row => row.providerCode === item.providerCode)
    if (provider) {
      const knownKeys = new Set(provider.fields.map(field => field.key))
      const extras: string[] = []
      Object.entries(item.env).forEach(([key, value]) => {
        if (knownKeys.has(key)) return
        extras.push(`${key}=${value}`)
      })
      dnsAccountForm.value.extraEnvText = extras.join('\n')
    }
  }

  dnsAccountFormVisible.value = true
}

const dnsEnvFieldValue = (key: string): string => {
  return dnsAccountForm.value.env[key] || ''
}

const setDnsEnvField = (key: string, value: string) => {
  dnsAccountForm.value.env[key] = value
}

const isSecretLikeField = (key: string): boolean => {
  const normalized = key.toLowerCase()
  return normalized.includes('token') || normalized.includes('secret') || normalized.includes('password') || normalized.includes('private_key') || normalized.includes('access_key') || normalized.includes('api_key') || normalized.endsWith('_key') || normalized.endsWith('_key_id') || normalized.endsWith('_secret')
}

const parseExtraEnvLines = (raw: string): { env: Record<string, string>; invalidLine: string } => {
  const env: Record<string, string> = {}
  const lines = raw.replace(/\r\n/g, '\n').replace(/\r/g, '\n').split('\n')
  for (const lineRaw of lines) {
    const line = lineRaw.trim()
    if (line === '' || line.startsWith('#')) continue
    const idx = line.indexOf('=')
    if (idx <= 0) {
      return { env: {}, invalidLine: line }
    }
    const key = line.slice(0, idx).trim()
    const value = line.slice(idx + 1).trim()
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(key) || value === '') {
      return { env: {}, invalidLine: line }
    }
    env[key] = value
  }
  return { env, invalidLine: '' }
}

const saveDNSAccount = async () => {
  if (!canSaveDNSAccount.value) return

  const parsedExtra = parseExtraEnvLines(dnsAccountForm.value.extraEnvText)
  if (parsedExtra.invalidLine !== '') {
    push.warning({
      duration: 4200,
      message: `额外环境变量格式错误：${parsedExtra.invalidLine}`,
    })
    return
  }

  const payloadEnv: Record<string, string> = {}
  Object.entries(dnsAccountForm.value.env).forEach(([key, value]) => {
    const normKey = key.trim()
    const normValue = String(value ?? '').trim()
    if (normKey === '' || normValue === '') return
    payloadEnv[normKey] = normValue
  })
  Object.entries(parsedExtra.env).forEach(([key, value]) => {
    payloadEnv[key] = value
  })

  savingDNSAccount.value = true
  try {
    const msg = await HttpUtils.post('api/acme-dns-account-save', {
      id: dnsAccountForm.value.id > 0 ? dnsAccountForm.value.id : undefined,
      name: dnsAccountForm.value.name,
      providerCode: dnsAccountForm.value.providerCode,
      envJson: JSON.stringify(payloadEnv),
      remark: dnsAccountForm.value.remark,
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      dnsAccountFormVisible.value = false
      push.success({
        duration: 3600,
        message: 'DNS 账号已保存',
      })
    }
  } finally {
    savingDNSAccount.value = false
  }
}

const deleteDNSAccount = async (item: AcmeDNSAccount) => {
  const confirmed = window.confirm(`确认删除 DNS 账号「${item.name}」吗？`)
  if (!confirmed) return

  const msg = await HttpUtils.post('api/acme-dns-account-delete', {
    id: item.id,
  })
  if (msg.success) {
    applyActionResult(msg.obj)
    push.success({
      duration: 3200,
      message: 'DNS 账号已删除',
    })
  }
}

const stopPolling = () => {
  if (pollTimer.value != null) {
    window.clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

const startPolling = () => {
  stopPolling()
  if (!props.active) return
  if (shouldPauseOverviewPolling.value) return
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  pollTimer.value = window.setInterval(() => {
    if (shouldPauseOverviewPolling.value) return
    void refreshOverview(true)
  }, 12000)
}

const handleVisibilityChange = () => {
  if (document.visibilityState === 'visible') {
    if (overviewRequestPromise == null) {
      void refreshOverview(true)
    }
    startPolling()
    return
  }
  stopPolling()
}

watch(() => props.active, (value) => {
  if (value) {
    if (!overviewLoaded) {
      void refreshOverview()
    } else if (overviewRequestPromise == null) {
      void refreshOverview(true)
    }
    void refreshSelfSignedAuthorities()
    startPolling()
    return
  }
  stopPolling()
})

watch(shouldPauseOverviewPolling, (paused) => {
  if (paused) {
    stopPolling()
    return
  }
  startPolling()
})

watch(installEmail, (value) => {
  const normalized = normalizeInlineEmail(value)
  if (normalized !== value) {
    installEmail.value = normalized
  }
})

watch(() => issueForm.value.acmeAccountId, (value) => {
  if (isIPCertificateMode.value) return
  if (value <= 0) return
  const account = availableAcmeAccountsByCA.value.find(item => item.id === value)
  if (!account) return
  if (account.server.trim() !== '') {
    issueForm.value.server = normalizeDomainCAValue(account.server, 'letsencrypt')
  }
  if (account.keyLength.trim() !== '') {
    issueForm.value.keyLength = account.keyLength
  }
})

watch(() => issueForm.value.server, (value) => {
  if (isIPCertificateMode.value) return
  const normalizedServer = normalizeDomainCAValue(value, 'letsencrypt')
  if (normalizedServer !== value) {
    issueForm.value.server = normalizedServer
    return
  }
  if (issueForm.value.acmeAccountId <= 0) return
  if (selectedAcmeAccountForIssue.value != null) return
  issueForm.value.acmeAccountId = 0
})

watch(acmeAccountItems, (items) => {
  if (isIPCertificateMode.value) return
  const currentID = issueForm.value.acmeAccountId
  if (currentID <= 0) return
  if (items.some(item => item.value === currentID)) return
  issueForm.value.acmeAccountId = 0
})

watch(
  () => [issueDialogVisible.value, issueLogVisible.value] as const,
  ([dialogVisible, logVisible]) => {
    if (!logVisible) return
    if (dialogVisible) {
      syncIssueLogZIndex()
      return
    }
    issueLogZIndex.value = 2600
  },
)

watch(() => issueForm.value.dnsAccountId, (value) => {
  if (value <= 0) return
  if (isIPCertificateMode.value) return
  const account = overview.value.dnsAccounts.find(item => item.id === value)
  if (!account) return
  issueForm.value.dnsProvider = account.providerCode
})

watch(() => issueForm.value.certificateType, (value) => {
  if (value === 'ip') {
    applyIPCertificateDefaults()
    void refreshIPCertificateOptions()
    void refreshIPPortStatus()
    return
  }
  fillIssueDefaults()
  void refreshIPPortStatus()
})

watch(() => issueForm.value.challenge, (value) => {
  if (value !== 'dns') {
    issueForm.value.dnsAccountId = 0
    issueForm.value.dnsProvider = ''
    issueForm.value.dnsEnv = ''
  }
  if (portChallengeValues.includes(value)) {
    void refreshIPPortStatus()
  }
  if (!isIPCertificateMode.value) return
  if (!['standalone', 'alpn'].includes(value)) {
    issueForm.value.challenge = 'standalone'
  }
})

watch(() => selfSignedForm.value.authorityId, (value) => {
  if (value <= 0) return
  const authority = selfSignedAuthorities.value.find(item => item.id === value)
  if (!authority) return
  if (authority.keyAlgorithm.trim() !== '') {
    selfSignedForm.value.keyAlgorithm = authority.keyAlgorithm
    selfSignedForm.value.signatureAlgorithm = authority.keyAlgorithm
  }
})

watch(() => selfSignedAuthorityForm.value.name, (value) => {
  if (selfSignedAuthorityForm.value.id > 0) return
  if (selfSignedAuthorityForm.value.platformName.trim() === '') {
    selfSignedAuthorityForm.value.platformName = value.trim()
  }
  if (selfSignedAuthorityForm.value.platformCode.trim() === '') {
    selfSignedAuthorityForm.value.platformCode = buildSelfSignedAuthorityPlatformCode(value)
  }
  if (selfSignedAuthorityForm.value.brand.trim() === '') {
    selfSignedAuthorityForm.value.brand = value.trim()
  }
  if (selfSignedAuthorityForm.value.issuerOrg.trim() === '') {
    selfSignedAuthorityForm.value.issuerOrg = selfSignedAuthorityForm.value.organization.trim()
  }
  if (selfSignedAuthorityForm.value.issuerName.trim() === '') {
    selfSignedAuthorityForm.value.issuerName = selfSignedAuthorityForm.value.subjectCn.trim()
  }
})

watch(() => selfSignedAuthorityForm.value.organization, (value) => {
  if (selfSignedAuthorityForm.value.id > 0) return
  if (selfSignedAuthorityForm.value.issuerOrg.trim() === '') {
    selfSignedAuthorityForm.value.issuerOrg = value.trim()
  }
})

watch(() => selfSignedAuthorityForm.value.subjectCn, (value) => {
  if (selfSignedAuthorityForm.value.id > 0) return
  if (selfSignedAuthorityForm.value.issuerName.trim() === '') {
    selfSignedAuthorityForm.value.issuerName = value.trim()
  }
})

watch(() => dnsAccountForm.value.providerCode, (value) => {
  const provider = overview.value.dnsProviders.find(item => item.providerCode === value)
  if (!provider) return

  const nextEnv: Record<string, string> = {}
  provider.fields.forEach((field) => {
    nextEnv[field.key] = dnsAccountForm.value.env[field.key] || ''
  })

  Object.entries(dnsAccountForm.value.env).forEach(([key, val]) => {
    if (Object.hasOwn(nextEnv, key)) return
    nextEnv[key] = val
  })

  dnsAccountForm.value.env = nextEnv
})

onMounted(() => {
  if (props.active) {
    void refreshOverview()
    void refreshSelfSignedAuthorities()
    startPolling()
  }
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', handleVisibilityChange)
  }
})

onBeforeUnmount(() => {
  stopPolling()
  closeIssueLog()
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
  }
})
</script>

<style scoped>
.acme-page {
  min-height: 480px;
}

.acme-hero {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(34, 197, 94, 0.2);
  background:
    radial-gradient(circle at top right, rgba(34, 197, 94, 0.2), transparent 42%),
    linear-gradient(140deg, rgba(15, 23, 42, 0.96), rgba(17, 94, 89, 0.9));
}

.acme-hero__bg {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(90deg, rgba(255, 255, 255, 0.03) 1px, transparent 1px),
    linear-gradient(rgba(255, 255, 255, 0.03) 1px, transparent 1px);
  background-size: 20px 20px;
  mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.9), transparent);
}

.acme-hero__content {
  position: relative;
  z-index: 1;
}

.acme-hero__top {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.acme-hero__icon {
  width: 58px;
  height: 58px;
  border-radius: 18px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #ecfdf5;
  background: linear-gradient(135deg, rgba(34, 197, 94, 0.75), rgba(20, 184, 166, 0.55));
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
}

.acme-hero__eyebrow {
  letter-spacing: 0.2em;
  color: rgba(187, 247, 208, 0.95);
}

.acme-hero__toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.acme-hero__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 16px;
}

.acme-hero__chips :deep(.v-chip) {
  font-weight: 600;
  letter-spacing: 0.02em;
}

.acme-hero-chip {
  min-height: 28px;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
}

.acme-hero-chip--version {
  min-width: 96px;
  background: rgba(59, 130, 246, 0.34) !important;
  color: #eff6ff !important;
}

.acme-hero-chip--ca {
  min-width: 118px;
  background: rgba(20, 184, 166, 0.24) !important;
  color: #ecfeff !important;
}

.acme-hero-chip--version :deep(.v-chip__content),
.acme-hero-chip--ca :deep(.v-chip__content) {
  color: inherit !important;
}

.acme-metric {
  height: 100%;
  padding: 14px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.42);
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.acme-muted {
  color: rgba(226, 232, 240, 0.86);
}

.acme-runtime {
  height: 100%;
}

.acme-runtime__actions {
  display: flex;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 10px;
}

.acme-version-select {
  flex: 1 1 260px;
  min-width: 220px;
}

.acme-runtime__button-group {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.acme-runtime-btn {
  min-width: 112px;
}

.acme-runtime-btn.acme-runtime-btn--check {
  background: rgba(59, 130, 246, 0.08) !important;
  border-color: rgba(96, 165, 250, 0.42) !important;
  color: #dbeafe !important;
}

.acme-runtime-btn.acme-runtime-btn--danger {
  background: rgba(239, 68, 68, 0.08) !important;
  border-color: rgba(248, 113, 113, 0.48) !important;
  color: #fee2e2 !important;
}

.acme-runtime-btn.acme-runtime-btn--check.v-btn--disabled,
.acme-runtime-btn.acme-runtime-btn--danger.v-btn--disabled {
  opacity: 1;
}

.acme-runtime-btn.acme-runtime-btn--check.v-btn--disabled {
  background: rgba(59, 130, 246, 0.12) !important;
  color: #bfdbfe !important;
}

.acme-runtime-btn.acme-runtime-btn--danger.v-btn--disabled {
  background: rgba(239, 68, 68, 0.14) !important;
  color: #fecaca !important;
}

.acme-runtime-btn.acme-runtime-btn--check :deep(.v-btn__content),
.acme-runtime-btn.acme-runtime-btn--danger :deep(.v-btn__content),
.acme-runtime-btn.acme-runtime-btn--check :deep(.v-icon),
.acme-runtime-btn.acme-runtime-btn--danger :deep(.v-icon) {
  opacity: 1;
}

.acme-account-selection {
  width: 100%;
  min-width: 0;
  line-height: 1.2;
}

.acme-account-option__primary {
  font-weight: 600;
  line-height: 1.2;
}

.acme-account-option__secondary {
  margin-top: 1px;
  font-size: 12px;
  line-height: 1.2;
  color: rgba(148, 163, 184, 0.95);
}

.acme-runtime__rows {
  display: grid;
  gap: 10px;
}

.acme-runtime__row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 14px;
  padding-bottom: 8px;
  border-bottom: 1px dashed rgba(148, 163, 184, 0.18);
}

.acme-runtime__row:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.acme-code {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  word-break: break-all;
  text-align: right;
}

.acme-ip-status {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: 12px;
}

.acme-ip-status__item {
  display: flex;
  align-items: center;
  gap: 10px;
  border: 1px solid rgba(var(--v-border-color), 0.18);
  border-radius: 8px;
  padding: 12px;
  background: rgba(var(--v-theme-surface-variant), 0.18);
}

.rotating {
  animation: acme-spin 1s linear infinite;
}

@keyframes acme-spin {
  from {
    transform: rotate(0deg);
  }

  to {
    transform: rotate(360deg);
  }
}

.acme-table-card {
  margin-top: 18px;
}

.acme-table-card__toolbar {
  display: flex;
  justify-content: space-between;
  gap: 14px;
  align-items: center;
  flex-wrap: wrap;
}

.acme-table-card__toolbar-right {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.acme-search {
  min-width: 260px;
}

.acme-count-text {
  color: #fb923c;
  font-size: 14px;
  font-weight: 600;
  white-space: nowrap;
}

.acme-table {
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.acme-id-cell {
  font-family: Consolas, 'Courier New', monospace;
  white-space: nowrap;
}

.acme-table :deep(th) {
  white-space: nowrap;
}

.acme-remark {
  max-width: 220px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.acme-menu-item--disabled {
  opacity: 0.46;
}

.acme-menu-item--disabled :deep(.v-list-item__content),
.acme-menu-item--disabled :deep(.v-list-item__prepend),
.acme-menu-item--disabled :deep(.v-icon) {
  color: rgba(148, 163, 184, 0.7) !important;
}

.acme-menu-item--disabled :deep(.v-list-item__overlay) {
  opacity: 0 !important;
}

.acme-menu-item--disabled {
  cursor: not-allowed;
}

.acme-sub-table {
  border: 1px solid rgba(148, 163, 184, 0.14);
}

.acme-log {
  max-height: 460px;
  overflow: auto;
  padding: 12px;
  background: rgba(15, 23, 42, 0.92);
  border-radius: 12px;
  border: 1px solid rgba(148, 163, 184, 0.22);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

.acme-view-text {
  font-family: Consolas, 'Courier New', monospace;
}

.self-authority-manager {
  min-height: 640px;
}

.self-authority-detail__label {
  width: 240px;
  color: rgb(var(--v-theme-primary));
  font-weight: 600;
  white-space: nowrap;
}

.acme-floating-log {
  position: fixed;
  right: 24px;
  bottom: 24px;
  z-index: 2600;
  width: min(520px, calc(100vw - 32px));
  max-height: min(460px, calc(100vh - 96px));
  overflow: hidden;
  border-radius: 14px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  background: rgb(var(--v-theme-surface));
  box-shadow: 0 18px 54px rgba(0, 0, 0, 0.38);
}

.acme-floating-log__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 12px 10px 14px;
}

.acme-floating-log__body {
  max-height: 360px;
  overflow: auto;
  padding: 12px 14px 14px;
  background: rgba(15, 23, 42, 0.92);
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.55;
  color: rgba(226, 232, 240, 0.96);
}

.acme-floating-log__line {
  white-space: pre-wrap;
  word-break: break-word;
}

@media (max-width: 960px) {
  .acme-hero__toolbar {
    width: 100%;
  }

  .acme-search {
    min-width: 100%;
  }

  .acme-runtime__actions {
    width: 100%;
  }

  .acme-version-select {
    width: 100%;
    min-width: 100%;
  }

  .acme-runtime__button-group {
    width: 100%;
  }

  .acme-floating-log {
    right: 12px;
    bottom: 12px;
    width: calc(100vw - 24px);
  }
}
</style>
