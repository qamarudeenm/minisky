package artifactregistry

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. Repositories are Terraform-managed and back a running Docker
// registry, so a forgotten repository is one the user's images are still in.
//
// This shim has no mutex — the repository map is written directly by the
// handlers — so the snapshot is taken without one rather than introducing
// locking this package does not otherwise have.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "artifactregistry_state"

type persistedState struct {
	Repos map[string]*Repository `json:"repos"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: ArtifactRegistry] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Repos {
		api.repos[key] = v
	}
	if len(stored.Repos) > 0 {
		log.Printf("[Shim: ArtifactRegistry] restored %d repository(ies)", len(stored.Repos))
	}
}

// persistIfMutated snapshots the state after a request that could have changed it.
func (api *API) persistIfMutated(r *http.Request) {
	if !persist.Mutating(r) {
		return
	}
	api.save()
}

// save writes the state out.
func (api *API) save() {
	if err := persist.Save(stateName, persistedState{Repos: api.repos}); err != nil {
		log.Printf("[Shim: ArtifactRegistry] could not persist state: %v", err)
	}
}
