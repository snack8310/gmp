package campaign

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingSlotID is returned when a slot has no identity.
	ErrMissingSlotID = errors.New("campaign: slot id is empty")
	// ErrUnknownSlot is returned when something references a slot nobody
	// registered.
	ErrUnknownSlot = errors.New("campaign: no such registered slot")
	// ErrDuplicateSlot is returned when one slot id is registered twice.
	ErrDuplicateSlot = errors.New("campaign: slot registered twice")
)

// SlotID identifies a place where something can be shown.
type SlotID string

// Slot is somewhere a treatment can be placed.
//
// Slots are registered rather than named freely, for the same reason sources
// and actions are: what exists is what engineering has wired up, and operations
// picks from that list. A typo in a free-form name would otherwise produce a
// slot nothing ever contends for, silently.
type Slot struct {
	id SlotID
}

// RegisterSlot declares a slot.
func RegisterSlot(id SlotID) (Slot, error) {
	if id == "" {
		return Slot{}, ErrMissingSlotID
	}
	return Slot{id: id}, nil
}

// ID reports which slot this is.
func (s Slot) ID() SlotID { return s.id }

func (s Slot) isZero() bool { return s.id == "" }

func (s Slot) String() string { return fmt.Sprintf("slot %q", string(s.id)) }
