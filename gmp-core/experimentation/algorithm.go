package experimentation

import (
	"errors"
	"fmt"
	"hash/fnv"
)

var (
	// ErrUnknownAlgorithm is returned when a definition names a bucketing
	// algorithm version the registry does not hold. Replaying an assignment
	// made under a retired version has to fail loudly: recomputing it under
	// today's version would silently disagree with what actually happened.
	ErrUnknownAlgorithm = errors.New("experimentation: unknown bucketing algorithm version")
	// ErrDuplicateAlgorithm is returned when two algorithms claim one version.
	ErrDuplicateAlgorithm = errors.New("experimentation: bucketing algorithm version registered twice")
)

// Salt is the value mixed into the hash so that separate assignments scatter
// keys independently.
//
// This package never generates one. The domain documents make cross-tenant
// isolation depend on the tenant being inside the salt, but never state how a
// salt is composed -- so composing one here would be inventing the very rule
// that was left open. The caller supplies it.
type Salt string

// AlgorithmVersion names one bucketing algorithm. Every assignment records the
// version it used, because an algorithm upgrade must not quietly re-bucket
// history.
type AlgorithmVersion string

// Algorithm maps a key into a position inside a space of the given size.
//
// Implementations must be pure: the same key, salt and total must always yield
// the same position, in this process and in the next one.
type Algorithm interface {
	Version() AlgorithmVersion
	Position(key AssignmentKey, salt Salt, total int) int
}

// AlgorithmRegistry resolves the version recorded on a definition to the
// algorithm that implements it.
type AlgorithmRegistry struct {
	byVersion map[AlgorithmVersion]Algorithm
}

// NewAlgorithmRegistry builds a registry holding the given algorithms.
func NewAlgorithmRegistry(algorithms ...Algorithm) (*AlgorithmRegistry, error) {
	r := &AlgorithmRegistry{byVersion: make(map[AlgorithmVersion]Algorithm, len(algorithms))}
	for _, a := range algorithms {
		if err := r.Register(a); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds one algorithm. Versions are immutable once taken: re-binding a
// version would change how already-recorded assignments replay.
func (r *AlgorithmRegistry) Register(a Algorithm) error {
	if a == nil {
		return errors.New("experimentation: cannot register a nil algorithm")
	}
	v := a.Version()
	if v == "" {
		return fmt.Errorf("%w: algorithm reports an empty version", ErrMissingAlgorithmVersion)
	}
	if _, taken := r.byVersion[v]; taken {
		return fmt.Errorf("%w: %q", ErrDuplicateAlgorithm, v)
	}
	r.byVersion[v] = a
	return nil
}

func (r *AlgorithmRegistry) resolve(v AlgorithmVersion) (Algorithm, error) {
	a, ok := r.byVersion[v]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownAlgorithm, v)
	}
	return a, nil
}

// FNV1aV1 is the first bucketing algorithm. Its version string is part of the
// contract: changing what this implementation computes requires a new version,
// never an edit here.
type FNV1aV1 struct{}

// Version implements Algorithm.
func (FNV1aV1) Version() AlgorithmVersion { return "fnv1a-v1" }

// Position implements Algorithm. Salt is mixed in ahead of the key so that two
// assignments with different salts scatter the same keys differently.
func (FNV1aV1) Position(key AssignmentKey, salt Salt, total int) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(salt))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key))
	return int(h.Sum64() % uint64(total))
}
