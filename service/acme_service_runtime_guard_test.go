package service

import (
	"strings"
	"testing"
)

func TestValidateAcmeCustomArgsAllowsOnlyNonManagedFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		wantErr bool
	}{
		{name: "debug and dns sleep", args: "--debug 2 --dnssleep 90"},
		{name: "config home", args: "--config-home /tmp/acme", wantErr: true},
		{name: "config home equals", args: "--config-home=/tmp/acme", wantErr: true},
		{name: "alternate CA", args: "--server https://example.invalid/directory", wantErr: true},
		{name: "alternate domain", args: "-d other.example.com", wantErr: true},
		{name: "hook", args: "--pre-hook=echo", wantErr: true},
		{name: "insecure output", args: "--output-insecure", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := validateAcmeCustomArgs(test.args)
			if test.wantErr {
				if err == nil {
					t.Fatalf("expected validation error for %q", test.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate custom args failed: %v", err)
			}
			if got != test.args {
				t.Fatalf("unexpected normalized args: got=%q want=%q", got, test.args)
			}
		})
	}
}

func TestBuildAcmeCommandEnvFiltersRuntimeOverrides(t *testing.T) {
	t.Setenv("LE_CONFIG_HOME", "/unsafe/config")
	t.Setenv("ACME_NO_COLOR", "1")
	t.Setenv("HOME", "/unsafe/home")
	t.Setenv("SAFE_INHERITED", "kept")

	env := envPairsToEnvMap(buildAcmeCommandEnv([]string{
		"CF_Token=dns-secret",
		"SAFE_DNS=value",
		"LE_WORKING_DIR=/unsafe/work",
		"PATH=/unsafe/path",
	}))
	for _, key := range []string{"LE_CONFIG_HOME", "ACME_NO_COLOR", "HOME", "LE_WORKING_DIR"} {
		if _, exists := env[key]; exists {
			t.Fatalf("managed runtime variable %s must be filtered: %#v", key, env)
		}
	}
	if got := env["SAFE_INHERITED"]; got != "kept" {
		t.Fatalf("safe inherited variable was lost: %q", got)
	}
	if got := env["CF_Token"]; got != "dns-secret" {
		t.Fatalf("DNS credential was not provided to acme command: %q", got)
	}
	if got := env["SAFE_DNS"]; got != "value" {
		t.Fatalf("safe DNS variable was not provided to acme command: %q", got)
	}
}

func TestRedactAcmeCommandOutputMasksDNSCredentials(t *testing.T) {
	output := redactAcmeCommandOutput(
		"provider received CF_Token=very-secret and Ali_Key=another-secret; account=public-id",
		[]string{"CF_Token=very-secret", "Ali_Key=another-secret", "CF_Account_ID=public-id"},
	)
	if strings.Contains(output, "very-secret") || strings.Contains(output, "another-secret") {
		t.Fatalf("DNS secret leaked through command output: %q", output)
	}
	if !strings.Contains(output, acmeMaskedEnvValue) {
		t.Fatalf("expected masked credential marker in output: %q", output)
	}
	if !strings.Contains(output, "public-id") {
		t.Fatalf("non-secret output should be preserved: %q", output)
	}
}
