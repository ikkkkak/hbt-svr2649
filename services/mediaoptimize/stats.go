package mediaoptimize

import (
	"fmt"
	"log"
	"os"
)

// Result captures size reduction for observability.
type Result struct {
	Kind           string  // video | image
	OriginalBytes  int64
	OptimizedBytes int64
	OutputPath     string
	OutputMIME     string
	Skipped        bool
	SkipReason     string
}

func (r Result) Ratio() float64 {
	if r.OriginalBytes <= 0 || r.OptimizedBytes <= 0 {
		return 1
	}
	return float64(r.OptimizedBytes) / float64(r.OriginalBytes)
}

func (r Result) SavedPercent() float64 {
	if r.OriginalBytes <= 0 {
		return 0
	}
	return (1 - r.Ratio()) * 100
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

func logCompression(r Result) {
	if r.Skipped {
		log.Printf("📷 mediaoptimize %s skipped (%s) orig=%s",
			r.Kind, r.SkipReason, formatBytes(r.OriginalBytes))
		return
	}
	log.Printf("📷 mediaoptimize %s orig=%s → opt=%s (%.1f%% smaller, ratio=%.2f)",
		r.Kind,
		formatBytes(r.OriginalBytes),
		formatBytes(r.OptimizedBytes),
		r.SavedPercent(),
		r.Ratio(),
	)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for n2 := n / unit; n2 >= unit; n2 /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(n)/float64(div), "KMGTPE"[exp])
}
