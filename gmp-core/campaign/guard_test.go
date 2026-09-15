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
	_, err := campaign.NewCampaign(campaign.CampaignSpec{
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
	_, err := campaign.NewCampaign(campaign.CampaignSpec{
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
	if _, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:       "separated-by-shares",
		Priority: campaign.NewPriority(10),
		Treatments: []campaign.Treatment{
			w.treatment("arm-one-banner", armOne, "a.png"),
			w.treatment("arm-two-banner", armTwo, "b.png"),
		},
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
			{When: newcomers, Then: forNew.ID()},
			{When: returning, Then: forReturning.ID()},
		},
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		t.Fatalf("building the decision table: %v", err)
	}
	if _, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID:         "separated-by-table",
		Priority:   campaign.NewPriority(10),
		Treatments: []campaign.Treatment{forNew, forReturning},
		Tables:     []campaign.DecisionTable{table},
	}); err != nil {
		t.Fatalf("treatments ordered by a table should be accepted, got %v", err)
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

	w.add(campaign.CampaignSpec{
		ID:       "gradual",
		Priority: campaign.NewPriority(10),
		Rollout:  admitted,
		Treatments: []campaign.Treatment{
			w.treatment("arm-one-banner", armOne, "a.png"),
			w.treatment("arm-two-banner", armTwo, "b.png"),
		},
	})

	var inside, outside int
	for _, person := range w.people {
		decision := w.decide(person.UID, audience.Live())
		if decision.Won {
			inside++
			continue
		}
		outside++
		losses := w.lossesFor(decision, "gradual")
		if len(losses) == 0 {
			t.Fatalf("%q is outside the rollout but nothing was recorded", person.UID)
		}
		// Both treatments are gated together, not one at a time.
		seen := map[campaign.TreatmentID]bool{}
		for _, loss := range losses {
			if loss.Reason != campaign.LossOutsideRollout {
				t.Fatalf("%q: recorded %q, expected being outside the rollout", person.UID, loss.Reason)
			}
			seen[loss.Treatment] = true
		}
		if !seen["arm-one-banner"] || !seen["arm-two-banner"] {
			t.Fatalf("%q: the rollout gated %v, expected both arms together", person.UID, seen)
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
	everyone := w.definition("anyone")
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID: "elsewhere", Slot: "unregistered-slot", Who: everyone, Call: w.call("x.png"),
	})
	if err != nil {
		t.Fatalf("building the treatment: %v", err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
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
		Rows:     []campaign.Row{{When: newcomers, Then: "not-held"}},
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		t.Fatalf("building the table: %v", err)
	}
	if _, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "stray-route", Priority: campaign.NewPriority(1),
		Treatments: []campaign.Treatment{held}, Tables: []campaign.DecisionTable{table},
	}); !errors.Is(err, campaign.ErrRowTreatmentNotInCampaign) {
		t.Fatalf("expected a stray-route refusal, got %v", err)
	}
}
