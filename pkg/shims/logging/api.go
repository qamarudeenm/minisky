package logging

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("logging.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI()
	})
}

type LogEntry struct {
	InsertId    string            `json:"insertId"`
	Timestamp   string            `json:"timestamp"`
	Severity    string            `json:"severity"`
	TextPayload string            `json:"textPayload,omitempty"`
	JsonPayload interface{}       `json:"jsonPayload,omitempty"`
	Resource    *MonitoredResource `json:"resource"`
	LogName     string            `json:"logName"`
}

type MonitoredResource struct {
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels"`
}

type API struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
}

func NewAPI() *API {
	api := &API{
		entries: make([]LogEntry, 0),
		maxSize: 5000,
	}
	api.load()
	return api
}

func (api *API) load() {
	f, err := os.Open(".minisky/cloud_logs.json")
	if err != nil {
		return
	}
	defer f.Close()
	json.NewDecoder(f).Decode(&api.entries)
	log.Printf("[Logging] Loaded %d entries from persistence.", len(api.entries))
}

func (api *API) save() {
	data, _ := json.Marshal(api.entries)
	os.WriteFile(".minisky/cloud_logs.json", data, 0644)
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Logging] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/entries:write") {
		api.handleWrite(w, r)
		return
	}

	if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/entries:list") {
		api.handleList(w, r)
		return
	}

	// Internal Dashboard API
	if strings.HasPrefix(r.URL.Path, "/v1/internal/logs") {
		api.handleInternalLogs(w, r)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (api *API) handleWrite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Entries []LogEntry `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	api.mu.Lock()
	defer api.mu.Unlock()

	for _, entry := range body.Entries {
		if entry.Timestamp == "" {
			entry.Timestamp = time.Now().Format(time.RFC3339)
		}
		if entry.InsertId == "" {
			entry.InsertId = fmt.Sprintf("%d", time.Now().UnixNano())
		}
		api.entries = append(api.entries, entry)
	}

	// Truncate if too large
	if len(api.entries) > api.maxSize {
		api.entries = api.entries[len(api.entries)-api.maxSize:]
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
	go api.save()
}

func (api *API) handleList(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	res := map[string]interface{}{
		"entries": api.entries,
	}
	json.NewEncoder(w).Encode(res)
}

func (api *API) handleInternalLogs(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	json.NewEncoder(w).Encode(api.entries)
}

// PushLog is a helper for internal shims to log directly
func (api *API) PushLog(projectId, severity, resourceType, resourceName, text string) {
	api.mu.Lock()
	defer api.mu.Unlock()

	if projectId == "" {
		projectId = "default-project"
	}

	entry := LogEntry{
		InsertId:  fmt.Sprintf("%d", time.Now().UnixNano()),
		Timestamp: time.Now().Format(time.RFC3339),
		Severity:  severity,
		TextPayload: text,
		LogName:   fmt.Sprintf("projects/%s/logs/%s", projectId, resourceType),
		Resource: &MonitoredResource{
			Type: resourceType,
			Labels: map[string]string{
				"name": resourceName,
			},
		},
	}
	api.entries = append(api.entries, entry)
	if len(api.entries) > api.maxSize {
		api.entries = api.entries[len(api.entries)-api.maxSize:]
	}
	go api.save()
}

// GetEntries returns a snapshot of all log entries.
func (api *API) GetEntries() []LogEntry {
	api.mu.RLock()
	defer api.mu.RUnlock()
	out := make([]LogEntry, len(api.entries))
	copy(out, api.entries)
	return out
}

// Reset clears all log entries.
func (api *API) Reset() {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.entries = make([]LogEntry, 0)
	go api.save()
}

// StartHarvester begins a background loop that tails logs from all minisky-serverless containers.
func (api *API) OnPostBoot(ctx *registry.Context) {
	api.StartHarvester(ctx.SvcMgr)
}

func (api *API) StartHarvester(sm *orchestrator.ServiceManager) {
	log.Printf("[Logging] 🚜 Starting Background Log Harvester...")
	
	// Track the last timestamp we saw for each container to avoid duplicates
	lastSeen := make(map[string]int64)

	go func() {
		for {
			containers := sm.ListManagedContainers()
			for _, c := range containers {
				// We no longer skip Exited containers so we can get their crash logs.

				since := lastSeen[c.Name]
				if since == 0 {
					// First time seeing this container, fetch the last hour to catch up
					since = time.Now().Add(-1 * time.Hour).Unix()
				}

				// Fetch logs for this container
				logs, _ := sm.GetContainerLogsSince(c.Name, since)
				
				// Update last seen to now
				lastSeen[c.Name] = time.Now().Unix()

				if logs == "" {
					continue
				}

				lines := strings.Split(logs, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" || line == "Log source not found." {
						continue
					}

					// Docker timestamps format: "2026-04-19T20:15:45.123456789Z Some log message"
					parts := strings.SplitN(line, " ", 2)
					msg := line
					ts := time.Now().Format(time.RFC3339)
					if len(parts) == 2 {
						if _, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
							ts = parts[0]
							msg = parts[1]
						}
					}

					// Push to central store
					severity := "INFO"
					upper := strings.ToUpper(msg)
					if strings.Contains(upper, "ERROR") || strings.Contains(upper, "FAILED") {
						severity = "ERROR"
					}
					
					resourceType := "container"
					resourceName := strings.TrimPrefix(c.Name, "minisky-")
					if strings.HasPrefix(resourceName, "serverless-") {
						resourceType = "cloud_function"
						resourceName = strings.TrimPrefix(resourceName, "serverless-")
					} else if strings.HasPrefix(resourceName, "compute-") {
						resourceType = "gce_instance"
						resourceName = strings.TrimPrefix(resourceName, "compute-")
					} else if strings.HasPrefix(resourceName, "sql-") {
						resourceType = "cloudsql_instance"
						resourceName = strings.TrimPrefix(resourceName, "sql-")
					} else if strings.HasPrefix(resourceName, "firebase-") {
						resourceType = "firebase_service"
						resourceName = strings.TrimPrefix(resourceName, "firebase-")
					} else if strings.HasPrefix(resourceName, "appengine-") {
						resourceType = "gae_app"
						resourceName = strings.TrimPrefix(resourceName, "appengine-")
					}

					project := "default-project"
					// If the container name contains a project hint, use it
					// e.g. minisky-serverless-my-proj-my-fn
					// For now we just use default-project as a safe fallback for harvesters
					
					api.mu.Lock()
					entry := LogEntry{
						InsertId:    fmt.Sprintf("%d", time.Now().UnixNano()),
						Timestamp:   ts,
						Severity:    severity,
						TextPayload: msg,
						LogName:     fmt.Sprintf("projects/%s/logs/%s", project, resourceType),
						Resource: &MonitoredResource{
							Type: resourceType,
							Labels: map[string]string{
								"name": resourceName,
							},
						},
					}
					api.entries = append(api.entries, entry)
					if len(api.entries) > api.maxSize {
						api.entries = api.entries[len(api.entries)-api.maxSize:]
					}
					api.mu.Unlock()
				}
				go api.save()
			}
			time.Sleep(3 * time.Second)
		}
	}()
}
