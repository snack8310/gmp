package campaign

import (
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
)

type EventName string

const (
	eventDelivered  = ":delivered"
	eventArrived    = ":arrived"
	eventNotArrived = ":not-arrived"
)

func DeliveredEvent(action ActionID) EventName  { return EventName(string(action) + eventDelivered) }
func ArrivedEvent(action ActionID) EventName    { return EventName(string(action) + eventArrived) }
func NotArrivedEvent(action ActionID) EventName { return EventName(string(action) + eventNotArrived) }

// Event is the value the application may publish after committing a receipt.
type Event struct {
	Name       EventName
	UID        audience.UID
	Key        IdempotencyKey
	Campaign   CampaignID
	Treatment  TreatmentID
	Action     ActionID
	Placements []audience.Placement
	Attributes map[string]string
	Outcome    Outcome
}

// Receipt is the downstream report for one delivery.
type Receipt struct {
	Key         IdempotencyKey
	Arrived     bool
	ExternalRef string
	Attributes  map[string]string
	Outcome     Outcome
}

func (r Receipt) validate() error {
	if r.Key == "" {
		return fmt.Errorf("%w: a receipt with no key", ErrNoSuchExecution)
	}
	return nil
}
