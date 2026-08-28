package goconfig

import (
	"fmt"
	"sync"

	"github.com/iancoleman/orderedmap"
)

// SourceData is an independent configuration payload returned by Source.
// Implementations must not reuse or mutate the returned byte slices.
type SourceData struct {
	Config []byte
	Schema []byte
}

// Source is the public, externally implementable persistence contract.
// Read returns the current JSON configuration and its JSON Schema. Write must
// durably replace the configuration or return an error without changing it.
type Source interface {
	Read() (SourceData, error)
	Write(config []byte) error
}

// ISource is the legacy source contract retained for v1 compatibility.
// New external implementations should implement Source and use
// NewManagerFromSource.
//
// Deprecated: use Source.
type ISource interface {
	// getConfigObject returns an independent OrderedMap snapshot.
	getConfigObject() *orderedmap.OrderedMap

	// getConfig returns the JSON string representation
	// The returned pointer should not be mutated
	getConfig() *string

	// getSchema returns the JSON schema string
	// The returned pointer should not be mutated
	getSchema() *string

	// setConfig persists a Manager-validated configuration atomically.
	setConfig(*orderedmap.OrderedMap) error
}

// sourceAdapter bridges the public Source API to the legacy internal contract.
// It owns a private parsed snapshot so external Source implementations never
// receive or expose Manager-owned mutable objects.
type sourceAdapter struct {
	mu     sync.RWMutex
	source Source
	object *orderedmap.OrderedMap
	config string
	schema string
}

func newSourceAdapter(source Source) (*sourceAdapter, error) {
	if source == nil {
		return nil, fmt.Errorf("source cannot be nil")
	}
	data, err := source.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read source: %w", err)
	}
	object, err := parseConfig(data.Config)
	if err != nil {
		return nil, err
	}
	return &sourceAdapter{
		source: source,
		object: object,
		config: string(append([]byte(nil), data.Config...)),
		schema: string(append([]byte(nil), data.Schema...)),
	}, nil
}

func (s *sourceAdapter) getConfigObject() *orderedmap.OrderedMap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	clone, _ := Clone(s.object)
	return clone
}

func (s *sourceAdapter) getConfig() *string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	config := s.config
	return &config
}

func (s *sourceAdapter) getSchema() *string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	schema := s.schema
	return &schema
}

func (s *sourceAdapter) setConfig(config *orderedmap.OrderedMap) error {
	owned, err := Clone(config)
	if err != nil {
		return err
	}
	data, err := owned.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := s.source.Write(data); err != nil {
		return err
	}

	s.mu.Lock()
	s.object = owned
	s.config = string(data)
	s.mu.Unlock()
	return nil
}
