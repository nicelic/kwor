package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcNetTCPStateIsActive(t *testing.T) {
	activeStates := []string{"01", "02", "03", "04", "05", "08", "09", "0B", "0C"}
	for _, state := range activeStates {
		if !procNetTCPStateIsActive(state) {
			t.Fatalf("expected state %s to be active", state)
		}
	}

	inactiveStates := []string{"06", "07", "0A", "FF", ""}
	for _, state := range inactiveStates {
		if procNetTCPStateIsActive(state) {
			t.Fatalf("expected state %s to be inactive", state)
		}
	}
}

func TestCountProcNetTCPActiveConnections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tcp")
	content := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 0100007F:0036 0200007F:1770 01 00000000:00000000 00:00000000 00000000     0        0 2 1 0000000000000000 100 0 0 10 0\n" +
		"   2: 0100007F:0037 0300007F:1770 06 00000000:00000000 00:00000000 00000000     0        0 3 1 0000000000000000 100 0 0 10 0\n" +
		"   3: 0100007F:0038 0400007F:1770 08 00000000:00000000 00:00000000 00000000     0        0 4 1 0000000000000000 100 0 0 10 0\n" +
		"   4: 0100007F:0039 0500007F:1770 03 00000000:00000000 00:00000000 00000000     0        0 5 1 0000000000000000 100 0 0 10 0\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	count, err := countProcNetTCPActiveConnections(path)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("expected 3 active tcp connections, got %d", count)
	}
}

func TestReadProcNetTCPStateCounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tcp")
	content := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:0035 00000000:0000 03 00000000:00000000 00:00000000 00000000     0        0 1 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 0100007F:0036 0200007F:1770 01 00000000:00000000 00:00000000 00000000     0        0 2 1 0000000000000000 100 0 0 10 0\n" +
		"   2: 0100007F:0037 0300007F:1770 0C 00000000:00000000 00:00000000 00000000     0        0 3 1 0000000000000000 100 0 0 10 0\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	counts, err := readProcNetTCPStateCounts(path)
	if err != nil {
		t.Fatal(err)
	}
	if counts.establishedCount() != 1 {
		t.Fatalf("expected 1 established tcp connection, got %d", counts.establishedCount())
	}
	if counts.halfOpenCount() != 2 {
		t.Fatalf("expected 2 half-open tcp connections, got %d", counts.halfOpenCount())
	}
}

func TestCountProcNetSocketEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "udp")
	content := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops\n" +
		"   0: 00000000:14E9 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 10 2 0000000000000000 0\n" +
		"   1: 0100007F:14EA 0200007F:1770 01 00000000:00000000 00:00000000 00000000     0        0 11 2 0000000000000000 0\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	count, err := countProcNetSocketEntries(path)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 udp socket entries, got %d", count)
	}
}

func TestReadProcNetProtocolStats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "netstat")
	content := "TcpExt: SyncookiesSent ListenOverflows ListenDrops\n" +
		"TcpExt: 12 3 4\n" +
		"Udp: InDatagrams NoPorts InErrors RcvbufErrors\n" +
		"Udp: 200 7 8 9\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	tcpStats, err := readProcNetProtocolStats(path, "TcpExt")
	if err != nil {
		t.Fatal(err)
	}
	if tcpStats["SyncookiesSent"] != 12 || tcpStats["ListenOverflows"] != 3 || tcpStats["ListenDrops"] != 4 {
		t.Fatalf("unexpected tcp stats: %#v", tcpStats)
	}

	udpStats, err := readProcNetProtocolStats(path, "Udp")
	if err != nil {
		t.Fatal(err)
	}
	if udpStats["NoPorts"] != 7 || udpStats["InErrors"] != 8 || udpStats["RcvbufErrors"] != 9 {
		t.Fatalf("unexpected udp stats: %#v", udpStats)
	}
}

func TestReadFirewallConnectionStatsFromSources_PartialFailureStillReturnsAvailableStats(t *testing.T) {
	dir := t.TempDir()
	tcpPath := filepath.Join(dir, "tcp")
	udpPath := filepath.Join(dir, "udp")
	netstatPath := filepath.Join(dir, "netstat")
	snmpPath := filepath.Join(dir, "snmp")

	tcpContent := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 0100007F:0035 00000000:0000 03 00000000:00000000 00:00000000 00000000     0        0 1 1 0000000000000000 100 0 0 10 0\n" +
		"   1: 0100007F:0036 0200007F:1770 01 00000000:00000000 00:00000000 00000000     0        0 2 1 0000000000000000 100 0 0 10 0\n"
	udpContent := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ref pointer drops\n" +
		"   0: 00000000:14E9 00000000:0000 07 00000000:00000000 00:00000000 00000000     0        0 10 2 0000000000000000 0\n"
	netstatContent := "TcpExt: SyncookiesSent ListenOverflows ListenDrops\n" +
		"TcpExt: 5 6 7\n"
	snmpContent := "Udp: InDatagrams NoPorts InErrors RcvbufErrors\n" +
		"Udp: 100 2 3 4\n"

	if err := os.WriteFile(tcpPath, []byte(tcpContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(udpPath, []byte(udpContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(netstatPath, []byte(netstatContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snmpPath, []byte(snmpContent), 0o600); err != nil {
		t.Fatal(err)
	}

	stats, err := readFirewallConnectionStatsFromSources(firewallConnectionStatSources{
		tcp4Path:    tcpPath,
		tcp6Path:    dir,
		udp4Path:    udpPath,
		udp6Path:    dir,
		tcpExtPath:  netstatPath,
		udpStatPath: snmpPath,
	})
	if err == nil {
		t.Fatal("expected partial read error")
	}
	if stats.TCPSynRecvCount != 1 {
		t.Fatalf("expected 1 tcp half-open connection, got %d", stats.TCPSynRecvCount)
	}
	if stats.TCPEstablishedCount != 1 {
		t.Fatalf("expected 1 tcp established connection, got %d", stats.TCPEstablishedCount)
	}
	if stats.TCPActiveCount != 2 {
		t.Fatalf("expected 2 active tcp connections, got %d", stats.TCPActiveCount)
	}
	if stats.UDPSocketCount != 1 {
		t.Fatalf("expected 1 udp socket entry, got %d", stats.UDPSocketCount)
	}
	if stats.TCPAnomalyTotal != 18 {
		t.Fatalf("expected tcp anomaly total 18, got %d", stats.TCPAnomalyTotal)
	}
	if stats.UDPAnomalyTotal != 9 {
		t.Fatalf("expected udp anomaly total 9, got %d", stats.UDPAnomalyTotal)
	}
}
