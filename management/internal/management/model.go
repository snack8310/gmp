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
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	RequiresSlot    bool     `json:"requiresSlot"`
	Available       bool     `json:"available"`
	ReceiptOutcomes []string `json:"receiptOutcomes,omitempty"`
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
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	AudienceRefs []AudienceRef `json:"audienceRefs"`
	Treatments   []Treatment   `json:"treatments"`
	ActivityFlow *Flow         `json:"activityFlow,omitempty"`
}

func (c Campaign) clone() Campaign {
	c.AudienceRefs = append([]AudienceRef{}, c.AudienceRefs...)
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
	treatmentByID := make(map[string]Treatment, len(c.Treatments))
	seenAudienceRefs := make(map[string]bool, len(c.AudienceRefs))
	for _, ref := range c.AudienceRefs {
		if !ref.validShape() {
			return problem(400, "invalid_audience_ref", "活动入口须为普通组 groupId，或包臂 bundleId 与 groupId")
		}
		key := audienceReferenceKey(ref)
		if seenAudienceRefs[key] {
			return problem(409, "duplicate_audience_entry", fmt.Sprintf("活动入口重复：%s", ref.GroupID))
		}
		seenAudienceRefs[key] = true
		if p := validateAudienceRef(ref, refs); p != nil {
			return p
		}
	}
	bySlot := map[string][]Treatment{}
	for _, t := range c.Treatments {
		if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.ActionID) == "" {
			return problem(400, "missing_field", "投放项 ID、名称和动作必填")
		}
		if seen[t.ID] {
			return problem(409, "duplicate_treatment", fmt.Sprintf("投放项 ID 重复：%s", t.ID))
		}
		seen[t.ID] = true
		treatmentByID[t.ID] = t
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
	incomingTreatments := map[string]bool{}
	if c.ActivityFlow != nil {
		for _, edge := range c.ActivityFlow.Edges {
			incomingTreatments[edge.ToTreatmentID] = true
		}
	}
	for _, treatment := range c.Treatments {
		if incomingTreatments[treatment.ID] {
			continue
		}
		found := false
		for _, ref := range c.AudienceRefs {
			if sameRef(ref, treatment.AudienceRef) {
				found = true
				break
			}
		}
		if !found {
			return problem(409, "missing_audience_entry", fmt.Sprintf("入口投放项 %s 未绑定活动入口人群", treatment.ID))
		}
	}
	if c.ActivityFlow == nil {
		return nil
	}
	if strings.TrimSpace(c.ActivityFlow.ID) == "" {
		return problem(400, "missing_field", "活动流程 ID 必填")
	}
	edgeIDs := map[string]bool{}
	tupleIDs := map[string]bool{}
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
		tuple := edge.FromTreatmentID + "\x00" + edge.ToTreatmentID + "\x00" + edge.ConditionLabel
		if tupleIDs[tuple] {
			return problem(409, "duplicate_edge", fmt.Sprintf("活动流程边重复：%s -> %s（%s）", edge.FromTreatmentID, edge.ToTreatmentID, edge.ConditionLabel))
		}
		tupleIDs[tuple] = true
		source := treatmentByID[edge.FromTreatmentID]
		var sourceAction *Action
		for i := range actions {
			if actions[i].ID == source.ActionID {
				sourceAction = &actions[i]
				break
			}
		}
		declared := false
		if sourceAction != nil {
			for _, outcome := range sourceAction.ReceiptOutcomes {
				if outcome == edge.ConditionLabel {
					declared = true
					break
				}
			}
		}
		if !declared {
			return problem(422, "invalid_edge_condition", fmt.Sprintf("动作 %s 未声明回执条件：%s", source.ActionID, edge.ConditionLabel))
		}
	}
	if receiptCycle(c.ActivityFlow.Edges) {
		return problem(409, "flow_cycle", "活动流程回执边不能形成循环")
	}
	return nil
}

func audienceReferenceKey(ref AudienceRef) string {
	return ref.Type + ":" + ref.BundleID + ":" + ref.GroupID
}

func validateAudienceRef(ref AudienceRef, refs []AudienceReference) *Problem {
	if !ref.validShape() {
		return problem(400, "invalid_audience_ref", "人群引用须为普通组 groupId，或包臂 bundleId 与 groupId")
	}
	for _, item := range refs {
		if sameRef(ref, AudienceRef{Type: item.Type, GroupID: item.GroupID, BundleID: item.BundleID}) {
			if !item.Available {
				return problem(422, "unavailable_audience", fmt.Sprintf("人群引用不可用：%s/%s/%s", item.Type, item.BundleID, item.GroupID))
			}
			return nil
		}
	}
	return problem(422, "unknown_audience", fmt.Sprintf("未知人群引用：%s/%s/%s", ref.Type, ref.BundleID, ref.GroupID))
}

func ensureEntryAudienceRefs(c *Campaign) {
	if c == nil {
		return
	}
	if c.AudienceRefs == nil {
		c.AudienceRefs = []AudienceRef{}
	}
	incoming := map[string]bool{}
	if c.ActivityFlow != nil {
		for _, edge := range c.ActivityFlow.Edges {
			incoming[edge.ToTreatmentID] = true
		}
	}
	seen := map[string]bool{}
	for _, ref := range c.AudienceRefs {
		seen[audienceReferenceKey(ref)] = true
	}
	for _, treatment := range c.Treatments {
		if incoming[treatment.ID] {
			continue
		}
		key := audienceReferenceKey(treatment.AudienceRef)
		if !seen[key] {
			c.AudienceRefs = append(c.AudienceRefs, treatment.AudienceRef)
			seen[key] = true
		}
	}
}

func receiptCycle(edges []Edge) bool {
	next := map[string][]string{}
	for _, edge := range edges {
		next[edge.FromTreatmentID] = append(next[edge.FromTreatmentID], edge.ToTreatmentID)
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, target := range next[id] {
			if visit(target) {
				return true
			}
		}
		delete(visiting, id)
		visited[id] = true
		return false
	}
	for _, edge := range edges {
		if visit(edge.FromTreatmentID) {
			return true
		}
	}
	return false
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
