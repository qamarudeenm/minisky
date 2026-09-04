package dns

import (
	"log"
	"net/http"

	"minisky/pkg/persist"
)

// Persistence. A managed zone and its record sets are Terraform-managed, and the
// zone's numeric id is part of its identity — regenerating it on restart would
// make the same zone read as a different resource.

// stateName is the file under ~/.minisky this state is kept in.
const stateName = "dns_state"

// persistedZone mirrors zoneStore, whose fields are unexported and so invisible
// to the JSON encoder.
type persistedZone struct {
	Zone      *ManagedZone                  `json:"zone"`
	Rrsets    map[string]*ResourceRecordSet `json:"rrsets,omitempty"`
	Changes   []*Change                     `json:"changes,omitempty"`
	ChangeSeq int                           `json:"changeSeq"`
}

type persistedState struct {
	Zones   map[string]*persistedZone `json:"zones"`
	ZoneSeq uint64                    `json:"zoneSeq"`
}

// restore loads saved state. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: DNS] ignoring unreadable state: %v", err)
		return
	}
	for key, z := range stored.Zones {
		rrsets := z.Rrsets
		if rrsets == nil {
			rrsets = map[string]*ResourceRecordSet{}
		}
		api.zones[key] = &zoneStore{
			zone:      z.Zone,
			rrsets:    rrsets,
			changes:   z.Changes,
			changeSeq: z.ChangeSeq,
		}
	}
	// The sequence continues where it left off, so a new zone cannot take an id
	// a deleted one already used.
	api.zoneSeq = stored.ZoneSeq
	if len(stored.Zones) > 0 {
		log.Printf("[Shim: DNS] restored %d managed zone(s)", len(stored.Zones))
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
	state := persistedState{Zones: make(map[string]*persistedZone, len(api.zones)), ZoneSeq: api.zoneSeq}
	for key, z := range api.zones {
		state.Zones[key] = &persistedZone{
			Zone:      z.zone,
			Rrsets:    z.rrsets,
			Changes:   z.changes,
			ChangeSeq: z.changeSeq,
		}
	}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: DNS] could not persist state: %v", err)
	}
}
