package goconfig

import (
	"errors"
	"testing"
)

func TestManagerInsertPersistsVersionsAndRecordsHistory(t *testing.T) {
	manager, source := newTestManager(t)
	items := mustNodeAt(t, manager.Config(), "items")

	var handledName string
	if err := manager.OnInsert(items, func(node *Node) error {
		var err error
		handledName, err = node.GetString("name")
		return err
	}); err != nil {
		t.Fatalf("OnInsert() error = %v", err)
	}

	value := map[string]interface{}{"id": float64(3), "name": "three"}
	if err := manager.insert("/items", 1, value); err != nil {
		t.Fatalf("insert() error = %v", err)
	}

	if handledName != "three" {
		t.Fatalf("handler name = %q, want three", handledName)
	}
	if manager.Version() != 2 {
		t.Fatalf("version = %d, want 2", manager.Version())
	}
	if got, err := mustNodeAt(t, manager.Config(), "items", 1, "name").GetString(); err != nil || got != "three" {
		t.Fatalf("inserted node name = %q, %v", got, err)
	}

	persistedItems, _ := decodeSourceConfig(t, source).Get("items")
	if got := len(persistedItems.([]interface{})); got != 3 {
		t.Fatalf("persisted items length = %d, want 3", got)
	}

	history := manager.GetHistory()
	if len(history) != 1 || history[0].Operation != "insert" || history[0].Version != 2 {
		t.Fatalf("history = %#v", history)
	}
}

func TestManagerRemovePersistsChange(t *testing.T) {
	manager, source := newTestManager(t)
	items := mustNodeAt(t, manager.Config(), "items")

	var removedName string
	if err := manager.OnRemove(items, func(node *Node) error {
		var err error
		removedName, err = node.GetString("name")
		return err
	}); err != nil {
		t.Fatalf("OnRemove() error = %v", err)
	}
	if err := manager.remove("/items", 0); err != nil {
		t.Fatalf("remove() error = %v", err)
	}

	if removedName != "one" {
		t.Fatalf("removed name = %q, want one", removedName)
	}
	if got, err := mustNodeAt(t, manager.Config(), "items", 0, "name").GetString(); err != nil || got != "two" {
		t.Fatalf("first remaining name = %q, %v", got, err)
	}
	persistedItems, _ := decodeSourceConfig(t, source).Get("items")
	if got := len(persistedItems.([]interface{})); got != 1 {
		t.Fatalf("persisted items length = %d, want 1", got)
	}
}

func TestManagerReplacePersistsChange(t *testing.T) {
	manager, source := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")

	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	if err := manager.replace("/name", "updated"); err != nil {
		t.Fatalf("replace() error = %v", err)
	}

	if got, err := mustNodeAt(t, manager.Config(), "name").GetString(); err != nil || got != "updated" {
		t.Fatalf("name = %q, %v", got, err)
	}
	if got, _ := decodeSourceConfig(t, source).Get("name"); got != "updated" {
		t.Fatalf("persisted name = %v, want updated", got)
	}
}

func TestManagerRejectsSchemaInvalidChangeWithoutPublishing(t *testing.T) {
	manager, source := newTestManager(t)
	port := mustNodeAt(t, manager.Config(), "port")
	if err := manager.OnReplace(port, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}

	if err := manager.replace("/port", "not-a-number"); err == nil {
		t.Fatal("replace() error = nil, want schema error")
	}
	if manager.Version() != 1 {
		t.Fatalf("version = %d, want 1", manager.Version())
	}
	if got, err := mustNodeAt(t, manager.Config(), "port").GetInt(); err != nil || got != 8080 {
		t.Fatalf("in-memory port = %d, %v", got, err)
	}
	if got, _ := decodeSourceConfig(t, source).Get("port"); got != float64(8080) {
		t.Fatalf("persisted port = %v, want 8080", got)
	}
	if len(manager.GetHistory()) != 0 {
		t.Fatal("failed change was added to history")
	}
}

func TestManagerHandlerCanRejectChange(t *testing.T) {
	manager, source := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	wantErr := errors.New("rejected by handler")

	if err := manager.OnReplace(name, func(*Node) error { return wantErr }); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	if err := manager.replace("/name", "updated"); !errors.Is(err, wantErr) {
		t.Fatalf("replace() error = %v, want %v", err, wantErr)
	}

	if got, err := mustNodeAt(t, manager.Config(), "name").GetString(); err != nil || got != "demo" {
		t.Fatalf("in-memory name = %q, %v", got, err)
	}
	if got, _ := decodeSourceConfig(t, source).Get("name"); got != "demo" {
		t.Fatalf("persisted name = %v, want demo", got)
	}
	if manager.Version() != 1 {
		t.Fatalf("version = %d, want 1", manager.Version())
	}
}

func TestManagerCustomValidatorRejectsChange(t *testing.T) {
	manager, _ := newTestManager(t)
	port := mustNodeAt(t, manager.Config(), "port")
	if err := manager.OnReplace(port, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	manager.AddValidator("/port", ValidateRange(1000, 9000))

	if err := manager.replace("/port", float64(9999)); err == nil {
		t.Fatal("replace() error = nil, want custom validation error")
	}
	if manager.Version() != 1 {
		t.Fatalf("version = %d, want 1", manager.Version())
	}
}

func TestManagerRequiresValidSchemaAndInitialConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config string
		schema string
	}{
		{name: "empty schema", config: testConfigJSON, schema: ""},
		{name: "malformed schema", config: testConfigJSON, schema: `{"type":`},
		{name: "schema mismatch", config: `{"name":"incomplete"}`, schema: testSchemaJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := NewStrSource(tt.config, tt.schema)
			if err != nil {
				t.Fatalf("NewStrSource() error = %v", err)
			}
			if _, err := NewManager(source); err == nil {
				t.Fatal("NewManager() error = nil, want error")
			}
		})
	}
}
