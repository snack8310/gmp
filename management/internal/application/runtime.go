package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/experimentation"
	"github.com/snack8310/gmp/management/internal/management"
)

var (
	ErrInvalidRuntimeSpec = errors.New("application: runtime specification is incomplete")
	ErrNoAlgorithm        = errors.New("application: assignment algorithm is not configured")
	ErrSourceMismatch     = errors.New("application: campaign audience does not use the configured source")
	ErrGroupUnavailable   = errors.New("application: audience references an unavailable experiment component or publication generation")
	ErrNoDeliverer        = errors.New("application: no deliverer is configured for this action")
)

type Deliverer interface {
	Deliver(context.Context, campaign.Request) (campaign.Ack, error)
}

type EventPublisher interface {
	Publish(context.Context, campaign.Event) error
}

type RuntimeSpec struct {
	Store       *management.MySQLStore
	Source      *management.MySQLAudienceSource
	SourceID    audience.SourceID
	Algorithms  map[experimentation.AlgorithmVersion]experimentation.Algorithm
	Components  map[experimentation.AssignmentID]experimentation.ComponentState
	Deliverers  map[campaign.ActionID]Deliverer
	Events      EventPublisher
	MaxAttempts int
	Logger      *slog.Logger
}

type Runtime struct {
	store       *management.MySQLStore
	source      *management.MySQLAudienceSource
	sourceID    audience.SourceID
	algorithms  map[experimentation.AlgorithmVersion]experimentation.Algorithm
	components  map[experimentation.AssignmentID]experimentation.ComponentState
	deliverers  map[campaign.ActionID]Deliverer
	events      EventPublisher
	maxAttempts int
	logger      *slog.Logger
}

func NewRuntime(spec RuntimeSpec) (*Runtime, error) {
	if spec.Store == nil || spec.Source == nil || spec.SourceID == "" || spec.Algorithms == nil ||
		spec.Components == nil || spec.Deliverers == nil || spec.Events == nil || spec.MaxAttempts <= 0 {
		return nil, ErrInvalidRuntimeSpec
	}
	if spec.Logger == nil {
		spec.Logger = slog.Default()
	}
	algorithms := make(map[experimentation.AlgorithmVersion]experimentation.Algorithm, len(spec.Algorithms))
	for version, algorithm := range spec.Algorithms {
		if version == "" || algorithm == nil || algorithm.Version() != version {
			return nil, fmt.Errorf("%w: invalid algorithm version mapping", ErrInvalidRuntimeSpec)
		}
		algorithms[version] = algorithm
	}
	components := make(map[experimentation.AssignmentID]experimentation.ComponentState, len(spec.Components))
	for id, state := range spec.Components {
		if id == "" || state.Definition().ID() != id {
			return nil, fmt.Errorf("%w: invalid component mapping", ErrInvalidRuntimeSpec)
		}
		components[id] = state
	}
	deliverers := make(map[campaign.ActionID]Deliverer, len(spec.Deliverers))
	for action, deliverer := range spec.Deliverers {
		if action == "" || deliverer == nil {
			return nil, fmt.Errorf("%w: invalid deliverer mapping", ErrInvalidRuntimeSpec)
		}
		deliverers[action] = deliverer
	}
	return &Runtime{
		store: spec.Store, source: spec.Source, sourceID: spec.SourceID,
		algorithms: algorithms, components: components, deliverers: deliverers,
		events: spec.Events, maxAttempts: spec.MaxAttempts, logger: spec.Logger,
	}, nil
}

func (r *Runtime) Run(ctx context.Context, built campaign.Campaign, treatment campaign.TreatmentID, occurrence campaign.Occurrence, mode RunMode) (campaign.Report, error) {
	report := campaign.Report{Campaign: built.ID(), Treatment: treatment, Rehearsal: mode.IsRehearsal()}
	if built.ID() == "" {
		return report, campaign.ErrUnbuiltCampaign
	}
	if occurrence == "" {
		return report, fmt.Errorf("%w (campaign %q, treatment %q)", campaign.ErrMissingOccurrence, built.ID(), treatment)
	}
	if !mode.IsSet() {
		return report, ErrModeNotSet
	}
	if err := built.Audience().Validate(); err != nil {
		return report, fmt.Errorf("campaign %q: validating entry audience: %w", built.ID(), err)
	}
	entry, err := campaign.ValidateEntry(built, treatment)
	if err != nil {
		return report, fmt.Errorf("campaign %q: validating entry treatment %q: %w", built.ID(), treatment, err)
	}
	if !mode.IsRehearsal() {
		if _, configured := r.deliverers[entry.Call().Action()]; !configured {
			return report, fmt.Errorf("%w: %q", ErrNoDeliverer, entry.Call().Action())
		}
	}
	definitions, err := r.runtimeDefinitions(built)
	if err != nil {
		return report, err
	}
	entryDefinitionCount := len(built.Audience().Groups())
	entryDefinitions := definitions[:entryDefinitionCount]
	rolloutDefinitions := definitions[entryDefinitionCount:]
	var cursor *string
	for {
		page, err := r.source.ReadPage(ctx, cursor)
		if err != nil {
			return report, fmt.Errorf("campaign %q: reading audience page: %w", built.ID(), err)
		}
		for _, member := range page.Members {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			record := audience.Record{UID: member.UID, Attributes: member.Attributes}
			facts, err := r.assignmentFacts(ctx, entryDefinitions, record, mode, nil)
			if err != nil {
				return report, fmt.Errorf("campaign %q: assigning audience member: %w", built.ID(), err)
			}
			_, selected, err := built.Audience().Match(record, facts)
			if err != nil {
				return report, fmt.Errorf("campaign %q: matching entry audience: %w", built.ID(), err)
			}
			if !selected {
				continue
			}
			facts, err = r.assignmentFacts(ctx, rolloutDefinitions, record, mode, facts)
			if err != nil {
				return report, fmt.Errorf("campaign %q: assigning rollout member: %w", built.ID(), err)
			}
			intent, matches, err := campaign.PlanEntry(built, treatment, record, facts, occurrence)
			if err != nil {
				return report, fmt.Errorf("campaign %q, treatment %q: planning delivery: %w", built.ID(), treatment, err)
			}
			if !matches {
				continue
			}
			if mode.IsRehearsal() {
				report.Planned = append(report.Planned, campaign.Plan{
					Key: intent.Key, UID: intent.UID, Action: intent.Action,
					Placements: append([]audience.Placement(nil), intent.Placements...),
				})
				continue
			}
			execution, err := r.deliver(ctx, intent, nil)
			if err != nil {
				return report, err
			}
			report.Executions = append(report.Executions, execution)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = page.NextCursor
	}
	r.logger.Info("application_campaign_run", "operation", "run", "result", "success", "rehearsal", report.Rehearsal, "planned_count", len(report.Planned), "execution_count", len(report.Executions))
	return report, nil
}

func (r *Runtime) RecordReceipt(ctx context.Context, built campaign.Campaign, receipt campaign.Receipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	execution, found, err := r.store.ExecutionLog().LookupContext(ctx, receipt.Key)
	if err != nil {
		return fmt.Errorf("application: looking up receipt execution: %w", err)
	}
	if !found {
		return fmt.Errorf("%w: %q", campaign.ErrNoSuchExecution, receipt.Key)
	}
	transition, err := campaign.ApplyReceipt(built, execution, receipt)
	if err != nil {
		return err
	}
	if _, err := r.runtimeDefinitions(built); err != nil {
		return fmt.Errorf("campaign %q: validating its entry audience before following a receipt: %w", built.ID(), err)
	}
	if transition.Noop {
		return nil
	}
	accepted, err := r.store.ExecutionLog().AcceptReceiptContext(ctx, transition.Execution)
	if err != nil {
		return err
	}
	if !accepted {
		return nil
	}
	if err := r.events.Publish(ctx, transition.Event); err != nil {
		return fmt.Errorf("application: publishing receipt event: %w", err)
	}
	for _, intent := range transition.Deliveries {
		existing, found, err := r.store.ExecutionLog().LookupContext(ctx, intent.Key)
		if err != nil {
			return fmt.Errorf("application: looking up next-step execution: %w", err)
		}
		if found && existing.State != campaign.StateFailed {
			continue
		}
		var prior *campaign.Execution
		if found {
			prior = &existing
		}
		_, err = r.deliver(ctx, intent, prior)
		if err != nil {
			return err
		}
	}
	r.logger.Info("application_receipt", "operation", "record_receipt", "result", "accepted", "delivery_count", len(transition.Deliveries))
	return nil
}

func (r *Runtime) assignmentFacts(ctx context.Context, definitions []audience.Definition, record audience.Record, mode RunMode, initial audience.AssignmentFacts) (audience.AssignmentFacts, error) {
	facts := append(audience.AssignmentFacts(nil), initial...)
	key := record.UID.AssignmentKey()
	for _, definition := range definitions {
		for {
			assignment, needed, _, err := definition.NextMissingAssignment(record, facts)
			if err != nil {
				return nil, err
			}
			if !needed {
				break
			}
			var existing *experimentation.ColoringRecord
			if assignment.Colored() {
				coloring, found, err := r.store.LookupColoring(ctx, assignment.ID(), key)
				if err != nil {
					return nil, fmt.Errorf("looking up assignment coloring: %w", err)
				}
				if found {
					existing = &coloring
				}
			}
			var component *experimentation.ComponentState
			if state, ok := r.components[assignment.ID()]; ok {
				copy := state
				component = &copy
			}
			algorithm, ok := r.algorithms[assignment.Algorithm()]
			whitelisted := false
			if component != nil {
				_, whitelisted = component.WhitelistFor(key)
			}
			if !ok && existing == nil && !whitelisted {
				return nil, fmt.Errorf("%w: %q", ErrNoAlgorithm, assignment.Algorithm())
			}
			result, err := experimentation.ResolveAssignment(assignment, key, algorithm, component, existing)
			if err != nil {
				return nil, err
			}
			if result.NeedsPersist && !mode.IsRehearsal() {
				winner, err := r.store.RecordColoringFirst(ctx, assignment.ID(), key, result.Coloring)
				if err != nil {
					return nil, fmt.Errorf("recording assignment coloring: %w", err)
				}
				result.Bucket = winner.Bucket
			}
			facts = append(facts, audience.AssignmentFact{Definition: assignment, Bucket: result.Bucket})
		}
	}
	return facts, nil
}

func (r *Runtime) deliver(ctx context.Context, intent campaign.DeliveryIntent, prior *campaign.Execution) (campaign.Execution, error) {
	deliverer, ok := r.deliverers[intent.Action]
	if !ok {
		return campaign.Execution{}, fmt.Errorf("%w: %q", ErrNoDeliverer, intent.Action)
	}
	if prior != nil && prior.State != campaign.StateFailed {
		return *prior, nil
	}
	var ack campaign.Ack
	var lastErr error
	attempts := 0
	for attempt := 1; attempt <= r.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return campaign.Execution{}, err
		}
		attempts = attempt
		ack, lastErr = deliverer.Deliver(ctx, campaign.Request{Key: intent.Key, UID: intent.UID, Call: intent.Call})
		if lastErr == nil {
			break
		}
	}
	execution, err := campaign.CompleteDelivery(intent, attempts, ack, lastErr == nil)
	if err != nil {
		return campaign.Execution{}, err
	}
	log := r.store.ExecutionLog()
	if prior == nil {
		err = log.AppendContext(ctx, execution)
	} else {
		err = log.UpdateContext(ctx, execution)
	}
	if err != nil {
		return campaign.Execution{}, fmt.Errorf("application: recording execution: %w", err)
	}
	if execution.State == campaign.StateDelivered {
		event := campaign.Event{
			Name: campaign.DeliveredEvent(execution.Action), UID: execution.UID, Key: execution.Key,
			Campaign: execution.Campaign, Treatment: execution.Treatment, Action: execution.Action,
			Placements: append([]audience.Placement(nil), execution.Placements...), Outcome: execution.Outcome,
		}
		if err := r.events.Publish(ctx, event); err != nil {
			return execution, fmt.Errorf("application: publishing delivery event: %w", err)
		}
	}
	return execution, nil
}

func (r *Runtime) runtimeDefinitions(built campaign.Campaign) ([]audience.Definition, error) {
	var definitions []audience.Definition
	for _, group := range built.Audience().Groups() {
		if err := r.validateGroupAvailability(group); err != nil {
			return nil, fmt.Errorf("campaign %q: validating group %q: %w", built.ID(), group.ID(), err)
		}
		definitions = append(definitions, group.Definition())
	}
	if rollout, limited := built.Rollout(); limited {
		if err := r.validateGroupAvailability(rollout); err != nil {
			return nil, fmt.Errorf("campaign %q: validating rollout %q: %w", built.ID(), rollout.ID(), err)
		}
		definitions = append(definitions, rollout.Definition())
	}
	if len(definitions) == 0 {
		return nil, fmt.Errorf("%w: campaign %q has no audience definition", campaign.ErrMissingCampaignAudience, built.ID())
	}
	for _, definition := range definitions {
		if definition.Source() != r.sourceID {
			return nil, fmt.Errorf("%w: definition %q belongs to %q", ErrSourceMismatch, definition.ID(), definition.Source())
		}
	}
	return definitions, nil
}

func (r *Runtime) validateGroupAvailability(group audience.Group) error {
	if err := group.Validate(); err != nil {
		return err
	}
	for _, assignment := range group.Definition().Assignments() {
		component, found := r.components[assignment.ID()]
		if !found || !component.Running() || !component.Definition().SameRevisionAs(assignment) {
			return fmt.Errorf("%w: group %q, assignment %q", ErrGroupUnavailable, group.ID(), assignment.ID())
		}
	}
	if assignment, generation, bundled := group.BundleAssignment(); bundled {
		component, found := r.components[assignment.ID()]
		if !found || !component.Active(assignment, generation) {
			return fmt.Errorf("%w: bundle group %q, assignment %q, generation %d", ErrGroupUnavailable, group.ID(), assignment.ID(), generation)
		}
	}
	return nil
}
