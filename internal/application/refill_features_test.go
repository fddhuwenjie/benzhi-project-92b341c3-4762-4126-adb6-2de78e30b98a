package application

import (
	"testing"
	"time"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/evidence"
	"pressure-tap-qualification/internal/store"
)

func governedFreeze(t *testing.T, svc *Service, batchID string, meta CommandMeta, diff, threshold string) (CommandResponse, error) {
	t.Helper()
	topology, err := svc.PreflightTopology(batchID, meta.ExpectedRevision)
	if err != nil {
		return CommandResponse{}, err
	}
	acks := []string{}
	for _, issue := range topology.Issues {
		if issue.Severity == domain.TopologyWarning {
			acks = append(acks, issue.IssueID)
		}
	}
	return svc.FreezeBaseline(batchID, FreezeCommand{CommandMeta: meta, ConfirmedDiffSummary: diff, ThresholdDigest: threshold, TopologyDigest: topology.Digest, WarningAcknowledgements: acks})
}

func TestExpiredReviewSnapshotReturnsWithoutCertificate(t *testing.T) {
	repo, err := store.NewFileRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	svc := New(repo, evidence.New(repo)).WithClock(func() time.Time { return now })
	created, err := svc.CreateBatch(CreateBatchCommand{CommandMeta: CommandMeta{RequestID: "c", ActorID: "owner"}, BatchID: "EXP-1", ModelCode: "M", TestObjective: "目标", Taps: []TapInput{{TapID: "T", Label: "孔", SurfaceZone: "Z", NominalDiameterMM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := governedFreeze(t, svc, "EXP-1", CommandMeta{RequestID: "f", ExpectedRevision: created.Revision, ActorID: "owner"}, created.DraftDiffSummary, created.ThresholdDigest)
	if err != nil {
		t.Fatal(err)
	}
	calibrated, err := svc.RegisterCalibration("EXP-1", CalibrationCommand{CommandMeta: CommandMeta{RequestID: "cal", ExpectedRevision: frozen.Revision, ActorID: "eng"}, Reference: "CAL", InstrumentSummary: "器具", ValidUntil: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	measured, err := svc.RecordMeasurement("EXP-1", MeasurementCommand{CommandMeta: CommandMeta{RequestID: "m", ExpectedRevision: calibrated.Revision, ActorID: "eng"}, RoundID: "R", TapID: "T", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := svc.SubmitForReview("EXP-1", SubmitCommand{CommandMeta: CommandMeta{RequestID: "s", ExpectedRevision: measured.Revision, ActorID: "eng"}})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	view, err := svc.GetBatch("EXP-1")
	if err != nil {
		t.Fatal(err)
	}
	items := make([]domain.ReviewItemResult, 0, len(view.Batch.ReviewChecklist.Items))
	for _, item := range view.Batch.ReviewChecklist.Items {
		items = append(items, domain.ReviewItemResult{ItemID: item.ItemID, Status: domain.ReviewItemPassed, Comment: "确认"})
	}
	result, err := svc.Review("EXP-1", ReviewCommand{CommandMeta: CommandMeta{RequestID: "a", ExpectedRevision: submitted.Revision, ActorID: "reviewer"}, Decision: domain.DecisionApproved, Note: "批准", Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != domain.StateRemediation || result.Certificate != nil {
		t.Fatalf("过期快照不应签发证书: %+v", result)
	}
	if _, err = repo.LoadCertificate("EXP-1"); domain.ErrorCodeOf(err) != domain.CodeNotFound {
		t.Fatal("过期快照产生了证书")
	}
}

func TestThresholdRevisionTopologyAndFrozenVersion(t *testing.T) {
	svc, _ := newExtensionService(t)
	created, err := svc.CreateBatch(CreateBatchCommand{
		CommandMeta: CommandMeta{RequestID: "c", ActorID: "owner"}, BatchID: "GOV-1", ModelCode: "M", TestObjective: "目标",
		Taps: []TapInput{
			{TapID: "A", Label: "甲", SurfaceZone: "Z1", NominalDiameterMM: 1, NeighborTapIDs: []string{"B"}},
			{TapID: "B", Label: "乙", SurfaceZone: "Z2", NominalDiameterMM: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pre, err := svc.PreflightThresholdRevision("GOV-1", ReviseThresholdCommand{
		CommandMeta:      CommandMeta{RequestID: "tp", ExpectedRevision: created.Revision, ActorID: "owner"},
		ThresholdProfile: domain.ThresholdProfile{MinimumPressureRatio: .9, MaximumPressureRatio: 1.05, MaximumDecaySeconds: 2.5, MaximumNeighborRatio: .05, RequiredConsecutivePasses: 2},
		Reason:           "缩短最大衰减时间",
	})
	if err != nil {
		t.Fatal(err)
	}
	if pre.Changes[2].Direction != domain.ThresholdStricter {
		t.Fatalf("最大衰减时间应判定为变严: %+v", pre.Changes[2])
	}
	revised, err := svc.ReviseThresholds("GOV-1", ReviseThresholdCommand{CommandMeta: CommandMeta{RequestID: "tr", ExpectedRevision: created.Revision, ActorID: "owner"}, ThresholdProfile: pre.Profile, Reason: pre.Reason, ConfirmedSummary: pre.Digest})
	if err != nil {
		t.Fatal(err)
	}
	topology, err := svc.PreflightTopology("GOV-1", revised.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if topology.ErrorCount != 1 {
		t.Fatalf("应识别单向邻接: %+v", topology)
	}
	if _, err = svc.FreezeBaseline("GOV-1", FreezeCommand{CommandMeta: CommandMeta{RequestID: "f0", ExpectedRevision: revised.Revision, ActorID: "owner"}, ConfirmedDiffSummary: created.DraftDiffSummary, ThresholdDigest: pre.Digest}); err == nil {
		t.Fatal("单向邻接不应允许冻结")
	}
	fixed, err := svc.ReviseBaseline("GOV-1", ReviseBaselineCommand{
		CommandMeta: CommandMeta{RequestID: "fix", ExpectedRevision: revised.Revision, ActorID: "owner"}, Reason: "补齐反向关系",
		Taps: []TapInput{
			{TapID: "A", Label: "甲", SurfaceZone: "Z1", NominalDiameterMM: 1, NeighborTapIDs: []string{"B"}},
			{TapID: "B", Label: "乙", SurfaceZone: "Z2", NominalDiameterMM: 1, NeighborTapIDs: []string{"A"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	topology, err = svc.PreflightTopology("GOV-1", fixed.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if topology.ErrorCount != 0 || topology.WarningCount != 1 {
		t.Fatalf("跨区应为一项警告: %+v", topology)
	}
	frozen, err := svc.FreezeBaseline("GOV-1", FreezeCommand{
		CommandMeta: CommandMeta{RequestID: "f", ExpectedRevision: fixed.Revision, ActorID: "owner"}, ConfirmedDiffSummary: fixed.DraftDiffSummary,
		ThresholdDigest: pre.Digest, TopologyDigest: topology.Digest, WarningAcknowledgements: []string{topology.Issues[0].IssueID},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetBatch("GOV-1")
	if view.Batch.FrozenThresholdVersion != 2 || view.Batch.FrozenTopologyDigest == "" || frozen.State != domain.StateBaselineFrozen {
		t.Fatal("冻结未锁定阈值版本和拓扑")
	}
}

func TestCalibrationInvalidationRecalculatesQualification(t *testing.T) {
	svc, now := newExtensionService(t)
	created, err := svc.CreateBatch(CreateBatchCommand{CommandMeta: CommandMeta{RequestID: "c", ActorID: "owner"}, BatchID: "CORR-1", ModelCode: "M", TestObjective: "目标", Taps: []TapInput{{TapID: "T", Label: "孔", SurfaceZone: "Z", NominalDiameterMM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := governedFreeze(t, svc, "CORR-1", CommandMeta{RequestID: "f", ExpectedRevision: created.Revision, ActorID: "owner"}, created.DraftDiffSummary, created.ThresholdDigest)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := svc.RegisterCalibration("CORR-1", CalibrationCommand{CommandMeta: CommandMeta{RequestID: "cal", ExpectedRevision: frozen.Revision, ActorID: "eng"}, Reference: "CAL", InstrumentSummary: "器具", ValidUntil: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	measured, err := svc.RecordMeasurement("CORR-1", MeasurementCommand{CommandMeta: CommandMeta{RequestID: "m", ExpectedRevision: cal.Revision, ActorID: "eng"}, RoundID: "R", TapID: "T", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1})
	if err != nil {
		t.Fatal(err)
	}
	invalid, err := svc.InvalidateCalibration("CORR-1", CalibrationInvalidationCommand{CommandMeta: CommandMeta{RequestID: "i", ExpectedRevision: measured.Revision, ActorID: "qa"}, CalibrationRef: "CAL", DiscoveredAt: now, Reason: "漂移超差", EvidenceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid.AffectedRoundIDs) != 1 {
		t.Fatal("失准未隔离引用轮次")
	}
	view, _ := svc.GetBatch("CORR-1")
	if view.Batch.Rounds[0].SupplyPressurePA != 1000 || !view.Batch.Rounds[0].CalibrationInvalid || view.Batch.Taps["T"].QualificationStatus != domain.TapPending {
		t.Fatal("失准后原值保留或资格回算错误")
	}
}

func TestBatchTreatmentAndRetestAtomicFlow(t *testing.T) {
	svc, now := newExtensionService(t)
	created, err := svc.CreateBatch(CreateBatchCommand{CommandMeta: CommandMeta{RequestID: "c", ActorID: "owner"}, BatchID: "ATOM-1", ModelCode: "M", TestObjective: "目标", Taps: []TapInput{{TapID: "T", Label: "孔", SurfaceZone: "Z", NominalDiameterMM: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := governedFreeze(t, svc, "ATOM-1", CommandMeta{RequestID: "f", ExpectedRevision: created.Revision, ActorID: "owner"}, created.DraftDiffSummary, created.ThresholdDigest)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := svc.RegisterCalibration("ATOM-1", CalibrationCommand{CommandMeta: CommandMeta{RequestID: "cal", ExpectedRevision: frozen.Revision, ActorID: "eng"}, Reference: "CAL", InstrumentSummary: "器具", ValidUntil: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	measured, err := svc.RecordMeasurement("ATOM-1", MeasurementCommand{CommandMeta: CommandMeta{RequestID: "m", ExpectedRevision: cal.Revision, ActorID: "eng"}, RoundID: "R", TapID: "T", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 1100, DecaySeconds: 4, NeighborResponsePA: 1})
	if err != nil {
		t.Fatal(err)
	}
	items := make([]BatchTreatmentItem, 0, len(measured.CreatedDefectIDs))
	for _, id := range measured.CreatedDefectIDs {
		items = append(items, BatchTreatmentItem{DefectID: id})
	}
	cmd := BatchTreatmentCommand{CommandMeta: CommandMeta{RequestID: "tp", ExpectedRevision: measured.Revision, ActorID: "tech"}, JobID: "JOB", Cause: "共同污染", CorrectiveAction: "清理管路", EvidenceDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Items: items}
	preview, err := svc.PreflightBatchTreatment("ATOM-1", cmd)
	if err != nil {
		t.Fatal(err)
	}
	cmd.RequestID, cmd.PreflightDigest, cmd.Confirm = "tc", preview.Digest, true
	treated, err := svc.TreatDefectsBatch("ATOM-1", cmd)
	if err != nil {
		t.Fatal(err)
	}
	view, _ := svc.GetBatch("ATOM-1")
	rows := []BatchRetestRow{}
	for _, id := range measured.CreatedDefectIDs {
		d := view.Batch.Defects[id]
		rows = append(rows, BatchRetestRow{RoundID: "RT-" + id, DefectID: id, TapID: "T", TreatmentVersionID: d.CurrentTreatment().VersionID, SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1})
	}
	retestCmd := BatchRetestCommand{CommandMeta: CommandMeta{RequestID: "rp", ExpectedRevision: treated.Revision, ActorID: "eng"}, CalibrationRef: "CAL", Rows: rows}
	retestPreview, err := svc.PreflightBatchRetest("ATOM-1", retestCmd)
	if err != nil {
		t.Fatal(err)
	}
	retestCmd.RequestID, retestCmd.PreflightDigest, retestCmd.Confirm = "rc", retestPreview.Digest, true
	if _, err = svc.RecordBatchRetests("ATOM-1", retestCmd); err != nil {
		t.Fatal(err)
	}
}
