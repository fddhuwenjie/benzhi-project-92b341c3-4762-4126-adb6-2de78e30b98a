package web

import (
	"embed"
	"io/fs"
	"net/http"

	"pressure-tap-qualification/internal/application"
)

//go:embed static/*
var assets embed.FS

type Server struct {
	app *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /api/batches", s.HandleListBatches)
	s.mux.HandleFunc("POST /api/batches", s.HandleCreateBatch)
	s.mux.HandleFunc("GET /api/batches/{batchID}", s.HandleGetBatch)
	s.mux.HandleFunc("POST /api/batches/{batchID}/freeze", s.HandleFreezeBaseline)
	s.mux.HandleFunc("POST /api/batches/{batchID}/info/preflight", s.HandleBatchInfoPreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/info/revisions", s.HandleBatchInfoRevision)
	s.mux.HandleFunc("POST /api/batches/{batchID}/baseline/revisions", s.HandleReviseBaseline)
	s.mux.HandleFunc("GET /api/batches/{batchID}/baseline/topology/preflight", s.HandleTopologyPreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/thresholds/preflight", s.HandleThresholdPreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/thresholds/revisions", s.HandleThresholdRevision)
	s.mux.HandleFunc("POST /api/batches/{batchID}/calibration", s.HandleCalibration)
	s.mux.HandleFunc("POST /api/batches/{batchID}/calibrations/{calibrationRef}/invalidate", s.HandleCalibrationInvalidation)
	s.mux.HandleFunc("GET /api/batches/{batchID}/calibration/impacts", s.HandleCalibrationImpacts)
	s.mux.HandleFunc("POST /api/batches/{batchID}/measurements", s.HandleMeasurement)
	s.mux.HandleFunc("GET /api/batches/{batchID}/measurements/{roundID}/void/preflight", s.HandleVoidRoundPreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/measurements/{roundID}/void", s.HandleVoidRound)
	s.mux.HandleFunc("POST /api/batches/{batchID}/measurements/batch/preflight", s.HandleBatchMeasurementPreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/measurements/batch", s.HandleBatchMeasurement)
	s.mux.HandleFunc("POST /api/batches/{batchID}/defects/{defectID}/treatment", s.HandleTreatment)
	s.mux.HandleFunc("POST /api/batches/{batchID}/defects/{defectID}/assignment", s.HandleDefectAssignment)
	s.mux.HandleFunc("GET /api/batches/{batchID}/defect-tasks", s.HandleDefectTasks)
	s.mux.HandleFunc("POST /api/batches/{batchID}/defects/batch-treatment/preflight", s.HandleBatchTreatmentPreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/defects/batch-treatment", s.HandleBatchTreatment)
	s.mux.HandleFunc("POST /api/batches/{batchID}/retests/batch/preflight", s.HandleBatchRetestPreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/retests/batch", s.HandleBatchRetest)
	s.mux.HandleFunc("POST /api/batches/{batchID}/defects/{defectID}/close", s.HandleCloseDefect)
	s.mux.HandleFunc("POST /api/batches/{batchID}/submit", s.HandleSubmit)
	s.mux.HandleFunc("GET /api/batches/{batchID}/submit/preflight", s.HandleSubmitPreflight)
	s.mux.HandleFunc("POST /api/batches/{batchID}/review", s.HandleReview)
	s.mux.HandleFunc("GET /api/batches/{batchID}/reviewer/preflight", s.HandleReviewerPreflight)
	s.mux.HandleFunc("GET /api/batches/{batchID}/drift", s.HandleDrift)
	s.mux.HandleFunc("GET /api/batches/{batchID}/statistics", s.HandleMeasurementQuality)
	s.mux.HandleFunc("GET /api/batches/{batchID}/history", s.HandleRevisionHistory)
	s.mux.HandleFunc("GET /api/batches/{batchID}/audit", s.HandleAudit)
	s.mux.HandleFunc("GET /api/batches/{batchID}/audit/download", s.HandleAuditDownload)
	s.mux.HandleFunc("POST /api/certificates/verify", s.HandleVerifyCertificate)
	s.mux.HandleFunc("GET /api/certificates", s.HandleCertificateLedger)
	s.mux.HandleFunc("POST /api/certificates/verify/batch", s.HandleVerifyCertificates)
	s.mux.HandleFunc("GET /api/batches/{batchID}/certificate", s.HandleDownloadCertificate)
	static, _ := fs.Sub(assets, "static")
	s.mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /", s.HandleWorkbench)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }
