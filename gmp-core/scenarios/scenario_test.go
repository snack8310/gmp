package scenarios_test

import (
	"errors"
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

// Scenario 2 in the shape operations would configure it: one campaign holding
// the arms of one assignment, each arm doing its own thing.
//
// The assertion is "each arm gets exactly what its arm prescribes and nothing
// else". Only checking that an arm got what it should would pass a campaign
// that also sent everyone everything.
func TestScenarioTwoOneCampaignHoldsEveryArm(t *testing.T) {
	stack := newStack(t, 800)
	built, err := stack.ArmCampaign()
	if err != nil {
		t.Fatalf("building the experiment campaign: %v", err)
	}

	// What the scenario asks for, stated here rather than read off the
	// campaign. Deriving it from the campaign would move with any mistake in
	// the campaign and catch nothing.
	prescribed := map[string][]campaign.ActionID{
		"A-control":        nil,
		"B-sms":            {scenarios.SMSAction},
		"C-sms-and-coupon": {scenarios.SMSAction, scenarios.CouponAction},
		"D-coupon":         {scenarios.CouponAction},
	}
	if len(prescribed) != len(built.Arms.Order) {
		t.Fatalf("the scenario prescribes %d arms, the experiment carves %d",
			len(prescribed), len(built.Arms.Order))
	}
	for _, name := range built.Arms.Order {
		if _, held := prescribed[name]; !held {
			t.Fatalf("arm %q is in the experiment but the scenario prescribes nothing for it", name)
		}
	}
	if got := prescribed[built.Arms.Control]; len(got) != 0 {
		t.Fatalf("the control arm %q is prescribed %v; it is supposed to get nothing",
			built.Arms.Control, got)
	}
	// Which share each arm is. Stated here, and deliberately a different value
	// per arm: an execution's placement is checked against this rather than
	// merely checked for being present. A placement that exists but always
	// says the same share would record every arm's results under one arm, and
	// the experiment side would attribute all of them wrongly.
	share := map[string]int{
		"A-control":        1,
		"B-sms":            2,
		"C-sms-and-coupon": 3,
		"D-coupon":         4,
	}
	for _, name := range built.Arms.Order {
		if _, held := share[name]; !held {
			t.Fatalf("arm %q is in the experiment but no share is stated for it", name)
		}
	}

	// The campaign has to hang exactly that, arm by arm.
	hangs := built.ByArm()
	for _, name := range built.Arms.Order {
		if !sameActions(hangs[name], prescribed[name]) {
			t.Fatalf("the campaign hangs %v on arm %q, the scenario prescribes %v",
				hangs[name], name, prescribed[name])
		}
	}

	// Without this the loops below can run over empty arms and pass while
	// asserting nothing.
	//
	// The bound is not "non-empty": one member of an arm is spent per
	// treatment below, to make a send fail. An arm with no more members than
	// it has treatments would have every member short of something, and for
	// the arm prescribed more than one action nobody would be left whose
	// comparison says both arrived -- with every other guard silent.
	treatmentsPerArm := map[string]int{}
	for _, treatment := range built.Treatments {
		treatmentsPerArm[treatment.Arm]++
	}
	for _, name := range built.Arms.Order {
		members, err := stack.Audiences.Enumerate(built.Arms.Arms[name], audience.Live())
		if err != nil {
			t.Fatalf("enumerating arm %q: %v", name, err)
		}
		if len(members) <= treatmentsPerArm[name] {
			t.Fatalf("arm %q has %d members and %d treatments; with one member spent per treatment, none is left whose comparison says everything arrived",
				name, len(members), treatmentsPerArm[name])
		}
	}
	var acting int
	for _, name := range built.Arms.Order {
		if len(prescribed[name]) > 0 {
			acting++
		}
	}
	if acting < 2 {
		t.Fatal("fewer than two arms do anything, so the campaign shows no difference between arms")
	}

	// Run every treatment, then ask what each person actually received. One
	// send is made to fail throughout: a failure still belongs to its arm --
	// dropping it would quietly filter the audience -- but it is not something
	// received, and the difference has to hold rather than merely be written
	// down.
	received := map[audience.UID][]campaign.ActionID{}
	failedBy := map[campaign.TreatmentID]audience.UID{}
	// What a person is short of because a send for them was made to fail.
	// Subtracted from their arm's prescription rather than excusing them from
	// the comparison: excusing them would stop the comparison from noticing a
	// failed send being counted as received.
	missing := map[audience.UID][]campaign.ActionID{}
	// How many treatments of an arm have already had a send made to fail, so
	// that the next one picks a different person. Failing the same person on
	// every treatment of their arm would subtract their whole prescription,
	// and their comparison would degenerate to empty against empty.
	failedSoFar := map[string]int{}
	var failures int
	for _, treatment := range built.Treatments {
		want, held := share[treatment.Arm]
		if !held {
			t.Fatalf("treatment %q names arm %q, which is not in the experiment",
				treatment.ID, treatment.Arm)
		}
		before := built.Log.Size()
		rehearsed, err := built.Runner.Run(built.Campaign, treatment.ID, "entry", audience.Rehearsal())
		if err != nil {
			t.Fatalf("rehearsing treatment %q: %v", treatment.ID, err)
		}
		if len(rehearsed.Planned) == 0 {
			t.Fatalf("the rehearsal of %q planned nothing, so the failure below is never injected",
				treatment.ID)
		}
		if built.Log.Size() != before {
			t.Fatalf("rehearsing %q changed the log, so a rehearsal is not free of effect",
				treatment.ID)
		}
		deliverer, wired := built.Deliverers[treatment.Action]
		if !wired {
			t.Fatalf("action %q has no deliverer", treatment.Action)
		}
		at := failedSoFar[treatment.Arm]
		failedSoFar[treatment.Arm]++
		if at >= len(rehearsed.Planned) {
			t.Fatalf("arm %q has more treatments than people, so two of them would fail the same person",
				treatment.Arm)
		}
		victim := rehearsed.Planned[at]
		deliverer.FailNext(victim.Key, 99)
		failedBy[treatment.ID] = victim.UID
		missing[victim.UID] = append(missing[victim.UID], treatment.Action)
		report, err := built.Runner.Run(built.Campaign, treatment.ID, "entry", audience.Live())
		if err != nil {
			t.Fatalf("running treatment %q: %v", treatment.ID, err)
		}
		if len(report.Executions) == 0 {
			t.Fatalf("treatment %q reached nobody, so it asserts nothing", treatment.ID)
		}
		for _, execution := range report.Executions {
			// Scenario 2's hard requirement: a result has to carry both which
			// treatment produced it and which share the person is in, or one
			// side of the metrics cannot be computed.
			if execution.Treatment != treatment.ID {
				t.Fatalf("%q was recorded under treatment %q, expected %q",
					execution.Key, execution.Treatment, treatment.ID)
			}
			if len(execution.Placements) != 1 {
				t.Fatalf("%q carries %d placements, expected exactly the one assignment its arm is a share of",
					execution.Key, len(execution.Placements))
			}
			placement := execution.Placements[0]
			if placement.Assignment != scenarios.ArmAssignment {
				t.Fatalf("%q is placed in assignment %q, expected %q",
					execution.Key, placement.Assignment, scenarios.ArmAssignment)
			}
			if placement.Share != want {
				t.Fatalf("%q is in arm %q and placed in share %d, expected share %d",
					execution.Key, treatment.Arm, placement.Share, want)
			}
			if execution.State == campaign.StateFailed {
				if execution.UID != failedBy[treatment.ID] {
					t.Fatalf("%q failed, but the send made to fail was %q's",
						execution.UID, failedBy[treatment.ID])
				}
				failures++
				// Still placed in its arm -- asserted above, before this --
				// but not something received.
				continue
			}
			received[execution.UID] = append(received[execution.UID], execution.Action)
		}
	}
	if failures != len(built.Treatments) {
		t.Fatalf("%d sends failed, expected one per treatment", failures)
	}
	// The failures are spread over different people. Stacking them on one
	// person would subtract their whole prescription, and for the arm that is
	// prescribed more than one action that person's comparison would stop
	// saying anything about getting both.
	for uid, actions := range missing {
		if len(actions) > 1 {
			t.Fatalf("%q had %v subtracted; failures are supposed to fall on different people",
				uid, actions)
		}
	}

	for _, name := range built.Arms.Order {
		members, err := stack.Audiences.Enumerate(built.Arms.Arms[name], audience.Live())
		if err != nil {
			t.Fatalf("enumerating arm %q: %v", name, err)
		}
		for _, uid := range members {
			want := without(prescribed[name], missing[uid])
			got := received[uid]
			if !sameActions(got, want) {
				t.Fatalf("%q is in arm %q and received %v, expected %v", uid, name, got, want)
			}
		}
	}
}

// The check that lets two treatments share one slot is the proof that shares of
// one assignment never overlap. Aim two at the same arm and the campaign must
// refuse to build -- left to run time, two of one campaign's treatments would
// reach the same person with nothing downstream flagging it.
func TestScenarioTwoTwoTreatmentsOnOneSlotForOneArmIsRefused(t *testing.T) {
	stack := newStack(t, 200)
	arms, err := stack.ExperimentArms()
	if err != nil {
		t.Fatalf("building the arms: %v", err)
	}
	action, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: scenarios.SMSAction, Direction: campaign.DirectionPush, Required: []string{"copy"},
	})
	if err != nil {
		t.Fatalf("registering the action: %v", err)
	}
	call, err := action.Call(map[string]string{"copy": "预热提醒 · 加购有礼"})
	if err != nil {
		t.Fatalf("filling in the action: %v", err)
	}
	one := arms.Arms["C-sms-and-coupon"]
	first, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "warmup-sms-c", Slot: scenarios.SMSSlot, Who: one, Call: call,
	})
	if err != nil {
		t.Fatalf("building the first treatment: %v", err)
	}
	second, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "warmup-sms-c-again", Slot: scenarios.SMSSlot, Who: one, Call: call,
	})
	if err != nil {
		t.Fatalf("building the second treatment: %v", err)
	}
	_, err = campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "double-eleven-experiment-broken",
		Priority:   campaign.NewPriority(100),
		Treatments: []campaign.Treatment{first, second},
	})
	if !errors.Is(err, campaign.ErrSlotContestedWithinCampaign) {
		t.Fatalf("building the campaign returned %v, expected it to refuse the contested slot", err)
	}

	// The same pair on different arms is the arrangement the scenario needs,
	// and it has to be allowed -- otherwise the refusal above would just mean
	// "two treatments on one slot", not "nothing separates them".
	other, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "warmup-sms-b", Slot: scenarios.SMSSlot, Who: arms.Arms["B-sms"], Call: call,
	})
	if err != nil {
		t.Fatalf("building the treatment for the other arm: %v", err)
	}
	if _, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "double-eleven-experiment-two-arms",
		Priority:   campaign.NewPriority(100),
		Treatments: []campaign.Treatment{first, other},
	}); err != nil {
		t.Fatalf("two arms on one slot were refused: %v", err)
	}
}

// sameActions compares what someone received against what their arm
// prescribes, order aside.
func sameActions(got, want []campaign.ActionID) bool {
	if len(got) != len(want) {
		return false
	}
	counts := map[campaign.ActionID]int{}
	for _, action := range got {
		counts[action]++
	}
	for _, action := range want {
		counts[action]--
		if counts[action] < 0 {
			return false
		}
	}
	return true
}

// without is an arm's prescription less what a person's failed sends never
// delivered.
func without(prescribed, missing []campaign.ActionID) []campaign.ActionID {
	short := map[campaign.ActionID]int{}
	for _, action := range missing {
		short[action]++
	}
	out := make([]campaign.ActionID, 0, len(prescribed))
	for _, action := range prescribed {
		if short[action] > 0 {
			short[action]--
			continue
		}
		out = append(out, action)
	}
	return out
}
