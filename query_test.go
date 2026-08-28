package goconfig

import "testing"

func TestManagerQueryDirectAndRoot(t *testing.T) {
	manager, _ := newTestManager(t)

	root, err := manager.Query("/")
	if err != nil {
		t.Fatalf("Query(/) error = %v", err)
	}
	if len(root) != 1 || root[0].Path != "/" || root[0].Node.Type() != Object {
		t.Fatalf("root result = %#v", root)
	}

	result, err := manager.QueryOne("/nested/value")
	if err != nil {
		t.Fatalf("QueryOne() error = %v", err)
	}
	if got, err := result.Node.GetString(); err != nil || got != "initial" {
		t.Fatalf("query value = %q, %v", got, err)
	}
}

func TestManagerQueryArrayWildcardAndFilter(t *testing.T) {
	manager, _ := newTestManager(t)

	allNames, err := manager.Query("/items/[*]/name")
	if err != nil {
		t.Fatalf("wildcard Query() error = %v", err)
	}
	if len(allNames) != 2 {
		t.Fatalf("wildcard result count = %d, want 2", len(allNames))
	}

	filtered, err := manager.Query("/items/[?id>=2]/name")
	if err != nil {
		t.Fatalf("filter Query() error = %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filter result count = %d, want 1", len(filtered))
	}
	if got, err := filtered[0].Node.GetString(); err != nil || got != "two" {
		t.Fatalf("filtered name = %q, %v", got, err)
	}
}

func TestQueryResultsAreIndependentCopies(t *testing.T) {
	manager, _ := newTestManager(t)
	result, err := manager.QueryOne("/nested")
	if err != nil {
		t.Fatalf("QueryOne() error = %v", err)
	}

	object, err := result.Node.GetObject()
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	object["value"] = mustParseNode(t, "changed")

	again, err := manager.QueryOne("/nested/value")
	if err != nil {
		t.Fatalf("second QueryOne() error = %v", err)
	}
	if got, err := again.Node.GetString(); err != nil || got != "initial" {
		t.Fatalf("manager query value = %q, %v", got, err)
	}
}
