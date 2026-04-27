package scheduler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"minisky/pkg/registry"
	"minisky/pkg/shims/logging"
)

func init() {
	registry.Register("cloudscheduler.googleapis.com", func(ctx *registry.Context) http.Handler {
		var logAPI *logging.API
		if l, ok := ctx.GetShim("logging.googleapis.com").(*logging.API); ok {
			logAPI = l
		}
		return NewAPI(logAPI)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Resource types
// ─────────────────────────────────────────────────────────────────────────────

type Job struct {
	Name            string           `json:"name"`
	Description     string           `json:"description,omitempty"`
	Target          *Target          `json:"target,omitempty"` // One of httpTarget, pubsubTarget, appEngineHttpTarget
	HttpTarget      *HttpTarget      `json:"httpTarget,omitempty"`
	PubsubTarget    *PubsubTarget    `json:"pubsubTarget,omitempty"`
	AppEngineTarget *AppEngineTarget `json:"appEngineHttpTarget,omitempty"`
	Schedule        string           `json:"schedule"`
	TimeZone        string           `json:"timeZone,omitempty"`
	State           string           `json:"state"` // ENABLED, PAUSED, DISABLED
	Status          *Status          `json:"status,omitempty"`
	LastAttemptTime string           `json:"lastAttemptTime,omitempty"`
	NextRunTime     string           `json:"nextRunTime,omitempty"`
}

type Target struct{}

type HttpTarget struct {
	Uri        string            `json:"uri"`
	HttpMethod string            `json:"httpMethod"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
}

type PubsubTarget struct {
	TopicName  string            `json:"topicName"`
	Data       string            `json:"data"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type AppEngineTarget struct {
	RelativeUri string            `json:"relativeUri"`
	HttpMethod  string            `json:"httpMethod"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
}

type Status struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ─────────────────────────────────────────────────────────────────────────────
// API shim
// ─────────────────────────────────────────────────────────────────────────────

type API struct {
	mu     sync.RWMutex
	jobs   map[string]*Job // key: projects/{p}/locations/{l}/jobs/{j}
	cron   *cron.Cron
	cronIDs map[string]cron.EntryID
	logAPI *logging.API
}

func NewAPI(logAPI *logging.API) *API {
	api := &API{
		jobs:    make(map[string]*Job),
		cron:    cron.New(),
		cronIDs: make(map[string]cron.EntryID),
		logAPI:  logAPI,
	}
	api.cron.Start()
	return api
}

func (api *API) pushLog(severity, jobId, text string) {
	if api.logAPI == nil {
		return
	}
	api.logAPI.PushLog(severity, "cloud_scheduler_job", jobId, text)
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Cloud Scheduler] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	path := r.URL.Path

	// Job verbs (run, pause, resume)
	switch {
	case strings.HasSuffix(path, ":run"):
		api.runJob(w, r, strings.TrimSuffix(path, ":run"))
		return
	case strings.HasSuffix(path, ":pause"):
		api.pauseJob(w, r, strings.TrimSuffix(path, ":pause"))
		return
	case strings.HasSuffix(path, ":resume"):
		api.resumeJob(w, r, strings.TrimSuffix(path, ":resume"))
		return
	}

	if strings.Contains(path, "/jobs") {
		api.routeJobs(w, r, path)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (api *API) routeJobs(w http.ResponseWriter, r *http.Request, path string) {
	jobName := extractJobName(path)

	switch r.Method {
	case http.MethodPost:
		api.createJob(w, r, path)
	case http.MethodGet:
		if jobName != "" {
			api.getJob(w, jobName)
		} else {
			api.listJobs(w, r, path)
		}
	case http.MethodDelete:
		api.deleteJob(w, jobName)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) createJob(w http.ResponseWriter, r *http.Request, path string) {
	var job Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// In GCP, Name is usually provided in the body or generated
	// If it's a relative path, we prefix it
	if !strings.HasPrefix(job.Name, "projects/") {
		job.Name = strings.TrimSuffix(path, "/") + "/" + job.Name
	}

	job.State = "ENABLED"
	job.Status = &Status{Code: 0, Message: "Job created"}

	api.mu.Lock()
	api.jobs[job.Name] = &job
	api.scheduleJobLocked(&job)
	api.mu.Unlock()

	api.pushLog("INFO", job.Name, "Job created: "+job.Schedule)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

func (api *API) getJob(w http.ResponseWriter, name string) {
	api.mu.RLock()
	job, ok := api.jobs[name]
	api.mu.RUnlock()

	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(job)
}

func (api *API) listJobs(w http.ResponseWriter, r *http.Request, path string) {
	prefix := strings.TrimSuffix(path, "/jobs") + "/jobs/"
	api.mu.RLock()
	var items []*Job
	for k, v := range api.jobs {
		if strings.HasPrefix(k, prefix) {
			items = append(items, v)
		}
	}
	api.mu.RUnlock()

	json.NewEncoder(w).Encode(map[string]interface{}{
		"jobs": items,
	})
}

func (api *API) deleteJob(w http.ResponseWriter, name string) {
	api.mu.Lock()
	if id, ok := api.cronIDs[name]; ok {
		api.cron.Remove(id)
		delete(api.cronIDs, name)
	}
	delete(api.jobs, name)
	api.mu.Unlock()

	api.pushLog("INFO", name, "Job deleted")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{})
}

func (api *API) runJob(w http.ResponseWriter, r *http.Request, name string) {
	api.mu.RLock()
	job, ok := api.jobs[name]
	api.mu.RUnlock()

	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	go api.executeJob(job)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(job)
}

func (api *API) pauseJob(w http.ResponseWriter, r *http.Request, name string) {
	api.mu.Lock()
	if job, ok := api.jobs[name]; ok {
		job.State = "PAUSED"
		if id, ok := api.cronIDs[name]; ok {
			api.cron.Remove(id)
			delete(api.cronIDs, name)
		}
	}
	api.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (api *API) resumeJob(w http.ResponseWriter, r *http.Request, name string) {
	api.mu.Lock()
	if job, ok := api.jobs[name]; ok {
		job.State = "ENABLED"
		api.scheduleJobLocked(job)
	}
	api.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// ─────────────────────────────────────────────────────────────────────────────
// Engine
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) scheduleJobLocked(job *Job) {
	if job.State != "ENABLED" {
		return
	}

	// Remove old if exists
	if id, ok := api.cronIDs[job.Name]; ok {
		api.cron.Remove(id)
	}

	id, err := api.cron.AddFunc(job.Schedule, func() {
		api.executeJob(job)
	})

	if err != nil {
		log.Printf("[Scheduler] Error scheduling job %s: %v", job.Name, err)
		return
	}
	api.cronIDs[job.Name] = id
}

func (api *API) executeJob(job *Job) {
	api.pushLog("INFO", job.Name, "Job started")
	startTime := time.Now()

	var err error
	if job.HttpTarget != nil {
		err = api.executeHttp(job.HttpTarget)
	} else if job.PubsubTarget != nil {
		err = api.executePubsub(job.PubsubTarget)
	} else if job.AppEngineTarget != nil {
		err = api.executeAppEngine(job.AppEngineTarget)
	}

	api.mu.Lock()
	job.LastAttemptTime = startTime.Format(time.RFC3339)
	if err != nil {
		job.Status = &Status{Code: 13, Message: err.Error()}
		api.pushLog("ERROR", job.Name, "Job failed: "+err.Error())
	} else {
		job.Status = &Status{Code: 0, Message: "Success"}
		api.pushLog("INFO", job.Name, "Job finished successfully")
	}
	api.mu.Unlock()
}

func (api *API) executeHttp(target *HttpTarget) error {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(target.HttpMethod, target.Uri, bytes.NewBufferString(target.Body))
	if err != nil {
		return err
	}
	for k, v := range target.Headers {
		req.Header.Set(k, v)
	}
	// Add MiniSky metadata
	req.Header.Set("User-Agent", "MiniSky-Cloud-Scheduler")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP error: %s", resp.Status)
	}
	return nil
}

func (api *API) executePubsub(target *PubsubTarget) error {
	// Publish to MiniSky PubSub shim
	payload := map[string]interface{}{
		"messages": []map[string]interface{}{
			{
				"data":       target.Data,
				"attributes": target.Attributes,
			},
		},
	}
	b, _ := json.Marshal(payload)
	resp, err := http.Post("http://localhost:8080/v1/"+target.TopicName+":publish", "application/json", bytes.NewBuffer(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("PubSub error: %s", resp.Status)
	}
	return nil
}

func (api *API) executeAppEngine(target *AppEngineTarget) error {
	// Mock AppEngine by calling local port 8080 with target path
	client := &http.Client{Timeout: 10 * time.Second}
	uri := "http://localhost:8080" + target.RelativeUri
	req, err := http.NewRequest(target.HttpMethod, uri, bytes.NewBufferString(target.Body))
	if err != nil {
		return err
	}
	for k, v := range target.Headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func extractJobName(path string) string {
	parts := strings.Split(path, "/jobs/")
	if len(parts) > 1 {
		return parts[0] + "/jobs/" + parts[1]
	}
	return ""
}
