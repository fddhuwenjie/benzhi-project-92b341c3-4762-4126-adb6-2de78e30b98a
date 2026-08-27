package evidence

import (
	"encoding/json"
	"sort"
	"strconv"
	"time"

	"pressure-tap-qualification/internal/domain"
)

type canonicalThresholds struct {
	MinimumPressureRatio      string `json:"minimum_pressure_ratio"`
	MaximumPressureRatio      string `json:"maximum_pressure_ratio"`
	MaximumDecaySeconds       string `json:"maximum_decay_seconds"`
	MaximumNeighborRatio      string `json:"maximum_neighbor_ratio"`
	RequiredConsecutivePasses int    `json:"required_consecutive_passes"`
}

type canonicalCalibration struct {
	Reference         string `json:"reference"`
	InstrumentSummary string `json:"instrument_summary"`
	ValidUntil        string `json:"valid_until"`
	RegisteredAt      string `json:"registered_at"`
	RegisteredBy      string `json:"registered_by"`
}

type canonicalTap struct {
	TapID                    string   `json:"tap_id"`
	Label                    string   `json:"label"`
	SurfaceZone              string   `json:"surface_zone"`
	NominalDiameterMM        string   `json:"nominal_diameter_mm"`
	NeighborTapIDs           []string `json:"neighbor_tap_ids"`
	QualificationStatus      string   `json:"qualification_status"`
	LatestMeasurementRoundID string   `json:"latest_measurement_round_id"`
}

type canonicalRound struct {
	RoundID              string                      `json:"round_id"`
	TapID                string                      `json:"tap_id"`
	RoundKind            string                      `json:"round_kind"`
	SourceRoundID        string                      `json:"source_round_id"`
	DefectID             string                      `json:"defect_id"`
	TreatmentVersionID   string                      `json:"treatment_version_id"`
	CalibrationRef       string                      `json:"calibration_ref"`
	OperatorID           string                      `json:"operator_id"`
	SupplyPressurePA     string                      `json:"supply_pressure_pa"`
	SteadyPressurePA     string                      `json:"steady_pressure_pa"`
	DecaySeconds         string                      `json:"decay_seconds"`
	NeighborResponsePA   string                      `json:"neighbor_response_pa"`
	RecordedAt           string                      `json:"recorded_at"`
	Passed               bool                        `json:"passed"`
	DefectTypes          []string                    `json:"defect_types"`
	PressureRatio        string                      `json:"pressure_ratio"`
	NeighborRatio        string                      `json:"neighbor_ratio"`
	RuleSnapshot         string                      `json:"rule_snapshot"`
	ThresholdVersion     uint64                      `json:"threshold_version"`
	CalibrationInvalid   bool                        `json:"calibration_invalid"`
	VoidedReason         string                      `json:"voided_reason"`
	NeighborResponses    []canonicalNeighborResponse `json:"neighbor_responses"`
	WorstNeighborTapID   string                      `json:"worst_neighbor_tap_id"`
	FrozenTopologyDigest string                      `json:"frozen_topology_digest"`
}

type canonicalNeighborResponse struct {
	TapID      string `json:"tap_id"`
	ResponsePA string `json:"response_pa"`
}

type canonicalDefect struct {
	DefectID             string                `json:"defect_id"`
	TapID                string                `json:"tap_id"`
	DefectType           string                `json:"defect_type"`
	Severity             string                `json:"severity"`
	RuleSnapshot         string                `json:"rule_snapshot"`
	SourceRoundID        string                `json:"source_round_id"`
	Cause                string                `json:"cause"`
	CorrectiveAction     string                `json:"corrective_action"`
	EvidenceDigest       string                `json:"evidence_digest"`
	TechnicianID         string                `json:"technician_id"`
	RetestRoundIDs       []string              `json:"retest_round_ids"`
	Status               string                `json:"status"`
	ClosedAt             string                `json:"closed_at"`
	TreatmentVersions    []canonicalTreatment  `json:"treatment_versions"`
	TriggerNeighborTapID string                `json:"trigger_neighbor_tap_id"`
	TriggerNeighborRatio string                `json:"trigger_neighbor_ratio"`
	FrozenTopologyDigest string                `json:"frozen_topology_digest"`
	Assignments          []canonicalAssignment `json:"assignments"`
	Handovers            []canonicalHandover   `json:"handovers"`
}

type canonicalTreatment struct {
	VersionID        string `json:"version_id"`
	Sequence         int    `json:"sequence"`
	Cause            string `json:"cause"`
	CorrectiveAction string `json:"corrective_action"`
	EvidenceDigest   string `json:"evidence_digest"`
	TechnicianID     string `json:"technician_id"`
	RecordedAt       string `json:"recorded_at"`
	SourceRoundID    string `json:"source_round_id"`
	HandoverNote     string `json:"handover_note"`
}

type canonicalAssignment struct {
	Version      uint64 `json:"version"`
	TechnicianID string `json:"technician_id"`
	DueAt        string `json:"due_at"`
	Priority     string `json:"priority"`
	Reason       string `json:"reason"`
	ActorID      string `json:"actor_id"`
	AssignedAt   string `json:"assigned_at"`
	CompletedAt  string `json:"completed_at"`
}
type canonicalHandover struct {
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note"`
	At   string `json:"at"`
}

type canonicalSnapshot struct {
	Schema                  string                 `json:"schema"`
	BatchID                 string                 `json:"batch_id"`
	ModelCode               string                 `json:"model_code"`
	TestObjective           string                 `json:"test_objective"`
	State                   string                 `json:"state"`
	BaselineRevision        uint64                 `json:"baseline_revision"`
	SnapshotRevision        uint64                 `json:"snapshot_revision"`
	CreatedBy               string                 `json:"created_by"`
	CreatedAt               string                 `json:"created_at"`
	SubmittedAt             string                 `json:"submitted_at"`
	Thresholds              canonicalThresholds    `json:"thresholds"`
	Calibration             canonicalCalibration   `json:"calibration"`
	CalibrationHistory      []canonicalCalibration `json:"calibration_history"`
	Taps                    []canonicalTap         `json:"taps"`
	Rounds                  []canonicalRound       `json:"rounds"`
	Defects                 []canonicalDefect      `json:"defects"`
	MeasurementParticipants []string               `json:"measurement_participants"`
	RemediationParticipants []string               `json:"remediation_participants"`
	ThresholdVersion        uint64                 `json:"threshold_version"`
	TopologyDigest          string                 `json:"topology_digest"`
	EffectiveRoundIDs       []string               `json:"effective_round_ids"`
	SnapshotValidUntil      string                 `json:"snapshot_valid_until"`
	QualificationSummary    string                 `json:"qualification_summary"`
	BatchInfoDigest         string                 `json:"batch_info_digest"`
}

func number(value float64) string {
	return strconv.FormatFloat(value, 'f', 9, 64)
}

func instant(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func participantIDs(participants map[string]bool) []string {
	ids := make([]string, 0, len(participants))
	for id, participated := range participants {
		if participated {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func canonicalCalibrations(history []domain.Calibration) []canonicalCalibration {
	out := make([]canonicalCalibration, 0, len(history))
	for _, calibration := range history {
		out = append(out, canonicalCalibration{Reference: calibration.Reference, InstrumentSummary: calibration.InstrumentSummary, ValidUntil: instant(calibration.ValidUntil), RegisteredAt: instant(calibration.RegisteredAt), RegisteredBy: calibration.RegisteredBy})
	}
	return out
}

func canonicalTaps(batch *domain.QualificationBatch) []canonicalTap {
	ids := make([]string, 0, len(batch.Taps))
	for id := range batch.Taps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	taps := make([]canonicalTap, 0, len(ids))
	for _, id := range ids {
		tap := batch.Taps[id]
		neighbors := append([]string(nil), tap.NeighborTapIDs...)
		sort.Strings(neighbors)
		taps = append(taps, canonicalTap{TapID: tap.TapID, Label: tap.Label, SurfaceZone: tap.SurfaceZone, NominalDiameterMM: number(tap.NominalDiameterMM), NeighborTapIDs: neighbors, QualificationStatus: string(tap.QualificationStatus), LatestMeasurementRoundID: tap.LatestMeasurementRoundID})
	}
	return taps
}

func canonicalRounds(batch *domain.QualificationBatch) []canonicalRound {
	rounds := append([]*domain.MeasurementRound(nil), batch.Rounds...)
	sort.Slice(rounds, func(i, j int) bool { return rounds[i].RoundID < rounds[j].RoundID })
	out := make([]canonicalRound, 0, len(rounds))
	for _, round := range rounds {
		defectTypes := make([]string, len(round.Result.DefectTypes))
		for i, defectType := range round.Result.DefectTypes {
			defectTypes[i] = string(defectType)
		}
		sort.Strings(defectTypes)
		voidedReason := ""
		if round.Voided != nil {
			voidedReason = round.Voided.Reason
		}
		responses := make([]canonicalNeighborResponse, 0, len(round.NeighborResponses))
		for _, item := range round.NeighborResponses {
			responses = append(responses, canonicalNeighborResponse{TapID: item.TapID, ResponsePA: number(item.ResponsePA)})
		}
		sort.Slice(responses, func(i, j int) bool { return responses[i].TapID < responses[j].TapID })
		out = append(out, canonicalRound{RoundID: round.RoundID, TapID: round.TapID, RoundKind: string(round.RoundKind), SourceRoundID: round.SourceRoundID, DefectID: round.DefectID, TreatmentVersionID: round.TreatmentVersionID, CalibrationRef: round.CalibrationRef, OperatorID: round.OperatorID, SupplyPressurePA: number(round.SupplyPressurePA), SteadyPressurePA: number(round.SteadyPressurePA), DecaySeconds: number(round.DecaySeconds), NeighborResponsePA: number(round.NeighborResponsePA), RecordedAt: instant(round.RecordedAt), Passed: round.Result.Passed, DefectTypes: defectTypes, PressureRatio: number(round.Result.PressureRatio), NeighborRatio: number(round.Result.NeighborRatio), RuleSnapshot: round.Result.RuleSnapshot, ThresholdVersion: round.ThresholdVersion, CalibrationInvalid: round.CalibrationInvalid, VoidedReason: voidedReason, NeighborResponses: responses, WorstNeighborTapID: round.WorstNeighborTapID, FrozenTopologyDigest: round.FrozenTopologyDigest})
	}
	return out
}

func canonicalDefects(batch *domain.QualificationBatch) []canonicalDefect {
	ids := make([]string, 0, len(batch.Defects))
	for id := range batch.Defects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]canonicalDefect, 0, len(ids))
	for _, id := range ids {
		defect := batch.Defects[id]
		retests := append([]string(nil), defect.RetestRoundIDs...)
		closedAt := ""
		if defect.ClosedAt != nil {
			closedAt = instant(*defect.ClosedAt)
		}
		versions := make([]canonicalTreatment, 0, len(defect.TreatmentVersions))
		for _, v := range defect.TreatmentVersions {
			versions = append(versions, canonicalTreatment{VersionID: v.VersionID, Sequence: v.Sequence, Cause: v.Cause, CorrectiveAction: v.CorrectiveAction, EvidenceDigest: v.EvidenceDigest, TechnicianID: v.TechnicianID, RecordedAt: instant(v.RecordedAt), SourceRoundID: v.SourceRoundID, HandoverNote: v.HandoverNote})
		}
		assignments := make([]canonicalAssignment, 0, len(defect.Assignments))
		for _, a := range defect.Assignments {
			completed := ""
			if a.CompletedAt != nil {
				completed = instant(*a.CompletedAt)
			}
			assignments = append(assignments, canonicalAssignment{Version: a.Version, TechnicianID: a.TechnicianID, DueAt: instant(a.DueAt), Priority: string(a.Priority), Reason: a.Reason, ActorID: a.ActorID, AssignedAt: instant(a.AssignedAt), CompletedAt: completed})
		}
		handovers := make([]canonicalHandover, 0, len(defect.Handovers))
		for _, h := range defect.Handovers {
			handovers = append(handovers, canonicalHandover{From: h.FromTechnicianID, To: h.ToTechnicianID, Note: h.Note, At: instant(h.At)})
		}
		out = append(out, canonicalDefect{DefectID: defect.DefectID, TapID: defect.TapID, DefectType: string(defect.DefectType), Severity: defect.Severity, RuleSnapshot: defect.RuleSnapshot, SourceRoundID: defect.SourceRoundID, Cause: defect.Cause, CorrectiveAction: defect.CorrectiveAction, EvidenceDigest: defect.EvidenceDigest, TechnicianID: defect.TechnicianID, RetestRoundIDs: retests, Status: string(defect.Status), ClosedAt: closedAt, TreatmentVersions: versions, TriggerNeighborTapID: defect.TriggerNeighborTapID, TriggerNeighborRatio: number(defect.TriggerNeighborRatio), FrozenTopologyDigest: defect.FrozenTopologyDigest, Assignments: assignments, Handovers: handovers})
	}
	return out
}

func SnapshotBytes(batch *domain.QualificationBatch) ([]byte, error) {
	calibration := canonicalCalibration{}
	if batch.Calibration != nil {
		calibration = canonicalCalibration{Reference: batch.Calibration.Reference, InstrumentSummary: batch.Calibration.InstrumentSummary, ValidUntil: instant(batch.Calibration.ValidUntil), RegisteredAt: instant(batch.Calibration.RegisteredAt), RegisteredBy: batch.Calibration.RegisteredBy}
	}
	submittedAt := ""
	if batch.SubmittedAt != nil {
		submittedAt = instant(*batch.SubmittedAt)
	}
	document := canonicalSnapshot{Schema: "pressure-tap-qualification-review-snapshot/v1", BatchID: batch.BatchID, ModelCode: batch.ModelCode, TestObjective: batch.TestObjective, State: string(domain.StateUnderReview), BaselineRevision: batch.BaselineRevision, SnapshotRevision: batch.ReviewSnapshot.Revision, CreatedBy: batch.CreatedBy, CreatedAt: instant(batch.CreatedAt), SubmittedAt: submittedAt, Thresholds: canonicalThresholds{MinimumPressureRatio: number(batch.ThresholdProfile.MinimumPressureRatio), MaximumPressureRatio: number(batch.ThresholdProfile.MaximumPressureRatio), MaximumDecaySeconds: number(batch.ThresholdProfile.MaximumDecaySeconds), MaximumNeighborRatio: number(batch.ThresholdProfile.MaximumNeighborRatio), RequiredConsecutivePasses: batch.ThresholdProfile.RequiredConsecutivePasses}, Calibration: calibration, CalibrationHistory: canonicalCalibrations(batch.CalibrationHistory), Taps: canonicalTaps(batch), Rounds: canonicalRounds(batch), Defects: canonicalDefects(batch), MeasurementParticipants: participantIDs(batch.MeasurementParticipants), RemediationParticipants: participantIDs(batch.RemediationParticipants), ThresholdVersion: batch.ReviewSnapshot.ThresholdVersion, TopologyDigest: batch.FrozenTopologyDigest, EffectiveRoundIDs: append([]string(nil), batch.ReviewSnapshot.EffectiveRoundIDs...), SnapshotValidUntil: instant(batch.ReviewSnapshot.ValidUntil), QualificationSummary: batch.ReviewSnapshot.QualificationSummary, BatchInfoDigest: batch.FrozenBatchInfoDigest}
	return json.Marshal(document)
}

func SnapshotDigest(batch *domain.QualificationBatch) (string, error) {
	data, err := SnapshotBytes(batch)
	if err != nil {
		return "", err
	}
	return sum(data), nil
}
