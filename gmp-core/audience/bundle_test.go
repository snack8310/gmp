package audience_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestBundleConstructionAndPublication(t *testing.T) {
	registry, err := experimentation.NewAlgorithmRegistry(experimentation.FNV1aV1{})
	if err != nil {
		t.Fatal(err)
	}
	experiments, err := experimentation.NewService(registry, experimentation.NewMemoryColoringStore())
	if err != nil {
		t.Fatal(err)
	}
	source, err := audience.RegisterSource(audience.SourceSpec{ID: "people", Supply: audience.SupplyPull, Category: audience.CategoryExternal})
	if err != nil {
		t.Fatal(err)
	}
	audiences := audience.NewService(experiments)
	if err := audiences.Register(source, audience.NewMemoryFeed()); err != nil {
		t.Fatal(err)
	}
	split, err := experimentation.EqualSplit(2)
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := experimentation.NewDefinition(experimentation.DefinitionSpec{ID: "split", Split: split, Salt: "salt", Algorithm: "fnv1a-v1", Colored: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := experiments.RegisterComponent(assignment); err != nil {
		t.Fatal(err)
	}
	arm := func(id audience.GroupID, assignment experimentation.Definition, share int, source audience.SourceID) audience.BundleArm {
		t.Helper()
		bucket, err := experimentation.NewBucketNumber(share)
		if err != nil {
			t.Fatal(err)
		}
		condition, err := audience.Share(assignment, bucket)
		if err != nil {
			t.Fatal(err)
		}
		definition, err := audience.NewDefinition(audience.DefinitionSpec{ID: audience.DefinitionID(id), Source: source, Conditions: []audience.Condition{condition}})
		if err != nil {
			t.Fatal(err)
		}
		return audience.BundleArm{ID: id, Share: bucket, Definition: definition}
	}
	first := arm("control", assignment, 1, source.ID())
	second := arm("active", assignment, 2, source.ID())
	valid := audience.BundleSpec{ID: "bundle", Source: source.ID(), Assignment: assignment, Arms: []audience.BundleArm{first, second}}
	bad := []struct {
		name   string
		change func(*audience.BundleSpec)
	}{
		{"missing identity", func(s *audience.BundleSpec) { s.ID = "" }},
		{"missing source", func(s *audience.BundleSpec) { s.Source = "" }},
		{"missing assignment", func(s *audience.BundleSpec) { s.Assignment = experimentation.Definition{} }},
		{"missing arm", func(s *audience.BundleSpec) { s.Arms = s.Arms[:1] }},
		{"duplicate share", func(s *audience.BundleSpec) { s.Arms[1] = arm("duplicate", assignment, 1, source.ID()) }},
		{"duplicate group", func(s *audience.BundleSpec) { s.Arms[1].ID = s.Arms[0].ID }},
		{"source mismatch", func(s *audience.BundleSpec) { s.Arms[1] = arm("other", assignment, 2, "other") }},
		{"wrong share condition", func(s *audience.BundleSpec) {
			s.Arms[1].Definition = arm("wrong", assignment, 1, source.ID()).Definition
		}},
		{"out of range share", func(s *audience.BundleSpec) { s.Arms[1].Share, _ = experimentation.NewBucketNumber(3) }},
		{"no share condition", func(s *audience.BundleSpec) {
			s.Arms[1].Definition, _ = audience.NewDefinition(audience.DefinitionSpec{ID: "no-share", Source: source.ID()})
		}},
	}
	other, err := experimentation.NewDefinition(experimentation.DefinitionSpec{ID: "other", Split: split, Salt: "other", Algorithm: "fnv1a-v1", Colored: true})
	if err != nil {
		t.Fatal(err)
	}
	bad = append(bad, struct {
		name   string
		change func(*audience.BundleSpec)
	}{"different assignment", func(s *audience.BundleSpec) { s.Arms[1] = arm("other", other, 2, source.ID()) }})
	revision, err := experimentation.NewDefinition(experimentation.DefinitionSpec{ID: assignment.ID(), Split: split, Salt: "new-salt", Algorithm: "fnv1a-v1", Colored: true})
	if err != nil {
		t.Fatal(err)
	}
	bad = append(bad, struct {
		name   string
		change func(*audience.BundleSpec)
	}{"different revision", func(s *audience.BundleSpec) { s.Arms[1] = arm("revision", revision, 2, source.ID()) }})
	for _, test := range bad {
		t.Run(test.name, func(t *testing.T) {
			spec := valid
			spec.Arms = append([]audience.BundleArm(nil), valid.Arms...)
			test.change(&spec)
			if _, err := audience.NewBundle(spec); !errors.Is(err, audience.ErrInvalidBundle) && !errors.Is(err, audience.ErrMissingBundleID) {
				t.Fatalf("accepted invalid bundle: %v", err)
			}
		})
	}
	if err := audiences.Publish(audience.Bundle{}); !errors.Is(err, audience.ErrInvalidBundle) {
		t.Fatalf("zero bundle: %v", err)
	}
	bundle, err := audience.NewBundle(valid)
	if err != nil {
		t.Fatal(err)
	}
	control, ok := bundle.Group(first.Share)
	if !ok {
		t.Fatal("control arm missing")
	}
	if err := control.Validate(); !errors.Is(err, audience.ErrUnpublishedGroup) {
		t.Fatalf("unpublished arm: %v", err)
	}
	if err := audiences.Publish(bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.Available() {
		t.Fatal("unstarted component made bundle available")
	}
	if err := experiments.StartComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if !bundle.Available() {
		t.Fatal("started bundle unavailable")
	}
	if got, ok := audiences.Bundle(bundle.ID()); !ok || got.ID() != bundle.ID() {
		t.Fatal("bundle lookup failed")
	}
	if got, ok := audiences.Group(control.ID()); !ok || got.ID() != control.ID() {
		t.Fatal("control group lookup failed")
	}
	if err := audience.NewService(experiments).ValidateGroup(control); !errors.Is(err, audience.ErrUnpublishedGroup) {
		t.Fatalf("foreign audience service accepted the arm: %v", err)
	}
	if err := audiences.Publish(bundle); !errors.Is(err, audience.ErrDuplicateBundle) {
		t.Fatalf("duplicate publish: %v", err)
	}
	if err := experiments.StopComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if err := control.Validate(); !errors.Is(err, audience.ErrGroupUnavailable) {
		t.Fatalf("retired control arm: %v", err)
	}
	if err := experiments.StartComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if bundle.Available() {
		t.Fatal("old bundle revived")
	}
	if err := experiments.StopComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	pendingSpec := valid
	pendingSpec.ID = "pending"
	pendingSpec.Arms = []audience.BundleArm{arm("pending-control", assignment, 1, source.ID()), arm("pending-active", assignment, 2, source.ID())}
	pending, err := audience.NewBundle(pendingSpec)
	if err != nil {
		t.Fatal(err)
	}
	if err := audiences.Publish(pending); err != nil {
		t.Fatal(err)
	}
	if err := experiments.ReviseComponent(revision); err != nil {
		t.Fatal(err)
	}
	if err := experiments.StartComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	if pending.Available() {
		t.Fatal("revised bundle remained available")
	}
}
