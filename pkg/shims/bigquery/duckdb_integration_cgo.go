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
	"minisky/pkg/config"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "github.com/marcboeker/go-duckdb"
)

// DuckDBBackend manages an embedded DuckDB database for BigQuery query execution.
type DuckDBBackend struct {
	enabled bool
	dbPath  string
	db      *sql.DB

	// datasetsMu guards knownDatasets, the set of dataset names the shim has
	// seen. The SQL translator uses it to tell `dataset.table` (a relation to
	// rewrite) apart from `alias.column` (which must be left alone).
	datasetsMu    sync.RWMutex
	knownDatasets map[string]bool
}

// NewDuckDBBackend returns a DuckDBBackend. Only active when
// MINISKY_BQ_BACKEND=duckdb is set.
func NewDuckDBBackend() *DuckDBBackend {
	enabled := strings.EqualFold(os.Getenv("MINISKY_BQ_BACKEND"), "duckdb")
	dbPath := os.Getenv("MINISKY_DUCKDB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join(config.GetMiniskyDir(), "data", "bigquery.duckdb")
	}

	b := &DuckDBBackend{enabled: enabled, dbPath: dbPath, knownDatasets: map[string]bool{}}

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
	dir := filepath.Join(config.GetMiniskyDir(), "data")
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

// SetKnownDatasets records the datasets the shim currently knows about, so the
// SQL translator can resolve two-part `dataset.table` references.
func (d *DuckDBBackend) SetKnownDatasets(datasets []string) {
	d.datasetsMu.Lock()
	defer d.datasetsMu.Unlock()
	for _, name := range datasets {
		if name != "" {
			d.knownDatasets[name] = true
		}
	}
}

// datasetSet is the union of the datasets registered through the API and the
// prefixes of tables already materialised in DuckDB. The second source keeps
// queries working after a restart, when the shim's in-memory dataset registry
// is empty but the database file still holds the tables.
func (d *DuckDBBackend) datasetSet() map[string]bool {
	known := map[string]bool{}

	d.datasetsMu.RLock()
	for name := range d.knownDatasets {
		known[name] = true
	}
	d.datasetsMu.RUnlock()

	if d.db != nil {
		rows, err := d.db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema = 'main'`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					continue
				}
				if dataset, _, ok := splitDuckTableName(name); ok {
					known[dataset] = true
				}
			}
		}
	}
	return known
}

// ExecuteQuery runs a BigQuery StandardSQL query and returns rows as a slice of maps.
// The query is first translated from BigQuery SQL dialect to DuckDB SQL.
func (d *DuckDBBackend) ExecuteQuery(query string) ([]map[string]interface{}, error) {
	result, err := d.ExecuteQueryWithSchema(query)
	if err != nil {
		return nil, err
	}
	return result.Rows, nil
}

// ExecuteQueryWithSchema runs a query and returns its rows together with the
// ordered, typed column list DuckDB reported.
func (d *DuckDBBackend) ExecuteQueryWithSchema(query string) (*QueryResult, error) {
	if !d.enabled {
		return nil, fmt.Errorf("duckdb backend not enabled")
	}
	translated := translateBQtoDuckWithDatasets(query, d.datasetSet())
	log.Printf("[DuckDBBackend] Executing: %s", translated)

	rows, err := d.db.Query(translated)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanResult(rows)
}

// scanResult reads every row, preserving column order and mapping DuckDB
// column types to their BigQuery equivalents.
func scanResult(rows *sql.Rows) (*QueryResult, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}

	result := &QueryResult{Columns: make([]QueryColumn, len(cols))}
	for i, name := range cols {
		bqType := "STRING"
		if i < len(types) {
			bqType = duckToBQType(types[i].DatabaseTypeName())
		}
		result.Columns[i] = QueryColumn{Name: name, Type: bqType}
	}

	for rows.Next() {
		values := make([]interface{}, len(cols))
		pointers := make([]interface{}, len(cols))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(cols))
		for i, name := range cols {
			value := *(pointers[i].(*interface{}))
			if b, ok := value.([]byte); ok {
				value = string(b)
			}
			row[name] = value
		}
		result.Rows = append(result.Rows, row)
	}
	return result, rows.Err()
}

// ListTables reports every table in the database, with its schema translated
// back into BigQuery types. It is how tables created by a query job (dbt models,
// CREATE TABLE AS ...) become visible to the metadata API.
func (d *DuckDBBackend) ListTables() ([]BackendTable, error) {
	if !d.enabled || d.db == nil {
		return nil, fmt.Errorf("duckdb backend not enabled")
	}

	rows, err := d.db.Query(`
		SELECT table_name, column_name, data_type, is_nullable, ordinal_position
		FROM information_schema.columns
		WHERE table_schema = 'main'
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	schemas := map[string]*TableSchema{}
	for rows.Next() {
		var tableName, columnName, dataType, isNullable string
		var position int
		if err := rows.Scan(&tableName, &columnName, &dataType, &isNullable, &position); err != nil {
			return nil, err
		}
		schema, ok := schemas[tableName]
		if !ok {
			schema = &TableSchema{}
			schemas[tableName] = schema
		}
		mode := "NULLABLE"
		if strings.EqualFold(isNullable, "NO") {
			mode = "REQUIRED"
		}
		schema.Fields = append(schema.Fields, FieldSchema{
			Name: columnName,
			Type: duckToBQType(dataType),
			Mode: mode,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)

	tables := make([]BackendTable, 0, len(names))
	for _, name := range names {
		dataset, table, ok := splitDuckTableName(name)
		if !ok {
			continue
		}
		tables = append(tables, BackendTable{Dataset: dataset, Table: table, Schema: schemas[name]})
	}
	return tables, nil
}

// DropTable removes a table from DuckDB, so deleting it through the metadata
// API does not leave its data behind.
func (d *DuckDBBackend) DropTable(dataset, table string) error {
	if !d.enabled || d.db == nil {
		return nil
	}
	_, err := d.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %q", duckTableName(dataset, table)))
	return err
}

// LoadData ingests a file or URL into a DuckDB table.
func (d *DuckDBBackend) LoadData(project, dataset, table, sourceURI, format string) error {
	if !d.enabled {
		return fmt.Errorf("duckdb backend not enabled")
	}
	d.SetKnownDatasets([]string{dataset})
	tableName := duckTableName(dataset, table)

	var query string
	format = strings.ToUpper(format)

	// Convert Windows path separators to forward slashes to prevent SQL escape sequence errors
	safeURI := filepath.ToSlash(sourceURI)

	switch format {
	case "CSV":
		query = fmt.Sprintf("CREATE OR REPLACE TABLE \"%s\" AS SELECT * FROM read_csv_auto('%s')", tableName, safeURI)
	case "JSON", "NEWLINE_DELIMITED_JSON":
		query = fmt.Sprintf("CREATE OR REPLACE TABLE \"%s\" AS SELECT * FROM read_json_auto('%s')", tableName, safeURI)
	case "PARQUET":
		query = fmt.Sprintf("CREATE OR REPLACE TABLE \"%s\" AS SELECT * FROM read_parquet('%s')", tableName, safeURI)
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
	d.SetKnownDatasets([]string{dataset})
	ddl := buildDDL(project, dataset, table, schema)
	log.Printf("[DuckDBBackend] Creating table: %s", ddl)

	_, err := d.db.Exec(ddl)
	if err != nil {
		log.Printf("[DuckDBBackend] Error creating table: %v", err)
	}
	return err
}

// buildDDL generates a CREATE TABLE IF NOT EXISTS statement for DuckDB.
func buildDDL(project, dataset, table string, schema *TableSchema) string {
	// DuckDB table name: dataset__table (project is ignored in local context)
	tableName := duckTableName(dataset, table)
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
