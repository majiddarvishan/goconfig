package goconfig

import (
	"context"
	"errors"
	"fmt"
)

// ChangeHandler is a compatibility pre-commit hook. Returning an error rejects
// the private candidate before persistence.
type ChangeHandler func(*Node) error

// ModificationType identifies a registered mutation capability.
type ModificationType int

const (
	Insertable ModificationType = iota
	Removable
	Replaceable
)

type modifiableType = ModificationType

type modifiable struct {
	Type    modifiableType
	Path    string
	Handler ChangeHandler
}

// Operation identifies a public mutation operation.
type Operation string

const (
	OperationInsert  Operation = "insert"
	OperationRemove  Operation = "remove"
	OperationReplace Operation = "replace"
)

// Mutation describes one transactional configuration change.
type Mutation struct {
	Operation       Operation
	Path            string
	Index           int
	Value           interface{}
	ExpectedVersion *int64
}

// Mutate applies a public mutation through the common transactional pipeline.
func (m *Manager) Mutate(ctx context.Context, mutation Mutation) error {
	var kind mutationKind
	switch mutation.Operation {
	case OperationInsert:
		kind = mutationInsert
	case OperationRemove:
		kind = mutationRemove
	case OperationReplace:
		kind = mutationReplace
	default:
		return fmt.Errorf("%w: unsupported operation %q", ErrInvalidMutation, mutation.Operation)
	}
	var expectedVersion *int64
	if mutation.ExpectedVersion != nil {
		version := *mutation.ExpectedVersion
		expectedVersion = &version
	}
	return m.mutate(ctx, mutationRequest{
		kind:            kind,
		path:            mutation.Path,
		index:           mutation.Index,
		value:           mutation.Value,
		expectedVersion: expectedVersion,
	})
}

// Insert inserts value into a registered array path.
func (m *Manager) Insert(path string, index int, value interface{}) error {
	return m.Mutate(context.Background(), Mutation{Operation: OperationInsert, Path: path, Index: index, Value: value})
}

// Remove removes an element from a registered array path.
func (m *Manager) Remove(path string, index int) error {
	return m.Mutate(context.Background(), Mutation{Operation: OperationRemove, Path: path, Index: index})
}

// Replace replaces a registered path value.
func (m *Manager) Replace(path string, value interface{}) error {
	return m.Mutate(context.Background(), Mutation{Operation: OperationReplace, Path: path, Value: value})
}

func (m *Manager) insert(path string, index int, value interface{}) error {
	return m.Insert(path, index, value)
}

func (m *Manager) remove(path string, index int) error { return m.Remove(path, index) }

func (m *Manager) replace(path string, value interface{}) error { return m.Replace(path, value) }

// OnInsert registers an array path and optional compatibility veto hook.
func (m *Manager) OnInsert(node *Node, handler ChangeHandler) error {
	return m.registerModification(Insertable, node, handler)
}

// OnRemove registers an array path and optional compatibility veto hook.
func (m *Manager) OnRemove(node *Node, handler ChangeHandler) error {
	return m.registerModification(Removable, node, handler)
}

// OnReplace registers a path and optional compatibility veto hook.
func (m *Manager) OnReplace(node *Node, handler ChangeHandler) error {
	return m.registerModification(Replaceable, node, handler)
}

func (m *Manager) registerModification(kind modifiableType, node *Node, handler ChangeHandler) error {
	if node == nil {
		return errors.New("node cannot be nil")
	}
	if (kind == Insertable || kind == Removable) && node.Type() != Array {
		return errors.New("node must be array for insert/remove operations")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	path, err := m.findAndSanitizeNodePathLocked(node)
	if err != nil {
		return err
	}
	current, err := nodeAtJSONPointer(m.config, path)
	if err != nil {
		return fmt.Errorf("registered node is no longer present: %w", err)
	}
	if (kind == Insertable || kind == Removable) && current.Type() != Array {
		return errors.New("node must be array for insert/remove operations")
	}
	m.modifiables = append(m.modifiables, modifiable{Type: kind, Path: path, Handler: handler})
	return nil
}

func (m *Manager) findModifiableLocked(kind modifiableType, path string) (*modifiable, error) {
	for index := range m.modifiables {
		if m.modifiables[index].Type == kind && m.modifiables[index].Path == path {
			return &m.modifiables[index], nil
		}
	}
	return nil, fmt.Errorf("%w: path %q for operation type %d", ErrMutationDenied, path, kind)
}

func (m *Manager) updateModifiablesLocked() {
	valid := make([]modifiable, 0, len(m.modifiables))
	for _, modification := range m.modifiables {
		node, err := nodeAtJSONPointer(m.config, modification.Path)
		if err != nil {
			continue
		}
		if (modification.Type == Insertable || modification.Type == Removable) && node.Type() != Array {
			continue
		}
		valid = append(valid, modification)
	}
	m.modifiables = valid
}

func (m *Manager) findNodePathLocked(node *Node) string {
	if node != nil && node.owner == m {
		if _, err := nodeAtJSONPointer(m.config, node.path); err == nil {
			return node.path
		}
	}
	return findNodePath(m.config, node)
}

func (m *Manager) findAndSanitizeNodePathLocked(node *Node) (string, error) {
	if node != nil && node.owner == m {
		if _, err := nodeAtJSONPointer(m.config, node.path); err != nil {
			return "", errors.New("node has no valid path in config tree")
		}
		return node.path, nil
	}
	path := m.findNodePathLocked(node)
	if path == "" && node != m.config {
		return "", errors.New("node has no valid path in config tree")
	}
	return path, nil
}
