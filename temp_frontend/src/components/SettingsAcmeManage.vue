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
                  class="acme-hero-action"
                  variant="outlined"
                  prepend-icon="mdi-file-certificate-outline"
                  :disabled="!overview.supported || !overview.installed"
                  @click="openIssueDialog">
                  申请证书
                </v-btn>
                <v-btn
                  class="acme-hero-action"
                  variant="outlined"
                  prepend-icon="mdi-shield-lock-outline"
                  @click="openSelfSignedDialog">
                  自签证书
                </v-btn>
                <v-btn
                  class="acme-hero-action"
                  variant="outlined"
                  prepend-icon="mdi-account-circle-outline"
                  @click="acmeAccountDialogVisible = true">
                  ACME 账号
                </v-btn>
                <v-btn
                  class="acme-hero-action"
                  variant="outlined"
                  prepend-icon="mdi-cloud-key-outline"
                  @click="dnsAccountDialogVisible = true">
                  DNS 账号
                </v-btn>
                <v-btn
                  v-if="issueTaskId !== '' && !issueLogVisible"
                  class="acme-hero-action"
                  variant="outlined"
                  prepend-icon="mdi-progress-clock"
                  @click="reopenIssueLog">
                  查看签发任务
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
              <th>推送目录</th>
              <th>备注</th>
              <th class="text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="cert in filteredCertificates" :key="cert.id">
              <td class="acme-id-cell">{{ cert.resourceId || `cert_${cert.displayId || cert.id}` }}</td>
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
                <div>ACME：{{ certificateAccountLabel(cert) }}</div>
                <div class="text-caption text-medium-emphasis mt-1">DNS：{{ certificateDNSAccountLabel(cert) }}</div>
              </td>
              <td>
                <v-chip size="small" :color="statusColor(cert)" variant="flat">{{ statusText(cert) }}</v-chip>
                <div class="text-caption text-error mt-1 acme-wrap-text" v-if="cert.lastError">{{ cert.lastError }}</div>
                <div class="text-caption text-warning mt-1 acme-wrap-text" v-if="cert.postActionError">后置动作警告：{{ cert.postActionError }}</div>
              </td>
              <td>
                <div>{{ formatTimestamp(cert.notAfter) }}</div>
                <div class="text-caption text-medium-emphasis mt-1">{{ expireSummary(cert.notAfter) }}</div>
              </td>
              <td>
                <v-chip size="small" :color="cert.autoRenew ? 'success' : 'grey'" variant="tonal">
                  {{ cert.autoRenew ? '开启' : '关闭' }}
                </v-chip>
                <div class="text-caption font-weight-medium mt-1 acme-auto-renew-state" v-if="autoRenewRetryText(cert)">
                  {{ autoRenewRetryText(cert) }}
                </div>
                <div class="text-caption text-medium-emphasis mt-1 acme-auto-renew-state" v-if="cert.autoRenewNextRetryAt > 0">
                  下次：{{ formatTimestamp(cert.autoRenewNextRetryAt) }}
                </div>
                <div class="text-caption text-medium-emphasis mt-1" v-if="cert.usageLabel">{{ cert.usageLabel }}</div>
              </td>
              <td>
                <v-chip size="small" :color="cert.pushEnabled ? 'success' : 'grey'" variant="tonal">
                  {{ cert.pushEnabled ? '1' : '0' }}
                </v-chip>
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
                    <v-list-item
                      v-if="isAcmeCertificate(cert)"
                      prepend-icon="mdi-pencil-refresh-outline"
                      title="编辑并重新签发"
                      @click="openReissueDialog(cert)" />
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
                      @click="deleteCertificate(cert)" />
                  </v-list>
                </v-menu>
              </td>
            </tr>
            <tr v-if="filteredCertificates.length === 0">
              <td colspan="11" class="text-center text-medium-emphasis py-8">暂无证书记录</td>
            </tr>
          </tbody>
        </v-table>
      </v-card-text>
    </v-card>

    <v-dialog v-model="issueDialogVisible" max-width="1080" content-class="acme-issue-dialog">
      <v-card rounded="xl">
        <v-card-title class="d-flex align-center justify-space-between">
          <div>
            <div class="text-subtitle-1 font-weight-medium">{{ isReissueMode ? '编辑并重新签发' : '申请证书' }}</div>
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
                label="证书类型"
                :disabled="isReissueMode"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-text-field
                v-if="!isIPCertificateMode"
                v-model="issueForm.mainDomain"
                label="主域名"
                placeholder="example.com 或 example.中国"
                hide-details
                @blur="normalizeIssueDomainFields" />
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
                hide-details
                @blur="normalizeIssueDomainFields" />
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
                :disabled="isIPCertificateMode || selectedAcmeAccountForIssue != null"
                :messages="!isIPCertificateMode && selectedAcmeAccountForIssue != null ? '由所选 ACME 账号决定' : []"
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
                clearable
                @click:clear="clearIssueAcmeAccount"
                @update:model-value="normalizeIssueAcmeAccountSelection"
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
                :disabled="issueForm.dnsAccountId > 0"
                :messages="dnsProviderSelectMessages"
                hide-details />
            </v-col>
            <v-col cols="12" v-if="issueForm.challenge === 'dns' && !isIPCertificateMode">
              <v-textarea
                v-model="issueForm.dnsEnv"
                :label="issueForm.dnsAccountId > 0 ? '手工 DNS 环境变量（已选择 DNS 账号时不可叠加）' : '手工 DNS 环境变量（KEY=VALUE 一行一个；签发成功后创建 DNS 账号）'"
                :disabled="issueForm.dnsAccountId > 0"
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
            <v-col cols="12" md="6">
              <v-switch
                v-model="issueForm.autoRenew"
                color="success"
                label="签发后自动续签"
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
          <v-btn variant="text" @click="closeIssueDialog">取消</v-btn>
          <v-btn
            color="primary"
            :loading="issuing"
            :disabled="!canSubmitIssue"
            @click="issueCertificate">
            {{ isReissueMode ? '重新签发' : '开始签发' }}
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
                <th>ID</th>
                <th>名称</th>
                <th>账号密钥算法</th>
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
                <th>ID</th>
                <th>名称</th>
                <th>邮箱</th>
                <th>CA 平台</th>
                <th>账号密钥算法</th>
                <th>备注</th>
                <th>更新时间</th>
                <th class="text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in overview.acmeAccounts" :key="item.id">
                <td class="acme-id-cell">{{ item.resourceId || `acme_${item.displayId || item.id}` }}</td>
                <td>{{ item.name }}</td>
                <td>{{ item.email }}</td>
                <td>{{ caLabel(item.server) }}</td>
                <td>
                  <div>{{ keyLengthLabel(item.accountKeyLength) }}</div>
                  <div class="text-caption text-medium-emphasis mt-1">{{ item.registered ? '已注册' : '待首次签发注册' }}</div>
                </td>
                <td>{{ item.remark || '-' }}</td>
                <td>{{ formatTimestamp(item.updatedAt) }}</td>
                <td class="text-right">
                  <div class="d-flex justify-end ga-2">
                    <v-btn size="small" variant="text" color="primary" icon="mdi-pencil" @click="openAcmeAccountForm(item)" />
                    <v-btn
                      size="small"
                      variant="text"
                      color="primary"
                      icon="mdi-key-change"
                      :disabled="!item.registered"
                      title="轮换账号密钥"
                      @click="openAcmeAccountRotateForm(item)" />
                    <v-btn size="small" variant="text" color="error" icon="mdi-delete" @click="deleteAcmeAccount(item)" />
                  </div>
                </td>
              </tr>
              <tr v-if="overview.acmeAccounts.length === 0">
                <td colspan="8" class="text-center text-medium-emphasis py-6">还没有 ACME 账号</td>
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
              <v-text-field
                v-model="acmeAccountForm.email"
                label="邮箱（ZeroSSL 必填）"
                hint="Let’s Encrypt 可留空；支持用英文逗号分隔多个邮箱"
                persistent-hint />
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="acmeAccountForm.server"
                :items="caServerItems"
                label="CA 平台"
                :disabled="acmeAccountForm.registered"
                hide-details />
            </v-col>
            <v-col cols="12" md="6">
              <v-select
                v-model="acmeAccountForm.accountKeyLength"
                :items="accountKeyLengthItems"
                label="账号密钥算法"
                :disabled="acmeAccountForm.registered"
                hide-details />
            </v-col>
            <v-col cols="12" v-if="acmeAccountForm.registered">
              <v-alert type="info" variant="tonal" density="comfortable">
                已注册账号的 CA 平台和账号密钥算法不能直接修改；如需更换账号密钥，请使用列表中的“轮换账号密钥”。
              </v-alert>
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

    <v-dialog v-model="acmeAccountRotateVisible" max-width="620">
      <v-card rounded="xl" :loading="savingAcmeAccount">
        <v-card-title class="text-subtitle-1 font-weight-medium">轮换 ACME 账号密钥</v-card-title>
        <v-divider />
        <v-card-text>
          <div class="text-body-2 text-medium-emphasis mb-4">
            账号：{{ acmeAccountRotateForm.resourceId }}{{ acmeAccountRotateForm.name ? ` · ${acmeAccountRotateForm.name}` : '' }}
          </div>
          <v-row>
            <v-col cols="12" md="6">
              <v-select
                v-model="acmeAccountRotateForm.accountKeyLength"
                :items="accountKeyLengthItems"
                label="新账号密钥算法"
                hide-details />
            </v-col>
          </v-row>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="acmeAccountRotateVisible = false">取消</v-btn>
          <v-btn color="primary" :loading="savingAcmeAccount" @click="rotateAcmeAccountKey">确认轮换</v-btn>
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
                <th>ID</th>
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
                <td class="acme-id-cell">{{ item.resourceId || `dns_${item.displayId || item.id}` }}</td>
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
                <td colspan="7" class="text-center text-medium-emphasis py-6">还没有 DNS 账号</td>
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
                :disabled="dnsAccountForm.providerLocked"
                hide-details />
            </v-col>
          </v-row>

          <v-alert
            v-if="dnsAccountForm.providerLocked"
            type="warning"
            variant="tonal"
            density="comfortable"
            class="mt-3">
            此 DNS 账号已绑定 ACME 证书，不能变更供应商。可在当前供应商下轮换凭据；如需切换供应商，请新建账号后在“编辑并重新签发”中重新绑定。
          </v-alert>

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

    <v-dialog v-model="pushDialogVisible" max-width="680" width="calc(100vw - 24px)">
      <v-card rounded="xl" class="acme-push-dialog">
        <v-card-title class="text-subtitle-1 font-weight-medium">推送证书到本地目录</v-card-title>
        <v-divider />
        <v-card-text>
          <div class="d-flex align-center justify-space-between ga-2 mb-3">
            <div class="text-body-2 text-medium-emphasis">
              将写入 <code>cert.pem</code>、<code>key.pem</code>、<code>fullchain.pem</code>（以及可用时的 <code>chain.pem</code>）。
            </div>
            <v-btn
              color="error"
              variant="text"
              prepend-icon="mdi-delete-outline"
              :disabled="!pushDialogHasVerifiedPaths"
              @click="requestClearPushDialog">
              删除
            </v-btn>
          </div>
          <v-text-field
            v-model="pushDialogTargetDir"
            label="目标目录"
            placeholder="/etc/ssl/my-app"
            hide-details
            class="acme-push-path-field" />
          <div v-if="pushDialogPathEntries.length > 0" class="acme-push-path-list mt-4">
            <v-text-field
              v-for="entry in pushDialogPathEntries"
              :key="entry.name"
              :model-value="entry.path"
              :label="entry.name"
              readonly
              hide-details
              class="acme-push-path-field" />
          </div>
        </v-card-text>
        <v-divider />
        <v-card-actions>
          <v-spacer />
          <v-btn variant="text" @click="closePushDialog">取消</v-btn>
          <v-btn
            color="primary"
            :loading="pushingId === pushDialogCertId"
            :disabled="pushDialogActionDisabled"
            @click="confirmPushDialog">
            {{ pushDialogActionLabel }}
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
          <div class="d-flex flex-wrap ga-2 mb-4">
            <v-btn size="small" variant="outlined" prepend-icon="mdi-download" :disabled="viewContent.certPem === ''" @click="downloadCertificateMaterial('cert')">下载证书</v-btn>
            <v-btn size="small" variant="outlined" prepend-icon="mdi-download" :disabled="viewContent.keyPem === ''" @click="downloadCertificateMaterial('key')">下载私钥</v-btn>
            <v-btn size="small" variant="outlined" prepend-icon="mdi-download" :disabled="viewContent.fullchainPem === ''" @click="downloadCertificateMaterial('fullchain')">下载完整链</v-btn>
            <v-btn size="small" variant="outlined" prepend-icon="mdi-download" :disabled="viewContent.chainPem === ''" @click="downloadCertificateMaterial('chain')">下载链文件</v-btn>
          </div>
          <v-row>
            <v-col cols="12" md="6">
              <v-textarea
                :model-value="viewContent.certPem"
                label="证书（cert.pem）"
                rows="10"
                auto-grow
                readonly
                variant="outlined"
                class="acme-view-text" />
            </v-col>
            <v-col cols="12" md="6">
              <v-textarea
                :model-value="viewContent.fullchainPem"
                label="公钥（fullchain.pem）"
                rows="10"
                auto-grow
                readonly
                variant="outlined"
                class="acme-view-text" />
            </v-col>
            <v-col cols="12" md="6">
              <v-textarea
                :model-value="viewContent.keyPem"
                label="私钥（key.pem）"
                rows="10"
                auto-grow
                readonly
                variant="outlined"
                class="acme-view-text" />
            </v-col>
            <v-col cols="12" md="6">
              <v-textarea
                :model-value="viewContent.chainPem"
                label="链文件（chain.pem）"
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
          <div class="text-subtitle-2 font-weight-medium">证书任务日志</div>
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
import { confirm } from '@/plugins/confirm'
import { i18n } from '@/locales'
import { formatPanelDateTime, panelNow } from '@/plugins/panelTime'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { push } from 'notivue'

const acmeInstallRequestTimeout = 2 * 60 * 1000
const acmeRemoveRequestTimeout = 90 * 1000
const acmeIssueRequestTimeout = 3 * 60 * 1000 + 30 * 1000
const confirmAction = (action: string) => i18n.global.t(`confirmDialog.actions.${action}`)

type AcmeCertificate = {
  id: number
  displayId: number
  resourceId: string
  sourceType: string
  sourceRef: string
  mainDomain: string
  domains: string[]
  certificateType: string
  challenge: string
  keyLength: string
  issuedKeyAlgorithm: string
  issuedSignatureAlgorithm: string
  caServer: string
  useEcc: boolean
  autoRenew: boolean
  autoRenewRetryPhase: string
  autoRenewRetryCount: number
  autoRenewNextRetryAt: number
  autoRenewLastAttemptAt: number
  acmeAccountId: number
  acmeAccountName: string
  dnsAccountId: number
  dnsAccountName: string
  applyTarget: string
  pushEnabled: boolean
  pushDir: string
  pushFilePaths: Record<string, string>
  remark: string
  webroot: string
  dnsProvider: string
  customArgs: string
  fingerprint: string
  notBefore: number
  notAfter: number
  lastIssuedAt: number
  lastRenewedAt: number
  updatedAt: number
  createdAt: number
  lastError: string
  postActionError: string
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
  certPem: string
  fullchainPem: string
  keyPem: string
  chainPem: string
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
  displayId: number
  resourceId: string
  name: string
  email: string
  server: string
  accountKeyLength: string
  registered: boolean
  remark: string
  createdAt: number
  updatedAt: number
}

type AcmeDNSAccount = {
  id: number
  displayId: number
  resourceId: string
  name: string
  providerName: string
  providerCode: string
  providerLocked: boolean
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
  warnings?: string[]
}

type AcmeTask = {
  id: string
  operation: string
  status: string
  logSessionId: string
  startedAt: number
  updatedAt: number
  finishedAt?: number
  error?: string
  warnings?: string[]
  result?: AcmeActionResult
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
  taskId?: string
  taskStatus?: string
  warnings?: string[]
  result?: AcmeActionResult
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
const reissuingCertificateId = ref(0)
const selfSignedDialogVisible = ref(false)
const selfSignedAuthorityManagerVisible = ref(false)
const selfSignedAuthorityFormVisible = ref(false)
const selfSignedAuthorityDetailVisible = ref(false)
const acmeAccountDialogVisible = ref(false)
const acmeAccountFormVisible = ref(false)
const acmeAccountRotateVisible = ref(false)
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
  certPem: '',
  fullchainPem: '',
  keyPem: '',
  chainPem: '',
  issuedKeyAlgorithm: '',
  issuedSignatureAlgorithm: '',
})

const pushDialogCertId = ref(0)
const pushDialogTargetDir = ref('')
const pushDialogFilePaths = ref<Record<string, string>>({})
const pushDialogClearRequested = ref(false)
const logCertId = ref(0)

const certificatePushFileOrder = ['cert.pem', 'key.pem', 'fullchain.pem', 'chain.pem'] as const
const pushDialogPathEntries = computed(() => certificatePushFileOrder
  .map(name => ({ name, path: pushDialogFilePaths.value[name] || '' }))
  .filter(entry => entry.path !== ''))
const pushDialogHasVerifiedPaths = computed(() => pushDialogPathEntries.value.length > 0)
const pushDialogClearMode = computed(() => (
  pushDialogClearRequested.value
  && pushDialogTargetDir.value.trim() === ''
  && pushDialogPathEntries.value.length === 0
))
const pushDialogActionLabel = computed(() => pushDialogClearMode.value ? '保存' : '推送')
const pushDialogActionDisabled = computed(() => {
  if (pushDialogCertId.value === 0) return true
  if (pushDialogClearMode.value) return false
  return pushDialogTargetDir.value.trim() === ''
})

const pollTimer = ref<number | null>(null)
const issueLogTimer = ref<number | null>(null)
const issueLogVisible = ref(false)
const issueTaskId = ref('')
const issueLogSessionId = ref('')
const issueLogStatus = ref('idle')
const issueLogLines = ref<string[]>([])
const issueLogBodyRef = ref<HTMLElement | null>(null)
const issueLogZIndex = ref(2600)
let overviewRequestPromise: Promise<void> | null = null
let selfSignedAuthoritiesRequestPromise: Promise<void> | null = null
let issueLogRequestPromise: Promise<void> | null = null
let completedIssueTaskId = ''
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
  autoRenew: true,
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
  accountKeyLength: 'ec-256',
  registered: false,
  remark: '',
})

const acmeAccountRotateForm = ref({
  id: 0,
  resourceId: '',
  name: '',
  accountKeyLength: 'ec-256',
})

const dnsAccountForm = ref({
  id: 0,
  name: '',
  providerCode: '',
  providerLocked: false,
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
  { title: 'EC-521', value: 'ec-521' },
  { title: 'RSA-2048', value: '2048' },
  { title: 'RSA-3072', value: '3072' },
  { title: 'RSA-4096', value: '4096' },
  { title: 'RSA-8192', value: '8192' },
]

const accountKeyLengthItems = [...keyLengthItems]

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

const normalizePushFilePaths = (value: unknown): Record<string, string> => {
  if (value == null || typeof value !== 'object' || Array.isArray(value)) return {}
  const raw = value as Record<string, unknown>
  const result: Record<string, string> = {}
  certificatePushFileOrder.forEach((name) => {
    const filePath = asString(raw[name]).trim()
    if (filePath !== '') result[name] = filePath
  })
  return result
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
      displayId: asNumber(item.displayId),
      resourceId: asString(item.resourceId),
      name: asString(item.name),
      email: asString(item.email),
      server: asString(item.server),
      accountKeyLength: asString(item.accountKeyLength, asString((item as any).keyLength, 'ec-256')),
      registered: asBoolean(item.registered),
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
      displayId: asNumber(item.displayId),
      resourceId: asString(item.resourceId),
      name: asString(item.name),
      providerName: asString(item.providerName),
      providerCode: asString(item.providerCode),
      providerLocked: asBoolean(item.providerLocked),
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
      resourceId: asString((item as any).resourceId),
      sourceType: asString((item as any).sourceType),
      sourceRef: asString((item as any).sourceRef),
      mainDomain: asString(item.mainDomain),
      domains,
      certificateType: asString((item as any).certificateType, 'domain'),
      challenge: asString(item.challenge),
      keyLength: asString(item.keyLength),
      issuedKeyAlgorithm: asString((item as any).issuedKeyAlgorithm),
      issuedSignatureAlgorithm: asString((item as any).issuedSignatureAlgorithm),
      caServer: asString(item.caServer),
      useEcc: asBoolean(item.useEcc),
      autoRenew: asBoolean(item.autoRenew, false),
      autoRenewRetryPhase: asString((item as any).autoRenewRetryPhase),
      autoRenewRetryCount: asNumber((item as any).autoRenewRetryCount),
      autoRenewNextRetryAt: asNumber((item as any).autoRenewNextRetryAt),
      autoRenewLastAttemptAt: asNumber((item as any).autoRenewLastAttemptAt),
      acmeAccountId: asNumber(item.acmeAccountId),
      acmeAccountName: asString(item.acmeAccountName),
      dnsAccountId: asNumber(item.dnsAccountId),
      dnsAccountName: asString(item.dnsAccountName),
      applyTarget: asString(item.applyTarget),
      pushEnabled: asBoolean((item as any).pushEnabled),
      pushDir: asString(item.pushDir),
      pushFilePaths: normalizePushFilePaths((item as any).pushFilePaths),
      remark: asString(item.remark),
      webroot: asString((item as any).webroot),
      dnsProvider: asString((item as any).dnsProvider),
      customArgs: asString((item as any).customArgs),
      fingerprint: asString(item.fingerprint),
      notBefore: asNumber(item.notBefore),
      notAfter: asNumber(item.notAfter),
      lastIssuedAt: asNumber(item.lastIssuedAt),
      lastRenewedAt: asNumber(item.lastRenewedAt),
      updatedAt: asNumber(item.updatedAt),
      createdAt: asNumber(item.createdAt),
      lastError: asString(item.lastError),
      postActionError: asString((item as any).postActionError),
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

const normalizeAcmeContactEmails = (value: string): string => {
  return normalizeInlineEmail(value)
    .split(',')
    .map(item => item.trim())
    .filter(item => item !== '')
    .join(',')
}

const isLikelyValidAcmeContactEmails = (value: string, required: boolean): boolean => {
  const normalized = normalizeAcmeContactEmails(value)
  if (normalized === '') return !required
  return normalized.split(',').every(item => isLikelyValidAcmeEmail(item))
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

const splitDomainInputTokens = (value: string): string[] => {
  return String(value ?? '')
    .replace(/\r\n/g, '\n')
    .replace(/\r/g, '\n')
    .replace(/,/g, ' ')
    .split(/\s+/)
    .map(item => item.trim())
    .filter(item => item !== '')
}

const normalizeStrictAcmeDomainToken = (raw: string): { value: string; error: string } => {
  const original = raw.trim()
  if (original === '') return { value: '', error: '域名不能为空' }
  if (/[\\/@:]/.test(original)) return { value: '', error: `域名格式无效：${original}` }

  let wildcard = false
  let host = original
  if (host.startsWith('*.')) {
    wildcard = true
    host = host.slice(2)
  }
  if (host.includes('*')) return { value: '', error: `通配符只能位于域名最前方：${original}` }
  host = host.replace(/\.$/, '')
  if (host === '' || host.startsWith('.') || host.endsWith('.')) {
    return { value: '', error: `域名格式无效：${original}` }
  }

  let ascii = ''
  try {
    ascii = new URL(`http://${host}`).hostname.toLowerCase()
  } catch {
    return { value: '', error: `国际化域名格式无效：${original}` }
  }
  if (ascii === '' || ascii.length > 253 || /^[0-9]+(?:\.[0-9]+){3}$/.test(ascii) || ascii.includes(':')) {
    return { value: '', error: `域名证书不能混入 IP 地址：${original}` }
  }
  const labels = ascii.split('.')
  if (labels.length < 2 || labels.some(label => !/^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(label))) {
    return { value: '', error: `域名标签无效：${original}` }
  }
  return { value: wildcard ? `*.${ascii}` : ascii, error: '' }
}

const validateIssueDomainInput = (): { domains: string[]; error: string } => {
  const tokens = splitDomainInputTokens(`${issueForm.value.mainDomain}\n${issueForm.value.extraDomains}`)
  const seen = new Set<string>()
  const domains: string[] = []
  for (const token of tokens) {
    const normalized = normalizeStrictAcmeDomainToken(token)
    if (normalized.error !== '') return { domains, error: normalized.error }
    if (seen.has(normalized.value)) continue
    seen.add(normalized.value)
    domains.push(normalized.value)
  }
  if (domains.length > 100) return { domains, error: '域名证书最多支持 100 个域名' }
  return { domains, error: '' }
}

const normalizeIssueDomainFields = () => {
  if (isIPCertificateMode.value) return
  const seen = new Set<string>()
  const values = splitDomainInputTokens(`${issueForm.value.mainDomain}\n${issueForm.value.extraDomains}`)
    .map(value => value.replace(/^\.+|\.+$/g, '').trim())
    .filter(value => {
      const key = value.toLowerCase()
      if (key === '' || seen.has(key)) return false
      seen.add(key)
      return true
    })
  issueForm.value.mainDomain = values.shift() || ''
  issueForm.value.extraDomains = values.join(', ')
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
  return validateIssueDomainInput().domains
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

const issueDomainValidationError = computed(() => {
  if (isIPCertificateMode.value) return ''
  return validateIssueDomainInput().error
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

const acmePortCheckHint = '检查 80/443 的 TCP 占用，临时防火墙仅放行最终选中的 TCP 验证端口'
const acmePortFallbackHint = '80 优先、443 兜底'

const issuePreviewText = computed(() => {
  const preview = issueDomainPreview.value
  if (!isIPCertificateMode.value) {
    if (issueDomainValidationError.value !== '') {
      return issueDomainValidationError.value
    }
    if (acmeAccountItems.value.length === 0) {
      return acmeAccountNoDataText.value
    }
    if (issueForm.value.acmeAccountId <= 0) {
      return '签发域名证书前请选择 ACME 账号。'
    }
    if (preview.split(', ').some(domain => domain.startsWith('*.')) && issueForm.value.challenge !== 'dns') {
      return '通配符域名只能使用 DNS 验证。'
    }
    if (issueForm.value.challenge === 'dns') {
      return `准备使用 DNS 验证签发域名证书：${preview || '请填写主域名'}。`
    }
    if (issueForm.value.challenge === 'webroot') {
      return selectedIPPortItem.value?.message
        || `准备使用 HTTP Webroot 签发域名证书：${preview || '请填写主域名'}。${acmePortCheckHint}。`
    }
    if (!hasAnyPortChallengeAvailable.value) {
      return `80/443 没有可用的验证端口组合。请释放冲突端口后重试。`
    }
    const selected = selectedIPPortItem.value
    const recommended = recommendedPortChallengeItem.value
    if (selected != null && !selected.available && recommended != null && recommended.challenge !== selected.challenge) {
      return `当前 ${ipChallengeTitle(selected.challenge)} 不可用，任务将自动切换为 ${ipChallengeTitle(recommended.challenge)}（${acmePortFallbackHint}）。`
    }
    return `准备使用 ${ipChallengeTitle(issueForm.value.challenge)} 签发域名证书：${preview || '请填写主域名'}。${acmePortCheckHint}。`
  }

  if (!hasAnyPortChallengeAvailable.value) {
    return '80/443 没有可用的验证端口组合，无法继续签发 IP 证书。'
  }
  if (!selectedPortChallengeAvailable.value) {
    const selected = selectedIPPortItem.value
    const recommended = recommendedPortChallengeItem.value
    if (recommended != null) {
      return `当前 ${ipChallengeTitle(selected?.challenge || issueForm.value.challenge)} 不可用，任务将自动切换为 ${ipChallengeTitle(recommended.challenge)}（${acmePortFallbackHint}）。`
    }
    return `当前验证方式不可用：${selected?.message || '端口已占用'}`
  }
  if (issueIPFamilyMode.value === 'ipv6') {
    return `准备为 ${preview || '请选择或输入公网 IP'} 签发 IP 证书，将使用 IPv6 监听模式。`
  }
  if (issueIPFamilyMode.value === 'dual') {
    return `准备为 ${preview || '请选择或输入公网 IP'} 签发 IP 证书。检测到双栈，请确认 IPv4/IPv6 均可从外部访问。`
  }
  return `准备为 ${preview || '请选择或输入公网 IP'} 签发 IP 证书。`
})

const canSubmitIssue = computed(() => {
  if (!overview.value.supported || !overview.value.installed) return false
  const domains = buildIssueDomains()
  if (domains.length === 0) return false
  if (isIPCertificateMode.value) {
    if (domains.length > 100) return false
    if (!['standalone', 'alpn'].includes(issueForm.value.challenge)) return false
  } else {
    if (issueDomainValidationError.value !== '') return false
    if (selectedAcmeAccountForIssue.value == null) return false
    if (issueForm.value.challenge === 'webroot' && issueForm.value.webroot.trim() === '') return false
    if (issueForm.value.challenge === 'dns' && issueForm.value.dnsProvider.trim() === '') return false
    if (buildIssueDomains().some(domain => domain.startsWith('*.')) && issueForm.value.challenge !== 'dns') return false
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

const availableAcmeAccountsByCA = computed(() => {
  if (isIPCertificateMode.value) return [] as AcmeAccount[]
  return overview.value.acmeAccounts
})

const selectedAcmeAccountForIssue = computed(() => {
  if (isIPCertificateMode.value) return null
  if (issueForm.value.acmeAccountId <= 0) return null
  return availableAcmeAccountsByCA.value.find(item => item.id === issueForm.value.acmeAccountId) ?? null
})

const selectedDNSAccountForIssue = computed(() => {
  if (isIPCertificateMode.value || issueForm.value.dnsAccountId <= 0) return null
  return overview.value.dnsAccounts.find(item => item.id === issueForm.value.dnsAccountId) ?? null
})

const issueDomainCAServer = computed(() => {
  const selected = selectedAcmeAccountForIssue.value
  if (selected != null) return normalizeDomainCAValue(selected.server, 'letsencrypt')
  return normalizeDomainCAValue(issueForm.value.server, 'letsencrypt')
})

const acmeAccountItems = computed(() => {
  return availableAcmeAccountsByCA.value.map(item => ({
    nameText: item.name.trim() === '' ? item.resourceId || `acme_${item.displayId || item.id}` : item.name.trim(),
    value: item.id,
    metaText: `${item.resourceId || `acme_${item.displayId || item.id}`} · CA：${caLabel(item.server)} · 邮箱：${item.email || '未设置'}`,
  }))
})

const acmeAccountNoDataText = computed(() => {
  return '暂无 ACME 账号，请先在 ACME 账号管理中创建'
})

const acmeAccountSelectMessages = computed(() => {
  if (isIPCertificateMode.value) return [] as string[]
  if (acmeAccountItems.value.length > 0) return [] as string[]
  return [acmeAccountNoDataText.value]
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
    title: `${item.resourceId || `dns_${item.displayId || item.id}`} · ${item.name} (${item.providerCode})`,
    value: item.id,
  }))
  return [...base, ...items]
})

const dnsProviderSelectMessages = computed(() => {
  const account = selectedDNSAccountForIssue.value
  if (account == null) return [] as string[]
  return [`已绑定 DNS 账号「${account.name || account.resourceId}」，Provider 固定为 ${account.providerCode}`]
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
    || acmeAccountRotateVisible.value
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
      item.resourceId,
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
  if (cert.postActionError.trim() !== '') {
    parts.push(`后置动作警告:\n${cert.postActionError.trim()}`)
  }
  if (cert.lastOutput.trim() !== '') {
    parts.push(`最近输出:\n${cert.lastOutput.trim()}`)
  }
  if (parts.length === 0) return '暂无输出日志'
  return parts.join('\n\n')
})

const issueLogStatusText = computed(() => {
  switch (issueLogStatus.value) {
    case 'queued':
      return '排队等待执行'
    case 'running':
      return '正在执行'
    case 'success':
      return '已完成'
    case 'warning':
      return '已完成，存在后置动作警告'
    case 'error':
      return '执行失败'
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
  const requiresEmail = normalizeDomainCAValue(acmeAccountForm.value.server, 'letsencrypt') === 'zerossl'
  return acmeAccountForm.value.name.trim() !== ''
    && isLikelyValidAcmeContactEmails(acmeAccountForm.value.email, requiresEmail)
})

const isReissueMode = computed(() => reissuingCertificateId.value > 0)

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
    const email = dnsEnvFieldValue('CF_Email').trim()
    const key = dnsEnvFieldValue('CF_Key').trim()
    const tokenMode = token !== ''
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
  if (normalized === 'ec-521') return 'EC-521'
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
  return formatPanelDateTime(unixTs * 1000)
}

const autoRenewRetryText = (cert: AcmeCertificate): string => {
  switch (cert.autoRenewRetryPhase.trim()) {
    case 'rapid_retry':
      return `快速重试 ${Math.min(Math.max(cert.autoRenewRetryCount + 1, 1), 3)}/3`
    case 'periodic_retry':
      return '等待 6 小时重试'
    case 'expired_disabled':
      return '到期后已停用'
    default:
      return ''
  }
}

const expireSummary = (unixTs: number): string => {
  if (!Number.isFinite(unixTs) || unixTs <= 0) return '未知到期时间'
  const now = panelNow().getTime()
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
    const remainDays = Math.floor((cert.notAfter * 1000 - panelNow().getTime()) / (24 * 3600 * 1000))
    if (remainDays <= 7) return '即将到期'
  }
  return '正常'
}

const statusColor = (cert: AcmeCertificate): string => {
  if (cert.status === 'expired') return 'error'
  if (cert.status === 'error') return 'warning'
  if (cert.notAfter > 0) {
    const remainDays = Math.floor((cert.notAfter * 1000 - panelNow().getTime()) / (24 * 3600 * 1000))
    if (remainDays <= 7) return 'warning'
  }
  return 'success'
}

const isAcmeCertificate = (cert: AcmeCertificate): boolean => {
  return cert.sourceType === 'acme'
}

const certificateAccountLabel = (cert: AcmeCertificate): string => {
  if (cert.certificateType === 'ip') return '系统 ip_acme'
  if (cert.acmeAccountName.trim() !== '') return cert.acmeAccountName
  return cert.acmeAccountId > 0 ? '已删除账号' : '待重新绑定'
}

const certificateDNSAccountLabel = (cert: AcmeCertificate): string => {
  if (cert.challenge !== 'dns') return '-'
  if (cert.dnsAccountName.trim() !== '') return cert.dnsAccountName
  return cert.dnsAccountId > 0 ? '已删除账号' : '待重新绑定'
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
  issueForm.value.autoRenew = true
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

const clearIssueAcmeAccount = () => {
  // Clearing the selection intentionally leaves the current CA choice intact
  // so the user can choose another account or inspect/edit the CA field.
  issueForm.value.acmeAccountId = 0
}

const normalizeIssueAcmeAccountSelection = (value: unknown) => {
  const id = asNumber(value)
  issueForm.value.acmeAccountId = id > 0 ? id : 0
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

const startIssueLogPolling = () => {
  stopIssueLogPolling()
  if (!issueLogVisible.value || issueLogSessionId.value === '') return
  if (typeof document !== 'undefined' && document.visibilityState !== 'visible') return
  issueLogTimer.value = window.setInterval(() => {
    void pollIssueLog()
  }, 1000)
  void pollIssueLog()
}

const scrollIssueLogToBottom = () => {
  void nextTick(() => {
    const el = issueLogBodyRef.value
    if (el == null) return
    el.scrollTop = el.scrollHeight
  })
}

const closeIssueLog = () => {
  // Closing the viewer never cancels the background ACME task. The active
  // task identifier remains in session storage and is restored on re-entry.
  issueLogVisible.value = false
  stopIssueLogPolling()
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
    taskId: asString(data.taskId),
    taskStatus: asString(data.taskStatus),
    warnings: Array.isArray(data.warnings)
      ? data.warnings.map(item => String(item ?? '').trim()).filter(item => item !== '')
      : [],
    result: data.result,
    startedAt: asNumber(data.startedAt),
    updatedAt: asNumber(data.updatedAt),
    finishedAt: asNumber(data.finishedAt),
  }
}

const normalizeAcmeTask = (raw: unknown): AcmeTask | null => {
  const data = (raw ?? {}) as Partial<AcmeTask>
  const id = asString(data.id).trim()
  const logSessionId = asString(data.logSessionId).trim()
  if (id === '' || logSessionId === '') return null
  return {
    id,
    operation: asString(data.operation),
    status: asString(data.status, 'queued'),
    logSessionId,
    startedAt: asNumber(data.startedAt),
    updatedAt: asNumber(data.updatedAt),
    finishedAt: asNumber(data.finishedAt),
    error: asString(data.error),
    warnings: Array.isArray(data.warnings)
      ? data.warnings.map(item => String(item ?? '').trim()).filter(item => item !== '')
      : [],
    result: data.result,
  }
}

const activeAcmeTaskStorageKey = 'kwor-acme-active-task'
const isTerminalAcmeTaskStatus = (status: string): boolean => ['success', 'warning', 'error', 'missing'].includes(status)

const rememberActiveAcmeTask = (task: AcmeTask) => {
  issueTaskId.value = task.id
  issueLogSessionId.value = task.logSessionId
  try {
    window.sessionStorage.setItem(activeAcmeTaskStorageKey, task.id)
  } catch {
    // Storage is optional; the server-side active-task query still works.
  }
}

const clearRememberedActiveAcmeTask = () => {
  try {
    window.sessionStorage.removeItem(activeAcmeTaskStorageKey)
  } catch {
    // Ignore unavailable storage.
  }
}

const openIssueTaskLog = (task: AcmeTask) => {
  stopIssueLogPolling()
  rememberActiveAcmeTask(task)
  issueLogStatus.value = task.status
  issueLogLines.value = task.status === 'queued'
    ? ['后台任务已排队，等待开始执行...']
    : ['后台任务已创建，正在读取日志...']
  issueLogVisible.value = true
  syncIssueLogZIndex()
  scrollIssueLogToBottom()
  startIssueLogPolling()
}

const reopenIssueLog = () => {
  if (issueLogSessionId.value === '') {
    void restoreActiveAcmeTask()
    return
  }
  issueLogVisible.value = true
  syncIssueLogZIndex()
  startIssueLogPolling()
}

const restoreActiveAcmeTask = async () => {
  if (!props.active) return
  let storedID = ''
  try {
    storedID = window.sessionStorage.getItem(activeAcmeTaskStorageKey) || ''
  } catch {
    storedID = ''
  }
  if (storedID !== '') {
    const storedMsg = await HttpUtils.get('api/acme-task', { id: storedID })
    if (storedMsg.success) {
      const storedTask = normalizeAcmeTask(storedMsg.obj)
      if (storedTask != null) {
        openIssueTaskLog(storedTask)
        return
      }
    }
    clearRememberedActiveAcmeTask()
  }

  const msg = await HttpUtils.get('api/acme-active-tasks')
  if (!msg.success || !Array.isArray(msg.obj)) return
  const tasks = msg.obj.map(normalizeAcmeTask).filter((item): item is AcmeTask => item != null)
  if (tasks.length === 0) return
  openIssueTaskLog(tasks[0])
}

const pollIssueLog = async (): Promise<void> => {
  if (issueLogRequestPromise) return issueLogRequestPromise
  const sessionID = issueLogSessionId.value
  if (sessionID === '') return
  const request = (async () => {
    const msg = await HttpUtils.get('api/acme-log', { id: sessionID })
    if (!msg.success || sessionID !== issueLogSessionId.value) return

    const session = normalizeAcmeLogSession(msg.obj)
    const effectiveStatus = session.taskStatus || session.status
    issueLogStatus.value = effectiveStatus
    issueLogLines.value = session.lines.length > 0 ? session.lines : ['等待后端开始写入日志...']
    if (session.taskId) issueTaskId.value = session.taskId
    scrollIssueLogToBottom()

    if (isTerminalAcmeTaskStatus(effectiveStatus)) {
      stopIssueLogPolling()
      clearRememberedActiveAcmeTask()
      const completedTaskID = session.taskId || issueTaskId.value || sessionID
      if (completedIssueTaskId !== completedTaskID) {
        completedIssueTaskId = completedTaskID
        if (session.result) {
          applyActionResult(session.result)
        } else {
          void refreshOverview(true)
        }
        if (effectiveStatus === 'warning' && (session.warnings?.length ?? 0) > 0) {
          push.warning({ duration: 5200, message: session.warnings?.[0] || '证书签发后的部分动作需要处理' })
        } else if (effectiveStatus === 'error' && session.error) {
          push.error({ duration: 5200, message: session.error })
        }
      }
    }
  })()
  issueLogRequestPromise = request
  try {
    await request
  } finally {
    if (issueLogRequestPromise === request) {
      issueLogRequestPromise = null
    }
  }
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
    }, { timeout: acmeInstallRequestTimeout })
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
  if (!(await confirm({
    message: '确认删除已下载的 acme.sh 吗？仅删除受管 acme，不会删除证书与用户自放文件。',
    severity: 'danger',
    confirmText: confirmAction('delete'),
  }))) {
    return
  }
  removingAcme.value = true
  try {
    const msg = await HttpUtils.post('api/acme-remove', {
      removeCertificates: false,
    }, { timeout: acmeRemoveRequestTimeout })
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
  reissuingCertificateId.value = 0
  clearIssueForm()
  issueDialogVisible.value = true
  void refreshIPCertificateOptions()
  void refreshIPPortStatus()
}

const closeIssueDialog = () => {
  issueDialogVisible.value = false
  reissuingCertificateId.value = 0
  clearIssueForm()
}

const openReissueDialog = (cert: AcmeCertificate) => {
  if (!isAcmeCertificate(cert)) return
  const certificateType = cert.certificateType === 'ip' ? 'ip' : 'domain'
  const domains = cert.domains.length > 0 ? cert.domains : [cert.mainDomain]
  const mainDomain = domains[0] || cert.mainDomain
  const selectedAccount = overview.value.acmeAccounts.find(item => item.id === cert.acmeAccountId)
  issueForm.value = {
    certificateType,
    mainDomain: certificateType === 'domain' ? mainDomain : '',
    extraDomains: certificateType === 'domain' ? domains.slice(1).join(', ') : '',
    ipAddresses: certificateType === 'ip' ? domains : [],
    challenge: cert.challenge || (certificateType === 'ip' ? 'standalone' : 'dns'),
    webroot: cert.webroot || '',
    dnsProvider: cert.dnsProvider || '',
    dnsAccountId: certificateType === 'domain' && cert.challenge === 'dns' ? cert.dnsAccountId : 0,
    dnsEnv: '',
    server: normalizeDomainCAValue(selectedAccount?.server || cert.caServer, 'letsencrypt'),
    keyLength: cert.keyLength || 'ec-256',
    customArgs: cert.customArgs || '',
    acmeAccountId: certificateType === 'domain' ? cert.acmeAccountId : 0,
    autoRenew: cert.autoRenew,
    remark: cert.remark || '',
    applyTarget: cert.applyTarget || '',
    pushDir: cert.pushEnabled ? cert.pushDir || '' : '',
  }
  reissuingCertificateId.value = cert.id
  issueDialogVisible.value = true
  if (certificateType === 'ip') {
    void refreshIPCertificateOptions()
  }
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
  const confirmed = await confirm({
    message: `确认删除平台「${item.name}」吗？`,
    severity: 'danger',
    confirmText: confirmAction('delete'),
  })
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
    const requiresEmail = normalizeDomainCAValue(account.server, 'letsencrypt') === 'zerossl'
    if (!isLikelyValidAcmeContactEmails(account.email, requiresEmail)) {
      push.warning({
        duration: 4200,
        message: requiresEmail
          ? '所选 ZeroSSL ACME 账号必须填写有效邮箱，请先在 ACME 账号管理中修正'
          : '所选 ACME 账号邮箱格式无效，请先在 ACME 账号管理中修正',
      })
      return
    }
  }

  normalizeIssueDomainFields()
  if (!isIPCertificateMode.value && issueDomainValidationError.value !== '') {
    push.warning({
      duration: 4600,
      message: issueDomainValidationError.value,
    })
    return
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
  if (!isIPCertificateMode.value && domains.some(domain => domain.startsWith('*.')) && issueForm.value.challenge !== 'dns') {
    push.warning({
      duration: 4600,
      message: '通配符域名只能使用 DNS 验证。',
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
  const reissue = isReissueMode.value
  const isDNSChallenge = !isIPCertificateMode.value && issueForm.value.challenge === 'dns'
  try {
    const msg = await HttpUtils.post(reissue ? 'api/acme-reissue-task' : 'api/acme-issue-task', {
      existingRecordId: reissue ? reissuingCertificateId.value : undefined,
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
      autoRenew: issueForm.value.autoRenew,
      remark: issueForm.value.remark,
      applyTarget: issueForm.value.applyTarget,
      pushDir: issueForm.value.pushDir,
    })
    if (msg.success) {
      const task = normalizeAcmeTask(msg.obj)
      if (task == null) {
        push.error({ duration: 4200, message: '后台签发任务返回无效，请刷新页面后查看证书库存。' })
        return
      }
      openIssueTaskLog(task)
      closeIssueDialog()
      push.success({
        duration: 3600,
        message: reissue ? '重新签发任务已创建' : '签发任务已创建',
      })
    }
  } finally {
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
    const confirmed = await confirm({
      message: `确认强制续签证书 ${cert.mainDomain} 吗？`,
      severity: 'warning',
      confirmText: confirmAction('renew'),
    })
    if (!confirmed) return
  }
  rowBusyId.value = cert.id
  try {
    const msg = await HttpUtils.post('api/acme-renew-task', {
      id: cert.id,
      force,
    })
    if (msg.success) {
      const task = normalizeAcmeTask(msg.obj)
      if (task == null) {
        push.error({ duration: 4200, message: '后台续签任务返回无效，请刷新页面后查看证书库存。' })
        return
      }
      openIssueTaskLog(task)
      push.success({
        duration: 3500,
        message: force ? '强制续签任务已创建' : '续签任务已创建',
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
    const confirmed = await confirm({
      message: ['确认取消应用到', targetLabel, '吗？'].join(''),
      severity: 'warning',
      confirmText: confirmAction('cancelApply'),
    })
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
  const bindingHint = cert.usageLabel.trim() === ''
    ? ''
    : `\n同时解除以下引用：${cert.usageLabel}`
  const confirmed = await confirm({
    message: `确认删除证书 ${cert.mainDomain} 吗？删除后会保留 ACME/DNS 账号。${bindingHint}`,
    severity: 'danger',
    confirmText: confirmAction('delete'),
  })
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
  pushDialogTargetDir.value = cert.pushEnabled ? cert.pushDir || '' : ''
  pushDialogFilePaths.value = cert.pushEnabled ? { ...cert.pushFilePaths } : {}
  pushDialogClearRequested.value = false
  pushDialogVisible.value = true
}

const closePushDialog = () => {
  pushDialogVisible.value = false
  pushDialogCertId.value = 0
  pushDialogTargetDir.value = ''
  pushDialogFilePaths.value = {}
  pushDialogClearRequested.value = false
}

const requestClearPushDialog = async () => {
  if (!pushDialogHasVerifiedPaths.value) return
  const confirmed = await confirm({
    message: '确认删除已推送的证书文件吗？目标目录及其其他文件会保留。',
    severity: 'danger',
    confirmText: confirmAction('delete'),
  })
  if (!confirmed) return
  pushDialogTargetDir.value = ''
  pushDialogFilePaths.value = {}
  pushDialogClearRequested.value = true
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
    certPem: '',
    fullchainPem: '',
    keyPem: '',
    chainPem: '',
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
        certPem: asString(raw.certPem),
        fullchainPem: asString(raw.fullchainPem),
        keyPem: asString(raw.keyPem),
        chainPem: asString(raw.chainPem),
        issuedKeyAlgorithm: asString(raw.issuedKeyAlgorithm, cert.issuedKeyAlgorithm),
        issuedSignatureAlgorithm: asString(raw.issuedSignatureAlgorithm, cert.issuedSignatureAlgorithm),
      }
    }
  } finally {
    viewLoading.value = false
  }
}

const downloadCertificateMaterial = (kind: 'cert' | 'key' | 'fullchain' | 'chain') => {
  const material = viewContent.value
  const contentByKind = {
    cert: material.certPem,
    key: material.keyPem,
    fullchain: material.fullchainPem,
    chain: material.chainPem,
  }
  const content = contentByKind[kind]
  if (content.trim() === '') return
  const fileByKind = {
    cert: 'cert.pem',
    key: 'key.pem',
    fullchain: 'fullchain.pem',
    chain: 'chain.pem',
  }
  const baseName = (material.mainDomain || 'certificate').replace(/[^a-zA-Z0-9._-]+/g, '_')
  const blob = new Blob([`${content.trim()}\n`], { type: 'application/x-pem-file;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `${baseName}-${fileByKind[kind]}`
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

const confirmPushDialog = async () => {
  if (pushDialogCertId.value === 0) return
  const clear = pushDialogClearMode.value
  const targetDir = pushDialogTargetDir.value.trim()
  if (!clear && targetDir === '') return

  pushingId.value = pushDialogCertId.value
  try {
    const msg = await HttpUtils.post('api/acme-push', {
      id: pushDialogCertId.value,
      ...(clear ? { clear: true } : { targetDir }),
    })
    if (msg.success) {
      applyActionResult(msg.obj)
      push.success({
        duration: 3500,
        message: clear ? '证书目录推送已清除' : '证书已推送到目录',
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
      accountKeyLength: 'ec-256',
      registered: false,
      remark: '',
    }
  } else {
    acmeAccountForm.value = {
      id: item.id,
      name: item.name,
      email: item.email,
      server: normalizeDomainCAValue(item.server, 'letsencrypt'),
      accountKeyLength: item.accountKeyLength || 'ec-256',
      registered: item.registered,
      remark: item.remark,
    }
  }
  acmeAccountFormVisible.value = true
}

const saveAcmeAccount = async () => {
  if (!canSaveAcmeAccount.value) return
  const normalizedEmail = normalizeAcmeContactEmails(acmeAccountForm.value.email)
  const normalizedServer = normalizeDomainCAValue(acmeAccountForm.value.server, '')
  const requiresEmail = normalizedServer === 'zerossl'
  if (!isLikelyValidAcmeContactEmails(normalizedEmail, requiresEmail)) {
    push.warning({
      duration: 3600,
      message: requiresEmail
        ? 'ZeroSSL 必须填写有效邮箱；可用英文逗号分隔多个邮箱'
        : '邮箱格式无效，请使用 ASCII 邮箱；可用英文逗号分隔多个邮箱',
    })
    return
  }
  if (normalizedEmail !== acmeAccountForm.value.email) {
    acmeAccountForm.value.email = normalizedEmail
  }
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
      accountKeyLength: acmeAccountForm.value.accountKeyLength,
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

const openAcmeAccountRotateForm = (item: AcmeAccount) => {
  if (!item.registered) return
  acmeAccountRotateForm.value = {
    id: item.id,
    resourceId: item.resourceId || `acme_${item.displayId || item.id}`,
    name: item.name,
    accountKeyLength: item.accountKeyLength || 'ec-256',
  }
  acmeAccountRotateVisible.value = true
}

const rotateAcmeAccountKey = async () => {
  const form = acmeAccountRotateForm.value
  if (form.id <= 0 || form.accountKeyLength.trim() === '') return
  const confirmed = await confirm({
    message: `确认轮换 ${form.resourceId} 的账号密钥吗？轮换会调用 acme.sh --update-account-key。`,
    severity: 'warning',
    confirmText: confirmAction('rotate'),
  })
  if (!confirmed) return

  savingAcmeAccount.value = true
  try {
    const msg = await HttpUtils.post('api/acme-account-key-rotate', {
      id: form.id,
      accountKeyLength: form.accountKeyLength,
    }, { timeout: acmeIssueRequestTimeout })
    if (msg.success) {
      applyActionResult(msg.obj)
      acmeAccountRotateVisible.value = false
      push.success({ duration: 3600, message: 'ACME 账号密钥已轮换' })
    }
  } finally {
    savingAcmeAccount.value = false
  }
}

const deleteAcmeAccount = async (item: AcmeAccount) => {
  const confirmed = await confirm({
    message: `确认删除 ACME 账号「${item.name}」吗？`,
    severity: 'danger',
    confirmText: confirmAction('delete'),
  })
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
      providerLocked: false,
      env: {},
      extraEnvText: '',
      remark: '',
    }
  } else {
    dnsAccountForm.value = {
      id: item.id,
      name: item.name,
      providerCode: item.providerCode,
      providerLocked: item.providerLocked,
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
  const confirmed = await confirm({
    message: `确认删除 DNS 账号「${item.name}」吗？`,
    severity: 'danger',
    confirmText: confirmAction('delete'),
  })
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
    if (issueLogVisible.value) {
      startIssueLogPolling()
    } else {
      void restoreActiveAcmeTask()
    }
    return
  }
  stopPolling()
  stopIssueLogPolling()
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
    void restoreActiveAcmeTask()
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
  const account = overview.value.acmeAccounts.find(item => item.id === value)
  if (!account) return
  if (account.server.trim() !== '') {
    issueForm.value.server = normalizeDomainCAValue(account.server, 'letsencrypt')
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
  issueForm.value.dnsEnv = ''
})

watch(() => issueForm.value.certificateType, (value) => {
  if (isReissueMode.value) {
    if (value === 'ip') {
      void refreshIPCertificateOptions()
      void refreshIPPortStatus()
    }
    return
  }
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
    void restoreActiveAcmeTask()
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
  justify-content: flex-end;
  gap: 10px;
  flex-wrap: wrap;
}

.acme-hero-action {
  min-width: 112px;
  min-height: 38px;
  border: 1px solid rgba(94, 234, 212, 0.52) !important;
  border-radius: 11px;
  color: #f8fafc !important;
  background: rgba(15, 78, 75, 0.24) !important;
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.05),
    0 10px 24px rgba(8, 47, 73, 0.14);
  backdrop-filter: blur(8px);
  transition:
    background-color 0.16s ease,
    border-color 0.16s ease,
    box-shadow 0.16s ease,
    transform 0.16s ease;
}

.acme-hero-action:hover {
  border-color: rgba(153, 246, 228, 0.86) !important;
  background: rgba(20, 184, 166, 0.2) !important;
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.08),
    0 12px 28px rgba(20, 184, 166, 0.18);
  transform: translateY(-1px);
}

.acme-hero-action:focus-visible {
  outline: 2px solid rgba(204, 251, 241, 0.72);
  outline-offset: 2px;
}

.acme-hero-action :deep(.v-btn__content) {
  color: inherit !important;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 0;
  white-space: nowrap;
}

.acme-hero-action :deep(.v-btn__prepend) {
  margin-inline-end: 8px;
}

.acme-hero-action :deep(.v-icon) {
  color: #ecfeff !important;
  opacity: 1;
}

.acme-hero-action.v-btn--disabled {
  opacity: 1 !important;
  border-color: var(--kwor-disabled-button-border) !important;
  color: var(--kwor-disabled-button-foreground) !important;
  background: var(--kwor-disabled-button-background) !important;
  box-shadow: none;
}

.acme-hero-action.v-btn--disabled :deep(.v-btn__content),
.acme-hero-action.v-btn--disabled :deep(.v-icon) {
  opacity: 1;
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

.acme-runtime-btn.acme-runtime-btn--check:not(.v-btn--loading) :deep(.v-btn__content),
.acme-runtime-btn.acme-runtime-btn--danger:not(.v-btn--loading) :deep(.v-btn__content),
.acme-runtime-btn.acme-runtime-btn--check:not(.v-btn--loading) :deep(.v-icon),
.acme-runtime-btn.acme-runtime-btn--danger:not(.v-btn--loading) :deep(.v-icon) {
  opacity: 1;
}

.acme-account-selection {
  width: 100%;
  min-width: 0;
  overflow: hidden;
  padding-inline-end: 2px;
  line-height: 1.2;
}

.acme-account-selection .acme-account-option__primary,
.acme-account-selection .acme-account-option__secondary {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
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

.acme-wrap-text,
.acme-auto-renew-state {
  max-width: 240px;
  white-space: normal;
  overflow-wrap: anywhere;
  word-break: break-word;
  line-height: 1.45;
}

.acme-remark {
  max-width: 220px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.acme-push-dialog {
  max-width: 100%;
}

.acme-push-path-list {
  display: grid;
  gap: 12px;
}

.acme-push-path-field :deep(input) {
  min-width: 0;
  overflow-x: auto;
  text-overflow: clip;
  white-space: nowrap;
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
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .acme-hero-action {
    width: 100%;
    min-width: 0;
  }

  .acme-hero-action :deep(.v-btn__content) {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
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

  .acme-push-dialog .v-card-actions {
    flex-wrap: wrap;
  }

  .acme-wrap-text,
  .acme-auto-renew-state {
    max-width: 190px;
  }
}
</style>
