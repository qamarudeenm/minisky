package scheduler

import (
	"net/http"
	"testing"
)

// Terraform's google_cloud_scheduler_job sends the full resource name in the
// create body, the way the real API documents it. The shim stored that name
// verbatim while looking jobs up by a name derived from the URL, which carries
// a leading slash and a version segment — so a job created this way could be
// created but never read, updated, paused or deleted.
func TestJobCreatedWithAFullResourceNameCanBeReadBack(t *testing.T) {
	isolate(t)

	const collection = "/v1/projects/probe/locations/us-central1/jobs"
	api := NewAPI(nil)

	code, created := call(t, api, http.MethodPost, collection,
		`{"name":"projects/probe/locations/us-central1/jobs/nightly","schedule":"0 2 * * *",
		  "httpTarget":{"uri":"http://localhost:1/noop","httpMethod":"GET"}}`)
	if code != http.StatusOK {
		t.Fatalf("create = %d, want 200", code)
	}
	if created["name"] != "projects/probe/locations/us-central1/jobs/nightly" {
		t.Errorf("name = %v, want the canonical resource name", created["name"])
	}

	if code, _ := call(t, api, http.MethodGet, collection+"/nightly", ""); code != http.StatusOK {
		t.Errorf("GET = %d, want 200", code)
	}
	if code, _ := call(t, api, http.MethodPost, collection+"/nightly:pause", ""); code != http.StatusOK {
		t.Errorf("pause = %d, want 200", code)
	}
	if code, _ := call(t, api, http.MethodDelete, collection+"/nightly", ""); code != http.StatusOK {
		t.Errorf("delete = %d, want 200", code)
	}
	if code, _ := call(t, api, http.MethodGet, collection+"/nightly", ""); code != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", code)
	}
}

// A bare job id in the body is the other shape clients send, and it has to end
// up under the same name as the full one.
func TestBareAndFullJobNamesResolveToTheSameJob(t *testing.T) {
	isolate(t)

	const collection = "/v1/projects/probe/locations/us-central1/jobs"
	api := NewAPI(nil)

	_, created := call(t, api, http.MethodPost, collection,
		`{"name":"nightly","schedule":"0 2 * * *",
		  "httpTarget":{"uri":"http://localhost:1/noop","httpMethod":"GET"}}`)
	if created["name"] != "projects/probe/locations/us-central1/jobs/nightly" {
		t.Errorf("name = %v, want the canonical resource name", created["name"])
	}

	code, listed := call(t, api, http.MethodGet, collection, "")
	if code != http.StatusOK {
		t.Fatalf("list = %d, want 200", code)
	}
	jobs, _ := listed["jobs"].([]interface{})
	if len(jobs) != 1 {
		t.Fatalf("list returned %d jobs, want 1", len(jobs))
	}
}

func TestJobNameFromPath(t *testing.T) {
	cases := map[string]string{
		"/v1/projects/p/locations/l/jobs/nightly": "projects/p/locations/l/jobs/nightly",
		"/projects/p/locations/l/jobs/nightly":    "projects/p/locations/l/jobs/nightly",
		"projects/p/locations/l/jobs/nightly":     "projects/p/locations/l/jobs/nightly",
		"/v1/projects/p/locations/l/jobs":         "",
	}
	for path, want := range cases {
		if got := jobNameFromPath(path); got != want {
			t.Errorf("jobNameFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}
