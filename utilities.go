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

func jsonSetByPath(jsonMap *orderedmap.OrderedMap, path string, value interface{}) error {
	if jsonMap == nil {
		return errors.New("jsonMap cannot be nil")
	}

	splited_path := strings.Split(path, "/")
	if len(splited_path) == 0 {
		return errors.New("invalid path: empty")
	}

	foundMap := jsonMap

	for i := 0; i < len(splited_path)-1; {
		if len(splited_path[i]) == 0 {
			i++
			continue
		}

		found, present := foundMap.Get(splited_path[i])
		if !present {
			return fmt.Errorf("path element '%s' not found", splited_path[i])
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
				return fmt.Errorf("expected OrderedMap at '%s', got %T", splited_path[i], found)
			}
		case reflect.Slice:
			s, ok := found.([]interface{})
			if !ok {
				return fmt.Errorf("expected []interface{} at '%s'", splited_path[i])
			}

			if i+1 >= len(splited_path) {
				return errors.New("invalid path: missing index after array")
			}

			index, err := strconv.ParseInt(splited_path[i+1], 10, 32)
			if err != nil {
				return fmt.Errorf("invalid array index '%s': %w", splited_path[i+1], err)
			}

			if index < 0 || int(index) >= len(s) {
				return fmt.Errorf("array index %d out of bounds [0,%d)", index, len(s))
			}

			found = s[index]
			i++ // Skip index element

			if om, ok := found.(*orderedmap.OrderedMap); ok {
				foundMap = om
			} else if om, ok := found.(orderedmap.OrderedMap); ok {
				foundMap = &om
			} else {
				return fmt.Errorf("expected OrderedMap at array index %d", index)
			}
		default:
			return fmt.Errorf("cannot traverse through type '%v' at '%s'", k, splited_path[i])
		}
		i++
	}

	foundMap.Set(splited_path[len(splited_path)-1], value)
	return nil
}

func jsonRemoveByPath(jsonMap *orderedmap.OrderedMap, path string, index int) error {
	if jsonMap == nil {
		return errors.New("jsonMap cannot be nil")
	}

	splited_path := strings.Split(path, "/")
	if len(splited_path) == 0 {
		return errors.New("invalid path: empty")
	}

	foundMap := jsonMap

	for i := 0; i < len(splited_path)-1; {
		if len(splited_path[i]) == 0 {
			i++
			continue
		}

		found, present := foundMap.Get(splited_path[i])
		if !present {
			return fmt.Errorf("path element '%s' not found", splited_path[i])
		}

		k := reflect.TypeOf(found).Kind()
		switch k {
		case reflect.Map, reflect.Struct:
			if om, ok := found.(*orderedmap.OrderedMap); ok {
				foundMap = om
			} else if om, ok := found.(orderedmap.OrderedMap); ok {
				foundMap = &om
			} else {
				return fmt.Errorf("expected OrderedMap at '%s'", splited_path[i])
			}
		case reflect.Slice:
			s, ok := found.([]interface{})
			if !ok {
				return fmt.Errorf("expected []interface{} at '%s'", splited_path[i])
			}

			if i+1 >= len(splited_path) {
				return errors.New("invalid path: missing index after array")
			}

			arrayIndex, err := strconv.ParseInt(splited_path[i+1], 10, 32)
			if err != nil {
				return fmt.Errorf("invalid array index '%s': %w", splited_path[i+1], err)
			}

			if arrayIndex < 0 || int(arrayIndex) >= len(s) {
				return fmt.Errorf("array index %d out of bounds", arrayIndex)
			}

			found = s[arrayIndex]
			i++

			if om, ok := found.(*orderedmap.OrderedMap); ok {
				foundMap = om
			} else if om, ok := found.(orderedmap.OrderedMap); ok {
				foundMap = &om
			} else {
				return fmt.Errorf("expected OrderedMap at array index %d", arrayIndex)
			}
		default:
			return fmt.Errorf("cannot traverse through type '%v'", k)
		}
		i++
	}

	found_list, present := foundMap.Get(splited_path[len(splited_path)-1])
	if !present {
		return fmt.Errorf("path element '%s' not found", splited_path[len(splited_path)-1])
	}

	list, ok := found_list.([]interface{})
	if !ok {
		return errors.New("target is not an array")
	}

	if index < 0 || index >= len(list) {
		return fmt.Errorf("index %d out of bounds [0,%d)", index, len(list))
	}

	newList := make([]interface{}, 0, len(list)-1)
	newList = append(newList, list[:index]...)
	newList = append(newList, list[index+1:]...)

	foundMap.Set(splited_path[len(splited_path)-1], newList)
	return nil
}

func jsonInsertByPath(jsonMap *orderedmap.OrderedMap, path string, index int, value interface{}) error {
	if jsonMap == nil {
		return errors.New("jsonMap cannot be nil")
	}

	splited_path := strings.Split(path, "/")
	if len(splited_path) == 0 {
		return errors.New("invalid path: empty")
	}

	foundMap := jsonMap

	for i := 0; i < len(splited_path)-1; {
		if len(splited_path[i]) == 0 {
			i++
			continue
		}

		found, present := foundMap.Get(splited_path[i])
		if !present {
			return fmt.Errorf("path element '%s' not found", splited_path[i])
		}

		k := reflect.TypeOf(found).Kind()
		switch k {
		case reflect.Map, reflect.Struct:
			if om, ok := found.(*orderedmap.OrderedMap); ok {
				foundMap = om
			} else if om, ok := found.(orderedmap.OrderedMap); ok {
				foundMap = &om
			} else {
				return fmt.Errorf("expected OrderedMap at '%s'", splited_path[i])
			}
		case reflect.Slice:
			s, ok := found.([]interface{})
			if !ok {
				return fmt.Errorf("expected []interface{} at '%s'", splited_path[i])
			}

			if i+1 >= len(splited_path) {
				return errors.New("invalid path: missing index after array")
			}

			arrayIndex, err := strconv.ParseInt(splited_path[i+1], 10, 32)
			if err != nil {
				return fmt.Errorf("invalid array index '%s': %w", splited_path[i+1], err)
			}

			if arrayIndex < 0 || int(arrayIndex) >= len(s) {
				return fmt.Errorf("array index %d out of bounds", arrayIndex)
			}

			found = s[arrayIndex]
			i++

			if om, ok := found.(*orderedmap.OrderedMap); ok {
				foundMap = om
			} else if om, ok := found.(orderedmap.OrderedMap); ok {
				foundMap = &om
			} else {
				return fmt.Errorf("expected OrderedMap at array index %d", arrayIndex)
			}
		default:
			return fmt.Errorf("cannot traverse through type '%v'", k)
		}
		i++
	}

	found_list, present := foundMap.Get(splited_path[len(splited_path)-1])
	if !present {
		return fmt.Errorf("path element '%s' not found", splited_path[len(splited_path)-1])
	}

	list, ok := found_list.([]interface{})
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

	foundMap.Set(splited_path[len(splited_path)-1], newList)
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
	case float64:
		node.value = v
	case bool:
		node.value = v
	case nil:
		node.value = nil
	default:
		node.value = nil
	}

	return node
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
