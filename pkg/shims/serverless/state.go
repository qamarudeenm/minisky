package serverless

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. Cloud Run services and Cloud Functions are containers built from
// the user's source. Rebuilding one costs a buildpacks run, so losing the record
// of a deployed service is expensive as well as wrong — and the container keeps
// serving traffic that the API can no longer describe.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "serverless_state"

type persistedState struct {
	Functions map[string]*Function `json:"functions,omitempty"`
	Services  map[string]*Service  `json:"services,omitempty"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: Serverless] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Functions {
		api.functions[key] = v
	}
	for key, v := range stored.Services {
		api.services[key] = v
	}
	if len(stored.Functions)+len(stored.Services) > 0 {
		log.Printf("[Shim: Serverless] restored %d function(s) and %d service(s)",
			len(stored.Functions), len(stored.Services))
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
	state := persistedState{Functions: api.functions, Services: api.services}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: Serverless] could not persist state: %v", err)
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
