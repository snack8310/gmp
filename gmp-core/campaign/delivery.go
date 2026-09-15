package campaign

import (
	"errors"
	"fmt"
	"strings"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var (
	// ErrMissingOccurrence is returned when nobody said which entry this is.
	ErrMissingOccurrence = errors.New("campaign: occurrence was not given")
	// ErrNoDeliverer is returned when an action has nowhere to be delivered.
	ErrNoDeliverer = errors.New("campaign: no deliverer registered for this action")
	// ErrDuplicateDeliverer is returned when one action is wired twice.
	ErrDuplicateDeliverer = errors.New("campaign: action already has a deliverer")
	// ErrPullCannotBeDeliveredInBatch is returned when a batch run is asked to
	// push something an outside system was supposed to come and ask for.
	//
	// Nothing is waiting on the other end of a pull action: whoever it was for
	// has already gone. Retrying one is not a smaller mistake than never
	// sending it.
	ErrPullCannotBeDeliveredInBatch = errors.New("campaign: a pull action cannot be delivered in a batch run")
)

// Occurrence identifies one entry into a treatment, as distinct from one more
// try at the same entry.
//
// It has to come from whoever triggered the run, because only they know which
// it is: the same person satisfying a condition again is a new entry and
// deserves a new delivery, while a timeout is the same entry and must not.
// There is no default -- guessing here means either sending twice or dropping
// one, both silently.
type Occurrence string

func (o Occurrence) validate() error {
	if o == "" {
		return ErrMissingOccurrence
	}
	return nil
}

// IdempotencyKey is what the downstream uses to recognise a delivery it has
// already handled.
//
// It is derived from what identifies the delivery, not generated afresh, which
// is what makes "the same key across retries" structural rather than something
// to remember. Changing it would mean changing the delivery's identity.
type IdempotencyKey string

// deriveKey composes the key. The parts are length-prefixed so that no
// combination of ids can be read as a different one.
func deriveKey(c CampaignID, t TreatmentID, uid audience.UID, o Occurrence) IdempotencyKey {
	var b strings.Builder
	for _, part := range []string{string(c), string(t), string(uid), string(o)} {
		fmt.Fprintf(&b, "%d:%s|", len(part), part)
	}
	return IdempotencyKey(b.String())
}

// Request is one delivery handed to the outside world.
type Request struct {
	// Key identifies this delivery. The downstream must treat two requests
	// carrying the same key as one.
	Key IdempotencyKey
	// UID is who it is for.
	UID audience.UID
	// Call is what to do, with its parameters.
	Call Call
}

// Ack is what the downstream says when it takes a delivery.
//
// Taking it is not the same as it arriving. That comes later, as a receipt, and
// for a message the difference is routine: the interface returning success is
// not the person receiving anything.
type Ack struct {
	// ExternalRef is the downstream's own handle, if it gave one.
	ExternalRef string
}

// Deliverer hands a request to whatever actually carries it out.
//
// Implementations must be idempotent: two requests with one key must produce
// one effect. That is a condition of being wired up at all rather than
// something checked per call -- and it is a claim, not a guarantee, so
// DelivererContract exists to make a new one demonstrate it.
type Deliverer interface {
	Deliver(Request) (Ack, error)
}
