package application

import (
	"fmt"
	"testing"
	"time"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/evidence"
	"pressure-tap-qualification/internal/store"
)

func TestCompleteWorkflowAndCertificate(t *testing.T) {
	repo, err := store.NewFileRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, evidence.New(repo))
	revision := uint64(0)
	seq := 0
	meta := func(actor string) CommandMeta {
		seq++
		return CommandMeta{RequestID: fmt.Sprintf("R%d", seq), ExpectedRevision: revision, ActorID: actor}
	}
	set := func(out CommandResponse, err error) CommandResponse {
		if err != nil {
			t.Fatal(err)
		}
		revision = out.Revision
		return out
	}
	created := set(svc.CreateBatch(CreateBatchCommand{CommandMeta: meta("owner"), BatchID: "B1", ModelCode: "M1", TestObjective: "目标", Taps: []TapInput{{TapID: "T1", Label: "孔 1", SurfaceZone: "wing", NominalDiameterMM: 1}}}))
	set(governedFreeze(t, svc, "B1", meta("owner"), created.DraftDiffSummary, created.ThresholdDigest))
	set(svc.RegisterCalibration("B1", CalibrationCommand{CommandMeta: meta("engineer"), Reference: "CAL", InstrumentSummary: "器具", ValidUntil: time.Now().Add(time.Hour)}))
	failed := set(svc.RecordMeasurement("B1", MeasurementCommand{CommandMeta: meta("engineer"), RoundID: "M1", TapID: "T1", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 1100, DecaySeconds: 1, NeighborResponsePA: 1}))
	defect := failed.CreatedDefectIDs[0]
	set(svc.TreatDefect("B1", TreatmentCommand{CommandMeta: meta("tech"), DefectID: defect, Cause: "密封", CorrectiveAction: "重装", EvidenceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", VersionID: "TREAT-1", SourceRoundID: "M1"}))
	set(svc.RecordMeasurement("B1", MeasurementCommand{CommandMeta: meta("engineer"), RoundID: "RT-PASS-OLD", TapID: "T1", RoundKind: domain.RoundRetest, SourceRoundID: "M1", DefectID: defect, TreatmentVersionID: "TREAT-1", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1}))
	set(svc.RecordMeasurement("B1", MeasurementCommand{CommandMeta: meta("engineer"), RoundID: "RT-FAIL", TapID: "T1", RoundKind: domain.RoundRetest, SourceRoundID: "M1", DefectID: defect, TreatmentVersionID: "TREAT-1", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 1100, DecaySeconds: 1, NeighborResponsePA: 1}))
	failedProgress, err := svc.GetBatch("B1")
	if err != nil {
		t.Fatal(err)
	}
	if failedProgress.DefectViews[0].Progress.ConsecutivePassed != 0 || failedProgress.DefectViews[0].Progress.LatestFailedRoundID != "RT-FAIL" {
		t.Fatal("失败复测未重置连续进度")
	}
	set(svc.TreatDefect("B1", TreatmentCommand{CommandMeta: meta("tech"), DefectID: defect, Cause: "二次诊断", CorrectiveAction: "更换接头", EvidenceDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", VersionID: "TREAT-2", SourceRoundID: "M1"}))
	for i := 0; i < 2; i++ {
		set(svc.RecordMeasurement("B1", MeasurementCommand{CommandMeta: meta("engineer"), RoundID: fmt.Sprintf("RT%d", i), TapID: "T1", RoundKind: domain.RoundRetest, SourceRoundID: "M1", DefectID: defect, TreatmentVersionID: "TREAT-2", CalibrationRef: "CAL", SupplyPressurePA: 1000, SteadyPressurePA: 980, DecaySeconds: 1, NeighborResponsePA: 1}))
	}
	versioned, err := svc.GetBatch("B1")
	if err != nil {
		t.Fatal(err)
	}
	if len(versioned.Batch.Defects[defect].TreatmentVersions) != 2 || versioned.Batch.Defects[defect].TreatmentVersions[0].Cause != "密封" {
		t.Fatal("追加处置覆盖了旧版本")
	}
	set(svc.CloseDefect("B1", CloseDefectCommand{CommandMeta: meta("engineer"), DefectID: defect}))
	set(svc.SubmitForReview("B1", SubmitCommand{CommandMeta: meta("engineer")}))
	view, err := svc.GetBatch("B1")
	if err != nil {
		t.Fatal(err)
	}
	items := make([]domain.ReviewItemResult, 0, len(view.Batch.ReviewChecklist.Items))
	for _, item := range view.Batch.ReviewChecklist.Items {
		items = append(items, domain.ReviewItemResult{ItemID: item.ItemID, Status: domain.ReviewItemPassed, Comment: "确认"})
	}
	if _, err = svc.Review("B1", ReviewCommand{CommandMeta: meta("engineer"), Decision: domain.DecisionApproved, Note: "不能自审", Items: items}); err == nil {
		t.Fatal("参与测量的人员通过了独立批准")
	}
	approved := set(svc.Review("B1", ReviewCommand{CommandMeta: meta("reviewer"), Decision: domain.DecisionApproved, Note: "依据完整", Items: items}))
	if approved.Certificate == nil {
		t.Fatal("批准后没有证书")
	}
	verified, err := svc.VerifyCertificate("B1", approved.Certificate.CanonicalDigest)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Valid {
		t.Fatalf("证书校验失败: %s", verified.Message)
	}
	if !verified.Checks.CanonicalContent.Valid || !verified.Checks.ReviewSnapshot.Valid || !verified.Checks.AuditHead.Valid || !verified.Checks.InputDigest.Valid {
		t.Fatal("证书分项核验未全部通过")
	}
	before, err := svc.GetBatch("B1")
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := svc.DownloadCertificate("B1")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := svc.DownloadCertificate("B1")
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("重复下载证书字节不一致")
	}
	wrong, err := svc.VerifyCertificate("B1", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	if wrong.Valid || wrong.Checks.InputDigest.Valid || !wrong.Checks.CanonicalContent.Valid || !wrong.Checks.ReviewSnapshot.Valid || !wrong.Checks.AuditHead.Valid {
		t.Fatalf("错误输入摘要的分项结果不正确: %+v", wrong.Checks)
	}
	after, err := svc.GetBatch("B1")
	if err != nil {
		t.Fatal(err)
	}
	if before.Batch.Revision != after.Batch.Revision || len(before.Batch.Audit) != len(after.Batch.Audit) {
		t.Fatal("证书下载或核验改变了批次")
	}
}
