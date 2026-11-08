package management

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"sync"
)

type Store struct {
	mu            sync.RWMutex
	campaigns     map[string]Campaign
	actions       []Action
	references    []AudienceReference
	slots         map[string]bool
	nextCampaign  uint64
	nextTreatment uint64
}

type Repository interface {
	Actions() []Action
	AudienceReferences() ([]AudienceReference, *Problem)
	Campaigns() ([]Campaign, *Problem)
	Campaign(id string) (Campaign, *Problem)
	CreateCampaign(name string) (Campaign, *Problem)
	UpdateCampaign(id string, next Campaign) (Campaign, *Problem)
	UpdateFlow(campaignID string, expectedEntries []EntryRoute, expectedEdges []Edge, flow Flow) (Campaign, *Problem)
	UpdateAudienceSelection(campaignID string, expected, nextSelection *AudienceSelection) (Campaign, *Problem)
	DeleteCampaign(id string) *Problem
	CreateTreatment(campaignID string, treatment Treatment) (Treatment, *Problem)
	UpdateTreatment(campaignID, treatmentID string, treatment Treatment) (Treatment, *Problem)
	DeleteTreatment(campaignID, treatmentID string) *Problem
	UpsertGroupAudienceReference(groupID, name string) (AudienceReference, *Problem)
	DeleteGroupAudienceReference(groupID string) *Problem
	Usages(ref AudienceRef) ([]Usage, *Problem)
}

func NewStore() *Store {
	return newStore(true)
}

func newStore(seedDemo bool) *Store {
	s := &Store{
		campaigns: make(map[string]Campaign),
		actions: []Action{
			{ID: "action-rcs", Name: "RCS 消息", Available: true, ReceiptOutcomes: []string{"成功", "失败"}},
			{ID: "action-email", Name: "邮件", Available: true, ReceiptOutcomes: []string{"成功", "失败"}},
			{ID: "action-sms", Name: "短信", Available: true, ReceiptOutcomes: []string{"成功", "失败"}},
			{ID: "action-show-image", Name: "展示图片", RequiresSlot: true, Available: true},
			{ID: "action-retired", Name: "已停用动作", Available: false},
		},
		slots: map[string]bool{"slot-home-banner": true, "slot-home-hero": true},
	}
	if seedDemo {
		s.references = []AudienceReference{
			{Type: "group", GroupID: "group-new", Name: "新客人群组", Available: true},
			{Type: "group", GroupID: "group-recall", Name: "跨境召回人群组", Available: true},
			{Type: "group", GroupID: "group-removed", Name: "旧人群组", Available: false},
			{Type: "arm", BundleID: "bundle-warmup", BundleName: "预热实验人群包", GroupID: "group-arm-a", Name: "A 臂", Available: true},
			{Type: "arm", BundleID: "bundle-warmup", BundleName: "预热实验人群包", GroupID: "group-arm-b", Name: "B 臂", Available: true},
			{Type: "arm", BundleID: "bundle-warmup", BundleName: "预热实验人群包", GroupID: "group-arm-control", Name: "对照臂", Available: true},
			{Type: "arm", BundleID: "bundle-retired", BundleName: "已停用实验人群包", GroupID: "group-arm-retired", Name: "旧臂", Available: false},
		}
		for _, c := range campaignSeeds() {
			if p := validateCampaign(c, s.actions, s.references, s.slots); p != nil {
				panic(p)
			}
			s.campaigns[c.ID] = c
		}
	}
	return s
}

func (s *Store) Actions() []Action {
	out := append([]Action{}, s.actions...)
	for i := range out {
		out[i].ReceiptOutcomes = append([]string(nil), out[i].ReceiptOutcomes...)
	}
	return out
}

func (s *Store) AudienceReferences() ([]AudienceReference, *Problem) {
	return append([]AudienceReference{}, s.references...), nil
}

func (s *Store) Campaigns() ([]Campaign, *Problem) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Campaign, 0, len(s.campaigns))
	for _, c := range s.campaigns {
		out = append(out, c.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) Campaign(id string) (Campaign, *Problem) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.campaigns[id]
	if !ok {
		return Campaign{}, problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	return c.clone(), nil
}

func (s *Store) CreateCampaign(name string) (Campaign, *Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var id string
	nextID := s.nextCampaign
	for {
		nextID++
		id = fmt.Sprintf("campaign-%d", nextID)
		if _, exists := s.campaigns[id]; !exists {
			break
		}
	}
	c := Campaign{ID: id, Name: name, Treatments: []Treatment{}}
	if p := s.validate(c); p != nil {
		return Campaign{}, p
	}
	s.nextCampaign = nextID
	s.campaigns[id] = c
	return c.clone(), nil
}

func (s *Store) UpdateCampaign(id string, next Campaign) (Campaign, *Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, ok := s.campaigns[id]
	if !ok {
		return Campaign{}, problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	if next.ID != id {
		return Campaign{}, problem(http.StatusConflict, "id_conflict", "活动 ID 不可修改")
	}
	if !reflect.DeepEqual(previous.ActivityFlow, next.ActivityFlow) {
		return Campaign{}, problem(http.StatusConflict, "flow_read_only", "请通过活动流程资源修改回执边")
	}
	if !sameTreatmentIDs(previous.Treatments, next.Treatments) {
		return Campaign{}, problem(http.StatusConflict, "treatment_set_conflict", "请通过投放项资源创建或删除投放项")
	}
	if next.Audience == nil {
		next.Audience = previous.Audience
	}
	if p := s.validate(next); p != nil {
		return Campaign{}, p
	}
	s.campaigns[id] = next.clone()
	return next.clone(), nil
}

func (s *Store) UpdateFlow(campaignID string, expectedEntries []EntryRoute, expectedEdges []Edge, flow Flow) (Campaign, *Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	campaign, ok := s.campaigns[campaignID]
	if !ok {
		return Campaign{}, problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	currentEntries, currentEdges := []EntryRoute{}, []Edge{}
	if campaign.ActivityFlow != nil {
		currentEntries = campaign.ActivityFlow.Entries
		currentEdges = campaign.ActivityFlow.Edges
	}
	if !sameEntryRoutes(currentEntries, expectedEntries) || !sameEdges(currentEdges, expectedEdges) {
		return Campaign{}, problem(http.StatusConflict, "flow_conflict", "活动流程入口或回执边已被其他页面修改，请刷新活动后重试")
	}
	if campaign.ActivityFlow != nil && flow.ID != campaign.ActivityFlow.ID {
		return Campaign{}, problem(http.StatusConflict, "flow_id_conflict", "活动流程 ID 不可修改")
	}
	next := campaign.clone()
	flow.Entries = append([]EntryRoute{}, flow.Entries...)
	for i := range flow.Entries {
		flow.Entries[i].TreatmentIDs = append([]string{}, flow.Entries[i].TreatmentIDs...)
	}
	flow.Edges = append([]Edge{}, flow.Edges...)
	next.ActivityFlow = &flow
	if p := s.validate(next); p != nil {
		return Campaign{}, p
	}
	s.campaigns[campaignID] = next.clone()
	return next.clone(), nil
}

func (s *Store) UpdateAudienceSelection(campaignID string, expected, nextSelection *AudienceSelection) (Campaign, *Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	campaign, ok := s.campaigns[campaignID]
	if !ok {
		return Campaign{}, problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	if !sameSelection(campaign.Audience, expected) {
		return Campaign{}, problem(http.StatusConflict, "audience_conflict", "活动受众已被其他页面修改，请刷新活动后重试")
	}
	next := campaign.clone()
	if !sameSelection(campaign.Audience, nextSelection) && next.ActivityFlow != nil {
		next.ActivityFlow.Entries = []EntryRoute{}
	}
	if nextSelection == nil {
		next.Audience = nil
	} else {
		selection := *nextSelection
		next.Audience = &selection
	}
	if p := s.validate(next); p != nil {
		return Campaign{}, p
	}
	s.campaigns[campaignID] = next.clone()
	return next.clone(), nil
}

func sameEdges(a, b []Edge) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index].ID != b[index].ID || a[index].FromTreatmentID != b[index].FromTreatmentID || a[index].ToTreatmentID != b[index].ToTreatmentID || a[index].ConditionLabel != b[index].ConditionLabel {
			return false
		}
	}
	return true
}

func sameEntryRoutes(a, b []EntryRoute) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if a[index].GroupID != b[index].GroupID || a[index].Kind != b[index].Kind || !reflect.DeepEqual(a[index].TreatmentIDs, b[index].TreatmentIDs) {
			return false
		}
	}
	return true
}

func sameTreatmentIDs(a, b []Treatment) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, t := range a {
		seen[t.ID] = true
	}
	for _, t := range b {
		if !seen[t.ID] {
			return false
		}
	}
	return true
}

func (s *Store) DeleteCampaign(id string) *Problem {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.campaigns[id]; !ok {
		return problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	delete(s.campaigns, id)
	return nil
}

func (s *Store) CreateTreatment(campaignID string, t Treatment) (Treatment, *Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.campaigns[campaignID]
	if !ok {
		return Treatment{}, problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	nextID := s.nextTreatment
	for {
		nextID++
		t.ID = fmt.Sprintf("treatment-%d", nextID)
		if !s.treatmentIDUsed(t.ID) {
			break
		}
	}
	next := c.clone()
	next.Treatments = append(next.Treatments, t)
	if p := s.validate(next); p != nil {
		return Treatment{}, p
	}
	s.nextTreatment = nextID
	s.campaigns[campaignID] = next
	return t, nil
}

func (s *Store) treatmentIDUsed(id string) bool {
	for _, c := range s.campaigns {
		for _, t := range c.Treatments {
			if t.ID == id {
				return true
			}
		}
	}
	return false
}

func (s *Store) UpdateTreatment(campaignID, treatmentID string, t Treatment) (Treatment, *Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.campaigns[campaignID]
	if !ok {
		return Treatment{}, problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	index := treatmentIndex(c, treatmentID)
	if index < 0 {
		return Treatment{}, problem(http.StatusNotFound, "not_found", "投放项不在该活动中")
	}
	if t.ID != treatmentID {
		return Treatment{}, problem(http.StatusConflict, "id_conflict", "投放项 ID 不可修改")
	}
	next := c.clone()
	next.Treatments[index] = t
	if p := s.validate(next); p != nil {
		return Treatment{}, p
	}
	s.campaigns[campaignID] = next
	return t, nil
}

func (s *Store) DeleteTreatment(campaignID, treatmentID string) *Problem {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.campaigns[campaignID]
	if !ok {
		return problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	index := treatmentIndex(c, treatmentID)
	if index < 0 {
		return problem(http.StatusNotFound, "not_found", "投放项不在该活动中")
	}
	if c.ActivityFlow != nil {
		for _, entry := range c.ActivityFlow.Entries {
			for _, id := range entry.TreatmentIDs {
				if id == treatmentID {
					return problem(http.StatusConflict, "entry_conflict", fmt.Sprintf("投放项 %s 被活动流程入口引用", treatmentID))
				}
			}
		}
		for _, edge := range c.ActivityFlow.Edges {
			if edge.FromTreatmentID == treatmentID || edge.ToTreatmentID == treatmentID {
				return problem(http.StatusConflict, "edge_conflict", fmt.Sprintf("投放项 %s 被活动流程边 %s 引用", treatmentID, edge.ID))
			}
		}
	}
	next := c.clone()
	next.Treatments = append(next.Treatments[:index], next.Treatments[index+1:]...)
	if p := s.validate(next); p != nil {
		return p
	}
	s.campaigns[campaignID] = next
	return nil
}

func treatmentIndex(c Campaign, id string) int {
	for i, t := range c.Treatments {
		if t.ID == id {
			return i
		}
	}
	return -1
}

func (s *Store) validate(c Campaign) *Problem {
	return validateCampaign(c, s.actions, s.references, s.slots)
}

func (s *Store) UpsertGroupAudienceReference(groupID, name string) (AudienceReference, *Problem) {
	groupID = strings.TrimSpace(groupID)
	name = strings.TrimSpace(name)
	if !validAudienceReferenceID(groupID) || name == "" || len([]rune(name)) > 48 {
		return AudienceReference{}, problem(http.StatusBadRequest, "invalid_audience_reference", "普通人群引用须有有效 ID 和 1 至 48 个字符的名称")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.references {
		ref := &s.references[i]
		if ref.Type == "group" && ref.GroupID == groupID {
			ref.Name, ref.Available = name, true
			return *ref, nil
		}
	}
	ref := AudienceReference{Type: "group", GroupID: groupID, Name: name, Available: true}
	s.references = append(s.references, ref)
	return ref, nil
}

func validAudienceReferenceID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}

func (s *Store) DeleteGroupAudienceReference(groupID string) *Problem {
	s.mu.Lock()
	defer s.mu.Unlock()
	index := -1
	for i, ref := range s.references {
		if ref.Type == "group" && ref.GroupID == groupID {
			index = i
			break
		}
	}
	if index < 0 {
		return nil
	}
	for _, campaign := range s.campaigns {
		if campaign.Audience != nil && campaign.Audience.Type == "group" && campaign.Audience.GroupID == groupID {
			return problem(http.StatusConflict, "audience_in_use", fmt.Sprintf("普通人群引用 %s 已被活动使用", groupID))
		}
		if campaign.ActivityFlow != nil {
			for _, entry := range campaign.ActivityFlow.Entries {
				if entry.GroupID == groupID {
					return problem(http.StatusConflict, "audience_in_use", fmt.Sprintf("普通人群引用 %s 已被活动流程入口使用", groupID))
				}
			}
		}
	}
	s.references = append(s.references[:index], s.references[index+1:]...)
	return nil
}

func (s *Store) Usages(ref AudienceRef) ([]Usage, *Problem) {
	if !ref.validShape() {
		return nil, problem(http.StatusBadRequest, "invalid_query", "查询需提供 type、groupId；包臂还需 bundleId")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Usage{}
	for _, c := range s.campaigns {
		role := ""
		if c.Audience != nil {
			if c.Audience.Type == "group" && ref.Type == "group" && c.Audience.GroupID == ref.GroupID {
				role = "activity-audience"
			}
			if c.Audience.Type == "bundle" && ref.Type == "arm" && c.Audience.BundleID == ref.BundleID {
				role = "experiment-audience"
			}
		}
		if role == "" && c.ActivityFlow != nil {
			for _, entry := range c.ActivityFlow.Entries {
				if entry.GroupID == ref.GroupID {
					role = "flow-entry"
					break
				}
			}
		}
		if role != "" {
			out = append(out, Usage{CampaignID: c.ID, CampaignName: c.Name, Role: role})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CampaignID < out[j].CampaignID
	})
	return out, nil
}
