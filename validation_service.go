package goconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// validationService provides external configuration validation
type validationService struct {
	URL     string
	Timeout time.Duration
	Headers map[string]string
	client  *http.Client
}

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

// NewvalidationService creates a new validation service client
func NewvalidationService(url string, timeout time.Duration) *validationService {
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &validationService{
		URL:     url,
		Timeout: timeout,
		Headers: make(map[string]string),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// SetHeader sets a custom header for validation requests
func (vs *validationService) SetHeader(key, value string) {
	vs.Headers[key] = value
}

// Validate sends configuration to external validation service
func (vs *validationService) Validate(ctx context.Context, config, schema interface{}) error {
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
	for key, value := range vs.Headers {
		httpReq.Header.Set(key, value)
	}

	resp, err := vs.client.Do(httpReq)
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

// validatorFunc is a custom validation function
type validatorFunc func(path string, oldValue, newValue *Node) error

// customValidator holds custom validation rules
type customValidator struct {
	validators map[string][]validatorFunc
}

// NewcustomValidator creates a new custom validator
func NewCustomValidator() *customValidator {
	return &customValidator{
		validators: make(map[string][]validatorFunc),
	}
}

// AddValidator adds a validation function for a specific path
func (cv *customValidator) AddValidator(path string, validator validatorFunc) {
	cv.validators[path] = append(cv.validators[path], validator)
}

// Validate runs all validators for the given path
func (cv *customValidator) Validate(path string, oldValue, newValue *Node) error {
	validators, exists := cv.validators[path]
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
func (cv *customValidator) ValidateAll(changes map[string]*Node) error {
	for path, newValue := range changes {
		if err := cv.Validate(path, nil, newValue); err != nil {
			return fmt.Errorf("validation failed at %s: %w", path, err)
		}
	}
	return nil
}

// Common validator functions

// ValidateRange validates that a numeric value is within a range
func ValidateRange(min, max float64) validatorFunc {
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

// ValidatePattern validates that a string matches a pattern
func ValidatePattern(pattern string) validatorFunc {
	return func(path string, oldValue, newValue *Node) error {
		if newValue == nil {
			return nil
		}

		str, err := newValue.GetString()
		if err != nil {
			return fmt.Errorf("expected string value at %s", path)
		}

		// Simple pattern matching (could use regexp for more complex patterns)
		if !matchPattern(str, pattern) {
			return fmt.Errorf("value '%s' at %s does not match pattern '%s'", str, path, pattern)
		}

		return nil
	}
}

// ValidateEnum validates that a value is one of allowed values
func ValidateEnum(allowed ...interface{}) validatorFunc {
	return func(path string, oldValue, newValue *Node) error {
		if newValue == nil {
			return nil
		}

		var value interface{}
		switch newValue.Type() {
		case String:
			value, _ = newValue.GetString()
		case Integral, FloatingPoint:
			value, _ = newValue.GetFloat()
		case Boolean:
			value, _ = newValue.GetBool()
		default:
			return fmt.Errorf("unsupported type for enum validation at %s", path)
		}

		for _, a := range allowed {
			if value == a {
				return nil
			}
		}

		return fmt.Errorf("value %v at %s is not in allowed values: %v", value, path, allowed)
	}
}

// ValidateRequired validates that a value is not null/empty
func ValidateRequired() validatorFunc {
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
func ValidateUnique(field string) validatorFunc {
	return func(path string, oldValue, newValue *Node) error {
		if newValue == nil || newValue.Type() != Array {
			return nil
		}

		arr, _ := newValue.GetArray()
		seen := make(map[interface{}]bool)

		for i, item := range arr {
			if item.Type() != Object {
				continue
			}

			fieldNode, err := item.At(field)
			if err != nil {
				continue
			}

			var value interface{}
			switch fieldNode.Type() {
			case String:
				value, _ = fieldNode.GetString()
			case Integral, FloatingPoint:
				value, _ = fieldNode.GetFloat()
			case Boolean:
				value, _ = fieldNode.GetBool()
			default:
				continue
			}

			if seen[value] {
				return fmt.Errorf("duplicate value '%v' for field '%s' at %s[%d]", value, field, path, i)
			}
			seen[value] = true
		}

		return nil
	}
}

func matchPattern(str, pattern string) bool {
	// Simple wildcard matching (* matches any characters)
	// For more complex patterns, use regexp
	if pattern == "*" {
		return true
	}
	return str == pattern
}
