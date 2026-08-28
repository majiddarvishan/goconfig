package goconfig

import "testing"

func TestNodeAccessorsAndTypes(t *testing.T) {
	root := mustParseNode(t, map[string]interface{}{
		"text":    "value",
		"enabled": true,
		"count":   float64(3),
		"items":   []interface{}{"first", "second"},
		"nothing": nil,
	})

	if root.Type() != Object {
		t.Fatalf("root type = %v, want Object", root.Type())
	}
	if got, err := root.GetString("text"); err != nil || got != "value" {
		t.Fatalf("GetString(text) = %q, %v", got, err)
	}
	if got, err := root.GetBool("enabled"); err != nil || !got {
		t.Fatalf("GetBool(enabled) = %v, %v", got, err)
	}
	if got, err := root.GetInt("count"); err != nil || got != 3 {
		t.Fatalf("GetInt(count) = %d, %v", got, err)
	}
	if mustNodeAt(t, root, "nothing").Type() != Null {
		t.Fatal("nil value type is not Null")
	}
	if got, err := mustNodeAt(t, root, "items", 1).GetString(); err != nil || got != "second" {
		t.Fatalf("array item = %q, %v", got, err)
	}
}

func TestNodeAccessErrors(t *testing.T) {
	root := mustParseNode(t, map[string]interface{}{"value": "text"})

	if _, err := root.At("missing"); err == nil {
		t.Fatal("At(missing) error = nil, want error")
	}
	if _, err := root.At(0); err == nil {
		t.Fatal("At(0) on object error = nil, want error")
	}
	if _, err := mustNodeAt(t, root, "value").GetBool(); err == nil {
		t.Fatal("GetBool() on string error = nil, want error")
	}
}

func TestNodeDeepCopyIsIndependent(t *testing.T) {
	original := mustParseNode(t, map[string]interface{}{
		"nested": map[string]interface{}{"value": "original"},
	})
	copyNode := original.DeepCopy()

	copyObject, err := mustNodeAt(t, copyNode, "nested").GetObject()
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	copyObject["value"] = mustParseNode(t, "changed")

	got, err := mustNodeAt(t, original, "nested", "value").GetString()
	if err != nil || got != "original" {
		t.Fatalf("original nested value = %q, %v", got, err)
	}
}

func TestJSONIntegerIsRepresentedAsIntegralNode(t *testing.T) {
	manager, _ := newTestManager(t)
	port := mustNodeAt(t, manager.Config(), "port")
	if port.Type() != Integral {
		t.Fatalf("port type = %v, want Integral", port.Type())
	}
	if got, err := port.GetInt(); err != nil || got != 8080 {
		t.Fatalf("port = %d, %v", got, err)
	}
}
