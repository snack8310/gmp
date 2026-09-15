package experimentation

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingAssignmentID is returned when a definition carries no identity.
	// Coloring records are filed under the identity, so an unnamed assignment
	// could not keep its coloring across a change of definition.
	ErrMissingAssignmentID = errors.New("experimentation: assignment id is empty")
	// ErrMissingSalt is returned when a definition carries no salt. There is no
	// default: see the Salt documentation.
	ErrMissingSalt = errors.New("experimentation: salt is empty")
	// ErrMissingAlgorithmVersion is returned when a definition does not say
	// which bucketing algorithm it used.
	//
	// Go would otherwise fill the field with its zero value and the assignment
	// would look complete while being unreplayable. The global constraints
	// forbid inventing a default here, so construction fails instead.
	ErrMissingAlgorithmVersion = errors.New("experimentation: algorithm version is empty")
	// ErrMissingSplit is returned when a definition does not say how it carves
	// the space.
	ErrMissingSplit = errors.New("experimentation: split is not set")
	// ErrAlignmentNotTransitive is returned when a definition tries to align
	// with one that is itself aligned.
	//
	// The spec says an operator picks an existing assignment and the platform
	// takes its salt and split; it never says what a chain of those means. A
	// chain would also admit a cycle. Refusing is the reading that cannot be
	// silently wrong, and it is recorded as a conservative choice rather than
	// as something the spec settled.
	ErrAlignmentNotTransitive = errors.New("experimentation: cannot align with an assignment that is itself aligned")
	// ErrIncompatibleSplit is returned when an aligned definition asks to carve
	// the space differently from the assignment it aligns with.
	//
	// Matching the salt but not the split does not put the same key in the same
	// share, and nothing about it fails on its own -- which is why the domain
	// documents ask for it to be refused on the spot.
	ErrIncompatibleSplit = errors.New("experimentation: aligned split does not match the target split")
)

// AssignmentID identifies one assignment across every revision of its
// definition.
//
// Changing a definition means building a new one under the same id, never
// editing an existing value. Coloring is filed under the id precisely so that
// "the already-coloured do not move when the definition changes" can hold.
type AssignmentID string

// Definition is one assignment: how the space is carved, what salt scatters it,
// which algorithm version did the work, and whether results are recorded.
//
// Values are immutable. Every field is set at construction and there is no way
// to mutate one afterwards, so a definition that has been used for assignment
// cannot change underneath the coloring records that refer to it.
type Definition struct {
	id        AssignmentID
	split     Split
	salt      Salt
	algorithm AlgorithmVersion
	colored   bool
	alignedTo AssignmentID
}

// DefinitionSpec is the input to building a Definition. Every field is stated
// by the caller; nothing is defaulted.
type DefinitionSpec struct {
	// ID identifies the assignment across revisions of its definition.
	ID AssignmentID
	// Split carves the space. Required for an unaligned definition; for an
	// aligned one it must either be left unset or match the target exactly.
	Split Split
	// Salt scatters the keys. Required for an unaligned definition; an aligned
	// one inherits the target's and must not state its own.
	Salt Salt
	// Algorithm names the bucketing algorithm version. Always required.
	Algorithm AlgorithmVersion
	// Colored says whether assignments under this definition are recorded so
	// they survive a later change of definition.
	Colored bool
}

// NewDefinition builds an assignment that scatters independently of every other
// assignment -- the default the domain documents call for, so that no one ends
// up in the control group of every experiment at once.
func NewDefinition(spec DefinitionSpec) (Definition, error) {
	if spec.ID == "" {
		return Definition{}, ErrMissingAssignmentID
	}
	if spec.Salt == "" {
		return Definition{}, ErrMissingSalt
	}
	if spec.Algorithm == "" {
		return Definition{}, fmt.Errorf("%w: assignment %q", ErrMissingAlgorithmVersion, spec.ID)
	}
	if spec.Split.isZero() {
		return Definition{}, fmt.Errorf("%w: assignment %q", ErrMissingSplit, spec.ID)
	}
	return Definition{
		id:        spec.ID,
		split:     spec.Split,
		salt:      spec.Salt,
		algorithm: spec.Algorithm,
		colored:   spec.Colored,
	}, nil
}

// NewAlignedDefinition builds an assignment that deliberately lands the same
// keys in the same shares as target, by taking target's salt and split.
//
// The caller states the alignment target, not the salt -- the salt is an
// implementation detail and "aligned with that one" is the business intent.
func NewAlignedDefinition(spec DefinitionSpec, target Definition) (Definition, error) {
	if spec.ID == "" {
		return Definition{}, ErrMissingAssignmentID
	}
	if target.id == "" {
		return Definition{}, fmt.Errorf("%w: alignment target of %q is not a built definition", ErrMissingAssignmentID, spec.ID)
	}
	if target.alignedTo != "" {
		return Definition{}, fmt.Errorf("%w: %q aligns with %q, which aligns with %q",
			ErrAlignmentNotTransitive, spec.ID, target.id, target.alignedTo)
	}
	if spec.Algorithm == "" {
		return Definition{}, fmt.Errorf("%w: assignment %q", ErrMissingAlgorithmVersion, spec.ID)
	}
	if spec.Salt != "" && spec.Salt != target.salt {
		return Definition{}, fmt.Errorf("experimentation: aligned assignment %q must not state its own salt", spec.ID)
	}
	if !spec.Split.isZero() && !spec.Split.equals(target.split) {
		return Definition{}, fmt.Errorf("%w: assignment %q aligns with %q", ErrIncompatibleSplit, spec.ID, target.id)
	}
	return Definition{
		id:        spec.ID,
		split:     target.split,
		salt:      target.salt,
		algorithm: spec.Algorithm,
		colored:   spec.Colored,
		alignedTo: target.id,
	}, nil
}

// ID reports the assignment this definition is a revision of.
func (d Definition) ID() AssignmentID { return d.id }

// Split reports how the space is carved.
func (d Definition) Split() Split { return d.split }

// Algorithm reports the bucketing algorithm version this definition uses.
func (d Definition) Algorithm() AlgorithmVersion { return d.algorithm }

// Colored reports whether assignments under this definition are recorded.
func (d Definition) Colored() bool { return d.colored }

// AlignedTo reports the assignment this one aligns with, or the empty id when
// it scatters independently.
func (d Definition) AlignedTo() AssignmentID { return d.alignedTo }

// isZero reports whether this is the zero value rather than a built definition.
func (d Definition) isZero() bool { return d.id == "" }

// SameRevisionAs reports whether two definitions are the same revision of the
// same assignment -- identical in every field that decides where a key lands.
//
// It exists because a caller above this layer may need to know that two
// conditions are talking about exactly the same carve-up, and cannot work that
// out for itself: the salt is deliberately not exposed. Answering here keeps
// it that way.
//
// Same assignment id is not enough on its own. Two revisions of one assignment
// may carve differently, and an uncoloured key moves between them.
func (d Definition) SameRevisionAs(other Definition) bool {
	if d.isZero() || other.isZero() {
		return false
	}
	return d.id == other.id &&
		d.salt == other.salt &&
		d.algorithm == other.algorithm &&
		d.colored == other.colored &&
		d.alignedTo == other.alignedTo &&
		d.split.equals(other.split)
}
