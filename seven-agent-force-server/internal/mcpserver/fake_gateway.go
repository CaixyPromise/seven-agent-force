package mcpserver

import "context"

// FakeGateway is an in-memory WorkforceGateway seam for protocol and client tests.
type FakeGateway struct {
	ListModelOptionsFunc   func(context.Context, string, int64) (ModelOptions, error)
	ListRuntimeOptionsFunc func(context.Context, string, int64, int64) (RuntimeOptions, error)

	GetCapabilitiesFunc func(context.Context, string) (Capabilities, error)
	CreateTaskFunc      func(context.Context, string, CreateTaskRequest) (Task, error)
	GetTaskFunc         func(context.Context, string, int64) (Task, error)
	ListOutputsFunc     func(context.Context, string, int64) ([]Output, error)
	OpenOutputFunc      func(context.Context, string, int64, int64) (OutputStream, error)
}

func (f *FakeGateway) GetCapabilities(ctx context.Context, key string) (Capabilities, error) {
	return f.GetCapabilitiesFunc(ctx, key)
}
func (f *FakeGateway) CreateTask(ctx context.Context, key string, request CreateTaskRequest) (Task, error) {
	return f.CreateTaskFunc(ctx, key, request)
}
func (f *FakeGateway) GetTask(ctx context.Context, key string, taskID int64) (Task, error) {
	return f.GetTaskFunc(ctx, key, taskID)
}
func (f *FakeGateway) ListOutputs(ctx context.Context, key string, taskID int64) ([]Output, error) {
	return f.ListOutputsFunc(ctx, key, taskID)
}
func (f *FakeGateway) OpenOutput(ctx context.Context, key string, taskID, outputID int64) (OutputStream, error) {
	return f.OpenOutputFunc(ctx, key, taskID, outputID)
}

func (f *FakeGateway) ListModelOptions(ctx context.Context, key string, projectID int64) (ModelOptions, error) {
	return f.ListModelOptionsFunc(ctx, key, projectID)
}
func (f *FakeGateway) ListRuntimeOptions(ctx context.Context, key string, projectID, credentialID int64) (RuntimeOptions, error) {
	return f.ListRuntimeOptionsFunc(ctx, key, projectID, credentialID)
}
