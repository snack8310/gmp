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

// AudienceRef identifies one reusable group or one published bundle arm.
type AudienceRef struct {
	Type     string `json:"type"`
	GroupID  string `json:"groupId"`
	BundleID string `json:"bundleId,omitempty"`
}

// AudienceSelection is the one entry audience attached to an activity.
// A group selects one reusable group; a bundle selects all of its arms.
type AudienceSelection struct {
	Type     string `json:"type"`
	GroupID  string `json:"groupId,omitempty"`
	BundleID string `json:"bundleId,omitempty"`
}

type Treatment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ActionID string `json:"actionId"`
	SlotID   string `json:"slotId,omitempty"`
}

type EntryRoute struct {
	GroupID      string   `json:"groupId"`
	Kind         string   `json:"kind"`
	TreatmentIDs []string `json:"treatmentIds"`
}

type Edge struct {
	ID              string `json:"id"`
	FromTreatmentID string `json:"fromTreatmentId"`
	ToTreatmentID   string `json:"toTreatmentId"`
	ConditionLabel  string `json:"conditionLabel"`
}

type Flow struct {
	ID      string       `json:"id"`
	Entries []EntryRoute `json:"entries"`
	Edges   []Edge       `json:"edges"`
}

type Campaign struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Audience     *AudienceSelection `json:"audience,omitempty"`
	Treatments   []Treatment        `json:"treatments"`
	ActivityFlow *Flow              `json:"activityFlow,omitempty"`
}

func (c Campaign) clone() Campaign {
	c.Treatments = append([]Treatment{}, c.Treatments...)
	if c.Audience != nil {
		selection := *c.Audience
		c.Audience = &selection
	}
	if c.ActivityFlow != nil {
		flow := *c.ActivityFlow
		flow.Entries = make([]EntryRoute, len(c.ActivityFlow.Entries))
		for i, entry := range c.ActivityFlow.Entries {
			flow.Entries[i] = EntryRoute{GroupID: entry.GroupID, Kind: entry.Kind, TreatmentIDs: append([]string{}, entry.TreatmentIDs...)}
		}
		flow.Edges = append([]Edge{}, c.ActivityFlow.Edges...)
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

func sameSelection(a, b *AudienceSelection) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Type == b.Type && a.GroupID == b.GroupID && a.BundleID == b.BundleID
}

func validateCampaign(c Campaign, actions []Action, refs []AudienceReference, slots map[string]bool) *Problem {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Name) == "" {
		return problem(400, "missing_field", "活动 ID 和名称必填")
	}
	if c.Audience == nil && (len(c.Treatments) > 0 || c.ActivityFlow != nil && len(c.ActivityFlow.Entries) > 0) {
		return problem(409, "missing_activity_audience", "活动添加投放项或入口路由前，必须先选择一个活动受众")
	}
	if c.Audience != nil {
		if p := validateAudienceSelection(*c.Audience, refs); p != nil {
			return p
		}
	}
	seen := make(map[string]bool, len(c.Treatments))
	treatmentByID := make(map[string]Treatment, len(c.Treatments))
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
		if p := validateTreatment(t, actions, slots); p != nil {
			return p
		}
		if t.SlotID != "" {
			bySlot[t.SlotID] = append(bySlot[t.SlotID], t)
		}
	}
	if c.ActivityFlow != nil {
		if p := validateFlow(c, *c.ActivityFlow, actions, refs, seen, treatmentByID); p != nil {
			return p
		}
	}
	for slot, sharing := range bySlot {
		if c.ActivityFlow == nil || len(c.ActivityFlow.Entries) == 0 {
			continue
		}
		for i := range sharing {
			for j := i + 1; j < len(sharing); j++ {
				if !entryGroupsExclusive(sharing[i].ID, sharing[j].ID, c.ActivityFlow, refs) {
					return problem(409, "slot_conflict", fmt.Sprintf("展示位 %s 的投放项 %s 与 %s 未由同一实验的不同入口臂分开", slot, sharing[i].ID, sharing[j].ID))
				}
			}
		}
	}
	return nil
}

func validateAudienceSelection(selection AudienceSelection, refs []AudienceReference) *Problem {
	switch selection.Type {
	case "group":
		if strings.TrimSpace(selection.GroupID) == "" || selection.BundleID != "" {
			return problem(400, "invalid_audience_selection", "活动受众须为一个普通人群组或一个实验人群包")
		}
		return validateAudienceRef(AudienceRef{Type: "group", GroupID: selection.GroupID}, refs)
	case "bundle":
		if strings.TrimSpace(selection.BundleID) == "" || selection.GroupID != "" {
			return problem(400, "invalid_audience_selection", "实验人群包受众只能指定 bundleId")
		}
		found := false
		for _, ref := range refs {
			if ref.Type != "arm" || ref.BundleID != selection.BundleID {
				continue
			}
			found = true
			if !ref.Available {
				return problem(422, "unavailable_audience", fmt.Sprintf("实验人群包不可用：%s", selection.BundleID))
			}
		}
		if !found {
			return problem(422, "unknown_audience", fmt.Sprintf("未知实验人群包：%s", selection.BundleID))
		}
		return nil
	default:
		return problem(400, "invalid_audience_selection", "活动受众须为一个普通人群组或一个实验人群包")
	}
}

func validateFlow(c Campaign, flow Flow, actions []Action, refs []AudienceReference, treatmentIDs map[string]bool, treatments map[string]Treatment) *Problem {
	if strings.TrimSpace(flow.ID) == "" {
		return problem(400, "missing_field", "活动流程 ID 必填")
	}
	entryGroups := map[string]bool{}
	entryTreatments := map[string]bool{}
	for _, entry := range flow.Entries {
		if strings.TrimSpace(entry.GroupID) == "" || entryGroups[entry.GroupID] {
			return problem(409, "invalid_entry_route", "活动流程入口人群不能为空且不得重复")
		}
		entryGroups[entry.GroupID] = true
		ref, refProblem := routeAudienceRef(c.Audience, entry.GroupID, refs)
		if refProblem != nil {
			return refProblem
		}
		switch entry.Kind {
		case "start":
			for _, id := range entry.TreatmentIDs {
				if strings.TrimSpace(id) == "" || !treatmentIDs[id] || entryTreatments[id] {
					return problem(409, "invalid_entry_route", fmt.Sprintf("入口 %s 的起始投放项无效或重复：%s", entry.GroupID, id))
				}
				entryTreatments[id] = true
			}
		case "control":
			if c.Audience != nil && c.Audience.Type != "bundle" {
				return problem(409, "invalid_control_route", "只有实验人群包的活动入口可以声明对照臂")
			}
			if len(entry.TreatmentIDs) != 0 {
				return problem(409, "invalid_control_route", "对照臂不得绑定起始投放项")
			}
		default:
			return problem(409, "invalid_entry_route", fmt.Sprintf("入口 %s 的类型无效：%s", entry.GroupID, entry.Kind))
		}
		_ = ref
	}

	edgeIDs := map[string]bool{}
	tupleIDs := map[string]bool{}
	incoming := map[string]bool{}
	for _, edge := range flow.Edges {
		if strings.TrimSpace(edge.ID) == "" || strings.TrimSpace(edge.ConditionLabel) == "" {
			return problem(400, "missing_field", "活动流程边 ID 和条件必填")
		}
		if edgeIDs[edge.ID] {
			return problem(409, "duplicate_edge", fmt.Sprintf("活动流程边 ID 重复：%s", edge.ID))
		}
		edgeIDs[edge.ID] = true
		if !treatmentIDs[edge.FromTreatmentID] || !treatmentIDs[edge.ToTreatmentID] || edge.FromTreatmentID == edge.ToTreatmentID {
			return problem(409, "invalid_edge", fmt.Sprintf("活动流程边 %s 的端点无效", edge.ID))
		}
		if entryTreatments[edge.ToTreatmentID] {
			return problem(409, "entry_receipt_conflict", fmt.Sprintf("投放项 %s 同时是活动入口和回执目标", edge.ToTreatmentID))
		}
		incoming[edge.ToTreatmentID] = true
		tuple := edge.FromTreatmentID + "\x00" + edge.ToTreatmentID + "\x00" + edge.ConditionLabel
		if tupleIDs[tuple] {
			return problem(409, "duplicate_edge", fmt.Sprintf("活动流程边重复：%s -> %s（%s）", edge.FromTreatmentID, edge.ToTreatmentID, edge.ConditionLabel))
		}
		tupleIDs[tuple] = true
		source := treatments[edge.FromTreatmentID]
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
	for id := range incoming {
		if entryTreatments[id] {
			return problem(409, "entry_receipt_conflict", fmt.Sprintf("投放项 %s 同时是活动入口和回执目标", id))
		}
	}
	if receiptCycle(flow.Edges) {
		return problem(409, "flow_cycle", "活动流程回执边不能形成循环")
	}
	return nil
}

func routeAudienceRef(selection *AudienceSelection, groupID string, refs []AudienceReference) (*AudienceReference, *Problem) {
	var wanted AudienceRef
	if selection == nil {
		for i := range refs {
			if refs[i].GroupID == groupID {
				if !refs[i].Available {
					return nil, problem(422, "unavailable_audience", fmt.Sprintf("流程入口不可用：%s", groupID))
				}
				return &refs[i], nil
			}
		}
		return nil, problem(422, "unknown_audience", fmt.Sprintf("未知流程入口人群：%s", groupID))
	}
	if selection.Type == "group" {
		if selection.GroupID != groupID {
			return nil, problem(409, "entry_outside_audience", fmt.Sprintf("流程入口 %s 不属于活动受众", groupID))
		}
		wanted = AudienceRef{Type: "group", GroupID: groupID}
	} else if selection.Type == "bundle" {
		wanted = AudienceRef{Type: "arm", BundleID: selection.BundleID, GroupID: groupID}
	}
	for i := range refs {
		if sameRef(wanted, AudienceRef{Type: refs[i].Type, BundleID: refs[i].BundleID, GroupID: refs[i].GroupID}) {
			if !refs[i].Available {
				return nil, problem(422, "unavailable_audience", fmt.Sprintf("流程入口不可用：%s", groupID))
			}
			return &refs[i], nil
		}
	}
	return nil, problem(409, "entry_outside_audience", fmt.Sprintf("流程入口 %s 不属于活动受众", groupID))
}

func validateAudienceRef(ref AudienceRef, refs []AudienceReference) *Problem {
	if !ref.validShape() {
		return problem(400, "invalid_audience_ref", "人群引用须为普通组 groupId，或包臂 bundleId 与 groupId")
	}
	for _, item := range refs {
		if sameRef(ref, AudienceRef{Type: item.Type, GroupID: item.GroupID, BundleID: item.BundleID}) {
			if !item.Available {
				return problem(422, "unavailable_audience", fmt.Sprintf("人群引用不可用：%s/%s/%s", ref.Type, ref.BundleID, ref.GroupID))
			}
			return nil
		}
	}
	return problem(422, "unknown_audience", fmt.Sprintf("未知人群引用：%s/%s/%s", ref.Type, ref.BundleID, ref.GroupID))
}

func validateTreatment(t Treatment, actions []Action, slots map[string]bool) *Problem {
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

func entryGroupsExclusive(left, right string, flow *Flow, refs []AudienceReference) bool {
	if flow == nil {
		return false
	}
	leftGroups, rightGroups := []string{}, []string{}
	for _, entry := range flow.Entries {
		for _, id := range entry.TreatmentIDs {
			if id == left {
				leftGroups = append(leftGroups, entry.GroupID)
			}
			if id == right {
				rightGroups = append(rightGroups, entry.GroupID)
			}
		}
	}
	if len(leftGroups) == 0 || len(rightGroups) == 0 {
		return false
	}
	for _, a := range leftGroups {
		for _, b := range rightGroups {
			var refA, refB *AudienceReference
			for i := range refs {
				if refs[i].GroupID == a && refs[i].Type == "arm" {
					refA = &refs[i]
				}
				if refs[i].GroupID == b && refs[i].Type == "arm" {
					refB = &refs[i]
				}
			}
			if refA == nil || refB == nil || refA.BundleID != refB.BundleID || a == b {
				return false
			}
		}
	}
	return true
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

type Usage struct {
	CampaignID   string `json:"campaignId"`
	CampaignName string `json:"campaignName"`
	Role         string `json:"role"`
}
