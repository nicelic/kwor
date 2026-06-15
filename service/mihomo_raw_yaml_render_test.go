package service

import (
	"strings"
	"testing"
)

func TestRenderMihomoDocumentYAML_UsesFlowStyleForSudokuCustomTables(t *testing.T) {
	document := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{
				"name":          "sudoku-node",
				"type":          "sudoku",
				"custom-tables": []string{"ppvxvxvv", "vxvpvpvx"},
			},
		},
	}

	raw, err := renderMihomoDocumentYAML(document, nil)
	if err != nil {
		t.Fatalf("renderMihomoDocumentYAML failed: %v", err)
	}

	text := string(raw)
	if !strings.Contains(text, `custom-tables: ["ppvxvxvv","vxvpvpvx"]`) {
		t.Fatalf("expected flow style custom-tables in rendered server yaml, got:\n%s", text)
	}
}
