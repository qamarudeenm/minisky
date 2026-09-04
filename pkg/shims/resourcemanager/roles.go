package resourcemanager

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"minisky/pkg/config"
)

//go:embed roles.json
var embeddedRoles []byte

// Role is one predefined role and the permissions it carries.
type Role struct {
	Title       string   `json:"title"`
	Permissions []string `json:"permissions"`
	// Includes names roles whose permissions this one also grants, which is how
	// the basic roles nest: owner contains editor contains viewer.
	Includes []string `json:"includes,omitempty"`
}

type roleCatalogue struct {
	Roles map[string]Role `json:"roles"`
}

var (
	rolesOnce sync.Once
	roles     map[string]Role
)

// loadRoles reads the embedded catalogue, then overlays ~/.minisky/roles.json
// so a user can correct or extend it without rebuilding.
//
// The embedded set is deliberately partial: Google publishes roughly 1,250
// roles and 10,000 permissions with no offline feed, so this covers what a
// landing zone uses and nothing more. Everything that reports a permission
// answer says which list it came from, because an absent permission here means
// "not in the curated set", not "denied".
func loadRoles() map[string]Role {
	rolesOnce.Do(func() {
		var catalogue roleCatalogue
		if err := json.Unmarshal(embeddedRoles, &catalogue); err != nil {
			log.Printf("[Shim: ResourceManager] embedded role catalogue is unreadable: %v", err)
			catalogue.Roles = map[string]Role{}
		}
		roles = catalogue.Roles

		path := filepath.Join(config.GetMiniskyDir(), "roles.json")
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		var overlay roleCatalogue
		if err := json.Unmarshal(data, &overlay); err != nil {
			log.Printf("[Shim: ResourceManager] ignoring unreadable %s: %v", path, err)
			return
		}
		for name, role := range overlay.Roles {
			roles[name] = role
		}
		log.Printf("[Shim: ResourceManager] role catalogue extended with %d role(s) from %s",
			len(overlay.Roles), path)
	})
	return roles
}

// permissionsFor expands a role into every permission it grants, following the
// roles it includes. known reports whether the catalogue has heard of the role
// at all, which is the difference between "grants nothing" and "cannot say".
func permissionsFor(name string) (permissions []string, known bool) {
	catalogue := loadRoles()
	seen := map[string]bool{}
	visited := map[string]bool{}

	var walk func(string) bool
	walk = func(role string) bool {
		if visited[role] {
			return true
		}
		visited[role] = true

		definition, ok := catalogue[role]
		if !ok {
			return false
		}
		for _, p := range definition.Permissions {
			seen[p] = true
		}
		for _, included := range definition.Includes {
			walk(included)
		}
		return true
	}

	if !walk(name) {
		return nil, false
	}

	for p := range seen {
		permissions = append(permissions, p)
	}
	sort.Strings(permissions)
	return permissions, true
}

// roleGrants reports whether a role carries a permission, and whether the role
// was known well enough for the answer to mean anything.
func roleGrants(role, permission string) (grants, known bool) {
	permissions, known := permissionsFor(role)
	if !known {
		return false, false
	}
	for _, p := range permissions {
		if p == permission {
			return true, true
		}
		// A permission group such as compute.instances.* is not GCP syntax, but
		// a hand-written catalogue may use it, so honour a trailing wildcard.
		if strings.HasSuffix(p, ".*") && strings.HasPrefix(permission, strings.TrimSuffix(p, "*")) {
			return true, true
		}
	}
	return false, true
}
