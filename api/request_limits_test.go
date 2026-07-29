package api

import (
	"testing"

	"github.com/alireza0/s-ui/database"
)

func TestAPIRequestBodyLimit(t *testing.T) {
	tests := []struct {
		action string
		want   int64
	}{
		{action: "save", want: apiDefaultRequestMaxBytes},
		{action: "importdb", want: database.MaxDatabaseImportUploadBytes()},
		{action: "restore-db-backup", want: database.MaxDBBackupArchiveUploadBytes()},
	}

	for _, tt := range tests {
		if got := apiRequestBodyLimit(tt.action); got != tt.want {
			t.Fatalf("apiRequestBodyLimit(%q) = %d, want %d", tt.action, got, tt.want)
		}
	}
}
