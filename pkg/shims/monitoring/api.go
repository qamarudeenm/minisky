package monitoring

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("monitoring.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI(ctx.SvcMgr)
	})
}

func (api *API) OnPostBoot(ctx *registry.Context) {
	api.StartCollector()
}

type TimeSeries struct {
	Metric struct {
		Type   string            `json:"type"`
		Labels map[string]string `json:"labels"`
	} `json:"metric"`
	Resource struct {
		Type   string            `json:"type"`
		Labels map[string]string `json:"labels"`
	} `json:"resource"`
	Points []Point `json:"points"`
}

type Point struct {
	Interval struct {
		EndTime string `json:"endTime"`
	} `json:"interval"`
	Value struct {
		DoubleValue *float64 `json:"doubleValue,omitempty"`
		Int64Value  *int64   `json:"int64Value,omitempty"`
	} `json:"value"`
}

type API struct {
	mu     sync.RWMutex
	svcMgr *orchestrator.ServiceManager
	stats  map[string][]Point // resourceId -> points
}

func NewAPI(sm *orchestrator.ServiceManager) *API {
	return &API{
		svcMgr: sm,
		stats:  make(map[string][]Point),
	}
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[Shim: Monitoring] %s %s", r.Method, r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	if strings.Contains(r.URL.Path, "/timeSeries") {
		api.handleTimeSeries(w, r)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{}"))
}

func (api *API) handleTimeSeries(w http.ResponseWriter, r *http.Request) {
	api.mu.RLock()
	defer api.mu.RUnlock()
	json.NewEncoder(w).Encode(api.stats)
}

func (api *API) GetStats() map[string][]Point {
	api.mu.RLock()
	defer api.mu.RUnlock()
	return api.stats
}

// StartCollector begins a background loop to collect CPU/Mem stats from Docker
func (api *API) StartCollector() {
	log.Printf("[Monitoring] 📈 Starting Metrics Collector...")
	go func() {
		for {
			containers := api.svcMgr.ListManagedContainers()
			now := time.Now().Format(time.RFC3339)
			
			api.mu.Lock()
			for _, c := range containers {
				if !strings.Contains(c.Status, "Up") {
					continue
				}
				
				stats, err := api.svcMgr.GetContainerStats(c.Name)
				if err != nil {
					continue
				}

				name := strings.TrimPrefix(c.Name, "minisky-")
				
				// CPU Point
				cpuPoint := Point{}
				cpuPoint.Interval.EndTime = now
				cpuVal := stats.CPUPercentage
				cpuPoint.Value.DoubleValue = &cpuVal
				
				api.stats[name+"_cpu"] = append(api.stats[name+"_cpu"], cpuPoint)
				if len(api.stats[name+"_cpu"]) > 60 { // keep last 60 points (10 minutes at 10s intervals)
					api.stats[name+"_cpu"] = api.stats[name+"_cpu"][1:]
				}

				// Memory Point
				memPoint := Point{}
				memPoint.Interval.EndTime = now
				memVal := stats.MemoryUsageMB
				memPoint.Value.DoubleValue = &memVal
				
				api.stats[name+"_mem"] = append(api.stats[name+"_mem"], memPoint)
				if len(api.stats[name+"_mem"]) > 60 {
					api.stats[name+"_mem"] = api.stats[name+"_mem"][1:]
				}
			}
			api.mu.Unlock()
			time.Sleep(10 * time.Second)
		}
	}()
}

