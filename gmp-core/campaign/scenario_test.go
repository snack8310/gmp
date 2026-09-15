package campaign_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
)

// Scenario 3: someone opens the app and the home banner has to be filled in on
// the spot. A promotion and a win-back campaign both want it; inside the
// promotion, new and returning visitors see different images.
func TestScenarioThreeBannerContest(t *testing.T) {
	w := newWorld(t, 300)

	newcomers := w.definition("new-visitors", w.condition(audience.Attribute("tier", "new")))
	returning := w.definition("returning-visitors", w.condition(audience.Attribute("tier", "returning")))
	everyone := w.definition("anyone")

	promoA := w.treatment("promo-image-a", newcomers, "promo-a.png")
	promoB := w.treatment("promo-image-b", returning, "promo-b.png")
	table, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: bannerID,
		Rows: []campaign.Row{
			{When: newcomers, Then: promoA.ID()},
			{When: returning, Then: promoB.ID()},
		},
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		t.Fatalf("building the decision table: %v", err)
	}
	w.add(campaign.CampaignSpec{
		ID:         "double-eleven",
		Priority:   campaign.NewPriority(100),
		Treatments: []campaign.Treatment{promoA, promoB},
		Tables:     []campaign.DecisionTable{table},
	})

	winback := w.treatment("winback-banner", everyone, "winback.png")
	w.add(campaign.CampaignSpec{
		ID:         "member-winback",
		Priority:   campaign.NewPriority(50),
		Treatments: []campaign.Treatment{winback},
	})

	var (
		sawNew       bool
		sawReturning bool
		sawOther     bool
	)
	for _, person := range w.people {
		decision := w.decide(person.UID, audience.Live())
		tier := person.Attributes["tier"]
		switch tier {
		case "new", "returning":
			sawNew = sawNew || tier == "new"
			sawReturning = sawReturning || tier == "returning"
			if !decision.Won {
				t.Fatalf("%q (%s) got nothing; the promotion outranks the win-back and should have won", person.UID, tier)
			}
			if decision.Award.Campaign != "double-eleven" {
				t.Fatalf("%q (%s): %q won, expected the promotion", person.UID, tier, decision.Award.Campaign)
			}
			wantTreatment := campaign.TreatmentID("promo-image-a")
			wantImage := "promo-a.png"
			if tier == "returning" {
				wantTreatment, wantImage = "promo-image-b", "promo-b.png"
			}
			if decision.Award.Treatment != wantTreatment {
				t.Fatalf("%q (%s): treatment %q, expected %q", person.UID, tier, decision.Award.Treatment, wantTreatment)
			}
			if image, ok := decision.Award.Call.Param("image"); !ok || image != wantImage {
				t.Fatalf("%q (%s): image %q, expected %q", person.UID, tier, image, wantImage)
			}
			// The win-back lost, and that has to be findable. Otherwise its
			// operator asking why coverage is so low has nowhere to look.
			losses := w.lossesFor(decision, "member-winback")
			if len(losses) == 0 {
				t.Fatalf("%q: the win-back lost and left no trace", person.UID)
			}
			if losses[0].Reason != campaign.LossOnPriority {
				t.Fatalf("%q: the win-back lost for %q, expected losing on priority", person.UID, losses[0].Reason)
			}

		default:
			sawOther = true
			// The promotion's table routes them nowhere, so the lower-ranked
			// campaign gets the slot rather than it going empty.
			if !decision.Won {
				t.Fatalf("%q (%s) got nothing; the win-back should have taken the slot", person.UID, tier)
			}
			if decision.Award.Campaign != "member-winback" {
				t.Fatalf("%q (%s): %q won, expected the win-back", person.UID, tier, decision.Award.Campaign)
			}
		}
	}
	if !sawNew || !sawReturning || !sawOther {
		t.Fatalf("the population did not cover all three cases: new=%v returning=%v other=%v", sawNew, sawReturning, sawOther)
	}
}

// A table is walked top down and the first match wins. Separate audience
// definitions give no such guarantee: someone matching two of them leaves
// nobody able to say which applies.
func TestDecisionTableTakesTheFirstMatchingRow(t *testing.T) {
	w := newWorld(t, 90)

	newcomers := w.definition("new-visitors", w.condition(audience.Attribute("tier", "new")))
	flagged := w.definition("flagged-visitors", w.condition(audience.Attribute("flagged", "yes")))

	first := w.treatment("first-row", newcomers, "first.png")
	second := w.treatment("second-row", flagged, "second.png")
	// Someone both new and flagged catches either row, and neither row demands
	// everything the other does, so both are reachable. Order is then the whole
	// answer to which one runs.
	table, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: bannerID,
		Rows: []campaign.Row{
			{When: newcomers, Then: first.ID()},
			{When: flagged, Then: second.ID()},
		},
		Fallback: campaign.RouteNowhere(),
	})
	if err != nil {
		t.Fatalf("building the decision table: %v", err)
	}
	w.add(campaign.CampaignSpec{
		ID:         "ordered",
		Priority:   campaign.NewPriority(10),
		Treatments: []campaign.Treatment{first, second},
		Tables:     []campaign.DecisionTable{table},
	})

	var bothRows, secondRowOnly bool
	for _, person := range w.people {
		isNew := person.Attributes["tier"] == "new"
		isFlagged := person.Attributes["flagged"] == "yes"
		if !isNew && !isFlagged {
			continue
		}
		decision := w.decide(person.UID, audience.Live())
		switch {
		case isNew:
			bothRows = bothRows || isFlagged
			if decision.Award.Treatment != "first-row" {
				t.Fatalf("%q: %q won; the first matching row should have", person.UID, decision.Award.Treatment)
			}
		default:
			secondRowOnly = true
			if decision.Award.Treatment != "second-row" {
				t.Fatalf("%q matches only the second row but got %q", person.UID, decision.Award.Treatment)
			}
		}
	}
	if !bothRows {
		t.Fatal("nobody in the population matched both rows, so the ordering was never exercised")
	}
	if !secondRowOnly {
		t.Fatal("nobody matched only the second row, so it was never shown to be reachable")
	}
}

// Whoever matches no row has to have somewhere to go, even if that somewhere is
// nothing. A table without one drops a whole class of people silently.
func TestDecisionTableNeedsAFallback(t *testing.T) {
	w := newWorld(t, 10)
	newcomers := w.definition("new-visitors", w.condition(audience.Attribute("tier", "new")))
	only := w.treatment("only", newcomers, "only.png")
	_, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: bannerID,
		Rows: []campaign.Row{{When: newcomers, Then: only.ID()}},
	})
	if !errors.Is(err, campaign.ErrFallbackNotSet) {
		t.Fatalf("a table without a fallback should be refused, got %v", err)
	}
	if _, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot:     bannerID,
		Rows:     []campaign.Row{{When: newcomers, Then: only.ID()}},
		Fallback: campaign.RouteNowhere(),
	}); err != nil {
		t.Fatalf("an explicit fallback of nothing should be accepted, got %v", err)
	}
}

// A row asking for everything an earlier row asks for, and more, can never be
// reached. Its numbers would sit at zero and read as a poor result rather than
// as a row that never ran.
func TestDecisionTableRefusesARowNothingCanReach(t *testing.T) {
	w := newWorld(t, 10)
	newcomers := w.definition("new-visitors", w.condition(audience.Attribute("tier", "new")))
	newAndSomething := w.definition("new-and-flagged",
		w.condition(audience.Attribute("tier", "new")),
		w.condition(audience.Attribute("flagged", "yes")))

	broad := w.treatment("broad", newcomers, "broad.png")
	narrow := w.treatment("narrow", newAndSomething, "narrow.png")

	_, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: bannerID,
		Rows: []campaign.Row{
			{When: newcomers, Then: broad.ID()},
			{When: newAndSomething, Then: narrow.ID()},
		},
		Fallback: campaign.RouteNowhere(),
	})
	if !errors.Is(err, campaign.ErrRowNeverReached) {
		t.Fatalf("a row taken entirely by an earlier one should be refused, got %v", err)
	}

	// The other order is fine: the narrower row first, then the broader one.
	if _, err := campaign.NewDecisionTable(campaign.TableSpec{
		Slot: bannerID,
		Rows: []campaign.Row{
			{When: newAndSomething, Then: narrow.ID()},
			{When: newcomers, Then: broad.ID()},
		},
		Fallback: campaign.RouteNowhere(),
	}); err != nil {
		t.Fatalf("narrow before broad should be accepted, got %v", err)
	}
}
