package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/majiddarvishan/goconfig"
)

func main() {
	source, err := goconfig.NewStrSource(
		`{"name":"demo"}`,
		`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`,
	)
	if err != nil {
		log.Fatal(err)
	}
	manager, err := goconfig.NewManager(source)
	if err != nil {
		log.Fatal(err)
	}
	name, err := manager.Lookup("/name")
	if err != nil {
		log.Fatal(err)
	}
	if err := manager.OnReplace(name.Node, nil); err != nil {
		log.Fatal(err)
	}
	server, err := goconfig.NewHTTPServer(manager, goconfig.WithAPIKey("example-secret"))
	if err != nil {
		log.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/config", bytes.NewBufferString(
		`{"op":"replace","path":"/name","value":"via-http","version":1}`,
	))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-Key", "example-secret")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	body, err := io.ReadAll(response.Result().Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("status=%d body=%s\n", response.Code, body)
}
