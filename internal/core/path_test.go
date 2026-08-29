package goconfig

import (
	"reflect"
	"testing"
)

func TestJSONPointerRoundTrip(t *testing.T) {
	tests := []struct {
		pointer  string
		segments []string
	}{
		{pointer: "", segments: []string{}},
		{pointer: "/name", segments: []string{"name"}},
		{pointer: "/a~1b", segments: []string{"a/b"}},
		{pointer: "/m~0n", segments: []string{"m~n"}},
		{pointer: "/", segments: []string{""}},
		{pointer: "/a//b", segments: []string{"a", "", "b"}},
		{pointer: "/items/0/name", segments: []string{"items", "0", "name"}},
	}

	for _, tt := range tests {
		t.Run(tt.pointer, func(t *testing.T) {
			segments, err := parseJSONPointer(tt.pointer)
			if err != nil {
				t.Fatalf("parseJSONPointer() error = %v", err)
			}
			if !reflect.DeepEqual(segments, tt.segments) {
				t.Fatalf("segments = %#v, want %#v", segments, tt.segments)
			}
			if got := buildJSONPointer(segments); got != tt.pointer {
				t.Fatalf("buildJSONPointer() = %q, want %q", got, tt.pointer)
			}
		})
	}
}

func TestJSONPointerRejectsInvalidSyntax(t *testing.T) {
	for _, pointer := range []string{"name", "/trailing~", "/invalid~2escape"} {
		t.Run(pointer, func(t *testing.T) {
			if _, err := parseJSONPointer(pointer); err == nil {
				t.Fatalf("parseJSONPointer(%q) error = nil, want error", pointer)
			}
		})
	}
}
