package goconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/iancoleman/orderedmap"
)

func TestNewStrSourceParsesConfiguration(t *testing.T) {
	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}

	if source.getConfigObject() == nil {
		t.Fatal("parsed config object is nil")
	}
	if got := source.getSchema(); got == nil || *got != testSchemaJSON {
		t.Fatalf("schema = %v, want original schema", got)
	}
}

func TestNewStrSourceRejectsEmptyAndMalformedConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		config string
	}{
		{name: "empty", config: ""},
		{name: "malformed", config: `{"name":`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewStrSource(tt.config, testSchemaJSON); err == nil {
				t.Fatal("NewStrSource() error = nil, want error")
			}
		})
	}
}

func TestStrSourceSetConfigUpdatesRepresentations(t *testing.T) {
	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}

	updated := orderedmap.New()
	updated.Set("name", "updated")
	if err := source.setConfig(updated); err != nil {
		t.Fatalf("setConfig() error = %v", err)
	}

	if got, ok := source.getConfigObject().Get("name"); !ok || got != "updated" {
		t.Fatalf("config object name = %v, %v", got, ok)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(*source.getConfig()), &decoded); err != nil {
		t.Fatalf("stored JSON is invalid: %v", err)
	}
	if decoded["name"] != "updated" {
		t.Fatalf("stored name = %v, want updated", decoded["name"])
	}
}

func TestFileSourcePersistsConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(testConfigJSON), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	source, err := NewFileSource(path, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewFileSource() error = %v", err)
	}

	updated, err := Clone(source.getConfigObject())
	if err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	updated.Set("name", "persisted")

	if err := source.setConfig(updated); err != nil {
		t.Fatalf("setConfig() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("persisted JSON is invalid: %v", err)
	}
	if decoded["name"] != "persisted" {
		t.Fatalf("persisted name = %v, want persisted", decoded["name"])
	}
}
