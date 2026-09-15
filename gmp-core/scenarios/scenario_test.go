package scenarios_test

import (
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/scenarios"
)

func newStack(t *testing.T, population int) *scenarios.Stack {
	t.Helper()
	stack, err := scenarios.NewStack(population)
	if err != nil {
		t.Fatalf("wiring the stack: %v", err)
	}
	return stack
}

// Scenario 3 end to end, across audience and campaign: someone opens the app,
// two campaigns want the banner, and the promotion routes new and returning
// visitors to different images while leaving everyone else to the win-back.
func TestScenarioThreeAcrossLayers(t *testing.T) {
	stack := newStack(t, 300)
	contest, err := stack.BannerContest()
	if err != nil {
		t.Fatalf("setting up the banner contest: %v", err)
	}

	seen := map[string]bool{}
	for _, person := range stack.People {
		decision, err := contest.Campaigns.Decide(scenarios.BannerSlot, person.UID, audience.Live())
		if err != nil {
			t.Fatalf("deciding for %q: %v", person.UID, err)
		}
		if !decision.Won {
			t.Fatalf("%q got nothing; one of the two campaigns should always take this slot", person.UID)
		}
		tier := person.Attributes["tier"]
		seen[tier] = true
		switch tier {
		case "new", "returning":
			if decision.Award.Campaign != contest.Promotion {
				t.Fatalf("%q (%s): %q won, expected the promotion to outrank the win-back",
					person.UID, tier, decision.Award.Campaign)
			}
			// The win-back lost and that has to be findable, or its operator
			// asking why coverage is low has nowhere to look.
			var traced bool
			for _, loss := range decision.Losses {
				if loss.Campaign == contest.WinBack && loss.Reason == campaign.LossOnPriority {
					traced = true
				}
			}
			if !traced {
				t.Fatalf("%q: the win-back lost on priority and left no trace", person.UID)
			}
		default:
			if decision.Award.Campaign != contest.WinBack {
				t.Fatalf("%q (%s): %q won, expected the win-back to take the slot the promotion routed away from",
					person.UID, tier, decision.Award.Campaign)
			}
		}
	}
	for _, tier := range []string{"new", "returning", "other"} {
		if !seen[tier] {
			t.Fatalf("the population never produced a %q visitor, so that branch went unchecked", tier)
		}
	}
}

// Scenario 4 across all three layers: the knob goes from five to twenty to a
// hundred, and whoever is already in stays in. This is why a rollout has to
// ride on bucketing rather than random sampling.
func TestScenarioFourRolloutIsMonotonicAcrossLayers(t *testing.T) {
	stack := newStack(t, 600)

	admittedAt := func(upTo int) map[audience.UID]bool {
		service, err := stack.RolloutAdmitting(upTo)
		if err != nil {
			t.Fatalf("setting up the rollout at %d: %v", upTo, err)
		}
		in := map[audience.UID]bool{}
		for _, person := range stack.People {
			decision, err := service.Decide(scenarios.BannerSlot, person.UID, audience.Live())
			if err != nil {
				t.Fatalf("deciding for %q at %d: %v", person.UID, upTo, err)
			}
			if decision.Won {
				in[person.UID] = true
				continue
			}
			// Anyone left out at this step must be left out for the rollout,
			// not for some other reason that would confound the comparison.
			for _, loss := range decision.Losses {
				if loss.Reason != campaign.LossOutsideRollout {
					t.Fatalf("%q was left out at %d for %q, expected the rollout", person.UID, upTo, loss.Reason)
				}
			}
		}
		return in
	}

	atFive, atTwenty, atHundred := admittedAt(5), admittedAt(20), admittedAt(100)
	if len(atFive) == 0 {
		t.Fatal("nobody was admitted at five percent, so monotonicity asserts nothing")
	}
	if len(atFive) >= len(atTwenty) || len(atTwenty) >= len(atHundred) {
		t.Fatalf("the steps did not widen: %d then %d then %d", len(atFive), len(atTwenty), len(atHundred))
	}
	for uid := range atFive {
		if !atTwenty[uid] {
			t.Fatalf("%q was in at five percent and dropped out at twenty", uid)
		}
	}
	for uid := range atTwenty {
		if !atHundred[uid] {
			t.Fatalf("%q was in at twenty percent and dropped out at full rollout", uid)
		}
	}
	if len(atHundred) != len(stack.People) {
		t.Fatalf("full rollout admitted %d of %d", len(atHundred), len(stack.People))
	}
}

// Scenario 2's grouping across audience and experimentation: the arms carve the
// base audience exactly, and the control arm is a real thing in the system
// rather than whatever is left over.
func TestScenarioTwoArmsAndControlAcrossLayers(t *testing.T) {
	stack := newStack(t, 800)
	arms, err := stack.ExperimentArms()
	if err != nil {
		t.Fatalf("setting up the experiment arms: %v", err)
	}

	base, err := stack.Audiences.Enumerate(arms.Base, audience.Live())
	if err != nil {
		t.Fatalf("enumerating the base audience: %v", err)
	}
	if len(base) == 0 {
		t.Fatal("the base audience is empty, so the partition asserts nothing")
	}
	inBase := make(map[audience.UID]bool, len(base))
	for _, uid := range base {
		inBase[uid] = true
	}

	placements := map[audience.UID][]string{}
	for _, name := range arms.Order {
		members, err := stack.Audiences.Enumerate(arms.Arms[name], audience.Live())
		if err != nil {
			t.Fatalf("enumerating arm %q: %v", name, err)
		}
		if len(members) == 0 {
			t.Fatalf("arm %q is empty, so the partition is degenerate", name)
		}
		for _, uid := range members {
			if !inBase[uid] {
				t.Fatalf("arm %q holds %q, who is not in the base audience", name, uid)
			}
			placements[uid] = append(placements[uid], name)
		}
	}
	for uid := range inBase {
		switch got := placements[uid]; len(got) {
		case 1:
		case 0:
			t.Fatalf("%q is in the base audience but in no arm", uid)
		default:
			t.Fatalf("%q is in several arms at once: %v", uid, got)
		}
	}

	// The control arm's people are in the system and can be told apart from
	// people who were never in the audience. Without that, the two would be
	// indistinguishable afterwards and the experiment could not be attributed.
	control := arms.Arms[arms.Control]
	outsider := audience.UID("someone-who-never-added-to-cart")
	inControl, err := stack.Audiences.Decide(control, audience.Live(), outsider)
	if err != nil {
		t.Fatalf("deciding about an outsider: %v", err)
	}
	if inControl {
		t.Fatal("someone outside the audience entirely came back as part of the control arm")
	}
	var controlMember audience.UID
	for uid, names := range placements {
		if names[0] == arms.Control {
			controlMember = uid
			break
		}
	}
	if controlMember == "" {
		t.Fatal("the control arm has no members, so it cannot be told apart from anything")
	}
	in, err := stack.Audiences.Decide(control, audience.Live(), controlMember)
	if err != nil {
		t.Fatalf("deciding about a control member: %v", err)
	}
	if !in {
		t.Fatalf("%q is listed in the control arm but deciding says otherwise", controlMember)
	}
}

// A rehearsal runs the identical path through all three layers and colours
// nobody. The mode has to survive campaign, then audience, then
// experimentation; a break at any hand-off would colour people while every
// other signal looked fine.
func TestRehearsalCrossesAllThreeLayersWithoutColouring(t *testing.T) {
	stack := newStack(t, 300)

	assignment, err := scenarios.EqualAssignment("coloured-arms", 2, true)
	if err != nil {
		t.Fatalf("building the assignment: %v", err)
	}
	share, err := scenarios.Share(1)
	if err != nil {
		t.Fatalf("building the share: %v", err)
	}
	inShare, err := audience.Share(assignment, share)
	if err != nil {
		t.Fatalf("building the share condition: %v", err)
	}
	armOne, err := stack.Definition("coloured-arm-one", inShare)
	if err != nil {
		t.Fatalf("building the audience definition: %v", err)
	}
	action, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: "show-image", Direction: campaign.DirectionPull, Required: []string{"image"},
	})
	if err != nil {
		t.Fatalf("registering the action: %v", err)
	}
	call, err := action.Call(map[string]string{"image": "arm-one.png"})
	if err != nil {
		t.Fatalf("filling in the action: %v", err)
	}
	banner, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "coloured-arm-banner", Slot: scenarios.BannerSlot, Who: armOne, Call: call,
	})
	if err != nil {
		t.Fatalf("building the treatment: %v", err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "coloured", Priority: campaign.NewPriority(10), Treatments: []campaign.Treatment{banner},
	})
	if err != nil {
		t.Fatalf("building the campaign: %v", err)
	}
	service, err := stack.NewCampaignService()
	if err != nil {
		t.Fatalf("building the campaign service: %v", err)
	}
	if err := service.Add(built); err != nil {
		t.Fatalf("putting the campaign into service: %v", err)
	}

	for _, person := range stack.People {
		if _, err := service.Decide(scenarios.BannerSlot, person.UID, audience.Rehearsal()); err != nil {
			t.Fatalf("rehearsing for %q: %v", person.UID, err)
		}
	}
	if stack.Colouring.Size() != 0 {
		t.Fatalf("a rehearsal through three layers wrote %d colouring records", stack.Colouring.Size())
	}

	if _, err := service.Decide(scenarios.BannerSlot, stack.People[0].UID, audience.Live()); err != nil {
		t.Fatalf("the real run: %v", err)
	}
	if stack.Colouring.Size() == 0 {
		t.Fatal("the real run coloured nobody, so the rehearsal assertion proves nothing")
	}
}

// Scenario 2's delivery half, across all three layers: the second arm gets a
// message, every execution carries both the treatment and the arm, and the ones
// the send fails for stay in the arm.
func TestScenarioTwoDeliveryCarriesTreatmentAndArm(t *testing.T) {
	stack := newStack(t, 600)
	delivery, err := stack.ArmDelivery(2)
	if err != nil {
		t.Fatalf("setting up the delivery: %v", err)
	}

	// Learn the keys without sending anything, then make every send fail so the
	// failing path is the one under test.
	planned, err := delivery.Runner.Run(delivery.Campaign, delivery.Treatment, "entry", audience.Rehearsal())
	if err != nil {
		t.Fatalf("rehearsing: %v", err)
	}
	if len(planned.Planned) == 0 {
		t.Fatal("the rehearsal planned nothing, so this asserts nothing")
	}
	if delivery.Log.Size() != 0 {
		t.Fatalf("the rehearsal recorded %d executions", delivery.Log.Size())
	}
	for _, plan := range planned.Planned[:1] {
		delivery.Deliverer.FailNext(plan.Key, 99)
	}

	report, err := delivery.Runner.Run(delivery.Campaign, delivery.Treatment, "entry", audience.Live())
	if err != nil {
		t.Fatalf("running the delivery: %v", err)
	}
	if len(report.Executions) != len(planned.Planned) {
		t.Fatalf("the real run produced %d executions, the rehearsal planned %d",
			len(report.Executions), len(planned.Planned))
	}

	var failed int
	for _, execution := range report.Executions {
		if execution.Treatment != delivery.Treatment {
			t.Fatalf("%q was recorded under treatment %q", execution.Key, execution.Treatment)
		}
		if len(execution.Placements) == 0 {
			t.Fatalf("%q carries no placement, so its result cannot be attributed to an arm", execution.Key)
		}
		if got := execution.Placements[0].Share; got != delivery.Arm {
			t.Fatalf("%q is placed in share %d, expected arm %d", execution.Key, got, delivery.Arm)
		}
		if execution.State == campaign.StateFailed {
			failed++
		}
	}
	if failed == 0 {
		t.Fatal("no delivery failed, so the intention-to-treat assertion proves nothing")
	}
	if failed == len(report.Executions) {
		t.Fatal("every delivery failed, so the comparison has no discriminating power")
	}
}
