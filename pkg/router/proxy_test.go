package router

import "testing"

// Anything running inside an emulated VM reaches the gateway as
// host.docker.internal (or an IP), never as "localhost". While path routing was
// gated on the Host header naming localhost, every such request fell through to
// the 501 branch — so a workload inside a VM could not call a single emulated
// API.
func TestPathMappedDomain(t *testing.T) {
	cases := map[string]string{
		"/storage/v1/b/my-bucket":                  "storage.googleapis.com",
		"/upload/storage/v1/b/my-bucket/o":         "storage.googleapis.com",
		"/bigquery/v2/projects/p/datasets":         "bigquery.googleapis.com",
		"/compute/v1/projects/p/zones/z/instances": "compute.googleapis.com",
		"/v1/projects/p/topics/t:publish":          "pubsub.googleapis.com",
		"/v1/projects/p/subscriptions/s":           "pubsub.googleapis.com",
		"/projects/p/topics/t":                     "pubsub.googleapis.com",
		"/v2/projects/p/locations/l/functions":     "cloudfunctions.googleapis.com",
		"/v1/projects/p/locations/l/services":      "cloudfunctions.googleapis.com",
		"/something/unmapped":                      "",
	}
	for path, want := range cases {
		if got := pathMappedDomain(path); got != want {
			t.Errorf("pathMappedDomain(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestIsGoogleServiceHost(t *testing.T) {
	google := []string{
		"bigquery.googleapis.com",
		"storage.googleapis.com:443",
		"my-project.firebaseio.com",
		"metadata.google.internal",
	}
	for _, host := range google {
		if !isGoogleServiceHost(host) {
			t.Errorf("%q should route by domain", host)
		}
	}

	// These all belong to callers that address the emulator directly and must
	// therefore be routed by URL prefix.
	others := []string{
		"localhost:8080",
		"127.0.0.1:8080",
		"host.docker.internal:8080",
		"172.21.0.1:8080",
		"minisky.local:8080",
	}
	for _, host := range others {
		if isGoogleServiceHost(host) {
			t.Errorf("%q should be path-routed, not domain-routed", host)
		}
	}
}
