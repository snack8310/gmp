package audience_test

import (
	"fmt"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
)

// Scenario 2, the audience half: everyone who added to cart in the last thirty
// days is carved into four arms -- a control that does nothing, and three
// treatment combinations.
//
// The arms have to partition the base audience exactly. Nobody in two arms at
// once, and nobody who falls out of all of them: an overlap would put one
// person in two treatments, and a gap would leave people counted in the base
// but reachable by nothing, both without failing anywhere.
func TestScenarioTwoArmsPartitionTheAudience(t *testing.T) {
	experiments, _ := newExperiments(t)
	source := mustSource(t)
	population := sample(1000)
	service := audience.NewService(experiments)
	if err := service.Register(source, audience.NewMemoryFeed(population...)); err != nil {
		t.Fatalf("registering the feed: %v", err)
	}

	cond := conditions{t: t}
	inCart := func() audience.Condition { return cond.must(audience.Attribute("added-to-cart", "yes")) }

	base, err := audience.NewDefinition(audience.DefinitionSpec{
		ID:         "added-to-cart-30d",
		Source:     source.ID(),
		Conditions: []audience.Condition{inCart()},
	})
	if err != nil {
		t.Fatalf("building the base definition: %v", err)
	}

	assignment := mustAssignment(t, "double-eleven-warmup", 4, true)
	arms := map[string]audience.Definition{}
	for i, name := range []string{"A-control", "B-sms", "C-sms-and-coupon", "D-coupon"} {
		definition, err := audience.NewDefinition(audience.DefinitionSpec{
			ID:     audience.DefinitionID(name),
			Source: source.ID(),
			Conditions: []audience.Condition{
				inCart(),
				cond.must(audience.Share(assignment, mustShare(t, i+1))),
			},
		})
		if err != nil {
			t.Fatalf("building arm %q: %v", name, err)
		}
		arms[name] = definition
	}

	listed, err := service.Enumerate(base, audience.Live())
	if err != nil {
		t.Fatalf("enumerating the base audience: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("the base audience is empty, so the partition proves nothing")
	}
	inBase := make(map[audience.UID]bool, len(listed))
	for _, uid := range listed {
		inBase[uid] = true
	}

	// The control arm exists in its own right. It hangs no treatments, but
	// without it there is no telling its people apart afterwards from people
	// who were never in the audience at all.
	if _, present := arms["A-control"]; !present {
		t.Fatal("the control arm is not in the system")
	}

	placements := make(map[audience.UID][]string, len(listed))
	for name, definition := range arms {
		armMembers, err := service.Enumerate(definition, audience.Live())
		if err != nil {
			t.Fatalf("enumerating arm %q: %v", name, err)
		}
		if len(armMembers) == 0 {
			t.Fatalf("arm %q is empty, so the partition is degenerate", name)
		}
		for _, uid := range armMembers {
			if !inBase[uid] {
				t.Fatalf("arm %q holds %q, who is not in the base audience", name, uid)
			}
			placements[uid] = append(placements[uid], name)
		}
	}

	for uid := range inBase {
		switch got := placements[uid]; len(got) {
		case 1:
			// exactly where everyone should be
		case 0:
			t.Fatalf("%q is in the base audience but in no arm", uid)
		default:
			t.Fatalf("%q is in several arms at once: %v", uid, got)
		}
	}

	// Scaling out: whoever the base holds, the arms hold between them.
	if len(placements) != len(inBase) {
		t.Fatalf("the arms cover %d people, the base holds %d", len(placements), len(inBase))
	}

	// And the person-level direction agrees with the list-level one for the
	// arms too, which is what the slot-serving path would actually ask.
	for uid := range inBase {
		for name, definition := range arms {
			decided, err := service.Decide(definition, audience.Live(), uid)
			if err != nil {
				t.Fatalf("deciding %q under arm %q: %v", uid, name, err)
			}
			want := len(placements[uid]) == 1 && placements[uid][0] == name
			if decided != want {
				t.Fatalf("%q under arm %q: decided %v, listing says %v", uid, name, decided, want)
			}
		}
	}

	t.Logf("base %d, arms %v", len(inBase), func() []string {
		out := make([]string, 0, len(arms))
		for name := range arms {
			members, _ := service.Enumerate(arms[name], audience.Live())
			out = append(out, fmt.Sprintf("%s=%d", name, len(members)))
		}
		return out
	}())
}
