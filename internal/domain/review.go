package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

func (b *QualificationBatch) Submit(now time.Time) error {
	if err := b.EligibleForReview(now); err != nil {
		return err
	}
	t := now.UTC()
	b.SubmittedAt = &t
	b.State = StateUnderReview
	references := map[string]bool{}
	validUntil := time.Time{}
	for _, roundID := range b.EffectiveRoundIDs() {
		round := b.FindRound(roundID)
		references[round.CalibrationRef] = true
		for _, calibration := range b.CalibrationHistory {
			if calibration.Reference == round.CalibrationRef && (validUntil.IsZero() || calibration.ValidUntil.Before(validUntil)) {
				validUntil = calibration.ValidUntil
			}
		}
	}
	calibrationReferences := make([]string, 0, len(references))
	for reference := range references {
		calibrationReferences = append(calibrationReferences, reference)
	}
	sort.Strings(calibrationReferences)
	thresholdVersion := b.FrozenThresholdVersion
	if thresholdVersion == 0 && b.CurrentThresholdRevision() != nil {
		thresholdVersion = b.CurrentThresholdRevision().Version
	}
	qualification := strings.Join(b.EffectiveRoundIDs(), ",") + ":all-qualified"
	qsum := sha256.Sum256([]byte(qualification))
	head := ""
	if len(b.Audit) > 0 {
		head = b.Audit[len(b.Audit)-1].Digest
	}
	b.ReviewSnapshot = &ReviewSnapshot{Revision: b.Revision + 1, CreatedAt: t, Coverage: b.Coverage(), TapCount: len(b.Taps), MeasurementCount: len(b.EffectiveRoundIDs()), Digest: b.SnapshotDigest(), CalibrationReferences: calibrationReferences, ValidUntil: validUntil, ThresholdVersion: thresholdVersion, EffectiveRoundIDs: b.EffectiveRoundIDs(), QualificationSummary: hex.EncodeToString(qsum[:]), AuditHeadDigest: head}
	return nil
}

func (b *QualificationBatch) SnapshotValidity(now time.Time) (bool, string) {
	if b.ReviewSnapshot == nil {
		return false, "送审快照不存在"
	}
	if !b.ReviewSnapshot.ValidUntil.After(now) {
		return false, "送审快照所引用的校准已经失效"
	}
	if blockers := b.QualificationBlockers(now); len(blockers) > 0 {
		return false, strings.Join(blockers, "；")
	}
	head := ""
	if len(b.Audit) > 0 {
		head = b.Audit[len(b.Audit)-1].Digest
	}
	if head != b.ReviewSnapshot.AuditHeadDigest {
		last := AuditEvent{}
		if len(b.Audit) > 0 {
			last = b.Audit[len(b.Audit)-1]
		}
		if last.Action != "review.submitted" || last.PreviousDigest != b.ReviewSnapshot.AuditHeadDigest {
			return false, "送审后的审计链头已变化"
		}
	}
	return true, ""
}

func (b *QualificationBatch) ExpireReviewSnapshot(reason string) {
	if b.ReviewSnapshot != nil {
		b.ReviewSnapshot.ExpiredReason = reason
	}
	b.ArchiveReview("送审快照失效：" + reason)
	b.State = StateRemediation
	b.ReviewSnapshot = nil
	b.ReviewChecklist = nil
	b.SubmittedAt = nil
}

func (b *QualificationBatch) ReturnReview() error {
	if b.State != StateUnderReview {
		return NewError(CodeState, "批次不在复核中")
	}
	b.State = StateRemediation
	b.ReviewSnapshot = nil
	b.ReviewChecklist = nil
	b.SubmittedAt = nil
	return nil
}

func (b *QualificationBatch) CheckReviewer(reviewer string) error {
	if reviewer == "" {
		return NewError(CodeInvalid, "复核员不能为空")
	}
	if b.State == StateUnderReview {
		eligibility, err := b.ReviewerPreflight(reviewer)
		if err != nil {
			return err
		}
		if !eligibility.CanApprove {
			return NewError(CodeForbidden, "复核员存在建档、测量或返修参与事实，不满足独立性")
		}
		return nil
	}
	if b.MeasurementParticipants[reviewer] || b.RemediationParticipants[reviewer] {
		return NewError(CodeForbidden, "复核员参与过测量或返修，不满足独立性")
	}
	if reviewer == b.CreatedBy {
		return NewError(CodeForbidden, "批次建立人不能执行独立批准")
	}
	return nil
}

func (b *QualificationBatch) Approve(reviewer string, now time.Time) error {
	if b.State != StateUnderReview || b.ReviewSnapshot == nil {
		return NewError(CodeState, "批次没有可批准的送审快照")
	}
	if err := b.CheckReviewer(reviewer); err != nil {
		return err
	}
	t := now.UTC()
	b.State = StateApproved
	b.ApprovedAt = &t
	return nil
}
