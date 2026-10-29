package management

import (
	"fmt"
	"strings"
)

type Problem struct {
	Status  int
	Code    string
	Message string
}

func (p *Problem) Error() string { return p.Message }

func problem(status int, code, message string) *Problem {
	return &Problem{Status: status, Code: code, Message: message}
}

type Action struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	RequiresSlot bool   `json:"requiresSlot"`
	Available    bool   `json:"available"`
}

type AudienceReference struct {
	Type       string `json:"type"`
	GroupID    string `json:"groupId"`
	BundleID   string `json:"bundleId,omitempty"`
	Name       string `json:"name"`
	BundleName string `json:"bundleName,omitempty"`
	Available  bool   `json:"available"`
}

type AudienceRef struct {
	Type     string `json:"type"`
	GroupID  string `json:"groupId"`
	BundleID string `json:"bundleId,omitempty"`
}

type Treatment struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	ActionID    string      `json:"actionId"`
	AudienceRef AudienceRef `json:"audienceRef"`
	SlotID      string      `json:"slotId,omitempty"`
}

type Edge struct {
	ID              string `json:"id"`
	FromTreatmentID string `json:"fromTreatmentId"`
	ToTreatmentID   string `json:"toTreatmentId"`
	ConditionLabel  string `json:"conditionLabel"`
}

type Flow struct {
	ID    string `json:"id"`
	Edges []Edge `json:"edges"`
}

type Campaign struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Treatments   []Treatment `json:"treatments"`
	ActivityFlow *Flow       `json:"activityFlow,omitempty"`
}

func (c Campaign) clone() Campaign {
	c.Treatments = append([]Treatment{}, c.Treatments...)
	if c.ActivityFlow != nil {
		flow := *c.ActivityFlow
		flow.Edges = append([]Edge{}, flow.Edges...)
		c.ActivityFlow = &flow
	}
	return c
}

func (r AudienceRef) validShape() bool {
	if strings.TrimSpace(r.GroupID) == "" {
		return false
	}
	switch r.Type {
	case "group":
		return r.BundleID == ""
	case "arm":
		return strings.TrimSpace(r.BundleID) != ""
	default:
		return false
	}
}

func sameRef(a, b AudienceRef) bool {
	return a.Type == b.Type && a.GroupID == b.GroupID && a.BundleID == b.BundleID
}

func validateCampaign(c Campaign, actions []Action, refs []AudienceReference, slots map[string]bool) *Problem {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Name) == "" {
		return problem(400, "missing_field", "活动 ID 和名称必填")
	}
	seen := make(map[string]bool, len(c.Treatments))
	bySlot := map[string][]Treatment{}
	for _, t := range c.Treatments {
		if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.ActionID) == "" {
			return problem(400, "missing_field", "投放项 ID、名称和动作必填")
		}
		if seen[t.ID] {
			return problem(409, "duplicate_treatment", fmt.Sprintf("投放项 ID 重复：%s", t.ID))
		}
		seen[t.ID] = true
		if p := validateTreatment(t, actions, refs, slots); p != nil {
			return p
		}
		if t.SlotID != "" {
			bySlot[t.SlotID] = append(bySlot[t.SlotID], t)
		}
	}
	for slot, sharing := range bySlot {
		for i := range sharing {
			for j := i + 1; j < len(sharing); j++ {
				a, b := sharing[i].AudienceRef, sharing[j].AudienceRef
				if a.Type != "arm" || b.Type != "arm" || a.BundleID != b.BundleID || a.GroupID == b.GroupID {
					return problem(409, "slot_conflict", fmt.Sprintf("展示位 %s 的投放项 %s 与 %s 未能证明人群互斥", slot, sharing[i].ID, sharing[j].ID))
				}
			}
		}
	}
	if c.ActivityFlow == nil {
		return nil
	}
	if strings.TrimSpace(c.ActivityFlow.ID) == "" {
		return problem(400, "missing_field", "活动流程 ID 必填")
	}
	edgeIDs := map[string]bool{}
	for _, edge := range c.ActivityFlow.Edges {
		if strings.TrimSpace(edge.ID) == "" || strings.TrimSpace(edge.ConditionLabel) == "" {
			return problem(400, "missing_field", "活动流程边 ID 和条件必填")
		}
		if edgeIDs[edge.ID] {
			return problem(409, "duplicate_edge", fmt.Sprintf("活动流程边 ID 重复：%s", edge.ID))
		}
		edgeIDs[edge.ID] = true
		if !seen[edge.FromTreatmentID] || !seen[edge.ToTreatmentID] || edge.FromTreatmentID == edge.ToTreatmentID {
			return problem(409, "invalid_edge", fmt.Sprintf("活动流程边 %s 的端点无效", edge.ID))
		}
	}
	return nil
}

func validateTreatment(t Treatment, actions []Action, refs []AudienceReference, slots map[string]bool) *Problem {
	var action *Action
	for i := range actions {
		if actions[i].ID == t.ActionID {
			action = &actions[i]
			break
		}
	}
	if action == nil {
		return problem(422, "unknown_action", fmt.Sprintf("未知动作：%s", t.ActionID))
	}
	if !action.Available {
		return problem(422, "unavailable_action", fmt.Sprintf("动作不可用：%s", t.ActionID))
	}
	if !t.AudienceRef.validShape() {
		return problem(400, "invalid_audience_ref", "人群引用须为普通组 groupId，或包臂 bundleId 与 groupId")
	}
	var ref *AudienceReference
	for i := range refs {
		r := refs[i]
		if sameRef(t.AudienceRef, AudienceRef{Type: r.Type, GroupID: r.GroupID, BundleID: r.BundleID}) {
			ref = &refs[i]
			break
		}
	}
	if ref == nil {
		return problem(422, "unknown_audience", fmt.Sprintf("未知人群引用：%s/%s/%s", t.AudienceRef.Type, t.AudienceRef.BundleID, t.AudienceRef.GroupID))
	}
	if !ref.Available {
		return problem(422, "unavailable_audience", fmt.Sprintf("人群引用不可用：%s/%s/%s", ref.Type, ref.BundleID, ref.GroupID))
	}
	if action.RequiresSlot {
		if t.SlotID == "" {
			return problem(422, "missing_slot", "展示动作必须指定展示位")
		}
		if !slots[t.SlotID] {
			return problem(422, "unknown_slot", fmt.Sprintf("未知展示位：%s", t.SlotID))
		}
	} else if t.SlotID != "" {
		return problem(422, "unexpected_slot", "非展示动作不得指定展示位")
	}
	return nil
}
