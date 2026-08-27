package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func (r *MeasurementRound) SupportsQualification() bool {
	return r != nil && r.Voided == nil && !r.CalibrationInvalid
}

func (b *QualificationBatch) FindRound(roundID string) *MeasurementRound {
	for _, round := range b.Rounds {
		if round.RoundID == roundID {
			return round
		}
	}
	return nil
}

func (b *QualificationBatch) CalibrationIsInvalid(reference string) bool {
	for _, invalidation := range b.CalibrationInvalidations {
		if invalidation.CalibrationRef == reference {
			return true
		}
	}
	return false
}

func (b *QualificationBatch) InvalidateCalibration(reference, reason, evidence, actor string, discoveredAt time.Time) ([]string, error) {
	if b.State == StateDraft || b.State == StateApproved {
		return nil, NewError(CodeState, "只有冻结后且批准前可登记校准失准")
	}
	if strings.TrimSpace(reason) == "" || strings.TrimSpace(actor) == "" || discoveredAt.IsZero() {
		return nil, NewError(CodeInvalid, "发现时间、失准原因和操作人不能为空")
	}
	if !ValidSHA256(evidence) {
		return nil, NewError(CodeInvalid, "证据摘要必须为 64 位 SHA-256 十六进制值")
	}
	found := false
	for _, calibration := range b.CalibrationHistory {
		if calibration.Reference == reference {
			found = true
			break
		}
	}
	if !found {
		return nil, NewError(CodeNotFound, "校准引用 %s 不存在", reference)
	}
	if b.CalibrationIsInvalid(reference) {
		return nil, NewError(CodeConflict, "校准引用 %s 已登记失准", reference)
	}
	b.CalibrationInvalidations = append(b.CalibrationInvalidations, CalibrationInvalidation{CalibrationRef: reference, DiscoveredAt: discoveredAt.UTC(), Reason: strings.TrimSpace(reason), EvidenceDigest: strings.ToLower(evidence), ActorID: actor})
	affected := make([]string, 0)
	for _, round := range b.Rounds {
		if round.CalibrationRef == reference {
			round.CalibrationInvalid = true
			affected = append(affected, round.RoundID)
		}
	}
	sort.Strings(affected)
	if b.Calibration != nil && b.Calibration.Reference == reference {
		b.Calibration = nil
	}
	if b.State == StateUnderReview {
		b.ArchiveReview("校准失准导致送审快照归档：" + strings.TrimSpace(reason))
		b.State = StateRemediation
		b.ReviewSnapshot = nil
		b.ReviewChecklist = nil
		b.SubmittedAt = nil
	}
	b.RecalculateQualification()
	return affected, nil
}

func (b *QualificationBatch) RoundVoidDependencies(roundID string) ([]string, error) {
	round := b.FindRound(roundID)
	if round == nil {
		return nil, NewError(CodeNotFound, "测量轮次不存在")
	}
	if round.Voided != nil {
		return nil, NewError(CodeConflict, "测量轮次已经作废")
	}
	dependencies := []string{}
	if round.RoundKind == RoundInitial {
		for _, defect := range b.Defects {
			if defect.SourceRoundID != roundID {
				continue
			}
			if len(defect.TreatmentVersions) > 0 {
				dependencies = append(dependencies, "缺陷 "+defect.DefectID+" 已有处置版本")
			}
			if len(defect.RetestRoundIDs) > 0 {
				dependencies = append(dependencies, "缺陷 "+defect.DefectID+" 已有关联复测")
			}
		}
	}
	sort.Strings(dependencies)
	return dependencies, nil
}

func (b *QualificationBatch) VoidRound(roundID, reason, actor string, now time.Time) error {
	if b.State == StateApproved {
		return NewError(CodeState, "批准终态禁止作废轮次")
	}
	if strings.TrimSpace(reason) == "" || strings.TrimSpace(actor) == "" {
		return NewError(CodeInvalid, "作废原因和操作人不能为空")
	}
	dependencies, err := b.RoundVoidDependencies(roundID)
	if err != nil {
		return err
	}
	if len(dependencies) > 0 {
		return NewError(CodeConflict, "轮次存在依赖，不能作废：%s", strings.Join(dependencies, "；"))
	}
	round := b.FindRound(roundID)
	round.Voided = &RoundCorrection{Reason: strings.TrimSpace(reason), ActorID: actor, At: now.UTC()}
	if round.RoundKind == RoundInitial {
		for _, defect := range b.Defects {
			if defect.SourceRoundID == roundID {
				defect.Status = DefectVoided
				defect.ClosedAt = nil
			}
		}
	}
	if b.State == StateUnderReview {
		b.ArchiveReview("测量轮次作废导致送审快照归档：" + roundID)
		b.State = StateRemediation
		b.ReviewSnapshot = nil
		b.ReviewChecklist = nil
		b.SubmittedAt = nil
	}
	b.RecalculateQualification()
	return nil
}

func (b *QualificationBatch) RecalculateQualification() {
	for _, defect := range b.Defects {
		if defect.Status == DefectVoided {
			continue
		}
		source := b.FindRound(defect.SourceRoundID)
		if source != nil && !source.SupportsQualification() {
			defect.Status = DefectVoided
			defect.ClosedAt = nil
			continue
		}
		if defect.Status == DefectClosed && b.ConsecutivePassingRetests(defect.DefectID) < b.ThresholdProfile.RequiredConsecutivePasses {
			defect.Status = DefectTreated
			defect.ClosedAt = nil
		}
	}
	for _, tapID := range SortedTapIDs(b.Taps) {
		tap := b.Taps[tapID]
		var latest *MeasurementRound
		for _, round := range b.Rounds {
			if round.TapID == tapID && round.RoundKind == RoundInitial && round.SupportsQualification() {
				latest = round
			}
		}
		if latest == nil {
			tap.LatestMeasurementRoundID = ""
			tap.QualificationStatus = TapPending
			continue
		}
		tap.LatestMeasurementRoundID = latest.RoundID
		status := TapQualified
		for _, defect := range b.Defects {
			if defect.TapID == tapID && defect.Status != DefectClosed && defect.Status != DefectVoided {
				status = TapDefective
			}
		}
		tap.QualificationStatus = status
	}
}

func (b *QualificationBatch) EffectiveRoundIDs() []string {
	selected := map[string]bool{}
	for _, tap := range b.Taps {
		if tap.LatestMeasurementRoundID != "" {
			selected[tap.LatestMeasurementRoundID] = true
		}
	}
	for _, defect := range b.Defects {
		if defect.Status != DefectClosed || b.Taps[defect.TapID] == nil || b.Taps[defect.TapID].LatestMeasurementRoundID != defect.SourceRoundID {
			continue
		}
		remaining := b.ThresholdProfile.RequiredConsecutivePasses
		for i := len(defect.RetestRoundIDs) - 1; i >= 0 && remaining > 0; i-- {
			round := b.FindRound(defect.RetestRoundIDs[i])
			if round == nil || !round.SupportsQualification() || !round.Result.Passed {
				break
			}
			selected[round.RoundID] = true
			remaining--
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (b *QualificationBatch) QualificationBlockers(now time.Time) []string {
	blockers := []string{}
	if b.Coverage() != 1 {
		blockers = append(blockers, fmt.Sprintf("测孔覆盖率为 %.0f%%，必须达到 100%%", b.Coverage()*100))
	}
	if b.Calibration == nil || b.CalibrationIsInvalid(func() string {
		if b.Calibration == nil {
			return ""
		}
		return b.Calibration.Reference
	}()) || !b.Calibration.ValidUntil.After(now) {
		blockers = append(blockers, "当前有效校准不存在或已经失效")
	}
	calibrations := map[string]Calibration{}
	for _, calibration := range b.CalibrationHistory {
		calibrations[calibration.Reference] = calibration
	}
	for _, roundID := range b.EffectiveRoundIDs() {
		round := b.FindRound(roundID)
		calibration, ok := calibrations[round.CalibrationRef]
		if !ok || b.CalibrationIsInvalid(round.CalibrationRef) || !calibration.ValidUntil.After(now) {
			blockers = append(blockers, "有效轮次 "+round.RoundID+" 的校准 "+round.CalibrationRef+" 无效")
		}
	}
	for _, tapID := range SortedTapIDs(b.Taps) {
		if b.Taps[tapID].QualificationStatus != TapQualified {
			blockers = append(blockers, "测孔 "+tapID+" 需要补测或尚未合格")
		}
	}
	for _, defect := range b.Defects {
		if defect.Status != DefectClosed && defect.Status != DefectVoided {
			blockers = append(blockers, "缺陷 "+defect.DefectID+" 尚未关闭")
		}
	}
	for _, requirement := range b.SortedOpenReviewRequirements() {
		blockers = append(blockers, "复核要求 "+requirement.RequirementID+" 尚未完成")
	}
	return blockers
}
