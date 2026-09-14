package experimentation_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

// Coloring exists for exactly one job: when the definition changes, the people
// already assigned do not move, and only newcomers follow the new definition.
func TestColoringKeepsAlreadyAssignedKeysInPlace(t *testing.T) {
	service, store := newService(t)
	before := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "arm-count-change",
		Split:     mustEqualSplit(t, 3),
		Salt:      "arm-count-change",
		Algorithm: algorithmUnderTest,
		Colored:   true,
	})

	early := keys("early", 200)
	assigned := make(map[experimentation.AssignmentKey]experimentation.BucketNumber, len(early))
	for _, key := range early {
		bucket, err := service.Assign(before, key)
		if err != nil {
			t.Fatalf("assigning %q: %v", key, err)
		}
		assigned[key] = bucket
	}

	// The definition is immutable, so changing it means building another one
	// under the same assignment id. Coloring is filed under that id, which is
	// what lets the records outlive the change.
	after := mustDefine(t, experimentation.DefinitionSpec{
		ID:        before.ID(),
		Split:     mustEqualSplit(t, 4),
		Salt:      "arm-count-change",
		Algorithm: algorithmUnderTest,
		Colored:   true,
	})

	for key, was := range assigned {
		now, err := service.Assign(after, key)
		if err != nil {
			t.Fatalf("re-assigning %q after the change: %v", key, err)
		}
		if now != was {
			t.Fatalf("coloured key %q jumped from %s to %s when the definition changed", key, was, now)
		}
	}

	// Newcomers are bucketed by the new definition, not by the old one. An
	// uncoloured twin of the new definition shows what that should be.
	fresh := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "arm-count-change-twin",
		Split:     mustEqualSplit(t, 4),
		Salt:      "arm-count-change",
		Algorithm: algorithmUnderTest,
	})
	newcomers := keys("late", 200)
	for _, key := range newcomers {
		got, err := service.Assign(after, key)
		if err != nil {
			t.Fatalf("assigning newcomer %q: %v", key, err)
		}
		want, err := service.Assign(fresh, key)
		if err != nil {
			t.Fatalf("assigning newcomer %q under the twin: %v", key, err)
		}
		if got != want {
			t.Fatalf("newcomer %q got %s, the new definition puts it in %s", key, got, want)
		}
	}

	if store.Size() != len(early)+len(newcomers) {
		t.Fatalf("store holds %d records, expected one per coloured key", store.Size())
	}
}

// Asking twice must not write twice. A second record for one key would mean
// two answers exist for a question that is supposed to have one.
func TestColoringIsIdempotent(t *testing.T) {
	service, store := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "idempotent",
		Split:     mustEqualSplit(t, 4),
		Salt:      "idempotent",
		Algorithm: algorithmUnderTest,
		Colored:   true,
	})

	key := experimentation.AssignmentKey("shopper-1")
	first, err := service.Assign(definition, key)
	if err != nil {
		t.Fatalf("first assignment: %v", err)
	}
	if store.Size() != 1 {
		t.Fatalf("one assignment wrote %d records", store.Size())
	}
	for round := 0; round < 10; round++ {
		again, err := service.Assign(definition, key)
		if err != nil {
			t.Fatalf("assignment round %d: %v", round, err)
		}
		if again != first {
			t.Fatalf("round %d returned %s, first returned %s", round, again, first)
		}
	}
	if store.Size() != 1 {
		t.Fatalf("repeated assignments grew the store to %d records", store.Size())
	}
}

// A rehearsal must not have side effects. Asking the ordinary way would colour
// the key and the rehearsal would have changed the real run it was previewing.
func TestReadOnlyAskWritesNothing(t *testing.T) {
	service, store := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "rehearsal",
		Split:     mustEqualSplit(t, 4),
		Salt:      "rehearsal",
		Algorithm: algorithmUnderTest,
		Colored:   true,
	})

	for _, key := range keys("rehearsed", 300) {
		if _, err := service.PeekAssign(definition, key); err != nil {
			t.Fatalf("read-only ask for %q: %v", key, err)
		}
	}
	if store.Size() != 0 {
		t.Fatalf("a read-only ask wrote %d coloring records", store.Size())
	}

	// And the answer it gives is the one the real run would give.
	for _, key := range keys("rehearsed", 50) {
		peeked, err := service.PeekAssign(definition, key)
		if err != nil {
			t.Fatalf("read-only ask for %q: %v", key, err)
		}
		real, err := service.Assign(definition, key)
		if err != nil {
			t.Fatalf("assigning %q: %v", key, err)
		}
		if peeked != real {
			t.Fatalf("key %q: rehearsal said %s, the real run said %s", key, peeked, real)
		}
	}
}

// A rehearsal over an already coloured key reports the colour rather than
// recomputing it, and still writes nothing.
func TestReadOnlyAskReportsExistingColour(t *testing.T) {
	service, store := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "rehearsal-after-colour",
		Split:     mustEqualSplit(t, 3),
		Salt:      "rehearsal-after-colour",
		Algorithm: algorithmUnderTest,
		Colored:   true,
	})

	key := experimentation.AssignmentKey("shopper-1")
	coloured, err := service.Assign(definition, key)
	if err != nil {
		t.Fatalf("assigning %q: %v", key, err)
	}
	sizeAfterColouring := store.Size()

	wider := mustDefine(t, experimentation.DefinitionSpec{
		ID:        definition.ID(),
		Split:     mustEqualSplit(t, 7),
		Salt:      "rehearsal-after-colour",
		Algorithm: algorithmUnderTest,
		Colored:   true,
	})
	peeked, err := service.PeekAssign(wider, key)
	if err != nil {
		t.Fatalf("read-only ask for %q: %v", key, err)
	}
	if peeked != coloured {
		t.Fatalf("rehearsal said %s, the colour on record is %s", peeked, coloured)
	}
	if store.Size() != sizeAfterColouring {
		t.Fatalf("a read-only ask changed the store from %d to %d records", sizeAfterColouring, store.Size())
	}
}

// The in-memory store cannot produce a real race, so this asserts the contract
// instead: concurrent first assignments for one key all see one winner. What
// the guarantee is worth under a real store is for that store to demonstrate.
func TestConcurrentFirstAssignmentYieldsOneShare(t *testing.T) {
	service, store := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "concurrent-first",
		Split:     mustEqualSplit(t, 8),
		Salt:      "concurrent-first",
		Algorithm: algorithmUnderTest,
		Colored:   true,
	})

	key := experimentation.AssignmentKey("shopper-1")
	const racers = 64
	results := make([]experimentation.BucketNumber, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = service.Assign(definition, key)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("racer %d: %v", i, err)
		}
	}
	for i, got := range results {
		if got != results[0] {
			t.Fatalf("racer %d got %s, racer 0 got %s", i, got, results[0])
		}
	}
	if store.Size() != 1 {
		t.Fatalf("concurrent first assignment wrote %d records", store.Size())
	}
}

// A definition that colours is unusable without somewhere to record. Failing
// here beats silently behaving like an uncoloured assignment, which would look
// correct until the definition changed and everyone jumped.
func TestColouredDefinitionNeedsAStore(t *testing.T) {
	registry, err := experimentation.NewAlgorithmRegistry(experimentation.FNV1aV1{})
	if err != nil {
		t.Fatalf("building the algorithm registry: %v", err)
	}
	service, err := experimentation.NewService(registry, nil)
	if err != nil {
		t.Fatalf("building the service: %v", err)
	}
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "no-store",
		Split:     mustEqualSplit(t, 2),
		Salt:      "no-store",
		Algorithm: algorithmUnderTest,
		Colored:   true,
	})
	if _, err := service.Assign(definition, "shopper-1"); !errors.Is(err, experimentation.ErrNoColoringStore) {
		t.Fatalf("expected a missing-store failure, got %v", err)
	}
}
