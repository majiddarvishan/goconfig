package goconfig

import (
	"reflect"
	"testing"

	"github.com/iancoleman/orderedmap"
)

func TestJSONPathMutationsForSupportedPaths(t *testing.T) {
	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	config, err := Clone(source.getConfigObject())
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}

	if _, err := applyMutation(config, mutationReplace, "/nested/value", 0, "changed"); err != nil {
		t.Fatalf("replace mutation error = %v", err)
	}
	if _, err := applyMutation(config, mutationInsert, "/items", 1, map[string]interface{}{"id": 3, "name": "three"}); err != nil {
		t.Fatalf("insert mutation error = %v", err)
	}
	if _, err := applyMutation(config, mutationRemove, "/items", 0, nil); err != nil {
		t.Fatalf("remove mutation error = %v", err)
	}

	nestedValue, _ := config.Get("nested")
	nested := orderedMapForTest(t, nestedValue)
	if got, _ := nested.Get("value"); got != "changed" {
		t.Fatalf("nested value = %v, want changed", got)
	}

	itemsValue, _ := config.Get("items")
	items := itemsValue.([]interface{})
	if len(items) != 2 {
		t.Fatalf("items length = %d, want 2", len(items))
	}
}

// orderedmap values decoded through JSON may be pointers or values depending
// on where they entered the tree.
func orderedMapForTest(t *testing.T, value interface{}) *orderedmap.OrderedMap {
	t.Helper()
	switch result := value.(type) {
	case *orderedmap.OrderedMap:
		return result
	case orderedmap.OrderedMap:
		return &result
	default:
		t.Fatalf("value type = %T, want ordered map", value)
		return nil
	}
}

func TestCloneProducesIndependentOrderedMap(t *testing.T) {
	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}

	clone, err := Clone(source.getConfigObject())
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	clone.Set("name", "clone")

	got, _ := source.getConfigObject().Get("name")
	if reflect.DeepEqual(got, "clone") {
		t.Fatal("changing clone changed original")
	}
}

func TestFindNodePath(t *testing.T) {
	root := mustParseNode(t, map[string]interface{}{
		"items": []interface{}{map[string]interface{}{"name": "first"}},
	})
	target := mustNodeAt(t, root, "items", 0, "name")

	if got := findNodePath(root, target); got != "/items/0/name" {
		t.Fatalf("findNodePath() = %q, want /items/0/name", got)
	}
	if got := findNodePath(root, mustParseNode(t, "outside")); got != "" {
		t.Fatalf("outside node path = %q, want empty", got)
	}
}
