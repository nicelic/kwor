package service

import (
	"strconv"
	"strings"
)

func listRuleCommentsByPrefix(chain string, prefix string) ([]chainRuleComment, error) {
	return listRuleCommentsByPrefixFn(chain, prefix)
}

var listRuleCommentsByPrefixFn = func(chain string, prefix string) ([]chainRuleComment, error) {
	out, err := runNft("--handle", "--numeric", "list", "chain", nftFamily, nftTable, chain)
	if err != nil {
		if nftObjectMissing(err) {
			return []chainRuleComment{}, nil
		}
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	rules := make([]chainRuleComment, 0)
	for _, line := range lines {
		comment, ok := extractRuleComment(line)
		if !ok || !strings.HasPrefix(comment, prefix) {
			continue
		}
		m := nftHandleRe.FindStringSubmatch(line)
		if len(m) != 2 {
			continue
		}
		handle, convErr := strconv.Atoi(m[1])
		if convErr != nil || handle <= 0 {
			continue
		}
		rules = append(rules, chainRuleComment{
			handle:  handle,
			comment: comment,
		})
	}
	return rules, nil
}

func deleteRulesByCommentPrefix(prefix string) error {
	if !nftSupported() || !nftTableExists() || prefix == "" {
		return nil
	}

	chains := []string{nftChainIn, nftChainOut, nftChainForward, nftChainPrerouting}
	var firstErr error
	for _, chain := range chains {
		rules, err := listRuleCommentsByPrefix(chain, prefix)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, rule := range rules {
			if err = deleteRuleByHandle(chain, rule.handle); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
