package campaign_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

// Two campaigns ranking the same for one slot is not something priority can
// settle, and nothing else is supposed to. Picking one would bury the choice:
// whichever lost would look like it simply did not apply.
func TestEqualPrioritiesAreRefusedRatherThanGuessed(t *testing.T) {
	w := newWorld(t, 30)
	everyone := w.definition("anyone")
	w.add(campaign.CampaignSpec{
		ID:         "first",
		Priority:   campaign.NewPriority(50),
		Treatments: []campaign.Treatment{w.treatment("first-banner", everyone, "first.png")},
	})
	w.add(campaign.CampaignSpec{
		ID:         "second",
		Priority:   campaign.NewPriority(50),
		Treatments: []campaign.Treatment{w.treatment("second-banner", everyone, "second.png")},
	})

	_, err := w.campaigns.Decide(bannerID, w.people[0].UID, audience.Live())
	if !errors.Is(err, campaign.ErrContentionUndecidable) {
		t.Fatalf("a tie should be reported, not resolved; got %v", err)
	}
}

// A campaign that never said how it ranks would otherwise rank lowest
// everywhere and quietly never win.
func TestCampaignNeedsAPriority(t *testing.T) {
	w := newWorld(t, 10)
	everyone := w.definition("anyone")
	_, err := w.build(campaign.CampaignSpec{
		ID:         "unranked",
		Treatments: []campaign.Treatment{w.treatment("banner", everyone, "x.png")},
	})
	if !errors.Is(err, campaign.ErrPriorityNotSet) {
		t.Fatalf("expected a missing-priority refusal, got %v", err)
	}
}

// Two treatments of one campaign on one slot need something to keep them from
// both applying. Caught while building, not while serving.
func TestTwoTreatmentsOnOneSlotNeedSeparating(t *testing.T) {
	w := newWorld(t, 10)
	newcomers := w.definition("new-visitors", w.condition(audience.Attribute("tier", "new")))
	returning := w.definition("returning-visitors", w.condition(audience.Attribute("tier", "returning")))

	// These two look disjoint to a reader. They are not provably disjoint, and
	// "looks exclusive" is exactly how two treatments reach one person.
	_, err := w.build(campaign.CampaignSpec{
		ID:       "unseparated",
		Priority: campaign.NewPriority(10),
		Treatments: []campaign.Treatment{
			w.treatment("for-new", newcomers, "a.png"),
			w.treatment("for-returning", returning, "b.png"),
		},
	})
	if !errors.Is(err, campaign.ErrSlotContestedWithinCampaign) {
		t.Fatalf("two unseparated treatments on one slot should be refused, got %v", err)
	}

	// Shares of one assignment are the proof this platform has.
	assignment := w.assignment("banner-split", 2, false)
	armOne := w.definition("arm-one", w.condition(audience.Share(assignment, w.share(1))))
	armTwo := w.definition("arm-two", w.condition(audience.Share(assignment, w.share(2))))
	first := w.treatment("arm-one-banner", armOne, "a.png")
	second := w.treatment("arm-two-banner", armTwo, "b.png")
	armOneShare, armTwoShare := w.share(1), w.share(2)
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "banner-split-audience", Source: sourceID, Assignment: assignment,
		Arms: []audience.BundleArm{
			{ID: "banner-arm-one", Share: armOneShare, Definition: armOne},
			{ID: "banner-arm-two", Share: armTwoShare, Definition: armTwo},
		},
	})
	if err != nil {
		t.Fatalf("building the bundle: %v", err)
	}
	if err := w.audiences.Publish(bundle); err != nil {
		t.Fatalf("publishing the bundle: %v", err)
	}
	groupOne, _ := bundle.Group(armOneShare)
	groupTwo, _ := bundle.Group(armTwoShare)
	flow, err := campaign.NewFlow(campaign.FlowSpec{ID: "separated-flow", Entries: []campaign.EntrySpec{
		{Group: groupOne.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}},
		{Group: groupTwo.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{second.ID()}},
	}})
	if err != nil {
		t.Fatalf("building the split entry routes: %v", err)
	}
	if _, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "separated-by-shares", Priority: campaign.NewPriority(10), Audience: audience.SelectBundle(bundle),
		Treatments: []campaign.Treatment{first, second}, Flow: flow,
	}); err != nil {
		t.Fatalf("treatments on different shares of one assignment should be accepted, got %v", err)
	}

	// A decision table orders them instead, which is the other way to make only
	// one of them ever apply.
	forNew := w.treatment("for-new", newcomers, "a.png")
	forReturning := w.treatment("for-returning", returning, "b.png")
	table, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: bannerID,
		Rows: []campaign.Row{
			{When: audience.Single(newcomers), Then: forNew.ID()},
			{When: audience.Single(returning), Then: forReturning.ID()},
		},
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		t.Fatalf("building the decision table: %v", err)
	}
	if _, err := w.build(campaign.CampaignSpec{
		ID:         "separated-by-table",
		Priority:   campaign.NewPriority(10),
		Treatments: []campaign.Treatment{forNew, forReturning},
		Tables:     []campaign.DecisionTable{table},
	}); err != nil {
		t.Fatalf("treatments ordered by a table should be accepted, got %v", err)
	}
}

// A decision table may describe treatments for several experiment arms, but
// each person can only be routed to treatments attached to their own arm.
func TestDecisionTableCannotRouteAcrossExperimentArms(t *testing.T) {
	w := newWorld(t, 400)
	assignment := w.assignment("table-arms", 2, false)
	shareOne, shareTwo := w.share(1), w.share(2)
	armOne := w.definition("table-arm-one", w.condition(audience.Share(assignment, shareOne)))
	armTwo := w.definition("table-arm-two", w.condition(audience.Share(assignment, shareTwo)))
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "table-arm-bundle", Source: sourceID, Assignment: assignment,
		Arms: []audience.BundleArm{
			{ID: "table-arm-a", Share: shareOne, Definition: armOne},
			{ID: "table-arm-b", Share: shareTwo, Definition: armTwo},
		},
	})
	if err != nil {
		t.Fatalf("building the experiment bundle: %v", err)
	}
	if err := w.audiences.Publish(bundle); err != nil {
		t.Fatalf("publishing the experiment bundle: %v", err)
	}
	groupOne, _ := bundle.Group(shareOne)
	groupTwo, _ := bundle.Group(shareTwo)
	first := w.treatment("table-arm-one-banner", armOne, "a.png")
	second := w.treatment("table-arm-two-banner", armTwo, "b.png")
	flow, err := campaign.NewFlow(campaign.FlowSpec{ID: "table-arm-flow", Entries: []campaign.EntrySpec{
		{Group: groupOne.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}},
		{Group: groupTwo.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{second.ID()}},
	}})
	if err != nil {
		t.Fatalf("building experiment entry routes: %v", err)
	}
	flagged := w.definition("table-flagged", w.condition(audience.Attribute("flagged", "yes")))
	newcomers := w.definition("table-newcomers", w.condition(audience.Attribute("tier", "new")))
	table, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: bannerID,
		Rows: []campaign.Row{
			{When: audience.Single(flagged), Then: first.ID()},
			{When: audience.Single(newcomers), Then: second.ID()},
		},
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		t.Fatalf("building the decision table: %v", err)
	}
	w.add(campaign.CampaignSpec{
		ID: "arm-scoped-table", Priority: campaign.NewPriority(10),
		Audience:   audience.SelectBundle(bundle),
		Treatments: []campaign.Treatment{first, second}, Flow: flow,
		Tables: []campaign.DecisionTable{table},
	})

	uidForNewUnflagged := func(group audience.Group) audience.UID {
		t.Helper()
		for _, person := range w.people {
			if person.Attributes["tier"] != "new" || person.Attributes["flagged"] == "yes" {
				continue
			}
			in, err := w.audiences.DecideGroup(group, audience.Live(), person.UID)
			if err != nil {
				t.Fatalf("checking experiment arm membership: %v", err)
			}
			if in {
				return person.UID
			}
		}
		t.Fatalf("no new, unflagged person found in arm %s", group.ID())
		return ""
	}

	armOnePerson := w.decide(uidForNewUnflagged(groupOne), audience.Live())
	if armOnePerson.Won {
		t.Fatalf("arm one was routed to another arm's table row: %+v", armOnePerson.Award)
	}
	armTwoPerson := w.decide(uidForNewUnflagged(groupTwo), audience.Live())
	if !armTwoPerson.Won || armTwoPerson.Award.Treatment != second.ID() {
		t.Fatalf("arm two should follow its own matching row, got %+v", armTwoPerson)
	}
}

// The proof of exclusivity is one-directional on purpose: different shares of
// one assignment, and nothing else.
func TestExclusivityProofIsConservative(t *testing.T) {
	w := newWorld(t, 10)
	newcomers := w.definition("new-visitors", w.condition(audience.Attribute("tier", "new")))
	returning := w.definition("returning-visitors", w.condition(audience.Attribute("tier", "returning")))
	if audience.ProvablyExclusive(newcomers, returning) {
		t.Fatal("attributes that merely look disjoint were reported as proven exclusive")
	}

	assignment := w.assignment("split", 3, false)
	one := w.definition("one", w.condition(audience.Share(assignment, w.share(1))))
	two := w.definition("two", w.condition(audience.Share(assignment, w.share(2))))
	alsoOne := w.definition("also-one", w.condition(audience.Share(assignment, w.share(1))))
	if !audience.ProvablyExclusive(one, two) {
		t.Fatal("different shares of one assignment should be proven exclusive")
	}
	if audience.ProvablyExclusive(one, alsoOne) {
		t.Fatal("the same share of one assignment is not exclusive with itself")
	}

	// Different assignments say nothing about each other, even at different
	// share numbers.
	other := w.assignment("other-split", 3, false)
	otherTwo := w.definition("other-two", w.condition(audience.Share(other, w.share(2))))
	if audience.ProvablyExclusive(one, otherTwo) {
		t.Fatal("shares of two unrelated assignments were reported as proven exclusive")
	}
}

// The rollout gates the whole campaign at once. Gating per treatment would let
// arms of one experiment open up at different rates, and the conclusion would
// be wrong with nothing failing.
func TestRolloutGatesTheWholeCampaign(t *testing.T) {
	w := newWorld(t, 400)
	assignment := w.assignment("rollout", 100, false)
	admitted := w.definition("admitted-so-far", w.condition(audience.ShareAtMost(assignment, w.share(20))))

	armSplit := w.assignment("arms", 2, false)
	armOne := w.definition("arm-one", w.condition(audience.Share(armSplit, w.share(1))))
	armTwo := w.definition("arm-two", w.condition(audience.Share(armSplit, w.share(2))))
	armOneShare, armTwoShare := w.share(1), w.share(2)
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "rollout-arms", Source: sourceID, Assignment: armSplit,
		Arms: []audience.BundleArm{
			{ID: "rollout-arm-one", Share: armOneShare, Definition: armOne},
			{ID: "rollout-arm-two", Share: armTwoShare, Definition: armTwo},
		},
	})
	if err != nil {
		t.Fatalf("building the rollout bundle: %v", err)
	}
	if err := w.audiences.Publish(bundle); err != nil {
		t.Fatalf("publishing the rollout bundle: %v", err)
	}
	groupOne, _ := bundle.Group(armOneShare)
	groupTwo, _ := bundle.Group(armTwoShare)
	first := w.treatment("arm-one-banner", armOne, "a.png")
	second := w.treatment("arm-two-banner", armTwo, "b.png")
	flow, err := campaign.NewFlow(campaign.FlowSpec{ID: "gradual-flow", Entries: []campaign.EntrySpec{
		{Group: groupOne.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}},
		{Group: groupTwo.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{second.ID()}},
	}})
	if err != nil {
		t.Fatalf("building the rollout flow: %v", err)
	}

	w.add(campaign.CampaignSpec{
		ID:         "gradual",
		Priority:   campaign.NewPriority(10),
		Audience:   audience.SelectBundle(bundle),
		Rollout:    audience.Single(admitted),
		Treatments: []campaign.Treatment{first, second}, Flow: flow,
	})

	var inside, outside int
	for _, person := range w.people {
		inOne, err := w.audiences.DecideGroup(groupOne, audience.Live(), person.UID)
		if err != nil {
			t.Fatal(err)
		}
		inTwo, err := w.audiences.DecideGroup(groupTwo, audience.Live(), person.UID)
		if err != nil {
			t.Fatal(err)
		}
		var expected campaign.TreatmentID
		switch {
		case inOne && !inTwo:
			expected = first.ID()
		case inTwo && !inOne:
			expected = second.ID()
		default:
			t.Fatalf("%q belongs to arm one=%v and arm two=%v", person.UID, inOne, inTwo)
		}
		withinRollout, err := w.audiences.DecideGroup(audience.Single(admitted), audience.Live(), person.UID)
		if err != nil {
			t.Fatal(err)
		}
		decision := w.decide(person.UID, audience.Live())
		if withinRollout {
			if !decision.Won || decision.Award.Treatment != expected {
				t.Fatalf("%q in rollout got %+v, want treatment %s", person.UID, decision, expected)
			}
			inside++
			continue
		}
		if decision.Won {
			t.Fatalf("%q outside rollout won with %+v", person.UID, decision.Award)
		}
		outside++
		losses := w.lossesFor(decision, "gradual")
		if len(losses) != 1 || losses[0].Treatment != expected || losses[0].Reason != campaign.LossOutsideRollout {
			t.Fatalf("%q outside rollout was gated as %+v, want only %s", person.UID, losses, expected)
		}
	}
	if inside == 0 || outside == 0 {
		t.Fatalf("the rollout admitted %d and excluded %d, so this proves nothing", inside, outside)
	}
}

// A rehearsal runs the whole path and colours nobody. The mode has to survive
// campaign, audience and experimentation in turn; a break anywhere would colour
// people while everything else looked fine.
func TestRehearsalRunsTheWholePathAndColoursNobody(t *testing.T) {
	w := newWorld(t, 200)
	coloured := w.assignment("coloured-arms", 2, true)
	armOne := w.definition("arm-one", w.condition(audience.Share(coloured, w.share(1))))
	w.add(campaign.CampaignSpec{
		ID:         "colouring",
		Priority:   campaign.NewPriority(10),
		Treatments: []campaign.Treatment{w.treatment("arm-one-banner", armOne, "a.png")},
	})

	for _, person := range w.people {
		if _, err := w.campaigns.Decide(bannerID, person.UID, audience.Rehearsal()); err != nil {
			t.Fatalf("rehearsing for %q: %v", person.UID, err)
		}
	}
	if w.colouring.Size() != 0 {
		t.Fatalf("a rehearsal wrote %d colouring records", w.colouring.Size())
	}

	if _, err := w.campaigns.Decide(bannerID, w.people[0].UID, audience.Live()); err != nil {
		t.Fatalf("the real run: %v", err)
	}
	if w.colouring.Size() == 0 {
		t.Fatal("the real run coloured nobody, so the rehearsal assertion proves nothing")
	}
}

func TestDecideRefusesWhatItCannotResolve(t *testing.T) {
	w := newWorld(t, 10)
	if _, err := w.campaigns.Decide("no-such-slot", w.people[0].UID, audience.Live()); !errors.Is(err, campaign.ErrUnknownSlot) {
		t.Fatalf("expected an unknown-slot refusal, got %v", err)
	}
	if _, err := w.campaigns.Decide(bannerID, "", audience.Live()); !errors.Is(err, audience.ErrEmptyUID) {
		t.Fatalf("expected an empty-uid refusal, got %v", err)
	}
	var forgotten audience.Mode
	everyone := w.definition("anyone")
	w.add(campaign.CampaignSpec{
		ID:         "any",
		Priority:   campaign.NewPriority(1),
		Treatments: []campaign.Treatment{w.treatment("banner", everyone, "x.png")},
	})
	if _, err := w.campaigns.Decide(bannerID, w.people[0].UID, forgotten); !errors.Is(err, audience.ErrModeNotSet) {
		t.Fatalf("deciding without a mode should fail, got %v", err)
	}
}

// A campaign wanting a slot nobody registered is a configuration mistake, not
// a campaign that simply never wins.
func TestCampaignNeedsItsSlotsRegistered(t *testing.T) {
	w := newWorld(t, 10)
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "elsewhere", Slot: "unregistered-slot", Call: w.call("x.png"),
	})
	if err != nil {
		t.Fatalf("building the treatment: %v", err)
	}
	built, err := w.build(campaign.CampaignSpec{
		ID: "stray", Priority: campaign.NewPriority(1), Treatments: []campaign.Treatment{treatment},
	})
	if err != nil {
		t.Fatalf("building the campaign: %v", err)
	}
	if err := w.campaigns.Add(built); !errors.Is(err, campaign.ErrUnknownSlot) {
		t.Fatalf("expected an unknown-slot refusal, got %v", err)
	}
}

// An action states what it needs; a call that leaves something out is refused
// here rather than discovered by whoever eventually runs it.
func TestActionParametersAreChecked(t *testing.T) {
	w := newWorld(t, 10)
	if _, err := w.show.Call(map[string]string{}); !errors.Is(err, campaign.ErrMissingActionParameter) {
		t.Fatalf("expected a missing-parameter refusal, got %v", err)
	}
	if _, err := w.show.Call(map[string]string{"image": "a.png", "colour": "red"}); !errors.Is(err, campaign.ErrUnknownActionParameter) {
		t.Fatalf("expected an unknown-parameter refusal, got %v", err)
	}
}

// A table routing somewhere the campaign does not hold would send people into
// nothing at serving time.
func TestTableMustRouteWithinItsCampaign(t *testing.T) {
	w := newWorld(t, 10)
	newcomers := w.definition("new-visitors", w.condition(audience.Attribute("tier", "new")))
	held := w.treatment("held", newcomers, "a.png")
	table, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot:     bannerID,
		Rows:     []campaign.Row{{When: audience.Single(newcomers), Then: "not-held"}},
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		t.Fatalf("building the table: %v", err)
	}
	if _, err := w.build(campaign.CampaignSpec{
		ID: "stray-route", Priority: campaign.NewPriority(1),
		Treatments: []campaign.Treatment{held}, Tables: []campaign.DecisionTable{table},
	}); !errors.Is(err, campaign.ErrRowTreatmentNotInCampaign) {
		t.Fatalf("expected a stray-route refusal, got %v", err)
	}
}
