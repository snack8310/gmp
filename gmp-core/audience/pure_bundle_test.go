package audience_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestBundlePublicationIsAnExplicitValueTransition(t *testing.T) {
	source, err := audience.NewSource(audience.SourceSpec{ID: "bundle-value-source", Supply: audience.SupplyPull, Category: audience.CategoryExternal})
	if err != nil {
		t.Fatal(err)
	}
	assignment := pureAudienceAssignment(t, "bundle-value-assignment", 2)
	first, _ := experimentation.NewBucketNumber(1)
	second, _ := experimentation.NewBucketNumber(2)
	firstCondition, err := audience.Share(assignment, first)
	if err != nil {
		t.Fatal(err)
	}
	secondCondition, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	firstDefinition := pureAudienceDefinition(t, "bundle-value-control-definition", source.ID(), firstCondition)
	secondDefinition := pureAudienceDefinition(t, "bundle-value-treatment-definition", source.ID(), secondCondition)
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "bundle-value", Source: source.ID(), Assignment: assignment,
		Arms: []audience.BundleArm{
			{ID: "bundle-value-control", Share: first, Definition: firstDefinition},
			{ID: "bundle-value-treatment", Share: second, Definition: secondDefinition},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := audience.SelectBundle(bundle).Validate(); !errors.Is(err, audience.ErrUnpublishedBundle) {
		t.Fatalf("unpublished bundle validation = %v, want ErrUnpublishedBundle", err)
	}

	component, err := experimentation.NewComponentState(assignment)
	if err != nil {
		t.Fatal(err)
	}
	active, err := component.Start()
	if err != nil {
		t.Fatal(err)
	}
	published, err := bundle.Publish(active)
	if err != nil {
		t.Fatalf("publishing from active component value: %v", err)
	}
	if bundle.Published() || !published.Published() || bundle.Generation() != 0 || published.Generation() != active.Generation() {
		t.Fatalf("publication mutated the prior value or lost its generation: old=%d/%v new=%d/%v", bundle.Generation(), bundle.Published(), published.Generation(), published.Published())
	}
	if err := audience.SelectBundle(published).Validate(); err != nil {
		t.Fatalf("validating published bundle value: %v", err)
	}
	group, ok := published.Group(first)
	if !ok {
		t.Fatal("published control arm is missing")
	}
	definition, generation, bundled := group.BundleAssignment()
	if !bundled || !definition.SameRevisionAs(assignment) || generation != active.Generation() {
		t.Fatalf("group runtime snapshot = (%#v, %d, %v)", definition, generation, bundled)
	}
	if err := audience.SelectGroup(audience.Single(group.Definition())).Validate(); !errors.Is(err, audience.ErrUnpublishedGroup) {
		t.Fatalf("wrapping a bundle arm as a standalone group = %v, want ErrUnpublishedGroup", err)
	}

	stopped, err := active.Stop()
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := stopped.Start()
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Active(assignment, generation) {
		t.Fatal("restarting a component made the old published snapshot active again")
	}
}
