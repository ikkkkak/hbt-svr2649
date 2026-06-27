package videoprocessing

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	slideshowWidth       = 1080
	slideshowHeight      = 1920
	slideshowFPS         = 30
	slideshowSecPerSlide = 4.0
	slideshowTransition  = 0.45
)

// SlideshowInput is everything FFmpeg needs to render a vertical marketing clip.
type SlideshowInput struct {
	ImagePaths  []string
	MusicPath   string // optional local path; empty = silent
	OutputPath  string
	Title       string
	Location    string
	Area        string
	Price       string
	CTA         string
	SecPerSlide float64
}

// GenerateSlideshowMP4 builds a 9:16 MP4 with Ken Burns motion, fades, text, and optional music.
func GenerateSlideshowMP4(ctx context.Context, in SlideshowInput) error {
	if len(in.ImagePaths) == 0 {
		return fmt.Errorf("slideshow: no images")
	}
	if in.SecPerSlide <= 0 {
		in.SecPerSlide = slideshowSecPerSlide
	}
	ffmpeg := ffmpegPath()
	if ffmpeg == "" {
		return fmt.Errorf("slideshow: ffmpeg not found (set FFMPEG_PATH)")
	}

	workDir := filepath.Dir(in.OutputPath)
	clips := make([]string, 0, len(in.ImagePaths))
	for i, img := range in.ImagePaths {
		clipPath := filepath.Join(workDir, fmt.Sprintf("slide_%03d.mp4", i))
		if err := renderKenBurnsClip(ctx, ffmpeg, img, clipPath, in.SecPerSlide, i); err != nil {
			return fmt.Errorf("slide %d: %w", i+1, err)
		}
		clips = append(clips, clipPath)
	}

	mergedSilent := filepath.Join(workDir, "merged_silent.mp4")
	if err := concatClipsWithFade(ctx, ffmpeg, clips, mergedSilent, in.SecPerSlide); err != nil {
		return err
	}

	withText := filepath.Join(workDir, "with_text.mp4")
	if err := burnTextOverlays(ctx, ffmpeg, mergedSilent, withText, in); err != nil {
		return err
	}

	if strings.TrimSpace(in.MusicPath) != "" {
		if err := muxMusic(ctx, ffmpeg, withText, in.MusicPath, in.OutputPath); err != nil {
			return err
		}
		return nil
	}
	return copyFile(withText, in.OutputPath)
}

func renderKenBurnsClip(ctx context.Context, ffmpeg, imagePath, outPath string, durationSec float64, index int) error {
	dFrames := int(durationSec * slideshowFPS)
	if dFrames < slideshowFPS {
		dFrames = slideshowFPS
	}

	var zoomExpr, xExpr, yExpr string
	switch index % 4 {
	case 0:
		zoomExpr = "min(zoom+0.0012,1.28)"
		xExpr = "iw/2-(iw/zoom/2)"
		yExpr = "ih/2-(ih/zoom/2)"
	case 1:
		zoomExpr = "if(lte(zoom,1.0),1.25,max(zoom-0.0012,1.0))"
		xExpr = "iw/2-(iw/zoom/2)"
		yExpr = "ih/2-(ih/zoom/2)"
	case 2:
		zoomExpr = "1.15"
		xExpr = "x+1.5"
		yExpr = "ih/2-(ih/zoom/2)"
	default:
		zoomExpr = "1.15"
		xExpr = "x-1.5"
		yExpr = "ih/2-(ih/zoom/2)"
	}

	zp := fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,"+
			"zoompan=z='%s':x='%s':y='%s':d=%d:s=%dx%d:fps=%d,"+
			"format=yuv420p",
		slideshowWidth, slideshowHeight, slideshowWidth, slideshowHeight,
		zoomExpr, xExpr, yExpr, dFrames, slideshowWidth, slideshowHeight, slideshowFPS,
	)

	args := []string{
		"-y", "-loop", "1", "-i", imagePath,
		"-vf", zp,
		"-t", fmt.Sprintf("%.3f", durationSec),
		"-c:v", "libx264", "-preset", "medium", "-crf", "22",
		"-pix_fmt", "yuv420p", "-an", outPath,
	}
	out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg kenburns: %w: %s", err, trimFFmpegOut(out))
	}
	return nil
}

func concatClipsWithFade(ctx context.Context, ffmpeg string, clips []string, outPath string, secPerSlide float64) error {
	if len(clips) == 1 {
		return copyFile(clips[0], outPath)
	}

	var inputs []string
	for _, c := range clips {
		inputs = append(inputs, "-i", c)
	}

	var filter strings.Builder
	prev := "[0:v]"
	offset := secPerSlide - slideshowTransition
	for i := 1; i < len(clips); i++ {
		outLabel := "[vout]"
		if i < len(clips)-1 {
			outLabel = fmt.Sprintf("[vx%d]", i)
		}
		if i > 1 {
			filter.WriteString(";")
		}
		fmt.Fprintf(&filter, "%s[%d:v]xfade=transition=fade:duration=%.2f:offset=%.2f%s",
			prev, i, slideshowTransition, offset, outLabel)
		prev = outLabel
		offset += secPerSlide - slideshowTransition
	}

	args := append([]string{"-y"}, inputs...)
	args = append(args,
		"-filter_complex", filter.String(),
		"-map", "[vout]",
		"-c:v", "libx264", "-preset", "medium", "-crf", "22",
		"-pix_fmt", "yuv420p", "-an", outPath,
	)
	_, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput()
	if err != nil {
		return concatClipsSimple(ctx, ffmpeg, clips, outPath)
	}
	return nil
}

func concatClipsSimple(ctx context.Context, ffmpeg string, clips []string, outPath string) error {
	listPath := filepath.Join(filepath.Dir(outPath), "concat.txt")
	var b strings.Builder
	for _, c := range clips {
		b.WriteString("file '")
		b.WriteString(strings.ReplaceAll(c, "'", `'\\''`))
		b.WriteString("'\n")
	}
	if err := os.WriteFile(listPath, []byte(b.String()), 0644); err != nil {
		return err
	}
	args := []string{
		"-y", "-f", "concat", "-safe", "0", "-i", listPath,
		"-c:v", "libx264", "-preset", "medium", "-crf", "22",
		"-pix_fmt", "yuv420p", "-an", outPath,
	}
	out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat: %w: %s", err, trimFFmpegOut(out))
	}
	return nil
}

func burnTextOverlays(ctx context.Context, ffmpeg, inPath, outPath string, in SlideshowInput) error {
	font := slideshowFontPath()
	title := escapeDrawtext(truncateRunes(in.Title, 48))
	loc := escapeDrawtext(truncateRunes(in.Location, 40))
	area := escapeDrawtext(strings.TrimSpace(in.Area))
	price := escapeDrawtext(strings.TrimSpace(in.Price))
	cta := escapeDrawtext(truncateRunes(in.CTA, 32))
	if cta == "" {
		cta = escapeDrawtext("Meskeny")
	}

	filter := fmt.Sprintf(
		"[0:v]drawbox=x=0:y=h*0.62:w=iw:h=h*0.38:color=black@0.45:t=fill,"+
			"drawtext=fontfile='%s':text='%s':fontsize=52:fontcolor=white:x=(w-text_w)/2:y=h*0.66:shadowcolor=black@0.6:shadowx=2:shadowy=2,"+
			"drawtext=fontfile='%s':text='%s':fontsize=36:fontcolor=white@0.95:x=(w-text_w)/2:y=h*0.74,"+
			"drawtext=fontfile='%s':text='%s':fontsize=32:fontcolor=0xFFD166:x=(w-text_w)/2:y=h*0.80,"+
			"drawtext=fontfile='%s':text='%s':fontsize=40:fontcolor=white:x=(w-text_w)/2:y=h*0.86,"+
			"drawtext=fontfile='%s':text='%s':fontsize=28:fontcolor=white@0.9:x=(w-text_w)/2:y=h*0.92[v]",
		font, title, font, loc, font, area, font, price, font, cta,
	)

	args := []string{
		"-y", "-i", inPath,
		"-filter_complex", filter,
		"-map", "[v]",
		"-c:v", "libx264", "-preset", "medium", "-crf", "22",
		"-pix_fmt", "yuv420p", "-an", outPath,
	}
	out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg drawtext: %w: %s", err, trimFFmpegOut(out))
	}
	return nil
}

func muxMusic(ctx context.Context, ffmpeg, videoPath, musicPath, outPath string) error {
	args := []string{
		"-y", "-i", videoPath, "-i", musicPath,
		"-filter_complex", "[1:a]volume=0.32[a]",
		"-map", "0:v", "-map", "[a]",
		"-c:v", "copy", "-c:a", "aac", "-b:a", "128k",
		"-shortest", outPath,
	}
	out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg mux: %w: %s", err, trimFFmpegOut(out))
	}
	return nil
}

func slideshowFontPath() string {
	if p := strings.TrimSpace(os.Getenv("SLIDESHOW_FONT_PATH")); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	candidates := []string{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
		"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
		"C:\\Windows\\Fonts\\arialbd.ttf",
		"/System/Library/Fonts/Supplemental/Arial Bold.ttf",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "DejaVuSans-Bold"
}

func escapeDrawtext(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `:`, `\:`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return s
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func trimFFmpegOut(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 400 {
		return s[len(s)-400:]
	}
	return s
}

func copyFile(src, dst string) error {
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0644)
}

// SlideshowDuration estimates total runtime for progress UI.
func SlideshowDuration(imageCount int, secPerSlide float64) time.Duration {
	if secPerSlide <= 0 {
		secPerSlide = slideshowSecPerSlide
	}
	if imageCount <= 1 {
		return time.Duration(secPerSlide * float64(time.Second))
	}
	total := float64(imageCount)*secPerSlide - float64(imageCount-1)*slideshowTransition
	return time.Duration(math.Max(total, secPerSlide) * float64(time.Second))
}
