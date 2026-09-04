package appengine

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. A deployed version is a container built from the user's source;
// losing the application, service and version records leaves it running with
// nothing able to describe, route to or delete it.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "appengine_state"

type persistedState struct {
	Apps     map[string]*App                           `json:"apps"`
	Services map[string]map[string]*Service            `json:"services,omitempty"`
	Versions map[string]map[string]map[string]*Version `json:"versions,omitempty"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: AppEngine] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Apps {
		api.apps[key] = v
	}
	for key, v := range stored.Services {
		api.services[key] = v
	}
	for key, v := range stored.Versions {
		api.versions[key] = v
	}
	if len(stored.Apps) > 0 {
		log.Printf("[Shim: AppEngine] restored %d application(s)", len(stored.Apps))
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
	state := persistedState{Apps: api.apps, Services: api.services, Versions: api.versions}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: AppEngine] could not persist state: %v", err)
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
