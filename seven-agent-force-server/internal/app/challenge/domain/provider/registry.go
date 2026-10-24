package provider

import "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"

type Registry struct {
	providers map[domain.ChallengeType]ChallengeStepProvider
}

func NewRegistry(items ...ChallengeStepProvider) *Registry {
	providers := make(map[domain.ChallengeType]ChallengeStepProvider, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		providers[item.Type()] = item
	}
	return &Registry{providers: providers}
}

func (r *Registry) Provider(challengeType domain.ChallengeType) (ChallengeStepProvider, bool) {
	if r == nil {
		return nil, false
	}
	item, ok := r.providers[challengeType]
	return item, ok
}
