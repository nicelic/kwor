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
			Installed:       true,
			Compatible:      true,
			InstalledTarget: SingboxCoreDownloadTarget{OS: "linux", Arch: "amd64", Libc: "glibc"},
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

		mihomoJSON, err := json.Marshal(MihomoCoreInfo{
			Installed:       true,
			Compatible:      false,
			InstalledTarget: MihomoCoreDownloadTarget{OS: "linux", Arch: "amd64", Amd64Level: "v2"},
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
}
