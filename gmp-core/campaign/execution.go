package campaign

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var (
	// ErrNoSuchExecution is returned when a receipt carries a key nothing was
	// ever delivered under.
	//
	// Dropping such a receipt quietly is how an execution ends up hanging for
	// good: the delivery is out there, nothing will ever say what became of it,
	// and no error was ever raised.
	ErrNoSuchExecution = errors.New("campaign: no execution under this key")
	// ErrNonPositiveAttempts is returned when a runner is told to try a
	// non-positive number of times.
	ErrNonPositiveAttempts = errors.New("campaign: attempt limit is not positive")
	// ErrDeliveryFailed wraps a downstream failure that survived every attempt.
	ErrDeliveryFailed = errors.New("campaign: delivery failed after every attempt")
	// ErrNoSuchTreatment is returned when a run names a treatment the campaign
	// does not hold.
	ErrNoSuchTreatment = errors.New("campaign: campaign does not hold this treatment")
	// ErrConflictingReceipt is returned when an execution already has an
	// accepted receipt and a different receipt arrives for the same key.
	ErrConflictingReceipt = errors.New("campaign: a different receipt was already accepted for this execution")
)

// State is where one execution has got to.
//
// Only what this round's actions can actually produce is here. A full lifecycle
// for an execution has never been settled, and inventing the rest would put
// states in the model that nothing can reach.
type State string

const (
	// StateDelivered means the downstream took it. Whether it arrived is not
	// yet known.
	StateDelivered State = "delivered"
	// StateFailed means every attempt failed.
	StateFailed State = "failed"
	// StateArrived means a receipt said it really arrived.
	StateArrived State = "arrived"
	// StateNotArrived means a receipt said it did not.
	StateNotArrived State = "not-arrived"
)

// Execution is the record of one delivery.
//
// It carries both the treatment and where the person was placed, because the
// two answer different questions and neither substitutes for the other: without
// the placement a result cannot be attributed to an arm, and without the
// treatment there is no saying how a treatment did.
type Execution struct {
	Key        IdempotencyKey
	Campaign   CampaignID
	Treatment  TreatmentID
	UID        audience.UID
	Action     ActionID
	Placements []audience.Placement
	State      State
	Attempts   int
	// ExternalRef is the reference returned when the delivery was accepted.
	ExternalRef string
	// Outcome is the receipt result in the action's vocabulary, empty until a
	// receipt classifies one.
	Outcome Outcome
	// ReceiptExternalRef is the downstream's reference reported by the first
	// accepted receipt, separate from the reference returned when delivery was
	// accepted.
	ReceiptExternalRef string
	// ReceiptAttributes are the attributes supplied with the accepted receipt.
	ReceiptAttributes map[string]string
}

// HasReceipt reports whether a receipt has classified this execution.
func (e Execution) HasReceipt() bool {
	return e.State == StateArrived || e.State == StateNotArrived
}

// MatchesReceipt reports whether a receipt exactly matches this execution's
// accepted receipt snapshot.
func (e Execution) MatchesReceipt(receipt Receipt) bool {
	if !e.HasReceipt() || e.Outcome != receipt.Outcome || e.ReceiptExternalRef != receipt.ExternalRef || !sameStringMap(e.ReceiptAttributes, receipt.Attributes) {
		return false
	}
	return (e.State == StateArrived) == receipt.Arrived
}

func (e Execution) sameAcceptedReceipt(other Execution) bool {
	return e.HasReceipt() && other.HasReceipt() && e.State == other.State && e.Outcome == other.Outcome &&
		e.ReceiptExternalRef == other.ReceiptExternalRef && sameStringMap(e.ReceiptAttributes, other.ReceiptAttributes)
}

func sameStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if other, ok := right[key]; !ok || other != value {
			return false
		}
	}
	return true
}

// Log keeps executions. Records are filed under the idempotency key, which is
// also what a receipt arrives carrying.
type Log interface {
	Append(Execution) error
	Update(Execution) error
	Lookup(IdempotencyKey) (Execution, bool, error)
	All() ([]Execution, error)
}

// MemoryLog is an in-memory Log: the first implementation.
type MemoryLog struct {
	mu      sync.Mutex
	entries map[IdempotencyKey]Execution
}

// NewMemoryLog builds an empty log.
func NewMemoryLog() *MemoryLog {
	return &MemoryLog{entries: make(map[IdempotencyKey]Execution)}
}

// Append implements Log. An existing key is left alone: the same delivery
// recorded twice is one delivery.
func (l *MemoryLog) Append(execution Execution) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, held := l.entries[execution.Key]; held {
		return nil
	}
	l.entries[execution.Key] = execution
	return nil
}

// Update implements Log.
func (l *MemoryLog) Update(execution Execution) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	previous, held := l.entries[execution.Key]
	if !held {
		return fmt.Errorf("%w: %q", ErrNoSuchExecution, execution.Key)
	}
	if previous.HasReceipt() && !previous.sameAcceptedReceipt(execution) {
		return fmt.Errorf("%w: %q", ErrConflictingReceipt, execution.Key)
	}
	l.entries[execution.Key] = execution
	return nil
}

// Lookup implements Log.
func (l *MemoryLog) Lookup(key IdempotencyKey) (Execution, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	execution, held := l.entries[key]
	return execution, held, nil
}

// All implements Log, in a stable order.
func (l *MemoryLog) All() ([]Execution, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Execution, 0, len(l.entries))
	for _, execution := range l.entries {
		out = append(out, execution)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Size reports how many executions are held.
func (l *MemoryLog) Size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.entries)
}
