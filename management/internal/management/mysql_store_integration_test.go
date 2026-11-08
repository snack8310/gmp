package management_test

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/snack8310/gmp/management/internal/management"
)

func TestMySQLStoreKeepsCampaignConfigurationAcrossRestart(t *testing.T) {
	ctx := context.Background()
	config := mysqlIntegrationConfig(t)
	logger := slog.New(slog.NewJSONHandler(ioDiscard{}, nil))
	store, err := management.OpenMySQLStore(ctx, config, logger)
	if err != nil {
		t.Fatalf("opening MySQL store: %v", err)
	}

	name := fmt.Sprintf("mysql-contract-%d", time.Now().UnixNano())
	groupID := fmt.Sprintf("mysql_group_%d", time.Now().UnixNano())
	ref, problem := store.UpsertGroupAudienceReference(groupID, name)
	if problem != nil {
		t.Fatalf("creating audience reference: %v", problem)
	}
	created, problem := store.CreateCampaign(name)
	if problem != nil {
		t.Fatalf("creating campaign: %v", problem)
	}
	selection := &management.AudienceSelection{Type: "group", GroupID: ref.GroupID}
	configured, problem := store.UpdateAudienceSelection(created.ID, nil, selection)
	if problem != nil {
		t.Fatalf("selecting audience: %v", problem)
	}
	treatment, problem := store.CreateTreatment(created.ID, management.Treatment{
		Name:     "回归触达",
		ActionID: "action-rcs",
	})
	if problem != nil {
		t.Fatalf("creating treatment: %v", problem)
	}
	configured, problem = store.UpdateFlow(created.ID, nil, nil, management.Flow{
		ID:      "mysql_contract_flow",
		Entries: []management.EntryRoute{{GroupID: groupID, Kind: "start", TreatmentIDs: []string{treatment.ID}}},
		Edges:   []management.Edge{},
	})
	if problem != nil {
		t.Fatalf("saving activity flow: %v", problem)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("closing first store: %v", err)
	}

	reopened, err := management.OpenMySQLStore(ctx, config, logger)
	if err != nil {
		t.Fatalf("reopening MySQL store: %v", err)
	}
	t.Cleanup(func() {
		_ = reopened.DeleteCampaign(created.ID)
		_ = reopened.DeleteGroupAudienceReference(groupID)
		_ = reopened.Close()
	})
	got, problem := reopened.Campaign(created.ID)
	if problem != nil {
		t.Fatalf("reading campaign after restart: %v", problem)
	}
	if !reflect.DeepEqual(got, configured) {
		t.Fatalf("campaign changed across restart:\n got: %#v\nwant: %#v", got, configured)
	}

	updated, problem := reopened.UpdateCampaign(created.ID, management.Campaign{
		ID:           created.ID,
		Name:         name + " updated",
		Audience:     got.Audience,
		Treatments:   got.Treatments,
		ActivityFlow: got.ActivityFlow,
	})
	if problem != nil {
		t.Fatalf("updating persisted campaign: %v", problem)
	}
	if updated.Name != name+" updated" {
		t.Fatalf("updated campaign name = %q", updated.Name)
	}
}

func TestMySQLStoreSerializesCampaignCreationAcrossInstances(t *testing.T) {
	config := mysqlIntegrationConfig(t)
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

	const workers = 12
	ids := make(chan string, workers)
	errors := make(chan error, workers)
	start := make(chan struct{})
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			<-start
			created, problem := stores[worker%len(stores)].CreateCampaign(fmt.Sprintf("concurrent-%d-%d", time.Now().UnixNano(), worker))
			if problem != nil {
				errors <- problem
				return
			}
			ids <- created.ID
		}(worker)
	}
	close(start)
	group.Wait()
	close(ids)
	close(errors)
	for err := range errors {
		t.Fatalf("concurrent campaign creation: %v", err)
	}
	seen := make(map[string]bool, workers)
	for id := range ids {
		if seen[id] {
			t.Fatalf("two concurrent writes received campaign ID %q", id)
		}
		seen[id] = true
	}
	for id := range seen {
		if problem := stores[0].DeleteCampaign(id); problem != nil {
			t.Errorf("deleting created campaign %q: %v", id, problem)
		}
	}
}

func TestMySQLStoreRollsBackARejectedDatabaseWrite(t *testing.T) {
	config := mysqlIntegrationConfig(t)
	logger := slog.New(slog.NewJSONHandler(ioDiscard{}, nil))
	store, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("opening MySQL store: %v", err)
	}
	defer store.Close()

	suffix := time.Now().UnixNano()
	groupID := fmt.Sprintf("rollback_group_%d", suffix)
	ref, problem := store.UpsertGroupAudienceReference(groupID, "事务回滚回归")
	if problem != nil {
		t.Fatalf("creating audience reference: %v", problem)
	}
	campaign, problem := store.CreateCampaign("事务回滚回归")
	if problem != nil {
		t.Fatalf("creating campaign: %v", problem)
	}
	selection := &management.AudienceSelection{Type: "group", GroupID: ref.GroupID}
	campaign, problem = store.UpdateAudienceSelection(campaign.ID, nil, selection)
	if problem != nil {
		t.Fatalf("selecting audience: %v", problem)
	}
	_, problem = store.CreateTreatment(campaign.ID, management.Treatment{Name: "既有投放项", ActionID: "action-rcs"})
	if problem != nil {
		t.Fatalf("creating treatment: %v", problem)
	}
	campaign, problem = store.Campaign(campaign.ID)
	if problem != nil {
		t.Fatalf("reading configured campaign: %v", problem)
	}

	adminDB, err := sql.Open("mysql", config.DSN)
	if err != nil {
		t.Fatalf("opening isolated test connection: %v", err)
	}
	defer adminDB.Close()
	trigger := fmt.Sprintf("gmp_test_reject_treatment_%d", suffix)
	if _, err := adminDB.ExecContext(context.Background(), "CREATE TRIGGER "+trigger+" BEFORE INSERT ON gmp_management_treatments FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'forced test failure'"); err != nil {
		t.Fatalf("creating transaction failure trigger: %v", err)
	}
	defer adminDB.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS "+trigger)

	_, problem = store.UpdateCampaign(campaign.ID, management.Campaign{
		ID:           campaign.ID,
		Name:         "不应提交",
		Audience:     campaign.Audience,
		Treatments:   campaign.Treatments,
		ActivityFlow: campaign.ActivityFlow,
	})
	if problem == nil || problem.Code != "storage_unavailable" {
		t.Fatalf("got %v, want storage_unavailable after the injected database failure", problem)
	}
	stored, problem := store.Campaign(campaign.ID)
	if problem != nil {
		t.Fatalf("reading campaign after rollback: %v", problem)
	}
	if stored.Name != campaign.Name {
		t.Fatalf("failed write changed campaign name to %q", stored.Name)
	}
	if _, err := adminDB.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS "+trigger); err != nil {
		t.Fatalf("dropping transaction failure trigger: %v", err)
	}
	if problem := store.DeleteCampaign(campaign.ID); problem != nil {
		t.Fatalf("deleting rollback fixture: %v", problem)
	}
	if problem := store.DeleteGroupAudienceReference(groupID); problem != nil {
		t.Fatalf("deleting rollback audience fixture: %v", problem)
	}
}

func mysqlIntegrationConfig(t *testing.T) management.MySQLConfig {
	t.Helper()
	dsn := os.Getenv("GMP_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("GMP_TEST_MYSQL_DSN is required for the MySQL integration contract")
	}
	return management.MySQLConfig{
		DSN:             dsn,
		MaxOpenConns:    4,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
