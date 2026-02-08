package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SystemEvent represents a real-time system event/notification
type SystemEvent struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`     // info, warning, success, error
	Category  string                 `json:"category"` // tape, drive, backup, system
	Title     string                 `json:"title"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// EventBus manages event subscriptions and broadcasting
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[chan SystemEvent]struct{}
	history     []SystemEvent
	maxHistory  int
}

const (
	// eventChannelBufferSize is the buffer size for subscriber event channels
	eventChannelBufferSize = 50
)

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan SystemEvent]struct{}),
		history:     make([]SystemEvent, 0),
		maxHistory:  200,
	}
}

// Subscribe creates a new subscription channel
func (eb *EventBus) Subscribe() chan SystemEvent {
	ch := make(chan SystemEvent, eventChannelBufferSize)
	eb.mu.Lock()
	eb.subscribers[ch] = struct{}{}
	eb.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscription
func (eb *EventBus) Unsubscribe(ch chan SystemEvent) {
	eb.mu.Lock()
	delete(eb.subscribers, ch)
	close(ch)
	eb.mu.Unlock()
}

// Publish sends an event to all subscribers
func (eb *EventBus) Publish(event SystemEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}

	eb.mu.Lock()
	eb.history = append(eb.history, event)
	if len(eb.history) > eb.maxHistory {
		eb.history = eb.history[len(eb.history)-eb.maxHistory:]
	}
	eb.mu.Unlock()

	eb.mu.RLock()
	for ch := range eb.subscribers {
		select {
		case ch <- event:
		default:
			// Drop event if subscriber is too slow
		}
	}
	eb.mu.RUnlock()
}

// GetHistory returns recent events
func (eb *EventBus) GetHistory() []SystemEvent {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	result := make([]SystemEvent, len(eb.history))
	copy(result, eb.history)
	return result
}

// handleEventStream handles SSE connections for real-time events
func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.respondError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := s.eventBus.Subscribe()
	defer s.eventBus.Unsubscribe(ch)

	// Send recent history first
	for _, event := range s.eventBus.GetHistory() {
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "data: %s\n\n", data)
	}
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// handleGetNotifications returns recent notification history
func (s *Server) handleGetNotifications(w http.ResponseWriter, r *http.Request) {
	events := s.eventBus.GetHistory()
	s.respondJSON(w, http.StatusOK, events)
}
