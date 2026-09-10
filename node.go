package goconfig

import (
	"errors"
	"fmt"
)

type Node struct {
	value interface{}
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
	case string, bool, int, int64, float64:
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
		return int(v), nil
	case float64:
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
	return obj, nil
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
	return arr, nil
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
		return &Node{value: objCopy}

	case []*Node:
		arrCopy := make([]*Node, len(v))
		for i, node := range v {
			arrCopy[i] = node.DeepCopy()
		}
		return &Node{value: arrCopy}

	default:
		// Primitive types are safe to copy directly
		return &Node{value: v}
	}
}
