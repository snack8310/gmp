package application_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/gmp-core/experimentation"
	"github.com/snack8310/gmp/management/internal/application"
	"github.com/snack8310/gmp/management/internal/management"
)

func TestRuntimeConcurrentReceiptEmitsAndFollowsOnce(t *testing.T) {
	config := isolatedRuntimeMySQLConfig(t)
	logger := runtimeTestLogger()
	stores, sources := openRuntimeStores(t, config, logger)
	built, firstAction, nextAction := receiptCampaign(t)
	placements := []audience.Placement{{Assignment: "receipt-assignment", Share: 2}}
	initial := campaign.Execution{
		Key: "runtime-execution-key", Campaign: built.ID(), Treatment: "runtime-first-treatment",
		UID: "uid_synthetic_receipt", Action: firstAction.ID(), Placements: placements,
		State: campaign.StateDelivered, Attempts: 1,
	}
	if err := stores[0].ExecutionLog().Append(initial); err != nil {
		t.Fatalf("appending source execution: %v", err)
	}

	events := &runtimeEventSpy{}
	deliverer := &runtimeDelivererSpy{}
	runtimes := make([]*application.Runtime, len(stores))
	for index := range stores {
		runtime, err := application.NewRuntime(application.RuntimeSpec{
			Store: stores[index], Source: sources[index], SourceID: "runtime-receipt-source",
			Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{
				"fnv1a-v1": experimentation.FNV1aV1{},
			},
			Components: map[experimentation.AssignmentID]experimentation.ComponentState{},
			Deliverers: map[campaign.ActionID]application.Deliverer{nextAction.ID(): deliverer},
			Events:     events, MaxAttempts: 1, Logger: logger,
		})
		if err != nil {
			t.Fatalf("building runtime %d: %v", index, err)
		}
		runtimes[index] = runtime
	}

	receipt := campaign.Receipt{Key: initial.Key, Arrived: true, Outcome: campaign.OutcomeSuccess}
	start := make(chan struct{})
	results := make(chan error, len(runtimes))
	for _, runtime := range runtimes {
		go func(runtime *application.Runtime) {
			<-start
			results <- runtime.RecordReceipt(context.Background(), built, receipt)
		}(runtime)
	}
	close(start)
	for range runtimes {
		if err := <-results; err != nil {
			t.Fatalf("recording concurrent receipt: %v", err)
		}
	}

	wantTransition, err := campaign.ApplyReceipt(built, initial, receipt)
	if err != nil {
		t.Fatalf("calculating expected receipt transition: %v", err)
	}
	if got := events.count(campaign.ArrivedEvent(firstAction.ID())); got != 1 {
		t.Fatalf("arrival events = %d, want one", got)
	}
	if got := events.count(campaign.DeliveredEvent(nextAction.ID())); got != 1 {
		t.Fatalf("next-step delivery events = %d, want one", got)
	}
	calls := deliverer.calls()
	if len(calls) != 1 {
		t.Fatalf("next-step calls = %d, want one", len(calls))
	}
	if calls[0].Key != wantTransition.Deliveries[0].Key || calls[0].UID != initial.UID {
		t.Fatalf("next-step request = %#v, want key %q and UID %q", calls[0], wantTransition.Deliveries[0].Key, initial.UID)
	}
	stored, found, err := stores[1].ExecutionLog().LookupContext(context.Background(), wantTransition.Deliveries[0].Key)
	if err != nil || !found || stored.UID != initial.UID || !reflect.DeepEqual(stored.Placements, placements) {
		t.Fatalf("next-step execution = %#v (found=%v err=%v), want original UID and placements", stored, found, err)
	}
	conflict := receipt
	conflict.Arrived = false
	conflict.Outcome = campaign.OutcomeFailure
	if err := runtimes[0].RecordReceipt(context.Background(), built, conflict); !errors.Is(err, campaign.ErrConflictingReceipt) {
		t.Fatalf("conflicting receipt error = %v, want ErrConflictingReceipt", err)
	}
	if got := events.count(campaign.ArrivedEvent(firstAction.ID())); got != 1 {
		t.Fatalf("conflicting receipt changed arrival event count to %d", got)
	}
	if got := len(deliverer.calls()); got != 1 {
		t.Fatalf("conflicting receipt changed downstream call count to %d", got)
	}
	if err := stores[0].Close(); err != nil {
		t.Fatalf("closing store before restart read: %v", err)
	}
	reopened, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("reopening MySQL store: %v", err)
	}
	defer reopened.Close()
	persisted, found, err := reopened.ExecutionLog().LookupContext(context.Background(), initial.Key)
	if err != nil || !found || persisted.State != campaign.StateArrived || persisted.Outcome != campaign.OutcomeSuccess {
		t.Fatalf("receipt after store restart = %#v (found=%v err=%v)", persisted, found, err)
	}
}

func TestRuntimeRejectsReceiptWhenPublishedBundleUnavailable(t *testing.T) {
	built, firstAction, nextAction, active := receiptBundleCampaign(t)
	stopped, err := active.Stop()
	if err != nil {
		t.Fatalf("stopping component: %v", err)
	}
	revisedSplit, err := experimentation.NewSplit(100, []int{50, 50})
	if err != nil {
		t.Fatal(err)
	}
	revisedDefinition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: "runtime-receipt-assignment", Split: revisedSplit, Salt: "revised-runtime-salt",
		Algorithm: "fnv1a-v1", Colored: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	revised, err := stopped.Revise(revisedDefinition)
	if err != nil {
		t.Fatalf("revising component: %v", err)
	}
	restarted, err := stopped.Start()
	if err != nil {
		t.Fatalf("restarting component: %v", err)
	}
	componentID := experimentation.AssignmentID("runtime-receipt-assignment")
	for _, testCase := range []struct {
		name       string
		components map[experimentation.AssignmentID]experimentation.ComponentState
	}{
		{name: "stopped", components: map[experimentation.AssignmentID]experimentation.ComponentState{componentID: stopped}},
		{name: "revised", components: map[experimentation.AssignmentID]experimentation.ComponentState{componentID: revised}},
		{name: "restarted at a new generation", components: map[experimentation.AssignmentID]experimentation.ComponentState{componentID: restarted}},
		{name: "missing", components: map[experimentation.AssignmentID]experimentation.ComponentState{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config := isolatedRuntimeMySQLConfig(t)
			logger := runtimeTestLogger()
			stores, sources := openRuntimeStores(t, config, logger)
			initial := campaign.Execution{
				Key: "unavailable-bundle-execution", Campaign: built.ID(), Treatment: "runtime-first-treatment",
				UID: "uid_unavailable_bundle", Action: firstAction.ID(), State: campaign.StateDelivered, Attempts: 1,
			}
			if err := stores[0].ExecutionLog().Append(initial); err != nil {
				t.Fatalf("appending source execution: %v", err)
			}
			events := &runtimeEventSpy{}
			deliverer := &runtimeDelivererSpy{}
			runtime, err := application.NewRuntime(application.RuntimeSpec{
				Store: stores[0], Source: sources[0], SourceID: "runtime-receipt-source",
				Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
				Components: testCase.components,
				Deliverers: map[campaign.ActionID]application.Deliverer{nextAction.ID(): deliverer},
				Events:     events, MaxAttempts: 1, Logger: logger,
			})
			if err != nil {
				t.Fatalf("building runtime: %v", err)
			}
			receipt := campaign.Receipt{Key: initial.Key, Arrived: true, Outcome: campaign.OutcomeSuccess}
			if err := runtime.RecordReceipt(context.Background(), built, receipt); !errors.Is(err, application.ErrGroupUnavailable) {
				t.Fatalf("receipt for %s bundle error = %v, want ErrGroupUnavailable", testCase.name, err)
			}
			stored, found, err := stores[0].ExecutionLog().LookupContext(context.Background(), initial.Key)
			if err != nil || !found || stored.HasReceipt() || stored.State != campaign.StateDelivered {
				t.Fatalf("%s-bundle receipt mutated execution = %#v (found=%v err=%v)", testCase.name, stored, found, err)
			}
			if events.count(campaign.ArrivedEvent(firstAction.ID())) != 0 || len(deliverer.calls()) != 0 {
				t.Fatalf("%s bundle emitted arrival=%d and downstream calls=%d, want neither", testCase.name, events.count(campaign.ArrivedEvent(firstAction.ID())), len(deliverer.calls()))
			}
		})
	}
}

func TestRuntimeRejectsReceiptWhenOrdinaryShareComponentUnavailable(t *testing.T) {
	for _, conditionKind := range []string{"share", "share-at-most"} {
		t.Run(conditionKind, func(t *testing.T) {
			assignment := runtimeAssignment(t, "ordinary-"+conditionKind+"-assignment", false)
			bucket, err := experimentation.NewBucketNumber(2)
			if err != nil {
				t.Fatal(err)
			}
			var condition audience.Condition
			if conditionKind == "share" {
				condition, err = audience.Share(assignment, bucket)
			} else {
				condition, err = audience.ShareAtMost(assignment, bucket)
			}
			if err != nil {
				t.Fatalf("building %s condition: %v", conditionKind, err)
			}
			group := runtimeGroup(t, "ordinary-"+conditionKind+"-group", "runtime-receipt-source", condition)
			built, firstAction, nextAction := receiptCampaignForGroup(t, group)
			component, err := experimentation.NewComponentState(assignment)
			if err != nil {
				t.Fatal(err)
			}
			active, err := component.Start()
			if err != nil {
				t.Fatal(err)
			}
			stopped, err := active.Stop()
			if err != nil {
				t.Fatal(err)
			}
			// Build a distinct revision under the same component identity.
			revisedDefinition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
				ID: assignment.ID(), Split: assignment.Split(), Salt: "revised-ordinary-runtime-salt",
				Algorithm: assignment.Algorithm(), Colored: assignment.Colored(),
			})
			if err != nil {
				t.Fatal(err)
			}
			revised, err := stopped.Revise(revisedDefinition)
			if err != nil {
				t.Fatal(err)
			}
			revised, err = revised.Start()
			if err != nil {
				t.Fatal(err)
			}

			for _, lifecycle := range []struct {
				name       string
				components map[experimentation.AssignmentID]experimentation.ComponentState
			}{
				{name: "missing", components: map[experimentation.AssignmentID]experimentation.ComponentState{}},
				{name: "stopped", components: map[experimentation.AssignmentID]experimentation.ComponentState{assignment.ID(): stopped}},
				{name: "revised", components: map[experimentation.AssignmentID]experimentation.ComponentState{assignment.ID(): revised}},
			} {
				t.Run(lifecycle.name, func(t *testing.T) {
					config := isolatedRuntimeMySQLConfig(t)
					logger := runtimeTestLogger()
					stores, sources := openRuntimeStores(t, config, logger)
					initial := campaign.Execution{
						Key: campaign.IdempotencyKey("ordinary-unavailable-" + lifecycle.name), Campaign: built.ID(),
						Treatment: "runtime-first-treatment", UID: "uid_ordinary_unavailable",
						Action: firstAction.ID(), State: campaign.StateDelivered, Attempts: 1,
					}
					if err := stores[0].ExecutionLog().Append(initial); err != nil {
						t.Fatalf("appending source execution: %v", err)
					}
					events := &runtimeEventSpy{}
					deliverer := &runtimeDelivererSpy{}
					runtime, err := application.NewRuntime(application.RuntimeSpec{
						Store: stores[0], Source: sources[0], SourceID: "runtime-receipt-source",
						Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
						Components: lifecycle.components,
						Deliverers: map[campaign.ActionID]application.Deliverer{nextAction.ID(): deliverer},
						Events:     events, MaxAttempts: 1, Logger: logger,
					})
					if err != nil {
						t.Fatalf("building runtime: %v", err)
					}
					receipt := campaign.Receipt{Key: initial.Key, Arrived: true, Outcome: campaign.OutcomeSuccess}
					if err := runtime.RecordReceipt(context.Background(), built, receipt); !errors.Is(err, application.ErrGroupUnavailable) {
						t.Fatalf("receipt error = %v, want ErrGroupUnavailable", err)
					}
					stored, found, err := stores[0].ExecutionLog().LookupContext(context.Background(), initial.Key)
					if err != nil || !found || stored.HasReceipt() || stored.State != campaign.StateDelivered {
						t.Fatalf("rejected receipt mutated execution = %#v (found=%v err=%v)", stored, found, err)
					}
					if len(events.events) != 0 || len(deliverer.calls()) != 0 {
						t.Fatalf("rejected receipt emitted %d events and made %d deliveries", len(events.events), len(deliverer.calls()))
					}
				})
			}
		})
	}
}

func TestRuntimeRejectsUnavailableOrdinaryShareBeforeColoring(t *testing.T) {
	assignment := runtimeAssignment(t, "runtime-unavailable-run-assignment", true)
	first, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	condition, err := audience.Share(assignment, first)
	if err != nil {
		t.Fatal(err)
	}
	group := runtimeGroup(t, "runtime-unavailable-run-group", "runtime-demo-source", condition)
	built, treatment, action := runtimeEntryCampaign(t, "runtime-unavailable-run", group, audience.Group{})
	component, err := experimentation.NewComponentState(assignment)
	if err != nil {
		t.Fatal(err)
	}
	active, err := component.Start()
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := active.Stop()
	if err != nil {
		t.Fatal(err)
	}
	revisedDefinition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: assignment.ID(), Split: assignment.Split(), Salt: "revised-runtime-run-salt",
		Algorithm: assignment.Algorithm(), Colored: assignment.Colored(),
	})
	if err != nil {
		t.Fatal(err)
	}
	revised, err := stopped.Revise(revisedDefinition)
	if err != nil {
		t.Fatal(err)
	}
	revised, err = revised.Start()
	if err != nil {
		t.Fatal(err)
	}
	for _, lifecycle := range []struct {
		name       string
		components map[experimentation.AssignmentID]experimentation.ComponentState
	}{
		{name: "missing", components: map[experimentation.AssignmentID]experimentation.ComponentState{}},
		{name: "stopped", components: map[experimentation.AssignmentID]experimentation.ComponentState{assignment.ID(): stopped}},
		{name: "revised", components: map[experimentation.AssignmentID]experimentation.ComponentState{assignment.ID(): revised}},
	} {
		t.Run(lifecycle.name, func(t *testing.T) {
			config := isolatedRuntimeMySQLConfig(t)
			logger := runtimeTestLogger()
			stores, sources := openRuntimeStores(t, config, logger)
			runtime, err := application.NewRuntime(application.RuntimeSpec{
				Store: stores[0], Source: sources[0], SourceID: "runtime-demo-source",
				Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
				Components: lifecycle.components,
				Deliverers: map[campaign.ActionID]application.Deliverer{action.ID(): &runtimeDelivererSpy{}},
				Events:     &runtimeEventSpy{}, MaxAttempts: 1, Logger: logger,
			})
			if err != nil {
				t.Fatalf("building runtime: %v", err)
			}
			if _, err := runtime.Run(context.Background(), built, treatment.ID(), "unavailable-share", application.Live()); !errors.Is(err, application.ErrGroupUnavailable) {
				t.Fatalf("run error = %v, want ErrGroupUnavailable", err)
			}
			if _, found, err := stores[0].LookupColoring(context.Background(), assignment.ID(), "uid_demo_001"); err != nil || found {
				t.Fatalf("unavailable component produced coloring %#v (found=%v err=%v)", assignment, found, err)
			}
		})
	}
}

func TestRuntimeRejectsRestartedRolloutBundleBeforeReadingMembers(t *testing.T) {
	assignment := runtimeAssignment(t, "runtime-rollout-assignment", false)
	first, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	firstCondition, err := audience.Share(assignment, first)
	if err != nil {
		t.Fatal(err)
	}
	secondCondition, err := audience.Share(assignment, second)
	if err != nil {
		t.Fatal(err)
	}
	firstDefinition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "runtime-rollout-first", Source: "runtime-demo-source", Conditions: []audience.Condition{firstCondition}})
	if err != nil {
		t.Fatal(err)
	}
	secondDefinition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "runtime-rollout-second", Source: "runtime-demo-source", Conditions: []audience.Condition{secondCondition}})
	if err != nil {
		t.Fatal(err)
	}
	component, err := experimentation.NewComponentState(assignment)
	if err != nil {
		t.Fatal(err)
	}
	active, err := component.Start()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "runtime-rollout-bundle", Source: "runtime-demo-source", Assignment: assignment,
		Arms: []audience.BundleArm{
			{ID: "runtime-rollout-arm-one", Share: first, Definition: firstDefinition},
			{ID: "runtime-rollout-arm-two", Share: second, Definition: secondDefinition},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err = bundle.Publish(active)
	if err != nil {
		t.Fatal(err)
	}
	rollout, found := bundle.Group(first)
	if !found {
		t.Fatal("published bundle is missing the rollout arm")
	}
	entry := runtimeGroup(t, "runtime-rollout-entry", "runtime-demo-source")
	built, treatment, action := runtimeEntryCampaign(t, "runtime-rollout-campaign", entry, rollout)
	stopped, err := active.Stop()
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := stopped.Start()
	if err != nil {
		t.Fatal(err)
	}
	config := isolatedRuntimeMySQLConfig(t)
	logger := runtimeTestLogger()
	stores, sources := openRuntimeStores(t, config, logger)
	runtime, err := application.NewRuntime(application.RuntimeSpec{
		Store: stores[0], Source: sources[0], SourceID: "runtime-demo-source",
		Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
		Components: map[experimentation.AssignmentID]experimentation.ComponentState{assignment.ID(): restarted},
		Deliverers: map[campaign.ActionID]application.Deliverer{action.ID(): &runtimeDelivererSpy{}},
		Events:     &runtimeEventSpy{}, MaxAttempts: 1, Logger: logger,
	})
	if err != nil {
		t.Fatalf("building runtime: %v", err)
	}
	if _, err := runtime.Run(context.Background(), built, treatment.ID(), "restarted-rollout", application.Live()); !errors.Is(err, application.ErrGroupUnavailable) {
		t.Fatalf("run error = %v, want ErrGroupUnavailable", err)
	}
}

func TestRuntimeValidatesEntryAndDelivererBeforeColoring(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		treatment campaign.TreatmentID
		configure func(*application.RuntimeSpec, campaign.ActionID)
		wantErr   error
	}{
		{
			name:      "missing treatment",
			treatment: "missing-treatment",
			configure: func(spec *application.RuntimeSpec, action campaign.ActionID) {
				spec.Deliverers = map[campaign.ActionID]application.Deliverer{action: &runtimeDelivererSpy{}}
			},
			wantErr: campaign.ErrNoSuchTreatment,
		},
		{
			name: "missing deliverer",
			configure: func(spec *application.RuntimeSpec, _ campaign.ActionID) {
				spec.Deliverers = map[campaign.ActionID]application.Deliverer{}
			},
			wantErr: application.ErrNoDeliverer,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config := isolatedRuntimeMySQLConfig(t)
			logger := runtimeTestLogger()
			stores, sources := openRuntimeStores(t, config, logger)
			assignment := runtimeAssignment(t, "runtime-entry-validation-assignment", true)
			first, err := experimentation.NewBucketNumber(1)
			if err != nil {
				t.Fatal(err)
			}
			condition, err := audience.Share(assignment, first)
			if err != nil {
				t.Fatal(err)
			}
			group := runtimeGroup(t, "runtime-entry-validation-group", "runtime-demo-source", condition)
			built, treatment, action := runtimeEntryCampaign(t, "runtime-entry-validation", group, audience.Group{})
			component, err := experimentation.NewComponentState(assignment)
			if err != nil {
				t.Fatal(err)
			}
			active, err := component.Start()
			if err != nil {
				t.Fatal(err)
			}
			spec := application.RuntimeSpec{
				Store: stores[0], Source: sources[0], SourceID: "runtime-demo-source",
				Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
				Components: map[experimentation.AssignmentID]experimentation.ComponentState{assignment.ID(): active},
				Events:     &runtimeEventSpy{}, MaxAttempts: 1, Logger: logger,
			}
			testCase.configure(&spec, action.ID())
			runtime, err := application.NewRuntime(spec)
			if err != nil {
				t.Fatalf("building runtime: %v", err)
			}
			requestedTreatment := testCase.treatment
			if requestedTreatment == "" {
				requestedTreatment = treatment.ID()
			}
			if _, err := runtime.Run(context.Background(), built, requestedTreatment, "entry-validation", application.Live()); !errors.Is(err, testCase.wantErr) {
				t.Fatalf("run error = %v, want %v", err, testCase.wantErr)
			}
			if _, found, err := stores[0].LookupColoring(context.Background(), assignment.ID(), "uid_demo_001"); err != nil || found {
				t.Fatalf("invalid run produced coloring %#v (found=%v err=%v)", assignment, found, err)
			}
		})
	}
}

func TestRuntimeReceiptPreservesDistinctUIDsAndPlacements(t *testing.T) {
	config := isolatedRuntimeMySQLConfig(t)
	logger := runtimeTestLogger()
	stores, sources := openRuntimeStores(t, config, logger)
	built, firstAction, nextAction := receiptCampaign(t)
	cases := []struct {
		key        campaign.IdempotencyKey
		uid        audience.UID
		placements []audience.Placement
	}{
		{key: "distinct-execution-one", uid: "uid_distinct_one", placements: []audience.Placement{{Assignment: "assignment-control", Share: 1}, {Assignment: "rollout-wide", Share: 4}}},
		{key: "distinct-execution-two", uid: "uid_distinct_two", placements: []audience.Placement{{Assignment: "assignment-treatment", Share: 3}, {Assignment: "rollout-narrow", Share: 2}}},
	}
	for _, testCase := range cases {
		initial := campaign.Execution{
			Key: testCase.key, Campaign: built.ID(), Treatment: "runtime-first-treatment", UID: testCase.uid,
			Action: firstAction.ID(), Placements: testCase.placements, State: campaign.StateDelivered, Attempts: 1,
		}
		if err := stores[0].ExecutionLog().Append(initial); err != nil {
			t.Fatalf("appending source execution %q: %v", initial.Key, err)
		}
	}
	events := &runtimeEventSpy{}
	deliverer := &runtimeDelivererSpy{}
	runtime, err := application.NewRuntime(application.RuntimeSpec{
		Store: stores[0], Source: sources[0], SourceID: "runtime-receipt-source",
		Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
		Components: map[experimentation.AssignmentID]experimentation.ComponentState{},
		Deliverers: map[campaign.ActionID]application.Deliverer{nextAction.ID(): deliverer},
		Events:     events, MaxAttempts: 1, Logger: logger,
	})
	if err != nil {
		t.Fatalf("building runtime: %v", err)
	}
	for _, testCase := range cases {
		receipt := campaign.Receipt{Key: testCase.key, Arrived: true, Outcome: campaign.OutcomeSuccess}
		if err := runtime.RecordReceipt(context.Background(), built, receipt); err != nil {
			t.Fatalf("recording receipt for %q: %v", testCase.key, err)
		}
		transition, err := campaign.ApplyReceipt(built, campaign.Execution{
			Key: testCase.key, Campaign: built.ID(), Treatment: "runtime-first-treatment", UID: testCase.uid,
			Action: firstAction.ID(), Placements: testCase.placements, State: campaign.StateDelivered, Attempts: 1,
		}, receipt)
		if err != nil || len(transition.Deliveries) != 1 {
			t.Fatalf("expected receipt transition for %q: %#v err=%v", testCase.key, transition, err)
		}
		stored, found, err := stores[0].ExecutionLog().LookupContext(context.Background(), transition.Deliveries[0].Key)
		if err != nil || !found || stored.UID != testCase.uid || !reflect.DeepEqual(stored.Placements, testCase.placements) {
			t.Fatalf("next execution for %q = %#v (found=%v err=%v), want UID %q and placements %#v", testCase.key, stored, found, err, testCase.uid, testCase.placements)
		}
	}
	calls := deliverer.calls()
	if len(calls) != len(cases) {
		t.Fatalf("downstream calls = %d, want %d", len(calls), len(cases))
	}
	for index, testCase := range cases {
		if calls[index].UID != testCase.uid {
			t.Fatalf("downstream request %d UID = %q, want %q", index, calls[index].UID, testCase.uid)
		}
	}
}

func TestRuntimeReceiptRetriesSameKeyAndRetainsAttributionOnExhaustion(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		failures      int
		wantState     campaign.State
		wantDelivered int
	}{
		{name: "success on final attempt", failures: 2, wantState: campaign.StateDelivered, wantDelivered: 1},
		{name: "exhausted", failures: 3, wantState: campaign.StateFailed, wantDelivered: 0},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config := isolatedRuntimeMySQLConfig(t)
			logger := runtimeTestLogger()
			stores, sources := openRuntimeStores(t, config, logger)
			built, firstAction, nextAction := receiptCampaign(t)
			placements := []audience.Placement{{Assignment: "source-arm", Share: 1}, {Assignment: "source-rollout", Share: 3}}
			initial := campaign.Execution{
				Key: "retry-source-execution", Campaign: built.ID(), Treatment: "runtime-first-treatment",
				UID: "uid_retry_attributed", Action: firstAction.ID(), Placements: placements,
				State: campaign.StateDelivered, Attempts: 1,
			}
			if err := stores[0].ExecutionLog().Append(initial); err != nil {
				t.Fatalf("appending source execution: %v", err)
			}
			events := &runtimeEventSpy{}
			deliverer := &runtimeRetryDeliverer{failures: testCase.failures}
			runtime, err := application.NewRuntime(application.RuntimeSpec{
				Store: stores[0], Source: sources[0], SourceID: "runtime-receipt-source",
				Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
				Components: map[experimentation.AssignmentID]experimentation.ComponentState{},
				Deliverers: map[campaign.ActionID]application.Deliverer{nextAction.ID(): deliverer},
				Events:     events, MaxAttempts: 3, Logger: logger,
			})
			if err != nil {
				t.Fatalf("building runtime: %v", err)
			}
			receipt := campaign.Receipt{Key: initial.Key, Arrived: true, Outcome: campaign.OutcomeSuccess}
			if err := runtime.RecordReceipt(context.Background(), built, receipt); err != nil {
				t.Fatalf("recording receipt: %v", err)
			}
			transition, err := campaign.ApplyReceipt(built, initial, receipt)
			if err != nil || len(transition.Deliveries) != 1 {
				t.Fatalf("expected next-step intent: %#v err=%v", transition, err)
			}
			calls := deliverer.calls()
			if len(calls) != 3 {
				t.Fatalf("actual delivery attempts = %d, want 3", len(calls))
			}
			for index, request := range calls {
				if request.Key != transition.Deliveries[0].Key || request.UID != initial.UID {
					t.Fatalf("attempt %d request = %#v, want same key %q and UID %q", index+1, request, transition.Deliveries[0].Key, initial.UID)
				}
			}
			stored, found, err := stores[0].ExecutionLog().LookupContext(context.Background(), transition.Deliveries[0].Key)
			if err != nil || !found || stored.State != testCase.wantState || stored.Attempts != 3 || stored.UID != initial.UID || !reflect.DeepEqual(stored.Placements, placements) {
				t.Fatalf("retry execution = %#v (found=%v err=%v), want state %q, UID and original placements", stored, found, err, testCase.wantState)
			}
			if got := events.count(campaign.DeliveredEvent(nextAction.ID())); got != testCase.wantDelivered {
				t.Fatalf("downstream delivered events = %d, want %d", got, testCase.wantDelivered)
			}
		})
	}
}

func TestRuntimeRunPublishesDeliveredEventForSuccessfulDelivery(t *testing.T) {
	config := isolatedRuntimeMySQLConfig(t)
	logger := runtimeTestLogger()
	stores, sources := openRuntimeStores(t, config, logger)
	group := runtimeGroup(t, "runtime-delivered-event-group", "runtime-demo-source")
	built, treatment, action := runtimeEntryCampaign(t, "runtime-delivered-event", group, audience.Group{})
	events := &runtimeEventSpy{}
	deliverer := &runtimeDelivererSpy{}
	runtime, err := application.NewRuntime(application.RuntimeSpec{
		Store: stores[0], Source: sources[0], SourceID: "runtime-demo-source",
		Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
		Components: map[experimentation.AssignmentID]experimentation.ComponentState{},
		Deliverers: map[campaign.ActionID]application.Deliverer{action.ID(): deliverer},
		Events:     events, MaxAttempts: 1, Logger: logger,
	})
	if err != nil {
		t.Fatalf("building runtime: %v", err)
	}
	report, err := runtime.Run(context.Background(), built, treatment.ID(), "delivery-event", application.Live())
	if err != nil {
		t.Fatalf("running campaign: %v", err)
	}
	published := events.snapshot()
	if len(report.Executions) == 0 || len(published) != len(report.Executions) {
		t.Fatalf("executions = %d, published events = %d; want a delivery event for each successful execution", len(report.Executions), len(published))
	}
	executions := make(map[campaign.IdempotencyKey]campaign.Execution, len(report.Executions))
	for _, execution := range report.Executions {
		executions[execution.Key] = execution
	}
	for _, event := range published {
		execution, found := executions[event.Key]
		if !found || event.Name != campaign.DeliveredEvent(action.ID()) || event.UID != execution.UID || event.Action != execution.Action {
			t.Fatalf("published delivery event = %#v, want DeliveredEvent linked to a returned execution", event)
		}
	}
}

func TestRuntimeRehearsalReadsColoringWithoutWriting(t *testing.T) {
	config := isolatedRuntimeMySQLConfig(t)
	logger := runtimeTestLogger()
	stores, sources := openRuntimeStores(t, config, logger)
	assignment := runtimeAssignment(t, "runtime-rehearsal-assignment", true)
	first, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stores[0].RecordColoringFirst(context.Background(), assignment.ID(), "uid_demo_001", experimentation.ColoringRecord{Bucket: first, Algorithm: assignment.Algorithm()}); err != nil {
		t.Fatalf("seeding existing coloring: %v", err)
	}
	condition, err := audience.Share(assignment, first)
	if err != nil {
		t.Fatal(err)
	}
	group := runtimeGroup(t, "runtime-rehearsal-group", "runtime-demo-source", condition)
	action, err := campaign.NewAction(campaign.ActionSpec{ID: "runtime-rehearsal-action", Direction: campaign.DirectionPush, Required: []string{"copy"}})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(map[string]string{"copy": "constructed rehearsal"})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "runtime-rehearsal-treatment", Call: call})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "runtime-rehearsal-flow",
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{treatment.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "runtime-rehearsal-campaign", Priority: campaign.NewPriority(1), Audience: audience.SelectGroup(group),
		Treatments: []campaign.Treatment{treatment}, Flow: flow,
	})
	if err != nil {
		t.Fatal(err)
	}
	deliverer := &runtimeDelivererSpy{}
	component, err := experimentation.NewComponentState(assignment)
	if err != nil {
		t.Fatal(err)
	}
	activeComponent, err := component.Start()
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := application.NewRuntime(application.RuntimeSpec{
		Store: stores[0], Source: sources[0], SourceID: "runtime-demo-source",
		Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
		Components: map[experimentation.AssignmentID]experimentation.ComponentState{assignment.ID(): activeComponent},
		Deliverers: map[campaign.ActionID]application.Deliverer{action.ID(): deliverer}, Events: &runtimeEventSpy{}, MaxAttempts: 1, Logger: logger,
	})
	if err != nil {
		t.Fatalf("building rehearsal runtime: %v", err)
	}
	db := openRuntimeDB(t, config.DSN)
	var before int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM gmp_management_experiment_colorings").Scan(&before); err != nil {
		t.Fatalf("counting coloring records before rehearsal: %v", err)
	}
	report, err := runtime.Run(context.Background(), built, treatment.ID(), "rehearsal-entry", application.Rehearsal())
	if err != nil {
		t.Fatalf("running rehearsal: %v", err)
	}
	var after int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM gmp_management_experiment_colorings").Scan(&after); err != nil {
		t.Fatalf("counting coloring records after rehearsal: %v", err)
	}
	if after != before {
		t.Fatalf("rehearsal changed coloring count from %d to %d", before, after)
	}
	foundExisting := false
	for _, planned := range report.Planned {
		if planned.UID == "uid_demo_001" {
			foundExisting = true
		}
	}
	if !report.Rehearsal || !foundExisting || len(report.Executions) != 0 {
		t.Fatalf("rehearsal report = %#v, want existing coloring in plan and no executions", report)
	}
	liveReport, err := runtime.Run(context.Background(), built, treatment.ID(), "live-entry", application.Live())
	if err != nil {
		t.Fatalf("running live campaign: %v", err)
	}
	var persistedColorings int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM gmp_management_experiment_colorings").Scan(&persistedColorings); err != nil {
		t.Fatalf("counting live coloring records: %v", err)
	}
	if len(liveReport.Executions) == 0 || persistedColorings <= before || len(deliverer.calls()) != len(liveReport.Executions) {
		t.Fatalf("live report has %d executions, %d delivery calls, %d colorings (before=%d)", len(liveReport.Executions), len(deliverer.calls()), persistedColorings, before)
	}
}

func TestRuntimeDoesNotColorMemberRejectedBeforeAssignment(t *testing.T) {
	config := isolatedRuntimeMySQLConfig(t)
	logger := runtimeTestLogger()
	stores, sources := openRuntimeStores(t, config, logger)
	assignment := runtimeAssignment(t, "runtime-lazy-assignment", true)
	first, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	region, err := audience.Attribute("region", "south")
	if err != nil {
		t.Fatal(err)
	}
	share, err := audience.Share(assignment, first)
	if err != nil {
		t.Fatal(err)
	}
	group := runtimeGroup(t, "runtime-lazy-group", "runtime-demo-source", region, share)
	action, err := campaign.NewAction(campaign.ActionSpec{ID: "runtime-lazy-action", Direction: campaign.DirectionPush, Required: []string{"copy"}})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(map[string]string{"copy": "constructed lazy input"})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "runtime-lazy-treatment", Call: call})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "runtime-lazy-flow",
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{treatment.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "runtime-lazy-campaign", Priority: campaign.NewPriority(1), Audience: audience.SelectGroup(group),
		Treatments: []campaign.Treatment{treatment}, Flow: flow,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := application.NewRuntime(application.RuntimeSpec{
		Store: stores[0], Source: sources[0], SourceID: "runtime-demo-source",
		Algorithms: map[experimentation.AlgorithmVersion]experimentation.Algorithm{"fnv1a-v1": experimentation.FNV1aV1{}},
		Components: map[experimentation.AssignmentID]experimentation.ComponentState{assignment.ID(): mustRuntimeActiveComponent(t, assignment)},
		Deliverers: map[campaign.ActionID]application.Deliverer{action.ID(): &runtimeDelivererSpy{}},
		Events:     &runtimeEventSpy{}, MaxAttempts: 1, Logger: logger,
	})
	if err != nil {
		t.Fatalf("building runtime: %v", err)
	}
	if _, err := runtime.Run(context.Background(), built, treatment.ID(), "lazy-entry", application.Live()); err != nil {
		t.Fatalf("running campaign: %v", err)
	}
	stored, found, err := stores[0].LookupColoring(context.Background(), assignment.ID(), "uid_demo_001")
	if err != nil || found {
		t.Fatalf("coloring for member rejected by the earlier region condition = %#v (found=%v err=%v), want no stored record", stored, found, err)
	}
}

func TestDemoUsesConstructedAudiencePoolAndPersistsExecutionsAndReceipts(t *testing.T) {
	config := isolatedRuntimeMySQLConfig(t)
	seedDemoUserPoolExample(t, config.DSN)
	logger := runtimeTestLogger()

	report, err := application.RunDemo(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("running MySQL demo against the constructed audience: %v", err)
	}
	if len(report.Executions) != 2 {
		t.Fatalf("demo execution count = %d, want the two active records from the repository sample", len(report.Executions))
	}

	store, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("reopening demo MySQL store: %v", err)
	}
	defer store.Close()
	for _, expected := range report.Executions {
		stored, found, err := store.ExecutionLog().LookupContext(context.Background(), expected.Key)
		if err != nil || !found || stored.UID != expected.UID || !stored.HasReceipt() {
			t.Fatalf("demo execution record = %#v (found=%v err=%v), want persisted receipt for %q", stored, found, err, expected.Key)
		}
	}
	db := openRuntimeDB(t, config.DSN)
	var poolRows int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM gmp_demo_user_pool").Scan(&poolRows); err != nil || poolRows != 4 {
		t.Fatalf("constructed audience row count = %d, err=%v, want four rows unchanged", poolRows, err)
	}
}

func seedDemoUserPoolExample(t *testing.T, dsn string) {
	t.Helper()
	db := openRuntimeDB(t, dsn)
	_, err := db.ExecContext(context.Background(), `CREATE TABLE gmp_demo_user_pool (
		member_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		uid VARCHAR(128) NOT NULL, region VARCHAR(64) NOT NULL, lifecycle VARCHAR(32) NOT NULL,
		inactive_days INT NOT NULL, tier VARCHAR(32) NOT NULL,
		UNIQUE KEY uq_gmp_demo_user_pool_uid (uid)
	) ENGINE=InnoDB`)
	if err != nil {
		t.Fatalf("creating demo audience table: %v", err)
	}
	rows := []struct {
		uid, region, lifecycle, tier string
		inactiveDays                 int
	}{
		{"uid_demo_001", "north", "active", "standard", 0},
		{"uid_demo_002", "south", "cooling", "silver", 8},
		{"uid_demo_003", "east", "inactive", "gold", 31},
		{"uid_demo_004", "west", "active", "standard", 2},
	}
	for _, row := range rows {
		if _, err := db.ExecContext(context.Background(), `INSERT INTO gmp_demo_user_pool
			(uid, region, lifecycle, inactive_days, tier) VALUES (?, ?, ?, ?, ?)`,
			row.uid, row.region, row.lifecycle, row.inactiveDays, row.tier); err != nil {
			t.Fatalf("inserting demo audience record: %v", err)
		}
	}
}

func receiptCampaign(t *testing.T) (campaign.Campaign, campaign.Action, campaign.Action) {
	t.Helper()
	source, err := audience.NewSource(audience.SourceSpec{ID: "runtime-receipt-source", Supply: audience.SupplyPull, Category: audience.CategoryExternal})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "runtime-receipt-audience", Source: source.ID()})
	if err != nil {
		t.Fatal(err)
	}
	return receiptCampaignForGroup(t, audience.Single(definition))
}

func runtimeEntryCampaign(t *testing.T, id string, entry, rollout audience.Group) (campaign.Campaign, campaign.Treatment, campaign.Action) {
	t.Helper()
	action, err := campaign.NewAction(campaign.ActionSpec{
		ID: campaign.ActionID(id + "-action"), Direction: campaign.DirectionPush, Required: []string{"copy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	call, err := action.Call(map[string]string{"copy": "constructed runtime entry"})
	if err != nil {
		t.Fatal(err)
	}
	treatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: campaign.TreatmentID(id + "-treatment"), Call: call})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      campaign.FlowID(id + "-flow"),
		Entries: []campaign.EntrySpec{{Group: entry.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{treatment.ID()}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: campaign.CampaignID(id), Priority: campaign.NewPriority(1), Audience: audience.SelectGroup(entry),
		Rollout: rollout, Treatments: []campaign.Treatment{treatment}, Flow: flow,
	})
	if err != nil {
		t.Fatal(err)
	}
	return built, treatment, action
}

func receiptBundleCampaign(t *testing.T) (campaign.Campaign, campaign.Action, campaign.Action, experimentation.ComponentState) {
	t.Helper()
	assignment := runtimeAssignment(t, "runtime-receipt-assignment", false)
	one, err := experimentation.NewBucketNumber(1)
	if err != nil {
		t.Fatal(err)
	}
	two, err := experimentation.NewBucketNumber(2)
	if err != nil {
		t.Fatal(err)
	}
	oneCondition, err := audience.Share(assignment, one)
	if err != nil {
		t.Fatal(err)
	}
	twoCondition, err := audience.Share(assignment, two)
	if err != nil {
		t.Fatal(err)
	}
	oneDefinition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "runtime-receipt-one", Source: "runtime-receipt-source", Conditions: []audience.Condition{oneCondition}})
	if err != nil {
		t.Fatal(err)
	}
	twoDefinition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "runtime-receipt-two", Source: "runtime-receipt-source", Conditions: []audience.Condition{twoCondition}})
	if err != nil {
		t.Fatal(err)
	}
	component, err := experimentation.NewComponentState(assignment)
	if err != nil {
		t.Fatal(err)
	}
	active, err := component.Start()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := audience.NewBundle(audience.BundleSpec{
		ID: "runtime-receipt-bundle", Source: "runtime-receipt-source", Assignment: assignment,
		Arms: []audience.BundleArm{{ID: "runtime-receipt-arm-one", Share: one, Definition: oneDefinition}, {ID: "runtime-receipt-arm-two", Share: two, Definition: twoDefinition}},
	})
	if err != nil {
		t.Fatal(err)
	}
	published, err := bundle.Publish(active)
	if err != nil {
		t.Fatal(err)
	}
	group, found := published.Group(one)
	if !found {
		t.Fatal("published bundle is missing its first arm")
	}
	built, firstAction, nextAction := receiptCampaignForGroup(t, group)
	return built, firstAction, nextAction, active
}

func receiptCampaignForGroup(t *testing.T, group audience.Group) (campaign.Campaign, campaign.Action, campaign.Action) {
	t.Helper()
	firstAction, err := campaign.NewAction(campaign.ActionSpec{
		ID: "runtime-receipt-first", Direction: campaign.DirectionPush, Required: []string{"copy"},
		Outcomes: []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure},
	})
	if err != nil {
		t.Fatal(err)
	}
	nextAction, err := campaign.NewAction(campaign.ActionSpec{ID: "runtime-receipt-next", Direction: campaign.DirectionPush, Required: []string{"copy"}})
	if err != nil {
		t.Fatal(err)
	}
	firstCall, err := firstAction.Call(map[string]string{"copy": "constructed first"})
	if err != nil {
		t.Fatal(err)
	}
	nextCall, err := nextAction.Call(map[string]string{"copy": "constructed next"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "runtime-first-treatment", Call: firstCall})
	if err != nil {
		t.Fatal(err)
	}
	next, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "runtime-next-treatment", Call: nextCall})
	if err != nil {
		t.Fatal(err)
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "runtime-receipt-flow",
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{first.ID()}}},
		Edges:   []campaign.EdgeSpec{{From: first.ID(), To: next.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess}},
	})
	if err != nil {
		t.Fatal(err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "runtime-receipt-campaign", Priority: campaign.NewPriority(1), Audience: audience.SelectGroup(group),
		Treatments: []campaign.Treatment{first, next}, Flow: flow,
	})
	if err != nil {
		t.Fatal(err)
	}
	return built, firstAction, nextAction
}

func runtimeAssignment(t *testing.T, id string, colored bool) experimentation.Definition {
	t.Helper()
	split, err := experimentation.NewSplit(100, []int{50, 50})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := experimentation.NewDefinition(experimentation.DefinitionSpec{
		ID: experimentation.AssignmentID(id), Split: split, Salt: "runtime-test-salt",
		Algorithm: "fnv1a-v1", Colored: colored,
	})
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

func mustRuntimeActiveComponent(t *testing.T, definition experimentation.Definition) experimentation.ComponentState {
	t.Helper()
	component, err := experimentation.NewComponentState(definition)
	if err != nil {
		t.Fatal(err)
	}
	active, err := component.Start()
	if err != nil {
		t.Fatal(err)
	}
	return active
}

func runtimeGroup(t *testing.T, id, source string, conditions ...audience.Condition) audience.Group {
	t.Helper()
	definition, err := audience.NewDefinition(audience.DefinitionSpec{ID: audience.DefinitionID(id), Source: audience.SourceID(source), Conditions: conditions})
	if err != nil {
		t.Fatal(err)
	}
	group, err := audience.NewGroup(audience.GroupID(id), definition)
	if err != nil {
		t.Fatal(err)
	}
	return group
}

func openRuntimeStores(t *testing.T, config management.MySQLConfig, logger *slog.Logger) ([]*management.MySQLStore, []*management.MySQLAudienceSource) {
	t.Helper()
	stores := make([]*management.MySQLStore, 2)
	sources := make([]*management.MySQLAudienceSource, 2)
	for index := range stores {
		store, err := management.OpenMySQLStore(context.Background(), config, logger)
		if err != nil {
			t.Fatalf("opening MySQL store %d: %v", index, err)
		}
		stores[index] = store
		t.Cleanup(func() { _ = store.Close() })
	}
	seedRuntimeUserPool(t, config.DSN)
	for index, store := range stores {
		source, err := store.NewDemoAudienceSource(2, logger)
		if err != nil {
			t.Fatalf("building MySQL audience source %d: %v", index, err)
		}
		sources[index] = source
	}
	return stores, sources
}

func seedRuntimeUserPool(t *testing.T, dsn string) {
	t.Helper()
	db := openRuntimeDB(t, dsn)
	_, err := db.ExecContext(context.Background(), `CREATE TABLE gmp_demo_user_pool (
		member_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		uid VARCHAR(128) NOT NULL, region VARCHAR(64) NOT NULL, lifecycle VARCHAR(32) NOT NULL,
		inactive_days INT NOT NULL, tier VARCHAR(32) NOT NULL,
		UNIQUE KEY uq_gmp_demo_user_pool_uid (uid)
	) ENGINE=InnoDB`)
	if err != nil {
		t.Fatalf("creating synthetic audience table: %v", err)
	}
	for index, uid := range []string{"uid_demo_001", "uid_demo_002", "uid_demo_003", "uid_demo_004"} {
		if _, err := db.ExecContext(context.Background(), "INSERT INTO gmp_demo_user_pool (uid, region, lifecycle, inactive_days, tier) VALUES (?, ?, ?, ?, ?)", uid, []string{"north", "south", "east", "west"}[index], "active", index, "constructed"); err != nil {
			t.Fatalf("seeding synthetic audience row %d: %v", index, err)
		}
	}
}

func isolatedRuntimeMySQLConfig(t *testing.T) management.MySQLConfig {
	t.Helper()
	dsn := os.Getenv("GMP_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("GMP_TEST_MYSQL_DSN is required for MySQL runtime integration")
	}
	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parsing test database configuration: %v", err)
	}
	admin := *parsed
	admin.DBName = ""
	db, err := sql.Open("mysql", admin.FormatDSN())
	if err != nil {
		t.Fatalf("opening test database admin connection: %v", err)
	}
	defer db.Close()
	databaseName := fmt.Sprintf("gmp_runtime_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(context.Background(), "CREATE DATABASE `"+databaseName+"`"); err != nil {
		t.Fatalf("creating isolated integration schema: %v", err)
	}
	t.Cleanup(func() {
		cleanup, err := sql.Open("mysql", admin.FormatDSN())
		if err != nil {
			t.Errorf("opening integration cleanup connection: %v", err)
			return
		}
		defer cleanup.Close()
		if _, err := cleanup.ExecContext(context.Background(), "DROP DATABASE `"+databaseName+"`"); err != nil {
			t.Errorf("dropping isolated integration schema: %v", err)
		}
	})
	parsed.DBName = databaseName
	return management.MySQLConfig{DSN: parsed.FormatDSN(), MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute}
}

func openRuntimeDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("opening test database connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func runtimeTestLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

type runtimeEventSpy struct {
	mu     sync.Mutex
	events []campaign.Event
}

func (s *runtimeEventSpy) Publish(_ context.Context, event campaign.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *runtimeEventSpy) count(name campaign.EventName) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, event := range s.events {
		if event.Name == name {
			count++
		}
	}
	return count
}

func (s *runtimeEventSpy) snapshot() []campaign.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]campaign.Event(nil), s.events...)
}

type runtimeDelivererSpy struct {
	mu       sync.Mutex
	requests []campaign.Request
}

func (s *runtimeDelivererSpy) Deliver(_ context.Context, request campaign.Request) (campaign.Ack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	return campaign.Ack{ExternalRef: "constructed-downstream-ref"}, nil
}

func (s *runtimeDelivererSpy) calls() []campaign.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]campaign.Request(nil), s.requests...)
}

type runtimeRetryDeliverer struct {
	mu       sync.Mutex
	failures int
	requests []campaign.Request
}

func (s *runtimeRetryDeliverer) Deliver(_ context.Context, request campaign.Request) (campaign.Ack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, request)
	if len(s.requests) <= s.failures {
		return campaign.Ack{}, errors.New("constructed downstream failure")
	}
	return campaign.Ack{ExternalRef: "constructed-retry-ref"}, nil
}

func (s *runtimeRetryDeliverer) calls() []campaign.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]campaign.Request(nil), s.requests...)
}
