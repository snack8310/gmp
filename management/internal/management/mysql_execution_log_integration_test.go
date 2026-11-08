package management_test

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/snack8310/gmp/gmp-core/audience"
	"github.com/snack8310/gmp/gmp-core/campaign"
	"github.com/snack8310/gmp/management/internal/management"
)

func TestMySQLExecutionLogPersistsReceiptAndRejectsConflicts(t *testing.T) {
	config := isolatedMySQLIntegrationConfig(t)
	logger := slog.New(slog.NewJSONHandler(ioDiscard{}, nil))
	store, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("opening MySQL store: %v", err)
	}
	log := store.ExecutionLog()
	execution := campaign.Execution{
		Key:         "execution-key-synthetic",
		Campaign:    "campaign-synthetic",
		Treatment:   "treatment-synthetic",
		UID:         "uid-synthetic",
		Action:      "action-synthetic",
		Placements:  []audience.Placement{{Assignment: "assignment-synthetic", Share: 2}},
		State:       campaign.StateDelivered,
		Attempts:    2,
		ExternalRef: "delivery-ref-synthetic",
	}
	if err := log.Append(execution); err != nil {
		t.Fatalf("appending execution: %v", err)
	}
	duplicate := execution
	duplicate.Campaign = "campaign-must-not-replace-first"
	if err := log.Append(duplicate); err != nil {
		t.Fatalf("appending the same execution key: %v", err)
	}
	firstWrite, found, err := log.Lookup(execution.Key)
	if err != nil || !found || !reflect.DeepEqual(firstWrite, execution) {
		t.Fatalf("duplicate append replaced the first execution: %#v (found=%v err=%v)", firstWrite, found, err)
	}

	receipted := execution
	receipted.State = campaign.StateArrived
	receipted.Outcome = campaign.OutcomeSuccess
	receipted.ReceiptExternalRef = "receipt-ref-synthetic"
	receipted.ReceiptAttributes = map[string]string{"status": "arrived", "marker": "constructed"}
	firstAccepted, err := log.AcceptReceipt(receipted)
	if err != nil || !firstAccepted {
		t.Fatalf("recording receipt: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("closing first store: %v", err)
	}

	reopened, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("reopening MySQL store: %v", err)
	}
	defer reopened.Close()
	reopenedLog := reopened.ExecutionLog()
	got, found, err := reopenedLog.Lookup(execution.Key)
	if err != nil || !found {
		t.Fatalf("looking up execution after restart (found=%v): %v", found, err)
	}
	if !reflect.DeepEqual(got, receipted) {
		t.Fatalf("persisted execution differs:\n got: %#v\nwant: %#v", got, receipted)
	}
	all, err := reopenedLog.All()
	if err != nil || len(all) != 1 || !reflect.DeepEqual(all[0], receipted) {
		t.Fatalf("all executions = %#v, err = %v", all, err)
	}
	firstAccepted, err = reopenedLog.AcceptReceipt(receipted)
	if err != nil || firstAccepted {
		t.Fatalf("replaying identical receipt: %v", err)
	}

	conflict := receipted
	conflict.State = campaign.StateNotArrived
	conflict.Outcome = campaign.OutcomeFailure
	if _, err := reopenedLog.AcceptReceipt(conflict); err != campaign.ErrConflictingReceipt {
		t.Fatalf("conflicting receipt error = %v, want ErrConflictingReceipt", err)
	}
	stored, found, err := reopenedLog.Lookup(execution.Key)
	if err != nil || !found || !reflect.DeepEqual(stored, receipted) {
		t.Fatalf("conflicting receipt changed stored execution: %#v (found=%v err=%v)", stored, found, err)
	}
}

func TestMySQLRunnerConcurrentIdenticalReceiptsPublishAndFollowOnce(t *testing.T) {
	config := isolatedMySQLIntegrationConfig(t)
	logger := slog.New(slog.NewJSONHandler(ioDiscard{}, nil))
	stores := make([]*management.MySQLStore, 2)
	for index := range stores {
		store, err := management.OpenMySQLStore(context.Background(), config, logger)
		if err != nil {
			t.Fatalf("opening MySQL store %d: %v", index, err)
		}
		stores[index] = store
		defer store.Close()
	}

	source, err := audience.RegisterSource(audience.SourceSpec{
		ID: "receipt-test-source", Supply: audience.SupplyPull, Category: audience.CategoryExternal,
	})
	if err != nil {
		t.Fatalf("registering test source: %v", err)
	}
	definition, err := audience.NewDefinition(audience.DefinitionSpec{ID: "receipt-test-audience", Source: source.ID()})
	if err != nil {
		t.Fatalf("building test audience: %v", err)
	}
	group := audience.Single(definition)
	audiences := audience.NewService(nil)
	if err := audiences.Register(source, audience.NewMemoryFeed(audience.Record{UID: "uid-receipt-test"})); err != nil {
		t.Fatalf("registering audience feed: %v", err)
	}
	firstAction, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: "receipt-test-first", Direction: campaign.DirectionPush, Required: []string{"copy"},
		Outcomes: []campaign.Outcome{campaign.OutcomeSuccess, campaign.OutcomeFailure},
	})
	if err != nil {
		t.Fatalf("registering first action: %v", err)
	}
	nextAction, err := campaign.RegisterAction(campaign.ActionSpec{
		ID: "receipt-test-next", Direction: campaign.DirectionPush, Required: []string{"copy"},
	})
	if err != nil {
		t.Fatalf("registering next action: %v", err)
	}
	firstCall, err := firstAction.Call(map[string]string{"copy": "synthetic first"})
	if err != nil {
		t.Fatalf("building first call: %v", err)
	}
	nextCall, err := nextAction.Call(map[string]string{"copy": "synthetic next"})
	if err != nil {
		t.Fatalf("building next call: %v", err)
	}
	firstTreatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "receipt-test-first", Call: firstCall})
	if err != nil {
		t.Fatalf("building first treatment: %v", err)
	}
	nextTreatment, err := campaign.NewTreatment(campaign.TreatmentSpec{ID: "receipt-test-next", Call: nextCall})
	if err != nil {
		t.Fatalf("building next treatment: %v", err)
	}
	flow, err := campaign.NewFlow(campaign.FlowSpec{
		ID:      "receipt-test-flow",
		Entries: []campaign.EntrySpec{{Group: group.ID(), Kind: campaign.EntryStart, Treatments: []campaign.TreatmentID{firstTreatment.ID()}}},
		Edges:   []campaign.EdgeSpec{{From: firstTreatment.ID(), To: nextTreatment.ID(), Kind: campaign.EdgeOnReceipt, When: campaign.OutcomeSuccess}},
	})
	if err != nil {
		t.Fatalf("building test flow: %v", err)
	}
	built, err := campaign.NewCampaign(campaign.CampaignSpec{
		ID: "receipt-test-campaign", Priority: campaign.NewPriority(10), Audience: audience.SelectGroup(group),
		Treatments: []campaign.Treatment{firstTreatment, nextTreatment}, Flow: flow,
	})
	if err != nil {
		t.Fatalf("building test campaign: %v", err)
	}

	execution := campaign.Execution{
		Key: "execution-key-concurrent-identical", Campaign: built.ID(), Treatment: firstTreatment.ID(),
		UID: "uid-receipt-test", Action: firstAction.ID(), State: campaign.StateDelivered, Attempts: 1,
	}
	if err := stores[0].ExecutionLog().Append(execution); err != nil {
		t.Fatalf("appending source execution: %v", err)
	}
	barrier := &receiptReadBarrier{ready: make(chan struct{})}
	events := &integrationEventSink{}
	runners := make([]*campaign.Runner, 2)
	deliverers := make([]*campaign.MemoryDeliverer, 2)
	for index := range runners {
		runner, err := campaign.NewRunner(campaign.RunnerSpec{
			Audiences: audiences,
			Log: &receiptLookupBarrierLog{
				Log: stores[index].ExecutionLog(), ReceiptAcceptor: stores[index].ExecutionLog(), barrier: barrier,
			},
			Events: events, MaxAttempts: 1,
		})
		if err != nil {
			t.Fatalf("building runner %d: %v", index, err)
		}
		deliverers[index] = campaign.NewMemoryDeliverer()
		if err := runner.RegisterDeliverer(nextAction.ID(), deliverers[index]); err != nil {
			t.Fatalf("wiring next action for runner %d: %v", index, err)
		}
		runners[index] = runner
	}

	receipt := campaign.Receipt{Key: execution.Key, Arrived: true, Outcome: campaign.OutcomeSuccess}
	results := make(chan error, len(runners))
	for _, runner := range runners {
		go func(runner *campaign.Runner) { results <- runner.RecordReceipt(built, receipt) }(runner)
	}
	for range runners {
		if err := <-results; err != nil {
			t.Fatalf("recording concurrent identical receipt: %v", err)
		}
	}
	if got := events.count(campaign.ArrivedEvent(firstAction.ID())); got != 1 {
		t.Fatalf("arrival events = %d, want one", got)
	}
	if got := events.count(campaign.DeliveredEvent(nextAction.ID())); got != 1 {
		t.Fatalf("next-step delivery events = %d, want one", got)
	}
	callCount := len(deliverers[0].Calls()) + len(deliverers[1].Calls())
	if callCount != 1 {
		t.Fatalf("next-step deliverer calls = %d, want one", callCount)
	}
}

type receiptReadBarrier struct {
	mu      sync.Mutex
	lookups int
	ready   chan struct{}
}

func (b *receiptReadBarrier) wait() {
	b.mu.Lock()
	b.lookups++
	wait := b.lookups <= 2
	if b.lookups == 2 {
		close(b.ready)
	}
	b.mu.Unlock()
	if wait {
		<-b.ready
	}
}

type receiptLookupBarrierLog struct {
	campaign.Log
	campaign.ReceiptAcceptor
	barrier *receiptReadBarrier
}

func (l *receiptLookupBarrierLog) Lookup(key campaign.IdempotencyKey) (campaign.Execution, bool, error) {
	execution, found, err := l.Log.Lookup(key)
	l.barrier.wait()
	return execution, found, err
}

type integrationEventSink struct {
	mu     sync.Mutex
	events []campaign.Event
}

func (s *integrationEventSink) Publish(event campaign.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *integrationEventSink) count(name campaign.EventName) int {
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

func TestMySQLExecutionLogConcurrentReceiptsKeepOneWinner(t *testing.T) {
	config := isolatedMySQLIntegrationConfig(t)
	logger := slog.New(slog.NewJSONHandler(ioDiscard{}, nil))
	stores := make([]*management.MySQLStore, 2)
	for index := range stores {
		store, err := management.OpenMySQLStore(context.Background(), config, logger)
		if err != nil {
			t.Fatalf("opening MySQL store %d: %v", index, err)
		}
		stores[index] = store
		defer store.Close()
	}
	base := campaign.Execution{
		Key: "execution-key-concurrent", Campaign: "campaign-synthetic", Treatment: "treatment-synthetic",
		UID: "uid-synthetic", Action: "action-synthetic", State: campaign.StateDelivered, Attempts: 1,
	}
	if err := stores[0].ExecutionLog().Append(base); err != nil {
		t.Fatalf("appending concurrent execution: %v", err)
	}
	arrived := base
	arrived.State = campaign.StateArrived
	arrived.Outcome = campaign.OutcomeSuccess
	notArrived := base
	notArrived.State = campaign.StateNotArrived
	notArrived.Outcome = campaign.OutcomeFailure
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		results <- stores[0].ExecutionLog().Update(arrived)
	}()
	go func() {
		<-start
		results <- stores[1].ExecutionLog().Update(notArrived)
	}()
	close(start)
	first, second := <-results, <-results
	if (first == nil) == (second == nil) {
		t.Fatalf("concurrent receipt results = [%v, %v], want one accepted and one rejected", first, second)
	}
	conflict := first
	if conflict == nil {
		conflict = second
	}
	if conflict != campaign.ErrConflictingReceipt {
		t.Fatalf("losing receipt error = %v, want ErrConflictingReceipt", conflict)
	}
	stored, found, err := stores[0].ExecutionLog().Lookup(base.Key)
	if err != nil || !found || !stored.HasReceipt() {
		t.Fatalf("reading concurrent receipt winner: %#v (found=%v err=%v)", stored, found, err)
	}
	if !((stored.State == campaign.StateArrived && stored.Outcome == campaign.OutcomeSuccess) ||
		(stored.State == campaign.StateNotArrived && stored.Outcome == campaign.OutcomeFailure)) {
		t.Fatalf("stored receipt is not one of the competing valid values: %#v", stored)
	}
}

func TestMySQLStoreUpgradesAnExistingManagementSchema(t *testing.T) {
	config := isolatedMySQLIntegrationConfig(t)
	adminDB := openMySQLAdminDB(t, config.DSN)
	script, err := os.ReadFile("migrations/001_management.sql")
	if err != nil {
		t.Fatalf("reading the prior management migration: %v", err)
	}
	for _, statement := range strings.Split(string(script), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := adminDB.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("creating the prior management schema: %v", err)
		}
	}
	if _, err := adminDB.ExecContext(context.Background(), "INSERT INTO gmp_management_schema_migrations (migration_id) VALUES ('001_management')"); err != nil {
		t.Fatalf("recording the prior schema version: %v", err)
	}

	store, err := management.OpenMySQLStore(context.Background(), config, slog.New(slog.NewJSONHandler(ioDiscard{}, nil)))
	if err != nil {
		t.Fatalf("upgrading the existing management schema: %v", err)
	}
	defer store.Close()
	execution := campaign.Execution{
		Key: "execution-key-after-upgrade", Campaign: "campaign-synthetic", Treatment: "treatment-synthetic",
		UID: "uid-synthetic", Action: "action-synthetic", State: campaign.StateDelivered, Attempts: 1,
	}
	if err := store.ExecutionLog().Append(execution); err != nil {
		t.Fatalf("writing to the upgraded execution table: %v", err)
	}
}

func TestDemoAudienceSourceUsesSharedMySQLStorePool(t *testing.T) {
	config := isolatedMySQLIntegrationConfig(t)
	logger := slog.New(slog.NewJSONHandler(ioDiscard{}, nil))
	store, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("opening MySQL store: %v", err)
	}
	defer store.Close()

	adminDB, err := sql.Open("mysql", config.DSN)
	if err != nil {
		t.Fatalf("opening demo schema setup connection: %v", err)
	}
	defer adminDB.Close()
	script, err := os.ReadFile("demo_user_pool.sql")
	if err != nil {
		t.Fatalf("reading demo user pool SQL: %v", err)
	}
	for _, statement := range strings.Split(string(script), ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := adminDB.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("applying synthetic demo user pool SQL: %v", err)
		}
	}

	source, err := store.NewDemoAudienceSource(2, logger)
	if err != nil {
		t.Fatalf("building source from the store's shared pool: %v", err)
	}
	page, err := source.ReadPage(context.Background(), nil)
	if err != nil {
		t.Fatalf("reading a demo audience page: %v", err)
	}
	if len(page.Members) != 2 || page.NextCursor == nil {
		t.Fatalf("first page = %#v, want two synthetic members and a continuation cursor", page)
	}
	if page.Members[0].UID == "" || page.Members[0].Attributes["region"] == "" {
		t.Fatalf("demo source omitted the UID or mapped region: %#v", page.Members[0])
	}
	secondPage, err := source.ReadPage(context.Background(), page.NextCursor)
	if err != nil {
		t.Fatalf("reading the next demo audience page: %v", err)
	}
	if len(secondPage.Members) == 0 || secondPage.Members[0].UID == page.Members[0].UID {
		t.Fatalf("keyset pagination repeated or omitted a member: first=%#v second=%#v", page.Members, secondPage.Members)
	}
	largePageSource, err := store.NewDemoAudienceSource(1_000_000_000, logger)
	if err != nil {
		t.Fatalf("building a source with an explicit large page size: %v", err)
	}
	largePage, err := largePageSource.ReadPage(context.Background(), nil)
	if err != nil || len(largePage.Members) != 4 {
		t.Fatalf("large-page source read returned %d members, err = %v", len(largePage.Members), err)
	}
	if _, err := adminDB.ExecContext(context.Background(), "ALTER TABLE gmp_demo_user_pool DROP INDEX uq_gmp_demo_user_pool_uid"); err != nil {
		t.Fatalf("removing the demo UID uniqueness contract: %v", err)
	}
	if _, err := store.NewDemoAudienceSource(2, logger); err == nil {
		t.Fatal("demo source accepted a user pool without the required UID uniqueness constraint")
	}
}

func TestDemoSourceConfigRequiresExplicitPageSize(t *testing.T) {
	for _, pageSize := range []int{0, -1} {
		if _, err := management.DemoAudienceSourceConfig(pageSize); err == nil {
			t.Fatalf("demo audience page size %d was accepted", pageSize)
		}
	}
}
