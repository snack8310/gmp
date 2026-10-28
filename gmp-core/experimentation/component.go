package experimentation

import (
	"errors"
	"fmt"
)

var (
	ErrComponentUnknown    = errors.New("experimentation: component is not registered")
	ErrComponentExists     = errors.New("experimentation: component is already registered")
	ErrComponentNotRunning = errors.New("experimentation: component is not running")
	ErrComponentRunning    = errors.New("experimentation: loaded component logic cannot change while running")
	ErrComponentRevision   = errors.New("experimentation: component definition revision differs")
	ErrComponentStructure  = errors.New("experimentation: runtime split changes component structure")
)

type component struct {
	definition Definition
	split      Split
	whitelist  map[AssignmentKey]BucketNumber
	running    bool
	generation uint64
}

func (s *Service) component(id AssignmentID) (component, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.components[id]
	return value, ok
}

// RegisterComponent loads a definition without starting it.
func (s *Service) RegisterComponent(definition Definition) error {
	if definition.isZero() {
		return ErrUnbuiltDefinition
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.components[definition.id]; exists {
		return ErrComponentExists
	}
	s.components[definition.id] = component{definition: definition, split: definition.split, generation: 1}
	return nil
}

// ComponentGeneration identifies the loaded revision used by a published bundle.
func (s *Service) ComponentGeneration(definition Definition) (uint64, error) {
	current, ok := s.component(definition.id)
	if !ok {
		return 0, ErrComponentUnknown
	}
	if !current.definition.SameRevisionAs(definition) {
		return 0, ErrComponentRevision
	}
	return current.generation, nil
}

// ComponentActive fails closed after a stop or revision, including after a later start.
func (s *Service) ComponentActive(definition Definition, generation uint64) bool {
	current, ok := s.component(definition.id)
	return ok && current.running && current.generation == generation && current.definition.SameRevisionAs(definition)
}

func (s *Service) StartComponent(id AssignmentID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.components[id]
	if !ok {
		return ErrComponentUnknown
	}
	current.running = true
	s.components[id] = current
	return nil
}

func (s *Service) StopComponent(id AssignmentID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.components[id]
	if !ok {
		return ErrComponentUnknown
	}
	current.running = false
	current.generation++
	s.components[id] = current
	return nil
}

// ReviseComponent replaces loaded logic only while not running.
func (s *Service) ReviseComponent(definition Definition) error {
	if definition.isZero() {
		return ErrUnbuiltDefinition
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.components[definition.id]
	if !ok {
		return ErrComponentUnknown
	}
	if current.running {
		return ErrComponentRunning
	}
	current.definition = definition
	current.split = definition.split
	current.whitelist = nil
	current.generation++
	s.components[definition.id] = current
	return nil
}

// UpdateSplit changes ratios while preserving the loaded number of shares and total.
func (s *Service) UpdateSplit(id AssignmentID, split Split) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.components[id]
	if !ok {
		return ErrComponentUnknown
	}
	if !current.running {
		return ErrComponentNotRunning
	}
	if split.isZero() || split.Total() != current.definition.split.Total() || split.Shares() != current.definition.split.Shares() {
		return ErrComponentStructure
	}
	current.split = split
	s.components[id] = current
	return nil
}

// UpdateWhitelist fixes new assignments for the supplied assignment keys to shares.
func (s *Service) UpdateWhitelist(id AssignmentID, entries map[AssignmentKey]BucketNumber) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.components[id]
	if !ok {
		return ErrComponentUnknown
	}
	if !current.running {
		return ErrComponentNotRunning
	}
	owned := make(map[AssignmentKey]BucketNumber, len(entries))
	for key, share := range entries {
		if err := key.validate(); err != nil {
			return err
		}
		if share.IsZero() || share.Value() > current.split.Shares() {
			return fmt.Errorf("%w: %s", ErrBucketOutOfRange, share)
		}
		owned[key] = share
	}
	current.whitelist = owned
	s.components[id] = current
	return nil
}
