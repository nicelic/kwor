package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type WarpService struct{}

const (
	warpRequestTimeout   = 45 * time.Second
	maxWarpResponseBytes = 2 << 20
)

var warpAPIBaseURL = "https://api.cloudflareclient.com/v0a2158"

type warpRegistrationResponse struct {
	ID      string `json:"id"`
	Token   string `json:"token"`
	Account struct {
		License string `json:"license"`
	} `json:"account"`
}

type warpInfoResponse struct {
	Config struct {
		ClientID  string `json:"client_id"`
		Interface struct {
			Addresses struct {
				V4 string `json:"v4"`
				V6 string `json:"v6"`
			} `json:"addresses"`
		} `json:"interface"`
		Peers []struct {
			Endpoint struct {
				Host string `json:"host"`
			} `json:"endpoint"`
			PublicKey string `json:"public_key"`
		} `json:"peers"`
	} `json:"config"`
}

type warpCredentials struct {
	AccessToken string `json:"access_token"`
	DeviceID    string `json:"device_id"`
	LicenseKey  string `json:"license_key"`
}

func newWarpHTTPClient() *http.Client {
	return &http.Client{Timeout: warpRequestTimeout}
}

func readWarpResponse(resp *http.Response) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("warp API returned an empty response")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxWarpResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxWarpResponseBytes {
		return nil, fmt.Errorf("warp API response is too large")
	}
	if resp.StatusCode != http.StatusOK {
		detail := strings.TrimSpace(string(body))
		if detail == "" {
			return nil, fmt.Errorf("warp API request failed: %s", resp.Status)
		}
		return nil, fmt.Errorf("warp API request failed: %s: %s", resp.Status, detail)
	}
	return body, nil
}

func (s *WarpService) getWarpInfo(deviceId string, accessToken string) ([]byte, error) {
	url := fmt.Sprintf("%s/reg/%s", warpAPIBaseURL, deviceId)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := newWarpHTTPClient().Do(req)
	if err != nil {
		return nil, err
	}
	return readWarpResponse(resp)
}

func (s *WarpService) RegisterWarp(ep *model.Endpoint) error {
	tos := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	privateKey, err := wgtypes.GenerateKey()
	if err != nil {
		return err
	}
	publicKey := privateKey.PublicKey().String()
	hostName, hostErr := os.Hostname()
	if hostErr != nil {
		hostName = "kwor"
	}

	payload, err := json.Marshal(struct {
		Key   string `json:"key"`
		TOS   string `json:"tos"`
		Type  string `json:"type"`
		Model string `json:"model"`
		Name  string `json:"name"`
	}{
		Key: publicKey, TOS: tos, Type: "PC", Model: "kwor", Name: hostName,
	})
	if err != nil {
		return err
	}
	url := warpAPIBaseURL + "/reg"

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}

	req.Header.Add("CF-Client-Version", "a-7.21-0721")
	req.Header.Add("Content-Type", "application/json")

	resp, err := newWarpHTTPClient().Do(req)
	if err != nil {
		return err
	}
	body, err := readWarpResponse(resp)
	if err != nil {
		return err
	}

	var registration warpRegistrationResponse
	err = json.Unmarshal(body, &registration)
	if err != nil {
		return err
	}
	if registration.ID == "" || registration.Token == "" {
		return fmt.Errorf("warp registration response is missing device credentials")
	}

	warpInfo, err := s.getWarpInfo(registration.ID, registration.Token)
	if err != nil {
		return err
	}

	var details warpInfoResponse
	err = json.Unmarshal(warpInfo, &details)
	if err != nil {
		return err
	}
	if strings.TrimSpace(details.Config.Interface.Addresses.V4) == "" || strings.TrimSpace(details.Config.Interface.Addresses.V6) == "" {
		return fmt.Errorf("warp registration response is missing interface addresses")
	}
	if len(details.Config.Peers) == 0 {
		return fmt.Errorf("warp registration response does not contain a peer")
	}
	peer := details.Config.Peers[0]
	peerEpAddress, peerEpPort, err := net.SplitHostPort(peer.Endpoint.Host)
	if err != nil {
		return err
	}
	peerPort, _ := strconv.Atoi(peerEpPort)
	if peerEpAddress == "" || peerPort <= 0 || peer.PublicKey == "" {
		return fmt.Errorf("warp registration response contains an invalid peer")
	}

	peers := []map[string]interface{}{
		{
			"address":     peerEpAddress,
			"port":        peerPort,
			"public_key":  peer.PublicKey,
			"allowed_ips": []string{"0.0.0.0/0", "::/0"},
			"reserved":    s.getReserved(details.Config.ClientID),
		},
	}

	warpData := map[string]interface{}{
		"access_token": registration.Token,
		"device_id":    registration.ID,
		"license_key":  registration.Account.License,
	}

	ep.Ext, err = json.MarshalIndent(warpData, "", "  ")
	if err != nil {
		return err
	}

	epOptions := make(map[string]interface{})
	if len(ep.Options) > 0 {
		err = json.Unmarshal(ep.Options, &epOptions)
		if err != nil {
			return err
		}
	}
	epOptions["private_key"] = privateKey.String()
	epOptions["address"] = []string{fmt.Sprintf("%s/32", details.Config.Interface.Addresses.V4), fmt.Sprintf("%s/128", details.Config.Interface.Addresses.V6)}
	epOptions["listen_port"] = 0
	epOptions["peers"] = peers

	ep.Options, err = json.MarshalIndent(epOptions, "", "  ")
	return err
}

func (s *WarpService) getReserved(clientID string) []int {
	var reserved []int
	decoded, err := base64.StdEncoding.DecodeString(clientID)
	if err != nil {
		return nil
	}

	hexString := ""
	for _, char := range decoded {
		hex := fmt.Sprintf("%02x", char)
		hexString += hex
	}

	for i := 0; i < len(hexString); i += 2 {
		hexByte := hexString[i : i+2]
		decValue, err := strconv.ParseInt(hexByte, 16, 32)
		if err != nil {
			return nil
		}
		reserved = append(reserved, int(decValue))
	}

	return reserved
}

func (s *WarpService) SetWarpLicense(old_license string, ep *model.Endpoint) error {
	var warpData warpCredentials
	err := json.Unmarshal(ep.Ext, &warpData)
	if err != nil {
		return err
	}

	if warpData.LicenseKey == old_license {
		return nil
	}
	if warpData.DeviceID == "" || warpData.AccessToken == "" {
		return fmt.Errorf("warp endpoint is missing device credentials")
	}

	url := fmt.Sprintf("%s/reg/%s/account", warpAPIBaseURL, warpData.DeviceID)
	payload, err := json.Marshal(struct {
		License string `json:"license"`
	}{License: warpData.LicenseKey})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+warpData.AccessToken)

	resp, err := newWarpHTTPClient().Do(req)
	if err != nil {
		return err
	}
	body, err := readWarpResponse(resp)
	if err != nil {
		return err
	}
	var response struct {
		Success *bool `json:"success"`
		Errors  []struct {
			Code    interface{} `json:"code"`
			Message string      `json:"message"`
		} `json:"errors"`
	}
	err = json.Unmarshal(body, &response)
	if err != nil {
		return err
	}

	if response.Success != nil && !*response.Success {
		if len(response.Errors) > 0 {
			return common.NewError(response.Errors[0].Code, response.Errors[0].Message)
		}
		return common.NewError("warp license update failed")
	}

	return nil
}
