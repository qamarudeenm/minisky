package iam

import (
	"log"

	"minisky/pkg/persist"
)

// ─────────────────────────────────────────────────────────────────────────────
// Persistence
//
// Service accounts and the policies bound to them were process-lifetime state.
// A restart left every service account gone while the workloads configured to
// authenticate as one were still running, and Terraform read the whole IAM
// surface as deleted.
//
// Key material is written along with the rest. The keys are self-issued by the
// emulator and the state file is created owner-only, so a key that keeps
// working across a restart is both what a caller expects and no more exposed
// than the credentials gcloud already keeps in the home directory.
// ─────────────────────────────────────────────────────────────────────────────

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "iam_state"

type persistedState struct {
	ServiceAccounts map[string]*ServiceAccount      `json:"serviceAccounts"`
	Keys            map[string][]*ServiceAccountKey `json:"keys,omitempty"`
	Policies        map[string]*IamPolicy           `json:"policies,omitempty"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: IAM] ignoring unreadable state: %v", err)
		return
	}

	for key, sa := range stored.ServiceAccounts {
		api.serviceAccounts[key] = sa
	}
	for key, keys := range stored.Keys {
		api.keys[key] = keys
	}
	for resource, policy := range stored.Policies {
		api.policies[resource] = policy
	}

	if len(stored.ServiceAccounts) > 0 || len(stored.Policies) > 0 {
		log.Printf("[Shim: IAM] restored %d service account(s) and %d policy binding set(s)",
			len(stored.ServiceAccounts), len(stored.Policies))
	}
}

// saveLocked writes the state out. Callers hold api.mu.
func (api *API) saveLocked() {
	state := persistedState{
		ServiceAccounts: api.serviceAccounts,
		Keys:            api.keys,
		Policies:        api.policies,
	}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: IAM] could not persist state: %v", err)
	}
}
