package compute

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

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
	Kind              string            `json:"kind"`
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	Zone              string            `json:"zone"`
	MachineType       string            `json:"machineType"`
	Status            string            `json:"status"`
	SelfLink          string            `json:"selfLink"`
	Description       string            `json:"description"`
	Labels            map[string]string `json:"labels,omitempty"`
	Metadata          *InstanceMetadata `json:"metadata,omitempty"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces"`
	Disks             []AttachedDisk    `json:"disks"`
	CreationTimestamp string            `json:"creationTimestamp"`
	HostPorts         []orchestrator.PortMapping `json:"hostPorts,omitempty"`
	// internal tracking only
	project string
	zone    string
}

type InstanceMetadata struct {
	Kind  string            `json:"kind"`
	Items []MetadataItem    `json:"items,omitempty"`
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
	Kind       string `json:"kind"`
	Type       string `json:"type"` // PERSISTENT, SCRATCH
	Mode       string `json:"mode"` // READ_WRITE, READ_ONLY
	Source     string `json:"source,omitempty"`
	DeviceName string `json:"deviceName"`
	Boot       bool   `json:"boot"`
	AutoDelete bool   `json:"autoDelete"`
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
	Priority    int             `json:"priority"`
	Action      string          `json:"action"`
	Description string          `json:"description,omitempty"`
	Match       *RuleMatch      `json:"match,omitempty"`
}

type RuleMatch struct {
	VersionedExpr string `json:"versionedExpr,omitempty"` // SRC_IPS_V1
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
	Direction         string          `json:"direction"`   // INGRESS, EGRESS
	Action            string          `json:"action"`      // allow, deny
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
	instances        map[string]*Instance        // key: project+":"+zone+":"+name
	networks         map[string]*Network         // key: project+":"+name
	securityPolicies map[string]*SecurityPolicy  // key: project+":"+name
	firewalls        map[string]*FirewallRule     // key: project+":"+name
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

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Compute Engine] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	switch {
	case strings.Contains(path, "/instances") && strings.Contains(path, "/zones/"):
		api.routeInstances(w, r, path)
	case strings.Contains(path, "/operations/"):
		api.routeOperations(w, r, path)
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
	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Compute resource not found: "+path)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Instances
// ─────────────────────────────────────────────────────────────────────────────

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
		Name        string `json:"name"`
		MachineType string `json:"machineType"`
		Description string `json:"description"`
		Labels      map[string]string `json:"labels"`
		Metadata    *InstanceMetadata `json:"metadata"`
		NetworkInterfaces []NetworkInterface `json:"networkInterfaces"`
		Disks       []AttachedDisk `json:"disks"`
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
	}

	key := instanceKey(project, zone, name)
	api.mu.Lock()
	api.instances[key] = inst
	api.mu.Unlock()

	// Register LRO
	op := api.opMgr.Register("compute#operation", "insert", targetLink, zone, "")
	op.Kind = "compute#operation"

	// Resolve the docker image mapping from the boot disk source
	osImage := "ubuntu:26.04" // Fallback to 2026 default
	for _, disk := range disks {
		if disk.Boot && disk.Source != "" {
			osImage = disk.Source
			break
		}
	}
	// Legacy CentOS check for backward compatibility or direct API calls
	if osImage == "ubuntu:26.04" {
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
		time.Sleep(1 * time.Second)
		api.mu.Lock()
		if i, ok := api.instances[key]; ok {
			i.Status = "STAGING"
		}
		api.mu.Unlock()

		if isGKE {
			// Kind already manages the docker daemon side. Mark running directly.
			api.mu.Lock()
			if i, ok := api.instances[key]; ok {
				i.Status = "RUNNING"
				i.Description = fmt.Sprintf("Docker Container ID mapping: %s", containerName)
			}
			api.mu.Unlock()
			return nil
		}

		vpcName := "default"
		api.mu.RLock()
		if i, ok := api.instances[key]; ok {
			if len(i.NetworkInterfaces) > 0 {
				parts := strings.Split(i.NetworkInterfaces[0].Network, "/")
				if len(parts) > 0 && parts[len(parts)-1] != "" {
					vpcName = parts[len(parts)-1]
				}
			}
		}
		api.mu.RUnlock()

		allowedPorts := api.getAllowedPortsForVPC(vpcName)

		// Tell the Orchestrator to physically spin up the Docker container!
		err := api.svcMgr.ProvisionComputeVM(containerName, osImage, vpcName, allowedPorts, []string{}, []string{"tail", "-f", "/dev/null"})
		api.mu.Lock()
		if i, ok := api.instances[key]; ok {
			if err != nil {
				i.Status = "TERMINATED"
				i.Description = fmt.Sprintf("Failed to provision docker data plane: %v", err)
			} else {
				i.Status = "RUNNING"
				i.Description = fmt.Sprintf("Docker Container ID mapping: %s", containerName)
			}
		}
		api.mu.Unlock()
		
		return err
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(op)
}

func (api *API) getInstance(w http.ResponseWriter, r *http.Request, project, zone, name string) {
	key := instanceKey(project, zone, name)
	api.mu.RLock()
	inst, ok := api.instances[key]
	api.mu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Instance '%s' not found in zone '%s'", name, zone))
		return
	}
	
	// Inject dynamic host ports from orchestrator
	cName := "minisky-vm-" + inst.Name
	if inst.Labels != nil && inst.Labels["managed-by"] == "gke" {
		cName = inst.Name
	}
	inst.HostPorts = api.svcMgr.GetVMPortMappings(cName)
	if len(inst.NetworkInterfaces) > 0 {
		inst.NetworkInterfaces[0].NetworkIP = api.svcMgr.GetContainerIP(cName)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(inst)
}

func (api *API) listInstances(w http.ResponseWriter, r *http.Request, project, zone string) {
	prefix := instanceKey(project, zone, "")
	api.mu.RLock()
	defer api.mu.RUnlock()

	items := []*Instance{}
	for k, v := range api.instances {
		if strings.HasPrefix(k, prefix) {
			// Create a copy to inject dynamic port mappings safely
			copyOfInst := *v
			cName := "minisky-vm-" + copyOfInst.Name
			if copyOfInst.Labels != nil && copyOfInst.Labels["managed-by"] == "gke" {
				cName = copyOfInst.Name
			}
			copyOfInst.HostPorts = api.svcMgr.GetVMPortMappings(cName)
			if len(copyOfInst.NetworkInterfaces) > 0 {
				copyOfInst.NetworkInterfaces[0].NetworkIP = api.svcMgr.GetContainerIP(cName)
			}
			items = append(items, &copyOfInst)
		}
	}

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
	if ok {
		if r.Header.Get("X-Minisky-GKE-Bypass") != "true" && inst.Labels != nil && inst.Labels["managed-by"] == "gke" {
			api.mu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			writeError(w, 403, "FORBIDDEN", "This instance is managed by Kubernetes Engine and cannot be manually deleted.")
			return
		}
		delete(api.instances, key)
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Instance '%s' not found", name))
		return
	}

	containerName := fmt.Sprintf("minisky-vm-%s", name)
	op := api.opMgr.Register("compute#operation", "delete",
		selfLinkInstance(project, zone, name), zone, "")
	api.opMgr.RunAsync(op.Name, func() error {
		api.svcMgr.DeleteComputeVM(containerName)
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
			Name                  string `json:"name"`
			Description           string `json:"description"`
			AutoCreateSubnetworks bool   `json:"autoCreateSubnetworks"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		n := &Network{
			Kind:                  "compute#network",
			ID:                    randomNumericID(),
			Name:                  body.Name,
			Description:           body.Description,
			AutoCreateSubnetworks: body.AutoCreateSubnetworks,
			SelfLink:              fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/global/networks/%s", project, body.Name),
			CreationTimestamp:     time.Now().UTC().Format(time.RFC3339),
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
			for k, v := range api.networks {
				if strings.HasPrefix(k, prefix) {
					items = append(items, v)
				}
			}
			api.mu.RUnlock()
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

	api.svcMgr.RegisterFirewallRule(body.Network, orchestrator.FirewallEntry{
		Name:      body.Name,
		VpcName:   extractNameFromURL(body.Network),
		Direction: body.Direction,
		Action:    body.Action,
		Protocol:  "all", // default, will refine below
		Ports:     []string{},
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
	api.svcMgr.RemoveFirewallRule(networkURL, name)
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

func (api *API) getAllowedPortsForVPC(vpcName string) []string {
	api.mu.RLock()
	defer api.mu.RUnlock()
	ports := []string{}
	for _, rule := range api.firewalls {
		nw := extractNameFromURL(rule.Network)
		if (nw == vpcName || (nw == "" && vpcName == "default")) && rule.Direction == "INGRESS" && rule.Action == "allow" {
			for _, allowed := range rule.Allowed {
				for _, p := range allowed.Ports {
					ports = append(ports, p)
				}
			}
		}
	}
	return ports
}

func (api *API) reapplyFirewallToVPC(networkURL string) {
	vpcName := extractNameFromURL(networkURL)
	var containerNames []string
	var osImages []string
	
	api.mu.RLock()
	for _, inst := range api.instances {
		if len(inst.NetworkInterfaces) > 0 {
			nw := extractNameFromURL(inst.NetworkInterfaces[0].Network)
			if nw == vpcName || (nw == "" && vpcName == "default") {
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
	}
	api.mu.RUnlock()
	
	if len(containerNames) > 0 {
		api.svcMgr.ApplyFirewallPortsToVPC(vpcName, containerNames, osImages)
	}
}
