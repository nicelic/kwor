package web

import "testing"

func TestSessionCookieNameIsStableAndInstanceScoped(t *testing.T) {
	first := sessionCookieName([]byte("first-instance-secret"))
	if first != sessionCookieName([]byte("first-instance-secret")) {
		t.Fatal("session cookie name is not stable")
	}
	if first == sessionCookieName([]byte("second-instance-secret")) {
		t.Fatal("different instance secrets produced the same cookie name")
	}
	if len(first) != len("kwor_")+12 {
		t.Fatalf("unexpected session cookie name: %q", first)
	}
}
