package domain

import "time"

type BatchState string

const (
	StateDraft          BatchState = "draft"
	StateBaselineFrozen BatchState = "baseline_frozen"
	StateRemediation    BatchState = "remediation"
	StateUnderReview    BatchState = "under_review"
	StateApproved       BatchState = "approved"
)

type QualificationStatus string

const (
	TapPending   QualificationStatus = "pending"
	TapQualified QualificationStatus = "qualified"
	TapDefective QualificationStatus = "defective"
)

type DefectType string

const (
	DefectMissing   DefectType = "missing"
	DefectBlocked   DefectType = "blocked"
	DefectLeak      DefectType = "leak"
	DefectLag       DefectType = "lag"
	DefectCrosstalk DefectType = "crosstalk"
)

type DefectStatus string

const (
	DefectOpen    DefectStatus = "open"
	DefectTreated DefectStatus = "treated"
	DefectClosed  DefectStatus = "closed"
	DefectVoided  DefectStatus = "source_voided"
)

type RoundKind string

const (
	RoundInitial RoundKind = "initial"
	RoundRetest  RoundKind = "retest"
)

type ReviewDecision string

const (
	DecisionApproved ReviewDecision = "approved"
	DecisionReturned ReviewDecision = "returned"
)

type ThresholdProfile struct {
	MinimumPressureRatio      float64 `json:"minimum_pressure_ratio"`
	MaximumPressureRatio      float64 `json:"maximum_pressure_ratio"`
	MaximumDecaySeconds       float64 `json:"maximum_decay_seconds"`
	MaximumNeighborRatio      float64 `json:"maximum_neighbor_ratio"`
	RequiredConsecutivePasses int     `json:"required_consecutive_passes"`
}

type ThresholdChangeDirection string

const (
	ThresholdStricter  ThresholdChangeDirection = "stricter"
	ThresholdLooser    ThresholdChangeDirection = "looser"
	ThresholdUnchanged ThresholdChangeDirection = "unchanged"
)

type ThresholdFieldChange struct {
	Field     string                   `json:"field"`
	Before    string                   `json:"before"`
	After     string                   `json:"after"`
	Direction ThresholdChangeDirection `json:"direction"`
}

type ThresholdRevision struct {
	Version uint64                 `json:"version"`
	Profile ThresholdProfile       `json:"profile"`
	Reason  string                 `json:"reason"`
	ActorID string                 `json:"actor_id"`
	At      time.Time              `json:"at"`
	Changes []ThresholdFieldChange `json:"changes"`
	Digest  string                 `json:"digest"`
}

type BatchInfoFieldChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type BatchInfoRevision struct {
	Version uint64                 `json:"version"`
	Reason  string                 `json:"reason"`
	ActorID string                 `json:"actor_id"`
	At      time.Time              `json:"at"`
	Changes []BatchInfoFieldChange `json:"changes"`
	Digest  string                 `json:"digest"`
}

func DefaultThresholds() ThresholdProfile {
	return ThresholdProfile{MinimumPressureRatio: .90, MaximumPressureRatio: 1.05, MaximumDecaySeconds: 3, MaximumNeighborRatio: .05, RequiredConsecutivePasses: 2}
}

type Calibration struct {
	Reference         string    `json:"reference"`
	InstrumentSummary string    `json:"instrument_summary"`
	ValidUntil        time.Time `json:"valid_until"`
	RegisteredAt      time.Time `json:"registered_at"`
	RegisteredBy      string    `json:"registered_by"`
}

type CalibrationInvalidation struct {
	CalibrationRef string    `json:"calibration_ref"`
	DiscoveredAt   time.Time `json:"discovered_at"`
	Reason         string    `json:"reason"`
	EvidenceDigest string    `json:"evidence_digest"`
	ActorID        string    `json:"actor_id"`
}

type RoundCorrection struct {
	Reason  string    `json:"reason"`
	ActorID string    `json:"actor_id"`
	At      time.Time `json:"at"`
}

type TapDefinition struct {
	TapID             string   `json:"tap_id"`
	Label             string   `json:"label"`
	SurfaceZone       string   `json:"surface_zone"`
	NominalDiameterMM float64  `json:"nominal_diameter_mm"`
	NeighborTapIDs    []string `json:"neighbor_tap_ids"`
}

type DraftTapChange struct {
	TapID  string         `json:"tap_id"`
	Before *TapDefinition `json:"before,omitempty"`
	After  *TapDefinition `json:"after,omitempty"`
}

type DraftBaselineDiff struct {
	Added    []DraftTapChange `json:"added"`
	Modified []DraftTapChange `json:"modified"`
	Deleted  []DraftTapChange `json:"deleted"`
	Summary  string           `json:"summary"`
}

type DraftBaselineRevision struct {
	Revision uint64            `json:"revision"`
	Reason   string            `json:"reason"`
	ActorID  string            `json:"actor_id"`
	At       time.Time         `json:"at"`
	Diff     DraftBaselineDiff `json:"diff"`
}

type PressureTap struct {
	TapID                    string              `json:"tap_id"`
	BatchID                  string              `json:"batch_id"`
	Label                    string              `json:"label"`
	SurfaceZone              string              `json:"surface_zone"`
	NominalDiameterMM        float64             `json:"nominal_diameter_mm"`
	NeighborTapIDs           []string            `json:"neighbor_tap_ids"`
	QualificationStatus      QualificationStatus `json:"qualification_status"`
	LatestMeasurementRoundID string              `json:"latest_measurement_round_id,omitempty"`
}

type MeasurementRound struct {
	RoundID              string             `json:"round_id"`
	BatchID              string             `json:"batch_id"`
	TapID                string             `json:"tap_id"`
	RoundKind            RoundKind          `json:"round_kind"`
	SourceRoundID        string             `json:"source_round_id,omitempty"`
	DefectID             string             `json:"defect_id,omitempty"`
	TreatmentVersionID   string             `json:"treatment_version_id,omitempty"`
	ReviewRequirementIDs []string           `json:"review_requirement_ids,omitempty"`
	CalibrationRef       string             `json:"calibration_ref"`
	OperatorID           string             `json:"operator_id"`
	SupplyPressurePA     float64            `json:"supply_pressure_pa"`
	SteadyPressurePA     float64            `json:"steady_pressure_pa"`
	DecaySeconds         float64            `json:"decay_seconds"`
	NeighborResponsePA   float64            `json:"neighbor_response_pa"`
	NeighborResponses    []NeighborResponse `json:"neighbor_responses"`
	WorstNeighborTapID   string             `json:"worst_neighbor_tap_id,omitempty"`
	FrozenTopologyDigest string             `json:"frozen_topology_digest,omitempty"`
	RecordedAt           time.Time          `json:"recorded_at"`
	Result               MeasurementResult  `json:"result"`
	ThresholdVersion     uint64             `json:"threshold_version"`
	CalibrationInvalid   bool               `json:"calibration_invalid,omitempty"`
	Voided               *RoundCorrection   `json:"voided,omitempty"`
}

type NeighborResponse struct {
	TapID      string  `json:"tap_id"`
	ResponsePA float64 `json:"response_pa"`
}

type TreatmentVersion struct {
	VersionID            string    `json:"version_id"`
	Sequence             int       `json:"sequence"`
	Cause                string    `json:"cause"`
	CorrectiveAction     string    `json:"corrective_action"`
	EvidenceDigest       string    `json:"evidence_digest"`
	TechnicianID         string    `json:"technician_id"`
	RecordedAt           time.Time `json:"recorded_at"`
	SourceRoundID        string    `json:"source_round_id"`
	JobID                string    `json:"job_id,omitempty"`
	ItemNote             string    `json:"item_note,omitempty"`
	HandoverNote         string    `json:"handover_note,omitempty"`
	ReviewRequirementIDs []string  `json:"review_requirement_ids,omitempty"`
}

type DefectPriority string

const (
	PriorityLow    DefectPriority = "low"
	PriorityNormal DefectPriority = "normal"
	PriorityHigh   DefectPriority = "high"
	PriorityUrgent DefectPriority = "urgent"
)

type DefectAssignment struct {
	Version      uint64         `json:"version"`
	TechnicianID string         `json:"technician_id"`
	DueAt        time.Time      `json:"due_at"`
	Priority     DefectPriority `json:"priority"`
	Reason       string         `json:"reason"`
	ActorID      string         `json:"actor_id"`
	AssignedAt   time.Time      `json:"assigned_at"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
}

type DefectHandover struct {
	FromTechnicianID string    `json:"from_technician_id"`
	ToTechnicianID   string    `json:"to_technician_id"`
	Note             string    `json:"note"`
	At               time.Time `json:"at"`
}

type MeasurementResult struct {
	Passed        bool         `json:"passed"`
	DefectTypes   []DefectType `json:"defect_types"`
	PressureRatio float64      `json:"pressure_ratio"`
	NeighborRatio float64      `json:"neighbor_ratio"`
	RuleSnapshot  string       `json:"rule_snapshot"`
}

type DefectCase struct {
	DefectID             string             `json:"defect_id"`
	BatchID              string             `json:"batch_id"`
	TapID                string             `json:"tap_id"`
	DefectType           DefectType         `json:"defect_type"`
	Severity             string             `json:"severity"`
	RuleSnapshot         string             `json:"rule_snapshot"`
	SourceRoundID        string             `json:"source_round_id"`
	Cause                string             `json:"cause,omitempty"`
	CorrectiveAction     string             `json:"corrective_action,omitempty"`
	EvidenceDigest       string             `json:"evidence_digest,omitempty"`
	TechnicianID         string             `json:"technician_id,omitempty"`
	RetestRoundIDs       []string           `json:"retest_round_ids"`
	TreatmentVersions    []TreatmentVersion `json:"treatment_versions"`
	Assignments          []DefectAssignment `json:"assignments"`
	Handovers            []DefectHandover   `json:"handovers"`
	Status               DefectStatus       `json:"status"`
	ClosedAt             *time.Time         `json:"closed_at,omitempty"`
	TriggerNeighborTapID string             `json:"trigger_neighbor_tap_id,omitempty"`
	TriggerNeighborRatio float64            `json:"trigger_neighbor_ratio,omitempty"`
	FrozenTopologyDigest string             `json:"frozen_topology_digest,omitempty"`
}

type ReviewItemStatus string

const (
	ReviewItemPassed   ReviewItemStatus = "passed"
	ReviewItemReturned ReviewItemStatus = "returned"
)

type ReviewItem struct {
	ItemID string `json:"item_id"`
	Title  string `json:"title"`
}

type ReviewItemResult struct {
	ItemID       string           `json:"item_id"`
	Status       ReviewItemStatus `json:"status"`
	Comment      string           `json:"comment"`
	ReturnTapIDs []string         `json:"return_tap_ids,omitempty"`
	ReviewedAt   time.Time        `json:"reviewed_at"`
}

type ReviewChecklist struct {
	ChecklistID    string             `json:"checklist_id"`
	SnapshotDigest string             `json:"snapshot_digest"`
	Items          []ReviewItem       `json:"items"`
	Results        []ReviewItemResult `json:"results"`
	ReviewerID     string             `json:"reviewer_id,omitempty"`
	Decision       ReviewDecision     `json:"decision,omitempty"`
	CompletedAt    *time.Time         `json:"completed_at,omitempty"`
}

type ReviewRecord struct {
	Checklist              ReviewChecklist               `json:"checklist"`
	Note                   string                        `json:"note"`
	Snapshot               *ReviewSnapshot               `json:"snapshot,omitempty"`
	Facts                  ReviewSnapshotFacts           `json:"facts"`
	RequirementIDs         []string                      `json:"requirement_ids,omitempty"`
	RequirementCompletions []ReviewRequirementCompletion `json:"requirement_completions,omitempty"`
}

type ReviewRequirementCompletion struct {
	RequirementID     string    `json:"requirement_id"`
	CompletedBy       string    `json:"completed_by"`
	CompletedAt       time.Time `json:"completed_at"`
	CompletedRoundIDs []string  `json:"completed_round_ids"`
}

type ReviewRequirementStatus string

const (
	ReviewRequirementOpen      ReviewRequirementStatus = "open"
	ReviewRequirementCompleted ReviewRequirementStatus = "completed"
)

type ReviewRequirement struct {
	RequirementID       string                  `json:"requirement_id"`
	ChecklistID         string                  `json:"checklist_id"`
	SnapshotDigest      string                  `json:"snapshot_digest"`
	ReviewItemID        string                  `json:"review_item_id"`
	SourceChecklistItem string                  `json:"source_checklist_item"`
	TapID               string                  `json:"tap_id"`
	RequirementText     string                  `json:"requirement_text"`
	RelatedDefectIDs    []string                `json:"related_defect_ids"`
	TreatmentVersionID  string                  `json:"treatment_version_id,omitempty"`
	EvidenceDigest      string                  `json:"evidence_digest,omitempty"`
	Status              ReviewRequirementStatus `json:"status"`
	CreatedBy           string                  `json:"created_by"`
	CreatedAt           time.Time               `json:"created_at"`
	CompletedBy         string                  `json:"completed_by,omitempty"`
	CompletedAt         *time.Time              `json:"completed_at,omitempty"`
	CompletedRoundIDs   []string                `json:"completed_round_ids"`
}

type ReviewSnapshotFacts struct {
	EffectiveRoundIDs    []string          `json:"effective_round_ids"`
	VoidedRoundIDs       []string          `json:"voided_round_ids"`
	TreatmentVersionIDs  []string          `json:"treatment_version_ids"`
	DefectStatuses       map[string]string `json:"defect_statuses"`
	CalibrationRefs      []string          `json:"calibration_refs"`
	QualificationSummary string            `json:"qualification_summary"`
}

type ReviewDifference struct {
	Category string `json:"category"`
	Action   string `json:"action"`
	EntityID string `json:"entity_id"`
	TapID    string `json:"tap_id,omitempty"`
	Before   string `json:"before,omitempty"`
	After    string `json:"after,omitempty"`
}

type ReviewDifferencePackage struct {
	FirstSubmission        bool               `json:"first_submission"`
	PreviousSnapshotDigest string             `json:"previous_snapshot_digest,omitempty"`
	CurrentSnapshotDigest  string             `json:"current_snapshot_digest"`
	Differences            []ReviewDifference `json:"differences"`
	Digest                 string             `json:"digest"`
}

type ReviewSnapshot struct {
	Revision              uint64                   `json:"revision"`
	CreatedAt             time.Time                `json:"created_at"`
	Coverage              float64                  `json:"coverage"`
	TapCount              int                      `json:"tap_count"`
	MeasurementCount      int                      `json:"measurement_count"`
	Digest                string                   `json:"digest"`
	CalibrationReferences []string                 `json:"calibration_references"`
	ValidUntil            time.Time                `json:"valid_until"`
	ThresholdVersion      uint64                   `json:"threshold_version"`
	EffectiveRoundIDs     []string                 `json:"effective_round_ids"`
	QualificationSummary  string                   `json:"qualification_summary"`
	AuditHeadDigest       string                   `json:"audit_head_digest"`
	ExpiredReason         string                   `json:"expired_reason,omitempty"`
	BatchInfoDigest       string                   `json:"batch_info_digest"`
	PreflightDigest       string                   `json:"preflight_digest"`
	DifferencePackage     *ReviewDifferencePackage `json:"difference_package,omitempty"`
	Facts                 ReviewSnapshotFacts      `json:"facts"`
}

type ReleaseCertificate struct {
	CertificateID      string         `json:"certificate_id"`
	BatchID            string         `json:"batch_id"`
	ReviewerID         string         `json:"reviewer_id"`
	ReviewDecision     ReviewDecision `json:"review_decision"`
	ReviewNote         string         `json:"review_note"`
	SnapshotRevision   uint64         `json:"snapshot_revision"`
	IssuedAt           time.Time      `json:"issued_at"`
	CanonicalDigest    string         `json:"canonical_digest"`
	AuditHeadDigest    string         `json:"audit_head_digest"`
	ThresholdVersion   uint64         `json:"threshold_version"`
	SnapshotValidUntil time.Time      `json:"snapshot_valid_until"`
}

type AuditEvent struct {
	Sequence       uint64    `json:"sequence"`
	At             time.Time `json:"at"`
	ActorID        string    `json:"actor_id"`
	Action         string    `json:"action"`
	Summary        string    `json:"summary"`
	PreviousDigest string    `json:"previous_digest"`
	Digest         string    `json:"digest"`
	RelatedIDs     []string  `json:"related_ids,omitempty"`
}
