package infrastructure

// FrameworkConfigResolver is the only bridge from Result Processor runtime
// materialization to the Framework System Config secret facility. The bridge
// uses ConfigFacade's scoped internal-read API; it never sees ciphertext or
// reaches a Config repository directly.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
)

const (
	resultProcessorConfigConsumerID = "workforce.result-processor"
	resultProcessorConfigScope      = "server:workforce"
	resultProcessorConfigPurpose    = "one-cycle result processor evaluation"
)

type FrameworkConfigResolver struct {
	config configfacade.ConfigFacade
}

func NewFrameworkConfigResolver(config configfacade.ConfigFacade) *FrameworkConfigResolver {
	if config == nil {
		return nil
	}
	return &FrameworkConfigResolver{config: config}
}

func (r *FrameworkConfigResolver) RegisterResultProcessorSecretRef(_ context.Context, ref string) error {
	if r == nil || r.config == nil {
		return fmt.Errorf("Framework ConfigFacade is unavailable")
	}
	ref = strings.TrimSpace(ref)
	if !validConfigReference(ref) {
		return fmt.Errorf("secret ref must be a fully-qualified Framework config key")
	}
	r.config.RegisterConfigConsumers([]configfacade.ConfigConsumerRegistration{{
		ConsumerID:         resultProcessorConfigConsumerID,
		FullyQualifiedKey:  ref,
		ServerScope:        resultProcessorConfigScope,
		Purpose:            resultProcessorConfigPurpose,
		AllowedSensitivity: "SECRET",
		Source:             "workforce_result_processor_environment_binding",
		ActualConsumer:     "workforce Result Processor cycle materializer",
		Activation:         "per-evaluation-cycle",
		CacheRule:          "Framework ConfigFacade scope/version fence",
	}})
	return nil
}

func (r *FrameworkConfigResolver) VerifyResultProcessorSecretRef(ctx context.Context, ref, version, phase string) error {
	if strings.TrimSpace(phase) != "EVALUATE" {
		return fmt.Errorf("secret phase is not EVALUATE")
	}
	_, err := r.read(ctx, ref, version)
	return err
}

func (r *FrameworkConfigResolver) ResolveResultProcessorValue(ctx context.Context, _ int64, ref, version, phase string) (string, error) {
	if strings.TrimSpace(phase) != "EVALUATE" {
		return "", fmt.Errorf("secret phase is not EVALUATE")
	}
	return r.read(ctx, ref, version)
}

func (r *FrameworkConfigResolver) read(ctx context.Context, ref, expectedVersion string) (string, error) {
	if r == nil || r.config == nil {
		return "", fmt.Errorf("Framework ConfigFacade is unavailable")
	}
	ref = strings.TrimSpace(ref)
	if !validConfigReference(ref) || strings.TrimSpace(expectedVersion) == "" {
		return "", fmt.Errorf("secret reference identity is invalid")
	}
	value, err := r.config.GetConfigForConsumer(ctx, configfacade.ConfigInternalReadRequest{
		ConsumerID: resultProcessorConfigConsumerID, FullyQualifiedKey: ref,
		ServerScope: resultProcessorConfigScope, Purpose: resultProcessorConfigPurpose,
		AllowedSensitivity: "SECRET",
	})
	if err != nil {
		return "", err
	}
	if value == nil || strconv.FormatInt(value.Version, 10) != strings.TrimSpace(expectedVersion) {
		return "", fmt.Errorf("secret version does not match frozen identity")
	}
	plain, err := configValueString(value.Value)
	if err != nil {
		return "", err
	}
	if len(plain) > 64<<10 {
		return "", fmt.Errorf("secret value is oversized")
	}
	return plain, nil
}

func validConfigReference(ref string) bool {
	if len(ref) < 3 || len(ref) > 256 || strings.Count(ref, ".") < 1 || strings.ContainsAny(ref, " \t\r\n/\\") {
		return false
	}
	for _, segment := range strings.Split(ref, ".") {
		if segment == "" {
			return false
		}
		for index, runeValue := range segment {
			if !(runeValue == '_' || runeValue >= 'A' && runeValue <= 'Z' || runeValue >= 'a' && runeValue <= 'z' || index > 0 && runeValue >= '0' && runeValue <= '9') {
				return false
			}
		}
	}
	return true
}

func configValueString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case nil:
		return "", nil
	case bool, float64, float32, int, int64, int32, json.Number:
		body, err := json.Marshal(typed)
		return string(body), err
	default:
		body, err := json.Marshal(typed)
		if err != nil {
			return "", err
		}
		return string(body), nil
	}
}

var _ EnvironmentValueResolver = (*FrameworkConfigResolver)(nil)
var _ EnvironmentSecretBindingVerifier = (*FrameworkConfigResolver)(nil)
var _ EnvironmentSecretRegistrar = (*FrameworkConfigResolver)(nil)
