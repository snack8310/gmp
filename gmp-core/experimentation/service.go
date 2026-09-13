package experimentation

import (
	"errors"
	"fmt"
)

var (
	// ErrUnbuiltDefinition is returned when a zero-value Definition is passed
	// in rather than one that went through a constructor and its checks.
	ErrUnbuiltDefinition = errors.New("experimentation: definition was not built through a constructor")
	// ErrNoColoringStore is returned when an assignment that records its
	// results is evaluated by a service that has nowhere to record them.
	ErrNoColoringStore = errors.New("experimentation: definition is coloured but the service has no coloring store")
)

// Service answers which share a key falls into.
//
// The interface is deliberately only (key, definition) -> share. There is no
// convenience entry point that takes a segment: the whole reason this package
// can be opened to third parties is that it has no idea what a segment is, and
// adding that shortcut would tie it to this project permanently.
type Service struct {
	algorithms *AlgorithmRegistry
	coloring   ColoringStore
}

// NewService builds a service over the given algorithm registry and coloring
// store. The store may be nil only if no definition passed in ever colours.
func NewService(algorithms *AlgorithmRegistry, coloring ColoringStore) (*Service, error) {
	if algorithms == nil {
		return nil, errors.New("experimentation: algorithm registry is nil")
	}
	return &Service{algorithms: algorithms, coloring: coloring}, nil
}

// Assign returns the share this key falls into, recording the result when the
// definition colours.
//
// For a coloured assignment an existing record always wins, which is what stops
// keys from jumping shares when the definition changes.
func (s *Service) Assign(definition Definition, key AssignmentKey) (BucketNumber, error) {
	return s.evaluate(definition, key, true)
}

// PeekAssign returns the share this key falls into without ever writing a
// coloring record.
//
// This is the read-only ask a rehearsal uses. Asking the ordinary way during a
// rehearsal would colour the key, and the rehearsal would then have changed the
// real run it was supposed to preview.
func (s *Service) PeekAssign(definition Definition, key AssignmentKey) (BucketNumber, error) {
	return s.evaluate(definition, key, false)
}

func (s *Service) evaluate(definition Definition, key AssignmentKey, mayRecord bool) (BucketNumber, error) {
	if definition.isZero() {
		return BucketNumber{}, ErrUnbuiltDefinition
	}
	if err := key.validate(); err != nil {
		return BucketNumber{}, err
	}

	if definition.colored {
		if s.coloring == nil {
			return BucketNumber{}, fmt.Errorf("%w: assignment %q", ErrNoColoringStore, definition.id)
		}
		existing, found, err := s.coloring.Lookup(definition.id, key)
		if err != nil {
			return BucketNumber{}, fmt.Errorf("experimentation: looking up coloring for %q: %w", definition.id, err)
		}
		if found {
			return existing.Bucket, nil
		}
	}

	algorithm, err := s.algorithms.resolve(definition.algorithm)
	if err != nil {
		return BucketNumber{}, fmt.Errorf("%w (assignment %q)", err, definition.id)
	}
	position := algorithm.Position(key, definition.salt, definition.split.Total())
	bucket, err := definition.split.bucketFor(position)
	if err != nil {
		return BucketNumber{}, fmt.Errorf("experimentation: assignment %q: %w", definition.id, err)
	}

	if !definition.colored || !mayRecord {
		return bucket, nil
	}
	inForce, err := s.coloring.RecordFirst(definition.id, key, ColoringRecord{
		Bucket:    bucket,
		Algorithm: definition.algorithm,
	})
	if err != nil {
		return BucketNumber{}, fmt.Errorf("experimentation: recording coloring for %q: %w", definition.id, err)
	}
	return inForce.Bucket, nil
}
