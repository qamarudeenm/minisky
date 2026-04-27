package memorystore

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minisky/pkg/config"
	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
	"minisky/pkg/shims/logging"
)

func init() {
	f := func(ctx *registry.Context) http.Handler {
		// Memorystore emits structured logs into Cloud Logging
		var logAPI *logging.API
		if l, ok := ctx.GetShim("logging.googleapis.com").(*logging.API); ok {
			logAPI = l
		}
		return NewAPI(ctx.OpMgr, ctx.SvcMgr, logAPI)
	}
	registry.Register("redis.googleapis.com", f)
	registry.Register("memcache.googleapis.com", f)
}

type Instance struct {
	Name         string            `json:"name"`
	DisplayName  string            `json:"displayName,omitempty"`
	Labels       map[string]string `json:"labels,omitempty"`
	Tier         string            `json:"tier"` // BASIC, STANDARD_HA
	MemorySizeGb int               `json:"memorySizeGb"`
	Host         string            `json:"host"`
	Port         int               `json:"port"`
	State        string            `json:"state"` // CREATING, READY, DELETING, REPAIRING, MAINTENANCE
	CreateTime   string            `json:"createTime"`
	LocationId   string            `json:"locationId"`
	AlternativeLocationId string    `json:"alternativeLocationId,omitempty"`
	AuthorizedNetwork     string    `json:"authorizedNetwork,omitempty"`
	PersistenceConfig     *PersistenceConfig `json:"persistenceConfig,omitempty"`
	EngineVersion         string    `json:"engineVersion,omitempty"` // REDIS_6_X, MEMCACHED_1_5, etc.
}

type PersistenceConfig struct {
	PersistenceMode    string `json:"persistenceMode"` // DISABLED, RDB
	RdbSnapshotPeriod  string `json:"rdbSnapshotPeriod,omitempty"`
}

type API struct {
	mu       sync.RWMutex
	opMgr    *orchestrator.OperationManager
	svcMgr   *orchestrator.ServiceManager
	logAPI   *logging.API
	redisInstances map[string]*Instance
	memcacheInstances map[string]*Instance
}

func NewAPI(opMgr *orchestrator.OperationManager, sm *orchestrator.ServiceManager, logAPI *logging.API) *API {
	return &API{
		opMgr:             opMgr,
		svcMgr:            sm,
		logAPI:            logAPI,
		redisInstances:    make(map[string]*Instance),
		memcacheInstances: make(map[string]*Instance),
	}
}

func (api *API) pushLog(severity, service, text string) {
	if api.logAPI == nil {
		return
	}
	api.logAPI.PushLog(severity, "memorystore_instance", service, text)
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Memorystore] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	// Routing for Redis and Memcached
	// Paths typically look like: /v1/projects/{project}/locations/{location}/instances
	
	isRedis := strings.Contains(r.Host, "redis")
	
	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/instances") {
		api.handleListInstances(w, r, isRedis)
		return
	}

	if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/instances") {
		api.handleCreateInstance(w, r, isRedis)
		return
	}

	if r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/instances/") {
		api.handleDeleteInstance(w, r, isRedis)
		return
	}

	if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/instances/") {
		api.handleGetInstance(w, r, isRedis)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (api *API) handleListInstances(w http.ResponseWriter, r *http.Request, isRedis bool) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	var instances []*Instance
	source := api.redisInstances
	if !isRedis {
		source = api.memcacheInstances
	}

	for _, inst := range source {
		instances = append(instances, inst)
	}

	res := map[string]interface{}{
		"instances": instances,
	}
	json.NewEncoder(w).Encode(res)
}

func (api *API) handleGetInstance(w http.ResponseWriter, r *http.Request, isRedis bool) {
	instanceId := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	
	api.mu.RLock()
	defer api.mu.RUnlock()

	var inst *Instance
	var ok bool
	if isRedis {
		inst, ok = api.redisInstances[instanceId]
	} else {
		inst, ok = api.memcacheInstances[instanceId]
	}

	if !ok {
		http.Error(w, "Instance not found", http.StatusNotFound)
		return
	}

	json.NewEncoder(w).Encode(inst)
}

func (api *API) handleCreateInstance(w http.ResponseWriter, r *http.Request, isRedis bool) {
	var req struct {
		InstanceId string `json:"instanceId"`
		Instance   Instance `json:"instance"`
	}
	// Google Cloud API sometimes passes instanceId as a query param
	req.InstanceId = r.URL.Query().Get("instanceId")

	if err := json.NewDecoder(r.Body).Decode(&req.Instance); err != nil {
		// If decoding the whole wrapper fails, try decoding just the instance
		// (Depends on the SDK version/client)
	}

	if req.InstanceId == "" {
		// Expecting .../instances?instanceId=...
		// But some clients might put it in the body
	}

	// For simplicity in the shim, we'll assume a project-wide unique ID
	id := req.InstanceId
	if id == "" {
		id = fmt.Sprintf("inst-%d", time.Now().Unix())
	}

	req.Instance.Name = r.URL.Path + "/" + id
	req.Instance.State = "CREATING"
	req.Instance.CreateTime = time.Now().Format(time.RFC3339)
	req.Instance.Host = "127.0.0.1"
	req.Instance.Port = 6379
	if !isRedis {
		req.Instance.Port = 11211
	}

	api.mu.Lock()
	if isRedis {
		api.redisInstances[id] = &req.Instance
	} else {
		api.memcacheInstances[id] = &req.Instance
	}
	api.mu.Unlock()

	op := api.opMgr.Register("memorystore#operation", "CREATE", req.Instance.Name, "", "us-central1")
	api.pushLog("INFO", id, fmt.Sprintf("Creating Memorystore instance %s (Tier: %s)", id, req.Instance.Tier))

	api.opMgr.RunAsync(op.Name, func() error {
		reg := config.GetImageRegistry()
		image := reg.Memorystore.Redis.DefaultImage
		containerPrefix := "redis"

		// Version mapping
		v := req.Instance.EngineVersion
		if strings.HasPrefix(v, "REDIS") {
			vparts := strings.Split(v, "_")
			if len(vparts) > 1 {
				targetV := vparts[1]
				if len(vparts) > 2 { targetV += "." + vparts[2] }
				for _, mv := range reg.Memorystore.Redis.Versions {
					if strings.HasPrefix(mv.Version, targetV) {
						image = mv.Image
						break
					}
				}
			}
		} else if strings.HasPrefix(v, "VALKEY") {
			image = reg.Memorystore.Valkey.DefaultImage
			containerPrefix = "valkey"
		} else if strings.HasPrefix(v, "MEMCACHED") {
			image = reg.Memorystore.Memcached.DefaultImage
			containerPrefix = "memcache"
		}
		
		containerName := fmt.Sprintf("minisky-%s-%s", containerPrefix, id)
		err := api.svcMgr.ProvisionComputeVM(containerName, image, "default", []string{}, []string{}, nil)
		
		api.mu.Lock()
		if err != nil {
			req.Instance.State = "REPAIRING"
			api.pushLog("ERROR", id, fmt.Sprintf("Failed to provision container: %v", err))
		} else {
			req.Instance.State = "READY"
			api.pushLog("INFO", id, fmt.Sprintf("Instance %s is now READY", id))
		}
		api.mu.Unlock()
		return err
	})

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(op)
}

func (api *API) handleDeleteInstance(w http.ResponseWriter, r *http.Request, isRedis bool) {
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

	api.mu.Lock()
	var inst *Instance
	var ok bool
	if isRedis {
		inst, ok = api.redisInstances[id]
	} else {
		inst, ok = api.memcacheInstances[id]
	}
	if ok {
		inst.State = "DELETING"
	}
	api.mu.Unlock()

	if !ok {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	op := api.opMgr.Register("memorystore#operation", "DELETE", inst.Name, "", "us-central1")
	api.pushLog("INFO", id, fmt.Sprintf("Deleting Memorystore instance %s", id))

	api.opMgr.RunAsync(op.Name, func() error {
		containerPrefix := "redis"
		if !isRedis {
			containerPrefix = "memcache"
		}
		containerName := fmt.Sprintf("minisky-%s-%s", containerPrefix, id)
		api.svcMgr.DeleteComputeVM(containerName)
		
		api.mu.Lock()
		if isRedis {
			delete(api.redisInstances, id)
		} else {
			delete(api.memcacheInstances, id)
		}
		api.mu.Unlock()
		return nil
	})

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(op)
}
