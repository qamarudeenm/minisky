package bigquery

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serve runs one request through the shim and returns the decoded JSON body.
func serve(t *testing.T, api *API, method, path, body string) (int, map[string]interface{}) {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	decoded := map[string]interface{}{}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("%s %s returned undecodable body %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec.Code, decoded
}

// google-cloud-bigquery — and therefore dbt, bq and every SDK — posts to
// /queries and reads results back from /queries/{jobId}. Before this surface
// existed both fell through to the 404 branch.
func TestQuerySurface_PostAndGet(t *testing.T) {
	isolate(t)
	api := NewAPI(nil)

	code, response := serve(t, api, http.MethodPost,
		"/bigquery/v2/projects/demo/queries", `{"query":"SELECT 1 AS ok","useLegacySql":false}`)
	if code != http.StatusOK {
		t.Fatalf("POST /queries = %d, want 200", code)
	}
	if complete, _ := response["jobComplete"].(bool); !complete {
		t.Errorf("jobs.query must return a completed job, got %v", response["jobComplete"])
	}

	reference, ok := response["jobReference"].(map[string]interface{})
	if !ok {
		t.Fatalf("response carries no jobReference: %v", response)
	}
	jobID, _ := reference["jobId"].(string)
	if jobID == "" {
		t.Fatal("jobReference.jobId is empty")
	}

	code, results := serve(t, api, http.MethodGet,
		"/bigquery/v2/projects/demo/queries/"+jobID, "")
	if code != http.StatusOK {
		t.Fatalf("GET /queries/{jobId} = %d, want 200", code)
	}
	if kind, _ := results["kind"].(string); kind != "bigquery#getQueryResultsResponse" {
		t.Errorf("kind = %v, want bigquery#getQueryResultsResponse", results["kind"])
	}
	if complete, _ := results["jobComplete"].(bool); !complete {
		t.Error("a finished job must report jobComplete")
	}
}

func TestQuerySurface_RejectsEmptyQuery(t *testing.T) {
	isolate(t)
	api := NewAPI(nil)
	code, _ := serve(t, api, http.MethodPost, "/bigquery/v2/projects/demo/queries", `{}`)
	if code != http.StatusBadRequest {
		t.Errorf("POST /queries with no statement = %d, want 400", code)
	}
}

// The original /jobs/{jobId}/results path stays supported.
func TestQuerySurface_LegacyJobResultsPathStillWorks(t *testing.T) {
	isolate(t)
	api := NewAPI(nil)

	code, job := serve(t, api, http.MethodPost, "/bigquery/v2/projects/demo/jobs",
		`{"configuration":{"query":{"query":"SELECT 1"}},"jobReference":{"projectId":"demo","jobId":"job-legacy"}}`)
	if code != http.StatusOK {
		t.Fatalf("POST /jobs = %d, want 200", code)
	}
	if job["jobReference"] == nil {
		t.Fatalf("POST /jobs returned no jobReference: %v", job)
	}

	code, results := serve(t, api, http.MethodGet,
		"/bigquery/v2/projects/demo/jobs/job-legacy/results", "")
	if code != http.StatusOK {
		t.Fatalf("GET /jobs/{id}/results = %d, want 200", code)
	}
	if _, ok := results["schema"]; !ok {
		t.Error("results carry no schema")
	}
}

// A dataset created through the API must reach the translator, otherwise
// dataset.table references in later queries are indistinguishable from
// alias.column and are left unresolved.
func TestDatasetCreationRegistersNameWithTranslator(t *testing.T) {
	isolate(t)
	api := NewAPI(nil)

	code, _ := serve(t, api, http.MethodPost, "/bigquery/v2/projects/demo/datasets",
		`{"datasetReference":{"projectId":"demo","datasetId":"retail_raw"},"location":"US"}`)
	if code != http.StatusOK {
		t.Fatalf("POST /datasets = %d, want 200", code)
	}

	ids := api.datasetIDs("demo")
	if len(ids) != 1 || ids[0] != "retail_raw" {
		t.Fatalf("datasetIDs = %v, want [retail_raw]", ids)
	}
}

// Terraform reconciles metadata with PATCH. If the shim echoes the stored
// resource back without applying the change, every subsequent plan proposes the
// same update again and `terraform apply` never converges.
func TestPatchAppliesDatasetMetadata(t *testing.T) {
	isolate(t)
	api := NewAPI(nil)

	serve(t, api, http.MethodPost, "/bigquery/v2/projects/demo/datasets",
		`{"datasetReference":{"projectId":"demo","datasetId":"retail_raw"},"location":"US"}`)

	code, patched := serve(t, api, http.MethodPatch, "/bigquery/v2/projects/demo/datasets/retail_raw",
		`{"friendlyName":"retail raw","description":"landing layer","labels":{"layer":"bronze"}}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH /datasets = %d, want 200", code)
	}
	if patched["friendlyName"] != "retail raw" {
		t.Errorf("friendlyName = %v, want 'retail raw'", patched["friendlyName"])
	}

	_, fetched := serve(t, api, http.MethodGet, "/bigquery/v2/projects/demo/datasets/retail_raw", "")
	if fetched["friendlyName"] != "retail raw" {
		t.Errorf("friendlyName did not persist: %v", fetched["friendlyName"])
	}
	if fetched["description"] != "landing layer" {
		t.Errorf("description did not persist: %v", fetched["description"])
	}
}

func TestPatchAppliesTableMetadata(t *testing.T) {
	isolate(t)
	api := NewAPI(nil)

	serve(t, api, http.MethodPost, "/bigquery/v2/projects/demo/datasets",
		`{"datasetReference":{"projectId":"demo","datasetId":"retail_raw"},"location":"US"}`)
	serve(t, api, http.MethodPost, "/bigquery/v2/projects/demo/datasets/retail_raw/tables",
		`{"tableReference":{"projectId":"demo","datasetId":"retail_raw","tableId":"orders"}}`)

	code, _ := serve(t, api, http.MethodPatch,
		"/bigquery/v2/projects/demo/datasets/retail_raw/tables/orders",
		`{"friendlyName":"raw orders","description":"one row per order line"}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH /tables = %d, want 200 (a table update used to be rejected as 405)", code)
	}

	_, fetched := serve(t, api, http.MethodGet,
		"/bigquery/v2/projects/demo/datasets/retail_raw/tables/orders", "")
	if fetched["friendlyName"] != "raw orders" {
		t.Errorf("friendlyName did not persist: %v", fetched["friendlyName"])
	}
}
