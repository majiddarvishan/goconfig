package goconfig_test

import (
	"testing"

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
