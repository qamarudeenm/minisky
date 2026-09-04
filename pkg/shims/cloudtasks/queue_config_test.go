package cloudtasks

import (
	"net/http"
	"testing"
	"time"
)

// google_cloud_tasks_queue manages rate limits, retry policy and routing. The
// shim stored only a name and a state, so everything the user declared was
// dropped on create and read back as absent — terraform plan proposed the same
// change on every run and never converged.
func TestQueueConfigurationIsStoredAndReadBack(t *testing.T) {
	isolate(t)

	const collection = "/v2/projects/probe/locations/us-central1/queues"
	api := NewAPI()

	code, created := call(t, api, http.MethodPost, collection, `{
		"name":"projects/probe/locations/us-central1/queues/emails",
		"rateLimits":{"maxDispatchesPerSecond":10,"maxConcurrentDispatches":5},
		"retryConfig":{"maxAttempts":3,"minBackoff":"1s","maxBackoff":"10s","maxDoublings":2},
		"appEngineRoutingOverride":{"service":"worker"},
		"stackdriverLoggingConfig":{"samplingRatio":0.5}}`)
	if code != http.StatusOK {
		t.Fatalf("create = %d, want 200", code)
	}

	for _, body := range []map[string]interface{}{created, mustGet(t, api, collection+"/emails")} {
		limits, ok := body["rateLimits"].(map[string]interface{})
		if !ok {
			t.Fatalf("rateLimits missing: %v", body)
		}
		if limits["maxDispatchesPerSecond"] != float64(10) {
			t.Errorf("maxDispatchesPerSecond = %v, want 10", limits["maxDispatchesPerSecond"])
		}
		if limits["maxConcurrentDispatches"] != float64(5) {
			t.Errorf("maxConcurrentDispatches = %v, want 5", limits["maxConcurrentDispatches"])
		}

		retry, ok := body["retryConfig"].(map[string]interface{})
		if !ok {
			t.Fatalf("retryConfig missing: %v", body)
		}
		if retry["maxAttempts"] != float64(3) || retry["minBackoff"] != "1s" {
			t.Errorf("retryConfig = %v", retry)
		}

		routing, ok := body["appEngineRoutingOverride"].(map[string]interface{})
		if !ok || routing["service"] != "worker" {
			t.Errorf("appEngineRoutingOverride = %v", body["appEngineRoutingOverride"])
		}
	}
}

// Terraform reads every computed field back. A queue created without a block
// must still report the values GCP would have filled in, or the provider sees a
// change it cannot resolve.
func TestUnsetQueueConfigurationGetsGoogleDefaults(t *testing.T) {
	isolate(t)

	const collection = "/v2/projects/probe/locations/us-central1/queues"
	api := NewAPI()
	call(t, api, http.MethodPost, collection,
		`{"name":"projects/probe/locations/us-central1/queues/plain"}`)

	queue := mustGet(t, api, collection+"/plain")
	limits, _ := queue["rateLimits"].(map[string]interface{})
	if limits == nil || limits["maxDispatchesPerSecond"] != float64(500) ||
		limits["maxConcurrentDispatches"] != float64(1000) {
		t.Errorf("rateLimits = %v, want Google's defaults", queue["rateLimits"])
	}
	retry, _ := queue["retryConfig"].(map[string]interface{})
	if retry == nil || retry["maxAttempts"] != float64(100) || retry["maxBackoff"] != "3600s" {
		t.Errorf("retryConfig = %v, want Google's defaults", queue["retryConfig"])
	}
}

// queues.patch is how a settings change reaches the API.
func TestQueuePatchAppliesTheChange(t *testing.T) {
	isolate(t)

	const collection = "/v2/projects/probe/locations/us-central1/queues"
	api := NewAPI()
	call(t, api, http.MethodPost, collection,
		`{"name":"projects/probe/locations/us-central1/queues/emails",
		  "rateLimits":{"maxDispatchesPerSecond":10,"maxConcurrentDispatches":5}}`)

	code, patched := call(t, api, http.MethodPatch, collection+"/emails?updateMask=rateLimits",
		`{"rateLimits":{"maxDispatchesPerSecond":25,"maxConcurrentDispatches":8}}`)
	if code != http.StatusOK {
		t.Fatalf("patch = %d, want 200", code)
	}
	limits, _ := patched["rateLimits"].(map[string]interface{})
	if limits == nil || limits["maxDispatchesPerSecond"] != float64(25) {
		t.Fatalf("patch response rateLimits = %v", patched["rateLimits"])
	}

	// An update that is not applied makes the next plan propose it again.
	queue := mustGet(t, api, collection+"/emails")
	limits, _ = queue["rateLimits"].(map[string]interface{})
	if limits["maxDispatchesPerSecond"] != float64(25) || limits["maxConcurrentDispatches"] != float64(8) {
		t.Errorf("rateLimits = %v after patch, want the new values", queue["rateLimits"])
	}

	// A group the mask does not name is left alone.
	retry, _ := queue["retryConfig"].(map[string]interface{})
	if retry == nil || retry["maxAttempts"] != float64(100) {
		t.Errorf("retryConfig = %v, want it untouched", queue["retryConfig"])
	}
}

// GCP's patch creates the queue when it does not exist.
func TestQueuePatchCreatesAMissingQueue(t *testing.T) {
	isolate(t)

	const collection = "/v2/projects/probe/locations/us-central1/queues"
	code, created := call(t, NewAPI(), http.MethodPatch, collection+"/fresh",
		`{"rateLimits":{"maxDispatchesPerSecond":7}}`)
	if code != http.StatusOK {
		t.Fatalf("patch of a missing queue = %d, want 200", code)
	}
	if created["name"] != "projects/probe/locations/us-central1/queues/fresh" {
		t.Errorf("name = %v", created["name"])
	}
	if created["state"] != "RUNNING" {
		t.Errorf("state = %v, want RUNNING", created["state"])
	}
}

func TestQueuePauseResumeAndPurge(t *testing.T) {
	isolate(t)

	const collection = "/v2/projects/probe/locations/us-central1/queues"
	const queue = collection + "/emails"
	api := NewAPI()
	call(t, api, http.MethodPost, collection,
		`{"name":"projects/probe/locations/us-central1/queues/emails"}`)
	call(t, api, http.MethodPost, queue+"/tasks",
		`{"task":{"httpRequest":{"url":"http://localhost:1/noop","httpMethod":"POST"}}}`)

	code, paused := call(t, api, http.MethodPost, queue+":pause", "")
	if code != http.StatusOK {
		t.Fatalf("pause = %d, want 200", code)
	}
	if paused["state"] != "PAUSED" {
		t.Errorf("pause returned state %v, want PAUSED", paused["state"])
	}
	if mustGet(t, api, queue)["state"] != "PAUSED" {
		t.Error("the queue did not stay paused")
	}

	code, resumed := call(t, api, http.MethodPost, queue+":resume", "")
	if code != http.StatusOK || resumed["state"] != "RUNNING" {
		t.Errorf("resume = %d, state %v", code, resumed["state"])
	}

	code, purged := call(t, api, http.MethodPost, queue+":purge", "")
	if code != http.StatusOK {
		t.Fatalf("purge = %d, want 200", code)
	}
	if purged["purgeTime"] == nil {
		t.Error("purge did not report a purgeTime")
	}
	listed := mustGet(t, api, queue+"/tasks")
	if tasks, _ := listed["tasks"].([]interface{}); len(tasks) != 0 {
		t.Errorf("purge left %d task(s) behind", len(tasks))
	}

	// The queue itself survives a purge; only its tasks go.
	if mustGet(t, api, queue)["name"] == nil {
		t.Error("purge removed the queue")
	}
}

func TestVerbsOnAMissingQueueAre404(t *testing.T) {
	isolate(t)

	const queue = "/v2/projects/probe/locations/us-central1/queues/absent"
	api := NewAPI()
	for _, verb := range []string{":pause", ":resume", ":purge"} {
		if code, _ := call(t, api, http.MethodPost, queue+verb, ""); code != http.StatusNotFound {
			t.Errorf("%s on a missing queue = %d, want 404", verb, code)
		}
	}
}

func mustGet(t *testing.T, api *API, path string) map[string]interface{} {
	t.Helper()
	code, body := call(t, api, http.MethodGet, path, "")
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", path, code)
	}
	return body
}

// A paused queue accepts tasks and holds them. Dispatching from one would make
// pause a label rather than a state.
func TestPausedQueueHoldsItsTasks(t *testing.T) {
	isolate(t)

	const collection = "/v2/projects/probe/locations/us-central1/queues"
	const queue = collection + "/emails"
	api := NewAPI()
	call(t, api, http.MethodPost, collection,
		`{"name":"projects/probe/locations/us-central1/queues/emails"}`)
	call(t, api, http.MethodPost, queue+":pause", "")

	code, task := call(t, api, http.MethodPost, queue+"/tasks",
		`{"task":{"httpRequest":{"url":"http://localhost:1/noop","httpMethod":"POST"}}}`)
	if code != http.StatusOK {
		t.Fatalf("create task on a paused queue = %d, want 200 — it is accepted, not rejected", code)
	}
	if task["status"] != "PENDING" {
		t.Errorf("status = %v, want PENDING", task["status"])
	}

	// executeTask waits two seconds before marking a task COMPLETED; give it
	// longer than that to prove nothing was dispatched.
	time.Sleep(3 * time.Second)

	listed := mustGet(t, api, queue+"/tasks")
	tasks, _ := listed["tasks"].([]interface{})
	if len(tasks) != 1 {
		t.Fatalf("list returned %d tasks, want 1", len(tasks))
	}
	if held := tasks[0].(map[string]interface{}); held["status"] != "PENDING" {
		t.Errorf("status = %v after the wait; the paused queue dispatched it", held["status"])
	}
}
