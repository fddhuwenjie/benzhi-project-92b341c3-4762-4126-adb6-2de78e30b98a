package web

import (
	"fmt"
	"net/http"

	"pressure-tap-qualification/internal/application"
	"pressure-tap-qualification/internal/domain"
)

func (s *Server) HandleMeasurement(w http.ResponseWriter, r *http.Request) {
	var cmd application.MeasurementCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if cmd.NeighborResponses == nil {
		writeError(w, domain.NewFieldError(domain.CodeInvalid, "neighbor_responses", "必须按冻结拓扑提交相邻响应明细，零相邻孔请提交空数组"))
		return
	}
	out, err := s.app.RecordMeasurement(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
func (s *Server) HandleBatchMeasurementPreflight(w http.ResponseWriter, r *http.Request) {
	var cmd application.BatchMeasurementCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if err := requireBatchNeighborResponses(cmd.Rows); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.PreflightBatchMeasurement(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleBatchMeasurement(w http.ResponseWriter, r *http.Request) {
	var cmd application.BatchMeasurementCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if err := requireBatchNeighborResponses(cmd.Rows); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.RecordBatchMeasurements(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
func (s *Server) HandleTreatment(w http.ResponseWriter, r *http.Request) {
	var cmd application.TreatmentCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	cmd.DefectID = r.PathValue("defectID")
	out, err := s.app.TreatDefect(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleDefectAssignment(w http.ResponseWriter, r *http.Request) {
	var cmd application.AssignDefectCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	cmd.DefectID = r.PathValue("defectID")
	out, err := s.app.AssignDefect(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleCloseDefect(w http.ResponseWriter, r *http.Request) {
	var cmd application.CloseDefectCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	cmd.DefectID = r.PathValue("defectID")
	out, err := s.app.CloseDefect(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	var cmd application.SubmitCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if cmd.PreflightDigest == "" {
		writeError(w, domain.NewFieldError(domain.CodeInvalid, "preflight_digest", "送审确认必须携带最近预检摘要"))
		return
	}
	out, err := s.app.SubmitForReview(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func requireBatchNeighborResponses(rows []application.MeasurementRow) error {
	for i, row := range rows {
		if row.NeighborResponses == nil {
			return domain.NewFieldError(domain.CodeInvalid, "rows.neighbor_responses", fmt.Sprintf("第 %d 行必须提交 neighbor_responses", i+1))
		}
	}
	return nil
}
func (s *Server) HandleSubmitPreflight(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.PreflightSubmission(r.PathValue("batchID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleReviewerPreflight(w http.ResponseWriter, r *http.Request) {
	out, err := s.app.PreflightReviewer(r.PathValue("batchID"), r.URL.Query().Get("reviewer_id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleReview(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviewCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.Review(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type verifyRequest struct {
	RequestID        string `json:"request_id,omitempty"`
	ExpectedRevision uint64 `json:"expected_revision,omitempty"`
	ActorID          string `json:"actor_id,omitempty"`
	BatchID          string `json:"batch_id"`
	Digest           string `json:"digest"`
}

func (s *Server) HandleVerifyCertificate(w http.ResponseWriter, r *http.Request) {
	var input verifyRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.VerifyCertificate(input.BatchID, input.Digest)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) HandleDownloadCertificate(w http.ResponseWriter, r *http.Request) {
	data, cert, err := s.app.DownloadCertificate(r.PathValue("batchID"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+cert.CertificateID+`.json"`)
	w.Header().Set("X-Content-SHA256", cert.CanonicalDigest)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
