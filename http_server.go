package goconfig

import (
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
	"net/http"
	"time"

	"github.com/iancoleman/orderedmap"
	"github.com/rs/cors"
)

const (
	maxBodySize     = 10 * 1024 * 1024 // 10MB
	defaultAddress  = "localhost"
	defaultPort     = 8080
	shutdownTimeout = 30 * time.Second
	readTimeout     = 15 * time.Second
	writeTimeout    = 15 * time.Second
	idleTimeout     = 60 * time.Second
)

type HttpServer struct {
	address    string
	port       int
	apiKey     string
	apiKeyHash [32]byte
	manager    *Manager

	server    *http.Server
	registrar RouteRegistrar
}

// ─────────────────────────────────────────────────────────────
// OPTIONS
// ─────────────────────────────────────────────────────────────

type HttpServerOption func(*HttpServer)

func WithAddress(address string) HttpServerOption {
	return func(hs *HttpServer) { hs.address = address }
}

func WithPort(port int) HttpServerOption {
	return func(hs *HttpServer) {
		if port > 0 && port <= 65535 {
			hs.port = port
		}
	}
}

func WithAPIKey(apiKey string) HttpServerOption {
	return func(hs *HttpServer) {
		if apiKey != "" {
			hs.apiKey = apiKey
			hs.apiKeyHash = sha256.Sum256([]byte(apiKey))
		}
	}
}

// Use when embedding goconfig into another web service
func WithRouteRegistrar(r RouteRegistrar) HttpServerOption {
	return func(hs *HttpServer) {
		hs.registrar = r
	}
}

// Use when goconfig owns the server
func WithServer(server *http.Server) HttpServerOption {
	return func(hs *HttpServer) {
		hs.server = server
	}
}

// ─────────────────────────────────────────────────────────────
// CONSTRUCTORS
// ─────────────────────────────────────────────────────────────

func newHttpServer(m *Manager, opts ...HttpServerOption) (*HttpServer, error) {
	if m == nil {
		return nil, fmt.Errorf("manager cannot be nil")
	}

	hs := &HttpServer{
		manager: m,
		address: defaultAddress,
		port:    defaultPort,
	}

	for _, opt := range opts {
		opt(hs)
	}

	return hs, nil
}

func newHttpServerFromNode(m *Manager, conf *Node) (*HttpServer, error) {
	hs, err := newHttpServer(m)
	if err != nil {
		return nil, err
	}

	if conf == nil {
		return hs, nil
	}

	if n, err := conf.At("address"); err == nil {
		if s, _ := n.GetString(); s != "" {
			hs.address = s
		}
	}

	if n, err := conf.At("port"); err == nil {
		if p, _ := n.GetInt(); p > 0 {
			hs.port = p
		}
	}

	if n, err := conf.At("api_key"); err == nil {
		if k, _ := n.GetString(); k != "" {
			hs.apiKey = k
			hs.apiKeyHash = sha256.Sum256([]byte(k))
		}
	}

	return hs, nil
}

// ─────────────────────────────────────────────────────────────
// ROUTES
// ─────────────────────────────────────────────────────────────

func (hs *HttpServer) registerRoutes(r RouteRegistrar) {
	r.HandleFunc("/config", hs.handleConfig, "GET", "POST", "OPTIONS")
	r.HandleFunc("/health", hs.handleHealth, "GET")
}

// ─────────────────────────────────────────────────────────────
// START / STOP
// ─────────────────────────────────────────────────────────────

func (hs *HttpServer) Start() error {
	// CASE 1: embedded into external web service
	if hs.registrar != nil {
		hs.registerRoutes(hs.registrar)
		log.Println("goconfig routes registered on external server")
		return nil
	}

	// CASE 2: goconfig owns HTTP server
	mux := http.NewServeMux()
	hs.registerRoutes(muxAdapter{mux})

	handler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-API-Key"},
		MaxAge:         3600,
	}).Handler(mux)

	if hs.server == nil {
		addr := fmt.Sprintf("%s:%d", hs.address, hs.port)
		hs.server = &http.Server{
			Addr:         addr,
			Handler:      handler,
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  idleTimeout,
		}
		log.Printf("Starting goconfig HTTP server on %s", addr)
	}

	if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (hs *HttpServer) Shutdown(ctx context.Context) error {
	if hs.server == nil {
		return nil
	}
	log.Println("Shutting down goconfig HTTP server")
	return hs.server.Shutdown(ctx)
}

// ─────────────────────────────────────────────────────────────
// HANDLERS
// ─────────────────────────────────────────────────────────────

func (hs *HttpServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		hs.onGet(w, r)
	case http.MethodPost:
		hs.onPost(w, r)
	case http.MethodOptions:
		hs.onOptions(w)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (hs *HttpServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

////////////////////////////////////////////////////////////////////////////////
// GET
////////////////////////////////////////////////////////////////////////////////

func (hs *HttpServer) onGet(w http.ResponseWriter, r *http.Request) {
	if !hs.checkAccess(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	data, err := hs.buildConfigState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build config: %s", err))
		return
	}

	writeSuccess(w, data)
}

////////////////////////////////////////////////////////////////////////////////
// POST
////////////////////////////////////////////////////////////////////////////////

func (hs *HttpServer) onPost(w http.ResponseWriter, r *http.Request) {
	if !hs.checkAccess(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("could not read body: %s", err))
		return
	}

	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "request body is empty")
		return
	}

	bodyJSON := orderedmap.New()
	if err := json.Unmarshal(body, &bodyJSON); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %s", err))
		return
	}

	op, err := getString(bodyJSON, "op")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	path, err := getString(bodyJSON, "path")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validate path format
	if path == "" || path[0] != '/' {
		writeError(w, http.StatusBadRequest, "path must start with '/'")
		return
	}

	value, hasValue := bodyJSON.Get("value")

	// Version-based optimistic locking (better than hash)
	var expectedVersion *int64
	if versionVal, ok := bodyJSON.Get("version"); ok {
		versionFloat, ok := versionVal.(float64)
		if !ok || math.Trunc(versionFloat) != versionFloat || versionFloat < 0 || versionFloat >= math.MaxInt64 {
			writeError(w, http.StatusBadRequest, "version must be a number")
			return
		}
		version := int64(versionFloat)
		expectedVersion = &version
	}

	// Execute operation
	switch op {
	case "insert":
		if !hasValue {
			writeError(w, http.StatusBadRequest, "value is required for insert")
			return
		}

		index, err := getIndex(bodyJSON)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := hs.manager.mutate(r.Context(), mutationRequest{kind: mutationInsert, path: path, index: index, value: value, expectedVersion: expectedVersion}); err != nil {
			hs.writeMutationError(w, err)
			return
		}

	case "remove":
		index, err := getIndex(bodyJSON)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if err := hs.manager.mutate(r.Context(), mutationRequest{kind: mutationRemove, path: path, index: index, expectedVersion: expectedVersion}); err != nil {
			hs.writeMutationError(w, err)
			return
		}

	case "replace":
		if !hasValue {
			writeError(w, http.StatusBadRequest, "value is required for replace")
			return
		}

		if err := hs.manager.mutate(r.Context(), mutationRequest{kind: mutationReplace, path: path, value: value, expectedVersion: expectedVersion}); err != nil {
			hs.writeMutationError(w, err)
			return
		}

	default:
		writeError(w, http.StatusBadRequest, fmt.Sprintf("unsupported operation: %s", op))
		return
	}

	// Build updated config for response
	data, err := hs.buildConfigState()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build config: %s", err))
		return
	}

	writeSuccess(w, data)
}

////////////////////////////////////////////////////////////////////////////////
// OPTIONS
////////////////////////////////////////////////////////////////////////////////

func (hs *HttpServer) onOptions(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, X-API-Key")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.WriteHeader(http.StatusOK)
}

////////////////////////////////////////////////////////////////////////////////
// BUILD CONFIG STATE
////////////////////////////////////////////////////////////////////////////////

func (hs *HttpServer) buildConfigState() (*orderedmap.OrderedMap, error) {
	snapshot, err := hs.manager.snapshot()
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

////////////////////////////////////////////////////////////////////////////////
// HELPERS
////////////////////////////////////////////////////////////////////////////////

func HashSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	errObj := orderedmap.New()
	errObj.Set("message", msg)
	errObj.Set("code", code)

	resp := orderedmap.New()
	resp.Set("success", false)
	resp.Set("error", errObj)

	out, _ := json.MarshalIndent(resp, "", "  ")
	w.Write(out)
}

func writeSuccess(w http.ResponseWriter, data *orderedmap.OrderedMap) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := orderedmap.New()
	resp.Set("success", true)
	resp.Set("data", data)

	out, _ := json.MarshalIndent(resp, "", "  ")
	w.Write(out)
}

func getString(m *orderedmap.OrderedMap, key string) (string, error) {
	v, ok := m.Get(key)
	if !ok {
		return "", fmt.Errorf("'%s' is missing", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("'%s' must be a string", key)
	}
	if s == "" {
		return "", fmt.Errorf("'%s' cannot be empty", key)
	}
	return s, nil
}

func getIndex(m *orderedmap.OrderedMap) (int, error) {
	val, ok := m.Get("index")
	if !ok {
		return 0, fmt.Errorf("'index' is missing")
	}
	f, ok := val.(float64)
	if !ok {
		return 0, fmt.Errorf("'index' must be a number")
	}
	if f < 0 || math.Trunc(f) != f || f > float64(maxInt()) {
		return 0, fmt.Errorf("'index' must be a non-negative integer")
	}
	return int(f), nil
}

func (hs *HttpServer) writeMutationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrVersionConflict):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrPersistence):
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		writeError(w, http.StatusBadRequest, err.Error())
	}
}

func (hs *HttpServer) checkAccess(r *http.Request) bool {
	if hs.apiKey == "" {
		return true // No auth required if no key set
	}

	providedKey := r.Header.Get("X-API-Key")
	if providedKey == "" {
		return false
	}

	// Constant-time comparison to prevent timing attacks
	providedHash := sha256.Sum256([]byte(providedKey))
	return subtle.ConstantTimeCompare(hs.apiKeyHash[:], providedHash[:]) == 1
}
