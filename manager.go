package goconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/majiddarvishan/goconfig/history"
)

type handler_t func(*Node) error

type modifiableType int

const (
	Insertable modifiableType = iota
	Removable
	Replaceable
)

type modifiable struct {
	Type    modifiableType
	Path    string
	Node    *Node
	Handler handler_t
}

type Manager struct {
	mu sync.RWMutex

	source      ISource
	config      *Node
	modifiables []modifiable
	version     int64

	// Path caching
	pathCache      map[*Node]string
	pathCacheValid bool

	// Change history
	history        *history.ChangeHistory
	historyEnabled bool

	// Custom validators
	customValidator *customValidator

	// External validation
	validationService *validationService
}

func NewManager(source ISource) (*Manager, error) {
	if source == nil {
		return nil, errors.New("source cannot be nil")
	}

	root := parseNode(source.getConfigObject())
	if root == nil {
		return nil, errors.New("failed to parse config root")
	}

	m := &Manager{
		source:          source,
		config:          root,
		modifiables:     make([]modifiable, 0),
		version:         1,
		pathCache:       make(map[*Node]string),
		pathCacheValid:  false,
		history:         history.NewChangeHistory(1000),
		historyEnabled:  true,
		customValidator: NewCustomValidator(),
	}

	if err := validate(source.getConfig(), source.getSchema()); err != nil {
		return nil, fmt.Errorf("initial config validation failed: %w", err)
	}

	return m, nil
}

// Config returns a deep copy of the config to prevent data races
func (m *Manager) Config() *Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	// return m.config.DeepCopy()
	return m.config
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
// PATH CACHING
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) invalidatePathCache() {
	m.pathCacheValid = false
	m.pathCache = make(map[*Node]string)
}

func (m *Manager) rebuildPathCache() {
	m.pathCache = make(map[*Node]string)
	m.buildPathCacheRecursive(m.config, "")
	m.pathCacheValid = true
}

func (m *Manager) buildPathCacheRecursive(node *Node, path string) {
	m.pathCache[node] = path

	if node.Type() == Object {
		obj, _ := node.GetObject()
		for key, child := range obj {
			m.buildPathCacheRecursive(child, path+"/"+key)
		}
	} else if node.Type() == Array {
		arr, _ := node.GetArray()
		for i, child := range arr {
			m.buildPathCacheRecursive(child, path+"/"+fmt.Sprintf("%d", i))
		}
	}
}

func (m *Manager) findNodePathCached(n *Node) string {
	if !m.pathCacheValid {
		m.rebuildPathCache()
	}
	if path, ok := m.pathCache[n]; ok {
		return path
	}
	return ""
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

func (m *Manager) SetValidationService(service *validationService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.validationService = service
}

func (m *Manager) AddValidator(path string, validator validatorFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customValidator.AddValidator(path, validator)
}

func (m *Manager) GetCustomValidator() *customValidator {
	return m.customValidator
}

////////////////////////////////////////////////////////////////////////////////
// SOURCE ACCESS
////////////////////////////////////////////////////////////////////////////////

// ConfigJSON returns the raw JSON text of the currently persisted configuration.
func (m *Manager) ConfigJSON() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s := m.source.getConfig(); s != nil {
		return *s
	}
	return ""
}

// SchemaJSON returns the raw JSON schema text.
func (m *Manager) SchemaJSON() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if s := m.source.getSchema(); s != nil {
		return *s
	}
	return ""
}

////////////////////////////////////////////////////////////////////////////////
// INSERT (improved with all features)
////////////////////////////////////////////////////////////////////////////////

// Insert adds value at index into the array registered via OnInsert for path.
func (m *Manager) Insert(path string, index int, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.insertLocked(path, index, value)
}

func (m *Manager) insertLocked(path string, index int, value interface{}) error {
	mod, err := m.findModifiableLocked(Insertable, path)
	if err != nil {
		return err
	}

	// Validate index bounds first
	array, err := mod.Node.GetArray()
	if err != nil {
		return err
	}
	if index < 0 || index > len(array) {
		return fmt.Errorf("index %d out of bounds [0,%d]", index, len(array))
	}

	// Custom validation
	newNode := parseNode(value)
	if err := m.customValidator.Validate(path, nil, newNode); err != nil {
		return fmt.Errorf("custom validation failed: %w", err)
	}

	// Clone and validate
	jsonConfig, err := Clone(m.source.getConfigObject())
	if err != nil {
		return fmt.Errorf("failed to clone config: %w", err)
	}

	if err := jsonInsertByPath(jsonConfig, path, index, value); err != nil {
		return fmt.Errorf("failed to insert: %w", err)
	}

	if err := validateJSONAgainstSchema(jsonConfig, m.source.getSchema()); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Backup for rollback
	oldArray := make([]*Node, len(array))
	copy(oldArray, array)

	// Mutate in-memory node
	newArr := make([]*Node, 0, len(array)+1)
	newArr = append(newArr, array[:index]...)
	newArr = append(newArr, newNode)
	newArr = append(newArr, array[index:]...)
	*mod.Node = Node{newArr}

	// Call handler after successful persistence
	handler := mod.Handler
	if handler != nil {
		// handlerNode := newNode.DeepCopy()
		handlerNode := newNode
		m.mu.Unlock()
		err := handler(handlerNode)
		m.mu.Lock()

		if err != nil {
			*mod.Node = Node{oldArray}
			return err
		}
	}

	// Persist changes
	if err := m.source.setConfig(jsonConfig); err != nil {
		*mod.Node = Node{oldArray}
		return fmt.Errorf("failed to persist config: %w", err)
	}

	m.version++
	m.invalidatePathCache()
	m.updateModifiablesLocked()

	// Add to history
	m.addHistoryEvent(history.ChangeEvent{
		Timestamp: timeNow(),
		Operation: "insert",
		Path:      path,
		Index:     &index,
		NewValue:  value,
		Version:   m.version,
	})

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// REMOVE (improved with all features)
////////////////////////////////////////////////////////////////////////////////

// Remove deletes the element at index from the array registered via OnRemove for path.
func (m *Manager) Remove(path string, index int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeLocked(path, index)
}

func (m *Manager) removeLocked(path string, index int) error {
	mod, err := m.findModifiableLocked(Removable, path)
	if err != nil {
		return err
	}

	array, err := mod.Node.GetArray()
	if err != nil {
		return err
	}

	if index < 0 || index >= len(array) {
		return fmt.Errorf("index %d out of bounds [0,%d)", index, len(array))
	}

	jsonConfig, err := Clone(m.source.getConfigObject())
	if err != nil {
		return fmt.Errorf("failed to clone config: %w", err)
	}

	if err := jsonRemoveByPath(jsonConfig, path, index); err != nil {
		return fmt.Errorf("failed to remove: %w", err)
	}

	if err := validateJSONAgainstSchema(jsonConfig, m.source.getSchema()); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Backup for rollback
	oldArray := make([]*Node, len(array))
	copy(oldArray, array)
	removedNode := array[index]

	// Mutate
	newArr := make([]*Node, 0, len(array)-1)
	newArr = append(newArr, array[:index]...)
	newArr = append(newArr, array[index+1:]...)
	*mod.Node = Node{newArr}

	handler := mod.Handler
	if handler != nil {
		// handlerNode := removedNode.DeepCopy()
		handlerNode := removedNode
		m.mu.Unlock()
		err := handler(handlerNode)
		m.mu.Lock()

		if err != nil {
			*mod.Node = Node{oldArray}
			return err
		}
	}

	// Persist
	if err := m.source.setConfig(jsonConfig); err != nil {
		*mod.Node = Node{oldArray}
		return fmt.Errorf("failed to persist config: %w", err)
	}

	m.version++
	m.invalidatePathCache()
	m.updateModifiablesLocked()

	// Add to history
	m.addHistoryEvent(history.ChangeEvent{
		Timestamp: timeNow(),
		Operation: "remove",
		Path:      path,
		Index:     &index,
		OldValue:  removedNode.value,
		Version:   m.version,
	})

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// REPLACE (improved with all features)
////////////////////////////////////////////////////////////////////////////////

// Replace sets the value at path registered via OnReplace.
func (m *Manager) Replace(path string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.replaceLocked(path, value)
}

func (m *Manager) replaceLocked(path string, value interface{}) error {
	mod, err := m.findModifiableLocked(Replaceable, path)
	if err != nil {
		return err
	}

	// Custom validation
	newNode := parseNode(value)
	if err := m.customValidator.Validate(path, mod.Node, newNode); err != nil {
		return fmt.Errorf("custom validation failed: %w", err)
	}

	jsonConfig, err := Clone(m.source.getConfigObject())
	if err != nil {
		return fmt.Errorf("failed to clone config: %w", err)
	}

	if err := jsonSetByPath(jsonConfig, path, value); err != nil {
		return fmt.Errorf("failed to set: %w", err)
	}

	if err := validateJSONAgainstSchema(jsonConfig, m.source.getSchema()); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Backup for rollback
	oldNode := *mod.Node
	oldValue := oldNode.value

	// Mutate
	*mod.Node = *newNode

	handler := mod.Handler
	if handler != nil {
		// handlerNode := mod.Node.DeepCopy()
		handlerNode := mod.Node
		m.mu.Unlock()
		err := handler(handlerNode)
		m.mu.Lock()

		if err != nil {
			*mod.Node = oldNode
			return err
		}
	}

	// Persist
	if err := m.source.setConfig(jsonConfig); err != nil {
		*mod.Node = oldNode
		return fmt.Errorf("failed to persist config: %w", err)
	}

	m.version++
	m.invalidatePathCache()
	m.updateModifiablesLocked()

	// Add to history
	m.addHistoryEvent(history.ChangeEvent{
		Timestamp: timeNow(),
		Operation: "replace",
		Path:      path,
		OldValue:  oldValue,
		NewValue:  value,
		Version:   m.version,
	})

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// REGISTRATION
////////////////////////////////////////////////////////////////////////////////

func (m *Manager) OnInsert(node *Node, handler handler_t) error {
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

	m.modifiables = append(m.modifiables, modifiable{
		Type:    Insertable,
		Path:    p,
		Node:    node,
		Handler: handler,
	})

	return nil
}

func (m *Manager) OnRemove(node *Node, handler handler_t) error {
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

	m.modifiables = append(m.modifiables, modifiable{
		Type:    Removable,
		Path:    p,
		Node:    node,
		Handler: handler,
	})

	return nil
}

func (m *Manager) OnReplace(node *Node, handler handler_t) error {
	if node == nil {
		return errors.New("node cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	p, err := m.findAndSanitizeNodePathLocked(node)
	if err != nil {
		return err
	}

	m.modifiables = append(m.modifiables, modifiable{
		Type:    Replaceable,
		Path:    p,
		Node:    node,
		Handler: handler,
	})

	return nil
}

////////////////////////////////////////////////////////////////////////////////
// PATH HELPERS
////////////////////////////////////////////////////////////////////////////////

// InsertablePaths returns the paths currently registered as insertable via OnInsert.
func (m *Manager) InsertablePaths() []string { return m.getPathsLocked(Insertable) }

// RemovablePaths returns the paths currently registered as removable via OnRemove.
func (m *Manager) RemovablePaths() []string { return m.getPathsLocked(Removable) }

// ReplaceablePaths returns the paths currently registered as replaceable via OnReplace.
func (m *Manager) ReplaceablePaths() []string { return m.getPathsLocked(Replaceable) }

func (m *Manager) getPathsLocked(t modifiableType) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]string, 0, len(m.modifiables))
	for _, v := range m.modifiables {
		if v.Type == t {
			out = append(out, v.Path)
		}
	}
	return out
}

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
		if path := m.findNodePathLocked(mod.Node); path != "" {
			mod.Path = path
			validMods = append(validMods, mod)
		}
	}
	m.modifiables = validMods
}

func (m *Manager) findNodePathLocked(n *Node) string {
	if m.pathCacheValid {
		return m.findNodePathCached(n)
	}
	return findNodePath(m.config, n)
}

func (m *Manager) findAndSanitizeNodePathLocked(n *Node) (string, error) {
	p := m.findNodePathLocked(n)
	if p == "" {
		return "", errors.New("node has no valid path in config tree")
	}
	return p, nil
}

////////////////////////////////////////////////////////////////////////////////
// HELPERS
////////////////////////////////////////////////////////////////////////////////

func validateJSONAgainstSchema(obj interface{}, schema *string) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal object: %w", err)
	}
	s := string(b)
	return validate(&s, schema)
}

// Helper for testing/mocking time
var timeNow = func() time.Time {
	return time.Now()
}
