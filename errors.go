package goconfig

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidPath     = errors.New("invalid configuration path")
	ErrPathNotFound    = errors.New("configuration path not found")
	ErrTypeMismatch    = errors.New("configuration type mismatch")
	ErrIndexOutOfRange = errors.New("configuration index out of range")
	ErrVersionConflict = errors.New("configuration version conflict")
	ErrValidation      = errors.New("configuration validation failed")
	ErrPersistence     = errors.New("configuration persistence failed")
)

// PathError describes a typed path traversal or mutation failure.
type PathError struct {
	Path string
	Err  error
	Msg  string
}

func (e *PathError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("path %q: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("path %q: %s: %v", e.Path, e.Msg, e.Err)
}

func (e *PathError) Unwrap() error { return e.Err }

// VersionConflictError reports an optimistic concurrency conflict.
type VersionConflictError struct {
	Expected int64
	Current  int64
}

func (e *VersionConflictError) Error() string {
	return fmt.Sprintf("version mismatch: expected %d, current %d", e.Expected, e.Current)
}

func (e *VersionConflictError) Unwrap() error { return ErrVersionConflict }

// ValidationError identifies which validation stage rejected a candidate.
type ValidationError struct {
	Stage string
	Err   error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s validation failed: %v", e.Stage, e.Err)
}

func (e *ValidationError) Unwrap() error { return e.Err }
func (e *ValidationError) Is(target error) bool {
	return target == ErrValidation
}

// PersistenceError wraps a Source failure during commit.
type PersistenceError struct {
	Err error
}

func (e *PersistenceError) Error() string { return fmt.Sprintf("failed to persist config: %v", e.Err) }
func (e *PersistenceError) Unwrap() error { return e.Err }
func (e *PersistenceError) Is(target error) bool {
	return target == ErrPersistence
}
