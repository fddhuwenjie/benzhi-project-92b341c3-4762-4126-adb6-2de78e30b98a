package web

import (
	"net/http"
	"strconv"

	"pressure-tap-qualification/internal/application"
	"pressure-tap-qualification/internal/domain"
)

func (s *Server) HandleWorkbench(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := assets.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "工作台资源不可用", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) HandleListBatches(w http.ResponseWriter, r *http.Request) {
	views, err := s.app.ListBatches()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"batches": views})
}
func (s *Server) HandleGetBatch(w http.ResponseWriter, r *http.Request) {
	filter := application.BatchFilter{SurfaceZone: r.URL.Query().Get("surface_zone"), QualificationStatus: domain.QualificationStatus(r.URL.Query().Get("qualification_status")), DefectType: domain.DefectType(r.URL.Query().Get("defect_type"))}
	if raw := r.URL.Query().Get("blocking"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeError(w, domain.NewError(domain.CodeInvalid, "blocking 必须为 true 或 false"))
			return
		}
		filter.Blocking = &value
	}
	view, err := s.app.GetBatchFiltered(r.PathValue("batchID"), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
func (s *Server) HandleReviseBaseline(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviseBaselineCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.ReviseBaseline(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleCreateBatch(w http.ResponseWriter, r *http.Request) {
	var cmd application.CreateBatchCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.CreateBatch(cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}
func (s *Server) HandleCalibrationImpacts(w http.ResponseWriter, r *http.Request) {
	view, err := s.app.GetBatch(r.PathValue("batchID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"calibration_history": view.CalibrationHistory, "impacts": view.CalibrationImpacts})
}
func (s *Server) HandleBatchInfoPreflight(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviseBatchInfoCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.PreflightBatchInfoRevision(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleBatchInfoRevision(w http.ResponseWriter, r *http.Request) {
	var cmd application.ReviseBatchInfoCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if cmd.ConfirmedDigest == "" {
		writeError(w, domain.NewFieldError(domain.CodeInvalid, "confirmed_digest", "批次信息修订必须携带预检摘要"))
		return
	}
	out, err := s.app.ReviseBatchInfoContext(r.Context(), r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleFreezeBaseline(w http.ResponseWriter, r *http.Request) {
	var cmd application.FreezeCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	if cmd.BatchInfoDigest == "" {
		writeError(w, domain.NewFieldError(domain.CodeInvalid, "batch_info_digest", "冻结必须确认最新批次信息摘要"))
		return
	}
	out, err := s.app.FreezeBaseline(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
func (s *Server) HandleCalibration(w http.ResponseWriter, r *http.Request) {
	var cmd application.CalibrationCommand
	if err := decodeJSON(r, &cmd); err != nil {
		writeError(w, err)
		return
	}
	out, err := s.app.RegisterCalibration(r.PathValue("batchID"), cmd)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
