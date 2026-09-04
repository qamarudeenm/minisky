package dataproc

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. A Dataproc cluster is backed by Spark containers that outlive the
// daemon, so forgetting the cluster leaves the containers running with nothing
// able to address or delete them.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "dataproc_state"

type persistedState struct {
	Clusters map[string]*Cluster `json:"clusters"`
	Jobs     map[string]*Job     `json:"jobs,omitempty"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: Dataproc] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Clusters {
		api.clusters[key] = v
	}
	for key, v := range stored.Jobs {
		api.jobs[key] = v
	}
	if len(stored.Clusters) > 0 {
		log.Printf("[Shim: Dataproc] restored %d cluster(s)", len(stored.Clusters))
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
	state := persistedState{Clusters: api.clusters, Jobs: api.jobs}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: Dataproc] could not persist state: %v", err)
	}
}
