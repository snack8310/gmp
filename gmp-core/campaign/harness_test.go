package campaign_test

import (
	"fmt"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

const (
	sourceID = audience.SourceID("app-profile")
	bannerID = campaign.SlotID("app-home-banner")
)

type world struct {
	t         *testing.T
	audiences *audience.Service
	campaigns *campaign.Service
	colouring *experimentation.MemoryColoringStore
	source    audience.Source
	people    []audience.Record
	show      campaign.Action
}

// newWorld wires the three layers together the way a real caller would: an
// experimentation service underneath, an audience service over it, and the
// campaign service on top.
func newWorld(t *testing.T, population int) *world {
	t.Helper()
	registry, err := experimentation.NewAlgorithmRegistry(experimentation.FNV1aV1{})
	if err != nil {
		t.Fatalf("building the algorithm registry: %v", err)
	}
	store := experimentation.NewMemoryColoringStore()
	experiments, err := experimentation.NewService(registry, store)
	if err != nil {
		t.Fatalf("building the experimentation service: %v", err)
	}

	source, err := audience.RegisterSource(audience.SourceSpec{
		ID:       sourceID,
		Supply:   audience.SupplyPull,
		Category: audience.CategoryExternal,
	})
	if err != nil {
		t.Fatalf("registering the source: %v", err)
	}

	people := make([]audience.Record, population)
	for i := range people {
		tier := "other"
		switch i % 3 {
		case 0:
			tier = "new"
		case 1:
			tier = "returning"
		}
		attributes := map[string]string{"tier": tier}
		// A second, independent fact: neither condition set contains the
		// other, so a table can have two rows that both catch some of the same
		// people without either row being unreachable.
		if i%4 == 0 {
			attributes["flagged"] = "yes"
		}
		people[i] = audience.Record{
			UID:        audience.UID(fmt.Sprintf("u-%05d", i)),
			Attributes: attributes,
		}
	}

	audiences := audience.NewService(experiments)
	if err := audiences.Register(source, audience.NewMemoryFeed(people...)); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}

	show, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: "show-image", Direction: campaign.DirectionPull, Required: []string{"image"},
	})
	if err != nil {
		t.Fatalf("registering the action: %v", err)
	}

	campaigns := campaign.NewService(audiences)
	slot, err := campaign.RegisterSlot(bannerID)
	if err != nil {
		t.Fatalf("registering the slot: %v", err)
	}
	if err := campaigns.RegisterSlot(slot); err != nil {
		t.Fatalf("putting the slot into service: %v", err)
	}

	return &world{
		t: t, audiences: audiences, campaigns: campaigns,
		colouring: store, source: source, people: people, show: show,
	}
}

func (w *world) definition(id string, conditions ...audience.Condition) audience.Definition {
	w.t.Helper()
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID:         audience.DefinitionID(id),
		Source:     sourceID,
		Conditions: conditions,
	})
	if err != nil {
		w.t.Fatalf("building audience definition %q: %v", id, err)
	}
	return definition
}

func (w *world) condition(condition audience.Condition, err error) audience.Condition {
	w.t.Helper()
	if err != nil {
		w.t.Fatalf("building a condition: %v", err)
	}
	return condition
}

func (w *world) call(image string) campaign.Call {
	w.t.Helper()
	call, err := w.show.Call(map[string]string{"image": image})
	if err != nil {
		w.t.Fatalf("filling in the action: %v", err)
	}
	return call
}

func (w *world) treatment(id string, who audience.Definition, image string) campaign.Treatment {
	w.t.Helper()
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{
		ID:   campaign.TreatmentID(id),
		Slot: bannerID,
		Who:  who,
		Call: w.call(image),
	})
	if err != nil {
		w.t.Fatalf("building treatment %q: %v", id, err)
	}
	return treatment
}

func (w *world) assignment(id string, shares int, coloured bool) experimentation.Definition {
	w.t.Helper()
	split, err := experimentation.EqualSplit(shares)
	if err != nil {
		w.t.Fatalf("carving %d shares: %v", shares, err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID:        experimentation.AssignmentID(id),
		Split:     split,
		Salt:      experimentation.Salt(id),
		Algorithm: "fnv1a-v1",
		Colored:   coloured,
	})
	if err != nil {
		w.t.Fatalf("building assignment %q: %v", id, err)
	}
	return definition
}

func (w *world) share(n int) experimentation.BucketNumber {
	w.t.Helper()
	share, err := experimentation.NewBucketNumber(n)
	if err != nil {
		w.t.Fatalf("building share %d: %v", n, err)
	}
	return share
}

func (w *world) add(spec campaign.CampaignSpec) campaign.Campaign {
	w.t.Helper()
	built, err := campaign.NewCampaign(spec)
	if err != nil {
		w.t.Fatalf("building campaign %q: %v", spec.ID, err)
	}
	if err := w.campaigns.Add(built); err != nil {
		w.t.Fatalf("putting campaign %q into service: %v", spec.ID, err)
	}
	return built
}

func (w *world) decide(uid audience.UID, mode audience.Mode) campaign.Decision {
	w.t.Helper()
	decision, err := w.campaigns.Decide(bannerID, uid, mode)
	if err != nil {
		w.t.Fatalf("deciding for %q: %v", uid, err)
	}
	return decision
}

func (w *world) lossesFor(decision campaign.Decision, id campaign.CampaignID) []campaign.Loss {
	var out []campaign.Loss
	for _, loss := range decision.Losses {
		if loss.Campaign == id {
			out = append(out, loss)
		}
	}
	return out
}
