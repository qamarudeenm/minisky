# Alternative Architecture: Global Event Registry

This document outlines a more robust architectural approach for MiniSky event handling, considered for future scalability beyond the initial **Shim-to-Shim** implementation.

## The Concept

Instead of shims communicating directly (e.g., Pub/Sub telling Serverless that a message arrived), a central **Event Broker** component within the Orchestrator manages the lifecycle of all platform events.

### Key Components

1.  **Event Broker Service**:
    - A central registry where any shim can "Publish" an event.
    - An "Event Schema" that standardizes metadata (Source, Type, Timestamp, Payload).
    - Persistent event history (optional) for replayability.

2.  **Subscriber Registry**:
    - Functions and services register "Event Filters" (e.g., source == 'pubsub' && topic == 'orders').
    - Support for multiple subscribers per event (Fan-out).

3.  **Delivery Engine**:
    - Handles retry logic if a function container is cold-starting or busy.
    - Supports different delivery modes (Push, Pull, Dead Letter Queues).

## Pros & Cons

| Feature | Shim-to-Shim (Current) | Global Event Registry (Future) |
| :--- | :--- | :--- |
| **Complexity** | Low | High |
| **Coupling** | High (Shims must know each other) | Low (Decoupled) |
| **Scalability** | Limited | High (Supports massive fan-out) |
| **Observability**| Harder to trace | Centralized Audit Log |
| **Reliability** | Synchronous Failure | Asynchronous Retries |

## Implementation Sketch

If implemented, the **Orchestrator** would expose an `EventBus` interface:

```go
type Event struct {
    ID        string
    Source    string
    Type      string
    Timestamp time.Time
    Data      []byte
}

type EventBus interface {
    Publish(ctx context.Context, e Event) error
    Subscribe(filter Filter, callback func(Event)) string
}
```

This would allow the **Pub/Sub Shim** to remain completely unaware of the **Serverless Shim**, simply broadcasting that a topic received data.
