package audience

import (
	"errors"
	"fmt"
	"time"

	"github.com/snack8310/gmp/gmp-core/experimentation"
)

var (
	// ErrMissingAttributeName is returned when a condition tests a fact with
	// no name.
	ErrMissingAttributeName = errors.New("audience: attribute name is empty")
	// ErrMissingEventName is returned when a time-window condition names no
	// event.
	ErrMissingEventName = errors.New("audience: event name is empty")
	// ErrNonPositiveWindow is returned when a time window is not a real length
	// of time.
	ErrNonPositiveWindow = errors.New("audience: time window is not positive")
	// ErrWindowShorterThanGranularity is returned when a condition asks about a
	// stretch of time shorter than its source can see.
	//
	// This is the one the domain documents call out by example: asking who has
	// not opened a message ten minutes after sending, when results come back
	// hour by hour, answers "nobody opened it" for everyone. The platform then
	// sends the follow-up to all of them, quietly and without failing.
	ErrWindowShorterThanGranularity = errors.New("audience: time window is shorter than the source's backflow granularity")
	// ErrUnbuiltShareCondition is returned when a share condition is built from
	// an assignment definition that never went through a constructor.
	ErrUnbuiltShareCondition = errors.New("audience: share condition needs a built assignment definition")
)

// Condition is one test a person either passes or does not.
type Condition interface {
	matches(eval evaluation, record Record) (bool, error)
	// Describe renders the condition for a human reading a configuration.
	Describe() string
}

// evaluation carries what conditions need beyond the record itself.
type evaluation struct {
	experiments *experimentation.Service
	mode        Mode
}

// --- attribute ---------------------------------------------------------------

type attributeCondition struct {
	name  string
	value string
}

// Attribute matches people whose source reports the given fact.
func Attribute(name, value string) (Condition, error) {
	if name == "" {
		return nil, ErrMissingAttributeName
	}
	return attributeCondition{name: name, value: value}, nil
}

func (c attributeCondition) matches(_ evaluation, record Record) (bool, error) {
	got, present := record.Attributes[c.name]
	return present && got == c.value, nil
}

func (c attributeCondition) Describe() string {
	return fmt.Sprintf("%s = %q", c.name, c.value)
}

// --- share of an assignment --------------------------------------------------

type shareCondition struct {
	assignment experimentation.Definition
	share      experimentation.BucketNumber
}

// Share matches people who fall into the given share of the given assignment.
//
// This is an ordinary condition, the same kind of thing as an attribute. This
// package never buckets anyone: it asks the experimentation layer through the
// only entry point that layer offers, and treats the answer as a fact about
// the person.
func Share(assignment experimentation.Definition, share experimentation.BucketNumber) (Condition, error) {
	if assignment.ID() == "" {
		return nil, ErrUnbuiltShareCondition
	}
	if share.IsZero() {
		return nil, fmt.Errorf("%w: share is the zero value, not a share", experimentation.ErrBucketOutOfRange)
	}
	if share.Value() > assignment.Split().Shares() {
		return nil, fmt.Errorf("%w: %s, assignment %q carves fewer", experimentation.ErrBucketOutOfRange, share, assignment.ID())
	}
	return shareCondition{assignment: assignment, share: share}, nil
}

func (c shareCondition) matches(eval evaluation, record Record) (bool, error) {
	if eval.experiments == nil {
		return false, errors.New("audience: no experimentation service to ask about shares")
	}
	key := record.UID.assignmentKey()
	var (
		got experimentation.BucketNumber
		err error
	)
	// The mode has to reach every call that could record a colouring, or a
	// rehearsal would colour people through this condition while looking
	// innocent everywhere else.
	if eval.mode.IsRehearsal() {
		got, err = eval.experiments.PeekAssign(c.assignment, key)
	} else {
		got, err = eval.experiments.Assign(c.assignment, key)
	}
	if err != nil {
		return false, fmt.Errorf("audience: asking for the share of %q: %w", record.UID, err)
	}
	return got == c.share, nil
}

func (c shareCondition) Describe() string {
	return fmt.Sprintf("assignment %q %s", c.assignment.ID(), c.share)
}

// --- elapsed since an event --------------------------------------------------

type elapsedCondition struct {
	event  string
	window time.Duration
}

// ElapsedSince matches people for whom the named event happened at least the
// given window ago, according to the source.
//
// The window is checked against what the source said it can see. A window
// shorter than that is refused here rather than answered wrongly later.
func ElapsedSince(source Source, event string, window time.Duration) (Condition, error) {
	if source.isZero() {
		return nil, ErrMissingSourceID
	}
	if event == "" {
		return nil, ErrMissingEventName
	}
	if window <= 0 {
		return nil, fmt.Errorf("%w: %s", ErrNonPositiveWindow, window)
	}
	granularity, declared := source.BackflowGranularity()
	if !declared {
		return nil, fmt.Errorf("%w: source %q", ErrGranularityNotDeclared, source.ID())
	}
	if window < granularity {
		return nil, fmt.Errorf("%w: window %s, source %q sees %s",
			ErrWindowShorterThanGranularity, window, source.ID(), granularity)
	}
	return elapsedCondition{event: event, window: window}, nil
}

func (c elapsedCondition) matches(_ evaluation, record Record) (bool, error) {
	age, happened := record.EventAges[c.event]
	if !happened {
		return false, nil
	}
	return age >= c.window, nil
}

func (c elapsedCondition) Describe() string {
	return fmt.Sprintf("%q happened at least %s ago", c.event, c.window)
}
