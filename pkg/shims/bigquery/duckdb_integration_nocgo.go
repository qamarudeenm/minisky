//go:build !cgo

package bigquery

import (
	"fmt"
	"log"
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
	log.Printf("[BigQuery] WARNING: SQL execution is disabled in this CGO-less build.")
	return nil, fmt.Errorf("SQL execution requires the CGO-enabled version of MiniSky")
}

func (d *DuckDBBackend) CreateTable(project, dataset, table string, schema *TableSchema) error {
	return nil
}
