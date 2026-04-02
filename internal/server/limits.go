package server

// Limits holds the feature limits for the current tier.
// All int limits: 0 means unlimited.
type Limits struct {
	MaxEndpoints      int
	MaxEventsPerMonth int
	RetentionDays     int
	MaxForwardTargets int
	ReplayHistory     bool
	RetryDeliveries   bool
	EventSearch       bool
	ExportJSON        bool
}

// DefaultLimits returns fully-unlocked limits for the standalone edition.
func DefaultLimits() Limits {
	return Limits{
		MaxEndpoints:      0,
		MaxEventsPerMonth: 0,
		RetentionDays:     90,
		MaxForwardTargets: 0,
		ReplayHistory:     true,
		RetryDeliveries:   true,
		EventSearch:       true,
		ExportJSON:        true,
	}
}

// LimitReached returns true if the current count meets or exceeds the limit.
// A limit of 0 is treated as unlimited.
func LimitReached(limit, current int) bool {
	if limit == 0 {
		return false
	}
	return current >= limit
}
