package history

import (
	"testing"
	"time"
)

func TestChangeHistoryCircularOrder(t *testing.T) {
	history := NewChangeHistory(2)
	for version := int64(1); version <= 3; version++ {
		history.Add(ChangeEvent{
			Timestamp: time.Unix(version, 0),
			Operation: "replace",
			Path:      "/value",
			Version:   version,
		})
	}

	events := history.GetAll()
	if len(events) != 2 || events[0].Version != 2 || events[1].Version != 3 {
		t.Fatalf("events = %#v", events)
	}

	recent := history.GetRecent(1)
	if len(recent) != 1 || recent[0].Version != 3 {
		t.Fatalf("recent = %#v", recent)
	}
}

func TestChangeHistoryFiltersAndClears(t *testing.T) {
	history := NewChangeHistory(5)
	history.Add(ChangeEvent{Path: "/a", Version: 1})
	history.Add(ChangeEvent{Path: "/b", Version: 2})
	history.Add(ChangeEvent{Path: "/a", Version: 3})

	events := history.GetByPath("/a", 1)
	if len(events) != 1 || events[0].Version != 3 {
		t.Fatalf("filtered events = %#v", events)
	}

	history.Clear()
	if got := history.GetAll(); len(got) != 0 {
		t.Fatalf("events after Clear() = %#v", got)
	}
}
