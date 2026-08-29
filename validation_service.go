package goconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// ValidationService provides external configuration validation.
type ValidationService struct {
	mu      sync.RWMutex
	URL     string
	Timeout time.Duration
	Headers map[string]string
	client  *http.Client
}

// validationService is retained for source compatibility inside the v1 API.
// Deprecated: use ValidationService.
type validationService = ValidationService

// ValidationRequest is sent to the validation service
type ValidationRequest struct {
	Config  interface{} `json:"config"`
	Schema  interface{} `json:"schema"`
	Context string      `json:"context,omitempty"`
}

// ValidationResponse is returned from the validation service
type ValidationResponse struct {
	Valid   bool     `json:"valid"`
	Errors  []string `json:"errors,omitempty"`
	Message string   `json:"message,omitempty"`
}

// NewValidationService creates a new validation service client.
func NewValidationService(url string, timeout time.Duration) *ValidationService {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	return &ValidationService{
		URL:     url,
		Timeout: timeout,
		Headers: make(map[string]string),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// NewvalidationService retains the original misspelled constructor.
// Deprecated: use NewValidationService.
func NewvalidationService(url string, timeout time.Duration) *ValidationService {
	return NewValidationService(url, timeout)
}

// SetHeader sets a custom header for validation requests
func (vs *ValidationService) SetHeader(key, value string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.Headers[key] = value
}

// Validate sends configuration to external validation service
func (vs *ValidationService) Validate(ctx context.Context, config, schema interface{}) error {
	if vs.URL == "" {
		return fmt.Errorf("validation service URL not configured")
	}

	req := ValidationRequest{
		Config: config,
		Schema: schema,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", vs.URL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	vs.mu.RLock()
	headers := make(map[string]string, len(vs.Headers))
	for key, value := range vs.Headers {
		headers[key] = value
	}
	client := vs.client
	vs.mu.RUnlock()

	for key, value := range headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("validation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("validation service returned status %d: %s", resp.StatusCode, string(body))
	}

	var valResp ValidationResponse
	if err := json.NewDecoder(resp.Body).Decode(&valResp); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !valResp.Valid {
		if len(valResp.Errors) > 0 {
			return fmt.Errorf("validation failed: %v", valResp.Errors)
		}
		return fmt.Errorf("validation failed: %s", valResp.Message)
	}

	return nil
}

// Validator checks the complete value at a registered mutation path before and
// after a candidate mutation. For insert and remove operations these values are
// the complete target arrays, not only the changed element.
type Validator func(path string, oldValue, newValue *Node) error

// validatorFunc retains the original internal name.
// Deprecated: use Validator.
type validatorFunc = Validator

// CustomValidator holds custom validation rules.
type CustomValidator struct {
	mu         sync.RWMutex
	validators map[string][]Validator
}

// customValidator retains the original internal name.
// Deprecated: use CustomValidator.
type customValidator = CustomValidator

// NewCustomValidator creates an empty custom validator registry.
func NewCustomValidator() *CustomValidator {
	return &CustomValidator{
		validators: make(map[string][]Validator),
	}
}

// AddValidator adds a validation function for a specific path
func (cv *CustomValidator) AddValidator(path string, validator Validator) {
	if validator == nil {
		return
	}
	cv.mu.Lock()
	defer cv.mu.Unlock()
	cv.validators[path] = append(cv.validators[path], validator)
}

// Validate runs all validators for the given path
func (cv *CustomValidator) Validate(path string, oldValue, newValue *Node) error {
	cv.mu.RLock()
	validators, exists := cv.validators[path]
	validators = append([]Validator(nil), validators...)
	cv.mu.RUnlock()
	if !exists {
		return nil
	}

	for _, validator := range validators {
		if err := validator(path, oldValue, newValue); err != nil {
			return err
		}
	}

	return nil
}

// ValidateAll runs validators for all registered paths
func (cv *CustomValidator) ValidateAll(changes map[string]*Node) error {
	for path, newValue := range changes {
		if err := cv.Validate(path, nil, newValue); err != nil {
			return fmt.Errorf("validation failed at %s: %w", path, err)
		}
	}
	return nil
}

// Common validator functions

// ValidateRange validates that a numeric value is within a range
func ValidateRange(min, max float64) Validator {
	return func(path string, oldValue, newValue *Node) error {
		if newValue == nil {
			return nil
		}

		val, err := newValue.GetFloat()
		if err != nil {
			return fmt.Errorf("expected numeric value at %s", path)
		}

		if val < min || val > max {
			return fmt.Errorf("value %.2f at %s must be between %.2f and %.2f", val, path, min, max)
		}

		return nil
	}
}

// ValidatePattern validates that a string matches a regular expression. The
// legacy pattern "*" remains supported as a match-all expression.
//
// Deprecated: use ValidateRegexp when construction errors can be handled.
func ValidatePattern(pattern string) Validator {
	if pattern == "*" {
		pattern = ".*"
	}
	validator, compileErr := ValidateRegexp(pattern)
	return func(path string, oldValue, newValue *Node) error {
		if compileErr != nil {
			return fmt.Errorf("invalid validation pattern %q: %w", pattern, compileErr)
		}
		return validator(path, oldValue, newValue)
	}
}

// ValidateRegexp compiles a regular expression validator.
func ValidateRegexp(pattern string) (Validator, error) {
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return func(path string, oldValue, newValue *Node) error {
		if newValue == nil {
			return nil
		}
		str, err := newValue.GetString()
		if err != nil {
			return fmt.Errorf("expected string value at %s", path)
		}
		if !compiled.MatchString(str) {
			return fmt.Errorf("value '%s' at %s does not match pattern '%s'", str, path, pattern)
		}
		return nil
	}, nil
}

// ValidateEnum validates that a value is one of allowed values
func ValidateEnum(allowed ...interface{}) Validator {
	return func(path string, oldValue, newValue *Node) error {
		if newValue == nil {
			return nil
		}

		value := newValue.value
		switch newValue.Type() {
		case String, Integral, FloatingPoint, Boolean:
		default:
			return fmt.Errorf("unsupported type for enum validation at %s", path)
		}

		for _, a := range allowed {
			if enumValuesEqual(value, a) {
				return nil
			}
		}

		return fmt.Errorf("value %v at %s is not in allowed values: %v", value, path, allowed)
	}
}

// ValidateRequired validates that a value is not null/empty
func ValidateRequired() Validator {
	return func(path string, oldValue, newValue *Node) error {
		if newValue == nil || newValue.Type() == Null {
			return fmt.Errorf("value at %s is required", path)
		}

		if newValue.Type() == String {
			str, _ := newValue.GetString()
			if str == "" {
				return fmt.Errorf("value at %s cannot be empty", path)
			}
		}

		return nil
	}
}

// ValidateUnique validates that array elements are unique (for specific field)
func ValidateUnique(field string) Validator {
	return func(path string, oldValue, newValue *Node) error {
		if newValue == nil || newValue.Type() != Array {
			return nil
		}

		arr, _ := newValue.GetArray()
		seen := make(map[string]bool)

		for i, item := range arr {
			if item.Type() != Object {
				continue
			}

			fieldNode, err := item.At(field)
			if err != nil {
				continue
			}

			key, ok := enumKey(fieldNode.value)
			if !ok {
				continue
			}

			if seen[key] {
				return fmt.Errorf("duplicate value '%v' for field '%s' at %s[%d]", fieldNode.value, field, path, i)
			}
			seen[key] = true
		}

		return nil
	}
}

func enumValuesEqual(left, right interface{}) bool {
	leftNumber, leftNumeric := normalizedNumber(left)
	rightNumber, rightNumeric := normalizedNumber(right)
	if leftNumeric || rightNumeric {
		return leftNumeric && rightNumeric && leftNumber == rightNumber
	}
	return left == right
}

func enumKey(value interface{}) (string, bool) {
	if number, ok := normalizedNumber(value); ok {
		return "number:" + number, true
	}
	switch typed := value.(type) {
	case string:
		return "string:" + typed, true
	case bool:
		return "bool:" + strconv.FormatBool(typed), true
	default:
		return "", false
	}
}

func normalizedNumber(value interface{}) (string, bool) {
	var text string
	switch typed := value.(type) {
	case json.Number:
		text = typed.String()
	case int:
		text = strconv.FormatInt(int64(typed), 10)
	case int8:
		text = strconv.FormatInt(int64(typed), 10)
	case int16:
		text = strconv.FormatInt(int64(typed), 10)
	case int32:
		text = strconv.FormatInt(int64(typed), 10)
	case int64:
		text = strconv.FormatInt(typed, 10)
	case uint:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint8:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint16:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint32:
		text = strconv.FormatUint(uint64(typed), 10)
	case uint64:
		text = strconv.FormatUint(typed, 10)
	case float32:
		text = strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case float64:
		text = strconv.FormatFloat(typed, 'g', -1, 64)
	default:
		return "", false
	}
	number, ok := new(big.Rat).SetString(text)
	if !ok {
		return "", false
	}
	return number.RatString(), true
}
