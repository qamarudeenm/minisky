package storage

import (
	"encoding/json"
	"testing"
)

// The defect this guards against: a bucket created with location "US" came back
// as fake-gcs-server's "US-CENTRAL1". Location is ForceNew in the Terraform
// provider, so every apply destroyed and recreated the bucket — along with
// every object in it.
func TestOverlayRestoresRequestedLocation(t *testing.T) {
	registry := newBucketRegistry()
	registry.record("", []byte(`{"name":"retail-raw","location":"US","labels":{"zone":"raw"}}`))

	emulatorResponse := []byte(`{"kind":"storage#bucket","name":"retail-raw","location":"US-CENTRAL1"}`)
	var bucket map[string]interface{}
	if err := json.Unmarshal(registry.overlay(emulatorResponse), &bucket); err != nil {
		t.Fatalf("overlay produced invalid JSON: %v", err)
	}

	if bucket["location"] != "US" {
		t.Errorf("location = %v, want US", bucket["location"])
	}
	labels, ok := bucket["labels"].(map[string]interface{})
	if !ok || labels["zone"] != "raw" {
		t.Errorf("labels = %v, want zone=raw", bucket["labels"])
	}
}

func TestOverlayNormalisesLocationCase(t *testing.T) {
	registry := newBucketRegistry()
	registry.record("", []byte(`{"name":"b","location":"us"}`))

	var bucket map[string]interface{}
	json.Unmarshal(registry.overlay([]byte(`{"kind":"storage#bucket","name":"b","location":"US-CENTRAL1"}`)), &bucket)
	if bucket["location"] != "US" {
		t.Errorf("location = %v, want US (GCS reports locations upper-cased)", bucket["location"])
	}
}

func TestOverlayHandlesBucketLists(t *testing.T) {
	registry := newBucketRegistry()
	registry.record("", []byte(`{"name":"a","location":"EU"}`))
	registry.record("", []byte(`{"name":"b","location":"ASIA"}`))

	listing := []byte(`{"kind":"storage#buckets","items":[
		{"name":"a","location":"US-CENTRAL1"},
		{"name":"b","location":"US-CENTRAL1"},
		{"name":"unknown","location":"US-CENTRAL1"}]}`)

	var payload struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(registry.overlay(listing), &payload); err != nil {
		t.Fatalf("overlay produced invalid JSON: %v", err)
	}
	if payload.Items[0]["location"] != "EU" || payload.Items[1]["location"] != "ASIA" {
		t.Errorf("recorded locations not applied: %v", payload.Items)
	}
	// A bucket the shim never saw created must pass through untouched.
	if payload.Items[2]["location"] != "US-CENTRAL1" {
		t.Errorf("unknown bucket was rewritten: %v", payload.Items[2])
	}
}

func TestOverlayLeavesUnrelatedBodiesAlone(t *testing.T) {
	registry := newBucketRegistry()
	for _, body := range []string{
		`{"kind":"storage#object","name":"seeds/orders.csv"}`,
		`not json at all`,
		`{}`,
	} {
		if got := string(registry.overlay([]byte(body))); got != body {
			t.Errorf("overlay(%q) = %q, want it unchanged", body, got)
		}
	}
}

func TestPatchKeepsUnsentAttributes(t *testing.T) {
	registry := newBucketRegistry()
	registry.record("", []byte(`{"name":"b","location":"EU","labels":{"env":"local"}}`))
	// A PATCH that only touches labels must not erase the recorded location.
	registry.record("b", []byte(`{"labels":{"env":"prod"}}`))

	attrs, ok := registry.lookup("b")
	if !ok || attrs.Location != "EU" {
		t.Errorf("location lost by a partial update: %+v", attrs)
	}
	if attrs.Labels["env"] != "prod" {
		t.Errorf("labels not updated: %+v", attrs.Labels)
	}
}

func TestForgetDropsDeletedBuckets(t *testing.T) {
	registry := newBucketRegistry()
	registry.record("", []byte(`{"name":"b","location":"EU"}`))
	registry.forget("b")
	if _, ok := registry.lookup("b"); ok {
		t.Error("a deleted bucket must not keep its recorded attributes")
	}
}

func TestBucketPathDetection(t *testing.T) {
	buckets := map[string]bool{
		"/storage/v1/b":                         true,
		"/storage/v1/b/":                        true,
		"/storage/v1/b/retail-raw":              true,
		"/storage/v1/b/retail-raw/o":            false,
		"/storage/v1/b/retail-raw/o/seeds%2Fa":  false,
		"/upload/storage/v1/b/retail-raw/o":     false,
		"/storage/v1/projects/p/serviceAccount": false,
	}
	for path, want := range buckets {
		if got := isBucketRequest(path); got != want {
			t.Errorf("isBucketRequest(%q) = %v, want %v", path, got, want)
		}
	}

	if got := bucketNameFromPath("/storage/v1/b/retail-raw"); got != "retail-raw" {
		t.Errorf("bucketNameFromPath = %q, want retail-raw", got)
	}
	if got := bucketNameFromPath("/storage/v1/b"); got != "" {
		t.Errorf("the bucket collection has no name, got %q", got)
	}
}
