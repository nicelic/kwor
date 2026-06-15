package service

import "sync/atomic"

var subscriptionTLSLoginWarning atomic.Value

func init() {
	subscriptionTLSLoginWarning.Store("")
}

func SetSubscriptionTLSLoginWarning(message string) {
	subscriptionTLSLoginWarning.Store(message)
}

func GetSubscriptionTLSLoginWarning() string {
	value := subscriptionTLSLoginWarning.Load()
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}
