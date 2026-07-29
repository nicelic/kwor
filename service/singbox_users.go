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

		normalizedUser := normalizeSingboxRuntimeUser(inboundType, user, hasTLS)
		if normalizedUser == nil {
			continue
		}

		normalized, err := json.Marshal(normalizedUser)
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

// normalizeSingboxRuntimeUser rebuilds a user from the schema accepted by each
// sing-box inbound. Client.Config is panel data, so it must not be forwarded as
// an arbitrary map into the runtime users list.
func normalizeSingboxRuntimeUser(inboundType string, user map[string]interface{}, hasTLS bool) map[string]interface{} {
	name := singboxUserName(user)
	username := singboxUserUsername(user)
	password := strings.TrimSpace(firstString(user["password"]))

	switch inboundType {
	case "mixed", "socks", "http", "naive":
		if username == "" || password == "" {
			return nil
		}
		return map[string]interface{}{
			"username": username,
			"password": password,
		}
	case "vmess":
		uuid := strings.TrimSpace(firstString(user["uuid"]))
		if uuid == "" {
			return nil
		}
		normalized := map[string]interface{}{"uuid": uuid}
		if name != "" {
			normalized["name"] = name
		}
		if alterID, ok := toInt(firstNonNil(user["alterId"], user["alter_id"])); ok && alterID >= 0 {
			normalized["alterId"] = alterID
		}
		return normalized
	case "vless":
		uuid := strings.TrimSpace(firstString(user["uuid"]))
		if uuid == "" {
			return nil
		}
		normalized := map[string]interface{}{"uuid": uuid}
		if name != "" {
			normalized["name"] = name
		}
		if hasTLS {
			if flow := strings.TrimSpace(firstString(user["flow"])); flow != "" {
				normalized["flow"] = flow
			}
		}
		return normalized
	case "trojan", "shadowtls", "hysteria2", "anytls":
		if password == "" {
			return nil
		}
		normalized := map[string]interface{}{"password": password}
		if name != "" {
			normalized["name"] = name
		}
		return normalized
	case "hysteria":
		authStr := strings.TrimSpace(firstString(user["auth_str"]))
		auth := strings.TrimSpace(firstString(user["auth"]))
		if authStr == "" && auth == "" {
			return nil
		}
		normalized := map[string]interface{}{}
		if name != "" {
			normalized["name"] = name
		}
		if authStr != "" {
			normalized["auth_str"] = authStr
		} else {
			normalized["auth"] = auth
		}
		return normalized
	case "tuic":
		uuid := strings.TrimSpace(firstString(user["uuid"]))
		if uuid == "" || password == "" {
			return nil
		}
		normalized := map[string]interface{}{
			"uuid":     uuid,
			"password": password,
		}
		if name != "" {
			normalized["name"] = name
		}
		return normalized
	default:
		return nil
	}
}

func singboxUserName(user map[string]interface{}) string {
	if name := strings.TrimSpace(firstString(user["name"])); name != "" {
		return name
	}
	return strings.TrimSpace(firstString(user["username"]))
}

func singboxUserUsername(user map[string]interface{}) string {
	if username := strings.TrimSpace(firstString(user["username"])); username != "" {
		return username
	}
	return strings.TrimSpace(firstString(user["name"]))
}
