package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

const mihomoInboundMetaFilename = "mihomo_inbounds_meta.json"

type MihomoInboundUserManagement struct {
	Selectable     bool   `json:"selectable"`
	UsesUsersField bool   `json:"uses_users_field"`
	Mode           string `json:"mode"`
	IdentityType   string `json:"identity_type"`
	Reason         string `json:"reason"`
}

type MihomoInboundMetaEntry struct {
	Id             uint                        `json:"id"`
	Tag            string                      `json:"tag"`
	Type           string                      `json:"type"`
	TlsId          uint                        `json:"tls_id,omitempty"`
	UserManagement MihomoInboundUserManagement `json:"user_management"`
}

type MihomoInboundMetaDocument struct {
	GeneratedAt int64                    `json:"generated_at"`
	Inbounds    []MihomoInboundMetaEntry `json:"inbounds"`
}

func buildMihomoInboundUserManagement(inboundType string, shadowTLSVersion int) MihomoInboundUserManagement {
	switch inboundType {
	case "mixed", "socks", "http":
		return MihomoInboundUserManagement{
			Selectable:     false,
			UsesUsersField: true,
			Mode:           "auth_users",
			IdentityType:   "username",
			Reason:         "proxy_auth_not_user_managed",
		}
	case "vmess":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "uuid",
			Reason:         "vmess_uuid_users",
		}
	case "vless":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "uuid",
			Reason:         "vless_uuid_users",
		}
	case "trojan":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "password",
			Reason:         "trojan_password_users",
		}
	case "tuic":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_map",
			IdentityType:   "uuid",
			Reason:         "tuic_user_map",
		}
	case "hysteria2":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_map",
			IdentityType:   "username",
			Reason:         "hysteria2_user_map",
		}
	case "anytls":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_map",
			IdentityType:   "username",
			Reason:         "anytls_user_map",
		}
	case "mieru":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_map",
			IdentityType:   "username",
			Reason:         "mieru_user_map",
		}
	case "sudoku":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: false,
			Mode:           "shared_uuid",
			IdentityType:   "uuid",
			Reason:         "sudoku_client_uuid",
		}
	case "trusttunnel":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "username",
			Reason:         "trusttunnel_username_password_users",
		}
	case "shadowtls":
		if shadowTLSVersion >= 3 {
			return MihomoInboundUserManagement{
				Selectable:     true,
				UsesUsersField: true,
				Mode:           "users_list",
				IdentityType:   "name",
				Reason:         "shadowtls_v3_users",
			}
		}
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: false,
			Mode:           "shared_password",
			IdentityType:   "shared_password",
			Reason:         "shadowtls_legacy_password",
		}
	case "shadowsocks":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: false,
			Mode:           "shared_password",
			IdentityType:   "shared_password",
			Reason:         "shadowsocks_single_password",
		}
	case "snell":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: false,
			Mode:           "shared_password",
			IdentityType:   "shared_password",
			Reason:         "snell_shared_psk",
		}
	case "ssh":
		return MihomoInboundUserManagement{
			Selectable:     true,
			UsesUsersField: false,
			Mode:           "shared_credentials",
			IdentityType:   "type_tag",
			Reason:         "ssh_subscription_outbound_only",
		}
	case "direct", "redirect", "tproxy", "tun":
		return MihomoInboundUserManagement{
			Selectable:     false,
			UsesUsersField: false,
			Mode:           "not_applicable",
			IdentityType:   "none",
			Reason:         "transport_listener",
		}
	default:
		return MihomoInboundUserManagement{
			Selectable:     false,
			UsesUsersField: false,
			Mode:           "not_applicable",
			IdentityType:   "none",
			Reason:         "no_user_management",
		}
	}
}

func buildMihomoInboundUserManagementFromOptions(inboundType string, options json.RawMessage) MihomoInboundUserManagement {
	var fields map[string]json.RawMessage
	if len(options) > 0 {
		_ = json.Unmarshal(options, &fields)
	}

	var shadowTLSVersion int
	if inboundType == "shadowtls" && fields != nil {
		_ = json.Unmarshal(fields["version"], &shadowTLSVersion)
	}

	return buildMihomoInboundUserManagement(inboundType, shadowTLSVersion)
}

func attachMihomoInboundUserManagementView(view map[string]interface{}, inbound model.MihomoInbound) MihomoInboundUserManagement {
	userManagement := buildMihomoInboundUserManagementFromOptions(inbound.Type, inbound.Options)
	view["user_management"] = userManagement
	return userManagement
}

func buildMihomoInboundMetaDocument(inbounds []model.MihomoInbound) *MihomoInboundMetaDocument {
	entries := make([]MihomoInboundMetaEntry, 0, len(inbounds))
	for _, inbound := range inbounds {
		entries = append(entries, MihomoInboundMetaEntry{
			Id:             inbound.Id,
			Tag:            inbound.Tag,
			Type:           inbound.Type,
			TlsId:          inbound.TlsId,
			UserManagement: buildMihomoInboundUserManagementFromOptions(inbound.Type, inbound.Options),
		})
	}

	return &MihomoInboundMetaDocument{
		GeneratedAt: time.Now().Unix(),
		Inbounds:    entries,
	}
}

func writeMihomoInboundMetaFile(coreDir string) error {
	db := database.GetDB()
	var inbounds []model.MihomoInbound
	if err := db.Model(model.MihomoInbound{}).Find(&inbounds).Error; err != nil {
		return fmt.Errorf("load mihomo inbounds for metadata failed: %w", err)
	}

	document := buildMihomoInboundMetaDocument(inbounds)
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mihomo inbound metadata failed: %w", err)
	}

	filePath := filepath.Join(coreDir, mihomoInboundMetaFilename)
	if err := ManagedRuntimeWriteFile(filePath, data); err != nil {
		return fmt.Errorf("write mihomo inbound metadata failed: %w", err)
	}

	return nil
}
