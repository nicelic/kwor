package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSingboxRuntimeOutboundTagsUsesStoredProtocolType(t *testing.T) {
	outbound := &model.Outbound{
		Type: "shadowtls",
		Tag:  "legacy-shadow",
		RawOutbound: json.RawMessage(`{
			"type":"shadowsocks",
			"tag":"legacy-shadow",
			"ss_config":{}
		}`),
	}

	tags, err := singboxRuntimeOutboundTags(outbound)
	if err != nil {
		t.Fatalf("collect runtime tags: %v", err)
	}
	want := []string{"legacy-shadow", "legacy-shadow-out"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("runtime tags = %#v, want %#v", tags, want)
	}
}

func TestSingboxRuntimeOutboundTagsRequiresObjectSSConfig(t *testing.T) {
	outbound := &model.Outbound{
		Type: "shadowtls",
		Tag:  "invalid-shadow",
		RawOutbound: json.RawMessage(`{
			"type":"shadowtls",
			"tag":"invalid-shadow",
			"ss_config":"invalid"
		}`),
	}

	tags, err := singboxRuntimeOutboundTags(outbound)
	if err != nil {
		t.Fatalf("collect runtime tags: %v", err)
	}
	want := []string{"invalid-shadow"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("runtime tags = %#v, want %#v", tags, want)
	}
}

func TestSingboxRuntimeOutboundTagsDoesNotInferMissingStoredProtocolType(t *testing.T) {
	outbound := &model.Outbound{
		Tag: "missing-type",
		RawOutbound: json.RawMessage(`{
			"type":"shadowtls",
			"tag":"missing-type",
			"ss_config":{}
		}`),
	}

	tags, err := singboxRuntimeOutboundTags(outbound)
	if err != nil {
		t.Fatalf("collect runtime tags: %v", err)
	}
	want := []string{"missing-type"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("runtime tags = %#v, want %#v", tags, want)
	}
}

func TestLoadSingboxRouteOutboundTagsUsesRuntimeProjection(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	if err := db.Create(&model.Outbound{
		Type: "shadowtls",
		Tag:  "stored-tag",
		RawOutbound: json.RawMessage(`{
			"type":"shadowsocks",
			"tag":"runtime-tag",
			"ss_config":{}
		}`),
	}).Error; err != nil {
		t.Fatalf("create outbound: %v", err)
	}

	tags, err := loadSingboxRouteOutboundTags(db)
	if err != nil {
		t.Fatalf("load route outbound tags: %v", err)
	}
	want := []string{"direct", "runtime-tag", "runtime-tag-out"}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("route outbound tags = %#v, want %#v", tags, want)
	}
}
