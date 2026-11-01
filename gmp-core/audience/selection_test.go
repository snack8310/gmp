package audience_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestSelectionRepresentsExactlyOneGroupOrOnePublishedBundle(t *testing.T) {
	experiments, _ := newExperiments(t)
	source := mustSource(t)
	audiences := audience.NewService(experiments)
	if err := audiences.Register(source, audience.NewMemoryFeed()); err != nil {
		t.Fatal(err)
	}

	definition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "everyone", Source: source.ID()})
	if err != nil {
		t.Fatal(err)
	}
	group := audience.Single(definition)
	selectedGroup := audience.SelectGroup(group)
	if err := selectedGroup.Validate(); err != nil {
		t.Fatalf("a built group selection was rejected: %v", err)
	}
	if got := selectedGroup.Groups(); len(got) != 1 || got[0].ID() != group.ID() {
		t.Fatalf("group selection returned %v, want only %q", groupIDs(got), group.ID())
	}
	if err := (audience.Selection{}).Validate(); !errors.Is(err, audience.ErrInvalidSelection) {
		t.Fatalf("zero selection should be rejected, got %v", err)
	}
	if err := audience.SelectGroup(audience.Group{}).Validate(); !errors.Is(err, audience.ErrInvalidSelection) {
		t.Fatalf("zero group selection should be rejected, got %v", err)
	}

	split, err := experimentation.EqualSplit(2)
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: "entry-test", Split: split, Salt: "entry-test-salt", Algorithm: "fnv1a-v1", Colored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := experiments.RegisterComponent(assignment); err != nil {
		t.Fatal(err)
	}
	if err := experiments.StartComponent(assignment.ID()); err != nil {
		t.Fatal(err)
	}
	arm := func(id string, number int) audience.BundleArm {
		t.Helper()
		share := mustShare(t, number)
		condition, err := audience.Share(assignment, share)
		if err != nil {
			t.Fatal(err)
		}
		definition, err := audience.NewDefinition(audience.DefinitionSpec{
			ID: audience.DefinitionID(id), Source: source.ID(), Conditions: []audience.Condition{condition},
		})
		if err != nil {
			t.Fatal(err)
		}
		return audience.BundleArm{ID: audience.GroupID(id), Share: share, Definition: definition}
	}
	control, active := arm("control", 1), arm("active", 2)
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "entry-bundle", Source: source.ID(), Assignment: assignment,
		Arms: []audience.BundleArm{active, control},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := audience.SelectBundle(bundle).Validate(); !errors.Is(err, audience.ErrUnpublishedBundle) {
		t.Fatalf("an unpublished bundle selection should be rejected, got %v", err)
	}
	if err := audiences.Publish(bundle); err != nil {
		t.Fatal(err)
	}
	selectedBundle := audience.SelectBundle(bundle)
	if err := selectedBundle.Validate(); err != nil {
		t.Fatalf("a published bundle selection was rejected: %v", err)
	}
	groups := selectedBundle.Groups()
	if got, want := groupIDs(groups), []audience.GroupID{"control", "active"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle groups = %v, want stable share order %v", got, want)
	}
}

func groupIDs(groups []audience.Group) []audience.GroupID {
	ids := make([]audience.GroupID, len(groups))
	for i, group := range groups {
		ids[i] = group.ID()
	}
	return ids
}
