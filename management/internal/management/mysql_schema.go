package management

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type mysqlSchemaColumn struct {
	name          string
	sqlType       string
	nullable      bool
	autoIncrement bool
}

type mysqlSchemaTable struct {
	name        string
	columns     []mysqlSchemaColumn
	uniqueKeys  [][]string
	foreignKeys []mysqlSchemaForeignKey
}

type mysqlSchemaForeignKey struct {
	columns          []string
	referencedTable  string
	referencedColumn []string
	onDelete         string
}

var managementSchemaContract = []mysqlSchemaTable{
	{
		name: "gmp_management_schema_migrations",
		columns: []mysqlSchemaColumn{
			{name: "migration_id", sqlType: "varchar(128)"},
			{name: "applied_at", sqlType: "timestamp"},
		},
		uniqueKeys: [][]string{{"migration_id"}},
	},
	{
		name:       "gmp_management_write_lock",
		columns:    []mysqlSchemaColumn{{name: "lock_id", sqlType: "tinyint unsigned"}},
		uniqueKeys: [][]string{{"lock_id"}},
	},
	{
		name: "gmp_management_sequences",
		columns: []mysqlSchemaColumn{
			{name: "sequence_name", sqlType: "varchar(32)"},
			{name: "next_value", sqlType: "bigint unsigned"},
		},
		uniqueKeys: [][]string{{"sequence_name"}},
	},
	{
		name: "gmp_management_campaigns",
		columns: []mysqlSchemaColumn{
			{name: "campaign_pk", sqlType: "bigint unsigned", autoIncrement: true},
			{name: "id_hash", sqlType: "binary(32)"},
			{name: "id_value", sqlType: "longtext"},
			{name: "name", sqlType: "longtext"},
			{name: "audience_type", sqlType: "varchar(16)", nullable: true},
			{name: "audience_group_id", sqlType: "longtext", nullable: true},
			{name: "audience_bundle_id", sqlType: "longtext", nullable: true},
		},
		uniqueKeys: [][]string{{"campaign_pk"}, {"id_hash"}},
	},
	{
		name: "gmp_management_treatments",
		columns: []mysqlSchemaColumn{
			{name: "treatment_pk", sqlType: "bigint unsigned", autoIncrement: true},
			{name: "campaign_pk", sqlType: "bigint unsigned"},
			{name: "id_hash", sqlType: "binary(32)"},
			{name: "id_value", sqlType: "longtext"},
			{name: "name", sqlType: "longtext"},
			{name: "action_id", sqlType: "longtext"},
			{name: "slot_id", sqlType: "longtext", nullable: true},
		},
		uniqueKeys: [][]string{{"treatment_pk"}, {"campaign_pk", "id_hash"}},
		foreignKeys: []mysqlSchemaForeignKey{{
			columns:          []string{"campaign_pk"},
			referencedTable:  "gmp_management_campaigns",
			referencedColumn: []string{"campaign_pk"},
			onDelete:         "CASCADE",
		}},
	},
	{
		name: "gmp_management_flows",
		columns: []mysqlSchemaColumn{
			{name: "campaign_pk", sqlType: "bigint unsigned"},
			{name: "flow_id", sqlType: "longtext"},
		},
		uniqueKeys: [][]string{{"campaign_pk"}},
		foreignKeys: []mysqlSchemaForeignKey{{
			columns:          []string{"campaign_pk"},
			referencedTable:  "gmp_management_campaigns",
			referencedColumn: []string{"campaign_pk"},
			onDelete:         "CASCADE",
		}},
	},
	{
		name: "gmp_management_flow_entries",
		columns: []mysqlSchemaColumn{
			{name: "entry_pk", sqlType: "bigint unsigned", autoIncrement: true},
			{name: "campaign_pk", sqlType: "bigint unsigned"},
			{name: "entry_order", sqlType: "int unsigned"},
			{name: "group_id", sqlType: "longtext"},
			{name: "kind", sqlType: "varchar(16)"},
		},
		uniqueKeys: [][]string{{"entry_pk"}, {"campaign_pk", "entry_order"}},
		foreignKeys: []mysqlSchemaForeignKey{{
			columns:          []string{"campaign_pk"},
			referencedTable:  "gmp_management_campaigns",
			referencedColumn: []string{"campaign_pk"},
			onDelete:         "CASCADE",
		}},
	},
	{
		name: "gmp_management_flow_entry_treatments",
		columns: []mysqlSchemaColumn{
			{name: "entry_pk", sqlType: "bigint unsigned"},
			{name: "treatment_order", sqlType: "int unsigned"},
			{name: "treatment_id", sqlType: "longtext"},
		},
		uniqueKeys: [][]string{{"entry_pk", "treatment_order"}},
		foreignKeys: []mysqlSchemaForeignKey{{
			columns:          []string{"entry_pk"},
			referencedTable:  "gmp_management_flow_entries",
			referencedColumn: []string{"entry_pk"},
			onDelete:         "CASCADE",
		}},
	},
	{
		name: "gmp_management_flow_edges",
		columns: []mysqlSchemaColumn{
			{name: "edge_pk", sqlType: "bigint unsigned", autoIncrement: true},
			{name: "campaign_pk", sqlType: "bigint unsigned"},
			{name: "edge_order", sqlType: "int unsigned"},
			{name: "id_value", sqlType: "longtext"},
			{name: "from_treatment_id", sqlType: "longtext"},
			{name: "to_treatment_id", sqlType: "longtext"},
			{name: "condition_label", sqlType: "longtext"},
		},
		uniqueKeys: [][]string{{"edge_pk"}, {"campaign_pk", "edge_order"}},
		foreignKeys: []mysqlSchemaForeignKey{{
			columns:          []string{"campaign_pk"},
			referencedTable:  "gmp_management_campaigns",
			referencedColumn: []string{"campaign_pk"},
			onDelete:         "CASCADE",
		}},
	},
	{
		name: "gmp_management_audience_references",
		columns: []mysqlSchemaColumn{
			{name: "reference_pk", sqlType: "bigint unsigned", autoIncrement: true},
			{name: "identity_hash", sqlType: "binary(32)"},
			{name: "reference_type", sqlType: "varchar(16)"},
			{name: "bundle_id", sqlType: "longtext"},
			{name: "group_id", sqlType: "longtext"},
			{name: "name", sqlType: "longtext"},
			{name: "bundle_name", sqlType: "longtext"},
			{name: "available", sqlType: "tinyint(1)"},
		},
		uniqueKeys: [][]string{{"reference_pk"}, {"identity_hash"}},
	},
	{
		name: "gmp_management_execution_records",
		columns: []mysqlSchemaColumn{
			{name: "execution_hash", sqlType: "binary(32)"},
			{name: "execution_key", sqlType: "longtext"},
			{name: "campaign_id", sqlType: "longtext"},
			{name: "treatment_id", sqlType: "longtext"},
			{name: "uid", sqlType: "longtext"},
			{name: "action_id", sqlType: "longtext"},
			{name: "placements", sqlType: "json"},
			{name: "state", sqlType: "varchar(32)"},
			{name: "attempts", sqlType: "int"},
			{name: "external_ref", sqlType: "longtext"},
		},
		uniqueKeys: [][]string{{"execution_hash"}},
	},
	{
		name: "gmp_management_execution_receipts",
		columns: []mysqlSchemaColumn{
			{name: "execution_hash", sqlType: "binary(32)"},
			{name: "arrived", sqlType: "tinyint(1)"},
			{name: "outcome", sqlType: "longtext"},
			{name: "external_ref", sqlType: "longtext"},
			{name: "attributes", sqlType: "json"},
		},
		uniqueKeys: [][]string{{"execution_hash"}},
		foreignKeys: []mysqlSchemaForeignKey{{
			columns:          []string{"execution_hash"},
			referencedTable:  "gmp_management_execution_records",
			referencedColumn: []string{"execution_hash"},
			onDelete:         "CASCADE",
		}},
	},
}

func verifyManagementSchema(ctx context.Context, db *sql.DB) error {
	for _, table := range managementSchemaContract {
		if err := verifyMySQLTable(ctx, db, table); err != nil {
			return err
		}
	}
	return nil
}

func verifyMySQLTable(ctx context.Context, db *sql.DB, contract mysqlSchemaTable) error {
	var engine sql.NullString
	if err := db.QueryRowContext(ctx,
		"SELECT ENGINE FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		contract.name,
	).Scan(&engine); err != nil {
		return fmt.Errorf("reading engine for %s: %w", contract.name, err)
	}
	if !engine.Valid || !strings.EqualFold(engine.String, "InnoDB") {
		return fmt.Errorf("table %s must use InnoDB", contract.name)
	}

	columns, err := readMySQLColumns(ctx, db, contract.name)
	if err != nil {
		return fmt.Errorf("reading columns for %s: %w", contract.name, err)
	}
	for _, expected := range contract.columns {
		actual, ok := columns[expected.name]
		if !ok {
			return fmt.Errorf("table %s is missing column %s", contract.name, expected.name)
		}
		if !strings.EqualFold(actual.sqlType, expected.sqlType) || actual.nullable != expected.nullable || actual.autoIncrement != expected.autoIncrement {
			return fmt.Errorf("table %s column %s does not match the required type contract", contract.name, expected.name)
		}
	}

	indexes, err := readMySQLIndexes(ctx, db, contract.name)
	if err != nil {
		return fmt.Errorf("reading indexes for %s: %w", contract.name, err)
	}
	for _, expected := range contract.uniqueKeys {
		if !hasUniqueIndex(indexes, expected) {
			return fmt.Errorf("table %s is missing a required unique key", contract.name)
		}
	}
	foreignKeys, err := readMySQLForeignKeys(ctx, db, contract.name)
	if err != nil {
		return fmt.Errorf("reading foreign keys for %s: %w", contract.name, err)
	}
	if len(foreignKeys) != len(contract.foreignKeys) {
		return fmt.Errorf("table %s does not match the required foreign key contract", contract.name)
	}
	for _, expected := range contract.foreignKeys {
		if !hasForeignKey(foreignKeys, expected) {
			return fmt.Errorf("table %s does not match the required foreign key contract", contract.name)
		}
	}
	return nil
}

type mysqlColumnInfo struct {
	sqlType       string
	nullable      bool
	autoIncrement bool
}

func readMySQLColumns(ctx context.Context, db *sql.DB, tableName string) (map[string]mysqlColumnInfo, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, EXTRA FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		tableName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]mysqlColumnInfo)
	for rows.Next() {
		var name, columnType, nullable, extra string
		if err := rows.Scan(&name, &columnType, &nullable, &extra); err != nil {
			return nil, err
		}
		columns[name] = mysqlColumnInfo{
			sqlType:       strings.ToLower(columnType),
			nullable:      strings.EqualFold(nullable, "YES"),
			autoIncrement: strings.Contains(strings.ToLower(extra), "auto_increment"),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(columns) == 0 {
		return nil, errors.New("table has no visible columns")
	}
	return columns, nil
}

type mysqlIndexInfo struct {
	unique     bool
	fullColumn bool
	columns    []string
}

func readMySQLIndexes(ctx context.Context, db *sql.DB, tableName string) (map[string]mysqlIndexInfo, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME, SUB_PART FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? ORDER BY INDEX_NAME, SEQ_IN_INDEX",
		tableName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	indexes := make(map[string]mysqlIndexInfo)
	for rows.Next() {
		var name, column string
		var nonUnique, sequence int
		var subPart sql.NullInt64
		if err := rows.Scan(&name, &nonUnique, &sequence, &column, &subPart); err != nil {
			return nil, err
		}
		index := indexes[name]
		if sequence != len(index.columns)+1 {
			return nil, errors.New("index column sequence is not contiguous")
		}
		index.unique = nonUnique == 0
		index.fullColumn = index.fullColumn || len(index.columns) == 0
		if subPart.Valid {
			index.fullColumn = false
		}
		index.columns = append(index.columns, column)
		indexes[name] = index
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(indexes) == 0 {
		return nil, errors.New("table has no visible indexes")
	}
	return indexes, nil
}

func hasUniqueIndex(indexes map[string]mysqlIndexInfo, columns []string) bool {
	for _, index := range indexes {
		if !index.unique || !index.fullColumn || len(index.columns) != len(columns) {
			continue
		}
		matches := true
		for position := range columns {
			if !strings.EqualFold(index.columns[position], columns[position]) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

type mysqlForeignKeyInfo struct {
	columns          []string
	referencedTable  string
	referencedColumn []string
	onDelete         string
}

func readMySQLForeignKeys(ctx context.Context, db *sql.DB, tableName string) ([]mysqlForeignKeyInfo, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT k.CONSTRAINT_NAME, k.COLUMN_NAME, k.REFERENCED_TABLE_NAME, k.REFERENCED_COLUMN_NAME, k.ORDINAL_POSITION, r.DELETE_RULE "+
			"FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE AS k "+
			"JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS AS r "+
			"ON r.CONSTRAINT_SCHEMA = k.CONSTRAINT_SCHEMA AND r.CONSTRAINT_NAME = k.CONSTRAINT_NAME AND r.TABLE_NAME = k.TABLE_NAME "+
			"WHERE k.CONSTRAINT_SCHEMA = DATABASE() AND k.TABLE_NAME = ? AND k.REFERENCED_TABLE_NAME IS NOT NULL "+
			"ORDER BY k.CONSTRAINT_NAME, k.ORDINAL_POSITION",
		tableName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	foreignKeys := make([]mysqlForeignKeyInfo, 0)
	byName := make(map[string]int)
	for rows.Next() {
		var name, column, referencedTable, referencedColumn, deleteRule string
		var position int
		if err := rows.Scan(&name, &column, &referencedTable, &referencedColumn, &position, &deleteRule); err != nil {
			return nil, err
		}
		index, exists := byName[name]
		if !exists {
			index = len(foreignKeys)
			byName[name] = index
			foreignKeys = append(foreignKeys, mysqlForeignKeyInfo{
				referencedTable: referencedTable,
				onDelete:        strings.ToUpper(deleteRule),
			})
		}
		foreignKey := &foreignKeys[index]
		if position != len(foreignKey.columns)+1 || !strings.EqualFold(foreignKey.referencedTable, referencedTable) || !strings.EqualFold(foreignKey.onDelete, deleteRule) {
			return nil, errors.New("foreign key metadata is inconsistent")
		}
		foreignKey.columns = append(foreignKey.columns, column)
		foreignKey.referencedColumn = append(foreignKey.referencedColumn, referencedColumn)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return foreignKeys, nil
}

func hasForeignKey(actual []mysqlForeignKeyInfo, expected mysqlSchemaForeignKey) bool {
	for _, candidate := range actual {
		if !strings.EqualFold(candidate.referencedTable, expected.referencedTable) || !strings.EqualFold(candidate.onDelete, expected.onDelete) ||
			!equalColumnList(candidate.columns, expected.columns) || !equalColumnList(candidate.referencedColumn, expected.referencedColumn) {
			continue
		}
		return true
	}
	return false
}

func equalColumnList(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !strings.EqualFold(left[index], right[index]) {
			return false
		}
	}
	return true
}
