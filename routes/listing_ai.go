package routes

import (
	"apartments-clone-server/services/listing_ai"

	"github.com/kataras/iris/v12"
)

// ListingAIWorker is set from main.go after startup.
var ListingAIWorker *listing_ai.Worker

// PostListingAIJob enqueues async listing generation (Add with AI).
// POST /api/listing-ai/jobs
func PostListingAIJob(ctx iris.Context) {
	if ListingAIWorker == nil {
		ctx.StatusCode(iris.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "Listing AI worker not ready"})
		return
	}

	var in listing_ai.GenerateInput
	if err := ctx.ReadJSON(&in); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}

	if in.Kind != listing_ai.KindRent && in.Kind != listing_ai.KindSale && in.Kind != listing_ai.KindLand {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "kind must be rent, sale, or land"})
		return
	}
	if len(in.Details) < 10 {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "details must be at least 10 characters"})
		return
	}
	if in.Currency == "" {
		in.Currency = "MRU"
	}
	if in.AreaUnit == "" {
		in.AreaUnit = "m²"
	}

	userID, _ := ctx.Values().Get("userID").(uint)
	job := ListingAIWorker.Enqueue(in, userID)
	listing_ai.RecordUsage(userID, in.Kind, "started", job.ID)
	ctx.JSON(iris.Map{
		"data": iris.Map{
			"job_id":   job.ID,
			"status":   job.Status,
			"progress": job.Progress,
		},
	})
}

// GetListingAIJob polls job status and result.
// GET /api/listing-ai/jobs/{jobId}
func GetListingAIJob(ctx iris.Context) {
	if ListingAIWorker == nil {
		ctx.StatusCode(iris.StatusServiceUnavailable)
		ctx.JSON(iris.Map{"error": "Listing AI worker not ready"})
		return
	}
	id := ctx.Params().Get("jobId")
	job, ok := ListingAIWorker.Get(id)
	if !ok {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Job not found"})
		return
	}
	ctx.JSON(iris.Map{"data": job})
}

// PostListingAIEvent records client-side funnel steps (e.g. published after review).
// POST /api/listing-ai/events
func PostListingAIEvent(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "Unauthorized"})
		return
	}

	var body struct {
		Kind  string `json:"kind"`
		Event string `json:"event"`
		JobID string `json:"job_id"`
	}
	if err := ctx.ReadJSON(&body); err != nil {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Invalid JSON"})
		return
	}
	kind := listing_ai.Kind(body.Kind)
	if kind != listing_ai.KindRent && kind != listing_ai.KindSale && kind != listing_ai.KindLand {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "kind must be rent, sale, or land"})
		return
	}
	if body.Event != "published" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "event must be published"})
		return
	}

	listing_ai.RecordUsage(userID, kind, "published", body.JobID)
	ctx.JSON(iris.Map{"data": iris.Map{"ok": true}})
}
