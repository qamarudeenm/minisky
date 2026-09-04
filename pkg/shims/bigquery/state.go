package bigquery

import (
	"log"

	"minisky/pkg/persist"
)

// ─────────────────────────────────────────────────────────────────────────────
// Metadata persistence
//
// Table *data* already survives a restart: it lives in the DuckDB file, and the
// translator rebuilds the set of dataset names from information_schema. The
// metadata around it did not. A dataset created before a restart answered 404
// afterwards even though its tables were still queryable, so Terraform read the
// dataset as gone and planned to recreate it, and every column type, label and
// description declared on a table was lost.
// ─────────────────────────────────────────────────────────────────────────────

// stateName is the file under ~/.minisky this metadata is kept in.
const stateName = "bigquery_metadata"

// persistedState is the shim's metadata as written to disk. Rows sit beside the
// Table rather than inside it: Table is also the wire format, and rows are not
// part of the BigQuery Table resource.
type persistedState struct {
	Datasets map[string]*Dataset                 `json:"datasets"`
	Tables   map[string]*Table                   `json:"tables"`
	Rows     map[string][]map[string]interface{} `json:"rows,omitempty"`
}

// restore loads previously saved metadata into the API. It runs during
// construction, before the shim serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: BigQuery] ignoring unreadable metadata: %v", err)
		return
	}

	var datasetNames []string
	for key, ds := range stored.Datasets {
		api.datasets[key] = ds
		datasetNames = append(datasetNames, ds.DatasetReference.DatasetId)
	}
	for key, t := range stored.Tables {
		t.rows = stored.Rows[key]
		api.tables[key] = t
	}

	if len(datasetNames) > 0 {
		// The SQL translator needs these names to tell `dataset.table` apart
		// from `alias.column`.
		api.backend.SetKnownDatasets(datasetNames)
		log.Printf("[Shim: BigQuery] restored %d dataset(s) and %d table(s)",
			len(stored.Datasets), len(stored.Tables))
	}
}

// saveLocked writes the metadata out. Callers hold api.mu.
func (api *API) saveLocked() {
	rows := map[string][]map[string]interface{}{}
	for key, t := range api.tables {
		if len(t.rows) > 0 {
			rows[key] = t.rows
		}
	}
	state := persistedState{Datasets: api.datasets, Tables: api.tables, Rows: rows}

	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: BigQuery] could not persist metadata: %v", err)
	}
}
