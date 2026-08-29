package goconfig

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHTTPFileSourceEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(testConfigJSON), 0o600); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	source, err := NewFileSource(path, testSchemaJSON)
	if err != nil {
		t.Fatalf("NewFileSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	boundary, err := NewHTTPServer(manager, WithAPIKey("integration-secret"))
	if err != nil {
		t.Fatalf("NewHTTPServer() error = %v", err)
	}
	client := &http.Client{Transport: handlerRoundTripper{handler: boundary.Handler()}}

	body := []byte(`{"op":"replace","path":"/name","value":"integrated","version":1}`)
	request, err := http.NewRequest(http.MethodPost, "http://config.test/config", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-Key", "integration-secret")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("POST /config error = %v", err)
	}
	responseBody, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatalf("read response: %v", readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST status = %d, body = %s", response.StatusCode, responseBody)
	}

	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted config: %v", err)
	}
	parsed, err := parseConfig(disk)
	if err != nil {
		t.Fatalf("parse persisted config: %v", err)
	}
	if value, _ := parsed.Get("name"); value != "integrated" {
		t.Fatalf("persisted name = %v, want integrated", value)
	}
	if manager.Version() != 2 || len(manager.History()) != 1 {
		t.Fatalf("manager version/history = %d/%d, want 2/1", manager.Version(), len(manager.History()))
	}

	getRequest, _ := http.NewRequest(http.MethodGet, "http://config.test/config", nil)
	getRequest.Header.Set("X-API-Key", "integration-secret")
	getResponse, err := client.Do(getRequest)
	if err != nil {
		t.Fatalf("GET /config error = %v", err)
	}
	defer getResponse.Body.Close()
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Version int64 `json:"version"`
			Config  struct {
				Name string `json:"name"`
			} `json:"config"`
		} `json:"data"`
	}
	if err := json.NewDecoder(getResponse.Body).Decode(&payload); err != nil {
		t.Fatalf("decode GET response: %v", err)
	}
	if !payload.Success || payload.Data.Version != 2 || payload.Data.Config.Name != "integrated" {
		t.Fatalf("GET payload = %#v", payload)
	}
}

type handlerRoundTripper struct {
	handler http.Handler
}

func (transport handlerRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	transport.handler.ServeHTTP(recorder, request)
	return recorder.Result(), nil
}
