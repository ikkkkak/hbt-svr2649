package mediaoptimize

import (
	"os"
	"strconv"
	"strings"
)

// Config tunes mobile-first compression (CRF + caps). Override via env in production.
type Config struct {
	Enabled           bool
	UploadFastPath    bool // skip FFmpeg on broker upload path — stream to CDN (default on)
	VideoCRF         int    // mezzanine / master normalize (23–28; 26 default)
	VideoMobileCRF   int    // progressive mobile MP4
	VideoPreset      string // veryfast on upload path (host-facing latency)
	VideoFastCRF     int    // slightly higher CRF on upload = faster encode
	UploadSkipBelowMB int   // skip FFmpeg if source already smaller (bytes saved negligible)
	VideoMaxWidth    int    // portrait width cap
	VideoMaxHeight   int    // portrait height cap
	VideoFPS         int
	VideoAudioKbps   int
	ImageMaxEdge     int
	ImageWebPQuality int // 1–100, ~80–85 sweet spot
	ImageThumbEdge   int
}

func LoadConfig() Config {
	return Config{
		Enabled:           envBool("MEDIA_OPTIMIZE_ENABLED", true),
		UploadFastPath:    envBool("MEDIA_UPLOAD_FAST_PATH", true),
		VideoCRF:         envInt("MEDIA_VIDEO_CRF", 26),
		VideoMobileCRF:   envInt("MEDIA_VIDEO_MOBILE_CRF", 28),
		VideoPreset:      envStr("MEDIA_VIDEO_PRESET", "veryfast"),
		VideoFastCRF:     envInt("MEDIA_VIDEO_FAST_CRF", 27),
		UploadSkipBelowMB: envInt("MEDIA_UPLOAD_SKIP_OPTIMIZE_BELOW_MB", 8),
		VideoMaxWidth:    envInt("MEDIA_VIDEO_MAX_WIDTH", 1080),
		VideoMaxHeight:   envInt("MEDIA_VIDEO_MAX_HEIGHT", 1920),
		VideoFPS:         envInt("MEDIA_VIDEO_FPS", 30),
		VideoAudioKbps:   envInt("MEDIA_VIDEO_AUDIO_KBPS", 96),
		ImageMaxEdge:     envInt("MEDIA_IMAGE_MAX_EDGE", 2048),
		ImageWebPQuality: envInt("MEDIA_IMAGE_WEBP_QUALITY", 82),
		ImageThumbEdge:   envInt("MEDIA_IMAGE_THUMB_EDGE", 720),
	}
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envStr(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}
