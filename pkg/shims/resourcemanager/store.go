package resourcemanager

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"minisky/pkg/config"
)

// maxHierarchyDepth matches Google's limit on folder nesting beneath an
// organization.
const maxHierarchyDepth = 10

// store holds the hierarchy and writes it to disk. A landing zone that
// evaporates when the daemon restarts cannot be used to test a landing zone,
// so this is persisted where the rest of MiniSky keeps its state.
type store struct {
	mu sync.RWMutex

	Organizations map[string]*Organization `json:"organizations"` // key: organizations/{id}
	Folders       map[string]*Folder       `json:"folders"`       // key: folders/{id}
	Projects      map[string]*project      `json:"projects"`      // key: projectId
	Policies      map[string]*IamPolicy    `json:"policies"`      // key: resource name

	path string
}

// newStoreAt opens a hierarchy at an explicit path, which lets a test reopen
// the same file the way a restart would.
func newStoreAt(path string) *store {
	s := &store{
		Organizations: map[string]*Organization{},
		Folders:       map[string]*Folder{},
		Projects:      map[string]*project{},
		Policies:      map[string]*IamPolicy{},
		path:          path,
	}
	s.load()
	s.seedOrganization()
	return s
}

func newStore() *store {
	s := &store{
		Organizations: map[string]*Organization{},
		Folders:       map[string]*Folder{},
		Projects:      map[string]*project{},
		Policies:      map[string]*IamPolicy{},
		path:          filepath.Join(config.GetMiniskyDir(), "resource_hierarchy.json"),
	}
	s.load()
	s.seedOrganization()
	return s
}

func (s *store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, s); err != nil {
		log.Printf("[Shim: ResourceManager] ignoring unreadable hierarchy at %s: %v", s.path, err)
		return
	}
	log.Printf("[Shim: ResourceManager] loaded %d folder(s) and %d project(s) from persistence",
		len(s.Folders), len(s.Projects))
}

// save persists the hierarchy. Callers hold the lock.
func (s *store) save() {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		log.Printf("[Shim: ResourceManager] could not persist hierarchy: %v", err)
	}
}

// seedOrganization gives the hierarchy a root. Google Cloud organizations come
// from Cloud Identity and cannot be created through this API, so without one
// there would be nothing to parent a folder to.
func (s *store) seedOrganization() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.Organizations) > 0 {
		return
	}

	domain := os.Getenv("MINISKY_ORGANIZATION_DOMAIN")
	if domain == "" {
		domain = "minisky.local"
	}
	id := os.Getenv("MINISKY_ORGANIZATION_ID")
	if id == "" {
		id = "100000000000"
	}

	name := "organizations/" + id
	now := time.Now().UTC().Format(time.RFC3339)
	s.Organizations[name] = &Organization{
		Name:           name,
		DisplayName:    domain,
		State:          StateActive,
		LifecycleState: StateActive,
		Owner:          &OrganizationOwner{DirectoryCustomerID: "C" + randomDigits(8)},
		CreateTime:     now,
		CreationTime:   now,
		Etag:           newEtag(),
	}
	s.save()
	log.Printf("[Shim: ResourceManager] seeded %s (%s); override with MINISKY_ORGANIZATION_ID / _DOMAIN",
		name, domain)
}

// ─────────────────────────────────────────────────────────────────────────────
// Lookups
// ─────────────────────────────────────────────────────────────────────────────

// resourceExists reports whether a parent reference points at something real.
func (s *store) resourceExists(name string) bool {
	switch {
	case strings.HasPrefix(name, "organizations/"):
		_, ok := s.Organizations[name]
		return ok
	case strings.HasPrefix(name, "folders/"):
		f, ok := s.Folders[name]
		return ok && f.State == StateActive
	}
	return false
}

// depthOf counts how many folders sit between name and its organization.
func (s *store) depthOf(name string) int {
	depth := 0
	for strings.HasPrefix(name, "folders/") {
		folder, ok := s.Folders[name]
		if !ok {
			return depth
		}
		depth++
		name = folder.Parent
		if depth > maxHierarchyDepth+1 {
			return depth
		}
	}
	return depth
}

// hasChildren reports whether anything still lives under a folder. Google
// refuses to delete a folder that is not empty.
func (s *store) hasChildren(name string) bool {
	for _, f := range s.Folders {
		if f.Parent == name && f.State == StateActive {
			return true
		}
	}
	for _, p := range s.Projects {
		if p.Parent == name && p.State == StateActive {
			return true
		}
	}
	return false
}

// siblingNameTaken reports whether a display name is already used under the
// same parent, which Google rejects.
func (s *store) siblingNameTaken(parent, displayName, exceptName string) bool {
	for name, f := range s.Folders {
		if name == exceptName || f.State != StateActive {
			continue
		}
		if f.Parent == parent && strings.EqualFold(f.DisplayName, displayName) {
			return true
		}
	}
	return false
}

// wouldCycle reports whether moving name under newParent would make the
// hierarchy point at itself.
func (s *store) wouldCycle(name, newParent string) bool {
	for cursor := newParent; strings.HasPrefix(cursor, "folders/"); {
		if cursor == name {
			return true
		}
		folder, ok := s.Folders[cursor]
		if !ok {
			return false
		}
		cursor = folder.Parent
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Identifiers
// ─────────────────────────────────────────────────────────────────────────────

// newFolderID mints the numeric id Google assigns to a folder. Callers choose a
// display name; they never choose the id.
func (s *store) newFolderID() string {
	for {
		id := randomDigits(12)
		if _, taken := s.Folders["folders/"+id]; !taken {
			return id
		}
	}
}

// newProjectNumber mints the immutable number a project carries alongside the
// id its creator picked.
func (s *store) newProjectNumber() string {
	for {
		number := randomDigits(12)
		unique := true
		for _, p := range s.Projects {
			if p.Number == number {
				unique = false
				break
			}
		}
		if unique {
			return number
		}
	}
}

func randomDigits(n int) string {
	const digits = "0123456789"
	out := make([]byte, n)
	out[0] = digits[1+rand.Intn(9)] // never leading zero
	for i := 1; i < n; i++ {
		out[i] = digits[rand.Intn(len(digits))]
	}
	return string(out)
}

func newEtag() string {
	return fmt.Sprintf("BwX%s", randomDigits(10))
}
