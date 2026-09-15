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
)

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
	id       ActionID
	required []string
}

// RegisterAction declares an action and the parameters it needs.
func RegisterAction(id ActionID, required ...string) (Action, error) {
	if id == "" {
		return Action{}, ErrMissingActionID
	}
	owned := make([]string, len(required))
	copy(owned, required)
	sort.Strings(owned)
	for i, name := range owned {
		if name == "" {
			return Action{}, fmt.Errorf("campaign: action %q declares an unnamed parameter at position %d", id, i+1)
		}
	}
	return Action{id: id, required: owned}, nil
}

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
	action ActionID
	params map[string]string
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
	return Call{action: a.id, params: owned}, nil
}

// Action reports which action this call runs.
func (c Call) Action() ActionID { return c.action }

// Param reads one filled-in parameter.
func (c Call) Param(name string) (string, bool) {
	value, ok := c.params[name]
	return value, ok
}

func (c Call) isZero() bool { return c.action == "" }
