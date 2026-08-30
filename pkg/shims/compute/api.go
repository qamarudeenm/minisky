package compute

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"minisky/pkg/config"
	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("compute.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.OpMgr, ctx.SvcMgr)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource types
// ─────────────────────────────────────────────────────────────────────────────

// Instance represents a GCE VM with its full lifecycle state.
type Instance struct {
	Kind              string                     `json:"kind"`
	ID                string                     `json:"id"`
	Name              string                     `json:"name"`
	Zone              string                     `json:"zone"`
	MachineType       string                     `json:"machineType"`
	Status            string                     `json:"status"`
	SelfLink          string                     `json:"selfLink"`
	Description       string                     `json:"description"`
	Labels            map[string]string          `json:"labels,omitempty"`
	Metadata          *InstanceMetadata          `json:"metadata,omitempty"`
	NetworkInterfaces []NetworkInterface         `json:"networkInterfaces"`
	Disks             []AttachedDisk             `json:"disks"`
	CreationTimestamp string                     `json:"creationTimestamp"`
	HostPorts         []orchestrator.PortMapping `json:"hostPorts,omitempty"`
	// internal tracking only
	project          string
	zone             string
	Fingerprint      string      `json:"fingerprint"`
	LabelFingerprint string      `json:"labelFingerprint"`
	Scheduling       *Scheduling `json:"scheduling,omitempty"`
	CanIpForward     bool        `json:"canIpForward"`
}

func (i *Instance) DeepCopy() *Instance {
	newInst := *i
	if i.Labels != nil {
		newInst.Labels = make(map[string]string)
		for k, v := range i.Labels {
			newInst.Labels[k] = v
		}
	}
	if i.Metadata != nil {
		newInst.Metadata = &InstanceMetadata{
			Kind: i.Metadata.Kind,
		}
		newInst.Metadata.Items = append([]MetadataItem{}, i.Metadata.Items...)
	}
	if i.NetworkInterfaces != nil {
		newInst.NetworkInterfaces = make([]NetworkInterface, len(i.NetworkInterfaces))
		copy(newInst.NetworkInterfaces, i.NetworkInterfaces)
		for j := range newInst.NetworkInterfaces {
			if newInst.NetworkInterfaces[j].AccessConfigs != nil {
				newInst.NetworkInterfaces[j].AccessConfigs = append([]AccessConfig{}, newInst.NetworkInterfaces[j].AccessConfigs...)
			}
		}
	}
	if i.Disks != nil {
		newInst.Disks = make([]AttachedDisk, len(i.Disks))
		copy(newInst.Disks, i.Disks)
	}
	if i.HostPorts != nil {
		newInst.HostPorts = append([]orchestrator.PortMapping{}, i.HostPorts...)
	}
	if i.Scheduling != nil {
		s := *i.Scheduling
		newInst.Scheduling = &s
	}
	return &newInst
}

type Scheduling struct {
	OnHostMaintenance string `json:"onHostMaintenance"`
	AutomaticRestart  bool   `json:"automaticRestart"`
	Preemptible       bool   `json:"preemptible"`
}

type InstanceMetadata struct {
	Kind  string         `json:"kind"`
	Items []MetadataItem `json:"items,omitempty"`
}

type MetadataItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type NetworkInterface struct {
	Kind          string         `json:"kind"`
	Name          string         `json:"name"`
	Network       string         `json:"network"`
	NetworkIP     string         `json:"networkIP"`
	Subnetwork    string         `json:"subnetwork,omitempty"`
	AccessConfigs []AccessConfig `json:"accessConfigs,omitempty"`
}

type AccessConfig struct {
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	Type  string `json:"type"` // ONE_TO_ONE_NAT
	NatIP string `json:"natIP,omitempty"`
}

type AttachedDisk struct {
	Kind             string                `json:"kind"`
	Type             string                `json:"type"` // PERSISTENT, SCRATCH
	Mode             string                `json:"mode"` // READ_WRITE, READ_ONLY
	Source           string                `json:"source,omitempty"`
	DeviceName       string                `json:"deviceName"`
	Boot             bool                  `json:"boot"`
	AutoDelete       bool                  `json:"autoDelete"`
	InitializeParams *DiskInitializeParams `json:"initializeParams,omitempty"`
}

// DiskInitializeParams carries the boot disk's creation parameters. Only the
// fields the shim acts on are modelled; the rest are round-tripped as sent.
// diskSizeGb is deliberately absent — the API types it as a string but clients
// differ, and decoding it strictly would reject an otherwise valid request.
type DiskInitializeParams struct {
	DiskName    string `json:"diskName,omitempty"`
	DiskType    string `json:"diskType,omitempty"`
	SourceImage string `json:"sourceImage,omitempty"`
}

// Network represents a VPC network.
type Network struct {
	Kind                  string `json:"kind"`
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	Description           string `json:"description,omitempty"`
	SelfLink              string `json:"selfLink"`
	AutoCreateSubnetworks bool   `json:"autoCreateSubnetworks"`
	CreationTimestamp     string `json:"creationTimestamp"`

	// GCE always reports this, defaulting to AFTER_CLASSIC_FIREWALL. Omitting it
	// made every terraform plan propose the same in-place update forever.
	NetworkFirewallPolicyEnforcementOrder string `json:"networkFirewallPolicyEnforcementOrder,omitempty"`
}

// SecurityPolicy represents a Cloud Armor WAF rule set.
type SecurityPolicy struct {
	Kind              string               `json:"kind"`
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Description       string               `json:"description,omitempty"`
	SelfLink          string               `json:"selfLink"`
	Rules             []SecurityPolicyRule `json:"rules"`
	CreationTimestamp string               `json:"creationTimestamp"`
}

type SecurityPolicyRule struct {
	Priority    int        `json:"priority"`
	Action      string     `json:"action"`
	Description string     `json:"description,omitempty"`
	Match       *RuleMatch `json:"match,omitempty"`
}

type RuleMatch struct {
	VersionedExpr string           `json:"versionedExpr,omitempty"` // SRC_IPS_V1
	Config        *RuleMatchConfig `json:"config,omitempty"`
}

type RuleMatchConfig struct {
	SrcIPRanges []string `json:"srcIpRanges,omitempty"`
}

// FirewallRule represents a VPC firewall rule.
type FirewallRule struct {
	Kind              string          `json:"kind"`
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description,omitempty"`
	Network           string          `json:"network"`
	Priority          int             `json:"priority"`
	Direction         string          `json:"direction"` // INGRESS, EGRESS
	Action            string          `json:"action"`    // allow, deny
	SourceRanges      []string        `json:"sourceRanges,omitempty"`
	DestinationRanges []string        `json:"destinationRanges,omitempty"`
	Allowed           []FirewallAllow `json:"allowed,omitempty"`
	Denied            []FirewallAllow `json:"denied,omitempty"`
	TargetTags        []string        `json:"targetTags,omitempty"`
	Disabled          bool            `json:"disabled"`
	SelfLink          string          `json:"selfLink"`
	CreationTimestamp string          `json:"creationTimestamp"`
}

type FirewallAllow struct {
	IPProtocol string   `json:"IPProtocol"` // tcp, udp, icmp, all
	Ports      []string `json:"ports,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// API shim struct
// ─────────────────────────────────────────────────────────────────────────────

// API is the high-fidelity Compute Engine v1 shim.
type API struct {
	mu               sync.RWMutex
	opMgr            *orchestrator.OperationManager
	svcMgr           *orchestrator.ServiceManager
	instances        map[string]*Instance       // key: project+":"+zone+":"+name
	networks         map[string]*Network        // key: project+":"+name
	securityPolicies map[string]*SecurityPolicy // key: project+":"+name
	firewalls        map[string]*FirewallRule   // key: project+":"+name
}

// NewAPI builds the Compute shim with the shared LRO manager and service manager.
func NewAPI(opMgr *orchestrator.OperationManager, svcMgr *orchestrator.ServiceManager) *API {
	return &API{
		opMgr:            opMgr,
		svcMgr:           svcMgr,
		instances:        make(map[string]*Instance),
		networks:         make(map[string]*Network),
		securityPolicies: make(map[string]*SecurityPolicy),
		firewalls:        make(map[string]*FirewallRule),
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Top-level routing
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) ListProjects() []string {
	api.mu.RLock()
	defer api.mu.RUnlock()

	projects := make(map[string]bool)
	for _, inst := range api.instances {
		if inst.project != "" {
			projects[inst.project] = true
		}
	}
	for k := range api.networks {
		p := strings.Split(k, ":")[0]
		projects[p] = true
	}
	for k := range api.firewalls {
		p := strings.Split(k, ":")[0]
		projects[p] = true
	}

	res := []string{}
	for p := range projects {
		res = append(res, p)
	}
	return res
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Compute Engine] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	switch {
	case strings.Contains(path, "/instances") && strings.Contains(path, "/zones/"):
		api.routeInstances(w, r, path)
	case strings.Contains(path, "/operations/"):
		api.routeOperations(w, r, path)
	case strings.Contains(path, "/zones/") && !strings.Contains(path, "/instances"):
		api.routeZones(w, r, path)
	case strings.Contains(path, "/global/networks"):
		api.routeNetworks(w, r, path)
	case strings.Contains(path, "/global/firewalls"):
		api.routeFirewalls(w, r, path)
	case strings.Contains(path, "/global/securityPolicies"):
		api.routeSecurityPolicies(w, r, path)
	case strings.Contains(path, "/global/backendServices") ||
		strings.Contains(path, "/global/urlMaps") ||
		strings.Contains(path, "/global/forwardingRules") ||
		strings.Contains(path, "/global/targetHttpProxies"):
		api.routeLoadBalancer(w, r, path)
	case strings.Contains(path, "/global/images"):
		api.routeImages(w, r, path)
	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Compute resource not found: "+path)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Instances
// ─────────────────────────────────────────────────────────────────────────────

// containerMappingLabel records which Docker container backs an instance.
// This used to be written into Description, which silently overwrote whatever
// description the caller had set — leaving terraform with permanent drift it
// then could not reconcile.
const containerMappingLabel = "minisky-container"

// setContainerMapping records the backing container without disturbing
// user-owned fields. A description is only synthesised when the caller supplied
// none, so the dashboard still has something to show for hand-made instances.
func (i *Instance) setContainerMapping(containerName string) {
	if i.Labels == nil {
		i.Labels = map[string]string{}
	}
	i.Labels[containerMappingLabel] = containerName
	if i.Description == "" {
		i.Description = fmt.Sprintf("Docker Container ID mapping: %s", containerName)
	}
}

// updateInstance handles instances.update / instances.patch. GCE reconciles the
// mutable fields in place; without this the shim answered 405 and any drift —
// including drift the shim itself introduced — made terraform apply fail.
func (api *API) updateInstance(w http.ResponseWriter, r *http.Request, project, zone, name string) {
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Instance name is required")
		return
	}

	var body struct {
		Description *string           `json:"description"`
		Labels      map[string]string `json:"labels"`
		Metadata    *InstanceMetadata `json:"metadata"`
		Status      *string           `json:"status"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	key := instanceKey(project, zone, name)
	api.mu.Lock()
	inst, ok := api.instances[key]
	if ok {
		if body.Description != nil {
			inst.Description = *body.Description
		}
		if body.Labels != nil {
			// The container mapping is shim-owned: preserve it across an update
			// that does not know about it.
			if mapping, had := inst.Labels[containerMappingLabel]; had {
				if _, sent := body.Labels[containerMappingLabel]; !sent {
					body.Labels[containerMappingLabel] = mapping
				}
			}
			inst.Labels = body.Labels
		}
		if body.Metadata != nil {
			inst.Metadata = body.Metadata
			if inst.Metadata.Items == nil {
				inst.Metadata.Items = []MetadataItem{}
			}
		}
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Instance "+name+" not found")
		return
	}

	op := api.opMgr.Register("compute#operation", "update", selfLinkInstance(project, zone, name), zone, "")
	op.Kind = "compute#operation"
	api.opMgr.RunAsync(op.Name, func() error { return nil })
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

func (api *API) routeInstances(w http.ResponseWriter, r *http.Request, path string) {
	project, zone := extractProjectZone(path)

	// Action suffixes (start / stop / reset)
	switch {
	case strings.HasSuffix(strings.TrimRight(path, "/"), "/start"):
		name := extractSegmentBefore(path, "/start")
		api.instanceAction(w, r, project, zone, name, "start")
		return
	case strings.HasSuffix(strings.TrimRight(path, "/"), "/stop"):
		name := extractSegmentBefore(path, "/stop")
		api.instanceAction(w, r, project, zone, name, "stop")
		return
	}

	// Determine instance name (if present)
	instanceName := extractAfterInstances(path)

	switch r.Method {
	case http.MethodPost:
		api.insertInstance(w, r, project, zone)
	case http.MethodGet:
		if instanceName != "" {
			api.getInstance(w, r, project, zone, instanceName)
		} else {
			api.listInstances(w, r, project, zone)
		}
	case http.MethodPatch, http.MethodPut:
		api.updateInstance(w, r, project, zone, instanceName)
	case http.MethodDelete:
		api.deleteInstance(w, r, project, zone, instanceName)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// insertInstance — handles instances.insert.
// Creates an in-memory instance in PROVISIONING state and kicks off an LRO.
func (api *API) insertInstance(w http.ResponseWriter, r *http.Request, project, zone string) {
	var body struct {
		Name              string             `json:"name"`
		MachineType       string             `json:"machineType"`
		Description       string             `json:"description"`
		Labels            map[string]string  `json:"labels"`
		Metadata          *InstanceMetadata  `json:"metadata"`
		NetworkInterfaces []NetworkInterface `json:"networkInterfaces"`
		Disks             []AttachedDisk     `json:"disks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Request body parse error: "+err.Error())
		return
	}

	name := body.Name
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Field 'name' is required for instances.insert")
		return
	}

	selfLink := selfLinkInstance(project, zone, name)
	targetLink := selfLink
	zoneFull := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s", project, zone)

	// Normalise MachineType (accept short or full form)
	machineType := body.MachineType
	if machineType == "" {
		machineType = "n1-standard-1"
	}
	if !strings.HasPrefix(machineType, "https://") {
		machineType = fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/machineTypes/%s",
			project, zone, machineType)
	}

	// Default network interfaces
	netIfaces := body.NetworkInterfaces
	if len(netIfaces) == 0 {
		netIfaces = []NetworkInterface{{
			Kind:      "compute#networkInterface",
			Name:      "nic0",
			Network:   fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/default", project),
			NetworkIP: "10.128.0.2",
		}}
	}

	// GCE requires every network interface to be on a distinct VPC network —
	// instances.insert (and thus terraform apply) rejects the request outright
	// rather than attaching only one of the duplicates.
	if dup := duplicateNetworkInterfaceVPC(netIfaces); dup != "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", fmt.Sprintf("Invalid value for field 'resource.networkInterfaces': network %q is used by more than one interface; each interface must be on a different VPC network", dup))
		return
	}

	// Default boot disk
	disks := body.Disks
	if len(disks) == 0 {
		disks = []AttachedDisk{{
			Kind:       "compute#attachedDisk",
			Type:       "PERSISTENT",
			Mode:       "READ_WRITE",
			DeviceName: name,
			Boot:       true,
			AutoDelete: true,
		}}
	}

	inst := &Instance{
		Kind:              "compute#instance",
		ID:                randomNumericID(),
		Name:              name,
		Zone:              zoneFull,
		MachineType:       machineType,
		Status:            "PROVISIONING",
		SelfLink:          selfLink,
		Description:       body.Description,
		Labels:            body.Labels,
		Metadata:          body.Metadata,
		NetworkInterfaces: netIfaces,
		Disks:             disks,
		CreationTimestamp: time.Now().UTC().Format(time.RFC3339),
		project:           project,
		zone:              zone,
		Fingerprint:       "minisky-mock-fingerprint",
		LabelFingerprint:  "minisky-label-fingerprint",
		Scheduling: &Scheduling{
			OnHostMaintenance: "MIGRATE",
			AutomaticRestart:  true,
			Preemptible:       false,
		},
		CanIpForward: false,
	}

	if inst.Metadata == nil {
		inst.Metadata = &InstanceMetadata{
			Kind:  "compute#metadata",
			Items: []MetadataItem{},
		}
	} else if inst.Metadata.Items == nil {
		inst.Metadata.Items = []MetadataItem{}
	}

	key := instanceKey(project, zone, name)
	api.mu.Lock()
	api.instances[key] = inst
	api.mu.Unlock()

	// Register LRO
	op := api.opMgr.Register("compute#operation", "insert", targetLink, zone, "")
	op.Kind = "compute#operation"

	// Resolve the docker image backing the boot disk. A caller can name an
	// existing disk (source) or, far more commonly, ask for an OS image family
	// through initializeParams.sourceImage — which is what Terraform's
	// boot_disk.initialize_params.image sends.
	osImage := ""
	for _, disk := range disks {
		if !disk.Boot {
			continue
		}
		if disk.Source != "" {
			osImage = disk.Source
			break
		}
		if disk.InitializeParams != nil {
			if resolved := resolveOsImage(disk.InitializeParams.SourceImage); resolved != "" {
				osImage = resolved
				break
			}
		}
	}
	if osImage == "" {
		osImage = defaultOsImage()
	}

	// Legacy CentOS check for backward compatibility or direct API calls
	if osImage == defaultOsImage() {
		lowerSource := strings.ToLower(machineType + " ")
		for _, disk := range disks {
			lowerSource += strings.ToLower(disk.Source)
		}
		if strings.Contains(lowerSource, "centos") {
			osImage = "centos:latest"
		}
	}

	containerName := fmt.Sprintf("minisky-vm-%s", name)
	isGKE := body.Labels != nil && body.Labels["managed-by"] == "gke"
	if isGKE {
		containerName = name // Kind sets container name exactly as kind cluster node name
	}

	// Drive state machine asynchronously: PROVISIONING → PROVISIONING_DOCKER → RUNNING
	opName := op.Name
	api.opMgr.RunAsync(opName, func() error {
		// 1. Initial Staging phase (simulates resource allocation)
		api.mu.Lock()
		if i, ok := api.instances[key]; ok {
			i.Status = "STAGING"
		}
		api.mu.Unlock()
		time.Sleep(2 * time.Second)

		if isGKE {
			// Kind already manages the docker daemon side. Mark running directly.
			api.mu.Lock()
			if i, ok := api.instances[key]; ok {
				i.Status = "RUNNING"
				i.setContainerMapping(containerName)
			}
			api.mu.Unlock()
			return nil
		}

		// 2. Provisioning phase (simulates Docker container startup)
		api.mu.Lock()
		if i, ok := api.instances[key]; ok {
			i.Status = "PROVISIONING"
		}
		api.mu.Unlock()

		// Build the full list of VPC names, one per network interface, so
		// ProvisionComputeVM below attaches every network, not just nic0.
		vpcNames := []string{"default"}
		api.mu.RLock()
		if i, ok := api.instances[key]; ok && len(i.NetworkInterfaces) > 0 {
			vpcNames = make([]string, len(i.NetworkInterfaces))
			for j, iface := range i.NetworkInterfaces {
				vpcNames[j] = networkInterfaceVPCName(iface)
			}
		}
		api.mu.RUnlock()

		// Merge ingress-allow ports across every attached VPC, mirroring how a real
		// GCE VM inherits firewall rules from all networks it's attached to, not just
		// the primary one.
		allowedPorts := orchestrator.MergeUniquePorts(vpcNames, api.getAllowedPortsForVPC)

		// Tell the Orchestrator to physically spin up the Docker container!
		err := api.svcMgr.ProvisionComputeVM(containerName, osImage, vpcNames, allowedPorts, []string{}, []string{"tail", "-f", "/dev/null"})

		api.mu.Lock()
		if i, ok := api.instances[key]; ok {
			if err != nil {
				i.Status = "TERMINATED"
				i.Description = fmt.Sprintf("Failed to provision docker data plane: %v", err)
				api.mu.Unlock()
				return err
			}

			// 3. Post-provisioning delay to ensure the UI catches the transition
			time.Sleep(1500 * time.Millisecond)

			i.Status = "RUNNING"
			i.setContainerMapping(containerName)
		}
		api.mu.Unlock()
		return nil
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

func (api *API) getInstance(w http.ResponseWriter, r *http.Request, project, zone, name string) {
	key := instanceKey(project, zone, name)
	api.mu.RLock()
	inst, ok := api.instances[key]

	if !ok {
		api.mu.RUnlock()
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Instance '%s' not found in zone '%s'", name, zone))
		return
	}

	// Deep copy under lock to avoid racing with background updates
	instCopy := inst.DeepCopy()
	api.mu.RUnlock()

	// Inject dynamic host ports from orchestrator
	cName := "minisky-vm-" + instCopy.Name
	if instCopy.Labels != nil && instCopy.Labels["managed-by"] == "gke" {
		cName = instCopy.Name
	}
	instCopy.HostPorts = api.svcMgr.GetVMPortMappings(cName)
	if len(instCopy.NetworkInterfaces) > 0 {
		api.patchNetworkIPs(instCopy, cName)
		defaultSubnetwork := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/regions/%s/subnetworks/default", project, strings.Join(strings.Split(zone, "-")[:2], "-"))
		for j := range instCopy.NetworkInterfaces {
			if instCopy.NetworkInterfaces[j].Subnetwork == "" {
				instCopy.NetworkInterfaces[j].Subnetwork = defaultSubnetwork
			}
		}
	}

	if instCopy.Fingerprint == "" {
		instCopy.Fingerprint = "minisky-mock-fingerprint"
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(instCopy)
}

func (api *API) listInstances(w http.ResponseWriter, r *http.Request, project, zone string) {
	prefix := instanceKey(project, zone, "")

	// Copy the matching instances under lock, then do the (potentially slow,
	// blocking) Docker lookups below without holding api.mu — otherwise every
	// insertInstance/deleteInstance writer stalls for the whole loop.
	api.mu.RLock()
	var copies []*Instance
	for k, v := range api.instances {
		if strings.HasPrefix(k, prefix) {
			copies = append(copies, v.DeepCopy())
		}
	}
	api.mu.RUnlock()

	// Each instance's Docker lookups are independent, so fan them out concurrently
	// instead of paying N sequential Docker round-trips on a UI-polled endpoint.
	items := make([]*Instance, len(copies))
	var wg sync.WaitGroup
	for idx, copyOfInst := range copies {
		wg.Add(1)
		go func(idx int, copyOfInst *Instance) {
			defer wg.Done()
			cName := "minisky-vm-" + copyOfInst.Name
			if copyOfInst.Labels != nil && copyOfInst.Labels["managed-by"] == "gke" {
				cName = copyOfInst.Name
			}
			copyOfInst.HostPorts = api.svcMgr.GetVMPortMappings(cName)
			if len(copyOfInst.NetworkInterfaces) > 0 {
				api.patchNetworkIPs(copyOfInst, cName)
			}
			items[idx] = copyOfInst
		}(idx, copyOfInst)
	}
	wg.Wait()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":  "compute#instanceList",
		"id":    fmt.Sprintf("projects/%s/zones/%s/instances", project, zone),
		"items": items,
	})
}

func (api *API) deleteInstance(w http.ResponseWriter, r *http.Request, project, zone, name string) {
	key := instanceKey(project, zone, name)
	api.mu.Lock()
	inst, ok := api.instances[key]
	if !ok {
		api.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Instance '%s' not found", name))
		return
	}

	if r.Header.Get("X-Minisky-GKE-Bypass") != "true" && inst.Labels != nil && inst.Labels["managed-by"] == "gke" {
		api.mu.Unlock()
		w.WriteHeader(http.StatusForbidden)
		writeError(w, 403, "FORBIDDEN", "This instance is managed by Kubernetes Engine and cannot be manually deleted.")
		return
	}

	// Mark as DELETING so the UI shows the "winding down" process
	inst.Status = "DELETING"
	api.mu.Unlock()

	containerName := fmt.Sprintf("minisky-vm-%s", name)
	op := api.opMgr.Register("compute#operation", "delete",
		selfLinkInstance(project, zone, name), zone, "")

	api.opMgr.RunAsync(op.Name, func() error {
		// Simulate winding down time
		time.Sleep(3 * time.Second)

		api.svcMgr.DeleteComputeVM(containerName)

		// Finally remove from memory
		api.mu.Lock()
		delete(api.instances, key)
		api.mu.Unlock()
		return nil
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

func (api *API) instanceAction(w http.ResponseWriter, r *http.Request, project, zone, name, action string) {
	key := instanceKey(project, zone, name)
	api.mu.Lock()
	inst, ok := api.instances[key]
	if ok {
		switch action {
		case "start":
			inst.Status = "RUNNING"
		case "stop":
			inst.Status = "TERMINATED"
		}
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Instance '%s' not found", name))
		return
	}

	op := api.opMgr.Register("compute#operation", action,
		selfLinkInstance(project, zone, name), zone, "")
	api.opMgr.RunAsync(op.Name, func() error { return nil })
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

// ─────────────────────────────────────────────────────────────────────────────
// Zones
// ─────────────────────────────────────────────────────────────────────────────

// routeZones handles GET requests for zone resources, e.g. from Terraform's
// zone-validation step before creating a Compute instance.
func (api *API) routeZones(w http.ResponseWriter, r *http.Request, path string) {
	project := extractProject(path)
	zone := extractSegmentAfter(path, "zones")

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if zone == "" {
		// List zones — return a minimal list of common zones.
		zones := []map[string]interface{}{}
		for _, z := range []string{"us-central1-a", "us-central1-b", "us-east1-b", "europe-west1-b"} {
			zones = append(zones, map[string]interface{}{
				"kind":     "compute#zone",
				"id":       randomNumericID(),
				"name":     z,
				"status":   "UP",
				"selfLink": fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s", project, z),
				"region":   fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/regions/%s", project, strings.Join(strings.Split(z, "-")[:2], "-")),
			})
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"kind":  "compute#zoneList",
			"items": zones,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":     "compute#zone",
		"id":       randomNumericID(),
		"name":     zone,
		"status":   "UP",
		"selfLink": fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s", project, zone),
		"region":   fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/regions/%s", project, strings.Join(strings.Split(zone, "-")[:2], "-")),
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Images
// ─────────────────────────────────────────────────────────────────────────────

// defaultOsImage is the image a VM boots when the request names none.
func defaultOsImage() string {
	if img := config.GetImageRegistry().Compute.DefaultImage; img != "" {
		return img
	}
	return "ubuntu:26.04"
}

// resolveOsImage maps a GCE source image reference onto the Docker image that
// backs it, using the same os_images registry the dashboard offers:
//
//	projects/ubuntu-os-cloud/global/images/family/ubuntu-2404-lts → ubuntu:24.04
//	ubuntu-2404-lts                                               → ubuntu:24.04
//	ubuntu:24.04                                                  → ubuntu:24.04
//
// It returns "" when the reference names no known family, leaving the caller to
// fall back to the default image.
func resolveOsImage(sourceImage string) string {
	name := strings.TrimSpace(sourceImage)
	if name == "" {
		return ""
	}
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}

	for _, img := range config.GetImageRegistry().Compute.OsImages {
		if strings.EqualFold(img.ID, name) {
			return img.Image
		}
	}

	// A raw Docker reference passes through, which is how the dashboard and
	// direct API callers pin an exact image.
	if strings.Contains(name, ":") {
		return name
	}
	return ""
}

func (api *API) routeImages(w http.ResponseWriter, r *http.Request, path string) {
	project := extractProject(path)
	// Example path: /compute/v1/projects/ubuntu-os-cloud/global/images/family/ubuntu-2604-lts
	family := ""
	if strings.Contains(path, "/family/") {
		parts := strings.Split(path, "/family/")
		family = parts[len(parts)-1]
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Return a synthetic image
	imageName := family
	if imageName == "" {
		imageName = "minisky-mock-image"
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":              "compute#image",
		"id":                randomNumericID(),
		"name":              imageName,
		"status":            "READY",
		"sourceType":        "RAW",
		"selfLink":          fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/images/%s", project, imageName),
		"creationTimestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Operations
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeOperations(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	// Find "operations" segment and take next segment as name
	opName := ""
	for i, p := range parts {
		if p == "operations" && i+1 < len(parts) {
			opName = parts[i+1]
			break
		}
	}

	if opName == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Missing operation name in path")
		return
	}

	op := api.opMgr.Get(opName)
	if op == nil {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Operation not found: "+opName)
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

// ─────────────────────────────────────────────────────────────────────────────
// Networks (VPC)
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeNetworks(w http.ResponseWriter, r *http.Request, path string) {
	project := extractProject(path)
	name := extractAfterGlobal(path, "networks")

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name                                  string `json:"name"`
			Description                           string `json:"description"`
			AutoCreateSubnetworks                 bool   `json:"autoCreateSubnetworks"`
			NetworkFirewallPolicyEnforcementOrder string `json:"networkFirewallPolicyEnforcementOrder"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		enforcementOrder := body.NetworkFirewallPolicyEnforcementOrder
		if enforcementOrder == "" {
			enforcementOrder = "AFTER_CLASSIC_FIREWALL" // GCE's default
		}
		n := &Network{
			Kind:                                  "compute#network",
			ID:                                    randomNumericID(),
			Name:                                  body.Name,
			Description:                           body.Description,
			AutoCreateSubnetworks:                 body.AutoCreateSubnetworks,
			NetworkFirewallPolicyEnforcementOrder: enforcementOrder,
			SelfLink:                              fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/%s", project, body.Name),
			CreationTimestamp:                     time.Now().UTC().Format(time.RFC3339),
		}
		key := project + ":" + body.Name
		api.mu.Lock()
		api.networks[key] = n
		api.mu.Unlock()

		if body.Name != "default" {
			api.svcMgr.CreateVPCNetwork(r.Context(), body.Name)
		}

		op := api.opMgr.Register("compute#operation", "insert",
			n.SelfLink, "", "")
		api.opMgr.RunAsync(op.Name, func() error { return nil })
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(op)

	case http.MethodGet:
		if name != "" {
			key := project + ":" + name
			api.mu.RLock()
			n, ok := api.networks[key]
			api.mu.RUnlock()

			if !ok && name == "default" {
				// Return a virtual default network
				n = &Network{
					Kind:                                  "compute#network",
					ID:                                    "0",
					Name:                                  "default",
					SelfLink:                              fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/default", project),
					CreationTimestamp:                     "2024-01-01T00:00:00Z",
					AutoCreateSubnetworks:                 true,
					NetworkFirewallPolicyEnforcementOrder: "AFTER_CLASSIC_FIREWALL",
				}
				ok = true
			}

			if !ok {
				w.WriteHeader(http.StatusNotFound)
				writeError(w, 404, "NOT_FOUND", "Network "+name+" not found")
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(n)
		} else {
			prefix := project + ":"
			api.mu.RLock()
			items := []*Network{}
			hasDefault := false
			for k, v := range api.networks {
				if strings.HasPrefix(k, prefix) {
					items = append(items, v)
					if v.Name == "default" {
						hasDefault = true
					}
				}
			}
			api.mu.RUnlock()

			if !hasDefault {
				items = append(items, &Network{
					Kind:              "compute#network",
					ID:                "0",
					Name:              "default",
					SelfLink:          fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/default", project),
					CreationTimestamp: "2024-01-01T00:00:00Z",
				})
			}

			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"kind":  "compute#networkList",
				"items": items,
			})
		}

	case http.MethodDelete:
		key := project + ":" + name
		api.mu.Lock()
		_, ok := api.networks[key]
		if ok {
			delete(api.networks, key)
		}
		api.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			writeError(w, 404, "NOT_FOUND", "Network "+name+" not found")
			return
		}
		if name != "default" {
			api.svcMgr.DeleteVPCNetwork(r.Context(), name)
		}

		op := api.opMgr.Register("compute#operation", "delete", "", "", "")
		api.opMgr.RunAsync(op.Name, func() error { return nil })
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(op)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Security Policies (Cloud Armor)
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeSecurityPolicies(w http.ResponseWriter, r *http.Request, path string) {
	project := extractProject(path)
	name := extractAfterGlobal(path, "securityPolicies")

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name        string               `json:"name"`
			Description string               `json:"description"`
			Rules       []SecurityPolicyRule `json:"rules"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		// Always add a default allow-all rule at priority 2147483647 (GCP convention)
		rules := body.Rules
		hasDefault := false
		for _, rule := range rules {
			if rule.Priority == 2147483647 {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			rules = append(rules, SecurityPolicyRule{
				Priority:    2147483647,
				Action:      "allow",
				Description: "default allow-all rule",
			})
		}

		sp := &SecurityPolicy{
			Kind:              "compute#securityPolicy",
			ID:                randomNumericID(),
			Name:              body.Name,
			Description:       body.Description,
			Rules:             rules,
			SelfLink:          fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/securityPolicies/%s", project, body.Name),
			CreationTimestamp: time.Now().UTC().Format(time.RFC3339),
		}
		key := project + ":" + body.Name
		api.mu.Lock()
		api.securityPolicies[key] = sp
		api.mu.Unlock()

		op := api.opMgr.Register("compute#operation", "insert", sp.SelfLink, "", "")
		api.opMgr.RunAsync(op.Name, func() error { return nil })
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(op)

	case http.MethodGet:
		if name != "" {
			key := project + ":" + name
			api.mu.RLock()
			sp, ok := api.securityPolicies[key]
			api.mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				writeError(w, 404, "NOT_FOUND", "SecurityPolicy "+name+" not found")
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(sp)
		} else {
			prefix := project + ":"
			api.mu.RLock()
			items := []*SecurityPolicy{}
			for k, v := range api.securityPolicies {
				if strings.HasPrefix(k, prefix) {
					items = append(items, v)
				}
			}
			api.mu.RUnlock()
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"kind":  "compute#securityPolicyList",
				"items": items,
			})
		}

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Load Balancer stubs (stateless for now, return accepted LRO)
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeLoadBalancer(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	project := extractProject(path)
	op := api.opMgr.Register("compute#operation", "insert",
		"https://www.googleapis.com/compute/v1/projects/"+project+path, "", "")
	api.opMgr.RunAsync(op.Name, func() error { return nil })
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

// ─────────────────────────────────────────────────────────────────────────────
// Path parsing helpers
// ─────────────────────────────────────────────────────────────────────────────

// extractProject returns the project from a path like /compute/v1/projects/{project}/...
func extractProject(path string) string {
	return extractSegmentAfter(path, "projects")
}

// extractProjectZone returns (project, zone) from a zones-scoped path.
func extractProjectZone(path string) (string, string) {
	return extractSegmentAfter(path, "projects"), extractSegmentAfter(path, "zones")
}

func extractSegmentAfter(path, keyword string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == keyword && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractSegmentBefore(path, suffix string) string {
	path = strings.TrimSuffix(strings.TrimRight(path, "/"), suffix)
	parts := strings.Split(strings.TrimRight(path, "/"), "/")
	return parts[len(parts)-1]
}

// extractAfterInstances returns the instance name component (if present).
func extractAfterInstances(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "instances" && i+1 < len(parts) {
			name := parts[i+1]
			// Exclude action suffixes
			if name != "" && name != "start" && name != "stop" && name != "reset" {
				return name
			}
		}
	}
	return ""
}

// extractAfterGlobal returns the resource name after /global/{collection}/{name}.
func extractAfterGlobal(path, collection string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == collection && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func instanceKey(project, zone, name string) string {
	return project + ":" + zone + ":" + name
}

func selfLinkInstance(project, zone, name string) string {
	return fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/instances/%s",
		project, zone, name)
}

func writeError(w http.ResponseWriter, code int, status, message string) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"status":  status,
			"message": message,
		},
	})
}

func randomNumericID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ─────────────────────────────────────────────────────────────────────────────
// Firewall Rules
// ─────────────────────────────────────────────────────────────────────────────

// firewallEffect derives what a rule actually permits from the resource as GCE
// models it. A firewall has no `action` field: allow versus deny is expressed by
// which of `allowed` / `denied` is populated, and the ports live inside those
// entries rather than at the top level.
//
// Registering a rule without reading them left every entry with an empty port
// list and an empty action, so allowedPortsForVPC — which matches on
// action == "allow" — never returned a port and no VM ever got a published host
// port, making services inside an emulated VM unreachable from the host.
func firewallEffect(rule *FirewallRule) (action, protocol string, ports []string) {
	// An explicitly declared action wins — the dashboard sets one, and a rule
	// marked "deny" must stay a deny however its ports are expressed. The real
	// API has no such field, so a rule that arrives without one (everything
	// Terraform sends) is classified by which list is populated.
	entries := rule.Allowed
	switch {
	case strings.EqualFold(rule.Action, "deny"):
		action = "deny"
		if len(rule.Denied) > 0 {
			entries = rule.Denied
		}
	case len(rule.Allowed) > 0:
		action = "allow"
	case len(rule.Denied) > 0:
		action = "deny"
		entries = rule.Denied
	default:
		// Neither list populated: fall back to whatever the caller declared.
		return rule.Action, "all", nil
	}
	if len(entries) == 0 {
		return action, "all", nil
	}

	protocol = "all"
	for _, entry := range entries {
		if entry.IPProtocol != "" {
			protocol = entry.IPProtocol
		}
		for _, port := range entry.Ports {
			ports = append(ports, expandPortRange(port)...)
		}
	}
	return action, protocol, ports
}

// maxExpandedPorts caps how many ports a single range may contribute. Docker
// binds each port individually, so a rule like "0-65535" would otherwise try to
// publish every port on the host.
const maxExpandedPorts = 64

// expandPortRange turns "8080" into ["8080"] and "8080-8082" into
// ["8080","8081","8082"], skipping ranges too large to bind.
func expandPortRange(port string) []string {
	low, high, found := strings.Cut(port, "-")
	if !found {
		return []string{port}
	}

	start, err := strconv.Atoi(strings.TrimSpace(low))
	if err != nil {
		return nil
	}
	end, err := strconv.Atoi(strings.TrimSpace(high))
	if err != nil || end < start {
		return nil
	}
	if end-start+1 > maxExpandedPorts {
		log.Printf("[Shim: Compute] port range %s spans more than %d ports — not binding it to the host",
			port, maxExpandedPorts)
		return nil
	}

	expanded := make([]string, 0, end-start+1)
	for p := start; p <= end; p++ {
		expanded = append(expanded, strconv.Itoa(p))
	}
	return expanded
}

func (api *API) routeFirewalls(w http.ResponseWriter, r *http.Request, path string) {
	project := extractProject(path)
	name := extractAfterGlobal(path, "firewalls")

	switch r.Method {
	case http.MethodPost:
		api.createFirewall(w, r, project)
	case http.MethodGet:
		if name != "" {
			api.getFirewall(w, project, name)
		} else {
			api.listFirewalls(w, project)
		}
	case http.MethodPatch, http.MethodPut:
		api.patchFirewall(w, r, project, name)
	case http.MethodDelete:
		api.deleteFirewall(w, project, name)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createFirewall(w http.ResponseWriter, r *http.Request, project string) {
	var body FirewallRule
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}
	if body.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "'name' is required")
		return
	}
	if body.Priority == 0 {
		body.Priority = 1000
	}
	if body.Direction == "" {
		body.Direction = "INGRESS"
	}
	if body.Network == "" {
		body.Network = fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/default", project)
	}
	body.Kind = "compute#firewall"
	body.ID = randomNumericID()
	body.SelfLink = fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/firewalls/%s", project, body.Name)
	body.CreationTimestamp = time.Now().UTC().Format(time.RFC3339)

	key := project + ":" + body.Name
	api.mu.Lock()
	api.firewalls[key] = &body
	api.mu.Unlock()

	// Keyed by the short VPC name, not the full network URL — allowedPortsForVPC and
	// ApplyFirewallPortsToVPC always look this up by short name (derived via
	// extractNameFromURL), so registering under the full URL would make the lookup
	// permanently miss.
	action, protocol, ports := firewallEffect(&body)
	direction := body.Direction
	if direction == "" {
		direction = "INGRESS" // GCE's default
	}

	api.svcMgr.RegisterFirewallRule(extractNameFromURL(body.Network), orchestrator.FirewallEntry{
		Name:      body.Name,
		VpcName:   extractNameFromURL(body.Network),
		Direction: direction,
		Action:    action,
		Protocol:  protocol,
		Ports:     ports,
		Ranges:    append(body.SourceRanges, body.DestinationRanges...),
	})

	op := api.opMgr.Register("compute#operation", "insert", body.SelfLink, "", "")
	api.opMgr.RunAsync(op.Name, func() error {
		api.reapplyFirewallToVPC(body.Network)
		return nil
	})
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

func (api *API) getFirewall(w http.ResponseWriter, project, name string) {
	key := project + ":" + name
	api.mu.RLock()
	fw, ok := api.firewalls[key]
	api.mu.RUnlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Firewall "+name+" not found")
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(fw)
}

func (api *API) listFirewalls(w http.ResponseWriter, project string) {
	prefix := project + ":"
	api.mu.RLock()
	items := []*FirewallRule{}
	for k, v := range api.firewalls {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":  "compute#firewallList",
		"items": items,
	})
}

func (api *API) patchFirewall(w http.ResponseWriter, r *http.Request, project, name string) {
	key := project + ":" + name
	api.mu.Lock()
	fw, ok := api.firewalls[key]
	if !ok {
		api.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Firewall "+name+" not found")
		return
	}
	var patch FirewallRule
	json.NewDecoder(r.Body).Decode(&patch)
	if len(patch.Allowed) > 0 {
		fw.Allowed = patch.Allowed
	}
	if len(patch.Denied) > 0 {
		fw.Denied = patch.Denied
	}
	if len(patch.SourceRanges) > 0 {
		fw.SourceRanges = patch.SourceRanges
	}
	if patch.Description != "" {
		fw.Description = patch.Description
	}
	if patch.Priority != 0 {
		fw.Priority = patch.Priority
	}
	result := fw
	api.mu.Unlock()

	op := api.opMgr.Register("compute#operation", "patch", result.SelfLink, "", "")
	api.opMgr.RunAsync(op.Name, func() error {
		api.reapplyFirewallToVPC(result.Network)
		return nil
	})
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

func (api *API) deleteFirewall(w http.ResponseWriter, project, name string) {
	key := project + ":" + name
	api.mu.Lock()
	fw, ok := api.firewalls[key]
	networkURL := ""
	if ok {
		networkURL = fw.Network
		delete(api.firewalls, key)
	}
	api.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Firewall "+name+" not found")
		return
	}
	api.svcMgr.RemoveFirewallRule(extractNameFromURL(networkURL), name)
	op := api.opMgr.Register("compute#operation", "delete", "", "", "")
	api.opMgr.RunAsync(op.Name, func() error {
		api.reapplyFirewallToVPC(networkURL)
		return nil
	})
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

// ─────────────────────────────────────────────────────────────────────────────
// Firewall/VPC Helpers
// ─────────────────────────────────────────────────────────────────────────────

func extractNameFromURL(urlStr string) string {
	parts := strings.Split(urlStr, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// networkInterfaceVPCName returns the short VPC name a network interface
// references, normalizing an empty Network field to "default" the same way
// ProvisionComputeVM's caller does.
func networkInterfaceVPCName(iface NetworkInterface) string {
	name := extractNameFromURL(iface.Network)
	if name == "" {
		name = "default"
	}
	return name
}

// duplicateNetworkInterfaceVPC returns the VPC name shared by two or more
// network interfaces, or "" if every interface is on a distinct network. Real
// GCE requires each interface to be on a different VPC network — attaching
// two NICs to the same network is rejected by instances.insert rather than
// silently honoring only one of them.
func duplicateNetworkInterfaceVPC(ifaces []NetworkInterface) string {
	seen := make(map[string]bool, len(ifaces))
	for _, iface := range ifaces {
		vpc := networkInterfaceVPCName(iface)
		if seen[vpc] {
			return vpc
		}
		seen[vpc] = true
	}
	return ""
}

// dockerNetworkNameForVPC delegates to orchestrator.DockerNetworkNameForVPC,
// the single source of truth for the VPC-name <-> Docker-network-name
// mapping shared with ProvisionComputeVM/ConnectContainerToNetwork/
// attachedVPCNames.
func dockerNetworkNameForVPC(vpcName string) string {
	return orchestrator.DockerNetworkNameForVPC(vpcName)
}

// patchNetworkIPs fills in each network interface's live container IP on its
// own Docker network, so a multi-NIC VM reports the correct address per
// interface instead of collapsing them all onto one. It fetches the
// container's network info once and reuses it across every interface, rather
// than issuing one Docker inspect call per NIC.
func (api *API) patchNetworkIPs(inst *Instance, cName string) {
	ips, err := api.svcMgr.GetContainerNetworks(cName)
	for j := range inst.NetworkInterfaces {
		vpcName := extractNameFromURL(inst.NetworkInterfaces[j].Network)
		dockerNetworkName := dockerNetworkNameForVPC(vpcName)
		ip := ""
		if err == nil {
			ip = ips[dockerNetworkName]
		}
		if ip == "" {
			ip = "10.128.0.2" // Fallback to avoid empty IP which can crash some providers
		}
		inst.NetworkInterfaces[j].NetworkIP = ip
	}
}

func (api *API) getAllowedPortsForVPC(vpcName string) []string {
	api.mu.RLock()
	defer api.mu.RUnlock()
	ports := []string{}
	for _, rule := range api.firewalls {
		if rule.Disabled {
			continue
		}
		nw := extractNameFromURL(rule.Network)
		if nw != vpcName && !(nw == "" && vpcName == "default") {
			continue
		}
		direction := rule.Direction
		if direction == "" {
			direction = "INGRESS" // GCE's default
		}
		if direction != "INGRESS" {
			continue
		}
		// Allow/deny and the port list both come from the allowed/denied
		// entries. Matching on rule.Action here meant matching on a field GCE
		// does not populate, so no rule ever contributed a port.
		action, _, rulePorts := firewallEffect(rule)
		if action != "allow" {
			continue
		}
		ports = append(ports, rulePorts...)
	}
	return ports
}

func (api *API) reapplyFirewallToVPC(networkURL string) {
	vpcName := extractNameFromURL(networkURL)
	var containerNames []string
	var osImages []string

	api.mu.RLock()
	for _, inst := range api.instances {
		matchesVPC := false
		for _, iface := range inst.NetworkInterfaces {
			nw := extractNameFromURL(iface.Network)
			if nw == vpcName || (nw == "" && vpcName == "default") {
				matchesVPC = true
				break
			}
		}
		if matchesVPC {
			cName := fmt.Sprintf("minisky-vm-%s", inst.Name)
			containerNames = append(containerNames, cName)
			img := "ubuntu:latest"
			for _, d := range inst.Disks {
				if strings.Contains(strings.ToLower(d.Source), "centos") {
					img = "centos:latest"
				}
			}
			osImages = append(osImages, img)
		}
	}
	api.mu.RUnlock()

	if len(containerNames) > 0 {
		api.svcMgr.ApplyFirewallPortsToVPC(vpcName, containerNames, osImages)
	}
}
