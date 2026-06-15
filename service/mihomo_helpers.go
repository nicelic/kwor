package service

import (
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func fillMihomoOutJson(inbound *model.MihomoInbound, hostname string) error {
	if inbound == nil {
		return nil
	}
	base := inbound.ToBase()
	if err := util.FillOutJson(&base, hostname); err != nil {
		return err
	}
	inbound.OutJson = base.OutJson
	return nil
}

func mihomoInboundSliceToBase(inbounds []model.MihomoInbound) []model.Inbound {
	result := make([]model.Inbound, 0, len(inbounds))
	for _, inbound := range inbounds {
		result = append(result, inbound.ToBase())
	}
	return result
}

func mihomoInboundPtrsToBase(inbounds []*model.MihomoInbound) []*model.Inbound {
	result := make([]*model.Inbound, 0, len(inbounds))
	for _, inbound := range inbounds {
		if inbound == nil {
			continue
		}
		base := inbound.ToBase()
		result = append(result, &base)
	}
	return result
}
