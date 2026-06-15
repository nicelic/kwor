package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

var (
	singlePortsIndexedKey = regexp.MustCompile(`^single_ports\[(\d+)\]$`)
	udpRangesIndexedKey   = regexp.MustCompile(`^udp_ranges\[(\d+)\]\[(id|tag|range)\]$`)
	udpRangesDotKey       = regexp.MustCompile(`^udp_ranges\[(\d+)\]\.(id|tag|range)$`)
	udpRangesNoIndexKey   = regexp.MustCompile(`^udp_ranges\[\]\[(id|tag|range)\]$`)
)

func bindPortCheckRequest(c *gin.Context) (service.PortCheckRequest, error) {
	req := service.PortCheckRequest{}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return req, err
	}
	return parsePortCheckRequestBody(body)
}

func parsePortCheckRequestBody(body []byte) (service.PortCheckRequest, error) {
	req := service.PortCheckRequest{}
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return req, nil
	}

	jsonErr := json.Unmarshal(body, &req)
	if jsonErr == nil {
		req.SinglePorts = sanitizePortList(req.SinglePorts)
		req.UDPRanges = sanitizeUDPRangeItems(req.UDPRanges)
		return req, nil
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return req, fmt.Errorf("invalid port occupancy payload")
	}
	decoded, ok := decodePortCheckRequestValues(values)
	if !ok {
		return req, fmt.Errorf("invalid port occupancy payload")
	}
	return decoded, nil
}

func decodePortCheckRequestValues(values url.Values) (service.PortCheckRequest, bool) {
	req := service.PortCheckRequest{
		SinglePorts: make([]int, 0),
		UDPRanges:   make([]service.PortRangeCheckItem, 0),
	}
	parsed := false

	ports := make([]int, 0)
	for key, vals := range values {
		if key != "single_ports" && key != "single_ports[]" && !singlePortsIndexedKey.MatchString(key) {
			continue
		}
		for _, value := range vals {
			port, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				continue
			}
			ports = append(ports, port)
			parsed = true
		}
	}

	if len(ports) == 0 {
		for _, raw := range values["single_ports"] {
			maybe := make([]int, 0)
			if err := json.Unmarshal([]byte(raw), &maybe); err != nil {
				continue
			}
			ports = append(ports, maybe...)
			if len(maybe) > 0 {
				parsed = true
			}
		}
	}
	req.SinglePorts = sanitizePortList(ports)

	for _, raw := range values["udp_ranges"] {
		list := make([]service.PortRangeCheckItem, 0)
		if err := json.Unmarshal([]byte(raw), &list); err == nil && len(list) > 0 {
			req.UDPRanges = append(req.UDPRanges, list...)
			parsed = true
			continue
		}
		item := service.PortRangeCheckItem{}
		if err := json.Unmarshal([]byte(raw), &item); err == nil {
			req.UDPRanges = append(req.UDPRanges, item)
			parsed = true
		}
	}

	type partialRange struct {
		ID    string
		Tag   string
		Range string
	}

	indexed := map[int]*partialRange{}
	for key, vals := range values {
		match := udpRangesIndexedKey.FindStringSubmatch(key)
		if match == nil {
			match = udpRangesDotKey.FindStringSubmatch(key)
		}
		if match == nil {
			continue
		}

		index, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}

		current := indexed[index]
		if current == nil {
			current = &partialRange{}
			indexed[index] = current
		}
		parsed = true
		value := ""
		if len(vals) > 0 {
			value = vals[len(vals)-1]
		}
		switch match[2] {
		case "id":
			current.ID = value
		case "tag":
			current.Tag = value
		case "range":
			current.Range = value
		}
	}

	if len(indexed) > 0 {
		indexes := make([]int, 0, len(indexed))
		for index := range indexed {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		for _, index := range indexes {
			item := indexed[index]
			req.UDPRanges = append(req.UDPRanges, service.PortRangeCheckItem{
				ID:    item.ID,
				Tag:   item.Tag,
				Range: item.Range,
			})
		}
	}

	byField := map[string][]string{}
	for key, vals := range values {
		match := udpRangesNoIndexKey.FindStringSubmatch(key)
		if match == nil {
			continue
		}
		parsed = true
		byField[match[1]] = vals
	}

	if len(byField) > 0 {
		maxLen := 0
		for _, fieldValues := range byField {
			if len(fieldValues) > maxLen {
				maxLen = len(fieldValues)
			}
		}
		for i := 0; i < maxLen; i++ {
			item := service.PortRangeCheckItem{}
			if ids := byField["id"]; i < len(ids) {
				item.ID = ids[i]
			}
			if tags := byField["tag"]; i < len(tags) {
				item.Tag = tags[i]
			}
			if ranges := byField["range"]; i < len(ranges) {
				item.Range = ranges[i]
			}
			req.UDPRanges = append(req.UDPRanges, item)
		}
	}

	req.UDPRanges = sanitizeUDPRangeItems(req.UDPRanges)
	return req, parsed
}

func sanitizePortList(ports []int) []int {
	out := make([]int, 0, len(ports))
	seen := make(map[int]struct{}, len(ports))
	for _, port := range ports {
		if port < 1 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	return out
}

func sanitizeUDPRangeItems(items []service.PortRangeCheckItem) []service.PortRangeCheckItem {
	out := make([]service.PortRangeCheckItem, 0, len(items))
	for _, item := range items {
		item.ID = strings.TrimSpace(item.ID)
		item.Tag = strings.TrimSpace(item.Tag)
		item.Range = strings.TrimSpace(item.Range)
		if item.ID == "" && item.Tag == "" && item.Range == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}
