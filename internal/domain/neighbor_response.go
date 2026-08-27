package domain

import (
	"math"
	"sort"
)

func NormalizeNeighborResponses(tap *PressureTap, responses []NeighborResponse) ([]NeighborResponse, float64, string, error) {
	if tap == nil {
		return nil, 0, "", NewError(CodeNotFound, "测孔不存在")
	}
	expected := map[string]bool{}
	for _, id := range tap.NeighborTapIDs {
		expected[id] = true
	}
	seen := map[string]bool{}
	result := make([]NeighborResponse, 0, len(responses))
	maxValue := 0.0
	worst := ""
	for _, item := range responses {
		if !expected[item.TapID] {
			return nil, 0, "", NewError(CodeInvalid, "相邻响应包含非冻结邻接测孔 %s", item.TapID)
		}
		if seen[item.TapID] {
			return nil, 0, "", NewError(CodeInvalid, "相邻响应测孔 %s 重复", item.TapID)
		}
		if math.IsNaN(item.ResponsePA) || math.IsInf(item.ResponsePA, 0) || item.ResponsePA < 0 {
			return nil, 0, "", NewError(CodeInvalid, "相邻测孔 %s 的响应必须是非负有限数", item.TapID)
		}
		seen[item.TapID] = true
		result = append(result, item)
		if worst == "" || item.ResponsePA > maxValue || item.ResponsePA == maxValue && item.TapID < worst {
			maxValue = item.ResponsePA
			worst = item.TapID
		}
	}
	for _, id := range tap.NeighborTapIDs {
		if !seen[id] {
			return nil, 0, "", NewError(CodeInvalid, "缺少冻结邻接测孔 %s 的响应", id)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TapID < result[j].TapID })
	return result, maxValue, worst, nil
}

func ClassifyMeasurementResponses(p ThresholdProfile, tap *PressureTap, supply, steady, decay float64, responses []NeighborResponse) (MeasurementResult, []NeighborResponse, float64, string, error) {
	normalized, maximum, worst, err := NormalizeNeighborResponses(tap, responses)
	if err != nil {
		return MeasurementResult{}, nil, 0, "", err
	}
	result, err := ClassifyMeasurement(p, supply, steady, decay, maximum)
	return result, normalized, maximum, worst, err
}
