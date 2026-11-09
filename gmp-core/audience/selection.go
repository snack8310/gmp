package audience

import "errors"

var (
	ErrInvalidSelection  = errors.New("audience: selection must contain one built group or one bundle")
	ErrUnpublishedBundle = errors.New("audience: selected bundle is not published")
	ErrSelectionOverlaps = errors.New("audience: more than one selected group matches the uid")
)

// Selection is one activity entry audience: either one ordinary group or one
// published experiment bundle. Its private fields prevent callers from
// combining unrelated groups into a multi-audience entry.
type Selection struct {
	group  Group
	bundle *Bundle
}

// SelectGroup makes one ordinary group the entry audience.
func SelectGroup(group Group) Selection { return Selection{group: group} }

// SelectBundle makes one experiment bundle the entry audience.
func SelectBundle(bundle Bundle) Selection { return Selection{bundle: &bundle} }

// Validate checks only the values in the selection. Current component
// availability is a runtime concern for the application layer.
func (s Selection) Validate() error {
	if s.group.ID() != "" && s.bundle != nil {
		return ErrInvalidSelection
	}
	if s.bundle != nil {
		if s.group.ID() != "" || s.bundle.id == "" {
			return ErrInvalidSelection
		}
		return s.bundle.Validate()
	}
	if s.group.ID() == "" {
		return ErrInvalidSelection
	}
	return s.group.Validate()
}

// Groups returns the selected group or the bundle's arms in share order.
func (s Selection) Groups() []Group {
	if s.bundle == nil {
		if s.group.ID() == "" {
			return nil
		}
		return []Group{s.group}
	}
	groups := make([]Group, 0, len(s.bundle.arms))
	for share := 1; share <= len(s.bundle.arms); share++ {
		if group, ok := s.bundle.arms[share]; ok {
			groups = append(groups, group)
		}
	}
	return groups
}

// IsBundle reports whether this selection is an experiment bundle.
func (s Selection) IsBundle() bool { return s.bundle != nil }

// Match returns the selected group containing record, using only explicit
// records and assignment facts.
func (s Selection) Match(record Record, facts AssignmentFacts) (Group, bool, error) {
	if err := s.Validate(); err != nil {
		return Group{}, false, err
	}
	var matched Group
	for _, group := range s.Groups() {
		in, err := group.Matches(record, facts)
		if err != nil {
			return Group{}, false, err
		}
		if !in {
			continue
		}
		if matched.ID() != "" {
			return Group{}, false, ErrSelectionOverlaps
		}
		matched = group
	}
	if matched.ID() == "" {
		return Group{}, false, nil
	}
	return matched, true, nil
}

// Group returns one selected group by identity.
func (s Selection) Group(id GroupID) (Group, bool) {
	for _, group := range s.Groups() {
		if group.ID() == id {
			return group, true
		}
	}
	return Group{}, false
}
