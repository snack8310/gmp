package scenarios_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/scenarios"
)

func TestPublishedArmAsActivityRolloutKeepsAttribution(t *testing.T) {
	stack, assignment, bundle, arms := buildPublishedBundle(t)
	rollout, _ := bundle.Group(arms[1].Share)
	everyone, err := stack.Definition("rollout-everyone")
	if err != nil {
		t.Fatal(err)
	}
	show, err := campaign.RegisterAction(campaign.ActionSpec{ID: "rollout-display", Direction: campaign.DirectionPull, RequiresSlot: true})
	if err != nil {
		t.Fatal(err)
	}
	showCall, err := show.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	display, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "display", Slot: scenarios.BannerSlot, Who: audience.Single(everyone), Call: showCall})
	if err != nil {
		t.Fatal(err)
	}
	send, err := campaign.RegisterAction(campaign.ActionSpec{ID: "rollout-send", Direction: campaign.DirectionPush})
	if err != nil {
		t.Fatal(err)
	}
	sendCall, err := send.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	push, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "send", Who: audience.Single(everyone), Call: sendCall})
	if err != nil {
		t.Fatal(err)
	}
	activity, err := campaign.NewCampaign(campaign.CampaignSpec{ID: "rollout-activity", Priority: campaign.NewPriority(1), Rollout: rollout, Treatments: []campaign.Treatment{display, push}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := stack.NewCampaignService()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Add(activity); err != nil {
		t.Fatal(err)
	}
	members, err := stack.Audiences.EnumerateGroup(rollout, audience.Live())
	if err != nil || len(members) == 0 {
		t.Fatalf("rollout arm has no members: %v", err)
	}
	decision, err := service.Decide(scenarios.BannerSlot, members[0], audience.Live())
	if err != nil || !decision.Won || len(decision.Award.Placements) != 1 ||
		decision.Award.Placements[0].Assignment != string(assignment.ID()) || decision.Award.Placements[0].Share != 2 {
		t.Fatalf("display rollout placement: %+v, %v", decision, err)
	}
	runner, err := campaign.NewRunner(campaign.RunnerSpec{Audiences: stack.Audiences, Log: campaign.NewMemoryLog(), Events: campaign.DiscardEvents(), MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.RegisterDeliverer(send.ID(), campaign.NewMemoryDeliverer()); err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(activity, push.ID(), "first", audience.Live())
	if err != nil || len(report.Executions) == 0 {
		t.Fatalf("push rollout did not execute: %v", err)
	}
	for _, execution := range report.Executions {
		if execution.Campaign != activity.ID() || execution.Treatment != push.ID() || len(execution.Placements) != 1 ||
			execution.Placements[0].Assignment != string(assignment.ID()) || execution.Placements[0].Share != 2 {
			t.Fatalf("push rollout placement: %+v", execution)
		}
	}
}

func TestRetiredBundleStopsReceiptEdge(t *testing.T) {
	stack, assignment, bundle, arms := buildPublishedBundle(t)
	group, _ := bundle.Group(arms[1].Share)
	action, err := campaign.RegisterAction(campaign.ActionSpec{ID: "retired-step", Direction: campaign.DirectionPush, Outcomes: []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure}})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "first", Who: group, Call: call})
	if err != nil {
		t.Fatal(err)
	}
	second, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "second", Who: group, Call: call})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{ID: "retired-flow", Edges: []campaign.EdgeSpec{{From: first.ID(), To: second.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess}}})
	if err != nil {
		t.Fatal(err)
	}
	activity, err := campaign.NewCampaign(campaign.CampaignSpec{ID: "retired-activity", Priority: campaign.NewPriority(1), Treatments: []campaign.Treatment{first, second}, Flow: flow})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := campaign.NewRunner(campaign.RunnerSpec{Audiences: stack.Audiences, Log: campaign.NewMemoryLog(), Events: campaign.DiscardEvents(), MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	deliverer := campaign.NewMemoryDeliverer()
	if err := runner.RegisterDeliverer(action.ID(), deliverer); err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(activity, first.ID(), "first", audience.Live())
	if err != nil || len(report.Executions) == 0 {
		t.Fatalf("entry did not execute: %v", err)
	}
	before := deliverer.Effects()
	if err := stack.Experiments.StopComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if err := runner.RecordReceipt(activity, campaign.Receipt{Key: report.Executions[0].Key, Arrived: true, Outcome: campaign.OutcomeSuccess}); !errors.Is(err, audience.ErrGroupUnavailable) {
		t.Fatalf("late receipt did not fail closed: %v", err)
	}
	if deliverer.Effects() != before {
		t.Fatal("retired arm delivered a receipt edge")
	}
}
