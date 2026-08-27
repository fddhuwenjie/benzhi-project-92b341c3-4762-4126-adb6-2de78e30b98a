package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"time"

	"pressure-tap-qualification/internal/domain"
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type FileRepository struct {
	root  string
	guard sync.Mutex
	locks map[string]*sync.Mutex
}

func NewFileRepository(root string) (*FileRepository, error) {
	if root == "" {
		return nil, fmt.Errorf("数据目录不能为空")
	}
	if err := os.MkdirAll(filepath.Join(root, "batches"), 0750); err != nil {
		return nil, err
	}
	r := &FileRepository{root: root, locks: map[string]*sync.Mutex{}}
	entries, err := os.ReadDir(filepath.Join(root, "batches"))
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err = r.recoverPending(entry.Name()); err != nil {
			return nil, fmt.Errorf("批次 %s 提交恢复失败: %w", entry.Name(), err)
		}
		if _, err = r.load(entry.Name()); err != nil {
			return nil, fmt.Errorf("批次 %s 数据损坏: %w", entry.Name(), err)
		}
	}
	return r, nil
}

func (r *FileRepository) lockFor(id string) *sync.Mutex {
	r.guard.Lock()
	defer r.guard.Unlock()
	if r.locks[id] == nil {
		r.locks[id] = &sync.Mutex{}
	}
	return r.locks[id]
}
func (r *FileRepository) dir(id string) string      { return filepath.Join(r.root, "batches", id) }
func (r *FileRepository) snapshot(id string) string { return filepath.Join(r.dir(id), "snapshot.json") }
func (r *FileRepository) events(id string) string   { return filepath.Join(r.dir(id), "events.frames") }
func (r *FileRepository) idem(id, request string) string {
	return filepath.Join(r.dir(id), "idempotency", request+".json")
}
func (r *FileRepository) cert(id string) string { return filepath.Join(r.dir(id), "certificate.json") }

func validateID(kind, id string) error {
	if !safeID.MatchString(id) {
		return domain.NewError(domain.CodeInvalid, "%s 格式无效", kind)
	}
	return nil
}

func (r *FileRepository) load(id string) (*domain.QualificationBatch, error) {
	var b domain.QualificationBatch
	if err := readJSON(r.snapshot(id), &b); err != nil {
		return nil, err
	}
	events, err := loadEvents(r.events(id))
	if err != nil {
		return nil, err
	}
	if uint64(len(events)) != b.Revision {
		return nil, fmt.Errorf("快照修订号 %d 与事件数 %d 不一致", b.Revision, len(events))
	}
	if len(events) > 0 && events[len(events)-1].Digest != b.Audit[len(b.Audit)-1].Digest {
		return nil, fmt.Errorf("快照审计链头不一致")
	}
	b.Audit = events
	if err = b.ValidateIntegrity(); err != nil {
		return nil, fmt.Errorf("聚合完整性校验失败: %w", err)
	}
	return &b, nil
}

func cloneBatch(b *domain.QualificationBatch) (*domain.QualificationBatch, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return nil, err
	}
	var out domain.QualificationBatch
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func fingerprintValue(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (r *FileRepository) Execute(id, requestID, fingerprint string, expected uint64, create bool, mutation Mutation) (CommitResult, error) {
	if err := validateID("batch_id", id); err != nil {
		return CommitResult{}, err
	}
	if err := validateID("request_id", requestID); err != nil {
		return CommitResult{}, err
	}
	if fingerprint == "" {
		return CommitResult{}, domain.NewError(domain.CodeInvalid, "请求指纹不能为空")
	}
	lock := r.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	var prior IdempotencyRecord
	if err := readJSON(r.idem(id, requestID), &prior); err == nil {
		if prior.Fingerprint != fingerprint {
			return CommitResult{}, domain.NewError(domain.CodeConflict, "request_id 已用于不同请求")
		}
		return CommitResult{Response: prior.Response, Revision: prior.Revision, Replayed: true}, nil
	} else if !os.IsNotExist(err) {
		return CommitResult{}, err
	}
	var b *domain.QualificationBatch
	loaded, err := r.load(id)
	if err == nil {
		b = loaded
	} else if os.IsNotExist(err) && create {
		b = nil
	} else {
		return CommitResult{}, func() error {
			if os.IsNotExist(err) {
				return domain.NewError(domain.CodeNotFound, "批次不存在")
			}
			return err
		}()
	}
	actual := uint64(0)
	if b != nil {
		actual = b.Revision
	}
	if actual != expected {
		return CommitResult{}, domain.NewError(domain.CodeConflict, "修订冲突：期望 %d，当前 %d", expected, actual)
	}
	working := b
	if b != nil {
		working, err = cloneBatch(b)
		if err != nil {
			return CommitResult{}, err
		}
	} else {
		working = &domain.QualificationBatch{}
	}
	response, event, err := mutation(working)
	if err != nil {
		return CommitResult{}, err
	}
	if working.BatchID == "" {
		return CommitResult{}, errors.New("提交未创建批次")
	}
	if working.BatchID != id {
		return CommitResult{}, errors.New("批次标识被修改")
	}
	working.Revision = actual + 1
	event.At = event.At.UTC()
	previous := ""
	if len(working.Audit) > 0 {
		previous = working.Audit[len(working.Audit)-1].Digest
	}
	audit := domain.AuditEvent{Sequence: working.Revision, At: event.At, ActorID: event.ActorID, Action: event.Action, Summary: event.Summary, PreviousDigest: previous, RelatedIDs: append([]string(nil), event.RelatedIDs...)}
	audit.Digest, err = eventDigest(audit)
	if err != nil {
		return CommitResult{}, err
	}
	working.Audit = append(working.Audit, audit)
	responseData, err := marshalResponse(response)
	if err != nil {
		return CommitResult{}, err
	}
	record := IdempotencyRecord{RequestID: requestID, Fingerprint: fingerprint, Response: responseData, Revision: working.Revision, RecordedAt: event.At}
	if err = os.MkdirAll(r.dir(id), 0750); err != nil {
		return CommitResult{}, err
	}
	transaction := pendingTransaction{Version: 1, BatchID: id, PreviousRevision: actual, Batch: working, Event: audit, Idempotency: record}
	if err = r.stageTransaction(transaction); err != nil {
		return CommitResult{}, err
	}
	if err = r.applyTransaction(&transaction); err != nil {
		return CommitResult{}, err
	}
	return CommitResult{Response: responseData, Revision: working.Revision}, nil
}

func (r *FileRepository) Get(id string) (*domain.QualificationBatch, error) {
	if err := validateID("batch_id", id); err != nil {
		return nil, err
	}
	lock := r.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	b, err := r.load(id)
	if os.IsNotExist(err) {
		return nil, domain.NewError(domain.CodeNotFound, "批次不存在")
	}
	return b, err
}

func (r *FileRepository) List() ([]*domain.QualificationBatch, error) {
	entries, err := os.ReadDir(filepath.Join(r.root, "batches"))
	if err != nil {
		return nil, err
	}
	out := []*domain.QualificationBatch{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		b, err := r.Get(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *FileRepository) SaveCertificate(id string, cert domain.ReleaseCertificate) error {
	lock := r.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	var old domain.ReleaseCertificate
	if err := readJSON(r.cert(id), &old); err == nil {
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(cert)
		if string(a) == string(b) {
			return nil
		}
		return domain.NewError(domain.CodeConflict, "资格证书已封存，不能覆盖")
	} else if !os.IsNotExist(err) {
		return err
	}
	return atomicJSON(r.cert(id), cert)
}
func (r *FileRepository) LoadCertificate(id string) (*domain.ReleaseCertificate, error) {
	var c domain.ReleaseCertificate
	if err := readJSON(r.cert(id), &c); os.IsNotExist(err) {
		return nil, domain.NewError(domain.CodeNotFound, "资格证书不存在")
	} else if err != nil {
		return nil, err
	}
	// 预埋缺陷：部分写入的证书被当作空成功，存储层没有把资源完整性错误传给证据层。
	if c.CertificateID == "" {
		return nil, nil
	}
	return &c, nil
}
func (r *FileRepository) AuditHead(id string) (string, error) {
	b, err := r.Get(id)
	if err != nil {
		return "", err
	}
	if len(b.Audit) == 0 {
		return "", nil
	}
	return b.Audit[len(b.Audit)-1].Digest, nil
}

func NewFingerprint(command any) (string, error) { return fingerprintValue(command) }
func nowUTC() time.Time                          { return time.Now().UTC() }
