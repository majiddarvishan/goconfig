package goconfig

import (
	"fmt"
	"sync"

	"github.com/iancoleman/orderedmap"
)

type StrSource struct {
	mu           sync.RWMutex
	configObject *orderedmap.OrderedMap
	config       string
	schema       string
}

func NewStrSource(config, schema string) (*StrSource, error) {
	if config == "" {
		return nil, fmt.Errorf("config cannot be empty")
	}

	configMap, err := parseConfig([]byte(config))
	if err != nil {
		return nil, err
	}

	return &StrSource{
		configObject: configMap,
		config:       config,
		schema:       schema,
	}, nil
}

func (s *StrSource) getConfigObject() *orderedmap.OrderedMap {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.configObject
}

func (s *StrSource) getConfig() *string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	config := s.config
	return &config
}

func (s *StrSource) getSchema() *string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	schema := s.schema
	return &schema
}

func (s *StrSource) setConfig(conf *orderedmap.OrderedMap, data []byte) error {
	if conf == nil {
		return fmt.Errorf("config cannot be nil")
	}
	if len(data) == 0 {
		return fmt.Errorf("data cannot be empty")
	}

	s.mu.Lock()
	s.configObject = conf
	s.config = string(data)
	s.mu.Unlock()

	return nil
}
