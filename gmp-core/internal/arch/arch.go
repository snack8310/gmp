// Package arch holds the dependency-direction check for gmp-core.
//
// It lives outside the packages it inspects on purpose. A check that shipped
// inside experimentation would disappear along with the package it guards, and
// deleting or renaming that package would then look like a clean run.
package arch

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var (
	// ErrNoSourcesExamined is returned when the directory under inspection
	// holds no Go files.
	//
	// Without it this whole check is discovery-driven and therefore able to
	// pass by finding nothing: rename the package and the walk visits zero
	// files, every assertion inside the loop runs zero times, and the check
	// reports success. That is the failure shape the repository has a rule
	// about, so the lower bound is part of the check rather than a nicety.
	ErrNoSourcesExamined = errors.New("arch: no Go sources were examined")
	// ErrForbiddenImport is returned when an inspected file imports a package
	// it is not allowed to depend on.
	ErrForbiddenImport = errors.New("arch: forbidden import")
)

// Violation is one forbidden import found in one file.
type Violation struct {
	File   string
	Import string
}

func (v Violation) String() string { return fmt.Sprintf("%s imports %q", v.File, v.Import) }

// Options tunes what ZeroDependency holds to the rule.
type Options struct {
	// ExemptExternalTests skips files declaring the external test package
	// (package X_test) when checking imports.
	//
	// Such a file is a consumer of the package, not part of it, and for an
	// upper layer it is the only place the whole stack gets wired together --
	// something has to construct the layers below in order to test through
	// them. Leave it false wherever the tests are expected to travel with the
	// package: the bottom layer is meant to be lifted out whole, tests
	// included, so a test of it reaching upward is as much of a problem as the
	// package itself doing so.
	//
	// In-package test files are never exempt: they are the package.
	ExemptExternalTests bool
}

// ZeroDependency parses every Go file under dir and reports any import whose
// path contains one of the forbidden fragments.
//
// It fails when dir cannot be read or holds no files it actually checked, so
// that a package which was renamed, deleted, or reduced to nothing but exempt
// files produces a failure rather than a quiet pass.
func ZeroDependency(dir string, forbidden []string, opts Options) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("arch: cannot inspect %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("arch: %s is not a directory", dir)
	}

	fset := token.NewFileSet()
	examined := 0
	var violations []Violation

	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("arch: parsing %s: %w", path, err)
		}
		if opts.ExemptExternalTests && strings.HasSuffix(file.Name.Name, "_test") {
			return nil
		}
		examined++
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("arch: reading an import path in %s: %w", path, err)
			}
			for _, fragment := range forbidden {
				if strings.Contains(imported, fragment) {
					violations = append(violations, Violation{File: path, Import: imported})
					break
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	if examined == 0 {
		return fmt.Errorf("%w: %s", ErrNoSourcesExamined, dir)
	}
	if len(violations) > 0 {
		sort.Slice(violations, func(i, j int) bool {
			if violations[i].File != violations[j].File {
				return violations[i].File < violations[j].File
			}
			return violations[i].Import < violations[j].Import
		})
		lines := make([]string, len(violations))
		for i, v := range violations {
			lines[i] = v.String()
		}
		return fmt.Errorf("%w: %s", ErrForbiddenImport, strings.Join(lines, "; "))
	}
	return nil
}
