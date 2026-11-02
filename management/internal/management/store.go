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

func NewStore() *Store {
	s := &Store{
		campaigns: make(map[string]Campaign),
		actions: []Action{
			{ID: "action-rcs", Name: "RCS 消息", Available: true, ReceiptOutcomes: []string{"成功", "失败"}},
			{ID: "action-email", Name: "邮件", Available: true, ReceiptOutcomes: []string{"成功", "失败"}},
			{ID: "action-sms", Name: "短信", Available: true, ReceiptOutcomes: []string{"成功", "失败"}},
			{ID: "action-show-image", Name: "展示图片", RequiresSlot: true, Available: true},
			{ID: "action-retired", Name: "已停用动作", Available: false},
		},
		references: []AudienceReference{
			{Type: "group", GroupID: "group-new", Name: "新客人群组", Available: true},
			{Type: "group", GroupID: "group-recall", Name: "跨境召回人群组", Available: true},
			{Type: "group", GroupID: "group-removed", Name: "旧人群组", Available: false},
			{Type: "arm", BundleID: "bundle-warmup", BundleName: "预热实验人群包", GroupID: "group-arm-a", Name: "A 臂", Available: true},
			{Type: "arm", BundleID: "bundle-warmup", BundleName: "预热实验人群包", GroupID: "group-arm-b", Name: "B 臂", Available: true},
			{Type: "arm", BundleID: "bundle-warmup", BundleName: "预热实验人群包", GroupID: "group-arm-control", Name: "对照臂", Available: true},
			{Type: "arm", BundleID: "bundle-retired", BundleName: "已停用实验人群包", GroupID: "group-arm-retired", Name: "旧臂", Available: false},
		},
		slots: map[string]bool{"slot-home-banner": true, "slot-home-hero": true},
	}
	for _, c := range campaignSeeds() {
		ensureEntryAudienceRefs(&c)
		if p := validateCampaign(c, s.actions, s.references, s.slots); p != nil {
			panic(p)
		}
		s.campaigns[c.ID] = c
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

func (s *Store) AudienceReferences() []AudienceReference {
	return append([]AudienceReference{}, s.references...)
}

func (s *Store) Campaigns() []Campaign {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Campaign, 0, len(s.campaigns))
	for _, c := range s.campaigns {
		out = append(out, c.clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
	c := Campaign{ID: id, Name: name, AudienceRefs: []AudienceRef{}, Treatments: []Treatment{}}
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
	if next.AudienceRefs == nil {
		next.AudienceRefs = append([]AudienceRef{}, previous.AudienceRefs...)
	}
	ensureEntryAudienceRefs(&next)
	if p := s.validate(next); p != nil {
		return Campaign{}, p
	}
	s.campaigns[id] = next.clone()
	return next.clone(), nil
}

func (s *Store) UpdateFlow(campaignID string, expectedEdges []Edge, flow Flow) (Campaign, *Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	campaign, ok := s.campaigns[campaignID]
	if !ok {
		return Campaign{}, problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	currentEdges := []Edge{}
	if campaign.ActivityFlow != nil {
		currentEdges = campaign.ActivityFlow.Edges
	}
	if !sameEdges(currentEdges, expectedEdges) {
		return Campaign{}, problem(http.StatusConflict, "flow_conflict", "活动流程已被其他页面修改，请刷新活动后重试")
	}
	if campaign.ActivityFlow != nil && flow.ID != campaign.ActivityFlow.ID {
		return Campaign{}, problem(http.StatusConflict, "flow_id_conflict", "活动流程 ID 不可修改")
	}
	next := campaign.clone()
	next.ActivityFlow = &flow
	ensureEntryAudienceRefs(&next)
	if p := s.validate(next); p != nil {
		return Campaign{}, p
	}
	s.campaigns[campaignID] = next.clone()
	return next.clone(), nil
}

func (s *Store) UpdateAudienceRefs(campaignID string, expectedRefs, refs []AudienceRef) (Campaign, *Problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	campaign, ok := s.campaigns[campaignID]
	if !ok {
		return Campaign{}, problem(http.StatusNotFound, "not_found", "活动不存在")
	}
	if !sameAudienceRefs(campaign.AudienceRefs, expectedRefs) {
		return Campaign{}, problem(http.StatusConflict, "audience_refs_conflict", "活动受众入口已被其他页面修改，请刷新活动后重试")
	}
	next := campaign.clone()
	next.AudienceRefs = append([]AudienceRef{}, refs...)
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

func sameAudienceRefs(a, b []AudienceRef) bool {
	if len(a) != len(b) {
		return false
	}
	for index := range a {
		if !sameRef(a[index], b[index]) {
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
	ensureEntryAudienceRefs(&next)
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
	ensureEntryAudienceRefs(&next)
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
		for _, edge := range c.ActivityFlow.Edges {
			if edge.FromTreatmentID == treatmentID || edge.ToTreatmentID == treatmentID {
				return problem(http.StatusConflict, "edge_conflict", fmt.Sprintf("投放项 %s 被活动流程边 %s 引用", treatmentID, edge.ID))
			}
		}
	}
	next := c.clone()
	next.Treatments = append(next.Treatments[:index], next.Treatments[index+1:]...)
	ensureEntryAudienceRefs(&next)
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
		for _, ref := range campaign.AudienceRefs {
			if ref.Type == "group" && ref.GroupID == groupID {
				return problem(http.StatusConflict, "audience_in_use", fmt.Sprintf("普通人群引用 %s 已被活动入口使用", groupID))
			}
		}
		for _, treatment := range campaign.Treatments {
			if treatment.AudienceRef.Type == "group" && treatment.AudienceRef.GroupID == groupID {
				return problem(http.StatusConflict, "audience_in_use", fmt.Sprintf("普通人群引用 %s 已被投放项使用", groupID))
			}
		}
	}
	s.references = append(s.references[:index], s.references[index+1:]...)
	return nil
}

type Usage struct {
	CampaignID    string `json:"campaignId"`
	CampaignName  string `json:"campaignName"`
	TreatmentID   string `json:"treatmentId"`
	TreatmentName string `json:"treatmentName"`
}

func (s *Store) Usages(ref AudienceRef) ([]Usage, *Problem) {
	if !ref.validShape() {
		return nil, problem(http.StatusBadRequest, "invalid_query", "查询需提供 type、groupId；包臂还需 bundleId")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []Usage{}
	for _, c := range s.campaigns {
		for _, t := range c.Treatments {
			if sameRef(ref, t.AudienceRef) {
				out = append(out, Usage{c.ID, c.Name, t.ID, t.Name})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CampaignID == out[j].CampaignID {
			return out[i].TreatmentID < out[j].TreatmentID
		}
		return out[i].CampaignID < out[j].CampaignID
	})
	return out, nil
}
