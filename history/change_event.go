package history

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"
)

// ChangeEvent represents a single modification to the configuration
type ChangeEvent struct {
	Timestamp time.Time   `json:"timestamp"`
	Operation string      `json:"operation"` // "insert", "remove", "replace"
	Path      string      `json:"path"`
	Index     *int        `json:"index,omitempty"`
	OldValue  interface{} `json:"old_value,omitempty"`
	NewValue  interface{} `json:"new_value,omitempty"`
	User      string      `json:"user,omitempty"`
	Version   int64       `json:"version"`
}

// ChangeHistory maintains a log of configuration changes
type ChangeHistory struct {
	mu         sync.RWMutex
	events     []ChangeEvent
	maxSize    int
	eventIndex int // circular buffer index
}

// NewChangeHistory creates a new change history with specified max size
func NewChangeHistory(maxSize int) *ChangeHistory {
	if maxSize <= 0 {
		maxSize = 1000 // default
	}
	return &ChangeHistory{
		events:  make([]ChangeEvent, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add appends a new change event
func (ch *ChangeHistory) Add(event ChangeEvent) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	event = cloneEvent(event)
	if len(ch.events) < ch.maxSize {
		ch.events = append(ch.events, event)
	} else {
		// Circular buffer: overwrite oldest
		ch.events[ch.eventIndex] = event
		ch.eventIndex = (ch.eventIndex + 1) % ch.maxSize
	}
}

// GetAll returns all events in chronological order
func (ch *ChangeHistory) GetAll() []ChangeEvent {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	return cloneEvents(ch.allLocked())
}

func (ch *ChangeHistory) allLocked() []ChangeEvent {
	if len(ch.events) < ch.maxSize {
		// Buffer not full yet
		result := make([]ChangeEvent, len(ch.events))
		copy(result, ch.events)
		return result
	}

	// Buffer is full, reorder from oldest to newest
	result := make([]ChangeEvent, ch.maxSize)
	copy(result, ch.events[ch.eventIndex:])
	copy(result[ch.maxSize-ch.eventIndex:], ch.events[:ch.eventIndex])
	return result
}

// GetByPath returns events for a specific path
func (ch *ChangeHistory) GetByPath(path string, limit int) []ChangeEvent {
	if limit <= 0 {
		return []ChangeEvent{}
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	all := ch.allLocked()
	result := make([]ChangeEvent, 0)

	for i := len(all) - 1; i >= 0 && len(result) < limit; i-- {
		if all[i].Path == path {
			result = append(result, all[i])
		}
	}

	return cloneEvents(result)
}

// GetRecent returns the N most recent events
func (ch *ChangeHistory) GetRecent(limit int) []ChangeEvent {
	if limit <= 0 {
		return []ChangeEvent{}
	}
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	all := ch.allLocked()
	if len(all) <= limit {
		return cloneEvents(all)
	}
	return cloneEvents(all[len(all)-limit:])
}

// Clear removes all events
func (ch *ChangeHistory) Clear() {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	ch.events = ch.events[:0]
	ch.eventIndex = 0
}

// ExportJSON exports history as JSON
func (ch *ChangeHistory) ExportJSON() ([]byte, error) {
	return json.MarshalIndent(ch.GetAll(), "", "  ")
}

func cloneEvents(events []ChangeEvent) []ChangeEvent {
	result := make([]ChangeEvent, len(events))
	for index, event := range events {
		result[index] = cloneEvent(event)
	}
	return result
}

func cloneEvent(event ChangeEvent) ChangeEvent {
	cloned := event
	if event.Index != nil {
		index := *event.Index
		cloned.Index = &index
	}
	cloned.OldValue = cloneJSONPayload(event.OldValue)
	cloned.NewValue = cloneJSONPayload(event.NewValue)
	return cloned
}

func cloneJSONPayload(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		// Change payloads are documented as JSON values. Unsupported values are
		// discarded rather than retaining a caller-owned mutable reference.
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var cloned interface{}
	if err := decoder.Decode(&cloned); err != nil {
		return nil
	}
	return cloned
}
