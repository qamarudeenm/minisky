package gke

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
	registry.Register("container.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.OpMgr)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource types
// ─────────────────────────────────────────────────────────────────────────────

// Cluster mirrors the GKE container.v1 Cluster resource.
type Cluster struct {
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	NodeConfig       *NodeConfig     `json:"nodeConfig,omitempty"`
	MasterAuth       *MasterAuth     `json:"masterAuth,omitempty"`
	LoggingService   string          `json:"loggingService"`
	MonitoringService string         `json:"monitoringService"`
	Network          string          `json:"network"`
	ClusterIpv4Cidr  string          `json:"clusterIpv4Cidr"`
	Endpoint         string          `json:"endpoint"`
	InitialClusterVersion string     `json:"initialClusterVersion"`
	CurrentMasterVersion  string     `json:"currentMasterVersion"`
	Status           string          `json:"status"` // PROVISIONING, RUNNING, RECONCILING, STOPPING, ERROR, DEGRADED
	StatusMessage    string          `json:"statusMessage,omitempty"`
	NodeIpv4CidrSize int             `json:"nodeIpv4CidrSize"`
	ServicesIpv4Cidr string          `json:"servicesIpv4Cidr"`
	SelfLink         string          `json:"selfLink"`
	Zone             string          `json:"zone"`
	Location         string          `json:"location"`
	CreateTime       string          `json:"createTime"`
	InitialNodeCount int             `json:"initialNodeCount"`
}

type NodeConfig struct {
	MachineType string            `json:"machineType"`
	DiskSizeGb  int               `json:"diskSizeGb"`
	OauthScopes []string          `json:"oauthScopes"`
	Labels      map[string]string `json:"labels,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

type MasterAuth struct {
	Username              string     `json:"username,omitempty"`
	ClusterCaCertificate  string     `json:"clusterCaCertificate"`
	ClientCertificate     string     `json:"clientCertificate"`
	ClientKey             string     `json:"clientKey"`
}

// GkeOperation mirrors the GKE Operation resource.
type GkeOperation struct {
	Name          string `json:"name"`
	Zone          string `json:"zone"`
	OperationType string `json:"operationType"`
	Status        string `json:"status"` // PENDING, RUNNING, DONE, ABORTING
	StatusMessage string `json:"statusMessage,omitempty"`
	SelfLink      string `json:"selfLink"`
	TargetLink    string `json:"targetLink"`
	StartTime     string `json:"startTime,omitempty"`
	EndTime       string `json:"endTime,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// API shim
// ─────────────────────────────────────────────────────────────────────────────

// API is the high-fidelity GKE container.v1 shim.
type API struct {
	mu       sync.RWMutex
	opMgr    *orchestrator.OperationManager
	backend  *KindBackend
	clusters map[string]*Cluster // key: project:zone:name
}

func NewAPI(opMgr *orchestrator.OperationManager) *API {
	return &API{
		opMgr:    opMgr,
		backend:  NewKindBackend(),
		clusters: make(map[string]*Cluster),
	}
}

// GetBackend exposes the backend for dynamic dashboard configuration.
func (api *API) GetBackend() *KindBackend {
	return api.backend
}

// ServeHTTP dispatches GKE container.v1 paths.
//
// Supported paths (container.googleapis.com):
//   POST   /v1/projects/{project}/zones/{zone}/clusters
//   GET    /v1/projects/{project}/zones/{zone}/clusters
//   GET    /v1/projects/{project}/zones/{zone}/clusters/{cluster}
//   DELETE /v1/projects/{project}/zones/{zone}/clusters/{cluster}
//   GET    /v1/projects/{project}/zones/{zone}/operations/{operation}
//   (location-based paths /v1/projects/{project}/locations/{zone}/... also handled)
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: GKE] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	switch {
	case strings.Contains(path, "/operations/"):
		api.getOperation(w, r, path)
	case strings.Contains(path, "/clusters"):
		api.routeClusters(w, r, path)
	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "GKE resource not found: "+path)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Clusters
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeClusters(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	zone := firstOf(extractSegmentAfter(path, "zones"), extractSegmentAfter(path, "locations"))
	clusterName := extractSegmentAfter(path, "clusters")

	switch r.Method {
	case http.MethodPost:
		api.createCluster(w, r, project, zone)
	case http.MethodGet:
		if clusterName != "" {
			api.getCluster(w, project, zone, clusterName)
		} else {
			api.listClusters(w, project, zone)
		}
	case http.MethodDelete:
		api.deleteCluster(w, r, project, zone, clusterName)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createCluster(w http.ResponseWriter, r *http.Request, project, zone string) {
	var body struct {
		Cluster Cluster `json:"cluster"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}

	cl := body.Cluster
	name := cl.Name
	if name == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "cluster.name is required")
		return
	}

	if zone == "" {
		zone = "us-central1-c"
	}
	if cl.InitialNodeCount == 0 {
		cl.InitialNodeCount = 3
	}
	if cl.InitialNodeCount > 3 {
		log.Printf("[Shim: GKE] Clamping requested node count from %d to 3 maximum limit", cl.InitialNodeCount)
		cl.InitialNodeCount = 3
	}
	if cl.Network == "" {
		cl.Network = "default"
	}
	if cl.NodeConfig == nil {
		cl.NodeConfig = &NodeConfig{
			MachineType: "e2-medium",
			DiskSizeGb:  100,
			OauthScopes: []string{
				"https://www.googleapis.com/auth/cloud-platform",
			},
		}
	}

	cl.Zone = zone
	cl.Location = zone
	cl.Status = "PROVISIONING"
	cl.Endpoint = "127.0.0.1"
	cl.ClusterIpv4Cidr = "10.4.0.0/14"
	cl.ServicesIpv4Cidr = "10.8.0.0/20"
	cl.LoggingService = "logging.googleapis.com/kubernetes"
	cl.MonitoringService = "monitoring.googleapis.com/kubernetes"
	cl.InitialClusterVersion = "1.29.4-gke.100"
	cl.CurrentMasterVersion = "1.29.4-gke.100"
	cl.MasterAuth = &MasterAuth{
		ClusterCaCertificate: "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0t(minisky-fake)",
		ClientCertificate:    "",
		ClientKey:            "",
	}
	cl.SelfLink = fmt.Sprintf(
		"https://container.googleapis.com/v1/projects/%s/zones/%s/clusters/%s",
		project, zone, name)
	cl.CreateTime = time.Now().UTC().Format(time.RFC3339)

	key := clusterKey(project, zone, name)
	api.mu.Lock()
	api.clusters[key] = &cl
	api.mu.Unlock()

	targetLink := cl.SelfLink
	op := api.opMgr.Register("container#operation", "CREATE_CLUSTER", targetLink, zone, "")

	api.opMgr.RunAsync(op.Name, func() error {
		if api.backend.Enabled() {
			api.backend.CreateCluster(name)
		} else {
			// Simulate cluster provision time
			time.Sleep(5 * time.Second)
		}

		api.mu.Lock()
		if c, ok := api.clusters[key]; ok {
			c.Status = "RUNNING"
		}
		api.mu.Unlock()

		// Loopback execution to register nodes in Compute API
		go func() {
			var nodeNames []string
			kindBase := sanitizeKindName(name)
			nodeNames = append(nodeNames, kindBase+"-control-plane")
			for i := 1; i <= cl.InitialNodeCount; i++ {
				if i == 1 {
					nodeNames = append(nodeNames, kindBase+"-worker")
				} else {
					nodeNames = append(nodeNames, fmt.Sprintf("%s-worker%d", kindBase, i))
				}
			}

			for _, n := range nodeNames {
				body := map[string]interface{}{
					"name":        n,
					"machineType": cl.NodeConfig.MachineType,
					"description": "GKE Managed Node",
					"labels": map[string]string{
						"managed-by":  "gke",
						"gke-cluster": name,
					},
				}
				b, _ := json.Marshal(body)
				url := fmt.Sprintf("http://localhost:8080/compute/v1/projects/%s/zones/%s/instances", project, zone)
				req, _ := http.NewRequest("POST", url, strings.NewReader(string(b)))
				req.Host = "compute.googleapis.com"
				req.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					log.Printf("[Shim: GKE] failed loopback registration for node %s: %v", n, err)
				} else {
					resp.Body.Close()
				}
			}
		}()

		return nil
	})

	gkeOp := toGkeOperation(op, "CREATE_CLUSTER", project, zone, targetLink)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(gkeOp)
}

func (api *API) getCluster(w http.ResponseWriter, project, zone, name string) {
	key := clusterKey(project, zone, name)
	api.mu.RLock()
	cl, ok := api.clusters[key]
	api.mu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Cluster '%s' not found in zone '%s'", name, zone))
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(cl)
}

func (api *API) listClusters(w http.ResponseWriter, project, zone string) {
	prefix := project + ":" + zone + ":"
	api.mu.RLock()
	items := []*Cluster{}
	for k, v := range api.clusters {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"clusters": items,
	})
}

func (api *API) deleteCluster(w http.ResponseWriter, r *http.Request, project, zone, name string) {
	key := clusterKey(project, zone, name)
	api.mu.Lock()
	cl, ok := api.clusters[key]
	if !ok {
		api.mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Cluster '%s' not found", name))
		return
	}
	
	// Mark as STOPPING to simulate winding down in the UI
	cl.Status = "STOPPING"
	api.mu.Unlock()

	op := api.opMgr.Register("container#operation", "DELETE_CLUSTER", cl.SelfLink, zone, "")
	api.opMgr.RunAsync(op.Name, func() error {
		// Simulate winding down time
		time.Sleep(3 * time.Second)

		if api.backend.Enabled() {
			api.backend.DeleteCluster(name)
		}

		// Loopback execution to delete nodes in Compute API
		go func() {
			kindBase := sanitizeKindName(name)
			var nodeNames []string
			nodeNames = append(nodeNames, kindBase+"-control-plane")
			for i := 1; i <= cl.InitialNodeCount; i++ {
				if i == 1 {
					nodeNames = append(nodeNames, kindBase+"-worker")
				} else {
					nodeNames = append(nodeNames, fmt.Sprintf("%s-worker%d", kindBase, i))
				}
			}

			for _, n := range nodeNames {
				url := fmt.Sprintf("http://localhost:8080/compute/v1/projects/%s/zones/%s/instances/%s", project, zone, n)
				req, _ := http.NewRequest("DELETE", url, nil)
				req.Host = "compute.googleapis.com"
				req.Header.Set("X-Minisky-GKE-Bypass", "true")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					log.Printf("[Shim: GKE] failed loopback deletion for node %s: %v", n, err)
				} else {
					resp.Body.Close()
				}
			}
		}()

		// Finally remove from memory
		api.mu.Lock()
		delete(api.clusters, key)
		api.mu.Unlock()

		return nil
	})
	gkeOp := toGkeOperation(op, "DELETE_CLUSTER", project, zone, cl.SelfLink)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(gkeOp)
}

// ─────────────────────────────────────────────────────────────────────────────
// Operations
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) getOperation(w http.ResponseWriter, r *http.Request, path string) {
	project := extractSegmentAfter(path, "projects")
	zone := firstOf(extractSegmentAfter(path, "zones"), extractSegmentAfter(path, "locations"))
	opName := extractSegmentAfter(path, "operations")

	op := api.opMgr.Get(opName)
	if op == nil {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Operation not found: "+opName)
		return
	}
	gkeOp := toGkeOperation(op, op.OperationType, project, zone, op.TargetLink)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(gkeOp)
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

func toGkeOperation(op *orchestrator.Operation, opType, project, zone, targetLink string) *GkeOperation {
	status := "PENDING"
	switch op.Status {
	case orchestrator.StatusRunning:
		status = "RUNNING"
	case orchestrator.StatusDone:
		status = "DONE"
	}
	selfLink := fmt.Sprintf(
		"https://container.googleapis.com/v1/projects/%s/zones/%s/operations/%s",
		project, zone, op.Name)
	return &GkeOperation{
		Name:          op.Name,
		Zone:          zone,
		OperationType: opType,
		Status:        status,
		SelfLink:      selfLink,
		TargetLink:    targetLink,
		StartTime:     op.StartTime,
		EndTime:       op.EndTime,
	}
}

func clusterKey(project, zone, name string) string {
	return project + ":" + zone + ":" + name
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

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
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
