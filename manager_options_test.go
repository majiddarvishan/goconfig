package goconfig

import "testing"

func TestManagerHistoryCapacityOption(t *testing.T) {
	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManagerWithOptions(source, WithHistoryCapacity(2))
	if err != nil {
		t.Fatalf("NewManagerWithOptions() error = %v", err)
	}
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	for _, value := range []string{"one", "two", "three"} {
		if err := manager.replace("/name", value); err != nil {
			t.Fatalf("replace(%q) error = %v", value, err)
		}
	}
	if events := manager.GetHistory(); len(events) != 2 || events[0].Version != 3 || events[1].Version != 4 {
		t.Fatalf("history = %#v, want last two events", events)
	}
}

func TestManagerRejectsInvalidHistoryCapacity(t *testing.T) {
	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	if _, err := NewManagerWithOptions(source, WithHistoryCapacity(0)); err == nil {
		t.Fatal("NewManagerWithOptions() error = nil, want invalid capacity error")
	}
}
