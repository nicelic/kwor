package service

// SingboxInboundUserManagement describes a UI-only client binding capability.
// It is attached to inbound list responses and is never persisted as an inbound option.
type SingboxInboundUserManagement struct {
	Selectable     bool   `json:"selectable"`
	UsesUsersField bool   `json:"uses_users_field"`
	Mode           string `json:"mode"`
	IdentityType   string `json:"identity_type"`
	Reason         string `json:"reason"`
}

func buildSingboxInboundUserManagement(inboundType string, shadowTLSVersion uint) SingboxInboundUserManagement {
	switch inboundType {
	case "mixed", "socks", "http":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "username",
			Reason:         "proxy_auth_users",
		}
	case "vmess":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "uuid",
			Reason:         "vmess_uuid_users",
		}
	case "vless":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "uuid",
			Reason:         "vless_uuid_users",
		}
	case "trojan", "anytls", "hysteria2":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "password",
			Reason:         inboundType + "_password_users",
		}
	case "naive":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "username",
			Reason:         "naive_username_password_users",
		}
	case "hysteria":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "auth_str",
			Reason:         "hysteria_auth_users",
		}
	case "shadowtls":
		if shadowTLSVersion >= 3 {
			return SingboxInboundUserManagement{
				Selectable:     true,
				UsesUsersField: true,
				Mode:           "users_list",
				IdentityType:   "name",
				Reason:         "shadowtls_v3_users",
			}
		}
		return SingboxInboundUserManagement{
			Selectable:     false,
			UsesUsersField: false,
			Mode:           "shared_password",
			IdentityType:   "shared_password",
			Reason:         "shadowtls_legacy_password",
		}
	case "tuic":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: true,
			Mode:           "users_list",
			IdentityType:   "uuid",
			Reason:         "tuic_uuid_users",
		}
	case "shadowsocks":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: false,
			Mode:           "shared_password",
			IdentityType:   "shared_password",
			Reason:         "shadowsocks_single_password",
		}
	case "ssh":
		return SingboxInboundUserManagement{
			Selectable:     true,
			UsesUsersField: false,
			Mode:           "shared_credentials",
			IdentityType:   "type_tag",
			Reason:         "ssh_subscription_outbound_only",
		}
	case "direct", "redirect", "tproxy", "tun":
		return SingboxInboundUserManagement{
			Selectable:     false,
			UsesUsersField: false,
			Mode:           "not_applicable",
			IdentityType:   "none",
			Reason:         "transport_listener",
		}
	default:
		return SingboxInboundUserManagement{
			Selectable:     false,
			UsesUsersField: false,
			Mode:           "not_applicable",
			IdentityType:   "none",
			Reason:         "no_user_management",
		}
	}
}
