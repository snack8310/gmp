package audience

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingDefinitionID is returned when an audience definition carries no
	// identity.
	ErrMissingDefinitionID = errors.New("audience: definition id is empty")
	// ErrNilCondition is returned when a definition is handed a condition that
	// is not there.
	ErrNilCondition = errors.New("audience: definition was given a nil condition")
	// ErrUnbuiltDefinition is returned when a zero-value Definition is used
	// rather than one that went through a constructor and its checks.
	ErrUnbuiltDefinition = errors.New("audience: definition was not built through a constructor")
)

// DefinitionID identifies one audience definition.
type DefinitionID string

// Definition is a source plus a set of conditions, all of which must hold.
//
// It is the only thing this layer offers. Values are immutable: a definition
// may be referenced by several treatments at once, so editing one in place
// would change what those other callers are asking for without them knowing.
// Changing a definition means building another one.
type Definition struct {
	id         DefinitionID
	source     SourceID
	conditions []Condition
}

// DefinitionSpec is the input to building a Definition.
type DefinitionSpec struct {
	// ID identifies the definition.
	ID DefinitionID
	// Source is which registered source the people come from.
	Source SourceID
	// Conditions all have to hold. An empty set means everyone the source
	// yields, which is a legitimate definition.
	Conditions []Condition
}

// NewDefinition builds an audience definition.
func NewDefinition(spec DefinitionSpec) (Definition, error) {
	if spec.ID == "" {
		return Definition{}, ErrMissingDefinitionID
	}
	if spec.Source == "" {
		return Definition{}, fmt.Errorf("%w: definition %q", ErrMissingSourceID, spec.ID)
	}
	owned := make([]Condition, len(spec.Conditions))
	for i, condition := range spec.Conditions {
		if condition == nil {
			return Definition{}, fmt.Errorf("%w: definition %q, position %d", ErrNilCondition, spec.ID, i+1)
		}
		owned[i] = condition
	}
	return Definition{id: spec.ID, source: spec.Source, conditions: owned}, nil
}

// ID reports which definition this is.
func (d Definition) ID() DefinitionID { return d.id }

// Source reports which source the people come from.
func (d Definition) Source() SourceID { return d.source }

// Describe renders every condition, in order, for a human reading a
// configuration.
func (d Definition) Describe() []string {
	out := make([]string, len(d.conditions))
	for i, condition := range d.conditions {
		out[i] = condition.Describe()
	}
	return out
}

func (d Definition) isZero() bool { return d.id == "" }
