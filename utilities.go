package goconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/iancoleman/orderedmap"
)

// navigateToParentMap walks all but the last segment of path, descending
// through objects and (for a segment naming an array) through one numeric
// index segment, and returns the OrderedMap that directly contains the
// final path segment, plus that final segment itself. jsonSetByPath/
// jsonRemoveByPath/jsonInsertByPath previously each carried their own
// ~90%-identical copy of this exact traversal.
func navigateToParentMap(jsonMap *orderedmap.OrderedMap, path string) (parent *orderedmap.OrderedMap, lastKey string, err error) {
	if jsonMap == nil {
		return nil, "", errors.New("jsonMap cannot be nil")
	}

	splitPath := strings.Split(path, "/")
	if len(splitPath) == 0 {
		return nil, "", errors.New("invalid path: empty")
	}

	foundMap := jsonMap

	for i := 0; i < len(splitPath)-1; {
		if len(splitPath[i]) == 0 {
			i++
			continue
		}

		found, present := foundMap.Get(splitPath[i])
		if !present {
			return nil, "", fmt.Errorf("path element '%s' not found", splitPath[i])
		}
		if found == nil {
			return nil, "", fmt.Errorf("path element '%s' is null, cannot traverse further", splitPath[i])
		}

		k := reflect.TypeOf(found).Kind()
		switch k {
		case reflect.Map, reflect.Struct:
			// Handle both *OrderedMap and OrderedMap
			if om, ok := found.(*orderedmap.OrderedMap); ok {
				foundMap = om
			} else if om, ok := found.(orderedmap.OrderedMap); ok {
				foundMap = &om
			} else {
				return nil, "", fmt.Errorf("expected OrderedMap at '%s', got %T", splitPath[i], found)
			}
		case reflect.Slice:
			s, ok := found.([]interface{})
			if !ok {
				return nil, "", fmt.Errorf("expected []interface{} at '%s'", splitPath[i])
			}

			if i+1 >= len(splitPath) {
				return nil, "", errors.New("invalid path: missing index after array")
			}

			idx, err := strconv.ParseInt(splitPath[i+1], 10, 32)
			if err != nil {
				return nil, "", fmt.Errorf("invalid array index '%s': %w", splitPath[i+1], err)
			}

			if idx < 0 || int(idx) >= len(s) {
				return nil, "", fmt.Errorf("array index %d out of bounds [0,%d)", idx, len(s))
			}

			found = s[idx]
			i++ // Skip index element

			if om, ok := found.(*orderedmap.OrderedMap); ok {
				foundMap = om
			} else if om, ok := found.(orderedmap.OrderedMap); ok {
				foundMap = &om
			} else {
				return nil, "", fmt.Errorf("expected OrderedMap at array index %d", idx)
			}
		default:
			return nil, "", fmt.Errorf("cannot traverse through type '%v' at '%s'", k, splitPath[i])
		}
		i++
	}

	return foundMap, splitPath[len(splitPath)-1], nil
}

func jsonSetByPath(jsonMap *orderedmap.OrderedMap, path string, value interface{}) error {
	parent, lastKey, err := navigateToParentMap(jsonMap, path)
	if err != nil {
		return err
	}

	parent.Set(lastKey, value)
	return nil
}

func jsonRemoveByPath(jsonMap *orderedmap.OrderedMap, path string, index int) error {
	parent, lastKey, err := navigateToParentMap(jsonMap, path)
	if err != nil {
		return err
	}

	foundList, present := parent.Get(lastKey)
	if !present {
		return fmt.Errorf("path element '%s' not found", lastKey)
	}

	list, ok := foundList.([]interface{})
	if !ok {
		return errors.New("target is not an array")
	}

	if index < 0 || index >= len(list) {
		return fmt.Errorf("index %d out of bounds [0,%d)", index, len(list))
	}

	newList := make([]interface{}, 0, len(list)-1)
	newList = append(newList, list[:index]...)
	newList = append(newList, list[index+1:]...)

	parent.Set(lastKey, newList)
	return nil
}

func jsonInsertByPath(jsonMap *orderedmap.OrderedMap, path string, index int, value interface{}) error {
	parent, lastKey, err := navigateToParentMap(jsonMap, path)
	if err != nil {
		return err
	}

	foundList, present := parent.Get(lastKey)
	if !present {
		return fmt.Errorf("path element '%s' not found", lastKey)
	}

	list, ok := foundList.([]interface{})
	if !ok {
		return errors.New("target is not an array")
	}

	if index < 0 || index > len(list) {
		return fmt.Errorf("index %d out of bounds [0,%d]", index, len(list))
	}

	newList := make([]interface{}, 0, len(list)+1)
	newList = append(newList, list[:index]...)
	newList = append(newList, value)
	newList = append(newList, list[index:]...)

	parent.Set(lastKey, newList)
	return nil
}

func findNodePath(parentNode *Node, desiredNode *Node) string {
	if parentNode == desiredNode {
		return ""
	}

	// Use slice of strings for path segments, join at the end
	var pathSegments []string
	if findNodePathRecursive(parentNode, desiredNode, &pathSegments) {
		if len(pathSegments) == 0 {
			return ""
		}
		return "/" + strings.Join(pathSegments, "/")
	}
	return ""
}

func findNodePathRecursive(parentNode *Node, desiredNode *Node, pathSegments *[]string) bool {
	if parentNode == desiredNode {
		return true
	}

	if parentNode.Type() == Array {
		arr, err := parentNode.GetArray()
		if err == nil {
			for index, innerNode := range arr {
				// Add segment
				*pathSegments = append(*pathSegments, strconv.Itoa(index))

				if findNodePathRecursive(innerNode, desiredNode, pathSegments) {
					return true
				}

				// Backtrack - remove last segment
				*pathSegments = (*pathSegments)[:len(*pathSegments)-1]
			}
		}
	} else if parentNode.Type() == Object {
		obj, err := parentNode.GetObject()
		if err == nil {
			for key, innerNode := range obj {
				// Add segment
				*pathSegments = append(*pathSegments, key)

				if findNodePathRecursive(innerNode, desiredNode, pathSegments) {
					return true
				}

				// Backtrack - remove last segment
				*pathSegments = (*pathSegments)[:len(*pathSegments)-1]
			}
		}
	}

	return false
}

func parseNode(value any) *Node {
	node := &Node{}

	switch v := value.(type) {
	case *map[string]interface{}:
		obj := make(map[string]*Node)
		for k, val := range *v {
			obj[k] = parseNode(val)
		}
		node.value = obj

	case map[string]interface{}:
		obj := make(map[string]*Node)
		for k, val := range v {
			obj[k] = parseNode(val)
		}
		node.value = obj

	case *orderedmap.OrderedMap:
		obj := make(map[string]*Node)
		keys := v.Keys()
		for _, k := range keys {
			val, _ := v.Get(k)
			obj[k] = parseNode(val)
		}
		node.value = obj

	case orderedmap.OrderedMap:
		obj := make(map[string]*Node)
		keys := v.Keys()
		for _, k := range keys {
			val, _ := v.Get(k)
			obj[k] = parseNode(val)
		}
		node.value = obj

	case *[]interface{}:
		arr := make([]*Node, 0, len(*v))
		for _, val := range *v {
			arr = append(arr, parseNode(val))
		}
		node.value = arr

	case []interface{}:
		arr := make([]*Node, 0, len(v))
		for _, val := range v {
			arr = append(arr, parseNode(val))
		}
		node.value = arr

	case string:
		node.value = v
	case int:
		node.value = v
	case int64:
		node.value = v
	case int32:
		node.value = int64(v)
	case int16:
		node.value = int64(v)
	case int8:
		node.value = int64(v)
	case uint:
		node.value = int64(v)
	case uint64:
		node.value = int64(v)
	case uint32:
		node.value = int64(v)
	case uint16:
		node.value = int64(v)
	case uint8:
		node.value = int64(v)
	case float64:
		node.value = v
	case float32:
		node.value = float64(v)
	case bool:
		node.value = v
	case nil:
		node.value = nil
	default:
		node.value = nil
	}

	return node
}

// nodeValueToPlain recursively converts a Node's internal tree
// representation (map[string]*Node / []*Node) into plain Go values
// (map[string]interface{} / []interface{}) suitable for storing outside this
// package - e.g. in a history.ChangeEvent. The raw internal representation
// holds unexported *Node pointers, which print as addresses (%v) and
// serialize to "{}" via encoding/json since Node.value is unexported.
func nodeValueToPlain(n *Node) interface{} {
	if n == nil {
		return nil
	}

	switch v := n.value.(type) {
	case map[string]*Node:
		out := make(map[string]interface{}, len(v))
		for k, child := range v {
			out[k] = nodeValueToPlain(child)
		}
		return out
	case []*Node:
		out := make([]interface{}, len(v))
		for i, child := range v {
			out[i] = nodeValueToPlain(child)
		}
		return out
	default:
		return v
	}
}

// Clone creates a deep copy of an OrderedMap
func Clone(om *orderedmap.OrderedMap) (*orderedmap.OrderedMap, error) {
	if om == nil {
		return nil, errors.New("cannot clone nil OrderedMap")
	}

	data, err := json.Marshal(om)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OrderedMap: %w", err)
	}

	clone := orderedmap.New()
	if err := json.Unmarshal(data, &clone); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OrderedMap: %w", err)
	}

	return clone, nil
}
