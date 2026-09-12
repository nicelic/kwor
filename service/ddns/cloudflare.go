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

type CloudflareProvider struct {
	code string
}

func init() {
	RegisterProvider(&CloudflareProvider{code: "dns_cf"})
	RegisterProvider(&CloudflareProvider{code: "cloudflare"})
}

func (c *CloudflareProvider) Code() string {
	if c.code != "" {
		return c.code
	}
	return "dns_cf"
}

func (c *CloudflareProvider) Name() string {
	return "Cloudflare"
}

type cfResponse struct {
	Success  bool            `json:"success"`
	Errors   []cfError       `json:"errors"`
	Messages []string        `json:"messages"`
	Result   json.RawMessage `json:"result"`
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cfDNSRecord struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Content   string `json:"content"`
	Proxiable bool   `json:"proxiable"`
	Proxied   bool   `json:"proxied"`
	TTL       int    `json:"ttl"`
}

func (c *CloudflareProvider) doRequest(ctx context.Context, env map[string]string, method, url string, body any) (*cfResponse, error) {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	token := strings.TrimSpace(env["CF_Token"])
	key := strings.TrimSpace(env["CF_Key"])
	email := strings.TrimSpace(env["CF_Email"])

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else if key != "" && email != "" {
		req.Header.Set("X-Auth-Key", key)
		req.Header.Set("X-Auth-Email", email)
	} else {
		return nil, errors.New("missing Cloudflare credentials: provide either CF_Token or (CF_Key + CF_Email)")
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var cfResp cfResponse
	if err := json.Unmarshal(respBody, &cfResp); err != nil {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	if !cfResp.Success {
		var errMsgs []string
		for _, e := range cfResp.Errors {
			errMsgs = append(errMsgs, fmt.Sprintf("[%d] %s", e.Code, e.Message))
		}
		if len(errMsgs) == 0 {
			errMsgs = append(errMsgs, string(respBody))
		}
		return nil, errors.New("Cloudflare API error: " + strings.Join(errMsgs, "; "))
	}

	return &cfResp, nil
}

func (c *CloudflareProvider) TestAuth(ctx context.Context, env map[string]string) error {
	token := strings.TrimSpace(env["CF_Token"])
	if token != "" {
		resp, err := c.doRequest(ctx, env, "GET", "https://api.cloudflare.com/client/v4/user/tokens/verify", nil)
		if err != nil {
			return err
		}
		if !resp.Success {
			return errors.New("Cloudflare token verification failed")
		}
		return nil
	}

	// Global Key test
	key := strings.TrimSpace(env["CF_Key"])
	email := strings.TrimSpace(env["CF_Email"])
	if key != "" && email != "" {
		resp, err := c.doRequest(ctx, env, "GET", "https://api.cloudflare.com/client/v4/user", nil)
		if err != nil {
			return err
		}
		if !resp.Success {
			return errors.New("Cloudflare Global API Key verification failed")
		}
		return nil
	}

	return errors.New("missing Cloudflare credentials (CF_Token or CF_Key+CF_Email)")
}

func (c *CloudflareProvider) getZoneID(ctx context.Context, env map[string]string, customZoneID, rootDomain string) (string, error) {
	if strings.TrimSpace(customZoneID) != "" {
		return strings.TrimSpace(customZoneID), nil
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones?name=%s&status=active", rootDomain)
	resp, err := c.doRequest(ctx, env, "GET", url, nil)
	if err != nil {
		return "", err
	}

	var zones []cfZone
	if err := json.Unmarshal(resp.Result, &zones); err != nil {
		return "", err
	}
	if len(zones) == 0 {
		return "", fmt.Errorf("no active zone found for domain %s on Cloudflare", rootDomain)
	}
	return zones[0].ID, nil
}

func (c *CloudflareProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	_, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return "", err
	}

	zoneID, err := c.getZoneID(ctx, env, env["CF_Zone_ID"], rootDomain)
	if err != nil {
		return "", err
	}

	// 1. 查询是否已存在匹配记录
	queryURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?name=%s&type=%s", zoneID, param.Domain, param.Type)
	resp, err := c.doRequest(ctx, env, "GET", queryURL, nil)
	if err != nil {
		return "", err
	}

	var records []cfDNSRecord
	if err := json.Unmarshal(resp.Result, &records); err != nil {
		return "", err
	}

	ttl := param.TTL
	if ttl <= 0 {
		ttl = 1 // 1 for Cloudflare "Auto"
	}

	payload := map[string]any{
		"type":    param.Type,
		"name":    param.Domain,
		"content": param.IP,
		"ttl":     ttl,
		"proxied": param.Proxy,
	}

	for _, existing := range records {
		if existing.Content == param.IP {
			if existing.Proxied == param.Proxy {
				return existing.ID, nil
			}
			// 更新记录
			updateURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, existing.ID)
			putResp, err := c.doRequest(ctx, env, "PUT", updateURL, payload)
			if err != nil {
				return "", err
			}
			var updated cfDNSRecord
			_ = json.Unmarshal(putResp.Result, &updated)
			return updated.ID, nil
		}
	}

	// 不存在该 IP 的记录：新增记录（支持多 IP）
	createURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	postResp, err := c.doRequest(ctx, env, "POST", createURL, payload)
	if err != nil {
		return "", err
	}
	var created cfDNSRecord
	_ = json.Unmarshal(postResp.Result, &created)
	return created.ID, nil
}

func (c *CloudflareProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	_, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return err
	}

	zoneID, err := c.getZoneID(ctx, env, env["CF_Zone_ID"], rootDomain)
	if err != nil {
		return err
	}

	queryURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?name=%s&type=%s", zoneID, param.Domain, param.Type)
	resp, err := c.doRequest(ctx, env, "GET", queryURL, nil)
	if err != nil {
		return err
	}

	var records []cfDNSRecord
	if err := json.Unmarshal(resp.Result, &records); err != nil {
		return err
	}

	if len(records) == 0 {
		return nil
	}

	for _, r := range records {
		if param.IP != "" && r.Content != param.IP {
			continue
		}
		delURL := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, r.ID)
		if _, delErr := c.doRequest(ctx, env, "DELETE", delURL, nil); delErr != nil {
			return fmt.Errorf("Cloudflare delete record %s failed: %w", r.ID, delErr)
		}
	}

	return nil
}

func (c *CloudflareProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	zoneID, err := c.getZoneID(ctx, env, env["CF_Zone_ID"], rootDomain)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records?per_page=100", zoneID)
	resp, err := c.doRequest(ctx, env, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	var records []cfDNSRecord
	if err := json.Unmarshal(resp.Result, &records); err != nil {
		return nil, err
	}

	result := make([]DNSRecord, 0, len(records))
	for _, r := range records {
		name := r.Name
		if strings.EqualFold(name, rootDomain) {
			name = "@"
		} else if strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(rootDomain)) {
			name = name[:len(name)-len(rootDomain)-1]
		}
		result = append(result, DNSRecord{
			ID:      r.ID,
			Name:    name,
			Type:    r.Type,
			Value:   r.Content,
			TTL:     r.TTL,
			Proxied: r.Proxied,
		})
	}
	return result, nil
}

func (c *CloudflareProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	zoneID, err := c.getZoneID(ctx, env, env["CF_Zone_ID"], rootDomain)
	if err != nil {
		return "", err
	}

	fullName := record.Name
	if fullName == "@" || fullName == "" {
		fullName = rootDomain
	} else if !strings.HasSuffix(strings.ToLower(fullName), "."+strings.ToLower(rootDomain)) {
		fullName = fullName + "." + rootDomain
	}

	ttl := record.TTL
	if ttl <= 0 {
		ttl = 1 // 1 for auto in Cloudflare
	}

	payload := map[string]any{
		"type":    strings.ToUpper(record.Type),
		"name":    fullName,
		"content": record.Value,
		"ttl":     ttl,
		"proxied": record.Proxied,
	}
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		payload["priority"] = record.Priority
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", zoneID)
	resp, err := c.doRequest(ctx, env, "POST", url, payload)
	if err != nil {
		return "", err
	}

	var created cfDNSRecord
	_ = json.Unmarshal(resp.Result, &created)
	return created.ID, nil
}

func (c *CloudflareProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	zoneID, err := c.getZoneID(ctx, env, env["CF_Zone_ID"], rootDomain)
	if err != nil {
		return err
	}

	if record.ID == "" {
		return errors.New("missing record ID for update")
	}

	fullName := record.Name
	if fullName == "@" || fullName == "" {
		fullName = rootDomain
	} else if !strings.HasSuffix(strings.ToLower(fullName), "."+strings.ToLower(rootDomain)) {
		fullName = fullName + "." + rootDomain
	}

	ttl := record.TTL
	if ttl <= 0 {
		ttl = 1
	}

	payload := map[string]any{
		"type":    strings.ToUpper(record.Type),
		"name":    fullName,
		"content": record.Value,
		"ttl":     ttl,
		"proxied": record.Proxied,
	}
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		payload["priority"] = record.Priority
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, record.ID)
	_, err = c.doRequest(ctx, env, "PUT", url, payload)
	return err
}

func (c *CloudflareProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	zoneID, err := c.getZoneID(ctx, env, env["CF_Zone_ID"], rootDomain)
	if err != nil {
		return err
	}

	if recordID == "" {
		return errors.New("missing record ID")
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, recordID)
	_, err = c.doRequest(ctx, env, "DELETE", url, nil)
	return err
}

func (c *CloudflareProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	url := "https://api.cloudflare.com/client/v4/zones?status=active&per_page=50"
	resp, err := c.doRequest(ctx, env, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	var zones []cfZone
	if err := json.Unmarshal(resp.Result, &zones); err != nil {
		return nil, err
	}
	domains := make([]string, 0, len(zones))
	for _, z := range zones {
		if z.Name != "" {
			domains = append(domains, z.Name)
		}
	}
	return domains, nil
}
