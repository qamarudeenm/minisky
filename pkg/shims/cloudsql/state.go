package cloudsql

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. A Cloud SQL instance is a Postgres or MySQL container holding the
// user's actual database. The container and its data survive the daemon; only
// the shim's record of them did not, so after a restart the database was still
// running with no instance, user or database resource describing it.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "cloudsql_state"

type persistedState struct {
	Instances map[string]*DatabaseInstance `json:"instances"`
	Databases map[string][]*Database       `json:"databases,omitempty"`
	Users     map[string][]*User           `json:"users,omitempty"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: CloudSQL] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Instances {
		api.instances[key] = v
	}
	for key, v := range stored.Databases {
		api.databases[key] = v
	}
	for key, v := range stored.Users {
		api.users[key] = v
	}
	if len(stored.Instances) > 0 {
		log.Printf("[Shim: CloudSQL] restored %d instance(s)", len(stored.Instances))
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
	state := persistedState{Instances: api.instances, Databases: api.databases, Users: api.users}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: CloudSQL] could not persist state: %v", err)
	}
}
