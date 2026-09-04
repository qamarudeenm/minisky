package cloudtasks

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minisky/pkg/registry"
	"minisky/pkg/shims/logging"
)

func init() {
	registry.Register("cloudtasks.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI()
	})

	registry.RegisterRoutes("cloudtasks.googleapis.com",
		"/v2/projects/*/locations/*/queues",
	)
}

type Task struct {
	Name         string            `json:"name"`
	HTTPRequest  *HTTPRequest      `json:"httpRequest,omitempty"`
	CreateTime   string            `json:"createTime"`
	ScheduleTime string            `json:"scheduleTime,omitempty"`
	Status       string            `json:"status"` // Internal use
}

type HTTPRequest struct {
	URL        string            `json:"url"`
	HTTPMethod string            `json:"httpMethod"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"` // Base64
}

// RateLimits caps how fast a queue dispatches.
type RateLimits struct {
	MaxDispatchesPerSecond  float64 `json:"maxDispatchesPerSecond,omitempty"`
	MaxBurstSize            int     `json:"maxBurstSize,omitempty"`
	MaxConcurrentDispatches int     `json:"maxConcurrentDispatches,omitempty"`
}

// RetryConfig describes what happens to a task that fails.
type RetryConfig struct {
	MaxAttempts      int    `json:"maxAttempts,omitempty"`
	MaxRetryDuration string `json:"maxRetryDuration,omitempty"`
	MinBackoff       string `json:"minBackoff,omitempty"`
	MaxBackoff       string `json:"maxBackoff,omitempty"`
	MaxDoublings     int    `json:"maxDoublings,omitempty"`
}

// AppEngineRouting overrides where a queue's App Engine tasks are sent.
type AppEngineRouting struct {
	Service  string `json:"service,omitempty"`
	Version  string `json:"version,omitempty"`
	Instance string `json:"instance,omitempty"`
	Host     string `json:"host,omitempty"`
}

// StackdriverLoggingConfig sets what fraction of operations are logged.
type StackdriverLoggingConfig struct {
	SamplingRatio float64 `json:"samplingRatio"`
}

type Queue struct {
	Name                     string                    `json:"name"`
	State                    string                    `json:"state"`
	RateLimits               *RateLimits               `json:"rateLimits,omitempty"`
	RetryConfig              *RetryConfig              `json:"retryConfig,omitempty"`
	AppEngineRoutingOverride *AppEngineRouting         `json:"appEngineRoutingOverride,omitempty"`
	StackdriverLoggingConfig *StackdriverLoggingConfig `json:"stackdriverLoggingConfig,omitempty"`
	PurgeTime                string                    `json:"purgeTime,omitempty"`
}

// Google's defaults for the fields a caller leaves unset. They matter because
// Terraform reads every one of them back as a computed value: a queue that
// reports nothing where the API would report 500 dispatches per second is drift
// the provider proposes to fix on every plan and never can.
const (
	defaultDispatchesPerSecond  = 500
	defaultBurstSize            = 100
	defaultConcurrentDispatches = 1000
	defaultMaxAttempts          = 100
	defaultMaxRetryDuration     = "0s"
	defaultMinBackoff           = "0.100s"
	defaultMaxBackoff           = "3600s"
	defaultMaxDoublings         = 16
)

// applyQueueDefaults fills in every value the caller left unset.
//
// It runs after a create and after a patch, because a patch replaces a whole
// group: a caller who sends retryConfig with only maxAttempts gets Google's
// defaults for the rest, which is what the real API does.
func applyQueueDefaults(q *Queue) {
	if q.State == "" {
		q.State = "RUNNING"
	}
	if q.RateLimits == nil {
		q.RateLimits = &RateLimits{}
	}
	if q.RateLimits.MaxDispatchesPerSecond == 0 {
		q.RateLimits.MaxDispatchesPerSecond = defaultDispatchesPerSecond
	}
	if q.RateLimits.MaxBurstSize == 0 {
		q.RateLimits.MaxBurstSize = defaultBurstSize
	}
	if q.RateLimits.MaxConcurrentDispatches == 0 {
		q.RateLimits.MaxConcurrentDispatches = defaultConcurrentDispatches
	}

	if q.RetryConfig == nil {
		q.RetryConfig = &RetryConfig{}
	}
	if q.RetryConfig.MaxAttempts == 0 {
		q.RetryConfig.MaxAttempts = defaultMaxAttempts
	}
	if q.RetryConfig.MaxRetryDuration == "" {
		q.RetryConfig.MaxRetryDuration = defaultMaxRetryDuration
	}
	if q.RetryConfig.MinBackoff == "" {
		q.RetryConfig.MinBackoff = defaultMinBackoff
	}
	if q.RetryConfig.MaxBackoff == "" {
		q.RetryConfig.MaxBackoff = defaultMaxBackoff
	}
	if q.RetryConfig.MaxDoublings == 0 {
		q.RetryConfig.MaxDoublings = defaultMaxDoublings
	}
}

type API struct {
	mu     sync.RWMutex
	queues map[string]*Queue
	tasks  map[string][]*Task
	logAPI *logging.API
}

func NewAPI() *API {
	api := &API{
		queues: make(map[string]*Queue),
		tasks:  make(map[string][]*Task),
	}
	api.restore()
	return api
}

func (api *API) OnPostBoot(ctx *registry.Context) {
	if logShim, ok := ctx.GetShim("logging.googleapis.com").(*logging.API); ok {
		api.logAPI = logShim
	}
}

func (api *API) pushLog(projectId, severity, resourceName, text string) {
	if api.logAPI == nil {
		return
	}
	api.logAPI.PushLog(projectId, severity, "cloud_tasks_queue", resourceName, text)
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer api.persistIfMutated(r)


	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	log.Printf("[Shim: Cloud Tasks DEBUG] %s %s (Parts: %d)", r.Method, r.URL.Path, len(parts))

	if len(parts) < 3 || parts[1] != "projects" {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "discovery") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	project := parts[2]

	// Handle REST API
	// v2/projects/{project}/locations/{location}/queues
	if len(parts) >= 6 && parts[0] == "v2" && parts[3] == "locations" && parts[5] == "queues" {
		location := parts[4]
		queueId, verb := "", ""
		if len(parts) >= 7 {
			// A custom method arrives attached to the queue id, as
			// ".../queues/emails:pause".
			queueId, verb, _ = strings.Cut(parts[6], ":")
		}

		switch {
		case len(parts) == 6:
			if r.Method == http.MethodGet {
				api.listQueues(w, r, project, location)
				return
			}
			if r.Method == http.MethodPost {
				api.createQueue(w, r, project)
				return
			}
		case len(parts) == 7:
			if verb != "" {
				if r.Method != http.MethodPost {
					w.WriteHeader(http.StatusMethodNotAllowed)
					return
				}
				switch verb {
				case "pause":
					api.setQueueState(w, project, location, queueId, "PAUSED")
					return
				case "resume":
					api.setQueueState(w, project, location, queueId, "RUNNING")
					return
				case "purge":
					api.purgeQueue(w, project, location, queueId)
					return
				}
				break
			}
			if r.Method == http.MethodGet {
				api.getQueue(w, project, location, queueId)
				return
			}
			if r.Method == http.MethodPatch {
				api.patchQueue(w, r, project, location, queueId)
				return
			}
			if r.Method == http.MethodDelete {
				api.deleteQueue(w, r, project, location, queueId)
				return
			}
		case len(parts) >= 8 && parts[7] == "tasks":
			if len(parts) == 8 {
				if r.Method == http.MethodGet {
					api.listTasks(w, r, project, location, queueId)
					return
				}
				if r.Method == http.MethodPost {
					api.createTask(w, r, project, location, queueId)
					return
				}
			} else if len(parts) == 9 {
				if r.Method == http.MethodDelete {
					api.deleteTask(w, r, project, location, queueId, parts[8])
					return
				}
			}
		}
	}

	log.Printf("[Shim ERROR: Cloud Tasks] Unhandled %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusNotFound)
}

// queueName is the canonical resource name of a queue. Every lookup used to
// build this with the location hardcoded to us-central1 while createQueue keyed
// by the name the client sent, so a queue in any other region could be created
// and then never read, listed or deleted.
func queueName(project, location, queueId string) string {
	return fmt.Sprintf("projects/%s/locations/%s/queues/%s", project, location, queueId)
}

// getQueue serves queues.get, which is how a client — Terraform included —
// reads a queue back after creating it.
func (api *API) getQueue(w http.ResponseWriter, project, location, queueId string) {
	name := queueName(project, location, queueId)

	api.mu.RLock()
	q, ok := api.queues[name]
	api.mu.RUnlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":404,"message":"Queue does not exist."}}`))
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(q)
}

func (api *API) listQueues(w http.ResponseWriter, r *http.Request, project, location string) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	var result []*Queue
	prefix := fmt.Sprintf("projects/%s/locations/%s/queues/", project, location)
	for name, q := range api.queues {
		if strings.HasPrefix(name, prefix) {
			result = append(result, q)
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"queues": result})
}

func (api *API) createQueue(w http.ResponseWriter, r *http.Request, project string) {
	var q Queue
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	api.mu.Lock()
	if _, exists := api.queues[q.Name]; exists {
		api.mu.Unlock()
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`{"error":{"code":409,"message":"Queue already exists"}}`))
		return
	}
	q.State = "RUNNING"
	applyQueueDefaults(&q)
	api.queues[q.Name] = &q
	api.mu.Unlock()

	api.pushLog(project, "INFO", q.Name, "Created queue")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(q)
}

// patchQueue serves queues.patch, which is how a settings change reaches the
// API. Like the real one it creates the queue when it does not exist.
//
// updateMask is honoured at the level of the top-level group: a mask naming a
// single leaf replaces the group that leaf belongs to. That is the same result
// whenever the caller sends the whole group, which every generated client does,
// and an omitted leaf then takes Google's default — as it does on the real API,
// where a patched group is replaced rather than merged.
func (api *API) patchQueue(w http.ResponseWriter, r *http.Request, project, location, queueId string) {
	var body Queue
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	name := queueName(project, location, queueId)
	masked := maskedGroups(r.URL.Query().Get("updateMask"))

	api.mu.Lock()
	q, existing := api.queues[name]
	if !existing {
		q = &Queue{Name: name}
		api.queues[name] = q
	}

	if masked("rateLimits") {
		q.RateLimits = body.RateLimits
	}
	if masked("retryConfig") {
		q.RetryConfig = body.RetryConfig
	}
	if masked("appEngineRoutingOverride") {
		q.AppEngineRoutingOverride = body.AppEngineRoutingOverride
	}
	if masked("stackdriverLoggingConfig") {
		q.StackdriverLoggingConfig = body.StackdriverLoggingConfig
	}
	applyQueueDefaults(q)
	updated := *q
	api.mu.Unlock()

	action := "Updated queue"
	if !existing {
		action = "Created queue by patch"
	}
	api.pushLog(project, "INFO", name, action)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updated)
}

// maskedGroups returns a predicate for whether a top-level field group is in
// the update mask. An empty mask means every group the body carries.
func maskedGroups(mask string) func(string) bool {
	if strings.TrimSpace(mask) == "" {
		return func(string) bool { return true }
	}
	groups := map[string]bool{}
	for _, path := range strings.Split(mask, ",") {
		path = strings.TrimSpace(path)
		if group, _, found := strings.Cut(path, "."); found {
			groups[group] = true
		} else if path != "" {
			groups[path] = true
		}
	}
	return func(group string) bool { return groups[group] }
}

// setQueueState serves queues.pause and queues.resume.
func (api *API) setQueueState(w http.ResponseWriter, project, location, queueId, state string) {
	name := queueName(project, location, queueId)

	api.mu.Lock()
	q, ok := api.queues[name]
	if ok {
		q.State = state
	}
	var updated Queue
	if ok {
		updated = *q
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":404,"message":"Queue does not exist."}}`))
		return
	}
	api.pushLog(project, "INFO", name, "Queue state set to "+state)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updated)
}

// purgeQueue serves queues.purge: it drops the queue's tasks and leaves the
// queue itself in place.
func (api *API) purgeQueue(w http.ResponseWriter, project, location, queueId string) {
	name := queueName(project, location, queueId)

	api.mu.Lock()
	q, ok := api.queues[name]
	if ok {
		delete(api.tasks, name)
		q.PurgeTime = time.Now().UTC().Format(time.RFC3339Nano)
	}
	var updated Queue
	if ok {
		updated = *q
	}
	api.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"code":404,"message":"Queue does not exist."}}`))
		return
	}
	api.pushLog(project, "INFO", name, "Purged queue")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updated)
}

func (api *API) deleteQueue(w http.ResponseWriter, r *http.Request, project, location, queueId string) {
	name := queueName(project, location, queueId)
	log.Printf("[Shim: Cloud Tasks] Attempting to delete queue: %s", name)
	
	api.mu.Lock()
	_, exists := api.queues[name]
	delete(api.queues, name)
	delete(api.tasks, name)
	api.mu.Unlock()

	if !exists {
		log.Printf("[Shim WARNING: Cloud Tasks] Queue not found for deletion: %s", name)
	} else {
		api.pushLog(project, "INFO", name, "Deleted queue")
	}
	w.WriteHeader(http.StatusOK)
}

func (api *API) listTasks(w http.ResponseWriter, r *http.Request, project, location, queueId string) {
	name := queueName(project, location, queueId)
	
	api.mu.RLock()
	tasks := api.tasks[name]
	api.mu.RUnlock()

	if tasks == nil {
		tasks = []*Task{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"tasks": tasks})
}

func (api *API) createTask(w http.ResponseWriter, r *http.Request, project, location, queueId string) {
	var body struct {
		Task *Task `json:"task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	queue := queueName(project, location, queueId)

	task := body.Task
	if task == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if task.Name == "" {
		task.Name = fmt.Sprintf("%s/tasks/%d", queue, time.Now().UnixNano())
	}
	task.CreateTime = time.Now().Format(time.RFC3339)
	task.Status = "PENDING"

	api.mu.Lock()
	api.tasks[queue] = append(api.tasks[queue], task)
	q, known := api.queues[queue]
	paused := known && q.State == "PAUSED"
	api.mu.Unlock()

	api.pushLog(project, "INFO", queue, "Task created: "+task.Name)

	// Background: Simulate task execution if it's an HTTP task. A paused queue
	// accepts tasks and holds them — dispatching from one would make pause a
	// label rather than a state.
	if task.HTTPRequest != nil && !paused {
		go api.executeTask(project, queue, task)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(task)
}

func (api *API) executeTask(project, queueName string, task *Task) {
	// Wait a bit to simulate asynchronous processing
	time.Sleep(2 * time.Second)
	
	log.Printf("[Shim: Cloud Tasks] Executing task %s -> %s %s", task.Name, task.HTTPRequest.HTTPMethod, task.HTTPRequest.URL)
	
	// In a real emulator, we would make the HTTP call here.
	// For now, we'll just log it in MiniSky's logs.
	api.pushLog(project, "INFO", queueName, fmt.Sprintf("Task executed successfully: %s (Target: %s)", task.Name, task.HTTPRequest.URL))
	
	// Update task status
	api.mu.Lock()
	for _, t := range api.tasks[queueName] {
		if t.Name == task.Name {
			t.Status = "COMPLETED"
			break
		}
	}
	api.mu.Unlock()
}

func (api *API) deleteTask(w http.ResponseWriter, r *http.Request, project, location, queueId, taskId string) {
	queue := queueName(project, location, queueId)
	taskName := fmt.Sprintf("%s/tasks/%s", queue, taskId)
	log.Printf("[Shim: Cloud Tasks] Attempting to delete task: %s", taskName)

	api.mu.Lock()
	defer api.mu.Unlock()

	tasks := api.tasks[queue]
	for i, t := range tasks {
		if t.Name == taskName {
			api.tasks[queue] = append(tasks[:i], tasks[i+1:]...)
			log.Printf("[Shim: Cloud Tasks] Successfully deleted task: %s", taskName)
			api.pushLog(project, "INFO", queue, "Task deleted: "+taskName)
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	log.Printf("[Shim WARNING: Cloud Tasks] Task not found for deletion: %s", taskName)
	w.WriteHeader(http.StatusNotFound)
}
