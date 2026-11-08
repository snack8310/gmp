package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/snack8310/gmp/management/internal/httpapi"
	"github.com/snack8310/gmp/management/internal/management"
)

func TestCampaignAPIPersistsItsExistingJSONContractInMySQL(t *testing.T) {
	dsn := os.Getenv("GMP_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("GMP_TEST_MYSQL_DSN is required for the MySQL HTTP contract")
	}
	config := management.MySQLConfig{DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute}
	logger := slog.New(slog.NewJSONHandler(ioDiscard{}, nil))

	store, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("opening MySQL store: %v", err)
	}
	name := fmt.Sprintf("api-mysql-contract-%d", time.Now().UnixNano())
	request := httptest.NewRequest(http.MethodPost, "/api/v1/campaigns", strings.NewReader(fmt.Sprintf("{\"name\":%q}", name)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	httpapi.NewHandler(store, logger).ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		_ = store.Close()
		t.Fatalf("POST /api/v1/campaigns returned %d: %s", response.Code, response.Body.String())
	}
	var created management.Campaign
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		_ = store.Close()
		t.Fatalf("decoding campaign response: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("closing first store: %v", err)
	}

	reopened, err := management.OpenMySQLStore(context.Background(), config, logger)
	if err != nil {
		t.Fatalf("reopening MySQL store: %v", err)
	}
	t.Cleanup(func() {
		_ = reopened.DeleteCampaign(created.ID)
		_ = reopened.Close()
	})
	request = httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/"+created.ID, nil)
	request.SetPathValue("campaignId", created.ID)
	response = httptest.NewRecorder()
	httpapi.NewHandler(reopened, logger).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/campaigns/{id} returned %d: %s", response.Code, response.Body.String())
	}
	var fetched management.Campaign
	if err := json.NewDecoder(response.Body).Decode(&fetched); err != nil {
		t.Fatalf("decoding persisted campaign response: %v", err)
	}
	if fetched.ID != created.ID || fetched.Name != created.Name {
		t.Fatalf("persisted API object = %#v, created object = %#v", fetched, created)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
