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
	// ErrEmptyOutcome is returned when a contract lists a receipt result with
	// no name.
	ErrEmptyOutcome = errors.New("campaign: outcome is empty")
	// ErrDuplicateOutcome is returned when a contract names one receipt result
	// twice.
	ErrDuplicateOutcome = errors.New("campaign: action contract names one outcome twice")
	// ErrIncompleteReceiptContract is returned when a contract names some
	// receipt results but not both success and failure.
	//
	// An edge can only branch on what the contract admits. A contract that
	// names success and never mentions failure makes the failure branch
	// impossible to write, and the missing side then looks like a branch
	// nobody took.
	ErrIncompleteReceiptContract = errors.New("campaign: an action that names receipt outcomes must name both success and failure")
)

// Outcome is one result an action's contract says a receipt may report.
type Outcome string

const (
	// OutcomeSuccess is the contract's success result.
	OutcomeSuccess Outcome = "success"
	// OutcomeFailure is the contract's failure result.
	OutcomeFailure Outcome = "failure"
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
//
// Each action also states which receipt results its contract admits. Success
// and failure are named by the contract, not inferred from whether a delivery
// was accepted. Further results are whatever that contract adds. An empty list
// means this action declares no receipt vocabulary, and a receipt edge may not
// leave a treatment that runs it.
type Action struct {
	id        ActionID
	direction Direction
	required  []string
	outcomes  []Outcome
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
	// Outcomes lists the receipt results this contract defines. Empty means
	// the action declares none. When any are listed, success and failure must
	// both be among them; other names are results this contract adds.
	Outcomes []Outcome
}

// RegisterAction declares an action, which way it talks, and what it needs.
func RegisterAction(spec ActionSpec) (Action, error) {
	if spec.ID == "" {
		return Action{}, ErrMissingActionID
	}
	if err := spec.Direction.validate(); err != nil {
		return Action{}, fmt.Errorf("%w (action %q)", err, spec.ID)
	}
	outcomes, err := normalizeOutcomes(spec.ID, spec.Outcomes)
	if err != nil {
		return Action{}, err
	}
	owned := make([]string, len(spec.Required))
	copy(owned, spec.Required)
	sort.Strings(owned)
	for i, name := range owned {
		if name == "" {
			return Action{}, fmt.Errorf("campaign: action %q declares an unnamed parameter at position %d", spec.ID, i+1)
		}
	}
	return Action{id: spec.ID, direction: spec.Direction, required: owned, outcomes: outcomes}, nil
}

func normalizeOutcomes(id ActionID, outcomes []Outcome) ([]Outcome, error) {
	if len(outcomes) == 0 {
		return nil, nil
	}
	owned := make([]Outcome, len(outcomes))
	copy(owned, outcomes)
	seen := make(map[Outcome]bool, len(owned))
	var success, failure bool
	for _, outcome := range owned {
		if outcome == "" {
			return nil, fmt.Errorf("%w: action %q", ErrEmptyOutcome, id)
		}
		if seen[outcome] {
			return nil, fmt.Errorf("%w: %q on action %q", ErrDuplicateOutcome, outcome, id)
		}
		seen[outcome] = true
		switch outcome {
		case OutcomeSuccess:
			success = true
		case OutcomeFailure:
			failure = true
		}
	}
	if !success || !failure {
		return nil, fmt.Errorf("%w: action %q", ErrIncompleteReceiptContract, id)
	}
	return owned, nil
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

// Outcomes lists the receipt results this contract defines.
func (a Action) Outcomes() []Outcome {
	out := make([]Outcome, len(a.outcomes))
	copy(out, a.outcomes)
	return out
}

func (a Action) isZero() bool { return a.id == "" }

// Call is one action with its parameters filled in: what a treatment produces
// and what an execution side would later carry out.
type Call struct {
	action    ActionID
	direction Direction
	params    map[string]string
	outcomes  []Outcome
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
	outcomes := make([]Outcome, len(a.outcomes))
	copy(outcomes, a.outcomes)
	return Call{action: a.id, direction: a.direction, params: owned, outcomes: outcomes}, nil
}

// Outcomes lists the receipt results the action behind this call declares.
func (c Call) Outcomes() []Outcome {
	out := make([]Outcome, len(c.outcomes))
	copy(out, c.outcomes)
	return out
}

// Declares reports whether this call's action contract names the outcome.
func (c Call) Declares(outcome Outcome) bool {
	for _, named := range c.outcomes {
		if named == outcome {
			return true
		}
	}
	return false
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
