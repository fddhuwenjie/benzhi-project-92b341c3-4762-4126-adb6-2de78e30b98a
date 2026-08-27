package domain

import (
	"sort"
	"time"
)

type DriftLevel string

const (
	DriftNormal      DriftLevel = "normal"
	DriftApproaching DriftLevel = "approaching_threshold"
	DriftWorsening   DriftLevel = "worsening"
)

type RoundDriftComparison struct {
	TapID               string     `json:"tap_id"`
	SurfaceZone         string     `json:"surface_zone"`
	RoundID             string     `json:"round_id"`
	RoundKind           RoundKind  `json:"round_kind"`
	RecordedAt          time.Time  `json:"recorded_at"`
	PressureRatio       float64    `json:"pressure_ratio"`
	DecaySeconds        float64    `json:"decay_seconds"`
	NeighborRatio       float64    `json:"neighbor_ratio"`
	PressureRatioDelta  *float64   `json:"pressure_ratio_delta,omitempty"`
	DecaySecondsDelta   *float64   `json:"decay_seconds_delta,omitempty"`
	NeighborRatioDelta  *float64   `json:"neighbor_ratio_delta,omitempty"`
	Level               DriftLevel `json:"level"`
	Comparable          bool       `json:"comparable"`
	NotComparableReason string     `json:"not_comparable_reason,omitempty"`
}

func (b *QualificationBatch) RoundDriftComparisons() []RoundDriftComparison {
	byTap := map[string][]*MeasurementRound{}
	for _, round := range b.Rounds {
		if round.SupportsQualification() {
			byTap[round.TapID] = append(byTap[round.TapID], round)
		}
	}
	out := []RoundDriftComparison{}
	for _, tapID := range SortedTapIDs(b.Taps) {
		rounds := byTap[tapID]
		sort.Slice(rounds, func(i, j int) bool {
			if rounds[i].RecordedAt.Equal(rounds[j].RecordedAt) {
				return rounds[i].RoundID < rounds[j].RoundID
			}
			return rounds[i].RecordedAt.Before(rounds[j].RecordedAt)
		})
		for i, round := range rounds {
			item := RoundDriftComparison{TapID: tapID, SurfaceZone: b.Taps[tapID].SurfaceZone, RoundID: round.RoundID, RoundKind: round.RoundKind, RecordedAt: round.RecordedAt, PressureRatio: round.Result.PressureRatio, DecaySeconds: round.DecaySeconds, NeighborRatio: round.Result.NeighborRatio, Level: DriftNormal}
			if len(rounds) < 2 {
				item.NotComparableReason = "有效轮次不足两轮"
			} else if i == 0 {
				item.NotComparableReason = "首个有效轮次没有上一轮"
			} else {
				previous := rounds[i-1]
				pressureDelta := round.Result.PressureRatio - previous.Result.PressureRatio
				decayDelta := round.DecaySeconds - previous.DecaySeconds
				neighborDelta := round.Result.NeighborRatio - previous.Result.NeighborRatio
				item.PressureRatioDelta, item.DecaySecondsDelta, item.NeighborRatioDelta = &pressureDelta, &decayDelta, &neighborDelta
				item.Comparable = true
				if pressureMarginShrank(b.ThresholdProfile, previous.Result.PressureRatio, round.Result.PressureRatio) || decayDelta > 0 || neighborDelta > 0 {
					item.Level = DriftWorsening
				} else if approachingThreshold(b.ThresholdProfile, round) {
					item.Level = DriftApproaching
				}
			}
			out = append(out, item)
		}
	}
	return out
}

func pressureMarginShrank(p ThresholdProfile, previous, current float64) bool {
	previousMargin := minFloat(previous-p.MinimumPressureRatio, p.MaximumPressureRatio-previous)
	currentMargin := minFloat(current-p.MinimumPressureRatio, p.MaximumPressureRatio-current)
	return currentMargin < previousMargin
}

func approachingThreshold(p ThresholdProfile, round *MeasurementRound) bool {
	pressureRange := p.MaximumPressureRatio - p.MinimumPressureRatio
	pressureMargin := minFloat(round.Result.PressureRatio-p.MinimumPressureRatio, p.MaximumPressureRatio-round.Result.PressureRatio)
	return pressureMargin <= pressureRange*.1 || p.MaximumDecaySeconds-round.DecaySeconds <= p.MaximumDecaySeconds*.1 || p.MaximumNeighborRatio-round.Result.NeighborRatio <= p.MaximumNeighborRatio*.1
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
