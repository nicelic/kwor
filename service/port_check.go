package service

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	procTCP  = "/proc/net/tcp"
	procTCP6 = "/proc/net/tcp6"
	procUDP  = "/proc/net/udp"
	procUDP6 = "/proc/net/udp6"
)

// PortCheckService reads /proc socket tables and reports current port occupancy.
// Every read uses open -> read -> close to get a fresh snapshot.
type PortCheckService struct{}

type PortRangeCheckItem struct {
	ID    string `json:"id"`
	Tag   string `json:"tag"`
	Range string `json:"range"`
}

type PortCheckRequest struct {
	SinglePorts []int                `json:"single_ports"`
	UDPRanges   []PortRangeCheckItem `json:"udp_ranges"`
}

type SinglePortStatus struct {
	Port int  `json:"port"`
	TCP  bool `json:"tcp"`
	UDP  bool `json:"udp"`
}

type UDPRangeStatus struct {
	ID               string `json:"id"`
	Tag              string `json:"tag"`
	Input            string `json:"input"`
	Normalized       string `json:"normalized"`
	Valid            bool   `json:"valid"`
	Error            string `json:"error,omitempty"`
	CheckedPortCount int    `json:"checked_port_count"`
	OccupiedCount    int    `json:"occupied_count"`
	OccupiedPorts    []int  `json:"occupied_ports"`
}

type PortCheckResponse struct {
	Supported bool               `json:"supported"`
	CheckedAt int64              `json:"checked_at"`
	Single    []SinglePortStatus `json:"single"`
	UDPRanges []UDPRangeStatus   `json:"udp_ranges"`
}

type socketSnapshot struct {
	tcp map[int]struct{}
	udp map[int]struct{}
}

type portSpan struct {
	start int
	end   int
}

func (s *PortCheckService) Check(req PortCheckRequest) (*PortCheckResponse, error) {
	resp := &PortCheckResponse{
		Supported: runtime.GOOS == "linux",
		CheckedAt: time.Now().Unix(),
		Single:    make([]SinglePortStatus, 0, len(req.SinglePorts)),
		UDPRanges: make([]UDPRangeStatus, 0, len(req.UDPRanges)),
	}

	if !resp.Supported {
		resp.Single = buildUnsupportedSingles(req.SinglePorts)
		resp.UDPRanges = buildUnsupportedRanges(req.UDPRanges)
		return resp, nil
	}

	snapshot, err := readSocketSnapshot()
	if err != nil {
		return nil, err
	}

	for _, port := range req.SinglePorts {
		if port < 1 || port > 65535 {
			continue
		}
		_, tcpUsed := snapshot.tcp[port]
		_, udpUsed := snapshot.udp[port]
		resp.Single = append(resp.Single, SinglePortStatus{
			Port: port,
			TCP:  tcpUsed,
			UDP:  udpUsed,
		})
	}

	for _, item := range req.UDPRanges {
		status := UDPRangeStatus{
			ID:    item.ID,
			Tag:   item.Tag,
			Input: strings.TrimSpace(item.Range),
			Valid: false,
		}
		ranges, normalized, parseErr := parseStrictPortRanges(item.Range)
		if parseErr != nil {
			status.Error = parseErr.Error()
			resp.UDPRanges = append(resp.UDPRanges, status)
			continue
		}

		occupied := collectOccupiedPorts(snapshot.udp, ranges)
		status.Valid = true
		status.Normalized = normalized
		status.CheckedPortCount = countPorts(ranges)
		status.OccupiedCount = len(occupied)
		status.OccupiedPorts = occupied
		resp.UDPRanges = append(resp.UDPRanges, status)
	}

	return resp, nil
}

func buildUnsupportedSingles(ports []int) []SinglePortStatus {
	out := make([]SinglePortStatus, 0, len(ports))
	for _, port := range ports {
		if port < 1 || port > 65535 {
			continue
		}
		out = append(out, SinglePortStatus{
			Port: port,
			TCP:  false,
			UDP:  false,
		})
	}
	return out
}

func buildUnsupportedRanges(items []PortRangeCheckItem) []UDPRangeStatus {
	out := make([]UDPRangeStatus, 0, len(items))
	for _, item := range items {
		out = append(out, UDPRangeStatus{
			ID:    item.ID,
			Tag:   item.Tag,
			Input: strings.TrimSpace(item.Range),
			Valid: false,
			Error: "port check is only supported on Linux",
		})
	}
	return out
}

func readSocketSnapshot() (*socketSnapshot, error) {
	snapshot := &socketSnapshot{
		tcp: map[int]struct{}{},
		udp: map[int]struct{}{},
	}

	if err := readProcPorts(procTCP, snapshot.tcp, true); err != nil {
		return nil, fmt.Errorf("read %s: %w", procTCP, err)
	}
	if err := readProcPorts(procTCP6, snapshot.tcp, true); err != nil {
		return nil, fmt.Errorf("read %s: %w", procTCP6, err)
	}
	if err := readProcPorts(procUDP, snapshot.udp, false); err != nil {
		return nil, fmt.Errorf("read %s: %w", procUDP, err)
	}
	if err := readProcPorts(procUDP6, snapshot.udp, false); err != nil {
		return nil, fmt.Errorf("read %s: %w", procUDP6, err)
	}

	return snapshot, nil
}

func readProcPorts(path string, ports map[int]struct{}, tcpListenOnly bool) error {
	data, err := readFileFresh(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for i := 1; i < len(lines); i++ { // skip header
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		if tcpListenOnly && fields[3] != "0A" {
			continue
		}
		port, ok := parseLocalPortHex(fields[1])
		if !ok {
			continue
		}
		ports[port] = struct{}{}
	}
	return nil
}

func readFileFresh(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func parseLocalPortHex(localAddress string) (int, bool) {
	idx := strings.LastIndex(localAddress, ":")
	if idx < 0 || idx+1 >= len(localAddress) {
		return 0, false
	}
	p, err := strconv.ParseInt(localAddress[idx+1:], 16, 32)
	if err != nil {
		return 0, false
	}
	port := int(p)
	if port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

func parseStrictPortRanges(input string) ([]portSpan, string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return nil, "", fmt.Errorf("empty range")
	}

	raw = strings.ReplaceAll(raw, "\uFF0C", ",")
	parts := strings.Split(raw, ",")
	ranges := make([]portSpan, 0, len(parts))

	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			return nil, "", fmt.Errorf("invalid empty segment")
		}
		token = strings.ReplaceAll(token, " ", "")

		hasDash := strings.Contains(token, "-")
		hasColon := strings.Contains(token, ":")
		if hasDash && hasColon {
			return nil, "", fmt.Errorf("invalid segment: %s", token)
		}

		if hasDash || hasColon {
			var sep string
			if hasDash {
				sep = "-"
			} else {
				sep = ":"
			}
			seg := strings.Split(token, sep)
			if len(seg) != 2 {
				return nil, "", fmt.Errorf("invalid segment: %s", token)
			}
			start, startErr := strconv.Atoi(seg[0])
			end, endErr := strconv.Atoi(seg[1])
			if startErr != nil || endErr != nil {
				return nil, "", fmt.Errorf("invalid segment: %s", token)
			}
			if !validPort(start) || !validPort(end) || start > end {
				return nil, "", fmt.Errorf("invalid segment: %s", token)
			}
			ranges = append(ranges, portSpan{start: start, end: end})
			continue
		}

		port, err := strconv.Atoi(token)
		if err != nil || !validPort(port) {
			return nil, "", fmt.Errorf("invalid segment: %s", token)
		}
		ranges = append(ranges, portSpan{start: port, end: port})
	}

	merged := mergePortSpans(ranges)
	return merged, formatPortSpans(merged), nil
}

func mergePortSpans(ranges []portSpan) []portSpan {
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].start == ranges[j].start {
			return ranges[i].end < ranges[j].end
		}
		return ranges[i].start < ranges[j].start
	})

	merged := []portSpan{ranges[0]}
	for i := 1; i < len(ranges); i++ {
		last := &merged[len(merged)-1]
		cur := ranges[i]
		if cur.start <= last.end+1 {
			if cur.end > last.end {
				last.end = cur.end
			}
			continue
		}
		merged = append(merged, cur)
	}
	return merged
}

func formatPortSpans(ranges []portSpan) string {
	if len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r.start == r.end {
			parts = append(parts, strconv.Itoa(r.start))
		} else {
			parts = append(parts, fmt.Sprintf("%d:%d", r.start, r.end))
		}
	}
	return strings.Join(parts, ",")
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}

func collectOccupiedPorts(udpPorts map[int]struct{}, ranges []portSpan) []int {
	if len(ranges) == 0 {
		return nil
	}
	out := make([]int, 0)
	for _, r := range ranges {
		for port := r.start; port <= r.end; port++ {
			if _, ok := udpPorts[port]; ok {
				out = append(out, port)
			}
		}
	}
	return out
}

func countPorts(ranges []portSpan) int {
	total := 0
	for _, r := range ranges {
		total += r.end - r.start + 1
	}
	return total
}
