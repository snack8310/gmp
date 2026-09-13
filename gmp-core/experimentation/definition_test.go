package experimentation_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

// What an operator configures is "aligned with that assignment", not a salt.
// The platform takes the target's salt and split, so the same key lands in the
// same share under both.
func TestAlignedAssignmentLandsKeysWhereItsTargetDoes(t *testing.T) {
	service, _ := newService(t)
	target := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "phase-one",
		Split:     mustEqualSplit(t, 4),
		Salt:      "phase-one-salt",
		Algorithm: algorithmUnderTest,
	})
	aligned, err := experimentation.NewAlignedDefinition(experimentation.DefinitionSpec{
		ID:        "phase-two",
		Algorithm: algorithmUnderTest,
	}, target)
	if err != nil {
		t.Fatalf("aligning phase two with phase one: %v", err)
	}
	if aligned.AlignedTo() != target.ID() {
		t.Fatalf("aligned definition points at %q, expected %q", aligned.AlignedTo(), target.ID())
	}

	for _, key := range keys("shopper", 500) {
		here, err := service.Assign(target, key)
		if err != nil {
			t.Fatalf("assigning %q under the target: %v", key, err)
		}
		there, err := service.Assign(aligned, key)
		if err != nil {
			t.Fatalf("assigning %q under the aligned assignment: %v", key, err)
		}
		if here != there {
			t.Fatalf("key %q is %s under the target and %s under the aligned assignment", key, here, there)
		}
	}
}

// Matching the salt while carving differently does not put the same key in the
// same share, and nothing about it fails on its own. So it is refused outright.
func TestAlignmentRefusesAnIncompatibleSplit(t *testing.T) {
	target := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "four-arms",
		Split:     mustEqualSplit(t, 4),
		Salt:      "four-arms",
		Algorithm: algorithmUnderTest,
	})
	_, err := experimentation.NewAlignedDefinition(experimentation.DefinitionSpec{
		ID:        "two-arms",
		Split:     mustEqualSplit(t, 2),
		Algorithm: algorithmUnderTest,
	}, target)
	if !errors.Is(err, experimentation.ErrIncompatibleSplit) {
		t.Fatalf("expected an incompatible-split refusal, got %v", err)
	}
}

// Restating the target's own split is not a conflict, so it is allowed.
func TestAlignmentAcceptsAnIdenticalSplit(t *testing.T) {
	target := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "four-arms",
		Split:     mustEqualSplit(t, 4),
		Salt:      "four-arms",
		Algorithm: algorithmUnderTest,
	})
	if _, err := experimentation.NewAlignedDefinition(experimentation.DefinitionSpec{
		ID:        "four-arms-again",
		Split:     mustEqualSplit(t, 4),
		Algorithm: algorithmUnderTest,
	}, target); err != nil {
		t.Fatalf("restating the same split should be accepted, got %v", err)
	}
}

// The salt is the platform's business, not the operator's. An aligned
// assignment that states one is refused rather than having it ignored.
func TestAlignmentRefusesItsOwnSalt(t *testing.T) {
	target := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "target",
		Split:     mustEqualSplit(t, 4),
		Salt:      "target-salt",
		Algorithm: algorithmUnderTest,
	})
	_, err := experimentation.NewAlignedDefinition(experimentation.DefinitionSpec{
		ID:        "aligned",
		Salt:      "a-salt-of-its-own",
		Algorithm: algorithmUnderTest,
	}, target)
	if err == nil {
		t.Fatal("an aligned assignment stating its own salt should be refused")
	}
}

// Chaining alignment is refused. The spec settles what aligning with one
// existing assignment means and nothing else; a chain also admits a cycle.
func TestAlignmentIsNotTransitive(t *testing.T) {
	first := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "a",
		Split:     mustEqualSplit(t, 4),
		Salt:      "a-salt",
		Algorithm: algorithmUnderTest,
	})
	second, err := experimentation.NewAlignedDefinition(experimentation.DefinitionSpec{
		ID:        "b",
		Algorithm: algorithmUnderTest,
	}, first)
	if err != nil {
		t.Fatalf("aligning b with a: %v", err)
	}
	if _, err := experimentation.NewAlignedDefinition(experimentation.DefinitionSpec{
		ID:        "c",
		Algorithm: algorithmUnderTest,
	}, second); !errors.Is(err, experimentation.ErrAlignmentNotTransitive) {
		t.Fatalf("expected a non-transitive refusal, got %v", err)
	}
}

func TestAlignmentRefusesAnUnbuiltTarget(t *testing.T) {
	if _, err := experimentation.NewAlignedDefinition(experimentation.DefinitionSpec{
		ID:        "aligned",
		Algorithm: algorithmUnderTest,
	}, experimentation.Definition{}); !errors.Is(err, experimentation.ErrMissingAssignmentID) {
		t.Fatalf("expected an unbuilt-target refusal, got %v", err)
	}
}

// Go would fill a missing algorithm version with the empty string and the
// assignment would look complete while being unreplayable. Construction fails
// instead: there is no default to invent.
func TestDefinitionRequiresAnAlgorithmVersion(t *testing.T) {
	_, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID:    "no-algorithm",
		Split: mustEqualSplit(t, 4),
		Salt:  "no-algorithm",
	})
	if !errors.Is(err, experimentation.ErrMissingAlgorithmVersion) {
		t.Fatalf("expected a missing-algorithm-version refusal, got %v", err)
	}

	target := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "target",
		Split:     mustEqualSplit(t, 4),
		Salt:      "target",
		Algorithm: algorithmUnderTest,
	})
	if _, err := experimentation.NewAlignedDefinition(experimentation.DefinitionSpec{ID: "aligned"}, target); !errors.Is(err, experimentation.ErrMissingAlgorithmVersion) {
		t.Fatalf("an aligned definition must state its algorithm version too, got %v", err)
	}
}

// The salt is never generated here: the documents make tenant isolation depend
// on what is inside it but never say how one is composed.
func TestDefinitionRequiresASalt(t *testing.T) {
	if _, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID:        "no-salt",
		Split:     mustEqualSplit(t, 4),
		Algorithm: algorithmUnderTest,
	}); !errors.Is(err, experimentation.ErrMissingSalt) {
		t.Fatalf("expected a missing-salt refusal, got %v", err)
	}
}

func TestDefinitionRequiresAnIDAndASplit(t *testing.T) {
	if _, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		Split:     mustEqualSplit(t, 4),
		Salt:      "salt",
		Algorithm: algorithmUnderTest,
	}); !errors.Is(err, experimentation.ErrMissingAssignmentID) {
		t.Fatalf("expected a missing-id refusal, got %v", err)
	}
	if _, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID:        "no-split",
		Salt:      "salt",
		Algorithm: algorithmUnderTest,
	}); !errors.Is(err, experimentation.ErrMissingSplit) {
		t.Fatalf("expected a missing-split refusal, got %v", err)
	}
}

func TestServiceRefusesAnUnbuiltDefinition(t *testing.T) {
	service, _ := newService(t)
	if _, err := service.Assign(experimentation.Definition{}, "shopper-1"); !errors.Is(err, experimentation.ErrUnbuiltDefinition) {
		t.Fatalf("expected an unbuilt-definition refusal, got %v", err)
	}
}

func TestServiceRefusesAnEmptyKey(t *testing.T) {
	service, _ := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "empty-key",
		Split:     mustEqualSplit(t, 4),
		Salt:      "empty-key",
		Algorithm: algorithmUnderTest,
	})
	if _, err := service.Assign(definition, ""); !errors.Is(err, experimentation.ErrEmptyKey) {
		t.Fatalf("expected an empty-key refusal, got %v", err)
	}
}

// A long key is not an error: the key is opaque and the caller decides what
// goes in it.
func TestServiceAcceptsALongKey(t *testing.T) {
	service, _ := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "long-key",
		Split:     mustEqualSplit(t, 4),
		Salt:      "long-key",
		Algorithm: algorithmUnderTest,
	})
	long := experimentation.AssignmentKey(strings.Repeat("k", 100_000))
	first, err := service.Assign(definition, long)
	if err != nil {
		t.Fatalf("assigning a long key: %v", err)
	}
	again, err := service.Assign(definition, long)
	if err != nil {
		t.Fatalf("re-assigning a long key: %v", err)
	}
	if first != again {
		t.Fatalf("a long key moved from %s to %s", first, again)
	}
}

// An assignment recorded under an algorithm the registry no longer holds must
// fail rather than be recomputed under today's algorithm.
func TestServiceRefusesAnUnknownAlgorithm(t *testing.T) {
	service, _ := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "retired-algorithm",
		Split:     mustEqualSplit(t, 4),
		Salt:      "retired-algorithm",
		Algorithm: "fnv1a-v0",
	})
	if _, err := service.Assign(definition, "shopper-1"); !errors.Is(err, experimentation.ErrUnknownAlgorithm) {
		t.Fatalf("expected an unknown-algorithm refusal, got %v", err)
	}
}

// Rebinding a version would change how already recorded assignments replay.
func TestAlgorithmVersionsCannotBeRebound(t *testing.T) {
	if _, err := experimentation.NewAlgorithmRegistry(experimentation.FNV1aV1{}, experimentation.FNV1aV1{}); !errors.Is(err, experimentation.ErrDuplicateAlgorithm) {
		t.Fatalf("expected a duplicate-version refusal, got %v", err)
	}
}
