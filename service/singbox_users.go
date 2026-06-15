package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

func normalizeSingboxUsersForList(inboundType string, users []string, hasTLS bool) ([]json.RawMessage, error) {
	usersJSON := make([]json.RawMessage, 0, len(users))
	for _, rawUser := range users {
		rawUser = strings.TrimSpace(rawUser)
		if rawUser == "" || strings.EqualFold(rawUser, "null") {
			continue
		}

		var user map[string]interface{}
		if err := json.Unmarshal([]byte(rawUser), &user); err != nil {
			return nil, fmt.Errorf("parse sing-box %s user failed: %w", inboundType, err)
		}

		switch inboundType {
		case "mixed", "socks", "http", "naive":
			if username := strings.TrimSpace(firstString(user["username"])); username == "" {
				if legacyName := strings.TrimSpace(firstString(user["name"])); legacyName != "" {
					user["username"] = legacyName
				}
			}
			delete(user, "name")
		case "vmess", "vless", "trojan", "anytls", "hysteria", "shadowtls", "tuic", "hysteria2":
			if name := strings.TrimSpace(firstString(user["name"])); name == "" {
				if legacyUsername := strings.TrimSpace(firstString(user["username"])); legacyUsername != "" {
					user["name"] = legacyUsername
				}
			}
			delete(user, "username")
		}

		if inboundType == "vless" && !hasTLS {
			delete(user, "flow")
		}

		normalized, err := json.Marshal(user)
		if err != nil {
			return nil, fmt.Errorf("marshal sing-box %s user failed: %w", inboundType, err)
		}
		usersJSON = append(usersJSON, json.RawMessage(normalized))
	}

	if len(usersJSON) == 0 {
		return nil, nil
	}
	return usersJSON, nil
}
