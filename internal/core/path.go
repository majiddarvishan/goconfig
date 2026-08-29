package goconfig

import (
	"fmt"
	"strings"
)

// parseJSONPointer parses an RFC 6901 JSON Pointer into unescaped path
// segments. The empty pointer identifies the document root.
func parseJSONPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return []string{}, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("JSON pointer must be empty or start with '/': %q", pointer)
	}

	encodedSegments := strings.Split(pointer[1:], "/")
	segments := make([]string, len(encodedSegments))
	for i, segment := range encodedSegments {
		decoded, err := unescapeJSONPointerSegment(segment)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON pointer segment %q: %w", segment, err)
		}
		segments[i] = decoded
	}
	return segments, nil
}

// buildJSONPointer builds a canonical RFC 6901 JSON Pointer. An empty segment
// list identifies the document root.
func buildJSONPointer(segments []string) string {
	if len(segments) == 0 {
		return ""
	}

	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = escapeJSONPointerSegment(segment)
	}
	return "/" + strings.Join(escaped, "/")
}

func escapeJSONPointerSegment(segment string) string {
	segment = strings.ReplaceAll(segment, "~", "~0")
	return strings.ReplaceAll(segment, "/", "~1")
}

func unescapeJSONPointerSegment(segment string) (string, error) {
	var result strings.Builder
	result.Grow(len(segment))

	for i := 0; i < len(segment); i++ {
		if segment[i] != '~' {
			result.WriteByte(segment[i])
			continue
		}
		if i+1 >= len(segment) {
			return "", fmt.Errorf("trailing '~' escape")
		}

		switch segment[i+1] {
		case '0':
			result.WriteByte('~')
		case '1':
			result.WriteByte('/')
		default:
			return "", fmt.Errorf("unsupported escape ~%c", segment[i+1])
		}
		i++
	}
	return result.String(), nil
}
