package scheduler

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. A scheduler job that survives as a record but not as a cron entry
// is worse than one that is forgotten: it reads back as ENABLED from the API
// while never firing again. Restoring re-arms the cron alongside the record.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "scheduler_state"

type persistedState struct {
	Jobs map[string]*Job `json:"jobs"`
}

// restore loads saved state and re-arms the cron. It runs during construction,
// before the shim serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: Scheduler] ignoring unreadable state: %v", err)
		return
	}
	for name, job := range stored.Jobs {
		api.jobs[name] = job
		// scheduleJobLocked skips anything not ENABLED, which is what a paused
		// job should keep doing after a restart.
		api.scheduleJobLocked(job)
	}
	if len(stored.Jobs) > 0 {
		log.Printf("[Shim: Scheduler] restored and rescheduled %d job(s)", len(stored.Jobs))
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
	if err := persist.Save(stateName, persistedState{Jobs: api.jobs}); err != nil {
		log.Printf("[Shim: Scheduler] could not persist state: %v", err)
	}
}
