package experimentation_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestSplitRefusesNoShares(t *testing.T) {
	if _, err := experimentation.NewSplit(10, nil); !errors.Is(err, experimentation.ErrNoShares) {
		t.Fatalf("expected a no-shares refusal, got %v", err)
	}
	if _, err := experimentation.EqualSplit(0); !errors.Is(err, experimentation.ErrNoShares) {
		t.Fatalf("expected a no-shares refusal from the equal split, got %v", err)
	}
}

func TestSplitRefusesANonPositiveTotal(t *testing.T) {
	for _, total := range []int{0, -1} {
		if _, err := experimentation.NewSplit(total, []int{1}); !errors.Is(err, experimentation.ErrNonPositiveTotal) {
			t.Fatalf("total %d: expected a non-positive-total refusal, got %v", total, err)
		}
	}
}

func TestSplitRefusesANegativeWeight(t *testing.T) {
	if _, err := experimentation.NewSplit(10, []int{12, -2}); !errors.Is(err, experimentation.ErrNegativeWeight) {
		t.Fatalf("expected a negative-weight refusal, got %v", err)
	}
}

// Ratios that do not account for the whole space are refused rather than
// normalised. Normalising would invent a rule the documents never stated, and
// the caller would never learn their configuration was incomplete.
func TestSplitRefusesWeightsThatDoNotSumToTheTotal(t *testing.T) {
	if _, err := experimentation.NewSplit(100, []int{50, 30, 10}); !errors.Is(err, experimentation.ErrWeightsDoNotSumToTotal) {
		t.Fatalf("expected a weights-do-not-sum refusal, got %v", err)
	}
	if _, err := experimentation.NewSplit(100, []int{50, 30, 30}); !errors.Is(err, experimentation.ErrWeightsDoNotSumToTotal) {
		t.Fatalf("weights overshooting the total should be refused too, got %v", err)
	}
}

// A single share is legal and swallows everything.
func TestSingleShareTakesEveryKey(t *testing.T) {
	service, _ := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "single-share",
		Split:     mustEqualSplit(t, 1),
		Salt:      "single-share",
		Algorithm: algorithmUnderTest,
	})
	for _, key := range keys("shopper", 200) {
		bucket, err := service.Assign(definition, key)
		if err != nil {
			t.Fatalf("assigning %q: %v", key, err)
		}
		if bucket.Value() != 1 {
			t.Fatalf("key %q landed in %s, a single-share split has only the first", key, bucket)
		}
	}
}

// Three equal shares of a total of three is exact -- no rounding, and every
// share is actually reachable.
func TestRatiosThatDoNotDivideEvenlyStayExact(t *testing.T) {
	service, _ := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "thirds",
		Split:     mustEqualSplit(t, 3),
		Salt:      "thirds",
		Algorithm: algorithmUnderTest,
	})
	seen := make(map[int]bool)
	for _, key := range keys("shopper", 500) {
		bucket, err := service.Assign(definition, key)
		if err != nil {
			t.Fatalf("assigning %q: %v", key, err)
		}
		seen[bucket.Value()] = true
	}
	for share := 1; share <= 3; share++ {
		if !seen[share] {
			t.Fatalf("share %d was never reached across the sample", share)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("keys landed in shares %v, the split has three", seen)
	}
}

// A share with no weight owns none of the space and must never come back.
func TestZeroWeightShareIsNeverAssigned(t *testing.T) {
	service, _ := newService(t)
	split, err := experimentation.NewSplit(10, []int{5, 0, 5})
	if err != nil {
		t.Fatalf("carving a split with an empty middle share: %v", err)
	}
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "empty-middle",
		Split:     split,
		Salt:      "empty-middle",
		Algorithm: algorithmUnderTest,
	})
	for _, key := range keys("shopper", 1000) {
		bucket, err := service.Assign(definition, key)
		if err != nil {
			t.Fatalf("assigning %q: %v", key, err)
		}
		if bucket.Value() == 2 {
			t.Fatalf("key %q landed in the zero-weight share", key)
		}
	}
}

// Shares are counted from one, matching how the domain documents name them.
// Nothing may ever hand back the zero value as if it were a share.
func TestShareNumbersAreCountedFromOne(t *testing.T) {
	service, _ := newService(t)
	const shares = 4
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "one-based",
		Split:     mustEqualSplit(t, shares),
		Salt:      "one-based",
		Algorithm: algorithmUnderTest,
	})
	for _, key := range keys("shopper", 1000) {
		bucket, err := service.Assign(definition, key)
		if err != nil {
			t.Fatalf("assigning %q: %v", key, err)
		}
		if bucket.IsZero() {
			t.Fatalf("key %q came back with the zero value instead of a share", key)
		}
		if bucket.Value() < 1 || bucket.Value() > shares {
			t.Fatalf("key %q landed in %s, outside the shares that exist", key, bucket)
		}
	}

	if _, err := experimentation.NewBucketNumber(0); !errors.Is(err, experimentation.ErrBucketOutOfRange) {
		t.Fatalf("share zero should not be constructible, got %v", err)
	}
	if _, err := experimentation.NewBucketNumber(-1); !errors.Is(err, experimentation.ErrBucketOutOfRange) {
		t.Fatalf("a negative share should not be constructible, got %v", err)
	}
}
