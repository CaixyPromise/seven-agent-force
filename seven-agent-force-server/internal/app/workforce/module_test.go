package workforce

import (
	"testing"
	"time"

	workforceconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

func TestProviderGatewayContractIsCanonicalAndCoversEgressPolicy(t *testing.T) {
	base := workforceconfig.ProviderSSRFConfig{
		Enabled: true, MaxRequestBytes: 128 << 20, MaxResponseHeaderBytes: 256 << 10,
		RequestTimeout: 5 * time.Minute, ResponseHeaderTimeout: 2 * time.Minute, TLSHandshakeTimeout: 30 * time.Second,
		AllowedList: []workforceconfig.ProviderSSRFAllowEntry{
			{Hostname: "B.EXAMPLE", CIDRs: []string{"10.1.0.0/16", "10.0.0.0/8"}, Ports: []int{8443, 443}},
			{Hostname: "a.example", CIDRs: []string{"192.168.1.0/24"}, Ports: []int{443}},
		},
	}
	reordered := base
	reordered.AllowedList = []workforceconfig.ProviderSSRFAllowEntry{base.AllowedList[1], base.AllowedList[0]}
	reordered.AllowedList[1].CIDRs = []string{"10.0.0.0/8", "10.1.0.0/16"}
	reordered.AllowedList[1].Ports = []int{443, 8443}
	if providerGatewayContract(base) != providerGatewayContract(reordered) {
		t.Fatal("equivalent egress policies produced different Gateway contract identities")
	}
	changed := base
	changed.Enabled = false
	if providerGatewayContract(base) == providerGatewayContract(changed) {
		t.Fatal("runtime egress policy change did not change Gateway contract identity")
	}
}
