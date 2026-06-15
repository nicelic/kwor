package service

import "strings"

func GetLoginWarning() string {
	parts := make([]string, 0, 2)
	if warning := strings.TrimSpace(GetPanelCertLoginWarning()); warning != "" {
		parts = append(parts, warning)
	}
	if warning := strings.TrimSpace(GetSubscriptionTLSLoginWarning()); warning != "" {
		parts = append(parts, warning)
	}
	return strings.Join(parts, "; ")
}
