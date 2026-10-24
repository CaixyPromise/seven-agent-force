package facade

import "context"

type DictFacade interface {
	BatchGetDict(ctx context.Context, request DictBatchRequest) (*DictBatchResponse, error)
	GetDictByCode(ctx context.Context, dictCode string) (*DictBatchResponse, error)
	BindDictConsumers(registrations []DictConsumerRegistration)
	GetDictForConsumer(ctx context.Context, request DictInternalReadRequest) (*DictBatchResponse, error)
	BatchGetDictForConsumer(ctx context.Context, request DictInternalBatchReadRequest) (*DictBatchResponse, error)
	ListDictsForConsumer(ctx context.Context, request DictInternalListRequest) (*DictBatchResponse, error)
	ListDictConsumers(ctx context.Context) []DictConsumerVO
}
