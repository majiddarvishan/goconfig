// Package httpserver serves HTTP routes with address/port/API-key/CORS
// concerns handled centrally. It has no dependency in the other direction:
// goconfig.Manager works standalone and never imports this package, so
// applications that don't need HTTP never pull it in.
//
// Two independent models are supported:
//
//   - Manager-bound (NewHttpServer/NewHttpServerFromNode): built from a
//     *goconfig.Manager, serves /config and /health. Either goconfig owns the
//     listener (Start/Shutdown) or the caller embeds it into its own
//     router/mux (RegisterRoutes, or the WithRouteRegistrar option + Start).
//   - Generic (NewServer): a plain HTTP server independent of any Manager.
//     Routes are added explicitly via AddRoute/AddRoutes - e.g. wire in a
//     Manager's own routes with AddRoutes(manager.GetRoutes()) only if/when
//     needed. apiKey, if set, protects every route added this way.
package httpserver

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/rs/cors"

	"github.com/majiddarvishan/goconfig"
)

const (
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
	manager    *goconfig.Manager

	baseAPI string
	mux     *http.ServeMux

	server    *http.Server
	registrar RouteRegistrar
}

// ─────────────────────────────────────────────────────────────
// OPTIONS (manager-bound model)
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

// WithRouteRegistrar is used when embedding goconfig into another web service.
func WithRouteRegistrar(r RouteRegistrar) HttpServerOption {
	return func(hs *HttpServer) {
		hs.registrar = r
	}
}

// WithServer is used when goconfig owns the server.
func WithServer(server *http.Server) HttpServerOption {
	return func(hs *HttpServer) {
		hs.server = server
	}
}

// ─────────────────────────────────────────────────────────────
// CONSTRUCTORS
// ─────────────────────────────────────────────────────────────

func NewHttpServer(m *goconfig.Manager, opts ...HttpServerOption) (*HttpServer, error) {
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

func NewHttpServerFromNode(m *goconfig.Manager, conf *goconfig.Node) (*HttpServer, error) {
	hs, err := NewHttpServer(m)
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

// NewServer creates a generic HTTP server with no dependency on any
// goconfig.Manager. Routes are added explicitly via AddRoute/AddRoutes -
// e.g. wire a Manager's own routes in with AddRoutes(manager.GetRoutes())
// only if/when needed.
//
// apiKey, if non-empty, protects every route added through AddRoute/AddRoutes
// (including your own, e.g. a custom /health). baseAPI is prepended to every
// route path and may be empty.
func NewServer(ip string, port int, apiKey string, baseAPI string) (*HttpServer, error) {
	hs := &HttpServer{
		address: ip,
		port:    port,
		baseAPI: baseAPI,
		mux:     http.NewServeMux(),
	}

	if apiKey != "" {
		hs.apiKey = apiKey
		hs.apiKeyHash = sha256.Sum256([]byte(apiKey))
	}

	return hs, nil
}

// ─────────────────────────────────────────────────────────────
// ROUTES
// ─────────────────────────────────────────────────────────────

// RegisterRoutes wires /health and the manager's own routes (see
// goconfig.Manager.GetRoutes) onto r. Use this directly when the caller owns
// its own web service/router and only wants goconfig's routes added to it,
// without goconfig ever starting its own server. Requires a manager-bound
// server (built with NewHttpServer/NewHttpServerFromNode).
func (hs *HttpServer) RegisterRoutes(r RouteRegistrar) error {
	if r == nil {
		return fmt.Errorf("registrar cannot be nil")
	}
	if hs.manager == nil {
		return fmt.Errorf("RegisterRoutes requires a server created with NewHttpServer/NewHttpServerFromNode")
	}

	r.HandleFunc("/health", hs.handleHealth, "GET")
	for _, route := range hs.manager.GetRoutes() {
		r.HandleFunc(route.Path, hs.protect(route.Handler), route.Methods...)
	}
	return nil
}

// AddRoute registers path (prefixed with baseAPI) on the server, protected by
// apiKey if one was set in NewServer. Only valid for servers created with
// NewServer.
func (hs *HttpServer) AddRoute(path string, handler http.HandlerFunc, _ ...string) error {
	if hs.mux == nil {
		return fmt.Errorf("AddRoute requires a server created with NewServer")
	}
	hs.mux.HandleFunc(hs.baseAPI+path, hs.protect(handler))
	return nil
}

// AddRoutes registers multiple routes at once, e.g. manager.GetRoutes().
// Only valid for servers created with NewServer.
func (hs *HttpServer) AddRoutes(routes []goconfig.Route) error {
	for _, route := range routes {
		if err := hs.AddRoute(route.Path, route.Handler, route.Methods...); err != nil {
			return err
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────
// START / STOP
// ─────────────────────────────────────────────────────────────

func (hs *HttpServer) Start() error {
	// CASE 1: embedded into external web service
	if hs.registrar != nil {
		if err := hs.RegisterRoutes(hs.registrar); err != nil {
			return err
		}
		log.Println("goconfig routes registered on external server")
		return nil
	}

	var mux *http.ServeMux
	if hs.mux != nil {
		// Generic model: routes already added via AddRoute/AddRoutes.
		mux = hs.mux
	} else {
		// Manager-bound model: goconfig owns the mux.
		mux = http.NewServeMux()
		if err := hs.RegisterRoutes(muxAdapter{mux}); err != nil {
			return err
		}
	}

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

func (hs *HttpServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

// ─────────────────────────────────────────────────────────────
// ACCESS CONTROL
// ─────────────────────────────────────────────────────────────

// protect wraps next so it only runs when checkAccess passes.
func (hs *HttpServer) protect(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hs.checkAccess(r) {
			writeUnauthorized(w)
			return
		}
		next(w, r)
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"success":false,"error":{"message":"unauthorized","code":401}}`))
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

////////////////////////////////////////////////////////////////////////////////
// HELPERS
////////////////////////////////////////////////////////////////////////////////

func HashSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
