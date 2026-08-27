package application

import (
	"fmt"
	"testing"
	"time"

	"pressure-tap-qualification/internal/domain"
)

func TestMeasurementQualitySnapshotFiltersInvalidRoundsAndIsReadOnly(t *testing.T) {
	svc, now := newExtensionService(t)
	rev := uint64(0)
	next := func(id, actor string) CommandMeta {
		return CommandMeta{RequestID: id, ExpectedRevision: rev, ActorID: actor}
	}
	set := func(out CommandResponse, err error) CommandResponse {
		if err != nil {
			t.Fatal(err)
		}
		rev = out.Revision
		return out
	}
	created := set(svc.CreateBatch(CreateBatchCommand{CommandMeta: next("qs-create", "owner"), BatchID: "QS-1", ModelCode: "M", TestObjective: "统计", Taps: []TapInput{{TapID: "A", Label: "甲", SurfaceZone: "Z1", NominalDiameterMM: 1}, {TapID: "B", Label: "乙", SurfaceZone: "Z2", NominalDiameterMM: 1}}}))
	set(governedFreeze(t, svc, "QS-1", next("qs-freeze", "owner"), created.DraftDiffSummary, created.ThresholdDigest))
	set(svc.RegisterCalibration("QS-1", CalibrationCommand{CommandMeta: next("qs-old-cal", "engineer"), Reference: "OLD", InstrumentSummary: "旧器具", ValidUntil: now.Add(48 * time.Hour)}))
	set(svc.RecordMeasurement("QS-1", MeasurementCommand{CommandMeta: next("qs-old-round", "engineer"), RoundID: "OLD-A", TapID: "A", CalibrationRef: "OLD", SupplyPressurePA: 1000, SteadyPressurePA: 970, DecaySeconds: 1, NeighborResponses: []domain.NeighborResponse{}}))
	set(svc.RegisterCalibration("QS-1", CalibrationCommand{CommandMeta: next("qs-new-cal", "engineer"), Reference: "NEW", InstrumentSummary: "新器具", ValidUntil: now.Add(12 * time.Hour)}))
	set(svc.RecordMeasurement("QS-1", MeasurementCommand{CommandMeta: next("qs-a", "engineer"), RoundID: "NEW-A", TapID: "A", CalibrationRef: "NEW", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponses: []domain.NeighborResponse{}}))
	set(svc.RecordMeasurement("QS-1", MeasurementCommand{CommandMeta: next("qs-b", "engineer"), RoundID: "NEW-B", TapID: "B", CalibrationRef: "NEW", SupplyPressurePA: 1000, SteadyPressurePA: 960, DecaySeconds: 2, NeighborResponses: []domain.NeighborResponse{}}))
	set(svc.VoidMeasurementRound("QS-1", VoidRoundCommand{CommandMeta: next("qs-void", "owner"), RoundID: "NEW-A", Reason: "错录"}))
	set(svc.InvalidateCalibration("QS-1", CalibrationInvalidationCommand{CommandMeta: next("qs-invalidate", "owner"), CalibrationRef: "OLD", DiscoveredAt: now, Reason: "失准", EvidenceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}))
	before, _ := svc.GetBatch("QS-1")
	first, err := svc.QueryMeasurementQuality("QS-1", MeasurementQualityFilter{SurfaceZone: "Z2", RoundKind: domain.RoundInitial})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.QueryMeasurementQuality("QS-1", MeasurementQualityFilter{SurfaceZone: "Z2", RoundKind: domain.RoundInitial})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := svc.GetBatch("QS-1")
	if len(first.Rows) != 1 || first.Rows[0].TapID != "B" || first.Rows[0].ValidRoundCount != 1 || first.Rows[0].PassRate != 1 || first.SummaryDigest != second.SummaryDigest {
		t.Fatalf("统计快照不正确或不稳定: %+v", first)
	}
	if before.Batch.Revision != after.Batch.Revision || len(before.Batch.Rounds) != len(after.Batch.Rounds) {
		t.Fatal("只读统计改变了批次")
	}
	if _, err = svc.QueryMeasurementQuality("QS-1", MeasurementQualityFilter{Level: "unknown"}); domain.ErrorCodeOf(err) != domain.CodeInvalid || domain.FieldErrorsOf(err)["level"] == "" {
		t.Fatalf("无效风险等级未返回字段错误: %v", err)
	}
}

func TestRevisionHistoryDifferenceAndFrozenDigestAreReadOnly(t *testing.T) {
	svc, _ := newExtensionService(t)
	created, err := svc.CreateBatch(CreateBatchCommand{CommandMeta: CommandMeta{RequestID: "h-create", ActorID: "owner"}, BatchID: "H-1", ModelCode: "M1", TestObjective: "历史", Taps: []TapInput{{TapID: "A", Label: "甲", SurfaceZone: "Z", NominalDiameterMM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	revised, err := svc.ReviseBaseline("H-1", ReviseBaselineCommand{CommandMeta: CommandMeta{RequestID: "h-revise", ExpectedRevision: created.Revision, ActorID: "owner"}, Reason: "增加测孔并调整孔径", Taps: []TapInput{{TapID: "A", Label: "甲", SurfaceZone: "Z", NominalDiameterMM: 1.5, NeighborTapIDs: []string{"B"}}, {TapID: "B", Label: "乙", SurfaceZone: "Z", NominalDiameterMM: 1, NeighborTapIDs: []string{"A"}}}})
	if err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetBatch("H-1")
	frozen, err := governedFreeze(t, svc, "H-1", CommandMeta{RequestID: "h-freeze", ExpectedRevision: revised.Revision, ActorID: "owner"}, view.DraftDiff.Summary, revised.ThresholdDigest)
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.QueryRevisionHistory("H-1", RevisionHistoryFilter{HistoryType: domain.HistoryBaseline, FromVersion: 1, ToVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !history.SequenceValid || !history.FrozenDigestValid || history.FrozenVersion != 2 || history.Difference == nil || len(history.Difference.Changes) < 2 {
		t.Fatalf("修订差异或冻结校验不完整: %+v", history)
	}
	after, _ := svc.GetBatch("H-1")
	if after.Batch.Revision != frozen.Revision || len(after.Batch.Taps) != 2 {
		t.Fatal("历史查询改变了批次")
	}
	if _, err = svc.QueryRevisionHistory("H-1", RevisionHistoryFilter{HistoryType: domain.HistoryBaseline, FromVersion: 1, ToVersion: 99}); domain.ErrorCodeOf(err) != domain.CodeNotFound || domain.FieldErrorsOf(err)["to_version"] == "" {
		t.Fatalf("不存在版本未返回定位错误: %v", err)
	}
}

func TestReviewRequirementNeedsMappedEvidenceAndConsecutiveRetests(t *testing.T) {
	svc, now := newExtensionService(t)
	rev, request := uint64(0), 0
	meta := func(actor string) CommandMeta {
		request++
		return CommandMeta{RequestID: fmt.Sprintf("rr-%d", request), ExpectedRevision: rev, ActorID: actor}
	}
	set := func(out CommandResponse, err error) CommandResponse {
		if err != nil {
			t.Fatal(err)
		}
		rev = out.Revision
		return out
	}
	created := set(svc.CreateBatch(CreateBatchCommand{CommandMeta: meta("owner"), BatchID: "RR-1", ModelCode: "M", TestObjective: "要求", Taps: []TapInput{{TapID: "A", Label: "甲", SurfaceZone: "Z", NominalDiameterMM: 1}}}))
	set(governedFreeze(t, svc, "RR-1", meta("owner"), created.DraftDiffSummary, created.ThresholdDigest))
	set(svc.RegisterCalibration("RR-1", CalibrationCommand{CommandMeta: meta("engineer"), Reference: "CAL", InstrumentSummary: "器具", ValidUntil: now.Add(48 * time.Hour)}))
	set(svc.RecordMeasurement("RR-1", MeasurementCommand{CommandMeta: meta("engineer"), RoundID: "R1", TapID: "A", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponses: []domain.NeighborResponse{}}))
	set(svc.SubmitForReview("RR-1", SubmitCommand{CommandMeta: meta("engineer")}))
	view, _ := svc.GetBatch("RR-1")
	items := make([]domain.ReviewItemResult, 0, len(view.Batch.ReviewChecklist.Items))
	for _, item := range view.Batch.ReviewChecklist.Items {
		result := domain.ReviewItemResult{ItemID: item.ItemID, Status: domain.ReviewItemPassed, Comment: "确认"}
		if item.ItemID == "evidence" {
			result.Status = domain.ReviewItemReturned
			result.Comment = "检查串扰证据"
			result.ReturnTapIDs = []string{"A"}
		}
		items = append(items, result)
	}
	set(svc.Review("RR-1", ReviewCommand{CommandMeta: meta("reviewer"), Decision: domain.DecisionReturned, Note: "补充检查", Items: items}))
	returned, _ := svc.GetBatch("RR-1")
	requirement := returned.Batch.ReviewRequirements[0]
	defect := returned.Batch.Defects[requirement.RelatedDefectIDs[0]]
	autoTreatment := defect.CurrentTreatment()
	set(svc.RecordMeasurement("RR-1", MeasurementCommand{CommandMeta: meta("engineer"), RoundID: "UNMAPPED", TapID: "A", RoundKind: domain.RoundRetest, DefectID: defect.DefectID, TreatmentVersionID: autoTreatment.VersionID, CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponses: []domain.NeighborResponse{}}))
	blocked, err := svc.PreflightSubmission("RR-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked.UncompletedRequirements) != 1 || blocked.RequirementDigest == "" {
		t.Fatalf("未映射要求未阻塞送审: %+v", blocked)
	}
	set(svc.TreatDefect("RR-1", TreatmentCommand{CommandMeta: meta("tech"), DefectID: defect.DefectID, VersionID: "TECH-TREAT", SourceRoundID: defect.SourceRoundID, Cause: "复核补充检查", CorrectiveAction: "检查并补充证据", EvidenceDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ReviewRequirementIDs: []string{requirement.RequirementID}}))
	for i := 1; i <= 2; i++ {
		set(svc.RecordMeasurement("RR-1", MeasurementCommand{CommandMeta: meta("engineer"), RoundID: fmt.Sprintf("MAPPED-%d", i), TapID: "A", RoundKind: domain.RoundRetest, DefectID: defect.DefectID, TreatmentVersionID: "TECH-TREAT", ReviewRequirementIDs: []string{requirement.RequirementID}, CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponses: []domain.NeighborResponse{}}))
	}
	completed, err := svc.PreflightSubmission("RR-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(completed.UncompletedRequirements) != 0 {
		t.Fatalf("有效证据和连续复测后要求仍未完成: %+v", completed.UncompletedRequirements)
	}
	finalView, _ := svc.GetBatch("RR-1")
	if finalView.Batch.ReviewRequirements[0].Status != domain.ReviewRequirementCompleted || len(finalView.Batch.ReviewHistory) != 1 || len(finalView.Batch.ReviewHistory[0].RequirementCompletions) != 1 || len(finalView.Batch.ReviewHistory[0].RequirementCompletions[0].CompletedRoundIDs) != 2 {
		t.Fatal("要求完成状态或历史退回记录未保留")
	}
}

func TestSubmissionPreflightIsReadOnlyAndReviewerEvidence(t *testing.T) {
	svc, now := newExtensionService(t)
	created, err := svc.CreateBatch(CreateBatchCommand{CommandMeta: CommandMeta{RequestID: "create-v40", ActorID: "owner"}, BatchID: "V40-PRE", ModelCode: "M", TestObjective: "目标", Taps: []TapInput{{TapID: "A", Label: "甲", SurfaceZone: "Z", NominalDiameterMM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := governedFreeze(t, svc, "V40-PRE", CommandMeta{RequestID: "freeze-v40", ExpectedRevision: created.Revision, ActorID: "owner"}, created.DraftDiffSummary, created.ThresholdDigest)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := svc.GetBatch("V40-PRE")
	preflight, err := svc.PreflightSubmission("V40-PRE")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := svc.GetBatch("V40-PRE")
	if preflight.Ready || len(preflight.Blockers) < 2 || preflight.FactDigest == "" {
		t.Fatalf("预检阻塞不完整: %+v", preflight)
	}
	if before.Batch.Revision != after.Batch.Revision || len(before.Batch.Audit) != len(after.Batch.Audit) {
		t.Fatal("只读预检修改了批次")
	}
	cal, err := svc.RegisterCalibration("V40-PRE", CalibrationCommand{CommandMeta: CommandMeta{RequestID: "cal-v40", ExpectedRevision: frozen.Revision, ActorID: "engineer"}, Reference: "CAL", InstrumentSummary: "器具", ValidUntil: now.Add(48 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	measured, err := svc.RecordMeasurement("V40-PRE", MeasurementCommand{CommandMeta: CommandMeta{RequestID: "measure-v40", ExpectedRevision: cal.Revision, ActorID: "engineer"}, RoundID: "R1", TapID: "A", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponses: []domain.NeighborResponse{}})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := svc.PreflightSubmission("V40-PRE")
	if err != nil || !ready.Ready {
		t.Fatalf("预检应可送审: %+v %v", ready, err)
	}
	submitted, err := svc.SubmitForReview("V40-PRE", SubmitCommand{CommandMeta: CommandMeta{RequestID: "submit-v40", ExpectedRevision: measured.Revision, ActorID: "engineer"}, PreflightDigest: ready.FactDigest})
	if err != nil {
		t.Fatal(err)
	}
	conflict, err := svc.PreflightReviewer("V40-PRE", "engineer")
	if err != nil || conflict.CanApprove || len(conflict.Conflicts) == 0 || conflict.Conflicts[0].EntityID != "R1" {
		t.Fatalf("测量参与依据错误: %+v %v", conflict, err)
	}
	independent, err := svc.PreflightReviewer("V40-PRE", "reviewer")
	if err != nil || !independent.CanApprove {
		t.Fatalf("独立复核员未通过: %+v %v", independent, err)
	}
	if submitted.State != domain.StateUnderReview {
		t.Fatal("未进入待复核")
	}
}
