package domain

import (
	"fmt"
	"sort"
)

func (b *QualificationBatch) ValidateIntegrity() error {
	if b.BatchID == "" || b.ModelCode == "" || b.TestObjective == "" || b.CreatedBy == "" {
		return fmt.Errorf("批次核心字段不完整")
	}
	if b.Revision == 0 || uint64(len(b.Audit)) != b.Revision {
		return fmt.Errorf("批次修订号与审计事件数不一致")
	}
	if err := b.ThresholdProfile.Validate(); err != nil {
		return fmt.Errorf("阈值配置损坏: %w", err)
	}
	if len(b.ThresholdHistory) == 0 {
		return fmt.Errorf("阈值方案履历为空")
	}
	for i, revision := range b.ThresholdHistory {
		if revision.Version != uint64(i+1) || revision.Digest != thresholdDigest(revision.Version, revision.Profile, revision.Changes) {
			return fmt.Errorf("阈值方案版本不连续或摘要无效")
		}
	}
	if b.ThresholdHistory[len(b.ThresholdHistory)-1].Profile != b.ThresholdProfile {
		return fmt.Errorf("当前阈值与最新版本不一致")
	}
	if err := b.validateStateIntegrity(); err != nil {
		return err
	}
	if err := b.validateTapIntegrity(); err != nil {
		return err
	}
	if err := b.validateRoundIntegrity(); err != nil {
		return err
	}
	if err := b.validateDefectIntegrity(); err != nil {
		return err
	}
	if err := b.validateCalibrationIntegrity(); err != nil {
		return err
	}
	if err := b.validateReviewIntegrity(); err != nil {
		return err
	}
	return b.validateAuditIntegrity()
}

func (b *QualificationBatch) validateStateIntegrity() error {
	for i, revision := range b.BatchInfoHistory {
		if revision.Version != uint64(i+1) || revision.Digest != batchInfoRevisionDigest(revision) || revision.ActorID == "" || revision.At.IsZero() {
			return fmt.Errorf("批次信息修订履历不连续或摘要无效")
		}
	}
	if b.FrozenBatchInfoDigest != "" && b.FrozenBatchInfoDigest != b.BatchInfoDigest() {
		return fmt.Errorf("冻结批次信息摘要与当前值不一致")
	}
	if b.FrozenBaselineDigest != "" {
		if b.FrozenDraftVersion != b.DraftRevision || b.FrozenBaselineDigest != BaselineDefinitionDigest(b.Taps) {
			return fmt.Errorf("冻结测孔清单版本或摘要无效")
		}
	}
	if b.DraftBaselineDiff != nil && b.DraftBaselineDiff.Summary != diffSummary(*b.DraftBaselineDiff) {
		return fmt.Errorf("草拟基线差异摘要不一致")
	}
	if len(b.DraftHistory) > 0 {
		if b.DraftRevision != b.DraftHistory[len(b.DraftHistory)-1].Revision {
			return fmt.Errorf("草拟修订号与履历不一致")
		}
		for i, revision := range b.DraftHistory {
			if revision.Revision != uint64(i+1) || revision.Diff.Summary != diffSummary(revision.Diff) {
				return fmt.Errorf("草拟修订履历不连续或摘要无效")
			}
		}
	}
	switch b.State {
	case StateDraft:
		if b.BaselineRevision != 0 {
			return fmt.Errorf("草拟批次不应包含冻结修订")
		}
	case StateBaselineFrozen, StateRemediation:
		if b.BaselineRevision == 0 {
			return fmt.Errorf("非草拟批次缺少冻结修订")
		}
		if b.ReviewSnapshot != nil {
			return fmt.Errorf("非送审状态仍保留送审快照")
		}
	case StateUnderReview:
		if b.ReviewSnapshot == nil || b.ReviewChecklist == nil || b.SubmittedAt == nil {
			return fmt.Errorf("复核状态缺少送审快照或送审时间")
		}
	case StateApproved:
		if b.ReviewSnapshot == nil || b.ReviewChecklist == nil || b.SubmittedAt == nil || b.ApprovedAt == nil {
			return fmt.Errorf("批准状态缺少送审或批准事实")
		}
	default:
		return fmt.Errorf("未知批次状态 %q", b.State)
	}
	if b.BaselineRevision > b.Revision {
		return fmt.Errorf("冻结修订晚于当前修订")
	}
	return nil
}

func (b *QualificationBatch) validateTapIntegrity() error {
	if err := b.ValidateTapDefinitions(mapTaps(b.Taps)); err != nil {
		return fmt.Errorf("测孔定义无效: %w", err)
	}
	for id, tap := range b.Taps {
		if tap == nil || tap.TapID != id || tap.BatchID != b.BatchID {
			return fmt.Errorf("测孔 %s 标识或所属批次不一致", id)
		}
		seen := map[string]bool{}
		for _, neighbor := range tap.NeighborTapIDs {
			if neighbor == id || b.Taps[neighbor] == nil {
				return fmt.Errorf("测孔 %s 的相邻孔 %s 无效", id, neighbor)
			}
			if seen[neighbor] {
				return fmt.Errorf("测孔 %s 的相邻孔 %s 重复", id, neighbor)
			}
			seen[neighbor] = true
		}
	}
	return nil
}

func (b *QualificationBatch) validateCalibrationIntegrity() error {
	seen := map[string]bool{}
	for _, c := range b.CalibrationHistory {
		if c.Reference == "" || c.InstrumentSummary == "" || c.RegisteredAt.IsZero() || c.ValidUntil.IsZero() || seen[c.Reference] {
			return fmt.Errorf("校准履历包含空字段或重复引用")
		}
		seen[c.Reference] = true
	}
	if b.Calibration != nil {
		if len(b.CalibrationHistory) == 0 {
			return fmt.Errorf("当前校准不在履历中")
		}
		last := b.CalibrationHistory[len(b.CalibrationHistory)-1]
		if last.Reference != b.Calibration.Reference || last.InstrumentSummary != b.Calibration.InstrumentSummary || !last.ValidUntil.Equal(b.Calibration.ValidUntil) {
			return fmt.Errorf("当前校准与最新履历不一致")
		}
	}
	invalidated := map[string]bool{}
	for _, item := range b.CalibrationInvalidations {
		if !seen[item.CalibrationRef] || invalidated[item.CalibrationRef] || item.DiscoveredAt.IsZero() || item.Reason == "" || item.ActorID == "" || !ValidSHA256(item.EvidenceDigest) {
			return fmt.Errorf("校准失准履历无效")
		}
		invalidated[item.CalibrationRef] = true
	}
	if b.Calibration != nil && invalidated[b.Calibration.Reference] {
		return fmt.Errorf("当前校准已登记失准")
	}
	return nil
}

func (b *QualificationBatch) validateReviewIntegrity() error {
	validate := func(c *ReviewChecklist) error {
		if c == nil {
			return nil
		}
		if c.ChecklistID == "" || c.SnapshotDigest == "" || len(c.Items) == 0 {
			return fmt.Errorf("复核清单核心字段不完整")
		}
		items := map[string]bool{}
		for _, item := range c.Items {
			if item.ItemID == "" || item.Title == "" || items[item.ItemID] {
				return fmt.Errorf("复核清单项无效")
			}
			items[item.ItemID] = true
		}
		for _, result := range c.Results {
			if !items[result.ItemID] || result.Comment == "" || result.ReviewedAt.IsZero() {
				return fmt.Errorf("复核清单结果无效")
			}
		}
		return nil
	}
	if err := validate(b.ReviewChecklist); err != nil {
		return err
	}
	for i := range b.ReviewHistory {
		if err := validate(&b.ReviewHistory[i].Checklist); err != nil {
			return fmt.Errorf("历史复核清单无效: %w", err)
		}
		seenCompletions := map[string]bool{}
		for _, completion := range b.ReviewHistory[i].RequirementCompletions {
			if completion.RequirementID == "" || completion.CompletedBy == "" || completion.CompletedAt.IsZero() || len(completion.CompletedRoundIDs) == 0 || seenCompletions[completion.RequirementID] {
				return fmt.Errorf("历史复核要求完成记录无效")
			}
			seenCompletions[completion.RequirementID] = true
		}
	}
	seenRequirements := map[string]bool{}
	for _, requirement := range b.ReviewRequirements {
		if requirement.RequirementID == "" || seenRequirements[requirement.RequirementID] || requirement.ChecklistID == "" || requirement.SnapshotDigest == "" || requirement.ReviewItemID == "" || requirement.SourceChecklistItem == "" || requirement.TapID == "" || requirement.RequirementText == "" || requirement.CreatedBy == "" || requirement.CreatedAt.IsZero() {
			return fmt.Errorf("复核要求核心字段不完整或标识重复")
		}
		seenRequirements[requirement.RequirementID] = true
		if b.Taps[requirement.TapID] == nil || len(requirement.RelatedDefectIDs) == 0 {
			return fmt.Errorf("复核要求 %s 的测孔或关联缺陷为空", requirement.RequirementID)
		}
		for _, defectID := range requirement.RelatedDefectIDs {
			if b.Defects[defectID] == nil || b.Defects[defectID].TapID != requirement.TapID {
				return fmt.Errorf("复核要求 %s 的关联缺陷无效", requirement.RequirementID)
			}
		}
		switch requirement.Status {
		case ReviewRequirementOpen:
			if requirement.CompletedAt != nil || len(requirement.CompletedRoundIDs) > 0 {
				return fmt.Errorf("开放复核要求 %s 包含完成事实", requirement.RequirementID)
			}
		case ReviewRequirementCompleted:
			if requirement.CompletedAt == nil || requirement.CompletedBy == "" || requirement.TreatmentVersionID == "" || !ValidSHA256(requirement.EvidenceDigest) || len(requirement.CompletedRoundIDs) < b.ThresholdProfile.RequiredConsecutivePasses {
				return fmt.Errorf("已完成复核要求 %s 的处置或复测事实不完整", requirement.RequirementID)
			}
		default:
			return fmt.Errorf("复核要求 %s 状态无效", requirement.RequirementID)
		}
	}
	return nil
}

func (b *QualificationBatch) validateRoundIntegrity() error {
	seen := map[string]bool{}
	latest := map[string]string{}
	for _, round := range b.Rounds {
		if round == nil || round.RoundID == "" || seen[round.RoundID] {
			return fmt.Errorf("存在空或重复测量轮次")
		}
		seen[round.RoundID] = true
		if round.BatchID != b.BatchID || b.Taps[round.TapID] == nil {
			return fmt.Errorf("测量轮次 %s 所属关系无效", round.RoundID)
		}
		if len(round.NeighborResponses) > 0 {
			normalized, maximum, worst, err := NormalizeNeighborResponses(b.Taps[round.TapID], round.NeighborResponses)
			if err != nil || len(normalized) != len(round.NeighborResponses) || maximum != round.NeighborResponsePA || worst != round.WorstNeighborTapID {
				return fmt.Errorf("测量轮次 %s 的逐通道响应无效", round.RoundID)
			}
		}
		if round.RoundKind == RoundRetest {
			defect := b.Defects[round.DefectID]
			if defect == nil || defect.TapID != round.TapID {
				return fmt.Errorf("复测轮次 %s 未关联同孔缺陷", round.RoundID)
			}
			if round.TreatmentVersionID == "" {
				return fmt.Errorf("复测轮次 %s 未关联处置版本", round.RoundID)
			}
		}
		if round.RoundKind == RoundInitial && round.SupportsQualification() {
			latest[round.TapID] = round.RoundID
		}
	}
	for tapID, tap := range b.Taps {
		if tap.LatestMeasurementRoundID != latest[tapID] {
			return fmt.Errorf("测孔 %s 的最新轮次索引不一致", tapID)
		}
	}
	return nil
}

func (b *QualificationBatch) validateDefectIntegrity() error {
	rounds := map[string]*MeasurementRound{}
	for _, round := range b.Rounds {
		rounds[round.RoundID] = round
	}
	for id, defect := range b.Defects {
		if defect == nil || defect.DefectID != id || defect.BatchID != b.BatchID || b.Taps[defect.TapID] == nil {
			return fmt.Errorf("缺陷 %s 标识或所属关系不一致", id)
		}
		if defect.SourceRoundID != "" {
			if source := rounds[defect.SourceRoundID]; source == nil || source.TapID != defect.TapID {
				return fmt.Errorf("缺陷 %s 的源轮次无效", id)
			}
		}
		for _, roundID := range defect.RetestRoundIDs {
			round := rounds[roundID]
			if round == nil || round.DefectID != id || round.RoundKind != RoundRetest {
				return fmt.Errorf("缺陷 %s 的复测轮次 %s 无效", id, roundID)
			}
		}
		for i, assignment := range defect.Assignments {
			if assignment.Version != uint64(i+1) || assignment.TechnicianID == "" || assignment.Reason == "" || assignment.ActorID == "" || assignment.DueAt.IsZero() || assignment.AssignedAt.IsZero() {
				return fmt.Errorf("缺陷 %s 的责任分派版本无效", id)
			}
		}
		versions := map[string]bool{}
		for i, version := range defect.TreatmentVersions {
			if version.VersionID == "" || versions[version.VersionID] || version.Sequence != i+1 {
				return fmt.Errorf("缺陷 %s 的处置版本无效", id)
			}
			versions[version.VersionID] = true
			if version.SourceRoundID != defect.SourceRoundID || !ValidSHA256(version.EvidenceDigest) {
				return fmt.Errorf("缺陷 %s 的处置证据或源轮次无效", id)
			}
		}
		for _, roundID := range defect.RetestRoundIDs {
			round := rounds[roundID]
			if !versions[round.TreatmentVersionID] {
				return fmt.Errorf("复测轮次 %s 的处置版本不存在", roundID)
			}
		}
		if defect.Status == DefectClosed && defect.ClosedAt == nil {
			return fmt.Errorf("已关闭缺陷 %s 缺少关闭时间", id)
		}
	}
	return nil
}

func (b *QualificationBatch) validateAuditIntegrity() error {
	previous := ""
	for i, event := range b.Audit {
		if event.Sequence != uint64(i+1) || event.PreviousDigest != previous || event.Digest == "" {
			return fmt.Errorf("审计事件序号或摘要关系不连续")
		}
		previous = event.Digest
	}
	return nil
}

func SortedTapIDs(taps map[string]*PressureTap) []string {
	ids := make([]string, 0, len(taps))
	for id := range taps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
