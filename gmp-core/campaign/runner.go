package campaign

import (
	"errors"
	"fmt"

	"github.com/snack8310/gmp/gmp-core/audience"
)

// Plan is what a rehearsal says would happen, without anything happening.
//
// It is a separate type from Execution on purpose: a rehearsal has no state to
// report, and reusing Execution would mean inventing one or leaving it blank
// for a reader to misread.
type Plan struct {
	Key        IdempotencyKey
	UID        audience.UID
	Action     ActionID
	Placements []audience.Placement
}

// Report is the outcome of one batch run.
type Report struct {
	Campaign  CampaignID
	Treatment TreatmentID
	// Rehearsal says which of the two lists below is filled in.
	Rehearsal  bool
	Planned    []Plan
	Executions []Execution
}

// Runner carries out the push direction: take a treatment, work out who it is
// for, and hand each delivery to the outside world.
//
// It never calls anything itself. Deliverers are ports, and what is registered
// here is what has been wired up -- the same arrangement as sources on the way
// in and actions on the way out.
type Runner struct {
	audiences   *audience.Service
	log         Log
	events      EventSink
	deliverers  map[ActionID]Deliverer
	maxAttempts int
}

// RunnerSpec is the input to building a runner.
type RunnerSpec struct {
	// Audiences answers who a treatment is for.
	Audiences *audience.Service
	// Log keeps the executions.
	Log Log
	// Events is where this platform's own events go. Use DiscardEvents when
	// nothing subscribes -- there is no nil default, because dropped events
	// look exactly like a next step whose conditions match nobody.
	Events EventSink
	// MaxAttempts is how many times one delivery may be tried before it is
	// given up on. There is no default: a silent one would decide, for every
	// downstream at once, how hard to try before someone goes unreached.
	MaxAttempts int
}

// NewRunner builds a runner.
func NewRunner(spec RunnerSpec) (*Runner, error) {
	if spec.Audiences == nil {
		return nil, errors.New("campaign: runner has no audience service")
	}
	if spec.Log == nil {
		return nil, errors.New("campaign: runner has no execution log")
	}
	if spec.Events == nil {
		return nil, ErrNoEventSink
	}
	if spec.MaxAttempts <= 0 {
		return nil, fmt.Errorf("%w: %d", ErrNonPositiveAttempts, spec.MaxAttempts)
	}
	return &Runner{
		audiences:   spec.Audiences,
		log:         spec.Log,
		events:      spec.Events,
		deliverers:  make(map[ActionID]Deliverer),
		maxAttempts: spec.MaxAttempts,
	}, nil
}

// RegisterDeliverer wires an action to whatever carries it out.
func (r *Runner) RegisterDeliverer(action ActionID, deliverer Deliverer) error {
	if action == "" {
		return ErrMissingActionID
	}
	if deliverer == nil {
		return fmt.Errorf("campaign: action %q was given no deliverer", action)
	}
	if _, taken := r.deliverers[action]; taken {
		return fmt.Errorf("%w: %q", ErrDuplicateDeliverer, action)
	}
	r.deliverers[action] = deliverer
	return nil
}

// Run carries out one treatment for everyone it is for.
//
// A rehearsal takes the identical path and hands nothing over, so what it
// reports is what a real run would do rather than a separate calculation that
// might disagree.
func (r *Runner) Run(c Campaign, id TreatmentID, occurrence Occurrence, mode audience.Mode) (Report, error) {
	report := Report{Campaign: c.ID(), Treatment: id, Rehearsal: mode.IsRehearsal()}
	if c.isZero() {
		return report, ErrUnbuiltCampaign
	}
	if err := occurrence.validate(); err != nil {
		return report, fmt.Errorf("%w (campaign %q, treatment %q)", err, c.ID(), id)
	}
	if !mode.IsSet() {
		return report, fmt.Errorf("%w (campaign %q, treatment %q)", audience.ErrModeNotSet, c.ID(), id)
	}

	var treatment Treatment
	var held bool
	for _, candidate := range c.treatments {
		if candidate.id == id {
			treatment, held = candidate, true
			break
		}
	}
	if !held {
		return report, fmt.Errorf("%w: %q in campaign %q", ErrNoSuchTreatment, id, c.ID())
	}
	if treatment.call.Direction() == DirectionPull {
		return report, fmt.Errorf("%w: treatment %q runs action %q",
			ErrPullCannotBeDeliveredInBatch, id, treatment.call.Action())
	}
	deliverer, wired := r.deliverers[treatment.call.Action()]
	if !wired && !mode.IsRehearsal() {
		return report, fmt.Errorf("%w: %q", ErrNoDeliverer, treatment.call.Action())
	}

	people, err := r.audiences.Enumerate(treatment.who, mode)
	if err != nil {
		return report, fmt.Errorf("campaign %q, treatment %q: %w", c.ID(), id, err)
	}
	rollout, limited := c.Rollout()

	for _, uid := range people {
		if limited {
			in, err := r.audiences.Decide(rollout, mode, uid)
			if err != nil {
				return report, fmt.Errorf("campaign %q: checking the rollout for %q: %w", c.ID(), uid, err)
			}
			if !in {
				continue
			}
		}
		// Where the person landed, recorded whether or not the delivery works
		// out: someone the send failed for is still in their arm.
		placements, err := r.audiences.Placements(treatment.who, mode, uid)
		if err != nil {
			return report, fmt.Errorf("campaign %q, treatment %q: %w", c.ID(), id, err)
		}
		// Derived once, before any attempt. Retrying reuses this value rather
		// than asking for another, which is what makes the key survive retries
		// by construction.
		key := deriveKey(c.ID(), id, uid, occurrence)

		if mode.IsRehearsal() {
			report.Planned = append(report.Planned, Plan{
				Key: key, UID: uid, Action: treatment.call.Action(), Placements: placements,
			})
			continue
		}

		execution := Execution{
			Key: key, Campaign: c.ID(), Treatment: id, UID: uid,
			Action: treatment.call.Action(), Placements: placements,
		}
		request := Request{Key: key, UID: uid, Call: treatment.call}
		var ack Ack
		var lastErr error
		for attempt := 1; attempt <= r.maxAttempts; attempt++ {
			execution.Attempts = attempt
			ack, lastErr = deliverer.Deliver(request)
			if lastErr == nil {
				break
			}
		}
		if lastErr != nil {
			execution.State = StateFailed
		} else {
			execution.State = StateDelivered
			execution.ExternalRef = ack.ExternalRef
		}
		if err := r.log.Append(execution); err != nil {
			return report, fmt.Errorf("campaign %q: recording the execution for %q: %w", c.ID(), uid, err)
		}
		// Only a delivery the downstream took becomes an event. A step that
		// failed every attempt must not tell the next one it succeeded.
		if execution.State == StateDelivered {
			if err := r.publish(execution, DeliveredEvent(execution.Action), nil); err != nil {
				return report, err
			}
		}
		report.Executions = append(report.Executions, execution)
	}
	return report, nil
}

// RecordReceipt takes what the downstream eventually says about a delivery.
//
// Taking a delivery and delivering it are two different moments, and for a
// message the gap is routine. The receipt finds its execution by the same key
// the delivery carried; a key nothing was delivered under is refused rather
// than dropped, because dropping it is exactly how an execution ends up
// hanging with nobody able to say what became of it.
func (r *Runner) RecordReceipt(receipt Receipt) error {
	if err := receipt.validate(); err != nil {
		return err
	}
	execution, found, err := r.log.Lookup(receipt.Key)
	if err != nil {
		return fmt.Errorf("campaign: looking up the execution for a receipt: %w", err)
	}
	if !found {
		return fmt.Errorf("%w: %q", ErrNoSuchExecution, receipt.Key)
	}
	name := NotArrivedEvent(execution.Action)
	execution.State = StateNotArrived
	if receipt.Arrived {
		name = ArrivedEvent(execution.Action)
		execution.State = StateArrived
	}
	if receipt.ExternalRef != "" {
		execution.ExternalRef = receipt.ExternalRef
	}
	if err := r.log.Update(execution); err != nil {
		return err
	}
	return r.publish(execution, name, receipt.Attributes)
}

// publish turns an execution into an event other treatments can react to.
//
// The event carries the key, so whatever reacts can trace it back to the
// execution that produced it; and the placement, so a result reached this way
// can still be attributed to an arm.
func (r *Runner) publish(execution Execution, name EventName, attributes map[string]string) error {
	owned := make(map[string]string, len(attributes))
	for key, value := range attributes {
		owned[key] = value
	}
	event := Event{
		Name: name, UID: execution.UID, Key: execution.Key,
		Campaign: execution.Campaign, Treatment: execution.Treatment,
		Action: execution.Action, Placements: execution.Placements,
		Attributes: owned,
	}
	if err := r.events.Publish(event); err != nil {
		return fmt.Errorf("campaign: publishing %q for %q: %w", name, execution.Key, err)
	}
	return nil
}
