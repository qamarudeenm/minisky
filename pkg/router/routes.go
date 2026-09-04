package router

import (
	"log"

	"minisky/pkg/orchestrator"
	"sort"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Path-based service resolution
//
// A request that names a Google API host is routed by that host. Everything
// else — Terraform's custom_endpoint, gcloud's api_endpoint_overrides, a client
// library pointed at the gateway — arrives as "localhost:8080" and has to be
// resolved from the URL alone.
//
// That cannot be done with prefix or substring tests, because the segment which
// identifies the service sits *after* variable segments:
//
//	/v1/projects/{PROJECT}/secrets                     → Secret Manager
//	/v1/projects/{PROJECT}/topics                      → Pub/Sub
//	/v1/projects/{PROJECT}/locations/{LOC}/keyRings    → Cloud KMS
//	/v1/projects/{PROJECT}/locations/{LOC}/clusters    → GKE
//	/v1/projects/{PROJECT}/regions/{REGION}/clusters   → Dataproc
//
// Thirteen services share "/v1/projects/". Each declares its own patterns via
// registry.RegisterRoutes, and the table below orders them so the most specific
// pattern wins regardless of registration order.
// ─────────────────────────────────────────────────────────────────────────────

// routeRule binds one URL pattern to the domain that owns it.
type routeRule struct {
	domain   string
	glob     string
	segments []string
	literals int // non-wildcard segments; drives specificity
}

// routeTable resolves a URL path to a service domain.
type routeTable struct {
	rules []routeRule
}

// newRouteTable builds a resolver from the declared patterns, ordered so that
// the most specific match is always found first:
//
//  1. more literal segments wins  — "/v1/projects/*/topics" beats "/v1/projects/*"
//  2. then more segments overall  — "/v1/projects/*/locations/*/jobs" beats "/v1/projects/*/jobs"
//  3. then the pattern itself, so the order is stable across runs
//
// Two domains claiming the identical pattern is a bug in the declarations, not
// something to resolve at request time, so it is reported at startup.
func newRouteTable(declared map[string][]string) *routeTable {
	seen := make(map[string]string, len(declared))
	var rules []routeRule

	for domain, globs := range declared {
		for _, glob := range globs {
			trimmed := strings.Trim(glob, "/")
			if trimmed == "" {
				log.Printf("[Router] ignoring empty route pattern for %s", domain)
				continue
			}
			if owner, dup := seen[trimmed]; dup && owner != domain {
				log.Printf("[Router] WARNING: %q is claimed by both %s and %s; %s wins by sort order",
					glob, owner, domain, minString(owner, domain))
			}
			seen[trimmed] = domain

			segments := strings.Split(trimmed, "/")
			literals := 0
			for _, seg := range segments {
				if seg != "*" {
					literals++
				}
			}
			rules = append(rules, routeRule{
				domain:   domain,
				glob:     "/" + trimmed,
				segments: segments,
				literals: literals,
			})
		}
	}

	sort.Slice(rules, func(i, j int) bool {
		if rules[i].literals != rules[j].literals {
			return rules[i].literals > rules[j].literals
		}
		if len(rules[i].segments) != len(rules[j].segments) {
			return len(rules[i].segments) > len(rules[j].segments)
		}
		if rules[i].glob != rules[j].glob {
			return rules[i].glob < rules[j].glob
		}
		return rules[i].domain < rules[j].domain
	})

	return &routeTable{rules: rules}
}

// resolve returns the domain owning path, or "" when no pattern claims it.
func (t *routeTable) resolve(path string) string {
	if t == nil {
		return ""
	}
	segments := splitPath(path)
	if len(segments) == 0 {
		return ""
	}
	for i := range t.rules {
		if matchPrefix(t.rules[i].segments, segments) {
			return t.rules[i].domain
		}
	}
	return ""
}

// candidates lists the domains whose patterns share a leading segment with the
// path. It turns "not implemented" into a message naming what the gateway does
// serve nearby.
func (t *routeTable) candidates(path string) []string {
	if t == nil {
		return nil
	}
	segments := splitPath(path)
	if len(segments) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var out []string
	for i := range t.rules {
		if t.rules[i].segments[0] != segments[0] {
			continue
		}
		if seen[t.rules[i].domain] {
			continue
		}
		seen[t.rules[i].domain] = true
		out = append(out, t.rules[i].domain)
	}
	sort.Strings(out)
	return out
}

// matchPrefix reports whether every glob segment matches the path segment in
// the same position. The path may be longer: a pattern claims the subtree
// beneath it, so "/v1/projects/*/secrets" also owns
// "/v1/projects/p/secrets/my-secret/versions/1".
func matchPrefix(glob, path []string) bool {
	if len(glob) > len(path) {
		return false
	}
	for i, seg := range glob {
		if seg == "*" {
			continue
		}
		if seg != path[i] {
			return false
		}
	}
	return true
}

// splitPath normalises a URL path into segments, dropping the query string.
func splitPath(path string) []string {
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func minString(a, b string) string {
	if a < b {
		return a
	}
	return b
}

// ─────────────────────────────────────────────────────────────────────────────
// Operation polling
//
// /v1/projects/*/locations/*/operations/{id} is not one service's path. GKE,
// Cloud Functions, Artifact Registry, Memorystore and others all expose it, and
// real GCP separates them by hostname — which a client pointed at the gateway
// does not send. Claiming the path for whichever service seems most likely
// would silently answer another service's polls.
//
// The id itself carries the answer. Whichever service created the operation
// recorded its kind on the shared OperationManager, so the poll can be routed
// back to its origin exactly, the same way a real client polls the service it
// called.
// ─────────────────────────────────────────────────────────────────────────────

// operationLookup reports which domain minted an operation. It is satisfied by
// orchestrator.OperationManager and stubbed in tests.
type operationLookup interface {
	Get(name string) *orchestrator.Operation
}

// resolveOperation returns the domain that minted the operation named in path,
// or "" when the path is not an operation poll or the operation is unknown.
func resolveOperation(path string, ops operationLookup, kinds map[string]string) string {
	if ops == nil || len(kinds) == 0 {
		return ""
	}

	name := operationNameFrom(path)
	if name == "" {
		return ""
	}

	op := ops.Get(name)
	if op == nil {
		return ""
	}
	return kinds[op.Kind]
}

// operationNameFrom returns the id following an "operations" segment, or "" if
// the path does not address a single operation. A collection path such as
// ".../operations" has no id and cannot be attributed to anyone.
func operationNameFrom(path string) string {
	segments := splitPath(path)
	for i := 0; i < len(segments)-1; i++ {
		if segments[i] == "operations" {
			return segments[i+1]
		}
	}
	return ""
}
