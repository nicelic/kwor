package service

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestSingboxSharedEditorsRejectMissingEditsAndIgnoreCopiedCreateIDs(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "singbox-shared-editor-identities.db")

	group := &model.SubGroup{Name: "existing-sub-group", Outbounds: "[]", SortOrder: 1}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup: %v", err)
	}
	subOutbound := &model.SubOutbound{Type: "direct", Tag: "existing-sub-node", RawOutbound: json.RawMessage(`{"type":"direct","tag":"existing-sub-node"}`)}
	if err := db.Create(subOutbound).Error; err != nil {
		t.Fatalf("create suboutbound: %v", err)
	}
	outboundGroup := &model.OutboundGroup{Name: "existing-outbound-group", Outbounds: "[]", SortOrder: 1}
	if err := db.Create(outboundGroup).Error; err != nil {
		t.Fatalf("create outbound group: %v", err)
	}

	missingEditCases := []struct {
		name string
		save func() error
	}{
		{
			name: "subgroup",
			save: func() error {
				return (&SubGroupService{}).Save(db, "edit", json.RawMessage(`{"id":99901,"name":"missing-sub-group","outbounds":"[]"}`))
			},
		},
		{
			name: "suboutbound",
			save: func() error {
				return (&SubOutboundService{}).Save(db, "edit", json.RawMessage(`{"id":99902,"type":"direct","tag":"missing-sub-node"}`))
			},
		},
		{
			name: "outbound group",
			save: func() error {
				return (&OutboundGroupService{}).Save(db, "edit", json.RawMessage(`{"id":99903,"name":"missing-outbound-group","outbounds":"[]"}`))
			},
		},
	}
	for _, test := range missingEditCases {
		t.Run("missing edit/"+test.name, func(t *testing.T) {
			if err := test.save(); err == nil {
				t.Fatal("missing edit unexpectedly succeeded")
			}
		})
	}

	if err := (&SubGroupService{}).Save(db, "new", json.RawMessage(`{"id":`+jsonUint(group.Id)+`,"name":"copied-sub-group","outbounds":"[]"}`)); err != nil {
		t.Fatalf("create copied subgroup payload: %v", err)
	}
	createdGroup := &model.SubGroup{}
	if err := db.Where("name = ?", "copied-sub-group").First(createdGroup).Error; err != nil {
		t.Fatalf("load copied subgroup: %v", err)
	}
	if createdGroup.Id == group.Id {
		t.Fatalf("copied subgroup reused existing id %d", group.Id)
	}

	if err := (&SubOutboundService{}).Save(db, "new", json.RawMessage(`{"id":`+jsonUint(subOutbound.Id)+`,"type":"direct","tag":"copied-sub-node"}`)); err != nil {
		t.Fatalf("create copied suboutbound payload: %v", err)
	}
	createdSubOutbound := &model.SubOutbound{}
	if err := db.Where("tag = ?", "copied-sub-node").First(createdSubOutbound).Error; err != nil {
		t.Fatalf("load copied suboutbound: %v", err)
	}
	if createdSubOutbound.Id == subOutbound.Id {
		t.Fatalf("copied suboutbound reused existing id %d", subOutbound.Id)
	}

	if _, err := (&OutboundGroupService{}).SaveWithRuntimeImpact(db, "new", json.RawMessage(`{"id":`+jsonUint(outboundGroup.Id)+`,"name":"copied-outbound-group","outbounds":"[]"}`)); err != nil {
		t.Fatalf("create copied outbound group payload: %v", err)
	}
	createdOutboundGroup := &model.OutboundGroup{}
	if err := db.Where("name = ?", "copied-outbound-group").First(createdOutboundGroup).Error; err != nil {
		t.Fatalf("load copied outbound group: %v", err)
	}
	if createdOutboundGroup.Id == outboundGroup.Id {
		t.Fatalf("copied outbound group reused existing id %d", outboundGroup.Id)
	}

	var unchanged model.SubGroup
	if err := db.Where("id = ?", group.Id).First(&unchanged).Error; err != nil {
		t.Fatalf("reload original subgroup: %v", err)
	}
	if unchanged.Name != "existing-sub-group" {
		t.Fatalf("original subgroup was overwritten: %q", unchanged.Name)
	}
}

func TestMihomoSharedEditorsRejectMissingEditsAndIgnoreCopiedCreateIDs(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "mihomo-shared-editor-identities.db")

	existingInbound := &model.MihomoInbound{
		Type:    "socks",
		Tag:     "existing-mihomo-inbound",
		Options: json.RawMessage(`{"listen":"127.0.0.1","listen_port":24001}`),
	}
	if err := db.Create(existingInbound).Error; err != nil {
		t.Fatalf("create mihomo inbound: %v", err)
	}
	existingOutbound := &model.MihomoOutbound{
		Type: "direct",
		Tag:  "existing-mihomo-outbound",
	}
	if err := db.Create(existingOutbound).Error; err != nil {
		t.Fatalf("create mihomo outbound: %v", err)
	}
	existingTLS := &model.MihomoTls{
		Name:   "existing-mihomo-tls",
		Mode:   model.MihomoTlsModeTLS,
		Server: json.RawMessage(`{}`),
		Client: json.RawMessage(`{}`),
	}
	if err := db.Create(existingTLS).Error; err != nil {
		t.Fatalf("create mihomo TLS: %v", err)
	}
	existingGroup := &model.MihomoOutboundGroup{
		Name:      "existing-mihomo-group",
		Outbounds: "[]",
		SortOrder: 1,
	}
	if err := db.Create(existingGroup).Error; err != nil {
		t.Fatalf("create mihomo outbound group: %v", err)
	}

	missingEditCases := []struct {
		name string
		save func() error
	}{
		{
			name: "inbound",
			save: func() error {
				_, err := (&MihomoInboundService{}).Save(db, "edit", json.RawMessage(`{"type":"socks","tag":"missing-mihomo-inbound","listen_port":24002}`), "", "")
				return err
			},
		},
		{
			name: "outbound",
			save: func() error {
				return (&MihomoOutboundService{}).Save(db, "edit", json.RawMessage(`{"type":"direct","tag":"missing-mihomo-outbound"}`))
			},
		},
		{
			name: "TLS",
			save: func() error {
				return (&MihomoTlsService{}).Save(db, "edit", json.RawMessage(`{"name":"missing-mihomo-tls","mode":"tls","server":{},"client":{}}`), "")
			},
		},
		{
			name: "outbound group",
			save: func() error {
				_, err := (&MihomoOutboundGroupService{}).SaveWithRuntimeImpact(db, "edit", json.RawMessage(`{"name":"missing-mihomo-group","outbounds":"[]"}`))
				return err
			},
		},
	}
	for _, test := range missingEditCases {
		t.Run("missing edit/"+test.name, func(t *testing.T) {
			if err := test.save(); err == nil {
				t.Fatal("missing edit unexpectedly succeeded")
			}
		})
	}

	if _, err := (&MihomoInboundService{}).Save(db, "new", json.RawMessage(`{"id":`+jsonUint(existingInbound.Id)+`,"type":"socks","tag":"copied-mihomo-inbound","listen":"127.0.0.1","listen_port":24003}`), "", ""); err != nil {
		t.Fatalf("create copied mihomo inbound: %v", err)
	}
	if err := (&MihomoOutboundService{}).Save(db, "new", json.RawMessage(`{"id":`+jsonUint(existingOutbound.Id)+`,"type":"direct","tag":"copied-mihomo-outbound"}`)); err != nil {
		t.Fatalf("create copied mihomo outbound: %v", err)
	}
	if err := (&MihomoTlsService{}).Save(db, "new", json.RawMessage(`{"id":`+jsonUint(existingTLS.Id)+`,"name":"copied-mihomo-tls","mode":"tls","server":{},"client":{}}`), ""); err != nil {
		t.Fatalf("create copied mihomo TLS: %v", err)
	}
	if _, err := (&MihomoOutboundGroupService{}).SaveWithRuntimeImpact(db, "new", json.RawMessage(`{"id":`+jsonUint(existingGroup.Id)+`,"name":"copied-mihomo-group","outbounds":"[]"}`)); err != nil {
		t.Fatalf("create copied mihomo outbound group: %v", err)
	}

	var copiedInbound model.MihomoInbound
	if err := db.Where("tag = ?", "copied-mihomo-inbound").First(&copiedInbound).Error; err != nil {
		t.Fatalf("load copied mihomo inbound: %v", err)
	}
	if copiedInbound.Id == existingInbound.Id {
		t.Fatalf("copied mihomo inbound reused existing id %d", existingInbound.Id)
	}
	var copiedOutbound model.MihomoOutbound
	if err := db.Where("tag = ?", "copied-mihomo-outbound").First(&copiedOutbound).Error; err != nil {
		t.Fatalf("load copied mihomo outbound: %v", err)
	}
	if copiedOutbound.Id == existingOutbound.Id {
		t.Fatalf("copied mihomo outbound reused existing id %d", existingOutbound.Id)
	}
	var copiedTLS model.MihomoTls
	if err := db.Where("name = ?", "copied-mihomo-tls").First(&copiedTLS).Error; err != nil {
		t.Fatalf("load copied mihomo TLS: %v", err)
	}
	if copiedTLS.Id == existingTLS.Id {
		t.Fatalf("copied mihomo TLS reused existing id %d", existingTLS.Id)
	}
	var copiedGroup model.MihomoOutboundGroup
	if err := db.Where("name = ?", "copied-mihomo-group").First(&copiedGroup).Error; err != nil {
		t.Fatalf("load copied mihomo outbound group: %v", err)
	}
	if copiedGroup.Id == existingGroup.Id {
		t.Fatalf("copied mihomo outbound group reused existing id %d", existingGroup.Id)
	}
}

func jsonUint(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
