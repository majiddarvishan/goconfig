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

	loadedSchema := gojsonschema.NewBytesLoader([]byte(*schema))
	documentLoader := gojsonschema.NewBytesLoader([]byte(*conf))

	result, err := gojsonschema.Validate(loadedSchema, documentLoader)
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
