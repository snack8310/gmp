package campaign

import (
	"errors"
	"fmt"
)

var (
	// ErrMissingSlotID is returned when a slot has no identity.
	ErrMissingSlotID = errors.New("campaign: slot id is empty")
	ErrUnknownSlot   = errors.New("campaign: slot is absent from the supplied catalog")
)

// SlotID identifies a place where something can be shown.
type SlotID string

// Slot is a value describing a place where a treatment can be shown. The
// application supplies the available slot catalog to pure decisions.
type Slot struct {
	id SlotID
}

// NewSlot builds a slot value.
func NewSlot(id SlotID) (Slot, error) {
	if id == "" {
		return Slot{}, ErrMissingSlotID
	}
	return Slot{id: id}, nil
}

// ID reports which slot this is.
func (s Slot) ID() SlotID { return s.id }

func (s Slot) isZero() bool { return s.id == "" }

func (s Slot) String() string { return fmt.Sprintf("slot %q", string(s.id)) }
