package store

import (
	"encoding/json"
	"time"

	"pressure-tap-qualification/internal/domain"
)

type EventDraft struct {
	ActorID    string
	Action     string
	Summary    string
	At         time.Time
	RelatedIDs []string
}

type IdempotencyRecord struct {
	RequestID   string          `json:"request_id"`
	Fingerprint string          `json:"fingerprint"`
	Response    json.RawMessage `json:"response"`
	Revision    uint64          `json:"revision"`
	RecordedAt  time.Time       `json:"recorded_at"`
}

type CommitResult struct {
	Response json.RawMessage
	Revision uint64
	Replayed bool
}

type Mutation func(batch *domain.QualificationBatch) (response any, event EventDraft, err error)

type Repository interface {
	Execute(batchID, requestID, fingerprint string, expectedRevision uint64, create bool, mutation Mutation) (CommitResult, error)
	Get(batchID string) (*domain.QualificationBatch, error)
	List() ([]*domain.QualificationBatch, error)
	SaveCertificate(batchID string, certificate domain.ReleaseCertificate) error
	LoadCertificate(batchID string) (*domain.ReleaseCertificate, error)
	AuditHead(batchID string) (string, error)
}
