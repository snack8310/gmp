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

// RunDemoFromEnv requires the same explicit MySQL configuration as the local
// management API. It never logs the DSN or source member values.
func RunDemoFromEnv(ctx context.Context, lookup func(string) (string, bool), logger *slog.Logger) (campaign.Report, error) {
	config, err := management.MySQLConfigFromEnv(lookup)
	if err != nil {
		return campaign.Report{}, err
	}
	return RunDemo(ctx, config, logger)
}

// RunDemo reads the repository-owned synthetic MySQL audience, then records
// executions and first receipts through the application's MySQL runtime.
func RunDemo(ctx context.Context, config management.MySQLConfig, logger *slog.Logger) (campaign.Report, error) {
	if ctx == nil {
		return campaign.Report{}, errors.New("application: demo context is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	store, err := management.OpenMySQLStore(ctx, config, logger)
	if err != nil {
		return campaign.Report{}, err
	}
	defer func() {
		if err := store.Close(); err != nil {
			logger.Error("demo_store_close_failed", "error_type", fmt.Sprintf("%T", err))
		}
	}()

	source, err := store.NewDemoAudienceSource(2, logger)
	if err != nil {
		return campaign.Report{}, err
	}
	sourceDescriptor, err := audience.NewSource(audience.SourceSpec{
		ID: "demo_user_pool", Supply: audience.SupplyPull, Category: audience.CategoryExternal,
	})
	if err != nil {
		return campaign.Report{}, err
	}
	activeLifecycle, err := audience.Attribute("lifecycle", "active")
	if err != nil {
		return campaign.Report{}, err
	}
	definition, err := audience.NewDefinition(audience.DefinitionSpec{
		ID: "demo_constructed_pool", Source: sourceDescriptor.ID(), Conditions: []audience.Condition{activeLifecycle},
	})
	if err != nil {
		return campaign.Report{}, err
	}
	group, err := audience.NewGroup("demo_constructed_group", definition)
	if err != nil {
		return campaign.Report{}, err
	}

	firstAction, err := demoAction("demo_coupon_action")
	if err != nil {
		return campaign.Report{}, err
	}
	nextAction, err := demoAction("demo_reminder_action")
	if err != nil {
		return campaign.Report{}, err
	}
	firstTreatment, err := demoTreatment("demo_coupon_treatment", firstAction)
	if err != nil {
		return campaign.Report{}, err
	}
	nextTreatment, err := demoTreatment("demo_reminder_treatment", nextAction)
	if err != nil {
		return campaign.Report{}, err
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "demo_mysql_flow",
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{firstTreatment.ID()}}},
		Edges:   []campaign.EdgeSpec{{From: firstTreatment.ID(), To: nextTreatment.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess}},
	})
	if err != nil {
		return campaign.Report{}, err
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "demo_mysql_campaign", Priority: campaign.NewPriority(1), Audience: audience.SelectGroup(group),
		Treatments: []campaign.Treatment{firstTreatment, nextTreatment}, Flow: flow,
	})
	if err != nil {
		return campaign.Report{}, err
	}
	runtime, err := NewRuntime(RuntimeSpec{
		Store: store, Source: source, SourceID: sourceDescriptor.ID(),
		Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{
			experimentation.FNV1aV1{}.Version(): experimentation.FNV1aV1{},
		},
		Components: map[experimentation.AssignmentID]experimentation.ComponentState{},
		Deliverers: map[campaign.ActionID]Deliverer{
			firstAction.ID(): demoDeliverer{logger: logger},
			nextAction.ID():  demoDeliverer{logger: logger},
		},
		Events: demoEventPublisher{logger: logger}, MaxAttempts: 1, Logger: logger,
	})
	if err != nil {
		return campaign.Report{}, err
	}
	report, err := runtime.Run(ctx, built, firstTreatment.ID(), "demo_mysql_occurrence_v1", Live())
	if err != nil {
		return report, err
	}
	for _, execution := range report.Executions {
		if err := runtime.RecordReceipt(ctx, built, campaign.Receipt{
			Key: execution.Key, Arrived: true, Outcome: campaign.OutcomeSuccess,
		}); err != nil {
			return report, err
		}
	}
	logger.Info("mysql_demo_completed", "operation", "campaign_run", "result", "success", "execution_count", len(report.Executions), "receipt_count", len(report.Executions))
	return report, nil
}

func demoAction(id string) (campaign.Action, error) {
	return campaign.NewAction(campaign.ActionSpec{
		ID: campaign.ActionID(id), Direction: campaign.DirectionPush,
		Required: []string{"template"}, Outcomes: []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure},
	})
}

func demoTreatment(id string, action campaign.Action) (campaign.Treatment, error) {
	call, err := action.Call(map[string]string{"template": "constructed demo content"})
	if err != nil {
		return campaign.Treatment{}, err
	}
	return campaign.NewTreatment(campaign.TreatmentSpec{ID: campaign.TreatmentID(id), Call: call})
}

type demoDeliverer struct{ logger *slog.Logger }

func (d demoDeliverer) Deliver(ctx context.Context, request campaign.Request) (campaign.Ack, error) {
	if err := ctx.Err(); err != nil {
		return campaign.Ack{}, err
	}
	d.logger.Info("demo_delivery", "operation", "deliver", "result", "accepted", "action_id", request.Call.Action())
	return campaign.Ack{ExternalRef: "constructed_demo_delivery"}, nil
}

type demoEventPublisher struct{ logger *slog.Logger }

func (p demoEventPublisher) Publish(ctx context.Context, event campaign.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.logger.Info("demo_event", "operation", "publish", "result", "accepted", "event_name", event.Name, "action_id", event.Action)
	return nil
}
