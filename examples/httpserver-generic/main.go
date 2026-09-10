// Example: the generic httpserver.NewServer model - a plain HTTP server with
// no dependency on goconfig.Manager. Routes (including your own, like
// /health here) are added explicitly via AddRoute, protected uniformly by
// apiKey. A Manager's own routes are wired in only if/when needed, via
// AddRoutes(manager.GetRoutes()).
// Try it with:
//
//	curl -H "X-API-Key: secret" http://localhost:8084/api/v1/health
//	curl -H "X-API-Key: secret" http://localhost:8084/api/v1/config
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/majiddarvishan/goconfig"
	"github.com/majiddarvishan/goconfig/httpserver"
)

const configJSON = `{"users": [{"name": "alice", "age": 30}]}`
const schemaJSON = `{
	"type": "object",
	"properties": {
		"users": {
			"type": "array",
			"items": {
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"age": {"type": "integer"}
				},
				"required": ["name", "age"]
			}
		}
	},
	"required": ["users"]
}`

func main() {
	hs, err := httpserver.NewServer("localhost", 8084, "secret", "/api/v1")
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	// A custom route - nothing to do with goconfig, still protected by apiKey.
	if err := hs.AddRoute("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	}, "GET"); err != nil {
		log.Fatalf("failed to add /health: %v", err)
	}

	// Wire in the manager's own routes only now, if/when needed.
	source, err := goconfig.NewStrSource(configJSON, schemaJSON)
	if err != nil {
		log.Fatalf("failed to create source: %v", err)
	}
	manager, err := goconfig.NewManager(source)
	if err != nil {
		log.Fatalf("failed to create manager: %v", err)
	}
	usersNode, err := manager.Config().At("users")
	if err != nil {
		log.Fatalf("users node not found: %v", err)
	}
	if err := manager.OnInsert(usersNode, nil); err != nil {
		log.Fatalf("failed to register OnInsert: %v", err)
	}

	if err := hs.AddRoutes(manager.GetRoutes()); err != nil {
		log.Fatalf("failed to add manager routes: %v", err)
	}

	go func() {
		if err := hs.Start(); err != nil {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := hs.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown failed: %v", err)
	}
}
