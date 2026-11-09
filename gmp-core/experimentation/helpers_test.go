package experimentation_test

import (
	"fmt"
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

const algorithmUnderTest = experimentation.AlgorithmVersion("fnv1a-v1")

type pureCalculator struct{}

func (pureCalculator) Assign(definition experimentation.Definition, key experimentation.AssignmentKey) (experimentation.BucketNumber, error) {
	return experimentation.Calculate(definition, key, experimentation.FNV1aV1{})
}

func mustEqualSplit(t *testing.T, shares int) experimentation.Split {
	t.Helper()
	split, err := experimentation.EqualSplit(shares)
	if err != nil {
		t.Fatal(err)
	}
	return split
}

func mustDefine(t *testing.T, spec experimentation.DefinitionSpec) experimentation.Definition {
	t.Helper()
	definition, err := experimentation.NewDefinition(spec)
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func keys(prefix string, count int) []experimentation.AssignmentKey {
	out := make([]experimentation.AssignmentKey, count)
	for index := range out {
		out[index] = experimentation.AssignmentKey(fmt.Sprintf("%s-%d", prefix, index))
	}
	return out
}
