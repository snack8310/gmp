package audience

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

var (
	// ErrMissingAssignmentFact is returned when a share condition has no
	// explicit bucket for the assignment revision it names.
	ErrMissingAssignmentFact = errors.New("audience: assignment fact is missing")
	// ErrConflictingAssignmentFacts is returned when one assignment revision is
	// supplied with different buckets in the same evaluation.
	ErrConflictingAssignmentFacts = errors.New("audience: assignment facts conflict")
)

// AssignmentFact is the bucket a particular immutable assignment revision
// produced for the current UID.
type AssignmentFact struct {
	Definition experimentation.Definition
	Bucket     experimentation.BucketNumber
}

// AssignmentFacts are the explicit experiment results used in one evaluation.
// A slice preserves distinct revisions that happen to share one assignment ID.
type AssignmentFacts []AssignmentFact

func (facts AssignmentFacts) bucketFor(definition experimentation.Definition) (experimentation.BucketNumber, error) {
	if definition.ID() == "" {
		return experimentation.BucketNumber{}, ErrUnbuiltShareCondition
	}
	var (
		bucket experimentation.BucketNumber
		found  bool
	)
	for _, fact := range facts {
		if !fact.Definition.SameRevisionAs(definition) {
			continue
		}
		if !found {
			bucket, found = fact.Bucket, true
			continue
		}
		if bucket != fact.Bucket {
			return experimentation.BucketNumber{}, fmt.Errorf("%w: assignment %q", ErrConflictingAssignmentFacts, definition.ID())
		}
	}
	if !found {
		return experimentation.BucketNumber{}, fmt.Errorf("%w: assignment %q", ErrMissingAssignmentFact, definition.ID())
	}
	if bucket.IsZero() || bucket.Value() > definition.Split().Shares() {
		return experimentation.BucketNumber{}, fmt.Errorf("%w: %s, assignment %q carves fewer", experimentation.ErrBucketOutOfRange, bucket, definition.ID())
	}
	return bucket, nil
}

// Assignments returns every exact assignment revision referenced by the
// definition, in first-use order.
func (d Definition) Assignments() []experimentation.Definition {
	if d.isZero() {
		return nil
	}
	out := make([]experimentation.Definition, 0)
	for _, condition := range d.conditions {
		assignment, ok := conditionAssignment(condition)
		if !ok {
			continue
		}
		found := false
		for _, existing := range out {
			if existing.SameRevisionAs(assignment) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, assignment)
		}
	}
	return out
}

// Matches evaluates one record using only its explicit facts.
func (d Definition) Matches(record Record, facts AssignmentFacts) (bool, error) {
	if d.isZero() {
		return false, ErrUnbuiltDefinition
	}
	if err := record.UID.validate(); err != nil {
		return false, err
	}
	return d.holdsFor(evaluation{facts: facts}, record)
}

func (d Definition) holdsFor(eval evaluation, record Record) (bool, error) {
	for _, condition := range d.conditions {
		holds, err := condition.matches(eval, record)
		if err != nil {
			return false, err
		}
		if !holds {
			return false, nil
		}
	}
	return true, nil
}

// NextMissingAssignment reports the next assignment fact needed to continue
// evaluating this definition in condition order. If no fact is needed, matched
// says whether all conditions hold for the supplied record and facts.
func (d Definition) NextMissingAssignment(record Record, facts AssignmentFacts) (assignment experimentation.Definition, needed, matched bool, err error) {
	if d.isZero() {
		return experimentation.Definition{}, false, false, ErrUnbuiltDefinition
	}
	if err := record.UID.validate(); err != nil {
		return experimentation.Definition{}, false, false, err
	}
	eval := evaluation{facts: facts}
	for _, condition := range d.conditions {
		if assignment, usesAssignment := conditionAssignment(condition); usesAssignment {
			if _, err := facts.bucketFor(assignment); errors.Is(err, ErrMissingAssignmentFact) {
				return assignment, true, false, nil
			} else if err != nil {
				return experimentation.Definition{}, false, false, err
			}
		}
		holds, err := condition.matches(eval, record)
		if err != nil {
			return experimentation.Definition{}, false, false, err
		}
		if !holds {
			return experimentation.Definition{}, false, false, nil
		}
	}
	return experimentation.Definition{}, false, true, nil
}

// Placements reports the assignment shares recorded in the explicit facts.
func (d Definition) Placements(uid UID, facts AssignmentFacts) ([]Placement, error) {
	if d.isZero() {
		return nil, ErrUnbuiltDefinition
	}
	if err := uid.validate(); err != nil {
		return nil, err
	}
	return d.placementsFor(evaluation{facts: facts}, uid)
}

func conditionAssignment(condition Condition) (experimentation.Definition, bool) {
	switch share := condition.(type) {
	case shareCondition:
		return share.assignment, true
	case shareAtMostCondition:
		return share.assignment, true
	default:
		return experimentation.Definition{}, false
	}
}

func (d Definition) placementsFor(eval evaluation, _ UID) ([]Placement, error) {
	var out []Placement
	seen := make(map[experimentation.AssignmentID]bool)
	for _, condition := range d.conditions {
		assignment, ok := conditionAssignment(condition)
		if !ok || seen[assignment.ID()] {
			continue
		}
		bucket, err := eval.facts.bucketFor(assignment)
		if err != nil {
			return nil, err
		}
		seen[assignment.ID()] = true
		out = append(out, Placement{Assignment: string(assignment.ID()), Share: bucket.Value()})
	}
	return out, nil
}
