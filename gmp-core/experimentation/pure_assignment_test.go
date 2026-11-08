package experimentation_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestCalculateAssignmentIsDeterministicWithoutAStore(t *testing.T) {
	split, err := experimentation.NewSplit(100, []int{25, 75})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID:        "homepage-banner",
		Split:     split,
		Salt:      "tenant-a:homepage-banner",
		Algorithm: experimentation.FNV1aV1{}.Version(),
		Colored:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	key := experimentation.AssignmentKey("member-42")
	first, err := experimentation.Calculate(definition, key, experimentation.FNV1aV1{})
	if err != nil {
		t.Fatalf("calculating first assignment: %v", err)
	}
	second, err := experimentation.Calculate(definition, key, experimentation.FNV1aV1{})
	if err != nil {
		t.Fatalf("calculating repeated assignment: %v", err)
	}
	if first != second {
		t.Fatalf("same explicit inputs returned %s and %s", first, second)
	}
}

func TestCalculateAssignmentRejectsAnAlgorithmVersionMismatch(t *testing.T) {
	split, err := experimentation.NewSplit(1, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID:        "algorithm-version-check",
		Split:     split,
		Salt:      "algorithm-version-check",
		Algorithm: "missing-v2",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = experimentation.Calculate(definition, "member-1", experimentation.FNV1aV1{})
	if !errors.Is(err, experimentation.ErrAlgorithmVersionMismatch) {
		t.Fatalf("got %v, want ErrAlgorithmVersionMismatch", err)
	}
}
