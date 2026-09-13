package experimentation

import "errors"

// ErrEmptyKey is returned when an assignment key carries no bytes. There is no
// sensible bucket for the absence of a key, and silently bucketing "" would put
// every caller that forgot to populate the field into the same share.
var ErrEmptyKey = errors.New("experimentation: assignment key is empty")

// AssignmentKey is opaque to this package. The caller decides whether it holds
// a user id, a device id or an order id; nothing here inspects its contents.
//
// The spec is explicit that keeping this opaque is what makes the service
// openable to third parties, so no method here may grow an opinion about what
// the key represents.
type AssignmentKey string

func (k AssignmentKey) validate() error {
	if len(k) == 0 {
		return ErrEmptyKey
	}
	return nil
}
