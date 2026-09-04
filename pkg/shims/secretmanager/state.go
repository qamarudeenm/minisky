package secretmanager

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. A secret whose payload is gone after a restart takes every
// workload that reads it at boot. The payloads are written to disk, into a file
// the helper creates owner-only — the same trade the local gcloud credential
// store already makes.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "secretmanager_state"

// persistedSecret mirrors secret, whose version list is unexported and so
// invisible to the JSON encoder.
type persistedSecret struct {
	Secret   *secret          `json:"secret"`
	Versions []*secretVersion `json:"versions,omitempty"`
}

type persistedState struct {
	Store map[string]map[string]*persistedSecret `json:"store"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: SecretManager] ignoring unreadable state: %v", err)
		return
	}

	secrets := 0
	for project, storedSecrets := range stored.Store {
		out := make(map[string]*secret, len(storedSecrets))
		for id, ps := range storedSecrets {
			s := ps.Secret
			s.versions = ps.Versions
			out[id] = s
			secrets++
		}
		api.store[project] = out
	}
	if secrets > 0 {
		log.Printf("[Shim: SecretManager] restored %d secret(s)", secrets)
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
	state := persistedState{Store: make(map[string]map[string]*persistedSecret, len(api.store))}
	for project, secrets := range api.store {
		out := make(map[string]*persistedSecret, len(secrets))
		for id, s := range secrets {
			out[id] = &persistedSecret{Secret: s, Versions: s.versions}
		}
		state.Store[project] = out
	}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: SecretManager] could not persist state: %v", err)
	}
}
