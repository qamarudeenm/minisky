package cloudbilling

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. google_project reads billing on every refresh, so a link that
// disappears on restart shows up as drift on a resource the user never touched.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "cloudbilling_state"

type persistedState struct {
	Linked   map[string]string          `json:"linked"`
	Accounts map[string]*BillingAccount `json:"accounts"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock. The default account seeded by the
// constructor stays unless a stored one replaces it.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: Billing] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Linked {
		api.linked[key] = v
	}
	for key, v := range stored.Accounts {
		api.accounts[key] = v
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
	state := persistedState{Linked: api.linked, Accounts: api.accounts}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: Billing] could not persist state: %v", err)
	}
}
