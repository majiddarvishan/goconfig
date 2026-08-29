package goconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/iancoleman/orderedmap"
	"github.com/majiddarvishan/goconfig/history"
	"github.com/xeipuuv/gojsonschema"
)

// ChangeHandler is a legacy-compatible pre-commit hook. Returning an error
// rejects the private candidate before persistence.
type ChangeHandler func(*Node) error

type modifiableType int

const (
	Insertable modifiableType = iota
	Removable
	Replaceable
)

type modifiable struct {
	Type    modifiableType
	Path    string
	Handler ChangeHandler
}

type Manager struct {
	mu sync.RWMutex

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

	// Http Server
	httpServer *HttpServer

	// Post-commit notifications
	observers []ChangeObserver
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

func (m *Manager) Source() ISource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.source
}

func (m *Manager) Version() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.version
}

////////////////////////////////////////////////////////////////////////////////
// HISTORY
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) EnableHistory(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.historyEnabled = enabled
}

func (m *Manager) GetHistory() []history.ChangeEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.history.GetAll()
}

func (m *Manager) GetHistoryByPath(path string, limit int) []history.ChangeEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.history.GetByPath(path, limit)
}

func (m *Manager) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.history.Clear()
}

func (m *Manager) addHistoryEvent(event history.ChangeEvent) {
	if m.historyEnabled && m.history != nil {
		m.history.Add(event)
	}
}

////////////////////////////////////////////////////////////////////////////////
// VALIDATION
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) SetValidationService(service *ValidationService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validationService = service
}

func (m *Manager) AddValidator(path string, validator Validator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customValidator.AddValidator(path, validator)
}

func (m *Manager) GetCustomValidator() *CustomValidator {
	return m.customValidator
}

////////////////////////////////////////////////////////////////////////////////
// HTTP Server
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) NewHttpServerFromNode(conf *Node) error {
	var err error
	m.httpServer, err = newHttpServerFromNode(m, conf)
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) NewHttpServer(opts ...HttpServerOption) error {
	var err error
	m.httpServer, err = newHttpServer(m, opts...)
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) StartHttpServer() {
	go func() {
		if err := m.httpServer.Start(); err != nil {
			panic(err)
		}
	}()
}

func (m *Manager) StopHttpServer() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.httpServer.Shutdown(ctx); err != nil {
		fmt.Printf("Shutdown error: %v", err)
	}
}

func (m *Manager) SetupRoutes(r RouteRegistrar) {
	m.httpServer.registerRoutes(r)
}

////////////////////////////////////////////////////////////////////////////////
// INSERT
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) insert(path string, index int, value interface{}) error {
	return m.mutate(context.Background(), mutationRequest{
		kind:  mutationInsert,
		path:  path,
		index: index,
		value: value,
	})
}

////////////////////////////////////////////////////////////////////////////////
// REMOVE
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) remove(path string, index int) error {
	return m.mutate(context.Background(), mutationRequest{
		kind:  mutationRemove,
		path:  path,
		index: index,
	})
}

////////////////////////////////////////////////////////////////////////////////
// REPLACE
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) replace(path string, value interface{}) error {
	return m.mutate(context.Background(), mutationRequest{
		kind:  mutationReplace,
		path:  path,
		value: value,
	})
}

////////////////////////////////////////////////////////////////////////////////
// REGISTRATION
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) OnInsert(node *Node, handler ChangeHandler) error {
	if node == nil {
		return errors.New("node cannot be nil")
	}
	if node.Type() != Array {
		return errors.New("node must be array for insert operations")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	p, err := m.findAndSanitizeNodePathLocked(node)
	if err != nil {
		return err
	}
	current, err := nodeAtJSONPointer(m.config, p)
	if err != nil {
		return fmt.Errorf("registered node is no longer present: %w", err)
	}
	if current.Type() != Array {
		return errors.New("node must be array for insert operations")
	}

	m.modifiables = append(m.modifiables, modifiable{
		Type:    Insertable,
		Path:    p,
		Handler: handler,
	})

	return nil
}

func (m *Manager) OnRemove(node *Node, handler ChangeHandler) error {
	if node == nil {
		return errors.New("node cannot be nil")
	}
	if node.Type() != Array {
		return errors.New("node must be array for remove operations")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	p, err := m.findAndSanitizeNodePathLocked(node)
	if err != nil {
		return err
	}
	current, err := nodeAtJSONPointer(m.config, p)
	if err != nil {
		return fmt.Errorf("registered node is no longer present: %w", err)
	}
	if current.Type() != Array {
		return errors.New("node must be array for remove operations")
	}

	m.modifiables = append(m.modifiables, modifiable{
		Type:    Removable,
		Path:    p,
		Handler: handler,
	})

	return nil
}

func (m *Manager) OnReplace(node *Node, handler ChangeHandler) error {
	if node == nil {
		return errors.New("node cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	p, err := m.findAndSanitizeNodePathLocked(node)
	if err != nil {
		return err
	}
	if _, err := nodeAtJSONPointer(m.config, p); err != nil {
		return fmt.Errorf("registered node is no longer present: %w", err)
	}

	m.modifiables = append(m.modifiables, modifiable{
		Type:    Replaceable,
		Path:    p,
		Handler: handler,
	})

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// PATH HELPERS
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) findModifiableLocked(t modifiableType, path string) (*modifiable, error) {
	for i := range m.modifiables {
		if m.modifiables[i].Type == t && m.modifiables[i].Path == path {
			return &m.modifiables[i], nil
		}
	}
	return nil, fmt.Errorf("path '%s' not modifiable for operation type %d", path, t)
}

func (m *Manager) updateModifiablesLocked() {
	validMods := make([]modifiable, 0, len(m.modifiables))
	for _, mod := range m.modifiables {
		node, err := nodeAtJSONPointer(m.config, mod.Path)
		if err != nil {
			continue
		}
		if (mod.Type == Insertable || mod.Type == Removable) && node.Type() != Array {
			continue
		}
		validMods = append(validMods, mod)
	}
	m.modifiables = validMods
}

func (m *Manager) findNodePathLocked(n *Node) string {
	if n != nil && n.owner == m {
		if _, err := nodeAtJSONPointer(m.config, n.path); err == nil {
			return n.path
		}
	}
	return findNodePath(m.config, n)
}

func (m *Manager) findAndSanitizeNodePathLocked(n *Node) (string, error) {
	if n != nil && n.owner == m {
		if _, err := nodeAtJSONPointer(m.config, n.path); err != nil {
			return "", errors.New("node has no valid path in config tree")
		}
		return n.path, nil
	}
	p := m.findNodePathLocked(n)
	if p == "" && n != m.config {
		return "", errors.New("node has no valid path in config tree")
	}
	return p, nil
}

////////////////////////////////////////////////////////////////////////////////
// HELPERS
////////////////////////////////////////////////////////////////////////////////

// Helper for testing/mocking time
var timeNow = func() time.Time {
	return time.Now()
}
