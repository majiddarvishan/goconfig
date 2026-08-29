package history

import (
	"sync"
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

func TestChangeHistoryCopiesPayloadsAndIndexes(t *testing.T) {
	index := 2
	payload := map[string]interface{}{"items": []interface{}{"original"}}
	history := NewChangeHistory(2)
	history.Add(ChangeEvent{Index: &index, NewValue: payload})

	index = 9
	payload["items"].([]interface{})[0] = "mutated"
	first := history.GetAll()
	if *first[0].Index != 2 || first[0].NewValue.(map[string]interface{})["items"].([]interface{})[0] != "original" {
		t.Fatalf("stored event was mutated indirectly: %#v", first[0])
	}

	*first[0].Index = 7
	first[0].NewValue.(map[string]interface{})["items"].([]interface{})[0] = "returned mutation"
	second := history.GetAll()
	if *second[0].Index != 2 || second[0].NewValue.(map[string]interface{})["items"].([]interface{})[0] != "original" {
		t.Fatalf("returned event exposed history state: %#v", second[0])
	}
}

func TestChangeHistoryRejectsNonPositiveLimits(t *testing.T) {
	history := NewChangeHistory(2)
	history.Add(ChangeEvent{Path: "/value"})
	if got := history.GetRecent(0); len(got) != 0 {
		t.Fatalf("GetRecent(0) = %#v, want empty", got)
	}
	if got := history.GetRecent(-1); len(got) != 0 {
		t.Fatalf("GetRecent(-1) = %#v, want empty", got)
	}
	if got := history.GetByPath("/value", 0); len(got) != 0 {
		t.Fatalf("GetByPath(limit 0) = %#v, want empty", got)
	}
}

func TestChangeHistorySupportsConcurrentAccess(t *testing.T) {
	history := NewChangeHistory(32)
	var group sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for iteration := 0; iteration < 100; iteration++ {
				history.Add(ChangeEvent{Path: "/value", Version: int64(worker*100 + iteration)})
				_ = history.GetAll()
				_ = history.GetRecent(5)
				_ = history.GetByPath("/value", 5)
			}
		}(worker)
	}
	group.Wait()
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
