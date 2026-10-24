package core

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type ModuleDescriptor struct {
	Name   string `json:"name"`
	Prefix string `json:"prefix"`
}

type ModuleCatalog interface {
	ListModules() []ModuleDescriptor
}

type Module interface {
	Descriptor() ModuleDescriptor
	Mount(router route.IRouter)
}

type ShutdownHook interface {
	Shutdown(ctx context.Context) error
}

type MiddlewareProvider interface {
	Middlewares() []app.HandlerFunc
}
