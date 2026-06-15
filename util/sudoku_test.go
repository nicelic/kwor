package util

import (
	"reflect"
	"testing"
)

func TestNormalizeSudokuCustomTable(t *testing.T) {
	if got := NormalizeSudokuCustomTable(" xpxvvpvv "); got != "xpxvvpvv" {
		t.Fatalf("expected normalized custom table, got %q", got)
	}
	if got := NormalizeSudokuCustomTable("invalid-table"); got != "" {
		t.Fatalf("expected invalid custom table to be dropped, got %q", got)
	}
	if got := NormalizeSudokuCustomTable(`["xpxvvpvv"]`); got != "xpxvvpvv" {
		t.Fatalf("expected JSON style value to be parsed, got %q", got)
	}
}

func TestNormalizeSudokuCustomTables(t *testing.T) {
	got := NormalizeSudokuCustomTables(`["xpxvvpvv", "vxpvxvvp", "bad", "xpxvvpvv"]`)
	want := []string{"xpxvvpvv", "vxpvxvvp"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected custom tables: got=%#v want=%#v", got, want)
	}

	got = NormalizeSudokuCustomTables(`["xpxvvpvv"，"vxpvxvvp"]`)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected full-width comma input to be normalized, got=%#v want=%#v", got, want)
	}

	got = NormalizeSudokuCustomTables([]string{"vvxxpvxp", "xvvvxxpp"})
	if got != nil {
		t.Fatalf("expected invalid custom tables to be dropped, got=%#v", got)
	}

	got = NormalizeSudokuCustomTables(`["vvxxpvxp","xvvvxxpp"]`)
	if got != nil {
		t.Fatalf("expected invalid JSON custom tables to be dropped, got=%#v", got)
	}
}

func TestNormalizeSudokuTableTypeForCustom(t *testing.T) {
	if got := NormalizeSudokuTableTypeForCustom("prefer_ascii", true); got != "prefer_entropy" {
		t.Fatalf("expected prefer_ascii to switch to prefer_entropy when custom table exists, got %q", got)
	}
	if got := NormalizeSudokuTableTypeForCustom("prefer_entropy", true); got != "prefer_entropy" {
		t.Fatalf("expected prefer_entropy to stay unchanged, got %q", got)
	}
	if got := NormalizeSudokuTableTypeForCustom("", false); got != "prefer_ascii" {
		t.Fatalf("expected default table type prefer_ascii when no custom table, got %q", got)
	}
}
