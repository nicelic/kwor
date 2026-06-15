package util

import "github.com/alireza0/s-ui/database/model"

func orderBySelectionIDs[T any](selection []uint, items []T, idOf func(T) uint) []T {
	if len(selection) == 0 || len(items) <= 1 || idOf == nil {
		return items
	}

	indexByID := make(map[uint]int, len(items))
	for index, item := range items {
		id := idOf(item)
		if id == 0 {
			continue
		}
		if _, exists := indexByID[id]; exists {
			continue
		}
		indexByID[id] = index
	}

	ordered := make([]T, 0, len(items))
	used := make(map[int]struct{}, len(items))
	for _, id := range selection {
		index, exists := indexByID[id]
		if !exists {
			continue
		}
		if _, exists := used[index]; exists {
			continue
		}
		ordered = append(ordered, items[index])
		used[index] = struct{}{}
	}

	for index, item := range items {
		if _, exists := used[index]; exists {
			continue
		}
		ordered = append(ordered, item)
	}

	return ordered
}

func OrderBaseInboundPtrsByIDs(selection []uint, inbounds []*model.Inbound) []*model.Inbound {
	return orderBySelectionIDs(selection, inbounds, func(inbound *model.Inbound) uint {
		if inbound == nil {
			return 0
		}
		return inbound.Id
	})
}

func OrderBaseInboundValuesByIDs(selection []uint, inbounds []model.Inbound) []model.Inbound {
	return orderBySelectionIDs(selection, inbounds, func(inbound model.Inbound) uint {
		return inbound.Id
	})
}

func OrderMihomoInboundPtrsByIDs(selection []uint, inbounds []*model.MihomoInbound) []*model.MihomoInbound {
	return orderBySelectionIDs(selection, inbounds, func(inbound *model.MihomoInbound) uint {
		if inbound == nil {
			return 0
		}
		return inbound.Id
	})
}

func OrderMihomoInboundValuesByIDs(selection []uint, inbounds []model.MihomoInbound) []model.MihomoInbound {
	return orderBySelectionIDs(selection, inbounds, func(inbound model.MihomoInbound) uint {
		return inbound.Id
	})
}
