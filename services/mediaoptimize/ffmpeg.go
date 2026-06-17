package mediaoptimize

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ffmpegBin() string {
	if p := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); p != "" {
		return p
	}
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ""
	}
	return p
}

// FFmpegAvailable reports whether ffmpeg is on PATH or FFMPEG_PATH is set.
func FFmpegAvailable() bool {
	return ffmpegBin() != ""
}

func runFFmpeg(ctx context.Context, args ...string) error {
	bin := ffmpegBin()
	if bin == "" {
		return fmt.Errorf("ffmpeg not found (set FFMPEG_PATH)")
	}
	full := append([]string{}, args...)
	cmd := exec.CommandContext(ctx, bin, full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 800 {
			out = out[:800]
		}
		return fmt.Errorf("ffmpeg: %w: %s", err, string(out))
	}
	return nil
}
