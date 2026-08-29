package goconfig

import (
	"encoding/json"
	"testing"

	"github.com/iancoleman/orderedmap"
)

const testConfigJSON = `{
  "name": "demo",
  "enabled": true,
  "port": 8080,
  "items": [
    {"id": 1, "name": "one"},
    {"id": 2, "name": "two"}
  ],
  "nested": {"value": "initial", "tags": ["a", "b"]}
}`

const testSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["name", "enabled", "port", "items", "nested"],
  "properties": {
    "name": {"type": "string", "minLength": 1},
    "enabled": {"type": "boolean"},
    "port": {"type": "integer", "minimum": 1, "maximum": 65535},
    "items": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "name"],
        "properties": {
          "id": {"type": "integer"},
          "name": {"type": "string"}
        },
        "additionalProperties": false
      }
    },
    "nested": {
      "type": "object",
      "required": ["value", "tags"],
      "properties": {
        "value": {"type": "string"},
        "tags": {"type": "array", "items": {"type": "string"}}
      },
      "additionalProperties": false
    }
  },
  "additionalProperties": false
}`

func newTestManager(t *testing.T) (*Manager, *StrSource) {
	t.Helper()

	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}

	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	return manager, source
}

func mustNodeAt(t *testing.T, node *Node, path ...interface{}) *Node {
	t.Helper()

	current := node
	for _, segment := range path {
		var err error
		current, err = current.At(segment)
		if err != nil {
			t.Fatalf("At(%v) error = %v", segment, err)
		}
	}
	return current
}

func decodeSourceConfig(t *testing.T, source ISource) *orderedmap.OrderedMap {
	t.Helper()

	raw := source.getConfig()
	if raw == nil {
		t.Fatal("source config is nil")
	}

	result := orderedmap.New()
	if err := json.Unmarshal([]byte(*raw), &result); err != nil {
		t.Fatalf("unmarshal source config: %v", err)
	}
	return result
}

func mustParseNode(t *testing.T, value interface{}) *Node {
	t.Helper()
	node, err := parseNodeStrict(value)
	if err != nil {
		t.Fatalf("parseNodeStrict(%T) error = %v", value, err)
	}
	return node
}
