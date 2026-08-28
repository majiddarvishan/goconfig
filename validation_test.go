package goconfig

import (
	"context"
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

	if err := validate(&valid, &schema); err != nil {
		t.Fatalf("validate(valid) error = %v", err)
	}
	if err := validate(&invalid, &schema); err == nil {
		t.Fatal("validate(invalid) error = nil, want error")
	}
	if err := validate(nil, &schema); err == nil {
		t.Fatal("validate(nil config) error = nil, want error")
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

	array := mustParseNode(t, []interface{}{
		map[string]interface{}{"id": "same"},
		map[string]interface{}{"id": "same"},
	})
	if err := ValidateUnique("id")("/items", nil, array); err == nil {
		t.Fatal("ValidateUnique(duplicate) error = nil, want error")
	}
}

func TestValidationServiceRequest(t *testing.T) {
	service := NewvalidationService("http://validator.test/validate", time.Second)
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
