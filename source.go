package goconfig

import "github.com/iancoleman/orderedmap"

// ISource defines the interface for configuration sources
// Implementations should be thread-safe
type ISource interface {
	// getConfigObject returns the internal OrderedMap representation
	// Implementations should protect concurrent access
	getConfigObject() *orderedmap.OrderedMap

	// getConfig returns the JSON string representation
	// The returned pointer should not be mutated
	getConfig() *string

	// getSchema returns the JSON schema string
	// The returned pointer should not be mutated
	getSchema() *string

	// setConfig updates the configuration atomically
	// Must validate and persist the configuration
	// Returns error if validation or persistence fails
	setConfig(*orderedmap.OrderedMap) error
}
