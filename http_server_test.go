package goconfig

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPConfigRequiresConfiguredAPIKey(t *testing.T) {
	manager, _ := newTestManager(t)
	server, err := newHttpServer(manager, WithAPIKey("secret"))
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/config", nil)
	response := httptest.NewRecorder()
	server.handleConfig(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}

	request = httptest.NewRequest(http.MethodGet, "/config", nil)
	request.Header.Set("X-API-Key", "secret")
	response = httptest.NewRecorder()
	server.handleConfig(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestHTTPInsertAndVersionConflict(t *testing.T) {
	manager, _ := newTestManager(t)
	items := mustNodeAt(t, manager.Config(), "items")
	if err := manager.OnInsert(items, nil); err != nil {
		t.Fatalf("OnInsert() error = %v", err)
	}
	server, err := newHttpServer(manager)
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}

	body := []byte(`{
      "op":"insert",
      "path":"/items",
      "index":1,
      "value":{"id":3,"name":"three"},
      "version":1
    }`)
	request := httptest.NewRequest(http.MethodPost, "/config", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.handleConfig(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("insert status = %d, body = %s", response.Code, response.Body.String())
	}
	if manager.Version() != 2 {
		t.Fatalf("version = %d, want 2", manager.Version())
	}

	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Version int64 `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Success || payload.Data.Version != 2 {
		t.Fatalf("response payload = %#v", payload)
	}

	request = httptest.NewRequest(http.MethodPost, "/config", bytes.NewReader(body))
	response = httptest.NewRecorder()
	server.handleConfig(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale version status = %d, want %d", response.Code, http.StatusConflict)
	}
}

func TestHTTPHealth(t *testing.T) {
	manager, _ := newTestManager(t)
	server, err := newHttpServer(manager)
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}

	response := httptest.NewRecorder()
	server.handleHealth(response, httptest.NewRequest(http.MethodGet, "/health", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"status":"ok"}` {
		t.Fatalf("health response = %d %q", response.Code, response.Body.String())
	}
}

func TestHTTPRejectsFractionalIndexAndVersion(t *testing.T) {
	manager, _ := newTestManager(t)
	items := mustNodeAt(t, manager.Config(), "items")
	if err := manager.OnInsert(items, nil); err != nil {
		t.Fatalf("OnInsert() error = %v", err)
	}
	server, err := newHttpServer(manager)
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}

	for _, body := range []string{
		`{"op":"insert","path":"/items","index":1.5,"value":{"id":3,"name":"three"}}`,
		`{"op":"insert","path":"/items","index":1,"value":{"id":3,"name":"three"},"version":1.5}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/config", bytes.NewBufferString(body))
		response := httptest.NewRecorder()
		server.handleConfig(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	}
	if manager.Version() != 1 {
		t.Fatalf("version = %d, want 1", manager.Version())
	}
}
