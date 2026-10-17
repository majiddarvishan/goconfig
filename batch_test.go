package goconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// A table whose schema requires at least one row and unique ids, plus a
// mirror of what an application would keep in memory for it.
const tableSchema = `{
  "type": "object",
  "properties": {
    "rows": {
      "type": "array",
      "minItems": 1,
      "uniqueItems": true,
      "items": {"type": "object", "properties": {"id": {"type": "integer"}, "v": {"type": "string"}}, "required": ["id", "v"]}
    }
  }
}`

type tableApp struct {
	m          *Manager
	state      []string // "id:v" per row the app knows about
	calls      []string
	failInsert string
}

func rowKey(n *Node) string {
	id, _ := n.At("id")
	v, _ := n.At("v")
	return fmt.Sprintf("%v:%v", nodeValueToPlain(id), nodeValueToPlain(v))
}

func newTableApp(t *testing.T, config string) *tableApp {
	t.Helper()
	src, err := NewStrSource(config, tableSchema)
	if err != nil {
		t.Fatalf("NewStrSource: %v", err)
	}
	m, err := NewManager(src)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	app := &tableApp{m: m}

	rows, _ := m.Config().At("rows")
	arr, _ := rows.GetArray()
	for _, r := range arr {
		app.state = append(app.state, rowKey(r))
	}

	if err := m.OnInsert(rows, func(n *Node) error {
		k := rowKey(n)
		app.calls = append(app.calls, "insert "+k)
		if k == app.failInsert {
			return errors.New("application refused " + k)
		}
		id := k[:strings.Index(k, ":")]
		for _, s := range app.state { // application-level uniqueness, stricter than the schema
			if s[:strings.Index(s, ":")] == id {
				return errors.New("duplicate id")
			}
		}
		app.state = append(app.state, k)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := m.OnRemove(rows, func(n *Node) error {
		k := rowKey(n)
		app.calls = append(app.calls, "remove "+k)
		for i, s := range app.state {
			if s == k {
				app.state = append(app.state[:i], app.state[i+1:]...)
				break
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return app
}

func (a *tableApp) rows() []string {
	rows, _ := a.m.Config().At("rows")
	arr, _ := rows.GetArray()
	out := []string{}
	for _, r := range arr {
		out = append(out, rowKey(r))
	}
	return out
}

func row(id int, v string) map[string]interface{} {
	return map[string]interface{}{"id": id, "v": v}
}

func sorted(s []string) []string {
	c := append([]string(nil), s...)
	sort.Strings(c)
	return c
}

func TestUpdate_ReplacesItemThroughRemoveAndInsertHandlers(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"},{"id":2,"v":"b"},{"id":3,"v":"c"}]}`)

	if got := app.m.UpdatablePaths(); !reflect.DeepEqual(got, []string{"/rows"}) {
		t.Fatalf("UpdatablePaths = %v", got)
	}

	// Same id: the app's uniqueness rule only passes because the old row is
	// removed before the new one is inserted.
	if err := app.m.Update("/rows", 1, row(2, "B")); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := app.rows(); !reflect.DeepEqual(got, []string{"1:a", "2:B", "3:c"}) {
		t.Errorf("rows = %v", got)
	}
	if !reflect.DeepEqual(app.calls, []string{"remove 2:b", "insert 2:B"}) {
		t.Errorf("calls = %v", app.calls)
	}
	if !strings.Contains(app.m.ConfigJSON(), `"v": "B"`) {
		t.Errorf("persisted config not updated: %s", app.m.ConfigJSON())
	}
}

func TestUpdate_OfTheOnlyItemPassesMinItems(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"}]}`)
	if err := app.m.Remove("/rows", 0); err == nil {
		t.Fatal("Remove of the only row should violate minItems")
	}
	if err := app.m.Update("/rows", 0, row(1, "z")); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := app.rows(); !reflect.DeepEqual(got, []string{"1:z"}) {
		t.Errorf("rows = %v", got)
	}
}

func TestUpdate_RejectsBadInputWithoutCallingHandlers(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"},{"id":2,"v":"b"}]}`)

	for name, err := range map[string]error{
		"schema":     app.m.Update("/rows", 0, map[string]interface{}{"id": 1}),
		"unique":     app.m.Update("/rows", 1, row(1, "a")),
		"bounds":     app.m.Update("/rows", 5, row(9, "x")),
		"unregister": app.m.Update("/other", 0, row(9, "x")),
	} {
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if len(app.calls) != 0 {
		t.Errorf("handlers called: %v", app.calls)
	}

	src, _ := NewStrSource(`{"rows":[{"id":1,"v":"a"}]}`, tableSchema)
	onlyInsert, _ := NewManager(src)
	rows, _ := onlyInsert.Config().At("rows")
	_ = onlyInsert.OnInsert(rows, func(*Node) error { return nil })
	if len(onlyInsert.UpdatablePaths()) != 0 {
		t.Error("an insert-only array must not be updatable")
	}
	if err := onlyInsert.Update("/rows", 0, row(1, "b")); err == nil {
		t.Error("Update on an insert-only array should fail")
	}
}

func TestUpdate_RestoresTheOldItemWhenInsertHandlerFails(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"},{"id":2,"v":"b"}]}`)
	app.failInsert = "2:B"
	configBefore, versionBefore := app.m.ConfigJSON(), app.m.Version()

	if err := app.m.Update("/rows", 1, row(2, "B")); err == nil {
		t.Fatal("expected an error")
	}
	if got := app.rows(); !reflect.DeepEqual(got, []string{"1:a", "2:b"}) {
		t.Errorf("rows = %v", got)
	}
	if !reflect.DeepEqual(app.state, []string{"1:a", "2:b"}) {
		t.Errorf("app state = %v (old row not re-adopted)", app.state)
	}
	if !reflect.DeepEqual(app.calls, []string{"remove 2:b", "insert 2:B", "insert 2:b"}) {
		t.Errorf("calls = %v", app.calls)
	}
	if app.m.ConfigJSON() != configBefore || app.m.Version() != versionBefore {
		t.Error("persisted config or version changed")
	}
}

func TestBatch_ValidatesOnlyTheFinalState(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"}]}`)
	versionBefore := app.m.Version()

	err := app.m.Batch([]Operation{
		{Op: "remove", Path: "/rows", Index: 0},
		{Op: "insert", Path: "/rows", Index: 0, Value: row(2, "b")},
		{Op: "insert", Path: "/rows", Index: 1, Value: row(3, "c")},
	}, false)
	if err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if got := app.rows(); !reflect.DeepEqual(got, []string{"2:b", "3:c"}) {
		t.Errorf("rows = %v", got)
	}
	if !reflect.DeepEqual(sorted(app.state), []string{"2:b", "3:c"}) {
		t.Errorf("app state = %v", app.state)
	}
	if app.m.Version() != versionBefore+1 {
		t.Errorf("a batch must bump the version once: %d -> %d", versionBefore, app.m.Version())
	}
	if n := len(app.m.GetHistory()); n != 3 {
		t.Errorf("history events = %d, want 3", n)
	}
}

func TestBatch_RejectsAnInvalidFinalStateBeforeAnyHandler(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"}]}`)
	before := app.m.ConfigJSON()

	if err := app.m.Batch([]Operation{
		{Op: "insert", Path: "/rows", Index: 1, Value: row(2, "b")},
		{Op: "remove", Path: "/rows", Index: 0},
		{Op: "remove", Path: "/rows", Index: 0}, // leaves the table empty
	}, false); err == nil {
		t.Error("expected minItems to fail")
	}
	if err := app.m.Batch([]Operation{{Op: "remove", Path: "/rows", Index: 3}}, false); err == nil {
		t.Error("expected a bounds error")
	}
	if len(app.calls) != 0 || app.m.ConfigJSON() != before {
		t.Errorf("nothing should have been applied: calls=%v", app.calls)
	}
}

func TestBatch_UndoesAppliedOperationsWhenOneFails(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"},{"id":2,"v":"b"},{"id":3,"v":"c"}]}`)
	before, versionBefore := app.m.ConfigJSON(), app.m.Version()
	app.failInsert = "9:x"

	err := app.m.Batch([]Operation{
		{Op: "update", Path: "/rows", Index: 0, Value: row(1, "A")},
		{Op: "remove", Path: "/rows", Index: 2},
		{Op: "insert", Path: "/rows", Index: 2, Value: row(9, "x")}, // refused
	}, false)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "batch operation #2 (insert /rows)") ||
		!strings.Contains(err.Error(), "application refused 9:x") {
		t.Errorf("error = %v", err)
	}

	if got := app.rows(); !reflect.DeepEqual(got, []string{"1:a", "2:b", "3:c"}) {
		t.Errorf("rows = %v", got)
	}
	if !reflect.DeepEqual(sorted(app.state), []string{"1:a", "2:b", "3:c"}) {
		t.Errorf("app state = %v", app.state)
	}
	if app.m.ConfigJSON() != before || app.m.Version() != versionBefore {
		t.Error("persisted config or version changed")
	}
}

func TestBatch_DryRunChangesNothingAndCallsNoHandler(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"}]}`)
	before := app.m.ConfigJSON()

	if err := app.m.Batch([]Operation{
		{Op: "update", Path: "/rows", Index: 0, Value: row(1, "z")},
		{Op: "insert", Path: "/rows", Index: 1, Value: row(2, "b")},
	}, true); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(app.calls) != 0 || app.m.ConfigJSON() != before {
		t.Error("dry run applied something")
	}

	if err := app.m.Batch([]Operation{{Op: "remove", Path: "/rows", Index: 0}}, true); err == nil {
		t.Error("dry run should report minItems")
	}
	if err := app.m.Batch([]Operation{{Op: "replace", Path: "/rows/0/v", Value: "q"}}, true); err == nil {
		t.Error("dry run should report an unregistered path")
	}
}

func postJSON(t *testing.T, m *Manager, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	m.serveConfigRoute(rec, httptest.NewRequest(http.MethodPost, "/config", bytes.NewReader(raw)))
	var out map[string]interface{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestRoutes_UpdateAndBatch(t *testing.T) {
	app := newTableApp(t, `{"rows":[{"id":1,"v":"a"},{"id":2,"v":"b"}]}`)

	rec := httptest.NewRecorder()
	app.m.serveConfigRoute(rec, httptest.NewRequest(http.MethodGet, "/config", nil))
	if !strings.Contains(rec.Body.String(), `"updatable": [`) || !strings.Contains(rec.Body.String(), `"/rows"`) {
		t.Fatalf("GET /config lacks updatable paths: %s", rec.Body.String())
	}

	code, out := postJSON(t, app.m, map[string]interface{}{
		"op": "update", "path": "/rows", "index": 1, "value": row(2, "B"), "version": app.m.Version()})
	if code != http.StatusOK {
		t.Fatalf("update: %d %v", code, out)
	}

	batch := map[string]interface{}{
		"op": "batch", "version": app.m.Version(), "dry_run": true,
		"operations": []interface{}{
			map[string]interface{}{"op": "remove", "path": "/rows", "index": 0},
			map[string]interface{}{"op": "remove", "path": "/rows", "index": 0},
			map[string]interface{}{"op": "insert", "path": "/rows", "index": 0, "value": row(7, "q")},
		},
	}
	if code, out := postJSON(t, app.m, batch); code != http.StatusOK {
		t.Fatalf("dry run: %d %v", code, out)
	}
	if got := app.rows(); !reflect.DeepEqual(got, []string{"1:a", "2:B"}) {
		t.Errorf("dry run changed rows: %v", got)
	}

	batch["dry_run"] = false
	if code, out := postJSON(t, app.m, batch); code != http.StatusOK {
		t.Fatalf("batch: %d %v", code, out)
	}
	if got := app.rows(); !reflect.DeepEqual(got, []string{"7:q"}) {
		t.Errorf("rows = %v", got)
	}

	batch["operations"] = []interface{}{map[string]interface{}{"op": "insert", "path": "/rows", "index": 0, "value": row(8, "r")}}
	if code, _ := postJSON(t, app.m, batch); code != http.StatusConflict {
		t.Errorf("stale version: got %d, want 409", code)
	}
	batch["version"] = app.m.Version()
	batch["operations"] = []interface{}{map[string]interface{}{"op": "remove", "path": "/rows", "index": 0}}
	if code, _ := postJSON(t, app.m, batch); code != http.StatusBadRequest {
		t.Errorf("minItems violation: got %d, want 400", code)
	}
	if got := app.rows(); !reflect.DeepEqual(got, []string{"7:q"}) {
		t.Errorf("rows = %v", got)
	}
}
