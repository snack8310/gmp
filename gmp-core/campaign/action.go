package campaign

import (
	"errors"
	"fmt"
	"sort"
)

var (
	// ErrMissingActionID is returned when an action has no identity.
	ErrMissingActionID = errors.New("campaign: action id is empty")
	// ErrMissingActionParameter is returned when a call leaves out something
	// the action said it needs.
	ErrMissingActionParameter = errors.New("campaign: action call is missing a required parameter")
	// ErrUnknownActionParameter is returned when a call carries something the
	// action never asked for.
	ErrUnknownActionParameter = errors.New("campaign: action call carries a parameter the action does not take")
	// ErrDuplicateAction is returned when one action id is registered twice.
	ErrDuplicateAction = errors.New("campaign: action registered twice")
	// ErrUnknownDirection is returned when an action does not say which way it
	// talks to the outside world.
	ErrUnknownDirection = errors.New("campaign: action direction is not one of the known ways")
)

// Direction says who calls whom.
//
// It is not decoration. We call a messaging or entitlement system, so a timeout
// can be retried. A slot calls us and we answer with what to show, so there is
// nothing to retry -- the person has already gone -- and there is no such thing
// as delivery succeeding.
type Direction string

const (
	// DirectionPush is us calling an outside system.
	DirectionPush Direction = "push"
	// DirectionPull is an outside system calling us.
	DirectionPull Direction = "pull"
)

func (d Direction) validate() error {
	switch d {
	case DirectionPush, DirectionPull:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrUnknownDirection, string(d))
	}
}

// ActionID identifies a kind of thing that can be done: show an image, send a
// message, grant an entitlement.
type ActionID string

// Action is a capability engineering has wired up. Which actions exist is not
// a field an operator fills in freely; it mirrors what has been connected, the
// same way sources do on the way in.
//
// Each action states its own parameters. There is deliberately no shared notion
// of "content" above them: a coupon id belongs to the coupon system and is only
// referenced here, and wrapping it in a platform-side concept would mean this
// platform hurts whenever that system changes.
type Action struct {
	id        ActionID
	direction Direction
	required  []string
}

// ActionSpec is the input to registering an action.
type ActionSpec struct {
	// ID identifies the action.
	ID ActionID
	// Direction says who calls whom. There is no default: an action that
	// forgot to say would otherwise be treated as retryable, and retrying
	// something nobody is waiting for any more is not a harmless mistake.
	Direction Direction
	// Required lists the parameters this action needs.
	Required []string
}

// RegisterAction declares an action, which way it talks, and what it needs.
func RegisterAction(spec ActionSpec) (Action, error) {
	if spec.ID == "" {
		return Action{}, ErrMissingActionID
	}
	if err := spec.Direction.validate(); err != nil {
		return Action{}, fmt.Errorf("%w (action %q)", err, spec.ID)
	}
	owned := make([]string, len(spec.Required))
	copy(owned, spec.Required)
	sort.Strings(owned)
	for i, name := range owned {
		if name == "" {
			return Action{}, fmt.Errorf("campaign: action %q declares an unnamed parameter at position %d", spec.ID, i+1)
		}
	}
	return Action{id: spec.ID, direction: spec.Direction, required: owned}, nil
}

// Direction reports which way this action talks to the outside world.
func (a Action) Direction() Direction { return a.direction }

// ID reports which action this is.
func (a Action) ID() ActionID { return a.id }

// Required lists the parameters this action needs.
func (a Action) Required() []string {
	out := make([]string, len(a.required))
	copy(out, a.required)
	return out
}

func (a Action) isZero() bool { return a.id == "" }

// Call is one action with its parameters filled in: what a treatment produces
// and what an execution side would later carry out.
type Call struct {
	action    ActionID
	direction Direction
	params    map[string]string
}

// Call fills this action's parameters in. Anything missing or unasked-for is
// refused here rather than discovered by whoever eventually runs it.
func (a Action) Call(params map[string]string) (Call, error) {
	if a.isZero() {
		return Call{}, ErrMissingActionID
	}
	for _, name := range a.required {
		if _, present := params[name]; !present {
			return Call{}, fmt.Errorf("%w: action %q needs %q", ErrMissingActionParameter, a.id, name)
		}
	}
	allowed := make(map[string]bool, len(a.required))
	for _, name := range a.required {
		allowed[name] = true
	}
	owned := make(map[string]string, len(params))
	for name, value := range params {
		if !allowed[name] {
			return Call{}, fmt.Errorf("%w: action %q, parameter %q", ErrUnknownActionParameter, a.id, name)
		}
		owned[name] = value
	}
	return Call{action: a.id, direction: a.direction, params: owned}, nil
}

// Action reports which action this call runs.
func (c Call) Action() ActionID { return c.action }

// Direction reports which way the action behind this call talks.
func (c Call) Direction() Direction { return c.direction }

// Param reads one filled-in parameter.
func (c Call) Param(name string) (string, bool) {
	value, ok := c.params[name]
	return value, ok
}

func (c Call) isZero() bool { return c.action == "" }
