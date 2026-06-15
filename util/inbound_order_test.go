package util

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestOrderBaseInboundPtrsByIDs(t *testing.T) {
	inbounds := []*model.Inbound{
		{Id: 1, Tag: "first"},
		{Id: 2, Tag: "second"},
		{Id: 3, Tag: "third"},
	}

	ordered := OrderBaseInboundPtrsByIDs([]uint{2, 1}, inbounds)
	got := []string{ordered[0].Tag, ordered[1].Tag, ordered[2].Tag}
	want := []string{"second", "first", "third"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestOrderMihomoInboundValuesByIDs(t *testing.T) {
	inbounds := []model.MihomoInbound{
		{Id: 4, Tag: "fourth"},
		{Id: 5, Tag: "fifth"},
		{Id: 6, Tag: "sixth"},
	}

	ordered := OrderMihomoInboundValuesByIDs([]uint{6, 4}, inbounds)
	got := []string{ordered[0].Tag, ordered[1].Tag, ordered[2].Tag}
	want := []string{"sixth", "fourth", "fifth"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}
