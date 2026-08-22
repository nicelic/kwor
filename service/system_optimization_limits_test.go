package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSystemOptimizationContentBoundaries(t *testing.T) {
	if err := validateSystemOptimizationContent(strings.Repeat("a", systemOptimizationContentMaxBytes)); err != nil {
		t.Fatalf("content at the limit was rejected: %v", err)
	}
	if err := validateSystemOptimizationContent(strings.Repeat("a", systemOptimizationContentMaxBytes+1)); err == nil {
		t.Fatal("content over the limit was accepted")
	}
	if err := validateSystemOptimizationContent(string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 content was accepted")
	}
}

func TestValidateSystemOptimizationNameServersInputBoundaries(t *testing.T) {
	if err := validateSystemOptimizationNameServersInput(strings.Repeat("1", systemOptimizationNameServersMaxBytes)); err != nil {
		t.Fatalf("DNS input at the limit was rejected: %v", err)
	}
	if err := validateSystemOptimizationNameServersInput(strings.Repeat("1", systemOptimizationNameServersMaxBytes+1)); err == nil {
		t.Fatal("DNS input over the limit was accepted")
	}
	if err := validateSystemOptimizationNameServersInput(string([]byte{0xff})); err == nil {
		t.Fatal("invalid UTF-8 DNS input was accepted")
	}
}

func TestReadSystemOptimizationTextFileRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.conf")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", systemOptimizationContentMaxBytes+1)), 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	if _, err := readSystemOptimizationTextFile(path); err == nil {
		t.Fatal("oversized optimization file was accepted")
	}
}

func TestBoundedCommandBufferRetainsOnlyTheConfiguredLimit(t *testing.T) {
	buffer := &boundedCommandBuffer{maxBytes: 4}
	if n, err := buffer.Write([]byte("abcdef")); err != nil || n != 6 {
		t.Fatalf("Write() = (%d, %v), want (6, nil)", n, err)
	}
	if got := buffer.buffer.String(); got != "abcd" || !buffer.exceeded {
		t.Fatalf("buffer = %q, exceeded=%v", got, buffer.exceeded)
	}
}
