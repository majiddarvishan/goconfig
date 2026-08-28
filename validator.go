package goconfig

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

func validate(conf, schema *string) error {
	if conf == nil {
		return errors.New("config cannot be nil")
	}
	if schema == nil {
		return errors.New("schema cannot be nil")
	}

	compiled, err := compileSchema(schema)
	if err != nil {
		return err
	}
	return validateWithSchema(compiled, []byte(*conf))
}

func compileSchema(schema *string) (*gojsonschema.Schema, error) {
	if schema == nil {
		return nil, errors.New("schema cannot be nil")
	}
	if strings.TrimSpace(*schema) == "" {
		return nil, errors.New("schema cannot be empty")
	}

	compiled, err := gojsonschema.NewSchema(gojsonschema.NewBytesLoader([]byte(*schema)))
	if err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}
	return compiled, nil
}

func validateWithSchema(schema *gojsonschema.Schema, document []byte) error {
	if schema == nil {
		return errors.New("compiled schema cannot be nil")
	}
	result, err := schema.Validate(gojsonschema.NewBytesLoader(document))
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	if !result.Valid() {
		var sb strings.Builder
		sb.WriteString("validation failed:")
		for i, desc := range result.Errors() {
			sb.WriteString("\n  ")
			sb.WriteString(fmt.Sprintf("[%d] %s", i+1, desc.String()))
		}
		return errors.New(sb.String())
	}

	return nil
}
