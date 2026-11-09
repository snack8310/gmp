package campaign

import (
	"errors"
	"fmt"
	"sort"

	"github.com/snack8310/gmp/gmp-core/audience"
)

var ErrContentionUndecidable = errors.New("campaign: two campaigns rank equally for one slot and nothing decides between them")

type pureApplicant struct {
	campaign  CampaignID
	priority  int
	treatment Treatment
	entry     audience.Group
	routeWhen audience.Group
	rollout   audience.Group
}

// Decide settles a slot contest from the campaign values and audience facts
// supplied by the caller. It does not load configuration or retain state.
func Decide(campaigns []Campaign, slots []Slot, slot SlotID, record audience.Record, facts audience.AssignmentFacts) (Decision, error) {
	decision := Decision{Slot: slot, UID: record.UID}
	if slot == "" {
		return decision, ErrMissingSlotID
	}
	known := false
	for _, candidate := range slots {
		if candidate.ID() == slot {
			known = true
			break
		}
	}
	if !known {
		return decision, fmt.Errorf("%w: %q", ErrUnknownSlot, slot)
	}
	if record.UID == "" {
		return decision, audience.ErrEmptyUID
	}

	var applicants []pureApplicant
	for _, built := range campaigns {
		if built.isZero() {
			return decision, ErrUnbuiltCampaign
		}
		candidate, entry, routeWhen, rollout, losses, err := considerPure(built, slot, record, facts)
		if err != nil {
			return decision, err
		}
		decision.Losses = append(decision.Losses, losses...)
		if candidate == nil {
			continue
		}
		applicants = append(applicants, pureApplicant{
			campaign: built.id, priority: built.priority.Value(), treatment: *candidate,
			entry: entry, routeWhen: routeWhen, rollout: rollout,
		})
	}
	if len(applicants) == 0 {
		return decision, nil
	}
	sort.SliceStable(applicants, func(i, j int) bool { return applicants[i].priority > applicants[j].priority })
	top := applicants[0]
	if len(applicants) > 1 && applicants[1].priority == top.priority {
		return decision, fmt.Errorf("%w: slot %q, campaigns %q and %q both rank %d",
			ErrContentionUndecidable, slot, top.campaign, applicants[1].campaign, top.priority)
	}

	placements, err := top.entry.Definition().Placements(record.UID, facts)
	if err != nil {
		return decision, err
	}
	if top.routeWhen.ID() != "" {
		routed, err := top.routeWhen.Definition().Placements(record.UID, facts)
		if err != nil {
			return decision, err
		}
		placements = mergePlacements(placements, routed)
	}
	if top.rollout.ID() != "" {
		admitted, err := top.rollout.Definition().Placements(record.UID, facts)
		if err != nil {
			return decision, err
		}
		placements = mergePlacements(placements, admitted)
	}
	decision.Won = true
	decision.Award = Award{
		Campaign: top.campaign, Treatment: top.treatment.id,
		Call: top.treatment.call, Placements: placements,
	}
	for _, loser := range applicants[1:] {
		decision.Losses = append(decision.Losses, Loss{
			Campaign: loser.campaign, Treatment: loser.treatment.id, Reason: LossOnPriority,
		})
	}
	return decision, nil
}

func considerPure(c Campaign, slot SlotID, record audience.Record, facts audience.AssignmentFacts) (*Treatment, audience.Group, audience.Group, audience.Group, []Loss, error) {
	wants := false
	for _, wanted := range c.slots() {
		if wanted == slot {
			wants = true
			break
		}
	}
	if !wants {
		return nil, audience.Group{}, audience.Group{}, audience.Group{}, nil, nil
	}

	entryGroup, qualifies, err := c.audience.Match(record, facts)
	if err != nil {
		return nil, audience.Group{}, audience.Group{}, audience.Group{}, nil, fmt.Errorf("campaign %q: checking the entry audience: %w", c.id, err)
	}
	if !qualifies {
		var losses []Loss
		for _, treatment := range c.treatments {
			if treatment.slot == slot {
				losses = append(losses, Loss{Campaign: c.id, Treatment: treatment.id, Reason: LossAudienceDidNotMatch})
			}
		}
		return nil, audience.Group{}, audience.Group{}, audience.Group{}, losses, nil
	}

	route, routed := c.flow.RouteForGroup(entryGroup.ID())
	if !routed || route.Kind() == EntryControl {
		return nil, entryGroup, audience.Group{}, audience.Group{}, nil, nil
	}
	entryTreatments := make(map[TreatmentID]bool, len(route.treatments))
	candidates := make([]Treatment, 0, len(route.treatments))
	for _, id := range route.treatments {
		treatment, held := c.treatment(id)
		if held && treatment.slot == slot {
			entryTreatments[id] = true
			candidates = append(candidates, treatment)
		}
	}
	if len(candidates) == 0 {
		return nil, entryGroup, audience.Group{}, audience.Group{}, nil, nil
	}

	var rolloutGroup audience.Group
	if rollout, limited := c.Rollout(); limited {
		rolloutGroup = rollout
		in, err := rollout.Matches(record, facts)
		if err != nil {
			return nil, audience.Group{}, audience.Group{}, audience.Group{}, nil, fmt.Errorf("campaign %q: checking the rollout: %w", c.id, err)
		}
		if !in {
			var losses []Loss
			for _, treatment := range candidates {
				losses = append(losses, Loss{Campaign: c.id, Treatment: treatment.id, Reason: LossOutsideRollout})
			}
			return nil, entryGroup, audience.Group{}, rolloutGroup, losses, nil
		}
	}

	if table, ordered := c.tables[slot]; ordered {
		treatment, routeWhen, losses, err := routePure(c, table, record, facts, entryTreatments)
		return treatment, entryGroup, routeWhen, rolloutGroup, losses, err
	}
	if len(candidates) > 1 {
		return nil, audience.Group{}, audience.Group{}, audience.Group{}, nil,
			fmt.Errorf("%w: multiple entry candidates on slot %q in campaign %q", ErrSlotContestedWithinCampaign, slot, c.id)
	}
	return &candidates[0], entryGroup, audience.Group{}, rolloutGroup, nil, nil
}

func routePure(c Campaign, table DecisionTable, record audience.Record, facts audience.AssignmentFacts, entries map[TreatmentID]bool) (*Treatment, audience.Group, []Loss, error) {
	held := make(map[TreatmentID]Treatment, len(c.treatments))
	for _, treatment := range c.treatments {
		held[treatment.id] = treatment
	}
	var losses []Loss
	for _, row := range table.rows {
		if row.Then != "" && !entries[row.Then] {
			continue
		}
		in, err := row.When.Matches(record, facts)
		if err != nil {
			return nil, audience.Group{}, nil, fmt.Errorf("campaign %q, table for slot %q: %w", c.id, table.slot, err)
		}
		if !in {
			if row.Then != "" && entries[row.Then] {
				losses = append(losses, Loss{Campaign: c.id, Treatment: row.Then, Reason: LossRoutedElsewhere})
			}
			continue
		}
		if row.Then == "" {
			return nil, audience.Group{}, losses, nil
		}
		treatment := held[row.Then]
		return &treatment, row.When, losses, nil
	}
	if table.fallback.then == "" || !entries[table.fallback.then] {
		return nil, audience.Group{}, losses, nil
	}
	treatment := held[table.fallback.then]
	return &treatment, audience.Group{}, losses, nil
}

// DeliveryIntent is a value-only request to execute one treatment for one UID.
type DeliveryIntent struct {
	Key        IdempotencyKey
	UID        audience.UID
	Campaign   CampaignID
	Treatment  TreatmentID
	Action     ActionID
	Call       Call
	Placements []audience.Placement
}

// ValidateEntry checks the member-independent conditions for starting a
// treatment from an audience and returns the treatment that will be planned.
func ValidateEntry(c Campaign, id TreatmentID) (Treatment, error) {
	if c.isZero() {
		return Treatment{}, ErrUnbuiltCampaign
	}
	treatment, held := c.treatment(id)
	if !held {
		return Treatment{}, fmt.Errorf("%w: %q in campaign %q", ErrNoSuchTreatment, id, c.ID())
	}
	if !c.isEntry(id) {
		return Treatment{}, fmt.Errorf("%w: %q in campaign %q", ErrNotAnEntry, id, c.ID())
	}
	entry, _ := c.flow.RouteForTreatment(id)
	_, selected := c.audience.Group(entry.Group())
	if !selected {
		return Treatment{}, fmt.Errorf("%w: entry group %q in campaign %q", ErrInvalidEntryRoute, entry.Group(), c.ID())
	}
	if treatment.call.Direction() == DirectionPull {
		return Treatment{}, fmt.Errorf("%w: treatment %q runs action %q", ErrPullCannotBeDeliveredInBatch, id, treatment.call.Action())
	}
	return treatment, nil
}

// PlanEntry produces the one delivery implied by an entry route, if the record
// is in that route's audience and rollout.
func PlanEntry(c Campaign, id TreatmentID, record audience.Record, facts audience.AssignmentFacts, occurrence Occurrence) (DeliveryIntent, bool, error) {
	if c.isZero() {
		return DeliveryIntent{}, false, ErrUnbuiltCampaign
	}
	if err := occurrence.validate(); err != nil {
		return DeliveryIntent{}, false, err
	}
	if record.UID == "" {
		return DeliveryIntent{}, false, audience.ErrEmptyUID
	}
	treatment, err := ValidateEntry(c, id)
	if err != nil {
		return DeliveryIntent{}, false, err
	}
	entry, _ := c.flow.RouteForTreatment(id)
	group, _ := c.audience.Group(entry.Group())
	in, err := group.Matches(record, facts)
	if err != nil {
		return DeliveryIntent{}, false, err
	}
	if !in {
		return DeliveryIntent{}, false, nil
	}
	placements, err := group.Definition().Placements(record.UID, facts)
	if err != nil {
		return DeliveryIntent{}, false, err
	}
	if rollout, limited := c.Rollout(); limited {
		admitted, err := rollout.Matches(record, facts)
		if err != nil {
			return DeliveryIntent{}, false, err
		}
		if !admitted {
			return DeliveryIntent{}, false, nil
		}
		rolloutPlacements, err := rollout.Definition().Placements(record.UID, facts)
		if err != nil {
			return DeliveryIntent{}, false, err
		}
		placements = mergePlacements(placements, rolloutPlacements)
	}
	return DeliveryIntent{
		Key: deriveKey(c.ID(), id, record.UID, occurrence), UID: record.UID,
		Campaign: c.ID(), Treatment: id, Action: treatment.call.Action(), Call: treatment.call,
		Placements: append([]audience.Placement(nil), placements...),
	}, true, nil
}

// CompleteDelivery turns an external delivery result into an execution value.
func CompleteDelivery(intent DeliveryIntent, attempts int, ack Ack, succeeded bool) (Execution, error) {
	if attempts <= 0 {
		return Execution{}, fmt.Errorf("%w: %d", ErrNonPositiveAttempts, attempts)
	}
	execution := Execution{
		Key: intent.Key, Campaign: intent.Campaign, Treatment: intent.Treatment, UID: intent.UID,
		Action: intent.Action, Placements: append([]audience.Placement(nil), intent.Placements...), Attempts: attempts,
	}
	if succeeded {
		execution.State = StateDelivered
		execution.ExternalRef = ack.ExternalRef
	} else {
		execution.State = StateFailed
	}
	return execution, nil
}

// ReceiptTransition is the pure state and side-effect intent produced by one
// receipt. The application must atomically accept Execution before emitting
// Event or executing Deliveries.
type ReceiptTransition struct {
	Execution  Execution
	Event      Event
	Deliveries []DeliveryIntent
	Noop       bool
}

// ApplyReceipt converts one receipt into a new execution, event and next-step
// intents without reading storage or executing those effects.
func ApplyReceipt(c Campaign, execution Execution, receipt Receipt) (ReceiptTransition, error) {
	if c.isZero() {
		return ReceiptTransition{}, ErrUnbuiltCampaign
	}
	if err := receipt.validate(); err != nil {
		return ReceiptTransition{}, err
	}
	if execution.Key != receipt.Key {
		return ReceiptTransition{}, fmt.Errorf("%w: %q", ErrNoSuchExecution, receipt.Key)
	}
	if c.ID() != execution.Campaign {
		return ReceiptTransition{}, fmt.Errorf("%w: execution %q belongs to campaign %q", ErrCampaignMismatch, receipt.Key, execution.Campaign)
	}
	treatment, held := c.treatment(execution.Treatment)
	if !held {
		return ReceiptTransition{}, fmt.Errorf("%w: %q in campaign %q", ErrNoSuchTreatment, execution.Treatment, c.ID())
	}
	if err := c.receiptOutcome(treatment, receipt.Outcome); err != nil {
		return ReceiptTransition{}, err
	}
	execution.Placements = append([]audience.Placement(nil), execution.Placements...)
	execution.ReceiptAttributes = cloneAttributes(execution.ReceiptAttributes)
	if execution.HasReceipt() {
		if !execution.MatchesReceipt(receipt) {
			return ReceiptTransition{}, fmt.Errorf("%w: %q", ErrConflictingReceipt, receipt.Key)
		}
		return ReceiptTransition{Execution: execution, Noop: true}, nil
	}

	eventName := NotArrivedEvent(execution.Action)
	execution.State = StateNotArrived
	if receipt.Arrived {
		eventName = ArrivedEvent(execution.Action)
		execution.State = StateArrived
	}
	execution.ReceiptExternalRef = receipt.ExternalRef
	execution.ReceiptAttributes = cloneAttributes(receipt.Attributes)
	execution.Outcome = receipt.Outcome
	event := Event{
		Name: eventName, UID: execution.UID, Key: execution.Key,
		Campaign: execution.Campaign, Treatment: execution.Treatment, Action: execution.Action,
		Placements: append([]audience.Placement(nil), execution.Placements...),
		Attributes: cloneAttributes(receipt.Attributes), Outcome: receipt.Outcome,
	}
	intents, err := receiptEdgeIntents(c, execution, receipt.Outcome)
	if err != nil {
		return ReceiptTransition{}, err
	}
	return ReceiptTransition{Execution: execution, Event: event, Deliveries: intents}, nil
}

func receiptEdgeIntents(c Campaign, upstream Execution, outcome Outcome) ([]DeliveryIntent, error) {
	var intents []DeliveryIntent
	for _, edge := range c.flow.edges {
		if edge.kind != EdgeOnReceipt || edge.from != upstream.Treatment || edge.when != outcome {
			continue
		}
		target, held := c.treatment(edge.to)
		if !held {
			return nil, fmt.Errorf("%w: %q", ErrNoSuchTreatment, edge.to)
		}
		if target.call.Direction() == DirectionPull {
			return nil, fmt.Errorf("%w: treatment %q runs action %q", ErrPullCannotBeDeliveredInBatch, target.id, target.call.Action())
		}
		intents = append(intents, DeliveryIntent{
			Key: deriveKey(c.ID(), target.id, upstream.UID, Occurrence(upstream.Key)),
			UID: upstream.UID, Campaign: c.ID(), Treatment: target.id, Action: target.call.Action(), Call: target.call,
			Placements: append([]audience.Placement(nil), upstream.Placements...),
		})
	}
	return intents, nil
}

func mergePlacements(placements, added []audience.Placement) []audience.Placement {
	for _, candidate := range added {
		found := false
		for _, existing := range placements {
			if existing.Assignment == candidate.Assignment {
				found = true
				break
			}
		}
		if !found {
			placements = append(placements, candidate)
		}
	}
	return placements
}

func cloneAttributes(attributes map[string]string) map[string]string {
	if attributes == nil {
		return nil
	}
	owned := make(map[string]string, len(attributes))
	for key, value := range attributes {
		owned[key] = value
	}
	return owned
}
