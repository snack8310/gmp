package audience

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

var (
	ErrMissingGroupID   = errors.New("audience: group id is empty")
	ErrUnbuiltGroup     = errors.New("audience: group was not built")
	ErrUnpublishedGroup = errors.New("audience: experimental group is not published")
	ErrMissingBundleID  = errors.New("audience: bundle id is empty")
	ErrInvalidBundle    = errors.New("audience: invalid experiment audience bundle")
	ErrDuplicateBundle  = errors.New("audience: bundle is already published")
)

type GroupID string
type BundleID string

// Group is the audience unit a campaign can reference.
type Group struct {
	id                 GroupID
	definition         Definition
	bundleID           BundleID
	bundleAssignment   experimentation.Definition
	bundleGeneration   uint64
	bundleWasPublished bool
}

// NewGroup presents a non-experimental configuration as one group.
func NewGroup(id GroupID, definition Definition) (Group, error) {
	if id == "" {
		return Group{}, ErrMissingGroupID
	}
	if definition.isZero() {
		return Group{}, ErrUnbuiltDefinition
	}
	if definition.bundleID != "" {
		return Group{}, ErrUnpublishedGroup
	}
	return Group{id: id, definition: definition}, nil
}

// Single presents one built configuration as a non-experimental group. A
// definition taken from a bundle remains marked and cannot bypass its bundle.
func Single(definition Definition) Group {
	return Group{id: GroupID(definition.ID()), definition: definition}
}

func (g Group) ID() GroupID            { return g.id }
func (g Group) Definition() Definition { return g.definition }

// Matches evaluates this group's definition using only the supplied record and
// assignment facts. Current component availability belongs to the application.
func (g Group) Matches(record Record, facts AssignmentFacts) (bool, error) {
	if err := g.Validate(); err != nil {
		return false, err
	}
	return g.definition.Matches(record, facts)
}

// Validate checks the value's construction and, for bundle arms, publication
// snapshot without consulting external or mutable state.
func (g Group) Validate() error {
	if g.id == "" || g.definition.isZero() {
		return ErrUnbuiltGroup
	}
	if g.definition.bundleID == "" {
		if g.bundleID != "" {
			return ErrInvalidBundle
		}
		return nil
	}
	if g.bundleID != g.definition.bundleID || !g.bundleWasPublished || g.bundleGeneration == 0 || g.bundleAssignment.ID() == "" {
		return ErrUnpublishedGroup
	}
	return nil
}

// BundleAssignment reports the component snapshot a caller must validate for
// this arm. Ordinary groups have no associated bundle component.
func (g Group) BundleAssignment() (experimentation.Definition, uint64, bool) {
	if g.bundleID == "" || g.bundleID != g.definition.bundleID {
		return experimentation.Definition{}, 0, false
	}
	return g.bundleAssignment, g.bundleGeneration, true
}

// BundleArm binds a named group to one exact share and its configuration.
type BundleArm struct {
	ID         GroupID
	Share      experimentation.BucketNumber
	Definition Definition
}

type BundleSpec struct {
	ID         BundleID
	Source     SourceID
	Assignment experimentation.Definition
	Arms       []BundleArm
}

// Bundle is an immutable assignment partition and its complete set of arms.
// Publishing returns a new value with an explicit component generation.
type Bundle struct {
	id         BundleID
	source     SourceID
	assignment experimentation.Definition
	arms       map[int]Group
	published  bool
	generation uint64
}

func NewBundle(spec BundleSpec) (Bundle, error) {
	if spec.ID == "" {
		return Bundle{}, ErrMissingBundleID
	}
	if spec.Source == "" || spec.Assignment.ID() == "" {
		return Bundle{}, ErrInvalidBundle
	}
	if len(spec.Arms) != spec.Assignment.Split().Shares() {
		return Bundle{}, fmt.Errorf("%w: expected %d arms, got %d", ErrInvalidBundle, spec.Assignment.Split().Shares(), len(spec.Arms))
	}
	arms := make(map[int]Group, len(spec.Arms))
	ids := make(map[GroupID]bool, len(spec.Arms))
	for _, arm := range spec.Arms {
		if arm.ID == "" || arm.Definition.isZero() || arm.Definition.Source() != spec.Source || arm.Definition.bundleID != "" ||
			arm.Share.IsZero() || arm.Share.Value() > len(spec.Arms) || ids[arm.ID] || arms[arm.Share.Value()].id != "" ||
			!armMatches(arm.Definition, spec.Assignment, arm.Share) {
			return Bundle{}, fmt.Errorf("%w: arm %q share %d", ErrInvalidBundle, arm.ID, arm.Share.Value())
		}
		ids[arm.ID] = true
		definition := arm.Definition
		definition.bundleID = spec.ID
		arms[arm.Share.Value()] = Group{
			id: arm.ID, definition: definition, bundleID: spec.ID,
			bundleAssignment: spec.Assignment,
		}
	}
	return Bundle{id: spec.ID, source: spec.Source, assignment: spec.Assignment, arms: arms}, nil
}

func (b Bundle) Publish(component experimentation.ComponentState) (Bundle, error) {
	if b.id == "" || b.assignment.ID() == "" || len(b.arms) != b.assignment.Split().Shares() {
		return Bundle{}, ErrInvalidBundle
	}
	if b.published {
		return Bundle{}, ErrDuplicateBundle
	}
	if !component.Definition().SameRevisionAs(b.assignment) {
		return Bundle{}, experimentation.ErrComponentRevision
	}
	if !component.Running() {
		return Bundle{}, experimentation.ErrComponentNotRunning
	}
	published := Bundle{
		id: b.id, source: b.source, assignment: b.assignment,
		arms: make(map[int]Group, len(b.arms)), published: true, generation: component.Generation(),
	}
	for share, group := range b.arms {
		group.bundleGeneration = component.Generation()
		group.bundleWasPublished = true
		published.arms[share] = group
	}
	return published, nil
}

func armMatches(definition Definition, assignment experimentation.Definition, share experimentation.BucketNumber) bool {
	found := false
	for _, condition := range definition.conditions {
		switch c := condition.(type) {
		case shareCondition:
			if found || c.share != share || !c.assignment.SameRevisionAs(assignment) {
				return false
			}
			found = true
		case shareAtMostCondition:
			return false
		}
	}
	return found
}

func (b Bundle) ID() BundleID                           { return b.id }
func (b Bundle) Source() SourceID                       { return b.source }
func (b Bundle) Assignment() experimentation.Definition { return b.assignment }
func (b Bundle) Published() bool                        { return b.published }
func (b Bundle) Generation() uint64                     { return b.generation }

func (b Bundle) Validate() error {
	if b.id == "" || b.source == "" || b.assignment.ID() == "" || len(b.arms) != b.assignment.Split().Shares() {
		return ErrInvalidBundle
	}
	if !b.published || b.generation == 0 {
		return ErrUnpublishedBundle
	}
	for share := 1; share <= len(b.arms); share++ {
		group, ok := b.arms[share]
		if !ok || group.bundleID != b.id || !group.bundleAssignment.SameRevisionAs(b.assignment) || group.bundleGeneration != b.generation {
			return ErrInvalidBundle
		}
		if err := group.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (b Bundle) Group(share experimentation.BucketNumber) (Group, bool) {
	g, ok := b.arms[share.Value()]
	return g, ok
}
