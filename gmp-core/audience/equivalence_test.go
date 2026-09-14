package audience_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

const backflowGranularity = time.Hour

func newExperiments(t *testing.T) (*experimentation.Service, *experimentation.MemoryColoringStore) {
	t.Helper()
	registry, err := experimentation.NewAlgorithmRegistry(experimentation.FNV1aV1{})
	if err != nil {
		t.Fatalf("building the algorithm registry: %v", err)
	}
	store := experimentation.NewMemoryColoringStore()
	service, err := experimentation.NewService(registry, store)
	if err != nil {
		t.Fatalf("building the experimentation service: %v", err)
	}
	return service, store
}

func mustAssignment(t *testing.T, id string, shares int, coloured bool) experimentation.Definition {
	t.Helper()
	split, err := experimentation.EqualSplit(shares)
	if err != nil {
		t.Fatalf("carving %d shares: %v", shares, err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID:        experimentation.AssignmentID(id),
		Split:     split,
		Salt:      experimentation.Salt(id),
		Algorithm: "fnv1a-v1",
		Colored:   coloured,
	})
	if err != nil {
		t.Fatalf("building assignment %q: %v", id, err)
	}
	return definition
}

func mustShare(t *testing.T, n int) experimentation.BucketNumber {
	t.Helper()
	share, err := experimentation.NewBucketNumber(n)
	if err != nil {
		t.Fatalf("building share %d: %v", n, err)
	}
	return share
}

func mustSource(t *testing.T) audience.Source {
	t.Helper()
	source, err := audience.RegisterSource(audience.SourceSpec{
		ID:                  "platform-backflow",
		Supply:              audience.SupplyPushStream,
		Category:            audience.CategoryBackflow,
		BackflowGranularity: backflowGranularity,
	})
	if err != nil {
		t.Fatalf("registering the source: %v", err)
	}
	return source
}

// sample builds a population with a spread of facts, so that a definition can
// actually divide it rather than matching everyone or nobody.
func sample(size int) []audience.Record {
	out := make([]audience.Record, size)
	for i := range out {
		uid := audience.UID(fmt.Sprintf("u-%05d", i))
		attributes := map[string]string{"tier": "regular"}
		if i%3 == 0 {
			attributes["tier"] = "new"
		}
		if i%5 == 0 {
			attributes["added-to-cart"] = "yes"
		}
		ages := map[string]time.Duration{}
		if i%2 == 0 {
			ages["sms-sent"] = time.Duration(i%9) * time.Hour
		}
		out[i] = audience.Record{UID: uid, Attributes: attributes, EventAges: ages}
	}
	return out
}

// conditions carries t so that a two-value constructor can be unwrapped in
// place: Go will not let a multi-value call sit among other arguments.
type conditions struct{ t *testing.T }

func (c conditions) must(condition audience.Condition, err error) audience.Condition {
	c.t.Helper()
	if err != nil {
		c.t.Fatalf("building a condition: %v", err)
	}
	return condition
}

// definitionsUnderTest covers the shapes that could make the two directions
// disagree: no conditions at all, plain facts, a share that reaches into the
// layer below, a time window, and several at once.
func definitionsUnderTest(t *testing.T, source audience.Source) []audience.Definition {
	t.Helper()
	cond := conditions{t: t}
	assignment := mustAssignment(t, "warmup", 4, false)
	build := func(id string, conditions ...audience.Condition) audience.Definition {
		definition, err := audience.NewDefinition(audience.DefinitionSpec{
			ID:         audience.DefinitionID(id),
			Source:     source.ID(),
			Conditions: conditions,
		})
		if err != nil {
			t.Fatalf("building definition %q: %v", id, err)
		}
		return definition
	}
	return []audience.Definition{
		build("everyone"),
		build("new-only", cond.must(audience.Attribute("tier", "new"))),
		build("second-share", cond.must(audience.Share(assignment, mustShare(t, 2)))),
		build("sms-two-hours-ago", cond.must(audience.ElapsedSince(source, "sms-sent", 2*time.Hour))),
		build("new-and-second-share",
			cond.must(audience.Attribute("tier", "new")),
			cond.must(audience.Share(assignment, mustShare(t, 2)))),
		build("cart-and-sms-and-share",
			cond.must(audience.Attribute("added-to-cart", "yes")),
			cond.must(audience.ElapsedSince(source, "sms-sent", 2*time.Hour)),
			cond.must(audience.Share(assignment, mustShare(t, 3)))),
		build("matches-nobody", cond.must(audience.Attribute("tier", "no-such-tier"))),
	}
}

// compareDirections walks every uid under every definition and reports where
// listing and deciding disagree. It is the harness the property rests on, and
// it is exercised against a deliberately broken decider below so that it is
// known to have teeth.
func compareDirections(t *testing.T, service *audience.Service, definitions []audience.Definition, population []audience.Record) []string {
	t.Helper()
	var disagreements []string
	for _, definition := range definitions {
		listed, err := service.Enumerate(definition, audience.Live())
		if err != nil {
			t.Fatalf("enumerating %q: %v", definition.ID(), err)
		}
		inList := make(map[audience.UID]bool, len(listed))
		for _, uid := range listed {
			inList[uid] = true
		}
		for _, record := range population {
			decided, err := service.Decide(definition, audience.Live(), record.UID)
			if err != nil {
				t.Fatalf("deciding %q under %q: %v", record.UID, definition.ID(), err)
			}
			if decided != inList[record.UID] {
				disagreements = append(disagreements, fmt.Sprintf(
					"%s: %s listed=%v decided=%v", definition.ID(), record.UID, inList[record.UID], decided))
			}
		}
	}
	return disagreements
}

// The assertion this whole layer exists for: one definition, two directions,
// the same answer. The slot-serving path only ever asks about one id, so a
// disagreement would never be reconciled against anything.
func TestListingAndDecidingAgree(t *testing.T) {
	experiments, _ := newExperiments(t)
	source := mustSource(t)
	population := sample(600)
	service := audience.NewService(experiments)
	if err := service.Register(source, audience.NewMemoryFeed(population...)); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}

	definitions := definitionsUnderTest(t, source)
	disagreements := compareDirections(t, service, definitions, population)
	if len(disagreements) > 0 {
		t.Fatalf("listing and deciding disagreed:\n  %v", disagreements)
	}

	// A comparison that never sees a true and a false proves nothing. Check the
	// population actually divides under these definitions.
	for _, definition := range definitions {
		listed, err := service.Enumerate(definition, audience.Live())
		if err != nil {
			t.Fatalf("enumerating %q: %v", definition.ID(), err)
		}
		switch definition.ID() {
		case "everyone":
			if len(listed) != len(population) {
				t.Fatalf("%q listed %d of %d", definition.ID(), len(listed), len(population))
			}
		case "matches-nobody":
			if len(listed) != 0 {
				t.Fatalf("%q listed %d, expected nobody", definition.ID(), len(listed))
			}
		default:
			if len(listed) == 0 || len(listed) == len(population) {
				t.Fatalf("%q listed %d of %d, so the comparison has no discriminating power",
					definition.ID(), len(listed), len(population))
			}
		}
	}
}

// brokenFeed answers Lookup differently from All for one uid, standing in for
// a future fast path on the decide side that drifts from the listing side.
type brokenFeed struct {
	inner  audience.Feed
	broken audience.UID
}

func (f brokenFeed) All() ([]audience.Record, error) { return f.inner.All() }

func (f brokenFeed) Lookup(uid audience.UID) (audience.Record, bool, error) {
	record, found, err := f.inner.Lookup(uid)
	if err != nil || !found {
		return record, found, err
	}
	if uid == f.broken {
		copied := map[string]string{}
		for k, v := range record.Attributes {
			copied[k] = v
		}
		copied["tier"] = "tampered"
		record.Attributes = copied
	}
	return record, found, nil
}

// The mutation the rule about non-vacuous checks asks for: if the two
// directions did drift, would the comparison above notice? It does.
func TestTheAgreementCheckWouldCatchDrift(t *testing.T) {
	experiments, _ := newExperiments(t)
	source := mustSource(t)
	population := sample(60)
	service := audience.NewService(experiments)
	feed := brokenFeed{inner: audience.NewMemoryFeed(population...), broken: population[0].UID}
	if err := service.Register(source, feed); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}

	cond := conditions{t: t}
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID:         "new-only",
		Source:     source.ID(),
		Conditions: []audience.Condition{cond.must(audience.Attribute("tier", "new"))},
	})
	if err != nil {
		t.Fatalf("building the definition: %v", err)
	}

	disagreements := compareDirections(t, service, []audience.Definition{definition}, population)
	if len(disagreements) == 0 {
		t.Fatal("a decider that disagrees with the listing went unnoticed: the agreement check is vacuous")
	}
}

// listingRefused fails any attempt to walk the source, so a decision that
// quietly enumerated would fail rather than pass.
type listingRefused struct{ inner audience.Feed }

var errListingRefused = errors.New("this feed refuses to be walked")

func (f listingRefused) All() ([]audience.Record, error) { return nil, errListingRefused }

func (f listingRefused) Lookup(uid audience.UID) (audience.Record, bool, error) {
	return f.inner.Lookup(uid)
}

// Deciding must not be implemented by listing everyone and searching. Listing
// may run offline for minutes; a decision happens with an app waiting.
func TestDecidingDoesNotWalkTheSource(t *testing.T) {
	experiments, _ := newExperiments(t)
	source := mustSource(t)
	population := sample(40)
	service := audience.NewService(experiments)
	if err := service.Register(source, listingRefused{inner: audience.NewMemoryFeed(population...)}); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}
	cond := conditions{t: t}
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID:         "new-only",
		Source:     source.ID(),
		Conditions: []audience.Condition{cond.must(audience.Attribute("tier", "new"))},
	})
	if err != nil {
		t.Fatalf("building the definition: %v", err)
	}

	if _, err := service.Decide(definition, audience.Live(), population[0].UID); err != nil {
		t.Fatalf("deciding walked the source: %v", err)
	}
	if _, err := service.Enumerate(definition, audience.Live()); !errors.Is(err, errListingRefused) {
		t.Fatalf("the listing side should have walked the source and failed, got %v", err)
	}
}
