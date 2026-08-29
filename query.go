package goconfig

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// QueryResult is an independent node snapshot and its canonical JSON Pointer.
type QueryResult struct {
	Path string
	Node *Node
}

// Query searches a snapshot using JSON Pointer segments plus the compatible
// extensions *, [*], [index], and [?condition]. The legacy query "/" still
// selects the root; returned root paths are the canonical empty pointer.
func (m *Manager) Query(query string) ([]QueryResult, error) {
	m.mu.RLock()
	root := m.config.DeepCopy()
	m.mu.RUnlock()

	if query == "" || query == "/" {
		return []QueryResult{{Path: "", Node: root}}, nil
	}
	segments, err := parseQuerySegments(query)
	if err != nil {
		return nil, err
	}
	return executeQuery(query, root, nil, segments)
}

// Lookup resolves an exact RFC 6901 JSON Pointer without interpreting query
// extension tokens. It is the unambiguous API for keys such as "*" or "[*]".
func (m *Manager) Lookup(pointer string) (*QueryResult, error) {
	segments, err := parseJSONPointer(pointer)
	if err != nil {
		return nil, &QueryError{Query: pointer, Err: ErrInvalidQuery, Msg: err.Error()}
	}
	m.mu.RLock()
	root := m.config.DeepCopy()
	m.mu.RUnlock()

	node, err := lookupQueryNode(pointer, root, segments)
	if err != nil {
		return nil, err
	}
	return &QueryResult{Path: buildJSONPointer(segments), Node: node.DeepCopy()}, nil
}

// QueryOne returns the first deterministic result.
func (m *Manager) QueryOne(query string) (*QueryResult, error) {
	results, err := m.Query(query)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, &QueryError{Query: query, Err: ErrQueryNoResults, Msg: "no results"}
	}
	result := results[0]
	result.Node = result.Node.DeepCopy()
	return &result, nil
}

// FindAll evaluates predicate over an independent snapshot without holding a
// Manager lock. Object traversal and returned paths are deterministic.
func (m *Manager) FindAll(predicate func(*Node) bool) []QueryResult {
	if predicate == nil {
		return []QueryResult{}
	}
	m.mu.RLock()
	root := m.config.DeepCopy()
	m.mu.RUnlock()

	results := make([]QueryResult, 0)
	findAllRecursive(root, nil, predicate, &results)
	return results
}

func findAllRecursive(node *Node, path []string, predicate func(*Node) bool, results *[]QueryResult) {
	if predicate(node) {
		*results = append(*results, QueryResult{Path: buildJSONPointer(path), Node: node.DeepCopy()})
	}

	switch node.Type() {
	case Object:
		object, _ := node.GetObject()
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			findAllRecursive(object[key], appendPathSegment(path, key), predicate, results)
		}
	case Array:
		array, _ := node.GetArray()
		for index, child := range array {
			findAllRecursive(child, appendPathSegment(path, strconv.Itoa(index)), predicate, results)
		}
	}
}

type querySegmentType int

const (
	queryLiteral querySegmentType = iota
	queryWildcard
	queryArrayWildcard
	queryLegacyIndex
	queryFilter
)

type querySegment struct {
	typeOf    querySegmentType
	value     string
	condition *filterCondition
}

type filterCondition struct {
	field    string
	operator string
	value    interface{}
}

func parseQuerySegments(query string) ([]querySegment, error) {
	parts, err := parseJSONPointer(query)
	if err != nil {
		return nil, &QueryError{Query: query, Err: ErrInvalidQuery, Msg: err.Error()}
	}
	segments := make([]querySegment, 0, len(parts))
	for _, part := range parts {
		switch {
		case part == "*":
			segments = append(segments, querySegment{typeOf: queryWildcard})
		case part == "[*]":
			segments = append(segments, querySegment{typeOf: queryArrayWildcard})
		case strings.HasPrefix(part, "[?") && strings.HasSuffix(part, "]"):
			condition, conditionErr := parseFilterCondition(part[2 : len(part)-1])
			if conditionErr != nil {
				return nil, &QueryError{Query: query, Err: ErrInvalidQuery, Msg: conditionErr.Error()}
			}
			segments = append(segments, querySegment{typeOf: queryFilter, condition: condition})
		case strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]"):
			index := part[1 : len(part)-1]
			if _, indexErr := parseCanonicalArrayIndex(index); indexErr != nil {
				return nil, &QueryError{Query: query, Err: ErrInvalidQuery, Msg: indexErr.Error()}
			}
			segments = append(segments, querySegment{typeOf: queryLegacyIndex, value: index})
		default:
			segments = append(segments, querySegment{typeOf: queryLiteral, value: part})
		}
	}
	return segments, nil
}

func parseFilterCondition(expression string) (*filterCondition, error) {
	operators := []string{">=", "<=", "==", "!=", ">", "<"}
	for _, operator := range operators {
		position := strings.Index(expression, operator)
		if position < 0 {
			continue
		}
		field := strings.TrimSpace(expression[:position])
		rawValue := strings.TrimSpace(expression[position+len(operator):])
		if field == "" || rawValue == "" {
			return nil, fmt.Errorf("filter field and value are required")
		}
		value, err := parseFilterValue(rawValue)
		if err != nil {
			return nil, err
		}
		if operator != "==" && operator != "!=" {
			if _, numeric := normalizedNumber(value); !numeric {
				return nil, fmt.Errorf("operator %q requires a numeric value", operator)
			}
		}
		return &filterCondition{field: field, operator: operator, value: value}, nil
	}
	return nil, fmt.Errorf("invalid filter condition %q", expression)
}

func parseFilterValue(value string) (interface{}, error) {
	if value == "true" {
		return true, nil
	}
	if value == "false" {
		return false, nil
	}
	if strings.HasPrefix(value, `"`) {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return nil, fmt.Errorf("invalid quoted filter value: %w", err)
		}
		return decoded, nil
	}
	if strings.HasPrefix(value, "'") {
		if len(value) < 2 || !strings.HasSuffix(value, "'") {
			return nil, fmt.Errorf("invalid quoted filter value %q", value)
		}
		return value[1 : len(value)-1], nil
	}
	if json.Valid([]byte(value)) {
		decoder := json.NewDecoder(strings.NewReader(value))
		decoder.UseNumber()
		var decoded interface{}
		if err := decoder.Decode(&decoded); err == nil {
			if number, numeric := decoded.(json.Number); numeric {
				return number, nil
			}
		}
	}
	return value, nil
}

func executeQuery(query string, node *Node, path []string, segments []querySegment) ([]QueryResult, error) {
	if len(segments) == 0 {
		return []QueryResult{{Path: buildJSONPointer(path), Node: node.DeepCopy()}}, nil
	}
	segment := segments[0]
	remaining := segments[1:]

	switch segment.typeOf {
	case queryLiteral:
		return executeLiteralSegment(query, node, path, segment.value, remaining)
	case queryLegacyIndex:
		return executeArrayIndex(query, node, path, segment.value, remaining)
	case queryWildcard:
		return executeWildcard(query, node, path, remaining, false)
	case queryArrayWildcard:
		return executeWildcard(query, node, path, remaining, true)
	case queryFilter:
		if node.Type() != Array {
			return nil, queryTraversalError(query, path, ErrTypeMismatch, "filter target is not an array")
		}
		array, _ := node.GetArray()
		results := make([]QueryResult, 0)
		for index, child := range array {
			if !matchesFilter(child, segment.condition) {
				continue
			}
			childPath := appendPathSegment(path, strconv.Itoa(index))
			childResults, err := executeQuery(query, child, childPath, remaining)
			if err != nil {
				return nil, err
			}
			results = append(results, childResults...)
		}
		return results, nil
	default:
		return nil, queryTraversalError(query, path, ErrInvalidQuery, "unsupported query segment")
	}
}

func executeLiteralSegment(query string, node *Node, path []string, value string, remaining []querySegment) ([]QueryResult, error) {
	switch node.Type() {
	case Object:
		child, err := node.At(value)
		if err != nil {
			return nil, queryTraversalError(query, path, ErrPathNotFound, fmt.Sprintf("key %q not found", value))
		}
		childPath := appendPathSegment(path, value)
		return executeQuery(query, child, childPath, remaining)
	case Array:
		return executeArrayIndex(query, node, path, value, remaining)
	default:
		return nil, queryTraversalError(query, path, ErrTypeMismatch, fmt.Sprintf("cannot traverse segment %q", value))
	}
}

func executeArrayIndex(query string, node *Node, path []string, value string, remaining []querySegment) ([]QueryResult, error) {
	if node.Type() != Array {
		return nil, queryTraversalError(query, path, ErrTypeMismatch, "array index target is not an array")
	}
	index, err := parseCanonicalArrayIndex(value)
	if err != nil {
		return nil, queryTraversalError(query, path, ErrInvalidQuery, err.Error())
	}
	array, _ := node.GetArray()
	if index >= len(array) {
		return nil, queryTraversalError(query, path, ErrIndexOutOfRange, fmt.Sprintf("array index %d out of bounds", index))
	}
	childPath := appendPathSegment(path, strconv.Itoa(index))
	return executeQuery(query, array[index], childPath, remaining)
}

func executeWildcard(query string, node *Node, path []string, remaining []querySegment, arrayOnly bool) ([]QueryResult, error) {
	results := make([]QueryResult, 0)
	if node.Type() == Array {
		array, _ := node.GetArray()
		for index, child := range array {
			childPath := appendPathSegment(path, strconv.Itoa(index))
			childResults, err := executeQuery(query, child, childPath, remaining)
			if err != nil {
				return nil, err
			}
			results = append(results, childResults...)
		}
		return results, nil
	}
	if arrayOnly || node.Type() != Object {
		return nil, queryTraversalError(query, path, ErrTypeMismatch, "wildcard target has incompatible type")
	}
	object, _ := node.GetObject()
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		childPath := appendPathSegment(path, key)
		childResults, err := executeQuery(query, object[key], childPath, remaining)
		if err != nil {
			return nil, err
		}
		results = append(results, childResults...)
	}
	return results, nil
}

func lookupQueryNode(query string, root *Node, segments []string) (*Node, error) {
	current := root
	path := make([]string, 0, len(segments))
	for _, segment := range segments {
		switch current.Type() {
		case Object:
			next, err := current.At(segment)
			if err != nil {
				return nil, queryTraversalError(query, path, ErrPathNotFound, fmt.Sprintf("key %q not found", segment))
			}
			current = next
		case Array:
			index, err := parseCanonicalArrayIndex(segment)
			if err != nil {
				return nil, queryTraversalError(query, path, ErrInvalidQuery, err.Error())
			}
			array, _ := current.GetArray()
			if index >= len(array) {
				return nil, queryTraversalError(query, path, ErrIndexOutOfRange, fmt.Sprintf("array index %d out of bounds", index))
			}
			current = array[index]
		default:
			return nil, queryTraversalError(query, path, ErrTypeMismatch, fmt.Sprintf("cannot traverse segment %q", segment))
		}
		path = appendPathSegment(path, segment)
	}
	return current, nil
}

func queryTraversalError(query string, path []string, err error, message string) error {
	return &QueryError{Query: query, Path: buildJSONPointer(path), Err: err, Msg: message}
}

func matchesFilter(node *Node, condition *filterCondition) bool {
	if node.Type() != Object {
		return false
	}
	fieldNode, err := node.At(condition.field)
	if err != nil {
		return false
	}
	var fieldValue interface{}
	switch fieldNode.Type() {
	case String, Integral, FloatingPoint, Boolean:
		fieldValue = fieldNode.value
	default:
		return false
	}
	return evaluateCondition(fieldValue, condition.operator, condition.value)
}

func evaluateCondition(left interface{}, operator string, right interface{}) bool {
	switch operator {
	case "==":
		return enumValuesEqual(left, right)
	case "!=":
		return !enumValuesEqual(left, right)
	case ">", "<", ">=", "<=":
		return compareNumeric(left, operator, right)
	default:
		return false
	}
}

func compareNumeric(left interface{}, operator string, right interface{}) bool {
	leftText, leftNumeric := normalizedNumber(left)
	rightText, rightNumeric := normalizedNumber(right)
	if !leftNumeric || !rightNumeric {
		return false
	}
	leftNumber, leftOK := new(big.Rat).SetString(leftText)
	rightNumber, rightOK := new(big.Rat).SetString(rightText)
	if !leftOK || !rightOK {
		return false
	}
	comparison := leftNumber.Cmp(rightNumber)
	switch operator {
	case ">":
		return comparison > 0
	case "<":
		return comparison < 0
	case ">=":
		return comparison >= 0
	case "<=":
		return comparison <= 0
	default:
		return false
	}
}

// QueryExists reports whether a query succeeds with at least one result.
func (m *Manager) QueryExists(query string) bool {
	results, err := m.Query(query)
	return err == nil && len(results) > 0
}

// QueryCount returns the number of deterministic query results.
func (m *Manager) QueryCount(query string) (int, error) {
	results, err := m.Query(query)
	if err != nil {
		return 0, err
	}
	return len(results), nil
}
