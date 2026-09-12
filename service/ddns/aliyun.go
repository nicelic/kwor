package ddns

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type AliyunProvider struct {
	code string
}

func init() {
	RegisterProvider(&AliyunProvider{code: "dns_ali"})
	RegisterProvider(&AliyunProvider{code: "aliyun"})
}

func (a *AliyunProvider) Code() string {
	if a.code != "" {
		return a.code
	}
	return "dns_ali"
}

func (a *AliyunProvider) Name() string {
	return "阿里云 (AliDNS)"
}

func (a *AliyunProvider) sign(accessKeySecret string, params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonicalizedQueryStrings []string
	for _, k := range keys {
		canonicalizedQueryStrings = append(canonicalizedQueryStrings,
			percentEncode(k)+"="+percentEncode(params.Get(k)))
	}
	canonicalized := strings.Join(canonicalizedQueryStrings, "&")
	stringToSign := "GET&" + percentEncode("/") + "&" + percentEncode(canonicalized)

	mac := hmac.New(sha1.New, []byte(accessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func percentEncode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func (a *AliyunProvider) doRequest(ctx context.Context, ak, sk string, params url.Values, target any) error {
	params.Set("Format", "JSON")
	params.Set("Version", "2015-01-09")
	params.Set("AccessKeyId", ak)
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	params.Set("SignatureVersion", "1.0")
	params.Set("SignatureNonce", fmt.Sprintf("%d", time.Now().UnixNano()))

	signature := a.sign(sk, params)
	params.Set("Signature", signature)

	reqURL := "https://alidns.aliyuncs.com/?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		}
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Message != "" {
			return fmt.Errorf("Aliyun DNS API error: [%s] %s", errResp.Code, errResp.Message)
		}
		return fmt.Errorf("Aliyun DNS HTTP %d: %s", resp.StatusCode, string(body))
	}

	if target != nil {
		return json.Unmarshal(body, target)
	}
	return nil
}

func (a *AliyunProvider) TestAuth(ctx context.Context, env map[string]string) error {
	ak := strings.TrimSpace(env["Ali_Key"])
	sk := strings.TrimSpace(env["Ali_Secret"])
	if ak == "" || sk == "" {
		return errors.New("missing Aliyun Access Key ID or Secret")
	}

	params := url.Values{}
	params.Set("Action", "DescribeDomains")
	params.Set("PageSize", "1")

	var result struct {
		TotalCount int64 `json:"TotalCount"`
	}
	return a.doRequest(ctx, ak, sk, params, &result)
}

func (a *AliyunProvider) SyncRecord(ctx context.Context, env map[string]string, param RecordParam) (string, error) {
	ak := strings.TrimSpace(env["Ali_Key"])
	sk := strings.TrimSpace(env["Ali_Secret"])
	if ak == "" || sk == "" {
		return "", errors.New("missing Aliyun Access Key ID or Secret")
	}

	subDomain, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return "", err
	}

	// 1. 查询解析记录
	queryParams := url.Values{}
	queryParams.Set("Action", "DescribeDomainRecords")
	queryParams.Set("DomainName", rootDomain)
	queryParams.Set("RRKeyWord", subDomain)
	queryParams.Set("TypeKeyWord", param.Type)

	var queryResult struct {
		DomainRecords struct {
			Record []struct {
				RecordId string `json:"RecordId"`
				RR       string `json:"RR"`
				Type     string `json:"Type"`
				Value    string `json:"Value"`
				TTL      int    `json:"TTL"`
			} `json:"Record"`
		} `json:"DomainRecords"`
	}

	if err := a.doRequest(ctx, ak, sk, queryParams, &queryResult); err != nil {
		return "", err
	}

	records := queryResult.DomainRecords.Record
	var matchedRecordId string
	for _, r := range records {
		if r.RR == subDomain && r.Type == param.Type && r.Value == param.IP {
			matchedRecordId = r.RecordId
			break
		}
	}

	ttl := param.TTL
	if ttl < 600 {
		ttl = 600 // 阿里云免费版最低 TTL 为 600 秒
	}

	if matchedRecordId != "" {
		return matchedRecordId, nil
	}

	// 新增记录
	addParams := url.Values{}
	addParams.Set("Action", "AddDomainRecord")
	addParams.Set("DomainName", rootDomain)
	addParams.Set("RR", subDomain)
	addParams.Set("Type", param.Type)
	addParams.Set("Value", param.IP)
	addParams.Set("TTL", fmt.Sprintf("%d", ttl))

	var addResult struct {
		RecordId string `json:"RecordId"`
	}
	if err := a.doRequest(ctx, ak, sk, addParams, &addResult); err != nil {
		return "", err
	}
	return addResult.RecordId, nil
}

func (a *AliyunProvider) DeleteRecord(ctx context.Context, env map[string]string, param RecordParam) error {
	ak := strings.TrimSpace(env["Ali_Key"])
	sk := strings.TrimSpace(env["Ali_Secret"])
	if ak == "" || sk == "" {
		return errors.New("missing Aliyun Access Key ID or Secret")
	}

	subDomain, rootDomain, err := SplitDomain(param.Domain)
	if err != nil {
		return err
	}

	queryParams := url.Values{}
	queryParams.Set("Action", "DescribeDomainRecords")
	queryParams.Set("DomainName", rootDomain)
	queryParams.Set("RRKeyWord", subDomain)
	queryParams.Set("TypeKeyWord", param.Type)

	var queryResult struct {
		DomainRecords struct {
			Record []struct {
				RecordId string `json:"RecordId"`
				RR       string `json:"RR"`
				Type     string `json:"Type"`
				Value    string `json:"Value"`
			} `json:"Record"`
		} `json:"DomainRecords"`
	}

	if err := a.doRequest(ctx, ak, sk, queryParams, &queryResult); err != nil {
		return err
	}

	for _, r := range queryResult.DomainRecords.Record {
		if r.RR == subDomain && r.Type == param.Type {
			if param.IP != "" && r.Value != param.IP {
				continue
			}
			delParams := url.Values{}
			delParams.Set("Action", "DeleteDomainRecord")
			delParams.Set("RecordId", r.RecordId)

			var delResult struct {
				RecordId string `json:"RecordId"`
			}
			if delErr := a.doRequest(ctx, ak, sk, delParams, &delResult); delErr != nil {
				return fmt.Errorf("Aliyun delete record %s failed: %w", r.RecordId, delErr)
			}
		}
	}

	return nil
}

func (a *AliyunProvider) ListRecords(ctx context.Context, env map[string]string, rootDomain string) ([]DNSRecord, error) {
	ak := strings.TrimSpace(env["Ali_Key"])
	sk := strings.TrimSpace(env["Ali_Secret"])
	if ak == "" || sk == "" {
		return nil, errors.New("missing Aliyun Access Key ID or Secret")
	}

	queryParams := url.Values{}
	queryParams.Set("Action", "DescribeDomainRecords")
	queryParams.Set("DomainName", rootDomain)
	queryParams.Set("PageSize", "100")

	var queryResult struct {
		DomainRecords struct {
			Record []struct {
				RecordId string `json:"RecordId"`
				RR       string `json:"RR"`
				Type     string `json:"Type"`
				Value    string `json:"Value"`
				TTL      int    `json:"TTL"`
				Priority int    `json:"Priority"`
			} `json:"Record"`
		} `json:"DomainRecords"`
	}

	if err := a.doRequest(ctx, ak, sk, queryParams, &queryResult); err != nil {
		return nil, err
	}

	records := queryResult.DomainRecords.Record
	result := make([]DNSRecord, 0, len(records))
	for _, r := range records {
		result = append(result, DNSRecord{
			ID:       r.RecordId,
			Name:     r.RR,
			Type:     r.Type,
			Value:    r.Value,
			TTL:      r.TTL,
			Priority: r.Priority,
		})
	}
	return result, nil
}

func (a *AliyunProvider) CreateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) (string, error) {
	ak := strings.TrimSpace(env["Ali_Key"])
	sk := strings.TrimSpace(env["Ali_Secret"])
	if ak == "" || sk == "" {
		return "", errors.New("missing Aliyun Access Key ID or Secret")
	}

	rr := record.Name
	if rr == "" {
		rr = "@"
	}

	ttl := record.TTL
	if ttl < 600 {
		ttl = 600 // 阿里云免费版最低 600 秒
	}

	params := url.Values{}
	params.Set("Action", "AddDomainRecord")
	params.Set("DomainName", rootDomain)
	params.Set("RR", rr)
	params.Set("Type", strings.ToUpper(record.Type))
	params.Set("Value", record.Value)
	params.Set("TTL", fmt.Sprintf("%d", ttl))
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		params.Set("Priority", fmt.Sprintf("%d", record.Priority))
	}

	var addResult struct {
		RecordId string `json:"RecordId"`
	}
	if err := a.doRequest(ctx, ak, sk, params, &addResult); err != nil {
		return "", err
	}
	return addResult.RecordId, nil
}

func (a *AliyunProvider) UpdateRecord(ctx context.Context, env map[string]string, rootDomain string, record DNSRecord) error {
	ak := strings.TrimSpace(env["Ali_Key"])
	sk := strings.TrimSpace(env["Ali_Secret"])
	if ak == "" || sk == "" {
		return errors.New("missing Aliyun Access Key ID or Secret")
	}

	if record.ID == "" {
		return errors.New("missing record ID")
	}

	rr := record.Name
	if rr == "" {
		rr = "@"
	}

	ttl := record.TTL
	if ttl < 600 {
		ttl = 600
	}

	params := url.Values{}
	params.Set("Action", "UpdateDomainRecord")
	params.Set("RecordId", record.ID)
	params.Set("RR", rr)
	params.Set("Type", strings.ToUpper(record.Type))
	params.Set("Value", record.Value)
	params.Set("TTL", fmt.Sprintf("%d", ttl))
	if strings.ToUpper(record.Type) == "MX" && record.Priority > 0 {
		params.Set("Priority", fmt.Sprintf("%d", record.Priority))
	}

	var updateResult struct {
		RecordId string `json:"RecordId"`
	}
	return a.doRequest(ctx, ak, sk, params, &updateResult)
}

func (a *AliyunProvider) DeleteRecordByID(ctx context.Context, env map[string]string, rootDomain, recordID string) error {
	ak := strings.TrimSpace(env["Ali_Key"])
	sk := strings.TrimSpace(env["Ali_Secret"])
	if ak == "" || sk == "" {
		return errors.New("missing Aliyun Access Key ID or Secret")
	}

	if recordID == "" {
		return errors.New("missing record ID")
	}

	delParams := url.Values{}
	delParams.Set("Action", "DeleteDomainRecord")
	delParams.Set("RecordId", recordID)

	var delResult struct {
		RecordId string `json:"RecordId"`
	}
	return a.doRequest(ctx, ak, sk, delParams, &delResult)
}

func (a *AliyunProvider) ListDomains(ctx context.Context, env map[string]string) ([]string, error) {
	ak := strings.TrimSpace(env["Ali_Key"])
	sk := strings.TrimSpace(env["Ali_Secret"])
	if ak == "" || sk == "" {
		return nil, errors.New("missing Aliyun Access Key ID or Secret")
	}

	params := url.Values{}
	params.Set("Action", "DescribeDomains")
	params.Set("PageSize", "100")

	var result struct {
		Domains struct {
			Domain []struct {
				DomainName string `json:"DomainName"`
			} `json:"Domain"`
		} `json:"Domains"`
	}
	if err := a.doRequest(ctx, ak, sk, params, &result); err != nil {
		return nil, err
	}

	domains := make([]string, 0, len(result.Domains.Domain))
	for _, d := range result.Domains.Domain {
		if d.DomainName != "" {
			domains = append(domains, d.DomainName)
		}
	}
	return domains, nil
}
