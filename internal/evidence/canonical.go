package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"pressure-tap-qualification/internal/domain"
)

type canonicalCertificate struct {
	Schema             string `json:"schema"`
	CertificateID      string `json:"certificate_id"`
	BatchID            string `json:"batch_id"`
	ModelCode          string `json:"model_code"`
	TestObjective      string `json:"test_objective"`
	BaselineRevision   uint64 `json:"baseline_revision"`
	TapCount           int    `json:"tap_count"`
	Coverage           string `json:"coverage"`
	ReviewerID         string `json:"reviewer_id"`
	Decision           string `json:"decision"`
	ReviewNote         string `json:"review_note"`
	SnapshotRevision   uint64 `json:"snapshot_revision"`
	SnapshotDigest     string `json:"snapshot_digest"`
	IssuedAt           string `json:"issued_at"`
	AuditHeadDigest    string `json:"audit_head_digest"`
	ThresholdVersion   uint64 `json:"threshold_version"`
	SnapshotValidUntil string `json:"snapshot_valid_until"`
}

type CertificateDocument struct {
	Schema             string `json:"schema"`
	CertificateID      string `json:"certificate_id"`
	BatchID            string `json:"batch_id"`
	ModelCode          string `json:"model_code"`
	TestObjective      string `json:"test_objective"`
	BaselineRevision   uint64 `json:"baseline_revision"`
	TapCount           int    `json:"tap_count"`
	Coverage           string `json:"coverage"`
	ReviewerID         string `json:"reviewer_id"`
	Decision           string `json:"decision"`
	ReviewNote         string `json:"review_note"`
	SnapshotRevision   uint64 `json:"snapshot_revision"`
	SnapshotDigest     string `json:"snapshot_digest"`
	IssuedAt           string `json:"issued_at"`
	CanonicalDigest    string `json:"canonical_digest"`
	AuditHeadDigest    string `json:"audit_head_digest"`
	ThresholdVersion   uint64 `json:"threshold_version"`
	SnapshotValidUntil string `json:"snapshot_valid_until"`
}

func canonicalBytes(batch *domain.QualificationBatch, certificate domain.ReleaseCertificate) ([]byte, error) {
	doc := canonicalCertificate{Schema: "pressure-tap-qualification-certificate/v1", CertificateID: certificate.CertificateID, BatchID: batch.BatchID, ModelCode: batch.ModelCode, TestObjective: batch.TestObjective, BaselineRevision: batch.BaselineRevision, TapCount: len(batch.Taps), Coverage: number(batch.Coverage()), ReviewerID: certificate.ReviewerID, Decision: string(certificate.ReviewDecision), ReviewNote: certificate.ReviewNote, SnapshotRevision: certificate.SnapshotRevision, SnapshotDigest: batch.ReviewSnapshot.Digest, IssuedAt: certificate.IssuedAt.UTC().Format(time.RFC3339Nano), AuditHeadDigest: certificate.AuditHeadDigest, ThresholdVersion: certificate.ThresholdVersion, SnapshotValidUntil: instant(certificate.SnapshotValidUntil)}
	return json.Marshal(doc)
}

func documentBytes(batch *domain.QualificationBatch, certificate domain.ReleaseCertificate) ([]byte, error) {
	doc := CertificateDocument{Schema: "pressure-tap-qualification-certificate/v1", CertificateID: certificate.CertificateID, BatchID: batch.BatchID, ModelCode: batch.ModelCode, TestObjective: batch.TestObjective, BaselineRevision: batch.BaselineRevision, TapCount: len(batch.Taps), Coverage: number(batch.Coverage()), ReviewerID: certificate.ReviewerID, Decision: string(certificate.ReviewDecision), ReviewNote: certificate.ReviewNote, SnapshotRevision: certificate.SnapshotRevision, SnapshotDigest: batch.ReviewSnapshot.Digest, IssuedAt: certificate.IssuedAt.UTC().Format(time.RFC3339Nano), CanonicalDigest: certificate.CanonicalDigest, AuditHeadDigest: certificate.AuditHeadDigest, ThresholdVersion: certificate.ThresholdVersion, SnapshotValidUntil: instant(certificate.SnapshotValidUntil)}
	return json.Marshal(doc)
}

func sum(data []byte) string { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }
