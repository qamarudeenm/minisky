package cloudsql

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minisky/pkg/orchestrator"
)

func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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

// A Cloud SQL instance is a Postgres container holding the user's database. The
// container and its data outlive the daemon; losing the instance record left
// the database running with nothing describing it, and Terraform planning to
// create one that is already there.
func TestInstanceAndDatabaseSurviveARestart(t *testing.T) {
	isolate(t)
	opMgr := orchestrator.NewOperationManager()

	api := NewAPI(opMgr, nil)
	code, _ := call(t, api, http.MethodPost, "/v1/projects/probe/instances",
		`{"name":"analytics-db","databaseVersion":"POSTGRES_15","region":"us-central1",
		  "settings":{"tier":"db-f1-micro"}}`)
	if code != http.StatusOK {
		t.Fatalf("create instance = %d, want 200", code)
	}

	restarted := NewAPI(opMgr, nil)
	code, inst := call(t, restarted, http.MethodGet, "/v1/projects/probe/instances/analytics-db", "")
	if code != http.StatusOK {
		t.Fatalf("GET instance after restart = %d, want 200", code)
	}
	if inst["databaseVersion"] != "POSTGRES_15" {
		t.Errorf("databaseVersion = %v after restart", inst["databaseVersion"])
	}
	if inst["region"] != "us-central1" {
		t.Errorf("region = %v after restart", inst["region"])
	}
}
