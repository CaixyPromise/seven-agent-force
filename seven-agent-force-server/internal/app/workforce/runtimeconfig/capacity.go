package config

// RequiredContainerBudget covers normal concurrent execution: each Agent may
// retain its Supervisor and Provider Proxy while a Current check runs; each
// Reviewer and Custom Delivery may retain one proxy alongside its runner.
// Delivery's existing zero value means one concurrent delivery, not disabled.
// This is a configuration budget, not a daemon-wide cleanup guarantee.
func RequiredContainerBudget(agents, reviews, deliveries int) int {
	// Values outside the supported total cannot produce a valid budget. Bound
	// before multiplication so hostile configuration integers cannot overflow.
	if agents < 0 || reviews < 0 || deliveries < 0 || agents > 128 || reviews > 128 || deliveries > 128 {
		return 129
	}
	if deliveries == 0 {
		deliveries = 1
	}
	return 3*agents + 2*reviews + 2*deliveries
}
