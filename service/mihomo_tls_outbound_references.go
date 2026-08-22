package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

type mihomoTLSOutboundReference struct {
	Path   string
	Target string
}

// visitMihomoTLSOutboundProxyFields visits only the TLS wrapper proxy fields
// that the listener renderer can emit into server.yaml.
func visitMihomoTLSOutboundProxyFields(server map[string]interface{}, mode string, visit func(string, map[string]interface{}, string) error) error {
	if server == nil || visit == nil {
		return nil
	}

	switch mode {
	case model.MihomoTlsModeShadowTLS:
		wrapper, _ := server["shadow_tls"].(map[string]interface{})
		if wrapper == nil {
			return nil
		}
		if handshake, _ := wrapper["handshake"].(map[string]interface{}); handshake != nil {
			if err := visit("shadow-tls.handshake.proxy", handshake, "proxy"); err != nil {
				return err
			}
		}
		mappings, _ := wrapper["handshake_for_server_name"].(map[string]interface{})
		if mappings == nil {
			return nil
		}
		names := make([]string, 0, len(mappings))
		for name := range mappings {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			handshake, _ := mappings[name].(map[string]interface{})
			if handshake == nil {
				continue
			}
			if err := visit(fmt.Sprintf("shadow-tls.handshake-for-server-name[%q].proxy", name), handshake, "proxy"); err != nil {
				return err
			}
		}
	case model.MihomoTlsModeRestls:
		wrapper, _ := server["res_tls"].(map[string]interface{})
		if wrapper != nil {
			return visit("res-tls.proxy", wrapper, "proxy")
		}
	case model.MihomoTlsModeJLS:
		wrapper, _ := server["jls_config"].(map[string]interface{})
		if wrapper != nil {
			return visit("jls-config.proxy", wrapper, "proxy")
		}
	}

	return nil
}

func collectMihomoTLSOutboundReferences(tls *model.MihomoTls) ([]mihomoTLSOutboundReference, error) {
	if tls == nil {
		return nil, nil
	}

	snapshot := *tls
	snapshot.Sanitize()
	server := map[string]interface{}{}
	if len(snapshot.Server) == 0 || strings.TrimSpace(string(snapshot.Server)) == "null" {
		return nil, nil
	}
	if err := json.Unmarshal(snapshot.Server, &server); err != nil {
		return nil, fmt.Errorf("parse Mihomo TLS server configuration: %w", err)
	}

	references := make([]mihomoTLSOutboundReference, 0)
	err := visitMihomoTLSOutboundProxyFields(server, snapshot.Mode, func(path string, parent map[string]interface{}, key string) error {
		raw, exists := parent[key]
		if !exists || raw == nil {
			return nil
		}
		target, ok := raw.(string)
		if !ok {
			return fmt.Errorf("mihomo TLS %s must be a string", path)
		}
		target = strings.TrimSpace(target)
		if target != "" {
			references = append(references, mihomoTLSOutboundReference{Path: path, Target: target})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return references, nil
}

func normalizeMihomoTLSOutboundReferenceTarget(path, raw string, targets *mihomoProxyConversionResult) (string, error) {
	normalized, ok := normalizeMihomoTarget(raw, targets)
	if !ok || normalized == "REJECT" || normalized == "REJECT-DROP" {
		return "", fmt.Errorf("mihomo TLS %s target %q is not a supported Mihomo proxy or proxy group", path, raw)
	}
	return normalized, nil
}

func normalizeMihomoTLSOutboundReferences(tls *model.MihomoTls, targets *mihomoProxyConversionResult) error {
	if tls == nil {
		return fmt.Errorf("mihomo TLS is required")
	}
	tls.Sanitize()

	server := map[string]interface{}{}
	if len(tls.Server) == 0 || strings.TrimSpace(string(tls.Server)) == "null" {
		return nil
	}
	if err := json.Unmarshal(tls.Server, &server); err != nil {
		return fmt.Errorf("parse Mihomo TLS server configuration: %w", err)
	}

	changed := false
	err := visitMihomoTLSOutboundProxyFields(server, tls.Mode, func(path string, parent map[string]interface{}, key string) error {
		raw, exists := parent[key]
		if !exists || raw == nil {
			return nil
		}
		target, ok := raw.(string)
		if !ok {
			return fmt.Errorf("mihomo TLS %s must be a string", path)
		}
		target = strings.TrimSpace(target)
		if target == "" {
			return nil
		}
		normalized, err := normalizeMihomoTLSOutboundReferenceTarget(path, target, targets)
		if err != nil {
			return err
		}
		if target != normalized {
			parent[key] = normalized
			changed = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	encoded, err := json.Marshal(server)
	if err != nil {
		return fmt.Errorf("marshal Mihomo TLS server configuration: %w", err)
	}
	tls.Server = json.RawMessage(encoded)
	return nil
}

func validateMihomoTLSOutboundReferences(tls *model.MihomoTls, targets *mihomoProxyConversionResult) error {
	if tls == nil {
		return nil
	}
	snapshot := *tls
	return normalizeMihomoTLSOutboundReferences(&snapshot, targets)
}

func validateAndNormalizeMihomoTLSOutboundReferences(tx *gorm.DB, tls *model.MihomoTls) error {
	if tx == nil {
		return fmt.Errorf("mihomo TLS outbound reference validation requires a database transaction")
	}
	targets, err := loadMihomoRouteTargets(tx)
	if err != nil {
		return err
	}
	return normalizeMihomoTLSOutboundReferences(tls, targets)
}
