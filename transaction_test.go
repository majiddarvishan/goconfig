package goconfig

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iancoleman/orderedmap"
)

func TestHandlerDoesNotObserveUncommittedState(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, func(candidate *Node) error {
		if got, err := candidate.GetString(); err != nil || got != "updated" {
			t.Fatalf("handler candidate = %q, %v", got, err)
		}
		visible, err := mustNodeAt(t, manager.Config(), "name").GetString()
		if err != nil || visible != "demo" {
			t.Fatalf("visible value during handler = %q, %v; want committed demo", visible, err)
		}
		return nil
	}); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}

	if err := manager.replace("/name", "updated"); err != nil {
		t.Fatalf("replace() error = %v", err)
	}
}

func TestPersistenceFailureDoesNotPublishCandidate(t *testing.T) {
	source := newFailingLegacySource(t)
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	source.writeErr = errors.New("disk unavailable")

	err = manager.replace("/name", "updated")
	if !errors.Is(err, ErrPersistence) {
		t.Fatalf("replace() error = %v, want ErrPersistence", err)
	}
	if manager.Version() != 1 {
		t.Fatalf("version = %d, want 1", manager.Version())
	}
	if got, _ := mustNodeAt(t, manager.Config(), "name").GetString(); got != "demo" {
		t.Fatalf("visible name = %q, want demo", got)
	}
	if len(manager.GetHistory()) != 0 {
		t.Fatal("failed persistence added a history event")
	}
}

func TestConcurrentExpectedVersionAllowsSingleCommit(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}

	expected := int64(1)
	start := make(chan struct{})
	errorsByCall := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, value := range []string{"first", "second"} {
		value := value
		go func() {
			ready.Done()
			<-start
			errorsByCall <- manager.mutate(nil, mutationRequest{
				kind:            mutationReplace,
				path:            "/name",
				value:           value,
				expectedVersion: &expected,
			})
		}()
	}
	ready.Wait()
	close(start)

	var successes, conflicts int
	for i := 0; i < 2; i++ {
		err := <-errorsByCall
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrVersionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected mutation error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d; want 1 and 1", successes, conflicts)
	}
	if manager.Version() != 2 || len(manager.GetHistory()) != 1 {
		t.Fatalf("version/history = %d/%d, want 2/1", manager.Version(), len(manager.GetHistory()))
	}
}

func TestReentrantHandlerCausesOuterConflictWithoutDeadlock(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	port := mustNodeAt(t, manager.Config(), "port")
	if err := manager.OnReplace(port, nil); err != nil {
		t.Fatalf("OnReplace(port) error = %v", err)
	}
	if err := manager.OnReplace(name, func(*Node) error {
		return manager.replace("/port", json.Number("9090"))
	}); err != nil {
		t.Fatalf("OnReplace(name) error = %v", err)
	}

	err := manager.replace("/name", "updated")
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("outer replace error = %v, want version conflict", err)
	}
	if got, _ := mustNodeAt(t, manager.Config(), "name").GetString(); got != "demo" {
		t.Fatalf("name = %q, want demo", got)
	}
	if got, _ := mustNodeAt(t, manager.Config(), "port").GetInt(); got != 9090 {
		t.Fatalf("port = %d, want 9090", got)
	}
}

func TestReplaceArrayElementAndNestedArrayValue(t *testing.T) {
	manager, source := newTestManager(t)
	firstItem := mustNodeAt(t, manager.Config(), "items", 0)
	firstTag := mustNodeAt(t, manager.Config(), "nested", "tags", 0)
	if err := manager.OnReplace(firstItem, nil); err != nil {
		t.Fatalf("OnReplace(item) error = %v", err)
	}
	if err := manager.OnReplace(firstTag, nil); err != nil {
		t.Fatalf("OnReplace(tag) error = %v", err)
	}

	if err := manager.replace("/items/0", map[string]interface{}{"id": 10, "name": "ten"}); err != nil {
		t.Fatalf("replace array element error = %v", err)
	}
	if err := manager.replace("/nested/tags/0", "changed"); err != nil {
		t.Fatalf("replace nested array value error = %v", err)
	}

	if got, _ := mustNodeAt(t, manager.Config(), "items", 0, "name").GetString(); got != "ten" {
		t.Fatalf("item name = %q, want ten", got)
	}
	if got, _ := mustNodeAt(t, manager.Config(), "nested", "tags", 0).GetString(); got != "changed" {
		t.Fatalf("tag = %q, want changed", got)
	}
	persisted := decodeSourceConfig(t, source)
	items, _ := persisted.Get("items")
	if len(items.([]interface{})) != 2 {
		t.Fatalf("persisted item count = %d", len(items.([]interface{})))
	}
}

func TestRegistrationFollowsArrayElementAfterRemoval(t *testing.T) {
	manager, _ := newTestManager(t)
	items := mustNodeAt(t, manager.Config(), "items")
	secondName := mustNodeAt(t, manager.Config(), "items", 1, "name")
	if err := manager.OnRemove(items, nil); err != nil {
		t.Fatalf("OnRemove() error = %v", err)
	}
	if err := manager.OnReplace(secondName, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}

	if err := manager.remove("/items", 0); err != nil {
		t.Fatalf("remove() error = %v", err)
	}
	if err := manager.replace("/items/0/name", "shifted"); err != nil {
		t.Fatalf("replace shifted registration error = %v", err)
	}
	if got, _ := mustNodeAt(t, manager.Config(), "items", 0, "name").GetString(); got != "shifted" {
		t.Fatalf("shifted name = %q, want shifted", got)
	}
}

func TestPostCommitObserverSeesCommittedState(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}

	called := false
	manager.AddObserver(func(change Change) {
		called = true
		if change.Version != 2 || change.Operation != "replace" || change.Path != "/name" {
			t.Errorf("observer change = %#v", change)
		}
		visible, _ := mustNodeAt(t, manager.Config(), "name").GetString()
		if visible != "updated" {
			t.Errorf("observer visible value = %q, want updated", visible)
		}
	})
	if err := manager.replace("/name", "updated"); err != nil {
		t.Fatalf("replace() error = %v", err)
	}
	if !called {
		t.Fatal("observer was not called")
	}
}

func TestMutationUsesEscapedJSONPointerPaths(t *testing.T) {
	source, err := NewStrSource(`{"a/b":{"m~n":"old"}}`, `{"type":"object"}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	target := mustNodeAt(t, manager.Config(), "a/b", "m~n")
	if err := manager.OnReplace(target, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	if err := manager.replace("/a~1b/m~0n", "new"); err != nil {
		t.Fatalf("replace escaped path error = %v", err)
	}
	if got, _ := mustNodeAt(t, manager.Config(), "a/b", "m~n").GetString(); got != "new" {
		t.Fatalf("escaped value = %q, want new", got)
	}
}

func TestRootReplacementUsesEmptyJSONPointer(t *testing.T) {
	source, err := NewStrSource(`{"name":"old"}`, `{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.OnReplace(manager.Config(), nil); err != nil {
		t.Fatalf("OnReplace(root) error = %v", err)
	}
	if err := manager.replace("", map[string]interface{}{"name": "new"}); err != nil {
		t.Fatalf("replace root error = %v", err)
	}
	if got, _ := manager.Config().GetString("name"); got != "new" {
		t.Fatalf("root name = %q, want new", got)
	}
}

func TestMutationReturnsTypedPathErrors(t *testing.T) {
	source, err := NewStrSource(testConfigJSON, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	config := source.getConfigObject()
	_, err = applyMutation(config, mutationReplace, "/items/00", 0, map[string]interface{}{"id": 3, "name": "three"})
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("applyMutation() error = %v, want ErrInvalidPath", err)
	}
}

func TestConfiguredExternalValidationFailsClosed(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	service := NewvalidationService("http://validator.test/validate", 0)
	service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"valid":false,"message":"rejected"}`)),
			Request:    request,
		}, nil
	})}
	manager.SetValidationService(service)

	err := manager.replace("/name", "updated")
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("replace() error = %v, want ErrValidation", err)
	}
	if manager.Version() != 1 {
		t.Fatalf("version = %d, want 1", manager.Version())
	}
	if got, _ := mustNodeAt(t, manager.Config(), "name").GetString(); got != "demo" {
		t.Fatalf("name = %q, want demo", got)
	}
}

func TestExternalValidationUsesMutationContext(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	service := NewValidationService("http://validator.test/validate", time.Second)
	service.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	manager.SetValidationService(service)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := manager.mutate(ctx, mutationRequest{kind: mutationReplace, path: "/name", value: "updated"})
	if !errors.Is(err, ErrValidation) || !errors.Is(err, context.Canceled) {
		t.Fatalf("mutate() error = %v, want validation wrapping context.Canceled", err)
	}
	if manager.Version() != 1 {
		t.Fatalf("version = %d, want 1", manager.Version())
	}
}

func TestInsertValidatorReceivesCompleteCandidateArray(t *testing.T) {
	manager, _ := newTestManager(t)
	items := mustNodeAt(t, manager.Config(), "items")
	if err := manager.OnInsert(items, nil); err != nil {
		t.Fatalf("OnInsert() error = %v", err)
	}
	manager.AddValidator("/items", ValidateUnique("id"))

	err := manager.insert("/items", 1, map[string]interface{}{"id": 1, "name": "duplicate"})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("insert() error = %v, want ErrValidation", err)
	}
	array, _ := mustNodeAt(t, manager.Config(), "items").GetArray()
	if manager.Version() != 1 || len(array) != 2 {
		t.Fatal("failed validation changed manager state")
	}
}

func TestRemoveValidatorReceivesCompleteCandidateArray(t *testing.T) {
	manager, _ := newTestManager(t)
	items := mustNodeAt(t, manager.Config(), "items")
	if err := manager.OnRemove(items, nil); err != nil {
		t.Fatalf("OnRemove() error = %v", err)
	}
	manager.AddValidator("/items", func(_ string, oldValue, newValue *Node) error {
		oldItems, _ := oldValue.GetArray()
		newItems, _ := newValue.GetArray()
		if len(oldItems) != 2 || len(newItems) != 1 {
			return errors.New("validator did not receive complete arrays")
		}
		return nil
	})

	if err := manager.remove("/items", 0); err != nil {
		t.Fatalf("remove() error = %v", err)
	}
}

func TestCommittedDirectorySyncFailureKeepsManagerAndFileConsistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(testConfigJSON), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	source, err := NewFileSource(path, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewFileSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	source.ops.dirSync = func(*os.File) error { return errors.New("injected directory sync failure") }

	if err := manager.replace("/name", "committed"); err != nil {
		t.Fatalf("replace() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	got, _ := mustNodeAt(t, manager.Config(), "name").GetString()
	if got != "committed" || !strings.Contains(string(data), `"committed"`) {
		t.Fatalf("manager and disk diverged: manager=%q disk=%s", got, data)
	}
}

func TestManagerPreservesSerializedObjectOrder(t *testing.T) {
	manager, source := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	if err := manager.replace("/name", "updated"); err != nil {
		t.Fatalf("replace() error = %v", err)
	}

	raw := *source.getConfig()
	namePosition := strings.Index(raw, `"name"`)
	enabledPosition := strings.Index(raw, `"enabled"`)
	portPosition := strings.Index(raw, `"port"`)
	if namePosition < 0 || enabledPosition <= namePosition || portPosition <= enabledPosition {
		t.Fatalf("serialized key order changed: %s", raw)
	}
}

type failingLegacySource struct {
	object   *orderedmap.OrderedMap
	config   string
	schema   string
	writeErr error
}

func newFailingLegacySource(t *testing.T) *failingLegacySource {
	t.Helper()
	object, err := parseConfig([]byte(testConfigJSON))
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	return &failingLegacySource{object: object, config: testConfigJSON, schema: testSchemaJSON}
}

func (s *failingLegacySource) getConfigObject() *orderedmap.OrderedMap {
	clone, _ := Clone(s.object)
	return clone
}

func (s *failingLegacySource) getConfig() *string {
	value := s.config
	return &value
}

func (s *failingLegacySource) getSchema() *string {
	value := s.schema
	return &value
}

func (s *failingLegacySource) setConfig(config *orderedmap.OrderedMap) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	clone, err := Clone(config)
	if err != nil {
		return err
	}
	s.object = clone
	return nil
}
