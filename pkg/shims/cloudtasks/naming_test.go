package cloudtasks

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

// queues.get is how Terraform reads a queue back. There was no handler for it,
// so a queue that had just been created answered 404 while listing correctly.
func TestQueueCanBeReadBackIndividually(t *testing.T) {
	isolate(t)

	const collection = "/v2/projects/probe/locations/us-central1/queues"
	api := NewAPI()

	if code, _ := call(t, api, http.MethodPost, collection,
		`{"name":"projects/probe/locations/us-central1/queues/emails"}`); code != http.StatusOK {
		t.Fatalf("create = %d, want 200", code)
	}

	code, queue := call(t, api, http.MethodGet, collection+"/emails", "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d, want 200", code)
	}
	if queue["name"] != "projects/probe/locations/us-central1/queues/emails" {
		t.Errorf("name = %v", queue["name"])
	}
	if queue["state"] != "RUNNING" {
		t.Errorf("state = %v, want RUNNING", queue["state"])
	}

	if code, _ := call(t, api, http.MethodGet, collection+"/nonexistent", ""); code != http.StatusNotFound {
		t.Errorf("GET of a missing queue = %d, want 404", code)
	}
}

// Every lookup built the queue's name with the location hardcoded to
// us-central1, while create stored it under the name the client sent. A queue
// in any other region could therefore be created but not read, listed or
// deleted.
func TestQueueOutsideUsCentralIsReachable(t *testing.T) {
	isolate(t)

	const collection = "/v2/projects/probe/locations/europe-west1/queues"
	api := NewAPI()

	if code, _ := call(t, api, http.MethodPost, collection,
		`{"name":"projects/probe/locations/europe-west1/queues/emails"}`); code != http.StatusOK {
		t.Fatalf("create = %d, want 200", code)
	}

	if code, _ := call(t, api, http.MethodGet, collection+"/emails", ""); code != http.StatusOK {
		t.Errorf("GET = %d, want 200", code)
	}

	code, listed := call(t, api, http.MethodGet, collection, "")
	if code != http.StatusOK {
		t.Fatalf("list = %d, want 200", code)
	}
	queues, _ := listed["queues"].([]interface{})
	if len(queues) != 1 {
		t.Errorf("list returned %d queues, want 1", len(queues))
	}

	if code, _ := call(t, api, http.MethodDelete, collection+"/emails", ""); code != http.StatusOK {
		t.Errorf("delete = %d, want 200", code)
	}
	if code, _ := call(t, api, http.MethodGet, collection+"/emails", ""); code != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", code)
	}
}

// Tasks hang off the queue and were keyed the same hardcoded way.
func TestTasksFollowTheirQueuesLocation(t *testing.T) {
	isolate(t)

	const queue = "/v2/projects/probe/locations/europe-west1/queues/emails"
	api := NewAPI()
	call(t, api, http.MethodPost, "/v2/projects/probe/locations/europe-west1/queues",
		`{"name":"projects/probe/locations/europe-west1/queues/emails"}`)

	if code, _ := call(t, api, http.MethodPost, queue+"/tasks",
		`{"task":{"httpRequest":{"url":"http://localhost:1/noop","httpMethod":"POST"}}}`); code != http.StatusOK {
		t.Fatalf("create task = %d, want 200", code)
	}

	code, listed := call(t, api, http.MethodGet, queue+"/tasks", "")
	if code != http.StatusOK {
		t.Fatalf("list tasks = %d, want 200", code)
	}
	tasks, _ := listed["tasks"].([]interface{})
	if len(tasks) != 1 {
		t.Errorf("list returned %d tasks, want 1", len(tasks))
	}
}
