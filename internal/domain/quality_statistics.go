package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type MeasurementRiskLevel string

const (
	MeasurementRiskNormal  MeasurementRiskLevel = "normal"
	MeasurementRiskWarning MeasurementRiskLevel = "warning"
	MeasurementRiskHigh    MeasurementRiskLevel = "high"
)

type MeasurementQualityFilter struct {
	TapID       string
	SurfaceZone string
	RoundKind   RoundKind
	Level       MeasurementRiskLevel
}

type StatisticValues struct {
	Count   int     `json:"count"`
	Minimum float64 `json:"minimum"`
	Maximum float64 `json:"maximum"`
	Average float64 `json:"average"`
}

type QualityRiskFlags struct {
	CoverageInsufficient           bool `json:"coverage_insufficient"`
	OpenDefect                     bool `json:"open_defect"`
	ConsecutiveRetestsInsufficient bool `json:"consecutive_retests_insufficient"`
	CalibrationExpiring            bool `json:"calibration_expiring"`
}

type QualityLocation struct {
	MatrixTapID    string   `json:"matrix_tap_id"`
	SurfaceZone    string   `json:"surface_zone"`
	DefectIDs      []string `json:"defect_ids"`
	RequirementIDs []string `json:"requirement_ids"`
}

type MeasurementQualityRow struct {
	SurfaceZone        string               `json:"surface_zone"`
	RoundKind          RoundKind            `json:"round_kind"`
	TapID              string               `json:"tap_id"`
	ValidRoundCount    int                  `json:"valid_round_count"`
	Coverage           float64              `json:"coverage"`
	PassRate           float64              `json:"pass_rate"`
	DefectCounts       map[DefectType]int   `json:"defect_counts"`
	PressureRatio      StatisticValues      `json:"pressure_ratio"`
	DecaySeconds       StatisticValues      `json:"decay_seconds"`
	WorstNeighborRatio StatisticValues      `json:"worst_neighbor_ratio"`
	RoundIDs           []string             `json:"round_ids"`
	RiskLevel          MeasurementRiskLevel `json:"risk_level"`
	Risks              QualityRiskFlags     `json:"risks"`
	Location           QualityLocation      `json:"location"`
}

type MeasurementQualityZone struct {
	SurfaceZone        string             `json:"surface_zone"`
	RoundKind          RoundKind          `json:"round_kind"`
	FrozenTapCount     int                `json:"frozen_tap_count"`
	MeasuredTapCount   int                `json:"measured_tap_count"`
	ValidRoundCount    int                `json:"valid_round_count"`
	Coverage           float64            `json:"coverage"`
	PassRate           float64            `json:"pass_rate"`
	DefectCounts       map[DefectType]int `json:"defect_counts"`
	PressureRatio      StatisticValues    `json:"pressure_ratio"`
	DecaySeconds       StatisticValues    `json:"decay_seconds"`
	WorstNeighborRatio StatisticValues    `json:"worst_neighbor_ratio"`
}

type MeasurementQualitySnapshot struct {
	BatchID          string                   `json:"batch_id"`
	BatchRevision    uint64                   `json:"batch_revision"`
	ThresholdVersion uint64                   `json:"threshold_version"`
	ThresholdDigest  string                   `json:"threshold_digest"`
	RoundIDs         []string                 `json:"round_ids"`
	Rows             []MeasurementQualityRow  `json:"rows"`
	Zones            []MeasurementQualityZone `json:"zones"`
	SummaryDigest    string                   `json:"summary_digest"`
}

func (b *QualificationBatch) MeasurementQualitySnapshot(filter MeasurementQualityFilter, now time.Time) MeasurementQualitySnapshot {
	out := MeasurementQualitySnapshot{BatchID: b.BatchID, BatchRevision: b.Revision, ThresholdVersion: b.FrozenThresholdVersion, ThresholdDigest: b.FrozenThresholdDigest, RoundIDs: []string{}, Rows: []MeasurementQualityRow{}, Zones: []MeasurementQualityZone{}}
	if out.ThresholdVersion == 0 && b.CurrentThresholdRevision() != nil {
		out.ThresholdVersion = b.CurrentThresholdRevision().Version
		out.ThresholdDigest = b.CurrentThresholdRevision().Digest
	}
	validByTapKind := map[string]map[RoundKind][]*MeasurementRound{}
	allKindsByTap := map[string]map[RoundKind]bool{}
	for _, round := range b.Rounds {
		if allKindsByTap[round.TapID] == nil {
			allKindsByTap[round.TapID] = map[RoundKind]bool{}
		}
		allKindsByTap[round.TapID][round.RoundKind] = true
		if !b.roundCountsForQuality(round) {
			continue
		}
		if validByTapKind[round.TapID] == nil {
			validByTapKind[round.TapID] = map[RoundKind][]*MeasurementRound{}
		}
		validByTapKind[round.TapID][round.RoundKind] = append(validByTapKind[round.TapID][round.RoundKind], round)
	}
	for _, tapID := range SortedTapIDs(b.Taps) {
		tap := b.Taps[tapID]
		if filter.TapID != "" && filter.TapID != tapID || filter.SurfaceZone != "" && filter.SurfaceZone != tap.SurfaceZone {
			continue
		}
		kinds := []RoundKind{RoundInitial}
		if filter.RoundKind != "" {
			kinds = []RoundKind{filter.RoundKind}
		} else if allKindsByTap[tapID][RoundRetest] {
			kinds = append(kinds, RoundRetest)
		}
		for _, kind := range kinds {
			row := b.qualityRow(tap, kind, validByTapKind[tapID][kind], now)
			if filter.Level != "" && row.RiskLevel != filter.Level {
				continue
			}
			out.Rows = append(out.Rows, row)
			out.RoundIDs = append(out.RoundIDs, row.RoundIDs...)
		}
	}
	sort.Slice(out.Rows, func(i, j int) bool {
		if out.Rows[i].SurfaceZone != out.Rows[j].SurfaceZone {
			return out.Rows[i].SurfaceZone < out.Rows[j].SurfaceZone
		}
		if out.Rows[i].TapID != out.Rows[j].TapID {
			return out.Rows[i].TapID < out.Rows[j].TapID
		}
		return out.Rows[i].RoundKind < out.Rows[j].RoundKind
	})
	sort.Strings(out.RoundIDs)
	out.RoundIDs = uniqueStrings(out.RoundIDs)
	out.Zones = aggregateQualityZones(b, out.Rows)
	copy := out
	copy.SummaryDigest = ""
	data, _ := json.Marshal(copy)
	sum := sha256.Sum256(data)
	out.SummaryDigest = hex.EncodeToString(sum[:])
	return out
}

func (b *QualificationBatch) roundCountsForQuality(round *MeasurementRound) bool {
	if !round.SupportsQualification() || b.CalibrationIsInvalid(round.CalibrationRef) || b.Taps[round.TapID] == nil {
		return false
	}
	if b.FrozenTopologyDigest != "" && round.FrozenTopologyDigest != b.FrozenTopologyDigest {
		return false
	}
	normalized, maximum, worst, err := NormalizeNeighborResponses(b.Taps[round.TapID], round.NeighborResponses)
	if err != nil || len(normalized) != len(round.NeighborResponses) || maximum != round.NeighborResponsePA || worst != round.WorstNeighborTapID {
		return false
	}
	return round.RoundKind == RoundInitial || round.RoundKind == RoundRetest
}

func (b *QualificationBatch) qualityRow(tap *PressureTap, kind RoundKind, rounds []*MeasurementRound, now time.Time) MeasurementQualityRow {
	sort.Slice(rounds, func(i, j int) bool {
		if rounds[i].RecordedAt.Equal(rounds[j].RecordedAt) {
			return rounds[i].RoundID < rounds[j].RoundID
		}
		return rounds[i].RecordedAt.Before(rounds[j].RecordedAt)
	})
	row := MeasurementQualityRow{SurfaceZone: tap.SurfaceZone, RoundKind: kind, TapID: tap.TapID, DefectCounts: emptyDefectCounts(), RoundIDs: []string{}, Location: QualityLocation{MatrixTapID: tap.TapID, SurfaceZone: tap.SurfaceZone, DefectIDs: []string{}, RequirementIDs: []string{}}}
	pressure, decay, neighbor := []float64{}, []float64{}, []float64{}
	passed := 0
	for _, round := range rounds {
		row.RoundIDs = append(row.RoundIDs, round.RoundID)
		pressure = append(pressure, round.Result.PressureRatio)
		decay = append(decay, round.DecaySeconds)
		neighbor = append(neighbor, round.Result.NeighborRatio)
		if round.Result.Passed {
			passed++
		}
		for _, defectType := range round.Result.DefectTypes {
			row.DefectCounts[defectType]++
		}
	}
	row.ValidRoundCount = len(rounds)
	if len(rounds) > 0 {
		row.Coverage = 1
		row.PassRate = float64(passed) / float64(len(rounds))
	}
	row.PressureRatio = summarizeValues(pressure)
	row.DecaySeconds = summarizeValues(decay)
	row.WorstNeighborRatio = summarizeValues(neighbor)
	for id, defect := range b.Defects {
		if defect.TapID == tap.TapID && defect.Status != DefectClosed && defect.Status != DefectVoided {
			row.Risks.OpenDefect = true
			row.Location.DefectIDs = append(row.Location.DefectIDs, id)
			if defect.Status == DefectTreated && b.ConsecutivePassingRetests(id) < b.ThresholdProfile.RequiredConsecutivePasses {
				row.Risks.ConsecutiveRetestsInsufficient = true
			}
		}
	}
	for _, requirement := range b.SortedOpenReviewRequirements() {
		if requirement.TapID == tap.TapID {
			row.Location.RequirementIDs = append(row.Location.RequirementIDs, requirement.RequirementID)
		}
	}
	row.Risks.CoverageInsufficient = kind == RoundInitial && len(rounds) == 0
	if kind == RoundInitial && len(rounds) == 0 {
		row.DefectCounts[DefectMissing]++
	}
	row.Risks.CalibrationExpiring = b.calibrationExpiring(rounds, now)
	sort.Strings(row.Location.DefectIDs)
	sort.Strings(row.Location.RequirementIDs)
	row.RiskLevel = MeasurementRiskNormal
	if row.Risks.CalibrationExpiring || row.Risks.ConsecutiveRetestsInsufficient {
		row.RiskLevel = MeasurementRiskWarning
	}
	if row.Risks.CoverageInsufficient || row.Risks.OpenDefect {
		row.RiskLevel = MeasurementRiskHigh
	}
	return row
}

func (b *QualificationBatch) calibrationExpiring(rounds []*MeasurementRound, now time.Time) bool {
	refs := map[string]bool{}
	for _, round := range rounds {
		refs[round.CalibrationRef] = true
	}
	if b.Calibration != nil {
		refs[b.Calibration.Reference] = true
	}
	for _, calibration := range b.CalibrationHistory {
		if refs[calibration.Reference] && calibration.ValidUntil.After(now) && calibration.ValidUntil.Sub(now) <= 24*time.Hour {
			return true
		}
	}
	return false
}

func summarizeValues(values []float64) StatisticValues {
	if len(values) == 0 {
		return StatisticValues{}
	}
	out := StatisticValues{Count: len(values), Minimum: values[0], Maximum: values[0]}
	for _, value := range values {
		if value < out.Minimum {
			out.Minimum = value
		}
		if value > out.Maximum {
			out.Maximum = value
		}
		out.Average += value
	}
	out.Average /= float64(len(values))
	return out
}

func emptyDefectCounts() map[DefectType]int {
	return map[DefectType]int{DefectMissing: 0, DefectBlocked: 0, DefectLeak: 0, DefectLag: 0, DefectCrosstalk: 0}
}

func aggregateQualityZones(b *QualificationBatch, rows []MeasurementQualityRow) []MeasurementQualityZone {
	type bucket struct {
		zone                      MeasurementQualityZone
		pressure, decay, neighbor []float64
		passed                    int
	}
	buckets := map[string]*bucket{}
	for _, row := range rows {
		key := row.SurfaceZone + "\x00" + string(row.RoundKind)
		item := buckets[key]
		if item == nil {
			item = &bucket{zone: MeasurementQualityZone{SurfaceZone: row.SurfaceZone, RoundKind: row.RoundKind, DefectCounts: emptyDefectCounts()}}
			for _, tap := range b.Taps {
				if tap.SurfaceZone == row.SurfaceZone {
					item.zone.FrozenTapCount++
				}
			}
			buckets[key] = item
		}
		if row.ValidRoundCount > 0 {
			item.zone.MeasuredTapCount++
		}
		item.zone.ValidRoundCount += row.ValidRoundCount
		item.passed += int(row.PassRate * float64(row.ValidRoundCount))
		for defectType, count := range row.DefectCounts {
			item.zone.DefectCounts[defectType] += count
		}
		for _, id := range row.RoundIDs {
			round := b.FindRound(id)
			if round != nil {
				item.pressure = append(item.pressure, round.Result.PressureRatio)
				item.decay = append(item.decay, round.DecaySeconds)
				item.neighbor = append(item.neighbor, round.Result.NeighborRatio)
			}
		}
	}
	out := make([]MeasurementQualityZone, 0, len(buckets))
	for _, item := range buckets {
		if item.zone.FrozenTapCount > 0 {
			item.zone.Coverage = float64(item.zone.MeasuredTapCount) / float64(item.zone.FrozenTapCount)
		}
		if item.zone.ValidRoundCount > 0 {
			item.zone.PassRate = float64(item.passed) / float64(item.zone.ValidRoundCount)
		}
		item.zone.PressureRatio, item.zone.DecaySeconds, item.zone.WorstNeighborRatio = summarizeValues(item.pressure), summarizeValues(item.decay), summarizeValues(item.neighbor)
		out = append(out, item.zone)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SurfaceZone == out[j].SurfaceZone {
			return out[i].RoundKind < out[j].RoundKind
		}
		return out[i].SurfaceZone < out[j].SurfaceZone
	})
	return out
}

func uniqueStrings(values []string) []string {
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}
