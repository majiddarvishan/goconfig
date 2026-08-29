package goconfig

import (
	"context"
	"errors"
	"testing"
)

func TestPublicMutateSupportsExpectedVersion(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	expected := int64(1)
	if err := manager.Mutate(context.Background(), Mutation{
		Operation:       OperationReplace,
		Path:            "/name",
		Value:           "updated",
		ExpectedVersion: &expected,
	}); err != nil {
		t.Fatalf("Mutate() error = %v", err)
	}
	if err := manager.Mutate(context.Background(), Mutation{
		Operation:       OperationReplace,
		Path:            "/name",
		Value:           "stale",
		ExpectedVersion: &expected,
	}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale Mutate() error = %v, want ErrVersionConflict", err)
	}
}

func TestPublicMutateRejectsUnknownOperation(t *testing.T) {
	manager, _ := newTestManager(t)
	if err := manager.Mutate(context.Background(), Mutation{Operation: "unknown"}); !errors.Is(err, ErrInvalidMutation) {
		t.Fatalf("Mutate(unknown) error = %v, want ErrInvalidMutation", err)
	}
}

func TestPublicMutateRejectsUnregisteredPath(t *testing.T) {
	manager, _ := newTestManager(t)
	if err := manager.Replace("/name", "updated"); !errors.Is(err, ErrMutationDenied) {
		t.Fatalf("Replace(unregistered) error = %v, want ErrMutationDenied", err)
	}
}
