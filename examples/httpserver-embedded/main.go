// Example: the app owns its web-service and it knows nothing about
// goconfig - newAppWebService below has no goconfig import, no manager
// parameter, nothing. Wiring goconfig's admin routes into that web-service
// is a separate, optional step taken later in main, only if needed.
// goconfig never starts a server of its own in this mode.
// Try it with:
//
//	curl http://localhost:8082/ping
//	curl http://localhost:8082/health
//	curl -H "X-API-Key: secret" http://localhost:8082/config
package main

import (
	"log"
	"net/http"

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

// serveMuxRegistrar adapts *http.ServeMux to httpserver.RouteRegistrar.
// Any router (gorilla/mux, chi, ...) can be wrapped the same way.
type serveMuxRegistrar struct {
	*http.ServeMux
}

func (r serveMuxRegistrar) HandleFunc(path string, h http.HandlerFunc, _ ...string) {
	r.ServeMux.HandleFunc(path, h)
}

// newAppWebService builds the application's own web-service. This is
// ordinary app code: it has no dependency on goconfig whatsoever.
func newAppWebService() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})
	return mux
}

func main() {
	mux := newAppWebService()

	// Only if/when config management needs to be exposed, wire goconfig's
	// routes into the existing web-service. Nothing above this line needed
	// to know that goconfig exists.
	const enableConfigAPI = true
	if enableConfigAPI {
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

		hs, err := httpserver.NewHttpServer(manager, httpserver.WithAPIKey("secret"))
		if err != nil {
			log.Fatalf("failed to create http server: %v", err)
		}

		if err := hs.RegisterRoutes(serveMuxRegistrar{mux}); err != nil {
			log.Fatalf("failed to register goconfig routes: %v", err)
		}
	}

	log.Println("listening on :8082")
	if err := http.ListenAndServe(":8082", mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
