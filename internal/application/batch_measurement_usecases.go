package application

import (
	"fmt"
	"strings"
	"time"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/store"
)

type MeasurementPreviewRow struct {
	RoundID            string                    `json:"round_id"`
	TapID              string                    `json:"tap_id"`
	Result             domain.MeasurementResult  `json:"result"`
	NeighborResponses  []domain.NeighborResponse `json:"neighbor_responses"`
	WorstNeighborTapID string                    `json:"worst_neighbor_tap_id,omitempty"`
}

type BatchMeasurementPreview struct {
	BatchID                     string                  `json:"batch_id"`
	Revision                    uint64                  `json:"revision"`
	CalibrationRef              string                  `json:"calibration_ref"`
	Rows                        []MeasurementPreviewRow `json:"rows"`
	Ready                       bool                    `json:"ready"`
	CalibrationRemainingSeconds int64                   `json:"calibration_remaining_seconds"`
	CalibrationWarningLevel     string                  `json:"calibration_warning_level"`
	MayExpireBeforeReview       bool                    `json:"may_expire_before_review"`
}

func (s *Service) PreflightBatchMeasurement(batchID string, cmd BatchMeasurementCommand) (BatchMeasurementPreview, error) {
	if err := validateMeta(cmd.CommandMeta); err != nil {
		return BatchMeasurementPreview{}, err
	}
	b, err := s.repo.Get(batchID)
	if err != nil {
		return BatchMeasurementPreview{}, err
	}
	if b.Revision != cmd.ExpectedRevision {
		return BatchMeasurementPreview{}, domain.NewError(domain.CodeConflict, "修订冲突：期望 %d，当前 %d", cmd.ExpectedRevision, b.Revision)
	}
	rows, err := preflightBatchRows(b, cmd, s.clock())
	if err != nil {
		return BatchMeasurementPreview{}, err
	}
	remaining := b.Calibration.ValidUntil.Sub(s.clock())
	warning := "none"
	mayExpire := remaining <= 24*time.Hour
	if mayExpire {
		warning = "warning"
	}
	return BatchMeasurementPreview{BatchID: batchID, Revision: b.Revision, CalibrationRef: cmd.CalibrationRef, Rows: rows, Ready: true, CalibrationRemainingSeconds: int64(remaining.Seconds()), CalibrationWarningLevel: warning, MayExpireBeforeReview: mayExpire}, nil
}

func preflightBatchRows(b *domain.QualificationBatch, cmd BatchMeasurementCommand, now time.Time) ([]MeasurementPreviewRow, error) {
	if b.State != domain.StateBaselineFrozen && b.State != domain.StateRemediation {
		return nil, domain.NewError(domain.CodeState, "当前状态不能批量追加测量")
	}
	if len(cmd.Rows) == 0 {
		return nil, domain.NewError(domain.CodeInvalid, "批量测量不能为空")
	}
	if b.Calibration == nil || b.Calibration.Reference != cmd.CalibrationRef || !b.Calibration.ValidUntil.After(now) {
		return nil, domain.NewError(domain.CodeUnqualified, "批量测量必须使用当前且有效的校准引用")
	}
	existing := map[string]bool{}
	for _, r := range b.Rounds {
		existing[r.RoundID] = true
	}
	seenRound := map[string]bool{}
	seenTap := map[string]bool{}
	out := make([]MeasurementPreviewRow, 0, len(cmd.Rows))
	for i, row := range cmd.Rows {
		if strings.TrimSpace(row.RoundID) == "" {
			return nil, domain.NewError(domain.CodeInvalid, "第 %d 行 round_id 不能为空", i+1)
		}
		if seenRound[row.RoundID] || existing[row.RoundID] {
			return nil, domain.NewError(domain.CodeConflict, "第 %d 行 round_id %s 重复", i+1, row.RoundID)
		}
		if seenTap[row.TapID] {
			return nil, domain.NewError(domain.CodeInvalid, "第 %d 行 tap_id %s 重复", i+1, row.TapID)
		}
		if b.Taps[row.TapID] == nil {
			return nil, domain.NewError(domain.CodeNotFound, "第 %d 行测孔 %s 不存在", i+1, row.TapID)
		}
		result, normalized, _, worst, err := classifyMeasurementInput(b, b.Taps[row.TapID], row.SupplyPressurePA, row.SteadyPressurePA, row.DecaySeconds, row.NeighborResponsePA, row.NeighborResponses)
		if err != nil {
			return nil, domain.NewError(domain.CodeInvalid, "第 %d 行：%v", i+1, err)
		}
		seenRound[row.RoundID] = true
		seenTap[row.TapID] = true
		out = append(out, MeasurementPreviewRow{RoundID: row.RoundID, TapID: row.TapID, Result: result, NeighborResponses: normalized, WorstNeighborTapID: worst})
	}
	return out, nil
}

func (s *Service) RecordBatchMeasurements(batchID string, cmd BatchMeasurementCommand) (CommandResponse, error) {
	if !cmd.Confirm {
		return CommandResponse{}, domain.NewError(domain.CodeInvalid, "批量提交必须显式设置 confirm=true")
	}
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		previews, err := preflightBatchRows(b, cmd, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		created := []string{}
		defectIndex := 0
		for i, row := range cmd.Rows {
			result := previews[i].Result
			thresholdVersion := b.FrozenThresholdVersion
			if thresholdVersion == 0 && b.CurrentThresholdRevision() != nil {
				thresholdVersion = b.CurrentThresholdRevision().Version
			}
			responses := previews[i].NeighborResponses
			maximum := row.NeighborResponsePA
			if row.NeighborResponses != nil || len(b.Taps[row.TapID].NeighborTapIDs) > 0 {
				maximum = result.NeighborRatio * row.SupplyPressurePA
			}
			round := &domain.MeasurementRound{RoundID: row.RoundID, BatchID: b.BatchID, TapID: row.TapID, RoundKind: domain.RoundInitial, CalibrationRef: cmd.CalibrationRef, OperatorID: cmd.ActorID, SupplyPressurePA: row.SupplyPressurePA, SteadyPressurePA: row.SteadyPressurePA, DecaySeconds: row.DecaySeconds, NeighborResponsePA: maximum, NeighborResponses: responses, WorstNeighborTapID: previews[i].WorstNeighborTapID, FrozenTopologyDigest: b.FrozenTopologyDigest, RecordedAt: now, Result: result, ThresholdVersion: thresholdVersion}
			b.Rounds = append(b.Rounds, round)
			tap := b.Taps[row.TapID]
			tap.LatestMeasurementRoundID = row.RoundID
			b.ResolveMissingDefects(row.TapID, now)
			if result.Passed {
				tap.QualificationStatus = domain.TapQualified
			} else {
				tap.QualificationStatus = domain.TapDefective
				b.State = domain.StateRemediation
				for _, typ := range result.DefectTypes {
					id := uniqueID("DEF", now, defectIndex)
					defectIndex++
					b.Defects[id] = newMeasurementDefect(b, row.TapID, id, typ, round)
					created = append(created, id)
				}
			}
		}
		b.MeasurementParticipants[cmd.ActorID] = true
		out := response(b, fmt.Sprintf("已原子提交 %d 个测孔的测量", len(cmd.Rows)))
		out.CreatedDefectIDs = created
		related := append([]string(nil), created...)
		for _, row := range cmd.Rows {
			related = append(related, row.TapID, row.RoundID)
		}
		return out, eventWithRelated(cmd.ActorID, "measurement.batch_recorded", fmt.Sprintf("批量记录 %d 个测孔，生成 %d 项缺陷", len(cmd.Rows), len(created)), now, related...), nil
	})
}
