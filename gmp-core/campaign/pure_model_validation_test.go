package campaign_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

func TestPureFlowRejectsIncompleteEdges(t *testing.T) {
	validReceipt := campaign.EdgeSpec{From: "first", To: "next", Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess}
	tests := []struct {
		name      string
		spec      campaign.EdgeSpec
		duplicate bool
		want      error
	}{
		{name: "missing source", spec: campaign.EdgeSpec{To: "next", Kind: campaign.EdgeParallel}, want: campaign.ErrMissingTreatmentID},
		{name: "unknown kind", spec: campaign.EdgeSpec{From: "first", To: "next"}, want: campaign.ErrUnknownEdgeKind},
		{name: "receipt outcome missing", spec: campaign.EdgeSpec{From: "first", To: "next", Kind: campaign.EdgeOnReceipt}, want: campaign.ErrReceiptEdgeMissingOutcome},
		{name: "parallel edge carries outcome", spec: campaign.EdgeSpec{From: "first", To: "next", Kind: campaign.EdgeParallel, When: campaign.OutcomeSuccess}, want: campaign.ErrParallelEdgeHasOutcome},
		{name: "self edge", spec: campaign.EdgeSpec{From: "first", To: "first", Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess}, want: campaign.ErrEdgeToSelf},
		{name: "duplicate edge", spec: validReceipt, duplicate: true, want: campaign.ErrDuplicateEdge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := campaign.FlowSpec{ID: "flow-validation", Edges: []campaign.EdgeSpec{test.spec}}
			if test.duplicate {
				spec.Edges = append(spec.Edges, test.spec)
			}
			if _, err := campaign.NewFlow(spec); !errors.Is(err, test.want) {
				t.Fatalf("NewFlow error = %v, want %v", err, test.want)
			}
		})
	}
	if _, err := campaign.NewFlow(campaign.FlowSpec{Edges: []campaign.EdgeSpec{validReceipt}}); !errors.Is(err, campaign.ErrMissingFlowID) {
		t.Fatalf("missing flow id error = %v, want ErrMissingFlowID", err)
	}
}

func TestNewFlowCopiesEntryTreatmentValues(t *testing.T) {
	treatments := []campaign.TreatmentID{"first"}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "immutable-flow",
		Entries: []campaign.EntrySpec{{Group: "everyone", Kind: campaign.EntryStart, Treatments: treatments}},
	})
	if err != nil {
		t.Fatal(err)
	}
	treatments[0] = "changed"
	routes := flow.EntryRoutes()
	routes[0].Treatments()[0] = "also-changed"
	got, ok := flow.RouteForGroup("everyone")
	if !ok || len(got.Treatments()) != 1 || got.Treatments()[0] != "first" {
		t.Fatalf("flow retained caller mutation: route = %#v, ok=%v", got, ok)
	}
}

func TestPureCampaignRejectsInvalidFlowAssociations(t *testing.T) {
	group := pureCampaignGroup(t, "flow-validation-group", nil)
	first := purePushTreatment(t, "flow-validation-first", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	next := purePushTreatment(t, "flow-validation-next", campaign.OutcomeSuccess, campaign.OutcomeFailure)

	tests := []struct {
		name       string
		flow       campaign.Flow
		treatments []campaign.Treatment
		want       error
	}{
		{
			name:       "no entry routes",
			flow:       mustValidationFlow(t, "flow-no-entry", nil),
			treatments: []campaign.Treatment{first},
			want:       campaign.ErrMissingEntryRoutes,
		},
		{
			name: "unknown edge target",
			flow: mustValidationFlow(t, "flow-unknown-target",
				[]campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}}},
				campaign.EdgeSpec{From: first.ID(), To: "missing-treatment", Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
			),
			treatments: []campaign.Treatment{first},
			want:       campaign.ErrEdgeTreatmentNotInCampaign,
		},
		{
			name: "undeclared outcome",
			flow: mustValidationFlow(t, "flow-undeclared-outcome",
				[]campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}}},
				campaign.EdgeSpec{From: first.ID(), To: next.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.Outcome("blocked")},
			),
			treatments: []campaign.Treatment{first, next},
			want:       campaign.ErrEdgeOutcomeNotDeclared,
		},
		{
			name: "receipt edge targets pull",
			flow: mustValidationFlow(t, "flow-pull-target",
				[]campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}}},
				campaign.EdgeSpec{From: first.ID(), To: "pull-treatment", Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
			),
			treatments: []campaign.Treatment{first, purePullTreatment(t, "pull-treatment")},
			want:       campaign.ErrReceiptEdgeTargetsPull,
		},
		{
			name: "receipt cycle",
			flow: mustValidationFlow(t, "flow-cycle",
				[]campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}}},
				campaign.EdgeSpec{From: first.ID(), To: next.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess},
				campaign.EdgeSpec{From: next.ID(), To: first.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeFailure},
			),
			treatments: []campaign.Treatment{first, next},
			want:       campaign.ErrFlowCycle,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := campaign.NewCampaign(campaign.CampaignSpec{
				ID: campaign.CampaignID(test.name), Priority: campaign.NewPriority(1), Audience: audience.SelectGroup(group),
				Treatments: test.treatments, Flow: test.flow,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("NewCampaign error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPureCampaignRequiresPriority(t *testing.T) {
	group := pureCampaignGroup(t, "unranked-group", nil)
	treatment := purePushTreatment(t, "unranked-treatment")
	flow := mustValidationFlow(t, "unranked-flow", []campaign.EntrySpec{{
		Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{treatment.ID()},
	}})
	_, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "unranked", Audience: audience.SelectGroup(group), Treatments: []campaign.Treatment{treatment}, Flow: flow,
	})
	if !errors.Is(err, campaign.ErrPriorityNotSet) {
		t.Fatalf("missing priority error = %v, want ErrPriorityNotSet", err)
	}
}

func TestPureActionAndTreatmentContractsRejectInvalidValues(t *testing.T) {
	if _, err := campaign.NewAction(campaign.ActionSpec{
		ID: "invalid-push-slot", Direction: campaign.DirectionPush, RequiresSlot: true,
	}); err == nil {
		t.Fatal("a push action that claims a display slot was accepted")
	}
	if _, err := campaign.NewAction(campaign.ActionSpec{
		ID: "incomplete-outcomes", Direction: campaign.DirectionPush,
		Outcomes: []campaign.Outcome{campaign.OutcomeSuccess},
	}); !errors.Is(err, campaign.ErrIncompleteReceiptContract) {
		t.Fatalf("incomplete receipt contract error = %v, want ErrIncompleteReceiptContract", err)
	}

	push := purePushAction(t, "validated-push", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	if _, err := push.Call(nil); !errors.Is(err, campaign.ErrMissingActionParameter) {
		t.Fatalf("missing call parameter error = %v, want ErrMissingActionParameter", err)
	}
	if _, err := push.Call(map[string]string{"copy": "constructed", "unexpected": "value"}); !errors.Is(err, campaign.ErrUnknownActionParameter) {
		t.Fatalf("unknown call parameter error = %v, want ErrUnknownActionParameter", err)
	}

	display, err := campaign.NewAction(campaign.ActionSpec{
		ID: "validated-display", Direction: campaign.DirectionPull, RequiresSlot: true,
		Required: []string{"copy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	call, err := display.Call(map[string]string{"copy": "constructed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "missing-display-slot", Call: call}); !errors.Is(err, campaign.ErrMissingSlotID) {
		t.Fatalf("missing display slot error = %v, want ErrMissingSlotID", err)
	}
	slot, err := campaign.NewSlot("validated-display-slot")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "built-display", Slot: slot.ID(), Call: call}); err != nil {
		t.Fatalf("display treatment with a slot should build: %v", err)
	}
	if _, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "push-with-slot", Slot: slot.ID(), Call: mustCall(t, push)}); !errors.Is(err, campaign.ErrUnexpectedSlotID) {
		t.Fatalf("non-display slot error = %v, want ErrUnexpectedSlotID", err)
	}
}

func TestPureCampaignRejectsUnseparatedTreatmentsOnOneSlot(t *testing.T) {
	slot, err := campaign.NewSlot("contested-slot")
	if err != nil {
		t.Fatal(err)
	}
	group := pureCampaignGroup(t, "contested-group", nil)
	first := pureDisplayTreatment(t, "contested-first", slot)
	second := pureDisplayTreatment(t, "contested-second", slot)
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "contested-flow",
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID(), second.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = campaign.NewCampaign(campaign.CampaignSpec{
		ID: "unseparated-display", Priority: campaign.NewPriority(1), Audience: audience.SelectGroup(group),
		Treatments: []campaign.Treatment{first, second}, Flow: flow,
	})
	if !errors.Is(err, campaign.ErrSlotContestedWithinCampaign) {
		t.Fatalf("same-slot treatments without ordering error = %v, want ErrSlotContestedWithinCampaign", err)
	}
}

func TestPureDecisionRejectsEqualPriority(t *testing.T) {
	slot, err := campaign.NewSlot("priority-contest-slot")
	if err != nil {
		t.Fatal(err)
	}
	group := pureCampaignGroup(t, "priority-contest-group", nil)
	first := pureDisplayCampaign(t, "priority-contest-first", 10, slot, group, "first-treatment", "first-action")
	second := pureDisplayCampaign(t, "priority-contest-second", 10, slot, group, "second-treatment", "second-action")
	_, err = campaign.Decide(
		[]campaign.Campaign{first, second}, []campaign.Slot{slot}, slot.ID(),
		audience.Record{UID: "uid-priority-contest"}, nil,
	)
	if !errors.Is(err, campaign.ErrContentionUndecidable) {
		t.Fatalf("equal-priority decision error = %v, want ErrContentionUndecidable", err)
	}
}

func TestPureDecisionTableRequiresFallbackAndRejectsUnreachableRows(t *testing.T) {
	source, err := audience.NewSource(audience.SourceSpec{
		ID: "table-validation-source", Supply: audience.SupplyPull, Category: audience.CategoryExternal,
	})
	if err != nil {
		t.Fatal(err)
	}
	everyoneDefinition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "table-everyone", Source: source.ID()})
	if err != nil {
		t.Fatal(err)
	}
	condition, err := audience.Attribute("tier", "premium")
	if err != nil {
		t.Fatal(err)
	}
	premiumDefinition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: "table-premium", Source: source.ID(), Conditions: []audience.Condition{condition},
	})
	if err != nil {
		t.Fatal(err)
	}
	slot, err := campaign.NewSlot("table-validation-slot")
	if err != nil {
		t.Fatal(err)
	}
	rows := []campaign.Row{
		{When: audience.Single(everyoneDefinition), Then: "first"},
		{When: audience.Single(premiumDefinition), Then: "second"},
	}
	if _, err := campaign.NewDecisionTable(campaign.TableSpec{Slot: slot.ID(), Rows: rows}); !errors.Is(err, campaign.ErrFallbackNotSet) {
		t.Fatalf("missing fallback error = %v, want ErrFallbackNotSet", err)
	}
	if _, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: slot.ID(), Rows: rows, Fallback: campaign.RouteNowhere(),
	}); !errors.Is(err, campaign.ErrRowNeverReached) {
		t.Fatalf("subsumed-row error = %v, want ErrRowNeverReached", err)
	}
}

func TestPureReceiptRequiresDeclaredOutcomeAndMatchingCampaign(t *testing.T) {
	built, first, _ := pureReceiptCampaign(t)
	execution := campaign.Execution{
		Key: "receipt-validation-key", Campaign: built.ID(), Treatment: first.ID(),
		UID: "uid-receipt-validation", Action: first.Call().Action(), State: campaign.StateDelivered, Attempts: 1,
	}
	if _, err := campaign.ApplyReceipt(built, execution, campaign.Receipt{Key: execution.Key, Arrived: true}); !errors.Is(err, campaign.ErrReceiptOutcomeRequired) {
		t.Fatalf("missing receipt outcome error = %v, want ErrReceiptOutcomeRequired", err)
	}
	if _, err := campaign.ApplyReceipt(built, execution, campaign.Receipt{
		Key: execution.Key, Arrived: true, Outcome: campaign.Outcome("blocked"),
	}); !errors.Is(err, campaign.ErrReceiptOutcomeNotDeclared) {
		t.Fatalf("undeclared receipt outcome error = %v, want ErrReceiptOutcomeNotDeclared", err)
	}
	flow, ok := built.Flow()
	if !ok {
		t.Fatal("receipt campaign has no flow")
	}
	other, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "other-receipt-campaign", Priority: built.Priority(), Audience: built.Audience(),
		Treatments: built.Treatments(), Flow: flow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := campaign.ApplyReceipt(other, execution, campaign.Receipt{
		Key: execution.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	}); !errors.Is(err, campaign.ErrCampaignMismatch) {
		t.Fatalf("foreign campaign receipt error = %v, want ErrCampaignMismatch", err)
	}
}

func purePushTreatment(t *testing.T, id string, outcomes ...campaign.Outcome) campaign.Treatment {
	t.Helper()
	action := purePushAction(t, id+"-action", outcomes...)
	call, err := action.Call(map[string]string{"copy": "constructed"})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: campaign.TreatmentID(id), Call: call})
	if err != nil {
		t.Fatal(err)
	}
	return treatment
}

func purePullTreatment(t *testing.T, id string) campaign.Treatment {
	t.Helper()
	action, err := campaign.NewAction(campaign.ActionSpec{
		ID: campaign.ActionID(id + "-action"), Direction: campaign.DirectionPull, Required: []string{"copy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(map[string]string{"copy": "constructed"})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: campaign.TreatmentID(id), Call: call})
	if err != nil {
		t.Fatal(err)
	}
	return treatment
}

func pureDisplayTreatment(t *testing.T, id string, slot campaign.Slot) campaign.Treatment {
	t.Helper()
	action, err := campaign.NewAction(campaign.ActionSpec{
		ID: campaign.ActionID(id + "-action"), Direction: campaign.DirectionPull,
		RequiresSlot: true, Required: []string{"copy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(map[string]string{"copy": "constructed"})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: campaign.TreatmentID(id), Slot: slot.ID(), Call: call})
	if err != nil {
		t.Fatal(err)
	}
	return treatment
}

func mustCall(t *testing.T, action campaign.Action) campaign.Call {
	t.Helper()
	call, err := action.Call(map[string]string{"copy": "constructed"})
	if err != nil {
		t.Fatal(err)
	}
	return call
}

func mustValidationFlow(t *testing.T, id string, entries []campaign.EntrySpec, edges ...campaign.EdgeSpec) campaign.Flow {
	t.Helper()
	flowSpec := campaign.FlowSpec{ID: campaign.FlowID(id), Entries: entries, Edges: edges}
	flow, err := campaign.NewFlow(flowSpec)
	if err != nil {
		t.Fatalf("NewFlow(%q): %v", id, err)
	}
	return flow
}
