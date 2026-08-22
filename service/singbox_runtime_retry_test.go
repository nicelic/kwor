package service

import (
	"testing"
)

func TestRetrySingboxRuntimeOnlyRegeneratesRuntime(t *testing.T) {
	original := regenerateCommittedSingboxRuntime
	calls := 0
	regenerateCommittedSingboxRuntime = func(configService *ConfigService) error {
		if configService == nil {
			t.Fatal("retry passed a nil config service to the runtime generator")
		}
		calls++
		return nil
	}
	t.Cleanup(func() { regenerateCommittedSingboxRuntime = original })

	if err := (&ConfigService{}).RetrySingboxRuntime(); err != nil {
		t.Fatalf("retry runtime: %v", err)
	}
	if calls != 1 {
		t.Fatalf("runtime generator calls = %d, want 1", calls)
	}
	if err := (*ConfigService)(nil).RetrySingboxRuntime(); err == nil || err.Error() != "config service is nil" {
		t.Fatalf("nil config service error = %v", err)
	}
}
