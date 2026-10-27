package campaign

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var (
	// ErrMissingTreatmentID is returned when a treatment has no identity.
	ErrMissingTreatmentID = errors.New("campaign: treatment id is empty")
	// ErrMissingTreatmentAudience is returned when a treatment does not say who
	// it is for.
	ErrMissingTreatmentAudience = errors.New("campaign: treatment has no audience definition")
	// ErrMissingTreatmentCall is returned when a treatment does not say what it
	// does.
	ErrMissingTreatmentCall = errors.New("campaign: treatment has no action call")
)

// TreatmentID identifies one treatment.
type TreatmentID string

// Treatment is one thing done to one person, in one step: one action.
//
// A later step is another treatment. Inside one campaign the flow connects
// them with edges, and a receipt edge says which outcome of this treatment's
// action runs which later treatment. Several treatments with no receipt edge
// between them are entries and can run in parallel. A different campaign can
// still be aimed at an audience built over what flowed back; that path is for
// cross-campaign reuse and for conditions an edge does not express, not the
// only way to take a second step inside one campaign.
type Treatment struct {
	id   TreatmentID
	slot SlotID
	who  audience.Definition
	call Call
}

// TreatmentSpec is the input to building a treatment.
type TreatmentSpec struct {
	// ID identifies the treatment.
	ID TreatmentID
	// Slot is where it wants to be shown.
	Slot SlotID
	// Who it is for.
	Who audience.Definition
	// Call is what it does: a registered action with its parameters.
	Call Call
}

// NewTreatment builds a treatment.
func NewTreatment(spec TreatmentSpec) (Treatment, error) {
	if spec.ID == "" {
		return Treatment{}, ErrMissingTreatmentID
	}
	if spec.Slot == "" {
		return Treatment{}, fmt.Errorf("%w: treatment %q", ErrMissingSlotID, spec.ID)
	}
	if spec.Who.ID() == "" {
		return Treatment{}, fmt.Errorf("%w: treatment %q", ErrMissingTreatmentAudience, spec.ID)
	}
	if spec.Call.isZero() {
		return Treatment{}, fmt.Errorf("%w: treatment %q", ErrMissingTreatmentCall, spec.ID)
	}
	return Treatment{id: spec.ID, slot: spec.Slot, who: spec.Who, call: spec.Call}, nil
}

// ID reports which treatment this is.
func (t Treatment) ID() TreatmentID { return t.id }

// Slot reports where it wants to be shown.
func (t Treatment) Slot() SlotID { return t.slot }

// Audience reports who it is for.
func (t Treatment) Audience() audience.Definition { return t.who }

// Call reports what it does.
func (t Treatment) Call() Call { return t.call }
