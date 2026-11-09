package audience_test

import (
	"errors"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/experimentation"
)

func TestDefinitionMatchesUsingExplicitAssignmentFacts(t *testing.T) {
	source := pureAudienceSource(t, "facts-source")
	assignment := pureAudienceAssignment(t, "facts-assignment", 2)
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	share, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	attribute, err := audience.Attribute("tier", "new")
	if err != nil {
		t.Fatal(err)
	}
	definition := pureAudienceDefinition(t, "facts-definition", source.ID(), share, attribute)
	facts := audience.AssignmentFacts{{Definition: assignment, Bucket: second}}

	got, err := definition.Matches(audience.Record{
		UID: "uid-1", Attributes: map[string]string{"tier": "new"},
	}, facts)
	if err != nil {
		t.Fatalf("matching with explicit facts: %v", err)
	}
	if !got {
		t.Fatal("record with the requested attribute and share did not match")
	}
}

func TestDefinitionReportsPlacementsFromFacts(t *testing.T) {
	source := pureAudienceSource(t, "placement-source")
	assignment := pureAudienceAssignment(t, "placement-assignment", 2)
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	share, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	upTo, err := audience.ShareAtMost(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	definition := pureAudienceDefinition(t, "placement-definition", source.ID(), share, upTo)
	facts := audience.AssignmentFacts{{Definition: assignment, Bucket: second}}

	got, err := definition.Placements("uid-2", facts)
	if err != nil {
		t.Fatalf("building placements: %v", err)
	}
	if len(got) != 1 || got[0].Assignment != string(assignment.ID()) || got[0].Share != 2 {
		t.Fatalf("placements = %#v, want one share-2 placement", got)
	}
	assignments := definition.Assignments()
	if len(assignments) != 1 || !assignments[0].SameRevisionAs(assignment) {
		t.Fatalf("assignments = %#v, want the referenced definition once", assignments)
	}
}

func TestSelectionMatchesExactlyOneBundleArm(t *testing.T) {
	source := pureAudienceSource(t, "bundle-source")
	assignment := pureAudienceAssignment(t, "bundle-assignment", 2)
	first, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	firstCondition, err := audience.Share(assignment, first)
	if err != nil {
		t.Fatal(err)
	}
	secondCondition, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	firstDefinition := pureAudienceDefinition(t, "bundle-control-definition", source.ID(), firstCondition)
	secondDefinition := pureAudienceDefinition(t, "bundle-treatment-definition", source.ID(), secondCondition)
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "bundle", Source: source.ID(), Assignment: assignment,
		Arms: []audience.BundleArm{
			{ID: "control", Share: first, Definition: firstDefinition},
			{ID: "treatment", Share: second, Definition: secondDefinition},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	component, err := experimentation.NewComponentState(assignment)
	if err != nil {
		t.Fatal(err)
	}
	active, err := component.Start()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err = bundle.Publish(active)
	if err != nil {
		t.Fatal(err)
	}

	group, matched, err := audience.SelectBundle(bundle).Match(
		audience.Record{UID: "uid-3"},
		audience.AssignmentFacts{{Definition: assignment, Bucket: second}},
	)
	if err != nil {
		t.Fatalf("matching bundle: %v", err)
	}
	if !matched || group.ID() != "treatment" {
		t.Fatalf("match = (%q, %v), want treatment arm", group.ID(), matched)
	}
}

func TestMissingAssignmentFactFailsClosed(t *testing.T) {
	source := pureAudienceSource(t, "missing-fact-source")
	assignment := pureAudienceAssignment(t, "missing-fact-assignment", 2)
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	share, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	definition := pureAudienceDefinition(t, "missing-fact-definition", source.ID(), share)

	_, err = definition.Matches(audience.Record{UID: "uid-4"}, nil)
	if !errors.Is(err, audience.ErrMissingAssignmentFact) {
		t.Fatalf("missing fact error = %v, want ErrMissingAssignmentFact", err)
	}
}

func TestNextMissingAssignmentRespectsConditionOrder(t *testing.T) {
	source := pureAudienceSource(t, "ordered-facts-source")
	assignment := pureAudienceAssignment(t, "ordered-facts-assignment", 2)
	first, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	region, err := audience.Attribute("region", "south")
	if err != nil {
		t.Fatal(err)
	}
	share, err := audience.Share(assignment, first)
	if err != nil {
		t.Fatal(err)
	}
	definition := pureAudienceDefinition(t, "ordered-facts-definition", source.ID(), region, share)

	next, needed, matched, err := definition.NextMissingAssignment(audience.Record{UID: "uid-north", Attributes: map[string]string{"region": "north"}}, nil)
	if err != nil || needed || matched {
		t.Fatalf("early rejected record requested an assignment: next=%#v needed=%v matched=%v err=%v", next, needed, matched, err)
	}

	record := audience.Record{UID: "uid-south", Attributes: map[string]string{"region": "south"}}
	next, needed, matched, err = definition.NextMissingAssignment(record, nil)
	if err != nil || !needed || matched || !next.SameRevisionAs(assignment) {
		t.Fatalf("matching prefix did not request its assignment: next=%#v needed=%v matched=%v err=%v", next, needed, matched, err)
	}
	next, needed, matched, err = definition.NextMissingAssignment(record, audience.AssignmentFacts{{Definition: assignment, Bucket: second}})
	if err != nil || needed || matched {
		t.Fatalf("failed share did not stop further evaluation: next=%#v needed=%v matched=%v err=%v", next, needed, matched, err)
	}
}

func TestUIDExposesItsOpaqueAssignmentKey(t *testing.T) {
	if got := audience.UID("uid-5").AssignmentKey(); got != experimentation.AssignmentKey("uid-5") {
		t.Fatalf("assignment key = %q, want the same UID bytes", got)
	}
}

func pureAudienceSource(t *testing.T, id string) audience.Source {
	t.Helper()
	source, err := audience.NewSource(audience.SourceSpec{
		ID: audience.SourceID(id), Supply: audience.SupplyPull, Category: audience.CategoryExternal,
	})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func pureAudienceAssignment(t *testing.T, id string, shares int) experimentation.Definition {
	t.Helper()
	split, err := experimentation.EqualSplit(shares)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: experimentation.AssignmentID(id), Split: split, Salt: experimentation.Salt(id),
		Algorithm: experimentation.FNV1aV1{}.Version(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func pureAudienceDefinition(t *testing.T, id string, source audience.SourceID, conditions ...audience.Condition) audience.Definition {
	t.Helper()
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: audience.DefinitionID(id), Source: source, Conditions: conditions,
	})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}
