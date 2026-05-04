package orchestrator

import (
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// OperationStatus mirrors GCP's LRO status strings.
type OperationStatus string

const (
	StatusPending OperationStatus = "PENDING"
	StatusRunning OperationStatus = "RUNNING"
	StatusDone    OperationStatus = "DONE"
)

// Operation represents a single GCP Long-Running Operation.
type Operation struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Kind          string          `json:"kind"`
	OperationType string          `json:"operationType"`
	Status        OperationStatus `json:"status"`
	TargetLink    string          `json:"targetLink,omitempty"`
	Progress      int             `json:"progress"`
	Done          bool            `json:"done"`
	// InsertTime / StartTime / EndTime in RFC3339 format
	InsertTime string `json:"insertTime,omitempty"`
	StartTime  string `json:"startTime,omitempty"`
	EndTime    string `json:"endTime,omitempty"`
	Metadata   interface{} `json:"metadata,omitempty"`
	// Error is only set when the operation fails.
	Error *OperationError `json:"error,omitempty"`
	// Zone or Region scoping (optional, service-specific)
	Zone   string `json:"zone,omitempty"`
	Region string `json:"region,omitempty"`
}

// OperationError provides GCP-shaped error details on failure.
type OperationError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// OperationManager is a thread-safe in-memory registry for all active LROs.
type OperationManager struct {
	mu  sync.RWMutex
	ops map[string]*Operation
}

// NewOperationManager returns a ready-to-use OperationManager.
func NewOperationManager() *OperationManager {
	return &OperationManager{
		ops: make(map[string]*Operation),
	}
}

// Register creates a new operation and stores it. Returns the operation for immediate serialisation.
func (om *OperationManager) Register(kind, operationType, targetLink, zone, region string) *Operation {
	id := fmt.Sprintf("%d", rand.Int63())
	name := fmt.Sprintf("operation-%d-%s", time.Now().Unix(), randomSuffix(8))

	op := &Operation{
		ID:            id,
		Name:          name,
		Kind:          kind,
		OperationType: operationType,
		Status:        StatusPending,
		TargetLink:    targetLink,
		Progress:      0,
		Done:          false,
		InsertTime:    time.Now().UTC().Format(time.RFC3339),
		Zone:          zone,
		Region:        region,
	}

	om.mu.Lock()
	om.ops[name] = op
	om.mu.Unlock()

	return op
}

// Get retrieves an operation by name. Returns nil if not found.
func (om *OperationManager) Get(name string) *Operation {
	om.mu.RLock()
	defer om.mu.RUnlock()
	return om.ops[name]
}

// Advance moves the operation through the PENDING → RUNNING → DONE state machine.
// It should be called from a background goroutine.
func (om *OperationManager) Advance(name string, progress int, status OperationStatus) {
	om.mu.Lock()
	defer om.mu.Unlock()

	op, ok := om.ops[name]
	if !ok {
		return
	}

	op.Progress = progress
	op.Status = status

	if status == StatusRunning && op.StartTime == "" {
		op.StartTime = time.Now().UTC().Format(time.RFC3339)
	}

	if status == StatusDone {
		op.Done = true
		op.Progress = 100
		op.EndTime = time.Now().UTC().Format(time.RFC3339)
	}
}

// UpdateMetadata updates the metadata of an operation.
func (om *OperationManager) UpdateMetadata(name string, metadata interface{}) {
	om.mu.Lock()
	defer om.mu.Unlock()
	if op, ok := om.ops[name]; ok {
		op.Metadata = metadata
	}
}

// MarkDone marks the operation as successfully completed.
func (om *OperationManager) MarkDone(name string) {
	om.Advance(name, 100, StatusDone)
}

// List returns all operations in the registry.
func (om *OperationManager) List() []*Operation {
	om.mu.RLock()
	defer om.mu.RUnlock()
	res := make([]*Operation, 0, len(om.ops))
	for _, op := range om.ops {
		res = append(res, op)
	}
	return res
}

// Fail marks the operation as DONE with an error.
func (om *OperationManager) Fail(name string, code int, message string) {
	om.mu.Lock()
	defer om.mu.Unlock()

	op, ok := om.ops[name]
	if !ok {
		return
	}
	op.Status = StatusDone
	op.Done = true
	op.Progress = 100
	op.EndTime = time.Now().UTC().Format(time.RFC3339)
	op.Error = &OperationError{Code: code, Message: message}
}

// RunAsync drives a standard 3-phase LRO lifecycle in a goroutine.
// It ensures that intermediate states (PENDING, RUNNING) are visible to polling clients
// by introducing artificial delays and granular progress increments.
func (om *OperationManager) RunAsync(name string, workFn func() error) {
	go func() {
		// 1. Initial delay to ensure the caller (Terraform/UI) registers the initial PENDING state
		time.Sleep(800 * time.Millisecond)
		
		// 2. Transition PENDING → RUNNING (Low progress)
		om.Advance(name, 5, StatusRunning)
		time.Sleep(1200 * time.Millisecond)
		
		// 3. Increment progress to show life before work starts
		om.Advance(name, 25, StatusRunning)
		time.Sleep(500 * time.Millisecond)

		// 4. Execute actual work (container boot, provisioning, etc.)
		if err := workFn(); err != nil {
			om.Fail(name, 500, err.Error())
			return
		}
		
		// 5. Successful work completion - show high progress before finishing
		om.Advance(name, 85, StatusRunning)
		time.Sleep(500 * time.Millisecond)

		// 6. Transition RUNNING → DONE
		om.Advance(name, 100, StatusDone)
	}()
}

func randomSuffix(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
