package goconfig

import (
	"testing"

	"github.com/iancoleman/orderedmap"
)

func mustOrderedMap(t *testing.T, jsonStr string) *orderedmap.OrderedMap {
	t.Helper()
	om := orderedmap.New()
	if err := om.UnmarshalJSON([]byte(jsonStr)); err != nil {
		t.Fatalf("failed to build test OrderedMap: %v", err)
	}
	return om
}

// B1 regression: a JSON null anywhere along a path must produce a clean
// error from jsonSetByPath/jsonRemoveByPath/jsonInsertByPath, not panic.
// Before the fix, reflect.TypeOf(found).Kind() on a nil `found` (an
// untyped nil interface{}, which is exactly what a JSON null unmarshals
// to) panicked with "invalid memory address or nil pointer dereference".
func TestJsonByPath_NilInPath_DoesNotPanic(t *testing.T) {
	om := mustOrderedMap(t, `{"a": {"b": null}}`)

	t.Run("jsonSetByPath", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("jsonSetByPath panicked on nil-in-path: %v", r)
			}
		}()
		if err := jsonSetByPath(om, "/a/b/c", "x"); err == nil {
			t.Fatal("expected an error traversing through a null value, got nil")
		}
	})

	t.Run("jsonRemoveByPath", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("jsonRemoveByPath panicked on nil-in-path: %v", r)
			}
		}()
		if err := jsonRemoveByPath(om, "/a/b/c", 0); err == nil {
			t.Fatal("expected an error traversing through a null value, got nil")
		}
	})

	t.Run("jsonInsertByPath", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("jsonInsertByPath panicked on nil-in-path: %v", r)
			}
		}()
		if err := jsonInsertByPath(om, "/a/b/c", 0, "x"); err == nil {
			t.Fatal("expected an error traversing through a null value, got nil")
		}
	})
}

// B1 regression, array variant: a null array *element* reached while
// resolving a numeric path segment must also error cleanly, not panic.
func TestJsonByPath_NilArrayElement_DoesNotPanic(t *testing.T) {
	om := mustOrderedMap(t, `{"a": [null]}`)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked on null array element: %v", r)
		}
	}()
	if err := jsonSetByPath(om, "/a/0/b", "x"); err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// B2 regression: parseNode must not silently coerce every numeric Go type
// other than int/float64 to Null - it used to fall through to the default
// case for int64/int32/int16/int8/uint*/float32.
func TestParseNode_NumericTypeCoverage(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want NodeType
	}{
		{"int", int(1), Integral},
		{"int8", int8(1), Integral},
		{"int16", int16(1), Integral},
		{"int32", int32(1), Integral},
		{"int64", int64(1), Integral},
		{"uint", uint(1), Integral},
		{"uint8", uint8(1), Integral},
		{"uint16", uint16(1), Integral},
		{"uint32", uint32(1), Integral},
		{"uint64", uint64(1), Integral},
		{"float32", float32(1.5), FloatingPoint},
		{"float64", float64(1.5), FloatingPoint},
		{"string", "s", String},
		{"bool", true, Boolean},
		{"nil", nil, Null},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := parseNode(c.in)
			if got := n.Type(); got != c.want {
				t.Errorf("parseNode(%v) (%T) -> Type() = %v, want %v", c.in, c.in, got, c.want)
			}
		})
	}
}
