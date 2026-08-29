package goconfig

import (
	"errors"
	"testing"
)

func TestRegisterValidatorCanonicalizesAndRejectsInvalidPaths(t *testing.T) {
	manager, _ := newTestManager(t)
	if err := manager.RegisterValidator("invalid", ValidateRequired()); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("RegisterValidator(invalid) error = %v, want ErrInvalidPath", err)
	}
	if err := manager.RegisterValidator("/name", nil); err == nil {
		t.Fatal("RegisterValidator(nil) error = nil, want error")
	}
	if err := manager.RegisterValidator("/name", ValidateRequired()); err != nil {
		t.Fatalf("RegisterValidator() error = %v", err)
	}
	if manager.CustomValidator() == nil || manager.GetCustomValidator() == nil {
		t.Fatal("custom validator registry is nil")
	}
}
