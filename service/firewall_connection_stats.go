package service

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type firewallConnectionStats struct {
	TCPActiveCount      int
	TCPSynRecvCount     int
	TCPEstablishedCount int
	TCPAnomalyTotal     int64
	UDPSocketCount      int
	UDPAnomalyTotal     int64
}

const (
	procNetSNMP    = "/proc/net/snmp"
	procNetNetstat = "/proc/net/netstat"
)

type firewallConnectionStatSources struct {
	tcp4Path    string
	tcp6Path    string
	udp4Path    string
	udp6Path    string
	tcpExtPath  string
	udpStatPath string
}

var defaultFirewallConnectionStatSources = firewallConnectionStatSources{
	tcp4Path:    procTCP,
	tcp6Path:    procTCP6,
	udp4Path:    procUDP,
	udp6Path:    procUDP6,
	tcpExtPath:  procNetNetstat,
	udpStatPath: procNetSNMP,
}

func readFirewallConnectionStats() (firewallConnectionStats, error) {
	if runtime.GOOS != "linux" {
		return firewallConnectionStats{}, nil
	}
	return readFirewallConnectionStatsFromSources(defaultFirewallConnectionStatSources)
}

func readFirewallConnectionStatsFromSources(sources firewallConnectionStatSources) (firewallConnectionStats, error) {
	stats := firewallConnectionStats{}
	tcpCounts := procNetTCPStateCounts{states: make(map[string]int)}
	var errs []error

	appendReadError := func(path string, err error) {
		if err == nil {
			return
		}
		errs = append(errs, fmt.Errorf("read %s: %w", path, err))
	}

	if counts, err := readProcNetTCPStateCounts(sources.tcp4Path); err != nil {
		appendReadError(sources.tcp4Path, err)
	} else {
		tcpCounts = mergeProcNetTCPStateCounts(tcpCounts, counts)
	}
	if counts, err := readProcNetTCPStateCounts(sources.tcp6Path); err != nil {
		appendReadError(sources.tcp6Path, err)
	} else {
		tcpCounts = mergeProcNetTCPStateCounts(tcpCounts, counts)
	}
	if count, err := countProcNetSocketEntries(sources.udp4Path); err != nil {
		appendReadError(sources.udp4Path, err)
	} else {
		stats.UDPSocketCount += count
	}
	if count, err := countProcNetSocketEntries(sources.udp6Path); err != nil {
		appendReadError(sources.udp6Path, err)
	} else {
		stats.UDPSocketCount += count
	}
	if tcpExtStats, err := readProcNetProtocolStats(sources.tcpExtPath, "TcpExt"); err != nil {
		appendReadError(sources.tcpExtPath, err)
	} else {
		tcpAnomalyTotal := tcpExtStats["SyncookiesSent"] + tcpExtStats["ListenOverflows"] + tcpExtStats["ListenDrops"]
		stats.TCPAnomalyTotal = clampInt64FromUint64(tcpAnomalyTotal)
	}
	if udpStats, err := readProcNetProtocolStats(sources.udpStatPath, "Udp"); err != nil {
		appendReadError(sources.udpStatPath, err)
	} else {
		udpAnomalyTotal := udpStats["NoPorts"] + udpStats["InErrors"] + udpStats["RcvbufErrors"]
		stats.UDPAnomalyTotal = clampInt64FromUint64(udpAnomalyTotal)
	}

	stats.TCPActiveCount = tcpCounts.activeCount()
	stats.TCPSynRecvCount = tcpCounts.halfOpenCount()
	stats.TCPEstablishedCount = tcpCounts.establishedCount()
	return stats, errors.Join(errs...)
}

type procNetTCPStateCounts struct {
	states map[string]int
}

func (c procNetTCPStateCounts) stateCount(state string) int {
	if c.states == nil {
		return 0
	}
	return c.states[strings.ToUpper(strings.TrimSpace(state))]
}

func (c procNetTCPStateCounts) establishedCount() int {
	return c.stateCount("01")
}

func (c procNetTCPStateCounts) halfOpenCount() int {
	return c.stateCount("03") + c.stateCount("0C")
}

func (c procNetTCPStateCounts) activeCount() int {
	total := 0
	for state, count := range c.states {
		if procNetTCPStateIsActive(state) {
			total += count
		}
	}
	return total
}

func readProcNetTCPStateCounts(path string) (procNetTCPStateCounts, error) {
	counts := procNetTCPStateCounts{states: make(map[string]int)}
	err := scanProcNetEntries(path, func(fields []string) {
		if len(fields) < 4 {
			return
		}
		counts.states[strings.ToUpper(strings.TrimSpace(fields[3]))]++
	})
	return counts, err
}

func mergeProcNetTCPStateCounts(items ...procNetTCPStateCounts) procNetTCPStateCounts {
	merged := procNetTCPStateCounts{states: make(map[string]int)}
	for _, item := range items {
		for state, count := range item.states {
			merged.states[state] += count
		}
	}
	return merged
}

func countProcNetTCPActiveConnections(path string) (int, error) {
	counts, err := readProcNetTCPStateCounts(path)
	if err != nil {
		return 0, err
	}
	return counts.activeCount(), nil
}

func countProcNetSocketEntries(path string) (int, error) {
	count := 0
	err := scanProcNetEntries(path, func(fields []string) {
		if len(fields) > 0 {
			count++
		}
	})
	return count, err
}

func scanProcNetEntries(path string, visit func(fields []string)) error {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	isHeader := true
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if isHeader {
			isHeader = false
			continue
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		visit(fields)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func procNetTCPStateIsActive(raw string) bool {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "01", "02", "03", "04", "05", "08", "09", "0B", "0C":
		return true
	default:
		return false
	}
}

func readProcNetProtocolStats(path string, prefix string) (map[string]uint64, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]uint64{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	headerKey := strings.TrimSpace(prefix) + ":"
	for scanner.Scan() {
		headerLine := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(headerLine, headerKey) {
			continue
		}
		if !scanner.Scan() {
			break
		}
		valueLine := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(valueLine, headerKey) {
			continue
		}

		headerFields := strings.Fields(headerLine)
		valueFields := strings.Fields(valueLine)
		if len(headerFields) <= 1 || len(valueFields) <= 1 {
			return map[string]uint64{}, nil
		}

		limit := len(headerFields)
		if len(valueFields) < limit {
			limit = len(valueFields)
		}
		result := make(map[string]uint64, limit-1)
		for index := 1; index < limit; index++ {
			value, parseErr := strconv.ParseUint(strings.TrimSpace(valueFields[index]), 10, 64)
			if parseErr != nil {
				continue
			}
			result[strings.TrimSpace(headerFields[index])] = value
		}
		return result, nil
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return map[string]uint64{}, nil
}
