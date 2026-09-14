package arch_test

import (
	"errors"
	"os"
	"path/filepath"
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
	},
	{
		name: "audience",
		dir:  "../../audience",
		// audience may depend on experimentation, and on nothing above it.
		forbidden: []string{
			"gmp-core/campaign",
			"gmp-core/scenarios",
		},
	},
}

func TestNoLayerDependsOnOneAboveIt(t *testing.T) {
	for _, l := range layers {
		t.Run(l.name, func(t *testing.T) {
			if err := arch.ZeroDependency(l.dir, l.forbidden); err != nil {
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
			err := arch.ZeroDependency(missing, l.forbidden)
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
			if err := arch.ZeroDependency(t.TempDir(), l.forbidden); !errors.Is(err, arch.ErrNoSourcesExamined) {
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
	}
	for _, l := range layers {
		t.Run(l.name, func(t *testing.T) {
			path, ok := upward[l.name]
			if !ok {
				t.Fatalf("no upward import fixture for layer %q", l.name)
			}
			dir := t.TempDir()
			source := "package " + l.name + "\n\nimport \"" + path + "\"\n\nvar _ = " + filepath.Base(path) + ".Anything\n"
			if err := os.WriteFile(filepath.Join(dir, "leak.go"), []byte(source), 0o600); err != nil {
				t.Fatalf("writing the fixture: %v", err)
			}
			if err := arch.ZeroDependency(dir, l.forbidden); !errors.Is(err, arch.ErrForbiddenImport) {
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
	var forbidden []string
	for _, l := range layers {
		if l.name == "audience" {
			forbidden = l.forbidden
		}
	}
	if forbidden == nil {
		t.Fatal("the audience layer is not in the table, so this test asserts nothing")
	}
	if err := arch.ZeroDependency(dir, forbidden); err != nil {
		t.Fatalf("depending downwards must pass the check, got %v", err)
	}
}
