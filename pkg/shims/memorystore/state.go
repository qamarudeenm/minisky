package memorystore

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. A Memorystore instance is a Redis or Memcached container that
// keeps running after the daemon stops. Losing the record left the container
// holding the port with no instance resource pointing at it.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "memorystore_state"

type persistedState struct {
	Redis    map[string]map[string]*Instance `json:"redis,omitempty"`
	Memcache map[string]map[string]*Instance `json:"memcache,omitempty"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: Memorystore] ignoring unreadable state: %v", err)
		return
	}
	for project, instances := range stored.Redis {
		api.redisInstances[project] = instances
	}
	for project, instances := range stored.Memcache {
		api.memcacheInstances[project] = instances
	}
	if len(stored.Redis)+len(stored.Memcache) > 0 {
		log.Printf("[Shim: Memorystore] restored instances for %d project(s)",
			len(stored.Redis)+len(stored.Memcache))
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
	state := persistedState{Redis: api.redisInstances, Memcache: api.memcacheInstances}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: Memorystore] could not persist state: %v", err)
	}
}
