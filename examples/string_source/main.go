package main

import (
	"fmt"
	"log"

	"github.com/majiddarvishan/goconfig"
)

func main() {
	source, err := goconfig.NewStrSource(
		`{"name":"demo","port":8080}`,
		`{"type":"object","required":["name","port"],"properties":{"name":{"type":"string"},"port":{"type":"integer"}}}`,
	)
	if err != nil {
		log.Fatal(err)
	}
	manager, err := goconfig.NewManager(source)
	if err != nil {
		log.Fatal(err)
	}

	name, err := manager.Config().GetString("name")
	if err != nil {
		log.Fatal(err)
	}
	port, err := manager.Config().GetInt("port")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s listens on %d\n", name, port)
}
