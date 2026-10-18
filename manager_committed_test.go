package goconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func committedTestManager(t *testing.T, document string) (*Manager, *Node) {
	t.Helper()
	source, err := NewStrSource(document, "{}")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatal(err)
	}
	node, err := manager.Config().At("level")
	if err != nil {
		t.Fatal(err)
	}
	return manager, node
}

func registerLevelValidation(t *testing.T, manager *Manager, node *Node) {
	t.Helper()
	if err := manager.OnReplace(node, func(replacement *Node) error {
		value, err := replacement.GetString()
		if err != nil {
			return err
		}
		if value != "info" && value != "debug" && value != "warn" {
			return errors.New("invalid level")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A successful persisted replacement broadcasts to both observers, while a
// rejected value invokes neither observer and leaves the node unchanged.
func TestOnReplaceCommittedBroadcastsAfterSuccessfulReplace(t *testing.T) {
	manager, node := committedTestManager(t, `{"level":"info"}`)
	registerLevelValidation(t, manager, node)

	var mu sync.Mutex
	var observed []string
	for i := 0; i < 2; i++ {
		index := i
		if err := manager.OnReplaceCommitted(node, func(n *Node) {
			level, err := n.GetString()
			if err != nil {
				t.Errorf("committed node: %v", err)
			}
			if manager.Version() != 2 {
				t.Errorf("callback %d ran before version increment", index)
			}
			if !strings.Contains(manager.ConfigJSON(), `"level": "debug"`) {
				t.Errorf("callback %d ran before source update", index)
			}
			mu.Lock()
			observed = append(observed, level)
			mu.Unlock()
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := manager.Replace("/level", "debug"); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 2 || observed[0] != "debug" || observed[1] != "debug" {
		t.Fatalf("committed callback observations = %v, want two debug values", observed)
	}
	if err := manager.Replace("/level", "invalid"); err == nil {
		t.Fatal("invalid replacement unexpectedly succeeded")
	}
	if len(observed) != 2 {
		t.Fatalf("validator failure emitted a committed callback: %v", observed)
	}
	level, _ := node.GetString()
	if level != "debug" {
		t.Fatalf("rejected value modified node to %q", level)
	}
}

// A FileSource failure after validation must not emit any committed callback,
// so the application can safely defer all runtime changes until commit.
func TestOnReplaceCommittedNotCalledAfterFilePersistenceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := `{"level":"info"}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := NewFileSource(path, "{}")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatal(err)
	}
	node, err := manager.Config().At("level")
	if err != nil {
		t.Fatal(err)
	}
	registerLevelValidation(t, manager, node)
	calls := 0
	if err := manager.OnReplaceCommitted(node, func(*Node) { calls++ }); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := manager.Replace("/level", "debug"); err == nil {
		t.Fatal("expected FileSource temp write to fail")
	}
	if calls != 0 {
		t.Fatalf("failed persistence delivered %d committed callbacks", calls)
	}
	level, _ := node.GetString()
	if level != "info" {
		t.Fatalf("failed persistence changed node to %q", level)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("failed persistence changed file to %q", content)
	}
}

// A successful batch notifies observers after the entire transaction, once
// per path, with the final value rather than each intermediate replacement.
func TestOnReplaceCommittedBatchDeliversFinalValueOnce(t *testing.T) {
	manager, node := committedTestManager(t, `{"level":"info"}`)
	registerLevelValidation(t, manager, node)
	var got []string
	for i := 0; i < 2; i++ {
		if err := manager.OnReplaceCommitted(node, func(n *Node) {
			value, err := n.GetString()
			if err != nil {
				t.Error(err)
				return
			}
			got = append(got, value)
		}); err != nil {
			t.Fatal(err)
		}
	}
	ops := []Operation{
		{Op: "replace", Path: "/level", Value: "warn"},
		{Op: "replace", Path: "/level", Value: "debug"},
	}
	if err := manager.Batch(ops, true); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("dry-run triggered %d callbacks", len(got))
	}
	if err := manager.Batch(ops, false); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "debug" || got[1] != "debug" {
		t.Fatalf("batch delivered callbacks %v, want [debug debug]", got)
	}
}

// Registration validates the node and callback and does not make a path
// replaceable by itself.
func TestOnReplaceCommittedValidationAndRegistration(t *testing.T) {
	manager, node := committedTestManager(t, `{"level":"info"}`)
	if err := manager.OnReplaceCommitted(nil, func(*Node) {}); err == nil {
		t.Fatal("nil node accepted")
	}
	if err := manager.OnReplaceCommitted(node, nil); err == nil {
		t.Fatal("nil observer accepted")
	}
	if err := manager.OnReplaceCommitted(node, func(*Node) {}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Replace("/level", "debug"); err == nil {
		t.Fatal("committed-only observer unexpectedly made the path replaceable")
	}
}
// Batch failure must not publish post-commit notifications, even if the
 // validation callback accepted the replacement before persistence failed.
func TestOnReplaceCommittedBatchPersistenceFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "batch.json")
	original := `{"level":"info"}`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	source, err := NewFileSource(path, "{}")
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatal(err)
	}
	node, err := manager.Config().At("level")
	if err != nil {
		t.Fatal(err)
	}
	registerLevelValidation(t, manager, node)
	calls := 0
	if err := manager.OnReplaceCommitted(node, func(*Node) { calls++ }); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	version := manager.Version()
	if err := manager.Batch([]Operation{{Op: "replace", Path: "/level", Value: "debug"}}, false); err == nil {
		t.Fatal("batch unexpectedly persisted with blocked temporary file")
	}
	if calls != 0 {
		t.Fatalf("failed batch sent %d committed notifications", calls)
	}
	if manager.Version() != version {
		t.Fatal("failed batch changed manager version")
	}
	level, err := node.GetString()
	if err != nil || level != "info" {
		t.Fatalf("failed batch changed config node: %q (%v)", level, err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(bytes) != original {
		t.Fatalf("failed batch changed config file: %q", bytes)
	}
}

// A callback may query the Manager and register a new observer. The new
// observer is not called until a later successful replacement.
func TestOnReplaceCommittedRegistrationDuringNotification(t *testing.T) {
	manager, node := committedTestManager(t, `{"level":"info"}`)
	registerLevelValidation(t, manager, node)
	primaryCalls := 0
	secondaryCalls := 0
	if err := manager.OnReplaceCommitted(node, func(*Node) {
		primaryCalls++
		if primaryCalls == 1 {
			if err := manager.OnReplaceCommitted(node, func(*Node) {
				secondaryCalls++
			}); err != nil {
				t.Errorf("register observer: %v", err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Replace("/level", "debug"); err != nil {
		t.Fatal(err)
	}
	if primaryCalls != 1 || secondaryCalls != 0 {
		t.Fatalf("first notification delivered primary=%d secondary=%d, want 1/0", primaryCalls, secondaryCalls)
	}
	if err := manager.Replace("/level", "warn"); err != nil {
		t.Fatal(err)
	}
	if primaryCalls != 2 || secondaryCalls != 1 {
		t.Fatalf("second notification delivered primary=%d secondary=%d, want 2/1", primaryCalls, secondaryCalls)
	}
}

