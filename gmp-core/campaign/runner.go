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

// Run carries out one entry treatment for everyone it is for.
//
// A treatment a receipt edge points at is not an entry: enumerating it would
// send to its audience as well as to the person on the receipt. A rehearsal
// takes the identical path and hands nothing over, so what it reports is what
// a real run would do rather than a separate calculation that might disagree.
// Rehearsing a receipt edge is refused for the same reason: the target
// audience is not a preview of who the edge would reach.
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
	if !c.isEntry(id) {
		return report, fmt.Errorf("%w: %q in campaign %q", ErrNotAnEntry, id, c.ID())
	}
	if treatment.call.Direction() == DirectionPull {
		return report, fmt.Errorf("%w: treatment %q runs action %q",
			ErrPullCannotBeDeliveredInBatch, id, treatment.call.Action())
	}
	deliverer, wired := r.deliverers[treatment.call.Action()]
	if !wired && !mode.IsRehearsal() {
		return report, fmt.Errorf("%w: %q", ErrNoDeliverer, treatment.call.Action())
	}

	people, err := r.audiences.EnumerateGroup(treatment.who, mode)
	if err != nil {
		return report, fmt.Errorf("campaign %q, treatment %q: %w", c.ID(), id, err)
	}
	rollout, limited := c.Rollout()

	for _, uid := range people {
		if limited {
			in, err := r.audiences.DecideGroup(rollout, mode, uid)
			if err != nil {
				return report, fmt.Errorf("campaign %q: checking the rollout for %q: %w", c.ID(), uid, err)
			}
			if !in {
				continue
			}
		}
		// Where the person landed, recorded whether or not the delivery works
		// out: someone the send failed for is still in their arm.
		placements, err := r.audiences.GroupPlacements(treatment.who, mode, uid)
		if err != nil {
			return report, fmt.Errorf("campaign %q, treatment %q: %w", c.ID(), id, err)
		}
		if limited {
			admitted, err := r.audiences.GroupPlacements(rollout, mode, uid)
			if err != nil {
				return report, fmt.Errorf("campaign %q: recording rollout for %q: %w", c.ID(), uid, err)
			}
			placements = mergePlacements(placements, admitted)
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

// RecordReceipt takes what the downstream eventually says about a delivery,
// then follows any receipt edge whose outcome matches.
//
// The campaign is the one that owns the execution: its flow is what decides
// the next treatment. Taking a delivery and delivering it are two different
// moments, and for a message the gap is routine. The receipt finds its
// execution by the same key the delivery carried; a key nothing was delivered
// under is refused rather than dropped, because dropping it is exactly how an
// execution ends up hanging with nobody able to say what became of it.
//
// A matching edge runs the target for the person on this execution only, and
// only one hop. The target's own receipt, if it has edges, waits until that
// receipt is recorded. Accepting the delivery is not that receipt: the
// action's contract says whether those are the same result, and this method
// does not guess.
func (r *Runner) RecordReceipt(c Campaign, receipt Receipt) error {
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
	if c.ID() != execution.Campaign {
		return fmt.Errorf("%w: execution %q belongs to campaign %q", ErrCampaignMismatch, receipt.Key, execution.Campaign)
	}
	treatment, held := c.treatment(execution.Treatment)
	if !held {
		return fmt.Errorf("%w: %q in campaign %q", ErrNoSuchTreatment, execution.Treatment, c.ID())
	}
	if err := c.receiptOutcome(treatment, receipt.Outcome); err != nil {
		return err
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
	execution.Outcome = receipt.Outcome
	if err := r.log.Update(execution); err != nil {
		return err
	}
	if err := r.publish(execution, name, receipt.Attributes); err != nil {
		return err
	}
	return r.follow(c, execution, receipt.Outcome)
}

// follow runs every receipt edge that leaves the upstream treatment on this
// outcome.
//
// The target audience is not consulted. The person is the one the receipt is
// about; filtering them through the target's audience would drop them whenever
// that audience was not rebuilt to name them, which is the failure an edge
// exists to avoid. The rollout is not consulted again either: they already
// entered, and applying it here would shrink one arm of a running flow.
func (r *Runner) follow(c Campaign, upstream Execution, outcome Outcome) error {
	if c.flow.id == "" || outcome == "" {
		return nil
	}
	if source, ok := c.treatment(upstream.Treatment); ok {
		if err := r.audiences.ValidateGroup(source.who); err != nil {
			return err
		}
	}
	for _, edge := range c.flow.edges {
		if edge.kind != EdgeOnReceipt || edge.from != upstream.Treatment || edge.when != outcome {
			continue
		}
		target, held := c.treatment(edge.to)
		if !held {
			return fmt.Errorf("%w: %q", ErrNoSuchTreatment, edge.to)
		}
		if err := r.deliverFromEdge(c, target, upstream); err != nil {
			return err
		}
	}
	return nil
}

// deliverFromEdge carries out one edge step for one person.
//
// The occurrence is the upstream execution key, so a repeated receipt derives
// the same downstream key, and a later entry — a new upstream key — is a new
// downstream delivery. An execution already recorded as anything but a total
// failure is left alone. A failure is tried again under that same key.
func (r *Runner) deliverFromEdge(c Campaign, target Treatment, upstream Execution) error {
	if err := r.audiences.ValidateGroup(target.who); err != nil {
		return err
	}
	if target.call.Direction() == DirectionPull {
		return fmt.Errorf("%w: treatment %q runs action %q",
			ErrPullCannotBeDeliveredInBatch, target.id, target.call.Action())
	}
	deliverer, wired := r.deliverers[target.call.Action()]
	if !wired {
		return fmt.Errorf("%w: %q", ErrNoDeliverer, target.call.Action())
	}
	occurrence := Occurrence(upstream.Key)
	key := deriveKey(c.ID(), target.id, upstream.UID, occurrence)
	existing, found, err := r.log.Lookup(key)
	if err != nil {
		return err
	}
	if found && existing.State != StateFailed {
		return nil
	}
	execution := Execution{
		Key: key, Campaign: c.ID(), Treatment: target.id, UID: upstream.UID,
		Action:     target.call.Action(),
		Placements: append([]audience.Placement(nil), upstream.Placements...),
	}
	request := Request{Key: key, UID: upstream.UID, Call: target.call}
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
	if found {
		err = r.log.Update(execution)
	} else {
		err = r.log.Append(execution)
	}
	if err != nil {
		return fmt.Errorf("campaign %q: recording the execution for %q: %w", c.ID(), upstream.UID, err)
	}
	if execution.State == StateDelivered {
		if err := r.publish(execution, DeliveredEvent(execution.Action), nil); err != nil {
			return err
		}
	}
	return nil
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
		Attributes: owned, Outcome: execution.Outcome,
	}
	if err := r.events.Publish(event); err != nil {
		return fmt.Errorf("campaign: publishing %q for %q: %w", name, execution.Key, err)
	}
	return nil
}
