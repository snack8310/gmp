// Package arch holds the dependency-direction check for gmp-core.
//
// It lives outside the packages it inspects on purpose. A check that shipped
// inside experimentation would disappear along with the package it guards, and
// deleting or renaming that package would then look like a clean run.
package arch

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
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
	ErrForbiddenImport           = errors.New("arch: forbidden import")
	ErrForbiddenRuntimeOwnership = errors.New("arch: Core declares a runtime owner or external-effect dependency")
	ErrNoTypeDeclarations        = errors.New("arch: no type declarations were examined")
)

var forbiddenRuntimeImports = []string{
	"context", "database/sql", "io", "log", "net", "os", "sync",
}

var allowedCoreImports = map[string]map[string]bool{
	"audience": {
		"errors": true, "fmt": true, "time": true,
		"github.com/snack8310/gmp/gmp-core/experimentation": true,
	},
	"experimentation": {"errors": true, "fmt": true, "hash/fnv": true},
	"campaign": {
		"errors": true, "fmt": true, "sort": true, "strconv": true, "strings": true,
		"github.com/snack8310/gmp/gmp-core/audience": true,
	},
	"scenarios": {},
}

var forbiddenClockCalls = map[string]bool{
	"After": true, "AfterFunc": true, "NewTicker": true, "NewTimer": true,
	"Now": true, "Since": true, "Sleep": true, "Tick": true, "Until": true,
}

var forbiddenFormattedIOCalls = map[string]bool{
	"Fprint": true, "Fprintf": true, "Fprintln": true,
	"Fscan": true, "Fscanf": true, "Fscanln": true,
	"Print": true, "Printf": true, "Println": true,
	"Scan": true, "Scanf": true, "Scanln": true,
}

// Reference fields are reviewed individually. A new reference field requires
// an explicit boundary review so an ordinary-named runtime owner cannot
// silently enter Core.
var allowedCoreValueReferenceFields = map[string]string{
	"audience.BundleSpec.Arms":                 "[]BundleArm",
	"audience.Bundle.arms":                     "map[int]Group",
	"audience.Definition.conditions":           "[]Condition",
	"audience.DefinitionSpec.Conditions":       "[]Condition",
	"audience.evaluation.facts":                "AssignmentFacts",
	"audience.Record.Attributes":               "map[string]string",
	"audience.Record.EventAges":                "map[string]time.Duration",
	"audience.Selection.bundle":                "*Bundle",
	"experimentation.Split.weights":            "[]int",
	"experimentation.ComponentState.whitelist": "map[AssignmentKey]BucketNumber",
	"campaign.Execution.Placements":            "[]audience.Placement",
	"campaign.Execution.ReceiptAttributes":     "map[string]string",
	"campaign.Event.Placements":                "[]audience.Placement",
	"campaign.Event.Attributes":                "map[string]string",
	"campaign.Receipt.Attributes":              "map[string]string",
	"campaign.Action.required":                 "[]string",
	"campaign.Action.outcomes":                 "[]Outcome",
	"campaign.ActionSpec.Required":             "[]string",
	"campaign.ActionSpec.Outcomes":             "[]Outcome",
	"campaign.Call.params":                     "map[string]string",
	"campaign.Call.outcomes":                   "[]Outcome",
	"campaign.Award.Placements":                "[]audience.Placement",
	"campaign.Decision.Losses":                 "[]Loss",
	"campaign.EntryRoute.treatments":           "[]TreatmentID",
	"campaign.EntrySpec.Treatments":            "[]TreatmentID",
	"campaign.Flow.entries":                    "[]EntryRoute",
	"campaign.Flow.edges":                      "[]Edge",
	"campaign.FlowSpec.Entries":                "[]EntrySpec",
	"campaign.FlowSpec.Edges":                  "[]EdgeSpec",
	"campaign.DecisionTable.rows":              "[]Row",
	"campaign.TableSpec.Rows":                  "[]Row",
	"campaign.Campaign.treatments":             "[]Treatment",
	"campaign.Campaign.tables":                 "map[SlotID]DecisionTable",
	"campaign.CampaignSpec.Treatments":         "[]Treatment",
	"campaign.CampaignSpec.Tables":             "[]DecisionTable",
	"campaign.DeliveryIntent.Placements":       "[]audience.Placement",
	"campaign.ReceiptTransition.Deliveries":    "[]DeliveryIntent",
	"campaign.Plan.Placements":                 "[]audience.Placement",
	"campaign.Report.Planned":                  "[]Plan",
	"campaign.Report.Executions":               "[]Execution",
}

var allowedCoreReferenceTypes = map[string]bool{
	"audience.AssignmentFacts": true,
}

var allowedCoreInterfaceTypes = map[string]bool{
	"audience.Condition":        true,
	"experimentation.Algorithm": true,
}

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

type typeContext struct {
	packageName string
	imports     map[string]string
	dotImports  []string
	typeParams  map[string]bool
}

type typeDeclaration struct {
	expression ast.Expr
	context    typeContext
}

type packageTypeIndex map[string]map[string]typeDeclaration

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

// NoRuntimeOwnership rejects storage, application orchestration, external
// effect ports, package-level mutable state, pointer receiver methods, and
// unreviewed mutable references inside one Core package.
func NoRuntimeOwnership(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("arch: cannot inspect %s: %w", dir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("arch: %s is not a directory", dir)
	}

	type sourceFile struct {
		path string
		file *ast.File
	}
	fset := token.NewFileSet()
	examined, typeDeclarations := 0, 0
	var violations []string
	var sources []sourceFile
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return fmt.Errorf("arch: parsing %s: %w", path, err)
		}
		if !strings.HasSuffix(path, "_test.go") {
			examined++
		}
		sources = append(sources, sourceFile{path: path, file: file})
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	packageTypes, err := corePackageTypeIndex(dir)
	if err != nil {
		return err
	}
	for _, source := range sources {
		if strings.HasSuffix(source.path, "_test.go") {
			continue
		}
		context, err := contextForFile(source.file)
		if err != nil {
			return fmt.Errorf("arch: reading types in %s: %w", source.path, err)
		}
		if packageTypes[context.packageName] == nil {
			packageTypes[context.packageName] = make(map[string]typeDeclaration)
		}
		for _, declaration := range source.file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				declarationContext := contextForTypeSpec(context, typeSpec)
				packageTypes[context.packageName][typeSpec.Name.Name] = typeDeclaration{expression: typeSpec.Type, context: declarationContext}
			}
		}
	}

	for _, source := range sources {
		path, file := source.path, source.file
		context, err := contextForFile(file)
		if err != nil {
			return fmt.Errorf("arch: reading types in %s: %w", path, err)
		}
		clockAliases := map[string]bool{}
		formatAliases := map[string]bool{}
		sortAliases := map[string]bool{}
		dotClockImport := false
		dotFormatImport := false
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("arch: reading an import path in %s: %w", path, err)
			}
			if forbiddenRuntimeImport(imported) {
				violations = append(violations, fmt.Sprintf("%s imports %q", path, imported))
			}
			if !strings.HasSuffix(path, "_test.go") && !allowedCoreImports[file.Name.Name][imported] {
				violations = append(violations, fmt.Sprintf("%s imports unreviewed Core dependency %q", path, imported))
			}
			if imported == "time" {
				if spec.Name == nil {
					clockAliases["time"] = true
				} else if spec.Name.Name == "." {
					dotClockImport = true
				} else if spec.Name.Name != "_" {
					clockAliases[spec.Name.Name] = true
				}
			}
			if imported == "fmt" {
				if spec.Name == nil {
					formatAliases["fmt"] = true
				} else if spec.Name.Name == "." {
					dotFormatImport = true
				} else if spec.Name.Name != "_" {
					formatAliases[spec.Name.Name] = true
				}
			}
			if imported == "sort" && spec.Name != nil && spec.Name.Name != "." && spec.Name.Name != "_" {
				sortAliases[spec.Name.Name] = true
			} else if imported == "sort" && spec.Name == nil {
				sortAliases["sort"] = true
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if function, ok := node.(*ast.FuncDecl); ok && function.Recv != nil && !strings.HasSuffix(path, "_test.go") {
				receiverName, receiverType := receiverIdentity(function.Recv.List[0])
				if function.Body != nil && receiverName != "" && receiverType != "" {
					ast.Inspect(function.Body, func(inner ast.Node) bool {
						var targets []ast.Expr
						switch statement := inner.(type) {
						case *ast.AssignStmt:
							targets = statement.Lhs
						case *ast.IncDecStmt:
							targets = []ast.Expr{statement.X}
						}
						for _, target := range targets {
							field, nested := receiverFieldPath(target, receiverName)
							key := file.Name.Name + "." + receiverType + "." + field
							if nested && (allowedCoreValueReferenceFields[key] != "" || receiverPathContainsMutableReference(target, receiverName, file.Name.Name, receiverType, packageTypes)) {
								violations = append(violations, fmt.Sprintf("%s mutates receiver reference field %q", path, key))
							}
						}
						if statement, ok := inner.(*ast.AssignStmt); ok {
							for index, right := range statement.Rhs {
								if index < len(statement.Lhs) {
									if local, ok := statement.Lhs[index].(*ast.Ident); ok && local.Name != receiverName {
										if containsReceiverReference(right, receiverName, file.Name.Name, receiverType, context, packageTypes) {
											violations = append(violations, fmt.Sprintf("%s aliases receiver reference field into local %q", path, local.Name))
										} else if containsReceiverAggregateMapReference(right, receiverName, file.Name.Name, receiverType, packageTypes) || receiverValueCopyContainsMutableReference(right, receiverName, file.Name.Name, receiverType, packageTypes) {
											violations = append(violations, fmt.Sprintf("%s aliases receiver value containing mutable references into local %q", path, local.Name))
										}
									}
								}
							}
						}
						if values, ok := inner.(*ast.ValueSpec); ok {
							for index, right := range values.Values {
								if index < len(values.Names) {
									if containsReceiverReference(right, receiverName, file.Name.Name, receiverType, context, packageTypes) {
										violations = append(violations, fmt.Sprintf("%s aliases receiver reference field into local %q", path, values.Names[index].Name))
									} else if containsReceiverAggregateMapReference(right, receiverName, file.Name.Name, receiverType, packageTypes) || receiverValueCopyContainsMutableReference(right, receiverName, file.Name.Name, receiverType, packageTypes) {
										violations = append(violations, fmt.Sprintf("%s aliases receiver value containing mutable references into local %q", path, values.Names[index].Name))
									}
								}
							}
						}
						if call, ok := inner.(*ast.CallExpr); ok {
							if mutatesReceiverReference(call, receiverName, file.Name.Name, receiverType, sortAliases, context, packageTypes) {
								violations = append(violations, fmt.Sprintf("%s passes receiver reference field to mutating operation %q", path, expressionString(call.Fun)))
							} else if passesReceiverReferenceToUnreviewedCall(call, receiverName, file.Name.Name, receiverType, formatAliases, context, packageTypes) {
								violations = append(violations, fmt.Sprintf("%s passes receiver reference field to unreviewed operation %q", path, expressionString(call.Fun)))
							}
						}
						if statement, ok := inner.(*ast.ReturnStmt); ok {
							for _, value := range statement.Results {
								if containsReceiverReference(value, receiverName, file.Name.Name, receiverType, context, packageTypes) {
									violations = append(violations, fmt.Sprintf("%s returns receiver reference field directly", path))
								}
							}
						}
						return true
					})
				}
				for _, receiver := range function.Recv.List {
					if _, ok := receiver.Type.(*ast.StarExpr); ok {
						violations = append(violations, fmt.Sprintf("%s declares pointer receiver method %q", path, function.Name.Name))
					}
				}
			}
			selector, ok := node.(*ast.SelectorExpr)
			if ok {
				packageName, isIdent := selector.X.(*ast.Ident)
				if isIdent && clockAliases[packageName.Name] && forbiddenClockCalls[selector.Sel.Name] {
					violations = append(violations, fmt.Sprintf("%s accesses time.%s", path, selector.Sel.Name))
				}
				if isIdent && formatAliases[packageName.Name] && forbiddenFormattedIOCalls[selector.Sel.Name] {
					violations = append(violations, fmt.Sprintf("%s accesses fmt.%s", path, selector.Sel.Name))
				}
			}
			identifier, isIdent := node.(*ast.Ident)
			if dotClockImport && isIdent && forbiddenClockCalls[identifier.Name] {
				violations = append(violations, fmt.Sprintf("%s accesses dot-imported time.%s", path, identifier.Name))
			}
			if dotFormatImport && isIdent && forbiddenFormattedIOCalls[identifier.Name] {
				violations = append(violations, fmt.Sprintf("%s accesses dot-imported fmt.%s", path, identifier.Name))
			}
			if call, ok := node.(*ast.CallExpr); ok {
				if builtin, ok := call.Fun.(*ast.Ident); ok && (builtin.Name == "print" || builtin.Name == "println") {
					violations = append(violations, fmt.Sprintf("%s calls builtin %s", path, builtin.Name))
				}
			}
			spec, ok := node.(*ast.TypeSpec)
			if !ok {
				return true
			}
			if !strings.HasSuffix(path, "_test.go") {
				typeDeclarations++
			}
			name := spec.Name.Name
			declarationContext := contextForTypeSpec(context, spec)
			if forbiddenRuntimeType(name) {
				violations = append(violations, fmt.Sprintf("%s declares type %q", path, name))
			}
			if _, ok := spec.Type.(*ast.InterfaceType); ok && !strings.HasSuffix(path, "_test.go") && !allowedCoreInterfaceTypes[file.Name.Name+"."+name] {
				violations = append(violations, fmt.Sprintf("%s declares unreviewed Core interface %q", path, name))
			}
			typeKey := file.Name.Name + "." + name
			if isMutableDeclaredType(spec.Type, declarationContext, packageTypes) && !allowedCoreReferenceTypes[typeKey] && !strings.HasSuffix(path, "_test.go") {
				violations = append(violations, fmt.Sprintf("%s declares mutable reference type %q", path, name))
			}
			if structure, ok := spec.Type.(*ast.StructType); ok && !strings.HasSuffix(path, "_test.go") {
				for _, field := range structure.Fields.List {
					if !isMutableReferenceType(field.Type, declarationContext, packageTypes, nil) {
						continue
					}
					fieldNames := field.Names
					if len(fieldNames) == 0 {
						violations = append(violations, fmt.Sprintf("%s has anonymous mutable reference field in %q", path, name))
						continue
					}
					for _, fieldName := range fieldNames {
						key := file.Name.Name + "." + name + "." + fieldName.Name
						expectedType, allowed := allowedCoreValueReferenceFields[key]
						if actualType := expressionString(field.Type); !allowed || actualType != expectedType {
							violations = append(violations, fmt.Sprintf("%s has unreviewed mutable reference field %q with type %q", path, key, actualType))
						}
					}
				}
			}
			return true
		})
		if !strings.HasSuffix(path, "_test.go") {
			for _, declaration := range file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.VAR {
					continue
				}
				for _, spec := range general.Specs {
					valueSpec := spec.(*ast.ValueSpec)
					if !isErrorSentinel(valueSpec) {
						for _, name := range valueSpec.Names {
							violations = append(violations, fmt.Sprintf("%s declares package variable %q outside the error-sentinel exception", path, name.Name))
						}
					}
				}
			}
		}
	}
	if examined == 0 {
		return fmt.Errorf("%w: %s", ErrNoSourcesExamined, dir)
	}
	if typeDeclarations == 0 {
		return fmt.Errorf("%w: %s", ErrNoTypeDeclarations, dir)
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("%w: %s", ErrForbiddenRuntimeOwnership, strings.Join(violations, "; "))
	}
	return nil
}

func corePackageTypeIndex(dir string) (packageTypeIndex, error) {
	index := make(packageTypeIndex)
	moduleRoot, found, err := goModuleRoot(dir)
	if err != nil || !found {
		return index, err
	}

	for _, packageDirName := range []string{"audience", "experimentation", "campaign", "scenarios"} {
		packageDir := filepath.Join(moduleRoot, packageDirName)
		if _, err := os.Stat(packageDir); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("arch: inspecting Core package %s: %w", packageDir, err)
		}
		walkErr := filepath.WalkDir(packageDir, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), sourcePath, nil, parser.AllErrors)
			if err != nil {
				return fmt.Errorf("arch: parsing Core type source %s: %w", sourcePath, err)
			}
			context, err := contextForFile(file)
			if err != nil {
				return fmt.Errorf("arch: reading types in %s: %w", sourcePath, err)
			}
			if index[context.packageName] == nil {
				index[context.packageName] = make(map[string]typeDeclaration)
			}
			for _, declaration := range file.Decls {
				general, ok := declaration.(*ast.GenDecl)
				if !ok || general.Tok != token.TYPE {
					continue
				}
				for _, spec := range general.Specs {
					typeSpec := spec.(*ast.TypeSpec)
					declarationContext := contextForTypeSpec(context, typeSpec)
					index[context.packageName][typeSpec.Name.Name] = typeDeclaration{expression: typeSpec.Type, context: declarationContext}
				}
			}
			return nil
		})
		if walkErr != nil {
			return nil, walkErr
		}
	}
	return index, nil
}

func goModuleRoot(dir string) (string, bool, error) {
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", false, err
	}
	for {
		candidate := filepath.Join(current, "go.mod")
		if _, err := os.Stat(candidate); err == nil {
			return current, true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", false, fmt.Errorf("arch: inspecting module file %s: %w", candidate, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false, nil
		}
		current = parent
	}
}

func contextForFile(file *ast.File) (typeContext, error) {
	context := typeContext{packageName: file.Name.Name, imports: make(map[string]string), typeParams: make(map[string]bool)}
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return typeContext{}, err
		}
		alias := path.Base(imported)
		if spec.Name != nil {
			alias = spec.Name.Name
		}
		if alias == "." {
			context.dotImports = append(context.dotImports, path.Base(imported))
			continue
		}
		if alias == "_" {
			continue
		}
		context.imports[alias] = path.Base(imported)
	}
	return context, nil
}

func contextForTypeSpec(context typeContext, spec *ast.TypeSpec) typeContext {
	declarationContext := context
	declarationContext.typeParams = make(map[string]bool, len(context.typeParams))
	for name, isTypeParam := range context.typeParams {
		declarationContext.typeParams[name] = isTypeParam
	}
	if spec.TypeParams != nil {
		for _, field := range spec.TypeParams.List {
			for _, name := range field.Names {
				declarationContext.typeParams[name.Name] = true
			}
		}
	}
	return declarationContext
}

func isMutableReferenceType(expression ast.Expr, context typeContext, packageTypes packageTypeIndex, visiting map[string]bool) bool {
	if expression == nil {
		return false
	}
	switch typed := expression.(type) {
	case *ast.MapType, *ast.StarExpr, *ast.ChanType, *ast.FuncType, *ast.InterfaceType:
		return true
	case *ast.ArrayType:
		return typed.Len == nil || isMutableReferenceType(typed.Elt, context, packageTypes, visiting)
	case *ast.StructType:
		for _, field := range typed.Fields.List {
			if isMutableReferenceType(field.Type, context, packageTypes, visiting) {
				return true
			}
		}
		return false
	case *ast.Ident:
		if context.typeParams[typed.Name] {
			// A type parameter may be instantiated with a reference-bearing value.
			// Keep generic fields out of Core unless a future proof can narrow them.
			return true
		}
		if typed.Name == "any" || typed.Name == "error" {
			return true
		}
		if isBuiltinValueType(typed.Name) {
			return false
		}
		if declaration, ok := packageTypes[context.packageName][typed.Name]; ok {
			return isMutableTypeDeclaration(context.packageName+"."+typed.Name, declaration, packageTypes, visiting)
		}
		for _, importedPackage := range context.dotImports {
			if importedPackage == "time" && typed.Name == "Duration" {
				return false
			}
			if declaration, ok := packageTypes[importedPackage][typed.Name]; ok {
				return isMutableTypeDeclaration(importedPackage+"."+typed.Name, declaration, packageTypes, visiting)
			}
			// A dot import can introduce named reference types without a selector.
			// If the name is not indexed, fail closed rather than treating it as a
			// scalar or value struct.
			return true
		}
		return false
	case *ast.SelectorExpr:
		packageAlias, ok := typed.X.(*ast.Ident)
		if !ok {
			return true
		}
		importedPackage, ok := context.imports[packageAlias.Name]
		if !ok {
			return true
		}
		if importedPackage == "time" && typed.Sel.Name == "Duration" {
			return false
		}
		declaration, ok := packageTypes[importedPackage][typed.Sel.Name]
		if !ok {
			// Unknown qualified types fail closed. Core dependencies are indexed
			// from source; an unrecognized selector needs explicit investigation.
			return true
		}
		return isMutableTypeDeclaration(importedPackage+"."+typed.Sel.Name, declaration, packageTypes, visiting)
	case *ast.IndexExpr, *ast.IndexListExpr:
		// Generic instantiations can hold reference values through type
		// parameters, so using one as a field requires an explicit allowlist.
		return true
	case *ast.ParenExpr:
		return isMutableReferenceType(typed.X, context, packageTypes, visiting)
	default:
		return false
	}
}

func isBuiltinValueType(name string) bool {
	switch name {
	case "bool", "byte", "complex64", "complex128", "float32", "float64", "int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr":
		return true
	default:
		return false
	}
}

func isMutableTypeDeclaration(name string, declaration typeDeclaration, packageTypes packageTypeIndex, visiting map[string]bool) bool {
	// A named struct is a value model. Its direct reference fields are checked at
	// their declarations; nested mutation is checked against each method receiver.
	if _, ok := declaration.expression.(*ast.StructType); ok {
		return false
	}
	if visiting == nil {
		visiting = make(map[string]bool)
	}
	if visiting[name] {
		return false
	}
	visiting[name] = true
	mutable := isMutableReferenceType(declaration.expression, declaration.context, packageTypes, visiting)
	delete(visiting, name)
	return mutable
}

func isMutableDeclaredType(expression ast.Expr, context typeContext, packageTypes packageTypeIndex) bool {
	switch expression.(type) {
	case *ast.InterfaceType, *ast.StructType:
		return false
	default:
		return isMutableReferenceType(expression, context, packageTypes, nil)
	}
}

func receiverIdentity(receiver *ast.Field) (name, typeName string) {
	if receiver == nil || len(receiver.Names) != 1 {
		return "", ""
	}
	name = receiver.Names[0].Name
	switch typed := receiver.Type.(type) {
	case *ast.Ident:
		return name, typed.Name
	case *ast.StarExpr:
		if base, ok := typed.X.(*ast.Ident); ok {
			return name, base.Name
		}
	}
	return "", ""
}

func receiverFieldPath(expression ast.Expr, receiver string) (field string, nested bool) {
	switch typed := expression.(type) {
	case *ast.SelectorExpr:
		if base, ok := typed.X.(*ast.Ident); ok && base.Name == receiver {
			return typed.Sel.Name, false
		}
		field, _ = receiverFieldPath(typed.X, receiver)
		return field, field != ""
	case *ast.IndexExpr:
		field, _ = receiverFieldPath(typed.X, receiver)
		return field, field != ""
	case *ast.IndexListExpr:
		field, _ = receiverFieldPath(typed.X, receiver)
		return field, field != ""
	case *ast.SliceExpr:
		field, _ = receiverFieldPath(typed.X, receiver)
		return field, field != ""
	case *ast.StarExpr:
		field, _ = receiverFieldPath(typed.X, receiver)
		return field, field != ""
	case *ast.ParenExpr:
		return receiverFieldPath(typed.X, receiver)
	default:
		return "", false
	}
}

func receiverPathContainsMutableReference(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) bool {
	fields, ok := receiverSelectorPath(expression, receiver)
	if !ok || len(fields) == 0 {
		return false
	}
	typeExpression, typeContext, ok := declaredFieldType(packageName, receiverType, fields[0], packageTypes, make(map[string]bool))
	if !ok {
		return false
	}
	if isMutableReferenceType(typeExpression, typeContext, packageTypes, nil) {
		return true
	}
	for _, fieldName := range fields[1:] {
		typeExpression, typeContext, ok = nestedFieldType(typeExpression, typeContext, fieldName, packageTypes, make(map[string]bool))
		if !ok {
			return false
		}
		if isMutableReferenceType(typeExpression, typeContext, packageTypes, nil) {
			return true
		}
	}
	return false
}

func receiverAggregateReference(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) bool {
	typeExpression, context, ok := receiverPathType(expression, receiver, packageName, receiverType, packageTypes)
	return ok && typeContainsMutableReference(typeExpression, context, packageTypes, make(map[string]bool))
}

func receiverAggregateMapReference(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) bool {
	typeExpression, context, ok := receiverPathType(expression, receiver, packageName, receiverType, packageTypes)
	return ok && typeContainsMapOrPointer(typeExpression, context, packageTypes, make(map[string]bool))
}

func containsReceiverAggregateMapReference(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) bool {
	if receiverAggregateMapReference(expression, receiver, packageName, receiverType, packageTypes) {
		return true
	}
	switch typed := expression.(type) {
	case *ast.CompositeLit:
		for _, element := range typed.Elts {
			if containsReceiverAggregateMapReference(element, receiver, packageName, receiverType, packageTypes) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return containsReceiverAggregateMapReference(typed.Value, receiver, packageName, receiverType, packageTypes)
	case *ast.ParenExpr:
		return containsReceiverAggregateMapReference(typed.X, receiver, packageName, receiverType, packageTypes)
	}
	return false
}

func containsReceiverAggregateReference(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) bool {
	if receiverAggregateReference(expression, receiver, packageName, receiverType, packageTypes) {
		return true
	}
	switch typed := expression.(type) {
	case *ast.CompositeLit:
		for _, element := range typed.Elts {
			if containsReceiverAggregateReference(element, receiver, packageName, receiverType, packageTypes) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return containsReceiverAggregateReference(typed.Value, receiver, packageName, receiverType, packageTypes)
	case *ast.ParenExpr:
		return containsReceiverAggregateReference(typed.X, receiver, packageName, receiverType, packageTypes)
	}
	return false
}

func receiverPathType(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) (ast.Expr, typeContext, bool) {
	switch typed := expression.(type) {
	case *ast.Ident:
		if typed.Name != receiver {
			return nil, typeContext{}, false
		}
		declaration, ok := packageTypes[packageName][receiverType]
		if !ok {
			return nil, typeContext{}, false
		}
		return declaration.expression, declaration.context, true
	case *ast.SelectorExpr:
		base, context, ok := receiverPathType(typed.X, receiver, packageName, receiverType, packageTypes)
		if !ok {
			return nil, typeContext{}, false
		}
		return nestedFieldType(base, context, typed.Sel.Name, packageTypes, make(map[string]bool))
	case *ast.IndexExpr:
		base, context, ok := receiverPathType(typed.X, receiver, packageName, receiverType, packageTypes)
		if !ok {
			return nil, typeContext{}, false
		}
		return indexedElementType(base, context, packageTypes, make(map[string]bool))
	case *ast.SliceExpr:
		return receiverPathType(typed.X, receiver, packageName, receiverType, packageTypes)
	case *ast.StarExpr:
		base, context, ok := receiverPathType(typed.X, receiver, packageName, receiverType, packageTypes)
		if !ok {
			return nil, typeContext{}, false
		}
		if pointer, ok := base.(*ast.StarExpr); ok {
			return pointer.X, context, true
		}
		return nil, typeContext{}, false
	case *ast.UnaryExpr:
		base, context, ok := receiverPathType(typed.X, receiver, packageName, receiverType, packageTypes)
		if !ok {
			return nil, typeContext{}, false
		}
		switch typed.Op {
		case token.AND:
			return &ast.StarExpr{X: base}, context, true
		case token.MUL:
			if pointer, ok := base.(*ast.StarExpr); ok {
				return pointer.X, context, true
			}
		}
	case *ast.ParenExpr:
		return receiverPathType(typed.X, receiver, packageName, receiverType, packageTypes)
	}
	return nil, typeContext{}, false
}

func indexedElementType(expression ast.Expr, context typeContext, packageTypes packageTypeIndex, visiting map[string]bool) (ast.Expr, typeContext, bool) {
	switch typed := expression.(type) {
	case *ast.MapType:
		return typed.Value, context, true
	case *ast.ArrayType:
		return typed.Elt, context, true
	case *ast.ParenExpr:
		return indexedElementType(typed.X, context, packageTypes, visiting)
	default:
		packageName, typeName, ok := resolveNamedType(expression, context, packageTypes)
		if !ok {
			return nil, typeContext{}, false
		}
		key := packageName + "." + typeName
		if visiting[key] {
			return nil, typeContext{}, false
		}
		declaration, ok := packageTypes[packageName][typeName]
		if !ok {
			return nil, typeContext{}, false
		}
		visiting[key] = true
		defer delete(visiting, key)
		return indexedElementType(declaration.expression, declaration.context, packageTypes, visiting)
	}
}

func typeContainsMapOrPointer(expression ast.Expr, context typeContext, packageTypes packageTypeIndex, visiting map[string]bool) bool {
	return typeContainsReference(expression, context, packageTypes, false, visiting)
}

func typeContainsMutableReference(expression ast.Expr, context typeContext, packageTypes packageTypeIndex, visiting map[string]bool) bool {
	return typeContainsReference(expression, context, packageTypes, true, visiting)
}

func typeContainsReference(expression ast.Expr, context typeContext, packageTypes packageTypeIndex, includeSlices bool, visiting map[string]bool) bool {
	if expression == nil {
		return false
	}
	switch typed := expression.(type) {
	case *ast.MapType, *ast.StarExpr:
		return true
	case *ast.ChanType, *ast.FuncType, *ast.InterfaceType:
		return false
	case *ast.ArrayType:
		return (includeSlices && typed.Len == nil) || typeContainsReference(typed.Elt, context, packageTypes, includeSlices, visiting)
	case *ast.StructType:
		for _, field := range typed.Fields.List {
			if typeContainsReference(field.Type, context, packageTypes, includeSlices, visiting) {
				return true
			}
		}
		return false
	case *ast.Ident:
		if context.typeParams[typed.Name] || typed.Name == "any" || typed.Name == "error" {
			return true
		}
		if isBuiltinValueType(typed.Name) {
			return false
		}
		if declaration, ok := packageTypes[context.packageName][typed.Name]; ok {
			return namedTypeContainsReference(context.packageName+"."+typed.Name, declaration, packageTypes, includeSlices, visiting)
		}
		for _, importedPackage := range context.dotImports {
			if importedPackage == "time" && typed.Name == "Duration" {
				return false
			}
			if declaration, ok := packageTypes[importedPackage][typed.Name]; ok {
				return namedTypeContainsReference(importedPackage+"."+typed.Name, declaration, packageTypes, includeSlices, visiting)
			}
			return true
		}
		return true
	case *ast.SelectorExpr:
		alias, ok := typed.X.(*ast.Ident)
		if !ok {
			return true
		}
		importedPackage, ok := context.imports[alias.Name]
		if !ok {
			return true
		}
		if importedPackage == "time" && typed.Sel.Name == "Duration" {
			return false
		}
		declaration, ok := packageTypes[importedPackage][typed.Sel.Name]
		if !ok {
			return true
		}
		return namedTypeContainsReference(importedPackage+"."+typed.Sel.Name, declaration, packageTypes, includeSlices, visiting)
	case *ast.IndexExpr, *ast.IndexListExpr:
		return true
	case *ast.ParenExpr:
		return typeContainsReference(typed.X, context, packageTypes, includeSlices, visiting)
	default:
		return false
	}
}

func namedTypeContainsReference(name string, declaration typeDeclaration, packageTypes packageTypeIndex, includeSlices bool, visiting map[string]bool) bool {
	if visiting[name] {
		return true
	}
	visiting[name] = true
	defer delete(visiting, name)
	return typeContainsReference(declaration.expression, declaration.context, packageTypes, includeSlices, visiting)
}

func receiverSelectorPath(expression ast.Expr, receiver string) (fields []string, ok bool) {
	switch typed := expression.(type) {
	case *ast.Ident:
		return nil, typed.Name == receiver
	case *ast.SelectorExpr:
		fields, ok = receiverSelectorPath(typed.X, receiver)
		if !ok {
			return nil, false
		}
		return append(fields, typed.Sel.Name), true
	case *ast.IndexExpr:
		return receiverSelectorPath(typed.X, receiver)
	case *ast.IndexListExpr:
		return receiverSelectorPath(typed.X, receiver)
	case *ast.SliceExpr:
		return receiverSelectorPath(typed.X, receiver)
	case *ast.StarExpr:
		return receiverSelectorPath(typed.X, receiver)
	case *ast.UnaryExpr:
		if typed.Op == token.AND || typed.Op == token.MUL {
			return receiverSelectorPath(typed.X, receiver)
		}
		return nil, false
	case *ast.ParenExpr:
		return receiverSelectorPath(typed.X, receiver)
	default:
		return nil, false
	}
}

func declaredFieldType(packageName, typeName, fieldName string, packageTypes packageTypeIndex, visiting map[string]bool) (ast.Expr, typeContext, bool) {
	key := packageName + "." + typeName
	if visiting[key] {
		return nil, typeContext{}, false
	}
	declaration, ok := packageTypes[packageName][typeName]
	if !ok {
		return nil, typeContext{}, false
	}
	visiting[key] = true
	defer delete(visiting, key)
	if structure, ok := declaration.expression.(*ast.StructType); ok {
		for _, field := range structure.Fields.List {
			if len(field.Names) == 0 {
				embeddedName := embeddedFieldName(field.Type)
				if embeddedName == fieldName {
					return field.Type, declaration.context, true
				}
				if embeddedPackage, embeddedType, ok := resolveNamedType(field.Type, declaration.context, packageTypes); ok {
					if embeddedField, embeddedContext, found := declaredFieldType(embeddedPackage, embeddedType, fieldName, packageTypes, visiting); found {
						return embeddedField, embeddedContext, true
					}
				}
				continue
			}
			for _, name := range field.Names {
				if name.Name == fieldName {
					return field.Type, declaration.context, true
				}
			}
		}
		return nil, typeContext{}, false
	}
	resolvedPackage, resolvedName, ok := resolveNamedType(declaration.expression, declaration.context, packageTypes)
	if !ok {
		return nil, typeContext{}, false
	}
	return declaredFieldType(resolvedPackage, resolvedName, fieldName, packageTypes, visiting)
}

func nestedFieldType(expression ast.Expr, context typeContext, fieldName string, packageTypes packageTypeIndex, visiting map[string]bool) (ast.Expr, typeContext, bool) {
	if structure, ok := expression.(*ast.StructType); ok {
		for _, field := range structure.Fields.List {
			if len(field.Names) == 0 {
				embeddedName := embeddedFieldName(field.Type)
				if embeddedName == fieldName {
					return field.Type, context, true
				}
				if embeddedPackage, embeddedType, ok := resolveNamedType(field.Type, context, packageTypes); ok {
					if embeddedField, embeddedContext, found := declaredFieldType(embeddedPackage, embeddedType, fieldName, packageTypes, visiting); found {
						return embeddedField, embeddedContext, true
					}
				}
				continue
			}
			for _, name := range field.Names {
				if name.Name == fieldName {
					return field.Type, context, true
				}
			}
		}
		return nil, typeContext{}, false
	}
	packageName, typeName, ok := resolveNamedType(expression, context, packageTypes)
	if !ok {
		return nil, typeContext{}, false
	}
	return declaredFieldType(packageName, typeName, fieldName, packageTypes, visiting)
}

func embeddedFieldName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		return typed.Sel.Name
	case *ast.StarExpr:
		return embeddedFieldName(typed.X)
	default:
		return ""
	}
}

func resolveNamedType(expression ast.Expr, context typeContext, packageTypes packageTypeIndex) (packageName, typeName string, ok bool) {
	switch typed := expression.(type) {
	case *ast.Ident:
		if isBuiltinValueType(typed.Name) || typed.Name == "any" || typed.Name == "error" || context.typeParams[typed.Name] {
			return "", "", false
		}
		if _, ok := packageTypes[context.packageName][typed.Name]; ok {
			return context.packageName, typed.Name, true
		}
		for _, importedPackage := range context.dotImports {
			if _, ok := packageTypes[importedPackage][typed.Name]; ok {
				return importedPackage, typed.Name, true
			}
		}
		return "", "", false
	case *ast.SelectorExpr:
		alias, ok := typed.X.(*ast.Ident)
		if !ok {
			return "", "", false
		}
		packageName, ok := context.imports[alias.Name]
		if !ok {
			return "", "", false
		}
		if _, ok := packageTypes[packageName][typed.Sel.Name]; !ok {
			return "", "", false
		}
		return packageName, typed.Sel.Name, true
	case *ast.ParenExpr:
		return resolveNamedType(typed.X, context, packageTypes)
	default:
		return "", "", false
	}
}

func receiverReferenceAlias(expression ast.Expr, receiver, packageName, receiverType string, context typeContext, packageTypes packageTypeIndex) bool {
	switch typed := expression.(type) {
	case *ast.SelectorExpr, *ast.SliceExpr, *ast.StarExpr:
		field, _ := receiverFieldPath(expression, receiver)
		key := packageName + "." + receiverType + "." + field
		return field != "" && (allowedCoreValueReferenceFields[key] != "" || receiverPathContainsMutableReference(expression, receiver, packageName, receiverType, packageTypes))
	case *ast.UnaryExpr:
		if typed.Op == token.AND || typed.Op == token.MUL {
			return receiverReferenceAlias(typed.X, receiver, packageName, receiverType, context, packageTypes)
		}
	case *ast.ParenExpr:
		return receiverReferenceAlias(typed.X, receiver, packageName, receiverType, context, packageTypes)
	}
	return false
}

func containsReceiverReference(expression ast.Expr, receiver, packageName, receiverType string, context typeContext, packageTypes packageTypeIndex) bool {
	if receiverReferenceAlias(expression, receiver, packageName, receiverType, context, packageTypes) {
		return true
	}
	switch typed := expression.(type) {
	case *ast.CompositeLit:
		for _, element := range typed.Elts {
			if containsReceiverReference(element, receiver, packageName, receiverType, context, packageTypes) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return containsReceiverReference(typed.Value, receiver, packageName, receiverType, context, packageTypes)
	case *ast.ParenExpr:
		return containsReceiverReference(typed.X, receiver, packageName, receiverType, context, packageTypes)
	}
	return false
}

func mutatesReceiverReference(call *ast.CallExpr, receiver, packageName, receiverType string, sortAliases map[string]bool, context typeContext, packageTypes packageTypeIndex) bool {
	if call == nil || len(call.Args) == 0 {
		return false
	}
	functionName, packageCall := "", false
	switch function := call.Fun.(type) {
	case *ast.Ident:
		functionName = function.Name
	case *ast.SelectorExpr:
		packageName, ok := function.X.(*ast.Ident)
		if !ok || !sortAliases[packageName.Name] {
			return false
		}
		functionName, packageCall = function.Sel.Name, true
	}
	mutatingBuiltin := map[string]bool{"append": true, "clear": true, "copy": true, "delete": true}
	mutatingSort := map[string]bool{"Float64s": true, "Ints": true, "Reverse": true, "Slice": true, "SliceIsSorted": false, "SliceStable": true, "Sort": true, "Stable": true, "Strings": true}
	if (!packageCall && !mutatingBuiltin[functionName]) || (packageCall && !mutatingSort[functionName]) {
		return false
	}
	return receiverReferenceAlias(call.Args[0], receiver, packageName, receiverType, context, packageTypes)
}

func passesReceiverReferenceToUnreviewedCall(call *ast.CallExpr, receiver, packageName, receiverType string, formatAliases map[string]bool, context typeContext, packageTypes packageTypeIndex) bool {
	for index, argument := range call.Args {
		hasReference := containsReceiverReference(argument, receiver, packageName, receiverType, context, packageTypes)
		if _, helperCall := call.Fun.(*ast.Ident); helperCall {
			hasReference = hasReference || containsReceiverAggregateReference(argument, receiver, packageName, receiverType, packageTypes) || receiverValueHasMutableReference(argument, receiver, packageName, receiverType, packageTypes)
		}
		if !hasReference {
			continue
		}
		if isReadOnlyReceiverReferenceUse(call, index, formatAliases) {
			continue
		}
		return true
	}
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok &&
		(receiverReferenceAlias(selector.X, receiver, packageName, receiverType, context, packageTypes) || receiverFieldAggregateReference(selector.X, receiver, packageName, receiverType, packageTypes)) {
		field, _ := receiverFieldPath(selector.X, receiver)
		key := packageName + "." + receiverType + "." + field
		if key == "audience.Selection.bundle" {
			return selector.Sel.Name != "Validate"
		}
		return true
	}
	return false
}

func receiverFieldAggregateReference(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) bool {
	fields, ok := receiverSelectorPath(expression, receiver)
	return ok && len(fields) > 0 && receiverAggregateMapReference(expression, receiver, packageName, receiverType, packageTypes)
}

func receiverValueHasMutableReference(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) bool {
	switch typed := expression.(type) {
	case *ast.Ident:
		if typed.Name != receiver {
			return false
		}
		declaration, ok := packageTypes[packageName][receiverType]
		return ok && typeContainsMutableReference(declaration.expression, declaration.context, packageTypes, make(map[string]bool))
	case *ast.UnaryExpr:
		if typed.Op == token.AND || typed.Op == token.MUL {
			return receiverValueHasMutableReference(typed.X, receiver, packageName, receiverType, packageTypes)
		}
	case *ast.ParenExpr:
		return receiverValueHasMutableReference(typed.X, receiver, packageName, receiverType, packageTypes)
	}
	return false
}

func receiverValueCopyContainsMutableReference(expression ast.Expr, receiver, packageName, receiverType string, packageTypes packageTypeIndex) bool {
	switch typed := expression.(type) {
	case *ast.SelectorExpr:
		return receiverAggregateReference(expression, receiver, packageName, receiverType, packageTypes)
	case *ast.Ident:
		if typed.Name != receiver {
			return false
		}
		declaration, ok := packageTypes[packageName][receiverType]
		return ok && typeContainsMutableReference(declaration.expression, declaration.context, packageTypes, make(map[string]bool))
	case *ast.UnaryExpr:
		if typed.Op == token.AND || typed.Op == token.MUL {
			return receiverValueCopyContainsMutableReference(typed.X, receiver, packageName, receiverType, packageTypes)
		}
	case *ast.ParenExpr:
		return receiverValueCopyContainsMutableReference(typed.X, receiver, packageName, receiverType, packageTypes)
	}
	return false
}

func isReadOnlyReceiverReferenceUse(call *ast.CallExpr, argument int, formatAliases map[string]bool) bool {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		switch function.Name {
		case "cap", "len":
			return argument == 0
		case "append", "copy":
			return argument > 0
		}
	case *ast.SelectorExpr:
		if imported, ok := function.X.(*ast.Ident); ok && formatAliases[imported.Name] {
			switch function.Sel.Name {
			case "Errorf", "Sprint", "Sprintf", "Sprintln":
				return true
			}
		}
	}
	return false
}

func expressionString(expression ast.Expr) string {
	var buffer bytes.Buffer
	if err := format.Node(&buffer, token.NewFileSet(), expression); err != nil {
		return "<unprintable>"
	}
	return buffer.String()
}

func isErrorSentinel(spec *ast.ValueSpec) bool {
	if len(spec.Names) != 1 || !strings.HasPrefix(spec.Names[0].Name, "Err") || len(spec.Values) != 1 {
		return false
	}
	call, ok := spec.Values[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "New" {
		return false
	}
	packageName, ok := selector.X.(*ast.Ident)
	return ok && packageName.Name == "errors"
}

func forbiddenRuntimeImport(imported string) bool {
	for _, prefix := range forbiddenRuntimeImports {
		if imported == prefix || strings.HasPrefix(imported, prefix+"/") {
			return true
		}
	}
	return false
}

func forbiddenRuntimeType(name string) bool {
	if strings.HasPrefix(name, "Memory") {
		return true
	}
	for _, suffix := range []string{"Service", "Runner", "Store", "Feed", "Log", "Deliverer", "EventSink", "EventPublisher", "ReceiptAcceptor"} {
		if name == suffix || strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return name == "AlgorithmRegistry"
}
