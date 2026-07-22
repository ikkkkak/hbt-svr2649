package routes

import (
	"time"

	"github.com/kataras/iris/v12"
)

// HabitatAPIVersion is a manual deploy fingerprint. BUMP THIS STRING in the
// same commit as any server change you want to verify reached production.
// It is echoed in tiles.json (and GET /habitat/version) as "api_version",
// so a single request confirms whether Dokploy actually rebuilt/restarted
// with the latest code — if the value below doesn't match what the live
// endpoint returns, the running binary is stale.
//
// Format: YYYY.MM.DD-N  (N = nth bump that day). Keep it short.
const HabitatAPIVersion = "2026.07.22-24"

// processStartedAt marks when THIS binary started — a fresh timestamp proves
// the process actually restarted (a redeploy), independent of the version
// string. Reset automatically on every boot.
var processStartedAt = time.Now().UTC()

// GET /api/habitat/version — no-cache deploy check.
// Returns the compiled version + how long this process has been running.
// A recent uptime with the expected api_version = the redeploy landed.
func GetHabitatAPIVersion(ctx iris.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.JSON(iris.Map{
		"api_version":    HabitatAPIVersion,
		"started_at":     processStartedAt.Format(time.RFC3339),
		"uptime_seconds": int(time.Since(processStartedAt).Seconds()),
	})
}
