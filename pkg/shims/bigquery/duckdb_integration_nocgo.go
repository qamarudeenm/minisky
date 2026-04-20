//go:build !cgo

package bigquery

import (
	"fmt"
)

// DuckDBBackend is a mock version for platforms without CGO (like Windows native build).
type DuckDBBackend struct {
	enabled bool
}

func NewDuckDBBackend() *DuckDBBackend {
	return &DuckDBBackend{enabled: false}
}

func (d *DuckDBBackend) Enabled() bool { return false }

func (d *DuckDBBackend) SetEnabled(enabled bool) error {
	if enabled {
		return fmt.Errorf("DuckDB backend requires CGO and is not available on this platform/build")
	}
	return nil
}

func (d *DuckDBBackend) ExecuteQuery(query string) ([]map[string]interface{}, error) {
	return nil, fmt.Errorf("duckdb backend requires CGO_ENABLED=1")
}

func (d *DuckDBBackend) LoadData(project, dataset, table, sourceURI, format string) error {
	return fmt.Errorf("duckdb backend requires CGO_ENABLED=1")
}

func (d *DuckDBBackend) CreateTable(project, dataset, table string, schema *TableSchema) error {
	return nil
}
