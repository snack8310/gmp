package experimentation_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestResolveAssignmentUsesExistingColoring(t *testing.T) {
	definition := pureTestDefinition(t, "resolve-existing", true)
	winner, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	existing := experimentation.ColoringRecord{Bucket: winner, Algorithm: "retired-algorithm"}

	got, err := experimentation.ResolveAssignment(
		definition, "uid-1", experimentation.FNV1aV1{}, nil, &existing,
	)
	if err != nil {
		t.Fatalf("resolving existing coloring: %v", err)
	}
	if got.Bucket != winner || got.NeedsPersist {
		t.Fatalf("assignment = %#v, want existing bucket %v without a write", got, winner)
	}
}

func TestResolveAssignmentReturnsColoringIntent(t *testing.T) {
	definition := pureTestDefinition(t, "resolve-new", true)

	got, err := experimentation.ResolveAssignment(
		definition, "uid-2", experimentation.FNV1aV1{}, nil, nil,
	)
	if err != nil {
		t.Fatalf("resolving new coloring: %v", err)
	}
	if got.Bucket.IsZero() || !got.NeedsPersist {
		t.Fatalf("assignment = %#v, want a valid bucket and a coloring write intent", got)
	}
	if got.Coloring.Bucket != got.Bucket || got.Coloring.Algorithm != definition.Algorithm() {
		t.Fatalf("coloring = %#v does not describe assignment %#v", got.Coloring, got)
	}
}

func TestComponentTransitionsReturnNewState(t *testing.T) {
	definition := pureTestDefinition(t, "component-transition", false)
	initial, err := experimentation.NewComponentState(definition)
	if err != nil {
		t.Fatal(err)
	}

	started, err := initial.Start()
	if err != nil {
		t.Fatal(err)
	}
	if initial.Active(definition, 1) {
		t.Fatal("starting the returned state changed the original value")
	}
	if !started.Active(definition, 1) {
		t.Fatal("started state is not active at its generation")
	}

	stopped, err := started.Stop()
	if err != nil {
		t.Fatal(err)
	}
	if !started.Active(definition, 1) {
		t.Fatal("stopping the returned state changed the original value")
	}
	if stopped.Active(definition, 1) || stopped.Active(definition, 2) {
		t.Fatal("stopped state remained active")
	}

	restarted, err := stopped.Start()
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Active(definition, 1) || !restarted.Active(definition, 2) {
		t.Fatal("restart did not advance and use the new generation")
	}
}

func TestComponentWhitelistTransitionKeepsPriorValuesImmutable(t *testing.T) {
	definition := pureTestDefinition(t, "component-whitelist-transition", true)
	state, err := experimentation.NewComponentState(definition)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.Start()
	if err != nil {
		t.Fatal(err)
	}
	first, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	key := experimentation.AssignmentKey("uid-immutable")
	input := map[experimentation.AssignmentKey]experimentation.BucketNumber{key: first}
	prior, err := state.UpdateWhitelist(input)
	if err != nil {
		t.Fatal(err)
	}
	input[key] = second
	if got, ok := prior.WhitelistFor(key); !ok || got != first {
		t.Fatalf("mutating the input map changed returned state: got %v, present=%t", got, ok)
	}

	updated, err := prior.UpdateWhitelist(input)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := prior.WhitelistFor(key); !ok || got != first {
		t.Fatalf("updating the returned state changed its predecessor: got %v, present=%t", got, ok)
	}
	if got, ok := updated.WhitelistFor(key); !ok || got != second {
		t.Fatalf("updated whitelist = %v, present=%t; want bucket %v", got, ok, second)
	}
}

func TestComponentRejectsInvalidSplitAndWhitelist(t *testing.T) {
	definition := pureTestDefinition(t, "component-validation", true)
	state, err := experimentation.NewComponentState(definition)
	if err != nil {
		t.Fatal(err)
	}
	state, err = state.Start()
	if err != nil {
		t.Fatal(err)
	}

	threeShares, err := experimentation.EqualSplit(3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateSplit(threeShares); !errors.Is(err, experimentation.ErrComponentStructure) {
		t.Fatalf("structural split change error = %v, want ErrComponentStructure", err)
	}

	first, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	three, err := experimentation.NewBucketNumber(3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.UpdateWhitelist(map[experimentation.AssignmentKey]experimentation.BucketNumber{"": first}); err == nil {
		t.Fatal("empty whitelist key was accepted")
	}
	if _, err := state.UpdateWhitelist(map[experimentation.AssignmentKey]experimentation.BucketNumber{"uid-3": three}); !errors.Is(err, experimentation.ErrBucketOutOfRange) {
		t.Fatalf("out-of-range whitelist error = %v, want ErrBucketOutOfRange", err)
	}
}

func pureTestDefinition(t *testing.T, id string, colored bool) experimentation.Definition {
	t.Helper()
	split, err := experimentation.NewSplit(100, []int{50, 50})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: experimentation.AssignmentID(id), Split: split, Salt: experimentation.Salt(id),
		Algorithm: experimentation.FNV1aV1{}.Version(), Colored: colored,
	})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}
