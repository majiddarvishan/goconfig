// Package goconfig provides transactional, schema-validated JSON configuration.
//
// The public package is a stable facade over the implementation in
// internal/core. Type aliases preserve method sets and type identity for v1
// consumers while allowing implementation files to remain grouped together.
package goconfig

import (
	"net/http"
	"time"

	"github.com/iancoleman/orderedmap"
	core "github.com/majiddarvishan/goconfig/internal/core"
)

const (
	DefaultHistoryCapacity = core.DefaultHistoryCapacity

	Null          = core.Null
	Boolean       = core.Boolean
	Integral      = core.Integral
	FloatingPoint = core.FloatingPoint
	String        = core.String
	Object        = core.Object
	Array         = core.Array

	Insertable  = core.Insertable
	Removable   = core.Removable
	Replaceable = core.Replaceable

	OperationInsert  = core.OperationInsert
	OperationRemove  = core.OperationRemove
	OperationReplace = core.OperationReplace
)

var (
	ErrInvalidPath     = core.ErrInvalidPath
	ErrPathNotFound    = core.ErrPathNotFound
	ErrTypeMismatch    = core.ErrTypeMismatch
	ErrIndexOutOfRange = core.ErrIndexOutOfRange
	ErrVersionConflict = core.ErrVersionConflict
	ErrValidation      = core.ErrValidation
	ErrPersistence     = core.ErrPersistence
	ErrInvalidQuery    = core.ErrInvalidQuery
	ErrQueryNoResults  = core.ErrQueryNoResults
	ErrInvalidMutation = core.ErrInvalidMutation
	ErrMutationDenied  = core.ErrMutationDenied

	ErrHTTPServerNotConfigured = core.ErrHTTPServerNotConfigured
	ErrHTTPServerStarted       = core.ErrHTTPServerStarted
)

type (
	Authenticator        = core.Authenticator
	CORSConfig           = core.CORSConfig
	Change               = core.Change
	ChangeHandler        = core.ChangeHandler
	ChangeObserver       = core.ChangeObserver
	CustomValidator      = core.CustomValidator
	FileSource           = core.FileSource
	HealthCheck          = core.HealthCheck
	HTTPLogger           = core.HTTPLogger
	HTTPServer           = core.HTTPServer
	HttpServer           = core.HttpServer
	HTTPServerOption     = core.HTTPServerOption
	HttpServerOption     = core.HttpServerOption
	HTTPTimeouts         = core.HTTPTimeouts
	ISource              = core.ISource
	Manager              = core.Manager
	ManagerOption        = core.ManagerOption
	ModificationType     = core.ModificationType
	Mutation             = core.Mutation
	Node                 = core.Node
	NodeType             = core.NodeType
	Operation            = core.Operation
	PathError            = core.PathError
	PersistenceError     = core.PersistenceError
	QueryError           = core.QueryError
	QueryResult          = core.QueryResult
	RouteRegistrar       = core.RouteRegistrar
	Snapshot             = core.Snapshot
	Source               = core.Source
	SourceData           = core.SourceData
	StrSource            = core.StrSource
	ValidationError      = core.ValidationError
	ValidationRequest    = core.ValidationRequest
	ValidationResponse   = core.ValidationResponse
	ValidationService    = core.ValidationService
	Validator            = core.Validator
	VersionConflictError = core.VersionConflictError
)

func NewManager(source ISource) (*Manager, error) { return core.NewManager(source) }

func NewManagerWithOptions(source ISource, options ...ManagerOption) (*Manager, error) {
	return core.NewManagerWithOptions(source, options...)
}

func NewManagerFromSource(source Source) (*Manager, error) {
	return core.NewManagerFromSource(source)
}

func NewManagerFromSourceWithOptions(source Source, options ...ManagerOption) (*Manager, error) {
	return core.NewManagerFromSourceWithOptions(source, options...)
}

func WithHistoryCapacity(capacity int) ManagerOption { return core.WithHistoryCapacity(capacity) }

func NewStrSource(config, schema string) (*StrSource, error) {
	return core.NewStrSource(config, schema)
}

func NewFileSource(configPath, schema string) (*FileSource, error) {
	return core.NewFileSource(configPath, schema)
}

func NewHTTPServer(manager *Manager, options ...HTTPServerOption) (*HTTPServer, error) {
	return core.NewHTTPServer(manager, options...)
}

func WithAddress(address string) HTTPServerOption { return core.WithAddress(address) }
func WithPort(port int) HTTPServerOption          { return core.WithPort(port) }
func WithAPIKey(apiKey string) HTTPServerOption   { return core.WithAPIKey(apiKey) }
func WithAuthenticator(authenticator Authenticator) HTTPServerOption {
	return core.WithAuthenticator(authenticator)
}
func WithRouteRegistrar(registrar RouteRegistrar) HTTPServerOption {
	return core.WithRouteRegistrar(registrar)
}
func WithServer(server *http.Server) HTTPServerOption { return core.WithServer(server) }
func WithCORS(config CORSConfig) HTTPServerOption     { return core.WithCORS(config) }
func WithLogger(logger HTTPLogger) HTTPServerOption   { return core.WithLogger(logger) }
func WithHTTPTimeouts(timeouts HTTPTimeouts) HTTPServerOption {
	return core.WithHTTPTimeouts(timeouts)
}
func WithMaxBodySize(size int64) HTTPServerOption { return core.WithMaxBodySize(size) }
func WithHealthCheck(check HealthCheck) HTTPServerOption {
	return core.WithHealthCheck(check)
}
func WithHealthEnabled(enabled bool) HTTPServerOption { return core.WithHealthEnabled(enabled) }

func NewValidationService(url string, timeout time.Duration) *ValidationService {
	return core.NewValidationService(url, timeout)
}

// Deprecated: use NewValidationService.
func NewvalidationService(url string, timeout time.Duration) *ValidationService {
	return core.NewvalidationService(url, timeout)
}

func NewCustomValidator() *CustomValidator             { return core.NewCustomValidator() }
func ValidateRange(min, max float64) Validator         { return core.ValidateRange(min, max) }
func ValidatePattern(pattern string) Validator         { return core.ValidatePattern(pattern) }
func ValidateRegexp(pattern string) (Validator, error) { return core.ValidateRegexp(pattern) }
func ValidateEnum(allowed ...interface{}) Validator    { return core.ValidateEnum(allowed...) }
func ValidateRequired() Validator                      { return core.ValidateRequired() }
func ValidateUnique(field string) Validator            { return core.ValidateUnique(field) }

func Clone(source *orderedmap.OrderedMap) (*orderedmap.OrderedMap, error) {
	return core.Clone(source)
}

func HashSHA256(value string) string { return core.HashSHA256(value) }
