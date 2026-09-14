package experimentation_test

import (
	"testing"

	"github.com/snack8310/gmp/gmp-core/experimentation"
	"github.com/snack8310/gmp/gmp-core/experimentation/coloringtest"
)

// The in-memory store is the first implementation to run the shared contract.
// A real store is expected to call the same suite rather than write its own.
func TestMemoryColoringStoreMeetsTheContract(t *testing.T) {
	coloringtest.Run(t, func() experimentation.ColoringStore {
		return experimentation.NewMemoryColoringStore()
	})
}
