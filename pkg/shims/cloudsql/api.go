package cloudsql

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minisky/pkg/orchestrator"
)

// ─────────────────────────────────────────────────────────────────────────────
// Resource types
// ─────────────────────────────────────────────────────────────────────────────

// DatabaseInstance mirrors the Cloud SQL DatabaseInstance resource.
type DatabaseInstance struct {
	Kind            string          `json:"kind"`
	Name            string          `json:"name"`
	Project         string          `json:"project"`
	SelfLink        string          `json:"selfLink"`
	DatabaseVersion string          `json:"databaseVersion"` // e.g. POSTGRES_15, MYSQL_8_0
	Region          string          `json:"region"`
	State           string          `json:"state"` // PENDING_CREATE → RUNNABLE → SUSPENDED → DELETED
	Settings        InstanceSettings `json:"settings"`
	ConnectionName  string          `json:"connectionName"`
	IpAddresses     []IpMapping     `json:"ipAddresses"`
	ServerCaCert    *SslCert        `json:"serverCaCert,omitempty"`
	CreateTime      string          `json:"createTime,omitempty"`
	Etag            string          `json:"etag"`
}

type InstanceSettings struct {
	Tier             string            `json:"tier"`             // e.g. db-n1-standard-2
	ActivationPolicy string            `json:"activationPolicy"` // ALWAYS, NEVER
	BackupConfiguration *BackupConfig  `json:"backupConfiguration,omitempty"`
	DatabaseFlags    []DatabaseFlag    `json:"databaseFlags,omitempty"`
	UserLabels       map[string]string `json:"userLabels,omitempty"`
	StorageAutoResize bool             `json:"storageAutoResize"`
	DataDiskSizeGb   string           `json:"dataDiskSizeGb"`
	DataDiskType     string           `json:"dataDiskType"` // PD_SSD, PD_HDD
}

type BackupConfig struct {
	Enabled   bool   `json:"enabled"`
	StartTime string `json:"startTime"`
}

type DatabaseFlag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type IpMapping struct {
	Type      string `json:"type"`      // PRIMARY, OUTGOING
	IpAddress string `json:"ipAddress"`
}

type SslCert struct {
	Kind             string `json:"kind"`
	CertSerialNumber string `json:"certSerialNumber"`
	Cert             string `json:"cert"`
	CommonName       string `json:"commonName"`
	ExpirationTime   string `json:"expirationTime"`
	Sha1Fingerprint  string `json:"sha1Fingerprint"`
	Instance         string `json:"instance"`
}

// Database represents a schema within a Cloud SQL instance.
type Database struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Instance string `json:"instance"`
	Project  string `json:"project"`
	SelfLink string `json:"selfLink"`
	Charset  string `json:"charset"`
	Collation string `json:"collation"`
	Etag     string `json:"etag"`
}

// User represents a database user.
type User struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Instance string `json:"instance"`
	Project  string `json:"project"`
	Password string `json:"password,omitempty"`
}

// SqlOperation mirrors the Cloud SQL operations resource.
type SqlOperation struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	TargetLink    string `json:"targetLink"`
	Status        string `json:"status"` // PENDING, RUNNING, DONE
	OperationType string `json:"operationType"`
	StartTime     string `json:"startTime,omitempty"`
	EndTime       string `json:"endTime,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// API shim
// ─────────────────────────────────────────────────────────────────────────────

// API is the high-fidelity Cloud SQL (sqladmin v1) shim.
type API struct {
	mu        sync.RWMutex
	opMgr     *orchestrator.OperationManager
	svcMgr    *orchestrator.ServiceManager
	instances map[string]*DatabaseInstance // key: project:instanceName
	databases map[string][]*Database       // key: project:instanceName
	users     map[string][]*User           // key: project:instanceName
}

func NewAPI(opMgr *orchestrator.OperationManager, svcMgr *orchestrator.ServiceManager) *API {
	return &API{
		opMgr:     opMgr,
		svcMgr:    svcMgr,
		instances: make(map[string]*DatabaseInstance),
		databases: make(map[string][]*Database),
		users:     make(map[string][]*User),
	}
}

// ServeHTTP dispatches Cloud SQL v1 paths.
//
// Supported paths (sqladmin.googleapis.com):
//   POST   /v1/projects/{project}/instances
//   GET    /v1/projects/{project}/instances
//   GET    /v1/projects/{project}/instances/{instance}
//   DELETE /v1/projects/{project}/instances/{instance}
//   POST   /v1/projects/{project}/instances/{instance}/databases
//   GET    /v1/projects/{project}/instances/{instance}/databases
//   POST   /v1/projects/{project}/instances/{instance}/users
//   GET    /v1/projects/{project}/instances/{instance}/users
//   GET    /v1/projects/{project}/operations/{operation}
func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Cloud SQL] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path
	project := extractSegmentAfter(path, "projects")

	switch {
	case strings.Contains(path, "/operations/"):
		api.getOperation(w, r, path)
	case strings.Contains(path, "/databases"):
		instance := extractSegmentAfter(path, "instances")
		api.routeDatabases(w, r, project, instance, path)
	case strings.Contains(path, "/users"):
		instance := extractSegmentAfter(path, "instances")
		api.routeUsers(w, r, project, instance)
	case strings.Contains(path, "/instances"):
		api.routeInstances(w, r, project, path)
	default:
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Cloud SQL resource not found: "+path)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Instances
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeInstances(w http.ResponseWriter, r *http.Request, project, path string) {
	instanceName := extractSegmentAfter(path, "instances")

	switch r.Method {
	case http.MethodPost:
		api.createInstance(w, r, project)
	case http.MethodGet:
		if instanceName != "" {
			api.getInstance(w, project, instanceName)
		} else {
			api.listInstances(w, project)
		}
	case http.MethodDelete:
		api.deleteInstance(w, r, project, instanceName)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createInstance(w http.ResponseWriter, r *http.Request, project string) {
	var body struct {
		Name            string          `json:"name"`
		DatabaseVersion string          `json:"databaseVersion"`
		Region          string          `json:"region"`
		Settings        InstanceSettings `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
		return
	}
	if body.Name == "" {
		w.WriteHeader(http.StatusBadRequest)
		writeError(w, 400, "INVALID_ARGUMENT", "Field 'name' is required")
		return
	}

	region := body.Region
	if region == "" {
		region = "us-central1"
	}
	dbVersion := body.DatabaseVersion
	if dbVersion == "" {
		dbVersion = "POSTGRES_15"
	}

	// Fill in opinionated defaults for missing settings
	settings := body.Settings
	if settings.Tier == "" {
		settings.Tier = "db-n1-standard-2"
	}
	if settings.ActivationPolicy == "" {
		settings.ActivationPolicy = "ALWAYS"
	}
	if settings.DataDiskSizeGb == "" {
		settings.DataDiskSizeGb = "10"
	}
	if settings.DataDiskType == "" {
		settings.DataDiskType = "PD_SSD"
	}

	selfLink := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/instances/%s", project, body.Name)
	inst := &DatabaseInstance{
		Kind:            "sql#instance",
		Name:            body.Name,
		Project:         project,
		SelfLink:        selfLink,
		DatabaseVersion: dbVersion,
		Region:          region,
		State:           "PENDING_CREATE",
		Settings:        settings,
		ConnectionName:  fmt.Sprintf("%s:%s:%s", project, region, body.Name),
		IpAddresses: []IpMapping{
			{Type: "PRIMARY", IpAddress: "127.0.0.1"},
			{Type: "OUTGOING", IpAddress: "127.0.0.1"},
		},
		ServerCaCert: &SslCert{
			Kind:             "sql#sslCert",
			CertSerialNumber: "0",
			Cert:             "-----BEGIN CERTIFICATE-----\n(minisky-fake-ca-cert)\n-----END CERTIFICATE-----\n",
			CommonName:       "minisky-local-ca",
			ExpirationTime:   time.Now().Add(87600 * time.Hour).UTC().Format(time.RFC3339),
			Sha1Fingerprint:  fmt.Sprintf("%x", time.Now().UnixNano()),
			Instance:         body.Name,
		},
		CreateTime: time.Now().UTC().Format(time.RFC3339),
		Etag:       newEtag(),
	}

	iKey := instanceKey(project, body.Name)
	api.mu.Lock()
	api.instances[iKey] = inst
	api.mu.Unlock()

	// Register LRO and drive state transitions asynchronously
	targetLink := selfLink
	op := api.opMgr.Register("sql#operation", "CREATE", targetLink, "", region)

	api.opMgr.RunAsync(op.Name, func() error {
		// Provision physical Docker container with standard "minisky" root password
		internalURL, err := api.svcMgr.ProvisionCloudSQLVM(body.Name, dbVersion, "minisky")
		if err != nil {
			log.Printf("[Shim: Cloud SQL] Provisioning failed: %v", err)
			return err
		}

		api.mu.Lock()
		if i, ok := api.instances[iKey]; ok {
			i.State = "RUNNABLE"
			// Extract ip:port from 'http://127.0.0.1:xxx'
			addr := strings.TrimPrefix(internalURL, "http://")
			i.IpAddresses = []IpMapping{
				{Type: "PRIMARY", IpAddress: addr},
			}
		}
		api.mu.Unlock()
		return nil
	})

	// Wrap the operation in Cloud SQL's own schema format
	sqlOp := toSqlOperation(op, "CREATE", selfLink)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(sqlOp)
}

func (api *API) getInstance(w http.ResponseWriter, project, name string) {
	key := instanceKey(project, name)
	api.mu.RLock()
	inst, ok := api.instances[key]
	api.mu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Instance '%s' not found in project '%s'", name, project))
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(inst)
}

func (api *API) listInstances(w http.ResponseWriter, project string) {
	prefix := project + ":"
	api.mu.RLock()
	items := []*DatabaseInstance{}
	for k, v := range api.instances {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"kind":  "sql#instancesList",
		"items": items,
	})
}

func (api *API) deleteInstance(w http.ResponseWriter, r *http.Request, project, name string) {
	key := instanceKey(project, name)
	api.mu.Lock()
	inst, ok := api.instances[key]
	if ok {
		inst.State = "DELETED"
		delete(api.instances, key)
		delete(api.databases, key)
		delete(api.users, key)
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", fmt.Sprintf("Instance '%s' not found", name))
		return
	}

	selfLink := fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/instances/%s", project, name)
	op := api.opMgr.Register("sql#operation", "DELETE", selfLink, "", "")
	api.opMgr.RunAsync(op.Name, func() error {
		api.svcMgr.DeleteCloudSQLVM(name)
		return nil
	})
	sqlOp := toSqlOperation(op, "DELETE", selfLink)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(sqlOp)
}

// ─────────────────────────────────────────────────────────────────────────────
// Databases
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeDatabases(w http.ResponseWriter, r *http.Request, project, instance, path string) {
	dbName := extractSegmentAfter(path, "databases")

	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name      string `json:"name"`
			Charset   string `json:"charset"`
			Collation string `json:"collation"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
			return
		}
		charset := body.Charset
		if charset == "" {
			charset = "UTF8"
		}
		collation := body.Collation
		if collation == "" {
			collation = "en_US.UTF8"
		}
		db := &Database{
			Kind:      "sql#database",
			Name:      body.Name,
			Instance:  instance,
			Project:   project,
			SelfLink:  fmt.Sprintf("https://sqladmin.googleapis.com/v1/projects/%s/instances/%s/databases/%s", project, instance, body.Name),
			Charset:   charset,
			Collation: collation,
			Etag:      newEtag(),
		}
		iKey := instanceKey(project, instance)
		api.mu.Lock()
		api.databases[iKey] = append(api.databases[iKey], db)
		api.mu.Unlock()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"kind":      "sql#operation",
			"status":    "DONE",
			"operationType": "CREATE_DATABASE",
			"targetLink": db.SelfLink,
		})

	case http.MethodGet:
		iKey := instanceKey(project, instance)
		api.mu.RLock()
		dbs := api.databases[iKey]
		api.mu.RUnlock()

		if dbName != "" {
			for _, d := range dbs {
				if d.Name == dbName {
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(d)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			writeError(w, 404, "NOT_FOUND", "Database "+dbName+" not found")
			return
		}

		if dbs == nil {
			dbs = []*Database{}
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"kind":  "sql#databasesList",
			"items": dbs,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Users
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeUsers(w http.ResponseWriter, r *http.Request, project, instance string) {
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Name     string `json:"name"`
			Host     string `json:"host"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			writeError(w, 400, "INVALID_ARGUMENT", "Parse error: "+err.Error())
			return
		}
		user := &User{
			Kind:     "sql#user",
			Name:     body.Name,
			Host:     body.Host,
			Instance: instance,
			Project:  project,
		}
		iKey := instanceKey(project, instance)
		api.mu.Lock()
		api.users[iKey] = append(api.users[iKey], user)
		api.mu.Unlock()

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"kind":      "sql#operation",
			"status":    "DONE",
			"operationType": "CREATE_USER",
		})

	case http.MethodGet:
		iKey := instanceKey(project, instance)
		api.mu.RLock()
		users := api.users[iKey]
		api.mu.RUnlock()

		if users == nil {
			users = []*User{}
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"kind":  "sql#usersList",
			"items": users,
		})

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Operations (Cloud SQL uses its own operation schema)
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) getOperation(w http.ResponseWriter, r *http.Request, path string) {
	opName := extractSegmentAfter(path, "operations")
	op := api.opMgr.Get(opName)
	if op == nil {
		w.WriteHeader(http.StatusNotFound)
		writeError(w, 404, "NOT_FOUND", "Operation not found: "+opName)
		return
	}
	sqlOp := toSqlOperation(op, op.OperationType, op.TargetLink)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(sqlOp)
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

func toSqlOperation(op *orchestrator.Operation, opType, targetLink string) *SqlOperation {
	status := "PENDING"
	switch op.Status {
	case orchestrator.StatusRunning:
		status = "RUNNING"
	case orchestrator.StatusDone:
		status = "DONE"
	}
	return &SqlOperation{
		Kind:          "sql#operation",
		Name:          op.Name,
		TargetLink:    targetLink,
		Status:        status,
		OperationType: opType,
		StartTime:     op.StartTime,
		EndTime:       op.EndTime,
	}
}

func instanceKey(project, name string) string {
	return project + ":" + name
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

func writeError(w http.ResponseWriter, code int, status, message string) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"status":  status,
			"message": message,
		},
	})
}

func newEtag() string {
	return fmt.Sprintf("SQLETAG%x", time.Now().UnixNano())
}
