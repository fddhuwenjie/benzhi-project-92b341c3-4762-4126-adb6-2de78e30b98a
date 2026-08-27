package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pressure-tap-qualification/internal/domain"
)

func TestIdempotentReplayAndConflict(t *testing.T) {
	r, err := NewFileRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mutate := func(b *domain.QualificationBatch) (any, EventDraft, error) {
		created, _ := domain.NewBatch("B1", "M1", "目标", "owner", domain.DefaultThresholds(), time.Now())
		*b = *created
		return map[string]string{"ok": "yes"}, EventDraft{ActorID: "owner", Action: "create", Summary: "建立", At: time.Now()}, nil
	}
	first, err := r.Execute("B1", "R1", "fingerprint", 0, true, mutate)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Execute("B1", "R1", "fingerprint", 0, true, mutate)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Replayed || second.Revision != first.Revision {
		t.Fatal("相同请求没有重放原结果")
	}
	if _, err = r.Execute("B1", "R1", "different", 1, false, mutate); err == nil {
		t.Fatal("不同指纹复用 request_id 应冲突")
	}
}

func TestStartupRejectsCorruptEventFrame(t *testing.T) {
	root := t.TempDir()
	r, err := NewFileRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Execute("B1", "R1", "fp", 0, true, func(b *domain.QualificationBatch) (any, EventDraft, error) {
		created, _ := domain.NewBatch("B1", "M1", "目标", "owner", domain.DefaultThresholds(), time.Now())
		*b = *created
		return "ok", EventDraft{ActorID: "owner", Action: "create", Summary: "建立", At: time.Now()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "batches", "B1", "events.frames")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("00000010 broken")
	_ = f.Close()
	if _, err = NewFileRepository(root); err == nil {
		t.Fatal("损坏事件帧未被拒绝")
	}
}

func TestPendingTransactionRecoversAfterEventAppend(t *testing.T) {
	root := t.TempDir()
	r, err := NewFileRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.Execute("B1", "R1", "fp1", 0, true, func(b *domain.QualificationBatch) (any, EventDraft, error) {
		created, _ := domain.NewBatch("B1", "M1", "目标", "owner", domain.DefaultThresholds(), time.Now())
		*b = *created
		return "created", EventDraft{ActorID: "owner", Action: "create", Summary: "建立", At: time.Now()}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.Get("B1")
	if err != nil {
		t.Fatal(err)
	}
	working, err := cloneBatch(b)
	if err != nil {
		t.Fatal(err)
	}
	working.ModelCode = "M2"
	working.Revision = 2
	previous := working.Audit[len(working.Audit)-1].Digest
	event := domain.AuditEvent{Sequence: 2, At: time.Now().UTC(), ActorID: "owner", Action: "batch.changed", Summary: "修改模型", PreviousDigest: previous}
	event.Digest, err = eventDigest(event)
	if err != nil {
		t.Fatal(err)
	}
	working.Audit = append(working.Audit, event)
	record := IdempotencyRecord{RequestID: "R2", Fingerprint: "fp2", Response: json.RawMessage(`{"ok":true}`), Revision: 2, RecordedAt: event.At}
	transaction := pendingTransaction{Version: 1, BatchID: "B1", PreviousRevision: 1, Batch: working, Event: event, Idempotency: record}
	if err = r.stageTransaction(transaction); err != nil {
		t.Fatal(err)
	}
	if err = appendEvent(r.events("B1"), event); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewFileRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := reopened.Get("B1")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Revision != 2 || recovered.ModelCode != "M2" || len(recovered.Audit) != 2 {
		t.Fatalf("提交恢复不完整: revision=%d model=%s audit=%d", recovered.Revision, recovered.ModelCode, len(recovered.Audit))
	}
}
