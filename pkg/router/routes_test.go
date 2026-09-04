package router

import (
	"testing"

	"minisky/pkg/orchestrator"
)

// Thirteen services share the "/v1/projects/" prefix and are told apart only by
// a segment that sits after the variable project id. These cases pin that the
// most specific pattern wins, whatever order the shims registered in.
func TestRouteTableResolvesSharedPrefixes(t *testing.T) {
	table := newRouteTable(map[string][]string{
		"secretmanager.googleapis.com":  {"/v1/projects/*/secrets"},
		"pubsub.googleapis.com":         {"/v1/projects/*/topics", "/v1/projects/*/subscriptions"},
		"iam.googleapis.com":            {"/v1/projects/*/serviceAccounts"},
		"cloudkms.googleapis.com":       {"/v1/projects/*/locations/*/keyRings"},
		"container.googleapis.com":      {"/v1/projects/*/locations/*/clusters"},
		"dataproc.googleapis.com":       {"/v1/projects/*/regions/*/clusters"},
		"cloudscheduler.googleapis.com": {"/v1/projects/*/locations/*/jobs"},
		"cloudfunctions.googleapis.com": {"/v1/projects/*/locations/*/functions"},
		"spanner.googleapis.com":        {"/v1/projects/*/instances"},
		"redis.googleapis.com":          {"/v1/projects/*/locations/*/instances"},
	})

	cases := map[string]string{
		"/v1/projects/p/secrets":                           "secretmanager.googleapis.com",
		"/v1/projects/p/topics":                            "pubsub.googleapis.com",
		"/v1/projects/p/subscriptions/sub":                 "pubsub.googleapis.com",
		"/v1/projects/p/serviceAccounts":                   "iam.googleapis.com",
		"/v1/projects/p/locations/l/keyRings":              "cloudkms.googleapis.com",
		"/v1/projects/p/locations/l/clusters":              "container.googleapis.com",
		"/v1/projects/p/regions/r/clusters":                "dataproc.googleapis.com",
		"/v1/projects/p/locations/l/jobs":                  "cloudscheduler.googleapis.com",
		"/v1/projects/p/locations/l/functions":             "cloudfunctions.googleapis.com",
		"/v1/projects/p/instances":                         "spanner.googleapis.com",
		"/v1/projects/p/locations/l/instances":             "redis.googleapis.com",
		"/v1/projects/p/instances/i/databases/d":           "spanner.googleapis.com",
		"/v1/projects/p/secrets/s/versions/1:access":       "secretmanager.googleapis.com",
		"/v1/projects/p/locations/l/keyRings/k/cryptoKeys": "cloudkms.googleapis.com",
	}
	for path, want := range cases {
		if got := table.resolve(path); got != want {
			t.Errorf("resolve(%q) = %q, want %q", path, got, want)
		}
	}
}

// A pattern claims the subtree beneath it, but must not claim a sibling.
func TestRouteTablePrefixSemantics(t *testing.T) {
	table := newRouteTable(map[string][]string{
		"compute.googleapis.com": {"/compute/v1"},
		"pubsub.googleapis.com":  {"/v1/projects/*/topics"},
	})

	if got := table.resolve("/compute/v1/projects/p/zones/z/instances"); got != "compute.googleapis.com" {
		t.Errorf("a pattern must own its subtree, got %q", got)
	}
	if got := table.resolve("/compute"); got != "" {
		t.Errorf("a path shorter than the pattern must not match, got %q", got)
	}
	if got := table.resolve("/v1/projects/p/snapshots"); got != "" {
		t.Errorf("an unclaimed sibling must not match, got %q", got)
	}
}

// Specificity has to beat map iteration order, which Go randomises.
func TestRouteTableSpecificityBeatsRegistrationOrder(t *testing.T) {
	for i := 0; i < 50; i++ {
		table := newRouteTable(map[string][]string{
			"resourcemanager.googleapis.com": {"/v3/projects/*"},
			"monitoring.googleapis.com":      {"/v3/projects/*/timeSeries"},
		})
		if got := table.resolve("/v3/projects/p/timeSeries"); got != "monitoring.googleapis.com" {
			t.Fatalf("iteration %d: more literal segments must win, got %q", i, got)
		}
		if got := table.resolve("/v3/projects/p"); got != "resourcemanager.googleapis.com" {
			t.Fatalf("iteration %d: the general pattern still owns the bare path, got %q", i, got)
		}
	}
}

func TestRouteTableIgnoresQueryString(t *testing.T) {
	table := newRouteTable(map[string][]string{
		"storage.googleapis.com": {"/storage/v1/b"},
	})
	if got := table.resolve("/storage/v1/b?project=p&prefix=seeds%2F"); got != "storage.googleapis.com" {
		t.Errorf("query string must not affect routing, got %q", got)
	}
}

// The 501 should name what the gateway serves nearby instead of the Host header.
func TestRouteTableCandidates(t *testing.T) {
	table := newRouteTable(map[string][]string{
		"pubsub.googleapis.com":        {"/v1/projects/*/topics"},
		"secretmanager.googleapis.com": {"/v1/projects/*/secrets"},
		"compute.googleapis.com":       {"/compute/v1"},
	})

	near := table.candidates("/v1/projects/p/somethingUnknown")
	if len(near) != 2 || near[0] != "pubsub.googleapis.com" || near[1] != "secretmanager.googleapis.com" {
		t.Errorf("candidates = %v, want the two /v1 services in sorted order", near)
	}
	if got := table.candidates("/totally/unknown"); len(got) != 0 {
		t.Errorf("an unrelated prefix has no candidates, got %v", got)
	}
}

func TestRouteTableHandlesEmptyInput(t *testing.T) {
	table := newRouteTable(map[string][]string{"x.googleapis.com": {"/v1/x"}})
	for _, path := range []string{"", "/", "///"} {
		if got := table.resolve(path); got != "" {
			t.Errorf("resolve(%q) = %q, want empty", path, got)
		}
	}
	var nilTable *routeTable
	if got := nilTable.resolve("/v1/x"); got != "" {
		t.Errorf("a nil table must resolve to empty, got %q", got)
	}
}

// stubOperations stands in for the shared OperationManager.
type stubOperations map[string]string // operation name -> kind

func (s stubOperations) Get(name string) *orchestrator.Operation {
	kind, ok := s[name]
	if !ok {
		return nil
	}
	return &orchestrator.Operation{Name: name, Kind: kind}
}

// GKE, Cloud Functions, Artifact Registry and Memorystore all publish
// /v1/projects/*/locations/*/operations. Whoever minted the operation owns the
// poll — picking a single winner for the path would answer another service's
// polls with the wrong shim.
func TestOperationPollRoutesToTheServiceThatMintedIt(t *testing.T) {
	kinds := map[string]string{
		"container#operation":        "container.googleapis.com",
		"cloudfunctions#operation":   "cloudfunctions.googleapis.com",
		"artifactregistry#operation": "artifactregistry.googleapis.com",
		"memorystore#operation":      "redis.googleapis.com",
		"sql#operation":              "sqladmin.googleapis.com",
	}
	ops := stubOperations{
		"operation-1-aaaa": "container#operation",
		"operation-2-bbbb": "cloudfunctions#operation",
		"operation-3-cccc": "artifactregistry#operation",
		"operation-4-dddd": "memorystore#operation",
		"operation-5-eeee": "sql#operation",
	}

	// The identical path shape resolves differently per operation.
	const shared = "/v1/projects/p/locations/us-central1/operations/"
	cases := map[string]string{
		shared + "operation-1-aaaa":                           "container.googleapis.com",
		shared + "operation-2-bbbb":                           "cloudfunctions.googleapis.com",
		shared + "operation-3-cccc":                           "artifactregistry.googleapis.com",
		shared + "operation-4-dddd":                           "redis.googleapis.com",
		"/sql/v1beta4/projects/p/operations/operation-5-eeee": "sqladmin.googleapis.com",
	}
	for path, want := range cases {
		if got := resolveOperation(path, ops, kinds); got != want {
			t.Errorf("resolveOperation(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestOperationResolutionDeclinesWhatItCannotAttribute(t *testing.T) {
	kinds := map[string]string{"container#operation": "container.googleapis.com"}
	ops := stubOperations{"operation-1-aaaa": "container#operation"}

	cases := []struct {
		name string
		path string
	}{
		{"collection path carries no id", "/v1/projects/p/locations/l/operations"},
		{"unknown operation", "/v1/projects/p/locations/l/operations/operation-9-zzzz"},
		{"not an operation path", "/v1/projects/p/locations/l/clusters"},
		{"operations as the final segment", "/v1/operations"},
	}
	for _, tc := range cases {
		if got := resolveOperation(tc.path, ops, kinds); got != "" {
			t.Errorf("%s: resolveOperation(%q) = %q, want empty so the path table decides",
				tc.name, tc.path, got)
		}
	}

	// A router with no OperationManager must fall through, not panic.
	if got := resolveOperation("/v1/projects/p/locations/l/operations/operation-1-aaaa", nil, kinds); got != "" {
		t.Errorf("without an operation manager, got %q, want empty", got)
	}
}
