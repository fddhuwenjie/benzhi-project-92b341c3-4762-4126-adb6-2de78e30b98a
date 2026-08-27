package application

import (
	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/store"
)

func (s *Service) TreatDefect(batchID string, cmd TreatmentCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		if b.State != domain.StateRemediation && b.State != domain.StateBaselineFrozen {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeState, "当前状态不能返修")
		}
		d := b.Defects[cmd.DefectID]
		if d == nil {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeNotFound, "缺陷不存在")
		}
		source := b.FindRound(d.SourceRoundID)
		if source == nil || !source.SupportsQualification() {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeState, "缺陷源轮次已作废或被校准失准隔离")
		}
		if err := d.CheckTreatmentAssignee(cmd.ActorID, cmd.HandoverNote, now); err != nil {
			return nil, store.EventDraft{}, err
		}
		if err := d.AddTreatment(cmd.VersionID, cmd.SourceRoundID, cmd.Cause, cmd.CorrectiveAction, cmd.EvidenceDigest, cmd.ActorID, now); err != nil {
			return nil, store.EventDraft{}, err
		}
		if err := b.LinkTreatmentRequirements(cmd.DefectID, cmd.VersionID, cmd.EvidenceDigest, cmd.ReviewRequirementIDs); err != nil {
			return nil, store.EventDraft{}, err
		}
		d.TreatmentVersions[len(d.TreatmentVersions)-1].HandoverNote = cmd.HandoverNote
		d.TreatmentVersions[len(d.TreatmentVersions)-1].ReviewRequirementIDs = append([]string(nil), cmd.ReviewRequirementIDs...)
		b.State = domain.StateRemediation
		b.RemediationParticipants[cmd.ActorID] = true
		return response(b, "返修记录已保存，等待复测"), eventWithRelated(cmd.ActorID, "defect.treated", "登记缺陷 "+cmd.DefectID+" 的原因、处置和证据", now, cmd.DefectID, cmd.VersionID, cmd.SourceRoundID), nil
	})
}

func (s *Service) CloseDefect(batchID string, cmd CloseDefectCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		if err := b.CloseDefect(cmd.DefectID, now); err != nil {
			return nil, store.EventDraft{}, err
		}
		b.CompleteReviewRequirementsOnClose(cmd.DefectID, cmd.ActorID, now)
		b.Defects[cmd.DefectID].CompleteAssignment(now)
		return response(b, "缺陷已依据连续合格复测关闭"), eventWithRelated(cmd.ActorID, "defect.closed", "关闭缺陷 "+cmd.DefectID, now, cmd.DefectID), nil
	})
}

func (s *Service) AssignDefect(batchID string, cmd AssignDefectCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		defect := b.Defects[cmd.DefectID]
		if defect == nil {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeNotFound, "缺陷不存在")
		}
		assignment, err := defect.Assign(cmd.TechnicianID, cmd.DueAt, cmd.Priority, cmd.Reason, cmd.ActorID, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		return response(b, "返修任务责任人与期限已追加分派"), eventWithRelated(cmd.ActorID, "defect.assigned", "分派缺陷 "+cmd.DefectID+" 给 "+assignment.TechnicianID, now, cmd.DefectID, assignment.TechnicianID), nil
	})
}
