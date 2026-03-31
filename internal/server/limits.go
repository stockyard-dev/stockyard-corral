package server

import "github.com/stockyard-dev/stockyard-corral/internal/license"

// Limits holds the feature limits for the current license tier.
// All int limits: 0 means unlimited (Pro tier only).
type Limits struct {
	MaxEndpoints int // 0 = unlimited (Pro)
	MaxEventsPerMonth int // 0 = unlimited (Pro)
	RetentionDays int // 0 = unlimited (Pro)
	MaxForwardTargets int // 0 = unlimited (Pro)
	ReplayHistory bool
	RetryDeliveries bool
	EventSearch bool
	ExportJSON bool
}

var freeLimits = Limits{
		MaxEndpoints: 3,
		MaxEventsPerMonth: 1000,
		RetentionDays: 7,
		MaxForwardTargets: 1,
		ReplayHistory: false,
		RetryDeliveries: false,
		EventSearch: false,
		ExportJSON: false,
}

var proLimits = Limits{
		MaxEndpoints: 0,
		MaxEventsPerMonth: 0,
		RetentionDays: 90,
		MaxForwardTargets: 0,
		ReplayHistory: true,
		RetryDeliveries: true,
		EventSearch: true,
		ExportJSON: true,
}

// LimitsFor returns the appropriate Limits for the given license info.
// nil info = no key set = free tier.
func LimitsFor(info *license.Info) Limits {
	if info != nil && info.IsPro() {
		return proLimits
	}
	return freeLimits
}

// LimitReached returns true if the current count meets or exceeds the limit.
// A limit of 0 is treated as unlimited.
func LimitReached(limit, current int) bool {
	if limit == 0 {
		return false
	}
	return current >= limit
}
