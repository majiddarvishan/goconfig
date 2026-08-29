package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/majiddarvishan/goconfig"
)

func main() {
	directory, err := os.MkdirTemp("", "goconfig-file-source-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(directory)

	path := filepath.Join(directory, "config.json")
	if err := os.WriteFile(path, []byte(`{"environment":"development"}`), 0o600); err != nil {
		log.Fatal(err)
	}
	source, err := goconfig.NewFileSource(
		path,
		`{"type":"object","required":["environment"],"properties":{"environment":{"type":"string"}}}`,
	)
	if err != nil {
		log.Fatal(err)
	}
	manager, err := goconfig.NewManager(source)
	if err != nil {
		log.Fatal(err)
	}

	environment, err := manager.Lookup("/environment")
	if err != nil {
		log.Fatal(err)
	}
	if err := manager.OnReplace(environment.Node, nil); err != nil {
		log.Fatal(err)
	}
	if err := manager.Replace("/environment", "production"); err != nil {
		log.Fatal(err)
	}

	persisted, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(persisted))
}
