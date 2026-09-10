package history

import (
	"encoding/json"
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
	all := ch.GetAll()
	result := make([]ChangeEvent, 0)

	for i := len(all) - 1; i >= 0 && len(result) < limit; i-- {
		if all[i].Path == path {
			result = append(result, all[i])
		}
	}

	return result
}

// GetRecent returns the N most recent events
func (ch *ChangeHistory) GetRecent(limit int) []ChangeEvent {
	all := ch.GetAll()
	if len(all) <= limit {
		return all
	}
	return all[len(all)-limit:]
}

// Clear removes all events
func (ch *ChangeHistory) Clear() {
	ch.events = ch.events[:0]
	ch.eventIndex = 0
}

// ExportJSON exports history as JSON
func (ch *ChangeHistory) ExportJSON() ([]byte, error) {
	return json.MarshalIndent(ch.GetAll(), "", "  ")
}