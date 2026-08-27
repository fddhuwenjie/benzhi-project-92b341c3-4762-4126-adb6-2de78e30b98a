package application

import (
	"fmt"
	"strings"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/store"
)

func (s *Service) RecordMeasurement(batchID string, cmd MeasurementCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		if b.State != domain.StateBaselineFrozen && b.State != domain.StateRemediation {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeState, "当前状态不能追加测量")
		}
		tap := b.Taps[cmd.TapID]
		if tap == nil {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeNotFound, "测孔不存在")
		}
		if b.Calibration == nil || b.Calibration.Reference != cmd.CalibrationRef || !b.Calibration.ValidUntil.After(now) {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeUnqualified, "校准引用不存在、不匹配或已失效")
		}
		if strings.TrimSpace(cmd.RoundID) == "" {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "round_id 不能为空")
		}
		for _, old := range b.Rounds {
			if old.RoundID == cmd.RoundID {
				return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "测量轮次已存在")
			}
		}
		kind := cmd.RoundKind
		if kind == "" {
			kind = domain.RoundInitial
		}
		if kind != domain.RoundInitial && kind != domain.RoundRetest {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "测量轮次类型无效")
		}
		if kind == domain.RoundInitial && cmd.DefectID != "" {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "原始测量不能关联缺陷")
		}
		var defect *domain.DefectCase
		if kind == domain.RoundRetest {
			defect = b.Defects[cmd.DefectID]
			if defect == nil || defect.TapID != cmd.TapID {
				return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "复测必须关联同一测孔的缺陷")
			}
			if defect.Status != domain.DefectTreated {
				return nil, store.EventDraft{}, domain.NewError(domain.CodeState, "复测前必须登记返修")
			}
			if cmd.SourceRoundID != "" && cmd.SourceRoundID != defect.SourceRoundID {
				return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "复测源轮次与缺陷不一致")
			}
			current := defect.CurrentTreatment()
			if current == nil || cmd.TreatmentVersionID == "" || cmd.TreatmentVersionID != current.VersionID {
				return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "复测必须关联缺陷当前处置版本")
			}
			source := b.FindRound(defect.SourceRoundID)
			if source == nil || !source.SupportsQualification() {
				return nil, store.EventDraft{}, domain.NewError(domain.CodeState, "缺陷源轮次已作废或被校准失准隔离")
			}
			if err := b.ValidateRetestRequirements(cmd.DefectID, cmd.TreatmentVersionID, cmd.ReviewRequirementIDs); err != nil {
				return nil, store.EventDraft{}, err
			}
		}
		result, responses, neighborMaximum, worstNeighbor, err := classifyMeasurementInput(b, tap, cmd.SupplyPressurePA, cmd.SteadyPressurePA, cmd.DecaySeconds, cmd.NeighborResponsePA, cmd.NeighborResponses)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		if kind == domain.RoundRetest && len(cmd.ReviewRequirementIDs) > 0 && !result.Passed {
			return nil, store.EventDraft{}, domain.NewFieldError(domain.CodeUnqualified, "review_requirement_ids", "复核要求关联复测未通过确定性判定")
		}
		thresholdVersion := b.FrozenThresholdVersion
		if thresholdVersion == 0 && b.CurrentThresholdRevision() != nil {
			thresholdVersion = b.CurrentThresholdRevision().Version
		}
		round := &domain.MeasurementRound{RoundID: cmd.RoundID, BatchID: b.BatchID, TapID: cmd.TapID, RoundKind: kind, SourceRoundID: cmd.SourceRoundID, DefectID: cmd.DefectID, TreatmentVersionID: cmd.TreatmentVersionID, ReviewRequirementIDs: append([]string(nil), cmd.ReviewRequirementIDs...), CalibrationRef: cmd.CalibrationRef, OperatorID: cmd.ActorID, SupplyPressurePA: cmd.SupplyPressurePA, SteadyPressurePA: cmd.SteadyPressurePA, DecaySeconds: cmd.DecaySeconds, NeighborResponsePA: neighborMaximum, NeighborResponses: responses, WorstNeighborTapID: worstNeighbor, FrozenTopologyDigest: b.FrozenTopologyDigest, RecordedAt: now, Result: result, ThresholdVersion: thresholdVersion}
		b.Rounds = append(b.Rounds, round)
		if kind == domain.RoundInitial {
			tap.LatestMeasurementRoundID = round.RoundID
		}
		b.MeasurementParticipants[cmd.ActorID] = true
		created := []string{}
		if kind == domain.RoundRetest {
			defect.RetestRoundIDs = append(defect.RetestRoundIDs, round.RoundID)
			b.CompleteMappedReviewRequirements(cmd.DefectID, cmd.ActorID, now)
			if !result.Passed {
				tap.QualificationStatus = domain.TapDefective
			}
		} else if result.Passed {
			b.ResolveMissingDefects(cmd.TapID, now)
			tap.QualificationStatus = domain.TapQualified
		} else {
			b.ResolveMissingDefects(cmd.TapID, now)
			tap.QualificationStatus = domain.TapDefective
			b.State = domain.StateRemediation
			for i, typ := range result.DefectTypes {
				id := uniqueID("DEF", now, i)
				b.Defects[id] = newMeasurementDefect(b, tap.TapID, id, typ, round)
				created = append(created, id)
			}
		}
		message := "测量合格"
		if !result.Passed {
			message = fmt.Sprintf("测量发现 %d 项缺陷", len(result.DefectTypes))
		}
		out := response(b, message)
		out.CreatedDefectIDs = created
		related := append([]string{tap.TapID, round.RoundID}, created...)
		return out, eventWithRelated(cmd.ActorID, "measurement.recorded", fmt.Sprintf("测孔 %s 记录 %s 轮次 %s：%s", tap.Label, kind, round.RoundID, message), now, related...), nil
	})
}

func classifyMeasurementInput(b *domain.QualificationBatch, tap *domain.PressureTap, supply, steady, decay, legacy float64, responses []domain.NeighborResponse) (domain.MeasurementResult, []domain.NeighborResponse, float64, string, error) {
	if responses == nil && len(tap.NeighborTapIDs) == 0 {
		result, err := domain.ClassifyMeasurement(b.ThresholdProfile, supply, steady, decay, legacy)
		return result, []domain.NeighborResponse{}, legacy, "", err
	}
	return domain.ClassifyMeasurementResponses(b.ThresholdProfile, tap, supply, steady, decay, responses)
}

func (s *Service) PreflightVoidRound(batchID, roundID string, expectedRevision uint64) ([]string, error) {
	b, err := s.repo.Get(batchID)
	if err != nil {
		return nil, err
	}
	if b.Revision != expectedRevision {
		return nil, domain.NewError(domain.CodeConflict, "修订冲突：期望 %d，当前 %d", expectedRevision, b.Revision)
	}
	return b.RoundVoidDependencies(roundID)
}

func (s *Service) VoidMeasurementRound(batchID string, cmd VoidRoundCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		if err := b.VoidRound(cmd.RoundID, cmd.Reason, cmd.ActorID, now); err != nil {
			return nil, store.EventDraft{}, err
		}
		return response(b, "测量轮次已追加作废记录并完成资格回算"), eventWithRelated(cmd.ActorID, "measurement.voided", "作废测量轮次 "+cmd.RoundID+"："+cmd.Reason, now, cmd.RoundID), nil
	})
}

func severity(t domain.DefectType) string {
	if t == domain.DefectCrosstalk || t == domain.DefectLeak {
		return "high"
	}
	return "medium"
}

func newMeasurementDefect(b *domain.QualificationBatch, tapID, defectID string, typ domain.DefectType, round *domain.MeasurementRound) *domain.DefectCase {
	defect := &domain.DefectCase{DefectID: defectID, BatchID: b.BatchID, TapID: tapID, DefectType: typ, Severity: severity(typ), RuleSnapshot: round.Result.RuleSnapshot, SourceRoundID: round.RoundID, Status: domain.DefectOpen}
	if typ == domain.DefectCrosstalk {
		defect.TriggerNeighborTapID = round.WorstNeighborTapID
		defect.TriggerNeighborRatio = round.Result.NeighborRatio
		defect.FrozenTopologyDigest = round.FrozenTopologyDigest
	}
	return defect
}
