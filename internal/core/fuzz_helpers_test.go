package goconfig

import (
	"bytes"
	"net/http"
)

type fuzzTestingContext interface {
	Helper()
	Fatalf(string, ...interface{})
}

func fuzzManager(testingContext fuzzTestingContext) *Manager {
	testingContext.Helper()
	source, err := NewStrSource(
		`{"name":"fuzz","a/b":true,"items":[{"id":1,"name":"one"}]}`,
		`{"type":"object"}`,
	)
	if err != nil {
		testingContext.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		testingContext.Fatalf("NewManager() error = %v", err)
	}
	return manager
}

func newFuzzRequest(body []byte) *http.Request {
	request, _ := http.NewRequest(http.MethodPost, "http://fuzz.test/config", bytes.NewReader(body))
	return request
}
