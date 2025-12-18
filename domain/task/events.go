package task

import (
	"sync"
	"time"
)

// EventType represents the type of task event.
type EventType uint8

const (
	// EventCreated indicates a task was created.
	EventCreated EventType = iota
	// EventQueued indicates a task was queued for execution.
	EventQueued
	// EventStarted indicates a task started executing.
	EventStarted
	// EventProgress indicates a progress update.
	EventProgress
	// EventPaused indicates a task was paused.
	EventPaused
	// EventResumed indicates a task was resumed.
	EventResumed
	// EventCompleted indicates a task completed successfully.
	EventCompleted
	// EventFailed indicates a task failed.
	EventFailed
	// EventCancelled indicates a task was cancelled.
	EventCancelled
	// EventTimeout indicates a task timed out.
	EventTimeout
	// EventRetry indicates a task is being retried.
	EventRetry
)

// String returns a human-readable representation of the event type.
func (t EventType) String() string {
	switch t {
	case EventCreated:
		return "created"
	case EventQueued:
		return "queued"
	case EventStarted:
		return "started"
	case EventProgress:
		return "progress"
	case EventPaused:
		return "paused"
	case EventResumed:
		return "resumed"
	case EventCompleted:
		return "completed"
	case EventFailed:
		return "failed"
	case EventCancelled:
		return "cancelled"
	case EventTimeout:
		return "timeout"
	case EventRetry:
		return "retry"
	default:
		return "unknown"
	}
}

// Event represents a domain event for task lifecycle.
type Event struct {
	Type      EventType
	TaskID    string
	Timestamp time.Time
	Data      any // Event-specific data
}

// ProgressData contains progress information.
type ProgressData struct {
	Current int64
	Total   int64
	Message string
}

// ErrorData contains error information.
type ErrorData struct {
	Error       error
	RetryCount  int
	WillRetry   bool
	NextRetryAt time.Time
}

// EventHandler is a function that handles task events.
type EventHandler func(Event)

// EventBus provides a simple publish-subscribe mechanism for task events.
type EventBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]EventHandler
	allSubs  []EventHandler // Subscribers for all events
}

// NewEventBus creates a new event bus.
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[EventType][]EventHandler),
	}
}

// Subscribe registers a handler for a specific event type.
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// SubscribeAll registers a handler for all events.
func (eb *EventBus) SubscribeAll(handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.allSubs = append(eb.allSubs, handler)
}

// Publish sends an event to all registered handlers.
func (eb *EventBus) Publish(event Event) {
	eb.mu.RLock()
	handlers := eb.handlers[event.Type]
	allSubs := eb.allSubs
	eb.mu.RUnlock()

	for _, h := range handlers {
		h(event)
	}
	for _, h := range allSubs {
		h(event)
	}
}

// PublishAsync publishes an event asynchronously.
func (eb *EventBus) PublishAsync(event Event) {
	go eb.Publish(event)
}

// Clear removes all handlers.
func (eb *EventBus) Clear() {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.handlers = make(map[EventType][]EventHandler)
	eb.allSubs = nil
}
