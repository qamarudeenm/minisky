package scheduler

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

// A restored job that is not put back on the cron is the worst outcome: it reads
// back as ENABLED from the API while never firing again, so nothing looks wrong
// until the work silently stops happening.
func TestRestoredJobIsRescheduled(t *testing.T) {
	isolate(t)

	const collection = "/v1/projects/probe/locations/us-central1/jobs"
	const name = "projects/probe/locations/us-central1/jobs/nightly"

	api := NewAPI(nil)
	code, _ := call(t, api, http.MethodPost, collection,
		`{"name":"nightly","schedule":"0 2 * * *",
		  "httpTarget":{"uri":"http://localhost:1/noop","httpMethod":"GET"}}`)
	if code != http.StatusOK {
		t.Fatalf("create job = %d, want 200", code)
	}

	restarted := NewAPI(nil)
	code, job := call(t, restarted, http.MethodGet, collection+"/nightly", "")
	if code != http.StatusOK {
		t.Fatalf("GET job after restart = %d, want 200", code)
	}
	if job["schedule"] != "0 2 * * *" {
		t.Errorf("schedule = %v after restart", job["schedule"])
	}

	restarted.mu.RLock()
	_, scheduled := restarted.cronIDs[name]
	restarted.mu.RUnlock()
	if !scheduled {
		t.Error("the restored job has no cron entry; it would read as ENABLED and never fire")
	}
}

// A paused job must stay paused and stay off the cron, or a restart quietly
// resumes work the user deliberately stopped.
func TestRestoredPausedJobIsNotRescheduled(t *testing.T) {
	isolate(t)

	const collection = "/v1/projects/probe/locations/us-central1/jobs"
	const name = "projects/probe/locations/us-central1/jobs/nightly"

	api := NewAPI(nil)
	call(t, api, http.MethodPost, collection,
		`{"name":"nightly","schedule":"0 2 * * *",
		  "httpTarget":{"uri":"http://localhost:1/noop","httpMethod":"GET"}}`)
	if code, _ := call(t, api, http.MethodPost, collection+"/nightly:pause", ""); code != http.StatusOK {
		t.Fatalf("pause = %d, want 200", code)
	}

	restarted := NewAPI(nil)
	restarted.mu.RLock()
	_, scheduled := restarted.cronIDs[name]
	restarted.mu.RUnlock()
	if scheduled {
		t.Error("a paused job was put back on the cron by the restart")
	}

	code, job := call(t, restarted, http.MethodGet, collection+"/nightly", "")
	if code != http.StatusOK {
		t.Fatalf("GET job after restart = %d, want 200", code)
	}
	if job["state"] != "PAUSED" {
		t.Errorf("state = %v after restart, want PAUSED", job["state"])
	}
}
