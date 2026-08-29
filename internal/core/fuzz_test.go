package goconfig

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/iancoleman/orderedmap"
)

func FuzzJSONPointerRoundTrip(f *testing.F) {
	for _, pointer := range []string{"", "/", "/items/0", "/a~1b/m~0n", "/a//b", "/~2"} {
		f.Add(pointer)
	}
	f.Fuzz(func(t *testing.T, pointer string) {
		segments, err := parseJSONPointer(pointer)
		if err != nil {
			return
		}
		rebuilt := buildJSONPointer(segments)
		roundTrip, err := parseJSONPointer(rebuilt)
		if err != nil {
			t.Fatalf("parse rebuilt pointer %q: %v", rebuilt, err)
		}
		if !reflect.DeepEqual(roundTrip, segments) {
			t.Fatalf("round trip = %#v, want %#v", roundTrip, segments)
		}
	})
}

func FuzzQueryParsingAndTraversal(f *testing.F) {
	manager := fuzzManager(f)
	for _, query := range []string{"", "/", "/items/0/name", "/items/[*]/name", "/items/[?id>=1]/name", "/a~1b", "invalid"} {
		f.Add(query)
	}
	f.Fuzz(func(t *testing.T, query string) {
		_, _ = manager.Query(query)
		_, _ = manager.Lookup(query)
	})
}

func FuzzHTTPMutationRequest(f *testing.F) {
	manager := fuzzManager(f)
	server, err := newHttpServer(manager, WithMaxBodySize(4096), WithLogger(nil))
	if err != nil {
		f.Fatalf("newHttpServer() error = %v", err)
	}
	for _, body := range []string{
		`{"op":"replace","path":"/name","value":"new","version":1}`,
		`{"op":"insert","path":"/items","index":0,"value":null}`,
		`{}`,
		`[1,2,3]`,
		`{`,
	} {
		f.Add(body)
	}
	f.Fuzz(func(t *testing.T, body string) {
		request := newFuzzRequest([]byte(body))
		payload, decodeErr := server.decodeMutationRequest(request)
		if decodeErr == nil {
			_, _ = payload.mutationRequest()
		}
	})
}

func FuzzApplyMutation(f *testing.F) {
	for _, seed := range []struct {
		op    int
		path  string
		index int
		value string
	}{
		{0, "/items", 0, `3`},
		{1, "/items", 0, `null`},
		{2, "/name", 0, `"updated"`},
		{2, "", 0, `{"name":"root","items":[]}`},
	} {
		f.Add(seed.op, seed.path, seed.index, seed.value)
	}
	f.Fuzz(func(t *testing.T, operation int, path string, index int, rawValue string) {
		root := orderedmap.New()
		root.Set("name", "base")
		root.Set("items", []interface{}{json.Number("1"), json.Number("2")})

		decoder := json.NewDecoder(bytes.NewBufferString(rawValue))
		decoder.UseNumber()
		var value interface{}
		if err := decoder.Decode(&value); err != nil {
			return
		}
		kind := mutationKind(operationName(operation))
		if _, err := applyMutation(root, kind, path, index, value); err != nil {
			return
		}
		encoded, err := json.Marshal(root)
		if err != nil {
			t.Fatalf("successful mutation produced unmarshalable state: %v", err)
		}
		if _, err := parseConfig(encoded); err != nil {
			t.Fatalf("successful mutation produced invalid object JSON: %v", err)
		}
	})
}

func operationName(value int) string {
	switch ((value % 3) + 3) % 3 {
	case 0:
		return string(mutationInsert)
	case 1:
		return string(mutationRemove)
	default:
		return string(mutationReplace)
	}
}
