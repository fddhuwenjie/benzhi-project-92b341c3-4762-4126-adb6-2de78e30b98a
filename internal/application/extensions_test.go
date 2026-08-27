package application

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/evidence"
	"pressure-tap-qualification/internal/store"
)

func newExtensionService(t *testing.T) (*Service, time.Time) {
	t.Helper()
	repo, err := store.NewFileRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	return New(repo, evidence.New(repo)).WithClock(func() time.Time { return now }), now
}

func TestDraftRevisionDiffAndFreezeConfirmation(t *testing.T) {
	svc, _ := newExtensionService(t)
	created, err := svc.CreateBatch(CreateBatchCommand{CommandMeta: CommandMeta{RequestID: "create", ExpectedRevision: 0, ActorID: "owner"}, BatchID: "DRAFT-1", ModelCode: "M", TestObjective: "目标", Taps: []TapInput{{TapID: "T1", Label: "孔1", SurfaceZone: "A", NominalDiameterMM: 1, NeighborTapIDs: []string{"T2"}}, {TapID: "T2", Label: "孔2", SurfaceZone: "A", NominalDiameterMM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ReviseBaseline("DRAFT-1", ReviseBaselineCommand{CommandMeta: CommandMeta{RequestID: "bad", ExpectedRevision: created.Revision, ActorID: "owner"}, Reason: "误删", Taps: []TapInput{{TapID: "T1", Label: "孔1", SurfaceZone: "A", NominalDiameterMM: 1, NeighborTapIDs: []string{"T2"}}}})
	if err == nil {
		t.Fatal("删除仍被引用的测孔应失败")
	}
	view, _ := svc.GetBatch("DRAFT-1")
	if view.Batch.Revision != created.Revision || len(view.Batch.Taps) != 2 {
		t.Fatal("失败的草拟修订改变了批次")
	}
	revised, err := svc.ReviseBaseline("DRAFT-1", ReviseBaselineCommand{CommandMeta: CommandMeta{RequestID: "revise", ExpectedRevision: created.Revision, ActorID: "owner"}, Reason: "修正相邻关系并删除误录孔", Taps: []TapInput{{TapID: "T1", Label: "孔一", SurfaceZone: "A", NominalDiameterMM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	view, _ = svc.GetBatch("DRAFT-1")
	if len(view.DraftDiff.Deleted) != 1 || len(view.DraftDiff.Modified) != 1 {
		t.Fatalf("草拟差异不完整: %+v", view.DraftDiff)
	}
	_, err = svc.FreezeBaseline("DRAFT-1", FreezeCommand{CommandMeta: CommandMeta{RequestID: "stale", ExpectedRevision: created.Revision, ActorID: "owner"}, ConfirmedDiffSummary: created.DraftDiffSummary, ThresholdDigest: created.ThresholdDigest})
	if err == nil {
		t.Fatal("旧修订冻结应冲突")
	}
	_, err = svc.FreezeBaseline("DRAFT-1", FreezeCommand{CommandMeta: CommandMeta{RequestID: "wrong", ExpectedRevision: revised.Revision, ActorID: "owner"}, ConfirmedDiffSummary: created.DraftDiffSummary, ThresholdDigest: created.ThresholdDigest})
	if err == nil {
		t.Fatal("旧差异摘要冻结应冲突")
	}
	frozen, err := governedFreeze(t, svc, "DRAFT-1", CommandMeta{RequestID: "freeze", ExpectedRevision: revised.Revision, ActorID: "owner"}, view.DraftDiff.Summary, created.ThresholdDigest)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.State != domain.StateBaselineFrozen {
		t.Fatal("确认最新差异后未冻结")
	}
	_, err = svc.ReviseBaseline("DRAFT-1", ReviseBaselineCommand{CommandMeta: CommandMeta{RequestID: "after", ExpectedRevision: frozen.Revision, ActorID: "owner"}, Reason: "冻结后修改", Taps: []TapInput{{TapID: "T1", Label: "变化", SurfaceZone: "A", NominalDiameterMM: 1}}})
	if err == nil {
		t.Fatal("冻结后草拟修订应失败")
	}
}

func TestBatchMeasurementPreflightAtomicityAndReplay(t *testing.T) {
	svc, now := newExtensionService(t)
	created, err := svc.CreateBatch(CreateBatchCommand{CommandMeta: CommandMeta{RequestID: "c", ActorID: "owner"}, BatchID: "BULK-1", ModelCode: "M", TestObjective: "目标", Taps: []TapInput{{TapID: "T1", Label: "孔1", SurfaceZone: "A", NominalDiameterMM: 1}, {TapID: "T2", Label: "孔2", SurfaceZone: "A", NominalDiameterMM: 1}, {TapID: "T3", Label: "孔3", SurfaceZone: "B", NominalDiameterMM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := governedFreeze(t, svc, "BULK-1", CommandMeta{RequestID: "f", ExpectedRevision: created.Revision, ActorID: "owner"}, created.DraftDiffSummary, created.ThresholdDigest)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := svc.RegisterCalibration("BULK-1", CalibrationCommand{CommandMeta: CommandMeta{RequestID: "cal", ExpectedRevision: frozen.Revision, ActorID: "eng"}, Reference: "CAL-1", InstrumentSummary: "器具", ValidUntil: now.Add(12 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	rows := []MeasurementRow{{RoundID: "R1", TapID: "T1", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1}, {RoundID: "R2", TapID: "T2", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1}, {RoundID: "R3", TapID: "T3", SupplyPressurePA: 1000, SteadyPressurePA: 1100, DecaySeconds: 1, NeighborResponsePA: 1}}
	preview, err := svc.PreflightBatchMeasurement("BULK-1", BatchMeasurementCommand{CommandMeta: CommandMeta{RequestID: "pre", ExpectedRevision: cal.Revision, ActorID: "eng"}, CalibrationRef: "CAL-1", Rows: rows})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Rows) != 3 || preview.Rows[2].Result.Passed {
		t.Fatal("批量预检判定不正确")
	}
	bad := append([]MeasurementRow(nil), rows...)
	bad[2].RoundID = "R2"
	_, err = svc.RecordBatchMeasurements("BULK-1", BatchMeasurementCommand{CommandMeta: CommandMeta{RequestID: "bad", ExpectedRevision: cal.Revision, ActorID: "eng"}, CalibrationRef: "CAL-1", Rows: bad, Confirm: true})
	if err == nil {
		t.Fatal("重复轮次应使整批失败")
	}
	view, _ := svc.GetBatch("BULK-1")
	if len(view.Batch.Rounds) != 0 || view.Coverage != 0 {
		t.Fatal("失败批次产生了部分测量")
	}
	cmd := BatchMeasurementCommand{CommandMeta: CommandMeta{RequestID: "bulk", ExpectedRevision: cal.Revision, ActorID: "eng"}, CalibrationRef: "CAL-1", Rows: rows, Confirm: true}
	out, err := svc.RecordBatchMeasurements("BULK-1", cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.RecordBatchMeasurements("BULK-1", cmd)
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replayed || replay.Revision != out.Revision {
		t.Fatal("批量请求未重放原响应")
	}
	view, _ = svc.GetBatchFiltered("BULK-1", BatchFilter{SurfaceZone: "B", DefectType: domain.DefectLeak})
	if view.Coverage != 1 || view.FilterHitCount != 1 || view.TapMatrix[0].TapID != "T3" {
		t.Fatalf("区域缺陷钻取错误: %+v", view)
	}
	if len(view.ZoneSummaries) != 2 || view.ZoneSummaries[0].SurfaceZone != "B" {
		t.Fatalf("区域风险排序错误: %+v", view.ZoneSummaries)
	}
}

func TestStructuredReviewReturnKeepsHistory(t *testing.T) {
	svc, now := newExtensionService(t)
	rev := uint64(0)
	request := 0
	meta := func(actor string) CommandMeta {
		request++
		return CommandMeta{RequestID: fmt.Sprintf("q%d", request), ExpectedRevision: rev, ActorID: actor}
	}
	set := func(out CommandResponse, err error) CommandResponse {
		if err != nil {
			t.Fatal(err)
		}
		rev = out.Revision
		return out
	}
	created := set(svc.CreateBatch(CreateBatchCommand{CommandMeta: meta("owner"), BatchID: "REVIEW-1", ModelCode: "M", TestObjective: "目标", Taps: []TapInput{{TapID: "T1", Label: "孔1", SurfaceZone: "A", NominalDiameterMM: 1}, {TapID: "T2", Label: "孔2", SurfaceZone: "B", NominalDiameterMM: 1}}}))
	set(governedFreeze(t, svc, "REVIEW-1", meta("owner"), created.DraftDiffSummary, created.ThresholdDigest))
	set(svc.RegisterCalibration("REVIEW-1", CalibrationCommand{CommandMeta: meta("eng"), Reference: "CAL", InstrumentSummary: "器具", ValidUntil: now.Add(48 * time.Hour)}))
	set(svc.RecordBatchMeasurements("REVIEW-1", BatchMeasurementCommand{CommandMeta: meta("eng"), CalibrationRef: "CAL", Confirm: true, Rows: []MeasurementRow{{RoundID: "R1", TapID: "T1", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1}, {RoundID: "R2", TapID: "T2", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1}}}))
	set(svc.SubmitForReview("REVIEW-1", SubmitCommand{CommandMeta: meta("eng")}))
	view, err := svc.GetBatch("REVIEW-1")
	if err != nil {
		t.Fatal(err)
	}
	firstChecklist := view.Batch.ReviewChecklist.ChecklistID
	items := make([]domain.ReviewItemResult, 0, len(view.Batch.ReviewChecklist.Items))
	for _, item := range view.Batch.ReviewChecklist.Items {
		result := domain.ReviewItemResult{ItemID: item.ItemID, Status: domain.ReviewItemPassed, Comment: "确认"}
		if item.ItemID == "evidence" {
			result.Status = domain.ReviewItemReturned
			result.Comment = "补拍接口证据"
			result.ReturnTapIDs = []string{"T1", "T2"}
		}
		items = append(items, result)
	}
	set(svc.Review("REVIEW-1", ReviewCommand{CommandMeta: meta("reviewer"), Decision: domain.DecisionReturned, Note: "补充证据", Items: items}))
	returned, err := svc.GetBatch("REVIEW-1")
	if err != nil {
		t.Fatal(err)
	}
	if returned.Batch.State != domain.StateRemediation || len(returned.Batch.ReviewHistory) != 1 || returned.Batch.ReviewSnapshot != nil {
		t.Fatal("结构化退回状态或历史不正确")
	}
	count := 0
	for _, d := range returned.Batch.Defects {
		if strings.HasPrefix(d.DefectID, "REVIEW-") {
			count++
			if !strings.Contains(d.Cause, "evidence：补拍接口证据") {
				t.Fatalf("补充要求未绑定到测孔: %s", d.Cause)
			}
		}
	}
	if count != 2 {
		t.Fatalf("预期两个受控复测缺陷，得到 %d", count)
	}
	for _, d := range returned.Batch.Defects {
		if !strings.HasPrefix(d.DefectID, "REVIEW-") {
			continue
		}
		treatment := d.CurrentTreatment()
		for i := 0; i < 2; i++ {
			set(svc.RecordMeasurement("REVIEW-1", MeasurementCommand{CommandMeta: meta("eng"), RoundID: fmt.Sprintf("%s-RT%d", d.TapID, i), TapID: d.TapID, RoundKind: domain.RoundRetest, SourceRoundID: d.SourceRoundID, DefectID: d.DefectID, TreatmentVersionID: treatment.VersionID, CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1}))
		}
		set(svc.CloseDefect("REVIEW-1", CloseDefectCommand{CommandMeta: meta("eng"), DefectID: d.DefectID}))
	}
	set(svc.SubmitForReview("REVIEW-1", SubmitCommand{CommandMeta: meta("eng")}))
	resubmitted, err := svc.GetBatch("REVIEW-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(resubmitted.Batch.ReviewHistory) != 1 || resubmitted.Batch.ReviewChecklist.ChecklistID == firstChecklist {
		t.Fatal("再次送审未保留旧清单或未生成新清单")
	}
}
