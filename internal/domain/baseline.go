package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

func tapDefinition(t *PressureTap) *TapDefinition {
	if t == nil {
		return nil
	}
	neighbors := append([]string{}, t.NeighborTapIDs...)
	sort.Strings(neighbors)
	return &TapDefinition{TapID: t.TapID, Label: t.Label, SurfaceZone: t.SurfaceZone, NominalDiameterMM: t.NominalDiameterMM, NeighborTapIDs: neighbors}
}

func (b *QualificationBatch) ValidateTapDefinitions(inputs []PressureTap) error {
	seen := map[string]bool{}
	for _, tap := range inputs {
		if strings.TrimSpace(tap.TapID) == "" || strings.TrimSpace(tap.Label) == "" || strings.TrimSpace(tap.SurfaceZone) == "" || tap.NominalDiameterMM <= 0 {
			return NewError(CodeInvalid, "测孔编号、标签、区域和标称孔径必须有效")
		}
		if seen[tap.TapID] {
			return NewError(CodeConflict, "测孔 %s 重复", tap.TapID)
		}
		seen[tap.TapID] = true
		neighbors := map[string]bool{}
		for _, n := range tap.NeighborTapIDs {
			if n == tap.TapID {
				return NewError(CodeInvalid, "测孔 %s 不能以自身为相邻孔", tap.TapID)
			}
			if neighbors[n] {
				return NewError(CodeInvalid, "测孔 %s 的相邻孔重复", tap.TapID)
			}
			neighbors[n] = true
		}
	}
	for _, tap := range inputs {
		for _, n := range tap.NeighborTapIDs {
			if !seen[n] {
				return NewError(CodeInvalid, "测孔 %s 的相邻孔 %s 不存在", tap.TapID, n)
			}
		}
	}
	return nil
}

func (b *QualificationBatch) DraftDiff(inputs []PressureTap) (DraftBaselineDiff, error) {
	if err := b.ValidateTapDefinitions(inputs); err != nil {
		return DraftBaselineDiff{}, err
	}
	current := map[string]*PressureTap{}
	for _, t := range inputs {
		t.BatchID = b.BatchID
		current[t.TapID] = &t
	}
	diff := DraftBaselineDiff{Added: []DraftTapChange{}, Modified: []DraftTapChange{}, Deleted: []DraftTapChange{}}
	for _, id := range SortedTapIDs(current) {
		if old := b.Taps[id]; old == nil {
			diff.Added = append(diff.Added, DraftTapChange{TapID: id, After: tapDefinition(current[id])})
		} else if !sameTapDefinition(old, current[id]) {
			diff.Modified = append(diff.Modified, DraftTapChange{TapID: id, Before: tapDefinition(old), After: tapDefinition(current[id])})
		}
	}
	for _, id := range SortedTapIDs(b.Taps) {
		if current[id] == nil {
			diff.Deleted = append(diff.Deleted, DraftTapChange{TapID: id, Before: tapDefinition(b.Taps[id])})
		}
	}
	diff.Summary = diffSummary(diff)
	return diff, nil
}

func sameTapDefinition(a, c *PressureTap) bool {
	if a.Label != c.Label || a.SurfaceZone != c.SurfaceZone || a.NominalDiameterMM != c.NominalDiameterMM {
		return false
	}
	return strings.Join(tapDefinition(a).NeighborTapIDs, "\x00") == strings.Join(tapDefinition(c).NeighborTapIDs, "\x00")
}

func diffSummary(diff DraftBaselineDiff) string {
	data, _ := json.Marshal(diffWithoutSummary(diff))
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
func diffWithoutSummary(diff DraftBaselineDiff) any {
	return struct {
		Added    []DraftTapChange `json:"added"`
		Modified []DraftTapChange `json:"modified"`
		Deleted  []DraftTapChange `json:"deleted"`
	}{diff.Added, diff.Modified, diff.Deleted}
}

func (b *QualificationBatch) ReviseBaseline(inputs []PressureTap, reason, actor string, now time.Time) (DraftBaselineDiff, error) {
	if b.State != StateDraft {
		return DraftBaselineDiff{}, NewError(CodeState, "只有草拟状态可以修订测孔清单")
	}
	if strings.TrimSpace(reason) == "" {
		return DraftBaselineDiff{}, NewError(CodeInvalid, "变更原因不能为空")
	}
	diff, err := b.DraftDiff(inputs)
	if err != nil {
		return DraftBaselineDiff{}, err
	}
	next := map[string]*PressureTap{}
	for _, t := range inputs {
		t.BatchID = b.BatchID
		t.QualificationStatus = TapPending
		next[t.TapID] = &t
	}
	b.Taps = next
	b.DraftRevision++
	b.DraftBaselineDiff = &diff
	b.DraftHistory = append(b.DraftHistory, DraftBaselineRevision{Revision: b.DraftRevision, Reason: reason, ActorID: actor, At: now.UTC(), Diff: diff})
	return diff, nil
}

func (b *QualificationBatch) FreezeBaselineConfirmed(summary string) error {
	if b.State != StateDraft {
		return NewError(CodeState, "只有草拟批次可冻结基线")
	}
	if len(b.Taps) == 0 {
		return NewError(CodeInvalid, "至少需要一个压力测孔")
	}
	if strings.TrimSpace(summary) != "" && b.DraftBaselineDiff != nil && summary != b.DraftBaselineDiff.Summary {
		return NewError(CodeConflict, "冻结差异摘要不一致，请刷新后重新确认")
	}
	if strings.TrimSpace(summary) != "" && b.DraftBaselineDiff == nil {
		return NewError(CodeConflict, "缺少待确认的草拟差异摘要")
	}
	if err := b.ValidateTapDefinitions(mapTaps(b.Taps)); err != nil {
		return err
	}
	b.State = StateBaselineFrozen
	b.BaselineRevision = b.Revision + 1
	return nil
}

func mapTaps(taps map[string]*PressureTap) []PressureTap {
	ids := SortedTapIDs(taps)
	out := make([]PressureTap, 0, len(ids))
	for _, id := range ids {
		out = append(out, *taps[id])
	}
	return out
}

func (b *QualificationBatch) CurrentDraftSummary() string {
	if b.DraftBaselineDiff == nil {
		return ""
	}
	return b.DraftBaselineDiff.Summary
}
