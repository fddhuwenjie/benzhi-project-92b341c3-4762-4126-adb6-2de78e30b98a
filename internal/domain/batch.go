package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type QualificationBatch struct {
	BatchID                  string                    `json:"batch_id"`
	ModelCode                string                    `json:"model_code"`
	TestObjective            string                    `json:"test_objective"`
	State                    BatchState                `json:"state"`
	ThresholdProfile         ThresholdProfile          `json:"threshold_profile"`
	ThresholdHistory         []ThresholdRevision       `json:"threshold_history"`
	FrozenThresholdVersion   uint64                    `json:"frozen_threshold_version,omitempty"`
	FrozenThresholdDigest    string                    `json:"frozen_threshold_digest,omitempty"`
	FrozenTopologyDigest     string                    `json:"frozen_topology_digest,omitempty"`
	FrozenBaselineDigest     string                    `json:"frozen_baseline_digest,omitempty"`
	FrozenDraftVersion       uint64                    `json:"frozen_draft_version,omitempty"`
	FrozenBatchInfoDigest    string                    `json:"frozen_batch_info_digest,omitempty"`
	BatchInfoHistory         []BatchInfoRevision       `json:"batch_info_history"`
	TopologyAcknowledgements []TopologyAcknowledgement `json:"topology_acknowledgements"`
	BaselineRevision         uint64                    `json:"baseline_revision"`
	DraftRevision            uint64                    `json:"draft_revision"`
	DraftBaselineDiff        *DraftBaselineDiff        `json:"draft_baseline_diff,omitempty"`
	DraftHistory             []DraftBaselineRevision   `json:"draft_history"`
	Revision                 uint64                    `json:"revision"`
	CreatedBy                string                    `json:"created_by"`
	CreatedAt                time.Time                 `json:"created_at"`
	SubmittedAt              *time.Time                `json:"submitted_at,omitempty"`
	ApprovedAt               *time.Time                `json:"approved_at,omitempty"`
	Calibration              *Calibration              `json:"calibration,omitempty"`
	CalibrationHistory       []Calibration             `json:"calibration_history"`
	CalibrationInvalidations []CalibrationInvalidation `json:"calibration_invalidations"`
	Taps                     map[string]*PressureTap   `json:"taps"`
	Rounds                   []*MeasurementRound       `json:"rounds"`
	Defects                  map[string]*DefectCase    `json:"defects"`
	MeasurementParticipants  map[string]bool           `json:"measurement_participants"`
	RemediationParticipants  map[string]bool           `json:"remediation_participants"`
	ReviewSnapshot           *ReviewSnapshot           `json:"review_snapshot,omitempty"`
	ReviewChecklist          *ReviewChecklist          `json:"review_checklist,omitempty"`
	ReviewHistory            []ReviewRecord            `json:"review_history"`
	ReviewRequirements       []ReviewRequirement       `json:"review_requirements"`
	Certificate              *ReleaseCertificate       `json:"certificate,omitempty"`
	Audit                    []AuditEvent              `json:"audit"`
}

func NewBatch(id, model, objective, creator string, profile ThresholdProfile, now time.Time) (*QualificationBatch, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(model) == "" || strings.TrimSpace(objective) == "" || strings.TrimSpace(creator) == "" {
		return nil, NewError(CodeInvalid, "批次编号、模型编号、试验目标和建立人均不能为空")
	}
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	b := &QualificationBatch{BatchID: id, ModelCode: strings.TrimSpace(model), TestObjective: strings.TrimSpace(objective), State: StateDraft, ThresholdProfile: profile, Revision: 1, CreatedBy: creator, CreatedAt: now.UTC(), Taps: map[string]*PressureTap{}, Defects: map[string]*DefectCase{}, MeasurementParticipants: map[string]bool{}, RemediationParticipants: map[string]bool{}}
	initial, err := NewInitialThresholdRevision(profile, creator, now)
	if err != nil {
		return nil, err
	}
	b.ThresholdHistory = []ThresholdRevision{initial}
	return b, nil
}

func (b *QualificationBatch) AddTap(tap PressureTap) error {
	if b.State != StateDraft {
		return NewError(CodeState, "基线冻结后不得增删测孔")
	}
	if strings.TrimSpace(tap.TapID) == "" || strings.TrimSpace(tap.Label) == "" || strings.TrimSpace(tap.SurfaceZone) == "" || tap.NominalDiameterMM <= 0 {
		return NewError(CodeInvalid, "测孔编号、标签、区域和标称孔径必须有效")
	}
	if _, exists := b.Taps[tap.TapID]; exists {
		return NewError(CodeConflict, "测孔 %s 已存在", tap.TapID)
	}
	tap.BatchID = b.BatchID
	tap.QualificationStatus = TapPending
	b.Taps[tap.TapID] = &tap
	return nil
}

func (b *QualificationBatch) FreezeBaseline() error {
	return b.FreezeBaselineConfirmed("")
}

func (b *QualificationBatch) SetCalibration(c Calibration, actor string, now time.Time) error {
	if b.State == StateDraft || b.State == StateUnderReview || b.State == StateApproved {
		return NewError(CodeState, "当前状态不能登记校准")
	}
	if strings.TrimSpace(c.Reference) == "" || strings.TrimSpace(c.InstrumentSummary) == "" {
		return NewError(CodeInvalid, "校准引用和器具摘要不能为空")
	}
	if c.ValidUntil.IsZero() || !c.ValidUntil.After(now) {
		return NewError(CodeInvalid, "校准有效期必须晚于当前时间")
	}
	for _, prior := range b.CalibrationHistory {
		if prior.Reference != c.Reference {
			continue
		}
		if prior.InstrumentSummary == c.InstrumentSummary && prior.ValidUntil.Equal(c.ValidUntil) {
			return NewError(CodeConflict, "校准引用 %s 已登记相同内容", c.Reference)
		}
		return NewError(CodeConflict, "校准引用 %s 已存在但内容冲突", c.Reference)
	}
	c.RegisteredAt = now.UTC()
	c.RegisteredBy = actor
	b.Calibration = &c
	b.CalibrationHistory = append(b.CalibrationHistory, c)
	return nil
}

func (b *QualificationBatch) Coverage() float64 {
	if len(b.Taps) == 0 {
		return 0
	}
	measured := 0
	for _, tap := range b.Taps {
		if tap.LatestMeasurementRoundID != "" {
			measured++
		}
	}
	return float64(measured) / float64(len(b.Taps))
}

func (b *QualificationBatch) OpenDefects() int {
	n := 0
	for _, d := range b.Defects {
		if d.Status != DefectClosed && d.Status != DefectVoided {
			n++
		}
	}
	return n
}

func (b *QualificationBatch) EligibleForReview(now time.Time) error {
	if b.State != StateBaselineFrozen && b.State != StateRemediation {
		return NewError(CodeState, "批次状态不允许送审")
	}
	if blockers := b.QualificationBlockers(now); len(blockers) > 0 {
		return NewError(CodeUnqualified, "%s", strings.Join(blockers, "；"))
	}
	return nil
}

func (b *QualificationBatch) SnapshotDigest() string {
	parts := []string{b.BatchID, b.ModelCode, b.TestObjective, fmt.Sprintf("%d", b.Revision)}
	ids := make([]string, 0, len(b.Taps))
	for id := range b.Taps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		t := b.Taps[id]
		parts = append(parts, id, t.Label, t.SurfaceZone, string(t.QualificationStatus), t.LatestMeasurementRoundID)
	}
	for _, r := range b.Rounds {
		parts = append(parts, r.RoundID, r.TapID, string(r.RoundKind), fmt.Sprintf("%.6f/%.6f/%.6f/%.6f", r.SupplyPressurePA, r.SteadyPressurePA, r.DecaySeconds, r.NeighborResponsePA))
	}
	s := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(s[:])
}
