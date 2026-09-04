package bigquery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minisky/pkg/orchestrator"
)

// isolate gives one test its own state directory. The real ~/.minisky must stay
// out of this: DuckDB admits one writer, so a running daemon would otherwise
// break the test, and the test would corrupt the developer's state.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// newIsolatedAPI builds a shim over the given home. Calling it twice with the
// same home is what a daemon restart looks like.
func newIsolatedAPI(t *testing.T, home string) *API {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return NewAPI(orchestrator.NewOperationManager())
}

func call(t *testing.T, api *API, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	var decoded map[string]interface{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}
	return rec.Code, decoded
}

// Table data already survived a restart in the DuckDB file, but the metadata
// did not: the dataset answered 404 afterwards, so Terraform read it as gone
// and planned to recreate it.
func TestDatasetMetadataSurvivesARestart(t *testing.T) {
	home := t.TempDir()

	api := newIsolatedAPI(t, home)
	code, _ := call(t, api, http.MethodPost, "/bigquery/v2/projects/probe/datasets",
		`{"datasetReference":{"projectId":"probe","datasetId":"retail"},"location":"EU","labels":{"tier":"raw"}}`)
	if code != http.StatusOK {
		t.Fatalf("create dataset = %d, want 200", code)
	}

	restarted := newIsolatedAPI(t, home)
	code, ds := call(t, restarted, http.MethodGet, "/bigquery/v2/projects/probe/datasets/retail", "")
	if code != http.StatusOK {
		t.Fatalf("GET dataset after restart = %d, want 200 — Terraform would recreate it", code)
	}
	if ds["location"] != "EU" {
		t.Errorf("location = %v after restart, want EU", ds["location"])
	}
	labels, ok := ds["labels"].(map[string]interface{})
	if !ok || labels["tier"] != "raw" {
		t.Errorf("labels = %v after restart, want tier=raw", ds["labels"])
	}
}

// A table's declared schema is the part that cannot be rebuilt from the data
// file: column descriptions and REQUIRED modes exist only in the metadata.
func TestTableSchemaSurvivesARestart(t *testing.T) {
	home := t.TempDir()

	api := newIsolatedAPI(t, home)
	call(t, api, http.MethodPost, "/bigquery/v2/projects/probe/datasets",
		`{"datasetReference":{"projectId":"probe","datasetId":"retail"}}`)
	code, _ := call(t, api, http.MethodPost, "/bigquery/v2/projects/probe/datasets/retail/tables",
		`{"tableReference":{"projectId":"probe","datasetId":"retail","tableId":"orders"},
		  "description":"one row per order",
		  "schema":{"fields":[{"name":"order_id","type":"STRING","mode":"REQUIRED","description":"primary key"}]}}`)
	if code != http.StatusOK {
		t.Fatalf("create table = %d, want 200", code)
	}

	restarted := newIsolatedAPI(t, home)
	code, table := call(t, restarted, http.MethodGet,
		"/bigquery/v2/projects/probe/datasets/retail/tables/orders", "")
	if code != http.StatusOK {
		t.Fatalf("GET table after restart = %d, want 200", code)
	}
	if table["description"] != "one row per order" {
		t.Errorf("description = %v after restart", table["description"])
	}

	schema, ok := table["schema"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema missing after restart: %v", table)
	}
	fields, _ := schema["fields"].([]interface{})
	if len(fields) != 1 {
		t.Fatalf("fields = %v, want the one declared column", fields)
	}
	field := fields[0].(map[string]interface{})
	if field["mode"] != "REQUIRED" || field["description"] != "primary key" {
		t.Errorf("column metadata lost after restart: %v", field)
	}
}

func TestDeletedDatasetStaysDeleted(t *testing.T) {
	home := t.TempDir()

	api := newIsolatedAPI(t, home)
	call(t, api, http.MethodPost, "/bigquery/v2/projects/probe/datasets",
		`{"datasetReference":{"projectId":"probe","datasetId":"scratch"}}`)
	if code, _ := call(t, api, http.MethodDelete,
		"/bigquery/v2/projects/probe/datasets/scratch", ""); code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204", code)
	}

	restarted := newIsolatedAPI(t, home)
	if code, _ := call(t, restarted, http.MethodGet,
		"/bigquery/v2/projects/probe/datasets/scratch", ""); code != http.StatusNotFound {
		t.Errorf("a deleted dataset came back after the restart (got %d)", code)
	}
}
