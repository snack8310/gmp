package experimentation_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestComponentRuntimeParametersKeepColoringAndRevision(t *testing.T) {
	registry, err := experimentation.NewAlgorithmRegistry(experimentation.FNV1aV1{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := experimentation.NewService(registry, experimentation.NewMemoryColoringStore())
	if err != nil {
		t.Fatal(err)
	}
	split, err := experimentation.NewSplit(100, []int{50, 50})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: "component", Split: split, Salt: "component-salt", Algorithm: "fnv1a-v1", Colored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterComponent(definition); err != nil {
		t.Fatal(err)
	}
	generation, err := service.ComponentGeneration(definition)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Assign(definition, "person-1"); !errors.Is(err, experimentation.ErrComponentNotRunning) {
		t.Fatalf("before start: %v", err)
	}
	if err := service.StartComponent(definition.ID()); err != nil {
		t.Fatal(err)
	}
	if !service.ComponentActive(definition, generation) {
		t.Fatal("started component is inactive")
	}

	key := experimentation.AssignmentKey("person-1")
	first, err := service.Assign(definition, key)
	if err != nil {
		t.Fatal(err)
	}
	newKey := experimentation.AssignmentKey("person-2")
	base, err := service.PeekAssign(definition, newKey)
	if err != nil {
		t.Fatal(err)
	}
	for i := 3; base == first && i < 100; i++ {
		newKey = experimentation.AssignmentKey(fmt.Sprintf("person-%d", i))
		base, err = service.PeekAssign(definition, newKey)
		if err != nil {
			t.Fatal(err)
		}
	}
	if base == first {
		t.Fatal("fixture did not produce both random shares")
	}
	wanted := first
	if err := service.UpdateWhitelist(definition.ID(), map[experimentation.AssignmentKey]experimentation.BucketNumber{key: base}); err != nil {
		t.Fatal(err)
	}
	if got, err := service.Assign(definition, key); err != nil || got != first {
		t.Fatalf("existing coloring moved to %v: %v", got, err)
	}
	if err := service.UpdateWhitelist(definition.ID(), map[experimentation.AssignmentKey]experimentation.BucketNumber{key: base, newKey: wanted}); err != nil {
		t.Fatal(err)
	}
	if got, err := service.Assign(definition, newKey); err != nil || got != wanted {
		t.Fatalf("new whitelist placement = %v: %v", got, err)
	}
	allSecond, err := experimentation.NewSplit(100, []int{0, 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateSplit(definition.ID(), allSecond); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateWhitelist(definition.ID(), nil); err != nil {
		t.Fatal(err)
	}
	if got, err := service.Assign(definition, newKey); err != nil || got != wanted {
		t.Fatalf("colored whitelist placement moved to %v: %v", got, err)
	}
	if got, err := service.Assign(definition, "ratio-new"); err != nil || got.Value() != 2 {
		t.Fatalf("updated ratio placement = %v: %v", got, err)
	}
	if !service.ComponentActive(definition, generation) {
		t.Fatal("runtime update retired component")
	}

	other, err := experimentation.NewDefinition(experimentation.DefinitionSpec{ID: definition.ID(), Split: split, Salt: "different", Algorithm: "fnv1a-v1", Colored: true})
	if err != nil {
		t.Fatal(err)
	}
	three, err := experimentation.EqualSplit(3)
	if err != nil {
		t.Fatal(err)
	}
	changedAlgorithm, err := experimentation.NewDefinition(experimentation.DefinitionSpec{ID: definition.ID(), Split: split, Salt: "component-salt", Algorithm: "another-version", Colored: true})
	if err != nil {
		t.Fatal(err)
	}
	changedStructure, err := experimentation.NewDefinition(experimentation.DefinitionSpec{ID: definition.ID(), Split: three, Salt: "component-salt", Algorithm: "fnv1a-v1", Colored: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []experimentation.Definition{definition, other, changedAlgorithm, changedStructure} {
		if err := service.ReviseComponent(candidate); !errors.Is(err, experimentation.ErrComponentRunning) {
			t.Fatalf("running logic revision: %v", err)
		}
	}
	if err := service.UpdateSplit(definition.ID(), three); !errors.Is(err, experimentation.ErrComponentStructure) {
		t.Fatalf("structural change: %v", err)
	}
	if err := service.UpdateWhitelist(definition.ID(), map[experimentation.AssignmentKey]experimentation.BucketNumber{"": wanted}); err == nil {
		t.Fatal("empty whitelist key accepted")
	}
	if err := service.StopComponent(definition.ID()); err != nil {
		t.Fatal(err)
	}
	if err := service.StartComponent(definition.ID()); err != nil {
		t.Fatal(err)
	}
	if service.ComponentActive(definition, generation) {
		t.Fatal("old generation revived")
	}
	if err := service.StopComponent(definition.ID()); err != nil {
		t.Fatal(err)
	}
	if err := service.ReviseComponent(other); err != nil {
		t.Fatal(err)
	}
	if err := service.StartComponent(definition.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Assign(definition, "person-4"); !errors.Is(err, experimentation.ErrComponentRevision) {
		t.Fatalf("old revision assigned: %v", err)
	}
}
