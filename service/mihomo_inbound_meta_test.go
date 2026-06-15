package service

import "testing"

func TestBuildMihomoInboundUserManagementSudoku(t *testing.T) {
	got := buildMihomoInboundUserManagement("sudoku", 0)

	if !got.Selectable {
		t.Fatalf("expected sudoku inbound to be selectable, got %#v", got)
	}
	if got.UsesUsersField {
		t.Fatalf("expected sudoku inbound to avoid runtime users field, got %#v", got)
	}
	if got.Mode != "shared_uuid" {
		t.Fatalf("expected mode shared_uuid, got %#v", got.Mode)
	}
	if got.IdentityType != "uuid" {
		t.Fatalf("expected identity_type uuid, got %#v", got.IdentityType)
	}
}
