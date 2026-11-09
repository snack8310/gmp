package experimentation

import "hash/fnv"

// Salt is supplied by the caller; this package does not invent its composition.
type Salt string

// AlgorithmVersion names the deterministic bucketing algorithm.
type AlgorithmVersion string

// Algorithm maps a key into a position in the supplied assignment space.
type Algorithm interface {
	Version() AlgorithmVersion
	Position(key AssignmentKey, salt Salt, total int) int
}

// FNV1aV1 is the first stable bucketing algorithm. Changes to its behavior
// require a new version.
type FNV1aV1 struct{}

func (FNV1aV1) Version() AlgorithmVersion { return "fnv1a-v1" }

func (FNV1aV1) Position(key AssignmentKey, salt Salt, total int) int {
	h := fnv.New64a()
	_, _ = h.Write([]byte(salt))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(key))
	return int(h.Sum64() % uint64(total))
}
