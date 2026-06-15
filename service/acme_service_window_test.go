package service

import (
	"path"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func TestComputeAutoRenewWindowSecondsOverFortyDaysUsesThirtyDays(t *testing.T) {
	now := time.Now().Unix()
	entry := &model.AcmeCertificate{
		NotBefore: now,
		NotAfter:  now + 90*24*3600,
	}

	got := computeAutoRenewWindowSeconds(entry)
	want := int64(30 * 24 * 3600)
	if got != want {
		t.Fatalf("unexpected window seconds: got=%d want=%d", got, want)
	}
}

func TestComputeAutoRenewWindowSecondsShortValidityUsesOneThirdFloor(t *testing.T) {
	now := time.Now().Unix()
	entry := &model.AcmeCertificate{
		NotBefore: now,
		NotAfter:  now + 7*24*3600,
	}

	got := computeAutoRenewWindowSeconds(entry)
	want := int64(2 * 24 * 3600)
	if got != want {
		t.Fatalf("unexpected window seconds for 7-day cert: got=%d want=%d", got, want)
	}
}

func TestComputeAutoRenewWindowSecondsAtLeastOneDay(t *testing.T) {
	now := time.Now().Unix()
	entry := &model.AcmeCertificate{
		NotBefore: now,
		NotAfter:  now + 2*24*3600,
	}

	got := computeAutoRenewWindowSeconds(entry)
	want := int64(1 * 24 * 3600)
	if got != want {
		t.Fatalf("unexpected window seconds for 2-day cert: got=%d want=%d", got, want)
	}
}

func TestShouldAutoRenewCertificateBoundaryInclusive(t *testing.T) {
	now := time.Now().Unix()
	entry := &model.AcmeCertificate{
		AutoRenew: true,
		NotBefore: now - 5*24*3600,
		NotAfter:  now + 2*24*3600,
	}

	window := int64(2 * 24 * 3600)
	if !shouldAutoRenewCertificate(entry, now, window) {
		t.Fatalf("expected renew to trigger when now+window equals notAfter")
	}
}

func TestNormalizeDefaultPushParentDir(t *testing.T) {
	got := normalizeDefaultPushParentDir("/opt/cert/1")
	if got != path.Clean("/opt/cert") {
		t.Fatalf("unexpected parent dir: got=%q", got)
	}

	root := normalizeDefaultPushParentDir("/opt")
	if root != path.Clean("/") {
		t.Fatalf("unexpected root normalize result: got=%q", root)
	}
}
