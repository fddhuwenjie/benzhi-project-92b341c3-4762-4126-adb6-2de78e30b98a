package domain

import (
	"fmt"
	"sort"
	"time"
)

type SubmissionBlocker struct {
	Category      string `json:"category"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	TapID         string `json:"tap_id,omitempty"`
	RoundID       string `json:"round_id,omitempty"`
	DefectID      string `json:"defect_id,omitempty"`
	RequirementID string `json:"requirement_id,omitempty"`
	NextAction    string `json:"next_action"`
}

type SubmissionPreflight struct {
	BatchID                   string              `json:"batch_id"`
	Revision                  uint64              `json:"revision"`
	Ready                     bool                `json:"ready"`
	Blockers                  []SubmissionBlocker `json:"blockers"`
	Warnings                  []SubmissionBlocker `json:"warnings"`
	FactDigest                string              `json:"fact_digest"`
	EarliestCalibrationExpiry *time.Time          `json:"earliest_calibration_expiry,omitempty"`
	RemainingValidSeconds     int64               `json:"remaining_valid_seconds"`
	RequirementDigest         string              `json:"requirement_digest"`
	UncompletedRequirements   []ReviewRequirement `json:"uncompleted_requirements"`
}

func (b *QualificationBatch) SubmissionFacts(now time.Time) SubmissionPreflight {
	out := SubmissionPreflight{BatchID: b.BatchID, Revision: b.Revision, Blockers: []SubmissionBlocker{}, Warnings: []SubmissionBlocker{}, UncompletedRequirements: []ReviewRequirement{}}
	calibrations := map[string]Calibration{}
	for _, calibration := range b.CalibrationHistory {
		calibrations[calibration.Reference] = calibration
	}
	for _, tapID := range SortedTapIDs(b.Taps) {
		tap := b.Taps[tapID]
		if tap.LatestMeasurementRoundID == "" {
			out.Blockers = append(out.Blockers, SubmissionBlocker{Category: "coverage", Severity: "hard", Message: "测孔缺少有效初测", TapID: tapID, NextAction: "record_measurement"})
		}
		if tap.QualificationStatus != TapQualified {
			out.Blockers = append(out.Blockers, SubmissionBlocker{Category: "tap_qualification", Severity: "hard", Message: "测孔尚未合格", TapID: tapID, NextAction: "open_tap"})
		}
	}
	if b.Calibration == nil || b.CalibrationIsInvalid(calibrationReference(b.Calibration)) || b.Calibration == nil || !b.Calibration.ValidUntil.After(now) {
		out.Blockers = append(out.Blockers, SubmissionBlocker{Category: "calibration", Severity: "hard", Message: "当前有效校准不存在或已经失效", NextAction: "register_calibration"})
	}
	for _, roundID := range b.EffectiveRoundIDs() {
		round := b.FindRound(roundID)
		calibration, exists := calibrations[round.CalibrationRef]
		if !exists || b.CalibrationIsInvalid(round.CalibrationRef) || !calibration.ValidUntil.After(now) {
			out.Blockers = append(out.Blockers, SubmissionBlocker{Category: "effective_round", Severity: "hard", Message: "有效轮次引用的校准无效", TapID: round.TapID, RoundID: roundID, NextAction: "record_replacement_round"})
			continue
		}
		if out.EarliestCalibrationExpiry == nil || calibration.ValidUntil.Before(*out.EarliestCalibrationExpiry) {
			expires := calibration.ValidUntil
			out.EarliestCalibrationExpiry = &expires
		}
	}
	defectIDs := make([]string, 0, len(b.Defects))
	for id := range b.Defects {
		defectIDs = append(defectIDs, id)
	}
	sort.Strings(defectIDs)
	for _, id := range defectIDs {
		defect := b.Defects[id]
		if defect.Status != DefectClosed && defect.Status != DefectVoided {
			out.Blockers = append(out.Blockers, SubmissionBlocker{Category: "open_defect", Severity: "hard", Message: "缺陷尚未关闭", TapID: defect.TapID, DefectID: id, NextAction: "open_defect"})
		}
		if defect.Status == DefectTreated {
			passes := b.ConsecutivePassingRetests(id)
			if passes < b.ThresholdProfile.RequiredConsecutivePasses {
				out.Blockers = append(out.Blockers, SubmissionBlocker{Category: "consecutive_retests", Severity: "hard", Message: fmt.Sprintf("连续合格复测 %d/%d", passes, b.ThresholdProfile.RequiredConsecutivePasses), TapID: defect.TapID, DefectID: id, NextAction: "record_retest"})
			}
		}
	}
	for _, requirement := range b.SortedOpenReviewRequirements() {
		copy := *requirement
		copy.RelatedDefectIDs = append([]string(nil), requirement.RelatedDefectIDs...)
		copy.CompletedRoundIDs = append([]string(nil), requirement.CompletedRoundIDs...)
		out.UncompletedRequirements = append(out.UncompletedRequirements, copy)
		defectID := ""
		if len(requirement.RelatedDefectIDs) > 0 {
			defectID = requirement.RelatedDefectIDs[0]
		}
		out.Blockers = append(out.Blockers, SubmissionBlocker{Category: "review_requirement", Severity: "hard", Message: "复核退回要求尚未完成：" + requirement.RequirementText, TapID: requirement.TapID, DefectID: defectID, RequirementID: requirement.RequirementID, NextAction: "open_review_requirement"})
	}
	out.RequirementDigest = b.ReviewRequirementDigest()
	if out.EarliestCalibrationExpiry != nil {
		out.RemainingValidSeconds = int64(out.EarliestCalibrationExpiry.Sub(now).Seconds())
		if out.RemainingValidSeconds > 0 && out.RemainingValidSeconds <= int64((24*time.Hour).Seconds()) {
			out.Warnings = append(out.Warnings, SubmissionBlocker{Category: "calibration_expiring", Severity: "warning", Message: "最早校准将在 24 小时内失效", NextAction: "review_calibration"})
		}
	}
	out.Ready = len(out.Blockers) == 0 && (b.State == StateBaselineFrozen || b.State == StateRemediation)
	return out
}

func calibrationReference(calibration *Calibration) string {
	if calibration == nil {
		return ""
	}
	return calibration.Reference
}

type ReviewerConflictFact struct {
	Role     string `json:"role"`
	Action   string `json:"action"`
	EntityID string `json:"entity_id,omitempty"`
	Sequence uint64 `json:"sequence,omitempty"`
}

type ReviewerEligibility struct {
	BatchID    string                 `json:"batch_id"`
	ReviewerID string                 `json:"reviewer_id"`
	CanApprove bool                   `json:"can_approve"`
	Conflicts  []ReviewerConflictFact `json:"conflicts"`
}

func (b *QualificationBatch) ReviewerPreflight(reviewer string) (ReviewerEligibility, error) {
	if b.State != StateUnderReview {
		return ReviewerEligibility{}, NewError(CodeState, "只有待复核批次可以执行复核员独立性预检")
	}
	if reviewer == "" {
		return ReviewerEligibility{}, NewError(CodeInvalid, "reviewer_id 不能为空")
	}
	out := ReviewerEligibility{BatchID: b.BatchID, ReviewerID: reviewer, Conflicts: []ReviewerConflictFact{}}
	if reviewer == b.CreatedBy {
		out.Conflicts = append(out.Conflicts, ReviewerConflictFact{Role: "creator", Action: "batch.created"})
	}
	for _, round := range b.Rounds {
		if round.OperatorID == reviewer {
			out.Conflicts = append(out.Conflicts, ReviewerConflictFact{Role: "measurement", Action: "measurement.recorded", EntityID: round.RoundID})
		}
	}
	for _, defect := range b.Defects {
		for _, treatment := range defect.TreatmentVersions {
			if treatment.TechnicianID == reviewer {
				out.Conflicts = append(out.Conflicts, ReviewerConflictFact{Role: "remediation", Action: "defect.treated", EntityID: treatment.VersionID})
			}
		}
	}
	for i := range out.Conflicts {
		for _, audit := range b.Audit {
			if audit.ActorID == reviewer && (audit.Action == out.Conflicts[i].Action || containsRelatedID(audit.RelatedIDs, out.Conflicts[i].EntityID)) {
				out.Conflicts[i].Sequence = audit.Sequence
				out.Conflicts[i].Action = audit.Action
				break
			}
		}
	}
	out.CanApprove = len(out.Conflicts) == 0
	return out, nil
}

func containsRelatedID(values []string, id string) bool {
	if id == "" {
		return false
	}
	for _, value := range values {
		if value == id {
			return true
		}
	}
	return false
}
