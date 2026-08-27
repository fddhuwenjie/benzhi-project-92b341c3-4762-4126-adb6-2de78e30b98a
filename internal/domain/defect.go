package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var sha256Digest = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func ValidSHA256(value string) bool { return sha256Digest.MatchString(value) }

func (d *DefectCase) AddTreatment(versionID, sourceRoundID, cause, action, evidence, technician string, now time.Time) error {
	if d.Status == DefectClosed {
		return NewError(CodeState, "已关闭缺陷不能再次返修")
	}
	if strings.TrimSpace(versionID) == "" || strings.TrimSpace(cause) == "" || strings.TrimSpace(action) == "" || strings.TrimSpace(technician) == "" {
		return NewError(CodeInvalid, "处置版本、原因、处置动作和技师不能为空")
	}
	if !ValidSHA256(evidence) {
		return NewError(CodeInvalid, "证据摘要必须为 64 位 SHA-256 十六进制值")
	}
	if sourceRoundID != d.SourceRoundID {
		return NewError(CodeInvalid, "处置源轮次与缺陷源轮次不一致")
	}
	for _, v := range d.TreatmentVersions {
		if v.VersionID == versionID {
			return NewError(CodeConflict, "处置版本 %s 已存在", versionID)
		}
	}
	v := TreatmentVersion{VersionID: versionID, Sequence: len(d.TreatmentVersions) + 1, Cause: cause, CorrectiveAction: action, EvidenceDigest: strings.ToLower(evidence), TechnicianID: technician, RecordedAt: now.UTC(), SourceRoundID: sourceRoundID}
	d.TreatmentVersions = append(d.TreatmentVersions, v)
	d.Cause = cause
	d.CorrectiveAction = action
	d.EvidenceDigest = strings.ToLower(evidence)
	d.TechnicianID = technician
	d.Status = DefectTreated
	return nil
}

func (d *DefectCase) CurrentTreatment() *TreatmentVersion {
	if len(d.TreatmentVersions) == 0 {
		return nil
	}
	return &d.TreatmentVersions[len(d.TreatmentVersions)-1]
}

func (b *QualificationBatch) ConsecutivePassingRetests(defectID string) int {
	d := b.Defects[defectID]
	if d == nil {
		return 0
	}
	count := 0
	for i := len(d.RetestRoundIDs) - 1; i >= 0; i-- {
		id := d.RetestRoundIDs[i]
		var found *MeasurementRound
		for _, r := range b.Rounds {
			if r.RoundID == id {
				found = r
				break
			}
		}
		if found == nil || !found.SupportsQualification() || !found.Result.Passed {
			break
		}
		count++
	}
	return count
}

func (b *QualificationBatch) CloseDefect(defectID string, now time.Time) error {
	d := b.Defects[defectID]
	if d == nil {
		return NewError(CodeNotFound, "缺陷不存在")
	}
	if d.Status != DefectTreated {
		return NewError(CodeState, "缺陷必须先登记返修")
	}
	if b.ConsecutivePassingRetests(defectID) < b.ThresholdProfile.RequiredConsecutivePasses {
		return NewError(CodeUnqualified, "连续合格复测次数不足")
	}
	t := now.UTC()
	d.Status = DefectClosed
	d.ClosedAt = &t
	for _, other := range b.Defects {
		if other.TapID == d.TapID && other.Status != DefectClosed {
			b.Taps[d.TapID].QualificationStatus = TapDefective
			return nil
		}
	}
	b.Taps[d.TapID].QualificationStatus = TapQualified
	return nil
}

func (b *QualificationBatch) ResolveMissingDefects(tapID string, now time.Time) {
	for _, defect := range b.Defects {
		if defect.TapID == tapID && defect.DefectType == DefectMissing && defect.Status == DefectOpen {
			t := now.UTC()
			defect.Status = DefectClosed
			defect.Cause = "基线冻结时尚无测量记录"
			defect.CorrectiveAction = "完成首次连通性测量"
			defect.ClosedAt = &t
		}
	}
}

func (b *QualificationBatch) ReturnTapsForRetest(tapIDs []string, requirements map[string]string, now time.Time) error {
	if b.State != StateUnderReview {
		return NewError(CodeState, "批次不在复核中")
	}
	if len(tapIDs) == 0 {
		return NewError(CodeInvalid, "退回时至少选择一个需要补充复测的测孔")
	}
	seen := map[string]bool{}
	for i, tapID := range tapIDs {
		if seen[tapID] {
			return NewError(CodeInvalid, "退回测孔 %s 重复", tapID)
		}
		seen[tapID] = true
		tap := b.Taps[tapID]
		if tap == nil {
			return NewError(CodeNotFound, "退回测孔 %s 不存在", tapID)
		}
		if tap.LatestMeasurementRoundID == "" {
			return NewError(CodeState, "测孔 %s 没有可供复核的源轮次", tapID)
		}
		requirement := strings.TrimSpace(requirements[tapID])
		if requirement == "" {
			return NewError(CodeInvalid, "测孔 %s 缺少补充要求", tapID)
		}
		id := fmt.Sprintf("REVIEW-%d-%d", now.UnixNano(), i)
		b.Defects[id] = &DefectCase{
			DefectID:          id,
			BatchID:           b.BatchID,
			TapID:             tapID,
			DefectType:        DefectMissing,
			Severity:          "medium",
			RuleSnapshot:      "independent-review-supplemental-retest/v1",
			SourceRoundID:     tap.LatestMeasurementRoundID,
			Cause:             requirement,
			CorrectiveAction:  "按独立复核意见补充连续合格复测",
			EvidenceDigest:    b.ReviewSnapshot.Digest,
			RetestRoundIDs:    []string{},
			TreatmentVersions: []TreatmentVersion{{VersionID: fmt.Sprintf("REVIEW-TREAT-%d-%d", now.UnixNano(), i), Sequence: 1, Cause: requirement, CorrectiveAction: "按独立复核意见补充连续合格复测", EvidenceDigest: b.ReviewSnapshot.Digest, TechnicianID: "independent-review", RecordedAt: now.UTC(), SourceRoundID: tap.LatestMeasurementRoundID}},
			Status:            DefectTreated,
		}
		tap.QualificationStatus = TapDefective
	}
	b.State = StateRemediation
	b.ReviewSnapshot = nil
	b.ReviewChecklist = nil
	b.SubmittedAt = nil
	return nil
}
