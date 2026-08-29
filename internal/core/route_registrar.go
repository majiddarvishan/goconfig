package goconfig

import "net/http"

// RouteRegistrar allows goconfig to register routes
// without knowing the underlying router/framework.
type RouteRegistrar interface {
	HandleFunc(path string, handler http.HandlerFunc, methods ...string)
}
