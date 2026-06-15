package service

import (
	"reflect"
	"testing"
)

func TestExtractActiveLinuxNameServers(t *testing.T) {
	content := "#nameserver 1.1.1.1\n  # nameserver 9.9.9.9\nnameserver 8.8.8.8\nnameserver 1.1.1.1 # keep\n"
	got := extractActiveLinuxNameServers(content)
	want := []string{"8.8.8.8", "1.1.1.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extractActiveLinuxNameServers = %#v, want %#v", got, want)
	}
}

func TestReplaceActiveLinuxNameServers(t *testing.T) {
	content := "# head\nnameserver 1.1.1.1\noptions edns0\n#nameserver 9.9.9.9\nnameserver 8.8.8.8\n"
	got := replaceActiveLinuxNameServers(content, []string{"4.4.4.4"})
	want := "# head\nnameserver 4.4.4.4\noptions edns0\n#nameserver 9.9.9.9\n"
	if got != want {
		t.Fatalf("replaceActiveLinuxNameServers = %q, want %q", got, want)
	}
}

func TestReplaceActiveLinuxNameServersWhenNoActive(t *testing.T) {
	content := "# only comments\noptions rotate\n"
	got := replaceActiveLinuxNameServers(content, []string{"1.1.1.1", "8.8.8.8"})
	want := "# only comments\noptions rotate\nnameserver 1.1.1.1\nnameserver 8.8.8.8\n"
	if got != want {
		t.Fatalf("replaceActiveLinuxNameServers(no-active) = %q, want %q", got, want)
	}
}

func TestNormalizeLinuxNameServerInput(t *testing.T) {
	got := normalizeLinuxNameServerInput("1.1.1.1, 8.8.8.8\n9.9.9.9")
	want := []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeLinuxNameServerInput = %#v, want %#v", got, want)
	}
}
