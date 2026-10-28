package scenarios_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/experimentation"
	"github.com/snack8310/gmp/gmp-core/scenarios"
)

func buildPublishedBundle(t *testing.T) (*scenarios.Stack, experimentation.Definition, audience.Bundle, []audience.BundleArm) {
	t.Helper()
	stack := newStack(t, 900)
	assignment, err := scenarios.EqualAssignment("double-eleven-shared", 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := stack.Experiments.RegisterComponent(assignment); err != nil {
		t.Fatal(err)
	}
	inCart, err := audience.Attribute("added-to-cart", "yes")
	if err != nil {
		t.Fatal(err)
	}
	arms := make([]audience.BundleArm, 3)
	for share := 1; share <= 3; share++ {
		bucket, err := scenarios.Share(share)
		if err != nil {
			t.Fatal(err)
		}
		condition, err := audience.Share(assignment, bucket)
		if err != nil {
			t.Fatal(err)
		}
		definition, err := stack.Definition(fmt.Sprintf("double-eleven-arm-%d", share), inCart, condition)
		if err != nil {
			t.Fatal(err)
		}
		arms[share-1] = audience.BundleArm{ID: audience.GroupID(fmt.Sprintf("shared-group-%d", share)), Share: bucket, Definition: definition}
	}
	bundle, err := audience.NewBundle(audience.BundleSpec{ID: "double-eleven-audience", Source: scenarios.SourceID, Assignment: assignment, Arms: arms})
	if err != nil {
		t.Fatal(err)
	}
	if err := stack.Audiences.Publish(bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Available() {
		t.Fatal("unstarted bundle became active")
	}
	if err := stack.Experiments.StartComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if !bundle.Available() {
		t.Fatal("started bundle unavailable")
	}
	bypass := audience.Single(arms[1].Definition)
	if err := stack.Audiences.ValidateGroup(bypass); !errors.Is(err, audience.ErrUnpublishedGroup) {
		t.Fatalf("standalone wrapper bypassed bundle: %v", err)
	}
	return stack, assignment, bundle, arms
}

func assertRuntimeWhitelist(t *testing.T, stack *scenarios.Stack, assignment experimentation.Definition, bundle audience.Bundle) {
	t.Helper()
	key := experimentation.AssignmentKey(stack.People[0].UID)
	before, err := stack.Experiments.PeekAssign(assignment, key)
	if err != nil {
		t.Fatal(err)
	}
	whitelistedShare := 2
	if before.Value() == 2 {
		whitelistedShare = 3
	}
	forced, err := scenarios.Share(whitelistedShare)
	if err != nil {
		t.Fatal(err)
	}
	if err := stack.Experiments.UpdateWhitelist(assignment.ID(), map[experimentation.AssignmentKey]experimentation.BucketNumber{key: forced}); err != nil {
		t.Fatal(err)
	}
	forcedGroup, _ := bundle.Group(forced)
	if in, err := stack.Audiences.DecideGroup(forcedGroup, audience.Live(), stack.People[0].UID); err != nil || !in {
		t.Fatalf("published arm ignored runtime whitelist: %v, %v", in, err)
	}
	if !bundle.Available() {
		t.Fatal("runtime parameter update retired bundle")
	}
}

func assertArmPartition(t *testing.T, stack *scenarios.Stack, bundle audience.Bundle, arms []audience.BundleArm) {
	t.Helper()
	for _, arm := range arms {
		group, ok := stack.Audiences.Group(arm.ID)
		if !ok || group.ID() != arm.ID {
			t.Fatalf("missing group %q", arm.ID)
		}
		members, err := stack.Audiences.EnumerateGroup(group, audience.Live())
		if err != nil {
			t.Fatal(err)
		}
		if len(members) == 0 {
			t.Fatalf("group %q empty", arm.ID)
		}
		for _, uid := range members {
			in, err := stack.Audiences.DecideGroup(group, audience.Live(), uid)
			if err != nil || !in {
				t.Fatalf("enumerate/decide disagree for %q: %v", uid, err)
			}
			for _, other := range arms {
				if other.ID == arm.ID {
					continue
				}
				otherGroup, _ := bundle.Group(other.Share)
				also, err := stack.Audiences.DecideGroup(otherGroup, audience.Live(), uid)
				if err != nil || also {
					t.Fatalf("%q entered two arms: %v", uid, err)
				}
			}
		}
	}
	control, _ := bundle.Group(arms[0].Share)
	if control.ID() != arms[0].ID {
		t.Fatal("control arm missing")
	}
}

func runSharedArmCampaigns(t *testing.T, stack *scenarios.Stack, assignment experimentation.Definition, bundle audience.Bundle, arms []audience.BundleArm) (*campaign.Runner, []campaign.Campaign) {
	t.Helper()
	action, err := campaign.RegisterAction(campaign.ActionSpec{ID: "shared-message", Direction: campaign.DirectionPush})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	log := campaign.NewMemoryLog()
	runner, err := campaign.NewRunner(campaign.RunnerSpec{Audiences: stack.Audiences, Log: log, Events: campaign.DiscardEvents(), MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.RegisterDeliverer(action.ID(), campaign.NewMemoryDeliverer()); err != nil {
		t.Fatal(err)
	}
	var campaigns []campaign.Campaign
	for i, share := range []int{2, 3, 2} {
		group, _ := bundle.Group(arms[share-1].Share)
		treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: campaign.TreatmentID(fmt.Sprintf("send-%d", i)), Who: group, Call: call})
		if err != nil {
			t.Fatal(err)
		}
		built, err := campaign.NewCampaign(campaign.CampaignSpec{ID: campaign.CampaignID(fmt.Sprintf("activity-%d", i)), Priority: campaign.NewPriority(1), Treatments: []campaign.Treatment{treatment}})
		if err != nil {
			t.Fatal(err)
		}
		campaigns = append(campaigns, built)
		report, err := runner.Run(built, treatment.ID(), campaign.Occurrence("first"), audience.Live())
		if err != nil {
			t.Fatal(err)
		}
		if len(report.Executions) == 0 {
			t.Fatalf("activity %d did not execute", i)
		}
		for _, execution := range report.Executions {
			if execution.Campaign != built.ID() || execution.Treatment != treatment.ID() || len(execution.Placements) != 1 ||
				execution.Placements[0].Assignment != string(assignment.ID()) || execution.Placements[0].Share != share {
				t.Fatalf("activity %d expected share %d, got %+v", i, share, execution)
			}
		}
	}
	if campaigns[0].Treatments()[0].Audience().ID() != campaigns[2].Treatments()[0].Audience().ID() {
		t.Fatal("shared arm was copied instead of reused")
	}
	return runner, campaigns
}

func addSharedArmDisplay(t *testing.T, stack *scenarios.Stack, bundle audience.Bundle, arms []audience.BundleArm) (*campaign.Service, campaign.Campaign, []audience.UID, audience.Group) {
	t.Helper()
	displayAction, err := campaign.RegisterAction(campaign.ActionSpec{ID: "shared-display", Direction: campaign.DirectionPull, RequiresSlot: true})
	if err != nil {
		t.Fatal(err)
	}
	displayCall, err := displayAction.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	group, _ := bundle.Group(arms[1].Share)
	everyone, err := stack.Definition("display-anyone")
	if err != nil {
		t.Fatal(err)
	}
	display, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "shared-banner", Slot: scenarios.BannerSlot, Who: audience.Single(everyone), Call: displayCall})
	if err != nil {
		t.Fatal(err)
	}
	table, err := campaign.NewDecisionTable(campaign.TableSpec{Slot: scenarios.BannerSlot, Rows: []campaign.Row{{When: group, Then: display.ID()}}, Fallback: campaign.RouteNowhere()})
	if err != nil {
		t.Fatal(err)
	}
	activity, err := campaign.NewCampaign(campaign.CampaignSpec{ID: "display-activity", Priority: campaign.NewPriority(1), Treatments: []campaign.Treatment{display}, Tables: []campaign.DecisionTable{table}})
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
	members, err := stack.Audiences.EnumerateGroup(group, audience.Live())
	if err != nil {
		t.Fatal(err)
	}
	decision, err := service.Decide(scenarios.BannerSlot, members[0], audience.Live())
	if err != nil || !decision.Won || len(decision.Award.Placements) != 1 ||
		decision.Award.Placements[0].Assignment != string(bundle.Assignment().ID()) || decision.Award.Placements[0].Share != 2 {
		t.Fatalf("display decision lost arm attribution: %+v, %v", decision, err)
	}
	return service, activity, members, group
}

func assertRatioUpdate(t *testing.T, stack *scenarios.Stack, assignment experimentation.Definition, bundle audience.Bundle, group audience.Group, member audience.UID) {
	t.Helper()
	newRatio, err := experimentation.NewSplit(3, []int{0, 0, 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := stack.Experiments.UpdateSplit(assignment.ID(), newRatio); err != nil {
		t.Fatal(err)
	}
	if !bundle.Available() {
		t.Fatal("ratio update retired bundle")
	}
	if got, err := stack.Experiments.Assign(assignment, "new-after-ratio"); err != nil || got.Value() != 3 {
		t.Fatalf("new ratio not in force: %v, %v", got, err)
	}
	if in, err := stack.Audiences.DecideGroup(group, audience.Live(), member); err != nil || !in {
		t.Fatalf("ratio update moved an existing coloring: %v, %v", in, err)
	}
}

func assertRetirement(t *testing.T, stack *scenarios.Stack, assignment experimentation.Definition, bundle audience.Bundle, service *campaign.Service, activity campaign.Campaign, members []audience.UID, runner *campaign.Runner, campaigns []campaign.Campaign) {
	t.Helper()
	if err := stack.Experiments.StopComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if bundle.Available() {
		t.Fatal("retired bundle remained available")
	}
	if _, err := service.Decide(scenarios.BannerSlot, members[0], audience.Live()); !errors.Is(err, audience.ErrGroupUnavailable) {
		t.Fatalf("old display reference: %v", err)
	}
	if _, err := runner.Run(campaigns[0], campaigns[0].Treatments()[0].ID(), campaign.Occurrence("second"), audience.Live()); !errors.Is(err, audience.ErrGroupUnavailable) {
		t.Fatalf("old push reference: %v", err)
	}
	newService, err := stack.NewCampaignService()
	if err != nil {
		t.Fatal(err)
	}
	if err := newService.Add(activity); !errors.Is(err, audience.ErrGroupUnavailable) {
		t.Fatalf("new attachment: %v", err)
	}
	if err := stack.Experiments.StartComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if bundle.Available() {
		t.Fatal("old bundle revived")
	}
}

func TestPublishedBundleAcrossCampaignsAndRetirement(t *testing.T) {
	stack, assignment, bundle, arms := buildPublishedBundle(t)
	assertRuntimeWhitelist(t, stack, assignment, bundle)
	assertArmPartition(t, stack, bundle, arms)
	runner, campaigns := runSharedArmCampaigns(t, stack, assignment, bundle, arms)
	service, activity, members, group := addSharedArmDisplay(t, stack, bundle, arms)
	assertRatioUpdate(t, stack, assignment, bundle, group, members[0])
	assertRetirement(t, stack, assignment, bundle, service, activity, members, runner, campaigns)
}

func TestUnregisteredShareCannotAttach(t *testing.T) {
	stack := newStack(t, 20)
	assignment, err := scenarios.EqualAssignment("unregistered", 2, false)
	if err != nil {
		t.Fatal(err)
	}
	share, err := scenarios.Share(1)
	if err != nil {
		t.Fatal(err)
	}
	condition, err := audience.Share(assignment, share)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := stack.Definition("unregistered-arm", condition)
	if err != nil {
		t.Fatal(err)
	}
	action, err := campaign.RegisterAction(campaign.ActionSpec{ID: "check-attachment", Direction: campaign.DirectionPull, RequiresSlot: true})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(nil)
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "arm", Slot: scenarios.BannerSlot, Who: audience.Single(definition), Call: call})
	if err != nil {
		t.Fatal(err)
	}
	activity, err := campaign.NewCampaign(campaign.CampaignSpec{ID: "unregistered-activity", Priority: campaign.NewPriority(1), Treatments: []campaign.Treatment{treatment}})
	if err != nil {
		t.Fatal(err)
	}
	service, err := stack.NewCampaignService()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Add(activity); !errors.Is(err, experimentation.ErrComponentUnknown) {
		t.Fatalf("unregistered share attached: %v", err)
	}
	if err := stack.Experiments.RegisterComponent(assignment); err != nil {
		t.Fatal(err)
	}
	if err := service.Add(activity); !errors.Is(err, audience.ErrGroupUnavailable) {
		t.Fatalf("stopped share attached: %v", err)
	}
	if err := stack.Experiments.StartComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if err := service.Add(activity); err != nil {
		t.Fatalf("running activity-local share failed to attach: %v", err)
	}
}
