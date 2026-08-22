package service

import "strings"

type managedRedirectRuleSnapshot struct {
	valid    map[string]bool
	comments map[string]struct{}
}

func (s *managedRedirectRuleSnapshot) hasComment(comment string) bool {
	if s == nil || comment == "" {
		return false
	}
	_, ok := s.comments[comment]
	return ok
}

// snapshotManagedRuleHandles reads one nft chain listing and indexes managed
// comments in memory. Integrity jobs can then validate every state row without
// spawning nft once per rule.
func snapshotManagedRuleHandles(chain string, prefix string) (map[string]int, error) {
	rules, err := listRuleCommentsByPrefix(chain, prefix)
	if err != nil {
		return nil, err
	}
	result := make(map[string]int, len(rules))
	for _, rule := range rules {
		if rule.comment == "" || rule.handle <= 0 {
			continue
		}
		if _, exists := result[rule.comment]; !exists {
			result[rule.comment] = rule.handle
		}
	}
	return result, nil
}

// snapshotManagedRedirectRules validates redirect comments from the same
// bounded chain snapshots used by the inbound integrity pass. A comment is
// valid only when every current-layout chain has one rule and all stale layout
// chains have none.
func snapshotManagedRedirectRules(prefix string) (*managedRedirectRuleSnapshot, error) {
	type countState struct {
		current int
		stale   int
	}
	counts := map[string]*countState{}
	for _, location := range nftRedirectRuleLocations() {
		out, err := runNft("--handle", "--numeric", "list", "chain", location.tableFamily, location.table, location.chain)
		if err != nil {
			if nftObjectMissing(err) {
				continue
			}
			return nil, err
		}
		for _, line := range strings.Split(string(out), "\n") {
			comment, ok := extractRuleComment(line)
			if !ok || !strings.HasPrefix(comment, prefix) {
				continue
			}
			state := counts[comment]
			if state == nil {
				state = &countState{}
				counts[comment] = state
			}
			if nftRedirectLocationIsCurrent(location) {
				state.current++
			} else {
				state.stale++
			}
		}
	}
	snapshot := &managedRedirectRuleSnapshot{
		valid:    make(map[string]bool, len(counts)),
		comments: make(map[string]struct{}, len(counts)),
	}
	for comment, count := range counts {
		snapshot.valid[comment] = count.current == len(nftCurrentRedirectRuleLocations()) && count.stale == 0
		snapshot.comments[comment] = struct{}{}
	}
	return snapshot, nil
}

func nftCurrentRedirectRuleLocations() []nftRuleLocation {
	locations := nftRedirectRuleLocations()
	current := make([]nftRuleLocation, 0, len(locations))
	for _, location := range locations {
		if nftRedirectLocationIsCurrent(location) {
			current = append(current, location)
		}
	}
	return current
}
