package service

import (
	"fmt"
	"io"
	"os"
	"unicode/utf8"
)

const (
	// Optimization files are host configuration, not arbitrary documents.
	// Keep their retained SQLite/API/UI representation bounded.
	systemOptimizationContentMaxBytes     = 256 * 1024
	systemOptimizationNameServersMaxBytes = 16 * 1024
)

func validateSystemOptimizationContent(content string) error {
	if !utf8.ValidString(content) {
		return fmt.Errorf("优化配置必须为有效 UTF-8 文本")
	}
	if len(content) > systemOptimizationContentMaxBytes {
		return fmt.Errorf("优化配置内容不能超过 %d KiB", systemOptimizationContentMaxBytes/1024)
	}
	return nil
}

func validateSystemOptimizationNameServersInput(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("DNS 地址必须为有效 UTF-8 文本")
	}
	if len(value) > systemOptimizationNameServersMaxBytes {
		return fmt.Errorf("DNS 地址内容不能超过 %d KiB", systemOptimizationNameServersMaxBytes/1024)
	}
	return nil
}

func readSystemOptimizationTextFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, systemOptimizationContentMaxBytes+1))
	if err != nil {
		return "", err
	}
	if len(raw) > systemOptimizationContentMaxBytes {
		return "", fmt.Errorf("优化配置文件不能超过 %d KiB", systemOptimizationContentMaxBytes/1024)
	}
	content := string(raw)
	if err := validateSystemOptimizationContent(content); err != nil {
		return "", err
	}
	return content, nil
}
