package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractMihomoVersion(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "stable build output",
			output: "Mihomo Meta v1.19.29 linux amd64 with go1.26.0 2026-08-10",
			want:   "v1.19.29",
		},
		{
			name:   "alpha build output",
			output: "Mihomo Meta alpha-cf98d2d linux amd64 with go1.26.0 2026-08-10",
			want:   "alpha-cf98d2d",
		},
		{
			name:   "legacy alpha output",
			output: "mihomo version alpha-cf98d2d",
			want:   "alpha-cf98d2d",
		},
		{
			name:   "do not capture Go version",
			output: "Mihomo Meta unknown linux amd64 with go1.26.0",
			want:   "",
		},
		{
			name:   "legacy unknown output does not capture separate Go version",
			output: "mihomo version unknown with go version 1.26.0",
			want:   "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := extractMihomoVersion(test.output); got != test.want {
				t.Fatalf("extractMihomoVersion(%q) = %q, want %q", test.output, got, test.want)
			}
		})
	}
}

func TestMihomoReleaseVersionIdentity(t *testing.T) {
	stable := GitHubRelease{TagName: "v1.19.29"}
	if got := mihomoReleaseVersionIdentity(stable); got != "v1.19.29" {
		t.Fatalf("stable release identity = %q, want v1.19.29", got)
	}

	alpha := GitHubRelease{
		TagName:    "Prerelease-Alpha",
		Prerelease: true,
		Assets: []GitHubAsset{
			{Name: "mihomo-linux-amd64-v3-alpha-cf98d2d.gz"},
		},
	}
	if got := mihomoReleaseVersionIdentity(alpha); got != "alpha-cf98d2d" {
		t.Fatalf("alpha release identity = %q, want alpha-cf98d2d", got)
	}

	fallback := GitHubRelease{
		TagName:    "Prerelease-Alpha",
		Prerelease: true,
		UpdatedAt:  "2026-08-10T12:34:56Z",
	}
	if got := mihomoReleaseVersionIdentity(fallback); got != "alpha@2026-08-10T12:34:56Z" {
		t.Fatalf("alpha fallback identity = %q, want updated-at identity", got)
	}

	betaFallback := GitHubRelease{
		TagName:    "Prerelease-Beta",
		Prerelease: true,
		UpdatedAt:  "2026-08-10T12:34:56Z",
	}
	if got := mihomoReleaseVersionIdentity(betaFallback); got != "beta@2026-08-10T12:34:56Z" {
		t.Fatalf("beta fallback identity = %q, want updated-at identity", got)
	}
}

func TestMihomoRemoteVersionIsNewer(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		local  string
		want   bool
	}{
		{name: "stable newer", remote: "v1.19.30", local: "v1.19.29", want: true},
		{name: "stable older", remote: "v1.19.28", local: "v1.19.29", want: false},
		{name: "stable equal", remote: "v1.19.29", local: "v1.19.29", want: false},
		{name: "alpha changed", remote: "alpha-def4567", local: "alpha-abc1234", want: true},
		{name: "alpha equal", remote: "alpha-abc1234", local: "alpha-abc1234", want: false},
		{name: "timestamp fallback is conservative", remote: "alpha@2026-08-10T12:34:56Z", local: "alpha-abc1234", want: false},
		{name: "unresolved prerelease tag is conservative", remote: "Prerelease-Alpha", local: "alpha-abc1234", want: false},
		{name: "unresolved beta tag is conservative", remote: "Prerelease-Beta", local: "beta-abc1234", want: false},
		{name: "cross channel identities are conservative", remote: "v1.19.30", local: "alpha-abc1234", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := mihomoRemoteVersionIsNewer(test.remote, test.local); got != test.want {
				t.Fatalf("mihomoRemoteVersionIsNewer(%q, %q) = %v, want %v", test.remote, test.local, got, test.want)
			}
		})
	}
}

func TestNormalizeMihomoInstalledTargetDoesNotUseDownloadPreference(t *testing.T) {
	installed := normalizeMihomoInstalledTarget(MihomoCoreDownloadTarget{
		OS:   "linux",
		Arch: "amd64",
	})
	if installed.Amd64Level != "" {
		t.Fatalf("unknown installed AMD64 level was fabricated: %+v", installed)
	}

	preference := (&MihomoCoreManagerService{}).normalizeStatusDownloadPreference(MihomoCoreDownloadPreference{
		Target: MihomoCoreDownloadTarget{OS: "linux", Arch: "amd64", Amd64Level: "v3"},
	})
	if preference.Target.Amd64Level == "" {
		t.Fatalf("download preference unexpectedly lost the selected AMD64 level: %+v", preference.Target)
	}
}

func TestNormalizeMihomoVersionFilterTargetPreservesOmittedAMD64Level(t *testing.T) {
	svc := &MihomoCoreManagerService{}
	got := svc.normalizeMihomoVersionFilterTarget(MihomoCoreDownloadTarget{
		OS:   "linux",
		Arch: "amd64",
	})
	if got.Amd64Level != "" {
		t.Fatalf("version filter fabricated AMD64 level: %+v", got)
	}

	got = svc.normalizeMihomoVersionFilterTarget(MihomoCoreDownloadTarget{
		OS:         "linux",
		Arch:       "amd64",
		Amd64Level: "v2",
	})
	if got.Amd64Level != "v2" {
		t.Fatalf("explicit AMD64 level was not preserved: %+v", got)
	}
}

func TestPickMihomoAssetWithoutAMD64LevelDoesNotInferOne(t *testing.T) {
	asset, ok := pickMihomoAssetFromAssets([]GitHubAsset{
		{Name: "mihomo-linux-amd64-alpha-cf98d2d.gz"},
	}, MihomoCoreDownloadTarget{
		OS:   "linux",
		Arch: "amd64",
	})
	if !ok || asset.Name != "mihomo-linux-amd64-alpha-cf98d2d.gz" {
		t.Fatalf("unqualified AMD64 version filter rejected the only matching asset: %+v, ok=%v", asset, ok)
	}
}

func TestFetchMihomoReleaseVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/version.txt" {
			t.Fatalf("unexpected version path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("alpha-cf98d2d\n"))
	}))
	defer server.Close()

	release := GitHubRelease{
		Prerelease: true,
		Assets: []GitHubAsset{
			{Name: "version.txt", BrowserDownloadURL: server.URL + "/version.txt"},
		},
	}
	version, err := (&MihomoCoreManagerService{}).fetchMihomoReleaseVersion(server.Client(), release)
	if err != nil {
		t.Fatalf("fetch Mihomo release version: %v", err)
	}
	if version != "alpha-cf98d2d" {
		t.Fatalf("release version = %q, want alpha-cf98d2d", version)
	}
}

func TestFetchMihomoReleaseVersionUsesFirstVersionToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("alpha-cf98d2d\nmetadata follows\n"))
	}))
	defer server.Close()

	release := GitHubRelease{
		Prerelease: true,
		Assets: []GitHubAsset{
			{Name: "version.txt", BrowserDownloadURL: server.URL},
		},
	}
	version, err := (&MihomoCoreManagerService{}).fetchMihomoReleaseVersion(server.Client(), release)
	if err != nil {
		t.Fatalf("fetch Mihomo release version: %v", err)
	}
	if version != "alpha-cf98d2d" {
		t.Fatalf("release version = %q, want alpha-cf98d2d", version)
	}
}

func TestSelectMihomoPendingUpdateForChannel(t *testing.T) {
	stablePending, alphaPending := selectMihomoPendingUpdateForChannel(
		&MihomoCoreInfo{
			LocalVersion:     "v1.19.29",
			InstalledChannel: mihomoReleaseChannelStable,
		},
		"v1.19.30",
		"alpha-def4567",
	)
	if stablePending != "v1.19.30" || alphaPending != "" {
		t.Fatalf("stable pending selection = %q/%q, want v1.19.30/empty", stablePending, alphaPending)
	}

	stablePending, alphaPending = selectMihomoPendingUpdateForChannel(
		&MihomoCoreInfo{
			LocalVersion:     "alpha-abc1234",
			InstalledChannel: mihomoReleaseChannelAlpha,
		},
		"v1.19.30",
		"alpha-def4567",
	)
	if stablePending != "" || alphaPending != "alpha-def4567" {
		t.Fatalf("alpha pending selection = %q/%q, want empty/alpha-def4567", stablePending, alphaPending)
	}
}
