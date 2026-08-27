package web

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"pressure-tap-qualification/internal/application"
	"pressure-tap-qualification/internal/domain"
)

func (s *Server) HandleDrift(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	out, err := s.app.QueryDrift(r.PathValue("batchID"), application.DriftFilter{TapID: query.Get("tap_id"), SurfaceZone: query.Get("surface_zone"), RoundKind: domain.RoundKind(query.Get("round_kind")), Level: domain.DriftLevel(query.Get("level"))})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comparisons": out})
}

func (s *Server) HandleMeasurementQuality(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	out, err := s.app.QueryMeasurementQuality(r.PathValue("batchID"), application.MeasurementQualityFilter{TapID: query.Get("tap_id"), SurfaceZone: query.Get("surface_zone"), RoundKind: domain.RoundKind(query.Get("round_kind")), Level: domain.MeasurementRiskLevel(query.Get("level"))})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) HandleRevisionHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	from, err := parseOptionalUint(query.Get("from_version"), "from_version")
	if err != nil {
		writeError(w, err)
		return
	}
	to, err := parseOptionalUint(query.Get("to_version"), "to_version")
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.QueryRevisionHistory(r.PathValue("batchID"), application.RevisionHistoryFilter{HistoryType: domain.RevisionHistoryType(query.Get("history_type")), FromVersion: from, ToVersion: to})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func parseOptionalUint(raw, field string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 {
		return 0, domain.NewFieldError(domain.CodeInvalid, field, field+" 必须为正整数")
	}
	return value, nil
}

func (s *Server) HandleDefectTasks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	dueAfter, err := parseOptionalTime(query.Get("due_after"), "due_after")
	if err != nil {
		writeError(w, err)
		return
	}
	dueBefore, err := parseOptionalTime(query.Get("due_before"), "due_before")
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.QueryDefectTasks(r.PathValue("batchID"), application.DefectTaskFilter{TechnicianID: query.Get("technician_id"), Priority: domain.DefectPriority(query.Get("priority")), DueAfter: dueAfter, DueBefore: dueBefore, TaskStatus: query.Get("task_status")})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": out})
}

func auditFilterFromRequest(r *http.Request) (application.AuditFilter, error) {
	query := r.URL.Query()
	page, pageErr := strconv.Atoi(defaultQuery(query.Get("page"), "1"))
	size, sizeErr := strconv.Atoi(defaultQuery(query.Get("page_size"), "20"))
	if pageErr != nil || sizeErr != nil {
		return application.AuditFilter{}, domain.NewError(domain.CodeInvalid, "page 和 page_size 必须为整数")
	}
	from, err := parseOptionalTime(query.Get("from"), "from")
	if err != nil {
		return application.AuditFilter{}, err
	}
	to, err := parseOptionalTime(query.Get("to"), "to")
	if err != nil {
		return application.AuditFilter{}, err
	}
	return application.AuditFilter{Action: query.Get("action"), ActorID: query.Get("actor_id"), From: from, To: to, RelatedID: query.Get("related_id"), Page: page, PageSize: size}, nil
}

func (s *Server) HandleAudit(w http.ResponseWriter, r *http.Request) {
	filter, err := auditFilterFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.QueryAudit(r.PathValue("batchID"), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) HandleAuditDownload(w http.ResponseWriter, r *http.Request) {
	filter, err := auditFilterFromRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	data, digest, err := s.app.DownloadAuditSegment(r.PathValue("batchID"), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-`+r.PathValue("batchID")+`.json"`)
	w.Header().Set("X-Content-SHA256", digest)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) HandleThresholdPreflight(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviseThresholdCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.PreflightThresholdRevision(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleThresholdRevision(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviseThresholdCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.ReviseThresholds(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleTopologyPreflight(w http.ResponseWriter, r *http.Request) {
	revision, err := strconv.ParseUint(r.URL.Query().Get("expected_revision"), 10, 64)
	if err != nil {
		writeError(w, domain.NewError(domain.CodeInvalid, "expected_revision 必须为有效整数"))
		return
	}
	out, err := s.app.PreflightTopology(r.PathValue("batchID"), revision)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleCalibrationInvalidation(w http.ResponseWriter, r *http.Request) {
	var cmd application.CalibrationInvalidationCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	cmd.CalibrationRef = r.PathValue("calibrationRef")
	out, err := s.app.InvalidateCalibration(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleVoidRoundPreflight(w http.ResponseWriter, r *http.Request) {
	revision, err := strconv.ParseUint(r.URL.Query().Get("expected_revision"), 10, 64)
	if err != nil {
		writeError(w, domain.NewError(domain.CodeInvalid, "expected_revision 必须为有效整数"))
		return
	}
	dependencies, err := s.app.PreflightVoidRound(r.PathValue("batchID"), r.PathValue("roundID"), revision)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"round_id": r.PathValue("roundID"), "dependencies": dependencies, "ready": len(dependencies) == 0, "revision": revision})
}
func (s *Server) HandleVoidRound(w http.ResponseWriter, r *http.Request) {
	var cmd application.VoidRoundCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	cmd.RoundID = r.PathValue("roundID")
	out, err := s.app.VoidMeasurementRound(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleBatchTreatmentPreflight(w http.ResponseWriter, r *http.Request) {
	var cmd application.BatchTreatmentCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.PreflightBatchTreatment(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleBatchTreatment(w http.ResponseWriter, r *http.Request) {
	var cmd application.BatchTreatmentCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.TreatDefectsBatch(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
func (s *Server) HandleBatchRetestPreflight(w http.ResponseWriter, r *http.Request) {
	var cmd application.BatchRetestCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if err := requireRetestNeighborResponses(cmd.Rows); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.PreflightBatchRetest(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleBatchRetest(w http.ResponseWriter, r *http.Request) {
	var cmd application.BatchRetestCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if err := requireRetestNeighborResponses(cmd.Rows); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.RecordBatchRetests(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func requireRetestNeighborResponses(rows []application.BatchRetestRow) error {
	for i, row := range rows {
		if row.NeighborResponses == nil {
			return domain.NewFieldError(domain.CodeInvalid, "rows.neighbor_responses", fmt.Sprintf("第 %d 行必须提交 neighbor_responses", i+1))
		}
	}
	return nil
}

func parseOptionalTime(raw, name string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, domain.NewError(domain.CodeInvalid, "%s 必须为 RFC3339 时间", name)
	}
	return &value, nil
}
func (s *Server) HandleCertificateLedger(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, pageErr := strconv.Atoi(defaultQuery(query.Get("page"), "1"))
	size, sizeErr := strconv.Atoi(defaultQuery(query.Get("page_size"), "20"))
	if pageErr != nil || sizeErr != nil {
		writeError(w, domain.NewError(domain.CodeInvalid, "page 和 page_size 必须为整数"))
		return
	}
	from, err := parseOptionalTime(query.Get("issued_from"), "issued_from")
	if err != nil {
		writeError(w, err)
		return
	}
	to, err := parseOptionalTime(query.Get("issued_to"), "issued_to")
	if err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.ListCertificateLedger(application.CertificateLedgerFilter{ModelCode: query.Get("model_code"), ReviewerID: query.Get("reviewer_id"), IssuedFrom: from, IssuedTo: to, DigestPrefix: query.Get("digest_prefix"), Page: page, PageSize: size})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func defaultQuery(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

type batchVerifyRequest struct {
	Items []application.CertificateVerifyItem `json:"items"`
}

func (s *Server) HandleVerifyCertificates(w http.ResponseWriter, r *http.Request) {
	var input batchVerifyRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.VerifyCertificatesContext(r.Context(), input.Items)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
