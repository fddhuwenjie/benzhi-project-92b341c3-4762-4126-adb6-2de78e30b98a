package application

import (
	"encoding/json"
	"strings"
	"time"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/evidence"
	"pressure-tap-qualification/internal/store"
)

type Clock func() time.Time
type Service struct {
	repo     store.Repository
	evidence *evidence.Service
	clock    Clock
}

func New(repo store.Repository, evidenceService *evidence.Service) *Service {
	return &Service{repo: repo, evidence: evidenceService, clock: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) WithClock(clock Clock) *Service { s.clock = clock; return s }

func validateMeta(m CommandMeta) error {
	if strings.TrimSpace(m.RequestID) == "" || strings.TrimSpace(m.ActorID) == "" {
		return domain.NewError(domain.CodeInvalid, "request_id 和 actor_id 不能为空")
	}
	return nil
}
func fingerprint(command any) (string, error) { return store.NewFingerprint(command) }

func decodeResult(result store.CommitResult) (CommandResponse, error) {
	var response CommandResponse
	if err := json.Unmarshal(result.Response, &response); err != nil {
		return response, err
	}
	response.Revision = result.Revision
	response.Replayed = result.Replayed
	return response, nil
}

func (s *Service) execute(batchID string, meta CommandMeta, command any, create bool, mutate store.Mutation) (CommandResponse, error) {
	if err := validateMeta(meta); err != nil {
		return CommandResponse{}, err
	}
	fp, err := fingerprint(command)
	if err != nil {
		return CommandResponse{}, err
	}
	result, err := s.repo.Execute(batchID, meta.RequestID, fp, meta.ExpectedRevision, create, mutate)
	if err != nil {
		return CommandResponse{}, err
	}
	return decodeResult(result)
}

func event(actor, action, summary string, at time.Time) store.EventDraft {
	return store.EventDraft{ActorID: actor, Action: action, Summary: summary, At: at}
}
func eventWithRelated(actor, action, summary string, at time.Time, related ...string) store.EventDraft {
	return store.EventDraft{ActorID: actor, Action: action, Summary: summary, At: at, RelatedIDs: related}
}
func response(b *domain.QualificationBatch, message string) CommandResponse {
	out := CommandResponse{BatchID: b.BatchID, State: b.State, Message: message, DraftDiffSummary: b.CurrentDraftSummary(), TopologyDigest: b.TopologyPreflight().Digest, BatchInfoDigest: b.BatchInfoDigest()}
	if current := b.CurrentThresholdRevision(); current != nil {
		out.ThresholdDigest = current.Digest
	}
	return out
}

func (s *Service) GetBatch(id string) (BatchView, error) {
	return s.GetBatchFiltered(id, BatchFilter{})
}

// loadCertificateForView keeps certificate lookup optional for read projections.
// The approved state is still projected when the backing certificate is unavailable.
func (s *Service) loadCertificateForView(id string) *domain.ReleaseCertificate {
	cert, _ := s.repo.LoadCertificate(id)
	return cert
}

func (s *Service) GetBatchFiltered(id string, filter BatchFilter) (BatchView, error) {
	b, err := s.repo.Get(id)
	if err != nil {
		return BatchView{}, err
	}
	var cert *domain.ReleaseCertificate
	if b.State == domain.StateApproved {
		cert = s.loadCertificateForView(id)
	}
	if err := validateFilter(b, filter); err != nil {
		return BatchView{}, err
	}
	return projectBatchFiltered(b, cert, s.clock(), filter), nil
}

func validateFilter(b *domain.QualificationBatch, f BatchFilter) error {
	if f.SurfaceZone != "" {
		found := false
		for _, t := range b.Taps {
			if t.SurfaceZone == f.SurfaceZone {
				found = true
				break
			}
		}
		if !found {
			return domain.NewError(domain.CodeInvalid, "区域 %s 不存在", f.SurfaceZone)
		}
	}
	if f.QualificationStatus != "" && f.QualificationStatus != domain.TapPending && f.QualificationStatus != domain.TapQualified && f.QualificationStatus != domain.TapDefective {
		return domain.NewError(domain.CodeInvalid, "资格状态 %s 无效", f.QualificationStatus)
	}
	if f.DefectType != "" && f.DefectType != domain.DefectMissing && f.DefectType != domain.DefectBlocked && f.DefectType != domain.DefectLeak && f.DefectType != domain.DefectLag && f.DefectType != domain.DefectCrosstalk {
		return domain.NewError(domain.CodeInvalid, "缺陷类型 %s 无效", f.DefectType)
	}
	return nil
}
func (s *Service) ListBatches() ([]BatchView, error) {
	batches, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	views := make([]BatchView, 0, len(batches))
	for _, b := range batches {
		views = append(views, projectBatch(b, nil, s.clock()))
	}
	return views, nil
}
func (s *Service) VerifyCertificate(id, digest string) (evidence.Verification, error) {
	return s.evidence.Verify(id, digest)
}
func (s *Service) DownloadCertificate(id string) ([]byte, *domain.ReleaseCertificate, error) {
	return s.evidence.Download(id)
}
