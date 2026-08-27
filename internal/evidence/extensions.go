package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"pressure-tap-qualification/internal/domain"
)

func StableDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func SubmissionPreflightDigest(preflight domain.SubmissionPreflight) (string, error) {
	preflight.FactDigest = ""
	preflight.RemainingValidSeconds = 0
	return StableDigest(preflight)
}

func DifferencePackage(history []domain.ReviewRecord, current domain.ReviewSnapshotFacts, currentDigest string) (domain.ReviewDifferencePackage, error) {
	pack := domain.ReviewDifferencePackage{FirstSubmission: len(history) == 0, CurrentSnapshotDigest: currentDigest, Differences: []domain.ReviewDifference{}}
	if len(history) > 0 {
		previous := history[len(history)-1]
		if previous.Snapshot != nil {
			pack.PreviousSnapshotDigest = previous.Snapshot.Digest
		}
		pack.Differences = compareFacts(previous.Facts, current)
	}
	digest, err := StableDigest(struct {
		First       bool                      `json:"first_submission"`
		Previous    string                    `json:"previous_snapshot_digest"`
		Current     string                    `json:"current_snapshot_digest"`
		Differences []domain.ReviewDifference `json:"differences"`
	}{pack.FirstSubmission, pack.PreviousSnapshotDigest, pack.CurrentSnapshotDigest, pack.Differences})
	if err != nil {
		return pack, err
	}
	pack.Digest = digest
	return pack, nil
}

func compareFacts(previous, current domain.ReviewSnapshotFacts) []domain.ReviewDifference {
	out := []domain.ReviewDifference{}
	out = append(out, compareSet("round", previous.EffectiveRoundIDs, current.EffectiveRoundIDs, "voided", "added")...)
	out = append(out, compareSet("voided_round", previous.VoidedRoundIDs, current.VoidedRoundIDs, "restored", "voided")...)
	out = append(out, compareSet("treatment", previous.TreatmentVersionIDs, current.TreatmentVersionIDs, "removed", "added")...)
	out = append(out, compareSet("calibration", previous.CalibrationRefs, current.CalibrationRefs, "removed", "changed")...)
	ids := map[string]bool{}
	for id := range previous.DefectStatuses {
		ids[id] = true
	}
	for id := range current.DefectStatuses {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		before, beforeOK := previous.DefectStatuses[id]
		after, afterOK := current.DefectStatuses[id]
		if beforeOK != afterOK || before != after {
			out = append(out, domain.ReviewDifference{Category: "defect", Action: "status_changed", EntityID: id, Before: before, After: after})
		}
	}
	if previous.QualificationSummary != current.QualificationSummary {
		out = append(out, domain.ReviewDifference{Category: "qualification", Action: "changed", EntityID: "qualification_summary", Before: previous.QualificationSummary, After: current.QualificationSummary})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category == out[j].Category {
			if out[i].EntityID == out[j].EntityID {
				return out[i].Action < out[j].Action
			}
			return out[i].EntityID < out[j].EntityID
		}
		return out[i].Category < out[j].Category
	})
	return out
}

func compareSet(category string, before, after []string, removedAction, addedAction string) []domain.ReviewDifference {
	a, c := map[string]bool{}, map[string]bool{}
	for _, id := range before {
		a[id] = true
	}
	for _, id := range after {
		c[id] = true
	}
	out := []domain.ReviewDifference{}
	for id := range a {
		if !c[id] {
			out = append(out, domain.ReviewDifference{Category: category, Action: removedAction, EntityID: id})
		}
	}
	for id := range c {
		if !a[id] {
			out = append(out, domain.ReviewDifference{Category: category, Action: addedAction, EntityID: id})
		}
	}
	return out
}

type AuditEventValidation struct {
	Sequence uint64 `json:"sequence"`
	Valid    bool   `json:"valid"`
	Reason   string `json:"reason,omitempty"`
}

type AuditSegmentValidation struct {
	Valid           bool                   `json:"valid"`
	FirstSequence   uint64                 `json:"first_sequence,omitempty"`
	LastSequence    uint64                 `json:"last_sequence,omitempty"`
	PrecedingDigest string                 `json:"preceding_digest,omitempty"`
	FollowingDigest string                 `json:"following_digest,omitempty"`
	AuditHeadDigest string                 `json:"audit_head_digest,omitempty"`
	Items           []AuditEventValidation `json:"items"`
}

func ValidateAuditSegment(full, segment []domain.AuditEvent) AuditSegmentValidation {
	out := AuditSegmentValidation{Valid: true, Items: []AuditEventValidation{}}
	if len(full) > 0 {
		out.AuditHeadDigest = full[len(full)-1].Digest
	}
	if len(segment) == 0 {
		return out
	}
	out.FirstSequence, out.LastSequence = segment[0].Sequence, segment[len(segment)-1].Sequence
	bySequence := map[uint64]domain.AuditEvent{}
	for _, event := range full {
		bySequence[event.Sequence] = event
	}
	if previous, ok := bySequence[out.FirstSequence-1]; ok {
		out.PrecedingDigest = previous.Digest
	}
	if following, ok := bySequence[out.LastSequence+1]; ok {
		out.FollowingDigest = following.Digest
	}
	for _, event := range segment {
		item := AuditEventValidation{Sequence: event.Sequence, Valid: true}
		calculated, err := auditEventDigest(event)
		if err != nil || calculated != event.Digest {
			item.Valid, item.Reason = false, "事件摘要不匹配"
		}
		if original, ok := bySequence[event.Sequence]; !ok || original.Digest != event.Digest {
			item.Valid, item.Reason = false, "事件不在完整审计链对应位置"
		}
		if event.Sequence > 1 {
			if previous, ok := bySequence[event.Sequence-1]; !ok || event.PreviousDigest != previous.Digest {
				item.Valid, item.Reason = false, "previous_digest 不连续"
			}
		} else if event.PreviousDigest != "" {
			item.Valid, item.Reason = false, "首事件 previous_digest 必须为空"
		}
		if !item.Valid {
			out.Valid = false
		}
		out.Items = append(out.Items, item)
	}
	return out
}

func auditEventDigest(event domain.AuditEvent) (string, error) {
	event.Digest = ""
	data, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
