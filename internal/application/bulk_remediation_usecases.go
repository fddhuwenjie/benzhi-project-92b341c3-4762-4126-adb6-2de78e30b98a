package application

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/store"
)

type BatchTreatmentPreviewItem struct {
	DefectID             string            `json:"defect_id"`
	TapID                string            `json:"tap_id"`
	DefectType           domain.DefectType `json:"defect_type"`
	VersionID            string            `json:"version_id"`
	Sequence             int               `json:"sequence"`
	Note                 string            `json:"note,omitempty"`
	ReviewRequirementIDs []string          `json:"review_requirement_ids,omitempty"`
}

type BatchTreatmentPreview struct {
	BatchID  string                      `json:"batch_id"`
	Revision uint64                      `json:"revision"`
	JobID    string                      `json:"job_id"`
	Items    []BatchTreatmentPreviewItem `json:"items"`
	Digest   string                      `json:"digest"`
	Ready    bool                        `json:"ready"`
}

type BatchRetestPreviewRow struct {
	RoundID              string                    `json:"round_id"`
	DefectID             string                    `json:"defect_id"`
	TapID                string                    `json:"tap_id"`
	TreatmentVersionID   string                    `json:"treatment_version_id"`
	Result               domain.MeasurementResult  `json:"result"`
	CurrentConsecutive   int                       `json:"current_consecutive"`
	EstimatedConsecutive int                       `json:"estimated_consecutive"`
	EstimatedRemaining   int                       `json:"estimated_remaining"`
	NeighborResponses    []domain.NeighborResponse `json:"neighbor_responses"`
	WorstNeighborTapID   string                    `json:"worst_neighbor_tap_id,omitempty"`
	ReviewRequirementIDs []string                  `json:"review_requirement_ids,omitempty"`
}

type BatchRetestPreview struct {
	BatchID        string                  `json:"batch_id"`
	Revision       uint64                  `json:"revision"`
	CalibrationRef string                  `json:"calibration_ref"`
	Rows           []BatchRetestPreviewRow `json:"rows"`
	Digest         string                  `json:"digest"`
	Ready          bool                    `json:"ready"`
}

func previewDigest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func treatmentVersionID(jobID, defectID string, sequence int, supplied string) string {
	if strings.TrimSpace(supplied) != "" {
		return supplied
	}
	return fmt.Sprintf("%s-%s-%d", jobID, defectID, sequence)
}

func preflightBatchTreatment(b *domain.QualificationBatch, cmd BatchTreatmentCommand) (BatchTreatmentPreview, error) {
	if b.State != domain.StateRemediation && b.State != domain.StateBaselineFrozen {
		return BatchTreatmentPreview{}, domain.NewError(domain.CodeState, "当前状态不能批量处置缺陷")
	}
	if strings.TrimSpace(cmd.JobID) == "" || strings.TrimSpace(cmd.Cause) == "" || strings.TrimSpace(cmd.CorrectiveAction) == "" {
		return BatchTreatmentPreview{}, domain.NewError(domain.CodeInvalid, "作业标识、共同原因和共同处置动作不能为空")
	}
	if !domain.ValidSHA256(cmd.EvidenceDigest) {
		return BatchTreatmentPreview{}, domain.NewError(domain.CodeInvalid, "证据摘要必须为 64 位 SHA-256 十六进制值")
	}
	if len(cmd.Items) == 0 {
		return BatchTreatmentPreview{}, domain.NewError(domain.CodeInvalid, "批量处置至少选择一个缺陷")
	}
	for _, defect := range b.Defects {
		for _, version := range defect.TreatmentVersions {
			if version.JobID == cmd.JobID {
				return BatchTreatmentPreview{}, domain.NewError(domain.CodeConflict, "共同作业标识 %s 已存在", cmd.JobID)
			}
		}
	}
	seen := map[string]bool{}
	items := make([]BatchTreatmentPreviewItem, 0, len(cmd.Items))
	for i, item := range cmd.Items {
		if seen[item.DefectID] {
			return BatchTreatmentPreview{}, domain.NewError(domain.CodeInvalid, "第 %d 项缺陷 %s 重复", i+1, item.DefectID)
		}
		seen[item.DefectID] = true
		defect := b.Defects[item.DefectID]
		if defect == nil {
			return BatchTreatmentPreview{}, domain.NewError(domain.CodeNotFound, "第 %d 项缺陷 %s 不存在于批次 %s", i+1, item.DefectID, b.BatchID)
		}
		if defect.Status == domain.DefectClosed || defect.Status == domain.DefectVoided {
			return BatchTreatmentPreview{}, domain.NewError(domain.CodeState, "第 %d 项缺陷 %s 已关闭或源轮次已失效", i+1, item.DefectID)
		}
		source := b.FindRound(defect.SourceRoundID)
		if source == nil || !source.SupportsQualification() {
			return BatchTreatmentPreview{}, domain.NewError(domain.CodeState, "第 %d 项缺陷 %s 的源轮次不再有效", i+1, item.DefectID)
		}
		sequence := len(defect.TreatmentVersions) + 1
		versionID := treatmentVersionID(cmd.JobID, item.DefectID, sequence, item.VersionID)
		for _, version := range defect.TreatmentVersions {
			if version.VersionID == versionID {
				return BatchTreatmentPreview{}, domain.NewError(domain.CodeConflict, "处置版本 %s 已存在", versionID)
			}
		}
		if err := b.ValidateTreatmentRequirements(item.DefectID, item.ReviewRequirementIDs); err != nil {
			return BatchTreatmentPreview{}, domain.NewError(domain.ErrorCodeOf(err), "第 %d 项：%v", i+1, err)
		}
		items = append(items, BatchTreatmentPreviewItem{DefectID: item.DefectID, TapID: defect.TapID, DefectType: defect.DefectType, VersionID: versionID, Sequence: sequence, Note: strings.TrimSpace(item.Note), ReviewRequirementIDs: append([]string(nil), item.ReviewRequirementIDs...)})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DefectID < items[j].DefectID })
	preview := BatchTreatmentPreview{BatchID: b.BatchID, Revision: b.Revision, JobID: cmd.JobID, Items: items, Ready: true}
	preview.Digest = previewDigest(preview)
	return preview, nil
}

func (s *Service) PreflightBatchTreatment(batchID string, cmd BatchTreatmentCommand) (BatchTreatmentPreview, error) {
	if err := validateMeta(cmd.CommandMeta); err != nil {
		return BatchTreatmentPreview{}, err
	}
	b, err := s.repo.Get(batchID)
	if err != nil {
		return BatchTreatmentPreview{}, err
	}
	if b.Revision != cmd.ExpectedRevision {
		return BatchTreatmentPreview{}, domain.NewError(domain.CodeConflict, "修订冲突：期望 %d，当前 %d", cmd.ExpectedRevision, b.Revision)
	}
	return preflightBatchTreatment(b, cmd)
}

func (s *Service) TreatDefectsBatch(batchID string, cmd BatchTreatmentCommand) (CommandResponse, error) {
	if !cmd.Confirm {
		return CommandResponse{}, domain.NewError(domain.CodeInvalid, "批量处置必须显式设置 confirm=true")
	}
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		preview, err := preflightBatchTreatment(b, cmd)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		if cmd.PreflightDigest == "" || cmd.PreflightDigest != preview.Digest {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "批量处置预检摘要已陈旧")
		}
		for i, input := range cmd.Items {
			defect := b.Defects[input.DefectID]
			item := preview.Items[i]
			if err := defect.CheckTreatmentAssignee(cmd.ActorID, cmd.HandoverNote, now); err != nil {
				return nil, store.EventDraft{}, err
			}
			if err := defect.AddTreatment(item.VersionID, defect.SourceRoundID, cmd.Cause, cmd.CorrectiveAction, cmd.EvidenceDigest, cmd.ActorID, now); err != nil {
				return nil, store.EventDraft{}, err
			}
			if err := b.LinkTreatmentRequirements(input.DefectID, item.VersionID, cmd.EvidenceDigest, input.ReviewRequirementIDs); err != nil {
				return nil, store.EventDraft{}, err
			}
			version := &defect.TreatmentVersions[len(defect.TreatmentVersions)-1]
			version.JobID = cmd.JobID
			version.ItemNote = item.Note
			version.HandoverNote = cmd.HandoverNote
			version.ReviewRequirementIDs = append([]string(nil), input.ReviewRequirementIDs...)
		}
		b.State = domain.StateRemediation
		b.RemediationParticipants[cmd.ActorID] = true
		related := []string{cmd.JobID}
		for _, item := range preview.Items {
			related = append(related, item.DefectID, item.VersionID)
		}
		return response(b, fmt.Sprintf("共同作业 %s 已原子处置 %d 项缺陷", cmd.JobID, len(cmd.Items))), eventWithRelated(cmd.ActorID, "defect.batch_treated", fmt.Sprintf("共同作业 %s 原子处置 %d 项缺陷", cmd.JobID, len(cmd.Items)), now, related...), nil
	})
}

func preflightBatchRetest(b *domain.QualificationBatch, cmd BatchRetestCommand, now time.Time) (BatchRetestPreview, error) {
	if b.State != domain.StateRemediation && b.State != domain.StateBaselineFrozen {
		return BatchRetestPreview{}, domain.NewError(domain.CodeState, "当前状态不能批量追加关联复测")
	}
	if len(cmd.Rows) == 0 {
		return BatchRetestPreview{}, domain.NewError(domain.CodeInvalid, "批量复测至少包含一行")
	}
	if b.Calibration == nil || b.Calibration.Reference != cmd.CalibrationRef || b.CalibrationIsInvalid(cmd.CalibrationRef) || !b.Calibration.ValidUntil.After(now) {
		return BatchRetestPreview{}, domain.NewError(domain.CodeUnqualified, "批量复测必须统一使用当前有效校准")
	}
	existing := map[string]bool{}
	for _, round := range b.Rounds {
		existing[round.RoundID] = true
	}
	seenRound := map[string]bool{}
	seenDefect := map[string]bool{}
	rows := make([]BatchRetestPreviewRow, 0, len(cmd.Rows))
	for i, row := range cmd.Rows {
		if strings.TrimSpace(row.RoundID) == "" || seenRound[row.RoundID] || existing[row.RoundID] {
			return BatchRetestPreview{}, domain.NewError(domain.CodeConflict, "第 %d 行 round_id 为空或重复", i+1)
		}
		if seenDefect[row.DefectID] {
			return BatchRetestPreview{}, domain.NewError(domain.CodeInvalid, "第 %d 行缺陷 %s 重复", i+1, row.DefectID)
		}
		defect := b.Defects[row.DefectID]
		if defect == nil {
			return BatchRetestPreview{}, domain.NewError(domain.CodeNotFound, "第 %d 行缺陷不存在", i+1)
		}
		if defect.TapID != row.TapID {
			return BatchRetestPreview{}, domain.NewError(domain.CodeInvalid, "第 %d 行缺陷与测孔不一致", i+1)
		}
		if defect.Status != domain.DefectTreated {
			return BatchRetestPreview{}, domain.NewError(domain.CodeState, "第 %d 行缺陷未处置或已关闭", i+1)
		}
		current := defect.CurrentTreatment()
		if current == nil || current.VersionID != row.TreatmentVersionID {
			return BatchRetestPreview{}, domain.NewError(domain.CodeConflict, "第 %d 行引用了非当前处置版本", i+1)
		}
		source := b.FindRound(defect.SourceRoundID)
		if source == nil || !source.SupportsQualification() {
			return BatchRetestPreview{}, domain.NewError(domain.CodeState, "第 %d 行源轮次已作废或被隔离", i+1)
		}
		if err := b.ValidateRetestRequirements(row.DefectID, row.TreatmentVersionID, row.ReviewRequirementIDs); err != nil {
			return BatchRetestPreview{}, domain.NewError(domain.ErrorCodeOf(err), "第 %d 行：%v", i+1, err)
		}
		result, normalized, _, worst, err := classifyMeasurementInput(b, b.Taps[row.TapID], row.SupplyPressurePA, row.SteadyPressurePA, row.DecaySeconds, row.NeighborResponsePA, row.NeighborResponses)
		if err != nil {
			return BatchRetestPreview{}, domain.NewError(domain.CodeInvalid, "第 %d 行：%v", i+1, err)
		}
		if len(row.ReviewRequirementIDs) > 0 && !result.Passed {
			return BatchRetestPreview{}, domain.NewError(domain.CodeUnqualified, "第 %d 行复核要求关联复测未通过确定性判定", i+1)
		}
		currentPasses := b.ConsecutivePassingRetests(row.DefectID)
		estimated := 0
		if result.Passed {
			estimated = currentPasses + 1
		}
		remaining := b.ThresholdProfile.RequiredConsecutivePasses - estimated
		if remaining < 0 {
			remaining = 0
		}
		seenRound[row.RoundID] = true
		seenDefect[row.DefectID] = true
		rows = append(rows, BatchRetestPreviewRow{RoundID: row.RoundID, DefectID: row.DefectID, TapID: row.TapID, TreatmentVersionID: row.TreatmentVersionID, Result: result, CurrentConsecutive: currentPasses, EstimatedConsecutive: estimated, EstimatedRemaining: remaining, NeighborResponses: normalized, WorstNeighborTapID: worst, ReviewRequirementIDs: append([]string(nil), row.ReviewRequirementIDs...)})
	}
	preview := BatchRetestPreview{BatchID: b.BatchID, Revision: b.Revision, CalibrationRef: cmd.CalibrationRef, Rows: rows, Ready: true}
	preview.Digest = previewDigest(preview)
	return preview, nil
}

func (s *Service) PreflightBatchRetest(batchID string, cmd BatchRetestCommand) (BatchRetestPreview, error) {
	if err := validateMeta(cmd.CommandMeta); err != nil {
		return BatchRetestPreview{}, err
	}
	b, err := s.repo.Get(batchID)
	if err != nil {
		return BatchRetestPreview{}, err
	}
	if b.Revision != cmd.ExpectedRevision {
		return BatchRetestPreview{}, domain.NewError(domain.CodeConflict, "修订冲突：期望 %d，当前 %d", cmd.ExpectedRevision, b.Revision)
	}
	return preflightBatchRetest(b, cmd, s.clock())
}

func (s *Service) RecordBatchRetests(batchID string, cmd BatchRetestCommand) (CommandResponse, error) {
	if !cmd.Confirm {
		return CommandResponse{}, domain.NewError(domain.CodeInvalid, "批量复测必须显式设置 confirm=true")
	}
	return s.execute(batchID, cmd.CommandMeta, cmd, false, func(b *domain.QualificationBatch) (any, store.EventDraft, error) {
		now := s.clock()
		preview, err := preflightBatchRetest(b, cmd, now)
		if err != nil {
			return nil, store.EventDraft{}, err
		}
		if cmd.PreflightDigest == "" || cmd.PreflightDigest != preview.Digest {
			return nil, store.EventDraft{}, domain.NewError(domain.CodeConflict, "批量复测预检摘要已陈旧")
		}
		version := b.FrozenThresholdVersion
		if version == 0 && b.CurrentThresholdRevision() != nil {
			version = b.CurrentThresholdRevision().Version
		}
		for i, row := range cmd.Rows {
			defect := b.Defects[row.DefectID]
			result := preview.Rows[i].Result
			maximum := row.NeighborResponsePA
			if row.NeighborResponses != nil || len(b.Taps[row.TapID].NeighborTapIDs) > 0 {
				maximum = result.NeighborRatio * row.SupplyPressurePA
			}
			round := &domain.MeasurementRound{RoundID: row.RoundID, BatchID: b.BatchID, TapID: row.TapID, RoundKind: domain.RoundRetest, SourceRoundID: defect.SourceRoundID, DefectID: row.DefectID, TreatmentVersionID: row.TreatmentVersionID, ReviewRequirementIDs: append([]string(nil), row.ReviewRequirementIDs...), CalibrationRef: cmd.CalibrationRef, OperatorID: cmd.ActorID, SupplyPressurePA: row.SupplyPressurePA, SteadyPressurePA: row.SteadyPressurePA, DecaySeconds: row.DecaySeconds, NeighborResponsePA: maximum, NeighborResponses: preview.Rows[i].NeighborResponses, WorstNeighborTapID: preview.Rows[i].WorstNeighborTapID, FrozenTopologyDigest: b.FrozenTopologyDigest, RecordedAt: now, Result: result, ThresholdVersion: version}
			b.Rounds = append(b.Rounds, round)
			defect.RetestRoundIDs = append(defect.RetestRoundIDs, row.RoundID)
			b.CompleteMappedReviewRequirements(row.DefectID, cmd.ActorID, now)
			if !result.Passed {
				b.Taps[row.TapID].QualificationStatus = domain.TapDefective
			}
		}
		b.MeasurementParticipants[cmd.ActorID] = true
		related := []string{}
		for _, row := range cmd.Rows {
			related = append(related, row.DefectID, row.TapID, row.RoundID, row.TreatmentVersionID)
		}
		return response(b, fmt.Sprintf("已原子提交 %d 个关联复测轮次", len(cmd.Rows))), eventWithRelated(cmd.ActorID, "retest.batch_recorded", fmt.Sprintf("批量记录 %d 个关联复测轮次", len(cmd.Rows)), now, related...), nil
	})
}
