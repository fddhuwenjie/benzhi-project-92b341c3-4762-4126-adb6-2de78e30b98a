package application

import "pressure-tap-qualification/internal/domain"

type BatchView struct {
	Batch                 *domain.QualificationBatch    `json:"batch"`
	Coverage              float64                       `json:"coverage"`
	OpenDefects           int                           `json:"open_defects"`
	Certificate           *domain.ReleaseCertificate    `json:"certificate,omitempty"`
	Readiness             ReleaseReadiness              `json:"readiness"`
	TapMatrix             []TapMatrixRow                `json:"tap_matrix"`
	Editable              bool                          `json:"editable"`
	DraftDiff             *domain.DraftBaselineDiff     `json:"draft_diff,omitempty"`
	CalibrationHistory    []CalibrationView             `json:"calibration_history"`
	CalibrationImpacts    []CalibrationImpact           `json:"calibration_impacts"`
	ZoneSummaries         []ZoneSummary                 `json:"zone_summaries"`
	DefectDistribution    map[domain.DefectType]int     `json:"defect_distribution"`
	FilterHitCount        int                           `json:"filter_hit_count"`
	DefectViews           []DefectView                  `json:"defect_views"`
	TopologyPreflight     *domain.TopologyPreflight     `json:"topology_preflight,omitempty"`
	ReviewSnapshotStatus  ReviewSnapshotStatus          `json:"review_snapshot_status"`
	FrozenBatchInfoDigest string                        `json:"frozen_batch_info_digest,omitempty"`
	BatchInfoDigest       string                        `json:"batch_info_digest"`
	DriftComparisons      []domain.RoundDriftComparison `json:"drift_comparisons"`
	DefectTasks           []DefectTaskView              `json:"defect_tasks"`
}

type ReleaseReadiness struct {
	Ready            bool     `json:"ready"`
	Blockers         []string `json:"blockers"`
	MeasuredTaps     int      `json:"measured_taps"`
	TotalTaps        int      `json:"total_taps"`
	CalibrationValid bool     `json:"calibration_valid"`
}

type TapMatrixRow struct {
	TapID                     string                     `json:"tap_id"`
	Label                     string                     `json:"label"`
	SurfaceZone               string                     `json:"surface_zone"`
	QualificationStatus       domain.QualificationStatus `json:"qualification_status"`
	LatestRoundID             string                     `json:"latest_round_id"`
	LatestResult              *domain.MeasurementResult  `json:"latest_result,omitempty"`
	OpenDefectIDs             []string                   `json:"open_defect_ids"`
	ConsecutivePassingRetests int                        `json:"consecutive_passing_retests"`
	MeasurementRoundIDs       []string                   `json:"measurement_round_ids"`
}

type CalibrationView struct {
	domain.Calibration
	Status           string                          `json:"status"`
	WarningLevel     string                          `json:"warning_level"`
	RemainingSeconds int64                           `json:"remaining_seconds"`
	Current          bool                            `json:"current"`
	Invalidation     *domain.CalibrationInvalidation `json:"invalidation,omitempty"`
}
type ReviewSnapshotStatus struct {
	Status           string `json:"status"`
	RemainingSeconds int64  `json:"remaining_seconds"`
	Reason           string `json:"reason,omitempty"`
}
type CalibrationImpact struct {
	CalibrationRef string `json:"calibration_ref"`
	TapID          string `json:"tap_id"`
	RoundID        string `json:"round_id"`
}
type ZoneSummary struct {
	SurfaceZone             string                    `json:"surface_zone"`
	FrozenTaps              int                       `json:"frozen_taps"`
	MeasuredTaps            int                       `json:"measured_taps"`
	QualifiedTaps           int                       `json:"qualified_taps"`
	OpenDefects             int                       `json:"open_defects"`
	HighSeverityOpenDefects int                       `json:"high_severity_open_defects"`
	UnmeasuredTaps          int                       `json:"unmeasured_taps"`
	Coverage                float64                   `json:"coverage"`
	DefectCounts            map[domain.DefectType]int `json:"defect_counts"`
}
type RetestProgress struct {
	Required            int    `json:"required"`
	ConsecutivePassed   int    `json:"consecutive_passed"`
	Remaining           int    `json:"remaining"`
	LatestFailedRoundID string `json:"latest_failed_round_id,omitempty"`
	LatestResult        string `json:"latest_result"`
	NextAction          string `json:"next_action"`
	CanClose            bool   `json:"can_close"`
}
type DefectView struct {
	Defect             *domain.DefectCase  `json:"defect"`
	Progress           RetestProgress      `json:"progress"`
	RetestsByTreatment map[string][]string `json:"retests_by_treatment"`
}

type DefectTaskView struct {
	DefectID          string                    `json:"defect_id"`
	TapID             string                    `json:"tap_id"`
	Status            string                    `json:"status"`
	Current           *domain.DefectAssignment  `json:"current,omitempty"`
	AssignmentHistory []domain.DefectAssignment `json:"assignment_history"`
	Handovers         []domain.DefectHandover   `json:"handovers"`
}

type CommandResponse struct {
	BatchID          string                     `json:"batch_id"`
	Revision         uint64                     `json:"revision"`
	State            domain.BatchState          `json:"state"`
	Message          string                     `json:"message"`
	Replayed         bool                       `json:"replayed,omitempty"`
	CreatedDefectIDs []string                   `json:"created_defect_ids,omitempty"`
	Certificate      *domain.ReleaseCertificate `json:"certificate,omitempty"`
	DraftDiffSummary string                     `json:"draft_diff_summary,omitempty"`
	ThresholdDigest  string                     `json:"threshold_digest,omitempty"`
	TopologyDigest   string                     `json:"topology_digest,omitempty"`
	AffectedRoundIDs []string                   `json:"affected_round_ids,omitempty"`
	BatchInfoDigest  string                     `json:"batch_info_digest,omitempty"`
}
