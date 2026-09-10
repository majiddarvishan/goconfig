package goconfig

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/iancoleman/orderedmap"
)

const maxRouteBodySize = 10 * 1024 * 1024 // 10MB

// Route describes a single HTTP route for the config admin API as plain
// data, decoupled from any specific HTTP server implementation. Manager only
// depends on the standard library's http.HandlerFunc here, never on any
// particular server/router - the caller wires these onto whatever it likes.
type Route struct {
	Path    string
	Methods []string
	Handler http.HandlerFunc
}

// GetRoutes returns the config admin API as routes: GET fetches the current
// config/schema/modifiable paths, POST performs insert/remove/replace with
// optimistic-locking via "version". Authentication and CORS are transport
// concerns left to whatever serves these routes.
func (m *Manager) GetRoutes() []Route {
	return []Route{
		{Path: "/config", Methods: []string{"GET", "POST", "OPTIONS"}, Handler: m.serveConfigRoute},
	}
}

func (m *Manager) serveConfigRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		m.serveConfigGet(w, r)
	case http.MethodPost:
		m.serveConfigPost(w, r)
	case http.MethodOptions:
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Origin, Content-Type, X-API-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusOK)
	default:
		writeRouteError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (m *Manager) serveConfigGet(w http.ResponseWriter, _ *http.Request) {
	data, err := m.buildConfigState()
	if err != nil {
		writeRouteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build config: %s", err))
		return
	}
	writeRouteSuccess(w, data)
}

func (m *Manager) serveConfigPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRouteBodySize)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeRouteError(w, http.StatusBadRequest, fmt.Sprintf("could not read body: %s", err))
		return
	}
	if len(body) == 0 {
		writeRouteError(w, http.StatusBadRequest, "request body is empty")
		return
	}

	bodyJSON := orderedmap.New()
	if err := json.Unmarshal(body, &bodyJSON); err != nil {
		writeRouteError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %s", err))
		return
	}

	op, err := getRouteString(bodyJSON, "op")
	if err != nil {
		writeRouteError(w, http.StatusBadRequest, err.Error())
		return
	}

	path, err := getRouteString(bodyJSON, "path")
	if err != nil {
		writeRouteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if path == "" || path[0] != '/' {
		writeRouteError(w, http.StatusBadRequest, "path must start with '/'")
		return
	}

	value, hasValue := bodyJSON.Get("value")

	// Version-based optimistic locking (better than hash)
	var expectedVersion int64
	if versionVal, ok := bodyJSON.Get("version"); ok {
		versionFloat, ok := versionVal.(float64)
		if !ok {
			writeRouteError(w, http.StatusBadRequest, "version must be a number")
			return
		}
		expectedVersion = int64(versionFloat)

		if current := m.Version(); current != expectedVersion {
			writeRouteError(w, http.StatusConflict,
				fmt.Sprintf("version mismatch: expected %d, current %d", expectedVersion, current))
			return
		}
	}

	switch op {
	case "insert":
		if !hasValue {
			writeRouteError(w, http.StatusBadRequest, "value is required for insert")
			return
		}
		index, err := getRouteIndex(bodyJSON)
		if err != nil {
			writeRouteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := m.Insert(path, index, value); err != nil {
			writeRouteError(w, http.StatusBadRequest, err.Error())
			return
		}

	case "remove":
		index, err := getRouteIndex(bodyJSON)
		if err != nil {
			writeRouteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := m.Remove(path, index); err != nil {
			writeRouteError(w, http.StatusBadRequest, err.Error())
			return
		}

	case "replace":
		if !hasValue {
			writeRouteError(w, http.StatusBadRequest, "value is required for replace")
			return
		}
		if err := m.Replace(path, value); err != nil {
			writeRouteError(w, http.StatusBadRequest, err.Error())
			return
		}

	default:
		writeRouteError(w, http.StatusBadRequest, fmt.Sprintf("unsupported operation: %s", op))
		return
	}

	data, err := m.buildConfigState()
	if err != nil {
		writeRouteError(w, http.StatusInternalServerError, fmt.Sprintf("failed to build config: %s", err))
		return
	}
	writeRouteSuccess(w, data)
}

func (m *Manager) buildConfigState() (*orderedmap.OrderedMap, error) {
	confJSON := orderedmap.New()
	schemaJSON := orderedmap.New()

	configStr := m.ConfigJSON()
	if configStr == "" {
		return nil, fmt.Errorf("config is empty")
	}
	if err := json.Unmarshal([]byte(configStr), &confJSON); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	if schemaStr := m.SchemaJSON(); schemaStr != "" {
		if err := json.Unmarshal([]byte(schemaStr), &schemaJSON); err != nil {
			return nil, fmt.Errorf("failed to unmarshal schema: %w", err)
		}
	}

	paths := orderedmap.New()
	paths.Set("insertable", m.InsertablePaths())
	paths.Set("removable", m.RemovablePaths())
	paths.Set("replaceable", m.ReplaceablePaths())

	out := orderedmap.New()
	out.Set("modifiable_paths", paths)
	out.Set("config", confJSON)
	out.Set("schema", schemaJSON)
	out.Set("version", m.Version())

	return out, nil
}

func writeRouteError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	errObj := orderedmap.New()
	errObj.Set("message", msg)
	errObj.Set("code", code)

	resp := orderedmap.New()
	resp.Set("success", false)
	resp.Set("error", errObj)

	out, _ := json.MarshalIndent(resp, "", "  ")
	w.Write(out)
}

func writeRouteSuccess(w http.ResponseWriter, data *orderedmap.OrderedMap) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	resp := orderedmap.New()
	resp.Set("success", true)
	resp.Set("data", data)

	out, _ := json.MarshalIndent(resp, "", "  ")
	w.Write(out)
}

func getRouteString(m *orderedmap.OrderedMap, key string) (string, error) {
	v, ok := m.Get(key)
	if !ok {
		return "", fmt.Errorf("'%s' is missing", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("'%s' must be a string", key)
	}
	if s == "" {
		return "", fmt.Errorf("'%s' cannot be empty", key)
	}
	return s, nil
}

func getRouteIndex(m *orderedmap.OrderedMap) (int, error) {
	val, ok := m.Get("index")
	if !ok {
		return 0, fmt.Errorf("'index' is missing")
	}
	f, ok := val.(float64)
	if !ok {
		return 0, fmt.Errorf("'index' must be a number")
	}
	if f < 0 {
		return 0, fmt.Errorf("'index' must be non-negative")
	}
	return int(f), nil
}
