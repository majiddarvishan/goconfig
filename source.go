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

	// setConfig updates the configuration atomically. `data` is the
	// already-JSON-marshaled form of conf (Manager marshals once and
	// reuses it for both schema validation and this call, rather than
	// marshaling the same object twice) - implementations should persist
	// `data` directly rather than re-marshaling `conf`.
	// Must persist the configuration; returns error if persistence fails.
	setConfig(conf *orderedmap.OrderedMap, data []byte) error
}
