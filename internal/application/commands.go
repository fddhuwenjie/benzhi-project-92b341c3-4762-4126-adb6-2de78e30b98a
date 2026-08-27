package application

import (
	"time"

	"pressure-tap-qualification/internal/domain"
)

type CommandMeta struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision uint64 `json:"expected_revision"`
	ActorID          string `json:"actor_id"`
}
type TapInput struct {
	TapID             string   `json:"tap_id"`
	Label             string   `json:"label"`
	SurfaceZone       string   `json:"surface_zone"`
	NominalDiameterMM float64  `json:"nominal_diameter_mm"`
	NeighborTapIDs    []string `json:"neighbor_tap_ids"`
}
type CreateBatchCommand struct {
	CommandMeta
	BatchID          string                  `json:"batch_id"`
	ModelCode        string                  `json:"model_code"`
	TestObjective    string                  `json:"test_objective"`
	ThresholdProfile domain.ThresholdProfile `json:"threshold_profile"`
	Taps             []TapInput              `json:"taps"`
}
type FreezeCommand struct {
	CommandMeta
	ConfirmedDiffSummary    string   `json:"confirmed_diff_summary"`
	ThresholdDigest         string   `json:"threshold_digest"`
	TopologyDigest          string   `json:"topology_digest"`
	WarningAcknowledgements []string `json:"warning_acknowledgements"`
	BatchInfoDigest         string   `json:"batch_info_digest,omitempty"`
}
type ReviseBatchInfoCommand struct {
	CommandMeta
	ModelCode       string `json:"model_code"`
	TestObjective   string `json:"test_objective"`
	Reason          string `json:"reason"`
	ConfirmedDigest string `json:"confirmed_digest,omitempty"`
}
type ReviseThresholdCommand struct {
	CommandMeta
	ThresholdProfile domain.ThresholdProfile `json:"threshold_profile"`
	Reason           string                  `json:"reason"`
	ConfirmedSummary string                  `json:"confirmed_summary,omitempty"`
}
type ReviseBaselineCommand struct {
	CommandMeta
	Reason string     `json:"reason"`
	Taps   []TapInput `json:"taps"`
}
type CalibrationCommand struct {
	CommandMeta
	Reference         string    `json:"reference"`
	InstrumentSummary string    `json:"instrument_summary"`
	ValidUntil        time.Time `json:"valid_until"`
}
type CalibrationInvalidationCommand struct {
	CommandMeta
	CalibrationRef string    `json:"calibration_ref"`
	DiscoveredAt   time.Time `json:"discovered_at"`
	Reason         string    `json:"reason"`
	EvidenceDigest string    `json:"evidence_digest"`
}
type VoidRoundCommand struct {
	CommandMeta
	RoundID string `json:"round_id"`
	Reason  string `json:"reason"`
}
type MeasurementCommand struct {
	CommandMeta
	RoundID              string                    `json:"round_id"`
	TapID                string                    `json:"tap_id"`
	RoundKind            domain.RoundKind          `json:"round_kind"`
	SourceRoundID        string                    `json:"source_round_id"`
	DefectID             string                    `json:"defect_id"`
	TreatmentVersionID   string                    `json:"treatment_version_id"`
	CalibrationRef       string                    `json:"calibration_ref"`
	SupplyPressurePA     float64                   `json:"supply_pressure_pa"`
	SteadyPressurePA     float64                   `json:"steady_pressure_pa"`
	DecaySeconds         float64                   `json:"decay_seconds"`
	NeighborResponsePA   float64                   `json:"neighbor_response_pa"`
	NeighborResponses    []domain.NeighborResponse `json:"neighbor_responses"`
	ReviewRequirementIDs []string                  `json:"review_requirement_ids,omitempty"`
}
type BatchMeasurementCommand struct {
	CommandMeta
	CalibrationRef string           `json:"calibration_ref"`
	Rows           []MeasurementRow `json:"rows"`
	Confirm        bool             `json:"confirm"`
}
type MeasurementRow struct {
	RoundID            string                    `json:"round_id"`
	TapID              string                    `json:"tap_id"`
	SupplyPressurePA   float64                   `json:"supply_pressure_pa"`
	SteadyPressurePA   float64                   `json:"steady_pressure_pa"`
	DecaySeconds       float64                   `json:"decay_seconds"`
	NeighborResponsePA float64                   `json:"neighbor_response_pa"`
	NeighborResponses  []domain.NeighborResponse `json:"neighbor_responses"`
}
type TreatmentCommand struct {
	CommandMeta
	DefectID             string   `json:"defect_id"`
	Cause                string   `json:"cause"`
	CorrectiveAction     string   `json:"corrective_action"`
	EvidenceDigest       string   `json:"evidence_digest"`
	VersionID            string   `json:"version_id"`
	SourceRoundID        string   `json:"source_round_id"`
	HandoverNote         string   `json:"handover_note,omitempty"`
	ReviewRequirementIDs []string `json:"review_requirement_ids,omitempty"`
}
type AssignDefectCommand struct {
	CommandMeta
	DefectID     string                `json:"defect_id"`
	TechnicianID string                `json:"technician_id"`
	DueAt        time.Time             `json:"due_at"`
	Priority     domain.DefectPriority `json:"priority"`
	Reason       string                `json:"reason"`
}
type BatchTreatmentItem struct {
	DefectID             string   `json:"defect_id"`
	VersionID            string   `json:"version_id"`
	Note                 string   `json:"note,omitempty"`
	ReviewRequirementIDs []string `json:"review_requirement_ids,omitempty"`
}
type BatchTreatmentCommand struct {
	CommandMeta
	JobID            string               `json:"job_id"`
	Cause            string               `json:"cause"`
	CorrectiveAction string               `json:"corrective_action"`
	EvidenceDigest   string               `json:"evidence_digest"`
	Items            []BatchTreatmentItem `json:"items"`
	PreflightDigest  string               `json:"preflight_digest,omitempty"`
	Confirm          bool                 `json:"confirm,omitempty"`
	HandoverNote     string               `json:"handover_note,omitempty"`
}
type BatchRetestRow struct {
	RoundID              string                    `json:"round_id"`
	DefectID             string                    `json:"defect_id"`
	TapID                string                    `json:"tap_id"`
	TreatmentVersionID   string                    `json:"treatment_version_id"`
	SupplyPressurePA     float64                   `json:"supply_pressure_pa"`
	SteadyPressurePA     float64                   `json:"steady_pressure_pa"`
	DecaySeconds         float64                   `json:"decay_seconds"`
	NeighborResponsePA   float64                   `json:"neighbor_response_pa"`
	NeighborResponses    []domain.NeighborResponse `json:"neighbor_responses"`
	ReviewRequirementIDs []string                  `json:"review_requirement_ids,omitempty"`
}
type BatchRetestCommand struct {
	CommandMeta
	CalibrationRef  string           `json:"calibration_ref"`
	Rows            []BatchRetestRow `json:"rows"`
	PreflightDigest string           `json:"preflight_digest,omitempty"`
	Confirm         bool             `json:"confirm,omitempty"`
}
type CloseDefectCommand struct {
	CommandMeta
	DefectID string `json:"defect_id"`
}
type SubmitCommand struct {
	CommandMeta
	PreflightDigest string `json:"preflight_digest,omitempty"`
}
type ReviewCommand struct {
	CommandMeta
	Decision         domain.ReviewDecision     `json:"decision"`
	Note             string                    `json:"note"`
	ReturnTapIDs     []string                  `json:"return_tap_ids,omitempty"`
	Items            []domain.ReviewItemResult `json:"items"`
	DifferenceDigest string                    `json:"difference_digest,omitempty"`
}

type DriftFilter struct {
	TapID       string
	SurfaceZone string
	RoundKind   domain.RoundKind
	Level       domain.DriftLevel
}

type MeasurementQualityFilter struct {
	TapID       string
	SurfaceZone string
	RoundKind   domain.RoundKind
	Level       domain.MeasurementRiskLevel
}

type RevisionHistoryFilter struct {
	HistoryType domain.RevisionHistoryType
	FromVersion uint64
	ToVersion   uint64
}

type DefectTaskFilter struct {
	TechnicianID string
	Priority     domain.DefectPriority
	DueBefore    *time.Time
	DueAfter     *time.Time
	TaskStatus   string
}

type AuditFilter struct {
	Action    string     `json:"action,omitempty"`
	ActorID   string     `json:"actor_id,omitempty"`
	From      *time.Time `json:"from,omitempty"`
	To        *time.Time `json:"to,omitempty"`
	RelatedID string     `json:"related_id,omitempty"`
	Page      int        `json:"page"`
	PageSize  int        `json:"page_size"`
}

type BatchFilter struct {
	SurfaceZone         string
	QualificationStatus domain.QualificationStatus
	DefectType          domain.DefectType
	Blocking            *bool
}

type CertificateLedgerFilter struct {
	ModelCode    string
	ReviewerID   string
	IssuedFrom   *time.Time
	IssuedTo     *time.Time
	DigestPrefix string
	Page         int
	PageSize     int
}

type CertificateVerifyItem struct {
	BatchID string `json:"batch_id"`
	Digest  string `json:"digest,omitempty"`
}
