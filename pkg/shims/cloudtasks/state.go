package cloudtasks

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. Queues are Terraform-managed. Tasks are kept with them because a
// queue that comes back empty silently drops work that was accepted with a 200.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "cloudtasks_state"

type persistedState struct {
	Queues map[string]*Queue  `json:"queues"`
	Tasks  map[string][]*Task `json:"tasks,omitempty"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: CloudTasks] ignoring unreadable state: %v", err)
		return
	}
	for key, v := range stored.Queues {
		api.queues[key] = v
	}
	for key, v := range stored.Tasks {
		api.tasks[key] = v
	}
	if len(stored.Queues) > 0 {
		log.Printf("[Shim: CloudTasks] restored %d queue(s)", len(stored.Queues))
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
	state := persistedState{Queues: api.queues, Tasks: api.tasks}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: CloudTasks] could not persist state: %v", err)
	}
}
