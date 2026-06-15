package cmd

import "testing"

func TestShouldRemoveInstallDirAfterUninstall(t *testing.T) {
	testCases := []struct {
		name string
		dir  string
		want bool
	}{
		{
			name: "dedicated kwor dir under opt must not be removed",
			dir:  "/opt/kwor",
			want: false,
		},
		{
			name: "legacy s-ui dir under usr local must not be removed",
			dir:  "/usr/local/s-ui",
			want: false,
		},
		{
			name: "public opt dir must not be removed",
			dir:  "/opt",
			want: false,
		},
		{
			name: "protected home dir must not be removed",
			dir:  "/home",
			want: false,
		},
		{
			name: "generic custom dir without app name must not be removed",
			dir:  "/data/releases/current",
			want: false,
		},
		{
			name: "windows program files must not be removed",
			dir:  "C:/Program Files",
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRemoveInstallDirAfterUninstall(tc.dir)
			if got != tc.want {
				t.Fatalf("shouldRemoveInstallDirAfterUninstall(%q) = %v, want %v", tc.dir, got, tc.want)
			}
		})
	}
}
