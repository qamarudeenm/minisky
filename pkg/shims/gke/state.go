package gke

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. A cluster is a kind cluster in Docker that survives the daemon.
// Forgetting the record leaves the kind cluster running and unreachable through
// the API that created it.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "gke_state"

type persistedState struct {
	Clusters map[string]*Cluster `json:"clusters"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: GKE] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Clusters {
		api.clusters[key] = v
	}
	if len(stored.Clusters) > 0 {
		log.Printf("[Shim: GKE] restored %d cluster(s)", len(stored.Clusters))
	}
}

// persistIfMutated snapshots the state after a request that could have changed it.
func (api *API) persistIfMutated(r *http.Request) {
	if !persist.Mutating(r) {
		return
	}
	api.mu.RLock()
	defer api.mu.RUnlock()
	api.saveLocked()
}

// saveLocked writes the state out. Callers hold api.mu.
func (api *API) saveLocked() {
	state := persistedState{Clusters: api.clusters}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: GKE] could not persist state: %v", err)
	}
}

// runAndPersist runs a long-running operation's callback and snapshots the state
// afterwards. These callbacks finish after the response has been sent, so the
// state they change is not covered by the snapshot ServeHTTP takes.
func (api *API) runAndPersist(opName string, fn func() error) {
	api.opMgr.RunAsync(opName, func() error {
		defer func() {
			api.mu.RLock()
			defer api.mu.RUnlock()
			api.saveLocked()
		}()
		return fn()
	})
}
