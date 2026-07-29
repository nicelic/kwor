//go:build !linux

package service

import "fmt"

func startPanelUninstallWorker(string, string) error {
	return fmt.Errorf("面板内卸载仅支持 Linux 宿主机部署")
}
