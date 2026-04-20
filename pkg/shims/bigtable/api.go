package bigtable

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	f := func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.OpMgr, ctx.SvcMgr)
	}
	registry.Register("bigtableadmin.googleapis.com", f)
	registry.Register("bigtable.googleapis.com", f)
}

// Instance mirrors the Bigtable Instance resource.
type Instance struct {
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName"`
	State       string            `json:"state"` // READY, CREATING
	Type        string            `json:"type"`  // PRODUCTION, DEVELOPMENT
	Labels      map[string]string `json:"labels"`
}

// Table mirrors the Bigtable Table resource.
type Table struct {
	Name           string                     `json:"name"`
	ColumnFamilies map[string]ColumnFamily    `json:"columnFamilies"`
	Granularity    string                     `json:"granularity"` // MILLIS
}

type ColumnFamily struct {
	GcRule *GcRule `json:"gcRule,omitempty"`
}

type GcRule struct {
	MaxAge         string  `json:"maxAge,omitempty"`
	MaxNumVersions int32   `json:"maxNumVersions,omitempty"`
}

// API is the high-fidelity Bigtable Admin shim.
type API struct {
	mu        sync.RWMutex
	opMgr     *orchestrator.OperationManager
	svcMgr    *orchestrator.ServiceManager
	instances map[string]*Instance // key: projects/{p}/instances/{i}
	tables    map[string]*Table    // key: projects/{p}/instances/{i}/tables/{t}
}

func NewAPI(opMgr *orchestrator.OperationManager, svcMgr *orchestrator.ServiceManager) *API {
	return &API{
		opMgr:     opMgr,
		svcMgr:    svcMgr,
		instances: make(map[string]*Instance),
		tables:    make(map[string]*Table),
	}
}

// ServeHTTP dispatches Bigtable Admin paths.
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Bigtable] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	switch {
	case strings.Contains(path, "/instances") && !strings.Contains(path, "/tables"):
		api.routeInstances(w, r, path)
	case strings.Contains(path, "/tables"):
		api.routeTables(w, r, path)
	case strings.Contains(path, "/clusters"):
		api.routeClusters(w, r, path)
	default:
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"code": 404, "message": "Bigtable resource not found: " + path},
		})
	}
}

func (api *API) routeInstances(w http.ResponseWriter, r *http.Request, path string) {
	// Path: /v2/projects/{project}/instances[/{instance}]
	parts := strings.Split(strings.Trim(path, "/"), "/")
	project := ""
	instanceId := ""
	if len(parts) >= 3 { project = parts[2] }
	if len(parts) >= 5 { instanceId = parts[4] }

	switch r.Method {
	case http.MethodPost:
		var body struct {
			InstanceId string   `json:"instanceId"`
			Instance   Instance `json:"instance"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		
		name := fmt.Sprintf("projects/%s/instances/%s", project, body.InstanceId)
		inst := &Instance{
			Name:        name,
			DisplayName: body.Instance.DisplayName,
			State:       "READY",
			Type:        body.Instance.Type,
			Labels:      body.Instance.Labels,
		}
		
		api.mu.Lock()
		api.instances[name] = inst
		api.mu.Unlock()

		// Trigger Emulator Provisioning if this is the first instance
		go func() {
			api.svcMgr.EnsureServiceRunning(r.Context(), "bigtable.googleapis.com")
		}()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(inst)

	case http.MethodGet:
		if instanceId != "" {
			name := fmt.Sprintf("projects/%s/instances/%s", project, instanceId)
			api.mu.RLock()
			inst, ok := api.instances[name]
			api.mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(inst)
		} else {
			api.mu.RLock()
			items := []*Instance{}
			for _, v := range api.instances {
				if strings.Contains(v.Name, "projects/"+project) {
					items = append(items, v)
				}
			}
			api.mu.RUnlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"instances": items})
		}

	case http.MethodDelete:
		name := fmt.Sprintf("projects/%s/instances/%s", project, instanceId)
		api.mu.Lock()
		delete(api.instances, name)
		api.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) routeTables(w http.ResponseWriter, r *http.Request, path string) {
	// Path: /v2/projects/{p}/instances/{i}/tables[/{t}]
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	project := parts[2]
	instance := parts[4]
	tableId := ""
	if len(parts) >= 7 { tableId = parts[6] }

	parent := fmt.Sprintf("projects/%s/instances/%s", project, instance)

	switch r.Method {
	case http.MethodPost:
		var body struct {
			TableId string `json:"tableId"`
			Table   Table  `json:"table"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		
		name := fmt.Sprintf("%s/tables/%s", parent, body.TableId)
		t := &Table{
			Name:           name,
			ColumnFamilies: body.Table.ColumnFamilies,
			Granularity:    "MILLIS",
		}
		if t.ColumnFamilies == nil {
			t.ColumnFamilies = make(map[string]ColumnFamily)
		}
		
		api.mu.Lock()
		api.tables[name] = t
		api.mu.Unlock()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(t)

	case http.MethodGet:
		if tableId != "" {
			name := fmt.Sprintf("%s/tables/%s", parent, tableId)
			api.mu.RLock()
			t, ok := api.tables[name]
			api.mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(t)
		} else {
			api.mu.RLock()
			items := []*Table{}
			for _, v := range api.tables {
				if strings.HasPrefix(v.Name, parent+"/tables") {
					items = append(items, v)
				}
			}
			api.mu.RUnlock()
			json.NewEncoder(w).Encode(map[string]interface{}{"tables": items})
		}

	case http.MethodDelete:
		name := fmt.Sprintf("%s/tables/%s", parent, tableId)
		api.mu.Lock()
		delete(api.tables, name)
		api.mu.Unlock()
		w.WriteHeader(http.StatusOK)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) routeClusters(w http.ResponseWriter, r *http.Request, path string) {
	// Basic list support to satisfy discovery/SDKs
	json.NewEncoder(w).Encode(map[string]interface{}{"clusters": []interface{}{}})
}
