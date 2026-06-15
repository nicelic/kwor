import Data from '@/store/modules/data'
import MihomoData from '@/store/modules/mihomoData'

export type UiNamespace = 'default' | 'mihomo'

export interface NamespaceCoreConfig {
  coreName: string
  modalTitle: string
  modalButtonLabel: string
  supportsPrereleaseChannel: boolean
  repoUrl: string
  statusEndpoint: string
  progressEndpoint: string
  versionsEndpoint: string
  updateInfoEndpoint: string
  updateSettingsEndpoint: string
  updateAckEndpoint: string
  downloadPreferenceEndpoint: string
  downloadEndpoint: string
  startEndpoint: string
  stopEndpoint: string
  restartEndpoint: string
  deleteEndpoint: string
  configPath: string
  binaryBaseName: string
}

export interface NamespaceApiConfig {
  syncEndpoint: string
  inboundIpsEndpoint: string
  portLogStorageKey: string
  itemsPerPageKey: string
  subscriptionPathPrefix: string
  supportsSubscriptionQr: boolean
  portHopTypes: string[]
  showCoreControlsOnInbounds: boolean
  core: NamespaceCoreConfig
}

const defaultNamespaceApi: NamespaceApiConfig = {
  syncEndpoint: 'api/syncToSubManager',
  inboundIpsEndpoint: 'api/inbound-ips',
  portLogStorageKey: 'inbounds-port-monitor-logs',
  itemsPerPageKey: 'items-per-page',
  subscriptionPathPrefix: '',
  supportsSubscriptionQr: true,
  portHopTypes: ['hysteria', 'hysteria2'],
  showCoreControlsOnInbounds: true,
  core: {
    coreName: 'sing-box',
    modalTitle: 'coreManager.singboxTitle',
    modalButtonLabel: 'coreManager.singboxTitle',
    supportsPrereleaseChannel: true,
    repoUrl: 'https://github.com/SagerNet/sing-box/releases',
    statusEndpoint: 'api/core-status',
    progressEndpoint: 'api/core-download-progress',
    versionsEndpoint: 'api/core-versions',
    updateInfoEndpoint: 'api/core-update-info',
    updateSettingsEndpoint: 'api/core-update-settings',
    updateAckEndpoint: 'api/core-update-ack',
    downloadPreferenceEndpoint: 'api/core-download-preference',
    downloadEndpoint: 'api/coreDownload',
    startEndpoint: 'api/coreStart',
    stopEndpoint: 'api/coreStop',
    restartEndpoint: 'api/coreRestart',
    deleteEndpoint: 'api/coreDelete',
    configPath: 'Promanager_data/core/singbox/config.json',
    binaryBaseName: 'sing-box',
  },
}

const mihomoNamespaceApi: NamespaceApiConfig = {
  syncEndpoint: 'api/mihomoSyncToSubManager',
  inboundIpsEndpoint: 'api/mihomo-inbound-ips',
  portLogStorageKey: 'mihomo-inbounds-port-monitor-logs',
  itemsPerPageKey: 'mihomo-items-per-page',
  subscriptionPathPrefix: 'mihomo/',
  supportsSubscriptionQr: true,
  portHopTypes: ['hysteria', 'hysteria2'],
  showCoreControlsOnInbounds: true,
  core: {
    coreName: 'mihomo',
    modalTitle: 'coreManager.mihomoTitle',
    modalButtonLabel: 'coreManager.mihomoTitle',
    supportsPrereleaseChannel: false,
    repoUrl: 'https://github.com/MetaCubeX/mihomo/releases',
    statusEndpoint: 'api/mihomo-core-status',
    progressEndpoint: 'api/mihomo-core-download-progress',
    versionsEndpoint: 'api/mihomo-core-versions',
    updateInfoEndpoint: 'api/mihomo-core-update-info',
    updateSettingsEndpoint: 'api/mihomo-core-update-settings',
    updateAckEndpoint: 'api/mihomo-core-update-ack',
    downloadPreferenceEndpoint: 'api/mihomo-core-download-preference',
    downloadEndpoint: 'api/mihomo-coreDownload',
    startEndpoint: 'api/mihomo-coreStart',
    stopEndpoint: 'api/mihomo-coreStop',
    restartEndpoint: 'api/mihomo-coreRestart',
    deleteEndpoint: 'api/mihomo-coreDelete',
    configPath: 'Promanager_data/core/mihomo/server.yaml',
    binaryBaseName: 'mihomo',
  },
}

export const normalizeNamespace = (namespace?: string): UiNamespace => {
  return namespace === 'mihomo' ? 'mihomo' : 'default'
}

export const getNamespaceStore = (namespace?: string) => {
  return normalizeNamespace(namespace) === 'mihomo' ? MihomoData() : Data()
}

export const getNamespaceApi = (namespace?: string): NamespaceApiConfig => {
  return normalizeNamespace(namespace) === 'mihomo' ? mihomoNamespaceApi : defaultNamespaceApi
}

export const getNamespaceCore = (namespace?: string): NamespaceCoreConfig => {
  return getNamespaceApi(namespace).core
}
