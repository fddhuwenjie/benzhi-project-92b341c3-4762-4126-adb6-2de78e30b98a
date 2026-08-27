package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

func thresholdValue(value float64) string {
	return strconv.FormatFloat(value, 'f', 9, 64)
}

func thresholdDirection(before, after float64, higherIsStricter bool) ThresholdChangeDirection {
	if before == after {
		return ThresholdUnchanged
	}
	if (after > before) == higherIsStricter {
		return ThresholdStricter
	}
	return ThresholdLooser
}

func ThresholdChanges(before, after ThresholdProfile) []ThresholdFieldChange {
	return []ThresholdFieldChange{
		{Field: "minimum_pressure_ratio", Before: thresholdValue(before.MinimumPressureRatio), After: thresholdValue(after.MinimumPressureRatio), Direction: thresholdDirection(before.MinimumPressureRatio, after.MinimumPressureRatio, true)},
		{Field: "maximum_pressure_ratio", Before: thresholdValue(before.MaximumPressureRatio), After: thresholdValue(after.MaximumPressureRatio), Direction: thresholdDirection(before.MaximumPressureRatio, after.MaximumPressureRatio, false)},
		{Field: "maximum_decay_seconds", Before: thresholdValue(before.MaximumDecaySeconds), After: thresholdValue(after.MaximumDecaySeconds), Direction: thresholdDirection(before.MaximumDecaySeconds, after.MaximumDecaySeconds, false)},
		{Field: "maximum_neighbor_ratio", Before: thresholdValue(before.MaximumNeighborRatio), After: thresholdValue(after.MaximumNeighborRatio), Direction: thresholdDirection(before.MaximumNeighborRatio, after.MaximumNeighborRatio, false)},
		{Field: "required_consecutive_passes", Before: strconv.Itoa(before.RequiredConsecutivePasses), After: strconv.Itoa(after.RequiredConsecutivePasses), Direction: thresholdDirection(float64(before.RequiredConsecutivePasses), float64(after.RequiredConsecutivePasses), true)},
	}
}

func thresholdDigest(version uint64, profile ThresholdProfile, changes []ThresholdFieldChange) string {
	data, _ := json.Marshal(struct {
		Version uint64                 `json:"version"`
		Profile ThresholdProfile       `json:"profile"`
		Changes []ThresholdFieldChange `json:"changes"`
	}{version, profile, changes})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func NewInitialThresholdRevision(profile ThresholdProfile, actor string, now time.Time) (ThresholdRevision, error) {
	if err := profile.Validate(); err != nil {
		return ThresholdRevision{}, err
	}
	changes := ThresholdChanges(profile, profile)
	return ThresholdRevision{Version: 1, Profile: profile, Reason: "建立批次初始阈值方案", ActorID: actor, At: now.UTC(), Changes: changes, Digest: thresholdDigest(1, profile, changes)}, nil
}

func (b *QualificationBatch) ReviseThresholds(profile ThresholdProfile, reason, actor string, now time.Time) (ThresholdRevision, error) {
	revision, err := b.PreviewThresholdRevision(profile, reason, actor, now)
	if err != nil {
		return ThresholdRevision{}, err
	}
	b.ThresholdProfile = profile
	b.ThresholdHistory = append(b.ThresholdHistory, revision)
	return revision, nil
}

func (b *QualificationBatch) PreviewThresholdRevision(profile ThresholdProfile, reason, actor string, now time.Time) (ThresholdRevision, error) {
	if b.State != StateDraft {
		return ThresholdRevision{}, NewError(CodeState, "冻结后不得修改阈值方案")
	}
	if strings.TrimSpace(reason) == "" || strings.TrimSpace(actor) == "" {
		return ThresholdRevision{}, NewError(CodeInvalid, "变更原因和操作人不能为空")
	}
	if err := profile.Validate(); err != nil {
		return ThresholdRevision{}, err
	}
	changes := ThresholdChanges(b.ThresholdProfile, profile)
	changed := false
	for _, change := range changes {
		if change.Direction != ThresholdUnchanged {
			changed = true
			break
		}
	}
	if !changed {
		return ThresholdRevision{}, NewError(CodeInvalid, "阈值方案没有实际变化")
	}
	version := uint64(len(b.ThresholdHistory) + 1)
	revision := ThresholdRevision{Version: version, Profile: profile, Reason: strings.TrimSpace(reason), ActorID: actor, At: now.UTC(), Changes: changes}
	revision.Digest = thresholdDigest(version, profile, changes)
	return revision, nil
}

func (b *QualificationBatch) CurrentThresholdRevision() *ThresholdRevision {
	if len(b.ThresholdHistory) == 0 {
		return nil
	}
	return &b.ThresholdHistory[len(b.ThresholdHistory)-1]
}
