package compute

import (
	"log"

	"minisky/pkg/orchestrator"
	"minisky/pkg/persist"
)

// ─────────────────────────────────────────────────────────────────────────────
// Metadata persistence
//
// The Docker containers backing instances outlive the daemon, but the shim's
// record of them did not: after a restart `instances list` was empty while the
// VMs were still running, and Terraform read every network, firewall rule and
// instance as deleted. Firewall rules were worse than forgotten — the service
// manager's enforcement registry was rebuilt empty, so rules that still existed
// in state stopped being applied.
// ─────────────────────────────────────────────────────────────────────────────

// stateName is the file under ~/.minisky this metadata is kept in.
const stateName = "compute_metadata"

type persistedState struct {
	Instances        map[string]*Instance       `json:"instances"`
	Networks         map[string]*Network        `json:"networks"`
	SecurityPolicies map[string]*SecurityPolicy `json:"securityPolicies"`
	Firewalls        map[string]*FirewallRule   `json:"firewalls"`
}

// restore loads saved metadata. It runs during construction, before the shim
// serves anything, so it takes no lock.
func (api *API) restore() {
	var stored persistedState
	if err := persist.Load(stateName, &stored); err != nil {
		log.Printf("[Shim: Compute] ignoring unreadable metadata: %v", err)
		return
	}
	if len(stored.Instances)+len(stored.Networks)+len(stored.Firewalls)+len(stored.SecurityPolicies) == 0 {
		return
	}

	for key, n := range stored.Networks {
		api.networks[key] = n
	}
	for key, sp := range stored.SecurityPolicies {
		api.securityPolicies[key] = sp
	}
	for key, inst := range stored.Instances {
		api.instances[key] = inst
	}
	for key, fw := range stored.Firewalls {
		api.firewalls[key] = fw
	}

	api.reconcileInstanceStatus()
	api.reregisterFirewallRules()

	log.Printf("[Shim: Compute] restored %d instance(s), %d network(s), %d firewall rule(s)",
		len(stored.Instances), len(stored.Networks), len(stored.Firewalls))
}

// reconcileInstanceStatus replaces the stored status with what Docker actually
// reports. A container can be stopped or removed while the daemon is down, and
// reporting the last status the shim happened to write would be a claim about
// the world rather than a reading of it.
func (api *API) reconcileInstanceStatus() {
	if api.svcMgr == nil {
		return
	}
	for key, inst := range api.instances {
		containerName := inst.Labels[containerMappingLabel]
		if containerName == "" {
			continue
		}
		status, err := api.svcMgr.CheckStatusPublic(containerName)
		if err != nil {
			// Docker is unreachable; the stored status is the best available
			// answer and saying nothing is better than saying the wrong thing.
			continue
		}
		switch status {
		case "running":
			inst.Status = "RUNNING"
		case "not_found":
			// The VM was removed behind the shim's back, so it no longer exists.
			delete(api.instances, key)
		default:
			inst.Status = "TERMINATED"
		}
	}
}

// reregisterFirewallRules puts the restored rules back into the service
// manager, which holds enforcement state of its own and starts empty.
func (api *API) reregisterFirewallRules() {
	if api.svcMgr == nil {
		return
	}
	for _, fw := range api.firewalls {
		action, protocol, ports := firewallEffect(fw)
		direction := fw.Direction
		if direction == "" {
			direction = "INGRESS"
		}
		vpc := extractNameFromURL(fw.Network)
		api.svcMgr.RegisterFirewallRule(vpc, orchestrator.FirewallEntry{
			Name:      fw.Name,
			VpcName:   vpc,
			Direction: direction,
			Action:    action,
			Protocol:  protocol,
			Ports:     ports,
			Ranges:    append(fw.SourceRanges, fw.DestinationRanges...),
		})
	}
}

// saveLocked writes the metadata out. Callers hold api.mu.
func (api *API) saveLocked() {
	state := persistedState{
		Instances:        api.instances,
		Networks:         api.networks,
		SecurityPolicies: api.securityPolicies,
		Firewalls:        api.firewalls,
	}
	if err := persist.Save(stateName, state); err != nil {
		log.Printf("[Shim: Compute] could not persist metadata: %v", err)
	}
}
