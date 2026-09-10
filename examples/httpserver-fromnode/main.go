// Example: building the HttpServer from a *goconfig.Node instead of functional
// options - useful when the server's own address/port/api_key should come
// from the same config document the manager already manages.
// Try it with:
//
//	curl http://localhost:8083/health
//	curl -H "X-API-Key: secret" http://localhost:8083/config
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/majiddarvishan/goconfig"
	"github.com/majiddarvishan/goconfig/httpserver"
)

const configJSON = `{
	"http_server": {
		"address": "localhost",
		"port": 8083,
		"api_key": "secret"
	},
	"users": [
		{"name": "alice", "age": 30}
	]
}`

const schemaJSON = `{
	"type": "object",
	"properties": {
		"http_server": {
			"type": "object",
			"properties": {
				"address": {"type": "string"},
				"port": {"type": "integer"},
				"api_key": {"type": "string"}
			}
		},
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

	// The server's address/port/api_key are read straight from the managed
	// config tree instead of being passed as Go code (WithAddress/WithPort/...).
	httpServerConf, err := manager.Config().At("http_server")
	if err != nil {
		log.Fatalf("http_server node not found: %v", err)
	}

	hs, err := httpserver.NewHttpServerFromNode(manager, httpServerConf)
	if err != nil {
		log.Fatalf("failed to create http server: %v", err)
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
