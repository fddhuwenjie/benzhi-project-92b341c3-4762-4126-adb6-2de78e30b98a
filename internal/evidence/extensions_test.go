package evidence

import (
	"testing"
	"time"

	"pressure-tap-qualification/internal/domain"
)

func TestDifferencePackageAndAuditDamageLocation(t *testing.T) {
	previous := domain.ReviewSnapshotFacts{EffectiveRoundIDs: []string{"R1"}, TreatmentVersionIDs: []string{}, DefectStatuses: map[string]string{"D1": "treated"}, CalibrationRefs: []string{"CAL-1"}, QualificationSummary: "old"}
	history := []domain.ReviewRecord{{Snapshot: &domain.ReviewSnapshot{Digest: "snapshot-old"}, Facts: previous}}
	current := domain.ReviewSnapshotFacts{EffectiveRoundIDs: []string{"R1", "R2"}, TreatmentVersionIDs: []string{"T1"}, DefectStatuses: map[string]string{"D1": "closed"}, CalibrationRefs: []string{"CAL-1"}, QualificationSummary: "new"}
	pack, err := DifferencePackage(history, current, "snapshot-new")
	if err != nil {
		t.Fatal(err)
	}
	if pack.FirstSubmission || pack.Digest == "" || len(pack.Differences) != 4 {
		t.Fatalf("差异包不完整: %+v", pack)
	}

	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	events := []domain.AuditEvent{{Sequence: 1, At: now, ActorID: "a", Action: "one", Summary: "one"}}
	events[0].Digest, _ = auditEventDigest(events[0])
	second := domain.AuditEvent{Sequence: 2, At: now, ActorID: "b", Action: "two", Summary: "two", PreviousDigest: events[0].Digest}
	second.Digest, _ = auditEventDigest(second)
	events = append(events, second)
	corrupt := append([]domain.AuditEvent(nil), events...)
	corrupt[1].PreviousDigest = "broken"
	validation := ValidateAuditSegment(events, corrupt)
	if validation.Valid || len(validation.Items) != 2 || validation.Items[1].Valid || validation.Items[1].Sequence != 2 {
		t.Fatalf("未定位损坏事件: %+v", validation)
	}
}
