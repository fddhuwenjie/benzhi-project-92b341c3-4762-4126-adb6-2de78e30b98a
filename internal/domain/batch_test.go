package domain

import (
	"testing"
	"time"
)

func TestFrozenBaselineCannotChange(t *testing.T) {
	now := time.Now().UTC()
	b, err := NewBatch("B-1", "M-1", "试验", "owner", DefaultThresholds(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.AddTap(PressureTap{TapID: "T1", Label: "测孔 1", SurfaceZone: "wing", NominalDiameterMM: 1}); err != nil {
		t.Fatal(err)
	}
	if err = b.FreezeBaseline(); err != nil {
		t.Fatal(err)
	}
	if err = b.AddTap(PressureTap{TapID: "T2", Label: "测孔 2", SurfaceZone: "wing", NominalDiameterMM: 1}); err == nil {
		t.Fatal("冻结后仍然可以添加测孔")
	}
}

func TestReviewerMustBeIndependent(t *testing.T) {
	b := &QualificationBatch{CreatedBy: "owner", MeasurementParticipants: map[string]bool{"operator": true}, RemediationParticipants: map[string]bool{"tech": true}}
	for _, id := range []string{"owner", "operator", "tech"} {
		if err := b.CheckReviewer(id); err == nil {
			t.Fatalf("人员 %s 不应通过独立性检查", id)
		}
	}
	if err := b.CheckReviewer("reviewer"); err != nil {
		t.Fatal(err)
	}
}
