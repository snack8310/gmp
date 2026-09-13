package arch_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/snack8310/gmp/gmp-core/internal/arch"
)

// experimentationDir is where the guarded package lives relative to this one.
// Renaming or moving that package breaks this path, and the tests below turn
// that into a failure rather than a quiet pass.
const experimentationDir = "../../experimentation"

// upperLayers are the layers experimentation must never reach for. The domain
// documents make zero dependency the precondition for lifting the package out
// whole, and inside one repository reaching upwards is a single extra import
// line -- which is why this is a check and not a paragraph.
var upperLayers = []string{
	"gmp-core/audience",
	"gmp-core/campaign",
	"gmp-core/scenarios",
}

func TestExperimentationDependsOnNoUpperLayer(t *testing.T) {
	if err := arch.ZeroDependency(experimentationDir, upperLayers); err != nil {
		t.Fatalf("experimentation must not depend on any upper layer: %v", err)
	}
}

// The mutation the rule about discovery-driven checks asks for: if the guarded
// package is renamed away, the check must fail. It does, because the directory
// it was told to inspect is no longer there.
func TestCheckFailsWhenTheGuardedPackageIsGone(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "renamed-away")
	err := arch.ZeroDependency(missing, upperLayers)
	if err == nil {
		t.Fatal("a missing package directory must fail the check, it passed")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected a not-exist failure, got %v", err)
	}
}

// The other half of the same mutation: a directory that exists but holds no Go
// files must fail too. Without the lower bound the walk would visit nothing,
// every assertion would run zero times, and the check would report success.
func TestCheckFailsWhenNothingWasExamined(t *testing.T) {
	empty := t.TempDir()
	if err := arch.ZeroDependency(empty, upperLayers); !errors.Is(err, arch.ErrNoSourcesExamined) {
		t.Fatalf("an empty directory must fail the check, got %v", err)
	}
}

// And the check must actually catch what it exists to catch, rather than
// passing everything it is handed.
func TestCheckCatchesAnUpwardImport(t *testing.T) {
	dir := t.TempDir()
	source := `package experimentation

import "github.com/snack8310/gmp/gmp-core/campaign"

var _ = campaign.Anything
`
	if err := os.WriteFile(filepath.Join(dir, "leak.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	err := arch.ZeroDependency(dir, upperLayers)
	if !errors.Is(err, arch.ErrForbiddenImport) {
		t.Fatalf("an upward import must fail the check, got %v", err)
	}
}

// A file that imports only the standard library and its own module is fine,
// so the check is not simply failing everything it sees.
func TestCheckAcceptsAllowedImports(t *testing.T) {
	dir := t.TempDir()
	source := `package experimentation

import (
	"hash/fnv"

	"github.com/snack8310/gmp/gmp-core/experimentation/coloringtest"
)

var (
	_ = fnv.New64a
	_ = coloringtest.Run
)
`
	if err := os.WriteFile(filepath.Join(dir, "fine.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	if err := arch.ZeroDependency(dir, upperLayers); err != nil {
		t.Fatalf("allowed imports must pass the check, got %v", err)
	}
}
