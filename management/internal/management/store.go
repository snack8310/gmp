package management

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
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
			{ID: "action-rcs", Name: "RCS 消息", Available: true},
			{ID: "action-email", Name: "邮件", Available: true},
			{ID: "action-sms", Name: "短信", Available: true},
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
		if p := validateCampaign(c, s.actions, s.references, s.slots); p != nil {
			panic(p)
		}
		s.campaigns[c.ID] = c
	}
	return s
}

func (s *Store) Actions() []Action {
	return append([]Action{}, s.actions...)
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
		return Campaign{}, problem(http.StatusConflict, "flow_read_only", "活动流程仅可读取，本期不提供边编辑")
	}
	if !sameTreatmentIDs(previous.Treatments, next.Treatments) {
		return Campaign{}, problem(http.StatusConflict, "treatment_set_conflict", "请通过投放项资源创建或删除投放项")
	}
	if p := s.validate(next); p != nil {
		return Campaign{}, p
	}
	s.campaigns[id] = next.clone()
	return next.clone(), nil
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
