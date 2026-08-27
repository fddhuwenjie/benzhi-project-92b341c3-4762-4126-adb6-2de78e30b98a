package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

func (b *QualificationBatch) BatchInfoDigest() string {
	data, _ := json.Marshal(struct {
		BatchID       string `json:"batch_id"`
		ModelCode     string `json:"model_code"`
		TestObjective string `json:"test_objective"`
	}{b.BatchID, b.ModelCode, b.TestObjective})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (b *QualificationBatch) PreviewBatchInfoRevision(modelCode, objective, reason, actor string, now time.Time) (BatchInfoRevision, error) {
	if b.State != StateDraft {
		return BatchInfoRevision{}, NewError(CodeState, "只有草拟状态可以修订批次信息")
	}
	modelCode = strings.TrimSpace(modelCode)
	objective = strings.TrimSpace(objective)
	reason = strings.TrimSpace(reason)
	if modelCode == "" {
		return BatchInfoRevision{}, NewFieldError(CodeInvalid, "model_code", "model_code 不能为空")
	}
	if objective == "" {
		return BatchInfoRevision{}, NewFieldError(CodeInvalid, "test_objective", "test_objective 不能为空")
	}
	if reason == "" {
		return BatchInfoRevision{}, NewFieldError(CodeInvalid, "reason", "变更原因不能为空")
	}
	if strings.TrimSpace(actor) == "" {
		return BatchInfoRevision{}, NewFieldError(CodeInvalid, "actor_id", "操作人不能为空")
	}
	changes := []BatchInfoFieldChange{}
	if b.ModelCode != modelCode {
		changes = append(changes, BatchInfoFieldChange{Field: "model_code", Before: b.ModelCode, After: modelCode})
	}
	if b.TestObjective != objective {
		changes = append(changes, BatchInfoFieldChange{Field: "test_objective", Before: b.TestObjective, After: objective})
	}
	if len(changes) == 0 {
		return BatchInfoRevision{}, NewError(CodeConflict, "批次信息没有实际变化")
	}
	revision := BatchInfoRevision{Version: uint64(len(b.BatchInfoHistory) + 1), Reason: reason, ActorID: actor, At: now.UTC(), Changes: changes}
	revision.Digest = batchInfoRevisionDigest(revision)
	for _, prior := range b.BatchInfoHistory {
		if prior.Digest == revision.Digest {
			return BatchInfoRevision{}, NewError(CodeConflict, "批次信息修订摘要重复")
		}
	}
	return revision, nil
}

func batchInfoRevisionDigest(revision BatchInfoRevision) string {
	data, _ := json.Marshal(struct {
		Reason  string                 `json:"reason"`
		Changes []BatchInfoFieldChange `json:"changes"`
	}{revision.Reason, revision.Changes})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (b *QualificationBatch) ReviseBatchInfo(modelCode, objective, reason, actor string, now time.Time) (BatchInfoRevision, error) {
	revision, err := b.PreviewBatchInfoRevision(modelCode, objective, reason, actor, now)
	if err != nil {
		return BatchInfoRevision{}, err
	}
	b.ModelCode = strings.TrimSpace(modelCode)
	b.TestObjective = strings.TrimSpace(objective)
	b.BatchInfoHistory = append(b.BatchInfoHistory, revision)
	return revision, nil
}
