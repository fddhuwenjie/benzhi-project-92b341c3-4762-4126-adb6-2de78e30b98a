package domain

import (
	"math"
	"testing"
	"time"
)

func TestBatchInfoRevisionAndFrozenDigest(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	b, err := NewBatch("B-INFO", "M-1", "原目标", "owner", DefaultThresholds(), now)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := b.ReviseBatchInfo("M-2", "补充后的目标", "纠正建档内容", "owner", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(revision.Changes) != 2 || revision.Changes[0].Field != "model_code" || len(b.BatchInfoHistory) != 1 {
		t.Fatalf("批次信息差异不完整: %+v", revision)
	}
	b.FrozenBatchInfoDigest = b.BatchInfoDigest()
	b.State = StateBaselineFrozen
	if _, err = b.ReviseBatchInfo("M-3", b.TestObjective, "冻结后修改", "owner", now); ErrorCodeOf(err) != CodeState {
		t.Fatal("冻结后仍可修订批次信息")
	}
}

func TestNeighborResponsesUseWorstFrozenChannel(t *testing.T) {
	tap := &PressureTap{TapID: "A", NeighborTapIDs: []string{"B", "C"}}
	result, normalized, maximum, worst, err := ClassifyMeasurementResponses(DefaultThresholds(), tap, 1000, 980, 1, []NeighborResponse{{TapID: "C", ResponsePA: 80}, {TapID: "B", ResponsePA: 20}})
	if err != nil {
		t.Fatal(err)
	}
	if worst != "C" || maximum != 80 || normalized[0].TapID != "B" || result.NeighborRatio != .08 || result.Passed {
		t.Fatalf("最不利通道判定错误: %+v", result)
	}
	if _, _, _, _, err = ClassifyMeasurementResponses(DefaultThresholds(), tap, 1000, 980, 1, []NeighborResponse{{TapID: "B", ResponsePA: 20}}); err == nil {
		t.Fatal("缺少冻结邻接通道未被拒绝")
	}
	if _, _, _, _, err = ClassifyMeasurementResponses(DefaultThresholds(), tap, 1000, 980, 1, []NeighborResponse{{TapID: "B", ResponsePA: 20}, {TapID: "X", ResponsePA: 1}}); err == nil {
		t.Fatal("未知通道未被拒绝")
	}
}

func TestDriftSequenceExcludesInvalidRounds(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	b := &QualificationBatch{ThresholdProfile: DefaultThresholds(), Taps: map[string]*PressureTap{"A": {TapID: "A", SurfaceZone: "Z"}}}
	for i, ratio := range []float64{.99, .96, .92} {
		b.Rounds = append(b.Rounds, &MeasurementRound{RoundID: string(rune('1' + i)), TapID: "A", RoundKind: RoundInitial, RecordedAt: now.Add(time.Duration(i) * time.Minute), SupplyPressurePA: 1000, SteadyPressurePA: ratio * 1000, DecaySeconds: 1 + float64(i)*.2, Result: MeasurementResult{PressureRatio: ratio, NeighborRatio: .01}})
	}
	comparisons := b.RoundDriftComparisons()
	if len(comparisons) != 3 || comparisons[2].Level != DriftWorsening {
		t.Fatalf("恶化提示错误: %+v", comparisons)
	}
	b.Rounds[1].CalibrationInvalid = true
	comparisons = b.RoundDriftComparisons()
	if len(comparisons) != 2 || !comparisons[1].Comparable || math.Abs(*comparisons[1].PressureRatioDelta+.07) > 1e-9 {
		t.Fatalf("隔离后相邻轮次未重连: %+v", comparisons)
	}
}

func TestAssignmentHandoverAndCompletion(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	d := &DefectCase{Status: DefectOpen}
	if _, err := d.Assign("tech-a", now.Add(time.Hour), PriorityHigh, "安排返修", "owner", now); err != nil {
		t.Fatal(err)
	}
	if err := d.CheckTreatmentAssignee("tech-b", "", now); err == nil {
		t.Fatal("非责任技师无接手说明仍可处置")
	}
	if err := d.CheckTreatmentAssignee("tech-b", "现场接手", now); err != nil {
		t.Fatal(err)
	}
	if len(d.Handovers) != 1 || d.Handovers[0].ToTechnicianID != "tech-b" {
		t.Fatal("交接事实未追加")
	}
	d.CompleteAssignment(now)
	if d.CurrentAssignment().CompletedAt == nil {
		t.Fatal("任务未完成")
	}
}
