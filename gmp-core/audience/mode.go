package audience

import "errors"

// ErrModeNotSet is returned when evaluation is asked for without saying
// whether it is the real thing or a rehearsal.
var ErrModeNotSet = errors.New("audience: evaluation mode was not set")

// Mode says whether an evaluation is real or a rehearsal.
//
// The domain documents record the cost of this distinction plainly: the caller
// has to pass it correctly, and passing it wrong colours people silently. A
// bool would make the wrong value the zero value, so forgetting to pass it
// would read as a real run. This type has no usable zero value instead --
// forgetting it fails loudly rather than quietly doing the dangerous one.
type Mode struct {
	rehearsal bool
	set       bool
}

// Live is the real thing: coloured assignments are recorded.
func Live() Mode { return Mode{rehearsal: false, set: true} }

// Rehearsal computes everything and records nothing, so that previewing a
// configuration cannot change the run it is previewing.
func Rehearsal() Mode { return Mode{rehearsal: true, set: true} }

// IsRehearsal reports whether this is a rehearsal.
func (m Mode) IsRehearsal() bool { return m.rehearsal }

func (m Mode) validate() error {
	if !m.set {
		return ErrModeNotSet
	}
	return nil
}
