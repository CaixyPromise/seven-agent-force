package microservice

import "sync"

type RoundRobin struct {
	mu       sync.Mutex
	counters map[string]uint64
}

func NewRoundRobin() *RoundRobin {
	return &RoundRobin{counters: make(map[string]uint64)}
}

func (r *RoundRobin) Select(serviceKey string, instances []ServiceInstance, excluded map[string]struct{}) (ServiceInstance, error) {
	if r == nil {
		return ServiceInstance{}, ErrInvalidDependency
	}
	eligible := make([]ServiceInstance, 0, len(instances))
	for _, instance := range instances {
		if !instance.Healthy {
			continue
		}
		_, excludedByIdentity := excluded[instance.IdentityKey()]
		_, excludedByID := excluded[instance.ID]
		if excludedByIdentity || excludedByID {
			continue
		}
		eligible = append(eligible, instance)
	}
	if len(eligible) == 0 {
		return ServiceInstance{}, ErrNoHealthyInstance
	}
	r.mu.Lock()
	if r.counters == nil {
		r.counters = make(map[string]uint64)
	}
	index := r.counters[serviceKey] % uint64(len(eligible))
	r.counters[serviceKey]++
	r.mu.Unlock()
	return eligible[index], nil
}
