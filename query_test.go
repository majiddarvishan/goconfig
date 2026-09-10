package goconfig

import (
	"testing"
)

const querySchema = `{
  "type": "object",
  "properties": {
    "title": {"type": "string"},
    "users": {"type": "array", "items": {"type": "object"}}
  }
}`

func newQueryTestManager(t *testing.T) *Manager {
	t.Helper()
	config := `{
		"title": "test config",
		"users": [
			{"name": "alice", "age": 30},
			{"name": "bob", "age": 15},
			{"name": "carol", "age": 22}
		]
	}`
	src, err := NewStrSource(config, querySchema)
	if err != nil {
		t.Fatalf("NewStrSource: %v", err)
	}
	m, err := NewManager(src)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

// Covers the query forms documented on Manager.Query's doc comment:
// direct path, wildcard, array-index, all-array-elements, and a filter
// condition.
func TestQuery_DocumentedForms(t *testing.T) {
	m := newQueryTestManager(t)

	t.Run("direct path", func(t *testing.T) {
		// A plain object-key path, no array segments involved - array
		// access in this DSL always needs the "[N]" bracket form, exercised
		// separately below ("specific array index [0]").
		results, err := m.Query("/title")
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
		s, err := results[0].Node.GetString()
		if err != nil || s != "test config" {
			t.Errorf("got %q (err=%v), want \"test config\"", s, err)
		}
	})

	t.Run("wildcard for any key", func(t *testing.T) {
		// "/users/[0]/*" - all fields of the first user, i.e. exercises
		// wildcard on an object.
		results, err := m.Query("/users/[0]/*")
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(results) != 2 { // name, age
			t.Fatalf("got %d results, want 2 (name, age)", len(results))
		}
	})

	t.Run("all array elements [*]", func(t *testing.T) {
		results, err := m.Query("/users/[*]/name")
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(results) != 3 {
			t.Fatalf("got %d results, want 3", len(results))
		}
	})

	t.Run("specific array index [0]", func(t *testing.T) {
		results, err := m.Query("/users/[0]/name")
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("got %d results, want 1", len(results))
		}
	})

	t.Run("filter condition [?age>18]", func(t *testing.T) {
		results, err := m.Query("/users/[?age>18]/name")
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		// alice (30) and carol (22) qualify; bob (15) does not.
		if len(results) != 2 {
			t.Fatalf("got %d results, want 2 (alice, carol), got paths: %v", len(results), pathsOf(results))
		}
		names := map[string]bool{}
		for _, r := range results {
			s, _ := r.Node.GetString()
			names[s] = true
		}
		if !names["alice"] || !names["carol"] || names["bob"] {
			t.Errorf("filter result set wrong: %v", names)
		}
	})

	t.Run("root query", func(t *testing.T) {
		results, err := m.Query("/")
		if err != nil {
			t.Fatalf("Query(\"/\"): %v", err)
		}
		if len(results) != 1 || results[0].Path != "/" {
			t.Fatalf("got %+v, want a single root result", results)
		}
	})
}

func pathsOf(results []QueryResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Path
	}
	return out
}

func TestQueryOne_NoResults(t *testing.T) {
	m := newQueryTestManager(t)
	if _, err := m.QueryOne("/users/[?age>1000]/name"); err == nil {
		t.Fatal("expected an error for a query with no results, got nil")
	}
}

func TestQueryExists_And_QueryCount(t *testing.T) {
	m := newQueryTestManager(t)

	if !m.QueryExists("/users/[0]/name") {
		t.Error("QueryExists: expected true for an existing path")
	}
	if m.QueryExists("/users/[99]/name") {
		t.Error("QueryExists: expected false for an out-of-bounds path")
	}

	count, err := m.QueryCount("/users/[*]/name")
	if err != nil {
		t.Fatalf("QueryCount: %v", err)
	}
	if count != 3 {
		t.Errorf("QueryCount = %d, want 3", count)
	}
}

func TestFindAll(t *testing.T) {
	m := newQueryTestManager(t)

	results := m.FindAll(func(n *Node) bool {
		return n.Type() == String
	})

	// "title" + 3 users' "name" fields = 4 string nodes.
	if len(results) != 4 {
		t.Errorf("FindAll(String) found %d nodes, want 4", len(results))
	}
}
