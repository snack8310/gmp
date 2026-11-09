package campaign_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestDecideUsesExplicitFactsAndPriority(t *testing.T) {
	assignment := pureCampaignAssignment(t, "decision-assignment")
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	share, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	group := pureCampaignGroup(t, "decision-group", share)
	slot, err := campaign.NewSlot("homepage")
	if err != nil {
		t.Fatal(err)
	}
	low := pureDisplayCampaign(t, "low-priority", 10, slot, group, "low-treatment", "low-action")
	high := pureDisplayCampaign(t, "high-priority", 20, slot, group, "high-treatment", "high-action")

	decision, err := campaign.Decide(
		[]campaign.Campaign{low, high}, []campaign.Slot{slot}, slot.ID(),
		audience.Record{UID: "uid-decision"},
		audience.AssignmentFacts{{Definition: assignment, Bucket: second}},
	)
	if err != nil {
		t.Fatalf("deciding with explicit facts: %v", err)
	}
	if !decision.Won || decision.Award.Campaign != high.ID() || decision.Award.Treatment != "high-treatment" {
		t.Fatalf("decision = %#v, want high-priority treatment", decision)
	}
	if len(decision.Losses) != 1 || decision.Losses[0].Campaign != low.ID() || decision.Losses[0].Reason != campaign.LossOnPriority {
		t.Fatalf("losses = %#v, want the lower-priority campaign", decision.Losses)
	}
}

func TestPlanEntryIncludesPlacementsAndDerivedKey(t *testing.T) {
	assignment := pureCampaignAssignment(t, "entry-assignment")
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	share, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	group := pureCampaignGroup(t, "entry-group", share)
	action := purePushAction(t, "entry-action", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	call, err := action.Call(map[string]string{"copy": "constructed copy"})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "entry-treatment", Call: call})
	if err != nil {
		t.Fatal(err)
	}
	built := purePushCampaign(t, "entry-campaign", group, []campaign.Treatment{treatment}, nil)
	record := audience.Record{UID: "uid-entry"}
	facts := audience.AssignmentFacts{{Definition: assignment, Bucket: second}}

	first, matched, err := campaign.PlanEntry(built, treatment.ID(), record, facts, "entry-1")
	if err != nil {
		t.Fatalf("planning entry: %v", err)
	}
	if !matched {
		t.Fatal("matching record produced no delivery intent")
	}
	secondPlan, matched, err := campaign.PlanEntry(built, treatment.ID(), record, facts, "entry-1")
	if err != nil || !matched {
		t.Fatalf("replanning same entry: matched=%v err=%v", matched, err)
	}
	if first.Key == "" || first.Key != secondPlan.Key {
		t.Fatalf("same entry keys are %q and %q", first.Key, secondPlan.Key)
	}
	if first.UID != record.UID || first.Campaign != built.ID() || first.Treatment != treatment.ID() || first.Action != action.ID() {
		t.Fatalf("intent identity = %#v", first)
	}
	if len(first.Placements) != 1 || first.Placements[0].Assignment != string(assignment.ID()) || first.Placements[0].Share != 2 {
		t.Fatalf("intent placements = %#v, want assignment share 2", first.Placements)
	}
}

func TestValidateEntryRejectsTargetsThatCannotStartFromAnAudience(t *testing.T) {
	assignment := pureCampaignAssignment(t, "validate-entry-assignment")
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	condition, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	group := pureCampaignGroup(t, "validate-entry-group", condition)
	action := purePushAction(t, "validate-entry-action")
	call, err := action.Call(map[string]string{"copy": "constructed copy"})
	if err != nil {
		t.Fatal(err)
	}
	entry, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "entry-treatment", Call: call})
	if err != nil {
		t.Fatal(err)
	}
	built := purePushCampaign(t, "validate-entry-campaign", group, []campaign.Treatment{entry}, nil)

	got, err := campaign.ValidateEntry(built, entry.ID())
	if err != nil || got.ID() != entry.ID() {
		t.Fatalf("valid entry = %#v, err=%v", got, err)
	}
	if _, err := campaign.ValidateEntry(built, "missing-treatment"); !errors.Is(err, campaign.ErrNoSuchTreatment) {
		t.Fatalf("missing treatment error = %v, want ErrNoSuchTreatment", err)
	}

	receiptBuilt, _, receiptOnly := pureReceiptCampaign(t)
	if _, err := campaign.ValidateEntry(receiptBuilt, receiptOnly.ID()); !errors.Is(err, campaign.ErrNotAnEntry) {
		t.Fatalf("receipt-only treatment error = %v, want ErrNotAnEntry", err)
	}

	pull := purePullTreatment(t, "entry-treatment")
	pullBuilt := purePushCampaign(t, "validate-pull-entry-campaign", pureCampaignGroup(t, "validate-pull-entry-group", nil), []campaign.Treatment{pull}, nil)
	if _, err := campaign.ValidateEntry(pullBuilt, pull.ID()); !errors.Is(err, campaign.ErrPullCannotBeDeliveredInBatch) {
		t.Fatalf("pull entry error = %v, want ErrPullCannotBeDeliveredInBatch", err)
	}
}

func TestCompleteDeliveryBuildsExecutionFromIntent(t *testing.T) {
	placements := []audience.Placement{{Assignment: "assignment-control", Share: 1}, {Assignment: "rollout-wide", Share: 4}}
	intent := campaign.DeliveryIntent{
		Key: "delivery-key", UID: "uid-delivery", Campaign: "campaign-delivery",
		Treatment: "treatment-delivery", Action: "action-delivery",
		Placements: placements,
	}
	execution, err := campaign.CompleteDelivery(intent, 2, campaign.Ack{ExternalRef: "downstream-ref"}, true)
	if err != nil {
		t.Fatalf("completing delivery: %v", err)
	}
	if execution.Key != intent.Key || execution.UID != intent.UID || execution.Campaign != intent.Campaign || execution.Treatment != intent.Treatment || execution.Action != intent.Action {
		t.Fatalf("execution identity = %#v", execution)
	}
	if execution.State != campaign.StateDelivered || execution.Attempts != 2 || execution.ExternalRef != "downstream-ref" || !reflect.DeepEqual(execution.Placements, intent.Placements) {
		t.Fatalf("execution result = %#v", execution)
	}
	if _, err := campaign.CompleteDelivery(intent, 0, campaign.Ack{}, false); !errors.Is(err, campaign.ErrNonPositiveAttempts) {
		t.Fatalf("zero attempts error = %v, want ErrNonPositiveAttempts", err)
	}
}

func TestApplyReceiptPreservesPersonAndPlacementOnEdge(t *testing.T) {
	built, first, _ := pureReceiptCampaign(t)
	placements := []audience.Placement{{Assignment: "arm-assignment", Share: 3}, {Assignment: "rollout-assignment", Share: 1}}
	execution := campaign.Execution{
		Key: "upstream-execution-key", Campaign: built.ID(), Treatment: first.ID(),
		UID: "uid-from-receipt", Action: first.Call().Action(), Placements: placements,
		State: campaign.StateDelivered, Attempts: 1,
	}
	receipt := campaign.Receipt{
		Key: execution.Key, Arrived: true, ExternalRef: "receipt-ref",
		Attributes: map[string]string{"status": "arrived"}, Outcome: campaign.OutcomeSuccess,
	}

	transition, err := campaign.ApplyReceipt(built, execution, receipt)
	if err != nil {
		t.Fatalf("applying receipt: %v", err)
	}
	if transition.Noop || transition.Execution.State != campaign.StateArrived || transition.Event.Name != campaign.ArrivedEvent(first.Call().Action()) {
		t.Fatalf("transition = %#v, want first arrived receipt", transition)
	}
	if len(transition.Deliveries) != 1 {
		t.Fatalf("downstream intents = %#v, want exactly one", transition.Deliveries)
	}
	next := transition.Deliveries[0]
	if next.UID != execution.UID || !reflect.DeepEqual(next.Placements, execution.Placements) {
		t.Fatalf("edge intent lost receipt person or placements: %#v", next)
	}
	wantKey := campaign.IdempotencyKey(fmt.Sprintf("%d:%s|%d:%s|%d:%s|%d:%s|",
		len("receipt-campaign"), "receipt-campaign", len("next-treatment"), "next-treatment",
		len(string(execution.UID)), string(execution.UID), len(string(execution.Key)), string(execution.Key)))
	if next.Key != wantKey {
		t.Fatalf("edge key = %q, want %q derived from the upstream key", next.Key, wantKey)
	}

	replayed, err := campaign.ApplyReceipt(built, transition.Execution, receipt)
	if err != nil {
		t.Fatalf("replaying accepted receipt: %v", err)
	}
	if !replayed.Noop || len(replayed.Deliveries) != 0 {
		t.Fatalf("receipt replay = %#v, want a side-effect-free no-op", replayed)
	}
}

func TestApplyReceiptRejectsConflictingOutcome(t *testing.T) {
	built, first, _ := pureReceiptCampaign(t)
	execution := campaign.Execution{
		Key: "already-receipted", Campaign: built.ID(), Treatment: first.ID(),
		UID: "uid-conflict", Action: first.Call().Action(), State: campaign.StateArrived,
		Attempts: 1, Outcome: campaign.OutcomeSuccess, ReceiptExternalRef: "first-ref",
		ReceiptAttributes: map[string]string{"source": "first"},
	}
	conflict := campaign.Receipt{
		Key: execution.Key, Arrived: false, ExternalRef: "second-ref",
		Attributes: map[string]string{"source": "second"}, Outcome: campaign.OutcomeFailure,
	}
	if _, err := campaign.ApplyReceipt(built, execution, conflict); !errors.Is(err, campaign.ErrConflictingReceipt) {
		t.Fatalf("conflicting outcome error = %v, want ErrConflictingReceipt", err)
	}
}

func TestApplyReceiptCopiesInputPlacements(t *testing.T) {
	built, first, _ := pureReceiptCampaign(t)
	placements := []audience.Placement{{Assignment: "arm-assignment", Share: 2}}
	execution := campaign.Execution{
		Key: "copy-key", Campaign: built.ID(), Treatment: first.ID(), UID: "uid-copy",
		Action: first.Call().Action(), Placements: placements, State: campaign.StateDelivered,
	}
	transition, err := campaign.ApplyReceipt(built, execution, campaign.Receipt{
		Key: execution.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	})
	if err != nil {
		t.Fatalf("applying receipt: %v", err)
	}
	placements[0].Share = 1
	if transition.Execution.Placements[0].Share != 2 {
		t.Fatalf("transition execution shares caller's placements: %#v", transition.Execution.Placements)
	}
}

func pureCampaignAssignment(t *testing.T, id string) experimentation.Definition {
	t.Helper()
	split, err := experimentation.EqualSplit(2)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: experimentation.AssignmentID(id), Split: split, Salt: experimentation.Salt(id),
		Algorithm: experimentation.FNV1aV1{}.Version(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func pureCampaignGroup(t *testing.T, id string, condition audience.Condition) audience.Group {
	t.Helper()
	source, err := audience.NewSource(audience.SourceSpec{
		ID: audience.SourceID(id + "-source"), Supply: audience.SupplyPull, Category: audience.CategoryExternal,
	})
	if err != nil {
		t.Fatal(err)
	}
	var conditions []audience.Condition
	if condition != nil {
		conditions = []audience.Condition{condition}
	}
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: audience.DefinitionID(id), Source: source.ID(), Conditions: conditions,
	})
	if err != nil {
		t.Fatal(err)
	}
	return audience.Single(definition)
}

func pureDisplayCampaign(t *testing.T, id string, priority int, slot campaign.Slot, group audience.Group, treatmentID, actionID string) campaign.Campaign {
	t.Helper()
	action, err := campaign.NewAction(campaign.ActionSpec{
		ID: campaign.ActionID(actionID), Direction: campaign.DirectionPull, RequiresSlot: true,
		Required: []string{"copy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(map[string]string{"copy": "constructed display"})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: campaign.TreatmentID(treatmentID), Slot: slot.ID(), Call: call,
	})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      campaign.FlowID(id + "-flow"),
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{treatment.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: campaign.CampaignID(id), Priority: campaign.NewPriority(priority),
		Audience: audience.SelectGroup(group), Treatments: []campaign.Treatment{treatment}, Flow: flow,
	})
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func purePushAction(t *testing.T, id string, outcomes ...campaign.Outcome) campaign.Action {
	t.Helper()
	action, err := campaign.NewAction(campaign.ActionSpec{
		ID: campaign.ActionID(id), Direction: campaign.DirectionPush,
		Required: []string{"copy"}, Outcomes: outcomes,
	})
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func purePushCampaign(t *testing.T, id string, group audience.Group, treatments []campaign.Treatment, edges []campaign.EdgeSpec) campaign.Campaign {
	t.Helper()
	entries := make([]campaign.TreatmentID, 0, len(treatments))
	for _, treatment := range treatments {
		if treatment.ID() == "first-treatment" || treatment.ID() == "entry-treatment" {
			entries = append(entries, treatment.ID())
		}
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      campaign.FlowID(id + "-flow"),
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: entries}},
		Edges:   edges,
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: campaign.CampaignID(id), Priority: campaign.NewPriority(10),
		Audience: audience.SelectGroup(group), Treatments: treatments, Flow: flow,
	})
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func pureReceiptCampaign(t *testing.T) (campaign.Campaign, campaign.Treatment, campaign.Treatment) {
	t.Helper()
	group := pureCampaignGroup(t, "receipt-group", nil)
	firstAction := purePushAction(t, "receipt-first-action", campaign.OutcomeSuccess, campaign.OutcomeFailure)
	firstCall, err := firstAction.Call(map[string]string{"copy": "first"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "first-treatment", Call: firstCall})
	if err != nil {
		t.Fatal(err)
	}
	nextAction := purePushAction(t, "receipt-next-action")
	nextCall, err := nextAction.Call(map[string]string{"copy": "next"})
	if err != nil {
		t.Fatal(err)
	}
	next, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "next-treatment", Call: nextCall})
	if err != nil {
		t.Fatal(err)
	}
	built := purePushCampaign(t, "receipt-campaign", group, []campaign.Treatment{first, next}, []campaign.EdgeSpec{{
		From: first.ID(), To: next.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess,
	}})
	return built, first, next
}
