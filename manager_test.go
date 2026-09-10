package goconfig

import (
	"strings"
	"sync"
	"testing"
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
