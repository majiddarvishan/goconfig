package goconfig

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/iancoleman/orderedmap"
)

func BenchmarkClone(b *testing.B) {
	source := benchmarkConfig(250)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		clone, err := Clone(source)
		if err != nil || clone == nil {
			b.Fatalf("Clone() = %v, %v", clone, err)
		}
	}
}

func BenchmarkCompiledSchemaValidation(b *testing.B) {
	schema := `{"type":"object","required":["items"],"properties":{"items":{"type":"array"}}}`
	compiled, err := compileSchema(&schema)
	if err != nil {
		b.Fatalf("compileSchema() error = %v", err)
	}
	document, err := json.Marshal(benchmarkConfig(250))
	if err != nil {
		b.Fatalf("marshal benchmark config: %v", err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(document)))
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if err := validateWithSchema(compiled, document); err != nil {
			b.Fatalf("validateWithSchema() error = %v", err)
		}
	}
}

func BenchmarkNodeAtJSONPointer(b *testing.B) {
	root, err := parseNodeStrict(benchmarkConfig(250))
	if err != nil {
		b.Fatalf("parseNodeStrict() error = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		node, err := nodeAtJSONPointer(root, "/items/200/name")
		if err != nil || node == nil {
			b.Fatalf("nodeAtJSONPointer() = %v, %v", node, err)
		}
	}
}

func benchmarkConfig(size int) *orderedmap.OrderedMap {
	config := orderedmap.New()
	items := make([]interface{}, size)
	for index := range items {
		item := orderedmap.New()
		item.Set("id", json.Number(fmt.Sprintf("%d", index)))
		item.Set("name", fmt.Sprintf("item-%d", index))
		items[index] = item
	}
	config.Set("items", items)
	return config
}
