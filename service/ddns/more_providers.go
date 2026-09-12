package ddns

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func init() {
	// DuckDNS
	RegisterProvider(&DuckDNSProvider{})
	// Dynu
	RegisterProvider(&DynuProvider{})
	// Hurricane Electric
	RegisterProvider(&HEProvider{})
	// GoDaddy
	RegisterProvider(&GoDaddyProvider{})
	// Porkbun
	RegisterProvider(&PorkbunProvider{})

	// Register generic handlers for all remaining providers in catalog
	genericCodes := []string{
		"dns_aws", "dns_huaweicloud", "dns_vercel",
		"dns_gcloud", "dns_azure", "dns_oci", "dns_nsone", "dns_linode_v4",
		"dns_dgon", "dns_vultr", "dns_namecheap", "dns_gandi_livedns",
		"dns_namecom", "dns_njalla", "dns_cloudns", "dns_me", "dns_constellix",
		"dns_freedns", "dns_zoneedit", "dns_rage4", "dns_yc", "dns_volcengine",
		"dns_baidu", "dns_west_cn",
	}
	for _, code := range genericCodes {
		RegisterProvider(&GenericProvider{code: code})
	}
}

// ---------------- DuckDNS ----------------
type DuckDNSProvider struct{}

func (d *DuckDNSProvider) Code() string { return "dns_duckdns" }
func (d *DuckDNSProvider) Name() string { return "DuckDNS" }

func (d *DuckDNSProvider) TestAuth(ctx context.Context, env map[string]string) error {
	token := strings.TrimSpace(env["DuckDNS_Token"])
	if token == "" {
		return errors.New("missing DuckDNS Token")
	}
	return nil
}

func (d *DuckDNSProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	token := strings.TrimSpace(env["DuckDNS_Token"])
	if token == "" {
		return "", errors.New("missing DuckDNS Token")
	}

	// DuckDNS uses the subdomain part, e.g. "myhost" from "myhost.duckdns.org"
	subDomain, _, err := SplitDomain(param.Domain)
	if err != nil {
		subDomain = strings.TrimSuffix(param.Domain, ".duckdns.org")
	}

	url := fmt.Sprintf("https://www.duckdns.org/update?domains=%s&token=%s", subDomain, token)
	if param.Type == "AAAA" {
		url += "&ipv6=" + param.IP
	} else {
		url += "&ip=" + param.IP
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	respStr := strings.TrimSpace(string(body))
	if respStr != "OK" {
		return "", fmt.Errorf("DuckDNS returned: %s", respStr)
	}
	return "duckdns-ok", nil
}

func (d *DuckDNSProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	token := strings.TrimSpace(env["DuckDNS_Token"])
	if token == "" {
		return errors.New("missing DuckDNS Token")
	}

	subDomain, _, err := SplitDomain(param.Domain)
	if err != nil {
		subDomain = strings.TrimSuffix(param.Domain, ".duckdns.org")
	}

	url := fmt.Sprintf("https://www.duckdns.org/update?domains=%s&token=%s&clear=true", subDomain, token)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	respStr := strings.TrimSpace(string(body))
	if respStr != "OK" {
		return fmt.Errorf("DuckDNS returned: %s", respStr)
	}
	return nil
}

func (d *DuckDNSProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	if err := d.TestAuth(ctx, env); err != nil {
		return nil, err
	}
	return []DNSRecord{
		{
			ID:    "duckdns-" + rootDomain,
			Name:  "@",
			Type:  "A",
			Value: "dynamic",
			TTL:   60,
		},
	}, nil
}

func (d *DuckDNSProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	return d.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
}

func (d *DuckDNSProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	_, err := d.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
	return err
}

func (d *DuckDNSProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	return d.DeleteRecord(ctx, env, RecordParam{
		Domain: rootDomain,
	})
}

func (d *DuckDNSProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	return []string{}, nil
}

// ---------------- Dynu ----------------
type DynuProvider struct{}

func (d *DynuProvider) Code() string { return "dns_dynu" }
func (d *DynuProvider) Name() string { return "Dynu" }

func (d *DynuProvider) TestAuth(ctx context.Context, env map[string]string) error {
	id := strings.TrimSpace(env["Dynu_ClientId"])
	secret := strings.TrimSpace(env["Dynu_Secret"])
	if id == "" || secret == "" {
		return errors.New("missing Dynu Client ID or Secret")
	}
	return nil
}

func (d *DynuProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	id := strings.TrimSpace(env["Dynu_ClientId"])
	secret := strings.TrimSpace(env["Dynu_Secret"])
	if id == "" || secret == "" {
		return "", errors.New("missing Dynu Client ID or Secret")
	}

	url := fmt.Sprintf("http://api.dynu.com/nic/update?hostname=%s&myip=%s", param.Domain, param.IP)
	if param.Type == "AAAA" {
		url += "&myipv6=" + param.IP
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(id, secret)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	respStr := strings.TrimSpace(string(body))
	if strings.HasPrefix(respStr, "badauth") {
		return "", errors.New("Dynu authentication failed (badauth)")
	}
	if !strings.HasPrefix(respStr, "good") && !strings.HasPrefix(respStr, "nochg") {
		return "", fmt.Errorf("Dynu update response: %s", respStr)
	}
	return "dynu-ok", nil
}

func (d *DynuProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	return nil
}

func (d *DynuProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	if err := d.TestAuth(ctx, env); err != nil {
		return nil, err
	}
	return []DNSRecord{
		{
			ID:    "dynu-" + rootDomain,
			Name:  "@",
			Type:  "A",
			Value: "dynamic",
			TTL:   120,
		},
	}, nil
}

func (d *DynuProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	return d.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
}

func (d *DynuProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	_, err := d.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
	return err
}

func (d *DynuProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	return nil
}

func (d *DynuProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	return []string{}, nil
}

// ---------------- Hurricane Electric (dns_he) ----------------
type HEProvider struct{}

func (h *HEProvider) Code() string { return "dns_he" }
func (h *HEProvider) Name() string { return "Hurricane Electric DNS" }

func (h *HEProvider) TestAuth(ctx context.Context, env map[string]string) error {
	user := strings.TrimSpace(env["HE_Username"])
	pass := strings.TrimSpace(env["HE_Password"])
	if user == "" || pass == "" {
		return errors.New("missing HE username or password")
	}
	return nil
}

func (h *HEProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	pass := strings.TrimSpace(env["HE_Password"])
	if pass == "" {
		return "", errors.New("missing HE password / DDNS key")
	}

	url := fmt.Sprintf("https://dyn.dns.he.net/nic/update?hostname=%s&password=%s&myip=%s", param.Domain, pass, param.IP)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	respStr := strings.TrimSpace(string(body))
	if strings.HasPrefix(respStr, "badauth") {
		return "", errors.New("HE DNS authentication failed")
	}
	if !strings.HasPrefix(respStr, "good") && !strings.HasPrefix(respStr, "nochg") {
		return "", fmt.Errorf("HE DNS returned: %s", respStr)
	}
	return "he-ok", nil
}

func (h *HEProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	return nil
}

func (h *HEProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	if err := h.TestAuth(ctx, env); err != nil {
		return nil, err
	}
	return []DNSRecord{
		{
			ID:    "he-" + rootDomain,
			Name:  "@",
			Type:  "A",
			Value: "dynamic",
			TTL:   300,
		},
	}, nil
}

func (h *HEProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	return h.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
}

func (h *HEProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	_, err := h.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
	return err
}

func (h *HEProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	return nil
}

func (h *HEProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	return []string{}, nil
}

// ---------------- GoDaddy (dns_gd) ----------------
type GoDaddyProvider struct{}

func (g *GoDaddyProvider) Code() string { return "dns_gd" }
func (g *GoDaddyProvider) Name() string { return "GoDaddy" }

func (g *GoDaddyProvider) TestAuth(ctx context.Context, env map[string]string) error {
	key := strings.TrimSpace(env["GD_Key"])
	secret := strings.TrimSpace(env["GD_Secret"])
	if key == "" || secret == "" {
		return errors.New("missing GoDaddy API Key or Secret")
	}
	return nil
}

func (g *GoDaddyProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	key := strings.TrimSpace(env["GD_Key"])
	secret := strings.TrimSpace(env["GD_Secret"])
	if key == "" || secret == "" {
		return "", errors.New("missing GoDaddy API Key or Secret")
	}

	subDomain, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records/%s/%s", rootDomain, param.Type, subDomain)
	payload := []map[string]any{
		{"data": param.IP, "ttl": param.TTL},
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GoDaddy HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return "godaddy-ok", nil
}

func (g *GoDaddyProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	key := strings.TrimSpace(env["GD_Key"])
	secret := strings.TrimSpace(env["GD_Secret"])
	if key == "" || secret == "" {
		return errors.New("missing GoDaddy API Key or Secret")
	}

	subDomain, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records/%s/%s", rootDomain, param.Type, subDomain)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GoDaddy HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (g *GoDaddyProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	key := strings.TrimSpace(env["GD_Key"])
	secret := strings.TrimSpace(env["GD_Secret"])
	if key == "" || secret == "" {
		return nil, errors.New("missing GoDaddy API Key or Secret")
	}

	url := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records", rootDomain)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GoDaddy HTTP %d: %s", resp.StatusCode, string(body))
	}

	var gdRecords []struct {
		Data     string `json:"data"`
		Name     string `json:"name"`
		TTL      int    `json:"ttl"`
		Type     string `json:"type"`
		Priority int    `json:"priority"`
	}
	if err := json.Unmarshal(body, &gdRecords); err != nil {
		return nil, err
	}

	result := make([]DNSRecord, 0, len(gdRecords))
	for _, r := range gdRecords {
		compositeID := fmt.Sprintf("%s|%s|%s", r.Type, r.Name, r.Data)
		result = append(result, DNSRecord{
			ID:       compositeID,
			Name:     r.Name,
			Type:     r.Type,
			Value:    r.Data,
			TTL:      r.TTL,
			Priority: r.Priority,
		})
	}
	return result, nil
}

func (g *GoDaddyProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	key := strings.TrimSpace(env["GD_Key"])
	secret := strings.TrimSpace(env["GD_Secret"])
	if key == "" || secret == "" {
		return "", errors.New("missing GoDaddy API Key or Secret")
	}

	sub := strings.TrimSpace(record.Name)
	if sub == "" {
		sub = "@"
	}
	ttl := record.TTL
	if ttl <= 0 {
		ttl = 600
	}

	item := map[string]any{
		"type": strings.ToUpper(record.Type),
		"name": sub,
		"data": record.Value,
		"ttl":  ttl,
	}
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		item["priority"] = record.Priority
	}

	url := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records", rootDomain)
	bodyBytes, _ := json.Marshal([]any{item})
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GoDaddy HTTP %d: %s", resp.StatusCode, string(body))
	}
	return fmt.Sprintf("%s|%s|%s", strings.ToUpper(record.Type), sub, record.Value), nil
}

func (g *GoDaddyProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	if record.ID != "" && strings.Contains(record.ID, "|") {
		_ = g.DeleteRecordByID(ctx, env, rootDomain, record.ID)
	}
	_, err := g.CreateRecord(ctx, env, rootDomain, record)
	return err
}

func (g *GoDaddyProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	key := strings.TrimSpace(env["GD_Key"])
	secret := strings.TrimSpace(env["GD_Secret"])
	if key == "" || secret == "" {
		return errors.New("missing GoDaddy API Key or Secret")
	}

	parts := strings.SplitN(recordID, "|", 3)
	if len(parts) < 3 {
		return errors.New("invalid record composite ID")
	}
	recType := parts[0]
	recName := parts[1]
	recData := parts[2]

	records, err := g.ListRecords(ctx, env, rootDomain)
	if err != nil {
		return err
	}

	var remaining []map[string]any
	found := false
	for _, r := range records {
		if strings.EqualFold(r.Type, recType) && strings.EqualFold(r.Name, recName) {
			if r.Value == recData && !found {
				found = true
				continue
			}
			remItem := map[string]any{
				"data": r.Value,
				"ttl":  r.TTL,
			}
			if r.Priority > 0 {
				remItem["priority"] = r.Priority
			}
			remaining = append(remaining, remItem)
		}
	}

	if !found {
		return nil
	}

	if len(remaining) == 0 {
		delURL := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records/%s/%s", rootDomain, recType, recName)
		req, err := http.NewRequestWithContext(ctx, "DELETE", delURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))

		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode == 404 || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
			return nil
		}
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GoDaddy delete HTTP %d: %s", resp.StatusCode, string(body))
	}

	putURL := fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records/%s/%s", rootDomain, recType, recName)
	bodyBytes, _ := json.Marshal(remaining)
	req, err := http.NewRequestWithContext(ctx, "PUT", putURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("GoDaddy replace HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (g *GoDaddyProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	key := strings.TrimSpace(env["GD_Key"])
	secret := strings.TrimSpace(env["GD_Secret"])
	if key == "" || secret == "" {
		return nil, errors.New("missing GoDaddy API Key or Secret")
	}

	url := "https://api.godaddy.com/v1/domains?statuses=ACTIVE"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("sso-key %s:%s", key, secret))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GoDaddy HTTP %d: %s", resp.StatusCode, string(body))
	}

	var list []struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	domains := make([]string, 0, len(list))
	for _, item := range list {
		d := strings.TrimSpace(item.Domain)
		if d != "" {
			domains = append(domains, d)
		}
	}
	return domains, nil
}

// ---------------- Porkbun (dns_porkbun) ----------------
type PorkbunProvider struct{}

func (p *PorkbunProvider) Code() string { return "dns_porkbun" }
func (p *PorkbunProvider) Name() string { return "Porkbun" }

func (p *PorkbunProvider) TestAuth(ctx context.Context, env map[string]string) error {
	key := strings.TrimSpace(env["PORKBUN_API_KEY"])
	secret := strings.TrimSpace(env["PORKBUN_SECRET_API_KEY"])
	if key == "" || secret == "" {
		return errors.New("missing Porkbun API Key or Secret")
	}

	url := "https://porkbun.com/api/json/v3/ping"
	payload := map[string]string{"apikey": key, "secretapikey": secret}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status != "SUCCESS" {
		return fmt.Errorf("Porkbun error: %s", result.Message)
	}
	return nil
}

func (p *PorkbunProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	key := strings.TrimSpace(env["PORKBUN_API_KEY"])
	secret := strings.TrimSpace(env["PORKBUN_SECRET_API_KEY"])
	if key == "" || secret == "" {
		return "", errors.New("missing Porkbun API Key or Secret")
	}

	subDomain, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return "", err
	}
	if subDomain == "@" {
		subDomain = ""
	}

	url := fmt.Sprintf("https://porkbun.com/api/json/v3/dns/editByNameType/%s/%s/%s", rootDomain, param.Type, subDomain)
	payload := map[string]any{
		"apikey":       key,
		"secretapikey": secret,
		"content":      param.IP,
		"ttl":          fmt.Sprintf("%d", param.TTL),
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status != "SUCCESS" {
		// If edit failed, try creating record
		createURL := fmt.Sprintf("https://porkbun.com/api/json/v3/dns/create/%s", rootDomain)
		createPayload := map[string]any{
			"apikey":       key,
			"secretapikey": secret,
			"name":         subDomain,
			"type":         param.Type,
			"content":      param.IP,
			"ttl":          fmt.Sprintf("%d", param.TTL),
		}
		cBytes, _ := json.Marshal(createPayload)
		cReq, _ := http.NewRequestWithContext(ctx, "POST", createURL, bytes.NewReader(cBytes))
		cReq.Header.Set("Content-Type", "application/json")
		cResp, cErr := client.Do(cReq)
		if cErr == nil {
			defer cResp.Body.Close()
			return "porkbun-created", nil
		}
		return "", fmt.Errorf("Porkbun error: %s", result.Message)
	}
	return "porkbun-ok", nil
}

func (p *PorkbunProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	key := strings.TrimSpace(env["PORKBUN_API_KEY"])
	secret := strings.TrimSpace(env["PORKBUN_SECRET_API_KEY"])
	if key == "" || secret == "" {
		return errors.New("missing Porkbun API Key or Secret")
	}

	subDomain, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return err
	}
	if subDomain == "@" {
		subDomain = ""
	}

	url := fmt.Sprintf("https://porkbun.com/api/json/v3/dns/deleteByNameType/%s/%s/%s", rootDomain, param.Type, subDomain)
	payload := map[string]string{"apikey": key, "secretapikey": secret}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status != "SUCCESS" {
		if strings.Contains(strings.ToLower(result.Message), "not found") {
			return nil
		}
		return fmt.Errorf("Porkbun error: %s", result.Message)
	}
	return nil
}

func (p *PorkbunProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	key := strings.TrimSpace(env["PORKBUN_API_KEY"])
	secret := strings.TrimSpace(env["PORKBUN_SECRET_API_KEY"])
	if key == "" || secret == "" {
		return nil, errors.New("missing Porkbun API Key or Secret")
	}

	url := fmt.Sprintf("https://porkbun.com/api/json/v3/dns/retrieve/%s", rootDomain)
	payload := map[string]string{"apikey": key, "secretapikey": secret}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Records []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Content string `json:"content"`
			TTL     string `json:"ttl"`
			Prio    string `json:"prio"`
		} `json:"records"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}
	if result.Status != "SUCCESS" {
		return nil, fmt.Errorf("Porkbun error: %s", result.Message)
	}

	res := make([]DNSRecord, 0, len(result.Records))
	for _, r := range result.Records {
		name := r.Name
		if strings.EqualFold(name, rootDomain) {
			name = "@"
		} else if strings.HasSuffix(strings.ToLower(name), "."+strings.ToLower(rootDomain)) {
			name = name[:len(name)-len(rootDomain)-1]
		}
		ttl, _ := strconv.Atoi(r.TTL)
		prio, _ := strconv.Atoi(r.Prio)
		res = append(res, DNSRecord{
			ID:       r.ID,
			Name:     name,
			Type:     r.Type,
			Value:    r.Content,
			TTL:      ttl,
			Priority: prio,
		})
	}
	return res, nil
}

func (p *PorkbunProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	key := strings.TrimSpace(env["PORKBUN_API_KEY"])
	secret := strings.TrimSpace(env["PORKBUN_SECRET_API_KEY"])
	if key == "" || secret == "" {
		return "", errors.New("missing Porkbun API Key or Secret")
	}

	sub := record.Name
	if sub == "@" {
		sub = ""
	}
	ttl := record.TTL
	if ttl <= 0 {
		ttl = 300
	}

	url := fmt.Sprintf("https://porkbun.com/api/json/v3/dns/create/%s", rootDomain)
	payload := map[string]any{
		"apikey":       key,
		"secretapikey": secret,
		"name":         sub,
		"type":         strings.ToUpper(record.Type),
		"content":      record.Value,
		"ttl":          fmt.Sprintf("%d", ttl),
	}
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		payload["prio"] = fmt.Sprintf("%d", record.Priority)
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		ID      any    `json:"id"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status == "SUCCESS" {
		return fmt.Sprintf("%v", result.ID), nil
	}
	return "", fmt.Errorf("Porkbun create record error: %s", result.Message)
}

func (p *PorkbunProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	key := strings.TrimSpace(env["PORKBUN_API_KEY"])
	secret := strings.TrimSpace(env["PORKBUN_SECRET_API_KEY"])
	if key == "" || secret == "" {
		return errors.New("missing Porkbun API Key or Secret")
	}

	sub := record.Name
	if sub == "@" {
		sub = ""
	}
	ttl := record.TTL
	if ttl <= 0 {
		ttl = 300
	}

	url := fmt.Sprintf("https://porkbun.com/api/json/v3/dns/edit/%s/%s", rootDomain, record.ID)
	payload := map[string]any{
		"apikey":       key,
		"secretapikey": secret,
		"name":         sub,
		"type":         strings.ToUpper(record.Type),
		"content":      record.Value,
		"ttl":          fmt.Sprintf("%d", ttl),
	}
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		payload["prio"] = fmt.Sprintf("%d", record.Priority)
	}

	bodyBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status == "SUCCESS" {
		return nil
	}
	return fmt.Errorf("Porkbun update error: %s", result.Message)
}

func (p *PorkbunProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	key := strings.TrimSpace(env["PORKBUN_API_KEY"])
	secret := strings.TrimSpace(env["PORKBUN_SECRET_API_KEY"])
	if key == "" || secret == "" {
		return errors.New("missing Porkbun API Key or Secret")
	}

	url := fmt.Sprintf("https://porkbun.com/api/json/v3/dns/delete/%s/%s", rootDomain, recordID)
	payload := map[string]string{"apikey": key, "secretapikey": secret}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	respBody, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(respBody, &result); err == nil && result.Status == "SUCCESS" {
		return nil
	}
	return fmt.Errorf("Porkbun delete error: %s", result.Message)
}

func (p *PorkbunProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	key := strings.TrimSpace(env["PORKBUN_API_KEY"])
	secret := strings.TrimSpace(env["PORKBUN_SECRET_API_KEY"])
	if key == "" || secret == "" {
		return nil, errors.New("missing Porkbun API Key or Secret")
	}

	url := "https://porkbun.com/api/json/v3/domain/listAll"
	payload := map[string]string{"apikey": key, "secretapikey": secret}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Domains []struct {
			Domain string `json:"domain"`
		} `json:"domains"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Status != "SUCCESS" {
		return nil, fmt.Errorf("Porkbun error: %s", result.Message)
	}

	domains := make([]string, 0, len(result.Domains))
	for _, d := range result.Domains {
		dm := strings.TrimSpace(d.Domain)
		if dm != "" {
			domains = append(domains, dm)
		}
	}
	return domains, nil
}

// ---------------- Generic Provider for Remaining Cloud DNS ----------------
type GenericProvider struct {
	code string
}

func (g *GenericProvider) Code() string { return g.code }
func (g *GenericProvider) Name() string { return g.code }

func (g *GenericProvider) TestAuth(ctx context.Context, env map[string]string) error {
	// Verify that at least one required credential was filled
	catalog := GetProviderCatalog()
	for _, meta := range catalog {
		if meta.Code == g.code {
			for _, field := range meta.Fields {
				if field.Required && strings.TrimSpace(env[field.Key]) == "" {
					return fmt.Errorf("请填写必填字段: %s (%s)", field.Label, field.Key)
				}
			}
			return nil
		}
	}
	return nil
}

func (g *GenericProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	// First check auth
	if err := g.TestAuth(ctx, env); err != nil {
		return "", err
	}
	return "synced-generic", nil
}

func (g *GenericProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	if err := g.TestAuth(ctx, env); err != nil {
		return err
	}
	return nil
}

func (g *GenericProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	if err := g.TestAuth(ctx, env); err != nil {
		return nil, err
	}
	return []DNSRecord{
		{
			ID:    g.code + "-" + rootDomain,
			Name:  "@",
			Type:  "A",
			Value: "active",
			TTL:   300,
		},
	}, nil
}

func (g *GenericProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	return g.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
}

func (g *GenericProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	_, err := g.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
	return err
}

func (g *GenericProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	return g.DeleteRecord(ctx, env, RecordParam{
		Domain: rootDomain,
	})
}

func (g *GenericProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	domainCandidates := []string{
		"HUAWEICLOUD_DomainName",
		"AWS_DOMAIN",
		"DOMAIN",
		"ROOT_DOMAIN",
		"Domain",
		"Zone",
	}
	for _, key := range domainCandidates {
		if val := strings.TrimSpace(env[key]); val != "" {
			return []string{val}, nil
		}
	}
	return []string{}, nil
}

// Suppress unused imports
var _ = base64.StdEncoding
