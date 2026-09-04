package storage

import (
	"encoding/json"
	"log"
	"strings"
	"sync"

	"minisky/pkg/persist"
)

// ─────────────────────────────────────────────────────────────────────────────
// Bucket metadata preservation
//
// fake-gcs-server does not model a bucket's location, labels or storage class:
// it accepts them on create and then reports its own defaults (location
// "US-CENTRAL1", no labels). Because `location` is a ForceNew attribute in the
// Terraform provider, a bucket created with `location = "US"` came back as
// "US-CENTRAL1" and every subsequent `terraform apply` destroyed and recreated
// it — silently discarding every object in it.
//
// The shim therefore remembers what the caller asked for and overlays it on the
// responses the emulator produces.
// ─────────────────────────────────────────────────────────────────────────────

// bucketAttrs holds the fields the emulator drops.
type bucketAttrs struct {
	Location     string
	StorageClass string
	Labels       map[string]string
}

// bucketRegistry stores the per-bucket attributes the emulator drops.
//
// It is persisted because losing it is destructive rather than merely
// forgetful: the location reverts to the emulator's default, Terraform sees a
// change on a ForceNew attribute, and the next apply destroys the bucket and
// every object in it.
type bucketRegistry struct {
	mu      sync.RWMutex
	buckets map[string]bucketAttrs
}

// stateName is the file under ~/.minisky this registry is kept in.
const stateName = "storage_buckets"

func newBucketRegistry() *bucketRegistry {
	r := &bucketRegistry{buckets: map[string]bucketAttrs{}}

	var stored map[string]bucketAttrs
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Storage] ignoring unreadable bucket metadata: %v", err)
	} else if len(stored) > 0 {
		r.buckets = stored
		log.Printf("[Storage] restored metadata for %d bucket(s)", len(stored))
	}
	return r
}

// save writes the registry out. Callers hold the lock.
func (r *bucketRegistry) save() {
	if err := persist.Save(stateName, r.buckets); err != nil {
		log.Printf("[Storage] could not persist bucket metadata: %v", err)
	}
}

// record captures the attributes from a bucket insert/patch request body.
// Fields absent from the request leave any previously recorded value in place,
// which is what a PATCH means.
func (r *bucketRegistry) record(name string, body []byte) {
	if name == "" && len(body) == 0 {
		return
	}

	var request struct {
		Name         string            `json:"name"`
		Location     string            `json:"location"`
		StorageClass string            `json:"storageClass"`
		Labels       map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return
	}
	if name == "" {
		name = request.Name
	}
	if name == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	attrs := r.buckets[name]
	if request.Location != "" {
		// GCS reports locations upper-cased regardless of how they were sent.
		attrs.Location = strings.ToUpper(request.Location)
	}
	if request.StorageClass != "" {
		attrs.StorageClass = request.StorageClass
	}
	if request.Labels != nil {
		attrs.Labels = request.Labels
	}
	r.buckets[name] = attrs
	r.save()
}

// forget drops a bucket's attributes once it is deleted.
func (r *bucketRegistry) forget(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.buckets, name)
	r.save()
}

// lookup returns the recorded attributes for a bucket.
func (r *bucketRegistry) lookup(name string) (bucketAttrs, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	attrs, ok := r.buckets[name]
	return attrs, ok
}

// overlay rewrites a bucket resource — or a bucket list — so the recorded
// attributes replace the emulator's defaults. It returns the body unchanged
// when there is nothing to overlay, so a response it does not understand is
// always passed through intact.
func (r *bucketRegistry) overlay(body []byte) []byte {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}

	changed := false
	switch payload["kind"] {
	case "storage#buckets":
		items, ok := payload["items"].([]interface{})
		if !ok {
			return body
		}
		for _, item := range items {
			bucket, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			changed = r.applyTo(bucket) || changed
		}
	case "storage#bucket":
		changed = r.applyTo(payload)
	default:
		return body
	}

	if !changed {
		return body
	}
	rewritten, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return rewritten
}

// applyTo overlays one bucket object in place, reporting whether it changed.
func (r *bucketRegistry) applyTo(bucket map[string]interface{}) bool {
	name, _ := bucket["name"].(string)
	attrs, ok := r.lookup(name)
	if !ok {
		return false
	}

	changed := false
	if attrs.Location != "" && bucket["location"] != attrs.Location {
		bucket["location"] = attrs.Location
		changed = true
	}
	if attrs.StorageClass != "" && bucket["storageClass"] != attrs.StorageClass {
		bucket["storageClass"] = attrs.StorageClass
		changed = true
	}
	if len(attrs.Labels) > 0 {
		labels := make(map[string]interface{}, len(attrs.Labels))
		for k, v := range attrs.Labels {
			labels[k] = v
		}
		bucket["labels"] = labels
		changed = true
	}
	return changed
}

// isBucketRequest reports whether a path addresses the bucket collection or a
// single bucket — /storage/v1/b, /storage/v1/b/{bucket} — as opposed to an
// object under one.
func isBucketRequest(path string) bool {
	trimmed := strings.TrimSuffix(path, "/")
	idx := strings.Index(trimmed, "/b")
	if idx < 0 {
		return false
	}
	rest := strings.TrimPrefix(trimmed[idx:], "/b")
	if rest == "" {
		return true // the bucket collection
	}
	if !strings.HasPrefix(rest, "/") {
		return false
	}
	// A single bucket has exactly one remaining segment; anything deeper
	// (…/o, …/o/{object}) addresses objects.
	return !strings.Contains(strings.TrimPrefix(rest, "/"), "/")
}

// bucketNameFromPath returns the bucket a single-bucket path addresses, or "".
func bucketNameFromPath(path string) string {
	trimmed := strings.TrimSuffix(path, "/")
	idx := strings.Index(trimmed, "/b/")
	if idx < 0 {
		return ""
	}
	rest := trimmed[idx+len("/b/"):]
	if rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	return rest
}
