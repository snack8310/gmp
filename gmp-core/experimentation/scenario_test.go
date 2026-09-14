package experimentation_test

import (
	"fmt"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

const algorithmUnderTest = experimentation.AlgorithmVersion("fnv1a-v1")

func newService(t *testing.T) (*experimentation.Service, *experimentation.MemoryColoringStore) {
	t.Helper()
	registry, err := experimentation.NewAlgorithmRegistry(experimentation.FNV1aV1{})
	if err != nil {
		t.Fatalf("building the algorithm registry: %v", err)
	}
	store := experimentation.NewMemoryColoringStore()
	service, err := experimentation.NewService(registry, store)
	if err != nil {
		t.Fatalf("building the service: %v", err)
	}
	return service, store
}

func mustEqualSplit(t *testing.T, shares int) experimentation.Split {
	t.Helper()
	split, err := experimentation.EqualSplit(shares)
	if err != nil {
		t.Fatalf("carving %d equal shares: %v", shares, err)
	}
	return split
}

func mustDefine(t *testing.T, spec experimentation.DefinitionSpec) experimentation.Definition {
	t.Helper()
	definition, err := experimentation.NewDefinition(spec)
	if err != nil {
		t.Fatalf("building definition %q: %v", spec.ID, err)
	}
	return definition
}

func keys(prefix string, count int) []experimentation.AssignmentKey {
	out := make([]experimentation.AssignmentKey, count)
	for i := range out {
		out[i] = experimentation.AssignmentKey(fmt.Sprintf("%s-%d", prefix, i))
	}
	return out
}

// Scenario 2, the grouping half: an experiment carves its audience into four
// arms, and the same person must stay in the same arm every time they are
// asked about.
func TestBucketingIsStable(t *testing.T) {
	service, _ := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "double-eleven-warmup",
		Split:     mustEqualSplit(t, 4),
		Salt:      "double-eleven-warmup-2026",
		Algorithm: algorithmUnderTest,
	})

	for _, key := range keys("shopper", 500) {
		first, err := service.Assign(definition, key)
		if err != nil {
			t.Fatalf("assigning %q: %v", key, err)
		}
		for round := 0; round < 5; round++ {
			again, err := service.Assign(definition, key)
			if err != nil {
				t.Fatalf("re-assigning %q: %v", key, err)
			}
			if again != first {
				t.Fatalf("key %q moved from %s to %s on round %d", key, first, again, round)
			}
		}
	}
}

// Scenario 4: a new mechanism is rolled out to five percent, then twenty, then
// everyone. Whoever is already in must not drop out at the next step -- the
// reason the rollout has to ride on bucketing rather than on random sampling.
func TestRolloutIsMonotonic(t *testing.T) {
	service, _ := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "auto-coupon-rollout",
		Split:     mustEqualSplit(t, 100),
		Salt:      "auto-coupon-rollout-2026",
		Algorithm: algorithmUnderTest,
	})

	included := func(upTo int) map[experimentation.AssignmentKey]bool {
		in := make(map[experimentation.AssignmentKey]bool)
		for _, key := range keys("member", 2000) {
			bucket, err := service.Assign(definition, key)
			if err != nil {
				t.Fatalf("assigning %q: %v", key, err)
			}
			if bucket.Value() <= upTo {
				in[key] = true
			}
		}
		return in
	}

	atFive := included(5)
	atTwenty := included(20)
	atHundred := included(100)

	if len(atFive) == 0 {
		t.Fatal("nobody was included at five percent, so the test asserts nothing")
	}
	for key := range atFive {
		if !atTwenty[key] {
			t.Fatalf("key %q was in at five percent and dropped out at twenty", key)
		}
	}
	for key := range atTwenty {
		if !atHundred[key] {
			t.Fatalf("key %q was in at twenty percent and dropped out at full rollout", key)
		}
	}
	if len(atHundred) != 2000 {
		t.Fatalf("full rollout should hold every key, held %d", len(atHundred))
	}
}

// Every assignment scatters on its own by default, so that one unlucky group
// does not end up in the control arm of every experiment ever run.
func TestSeparateAssignmentsScatterIndependently(t *testing.T) {
	service, _ := newService(t)
	first := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "experiment-e",
		Split:     mustEqualSplit(t, 4),
		Salt:      "experiment-e",
		Algorithm: algorithmUnderTest,
	})
	second := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "experiment-f",
		Split:     mustEqualSplit(t, 4),
		Salt:      "experiment-f",
		Algorithm: algorithmUnderTest,
	})

	sample := keys("shopper", 1000)
	agreements := 0
	for _, key := range sample {
		a, err := service.Assign(first, key)
		if err != nil {
			t.Fatalf("assigning %q under the first assignment: %v", key, err)
		}
		b, err := service.Assign(second, key)
		if err != nil {
			t.Fatalf("assigning %q under the second assignment: %v", key, err)
		}
		if a == b {
			agreements++
		}
	}

	// Four shares means chance alone puts about a quarter of the keys in the
	// same place twice. A salt that failed to scatter would put all of them
	// there, so the bound only has to exclude the degenerate case.
	if agreements == len(sample) {
		t.Fatal("every key landed in the same share under both assignments: the salt is not scattering")
	}
	if agreements > len(sample)/2 {
		t.Fatalf("%d of %d keys agreed across two assignments, far above chance", agreements, len(sample))
	}
}

// Two shares of one assignment are mutually exclusive by construction: a key
// is in exactly one of them. The relaxed slot-contention rule leans on this.
func TestSharesOfOneAssignmentAreMutuallyExclusive(t *testing.T) {
	service, _ := newService(t)
	definition := mustDefine(t, experimentation.DefinitionSpec{
		ID:        "exclusivity",
		Split:     mustEqualSplit(t, 4),
		Salt:      "exclusivity",
		Algorithm: algorithmUnderTest,
	})

	for _, key := range keys("shopper", 500) {
		bucket, err := service.Assign(definition, key)
		if err != nil {
			t.Fatalf("assigning %q: %v", key, err)
		}
		hits := 0
		for share := 1; share <= definition.Split().Shares(); share++ {
			if bucket.Value() == share {
				hits++
			}
		}
		if hits != 1 {
			t.Fatalf("key %q matched %d shares, expected exactly one", key, hits)
		}
	}
}
