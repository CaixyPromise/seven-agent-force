package controller

import (
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/route"
)

func TestMountExposesProcessorAuthoringWithoutTemplateAPI(t *testing.T) {
	engine := route.NewEngine(config.NewOptions(nil))
	New(nil).Mount(engine, func(_ string, handler app.HandlerFunc) app.HandlerFunc { return handler })

	foundProcessors := false
	for _, item := range engine.Routes() {
		if strings.Contains(item.Path, "result-processor-templates") {
			t.Fatalf("removed Result Processor Template route is still mounted: %s %s", item.Method, item.Path)
		}
		if item.Method == "POST" && item.Path == "/workforce/result-processors" {
			foundProcessors = true
		}
	}
	if !foundProcessors {
		t.Fatal("direct Result Processor draft creation route is missing")
	}
}
