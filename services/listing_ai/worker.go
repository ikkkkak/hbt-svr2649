package listing_ai

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

// Worker processes listing AI jobs asynchronously (in-process queue).
type Worker struct {
	gen  *Generator
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewWorker starts the background listing AI worker.
func NewWorker(gen *Generator) *Worker {
	w := &Worker{
		gen:  gen,
		jobs: make(map[string]*Job),
	}
	return w
}

func newJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Enqueue creates a job and processes it in a goroutine.
func (w *Worker) Enqueue(in GenerateInput, userID uint) *Job {
	id := newJobID()
	job := &Job{
		ID:       id,
		UserID:   userID,
		Status:   JobPending,
		Progress: "queued",
		Input:    in,
	}
	w.mu.Lock()
	w.jobs[id] = job
	w.mu.Unlock()

	go w.run(job)
	return job
}

func (w *Worker) run(job *Job) {
	w.setProgress(job.ID, JobProcessing, "matching_location")

	w.setProgress(job.ID, JobProcessing, "writing_listing")
	result, err := w.gen.Generate(job.Input)
	if err != nil {
		w.mu.Lock()
		if j, ok := w.jobs[job.ID]; ok {
			j.Status = JobFailed
			j.Progress = "failed"
			j.Error = err.Error()
		}
		w.mu.Unlock()
		RecordUsage(job.UserID, job.Input.Kind, "failed", job.ID)
		return
	}

	w.setProgress(job.ID, JobProcessing, "finalizing")

	w.mu.Lock()
	if j, ok := w.jobs[job.ID]; ok {
		j.Status = JobCompleted
		j.Progress = "done"
		j.Result = result
	}
	w.mu.Unlock()
	RecordUsage(job.UserID, job.Input.Kind, "completed", job.ID)
}

func (w *Worker) setProgress(id string, status JobStatus, progress string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if j, ok := w.jobs[id]; ok {
		j.Status = status
		j.Progress = progress
	}
}

// Get returns a job snapshot by id.
func (w *Worker) Get(id string) (*Job, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	j, ok := w.jobs[id]
	if !ok {
		return nil, false
	}
	copy := *j
	if j.Result != nil {
		r := *j.Result
		copy.Result = &r
	}
	return &copy, true
}

// Global worker instance (initialized from main).
var DefaultWorker *Worker
