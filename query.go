package goconfig

import (
	"fmt"
	"strconv"
	"strings"
)

// QueryResult represents a node found by a query with its path
type QueryResult struct {
	Path string
	Node *Node
}

// Query searches the configuration tree using a simple query language
// Supports:
//   - "/path/to/key" - direct path
//   - "/users/*/name" - wildcard for any key
//   - "/items/[*]" - all array elements
//   - "/items/[0]" - specific array index
//   - "/users/[?age>18]" - filter by condition
func (m *Manager) Query(query string) ([]QueryResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.queryLocked(query)
}

// queryLocked performs query without acquiring lock (caller must hold lock)
func (m *Manager) queryLocked(query string) ([]QueryResult, error) {
	if query == "" || query == "/" {
		return []QueryResult{{Path: "/", Node: m.config.DeepCopy()}}, nil
	}

	// Parse query into segments
	segments, err := parseQuerySegments(query)
	if err != nil {
		return nil, err
	}

	// Execute query
	return m.executeQuery(m.config, "", segments)
}

// QueryOne returns the first result or error if not found
func (m *Manager) QueryOne(query string) (*QueryResult, error) {
	results, err := m.Query(query)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no results found for query: %s", query)
	}
	return &results[0], nil
}

// FindAll finds all nodes matching a predicate
func (m *Manager) FindAll(predicate func(*Node) bool) []QueryResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]QueryResult, 0)
	m.findAllRecursive(m.config, "", predicate, &results)
	return results
}

func (m *Manager) findAllRecursive(node *Node, path string, predicate func(*Node) bool, results *[]QueryResult) {
	if predicate(node) {
		*results = append(*results, QueryResult{
			Path: path,
			Node: node.DeepCopy(),
		})
	}

	if node.Type() == Object {
		obj, _ := node.GetObject()
		for key, child := range obj {
			childPath := path + "/" + key
			m.findAllRecursive(child, childPath, predicate, results)
		}
	} else if node.Type() == Array {
		arr, _ := node.GetArray()
		for i, child := range arr {
			childPath := path + "/" + strconv.Itoa(i)
			m.findAllRecursive(child, childPath, predicate, results)
		}
	}
}

type querySegment struct {
	Type      string // "key", "wildcard", "array", "filter"
	Value     string
	Condition *filterCondition
}

type filterCondition struct {
	Field    string
	Operator string // ">", "<", ">=", "<=", "==", "!="
	Value    interface{}
}

func parseQuerySegments(query string) ([]querySegment, error) {
	if !strings.HasPrefix(query, "/") {
		return nil, fmt.Errorf("query must start with '/'")
	}

	parts := strings.Split(query[1:], "/")
	segments := make([]querySegment, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue
		}

		if part == "*" {
			segments = append(segments, querySegment{Type: "wildcard"})
		} else if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			// Array access or filter
			inner := part[1 : len(part)-1]
			if inner == "*" {
				segments = append(segments, querySegment{Type: "array", Value: "*"})
			} else if strings.HasPrefix(inner, "?") {
				// Filter condition
				cond, err := parseFilterCondition(inner[1:])
				if err != nil {
					return nil, err
				}
				segments = append(segments, querySegment{Type: "filter", Condition: cond})
			} else {
				// Specific index
				segments = append(segments, querySegment{Type: "array", Value: inner})
			}
		} else {
			segments = append(segments, querySegment{Type: "key", Value: part})
		}
	}

	return segments, nil
}

func parseFilterCondition(expr string) (*filterCondition, error) {
	// Simple parser for conditions like "age>18" or "name==John"
	operators := []string{">=", "<=", "==", "!=", ">", "<"}

	for _, op := range operators {
		if idx := strings.Index(expr, op); idx != -1 {
			field := strings.TrimSpace(expr[:idx])
			valueStr := strings.TrimSpace(expr[idx+len(op):])

			// Try to parse value as number
			var value interface{}
			if num, err := strconv.ParseFloat(valueStr, 64); err == nil {
				value = num
			} else if valueStr == "true" {
				value = true
			} else if valueStr == "false" {
				value = false
			} else {
				// String value
				value = strings.Trim(valueStr, "\"'")
			}

			return &filterCondition{
				Field:    field,
				Operator: op,
				Value:    value,
			}, nil
		}
	}

	return nil, fmt.Errorf("invalid filter condition: %s", expr)
}

func (m *Manager) executeQuery(node *Node, currentPath string, segments []querySegment) ([]QueryResult, error) {
	if len(segments) == 0 {
		return []QueryResult{{Path: currentPath, Node: node.DeepCopy()}}, nil
	}

	segment := segments[0]
	remaining := segments[1:]
	results := make([]QueryResult, 0)

	switch segment.Type {
	case "key":
		if node.Type() != Object {
			return nil, fmt.Errorf("cannot access key '%s' on non-object at %s", segment.Value, currentPath)
		}
		child, err := node.At(segment.Value)
		if err != nil {
			return nil, fmt.Errorf("key '%s' not found at %s", segment.Value, currentPath)
		}
		childPath := currentPath + "/" + segment.Value
		return m.executeQuery(child, childPath, remaining)

	case "wildcard":
		if node.Type() != Object {
			return nil, fmt.Errorf("cannot use wildcard on non-object at %s", currentPath)
		}
		obj, _ := node.GetObject()
		for key, child := range obj {
			childPath := currentPath + "/" + key
			childResults, err := m.executeQuery(child, childPath, remaining)
			if err != nil {
				continue // Skip errors for wildcard
			}
			results = append(results, childResults...)
		}

	case "array":
		if node.Type() != Array {
			return nil, fmt.Errorf("cannot use array access on non-array at %s", currentPath)
		}
		arr, _ := node.GetArray()

		if segment.Value == "*" {
			// All elements
			for i, child := range arr {
				childPath := currentPath + "/" + strconv.Itoa(i)
				childResults, err := m.executeQuery(child, childPath, remaining)
				if err != nil {
					continue
				}
				results = append(results, childResults...)
			}
		} else {
			// Specific index
			idx, err := strconv.Atoi(segment.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid array index: %s", segment.Value)
			}
			if idx < 0 || idx >= len(arr) {
				return nil, fmt.Errorf("array index %d out of bounds at %s", idx, currentPath)
			}
			childPath := currentPath + "/" + strconv.Itoa(idx)
			return m.executeQuery(arr[idx], childPath, remaining)
		}

	case "filter":
		if node.Type() != Array {
			return nil, fmt.Errorf("cannot use filter on non-array at %s", currentPath)
		}
		arr, _ := node.GetArray()

		for i, child := range arr {
			if m.matchesFilter(child, segment.Condition) {
				childPath := currentPath + "/" + strconv.Itoa(i)
				childResults, err := m.executeQuery(child, childPath, remaining)
				if err != nil {
					continue
				}
				results = append(results, childResults...)
			}
		}
	}

	return results, nil
}

func (m *Manager) matchesFilter(node *Node, cond *filterCondition) bool {
	if node.Type() != Object {
		return false
	}

	fieldNode, err := node.At(cond.Field)
	if err != nil {
		return false
	}

	var fieldValue interface{}
	switch fieldNode.Type() {
	case String:
		fieldValue, _ = fieldNode.GetString()
	case Integral, FloatingPoint:
		fieldValue, _ = fieldNode.GetFloat()
	case Boolean:
		fieldValue, _ = fieldNode.GetBool()
	default:
		return false
	}

	return evaluateCondition(fieldValue, cond.Operator, cond.Value)
}

func evaluateCondition(left interface{}, op string, right interface{}) bool {
	switch op {
	case "==":
		return compareEqual(left, right)
	case "!=":
		return !compareEqual(left, right)
	case ">", "<", ">=", "<=":
		return compareNumeric(left, op, right)
	}
	return false
}

func compareEqual(left, right interface{}) bool {
	// Type-safe comparison
	switch l := left.(type) {
	case string:
		r, ok := right.(string)
		return ok && l == r
	case float64:
		r, ok := right.(float64)
		return ok && l == r
	case bool:
		r, ok := right.(bool)
		return ok && l == r
	}
	return false
}

func compareNumeric(left interface{}, op string, right interface{}) bool {
	l, lok := toFloat64(left)
	r, rok := toFloat64(right)
	if !lok || !rok {
		return false
	}

	switch op {
	case ">":
		return l > r
	case "<":
		return l < r
	case ">=":
		return l >= r
	case "<=":
		return l <= r
	}
	return false
}

func toFloat64(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

// QueryExists checks if a query returns any results
func (m *Manager) QueryExists(query string) bool {
	results, err := m.Query(query)
	return err == nil && len(results) > 0
}

// QueryCount returns the number of results for a query
func (m *Manager) QueryCount(query string) (int, error) {
	results, err := m.Query(query)
	if err != nil {
		return 0, err
	}
	return len(results), nil
}
