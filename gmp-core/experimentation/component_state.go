package experimentation

import (
	"errors"
	"fmt"
)

var (
	ErrComponentNotRunning = errors.New("experimentation: component is not running")
	ErrComponentRunning    = errors.New("experimentation: component cannot change while running")
	ErrComponentRevision   = errors.New("experimentation: component definition revision differs")
	ErrComponentStructure  = errors.New("experimentation: runtime split changes component structure")
)

// ComponentState is the explicit lifecycle and runtime assignment state for
// one experiment component. Transitions return a new value and never mutate
// the state passed in.
type ComponentState struct {
	definition Definition
	split      Split
	whitelist  map[AssignmentKey]BucketNumber
	running    bool
	generation uint64
}

// NewComponentState creates an inactive component at generation one.
func NewComponentState(definition Definition) (ComponentState, error) {
	if definition.isZero() {
		return ComponentState{}, ErrUnbuiltDefinition
	}
	return ComponentState{
		definition: definition,
		split:      definition.split,
		generation: 1,
	}, nil
}

// Definition returns the definition held by this state.
func (s ComponentState) Definition() Definition { return s.definition }

// Split returns the current runtime split.
func (s ComponentState) Split() Split { return s.split }

// Running reports whether this component is active.
func (s ComponentState) Running() bool { return s.running }

// Generation reports the generation used to invalidate previously published
// groups after a stop or revision.
func (s ComponentState) Generation() uint64 { return s.generation }

// WhitelistFor returns the explicitly pinned share for key, if any.
func (s ComponentState) WhitelistFor(key AssignmentKey) (BucketNumber, bool) {
	bucket, ok := s.whitelist[key]
	return bucket, ok
}

// Start returns a running state without changing its generation.
func (s ComponentState) Start() (ComponentState, error) {
	if s.definition.isZero() {
		return ComponentState{}, ErrUnbuiltDefinition
	}
	s.running = true
	return s, nil
}

// Stop returns an inactive state and advances its generation.
func (s ComponentState) Stop() (ComponentState, error) {
	if s.definition.isZero() {
		return ComponentState{}, ErrUnbuiltDefinition
	}
	s.running = false
	s.generation++
	return s, nil
}

// Revise replaces the loaded definition only while inactive.
func (s ComponentState) Revise(definition Definition) (ComponentState, error) {
	if definition.isZero() {
		return ComponentState{}, ErrUnbuiltDefinition
	}
	if s.definition.isZero() {
		return ComponentState{}, ErrUnbuiltDefinition
	}
	if s.running {
		return ComponentState{}, ErrComponentRunning
	}
	if definition.ID() != s.definition.ID() {
		return ComponentState{}, ErrComponentRevision
	}
	s.definition = definition
	s.split = definition.split
	s.whitelist = nil
	s.generation++
	return s, nil
}

// UpdateSplit changes ratios while retaining the loaded split structure.
func (s ComponentState) UpdateSplit(split Split) (ComponentState, error) {
	if s.definition.isZero() {
		return ComponentState{}, ErrUnbuiltDefinition
	}
	if !s.running {
		return ComponentState{}, ErrComponentNotRunning
	}
	if split.isZero() || split.Total() != s.definition.split.Total() || split.Shares() != s.definition.split.Shares() {
		return ComponentState{}, ErrComponentStructure
	}
	s.split = split
	return s, nil
}

// UpdateWhitelist replaces the explicit assignments for future calculations.
func (s ComponentState) UpdateWhitelist(entries map[AssignmentKey]BucketNumber) (ComponentState, error) {
	if s.definition.isZero() {
		return ComponentState{}, ErrUnbuiltDefinition
	}
	if !s.running {
		return ComponentState{}, ErrComponentNotRunning
	}
	owned := make(map[AssignmentKey]BucketNumber, len(entries))
	for key, bucket := range entries {
		if err := key.validate(); err != nil {
			return ComponentState{}, err
		}
		if bucket.IsZero() || bucket.Value() > s.split.Shares() {
			return ComponentState{}, fmt.Errorf("%w: %s", ErrBucketOutOfRange, bucket)
		}
		owned[key] = bucket
	}
	s.whitelist = owned
	return s, nil
}

// Active reports whether this exact definition and generation is running.
func (s ComponentState) Active(definition Definition, generation uint64) bool {
	return !s.definition.isZero() && s.running && s.generation == generation && s.definition.SameRevisionAs(definition)
}

// AssignmentResult reports the bucket and, when needed, the coloring value an
// outer application should persist.
type AssignmentResult struct {
	Bucket       BucketNumber
	Coloring     ColoringRecord
	NeedsPersist bool
}

// ResolveAssignment calculates from explicit values without reading or writing
// a coloring store. A persisted coloring wins; the caller decides whether to
// store a newly produced coloring and must use its atomic first-write result.
func ResolveAssignment(
	definition Definition,
	key AssignmentKey,
	algorithm Algorithm,
	component *ComponentState,
	existing *ColoringRecord,
) (AssignmentResult, error) {
	if definition.isZero() {
		return AssignmentResult{}, ErrUnbuiltDefinition
	}
	if err := key.validate(); err != nil {
		return AssignmentResult{}, err
	}
	if component != nil {
		if component.definition.isZero() {
			return AssignmentResult{}, ErrUnbuiltDefinition
		}
		if !component.definition.SameRevisionAs(definition) {
			return AssignmentResult{}, ErrComponentRevision
		}
		if !component.running {
			return AssignmentResult{}, ErrComponentNotRunning
		}
	}
	if existing != nil {
		return AssignmentResult{Bucket: existing.Bucket, Coloring: *existing}, nil
	}

	if component != nil {
		if bucket, pinned := component.whitelist[key]; pinned {
			return assignmentResult(definition, bucket), nil
		}
	}
	if algorithm == nil {
		return AssignmentResult{}, errors.New("experimentation: algorithm is nil")
	}
	if algorithm.Version() != definition.algorithm {
		return AssignmentResult{}, fmt.Errorf("%w: definition %q wants %q, got %q", ErrAlgorithmVersionMismatch, definition.id, definition.algorithm, algorithm.Version())
	}
	split := definition.split
	if component != nil {
		split = component.split
	}
	position := algorithm.Position(key, definition.salt, split.total)
	bucket, err := split.bucketFor(position)
	if err != nil {
		return AssignmentResult{}, fmt.Errorf("experimentation: assignment %q: %w", definition.id, err)
	}
	return assignmentResult(definition, bucket), nil
}

func assignmentResult(definition Definition, bucket BucketNumber) AssignmentResult {
	result := AssignmentResult{Bucket: bucket}
	if definition.colored {
		result.Coloring = ColoringRecord{Bucket: bucket, Algorithm: definition.algorithm}
		result.NeedsPersist = true
	}
	return result
}
