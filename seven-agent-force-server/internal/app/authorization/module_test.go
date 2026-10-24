package authorization

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestAuthorizationModuleDoesNotMountGetStepUpValidate(t *testing.T) {
	engine := server.Default()
	(&Module{}).Mount(engine)

	resp := ut.PerformRequest(engine.Engine, "GET", "/auth/step-up/validate?proofToken=proof-token-live", nil)

	if resp.Code == 200 {
		t.Fatalf("legacy GET step-up validation route must not be mounted; status=%d body=%s", resp.Code, resp.Body.String())
	}
}
