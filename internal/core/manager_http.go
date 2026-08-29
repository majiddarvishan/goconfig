package goconfig

import (
	"context"
	"net/http"
)

// NewHttpServerFromNode configures HTTP from a Node.
// Deprecated: use NewHTTPServer with options.
func (m *Manager) NewHttpServerFromNode(config *Node) error {
	server, err := newHttpServerFromNode(m, config)
	if err != nil {
		return err
	}
	m.httpMu.Lock()
	m.httpServer = server
	m.httpMu.Unlock()
	return nil
}

// NewHttpServer configures the Manager-owned HTTP boundary.
// Deprecated: use NewHTTPServer when direct lifecycle control is preferred.
func (m *Manager) NewHttpServer(options ...HttpServerOption) error {
	server, err := newHttpServer(m, options...)
	if err != nil {
		return err
	}
	m.httpMu.Lock()
	m.httpServer = server
	m.httpMu.Unlock()
	return nil
}

// Handler returns the configured reusable HTTP handler. If HTTP has not been
// configured, it installs a server with safe defaults.
func (m *Manager) Handler() http.Handler {
	m.httpMu.Lock()
	defer m.httpMu.Unlock()
	if m.httpServer == nil {
		m.httpServer, _ = newHttpServer(m)
	}
	return m.httpServer.Handler()
}

// StartHTTPServer starts the configured server and returns listener errors.
func (m *Manager) StartHTTPServer() error {
	server, err := m.configuredHTTPServer()
	if err != nil {
		return err
	}
	return server.Start()
}

// StartHttpServer retains the original asynchronous behavior without panic.
// Deprecated: use StartHTTPServer and handle the returned error.
func (m *Manager) StartHttpServer() {
	go func() { _ = m.StartHTTPServer() }()
}

// ShutdownHTTPServer gracefully stops the configured HTTP server.
func (m *Manager) ShutdownHTTPServer(ctx context.Context) error {
	server, err := m.configuredHTTPServer()
	if err != nil {
		return err
	}
	return server.Shutdown(ctx)
}

// StopHttpServer retains the original best-effort shutdown behavior.
// Deprecated: use ShutdownHTTPServer and handle the returned error.
func (m *Manager) StopHttpServer() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	_ = m.ShutdownHTTPServer(ctx)
}

// RegisterHTTPRoutes attaches the configured handler to a route registrar.
func (m *Manager) RegisterHTTPRoutes(registrar RouteRegistrar) error {
	server, err := m.configuredHTTPServer()
	if err != nil {
		return err
	}
	return server.RegisterRoutes(registrar)
}

// SetupRoutes retains the original no-error adapter.
// Deprecated: use RegisterHTTPRoutes.
func (m *Manager) SetupRoutes(registrar RouteRegistrar) { _ = m.RegisterHTTPRoutes(registrar) }

func (m *Manager) configuredHTTPServer() (*HTTPServer, error) {
	m.httpMu.RLock()
	defer m.httpMu.RUnlock()
	if m.httpServer == nil {
		return nil, ErrHTTPServerNotConfigured
	}
	return m.httpServer, nil
}
