package application

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/evidence"
)

func (s *Service) QueryDrift(batchID string, filter DriftFilter) ([]domain.RoundDriftComparison, error) {
	b, err := s.repo.Get(batchID)
	if err != nil {
		return nil, err
	}
	if filter.TapID != "" && b.Taps[filter.TapID] == nil {
		return nil, domain.NewError(domain.CodeInvalid, "测孔 %s 不存在", filter.TapID)
	}
	if filter.RoundKind != "" && filter.RoundKind != domain.RoundInitial && filter.RoundKind != domain.RoundRetest {
		return nil, domain.NewError(domain.CodeInvalid, "round_kind 无效")
	}
	if filter.Level != "" && filter.Level != domain.DriftNormal && filter.Level != domain.DriftApproaching && filter.Level != domain.DriftWorsening {
		return nil, domain.NewError(domain.CodeInvalid, "level 无效")
	}
	cacheKey := fmt.Sprintf("%s:%d:%s:%s:%s:%s", batchID, b.Revision, filter.TapID, filter.SurfaceZone, filter.RoundKind, filter.Level)
	if cached, ok := s.driftCache[cacheKey]; ok {
		return cloneDriftComparisons(cached), nil
	}
	out := []domain.RoundDriftComparison{}
	for _, item := range b.RoundDriftComparisons() {
		if filter.TapID != "" && item.TapID != filter.TapID || filter.SurfaceZone != "" && item.SurfaceZone != filter.SurfaceZone || filter.RoundKind != "" && item.RoundKind != filter.RoundKind || filter.Level != "" && item.Level != filter.Level {
			continue
		}
		out = append(out, item)
	}
	s.driftCache[cacheKey] = cloneDriftComparisons(out)
	return cloneDriftComparisons(out), nil
}

func cloneDriftComparisons(input []domain.RoundDriftComparison) []domain.RoundDriftComparison {
	out := append([]domain.RoundDriftComparison(nil), input...)
	for i := range out {
		if input[i].PressureRatioDelta != nil {
			value := *input[i].PressureRatioDelta
			out[i].PressureRatioDelta = &value
		}
		if input[i].DecaySecondsDelta != nil {
			value := *input[i].DecaySecondsDelta
			out[i].DecaySecondsDelta = &value
		}
		if input[i].NeighborRatioDelta != nil {
			value := *input[i].NeighborRatioDelta
			out[i].NeighborRatioDelta = &value
		}
	}
	return out
}

func (s *Service) QueryMeasurementQuality(batchID string, filter MeasurementQualityFilter) (domain.MeasurementQualitySnapshot, error) {
	b, err := s.repo.Get(batchID)
	if err != nil {
		return domain.MeasurementQualitySnapshot{}, err
	}
	if filter.TapID != "" && b.Taps[filter.TapID] == nil {
		return domain.MeasurementQualitySnapshot{}, domain.NewFieldError(domain.CodeInvalid, "tap_id", "tap_id 指定的测孔不存在")
	}
	if filter.SurfaceZone != "" {
		found := false
		for _, tap := range b.Taps {
			if tap.SurfaceZone == filter.SurfaceZone {
				found = true
				break
			}
		}
		if !found {
			return domain.MeasurementQualitySnapshot{}, domain.NewFieldError(domain.CodeInvalid, "surface_zone", "surface_zone 指定的区域不存在")
		}
	}
	if filter.RoundKind != "" && filter.RoundKind != domain.RoundInitial && filter.RoundKind != domain.RoundRetest {
		return domain.MeasurementQualitySnapshot{}, domain.NewFieldError(domain.CodeInvalid, "round_kind", "round_kind 必须为 initial 或 retest")
	}
	if filter.Level != "" && filter.Level != domain.MeasurementRiskNormal && filter.Level != domain.MeasurementRiskWarning && filter.Level != domain.MeasurementRiskHigh {
		return domain.MeasurementQualitySnapshot{}, domain.NewFieldError(domain.CodeInvalid, "level", "level 必须为 normal、warning 或 high")
	}
	return b.MeasurementQualitySnapshot(domain.MeasurementQualityFilter{TapID: filter.TapID, SurfaceZone: filter.SurfaceZone, RoundKind: filter.RoundKind, Level: filter.Level}, s.clock()), nil
}

func (s *Service) QueryRevisionHistory(batchID string, filter RevisionHistoryFilter) (domain.RevisionHistoryView, error) {
	b, err := s.repo.Get(batchID)
	if err != nil {
		return domain.RevisionHistoryView{}, err
	}
	return b.RevisionHistory(filter.HistoryType, filter.FromVersion, filter.ToVersion)
}

func (s *Service) QueryDefectTasks(batchID string, filter DefectTaskFilter) ([]DefectTaskView, error) {
	b, err := s.repo.Get(batchID)
	if err != nil {
		return nil, err
	}
	if filter.DueAfter != nil && filter.DueBefore != nil && filter.DueAfter.After(*filter.DueBefore) {
		return nil, domain.NewError(domain.CodeInvalid, "期限起点不能晚于终点")
	}
	validStatus := map[string]bool{"": true, "unassigned": true, "in_progress": true, "due_soon": true, "overdue": true, "completed": true}
	if !validStatus[filter.TaskStatus] {
		return nil, domain.NewError(domain.CodeInvalid, "task_status 无效")
	}
	if filter.Priority != "" && filter.Priority != domain.PriorityLow && filter.Priority != domain.PriorityNormal && filter.Priority != domain.PriorityHigh && filter.Priority != domain.PriorityUrgent {
		return nil, domain.NewError(domain.CodeInvalid, "priority 无效")
	}
	out := []DefectTaskView{}
	for _, item := range projectDefectTasks(b, s.clock()) {
		if filter.TaskStatus != "" && item.Status != filter.TaskStatus {
			continue
		}
		if filter.TechnicianID != "" && (item.Current == nil || item.Current.TechnicianID != filter.TechnicianID) {
			continue
		}
		if filter.Priority != "" && (item.Current == nil || item.Current.Priority != filter.Priority) {
			continue
		}
		if filter.DueAfter != nil && (item.Current == nil || item.Current.DueAt.Before(*filter.DueAfter)) {
			continue
		}
		if filter.DueBefore != nil && (item.Current == nil || item.Current.DueAt.After(*filter.DueBefore)) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

type AuditPage struct {
	BatchID    string                          `json:"batch_id"`
	Filter     AuditFilter                     `json:"filter"`
	Events     []domain.AuditEvent             `json:"events"`
	Total      int                             `json:"total"`
	Page       int                             `json:"page"`
	PageSize   int                             `json:"page_size"`
	Validation evidence.AuditSegmentValidation `json:"validation"`
}

func (s *Service) QueryAudit(batchID string, filter AuditFilter) (AuditPage, error) {
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 20
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 {
		return AuditPage{}, domain.NewError(domain.CodeInvalid, "page 必须大于 0，page_size 必须为 1 到 100")
	}
	if filter.From != nil && filter.To != nil && filter.From.After(*filter.To) {
		return AuditPage{}, domain.NewError(domain.CodeInvalid, "时间起点不能晚于终点")
	}
	b, err := s.repo.Get(batchID)
	if err != nil {
		return AuditPage{}, err
	}
	matched := make([]domain.AuditEvent, 0, len(b.Audit))
	for _, event := range b.Audit {
		if filter.Action != "" && event.Action != filter.Action || filter.ActorID != "" && event.ActorID != filter.ActorID || filter.From != nil && event.At.Before(*filter.From) || filter.To != nil && event.At.After(*filter.To) {
			continue
		}
		if filter.RelatedID != "" && !strings.Contains(event.Summary, filter.RelatedID) && !containsString(event.RelatedIDs, filter.RelatedID) {
			continue
		}
		matched = append(matched, event)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Sequence < matched[j].Sequence })
	total := len(matched)
	start := (filter.Page - 1) * filter.PageSize
	if start > total {
		start = total
	}
	end := start + filter.PageSize
	if end > total {
		end = total
	}
	events := append([]domain.AuditEvent(nil), matched[start:end]...)
	return AuditPage{BatchID: batchID, Filter: filter, Events: events, Total: total, Page: filter.Page, PageSize: filter.PageSize, Validation: evidence.ValidateAuditSegment(b.Audit, events)}, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type auditDownload struct {
	Schema     string                          `json:"schema"`
	BatchID    string                          `json:"batch_id"`
	Filter     AuditFilter                     `json:"filter"`
	Events     []domain.AuditEvent             `json:"events"`
	Validation evidence.AuditSegmentValidation `json:"validation"`
}

func (s *Service) DownloadAuditSegment(batchID string, filter AuditFilter) ([]byte, string, error) {
	page, err := s.QueryAudit(batchID, filter)
	if err != nil {
		return nil, "", err
	}
	document := auditDownload{Schema: "pressure-tap-audit-segment/v1", BatchID: batchID, Filter: page.Filter, Events: page.Events, Validation: page.Validation}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, "", err
	}
	digest, err := evidence.StableDigest(document)
	return data, digest, err
}
