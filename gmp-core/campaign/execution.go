package campaign

import (
	"errors"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var (
	ErrNoSuchExecution     = errors.New("campaign: no execution under this key")
	ErrNonPositiveAttempts = errors.New("campaign: attempt count is not positive")
	ErrNoSuchTreatment     = errors.New("campaign: campaign does not hold this treatment")
	ErrConflictingReceipt  = errors.New("campaign: a different receipt was already accepted for this execution")
)

// State is the outcome value for one execution.
type State string

const (
	StateDelivered  State = "delivered"
	StateFailed     State = "failed"
	StateArrived    State = "arrived"
	StateNotArrived State = "not-arrived"
)

// Execution is one delivery and its receipt snapshot.
type Execution struct {
	Key                IdempotencyKey
	Campaign           CampaignID
	Treatment          TreatmentID
	UID                audience.UID
	Action             ActionID
	Placements         []audience.Placement
	State              State
	Attempts           int
	ExternalRef        string
	Outcome            Outcome
	ReceiptExternalRef string
	ReceiptAttributes  map[string]string
}

func (e Execution) HasReceipt() bool {
	return e.State == StateArrived || e.State == StateNotArrived
}

func (e Execution) MatchesReceipt(receipt Receipt) bool {
	if !e.HasReceipt() || e.Outcome != receipt.Outcome || e.ReceiptExternalRef != receipt.ExternalRef || len(e.ReceiptAttributes) != len(receipt.Attributes) {
		return false
	}
	for key, value := range e.ReceiptAttributes {
		if other, ok := receipt.Attributes[key]; !ok || other != value {
			return false
		}
	}
	return (e.State == StateArrived) == receipt.Arrived
}
