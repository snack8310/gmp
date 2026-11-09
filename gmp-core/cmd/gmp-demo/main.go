package main

import (
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func main() {
	assignment, err := newAssignment()
	check(err)
	bucket, err := experimentation.Calculate(assignment, "constructed-demo-uid", experimentation.FNV1aV1{})
	check(err)

	source, err := audience.NewSource(audience.SourceSpec{
		ID: "demo-source", Supply: audience.SupplyPull, Category: audience.CategoryExternal,
	})
	check(err)
	condition, err := audience.Share(assignment, bucket)
	check(err)
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: "demo-audience", Source: source.ID(), Conditions: []audience.Condition{condition},
	})
	check(err)
	group := audience.Single(definition)
	record := audience.Record{UID: "constructed-demo-uid"}
	facts := audience.AssignmentFacts{{Definition: assignment, Bucket: bucket}}
	matched, err := definition.Matches(record, facts)
	check(err)

	firstAction, err := campaign.NewAction(campaign.ActionSpec{
		ID: "demo-first-action", Direction: campaign.DirectionPush,
		Outcomes: []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure},
	})
	check(err)
	nextAction, err := campaign.NewAction(campaign.ActionSpec{
		ID: "demo-next-action", Direction: campaign.DirectionPush,
		Outcomes: []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure},
	})
	check(err)
	firstCall, err := firstAction.Call(nil)
	check(err)
	nextCall, err := nextAction.Call(nil)
	check(err)
	first, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "demo-first-treatment", Call: firstCall})
	check(err)
	next, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "demo-next-treatment", Call: nextCall})
	check(err)
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "demo-flow",
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}}},
		Edges:   []campaign.EdgeSpec{{From: first.ID(), To: next.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess}},
	})
	check(err)
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "demo-campaign", Priority: campaign.NewPriority(1), Audience: audience.SelectGroup(group),
		Treatments: []campaign.Treatment{first, next}, Flow: flow,
	})
	check(err)
	intent, shouldDeliver, err := campaign.PlanEntry(built, first.ID(), record, facts, "demo-occurrence")
	check(err)
	if !shouldDeliver {
		panic("constructed demo record was unexpectedly excluded")
	}
	execution, err := campaign.CompleteDelivery(intent, 1, campaign.Ack{ExternalRef: "constructed-ack"}, true)
	check(err)
	transition, err := campaign.ApplyReceipt(built, execution, campaign.Receipt{
		Key: execution.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
	})
	check(err)

	fmt.Printf("assignment=%s share=%d matched=%t delivery_key=%s receipt_event=%s next_step=%s\n",
		assignment.ID(), bucket.Value(), matched, intent.Key, transition.Event.Name, transition.Deliveries[0].Treatment)
}

func newAssignment() (experimentation.Definition, error) {
	split, err := experimentation.EqualSplit(2)
	if err != nil {
		return experimentation.Definition{}, err
	}
	return experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: "demo-assignment", Split: split, Salt: "demo-salt", Algorithm: experimentation.FNV1aV1{}.Version(),
	})
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}
