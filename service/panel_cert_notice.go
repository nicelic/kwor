package service

import "sync/atomic"

var panelCertLoginWarning atomic.Value

func init() {
	panelCertLoginWarning.Store("")
}

func SetPanelCertLoginWarning(message string) {
	panelCertLoginWarning.Store(message)
}

func GetPanelCertLoginWarning() string {
	value := panelCertLoginWarning.Load()
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
