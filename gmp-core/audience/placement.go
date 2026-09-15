package audience

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

// Placement records which share of which assignment a person landed in.
//
// It carries plain values rather than the experimentation layer's own types so
// that a layer above can record where someone was placed without reaching past
// this one. That matters because an execution result has to carry both the
// treatment and the share: with only the treatment there is no attributing a
// result to an arm, and with only the share there is no saying how a treatment
// did.
type Placement struct {
	// Assignment is the assignment this placement belongs to.
	Assignment string
	// Share is which share, counted from one as everywhere else.
	Share int
}

func (p Placement) String() string {
	return fmt.Sprintf("assignment %q share %d", p.Assignment, p.Share)
}

// Placements reports where this definition places the given person.
//
// It walks the conditions that refer to an assignment and asks the layer below
// where the key actually lands -- not whether the condition held. A person in
// share three fails a condition demanding share two, and that they are in share
// three is still the fact worth recording.
//
// The mode is honoured here as everywhere else: asking during a rehearsal must
// not colour anyone.
func (s *Service) Placements(definition Definition, mode Mode, uid UID) ([]Placement, error) {
	if definition.isZero() {
		return nil, ErrUnbuiltDefinition
	}
	if err := uid.validate(); err != nil {
		return nil, err
	}
	if err := mode.validate(); err != nil {
		return nil, fmt.Errorf("%w (definition %q)", err, definition.id)
	}
	eval := evaluation{experiments: s.experiments, mode: mode}
	record := Record{UID: uid}

	var out []Placement
	seen := map[string]bool{}
	for _, condition := range definition.conditions {
		placed, ok := condition.(placer)
		if !ok {
			continue
		}
		placement, err := placed.placementFor(eval, record)
		if err != nil {
			return nil, err
		}
		if seen[placement.Assignment] {
			continue
		}
		seen[placement.Assignment] = true
		out = append(out, placement)
	}
	return out, nil
}

// placer is a condition that can say where a person landed, not only whether
// the condition held.
type placer interface {
	placementFor(eval evaluation, record Record) (Placement, error)
}

func (c shareCondition) placementFor(eval evaluation, record Record) (Placement, error) {
	return placementOf(eval, record, c.assignment)
}

func (c shareAtMostCondition) placementFor(eval evaluation, record Record) (Placement, error) {
	return placementOf(eval, record, c.assignment)
}

func placementOf(eval evaluation, record Record, assignment experimentation.Definition) (Placement, error) {
	if eval.experiments == nil {
		return Placement{}, errors.New("audience: no experimentation service to ask about shares")
	}
	key := record.UID.assignmentKey()
	var (
		got experimentation.BucketNumber
		err error
	)
	if eval.mode.IsRehearsal() {
		got, err = eval.experiments.PeekAssign(assignment, key)
	} else {
		got, err = eval.experiments.Assign(assignment, key)
	}
	if err != nil {
		return Placement{}, fmt.Errorf("audience: asking where %q landed: %w", record.UID, err)
	}
	return Placement{Assignment: string(assignment.ID()), Share: got.Value()}, nil
}
