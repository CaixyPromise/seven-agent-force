package domain

import (
	"errors"
	"testing"
	"time"
)

func validGroupRoute() GroupRoute {
	return GroupRoute{ID: 1, OrganizationID: 2, DisplayName: "研发通知群", Provider: GroupRouteProviderFeishu,
		Transport: GroupRouteTransportFeishuAppChat, DeliveryChannelID: 3, RequiredPermissionCode: "workforce:task:create",
		TargetCiphertext: "cipher", TargetEDEK: "edek", TargetWrapKeyRef: "kid", TargetDigest: "digest", Status: GroupRouteDraft,
		RouteVersion: 1, ChannelVersion: 1, ConnectionVersion: 1, ConfigDigest: "digest", CreatorID: 4, UpdaterID: 4}
}

func TestGroupRouteRequiresEncryptedApplicationGroupTarget(t *testing.T) {
	route := validGroupRoute()
	route.TargetCiphertext = ""
	if !errors.Is(route.Validate(), ErrGroupRouteTargetInvalid) {
		t.Fatal("application group target without ciphertext was accepted")
	}
}

func TestGroupRouteRequiresProbeBeforeEnable(t *testing.T) {
	route := validGroupRoute()
	if !errors.Is(route.Enable(time.Now()), ErrGroupRouteDrift) {
		t.Fatal("unverified route was enabled")
	}
	route.MarkProbe(true, "", time.Now())
	if err := route.Enable(time.Now()); err != nil || route.Status != GroupRouteEnabled {
		t.Fatalf("enable verified route: status=%s err=%v", route.Status, err)
	}
}

func TestGroupRouteProviderTransportMustMatch(t *testing.T) {
	route := validGroupRoute()
	route.Provider = GroupRouteProviderWeCom
	if route.Transport.Valid() && route.Provider.Valid() && route.Validate() == nil {
		t.Fatal("WeCom route accepted Feishu application transport")
	}
}
