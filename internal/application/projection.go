package application

import (
	"sort"
	"time"

	"pressure-tap-qualification/internal/domain"
)

func projectBatch(batch *domain.QualificationBatch, certificate *domain.ReleaseCertificate, now time.Time) BatchView {
	return projectBatchFiltered(batch, certificate, now, BatchFilter{})
}

func projectBatchFiltered(batch *domain.QualificationBatch, certificate *domain.ReleaseCertificate, now time.Time, filter BatchFilter) BatchView {
	rows := projectTapMatrix(batch)
	rows = filterRows(batch, rows, filter)
	view := BatchView{Batch: batch, Coverage: batch.Coverage(), OpenDefects: batch.OpenDefects(), Certificate: certificate, Readiness: projectReadiness(batch, now), TapMatrix: rows, Editable: batch.State == domain.StateDraft, DraftDiff: batch.DraftBaselineDiff, CalibrationHistory: projectCalibrations(batch, now), CalibrationImpacts: projectCalibrationImpacts(batch), ZoneSummaries: projectZones(batch), DefectDistribution: projectDefectDistribution(batch), FilterHitCount: len(rows), DefectViews: projectDefects(batch), ReviewSnapshotStatus: projectSnapshotStatus(batch, now), FrozenBatchInfoDigest: batch.FrozenBatchInfoDigest, BatchInfoDigest: batch.BatchInfoDigest(), DriftComparisons: batch.RoundDriftComparisons(), DefectTasks: projectDefectTasks(batch, now)}
	if batch.State == domain.StateDraft {
		topology := batch.TopologyPreflight()
		view.TopologyPreflight = &topology
	}
	return view
}

func projectDefectTasks(batch *domain.QualificationBatch, now time.Time) []DefectTaskView {
	ids := make([]string, 0, len(batch.Defects))
	for id := range batch.Defects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]DefectTaskView, 0, len(ids))
	for _, id := range ids {
		defect := batch.Defects[id]
		status := "unassigned"
		var current *domain.DefectAssignment
		if assignment := defect.CurrentAssignment(); assignment != nil {
			copy := *assignment
			current = &copy
			status = "in_progress"
			if assignment.CompletedAt != nil || defect.Status == domain.DefectClosed {
				status = "completed"
			} else if !assignment.DueAt.After(now) {
				status = "overdue"
			} else if assignment.DueAt.Sub(now) <= 24*time.Hour {
				status = "due_soon"
			}
		} else if defect.Status == domain.DefectClosed {
			status = "completed"
		}
		out = append(out, DefectTaskView{DefectID: id, TapID: defect.TapID, Status: status, Current: current, AssignmentHistory: append([]domain.DefectAssignment(nil), defect.Assignments...), Handovers: append([]domain.DefectHandover(nil), defect.Handovers...)})
	}
	return out
}

func projectReadiness(batch *domain.QualificationBatch, now time.Time) ReleaseReadiness {
	readiness := ReleaseReadiness{TotalTaps: len(batch.Taps), Blockers: batch.QualificationBlockers(now)}
	for _, tap := range batch.Taps {
		if tap.LatestMeasurementRoundID != "" {
			readiness.MeasuredTaps++
		}
	}
	readiness.CalibrationValid = batch.Calibration != nil && batch.Calibration.ValidUntil.After(now)
	readiness.Ready = len(readiness.Blockers) == 0 && (batch.State == domain.StateBaselineFrozen || batch.State == domain.StateRemediation || batch.State == domain.StateUnderReview || batch.State == domain.StateApproved)
	return readiness
}

func projectTapMatrix(batch *domain.QualificationBatch) []TapMatrixRow {
	byID := map[string]*domain.MeasurementRound{}
	for _, round := range batch.Rounds {
		byID[round.RoundID] = round
	}
	rows := make([]TapMatrixRow, 0, len(batch.Taps))
	for _, tapID := range domain.SortedTapIDs(batch.Taps) {
		tap := batch.Taps[tapID]
		row := TapMatrixRow{TapID: tap.TapID, Label: tap.Label, SurfaceZone: tap.SurfaceZone, QualificationStatus: tap.QualificationStatus, LatestRoundID: tap.LatestMeasurementRoundID, OpenDefectIDs: []string{}, MeasurementRoundIDs: []string{}}
		if round := byID[tap.LatestMeasurementRoundID]; round != nil {
			result := round.Result
			row.LatestResult = &result
		}
		for _, round := range batch.Rounds {
			if round.TapID == tapID {
				row.MeasurementRoundIDs = append(row.MeasurementRoundIDs, round.RoundID)
			}
		}
		for defectID, defect := range batch.Defects {
			if defect.TapID == tapID && defect.Status != domain.DefectClosed {
				row.OpenDefectIDs = append(row.OpenDefectIDs, defectID)
				passes := batch.ConsecutivePassingRetests(defectID)
				if passes > row.ConsecutivePassingRetests {
					row.ConsecutivePassingRetests = passes
				}
			}
		}
		sort.Strings(row.OpenDefectIDs)
		rows = append(rows, row)
	}
	return rows
}

func projectCalibrations(batch *domain.QualificationBatch, now time.Time) []CalibrationView {
	out := make([]CalibrationView, 0, len(batch.CalibrationHistory))
	for i := len(batch.CalibrationHistory) - 1; i >= 0; i-- {
		c := batch.CalibrationHistory[i]
		remaining := c.ValidUntil.Sub(now)
		status := "replaced"
		current := batch.Calibration != nil && batch.Calibration.Reference == c.Reference
		warning := "none"
		if !c.ValidUntil.After(now) {
			status = "expired"
			warning = "critical"
		} else if current {
			status = "current"
			if remaining <= 24*time.Hour {
				status = "expiring"
				warning = "warning"
			}
		}
		var invalidation *domain.CalibrationInvalidation
		for j := range batch.CalibrationInvalidations {
			if batch.CalibrationInvalidations[j].CalibrationRef == c.Reference {
				copy := batch.CalibrationInvalidations[j]
				invalidation = &copy
				status = "invalidated"
				warning = "critical"
				current = false
				break
			}
		}
		out = append(out, CalibrationView{Calibration: c, Status: status, WarningLevel: warning, RemainingSeconds: int64(remaining.Seconds()), Current: current, Invalidation: invalidation})
	}
	return out
}

func projectSnapshotStatus(batch *domain.QualificationBatch, now time.Time) ReviewSnapshotStatus {
	if batch.ReviewSnapshot == nil {
		return ReviewSnapshotStatus{Status: "none"}
	}
	remaining := batch.ReviewSnapshot.ValidUntil.Sub(now)
	status := "valid"
	reason := ""
	if remaining <= 0 {
		status = "expired"
		reason = "送审快照所引用的校准已经失效"
	} else if remaining <= 24*time.Hour {
		status = "expiring"
	}
	return ReviewSnapshotStatus{Status: status, RemainingSeconds: int64(remaining.Seconds()), Reason: reason}
}
func projectCalibrationImpacts(batch *domain.QualificationBatch) []CalibrationImpact {
	out := []CalibrationImpact{}
	current := ""
	if batch.Calibration != nil {
		current = batch.Calibration.Reference
	}
	for _, r := range batch.Rounds {
		if r.CalibrationRef != current {
			out = append(out, CalibrationImpact{CalibrationRef: r.CalibrationRef, TapID: r.TapID, RoundID: r.RoundID})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TapID == out[j].TapID {
			return out[i].RoundID < out[j].RoundID
		}
		return out[i].TapID < out[j].TapID
	})
	return out
}
func projectDefectDistribution(batch *domain.QualificationBatch) map[domain.DefectType]int {
	out := map[domain.DefectType]int{}
	for _, d := range batch.Defects {
		if d.Status != domain.DefectClosed {
			out[d.DefectType]++
		}
	}
	return out
}
func projectZones(batch *domain.QualificationBatch) []ZoneSummary {
	by := map[string]*ZoneSummary{}
	for _, t := range batch.Taps {
		z := by[t.SurfaceZone]
		if z == nil {
			z = &ZoneSummary{SurfaceZone: t.SurfaceZone, DefectCounts: map[domain.DefectType]int{}}
			by[t.SurfaceZone] = z
		}
		z.FrozenTaps++
		if t.LatestMeasurementRoundID != "" {
			z.MeasuredTaps++
		} else {
			z.UnmeasuredTaps++
		}
		if t.QualificationStatus == domain.TapQualified {
			z.QualifiedTaps++
		}
	}
	for _, d := range batch.Defects {
		if d.Status == domain.DefectClosed {
			continue
		}
		z := by[batch.Taps[d.TapID].SurfaceZone]
		z.OpenDefects++
		z.DefectCounts[d.DefectType]++
		if d.Severity == "high" {
			z.HighSeverityOpenDefects++
		}
	}
	out := make([]ZoneSummary, 0, len(by))
	for _, z := range by {
		if z.FrozenTaps > 0 {
			z.Coverage = float64(z.MeasuredTaps) / float64(z.FrozenTaps)
		}
		out = append(out, *z)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].HighSeverityOpenDefects != out[j].HighSeverityOpenDefects {
			return out[i].HighSeverityOpenDefects > out[j].HighSeverityOpenDefects
		}
		if out[i].OpenDefects != out[j].OpenDefects {
			return out[i].OpenDefects > out[j].OpenDefects
		}
		if out[i].UnmeasuredTaps != out[j].UnmeasuredTaps {
			return out[i].UnmeasuredTaps > out[j].UnmeasuredTaps
		}
		return out[i].SurfaceZone < out[j].SurfaceZone
	})
	return out
}
func filterRows(batch *domain.QualificationBatch, rows []TapMatrixRow, f BatchFilter) []TapMatrixRow {
	out := []TapMatrixRow{}
	for _, r := range rows {
		if f.SurfaceZone != "" && r.SurfaceZone != f.SurfaceZone {
			continue
		}
		if f.QualificationStatus != "" && r.QualificationStatus != f.QualificationStatus {
			continue
		}
		matching := false
		for _, d := range batch.Defects {
			if d.TapID == r.TapID && d.Status != domain.DefectClosed && (f.DefectType == "" || d.DefectType == f.DefectType) {
				matching = true
			}
		}
		if f.DefectType != "" && !matching {
			continue
		}
		if f.Blocking != nil {
			blocking := len(r.OpenDefectIDs) > 0 || r.LatestRoundID == ""
			if blocking != *f.Blocking {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}
func projectDefects(batch *domain.QualificationBatch) []DefectView {
	ids := make([]string, 0, len(batch.Defects))
	for id := range batch.Defects {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]DefectView, 0, len(ids))
	for _, id := range ids {
		d := batch.Defects[id]
		passes := batch.ConsecutivePassingRetests(id)
		p := RetestProgress{Required: batch.ThresholdProfile.RequiredConsecutivePasses, ConsecutivePassed: passes, Remaining: batch.ThresholdProfile.RequiredConsecutivePasses - passes, NextAction: "record_treatment"}
		if p.Remaining < 0 {
			p.Remaining = 0
		}
		by := map[string][]string{}
		for _, rid := range d.RetestRoundIDs {
			for _, r := range batch.Rounds {
				if r.RoundID == rid {
					by[r.TreatmentVersionID] = append(by[r.TreatmentVersionID], rid)
					if r.Result.Passed {
						p.LatestResult = "passed"
					} else {
						p.LatestResult = "failed"
						p.LatestFailedRoundID = rid
					}
				}
			}
		}
		if d.Status == domain.DefectClosed {
			p.NextAction = "none"
		} else if len(d.TreatmentVersions) > 0 {
			if p.Remaining == 0 {
				p.CanClose = true
				p.NextAction = "close"
			} else {
				p.NextAction = "retest"
			}
		}
		out = append(out, DefectView{Defect: d, Progress: p, RetestsByTreatment: by})
	}
	return out
}
