package application

import (
	"strings"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/evidence"
	"pressure-tap-qualification/internal/store"
)

func (s *Service) SubmitForReview(batchID string, cmd SubmitCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		preflight := b.SubmissionFacts(now)
		preflightDigest, err := evidence.SubmissionPreflightDigest(preflight)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		preflight.FactDigest = preflightDigest
		if cmd.PreflightDigest != "" && cmd.PreflightDigest != preflightDigest {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "送审预检摘要已陈旧，请刷新后重新确认")
		}
		if err := b.Submit(now); err != nil {
			return nil, store.EventDraft{}, err
		}
		digest, err := evidence.SnapshotDigest(b)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		b.ReviewSnapshot.Digest = digest
		b.ReviewSnapshot.PreflightDigest = preflightDigest
		b.ReviewSnapshot.BatchInfoDigest = b.FrozenBatchInfoDigest
		b.ReviewSnapshot.Facts = b.CurrentReviewFacts()
		pack, err := evidence.DifferencePackage(b.ReviewHistory, b.ReviewSnapshot.Facts, digest)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		b.ReviewSnapshot.DifferencePackage = &pack
		b.ReviewChecklist = domain.NewReviewChecklist(b.BatchID, digest, b.ReviewSnapshot.Revision)
		return response(b, "送审快照已形成，业务数据进入只读状态"), event(cmd.ActorID, "review.submitted", "形成全覆盖且无开放缺陷的送审快照", now), nil
	})
}

func (s *Service) PreflightSubmission(batchID string) (domain.SubmissionPreflight, error) {
	b, err := s.repo.Get(batchID)
	if err != nil {
		return domain.SubmissionPreflight{}, err
	}
	cacheable := b.State == domain.StateDraft && b.Calibration == nil && len(b.CalibrationHistory) == 0
	key := submissionPreflightCacheKey{batchID: b.BatchID, revision: b.Revision}
	if cacheable {
		s.submissionPreflightMu.Lock()
		cached, ok := s.submissionPreflightCache[key]
		s.submissionPreflightMu.Unlock()
		if ok {
			return cached, nil
		}
	}
	preflight := b.SubmissionFacts(s.clock())
	digest, err := evidence.SubmissionPreflightDigest(preflight)
	if err != nil {
		return domain.SubmissionPreflight{}, err
	}
	preflight.FactDigest = digest
	if cacheable {
		s.submissionPreflightMu.Lock()
		s.submissionPreflightCache[key] = preflight
		s.submissionPreflightMu.Unlock()
	}
	return preflight, nil
}

func (s *Service) PreflightReviewer(batchID, reviewerID string) (domain.ReviewerEligibility, error) {
	b, err := s.repo.Get(batchID)
	if err != nil {
		return domain.ReviewerEligibility{}, err
	}
	return b.ReviewerPreflight(strings.TrimSpace(reviewerID))
}

func (s *Service) Review(batchID string, cmd ReviewCommand) (CommandResponse, error) {
	if cmd.Decision != domain.DecisionApproved && cmd.Decision != domain.DecisionReturned {
		return CommandResponse{}, domain.NewError(domain.CodeInvalid, "复核结论必须为 approved 或 returned")
	}
	if strings.TrimSpace(cmd.Note) == "" {
		return CommandResponse{}, domain.NewError(domain.CodeInvalid, "复核意见不能为空")
	}
	out, err := s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		if cmd.Decision == domain.DecisionApproved {
			if b.ReviewSnapshot != nil && b.ReviewSnapshot.DifferencePackage != nil && !b.ReviewSnapshot.DifferencePackage.FirstSubmission && cmd.DifferenceDigest != b.ReviewSnapshot.DifferencePackage.Digest {
				return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "送审差异包确认摘要已陈旧或尚未确认")
			}
			valid, reason := b.SnapshotValidity(now)
			if valid {
				digest, digestErr := evidence.SnapshotDigest(b)
				if digestErr != nil {
					return nil, store.EventDraft{}, digestErr
				}
				if digest != b.ReviewSnapshot.Digest {
					valid = false
					reason = "送审快照摘要与当前放行事实不一致"
				}
			}
			if !valid {
				b.ExpireReviewSnapshot(reason)
				return response(b, "送审快照已失效，批次已受控退回返修"), event(cmd.ActorID, "review.snapshot_expired", "批准前复验失败："+reason, now), nil
			}
		}
		returned, err := b.CompleteReview(cmd.ActorID, cmd.Decision, cmd.Note, cmd.Items, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		if cmd.Decision == domain.DecisionReturned {
			requirementIDs, requirementErr := b.CreateReviewRequirements(cmd.ActorID, now)
			if requirementErr != nil {
				return nil, store.EventDraft{}, requirementErr
			}
			b.ArchiveReview(cmd.Note)
			if len(returned) == 0 {
				return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "退回决定没有关联测孔")
			}
			if err := b.ReturnReview(); err != nil {
				return nil, store.EventDraft{}, err
			}
			return response(b, "复核已退回，返修要求已建立并等待证据与复测"), eventWithRelated(cmd.ActorID, "review.returned", "独立复核退回："+cmd.Note, now, requirementIDs...), nil
		}
		if err := b.Approve(cmd.ActorID, now); err != nil {
			return nil, store.EventDraft{}, err
		}
		b.ArchiveReview(cmd.Note)
		return response(b, "独立复核批准，准备封存资格证书"), event(cmd.ActorID, "review.approved", "独立复核批准："+cmd.Note, now), nil
	})
	if err != nil {
		return out, err
	}
	if cmd.Decision == domain.DecisionApproved && out.State == domain.StateApproved {
		batch, err := s.repo.Get(batchID)
		if err != nil {
			return out, err
		}
		head, err := s.repo.AuditHead(batchID)
		if err != nil {
			return out, err
		}
		cert, err := s.evidence.Issue(batch, cmd.ActorID, cmd.Note, head, *batch.ApprovedAt)
		if err != nil {
			return out, err
		}
		out.Certificate = cert
		out.Message = "独立复核已批准，资格证书已不可变封存"
	}
	return out, nil
}
