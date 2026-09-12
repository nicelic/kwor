package ddns

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type WebhookProvider struct{}

func init() {
	RegisterProvider(&WebhookProvider{})
}

func (w *WebhookProvider) Code() string {
	return "webhook"
}

func (w *WebhookProvider) Name() string {
	return "自定义 Webhook"
}

func replaceVariables(template string, param RecordParam) string {
	s := template
	s = strings.ReplaceAll(s, "#{ip}", param.IP)
	s = strings.ReplaceAll(s, "#{domain}", param.Domain)
	s = strings.ReplaceAll(s, "#{type}", param.Type)
	s = strings.ReplaceAll(s, "#{ttl}", fmt.Sprintf("%d", param.TTL))
	return s
}

func (w *WebhookProvider) TestAuth(ctx context.Context, env map[string]string) error {
	rawURL := strings.TrimSpace(env["Webhook_URL"])
	if rawURL == "" {
		return errors.New("missing Webhook URL")
	}
	return nil
}

func (w *WebhookProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	rawURL := strings.TrimSpace(env["Webhook_URL"])
	if rawURL == "" {
		return "", errors.New("missing Webhook URL")
	}

	finalURL := replaceVariables(rawURL, param)
	method := strings.ToUpper(strings.TrimSpace(env["Webhook_Method"]))
	if method != "POST" && method != "PUT" {
		method = "GET"
	}

	var bodyReader io.Reader
	if method == "POST" || method == "PUT" {
		rawBody := env["Webhook_Body"]
		if rawBody != "" {
			finalBody := replaceVariables(rawBody, param)
			bodyReader = strings.NewReader(finalBody)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, finalURL, bodyReader)
	if err != nil {
		return "", err
	}

	if method == "POST" || method == "PUT" {
		req.Header.Set("Content-Type", "application/json")
	}

	headersText := env["Webhook_Headers"]
	if headersText != "" {
		lines := strings.Split(headersText, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				req.Header.Set(strings.TrimSpace(parts[0]), replaceVariables(strings.TrimSpace(parts[1]), param))
			}
		}
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("webhook HTTP %d: %s", resp.StatusCode, string(body))
	}

	return "webhook-ok", nil
}

func (w *WebhookProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	return w.TestAuth(ctx, env)
}

func (w *WebhookProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	if err := w.TestAuth(ctx, env); err != nil {
		return nil, err
	}
	return []DNSRecord{
		{
			ID:    "webhook-" + rootDomain,
			Name:  "@",
			Type:  "A",
			Value: "webhook-endpoint",
			TTL:   300,
		},
	}, nil
}

func (w *WebhookProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	return w.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
}

func (w *WebhookProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	fullDomain := rootDomain
	if record.Name != "" && record.Name != "@" {
		fullDomain = record.Name + "." + rootDomain
	}
	_, err := w.SyncRecord(ctx, env, RecordParam{
		Domain: fullDomain,
		Type:   record.Type,
		IP:     record.Value,
		TTL:    record.TTL,
	})
	return err
}

func (w *WebhookProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	return w.DeleteRecord(ctx, env, RecordParam{Domain: rootDomain})
}

func (w *WebhookProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	return []string{}, nil
}
