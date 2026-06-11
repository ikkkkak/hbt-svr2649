package routes

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"sync"

	"github.com/kataras/iris/v12"
)

const maxPropertySaleCreateBody = 3 * 1024 * 1024 // 3MB JSON max

// PropertySaleCreateJob tracks async create progress for mobile polling.
type PropertySaleCreateJob struct {
	ID         string `json:"id"`
	Status     string `json:"status"` // pending | processing | completed | failed
	Percent    int    `json:"percent"`
	PropertyID uint   `json:"property_id,omitempty"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
	UserID     uint   `json:"-"`
}

type propertySaleCreateJobStore struct {
	mu   sync.RWMutex
	jobs map[string]*PropertySaleCreateJob
}

var propertySaleCreateJobs propertySaleCreateJobStore = propertySaleCreateJobStore{
	jobs: make(map[string]*PropertySaleCreateJob),
}

func newPropertySaleCreateJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *propertySaleCreateJobStore) register(job *PropertySaleCreateJob) {
	s.mu.Lock()
	s.jobs[job.ID] = job
	s.mu.Unlock()
}

func (s *propertySaleCreateJobStore) setPercent(id string, pct int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		if pct > j.Percent {
			j.Percent = pct
		}
		j.Status = "processing"
	}
}

func (s *propertySaleCreateJobStore) complete(id string, propertyID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Status = "completed"
		j.Percent = 100
		j.PropertyID = propertyID
		j.Message = "Property created successfully"
	}
	log.Printf("✅ create-job %s completed property_id=%d", id, propertyID)
}

func (s *propertySaleCreateJobStore) fail(id, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		j.Status = "failed"
		j.Error = errMsg
	}
	log.Printf("❌ create-job %s failed: %s", id, errMsg)
}

func runPropertySaleCreateJob(jobID string, userID uint, body []byte) {
	log.Printf("🏗️ create-job %s processing (%d bytes)", jobID, len(body))
	var payload PropertySaleCreatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		propertySaleCreateJobs.fail(jobID, "Invalid JSON: "+err.Error())
		return
	}

	propertySaleCreateJobs.setPercent(jobID, 58)
	pid, err := ExecuteCreatePropertySale(userID, &payload, func(p int) {
		propertySaleCreateJobs.setPercent(jobID, p)
	})
	if err != nil {
		propertySaleCreateJobs.fail(jobID, err.Error())
		return
	}
	propertySaleCreateJobs.complete(jobID, pid)
}

func getPropertySaleCreateJob(id string, userID uint) (*PropertySaleCreateJob, bool) {
	propertySaleCreateJobs.mu.RLock()
	defer propertySaleCreateJobs.mu.RUnlock()
	j, ok := propertySaleCreateJobs.jobs[id]
	if !ok || j.UserID != userID {
		return nil, false
	}
	copy := *j
	return &copy, true
}

// PostPropertySaleCreateJob enqueues async create and responds 202 immediately.
// POST /api/property-sales/create-jobs/
func PostPropertySaleCreateJob(ctx iris.Context) {
	log.Printf(
		"🚀 POST create-jobs HIT path=%q method=%s content-length=%s",
		ctx.Path(),
		ctx.Method(),
		ctx.GetHeader("Content-Length"),
	)

	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		log.Printf("❌ POST create-jobs unauthorized (no userID in context)")
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	body, err := ctx.GetBody()
	if err != nil {
		log.Printf("❌ POST create-jobs read body: %v", err)
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "Failed to read body"})
		return
	}
	if len(body) > maxPropertySaleCreateBody {
		ctx.StatusCode(iris.StatusRequestEntityTooLarge)
		ctx.JSON(iris.Map{"error": "Payload too large — upload media first, send URLs only"})
		return
	}

	jobID := newPropertySaleCreateJobID()
	job := &PropertySaleCreateJob{
		ID:      jobID,
		UserID:  userID,
		Status:  "pending",
		Percent: 58,
	}
	propertySaleCreateJobs.register(job)

	// Respond before DB work so the client can start polling immediately.
	ctx.StatusCode(iris.StatusAccepted)
	ctx.JSON(iris.Map{
		"data": iris.Map{
			"job_id":  job.ID,
			"status":  job.Status,
			"percent": job.Percent,
		},
	})
	log.Printf("✅ POST create-jobs responded job_id=%s user=%d body_bytes=%d", jobID, userID, len(body))

	go runPropertySaleCreateJob(jobID, userID, body)
}

// GetPropertySaleCreateJob polls create progress (percent 58–100 from server).
// GET /api/property-sales/create-jobs/{jobId}
func GetPropertySaleCreateJob(ctx iris.Context) {
	userID, ok := ctx.Values().Get("userID").(uint)
	if !ok || userID == 0 {
		ctx.StatusCode(iris.StatusUnauthorized)
		ctx.JSON(iris.Map{"error": "unauthorized"})
		return
	}

	jobID := ctx.Params().Get("jobId")
	job, ok := getPropertySaleCreateJob(jobID, userID)
	if !ok {
		ctx.StatusCode(iris.StatusNotFound)
		ctx.JSON(iris.Map{"error": "Job not found"})
		return
	}

	ctx.JSON(iris.Map{"data": job})
}
