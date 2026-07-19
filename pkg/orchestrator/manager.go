package orchestrator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"minisky/pkg/config"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const networkName = "minisky-net"

// DockerNetworkNameForVPC maps a VPC name to the Docker network that backs it:
// "" or "default" is the shared bridge, everything else gets its own
// "minisky-vpc-<name>" network. This is the single source of truth for the
// mapping — every place that needs to go from VPC name to Docker network (or
// back) should call this or VPCNameForDockerNetwork instead of re-deriving it.
func DockerNetworkNameForVPC(vpcName string) string {
	if vpcName == "" || vpcName == "default" {
		return networkName
	}
	return "minisky-vpc-" + vpcName
}

// VPCNameForDockerNetwork is the inverse of DockerNetworkNameForVPC: given a
// Docker network name, returns the VPC name it represents, or ("", false) if
// the network isn't one minisky manages (e.g. Docker's own "bridge" network).
func VPCNameForDockerNetwork(dockerNet string) (string, bool) {
	if dockerNet == networkName {
		return "default", true
	}
	if strings.HasPrefix(dockerNet, "minisky-vpc-") {
		return strings.TrimPrefix(dockerNet, "minisky-vpc-"), true
	}
	return "", false
}

// MergeUniquePorts returns the ports allowed across every given VPC, deduped
// while preserving first-seen order. portsFor returns the ingress-allow ports
// for a single VPC (e.g. ServiceManager.allowedPortsForVPC or a shim's
// getAllowedPortsForVPC), so both orchestrator and shim packages can share
// this merge logic instead of reimplementing it.
func MergeUniquePorts(vpcNames []string, portsFor func(string) []string) []string {
	var merged []string
	for _, vpc := range vpcNames {
		merged = append(merged, portsFor(vpc)...)
	}
	return dedupStrings(merged)
}

// dedupStrings removes duplicate entries while preserving first-seen order.
func dedupStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ServiceManager handles native REST-driven lifecycle events over the Docker Unix Socket.
type ServiceManager struct {
	mu           sync.RWMutex
	dockerClient *http.Client
	sockPath     string
	portRegistry map[string][]PortMapping   // containerName → host ports
	fwRules      map[string][]FirewallEntry // vpcName → rules
	vpcRegistry  map[string][]string        // containerName → vpcNames it was last provisioned with
}

// ContainerConfig describes one backend emulator container.
type ContainerConfig struct {
	Name          string
	Image         string
	ContainerPort string // e.g. "4443/tcp"
	Cmd           []string
	Volume        string
	Env           []string
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

func NewServiceManager() (*ServiceManager, error) {
	sockPath := resolveDockerSocket()
	// On Unix, ensure DOCKER_HOST is set if we found a socket
	if !strings.HasPrefix(sockPath, "//./pipe/") && os.Getenv("DOCKER_HOST") == "" {
		os.Setenv("DOCKER_HOST", "unix://"+sockPath)
	}
	log.Printf("[ServiceManager] Docker socket resolved: %s", sockPath)
	sm := &ServiceManager{
		sockPath:     sockPath,
		portRegistry: make(map[string][]PortMapping),
		fwRules:      make(map[string][]FirewallEntry),
		vpcRegistry:  make(map[string][]string),
	}
	transport := &http.Transport{
		DialContext: sm.dialDocker,
	}
	sm.dockerClient = &http.Client{Transport: transport}
	return sm, nil
}

// NewServiceManagerForTesting builds a ServiceManager backed by the given
// http.RoundTripper instead of a real Docker socket, so other packages' tests
// can exercise Docker-calling logic (e.g. compute's patchNetworkIPs) without a
// Docker daemon.
func NewServiceManagerForTesting(transport http.RoundTripper) *ServiceManager {
	return &ServiceManager{
		dockerClient: &http.Client{Transport: transport},
		portRegistry: make(map[string][]PortMapping),
		fwRules:      make(map[string][]FirewallEntry),
		vpcRegistry:  make(map[string][]string),
	}
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
func (sm *ServiceManager) EnsureServiceRunning(ctx context.Context, domain string, env ...string) (string, error) {
	reg := config.GetImageRegistry()
	cfg, exists := reg.Emulators[domain]
	if !exists {
		// Native Go shims never need Docker containers
		return "", nil
	}

	// Map config to internal ContainerConfig
	iconfig := ContainerConfig{
		Name:          cfg.Name,
		Image:         cfg.Image,
		ContainerPort: cfg.Port,
		Cmd:           cfg.Cmd,
		Volume:        cfg.Volume,
		Env:           env,
	}

	status, err := sm.checkStatus(cfg.Name)
	if err != nil {
		return "", fmt.Errorf("status check failed: %v", err)
	}

	if status != "running" {
		log.Printf("[Orchestrator] Cold-starting '%s' for domain '%s'...", iconfig.Name, domain)
		if status == "not_found" {
			exists, err := sm.ImageExistsPublic(iconfig.Image)
			if err != nil {
				log.Printf("[Orchestrator] Image check error: %v", err)
			}
			if !exists {
				log.Printf("[Orchestrator] Pulling image '%s'...", iconfig.Image)
				if err := sm.pullImageInternal(iconfig.Image); err != nil {
					log.Printf("[Orchestrator] Image pull warning: %v", err)
				}
				log.Printf("[Orchestrator] Image '%s' pull complete.", iconfig.Image)
			} else {
				log.Printf("[Orchestrator] Image '%s' already exists locally, skipping pull.", iconfig.Image)
			}
			log.Printf("[Orchestrator] Creating container '%s'...", iconfig.Name)
			if err := sm.createContainer(iconfig); err != nil {
				return "", fmt.Errorf("create container: %v", err)
			}
			log.Printf("[Orchestrator] Container '%s' created.", iconfig.Name)
		}
		log.Printf("[Orchestrator] Starting container '%s'...", iconfig.Name)
		if err := sm.startContainer(iconfig.Name); err != nil {
			return "", fmt.Errorf("start container: %v", err)
		}
		log.Printf("[Orchestrator] Container '%s' started.", iconfig.Name)
	}

	// Discover the internal bridge IP — no host port binding needed
	log.Printf("[Orchestrator] Discovering internal URL for '%s'...", iconfig.Name)
	internalURL, err := sm.discoverInternalURL(iconfig)
	if err != nil {
		return "", fmt.Errorf("port discovery: %v", err)
	}

	// Wait until the emulator is truly ready inside the network
	containerPort := strings.Split(iconfig.ContainerPort, "/")[0]
	internalAddr := strings.TrimPrefix(internalURL, "http://")
	log.Printf("[Orchestrator] Waiting for readiness probe at %s...", internalAddr)
	if err := sm.waitUntilReady(internalAddr, 60*time.Second); err != nil {
		return "", fmt.Errorf("readiness probe failed: %v", err)
	}

	log.Printf("[Orchestrator] ✅ '%s' is ONLINE at internal %s (port %s)", iconfig.Name, internalURL, containerPort)
	return internalURL, nil
}

// StopServiceContainer stops the underlying docker container for a given service domain.
func (sm *ServiceManager) StopAndRemoveContainer(name string) error {
	// 1. Stop
	stopURL := fmt.Sprintf("http://localhost/containers/%s/stop", name)
	reqStop, _ := http.NewRequest("POST", stopURL, nil)
	respStop, err := sm.dockerClient.Do(reqStop)
	if err == nil {
		respStop.Body.Close()
	}

	// 2. Remove
	rmURL := fmt.Sprintf("http://localhost/containers/%s?force=true", name)
	reqRm, _ := http.NewRequest("DELETE", rmURL, nil)
	respRm, err := sm.dockerClient.Do(reqRm)
	if err != nil {
		return err
	}
	defer respRm.Body.Close()
	if respRm.StatusCode >= 400 && respRm.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(respRm.Body)
		return fmt.Errorf("remove container %q rejected %d: %s", name, respRm.StatusCode, b)
	}
	return nil
}

func (sm *ServiceManager) StopServiceContainer(ctx context.Context, domain string) error {
	reg := config.GetImageRegistry()
	cfg, exists := reg.Emulators[domain]
	if !exists {
		return fmt.Errorf("domain %s not found in registry", domain)
	}

	status, err := sm.checkStatus(cfg.Name)
	if err != nil {
		return fmt.Errorf("status check failed: %v", err)
	}

	if status == "running" {
		log.Printf("[Orchestrator] Stopping service container '%s'...", cfg.Name)
		stopURL := fmt.Sprintf("http://localhost/containers/%s/stop", cfg.Name)
		req, _ := http.NewRequestWithContext(ctx, "POST", stopURL, nil)
		resp, err := sm.dockerClient.Do(req)
		if err != nil {
			return fmt.Errorf("stop container network error: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotModified {
			return fmt.Errorf("stop rejected %d", resp.StatusCode)
		}
		log.Printf("[Orchestrator] Container '%s' stopped successfully.", cfg.Name)
	} else {
		log.Printf("[Orchestrator] Container '%s' is already not running (status: %s)", cfg.Name, status)
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

// GetContainerHostPort reads the host-bound port assigned by Docker.
func (sm *ServiceManager) GetContainerHostPort(containerName string, containerPort string) (string, error) {
	resp, err := sm.dockerClient.Get(fmt.Sprintf("http://localhost/containers/%s/json", containerName))
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

	bindings, ok := info.NetworkSettings.Ports[containerPort]
	if !ok || len(bindings) == 0 || bindings[0].HostPort == "" {
		return "", fmt.Errorf("container '%s' has no host port binding for %s", containerName, containerPort)
	}

	return bindings[0].HostPort, nil
}

// Teardown stops and removes all minisky-* containers and the minisky-net network.
func (sm *ServiceManager) Teardown(ctx context.Context) {
	reg := config.GetImageRegistry()
	for _, cfg := range reg.Emulators {
		stopURL := fmt.Sprintf("http://localhost/containers/%s/stop", cfg.Name)
		req, _ := http.NewRequestWithContext(ctx, "POST", stopURL, nil)
		sm.dockerClient.Do(req)

		rmURL := fmt.Sprintf("http://localhost/containers/%s?force=true", cfg.Name)
		req, _ = http.NewRequestWithContext(ctx, "DELETE", rmURL, nil)
		sm.dockerClient.Do(req)
		log.Printf("[Orchestrator] Removed container '%s'", cfg.Name)
	}
	rmNetURL := "http://localhost/networks/" + networkName
	req, _ := http.NewRequestWithContext(ctx, "DELETE", rmNetURL, nil)
	sm.dockerClient.Do(req)
	log.Printf("[Orchestrator] Removed network '%s'", networkName)
}

// PruneExitedContainers removes all containers that are not running.
func (sm *ServiceManager) PruneExitedContainers(ctx context.Context) error {
	resp, err := sm.dockerClient.Get("http://localhost/containers/json?all=true&filters={\"status\":[\"exited\",\"created\",\"dead\"]}")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var containers []struct {
		Id    string
		Names []string
	}
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return err
	}

	for _, c := range containers {
		name := "unknown"
		if len(c.Names) > 0 {
			name = c.Names[0]
		}
		log.Printf("[Orchestrator] Pruning container: %s (%s)", name, c.Id)
		rmURL := fmt.Sprintf("http://localhost/containers/%s?force=true", c.Id)
		req, _ := http.NewRequestWithContext(ctx, "DELETE", rmURL, nil)
		sm.dockerClient.Do(req)
	}
	return nil
}

// PruneUnusedImages removes all minisky-fn-* and minisky-svc-* images that are not used by any container.
func (sm *ServiceManager) PruneUnusedImages(ctx context.Context) error {
	// 1. Get all containers to see which images are in use
	resp, err := sm.dockerClient.Get("http://localhost/containers/json?all=true")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var containers []struct {
		Image   string
		ImageID string
	}
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return err
	}

	usedImages := make(map[string]bool)
	for _, c := range containers {
		usedImages[c.Image] = true
		usedImages[c.ImageID] = true
	}

	// 2. List all images
	imgResp, err := sm.dockerClient.Get("http://localhost/images/json")
	if err != nil {
		return err
	}
	defer imgResp.Body.Close()

	var images []struct {
		Id       string
		RepoTags []string
	}
	if err := json.NewDecoder(imgResp.Body).Decode(&images); err != nil {
		return err
	}

	for _, img := range images {
		isMiniSky := false
		tagName := ""
		for _, tag := range img.RepoTags {
			if strings.Contains(tag, "minisky-fn-") || strings.Contains(tag, "minisky-svc-") {
				isMiniSky = true
				tagName = tag
				break
			}
		}

		if isMiniSky {
			// Check if used
			if usedImages[img.Id] || (tagName != "" && usedImages[tagName]) {
				continue
			}

			log.Printf("[Orchestrator] Pruning unused MiniSky image: %s (%s)", tagName, img.Id)
			rmURL := fmt.Sprintf("http://localhost/images/%s?force=true", img.Id)
			req, _ := http.NewRequestWithContext(ctx, "DELETE", rmURL, nil)
			sm.dockerClient.Do(req)
		}
	}

	return nil
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

func (sm *ServiceManager) pullImageInternal(image string) error {
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://localhost/images/create?fromImage=%s", image), nil)
	resp, err := sm.dockerClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

func (sm *ServiceManager) ImageExistsPublic(image string) (bool, error) {
	// Docker inspect image endpoint
	url := fmt.Sprintf("http://localhost/images/%s/json", image)
	resp, err := sm.dockerClient.Get(url)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// ProvisionComputeVM actively boots a Data Plane Docker container mimicking a GCE VM.
// It also creates the container on its primary network (vpcNames[0], or
// "default" if empty). Docker's /containers/create only accepts one network at
// creation time, so any additional entries in vpcNames are attached afterwards
// via ConnectContainerToNetwork.
func (sm *ServiceManager) ProvisionComputeVM(containerName string, osImage string, vpcNames []string, ports []string, env []string, cmd []string) error {
	vpcNames = dedupStrings(vpcNames)
	primaryVpc := "default"
	if len(vpcNames) > 0 {
		primaryVpc = vpcNames[0]
	}
	log.Printf("[Orchestrator] Provisioning compute VM: %s (image: %s vpcs: %v ports: %d env: %d cmd: %v)", containerName, osImage, vpcNames, len(ports), len(env), cmd)

	exists, err := sm.ImageExistsPublic(osImage)
	if err != nil {
		log.Printf("[Orchestrator] Image check error for %s: %v", osImage, err)
	}
	if !exists {
		if err := sm.pullImageInternal(osImage); err != nil {
			log.Printf("[Orchestrator] Data Plane pull warning for %s: %v", osImage, err)
		}
	} else {
		log.Printf("[Orchestrator] Image '%s' already exists locally, skipping pull.", osImage)
	}

	netMode := DockerNetworkNameForVPC(primaryVpc)

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
		"Env":          append(sm.standardEnv(), env...),
		"ExposedPorts": exposedPorts,
		"HostConfig": map[string]interface{}{
			"NetworkMode":  netMode,
			"PortBindings": portBindings,
		},
	}
	if len(cmd) > 0 {
		payload["Cmd"] = cmd
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

	// From here on, GCE's instances.insert is atomic — a VM either comes up fully
	// or doesn't get created at all — so any failure rolls the container back
	// rather than leaving a half-provisioned container behind.
	if err := sm.startContainer(containerName); err != nil {
		if rmErr := sm.StopAndRemoveContainer(containerName); rmErr != nil {
			log.Printf("[Orchestrator] rollback of '%s' after start failure failed: %v", containerName, rmErr)
		}
		return fmt.Errorf("start container %q: %v", containerName, err)
	}

	// Attach every network past the primary one now that the container exists.
	// A failed attach here rolls the container back the same way, rather than
	// leaving it half-attached. ConnectContainerToNetwork tolerates the container already being a
	// member of the target network, so a retried/duplicate call for the same
	// containerName+vpcNames (e.g. a retried insert, or a firewall-triggered recreate
	// whose preceding delete didn't fully take effect) is idempotent and won't roll
	// back an otherwise-healthy container.
	var extraVPCs []string
	if len(vpcNames) > 0 {
		extraVPCs = vpcNames[1:]
	}
	for _, extra := range extraVPCs {
		if err := sm.ConnectContainerToNetwork(containerName, extra); err != nil {
			if rmErr := sm.StopAndRemoveContainer(containerName); rmErr != nil {
				log.Printf("[Orchestrator] rollback of '%s' failed: %v", containerName, rmErr)
			}
			return fmt.Errorf("attach network %q: %v", extra, err)
		}
	}

	finalVPCNames := vpcNames
	if len(finalVPCNames) == 0 {
		finalVPCNames = []string{"default"}
	}
	sm.mu.Lock()
	sm.vpcRegistry[containerName] = finalVPCNames
	sm.mu.Unlock()

	return sm.updatePortRegistry(containerName)
}

// ProvisionCloudSQLVM starts a fully-interactive PostgreSQL or MySQL docker database data plane.
func (sm *ServiceManager) ProvisionBuildStep(containerName string, image string, binds []string, env []string, cmd []string) error {
	log.Printf("[Orchestrator] Provisioning build step: %s (image: %s binds: %v cmd: %v)", containerName, image, binds, cmd)

	exists, _ := sm.ImageExistsPublic(image)
	if !exists {
		sm.pullImageInternal(image)
	}

	payload := map[string]interface{}{
		"Image":      image,
		"WorkingDir": "/workspace",
		"Env":        append(sm.standardEnv(), env...),
		"HostConfig": map[string]interface{}{
			"NetworkMode": networkName,
			"Binds":       binds,
		},
		"Labels": map[string]string{
			"managed-by": "minisky-build",
			"build-id":   containerName,
		},
	}
	if len(cmd) > 0 {
		payload["Cmd"] = cmd
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

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("build step creation rejected %d: %s", resp.StatusCode, b)
	}

	return sm.startContainer(containerName)
}

func (sm *ServiceManager) ProvisionCloudSQLVM(instanceName string, version string, rootPassword string) (string, error) {
	var image string
	var env []string
	var expPort string

	reg := config.GetImageRegistry()
	if strings.HasPrefix(version, "POSTGRES") {
		// Version can be "POSTGRES_18", "POSTGRES_17", or just "POSTGRES"
		vparts := strings.Split(version, "_")
		if len(vparts) > 1 {
			targetV := vparts[1]
			for _, v := range reg.Sql.Postgres.Versions {
				if v.Version == targetV {
					image = v.Image
					break
				}
			}
		}
		if image == "" {
			image = reg.Sql.Postgres.DefaultImage
		}
		env = append(sm.standardEnv(),
			"POSTGRES_PASSWORD="+rootPassword,
			"PGDATA=/var/lib/postgresql/data",
		)
		expPort = "5432/tcp"
	} else if strings.HasPrefix(version, "MYSQL") {
		vparts := strings.Split(version, "_")
		if len(vparts) > 1 {
			targetV := vparts[1]
			// Handle legacy version strings like MYSQL_8_0
			if len(vparts) > 2 {
				targetV = vparts[1] + "." + vparts[2]
			}
			for _, v := range reg.Sql.Mysql.Versions {
				if v.Version == targetV || strings.HasPrefix(v.Version, vparts[1]) {
					image = v.Image
					break
				}
			}
		}
		if image == "" {
			image = reg.Sql.Mysql.DefaultImage
		}
		env = append(sm.standardEnv(), "MYSQL_ROOT_PASSWORD="+rootPassword)
		expPort = "3306/tcp"
	} else {
		return "", fmt.Errorf("unsupported database version: %s", version)
	}

	containerName := "minisky-sql-" + instanceName
	log.Printf("[Orchestrator] Provisioning Cloud SQL VM: %s (image: %s)", containerName, image)

	exists, err := sm.ImageExistsPublic(image)
	if err != nil {
		log.Printf("[Orchestrator] Image check error for %s: %v", image, err)
	}
	if !exists {
		if err := sm.pullImageInternal(image); err != nil {
			log.Printf("[Orchestrator] Pull warning for %s: %v", image, err)
		}
	} else {
		log.Printf("[Orchestrator] Image '%s' already exists locally, skipping pull.", image)
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
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remove container %q rejected %d: %s", containerName, resp.StatusCode, b)
	}
	return nil
}

// ProvisionServerlessVM starts a container from a custom image (typically built by Buildpacks).
func (sm *ServiceManager) ProvisionServerlessVM(resourceName string, image string, env []string) (string, error) {
	containerName := "minisky-serverless-" + resourceName
	log.Printf("[Orchestrator] Provisioning Serverless VM: %s (image: %s)", containerName, image)

	// Clean up any stale container
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("http://localhost/containers/%s?force=true", containerName), nil)
	sm.dockerClient.Do(req)

	expPort := "8080/tcp"
	payload := map[string]interface{}{
		"Image": image,
		"Env":   append(sm.standardEnv(), env...),
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
		},
		"Labels": map[string]string{
			"managed-by": "minisky-serverless",
			"resource":   resourceName,
		},
	}

	b, _ := json.Marshal(payload)
	cReq, _ := http.NewRequest("POST", "http://localhost/containers/create?name="+containerName, bytes.NewBuffer(b))
	cReq.Header.Set("Content-Type", "application/json")
	resp, err := sm.dockerClient.Do(cReq)
	if err != nil {
		return "", fmt.Errorf("create Serverless container: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create Serverless container %d: %s", resp.StatusCode, string(respBody))
	}

	if err := sm.startContainer(containerName); err != nil {
		return "", fmt.Errorf("start Serverless container: %v", err)
	}

	config := ContainerConfig{Name: containerName, ContainerPort: expPort}
	internalURL, err := sm.discoverInternalURL(config)
	if err != nil {
		return "", fmt.Errorf("port discovery: %v", err)
	}

	log.Printf("[Orchestrator] ✅ Serverless Instance '%s' ONLINE at %s", resourceName, internalURL)
	return internalURL, nil
}

// GetContainerLogs returns the last 'tail' lines of stdout/stderr from a container.
func (sm *ServiceManager) GetContainerLogs(containerName string, tail int) (string, error) {
	url := fmt.Sprintf("http://localhost/containers/%s/logs?stdout=true&stderr=true&tail=%d", containerName, tail)
	return sm.fetchLogs(url)
}

// GetContainerLogsSince returns stdout/stderr logs since a specific unix timestamp
func (sm *ServiceManager) GetContainerLogsSince(containerName string, since int64) (string, error) {
	url := fmt.Sprintf("http://localhost/containers/%s/logs?stdout=true&stderr=true&timestamps=true&since=%d", containerName, since)
	return sm.fetchLogs(url)
}

func (sm *ServiceManager) fetchLogs(url string) (string, error) {
	resp, err := sm.dockerClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "Log source not found.", nil
	}

	// Docker logs stream format: [8]byte header + payload.
	body, _ := io.ReadAll(resp.Body)

	// Quick header strip for standard docker logs stream headers (8 bytes)
	var result strings.Builder
	for i := 0; i < len(body); {
		if i+8 > len(body) {
			break
		}
		i += 8
		// read until end of chunk or next header
		next := i
		for next < len(body) && (next+8 > len(body) || (body[next] != 1 && body[next] != 2)) {
			next++
		}
		result.Write(body[i:next])
		i = next
	}

	if result.Len() == 0 && len(body) > 0 {
		return string(body), nil
	}

	return result.String(), nil
}

// RunCommandInContainer executes a non-interactive command inside a container.
func (sm *ServiceManager) RunCommandInContainer(name string, cmd []string) (string, error) {
	log.Printf("[Orchestrator] Executing command in '%s': %v", name, cmd)

	// 1. Create the exec instance
	payload := map[string]interface{}{
		"AttachStdin":  false,
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          false,
		"Cmd":          cmd,
	}
	body, _ := json.Marshal(payload)
	createURL := fmt.Sprintf("http://localhost/containers/%s/exec", name)
	resp, err := sm.dockerClient.Post(createURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create exec (%d): %s", resp.StatusCode, b)
	}

	var execData struct{ Id string }
	json.NewDecoder(resp.Body).Decode(&execData)

	// 2. Start the exec instance
	startPayload := `{"Detach": false, "Tty": false}`
	startURL := fmt.Sprintf("http://localhost/exec/%s/start", execData.Id)
	startResp, err := sm.dockerClient.Post(startURL, "application/json", strings.NewReader(startPayload))
	if err != nil {
		return "", err
	}
	defer startResp.Body.Close()

	if startResp.StatusCode >= 400 {
		b, _ := io.ReadAll(startResp.Body)
		return "", fmt.Errorf("failed to start exec (%d): %s", startResp.StatusCode, b)
	}

	// 3. Collect output (Docker stream format)
	rawOutput, _ := io.ReadAll(startResp.Body)

	// Helper to strip headers
	var result strings.Builder
	for i := 0; i < len(rawOutput); {
		if i+8 > len(rawOutput) {
			break
		}
		// Skip header
		i += 8
		next := i
		// Read until next header or end
		for next < len(rawOutput) && (next+8 > len(rawOutput) || (rawOutput[next] != 1 && rawOutput[next] != 2)) {
			next++
		}
		result.Write(rawOutput[i:next])
		i = next
	}

	if result.Len() == 0 && len(rawOutput) > 0 {
		return string(rawOutput), nil
	}

	return result.String(), nil
}

func (sm *ServiceManager) createContainer(c ContainerConfig) error {
	// Bind container port to a random localhost port — works with Docker Desktop
	// (which runs in a VM where internal bridge IPs aren't host-reachable).
	hostCfg := map[string]interface{}{
		"NetworkMode": networkName,
		"PortBindings": map[string]interface{}{
			c.ContainerPort: []map[string]string{
				{"HostIp": "127.0.0.1", "HostPort": "0"},
			},
		},
	}
	if c.Volume != "" {
		vol := c.Volume
		lastColon := strings.LastIndex(vol, ":")
		if lastColon > 0 {
			hostPath := vol[:lastColon]
			containerPath := vol[lastColon+1:]
			if strings.ContainsAny(hostPath, `/\`) || hostPath == "." || hostPath == ".." {
				if !filepath.IsAbs(hostPath) {
					if abs, err := filepath.Abs(hostPath); err == nil {
						vol = abs + ":" + containerPath
					}
				}
			}
		}
		hostCfg["Binds"] = []string{vol}
	}

	payload := map[string]interface{}{
		"Image":        c.Image,
		"Cmd":          c.Cmd,
		"ExposedPorts": map[string]interface{}{c.ContainerPort: struct{}{}},
		"HostConfig":   hostCfg,
		"Env":          c.Env,
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

// resolveDockerSocket and dialDocker are implemented in OS-specific files (dialer_unix.go, dialer_windows.go)

// ─────────────────────────────────────────────────────────────────────────────
// Level 1: VPC Network Management
// ─────────────────────────────────────────────────────────────────────────────

func (sm *ServiceManager) CreateVPCNetwork(ctx context.Context, name string) error {
	netName := DockerNetworkNameForVPC(name)
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
	netName := DockerNetworkNameForVPC(name)
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

// containerInspectInfo is the subset of Docker's container-inspect response
// (GET /containers/{name}/json) that ServiceManager cares about. Every
// caller that needs it goes through inspectContainer instead of issuing its
// own HTTP call + decode, so the non-2xx status check only has to live in
// one place.
type containerInspectInfo struct {
	HostConfig struct {
		NetworkMode string
	}
	NetworkSettings struct {
		Networks map[string]struct {
			IPAddress string
		}
		Ports map[string][]struct {
			HostIp   string
			HostPort string
		}
	}
}

// inspectContainer fetches and decodes a container's Docker inspect response.
// It returns an error for both connection failures and non-2xx responses —
// a 404/5xx JSON error body would otherwise decode "successfully" into a
// zero-valued containerInspectInfo, masking the failure as an empty result.
func (sm *ServiceManager) inspectContainer(containerName string) (*containerInspectInfo, error) {
	resp, err := sm.dockerClient.Get(fmt.Sprintf("http://localhost/containers/%s/json", containerName))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("inspect %q failed %d: %s", containerName, resp.StatusCode, b)
	}
	var info containerInspectInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}
	return &info, nil
}

func (sm *ServiceManager) updatePortRegistry(containerName string) error {
	info, err := sm.inspectContainer(containerName)
	if err != nil {
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
	log.Printf("[Orchestrator] Applying firewall change for VPC '%s' (recreating %d VMs)", vpcName, len(containerNames))
	for i, cName := range containerNames {
		osImage := osImages[i]

		// Capture every network cName is currently attached to before tearing it
		// down. Prefer what ProvisionComputeVM recorded at provision time
		// (cheap, no Docker round-trip); fall back to asking Docker directly for
		// containers minisky doesn't have in-memory state for (e.g. it was
		// restarted since this container was provisioned).
		vpcNames := sm.registeredVPCNames(cName)
		if vpcNames == nil {
			var err error
			vpcNames, err = sm.attachedVPCNames(cName)
			if err != nil {
				log.Printf("[Orchestrator] could not determine attached VPCs for '%s', skipping firewall recreate to avoid dropping its other network memberships: %v", cName, err)
				continue
			}
		}
		if len(vpcNames) == 0 {
			vpcNames = []string{vpcName}
		}

		// Merge ingress-allow ports across every attached VPC, not just the one
		// whose rules changed — mirrors how a real GCE VM inherits firewall rules
		// from every network it's attached to.
		allowedPorts := MergeUniquePorts(vpcNames, sm.allowedPortsForVPC)

		log.Printf("[Orchestrator] Recreating '%s' with ports %v on vpcs %v", cName, allowedPorts, vpcNames)
		if err := sm.DeleteComputeVM(cName); err != nil {
			log.Printf("[Orchestrator] failed to delete '%s' before firewall recreate: %v", cName, err)
		}
		if err := sm.ProvisionComputeVM(cName, osImage, vpcNames, allowedPorts, []string{}, []string{"tail", "-f", "/dev/null"}); err != nil {
			log.Printf("[Orchestrator] failed to recreate '%s' after firewall change, it may now be missing: %v", cName, err)
		}
	}
	return nil
}

// registeredVPCNames returns the VPC names ProvisionComputeVM last recorded
// for containerName, or nil if minisky has no in-memory record of it.
func (sm *ServiceManager) registeredVPCNames(containerName string) []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.vpcRegistry[containerName]
}

// allowedPortsForVPC returns the ingress-allow ports for a single VPC's firewall rules.
func (sm *ServiceManager) allowedPortsForVPC(vpcName string) []string {
	sm.mu.RLock()
	rules := sm.fwRules[vpcName]
	sm.mu.RUnlock()

	var ports []string
	for _, r := range rules {
		if r.Action == "allow" && r.Direction == "INGRESS" {
			ports = append(ports, r.Ports...)
		}
	}
	return ports
}

// attachedVPCNames inspects a running container's Docker networks and translates
// each one back into the VPC name ProvisionComputeVM understands (the inverse of
// the netMode derivation there), so a firewall-triggered recreate can reattach
// every network the container had, not just the one whose rules changed.
//
// The result is sorted, with the container's original primary network (its
// HostConfig.NetworkMode at creation time) moved to the front — Docker's own
// NetworkSettings.Networks is a map, so iterating it directly would make
// vpcNames[0] (and thus the primary network on the next recreate) change
// randomly from one call to the next.
//
// An error is returned (rather than an empty slice) when the container's
// networks couldn't be determined, so callers can distinguish "this container
// legitimately has no recognized VPCs attached" from "we failed to find out" —
// conflating the two would silently drop VPC membership on a transient
// Docker API hiccup.
func (sm *ServiceManager) attachedVPCNames(containerName string) ([]string, error) {
	info, err := sm.inspectContainer(containerName)
	if err != nil {
		return nil, err
	}

	var vpcNames []string
	for dockerNet := range info.NetworkSettings.Networks {
		if vpc, ok := VPCNameForDockerNetwork(dockerNet); ok {
			vpcNames = append(vpcNames, vpc)
		}
	}
	sort.Strings(vpcNames)

	if primaryVpc, ok := VPCNameForDockerNetwork(info.HostConfig.NetworkMode); ok {
		for i, v := range vpcNames {
			if v == primaryVpc {
				vpcNames[0], vpcNames[i] = vpcNames[i], vpcNames[0]
				break
			}
		}
	}

	return vpcNames, nil
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

// StreamContainerExec initiates an interactive session with a container.
// It returns a hijacked physical connection to the Docker daemon.
func (sm *ServiceManager) StreamContainerExec(name string) (net.Conn, error) {
	// 1. Create the exec instance
	// We try bash first, falling back to sh if needed
	payload := map[string]interface{}{
		"AttachStdin":  true,
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          true,
		"Cmd":          []string{"/bin/bash"},
		"User":         "root",
	}
	body, _ := json.Marshal(payload)
	createURL := fmt.Sprintf("http://localhost/containers/%s/exec", name)
	resp, err := sm.dockerClient.Post(createURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		// Try fallback to /bin/sh
		payload["Cmd"] = []string{"/bin/sh"}
		body, _ = json.Marshal(payload)
		resp, err = sm.dockerClient.Post(createURL, "application/json", bytes.NewBuffer(body))
		if err != nil || resp.StatusCode != http.StatusCreated {
			return nil, fmt.Errorf("failed to create exec: %d", resp.StatusCode)
		}
		defer resp.Body.Close()
	}

	var execData struct{ Id string }
	json.NewDecoder(resp.Body).Decode(&execData)

	// 2. Start and Hijack the connection
	// We must dial the socket directly to bypass http.Client's pooling and response handling
	conn, err := sm.dialDocker(context.Background(), "", "")
	if err != nil {
		return nil, err
	}

	startPayload := `{"Detach": false, "Tty": true}`
	reqStr := fmt.Sprintf("POST /exec/%s/start HTTP/1.1\r\n"+
		"Host: localhost\r\n"+
		"Content-Type: application/json\r\n"+
		"Connection: Upgrade\r\n"+
		"Upgrade: tcp\r\n"+
		"Content-Length: %d\r\n\r\n%s",
		execData.Id, len(startPayload), startPayload)

	if _, err := conn.Write([]byte(reqStr)); err != nil {
		conn.Close()
		return nil, err
	}

	// Read the response header to ensure it started correctly
	// We use a buffered reader to parse the response, then return a wrapper
	// that continues reading from the same buffer to avoid data loss.
	bufReader := bufio.NewReader(conn)
	r, err := http.ReadResponse(bufReader, &http.Request{Method: "POST"})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read exec start response: %v", err)
	}
	if r.StatusCode != http.StatusOK && r.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("unexpected exec start status: %s", r.Status)
	}

	return &bufferedConn{Conn: conn, r: bufReader}, nil
}

type bufferedConn struct {
	net.Conn
	r io.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	return b.r.Read(p)
}

func (sm *ServiceManager) DoDockerRequest(req *http.Request) (*http.Response, error) {
	return sm.dockerClient.Do(req)
}

// GetContainerNetworks fetches a container's Docker network membership in a
// single inspect call, returning a map of Docker network name -> IP address
// on that network. Callers that need the IP on more than one network (e.g. a
// multi-NIC VM) should call this once and look up each network locally,
// rather than issuing one inspect per network.
func (sm *ServiceManager) GetContainerNetworks(containerName string) (map[string]string, error) {
	info, err := sm.inspectContainer(containerName)
	if err != nil {
		return nil, err
	}

	ips := make(map[string]string, len(info.NetworkSettings.Networks))
	for name, n := range info.NetworkSettings.Networks {
		ips[name] = n.IPAddress
	}
	return ips, nil
}

// GetContainerIPForNetwork retrieves a container's IP on one specific Docker
// network, so a multi-NIC VM can report the correct address per interface
// instead of collapsing every interface onto a single network's IP.
func (sm *ServiceManager) GetContainerIPForNetwork(containerName string, dockerNetworkName string) (string, error) {
	ips, err := sm.GetContainerNetworks(containerName)
	if err != nil {
		return "", err
	}
	return ips[dockerNetworkName], nil
}

// ConnectContainerToNetwork attaches an already-running container to an additional
// Docker network. Docker's /containers/create only accepts one network at creation
// time (see netMode in ProvisionComputeVM above); every network past the first has
// to be attached after the fact through this endpoint.
func (sm *ServiceManager) ConnectContainerToNetwork(containerName string, vpcName string) error {
	dockerNetworkName := DockerNetworkNameForVPC(vpcName)
	payload := map[string]interface{}{
		"Container": containerName,
	}
	body, _ := json.Marshal(payload)
	connectUrl := fmt.Sprintf("http://localhost/networks/%s/connect", dockerNetworkName)
	resp, err := sm.dockerClient.Post(connectUrl, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		// Docker rejects connecting a container that's already a member of the target
		// network. ProvisionComputeVM's caller can legitimately hit this on a
		// retried/duplicate provision call for the same containerName+vpcNames — real
		// GCE APIs are idempotent under retry, so treat "already attached" the same
		// way: a no-op success, not a failure that should roll back an
		// otherwise-healthy container.
		if isAlreadyAttachedError(string(b)) {
			return nil
		}
		return fmt.Errorf("connect failed %d: %s", resp.StatusCode, b)
	}
	return nil
}

// isAlreadyAttachedError reports whether a Docker network-connect error body
// indicates the container is already a member of the target network, rather
// than a genuine failure (network missing, daemon unreachable, etc.). Docker
// has mapped this case to different HTTP status codes across engine
// versions, so the message text is the more stable signal to match on.
func isAlreadyAttachedError(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "already exists in network") ||
		strings.Contains(lower, "already connected to network") ||
		strings.Contains(lower, "endpoint with name")
}

type ContainerSummary struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Image  string `json:"image"`
}

// ListManagedContainers lists all minisky-* containers.
func (sm *ServiceManager) ListManagedContainers() []ContainerSummary {
	resp, err := sm.dockerClient.Get(`http://localhost/containers/json?all=true&filters={"name":["minisky-"]}`)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	var raw []struct {
		Names  []string `json:"Names"`
		Status string   `json:"Status"`
		Image  string   `json:"Image"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil
	}
	out := make([]ContainerSummary, 0, len(raw))
	for _, c := range raw {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		out = append(out, ContainerSummary{Name: name, Status: c.Status, Image: c.Image})
	}
	return out
}

// ContainerStats represents the CPU and Memory usage of a container.
type ContainerStats struct {
	CPUPercentage float64
	MemoryUsageMB float64
}

// GetContainerStats retrieves the resource usage stats of a container.
func (sm *ServiceManager) GetContainerStats(name string) (*ContainerStats, error) {
	url := fmt.Sprintf("http://localhost/containers/%s/stats?stream=false", name)
	resp, err := sm.dockerClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get stats: status %d", resp.StatusCode)
	}

	var raw struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs     uint32 `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemCPUUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64            `json:"usage"`
			Stats map[string]uint64 `json:"stats"`
		} `json:"memory_stats"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	stats := &ContainerStats{}

	// Calculate CPU percentage
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(raw.CPUStats.SystemCPUUsage) - float64(raw.PreCPUStats.SystemCPUUsage)

	if systemDelta > 0.0 && cpuDelta > 0.0 {
		stats.CPUPercentage = (cpuDelta / systemDelta) * float64(raw.CPUStats.OnlineCPUs) * 100.0
	}

	// Calculate Memory Usage in MB (subtract inactive_file cache if available)
	memUsage := raw.MemoryStats.Usage
	if inactiveFile, ok := raw.MemoryStats.Stats["inactive_file"]; ok {
		if memUsage > inactiveFile {
			memUsage -= inactiveFile
		}
	}
	stats.MemoryUsageMB = float64(memUsage) / 1024.0 / 1024.0

	return stats, nil
}
func (sm *ServiceManager) standardEnv() []string {
	return []string{
		"SECRET_MANAGER_EMULATOR_HOST=minisky-secretmanager:8080",
		"PUBSUB_EMULATOR_HOST=minisky-pubsub:8085",
		"FIRESTORE_EMULATOR_HOST=minisky-firestore:8082",
		"DATASTORE_EMULATOR_HOST=minisky-datastore:8081",
		"BIGTABLE_EMULATOR_HOST=minisky-bigtable:8086",
		"STORAGE_EMULATOR_HOST=http://minisky-gcs:4443",
		"GOOGLE_CLOUD_PROJECT=default-project",
		// Internal gateway for REST shims
		"MINISKY_GATEWAY=172.17.0.1:8080",
	}
}
