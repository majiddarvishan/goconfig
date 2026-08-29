package goconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/iancoleman/orderedmap"
	"github.com/majiddarvishan/goconfig/history"
	"github.com/xeipuuv/gojsonschema"
)

type Manager struct {
	mu     sync.RWMutex
	httpMu sync.RWMutex

	source         ISource
	config         *Node
	configObject   *orderedmap.OrderedMap
	schemaJSON     string
	compiledSchema *gojsonschema.Schema
	modifiables    []modifiable
	version        int64

	// Change history
	history        *history.ChangeHistory
	historyEnabled bool

	// Custom validators
	customValidator *CustomValidator

	// External validation
	validationService *ValidationService

	// HTTP server
	httpServer *HTTPServer

	// Post-commit notifications
	observers []ChangeObserver
}

// Snapshot is an independently owned serialized Manager state.
type Snapshot struct {
	Config  []byte
	Schema  []byte
	Version int64
}

func NewManager(source ISource) (*Manager, error) {
	return NewManagerWithOptions(source)
}

// NewManagerWithOptions constructs a Manager with explicit options.
func NewManagerWithOptions(source ISource, options ...ManagerOption) (*Manager, error) {
	if source == nil {
		return nil, errors.New("source cannot be nil")
	}
	settings, err := applyManagerOptions(options)
	if err != nil {
		return nil, err
	}

	configObject, err := Clone(source.getConfigObject())
	if err != nil {
		return nil, fmt.Errorf("failed to clone initial config: %w", err)
	}
	schema := source.getSchema()
	compiledSchema, err := compileSchema(schema)
	if err != nil {
		return nil, fmt.Errorf("initial config validation failed: %w", err)
	}
	configBytes, err := json.Marshal(configObject)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal initial config: %w", err)
	}
	if err := validateWithSchema(compiledSchema, configBytes); err != nil {
		return nil, fmt.Errorf("initial config validation failed: %w", err)
	}

	root, err := parseNodeStrict(configObject)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config root: %w", err)
	}

	m := &Manager{
		source:          source,
		config:          root,
		configObject:    configObject,
		schemaJSON:      *schema,
		compiledSchema:  compiledSchema,
		modifiables:     make([]modifiable, 0),
		version:         1,
		history:         history.NewChangeHistory(settings.historyCapacity),
		historyEnabled:  true,
		customValidator: NewCustomValidator(),
	}
	bindNodeTree(root, m, "")

	return m, nil
}

// NewManagerFromSource constructs a Manager from the public, externally
// implementable Source contract.
func NewManagerFromSource(source Source) (*Manager, error) {
	return NewManagerFromSourceWithOptions(source)
}

// NewManagerFromSourceWithOptions constructs a Manager from a public Source
// with explicit options.
func NewManagerFromSourceWithOptions(source Source, options ...ManagerOption) (*Manager, error) {
	adapter, err := newSourceAdapter(source)
	if err != nil {
		return nil, err
	}
	return NewManagerWithOptions(adapter, options...)
}

// Config returns an independent snapshot of the current config.
func (m *Manager) Config() *Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config.DeepCopy()
}

// Source returns the legacy persistence implementation.
// Deprecated: use Snapshot for public state reads.
func (m *Manager) Source() ISource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.source
}

// Snapshot returns configuration, schema, and version from one Manager lock.
func (m *Manager) Snapshot() (Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	config, err := json.Marshal(m.configObject)
	if err != nil {
		return Snapshot{}, fmt.Errorf("failed to marshal config snapshot: %w", err)
	}
	return Snapshot{
		Config:  append([]byte(nil), config...),
		Schema:  []byte(m.schemaJSON),
		Version: m.version,
	}, nil
}

func (m *Manager) Version() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}
