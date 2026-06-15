package util

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestApplySudokuCustomTablesFlowYAML(t *testing.T) {
	document := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{
				"type":          "sudoku",
				"custom-tables": []string{"ppvxvxvv", "vxvpvpvx"},
			},
		},
	}

	ApplySudokuCustomTablesFlowYAML(document)
	raw, err := yaml.Marshal(document)
	if err != nil {
		t.Fatalf("yaml.Marshal failed: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `custom-tables: ["ppvxvxvv", "vxvpvpvx"]`) {
		t.Fatalf("expected flow style custom-tables, got:\n%s", text)
	}

	compact := string(CompactSudokuCustomTablesFlowYAML(raw))
	if !strings.Contains(compact, `custom-tables: ["ppvxvxvv","vxvpvpvx"]`) {
		t.Fatalf("expected compact flow style custom-tables, got:\n%s", compact)
	}
}
