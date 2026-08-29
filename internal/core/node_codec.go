package goconfig

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/iancoleman/orderedmap"
)

func parseNodeStrict(value interface{}) (*Node, error) {
	node := &Node{}

	switch typed := value.(type) {
	case *map[string]interface{}:
		if typed == nil {
			node.value = nil
			return node, nil
		}
		return parseNodeStrict(*typed)
	case map[string]interface{}:
		object := make(map[string]*Node, len(typed))
		for key, childValue := range typed {
			child, err := parseNodeStrict(childValue)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			object[key] = child
		}
		node.value = object
	case *orderedmap.OrderedMap:
		if typed == nil {
			node.value = nil
			return node, nil
		}
		object := make(map[string]*Node, len(typed.Keys()))
		for _, key := range typed.Keys() {
			childValue, _ := typed.Get(key)
			child, err := parseNodeStrict(childValue)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			object[key] = child
		}
		node.value = object
	case orderedmap.OrderedMap:
		return parseNodeStrict(&typed)
	case *[]interface{}:
		if typed == nil {
			node.value = nil
			return node, nil
		}
		return parseNodeStrict(*typed)
	case []interface{}:
		array := make([]*Node, len(typed))
		for index, childValue := range typed {
			child, err := parseNodeStrict(childValue)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", index, err)
			}
			array[index] = child
		}
		node.value = array
	case string, bool, int, int64, float64, json.Number, nil:
		if number, ok := typed.(float64); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
			return nil, fmt.Errorf("non-finite numeric value %v", number)
		}
		node.value = typed
	case int8:
		node.value = int64(typed)
	case int16:
		node.value = int64(typed)
	case int32:
		node.value = int64(typed)
	case uint:
		return parseUnsignedNode(uint64(typed))
	case uint8:
		return parseUnsignedNode(uint64(typed))
	case uint16:
		return parseUnsignedNode(uint64(typed))
	case uint32:
		return parseUnsignedNode(uint64(typed))
	case uint64:
		return parseUnsignedNode(typed)
	case float32:
		return parseNodeStrict(float64(typed))
	default:
		return nil, fmt.Errorf("unsupported node value type %T", value)
	}

	return node, nil
}

func parseUnsignedNode(value uint64) (*Node, error) {
	if value > math.MaxInt64 {
		return nil, fmt.Errorf("unsigned value %d exceeds supported integer range", value)
	}
	return &Node{value: int64(value)}, nil
}
