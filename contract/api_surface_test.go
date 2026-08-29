package contract_test

import "github.com/majiddarvishan/goconfig"

// These compile-time references protect the facade from accidentally dropping
// a public v1 symbol when internal files are reorganized.
var (
	_ = goconfig.DefaultHistoryCapacity
	_ = goconfig.Null
	_ = goconfig.Boolean
	_ = goconfig.Integral
	_ = goconfig.FloatingPoint
	_ = goconfig.String
	_ = goconfig.Object
	_ = goconfig.Array
	_ = goconfig.Insertable
	_ = goconfig.Removable
	_ = goconfig.Replaceable
	_ = goconfig.OperationInsert
	_ = goconfig.OperationRemove
	_ = goconfig.OperationReplace

	_ = goconfig.ErrInvalidPath
	_ = goconfig.ErrPathNotFound
	_ = goconfig.ErrTypeMismatch
	_ = goconfig.ErrIndexOutOfRange
	_ = goconfig.ErrVersionConflict
	_ = goconfig.ErrValidation
	_ = goconfig.ErrPersistence
	_ = goconfig.ErrInvalidQuery
	_ = goconfig.ErrQueryNoResults
	_ = goconfig.ErrInvalidMutation
	_ = goconfig.ErrMutationDenied
	_ = goconfig.ErrHTTPServerNotConfigured
	_ = goconfig.ErrHTTPServerStarted

	_ = goconfig.NewManager
	_ = goconfig.NewManagerWithOptions
	_ = goconfig.NewManagerFromSource
	_ = goconfig.NewManagerFromSourceWithOptions
	_ = goconfig.WithHistoryCapacity
	_ = goconfig.NewStrSource
	_ = goconfig.NewFileSource
	_ = goconfig.NewHTTPServer
	_ = goconfig.WithAddress
	_ = goconfig.WithPort
	_ = goconfig.WithAPIKey
	_ = goconfig.WithAuthenticator
	_ = goconfig.WithRouteRegistrar
	_ = goconfig.WithServer
	_ = goconfig.WithCORS
	_ = goconfig.WithLogger
	_ = goconfig.WithHTTPTimeouts
	_ = goconfig.WithMaxBodySize
	_ = goconfig.WithHealthCheck
	_ = goconfig.WithHealthEnabled
	_ = goconfig.NewValidationService
	_ = goconfig.NewvalidationService
	_ = goconfig.NewCustomValidator
	_ = goconfig.ValidateRange
	_ = goconfig.ValidatePattern
	_ = goconfig.ValidateRegexp
	_ = goconfig.ValidateEnum
	_ = goconfig.ValidateRequired
	_ = goconfig.ValidateUnique
	_ = goconfig.Clone
	_ = goconfig.HashSHA256
)

type facadeTypes struct {
	authenticator      goconfig.Authenticator
	cors               goconfig.CORSConfig
	change             goconfig.Change
	changeHandler      goconfig.ChangeHandler
	changeObserver     goconfig.ChangeObserver
	customValidator    *goconfig.CustomValidator
	fileSource         *goconfig.FileSource
	healthCheck        goconfig.HealthCheck
	logger             goconfig.HTTPLogger
	httpServer         *goconfig.HTTPServer
	httpServerLegacy   *goconfig.HttpServer
	httpOption         goconfig.HTTPServerOption
	httpOptionLegacy   goconfig.HttpServerOption
	timeouts           goconfig.HTTPTimeouts
	legacySource       goconfig.ISource
	manager            *goconfig.Manager
	managerOption      goconfig.ManagerOption
	modification       goconfig.ModificationType
	mutation           goconfig.Mutation
	node               *goconfig.Node
	nodeType           goconfig.NodeType
	operation          goconfig.Operation
	pathError          *goconfig.PathError
	persistenceError   *goconfig.PersistenceError
	queryError         *goconfig.QueryError
	queryResult        goconfig.QueryResult
	routeRegistrar     goconfig.RouteRegistrar
	snapshot           goconfig.Snapshot
	source             goconfig.Source
	sourceData         goconfig.SourceData
	stringSource       *goconfig.StrSource
	validationError    *goconfig.ValidationError
	validationRequest  goconfig.ValidationRequest
	validationResponse goconfig.ValidationResponse
	validationService  *goconfig.ValidationService
	validator          goconfig.Validator
	versionConflict    *goconfig.VersionConflictError
}
