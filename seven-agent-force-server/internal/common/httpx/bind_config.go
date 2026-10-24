package httpx

import (
	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
	"github.com/cloudwego/hertz/pkg/app/server/binding"
)

func init() {
	newConfiguredBindConfig()
}

// NewBindConfig returns the configured Hertz binding policy for business JSON APIs.
func NewBindConfig() *binding.BindConfig {
	return newConfiguredBindConfig()
}

func newConfiguredBindConfig() *binding.BindConfig {
	config := binding.NewBindConfig()
	config.UseThirdPartyJSONUnmarshaler(jsoncompat.Unmarshal)
	return config
}
