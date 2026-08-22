import Data from '@/store/modules/data'
import MihomoData from '@/store/modules/mihomoData'

export type UiNamespace = 'default' | 'mihomo'

export interface NamespaceCoreConfig {
  modalButtonLabel: string
  supportsPrereleaseChannel: boolean
  statusEndpoint: string
  progressEndpoint: string
  updateInfoEndpoint: string
  startEndpoint: string
  stopEndpoint: string
  restartEndpoint: string
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
    modalButtonLabel: 'coreManager.singboxTitle',
    supportsPrereleaseChannel: true,
    statusEndpoint: 'api/core-status',
    progressEndpoint: 'api/core-download-progress',
    updateInfoEndpoint: 'api/core-update-info',
    startEndpoint: 'api/coreStart',
    stopEndpoint: 'api/coreStop',
    restartEndpoint: 'api/coreRestart',
  },
}

const mihomoNamespaceApi: NamespaceApiConfig = {
  syncEndpoint: 'api/mihomoSyncToSubManager',
  inboundIpsEndpoint: 'api/mihomo-inbound-ips',
  portLogStorageKey: 'mihomo-inbounds-port-monitor-logs',
  itemsPerPageKey: 'mihomo-items-per-page',
  subscriptionPathPrefix: 'mihomo/',
  supportsSubscriptionQr: true,
  portHopTypes: ['hysteria2'],
  showCoreControlsOnInbounds: true,
  core: {
    modalButtonLabel: 'coreManager.mihomoTitle',
    supportsPrereleaseChannel: true,
    statusEndpoint: 'api/mihomo-core-status',
    progressEndpoint: 'api/mihomo-core-download-progress',
    updateInfoEndpoint: 'api/mihomo-core-update-info',
    startEndpoint: 'api/mihomo-coreStart',
    stopEndpoint: 'api/mihomo-coreStop',
    restartEndpoint: 'api/mihomo-coreRestart',
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
