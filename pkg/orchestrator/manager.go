package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const networkName = "minisky-net"

// ServiceManager handles native REST-driven lifecycle events over the Docker Unix Socket.
type ServiceManager struct {
	mu           sync.RWMutex
	dockerClient *http.Client
	sockPath     string
	portRegistry map[string][]PortMapping  // containerName → host ports
	fwRules      map[string][]FirewallEntry // vpcName → rules
}

// ContainerConfig describes one backend emulator container.
type ContainerConfig struct {
	Name          string
	Image         string
	ContainerPort string // e.g. "4443/tcp"
	Cmd           []string
}

// PortMapping tracks a host:container port pair for a VM.
type PortMapping struct {
	ContainerPort string
	HostPort      string
	Protocol      string
}

// FirewallEntry is a simplified snapshot for Level-3 enforcement.
type FirewallEntry struct {
	Name      string
	VpcName   string
	Direction string   // INGRESS, EGRESS
	Action    string   // allow, deny
	Protocol  string   // tcp, udp, icmp, all
	Ports     []string // empty = all ports
	Ranges    []string // CIDR source/dest ranges
}

// registry maps GCP API domains to their Docker emulator payloads.
// NOTE: No HostPort is registered — MiniSky connects via the internal bridge IP.
var registry = map[string]ContainerConfig{
	"storage.googleapis.com": {
		Name:          "minisky-gcs",
		Image:         "fsouza/fake-gcs-server:latest",
		ContainerPort: "4443/tcp",
		Cmd:           []string{"-scheme", "http"},
	},
	"pubsub.googleapis.com": {
		Name:          "minisky-pubsub",
		Image:         "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators",
		ContainerPort: "8085/tcp",
		Cmd:           []string{"gcloud", "beta", "emulators", "pubsub", "start", "--host-port=0.0.0.0:8085"},
	},
	"firestore.googleapis.com": {
		Name:          "minisky-firestore",
		Image:         "gcr.io/google.com/cloudsdktool/cloud-sdk:emulators",
		ContainerPort: "8082/tcp",
		Cmd:           []string{"gcloud", "beta", "emulators", "firestore", "start", "--host-port=0.0.0.0:8082"},
	},
}

func NewServiceManager() (*ServiceManager, error) {
	sockPath := resolveDockerSocket()
	log.Printf("[ServiceManager] Docker socket resolved: %s", sockPath)
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sockPath)
		},
	}
	return &ServiceManager{
		dockerClient: &http.Client{Transport: transport},
		sockPath:     sockPath,
		portRegistry: make(map[string][]PortMapping),
		fwRules:      make(map[string][]FirewallEntry),
	}, nil
}

// EnsureNetwork creates the isolated minisky-net bridge network if it doesn't exist.
func (sm *ServiceManager) EnsureNetwork(ctx context.Context) error {
	// Check if it already exists
	resp, err := sm.dockerClient.Get("http://localhost/networks/" + networkName)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		log.Printf("[Orchestrator] Network '%s' already exists.", networkName)
		return nil
	}

	// Create isolated bridge network
	payload := map[string]interface{}{
		"Name":   networkName,
		"Driver": "bridge",
		"Labels": map[string]string{"managed-by": "minisky"},
	}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost/networks/create", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	createResp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer createResp.Body.Close()
	if createResp.StatusCode >= 400 {
		b, _ := io.ReadAll(createResp.Body)
		return fmt.Errorf("network create failed %d: %s", createResp.StatusCode, b)
	}
	log.Printf("[Orchestrator] Created isolated network '%s'.", networkName)
	return nil
}

// EnsureServiceRunning boots the container if needed and returns its internal bridge URL.
func (sm *ServiceManager) EnsureServiceRunning(ctx context.Context, domain string) (string, error) {
	config, exists := registry[domain]
	if !exists {
		// Native Go shims never need Docker containers
		return "", nil
	}

	status, err := sm.checkStatus(config.Name)
	if err != nil {
		return "", fmt.Errorf("status check failed: %v", err)
	}

	if status != "running" {
		log.Printf("[Orchestrator] Cold-starting '%s' for domain '%s'...", config.Name, domain)
		if status == "not_found" {
			if err := sm.pullImage(config.Image); err != nil {
				log.Printf("[Orchestrator] Image pull warning: %v", err)
			}
			if err := sm.createContainer(config); err != nil {
				return "", fmt.Errorf("create container: %v", err)
			}
		}
		if err := sm.startContainer(config.Name); err != nil {
			return "", fmt.Errorf("start container: %v", err)
		}
	}

	// Discover the internal bridge IP — no host port binding needed
	internalURL, err := sm.discoverInternalURL(config)
	if err != nil {
		return "", fmt.Errorf("port discovery: %v", err)
	}

	// Wait until the emulator is truly ready inside the network
	containerPort := strings.Split(config.ContainerPort, "/")[0]
	internalAddr := strings.TrimPrefix(internalURL, "http://")
	if err := sm.waitUntilReady(internalAddr, 20*time.Second); err != nil {
		return "", fmt.Errorf("readiness probe failed: %v", err)
	}

	log.Printf("[Orchestrator] ✅ '%s' is ONLINE at internal %s (port %s)", config.Name, internalURL, containerPort)
	return internalURL, nil
}

// StopServiceContainer stops the underlying docker container for a given service domain.
func (sm *ServiceManager) StopServiceContainer(ctx context.Context, domain string) error {
	config, exists := registry[domain]
	if !exists {
		return fmt.Errorf("domain %s not found in registry", domain)
	}

	status, err := sm.checkStatus(config.Name)
	if err != nil {
		return fmt.Errorf("status check failed: %v", err)
	}

	if status == "running" {
		log.Printf("[Orchestrator] Stopping service container '%s'...", config.Name)
		stopURL := fmt.Sprintf("http://localhost/containers/%s/stop", config.Name)
		req, _ := http.NewRequestWithContext(ctx, "POST", stopURL, nil)
		resp, err := sm.dockerClient.Do(req)
		if err != nil {
			return fmt.Errorf("stop container network error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotModified {
			return fmt.Errorf("stop rejected %d", resp.StatusCode)
		}
		log.Printf("[Orchestrator] Container '%s' stopped successfully.", config.Name)
	} else {
		log.Printf("[Orchestrator] Container '%s' is already not running (status: %s)", config.Name, status)
	}

	return nil
}

// discoverInternalURL reads the host-bound port assigned by Docker and returns
// a localhost URL reachable from the host (compatible with Docker Desktop / VM environments).
func (sm *ServiceManager) discoverInternalURL(config ContainerConfig) (string, error) {
	resp, err := sm.dockerClient.Get(fmt.Sprintf("http://localhost/containers/%s/json", config.Name))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var info struct {
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIp   string
				HostPort string
			}
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}

	bindings, ok := info.NetworkSettings.Ports[config.ContainerPort]
	if !ok || len(bindings) == 0 || bindings[0].HostPort == "" {
		return "", fmt.Errorf("container '%s' has no host port binding for %s", config.Name, config.ContainerPort)
	}

	return fmt.Sprintf("http://127.0.0.1:%s", bindings[0].HostPort), nil
}

// Teardown stops and removes all minisky-* containers and the minisky-net network.
func (sm *ServiceManager) Teardown(ctx context.Context) {
	for _, config := range registry {
		stopURL := fmt.Sprintf("http://localhost/containers/%s/stop", config.Name)
		req, _ := http.NewRequestWithContext(ctx, "POST", stopURL, nil)
		sm.dockerClient.Do(req)

		rmURL := fmt.Sprintf("http://localhost/containers/%s?force=true", config.Name)
		req, _ = http.NewRequestWithContext(ctx, "DELETE", rmURL, nil)
		sm.dockerClient.Do(req)
		log.Printf("[Orchestrator] Removed container '%s'", config.Name)
	}
	rmNetURL := "http://localhost/networks/" + networkName
	req, _ := http.NewRequestWithContext(ctx, "DELETE", rmNetURL, nil)
	sm.dockerClient.Do(req)
	log.Printf("[Orchestrator] Removed network '%s'", networkName)
}

func (sm *ServiceManager) checkStatus(name string) (string, error) {
	resp, err := sm.dockerClient.Get(fmt.Sprintf("http://localhost/containers/%s/json", name))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "not_found", nil
	}
	var state struct {
		State struct{ Status string }
	}
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return "", err
	}
	return state.State.Status, nil
}

// CheckStatusPublic allows external packages to see if a container is running.
func (sm *ServiceManager) CheckStatusPublic(name string) (string, error) {
	return sm.checkStatus(name)
}

func (sm *ServiceManager) pullImage(image string) error {
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://localhost/images/create?fromImage=%s", image), nil)
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

// ProvisionComputeVM actively boots a Data Plane Docker container mimicking a GCE VM.
// To keep it permanently running for SSH, we use `tail -f /dev/null`.
func (sm *ServiceManager) ProvisionComputeVM(containerName string, osImage string, vpcName string, ports []string) error {
	log.Printf("[Orchestrator] Provisioning compute VM: %s (image: %s vpc: %s ports: %d)", containerName, osImage, vpcName, len(ports))
	
	if err := sm.pullImage(osImage); err != nil {
		log.Printf("[Orchestrator] Data Plane pull warning for %s: %v", osImage, err)
	}

	netMode := networkName
	if vpcName != "" && vpcName != "default" {
		netMode = "minisky-vpc-" + vpcName
	}

	exposedPorts := make(map[string]interface{})
	portBindings := make(map[string]interface{})
	for _, port := range ports {
		if !strings.Contains(port, "/") {
			port += "/tcp"
		}
		exposedPorts[port] = struct{}{}
		portBindings[port] = []map[string]interface{}{
			{"HostIp": "127.0.0.1", "HostPort": ""},
		}
	}

	payload := map[string]interface{}{
		"Image":        osImage,
		"Cmd":          []string{"tail", "-f", "/dev/null"},
		"ExposedPorts": exposedPorts,
		"HostConfig": map[string]interface{}{
			"NetworkMode":  netMode,
			"PortBindings": portBindings,
		},
	}
	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("http://localhost/containers/create?name=%s", containerName)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusConflict { // 409
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vm creation rejected %d: %s", resp.StatusCode, b)
	}

	if err := sm.startContainer(containerName); err != nil {
		return err
	}
	
	return sm.updatePortRegistry(containerName)
}

// ProvisionCloudSQLVM starts a fully-interactive PostgreSQL or MySQL docker database data plane.
func (sm *ServiceManager) ProvisionCloudSQLVM(instanceName string, version string, rootPassword string) (string, error) {
	var image string
	var env []string
	var expPort string

	if strings.HasPrefix(version, "POSTGRES") {
		parts := strings.Split(version, "_")
		if len(parts) > 1 {
			image = "postgres:" + parts[1]
		} else {
			image = "postgres:15"
		}
		env = []string{"POSTGRES_PASSWORD=" + rootPassword}
		expPort = "5432/tcp"
	} else if strings.HasPrefix(version, "MYSQL") {
		parts := strings.Split(version, "_")
		if len(parts) > 2 {
			image = "mysql:" + parts[1] + "." + parts[2]
		} else if len(parts) > 1 {
			image = "mysql:" + parts[1]
		} else {
			image = "mysql:8.0"
		}
		env = []string{"MYSQL_ROOT_PASSWORD=" + rootPassword}
		expPort = "3306/tcp"
	} else {
		return "", fmt.Errorf("unsupported database version: %s", version)
	}

	containerName := "minisky-sql-" + instanceName
	log.Printf("[Orchestrator] Provisioning Cloud SQL VM: %s (image: %s)", containerName, image)

	if err := sm.pullImage(image); err != nil {
		log.Printf("[Orchestrator] Pull warning for %s: %v", image, err)
	}

	// Clean up any stale container
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("http://localhost/containers/%s?force=true", containerName), nil)
	sm.dockerClient.Do(req)

	// Volumes - mount a docker volume for persistence
	volName := "minisky-db-" + instanceName
	sm.dockerClient.Post("http://localhost/volumes/create", "application/json", strings.NewReader(`{"Name": "`+volName+`"}`))

	var mountTarget string
	if strings.HasPrefix(version, "MYSQL") {
		mountTarget = "/var/lib/mysql"
	} else {
		mountTarget = "/var/lib/postgresql/data"
	}

	payload := map[string]interface{}{
		"Image": image,
		"Env":   env,
		"ExposedPorts": map[string]interface{}{
			expPort: struct{}{},
		},
		"HostConfig": map[string]interface{}{
			"NetworkMode": networkName,
			"PortBindings": map[string]interface{}{
				expPort: []map[string]string{
					{"HostIp": "127.0.0.1", "HostPort": "0"},
				},
			},
			"Binds": []string{
				fmt.Sprintf("%s:%s", volName, mountTarget),
			},
		},
		"Labels": map[string]string{
			"managed-by": "minisky-sql",
			"instance":   instanceName,
		},
	}

	b, _ := json.Marshal(payload)
	cReq, _ := http.NewRequest("POST", "http://localhost/containers/create?name="+containerName, bytes.NewBuffer(b))
	cReq.Header.Set("Content-Type", "application/json")
	resp, err := sm.dockerClient.Do(cReq)
	if err != nil {
		return "", fmt.Errorf("create SQL container: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create SQL container %d: %s", resp.StatusCode, string(respBody))
	}

	if err := sm.startContainer(containerName); err != nil {
		return "", fmt.Errorf("start SQL container: %v", err)
	}

	config := ContainerConfig{Name: containerName, ContainerPort: expPort}
	internalURL, err := sm.discoverInternalURL(config)
	if err != nil {
		return "", fmt.Errorf("port discovery: %v", err)
	}

	log.Printf("[Orchestrator] ✅ SQL Instance '%s' ONLINE at %s", instanceName, internalURL)
	return internalURL, nil
}

// DeleteCloudSQLVM stops and forcefully removes a Cloud SQL node.
func (sm *ServiceManager) DeleteCloudSQLVM(instanceName string) error {
	containerName := "minisky-sql-" + instanceName
	log.Printf("[Orchestrator] Tearing down Cloud SQL VM: %s", containerName)

	stopURL := fmt.Sprintf("http://localhost/containers/%s/stop?t=2", containerName)
	req, _ := http.NewRequest("POST", stopURL, nil)
	sm.dockerClient.Do(req)

	rmURL := fmt.Sprintf("http://localhost/containers/%s?force=true", containerName)
	req, _ = http.NewRequest("DELETE", rmURL, nil)
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// DeleteComputeVM permanently destroys a physical Data Plane compute instance.
func (sm *ServiceManager) DeleteComputeVM(containerName string) error {
	log.Printf("[Orchestrator] Tearing down Data Plane VM: %s", containerName)
	
	stopURL := fmt.Sprintf("http://localhost/containers/%s/stop?t=2", containerName)
	req, _ := http.NewRequest("POST", stopURL, nil)
	sm.dockerClient.Do(req)

	rmURL := fmt.Sprintf("http://localhost/containers/%s?force=true", containerName)
	req, _ = http.NewRequest("DELETE", rmURL, nil)
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (sm *ServiceManager) createContainer(c ContainerConfig) error {
	// Bind container port to a random localhost port — works with Docker Desktop
	// (which runs in a VM where internal bridge IPs aren't host-reachable).
	payload := map[string]interface{}{
		"Image": c.Image,
		"Cmd":   c.Cmd,
		"ExposedPorts": map[string]interface{}{
			c.ContainerPort: struct{}{},
		},
		"HostConfig": map[string]interface{}{
			"NetworkMode": networkName,
			"PortBindings": map[string]interface{}{
				c.ContainerPort: []map[string]string{
					{"HostIp": "127.0.0.1", "HostPort": "0"},
				},
			},
		},
	}
	data, _ := json.Marshal(payload)
	url := fmt.Sprintf("http://localhost/containers/create?name=%s", c.Name)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create rejected %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (sm *ServiceManager) startContainer(name string) error {
	url := fmt.Sprintf("http://localhost/containers/%s/start", name)
	req, _ := http.NewRequest("POST", url, nil)
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotModified {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("start rejected %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (sm *ServiceManager) waitUntilReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("'%s' not reachable after %s", addr, timeout)
}

// resolveDockerSocket auto-detects the correct Docker socket across all environments.
func resolveDockerSocket() string {
	// 1. Explicit DOCKER_HOST env var (standard Docker convention)
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return strings.TrimPrefix(host, "unix://")
	}
	// 2. Probe known locations in priority order
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".docker", "desktop", "docker.sock"), // Docker Desktop (Linux/Mac)
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "docker.sock"),            // Rootless Docker
		filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "podman", "podman.sock"),  // Podman
		"/var/run/docker.sock", // Classic system Docker
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/var/run/docker.sock"
}

// ─────────────────────────────────────────────────────────────────────────────
// Level 1: VPC Network Management
// ─────────────────────────────────────────────────────────────────────────────

func (sm *ServiceManager) CreateVPCNetwork(ctx context.Context, name string) error {
	netName := "minisky-vpc-" + name
	log.Printf("[Orchestrator] Creating VPC Docker network '%s'", netName)
	payload := map[string]interface{}{
		"Name":   netName,
		"Driver": "bridge",
		"Labels": map[string]string{"managed-by": "minisky-vpc"},
	}
	data, _ := json.Marshal(payload)
	req, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost/networks/create", bytes.NewBuffer(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusConflict { // 409
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vpc network create failed %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (sm *ServiceManager) DeleteVPCNetwork(ctx context.Context, name string) error {
	netName := "minisky-vpc-" + name
	log.Printf("[Orchestrator] Deleting VPC Docker network '%s'", netName)
	req, _ := http.NewRequestWithContext(ctx, "DELETE", "http://localhost/networks/"+netName, nil)
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("vpc network delete failed %d", resp.StatusCode)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Level 2: Port Binding & Firewall Re-application
// ─────────────────────────────────────────────────────────────────────────────

func (sm *ServiceManager) updatePortRegistry(containerName string) error {
	resp, err := sm.dockerClient.Get(fmt.Sprintf("http://localhost/containers/%s/json", containerName))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var info struct {
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIp   string
				HostPort string
			}
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return err
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	mappings := []PortMapping{}
	for cPort, bindings := range info.NetworkSettings.Ports {
		if len(bindings) > 0 && bindings[0].HostPort != "" {
			parts := strings.Split(cPort, "/")
			p := parts[0]
			proto := "tcp"
			if len(parts) > 1 {
				proto = parts[1]
			}
			mappings = append(mappings, PortMapping{
				ContainerPort: p,
				HostPort:      bindings[0].HostPort,
				Protocol:      proto,
			})
		}
	}
	sm.portRegistry[containerName] = mappings
	return nil
}

func (sm *ServiceManager) GetVMPortMappings(containerName string) []PortMapping {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.portRegistry[containerName]
}

func (sm *ServiceManager) ApplyFirewallPortsToVPC(vpcName string, containerNames []string, osImages []string) error {
	allowedPorts := []string{}
	sm.mu.RLock()
	rules := sm.fwRules[vpcName]
	sm.mu.RUnlock()

	for _, r := range rules {
		if r.Action == "allow" && r.Direction == "INGRESS" {
			for _, p := range r.Ports {
				allowedPorts = append(allowedPorts, p)
			}
		}
	}

	log.Printf("[Orchestrator] Applying firewall ports %v to VPC '%s' (recreating %d VMs)", allowedPorts, vpcName, len(containerNames))
	for i, cName := range containerNames {
		osImage := osImages[i]
		sm.DeleteComputeVM(cName)
		sm.ProvisionComputeVM(cName, osImage, vpcName, allowedPorts)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Level 3: Proxy-Level Enforcement
// ─────────────────────────────────────────────────────────────────────────────

func (sm *ServiceManager) RegisterFirewallRule(vpc string, entry FirewallEntry) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.fwRules[vpc] == nil {
		sm.fwRules[vpc] = []FirewallEntry{}
	}
	sm.fwRules[vpc] = append(sm.fwRules[vpc], entry)
}

func (sm *ServiceManager) RemoveFirewallRule(vpc, ruleName string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	rules := sm.fwRules[vpc]
	for i, r := range rules {
		if r.Name == ruleName {
			sm.fwRules[vpc] = append(rules[:i], rules[i+1:]...)
			break
		}
	}
}

func (sm *ServiceManager) CheckFirewallAllows(vpcName, protocol, port, sourceIP string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	rules := sm.fwRules[vpcName]
	
	allowed := false
	for _, r := range rules {
		if r.Direction == "INGRESS" {
			protoMatch := r.Protocol == "all" || strings.ToLower(r.Protocol) == strings.ToLower(protocol)
			portMatch := len(r.Ports) == 0
			for _, p := range r.Ports {
				if p == port {
					portMatch = true
					break
				}
			}
			rangeMatch := len(r.Ranges) == 0
			for _, src := range r.Ranges {
				if src == "0.0.0.0/0" || src == sourceIP {
					rangeMatch = true
					break
				}
			}

			if protoMatch && portMatch && rangeMatch {
				if r.Action == "deny" {
					return false
				}
				if r.Action == "allow" {
					allowed = true
				}
			}
		}
	}
	return allowed
}
