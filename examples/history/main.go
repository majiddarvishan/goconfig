package main

import (
	"fmt"
	"log"

	"github.com/majiddarvishan/goconfig"
)

func main() {
	source, err := goconfig.NewStrSource(
		`{"mode":"development"}`,
		`{"type":"object","required":["mode"],"properties":{"mode":{"type":"string"}}}`,
	)
	if err != nil {
		log.Fatal(err)
	}
	manager, err := goconfig.NewManagerWithOptions(source, goconfig.WithHistoryCapacity(10))
	if err != nil {
		log.Fatal(err)
	}
	mode, err := manager.Lookup("/mode")
	if err != nil {
		log.Fatal(err)
	}
	if err := manager.OnReplace(mode.Node, nil); err != nil {
		log.Fatal(err)
	}

	for _, value := range []string{"staging", "production"} {
		if err := manager.Replace("/mode", value); err != nil {
			log.Fatal(err)
		}
	}
	for _, event := range manager.HistoryByPath("/mode", 10) {
		fmt.Printf("v%d %s %s\n", event.Version, event.Operation, event.Path)
	}
}
