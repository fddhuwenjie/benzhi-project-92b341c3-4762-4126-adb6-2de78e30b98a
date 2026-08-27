package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

type RevisionHistoryType string

const (
	HistoryBatchInfo RevisionHistoryType = "batch_info"
	HistoryThreshold RevisionHistoryType = "threshold"
	HistoryBaseline  RevisionHistoryType = "baseline"
)

type NormalizedFieldChange struct {
	EntityID string `json:"entity_id"`
	Field    string `json:"field"`
	Action   string `json:"action"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
}

type RevisionHistoryEntry struct {
	Version      uint64                  `json:"version"`
	ActorID      string                  `json:"actor_id"`
	At           string                  `json:"at"`
	Reason       string                  `json:"reason"`
	BeforeDigest string                  `json:"before_digest"`
	AfterDigest  string                  `json:"after_digest"`
	StoredDigest string                  `json:"stored_digest"`
	Changes      []NormalizedFieldChange `json:"changes"`
}

type RevisionDifference struct {
	HistoryType RevisionHistoryType     `json:"history_type"`
	FromVersion uint64                  `json:"from_version"`
	ToVersion   uint64                  `json:"to_version"`
	FromDigest  string                  `json:"from_digest"`
	ToDigest    string                  `json:"to_digest"`
	Changes     []NormalizedFieldChange `json:"changes"`
	Digest      string                  `json:"digest"`
}

type RevisionHistoryView struct {
	BatchID            string                 `json:"batch_id"`
	BatchRevision      uint64                 `json:"batch_revision"`
	HistoryType        RevisionHistoryType    `json:"history_type"`
	Entries            []RevisionHistoryEntry `json:"entries"`
	Difference         *RevisionDifference    `json:"difference,omitempty"`
	FrozenVersion      uint64                 `json:"frozen_version,omitempty"`
	FrozenDigest       string                 `json:"frozen_digest,omitempty"`
	FrozenDigestValid  bool                   `json:"frozen_digest_valid"`
	SequenceValid      bool                   `json:"sequence_valid"`
	UnsubmittedChanges *DraftBaselineDiff     `json:"unsubmitted_changes,omitempty"`
	EditLocation       string                 `json:"edit_location"`
}

func BaselineDefinitionDigest(taps map[string]*PressureTap) string {
	definitions := make([]TapDefinition, 0, len(taps))
	for _, id := range SortedTapIDs(taps) {
		definitions = append(definitions, *tapDefinition(taps[id]))
	}
	return stableHistoryDigest(definitions)
}

func (b *QualificationBatch) RevisionHistory(kind RevisionHistoryType, fromVersion, toVersion uint64) (RevisionHistoryView, error) {
	out := RevisionHistoryView{BatchID: b.BatchID, BatchRevision: b.Revision, HistoryType: kind, Entries: []RevisionHistoryEntry{}, SequenceValid: true}
	var states map[uint64]map[string]string
	switch kind {
	case HistoryBatchInfo:
		out.Entries, states = b.batchInfoHistoryEntries()
		out.FrozenVersion, out.FrozenDigest = uint64(len(b.BatchInfoHistory)), b.FrozenBatchInfoDigest
		out.EditLocation = "overview.batch-info-editor"
	case HistoryThreshold:
		out.Entries, states = b.thresholdHistoryEntries()
		out.FrozenVersion, out.FrozenDigest = b.FrozenThresholdVersion, b.FrozenThresholdDigest
		out.EditLocation = "overview.threshold-editor"
	case HistoryBaseline:
		out.Entries, states = b.baselineHistoryEntries()
		out.FrozenVersion, out.FrozenDigest = b.FrozenDraftVersion, b.FrozenBaselineDigest
		out.EditLocation = "overview.baseline-editor"
		if b.State == StateDraft && b.DraftBaselineDiff != nil {
			diff := *b.DraftBaselineDiff
			out.UnsubmittedChanges = &diff
		}
	default:
		return RevisionHistoryView{}, NewFieldError(CodeInvalid, "history_type", "history_type 必须为 batch_info、threshold 或 baseline")
	}
	for i, entry := range out.Entries {
		if entry.Version != uint64(i+1) {
			out.SequenceValid = false
		}
	}
	if !out.SequenceValid {
		return RevisionHistoryView{}, NewError(CodeState, "历史版本序号不连续")
	}
	if out.FrozenDigest != "" {
		switch kind {
		case HistoryBatchInfo:
			out.FrozenDigestValid = out.FrozenVersion == uint64(len(b.BatchInfoHistory)) && b.BatchInfoDigest() == out.FrozenDigest
		case HistoryThreshold:
			for _, revision := range b.ThresholdHistory {
				if revision.Version == out.FrozenVersion {
					out.FrozenDigestValid = revision.Digest == out.FrozenDigest
				}
			}
		case HistoryBaseline:
			out.FrozenDigestValid = out.FrozenVersion == b.DraftRevision && BaselineDefinitionDigest(b.Taps) == out.FrozenDigest
		}
		if !out.FrozenDigestValid {
			return RevisionHistoryView{}, NewError(CodeState, "冻结摘要无法由历史版本复算")
		}
	}
	if fromVersion == 0 && toVersion == 0 {
		return out, nil
	}
	if fromVersion == 0 || toVersion == 0 {
		return RevisionHistoryView{}, NewFieldError(CodeInvalid, "from_version", "from_version 和 to_version 必须同时提供")
	}
	from, fromOK := states[fromVersion]
	to, toOK := states[toVersion]
	if !fromOK {
		return RevisionHistoryView{}, NewFieldError(CodeNotFound, "from_version", fmt.Sprintf("版本 %d 不存在", fromVersion))
	}
	if !toOK {
		return RevisionHistoryView{}, NewFieldError(CodeNotFound, "to_version", fmt.Sprintf("版本 %d 不存在", toVersion))
	}
	difference := RevisionDifference{HistoryType: kind, FromVersion: fromVersion, ToVersion: toVersion, FromDigest: stableHistoryDigest(from), ToDigest: stableHistoryDigest(to), Changes: compareHistoryStates(from, to)}
	copy := difference
	copy.Digest = ""
	difference.Digest = stableHistoryDigest(copy)
	out.Difference = &difference
	return out, nil
}

func (b *QualificationBatch) batchInfoHistoryEntries() ([]RevisionHistoryEntry, map[uint64]map[string]string) {
	states := map[uint64]map[string]string{}
	current := map[string]string{"model_code": b.ModelCode, "test_objective": b.TestObjective}
	for i := len(b.BatchInfoHistory) - 1; i >= 0; i-- {
		revision := b.BatchInfoHistory[i]
		states[revision.Version] = cloneStringMap(current)
		for _, change := range revision.Changes {
			current[change.Field] = change.Before
		}
	}
	entries := make([]RevisionHistoryEntry, 0, len(b.BatchInfoHistory))
	before := current
	for _, revision := range b.BatchInfoHistory {
		after := states[revision.Version]
		entries = append(entries, RevisionHistoryEntry{Version: revision.Version, ActorID: revision.ActorID, At: revision.At.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), Reason: revision.Reason, BeforeDigest: stableHistoryDigest(before), AfterDigest: stableHistoryDigest(after), StoredDigest: revision.Digest, Changes: compareHistoryStates(before, after)})
		before = after
	}
	return entries, states
}

func (b *QualificationBatch) thresholdHistoryEntries() ([]RevisionHistoryEntry, map[uint64]map[string]string) {
	entries := make([]RevisionHistoryEntry, 0, len(b.ThresholdHistory))
	states := map[uint64]map[string]string{}
	before := map[string]string{}
	for _, revision := range b.ThresholdHistory {
		after := thresholdState(revision.Profile)
		states[revision.Version] = after
		entries = append(entries, RevisionHistoryEntry{Version: revision.Version, ActorID: revision.ActorID, At: revision.At.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), Reason: revision.Reason, BeforeDigest: stableHistoryDigest(before), AfterDigest: stableHistoryDigest(after), StoredDigest: revision.Digest, Changes: compareHistoryStates(before, after)})
		before = after
	}
	return entries, states
}

func (b *QualificationBatch) baselineHistoryEntries() ([]RevisionHistoryEntry, map[uint64]map[string]string) {
	states := map[uint64]map[string]string{}
	current := baselineState(b.Taps)
	for i := len(b.DraftHistory) - 1; i >= 0; i-- {
		revision := b.DraftHistory[i]
		states[revision.Revision] = cloneStringMap(current)
		for _, change := range revision.Diff.Added {
			deleteTapState(current, change.TapID)
		}
		for _, change := range revision.Diff.Modified {
			setTapState(current, change.Before)
		}
		for _, change := range revision.Diff.Deleted {
			setTapState(current, change.Before)
		}
	}
	entries := make([]RevisionHistoryEntry, 0, len(b.DraftHistory))
	before := current
	for _, revision := range b.DraftHistory {
		after := states[revision.Revision]
		entries = append(entries, RevisionHistoryEntry{Version: revision.Revision, ActorID: revision.ActorID, At: revision.At.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), Reason: revision.Reason, BeforeDigest: stableHistoryDigest(before), AfterDigest: stableHistoryDigest(after), StoredDigest: revision.Diff.Summary, Changes: compareHistoryStates(before, after)})
		before = after
	}
	return entries, states
}

func thresholdState(profile ThresholdProfile) map[string]string {
	return map[string]string{"minimum_pressure_ratio": thresholdValue(profile.MinimumPressureRatio), "maximum_pressure_ratio": thresholdValue(profile.MaximumPressureRatio), "maximum_decay_seconds": thresholdValue(profile.MaximumDecaySeconds), "maximum_neighbor_ratio": thresholdValue(profile.MaximumNeighborRatio), "required_consecutive_passes": strconv.Itoa(profile.RequiredConsecutivePasses)}
}
func baselineState(taps map[string]*PressureTap) map[string]string {
	out := map[string]string{}
	for _, id := range SortedTapIDs(taps) {
		setTapState(out, tapDefinition(taps[id]))
	}
	return out
}
func setTapState(state map[string]string, tap *TapDefinition) {
	if tap == nil {
		return
	}
	prefix := "tap." + tap.TapID + "."
	neighbors := append([]string(nil), tap.NeighborTapIDs...)
	sort.Strings(neighbors)
	data, _ := json.Marshal(neighbors)
	state[prefix+"label"], state[prefix+"surface_zone"], state[prefix+"nominal_diameter_mm"], state[prefix+"neighbor_tap_ids"] = tap.Label, tap.SurfaceZone, strconv.FormatFloat(tap.NominalDiameterMM, 'f', 9, 64), string(data)
}
func deleteTapState(state map[string]string, tapID string) {
	prefix := "tap." + tapID + "."
	for key := range state {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(state, key)
		}
	}
}
func compareHistoryStates(before, after map[string]string) []NormalizedFieldChange {
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	out := []NormalizedFieldChange{}
	for _, key := range ordered {
		a, aok := before[key]
		c, cok := after[key]
		if aok && cok && a == c {
			continue
		}
		action := "modified"
		if !aok {
			action = "added"
		}
		if !cok {
			action = "deleted"
		}
		entity, field := "batch", key
		if len(key) > 4 && key[:4] == "tap." {
			rest := key[4:]
			for i, ch := range rest {
				if ch == '.' {
					entity, field = rest[:i], rest[i+1:]
					break
				}
			}
		}
		out = append(out, NormalizedFieldChange{EntityID: entity, Field: field, Action: action, Before: a, After: c})
	}
	return out
}
func stableHistoryDigest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func cloneStringMap(input map[string]string) map[string]string {
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
