package goconfig

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

const testSchema = `{
  "type": "object",
  "properties": {
    "items": {"type": "array", "items": {"type": "object"}}
  }
}`

func newTestManager(t *testing.T, configJSON string) *Manager {
	t.Helper()
	src, err := NewStrSource(configJSON, testSchema)
	if err != nil {
		t.Fatalf("NewStrSource: %v", err)
	}
	m, err := NewManager(src)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

// Rollback regression (single-threaded, no concurrency involved - see
// TestManager_PerPathLocking_ConcurrentSamePath below for the concurrent
// variant): when a registered handler returns an error, Insert/Remove/
// Replace must leave the in-memory tree, the persisted config, and the
// version counter exactly as they were before the call - not partially
// applied.
func TestManager_Insert_RollsBackOnHandlerError(t *testing.T) {
	m := newTestManager(t, `{"items":[{"a":1}]}`)
	itemsNode, err := m.Config().At("items")
	if err != nil {
		t.Fatalf("At(items): %v", err)
	}

	wantErr := errors.New("handler rejects this insert")
	if err := m.OnInsert(itemsNode, func(n *Node) error { return wantErr }); err != nil {
		t.Fatalf("OnInsert: %v", err)
	}

	versionBefore := m.Version()
	configBefore := m.ConfigJSON()

	err = m.Insert("/items", 1, map[string]interface{}{"a": 2})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Insert error = %v, want %v", err, wantErr)
	}

	if m.Version() != versionBefore {
		t.Errorf("version changed after a rolled-back insert: before=%d after=%d", versionBefore, m.Version())
	}
	if m.ConfigJSON() != configBefore {
		t.Errorf("persisted config changed after a rolled-back insert")
	}

	arr, _ := itemsNode.GetArray()
	if len(arr) != 1 {
		t.Errorf("in-memory array has %d elements after a rolled-back insert, want 1", len(arr))
	}
}

func TestManager_Remove_RollsBackOnHandlerError(t *testing.T) {
	m := newTestManager(t, `{"items":[{"a":1},{"a":2}]}`)
	itemsNode, err := m.Config().At("items")
	if err != nil {
		t.Fatalf("At(items): %v", err)
	}

	wantErr := errors.New("handler rejects this remove")
	if err := m.OnRemove(itemsNode, func(n *Node) error { return wantErr }); err != nil {
		t.Fatalf("OnRemove: %v", err)
	}

	versionBefore := m.Version()
	configBefore := m.ConfigJSON()

	if err := m.Remove("/items", 0); !errors.Is(err, wantErr) {
		t.Fatalf("Remove error = %v, want %v", err, wantErr)
	}

	if m.Version() != versionBefore {
		t.Errorf("version changed after a rolled-back remove")
	}
	if m.ConfigJSON() != configBefore {
		t.Errorf("persisted config changed after a rolled-back remove")
	}

	arr, _ := itemsNode.GetArray()
	if len(arr) != 2 {
		t.Errorf("in-memory array has %d elements after a rolled-back remove, want 2", len(arr))
	}
}

func TestManager_Replace_RollsBackOnHandlerError(t *testing.T) {
	m := newTestManager(t, `{"items":[{"a":1}]}`)
	aNode, err := m.Config().At("items")
	if err != nil {
		t.Fatalf("At(items): %v", err)
	}
	aNode, err = aNode.At(0)
	if err != nil {
		t.Fatalf("At(0): %v", err)
	}
	aNode, err = aNode.At("a")
	if err != nil {
		t.Fatalf("At(a): %v", err)
	}

	wantErr := errors.New("handler rejects this replace")
	if err := m.OnReplace(aNode, func(n *Node) error { return wantErr }); err != nil {
		t.Fatalf("OnReplace: %v", err)
	}

	versionBefore := m.Version()
	configBefore := m.ConfigJSON()

	if err := m.Replace("/items/0/a", 99); !errors.Is(err, wantErr) {
		t.Fatalf("Replace error = %v, want %v", err, wantErr)
	}

	if m.Version() != versionBefore {
		t.Errorf("version changed after a rolled-back replace")
	}
	if m.ConfigJSON() != configBefore {
		t.Errorf("persisted config changed after a rolled-back replace")
	}

	v, err := aNode.GetInt()
	if err != nil || v != 1 {
		t.Errorf("value after rolled-back replace = %v (err=%v), want 1", v, err)
	}
}

// B3 regression: history.ChangeEvent.OldValue for an object-typed value must
// be a plain map[string]interface{}, not the internal map[string]*Node
// representation (which used to leak as pointer addresses / serialize to
// "{}").
func TestManager_History_OldValueIsPlain(t *testing.T) {
	m := newTestManager(t, `{"items":[{"a":1,"b":"x"}]}`)
	m.EnableHistory(true)

	itemsNode, err := m.Config().At("items")
	if err != nil {
		t.Fatalf("At(items): %v", err)
	}
	if err := m.OnRemove(itemsNode, func(n *Node) error { return nil }); err != nil {
		t.Fatalf("OnRemove: %v", err)
	}

	if err := m.Remove("/items", 0); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	hist := m.GetHistory()
	if len(hist) == 0 {
		t.Fatal("expected a history event, got none")
	}
	last := hist[len(hist)-1]

	plain, ok := last.OldValue.(map[string]interface{})
	if !ok {
		t.Fatalf("OldValue is %T, want map[string]interface{}", last.OldValue)
	}
	if plain["a"] != int64(1) && plain["a"] != float64(1) {
		// parseNode normalizes JSON numbers to float64 on the way in; either
		// representation proves it's a plain value, not a *Node.
		t.Errorf("OldValue[\"a\"] = %v (%T), want a plain numeric value", plain["a"], plain["a"])
	}
	if plain["b"] != "x" {
		t.Errorf("OldValue[\"b\"] = %v, want \"x\"", plain["b"])
	}

	// Must also survive JSON export cleanly - ExportJSON used to silently
	// produce "{}" for the OldValue field in this case, since Node.value is
	// unexported and invisible to encoding/json.
	data, err := m.history.ExportJSON()
	if err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}
	if !strings.Contains(string(data), `"a": 1`) {
		t.Errorf("exported history JSON doesn't contain the plain old value, got: %s", data)
	}
}

// B4 regression: customValidator must tolerate concurrent AddValidator +
// Validate without a data race. Run with `go test -race` for the race
// detector to actually catch a regression; without -race this only checks
// for an outright panic/deadlock.
func TestCustomValidator_ConcurrentAccess(t *testing.T) {
	m := newTestManager(t, `{"items":[]}`)
	cv := m.GetCustomValidator()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cv.AddValidator("/x", func(path string, o, n *Node) error { return nil })
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = cv.Validate("/x", nil, nil)
		}
	}()

	wg.Wait()
}

// C1 regression: two concurrent mutations on the *same* path must not
// interleave across the handler-call window (where the global mutex is
// released). Before per-path locking, a rollback in one mutation (triggered
// by its handler returning an error) could wipe out a completely unrelated
// concurrent mutation on the same path that happened to land during that
// window - even though that second mutation succeeded on its own.
//
// Scenario: goroutine A inserts "trigger-fail" - its handler sleeps
// (simulating real work) and then returns an error, forcing a rollback to
// the array as it was *before* A's insert. Goroutine B inserts "b"
// concurrently, with a handler that always succeeds. If A and B correctly
// serialize on the same path, B's insert cannot start until A (including
// its rollback) has fully finished - so the end state must be exactly
// ["b"], never something A's rollback partially or fully erased.
func TestManager_PerPathLocking_ConcurrentSamePath(t *testing.T) {
	schema := `{"type":"object","properties":{"items":{"type":"array","items":{"type":"string"}}}}`
	src, err := NewStrSource(`{"items":[]}`, schema)
	if err != nil {
		t.Fatalf("NewStrSource: %v", err)
	}
	m, err := NewManager(src)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	itemsNode, err := m.Config().At("items")
	if err != nil {
		t.Fatalf("At(items): %v", err)
	}

	if err := m.OnInsert(itemsNode, func(n *Node) error {
		s, _ := n.GetString()
		if s == "trigger-fail" {
			time.Sleep(50 * time.Millisecond)
			return errors.New("intentional failure to trigger rollback")
		}
		return nil
	}); err != nil {
		t.Fatalf("OnInsert: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	var errA, errB error
	go func() {
		defer wg.Done()
		errA = m.Insert("/items", 0, "trigger-fail")
	}()

	// Give goroutine A a head start so it's inside its handler's sleep
	// window when B attempts its own Insert on the same path.
	time.Sleep(10 * time.Millisecond)

	go func() {
		defer wg.Done()
		errB = m.Insert("/items", 0, "b")
	}()

	wg.Wait()

	if errA == nil {
		t.Fatal("expected A's insert to fail (handler returns an error), got nil")
	}
	if errB != nil {
		t.Fatalf("expected B's insert to succeed, got: %v", errB)
	}

	arr, err := m.Config().At("items")
	if err != nil {
		t.Fatalf("At(items) after mutations: %v", err)
	}
	items, err := arr.GetArray()
	if err != nil {
		t.Fatalf("GetArray: %v", err)
	}

	got := make([]string, len(items))
	for i, n := range items {
		got[i], _ = n.GetString()
	}

	if len(got) != 1 || got[0] != "b" {
		t.Fatalf("final items = %v, want exactly [\"b\"] (A's rollback must not have touched B's successful insert, and B must not have been lost to A's rollback)", got)
	}
}
