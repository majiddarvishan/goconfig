package goconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Node struct {
	value interface{}
	owner *Manager
	path  string
}

type NodeType int

const (
	Null NodeType = iota
	Boolean
	Integral
	FloatingPoint
	String
	Object
	Array
)

func (n *Node) Type() NodeType {
	if n == nil {
		return Null
	}

	switch n.value.(type) {
	case nil:
		return Null
	case bool:
		return Boolean
	case int64:
		return Integral
	case int:
		return Integral
	case json.Number:
		if strings.ContainsAny(string(n.value.(json.Number)), ".eE") {
			return FloatingPoint
		}
		return Integral
	case float64:
		return FloatingPoint
	case string:
		return String
	case map[string]*Node:
		return Object
	case []*Node:
		return Array
	default:
		return Null
	}
}

func (n *Node) get() (interface{}, error) {
	if n == nil {
		return nil, errors.New("node is nil")
	}

	switch v := n.value.(type) {
	case string, bool, int, int64, float64, json.Number:
		return v, nil
	case map[string]*Node:
		return v, nil
	case []*Node:
		return v, nil
	case nil:
		return nil, errors.New("node value is nil")
	default:
		return nil, fmt.Errorf("node has invalid type: %T", v)
	}
}

func (n *Node) GetString(param ...string) (string, error) {
	if len(param) > 1 {
		return "", errors.New("too many arguments: expected 0 or 1")
	}
	if len(param) == 1 {
		sn, err := n.atString(param[0])
		if err != nil {
			return "", err
		}
		return sn.getString()
	}
	return n.getString()
}

func (n *Node) GetBool(param ...string) (bool, error) {
	if len(param) > 1 {
		return false, errors.New("too many arguments: expected 0 or 1")
	}
	if len(param) == 1 {
		sn, err := n.atString(param[0])
		if err != nil {
			return false, err
		}
		return sn.getBool()
	}
	return n.getBool()
}

func (n *Node) GetInt(param ...string) (int, error) {
	if len(param) > 1 {
		return 0, errors.New("too many arguments: expected 0 or 1")
	}
	if len(param) == 1 {
		sn, err := n.atString(param[0])
		if err != nil {
			return 0, err
		}
		return sn.getInt()
	}
	return n.getInt()
}

func (n *Node) GetFloat(param ...string) (float64, error) {
	if len(param) > 1 {
		return 0, errors.New("too many arguments: expected 0 or 1")
	}
	if len(param) == 1 {
		sn, err := n.atString(param[0])
		if err != nil {
			return 0, err
		}
		return sn.getFloat()
	}
	return n.getFloat()
}

func (n *Node) getString() (string, error) {
	value, err := n.get()
	if err != nil {
		return "", err
	}
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("node is %T, not string", value)
	}
	return str, nil
}

func (n *Node) getBool() (bool, error) {
	value, err := n.get()
	if err != nil {
		return false, err
	}
	b, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("node is %T, not bool", value)
	}
	return b, nil
}

func (n *Node) getInt() (int, error) {
	value, err := n.get()
	if err != nil {
		return 0, err
	}

	// Handle both int and float64 (JSON numbers are float64)
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return checkedInt(v)
	case json.Number:
		integer, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("node value %q is not an integer: %w", v, err)
		}
		return checkedInt(integer)
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v {
			return 0, fmt.Errorf("node value %v is not an integer", v)
		}
		if (strconv.IntSize == 32 && (v < math.MinInt32 || v > math.MaxInt32)) ||
			(strconv.IntSize == 64 && (v < math.MinInt64 || v >= math.MaxInt64)) {
			return 0, fmt.Errorf("node value %v overflows int", v)
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("node is %T, not numeric", value)
	}
}

func (n *Node) getFloat() (float64, error) {
	value, err := n.get()
	if err != nil {
		return 0, err
	}

	switch v := value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case json.Number:
		parsed, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("invalid numeric node value %q: %w", v, err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("node is %T, not numeric", value)
	}
}

func (n *Node) GetObject() (map[string]*Node, error) {
	value, err := n.get()
	if err != nil {
		return nil, err
	}
	obj, ok := value.(map[string]*Node)
	if !ok {
		return nil, fmt.Errorf("node is %T, not object", value)
	}
	copyObject := make(map[string]*Node, len(obj))
	for key, node := range obj {
		copyObject[key] = node
	}
	return copyObject, nil
}

func (n *Node) GetArray() ([]*Node, error) {
	value, err := n.get()
	if err != nil {
		return nil, err
	}
	arr, ok := value.([]*Node)
	if !ok {
		return nil, fmt.Errorf("node is %T, not array", value)
	}
	return append([]*Node(nil), arr...), nil
}

func (n *Node) atString(key string) (*Node, error) {
	if n == nil {
		return nil, errors.New("node is nil")
	}

	object, ok := n.value.(map[string]*Node)
	if !ok {
		return nil, fmt.Errorf("cannot call At(key) on non-object node (type: %v)", n.Type())
	}

	value, ok := object[key]
	if !ok {
		return nil, fmt.Errorf("key '%s' not found in object", key)
	}

	return value, nil
}

func (n *Node) atInt(index int) (*Node, error) {
	if n == nil {
		return nil, errors.New("node is nil")
	}

	array, ok := n.value.([]*Node)
	if !ok {
		return nil, fmt.Errorf("cannot call At(index) on non-array node (type: %v)", n.Type())
	}

	if index < 0 || index >= len(array) {
		return nil, fmt.Errorf("index %d out of bounds [0,%d)", index, len(array))
	}

	return array[index], nil
}

func (n *Node) At(param interface{}) (*Node, error) {
	switch v := param.(type) {
	case int:
		return n.atInt(v)
	case string:
		return n.atString(v)
	default:
		return nil, fmt.Errorf("param must be int or string, got %T", param)
	}
}

// DeepCopy creates a deep copy of the node tree
func (n *Node) DeepCopy() *Node {
	if n == nil {
		return nil
	}

	switch v := n.value.(type) {
	case map[string]*Node:
		objCopy := make(map[string]*Node, len(v))
		for key, node := range v {
			objCopy[key] = node.DeepCopy()
		}
		return &Node{value: objCopy, owner: n.owner, path: n.path}

	case []*Node:
		arrCopy := make([]*Node, len(v))
		for i, node := range v {
			arrCopy[i] = node.DeepCopy()
		}
		return &Node{value: arrCopy, owner: n.owner, path: n.path}

	default:
		// Primitive types are safe to copy directly
		return &Node{value: v, owner: n.owner, path: n.path}
	}
}

func checkedInt(value int64) (int, error) {
	if value < int64(minInt()) || value > int64(maxInt()) {
		return 0, fmt.Errorf("node value %d overflows int", value)
	}
	return int(value), nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func minInt() int {
	return -maxInt() - 1
}

func bindNodeTree(node *Node, manager *Manager, path string) {
	segments, err := parseJSONPointer(path)
	if err != nil {
		return
	}
	bindNodeTreeSegments(node, manager, segments)
}

func bindNodeTreeSegments(node *Node, manager *Manager, segments []string) {
	if node == nil {
		return
	}
	node.owner = manager
	node.path = buildJSONPointer(segments)

	switch value := node.value.(type) {
	case map[string]*Node:
		for key, child := range value {
			childSegments := appendPathSegment(segments, key)
			bindNodeTreeSegments(child, manager, childSegments)
		}
	case []*Node:
		for index, child := range value {
			childSegments := appendPathSegment(segments, strconv.Itoa(index))
			bindNodeTreeSegments(child, manager, childSegments)
		}
	}
}

func appendPathSegment(segments []string, segment string) []string {
	result := make([]string, len(segments)+1)
	copy(result, segments)
	result[len(segments)] = segment
	return result
}
