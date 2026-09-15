package campaign

import "errors"

// ErrPriorityNotSet is returned when a campaign never said how it ranks.
var ErrPriorityNotSet = errors.New("campaign: priority was not set")

// Priority ranks campaigns against each other when they want the same slot.
// Higher wins.
//
// One value per campaign, used at every slot: ranking separately per slot was
// considered and rejected. It has no usable zero value, because a campaign
// that forgot to set one would otherwise rank lowest everywhere and quietly
// never win anything.
type Priority struct {
	value int
	set   bool
}

// NewPriority ranks a campaign. Any integer is allowed; only the ordering
// between campaigns matters.
func NewPriority(value int) Priority { return Priority{value: value, set: true} }

// Value reports the rank.
func (p Priority) Value() int { return p.value }

func (p Priority) validate() error {
	if !p.set {
		return ErrPriorityNotSet
	}
	return nil
}
