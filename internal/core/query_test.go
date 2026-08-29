package goconfig

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestManagerQueryDirectAndRoot(t *testing.T) {
	manager, _ := newTestManager(t)

	root, err := manager.Query("/")
	if err != nil {
		t.Fatalf("Query(/) error = %v", err)
	}
	if len(root) != 1 || root[0].Path != "" || root[0].Node.Type() != Object {
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

func TestQueryUsesCanonicalJSONPointerAndPlainArrayIndexes(t *testing.T) {
	source, err := NewStrSource(`{"a/b":{"m~n":"value"},"items":[{"name":"first"}]}`, `{"type":"object"}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	escaped, err := manager.QueryOne("/a~1b/m~0n")
	if err != nil {
		t.Fatalf("QueryOne(escaped) error = %v", err)
	}
	if escaped.Path != "/a~1b/m~0n" {
		t.Fatalf("escaped result path = %q", escaped.Path)
	}
	indexed, err := manager.QueryOne("/items/0/name")
	if err != nil {
		t.Fatalf("QueryOne(index) error = %v", err)
	}
	if got, _ := indexed.Node.GetString(); got != "first" {
		t.Fatalf("indexed value = %q", got)
	}
}

func TestLookupDoesNotInterpretQueryTokens(t *testing.T) {
	source, err := NewStrSource(`{"":{"value":"empty"},"*":"star","[*]":"brackets"}`, `{"type":"object"}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	for pointer, want := range map[string]string{"/*": "star", "/[*]": "brackets", "//value": "empty"} {
		result, lookupErr := manager.Lookup(pointer)
		if lookupErr != nil {
			t.Fatalf("Lookup(%q) error = %v", pointer, lookupErr)
		}
		if got, _ := result.Node.GetString(); got != want {
			t.Fatalf("Lookup(%q) = %q, want %q", pointer, got, want)
		}
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

func TestQueryFilterSupportsEscapedFieldsAndExactDecimals(t *testing.T) {
	source, err := NewStrSource(`{"items":[{"a/b":0.5,"name":"half"},{"a/b":0.7,"name":"high"}]}`, `{"type":"object"}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	results, err := manager.Query(`/items/[?a~1b==0.5]/name`)
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if got, _ := results[0].Node.GetString(); got != "half" {
		t.Fatalf("filtered name = %q, want half", got)
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

func TestQueryObjectTraversalIsLexicallyDeterministic(t *testing.T) {
	source, err := NewStrSource(`{"z":1,"a":2,"m":3}`, `{"type":"object"}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	results, err := manager.Query("/*")
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	paths := make([]string, len(results))
	for index, result := range results {
		paths[index] = result.Path
	}
	if want := []string{"/a", "/m", "/z"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestQueryWildcardReturnsDeterministicTraversalError(t *testing.T) {
	source, err := NewStrSource(`{"users":{"b":{"name":"b"},"a":{}}}`, `{"type":"object"}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	_, err = manager.Query("/users/*/name")
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("Query() error = %v, want ErrPathNotFound", err)
	}
	var queryErr *QueryError
	if !errors.As(err, &queryErr) || queryErr.Path != "/users/a" {
		t.Fatalf("QueryError = %#v, want first lexical failing path", queryErr)
	}
}

func TestQueryFilterDoesNotDiscardMatchedBranchErrors(t *testing.T) {
	source, err := NewStrSource(`{"items":[{"id":1},{"id":2,"name":"two"}]}`, `{"type":"object"}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	_, err = manager.Query("/items/[?id>=1]/name")
	if !errors.Is(err, ErrPathNotFound) {
		t.Fatalf("Query() error = %v, want ErrPathNotFound", err)
	}
	var queryErr *QueryError
	if !errors.As(err, &queryErr) || queryErr.Path != "/items/0" {
		t.Fatalf("QueryError = %#v, want failing array path", queryErr)
	}
}

func TestFindAllRunsPredicateWithoutManagerLock(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	done := make(chan struct{})
	go func() {
		var once sync.Once
		manager.FindAll(func(*Node) bool {
			once.Do(func() {
				if err := manager.replace("/name", "from-predicate"); err != nil {
					t.Errorf("replace from predicate: %v", err)
				}
			})
			return false
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("FindAll predicate deadlocked with Manager mutation")
	}
	if got, _ := mustNodeAt(t, manager.Config(), "name").GetString(); got != "from-predicate" {
		t.Fatalf("name = %q, want from-predicate", got)
	}
}

func TestFindAllReturnsCanonicalLexicalPaths(t *testing.T) {
	source, err := NewStrSource(`{"z":"match","a/b":"match"}`, `{"type":"object"}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	results := manager.FindAll(func(node *Node) bool {
		value, valueErr := node.GetString()
		return valueErr == nil && value == "match"
	})
	paths := []string{results[0].Path, results[1].Path}
	if want := []string{"/a~1b", "/z"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
	if got := manager.FindAll(nil); len(got) != 0 {
		t.Fatalf("FindAll(nil) = %#v, want empty", got)
	}
}

func TestQueryReportsTypedInvalidAndEmptyResults(t *testing.T) {
	manager, _ := newTestManager(t)
	if _, err := manager.Query("not-a-pointer"); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("Query(invalid) error = %v, want ErrInvalidQuery", err)
	}
	if _, err := manager.QueryOne("/items/[?id>999]"); !errors.Is(err, ErrQueryNoResults) {
		t.Fatalf("QueryOne(empty) error = %v, want ErrQueryNoResults", err)
	}
	for _, query := range []string{"/items/[?id>]", "/items/[?id>name]", "/items/[00]"} {
		if _, err := manager.Query(query); !errors.Is(err, ErrInvalidQuery) {
			t.Fatalf("Query(%q) error = %v, want ErrInvalidQuery", query, err)
		}
	}
}
