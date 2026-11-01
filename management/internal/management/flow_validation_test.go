package management

import (
	"net/http"
	"testing"
)

func TestActivityFlowCanBeUpdatedAndInvalidEdgesLeaveCampaignUnchanged(t *testing.T) {
	store := NewStore()
	current, problem := store.Campaign("campaign-branch")
	if problem != nil {
		t.Fatal(problem)
	}
	expectedEdges := append([]Edge{}, current.ActivityFlow.Edges...)
	current.ActivityFlow.Edges = append(current.ActivityFlow.Edges, Edge{
		ID: "edge-email-sms", FromTreatmentID: "treatment-email", ToTreatmentID: "treatment-sms", ConditionLabel: "成功",
	})
	if _, problem = store.UpdateFlow(current.ID, expectedEdges, *current.ActivityFlow); problem != nil {
		t.Fatalf("valid flow update rejected: %+v", problem)
	}
	stored, problem := store.Campaign(current.ID)
	if problem != nil || len(stored.ActivityFlow.Edges) != 3 {
		t.Fatalf("valid flow update not stored: %+v, problem=%+v", stored.ActivityFlow, problem)
	}
	staleFlow := Flow{ID: stored.ActivityFlow.ID, Edges: expectedEdges}
	if _, problem = store.UpdateFlow(current.ID, expectedEdges, staleFlow); problem == nil || problem.Status != http.StatusConflict || problem.Code != "flow_conflict" {
		t.Fatalf("stale flow update problem = %+v, want flow_conflict", problem)
	}
	storedEdges := append([]Edge{}, stored.ActivityFlow.Edges...)

	invalidCases := []struct {
		name   string
		append Edge
		flowID string
		status int
		code   string
	}{
		{"undeclared outcome", Edge{ID: "edge-unknown", FromTreatmentID: "treatment-email", ToTreatmentID: "treatment-sms", ConditionLabel: "已读"}, "", http.StatusUnprocessableEntity, "invalid_edge_condition"},
		{"duplicate route", Edge{ID: "edge-duplicate", FromTreatmentID: "treatment-rcs", ToTreatmentID: "treatment-email", ConditionLabel: "成功"}, "", http.StatusConflict, "duplicate_edge"},
		{"cycle", Edge{ID: "edge-cycle", FromTreatmentID: "treatment-sms", ToTreatmentID: "treatment-rcs", ConditionLabel: "失败"}, "", http.StatusConflict, "flow_cycle"},
		{"flow ID change", Edge{}, "other-flow", http.StatusConflict, "flow_id_conflict"},
	}
	for _, testCase := range invalidCases {
		t.Run(testCase.name, func(t *testing.T) {
			candidate, problem := store.Campaign(current.ID)
			if problem != nil {
				t.Fatal(problem)
			}
			if testCase.flowID != "" {
				candidate.ActivityFlow.ID = testCase.flowID
			} else {
				candidate.ActivityFlow.Edges = append(candidate.ActivityFlow.Edges, testCase.append)
			}
			if _, problem := store.UpdateFlow(current.ID, storedEdges, *candidate.ActivityFlow); problem == nil || problem.Status != testCase.status || problem.Code != testCase.code {
				t.Fatalf("problem = %+v, want HTTP %d code %s", problem, testCase.status, testCase.code)
			}
			unchanged, readProblem := store.Campaign(current.ID)
			if readProblem != nil || len(unchanged.ActivityFlow.Edges) != 3 {
				t.Fatalf("invalid update changed stored edges: %+v (%v)", unchanged.ActivityFlow, readProblem)
			}
		})
	}
}

func TestReceiptCycle(t *testing.T) {
	if !receiptCycle([]Edge{{FromTreatmentID: "a", ToTreatmentID: "b"}, {FromTreatmentID: "b", ToTreatmentID: "a"}}) {
		t.Fatal("two-edge cycle was not detected")
	}
	if receiptCycle([]Edge{{FromTreatmentID: "a", ToTreatmentID: "b"}, {FromTreatmentID: "a", ToTreatmentID: "c"}}) {
		t.Fatal("parallel branches were mistaken for a cycle")
	}
}

func TestReceiptOutcomesAreOwnedByActionCatalog(t *testing.T) {
	store := NewStore()
	actions := store.Actions()
	var rcs *Action
	for index := range actions {
		if actions[index].ID == "action-rcs" {
			rcs = &actions[index]
		}
	}
	if rcs == nil || len(rcs.ReceiptOutcomes) != 2 || rcs.ReceiptOutcomes[0] != "成功" || rcs.ReceiptOutcomes[1] != "失败" {
		t.Fatalf("RCS receipt outcomes = %#v", rcs)
	}
	rcs.ReceiptOutcomes[0] = "mutated"
	for _, action := range store.Actions() {
		if action.ID == "action-rcs" && action.ReceiptOutcomes[0] == "mutated" {
			t.Fatal("action catalog exposes its mutable receipt outcome slice")
		}
	}
}
