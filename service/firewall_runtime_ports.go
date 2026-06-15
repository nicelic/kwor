package service

import "sync"

type FirewallRuntimePortProvider interface {
	GetActivePanelPort() int
	GetActiveSubPort() int
}

var firewallRuntimePortProviderState struct {
	mu       sync.RWMutex
	provider FirewallRuntimePortProvider
}

func RegisterFirewallRuntimePortProvider(provider FirewallRuntimePortProvider) {
	firewallRuntimePortProviderState.mu.Lock()
	firewallRuntimePortProviderState.provider = provider
	firewallRuntimePortProviderState.mu.Unlock()
}

func loadActiveRuntimePanelPort() int {
	firewallRuntimePortProviderState.mu.RLock()
	provider := firewallRuntimePortProviderState.provider
	firewallRuntimePortProviderState.mu.RUnlock()
	if provider == nil {
		return 0
	}
	return provider.GetActivePanelPort()
}

func loadActiveRuntimeSubPort() int {
	firewallRuntimePortProviderState.mu.RLock()
	provider := firewallRuntimePortProviderState.provider
	firewallRuntimePortProviderState.mu.RUnlock()
	if provider == nil {
		return 0
	}
	return provider.GetActiveSubPort()
}
