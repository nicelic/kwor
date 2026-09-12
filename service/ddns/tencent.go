package ddns

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type TencentProvider struct {
	code string
}

func init() {
	RegisterProvider(&TencentProvider{code: "dns_tencent"})
	RegisterProvider(&TencentProvider{code: "tencent"})
}

func (t *TencentProvider) Code() string {
	if t.code != "" {
		return t.code
	}
	return "dns_tencent"
}

func (t *TencentProvider) Name() string {
	return "腾讯云 DNSPod"
}

func sha256Hex(s string) string {
	b := sha256.Sum256([]byte(s))
	return hex.EncodeToString(b[:])
}

func hmacSha256(key []byte, s string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(s))
	return h.Sum(nil)
}

func (t *TencentProvider) doRequest(ctx context.Context, secretId, secretKey, action string, payload map[string]any, target any) error {
	service := "dnspod"
	host := "dnspod.tencentcloudapi.com"
	version := "2021-03-23"
	httpMethod := "POST"
	canonicalURI := "/"
	canonicalQueryString := ""

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	payloadStr := string(bodyBytes)

	now := time.Now().UTC()
	timestamp := now.Unix()
	date := now.Format("2006-01-02")

	// 1. Canonical request
	canonicalHeaders := fmt.Sprintf("content-type:application/json; charset=utf-8\nhost:%s\nx-tc-action:%s\n", host, strings.ToLower(action))
	signedHeaders := "content-type;host;x-tc-action"
	hashedRequestPayload := sha256Hex(payloadStr)
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		httpMethod, canonicalURI, canonicalQueryString, canonicalHeaders, signedHeaders, hashedRequestPayload)

	// 2. String to sign
	algorithm := "TC3-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	hashedCanonicalRequest := sha256Hex(canonicalRequest)
	stringToSign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm, timestamp, credentialScope, hashedCanonicalRequest)

	// 3. Signature
	secretDate := hmacSha256([]byte("TC3"+secretKey), date)
	secretService := hmacSha256(secretDate, service)
	secretSigning := hmacSha256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSha256(secretSigning, stringToSign))

	// 4. Authorization header
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, secretId, credentialScope, signedHeaders, signature)

	req, err := http.NewRequestWithContext(ctx, httpMethod, "https://"+host, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", version)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("Authorization", authorization)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var baseResp struct {
		Response struct {
			Error *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}

	if err := json.Unmarshal(respBody, &baseResp); err == nil && baseResp.Response.Error != nil {
		return fmt.Errorf("Tencent DNSPod API error: [%s] %s", baseResp.Response.Error.Code, baseResp.Response.Error.Message)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Tencent DNSPod HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if target != nil {
		return json.Unmarshal(respBody, target)
	}
	return nil
}

func (t *TencentProvider) TestAuth(ctx context.Context, env map[string]string) error {
	secretId := strings.TrimSpace(env["Tencent_SecretId"])
	secretKey := strings.TrimSpace(env["Tencent_SecretKey"])
	if secretId == "" || secretKey == "" {
		return errors.New("missing Tencent SecretId or SecretKey")
	}

	var result struct {
		Response struct {
			TotalCount int64 `json:"TotalCount"`
		} `json:"Response"`
	}
	payload := map[string]any{"Limit": 1}
	return t.doRequest(ctx, secretId, secretKey, "DescribeDomainList", payload, &result)
}

func (t *TencentProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	secretId := strings.TrimSpace(env["Tencent_SecretId"])
	secretKey := strings.TrimSpace(env["Tencent_SecretKey"])
	if secretId == "" || secretKey == "" {
		return "", errors.New("missing Tencent SecretId or SecretKey")
	}

	subDomain, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return "", err
	}

	// 1. 查询记录
	queryPayload := map[string]any{
		"Domain":     rootDomain,
		"Subdomain":  subDomain,
		"RecordType": param.Type,
	}

	var queryResult struct {
		Response struct {
			RecordList []struct {
				RecordId uint64 `json:"RecordId"`
				Name     string `json:"Name"`
				Type     string `json:"Type"`
				Value    string `json:"Value"`
				TTL      uint64 `json:"TTL"`
			} `json:"RecordList"`
		} `json:"Response"`
	}

	if err := t.doRequest(ctx, secretId, secretKey, "DescribeRecordList", queryPayload, &queryResult); err != nil {
		// 如果是没找到记录的报错，不退出，继续执行新建
		if !strings.Contains(err.Error(), "ResourceNotFound.NoDataOfRecord") {
			return "", err
		}
	}

	var matchedRecordId uint64
	for _, r := range queryResult.Response.RecordList {
		if r.Name == subDomain && r.Type == param.Type && r.Value == param.IP {
			matchedRecordId = r.RecordId
			break
		}
	}

	ttl := param.TTL
	if ttl < 600 {
		ttl = 600
	}

	if matchedRecordId > 0 {
		return strconv.FormatUint(matchedRecordId, 10), nil
	}

	// 新建记录
	createPayload := map[string]any{
		"Domain":     rootDomain,
		"SubDomain":  subDomain,
		"RecordType": param.Type,
		"RecordLine": "默认",
		"Value":      param.IP,
		"TTL":        ttl,
	}
	var createResult struct {
		Response struct {
			RecordId uint64 `json:"RecordId"`
		} `json:"Response"`
	}
	if err := t.doRequest(ctx, secretId, secretKey, "CreateRecord", createPayload, &createResult); err != nil {
		return "", err
	}
	return strconv.FormatUint(createResult.Response.RecordId, 10), nil
}

func (t *TencentProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	secretId := strings.TrimSpace(env["Tencent_SecretId"])
	secretKey := strings.TrimSpace(env["Tencent_SecretKey"])
	if secretId == "" || secretKey == "" {
		return errors.New("missing Tencent SecretId or SecretKey")
	}

	subDomain, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return err
	}

	queryPayload := map[string]any{
		"Domain":     rootDomain,
		"Subdomain":  subDomain,
		"RecordType": param.Type,
	}

	var queryResult struct {
		Response struct {
			RecordList []struct {
				RecordId uint64 `json:"RecordId"`
				Name     string `json:"Name"`
				Type     string `json:"Type"`
				Value    string `json:"Value"`
				TTL      uint64 `json:"TTL"`
			} `json:"RecordList"`
		} `json:"Response"`
	}

	if err := t.doRequest(ctx, secretId, secretKey, "DescribeRecordList", queryPayload, &queryResult); err != nil {
		if strings.Contains(err.Error(), "ResourceNotFound.NoDataOfRecord") {
			return nil
		}
		return err
	}

	for _, r := range queryResult.Response.RecordList {
		if r.Name == subDomain && r.Type == param.Type {
			if param.IP != "" && r.Value != param.IP {
				continue
			}
			delPayload := map[string]any{
				"Domain":   rootDomain,
				"RecordId": r.RecordId,
			}
			var delResult struct {
				Response struct {
					RecordId uint64 `json:"RecordId"`
				} `json:"Response"`
			}
			if delErr := t.doRequest(ctx, secretId, secretKey, "DeleteRecord", delPayload, &delResult); delErr != nil {
				return fmt.Errorf("Tencent delete record %d failed: %w", r.RecordId, delErr)
			}
		}
	}

	return nil
}

func (t *TencentProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	secretId := strings.TrimSpace(env["Tencent_SecretId"])
	secretKey := strings.TrimSpace(env["Tencent_SecretKey"])
	if secretId == "" || secretKey == "" {
		return nil, errors.New("missing Tencent SecretId or SecretKey")
	}

	payload := map[string]any{
		"Domain": rootDomain,
		"Limit":  100,
	}

	var result struct {
		Response struct {
			RecordList []struct {
				RecordId uint64 `json:"RecordId"`
				Name     string `json:"Name"`
				Type     string `json:"Type"`
				Value    string `json:"Value"`
				TTL      uint64 `json:"TTL"`
				MX       uint64 `json:"MX"`
			} `json:"RecordList"`
		} `json:"Response"`
	}

	if err := t.doRequest(ctx, secretId, secretKey, "DescribeRecordList", payload, &result); err != nil {
		if strings.Contains(err.Error(), "ResourceNotFound.NoDataOfRecord") {
			return []DNSRecord{}, nil
		}
		return nil, err
	}

	list := result.Response.RecordList
	res := make([]DNSRecord, 0, len(list))
	for _, r := range list {
		res = append(res, DNSRecord{
			ID:       strconv.FormatUint(r.RecordId, 10),
			Name:     r.Name,
			Type:     r.Type,
			Value:    r.Value,
			TTL:      int(r.TTL),
			Priority: int(r.MX),
		})
	}
	return res, nil
}

func (t *TencentProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	secretId := strings.TrimSpace(env["Tencent_SecretId"])
	secretKey := strings.TrimSpace(env["Tencent_SecretKey"])
	if secretId == "" || secretKey == "" {
		return "", errors.New("missing Tencent SecretId or SecretKey")
	}

	sub := record.Name
	if sub == "" {
		sub = "@"
	}

	ttl := record.TTL
	if ttl < 600 {
		ttl = 600
	}

	payload := map[string]any{
		"Domain":     rootDomain,
		"SubDomain":  sub,
		"RecordType": strings.ToUpper(record.Type),
		"RecordLine": "默认",
		"Value":      record.Value,
		"TTL":        ttl,
	}
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		payload["MX"] = record.Priority
	}

	var result struct {
		Response struct {
			RecordId uint64 `json:"RecordId"`
		} `json:"Response"`
	}
	if err := t.doRequest(ctx, secretId, secretKey, "CreateRecord", payload, &result); err != nil {
		return "", err
	}
	return strconv.FormatUint(result.Response.RecordId, 10), nil
}

func (t *TencentProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	secretId := strings.TrimSpace(env["Tencent_SecretId"])
	secretKey := strings.TrimSpace(env["Tencent_SecretKey"])
	if secretId == "" || secretKey == "" {
		return errors.New("missing Tencent SecretId or SecretKey")
	}

	recID, err := strconv.ParseUint(record.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid record ID: %s", record.ID)
	}

	sub := record.Name
	if sub == "" {
		sub = "@"
	}

	ttl := record.TTL
	if ttl < 600 {
		ttl = 600
	}

	payload := map[string]any{
		"Domain":     rootDomain,
		"RecordId":   recID,
		"SubDomain":  sub,
		"RecordType": strings.ToUpper(record.Type),
		"RecordLine": "默认",
		"Value":      record.Value,
		"TTL":        ttl,
	}
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		payload["MX"] = record.Priority
	}

	var result struct {
		Response struct {
			RecordId uint64 `json:"RecordId"`
		} `json:"Response"`
	}
	return t.doRequest(ctx, secretId, secretKey, "ModifyRecord", payload, &result)
}

func (t *TencentProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	secretId := strings.TrimSpace(env["Tencent_SecretId"])
	secretKey := strings.TrimSpace(env["Tencent_SecretKey"])
	if secretId == "" || secretKey == "" {
		return errors.New("missing Tencent SecretId or SecretKey")
	}

	recID, err := strconv.ParseUint(recordID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid record ID: %s", recordID)
	}

	payload := map[string]any{
		"Domain":   rootDomain,
		"RecordId": recID,
	}

	var result struct {
		Response struct {
			RecordId uint64 `json:"RecordId"`
		} `json:"Response"`
	}
	return t.doRequest(ctx, secretId, secretKey, "DeleteRecord", payload, &result)
}

func (t *TencentProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	secretId := strings.TrimSpace(env["Tencent_SecretId"])
	secretKey := strings.TrimSpace(env["Tencent_SecretKey"])
	if secretId == "" || secretKey == "" {
		return nil, errors.New("missing Tencent SecretId or SecretKey")
	}

	payload := map[string]any{"Limit": 100}
	var result struct {
		Response struct {
			DomainList []struct {
				Name string `json:"Name"`
			} `json:"DomainList"`
		} `json:"Response"`
	}
	if err := t.doRequest(ctx, secretId, secretKey, "DescribeDomainList", payload, &result); err != nil {
		return nil, err
	}

	domains := make([]string, 0, len(result.Response.DomainList))
	for _, d := range result.Response.DomainList {
		if d.Name != "" {
			domains = append(domains, d.Name)
		}
	}
	return domains, nil
}
