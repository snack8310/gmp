package audience

import (
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

var (
	ErrMissingGroupID   = errors.New("audience: group id is empty")
	ErrUnbuiltGroup     = errors.New("audience: group was not built")
	ErrUnpublishedGroup = errors.New("audience: experimental group is not published")
	ErrGroupUnavailable = errors.New("audience: experimental group is unavailable")
	ErrMissingBundleID  = errors.New("audience: bundle id is empty")
	ErrInvalidBundle    = errors.New("audience: invalid experiment audience bundle")
	ErrDuplicateBundle  = errors.New("audience: bundle already published")
	ErrDuplicateGroup   = errors.New("audience: group identity already published")
)

type GroupID string
type BundleID string

// Group is the audience unit a campaign can reference.
type Group struct {
	id         GroupID
	definition Definition
	bundle     *bundleState
}

type bundleState struct {
	published   atomic.Bool
	experiments *experimentation.Service
	assignment  experimentation.Definition
	generation  uint64
}

// NewGroup presents a non-experimental configuration as one group.
func NewGroup(id GroupID, definition Definition) (Group, error) {
	if id == "" {
		return Group{}, ErrMissingGroupID
	}
	if definition.isZero() {
		return Group{}, ErrUnbuiltDefinition
	}
	return Group{id: id, definition: definition}, nil
}

// Single presents one built configuration as a non-experimental group.
// Its identity is the configuration identity; an unbuilt configuration stays
// unbuilt and is rejected at the campaign boundary.
func Single(definition Definition) Group {
	return Group{id: GroupID(definition.ID()), definition: definition}
}

func (g Group) ID() GroupID            { return g.id }
func (g Group) Definition() Definition { return g.definition }

// Validate refuses unpublished and retired experimental arms at every use.
func (g Group) Validate() error {
	if g.id == "" || g.definition.isZero() {
		return ErrUnbuiltGroup
	}
	if g.bundle == nil {
		return nil
	}
	if !g.bundle.published.Load() {
		return ErrUnpublishedGroup
	}
	if !g.bundle.experiments.ComponentActive(g.bundle.assignment, g.bundle.generation) {
		return fmt.Errorf("%w: %q", ErrGroupUnavailable, g.id)
	}
	return nil
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

// Bundle is one publishable assignment and its complete set of arm groups.
type Bundle struct {
	id         BundleID
	source     SourceID
	assignment experimentation.Definition
	arms       map[int]Group
	state      *bundleState
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
	state := &bundleState{assignment: spec.Assignment}
	arms := make(map[int]Group, len(spec.Arms))
	ids := make(map[GroupID]bool, len(spec.Arms))
	for _, arm := range spec.Arms {
		if arm.ID == "" || arm.Definition.isZero() || arm.Definition.Source() != spec.Source ||
			arm.Share.IsZero() || arm.Share.Value() > len(spec.Arms) || ids[arm.ID] || arms[arm.Share.Value()].id != "" ||
			!armMatches(arm.Definition, spec.Assignment, arm.Share) {
			return Bundle{}, fmt.Errorf("%w: arm %q share %d", ErrInvalidBundle, arm.ID, arm.Share.Value())
		}
		ids[arm.ID] = true
		arms[arm.Share.Value()] = Group{id: arm.ID, definition: arm.Definition, bundle: state}
	}
	return Bundle{id: spec.ID, source: spec.Source, assignment: spec.Assignment, arms: arms, state: state}, nil
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

func (b Bundle) Group(share experimentation.BucketNumber) (Group, bool) {
	g, ok := b.arms[share.Value()]
	return g, ok
}

func (b Bundle) Available() bool {
	return b.state != nil && b.state.published.Load() && b.state.experiments.ComponentActive(b.assignment, b.state.generation)
}
