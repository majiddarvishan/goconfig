// Example: using goconfig.Manager standalone, with no HTTP involved at all.
package main

import (
	"fmt"
	"log"

	"github.com/majiddarvishan/goconfig"
)

const configJSON = `{
	"users": [
		{"name": "alice", "age": 30},
		{"name": "bob", "age": 25}
	],
	"settings": {
		"port": 8080
	}
}`

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
		},
		"settings": {
			"type": "object",
			"properties": {
				"port": {"type": "integer"}
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

	manager.EnableHistory(true)

	// Register which nodes may be mutated, and a handler that runs on every mutation.
	usersNode, err := manager.Config().At("users")
	if err != nil {
		log.Fatalf("users node not found: %v", err)
	}
	if err := manager.OnInsert(usersNode, func(n *goconfig.Node) error {
		name, _ := n.GetString("name")
		fmt.Printf("user inserted: %s\n", name)
		return nil
	}); err != nil {
		log.Fatalf("failed to register OnInsert: %v", err)
	}

	portNode, err := manager.Config().At("settings")
	if err != nil {
		log.Fatalf("settings node not found: %v", err)
	}
	portNode, err = portNode.At("port")
	if err != nil {
		log.Fatalf("port node not found: %v", err)
	}
	if err := manager.OnReplace(portNode, func(n *goconfig.Node) error {
		port, _ := n.GetInt()
		fmt.Printf("port replaced: %d\n", port)
		return nil
	}); err != nil {
		log.Fatalf("failed to register OnReplace: %v", err)
	}

	// Mutate config directly through Manager - no HTTP server required.
	if err := manager.Insert("/users", 2, map[string]interface{}{"name": "carol", "age": 22}); err != nil {
		log.Fatalf("insert failed: %v", err)
	}
	if err := manager.Replace("/settings/port", 9090); err != nil {
		log.Fatalf("replace failed: %v", err)
	}

	// Query the config with the built-in JSONPath-like DSL.
	results, err := manager.Query("/users/[?age>=25]")
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}
	fmt.Printf("users with age >= 25: %d\n", len(results))

	// Inspect the change history.
	for _, event := range manager.GetHistory() {
		fmt.Printf("history: %s %s (v%d)\n", event.Operation, event.Path, event.Version)
	}

	fmt.Printf("current config: %s\n", manager.ConfigJSON())
}
