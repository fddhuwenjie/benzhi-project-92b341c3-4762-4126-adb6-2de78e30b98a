package domain

import (
	"strings"
	"time"
)

func (d *DefectCase) Assign(technician string, dueAt time.Time, priority DefectPriority, reason, actor string, now time.Time) (DefectAssignment, error) {
	if d.Status == DefectClosed || d.Status == DefectVoided {
		return DefectAssignment{}, NewError(CodeState, "已关闭或失效缺陷不能分派")
	}
	if strings.TrimSpace(technician) == "" || strings.TrimSpace(reason) == "" || strings.TrimSpace(actor) == "" {
		return DefectAssignment{}, NewError(CodeInvalid, "责任技师、分派原因和操作人不能为空")
	}
	if !dueAt.After(now) {
		return DefectAssignment{}, NewError(CodeInvalid, "计划完成时间必须晚于服务端当前时间")
	}
	if priority == "" {
		priority = PriorityNormal
	}
	if priority != PriorityLow && priority != PriorityNormal && priority != PriorityHigh && priority != PriorityUrgent {
		return DefectAssignment{}, NewError(CodeInvalid, "优先级必须为 low、normal、high 或 urgent")
	}
	assignment := DefectAssignment{Version: uint64(len(d.Assignments) + 1), TechnicianID: strings.TrimSpace(technician), DueAt: dueAt.UTC(), Priority: priority, Reason: strings.TrimSpace(reason), ActorID: actor, AssignedAt: now.UTC()}
	d.Assignments = append(d.Assignments, assignment)
	return assignment, nil
}

func (d *DefectCase) CurrentAssignment() *DefectAssignment {
	if len(d.Assignments) == 0 {
		return nil
	}
	return &d.Assignments[len(d.Assignments)-1]
}

func (d *DefectCase) CheckTreatmentAssignee(technician, handoverNote string, now time.Time) error {
	current := d.CurrentAssignment()
	if current == nil || current.TechnicianID == technician {
		return nil
	}
	if strings.TrimSpace(handoverNote) == "" {
		return NewError(CodeInvalid, "实际技师与当前责任人不一致，必须填写接手说明")
	}
	d.Handovers = append(d.Handovers, DefectHandover{FromTechnicianID: current.TechnicianID, ToTechnicianID: technician, Note: strings.TrimSpace(handoverNote), At: now.UTC()})
	return nil
}

func (d *DefectCase) CompleteAssignment(now time.Time) {
	if len(d.Assignments) == 0 {
		return
	}
	completed := now.UTC()
	d.Assignments[len(d.Assignments)-1].CompletedAt = &completed
}
