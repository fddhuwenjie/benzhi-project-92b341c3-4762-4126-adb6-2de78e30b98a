package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type TopologySeverity string

const (
	TopologyError   TopologySeverity = "error"
	TopologyWarning TopologySeverity = "warning"
)

type TopologyIssue struct {
	IssueID      string           `json:"issue_id"`
	Code         string           `json:"code"`
	Severity     TopologySeverity `json:"severity"`
	TapID        string           `json:"tap_id"`
	RelatedTapID string           `json:"related_tap_id,omitempty"`
	Message      string           `json:"message"`
}

type TopologyPreflight struct {
	DraftRevision uint64          `json:"draft_revision"`
	Digest        string          `json:"digest"`
	Issues        []TopologyIssue `json:"issues"`
	ErrorCount    int             `json:"error_count"`
	WarningCount  int             `json:"warning_count"`
	Ready         bool            `json:"ready"`
}

type TopologyAcknowledgement struct {
	IssueID string    `json:"issue_id"`
	ActorID string    `json:"actor_id"`
	At      time.Time `json:"at"`
}

func topologyIssueID(code, tap, related string) string {
	return code + ":" + tap + ":" + related
}

func (b *QualificationBatch) TopologyPreflight() TopologyPreflight {
	issues := make([]TopologyIssue, 0)
	incoming := map[string]int{}
	for _, tapID := range SortedTapIDs(b.Taps) {
		tap := b.Taps[tapID]
		seen := map[string]bool{}
		for _, neighbor := range tap.NeighborTapIDs {
			if seen[neighbor] {
				issues = append(issues, TopologyIssue{IssueID: topologyIssueID("duplicate_reference", tapID, neighbor), Code: "duplicate_reference", Severity: TopologyError, TapID: tapID, RelatedTapID: neighbor, Message: fmt.Sprintf("测孔 %s 重复引用相邻孔 %s", tapID, neighbor)})
				continue
			}
			seen[neighbor] = true
			if neighbor == tapID {
				issues = append(issues, TopologyIssue{IssueID: topologyIssueID("self_reference", tapID, neighbor), Code: "self_reference", Severity: TopologyError, TapID: tapID, RelatedTapID: neighbor, Message: fmt.Sprintf("测孔 %s 不能引用自身", tapID)})
				continue
			}
			other := b.Taps[neighbor]
			if other == nil {
				issues = append(issues, TopologyIssue{IssueID: topologyIssueID("unknown_reference", tapID, neighbor), Code: "unknown_reference", Severity: TopologyError, TapID: tapID, RelatedTapID: neighbor, Message: fmt.Sprintf("测孔 %s 引用了未知测孔 %s", tapID, neighbor)})
				continue
			}
			incoming[neighbor]++
			if !containsString(other.NeighborTapIDs, tapID) {
				issues = append(issues, TopologyIssue{IssueID: topologyIssueID("one_way", tapID, neighbor), Code: "one_way", Severity: TopologyError, TapID: tapID, RelatedTapID: neighbor, Message: fmt.Sprintf("测孔 %s 到 %s 的邻接关系缺少反向声明", tapID, neighbor)})
			}
			if tap.SurfaceZone != other.SurfaceZone && tapID < neighbor {
				issues = append(issues, TopologyIssue{IssueID: topologyIssueID("cross_zone", tapID, neighbor), Code: "cross_zone", Severity: TopologyWarning, TapID: tapID, RelatedTapID: neighbor, Message: fmt.Sprintf("测孔 %s 与 %s 跨 surface_zone 相邻", tapID, neighbor)})
			}
		}
	}
	for _, tapID := range SortedTapIDs(b.Taps) {
		if len(b.Taps[tapID].NeighborTapIDs) == 0 && incoming[tapID] == 0 {
			issues = append(issues, TopologyIssue{IssueID: topologyIssueID("isolated", tapID, ""), Code: "isolated", Severity: TopologyWarning, TapID: tapID, Message: fmt.Sprintf("测孔 %s 为孤立测孔", tapID)})
		}
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].IssueID < issues[j].IssueID })
	result := TopologyPreflight{DraftRevision: b.DraftRevision, Issues: issues}
	for _, issue := range issues {
		if issue.Severity == TopologyError {
			result.ErrorCount++
		} else {
			result.WarningCount++
		}
	}
	canonicalTaps := mapTaps(b.Taps)
	for i := range canonicalTaps {
		canonicalTaps[i].NeighborTapIDs = append([]string(nil), canonicalTaps[i].NeighborTapIDs...)
		sort.Strings(canonicalTaps[i].NeighborTapIDs)
	}
	data, _ := json.Marshal(struct {
		Revision uint64          `json:"revision"`
		Taps     []PressureTap   `json:"taps"`
		Issues   []TopologyIssue `json:"issues"`
	}{b.DraftRevision, canonicalTaps, issues})
	sum := sha256.Sum256(data)
	result.Digest = hex.EncodeToString(sum[:])
	result.Ready = result.ErrorCount == 0
	return result
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (b *QualificationBatch) ConfirmTopology(digest string, warningIDs []string, actor string, now time.Time) error {
	preflight := b.TopologyPreflight()
	if strings.TrimSpace(digest) == "" || digest != preflight.Digest {
		return NewError(CodeConflict, "拓扑预检摘要已陈旧，请重新预检")
	}
	if preflight.ErrorCount > 0 {
		return NewError(CodeInvalid, "拓扑预检仍有 %d 项错误", preflight.ErrorCount)
	}
	warnings := map[string]bool{}
	for _, issue := range preflight.Issues {
		if issue.Severity == TopologyWarning {
			warnings[issue.IssueID] = true
		}
	}
	acknowledged := map[string]bool{}
	for _, id := range warningIDs {
		if !warnings[id] {
			return NewError(CodeInvalid, "拓扑警告确认 %s 不存在或已经陈旧", id)
		}
		if acknowledged[id] {
			return NewError(CodeInvalid, "拓扑警告确认 %s 重复", id)
		}
		acknowledged[id] = true
	}
	if len(acknowledged) != len(warnings) {
		return NewError(CodeInvalid, "必须逐项确认全部拓扑警告")
	}
	b.TopologyAcknowledgements = make([]TopologyAcknowledgement, 0, len(warningIDs))
	sort.Strings(warningIDs)
	for _, id := range warningIDs {
		b.TopologyAcknowledgements = append(b.TopologyAcknowledgements, TopologyAcknowledgement{IssueID: id, ActorID: actor, At: now.UTC()})
	}
	b.FrozenTopologyDigest = digest
	for _, tap := range b.Taps {
		sort.Strings(tap.NeighborTapIDs)
	}
	return nil
}
