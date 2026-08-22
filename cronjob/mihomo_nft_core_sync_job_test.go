package cronjob

import "testing"

func TestNextMihomoCoreRecoveryRetryIntervalIsBounded(t *testing.T) {
	if got := nextMihomoCoreRecoveryRetryInterval(0); got != managedCoreAutoRecoverRetryInterval {
		t.Fatalf("initial retry interval = %s, want %s", got, managedCoreAutoRecoverRetryInterval)
	}
	if got := nextMihomoCoreRecoveryRetryInterval(mihomoCoreRecoveryMaxRetryInterval); got != mihomoCoreRecoveryMaxRetryInterval {
		t.Fatalf("max retry interval = %s, want %s", got, mihomoCoreRecoveryMaxRetryInterval)
	}
}
