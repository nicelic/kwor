package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestNormalizeClientSpeedLimitMbps(t *testing.T) {
	cases := []struct {
		name  string
		input int
		want  int
	}{
		{name: "negative becomes zero", input: -5, want: 0},
		{name: "zero stays zero", input: 0, want: 0},
		{name: "positive stays positive", input: 200, want: 200},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeClientSpeedLimitMbps(tc.input); got != tc.want {
				t.Fatalf("unexpected normalized speed limit: got=%d want=%d", got, tc.want)
			}
		})
	}
}

func TestClientGetAllIncludesSpeedLimitMbps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "client-speed-limit.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	client := model.Client{
		Enable:                true,
		Name:                  "speed-user",
		Inbounds:              json.RawMessage(`[]`),
		Links:                 json.RawMessage(`[]`),
		SpeedLimitMbps:        200,
		TrafficResetRequested: false,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	clients, err := (&ClientService{}).GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(*clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(*clients))
	}
	if got := (*clients)[0].SpeedLimitMbps; got != 200 {
		t.Fatalf("expected speedLimitMbps=200, got %d", got)
	}
}

func TestMihomoClientGetAllIncludesSpeedLimitMbps(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-client-speed-limit.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	client := model.MihomoClient{
		Enable:         true,
		Name:           "mihomo-speed-user",
		Inbounds:       json.RawMessage(`[]`),
		Links:          json.RawMessage(`[]`),
		SpeedLimitMbps: 300,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	clients, err := (&MihomoClientService{}).GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(*clients) != 1 {
		t.Fatalf("expected 1 mihomo client, got %d", len(*clients))
	}
	if got := (*clients)[0].SpeedLimitMbps; got != 300 {
		t.Fatalf("expected speedLimitMbps=300, got %d", got)
	}
}
