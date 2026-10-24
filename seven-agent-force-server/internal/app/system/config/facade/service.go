package facade

import "context"

import authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"

type ConfigFacade interface {
	// Deprecated: unscoped reads are rejected; use GetConfigForConsumer.
	GetConfigByKey(ctx context.Context, configKey string) (*ConfigValueDTO, error)
	// Deprecated: unscoped reads are rejected; use GetConfigForConsumer.
	GetConfigBatch(ctx context.Context, request ConfigBatchRequest) (map[string]ConfigValueDTO, error)
	BindConfigConsumers(registrations []ConfigConsumerRegistration)
	// RegisterConfigConsumers adds or refreshes exact internal consumer
	// declarations without replacing registrations owned by other modules.
	// Workforce uses it when an operator binds a Result Processor SECRET ref.
	RegisterConfigConsumers(registrations []ConfigConsumerRegistration)
	GetConfigForConsumer(ctx context.Context, request ConfigInternalReadRequest) (*ConfigValueDTO, error)
	GetConfigBatchForConsumer(ctx context.Context, request ConfigInternalBatchReadRequest) (map[string]ConfigValueDTO, error)
	ListConfigsForConsumer(ctx context.Context, request ConfigInternalListRequest) (map[string]ConfigValueDTO, error)
	ListConfigConsumers(ctx context.Context) []ConfigConsumerVO
	authorizationfacade.RoleGrantConfigScopePort
}
