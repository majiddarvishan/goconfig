package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/majiddarvishan/goconfig"
)

func main() {
	validator := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.Header.Get("X-Validator-Token") != "example-token" {
			http.Error(response, "unauthorized", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"valid":true}`))
	}))
	defer validator.Close()

	source, err := goconfig.NewStrSource(
		`{"release":"candidate"}`,
		`{"type":"object","required":["release"],"properties":{"release":{"type":"string"}}}`,
	)
	if err != nil {
		log.Fatal(err)
	}
	manager, err := goconfig.NewManager(source)
	if err != nil {
		log.Fatal(err)
	}
	release, err := manager.Lookup("/release")
	if err != nil {
		log.Fatal(err)
	}
	if err := manager.OnReplace(release.Node, nil); err != nil {
		log.Fatal(err)
	}

	service := goconfig.NewValidationService(validator.URL, 2*time.Second)
	service.SetHeader("X-Validator-Token", "example-token")
	manager.SetValidationService(service)
	if err := manager.Replace("/release", "stable"); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("external validation accepted version %d\n", manager.Version())
}
