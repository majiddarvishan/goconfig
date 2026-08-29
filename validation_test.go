package goconfig

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestValidateAgainstJSONSchema(t *testing.T) {
	valid := testConfigJSON
	invalid := `{"name":"incomplete"}`
	schema := testSchemaJSON

	compiled, err := compileSchema(&schema)
	if err != nil {
		t.Fatalf("compileSchema() error = %v", err)
	}
	if err := validateWithSchema(compiled, []byte(valid)); err != nil {
		t.Fatalf("validate(valid) error = %v", err)
	}
	if err := validateWithSchema(compiled, []byte(invalid)); err == nil {
		t.Fatal("validate(invalid) error = nil, want error")
	}
	if _, err := compileSchema(nil); err == nil {
		t.Fatal("compileSchema(nil) error = nil, want error")
	}
}

func TestCommonValidators(t *testing.T) {
	if err := ValidateRange(1, 10)("/value", nil, mustParseNode(t, float64(5))); err != nil {
		t.Fatalf("ValidateRange(valid) error = %v", err)
	}
	if err := ValidateRange(1, 10)("/value", nil, mustParseNode(t, float64(11))); err == nil {
		t.Fatal("ValidateRange(invalid) error = nil, want error")
	}
	if err := ValidateRequired()("/value", nil, mustParseNode(t, "")); err == nil {
		t.Fatal("ValidateRequired(empty) error = nil, want error")
	}
	if err := ValidateEnum("a", "b")("/value", nil, mustParseNode(t, "b")); err != nil {
		t.Fatalf("ValidateEnum(valid) error = %v", err)
	}
	if err := ValidateEnum(1)("/value", nil, mustParseNode(t, json.Number("1.0"))); err != nil {
		t.Fatalf("ValidateEnum(normalized number) error = %v", err)
	}
	if err := ValidateEnum(json.Number("9007199254740993"))("/value", nil, mustParseNode(t, json.Number("9007199254740992"))); err == nil {
		t.Fatal("ValidateEnum(distinct large integers) error = nil, want error")
	}
	if err := ValidatePattern(`^[a-z]+-[0-9]+$`)("/value", nil, mustParseNode(t, "item-42")); err != nil {
		t.Fatalf("ValidatePattern(valid regexp) error = %v", err)
	}
	if err := ValidatePattern(`^[a-z]+$`)("/value", nil, mustParseNode(t, "42")); err == nil {
		t.Fatal("ValidatePattern(non-match) error = nil, want error")
	}
	if err := ValidatePattern(`[`)("/value", nil, mustParseNode(t, "value")); err == nil {
		t.Fatal("ValidatePattern(invalid regexp) error = nil, want error")
	}
	if err := ValidatePattern(`*`)("/value", nil, mustParseNode(t, "legacy")); err != nil {
		t.Fatalf("ValidatePattern(legacy wildcard) error = %v", err)
	}
	if _, err := ValidateRegexp(`[`); err == nil {
		t.Fatal("ValidateRegexp(invalid regexp) error = nil, want construction error")
	}

	array := mustParseNode(t, []interface{}{
		map[string]interface{}{"id": "same"},
		map[string]interface{}{"id": "same"},
	})
	if err := ValidateUnique("id")("/items", nil, array); err == nil {
		t.Fatal("ValidateUnique(duplicate) error = nil, want error")
	}
}

func TestValidationServiceRequest(t *testing.T) {
	service := NewValidationService("http://validator.test/validate", time.Second)
	service.SetHeader("X-Test-Token", "token")
	service.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("X-Test-Token"); got != "token" {
			t.Errorf("X-Test-Token = %q, want token", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"valid":true}`)),
			Request:    r,
		}, nil
	})}

	if err := service.Validate(context.Background(), map[string]interface{}{"ok": true}, map[string]interface{}{}); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}
