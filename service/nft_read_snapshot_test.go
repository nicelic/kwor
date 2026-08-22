package service

import (
	"bytes"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithNftReadSnapshotReusesReadAndInvalidatesAfterMutation(t *testing.T) {
	originalRunNft := runNftFn
	defer func() {
		runNftFn = originalRunNft
		invalidateNftReadSnapshotCache()
	}()

	var calls atomic.Int32
	runNftFn = func(args ...string) ([]byte, error) {
		calls.Add(1)
		return []byte("table inet kwor {}"), nil
	}

	WithNftReadSnapshot(func() {
		if _, err := runNft("--handle", "--numeric", "list", "chain", "inet", "kwor", "input"); err != nil {
			t.Fatalf("first nft read failed: %v", err)
		}
		if _, err := runNft("--handle", "--numeric", "list", "chain", "inet", "kwor", "input"); err != nil {
			t.Fatalf("cached nft read failed: %v", err)
		}
		if got := calls.Load(); got != 1 {
			t.Fatalf("same snapshot read calls=%d, want 1", got)
		}

		if _, err := runNft("add", "rule", "inet", "kwor", "input", "counter"); err != nil {
			t.Fatalf("nft mutation failed: %v", err)
		}
		if _, err := runNft("--handle", "--numeric", "list", "chain", "inet", "kwor", "input"); err != nil {
			t.Fatalf("read after mutation failed: %v", err)
		}
	})

	if got := calls.Load(); got != 3 {
		t.Fatalf("read cache was not invalidated after mutation: calls=%d want 3", got)
	}
}

func TestWithNftReadSnapshotDoesNotReinsertReadThatRacedWithMutation(t *testing.T) {
	originalRunNft := runNftFn
	defer func() {
		runNftFn = originalRunNft
		invalidateNftReadSnapshotCache()
	}()

	readStarted := make(chan struct{})
	releaseRead := make(chan struct{})
	var calls atomic.Int32
	runNftFn = func(args ...string) ([]byte, error) {
		call := calls.Add(1)
		if nftReadOnlyCommand(args) && call == 1 {
			close(readStarted)
			<-releaseRead
			return []byte("old snapshot"), nil
		}
		if nftReadOnlyCommand(args) {
			return []byte("new snapshot"), nil
		}
		return nil, nil
	}

	WithNftReadSnapshot(func() {
		readResult := make(chan []byte, 1)
		go func() {
			data, err := runNft("list", "chain", "inet", "kwor", "input")
			if err != nil {
				readResult <- nil
				return
			}
			readResult <- data
		}()

		select {
		case <-readStarted:
		case <-time.After(time.Second):
			t.Fatal("read did not begin")
		}
		if _, err := runNft("add", "rule", "inet", "kwor", "input", "counter"); err != nil {
			t.Fatalf("mutation failed: %v", err)
		}
		close(releaseRead)
		if data := <-readResult; string(data) != "old snapshot" {
			t.Fatalf("racing read returned %q, want old snapshot fixture", data)
		}

		data, err := runNft("list", "chain", "inet", "kwor", "input")
		if err != nil {
			t.Fatalf("post-mutation read failed: %v", err)
		}
		if string(data) != "new snapshot" {
			t.Fatalf("post-mutation cache reused stale data: %q", data)
		}
	})

	if got := calls.Load(); got != 3 {
		t.Fatalf("nft calls=%d, want one read, one mutation, one fresh read", got)
	}
}

func TestWithNftReadSnapshotDoesNotRetainReadThatRacedWithScriptMutation(t *testing.T) {
	originalRunNft := runNftFn
	originalRunNftScript := runNftScriptFn
	defer func() {
		runNftFn = originalRunNft
		runNftScriptFn = originalRunNftScript
		invalidateNftReadSnapshotCache()
	}()

	scriptStarted := make(chan struct{})
	releaseScript := make(chan struct{})
	var readCalls atomic.Int32
	runNftFn = func(args ...string) ([]byte, error) {
		if readCalls.Add(1) == 1 {
			return []byte("old snapshot"), nil
		}
		return []byte("new snapshot"), nil
	}
	runNftScriptFn = func(string) ([]byte, error) {
		close(scriptStarted)
		<-releaseScript
		return nil, nil
	}

	WithNftReadSnapshot(func() {
		scriptDone := make(chan struct{})
		go func() {
			_, _ = runNftScript("flush table inet kwor")
			close(scriptDone)
		}()
		select {
		case <-scriptStarted:
		case <-time.After(time.Second):
			t.Fatal("script mutation did not start")
		}

		data, err := runNft("list", "chain", "inet", "kwor", "input")
		if err != nil || string(data) != "old snapshot" {
			t.Fatalf("read during script mutation = %q, %v", data, err)
		}
		close(releaseScript)
		select {
		case <-scriptDone:
		case <-time.After(time.Second):
			t.Fatal("script mutation did not finish")
		}

		data, err = runNft("list", "chain", "inet", "kwor", "input")
		if err != nil {
			t.Fatalf("read after script mutation failed: %v", err)
		}
		if string(data) != "new snapshot" {
			t.Fatalf("script mutation retained stale cache data: %q", data)
		}
	})

	if got := readCalls.Load(); got != 2 {
		t.Fatalf("nft read calls=%d, want a fresh read after script mutation", got)
	}
}

func TestWithNftReadSnapshotCachesSupportProbeForOneRound(t *testing.T) {
	originalSupported := nftSupportedFn
	defer func() {
		nftSupportedFn = originalSupported
		invalidateNftReadSnapshotCache()
	}()

	var calls atomic.Int32
	nftSupportedFn = func() bool {
		calls.Add(1)
		return true
	}

	WithNftReadSnapshot(func() {
		if !nftSupported() || !nftSupported() {
			t.Fatal("fixture nft support probe returned false")
		}
	})
	if got := calls.Load(); got != 1 {
		t.Fatalf("support probe calls=%d, want 1 in one snapshot", got)
	}

	if !nftSupported() {
		t.Fatal("fixture nft support probe returned false outside the snapshot")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("support probe calls=%d, want a fresh probe outside the snapshot", got)
	}
}

func TestWithNftReadSnapshotBoundsCachedOutput(t *testing.T) {
	originalRunNft := runNftFn
	defer func() {
		runNftFn = originalRunNft
		invalidateNftReadSnapshotCache()
	}()

	payload := bytes.Repeat([]byte("x"), nftReadSnapshotCacheMaxBytes/2+1)
	var calls atomic.Int32
	runNftFn = func(args ...string) ([]byte, error) {
		calls.Add(1)
		return append([]byte(nil), payload...), nil
	}

	WithNftReadSnapshot(func() {
		for _, chain := range []string{"input", "output", "forward"} {
			if _, err := runNft("list", "chain", "inet", "kwor", chain); err != nil {
				t.Fatalf("snapshot read %s failed: %v", chain, err)
			}
		}
		nftReadSnapshotCache.Lock()
		cachedBytes := nftReadSnapshotCache.bytes
		cachedEntries := len(nftReadSnapshotCache.entries)
		nftReadSnapshotCache.Unlock()
		if cachedBytes > nftReadSnapshotCacheMaxBytes || cachedEntries != 1 {
			t.Fatalf("snapshot cache bounds = bytes:%d entries:%d", cachedBytes, cachedEntries)
		}
		if _, err := runNft("list", "chain", "inet", "kwor", "input"); err != nil {
			t.Fatalf("cached snapshot read failed: %v", err)
		}
		if _, err := runNft("list", "chain", "inet", "kwor", "output"); err != nil {
			t.Fatalf("uncached snapshot read failed: %v", err)
		}
	})
	if got := calls.Load(); got != 4 {
		t.Fatalf("nft read calls=%d, want one cached and one uncached repeat", got)
	}
}

func TestWithNftReadSnapshotBoundsEmptyEntries(t *testing.T) {
	originalRunNft := runNftFn
	defer func() {
		runNftFn = originalRunNft
		invalidateNftReadSnapshotCache()
	}()

	runNftFn = func(args ...string) ([]byte, error) {
		return nil, nil
	}

	WithNftReadSnapshot(func() {
		for index := 0; index < nftReadSnapshotCacheMaxEntries+32; index++ {
			if _, err := runNft("list", "chain", "inet", "kwor", "empty-"+strconv.Itoa(index)); err != nil {
				t.Fatalf("empty snapshot read %d failed: %v", index, err)
			}
		}

		nftReadSnapshotCache.Lock()
		cachedEntries := len(nftReadSnapshotCache.entries)
		nftReadSnapshotCache.Unlock()
		if cachedEntries > nftReadSnapshotCacheMaxEntries {
			t.Fatalf("empty snapshot cache entries=%d, want at most %d", cachedEntries, nftReadSnapshotCacheMaxEntries)
		}
	})
}

func TestWithNftReadSnapshotSharesChainOutputBetweenIntegrityAndCounters(t *testing.T) {
	originalRunNft := runNftFn
	originalSupported := nftSupportedFn
	defer func() {
		runNftFn = originalRunNft
		nftSupportedFn = originalSupported
		invalidateNftReadSnapshotCache()
	}()

	nftSupportedFn = func() bool { return true }
	var calls atomic.Int32
	runNftFn = func(args ...string) ([]byte, error) {
		calls.Add(1)
		return []byte(`
tcp dport 443 counter packets 2 bytes 321 comment "kwor-inbound:sample" # handle 31
`), nil
	}

	WithNftReadSnapshot(func() {
		values, err := getChainRuleBytesByHandles(nftChainIn, []int{31})
		if err != nil {
			t.Fatalf("read nft counter from sampler snapshot failed: %v", err)
		}
		if values[31] != 321 {
			t.Fatalf("snapshot counter = %d, want 321", values[31])
		}
		rules, err := listRuleCommentsByPrefix(nftChainIn, "kwor-inbound:")
		if err != nil {
			t.Fatalf("read nft integrity comments from sampler snapshot failed: %v", err)
		}
		if len(rules) != 1 || rules[0].handle != 31 {
			t.Fatalf("snapshot rules = %#v, want one handle 31", rules)
		}
	})

	if got := calls.Load(); got != 1 {
		t.Fatalf("counter and integrity reads used %d nft commands, want one shared chain snapshot", got)
	}
}
