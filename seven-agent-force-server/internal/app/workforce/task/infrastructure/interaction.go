package infrastructure

import interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"

func (s *Store) SetInteractionLifecycle(lifecycle interaction.AttemptLifecycle) {
	s.interactions = lifecycle
}
