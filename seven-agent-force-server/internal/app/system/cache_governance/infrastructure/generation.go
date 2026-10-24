package infrastructure

import (
	"context"
	"fmt"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

type GenerationAdapter struct {
	cache    cacheinfra.GovernedCache
	targeted cacheinfra.TargetedGovernedCache
	refresh  cacheinfra.GlobalRefreshGovernedCache
}

func NewGenerationAdapter(cache cacheinfra.GovernedCache) *GenerationAdapter {
	targeted, _ := cache.(cacheinfra.TargetedGovernedCache)
	refresh, _ := cache.(cacheinfra.GlobalRefreshGovernedCache)
	return &GenerationAdapter{cache: cache, targeted: targeted, refresh: refresh}
}

func (a *GenerationAdapter) AdvanceGlobalRefresh(ctx context.Context, eventID string) (bool, error) {
	if a == nil || a.refresh == nil {
		return false, fmt.Errorf("global cache refresh generation layer is not configured")
	}
	return a.refresh.AdvanceGlobalRefresh(ctx, eventID)
}

func (a *GenerationAdapter) MarkGlobalRefreshDirty(eventID string) {
	if a != nil && a.refresh != nil {
		a.refresh.MarkGlobalRefreshDirty(eventID)
	}
}

func (a *GenerationAdapter) EvictAllGovernedLocal(eventID string) {
	if a != nil && a.refresh != nil {
		a.refresh.EvictAllGovernedLocal(eventID)
	}
}

func (a *GenerationAdapter) SetGlobalRefreshFanoutHealthy(healthy bool) {
	if a != nil && a.cache != nil {
		a.cache.SetFanoutHealthy(healthy)
	}
}

func (a *GenerationAdapter) AdvanceTarget(ctx context.Context, eventID string, request cachepolicy.TargetedReadRequest) (bool, error) {
	if a == nil || a.targeted == nil {
		return false, fmt.Errorf("targeted cache generation layer is not configured")
	}
	return a.targeted.AdvanceTargetGeneration(ctx, eventID, request)
}
func (a *GenerationAdapter) MarkTargetDirty(eventID string, request cachepolicy.TargetedReadRequest) {
	if a != nil && a.targeted != nil {
		a.targeted.MarkTargetLocalDirty(eventID, request)
	}
}
func (a *GenerationAdapter) EvictTarget(eventID string, request cachepolicy.TargetedReadRequest) {
	if a != nil && a.targeted != nil {
		a.targeted.EvictTargetLocalAndResolve(eventID, request)
	}
}
func (a *GenerationAdapter) SetTargetFanoutHealthy(healthy bool) {
	if a != nil && a.cache != nil {
		a.cache.SetFanoutHealthy(healthy)
	}
}

func (a *GenerationAdapter) Advance(ctx context.Context, eventID string, dataClass cachepolicy.DataClass) (bool, error) {
	if a == nil || a.cache == nil {
		return false, fmt.Errorf("cache generation layer is not configured")
	}
	return a.cache.AdvanceGeneration(ctx, eventID, dataClass)
}

func (a *GenerationAdapter) MarkWriterDirty(eventID string, dataClass cachepolicy.DataClass) {
	if a != nil && a.cache != nil {
		a.cache.MarkLocalDirty(eventID, dataClass)
	}
}

func (a *GenerationAdapter) EvictAndResolve(eventID string, dataClass cachepolicy.DataClass) {
	if a != nil && a.cache != nil {
		a.cache.EvictLocalAndResolve(eventID, dataClass)
	}
}

func (a *GenerationAdapter) SetFanoutHealthy(healthy bool) {
	if a != nil && a.cache != nil {
		a.cache.SetFanoutHealthy(healthy)
	}
}

func (a *GenerationAdapter) RecordRejectedFanout() {
	if a != nil && a.cache != nil {
		a.cache.RecordRejectedFanout()
	}
}

var _ cachepolicy.GenerationPort = (*GenerationAdapter)(nil)
var _ cachepolicy.TargetedGenerationPort = (*GenerationAdapter)(nil)
var _ cachepolicy.RefreshGenerationPort = (*GenerationAdapter)(nil)
