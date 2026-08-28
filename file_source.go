package goconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/iancoleman/orderedmap"
)

type FileSource struct {
	mu           sync.RWMutex
	configPath   string
	configObject *orderedmap.OrderedMap
	config       string
	schema       string
}

// Read implements Source and returns independent configuration bytes.
func (fs *FileSource) Read() (SourceData, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return SourceData{
		Config: append([]byte(nil), fs.config...),
		Schema: append([]byte(nil), fs.schema...),
	}, nil
}

// Write implements Source after verifying that config is a JSON object.
func (fs *FileSource) Write(config []byte) error {
	object, err := parseConfig(config)
	if err != nil {
		return err
	}
	return fs.setConfig(object)
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
	clone, _ := Clone(fs.configObject)
	return clone
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

	ownedConfig, err := Clone(conf)
	if err != nil {
		return fmt.Errorf("failed to clone config: %w", err)
	}

	configBytes, err := json.MarshalIndent(ownedConfig, "", "  ")
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
	fs.configObject = ownedConfig
	fs.config = string(configBytes)
	fs.mu.Unlock()

	return nil
}
