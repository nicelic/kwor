package service

import "testing"

func TestManagedRedirectRuleSnapshotHasComment(t *testing.T) {
	snapshot := &managedRedirectRuleSnapshot{
		comments: map[string]struct{}{
			"mihomo-redirect:legacy": {},
		},
	}
	if !snapshot.hasComment("mihomo-redirect:legacy") {
		t.Fatal("expected managed redirect comment to be found in the snapshot")
	}
	if snapshot.hasComment("mihomo-redirect:missing") {
		t.Fatal("unexpected missing managed redirect comment")
	}
	if (*managedRedirectRuleSnapshot)(nil).hasComment("mihomo-redirect:legacy") {
		t.Fatal("nil snapshot must not report managed redirect comments")
	}
}
