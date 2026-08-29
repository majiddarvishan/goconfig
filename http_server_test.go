package goconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
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
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.Code != http.StatusOK || payload["status"] != "ok" {
		t.Fatalf("health response = %d %q", response.Code, response.Body.String())
	}
}

func TestManagerHandlerUsesSafeDefaults(t *testing.T) {
	manager, _ := newTestManager(t)
	response := httptest.NewRecorder()
	manager.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/config", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHTTPStrictRequestDecoding(t *testing.T) {
	manager, _ := newTestManager(t)
	items := mustNodeAt(t, manager.Config(), "items")
	if err := manager.OnInsert(items, nil); err != nil {
		t.Fatalf("OnInsert() error = %v", err)
	}
	server, err := newHttpServer(manager, WithMaxBodySize(128))
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}

	tests := []struct {
		name        string
		body        string
		contentType string
		status      int
	}{
		{name: "unknown field", body: `{"op":"insert","path":"/items","index":1,"value":{},"extra":true}`, status: http.StatusBadRequest},
		{name: "exponent index", body: `{"op":"insert","path":"/items","index":1e0,"value":{}}`, status: http.StatusBadRequest},
		{name: "null version", body: `{"op":"insert","path":"/items","index":1,"value":{},"version":null}`, status: http.StatusBadRequest},
		{name: "trailing JSON", body: `{"op":"insert","path":"/items","index":1,"value":{}} {}`, status: http.StatusBadRequest},
		{name: "wrong content type", body: `{"op":"insert"}`, contentType: "text/plain", status: http.StatusUnsupportedMediaType},
		{name: "too large", body: strings.Repeat(" ", 128) + `{"op":"insert"}`, status: http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader(test.body))
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestHTTPRootReplacementAcceptsEmptyJSONPointer(t *testing.T) {
	source, err := NewStrSource(`{"name":"old"}`, `{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`)
	if err != nil {
		t.Fatalf("NewStrSource() error = %v", err)
	}
	manager, err := NewManager(source)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.OnReplace(manager.Config(), nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	server, _ := newHttpServer(manager)
	request := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader(`{"op":"replace","path":"","value":{"name":"new"}}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHTTPMapsMutationErrors(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	manager.AddValidator("/name", func(string, *Node, *Node) error { return errors.New("rejected") })
	server, _ := newHttpServer(manager)

	response := performHTTPMutation(server.Handler(), `{"op":"replace","path":"/name","value":"new"}`)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("validation status = %d, body = %s", response.Code, response.Body.String())
	}

	missingManager, _ := newTestManager(t)
	missingManager.modifiables = append(missingManager.modifiables, modifiable{Type: Replaceable, Path: "/missing"})
	missingServer, _ := newHttpServer(missingManager)
	response = performHTTPMutation(missingServer.Handler(), `{"op":"replace","path":"/missing","value":"new"}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("missing path status = %d, body = %s", response.Code, response.Body.String())
	}

	failingSource := newFailingLegacySource(t)
	failingManager, err := NewManager(failingSource)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	failingName := mustNodeAt(t, failingManager.Config(), "name")
	if err := failingManager.OnReplace(failingName, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	failingSource.writeErr = errors.New("sensitive disk detail")
	failingServer, _ := newHttpServer(failingManager)
	response = performHTTPMutation(failingServer.Handler(), `{"op":"replace","path":"/name","value":"new"}`)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "sensitive") {
		t.Fatalf("persistence response = %d %s", response.Code, response.Body.String())
	}
}

func TestHTTPConfigurableCORSAuthenticationAndHealth(t *testing.T) {
	manager, _ := newTestManager(t)
	server, err := newHttpServer(manager,
		WithAuthenticator(func(request *http.Request) bool { return request.Header.Get("Authorization") == "allowed" }),
		WithCORS(CORSConfig{
			AllowedOrigins: []string{"https://client.test"},
			AllowedMethods: []string{http.MethodGet, http.MethodOptions},
			AllowedHeaders: []string{"Authorization"},
		}),
		WithHealthCheck(func(context.Context) error { return errors.New("unhealthy") }),
	)
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}

	unauthorized := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/config", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/config", nil)
	request.Header.Set("Authorization", "allowed")
	request.Header.Set("Origin", "https://client.test")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Access-Control-Allow-Origin") != "https://client.test" {
		t.Fatalf("authorized CORS response = %d, headers = %#v", response.Code, response.Header())
	}

	health := httptest.NewRecorder()
	server.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusServiceUnavailable {
		t.Fatalf("health status = %d, want 503", health.Code)
	}
}

func TestHTTPSuppliedServerPreservesExistingHandler(t *testing.T) {
	manager, _ := newTestManager(t)
	supplied := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	})}
	server, err := newHttpServer(manager, WithServer(supplied), WithLogger(nil))
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}
	server.mu.Lock()
	prepared := server.prepareHTTPServerLocked()
	server.mu.Unlock()

	configResponse := httptest.NewRecorder()
	prepared.Handler.ServeHTTP(configResponse, httptest.NewRequest(http.MethodGet, "/config", nil))
	if configResponse.Code != http.StatusOK {
		t.Fatalf("config status = %d", configResponse.Code)
	}
	existingResponse := httptest.NewRecorder()
	prepared.Handler.ServeHTTP(existingResponse, httptest.NewRequest(http.MethodGet, "/existing", nil))
	if existingResponse.Code != http.StatusCreated {
		t.Fatalf("existing handler status = %d", existingResponse.Code)
	}
}

func TestHTTPRouteRegistrarIntegration(t *testing.T) {
	manager, _ := newTestManager(t)
	registrar := &recordingRegistrar{handlers: make(map[string]http.HandlerFunc)}
	server, err := newHttpServer(manager, WithRouteRegistrar(registrar))
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	response := httptest.NewRecorder()
	registrar.handlers["/config"](response, httptest.NewRequest(http.MethodGet, "/config", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("registered config status = %d", response.Code)
	}
}

func TestHTTPConcurrentExpectedVersionAllowsOneCommit(t *testing.T) {
	manager, _ := newTestManager(t)
	name := mustNodeAt(t, manager.Config(), "name")
	if err := manager.OnReplace(name, nil); err != nil {
		t.Fatalf("OnReplace() error = %v", err)
	}
	server, _ := newHttpServer(manager)
	start := make(chan struct{})
	statuses := make(chan int, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, value := range []string{"first", "second"} {
		value := value
		go func() {
			ready.Done()
			<-start
			body := `{"op":"replace","path":"/name","value":"` + value + `","version":1}`
			statuses <- performHTTPMutation(server.Handler(), body).Code
		}()
	}
	ready.Wait()
	close(start)
	counts := map[int]int{}
	counts[<-statuses]++
	counts[<-statuses]++
	if counts[http.StatusOK] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("statuses = %#v, want one 200 and one 409", counts)
	}
}

func TestHTTPLifecycleReturnsErrors(t *testing.T) {
	manager, _ := newTestManager(t)
	if err := manager.StartHTTPServer(); !errors.Is(err, ErrHTTPServerNotConfigured) {
		t.Fatalf("StartHTTPServer() error = %v, want not configured", err)
	}
	if err := manager.ShutdownHTTPServer(context.Background()); !errors.Is(err, ErrHTTPServerNotConfigured) {
		t.Fatalf("ShutdownHTTPServer() error = %v, want not configured", err)
	}
	for _, option := range []HTTPServerOption{
		WithPort(0),
		WithMaxBodySize(0),
		WithHTTPTimeouts(HTTPTimeouts{Read: -time.Second}),
		WithServer(nil),
		WithRouteRegistrar(nil),
	} {
		if _, err := newHttpServer(manager, option); err == nil {
			t.Fatalf("newHttpServer(%T) error = nil, want error", option)
		}
	}
	badListener := &http.Server{Addr: "127.0.0.1:-1"}
	server, err := newHttpServer(manager, WithServer(badListener), WithLogger(nil))
	if err != nil {
		t.Fatalf("newHttpServer() error = %v", err)
	}
	if err := server.Start(); err == nil {
		t.Fatal("Start() listener error = nil, want error")
	}
	if err := server.Shutdown(nil); err == nil {
		t.Fatal("Shutdown(nil) error = nil, want error")
	}
}

func performHTTPMutation(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/config", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type recordingRegistrar struct {
	handlers map[string]http.HandlerFunc
}

func (registrar *recordingRegistrar) HandleFunc(path string, handler http.HandlerFunc, _ ...string) {
	registrar.handlers[path] = handler
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
