package audience

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

var (
	// ErrUnknownSource is returned when a definition names a source nobody
	// registered.
	ErrUnknownSource = errors.New("audience: no such registered source")
	// ErrDuplicateSource is returned when a source id is registered twice.
	ErrDuplicateSource = errors.New("audience: source registered twice")
)

// Service answers the two questions this layer exists for.
type Service struct {
	experiments *experimentation.Service
	sources     map[SourceID]Source
	feeds       map[SourceID]Feed
	bundles     map[BundleID]Bundle
	groups      map[GroupID]Group
}

// NewService builds a service. The experimentation service may be nil only if
// no definition passed in ever references a share.
func NewService(experiments *experimentation.Service) *Service {
	return &Service{
		experiments: experiments,
		sources:     make(map[SourceID]Source),
		feeds:       make(map[SourceID]Feed),
		bundles:     make(map[BundleID]Bundle),
		groups:      make(map[GroupID]Group),
	}
}

// Publish makes a complete bundle and every arm available by stable identity.
func (s *Service) Publish(bundle Bundle) error {
	if bundle.state == nil || bundle.id == "" {
		return ErrInvalidBundle
	}
	if bundle.state.published.Load() || s.bundles[bundle.id].id != "" {
		return ErrDuplicateBundle
	}
	if _, ok := s.sources[bundle.source]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownSource, bundle.source)
	}
	if s.experiments == nil {
		return experimentation.ErrComponentUnknown
	}
	generation, err := s.experiments.ComponentGeneration(bundle.assignment)
	if err != nil {
		return err
	}
	for _, group := range bundle.arms {
		if _, exists := s.groups[group.id]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicateGroup, group.id)
		}
	}
	bundle.state.experiments = s.experiments
	bundle.state.generation = generation
	s.bundles[bundle.id] = bundle
	for _, group := range bundle.arms {
		s.groups[group.id] = group
	}
	bundle.state.published.Store(true)
	return nil
}

func (s *Service) Bundle(id BundleID) (Bundle, bool) {
	bundle, ok := s.bundles[id]
	return bundle, ok
}

func (s *Service) Group(id GroupID) (Group, bool) {
	group, ok := s.groups[id]
	return group, ok
}

// ValidateGroup checks the running component behind standalone share groups.
// A published bundle arm must retain its bundle lifecycle.
func (s *Service) ValidateGroup(group Group) error {
	if err := group.Validate(); err != nil {
		return err
	}
	if group.bundle != nil {
		published, ok := s.groups[group.id]
		if !ok || published.bundle != group.bundle {
			return ErrUnpublishedGroup
		}
		return nil
	}
	for _, condition := range group.definition.conditions {
		assignment, ok := conditionAssignment(condition)
		if !ok {
			continue
		}
		if s.experiments == nil {
			return experimentation.ErrComponentUnknown
		}
		generation, err := s.experiments.ComponentGeneration(assignment)
		if err != nil {
			return err
		}
		for _, bundle := range s.bundles {
			if bundle.assignment.SameRevisionAs(assignment) {
				return ErrUnpublishedGroup
			}
		}
		if !s.experiments.ComponentActive(assignment, generation) {
			return ErrGroupUnavailable
		}
	}
	return nil
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

func (s *Service) EnumerateGroup(group Group, mode Mode) ([]UID, error) {
	if err := s.ValidateGroup(group); err != nil {
		return nil, err
	}
	return s.Enumerate(group.Definition(), mode)
}

func (s *Service) DecideGroup(group Group, mode Mode, uid UID) (bool, error) {
	if err := s.ValidateGroup(group); err != nil {
		return false, err
	}
	return s.Decide(group.Definition(), mode, uid)
}

func (s *Service) GroupPlacements(group Group, mode Mode, uid UID) ([]Placement, error) {
	if err := s.ValidateGroup(group); err != nil {
		return nil, err
	}
	return s.Placements(group.Definition(), mode, uid)
}

// Register wires a registered source to what actually holds its data.
// Engineering does this; operations only picks from what is registered.
func (s *Service) Register(source Source, feed Feed) error {
	if source.isZero() {
		return ErrMissingSourceID
	}
	if feed == nil {
		return fmt.Errorf("audience: source %q has no feed", source.ID())
	}
	if _, taken := s.sources[source.ID()]; taken {
		return fmt.Errorf("%w: %q", ErrDuplicateSource, source.ID())
	}
	s.sources[source.ID()] = source
	s.feeds[source.ID()] = feed
	return nil
}

// Source returns a registered source descriptor.
func (s *Service) Source(id SourceID) (Source, bool) {
	source, ok := s.sources[id]
	return source, ok
}

// Enumerate lists everyone this definition covers.
//
// It may take its time: this is the offline direction, and nothing is waiting
// on it.
func (s *Service) Enumerate(definition Definition, mode Mode) ([]UID, error) {
	feed, eval, err := s.prepare(definition, mode)
	if err != nil {
		return nil, err
	}
	records, err := feed.All()
	if err != nil {
		return nil, fmt.Errorf("audience: reading source %q: %w", definition.source, err)
	}
	var out []UID
	for _, record := range records {
		if err := record.UID.validate(); err != nil {
			return nil, fmt.Errorf("audience: source %q yielded a record with no uid: %w", definition.source, err)
		}
		ok, err := definition.holdsFor(eval, record)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, record.UID)
		}
	}
	return out, nil
}

// Decide answers whether this one person counts.
//
// It runs on the request path with a caller waiting, so it looks one record up
// rather than walking the source. It must agree with Enumerate for every uid;
// nothing downstream would ever notice if it did not.
func (s *Service) Decide(definition Definition, mode Mode, uid UID) (bool, error) {
	if err := uid.validate(); err != nil {
		return false, err
	}
	feed, eval, err := s.prepare(definition, mode)
	if err != nil {
		return false, err
	}
	record, found, err := feed.Lookup(uid)
	if err != nil {
		return false, fmt.Errorf("audience: looking up %q in source %q: %w", uid, definition.source, err)
	}
	if !found {
		return false, nil
	}
	return definition.holdsFor(eval, record)
}

func (s *Service) prepare(definition Definition, mode Mode) (Feed, evaluation, error) {
	if definition.isZero() {
		return nil, evaluation{}, ErrUnbuiltDefinition
	}
	if err := mode.validate(); err != nil {
		return nil, evaluation{}, fmt.Errorf("%w (definition %q)", err, definition.id)
	}
	feed, ok := s.feeds[definition.source]
	if !ok {
		return nil, evaluation{}, fmt.Errorf("%w: %q, wanted by definition %q", ErrUnknownSource, definition.source, definition.id)
	}
	return feed, evaluation{experiments: s.experiments, mode: mode}, nil
}

// holdsFor reports whether every condition holds for this record.
//
// Both directions go through here today, so they agree by construction. That
// is worth stating plainly rather than claiming the property test proves
// something it does not: what the test guards is the day a fast path is added
// to one side and the two drift apart.
func (d Definition) holdsFor(eval evaluation, record Record) (bool, error) {
	for _, condition := range d.conditions {
		ok, err := condition.matches(eval, record)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
