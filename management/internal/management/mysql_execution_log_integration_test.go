package management_test

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"reflect"
	"strings"
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
	if err := log.Update(receipted); err != nil {
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
	if err := reopenedLog.Update(receipted); err != nil {
		t.Fatalf("replaying identical receipt: %v", err)
	}

	conflict := receipted
	conflict.State = campaign.StateNotArrived
	conflict.Outcome = campaign.OutcomeFailure
	if err := reopenedLog.Update(conflict); err != campaign.ErrConflictingReceipt {
		t.Fatalf("conflicting receipt error = %v, want ErrConflictingReceipt", err)
	}
	stored, found, err := reopenedLog.Lookup(execution.Key)
	if err != nil || !found || !reflect.DeepEqual(stored, receipted) {
		t.Fatalf("conflicting receipt changed stored execution: %#v (found=%v err=%v)", stored, found, err)
	}
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
