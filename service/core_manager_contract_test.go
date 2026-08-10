package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCoreManagerContracts(t *testing.T) {
	t.Run("legacy sing-box preference migration is idempotent", func(t *testing.T) {
		legacy := `{"target":{"os":"linux","arch":"amd64","libc":"musl","amd64Level":"v3"},"customUrl":"https://example.com/sing-box.tar.gz"}`
		migrated, changed, err := migrateLegacySingboxCoreDownloadPreferenceJSON(legacy)
		if err != nil {
			t.Fatalf("migrate legacy preference: %v", err)
		}
		if !changed {
			t.Fatal("expected legacy preference to be migrated")
		}

		var preference SingboxCoreDownloadPreference
		if err := json.Unmarshal([]byte(migrated), &preference); err != nil {
			t.Fatalf("decode migrated preference: %v", err)
		}
		if preference.Target.OS != "linux" || preference.Target.Arch != "amd64" || preference.Target.Libc != "musl" {
			t.Fatalf("unexpected migrated target: %+v", preference.Target)
		}
		if preference.CustomURL != "https://example.com/sing-box.tar.gz" {
			t.Fatalf("custom URL was not preserved: %q", preference.CustomURL)
		}
		if strings.Contains(migrated, "amd64Level") {
			t.Fatalf("migrated preference still contains amd64Level: %s", migrated)
		}

		second, changedAgain, err := migrateLegacySingboxCoreDownloadPreferenceJSON(migrated)
		if err != nil {
			t.Fatalf("repeat migration: %v", err)
		}
		if changedAgain || second != migrated {
			t.Fatalf("migration is not idempotent: changed=%v first=%s second=%s", changedAgain, migrated, second)
		}
	})

	t.Run("status JSON contracts are independent", func(t *testing.T) {
		singboxJSON, err := json.Marshal(SingboxCoreInfo{
			Installed:        true,
			Compatible:       true,
			InstalledTarget:  SingboxCoreDownloadTarget{OS: "linux", Arch: "amd64", Libc: "glibc"},
			InstalledChannel: singboxReleaseChannelAlpha,
			DownloadPreference: SingboxCoreDownloadPreference{
				Target: SingboxCoreDownloadTarget{OS: "linux", Arch: "amd64", Libc: "musl"},
			},
		})
		if err != nil {
			t.Fatalf("marshal sing-box status: %v", err)
		}
		if strings.Contains(string(singboxJSON), "amd64Level") {
			t.Fatalf("sing-box status exposes amd64Level: %s", singboxJSON)
		}
		if !strings.Contains(string(singboxJSON), `"installed":true`) || !strings.Contains(string(singboxJSON), `"compatible":true`) {
			t.Fatalf("sing-box status lost installed/compatible fields: %s", singboxJSON)
		}
		if !strings.Contains(string(singboxJSON), `"installedChannel":"alpha"`) {
			t.Fatalf("sing-box status lost installed channel: %s", singboxJSON)
		}

		mihomoJSON, err := json.Marshal(MihomoCoreInfo{
			Installed:        true,
			Compatible:       false,
			InstalledTarget:  MihomoCoreDownloadTarget{OS: "linux", Arch: "amd64", Amd64Level: "v2"},
			InstalledChannel: mihomoReleaseChannelAlpha,
			DownloadPreference: MihomoCoreDownloadPreference{
				Target: MihomoCoreDownloadTarget{OS: "linux", Arch: "amd64", Amd64Level: "v3"},
			},
		})
		if err != nil {
			t.Fatalf("marshal Mihomo status: %v", err)
		}
		if !strings.Contains(string(mihomoJSON), `"amd64Level":"v2"`) ||
			!strings.Contains(string(mihomoJSON), `"amd64Level":"v3"`) {
			t.Fatalf("Mihomo status lost AMD64 levels: %s", mihomoJSON)
		}
		if !strings.Contains(string(mihomoJSON), `"installedChannel":"alpha"`) {
			t.Fatalf("Mihomo status lost installed channel: %s", mihomoJSON)
		}
		if !strings.Contains(string(mihomoJSON), `"installed":true`) || !strings.Contains(string(mihomoJSON), `"compatible":false`) {
			t.Fatalf("Mihomo status lost installed/compatible fields: %s", mihomoJSON)
		}

		undetectedJSON, err := json.Marshal(SingboxCoreInfo{Installed: true, Compatible: false})
		if err != nil {
			t.Fatalf("marshal undetected sing-box status: %v", err)
		}
		if strings.Contains(string(undetectedJSON), `"arch":"amd64"`) {
			t.Fatalf("undetected status fabricated a host architecture: %s", undetectedJSON)
		}
	})

	t.Run("Mihomo status preference backfills missing runtime target fields", func(t *testing.T) {
		svc := &MihomoCoreManagerService{}
		normalized := svc.normalizeStatusDownloadPreference(MihomoCoreDownloadPreference{
			Target:    MihomoCoreDownloadTarget{Arch: "amd64"},
			CustomURL: " https://example.com/mihomo.gz ",
		})
		wantTarget := svc.normalizeDownloadTarget(MihomoCoreDownloadTarget{Arch: "amd64"})
		if normalized.Target != wantTarget {
			t.Fatalf("normalized Mihomo target mismatch: got=%+v want=%+v", normalized.Target, wantTarget)
		}
		if normalized.CustomURL != "https://example.com/mihomo.gz" {
			t.Fatalf("normalized Mihomo custom URL mismatch: %q", normalized.CustomURL)
		}
	})
}

func TestCoreAssetSelectionContracts(t *testing.T) {
	t.Run("sing-box libc variants", func(t *testing.T) {
		assets := []GitHubAsset{
			{Name: "sing-box-1.14.0-linux-amd64.tar.gz"},
			{Name: "sing-box-1.14.0-linux-amd64-glibc.tar.gz"},
			{Name: "sing-box-1.14.0-linux-amd64-musl.tar.gz"},
		}
		tests := []struct {
			libc string
			want string
		}{
			{libc: "glibc", want: "sing-box-1.14.0-linux-amd64-glibc.tar.gz"},
			{libc: "musl", want: "sing-box-1.14.0-linux-amd64-musl.tar.gz"},
			{libc: "universal", want: "sing-box-1.14.0-linux-amd64.tar.gz"},
		}
		for _, test := range tests {
			asset, ok := pickSingboxAssetFromAssets("v1.14.0", assets, SingboxCoreDownloadTarget{
				OS: "linux", Arch: "amd64", Libc: test.libc,
			})
			if !ok || asset.Name != test.want {
				t.Fatalf("libc=%s: got=%q ok=%v want=%q", test.libc, asset.Name, ok, test.want)
			}
		}
	})

	t.Run("sing-box explicit libc does not fall back", func(t *testing.T) {
		assets := []GitHubAsset{
			{Name: "sing-box-1.14.0-linux-amd64.tar.gz"},
		}
		if _, ok := pickSingboxAssetFromAssets("v1.14.0", assets, SingboxCoreDownloadTarget{
			OS: "linux", Arch: "amd64", Libc: "glibc",
		}); ok {
			t.Fatal("explicit glibc target unexpectedly fell back to universal asset")
		}
		if _, ok := pickSingboxAssetFromAssets("v1.14.0", assets, SingboxCoreDownloadTarget{
			OS: "linux", Arch: "amd64", Libc: "musl",
		}); ok {
			t.Fatal("explicit musl target unexpectedly fell back to universal asset")
		}
	})

	t.Run("Mihomo AMD64 levels", func(t *testing.T) {
		assets := []GitHubAsset{
			{Name: "mihomo-linux-amd64-v1-v1.19.10.gz"},
			{Name: "mihomo-linux-amd64-v2-v1.19.10.gz"},
			{Name: "mihomo-linux-amd64-v3-v1.19.10.gz"},
		}
		for _, level := range []string{"v1", "v2", "v3"} {
			asset, ok := pickMihomoAssetFromAssets(assets, MihomoCoreDownloadTarget{
				OS: "linux", Arch: "amd64", Amd64Level: level,
			})
			want := "mihomo-linux-amd64-" + level + "-v1.19.10.gz"
			if !ok || asset.Name != want {
				t.Fatalf("level=%s: got=%q ok=%v want=%q", level, asset.Name, ok, want)
			}
		}
	})

	t.Run("Mihomo explicit AMD64 level does not fall back", func(t *testing.T) {
		assets := []GitHubAsset{
			{Name: "mihomo-linux-amd64-v1-v1.19.10.gz"},
			{Name: "mihomo-linux-amd64-v1.19.10.gz"},
		}
		if _, ok := pickMihomoAssetFromAssets(assets, MihomoCoreDownloadTarget{
			OS: "linux", Arch: "amd64", Amd64Level: "v2",
		}); ok {
			t.Fatal("explicit v2 target unexpectedly fell back to a non-v2 Mihomo asset")
		}
		if _, ok := pickMihomoAssetFromAssets(assets, MihomoCoreDownloadTarget{
			OS: "linux", Arch: "amd64", Amd64Level: "v3",
		}); ok {
			t.Fatal("explicit v3 target unexpectedly fell back to a non-v3 Mihomo asset")
		}
	})

	t.Run("Mihomo rolling assets keep the requested AMD64 level", func(t *testing.T) {
		assets := []GitHubAsset{
			{Name: "mihomo-linux-amd64-compatible-alpha-cf98d2d.gz"},
			{Name: "mihomo-linux-amd64-v1-alpha-cf98d2d.gz"},
			{Name: "mihomo-linux-amd64-v1-go123-alpha-cf98d2d.gz"},
			{Name: "mihomo-linux-amd64-v2-alpha-cf98d2d.gz"},
			{Name: "mihomo-linux-amd64-v2-go123-alpha-cf98d2d.gz"},
			{Name: "mihomo-linux-amd64-v3-alpha-cf98d2d.gz"},
			{Name: "mihomo-linux-amd64-v3-go123-alpha-cf98d2d.gz"},
		}
		for _, test := range []struct {
			level string
			want  string
		}{
			{level: "v1", want: "mihomo-linux-amd64-v1-alpha-cf98d2d.gz"},
			{level: "v2", want: "mihomo-linux-amd64-v2-alpha-cf98d2d.gz"},
			{level: "v3", want: "mihomo-linux-amd64-v3-alpha-cf98d2d.gz"},
		} {
			asset, ok := pickMihomoAssetFromAssets(assets, MihomoCoreDownloadTarget{
				OS: "linux", Arch: "amd64", Amd64Level: test.level,
			})
			if !ok || asset.Name != test.want {
				t.Fatalf("level=%s: got=%q ok=%v want=%q", test.level, asset.Name, ok, test.want)
			}
		}
	})

	t.Run("sing-box update info exposes auto update fields", func(t *testing.T) {
		payload, err := json.Marshal(SingboxCoreUpdateInfo{
			Enabled:                 true,
			IntervalHours:           12,
			AutoUpdateEnabled:       true,
			AutoUpdateDisabled:      true,
			AutoUpdateDisableReason: "本地未安装 sing-box 内核",
			AutoUpdateLastAttemptAt: 100,
			AutoUpdateLastSuccessAt: 200,
			AutoUpdateError:         "下载失败",
			AutoUpdateErrorAt:       300,
		})
		if err != nil {
			t.Fatalf("marshal sing-box update info: %v", err)
		}
		jsonText := string(payload)
		for _, fragment := range []string{
			`"autoUpdateEnabled":true`,
			`"autoUpdateDisabled":true`,
			`"autoUpdateDisableReason":"本地未安装 sing-box 内核"`,
			`"autoUpdateLastAttemptAt":100`,
			`"autoUpdateLastSuccessAt":200`,
			`"autoUpdateError":"下载失败"`,
			`"autoUpdateErrorAt":300`,
		} {
			if !strings.Contains(jsonText, fragment) {
				t.Fatalf("sing-box update info lost field %s: %s", fragment, jsonText)
			}
		}
	})

	t.Run("Mihomo update info exposes auto update fields", func(t *testing.T) {
		payload, err := json.Marshal(MihomoCoreUpdateInfo{
			Enabled:                 true,
			IntervalHours:           12,
			AutoUpdateEnabled:       true,
			AutoUpdateDisabled:      true,
			AutoUpdateDisableReason: "missing local Mihomo core",
			AutoUpdateLastAttemptAt: 101,
			AutoUpdateLastSuccessAt: 202,
			AutoUpdateError:         "download failed",
			AutoUpdateErrorAt:       303,
		})
		if err != nil {
			t.Fatalf("marshal Mihomo update info: %v", err)
		}
		jsonText := string(payload)
		for _, fragment := range []string{
			`"autoUpdateEnabled":true`,
			`"autoUpdateDisabled":true`,
			`"autoUpdateDisableReason":"missing local Mihomo core"`,
			`"autoUpdateLastAttemptAt":101`,
			`"autoUpdateLastSuccessAt":202`,
			`"autoUpdateError":"download failed"`,
			`"autoUpdateErrorAt":303`,
		} {
			if !strings.Contains(jsonText, fragment) {
				t.Fatalf("Mihomo update info lost field %s: %s", fragment, jsonText)
			}
		}
	})
}
