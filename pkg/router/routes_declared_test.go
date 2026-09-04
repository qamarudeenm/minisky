package router

import (
	"testing"

	"minisky/pkg/registry"

	// Blank import so every shim's init() runs and declares its routes. This is
	// the same wiring cmd/minisky uses, so the table under test is the real one.
	_ "minisky/pkg/shims"
)

// Every path here was probed against a running v1.4.2 gateway on 4 Sep 2026.
// Eighteen of them answered 501 or 404 from the wrong shim while the owning
// shim answered 200 when addressed by hostname. This pins that each now
// resolves to the service that owns it.
func TestDeclaredRoutesResolveProbedPaths(t *testing.T) {
	table := newRouteTable(registry.Routes())

	cases := []struct {
		path string
		want string
	}{
		// Reachable before this change — regression guard.
		{"/compute/v1/projects/p/global/networks", "compute.googleapis.com"},
		{"/storage/v1/b?project=p", "storage.googleapis.com"},
		{"/bigquery/v2/projects/p/datasets", "bigquery.googleapis.com"},
		{"/v1/projects/p/topics", "pubsub.googleapis.com"},
		{"/v2/projects/p/locations/us-central1/services", "run.googleapis.com"},

		// Answered 501 — no route reached a finished shim.
		{"/sql/v1beta4/projects/p/instances", "sqladmin.googleapis.com"},
		{"/v1/projects/p/serviceAccounts", "iam.googleapis.com"},
		{"/v1/projects/p/secrets", "secretmanager.googleapis.com"},
		{"/dns/v1/projects/p/managedZones", "dns.googleapis.com"},
		{"/v3/projects/p/timeSeries", "monitoring.googleapis.com"},
		{"/v1/projects/p/instances", "spanner.googleapis.com"},
		{"/v1/apps/p", "appengine.googleapis.com"},

		// Answered 404 — routed to Cloud Functions by the blanket rules.
		{"/v1/projects/p/locations/us-central1/clusters", "container.googleapis.com"},
		{"/v2/entries:list", "logging.googleapis.com"},
		{"/v1/projects/p/locations/us-central1/repositories", "artifactregistry.googleapis.com"},
		{"/v1/projects/p/locations/us-central1/keyRings", "cloudkms.googleapis.com"},
		{"/v2/projects/p/locations/us-central1/queues", "cloudtasks.googleapis.com"},
		{"/v1/projects/p/locations/us-central1/jobs", "cloudscheduler.googleapis.com"},
		{"/v1/projects/p/locations/us-central1/instances", "redis.googleapis.com"},
		{"/v2/projects/p/instances", "bigtableadmin.googleapis.com"},
		{"/v1/projects/p/locations/us-central1/models", "aiplatform.googleapis.com"},

		// Neighbours that must not be captured by the patterns above.
		{"/v1/projects/p/regions/us-central1/clusters", "dataproc.googleapis.com"},
		{"/v2/projects/p/locations/us-central1/functions", "cloudfunctions.googleapis.com"},
		{"/v1/projects/p/builds", "cloudbuild.googleapis.com"},
	}

	for _, tc := range cases {
		if got := table.resolve(tc.path); got != tc.want {
			t.Errorf("resolve(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// Resource Manager is not built yet, so its paths must stay unrouted rather than
// being swallowed by a neighbour — a wrong 200 would be worse than a 501.
func TestResourceManagerPathsAreStillUnclaimed(t *testing.T) {
	table := newRouteTable(registry.Routes())

	for _, path := range []string{
		"/v3/folders?parent=organizations/1",
		"/v1/organizations",
	} {
		if got := table.resolve(path); got != "" {
			t.Errorf("resolve(%q) = %q, want no owner until the shim exists", path, got)
		}
	}
}

// A declaration broad enough to swallow its neighbours is a routing bug, so keep
// the shared prefixes honest: nothing may claim "/v1/projects/*" wholesale.
func TestNoDeclarationIsDangerouslyBroad(t *testing.T) {
	for domain, globs := range registry.Routes() {
		for _, glob := range globs {
			switch glob {
			case "/v1/projects/*", "/v1/projects", "/v1", "/v2", "/v3", "/":
				t.Errorf("%s declares %q, which would capture unrelated services", domain, glob)
			}
		}
	}
}

// Every declared operation kind must name a domain that actually has a shim,
// or a poll would resolve to a service the gateway cannot dispatch to.
func TestDeclaredOperationKindsNameRealServices(t *testing.T) {
	kinds := registry.OperationKinds()
	if len(kinds) == 0 {
		t.Fatal("no operation kinds declared; operation polls cannot be attributed")
	}

	routed := registry.Routes()
	for kind, domain := range kinds {
		if _, ok := routed[domain]; !ok {
			t.Errorf("kind %q maps to %q, which declares no routes", kind, domain)
		}
	}

	// These two do not follow the domain name, which is why the mapping is
	// declared rather than derived.
	if kinds["sql#operation"] != "sqladmin.googleapis.com" {
		t.Errorf("sql#operation = %q, want sqladmin.googleapis.com", kinds["sql#operation"])
	}
	if kinds["memorystore#operation"] != "redis.googleapis.com" {
		t.Errorf("memorystore#operation = %q, want redis.googleapis.com", kinds["memorystore#operation"])
	}
}

// The shared regional operations path must belong to nobody, so that polls are
// attributed by operation id instead of by a guess.
func TestSharedOperationsPathIsUnclaimed(t *testing.T) {
	table := newRouteTable(registry.Routes())

	if got := table.resolve("/v1/projects/p/locations/us-central1/operations/op-1"); got != "" {
		t.Errorf("the shared operations path resolved to %q by pattern; it must be attributed "+
			"by operation id, since GKE, Cloud Functions, Artifact Registry and Memorystore "+
			"all publish it", got)
	}

	// The zonal path is GKE's alone, so it stays a normal route.
	if got := table.resolve("/v1/projects/p/zones/us-central1-a/operations/op-1"); got != "container.googleapis.com" {
		t.Errorf("zonal operations = %q, want container.googleapis.com", got)
	}
}

// The Vertex AI shim serves the generative surface by translating to a local
// LLM provider, and that path carries a "publishers" segment. Routing only
// .../models would 501 the one endpoint the shim actually implements.
func TestVertexGenerativePathIsRouted(t *testing.T) {
	table := newRouteTable(registry.Routes())

	const genai = "/v1/projects/p/locations/us-central1/publishers/google/models/gemini-1.5-flash:generateContent"
	if got := table.resolve(genai); got != "aiplatform.googleapis.com" {
		t.Errorf("generateContent resolved to %q, want aiplatform.googleapis.com", got)
	}
}

// Firestore and Datastore are Docker-backed, so they need routes like any other
// service — a client reaching them through the gateway sends no hostname.
func TestDockerBackedServicesAreRouted(t *testing.T) {
	table := newRouteTable(registry.Routes())

	cases := map[string]string{
		"/v1/projects/p/databases/(default)/documents": "firestore.googleapis.com",
		"/v1/projects/p:runQuery":                      "datastore.googleapis.com",
		"/v1/projects/p:commit":                        "datastore.googleapis.com",
		"/v1/projects/p/instances":                     "spanner.googleapis.com",
	}
	for path, want := range cases {
		if got := table.resolve(path); got != want {
			t.Errorf("resolve(%q) = %q, want %q", path, got, want)
		}
	}
}
