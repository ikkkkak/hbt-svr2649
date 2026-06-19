package routes

import (
	"context"
	"runtime"
	"sync/atomic"
	"time"

	"apartments-clone-server/storage"

	"github.com/kataras/iris/v12"
)

var (
	serverStartedAt   = time.Now()
	activeHTTPRequests int64
)

// IncActiveRequest increments in-flight HTTP counter (middleware).
func IncActiveRequest() { atomic.AddInt64(&activeHTTPRequests, 1) }

// DecActiveRequest decrements in-flight HTTP counter (middleware).
func DecActiveRequest() { atomic.AddInt64(&activeHTTPRequests, -1) }

func activeRequestCount() int64 {
	return atomic.LoadInt64(&activeHTTPRequests)
}

// HealthLive — process is up (Render liveness probe).
func HealthLive(ctx iris.Context) {
	ctx.JSON(iris.Map{
		"ok":     true,
		"code":   "SERVER_OK",
		"status": "ok",
		"uptime": time.Since(serverStartedAt).Round(time.Second).String(),
	})
}

// HealthReady — can this instance serve traffic? (DB must respond quickly).
func HealthReady(ctx iris.Context) {
	start := time.Now()
	dbOK, dbMs, dbDetail := pingDatabase(3 * time.Second)
	ready := dbOK
	code := iris.StatusOK
	if !ready {
		code = iris.StatusServiceUnavailable
	}
	ctx.StatusCode(code)
	ctx.JSON(iris.Map{
		"ok":        ready,
		"code":      ternary(ready, "READY", "NOT_READY"),
		"status":    ternary(ready, "ready", "degraded"),
		"db":        iris.Map{"ok": dbOK, "latency_ms": dbMs, "detail": dbDetail},
		"in_flight": activeRequestCount(),
		"elapsed_ms": time.Since(start).Milliseconds(),
	})
}

// HealthDeep — engineering diagnostics (pool pressure, redis, load).
func HealthDeep(ctx iris.Context) {
	start := time.Now()
	dbOK, dbMs, dbDetail := pingDatabase(5 * time.Second)
	pool := dbPoolStats()
	redisOK, redisDetail := pingRedis(2 * time.Second)

	degraded := !dbOK || activeRequestCount() > int64(pool.MaxOpen)*8/10
	ctx.StatusCode(ternaryIris(degraded, iris.StatusServiceUnavailable, iris.StatusOK))
	ctx.JSON(iris.Map{
		"ok":         dbOK,
		"code":       ternary(dbOK && !degraded, "HEALTHY", "DEGRADED"),
		"status":     ternary(dbOK && !degraded, "healthy", "degraded"),
		"uptime":     time.Since(serverStartedAt).Round(time.Second).String(),
		"started_at": serverStartedAt.UTC().Format(time.RFC3339),
		"in_flight":  activeRequestCount(),
		"goroutines": runtime.NumGoroutine(),
		"db": iris.Map{
			"ok":         dbOK,
			"latency_ms": dbMs,
			"detail":     dbDetail,
			"pool":       pool,
		},
		"redis": iris.Map{
			"ok":     redisOK,
			"detail": redisDetail,
		},
		"hints": buildHealthHints(dbOK, dbMs, pool, activeRequestCount()),
		"elapsed_ms": time.Since(start).Milliseconds(),
	})
}

type poolSnapshot struct {
	MaxOpen           int   `json:"max_open"`
	Open              int   `json:"open"`
	InUse             int   `json:"in_use"`
	Idle              int   `json:"idle"`
	WaitCount         int64 `json:"wait_count"`
	WaitDurationMs    int64 `json:"wait_duration_ms"`
	MaxIdleClosed     int64 `json:"max_idle_closed"`
	MaxLifetimeClosed int64 `json:"max_lifetime_closed"`
}

func dbPoolStats() poolSnapshot {
	out := poolSnapshot{}
	if storage.DB == nil {
		return out
	}
	sqlDB, err := storage.DB.DB()
	if err != nil || sqlDB == nil {
		return out
	}
	st := sqlDB.Stats()
	out = poolSnapshot{
		MaxOpen:           st.MaxOpenConnections,
		Open:              st.OpenConnections,
		InUse:             st.InUse,
		Idle:              st.Idle,
		WaitCount:         st.WaitCount,
		WaitDurationMs:    st.WaitDuration.Milliseconds(),
		MaxIdleClosed:     st.MaxIdleClosed,
		MaxLifetimeClosed: st.MaxLifetimeClosed,
	}
	return out
}

func pingDatabase(timeout time.Duration) (bool, int64, string) {
	if storage.DB == nil {
		return false, 0, "database not initialized"
	}
	sqlDB, err := storage.DB.DB()
	if err != nil || sqlDB == nil {
		return false, 0, "sql.DB unavailable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	t0 := time.Now()
	if err := sqlDB.PingContext(ctx); err != nil {
		return false, time.Since(t0).Milliseconds(), err.Error()
	}
	return true, time.Since(t0).Milliseconds(), "pong"
}

func pingRedis(timeout time.Duration) (bool, string) {
	if storage.Redis == nil {
		return false, "redis not initialized"
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := storage.Redis.Ping(ctx).Err(); err != nil {
		return false, err.Error()
	}
	return true, "pong"
}

func buildHealthHints(dbOK bool, dbMs int64, pool poolSnapshot, inFlight int64) []string {
	var hints []string
	if !dbOK {
		hints = append(hints, "Database unreachable — all API routes will stall or 500.")
	}
	if dbMs > 2000 {
		hints = append(hints, "DB ping slow — connection pool may be exhausted; check long-running queries.")
	}
	if pool.MaxOpen > 0 && pool.InUse >= pool.MaxOpen-2 {
		hints = append(hints, "DB pool nearly full (in_use ≈ max_open) — new requests queue until a connection frees.")
	}
	if pool.WaitCount > 0 {
		hints = append(hints, "Requests already waited for DB connections — increase pool or kill slow SQL.")
	}
	if inFlight > 50 {
		hints = append(hints, "High in-flight HTTP count — instance may be overloaded or clients are polling aggressively.")
	}
	if len(hints) == 0 {
		hints = append(hints, "All checks nominal.")
	}
	return hints
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func ternaryIris(cond bool, a, b int) int {
	if cond {
		return a
	}
	return b
}
