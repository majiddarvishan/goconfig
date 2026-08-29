package goconfig

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileSourcePreCommitFailurePointsLeaveStateUnchanged(t *testing.T) {
	tests := []struct {
		name   string
		inject func(*FileSource)
	}{
		{
			name: "create temp",
			inject: func(source *FileSource) {
				source.ops.createTemp = func(string, string) (*os.File, error) {
					return nil, errors.New("injected create failure")
				}
			},
		},
		{
			name: "file sync",
			inject: func(source *FileSource) {
				source.ops.fileSync = func(*os.File) error { return errors.New("injected file sync failure") }
			},
		},
		{
			name: "open directory",
			inject: func(source *FileSource) {
				source.ops.open = func(string) (*os.File, error) { return nil, errors.New("injected open failure") }
			},
		},
		{
			name: "rename",
			inject: func(source *FileSource) {
				source.ops.rename = func(string, string) error { return errors.New("injected rename failure") }
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "config.json")
			if err := os.WriteFile(path, []byte(testConfigJSON), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			source, err := NewFileSource(path, testSchemaJSON)
			if err != nil {
				t.Fatalf("NewFileSource() error = %v", err)
			}
			test.inject(source)
			updated, err := Clone(source.getConfigObject())
			if err != nil {
				t.Fatalf("Clone() error = %v", err)
			}
			updated.Set("name", "should-not-commit")
			if err := source.setConfig(updated); err == nil {
				t.Fatal("setConfig() error = nil, want injected failure")
			}

			disk, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			if string(disk) != testConfigJSON || *source.getConfig() != testConfigJSON {
				t.Fatalf("state changed: disk=%s memory=%s", disk, *source.getConfig())
			}
			temporary, err := filepath.Glob(filepath.Join(directory, ".config.json.tmp-*"))
			if err != nil || len(temporary) != 0 {
				t.Fatalf("temporary files = %v, error = %v", temporary, err)
			}
		})
	}
}

func TestExternalValidationFailureModesRollbackCandidate(t *testing.T) {
	tests := []struct {
		name      string
		transport roundTripFunc
	}{
		{
			name: "transport",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("injected transport failure")
			},
		},
		{
			name: "non ok status",
			transport: func(request *http.Request) (*http.Response, error) {
				return validationHTTPResponse(request, http.StatusBadGateway, `upstream failure`), nil
			},
		},
		{
			name: "malformed response",
			transport: func(request *http.Request) (*http.Response, error) {
				return validationHTTPResponse(request, http.StatusOK, `{`), nil
			},
		},
		{
			name: "explicit rejection",
			transport: func(request *http.Request) (*http.Response, error) {
				return validationHTTPResponse(request, http.StatusOK, `{"valid":false,"errors":["rejected"]}`), nil
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, _ := newTestManager(t)
			name := mustNodeAt(t, manager.Config(), "name")
			if err := manager.OnReplace(name, nil); err != nil {
				t.Fatalf("OnReplace() error = %v", err)
			}
			service := NewValidationService("http://validator.test/validate", time.Second)
			service.client = &http.Client{Transport: test.transport}
			manager.SetValidationService(service)

			err := manager.Replace("/name", "not-committed")
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("Replace() error = %v, want ErrValidation", err)
			}
			if manager.Version() != 1 || len(manager.History()) != 0 {
				t.Fatalf("version/history = %d/%d, want 1/0", manager.Version(), len(manager.History()))
			}
			if got, _ := mustNodeAt(t, manager.Config(), "name").GetString(); got != "demo" {
				t.Fatalf("name = %q, want demo", got)
			}
		})
	}
}

func validationHTTPResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}
