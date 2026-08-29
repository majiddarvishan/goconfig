package goconfig_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/majiddarvishan/goconfig"
)

func TestExternalPackageCanImplementSource(t *testing.T) {
	source := &externalSource{
		config: []byte(`{"name":"external"}`),
		schema: []byte(`{
          "type":"object",
          "required":["name"],
          "properties":{"name":{"type":"string"}},
          "additionalProperties":false
        }`),
	}

	manager, err := goconfig.NewManagerFromSource(source)
	if err != nil {
		t.Fatalf("NewManagerFromSource() error = %v", err)
	}
	if got, err := manager.Config().GetString("name"); err != nil || got != "external" {
		t.Fatalf("name = %q, %v", got, err)
	}
}

func TestExternalPackageCanUsePhaseFiveAPIs(t *testing.T) {
	source := &externalSource{
		config: []byte(`{"name":"external"}`),
		schema: []byte(`{"type":"object","properties":{"name":{"type":"string"}}}`),
	}
	manager, err := goconfig.NewManagerFromSourceWithOptions(source, goconfig.WithHistoryCapacity(2))
	if err != nil {
		t.Fatalf("NewManagerFromSourceWithOptions() error = %v", err)
	}
	var validator goconfig.Validator = goconfig.ValidateRequired()
	manager.AddValidator("/name", validator)
	registry := goconfig.NewCustomValidator()
	registry.AddValidator("/name", validator)
	service := goconfig.NewValidationService("http://validator.test", time.Second)
	service.SetHeader("X-Test", "value")
	manager.SetValidationService(service)
}

func TestExternalPackageCanUseHTTPHandler(t *testing.T) {
	source := &externalSource{
		config: []byte(`{"name":"external"}`),
		schema: []byte(`{"type":"object","properties":{"name":{"type":"string"}}}`),
	}
	manager, err := goconfig.NewManagerFromSource(source)
	if err != nil {
		t.Fatalf("NewManagerFromSource() error = %v", err)
	}
	server, err := goconfig.NewHTTPServer(manager, goconfig.WithHealthEnabled(false))
	if err != nil {
		t.Fatalf("NewHTTPServer() error = %v", err)
	}
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

type externalSource struct {
	config []byte
	schema []byte
}

func (s *externalSource) Read() (goconfig.SourceData, error) {
	return goconfig.SourceData{
		Config: append([]byte(nil), s.config...),
		Schema: append([]byte(nil), s.schema...),
	}, nil
}

func (s *externalSource) Write(config []byte) error {
	s.config = append([]byte(nil), config...)
	return nil
}
