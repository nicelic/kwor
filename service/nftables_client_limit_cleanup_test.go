package service

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestClientLimitRemoveRulesFromStateFallsBackToComment(t *testing.T) {
	originalDeleteByHandle := deleteRuleByHandleFn
	originalDeleteByComment := deleteRuleByCommentFn
	originalNftSupported := nftSupportedFn
	defer func() {
		deleteRuleByHandleFn = originalDeleteByHandle
		deleteRuleByCommentFn = originalDeleteByComment
		nftSupportedFn = originalNftSupported
	}()

	nftSupportedFn = func() bool { return true }
	calls := make([]string, 0, 2)
	deleteRuleByHandleFn = func(chain string, handle int) error {
		calls = append(calls, "handle")
		return nil
	}
	deleteRuleByCommentFn = func(chain string, comment string) error {
		calls = append(calls, chain+":"+comment)
		return nil
	}

	err := (&ClientRateLimitService{}).removeRulesFromState(&model.ClientPortLimitState{
		Port:      30000,
		InHandle:  0,
		OutHandle: 0,
	})
	if err != nil {
		t.Fatalf("removeRulesFromState failed: %v", err)
	}

	want := []string{
		nftChainIn + ":" + singboxLimitNftRuleComments.in("30000"),
		nftChainOut + ":" + singboxLimitNftRuleComments.out("30000"),
	}
	if len(calls) != len(want) {
		t.Fatalf("unexpected calls: got=%v want=%v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("unexpected call %d: got=%q want=%q", i, calls[i], want[i])
		}
	}
}

func TestClientLimitCleanupOrphanRulesDeletesOnlyInvalidComments(t *testing.T) {
	originalList := listRuleCommentsByPrefixFn
	originalDelete := deleteRuleByHandleFn
	originalExists := nftTableExistsFn
	originalNftSupported := nftSupportedFn
	defer func() {
		listRuleCommentsByPrefixFn = originalList
		deleteRuleByHandleFn = originalDelete
		nftTableExistsFn = originalExists
		nftSupportedFn = originalNftSupported
	}()

	nftSupportedFn = func() bool { return true }
	nftTableExistsFn = func() bool { return true }
	listRuleCommentsByPrefixFn = func(chain string, prefix string) ([]chainRuleComment, error) {
		if chain == nftChainIn {
			return []chainRuleComment{
				{handle: 10, comment: singboxLimitNftRuleComments.in("30000")},
				{handle: 11, comment: singboxLimitNftRuleComments.in("40000")},
			}, nil
		}
		return []chainRuleComment{
			{handle: 20, comment: singboxLimitNftRuleComments.out("30000")},
			{handle: 21, comment: singboxLimitNftRuleComments.out("40000")},
		}, nil
	}

	deleted := make([]int, 0, 2)
	deleteRuleByHandleFn = func(chain string, handle int) error {
		deleted = append(deleted, handle)
		return nil
	}

	err := (&ClientRateLimitService{}).cleanupOrphanRules(map[string]struct{}{
		singboxLimitNftRuleComments.in("30000"):  {},
		singboxLimitNftRuleComments.out("30000"): {},
	})
	if err != nil {
		t.Fatalf("cleanupOrphanRules failed: %v", err)
	}

	if len(deleted) != 2 || deleted[0] != 11 || deleted[1] != 21 {
		t.Fatalf("unexpected deleted handles: %v", deleted)
	}
}
