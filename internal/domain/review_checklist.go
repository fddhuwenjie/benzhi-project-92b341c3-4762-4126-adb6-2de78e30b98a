package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func NewReviewChecklist(batchID, snapshotDigest string, revision uint64) *ReviewChecklist {
	return &ReviewChecklist{ChecklistID: fmt.Sprintf("CHECK-%s-%d", batchID, revision), SnapshotDigest: snapshotDigest, Items: []ReviewItem{
		{ItemID: "baseline", Title: "冻结基线与测孔关系完整"},
		{ItemID: "calibration", Title: "校准引用及有效期符合要求"},
		{ItemID: "coverage", Title: "测孔覆盖率与判定完整"},
		{ItemID: "defects", Title: "缺陷均已完成合格复测并关闭"},
		{ItemID: "evidence", Title: "返修证据与审计留痕完整"},
	}}
}

func (b *QualificationBatch) CompleteReview(reviewer string, decision ReviewDecision, note string, results []ReviewItemResult, now time.Time) ([]string, error) {
	if b.State != StateUnderReview || b.ReviewSnapshot == nil || b.ReviewChecklist == nil {
		return nil, NewError(CodeState, "批次没有可处理的结构化复核清单")
	}
	if err := b.CheckReviewer(reviewer); err != nil {
		return nil, err
	}
	if strings.TrimSpace(note) == "" {
		return nil, NewError(CodeInvalid, "复核意见不能为空")
	}
	if b.ReviewChecklist.SnapshotDigest != b.ReviewSnapshot.Digest {
		return nil, NewError(CodeConflict, "送审快照摘要已变化，请重新送审")
	}
	byID := map[string]ReviewItemResult{}
	for _, result := range results {
		if _, exists := byID[result.ItemID]; exists {
			return nil, NewError(CodeInvalid, "复核项 %s 重复", result.ItemID)
		}
		if result.Status != ReviewItemPassed && result.Status != ReviewItemReturned {
			return nil, NewError(CodeInvalid, "复核项 %s 的结论无效", result.ItemID)
		}
		if strings.TrimSpace(result.Comment) == "" {
			return nil, NewError(CodeInvalid, "复核项 %s 必须填写意见", result.ItemID)
		}
		for _, tapID := range result.ReturnTapIDs {
			if b.Taps[tapID] == nil {
				return nil, NewError(CodeInvalid, "复核退回测孔 %s 不在送审快照中", tapID)
			}
		}
		if result.Status == ReviewItemReturned && len(result.ReturnTapIDs) == 0 {
			return nil, NewError(CodeInvalid, "退回复核项 %s 必须选择测孔", result.ItemID)
		}
		result.ReviewedAt = now.UTC()
		byID[result.ItemID] = result
	}
	ordered := make([]ReviewItemResult, 0, len(b.ReviewChecklist.Items))
	returned := []string{}
	seenTap := map[string]bool{}
	for _, item := range b.ReviewChecklist.Items {
		result, ok := byID[item.ItemID]
		if !ok {
			return nil, NewError(CodeInvalid, "复核项 %s 尚未填写", item.ItemID)
		}
		ordered = append(ordered, result)
		if result.Status == ReviewItemReturned {
			for _, id := range result.ReturnTapIDs {
				if !seenTap[id] {
					returned = append(returned, id)
					seenTap[id] = true
				}
			}
		}
	}
	if decision == DecisionApproved && len(returned) > 0 {
		return nil, NewError(CodeUnqualified, "存在退回复核项，不能批准")
	}
	if decision == DecisionReturned && len(returned) == 0 {
		return nil, NewError(CodeInvalid, "退回决定至少需要一个退回复核项")
	}
	b.ReviewChecklist.Results = ordered
	b.ReviewChecklist.ReviewerID = reviewer
	b.ReviewChecklist.Decision = decision
	completed := now.UTC()
	b.ReviewChecklist.CompletedAt = &completed
	return returned, nil
}

func (b *QualificationBatch) ArchiveReview(note string) {
	if b.ReviewChecklist == nil {
		return
	}
	var snapshot *ReviewSnapshot
	if b.ReviewSnapshot != nil {
		copy := *b.ReviewSnapshot
		snapshot = &copy
	}
	facts := b.CurrentReviewFacts()
	if b.ReviewSnapshot != nil {
		facts = b.ReviewSnapshot.Facts
	}
	requirementIDs := []string{}
	for _, requirement := range b.ReviewRequirements {
		if requirement.ChecklistID == b.ReviewChecklist.ChecklistID {
			requirementIDs = append(requirementIDs, requirement.RequirementID)
		}
	}
	sort.Strings(requirementIDs)
	b.ReviewHistory = append(b.ReviewHistory, ReviewRecord{Checklist: *b.ReviewChecklist, Note: note, Snapshot: snapshot, Facts: facts, RequirementIDs: requirementIDs})
}

func (b *QualificationBatch) CurrentReviewFacts() ReviewSnapshotFacts {
	facts := ReviewSnapshotFacts{EffectiveRoundIDs: b.EffectiveRoundIDs(), VoidedRoundIDs: []string{}, TreatmentVersionIDs: []string{}, DefectStatuses: map[string]string{}, CalibrationRefs: []string{}}
	calibrations := map[string]bool{}
	for _, round := range b.Rounds {
		if round.Voided != nil {
			facts.VoidedRoundIDs = append(facts.VoidedRoundIDs, round.RoundID)
		}
		if round.SupportsQualification() {
			calibrations[round.CalibrationRef] = true
		}
	}
	for id, defect := range b.Defects {
		facts.DefectStatuses[id] = string(defect.Status)
		for _, version := range defect.TreatmentVersions {
			facts.TreatmentVersionIDs = append(facts.TreatmentVersionIDs, version.VersionID)
		}
	}
	for reference := range calibrations {
		facts.CalibrationRefs = append(facts.CalibrationRefs, reference)
	}
	sort.Strings(facts.VoidedRoundIDs)
	sort.Strings(facts.TreatmentVersionIDs)
	sort.Strings(facts.CalibrationRefs)
	if b.ReviewSnapshot != nil {
		facts.QualificationSummary = b.ReviewSnapshot.QualificationSummary
	}
	return facts
}

func (c *ReviewChecklist) ReturnRequirements() map[string]string {
	byTap := map[string][]string{}
	for _, result := range c.Results {
		if result.Status != ReviewItemReturned {
			continue
		}
		for _, tapID := range result.ReturnTapIDs {
			byTap[tapID] = append(byTap[tapID], result.ItemID+"："+result.Comment)
		}
	}
	out := map[string]string{}
	for tapID, requirements := range byTap {
		out[tapID] = strings.Join(requirements, "；")
	}
	return out
}
