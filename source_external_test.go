package goconfig_test

import (
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
