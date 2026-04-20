//go:build cgo

package bigquery

// ─────────────────────────────────────────────────────────────────────────────
// Phase 5b — DuckDB Integration
//
// This file wires the BigQuery shim to a real local DuckDB instance.
// When enabled via MINISKY_BQ_BACKEND=duckdb, jobs.insert calls execute the
// SQL query against an embedded DuckDB database instead of returning empty results.
//
// Prerequisites:
//   - Add dependency: go get github.com/marcboeker/go-duckdb
//   - CGO must be enabled (DuckDB requires it): CGO_ENABLED=1
//
// Enable with: export MINISKY_BQ_BACKEND=duckdb
//
// Table DDL Mapping:
//   When a BigQuery table is created with a schema, MiniSky automatically
//   creates a matching DuckDB table using the mapped types below.
//
// BigQuery → DuckDB Type Mapping:
//   STRING    → VARCHAR
//   INTEGER   → BIGINT
//   FLOAT     → DOUBLE
//   BOOLEAN   → BOOLEAN
//   TIMESTAMP → TIMESTAMP WITH TIME ZONE
//   DATE      → DATE
//   RECORD    → STRUCT (nested)
//   BYTES     → BLOB
// ─────────────────────────────────────────────────────────────────────────────

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	_ "github.com/marcboeker/go-duckdb"
)

// DuckDBBackend manages an embedded DuckDB database for BigQuery query execution.
type DuckDBBackend struct {
	enabled bool
	dbPath  string
	db      *sql.DB
}

// NewDuckDBBackend returns a DuckDBBackend. Only active when
// MINISKY_BQ_BACKEND=duckdb is set.
func NewDuckDBBackend() *DuckDBBackend {
	enabled := strings.EqualFold(os.Getenv("MINISKY_BQ_BACKEND"), "duckdb")
	dbPath := os.Getenv("MINISKY_DUCKDB_PATH")
	if dbPath == "" {
		dbPath = ".minisky/data/bigquery.duckdb"
	}

	b := &DuckDBBackend{enabled: enabled, dbPath: dbPath}

	if enabled {
		log.Printf("[DuckDBBackend] ✅ DuckDB integration ENABLED — queries will execute against %s", dbPath)
		if err := b.init(); err != nil {
			log.Printf("[DuckDBBackend] WARNING: DuckDB init failed: %v. Falling back to empty results.", err)
			b.enabled = false
		}
	}
	return b
}

// Enabled reports whether DuckDB backend is active.
func (d *DuckDBBackend) Enabled() bool { return d.enabled }

// SetEnabled toggles the DuckDB backend dynamically.
func (d *DuckDBBackend) SetEnabled(enabled bool) error {
	d.enabled = enabled
	if enabled {
		log.Printf("[DuckDBBackend] dynamically ENABLED via UI")
		return d.init()
	}
	log.Printf("[DuckDBBackend] dynamically DISABLED via UI")
	// If it was already opened, you would close d.db here.
	return nil
}

// init opens or creates the DuckDB database file.
// Uncomment the sql.Open call once go-duckdb is added to go.mod.
func (d *DuckDBBackend) init() error {
	// Ensure the data directory exists
	dir := ".minisky/data"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create data directory: %w", err)
	}

	db, err := sql.Open("duckdb", d.dbPath)
	if err != nil {
		return fmt.Errorf("open duckdb: %w", err)
	}
	d.db = db
	return db.Ping()
}

// ExecuteQuery runs a BigQuery StandardSQL query and returns rows as a slice of maps.
// The query is first translated from BigQuery SQL dialect to DuckDB SQL.
func (d *DuckDBBackend) ExecuteQuery(query string) ([]map[string]interface{}, error) {
	if !d.enabled {
		return nil, fmt.Errorf("duckdb backend not enabled")
	}
	translated := translateBQtoDuck(query)
	log.Printf("[DuckDBBackend] Executing: %s", translated)

	rows, err := d.db.Query(translated)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func scanRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	for rows.Next() {
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}
		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]interface{})
		for i, colName := range cols {
			val := columnPointers[i].(*interface{})
			// Convert bytes arrays into strings if possible
			v := *val
			if b, ok := v.([]byte); ok {
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = v
			}
		}
		results = append(results, rowMap)
	}
	return results, nil
}

// LoadData ingests a file or URL into a DuckDB table.
func (d *DuckDBBackend) LoadData(project, dataset, table, sourceURI, format string) error {
	if !d.enabled {
		return fmt.Errorf("duckdb backend not enabled")
	}
	tableName := fmt.Sprintf("%s__%s", dataset, table)
	
	var query string
	format = strings.ToUpper(format)
	switch format {
	case "CSV":
		query = fmt.Sprintf("CREATE OR REPLACE TABLE \"%s\" AS SELECT * FROM read_csv_auto('%s')", tableName, sourceURI)
	case "JSON", "NEWLINE_DELIMITED_JSON":
		query = fmt.Sprintf("CREATE OR REPLACE TABLE \"%s\" AS SELECT * FROM read_json_auto('%s')", tableName, sourceURI)
	case "PARQUET":
		query = fmt.Sprintf("CREATE OR REPLACE TABLE \"%s\" AS SELECT * FROM read_parquet('%s')", tableName, sourceURI)
	default:
		return fmt.Errorf("unsupported format for DuckDB load: %s", format)
	}

	log.Printf("[DuckDBBackend] Loading data: %s", query)
	_, err := d.db.Exec(query)
	return err
}

// CreateTable creates a DuckDB table from a BigQuery TableSchema.
func (d *DuckDBBackend) CreateTable(project, dataset, table string, schema *TableSchema) error {
	if !d.enabled || schema == nil {
		return nil
	}
	ddl := buildDDL(project, dataset, table, schema)
	log.Printf("[DuckDBBackend] Creating table: %s", ddl)

	_, err := d.db.Exec(ddl)
	if err != nil {
		log.Printf("[DuckDBBackend] Error creating table: %v", err)
	}
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// SQL Translation helpers
// ─────────────────────────────────────────────────────────────────────────────

// bqToDuckTypeMap maps BigQuery field types to DuckDB equivalents.
var bqToDuckTypeMap = map[string]string{
	"STRING":    "VARCHAR",
	"BYTES":     "BLOB",
	"INTEGER":   "BIGINT",
	"INT64":     "BIGINT",
	"FLOAT":     "DOUBLE",
	"FLOAT64":   "DOUBLE",
	"NUMERIC":   "DECIMAL(38,9)",
	"BIGNUMERIC": "DECIMAL(76,38)",
	"BOOLEAN":   "BOOLEAN",
	"BOOL":      "BOOLEAN",
	"TIMESTAMP": "TIMESTAMPTZ",
	"DATE":      "DATE",
	"TIME":      "TIME",
	"DATETIME":  "TIMESTAMP",
	"GEOGRAPHY": "VARCHAR", // approximate — DuckDB lacks native GEOGRAPHY
	"JSON":      "JSON",
	"RECORD":    "STRUCT", // nested — requires recursive handling
	"STRUCT":    "STRUCT",
}

// translateBQtoDuck does lightweight BigQuery → DuckDB SQL dialect conversion.
// Handles the most common divergences between the two dialects.
func translateBQtoDuck(bqSQL string) string {
	s := bqSQL

	// Backtick → double-quote identifiers  (`project.dataset.table` → "project.dataset.table")
	s = strings.ReplaceAll(s, "`", "\"")

	// CURRENT_TIMESTAMP() → CURRENT_TIMESTAMP
	s = strings.ReplaceAll(s, "CURRENT_TIMESTAMP()", "CURRENT_TIMESTAMP")

	// TIMESTAMP_TRUNC(x, DAY) → DATE_TRUNC('day', x)  (basic form)
	// Note: Full translation requires a proper SQL parser; this handles the common case.
	if strings.Contains(s, "TIMESTAMP_TRUNC") {
		log.Printf("[DuckDBBackend] WARN: TIMESTAMP_TRUNC requires manual translation — result may vary")
	}

	// SAFE_DIVIDE(a, b) → CASE WHEN b = 0 THEN NULL ELSE a / b END
	if strings.Contains(s, "SAFE_DIVIDE") {
		log.Printf("[DuckDBBackend] WARN: SAFE_DIVIDE not auto-translated — consider rewriting query")
	}

	// dataset.table → dataset__table (DuckDB internal mapping)
	// Supports project.dataset.table (3 segments) and dataset.table (2 segments)
	// 1. project.dataset.table -> dataset.table
	projectRe := regexp.MustCompile(`([a-zA-Z0-9_-]+)\.([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)`)
	s = projectRe.ReplaceAllString(s, "$2.$3")

	// 2. dataset.table -> dataset__table
	datasetRe := regexp.MustCompile(`([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)`)
	s = datasetRe.ReplaceAllString(s, "${1}__$2")

	return s
}

// buildDDL generates a CREATE TABLE IF NOT EXISTS statement for DuckDB.
func buildDDL(project, dataset, table string, schema *TableSchema) string {
	// DuckDB table name: dataset__table (project is ignored in local context)
	tableName := fmt.Sprintf("%s__%s", dataset, table)
	cols := make([]string, 0, len(schema.Fields))
	for _, f := range schema.Fields {
		duckType := bqToDuckTypeMap[strings.ToUpper(f.Type)]
		if duckType == "" {
			duckType = "VARCHAR"
		}
		nullable := ""
		if strings.ToUpper(f.Mode) == "REQUIRED" {
			nullable = " NOT NULL"
		}
		cols = append(cols, fmt.Sprintf("  \"%s\" %s%s", f.Name, duckType, nullable))
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS \"%s\" (\n%s\n);",
		tableName, strings.Join(cols, ",\n"))
}
