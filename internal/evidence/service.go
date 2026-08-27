package evidence

import (
	"crypto/subtle"
	"fmt"
	"regexp"
	"sync"
	"time"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/store"
)

type Service struct {
	repository   store.Repository
	mu           sync.Mutex
	certificates map[string]*domain.ReleaseCertificate
}

func New(repository store.Repository) *Service {
	return &Service{repository: repository, certificates: map[string]*domain.ReleaseCertificate{}}
}

func (s *Service) loadCertificate(batchID string) (*domain.ReleaseCertificate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if certificate := s.certificates[batchID]; certificate != nil {
		return certificate, nil
	}
	certificate, err := s.repository.LoadCertificate(batchID)
	if err != nil {
		return nil, err
	}
	s.certificates[batchID] = certificate
	return certificate, nil
}

func (s *Service) Issue(batch *domain.QualificationBatch, reviewer, note, auditHead string, issuedAt time.Time) (*domain.ReleaseCertificate, error) {
	if batch.State != domain.StateApproved || batch.ReviewSnapshot == nil {
		return nil, domain.NewError(domain.CodeState, "批准批次缺少送审快照")
	}
	cert := domain.ReleaseCertificate{CertificateID: "CERT-" + batch.BatchID, BatchID: batch.BatchID, ReviewerID: reviewer, ReviewDecision: domain.DecisionApproved, ReviewNote: note, SnapshotRevision: batch.ReviewSnapshot.Revision, IssuedAt: issuedAt.UTC(), AuditHeadDigest: auditHead, ThresholdVersion: batch.ReviewSnapshot.ThresholdVersion, SnapshotValidUntil: batch.ReviewSnapshot.ValidUntil}
	data, err := canonicalBytes(batch, cert)
	if err != nil {
		return nil, err
	}
	cert.CanonicalDigest = sum(data)
	if err = s.repository.SaveCertificate(batch.BatchID, cert); err != nil {
		return nil, err
	}
	return &cert, nil
}

type Verification struct {
	Valid       bool                       `json:"valid"`
	Message     string                     `json:"message"`
	Certificate *domain.ReleaseCertificate `json:"certificate,omitempty"`
	Checks      VerificationChecks         `json:"checks"`
}
type CheckResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}
type VerificationChecks struct {
	CanonicalContent CheckResult `json:"canonical_content"`
	ReviewSnapshot   CheckResult `json:"review_snapshot"`
	AuditHead        CheckResult `json:"audit_head"`
	InputDigest      CheckResult `json:"input_digest"`
}

var digestPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

func (s *Service) Download(batchID string) ([]byte, *domain.ReleaseCertificate, error) {
	batch, err := s.repository.Get(batchID)
	if err != nil {
		return nil, nil, err
	}
	if batch.State != domain.StateApproved {
		return nil, nil, domain.NewError(domain.CodeState, "只有批准终态批次可下载资格证书")
	}
	cert, err := s.loadCertificate(batchID)
	if err != nil {
		return nil, nil, err
	}
	data, err := documentBytes(batch, *cert)
	return data, cert, err
}

func (s *Service) Verify(batchID, claimedDigest string) (Verification, error) {
	if claimedDigest != "" && !digestPattern.MatchString(claimedDigest) {
		return Verification{}, domain.NewError(domain.CodeInvalid, "输入摘要必须为 64 位 SHA-256 十六进制值")
	}
	batch, err := s.repository.Get(batchID)
	if err != nil {
		return Verification{}, err
	}
	if batch.State != domain.StateApproved {
		return Verification{}, domain.NewError(domain.CodeState, "批次不是批准终态")
	}
	cert, err := s.loadCertificate(batchID)
	if err != nil {
		return Verification{}, err
	}
	data, err := canonicalBytes(batch, *cert)
	if err != nil {
		return Verification{}, err
	}
	calculated := sum(data)
	head, err := s.repository.AuditHead(batchID)
	if err != nil {
		return Verification{}, err
	}
	snapshotDigest, err := SnapshotDigest(batch)
	if err != nil {
		return Verification{}, err
	}
	digestOK := subtle.ConstantTimeCompare([]byte(calculated), []byte(cert.CanonicalDigest)) == 1
	headOK := subtle.ConstantTimeCompare([]byte(head), []byte(cert.AuditHeadDigest)) == 1
	snapshotOK := subtle.ConstantTimeCompare([]byte(snapshotDigest), []byte(batch.ReviewSnapshot.Digest)) == 1
	claimOK := claimedDigest == "" || subtle.ConstantTimeCompare([]byte(claimedDigest), []byte(cert.CanonicalDigest)) == 1
	valid := digestOK && headOK && snapshotOK && claimOK
	message := "证书内容、审计链头和封存记录一致"
	if !valid {
		message = fmt.Sprintf("校验失败：证书内容=%t，送审快照=%t，审计链=%t，输入摘要=%t", digestOK, snapshotOK, headOK, claimOK)
	}
	checks := VerificationChecks{CanonicalContent: CheckResult{Valid: digestOK, Message: checkMessage(digestOK, "证书规范内容一致", "证书规范内容与封存摘要不一致")}, ReviewSnapshot: CheckResult{Valid: snapshotOK, Message: checkMessage(snapshotOK, "送审快照一致", "送审快照摘要不一致")}, AuditHead: CheckResult{Valid: headOK, Message: checkMessage(headOK, "审计链头一致", "当前审计链头与证书不一致")}, InputDigest: CheckResult{Valid: claimOK, Message: checkMessage(claimOK, "输入摘要一致", "输入摘要与证书摘要不一致")}}
	return Verification{Valid: valid, Message: message, Certificate: cert, Checks: checks}, nil
}

func checkMessage(ok bool, pass, fail string) string {
	if ok {
		return pass
	}
	return fail
}
