package ai

import (
	"sync"
	"time"
)

type rateBucket struct {
	count int
	reset time.Time
}

var (
	agentRateMu sync.Mutex
	agentRates  = map[string]*rateBucket{}
)

// AllowAgentRun returns false when tier limits are exceeded.
func AllowAgentRun(key string, tier string) bool {
	limit := 30
	window := time.Hour
	switch tier {
	case "pro", "broker", "enterprise":
		return true
	case "free":
		limit = 60
	default: // anon
		limit = 15
	}
	agentRateMu.Lock()
	defer agentRateMu.Unlock()
	now := time.Now()
	b, ok := agentRates[key]
	if !ok || now.After(b.reset) {
		agentRates[key] = &rateBucket{count: 1, reset: now.Add(window)}
		return true
	}
	if b.count >= limit {
		return false
	}
	b.count++
	return true
}
