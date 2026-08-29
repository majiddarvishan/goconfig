package goconfig

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/iancoleman/orderedmap"
	"github.com/rs/cors"
)

const (
	defaultMaxBodySize = int64(10 * 1024 * 1024)
	defaultAddress     = "localhost"
	defaultPort        = 8080
	shutdownTimeout    = 30 * time.Second
	readTimeout        = 15 * time.Second
	writeTimeout       = 15 * time.Second
	idleTimeout        = 60 * time.Second
)

var (
	ErrHTTPServerNotConfigured = errors.New("HTTP server is not configured")
	ErrHTTPServerStarted       = errors.New("HTTP server is already running")
	errHTTPRequestTooLarge     = errors.New("HTTP request body is too large")
)

// HTTPLogger is the minimal logging contract used by HTTPServer.
type HTTPLogger interface {
	Printf(format string, values ...interface{})
}

// Authenticator decides whether an HTTP request may access configuration.
type Authenticator func(*http.Request) bool

// HealthCheck reports whether the service should be considered healthy.
type HealthCheck func(context.Context) error

// CORSConfig controls cross-origin behavior for the reusable handler.
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         int
}

// HTTPTimeouts controls standalone server timeouts.
type HTTPTimeouts struct {
	Read  time.Duration
	Write time.Duration
	Idle  time.Duration
}

// HTTPServer exposes goconfig through a reusable handler, an external route
// registrar, a supplied http.Server, or a package-owned standalone server.
type HTTPServer struct {
	mu sync.Mutex

	address string
	port    int
	manager *Manager

	apiKeyHash [32]byte
	apiKeySet  bool
	auth       Authenticator

	server             *http.Server
	serverOptionSet    bool
	registrar          RouteRegistrar
	registrarOptionSet bool
	logger             HTTPLogger
	cors               CORSConfig
	timeouts           HTTPTimeouts
	maxBodySize        int64
	healthEnabled      bool
	healthCheck        HealthCheck

	handlerOnce sync.Once
	handler     http.Handler
	handlerSet  bool
	running     bool
}

// HttpServer retains the original public spelling.
// Deprecated: use HTTPServer.
type HttpServer = HTTPServer

// HttpServerOption configures HTTPServer. The original spelling is retained
// for v1 source compatibility.
type HttpServerOption func(*HTTPServer)

// HTTPServerOption is the idiomatic spelling of HttpServerOption.
type HTTPServerOption = HttpServerOption

// WithAddress sets the standalone listen address.
func WithAddress(address string) HTTPServerOption {
	return func(server *HTTPServer) { server.address = address }
}

// WithPort sets the standalone listen port.
func WithPort(port int) HTTPServerOption {
	return func(server *HTTPServer) { server.port = port }
}

// WithAPIKey enables X-API-Key authentication without retaining the raw key.
func WithAPIKey(apiKey string) HTTPServerOption {
	return func(server *HTTPServer) {
		server.auth = nil
		server.apiKeyHash = sha256.Sum256([]byte(apiKey))
		server.apiKeySet = apiKey != ""
	}
}

// WithAuthenticator configures request authentication. A nil authenticator
// disables authentication.
func WithAuthenticator(authenticator Authenticator) HTTPServerOption {
	return func(server *HTTPServer) {
		server.auth = authenticator
		server.apiKeyHash = [32]byte{}
		server.apiKeySet = false
	}
}

// WithRouteRegistrar embeds routes into a router or framework adapter.
func WithRouteRegistrar(registrar RouteRegistrar) HTTPServerOption {
	return func(server *HTTPServer) {
		server.registrar = registrar
		server.registrarOptionSet = true
	}
}

// WithServer makes a supplied http.Server own the listener. Existing routes on
// its Handler remain available and goconfig routes are mounted ahead of them.
func WithServer(httpServer *http.Server) HTTPServerOption {
	return func(server *HTTPServer) {
		server.server = httpServer
		server.serverOptionSet = true
	}
}

// WithCORS replaces the default CORS policy.
func WithCORS(config CORSConfig) HTTPServerOption {
	return func(server *HTTPServer) { server.cors = cloneCORSConfig(config) }
}

// WithLogger sets lifecycle logging. Passing nil disables lifecycle logs.
func WithLogger(logger HTTPLogger) HTTPServerOption {
	return func(server *HTTPServer) { server.logger = logger }
}

// WithHTTPTimeouts sets defaults applied when server timeouts are zero.
func WithHTTPTimeouts(timeouts HTTPTimeouts) HTTPServerOption {
	return func(server *HTTPServer) { server.timeouts = timeouts }
}

// WithMaxBodySize sets the maximum POST request size in bytes.
func WithMaxBodySize(size int64) HTTPServerOption {
	return func(server *HTTPServer) { server.maxBodySize = size }
}

// WithHealthCheck enables /health and installs its policy.
func WithHealthCheck(check HealthCheck) HTTPServerOption {
	return func(server *HTTPServer) {
		server.healthEnabled = true
		server.healthCheck = check
	}
}

// WithHealthEnabled controls whether /health is registered.
func WithHealthEnabled(enabled bool) HTTPServerOption {
	return func(server *HTTPServer) { server.healthEnabled = enabled }
}

// NewHTTPServer constructs an HTTP boundary for manager.
func NewHTTPServer(manager *Manager, options ...HTTPServerOption) (*HTTPServer, error) {
	return newHttpServer(manager, options...)
}

func newHttpServer(manager *Manager, options ...HttpServerOption) (*HTTPServer, error) {
	if manager == nil {
		return nil, fmt.Errorf("manager cannot be nil")
	}
	server := &HTTPServer{
		manager:       manager,
		address:       defaultAddress,
		port:          defaultPort,
		logger:        log.Default(),
		cors:          defaultCORSConfig(),
		timeouts:      HTTPTimeouts{Read: readTimeout, Write: writeTimeout, Idle: idleTimeout},
		maxBodySize:   defaultMaxBodySize,
		healthEnabled: true,
	}
	for index, option := range options {
		if option == nil {
			return nil, fmt.Errorf("HTTP server option %d is nil", index)
		}
		option(server)
	}
	if err := server.validateOptions(); err != nil {
		return nil, err
	}
	return server, nil
}

func newHttpServerFromNode(manager *Manager, config *Node) (*HTTPServer, error) {
	options := make([]HTTPServerOption, 0, 3)
	if config != nil {
		if node, err := config.At("address"); err == nil {
			if address, valueErr := node.GetString(); valueErr == nil {
				options = append(options, WithAddress(address))
			}
		}
		if node, err := config.At("port"); err == nil {
			if port, valueErr := node.GetInt(); valueErr == nil {
				options = append(options, WithPort(port))
			}
		}
		if node, err := config.At("api_key"); err == nil {
			if key, valueErr := node.GetString(); valueErr == nil {
				options = append(options, WithAPIKey(key))
			}
		}
	}
	return newHttpServer(manager, options...)
}

func (server *HTTPServer) validateOptions() error {
	if strings.TrimSpace(server.address) == "" {
		return fmt.Errorf("HTTP address cannot be empty")
	}
	if server.port <= 0 || server.port > 65535 {
		return fmt.Errorf("HTTP port must be between 1 and 65535")
	}
	if server.maxBodySize <= 0 || server.maxBodySize == math.MaxInt64 {
		return fmt.Errorf("HTTP maximum body size must be between 1 and %d", int64(math.MaxInt64-1))
	}
	if server.timeouts.Read < 0 || server.timeouts.Write < 0 || server.timeouts.Idle < 0 {
		return fmt.Errorf("HTTP timeouts cannot be negative")
	}
	if server.serverOptionSet && server.server == nil {
		return fmt.Errorf("supplied HTTP server cannot be nil")
	}
	if server.registrarOptionSet && server.registrar == nil {
		return fmt.Errorf("route registrar cannot be nil")
	}
	if server.serverOptionSet && server.registrarOptionSet {
		return fmt.Errorf("HTTP server and route registrar modes are mutually exclusive")
	}
	return nil
}

// Handler returns a reusable, concurrency-safe HTTP handler.
func (server *HTTPServer) Handler() http.Handler {
	server.handlerOnce.Do(func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/config", server.handleConfig)
		if server.healthEnabled {
			mux.HandleFunc("/health", server.handleHealth)
		}
		config := server.cors
		server.handler = cors.New(cors.Options{
			AllowedOrigins: append([]string(nil), config.AllowedOrigins...),
			AllowedMethods: append([]string(nil), config.AllowedMethods...),
			AllowedHeaders: append([]string(nil), config.AllowedHeaders...),
			MaxAge:         config.MaxAge,
		}).Handler(mux)
	})
	return server.handler
}

// RegisterRoutes attaches the reusable handler to an external route registrar.
func (server *HTTPServer) RegisterRoutes(registrar RouteRegistrar) error {
	if registrar == nil {
		return fmt.Errorf("route registrar cannot be nil")
	}
	handler := server.Handler()
	registrar.HandleFunc("/config", handler.ServeHTTP, http.MethodGet, http.MethodPost, http.MethodOptions)
	if server.healthEnabled {
		registrar.HandleFunc("/health", handler.ServeHTTP, http.MethodGet, http.MethodOptions)
	}
	return nil
}

func (server *HTTPServer) registerRoutes(registrar RouteRegistrar) {
	_ = server.RegisterRoutes(registrar)
}

// Start serves according to the configured ownership mode. It blocks while a
// standalone or supplied http.Server is serving and returns listener errors.
func (server *HTTPServer) Start() error {
	if server.registrar != nil {
		return server.RegisterRoutes(server.registrar)
	}

	server.mu.Lock()
	if server.running {
		server.mu.Unlock()
		return ErrHTTPServerStarted
	}
	httpServer := server.prepareHTTPServerLocked()
	server.running = true
	server.mu.Unlock()

	server.logf("starting goconfig HTTP server on %s", httpServer.Addr)
	err := httpServer.ListenAndServe()

	server.mu.Lock()
	server.running = false
	server.mu.Unlock()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (server *HTTPServer) prepareHTTPServerLocked() *http.Server {
	if server.server == nil {
		server.server = &http.Server{Addr: fmt.Sprintf("%s:%d", server.address, server.port)}
	}
	if server.server.Addr == "" {
		server.server.Addr = fmt.Sprintf("%s:%d", server.address, server.port)
	}
	if server.server.Handler == nil {
		server.server.Handler = server.Handler()
		server.handlerSet = true
	} else if !server.handlerSet {
		existing := server.server.Handler
		mux := http.NewServeMux()
		mux.Handle("/config", server.Handler())
		if server.healthEnabled {
			mux.Handle("/health", server.Handler())
		}
		mux.Handle("/", existing)
		server.server.Handler = mux
		server.handlerSet = true
	}
	if server.server.ReadTimeout == 0 {
		server.server.ReadTimeout = server.timeouts.Read
	}
	if server.server.WriteTimeout == 0 {
		server.server.WriteTimeout = server.timeouts.Write
	}
	if server.server.IdleTimeout == 0 {
		server.server.IdleTimeout = server.timeouts.Idle
	}
	return server.server
}

// Shutdown gracefully stops a standalone or supplied server.
func (server *HTTPServer) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("shutdown context cannot be nil")
	}
	server.mu.Lock()
	httpServer := server.server
	server.mu.Unlock()
	if httpServer == nil {
		return nil
	}
	server.logf("shutting down goconfig HTTP server")
	return httpServer.Shutdown(ctx)
}

func (server *HTTPServer) handleConfig(writer http.ResponseWriter, request *http.Request) {
	switch request.Method {
	case http.MethodGet:
		server.onGet(writer, request)
	case http.MethodPost:
		server.onPost(writer, request)
	case http.MethodOptions:
		writer.WriteHeader(http.StatusNoContent)
	default:
		writer.Header().Set("Allow", "GET, POST, OPTIONS")
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (server *HTTPServer) handleHealth(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodOptions {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", "GET, OPTIONS")
		writeError(writer, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if server.healthCheck != nil {
		if err := server.healthCheck(request.Context()); err != nil {
			writeJSON(writer, http.StatusServiceUnavailable, map[string]interface{}{"status": "unavailable"})
			return
		}
	}
	writeJSON(writer, http.StatusOK, map[string]interface{}{"status": "ok"})
}

func (server *HTTPServer) onGet(writer http.ResponseWriter, request *http.Request) {
	if !server.checkAccess(request) {
		writeError(writer, http.StatusUnauthorized, "unauthorized")
		return
	}
	data, err := server.buildConfigState()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to build configuration response")
		return
	}
	writeSuccess(writer, data)
}

type httpMutationRequest struct {
	Operation string          `json:"op"`
	Path      *string         `json:"path"`
	Index     json.RawMessage `json:"index"`
	Value     json.RawMessage `json:"value"`
	Version   json.RawMessage `json:"version"`
}

func (server *HTTPServer) onPost(writer http.ResponseWriter, request *http.Request) {
	if !server.checkAccess(request) {
		writeError(writer, http.StatusUnauthorized, "unauthorized")
		return
	}
	if contentType := request.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			writeError(writer, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return
		}
	}

	payload, err := server.decodeMutationRequest(request)
	if err != nil {
		if errors.Is(err, errHTTPRequestTooLarge) {
			writeError(writer, http.StatusRequestEntityTooLarge, "request body is too large")
		} else {
			writeError(writer, http.StatusBadRequest, err.Error())
		}
		return
	}

	mutation, err := payload.mutationRequest()
	if err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if err := server.manager.mutate(request.Context(), mutation); err != nil {
		server.writeMutationError(writer, err)
		return
	}

	data, err := server.buildConfigState()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "failed to build configuration response")
		return
	}
	writeSuccess(writer, data)
}

func (server *HTTPServer) decodeMutationRequest(request *http.Request) (httpMutationRequest, error) {
	defer request.Body.Close()
	body, err := io.ReadAll(io.LimitReader(request.Body, server.maxBodySize+1))
	if err != nil {
		return httpMutationRequest{}, fmt.Errorf("failed to read request body: %w", err)
	}
	if int64(len(body)) > server.maxBodySize {
		return httpMutationRequest{}, errHTTPRequestTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var payload httpMutationRequest
	if err := decoder.Decode(&payload); err != nil {
		if errors.Is(err, io.EOF) {
			return payload, fmt.Errorf("request body is empty")
		}
		return payload, fmt.Errorf("invalid JSON request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return payload, fmt.Errorf("request body must contain one JSON object")
		}
		return payload, fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return payload, nil
}

func (payload httpMutationRequest) mutationRequest() (mutationRequest, error) {
	if payload.Operation == "" {
		return mutationRequest{}, fmt.Errorf("'op' is required")
	}
	if payload.Path == nil {
		return mutationRequest{}, fmt.Errorf("'path' is required")
	}
	if _, err := parseJSONPointer(*payload.Path); err != nil {
		return mutationRequest{}, fmt.Errorf("invalid path: %w", err)
	}
	expectedVersion, err := parseOptionalInt64(payload.Version, "version", math.MaxInt64)
	if err != nil {
		return mutationRequest{}, err
	}

	request := mutationRequest{path: *payload.Path, expectedVersion: expectedVersion}
	switch payload.Operation {
	case string(mutationInsert):
		request.kind = mutationInsert
		request.index, err = parseRequiredIndex(payload.Index)
		if err != nil {
			return mutationRequest{}, err
		}
		request.value, err = decodeRequiredValue(payload.Value, "insert")
	case string(mutationRemove):
		request.kind = mutationRemove
		request.index, err = parseRequiredIndex(payload.Index)
		if err == nil && len(payload.Value) != 0 {
			err = fmt.Errorf("'value' is not allowed for remove")
		}
	case string(mutationReplace):
		request.kind = mutationReplace
		if len(payload.Index) != 0 {
			return mutationRequest{}, fmt.Errorf("'index' is not allowed for replace")
		}
		request.value, err = decodeRequiredValue(payload.Value, "replace")
	default:
		return mutationRequest{}, fmt.Errorf("unsupported operation: %s", payload.Operation)
	}
	if err != nil {
		return mutationRequest{}, err
	}
	return request, nil
}

func parseRequiredIndex(raw json.RawMessage) (int, error) {
	value, err := parseOptionalInt64(raw, "index", int64(maxInt()))
	if err != nil {
		return 0, err
	}
	if value == nil {
		return 0, fmt.Errorf("'index' is required")
	}
	return int(*value), nil
}

func parseOptionalInt64(raw json.RawMessage, name string, maximum int64) (*int64, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("'%s' must be a non-negative integer", name)
	}
	number, ok := value.(json.Number)
	if !ok || strings.ContainsAny(number.String(), ".eE") {
		return nil, fmt.Errorf("'%s' must be a non-negative integer", name)
	}
	parsed, err := number.Int64()
	if err != nil || parsed < 0 || parsed > maximum {
		return nil, fmt.Errorf("'%s' must be a non-negative integer", name)
	}
	return &parsed, nil
}

func decodeRequiredValue(raw json.RawMessage, operation string) (interface{}, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("'value' is required for %s", operation)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("invalid mutation value: %w", err)
	}
	return value, nil
}

func (server *HTTPServer) buildConfigState() (*orderedmap.OrderedMap, error) {
	snapshot, err := server.manager.snapshot()
	if err != nil {
		return nil, err
	}
	schemaJSON := orderedmap.New()
	if snapshot.schema != "" {
		if err := json.Unmarshal([]byte(snapshot.schema), &schemaJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
		}
	}
	paths := orderedmap.New()
	paths.Set("insertable", snapshot.insertable)
	paths.Set("removable", snapshot.removable)
	paths.Set("replaceable", snapshot.replaceable)
	out := orderedmap.New()
	out.Set("modifiable_paths", paths)
	out.Set("config", snapshot.config)
	out.Set("schema", schemaJSON)
	out.Set("version", snapshot.version)
	return out, nil
}

func HashSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]interface{}{
		"success": false,
		"error": map[string]interface{}{
			"message": message,
			"code":    status,
		},
	})
}

func writeSuccess(writer http.ResponseWriter, data *orderedmap.OrderedMap) {
	writeJSON(writer, http.StatusOK, map[string]interface{}{"success": true, "data": data})
}

func writeJSON(writer http.ResponseWriter, status int, value interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		http.Error(writer, `{"success":false,"error":{"message":"failed to encode response","code":500}}`, http.StatusInternalServerError)
		return
	}
	writer.WriteHeader(status)
	_, _ = writer.Write(data)
}

func (server *HTTPServer) writeMutationError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrVersionConflict):
		writeError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, ErrValidation):
		writeError(writer, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrPathNotFound):
		writeError(writer, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrMutationDenied):
		writeError(writer, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrInvalidPath), errors.Is(err, ErrTypeMismatch), errors.Is(err, ErrIndexOutOfRange):
		writeError(writer, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrPersistence):
		writeError(writer, http.StatusInternalServerError, "configuration persistence failed")
	default:
		writeError(writer, http.StatusInternalServerError, "configuration mutation failed")
	}
}

func (server *HTTPServer) checkAccess(request *http.Request) bool {
	if server.auth != nil {
		return server.auth(request)
	}
	if !server.apiKeySet {
		return true
	}
	providedKey := request.Header.Get("X-API-Key")
	if providedKey == "" {
		return false
	}
	providedHash := sha256.Sum256([]byte(providedKey))
	return subtle.ConstantTimeCompare(server.apiKeyHash[:], providedHash[:]) == 1
}

func (server *HTTPServer) logf(format string, values ...interface{}) {
	if server.logger != nil {
		server.logger.Printf(format, values...)
	}
}

func defaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-API-Key"},
		MaxAge:         3600,
	}
}

func cloneCORSConfig(config CORSConfig) CORSConfig {
	config.AllowedOrigins = append([]string(nil), config.AllowedOrigins...)
	config.AllowedMethods = append([]string(nil), config.AllowedMethods...)
	config.AllowedHeaders = append([]string(nil), config.AllowedHeaders...)
	return config
}
