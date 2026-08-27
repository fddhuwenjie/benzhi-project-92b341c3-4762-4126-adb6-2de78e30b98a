package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"pressure-tap-qualification/internal/domain"
)

type pendingTransaction struct {
	Version          int                        `json:"version"`
	BatchID          string                     `json:"batch_id"`
	PreviousRevision uint64                     `json:"previous_revision"`
	Batch            *domain.QualificationBatch `json:"batch"`
	Event            domain.AuditEvent          `json:"event"`
	Idempotency      IdempotencyRecord          `json:"idempotency"`
}

func (r *FileRepository) pending(id string) string {
	return filepath.Join(r.dir(id), "pending-commit.json")
}

func (r *FileRepository) stageTransaction(transaction pendingTransaction) error {
	if transaction.Version != 1 {
		return fmt.Errorf("不支持的提交日志版本 %d", transaction.Version)
	}
	if transaction.Batch == nil || transaction.Batch.BatchID != transaction.BatchID {
		return fmt.Errorf("提交日志批次不一致")
	}
	if transaction.Batch.Revision != transaction.PreviousRevision+1 {
		return fmt.Errorf("提交日志修订不连续")
	}
	if transaction.Event.Sequence != transaction.Batch.Revision {
		return fmt.Errorf("提交日志事件序号不一致")
	}
	return atomicJSON(r.pending(transaction.BatchID), transaction)
}

func (r *FileRepository) readPending(id string) (*pendingTransaction, error) {
	var transaction pendingTransaction
	if err := readJSON(r.pending(id), &transaction); err != nil {
		return nil, err
	}
	if transaction.Version != 1 || transaction.BatchID != id || transaction.Batch == nil {
		return nil, fmt.Errorf("批次 %s 的提交日志无效", id)
	}
	if transaction.Batch.Revision != transaction.PreviousRevision+1 {
		return nil, fmt.Errorf("批次 %s 的提交日志修订无效", id)
	}
	if transaction.Event.Sequence != transaction.Batch.Revision {
		return nil, fmt.Errorf("批次 %s 的提交日志事件无效", id)
	}
	return &transaction, nil
}

func (r *FileRepository) applyTransaction(transaction *pendingTransaction) error {
	events, validBytes, err := loadValidEventPrefix(r.events(transaction.BatchID))
	if err != nil {
		return err
	}
	count := uint64(len(events))
	switch count {
	case transaction.PreviousRevision:
		info, statErr := os.Stat(r.events(transaction.BatchID))
		if statErr == nil && info.Size() != validBytes {
			if truncateErr := os.Truncate(r.events(transaction.BatchID), validBytes); truncateErr != nil {
				return fmt.Errorf("截断中断事件帧: %w", truncateErr)
			}
		}
		if err = appendEvent(r.events(transaction.BatchID), transaction.Event); err != nil {
			return fmt.Errorf("追加提交事件: %w", err)
		}
	case transaction.Batch.Revision:
		last := events[len(events)-1]
		if last.Digest != transaction.Event.Digest {
			return fmt.Errorf("已追加事件与提交日志摘要不一致")
		}
		info, statErr := os.Stat(r.events(transaction.BatchID))
		if statErr == nil && info.Size() != validBytes {
			return fmt.Errorf("已提交事件后存在额外损坏数据")
		}
	default:
		return fmt.Errorf("事件修订 %d 无法恢复提交 %d -> %d", count, transaction.PreviousRevision, transaction.Batch.Revision)
	}
	if err = atomicJSON(r.snapshot(transaction.BatchID), transaction.Batch); err != nil {
		return fmt.Errorf("保存提交快照: %w", err)
	}
	if err = atomicJSON(r.idem(transaction.BatchID, transaction.Idempotency.RequestID), transaction.Idempotency); err != nil {
		return fmt.Errorf("保存幂等结果: %w", err)
	}
	if err = os.Remove(r.pending(transaction.BatchID)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("清理提交日志: %w", err)
	}
	directory, err := os.Open(r.dir(transaction.BatchID))
	if err != nil {
		return err
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (r *FileRepository) recoverPending(id string) error {
	transaction, err := r.readPending(id)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.applyTransaction(transaction)
}

func marshalResponse(value any) (json.RawMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return data, nil
}
