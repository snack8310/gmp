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
func SelectGroup(group Group) Selection {
	return Selection{group: group}
}

// SelectBundle makes one published experiment bundle the entry audience.
func SelectBundle(bundle Bundle) Selection {
	return Selection{bundle: &bundle}
}

// Validate checks that the selection contains exactly one built audience
// object and that an experiment bundle is published and active.
func (s Selection) Validate() error {
	if s.group.ID() != "" && s.bundle != nil {
		return ErrInvalidSelection
	}
	if s.bundle != nil {
		if s.bundle.id == "" || s.bundle.state == nil {
			return ErrInvalidSelection
		}
		if !s.bundle.state.published.Load() {
			return ErrUnpublishedBundle
		}
		if !s.bundle.Available() {
			return ErrGroupUnavailable
		}
		return nil
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

// Group returns one selected group by identity.
func (s Selection) Group(id GroupID) (Group, bool) {
	for _, group := range s.Groups() {
		if group.ID() == id {
			return group, true
		}
	}
	return Group{}, false
}

// ValidateSelection checks both the selection shape and that each group is
// registered and available in this audience service.
func (s *Service) ValidateSelection(selection Selection) error {
	if err := selection.Validate(); err != nil {
		return err
	}
	for _, group := range selection.Groups() {
		if err := s.ValidateGroup(group); err != nil {
			return err
		}
	}
	return nil
}

// MatchSelection returns the selected group containing uid, if any. Bundle
// arms are mutually exclusive; more than one matching arm means the bundle's
// audience definition no longer describes one experiment partition.
func (s *Service) MatchSelection(selection Selection, mode Mode, uid UID) (Group, bool, error) {
	if uid == "" {
		return Group{}, false, ErrEmptyUID
	}
	if err := s.ValidateSelection(selection); err != nil {
		return Group{}, false, err
	}
	var matched Group
	for _, group := range selection.Groups() {
		in, err := s.DecideGroup(group, mode, uid)
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
