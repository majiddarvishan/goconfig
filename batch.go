package goconfig

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/iancoleman/orderedmap"
	"github.com/majiddarvishan/goconfig/history"
)

// Operation is one step of a Batch call. Index is used by insert, remove
// and update; Value by insert, replace and update.
type Operation struct {
	Op    string // "insert", "remove", "replace" or "update"
	Path  string
	Index int
	Value interface{}
}

// UpdatablePaths returns the arrays registered both insertable and
// removable - the ones Update (and a batch "update") works on.
func (m *Manager) UpdatablePaths() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]string, 0)
	seen := make(map[string]bool)
	for _, v := range m.modifiables {
		if v.Type == Insertable && !seen[v.Path] && m.isUpdatableLocked(v.Path) {
			seen[v.Path] = true
			out = append(out, v.Path)
		}
	}
	return out
}

func (m *Manager) isUpdatableLocked(path string) bool {
	_, insErr := m.findModifiableLocked(Insertable, path)
	_, remErr := m.findModifiableLocked(Removable, path)
	return insErr == nil && remErr == nil
}

// Update replaces the item at index of an array that is both insertable and
// removable. It needs no handler of its own: the existing OnRemove handler is
// called with the old item, then the OnInsert handler with the new one, at
// the same index. The schema is checked once on the final state, so a
// minItems limit never trips over the transient removal. If the insert
// handler fails, the old item is put back and handed to the insert handler
// again so the application can re-adopt it.
func (m *Manager) Update(path string, index int, value interface{}) error {
	return m.batch([]Operation{{Op: "update", Path: path, Index: index, Value: value}}, false, true)
}

// Batch applies ops in order as one unit: the schema is checked once, on the
// state after the last operation, and if any operation's handler fails, every
// operation already applied is undone in reverse order through the inverse
// operation's handler. Paths and indexes of each operation refer to the state
// left by the previous ones. With dryRun, nothing is applied and no handler
// runs - only structure, schema and registration (against the current
// registrations) are checked.
//
// Concurrency: holds the per-path lock of every path in ops for the whole
// call (see Insert).
func (m *Manager) Batch(ops []Operation, dryRun bool) error {
	return m.batch(ops, dryRun, false)
}

func (m *Manager) batch(ops []Operation, dryRun, single bool) error {
	wrap := func(i int, err error) error {
		if single {
			return err
		}
		return fmt.Errorf("batch operation #%d (%s %s): %w", i, ops[i].Op, ops[i].Path, err)
	}

	paths := make([]string, 0, len(ops))
	seen := make(map[string]bool)
	for _, op := range ops {
		if !seen[op.Path] {
			seen[op.Path] = true
			paths = append(paths, op.Path)
		}
	}
	sort.Strings(paths) // a fixed order, so two batches can't deadlock
	for _, p := range paths {
		pl := m.pathLock(p)
		pl.Lock()
		defer pl.Unlock()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	jsonConfig, err := Clone(m.source.getConfigObject())
	if err != nil {
		return fmt.Errorf("failed to clone config: %w", err)
	}
	for i, op := range ops {
		if err := applyToJSON(jsonConfig, op); err != nil {
			return wrap(i, err)
		}
	}

	configBytes, err := json.MarshalIndent(jsonConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := validateJSONBytesAgainstSchema(configBytes, m.source.getSchema()); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if dryRun {
		for i, op := range ops {
			if err := m.checkRegisteredLocked(op); err != nil {
				return wrap(i, err)
			}
		}
		return nil
	}

	undo := make([]Operation, 0, len(ops))
	events := make([]history.ChangeEvent, 0, len(ops))

	rollback := func(err error) error {
		for j := len(undo) - 1; j >= 0; j-- {
			if _, _, uerr := m.applyToNodesLocked(undo[j]); uerr != nil {
				err = fmt.Errorf("%w; undoing operation #%d failed: %v - the application state may no longer match the config", err, j, uerr)
			}
			m.invalidatePathCache()
			m.updateModifiablesLocked()
		}
		return err
	}

	for i, op := range ops {
		inverse, event, err := m.applyToNodesLocked(op)
		if err != nil {
			return rollback(wrap(i, err))
		}
		undo = append(undo, inverse)
		events = append(events, event)
		m.invalidatePathCache()
		m.updateModifiablesLocked()
	}

	if err := m.source.setConfig(jsonConfig, configBytes); err != nil {
		return rollback(fmt.Errorf("failed to persist config: %w", err))
	}

	m.version++
	m.invalidatePathCache()
	m.updateModifiablesLocked()
	for _, event := range events {
		event.Version = m.version
		m.addHistoryEvent(event)
	}
	return nil
}

func applyToJSON(om *orderedmap.OrderedMap, op Operation) error {
	switch op.Op {
	case "insert":
		return jsonInsertByPath(om, op.Path, op.Index, op.Value)
	case "remove":
		return jsonRemoveByPath(om, op.Path, op.Index)
	case "replace":
		return jsonSetByPath(om, op.Path, op.Value)
	case "update":
		if err := jsonRemoveByPath(om, op.Path, op.Index); err != nil {
			return err
		}
		return jsonInsertByPath(om, op.Path, op.Index, op.Value)
	}
	return fmt.Errorf("unsupported operation: %s", op.Op)
}

func (m *Manager) checkRegisteredLocked(op Operation) error {
	switch op.Op {
	case "insert":
		_, err := m.findModifiableLocked(Insertable, op.Path)
		return err
	case "remove":
		_, err := m.findModifiableLocked(Removable, op.Path)
		return err
	case "replace":
		_, err := m.findModifiableLocked(Replaceable, op.Path)
		return err
	case "update":
		ins, err := m.findModifiableLocked(Insertable, op.Path)
		if err != nil {
			return fmt.Errorf("path '%s' is not updatable (it must be both insertable and removable)", op.Path)
		}
		rem, err := m.findModifiableLocked(Removable, op.Path)
		if err != nil {
			return fmt.Errorf("path '%s' is not updatable (it must be both insertable and removable)", op.Path)
		}
		if ins.Node != rem.Node {
			return fmt.Errorf("path '%s' has insert and remove observers on different nodes", op.Path)
		}
		return nil
	}
	return fmt.Errorf("unsupported operation: %s", op.Op)
}

// callHandlerLocked runs handler with the global lock released, like
// insertLocked/removeLocked/replaceLocked do (see Manager's concurrency
// contract).
func (m *Manager) callHandlerLocked(handler handler_t, n *Node) error {
	if handler == nil {
		return nil
	}
	m.mu.Unlock()
	defer m.mu.Lock()
	return handler(n)
}

// applyToNodesLocked mutates the node tree for op and calls its handler(s);
// on a handler error the tree is left as it was before op. Returns the
// operation that undoes op, and op's history event (version not yet set).
func (m *Manager) applyToNodesLocked(op Operation) (Operation, history.ChangeEvent, error) {
	event := history.ChangeEvent{Timestamp: timeNow(), Operation: op.Op, Path: op.Path}
	if err := m.checkRegisteredLocked(op); err != nil {
		return Operation{}, event, err
	}

	switch op.Op {
	case "insert", "remove", "update":
		mod, _ := m.findModifiableLocked(Removable, op.Path)
		if op.Op == "insert" {
			mod, _ = m.findModifiableLocked(Insertable, op.Path)
		}
		node := mod.Node
		array, err := node.GetArray()
		if err != nil {
			return Operation{}, event, err
		}
		backup := make([]*Node, len(array))
		copy(backup, array)
		index := op.Index
		event.Index = &index

		if op.Op == "insert" {
			if index < 0 || index > len(array) {
				return Operation{}, event, fmt.Errorf("index %d out of bounds [0,%d]", index, len(array))
			}
			newNode := parseNode(op.Value)
			if err := m.customValidator.Validate(op.Path, nil, newNode); err != nil {
				return Operation{}, event, fmt.Errorf("custom validation failed: %w", err)
			}
			*node = Node{insertAt(backup, index, newNode)}
			if err := m.callHandlerLocked(mod.Handler, newNode); err != nil {
				*node = Node{backup}
				return Operation{}, event, err
			}
			event.NewValue = op.Value
			return Operation{Op: "remove", Path: op.Path, Index: index}, event, nil
		}

		if index < 0 || index >= len(array) {
			return Operation{}, event, fmt.Errorf("index %d out of bounds [0,%d)", index, len(array))
		}
		oldNode := backup[index]
		without := removeAt(backup, index)

		if op.Op == "remove" {
			*node = Node{without}
			if err := m.callHandlerLocked(mod.Handler, oldNode); err != nil {
				*node = Node{backup}
				return Operation{}, event, err
			}
			event.OldValue = nodeValueToPlain(oldNode)
			return Operation{Op: "insert", Path: op.Path, Index: index, Value: event.OldValue}, event, nil
		}

		// update: exactly what remove would do, then what insert would do.
		insertMod, _ := m.findModifiableLocked(Insertable, op.Path)
		insertHandler := insertMod.Handler
		newNode := parseNode(op.Value)
		if err := m.customValidator.Validate(op.Path, nil, newNode); err != nil {
			return Operation{}, event, fmt.Errorf("custom validation failed: %w", err)
		}

		*node = Node{without}
		if err := m.callHandlerLocked(mod.Handler, oldNode); err != nil {
			*node = Node{backup}
			return Operation{}, event, err
		}

		*node = Node{insertAt(without, index, newNode)}
		if err := m.callHandlerLocked(insertHandler, newNode); err != nil {
			// The application already dropped the old item - put it back
			// and let it adopt it again.
			*node = Node{backup}
			if rerr := m.callHandlerLocked(insertHandler, oldNode); rerr != nil {
				return Operation{}, event, fmt.Errorf("%w; restoring the previous item also failed: %v", err, rerr)
			}
			return Operation{}, event, err
		}
		event.OldValue = nodeValueToPlain(oldNode)
		event.NewValue = op.Value
		return Operation{Op: "update", Path: op.Path, Index: index, Value: event.OldValue}, event, nil

	case "replace":
		mod, _ := m.findModifiableLocked(Replaceable, op.Path)
		target := mod.Node
		newNode := parseNode(op.Value)
		if err := m.customValidator.Validate(op.Path, target, newNode); err != nil {
			return Operation{}, event, fmt.Errorf("custom validation failed: %w", err)
		}
		oldNode := *target
		*target = *newNode
		if err := m.callHandlerLocked(mod.Handler, target); err != nil {
			*target = oldNode
			return Operation{}, event, err
		}
		event.OldValue = nodeValueToPlain(&oldNode)
		event.NewValue = op.Value
		return Operation{Op: "replace", Path: op.Path, Value: event.OldValue}, event, nil
	}

	return Operation{}, event, fmt.Errorf("unsupported operation: %s", op.Op)
}

func insertAt(array []*Node, index int, n *Node) []*Node {
	out := make([]*Node, 0, len(array)+1)
	out = append(out, array[:index]...)
	out = append(out, n)
	return append(out, array[index:]...)
}

func removeAt(array []*Node, index int) []*Node {
	out := make([]*Node, 0, len(array)-1)
	out = append(out, array[:index]...)
	return append(out, array[index+1:]...)
}
