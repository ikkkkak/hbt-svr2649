package mediaoptimize

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TranscodeJob represents a video queued for HLS transcoding
type TranscodeJob struct {
	JobID       string    // unique identifier
	VideoID     string    // business entity ID (used for DB updates)
	InputPath   string    // temp file path to upload
	UserID      int       // who uploaded this
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	// Callbacks
	OnSuccess func(hlsURL, thumbnailURL string)
	OnError   func(err error)
}

type JobStatus struct {
	JobID       string
	VideoID     string
	Status      string // "pending", "transcoding", "uploading", "completed", "failed"
	Progress    int    // 0-100
	ErrorMsg    string
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// TranscodeWorkerPool manages parallel HLS transcoding with a bounded goroutine pool
type TranscodeWorkerPool struct {
	workers    int
	jobQueue   chan TranscodeJob
	statusMap  map[string]JobStatus
	statusMutex sync.RWMutex
	wg         sync.WaitGroup
	// CDN uploader (S3/DO Spaces)
	cdnUploader CDNUploader
	// Redis-like cache for job status (optional)
	statusStore StatusStore
	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

// CDNUploader interface for uploading HLS files to storage
type CDNUploader interface {
	// UploadHLSStream uploads all HLS files to CDN and returns master URL
	UploadHLSStream(ctx context.Context, localDir string, videoID string, userID int) (masterURL, thumbnailURL string, err error)
	// DeleteHLSStream removes all files for a video from CDN
	DeleteHLSStream(ctx context.Context, videoID string) error
}

// StatusStore interface for persisting job status (optional; can be in-memory for MVP)
type StatusStore interface {
	SaveStatus(ctx context.Context, status JobStatus) error
	GetStatus(ctx context.Context, jobID string) (*JobStatus, error)
}

// NewTranscodeWorkerPool creates a new worker pool with `workerCount` parallel transcoding goroutines
// Default is 2-3 workers on a 2-4 CPU instance to avoid overwhelming the server
func NewTranscodeWorkerPool(workerCount int, cdnUploader CDNUploader) *TranscodeWorkerPool {
	if workerCount <= 0 {
		workerCount = 2
	}
	ctx, cancel := context.WithCancel(context.Background())
	pool := &TranscodeWorkerPool{
		workers:    workerCount,
		jobQueue:   make(chan TranscodeJob, 100), // buffer 100 jobs
		statusMap:  make(map[string]JobStatus),
		ctx:        ctx,
		cancel:     cancel,
		cdnUploader: cdnUploader,
	}

	// Start worker goroutines
	pool.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go pool.worker(i)
	}

	log.Printf("✅ HLS TranscodeWorkerPool started: %d workers, queue buffer: 100\n", workerCount)
	return pool
}

// Submit enqueues a job for transcoding (non-blocking)
// Returns immediately; progress is tracked via GetStatus()
func (p *TranscodeWorkerPool) Submit(job TranscodeJob) error {
	// Initialize status
	status := JobStatus{
		JobID:     job.JobID,
		VideoID:   job.VideoID,
		Status:    "pending",
		Progress:  0,
		CreatedAt: time.Now(),
	}

	p.statusMutex.Lock()
	p.statusMap[job.JobID] = status
	p.statusMutex.Unlock()

	select {
	case p.jobQueue <- job:
		return nil
	case <-p.ctx.Done():
		return fmt.Errorf("worker pool is shutting down")
	default:
		return fmt.Errorf("job queue is full (max 100 concurrent transcodes)")
	}
}

// GetStatus retrieves the current status of a transcoding job
func (p *TranscodeWorkerPool) GetStatus(jobID string) *JobStatus {
	p.statusMutex.RLock()
	defer p.statusMutex.RUnlock()
	if status, ok := p.statusMap[jobID]; ok {
		return &status
	}
	return nil
}

// Shutdown gracefully stops the worker pool, waiting for in-progress jobs to complete
// Timeout should be set to account for longest possible video (e.g., 10 minutes)
func (p *TranscodeWorkerPool) Shutdown(ctx context.Context) error {
	log.Println("🛑 HLS TranscodeWorkerPool shutting down...")
	p.cancel()
	close(p.jobQueue)

	// Wait for all workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("✅ HLS TranscodeWorkerPool shut down cleanly")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown timeout exceeded")
	}
}

// worker is the main loop for a transcoding goroutine
func (p *TranscodeWorkerPool) worker(id int) {
	defer p.wg.Done()
	log.Printf("🔧 HLS Worker %d started\n", id)

	for job := range p.jobQueue {
		p.processJob(job)
	}

	log.Printf("🔧 HLS Worker %d stopped\n", id)
}

// processJob executes the full transcoding pipeline for a single job
func (p *TranscodeWorkerPool) processJob(job TranscodeJob) {
	log.Printf("📹 [%s] HLS transcode started for video %s (user %d)\n", job.JobID, job.VideoID, job.UserID)

	// Mark as in-progress
	p.updateStatus(job.JobID, "transcoding", 10)
	now := time.Now()
	p.statusMutex.Lock()
	status, _ := p.statusMap[job.JobID]
	status.StartedAt = &now
	p.statusMap[job.JobID] = status
	p.statusMutex.Unlock()

	// Create temp directory for HLS output
	outputDir, err := os.MkdirTemp("", "hls_"+job.VideoID+"_")
	if err != nil {
		p.failJob(job, fmt.Errorf("mkdir temp: %w", err))
		return
	}
	defer os.RemoveAll(outputDir) // cleanup temp files after upload

	// Step 1: Transcode to HLS (60% of progress)
	log.Printf("📹 [%s] Starting FFmpeg HLS transcoding...\n", job.JobID)
	ctx, cancel := context.WithTimeout(p.ctx, 15*time.Minute) // max 15 min per video
	defer cancel()

	result, err := TranscodeToHLS(ctx, job.InputPath, outputDir)
	if err != nil {
		p.failJob(job, fmt.Errorf("transcode: %w", err))
		return
	}
	p.updateStatus(job.JobID, "uploading", 60)
	log.Printf("✅ [%s] FFmpeg transcoding complete: %d segments, %.1fs duration\n",
		job.JobID, len(result.Segments), result.Duration)

	// Step 2: Extract thumbnail (10% of progress)
	log.Printf("📷 [%s] Extracting thumbnail...\n", job.JobID)
	thumbPath := filepath.Join(outputDir, "thumb.jpg")
	if err := ExtractThumbnailFromVideo(ctx, job.InputPath, thumbPath); err != nil {
		log.Printf("⚠️  [%s] Thumbnail extraction failed: %v (continuing without)\n", job.JobID, err)
		// Non-fatal error; continue without thumbnail
	}
	p.updateStatus(job.JobID, "uploading", 70)

	// Step 3: Upload to CDN (30% of progress)
	log.Printf("☁️  [%s] Uploading HLS to CDN...\n", job.JobID)
	hlsURL, thumbURL, err := p.cdnUploader.UploadHLSStream(ctx, outputDir, job.VideoID, job.UserID)
	if err != nil {
		p.failJob(job, fmt.Errorf("upload CDN: %w", err))
		return
	}
	p.updateStatus(job.JobID, "completed", 100)

	// Mark as completed
	p.statusMutex.Lock()
	status, _ = p.statusMap[job.JobID]
	completed := time.Now()
	status.Status = "completed"
	status.Progress = 100
	status.CompletedAt = &completed
	p.statusMap[job.JobID] = status
	p.statusMutex.Unlock()

	log.Printf("✅ [%s] HLS transcoding COMPLETE\n  Master URL: %s\n  Thumbnail: %s\n",
		job.JobID, hlsURL, thumbURL)

	// Trigger callback
	if job.OnSuccess != nil {
		job.OnSuccess(hlsURL, thumbURL)
	}
}

// updateStatus updates job status atomically
func (p *TranscodeWorkerPool) updateStatus(jobID, status string, progress int) {
	p.statusMutex.Lock()
	defer p.statusMutex.Unlock()
	if s, ok := p.statusMap[jobID]; ok {
		s.Status = status
		s.Progress = progress
		p.statusMap[jobID] = s
	}
}

// failJob marks a job as failed and triggers error callback
func (p *TranscodeWorkerPool) failJob(job TranscodeJob, err error) {
	log.Printf("❌ [%s] HLS transcode FAILED: %v\n", job.JobID, err)

	p.statusMutex.Lock()
	status, _ := p.statusMap[job.JobID]
	now := time.Now()
	status.Status = "failed"
	status.ErrorMsg = err.Error()
	status.CompletedAt = &now
	p.statusMap[job.JobID] = status
	p.statusMutex.Unlock()

	if job.OnError != nil {
		job.OnError(err)
	}
}
