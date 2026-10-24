package profiling

import (
	"strings"

	"github.com/cloudwego/hertz/pkg/route"
	hertzpprof "github.com/hertz-contrib/pprof"
)

func Attach(group *route.RouterGroup, prefix string) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "/debug/pprof"
	}
	hertzpprof.RouteRegister(group, strings.TrimPrefix(prefix, "/ops"))
}
