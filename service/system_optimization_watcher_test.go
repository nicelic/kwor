package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOptimizationWatcherGoldenSaveReadRemove(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KWOR_DATA_FOLDER", tmpDir)

	goldenName := "test_golden.conf"
	content := "net.ipv4.tcp_fastopen=3\n"

	if err := SaveOptimizationGoldenFile(goldenName, content); err != nil {
		t.Fatalf("SaveOptimizationGoldenFile failed: %v", err)
	}

	readBack, err := ReadOptimizationGoldenFile(goldenName)
	if err != nil {
		t.Fatalf("ReadOptimizationGoldenFile failed: %v", err)
	}
	if readBack != content {
		t.Fatalf("expected %q, got %q", content, readBack)
	}

	if err := RemoveOptimizationGoldenFile(goldenName); err != nil {
		t.Fatalf("RemoveOptimizationGoldenFile failed: %v", err)
	}

	if _, err := ReadOptimizationGoldenFile(goldenName); err == nil {
		t.Fatalf("expected error after golden file removal, got nil")
	}
}

func TestOptimizationWatcherAutoCorrection(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KWOR_DATA_FOLDER", tmpDir)

	goldenName := "sysctl_test.conf"
	goldenContent := "net.core.default_qdisc=cake\nnet.ipv4.tcp_congestion_control=bbr\n"

	if err := SaveOptimizationGoldenFile(goldenName, goldenContent); err != nil {
		t.Fatalf("SaveOptimizationGoldenFile failed: %v", err)
	}

	targetFile := filepath.Join(tmpDir, "sysctl.conf")
	// 初始写入被篡改的内容
	tamperedContent := "net.ipv4.tcp_congestion_control=cubic\n"
	if err := os.WriteFile(targetFile, []byte(tamperedContent), 0o644); err != nil {
		t.Fatalf("failed to write tampered file: %v", err)
	}

	correctedCalled := false
	watcher := &SystemOptimizationWatcher{
		items:  make(map[string]*OptimizationWatchItem),
		wakeCh: make(chan struct{}, 1),
	}

	watcher.Register(OptimizationWatchItem{
		Key:            "test-sysctl",
		DisplayName:    "测试 sysctl",
		TargetPath:     targetFile,
		GoldenFileName: goldenName,
		OnCorrected: func(ctx context.Context, correctedPath string) error {
			correctedCalled = true
			if correctedPath != targetFile {
				t.Errorf("expected %s, got %s", targetFile, correctedPath)
			}
			return nil
		},
	})

	// 执行一次检查与纠正
	watcher.checkAndCorrectAll()

	// 验证目标文件是否已被自动纠正为母本内容
	correctedData, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("failed to read target file: %v", err)
	}
	if string(correctedData) != goldenContent {
		t.Fatalf("file content not corrected: expected %q, got %q", goldenContent, string(correctedData))
	}

	if !correctedCalled {
		t.Fatalf("expected OnCorrected callback to be called")
	}
}

func TestOptimizationWatcherAutoCorrectionOnMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KWOR_DATA_FOLDER", tmpDir)

	goldenName := "resolv_test.conf"
	goldenContent := "nameserver 1.1.1.1\nnameserver 8.8.8.8\n"

	if err := SaveOptimizationGoldenFile(goldenName, goldenContent); err != nil {
		t.Fatalf("SaveOptimizationGoldenFile failed: %v", err)
	}

	targetFile := filepath.Join(tmpDir, "resolv.conf")
	// 确保目标文件不存在
	_ = os.Remove(targetFile)

	watcher := &SystemOptimizationWatcher{
		items:  make(map[string]*OptimizationWatchItem),
		wakeCh: make(chan struct{}, 1),
	}

	watcher.Register(OptimizationWatchItem{
		Key:            "test-dns",
		DisplayName:    "测试 DNS",
		TargetPath:     targetFile,
		GoldenFileName: goldenName,
	})

	watcher.checkAndCorrectAll()

	restored, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("target file was not created by correction: %v", err)
	}
	if string(restored) != goldenContent {
		t.Fatalf("restored content mismatch: expected %q, got %q", goldenContent, string(restored))
	}
}

func TestOptimizationWatcherPreciseFileDeletion(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("KWOR_DATA_FOLDER", tmpDir)

	// 同时落盘三个开关的文件以及一个模拟的 MTU 脚本
	files := map[string]string{
		GoldenSysctlMain: "net.ipv4.tcp_congestion_control=bbr\n",
		GoldenJournald:   "[Journal]\nStorage=none\n",
		GoldenDNS:        "nameserver 1.1.1.1\n",
		"_set_mtu_.sh":   "#!/bin/bash\nip link set eth0 mtu 1470\n",
	}

	for name, content := range files {
		if err := SaveOptimizationGoldenFile(name, content); err != nil {
			t.Fatalf("SaveOptimizationGoldenFile failed for %s: %v", name, err)
		}
	}

	goldenDir := GetOptimizationGoldenDir()
	if _, err := os.Stat(goldenDir); err != nil {
		t.Fatalf("expected golden dir to exist, got: %v", err)
	}

	// 1. 模拟关闭 sysctl 开关：仅删除 sysctl.conf
	if err := RemoveOptimizationGoldenFile(GoldenSysctlMain); err != nil {
		t.Fatalf("RemoveOptimizationGoldenFile failed: %v", err)
	}

	// 验证 sysctl.conf 已删除
	if _, err := os.Stat(GetOptimizationGoldenPath(GoldenSysctlMain)); !os.IsNotExist(err) {
		t.Fatalf("expected sysctl.conf to be removed")
	}

	// 验证其他三个文件和目录均完好存在，未被误删
	for _, remaining := range []string{GoldenJournald, GoldenDNS, "_set_mtu_.sh"} {
		if _, err := os.Stat(GetOptimizationGoldenPath(remaining)); err != nil {
			t.Fatalf("expected %s to still exist, but got: %v", remaining, err)
		}
	}
	if _, err := os.Stat(goldenDir); err != nil {
		t.Fatalf("expected golden dir to NEVER be deleted, but got: %v", err)
	}

	// 2. 模拟关闭 journald 开关：仅删除 journald.conf
	if err := RemoveOptimizationGoldenFile(GoldenJournald); err != nil {
		t.Fatalf("RemoveOptimizationGoldenFile failed: %v", err)
	}

	if _, err := os.Stat(GetOptimizationGoldenPath(GoldenJournald)); !os.IsNotExist(err) {
		t.Fatalf("expected journald.conf to be removed")
	}
	for _, remaining := range []string{GoldenDNS, "_set_mtu_.sh"} {
		if _, err := os.Stat(GetOptimizationGoldenPath(remaining)); err != nil {
			t.Fatalf("expected %s to still exist, but got: %v", remaining, err)
		}
	}
	if _, err := os.Stat(goldenDir); err != nil {
		t.Fatalf("expected golden dir to NEVER be deleted, but got: %v", err)
	}

	// 3. 模拟关闭 dns 开关：仅删除 resolv.conf
	if err := RemoveOptimizationGoldenFile(GoldenDNS); err != nil {
		t.Fatalf("RemoveOptimizationGoldenFile failed: %v", err)
	}

	if _, err := os.Stat(GetOptimizationGoldenPath(GoldenDNS)); !os.IsNotExist(err) {
		t.Fatalf("expected resolv.conf to be removed")
	}
	// MTU 脚本与目录永远保留
	if _, err := os.Stat(GetOptimizationGoldenPath("_set_mtu_.sh")); err != nil {
		t.Fatalf("expected _set_mtu_.sh to still exist, but got: %v", err)
	}
	if _, err := os.Stat(goldenDir); err != nil {
		t.Fatalf("expected golden dir to NEVER be deleted, but got: %v", err)
	}
}
