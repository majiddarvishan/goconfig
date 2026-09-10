package goconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/iancoleman/orderedmap"
)

func parseConfig(config []byte) (*orderedmap.OrderedMap, error) {
	if len(config) == 0 {
		return nil, fmt.Errorf("config is empty")
	}

	result := orderedmap.New()
	err := json.Unmarshal(config, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return result, nil
}

type FileSource struct {
	mu           sync.RWMutex
	configPath   string
	configObject *orderedmap.OrderedMap
	config       string
	schema       string
}

func NewFileSource(configPath string, schema string) (*FileSource, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path cannot be empty")
	}

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config, err := parseConfig(configBytes)
	if err != nil {
		return nil, err
	}

	return &FileSource{
		configPath:   configPath,
		configObject: config,
		config:       string(configBytes),
		schema:       schema,
	}, nil
}

func (fs *FileSource) getConfigObject() *orderedmap.OrderedMap {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.configObject
}

func (fs *FileSource) getConfig() *string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	config := fs.config
	return &config
}

func (fs *FileSource) getSchema() *string {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	schema := fs.schema
	return &schema
}

func (fs *FileSource) setConfig(conf *orderedmap.OrderedMap) error {
	if conf == nil {
		return fmt.Errorf("config cannot be nil")
	}

	configBytes, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to temp file first, then rename (atomic operation)
	tempPath := fs.configPath + ".tmp"
	err = os.WriteFile(tempPath, configBytes, 0644)
	if err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	err = os.Rename(tempPath, fs.configPath)
	if err != nil {
		os.Remove(tempPath) // Clean up temp file
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	fs.mu.Lock()
	fs.configObject = conf
	fs.config = string(configBytes)
	fs.mu.Unlock()

	return nil
}
