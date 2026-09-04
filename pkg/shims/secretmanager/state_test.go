package secretmanager

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

// A secret that comes back empty after a restart takes down every workload that
// reads it at boot, and the payload cannot be recovered from anywhere else.
func TestSecretPayloadSurvivesARestart(t *testing.T) {
	isolate(t)

	api := NewAPI(nil, nil)
	if code, _ := call(t, api, http.MethodPost, "/v1/projects/probe/secrets?secretId=db-password",
		`{"replication":{"automatic":{}},"labels":{"env":"local"}}`); code != http.StatusOK {
		t.Fatalf("create secret = %d, want 200", code)
	}
	// "cDRzc3cwcmQ=" is "p4ssw0rd".
	if code, _ := call(t, api, http.MethodPost, "/v1/projects/probe/secrets/db-password:addVersion",
		`{"payload":{"data":"cDRzc3cwcmQ="}}`); code != http.StatusOK {
		t.Fatalf("addVersion = %d, want 200", code)
	}

	restarted := NewAPI(nil, nil)
	code, got := call(t, restarted, http.MethodGet,
		"/v1/projects/probe/secrets/db-password/versions/latest:access", "")
	if code != http.StatusOK {
		t.Fatalf("access after restart = %d, want 200", code)
	}
	payload, ok := got["payload"].(map[string]interface{})
	if !ok || payload["data"] != "cDRzc3cwcmQ=" {
		t.Errorf("payload = %v after restart, want the stored secret", got["payload"])
	}

	// The secret's own metadata has to come back too, or Terraform recreates it.
	code, secret := call(t, restarted, http.MethodGet, "/v1/projects/probe/secrets/db-password", "")
	if code != http.StatusOK {
		t.Fatalf("get secret after restart = %d, want 200", code)
	}
	labels, ok := secret["labels"].(map[string]interface{})
	if !ok || labels["env"] != "local" {
		t.Errorf("labels = %v after restart", secret["labels"])
	}
}

func TestDeletedSecretStaysDeleted(t *testing.T) {
	isolate(t)

	api := NewAPI(nil, nil)
	call(t, api, http.MethodPost, "/v1/projects/probe/secrets?secretId=scratch", `{"replication":{"automatic":{}}}`)
	call(t, api, http.MethodDelete, "/v1/projects/probe/secrets/scratch", "")

	if code, _ := call(t, NewAPI(nil, nil), http.MethodGet,
		"/v1/projects/probe/secrets/scratch", ""); code != http.StatusNotFound {
		t.Errorf("a deleted secret came back after the restart (got %d)", code)
	}
}
