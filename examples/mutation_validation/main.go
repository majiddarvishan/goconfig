package main

import (
	"fmt"
	"log"

	"github.com/majiddarvishan/goconfig"
)

func main() {
	source, err := goconfig.NewStrSource(
		`{"port":8080,"hosts":["api-1"]}`,
		`{"type":"object","required":["port","hosts"],"properties":{"port":{"type":"integer"},"hosts":{"type":"array","items":{"type":"string"}}}}`,
	)
	if err != nil {
		log.Fatal(err)
	}
	manager, err := goconfig.NewManager(source)
	if err != nil {
		log.Fatal(err)
	}

	port, err := manager.Lookup("/port")
	if err != nil {
		log.Fatal(err)
	}
	hosts, err := manager.Lookup("/hosts")
	if err != nil {
		log.Fatal(err)
	}
	if err := manager.OnReplace(port.Node, nil); err != nil {
		log.Fatal(err)
	}
	if err := manager.OnInsert(hosts.Node, nil); err != nil {
		log.Fatal(err)
	}
	if err := manager.RegisterValidator("/port", goconfig.ValidateRange(1, 65535)); err != nil {
		log.Fatal(err)
	}

	if err := manager.Replace("/port", 9090); err != nil {
		log.Fatal(err)
	}
	if err := manager.Insert("/hosts", 1, "api-2"); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("version after valid mutations: %d\n", manager.Version())

	if err := manager.Replace("/port", 70000); err != nil {
		fmt.Printf("rejected invalid port: %v\n", err)
	}
}
