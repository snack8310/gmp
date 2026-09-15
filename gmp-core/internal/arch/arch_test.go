package arch_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snack8310/gmp/gmp-core/internal/arch"
)

// layer is one rung of the dependency layering and what it may not reach for.
//
// The paths are relative to this package. Renaming or moving a guarded package
// breaks its path, and the mutations below turn that into a failure rather
// than a quiet pass.
type layer struct {
	name      string
	dir       string
	forbidden []string
	opts      arch.Options
}

// layers is the whole layering, stated once. The domain documents make zero
// dependency the precondition for lifting experimentation out whole, and
// inside one repository reaching upwards is a single extra import line --
// which is why this is a check and not a paragraph.
var layers = []layer{
	{
		name: "experimentation",
		dir:  "../../experimentation",
		forbidden: []string{
			"gmp-core/audience",
			"gmp-core/campaign",
			"gmp-core/scenarios",
		},
		// The bottom layer is meant to be lifted out whole, and the domain
		// documents say its tests travel with it rather than being rewritten.
		// So its test files are held to the rule too.
		opts: arch.Options{ExemptExternalTests: false},
	},
	{
		name: "audience",
		dir:  "../../audience",
		// audience may depend on experimentation, and on nothing above it.
		forbidden: []string{
			"gmp-core/campaign",
			"gmp-core/scenarios",
		},
		opts: arch.Options{ExemptExternalTests: true},
	},
	{
		name: "campaign",
		dir:  "../../campaign",
		// campaign may depend on audience, and on nothing above it. Reaching
		// experimentation directly is forbidden too: a rollout is expressed as
		// an audience definition precisely so this layer never has to.
		forbidden: []string{
			"gmp-core/experimentation",
			"gmp-core/scenarios",
		},
		// An upper layer's external test package is where the whole stack gets
		// wired together: testing through campaign means constructing the
		// layers underneath it. The package's own files are still held to the
		// rule, which is what the layering is actually about.
		opts: arch.Options{ExemptExternalTests: true},
	},
	{
		name: "scenarios",
		dir:  "../../scenarios",
		// Nothing sits above scenarios, so it has nothing to forbid. It is in
		// the table anyway: the completeness check below requires every layer
		// to be listed, and leaving this one out would make that check pass by
		// not knowing about it.
		forbidden: nil,
		opts:      arch.Options{ExemptExternalTests: true},
	},
}

// notLayers are directories under gmp-core that are deliberately outside the
// layering.
//
// cmd is the composition root: assembling the model is exactly the job of
// seeing every layer at once, so a rule against that would be a rule against
// the thing it exists to do. internal holds this check itself.
var notLayers = map[string]bool{"cmd": true, "internal": true}

// The table is the only place the layering is written down, so a layer missing
// from it is not checked at all -- and nothing would fail. Adding a package
// under gmp-core and forgetting to list it is the exact mistake this catches.
func TestEveryLayerIsInTheTable(t *testing.T) {
	const root = "../.."
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	listed := map[string]bool{}
	for _, l := range layers {
		listed[l.name] = true
	}

	found := 0
	for _, entry := range entries {
		if !entry.IsDir() || notLayers[entry.Name()] || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		holdsGo, err := filepath.Glob(filepath.Join(root, entry.Name(), "*.go"))
		if err != nil {
			t.Fatalf("looking for Go files in %s: %v", entry.Name(), err)
		}
		if len(holdsGo) == 0 {
			continue
		}
		found++
		if !listed[entry.Name()] {
			t.Fatalf("%q is a layer under gmp-core but is not in the layering table, so nothing checks it", entry.Name())
		}
	}
	if found == 0 {
		t.Fatal("no layers were found on disk, so this check asserted nothing")
	}
	if found != len(layers) {
		t.Fatalf("the table lists %d layers and %d were found on disk", len(layers), found)
	}
}

func TestNoLayerDependsOnOneAboveIt(t *testing.T) {
	for _, l := range layers {
		t.Run(l.name, func(t *testing.T) {
			if err := arch.ZeroDependency(l.dir, l.forbidden, l.opts); err != nil {
				t.Fatalf("%s must not depend on any upper layer: %v", l.name, err)
			}
		})
	}
	if len(layers) == 0 {
		t.Fatal("no layers are being checked, so this test asserts nothing")
	}
}

// The mutation the rule about discovery-driven checks asks for: if a guarded
// package is renamed away, the check must fail. It does, because the directory
// it was told to inspect is no longer there.
func TestCheckFailsWhenAGuardedPackageIsGone(t *testing.T) {
	for _, l := range layers {
		t.Run(l.name, func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "renamed-away")
			err := arch.ZeroDependency(missing, l.forbidden, l.opts)
			if err == nil {
				t.Fatal("a missing package directory must fail the check, it passed")
			}
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("expected a not-exist failure, got %v", err)
			}
		})
	}
}

// The other half of the same mutation: a directory that exists but holds no Go
// files must fail too. Without the lower bound the walk would visit nothing,
// every assertion would run zero times, and the check would report success.
func TestCheckFailsWhenNothingWasExamined(t *testing.T) {
	for _, l := range layers {
		t.Run(l.name, func(t *testing.T) {
			if err := arch.ZeroDependency(t.TempDir(), l.forbidden, l.opts); !errors.Is(err, arch.ErrNoSourcesExamined) {
				t.Fatalf("an empty directory must fail the check, got %v", err)
			}
		})
	}
}

// And the check must catch what it exists to catch, for every layer, rather
// than passing everything it is handed.
func TestCheckCatchesAnUpwardImport(t *testing.T) {
	upward := map[string]string{
		"experimentation": "github.com/snack8310/gmp/gmp-core/audience",
		"audience":        "github.com/snack8310/gmp/gmp-core/campaign",
		"campaign":        "github.com/snack8310/gmp/gmp-core/scenarios",
	}
	for _, l := range layers {
		t.Run(l.name, func(t *testing.T) {
			path, ok := upward[l.name]
			if len(l.forbidden) == 0 {
				// Nothing sits above this layer, so there is no upward import
				// to try. Skipping silently would let a layer that should have
				// a fixture lose one unnoticed, so say why.
				if ok {
					t.Fatalf("layer %q forbids nothing yet has an upward fixture", l.name)
				}
				t.Skipf("%q is the top layer: nothing above it to reach for", l.name)
			}
			if !ok {
				t.Fatalf("no upward import fixture for layer %q", l.name)
			}
			dir := t.TempDir()
			source := "package " + l.name + "\n\nimport \"" + path + "\"\n\nvar _ = " + filepath.Base(path) + ".Anything\n"
			if err := os.WriteFile(filepath.Join(dir, "leak.go"), []byte(source), 0o600); err != nil {
				t.Fatalf("writing the fixture: %v", err)
			}
			if err := arch.ZeroDependency(dir, l.forbidden, l.opts); !errors.Is(err, arch.ErrForbiddenImport) {
				t.Fatalf("an upward import must fail the check, got %v", err)
			}
		})
	}
}

// Depending downwards is allowed and must not be mistaken for a violation:
// audience is expected to reach experimentation.
func TestDownwardImportIsAllowed(t *testing.T) {
	dir := t.TempDir()
	source := `package audience

import (
	"hash/fnv"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

var (
	_ = fnv.New64a
	_ = experimentation.FNV1aV1{}
)
`
	if err := os.WriteFile(filepath.Join(dir, "fine.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	var found *layer
	for i := range layers {
		if layers[i].name == "audience" {
			found = &layers[i]
		}
	}
	if found == nil {
		t.Fatal("the audience layer is not in the table, so this test asserts nothing")
	}
	if err := arch.ZeroDependency(dir, found.forbidden, found.opts); err != nil {
		t.Fatalf("depending downwards must pass the check, got %v", err)
	}
}

// The exemption for external test files is a hole in the check, so it gets its
// own cases. An in-package test file is part of the package and stays held to
// the rule even where the exemption is on.
func TestInPackageTestFilesAreNeverExempt(t *testing.T) {
	dir := t.TempDir()
	source := `package campaign

import "github.com/snack8310/gmp/gmp-core/scenarios"

var _ = scenarios.Anything
`
	if err := os.WriteFile(filepath.Join(dir, "sneaky_test.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	opts := arch.Options{ExemptExternalTests: true}
	if err := arch.ZeroDependency(dir, []string{"gmp-core/scenarios"}, opts); !errors.Is(err, arch.ErrForbiddenImport) {
		t.Fatalf("an in-package test file reaching upward must fail even with the exemption on, got %v", err)
	}
}

// And a package that is nothing but exempt files has had nothing checked, which
// must read as a failure rather than a pass.
func TestOnlyExemptFilesCountsAsNothingExamined(t *testing.T) {
	dir := t.TempDir()
	source := `package campaign_test

import "github.com/snack8310/gmp/gmp-core/scenarios"

var _ = scenarios.Anything
`
	if err := os.WriteFile(filepath.Join(dir, "wiring_test.go"), []byte(source), 0o600); err != nil {
		t.Fatalf("writing the fixture: %v", err)
	}
	opts := arch.Options{ExemptExternalTests: true}
	if err := arch.ZeroDependency(dir, []string{"gmp-core/scenarios"}, opts); !errors.Is(err, arch.ErrNoSourcesExamined) {
		t.Fatalf("a package of nothing but exempt files must fail the check, got %v", err)
	}
	// With the exemption off, the same file is checked and caught.
	if err := arch.ZeroDependency(dir, []string{"gmp-core/scenarios"}, arch.Options{}); !errors.Is(err, arch.ErrForbiddenImport) {
		t.Fatalf("with the exemption off the same file must be caught, got %v", err)
	}
}
