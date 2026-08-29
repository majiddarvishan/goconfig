package goconfig

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/iancoleman/orderedmap"
)

type mutationKind string

const (
	mutationInsert  mutationKind = "insert"
	mutationRemove  mutationKind = "remove"
	mutationReplace mutationKind = "replace"
)

type mutationChange struct {
	oldValue     interface{}
	newValue     interface{}
	oldCandidate interface{}
	newCandidate interface{}
}

func applyMutation(root *orderedmap.OrderedMap, kind mutationKind, pointer string, index int, value interface{}) (mutationChange, error) {
	segments, err := parseJSONPointer(pointer)
	if err != nil {
		return mutationChange{}, &PathError{Path: pointer, Err: ErrInvalidPath, Msg: err.Error()}
	}

	switch kind {
	case mutationInsert:
		target, err := lookupJSONValue(root, segments, pointer)
		if err != nil {
			return mutationChange{}, err
		}
		array, ok := target.([]interface{})
		if !ok {
			return mutationChange{}, &PathError{Path: pointer, Err: ErrTypeMismatch, Msg: fmt.Sprintf("insert target is %T, want array", target)}
		}
		if index < 0 || index > len(array) {
			return mutationChange{}, &PathError{Path: pointer, Err: ErrIndexOutOfRange, Msg: fmt.Sprintf("index %d outside [0,%d]", index, len(array))}
		}
		normalized, err := cloneJSONValue(value)
		if err != nil {
			return mutationChange{}, fmt.Errorf("invalid insert value: %w", err)
		}
		updated := make([]interface{}, 0, len(array)+1)
		updated = append(updated, array[:index]...)
		updated = append(updated, normalized)
		updated = append(updated, array[index:]...)
		if err := setJSONValue(root, segments, updated, pointer); err != nil {
			return mutationChange{}, err
		}
		oldCandidate, err := cloneJSONValue(array)
		if err != nil {
			return mutationChange{}, err
		}
		newCandidate, err := cloneJSONValue(updated)
		if err != nil {
			return mutationChange{}, err
		}
		return mutationChange{newValue: normalized, oldCandidate: oldCandidate, newCandidate: newCandidate}, nil

	case mutationRemove:
		target, err := lookupJSONValue(root, segments, pointer)
		if err != nil {
			return mutationChange{}, err
		}
		array, ok := target.([]interface{})
		if !ok {
			return mutationChange{}, &PathError{Path: pointer, Err: ErrTypeMismatch, Msg: fmt.Sprintf("remove target is %T, want array", target)}
		}
		if index < 0 || index >= len(array) {
			return mutationChange{}, &PathError{Path: pointer, Err: ErrIndexOutOfRange, Msg: fmt.Sprintf("index %d outside [0,%d)", index, len(array))}
		}
		oldValue, err := cloneJSONValue(array[index])
		if err != nil {
			return mutationChange{}, err
		}
		updated := make([]interface{}, 0, len(array)-1)
		updated = append(updated, array[:index]...)
		updated = append(updated, array[index+1:]...)
		if err := setJSONValue(root, segments, updated, pointer); err != nil {
			return mutationChange{}, err
		}
		oldCandidate, err := cloneJSONValue(array)
		if err != nil {
			return mutationChange{}, err
		}
		newCandidate, err := cloneJSONValue(updated)
		if err != nil {
			return mutationChange{}, err
		}
		return mutationChange{oldValue: oldValue, oldCandidate: oldCandidate, newCandidate: newCandidate}, nil

	case mutationReplace:
		oldValue, err := lookupJSONValue(root, segments, pointer)
		if err != nil {
			return mutationChange{}, err
		}
		oldValue, err = cloneJSONValue(oldValue)
		if err != nil {
			return mutationChange{}, err
		}
		normalized, err := cloneJSONValue(value)
		if err != nil {
			return mutationChange{}, fmt.Errorf("invalid replacement value: %w", err)
		}
		if err := setJSONValue(root, segments, normalized, pointer); err != nil {
			return mutationChange{}, err
		}
		return mutationChange{oldValue: oldValue, newValue: normalized, oldCandidate: oldValue, newCandidate: normalized}, nil
	default:
		return mutationChange{}, fmt.Errorf("unsupported mutation kind %q", kind)
	}
}

func lookupJSONValue(root *orderedmap.OrderedMap, segments []string, pointer string) (interface{}, error) {
	var current interface{} = root
	for position, segment := range segments {
		switch typed := current.(type) {
		case *orderedmap.OrderedMap:
			value, found := typed.Get(segment)
			if !found {
				return nil, &PathError{Path: pointer, Err: ErrPathNotFound, Msg: fmt.Sprintf("segment %q at position %d", segment, position)}
			}
			current = value
		case orderedmap.OrderedMap:
			value, found := typed.Get(segment)
			if !found {
				return nil, &PathError{Path: pointer, Err: ErrPathNotFound, Msg: fmt.Sprintf("segment %q at position %d", segment, position)}
			}
			current = value
		case map[string]interface{}:
			value, found := typed[segment]
			if !found {
				return nil, &PathError{Path: pointer, Err: ErrPathNotFound, Msg: fmt.Sprintf("segment %q at position %d", segment, position)}
			}
			current = value
		case []interface{}:
			index, err := parseCanonicalArrayIndex(segment)
			if err != nil {
				return nil, &PathError{Path: pointer, Err: ErrInvalidPath, Msg: err.Error()}
			}
			if index >= len(typed) {
				return nil, &PathError{Path: pointer, Err: ErrIndexOutOfRange, Msg: fmt.Sprintf("index %d outside [0,%d)", index, len(typed))}
			}
			current = typed[index]
		default:
			return nil, &PathError{Path: pointer, Err: ErrTypeMismatch, Msg: fmt.Sprintf("cannot traverse segment %q through %T", segment, current)}
		}
	}
	return current, nil
}

func setJSONValue(root *orderedmap.OrderedMap, segments []string, value interface{}, pointer string) error {
	if len(segments) == 0 {
		replacement, ok := value.(*orderedmap.OrderedMap)
		if !ok {
			return &PathError{Path: pointer, Err: ErrTypeMismatch, Msg: fmt.Sprintf("config root must remain an object, got %T", value)}
		}
		owned, err := Clone(replacement)
		if err != nil {
			return err
		}
		*root = *owned
		return nil
	}

	parent, err := lookupJSONValue(root, segments[:len(segments)-1], pointer)
	if err != nil {
		return err
	}
	last := segments[len(segments)-1]
	switch typed := parent.(type) {
	case *orderedmap.OrderedMap:
		if _, found := typed.Get(last); !found {
			return &PathError{Path: pointer, Err: ErrPathNotFound, Msg: fmt.Sprintf("segment %q", last)}
		}
		typed.Set(last, value)
	case orderedmap.OrderedMap:
		if _, found := typed.Get(last); !found {
			return &PathError{Path: pointer, Err: ErrPathNotFound, Msg: fmt.Sprintf("segment %q", last)}
		}
		typed.Set(last, value)
	case map[string]interface{}:
		if _, found := typed[last]; !found {
			return &PathError{Path: pointer, Err: ErrPathNotFound, Msg: fmt.Sprintf("segment %q", last)}
		}
		typed[last] = value
	case []interface{}:
		index, err := parseCanonicalArrayIndex(last)
		if err != nil {
			return &PathError{Path: pointer, Err: ErrInvalidPath, Msg: err.Error()}
		}
		if index >= len(typed) {
			return &PathError{Path: pointer, Err: ErrIndexOutOfRange, Msg: fmt.Sprintf("index %d outside [0,%d)", index, len(typed))}
		}
		typed[index] = value
	default:
		return &PathError{Path: pointer, Err: ErrTypeMismatch, Msg: fmt.Sprintf("cannot assign through %T", parent)}
	}
	return nil
}

func parseCanonicalArrayIndex(segment string) (int, error) {
	if segment == "" || (len(segment) > 1 && segment[0] == '0') || strings.HasPrefix(segment, "+") || strings.HasPrefix(segment, "-") {
		return 0, fmt.Errorf("invalid canonical array index %q", segment)
	}
	index, err := strconv.ParseInt(segment, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("invalid array index %q", segment)
	}
	return int(index), nil
}

func nodeAtJSONPointer(root *Node, pointer string) (*Node, error) {
	segments, err := parseJSONPointer(pointer)
	if err != nil {
		return nil, err
	}
	current := root
	for _, segment := range segments {
		if current.Type() == Array {
			index, err := parseCanonicalArrayIndex(segment)
			if err != nil {
				return nil, err
			}
			current, err = current.atInt(index)
			if err != nil {
				return nil, err
			}
			continue
		}
		current, err = current.atString(segment)
		if err != nil {
			return nil, err
		}
	}
	return current, nil
}
