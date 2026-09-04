package iam

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// isolate gives one test its own state file, away from the developer's real
// ~/.minisky, which construction now reads.
func isolate(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
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

// A service account that disappears on restart takes every workload configured
// to authenticate as it, and Terraform reads the account as deleted.
func TestServiceAccountSurvivesARestart(t *testing.T) {
	home := isolate(t)

	api := NewAPI()
	code, created := call(t, api, http.MethodPost, "/v1/projects/probe/serviceAccounts",
		`{"accountId":"pipeline","serviceAccount":{"displayName":"Pipeline runner"}}`)
	if code != http.StatusOK {
		t.Fatalf("create service account = %d, want 200", code)
	}
	email, _ := created["email"].(string)
	if email == "" {
		t.Fatalf("no email in the created account: %v", created)
	}

	t.Setenv("HOME", home)
	restarted := NewAPI()
	code, got := call(t, restarted, http.MethodGet, "/v1/projects/probe/serviceAccounts/"+email, "")
	if code != http.StatusOK {
		t.Fatalf("GET service account after restart = %d, want 200", code)
	}
	if got["displayName"] != "Pipeline runner" {
		t.Errorf("displayName = %v after restart", got["displayName"])
	}
}

// Policies are what `minisky iam explain` reads, so losing them turns a
// deliberate grant into an unexplained denial.
func TestPolicySurvivesARestart(t *testing.T) {
	home := isolate(t)

	api := NewAPI()
	code, _ := call(t, api, http.MethodPost, "/v1/projects/probe:setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/storage.objectViewer",
		  "members":["serviceAccount:pipeline@probe.iam.gserviceaccount.com"]}]}}`)
	if code != http.StatusOK {
		t.Fatalf("setIamPolicy = %d, want 200", code)
	}

	t.Setenv("HOME", home)
	restarted := NewAPI()
	code, policy := call(t, restarted, http.MethodPost, "/v1/projects/probe:getIamPolicy", `{}`)
	if code != http.StatusOK {
		t.Fatalf("getIamPolicy after restart = %d, want 200", code)
	}

	bindings, _ := policy["bindings"].([]interface{})
	if len(bindings) != 1 {
		t.Fatalf("bindings = %v after restart, want the one that was set", policy["bindings"])
	}
	binding := bindings[0].(map[string]interface{})
	if binding["role"] != "roles/storage.objectViewer" {
		t.Errorf("role = %v after restart", binding["role"])
	}
}
