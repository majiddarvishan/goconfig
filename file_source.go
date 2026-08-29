package goconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/iancoleman/orderedmap"
)

type FileSource struct {
	mu           sync.RWMutex
	configPath   string
	configObject *orderedmap.OrderedMap
	config       string
	schema       string
	ops          fileSourceOps
}

type fileSourceOps struct {
	createTemp func(string, string) (*os.File, error)
	rename     func(string, string) error
	open       func(string) (*os.File, error)
	fileSync   func(*os.File) error
	dirSync    func(*os.File) error
}

func defaultFileSourceOps() fileSourceOps {
	return fileSourceOps{
		createTemp: os.CreateTemp,
		rename:     os.Rename,
		open:       os.Open,
		fileSync:   func(file *os.File) error { return file.Sync() },
		dirSync:    func(directory *os.File) error { return directory.Sync() },
	}
}

type committedFileWriteError struct {
	err error
}

func (e *committedFileWriteError) Error() string { return e.err.Error() }
func (e *committedFileWriteError) Unwrap() error { return e.err }
func (e *committedFileWriteError) committed()    {}

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
	err = fs.setConfig(object)
	var committed *committedFileWriteError
	if errors.As(err, &committed) {
		// Rename is the logical commit point. The public Source contract cannot
		// report failure after the replacement has already become visible.
		return nil
	}
	return err
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
		ops:          defaultFileSourceOps(),
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

	fs.mu.Lock()
	defer fs.mu.Unlock()

	info, err := os.Stat(fs.configPath)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}
	directoryPath := filepath.Dir(fs.configPath)
	baseName := filepath.Base(fs.configPath)
	temp, err := fs.ops.createTemp(directoryPath, "."+baseName+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := temp.Name()
	tempClosed := false
	defer func() {
		if !tempClosed {
			_ = temp.Close()
		}
		_ = os.Remove(tempPath)
	}()

	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return fmt.Errorf("failed to preserve config permissions: %w", err)
	}
	written, err := temp.Write(configBytes)
	if err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if written != len(configBytes) {
		return fmt.Errorf("failed to write temp file: %w", io.ErrShortWrite)
	}
	if err := fs.ops.fileSync(temp); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tempClosed = true

	directory, err := fs.ops.open(directoryPath)
	if err != nil {
		return fmt.Errorf("failed to open config directory: %w", err)
	}
	defer directory.Close()

	if err := fs.ops.rename(tempPath, fs.configPath); err != nil {
		return fmt.Errorf("failed to atomically replace config: %w", err)
	}

	// Rename is the commit point. Publish the matching source snapshot before
	// attempting the directory sync, whose failure cannot safely undo rename.
	fs.configObject = ownedConfig
	fs.config = string(configBytes)
	if err := fs.ops.dirSync(directory); err != nil {
		return &committedFileWriteError{err: fmt.Errorf("config replaced but directory sync failed: %w", err)}
	}

	return nil
}
