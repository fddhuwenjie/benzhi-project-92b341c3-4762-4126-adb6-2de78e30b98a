package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (b *QualificationBatch) SortedOpenReviewRequirements() []*ReviewRequirement {
	out := []*ReviewRequirement{}
	for i := range b.ReviewRequirements {
		if b.ReviewRequirements[i].Status == ReviewRequirementOpen {
			out = append(out, &b.ReviewRequirements[i])
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TapID == out[j].TapID {
			return out[i].RequirementID < out[j].RequirementID
		}
		return out[i].TapID < out[j].TapID
	})
	return out
}

func (b *QualificationBatch) ReviewRequirementDigest() string {
	type fact struct {
		ID                 string                  `json:"id"`
		Status             ReviewRequirementStatus `json:"status"`
		TapID              string                  `json:"tap_id"`
		DefectIDs          []string                `json:"defect_ids"`
		TreatmentVersionID string                  `json:"treatment_version_id"`
		EvidenceDigest     string                  `json:"evidence_digest"`
		RoundIDs           []string                `json:"round_ids"`
	}
	facts := make([]fact, 0, len(b.ReviewRequirements))
	for _, requirement := range b.ReviewRequirements {
		defects := append([]string(nil), requirement.RelatedDefectIDs...)
		rounds := append([]string(nil), requirement.CompletedRoundIDs...)
		sort.Strings(defects)
		sort.Strings(rounds)
		facts = append(facts, fact{ID: requirement.RequirementID, Status: requirement.Status, TapID: requirement.TapID, DefectIDs: defects, TreatmentVersionID: requirement.TreatmentVersionID, EvidenceDigest: requirement.EvidenceDigest, RoundIDs: rounds})
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	data, _ := json.Marshal(facts)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func reviewRequirementID(checklistID, itemID, tapID string) string {
	sum := sha256.Sum256([]byte(checklistID + "\x00" + itemID + "\x00" + tapID))
	return "REQ-" + hex.EncodeToString(sum[:8])
}

func (b *QualificationBatch) CreateReviewRequirements(reviewer string, now time.Time) ([]string, error) {
	if b.State != StateUnderReview || b.ReviewChecklist == nil || b.ReviewSnapshot == nil {
		return nil, NewError(CodeState, "批次没有可转为返修要求的复核结果")
	}
	titles := map[string]string{}
	for _, item := range b.ReviewChecklist.Items {
		titles[item.ItemID] = item.Title
	}
	created := []string{}
	index := 0
	for _, result := range b.ReviewChecklist.Results {
		if result.Status != ReviewItemReturned {
			continue
		}
		text := strings.TrimSpace(result.Comment)
		if text == "" {
			return nil, NewFieldError(CodeInvalid, "items.comment", "退回复核要求文本不能为空")
		}
		for _, tapID := range result.ReturnTapIDs {
			if b.Taps[tapID] == nil {
				return nil, NewFieldError(CodeInvalid, "items.return_tap_ids", fmt.Sprintf("测孔 %s 不在送审快照中", tapID))
			}
			requirementID := reviewRequirementID(b.ReviewChecklist.ChecklistID, result.ItemID, tapID)
			if b.FindReviewRequirement(requirementID) != nil {
				return nil, NewError(CodeConflict, "复核要求 %s 已存在", requirementID)
			}
			defectID := "REVIEW-" + strings.TrimPrefix(requirementID, "REQ-")
			if b.Defects[defectID] != nil {
				return nil, NewError(CodeConflict, "复核要求关联缺陷 %s 已存在", defectID)
			}
			tap := b.Taps[tapID]
			if tap.LatestMeasurementRoundID == "" {
				return nil, NewError(CodeState, "测孔 %s 没有可供复核的源轮次", tapID)
			}
			treatmentID := fmt.Sprintf("REVIEW-TREAT-%d-%d", now.UnixNano(), index)
			b.Defects[defectID] = &DefectCase{
				DefectID: defectID, BatchID: b.BatchID, TapID: tapID, DefectType: DefectMissing,
				Severity: "medium", RuleSnapshot: "independent-review-requirement/v2",
				SourceRoundID:     tap.LatestMeasurementRoundID,
				Cause:             result.ItemID + "：" + text,
				CorrectiveAction:  "按独立复核要求补充处置证据和连续合格复测",
				EvidenceDigest:    b.ReviewSnapshot.Digest,
				RetestRoundIDs:    []string{},
				TreatmentVersions: []TreatmentVersion{{VersionID: treatmentID, Sequence: 1, Cause: result.ItemID + "：" + text, CorrectiveAction: "按独立复核要求补充处置证据和连续合格复测", EvidenceDigest: b.ReviewSnapshot.Digest, TechnicianID: "independent-review", RecordedAt: now.UTC(), SourceRoundID: tap.LatestMeasurementRoundID}},
				Status:            DefectTreated,
			}
			b.ReviewRequirements = append(b.ReviewRequirements, ReviewRequirement{
				RequirementID: requirementID, ChecklistID: b.ReviewChecklist.ChecklistID,
				SnapshotDigest: b.ReviewSnapshot.Digest, ReviewItemID: result.ItemID,
				SourceChecklistItem: titles[result.ItemID], TapID: tapID, RequirementText: text,
				RelatedDefectIDs: []string{defectID}, Status: ReviewRequirementOpen,
				CreatedBy: reviewer, CreatedAt: now.UTC(), CompletedRoundIDs: []string{},
			})
			tap.QualificationStatus = TapDefective
			created = append(created, requirementID)
			index++
		}
	}
	sort.Strings(created)
	return created, nil
}

func (b *QualificationBatch) FindReviewRequirement(id string) *ReviewRequirement {
	for i := range b.ReviewRequirements {
		if b.ReviewRequirements[i].RequirementID == id {
			return &b.ReviewRequirements[i]
		}
	}
	return nil
}

func (b *QualificationBatch) OpenReviewRequirementsForDefect(defectID string) []*ReviewRequirement {
	out := []*ReviewRequirement{}
	for i := range b.ReviewRequirements {
		requirement := &b.ReviewRequirements[i]
		if requirement.Status == ReviewRequirementOpen && containsStringValue(requirement.RelatedDefectIDs, defectID) {
			out = append(out, requirement)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequirementID < out[j].RequirementID })
	return out
}

func (b *QualificationBatch) LinkTreatmentRequirements(defectID, treatmentVersionID, evidence string, ids []string) error {
	required, err := b.validateTreatmentRequirements(defectID, ids)
	if err != nil {
		return err
	}
	for _, requirement := range required {
		requirement.TreatmentVersionID = treatmentVersionID
		requirement.EvidenceDigest = strings.ToLower(evidence)
	}
	return nil
}

func (b *QualificationBatch) ValidateTreatmentRequirements(defectID string, ids []string) error {
	_, err := b.validateTreatmentRequirements(defectID, ids)
	return err
}

func (b *QualificationBatch) validateTreatmentRequirements(defectID string, ids []string) ([]*ReviewRequirement, error) {
	required := b.OpenReviewRequirementsForDefect(defectID)
	if len(required) == 0 {
		if len(ids) > 0 {
			return nil, NewFieldError(CodeInvalid, "review_requirement_ids", "处置引用的复核要求与缺陷不匹配")
		}
		return required, nil
	}
	if len(ids) == 0 {
		return nil, NewFieldError(CodeUnqualified, "review_requirement_ids", "复核退回缺陷处置必须映射当前复核要求")
	}
	provided := map[string]bool{}
	for _, id := range ids {
		if provided[id] {
			return nil, NewFieldError(CodeInvalid, "review_requirement_ids", "复核要求引用重复")
		}
		provided[id] = true
		requirement := b.FindReviewRequirement(id)
		if requirement == nil || !containsStringValue(requirement.RelatedDefectIDs, defectID) {
			return nil, NewFieldError(CodeInvalid, "review_requirement_ids", fmt.Sprintf("复核要求 %s 与缺陷不匹配", id))
		}
		if requirement.Status != ReviewRequirementOpen {
			return nil, NewFieldError(CodeConflict, "review_requirement_ids", fmt.Sprintf("复核要求 %s 已完成或已经过期", id))
		}
	}
	for _, requirement := range required {
		if !provided[requirement.RequirementID] {
			return nil, NewFieldError(CodeUnqualified, "review_requirement_ids", fmt.Sprintf("缺少复核要求映射 %s", requirement.RequirementID))
		}
	}
	return required, nil
}

func (b *QualificationBatch) ValidateRetestRequirements(defectID, treatmentVersionID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	required := b.OpenReviewRequirementsForDefect(defectID)
	provided := map[string]bool{}
	for _, id := range ids {
		if provided[id] {
			return NewFieldError(CodeInvalid, "review_requirement_ids", "复核要求引用重复")
		}
		provided[id] = true
		requirement := b.FindReviewRequirement(id)
		if requirement == nil || !containsStringValue(requirement.RelatedDefectIDs, defectID) {
			return NewFieldError(CodeInvalid, "review_requirement_ids", fmt.Sprintf("复核要求 %s 与复测缺陷不匹配", id))
		}
		if requirement.Status != ReviewRequirementOpen {
			return NewFieldError(CodeConflict, "review_requirement_ids", fmt.Sprintf("复核要求 %s 已完成或已经过期", id))
		}
		if requirement.TreatmentVersionID == "" || requirement.TreatmentVersionID != treatmentVersionID || !ValidSHA256(requirement.EvidenceDigest) {
			return NewFieldError(CodeUnqualified, "review_requirement_ids", fmt.Sprintf("复核要求 %s 缺少有效处置证据映射", id))
		}
	}
	for _, requirement := range required {
		if !provided[requirement.RequirementID] {
			return NewFieldError(CodeUnqualified, "review_requirement_ids", fmt.Sprintf("关联复测缺少复核要求 %s", requirement.RequirementID))
		}
	}
	return nil
}

func (b *QualificationBatch) CompleteMappedReviewRequirements(defectID, actor string, now time.Time) {
	defect := b.Defects[defectID]
	if defect == nil || b.ConsecutivePassingRetests(defectID) < b.ThresholdProfile.RequiredConsecutivePasses {
		return
	}
	for _, requirement := range b.OpenReviewRequirementsForDefect(defectID) {
		if requirement.TreatmentVersionID == "" || !ValidSHA256(requirement.EvidenceDigest) {
			continue
		}
		rounds := latestPassingRequirementRetests(b, defect, requirement, b.ThresholdProfile.RequiredConsecutivePasses)
		if len(rounds) != b.ThresholdProfile.RequiredConsecutivePasses {
			continue
		}
		requirement.Status = ReviewRequirementCompleted
		requirement.CompletedBy = actor
		completed := now.UTC()
		requirement.CompletedAt = &completed
		requirement.CompletedRoundIDs = rounds
		b.recordRequirementCompletion(requirement)
	}
}

func latestPassingRequirementRetests(b *QualificationBatch, defect *DefectCase, requirement *ReviewRequirement, required int) []string {
	if required <= 0 || len(defect.RetestRoundIDs) < required {
		return nil
	}
	ids := make([]string, 0, required)
	for i := len(defect.RetestRoundIDs) - 1; i >= 0 && len(ids) < required; i-- {
		round := b.FindRound(defect.RetestRoundIDs[i])
		if round == nil || !round.SupportsQualification() || !round.Result.Passed || round.TreatmentVersionID != requirement.TreatmentVersionID || !containsStringValue(round.ReviewRequirementIDs, requirement.RequirementID) {
			return nil
		}
		ids = append(ids, round.RoundID)
	}
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

func (b *QualificationBatch) CompleteReviewRequirementsOnClose(defectID, actor string, now time.Time) {
	defect := b.Defects[defectID]
	if defect == nil {
		return
	}
	for _, requirement := range b.OpenReviewRequirementsForDefect(defectID) {
		treatment := defect.CurrentTreatment()
		if treatment == nil || !ValidSHA256(treatment.EvidenceDigest) {
			continue
		}
		rounds := latestPassingRetests(b, defect, b.ThresholdProfile.RequiredConsecutivePasses)
		if len(rounds) != b.ThresholdProfile.RequiredConsecutivePasses {
			continue
		}
		requirement.TreatmentVersionID = treatment.VersionID
		requirement.EvidenceDigest = treatment.EvidenceDigest
		requirement.Status = ReviewRequirementCompleted
		requirement.CompletedBy = actor
		completed := now.UTC()
		requirement.CompletedAt = &completed
		requirement.CompletedRoundIDs = rounds
		b.recordRequirementCompletion(requirement)
	}
}

func (b *QualificationBatch) recordRequirementCompletion(requirement *ReviewRequirement) {
	if requirement == nil || requirement.CompletedAt == nil {
		return
	}
	for i := range b.ReviewHistory {
		record := &b.ReviewHistory[i]
		if record.Checklist.ChecklistID != requirement.ChecklistID {
			continue
		}
		for _, completion := range record.RequirementCompletions {
			if completion.RequirementID == requirement.RequirementID {
				return
			}
		}
		record.RequirementCompletions = append(record.RequirementCompletions, ReviewRequirementCompletion{RequirementID: requirement.RequirementID, CompletedBy: requirement.CompletedBy, CompletedAt: requirement.CompletedAt.UTC(), CompletedRoundIDs: append([]string(nil), requirement.CompletedRoundIDs...)})
		sort.Slice(record.RequirementCompletions, func(i, j int) bool {
			return record.RequirementCompletions[i].RequirementID < record.RequirementCompletions[j].RequirementID
		})
		return
	}
}

func latestPassingRetests(b *QualificationBatch, defect *DefectCase, required int) []string {
	if required <= 0 || len(defect.RetestRoundIDs) < required {
		return nil
	}
	ids := make([]string, 0, required)
	for i := len(defect.RetestRoundIDs) - 1; i >= 0 && len(ids) < required; i-- {
		round := b.FindRound(defect.RetestRoundIDs[i])
		if round == nil || !round.SupportsQualification() || !round.Result.Passed {
			return nil
		}
		ids = append(ids, round.RoundID)
	}
	for i, j := 0, len(ids)-1; i < j; i, j = i+1, j-1 {
		ids[i], ids[j] = ids[j], ids[i]
	}
	return ids
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
