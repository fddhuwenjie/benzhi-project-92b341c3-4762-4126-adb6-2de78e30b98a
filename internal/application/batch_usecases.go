package application

import (
	"fmt"
	"time"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/store"
)

func (s *Service) CreateBatch(cmd CreateBatchCommand) (CommandResponse, error) {
	return s.execute(cmd.BatchID, cmd.CommandMeta, cmd, true, func(slot *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		profile := cmd.ThresholdProfile
		if profile == (domain.ThresholdProfile{}) {
			profile = domain.DefaultThresholds()
		}
		b, err := domain.NewBatch(cmd.BatchID, cmd.ModelCode, cmd.TestObjective, cmd.ActorID, profile, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		inputs := pressureTaps(cmd.Taps)
		if _, err = b.ReviseBaseline(inputs, "建立批次初始测孔清单", cmd.ActorID, now); err != nil {
			return nil, store.EventDraft{}, err
		}
		*slot = *b
		return response(slot, "验收批次已建立"), event(cmd.ActorID, "batch.created", fmt.Sprintf("建立模型 %s 的验收批次，包含 %d 个测孔", cmd.ModelCode, len(cmd.Taps)), now), nil
	})
}

func (s *Service) FreezeBaseline(batchID string, cmd FreezeCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		currentInfoDigest := b.BatchInfoDigest()
		if cmd.BatchInfoDigest != "" && cmd.BatchInfoDigest != currentInfoDigest {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "批次信息确认摘要已陈旧，请刷新后重新确认")
		}
		if cmd.ConfirmedDiffSummary == "" {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "冻结命令必须携带 confirmed_diff_summary")
		}
		currentThreshold := b.CurrentThresholdRevision()
		if currentThreshold == nil {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeState, "批次缺少阈值方案")
		}
		if cmd.ThresholdDigest == "" {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "冻结命令必须携带 threshold_digest")
		}
		if cmd.ThresholdDigest != currentThreshold.Digest {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "阈值确认摘要已陈旧，请刷新后重新确认")
		}
		topology := b.TopologyPreflight()
		if topology.ErrorCount > 0 {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "拓扑预检仍有 %d 项错误", topology.ErrorCount)
		}
		if topology.WarningCount > 0 && cmd.TopologyDigest == "" {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "拓扑预检包含警告，冻结必须携带 topology_digest 并逐项确认")
		}
		if cmd.TopologyDigest != "" {
			if err := b.ConfirmTopology(cmd.TopologyDigest, cmd.WarningAcknowledgements, cmd.ActorID, now); err != nil {
				return nil, store.EventDraft{}, err
			}
		} else if len(cmd.WarningAcknowledgements) > 0 {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeInvalid, "拓扑警告确认必须携带 topology_digest")
		}
		if err := b.FreezeBaselineConfirmed(cmd.ConfirmedDiffSummary); err != nil {
			return nil, store.EventDraft{}, err
		}
		b.FrozenThresholdVersion = currentThreshold.Version
		b.FrozenThresholdDigest = currentThreshold.Digest
		b.FrozenBatchInfoDigest = currentInfoDigest
		b.FrozenDraftVersion = b.DraftRevision
		b.FrozenBaselineDigest = domain.BaselineDefinitionDigest(b.Taps)
		for id, tap := range b.Taps {
			defectID := "MISSING-" + id
			b.Defects[defectID] = &domain.DefectCase{
				DefectID:     defectID,
				BatchID:      b.BatchID,
				TapID:        id,
				DefectType:   domain.DefectMissing,
				Severity:     "medium",
				RuleSnapshot: "frozen-baseline-requires-initial-measurement/v1",
				Status:       domain.DefectOpen,
			}
			tap.QualificationStatus = domain.TapPending
		}
		return response(b, "测孔基线已冻结"), event(cmd.ActorID, "baseline.frozen", fmt.Sprintf("冻结 %d 个测孔的基线", len(b.Taps)), now), nil
	})
}

func (s *Service) PreflightBatchInfoRevision(batchID string, cmd ReviseBatchInfoCommand) (domain.BatchInfoRevision, error) {
	if err := validateMeta(cmd.CommandMeta); err != nil {
		return domain.BatchInfoRevision{}, err
	}
	b, err := s.repo.Get(batchID)
	if err != nil {
		return domain.BatchInfoRevision{}, err
	}
	if b.Revision != cmd.ExpectedRevision {
		return domain.BatchInfoRevision{}, domain.NewError(domain.CodeConflict, "修订冲突：期望 %d，当前 %d", cmd.ExpectedRevision, b.Revision)
	}
	return b.PreviewBatchInfoRevision(cmd.ModelCode, cmd.TestObjective, cmd.Reason, cmd.ActorID, s.clock())
}

func (s *Service) ReviseBatchInfo(batchID string, cmd ReviseBatchInfoCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		preview, err := b.PreviewBatchInfoRevision(cmd.ModelCode, cmd.TestObjective, cmd.Reason, cmd.ActorID, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		if cmd.ConfirmedDigest != "" && cmd.ConfirmedDigest != preview.Digest {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "批次信息修订摘要已变化，请重新确认")
		}
		revision, err := b.ReviseBatchInfo(cmd.ModelCode, cmd.TestObjective, cmd.Reason, cmd.ActorID, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		return response(b, "草拟批次信息已追加修订版本"), event(cmd.ActorID, "batch.info_revised", fmt.Sprintf("批次信息版本 %d：%s；摘要 %s", revision.Version, revision.Reason, revision.Digest), now), nil
	})
}

func (s *Service) PreflightThresholdRevision(batchID string, cmd ReviseThresholdCommand) (domain.ThresholdRevision, error) {
	if err := validateMeta(cmd.CommandMeta); err != nil {
		return domain.ThresholdRevision{}, err
	}
	b, err := s.repo.Get(batchID)
	if err != nil {
		return domain.ThresholdRevision{}, err
	}
	if b.Revision != cmd.ExpectedRevision {
		return domain.ThresholdRevision{}, domain.NewError(domain.CodeConflict, "修订冲突：期望 %d，当前 %d", cmd.ExpectedRevision, b.Revision)
	}
	return b.PreviewThresholdRevision(cmd.ThresholdProfile, cmd.Reason, cmd.ActorID, s.clock())
}

func (s *Service) ReviseThresholds(batchID string, cmd ReviseThresholdCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		preview, err := b.PreviewThresholdRevision(cmd.ThresholdProfile, cmd.Reason, cmd.ActorID, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		if cmd.ConfirmedSummary == "" || cmd.ConfirmedSummary != preview.Digest {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "阈值修订确认摘要不一致，请重新预检")
		}
		revision, err := b.ReviseThresholds(cmd.ThresholdProfile, cmd.Reason, cmd.ActorID, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		return response(b, "草拟阈值方案已追加版本"), event(cmd.ActorID, "threshold.revised", fmt.Sprintf("阈值方案版本 %d：%s；摘要 %s", revision.Version, revision.Reason, revision.Digest), now), nil
	})
}

func (s *Service) PreflightTopology(batchID string, expectedRevision uint64) (domain.TopologyPreflight, error) {
	b, err := s.repo.Get(batchID)
	if err != nil {
		return domain.TopologyPreflight{}, err
	}
	if b.Revision != expectedRevision {
		return domain.TopologyPreflight{}, domain.NewError(domain.CodeConflict, "修订冲突：期望 %d，当前 %d", expectedRevision, b.Revision)
	}
	if b.State != domain.StateDraft {
		return domain.TopologyPreflight{}, domain.NewError(domain.CodeState, "只有草拟批次可以执行拓扑预检")
	}
	return b.TopologyPreflight(), nil
}

func (s *Service) RegisterCalibration(batchID string, cmd CalibrationCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		c := domain.Calibration{Reference: cmd.Reference, InstrumentSummary: cmd.InstrumentSummary, ValidUntil: cmd.ValidUntil}
		if err := b.SetCalibration(c, cmd.ActorID, now); err != nil {
			return nil, store.EventDraft{}, err
		}
		return response(b, "校准依据已登记"), event(cmd.ActorID, "calibration.registered", "登记校准引用 "+cmd.Reference, now), nil
	})
}

func (s *Service) InvalidateCalibration(batchID string, cmd CalibrationInvalidationCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		discoveredAt := cmd.DiscoveredAt
		if discoveredAt.IsZero() {
			discoveredAt = now
		}
		affected, err := b.InvalidateCalibration(cmd.CalibrationRef, cmd.Reason, cmd.EvidenceDigest, cmd.ActorID, discoveredAt)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		out := response(b, fmt.Sprintf("校准已登记失准，隔离 %d 个轮次", len(affected)))
		out.AffectedRoundIDs = affected
		return out, event(cmd.ActorID, "calibration.invalidated", fmt.Sprintf("校准 %s 失准，隔离轮次 %d 个：%s", cmd.CalibrationRef, len(affected), cmd.Reason), now), nil
	})
}

func (s *Service) ReviseBaseline(batchID string, cmd ReviseBaselineCommand) (CommandResponse, error) {
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		diff, err := b.ReviseBaseline(pressureTaps(cmd.Taps), cmd.Reason, cmd.ActorID, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		out := response(b, "草拟测孔清单已修订")
		return out, event(cmd.ActorID, "baseline.revised", fmt.Sprintf("草拟修订 %d：新增 %d、修改 %d、删除 %d；原因：%s；差异摘要 %s", b.DraftRevision, len(diff.Added), len(diff.Modified), len(diff.Deleted), cmd.Reason, diff.Summary), now), nil
	})
}

func pressureTaps(inputs []TapInput) []domain.PressureTap {
	out := make([]domain.PressureTap, 0, len(inputs))
	for _, input := range inputs {
		out = append(out, domain.PressureTap{TapID: input.TapID, Label: input.Label, SurfaceZone: input.SurfaceZone, NominalDiameterMM: input.NominalDiameterMM, NeighborTapIDs: input.NeighborTapIDs})
	}
	return out
}

func uniqueID(prefix string, now time.Time, index int) string {
	return fmt.Sprintf("%s-%d-%d", prefix, now.UnixNano(), index)
}
