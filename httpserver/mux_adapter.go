package httpserver

import "net/http"

type muxAdapter struct {
	*http.ServeMux
}

func (m muxAdapter) HandleFunc(path string, h http.HandlerFunc, _ ...string) {
	m.ServeMux.HandleFunc(path, h)
}
