// Example: tracking config changes with Manager's built-in audit history.
package main

import (
	"fmt"
	"log"

	"github.com/majiddarvishan/goconfig"
)

const configJSON = `{
	"users": [
		{"name": "alice", "age": 30}
	],
	"settings": {
		"timeout": 30
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
				"timeout": {"type": "integer"}
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

	// History is on by default; EnableHistory(false) turns it off.
	manager.EnableHistory(true)

	usersNode, err := manager.Config().At("users")
	if err != nil {
		log.Fatalf("users node not found: %v", err)
	}
	if err := manager.OnInsert(usersNode, nil); err != nil {
		log.Fatalf("failed to register OnInsert: %v", err)
	}
	if err := manager.OnRemove(usersNode, nil); err != nil {
		log.Fatalf("failed to register OnRemove: %v", err)
	}

	timeoutNode, err := manager.Config().At("settings")
	if err != nil {
		log.Fatalf("settings node not found: %v", err)
	}
	timeoutNode, err = timeoutNode.At("timeout")
	if err != nil {
		log.Fatalf("timeout node not found: %v", err)
	}
	if err := manager.OnReplace(timeoutNode, nil); err != nil {
		log.Fatalf("failed to register OnReplace: %v", err)
	}

	// A few mutations across different paths.
	if err := manager.Insert("/users", 1, map[string]interface{}{"name": "bob", "age": 25}); err != nil {
		log.Fatalf("insert failed: %v", err)
	}
	if err := manager.Replace("/settings/timeout", 60); err != nil {
		log.Fatalf("replace failed: %v", err)
	}
	if err := manager.Remove("/users", 0); err != nil {
		log.Fatalf("remove failed: %v", err)
	}

	fmt.Println("full history:")
	for _, event := range manager.GetHistory() {
		fmt.Printf("  v%d %s %s at %s\n", event.Version, event.Operation, event.Path, event.Timestamp.Format("15:04:05"))
	}

	fmt.Println("history for /users only:")
	for _, event := range manager.GetHistoryByPath("/users", 10) {
		fmt.Printf("  v%d %s %s\n", event.Version, event.Operation, event.Path)
	}

	manager.ClearHistory()
	fmt.Printf("history after ClearHistory: %d events\n", len(manager.GetHistory()))
}
