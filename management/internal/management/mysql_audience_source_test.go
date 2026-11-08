package management_test

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/snack8310/gmp/management/internal/management"
)

func TestMySQLAudienceSourceRequiresExplicitSafeMapping(t *testing.T) {
	base := management.MySQLAudienceSourceConfig{
		Table:        "gmp_user_pool",
		UIDColumn:    "user_id",
		UIDUnique:    true,
		CursorColumn: "cursor_id",
		CursorUnique: true,
		CursorStable: true,
		Attributes:   map[string]string{"lifecycle": "lifecycle_value"},
		PageSize:     100,
	}
	if _, err := management.NewMySQLAudienceSource(nil, base, nil); err == nil {
		t.Fatal("source without a database handle was accepted")
	}

	invalid := base
	invalid.UIDUnique = false
	if _, err := management.NewMySQLAudienceSource(nil, invalid, nil); err == nil {
		t.Fatal("source without an explicit UID uniqueness declaration was accepted")
	}

	invalid = base
	invalid.Table = "users; DELETE FROM users"
	if _, err := management.NewMySQLAudienceSource(nil, invalid, nil); err == nil {
		t.Fatal("unsafe table identifier was accepted")
	}

	invalid = base
	invalid.PageSize = 0
	if _, err := management.NewMySQLAudienceSource(nil, invalid, nil); err == nil {
		t.Fatal("missing page size was accepted")
	}

	invalid = base
	invalid.CursorUnique = false
	if _, err := management.NewMySQLAudienceSource(nil, invalid, nil); err == nil {
		t.Fatal("cursor without an explicit uniqueness declaration was accepted")
	}

	invalid = base
	invalid.CursorStable = false
	if _, err := management.NewMySQLAudienceSource(nil, invalid, nil); err == nil {
		t.Fatal("cursor without an explicit immutability declaration was accepted")
	}

	invalid = base
	invalid.Attributes = nil
	if _, err := management.NewMySQLAudienceSource(nil, invalid, nil); err == nil {
		t.Fatal("missing explicit attribute mapping was accepted")
	}
}

func TestMySQLAudienceSourceReadsOnlyMappedFieldsInPages(t *testing.T) {
	dsn := os.Getenv("GMP_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("GMP_TEST_MYSQL_DSN is required for the MySQL source contract")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("opening isolated test database: %v", err)
	}
	defer db.Close()

	table := fmt.Sprintf("gmp_source_test_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(context.Background(), "CREATE TABLE `"+table+"` (cursor_id BIGINT NOT NULL PRIMARY KEY, user_id VARCHAR(64) NOT NULL, lifecycle_value VARCHAR(32) NULL, country_value VARCHAR(32) NOT NULL)"); err != nil {
		t.Fatalf("creating synthetic source table: %v", err)
	}
	defer db.ExecContext(context.Background(), "DROP TABLE IF EXISTS `"+table+"`")
	for _, row := range []struct {
		cursor    int
		uid       string
		lifecycle any
		country   string
	}{
		{cursor: 1, uid: "member-synthetic-1", lifecycle: "new", country: "SG"},
		{cursor: 2, uid: "member-synthetic-2", lifecycle: "active", country: "AU"},
		{cursor: 3, uid: "member-synthetic-3", lifecycle: nil, country: "NZ"},
	} {
		if _, err := db.ExecContext(context.Background(), "INSERT INTO `"+table+"` (cursor_id, user_id, lifecycle_value, country_value) VALUES (?, ?, ?, ?)", row.cursor, row.uid, row.lifecycle, row.country); err != nil {
			t.Fatalf("inserting constructed source row: %v", err)
		}
	}

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	attributeMapping := map[string]string{"lifecycle": "lifecycle_value", "country": "country_value"}
	source, err := management.NewMySQLAudienceSource(db, management.MySQLAudienceSourceConfig{
		Table:        table,
		UIDColumn:    "user_id",
		UIDUnique:    true,
		CursorColumn: "cursor_id",
		CursorUnique: true,
		CursorStable: true,
		Attributes:   attributeMapping,
		PageSize:     2,
	}, logger)
	if err != nil {
		t.Fatalf("building configured source: %v", err)
	}
	attributeMapping["country"] = "not_a_source_column"

	first, err := source.ReadPage(context.Background(), nil)
	if err != nil {
		t.Fatalf("reading first source page: %v", err)
	}
	if len(first.Members) != 2 || first.NextCursor == nil || *first.NextCursor != "2" {
		t.Fatalf("first page = %#v", first)
	}
	if first.Members[1].UID != "member-synthetic-2" || first.Members[1].Attributes["country"] != "AU" {
		t.Fatalf("mapped source member = %#v", first.Members[1])
	}

	second, err := source.ReadPage(context.Background(), first.NextCursor)
	if err != nil {
		t.Fatalf("reading second source page: %v", err)
	}
	if len(second.Members) != 1 || second.NextCursor != nil {
		t.Fatalf("second page = %#v", second)
	}
	if _, present := second.Members[0].Attributes["lifecycle"]; present {
		t.Fatal("nullable source value was converted to a present attribute")
	}
	if strings.Contains(logs.String(), "member-synthetic") || strings.Contains(logs.String(), "lifecycle_value") {
		t.Fatalf("source logs contain a member or mapped attribute: %s", logs.String())
	}

	view := table + "_view"
	if _, err := db.ExecContext(context.Background(), "CREATE VIEW `"+view+"` AS SELECT cursor_id, user_id, lifecycle_value, country_value FROM `"+table+"`"); err != nil {
		t.Fatalf("creating synthetic source view: %v", err)
	}
	defer db.ExecContext(context.Background(), "DROP VIEW IF EXISTS `"+view+"`")
	viewSource, err := management.NewMySQLAudienceSource(db, management.MySQLAudienceSourceConfig{
		Table:        view,
		UIDColumn:    "user_id",
		UIDUnique:    true,
		CursorColumn: "cursor_id",
		CursorUnique: true,
		CursorStable: true,
		Attributes:   map[string]string{"lifecycle": "lifecycle_value", "country": "country_value"},
		PageSize:     2,
	}, logger)
	if err != nil {
		t.Fatalf("building source from configured view: %v", err)
	}
	viewPage, err := viewSource.ReadPage(context.Background(), nil)
	if err != nil || len(viewPage.Members) != 2 {
		t.Fatalf("reading configured source view: page=%#v error=%v", viewPage, err)
	}
}
