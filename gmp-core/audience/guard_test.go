package audience_test

import (
	"errors"
	"testing"
	"time"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

// A rehearsal must leave no trace. Asking the ordinary way would colour every
// person it touched, and the real run would then find them already pinned --
// the preview having changed what it was previewing.
func TestRehearsalColoursNobody(t *testing.T) {
	experiments, store := newExperiments(t)
	source := mustSource(t)
	population := sample(300)
	service := audience.NewService(experiments)
	if err := service.Register(source, audience.NewMemoryFeed(population...)); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}

	cond := conditions{t: t}
	coloured := mustAssignment(t, "coloured-warmup", 4, true)
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID:     "coloured-second-share",
		Source: source.ID(),
		Conditions: []audience.Condition{
			cond.must(audience.Attribute("tier", "new")),
			cond.must(audience.Share(coloured, mustShare(t, 2))),
		},
	})
	if err != nil {
		t.Fatalf("building the definition: %v", err)
	}

	if _, err := service.Enumerate(definition, audience.Rehearsal()); err != nil {
		t.Fatalf("rehearsing the listing: %v", err)
	}
	for _, record := range population[:50] {
		if _, err := service.Decide(definition, audience.Rehearsal(), record.UID); err != nil {
			t.Fatalf("rehearsing a decision: %v", err)
		}
	}
	if store.Size() != 0 {
		t.Fatalf("a rehearsal wrote %d colouring records", store.Size())
	}

	// And the real thing does colour, so the check above is not passing simply
	// because nothing ever colours.
	if _, err := service.Enumerate(definition, audience.Live()); err != nil {
		t.Fatalf("the real listing: %v", err)
	}
	if store.Size() == 0 {
		t.Fatal("the real run coloured nobody, so the rehearsal assertion proves nothing")
	}
}

// Forgetting to say which mode it is must fail rather than quietly running the
// dangerous one.
func TestModeMustBeSet(t *testing.T) {
	experiments, _ := newExperiments(t)
	source := mustSource(t)
	service := audience.NewService(experiments)
	if err := service.Register(source, audience.NewMemoryFeed(sample(10)...)); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}
	definition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "everyone", Source: source.ID()})
	if err != nil {
		t.Fatalf("building the definition: %v", err)
	}

	var forgotten audience.Mode
	if _, err := service.Enumerate(definition, forgotten); !errors.Is(err, audience.ErrModeNotSet) {
		t.Fatalf("listing without a mode should fail, got %v", err)
	}
	if _, err := service.Decide(definition, forgotten, "u-00000"); !errors.Is(err, audience.ErrModeNotSet) {
		t.Fatalf("deciding without a mode should fail, got %v", err)
	}
	if forgotten.IsRehearsal() {
		t.Fatal("the zero mode reads as a rehearsal, which would make forgetting it silently safe-looking")
	}
}

// Asking about a stretch of time shorter than the source can see answers
// "nobody" for everyone, and the platform then acts on that without failing.
func TestWindowShorterThanTheSourceCanSeeIsRefused(t *testing.T) {
	source := mustSource(t)
	_, err := audience.ElapsedSince(source, "sms-sent", 10*time.Minute)
	if !errors.Is(err, audience.ErrWindowShorterThanGranularity) {
		t.Fatalf("a ten-minute window over an hourly source should be refused, got %v", err)
	}
	if _, err := audience.ElapsedSince(source, "sms-sent", backflowGranularity); err != nil {
		t.Fatalf("a window equal to the granularity should be accepted, got %v", err)
	}
	if _, err := audience.ElapsedSince(source, "sms-sent", 4*time.Hour); err != nil {
		t.Fatalf("a longer window should be accepted, got %v", err)
	}
}

// A source that never said how far behind it runs cannot carry a time window.
// There is no hour-level constant to fall back on: the documents say that
// about this platform's own backflow, not about any particular source.
func TestTimeWindowNeedsADeclaredGranularity(t *testing.T) {
	undeclared, err := audience.RegisterSource(audience.SourceSpec{
		ID:       "orders",
		Supply:   audience.SupplyPull,
		Category: audience.CategoryExternal,
	})
	if err != nil {
		t.Fatalf("registering a source without a granularity: %v", err)
	}
	if _, err := audience.ElapsedSince(undeclared, "sms-sent", time.Hour); !errors.Is(err, audience.ErrGranularityNotDeclared) {
		t.Fatalf("expected a missing-granularity refusal, got %v", err)
	}
}

// A share condition must be answered by the experimentation layer, not by
// anything this package works out for itself.
func TestShareConditionAgreesWithTheLayerBelow(t *testing.T) {
	experiments, _ := newExperiments(t)
	source := mustSource(t)
	population := sample(400)
	service := audience.NewService(experiments)
	if err := service.Register(source, audience.NewMemoryFeed(population...)); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}

	cond := conditions{t: t}
	assignment := mustAssignment(t, "warmup", 4, false)
	wanted := mustShare(t, 3)
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID:         "third-share",
		Source:     source.ID(),
		Conditions: []audience.Condition{cond.must(audience.Share(assignment, wanted))},
	})
	if err != nil {
		t.Fatalf("building the definition: %v", err)
	}

	listed, err := service.Enumerate(definition, audience.Live())
	if err != nil {
		t.Fatalf("enumerating: %v", err)
	}
	inList := make(map[audience.UID]bool, len(listed))
	for _, uid := range listed {
		inList[uid] = true
	}
	for _, record := range population {
		share, err := experiments.PeekAssign(assignment, experimentation.AssignmentKey(record.UID))
		if err != nil {
			t.Fatalf("asking the layer below about %q: %v", record.UID, err)
		}
		if want := share == wanted; inList[record.UID] != want {
			t.Fatalf("%q: audience says %v, the layer below puts it in %s", record.UID, inList[record.UID], share)
		}
	}
	if len(listed) == 0 || len(listed) == len(population) {
		t.Fatalf("the share condition selected %d of %d, so this proves nothing", len(listed), len(population))
	}
}

func TestSourceRefusesAnUnknownSupplyOrCategory(t *testing.T) {
	if _, err := audience.RegisterSource(audience.SourceSpec{ID: "s", Category: audience.CategoryExternal}); !errors.Is(err, audience.ErrUnknownSupply) {
		t.Fatalf("expected an unknown-supply refusal, got %v", err)
	}
	if _, err := audience.RegisterSource(audience.SourceSpec{ID: "s", Supply: audience.SupplyPull}); !errors.Is(err, audience.ErrUnknownCategory) {
		t.Fatalf("expected an unknown-category refusal, got %v", err)
	}
	if _, err := audience.RegisterSource(audience.SourceSpec{Supply: audience.SupplyPull, Category: audience.CategoryExternal}); !errors.Is(err, audience.ErrMissingSourceID) {
		t.Fatalf("expected a missing-id refusal, got %v", err)
	}
}

func TestDefinitionRefusesWhatItCannotEvaluate(t *testing.T) {
	if _, err := audience.NewDefinition(audience.DefinitionSpec{Source: "s"}); !errors.Is(err, audience.ErrMissingDefinitionID) {
		t.Fatalf("expected a missing-id refusal, got %v", err)
	}
	if _, err := audience.NewDefinition(audience.DefinitionSpec{ID: "d"}); !errors.Is(err, audience.ErrMissingSourceID) {
		t.Fatalf("expected a missing-source refusal, got %v", err)
	}
	if _, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: "d", Source: "s", Conditions: []audience.Condition{nil},
	}); !errors.Is(err, audience.ErrNilCondition) {
		t.Fatalf("expected a nil-condition refusal, got %v", err)
	}
}

func TestShareConditionRefusesAnImpossibleShare(t *testing.T) {
	assignment := mustAssignment(t, "four-shares", 4, false)
	if _, err := audience.Share(assignment, mustShare(t, 5)); !errors.Is(err, experimentation.ErrBucketOutOfRange) {
		t.Fatalf("a fifth share of a four-share assignment should be refused, got %v", err)
	}
	if _, err := audience.Share(experimentation.Definition{}, mustShare(t, 1)); !errors.Is(err, audience.ErrUnbuiltShareCondition) {
		t.Fatalf("expected an unbuilt-assignment refusal, got %v", err)
	}
}

func TestServiceRefusesWhatItCannotResolve(t *testing.T) {
	experiments, _ := newExperiments(t)
	service := audience.NewService(experiments)
	definition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "d", Source: "never-registered"})
	if err != nil {
		t.Fatalf("building the definition: %v", err)
	}
	if _, err := service.Enumerate(definition, audience.Live()); !errors.Is(err, audience.ErrUnknownSource) {
		t.Fatalf("expected an unknown-source refusal, got %v", err)
	}
	if _, err := service.Enumerate(audience.Definition{}, audience.Live()); !errors.Is(err, audience.ErrUnbuiltDefinition) {
		t.Fatalf("expected an unbuilt-definition refusal, got %v", err)
	}
	if _, err := service.Decide(definition, audience.Live(), ""); !errors.Is(err, audience.ErrEmptyUID) {
		t.Fatalf("expected an empty-uid refusal, got %v", err)
	}
}

// A uid the source has never heard of is simply not in the audience.
func TestUnknownPersonIsNotInTheAudience(t *testing.T) {
	experiments, _ := newExperiments(t)
	source := mustSource(t)
	service := audience.NewService(experiments)
	if err := service.Register(source, audience.NewMemoryFeed(sample(10)...)); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}
	definition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "everyone", Source: source.ID()})
	if err != nil {
		t.Fatalf("building the definition: %v", err)
	}
	in, err := service.Decide(definition, audience.Live(), "someone-else-entirely")
	if err != nil {
		t.Fatalf("deciding about a stranger: %v", err)
	}
	if in {
		t.Fatal("a uid the source never yielded came back as part of the audience")
	}
}
