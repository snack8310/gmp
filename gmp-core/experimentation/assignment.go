package experimentation

import (
	"errors"
	"fmt"
)

var (
	// ErrAlgorithmVersionMismatch is returned when the supplied implementation
	// does not match the version captured by the definition.
	ErrAlgorithmVersionMismatch = errors.New("experimentation: algorithm version does not match the definition")
)

// Calculate computes a bucket from explicit values. It does not load or save
// coloring, inspect component lifecycle, or retain the result.
func Calculate(definition Definition, key AssignmentKey, algorithm Algorithm) (BucketNumber, error) {
	if definition.isZero() {
		return BucketNumber{}, ErrUnbuiltDefinition
	}
	if err := key.validate(); err != nil {
		return BucketNumber{}, err
	}
	if algorithm == nil {
		return BucketNumber{}, errors.New("experimentation: algorithm is nil")
	}
	if algorithm.Version() != definition.algorithm {
		return BucketNumber{}, fmt.Errorf("%w: definition %q wants %q, got %q", ErrAlgorithmVersionMismatch, definition.id, definition.algorithm, algorithm.Version())
	}
	position := algorithm.Position(key, definition.salt, definition.split.total)
	return definition.split.bucketFor(position)
}
