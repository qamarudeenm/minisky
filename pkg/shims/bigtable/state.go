package bigtable

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. The Bigtable emulator itself holds row data in memory and cannot
// persist it, but instance and table definitions are Terraform-managed
// resources: forgetting them makes every plan after a restart propose to create
// what is already declared.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "bigtable_state"

type persistedState struct {
	Instances map[string]*Instance `json:"instances"`
	Tables    map[string]*Table    `json:"tables"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: Bigtable] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Instances {
		api.instances[key] = v
	}
	for key, v := range stored.Tables {
		api.tables[key] = v
	}
	if len(stored.Instances) > 0 {
		log.Printf("[Shim: Bigtable] restored %d instance(s) and %d table(s)",
			len(stored.Instances), len(stored.Tables))
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
	state := persistedState{Instances: api.instances, Tables: api.tables}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: Bigtable] could not persist state: %v", err)
	}
}
