package ddns

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SpaceshipProvider struct{}

func init() {
	RegisterProvider(&SpaceshipProvider{})
}

func (s *SpaceshipProvider) Code() string { return "dns_spaceship" }
func (s *SpaceshipProvider) Name() string { return "Spaceship" }

type spaceshipRecordItem struct {
	Type       string `json:"type"`
	Name       string `json:"name"`
	Value      string `json:"value,omitempty"`
	Address    string `json:"address,omitempty"`
	Cname      string `json:"cname,omitempty"`
	Exchange   string `json:"exchange,omitempty"`
	Preference int    `json:"preference,omitempty"`
	Text       string `json:"text,omitempty"`
	Target     string `json:"target,omitempty"`
	TTL        int    `json:"ttl,omitempty"`
}

type spaceshipListResponse struct {
	Items []spaceshipRecordItem `json:"items"`
	Total int                   `json:"total"`
}

func (s *SpaceshipProvider) getCredentials(env map[string]string) (apiKey, apiSecret, rootDomain string, err error) {
	apiKey = strings.TrimSpace(env["SPACESHIP_API_KEY"])
	apiSecret = strings.TrimSpace(env["SPACESHIP_API_SECRET"])
	rootDomain = strings.TrimSpace(env["SPACESHIP_ROOT_DOMAIN"])
	if apiKey == "" || apiSecret == "" {
		return "", "", "", errors.New("missing Spaceship API Key or Secret (SPACESHIP_API_KEY, SPACESHIP_API_SECRET)")
	}
	return apiKey, apiSecret, rootDomain, nil
}

func (s *SpaceshipProvider) resolveDomain(ctx context.Context, apiKey, apiSecret, fullDomain, manualRoot string) (subDomain, rootDomain string, err error) {
	fullDomain = strings.TrimSpace(strings.TrimSuffix(fullDomain, "."))
	if manualRoot != "" {
		rootDomain = strings.TrimSpace(strings.TrimSuffix(manualRoot, "."))
		if strings.EqualFold(fullDomain, rootDomain) {
			subDomain = "@"
		} else if strings.HasSuffix(strings.ToLower(fullDomain), "."+strings.ToLower(rootDomain)) {
			subDomain = fullDomain[:len(fullDomain)-len(rootDomain)-1]
		} else {
			subDomain = fullDomain
		}
		return subDomain, rootDomain, nil
	}

	// 尝试自动探测 Spaceship 上的有效根域名 Zone
	parts := strings.Split(fullDomain, ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid domain name: %s", fullDomain)
	}

	// 从较长的可能根域名开始尝试逐级查找有效 zone
	for i := 0; i < len(parts)-1; i++ {
		candidateRoot := strings.Join(parts[i:], ".")
		checkURL := fmt.Sprintf("https://spaceship.dev/api/v1/dns/records/%s?take=1&skip=0", candidateRoot)
		req, reqErr := http.NewRequestWithContext(ctx, "GET", checkURL, nil)
		if reqErr != nil {
			continue
		}
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("X-API-Secret", apiSecret)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 8 * time.Second}
		resp, doErr := client.Do(req)
		if doErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				rootDomain = candidateRoot
				if i == 0 {
					subDomain = "@"
				} else {
					subDomain = strings.Join(parts[:i], ".")
				}
				return subDomain, rootDomain, nil
			}
		}
	}

	// 回退到通用规则
	sub, root, splitErr := SplitDomain(fullDomain)
	if splitErr != nil {
		return "", "", splitErr
	}
	return sub, root, nil
}

func (s *SpaceshipProvider) TestAuth(ctx context.Context, env map[string]string) error {
	apiKey, apiSecret, rootDomain, err := s.getCredentials(env)
	if err != nil {
		return err
	}

	testZone := rootDomain
	if testZone == "" {
		testZone = "test.com"
	}
	url := fmt.Sprintf("https://spaceship.dev/api/v1/dns/records/%s?take=1&skip=0", testZone)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-API-Secret", apiSecret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connect Spaceship failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Spaceship authentication failed (HTTP %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (s *SpaceshipProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	apiKey, apiSecret, manualRoot, err := s.getCredentials(env)
	if err != nil {
		return "", err
	}

	subDomain, rootDomain, err := s.resolveDomain(ctx, apiKey, apiSecret, param.Domain, manualRoot)
	if err != nil {
		return "", err
	}

	ttl := param.TTL
	if ttl <= 0 {
		ttl = 600
	}

	recordName := subDomain
	if recordName == "@" {
		recordName = ""
	}

	item := map[string]any{
		"type": strings.ToUpper(param.Type),
		"name": recordName,
		"ttl":  ttl,
	}
	switch strings.ToUpper(param.Type) {
	case "A", "AAAA":
		item["address"] = param.IP
		item["value"] = param.IP
	case "CNAME":
		item["cname"] = param.IP
		item["value"] = param.IP
	case "MX":
		item["exchange"] = param.IP
		item["value"] = param.IP
	case "TXT":
		item["text"] = param.IP
		item["value"] = param.IP
	default:
		item["value"] = param.IP
		item["address"] = param.IP
	}

	payload := map[string]any{
		"force": true,
		"items": []any{item},
	}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://spaceship.dev/api/v1/dns/records/%s", rootDomain)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-API-Secret", apiSecret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Spaceship request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return "", fmt.Errorf("Spaceship DNS update error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	return fmt.Sprintf("spaceship-%s-%s", rootDomain, subDomain), nil
}

func (s *SpaceshipProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	apiKey, apiSecret, manualRoot, err := s.getCredentials(env)
	if err != nil {
		return err
	}

	subDomain, rootDomain, err := s.resolveDomain(ctx, apiKey, apiSecret, param.Domain, manualRoot)
	if err != nil {
		return err
	}

	// 1. 先查询该 rootDomain 下是否存在匹配的解析记录
	queryURL := fmt.Sprintf("https://spaceship.dev/api/v1/dns/records/%s?take=100&skip=0", rootDomain)
	qReq, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return err
	}
	qReq.Header.Set("X-API-Key", apiKey)
	qReq.Header.Set("X-API-Secret", apiSecret)
	qReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 20 * time.Second}
	qResp, err := client.Do(qReq)
	if err != nil {
		return fmt.Errorf("failed to query Spaceship DNS records: %w", err)
	}
	defer qResp.Body.Close()

	if qResp.StatusCode == 404 {
		// 根域名 Zone 不存在，远端自然无该记录
		return nil
	}
	if qResp.StatusCode != 200 {
		body, _ := io.ReadAll(qResp.Body)
		return fmt.Errorf("query Spaceship records failed (HTTP %d): %s", qResp.StatusCode, string(body))
	}

	var listResp spaceshipListResponse
	bodyBytes, _ := io.ReadAll(qResp.Body)
	_ = json.Unmarshal(bodyBytes, &listResp)

	// 筛选出匹配类型和子域名的记录
	var toDelete []map[string]any
	for _, item := range listResp.Items {
		nameMatches := strings.EqualFold(item.Name, subDomain) ||
			(subDomain == "@" && item.Name == "") ||
			(subDomain == "" && item.Name == "@")
		if strings.EqualFold(item.Type, param.Type) && nameMatches {
			val := item.Value
			if val == "" {
				val = item.Address
			}
			if val == "" {
				val = item.Cname
			}
			if val == "" {
				val = item.Exchange
			}
			if val == "" {
				val = item.Text
			}
			delName := item.Name
			if delName == "@" {
				delName = ""
			}
			delItem := map[string]any{
				"type":  item.Type,
				"name":  delName,
				"value": val,
			}
			switch strings.ToUpper(item.Type) {
			case "A", "AAAA":
				delItem["address"] = val
			case "CNAME":
				delItem["cname"] = val
			case "MX":
				delItem["exchange"] = val
				if item.Preference > 0 {
					delItem["preference"] = item.Preference
				}
			case "TXT":
				delItem["text"] = val
			}
			toDelete = append(toDelete, delItem)
		}
	}

	// 如果没有找到任何匹配记录，或者没有需要删除的，远端已经干净，直接返回成功
	if len(toDelete) == 0 {
		return nil
	}

	// 2. 调用 DELETE 接口删除匹配记录
	delBody, err := json.Marshal(toDelete)
	if err != nil {
		return err
	}

	delURL := fmt.Sprintf("https://spaceship.dev/api/v1/dns/records/%s", rootDomain)
	delReq, err := http.NewRequestWithContext(ctx, "DELETE", delURL, bytes.NewReader(delBody))
	if err != nil {
		return err
	}
	delReq.Header.Set("X-API-Key", apiKey)
	delReq.Header.Set("X-API-Secret", apiSecret)
	delReq.Header.Set("Content-Type", "application/json")

	delResp, err := client.Do(delReq)
	if err != nil {
		return fmt.Errorf("Spaceship delete request failed: %w", err)
	}
	defer delResp.Body.Close()

	delRespBody, _ := io.ReadAll(delResp.Body)
	if delResp.StatusCode != 200 && delResp.StatusCode != 204 {
		return fmt.Errorf("Spaceship delete record failed (HTTP %d): %s", delResp.StatusCode, string(delRespBody))
	}

	return nil
}

func (s *SpaceshipProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	apiKey, apiSecret, _, err := s.getCredentials(env)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://spaceship.dev/api/v1/dns/records/%s?take=100&skip=0", rootDomain)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-API-Secret", apiSecret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Spaceship HTTP %d: %s", resp.StatusCode, string(body))
	}

	var listResp spaceshipListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, err
	}

	result := make([]DNSRecord, 0, len(listResp.Items))
	for _, item := range listResp.Items {
		val := item.Value
		if val == "" {
			val = item.Address
		}
		if val == "" {
			val = item.Cname
		}
		if val == "" {
			val = item.Exchange
		}
		if val == "" {
			val = item.Text
		}
		if val == "" {
			val = item.Target
		}
		name := item.Name
		if name == "" {
			name = "@"
		}
		compositeID := fmt.Sprintf("%s|%s|%s", item.Type, name, val)
		result = append(result, DNSRecord{
			ID:       compositeID,
			Name:     name,
			Type:     item.Type,
			Value:    val,
			TTL:      item.TTL,
			Priority: item.Preference,
		})
	}
	return result, nil
}

func (s *SpaceshipProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	apiKey, apiSecret, _, err := s.getCredentials(env)
	if err != nil {
		return "", err
	}

	name := record.Name
	if name == "" || name == "@" {
		name = ""
	}
	ttl := record.TTL
	if ttl <= 0 {
		ttl = 300
	}

	item := map[string]any{
		"type": strings.ToUpper(record.Type),
		"name": name,
		"ttl":  ttl,
	}
	switch strings.ToUpper(record.Type) {
	case "A", "AAAA":
		item["address"] = record.Value
		item["value"] = record.Value
	case "CNAME":
		item["cname"] = record.Value
		item["value"] = record.Value
	case "MX":
		item["exchange"] = record.Value
		item["value"] = record.Value
		if record.Priority > 0 {
			item["preference"] = record.Priority
		}
	case "TXT":
		item["value"] = record.Value
		item["text"] = record.Value
	default:
		item["value"] = record.Value
	}

	putURL := fmt.Sprintf("https://spaceship.dev/api/v1/dns/records/%s", rootDomain)
	payload := map[string]any{
		"force": true,
		"items": []any{item},
	}
	putBody, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PUT", putURL, bytes.NewReader(putBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-API-Secret", apiSecret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Spaceship create record failed (HTTP %d): %s", resp.StatusCode, string(b))
	}
	displayName := record.Name
	if displayName == "" {
		displayName = "@"
	}
	return fmt.Sprintf("%s|%s|%s", strings.ToUpper(record.Type), displayName, record.Value), nil
}

func (s *SpaceshipProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	if record.ID != "" && strings.Contains(record.ID, "|") {
		_ = s.DeleteRecordByID(ctx, env, rootDomain, record.ID)
	}
	_, err := s.CreateRecord(ctx, env, rootDomain, record)
	return err
}

func (s *SpaceshipProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	apiKey, apiSecret, _, err := s.getCredentials(env)
	if err != nil {
		return err
	}

	parts := strings.SplitN(recordID, "|", 3)
	if len(parts) < 3 {
		return errors.New("invalid record composite ID")
	}
	delName := parts[1]
	if delName == "@" {
		delName = ""
	}
	delItem := map[string]any{
		"type":  parts[0],
		"name":  delName,
		"value": parts[2],
	}
	if parts[0] == "A" || parts[0] == "AAAA" {
		delItem["address"] = parts[2]
	} else if parts[0] == "CNAME" {
		delItem["cname"] = parts[2]
	} else if parts[0] == "MX" {
		delItem["exchange"] = parts[2]
	} else if parts[0] == "TXT" {
		delItem["text"] = parts[2]
	}

	delURL := fmt.Sprintf("https://spaceship.dev/api/v1/dns/records/%s", rootDomain)
	delBody, _ := json.Marshal([]any{delItem})
	req, err := http.NewRequestWithContext(ctx, "DELETE", delURL, bytes.NewReader(delBody))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-API-Secret", apiSecret)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Spaceship delete record failed (HTTP %d): %s", resp.StatusCode, string(b))
	}
	return nil
}

func (s *SpaceshipProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	apiKey, apiSecret, rootDomain, err := s.getCredentials(env)
	if err != nil {
		return nil, err
	}

	url := "https://spaceship.dev/api/v1/domains?take=100&skip=0"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err == nil {
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("X-API-Secret", apiSecret)
		req.Header.Set("Content-Type", "application/json")

		client := &http.Client{Timeout: 10 * time.Second}
		resp, doErr := client.Do(req)
		if doErr == nil {
			defer resp.Body.Close()
			if resp.StatusCode == 200 {
				var res struct {
					Items []struct {
						Name string `json:"name"`
					} `json:"items"`
				}
				if jsonErr := json.NewDecoder(resp.Body).Decode(&res); jsonErr == nil && len(res.Items) > 0 {
					domains := make([]string, 0, len(res.Items))
					for _, item := range res.Items {
						d := strings.TrimSpace(item.Name)
						if d != "" {
							domains = append(domains, d)
						}
					}
					if len(domains) > 0 {
						return domains, nil
					}
				}
			} else if resp.StatusCode == 403 {
				if strings.TrimSpace(rootDomain) != "" {
					return []string{strings.TrimSpace(rootDomain)}, nil
				}
				return nil, errors.New("Spaceship API Key 缺少 domains:read 权限 (HTTP 403)，无法自动列出根域名，请在 Spaceship 控制台补勾该权限或手动输入域名")
			}
		}
	}

	if strings.TrimSpace(rootDomain) != "" {
		return []string{strings.TrimSpace(rootDomain)}, nil
	}
	return []string{}, nil
}
