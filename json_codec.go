package goconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"

	"github.com/iancoleman/orderedmap"
)

func parseConfig(config []byte) (*orderedmap.OrderedMap, error) {
	if len(bytes.TrimSpace(config)) == 0 {
		return nil, errors.New("config is empty")
	}

	decoder := json.NewDecoder(bytes.NewReader(config))
	decoder.UseNumber()

	value, err := decodeJSONValue(decoder)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("failed to parse config: multiple JSON values")
		}
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	root, ok := value.(*orderedmap.OrderedMap)
	if !ok {
		return nil, fmt.Errorf("config root must be an object, got %T", value)
	}
	return root, nil
}

func decodeJSONValue(decoder *json.Decoder) (interface{}, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}

	delimiter, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}

	switch delimiter {
	case '{':
		result := orderedmap.New()
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("object key is %T, want string", keyToken)
			}
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			result.Set(key, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return result, nil

	case '[':
		result := make([]interface{}, 0)
		for decoder.More() {
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func cloneJSONValue(value interface{}) (interface{}, error) {
	switch typed := value.(type) {
	case nil, bool, string, json.Number:
		return typed, nil
	case int:
		return json.Number(strconv.FormatInt(int64(typed), 10)), nil
	case int8:
		return json.Number(strconv.FormatInt(int64(typed), 10)), nil
	case int16:
		return json.Number(strconv.FormatInt(int64(typed), 10)), nil
	case int32:
		return json.Number(strconv.FormatInt(int64(typed), 10)), nil
	case int64:
		return json.Number(strconv.FormatInt(typed, 10)), nil
	case uint:
		return json.Number(strconv.FormatUint(uint64(typed), 10)), nil
	case uint8:
		return json.Number(strconv.FormatUint(uint64(typed), 10)), nil
	case uint16:
		return json.Number(strconv.FormatUint(uint64(typed), 10)), nil
	case uint32:
		return json.Number(strconv.FormatUint(uint64(typed), 10)), nil
	case uint64:
		return json.Number(strconv.FormatUint(typed, 10)), nil
	case float32:
		return cloneJSONFloat(float64(typed))
	case float64:
		return cloneJSONFloat(typed)
	case *orderedmap.OrderedMap:
		if typed == nil {
			return nil, nil
		}
		result := orderedmap.New()
		for _, key := range typed.Keys() {
			child, _ := typed.Get(key)
			cloned, err := cloneJSONValue(child)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			result.Set(key, cloned)
		}
		return result, nil
	case orderedmap.OrderedMap:
		return cloneJSONValue(&typed)
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := orderedmap.New()
		for _, key := range keys {
			cloned, err := cloneJSONValue(typed[key])
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			result.Set(key, cloned)
		}
		return result, nil
	case []interface{}:
		result := make([]interface{}, len(typed))
		for i, child := range typed {
			cloned, err := cloneJSONValue(child)
			if err != nil {
				return nil, fmt.Errorf("index %d: %w", i, err)
			}
			result[i] = cloned
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported JSON value type %T", value)
	}
}

func cloneJSONFloat(value float64) (interface{}, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("non-finite JSON number %v", value)
	}
	return value, nil
}

// Clone creates a deep copy of an OrderedMap while retaining key order and
// exact JSON number representations.
func Clone(source *orderedmap.OrderedMap) (*orderedmap.OrderedMap, error) {
	if source == nil {
		return nil, errors.New("cannot clone nil OrderedMap")
	}

	cloned, err := cloneJSONValue(source)
	if err != nil {
		return nil, err
	}
	return cloned.(*orderedmap.OrderedMap), nil
}
