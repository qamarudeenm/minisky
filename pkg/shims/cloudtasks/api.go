package cloudtasks

import (
	"encoding/json"
	"fmt"
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

type Queue struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type API struct {
	mu     sync.RWMutex
	queues map[string]*Queue
	tasks  map[string][]*Task
	logAPI *logging.API
}

func NewAPI() *API {
	return &API{
		queues: make(map[string]*Queue),
		tasks:  make(map[string][]*Task),
	}
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
		queueId := ""
		if len(parts) >= 7 {
			queueId = parts[6]
		}

		switch {
		case len(parts) == 6:
			if r.Method == http.MethodGet {
				api.listQueues(w, r, project)
				return
			}
			if r.Method == http.MethodPost {
				api.createQueue(w, r, project)
				return
			}
		case len(parts) == 7:
			if r.Method == http.MethodDelete {
				api.deleteQueue(w, r, project, queueId)
				return
			}
		case len(parts) >= 8 && parts[7] == "tasks":
			if len(parts) == 8 {
				if r.Method == http.MethodGet {
					api.listTasks(w, r, project, queueId)
					return
				}
				if r.Method == http.MethodPost {
					api.createTask(w, r, project, queueId)
					return
				}
			} else if len(parts) == 9 {
				if r.Method == http.MethodDelete {
					api.deleteTask(w, r, project, queueId, parts[8])
					return
				}
			}
		}
	}

	log.Printf("[Shim ERROR: Cloud Tasks] Unhandled %s %s", r.Method, r.URL.Path)
	w.WriteHeader(http.StatusNotFound)
}

func (api *API) listQueues(w http.ResponseWriter, r *http.Request, project string) {
	api.mu.RLock()
	defer api.mu.RUnlock()

	var result []*Queue
	prefix := fmt.Sprintf("projects/%s/locations/us-central1/queues/", project)
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
	api.queues[q.Name] = &q
	api.mu.Unlock()

	api.pushLog(project, "INFO", q.Name, "Created queue")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(q)
}

func (api *API) deleteQueue(w http.ResponseWriter, r *http.Request, project, queueId string) {
	name := fmt.Sprintf("projects/%s/locations/us-central1/queues/%s", project, queueId)
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

func (api *API) listTasks(w http.ResponseWriter, r *http.Request, project, queueId string) {
	name := fmt.Sprintf("projects/%s/locations/us-central1/queues/%s", project, queueId)
	
	api.mu.RLock()
	tasks := api.tasks[name]
	api.mu.RUnlock()

	if tasks == nil {
		tasks = []*Task{}
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"tasks": tasks})
}

func (api *API) createTask(w http.ResponseWriter, r *http.Request, project, queueId string) {
	var body struct {
		Task *Task `json:"task"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	queueName := fmt.Sprintf("projects/%s/locations/us-central1/queues/%s", project, queueId)
	
	task := body.Task
	if task == nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if task.Name == "" {
		task.Name = fmt.Sprintf("%s/tasks/%d", queueName, time.Now().UnixNano())
	}
	task.CreateTime = time.Now().Format(time.RFC3339)
	task.Status = "PENDING"

	api.mu.Lock()
	api.tasks[queueName] = append(api.tasks[queueName], task)
	api.mu.Unlock()

	api.pushLog(project, "INFO", queueName, "Task created: "+task.Name)
	
	// Background: Simulate task execution if it's an HTTP task
	if task.HTTPRequest != nil {
		go api.executeTask(project, queueName, task)
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

func (api *API) deleteTask(w http.ResponseWriter, r *http.Request, project, queueId, taskId string) {
	queueName := fmt.Sprintf("projects/%s/locations/us-central1/queues/%s", project, queueId)
	taskName := fmt.Sprintf("%s/tasks/%s", queueName, taskId)
	log.Printf("[Shim: Cloud Tasks] Attempting to delete task: %s", taskName)

	api.mu.Lock()
	defer api.mu.Unlock()

	tasks := api.tasks[queueName]
	for i, t := range tasks {
		if t.Name == taskName {
			api.tasks[queueName] = append(tasks[:i], tasks[i+1:]...)
			log.Printf("[Shim: Cloud Tasks] Successfully deleted task: %s", taskName)
			api.pushLog(project, "INFO", queueName, "Task deleted: "+taskName)
			w.WriteHeader(http.StatusOK)
			return
		}
	}

	log.Printf("[Shim WARNING: Cloud Tasks] Task not found for deletion: %s", taskName)
	w.WriteHeader(http.StatusNotFound)
}
