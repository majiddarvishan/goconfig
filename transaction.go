package goconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/iancoleman/orderedmap"
	"github.com/majiddarvishan/goconfig/history"
)

type mutationRequest struct {
	kind            mutationKind
	path            string
	index           int
	value           interface{}
	expectedVersion *int64
}

// Change is an immutable description delivered to post-commit observers.
type Change struct {
	Operation string
	Path      string
	Index     *int
	OldValue  *Node
	NewValue  *Node
	Version   int64
}

// ChangeObserver is invoked after a successful commit without Manager locks.
// Observers cannot reject or roll back an already committed change.
type ChangeObserver func(Change)

// AddObserver registers a post-commit observer.
func (m *Manager) AddObserver(observer ChangeObserver) {
	if observer == nil {
		return
	}
	m.mu.Lock()
	m.observers = append(m.observers, observer)
	m.mu.Unlock()
}

func (m *Manager) mutate(ctx context.Context, request mutationRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	segments, err := parseJSONPointer(request.path)
	if err != nil {
		return &PathError{Path: request.path, Err: ErrInvalidPath, Msg: err.Error()}
	}
	canonicalPath := buildJSONPointer(segments)

	m.mu.RLock()
	baseVersion := m.version
	if request.expectedVersion != nil && *request.expectedVersion != baseVersion {
		m.mu.RUnlock()
		return &VersionConflictError{Expected: *request.expectedVersion, Current: baseVersion}
	}
	modifiableType, err := mutationModifiableType(request.kind)
	if err != nil {
		m.mu.RUnlock()
		return err
	}
	registered, err := m.findModifiableLocked(modifiableType, canonicalPath)
	if err != nil {
		m.mu.RUnlock()
		return err
	}
	modifiable := *registered
	candidate, err := Clone(m.configObject)
	if err != nil {
		m.mu.RUnlock()
		return fmt.Errorf("failed to clone config snapshot: %w", err)
	}
	compiledSchema := m.compiledSchema
	schemaJSON := m.schemaJSON
	externalValidator := m.validationService
	m.mu.RUnlock()

	change, err := applyMutation(candidate, request.kind, canonicalPath, request.index, request.value)
	if err != nil {
		return err
	}
	oldNode, err := optionalNode(change.oldValue)
	if err != nil {
		return fmt.Errorf("failed to parse previous value: %w", err)
	}
	newNode, err := optionalNode(change.newValue)
	if err != nil {
		return fmt.Errorf("failed to parse candidate value: %w", err)
	}

	candidateJSON, err := json.Marshal(candidate)
	if err != nil {
		return fmt.Errorf("failed to marshal candidate config: %w", err)
	}
	if err := validateWithSchema(compiledSchema, candidateJSON); err != nil {
		return &ValidationError{Stage: "schema", Err: err}
	}
	if err := m.customValidator.Validate(canonicalPath, copyOptionalNode(oldNode), copyOptionalNode(newNode)); err != nil {
		return &ValidationError{Stage: "custom", Err: err}
	}
	if externalValidator != nil {
		var schema interface{}
		if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
			return &ValidationError{Stage: "external", Err: fmt.Errorf("invalid stored schema: %w", err)}
		}
		if err := externalValidator.Validate(ctx, candidate, schema); err != nil {
			return &ValidationError{Stage: "external", Err: err}
		}
	}

	if modifiable.Handler != nil {
		handlerNode := newNode
		if request.kind == mutationRemove {
			handlerNode = oldNode
		}
		if err := modifiable.Handler(copyOptionalNode(handlerNode)); err != nil {
			return err
		}
	}

	candidateRoot, err := parseNodeStrict(candidate)
	if err != nil {
		return fmt.Errorf("failed to build candidate node tree: %w", err)
	}
	historyOldValue, err := cloneOptionalJSONValue(change.oldValue)
	if err != nil {
		return fmt.Errorf("failed to snapshot previous history value: %w", err)
	}
	historyNewValue, err := cloneOptionalJSONValue(change.newValue)
	if err != nil {
		return fmt.Errorf("failed to snapshot new history value: %w", err)
	}

	m.mu.Lock()
	if m.version != baseVersion {
		current := m.version
		m.mu.Unlock()
		return &VersionConflictError{Expected: baseVersion, Current: current}
	}
	if err := m.source.setConfig(candidate); err != nil {
		m.mu.Unlock()
		return &PersistenceError{Err: err}
	}

	m.configObject = candidate
	m.config = candidateRoot
	bindNodeTree(candidateRoot, m, "")
	m.shiftModifiablePathsLocked(request.kind, canonicalPath, request.index)
	m.updateModifiablesLocked()
	m.version++
	committedVersion := m.version

	historyIndex := optionalHistoryIndex(request.kind, request.index)
	m.addHistoryEvent(history.ChangeEvent{
		Timestamp: timeNow(),
		Operation: string(request.kind),
		Path:      canonicalPath,
		Index:     historyIndex,
		OldValue:  historyOldValue,
		NewValue:  historyNewValue,
		Version:   committedVersion,
	})
	observers := append([]ChangeObserver(nil), m.observers...)
	m.mu.Unlock()

	observerChange := Change{
		Operation: string(request.kind),
		Path:      canonicalPath,
		Index:     optionalHistoryIndex(request.kind, request.index),
		OldValue:  copyOptionalNode(oldNode),
		NewValue:  copyOptionalNode(newNode),
		Version:   committedVersion,
	}
	for _, observer := range observers {
		changeCopy := observerChange
		changeCopy.Index = copyOptionalIndex(observerChange.Index)
		changeCopy.OldValue = copyOptionalNode(observerChange.OldValue)
		changeCopy.NewValue = copyOptionalNode(observerChange.NewValue)
		observer(changeCopy)
	}
	return nil
}

func mutationModifiableType(kind mutationKind) (modifiableType, error) {
	switch kind {
	case mutationInsert:
		return Insertable, nil
	case mutationRemove:
		return Removable, nil
	case mutationReplace:
		return Replaceable, nil
	default:
		return 0, fmt.Errorf("unsupported mutation kind %q", kind)
	}
}

func optionalNode(value interface{}) (*Node, error) {
	if value == nil {
		return nil, nil
	}
	return parseNodeStrict(value)
}

func copyOptionalNode(node *Node) *Node {
	if node == nil {
		return nil
	}
	return node.DeepCopy()
}

func cloneOptionalJSONValue(value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	return cloneJSONValue(value)
}

func optionalHistoryIndex(kind mutationKind, index int) *int {
	if kind != mutationInsert && kind != mutationRemove {
		return nil
	}
	copyIndex := index
	return &copyIndex
}

func copyOptionalIndex(index *int) *int {
	if index == nil {
		return nil
	}
	copyIndex := *index
	return &copyIndex
}

func (m *Manager) shiftModifiablePathsLocked(kind mutationKind, arrayPath string, changedIndex int) {
	if kind != mutationInsert && kind != mutationRemove {
		return
	}
	arraySegments, err := parseJSONPointer(arrayPath)
	if err != nil {
		return
	}

	shifted := make([]modifiable, 0, len(m.modifiables))
	for _, modifiable := range m.modifiables {
		segments, err := parseJSONPointer(modifiable.Path)
		if err != nil || len(segments) <= len(arraySegments) || !equalPathPrefix(segments, arraySegments) {
			shifted = append(shifted, modifiable)
			continue
		}

		index, err := parseCanonicalArrayIndex(segments[len(arraySegments)])
		if err != nil {
			shifted = append(shifted, modifiable)
			continue
		}
		switch kind {
		case mutationInsert:
			if index >= changedIndex {
				segments[len(arraySegments)] = strconv.Itoa(index + 1)
				modifiable.Path = buildJSONPointer(segments)
			}
			shifted = append(shifted, modifiable)
		case mutationRemove:
			if index == changedIndex {
				continue
			}
			if index > changedIndex {
				segments[len(arraySegments)] = strconv.Itoa(index - 1)
				modifiable.Path = buildJSONPointer(segments)
			}
			shifted = append(shifted, modifiable)
		}
	}
	m.modifiables = shifted
}

func equalPathPrefix(path, prefix []string) bool {
	if len(path) < len(prefix) {
		return false
	}
	for index := range prefix {
		if path[index] != prefix[index] {
			return false
		}
	}
	return true
}

type managerSnapshot struct {
	config      *orderedmap.OrderedMap
	schema      string
	insertable  []string
	removable   []string
	replaceable []string
	version     int64
}

func (m *Manager) snapshot() (managerSnapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	config, err := Clone(m.configObject)
	if err != nil {
		return managerSnapshot{}, err
	}
	result := managerSnapshot{
		config:  config,
		schema:  m.schemaJSON,
		version: m.version,
	}
	for _, modifiable := range m.modifiables {
		switch modifiable.Type {
		case Insertable:
			result.insertable = append(result.insertable, modifiable.Path)
		case Removable:
			result.removable = append(result.removable, modifiable.Path)
		case Replaceable:
			result.replaceable = append(result.replaceable, modifiable.Path)
		}
	}
	return result, nil
}
