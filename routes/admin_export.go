package routes

import (
	"net/http"
	"sync"
	"time"

	"github.com/kataras/iris/v12"
)

type exportJob struct {
	ID        string `json:"id"`
	Resource  string `json:"resource"`
	Status    string `json:"status"` // pending, processing, done, failed
	CreatedAt int64  `json:"created_at"`
}

var (
	exportJobs   = map[string]*exportJob{}
	exportJobsMu sync.Mutex
)

// POST /admin/export { resource: string, filters: object }
func AdminCreateExport(ctx iris.Context) {
	var body struct {
		Resource string                 `json:"resource"`
		Filters  map[string]interface{} `json:"filters"`
	}
	if err := ctx.ReadJSON(&body); err != nil || body.Resource == "" {
		ctx.StatusCode(http.StatusUnprocessableEntity)
		ctx.JSON(iris.Map{"error": "invalid_payload", "message": "resource required"})
		return
	}
	id := time.Now().Format("20060102150405.000000")
	job := &exportJob{ID: id, Resource: body.Resource, Status: "pending", CreatedAt: time.Now().Unix()}
	exportJobsMu.Lock()
	exportJobs[id] = job
	exportJobsMu.Unlock()

	// Simulate async processing
	go func(j *exportJob) {
		j.Status = "processing"
		time.Sleep(500 * time.Millisecond)
		j.Status = "done"
	}(job)

	ctx.JSON(iris.Map{"data": iris.Map{"id": id, "status": job.Status}})
}

// GET /admin/export/:id
func AdminGetExport(ctx iris.Context) {
	id := ctx.Params().GetString("id")
	exportJobsMu.Lock()
	job, ok := exportJobs[id]
	exportJobsMu.Unlock()
	if !ok {
		ctx.StatusCode(http.StatusNotFound)
		ctx.JSON(iris.Map{"error": "not_found", "message": "job not found"})
		return
	}
	ctx.JSON(iris.Map{"data": job})
}
