package goconfig

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestManagerConfigSnapshotCannotMutateManager(t *testing.T) {
	manager, _ := newTestManager(t)
	snapshot := manager.Config()
	object, err := snapshot.GetObject()
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	object["name"] = mustParseNode(t, "hacked")

	items, err := mustNodeAt(t, snapshot, "items").GetArray()
	if err != nil {
		t.Fatalf("GetArray() error = %v", err)
	}
	items[0] = mustParseNode(t, map[string]interface{}{"id": 99, "name": "hacked"})

	if got, _ := mustNodeAt(t, manager.Config(), "name").GetString(); got != "demo" {
		t.Fatalf("manager name = %q, want demo", got)
	}
	if got, _ := mustNodeAt(t, manager.Config(), "items", 0, "name").GetString(); got != "one" {
		t.Fatalf("manager item name = %q, want one", got)
	}
}

func TestSourceObjectSnapshotCannotMutateSource(t *testing.T) {
	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	snapshot := source.getConfigObject()
	snapshot.Set("name", "hacked")

	again := source.getConfigObject()
	if got, _ := again.Get("name"); got != "demo" {
		t.Fatalf("source name = %v, want demo", got)
	}
}

func TestGetIntRejectsFractionalValue(t *testing.T) {
	if _, err := mustParseNode(t, float64(1.5)).GetInt(); err == nil {
		t.Fatal("GetInt(1.5) error = nil, want error")
	}
	if got, err := mustParseNode(t, json.Number("42")).GetInt(); err != nil || got != 42 {
		t.Fatalf("GetInt(json.Number(42)) = %d, %v", got, err)
	}
}

func TestMutationRejectsUnsupportedValueWithoutChangingState(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}

	err := manager.replace("/name", make(chan int))
	if err == nil {
		t.Fatal("replace(channel) error = nil, want error")
	}
	if errors.Is(err, ErrPersistence) {
		t.Fatalf("unsupported value reached persistence: %v", err)
	}
	if got, _ := mustNodeAt(t, manager.Config(), "name").GetString(); got != "demo" {
		t.Fatalf("manager name = %q, want demo", got)
	}
}

func TestPublicSourceCanConstructManager(t *testing.T) {
	source := &memoryPublicSource{data: SourceData{Config: []byte(testConfigJSON), Schema: []byte(testSchemaJSON)}}
	manager, err := NewManagerFromSource(source)
	if err != nil {
		t.Fatalf("NewManagerFromSource() error = %v", err)
	}
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	if err := manager.replace("/name", "public-source"); err != nil {
		t.Fatalf("replace() error = %v", err)
	}

	parsed, err := parseConfig(source.data.Config)
	if err != nil {
		t.Fatalf("parse public source config: %v", err)
	}
	if got, _ := parsed.Get("name"); got != "public-source" {
		t.Fatalf("public source name = %v, want public-source", got)
	}
}

type memoryPublicSource struct {
	data SourceData
}

func (s *memoryPublicSource) Read() (SourceData, error) {
	return SourceData{
		Config: append([]byte(nil), s.data.Config...),
		Schema: append([]byte(nil), s.data.Schema...),
	}, nil
}

func (s *memoryPublicSource) Write(config []byte) error {
	s.data.Config = append([]byte(nil), config...)
	return nil
}
