package domain

import (
	"fmt"
	"math"
)

func (p ThresholdProfile) Validate() error {
	if p.MinimumPressureRatio <= 0 || p.MinimumPressureRatio >= 1 {
		return NewError(CodeInvalid, "最低稳态压力比必须在 0 和 1 之间")
	}
	if p.MaximumPressureRatio < 1 || p.MaximumPressureRatio < p.MinimumPressureRatio {
		return NewError(CodeInvalid, "最高稳态压力比无效")
	}
	if p.MaximumDecaySeconds <= 0 {
		return NewError(CodeInvalid, "最大衰减时间必须大于 0")
	}
	if p.MaximumNeighborRatio <= 0 || p.MaximumNeighborRatio >= 1 {
		return NewError(CodeInvalid, "相邻响应比必须在 0 和 1 之间")
	}
	if p.RequiredConsecutivePasses < 1 || p.RequiredConsecutivePasses > 10 {
		return NewError(CodeInvalid, "连续合格次数必须为 1 到 10")
	}
	return nil
}

func ClassifyMeasurement(p ThresholdProfile, supply, steady, decay, neighbor float64) (MeasurementResult, error) {
	if err := p.Validate(); err != nil {
		return MeasurementResult{}, err
	}
	values := []float64{supply, steady, decay, neighbor}
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return MeasurementResult{}, NewError(CodeInvalid, "测量值必须是非负有限数")
		}
	}
	if supply <= 0 {
		return MeasurementResult{}, NewError(CodeInvalid, "供压必须大于 0")
	}
	pressureRatio := steady / supply
	neighborRatio := neighbor / supply
	defects := make([]DefectType, 0, 3)
	if pressureRatio < p.MinimumPressureRatio {
		defects = append(defects, DefectBlocked)
	}
	if pressureRatio > p.MaximumPressureRatio {
		defects = append(defects, DefectLeak)
	}
	if decay > p.MaximumDecaySeconds {
		defects = append(defects, DefectLag)
	}
	if neighborRatio > p.MaximumNeighborRatio {
		defects = append(defects, DefectCrosstalk)
	}
	rule := fmt.Sprintf("pressure_ratio=[%.6f,%.6f];decay<=%.6f;neighbor_ratio<=%.6f", p.MinimumPressureRatio, p.MaximumPressureRatio, p.MaximumDecaySeconds, p.MaximumNeighborRatio)
	return MeasurementResult{Passed: len(defects) == 0, DefectTypes: defects, PressureRatio: pressureRatio, NeighborRatio: neighborRatio, RuleSnapshot: rule}, nil
}
