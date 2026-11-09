package campaign

import (
	"errors"
	"strconv"
	"strings"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var (
	ErrMissingOccurrence            = errors.New("campaign: occurrence was not given")
	ErrPullCannotBeDeliveredInBatch = errors.New("campaign: a pull action cannot be delivered in a batch run")
)

// Occurrence identifies one entry into a treatment, as distinct from another
// attempt at that same entry.
type Occurrence string

func (o Occurrence) validate() error {
	if o == "" {
		return ErrMissingOccurrence
	}
	return nil
}

// IdempotencyKey identifies one delivery across retries.
type IdempotencyKey string

func deriveKey(c CampaignID, t TreatmentID, uid audience.UID, o Occurrence) IdempotencyKey {
	var b strings.Builder
	for _, part := range []string{string(c), string(t), string(uid), string(o)} {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
		b.WriteByte('|')
	}
	return IdempotencyKey(b.String())
}

// Request is the application-facing action input derived from a delivery
// intent.
type Request struct {
	Key  IdempotencyKey
	UID  audience.UID
	Call Call
}

// Ack is the value returned by an external delivery adapter.
type Ack struct {
	ExternalRef string
}
