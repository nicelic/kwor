package api

import (
	"testing"

	"github.com/alireza0/s-ui/service"
)

func TestApiServiceUsesInjectedSharedCoreManagers(t *testing.T) {
	singbox := &service.CoreManagerService{}
	mihomo := &service.MihomoCoreManagerService{}
	apiService := &ApiService{}
	apiService.SetCoreManagers(singbox, mihomo)

	if apiService.coreManagerService() != singbox {
		t.Fatal("sing-box API did not retain the shared Core Manager instance")
	}
	if apiService.mihomoCoreManagerService() != mihomo {
		t.Fatal("Mihomo API did not retain the shared Core Manager instance")
	}
}
